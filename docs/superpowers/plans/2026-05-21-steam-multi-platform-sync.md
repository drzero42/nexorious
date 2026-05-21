# Steam Multi-Platform Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit one `external_games` row per platform per Steam game by calling `store.steampowered.com/api/appdetails` per appid, cache results in `external_games`, and add parity fixes for GOG Mac emission and `pc-mac` platform resolution.

**Architecture:** Steam sync calls `GetOwnedGames` for appids/titles, batch-queries existing `external_games` rows as a per-user cache, calls `GetAppDetailsPlatforms` per uncached appid (rate-limited 5 req/s), and fans out one upsert per detected platform. `item_key` for Steam job_items changes to `external_id:raw_platform` matching the GOG pattern. GOG gains a Mac emission branch. `platformresolution` learns `pc-mac → mac`. No schema change required.

**Tech Stack:** Go 1.25, `golang.org/x/time/rate`, `net/http/httptest` (tests), `uptrace/bun` (DB), `riverqueue/river` (job queue)

---

### Task 1: Add `pc-mac` to platform resolution

**Files:**
- Modify: `internal/services/platformresolution/resolution.go`
- Modify: `internal/services/platformresolution/resolution_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/services/platformresolution/resolution_test.go`:

```go
func TestRawPlatformToSlug_PCMac(t *testing.T) {
	slug, ok := platformresolution.RawPlatformToSlug("pc-mac")
	if !ok {
		t.Fatal("expected pc-mac to resolve, got false")
	}
	if slug != "mac" {
		t.Errorf("got %q, want %q", slug, "mac")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/services/platformresolution/... -run TestRawPlatformToSlug_PCMac -v
```
Expected: FAIL — `expected pc-mac to resolve, got false`

- [ ] **Step 3: Add the case to `RawPlatformToSlug`**

In `internal/services/platformresolution/resolution.go`, add before `default`:
```go
case "pc-mac":
	return "mac", true
```

- [ ] **Step 4: Run all platformresolution tests**

```bash
go test ./internal/services/platformresolution/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/services/platformresolution/resolution.go internal/services/platformresolution/resolution_test.go
git commit -m "feat(platformresolution): map pc-mac to mac platform slug"
```

---

### Task 2: Add Mac emission to GOG library

**Files:**
- Modify: `internal/services/gog/library.go`
- Modify: `internal/services/gog/library_test.go`

- [ ] **Step 1: Update the `product` test helper signature and add a failing test**

In `internal/services/gog/library_test.go`, change the `product` helper to accept a `mac` bool:

```go
func product(id int64, title string, windows, mac, linux bool) map[string]any {
	return map[string]any{
		"id":    id,
		"title": title,
		"worksOn": map[string]any{
			"Windows": windows,
			"Mac":     mac,
			"Linux":   linux,
		},
	}
}
```

Update every existing call to `product(...)` in the file — each currently has 4 args; add `false` as the third argument (mac position):

```go
// Before:                         product(1001, "Game A", true, false)
// After (add false for mac):
product(1001, "Game A", true, false, false)
// repeat for all other product(...) calls in the file
```

Then add the new test at the end of the file:

```go
func TestGetLibrary_MacGameEmitsMacEntry(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(5001, "Mac Game", true, true, false)},
	})
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	var entries []gog.ExternalLibraryEntry
	err := c.GetLibrary(context.Background(), "token", 50, func(batch []gog.ExternalLibraryEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (Windows + Mac), got %d", len(entries))
	}
	platforms := map[string]bool{}
	for _, e := range entries {
		if e.ExternalID != "5001" {
			t.Errorf("unexpected ExternalID %q", e.ExternalID)
		}
		platforms[e.RawPlatform] = true
	}
	if !platforms["pc-windows"] {
		t.Error("expected pc-windows entry")
	}
	if !platforms["pc-mac"] {
		t.Error("expected pc-mac entry")
	}
}
```

- [ ] **Step 2: Verify new test fails, existing tests still pass**

```bash
go test ./internal/services/gog/... -v
```
Expected: `TestGetLibrary_MacGameEmitsMacEntry` FAIL; all other tests PASS.

- [ ] **Step 3: Add Mac branch in `library.go`**

