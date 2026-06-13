# Play Planning (backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the backend for Play Planning pools (#955): a `pools` + `pool_games` schema, pool CRUD + nav-reorder API, declarative membership/queue API, a reusable `ApplyTimeToBeat` filter primitive, an OR-of-cards `ApplyPoolFilter`, a `?pool=:id` suggestion view on the user-games list, and a `RemoveFromPoolsIfFinished` completion hook.

**Architecture:** Pools are a sibling of tags. Handlers use raw SQL through `*bun.DB` exactly like `internal/api/tags.go`. The filter engine (`internal/filter`) gains a time-to-beat primitive and an OR-of-faceted-cards applier. The suggestion/filtered view reuses the existing `GET /api/user-games` list endpoint via a `?pool=:id` param. A finished play-status removes a game from every pool via an explicit helper in `internal/usergame`, wired into the single + bulk play-status write paths (mirroring `ClearWishlistOnAcquire`).

**Tech Stack:** Go 1.26, Echo v5 (`*echo.Context`), Bun ORM + `pgdriver`, Bun migrate (timestamp-prefixed SQL), PostgreSQL 18, stdlib `testing` + testcontainers (shared container per package via `TestMain`).

**Design source of truth:** `docs/superpowers/specs/2026-06-12-play-planning-backend-design.md` and the data-model comment on issue #939.

---

## Key facts verified against the codebase

