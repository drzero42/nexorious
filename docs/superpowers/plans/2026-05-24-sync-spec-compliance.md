# Sync Spec Compliance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all eight gaps between the live codebase and `docs/sync.md`, covering functional bugs (A–D, H) and the unified `StorefrontAdapter` architecture (E–G).

**Architecture:** Four separate storefront adapter interfaces and a ~600-line switch in `DispatchSyncWorker.Work` are replaced by a single `StorefrontAdapter` interface and a factory function wired at startup; service packages gain `Adapter` structs that implement the interface; credential decryption and token refresh move into the factory so `Work` becomes storefront-agnostic. Functional bugs (HandleSkipItem missing completion check, HandleResolveItem wrong stage + premature DB write, missing CleanupSyncChanges pruning, Epic excluded from scheduled sync) are fixed before the architecture tasks.

**Tech Stack:** Go 1.25, Bun ORM, River job queue, Echo v5, testcontainers-go (shared `testDB` per package).

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `internal/worker/tasks/sync.go` | Export `SyncCheckJobCompletion`; add `StorefrontAdapter`/`ExternalGameEntry`/`ErrCredentials`; update `EpicClientAdapter`; rewrite `DispatchSyncWorker` |
| Modify | `internal/api/job_items.go` | Fix `HandleSkipItem` (gap A) and `HandleResolveItem` (gaps B+C) |
| Modify | `internal/api/job_items_test.go` | Tests for gaps A, B, C |
| Modify | `internal/config/config.go` | Add `SyncHistoryRetentionDays` |
| Modify | `internal/scheduler/scheduler.go` | Add `CleanupSyncChangesWorker`; remove Epic guard |
| Modify | `internal/scheduler/cleanup_test.go` | Test for `CleanupSyncChanges` |
| Modify | `internal/services/platformresolution/resolution.go` | Rename `RawPlatformToSlug` → `PlatformToSlug` |
| Modify | `internal/services/platformresolution/resolution_test.go` | Update renamed function call |
| Create | `internal/services/steam/adapter.go` | `steam.Adapter` implements `StorefrontAdapter` |
| Modify | `internal/services/psn/client.go` | Rename `RawPlatform` → `Platforms []string` |
| Create | `internal/services/psn/adapter.go` | `psn.Adapter` implements `StorefrontAdapter` |
| Modify | `internal/services/gog/library.go` | Rename `RawPlatform` → `Platforms []string`; consolidate per-game entries |
| Create | `internal/services/gog/adapter.go` | `gog.Adapter` implements `StorefrontAdapter` |
| Modify | `internal/services/epic/client.go` | (No-op: `ExternalGameEntry` already renamed) |
| Modify | `cmd/nexorious/serve.go` | Add `buildAdapterFactory`; update both `DispatchSyncWorker` wiring blocks; register `CleanupSyncChangesWorker` |
| Modify | `internal/worker/tasks/sync_test.go` | Collapse three fake adapters into one `fakeStorefrontAdapter` |

---

## Task 1: Export `SyncCheckJobCompletion` and fix gap A (HandleSkipItem)

**Files:**
- Modify: `internal/worker/tasks/sync.go:1263`
- Modify: `internal/api/job_items.go:168`
- Test: `internal/api/job_items_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/job_items_test.go`:

```go
func TestSkipItem_TerminatesJobWhenLastItem(t *testing.T) {
    truncateAllTables(t)
    e := newTestEchoWithPool(t, testDB)

    userID, token := setupTagUser(t, testDB, e, "ji-skip-term")

    // Create a sync job with exactly one pending_review item.
    insertJob(t, testDB, "job-skip-term", userID, "sync", "steam", "processing")
    // Insert an external_game so HandleSkipItem can mark it skipped.
    _, err := testDB.ExecContext(context.Background(),
        `INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
         VALUES ('eg-skip-term', ?, 'steam', 'app999', 'Skip Me', true, false, 'owned', now(), now())`,
        userID,
    )
    if err != nil {
        t.Fatalf("insert external_game: %v", err)
    }
    _, err = testDB.ExecContext(context.Background(),
        `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
         VALUES ('ji-skip-term-1', 'job-skip-term', ?, 'app999', 'Skip Me', 'eg-skip-term', '{}', 'pending_review', '{}', '[]', now())`,
        userID,
    )
    if err != nil {
        t.Fatalf("insert job_item: %v", err)
    }

    rec := postJSONAuth(t, e, "/api/job-items/ji-skip-term-1/skip", map[string]any{}, token)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }

    var jobStatus string
    if err := testDB.QueryRowContext(context.Background(),
        `SELECT status FROM jobs WHERE id = 'job-skip-term'`,
    ).Scan(&jobStatus); err != nil {
        t.Fatalf("query job status: %v", err)
    }
    if jobStatus != "completed" {
        t.Fatalf("expected job status=completed, got %q", jobStatus)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -run TestSkipItem_TerminatesJobWhenLastItem -v
```

Expected: FAIL — job status is `processing`, not `completed`.

- [ ] **Step 3: Export `syncCheckJobCompletion` in `sync.go`**

In `internal/worker/tasks/sync.go`, rename the function signature and all ~15 internal call sites:

```go
// Old — line 1263:
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {

// New:
func SyncCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
```

Then replace every `syncCheckJobCompletion(` call within the same file with `SyncCheckJobCompletion(`.

- [ ] **Step 4: Add `tasks.SyncCheckJobCompletion` call in `HandleSkipItem`**

In `internal/api/job_items.go`, add import of tasks package and add the call at the end of `HandleSkipItem`, just before the `return c.JSON(...)` line:

```go
import (
    // existing imports...
    "github.com/drzero42/nexorious/internal/worker/tasks"
)

// In HandleSkipItem, after the external_game update block (~line 213),
// before return c.JSON(http.StatusOK, map[string]string{"status": "skipped"}):
tasks.SyncCheckJobCompletion(context.Background(), h.db, item.JobID)

return c.JSON(http.StatusOK, map[string]string{"status": "skipped"})
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/api/... -run TestSkipItem -v
go test ./internal/worker/tasks/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/api/job_items.go internal/api/job_items_test.go
git commit -m "fix(sync): export SyncCheckJobCompletion and call it from HandleSkipItem (gap A)"
```

---

## Task 2: Fix gaps B and C — `HandleResolveItem` wrong stage and premature DB write

**Files:**
- Modify: `internal/api/job_items.go`
- Test: `internal/api/job_items_test.go`

