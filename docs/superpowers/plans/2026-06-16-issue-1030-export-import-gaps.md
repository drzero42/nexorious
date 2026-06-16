# JSON export/import round-trip gaps (#1030) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Nexorious JSON export (`nexorious-library`) and its importer a complete round-trip for three currently-dropped user fields: `is_wishlisted`, per-platform `is_available`, and Play Planning pools/queue.

**Architecture:** The JSON restore runs `HandleImportNexorious` → the legacy `ImportItemWorker` path (symmetric pair of `buildJSONDoc`/`exportGameJSON`). The two scalar fields are symmetric field-adds on both sides. Pools are a new top-level export section; on import they are stashed in a synthetic, pre-completed `job_item` and applied once at the job-completion transition (after every game's `user_game` exists), find-or-created by `(user_id, name)` and merged additively. Format version bumps `2.0 → 2.1`; the importer accepts both and keys behaviour off field presence.

**Tech Stack:** Go, Bun ORM, River queue, PostgreSQL, testcontainers-go.

**Spec:** `docs/superpowers/specs/2026-06-16-issue-1030-export-import-gaps-design.md`

---

## File structure

- `internal/worker/tasks/export.go` — JSON export: add scalar fields, version bump, pools types + `loadPoolsForExport`, wire pools into `buildJSONDoc`/`writeJSONExport`/`ExportJSONWorker.Work`.
- `internal/worker/tasks/import_item.go` — legacy importer: add scalar fields to `importGameData`/`importPlatformData`, apply them; add `PoolsItemKey`, `importPoolData`, `applyImportedPools`, `findOrCreatePool`; hook pools into `checkJobCompletion`.
- `internal/worker/tasks/testexports.go` — update `BuildJSONDocForTest` signature, add `LoadPoolsForExportForTest`.
- `internal/api/import.go` — accept versions `2.0`+`2.1`; parse `pools`; create synthetic `__pools__` item.
- `internal/api/jobs.go` — exclude `__pools__` from `HandleGetJobItems` list + count.
- Tests: `internal/worker/tasks/export_test.go` (new or existing), `internal/worker/tasks/import_item_test.go`, `internal/worker/tasks/import_roundtrip_test.go`, `internal/api/import_test.go`.

**No migration** — every column already exists; `job_items.source_metadata` is `jsonb`.

---

## Task 1: Export — version 2.1 + `is_wishlisted` and `is_available` scalar fields

**Files:**
- Modify: `internal/worker/tasks/export.go` (`exportGameJSON`, `exportPlatformJSON`, `buildJSONDoc`)
- Test: `internal/worker/tasks/export_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/worker/tasks/export_test.go` (if it already exists, append the function):

```go
package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestBuildJSONDoc_ScalarFieldsAndVersion(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	g := &models.Game{ID: 4242, Title: "Wishlist Game", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	ug := &models.UserGame{
		ID: uuid.NewString(), UserID: userID, GameID: 4242, IsWishlisted: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := testDB.NewInsert().Model(ug).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}
	plat := "pc-windows"
	ugp := &models.UserGamePlatform{
		ID: uuid.NewString(), UserGameID: ug.ID, Platform: &plat, IsAvailable: false,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := testDB.NewInsert().Model(ugp).Exec(ctx); err != nil {
		t.Fatalf("insert ugp: %v", err)
	}

	ugs, err := tasks.LoadUserGamesWithRelationsForTest(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	doc := tasks.BuildJSONDocForTest(ugs, nil)

	if doc.Version != "2.1" {
		t.Errorf("version = %q, want 2.1", doc.Version)
	}
	if len(doc.Games) != 1 {
		t.Fatalf("games = %d, want 1", len(doc.Games))
	}
	if !doc.Games[0].IsWishlisted {
		t.Errorf("is_wishlisted = false, want true")
	}
	if len(doc.Games[0].Platforms) != 1 {
		t.Fatalf("platforms = %d, want 1", len(doc.Games[0].Platforms))
	}
	if doc.Games[0].Platforms[0].IsAvailable {
		t.Errorf("is_available = true, want false")
	}
}
```

Note: this test passes `nil` for the new `pools` parameter of `BuildJSONDocForTest`, which is introduced in Task 2. To keep Task 1 self-contained and compiling, **temporarily** call `tasks.BuildJSONDocForTest(ugs)` in this step, then update it to `(ugs, nil)` in Task 2 Step 1. (If you are executing tasks in order, simplest is to write `tasks.BuildJSONDocForTest(ugs)` now.)

Use `tasks.BuildJSONDocForTest(ugs)` for this task.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestBuildJSONDoc_ScalarFieldsAndVersion -v`
Expected: FAIL — `doc.Games[0].IsWishlisted` / `.Platforms[0].IsAvailable` undefined (fields don't exist), or version mismatch (`2.0`).

- [ ] **Step 3: Add the fields and bump the version**

In `internal/worker/tasks/export.go`:

Add to `exportGameJSON` (after `IsLoved`):
```go
	IsWishlisted   bool                 `json:"is_wishlisted"`
```

Add to `exportPlatformJSON` (after `HoursPlayed`):
```go
	IsAvailable     bool     `json:"is_available"`
```

In `buildJSONDoc`, set the platform field where `pj` is built:
```go
			pj := exportPlatformJSON{
				Platform:        p.Platform,
				Storefront:      p.Storefront,
				OwnershipStatus: p.OwnershipStatus,
				HoursPlayed:     p.HoursPlayed,
				IsAvailable:     p.IsAvailable,
			}
```

In `buildJSONDoc`, set the game field in the `exportGameJSON{...}` literal (after `IsLoved: ug.IsLoved,`):
```go
			IsWishlisted:   ug.IsWishlisted,
```

In `buildJSONDoc`'s returned `exportDocJSON`, change the version:
```go
		Version:    "2.1",
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run TestBuildJSONDoc_ScalarFieldsAndVersion -v`
Expected: PASS

- [ ] **Step 5: Check for existing export tests asserting version 2.0**

Run: `grep -rn '"2.0"\|version.*2\.0\|Version' internal/worker/tasks/*_test.go`
Any test asserting the *export* doc version equals `2.0` must be updated to `2.1`. (Import-side tests posting `version: "2.0"` are fine — the importer still accepts 2.0; do not change those.) Fix any export-side assertions you find.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/export_test.go
git commit -m "feat: export is_wishlisted + is_available, bump JSON format to 2.1 (#1030)"
```

---

## Task 2: Export — pools section

**Files:**
- Modify: `internal/worker/tasks/export.go` (new types, `loadPoolsForExport`, `buildJSONDoc`, `writeJSONExport`, `ExportJSONWorker.Work`, `exportDocJSON`)
- Modify: `internal/worker/tasks/testexports.go` (bridge signature + new bridge)
- Modify: `internal/worker/tasks/import_roundtrip_test.go` (existing `BuildJSONDocForTest(ugs)` call → `(ugs, nil)`)
- Test: `internal/worker/tasks/export_test.go`

- [ ] **Step 1: Update the `BuildJSONDocForTest` call sites to the new 2-arg form**

The bridge gains a `pools` parameter in Step 4. Update existing callers now so the package compiles:
- `internal/worker/tasks/import_roundtrip_test.go:82` → `doc := tasks.BuildJSONDocForTest(ugs, nil)`
- `internal/worker/tasks/export_test.go` (Task 1) → `doc := tasks.BuildJSONDocForTest(ugs, nil)`

- [ ] **Step 2: Write the failing test**

Append to `internal/worker/tasks/export_test.go`:

```go
func TestLoadPoolsForExport_TranslatesMembershipToIGDBID(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	for _, id := range []int32{5001, 5002} {
		g := &models.Game{ID: id, Title: "G", LastUpdated: time.Now(), CreatedAt: time.Now()}
		if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
			t.Fatalf("insert game: %v", err)
		}
	}
	ug1 := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: 5001, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	ug2 := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: 5002, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ug1).Exec(ctx); err != nil {
		t.Fatalf("insert ug1: %v", err)
	}
	if _, err := testDB.NewInsert().Model(ug2).Exec(ctx); err != nil {
		t.Fatalf("insert ug2: %v", err)
	}
	poolID := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pools (id, user_id, name, color, position, filter) VALUES (?, ?, 'Backlog', '#abc', 0, '{"loved":true}')`,
		poolID, userID); err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	// ug1 = Candidate (NULL position); ug2 = queued at position 0.
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, NULL)`,
		uuid.NewString(), poolID, ug1.ID); err != nil {
		t.Fatalf("insert pg1: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, 0)`,
		uuid.NewString(), poolID, ug2.ID); err != nil {
		t.Fatalf("insert pg2: %v", err)
	}

	pools, err := tasks.LoadPoolsForExportForTest(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("loadPoolsForExport: %v", err)
	}
	doc := tasks.BuildJSONDocForTest(nil, pools)
	if len(doc.Pools) != 1 {
		t.Fatalf("pools = %d, want 1", len(doc.Pools))
	}
	p := doc.Pools[0]
	if p.Name != "Backlog" {
		t.Errorf("name = %q, want Backlog", p.Name)
	}
	if len(p.Games) != 2 {
		t.Fatalf("members = %d, want 2", len(p.Games))
	}
	// Queued member (position set) sorts before the Candidate (NULL position last).
	if p.Games[0].IGDBID != 5002 || p.Games[0].Position == nil || *p.Games[0].Position != 0 {
		t.Errorf("member0 = %+v, want igdb 5002 @ pos 0", p.Games[0])
	}
	if p.Games[1].IGDBID != 5001 || p.Games[1].Position != nil {
		t.Errorf("member1 = %+v, want igdb 5001 candidate", p.Games[1])
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestLoadPoolsForExport -v`
Expected: FAIL — `tasks.LoadPoolsForExportForTest` / `doc.Pools` undefined.

- [ ] **Step 4: Add the pools types, loader, and wiring**

In `internal/worker/tasks/export.go`, add the types near `exportTagJSON`:
```go
type exportPoolJSON struct {
	Name     string               `json:"name"`
	Color    *string              `json:"color"`
	Position int                  `json:"position"`
	Filter   json.RawMessage      `json:"filter,omitempty"`
	Games    []exportPoolGameJSON `json:"games"`
}

type exportPoolGameJSON struct {
	IGDBID   int32 `json:"igdb_id"`
	Position *int  `json:"position"`
}
```

Add `Pools` to `exportDocJSON`:
```go
type exportDocJSON struct {
	Format     string           `json:"format"`
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Games      []exportGameJSON `json:"games"`
	Pools      []exportPoolJSON `json:"pools"`
}
```

Add the loader (uses `models.Pool`; member query uses explicit bun column tags per the raw-scan gotcha):
```go
// loadPoolsForExport returns the user's Play Planning pools with each membership
// translated from the opaque user_game_id to the game's igdb_id (the stable
// cross-instance key). Queued members (position set) sort before Candidates.
func loadPoolsForExport(ctx context.Context, db *bun.DB, userID string) ([]exportPoolJSON, error) {
	var pools []models.Pool
	if err := db.NewSelect().Model(&pools).
		Where("user_id = ?", userID).Order("position").Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]exportPoolJSON, 0, len(pools))
	for _, p := range pools {
		var members []struct {
			Position *int  `bun:"position"`
			GameID   int32 `bun:"game_id"`
		}
		if err := db.NewRaw(
			`SELECT pg.position AS position, ug.game_id AS game_id
			 FROM pool_games pg JOIN user_games ug ON ug.id = pg.user_game_id
			 WHERE pg.pool_id = ?
			 ORDER BY pg.position NULLS LAST, ug.game_id`, p.ID,
		).Scan(ctx, &members); err != nil {
			return nil, err
		}
		games := make([]exportPoolGameJSON, 0, len(members))
		for _, m := range members {
			games = append(games, exportPoolGameJSON{IGDBID: m.GameID, Position: m.Position})
		}
		var filter json.RawMessage
		if len(p.Filter) > 0 {
			filter = p.Filter
		}
		out = append(out, exportPoolJSON{
			Name: p.Name, Color: p.Color, Position: p.Position, Filter: filter, Games: games,
		})
	}
	return out, nil
}
```

Change `buildJSONDoc` to accept pools and emit them:
```go
func buildJSONDoc(ugs []models.UserGame, pools []exportPoolJSON) exportDocJSON {
```
and in its returned literal add:
```go
		Games:      games,
		Pools:      pools,
```

Change `writeJSONExport` to accept and forward pools:
```go
func writeJSONExport(storagePath, userID string, ugs []models.UserGame, pools []exportPoolJSON) (string, error) {
	...
	doc := buildJSONDoc(ugs, pools)
	...
}
```

In `ExportJSONWorker.Work`, load pools and pass them:
```go
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	pools, err := loadPoolsForExport(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load pools: %v", err))
		return nil
	}
	outPath, err := writeJSONExport(w.StoragePath, j.UserID, userGames, pools)
```

- [ ] **Step 5: Update the test bridge**

In `internal/worker/tasks/testexports.go`:
```go
// BuildJSONDocForTest exposes buildJSONDoc for cross-package tests.
func BuildJSONDocForTest(ugs []models.UserGame, pools []exportPoolJSON) exportDocJSON {
	return buildJSONDoc(ugs, pools)
}

// LoadPoolsForExportForTest exposes loadPoolsForExport for cross-package tests.
func LoadPoolsForExportForTest(ctx context.Context, db *bun.DB, userID string) ([]exportPoolJSON, error) {
	return loadPoolsForExport(ctx, db, userID)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/worker/tasks/ -run 'TestLoadPoolsForExport|TestBuildJSONDoc' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/testexports.go internal/worker/tasks/export_test.go internal/worker/tasks/import_roundtrip_test.go
git commit -m "feat: export Play Planning pools in JSON format (#1030)"
```

---

## Task 3: Import — accept format version 2.0 and 2.1

**Files:**
- Modify: `internal/api/import.go` (`HandleImportNexorious` version gate)
- Test: `internal/api/import_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/import_test.go` (follows the setup pattern of the existing `TestImportNexorious_*` tests):

```go
func TestImportNexorious_AcceptsVersion21(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "imp-v21")

	export := map[string]any{
		"format":  "nexorious-library",
		"version": "2.1",
		"games":   []map[string]any{{"igdb_id": 1, "title": "Game 1"}},
	}
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
```

Note: the existing `TestImportNexorious_WrongVersion` asserts the error message still contains the substring `"2.0"`. The Step 3 message ("versions 2.0 and 2.1") preserves that substring, so that test stays green — do not change it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestImportNexorious_AcceptsVersion21 -v`
Expected: FAIL — status 400 "Unsupported import file. Only Nexorious library format version 2.0 is supported."

- [ ] **Step 3: Relax the version gate**

In `internal/api/import.go`, replace the version check block:
```go
	if export.Version != "2.0" {
		msg := "Unsupported import file. Only Nexorious library format version 2.0 is supported."
		if export.ExportVersion != "" {
			msg = fmt.Sprintf("Unsupported legacy export (version %s). Only Nexorious library format version 2.0 is supported.", export.ExportVersion)
		}
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}
```
with:
```go
	if export.Version != "2.0" && export.Version != "2.1" {
		msg := "Unsupported import file. Only Nexorious library format versions 2.0 and 2.1 are supported."
		if export.ExportVersion != "" {
			msg = fmt.Sprintf("Unsupported legacy export (version %s). Only Nexorious library format versions 2.0 and 2.1 are supported.", export.ExportVersion)
		}
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestImportNexorious_AcceptsVersion21 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/import.go internal/api/import_test.go
git commit -m "feat: accept nexorious import format version 2.1 (#1030)"
```

---

## Task 4: Import — apply `is_wishlisted` and `is_available`

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (`importGameData`, `importPlatformData`, `ImportItemWorker.Work`)
- Test: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/worker/tasks/import_item_test.go`:

```go
func TestImportItem_AppliesWishlistAndAvailability(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	g := &models.Game{ID: 6001, Title: "Avail", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, userID, 1)

	// Wishlisted game with one platform marked unavailable.
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{
		"igdb_id":       6001,
		"title":         "Avail",
		"is_wishlisted": true,
		"platforms": []map[string]any{
			{"platform": "pc-windows", "is_available": false},
		},
	})

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("work: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(6001)).Scan(ctx); err != nil {
		t.Fatalf("user_game: %v", err)
	}
	// A wishlisted game DOES carry a platform here, so ClearWishlistOnAcquire clears it.
	if ug.IsWishlisted {
		t.Errorf("is_wishlisted = true, want false (cleared on acquire because a platform exists)")
	}
	var ugp models.UserGamePlatform
	if err := testDB.NewSelect().Model(&ugp).Where("user_game_id = ?", ug.ID).Scan(ctx); err != nil {
		t.Fatalf("platform: %v", err)
	}
	if ugp.IsAvailable {
		t.Errorf("is_available = true, want false")
	}
}

func TestImportItem_WishlistSurvivesWithoutPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	g := &models.Game{ID: 6002, Title: "Wished", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, userID, 1)
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{
		"igdb_id":       6002,
		"title":         "Wished",
		"is_wishlisted": true,
		"platforms":     []map[string]any{},
	})

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("work: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(6002)).Scan(ctx); err != nil {
		t.Fatalf("user_game: %v", err)
	}
	if !ug.IsWishlisted {
		t.Errorf("is_wishlisted = false, want true (no platforms ⇒ flag survives)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/worker/tasks/ -run 'TestImportItem_AppliesWishlistAndAvailability|TestImportItem_WishlistSurvivesWithoutPlatforms' -v`
Expected: FAIL — `is_available` ignored (always true) and `is_wishlisted` never set (always false).

- [ ] **Step 3: Add the fields and apply them**

In `internal/worker/tasks/import_item.go`:

Add to `importGameData` (after `IsLoved`):
```go
	IsWishlisted   bool                 `json:"is_wishlisted"`
```

Add to `importPlatformData` (after `HoursPlayed`):
```go
	IsAvailable     *bool    `json:"is_available"`
```

In `Work`, in the new-`UserGame` literal (after `IsLoved: gd.IsLoved,`):
```go
			IsWishlisted:   gd.IsWishlisted,
```

In `Work`, in the `UserGamePlatform` literal, replace the hard-coded availability:
```go
			IsAvailable:     pd.IsAvailable == nil || *pd.IsAvailable, // absent ⇒ available; sync re-derives
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/worker/tasks/ -run 'TestImportItem_AppliesWishlistAndAvailability|TestImportItem_WishlistSurvivesWithoutPlatforms' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go
git commit -m "feat: import is_wishlisted + is_available from JSON (#1030)"
```

---

## Task 5: Import — apply pools at the completion transition

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (`PoolsItemKey`, `importPoolData`, `importPoolGameData`, `applyImportedPools`, `findOrCreatePool`, `checkJobCompletion`)
- Test: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/worker/tasks/import_item_test.go`:

```go
func TestImportItem_AppliesPoolsOnCompletion(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	for _, id := range []int32{7001, 7002} {
		g := &models.Game{ID: id, Title: "G", LastUpdated: time.Now(), CreatedAt: time.Now()}
		if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
			t.Fatalf("insert game: %v", err)
		}
	}
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, userID, 2)

	// Two game items.
	item1 := insertTestJobItem(t, testDB, jobID, userID, map[string]any{"igdb_id": 7001, "title": "G1"})
	item2 := insertTestJobItem(t, testDB, jobID, userID, map[string]any{"igdb_id": 7002, "title": "G2"})

	// Synthetic pools item (mirrors what HandleImportNexorious writes).
	poolsPayload := []map[string]any{{
		"name":     "Backlog",
		"color":    "#abc",
		"position": 0,
		"filter":   map[string]any{"loved": true},
		"games": []map[string]any{
			{"igdb_id": 7001, "position": nil},
			{"igdb_id": 7002, "position": 0},
		},
	}}
	insertTestPoolsItem(t, testDB, jobID, userID, poolsPayload)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	for _, id := range []string{item1, item2} {
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: id}}); err != nil {
			t.Fatalf("work: %v", err)
		}
	}

	var poolName string
	var memberCount int
	if err := testDB.NewRaw(`SELECT name FROM pools WHERE user_id = ? AND name = 'Backlog'`, userID).Scan(ctx, &poolName); err != nil {
		t.Fatalf("pool not created: %v", err)
	}
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM pool_games pg JOIN pools p ON p.id = pg.pool_id WHERE p.user_id = ?`, userID,
	).Scan(ctx, &memberCount); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if memberCount != 2 {
		t.Errorf("pool members = %d, want 2", memberCount)
	}
}
```

Add this helper to `internal/worker/tasks/main_test.go` (next to `insertTestJobItem`):
```go
func insertTestPoolsItem(t *testing.T, db *bun.DB, jobID, userID string, pools any) {
	t.Helper()
	sourceMetadata := mustMarshal(t, map[string]any{"item_type": "pools", "data": pools})
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, ?, ?, ?, 'completed', '{}', '[]')`,
		uuid.NewString(), jobID, userID, tasks.PoolsItemKey, "(pools)", sourceMetadata,
	)
	if err != nil {
		t.Fatalf("insertTestPoolsItem: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestImportItem_AppliesPoolsOnCompletion -v`
Expected: FAIL — `tasks.PoolsItemKey` undefined (compile error), then once that compiles, no pool created.

- [ ] **Step 3: Add the sentinel, types, and pools application**

In `internal/worker/tasks/import_item.go`, add near the top (after imports):
```go
// PoolsItemKey is the sentinel item_key of the synthetic job_item that carries
// the Play Planning pools payload for a Nexorious JSON import. It is created
// pre-completed and applied once at the job-completion transition; it is not a
// game item and is excluded from per-job item listings.
const PoolsItemKey = "__pools__"

type importPoolData struct {
	Name     string               `json:"name"`
	Color    *string              `json:"color"`
	Position int                  `json:"position"`
	Filter   json.RawMessage      `json:"filter"`
	Games    []importPoolGameData `json:"games"`
}

type importPoolGameData struct {
	IGDBID   int32 `json:"igdb_id"`
	Position *int  `json:"position"`
}
```

Add the application functions (anywhere in the file, e.g. after `findOrCreateTag`):
```go
// applyImportedPools reads the synthetic pools job_item for a finished import job
// and applies its pools additively: find-or-create each pool by (user_id, name),
// then attach members resolved from igdb_id to the user's user_games. It is
// best-effort — a per-pool or per-member failure is logged and skipped, never
// failing the job. Safe to call only on the single job-completion transition.
func applyImportedPools(ctx context.Context, db *bun.DB, jobID, userID string) {
	var item models.JobItem
	err := db.NewSelect().Model(&item).
		Where("job_id = ? AND item_key = ?", jobID, PoolsItemKey).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "import: load pools item", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	var wrapper struct {
		Data []importPoolData `json:"data"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &wrapper); err != nil {
		slog.WarnContext(ctx, "import: parse pools payload", logging.KeyErr, err, logging.Cat(logging.CategoryValidation))
		return
	}
	for _, p := range wrapper.Data {
		poolID, err := findOrCreatePool(ctx, db, userID, p)
		if err != nil {
			slog.WarnContext(ctx, "import: find/create pool", logging.KeyErr, err, "pool", p.Name, logging.Cat(logging.CategoryDB))
			continue
		}
		for _, m := range p.Games {
			var ugID string
			if err := db.NewRaw(
				`SELECT id FROM user_games WHERE user_id = ? AND game_id = ?`, userID, m.IGDBID,
			).Scan(ctx, &ugID); err != nil {
				// Game absent (failed import) or lookup error: skip this member.
				continue
			}
			if _, err := db.NewRaw(
				`INSERT INTO pool_games (id, pool_id, user_game_id, position, created_at)
				 VALUES (?, ?, ?, ?, now())
				 ON CONFLICT (pool_id, user_game_id) DO NOTHING`,
				uuid.NewString(), poolID, ugID, m.Position,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "import: insert pool_game", logging.KeyErr, err, "pool", p.Name, logging.Cat(logging.CategoryDB))
			}
		}
	}
}

// findOrCreatePool returns the id of the user's pool named p.Name, creating it
// (with the imported color/filter and next position) if absent. An existing
// pool's curation is never overwritten — only its id is returned.
func findOrCreatePool(ctx context.Context, db *bun.DB, userID string, p importPoolData) (string, error) {
	var filterArg any
	if len(p.Filter) > 0 {
		filterArg = string(p.Filter)
	}
	if _, err := db.NewRaw(
		`INSERT INTO pools (id, user_id, name, color, position, filter, created_at, updated_at)
		 VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM pools WHERE user_id = ?), 0), ?, now(), now())
		 ON CONFLICT (user_id, name) DO NOTHING`,
		uuid.NewString(), userID, p.Name, p.Color, userID, filterArg,
	).Exec(ctx); err != nil {
		return "", err
	}
	var poolID string
	if err := db.NewRaw(
		`SELECT id FROM pools WHERE user_id = ? AND name = ?`, userID, p.Name,
	).Scan(ctx, &poolID); err != nil {
		return "", err
	}
	return poolID, nil
}
```

Hook into `checkJobCompletion` — replace the tail of the function:
```go
	finalizeJobCompleted(ctx, db, jobID, "import_item: update job status", false)

	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
```
with:
```go
	finalized := finalizeJobCompleted(ctx, db, jobID, "import_item: update job status", false)

	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
	if finalized {
		applyImportedPools(ctx, db, jobID, uid)
	}
```

(The `errors`, `database/sql`, `encoding/json`, `log/slog`, `github.com/google/uuid`, and `logging` imports are already present in this file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run TestImportItem_AppliesPoolsOnCompletion -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go internal/worker/tasks/main_test.go
git commit -m "feat: apply imported Play Planning pools at job completion (#1030)"
```

---

## Task 6: Import handler — create the synthetic pools item; hide it from listings

**Files:**
- Modify: `internal/api/import.go` (`nexoriousExport`, `HandleImportNexorious`)
- Modify: `internal/api/jobs.go` (`HandleGetJobItems` list + count)
- Test: `internal/api/import_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/import_test.go`:

```go
func TestImportNexorious_CreatesPoolsItemHiddenFromListing(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "imp-pools")

	export := map[string]any{
		"format":  "nexorious-library",
		"version": "2.1",
		"games":   []map[string]any{{"igdb_id": 1, "title": "Game 1"}},
		"pools": []map[string]any{{
			"name":     "Backlog",
			"position": 0,
			"games":    []map[string]any{{"igdb_id": 1, "position": nil}},
		}},
	}
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	jobID, _ := resp["job_id"].(string)
	if jobID == "" {
		t.Fatal("no job_id in response")
	}

	// total_items counts games only, not the synthetic pools item.
	if got := resp["total_items"]; got != float64(1) {
		t.Errorf("total_items = %v, want 1", got)
	}

	// The synthetic __pools__ item exists in the DB (shared api-test handle is testDB).
	var raw int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND item_key = '__pools__'`, jobID,
	).Scan(context.Background(), &raw); err != nil {
		t.Fatalf("count raw: %v", err)
	}
	if raw != 1 {
		t.Errorf("synthetic pools items = %d, want 1", raw)
	}
}
```

(`internal/api/main_test.go` declares `var testDB *bun.DB`; the import job is created synchronously by the handler, so the `__pools__` row is queryable immediately after the request returns. The listing-exclusion in Step 4 is additionally covered by the round-trip/`HandleGetJobItems` behaviour; if you prefer an HTTP-level assertion, `GET /api/jobs/<jobID>/items` should return exactly the 1 game item.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestImportNexorious_CreatesPoolsItemHiddenFromListing -v`
Expected: FAIL — no `__pools__` row created (pools not parsed/persisted yet).

- [ ] **Step 3: Parse pools and write the synthetic item**

In `internal/api/import.go`, add the field to `nexoriousExport`:
```go
type nexoriousExport struct {
	Version       string            `json:"version"`
	ExportVersion string            `json:"export_version"` // legacy 1.x key, used only for error messages
	Games         []json.RawMessage `json:"games"`
	Pools         json.RawMessage   `json:"pools"`
}
```

In `HandleImportNexorious`, insert the pools block **before the games loop** (right after `reqCtx := c.Request().Context()` and before `var skipCount int`). It MUST precede any River enqueue: the legacy `checkJobCompletion` does not gate on `dispatch_complete`, so a fast worker could finalize the job before a later-inserted pools item exists, losing the pools. Creating it first guarantees it is present when the final game item triggers completion.
```go
	// Stash any pools section as a synthetic, pre-completed job_item. It carries
	// no River task and is applied once at the job-completion transition, after
	// every game's user_game exists. It is NOT counted in total_items. Inserted
	// before any game task is enqueued so it is present at completion time.
	if len(export.Pools) > 0 && string(export.Pools) != "null" {
		poolsMeta, err := json.Marshal(map[string]any{"item_type": "pools", "data": export.Pools})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to build pools item metadata")
		}
		poolsItem := &models.JobItem{
			ID:             uuid.NewString(),
			JobID:          job.ID,
			UserID:         userID,
			ItemKey:        tasks.PoolsItemKey,
			SourceTitle:    "(pools)",
			SourceMetadata: poolsMeta,
			Status:         models.JobItemStatusCompleted,
			Result:         json.RawMessage(`{}`),
			IGDBCandidates: json.RawMessage(`[]`),
		}
		if _, err := h.db.NewInsert().Model(poolsItem).Exec(ctx); err != nil {
			slog.ErrorContext(reqCtx, "import: create pools item", logging.KeyJobID, job.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
	}
```

- [ ] **Step 4: Exclude the synthetic item from the per-job item listing**

In `internal/api/jobs.go`, in `HandleGetJobItems`, add the sentinel filter to both the list and count builders (around lines 529–530):
```go
	q := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID).Where("item_key <> ?", tasks.PoolsItemKey)
	countQ := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID).Where("item_key <> ?", tasks.PoolsItemKey)
```
(`internal/api/jobs.go` already imports `internal/worker/tasks`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestImportNexorious_CreatesPoolsItemHiddenFromListing|TestImportNexorious_AcceptsVersion21' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/import.go internal/api/jobs.go internal/api/import_test.go
git commit -m "feat: persist imported pools as synthetic job_item, hide from listing (#1030)"
```

---

## Task 7: Full round-trip + backward-compat + merge coverage

**Files:**
- Modify: `internal/worker/tasks/import_roundtrip_test.go`
- Test: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Extend the round-trip test to cover all three gaps**

In `internal/worker/tasks/import_roundtrip_test.go`, extend `TestImport_RoundTripPreservesUserData`:

(a) Change the source platform to be unavailable — in the `ugp` literal change `IsAvailable: true` to `IsAvailable: false`.

(b) After the existing source `ugp`/tag inserts, add a second game, a wishlisted (platform-less) game, and a pool:
```go
	// Second game, plus a platform-less wishlisted game.
	game2 := &models.Game{ID: 7778, Title: "Queued", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(game2).Exec(ctx); err != nil {
		t.Fatalf("insert game2: %v", err)
	}
	ug2 := &models.UserGame{ID: uuid.NewString(), UserID: srcUser, GameID: 7778, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ug2).Exec(ctx); err != nil {
		t.Fatalf("insert ug2: %v", err)
	}
	gameW := &models.Game{ID: 7779, Title: "Wished", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(gameW).Exec(ctx); err != nil {
		t.Fatalf("insert gameW: %v", err)
	}
	ugW := &models.UserGame{ID: uuid.NewString(), UserID: srcUser, GameID: 7779, IsWishlisted: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ugW).Exec(ctx); err != nil {
		t.Fatalf("insert ugW: %v", err)
	}
	// Pool: ug (7777) Candidate, ug2 (7778) queued at 0.
	poolID := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pools (id, user_id, name, color, position, filter) VALUES (?, ?, 'Backlog', '#abc', 0, '{"loved":true}')`,
		poolID, srcUser); err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, NULL)`, uuid.NewString(), poolID, ug.ID); err != nil {
		t.Fatalf("insert pg candidate: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, 0)`, uuid.NewString(), poolID, ug2.ID); err != nil {
		t.Fatalf("insert pg queued: %v", err)
	}
```

(c) Change the export call to load and pass pools:
```go
	pools, err := tasks.LoadPoolsForExportForTest(ctx, testDB, srcUser)
	if err != nil {
		t.Fatalf("load pools: %v", err)
	}
	doc := tasks.BuildJSONDocForTest(ugs, pools)
	if len(doc.Games) != 3 {
		t.Fatalf("expected 3 exported games, got %d", len(doc.Games))
	}
```

(d) After importing the games, write the synthetic pools item and re-run completion so pools apply. The simplest faithful approach: insert each game item, run it, then before the final item insert the synthetic pools item so the last `checkJobCompletion` applies it. Replace the import loop + `insertTestJob` call:
```go
	dstUser := uuid.NewString()
	insertTestUser(t, testDB, dstUser)
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, dstUser, len(doc.Games))

	// Stash pools BEFORE running items so the completion transition applies them.
	poolsPayload := make([]map[string]any, 0, len(doc.Pools))
	for _, p := range doc.Pools {
		members := make([]map[string]any, 0, len(p.Games))
		for _, m := range p.Games {
			members = append(members, map[string]any{"igdb_id": m.IGDBID, "position": m.Position})
		}
		poolsPayload = append(poolsPayload, map[string]any{
			"name": p.Name, "color": p.Color, "position": p.Position, "games": members,
		})
	}
	insertTestPoolsItem(t, testDB, jobID, dstUser, poolsPayload)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	for _, g := range doc.Games {
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("marshal exported game: %v", err)
		}
		var asMap map[string]any
		if err := json.Unmarshal(raw, &asMap); err != nil {
			t.Fatalf("game to map: %v", err)
		}
		itemID := insertTestJobItem(t, testDB, jobID, dstUser, asMap)
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
			t.Fatalf("import work: %v", err)
		}
	}
```

Note: `p.Position` and `m.Position` and `p.Color` are unexported-struct fields accessed through the returned `doc.Pools` / `doc.Games` values — that is allowed because the test only reads fields off values it received, never names the type. `exportPoolGameJSON.Position` is `*int`; passing it into the map is fine.

(e) Replace the platform availability assertion and add wishlist + pool assertions. Change the existing acquired-date assertion block's neighbourhood to also check availability:
```go
	if gotP.AcquiredDate == nil || gotP.AcquiredDate.Format("2006-01-02") != "2024-12-25" {
		t.Errorf("acquired_date = %v, want 2024-12-25", gotP.AcquiredDate)
	}
	if gotP.IsAvailable {
		t.Errorf("is_available = true, want false (round-trip)")
	}

	// Platform-less wishlisted game keeps its flag.
	var gotW models.UserGame
	if err := testDB.NewSelect().Model(&gotW).Where("user_id = ? AND game_id = ?", dstUser, int32(7779)).Scan(ctx); err != nil {
		t.Fatalf("dst wishlist game not found: %v", err)
	}
	if !gotW.IsWishlisted {
		t.Errorf("is_wishlisted = false, want true")
	}

	// Pool restored with both members.
	var members int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM pool_games pg JOIN pools p ON p.id = pg.pool_id
		 WHERE p.user_id = ? AND p.name = 'Backlog'`, dstUser,
	).Scan(ctx, &members); err != nil {
		t.Fatalf("count pool members: %v", err)
	}
	if members != 2 {
		t.Errorf("pool members = %d, want 2", members)
	}
```

- [ ] **Step 2: Run the round-trip test**

Run: `go test ./internal/worker/tasks/ -run TestImport_RoundTripPreservesUserData -v`
Expected: PASS

- [ ] **Step 3: Add backward-compat + pool-merge tests**

Append to `internal/worker/tasks/import_item_test.go`:

```go
func TestImportItem_BackwardCompatDefaults(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	g := &models.Game{ID: 8001, Title: "Legacy", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, userID, 1)
	// A 2.0-shaped game: no is_wishlisted, platform with no is_available.
	itemID := insertTestJobItem(t, testDB, jobID, userID, map[string]any{
		"igdb_id":   8001,
		"title":     "Legacy",
		"platforms": []map[string]any{{"platform": "pc-windows"}},
	})

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("work: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(8001)).Scan(ctx); err != nil {
		t.Fatalf("user_game: %v", err)
	}
	if ug.IsWishlisted {
		t.Errorf("is_wishlisted = true, want false (absent ⇒ false)")
	}
	var ugp models.UserGamePlatform
	if err := testDB.NewSelect().Model(&ugp).Where("user_game_id = ?", ug.ID).Scan(ctx); err != nil {
		t.Fatalf("platform: %v", err)
	}
	if !ugp.IsAvailable {
		t.Errorf("is_available = false, want true (absent ⇒ true)")
	}
}

func TestApplyImportedPools_MergesIntoExistingPool(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	for _, id := range []int32{9001, 9002} {
		g := &models.Game{ID: id, Title: "G", LastUpdated: time.Now(), CreatedAt: time.Now()}
		if _, err := testDB.NewInsert().Model(g).Exec(ctx); err != nil {
			t.Fatalf("insert game: %v", err)
		}
	}
	ug1 := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: 9001, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	ug2 := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: 9002, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ug1).Exec(ctx); err != nil {
		t.Fatalf("insert ug1: %v", err)
	}
	if _, err := testDB.NewInsert().Model(ug2).Exec(ctx); err != nil {
		t.Fatalf("insert ug2: %v", err)
	}
	// Pre-existing pool named Backlog with ug1 already a member and a custom color.
	existingPool := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pools (id, user_id, name, color, position) VALUES (?, ?, 'Backlog', '#existing', 5)`, existingPool, userID); err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, NULL)`, uuid.NewString(), existingPool, ug1.ID); err != nil {
		t.Fatalf("insert pg: %v", err)
	}

	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, userID, 0)
	// Import payload re-adds ug1 (dup) and adds ug2, with a DIFFERENT color.
	insertTestPoolsItem(t, testDB, jobID, userID, []map[string]any{{
		"name":  "Backlog",
		"color": "#imported",
		"games": []map[string]any{
			{"igdb_id": 9001, "position": nil},
			{"igdb_id": 9002, "position": 0},
		},
	}})

	tasks.ApplyImportedPoolsForTest(ctx, testDB, jobID, userID)

	// Existing pool kept (color unchanged, no duplicate pool), ug2 merged in.
	var poolCount, memberCount int
	var color string
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM pools WHERE user_id = ? AND name = 'Backlog'`, userID).Scan(ctx, &poolCount); err != nil {
		t.Fatalf("count pools: %v", err)
	}
	if poolCount != 1 {
		t.Errorf("pool count = %d, want 1 (find-or-create, no dup)", poolCount)
	}
	if err := testDB.NewRaw(`SELECT color FROM pools WHERE id = ?`, existingPool).Scan(ctx, &color); err != nil {
		t.Fatalf("select color: %v", err)
	}
	if color != "#existing" {
		t.Errorf("color = %q, want #existing (curation not overwritten)", color)
	}
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM pool_games WHERE pool_id = ?`, existingPool).Scan(ctx, &memberCount); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if memberCount != 2 {
		t.Errorf("member count = %d, want 2 (ug1 dedup + ug2 added)", memberCount)
	}
}
```

Add the test bridge to `internal/worker/tasks/testexports.go`:
```go
// ApplyImportedPoolsForTest exposes applyImportedPools for cross-package tests.
func ApplyImportedPoolsForTest(ctx context.Context, db *bun.DB, jobID, userID string) {
	applyImportedPools(ctx, db, jobID, userID)
}
```

- [ ] **Step 4: Run the full tasks + api suites**

Run: `go test ./internal/worker/tasks/ ./internal/api/ -v 2>&1 | tail -40`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_roundtrip_test.go internal/worker/tasks/import_item_test.go internal/worker/tasks/testexports.go
git commit -m "test: round-trip + backward-compat + pool-merge coverage for #1030"
```

---

## Task 8: Final verification

- [ ] **Step 1: Build + full backend suite**

Run: `go build ./... && go test -timeout 600s ./...`
Expected: build clean, all tests pass.

- [ ] **Step 2: Lint**

Run: `golangci-lint run`
Expected: zero findings. (Watch for `errcheck` on the new `_, err := db.NewRaw(...).Exec(ctx)` discards — each is either handled with a `slog.Warn`/returned error as written above, or, where intentionally advisory, add a `//nolint:errcheck // <reason>`.)

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin issue-1030-export-import-gaps
gh pr create --title "feat: complete JSON export/import round-trip (wishlist, availability, pools)" \
  --body "$(cat <<'EOF'
Closes #1030

Makes the Nexorious JSON export/import a complete round-trip for three user fields that were silently dropped:

- **`is_wishlisted`** — wishlist entries no longer convert to owned library entries on restore.
- **`is_available`** — per-platform availability survives the round-trip.
- **Play Planning pools & queue** — a new top-level `pools` section; on import, pools are applied at the job-completion transition, find-or-created by name and merged additively.

Format version bumps to **2.1**; the importer accepts both 2.0 and 2.1 and keys behaviour off field presence, so older/hand-edited files still import (missing `is_wishlisted`⇒false, `is_available`⇒true, `pools`⇒none).

Design: `docs/superpowers/specs/2026-06-16-issue-1030-export-import-gaps-design.md`
EOF
)"
```

---

## Self-review notes

- **Spec coverage:** version 2.1 (T1/T3), `is_wishlisted` export+import (T1/T4), `is_available` export+import (T1/T4), pools export (T2), pools import via synthetic item + completion hook (T5/T6), listing exclusion (T6), backward-compat defaults (T7), pool merge semantics (T7), round-trip (T7). All spec sections map to a task.
- **CSV untouched:** no task modifies `buildCSVRow`/`csvHeaders` — per the spec, JSON-only.
- **Type consistency:** `exportPoolJSON`/`exportPoolGameJSON` (export) vs `importPoolData`/`importPoolGameData` (import) are deliberately distinct types on each side; `PoolsItemKey` is the single shared constant used by the handler, the listing filter, and the worker. `BuildJSONDocForTest(ugs, pools)` 2-arg form is used consistently after Task 2.
- **No migration:** confirmed all columns exist and `job_items.source_metadata` is jsonb.
