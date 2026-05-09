package igdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/services/matching"
)

const (
	defaultIGDBAPIURL    = "https://api.igdb.com/v4"
	igdbImageBaseURL     = "https://images.igdb.com/igdb/image/upload/t_cover_big/"
	fuzzySearchThreshold = 0.6
)

// Client provides access to the IGDB API with rate limiting and authentication.
type Client struct {
	httpClient *http.Client
	auth       *AuthManager
	limiter    *rate.Limiter
	apiURL     string
	configured bool
}

// NewClient creates an IGDB client from config. If IGDB credentials are missing,
// returns a client that errors with ErrIGDBNotConfigured on all calls.
func NewClient(cfg *config.Config) *Client {
	if cfg.IGDBClientID == "" || cfg.IGDBClientSecret == "" {
		return &Client{configured: false}
	}

	interval := time.Duration(float64(time.Second) / cfg.IGDBRequestsPerSecond)
	limiter := rate.NewLimiter(rate.Every(interval), cfg.IGDBBurstCapacity)

	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		auth:       NewAuthManager(cfg.IGDBClientID, cfg.IGDBClientSecret, cfg.IGDBAccessToken),
		limiter:    limiter,
		apiURL:     defaultIGDBAPIURL,
		configured: true,
	}
}

// Configured reports whether the client has IGDB credentials.
func (c *Client) Configured() bool {
	return c.configured
}

// ValidateCredentials verifies the IGDB credentials by fetching a Twitch token.
// Returns ErrIGDBNotConfigured if credentials are absent, or an error (possibly
// wrapping ErrTwitchAuth) if authentication fails.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	if !c.configured {
		return ErrIGDBNotConfigured
	}
	_, err := c.auth.GetAccessToken(ctx)
	return err
}

// NewClientWithTokenURL creates an IGDB client with a custom Twitch token URL.
// Used in tests to point at a local httptest server.
func NewClientWithTokenURL(cfg *config.Config, tokenURL string) *Client {
	c := NewClient(cfg)
	if c.auth != nil {
		c.auth.tokenURL = tokenURL
	}
	return c
}

type scoredCandidate struct {
	metadata GameMetadata
	score    float64
}

// SearchGames implements the full IGDB search pipeline.
func (c *Client) SearchGames(ctx context.Context, query string, limit int) ([]GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	queries := expandQueries(query)
	original := queries[0]

	// Step 1: Concurrent search for original query (fuzzy + exact)
	var fuzzyResults, exactResults []igdbGameResponse
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		fuzzyResults, err = c.searchIGDB(gctx, fmt.Sprintf(`search "%s"; fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; limit %d;`, escapeIGDB(original), limit))
		return err
	})

	g.Go(func() error {
		var err error
		exactResults, err = c.searchIGDB(gctx, fmt.Sprintf(`fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; where name = "%s"; limit %d;`, escapeIGDB(original), limit))
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge: exact first, then fuzzy, deduplicate
	seen := make(map[int]bool)
	var merged []igdbGameResponse

	for _, game := range exactResults {
		if !seen[game.ID] {
			seen[game.ID] = true
			merged = append(merged, game)
		}
	}
	for _, game := range fuzzyResults {
		if !seen[game.ID] {
			seen[game.ID] = true
			merged = append(merged, game)
		}
	}

	// Step 2: Sequential expanded-query searches
	for _, expandedQuery := range queries[1:] {
		results, err := c.searchIGDB(ctx, fmt.Sprintf(`search "%s"; fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; limit %d;`, escapeIGDB(expandedQuery), limit))
		if err != nil {
			continue // Non-fatal
		}
		for _, game := range results {
			if !seen[game.ID] {
				seen[game.ID] = true
				merged = append(merged, game)
			}
		}
	}

	// Step 3: Convert to GameMetadata and post-rank
	normalizedQuery := matching.NormalizeTitle(query)
	var candidates []scoredCandidate

	for _, game := range merged {
		md := convertToGameMetadata(game)
		normalizedTitle := matching.NormalizeTitle(md.Title)
		score := matching.FuzzyConfidence(normalizedQuery, normalizedTitle)
		if score >= fuzzySearchThreshold {
			candidates = append(candidates, scoredCandidate{metadata: md, score: score})
		}
	}

	// Sort by score descending
	sortByScore(candidates)

	// Truncate to limit
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]GameMetadata, len(candidates))
	for i, c := range candidates {
		results[i] = c.metadata
	}
	return results, nil
}

// GetGameByID fetches a single game from IGDB (lightweight, no time-to-beat).
func (c *Client) GetGameByID(ctx context.Context, igdbID int) (*GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	query := fmt.Sprintf(`fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; where id = %d;`, igdbID)
	results, err := c.searchIGDB(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrGameNotFound
	}

	md := convertToGameMetadata(results[0])
	return &md, nil
}

// FetchFullMetadata fetches complete game data including time-to-beat.
func (c *Client) FetchFullMetadata(ctx context.Context, igdbID int) (*GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	// Fetch game data (same fields as GetGameByID)
	query := fmt.Sprintf(`fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; where id = %d;`, igdbID)
	results, err := c.searchIGDB(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrGameNotFound
	}

	md := convertToGameMetadata(results[0])

	// Fetch time-to-beat from separate endpoint
	ttb, err := c.fetchTimeToBeat(ctx, igdbID)
	if err == nil && ttb != nil {
		md.HowlongtobeatMain = ttb.hastily
		md.HowlongtobeatExtra = ttb.normally
		md.HowlongtobeatCompletionist = ttb.completely
	}
	// Non-fatal if time-to-beat fails — game still imports without it

	return &md, nil
}

