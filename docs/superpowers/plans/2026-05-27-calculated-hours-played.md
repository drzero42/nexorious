# Calculated game-level `hours_played` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `hours_played` on a user-game a calculated sum of its `user_game_platforms.hours_played` rows — returned by the API on every user-game response and usable as a list sort field — and prove with tests that no stored game-level value exists or is written.

**Architecture:** The API computes the game-level sum in Go from the already-eager-loaded platforms (mirroring `export.go`), so a single response builder (`toUserGameWithPlatformsResponse`) covers all endpoints. Sorting reuses the existing two-phase list-query pattern: a pre-aggregated `user_game_platforms` subquery is `LEFT JOIN`ed under a stable alias (`hp`) in both phases, and the order key is `COALESCE(hp.total, 0)` so platformless games sort as 0 rather than `NULL`.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM (`uptrace/bun`), PostgreSQL (testcontainers), React 19 + TypeScript + Vitest.

**Spec:** `docs/superpowers/specs/2026-05-27-hours-played-calculated-design.md`
**Branch:** `fix/613-calculated-hours-played` (already created; spec already committed)

---

## File Structure

**Backend (Go)**
- Modify `internal/api/user_games.go`:
  - `userGameWithPlatformsResponse` struct + `toUserGameWithPlatformsResponse` — add calculated `HoursPlayed` (Task 1).
  - `allowedUserGameSortFields`, add `sortFieldsRequiringHoursJoin`, and the two query phases in `HandleListUserGames` — enable `sort_by=hours_played` (Task 2).
- Modify `internal/api/user_games_test.go`:
  - New `TestUserGameCalculatedHours` (Task 1).
  - New `TestListUserGamesSortByHours` (Task 2).
  - New `TestUserGamesNoStoredHoursColumn` and `TestManualPlatformHoursReflectedInSum` (Task 3).

**Frontend (TypeScript)**
- Modify `ui/frontend/src/components/games/game-edit-form.tsx` — simplify the `totalHoursPlayed` memo to drop the now-redundant `game.hours_played` fallback (Task 4).

No new files, no DB migration, no new route, no `slumber.yaml` change.

---

## Task 1: API returns calculated game-level `hours_played`

