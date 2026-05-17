# PSN Library API v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the PSN trophy-title proxy in `GetLibrary` with two purpose-built Sony endpoints (gamelist/v2 play history and GraphQL purchased games), merged into a single library result, plus propagate playtime to `user_games.playtime_hours`.

**Architecture:** `fetchPlayHistory` and `fetchPurchasedGames` are unexported methods on `*psn.Client` that call the real Sony endpoints. `GetLibrary` authenticates via the existing psnsdk flow, calls both fetchers, merges (play history first, purchased games upsert), then calls `onBatch` in slices. No changes to `PSNLibraryAdapter` interface or worker dispatch logic beyond two additions: `playtime_hours` in `source_metadata` and a write to `user_games.playtime_hours` in `ProcessSyncItemWorker`.

**Tech Stack:** Go stdlib `net/http`, `encoding/json`, `regexp`, `net/url`; `go-psn-api` SDK (auth only); `httptest` for unit tests; existing `testcontainers` integration tests for worker layer.

---

## File Map

**Create:**
- `internal/services/psn/duration.go` — `parseDurationHours(s string) int` for ISO 8601 durations
- `internal/services/psn/duration_test.go` — unit tests for duration parser
- `internal/services/psn/export_test.go` — test-only setters for injecting HTTP client and base URLs
- `internal/services/psn/library_test.go` — unit tests for `fetchPlayHistory`, `fetchPurchasedGames`, and `GetLibrary` merge logic via `httptest`

**Modify:**
- `internal/services/psn/client.go` — add configurable fields to `Client`; add `ErrPSNGraphQLSchemaChanged`; add `fetchPlayHistory`, `fetchPurchasedGames`, `mergePlayedPurchased`; replace `GetLibrary` body
- `internal/worker/tasks/sync.go` — add `playtime_hours` to `source_metadata` in Steam and PSN dispatch; add `PlaytimeHours int` to meta parse struct; write `playtime_hours` to `user_games` in `ProcessSyncItemWorker`
- `internal/worker/tasks/sync_test.go` — add `TestDispatchSync_PSNGraphQLSchemaChanged_PreservesToken` and `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame`

---

## Task 1: ISO 8601 duration parser

**Files:**
- Create: `internal/services/psn/duration.go`
- Create: `internal/services/psn/duration_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/services/psn/duration_test.go`:

```go
package psn

import "testing"

func TestParseDurationHours(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"PT0S", 0},
		{"PT340H46M13S", 340},
		{"PT1H", 1},
		{"PT30M", 0},
		{"PT2H0M0S", 2},
		{"PT99H59M59S", 99},
		{"", 0},
		{"invalid", 0},
		{"P1DT2H", 0}, // unsupported format — no days component, returns 0
	}
	for _, tc := range cases {
		got := parseDurationHours(tc.input)
		if got != tc.want {
			t.Errorf("parseDurationHours(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/services/psn/... -run TestParseDurationHours -v
```

Expected: compile error — `parseDurationHours` undefined.

- [ ] **Step 3: Implement the parser**

Create `internal/services/psn/duration.go`:

```go
package psn

import (
	"regexp"
	"strconv"
)

// durationRE matches ISO 8601 durations of the form PTxHxMxS.
// Days and larger units are not produced by the Sony API and are not supported.
var durationRE = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:\d+S)?$`)

// parseDurationHours extracts the hours component from an ISO 8601 duration string
// such as "PT340H46M13S". Truncates; does not round. Returns 0 for unrecognised input.
func parseDurationHours(s string) int {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	return h
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/services/psn/... -run TestParseDurationHours -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/psn/duration.go internal/services/psn/duration_test.go
git commit -m "feat(psn): add ISO 8601 duration hour parser"
```

---

## Task 2: Make Client testable — inject HTTP client and base URLs

**Files:**
- Modify: `internal/services/psn/client.go`
- Create: `internal/services/psn/export_test.go`

The new `fetchPlayHistory` and `fetchPurchasedGames` methods need configurable base URLs and an injectable `*http.Client` so `httptest.NewServer` can intercept calls during testing. An `authFn` field allows tests to bypass the psnsdk network call.

- [ ] **Step 1: Update `Client` struct and `NewClient` in `client.go`**

Replace the existing:
```go
// Client wraps the go-psn-api library.
type Client struct{}

// NewClient creates a new PSN client.
func NewClient() *Client { return &Client{} }
```

With:
```go
// Client wraps the go-psn-api library.
type Client struct {
	httpClient  *http.Client
	gamelistURL string // base URL for m.np.playstation.com
	graphqlURL  string // base URL for web.np.playstation.com
	// authFn overrides psnsdk authentication; used in tests only.
	authFn func(ctx context.Context, npssoToken string) (string, error)
}

// NewClient creates a new PSN client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient:  http.DefaultClient,
		gamelistURL: "https://m.np.playstation.com",
		graphqlURL:  "https://web.np.playstation.com",
	}
}
```

- [ ] **Step 2: Create `export_test.go` with test-only setters**

Create `internal/services/psn/export_test.go`:

```go
package psn