In `internal/services/gog/library.go`, in `fetchPage`, add after the Linux block (after `if p.WorksOn.Linux { ... }`):

```go
if p.WorksOn.Mac {
	entries = append(entries, ExternalLibraryEntry{
		ExternalID:      id,
		Title:           p.Title,
		RawPlatform:     "pc-mac",
		PlaytimeHours:   0,
		OwnershipStatus: "owned",
		IsSubscription:  false,
	})
}
```

Also update the `RawPlatform` doc comment on `ExternalLibraryEntry`:
```go
RawPlatform     string // "pc-windows", "pc-mac", or "pc-linux"
```

- [ ] **Step 4: Run all GOG tests**

```bash
go test ./internal/services/gog/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/services/gog/library.go internal/services/gog/library_test.go
git commit -m "feat(gog): emit pc-mac entries for Mac-supported games"
```

---

### Task 3: Refactor steam client and update consumers atomically

Changing `GetOwnedGames`'s return type from `[]ExternalLibraryEntry` to `[]OwnedGame` breaks `sync.go` and `sync_test.go`. All three files must be updated together to keep the build green. This task also adds `GetAppDetailsPlatforms`, the rate limiter, and base-URL fields for test injection, plus creates `client_test.go`.

**Files:**
- Modify: `internal/services/steam/client.go`
- Create: `internal/services/steam/client_test.go`
- Modify: `internal/worker/tasks/sync.go` (interface only — Steam case rewrite is Task 4)
- Modify: `internal/worker/tasks/sync_test.go` (fake adapter + existing Steam tests)

- [ ] **Step 1: Rewrite `internal/services/steam/client.go`**

Replace the entire file with:

```go
package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Client is an HTTP client for the Steam Web API.
type Client struct {
	http           *http.Client
	limiter        *rate.Limiter
	ownedGamesBase string // default "https://api.steampowered.com"
	appDetailsBase string // default "https://store.steampowered.com"
}

// NewClient creates a new Steam API client.
func NewClient() *Client {
	return &Client{
		http:           &http.Client{},
		limiter:        rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
		ownedGamesBase: "https://api.steampowered.com",
		appDetailsBase: "https://store.steampowered.com",
	}
}

// NewClientForTests creates a Steam API client with custom HTTP client, rate limiter,
// and base URLs. Only for use in tests.
func NewClientForTests(httpClient *http.Client, limiter *rate.Limiter, ownedGamesBase, appDetailsBase string) *Client {
	return &Client{
		http:           httpClient,
		limiter:        limiter,
		ownedGamesBase: ownedGamesBase,
		appDetailsBase: appDetailsBase,
	}
}

// SteamPlayerSummary is the steam-local type — does NOT import the api package.
type SteamPlayerSummary struct {
	PersonaName              string
	CommunityVisibilityState int
}

// OwnedGame is a game entry from the user's Steam library.
type OwnedGame struct {
	AppID         int
	Title         string
	PlaytimeHours int
}

// Platforms represents per-OS availability from the Steam store appdetails endpoint.
type Platforms struct {
	Windows bool
	Mac     bool
	Linux   bool
}

// GetPlayerSummaries fetches the player summary for the given steamID.
// Returns nil, nil if no player was found for that steamID.
func (c *Client) GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error) {
	url := fmt.Sprintf(
		"%s/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s&format=json",
		c.ownedGamesBase, apiKey, steamID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("steam: failed to create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steam network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("steam rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
	}

	var body struct {
		Response struct {
			Players []struct {
				PersonaName              string `json:"personaname"`
				CommunityVisibilityState int    `json:"communityvisibilitystate"`
			} `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("steam decode error: %w", err)
	}
	if len(body.Response.Players) == 0 {
		return nil, nil
	}
	p := body.Response.Players[0]
	return &SteamPlayerSummary{
		PersonaName:              p.PersonaName,
		CommunityVisibilityState: p.CommunityVisibilityState,
	}, nil
}