**Context:** Currently `HandleResolveItem` calls `retryInsert(... job.JobType ...)` which enqueues `IGDBMatchArgs` (Stage 2). Spec says it must enqueue `UserGameArgs` (Stage 3) directly. Also, `HandleResolveItem` writes `external_game.resolved_igdb_id` for the primary item immediately — spec says Stage 3 does it. Sibling external_game updates STAY (spec push mechanic).

- [ ] **Step 1: Write tests to capture the correct post-condition**

Add to `internal/api/job_items_test.go`:

```go
func TestResolveItem_EnqueuesStage3NotStage2(t *testing.T) {
    truncateAllTables(t)
    e := newTestEchoWithPool(t, testDB)
    ctx := context.Background()

    userID, token := setupTagUser(t, testDB, e, "ji-res-stage")
    rc := newTestRiverClient(t)

    insertJob(t, testDB, "job-res-stage", userID, "sync", "psn", "processing")

    _, err := testDB.ExecContext(ctx,
        `INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
         VALUES ('eg-res-stage', ?, 'psn', 'PPSA-001', 'My Game', true, false, 'owned', now(), now())`,
        userID,
    )
    if err != nil {
        t.Fatalf("insert external_game: %v", err)
    }
    _, err = testDB.ExecContext(ctx,
        `INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
         VALUES ('ji-res-stage-1', 'job-res-stage', ?, 'PPSA-001', 'My Game', 'eg-res-stage', '{}', 'pending_review', '{}', '[]', now())`,
        userID,
    )
    if err != nil {
        t.Fatalf("insert job_item: %v", err)
    }

    // Wire a real River client so we can inspect what gets enqueued.
    h := api.NewJobItemsHandler(testDB, rc)
    _ = h // accessed via e; rebuild e with h if needed — or POST directly and query river_jobs table

    rec := postJSONAuth(t, e, "/api/job-items/ji-res-stage-1/resolve", map[string]any{"igdb_id": 12345}, token)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }

    // Assert external_game.resolved_igdb_id is still NULL (Stage 3 sets it, not the handler).
    var resolvedIGDBID *int
    if err := testDB.QueryRowContext(ctx,
        `SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-res-stage'`,
    ).Scan(&resolvedIGDBID); err != nil {
        t.Fatalf("query external_game: %v", err)
    }
    if resolvedIGDBID != nil {
        t.Fatalf("expected external_game.resolved_igdb_id=NULL, got %v", *resolvedIGDBID)
    }

    // Assert job_item.resolved_igdb_id is set and status is pending.
    var itemStatus string
    var itemIGDBID *int
    if err := testDB.QueryRowContext(ctx,
        `SELECT status, resolved_igdb_id FROM job_items WHERE id = 'ji-res-stage-1'`,
    ).Scan(&itemStatus, &itemIGDBID); err != nil {
        t.Fatalf("query job_item: %v", err)
    }
    if itemStatus != "pending" {
        t.Fatalf("expected job_item status=pending, got %q", itemStatus)
    }
    if itemIGDBID == nil || *itemIGDBID != 12345 {
        t.Fatalf("expected job_item.resolved_igdb_id=12345, got %v", itemIGDBID)
    }

    // Assert a user_game_write river job was enqueued (not igdb_match).
    var kind string
    err = testDB.QueryRowContext(ctx,
        `SELECT args->>'job_item_id', kind FROM river_jobs WHERE kind IN ('user_game_write','igdb_match') ORDER BY id DESC LIMIT 1`,
    ).Scan(new(string), &kind)
    if err != nil {
        t.Fatalf("query river_jobs: %v", err)
    }
    if kind != "user_game_write" {
        t.Fatalf("expected kind=user_game_write, got %q", kind)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/... -run TestResolveItem_EnqueuesStage3NotStage2 -v
```

Expected: FAIL — `external_game.resolved_igdb_id` is not NULL, or kind is `igdb_match`.

- [ ] **Step 3: Fix `HandleResolveItem` in `job_items.go`**

The handler currently (around line 93) does:

```go
// 1. Ensure game row (KEEP)
if _, err := h.db.NewRaw(
    `INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
    body.IGDBID, eg.Title,
).Exec(context.Background()); err != nil { ... }

// 2. Update primary external_game (REMOVE THIS BLOCK):
if _, err := h.db.NewRaw(
    `UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
    body.IGDBID, eg.ID,
).Exec(context.Background()); err != nil { ... }
```

Delete the "Update primary external_game" block entirely. Keep the "Ensure game row" INSERT.

For siblings (around line 148), replace:
```go
var sibJob models.Job
if jErr := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, si.JobID).Scan(context.Background(), &sibJob); jErr == nil {
    retryInsert(context.Background(), h.db, h.riverClient, sibJob.JobType, si.ID)
}
```

With:
```go
if err := tasks.EnqueueOrFail(context.Background(), h.db, h.riverClient, si.ID, tasks.UserGameArgs{JobItemID: si.ID}); err != nil {
    slog.Error("job_items: enqueue sibling Stage 3 failed", "err", err, "job_item_id", si.ID)
}
```

For the primary item (around line 163), replace:
```go
var job models.Job
err = h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, item.JobID).
    Scan(context.Background(), &job)
