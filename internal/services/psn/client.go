package psn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	psnsdk "github.com/sizovilya/go-psn-api"
	"golang.org/x/time/rate"
)

// PSNAccountInfo is the psn-local type — does NOT import the api package.
type PSNAccountInfo struct {
	OnlineID  string
	AccountID string
	Region    string
}

// ErrInvalidNPSSOToken is returned when authentication with the NPSSO token fails.
var ErrInvalidNPSSOToken = errors.New("invalid npsso token")

// ErrPSNGraphQLSchemaChanged is returned when the GraphQL purchases endpoint
// response is missing data.purchasedTitlesRetrieve, indicating the persisted
// query hash is no longer valid and requires a code update.
var ErrPSNGraphQLSchemaChanged = errors.New("psn graphql schema changed")

// Client wraps the go-psn-api library.
type Client struct {
	httpClient      *http.Client
	gamelistURL     string
	graphqlURL      string
	graphqlPageSize int
	limiter         *rate.Limiter
	// authFn overrides psnsdk authentication; used in tests only.
	authFn func(ctx context.Context, npssoToken string) (string, error)
}

// NewClient creates a new PSN client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient:      http.DefaultClient,
		gamelistURL:     "https://m.np.playstation.com",
		graphqlURL:      "https://web.np.playstation.com",
		graphqlPageSize: 200,
		// 5 req/sec, matching internal/services/steam/client.go.
		// docs/sync.md § PSN requires a conservative request delay
		// between pages; PSN has no published rate ceiling.
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
	}
}

// GetAccountInfo authenticates with PSN using the given NPSSO token and returns
// account information for the authenticated user.
// Returns ErrInvalidNPSSOToken if authentication fails.
func (c *Client) GetAccountInfo(ctx context.Context, npssoToken string) (*PSNAccountInfo, error) {
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{
		Lang:   "en",
		Region: "us",
		Npsso:  npssoToken,
	})
	if err != nil {
		return nil, fmt.Errorf("psn: failed to create client: %w", err)
	}

	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		// Auth failure indicates invalid/expired NPSSO token.
		if strings.Contains(err.Error(), "authentication failed") ||
			strings.Contains(err.Error(), "expected redirect") ||
			strings.Contains(err.Error(), "npsso") {
			return nil, ErrInvalidNPSSOToken
		}
		return nil, ErrInvalidNPSSOToken
	}

	// Fetch the authenticated user's own profile using the "me" alias supported
	// by Sony's profile API.
	accessToken, _ := psnClient.AccessToken()
	if accessToken == "" {
		return nil, fmt.Errorf("psn: access token unavailable after authentication")
	}
	profile, err := fetchMyProfile(ctx, accessToken)
	if err != nil {
		// If we can't fetch the profile, still return what we have from auth.
		// This can happen if the profile API changes; return a partial result.
		return &PSNAccountInfo{
			Region: psnClient.Region(),
		}, nil
	}

	return &PSNAccountInfo{
		OnlineID:  profile.OnlineID,
		AccountID: profile.NpID,
		Region:    psnClient.Region(),
	}, nil
}

// profileSelf is the shape of a Sony "me" profile response.
type profileSelf struct {
	Profile struct {
		OnlineID string `json:"onlineId"`
		NpID     string `json:"npId"`
	} `json:"profile"`
}

// fetchMyProfile calls Sony's profile API with the "me" path alias.
func fetchMyProfile(ctx context.Context, accessToken string) (*struct {
	OnlineID string
	NpID     string
}, error) {
	const meURL = "https://us-prof.np.community.playstation.net/userProfile/v1/users/me/profile2?fields=onlineId,npId"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL, nil)
	if err != nil {
		return nil, fmt.Errorf("psn: failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("psn: profile request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("psn: profile HTTP %d", resp.StatusCode)
	}

	var body profileSelf
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("psn: profile decode error: %w", err)
	}

	return &struct {
		OnlineID string
		NpID     string
	}{
		OnlineID: body.Profile.OnlineID,
		NpID:     body.Profile.NpID,
	}, nil
}

