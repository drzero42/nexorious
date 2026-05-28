# Half-hour-granular playtime — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-05-28-half-hour-playtime-design.md`](../specs/2026-05-28-half-hour-playtime-design.md)
**Issue:** [#641](https://github.com/drzero42/nexorious/issues/641)
**Branch:** `feat/641-half-hour-playtime` (already created and contains the spec commit)

**Goal:** Capture Steam/PSN sub-hour playtime at sync time and render all recorded-playtime values to half-hour buckets below 10h and whole hours from 10h up, eliminating float-summation artifacts in the UI.

**Architecture:** Two coordinated changes — a single frontend formatter `formatHoursPlayed` applied at every recorded-playtime display site (rule: `<10h` → nearest 0.5, `≥10h` → nearest integer), and a `PlaytimeHours int → float64` promotion through the storefront adapter contract so Steam can do real fractional division and PSN can preserve minutes. Storage stays raw (`NUMERIC(10,2)` accommodates it today); the bucket is a display rule only.

**Tech Stack:** Go (Bun, testcontainers), React/TypeScript (Vitest), no schema changes.

**Task order rationale:** Frontend formatter first (independent, immediately user-visible against existing data). Then a single mechanical commit that promotes the adapter type end-to-end with no behavior change (so a `git bisect` between any two intermediate commits stays sane). Then the two behavioural changes (Steam fractional, PSN fractional) as TDD steps. Then the small remaining frontend tweak (manual entry input). Then final verification.

---

## Task 1: Frontend formatter and display swaps

**Files:**
- Create: `ui/frontend/src/lib/game-utils.test.ts`
- Modify: `ui/frontend/src/lib/game-utils.ts:1-3`
- Modify: `ui/frontend/src/components/games/game-card.tsx:149`
- Modify: `ui/frontend/src/components/games/game-list.tsx:180`
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx:395,410`
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx:385`
- Modify: `ui/frontend/src/components/dashboard/progress-statistics.tsx:109,180`

- [ ] **Step 1: Write the failing test for `formatHoursPlayed`**

Create `ui/frontend/src/lib/game-utils.test.ts`:

```ts
import { describe, expect, it } from 'vitest';

import { formatHoursPlayed } from './game-utils';

describe('formatHoursPlayed', () => {
  it.each([
    [0, '0h'],
    [null, '0h'],
    [undefined, '0h'],
    [1.2, '1h'],
    [1.3, '1.5h'],
    [7.4, '7.5h'],
    [9.8, '10h'], // boundary: bucket crosses 10
    [10, '10h'], // boundary: exactly 10 → integer rule
    [30.299999999999997, '30h'], // canonical float artifact from issue #641
    [134, '134h'],
  ])('formats %s as %s', (input, expected) => {
    expect(formatHoursPlayed(input as number | null | undefined)).toBe(expected);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd ui/frontend && npm run test -- game-utils.test.ts
```

Expected: FAIL with `formatHoursPlayed is not a function` (or similar — import resolution error).

- [ ] **Step 3: Implement `formatHoursPlayed`**

Edit `ui/frontend/src/lib/game-utils.ts`. The current file has only `formatTtb` and `formatIgdbRating`. Add the new helper after `formatTtb`:

```ts
export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}

export function formatHoursPlayed(hours: number | null | undefined): string {
  const h = hours ?? 0;
  // Half-hour buckets below 10h, whole hours from 10h up.
  const rounded = h < 10 ? Math.round(h * 2) / 2 : Math.round(h);
  return `${rounded}h`;
}

export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd ui/frontend && npm run test -- game-utils.test.ts
```

Expected: PASS, 10/10 cases.

- [ ] **Step 5: Apply formatter at `game-card.tsx:149`**

Add the import (the file already imports from `@/lib/game-utils` if it uses any of those helpers; otherwise add a new import). Replace the recorded-playtime span:

```tsx
// Before (line 149):
<span className="text-sm text-muted-foreground">{game.hours_played || 0}h</span>
// After:
<span className="text-sm text-muted-foreground">{formatHoursPlayed(game.hours_played)}</span>
```

Make sure `formatHoursPlayed` is imported from `@/lib/game-utils` at the top of the file.

- [ ] **Step 6: Apply formatter at `game-list.tsx:180`**

```tsx
// Before:
<span className="text-sm">{game.hours_played || 0}h</span>
// After:
<span className="text-sm">{formatHoursPlayed(game.hours_played)}</span>
```

Add the import from `@/lib/game-utils` if not present.

- [ ] **Step 7: Apply formatter at `$id.index.tsx:395` and `:410`**

```tsx
// Line 395 — before:
<dd className="mt-1 font-medium">{game.hours_played || 0}h</dd>
// After:
<dd className="mt-1 font-medium">{formatHoursPlayed(game.hours_played)}</dd>

// Line 410 — before:
<span>{p.hours_played}h</span>
// After:
<span>{formatHoursPlayed(p.hours_played)}</span>
```

Add the import.

- [ ] **Step 8: Apply formatter at `game-edit-form.tsx:385`**

```tsx
// Before:
<p className="text-lg font-medium">{totalHoursPlayed} hours total</p>
// After:
<p className="text-lg font-medium">{formatHoursPlayed(totalHoursPlayed)} total</p>
```

`totalHoursPlayed` is the local `useMemo` sum at line 140; it stays a `number`. Add the import.

- [ ] **Step 9: Apply formatter at `progress-statistics.tsx:109` and `:180`**

Both sites currently do `{totalHoursPlayed.toLocaleString()}`. Replace each with `{formatHoursPlayed(totalHoursPlayed)}`. The `> 0` guard on `:171` reads the raw number and is unchanged. Add the import.

- [ ] **Step 10: Run frontend typecheck and tests**

```bash
cd ui/frontend && npm run check && npm run test -- game-utils.test.ts
```

Expected: typecheck passes, formatter tests still pass. The pre-existing component tests that may reference `hours_played` text content (if any) will surface here — if any break, update the expected string to match the new bucket (e.g. `"30h"` instead of `"30.299999999999997h"`).

- [ ] **Step 11: Commit**

```bash
git add ui/frontend/src/lib/game-utils.ts ui/frontend/src/lib/game-utils.test.ts \
        ui/frontend/src/components/games/game-card.tsx \
        ui/frontend/src/components/games/game-list.tsx \
        ui/frontend/src/routes/_authenticated/games/\$id.index.tsx \
        ui/frontend/src/components/games/game-edit-form.tsx \
        ui/frontend/src/components/dashboard/progress-statistics.tsx
git commit -m "feat: half-hour-bucket recorded playtime display"
```

---

## Task 2: Promote PlaytimeHours to float64 end-to-end (no behavior change)

**Files (Go source):**
- Modify: `internal/services/storefrontadapter/storefrontadapter.go:12`
- Modify: `internal/services/steam/client.go:56,151`
- Modify: `internal/services/psn/client.go:217,338` (also: `:316` if relevant — the literal `0` keeps compiling)
- Modify: `internal/services/gog/library.go:18`
- Modify: `internal/worker/tasks/sync.go:101-103,105`

**Files (Go tests — format-verb updates only):**
- Modify: `internal/services/steam/client_test.go:44,47`
- Modify: `internal/services/psn/library_test.go:74,221,350`
- Modify: `internal/services/gog/library_test.go:180`
- Modify: `internal/services/psn/duration_test.go` (signature only — full rewrite happens in Task 4)

This commit changes types but preserves observable behavior. Steam still integer-truncates (via a temporary `float64(...)` wrap of the int division); PSN still drops minutes (via a `float64(parseDurationHours(...))` cast). Subsequent tasks unlock the actual fractional behavior.

- [ ] **Step 1: Promote the adapter contract field**

Edit `internal/services/storefrontadapter/storefrontadapter.go`:

```go
// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	PlaytimeHours   float64  // 0 when the storefront does not provide playtime; fractional when available (Steam, PSN)
	Platforms       []string // storefront-specific names; canonicalised to slugs by the worker
	OwnershipStatus string   // "owned", "subscription", etc.
	IsSubscription  bool
}
```

- [ ] **Step 2: Change `upsertPlatforms` signature**

Edit `internal/worker/tasks/sync.go:101-106`. The function and its single caller already exist; only types change.

```go
func upsertPlatforms(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64) {
	for i, platform := range platforms {
		hours := 0.0
		if i == 0 {
			hours = playtimeHours
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
			VALUES (?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
			uuid.NewString(), egID, platform, hours,
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: upsert platform failed", "err", err, "external_game_id", egID, "platform", platform)
		}
	}
}
```

(`hours := 0` → `hours := 0.0` so the variable is typed `float64`.) The caller at `:195` is unchanged — `e.PlaytimeHours` is now `float64`.

- [ ] **Step 3: Promote Steam's internal `OwnedGame.PlaytimeHours`**

Edit `internal/services/steam/client.go`. Find the `OwnedGame` struct (around line 56) and change the field type:

```go
type OwnedGame struct {
	AppID         int
	Title         string
	PlaytimeHours float64
}
```

Then at line 151, wrap the existing integer division in `float64(...)` so the assignment compiles without changing behavior:

```go
games = append(games, OwnedGame{
	AppID:         g.AppID,
	Title:         g.Name,
	PlaytimeHours: float64(g.PlaytimeForever / 60), // still int-truncating; Task 3 makes it fractional
})
```

The adapter forward at `steam/adapter.go:107` (`PlaytimeHours: og.PlaytimeHours`) needs no change — both sides are now `float64`.

- [ ] **Step 4: Promote PSN's internal struct**

Edit `internal/services/psn/client.go`. At line 338, the merge struct:

```go
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	Platforms       []string
	PlaytimeHours   float64
	OwnershipStatus string
	IsSubscription  bool
}
```

At line 217, wrap the parse in a `float64` cast (kept temporarily — Task 4 replaces the function):

```go
result[t.TitleID] = ExternalGameEntry{
	ExternalID:      t.TitleID,
	Title:           t.Name,
	Platforms:       []string{rawPlatform},
	PlaytimeHours:   float64(parseDurationHours(t.PlayDuration)),
	OwnershipStatus: ownership,
	IsSubscription:  isSub,
}
```

At line 316, the literal `PlaytimeHours: 0,` stays as-is — `0` is a valid untyped constant for `float64`. The adapter forward at `psn/adapter.go:29` is unchanged.

- [ ] **Step 5: Promote GOG's internal struct**

Edit `internal/services/gog/library.go`. Around line 18:

```go
// PlaytimeHours is always 0 — the GOG library API has no playtime field.
type GameEntry struct {
	// ... (existing fields)
	PlaytimeHours float64
	// ... (existing fields)
}
```

(Match the surrounding struct definition; only the type changes.) The literal `PlaytimeHours: 0,` at `:129` keeps compiling. Adapter forward at `gog/adapter.go:62` is unchanged. Epic adapter at `epic/adapter.go:74` (`PlaytimeHours: 0,`) is unchanged.

- [ ] **Step 6: Update test format verbs**

`%d` formats an integer; with `float64` it goes through `fmt.Errorf` and produces a runtime error (`%!d(float64=...)`). Switch every `%d` that prints a `PlaytimeHours` to `%v`.

Edit `internal/services/steam/client_test.go`:

```go
// Line 44:
t.Errorf("PlaytimeHours: got %v, want 2 (120 min / 60)", games[0].PlaytimeHours)
// Line 47:
t.Errorf("PlaytimeHours for 0-minute game: got %v, want 0", games[1].PlaytimeHours)
```

Edit `internal/services/psn/library_test.go`:

```go
// Line 74:
t.Errorf("expected PlaytimeHours=340, got %v", ps5.PlaytimeHours)
// Line 221:
t.Errorf("expected PlaytimeHours=0, got %v", ps4.PlaytimeHours)
// Line 350:
t.Errorf("expected playtime preserved from play history (10), got %v", e.PlaytimeHours)
```

Edit `internal/services/gog/library_test.go:180`:

```go
t.Errorf("PlaytimeHours should be 0, got %v", entries[0].PlaytimeHours)
```

`epic/adapter_test.go` already uses `%v` — leave it.

- [ ] **Step 7: Update PSN duration test signature only**

Edit `internal/services/psn/duration_test.go`. The full rewrite happens in Task 4; for now just promote the `want` field so the file still compiles against the (still `int`-returning) `parseDurationHours`. Actually, this file stays compilable as-is because `parseDurationHours` still returns `int` — leave it untouched in this task.

(No edit needed in this step. Kept here as a reminder: when Task 4 changes the return type, this file is rewritten.)

- [ ] **Step 8: Build and run all backend tests**

```bash
go build ./... && go test ./internal/...
```

Expected: build PASS, tests PASS. All existing assertions still hold because behavior is unchanged (Steam still int-truncates via the temporary cast; PSN still drops minutes via the temporary cast).

- [ ] **Step 9: Commit**

```bash
git add internal/services/storefrontadapter/storefrontadapter.go \
        internal/services/steam/client.go \
        internal/services/steam/client_test.go \
        internal/services/psn/client.go \
        internal/services/psn/library_test.go \
        internal/services/gog/library.go \
        internal/services/gog/library_test.go \
        internal/worker/tasks/sync.go
git commit -m "refactor: promote PlaytimeHours to float64 across adapter contract"
```

---

## Task 3: Steam fractional capture (TDD)

**Files:**
- Modify: `internal/services/steam/client_test.go:17-51` (the `TestGetOwnedGames_ParsesResponse` table)
- Modify: `internal/services/steam/client.go:151`

- [ ] **Step 1: Add fractional Steam test cases (RED)**

Edit `internal/services/steam/client_test.go`. Extend the games fixture and add assertions for sub-hour precision. Replace the body of `TestGetOwnedGames_ParsesResponse` so the fixture covers the new cases:

```go
func TestGetOwnedGames_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"games": []map[string]any{
					{"appid": 730, "name": "Counter-Strike 2", "playtime_forever": 120}, // 120 min → 2.0h
					{"appid": 440, "name": "Team Fortress 2", "playtime_forever": 0},    // 0 min → 0.0h
					{"appid": 570, "name": "Dota 2", "playtime_forever": 90},            // 90 min → 1.5h (sub-hour)
					{"appid": 620, "name": "Portal 2", "playtime_forever": 45},          // 45 min → 0.75h
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
	if len(games) != 4 {
		t.Fatalf("want 4 games, got %d", len(games))
	}
	if games[0].AppID != 730 {
		t.Errorf("AppID: got %d, want 730", games[0].AppID)
	}
	if games[0].Title != "Counter-Strike 2" {
		t.Errorf("Title: got %q", games[0].Title)
	}
	if games[0].PlaytimeHours != 2.0 {
		t.Errorf("PlaytimeHours: got %v, want 2.0 (120 min / 60)", games[0].PlaytimeHours)
	}
	if games[1].PlaytimeHours != 0.0 {
		t.Errorf("PlaytimeHours for 0-minute game: got %v, want 0.0", games[1].PlaytimeHours)
	}
	if games[2].PlaytimeHours != 1.5 {
		t.Errorf("PlaytimeHours for 90-min game: got %v, want 1.5 (sub-hour precision)", games[2].PlaytimeHours)
	}
	if games[3].PlaytimeHours != 0.75 {
		t.Errorf("PlaytimeHours for 45-min game: got %v, want 0.75", games[3].PlaytimeHours)
	}
}
```

(90/60 and 45/60 produce exact binary float64 values, so direct `==` comparison is safe — no delta needed.)

- [ ] **Step 2: Run Steam tests to verify the new cases FAIL**

```bash
go test ./internal/services/steam/... -run TestGetOwnedGames_ParsesResponse -v
```

Expected: FAIL — `games[2].PlaytimeHours` is `1` (from `float64(90/60)` = `float64(1)`), not `1.5`; `games[3]` is `0`, not `0.75`.

- [ ] **Step 3: Implement real float division**

Edit `internal/services/steam/client.go:151`. Replace the temporary `float64(g.PlaytimeForever / 60)` cast with real float division:

```go
games = append(games, OwnedGame{
	AppID:         g.AppID,
	Title:         g.Name,
	PlaytimeHours: float64(g.PlaytimeForever) / 60.0,
})
```

- [ ] **Step 4: Run Steam tests to verify they PASS**

```bash
go test ./internal/services/steam/... -run TestGetOwnedGames_ParsesResponse -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/steam/client.go internal/services/steam/client_test.go
git commit -m "feat: capture Steam sub-hour playtime"
```

---

## Task 4: PSN fractional capture (TDD)

**Files:**
- Modify: `internal/services/psn/duration_test.go` (full rewrite)
- Modify: `internal/services/psn/duration.go` (rename + reimplement)
- Modify: `internal/services/psn/client.go:217`
- Modify: `internal/services/psn/library_test.go:73-75`

- [ ] **Step 1: Rewrite duration test for fractional output (RED)**

Replace the contents of `internal/services/psn/duration_test.go`:

```go
package psn

import (
	"math"
	"testing"
)

func TestParseDurationFractionalHours(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"PT0S", 0},
		{"PT0H", 0},
		{"PT1H", 1},
		{"PT30M", 0.5},
		{"PT2H30M", 2.5},
		{"PT1H59M", 1 + 59.0/60.0},
		{"PT340H46M13S", 340 + 46.0/60.0}, // seconds dropped — minutes-only resolution
		{"PT2H0M0S", 2},
		{"PT99H59M59S", 99 + 59.0/60.0},
		{"", 0},
		{"invalid", 0},
		{"P1DT2H", 0}, // days component not supported
	}
	for _, tc := range cases {
		got := parseDurationFractionalHours(tc.input)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("parseDurationFractionalHours(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run duration test to verify it FAILS**

```bash
go test ./internal/services/psn/... -run TestParseDurationFractionalHours -v
```

Expected: compile error — `parseDurationFractionalHours` is undefined.

- [ ] **Step 3: Rename and reimplement the parser**

Replace `internal/services/psn/duration.go`:

```go
package psn

import (
	"regexp"
	"strconv"
)

// durationRE matches ISO 8601 durations of the form PTxHxMxS.
// Days and larger units are not produced by the Sony API and are not supported.
var durationRE = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:\d+S)?$`)

// parseDurationFractionalHours parses an ISO 8601 duration string such as
// "PT340H46M13S" and returns hours as H + M/60. Seconds are intentionally
// dropped — the display layer buckets to half-hours, so second-level
// precision is invisible end-to-end. Returns 0 for unrecognised input.
func parseDurationFractionalHours(s string) float64 {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])    //nolint:errcheck // optional hours group; empty match yields 0
	mins, _ := strconv.Atoi(m[2]) //nolint:errcheck // optional minutes group; empty match yields 0
	return float64(h) + float64(mins)/60.0
}
```

