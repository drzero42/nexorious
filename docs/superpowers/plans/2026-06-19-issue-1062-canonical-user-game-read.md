# Canonical user-game read/projection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ~9 hand-copied user-game relation-loading query blocks in `internal/api` with one canonical relation decorator + two loader helpers, reusing the existing projection DTO.

**Architecture:** A new file `internal/api/user_game_read.go` holds three package-level free functions: `withUserGameRelations` (the one definition of the Game/Platforms/Tags/ExternalGame relation set), `loadUserGameDetail` (single owned row), and `loadUserGameCardsByIDs` (by-id list). All card/detail read sites in `user_games.go` and `pools.go` call them. The DTO projection `toUserGameWithPlatformsResponse` is unchanged.

**Tech Stack:** Go 1.26, Bun ORM (`uptrace/bun`), Echo v5, PostgreSQL via testcontainers.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-06-19-issue-1062-canonical-user-game-read-design.md`.
- Scope is `internal/api` only — do **not** touch `internal/worker/tasks/export.go`, stats/facet handlers, or `internal/usergame`.
- `ExternalGame` is part of the **one** canonical relation set — loaded everywhere. This is a deliberate additive behaviour change: list cards / pool cards / mutation responses now carry the platform `store_url` deep-link they previously lacked (built by `toUserGamePlatformResponse` from `ugp.ExternalGame`).
- All three helpers are package-level free functions taking `*bun.DB` (so both `UserGamesHandler` and `PoolsHandler` can call them).
- `loadUserGameDetail` always scopes by `user_id`; callers map `sql.ErrNoRows` (via `errors.Is`) to their existing 404/500 handling.
- Echo handler signature is `func (h *Handler) X(c *echo.Context) error` (pointer Context, v5).
- Tests use the shared `testDB` package var + `truncateAllTables(t)`; platform/storefront FK rows must use **seeded** names (`pc-windows`, `steam`).
- Run targeted tests for changed logic; the Stop/pre-push hooks run build/lint/full suites.

---

### Task 1: Canonical read helpers + focused test

**Files:**
- Create: `internal/api/user_game_read.go`
- Create test: `internal/api/user_game_read_test.go`

**Interfaces:**
- Produces:
  - `func withUserGameRelations(q *bun.SelectQuery) *bun.SelectQuery`
  - `func loadUserGameDetail(ctx context.Context, db *bun.DB, userGameID, userID string) (*models.UserGame, error)`
  - `func loadUserGameCardsByIDs(ctx context.Context, db *bun.DB, ids []string) ([]models.UserGame, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/api/user_game_read_test.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestLoadUserGameDetail(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := "u-read-1"
	insertAuthTestUser(t, testDB, userID, "readuser1", "pass123", true, false)
	gameID := insertTestGame(t, testDB, "Read Detail Game")
	insertTestUserGame(t, testDB, "ug-read-1", userID, int(gameID))

	platform := "pc-windows"
	storefront := "steam"
	insertTestUserGamePlatform(t, testDB, "ugp-read-1", "ug-read-1", &platform, &storefront)

	// External game wired to the platform — its presence is what makes the
	// store_url deep-link resolvable in the projection.
	insertExternalGame(t, testDB, "eg-read-1", userID, "steam", "ext-1", "Read Detail Game")
	if _, err := testDB.ExecContext(ctx,
		`UPDATE user_game_platforms SET external_game_id = ? WHERE id = ?`,
		"eg-read-1", "ugp-read-1"); err != nil {
		t.Fatalf("wire external game: %v", err)
	}

	insertTag(t, testDB, "tag-read-1", userID, "favorites", nil)
	insertUserGameTag(t, testDB, "ugt-read-1", "ug-read-1", "tag-read-1")

	t.Run("loads full relation set", func(t *testing.T) {
		ug, err := loadUserGameDetail(ctx, testDB, "ug-read-1", userID)
		if err != nil {
			t.Fatalf("loadUserGameDetail: %v", err)
		}
		if ug.Game == nil {
			t.Error("Game relation not loaded")
		}
		if len(ug.Platforms) != 1 {
			t.Fatalf("expected 1 platform, got %d", len(ug.Platforms))
		}
		p := ug.Platforms[0]
		if p.PlatformRecord == nil {
			t.Error("PlatformRecord not loaded")
		}
		if p.StorefrontRecord == nil {
			t.Error("StorefrontRecord not loaded")
		}
		if p.ExternalGame == nil {
			t.Error("ExternalGame not loaded")
		}
		if len(ug.Tags) != 1 || ug.Tags[0].Tag == nil {
			t.Errorf("Tags/Tag relation not loaded: %+v", ug.Tags)
		}
	})

	t.Run("scopes by user_id", func(t *testing.T) {
		_, err := loadUserGameDetail(ctx, testDB, "ug-read-1", "u-other")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows for another user's game, got %v", err)
		}
	})

	t.Run("by-ids loader returns the same relation set", func(t *testing.T) {
		ugs, err := loadUserGameCardsByIDs(ctx, testDB, []string{"ug-read-1"})
		if err != nil {
			t.Fatalf("loadUserGameCardsByIDs: %v", err)
		}
		if len(ugs) != 1 {
			t.Fatalf("expected 1, got %d", len(ugs))
		}
		if ugs[0].Game == nil || len(ugs[0].Platforms) != 1 || ugs[0].Platforms[0].ExternalGame == nil {
			t.Error("by-ids loader did not load full relation set")
		}
	})

	t.Run("by-ids loader with empty input", func(t *testing.T) {
		ugs, err := loadUserGameCardsByIDs(ctx, testDB, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ugs) != 0 {
			t.Errorf("expected empty slice, got %d", len(ugs))
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestLoadUserGameDetail -v`
Expected: FAIL — `undefined: loadUserGameDetail` / `undefined: loadUserGameCardsByIDs` (compile error).

- [ ] **Step 3: Write the helpers**

Create `internal/api/user_game_read.go`:

```go
package api

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// withUserGameRelations applies the canonical set of relations for projecting a
// user-game into a card/detail response: the game, its platforms (with platform,
// storefront, and external-game records — the last drives the store_url
// deep-link), and its tags. This is the single definition of that relation set;
// every card/detail read in this package loads through it.
func withUserGameRelations(q *bun.SelectQuery) *bun.SelectQuery {
	return q.
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord").Relation("ExternalGame")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		})
}

// loadUserGameDetail loads a single user-game owned by userID with the canonical
// relation set. Returns sql.ErrNoRows when the game does not exist or is not the
// caller's; callers map that to a 404.
func loadUserGameDetail(ctx context.Context, db *bun.DB, userGameID, userID string) (*models.UserGame, error) {
	var ug models.UserGame
	if err := withUserGameRelations(db.NewSelect().Model(&ug)).
		Where("user_game.id = ?", userGameID).
		Where("user_game.user_id = ?", userID).
		Scan(ctx); err != nil {
		return nil, err
	}
	return &ug, nil
}

// loadUserGameCardsByIDs loads user-games for the given ids with the canonical
// relation set, for list/card projections. Order is not guaranteed; callers that
// need a specific order re-apply it (HandleListUserGames) or key by id (pools).
func loadUserGameCardsByIDs(ctx context.Context, db *bun.DB, ids []string) ([]models.UserGame, error) {
	var userGames []models.UserGame
	if len(ids) == 0 {
		return userGames, nil
	}
	if err := withUserGameRelations(db.NewSelect().Model(&userGames)).
		Where("user_game.id IN (?)", bun.List(ids)).
		Scan(ctx); err != nil {
		return nil, err
	}
	return userGames, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestLoadUserGameDetail -v`
Expected: PASS (all four subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_game_read.go internal/api/user_game_read_test.go
git commit -m "feat(api): canonical user-game read helpers (#1062)"
```

---

### Task 2: Wire user_games.go onto the helpers

**Files:**
- Modify: `internal/api/user_games.go` — the list path (~324-334) and six single-id re-read blocks.

**Interfaces:**
- Consumes: `withUserGameRelations`, `loadUserGameDetail`, `loadUserGameCardsByIDs` from Task 1.

- [ ] **Step 1: Replace the list-path relation block**

In `HandleListUserGames`, replace the query construction (currently the `var userGames` + `q := h.db.NewSelect().Model(...).Where(...).Relation(...)` triple, ~lines 324-334) with the decorator. Leave the sort-join / `OrderExpr` / `Scan` block that follows it untouched:

```go
	// Fetch full records with relations, preserving sort order.
	var userGames []models.UserGame
	q := withUserGameRelations(h.db.NewSelect().
		Model(&userGames).
		Where("user_game.id IN (?)", bun.List(ids)))
```

- [ ] **Step 2: Replace the `HandleCreateUserGame` re-read**

Replace the `ug := &models.UserGame{}` re-select block (~lines 482-498) with:

```go
	ug, err := loadUserGameDetail(ctx, h.db, res.UserGameID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusCreated, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 3: Replace the `HandleGetUserGame` body**

Replace the re-select block (~lines 511-530) with:

```go
	ug, err := loadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 4: Replace the `HandleUpdateUserGame` re-read**

Replace the `var ug models.UserGame` re-select block (~lines 634-648) with:

```go
	ug, err := loadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 5: Replace the `HandleReplaceTags` re-read**

Replace the `var ug models.UserGame` re-select block (~lines 720-734) with:

```go
	ug, err := loadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 6: Replace the `HandleUpdateProgress` re-read**

Replace the `var ug models.UserGame` re-select block (~lines 771-785) with:

```go
	ug, err := loadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 7: Replace the `HandleMoveToLibrary` re-read (keep the log)**

Replace the `var ug models.UserGame` re-select block (~lines 1244-1258) with — preserving the existing error log:

```go
	ug, err := loadUserGameDetail(ctx, h.db, userGameID, userID)
	if err != nil {
		slog.ErrorContext(ctx, "user_games: reload after move-to-library", logging.KeyErr, err, "user_game_id", userGameID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
```

- [ ] **Step 8: Build and run the user-games endpoint tests**

Run: `go build ./internal/api/ && go test ./internal/api/ -run 'TestCreateUserGame|TestGetUserGame|TestUpdateUserGame|TestListUserGames|TestReplaceTags|TestUpdateProgress|TestMoveToLibrary' -v`
Expected: PASS. (The PostToolUse hook runs `gofmt`+`golangci-lint`; if any `bun` relation import became unused it would flag — none should here, `user_games.go` still uses `bun.List` and `bun` elsewhere.)

If any test asserts that a list/mutation platform response has **no** `store_url`, that assertion is now wrong per the deliberate behaviour change — update it to allow/expect the deep-link rather than reverting the wiring.

- [ ] **Step 9: Commit**

```bash
git add internal/api/user_games.go
git commit -m "refactor(api): route user_games reads through canonical loaders (#1062)"
```

---

### Task 3: Wire pools.go + dead-code + final verification

**Files:**
- Modify: `internal/api/pools.go` — `loadUserGameCards` (~lines 377-397).

**Interfaces:**
- Consumes: `loadUserGameCardsByIDs` from Task 1.

- [ ] **Step 1: Replace pools' `loadUserGameCards` body**

Replace the function body (the `var userGames` + `NewSelect().Model(...).Relation(...)...Scan` block) with a call to the shared loader, keeping the map-keying projection:

```go
// loadUserGameCards fetches user_games with relations for a set of ids and
// returns them keyed by id, reusing the list-item DTO shape.
func (h *PoolsHandler) loadUserGameCards(ctx context.Context, ids []string) (map[string]userGameWithPlatformsResponse, error) {
	userGames, err := loadUserGameCardsByIDs(ctx, h.db, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]userGameWithPlatformsResponse, len(userGames))
	for _, ug := range userGames {
		out[ug.ID] = toUserGameWithPlatformsResponse(ug)
	}
	return out, nil
}
```

- [ ] **Step 2: Build (catches any now-unused import in pools.go)**

Run: `go build ./internal/api/`
Expected: success. If `bun` is now unused in `pools.go`, remove it from the import block (the `gofmt`/lint hook also surfaces this). Verify `bun` is still referenced elsewhere in `pools.go` before removing — if it is, leave the import.

- [ ] **Step 3: Run the pools tests**

Run: `go test ./internal/api/ -run 'TestPool|TestListGameMemberships' -v`
Expected: PASS.

- [ ] **Step 4: Dead-code check**

Run: `make deadcode`
Expected: no **new** entries attributable to this diff. All three helpers are referenced; the old inline blocks were replaced, not orphaned. Reconcile any new entry against the diff before proceeding.

- [ ] **Step 5: Full package test + vet**

Run: `go test ./internal/api/ && go vet ./internal/api/`
Expected: PASS / no findings.

- [ ] **Step 6: Commit**

```bash
git add internal/api/pools.go
git commit -m "refactor(api): route pool card reads through canonical loader (#1062)"
```

---

## Self-Review

**Spec coverage:**
- Relation decorator (`withUserGameRelations`) — Task 1. ✓
- Single-row detail loader (`loadUserGameDetail`, user_id-scoped) replacing all six single-id blocks — Task 1 (def) + Task 2 (six sites). ✓
- By-id-list loader (`loadUserGameCardsByIDs`) for list + pools — Task 1 (def) + Task 2 step 1 (list) + Task 3 (pools). ✓
- `ExternalGame` always loaded; deliberate additive `store_url` behaviour change — encoded in the decorator (Task 1) and flagged in Task 2 step 8. ✓
- Projection `toUserGameWithPlatformsResponse` unchanged — never edited. ✓
- Home = `internal/api` only; export/stats/facets/usergame untouched — Global Constraints. ✓
- Tests: relation set populated + ownership scoping + by-ids — Task 1. ✓
- Dead-code check — Task 3 step 4. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. ✓

**Type consistency:** Helper names/signatures identical across Task 1 (definitions) and Tasks 2–3 (call sites): `withUserGameRelations`, `loadUserGameDetail(ctx, db, userGameID, userID)`, `loadUserGameCardsByIDs(ctx, db, ids)`. Detail loader returns `*models.UserGame` → callers project `*ug`. ✓