if err != nil {
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to get parent job")
}
retryInsert(context.Background(), h.db, h.riverClient, job.JobType, itemID)
```

With:
```go
if err := tasks.EnqueueOrFail(context.Background(), h.db, h.riverClient, itemID, tasks.UserGameArgs{JobItemID: itemID}); err != nil {
    slog.Error("job_items: enqueue Stage 3 failed", "err", err, "job_item_id", itemID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue processing")
}
```

The sibling `models.Job` query that was only used to get `sibJob.JobType` for `retryInsert` is now unnecessary — remove it.

- [ ] **Step 4: Update existing resolve tests**

`TestResolveItem` in `job_items_test.go` currently asserts `status=pending` and `resolved_igdb_id=99999` — both still hold. But the test uses an `import` job type which has no external_game, so the external_game block is skipped. The test should still pass without changes.

`TestResolveItem_PropagatesResolutionToSiblings` must now additionally assert that the **primary** external_game has `resolved_igdb_id IS NULL` (Stage 3 sets it), while sibling external_games still have `resolved_igdb_id` set. Update the assertion block if needed.

- [ ] **Step 5: Run all api tests**

```bash
go test ./internal/api/... -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/job_items.go internal/api/job_items_test.go
git commit -m "fix(sync): resolve enqueues Stage 3 directly; remove premature external_game update (gaps B+C)"
```

---

## Task 3: `CleanupSyncChangesWorker` and `SYNC_HISTORY_RETENTION_DAYS` config (gap D)

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/cleanup_test.go`
- Modify: `cmd/nexorious/serve.go` (both wiring blocks)

- [ ] **Step 1: Add `SyncHistoryRetentionDays` to `config.go`**

In `internal/config/config.go`, inside the `Config` struct, add after `StaleJobThreshold`:

```go
// SyncHistoryRetentionDays is the number of days sync_changes rows are kept.
// Rows older than this are deleted by the nightly CleanupSyncChangesWorker.
SyncHistoryRetentionDays int `env:"SYNC_HISTORY_RETENTION_DAYS" envDefault:"90"`
```

- [ ] **Step 2: Write the failing test**

Add to `internal/scheduler/cleanup_test.go`:

```go
func TestCleanupSyncChanges_DeletesOldRows(t *testing.T) {
    truncateAllTables(t)
    ctx := context.Background()
    userID := insertUser(t, ctx, nil)

    // Insert a job so sync_changes can reference it.
    jobID := uuid.NewString()
    _, err := testDB.ExecContext(ctx,
        `INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
         VALUES (?, ?, 'sync', 'steam', 'completed', 'low', now())`,
        jobID, userID,
    )
    if err != nil {
        t.Fatalf("insert job: %v", err)
    }

    // 100-day-old row — should be deleted when retention=90.
    _, err = testDB.ExecContext(ctx,
        `INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Old Game', now() - interval '100 days')`,
        uuid.NewString(), jobID, userID,
    )
    if err != nil {
        t.Fatalf("insert old sync_change: %v", err)
    }
    // 50-day-old row — should remain.
    _, err = testDB.ExecContext(ctx,
        `INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Mid Game', now() - interval '50 days')`,
        uuid.NewString(), jobID, userID,
    )
    if err != nil {
        t.Fatalf("insert mid sync_change: %v", err)
    }
    // 1-day-old row — should remain.
    _, err = testDB.ExecContext(ctx,
        `INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'New Game', now() - interval '1 day')`,
        uuid.NewString(), jobID, userID,
    )
    if err != nil {
        t.Fatalf("insert new sync_change: %v", err)
    }

    scheduler.CleanupSyncChanges(ctx, testDB, 90)

    var count int
    if err := testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_changes`).Scan(&count); err != nil {
        t.Fatalf("count: %v", err)
    }
    if count != 2 {
        t.Fatalf("expected 2 rows remaining, got %d", count)
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/scheduler/... -run TestCleanupSyncChanges_DeletesOldRows -v
```

Expected: FAIL — `scheduler.CleanupSyncChanges` doesn't exist yet.

- [ ] **Step 4: Add `CleanupSyncChangesWorker` to `scheduler.go`**

Add to `internal/scheduler/scheduler.go`:

```go
// ── CleanupSyncChanges ────────────────────────────────────────────────────────

type CleanupSyncChangesArgs struct {
    RetentionDays int `json:"retention_days"`
}

func (CleanupSyncChangesArgs) Kind() string { return "cleanup_sync_changes" }

type CleanupSyncChangesWorker struct {
    river.WorkerDefaults[CleanupSyncChangesArgs]
    DB *bun.DB
}

func (w *CleanupSyncChangesWorker) Work(ctx context.Context, job *river.Job[CleanupSyncChangesArgs]) error {
    CleanupSyncChanges(ctx, w.DB, job.Args.RetentionDays)
    return nil
}

// CleanupSyncChanges deletes sync_changes rows older than retentionDays days.
func CleanupSyncChanges(ctx context.Context, db *bun.DB, retentionDays int) {
    result, err := db.NewRaw(
        `DELETE FROM sync_changes WHERE created_at < now() - ($1 || ' days')::interval`,
        retentionDays,
    ).Exec(ctx)
    if err != nil {
        slog.Error("cleanup: failed to delete old sync_changes", "err", err)
        return
    }
    rows, _ := result.RowsAffected()
    if rows > 0 {
        slog.Info("cleanup: deleted old sync_changes", "count", rows)
    }
}
```

Add to `BuildPeriodicJobs` in `scheduler.go`, inside the return slice:

```go
river.NewPeriodicJob(
    mustCron("0 2 * * *"),
    func() (river.JobArgs, *river.InsertOpts) {
        return CleanupSyncChangesArgs{RetentionDays: cfg.SyncHistoryRetentionDays}, nil
    },
    &river.PeriodicJobOpts{RunOnStart: false},
),
```

- [ ] **Step 5: Register worker in both `serve.go` wiring blocks**

In `cmd/nexorious/serve.go`, in the primary worker block (~line 198) and in the reload path (~line 282), add:

```go
river.AddWorker(workers, &scheduler.CleanupSyncChangesWorker{DB: db})
// (use newDB in the reload path)
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/scheduler/... -run TestCleanupSyncChanges -v
go build ./...
```