- **Latest migration** is `20260609000001_*`; the new pair is `20260612000001_create_pools.{up,down}.sql`.
- **Handlers use raw SQL**, not Bun models, for CRUD (see `internal/api/tags.go`). `isDuplicateKeyError(err)` (defined in `tags.go`, same package) maps unique-violation → 409.
- **`internal/filter`** has `FilterBuilder` (`AddJoin`/`AddWhere`/`Apply`), `joinGames = "LEFT JOIN games AS g ON g.id = ug.game_id"`, `joinUserGamePlatforms = "LEFT JOIN user_game_platforms AS ugp ON ugp.user_game_id = ug.id"`, and `dbutil.LikeContains`. There is **no filter test harness** — all filter behaviour is tested through the API HTTP endpoints (`user_games_test.go`). We follow that.
- **`games.howlongtobeat_main`** is `*float64` (`internal/db/models/models.go`).
- **Finished set** values exist in `internal/enum/enum.go`: `completed`, `mastered`, `dominated`, `dropped`. There is currently no finished-set helper — Task 2 adds one.
- **`HandleUpdateUserGame`** (single, raw `UPDATE … RETURNING`, **not** currently in a txn) and **`HandleBulkUpdate`** (bulk, already in `RunInTx`) are the two play-status write paths (`internal/api/user_games.go`).
- **`UserGamesHandler`** has `db *bun.DB`; routes registered in `internal/api/router.go` (`userGamesGroup`, static routes before `/:id`). `TagsHandler` is constructed inline in `router.go` (`th := NewTagsHandler(db)`); pools follow the same wiring.
- **API tests** go through the full app via `newTestEcho(t, testDB, cfg)` → `api.New(...)`, with session cookies. Helpers: `insertAuthTestUser`, `loginAndGetToken`, `insertTestGame(t,db,title) int32`, `insertTestGameWithGenre`, `insertTestGameWithMetadata`, `insertTestUserGame(t,db,id,userID,gameID int)`, `insertTestUserGamePlatform`, `postJSONAuth`, `putJSONAuth`, `getAuth`, `deleteAuth`, `truncateAllTables`.
- **`truncateAllTables`** (`internal/api/main_test.go`) lists every data table; `pools` and `pool_games` must be added.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/db/migrations/20260612000001_create_pools.up.sql` / `.down.sql` | Schema: `pools`, `pool_games`, indexes, constraints |
| `internal/db/models/models.go` (modify) | `Pool`, `PoolGame` Bun structs |
| `internal/enum/enum.go` (modify) | `FinishedPlayStatuses` + `FinishedPlayStatusStrings()` |
| `internal/filter/criteria.go` (modify) | `ApplyTimeToBeat` primitive |
| `internal/filter/pool.go` (create) | `PoolFilter`, `FilterCard`, `ParsePoolFilter`, `FilterCard.HasFacets`, `ApplyPoolFilter` |
| `internal/usergame/pools.go` (create) | `RemoveFromPoolsIfFinished` helper |
| `internal/api/user_games.go` (modify) | wire `time_to_beat_*` params; wire completion hook into single + bulk paths; `?pool=:id` suggestion view + `pool_membership` |
| `internal/api/pools.go` (create) | `PoolsHandler` — CRUD, reorder, membership, queue |
| `internal/api/router.go` (modify) | register `/api/pools` routes |
| `internal/api/main_test.go` (modify) | add `pools`, `pool_games` to `truncateAllTables` |
| `internal/api/pools_test.go` (create) | pool CRUD, membership, queue, completion-hook, suggestion tests |
| `internal/api/user_games_test.go` (modify) | `time_to_beat` filter test |

---

## Task 1: Schema — `pools` + `pool_games` migration, models, truncate

**Files:**
- Create: `internal/db/migrations/20260612000001_create_pools.up.sql`
- Create: `internal/db/migrations/20260612000001_create_pools.down.sql`
- Modify: `internal/db/models/models.go` (append `Pool`, `PoolGame`)
- Modify: `internal/api/main_test.go` (truncate list)

- [ ] **Step 1: Write the up migration**

Create `internal/db/migrations/20260612000001_create_pools.up.sql`:

```sql
-- Play Planning pools (#955). A pool is a sibling of tags: a named, ordered,
-- user-defined collection of games to play, with an optional saved filter that
-- drives suggestions. pool_games is membership AND queue in one table:
-- position IS NULL = Candidate, position NOT NULL = in the Up Next queue.
CREATE TABLE pools (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT,
    position   INTEGER NOT NULL,
    filter     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX pools_user_id_idx ON pools (user_id);

CREATE TABLE pool_games (
    id           TEXT PRIMARY KEY,
    pool_id      TEXT NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
    user_game_id TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    position     INTEGER,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(pool_id, user_game_id)
);

CREATE INDEX pool_games_pool_id_idx ON pool_games (pool_id);
CREATE INDEX pool_games_user_game_id_idx ON pool_games (user_game_id);
```

- [ ] **Step 2: Write the down migration**

Create `internal/db/migrations/20260612000001_create_pools.down.sql` (child first):

```sql
DROP TABLE IF EXISTS pool_games;
DROP TABLE IF EXISTS pools;
```

- [ ] **Step 3: Add Bun models**

Append to `internal/db/models/models.go` (the file already `import`s `time` and `encoding/json`; if `encoding/json` is not imported, add it):

```go
// Pool is a Play Planning pool — a sibling of Tag with ordering, an optional
// saved filter, and queue membership via PoolGame (#955).
type Pool struct {
	bun.BaseModel `bun:"table:pools"`

	ID        string          `bun:"id,pk"               json:"id"`
	UserID    string          `bun:"user_id,notnull"     json:"user_id"`
	Name      string          `bun:"name,notnull"        json:"name"`
	Color     *string         `bun:"color"               json:"color"`
	Position  int             `bun:"position,notnull"    json:"position"`
	Filter    json.RawMessage `bun:"filter,type:jsonb"   json:"filter"`
	CreatedAt time.Time       `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt time.Time       `bun:"updated_at,notnull"  json:"updated_at"`
}

// PoolGame is a pool membership row. position IS NULL = Candidate;
// position NOT NULL = queued (Up Next). Unique per (pool_id, user_game_id).
type PoolGame struct {
	bun.BaseModel `bun:"table:pool_games"`

	ID         string    `bun:"id,pk"                 json:"id"`
	PoolID     string    `bun:"pool_id,notnull"       json:"pool_id"`
	UserGameID string    `bun:"user_game_id,notnull"  json:"user_game_id"`
	Position   *int      `bun:"position"              json:"position"`
	CreatedAt  time.Time `bun:"created_at,notnull"    json:"created_at"`
}
```

Verify `encoding/json` is in the import block of `models.go`; if absent, add it.

- [ ] **Step 4: Add the new tables to `truncateAllTables`**

In `internal/api/main_test.go`, inside the `TRUNCATE TABLE` list, add `pools` and `pool_games` (put them after `user_game_tags`):

```go
			tags,
			user_game_tags,
			pools,
			pool_games,
			external_games,
```

- [ ] **Step 5: Build and run a package that applies migrations**

Run: `go build ./... && go test ./internal/api/... -run TestListTags -v`
Expected: build succeeds; the test runs (proving migrations — including the new pair — apply cleanly in `TestMain`). If there is no `TestListTags`, run any existing api test, e.g. `go test ./internal/api/... -run TestCreateUserGame -v`.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/20260612000001_create_pools.up.sql \
        internal/db/migrations/20260612000001_create_pools.down.sql \
        internal/db/models/models.go internal/api/main_test.go \
        docs/superpowers/plans/2026-06-12-play-planning-backend.md
git commit -m "feat: add pools and pool_games schema and models"
```

---

## Task 2: Finished-set enum helper

**Files:**
- Modify: `internal/enum/enum.go`

- [ ] **Step 1: Add the finished-set values and helper**

Append to `internal/enum/enum.go` (after the `validPlayStatuses` map / `Valid()` method):

```go
// FinishedPlayStatuses are the play statuses that remove a game from every pool
// and exclude it from pool suggestions (#955). dropped is included deliberately:
// it is the strongest "not next" signal, so it leaves the plan like a completion.
var FinishedPlayStatuses = []PlayStatus{
	PlayStatusCompleted,
	PlayStatusMastered,
	PlayStatusDominated,
	PlayStatusDropped,
}

// FinishedPlayStatusStrings returns the finished set as plain strings, for use
// in SQL `IN (?)` clauses via bun.In.
func FinishedPlayStatusStrings() []string {
	out := make([]string, len(FinishedPlayStatuses))
	for i, s := range FinishedPlayStatuses {
		out[i] = string(s)
	}
	return out
}
```

- [ ] **Step 2: Build**

Run: `go build ./internal/enum/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/enum/enum.go
git commit -m "feat: add finished play-status set helper"
```

> No standalone test: this is a trivial fixed-set accessor (per the repo's testing policy, a struct/accessor wrapper does not warrant a test). Its behaviour is covered indirectly by the completion-hook and suggestion tests in Tasks 4 and 7.

---

## Task 3: `ApplyTimeToBeat` filter primitive + wire into the library list

**Files:**
- Modify: `internal/filter/criteria.go` (add `ApplyTimeToBeat`)
- Modify: `internal/api/user_games.go` (parse `time_to_beat_min` / `time_to_beat_max`)
- Test: `internal/api/user_games_test.go` (new test, via HTTP)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/user_games_test.go`:

```go
func TestListUserGamesTimeToBeatFilter(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "ttb")

	// Three games with distinct howlongtobeat_main, plus one with NULL.
	shortID := insertTestGameWithHLTB(t, testDB, "Short Game", floatPtr(5))
	midID := insertTestGameWithHLTB(t, testDB, "Mid Game", floatPtr(20))
	longID := insertTestGameWithHLTB(t, testDB, "Long Game", floatPtr(80))
	nullID := insertTestGameWithHLTB(t, testDB, "Unknown Game", nil)

	insertTestUserGame(t, testDB, "ug-ttb-short", userID, int(shortID))
	insertTestUserGame(t, testDB, "ug-ttb-mid", userID, int(midID))
	insertTestUserGame(t, testDB, "ug-ttb-long", userID, int(longID))
	insertTestUserGame(t, testDB, "ug-ttb-null", userID, int(nullID))

	titlesFor := func(query string) map[string]bool {
		rec := getAuth(t, e, "/api/user-games?"+query, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			UserGames []struct {
				Game struct {
					Title string `json:"title"`
				} `json:"game"`
			} `json:"user_games"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got := map[string]bool{}
		for _, ug := range resp.UserGames {
			got[ug.Game.Title] = true
		}
		return got
	}

	t.Run("max only excludes longer and NULL", func(t *testing.T) {
		got := titlesFor("time_to_beat_max=25")
		if !got["Short Game"] || !got["Mid Game"] {
			t.Fatalf("expected Short+Mid, got %v", got)
		}
		if got["Long Game"] || got["Unknown Game"] {
			t.Fatalf("did not expect Long/Unknown, got %v", got)
		}
	})

	t.Run("min only excludes shorter and NULL", func(t *testing.T) {
		got := titlesFor("time_to_beat_min=10")
		if !got["Mid Game"] || !got["Long Game"] {
			t.Fatalf("expected Mid+Long, got %v", got)
		}
		if got["Short Game"] || got["Unknown Game"] {
			t.Fatalf("did not expect Short/Unknown, got %v", got)
		}
	})

	t.Run("range", func(t *testing.T) {
		got := titlesFor("time_to_beat_min=10&time_to_beat_max=25")
		if !got["Mid Game"] || len(got) != 1 {
			t.Fatalf("expected only Mid Game, got %v", got)
		}
	})
}
```

Add these helpers to `internal/api/user_games_test.go` (near the other `insertTestGameWith*` helpers) if not already present:

```go
func floatPtr(f float64) *float64 { return &f }

func insertTestGameWithHLTB(t *testing.T, db *bun.DB, title string, hltbMain *float64) int32 {
	t.Helper()
	id := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET howlongtobeat_main = ? WHERE id = ?`, hltbMain, id)
	if err != nil {
		t.Fatalf("insertTestGameWithHLTB: %v", err)
	}
	return id
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestListUserGamesTimeToBeatFilter -v`
Expected: FAIL — the `time_to_beat_*` params are ignored, so `max only` returns Long/Unknown too. (If `insertTestGameWithHLTB`/`floatPtr` collide with existing helpers, the compile error tells you to drop the duplicate.)

- [ ] **Step 3: Add the `ApplyTimeToBeat` primitive**

Append to `internal/filter/criteria.go`:

```go
// ApplyTimeToBeat filters by games.howlongtobeat_main within [min, max].
// A NULL howlongtobeat_main never matches a range (NULL comparisons are false).
// Requires the games JOIN, like the genre/theme facets.
func ApplyTimeToBeat(fb *FilterBuilder, min, max *float64) {
	if min == nil && max == nil {
		return
	}
	fb.AddJoin("g", joinGames)
	if min != nil {
		fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("g.howlongtobeat_main >= ?", *min)
		})
	}
	if max != nil {
		fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("g.howlongtobeat_main <= ?", *max)
		})
	}
}
```

- [ ] **Step 4: Wire the params into `HandleListUserGames`**

In `internal/api/user_games.go`, in `HandleListUserGames`, immediately after the `rating_max` parse block (the one ending `filter.ApplyRatingMax(fb, &v)`), add:

```go
	var ttbMin, ttbMax *float64
	if str := c.QueryParam("time_to_beat_min"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			ttbMin = &v
		}
	}
	if str := c.QueryParam("time_to_beat_max"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			ttbMax = &v
		}
	}
	filter.ApplyTimeToBeat(fb, ttbMin, ttbMax)
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestListUserGamesTimeToBeatFilter -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/criteria.go internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat: add time-to-beat filter to the user-games list"
```

---

## Task 4: `RemoveFromPoolsIfFinished` completion hook + wiring

**Files:**
- Create: `internal/usergame/pools.go`
- Modify: `internal/api/user_games.go` (`HandleUpdateUserGame`, `HandleBulkUpdate`)
- Test: `internal/api/pools_test.go` (create; completion tests added here, CRUD tests come in Task 6 — but this task's tests need the membership insert helper below, which writes `pool_games` directly so it does not depend on Task 6's handlers)

- [ ] **Step 1: Write the completion-hook helper**

Create `internal/usergame/pools.go`:

```go
package usergame

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
)

// RemoveFromPoolsIfFinished deletes every pool membership for a user game when
// its current play_status is in the finished set (#955). It re-reads the row's
// play_status via an EXISTS guard, so it is idempotent and safe to call
// unconditionally on any play-status write path — it deletes only when the new
// status is finished. Mirrors ClearWishlistOnAcquire: accepts bun.IDB so it
// runs inside a caller's transaction. No queue renumber — gaps are tolerated by
// the data model and the next explicit queue write renumbers contiguous.
func RemoveFromPoolsIfFinished(ctx context.Context, db bun.IDB, userGameID string) error {
	_, err := db.NewRaw(
		`DELETE FROM pool_games
		 WHERE user_game_id = ?
		   AND EXISTS (
		       SELECT 1 FROM user_games
		       WHERE id = ? AND play_status IN (?)
		   )`,
		userGameID, userGameID, bun.List(enum.FinishedPlayStatusStrings()),
	).Exec(ctx)
	return err
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/api/pools_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// insertPool inserts a pool row directly and returns its ID.
func insertPool(t *testing.T, db *bun.DB, id, userID, name string, position int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO pools (id, user_id, name, position) VALUES (?, ?, ?, ?)`,
		id, userID, name, position,
	)
	if err != nil {
		t.Fatalf("insertPool: %v", err)
	}
}