import "net/http"

// Test-only setters — compiled only during go test.

func (c *Client) SetHTTPClient(h *http.Client)        { c.httpClient = h }
func (c *Client) SetGamelistURL(url string)            { c.gamelistURL = url }
func (c *Client) SetGraphQLURL(url string)             { c.graphqlURL = url }
func (c *Client) SetAuthFn(fn func(ctx interface{}, npssoToken string) (string, error)) {
	// authFn uses context.Context — cast at call site; keep import-free here.
}
```

Wait — `authFn` needs `context.Context`. Export it properly:

```go
package psn

import (
	"context"
	"net/http"
)

func (c *Client) SetHTTPClient(h *http.Client)                                       { c.httpClient = h }
func (c *Client) SetGamelistURL(url string)                                          { c.gamelistURL = url }
func (c *Client) SetGraphQLURL(url string)                                           { c.graphqlURL = url }
func (c *Client) SetAuthFn(fn func(ctx context.Context, token string) (string, error)) { c.authFn = fn }
```

- [ ] **Step 3: Verify the package still compiles**

```bash
go build ./internal/services/psn/...
```

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/services/psn/client.go internal/services/psn/export_test.go
git commit -m "feat(psn): make Client testable with injectable HTTP client and base URLs"
```

---

## Task 3: Implement `fetchPlayHistory` + tests

**Files:**
- Modify: `internal/services/psn/client.go`
- Create: `internal/services/psn/library_test.go`

- [ ] **Step 1: Write failing tests for `fetchPlayHistory`**

Create `internal/services/psn/library_test.go`:

```go
package psn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// fetchPlayHistory
// ---------------------------------------------------------------------------

func TestFetchPlayHistory_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"titles": []map[string]any{
				{
					"titleId":      "PPSA07950_00",
					"name":         "Call of Duty®",
					"category":     "ps5_native_game",
					"service":      "none(purchased)",
					"playDuration": "PT340H46M13S",
				},
				{
					"titleId":      "CUSA10410_00",
					"name":         "CODE VEIN",
					"category":     "ps4_game",
					"service":      "ps_plus_extra",
					"playDuration": "PT12H",
				},
				{
					"titleId":      "IGNORED_00",
					"name":         "PC Port",
					"category":     "pspc_game",
					"service":      "none(purchased)",
					"playDuration": "PT1H",
				},
			},
			"nextOffset":     200,
			"totalItemCount": 2, // only 2 valid entries; loop should stop
		})
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)

	result, err := c.fetchPlayHistory(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries (pspc_game excluded), got %d", len(result))
	}

	ps5 := result["PPSA07950_00"]
	if ps5.RawPlatform != "playstation-5" {
		t.Errorf("expected playstation-5, got %q", ps5.RawPlatform)
	}
	if ps5.PlaytimeHours != 340 {
		t.Errorf("expected 340 hours, got %d", ps5.PlaytimeHours)
	}
	if ps5.OwnershipStatus != "owned" || ps5.IsSubscription {
		t.Errorf("unexpected ownership for purchased PS5 game: status=%q sub=%v", ps5.OwnershipStatus, ps5.IsSubscription)
	}

	ps4 := result["CUSA10410_00"]
	if ps4.RawPlatform != "playstation-4" {
		t.Errorf("expected playstation-4, got %q", ps4.RawPlatform)
	}
	if ps4.OwnershipStatus != "subscription" || !ps4.IsSubscription {
		t.Errorf("unexpected ownership for PS Plus PS4 game: status=%q sub=%v", ps4.OwnershipStatus, ps4.IsSubscription)
	}
}

func TestFetchPlayHistory_Pagination(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		offset := r.URL.Query().Get("offset")
		switch offset {
		case "0", "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"titles": []map[string]any{
					{"titleId": "GAME1", "name": "Game One", "category": "ps4_game", "service": "none(purchased)", "playDuration": "PT1H"},
				},
				"nextOffset":     200,
				"totalItemCount": 2,
			})
		case "200":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"titles": []map[string]any{
					{"titleId": "GAME2", "name": "Game Two", "category": "ps5_native_game", "service": "none(purchased)", "playDuration": "PT2H"},
				},
				"nextOffset":     400,
				"totalItemCount": 2,
			})
		default:
			// offset >= totalItemCount — should never be reached
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)

	result, err := c.fetchPlayHistory(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls (pagination), got %d", calls)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries from 2 pages, got %d", len(result))
	}
}

func TestFetchPlayHistory_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)

	_, err := c.fetchPlayHistory(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error for 503 response, got nil")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/services/psn/... -run TestFetchPlayHistory -v
```