// GetOwnedGames fetches the full Steam library for the given steamID.
// playtime_forever from the API is in minutes; this method converts it to hours.
func (c *Client) GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]OwnedGame, error) {
	url := fmt.Sprintf(
		"%s/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&include_appinfo=true&format=json",
		c.ownedGamesBase, apiKey, steamID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("steam: failed to create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steam network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
	}

	var body struct {
		Response struct {
			Games []struct {
				AppID           int    `json:"appid"`
				Name            string `json:"name"`
				PlaytimeForever int    `json:"playtime_forever"`
			} `json:"games"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("steam decode error: %w", err)
	}

	games := make([]OwnedGame, 0, len(body.Response.Games))
	for _, g := range body.Response.Games {
		games = append(games, OwnedGame{
			AppID:         g.AppID,
			Title:         g.Name,
			PlaytimeHours: g.PlaytimeForever / 60,
		})
	}
	return games, nil
}

// GetAppDetailsPlatforms fetches platform availability for the given appID.
// Returns (Platforms{}, error) for non-200, success=false, decode error, or missing key.
// Returns (Platforms{}, nil) when success=true but all platforms are false — caller decides fallback.
func (c *Client) GetAppDetailsPlatforms(ctx context.Context, appID int) (Platforms, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return Platforms{}, err
	}
	url := fmt.Sprintf("%s/api/appdetails?appids=%d&filters=basics", c.appDetailsBase, appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Platforms{}, fmt.Errorf("steam appdetails HTTP %d for appid %d", resp.StatusCode, appID)
	}

	var body map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Platforms struct {
				Windows bool `json:"windows"`
				Mac     bool `json:"mac"`
				Linux   bool `json:"linux"`
			} `json:"platforms"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails decode error: %w", err)
	}
	key := fmt.Sprintf("%d", appID)
	entry, ok := body[key]
	if !ok {
		return Platforms{}, fmt.Errorf("steam appdetails: missing key %q in response", key)
	}
	if !entry.Success {
		return Platforms{}, fmt.Errorf("steam appdetails: success=false for appid %d", appID)
	}
	return Platforms{
		Windows: entry.Data.Platforms.Windows,
		Mac:     entry.Data.Platforms.Mac,
		Linux:   entry.Data.Platforms.Linux,
	}, nil
}
```

- [ ] **Step 2: Create `internal/services/steam/client_test.go`**

```go
package steam_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/services/steam"
)

func TestGetOwnedGames_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"games": []map[string]any{
					{"appid": 730, "name": "Counter-Strike 2", "playtime_forever": 120},
					{"appid": 440, "name": "Team Fortress 2", "playtime_forever": 0},
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if err != nil {
		t.Fatalf("GetOwnedGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}
	if games[0].AppID != 730 {
		t.Errorf("AppID: got %d, want 730", games[0].AppID)
	}
	if games[0].Title != "Counter-Strike 2" {
		t.Errorf("Title: got %q", games[0].Title)
	}
	if games[0].PlaytimeHours != 2 {
		t.Errorf("PlaytimeHours: got %d, want 2 (120 min / 60)", games[0].PlaytimeHours)
	}
	if games[1].PlaytimeHours != 0 {
		t.Errorf("PlaytimeHours for 0-minute game: got %d, want 0", games[1].PlaytimeHours)
	}
}

func TestGetAppDetailsPlatforms_HappyPath_MixedPlatforms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"730": map[string]any{
				"success": true,
				"data": map[string]any{
					"platforms": map[string]any{
						"windows": true,
						"mac":     false,
						"linux":   true,
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	pl, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err != nil {
		t.Fatalf("GetAppDetailsPlatforms: %v", err)
	}
	if !pl.Windows {
		t.Error("expected Windows=true")
	}
	if pl.Mac {
		t.Error("expected Mac=false")
	}
	if !pl.Linux {
		t.Error("expected Linux=true")
	}
}

func TestGetAppDetailsPlatforms_SuccessFalse_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"730": map[string]any{"success": false},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for success=false, got nil")
	}
}

func TestGetAppDetailsPlatforms_HTTP429_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for HTTP 429, got nil")
	}
}

func TestGetAppDetailsPlatforms_HTTP500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestGetAppDetailsPlatforms_AllFalsePlatforms_ReturnsZeroValueNoError(t *testing.T) {
	// success=true but all platforms false → caller applies Windows-only fallback.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"730": map[string]any{
				"success": true,
				"data": map[string]any{
					"platforms": map[string]any{
						"windows": false,
						"mac":     false,
						"linux":   false,
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	pl, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err != nil {
		t.Fatalf("expected no error for all-false platforms, got %v", err)
	}
	if pl.Windows || pl.Mac || pl.Linux {
		t.Errorf("expected all-false Platforms{}, got %+v", pl)
	}
}

func TestGetAppDetailsPlatforms_MissingAppIDKey_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"999": map[string]any{"success": true},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for missing appid key in response, got nil")
	}
}
```

- [ ] **Step 3: Update `SteamLibraryAdapter` in `internal/worker/tasks/sync.go`**

Replace lines 27–30 (the `SteamLibraryAdapter` interface declaration):

```go
// Before:
// SteamLibraryAdapter fetches the Steam game library.
type SteamLibraryAdapter interface {
	GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]steamsvc.ExternalLibraryEntry, error)
}