Expected: test PASS, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/scheduler/scheduler.go internal/scheduler/cleanup_test.go cmd/nexorious/serve.go
git commit -m "feat(sync): add CleanupSyncChangesWorker with SYNC_HISTORY_RETENTION_DAYS config (gap D)"
```

---

## Task 4: Remove Epic scheduled sync guard (gap H)

**Files:**
- Modify: `internal/scheduler/scheduler.go:151`

- [ ] **Step 1: Remove the guard**

In `CheckPendingSyncsWorker.Work`, delete lines 151–153:

```go
// DELETE THIS BLOCK:
if cfg.Storefront == "epic" {
    continue
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/scheduler/... -v
go build ./...
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat(sync): include epic in scheduled sync (gap H)"
```

---

## Task 5: Rename `RawPlatformToSlug` → `PlatformToSlug`

**Files:**
- Modify: `internal/services/platformresolution/resolution.go`
- Modify: `internal/services/platformresolution/resolution_test.go`
- Modify: `internal/worker/tasks/sync.go` (all callers)

- [ ] **Step 1: Rename in `resolution.go`**

In `internal/services/platformresolution/resolution.go`, change:

```go
// Old:
func RawPlatformToSlug(raw string) (string, bool) {

// New:
func PlatformToSlug(raw string) (string, bool) {
```

- [ ] **Step 2: Update callers in `sync.go`**

```bash
grep -n "RawPlatformToSlug" internal/worker/tasks/sync.go
```

Replace every `platformresolution.RawPlatformToSlug(` with `platformresolution.PlatformToSlug(`.

- [ ] **Step 3: Update the test file**

In `internal/services/platformresolution/resolution_test.go`, replace any call to `RawPlatformToSlug` with `PlatformToSlug`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/services/platformresolution/... -v
go build ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/platformresolution/resolution.go internal/services/platformresolution/resolution_test.go internal/worker/tasks/sync.go
git commit -m "refactor(sync): rename RawPlatformToSlug to PlatformToSlug"
```

---

## Task 6: Define `StorefrontAdapter`, `ExternalGameEntry`, and `ErrCredentials` in `tasks`

**Files:**
- Modify: `internal/worker/tasks/sync.go`

These new types are added alongside the existing four adapter interfaces (which are removed later in Task 11). The build stays green at every step.

- [ ] **Step 1: Add types to `sync.go`**

At the top of `internal/worker/tasks/sync.go`, just before `// SteamLibraryAdapter`, add:

```go
// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    PlaytimeHours   float64  // 0 when the storefront does not provide playtime
    Platforms       []string // storefront-specific names; canonicalised to slugs by the worker
    OwnershipStatus string   // "owned", "subscription", etc.
    IsSubscription  bool
}

// StorefrontAdapter is the interface every storefront adapter must satisfy.
type StorefrontAdapter interface {
    GetLibrary(ctx context.Context, batchSize int, onBatch func([]ExternalGameEntry) error) error
}

// ErrCredentials is returned by an adapter when credentials are invalid,
// expired, or cannot be decrypted. DispatchSyncWorker marks the job failed on this error.
var ErrCredentials = errors.New("credentials error")
```

The `errors` package is already imported in `sync.go`.

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: succeeds (new types are additive).

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "feat(sync): add StorefrontAdapter interface, ExternalGameEntry, and ErrCredentials"
```

---

## Task 7: Steam adapter (gap F)

**Files:**
- Create: `internal/services/steam/adapter.go`

The Steam adapter implements `tasks.StorefrontAdapter`. It calls the existing `GetOwnedGames` + `GetAppDetailsPlatforms` methods, processes games in batches, and brings the global backoff logic from `DispatchSyncWorker.Work` down into the adapter.

- [ ] **Step 1: Create `internal/services/steam/adapter.go`**

```go
package steam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Adapter wraps a Client with pre-configured credentials and implements tasks.StorefrontAdapter.
type Adapter struct {
	client  *Client
	apiKey  string
	steamID string
}

// NewAdapter returns a tasks.StorefrontAdapter for Steam.
func NewAdapter(client *Client, apiKey, steamID string) tasks.StorefrontAdapter {
	return &Adapter{client: client, apiKey: apiKey, steamID: steamID}
}

// GetLibrary fetches the user's Steam library and streams results in batches of batchSize.
// PlaytimeHours in each ExternalGameEntry holds the total for the game; the worker assigns
// it to the first platform row only.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]tasks.ExternalGameEntry) error) error {
	owned, err := a.client.GetOwnedGames(ctx, a.apiKey, a.steamID)
	if err != nil {
		return fmt.Errorf("steam: fetch owned games: %w", err)
	}

	// Global backoff state shared across the game loop.
	backoffs := []time.Duration{2 * time.Minute, 5 * time.Minute}
	backoffIdx := 0

	for start := 0; start < len(owned); start += batchSize {
		end := start + batchSize
		if end > len(owned) {
			end = len(owned)
		}

		var entries []tasks.ExternalGameEntry
		for _, og := range owned[start:end] {
			pl, detErr := a.client.GetAppDetailsPlatforms(ctx, og.AppID)
			if detErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(detErr, ErrRateLimited) && backoffIdx < len(backoffs) {
					d := backoffs[backoffIdx]
					backoffIdx++
					slog.Warn("steam: rate limited, backing off", "wait", d, "appid", og.AppID)
					timer := time.NewTimer(d)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						return ctx.Err()
					}
					pl, detErr = a.client.GetAppDetailsPlatforms(ctx, og.AppID)
				}
				if detErr != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					slog.Warn("steam: appdetails failed, skipping platform update", "appid", og.AppID, "err", detErr)
					continue
				}
			}

			var platforms []string
			if pl.Windows {
				platforms = append(platforms, "pc-windows")
			}
			if pl.Mac {
				platforms = append(platforms, "mac")
			}
			if pl.Linux {
				platforms = append(platforms, "pc-linux")
			}
			if len(platforms) == 0 {
				platforms = []string{"pc-windows"}
			}

			entries = append(entries, tasks.ExternalGameEntry{
				ExternalID:      fmt.Sprintf("%d", og.AppID),
				Title:           og.Title,
				PlaytimeHours:   float64(og.PlaytimeHours),
				Platforms:       platforms,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
		}

		if len(entries) > 0 {
			if err := onBatch(entries); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/services/steam/adapter.go
git commit -m "feat(sync): add steam.Adapter implementing StorefrontAdapter (gap F)"
```

---

## Task 8: PSN adapter — rename `RawPlatform` → `Platforms []string` and add adapter

**Files:**
- Modify: `internal/services/psn/client.go`
- Create: `internal/services/psn/adapter.go`

- [ ] **Step 1: Rename `RawPlatform` → `Platforms []string` in `client.go`**

In `internal/services/psn/client.go`, change the `ExternalGameEntry` struct:

```go
// Old:
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    RawPlatform     string
    PlaytimeHours   int
    OwnershipStatus string
    IsSubscription  bool
}

// New:
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    Platforms       []string // single element per entry; PSN creates one ExternalGame per title ID
    PlaytimeHours   int
    OwnershipStatus string
    IsSubscription  bool
}
```

Update all assignment sites in `fetchPlayHistory` and `fetchPurchasedGames` — each currently sets `RawPlatform: rawPlatform`. Change to `Platforms: []string{rawPlatform}`.

- [ ] **Step 2: Create `internal/services/psn/adapter.go`**

```go
package psn

import (
	"context"
	"errors"
	"fmt"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Adapter wraps a Client with a pre-configured NPSSO token and implements tasks.StorefrontAdapter.
type Adapter struct {
	client     *Client
	npssoToken string
}

// NewAdapter returns a tasks.StorefrontAdapter for PSN.
func NewAdapter(client *Client, npssoToken string) tasks.StorefrontAdapter {
	return &Adapter{client: client, npssoToken: npssoToken}
}

func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]tasks.ExternalGameEntry) error) error {
	err := a.client.GetLibrary(ctx, a.npssoToken, batchSize, func(entries []ExternalGameEntry) error {
		mapped := make([]tasks.ExternalGameEntry, 0, len(entries))
		for _, e := range entries {
			mapped = append(mapped, tasks.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   float64(e.PlaytimeHours),
				Platforms:       e.Platforms,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}
		return onBatch(mapped)
	})
	if errors.Is(err, ErrInvalidNPSSOToken) || errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		return fmt.Errorf("%w: %v", tasks.ErrCredentials, err)
	}
	return err
}
```

- [ ] **Step 3: Build and run PSN tests**

```bash
go build ./...
go test ./internal/services/psn/... -v
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/services/psn/client.go internal/services/psn/adapter.go
git commit -m "feat(sync): add psn.Adapter implementing StorefrontAdapter; rename RawPlatform to Platforms"
```

---

## Task 9: GOG adapter — consolidate per-game entries and add adapter (gap G)

**Files:**
- Modify: `internal/services/gog/library.go`
- Create: `internal/services/gog/adapter.go`

**Context:** GOG currently emits one `ExternalGameEntry` per platform per product (e.g., two entries for a Windows+Linux game). The spec requires one entry per game with `Platforms []string`. The `dispatchedInBatch` deduplication in `DispatchSyncWorker` was a workaround for this — it disappears after consolidation.

- [ ] **Step 1: Rename `RawPlatform` → `Platforms []string` in `library.go`**

Change the struct:

```go
// Old:
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    RawPlatform     string
    PlaytimeHours   int
    OwnershipStatus string
    IsSubscription  bool
}

// New:
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    Platforms       []string // all platforms this product runs on
    PlaytimeHours   int
    OwnershipStatus string
    IsSubscription  bool
}
```

- [ ] **Step 2: Rewrite `fetchPage` to consolidate per product**

Replace the body of `fetchPage` starting from the `entries` allocation through the end:

```go
// Old code emitted one ExternalGameEntry per platform:
entries := make([]ExternalGameEntry, 0, len(body.Products)*2)
for _, p := range body.Products {
    id := strconv.FormatInt(p.ID, 10)
    if p.WorksOn.Windows {
        entries = append(entries, ExternalGameEntry{..., RawPlatform: "pc-windows"})
    }
    if p.WorksOn.Mac {
        entries = append(entries, ExternalGameEntry{..., RawPlatform: "pc-mac"})
    }
    if p.WorksOn.Linux {
        entries = append(entries, ExternalGameEntry{..., RawPlatform: "pc-linux"})
    }
}