(The old `parseDurationHours` function is removed; the regex is preserved verbatim.)

- [ ] **Step 4: Update the PSN client caller**

Edit `internal/services/psn/client.go:217`. Replace the temporary `float64(...)` cast with a direct call to the new function:

```go
result[t.TitleID] = ExternalGameEntry{
	ExternalID:      t.TitleID,
	Title:           t.Name,
	Platforms:       []string{rawPlatform},
	PlaytimeHours:   parseDurationFractionalHours(t.PlayDuration),
	OwnershipStatus: ownership,
	IsSubscription:  isSub,
}
```

- [ ] **Step 5: Run duration test to verify it PASSES**

```bash
go test ./internal/services/psn/... -run TestParseDurationFractionalHours -v
```

Expected: PASS.

- [ ] **Step 6: Update the PSN library test expectation**

Edit `internal/services/psn/library_test.go:73-75`. The fixture `PT340H46M13S` now yields `340 + 46/60`, not `340`. Switch to a delta comparison:

```go
// Before:
if ps5.PlaytimeHours != 340 {
    t.Errorf("expected PlaytimeHours=340, got %v", ps5.PlaytimeHours)
}
// After:
const wantPS5 = 340 + 46.0/60.0
if math.Abs(ps5.PlaytimeHours-wantPS5) > 1e-9 {
    t.Errorf("expected PlaytimeHours=%v, got %v", wantPS5, ps5.PlaytimeHours)
}
```