// After:
// SteamLibraryAdapter fetches the Steam game library.
type SteamLibraryAdapter interface {
	GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]steamsvc.OwnedGame, error)
	GetAppDetailsPlatforms(ctx context.Context, appID int) (steamsvc.Platforms, error)
}
```

- [ ] **Step 4: Replace `fakeSteamAdapter` in `internal/worker/tasks/sync_test.go`**

Replace the existing `fakeSteamAdapter` struct and its `GetOwnedGames` method (lines 33–40) with:

```go
// fakeSteamAdapter implements SteamLibraryAdapter for testing.
type fakeSteamAdapter struct {
	games             []steamsvc.OwnedGame
	ownedErr          error
	platformsByAppID  map[int]steamsvc.Platforms // nil entry → default {Windows: true}
	platformErrByAppID map[int]error
	queriedAppIDs     []int
}

func (f *fakeSteamAdapter) GetOwnedGames(_ context.Context, _, _ string) ([]steamsvc.OwnedGame, error) {
	return f.games, f.ownedErr
}

func (f *fakeSteamAdapter) GetAppDetailsPlatforms(_ context.Context, appID int) (steamsvc.Platforms, error) {
	f.queriedAppIDs = append(f.queriedAppIDs, appID)
	if f.platformErrByAppID != nil {
		if err, ok := f.platformErrByAppID[appID]; ok {
			return steamsvc.Platforms{}, err
		}
	}
	if f.platformsByAppID != nil {
		if pl, ok := f.platformsByAppID[appID]; ok {
			return pl, nil
		}
	}
	return steamsvc.Platforms{Windows: true}, nil
}
```

- [ ] **Step 5: Update existing Steam tests in `sync_test.go` to use new types**

In `TestDispatchSync_SteamFetchError`, change `err:` to `ownedErr:`:
```go
// Before:
adapter := &fakeSteamAdapter{err: errSteamFetch}
// After:
adapter := &fakeSteamAdapter{ownedErr: errSteamFetch}
```

In `TestDispatchSync_SteamSuccess`, change the `games` slice:
```go
// Before:
adapter := &fakeSteamAdapter{
	games: []steamsvc.ExternalLibraryEntry{
		{ExternalID: "730", Title: "Counter-Strike 2", RawPlatform: "PC", PlaytimeHours: 100, OwnershipStatus: "owned"},
	},
}
// After:
adapter := &fakeSteamAdapter{
	games: []steamsvc.OwnedGame{
		{AppID: 730, Title: "Counter-Strike 2", PlaytimeHours: 100},
	},
}
```

- [ ] **Step 6: Verify the build is clean**

```bash
go build ./...
```
Expected: zero errors

- [ ] **Step 7: Run steam client tests**

```bash
go test ./internal/services/steam/... -v
```
Expected: all PASS

- [ ] **Step 8: Run existing sync tests**

```bash
go test ./internal/worker/tasks/... -run "TestDispatchSync_Steam" -v -timeout 120s
```
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add internal/services/steam/client.go internal/services/steam/client_test.go \
        internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(steam): add OwnedGame/Platforms types, GetAppDetailsPlatforms, rate limiter"
```

---

### Task 4: Rewrite Steam dispatch case with multi-platform logic

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write four new failing DB tests in `sync_test.go`**