// New code: one entry per product with all platforms:
entries := make([]ExternalGameEntry, 0, len(body.Products))
for _, p := range body.Products {
    id := strconv.FormatInt(p.ID, 10)
    var platforms []string
    if p.WorksOn.Windows {
        platforms = append(platforms, "pc-windows")
    }
    if p.WorksOn.Mac {
        platforms = append(platforms, "pc-mac")
    }
    if p.WorksOn.Linux {
        platforms = append(platforms, "pc-linux")
    }
    if len(platforms) == 0 {
        platforms = []string{"pc-windows"}
    }
    entries = append(entries, ExternalGameEntry{
        ExternalID:      id,
        Title:           p.Title,
        Platforms:       platforms,
        PlaytimeHours:   0,
        OwnershipStatus: "owned",
        IsSubscription:  false,
    })
}
```

Also update the `GetLibrary` doc comment — remove the mention of "two entries for games on both Windows and Linux".

- [ ] **Step 3: Create `internal/services/gog/adapter.go`**

```go
package gog

import (
	"context"
	"errors"
	"fmt"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// Adapter wraps a Client with a pre-configured access token and implements tasks.StorefrontAdapter.
type Adapter struct {
	client      *Client
	accessToken string
}

// NewAdapter returns a tasks.StorefrontAdapter for GOG.
func NewAdapter(client *Client, accessToken string) tasks.StorefrontAdapter {
	return &Adapter{client: client, accessToken: accessToken}
}

func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]tasks.ExternalGameEntry) error) error {
	err := a.client.GetLibrary(ctx, a.accessToken, batchSize, func(entries []ExternalGameEntry) error {
		mapped := make([]tasks.ExternalGameEntry, 0, len(entries))
		for _, e := range entries {
			mapped = append(mapped, tasks.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   float64(e.PlaytimeHours),
				Platforms:       e.Platforms,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}
		return onBatch(mapped)
	})
	if errors.Is(err, ErrGOGAuthExpired) {
		return fmt.Errorf("%w: %v", tasks.ErrCredentials, err)
	}
	return err
}
```

- [ ] **Step 4: Build and run GOG tests**

```bash
go build ./...
go test ./internal/services/gog/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/gog/library.go internal/services/gog/adapter.go
git commit -m "feat(sync): add gog.Adapter implementing StorefrontAdapter; consolidate per-game entries (gap G)"
```

---

## Task 10: Update `EpicClientAdapter` to implement `StorefrontAdapter`

**Files:**
- Modify: `internal/worker/tasks/sync.go` — `EpicClientAdapter` section

**Context:** `EpicClientAdapter` currently has `GetLibrary(ctx, userID string, onBatch func([]epicsvc.ExternalGameEntry) error)`. It must become `GetLibrary(ctx, _ int, onBatch func([]ExternalGameEntry) error)` to satisfy the new `StorefrontAdapter` interface. `UserID` moves to a struct field so the factory can pre-populate it.

- [ ] **Step 1: Update `EpicClientAdapter` in `sync.go`**

Add `UserID string` field:

```go
// Old:
type EpicClientAdapter struct {
    Client    epicSubprocessClient
    DB        *bun.DB
    Encrypter *crypto.Encrypter
}

// New:
type EpicClientAdapter struct {
    Client    epicSubprocessClient
    DB        *bun.DB
    Encrypter *crypto.Encrypter
    UserID    string // set by the adapter factory; used in place of the old userID parameter
}
```

Change `epicSubprocessClient` interface's `GetLibrary` to use the (already-renamed) `epicsvc.ExternalGameEntry`:

```go
// This likely already compiles since epic package already uses ExternalGameEntry.
// Verify it matches:
type epicSubprocessClient interface {
    Configured() bool
    RestoreSnapshot(userID string, snapshot map[string]string) error
    GetLibrary(ctx context.Context, userID string, onBatch func([]epicsvc.ExternalGameEntry) error) error
    CaptureSnapshot(userID string) (map[string]string, error)
}
```

Change `EpicClientAdapter.GetLibrary` signature and body:

```go
// Old signature:
func (a *EpicClientAdapter) GetLibrary(ctx context.Context, userID string, onBatch func([]epicsvc.ExternalGameEntry) error) error {

// New signature (implements StorefrontAdapter):
func (a *EpicClientAdapter) GetLibrary(ctx context.Context, _ int, onBatch func([]ExternalGameEntry) error) error {
```

Inside the function, replace the `fetchErr := a.Client.GetLibrary(ctx, userID, onBatch)` call with:

```go
fetchErr := a.Client.GetLibrary(ctx, a.UserID, func(batch []epicsvc.ExternalGameEntry) error {
    mapped := make([]ExternalGameEntry, 0, len(batch))
    for _, e := range batch {
        mapped = append(mapped, ExternalGameEntry{
            ExternalID:      e.ExternalID,
            Title:           e.Title,
            PlaytimeHours:   0,
            Platforms:       []string{"pc-windows"},
            OwnershipStatus: e.OwnershipStatus,
            IsSubscription:  false,
        })
    }
    return onBatch(mapped)
})
```

Also wrap the two credential failure returns to use `ErrCredentials`:

```go
// Old:
if err := a.DB.NewRaw(...).Scan(ctx, &ciphertextStr); err != nil || ciphertextStr == "" {
    return fmt.Errorf("epic: no legendary state found for user (not connected)")
}
plainState, err := a.Encrypter.Decrypt(ciphertextStr)
if err != nil {
    slog.Warn("epic: legendary state decrypt failed", ...)
    return fmt.Errorf("epic: legendary state decrypt failed")
}

// New:
if err := a.DB.NewRaw(...).Scan(ctx, &ciphertextStr); err != nil || ciphertextStr == "" {
    return fmt.Errorf("%w: epic legendary state not found", ErrCredentials)
}
plainState, err := a.Encrypter.Decrypt(ciphertextStr)
if err != nil {
    slog.Warn("epic: legendary state decrypt failed", "user_id", a.UserID, "err", err)
    return fmt.Errorf("%w: epic legendary state decrypt failed", ErrCredentials)
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: succeeds. (`EpicClientAdapter` now satisfies `StorefrontAdapter` but the old 4 interfaces still exist — no conflicts.)

- [ ] **Step 3: Run tasks tests**

```bash
go test ./internal/worker/tasks/... -v
```

Expected: PASS (epic adapter tests use `fakeEpicSubprocessClient` which is unchanged).

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "feat(sync): update EpicClientAdapter to implement StorefrontAdapter"
```

---

## Task 11: Rewrite `DispatchSyncWorker` — remove old interfaces and unified `Work`

**Files:**
- Modify: `internal/worker/tasks/sync.go`

This is the big refactor step. The four separate interfaces (`SteamLibraryAdapter`, `PSNLibraryAdapter`, `EpicLibraryAdapter`, `GOGLibraryAdapter`) and the ~600-line switch are removed. The struct gains an `Adapter` factory field. `Work` becomes storefront-agnostic.

- [ ] **Step 1: Remove old adapter interfaces and imports**

Delete from `sync.go`:
- `SteamLibraryAdapter` interface (lines ~29–32)
- `PSNLibraryAdapter` interface (lines ~35–37)
- `psnLibraryBatchSize` const
- `EpicLibraryAdapter` interface (lines ~41–48)
- `GOGLibraryAdapter` interface (lines ~51–55)

Remove from the `import` block: `psnsvc`, `steamsvc`, `gogsvc`. Keep: `epicsvc` (EpicClientAdapter), `igdbsvc` (IGDBMatchWorker), `matching`, `platformresolution`, `crypto`.

- [ ] **Step 2: Update `DispatchSyncWorker` struct**

```go
// Old:
type DispatchSyncWorker struct {
    river.WorkerDefaults[DispatchSyncArgs]
    DB          *bun.DB
    Encrypter   *crypto.Encrypter
    Steam       SteamLibraryAdapter
    PSN         PSNLibraryAdapter
    Epic        EpicLibraryAdapter
    GOG         GOGLibraryAdapter
    RiverClient *river.Client[pgx.Tx]
}

// New:
type DispatchSyncWorker struct {
    river.WorkerDefaults[DispatchSyncArgs]
    DB          *bun.DB
    Adapter     func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (StorefrontAdapter, error)
    RiverClient *river.Client[pgx.Tx]
}
```

- [ ] **Step 3: Add helper functions**

Add these four helpers before the `Work` function:

```go
func resolvePlatforms(platforms []string) []string {
    var resolved []string
    for _, p := range platforms {
        if slug, ok := platformresolution.PlatformToSlug(p); ok {
            resolved = append(resolved, slug)
        }
    }
    if len(resolved) == 0 {
        resolved = []string{"pc-windows"}
    }
    return resolved
}

func upsertExternalGame(ctx context.Context, db *bun.DB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool) {
    var row struct {
        ID        string `bun:"id"`
        IsSkipped bool   `bun:"is_skipped"`
    }
    if err := db.NewRaw(`
        INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, true, ?, ?, now(), now())
        ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
            title = EXCLUDED.title,
            is_subscription = EXCLUDED.is_subscription,
            ownership_status = EXCLUDED.ownership_status,
            is_available = true,
            updated_at = now()
        RETURNING id, is_skipped`,
        uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
        e.IsSubscription, e.OwnershipStatus,
    ).Scan(ctx, &row); err != nil {
        slog.Error("dispatch_sync: upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", e.ExternalID)
        return "", false
    }
    return row.ID, row.IsSkipped
}

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

func insertJobItem(ctx context.Context, db *bun.DB, egID string, e ExternalGameEntry, p DispatchSyncArgs) {
    if _, err := db.NewRaw(`
        INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
        VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
        ON CONFLICT (job_id, item_key) DO NOTHING`,
        uuid.NewString(), p.JobID, p.UserID, e.ExternalID, e.Title, egID,
    ).Exec(ctx); err != nil {
        slog.Error("dispatch_sync: insert job_item failed", "err", err, "job_id", p.JobID, "external_id", e.ExternalID)
    }
}

func enqueueBatch(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobID string) {
    var items []struct {
        ID string `bun:"id"`
    }
    if err := db.NewRaw(
        `SELECT id FROM job_items WHERE job_id = ? AND status = 'pending'`,
        jobID,
    ).Scan(ctx, &items); err != nil {
        slog.Error("dispatch_sync: enqueueBatch query", "job_id", jobID, "err", err)
        return
    }
    for _, item := range items {
        _ = EnqueueOrFail(ctx, db, rc, item.ID, IGDBMatchArgs{JobItemID: item.ID})
    }
}
```

- [ ] **Step 4: Rewrite `Work`**

Replace the entire existing `Work` function body with:

```go
func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
    p := job.Args

    // 1. Mark job as processing.
    now := time.Now().UTC()
    if _, err := w.DB.NewRaw(
        `UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
        now, p.JobID,
    ).Exec(ctx); err != nil {
        slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
    }

    // 2. Load sync config.
    var cfg models.UserSyncConfig
    if err := w.DB.NewSelect().Model(&cfg).
        Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
        Scan(ctx); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "no sync config found")
        return nil
    }

    // 3. Build adapter (credential loading, decryption, and token refresh happen inside).
    adapter, err := w.Adapter(ctx, p.Storefront, cfg)
    if errors.Is(err, ErrCredentials) {
        failSyncJob(ctx, w.DB, p.JobID, "credentials error")
        return nil
    }
    if err != nil {
        failSyncJob(ctx, w.DB, p.JobID, err.Error())
        return nil
    }

    fetchedIDs := make(map[string]struct{})
    seenPlatforms := make(map[string][]string) // egID → canonical platform slugs

    // 4. Fetch library; upsert external_games + platforms; insert job_items.
    slog.Info("dispatch_sync: starting library fetch", "job_id", p.JobID, "user_id", p.UserID, "storefront", p.Storefront)
    if err := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
        for _, e := range batch {
            fetchedIDs[e.ExternalID] = struct{}{}
            platforms := resolvePlatforms(e.Platforms)
            egID, isSkipped := upsertExternalGame(ctx, w.DB, e, p)
            if egID == "" {
                continue
            }
            seenPlatforms[egID] = append(seenPlatforms[egID], platforms...)
            upsertPlatforms(ctx, w.DB, egID, platforms, e.PlaytimeHours)
            if !isSkipped {
                insertJobItem(ctx, w.DB, egID, e, p)
            }
        }
        return nil
    }); err != nil {
        if errors.Is(err, ErrCredentials) {
            failSyncJob(ctx, w.DB, p.JobID, "credentials error")
            return nil
        }
        slog.Error("dispatch_sync: library fetch failed", "job_id", p.JobID, "err", err)
        failSyncJob(ctx, w.DB, p.JobID, err.Error())
        return nil
    }

    // 5. Enqueue Stage 2 (IGDBMatch) for all pending items in this job (deferred pattern).
    enqueueBatch(ctx, w.DB, w.RiverClient, p.JobID)

    // 6. Stale platform sweep: remove platform rows no longer present upstream.
    for egID, platforms := range seenPlatforms {
        if _, err := w.DB.NewRaw(`
            DELETE FROM external_game_platforms
            WHERE external_game_id = ? AND platform NOT IN (?)`,
            egID, bun.List(platforms),
        ).Exec(ctx); err != nil {
            slog.Error("dispatch_sync: delete stale platforms failed", "err", err, "external_game_id", egID)
        }
    }

    // 7. Mark removed games as unavailable and write sync_changes('removed').
    var available []models.ExternalGame
    if err := w.DB.NewSelect().Model(&available).
        Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
        Scan(ctx); err != nil {
        slog.Error("dispatch_sync: query available games failed", "err", err, "job_id", p.JobID)
    }
    for _, eg := range available {
        if _, found := fetchedIDs[eg.ExternalID]; !found {
            if _, err := w.DB.NewRaw(
                `UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
                eg.ID,
            ).Exec(ctx); err != nil {
                slog.Error("dispatch_sync: mark game unavailable failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
            }
            if _, err := w.DB.NewRaw(
                `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
                 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
                uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
            ).Exec(ctx); err != nil {
                slog.Error("dispatch_sync: insert sync_change (removed) failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
            }
        }
    }

    // 8. Update last_synced_at.
    syncedNow := time.Now().UTC()
    if _, err := w.DB.NewRaw(
        `UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
        syncedNow, p.UserID, p.Storefront,
    ).Exec(context.Background()); err != nil {
        slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
    }

    return nil
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: FAIL — `sync_test.go` still references the old struct fields (`Steam`, `PSN`, `Epic`, `GOG`, `Encrypter`). That's fixed in Task 12.

- [ ] **Step 6: Commit (after Task 12 passes build)**

Hold this commit until Task 12 makes the build green, then commit both together.

---

## Task 12: Update `serve.go` and `sync_test.go`

**Files:**
- Modify: `cmd/nexorious/serve.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Add `buildAdapterFactory` to `serve.go`**

Add the following function at the bottom of `cmd/nexorious/serve.go` (still in `package main`). It references `db`, `encrypter`, and `epicClient` passed as parameters so it works for both the primary and reload wiring blocks:

```go
func buildAdapterFactory(
    db *bun.DB,
    encrypter *crypto.Encrypter,
    epicClient *epicsvc.Client,
) func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
    return func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
        switch storefront {
        case "steam":
            if cfg.StorefrontCredentials == nil {
                return nil, tasks.ErrCredentials
            }
            plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
            if err != nil {
                slog.Warn("adapter factory: steam decrypt failed", "user_id", cfg.UserID, "err", err)
                return nil, tasks.ErrCredentials
            }
            var creds struct {
                WebAPIKey string `json:"web_api_key"`
                SteamID   string `json:"steam_id"`
            }
            if err := json.Unmarshal(plain, &creds); err != nil {
                return nil, tasks.ErrCredentials
            }
            return steamsvc.NewAdapter(steamsvc.NewClient(), creds.WebAPIKey, creds.SteamID), nil

        case "psn":
            if cfg.StorefrontCredentials == nil {
                return nil, tasks.ErrCredentials
            }
            plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
            if err != nil {
                slog.Warn("adapter factory: psn decrypt failed", "user_id", cfg.UserID, "err", err)
                return nil, tasks.ErrCredentials
            }
            var creds struct {
                NPSSOToken string `json:"npsso_token"`
            }
            if err := json.Unmarshal(plain, &creds); err != nil {
                return nil, tasks.ErrCredentials
            }
            return psnsvc.NewAdapter(psnsvc.NewClient(), creds.NPSSOToken), nil

        case "gog":
            if cfg.StorefrontCredentials == nil {
                return nil, tasks.ErrCredentials
            }
            plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
            if err != nil {
                slog.Warn("adapter factory: gog decrypt failed", "user_id", cfg.UserID, "err", err)
                return nil, tasks.ErrCredentials
            }
            var creds struct {
                AccessToken  string `json:"access_token"`
                RefreshToken string `json:"refresh_token"`
                UserID       string `json:"user_id"`
                Username     string `json:"username"`
            }
            if err := json.Unmarshal(plain, &creds); err != nil {
                return nil, tasks.ErrCredentials
            }
            gogClient := gogsvc.NewClient()
            newTok, err := gogClient.RefreshToken(ctx, creds.RefreshToken)
            if err != nil {
                slog.Warn("adapter factory: gog token refresh failed", "user_id", cfg.UserID, "err", err)
                return nil, tasks.ErrCredentials
            }
            creds.AccessToken = newTok.AccessToken
            creds.RefreshToken = newTok.RefreshToken
            if newCredsJSON, merr := json.Marshal(creds); merr == nil {
                enc, encErr := encrypter.Encrypt(newCredsJSON)
                if encErr != nil {
                    slog.Error("adapter factory: encrypt refreshed gog token failed", "err", encErr, "user_id", cfg.UserID)
                } else if _, err := db.NewRaw(
                    `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
                    enc, cfg.UserID,
                ).Exec(ctx); err != nil {
                    slog.Error("adapter factory: persist refreshed gog token failed", "err", err, "user_id", cfg.UserID)
                }
            }
            return gogsvc.NewAdapter(gogClient, newTok.AccessToken), nil

        case "epic":
            return &tasks.EpicClientAdapter{
                Client:    epicClient,
                DB:        db,
                Encrypter: encrypter,
                UserID:    cfg.UserID,
            }, nil

        default:
            return nil, fmt.Errorf("unknown storefront: %s", storefront)
        }
    }
}
```

Add `"encoding/json"` to the import block if not already present, and add `"github.com/drzero42/nexorious/internal/db/models"`.

- [ ] **Step 2: Update primary wiring block in `serve.go` (~line 172)**

Replace:

```go
dispatchSyncWorker := &tasks.DispatchSyncWorker{
    DB:        db,
    Encrypter: encrypter,
    Steam:     steamsvc.NewClient(),
    PSN:       psnsvc.NewClient(),
    Epic:      &tasks.EpicClientAdapter{Client: epicsvc.NewClient(cfg.LegendaryWorkDir), DB: db, Encrypter: encrypter},
    GOG:       gogsvc.NewClient(),
}
```

With:

```go
epicClient := epicsvc.NewClient(cfg.LegendaryWorkDir)
dispatchSyncWorker := &tasks.DispatchSyncWorker{
    DB:      db,
    Adapter: buildAdapterFactory(db, encrypter, epicClient),
}
```

- [ ] **Step 3: Update reload path wiring block in `serve.go` (~line 250)**

Replace:

```go
newDispatchSync := &tasks.DispatchSyncWorker{
    DB:        newDB,
    Encrypter: encrypter,
    Steam:     steamsvc.NewClient(),
    PSN:       psnsvc.NewClient(),
    Epic:      &tasks.EpicClientAdapter{Client: epicsvc.NewClient(cfg.LegendaryWorkDir), DB: newDB, Encrypter: encrypter},
    GOG:       gogsvc.NewClient(),
}
```

With:

```go
newEpicClient := epicsvc.NewClient(cfg.LegendaryWorkDir)
newDispatchSync := &tasks.DispatchSyncWorker{
    DB:      newDB,
    Adapter: buildAdapterFactory(newDB, encrypter, newEpicClient),
}
```

- [ ] **Step 4: Collapse fake adapters in `sync_test.go`**

Replace the three fake adapter types with one:

```go
// Replace fakeSteamAdapter, fakePSNAdapter, and fakeEpicAdapter with:

type fakeStorefrontAdapter struct {
    batches [][]tasks.ExternalGameEntry
    err     error
}

func (f *fakeStorefrontAdapter) GetLibrary(_ context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
    if f.err != nil {
        return f.err
    }
    for _, batch := range f.batches {
        if err := onBatch(batch); err != nil {
            return err
        }
    }
    return nil
}

// adapterFactory returns a factory that always returns the given adapter regardless of storefront.
func adapterFactory(adapter tasks.StorefrontAdapter) func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
    return func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
        return adapter, nil
    }
}

// credErrFactory returns a factory that always returns ErrCredentials.
func credErrFactory() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
    return func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
        return nil, tasks.ErrCredentials
    }
}
```

Remove the imports of `steamsvc`, `psnsvc`, `gogsvc`, `epicsvc` from the test file's import block. Add `"github.com/drzero42/nexorious/internal/db/models"` import.

- [ ] **Step 5: Update all test instantiations in `sync_test.go`**

Every `&tasks.DispatchSyncWorker{..., Steam: ..., PSN: ..., Epic: ..., GOG: ..., Encrypter: ...}` must be replaced.

**Pattern for tests that just need a no-op adapter:**
```go
// Old:
w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: &fakeSteamAdapter{}, RiverClient: nil}

// New:
w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), RiverClient: nil}
```

**Pattern for tests with specific game data (e.g., steam tests):**
```go
// Old:
adapter := &fakeSteamAdapter{
    games: []steamsvc.OwnedGame{{AppID: 570, Title: "Dota 2", PlaytimeHours: 100}},
}
w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: nil}

// New:
fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
    {{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 100, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
}}
w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
```

**Pattern for credential error tests:**
```go
// Old (any storefront with nil/invalid credentials):
// (worker would try to decrypt from DB and fail)

// New:
w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
```

Go through every test in `sync_test.go` and apply the appropriate pattern. The `queriedAppIDs` field from `fakeSteamAdapter` was used in platform-related assertions — those tests must be rewritten to assert against DB state instead (query `external_game_platforms` after `Work` completes).

The `fakeEpicSubprocessClient` and its associated tests remain unchanged (they test `EpicClientAdapter.GetLibrary` directly, not through `DispatchSyncWorker`).

Also remove any test setup code that inserted `storefront_credentials` into the DB purely to enable credential decryption — it's no longer needed since the factory is faked.

- [ ] **Step 6: Build and run all tests**

```bash
go build ./...
go test ./... -timeout 600s
```

Expected: all PASS.

- [ ] **Step 7: Commit Tasks 11 and 12 together**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go cmd/nexorious/serve.go
git commit -m "refactor(sync): unified StorefrontAdapter; rewrite DispatchSyncWorker; update serve wiring (gaps E-G)"
```

---

## Self-Review

**Spec coverage check:**

| Gap | Task | Covered? |
|-----|------|----------|
| A — HandleSkipItem missing completion check | 1 | ✅ |
| B — Resolve enqueues Stage 3 directly | 2 | ✅ |
| C — Don't update external_game at resolve time | 2 | ✅ |
| D — sync_changes pruning worker | 3 | ✅ |
| E — Unified StorefrontAdapter interface | 6, 11, 12 | ✅ |
| F — Steam uses batch callback pattern | 7 | ✅ |
| G — GOG consolidates per-game, batch size ≤10 | 9 | ✅ |
| H — Epic included in scheduled sync | 4 | ✅ |
| RawPlatformToSlug rename | 5 | ✅ |

**Type consistency check:** `ExternalGameEntry` defined in Task 6 is used by Tasks 7–10 as `tasks.ExternalGameEntry`. In Tasks 8 and 9, the local service-package `ExternalGameEntry` is mapped to the tasks one inside the adapters. No cross-task naming drift.

**Placeholder check:** All steps have concrete code. No "TBD" or "implement later".

**Import cycle:** After Task 11, `tasks` no longer imports `steamsvc`/`psnsvc`/`gogsvc`. Those packages import `tasks`. The cycle is resolved.