type playHistoryResponse struct {
	Titles []struct {
		TitleID      string `json:"titleId"`
		Name         string `json:"name"`
		Category     string `json:"category"`
		Service      string `json:"service"`
		PlayDuration string `json:"playDuration"`
	} `json:"titles"`
	NextOffset     int `json:"nextOffset"`
	TotalItemCount int `json:"totalItemCount"`
}

func (c *Client) fetchPlayHistory(ctx context.Context, accessToken string) (map[string]ExternalGameEntry, error) {
	const limit = 200
	result := make(map[string]ExternalGameEntry)

	for offset := 0; ; offset += limit {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("psn: rate limiter wait: %w", err)
		}
		u := fmt.Sprintf(
			"%s/api/gamelist/v2/users/me/titles?categories=ps4_game,ps5_native_game&limit=%d&offset=%d",
			c.gamelistURL, limit, offset,
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("psn: gamelist request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("psn: gamelist fetch: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("psn: gamelist HTTP %d", resp.StatusCode)
		}

		var body playHistoryResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("psn: gamelist decode: %w", err)
		}

		for _, t := range body.Titles {
			var rawPlatform string
			switch t.Category {
			case "ps4_game":
				rawPlatform = "playstation-4"
			case "ps5_native_game":
				rawPlatform = "playstation-5"
			default:
				continue
			}

			ownership := "owned"
			isSub := false
			if strings.HasPrefix(t.Service, "ps_plus") {
				ownership = "subscription"
				isSub = true
			}

			result[t.TitleID] = ExternalGameEntry{
				ExternalID:      t.TitleID,
				Title:           t.Name,
				Platforms:       []string{rawPlatform},
				PlaytimeHours:   parseDurationHours(t.PlayDuration),
				OwnershipStatus: ownership,
				IsSubscription:  isSub,
			}
		}

		slog.Debug("psn: play history page fetched",
			"offset", offset, "page_count", len(body.Titles), "total", body.TotalItemCount, "running_total", len(result))

		if offset+limit >= body.TotalItemCount {
			break
		}
	}

	return result, nil
}

type purchasedGamesResponse struct {
	Data struct {
		PurchasedTitlesRetrieve *struct {
			Games []struct {
				TitleID             string `json:"titleId"`
				Name                string `json:"name"`
				Platform            string `json:"platform"`
				SubscriptionService string `json:"subscriptionService"`
			} `json:"games"`
		} `json:"purchasedTitlesRetrieve"`
	} `json:"data"`
}

const graphqlHash = "827a423f6a8ddca4107ac01395af2ec0eafd8396fc7fa204aaf9b7ed2eefa168"

func (c *Client) fetchPurchasedGames(ctx context.Context, accessToken string) (map[string]ExternalGameEntry, error) {
	size := c.graphqlPageSize
	result := make(map[string]ExternalGameEntry)

	for start := 0; ; start += size {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("psn: rate limiter wait: %w", err)
		}
		variables := fmt.Sprintf(`{"platform":["ps4","ps5"],"size":%d,"start":%d,"sortBy":"ACTIVE_DATE","sortDirection":"desc"}`, size, start)
		extensions := fmt.Sprintf(`{"persistedQuery":{"version":1,"sha256Hash":"%s"}}`, graphqlHash)

		u := fmt.Sprintf(
			"%s/api/graphql/v1/op?operationName=getPurchasedGameList&variables=%s&extensions=%s",
			c.graphqlURL,
			url.QueryEscape(variables),
			url.QueryEscape(extensions),
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("psn: graphql request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Apollo-Require-Preflight", "true")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("psn: graphql fetch: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body) //nolint:errcheck // body read only to enrich the error log line
			slog.Error("psn: graphql non-200", "status", resp.StatusCode, "body", string(body))
			return nil, fmt.Errorf("psn: graphql HTTP %d", resp.StatusCode)
		}

		var body purchasedGamesResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("psn: graphql decode: %w", err)
		}

		if body.Data.PurchasedTitlesRetrieve == nil {
			return nil, ErrPSNGraphQLSchemaChanged
		}

		games := body.Data.PurchasedTitlesRetrieve.Games
		for _, g := range games {
			var rawPlatform string
			switch g.Platform {
			case "PS4":
				rawPlatform = "playstation-4"
			case "PS5":
				rawPlatform = "playstation-5"
			default:
				continue
			}

			isSub := g.SubscriptionService == "PS_PLUS"
			ownership := "owned"
			if isSub {
				ownership = "subscription"
			}

			result[g.TitleID] = ExternalGameEntry{
				ExternalID:      g.TitleID,
				Title:           g.Name,
				Platforms:       []string{rawPlatform},
				PlaytimeHours:   0,
				OwnershipStatus: ownership,
				IsSubscription:  isSub,
			}
		}

		slog.Debug("psn: purchased games page fetched",
			"start", start, "page_count", len(games), "running_total", len(result))

		if len(games) < size {
			break
		}
	}

	return result, nil
}