// insertPoolGame inserts a pool_games membership row directly. position nil = Candidate.
func insertPoolGame(t *testing.T, db *bun.DB, poolID, userGameID string, position *int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), poolID, userGameID, position,
	)
	if err != nil {
		t.Fatalf("insertPoolGame: %v", err)
	}
}

// poolGameCount returns how many pool_games rows reference a user_game.
func poolGameCount(t *testing.T, db *bun.DB, userGameID string) int {
	t.Helper()
	n, err := db.NewSelect().Table("pool_games").
		Where("user_game_id = ?", userGameID).Count(context.Background())
	if err != nil {
		t.Fatalf("poolGameCount: %v", err)
	}
	return n
}

func TestCompletionRemovesFromPools(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "complete")

	t.Run("single update to finished removes from all pools", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Done Single")
		insertTestUserGame(t, testDB, "ug-done-1", userID, int(gameID))
		insertPool(t, testDB, "pool-a", userID, "Pool A", 0)
		insertPool(t, testDB, "pool-b", userID, "Pool B", 1)
		insertPoolGame(t, testDB, "pool-a", "ug-done-1", nil)
		pos := 0
		insertPoolGame(t, testDB, "pool-b", "ug-done-1", &pos)

		if poolGameCount(t, testDB, "ug-done-1") != 2 {
			t.Fatalf("setup: expected 2 memberships")
		}

		rec := putJSONAuth(t, e, "/api/user-games/ug-done-1",
			map[string]any{"play_status": "completed"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-done-1"); got != 0 {
			t.Fatalf("expected 0 memberships after completion, got %d", got)
		}
	})

	t.Run("single update to eligible status keeps memberships", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Still Playing")
		insertTestUserGame(t, testDB, "ug-play-1", userID, int(gameID))
		insertPool(t, testDB, "pool-c", userID, "Pool C", 2)
		insertPoolGame(t, testDB, "pool-c", "ug-play-1", nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-play-1",
			map[string]any{"play_status": "in_progress"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-play-1"); got != 1 {
			t.Fatalf("expected membership kept, got %d", got)
		}
	})

	t.Run("dropped also removes", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Dropped Game")
		insertTestUserGame(t, testDB, "ug-drop-1", userID, int(gameID))
		insertPool(t, testDB, "pool-d", userID, "Pool D", 3)
		insertPoolGame(t, testDB, "pool-d", "ug-drop-1", nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-drop-1",
			map[string]any{"play_status": "dropped"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := poolGameCount(t, testDB, "ug-drop-1"); got != 0 {
			t.Fatalf("expected 0 after dropped, got %d", got)
		}
	})

	t.Run("bulk update to finished removes from pools", func(t *testing.T) {
		g1 := insertTestGame(t, testDB, "Bulk Done 1")
		g2 := insertTestGame(t, testDB, "Bulk Done 2")
		insertTestUserGame(t, testDB, "ug-bulk-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-bulk-2", userID, int(g2))
		insertPool(t, testDB, "pool-e", userID, "Pool E", 4)
		insertPoolGame(t, testDB, "pool-e", "ug-bulk-1", nil)
		insertPoolGame(t, testDB, "pool-e", "ug-bulk-2", nil)

		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bulk-1", "ug-bulk-2"},
			"updates": map[string]any{"play_status": "mastered"},
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if poolGameCount(t, testDB, "ug-bulk-1") != 0 || poolGameCount(t, testDB, "ug-bulk-2") != 0 {
			t.Fatalf("expected both removed after bulk completion")
		}
	})
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestCompletionRemovesFromPools -v`
Expected: FAIL — memberships are not removed (hook not wired). The `single update to eligible status` and bulk subtests fail because the hook isn't called at all.

- [ ] **Step 4: Wire the hook into `HandleUpdateUserGame` (single)**

In `internal/api/user_games.go`, `HandleUpdateUserGame` currently runs a raw `UPDATE … RETURNING` on `h.db` then eager-loads. Wrap the update **and** the hook in a transaction. Replace the block that runs the update (the `var ug models.UserGame` + `h.db.NewRaw(query, args...).Scan(ctx, &ug)` + its error handling) with:

```go
	_, statusChanged := body["play_status"]

	var ug models.UserGame
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if scanErr := tx.NewRaw(query, args...).Scan(ctx, &ug); scanErr != nil {
			return scanErr
		}
		if statusChanged {
			// The UPDATE above is applied within this txn, so the helper's
			// EXISTS guard sees the new play_status. Removes from every pool
			// if the new status is finished; no-op otherwise.
			return usergame.RemoveFromPoolsIfFinished(ctx, tx, ug.ID)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
```

(Leave the subsequent eager-load `h.db.NewSelect().Model(&ug)…` block and the final `return c.JSON(...)` unchanged. `usergame` is already imported in this file.)

- [ ] **Step 5: Wire the hook into `HandleBulkUpdate` (bulk)**

In `HandleBulkUpdate`, inside the existing `RunInTx` closure, after `rowsAffected, err = res.RowsAffected()` and before `return err`, add a per-id hook call gated on play_status being updated. Replace:

```go
		rowsAffected, err = res.RowsAffected()
		return err
	})
```

with:

```go
		rowsAffected, err = res.RowsAffected()
		if err != nil {
			return err
		}
		if _, ok := req.Updates["play_status"]; ok {
			for _, id := range req.IDs {
				if hookErr := usergame.RemoveFromPoolsIfFinished(ctx, tx, id); hookErr != nil {
					return hookErr
				}
			}
		}
		return nil
	})
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestCompletionRemovesFromPools -v`
Expected: PASS (all four subtests).

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/pools.go internal/api/user_games.go internal/api/pools_test.go
git commit -m "feat: remove games from pools when play-status becomes finished"
```

---

## Task 5: `PoolFilter` / `FilterCard` types + `ApplyPoolFilter` + strict parse

**Files:**
- Create: `internal/filter/pool.go`

This task adds the filter machinery. It is exercised end-to-end by the suggestion-endpoint test in Task 7 (no standalone filter harness exists). Build is the gate here; behaviour is verified in Task 7.

- [ ] **Step 1: Write the pool filter types and applier**

Create `internal/filter/pool.go`:

