package psn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
		// If we can't fetch the profile, auth itself still succeeded.
		// This can happen if the profile API changes; return an empty result.
		return &PSNAccountInfo{}, nil
	}

	return &PSNAccountInfo{
		OnlineID:  profile.OnlineID,
		AccountID: profile.NpID,
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

			// Play history contributes playtime only; ownership / subscription
			// classification comes entirely from the owned endpoint (issue #753).
			result[t.TitleID] = ExternalGameEntry{
				ExternalID:    t.TitleID,
				Title:         t.Name,
				Platforms:     []string{rawPlatform},
				PlaytimeHours: parseDurationFractionalHours(t.PlayDuration),
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
	PlaytimeHours   float64
	OwnershipStatus string
	IsSubscription  bool
}

// applyPlaytimeToOwned returns the user's library using the owned (purchased)
// endpoint as the single source of truth. Library membership and ownership /
// subscription classification come entirely from owned. Play history is
// consulted only to enrich playtime for titles already in the owned set; any
// play-history entry whose title is not owned is ignored entirely (no playtime,
// no classification, not added). Owned titles that were never played keep zero
// playtime.
func applyPlaytimeToOwned(owned, played map[string]ExternalGameEntry) []ExternalGameEntry {
	all := make([]ExternalGameEntry, 0, len(owned))
	for id, e := range owned {
		if p, ok := played[id]; ok {
			e.PlaytimeHours = p.PlaytimeHours
		}
		all = append(all, e)
	}
	return all
}

// Authenticate exchanges an NPSSO token for a PSN access token.
func (c *Client) Authenticate(ctx context.Context, npssoToken string) (string, error) {
	if c.authFn != nil {
		return c.authFn(ctx, npssoToken)
	}
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{Lang: "en", Region: "us", Npsso: npssoToken})
	if err != nil {
		return "", fmt.Errorf("psn: failed to create client: %w", err)
	}
	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		return "", ErrInvalidNPSSOToken
	}
	token, _ := psnClient.AccessToken()
	if token == "" {
		return "", fmt.Errorf("psn: access token unavailable after authentication")
	}
	return token, nil
}

type conceptsResponse struct {
	Concepts []struct {
		ID string `json:"id"`
	} `json:"concepts"`
}

// ResolveConceptID resolves a PSN titleId to its store concept ID via the
// catalog API. Returns "" (no error) when the title has no resolvable concept.
//
// NOTE: the exact JSON shape of /catalog/v2/titles/{id}/concepts is unverified;
// confirm against a live response and adjust conceptsResponse + extraction if
// needed. Authenticated with the access token from Authenticate.
func (c *Client) ResolveConceptID(ctx context.Context, accessToken, titleID string) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("psn: rate limiter wait: %w", err)
	}
	u := fmt.Sprintf("%s/api/catalog/v2/titles/%s/concepts", c.gamelistURL, titleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("psn: concepts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("psn: concepts fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("psn: concepts HTTP %d", resp.StatusCode)
	}
	var body conceptsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("psn: concepts decode: %w", err)
	}
	if len(body.Concepts) == 0 || body.Concepts[0].ID == "" {
		return "", nil
	}
	return body.Concepts[0].ID, nil
}

// GetLibrary fetches the user's PSN game library. The owned/purchased endpoint
// (GraphQL) is the single source of truth for library membership and ownership /
// subscription classification; play history (gamelist/v2) is used only to enrich
// playtime for titles already owned. Played-but-not-owned titles are dropped.
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

	// ── Owned set is authoritative; play history only enriches playtime ───
	all := applyPlaytimeToOwned(purchased, played)
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
