# Immediate Metadata Fetch for Newly Synced Games — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a sync run adds a new game, immediately enqueue a fire-and-forget IGDB metadata fetch so the game has cover art and full metadata within seconds instead of waiting up to 24 h for the next bulk refresh.

**Architecture:** Extract the existing "fetch IGDB metadata → update games row → download cover art" logic from `MetadataRefreshItemWorker` into a shared package-level helper `fetchAndStoreMetadata`. Add a new fire-and-forget River worker `MetadataFetchWorker` (kind `metadata_fetch`, priority 2, 3 attempts, no `jobs`/`job_items` tracking) that calls the helper for a single game. At the end of sync Stage 3 (`UserGameWorker.Work`), when IGDB is configured and the `games` row has no `description`, enqueue a `metadata_fetch` job. No schema changes, no migrations.

**Tech Stack:** Go 1.25, River job queue (`riverqueue/river`), Bun ORM, PostgreSQL (testcontainers-go for tests), IGDB client (`internal/services/igdb`).

---

## Background — key facts established from the codebase

- **`MetadataRefreshItemWorker.Work`** lives in [internal/worker/tasks/metadata_refresh.go:171-316](internal/worker/tasks/metadata_refresh.go#L171-L316). Its core (load game row, `FetchFullMetadata`, `UPDATE games`, cover-art download) is steps 3 + 5 + 6 + 7. This is what gets extracted.
- **`UserGameWorker.Work`** lives in [internal/worker/tasks/sync.go:521-745](internal/worker/tasks/sync.go#L521-L745). The struct (lines 514-519) currently has only `DB` and `RiverClient`. The successful write path reaches the bottom block at lines 716-744; skipped games and unresolved games `return` earlier. `*eg.ResolvedIGDBID` is the IGDB game ID and is non-nil at the bottom of the function.
- **IGDB client** `*igdbsvc.Client` has `Configured() bool` ([internal/services/igdb/igdb.go:71](internal/services/igdb/igdb.go#L71)) — a local bool, no network call. `sync.go` already imports `igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"`.
- **`games.description`** is nullable and is `NULL` on the bare insert that Stage 2/Stage 3 perform: `INSERT INTO games (id, title, last_updated, created_at)` (e.g. [sync.go:587-590](internal/worker/tasks/sync.go#L587-L590)). `convertToGameMetadata` maps IGDB `summary` → `md.Description` ([igdb.go:542-544](internal/services/igdb/igdb.go#L542-L544)).
- **River worker wiring** is in [cmd/nexorious/serve.go](cmd/nexorious/serve.go) in TWO places: the initial wiring (~lines 185-222) and `RebuildServices` (~lines 261-297). Both must be updated.
- **Fire-and-forget enqueue** must use `RiverClient.Insert` directly, NOT `EnqueueOrFail` — `EnqueueOrFail` marks a *job_item* failed on insert error, but `metadata_fetch` has no job_item.
- **Test harness:** package `tasks_test` uses a shared `testDB` and `truncateAllTables(t)` ([internal/worker/tasks/main_test.go](internal/worker/tasks/main_test.go)). Helpers `insertTestGame`, `igdbTestServer`, `newTestIGDBClient` are in [metadata_refresh_test.go](internal/worker/tasks/metadata_refresh_test.go); `newTestRiverClient`, `insertTestUser` are in [sync_test.go](internal/worker/tasks/sync_test.go)/[main_test.go](internal/worker/tasks/main_test.go). All test files share the same package, so all helpers are visible across them.
- **Setting `Attempt`/`MaxAttempts` in tests:** `&river.Job[T]{JobRow: &rivertype.JobRow{Attempt: n, MaxAttempts: m}, Args: ...}` (pattern at [sync_test.go:1315-1316](internal/worker/tasks/sync_test.go#L1315-L1316)). `river.Job[T]` embeds `*rivertype.JobRow`; leaving `JobRow` nil makes `job.Attempt` panic, so set it whenever the worker reads those fields.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `internal/worker/tasks/metadata_refresh.go` | Modify | Add `fetchAndStoreMetadata` helper; slim `MetadataRefreshItemWorker.Work` to call it. |
| `internal/worker/tasks/metadata_fetch.go` | Create | New `MetadataFetchArgs` + `MetadataFetchWorker` (fire-and-forget single-game fetch). |
| `internal/worker/tasks/metadata_fetch_test.go` | Create | Tests for `MetadataFetchWorker`. |
| `internal/worker/tasks/sync.go` | Modify | Add `IGDBClient` field to `UserGameWorker`; add `maybeEnqueueImmediateMetadataFetch`; call it at end of Stage 3. |
| `internal/worker/tasks/sync_test.go` | Modify | Add `UserGameWorker` enqueue tests. |
| `cmd/nexorious/serve.go` | Modify | Register `MetadataFetchWorker`; pass `IGDBClient` into `UserGameWorker` (both wiring sites). |
| `docs/sync.md` | Modify | Add Stage 3 step; replace Maintenance paragraph with pointer. |
| `docs/maintenance.md` | Create | Process reference for all periodic maintenance workers. |

---

## Task 1: Feature branch + commit the plan

**Files:**
- Commit: `docs/superpowers/plans/2026-05-27-issue-622-immediate-metadata-fetch.md`

- [ ] **Step 1: Create the feature branch**

Run:
```bash
cd /home/abo/workspace/home/nexorious
git checkout -b feat/issue-622-immediate-metadata-fetch
```
Expected: `Switched to a new branch 'feat/issue-622-immediate-metadata-fetch'`

- [ ] **Step 2: Commit the plan file**

Run:
```bash
git add docs/superpowers/plans/2026-05-27-issue-622-immediate-metadata-fetch.md
git commit -m "docs: add implementation plan for issue #622 immediate metadata fetch"
```
Expected: one file committed.

---

## Task 2: Extract shared `fetchAndStoreMetadata` helper

This is a **refactor** of existing behaviour — keep the existing `MetadataRefreshItemWorker` tests green throughout. No new test is written; the existing tests (`TestMetadataRefreshItem_Success`, `_IGDBError`, `_CoverArtFailureNonFatal`, `_CoverArtUnchanged`, `_IGDBNotConfiguredAtItemLevel`) are the regression guard.

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go`

- [ ] **Step 1: Run the existing item-worker tests to confirm a green baseline**

Run:
```bash
go test ./internal/worker/tasks/ -run 'TestMetadataRefreshItem' -v
```
Expected: PASS for `TestMetadataRefreshItem_Success`, `_IGDBError`, `_CoverArtFailureNonFatal`, `_CoverArtUnchanged`, `_JobItemNotFound`, `_IGDBNotConfiguredAtItemLevel`, `_BadSourceMetadata`.

- [ ] **Step 2: Add the `fetchAndStoreMetadata` helper**

In `internal/worker/tasks/metadata_refresh.go`, add this function in the `// ─── Helpers ───` section (after `metaRefreshCheckJobCompletion`, at the end of the file). It is the verbatim extraction of the current item-worker steps 3, 5, 6, 7:

```go
// fetchAndStoreMetadata fetches fresh IGDB metadata for a single game and writes
// it to the games row, including cover art (cover-art failure is non-fatal). It
// is the shared core used by both MetadataRefreshItemWorker (which layers
// job_items tracking on top) and the fire-and-forget MetadataFetchWorker. The
// caller must verify IGDB is configured before calling.
func fetchAndStoreMetadata(ctx context.Context, db *bun.DB, igdbClient *igdbsvc.Client, storagePath string, gameID int32) error {
	// Load the current games row; cover_art_url drives the cover re-download decision.
	var game struct {
		ID          int32   `bun:"id"`
		Title       string  `bun:"title"`
		CoverArtUrl *string `bun:"cover_art_url"`
	}
	if err := db.NewRaw(
		`SELECT id, title, cover_art_url FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &game); err != nil {
		return fmt.Errorf("load game: %w", err)
	}

	md, err := igdbClient.FetchFullMetadata(ctx, int(gameID))
	if err != nil {
		return err
	}

	var releaseDate *time.Time
	if md.ReleaseDate != nil {
		if t, err := time.Parse("2006-01-02", *md.ReleaseDate); err == nil {
			releaseDate = &t
		}
	}

	var igdbSlug *string
	if md.IgdbSlug != "" {
		igdbSlug = &md.IgdbSlug
	}

	var igdbPlatformIds *string
	if len(md.PlatformIDs) > 0 {
		b, _ := json.Marshal(md.PlatformIDs)
		s := string(b)
		igdbPlatformIds = &s
	}

	var igdbPlatformNames *string
	if len(md.PlatformNames) > 0 {
		b, _ := json.Marshal(md.PlatformNames)
		s := string(b)
		igdbPlatformNames = &s
	}

	if _, err := db.NewRaw(
		`UPDATE games SET
			title = ?,
			description = ?,
			genre = ?,
			developer = ?,
			publisher = ?,
			release_date = ?,
			rating_average = ?,
			rating_count = ?,
			howlongtobeat_main = ?,
			howlongtobeat_extra = ?,
			howlongtobeat_completionist = ?,
			igdb_slug = ?,
			igdb_platform_ids = ?,
			igdb_platform_names = ?,
			game_modes = ?,
			themes = ?,
			player_perspectives = ?,
			last_updated = now()
		WHERE id = ?`,
		md.Title,
		md.Description,
		md.Genre,
		md.Developer,
		md.Publisher,
		releaseDate,
		md.RatingAverage,
		md.RatingCount,
		md.HowlongtobeatMain,
		md.HowlongtobeatExtra,
		md.HowlongtobeatCompletionist,
		igdbSlug,
		igdbPlatformIds,
		igdbPlatformNames,
		md.GameModes,
		md.Themes,
		md.PlayerPerspectives,
		gameID,
	).Exec(ctx); err != nil {
		return fmt.Errorf("update games: %w", err)
	}

	// Cover art (non-fatal).
	if md.CoverImageID != "" {
		expectedURLPath := "/static/cover_art/" + md.CoverImageID + ".jpg"
		if game.CoverArtUrl == nil || *game.CoverArtUrl != expectedURLPath {
			coverURLPath, err := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
			if err != nil {
				slog.Warn("metadata fetch: cover art download failed",
					"game_id", game.ID, "image_id", md.CoverImageID, "err", err)
			} else if coverURLPath != "" {
				if _, err := db.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx); err != nil {
					slog.Error("metadata fetch: update cover_art_url failed", "err", err, "game_id", game.ID)
				}
			}
		}
	}

	return nil
}
```

- [ ] **Step 3: Replace the body of `MetadataRefreshItemWorker.Work` to call the helper**

In `internal/worker/tasks/metadata_refresh.go`, replace the entire `Work` method (currently [lines 171-316](internal/worker/tasks/metadata_refresh.go#L171-L316)) with this slimmed version. Steps 3 (load game), 5 (fetch), 6 (update), 7 (cover) are now inside the helper; the per-item IGDB guard is **kept** to preserve the exact `"igdb_not_configured"` failure message:

```go
func (w *MetadataRefreshItemWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshItemArgs]) error {
	jobItemID := job.Args.JobItemID

	// Step 1 — Load job_item.
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", jobItemID).Scan(ctx); err != nil {
		slog.Error("metadata_refresh_item: load job_item", "id", jobItemID, "err", err)
		return nil
	}

	// Step 2 — Parse source_metadata.
	var sourceMeta metadataRefreshSourceMeta
	if err := json.Unmarshal(item.SourceMetadata, &sourceMeta); err != nil {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 3 — IGDB guard (defensive; preserves the per-item failure message).
	if !w.IGDBClient.Configured() {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, "igdb_not_configured")
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 4 — Fetch IGDB metadata and update the games row (shared helper).
	if err := fetchAndStoreMetadata(ctx, w.DB, w.IGDBClient, w.StoragePath, sourceMeta.GameID); err != nil {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, err.Error())
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 5 — Mark item completed.
	metaRefreshMarkItemCompleted(ctx, w.DB, &item)

	// Step 6 — Check job completion.
	metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)

	return nil
}
```

Note: the `metadataRefreshSourceMeta` type (lines 167-169) is unchanged and still used. The imports `time` and `igdbsvc` are still used (by the helper). Do not remove any imports.

- [ ] **Step 4: Build and run the item-worker tests to confirm they still pass**

Run:
```bash
go build ./... && go test ./internal/worker/tasks/ -run 'TestMetadataRefreshItem' -v
```
Expected: build succeeds; all `TestMetadataRefreshItem_*` tests PASS (identical results to Step 1).

- [ ] **Step 5: Lint and commit**

Run:
```bash
golangci-lint run ./internal/worker/tasks/...
git add internal/worker/tasks/metadata_refresh.go
git commit -m "refactor: extract shared fetchAndStoreMetadata helper from metadata refresh worker"
```
Expected: zero lint findings; commit succeeds.

---

## Task 3: New `MetadataFetchWorker` (fire-and-forget single-game fetch)

**Files:**
- Create: `internal/worker/tasks/metadata_fetch.go`
- Test: `internal/worker/tasks/metadata_fetch_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/worker/tasks/metadata_fetch_test.go`:

```go
package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/ratelimit"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestMetadataFetchWorker_Success(t *testing.T) {
	truncateAllTables(t)
	// Bare game row: title set, description NULL (as created by sync Stage 2/3).
	insertTestGame(t, 3001, "Old Title", time.Now().Add(-24*time.Hour))

	// No cover field — avoids a real IGDB CDN download in tests.
	gamesJSON := `[{"id":3001,"name":"Fetched Title","slug":"fetched-title","summary":"A great game"}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	job := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3001},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()
	var title string
	var description *string
	_ = testDB.NewRaw(`SELECT title, description FROM games WHERE id = 3001`).Scan(ctx, &title, &description)
	if title != "Fetched Title" {
		t.Errorf("title: want 'Fetched Title', got %q", title)
	}
	if description == nil || *description != "A great game" {
		t.Errorf("description: want 'A great game', got %v", description)
	}
}

func TestMetadataFetchWorker_IGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	insertTestGame(t, 3005, "Untouched", time.Now().Add(-24*time.Hour))

	unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: unconfigured, StoragePath: t.TempDir()}
	job := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3005},
	}
	// Returns nil (no retry) and leaves the game untouched.
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var description *string
	_ = testDB.NewRaw(`SELECT description FROM games WHERE id = 3005`).Scan(context.Background(), &description)
	if description != nil {
		t.Errorf("description: want nil (untouched), got %v", *description)
	}
}

func TestMetadataFetchWorker_RetriesThenGivesUp(t *testing.T) {
	truncateAllTables(t)
	insertTestGame(t, 3010, "Not In IGDB", time.Now().Add(-24*time.Hour))

	// Empty IGDB response → FetchFullMetadata returns ErrGameNotFound.
	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}

	// Non-final attempt → returns an error so River retries.
	nonFinal := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3010},
	}
	if err := w.Work(context.Background(), nonFinal); err == nil {
		t.Error("non-final attempt: want error (so River retries), got nil")
	}

	// Final attempt → logs at error and returns nil (no further retry, no noise).
	final := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3010},
	}
	if err := w.Work(context.Background(), final); err != nil {
		t.Errorf("final attempt: want nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run:
```bash
go test ./internal/worker/tasks/ -run 'TestMetadataFetchWorker' -v
```
Expected: FAIL — compile error `undefined: tasks.MetadataFetchWorker` / `undefined: tasks.MetadataFetchArgs`.

- [ ] **Step 3: Create the worker**

Create `internal/worker/tasks/metadata_fetch.go`:

```go
package tasks

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
)

// MetadataFetchArgs is the River job args type for "metadata_fetch": an
// immediate, fire-and-forget IGDB metadata fetch for a single newly added game.
type MetadataFetchArgs struct {
	GameID int32 `json:"game_id"`
}

func (MetadataFetchArgs) Kind() string { return "metadata_fetch" }

// InsertOpts sets priority 2 (between sync workers at 1 and the bulk metadata
// refresh at 3) and 3 attempts. The fetch runs promptly after the triggering
// sync without delaying in-flight sync jobs, and transient IGDB failures are
// retried with backoff before giving up.
func (MetadataFetchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 2}
}

// MetadataFetchWorker performs a single-game IGDB metadata fetch with no
// jobs/job_items tracking layer. It is triggered at the end of sync Stage 3 for
// games that have no metadata yet. Fire-and-forget: success logs at debug,
// exhausted retries log at error, and the periodic bulk refresh remains the
// safety net for anything this misses.
type MetadataFetchWorker struct {
	river.WorkerDefaults[MetadataFetchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	StoragePath string
}

func (w *MetadataFetchWorker) Work(ctx context.Context, job *river.Job[MetadataFetchArgs]) error {
	gameID := job.Args.GameID

	// IGDB guard: nothing to do if not configured. Return nil so River does not retry.
	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		slog.Debug("metadata_fetch: IGDB not configured, skipping", "game_id", gameID)
		return nil
	}

	if err := fetchAndStoreMetadata(ctx, w.DB, w.IGDBClient, w.StoragePath, gameID); err != nil {
		// Exhausted retries: log at error and stop (return nil). The periodic
		// bulk refresh will still pick the game up eventually.
		if job.Attempt >= job.MaxAttempts {
			slog.Error("metadata_fetch: exhausted retries", "game_id", gameID, "err", err)
			return nil
		}
		// Otherwise return the error so River retries with backoff.
		return fmt.Errorf("metadata_fetch game %d: %w", gameID, err)
	}

	slog.Debug("metadata_fetch: completed", "game_id", gameID)
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:
```bash
go test ./internal/worker/tasks/ -run 'TestMetadataFetchWorker' -v
```
Expected: PASS for `TestMetadataFetchWorker_Success`, `_IGDBNotConfigured`, `_RetriesThenGivesUp`.

- [ ] **Step 5: Lint and commit**

Run:
```bash
golangci-lint run ./internal/worker/tasks/...
git add internal/worker/tasks/metadata_fetch.go internal/worker/tasks/metadata_fetch_test.go
git commit -m "feat: add fire-and-forget MetadataFetchWorker for single-game metadata"
```
Expected: zero lint findings; commit succeeds.

---

## Task 4: Enqueue immediate fetch at end of sync Stage 3

**Files:**
- Modify: `internal/worker/tasks/sync.go` (struct at lines 514-519; call site near line 740; new method near the sync helpers)
- Test: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/worker/tasks/sync_test.go`. These mirror `TestUserGameWorker_CreatesUserGameAndSyncChange` ([sync_test.go:1511](internal/worker/tasks/sync_test.go#L1511)) but vary the IGDB-configured / description / river-client conditions and assert on the `metadata_fetch` River job. `newTestIGDBClient`, `newTestRiverClient`, `insertTestUser`, `igdbTestServer` are existing package helpers (visible across the `tasks_test` files). **Add no new imports** — `sync_test.go` already imports everything used here: `config`, `ratelimit`, and the IGDB package aliased as `igdb` (NOT `igdbsvc`), plus `uuid`, `river`, `rivertype`, `context`. Use the `igdb.NewClient(...)` form, matching that file's existing alias.

```go
// userGameStage3Fixture seeds the rows a UserGameWorker run needs: user,
// platform, storefront, games row (description NULL), external_game (resolved),
// one external_game_platform, and a pending sync job_item. It returns the
// job_item ID and the IGDB game ID.
func userGameStage3Fixture(t *testing.T) (itemID string, igdbID int32) {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 42.5, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID = uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)
	return itemID, igdbID
}

func TestUserGameWorker_EnqueuesImmediateMetadataFetch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, igdbID := userGameStage3Fixture(t)

	srv := igdbTestServer(t, `[]`) // never actually called — UserGameWorker only checks Configured()
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch' AND (args->>'game_id')::int = ?`, igdbID,
	).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("metadata_fetch river_job for game %d: want 1, got %d", igdbID, count)
	}
}