Add `"math"` to the file's import block if not already present (it likely isn't).

- [ ] **Step 7: Run the full PSN package tests**

```bash
go test ./internal/services/psn/... -v
```

Expected: PASS. The `:221` and `:350` assertions still work — they compare against `0` and `10`, which the test fixtures (a 0-playtime case and a `PT10H` literal in the merge test) still satisfy exactly.

- [ ] **Step 8: Commit**

```bash
git add internal/services/psn/duration.go internal/services/psn/duration_test.go \
        internal/services/psn/client.go internal/services/psn/library_test.go
git commit -m "feat: preserve PSN sub-hour playtime to minute resolution"
```

---

## Task 5: Fractional upsert guard test

**Files:**
- Modify: `internal/worker/tasks/sync_test.go` (append after `TestDispatchSync_Steam_PlaytimeStoredOnPlatform`)

This task adds a single test that proves the fractional value survives the worker → DB roundtrip. The DB column is already `NUMERIC(10,2)`, so the test is a regression guard — it would fail loudly if anyone later truncated `playtimeHours` to int inside `upsertPlatforms`.

- [ ] **Step 1: Add the fractional upsert test**

Open `internal/worker/tasks/sync_test.go`. Find `TestDispatchSync_Steam_PlaytimeStoredOnPlatform` (around line 600). Add a sibling test directly after it. Use the same pattern as the existing test — the fixture from line 600 is the model.