// ExternalGameEntry is a normalised game entry from PSN.
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	Platforms       []string // single element per entry; PSN creates one ExternalGame per title ID
	PlaytimeHours   int
	OwnershipStatus string
	IsSubscription  bool
}

func mergePlayedPurchased(played, purchased map[string]ExternalGameEntry) []ExternalGameEntry {
	merged := make(map[string]ExternalGameEntry, len(played)+len(purchased))
	maps.Copy(merged, played)
	for id, e := range purchased {
		if existing, ok := merged[id]; ok {
			if e.IsSubscription {
				existing.IsSubscription = true
				existing.OwnershipStatus = "subscription"
			}
			merged[id] = existing
		} else {
			merged[id] = e
		}
	}
	all := make([]ExternalGameEntry, 0, len(merged))
	for _, e := range merged {
		all = append(all, e)
	}
	return all
}

// GetLibrary fetches the user's PSN game library by merging play history
// (gamelist/v2) and purchased games (GraphQL) into a unified set.
// onBatch is called for each page of batchSize entries and may return an error to abort.
func (c *Client) GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]ExternalGameEntry) error) error {
	// ── Auth ─────────────────────────────────────────────────────────────
	var accessToken string
	if c.authFn != nil {
		var err error
		accessToken, err = c.authFn(ctx, npssoToken)
		if err != nil {
			return ErrInvalidNPSSOToken
		}
	} else {
		psnClient, err := psnsdk.NewClient(&psnsdk.Options{
			Lang:   "en",
			Region: "us",
			Npsso:  npssoToken,
		})
		if err != nil {
			return fmt.Errorf("psn: failed to create client: %w", err)
		}
		if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
			slog.Error("psn: auth failed", "err", err)
			return ErrInvalidNPSSOToken
		}
		accessToken, _ = psnClient.AccessToken()
		if accessToken == "" {
			return fmt.Errorf("psn: access token unavailable after authentication")
		}
	}
	slog.Info("psn: auth succeeded")

	// ── Fetch play history ────────────────────────────────────────────────
	played, err := c.fetchPlayHistory(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("psn: play history: %w", err)
	}
	slog.Info("psn: play history fetched", "count", len(played))

	// ── Fetch purchased games ─────────────────────────────────────────────
	purchased, err := c.fetchPurchasedGames(ctx, accessToken)
	if err != nil {
		return err // preserve ErrPSNGraphQLSchemaChanged unwrapped
	}
	slog.Info("psn: purchased games fetched", "count", len(purchased))

	// ── Merge ─────────────────────────────────────────────────────────────
	all := mergePlayedPurchased(played, purchased)
	slog.Info("psn: library fetch complete", "total_titles", len(all))

	// ── Dispatch in batches ───────────────────────────────────────────────
	for i := 0; i < len(all); i += batchSize {
		end := min(i+batchSize, len(all))
		if err := onBatch(all[i:end]); err != nil {
			return err
		}
	}
	return nil
}
