# Metadata Refresh — Design Spec

## Overview

Phase 4 specified a metadata refresh job but it was deferred from the sync API implementation plan. This spec covers the missing pieces:

- A `metadata_refresh_dispatch` worker task that selects all games and queues per-game refresh tasks.
- A `metadata_refresh_item` worker task that fetches current metadata from IGDB and updates the `games` row.
- A scheduler job that submits `metadata_refresh_dispatch` on a configurable interval.
- A minor addition to `GameMetadata` to expose `CoverImageID` without URL parsing.

Jobs and items are user-visible in the existing jobs/job_items UI. No new API endpoints or migration files are required. The existing initial migration is edited to drop the unused `estimated_playtime_hours` column (see the Estimated Playtime Cleanup section).

---

## Constraints and Invariants

- All games in the `games` table have an IGDB ID as their primary key. Every row is a valid refresh target.
- Metadata refresh only runs when IGDB is configured (`igdbClient.Configured()`). If IGDB is absent the scheduler still fires but the dispatch task returns immediately without creating a job.
- The `jobs` table requires a non-null `user_id`. System-initiated jobs use the admin user's ID (queried at dispatch time). If no admin exists, the task skips with a warning.
- Only one metadata refresh job may be active at a time. The dispatch task checks for an existing `pending` or `processing` job and skips if found.
- `METADATA_REFRESH_INTERVAL` is already defined in `internal/config/config.go` with default `"24h"`. No config changes are needed.
- `JobTypeMetadataRefresh = "metadata_refresh"` is already defined in `internal/db/models/jobs.go`. No model changes needed.

---

## IGDB Models Change

**File:** `internal/services/igdb/models.go`

Add one field to `GameMetadata` and remove the unused `EstimatedPlaytimeHours` field:

```go
CoverImageID string // IGDB image_id, e.g. "co1wyy". Empty when no cover.
```

Remove:
```go
EstimatedPlaytimeHours *int32
```

`EstimatedPlaytimeHours` has no data source — it is never populated by `FetchFullMetadata`, `convertToGameMetadata`, or any other code path — and is not displayed anywhere in the UI. It is being removed in full (see Estimated Playtime Cleanup section).

**File:** `internal/services/igdb/igdb.go`

In `convertToGameMetadata`, populate both cover fields together:

```go
if g.Cover != nil && g.Cover.ImageID != "" {
    md.CoverImageID = g.Cover.ImageID
    url := igdbImageBaseURL + g.Cover.ImageID + ".jpg"
    md.CoverArtURL = &url
}
```

**File:** `internal/worker/tasks/import_item.go`

Fix `igdbMetadataToGame`: the existing code stores `igdb_platform_ids` and `igdb_platform_names` as comma-joined strings (`strings.Join`), which is inconsistent with the DB schema comment (`-- JSON array as text`). Change both to use `json.Marshal`, matching the format the refresh task uses:

```go
// Before (wrong — comma-joined):
s := strings.Join(ids, ",")
game.IgdbPlatformIds = &s

// After (correct — JSON array):
b, _ := json.Marshal(ids)
s := string(b)
game.IgdbPlatformIds = &s
```

Apply the same fix for `IgdbPlatformNames`. This corrects a latent inconsistency; games imported before this fix will have comma-join format in the DB, but will be normalised to JSON on their next metadata refresh.

> **Why this is safe:** `igdb_platform_ids` and `igdb_platform_names` are stored-only fields — they are not rendered anywhere in the UI and are not parsed by any Go code path outside of the write paths in `import_item.go` and `metadata_refresh.go`. The mixed comma-join/JSON format in the DB during the transition period causes no observable problem. No frontend changes are required.

These are the only changes outside `internal/worker/tasks/metadata_refresh.go` and `internal/scheduler/` that are part of the core refresh logic. All existing callers of `GameMetadata` that do not touch `EstimatedPlaytimeHours` are unaffected.

---

## Estimated Playtime Cleanup

`estimated_playtime_hours` is a dead field. It exists in the DB schema, Go models, and frontend types but has no data source and is never rendered in the UI. Remove it everywhere.

**Migration:** Edit the existing initial migration (`internal/db/migrations/20260503000001_initial.up.sql`) — remove the `estimated_playtime_hours` column from the `CREATE TABLE games` statement. No new migration file is needed; drop and recreate the dev DB after the change.

**File:** `internal/db/models/models.go`

Remove the `EstimatedPlaytimeHours` field from `Game`.