Expected: compile error — `fetchPlayHistory` undefined.

- [ ] **Step 3: Implement `fetchPlayHistory` in `client.go`**

Add the following after the `NewClient` function:

```go
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

func (c *Client) fetchPlayHistory(ctx context.Context, accessToken string) (map[string]ExternalLibraryEntry, error) {
	const limit = 200
	result := make(map[string]ExternalLibraryEntry)

	for offset := 0; ; offset += limit {
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
				continue // skip pspc_game and unknown categories
			}

			ownership := "owned"
			isSub := false
			if strings.HasPrefix(t.Service, "ps_plus") {
				ownership = "subscription"
				isSub = true
			}

			result[t.TitleID] = ExternalLibraryEntry{
				ExternalID:      t.TitleID,
				Title:           t.Name,
				RawPlatform:     rawPlatform,
				PlaytimeHours:   parseDurationHours(t.PlayDuration),
				OwnershipStatus: ownership,
				IsSubscription:  isSub,
			}
		}

		if offset+limit >= body.TotalItemCount {
			break
		}
	}

	return result, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/services/psn/... -run TestFetchPlayHistory -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/psn/client.go internal/services/psn/library_test.go
git commit -m "feat(psn): implement fetchPlayHistory from gamelist/v2 endpoint"
```

---

## Task 4: Add `ErrPSNGraphQLSchemaChanged` and implement `fetchPurchasedGames`

**Files:**
- Modify: `internal/services/psn/client.go`
- Modify: `internal/services/psn/library_test.go`

- [ ] **Step 1: Add failing tests for `fetchPurchasedGames`**

Append to `internal/services/psn/library_test.go`:

```go
// ---------------------------------------------------------------------------
// fetchPurchasedGames
// ---------------------------------------------------------------------------

func TestFetchPurchasedGames_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"purchasedTitlesRetrieve": map[string]any{
					"games": []map[string]any{
						{"titleId": "CUSA10410_00", "name": "CODE VEIN", "platform": "PS4", "subscriptionService": "PS_PLUS", "isActive": true},
						{"titleId": "PPSA07950_00", "name": "Call of Duty®", "platform": "PS5", "subscriptionService": "NONE", "isActive": true},
						{"titleId": "PC_GAME_00", "name": "PC Port", "platform": "PC", "subscriptionService": "NONE", "isActive": true},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	result, err := c.fetchPurchasedGames(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries (PC excluded), got %d", len(result))
	}

	ps4 := result["CUSA10410_00"]
	if ps4.RawPlatform != "playstation-4" {
		t.Errorf("expected playstation-4, got %q", ps4.RawPlatform)
	}
	if !ps4.IsSubscription {
		t.Error("expected IsSubscription=true for PS_PLUS game")
	}
	if ps4.PlaytimeHours != 0 {
		t.Error("expected playtime=0 from purchased endpoint")
	}

	ps5 := result["PPSA07950_00"]
	if ps5.RawPlatform != "playstation-5" {
		t.Errorf("expected playstation-5, got %q", ps5.RawPlatform)
	}
	if ps5.IsSubscription {
		t.Error("expected IsSubscription=false for non-PS_PLUS game")
	}
}

func TestFetchPurchasedGames_GraphQLSchemaChanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Missing data.purchasedTitlesRetrieve — hash changed.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{},
		})
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	_, err := c.fetchPurchasedGames(context.Background(), "tok")
	if !errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Errorf("expected ErrPSNGraphQLSchemaChanged, got %v", err)
	}
}

func TestFetchPurchasedGames_Pagination(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// First page: full 200 games (simulate with 1 game but pretend page is full via len check).
		// Second page: fewer than size — pagination stops.
		var games []map[string]any
		if calls == 1 {
			// Return exactly 2 games to simulate a "full" page when size=2 in test.
			games = []map[string]any{
				{"titleId": "GAME1", "name": "G1", "platform": "PS4", "subscriptionService": "NONE"},
				{"titleId": "GAME2", "name": "G2", "platform": "PS5", "subscriptionService": "NONE"},
			}
		} else {
			// Partial page — stop paginating.
			games = []map[string]any{
				{"titleId": "GAME3", "name": "G3", "platform": "PS4", "subscriptionService": "NONE"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"purchasedTitlesRetrieve": map[string]any{"games": games},
			},
		})
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)
	c.SetGraphQLPageSize(2) // test-only setter to override 200

	result, err := c.fetchPurchasedGames(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", calls)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 games across 2 pages, got %d", len(result))
	}
}

func TestFetchPurchasedGames_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	_, err := c.fetchPurchasedGames(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Error("HTTP 500 should not produce ErrPSNGraphQLSchemaChanged")
	}
}
```