Add the following four test functions after `TestDispatchSync_SteamSuccess`. Note: `"errors"` is already imported in the file.

```go
func TestDispatchSync_Steam_MultiPlatform_WindowsAndLinux(t *testing.T) {
	// appdetails reports {Windows, Linux} for appid 730 →
	// expect two external_games rows and two job_items keyed "730:pc-windows" / "730:pc-linux".
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 730, Title: "Counter-Strike 2", PlaytimeHours: 100},
		},
		platformsByAppID: map[int]steamsvc.Platforms{
			730: {Windows: true, Linux: true},
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '730'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 2 {
		t.Errorf("expected 2 external_games rows (Windows+Linux), got %d", egCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 2 {
		t.Errorf("expected 2 job_items, got %d", itemCount)
	}

	var keys []string
	_ = testDB.NewRaw(
		`SELECT item_key FROM job_items WHERE job_id = ? ORDER BY item_key`, jobID,
	).Scan(ctx, &keys)
	if len(keys) != 2 || keys[0] != "730:pc-linux" || keys[1] != "730:pc-windows" {
		t.Errorf("unexpected item_keys: %v", keys)
	}
}

func TestDispatchSync_Steam_CacheHit_SkipsAppDetails(t *testing.T) {
	// Pre-seed an external_games row for appid 999.
	// Worker must NOT call GetAppDetailsPlatforms for 999; existing row stays available.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, creds,
	).Exec(ctx)

	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, raw_platform, is_available, is_skipped, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '999', 'Cached Game', 'pc-linux', true, false, false, 0)`,
		uuid.NewString(), userID,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 999, Title: "Cached Game", PlaytimeHours: 5},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, id := range adapter.queriedAppIDs {
		if id == 999 {
			t.Errorf("GetAppDetailsPlatforms was called for cached appid 999")
		}
	}

	var isAvail bool
	_ = testDB.NewRaw(
		`SELECT is_available FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '999'`,
		userID,
	).Scan(ctx, &isAvail)
	if !isAvail {
		t.Error("expected pre-seeded external_games row to remain is_available=true")
	}
}

