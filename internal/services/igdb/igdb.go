package igdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/matching"
)

const (
	defaultIGDBAPIURL    = "https://api.igdb.com/v4"
	igdbImageBaseURL     = "https://images.igdb.com/igdb/image/upload/t_cover_big/"
	fuzzySearchThreshold = 0.6

	StatusOK                 = "ok"
	StatusNotConfigured      = "not_configured"
	StatusInvalidCredentials = "invalid_credentials" //nolint:gosec // status enum value, not a credential
)

// Client provides access to the IGDB API with rate limiting and authentication.
type Client struct {
	httpClient    *http.Client
	auth          *AuthManager
	limiter       ratelimit.Limiter
	apiURL        string
	configured    bool
	status        string
	maxRetries    int
	backoffFactor float64
}

// NewClient creates an IGDB client from config. If IGDB credentials are missing,
// returns a client that errors with ErrIGDBNotConfigured on all calls.
func NewClient(cfg *config.Config, limiter ratelimit.Limiter) *Client {
	if cfg.IGDBClientID == "" || cfg.IGDBClientSecret == "" {
		return &Client{configured: false, status: StatusNotConfigured}
	}

	return &Client{
		httpClient:    &http.Client{Timeout: 30 * time.Second, Transport: logging.NewRoundTripper(nil)},
		auth:          NewAuthManager(cfg.IGDBClientID, cfg.IGDBClientSecret, cfg.IGDBAccessToken),
		limiter:       limiter,
		apiURL:        defaultIGDBAPIURL,
		configured:    true,
		status:        StatusOK,
		maxRetries:    cfg.IGDBMaxRetries,
		backoffFactor: cfg.IGDBBackoffFactor,
	}
}

// NewInvalidCredentialsClient returns an unconfigured client that reports
// StatusInvalidCredentials — used when credentials are present but fail auth validation.
func NewInvalidCredentialsClient(limiter ratelimit.Limiter) *Client {
	return &Client{configured: false, limiter: limiter, status: StatusInvalidCredentials}
}

// Configured reports whether the client has IGDB credentials.
func (c *Client) Configured() bool {
	return c.configured
}