Also add `"errors"` to the import block at the top of `library_test.go`.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/services/psn/... -run TestFetchPurchasedGames -v
```

Expected: compile error — `fetchPurchasedGames`, `ErrPSNGraphQLSchemaChanged`, and `SetGraphQLPageSize` undefined.

- [ ] **Step 3: Add `ErrPSNGraphQLSchemaChanged` and `graphqlPageSize` field to `client.go`**

Add the sentinel after `ErrInvalidNPSSOToken`:

```go
// ErrPSNGraphQLSchemaChanged is returned when the GraphQL purchases endpoint
// response is missing data.purchasedTitlesRetrieve, indicating the persisted
// query hash is no longer valid and requires a code update.
var ErrPSNGraphQLSchemaChanged = errors.New("psn graphql schema changed")
```

Add `graphqlPageSize int` to the `Client` struct:

```go
type Client struct {
	httpClient      *http.Client
	gamelistURL     string
	graphqlURL      string
	graphqlPageSize int // default 200; overridable in tests
	authFn          func(ctx context.Context, npssoToken string) (string, error)
}
```

Update `NewClient` to set the page size:

```go
func NewClient() *Client {
	return &Client{
		httpClient:      http.DefaultClient,
		gamelistURL:     "https://m.np.playstation.com",
		graphqlURL:      "https://web.np.playstation.com",
		graphqlPageSize: 200,
	}
}
```

Add `SetGraphQLPageSize` to `export_test.go`:

```go
func (c *Client) SetGraphQLPageSize(n int) { c.graphqlPageSize = n }
```

- [ ] **Step 4: Implement `fetchPurchasedGames` in `client.go`**

Add after `fetchPlayHistory`:

```go
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

func (c *Client) fetchPurchasedGames(ctx context.Context, accessToken string) (map[string]ExternalLibraryEntry, error) {
	size := c.graphqlPageSize
	result := make(map[string]ExternalLibraryEntry)

	for start := 0; ; start += size {
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

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("psn: graphql fetch: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
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

			result[g.TitleID] = ExternalLibraryEntry{
				ExternalID:      g.TitleID,
				Title:           g.Name,
				RawPlatform:     rawPlatform,
				PlaytimeHours:   0,
				OwnershipStatus: ownership,
				IsSubscription:  isSub,
			}
		}

		if len(games) < size {
			break
		}
	}

	return result, nil
}
```

Add `"net/url"` to imports in `client.go`.

- [ ] **Step 5: Run tests to confirm they pass**

```bash
go test ./internal/services/psn/... -run TestFetchPurchasedGames -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/psn/client.go internal/services/psn/export_test.go internal/services/psn/library_test.go
git commit -m "feat(psn): add ErrPSNGraphQLSchemaChanged and fetchPurchasedGames"
```

---

## Task 5: Merge logic and replace `GetLibrary`

**Files:**
- Modify: `internal/services/psn/client.go`
- Modify: `internal/services/psn/library_test.go`

- [ ] **Step 1: Write failing tests for merge logic and the new `GetLibrary`**

Append to `library_test.go`:

```go
// ---------------------------------------------------------------------------
// mergePlayedPurchased
// ---------------------------------------------------------------------------

func TestMergePlayedPurchased_PlayHistoryFirst(t *testing.T) {
	played := map[string]ExternalLibraryEntry{
		"GAME1": {ExternalID: "GAME1", Title: "Game One", RawPlatform: "playstation-4", PlaytimeHours: 42, OwnershipStatus: "owned", IsSubscription: false},
	}
	purchased := map[string]ExternalLibraryEntry{
		"GAME2": {ExternalID: "GAME2", Title: "Game Two", RawPlatform: "playstation-5", PlaytimeHours: 0, OwnershipStatus: "owned", IsSubscription: false},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(result))
	}
}