```go
package filter

import (
	"bytes"
	"encoding/json"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbutil"
)

// PoolFilter is a pool's saved filter: an ordered list of faceted cards
// evaluated as OR — a game matches the pool if it matches ANY card (#955).
type PoolFilter struct {
	Filters []FilterCard `json:"filters"`
}

// FilterCard mirrors the library list params. Each facet ANDs with the others
// within a card; multiple values within a single facet OR together.
type FilterCard struct {
	PlayStatus        *string  `json:"play_status,omitempty"`
	Genre             []string `json:"genre,omitempty"`
	Theme             []string `json:"theme,omitempty"`
	Tag               []string `json:"tag,omitempty"`
	Platform          []string `json:"platform,omitempty"`
	Storefront        []string `json:"storefront,omitempty"`
	RatingMin         *float64 `json:"rating_min,omitempty"`
	RatingMax         *float64 `json:"rating_max,omitempty"`
	IsLoved           *bool    `json:"is_loved,omitempty"`
	GameMode          []string `json:"game_mode,omitempty"`
	PlayerPerspective []string `json:"player_perspective,omitempty"`
	Q                 *string  `json:"q,omitempty"`
	TimeToBeatMin     *float64 `json:"time_to_beat_min,omitempty"`
	TimeToBeatMax     *float64 `json:"time_to_beat_max,omitempty"`
}

// ParsePoolFilter unmarshals a saved filter with unknown keys rejected
// ("typed in Go, JSONB at rest"). DisallowUnknownFields applies recursively to
// the nested cards too.
func ParsePoolFilter(raw []byte) (PoolFilter, error) {
	var pf PoolFilter
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pf); err != nil {
		return PoolFilter{}, err
	}
	return pf, nil
}

// HasFacets reports whether the card constrains at least one facet. An empty
// card (no facets) is rejected at pool create/update time.
func (c FilterCard) HasFacets() bool {
	return c.PlayStatus != nil ||
		len(c.Genre) > 0 ||
		len(c.Theme) > 0 ||
		len(c.Tag) > 0 ||
		len(c.Platform) > 0 ||
		len(c.Storefront) > 0 ||
		c.RatingMin != nil ||
		c.RatingMax != nil ||
		c.IsLoved != nil ||
		len(c.GameMode) > 0 ||
		len(c.PlayerPerspective) > 0 ||
		(c.Q != nil && *c.Q != "") ||
		c.TimeToBeatMin != nil ||
		c.TimeToBeatMax != nil
}

func (c FilterCard) needsGamesJoin() bool {
	return len(c.Genre) > 0 || len(c.Theme) > 0 || len(c.GameMode) > 0 ||
		len(c.PlayerPerspective) > 0 || (c.Q != nil && *c.Q != "") ||
		c.TimeToBeatMin != nil || c.TimeToBeatMax != nil
}

func (c FilterCard) needsPlatformsJoin() bool {
	return len(c.Platform) > 0 || len(c.Storefront) > 0
}

// ApplyPoolFilter applies the OR-of-cards predicate: (card1) OR (card2) OR …,
// where each card is the AND of its facet predicates. Required JOINs are
// registered once. The global finished-status exclusion is applied by the
// caller, OUTSIDE this function — it is never stored in a card.
func ApplyPoolFilter(fb *FilterBuilder, pf PoolFilter) {
	if len(pf.Filters) == 0 {
		return
	}
	for _, c := range pf.Filters {
		if c.needsGamesJoin() {
			fb.AddJoin("g", joinGames)
		}
		if c.needsPlatformsJoin() {
			fb.AddJoin("ugp", joinUserGamePlatforms)
		}
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, c := range pf.Filters {
				c := c
				q = q.WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return applyCardPredicates(q, c)
				})
			}
			return q
		})
	})
}

// applyCardPredicates ANDs one card's facet predicates onto q. Multi-value
// facets OR their values inside an AND-attached group, mirroring the standalone
// Apply* helpers in criteria.go.
func applyCardPredicates(q *bun.SelectQuery, c FilterCard) *bun.SelectQuery {
	if c.PlayStatus != nil {
		q = q.Where("ug.play_status = ?", *c.PlayStatus)
	}
	if c.IsLoved != nil {
		q = q.Where("ug.is_loved = ?", *c.IsLoved)
	}
	if c.RatingMin != nil {
		q = q.Where("ug.personal_rating >= ?", *c.RatingMin)
	}
	if c.RatingMax != nil {
		q = q.Where("ug.personal_rating <= ?", *c.RatingMax)
	}
	if c.TimeToBeatMin != nil {
		q = q.Where("g.howlongtobeat_main >= ?", *c.TimeToBeatMin)
	}
	if c.TimeToBeatMax != nil {
		q = q.Where("g.howlongtobeat_main <= ?", *c.TimeToBeatMax)
	}
	q = orILike(q, "g.genre", c.Genre)
	q = orILike(q, "g.themes", c.Theme)
	q = orILike(q, "g.game_modes", c.GameMode)
	q = orILike(q, "g.player_perspectives", c.PlayerPerspective)
	if c.Q != nil && *c.Q != "" {
		pattern := dbutil.LikeContains(*c.Q)
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereOr("g.title ILIKE ?", pattern)
			q = q.WhereOr("ug.personal_notes IS NOT NULL AND ug.personal_notes ILIKE ?", pattern)
			return q
		})
	}
	q = orIn(q, "ugp.platform", c.Platform)
	q = orIn(q, "ugp.storefront", c.Storefront)
	if len(c.Tag) > 0 {
		q = q.Where("ug.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (?))", bun.List(c.Tag))
	}
	return q
}

// orILike ANDs an (col ILIKE v1 OR col ILIKE v2 …) group onto q.
func orILike(q *bun.SelectQuery, col string, values []string) *bun.SelectQuery {
	if len(values) == 0 {
		return q
	}
	return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, v := range values {
			q = q.WhereOr(col+" ILIKE ?", dbutil.LikeContains(v))
		}
		return q
	})
}

// orIn ANDs a (col IN (values)) group onto q.
func orIn(q *bun.SelectQuery, col string, values []string) *bun.SelectQuery {
	if len(values) == 0 {
		return q
	}
	return q.Where(col+" IN (?)", bun.List(values))
}
```

Note: `bun.List(slice)` expands to a comma-separated value list and pairs with `IN (?)` — the codebase convention (see `ApplyTag` in `criteria.go`). Do **not** use `bun.In` here: it adds its own parentheses and would emit invalid `IN ((a,b,c))`.

- [ ] **Step 2: Build**

Run: `go build ./internal/filter/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/filter/pool.go
git commit -m "feat: add pool filter types and OR-of-cards applier"
```

---

## Task 6: Pool CRUD + nav reorder handlers + routes

**Files:**
- Create: `internal/api/pools.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/pools_test.go` (append CRUD tests)

- [ ] **Step 1: Write the failing CRUD test**

Append to `internal/api/pools_test.go`:

```go
func TestPoolCRUD(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "crud")

	t.Run("create requires name", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create and list", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Backlog"}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Position int    `json:"position"`
		}
		mustUnmarshal(t, rec, &created)
		if created.ID == "" || created.Name != "Backlog" {
			t.Fatalf("unexpected create response: %+v", created)
		}

		// Second pool appends at max+1.
		rec2 := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Co-op"}, token)
		var created2 struct {
			Position int `json:"position"`
		}
		mustUnmarshal(t, rec2, &created2)
		if created2.Position <= created.Position {
			t.Fatalf("expected appended position > %d, got %d", created.Position, created2.Position)
		}

		listRec := getAuth(t, e, "/api/pools", token)
		var list []struct {
			Name           string `json:"name"`
			HasFilter      bool   `json:"has_filter"`
			QueueCount     int    `json:"queue_count"`
			CandidateCount int    `json:"candidate_count"`
		}
		mustUnmarshal(t, listRec, &list)
		if len(list) != 2 {
			t.Fatalf("expected 2 pools, got %d", len(list))
		}
	})

	t.Run("duplicate name conflicts", func(t *testing.T) {
		postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Unique"}, token)
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Unique"}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty card rejected", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "BadFilter",
			"filter": map[string]any{"filters": []any{map[string]any{}}},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty card, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown filter key rejected", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "BadKey",
			"filter": map[string]any{"filters": []any{map[string]any{"nope": "x"}}},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for unknown key, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty filters coerced to null", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{
			"name":   "ManualPool",
			"filter": map[string]any{"filters": []any{}},
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created struct {
			ID        string `json:"id"`
			HasFilter bool   `json:"has_filter"`
		}
		mustUnmarshal(t, rec, &created)
		if created.HasFilter {
			t.Fatalf("expected has_filter=false for empty filters")
		}
	})

	t.Run("update and delete", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "ToEdit"}, token)
		var created struct {
			ID string `json:"id"`
		}
		mustUnmarshal(t, rec, &created)

		newName := "Edited"
		upd := putJSONAuth(t, e, "/api/pools/"+created.ID, map[string]any{"name": newName}, token)
		if upd.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", upd.Code, upd.Body.String())
		}

		del := deleteAuth(t, e, "/api/pools/"+created.ID, token)
		if del.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", del.Code, del.Body.String())
		}

		// Deleting again → 404.
		del2 := deleteAuth(t, e, "/api/pools/"+created.ID, token)
		if del2.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", del2.Code, del2.Body.String())
		}
	})

	t.Run("reorder renumbers contiguous", func(t *testing.T) {
		truncateAllTables(t)
		_, tok := setupUserGamesUser(t, testDB, e, "reorder")
		var ids []string
		for _, n := range []string{"P1", "P2", "P3"} {
			rec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": n}, tok)
			var c struct {
				ID string `json:"id"`
			}
			mustUnmarshal(t, rec, &c)
			ids = append(ids, c.ID)
		}
		// Reverse order.
		reordered := []string{ids[2], ids[1], ids[0]}
		rec := postJSONAuth(t, e, "/api/pools/reorder", map[string]any{"ids": reordered}, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		listRec := getAuth(t, e, "/api/pools", tok)
		var list []struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}
		mustUnmarshal(t, listRec, &list)
		// List is ordered by position; expect reversed order with positions 0,1,2.
		for i, want := range reordered {
			if list[i].ID != want {
				t.Fatalf("position %d: expected %s, got %s", i, want, list[i].ID)
			}
			if list[i].Position != i {
				t.Fatalf("expected contiguous position %d, got %d", i, list[i].Position)
			}
		}
	})
}
```

