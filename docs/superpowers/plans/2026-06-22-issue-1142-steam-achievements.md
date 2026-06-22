# Steam Achievement Progress ("X of Y") Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface per-storefront Steam achievement completion as a compact "X of Y" count on each user-game (card badge + detail-view line), folded into the existing Steam library sync.

**Architecture:** Achievement counts follow the exact two-hop path `hours_played` already takes: the Steam adapter fetches them per played game → `ExternalGameEntry` → `upsertPlatforms` → `external_game_platforms` → read back into `usergame.PlatformInput` → `usergame.Acquire` → `user_game_platforms` → REST DTO → React. Two nullable `integer` columns on both platform tables; `NULL` means "no badge" for any reason.

**Tech Stack:** Go 1.26 (Bun ORM, Echo v5, River), PostgreSQL, Vite/React 19/TypeScript, Vitest.

## Global Constraints

- **Steam only; counts only; display only.** No per-achievement names; no list filter/sort by completion. (Spec scope.)
- **Both new columns are nullable `integer`** on both `external_game_platforms` and `user_game_platforms`. No `NOT NULL`, no default.
- **`success:false` from Steam → leave counts `NULL`** (covers both private-profile and no-achievements; do not parse the error string to synthesize `total=0`).
- **Fetch achievements only when `playtime_forever > 0`.** No skip-if-recent guard, no `achievements_synced_at` column.
- **The sync never fails on an achievement error** — log and continue, counts stay `NULL`.
- **All writes to `user_game_platforms` route through `internal/usergame`** (the #1056 mutation boundary) — never hand-chain SQL elsewhere.
- Migration naming: `YYYYMMDD<nnnnnn>_name.up.sql` / `.down.sql`; next file is `20260622000002_*` (running number after `20260622000001_create_smell_ignores`).
- `errcheck` runs with `check-blank`; `gosec` enabled. Use `slog.*Context(ctx, …)` + `logging.Cat(...)` on error boundaries in worker code.

---

### Task 1: Migration + model structs

**Files:**
- Create: `internal/db/migrations/20260622000002_add_achievement_counts.up.sql`
- Create: `internal/db/migrations/20260622000002_add_achievement_counts.down.sql`
- Modify: `internal/db/models/models.go` (`UserGamePlatform` ~95-114, `ExternalGamePlatform` ~202-210)

**Interfaces:**
- Produces: columns `achievements_unlocked`, `achievements_total` (nullable `integer`) on both tables; model fields `UserGamePlatform.AchievementsUnlocked *int`, `.AchievementsTotal *int`, and the same two on `ExternalGamePlatform`.

- [ ] **Step 1: Write the up migration**

Create `internal/db/migrations/20260622000002_add_achievement_counts.up.sql`:

```sql
ALTER TABLE external_game_platforms
    ADD COLUMN achievements_unlocked integer,
    ADD COLUMN achievements_total    integer;

ALTER TABLE user_game_platforms
    ADD COLUMN achievements_unlocked integer,
    ADD COLUMN achievements_total    integer;
```

- [ ] **Step 2: Write the down migration**

Create `internal/db/migrations/20260622000002_add_achievement_counts.down.sql`:

```sql
ALTER TABLE user_game_platforms
    DROP COLUMN achievements_unlocked,
    DROP COLUMN achievements_total;

ALTER TABLE external_game_platforms
    DROP COLUMN achievements_unlocked,
    DROP COLUMN achievements_total;
```

- [ ] **Step 3: Add fields to the `UserGamePlatform` model**

In `internal/db/models/models.go`, add to the `UserGamePlatform` struct (after `HoursPlayed`):

```go
	AchievementsUnlocked *int `bun:"achievements_unlocked" json:"achievements_unlocked"`
	AchievementsTotal    *int `bun:"achievements_total"    json:"achievements_total"`
```

- [ ] **Step 4: Add fields to the `ExternalGamePlatform` model**

In the same file, add to the `ExternalGamePlatform` struct (after `HoursPlayed`):

```go
	AchievementsUnlocked *int `bun:"achievements_unlocked" json:"achievements_unlocked"`
	AchievementsTotal    *int `bun:"achievements_total"    json:"achievements_total"`
```

- [ ] **Step 5: Build to verify the package compiles**

Run: `go build ./internal/db/...`
Expected: no output, exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/20260622000002_add_achievement_counts.up.sql \
        internal/db/migrations/20260622000002_add_achievement_counts.down.sql \
        internal/db/models/models.go
git commit -m "feat: add achievement count columns to platform tables (#1142)"
```

---

### Task 2: Steam client `GetPlayerAchievements`

**Files:**
- Modify: `internal/services/steam/client.go` (`Client` struct ~20-25, `NewClient` ~28-35, `NewClientForTests` ~39-46; add new method)
- Test: `internal/services/steam/client_test.go`

**Interfaces:**
- Consumes: `Client.ownedGamesBase`, `Client.http`.
- Produces: `func (c *Client) GetPlayerAchievements(ctx context.Context, apiKey, steamID string, appID int) (unlocked, total int, ok bool, err error)`. `ok=false` (with `err=nil`) when Steam returns `success:false`; `err` set only on transport/HTTP/decode failure.

- [ ] **Step 1: Add a dedicated achievements rate limiter to the client**

In `internal/services/steam/client.go`, add a field to `Client`:

```go
type Client struct {
	http                *http.Client
	limiter             *rate.Limiter
	achievementsLimiter *rate.Limiter
	ownedGamesBase      string // default "https://api.steampowered.com"
	appDetailsBase      string // default "https://store.steampowered.com"
}
```

In `NewClient`, initialise it (mirror the 200 ms cadence):

```go
	return &Client{
		http:                &http.Client{Transport: observability.HTTPTransport()},
		limiter:             rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
		achievementsLimiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
		ownedGamesBase:      "https://api.steampowered.com",
		appDetailsBase:      "https://store.steampowered.com",
	}
```

In `NewClientForTests`, add the same limiter so tests do not nil-panic:

```go
	return &Client{
		http:                httpClient,
		limiter:             limiter,
		achievementsLimiter: rate.NewLimiter(rate.Inf, 1),
		ownedGamesBase:      ownedGamesBase,
		appDetailsBase:      appDetailsBase,
	}
```

- [ ] **Step 2: Write the failing test**

Add to `internal/services/steam/client_test.go` (create the file if absent; package `steam`):

```go
package steam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestGetPlayerAchievements(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantUnlocked int
		wantTotal    int
		wantOK       bool
	}{
		{
			name:         "mixed",
			body:         `{"playerstats":{"success":true,"achievements":[{"apiname":"a","achieved":1},{"apiname":"b","achieved":0},{"apiname":"c","achieved":1}]}}`,
			wantUnlocked: 2, wantTotal: 3, wantOK: true,
		},
		{
			name:         "all locked",
			body:         `{"playerstats":{"success":true,"achievements":[{"apiname":"a","achieved":0},{"apiname":"b","achieved":0}]}}`,
			wantUnlocked: 0, wantTotal: 2, wantOK: true,
		},
		{
			name:   "private profile",
			body:   `{"playerstats":{"error":"Profile is not public","success":false}}`,
			wantOK: false,
		},
		{
			name:   "no stats",
			body:   `{"playerstats":{"error":"Requested app has no stats","success":false}}`,
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)

			unlocked, total, ok, err := c.GetPlayerAchievements(context.Background(), "key", "steam123", 440)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (unlocked != tc.wantUnlocked || total != tc.wantTotal) {
				t.Errorf("got %d/%d, want %d/%d", unlocked, total, tc.wantUnlocked, tc.wantTotal)
			}
		})
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/services/steam/ -run TestGetPlayerAchievements -v`
Expected: FAIL — `c.GetPlayerAchievements undefined`.

- [ ] **Step 4: Implement `GetPlayerAchievements`**

Add to `internal/services/steam/client.go`:

```go
// GetPlayerAchievements fetches per-game achievement state for the given appID.
// Returns (unlocked, total, ok=true, nil) on a success=true response, where
// total = number of achievements and unlocked = number with achieved == 1.
// Returns (0, 0, false, nil) when Steam responds success=false (private profile
// OR the app has no achievements — indistinguishable without parsing the error
// string, and both map to "no badge"). Returns a non-nil error only on a
// transport, HTTP, or decode failure.
func (c *Client) GetPlayerAchievements(ctx context.Context, apiKey, steamID string, appID int) (int, int, bool, error) {
	if err := c.achievementsLimiter.Wait(ctx); err != nil {
		return 0, 0, false, err
	}
	url := fmt.Sprintf(
		"%s/ISteamUserStats/GetPlayerAchievements/v0001/?appid=%d&key=%s&steamid=%s&format=json",
		c.ownedGamesBase, appID, apiKey, steamID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, false, fmt.Errorf("steam achievements: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, false, fmt.Errorf("steam achievements network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 400/403 are Steam's response for private profiles on some apps; treat any
	// non-200 as "no data" rather than failing the sync.
	if resp.StatusCode != http.StatusOK {
		return 0, 0, false, nil
	}

	var body struct {
		PlayerStats struct {
			Success      bool `json:"success"`
			Achievements []struct {
				Achieved int `json:"achieved"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, 0, false, fmt.Errorf("steam achievements decode error: %w", err)
	}
	if !body.PlayerStats.Success {
		return 0, 0, false, nil
	}
	total := len(body.PlayerStats.Achievements)
	unlocked := 0
	for _, a := range body.PlayerStats.Achievements {
		if a.Achieved == 1 {
			unlocked++
		}
	}
	return unlocked, total, true, nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/services/steam/ -run TestGetPlayerAchievements -v`
Expected: PASS (all four sub-cases).

- [ ] **Step 6: Commit**

```bash
git add internal/services/steam/client.go internal/services/steam/client_test.go
git commit -m "feat: add Steam GetPlayerAchievements client method (#1142)"
```

---

### Task 3: `ExternalGameEntry` fields + adapter wiring

**Files:**
- Modify: `internal/services/storefrontadapter/storefrontadapter.go` (`ExternalGameEntry` ~9-20)
- Modify: `internal/services/steam/adapter.go` (`GetLibrary` per-game loop ~84-112; add a helper)
- Test: `internal/services/steam/adapter_test.go`

**Interfaces:**
- Consumes: `Client.GetPlayerAchievements` (Task 2); `OwnedGame.PlaytimeHours`.
- Produces: `ExternalGameEntry.AchievementsUnlocked *int`, `.AchievementsTotal *int` (nil when the source provides nothing).

- [ ] **Step 1: Add the two fields to `ExternalGameEntry`**

In `internal/services/storefrontadapter/storefrontadapter.go`, add to the struct (after `IsSubscription`):

```go
	// AchievementsUnlocked / AchievementsTotal hold per-game achievement counts
	// when the source provides them (Steam only in v1). Nil = not fetched / not
	// available. Carried onto the first platform row, like PlaytimeHours.
	AchievementsUnlocked *int
	AchievementsTotal    *int
```

- [ ] **Step 2: Write the failing adapter test**

Add to `internal/services/steam/adapter_test.go` (package `steam`). This stubs both the owned-games web API and the store appdetails API on one server, keyed by path:

```go
func TestGetLibrary_PopulatesAchievements(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "GetOwnedGames"):
			_, _ = w.Write([]byte(`{"response":{"games":[
				{"appid":10,"name":"Played","playtime_forever":120},
				{"appid":20,"name":"Unplayed","playtime_forever":0}]}}`))
		case strings.Contains(r.URL.Path, "GetPlayerAchievements"):
			// Only the played game should reach here.
			_, _ = w.Write([]byte(`{"playerstats":{"success":true,"achievements":[
				{"apiname":"a","achieved":1},{"apiname":"b","achieved":0}]}}`))
		case strings.Contains(r.URL.Path, "appdetails"):
			_, _ = w.Write([]byte(`{"10":{"success":true,"data":{"platforms":{"windows":true}}},
				"20":{"success":true,"data":{"platforms":{"windows":true}}}}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	a := NewAdapterForTests(c, "key", "steam123", 0)

	var got []storefrontadapter.ExternalGameEntry
	if err := a.GetLibrary(context.Background(), 10, func(b []storefrontadapter.ExternalGameEntry) error {
		got = append(got, b...)
		return nil
	}); err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}

	byID := map[string]storefrontadapter.ExternalGameEntry{}
	for _, e := range got {
		byID[e.ExternalID] = e
	}
	played := byID["10"]
	if played.AchievementsTotal == nil || *played.AchievementsTotal != 2 ||
		played.AchievementsUnlocked == nil || *played.AchievementsUnlocked != 1 {
		t.Errorf("played game: got %v/%v, want 1/2", played.AchievementsUnlocked, played.AchievementsTotal)
	}
	if unplayed := byID["20"]; unplayed.AchievementsTotal != nil || unplayed.AchievementsUnlocked != nil {
		t.Errorf("unplayed game should have nil achievements, got %v/%v",
			unplayed.AchievementsUnlocked, unplayed.AchievementsTotal)
	}
}
```

Ensure the test file imports `"context"`, `"net/http"`, `"net/http/httptest"`, `"strings"`, `"testing"`, `"golang.org/x/time/rate"`, and `"github.com/drzero42/nexorious/internal/services/storefrontadapter"`.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/services/steam/ -run TestGetLibrary_PopulatesAchievements -v`
Expected: FAIL — achievement fields nil on the played game (not yet wired).

- [ ] **Step 4: Wire the fetch into `GetLibrary`**

In `internal/services/steam/adapter.go`, inside the per-game loop, after the `platforms` slice is built and before the `entries = append(...)` call (~line 104), fetch achievements for played games:

```go
			unlocked, total := a.fetchAchievements(ctx, og)

			entries = append(entries, storefrontadapter.ExternalGameEntry{
				ExternalID:           fmt.Sprintf("%d", og.AppID),
				Title:                og.Title,
				PlaytimeHours:        og.PlaytimeHours,
				Platforms:            platforms,
				OwnershipStatus:      "owned",
				IsSubscription:       false,
				AchievementsUnlocked: unlocked,
				AchievementsTotal:    total,
			})
```

Add the helper to the same file:

```go
// fetchAchievements returns the unlocked/total achievement counts for one game,
// or (nil, nil) when the game is unplayed, Steam has no public stats, or the
// call errors. Achievement data must never fail the sync, so errors are logged
// and swallowed.
func (a *Adapter) fetchAchievements(ctx context.Context, og OwnedGame) (*int, *int) {
	if og.PlaytimeHours <= 0 {
		return nil, nil
	}
	unlocked, total, ok, err := a.client.GetPlayerAchievements(ctx, a.apiKey, a.steamID, og.AppID)
	if err != nil {
		slog.WarnContext(ctx, "steam: fetch achievements failed",
			"appid", og.AppID, "title", og.Title, logging.KeyErr, err,
			logging.Cat(logging.CategoryExternalAPI))
		return nil, nil
	}
	if !ok {
		return nil, nil
	}
	return &unlocked, &total
}
```

(`slog`, `logging`, and `time` are already imported in this file.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/services/steam/ -run TestGetLibrary_PopulatesAchievements -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/storefrontadapter/storefrontadapter.go \
        internal/services/steam/adapter.go internal/services/steam/adapter_test.go
git commit -m "feat: fetch Steam achievements per played game during sync (#1142)"
```

---

### Task 4: `usergame` PlatformInput + `mergeOnePlatform` persistence

**Files:**
- Modify: `internal/usergame/types.go` (`PlatformInput` ~30-45)
- Modify: `internal/usergame/acquire.go` (`mergeOnePlatform` INSERT ~168-174 and UPDATE ~198-201)
- Test: `internal/usergame/acquire_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `PlatformInput.AchievementsUnlocked *int`, `.AchievementsTotal *int`; `Acquire` persists them to `user_game_platforms` on insert, and refreshes them on update via `COALESCE(?, existing)` (a `nil` input preserves the stored value — a transient private-profile blip must not wipe known counts).

> **Scope note:** Only `mergeOnePlatform` (the `Acquire`/sync path) writes achievements. `insertPlatformStrict` and `AddPlatformBulk` are manual paths that carry no achievement data; they omit the columns, which default to `NULL`. No change needed there.

- [ ] **Step 1: Add the two fields to `PlatformInput`**

In `internal/usergame/types.go`, add to the `PlatformInput` struct (after `SyncFromSource`):

```go
	// AchievementsUnlocked / AchievementsTotal carry Steam achievement counts on
	// the sync path. Nil leaves the columns NULL on insert and unchanged on update.
	AchievementsUnlocked *int
	AchievementsTotal    *int
```

- [ ] **Step 2: Write the failing test**

Add to `internal/usergame/acquire_test.go` (package `usergame`, uses the shared `testDB`). Follow the existing `TestAcquire_*` setup pattern (truncate, seed a user + game). Use the existing helpers (`seedUserAndGame` or whatever the neighbouring tests use — match them):

```go
func TestAcquire_PersistsAndRefreshesAchievements(t *testing.T) {
	truncateAllTables(t)
	u, gameID := seedUserAndGame(t) // match the helper neighbouring tests use

	two, one := 2, 1
	res, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: gameID, Mode: ModeUpsert,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(3), SyncFromSource: true,
			AchievementsUnlocked: &one, AchievementsTotal: &two,
		}},
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	var gotUnlocked, gotTotal *int
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM user_game_platforms WHERE user_game_id = ?`,
		res.UserGameID,
	).Scan(context.Background(), &gotUnlocked, &gotTotal); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if gotUnlocked == nil || *gotUnlocked != 1 || gotTotal == nil || *gotTotal != 2 {
		t.Fatalf("after insert: got %v/%v, want 1/2", gotUnlocked, gotTotal)
	}

	// Re-sync with nil achievements (success=false this round) must preserve counts.
	if _, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: gameID, Mode: ModeUpsert,
		Platforms: []PlatformInput{{
			Platform: strptr("pc-windows"), Storefront: strptr("steam"),
			HoursPlayed: fptr(4), SyncFromSource: true,
			AchievementsUnlocked: nil, AchievementsTotal: nil,
		}},
	}); err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM user_game_platforms WHERE user_game_id = ?`,
		res.UserGameID,
	).Scan(context.Background(), &gotUnlocked, &gotTotal); err != nil {
		t.Fatalf("scan after re-acquire: %v", err)
	}
	if gotUnlocked == nil || *gotUnlocked != 1 || gotTotal == nil || *gotTotal != 2 {
		t.Fatalf("after nil re-sync: got %v/%v, want preserved 1/2", gotUnlocked, gotTotal)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/usergame/ -run TestAcquire_PersistsAndRefreshesAchievements -v`
Expected: FAIL — columns written as NULL (insert doesn't bind them yet) or scan mismatch.

- [ ] **Step 4: Update the INSERT branch of `mergeOnePlatform`**

In `internal/usergame/acquire.go`, change the `sql.ErrNoRows` (insert) branch to include the new columns:

```go
		_, err := tx.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, external_game_id, acquired_date, sync_from_source, achievements_unlocked, achievements_total, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			uuid.NewString(), ugID, in.Platform, in.Storefront, available, in.HoursPlayed, ownership, in.ExternalGameID, in.AcquiredDate, in.SyncFromSource, in.AchievementsUnlocked, in.AchievementsTotal,
		).Exec(ctx)
```

- [ ] **Step 5: Update the UPDATE branch of `mergeOnePlatform`**

Change the UPDATE statement to refresh achievements with `COALESCE` (nil preserves existing):

```go
		_, err := tx.NewRaw(
			`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, external_game_id = COALESCE(?, external_game_id), achievements_unlocked = COALESCE(?, achievements_unlocked), achievements_total = COALESCE(?, achievements_total), updated_at = now() WHERE id = ?`,
			finalOwnership, finalHours, in.ExternalGameID, in.AchievementsUnlocked, in.AchievementsTotal, existingID,
		).Exec(ctx)
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/usergame/ -run TestAcquire_PersistsAndRefreshesAchievements -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/types.go internal/usergame/acquire.go internal/usergame/acquire_test.go
git commit -m "feat: persist achievement counts through usergame.Acquire (#1142)"
```

---

### Task 5: Sync worker — carry counts to `external_game_platforms` and back

**Files:**
- Modify: `internal/worker/tasks/sync.go` (`upsertPlatforms` ~127-143; call site ~225; read-back loop ~728-735)
- Test: `internal/worker/tasks/sync_test.go`

**Interfaces:**
- Consumes: `ExternalGameEntry.AchievementsUnlocked/Total` (Task 3); `PlatformInput.AchievementsUnlocked/Total` (Task 4).
- Produces: counts written to `external_game_platforms` (platform index 0 only) and forwarded into `usergame.PlatformInput` in the read-back loop.

- [ ] **Step 1: Write the failing test for `upsertPlatforms`**

Add to `internal/worker/tasks/sync_test.go` (package `tasks`, shared `testDB`). Seed an `external_games` row first (match the seeding helper neighbouring sync tests use), then:

```go
func TestUpsertPlatforms_WritesAchievements(t *testing.T) {
	truncateAllTables(t)
	egID := seedExternalGame(t) // match the helper used by neighbouring sync tests

	four, three := 4, 3
	upsertPlatforms(context.Background(), testDB, egID, []string{"pc-windows", "pc-linux"}, 5.0, &three, &four)

	var unlocked, total *int
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM external_game_platforms WHERE external_game_id = ? AND platform = 'pc-windows'`,
		egID,
	).Scan(context.Background(), &unlocked, &total); err != nil {
		t.Fatalf("scan windows: %v", err)
	}
	if unlocked == nil || *unlocked != 3 || total == nil || *total != 4 {
		t.Fatalf("index-0 row: got %v/%v, want 3/4", unlocked, total)
	}

	// Non-first platform rows get NULL achievements (like hours).
	if err := testDB.NewRaw(
		`SELECT achievements_unlocked, achievements_total FROM external_game_platforms WHERE external_game_id = ? AND platform = 'pc-linux'`,
		egID,
	).Scan(context.Background(), &unlocked, &total); err != nil {
		t.Fatalf("scan linux: %v", err)
	}
	if unlocked != nil || total != nil {
		t.Fatalf("non-first row should be NULL, got %v/%v", unlocked, total)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestUpsertPlatforms_WritesAchievements -v`
Expected: FAIL — `upsertPlatforms` signature has no achievement params (compile error).

- [ ] **Step 3: Update `upsertPlatforms`**

In `internal/worker/tasks/sync.go`, change the signature and SQL:

```go
func upsertPlatforms(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64, achievementsUnlocked, achievementsTotal *int) {
	for i, platform := range platforms {
		hours := 0.0
		var unlocked, total *int
		if i == 0 {
			hours = playtimeHours
			unlocked = achievementsUnlocked
			total = achievementsTotal
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, achievements_unlocked, achievements_total, created_at)
			VALUES (?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played),
				achievements_unlocked = COALESCE(EXCLUDED.achievements_unlocked, external_game_platforms.achievements_unlocked),
				achievements_total = COALESCE(EXCLUDED.achievements_total, external_game_platforms.achievements_total)`,
			uuid.NewString(), egID, platform, hours, unlocked, total,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "dispatch_sync: upsert platform failed", logging.KeyErr, err, logging.KeyExternalGameID, egID, "platform", platform, logging.Cat(logging.CategoryDB))
		}
	}
}
```

- [ ] **Step 4: Update the call site**

At ~line 225, pass the entry's achievement counts:

```go
			upsertPlatforms(ctx, w.DB, egID, platforms, e.PlaytimeHours, e.AchievementsUnlocked, e.AchievementsTotal)
```

- [ ] **Step 5: Forward counts in the read-back loop**

In the `process_sync_item` read-back loop (~728-735), add the two fields to each `usergame.PlatformInput`:

```go
		plats = append(plats, usergame.PlatformInput{
			Platform: &egp.Platform, Storefront: &storefrontSlug,
			HoursPlayed: &egp.HoursPlayed, OwnershipStatus: &ownership,
			IsAvailable: boolptr(true), ExternalGameID: &eg.ID,
			SyncFromSource:       true,
			AchievementsUnlocked: egp.AchievementsUnlocked,
			AchievementsTotal:    egp.AchievementsTotal,
		})
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run TestUpsertPlatforms_WritesAchievements -v`
Expected: PASS.

- [ ] **Step 7: Build the whole worker package and commit**

Run: `go build ./internal/worker/...`
Expected: exit 0.

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: carry achievement counts through the sync pipeline (#1142)"
```

---

### Task 6: REST API DTO

**Files:**
- Modify: `internal/api/user_games.go` (`userGamePlatformResponse` ~52-68; `toUserGamePlatformResponse` ~70-84)
- Test: `internal/api/user_games_test.go`

**Interfaces:**
- Consumes: `models.UserGamePlatform.AchievementsUnlocked/Total` (Task 1).
- Produces: JSON keys `achievements_unlocked`, `achievements_total` on each platform object.

- [ ] **Step 1: Add the fields to the DTO**

In `internal/api/user_games.go`, add to `userGamePlatformResponse` (after `HoursPlayed`):

```go
	AchievementsUnlocked *int                `json:"achievements_unlocked"`
	AchievementsTotal    *int                `json:"achievements_total"`
```

- [ ] **Step 2: Map them in `toUserGamePlatformResponse`**

Add to the struct literal (after `HoursPlayed: ugp.HoursPlayed,`):

```go
		AchievementsUnlocked: ugp.AchievementsUnlocked,
		AchievementsTotal:    ugp.AchievementsTotal,
```

- [ ] **Step 3: Write a test asserting the JSON keys**

Add to `internal/api/user_games_test.go` a direct unit test of the mapper (no HTTP needed):

```go
func TestToUserGamePlatformResponse_Achievements(t *testing.T) {
	u, total := 7, 10
	resp := toUserGamePlatformResponse(models.UserGamePlatform{
		ID:                   "p1",
		AchievementsUnlocked: &u,
		AchievementsTotal:    &total,
	})
	if resp.AchievementsUnlocked == nil || *resp.AchievementsUnlocked != 7 ||
		resp.AchievementsTotal == nil || *resp.AchievementsTotal != 10 {
		t.Fatalf("got %v/%v, want 7/10", resp.AchievementsUnlocked, resp.AchievementsTotal)
	}
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/api/ -run TestToUserGamePlatformResponse_Achievements -v`
Expected: PASS (write fails first if the fields are absent — run before Step 1/2 if practising strict TDD).

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat: expose achievement counts in user-game platform API (#1142)"
```

---

### Task 7: Frontend types + transform

**Files:**
- Modify: `ui/frontend/src/types/game.ts` (`UserGamePlatform` ~76-88)
- Modify: `ui/frontend/src/api/games.ts` (`UserGamePlatformApiResponse` ~55-67)

**Interfaces:**
- Produces: `UserGamePlatform.achievements_unlocked?: number | null`, `.achievements_total?: number | null` available to components. `transformUserGamePlatform` already spreads `...apiPlatform`, so no transform change is needed once the API interface carries the fields.

- [ ] **Step 1: Add fields to the domain type**

In `ui/frontend/src/types/game.ts`, add to `UserGamePlatform` (after `hours_played`):

```ts
  achievements_unlocked?: number | null;
  achievements_total?: number | null;
```

- [ ] **Step 2: Add fields to the API response interface**

In `ui/frontend/src/api/games.ts`, add to `UserGamePlatformApiResponse` (after `hours_played`):

```ts
  achievements_unlocked?: number | null;
  achievements_total?: number | null;
```

- [ ] **Step 3: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/types/game.ts ui/frontend/src/api/games.ts
git commit -m "feat: add achievement counts to frontend platform types (#1142)"
```

---

### Task 8: Card badge (highest-progress helper + render)

**Files:**
- Modify: `ui/frontend/src/lib/game-utils.ts` (add helper)
- Modify: `ui/frontend/src/components/games/game-card.tsx` (~7 import, ~146-157 render)
- Test: `ui/frontend/src/lib/game-utils.test.ts` (create if absent) and/or `ui/frontend/src/components/games/game-card.test.tsx`

**Interfaces:**
- Consumes: `UserGamePlatform.achievements_*` (Task 7).
- Produces: `bestAchievementProgress(platforms?: UserGamePlatform[]): { unlocked: number; total: number } | null` — the platform row with the highest `unlocked/total` ratio among rows with `total > 0`; `null` when none qualify.

- [ ] **Step 1: Write the failing helper test**

Add to `ui/frontend/src/lib/game-utils.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { bestAchievementProgress } from './game-utils';
import type { UserGamePlatform } from '@/types';

const plat = (over: Partial<UserGamePlatform>): UserGamePlatform =>
  ({ id: 'x', is_available: true, hours_played: 0, ownership_status: 'owned', created_at: '', ...over }) as UserGamePlatform;

describe('bestAchievementProgress', () => {
  it('returns null when undefined or empty', () => {
    expect(bestAchievementProgress(undefined)).toBeNull();
    expect(bestAchievementProgress([])).toBeNull();
  });
  it('ignores rows with null or zero total', () => {
    expect(bestAchievementProgress([plat({}), plat({ achievements_total: 0, achievements_unlocked: 0 })])).toBeNull();
  });
  it('returns the single qualifying row', () => {
    expect(bestAchievementProgress([plat({ achievements_unlocked: 3, achievements_total: 10 })])).toEqual({ unlocked: 3, total: 10 });
  });
  it('picks the highest unlocked/total ratio', () => {
    const result = bestAchievementProgress([
      plat({ achievements_unlocked: 5, achievements_total: 10 }), // 0.5
      plat({ achievements_unlocked: 9, achievements_total: 10 }), // 0.9
    ]);
    expect(result).toEqual({ unlocked: 9, total: 10 });
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run (from `ui/frontend/`): `npm run test game-utils.test.ts`
Expected: FAIL — `bestAchievementProgress` not exported.

- [ ] **Step 3: Implement the helper**

Add to `ui/frontend/src/lib/game-utils.ts` (import the type at the top if not present: `import type { UserGamePlatform } from '@/types';`):

```ts
export function bestAchievementProgress(
  platforms?: UserGamePlatform[],
): { unlocked: number; total: number } | null {
  if (!platforms) return null;
  let best: { unlocked: number; total: number } | null = null;
  for (const p of platforms) {
    if (p.achievements_total == null || p.achievements_total <= 0) continue;
    const unlocked = p.achievements_unlocked ?? 0;
    if (best === null || unlocked / p.achievements_total > best.unlocked / best.total) {
      best = { unlocked, total: p.achievements_total };
    }
  }
  return best;
}
```

- [ ] **Step 4: Run to verify it passes**

Run (from `ui/frontend/`): `npm run test game-utils.test.ts`
Expected: PASS.

- [ ] **Step 5: Render the badge on the card**

In `ui/frontend/src/components/games/game-card.tsx`, update the lucide import (line 7) to include `Trophy`:

```tsx
import { Timer, Gamepad2, Trophy } from 'lucide-react';
```

Add `bestAchievementProgress` to the `game-utils` import (line 9). Inside the component body (after `const buyFirst = ...`, ~line 33):

```tsx
  const achievements = bestAchievementProgress(game.platforms);
```

Render a compact row after the HLTB block (after line 157, before the actions slot):

```tsx
        {achievements && (
          <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
            <Trophy className="h-3 w-3" />
            <span>
              {achievements.unlocked}/{achievements.total}
            </span>
          </div>
        )}
```

- [ ] **Step 6: Add a card render test**

Add to `ui/frontend/src/components/games/game-card.test.tsx` (match the existing render-test setup in that file for building a `UserGame` fixture):

```tsx
it('shows the achievement badge from the highest-progress platform', () => {
  const game = makeGame({
    platforms: [
      makePlatform({ achievements_unlocked: 9, achievements_total: 10 }),
      makePlatform({ achievements_unlocked: 1, achievements_total: 10 }),
    ],
  });
  render(<GameCard game={game} />);
  expect(screen.getByText('9/10')).toBeInTheDocument();
});

it('hides the achievement badge when no platform has achievements', () => {
  const game = makeGame({ platforms: [makePlatform({})] });
  render(<GameCard game={game} />);
  expect(screen.queryByText(/\/\d/)).not.toBeInTheDocument();
});
```

(Use the file's existing fixture builders; if it lacks `makeGame`/`makePlatform`, mirror whatever pattern the neighbouring tests use to construct props.)

- [ ] **Step 7: Run card tests + typecheck**

Run (from `ui/frontend/`): `npm run test game-card.test.tsx && npm run check`
Expected: PASS, no type errors.

- [ ] **Step 8: Commit**

```bash
git add ui/frontend/src/lib/game-utils.ts ui/frontend/src/lib/game-utils.test.ts \
        ui/frontend/src/components/games/game-card.tsx ui/frontend/src/components/games/game-card.test.tsx
git commit -m "feat: show achievement badge on the game card (#1142)"
```

---

### Task 9: Detail-view per-storefront achievement line

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` (`Platforms & Ownership` block ~494-521; ensure `Trophy` imported)

**Interfaces:**
- Consumes: `UserGamePlatform.achievements_*` (Task 7).

- [ ] **Step 1: Ensure `Trophy` is imported**

In `ui/frontend/src/routes/_authenticated/games/$id.index.tsx`, add `Trophy` to the existing `lucide-react` import.

- [ ] **Step 2: Render the per-platform achievement count**

Inside the platform row's right-hand cluster (the `<div className="flex items-center gap-2">` at ~510), add before the ownership `<Badge>`:

```tsx
                            {p.achievements_total != null && p.achievements_total > 0 && (
                              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                <Trophy className="h-3 w-3" />
                                {p.achievements_unlocked ?? 0}/{p.achievements_total}
                              </span>
                            )}
```

- [ ] **Step 3: Regenerate route tree, typecheck, knip**

Run (from `ui/frontend/`): `npm run build && npm run check && npm run knip`
Expected: build succeeds; no type errors; no knip findings. (`routeTree.gen.ts` should be unchanged here since no route was added — commit it only if it changed.)

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/games/\$id.index.tsx
git commit -m "feat: show per-storefront achievement progress on game detail (#1142)"
```

---

### Task 10: Full-suite verification + docs touch-up

**Files:**
- (Verification only; optionally `docs/sync.md` if it enumerates synced fields.)

- [ ] **Step 1: Run the Go suite for touched packages**

Run: `go test ./internal/services/steam/... ./internal/usergame/... ./internal/worker/... ./internal/api/...`
Expected: PASS.

- [ ] **Step 2: Run dead-code check (signature of `upsertPlatforms` changed)**

Run: `make deadcode`
Expected: no *new* entries attributable to this diff.

- [ ] **Step 3: Run the frontend gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all green.

- [ ] **Step 4: Optional docs**

If `docs/sync.md` lists per-game synced fields, add a line noting Steam achievement counts (unlocked/total) are now captured for played games. Skip if no such list exists.

- [ ] **Step 5: Final commit (if docs changed)**

```bash
git add docs/sync.md
git commit -m "docs: note Steam achievement counts in sync reference (#1142)"
```

---

## Self-Review

**Spec coverage:**
- Migration (both tables, nullable) → Task 1. ✅
- Steam single-call fetch, `success:false`→NULL, playtime gate, own rate limiter → Tasks 2–3. ✅
- `ExternalGameEntry` field + sync flow + index-0 attachment → Tasks 3, 5. ✅
- `usergame.Acquire` persistence (mutation boundary) → Task 4. ✅
- REST DTO → Task 6. ✅
- TS types + transform → Task 7. ✅
- Card badge, highest-progress-wins, hidden when none → Task 8. ✅
- Detail-view per-storefront line → Task 9. ✅
- Testing (count derivation, NULL-vs-false, playtime gate, plumbing, frontend selection) → Tasks 2,3,4,5,8. ✅

**Placeholder scan:** No TBD/TODO; every code step has concrete code. Test-helper names (`seedUserAndGame`, `seedExternalGame`, `makeGame`/`makePlatform`) are flagged as "match the neighbouring test" because the exact helper name must follow the existing file — the implementer confirms against the file rather than inventing one.

**Type consistency:** `*int` used end-to-end in Go (`models`, `PlatformInput`, `ExternalGameEntry`, DTO, `upsertPlatforms` params); `number | null` in TS. `bestAchievementProgress` signature identical in Task 8 interface block, implementation, and tests. `GetPlayerAchievements` return tuple `(int, int, bool, error)` consistent between Task 2 definition and Task 3 consumption.