func TestMergePlayedPurchased_PurchasedUpdatesSub(t *testing.T) {
	// Game in play history as owned, purchased endpoint says PS Plus.
	played := map[string]ExternalLibraryEntry{
		"GAME1": {ExternalID: "GAME1", Title: "G1", RawPlatform: "playstation-4", PlaytimeHours: 10, OwnershipStatus: "owned", IsSubscription: false},
	}
	purchased := map[string]ExternalLibraryEntry{
		"GAME1": {ExternalID: "GAME1", Title: "G1", RawPlatform: "playstation-4", PlaytimeHours: 0, OwnershipStatus: "subscription", IsSubscription: true},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	entry := result[0]
	if !entry.IsSubscription {
		t.Error("expected IsSubscription=true after purchased endpoint marks PS Plus")
	}
	if entry.PlaytimeHours != 10 {
		t.Errorf("expected playtime preserved from play history (10), got %d", entry.PlaytimeHours)
	}
}

func TestMergePlayedPurchased_PurchasedDoesNotDowngradeOwnership(t *testing.T) {
	// Game in play history as owned (disc), purchased says no subscription. Keep owned.
	played := map[string]ExternalLibraryEntry{
		"GAME1": {ExternalID: "GAME1", Title: "G1", RawPlatform: "playstation-4", PlaytimeHours: 5, OwnershipStatus: "owned", IsSubscription: false},
	}
	purchased := map[string]ExternalLibraryEntry{
		"GAME1": {ExternalID: "GAME1", Title: "G1", RawPlatform: "playstation-4", PlaytimeHours: 0, OwnershipStatus: "owned", IsSubscription: false},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].OwnershipStatus != "owned" {
		t.Errorf("expected owned status preserved, got %q", result[0].OwnershipStatus)
	}
}

func TestMergePlayedPurchased_DiscGameNotInPurchased(t *testing.T) {
	// Disc game only in play history (not in purchased list) — must be included.
	played := map[string]ExternalLibraryEntry{
		"DISC1": {ExternalID: "DISC1", Title: "Disc Game", RawPlatform: "playstation-4", PlaytimeHours: 3, OwnershipStatus: "owned"},
	}
	purchased := map[string]ExternalLibraryEntry{}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 1 || result[0].ExternalID != "DISC1" {
		t.Errorf("expected disc game in result, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// GetLibrary end-to-end (auth injected via SetAuthFn)
// ---------------------------------------------------------------------------

func TestGetLibrary_MergesResults(t *testing.T) {
	gamelistSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"titles": []map[string]any{
				{"titleId": "DISC1", "name": "Disc Game", "category": "ps4_game", "service": "none(purchased)", "playDuration": "PT5H"},
			},
			"totalItemCount": 1,
		})
	}))
	defer gamelistSrv.Close()

	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"purchasedTitlesRetrieve": map[string]any{
					"games": []map[string]any{
						{"titleId": "DL1", "name": "Digital Game", "platform": "PS5", "subscriptionService": "NONE"},
					},
				},
			},
		})
	}))
	defer graphqlSrv.Close()

	c := NewClient()
	c.SetHTTPClient(gamelistSrv.Client())
	c.SetGamelistURL(gamelistSrv.URL)
	c.SetGraphQLURL(graphqlSrv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "test-token", nil })

	var batches [][]ExternalLibraryEntry
	err := c.GetLibrary(context.Background(), "fake-npsso", 10, func(batch []ExternalLibraryEntry) error {
		batches = append(batches, batch)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 2 {
		t.Errorf("expected 2 merged entries (disc + digital), got %d", total)
	}
}

func TestGetLibrary_PlayHistoryError_ReturnsError(t *testing.T) {
	gamelistSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer gamelistSrv.Close()

	c := NewClient()
	c.SetHTTPClient(gamelistSrv.Client())
	c.SetGamelistURL(gamelistSrv.URL)
	c.SetGraphQLURL(gamelistSrv.URL) // won't be reached
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "tok", nil })

	err := c.GetLibrary(context.Background(), "npsso", 10, func([]ExternalLibraryEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when gamelist endpoint fails")
	}
}