```go
func TestDispatchSync_Steam_FractionalPlaytimeStoredOnPlatform(t *testing.T) {
	// Regression guard: PlaytimeHours=1.5 (the canonical sub-hour case from Steam)
	// must reach external_game_platforms.hours_played intact. NUMERIC(10,2) preserves
	// it; this test fails if anyone later truncates to int inside upsertPlatforms.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 1.5, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hours float64
	if err := testDB.NewRaw(`
		SELECT egp.hours_played
		FROM external_game_platforms egp
		JOIN external_games eg ON eg.id = egp.external_game_id
		WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '570'`,
		userID,
	).Scan(ctx, &hours); err != nil {
		t.Fatalf("scan hours_played: %v", err)
	}
	if hours != 1.5 {
		t.Errorf("hours_played: want 1.5, got %v", hours)
	}
}
```

- [ ] **Step 2: Run the new test**

```bash
go test ./internal/worker/tasks/... -run TestDispatchSync_Steam_FractionalPlaytimeStoredOnPlatform -v
```

Expected: PASS. (This is a guard, so the first run should succeed — `upsertPlatforms` already takes `float64` and writes it to a `NUMERIC(10,2)` column.)

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/sync_test.go
git commit -m "test: guard fractional playtime through upsertPlatforms"
```

---

## Task 6: Manual edit form accepts half-hour values

**Files:**
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx:481-489`