func TestUserGameWorker_SkipsMetadataFetchWhenDescriptionPresent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, igdbID := userGameStage3Fixture(t)
	// Game already has metadata.
	_, _ = testDB.NewRaw(`UPDATE games SET description = 'already here' WHERE id = ?`, igdbID).Exec(ctx)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch'`).Scan(ctx, &count)
	if count != 0 {
		t.Errorf("metadata_fetch river_job: want 0 (description present), got %d", count)
	}
}

func TestUserGameWorker_SkipsMetadataFetchWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, _ := userGameStage3Fixture(t)

	unconfigured := igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: unconfigured, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch'`).Scan(ctx, &count)
	if count != 0 {
		t.Errorf("metadata_fetch river_job: want 0 (IGDB not configured), got %d", count)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:
```bash
go test ./internal/worker/tasks/ -run 'TestUserGameWorker_(Enqueues|Skips)' -v
```
Expected: FAIL — compile error `unknown field 'IGDBClient' in struct literal of type tasks.UserGameWorker`.

- [ ] **Step 3: Add the `IGDBClient` field to `UserGameWorker`**

In `internal/worker/tasks/sync.go`, change the struct ([lines 514-519](internal/worker/tasks/sync.go#L514-L519)):

```go
// UserGameWorker writes the user_game and user_game_platform rows for a resolved sync item.
type UserGameWorker struct {
	river.WorkerDefaults[UserGameArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}
```

- [ ] **Step 4: Add the `maybeEnqueueImmediateMetadataFetch` method**

In `internal/worker/tasks/sync.go`, add this method immediately after the `UserGameWorker.Work` method (after line 745, before `syncMarkItemFailed`):

```go
// maybeEnqueueImmediateMetadataFetch enqueues a fire-and-forget metadata_fetch
// job for gameID when IGDB is configured and the games row has no description
// yet. Every failure mode is non-fatal — the periodic bulk refresh is the
// safety net — so we log at warn and move on rather than failing the job_item.
func (w *UserGameWorker) maybeEnqueueImmediateMetadataFetch(ctx context.Context, gameID int32) {
	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		return
	}

	var descriptionIsNull bool
	if err := w.DB.NewRaw(
		`SELECT description IS NULL FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &descriptionIsNull); err != nil {
		slog.Warn("user_game_write: check game description for immediate metadata fetch",
			"err", err, "game_id", gameID)
		return
	}
	if !descriptionIsNull {
		return // already has metadata
	}

	if w.RiverClient == nil {
		slog.Warn("user_game_write: river client unavailable, skipping immediate metadata fetch",
			"game_id", gameID)
		return
	}
	if _, err := w.RiverClient.Insert(ctx, MetadataFetchArgs{GameID: gameID}, nil); err != nil {
		slog.Warn("user_game_write: enqueue immediate metadata fetch failed",
			"err", err, "game_id", gameID)
	}
}
```

- [ ] **Step 5: Call it at the end of Stage 3**

In `internal/worker/tasks/sync.go`, in `UserGameWorker.Work`, insert the call between the `sync_changes` if/else block and `syncMarkItemCompleted` (currently [lines 740-743](internal/worker/tasks/sync.go#L740-L743)). The result should read:

```go
		} else if !platformUpgraded {
			if _, err := w.DB.NewRaw(
				`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'already_in_library', ?, now())`,
				uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("user_game_write: insert sync_change (already_in_library)", "err", err)
			}
		}

		// Immediate metadata fetch: if IGDB is configured and the games row has no
		// description yet, enqueue a fire-and-forget fetch so newly added games get
		// cover art and full IGDB data within seconds rather than waiting for the
		// next scheduled bulk refresh. Non-fatal — the bulk refresh is the safety net.
		w.maybeEnqueueImmediateMetadataFetch(ctx, *eg.ResolvedIGDBID)

		syncMarkItemCompleted(ctx, w.DB, &item)
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
}
```

(`*eg.ResolvedIGDBID` is guaranteed non-nil here — the function returned earlier at the `eg.ResolvedIGDBID == nil` guard, line 580.)

- [ ] **Step 6: Build and run the new tests**

Run:
```bash
go build ./... && go test ./internal/worker/tasks/ -run 'TestUserGameWorker_(Enqueues|Skips)' -v
```
Expected: build succeeds; `TestUserGameWorker_EnqueuesImmediateMetadataFetch`, `_SkipsMetadataFetchWhenDescriptionPresent`, `_SkipsMetadataFetchWhenIGDBNotConfigured` all PASS.

- [ ] **Step 7: Run the full tasks package to confirm no regressions**

Run:
```bash
go test ./internal/worker/tasks/ -v 2>&1 | tail -40
```
Expected: all existing `TestUserGameWorker_*`, `TestDispatchSyncWorker_*`, `TestIGDBMatch*`, `TestMetadataRefresh*` tests still PASS (the pre-existing `RiverClient: nil` / no-`IGDBClient` tests hit the `w.IGDBClient == nil` guard and behave unchanged).

- [ ] **Step 8: Lint and commit**

Run:
```bash
golangci-lint run ./internal/worker/tasks/...
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(sync): enqueue immediate metadata fetch for newly added games"
```
Expected: zero lint findings; commit succeeds.

---

## Task 5: Wire `MetadataFetchWorker` and `UserGameWorker.IGDBClient` in serve.go

`serve.go` registers River workers in **two** places (initial startup and `RebuildServices`). Both must be updated identically. There is no unit test for serve.go wiring; the verification is a successful build plus the existing tests.

**Files:**
- Modify: `cmd/nexorious/serve.go`

- [ ] **Step 1: Update the initial wiring site**

In `cmd/nexorious/serve.go`, change the `userGameWorker` construction ([line 186](cmd/nexorious/serve.go#L186)) from:

```go
	userGameWorker := &tasks.UserGameWorker{DB: db}