func TestGetLibrary_GraphQLSchemaChanged_ReturnsSentinel(t *testing.T) {
	gamelistSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"titles": []any{}, "totalItemCount": 0})
	}))
	defer gamelistSrv.Close()

	graphqlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Missing purchasedTitlesRetrieve
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer graphqlSrv.Close()

	c := NewClient()
	c.SetHTTPClient(gamelistSrv.Client())
	c.SetGamelistURL(gamelistSrv.URL)
	c.SetGraphQLURL(graphqlSrv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "tok", nil })

	err := c.GetLibrary(context.Background(), "npsso", 10, func([]ExternalLibraryEntry) error { return nil })
	if !errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Errorf("expected ErrPSNGraphQLSchemaChanged, got %v", err)
	}
}
```

Note: the two `httptest` servers in `TestGetLibrary_MergesResults` use different clients. Pass the gamelist server's HTTP client, which is `httptest.NewServer`'s client and routes to that server's `Listener`. For the graphql server call to work, we also need that client. Since they have different listeners, inject a transport that routes both hosts. The simplest solution: use a single handler that dispatches on URL path, or make `httpClient` per-fetcher.

**Revised test approach for `TestGetLibrary_MergesResults`:** Use a single `httptest.NewServer` with path-based dispatch:

```go
func TestGetLibrary_MergesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/gamelist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"titles": []map[string]any{
					{"titleId": "DISC1", "name": "Disc Game", "category": "ps4_game", "service": "none(purchased)", "playDuration": "PT5H"},
				},
				"totalItemCount": 1,
			})
		case strings.HasPrefix(r.URL.Path, "/api/graphql"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"purchasedTitlesRetrieve": map[string]any{
						"games": []map[string]any{
							{"titleId": "DL1", "name": "Digital Game", "platform": "PS5", "subscriptionService": "NONE"},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
	c.SetGraphQLURL(srv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "test-token", nil })

	var total int
	err := c.GetLibrary(context.Background(), "fake-npsso", 10, func(batch []ExternalLibraryEntry) error {
		total += len(batch)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 merged entries (disc + digital), got %d", total)
	}
}
```

Apply the same single-server pattern to `TestGetLibrary_GraphQLSchemaChanged_ReturnsSentinel`. Add `"strings"` to the `library_test.go` import block.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/services/psn/... -run "TestMerge|TestGetLibrary" -v
```

Expected: compile errors — `mergePlayedPurchased` undefined, new `GetLibrary` signature mismatch with `authFn`.

- [ ] **Step 3: Implement `mergePlayedPurchased` and replace `GetLibrary` in `client.go`**

Add `mergePlayedPurchased` before `GetLibrary`:

```go
func mergePlayedPurchased(played, purchased map[string]ExternalLibraryEntry) []ExternalLibraryEntry {
	merged := make(map[string]ExternalLibraryEntry, len(played)+len(purchased))
	for id, e := range played {
		merged[id] = e
	}
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

	all := make([]ExternalLibraryEntry, 0, len(merged))
	for _, e := range merged {
		all = append(all, e)
	}
	return all
}
```

Replace the existing `GetLibrary` implementation with:

```go
func (c *Client) GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]ExternalLibraryEntry) error) error {
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
		return err // preserve ErrPSNGraphQLSchemaChanged
	}
	slog.Info("psn: purchased games fetched", "count", len(purchased))

	// ── Merge ─────────────────────────────────────────────────────────────
	all := mergePlayedPurchased(played, purchased)
	slog.Info("psn: library fetch complete", "total_titles", len(all))

	// ── Dispatch in batches ───────────────────────────────────────────────
	for i := 0; i < len(all); i += batchSize {
		end := i + batchSize
		if end > len(all) {
			end = len(all)
		}
		if err := onBatch(all[i:end]); err != nil {
			return err
		}
	}

	return nil
}
```

Remove the now-unused `platformMap` and `psnsdk.GetTrophyTitles` imports/usages.

- [ ] **Step 4: Run all PSN tests**

```bash
go test ./internal/services/psn/... -v
```

Expected: all PASS.

- [ ] **Step 5: Run the full test suite to check for regressions**

```bash
go test -timeout 600s ./...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/psn/client.go internal/services/psn/library_test.go
git commit -m "feat(psn): replace GetLibrary with gamelist/v2 + GraphQL merge"
```

---

## Task 6: Add `playtime_hours` to `source_metadata` in dispatch

**Files:**
- Modify: `internal/worker/tasks/sync.go`

Both Steam and PSN dispatch paths currently build `map[string]string` metadata. Change both to include `playtime_hours` as a JSON integer so `ProcessSyncItemWorker` can read it.

- [ ] **Step 1: Update `source_metadata` in the Steam dispatch path**

In `DispatchSyncWorker.Work`, find the Steam dispatch block. Change:

```go
meta := map[string]string{
    "external_game_id": eg.ID,
    "raw_platform":     rawPlatformByExtID[eg.ExternalID],
}
metaJSON, _ := json.Marshal(meta)
```

To:

```go
metaJSON, _ := json.Marshal(map[string]any{
    "external_game_id": eg.ID,
    "raw_platform":     rawPlatformByExtID[eg.ExternalID],
    "playtime_hours":   eg.PlaytimeHours,
})
```

- [ ] **Step 2: Update `source_metadata` in the PSN dispatch path**

In the PSN batch callback (`func(batch []psnsvc.ExternalLibraryEntry) error`), find:

```go
meta := map[string]string{
    "external_game_id": eg.ID,
    "raw_platform":     rawPlatformByExtID[eg.ExternalID],
}
metaJSON, _ := json.Marshal(meta)
```

Change to:

```go
metaJSON, _ := json.Marshal(map[string]any{
    "external_game_id": eg.ID,
    "raw_platform":     rawPlatformByExtID[eg.ExternalID],
    "playtime_hours":   eg.PlaytimeHours,
})
```

- [ ] **Step 3: Update meta parse struct in `ProcessSyncItemWorker`**

In `ProcessSyncItemWorker.Work`, find:

```go
var meta struct {
    ExternalGameID string `json:"external_game_id"`
    RawPlatform    string `json:"raw_platform"`
}
```

Change to:

```go
var meta struct {
    ExternalGameID string `json:"external_game_id"`
    RawPlatform    string `json:"raw_platform"`
    PlaytimeHours  int    `json:"playtime_hours"`
}
```

- [ ] **Step 4: Build to confirm no compile errors**

```bash
go build ./internal/worker/...
```

Expected: success.

- [ ] **Step 5: Run existing sync tests to confirm no regressions**

```bash
go test ./internal/worker/... -v -timeout 600s
```

Expected: all existing tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "feat(sync): add playtime_hours to source_metadata for Steam and PSN dispatch"
```

---

## Task 7: Write `user_games.playtime_hours` in `ProcessSyncItemWorker`

**Files:**
- Modify: `internal/worker/tasks/sync.go`

`user_games.playtime_hours INTEGER NOT NULL DEFAULT 0` exists (confirmed in `20260503000001_initial.up.sql`). No migration needed. Currently `ProcessSyncItemWorker` inserts `user_games` without `playtime_hours` and never updates it. This task adds the write.

- [ ] **Step 1: Update the user_game INSERT to include `playtime_hours`**

In `ProcessSyncItemWorker.Work`, step 8 ("Find or create user_game"), find:

```go
_, _ = w.DB.NewRaw(
    `INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?)
     ON CONFLICT (user_id, game_id) DO NOTHING`,
    ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
).Exec(ctx)
```

Change to:

```go
_, _ = w.DB.NewRaw(
    `INSERT INTO user_games (id, user_id, game_id, playtime_hours, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?)
     ON CONFLICT (user_id, game_id) DO NOTHING`,
    ugID, item.UserID, *eg.ResolvedIGDBID, meta.PlaytimeHours, now, now,
).Exec(ctx)
```

- [ ] **Step 2: Update `user_games.playtime_hours` when the row already exists**

Immediately after the existing `SELECT id FROM user_games` succeeds (the `else` branch where `err == nil`), add a playtime update. Find the code path where `ugID` is fetched successfully (no error). In the current code there's no else block — the `err != nil` block handles the miss; after that block, execution falls through with `ugID` set. Add an update after the `if err != nil { ... }` block:

```go
} else if meta.PlaytimeHours > 0 {
    _, _ = w.DB.NewRaw(
        `UPDATE user_games SET playtime_hours = ?, updated_at = now() WHERE id = ? AND playtime_hours < ?`,
        meta.PlaytimeHours, ugID, meta.PlaytimeHours,
    ).Exec(ctx)
}
```

The `AND playtime_hours < ?` guard prevents resetting a higher value if the user manually set a larger number.

- [ ] **Step 3: Build to confirm no compile errors**

```bash
go build ./internal/worker/...
```

Expected: success.

- [ ] **Step 4: Run existing sync tests**

```bash
go test ./internal/worker/... -timeout 600s -v -run TestProcessSyncItem
```

Expected: all PASS (the existing test `TestProcessSyncItem_WithResolvedIGDBID_Completed` will now also set `playtime_hours=0` on the user_game, which is fine).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "feat(sync): write playtime_hours to user_games in ProcessSyncItemWorker"
```

---

## Task 8: New worker integration tests

**Files:**
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Add `TestDispatchSync_PSNGraphQLSchemaChanged_PreservesToken`**

Append to `sync_test.go`:

```go
func TestDispatchSync_PSNGraphQLSchemaChanged_PreservesToken(t *testing.T) {
	// ErrPSNGraphQLSchemaChanged is a non-auth error: job fails, token stays verified.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakePSNAdapter{err: psnsvc.ErrPSNGraphQLSchemaChanged}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed, got %q", status)
	}

	// Token must NOT be marked expired — this is a schema change, not an auth error.
	var rawCreds string
	_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
	var parsedCreds struct {
		IsVerified bool `json:"is_verified"`
	}
	_ = json.Unmarshal([]byte(rawCreds), &parsedCreds)
	if !parsedCreds.IsVerified {
		t.Error("expected is_verified=true after schema-changed error (token not expired)")
	}
}
```

- [ ] **Step 2: Add `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame`**

Append to `sync_test.go`:

```go
func TestProcessSyncItem_PlaytimeHoursWrittenToUserGame(t *testing.T) {
	// Verifies that user_games.playtime_hours is written from source_metadata.playtime_hours.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'low', 1)`,
		jobID, userID,
	)

	const igdbID = int32(9999)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)

	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'PPSA09999_00', 'Test Game', false, true, false, 47, ?)`,
		egID, userID, igdbIDVal,
	)

	// source_metadata includes playtime_hours=47, matching what was stored in external_games.
	metaJSON, _ := json.Marshal(map[string]any{
		"external_game_id": egID,
		"raw_platform":     "playstation-4",
		"playtime_hours":   47,
	})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'PPSA09999_00', 'Test Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var itemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus)
	if itemStatus != "completed" {
		t.Errorf("expected item completed, got %q", itemStatus)
	}

	var playtime int
	_ = testDB.NewRaw(
		`SELECT playtime_hours FROM user_games WHERE user_id = ? AND game_id = ?`,
		userID, igdbID,
	).Scan(ctx, &playtime)
	if playtime != 47 {
		t.Errorf("expected user_games.playtime_hours=47, got %d", playtime)
	}
}
```

