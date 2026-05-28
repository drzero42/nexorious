# Issue #639 — Sort Whitelist Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the `howlongtobeat_main` ("Time to Beat") and `rating_average` ("IGDB Rating") sort options on `GET /api/user-games` so the frontend dropdown no longer 400s, with NULLs sinking to the bottom regardless of sort direction.

**Architecture:** Backend-only patch to `internal/api/user_games.go`. Two new entries in the existing sort whitelist + games-join flag, plus a new `sortFieldsNullsLast` map and a one-line `orderExpr` builder reused in both phases of the two-phase list query. Frontend already sends the correct values; no UI work.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM, PostgreSQL, `testing` + testcontainers.

**Spec:** `docs/superpowers/specs/2026-05-28-issue-639-sort-whitelist-design.md`

---

## File Structure

- **Modify:** `internal/api/user_games.go`
  - Extend `allowedUserGameSortFields` (`:133-144`) and `sortFieldsRequiringGamesJoin` (`:146-149`).
  - Add new `sortFieldsNullsLast` map below them.
  - Build a single `orderExpr` value where `sortCol` and `sortOrder` are resolved (`:178-189`).
  - Replace inline `sortCol + " " + sortOrder` at `:270` and `:318` with `orderExpr`.
- **Modify:** `internal/api/user_games_test.go`
  - Append `TestListUserGamesSortByGameNumerics` after the existing `TestListUserGamesSortByHours` (currently `:1207–1266`).

No other files touched. No schema change. No frontend change.

---

## Task 1: Add the failing regression test

**Files:**
- Test: `internal/api/user_games_test.go` (append new test function)

- [ ] **Step 1: Append the new test function to `internal/api/user_games_test.go`**

Add this function after `TestListUserGamesSortByHours` (the current last test in the sort family). Insert it just before `TestUserGameCalculatedHours` so the sort-related tests stay grouped:

```go
// TestListUserGamesSortByGameNumerics is the regression guard for issue #639:
// sort_by=howlongtobeat_main and sort_by=rating_average must return 200 (not
// the prior 400) and order results correctly with NULLs sinking to the bottom
// in both directions.
func TestListUserGamesSortByGameNumerics(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "sortnumerics")

	gLow := insertTestGame(t, testDB, "Low Values")
	gHigh := insertTestGame(t, testDB, "High Values")
	gNull := insertTestGame(t, testDB, "Null Values")

	insertTestUserGame(t, testDB, "ug-low", userID, int(gLow))
	insertTestUserGame(t, testDB, "ug-high", userID, int(gHigh))
	insertTestUserGame(t, testDB, "ug-null", userID, int(gNull))

	// Set both columns with parallel low/high mapping on the same fixture so
	// rating_average and howlongtobeat_main produce the same expected ordering.
	// ug-null is left with both columns NULL (the default).
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE games SET rating_average = 50, howlongtobeat_main = 10 WHERE id = ?`, gLow); err != nil {
		t.Fatalf("update gLow: %v", err)
	}
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE games SET rating_average = 90, howlongtobeat_main = 100 WHERE id = ?`, gHigh); err != nil {
		t.Fatalf("update gHigh: %v", err)
	}

	idsInOrder := func(t *testing.T, field, order string) []string {
		t.Helper()
		rec := getAuth(t, e, "/api/user-games?sort_by="+field+"&sort_order="+order, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		games := resp["user_games"].([]any)
		ids := make([]string, len(games))
		for i, g := range games {
			ids[i] = g.(map[string]any)["id"].(string)
		}
		return ids
	}

	for _, field := range []string{"rating_average", "howlongtobeat_main"} {
		field := field // capture
		t.Run(field+" desc orders high, low, null", func(t *testing.T) {
			ids := idsInOrder(t, field, "desc")
			want := []string{"ug-high", "ug-low", "ug-null"}
			if len(ids) != len(want) {
				t.Fatalf("got %d ids, want %d: %v", len(ids), len(want), ids)
			}
			for i := range want {
				if ids[i] != want[i] {
					t.Fatalf("desc order mismatch: got %v, want %v", ids, want)
				}
			}
		})
		t.Run(field+" asc orders low, high, null", func(t *testing.T) {
			ids := idsInOrder(t, field, "asc")
			want := []string{"ug-low", "ug-high", "ug-null"}
			if len(ids) != len(want) {
				t.Fatalf("got %d ids, want %d: %v", len(ids), len(want), ids)
			}
			for i := range want {
				if ids[i] != want[i] {
					t.Fatalf("asc order mismatch: got %v, want %v", ids, want)
				}
			}
		})
	}
}
```