**File:** `ui/frontend/src/api/games.ts`

Remove `estimated_playtime_hours?: number` from the API response type, and remove its pass-through in the game mapper function (`apiGame.estimated_playtime_hours`).

**File:** `ui/frontend/src/types/game.ts`

Remove `estimated_playtime_hours?: number` from the domain `Game` type.

**Test fixtures**

Remove `estimated_playtime_hours` from the mock game objects in:
- `ui/frontend/src/api/games.test.ts`
- `ui/frontend/src/hooks/use-games.test.tsx`

---

## Scheduler Change

**File:** `internal/scheduler/scheduler.go`

`NewScheduler` gains a `*config.Config` parameter and stores the parsed duration on the struct:

```go
type Scheduler struct {
    db                      *bun.DB
    pool                    *worker.Pool
    backupSvc               *backup.Service
    metadataRefreshInterval time.Duration
    scheduler               gocron.Scheduler
    backupJob               gocron.Job
}

func NewScheduler(db *bun.DB, pool *worker.Pool, backupSvc *backup.Service, cfg *config.Config) *Scheduler {
    interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
    if err != nil {
        slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
            "value", cfg.MetadataRefreshInterval, "err", err)
        interval = 24 * time.Hour
    }
    return &Scheduler{
        db:                      db,
        pool:                    pool,
        backupSvc:               backupSvc,
        metadataRefreshInterval: interval,
    }
}
```

In `Start`, register the new job after the existing `CheckPendingSyncs` job:

```go
// Metadata refresh dispatch — configurable interval.
_, _ = s.scheduler.NewJob(
    gocron.DurationJob(s.metadataRefreshInterval),
    gocron.NewTask(func() {
        _ = s.pool.Submit(ctx, "metadata_refresh_dispatch", nil, 1)
    }),
)
```

Priority `1` is low (same as the `CheckPendingSyncs`-dispatched sync jobs).

**File:** `cmd/nexorious/main.go`

Both `scheduler.NewScheduler` call sites (initial start and `RebuildServices` restore callback) gain the `cfg` argument:

```go
sched = scheduler.NewScheduler(db, pool, backupSvc, cfg)
```

---

## Worker Task: `metadata_refresh_dispatch`

**File:** `internal/worker/tasks/metadata_refresh.go` (new file)

### Constructor

```go
func NewMetadataRefreshDispatchHandler(
    db         *bun.DB,
    pool       *worker.Pool,
    igdbClient *igdbsvc.Client,
) func(ctx context.Context, task *models.PendingTask) error
```

### Algorithm

**Step 1 — IGDB guard.**
If `!igdbClient.Configured()`: log `slog.Warn("metadata_refresh_dispatch: IGDB not configured, skipping")`, return nil. No job is created.

**Step 2 — Find admin user.**
```sql
SELECT id FROM users WHERE is_admin = true LIMIT 1
```
If `sql.ErrNoRows`: log `slog.Warn("metadata_refresh_dispatch: no admin user found, skipping")`, return nil.

**Step 3 — Duplicate-run guard.**
```sql
SELECT id FROM jobs
WHERE job_type = 'metadata_refresh'
  AND status IN ('pending', 'processing')
LIMIT 1
```
If a row is found: log `slog.Info("metadata_refresh_dispatch: job already active, skipping")`, return nil.

**Step 4 — Select games.**
```sql
SELECT id, title FROM games ORDER BY last_updated ASC
```
Ordering by `last_updated ASC` ensures stalest games are processed first. If the result is empty: return nil (no job created).

**Step 5 — Create job.**
Insert into `jobs`:
- `id`: new UUID
- `user_id`: admin user ID from step 2
- `job_type`: `'metadata_refresh'`
- `source`: `'system'`
- `status`: `'pending'`
- `priority`: `'low'`
- `total_items`: `len(games)`
- `created_at`: `now()`

**Step 6 — Create job_items and pending_tasks.**
For each game (in `last_updated ASC` order):

Insert `job_items`:
- `id`: new UUID (saved as `itemID`)
- `job_id`: job ID from step 5
- `user_id`: admin user ID
- `item_key`: `strconv.Itoa(int(game.ID))` — IGDB integer ID as string
- `source_title`: `game.Title`
- `source_metadata`: `{"game_id": <id>}` — integer, not string
- `status`: `'pending'`
- `result`: `'{}'`
- `igdb_candidates`: `'[]'`
- `created_at`: `now()`