- [ ] **Step 1: Switch to `parseFloat` and `step="0.5"`**

Edit the `<Input>` block that captures per-platform hours played. The current block:

```tsx
<Input
  type="number"
  min="0"
  className="h-9 w-24"
  value={platformPlaytimes[p.id] ?? p.hours_played}
  onChange={(e) =>
    setPlatformPlaytimes((prev) => ({
      ...prev,
      [p.id]: parseInt(e.target.value) || 0,
    }))
  }
  disabled={isSteamSynced}
/>
```

Becomes:

```tsx
<Input
  type="number"
  min="0"
  step="0.5"
  className="h-9 w-24"
  value={platformPlaytimes[p.id] ?? p.hours_played}
  onChange={(e) =>
    setPlatformPlaytimes((prev) => ({
      ...prev,
      [p.id]: parseFloat(e.target.value) || 0,
    }))
  }
  disabled={isSteamSynced}
/>
```

Only two changes: add `step="0.5"`, swap `parseInt` for `parseFloat`. The state type stays `Record<string, number>`; `totalHoursPlayed` (Task 1, line 140) keeps summing the same numeric type.

- [ ] **Step 2: Run frontend typecheck**

```bash
cd ui/frontend && npm run check
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/games/game-edit-form.tsx
git commit -m "feat: accept half-hour values in manual playtime edit"
```