func TestDispatchSync_Steam_AppDetailsFailure_NoRowNoItem(t *testing.T) {
	// First-time sync: GetAppDetailsPlatforms returns an error for appid 888 →
	// no external_games row written, no job_item created.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 888, Title: "Rate Limited Game", PlaytimeHours: 0},
		},
		platformErrByAppID: map[int]error{
			888: errors.New("appdetails 429: rate limited"),
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 0 {
		t.Errorf("expected 0 external_games rows after appdetails failure, got %d", egCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 0 {
		t.Errorf("expected 0 job_items for appdetails failure, got %d", itemCount)
	}

	queried := false
	for _, id := range adapter.queriedAppIDs {
		if id == 888 {
			queried = true
		}
	}
	if !queried {
		t.Error("expected GetAppDetailsPlatforms to be called for appid 888")
	}
}

func TestDispatchSync_Steam_NoPlatformsFallback_EmitsWindowsRow(t *testing.T) {
	// appdetails returns Platforms{} (success=true, all false) for appid 777 →
	// worker falls back to a single pc-windows row, item_key = "777:pc-windows".
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 777, Title: "No Platform Game", PlaytimeHours: 0},
		},
		platformsByAppID: map[int]steamsvc.Platforms{
			777: {}, // all false
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '777' AND raw_platform = 'pc-windows'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 1 {
		t.Errorf("expected 1 pc-windows external_games row for no-platforms fallback, got %d", egCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "777:pc-windows" {
		t.Errorf("expected item_key=777:pc-windows, got %q", itemKey)
	}
}
```

- [ ] **Step 2: Run new tests to verify they fail**

```bash
go test ./internal/worker/tasks/... \
  -run "TestDispatchSync_Steam_Multi|TestDispatchSync_Steam_Cache|TestDispatchSync_Steam_App|TestDispatchSync_Steam_No" \
  -v -timeout 120s
```
Expected: all FAIL — the Steam case still uses the old single-platform logic.

- [ ] **Step 3: Replace the Steam case in `sync.go`**

Find the `case "steam":` block in `Work()` (starts after the storefront switch and ends before `case "psn":`). Replace the entire block with:

```go
	case "steam":
		var creds struct {
			WebAPIKey string `json:"web_api_key"`
			SteamID   string `json:"steam_id"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
			return nil
		}
		owned, err := w.Steam.GetOwnedGames(ctx, creds.WebAPIKey, creds.SteamID)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("fetch steam library: %v", err))
			return nil
		}

		// Batch-load existing platform rows — rows already present skip appdetails.
		var cachedRows []struct {
			ExternalID  string `bun:"external_id"`
			RawPlatform string `bun:"raw_platform"`
		}
		_ = w.DB.NewRaw(
			`SELECT external_id, raw_platform FROM external_games WHERE user_id = ? AND storefront = 'steam'`,
			p.UserID,
		).Scan(ctx, &cachedRows)
		existing := make(map[string][]string, len(cachedRows))
		for _, r := range cachedRows {
			existing[r.ExternalID] = append(existing[r.ExternalID], r.RawPlatform)
		}

		for _, og := range owned {
			appidStr := fmt.Sprintf("%d", og.AppID)
			fetchedIDs[appidStr] = struct{}{}

			platforms := existing[appidStr]
			if len(platforms) == 0 {
				pl, detErr := w.Steam.GetAppDetailsPlatforms(ctx, og.AppID)
				if detErr != nil {
					slog.Warn("steam appdetails failed, skipping game this sync", "appid", og.AppID, "err", detErr)
					continue
				}
				if pl.Windows {
					platforms = append(platforms, "pc-windows")
				}
				if pl.Mac {
					platforms = append(platforms, "pc-mac")
				}
				if pl.Linux {
					platforms = append(platforms, "pc-linux")
				}
				if len(platforms) == 0 {
					platforms = []string{"pc-windows"}
				}
			}

			ownership := "owned"
			upsertNow := time.Now().UTC()
			for _, raw := range platforms {
				row := &models.ExternalGame{
					ID:              uuid.NewString(),
					UserID:          p.UserID,
					Storefront:      p.Storefront,
					ExternalID:      appidStr,
					Title:           og.Title,
					IsAvailable:     true,
					IsSubscription:  false,
					PlaytimeHours:   og.PlaytimeHours,
					OwnershipStatus: &ownership,
					RawPlatform:     raw,
					CreatedAt:       upsertNow,
					UpdatedAt:       upsertNow,
				}
				_, _ = w.DB.NewInsert().Model(row).
					On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
					Exec(ctx)
			}
		}

		var toProcess []models.ExternalGame
		_ = w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx)
		for _, eg := range toProcess {
			itemKey := eg.ExternalID + ":" + eg.RawPlatform
			metaJSON, _ := json.Marshal(map[string]any{
				"external_game_id": eg.ID,
				"raw_platform":     eg.RawPlatform,
				"playtime_hours":   eg.PlaytimeHours,
			})
			itemID := uuid.NewString()
			_, _ = w.DB.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				 ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, itemKey, eg.Title, string(metaJSON),
			).Exec(ctx)
			if w.RiverClient != nil {
				_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
			}
		}
```

- [ ] **Step 4: Run the four new tests to verify they pass**

```bash
go test ./internal/worker/tasks/... \
  -run "TestDispatchSync_Steam_Multi|TestDispatchSync_Steam_Cache|TestDispatchSync_Steam_App|TestDispatchSync_Steam_No" \
  -v -timeout 120s
```
Expected: all PASS

- [ ] **Step 5: Run the full test suite**

```bash
go test ./... -timeout 600s
```
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(steam): multi-platform detection via appdetails with external_games cache"
```

---

### Task 5: Final verification and plan file commit

- [ ] **Step 1: Run linter**

```bash
golangci-lint run
```
Expected: zero errors

- [ ] **Step 2: Run full test suite one final time**

```bash
go test -timeout 600s -cover ./...
```
Expected: all PASS

- [ ] **Step 3: Commit the plan file**

```bash
git add docs/superpowers/plans/2026-05-21-steam-multi-platform-sync.md
git commit -m "docs: add implementation plan for issue #526 steam multi-platform sync"
```