// Status returns the IGDB availability status: StatusOK, StatusNotConfigured,
// or StatusInvalidCredentials.
func (c *Client) Status() string {
	return c.status
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
func NewClientWithTokenURL(cfg *config.Config, tokenURL string, limiter ratelimit.Limiter) *Client {
	c := NewClient(cfg, limiter)
	if c.auth != nil {
		c.auth.tokenURL = tokenURL
	}
	return c
}

// SetAPIURLForTest overrides the IGDB API URL. For use in tests only.
func (c *Client) SetAPIURLForTest(url string) {
	c.apiURL = url
}

type scoredCandidate struct {
	metadata GameMetadata
	score    float64
}

const (
	// igdbGameFields is the common field list for all /games queries.
	// platforms.id is requested explicitly so SearchGames' post-filter can
	// intersect each candidate's platforms against the requested platformIDs
	// (issue #615) — relying on IGDB's "id always returned for reference
	// fields" default would be brittle.
	igdbGameFields = `name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.id,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name`
)

// SearchGames implements the full IGDB search pipeline. When platformIDs is
// non-empty, every IGDB query is scoped to those platforms; if the filtered
// search returns zero candidates, SearchGames retries once unfiltered (IGDB's
// platform tagging is incomplete and some legitimate titles lack PC tags).
func (c *Client) SearchGames(ctx context.Context, query string, limit int, platformIDs []int) ([]GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	whereSuffix, searchTail := buildPlatformsClause(platformIDs)

	queries := expandQueries(query)
	original := queries[0]

	// Step 1: Concurrent search for original query (fuzzy + exact).
	// No category filter — IGDB leaves category null on many legitimate games,
	// so an allowlist would silently exclude them. DLC pollution is handled by
	// post-ranking: exact name matches score 1.0 and beat DLC skins at 0.88.
	var fuzzyResults, exactResults []igdbGameResponse
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		fuzzyResults, err = c.searchIGDB(gctx, fmt.Sprintf(`search "%s"; fields %s; limit %d;%s`, escapeIGDB(original), igdbGameFields, limit, searchTail))
		return err
	})

	g.Go(func() error {
		var err error
		exactResults, err = c.searchIGDB(gctx, fmt.Sprintf(`fields %s; where name ~ "%s"%s; limit %d;`, igdbGameFields, escapeIGDB(original), whereSuffix, limit))
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge: exact first (highest precision), then fuzzy, deduplicate
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

	// Step 2: For each expanded query run exact search first, then fuzzy.
	// Exact search is critical here: e.g. "Batman™: Arkham Knight" expands to
	// "Batman: Arkham Knight", whose exact search finds the base game (score 1.0)
	// and beats DLC skins (score 0.88) in post-ranking.
	for _, expandedQuery := range queries[1:] {
		exactExp, err := c.searchIGDB(ctx, fmt.Sprintf(`fields %s; where name ~ "%s"%s; limit %d;`, igdbGameFields, escapeIGDB(expandedQuery), whereSuffix, limit))
		if err == nil {
			for _, game := range exactExp {
				if !seen[game.ID] {
					seen[game.ID] = true
					merged = append(merged, game)
				}
			}
		}
		fuzzyExp, err := c.searchIGDB(ctx, fmt.Sprintf(`search "%s"; fields %s; limit %d;%s`, escapeIGDB(expandedQuery), igdbGameFields, limit, searchTail))
		if err == nil {
			for _, game := range fuzzyExp {
				if !seen[game.ID] {
					seen[game.ID] = true
					merged = append(merged, game)
				}
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

	// Step 3.5: Post-filter by platform. IGDB's `where platforms = (...)` clause
	// is honored on exact-name queries but ignored on `search "..."` queries —
	// so the fuzzy half of our pipeline can leak wrong-platform candidates
	// (issue #615). Drop them in Go. Candidates with no platforms data are
	// kept (IGDB tagging is sometimes incomplete; same recall-preserving
	// philosophy as the empty-result fallback below).
	if len(platformIDs) > 0 {
		wanted := make(map[int]bool, len(platformIDs))
		for _, id := range platformIDs {
			wanted[id] = true
		}
		filtered := candidates[:0]
		dropped := 0
		for _, sc := range candidates {
			if len(sc.metadata.PlatformIDs) == 0 {
				filtered = append(filtered, sc)
				continue
			}
			match := false
			for _, pid := range sc.metadata.PlatformIDs {
				if wanted[pid] {
					match = true
					break
				}
			}
			if match {
				filtered = append(filtered, sc)
			} else {
				dropped++
			}
		}
		if dropped > 0 {
			slog.Debug("igdb: post-filter dropped wrong-platform candidates",
				"query", query, "platform_ids", platformIDs, "dropped", dropped, "kept", len(filtered))
		}
		candidates = filtered
	}

	sortByScore(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]GameMetadata, len(candidates))
	for i, c := range candidates {
		results[i] = c.metadata
	}

	// Empty-result fallback: if a platform filter was applied and produced no
	// candidates, retry once without the filter. IGDB's platform tagging is
	// incomplete; legitimate Steam games occasionally lack a PC platform tag.
	if len(results) == 0 && len(platformIDs) > 0 {
		slog.Debug("igdb: platform-filtered search returned 0 candidates, retrying unfiltered",
			"query", query, "platform_ids", platformIDs)
		return c.SearchGames(ctx, query, limit, nil)
	}

	return results, nil
}

// GetGameByID fetches a single game from IGDB (lightweight, no time-to-beat).
func (c *Client) GetGameByID(ctx context.Context, igdbID int) (*GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	query := fmt.Sprintf(`fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.id,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; where id = %d;`, igdbID)
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
	query := fmt.Sprintf(`fields name,slug,summary,first_release_date,cover.image_id,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,platforms.id,platforms.name,total_rating,total_rating_count,game_modes.name,themes.name,player_perspectives.name; where id = %d;`, igdbID)
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
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
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

	f, err := os.Create(localPath) //nolint:gosec // localPath is an internally-derived cover-art cache path, not user input
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
	body := fmt.Sprintf(`fields hastily, normally, completely; where game_id = %d;`, igdbID)
	resp, err := c.doPost(ctx, c.apiURL+"/game_time_to_beats", body)
	if err != nil {
		return nil, err
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
		result.hastily = clampHowlongtobeat(*data[0].Hastily)
	}
	if data[0].Normally != nil {
		result.normally = clampHowlongtobeat(*data[0].Normally)
	}
	if data[0].Completely != nil {
		result.completely = clampHowlongtobeat(*data[0].Completely)
	}
	return result, nil
}

// maxHowlongtobeat is the largest value the howlongtobeat_* columns can hold
// (NUMERIC(6,2) → 9999.99). Live-service titles can report completion times in
// hours that exceed this; clamping here prevents the games UPDATE from
// overflowing (SQLSTATE 22003) and rolling back the whole metadata enrichment.
const maxHowlongtobeat = 9999.99

// clampHowlongtobeat converts an IGDB time-to-beat value (seconds) to hours and
// caps it at the column maximum so an over-range value can never hard-fail
// enrichment. The capped value is distorted but lets the rest of the metadata
// populate.
func clampHowlongtobeat(seconds int) *float64 {
	v := float64(seconds) / 3600.0
	if v > maxHowlongtobeat {
		v = maxHowlongtobeat
	}
	return &v
}

func (c *Client) searchIGDB(ctx context.Context, body string) ([]igdbGameResponse, error) {
	resp, err := c.doPost(ctx, c.apiURL+"/games", body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IGDB API returned status %d", resp.StatusCode)
	}

	var games []igdbGameResponse
	if err := json.NewDecoder(resp.Body).Decode(&games); err != nil {
		return nil, fmt.Errorf("decode IGDB response: %w", err)
	}
	return games, nil
}

// doPost executes a POST against IGDB with rate limiting, auth, and transparent
// retries: a single 401 retry that refreshes the access token, plus up to
// c.maxRetries 429 retries that honor the Retry-After header (integer seconds)
// or fall back to exponential backoff scaled by c.backoffFactor. The caller
// owns resp.Body and must close it.
func (c *Client) doPost(ctx context.Context, url, body string) (*http.Response, error) {
	triedRefresh := false
	rateLimitRetries := 0
	for {
		resp, err := c.executePost(ctx, url, body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusUnauthorized && !triedRefresh {
			triedRefresh = true
			drainAndClose(resp.Body)
			c.auth.InvalidateToken()
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests && rateLimitRetries < c.maxRetries {
			d := c.retryAfterDelay(resp.Header.Get("Retry-After"), rateLimitRetries)
			rateLimitRetries++
			drainAndClose(resp.Body)
			if err := sleepCtx(ctx, d); err != nil {
				return nil, err
			}
			continue
		}
		return resp, nil
	}
}

func (c *Client) executePost(ctx context.Context, url, body string) (*http.Response, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	token, err := c.auth.GetAccessToken(ctx)
	if err != nil {
		return nil, err
	}
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
	return resp, nil
}

// retryAfterDelay returns the sleep duration before the next 429 retry.
// It honors a Retry-After header expressed as integer seconds (the IGDB form);
// otherwise it falls back to exponential backoff of c.backoffFactor * 2^attempt
// seconds. A non-positive result means "no sleep" (retry immediately).
func (c *Client) retryAfterDelay(header string, attempt int) time.Duration {
	if header != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if c.backoffFactor <= 0 {
		return 0
	}
	multiplier := 1 << attempt
	return time.Duration(c.backoffFactor * float64(multiplier) * float64(time.Second))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body) //nolint:errcheck // draining body for connection reuse
	_ = body.Close()
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
		md.CoverImageID = g.Cover.ImageID
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

// buildPlatformsClause builds Apicalypse fragments that scope a query to a
// platform set. Returns ("", "") when platformIDs is empty.
//
// For queries that already carry a `where ... = "..."` clause (exact-name
// lookups), the caller appends whereSuffix — " & platforms = (6,14,3)" — to
// the existing where (Apicalypse AND-joins with &).
//
// For `search "..."; fields ...` queries (which take an optional standalone
// `where` placed after fields), the caller appends searchTail — a fully-formed
// ` where platforms = (6,14,3);` statement.
func buildPlatformsClause(platformIDs []int) (whereSuffix, searchTail string) {
	if len(platformIDs) == 0 {
		return "", ""
	}
	parts := make([]string, len(platformIDs))
	for i, id := range platformIDs {
		parts[i] = strconv.Itoa(id)
	}
	csv := strings.Join(parts, ",")
	whereSuffix = " & platforms = (" + csv + ")"
	searchTail = " where platforms = (" + csv + ");"
	return
}

// sortByScore sorts candidates by score descending (insertion sort — lists are small).
func sortByScore(items []scoredCandidate) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].score > items[j-1].score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