---

## Task 7: Full verification and PR

- [ ] **Step 1: Run the complete backend test suite**

```bash
go test -timeout 600s ./...
```

Expected: PASS.

- [ ] **Step 2: Run lint**

```bash
golangci-lint run
```

Expected: zero findings. (`errcheck` with `check-blank` is on — any new `_ =` would be flagged.)

- [ ] **Step 3: Run the complete frontend gates**

```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```

Expected: PASS, zero knip findings.

- [ ] **Step 4: Push the branch**

```bash
git push -u origin feat/641-half-hour-playtime
```

(The pre-push hook runs the same suites; this is a safety net.)

- [ ] **Step 5: Open the PR**

```bash
gh pr create --base main \
  --title "feat: half-hour-granular playtime — capture Steam/PSN sub-hour precision and clean up hours display" \
  --body "$(cat <<'EOF'
Closes #641.

## What changes

- **Display.** New `formatHoursPlayed` helper (`ui/frontend/src/lib/game-utils.ts`) buckets recorded playtime: nearest 0.5 below 10h, nearest integer from 10h up. Applied at every recorded-playtime site (game card, list, detail page, edit-form total, two dashboard totals). No more `30.299999999999997h` artifacts.
- **Capture.** `ExternalGameEntry.PlaytimeHours` promoted from `int` to `float64`. Steam stops integer-truncating (`90 min → 1.5h`); PSN keeps minutes (`PT340H46M13S → 340.77h`). GOG and Epic stay at `0.0` — no upstream source.
- **Manual entry.** Per-platform hours input accepts half-hour steps (`step="0.5"`, `parseFloat`).

## What does not change

- Averages keep their `.toFixed(1)` calls (averages are inherently fractional).
- No DB migration — `user_game_platforms.hours_played` is already `NUMERIC(10,2)`.
- No API contract change — JSON serialises `float64` and `int` identically for our values.

## Tests

- New `formatHoursPlayed` unit test covering the boundary cases from the issue (`9.8 → 10h`, `7.4 → 7.5h`, `30.299999999999997 → 30h`, null/undefined → `0h`).
- Steam adapter gains `90 min → 1.5h` and `45 min → 0.75h` cases.
- New `parseDurationFractionalHours` test pinning the minute-resolution rule and the dropped-seconds policy.
- Worker upsert guard pinning that `1.5` reaches `external_game_platforms.hours_played` intact.

## Spec

[`docs/superpowers/specs/2026-05-28-half-hour-playtime-design.md`](docs/superpowers/specs/2026-05-28-half-hour-playtime-design.md)
EOF
)"
```