Insert `pending_tasks`:
- `id`: new UUID
- `task_type`: `'metadata_refresh_item'`
- `payload`: `{"job_item_id": "<itemID>"}`
- `priority`: `1`
- `status`: `'pending'`
- `attempts`: `0`
- `created_at`: `now()`

**Step 7 — Update job to processing.**
```sql
UPDATE jobs SET status = 'processing', started_at = now() WHERE id = ?
```

**Step 8 — Return nil.**

The dispatch handler does not call `pool.Submit` for the item tasks — it inserts `pending_tasks` rows directly (same pattern as `DispatchSyncTask`). The worker pool picks them up from the DB.

> **Crash safety note:** creating the job as `'pending'` before inserting items means a server crash between steps 5 and 7 leaves the job in `'pending'` rather than `'processing'`. The duplicate-run guard (step 3) still blocks future refreshes until the stuck job is cleaned up. A stale-job cleanup scheduler job (see Phase 5) handles this by failing orphaned `pending`/`processing` metadata refresh jobs that have exceeded a timeout threshold.

---

## Worker Task: `metadata_refresh_item`

**File:** `internal/worker/tasks/metadata_refresh.go`

### Constructor

```go
func NewMetadataRefreshItemHandler(
    db          *bun.DB,
    igdbClient  *igdbsvc.Client,
    storagePath string,
) func(ctx context.Context, task *models.PendingTask) error
```

### Payload shape

```go
type metadataRefreshItemPayload struct {
    JobItemID string `json:"job_item_id"`
}
```

`source_metadata` on the `job_items` row carries:
```go
type metadataRefreshSourceMeta struct {
    GameID int32 `json:"game_id"`
}
```

### Algorithm

**Step 1 — Parse payload.**
Unmarshal `task.Payload` into `metadataRefreshItemPayload`. On error: log and return nil (unrecoverable).

**Step 2 — Load job_item.**
`SELECT * FROM job_items WHERE id = ?`. On error: log and return nil.

**Step 3 — Parse source_metadata.**
Unmarshal `item.SourceMetadata` into `metadataRefreshSourceMeta`. On error: call `metaRefreshMarkItemFailed`, call `metaRefreshCheckJobCompletion`, return nil.

**Step 4 — Load game.**
`SELECT id, title FROM games WHERE id = ?`. On error (including not found): mark item failed, check completion, return nil.

**Step 5 — IGDB guard.**
If `!igdbClient.Configured()`: mark item failed with message `"igdb_not_configured"`, check completion, return nil. (Should not happen in practice since the dispatch task guards, but defensively handle it.)

**Step 6 — Fetch metadata.**
Call `igdbClient.FetchFullMetadata(ctx, int(game.ID))`. On error: mark item failed with `err.Error()`, check completion, return nil.

**Step 7 — Update games row.**
Update all metadata columns in a single `UPDATE games SET ... WHERE id = ?`:

| Column | Source |
|---|---|
| `title` | `md.Title` |
| `description` | `md.Description` |
| `genre` | `md.Genre` |
| `developer` | `md.Developer` |
| `publisher` | `md.Publisher` |
| `release_date` | parse `md.ReleaseDate` (`"YYYY-MM-DD"` → `time.Time`); nil if absent or unparseable |
| `rating_average` | `md.RatingAverage` |
| `rating_count` | `md.RatingCount` |
| `howlongtobeat_main` | `md.HowlongtobeatMain` |
| `howlongtobeat_extra` | `md.HowlongtobeatExtra` |
| `howlongtobeat_completionist` | `md.HowlongtobeatCompletionist` |
| `igdb_slug` | `md.IgdbSlug` if non-empty, else `NULL` |
| `igdb_platform_ids` | `json.Marshal(md.PlatformIDs)` as string; `nil` if slice is empty. **JSON array format** — e.g. `[6,48]`. This matches the DB schema (`-- JSON array as text`). |
| `igdb_platform_names` | `json.Marshal(md.PlatformNames)` as string; `nil` if slice is empty. **JSON array format** — e.g. `["PC","PlayStation 5"]`. |
| `game_modes` | `md.GameModes` |
| `themes` | `md.Themes` |
| `player_perspectives` | `md.PlayerPerspectives` |
| `last_updated` | `now()` |

On DB error: mark item failed with `fmt.Sprintf("update games: %v", err)`, check completion, return nil.