The test relies on `context`, `encoding/json`, `net/http`, and `testing` — all already imported at the top of `user_games_test.go` (verified at `:1-13`).

- [ ] **Step 2: Run the new test and confirm it fails the right way**

Run:

```bash
go test -timeout 600s ./internal/api/... -run TestListUserGamesSortByGameNumerics -v
```

Expected: FAIL. Each of the four sub-tests should hit
`t.Fatalf("expected 200, got %d: ...", 400, ...)` because the handler currently
returns `400 "invalid sort_by field"` for both `sort_by=rating_average` and
`sort_by=howlongtobeat_main`. If the failure is anything else (e.g. panic on
nil map, compile error, 500), stop and investigate before continuing — the
test must fail for the *right* reason before we trust the green state.

Do **not** commit yet — the production fix lands in Task 2 and we commit the
test + fix together as one logical change.

---

## Task 2: Implement the backend fix

**Files:**
- Modify: `internal/api/user_games.go:133-149` (sort maps) and `:178-189`, `:269-271`, `:316-319` (order-expression builder + call sites)

- [ ] **Step 1: Extend the two existing sort maps and add the new `sortFieldsNullsLast` map**

In `internal/api/user_games.go`, replace the block at lines 133-153 (everything from `var allowedUserGameSortFields` through the closing brace of `sortFieldsRequiringHoursJoin`) with:

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
	"hours_played":       "COALESCE(hp.total, 0)",
	"howlongtobeat_main": "g.howlongtobeat_main",
	"rating_average":     "g.rating_average",
}

var sortFieldsRequiringGamesJoin = map[string]bool{
	"title":              true,
	"release_date":       true,
	"howlongtobeat_main": true,
	"rating_average":     true,
}

var sortFieldsRequiringHoursJoin = map[string]bool{
	"hours_played": true,
}

// sortFieldsNullsLast lists sort fields whose ORDER BY clause should append
// "NULLS LAST", so games without IGDB data (NULL) sink to the bottom regardless
// of sort direction. release_date is intentionally NOT in this set — changing
// its NULL ordering would be a user-visible behavior change beyond the scope
// of issue #639.
var sortFieldsNullsLast = map[string]bool{
	"howlongtobeat_main": true,
	"rating_average":     true,
}
```

- [ ] **Step 2: Build `orderExpr` once where the sort params are resolved**

In `HandleListUserGames`, locate the block where `sortBy`/`sortOrder`/`sortCol`
are parsed (currently lines 177-189):

```go
	// Parse sort.
	sortBy := c.QueryParam("sort_by")
	sortOrder := c.QueryParam("sort_order")
	var sortCol string
	if sortBy != "" {
		col, ok := allowedUserGameSortFields[sortBy]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid sort_by field")
		}
		sortCol = col
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
```

Append the `orderExpr` builder immediately after the `sortOrder` default
clamp, so it ends as:

```go
	// Parse sort.
	sortBy := c.QueryParam("sort_by")
	sortOrder := c.QueryParam("sort_order")
	var sortCol string
	if sortBy != "" {
		col, ok := allowedUserGameSortFields[sortBy]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid sort_by field")
		}
		sortCol = col
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	// Compose the ORDER BY expression once so both phases of the two-phase
	// list query stay in sync. NULLS LAST is opt-in per field.
	var orderExpr string
	if sortCol != "" {
		orderExpr = sortCol + " " + sortOrder
		if sortFieldsNullsLast[sortBy] {
			orderExpr += " NULLS LAST"
		}
	}
```

- [ ] **Step 3: Replace the inline ORDER BY in the ID-phase query**

At lines 269-271 (immediately after `idQ = fb.Apply(idQ)`):

```go
	if sortCol != "" {
		idQ = idQ.OrderExpr(sortCol + " " + sortOrder)
	}
```

Replace with:

```go
	if orderExpr != "" {
		idQ = idQ.OrderExpr(orderExpr)
	}
```

- [ ] **Step 4: Replace the inline ORDER BY in the model-phase query**

At line 318 (inside the `if sortCol != ""` block that re-applies sort on the
model query):

```go
		q = q.OrderExpr(sortCol + " " + sortOrder)
```

Replace with:

```go
		q = q.OrderExpr(orderExpr)