Add this helper to `internal/api/pools_test.go` (near the top, after the imports — and add `"encoding/json"`, `"net/http/httptest"`, `"testing"` to the import block as needed):

```go
func mustUnmarshal(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("unmarshal (%d): %v — body: %s", rec.Code, err, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestPoolCRUD -v`
Expected: FAIL — routes return 404 (handlers not registered yet). Likely a compile error first if `mustUnmarshal` imports are missing; fix imports until it compiles, then it fails on 404.

- [ ] **Step 3: Write the pools handler (CRUD + reorder)**

Create `internal/api/pools.go`:

```go
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/filter"
)

// PoolsHandler handles /api/pools endpoints (Play Planning, #955).
type PoolsHandler struct {
	db *bun.DB
}

// NewPoolsHandler returns a new PoolsHandler.
func NewPoolsHandler(db *bun.DB) *PoolsHandler {
	return &PoolsHandler{db: db}
}

// poolListItem is the response shape for GET /api/pools.
type poolListItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Color          *string `json:"color"`
	Position       int     `json:"position"`
	HasFilter      bool    `json:"has_filter"`
	QueueCount     int64   `json:"queue_count"`
	CandidateCount int64   `json:"candidate_count"`
}

// poolResponse is the response shape for create/update.
type poolResponse struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Name      string          `json:"name"`
	Color     *string         `json:"color"`
	Position  int             `json:"position"`
	Filter    json.RawMessage `json:"filter"`
	HasFilter bool            `json:"has_filter"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// HandleListPools handles GET /api/pools.
func (h *PoolsHandler) HandleListPools(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var pools []poolListItem
	err := h.db.NewRaw(`
		SELECT p.id, p.name, p.color, p.position,
		       (p.filter IS NOT NULL) AS has_filter,
		       COUNT(pg.id) FILTER (WHERE pg.position IS NOT NULL) AS queue_count,
		       COUNT(pg.id) FILTER (WHERE pg.position IS NULL)     AS candidate_count
		FROM pools p
		LEFT JOIN pool_games pg ON pg.pool_id = p.id
		WHERE p.user_id = ?
		GROUP BY p.id
		ORDER BY p.position`,
		userID,
	).Scan(context.Background(), &pools)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list pools")
	}
	if pools == nil {
		pools = []poolListItem{}
	}
	return c.JSON(http.StatusOK, pools)
}

// createPoolRequest is the body for POST /api/pools.
type createPoolRequest struct {
	Name   string          `json:"name"`
	Color  *string         `json:"color"`
	Filter json.RawMessage `json:"filter"`
}

// HandleCreatePool handles POST /api/pools.
func (h *PoolsHandler) HandleCreatePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req createPoolRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if len(req.Name) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be 100 characters or less")
	}

	normFilter, err := normalizePoolFilter(req.Filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	now := time.Now().UTC()
	id := uuid.NewString()
	ctx := context.Background()

	var pool poolResponse
	err = h.db.NewRaw(`
		INSERT INTO pools (id, user_id, name, color, position, filter, created_at, updated_at)
		VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM pools WHERE user_id = ?), 0), ?, ?, ?)
		RETURNING id, user_id, name, color, position, filter, created_at, updated_at`,
		id, userID, req.Name, req.Color, userID, normFilter, now, now,
	).Scan(ctx, &pool)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "pool name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create pool")
	}
	pool.HasFilter = pool.Filter != nil
	return c.JSON(http.StatusCreated, pool)
}

// updatePoolRequest is the body for PUT /api/pools/:id. Fields absent → unchanged.
type updatePoolRequest struct {
	Name   *string         `json:"name"`
	Color  *string         `json:"color"`
	Filter json.RawMessage `json:"filter"`
}

// HandleUpdatePool handles PUT /api/pools/:id.
func (h *PoolsHandler) HandleUpdatePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	// Decode into a map first to detect whether "filter" was present at all.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	if nameRaw, ok := raw["name"]; ok {
		var name string
		if err := json.Unmarshal(nameRaw, &name); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid name")
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "name is required")
		}
		if len(name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "name must be 100 characters or less")
		}
		setClauses = append(setClauses, "name = ?")
		args = append(args, name)
	}
	if colorRaw, ok := raw["color"]; ok {
		var color *string
		if err := json.Unmarshal(colorRaw, &color); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid color")
		}
		setClauses = append(setClauses, "color = ?")
		args = append(args, color)
	}
	if filterRaw, ok := raw["filter"]; ok {
		normFilter, err := normalizePoolFilter(filterRaw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		setClauses = append(setClauses, "filter = ?")
		args = append(args, normFilter)
	}

	if len(setClauses) == 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}

	args = append(args, poolID, userID)
	query := fmt.Sprintf(`
		UPDATE pools SET %s
		WHERE id = ? AND user_id = ?
		RETURNING id, user_id, name, color, position, filter, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)

	var pool poolResponse
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &pool)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "pool name already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update pool")
	}
	pool.HasFilter = pool.Filter != nil
	return c.JSON(http.StatusOK, pool)
}

// HandleDeletePool handles DELETE /api/pools/:id.
func (h *PoolsHandler) HandleDeletePool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	res, err := h.db.NewRaw(`DELETE FROM pools WHERE id = ? AND user_id = ?`, poolID, userID).
		Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete pool")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete pool")
	}
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// reorderRequest is the body for POST /api/pools/reorder and PUT queue.
type reorderRequest struct {
	IDs []string `json:"ids"`
}

// HandleReorderPools handles POST /api/pools/reorder — renumber pools.position
// contiguous in the given order, in a txn. Only the caller's own pools move.
func (h *PoolsHandler) HandleReorderPools(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx := context.Background()
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for i, id := range req.IDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE pools SET position = ?, updated_at = now() WHERE id = ? AND user_id = ?`,
				i, id, userID,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reorder pools")
	}
	return c.NoContent(http.StatusNoContent)
}