- [ ] **Step 6: Confirm CI green**

Watch the PR checks. If any fail, investigate; do not merge.

---

## Self-review notes

Cross-checked the plan against the spec section by section:

- **Display rule and 7 application sites** — Task 1, steps 5–9. Boundary cases match the spec's testing table verbatim.
- **`PlaytimeHours int → float64` promotion** — Task 2 covers every file the spec's "Files touched" tables list (adapter contract, worker, Steam internal, PSN internal, GOG internal). Adapter forwards and `epic/adapter.go:74` correctly noted as no-edit.
- **Format-verb updates (`%d → %v`)** — Task 2 step 6. Each file:line from the spec's "files where the type ripple is invisible" detail covered.
- **Steam fractional division** — Task 3 step 3 matches the spec exactly.
- **PSN minutes-only fractional duration** — Task 4. Function rename, signature, no-seconds policy, and library-test delta comparison all match.
- **Sync fractional guard** — Task 5 matches the spec's "Add one new case to the existing upsert table" requirement; given the existing sync test pattern uses a full Work cycle rather than a small table, I write it as a sibling test (still inside the existing file).
- **Manual edit form** — Task 6 matches the spec's `parseFloat + step="0.5"` requirement.
- **Out-of-scope items** — none of the averages, sort SQL, #639 work, or schema migrations appear anywhere in the plan. ✓

No placeholders found. Type names consistent across tasks (`formatHoursPlayed`, `parseDurationFractionalHours`, `playtimeHours` parameter, `PlaytimeHours` field).