```

The surrounding `if sortCol != ""` guard stays as-is — `orderExpr` is empty
exactly when `sortCol` is empty, so the two are equivalent gates.

- [ ] **Step 5: Run the regression test and confirm it now passes**

Run:

```bash
go test -timeout 600s ./internal/api/... -run TestListUserGamesSortByGameNumerics -v
```

Expected: PASS, all four sub-tests green:

```
=== RUN   TestListUserGamesSortByGameNumerics
=== RUN   TestListUserGamesSortByGameNumerics/rating_average_desc_orders_high,_low,_null
=== RUN   TestListUserGamesSortByGameNumerics/rating_average_asc_orders_low,_high,_null
=== RUN   TestListUserGamesSortByGameNumerics/howlongtobeat_main_desc_orders_high,_low,_null
=== RUN   TestListUserGamesSortByGameNumerics/howlongtobeat_main_asc_orders_low,_high,_null
--- PASS: TestListUserGamesSortByGameNumerics (...)
```

- [ ] **Step 6: Run the full user-games test set to confirm no regression in adjacent sorts**

Run:

```bash
go test -timeout 600s ./internal/api/... -run 'TestListUserGames|TestListUserGameIDs|TestUserGameCalculatedHours' -v
```

Expected: PASS for every sub-test. Particular attention to
`TestListUserGamesSortByHours` (it shares the `OrderExpr` plumbing we just
touched) and any sort-by-title / sort-by-release-date sub-tests in
`TestListUserGames`. If any pre-existing test fails, the most likely culprit
is the `orderExpr` refactor — re-check that the guard semantics match the
original (`sortCol != ""` ↔ `orderExpr != ""`).

- [ ] **Step 7: Commit the fix + regression test together**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "fix: accept howlongtobeat_main and rating_average user-games sorts

The frontend dropdown offered \"Time to Beat\" and \"IGDB Rating\" sort
options, but the backend whitelist did not, so selecting either failed the
list fetch with HTTP 400 instead of sorting.

Add both fields to allowedUserGameSortFields and sortFieldsRequiringGamesJoin
so the existing LEFT JOIN games AS g is applied in both phases of the
two-phase list query. Introduce sortFieldsNullsLast and centralize the
ORDER BY expression so the two new sorts emit \"NULLS LAST\" in both
directions — games without IGDB ratings or HowLongToBeat estimates sink
to the bottom rather than floating to the top under DESC.

Closes #639"
```

The `fix:` prefix triggers a patch bump on the next release (per Conventional
Commits convention in `CLAUDE.md` → Release Process).

---

## Self-Review

**Spec coverage:**
- ✅ Extend `allowedUserGameSortFields` (spec → Task 2 Step 1).
- ✅ Extend `sortFieldsRequiringGamesJoin` (spec → Task 2 Step 1).
- ✅ Introduce `sortFieldsNullsLast` (spec → Task 2 Step 1).
- ✅ Centralize `orderExpr` build & reuse in both phases (spec → Task 2 Steps 2-4).
- ✅ NULLS LAST opt-in only for the two new sorts, `release_date` untouched (spec → Task 2 Step 1 comment).
- ✅ Regression test mirroring `TestListUserGamesSortByHours` with low/high/null fixture, both columns, both directions (spec → Task 1 Step 1).
- ✅ 200-status assertion as regression guard against the original 400 (Task 1 Step 1 — `idsInOrder` helper checks `rec.Code != http.StatusOK`).

**Placeholder scan:** No "TBD", "TODO", "appropriate error handling", or vague references. Every code step shows the exact final code.

**Type consistency:** `orderExpr` is `string` in both the builder and the two call sites. Map keys (`"howlongtobeat_main"`, `"rating_average"`) match across all three maps and the test's `sort_by` URL parameters. Test fixture IDs (`ug-low`, `ug-high`, `ug-null`) are referenced consistently in `want` slices for all four sub-tests.

---

## Notes for the executor

- Automated hooks will `gofmt -w` and `golangci-lint run` on the edited files after each Edit. Don't fight them.
- The `git push` pre-push hook will run the full `go test ./...` plus `npm run check && npm run knip && npm run test`. Run targeted tests during development per the steps above; the hook is the hard gate.
- The branch `fix/issue-639-sort-whitelist` already exists with the spec committed (`56293c99`). All work in this plan happens on that branch.
- No frontend, schema, slumber, or Nix changes are required — `slumber.yaml` already exercises the list endpoint and the sort fields are query params, not new routes.