// DownloadCoverArt downloads cover art from IGDB CDN to local storage.
// Returns the relative URL path (e.g. "/static/cover_art/abc123.jpg").
func (c *Client) DownloadCoverArt(ctx context.Context, imageID string, storagePath string) (string, error) {
	if imageID == "" {
		return "", nil
	}

	filename := imageID + ".jpg"
	localPath := filepath.Join(storagePath, "cover_art", filename)

	// Skip if already exists
	if _, err := os.Stat(localPath); err == nil {
		return "/static/cover_art/" + filename, nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", fmt.Errorf("create cover_art dir: %w", err)
	}

	url := igdbImageBaseURL + imageID + ".jpg"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download cover art: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cover art download returned status %d", resp.StatusCode)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	return "/static/cover_art/" + filename, nil
}

// searchIGDB executes a raw IGDB API query against the /games endpoint.
// timeToBeatResult holds the converted time-to-beat values in hours.
type timeToBeatResult struct {
	hastily    *float64
	normally   *float64
	completely *float64
}

// igdbTimeToBeatResponse matches the JSON from the /game_time_to_beats endpoint.
type igdbTimeToBeatResponse struct {
	Hastily    *int `json:"hastily"`
	Normally   *int `json:"normally"`
	Completely *int `json:"completely"`
}

// fetchTimeToBeat fetches time-to-beat data from the separate IGDB endpoint.
// Returns nil (no error) if no data exists for the game.
func (c *Client) fetchTimeToBeat(ctx context.Context, igdbID int) (*timeToBeatResult, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	token, err := c.auth.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`fields hastily, normally, completely; where game_id = %d;`, igdbID)
	url := c.apiURL + "/game_time_to_beats"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", c.auth.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB time-to-beat request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IGDB time-to-beat returned status %d", resp.StatusCode)
	}

	var data []igdbTimeToBeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	result := &timeToBeatResult{}
	if data[0].Hastily != nil {
		v := float64(*data[0].Hastily) / 3600.0
		result.hastily = &v
	}
	if data[0].Normally != nil {
		v := float64(*data[0].Normally) / 3600.0
		result.normally = &v
	}
	if data[0].Completely != nil {
		v := float64(*data[0].Completely) / 3600.0
		result.completely = &v
	}
	return result, nil
}

func (c *Client) searchIGDB(ctx context.Context, body string) ([]igdbGameResponse, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	token, err := c.auth.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	url := c.apiURL + "/games"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", c.auth.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 401 — invalidate token and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		c.auth.InvalidateToken()
		token, err = c.auth.GetAccessToken(ctx)
		if err != nil {
			return nil, err
		}

		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}

		req, _ = http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		req.Header.Set("Client-ID", c.auth.clientID)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "text/plain")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("IGDB retry failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IGDB API returned status %d", resp.StatusCode)
	}

	var games []igdbGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&games); err != nil {
		return nil, fmt.Errorf("decode IGDB response: %w", err)
	}
	return games, nil
}

// convertToGameMetadata converts an IGDB API response to our internal DTO.
func convertToGameMetadata(g igdbGameResponse) GameMetadata {
	md := GameMetadata{
		IgdbID:   g.ID,
		IgdbSlug: g.Slug,
		Title:    g.Name,
	}

	if g.Summary != nil {
		md.Description = g.Summary
	}

	if g.FirstReleaseDate != nil {
		t := time.Unix(*g.FirstReleaseDate, 0).UTC()
		dateStr := t.Format("2006-01-02")
		md.ReleaseDate = &dateStr
	}

	if g.Cover != nil && g.Cover.ImageID != "" {
		url := igdbImageBaseURL + g.Cover.ImageID + ".jpg"
		md.CoverArtURL = &url
	}

	if g.TotalRating != nil {
		md.RatingAverage = g.TotalRating
	}
	md.RatingCount = g.TotalRatingCount

	// Extract genre (first genre name)
	if len(g.Genres) > 0 {
		genre := g.Genres[0].Name
		md.Genre = &genre
	}

	// Extract developer and publisher
	for _, ic := range g.InvolvedCompanies {
		if ic.Developer && md.Developer == nil {
			name := ic.Company.Name
			md.Developer = &name
		}
		if ic.Publisher && md.Publisher == nil {
			name := ic.Company.Name
			md.Publisher = &name
		}
	}

	// Platform names
	for _, p := range g.Platforms {
		md.PlatformIDs = append(md.PlatformIDs, p.ID)
		md.PlatformNames = append(md.PlatformNames, p.Name)
	}

	// Game modes, themes, perspectives as comma-separated strings
	if len(g.GameModes) > 0 {
		names := namedItemNames(g.GameModes)
		md.GameModes = &names
	}
	if len(g.Themes) > 0 {
		names := namedItemNames(g.Themes)
		md.Themes = &names
	}
	if len(g.PlayerPerspectives) > 0 {
		names := namedItemNames(g.PlayerPerspectives)
		md.PlayerPerspectives = &names
	}

	return md
}

func namedItemNames(items []igdbNamedItem) string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return strings.Join(names, ", ")
}

func escapeIGDB(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// sortByScore sorts candidates by score descending (insertion sort — lists are small).
func sortByScore(items []scoredCandidate) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].score > items[j-1].score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