// normalizePoolFilter validates and canonicalises a raw pool filter.
//   - absent / JSON null / empty filters array → returns nil (pure manual pool)
//   - unknown keys → error
//   - any card with no facets → error
//
// It returns the canonical JSON to store (or nil for NULL).
func normalizePoolFilter(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	pf, err := filter.ParsePoolFilter(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %v", err)
	}
	if len(pf.Filters) == 0 {
		return nil, nil
	}
	for _, card := range pf.Filters {
		if !card.HasFacets() {
			return nil, errors.New("filter card has no facets")
		}
	}
	canonical, err := json.Marshal(pf)
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %v", err)
	}
	return canonical, nil
}
```

- [ ] **Step 4: Register the routes**

In `internal/api/router.go`, immediately after the tags-group block (lines registering `tagsGroup`), add:

```go
		// Pools routes (Play Planning, #955). Static /reorder before /:id.
		poolsHandler := NewPoolsHandler(db)
		poolsGroup := e.Group("/api/pools", auth.AuthMiddleware(db))
		poolsGroup.GET("", poolsHandler.HandleListPools)
		poolsGroup.POST("", poolsHandler.HandleCreatePool)
		poolsGroup.POST("/reorder", poolsHandler.HandleReorderPools)
		poolsGroup.GET("/:id", poolsHandler.HandleGetPool)
		poolsGroup.PUT("/:id", poolsHandler.HandleUpdatePool)
		poolsGroup.DELETE("/:id", poolsHandler.HandleDeletePool)
		poolsGroup.POST("/:id/games", poolsHandler.HandleAddPoolGame)
		poolsGroup.DELETE("/:id/games/:userGameId", poolsHandler.HandleRemovePoolGame)
		poolsGroup.PUT("/:id/queue", poolsHandler.HandleSetQueue)