- [ ] **Step 3: Run the new tests to confirm they fail for the right reason**

```bash
go test ./internal/worker/... -timeout 600s -run "TestDispatchSync_PSNGraphQLSchemaChanged|TestProcessSyncItem_PlaytimeHours" -v
```

Expected: `TestDispatchSync_PSNGraphQLSchemaChanged_PreservesToken` PASS (the existing worker already handles non-auth errors without touching token); `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame` PASS (Task 7 already implemented the write).

- [ ] **Step 4: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/sync_test.go
git commit -m "test(sync): add PSN GraphQL schema-changed and playtime propagation tests"
```

---

## Self-Review

### Spec coverage check

| Spec requirement | Task |
|---|---|
| Play history endpoint (`gamelist/v2`) with pagination | Task 3 |
| Purchased games endpoint (GraphQL) with pagination | Task 4 |
| Platform mapping (ps4_game → playstation-4, ps5_native_game → playstation-5) | Task 3 |
| Platform mapping (PS4 → playstation-4, PS5 → playstation-5) | Task 4 |
| Ownership mapping (none(purchased) → owned, ps_plus* → subscription) | Task 3 |
| ISO 8601 duration → playtime hours (truncate) | Task 1 |
| Merge: play history first, purchased games upsert | Task 5 |
| Disc games (play history only) included | Task 5 merge tests |
| `ErrPSNGraphQLSchemaChanged` when `data.purchasedTitlesRetrieve` missing | Task 4 |
| `ErrPSNGraphQLSchemaChanged` falls to generic error path in worker | Task 8 |
| Token NOT marked expired for `ErrPSNGraphQLSchemaChanged` | Task 8 |
| Both endpoints required; either failure aborts | Task 5 (`GetLibrary` returns error immediately) |
| `playtime_hours` in `source_metadata` | Task 6 |
| `user_games.playtime_hours` written by `ProcessSyncItemWorker` | Task 7 |
| No migration needed | ✓ column exists |
| `PSNLibraryAdapter` interface unchanged | ✓ not touched |
| `ExternalLibraryEntry` unchanged | ✓ not touched |
| Auth via existing `AuthWithNPSSO` | Task 5 (`GetLibrary` keeps auth flow) |
| No PS3 / pspc_game / PlayStation PC | Task 3 (skip unknown categories), Task 4 (skip non-PS4/PS5) |

### Placeholder scan

No TBDs, TODOs, or vague instructions present. All steps include complete code.

### Type consistency

- `parseDurationHours` returns `int` — matches `ExternalLibraryEntry.PlaytimeHours int` ✓
- `fetchPlayHistory` / `fetchPurchasedGames` both return `map[string]ExternalLibraryEntry` — consumed by `mergePlayedPurchased(played, purchased map[string]ExternalLibraryEntry)` ✓
- `mergePlayedPurchased` returns `[]ExternalLibraryEntry` — iterated in `GetLibrary` ✓
- `meta.PlaytimeHours int` parsed from JSON int stored as `map[string]any{"playtime_hours": eg.PlaytimeHours}` — JSON number decodes to `int` correctly ✓
- `ErrPSNGraphQLSchemaChanged` is `var` sentinel — `errors.Is` works ✓