**Step 8 — Cover art (non-fatal).**
If `md.CoverImageID == ""` (IGDB returned no cover), skip this step entirely — the existing `cover_art_url` in the database is preserved unchanged.

If `md.CoverImageID != ""`:
```go
localPath, err := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
if err != nil {
    slog.Warn("metadata_refresh_item: cover art download failed",
        "game_id", game.ID, "image_id", md.CoverImageID, "err", err)
    // non-fatal — continue to mark item completed
} else if localPath != "" {
    _, _ = db.NewRaw(
        `UPDATE games SET cover_art_url = ? WHERE id = ?`, localPath, game.ID,
    ).Exec(ctx)
}
```

**Step 9 — Mark item completed.**

```go
func metaRefreshMarkItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem)
```

Sets `status='completed'`, `processed_at=now()`.

**Step 10 — Check job completion.**

```go
func metaRefreshCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string)
```

```sql
SELECT COUNT(*) FROM job_items
WHERE job_id = ?
  AND status NOT IN ('completed', 'failed', 'skipped')
```
If count is zero: `UPDATE jobs SET status='completed', completed_at=now() WHERE id = ?`.

There is no `pending_review` state for metadata refresh items — nothing requires manual user resolution.

### Helper functions (file-private)

```go
func metaRefreshMarkItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string)
func metaRefreshMarkItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem)
func metaRefreshCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string)
```

These parallel the `syncMark*` helpers in `sync.go` but are separate functions. Do not reuse the sync helpers — they are unexported to their file.

---

## main.go Changes

**Worker handler registration** (both the initial block and inside `RebuildServices`):

```go
pool.Register("metadata_refresh_dispatch",
    tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient))
pool.Register("metadata_refresh_item",
    tasks.NewMetadataRefreshItemHandler(db, igdbClient, cfg.StoragePath))
```

Remove the existing commented-out line:
```go
// pool.Register("metadata_refresh_process", metadataHandler)
```

**Scheduler constructor** (both call sites):
```go
sched = scheduler.NewScheduler(db, pool, backupSvc, cfg)
```

---

## Slumber Collection

Metadata refresh has no HTTP trigger endpoint (it is scheduler-only). No slumber requests are needed. Add a comment alongside the `jobs/` folder in `slumber.yaml` noting this.

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| IGDB not configured | Dispatch: skip, no job. Item: mark failed (defensive). |
| No admin user | Dispatch: skip, no job, log warning. |
| Active job already running | Dispatch: skip silently. |
| No games in DB | Dispatch: skip, no job. |
| `FetchFullMetadata` error | Item: mark failed, check completion. |
| DB update error (games row) | Mark item failed; check job completion. |
| `DownloadCoverArt` error | Log warning; item still completes (cover art is non-critical). |
| IGDB game not found (returns empty) | `FetchFullMetadata` returns `ErrGameNotFound`; item marked failed. |

---

## Testing

**File:** `internal/worker/tasks/metadata_refresh_test.go`

Uses `testcontainers-go` for a real PostgreSQL container (same pattern as `sync_test.go`). IGDB calls are stubbed via a local `igdb.Client` constructed with `igdb.NewClientWithTokenURL` pointing at an `httptest.Server`.

| Test | Assertion |
|---|---|
| `TestMetadataRefreshDispatch_IGDBNotConfigured` | No `jobs` row created; handler returns nil. |
| `TestMetadataRefreshDispatch_NoAdminUser` | No `jobs` row created; handler returns nil. |
| `TestMetadataRefreshDispatch_AlreadyRunning` | Pre-existing `processing` job → no duplicate; returns nil. |
| `TestMetadataRefreshDispatch_NoGames` | No `jobs` row created; returns nil. |
| `TestMetadataRefreshDispatch_CreatesJobAndItems` | 3 games → 1 `jobs` row (`status='processing'`), 3 `job_items`, 3 `pending_tasks` with `task_type='metadata_refresh_item'`. |
| `TestMetadataRefreshItem_Success` | Game fields updated; `cover_art_url` set to local path; item `completed`; job `completed`. |
| `TestMetadataRefreshItem_IGDBError` | Item `failed`; job `completed` once all items terminal. |
| `TestMetadataRefreshItem_CoverArtFailureNonFatal` | DownloadCoverArt fails; item still `completed`. |
| `TestMetadataRefreshItem_JobCompletionPartial` | Two items: first completes, job still `processing`; second completes, job `completed`. |