```
to:
```go
	userGameWorker := &tasks.UserGameWorker{DB: db, IGDBClient: igdbClient}
```

Then, immediately after the `MetadataRefreshItemWorker` registration ([line 196](cmd/nexorious/serve.go#L196)), add the new worker:

```go
	river.AddWorker(workers, &tasks.MetadataRefreshItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.MetadataFetchWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
```

(`userGameWorker.RiverClient = riverClient` at line 222 is already present — no change needed there.)

- [ ] **Step 2: Update the `RebuildServices` wiring site**

In the same file, inside `RebuildServices`, change `newUserGame` ([line 262](cmd/nexorious/serve.go#L262)) from:

```go
			newUserGame := &tasks.UserGameWorker{DB: newDB}
```
to:
```go
			newUserGame := &tasks.UserGameWorker{DB: newDB, IGDBClient: igdbClient}
```

Then, immediately after the `MetadataRefreshItemWorker` registration ([line 272](cmd/nexorious/serve.go#L272)), add:

```go
			river.AddWorker(newWorkers, &tasks.MetadataRefreshItemWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, &tasks.MetadataFetchWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
```

(`newUserGame.RiverClient = newClient` at line 297 is already present — no change needed there.)

- [ ] **Step 3: Build to confirm both sites compile**

Run:
```bash
go build ./...
```
Expected: build succeeds with no errors.

- [ ] **Step 4: Lint and commit**

Run:
```bash
golangci-lint run ./cmd/...
git add cmd/nexorious/serve.go
git commit -m "feat: register MetadataFetchWorker and wire IGDB into UserGameWorker"
```
Expected: zero lint findings; commit succeeds.

---

## Task 6: Documentation

**Files:**
- Modify: `docs/sync.md`
- Create: `docs/maintenance.md`

- [ ] **Step 1: Add the Stage 3 documentation step**

In `docs/sync.md`, in the `## Stage 3 — User Game Write` numbered list, add a new step after step 5 ([line 226](docs/sync.md#L226)). The list currently ends at:

```
5. Update `external_game.updated_at` — always, whether the game was skipped or not
```
Change it to:
```
5. Update `external_game.updated_at` — always, whether the game was skipped or not
6. After writing all platform rows, if IGDB is configured and the `games` row has no description, an immediate metadata fetch is enqueued for that game. This ensures newly added games have cover art and full IGDB data within seconds rather than waiting for the next scheduled bulk refresh. The enqueue is fire-and-forget and non-fatal — the periodic bulk refresh (see [docs/maintenance.md](../maintenance.md) § "Metadata refresh") remains the safety net.
```

- [ ] **Step 2: Replace the Maintenance section paragraph with a pointer**

In `docs/sync.md`, replace the body of the `## Maintenance` section ([lines 316-318](docs/sync.md#L316-L318)). Change:

```
## Maintenance

A periodic maintenance worker prunes `sync_changes` entries older than the retention period configured by `SYNC_HISTORY_RETENTION_DAYS` (default: 90 days). This keeps the table from growing unboundedly while preserving recent history for the Sync History UI.
```
to:
```
## Maintenance

Maintenance tasks that support the sync system — sync history pruning, orphaned item rescue, and stale job cleanup — are documented in [docs/maintenance.md](../maintenance.md).
```

- [ ] **Step 3: Create `docs/maintenance.md`**

Create `docs/maintenance.md` (the retention figures below are verified against the code: jobs 30 days at [scheduler.go:308-313](internal/scheduler/scheduler.go#L308-L313), exports 24 hours at [scheduler.go:326-334](internal/scheduler/scheduler.go#L326-L334), sync history default 90 days, orphaned-item age 1 hour at [scheduler.go:97](internal/scheduler/scheduler.go#L97), metadata refresh default 24 hours at [scheduler.go:243-245](internal/scheduler/scheduler.go#L243-L245)):

```markdown
# Maintenance

Nexorious runs a set of periodic maintenance workers on cron-style schedules,
registered in `scheduler.BuildPeriodicJobs()`. This document is a process
reference — what each task does and why — not an implementation guide.

## Sync history pruning

Removes `sync_changes` entries older than the configured retention period
(default: 90 days, configurable via `SYNC_HISTORY_RETENTION_DAYS`) to keep the
sync history table from growing unboundedly while preserving recent history for
the Sync History UI.

## Job pruning

Removes completed, failed, and cancelled jobs (and their associated items, via
cascade) after 30 days.

## Export cleanup

Removes export files from disk and clears their stored path 24 hours after the
export job completed.

## Unreferenced game cleanup

Removes `games` catalogue entries that no longer have any user in their library.
This can occur when a user removes all copies of a game, or after a rematch that
leaves an old IGDB entry with no references.

## Session cleanup

Removes expired login sessions.

## Stale job cleanup

Detects `metadata_refresh` jobs stuck in an active state with no remaining
unfinished items (indicating a crash during dispatch) and marks them failed.
This releases the duplicate-run guard so the next scheduled dispatch can proceed.

## Orphaned item rescue

Detects `job_items` stuck in `pending` with no backing River job and re-enqueues
them. This is a safety net for items whose River job was lost due to a crash or
deployment. Only items older than one hour are considered, to avoid racing
freshly-created items.

## Metadata refresh

Periodically fetches fresh IGDB metadata for every game in the catalogue,
ordered by staleness (least recently updated first). Covers description, cover
art, genres, release date, developer, publisher, rating, platform names, game
modes, themes, player perspectives, and HowLongToBeat times. The schedule is
configurable via `METADATA_REFRESH_INTERVAL` (default: 24 hours) and can also be
triggered manually by an admin.

The immediate per-game fetch (triggered at the end of sync Stage 3 — see
[docs/sync.md](sync.md)) is the complement: it handles newly added games so they
do not wait for the next scheduled window. Games whose immediate fetch exhausts
all retries are still picked up by this scheduled refresh.

## Scheduled backup

Checks every minute whether a user-configured backup schedule is due. When a
backup is due, runs it and applies the configured retention policy.

> The scheduled sync trigger (`CheckPendingSyncs`) is documented in
> [docs/sync.md](sync.md) under "Scheduled Sync" and is not duplicated here.
```

- [ ] **Step 4: Verify the docs and commit**

Run:
```bash
git add docs/sync.md docs/maintenance.md
git diff --cached --stat
git commit -m "docs: document immediate metadata fetch and add maintenance reference"
```
Expected: `docs/sync.md` and `docs/maintenance.md` staged; commit succeeds.

---

## Task 7: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Full build**

Run:
```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 2: Full Go test suite**

Run:
```bash
go test -timeout 600s ./...
```
Expected: all packages PASS. Pay attention to `internal/worker/tasks` (the changed package) and `cmd/nexorious`.

- [ ] **Step 3: Full lint**

Run:
```bash
golangci-lint run
```
Expected: zero findings.

- [ ] **Step 4: Confirm no frontend or slumber changes are needed**

No new HTTP API route is added (the change is a River worker + an internal enqueue), so neither `slumber.yaml` nor the frontend requires changes. Confirm by reviewing the diff:
```bash
git diff main --stat
```
Expected: changes only under `internal/worker/tasks/`, `cmd/nexorious/`, and `docs/`.

- [ ] **Step 5: Summarise for the user and stop**

Report: all tasks complete, tests/lint green, branch `feat/issue-622-immediate-metadata-fetch` ready. Do NOT open a PR or merge — wait for the user to instruct (per CLAUDE.md branch workflow).

---

## Self-Review

**Spec coverage:**
- "New Worker: Immediate Metadata Fetch" (kind `metadata_fetch`, priority 2, IGDB guard, fire-and-forget, 3 attempts) → Task 3.
- "Shared Fetch Logic" extracted from `MetadataRefreshItemWorker`, cover-art non-fatal in both paths → Task 2.
- "Enqueue Point" at end of `UserGameWorker.Work`, conditional on IGDB configured + `description IS NULL`, non-fatal, covers both auto-resolve and manual-resolve paths (both flow through the bottom of `Work`) → Task 4.
- "What Does Not Change" (dispatch/item workers' schedule + UI, schema, migrations, Stages 1-2) → respected: Task 2 keeps the item worker's `job_items` tracking and IGDB guard; no migration tasks exist.
- "Documentation Changes" (`docs/sync.md` Stage 3 step + Maintenance pointer; new `docs/maintenance.md`) → Task 6.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command shows expected output.

**Type consistency:** `MetadataFetchArgs{GameID int32}` is defined in Task 3 and used identically in Task 4's enqueue and tests. `fetchAndStoreMetadata(ctx, db, igdbClient, storagePath, gameID int32) error` is defined in Task 2 and called with matching arguments in Task 2 (item worker) and Task 3 (fetch worker). `MetadataFetchWorker{DB, IGDBClient, StoragePath}` fields match between Task 3's definition and Task 5's wiring. `UserGameWorker{DB, IGDBClient, RiverClient}` fields match between Task 4's struct change and Task 5's wiring. `maybeEnqueueImmediateMetadataFetch(ctx, gameID int32)` defined and called in Task 4 with consistent signature.