```

> `HandleGetPool`, `HandleAddPoolGame`, `HandleRemovePoolGame`, and `HandleSetQueue` are implemented in Task 7. To keep this task compiling on its own, add **temporary stubs** at the bottom of `internal/api/pools.go` now and replace them in Task 7:

```go
// Stubs replaced in Task 7.
func (h *PoolsHandler) HandleGetPool(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleAddPoolGame(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleRemovePoolGame(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
func (h *PoolsHandler) HandleSetQueue(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestPoolCRUD -v`
Expected: PASS (all subtests).

- [ ] **Step 6: Commit**

```bash
git add internal/api/pools.go internal/api/router.go internal/api/pools_test.go
git commit -m "feat: add pool CRUD and nav-reorder endpoints"
```

---

## Task 7: Membership + queue handlers + pool detail

**Files:**
- Modify: `internal/api/pools.go` (replace the four stubs)
- Test: `internal/api/pools_test.go` (append membership/queue tests)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/pools_test.go`:

```go
func TestPoolMembershipAndQueue(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "queue")

	// One pool, three owned user-games.
	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Queue Pool"}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)

	var ugIDs []string
	for i, title := range []string{"G1", "G2", "G3"} {
		gid := insertTestGame(t, testDB, title)
		ugID := fmt.Sprintf("ug-q-%d", i)
		insertTestUserGame(t, testDB, ugID, userID, int(gid))
		ugIDs = append(ugIDs, ugID)
	}

	t.Run("add lands as candidate, idempotent", func(t *testing.T) {
		for _, ugID := range ugIDs {
			rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
				map[string]any{"user_game_id": ugID}, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		// Re-add is a 200 no-op.
		rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
			map[string]any{"user_game_id": ugIDs[0]}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected idempotent 200, got %d: %s", rec.Code, rec.Body.String())
		}

		detail := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d struct {
			Queue      []map[string]any `json:"queue"`
			Candidates []map[string]any `json:"candidates"`
		}
		mustUnmarshal(t, detail, &d)
		if len(d.Candidates) != 3 || len(d.Queue) != 0 {
			t.Fatalf("expected 3 candidates / 0 queued, got %d / %d", len(d.Candidates), len(d.Queue))
		}
	})

	t.Run("add rejects non-existent user_game", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games",
			map[string]any{"user_game_id": "does-not-exist"}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("queue promote, reorder, demote in one PUT", func(t *testing.T) {
		// Promote G1, G2 (in that order); G3 stays candidate.
		rec := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{ugIDs[0], ugIDs[1]}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		detail := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d struct {
			Queue []struct {
				ID string `json:"id"`
			} `json:"queue"`
			Candidates []struct {
				ID string `json:"id"`
			} `json:"candidates"`
		}
		mustUnmarshal(t, detail, &d)
		if len(d.Queue) != 2 || d.Queue[0].ID != ugIDs[0] || d.Queue[1].ID != ugIDs[1] {
			t.Fatalf("unexpected queue: %+v", d.Queue)
		}
		if len(d.Candidates) != 1 || d.Candidates[0].ID != ugIDs[2] {
			t.Fatalf("unexpected candidates: %+v", d.Candidates)
		}

		// Reorder (swap) and demote G1 back to candidate by omitting it.
		rec2 := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{ugIDs[1]}}, token)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
		}
		detail2 := getAuth(t, e, "/api/pools/"+pool.ID, token)
		var d2 struct {
			Queue []struct {
				ID string `json:"id"`
			} `json:"queue"`
		}
		mustUnmarshal(t, detail2, &d2)
		if len(d2.Queue) != 1 || d2.Queue[0].ID != ugIDs[1] {
			t.Fatalf("expected only G2 queued, got %+v", d2.Queue)
		}
	})

	t.Run("queue rejects a non-member id", func(t *testing.T) {
		gid := insertTestGame(t, testDB, "Outsider")
		insertTestUserGame(t, testDB, "ug-outsider", userID, int(gid))
		rec := putJSONAuth(t, e, "/api/pools/"+pool.ID+"/queue",
			map[string]any{"ids": []string{"ug-outsider"}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-member, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("remove membership", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/pools/"+pool.ID+"/games/"+ugIDs[2], token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		// Removing again → 404.
		rec2 := deleteAuth(t, e, "/api/pools/"+pool.ID+"/games/"+ugIDs[2], token)
		if rec2.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec2.Code, rec2.Body.String())
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestPoolMembershipAndQueue -v`
Expected: FAIL — the stubs return 501.

- [ ] **Step 3: Replace the four stubs with real handlers**

In `internal/api/pools.go`, remove the four stub functions and add the real implementations (and the response shapes). Reuse `userGameWithPlatformsResponse` / `toUserGameWithPlatformsResponse` from `user_games.go` (same package) for member cards:

```go
// poolDetailResponse is the response for GET /api/pools/:id.
type poolDetailResponse struct {
	poolResponse
	Queue      []userGameWithPlatformsResponse `json:"queue"`
	Candidates []userGameWithPlatformsResponse `json:"candidates"`
}

// poolMember pairs a user_game id with its queue position (NULL = candidate).
type poolMember struct {
	UserGameID string `bun:"user_game_id"`
	Position   *int   `bun:"position"`
}

// HandleGetPool handles GET /api/pools/:id — pool meta + members inline.
func (h *PoolsHandler) HandleGetPool(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")
	ctx := context.Background()

	var pool poolResponse
	err := h.db.NewRaw(`
		SELECT id, user_id, name, color, position, filter, created_at, updated_at
		FROM pools WHERE id = ? AND user_id = ?`,
		poolID, userID,
	).Scan(ctx, &pool)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	pool.HasFilter = pool.Filter != nil

	// Membership rows ordered: queued first by (position, created_at), then
	// candidates by created_at.
	var members []poolMember
	err = h.db.NewRaw(`
		SELECT user_game_id, position
		FROM pool_games
		WHERE pool_id = ?
		ORDER BY (position IS NULL), position, created_at`,
		poolID,
	).Scan(ctx, &members)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	detail := poolDetailResponse{
		poolResponse: pool,
		Queue:        []userGameWithPlatformsResponse{},
		Candidates:   []userGameWithPlatformsResponse{},
	}
	if len(members) == 0 {
		return c.JSON(http.StatusOK, detail)
	}

	// Collect ids and fetch full user-game cards in one query, then re-split,
	// preserving membership order.
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserGameID
	}
	cards, err := h.loadUserGameCards(ctx, ids)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, m := range members {
		card, ok := cards[m.UserGameID]
		if !ok {
			continue
		}
		if m.Position != nil {
			detail.Queue = append(detail.Queue, card)
		} else {
			detail.Candidates = append(detail.Candidates, card)
		}
	}
	return c.JSON(http.StatusOK, detail)
}

// loadUserGameCards fetches user_games with relations for a set of ids and
// returns them keyed by id, reusing the list-item DTO shape.
func (h *PoolsHandler) loadUserGameCards(ctx context.Context, ids []string) (map[string]userGameWithPlatformsResponse, error) {
	var userGames []models.UserGame
	if err := h.db.NewSelect().
		Model(&userGames).
		Where("user_game.id IN (?)", bun.List(ids)).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make(map[string]userGameWithPlatformsResponse, len(userGames))
	for _, ug := range userGames {
		out[ug.ID] = toUserGameWithPlatformsResponse(ug)
	}
	return out, nil
}

// addPoolGameRequest is the body for POST /api/pools/:id/games.
type addPoolGameRequest struct {
	UserGameID string `json:"user_game_id"`
}

// HandleAddPoolGame handles POST /api/pools/:id/games — insert as a Candidate.
// Idempotent: re-adding an existing member is a 200 no-op. Pools never create
// user_games; the user_game must already exist and belong to the user.
func (h *PoolsHandler) HandleAddPoolGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	var req addPoolGameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.UserGameID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_id is required")
	}
	ctx := context.Background()

	// Pool must exist and belong to the user.
	poolOK, err := h.db.NewSelect().Table("pools").
		Where("id = ? AND user_id = ?", poolID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !poolOK {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	// user_game must exist and belong to the user (owned OR wishlisted).
	ugOK, err := h.db.NewSelect().Table("user_games").
		Where("id = ? AND user_id = ?", req.UserGameID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !ugOK {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game not found")
	}

	// Insert as Candidate; idempotent on (pool_id, user_game_id).
	_, err = h.db.NewRaw(`
		INSERT INTO pool_games (id, pool_id, user_game_id, position, created_at)
		VALUES (?, ?, ?, NULL, now())
		ON CONFLICT (pool_id, user_game_id) DO NOTHING`,
		uuid.NewString(), poolID, req.UserGameID,
	).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleRemovePoolGame handles DELETE /api/pools/:id/games/:userGameId.
func (h *PoolsHandler) HandleRemovePoolGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")
	userGameID := c.Param("userGameId")

	res, err := h.db.NewRaw(`
		DELETE FROM pool_games
		WHERE pool_id = ? AND user_game_id = ?
		  AND pool_id IN (SELECT id FROM pools WHERE user_id = ?)`,
		poolID, userGameID, userID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleSetQueue handles PUT /api/pools/:id/queue — declarative queue state.
// Body {ids:[…ordered]} is the desired queued set: every id must already be a
// member (else 400); each listed id gets position = index; any member not in
// the list is demoted to Candidate (position NULL). Atomic.
func (h *PoolsHandler) HandleSetQueue(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	poolID := c.Param("id")

	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	ctx := context.Background()

	// Pool must exist and belong to the user.
	poolOK, err := h.db.NewSelect().Table("pools").
		Where("id = ? AND user_id = ?", poolID, userID).Exists(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if !poolOK {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Guard: every listed id must already be a member of this pool.
		if len(req.IDs) > 0 {
			var memberCount int
			if err := tx.NewSelect().Table("pool_games").
				Where("pool_id = ?", poolID).
				Where("user_game_id IN (?)", bun.List(req.IDs)).
				ColumnExpr("COUNT(*)").Scan(ctx, &memberCount); err != nil {
				return err
			}
			if memberCount != len(uniqueStrings(req.IDs)) {
				return errNotAllMembers
			}
		}
		// Demote everything to Candidate first.
		if _, err := tx.ExecContext(ctx,
			`UPDATE pool_games SET position = NULL WHERE pool_id = ?`, poolID,
		); err != nil {
			return err
		}
		// Promote listed ids to contiguous positions in order.
		for i, id := range req.IDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE pool_games SET position = ? WHERE pool_id = ? AND user_game_id = ?`,
				i, poolID, id,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errNotAllMembers) {
			return echo.NewHTTPError(http.StatusBadRequest, "all ids must already be pool members")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

var errNotAllMembers = errors.New("not all ids are members")

// uniqueStrings returns the distinct values of s, preserving first-seen order.
func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
```

Add `"github.com/drzero42/nexorious/internal/db/models"` to the imports of `internal/api/pools.go`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestPoolMembershipAndQueue -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/api/pools.go internal/api/pools_test.go
git commit -m "feat: add pool membership, queue, and detail endpoints"
```

---

## Task 8: `?pool=:id` suggestion / filtered view on the user-games list

**Files:**
- Modify: `internal/api/user_games.go` (`HandleListUserGames`, DTO)
- Test: `internal/api/pools_test.go` (append suggestion test)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/pools_test.go`:

```go
func TestPoolSuggestionView(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "suggest")

	// Two RPGs and one Shooter (genre metadata drives the filter).
	rpg1 := insertTestGameWithGenre(t, testDB, "RPG One", "Role-playing (RPG)")
	rpg2 := insertTestGameWithGenre(t, testDB, "RPG Two", "Role-playing (RPG)")
	shooter := insertTestGameWithGenre(t, testDB, "Shooter", "Shooter")
	insertTestUserGame(t, testDB, "ug-rpg1", userID, int(rpg1))
	insertTestUserGame(t, testDB, "ug-rpg2", userID, int(rpg2))
	insertTestUserGame(t, testDB, "ug-shooter", userID, int(shooter))

	// A finished RPG must never surface as a suggestion.
	rpgDone := insertTestGameWithGenre(t, testDB, "RPG Done", "Role-playing (RPG)")
	insertTestUserGame(t, testDB, "ug-rpgdone", userID, int(rpgDone))
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'completed' WHERE id = 'ug-rpgdone'`); err != nil {
		t.Fatalf("set completed: %v", err)
	}

	// Pool filtered to RPGs, with ug-rpg1 already a candidate member.
	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{
		"name": "RPG Pool",
		"filter": map[string]any{
			"filters": []any{map[string]any{"genre": []string{"Role-playing (RPG)"}}},
		},
	}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)
	postJSONAuth(t, e, "/api/pools/"+pool.ID+"/games", map[string]any{"user_game_id": "ug-rpg1"}, token)

	rec := getAuth(t, e, "/api/user-games?pool="+pool.ID, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		UserGames []struct {
			ID             string  `json:"id"`
			PoolMembership *string `json:"pool_membership"`
			Game           struct {
				Title string `json:"title"`
			} `json:"game"`
		} `json:"user_games"`
		Total int `json:"total"`
	}
	mustUnmarshal(t, rec, &resp)

	got := map[string]*string{}
	for _, ug := range resp.UserGames {
		got[ug.ID] = ug.PoolMembership
	}
	// RPG One and RPG Two match; Shooter and the finished RPG do not.
	if _, ok := got["ug-shooter"]; ok {
		t.Fatalf("shooter should not match RPG pool filter")
	}
	if _, ok := got["ug-rpgdone"]; ok {
		t.Fatalf("finished RPG must be excluded from suggestions")
	}
	if v, ok := got["ug-rpg1"]; !ok || v == nil || *v != "candidate" {
		t.Fatalf("ug-rpg1 should be a candidate member, got %v", got["ug-rpg1"])
	}
	if v, ok := got["ug-rpg2"]; !ok || v != nil {
		t.Fatalf("ug-rpg2 should match with null membership (a suggestion), got %v", got["ug-rpg2"])
	}
}

func TestPoolSuggestionNullFilterEmpty(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "nullfilter")
	gid := insertTestGame(t, testDB, "Lonely")
	insertTestUserGame(t, testDB, "ug-lonely", userID, int(gid))

	poolRec := postJSONAuth(t, e, "/api/pools", map[string]any{"name": "Manual"}, token)
	var pool struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, poolRec, &pool)

	rec := getAuth(t, e, "/api/user-games?pool="+pool.ID, token)
	var resp struct {
		Total int `json:"total"`
	}
	mustUnmarshal(t, rec, &resp)
	if resp.Total != 0 {
		t.Fatalf("expected empty result for NULL-filter pool, got total=%d", resp.Total)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run 'TestPoolSuggestion' -v`
Expected: FAIL — `?pool` is ignored, so Shooter/finished show up and `pool_membership` is absent.

- [ ] **Step 3: Add the `pool_membership` field to the list DTO**

In `internal/api/user_games.go`, add a field to `userGameWithPlatformsResponse`:

```go
type userGameWithPlatformsResponse struct {
	models.UserGame
	HoursPlayed    float64                    `json:"hours_played"`
	Platforms      []userGamePlatformResponse `json:"platforms"`
	PoolMembership *string                    `json:"pool_membership,omitempty"`
}
```

(`omitempty` keeps it absent on every existing list/detail response; it is only set when `?pool` is supplied.)

- [ ] **Step 4: Branch `HandleListUserGames` on `?pool`**

In `HandleListUserGames`, after `userID` is resolved and pagination/sort are parsed but **before** the `fb := filter.NewFilterBuilder()` line, insert the pool branch. Replace the filter-building block:

```go
	// Build filter.
	fb := filter.NewFilterBuilder()
	filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
	filter.ApplySearch(fb, c.QueryParam("q"))
	filter.ApplyWishlist(fb, c.QueryParam("wishlist") == "true")
	// … the is_loved / has_notes / rating / platform / … / time_to_beat block …
```

with a `poolID` branch around it:

```go
	poolID := c.QueryParam("pool")
	fb := filter.NewFilterBuilder()

	if poolID != "" {
		// Pool suggestion / filtered view: the pool's saved filter drives the
		// query (owned + wishlist), AND NOT finished. Ad-hoc facet params are
		// NOT merged in v1 (sort + pagination still honoured).
		var rawFilter json.RawMessage
		err := h.db.NewRaw(
			`SELECT filter FROM pools WHERE id = ? AND user_id = ?`, poolID, userID,
		).Scan(context.Background(), &rawFilter)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		if len(rawFilter) == 0 || string(rawFilter) == "null" {
			// Pure manual pool — no suggestions.
			return c.JSON(http.StatusOK, UserGameListResponse{
				UserGames: []userGameWithPlatformsResponse{},
				Total:     0, Page: page, PerPage: perPage, Pages: 1,
			})
		}
		pf, perr := filter.ParsePoolFilter(rawFilter)
		if perr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "invalid stored filter")
		}
		filter.ApplyPoolFilter(fb, pf)
		// Global finished-status exclusion (NULL stays eligible).
		fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("(ug.play_status IS NULL OR ug.play_status NOT IN (?))",
				bun.List(enum.FinishedPlayStatusStrings()))
		})
	} else {
		filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
		filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
		filter.ApplySearch(fb, c.QueryParam("q"))
		filter.ApplyWishlist(fb, c.QueryParam("wishlist") == "true")

		if str := c.QueryParam("is_loved"); str != "" {
			v := str == "true"
			filter.ApplyIsLoved(fb, &v)
		}
		if str := c.QueryParam("has_notes"); str != "" {
			v := str == "true"
			filter.ApplyHasNotes(fb, &v)
		}
		if str := c.QueryParam("rating_min"); str != "" {
			if v, err := strconv.ParseFloat(str, 64); err == nil {
				filter.ApplyRatingMin(fb, &v)
			}
		}
		if str := c.QueryParam("rating_max"); str != "" {
			if v, err := strconv.ParseFloat(str, 64); err == nil {
				filter.ApplyRatingMax(fb, &v)
			}
		}
		var ttbMin, ttbMax *float64
		if str := c.QueryParam("time_to_beat_min"); str != "" {
			if v, err := strconv.ParseFloat(str, 64); err == nil {
				ttbMin = &v
			}
		}
		if str := c.QueryParam("time_to_beat_max"); str != "" {
			if v, err := strconv.ParseFloat(str, 64); err == nil {
				ttbMax = &v
			}
		}
		filter.ApplyTimeToBeat(fb, ttbMin, ttbMax)
		filter.ApplyPlatform(fb, c.QueryParams()["platform"])
		filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
		filter.ApplyGenre(fb, c.QueryParams()["genre"])
		filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
		filter.ApplyTheme(fb, c.QueryParams()["theme"])
		filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
		filter.ApplyTag(fb, c.QueryParams()["tag"])
	}
```

> This moves the **existing** non-pool facet applications (including the `time_to_beat` block added in Task 3) into the `else` branch verbatim. Delete the original standalone copies of those lines so they are not applied twice. Ensure `enum` is imported in `user_games.go` (it already is).

- [ ] **Step 5: Populate `pool_membership` on the page DTOs**

In `HandleListUserGames`, after the `dtos := make([...]...)` loop that builds the response (just before the final `return c.JSON(...)`), add:

```go
	if poolID != "" && len(dtos) > 0 {
		pageIDs := make([]string, len(dtos))
		for i := range dtos {
			pageIDs[i] = dtos[i].ID
		}
		var members []poolMember
		if err := h.db.NewRaw(
			`SELECT user_game_id, position FROM pool_games WHERE pool_id = ? AND user_game_id IN (?)`,
			poolID, bun.List(pageIDs),
		).Scan(ctx, &members); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		membership := make(map[string]string, len(members))
		for _, m := range members {
			if m.Position != nil {
				membership[m.UserGameID] = "queued"
			} else {
				membership[m.UserGameID] = "candidate"
			}
		}
		for i := range dtos {
			if state, ok := membership[dtos[i].ID]; ok {
				s := state
				dtos[i].PoolMembership = &s
			}
		}
	}
```

(`poolMember` is defined in `pools.go`, same package, so it is reused here.)

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/api/... -run 'TestPoolSuggestion' -v`
Expected: PASS (both tests).

- [ ] **Step 7: Run the full pools + user-games test set**

Run: `go test ./internal/api/... -run 'TestPool|TestCompletionRemovesFromPools|TestListUserGames' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/user_games.go internal/api/pools_test.go
git commit -m "feat: add pool suggestion view to the user-games list endpoint"
```

---

## Final verification

- [ ] **Step 1: Full backend build + lint + tests**

Run: `go build ./... && golangci-lint run && go test -timeout 600s ./...`
Expected: clean build, no lint errors, all tests pass. (Watch for `errcheck` on any new `_ =` discards and `gosec` on raw SQL — all SQL here is parameterised, so no G201 findings expected.)

- [ ] **Step 2: Confirm migration down is reversible (optional manual check)**

Run: `./nexorious migrate status` against a scratch DB after `make build`, or rely on the test container applying the up migration in `TestMain`. Down is a plain `DROP TABLE` pair.

- [ ] **Step 3: Push (triggers the pre-push full-suite gate)**

```bash
git push -u origin feat/play-planning-backend-955
```

---

## Spec coverage self-check

- **Schema (pools, pool_games, unique/index)** → Task 1 ✓
- **Bun models** → Task 1 ✓
- **Finished set single source of truth** → Task 2 ✓
- **`ApplyTimeToBeat` + wired into library** → Task 3 ✓
- **`RemoveFromPoolsIfFinished` + single + bulk wiring, idempotent, no renumber** → Task 4 ✓
- **`PoolFilter`/`FilterCard`, strict parse (unknown keys rejected), OR-of-cards, finished exclusion outside** → Task 5 (machinery) + Task 8 (caller applies finished exclusion) ✓
- **Pool CRUD: list with counts, create (append max+1, filter validation), get-with-members, update, delete cascade, reorder** → Tasks 6 & 7 ✓
- **Filter validation: omitted/null→NULL, empty filters→NULL, empty card→400, unknown key→400** → Task 6 ✓
- **Membership: add as Candidate, idempotent 200, reject non-existent/other-user user_game, never create user_games; remove 404 when not a member** → Task 7 ✓
- **Queue `PUT`: declarative, member-must-exist (400), demote-absent, renumber contiguous** → Task 7 ✓
- **Suggestion view `?pool=:id`: pool filter applied, finished excluded, owned+wishlist, `pool_membership` null/candidate/queued, NULL-filter→empty, ad-hoc facets not merged (v1)** → Task 8 ✓
- **Route ordering (`/reorder` before `/:id`)** → Task 6 ✓

**Deviation from the spec's test plan:** the spec lists unit tests in `internal/filter`. That package has **no test harness** and every existing filter is tested through the API list endpoint. To stay consistent with the codebase (and avoid standing up a second container harness), `ApplyTimeToBeat` and `ApplyPoolFilter` are tested through the HTTP endpoints that exercise them (Tasks 3 and 8). Behaviour coverage is equivalent.