**Files:**
- Modify: `internal/api/user_games.go:100-115` (`userGameWithPlatformsResponse` + `toUserGameWithPlatformsResponse`)
- Test: `internal/api/user_games_test.go` (append a new test function)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/user_games_test.go`:

```go
func TestUserGameCalculatedHours(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "calchours")

	// Two platforms on one game: 10 + 25.5 = 35.5
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC'),('ps5','PS5') ON CONFLICT DO NOTHING`)
	g1 := insertTestGame(t, testDB, "Calc Hours Game")
	insertTestUserGame(t, testDB, "ug-calc-1", userID, int(g1))
	pc, ps5 := "pc", "ps5"
	insertTestUserGamePlatform(t, testDB, "ugp-calc-1", "ug-calc-1", &pc, nil)
	insertTestUserGamePlatform(t, testDB, "ugp-calc-2", "ug-calc-1", &ps5, nil)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 10 WHERE id = 'ugp-calc-1'`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 25.5 WHERE id = 'ugp-calc-2'`)

	// A second game with no platform hours → 0
	g2 := insertTestGame(t, testDB, "No Hours Game")
	insertTestUserGame(t, testDB, "ug-calc-2", userID, int(g2))

	t.Run("single GET returns summed hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-calc-1", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["hours_played"].(float64) != 35.5 {
			t.Fatalf("expected hours_played=35.5, got %v", resp["hours_played"])
		}
	})

	t.Run("single GET returns 0 when no platform hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-calc-2", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["hours_played"].(float64) != 0 {
			t.Fatalf("expected hours_played=0, got %v", resp["hours_played"])
		}
	})

	t.Run("list returns summed hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		var calc1 map[string]any
		for _, g := range games {
			gm := g.(map[string]any)
			if gm["id"] == "ug-calc-1" {
				calc1 = gm
			}
		}
		if calc1 == nil {
			t.Fatal("ug-calc-1 not found in list response")
		}
		if calc1["hours_played"].(float64) != 35.5 {
			t.Fatalf("expected list hours_played=35.5, got %v", calc1["hours_played"])
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestUserGameCalculatedHours -v`
Expected: FAIL — `resp["hours_played"]` is `nil` (the key is absent), so the `.(float64)` type assertion panics / the value is not 35.5.

- [ ] **Step 3: Add the calculated field to the response DTO and builder**

In `internal/api/user_games.go`, replace the struct and builder (currently lines ~100-115):

```go
// userGameWithPlatformsResponse wraps UserGame but serialises Platforms as DTOs with
// nested details and exposes a calculated game-level HoursPlayed (sum of platform hours).
type userGameWithPlatformsResponse struct {
	models.UserGame
	HoursPlayed float64                    `json:"hours_played"`
	Platforms   []userGamePlatformResponse `json:"platforms"`
}

func toUserGameWithPlatformsResponse(ug models.UserGame) userGameWithPlatformsResponse {
	resp := userGameWithPlatformsResponse{UserGame: ug}
	var totalHours float64
	for _, p := range ug.Platforms {
		if p.HoursPlayed != nil {
			totalHours += *p.HoursPlayed
		}
		resp.Platforms = append(resp.Platforms, toUserGamePlatformResponse(p))
	}
	resp.HoursPlayed = totalHours
	if resp.Platforms == nil {
		resp.Platforms = []userGamePlatformResponse{}
	}
	return resp
}
```

(`models.UserGame` has no `HoursPlayed` field, so there is no JSON key collision; the wrapper's `hours_played` is promoted at the top level, exactly as `platforms` already is.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestUserGameCalculatedHours -v`
Expected: PASS (all three sub-tests).

- [ ] **Step 5: Run the existing user-games tests to check for regressions**

Run: `go test ./internal/api/... -run 'TestListUserGames|TestGetUserGame|TestCreateUserGame|TestUpdateProgress' -v`
Expected: PASS (the new field is additive; existing assertions don't inspect `hours_played`).

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "fix: return calculated game-level hours_played on user-game responses"
```

---

## Task 2: Support `sort_by=hours_played` on the user-games list

**Files:**
- Modify: `internal/api/user_games.go:126-139` (sort whitelists), `:209-212` (phase-1 join), `:290-298` (phase-2 join)
- Test: `internal/api/user_games_test.go` (append a new test function)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/user_games_test.go`:

```go
func TestListUserGamesSortByHours(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "sorthours")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC') ON CONFLICT DO NOTHING`)
	pc := "pc"

	gLow := insertTestGame(t, testDB, "Low Hours")
	gHigh := insertTestGame(t, testDB, "High Hours")
	gZero := insertTestGame(t, testDB, "Zero Hours")
	insertTestUserGame(t, testDB, "ug-sh-low", userID, int(gLow))
	insertTestUserGame(t, testDB, "ug-sh-high", userID, int(gHigh))
	insertTestUserGame(t, testDB, "ug-sh-zero", userID, int(gZero)) // no platforms → 0

	insertTestUserGamePlatform(t, testDB, "ugp-sh-low", "ug-sh-low", &pc, nil)
	insertTestUserGamePlatform(t, testDB, "ugp-sh-high", "ug-sh-high", &pc, nil)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 5 WHERE id = 'ugp-sh-low'`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 100 WHERE id = 'ugp-sh-high'`)

	idsInOrder := func(t *testing.T, order string) []string {
		t.Helper()
		rec := getAuth(t, e, "/api/user-games?sort_by=hours_played&sort_order="+order, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		ids := make([]string, len(games))
		for i, g := range games {
			ids[i] = g.(map[string]any)["id"].(string)
		}
		return ids
	}

	t.Run("desc orders highest hours first, zero last", func(t *testing.T) {
		ids := idsInOrder(t, "desc")
		want := []string{"ug-sh-high", "ug-sh-low", "ug-sh-zero"}
		for i := range want {
			if ids[i] != want[i] {
				t.Fatalf("desc order mismatch: got %v, want %v", ids, want)
			}
		}
	})

	t.Run("asc orders zero first, highest last", func(t *testing.T) {
		ids := idsInOrder(t, "asc")
		want := []string{"ug-sh-zero", "ug-sh-low", "ug-sh-high"}
		for i := range want {
			if ids[i] != want[i] {
				t.Fatalf("asc order mismatch: got %v, want %v", ids, want)
			}
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestListUserGamesSortByHours -v`
Expected: FAIL — the request returns HTTP 400 (`invalid sort_by field`), so `rec.Code` is 400 and the test fails at the `expected 200` check.

- [ ] **Step 3: Add the sort whitelist entry and the hours-join set**

In `internal/api/user_games.go`, update the two maps (currently lines ~126-139). Add the `hours_played` entry and a new `sortFieldsRequiringHoursJoin` set:

```go
var allowedUserGameSortFields = map[string]string{
	"title":           "g.title",
	"created_at":      "ug.created_at",
	"updated_at":      "ug.updated_at",
	"play_status":     "ug.play_status",
	"personal_rating": "ug.personal_rating",
	"is_loved":        "ug.is_loved",
	"release_date":    "g.release_date",
	// hours_played sorts on the joined aggregate alias `hp`; COALESCE so games with no
	// platforms (LEFT JOIN → NULL) sort as 0 instead of NULL-first under DESC.
	"hours_played": "COALESCE(hp.total, 0)",
}

var sortFieldsRequiringGamesJoin = map[string]bool{
	"title":        true,
	"release_date": true,
}

var sortFieldsRequiringHoursJoin = map[string]bool{
	"hours_played": true,
}
```

- [ ] **Step 4: Add the aggregate join in phase 1 (ID query)**

In `HandleListUserGames`, find the existing games-join block (currently ~lines 209-212):

```go
	// If sort field needs games join, add it.
	if sortBy != "" && sortFieldsRequiringGamesJoin[sortBy] {
		fb.AddJoin("g", "LEFT JOIN games AS g ON g.id = ug.game_id")
	}
```

Add immediately after it:

```go
	// If sort field needs the aggregated platform-hours join, add it.
	if sortBy != "" && sortFieldsRequiringHoursJoin[sortBy] {
		fb.AddJoin("hp", "LEFT JOIN (SELECT user_game_id, COALESCE(SUM(hours_played), 0) AS total FROM user_game_platforms GROUP BY user_game_id) hp ON hp.user_game_id = ug.id")
	}
```

(The aggregate subquery yields at most one row per `user_game_id`, so it is safe under the `DISTINCT` in the ID query and cannot multiply rows.)

- [ ] **Step 5: Add the aggregate join in phase 2 (model fetch)**

Find the phase-2 re-apply block (currently ~lines 290-298):

```go
	// Re-apply sort on the Model query.
	if sortCol != "" {
		// For game-table sorts, join games again on the model query.
		if sortFieldsRequiringGamesJoin[sortBy] {
			q = q.Join("LEFT JOIN games AS g ON g.id = user_game.game_id")
		}
		q = q.OrderExpr(sortCol + " " + sortOrder)
	}
```

Add the hours-join branch alongside the games-join branch:

```go
	// Re-apply sort on the Model query.
	if sortCol != "" {
		// For game-table sorts, join games again on the model query.
		if sortFieldsRequiringGamesJoin[sortBy] {
			q = q.Join("LEFT JOIN games AS g ON g.id = user_game.game_id")
		}
		// For the hours sort, join the same aggregated subquery (alias hp).
		if sortFieldsRequiringHoursJoin[sortBy] {
			q = q.Join("LEFT JOIN (SELECT user_game_id, COALESCE(SUM(hours_played), 0) AS total FROM user_game_platforms GROUP BY user_game_id) hp ON hp.user_game_id = user_game.id")
		}
		q = q.OrderExpr(sortCol + " " + sortOrder)
	}
```

(The `ORDER BY COALESCE(hp.total, 0)` expression is identical in both phases — the alias `hp` belongs to the join, independent of the base-table alias `ug` vs `user_game` — so the existing `sortCol + " " + sortOrder` machinery needs no further change.)

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestListUserGamesSortByHours -v`
Expected: PASS (both `desc` and `asc` sub-tests).

- [ ] **Step 7: Run the existing list tests to check for regressions**

Run: `go test ./internal/api/... -run TestListUserGames -v`
Expected: PASS — including the existing `sort by title` and `invalid sort field` sub-tests (the `hacked` field is still rejected with 400).

- [ ] **Step 8: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "fix: support sort_by=hours_played on the user-games list"
```

---

## Task 3: Verification — no stored column, and manual entry feeds the sum

**Files:**
- Test only: `internal/api/user_games_test.go` (append two new test functions)

These prove the issue's invariants: there is no stored `user_games.hours_played` column, and manually-entered platform hours (via the existing platform-update endpoint) are reflected in the calculated game-level value. No production code changes — they pass once Task 1 is in place.

- [ ] **Step 1: Write the verification tests**

Append to `internal/api/user_games_test.go`:

```go
func TestUserGamesNoStoredHoursColumn(t *testing.T) {
	truncateAllTables(t)
	var count int
	err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_name = 'user_games' AND column_name = 'hours_played'`).Scan(&count)
	if err != nil {
		t.Fatalf("information_schema query: %v", err)
	}
	if count != 0 {
		t.Fatalf("user_games.hours_played must not be a stored column; found %d", count)
	}
}

func TestManualPlatformHoursReflectedInSum(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "manualhours")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC') ON CONFLICT DO NOTHING`)
	g := insertTestGame(t, testDB, "Manual Hours Game")
	insertTestUserGame(t, testDB, "ug-mh-1", userID, int(g))
	pc := "pc"
	insertTestUserGamePlatform(t, testDB, "ugp-mh-1", "ug-mh-1", &pc, nil)

	// Manually set hours via the platform-update endpoint.
	rec := putJSONAuth(t, e, "/api/user-games/ug-mh-1/platforms/ugp-mh-1", map[string]any{
		"hours_played": 42.5,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from platform update, got %d: %s", rec.Code, rec.Body.String())
	}

	// The calculated game-level value reflects the manual entry — proving it is derived,
	// not stored.
	rec = getAuth(t, e, "/api/user-games/ug-mh-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["hours_played"].(float64) != 42.5 {
		t.Fatalf("expected calculated hours_played=42.5, got %v", resp["hours_played"])
	}
}
```

- [ ] **Step 2: Run the tests to verify they pass**

Run: `go test ./internal/api/... -run 'TestUserGamesNoStoredHoursColumn|TestManualPlatformHoursReflectedInSum' -v`
Expected: PASS. (If `TestManualPlatformHoursReflectedInSum` fails because `hours_played` is absent, Task 1 was not applied — fix that first.)

- [ ] **Step 3: Commit**

```bash
git add internal/api/user_games_test.go
git commit -m "test: verify hours_played is calculated, not stored, and reflects manual entry"
```

---

## Task 4: Frontend — simplify the redundant edit-form fallback

**Files:**
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx:138-142` (`totalHoursPlayed` memo)
- Existing test (must stay green): `ui/frontend/src/components/games/game-edit-form.test.tsx:133-138`

Now that the API returns `game.hours_played` as the platform sum, the edit form's
`platformHours > 0 ? platformHours : game.hours_played` fallback is redundant: the form
already initialises `platformPlaytimes` from each platform's hours (line ~79-81), so the
live reduce already equals `game.hours_played`. Drop the fallback so the displayed total
purely reflects the (possibly edited) per-platform inputs.

- [ ] **Step 1: Confirm the existing test pins current behavior**

Run (from `ui/frontend/`): `npm run test -- game-edit-form.test.tsx`
Expected: PASS — including `renders hours played summary` which asserts `'10 hours total'` (mock platform hours = 10).

- [ ] **Step 2: Simplify the memo**

In `ui/frontend/src/components/games/game-edit-form.tsx`, replace the memo (currently lines ~138-142):

```tsx
  // Compute total hours from platform playtimes
  const totalHoursPlayed = useMemo(() => {
    const platformHours = Object.values(platformPlaytimes).reduce((sum, h) => sum + h, 0);
    return platformHours > 0 ? platformHours : game.hours_played;
  }, [platformPlaytimes, game.hours_played]);
```

with:

```tsx
  // Total hours = sum of the per-platform playtime inputs (the calculated value the API
  // now returns as game.hours_played, kept live so in-progress edits are reflected).
  const totalHoursPlayed = useMemo(
    () => Object.values(platformPlaytimes).reduce((sum, h) => sum + h, 0),
    [platformPlaytimes],
  );
```

- [ ] **Step 3: Run the edit-form test to verify it still passes**

Run (from `ui/frontend/`): `npm run test -- game-edit-form.test.tsx`
Expected: PASS — the mock platform's `hours_played` is 10, so the reduce yields 10 and `'10 hours total'` still renders.

- [ ] **Step 4: Run the frontend quality gates**

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: zero TypeScript/lint errors and zero knip findings. (`game.hours_played` is still referenced by `game-card.tsx`, `game-list.tsx`, and the detail route, so the `UserGame.hours_played` type stays in use — no knip removal.)

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/games/game-edit-form.tsx
git commit -m "refactor: drop redundant game.hours_played fallback in edit form"
```

---

## Final verification

- [ ] **Run the full Go API package tests**

Run: `go test ./internal/api/... -v`
Expected: PASS, including all four new tests and the pre-existing suite.

- [ ] **Run the full frontend suite**

Run (from `ui/frontend/`): `npm run test`
Expected: PASS — `game-card`, `game-list`, `game-edit-form`, and `$id.index` hours assertions all green.

- [ ] **Push (triggers the pre-push hard gate: full `go test` + frontend `check`/`knip`/`test`)**

```bash
git push -u origin fix/613-calculated-hours-played
```

When opening the PR, title it with **`fix:`** (per spec §9) so release-please cuts a patch
release — e.g. `fix: calculate user-game hours_played from platform hours (#613)`. Reference
issue #613 in the body. (#639 is tracked separately for the `howlongtobeat_main` /
`rating_average` sorts and is out of scope here.)

---

## Spec coverage check

| Spec requirement | Task |
|---|---|
| Verify no stored `user_games.hours_played` column | Task 3 (`TestUserGamesNoStoredHoursColumn`) |
| Verify sync writes hours only to platform tables | Covered transitively by Task 3 `TestManualPlatformHoursReflectedInSum` + the no-column invariant (the value is derived); sync code is unchanged |
| API returns calculated game-level `hours_played` on all responses | Task 1 (single builder used by list/GET/create/update/progress) |
| API supports `sort_by=hours_played` | Task 2 |
| Frontend consumes the value; remove redundant edit-form fallback | Task 4 (displays already read `game.hours_played`; fallback removed) |
| Verify manual per-platform entry reflected in the sum | Task 3 (`TestManualPlatformHoursReflectedInSum`) |
| Write side unchanged (synced locked) | No task touches the write path — invariant preserved by omission |
| `howlongtobeat_main` / `rating_average` sorts | Out of scope — issue #639 |
