# Worker Pool & Job Tracking — Design Spec

## Scope

Infrastructure layer for Phase 3: the database-backed task queue, worker pool, job/job-item tracking, and scheduler skeleton with cleanup jobs. Task consumers (import, export, backup, sync handlers) are out of scope — they will be covered in follow-up specs that build on this infrastructure.

## Schema

All three tables are added to the existing `20260503000001_initial.up.sql` migration (no production database exists yet).

### `pending_tasks`

Database-backed task queue. Workers claim rows using `SELECT ... FOR UPDATE SKIP LOCKED`.

```sql
CREATE TABLE pending_tasks (
    id          TEXT PRIMARY KEY,
    task_type   TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    priority    INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending',
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at  TIMESTAMPTZ,
    done_at     TIMESTAMPTZ
);
CREATE INDEX pending_tasks_claim_idx ON pending_tasks (status, priority DESC, created_at)
    WHERE status = 'pending';
```

- `id`: UUID v4 as text.
- `task_type`: string key dispatched to a registered handler (e.g. `"sync"`, `"import_item"`, `"export"`).
- `payload`: arbitrary JSONB; contains `job_id` when the task is associated with a user-visible job.
- `priority`: higher values are claimed first. Default 0.
- `status`: `pending` → `running` → `done` | `failed`.
- `attempts`: incremented each time a worker claims the task.
- `claimed_at`: set when a worker picks it up.
- `done_at`: set on completion (success or failure).

The partial index covers only `pending` rows — the hot path for worker claims. Queries against `running`/`done`/`failed` rows do a sequential scan, which is acceptable for operational/monitoring use.

### `jobs`

User-visible job tracking. Every long-running operation (sync, import, export, metadata refresh) creates a `jobs` row before submitting tasks to the worker pool.

```sql
CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type        TEXT NOT NULL,
    source          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    priority        TEXT NOT NULL DEFAULT 'high',
    file_path       TEXT,
    total_items     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    auto_retry_done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);
CREATE INDEX jobs_user_id_idx ON jobs (user_id);
CREATE INDEX jobs_job_type_idx ON jobs (job_type);
CREATE INDEX jobs_source_idx ON jobs (source);
CREATE INDEX jobs_status_idx ON jobs (status);
```

- `job_type`: `sync`, `import`, `export`, `metadata_refresh`.
- `source`: `steam`, `epic`, `psn`, `gog`, `manual`, `nexorious`, `csv`, `system`.
- `status`: `pending`, `processing`, `completed`, `failed`, `cancelled`.
- `priority`: `low`, `normal`, `high`.
- `file_path`: set by export jobs to reference the output file on disk.
- `auto_retry_done`: whether the system has automatically retried failed items (prevents infinite retry loops).

### `job_items`

Per-item tracking within a job. Each item represents one game being synced, imported, or processed.

```sql
CREATE TABLE job_items (
    id                  TEXT PRIMARY KEY,
    job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_key            TEXT NOT NULL,
    source_title        TEXT NOT NULL,
    source_metadata     JSONB NOT NULL DEFAULT '{}',
    status              TEXT NOT NULL DEFAULT 'pending',
    result              JSONB NOT NULL DEFAULT '{}',
    error_message       TEXT,
    igdb_candidates     JSONB NOT NULL DEFAULT '[]',
    resolved_igdb_id    INTEGER,
    match_confidence    NUMERIC(5,4),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at        TIMESTAMPTZ,
    resolved_at         TIMESTAMPTZ,
    UNIQUE(job_id, item_key)
);
CREATE INDEX job_items_job_id_idx ON job_items (job_id);
CREATE INDEX job_items_user_id_idx ON job_items (user_id);
CREATE INDEX job_items_status_idx ON job_items (status);
```

- `item_key`: unique identifier within a job (e.g. Steam app ID, external game title).
- `source_title`: the game title as reported by the source platform.
- `source_metadata`: arbitrary JSON from the source (platform-specific fields).
- `status`: `pending`, `processing`, `completed`, `pending_review`, `skipped`, `failed`.
- `result`: outcome JSON (e.g. `{ "game_title": "...", "user_game_id": "...", "is_new_addition": true }`).
- `igdb_candidates`: array of IGDB search results when the item needs manual review.
- `resolved_igdb_id`: set when the user resolves a `pending_review` item to a specific IGDB game.
- `match_confidence`: 0.0–1.0 confidence score from automatic IGDB matching.

Note: JSONB columns (`source_metadata`, `result`, `igdb_candidates`) are native JSONB rather than the Python codebase's JSON-as-text-string pattern. Bun handles JSONB natively.

## Bun Model Structs

New file `internal/db/models/jobs.go` with three structs following existing patterns in `models.go`.

### `PendingTask`

Maps to `pending_tasks`. `Payload` is `json.RawMessage` — the pool passes the raw JSON to the handler, which deserializes it into whatever struct it needs.

### `Job`

Maps to `jobs`. Enum fields (`job_type`, `source`, `status`, `priority`) are plain `string` with Go constants:

```go
const (
    JobTypSync            = "sync"
    JobTypeImport         = "import"
    JobTypeExport         = "export"
    JobTypeMetadataRefresh = "metadata_refresh"

    JobSourceSteam     = "steam"
    JobSourceEpic      = "epic"
    JobSourcePSN       = "psn"
    JobSourceGOG       = "gog"
    JobSourceManual    = "manual"
    JobSourceNexorious = "nexorious"
    JobSourceCSV       = "csv"
    JobSourceSystem    = "system"

    JobStatusPending    = "pending"
    JobStatusProcessing = "processing"
    JobStatusCompleted  = "completed"
    JobStatusFailed     = "failed"
    JobStatusCancelled  = "cancelled"

    JobPriorityLow    = "low"
    JobPriorityNormal = "normal"
    JobPriorityHigh   = "high"
)
```

Methods ported from the Python model:
- `IsActive() bool` — returns true if status is `pending` or `processing`.
- `IsTerminal() bool` — returns true if status is `completed`, `failed`, or `cancelled`.
- `DurationSeconds() *float64` — returns elapsed seconds from `started_at` to `completed_at` (or now if still running). Returns nil if `started_at` is not set.

### `JobItem`

Maps to `job_items`. Same patterns. JSONB fields are `json.RawMessage`. Constants for status values:

```go
const (
    JobItemStatusPending       = "pending"
    JobItemStatusProcessing    = "processing"
    JobItemStatusCompleted     = "completed"
    JobItemStatusPendingReview = "pending_review"
    JobItemStatusSkipped       = "skipped"
    JobItemStatusFailed        = "failed"
)
```

## Worker Pool (`internal/worker/`)

### Files

- `internal/worker/pool.go` — Pool type and worker loop.
- `internal/worker/pool_test.go` — tests using testcontainers-go.

### Types

```go
type TaskHandler func(ctx context.Context, task *models.PendingTask) error

type Pool struct {
    db       *bun.DB
    handlers map[string]TaskHandler
    notify   chan struct{}   // capacity 1; non-blocking send
    wg       sync.WaitGroup
    cancel   context.CancelFunc
}
```

### Public API

- `NewPool(db *bun.DB) *Pool` — creates pool with empty handler registry and a `notify` channel of capacity 1.
- `Register(taskType string, handler TaskHandler)` — adds a handler to the registry. Called at startup before `Start`. Panics if called after `Start` (programming error, not a runtime condition).
- `Submit(ctx context.Context, taskType string, payload any, priority int) error` — marshals `payload` to JSON, inserts a `pending_tasks` row, sends on `notify` (non-blocking; drops if already pending). Returns DB error only. Never blocks.
- `Start(ctx context.Context, workers int)` — spawns `workers` goroutines. The number of workers is controlled by the `WORKER_COUNT` env var (default 4), passed in from `cfg.WorkerCount`.
- `Shutdown()` — cancels the context, waits for all in-flight tasks to complete via `wg.Wait()`.

### Worker Loop

Each goroutine runs the following loop until the context is cancelled:

1. **Wait for signal:** `select` on the `notify` channel or a 1-second ticker, whichever fires first.
2. **Claim a task:** execute the atomic claim query:
   ```sql
   UPDATE pending_tasks
   SET status = 'running', claimed_at = now(), attempts = attempts + 1
   WHERE id = (
       SELECT id FROM pending_tasks
       WHERE status = 'pending'
       ORDER BY priority DESC, created_at
       LIMIT 1
       FOR UPDATE SKIP LOCKED
   )
   RETURNING *;
   ```
3. **No row claimed:** back to step 1.
4. **Look up handler** by `task.TaskType` in the registry map.
5. **No handler found:** mark task `failed` with `last_error = "unknown task type: <type>"`, back to step 1.
6. **Call handler:** `handler(ctx, &task)`.
7. **Handler returns nil:** mark task `status = 'done'`, set `done_at = now()`.
8. **Handler returns error:** mark task `status = 'failed'`, set `last_error = err.Error()`, `done_at = now()`.
9. Back to step 1.

### Error Handling

- The worker loop never crashes. Panics inside handlers are recovered with `defer/recover` and treated as a failed task with `last_error = "panic: <message>"`.
- If the claim query itself fails (DB connection error), the worker logs the error and retries after the next tick. It does not exit.
- `Submit` never blocks and never drops tasks. The only failure mode is a DB error on the INSERT, which is returned to the caller (HTTP handler returns 503; scheduler logs and retries at next tick).

### Graceful Shutdown

When the parent context is cancelled (SIGTERM/SIGINT), workers finish their current in-flight task and exit. `Shutdown()` blocks until all goroutines have drained.

## Job & Job-Item API Endpoints

### Jobs Handler (`internal/api/jobs.go`)

Constructor: `NewJobsHandler(db *bun.DB, pool *worker.Pool)`. Takes the pool reference for retry operations. All endpoints are JWT-protected and scoped to the current user's jobs.

| Endpoint | Method | Description |
|---|---|---|
| `/api/jobs` | GET | List jobs. Query params: `page`, `per_page`, `job_type`, `source`, `status`, `sort_by`, `sort_order`. Returns paginated response. |
| `/api/jobs/summary` | GET | Counts of running and failed jobs (frontend navbar badge). |
| `/api/jobs/pending-review-count` | GET | Total count of `job_items` with `status = 'pending_review'` across all user's jobs. |
| `/api/jobs/active/:job_type` | GET | Active (pending/processing) job for a type, or most recent completed if none active. |
| `/api/jobs/recent/:source` | GET | Recent completed jobs for a source with inline item summaries. Query param: `limit` (default 5, max 20). Item summaries include `game_title`, `is_new_addition`, `user_game_id` extracted from the item's `result` JSONB. |
| `/api/jobs/:id` | GET | Single job detail with computed `duration_seconds` and item count breakdowns by status. |
| `/api/jobs/:id/items` | GET | Paginated job items for a job. Optional `status` query param filter. |
| `/api/jobs/:id/cancel` | POST | Cancel an in-progress job: sets status → `cancelled`, deletes associated `pending_tasks` rows that are still `pending`. Returns 409 if job is already terminal. |
| `/api/jobs/:id` | DELETE | Delete a terminal job and its items (CASCADE). Returns 409 if job is still active. |
| `/api/jobs/:id/retry-failed` | POST | Re-queue all failed items: reset their status → `pending`, create new `pending_tasks` rows via `pool.Submit()`, reset job status → `processing`. |

### Job Items Handler (`internal/api/job_items.go`)

Constructor: `NewJobItemsHandler(db *bun.DB, pool *worker.Pool)`. Takes the pool reference for retry operations.

| Endpoint | Method | Description |
|---|---|---|
| `/api/job-items/:id` | GET | Single job item detail including `igdb_candidates` for the review UI. |
| `/api/job-items/:id/resolve` | POST | Resolve a `pending_review` item. Request: `{ "igdb_id": 12345 }`. Sets `resolved_igdb_id`, status → `completed`, `resolved_at = now()`. The actual game creation and user-game linking logic will be implemented by the consumer spec that handles sync/import — this endpoint is the hook point for it. Returns 409 if item is not `pending_review`. |
| `/api/job-items/:id/skip` | POST | Skip a review item. Request: `{ "reason": "optional" }`. Sets status → `skipped`. Returns 409 if item is not `pending_review`. |
| `/api/job-items/:id/retry` | POST | Retry a single failed item: reset status → `pending`, create a new `pending_tasks` row via `pool.Submit()`. Returns 409 if item is not `failed`. |

### Ownership Enforcement

All endpoints verify that the job/item belongs to the authenticated user via `WHERE user_id = ?` in the query. A job/item belonging to another user returns 404 (not 403) to avoid leaking existence.

## Scheduler (`internal/scheduler/`)

### Files

- `internal/scheduler/scheduler.go` — gocron setup and cleanup job functions.
- `internal/scheduler/scheduler_test.go`

### Type

```go
type Scheduler struct {
    db        *bun.DB
    pool      *worker.Pool
    scheduler gocron.Scheduler
}
```

- `NewScheduler(db *bun.DB, pool *worker.Pool) *Scheduler`
- `Start(ctx context.Context) error` — registers all jobs and starts gocron.
- `Stop()` — shuts down gocron.

### Registered Jobs

All four cleanup jobs run inline in the gocron goroutine — they are fast single-query DB operations with no external I/O and do not go through the task queue.

| Job | Schedule | Description |
|---|---|---|
| Cleanup job results | Daily 3:00 AM UTC | Delete terminal jobs older than 30 days and their items (CASCADE). |
| Cleanup exports | Daily 4:00 AM UTC | Delete expired export files from disk + their job rows. |
| Cleanup unreferenced games | Daily 5:00 AM UTC | Anti-join query to find games with no `user_games` rows; delete rows + cover art files. |
| Cleanup sessions | Every 30 minutes | Delete expired `user_sessions` rows. |

Jobs that submit to the pool (scheduled backup, metadata refresh dispatch, check pending syncs) are **not registered in this spec** — they will be added when their respective consumer specs are implemented. The scheduler skeleton accepts the pool reference and is ready for them.

### Startup Constraint

The scheduler starts only after the migrator transitions to `Ready`, matching the worker pool. Both are started in `main.go` after migration completes.

## Startup & Shutdown Wiring

### Startup Sequence (after migrator reaches `Ready`)

1. `pool := worker.NewPool(db)`
2. Register handlers (none initially — consumers add theirs in follow-up work)
3. `pool.Start(ctx, cfg.WorkerCount)`
4. `sched := scheduler.NewScheduler(db, pool)`
5. `sched.Start(ctx)`
6. Register job/job-item API routes in `registerRoutes` (pool passed for job-items handler)

### Shutdown Sequence (SIGTERM/SIGINT)

1. Scheduler stops (no new cron-triggered tasks)
2. Echo server shuts down (no new HTTP requests)
3. Pool shuts down (drains in-flight tasks)
4. DB connection closes

### Router Changes

Two new route groups in `registerRoutes`:

```go
jh := NewJobsHandler(db, pool)
jobsGroup := e.Group("/api/jobs", auth.JWTMiddleware(cfg.SecretKey, db))
// ... register all job routes

jih := NewJobItemsHandler(db, pool)
jobItemsGroup := e.Group("/api/job-items", auth.JWTMiddleware(cfg.SecretKey, db))
// ... register all job-item routes
```

The pool is passed to `registerRoutes` so the job-items handler can submit retry tasks.

## Testing

Tests use testcontainers-go with real PostgreSQL, matching the existing test patterns.

### Worker Pool Tests

- Submit a task, start pool with 1 worker, assert task reaches `done` status.
- Submit a task with no registered handler, assert `failed` with appropriate `last_error`.
- Submit a task whose handler returns an error, assert `failed` with error message.
- Concurrent claim test: submit 1 task, start 2 workers, assert exactly 1 execution.
- Priority ordering: submit low and high priority tasks, assert high priority claimed first.
- Graceful shutdown: submit a slow task, call `Shutdown()`, assert it completes before returning.

### Job/Job-Item Endpoint Tests

- CRUD lifecycle: create a job (directly via DB insert in test), list, get, delete.
- Cancel: create an active job with pending tasks, cancel, assert job cancelled and pending_tasks deleted.
- Retry-failed: create a job with failed items, retry, assert items reset and new pending_tasks created.
- Resolve/skip/retry item endpoints.
- Ownership: assert 404 when accessing another user's job/item.
- Status guards: assert 409 when cancelling a terminal job, deleting an active job, resolving a non-review item.

### Scheduler Tests

- Assert cleanup queries delete the right rows (expired jobs, sessions, unreferenced games).
- Assert cleanup doesn't delete non-expired/active rows.

## Checklist

- [ ] Add `pending_tasks`, `jobs`, `job_items` tables to `20260503000001_initial.up.sql`
- [ ] Add corresponding DROP statements to `20260503000001_initial.down.sql`
- [ ] Create `internal/db/models/jobs.go` with Bun model structs and constants
- [ ] Create `internal/worker/pool.go` with Pool type, registry, worker loop
- [ ] Create `internal/worker/pool_test.go`
- [ ] Create `internal/api/jobs.go` with all job endpoints
- [ ] Create `internal/api/jobs_test.go`
- [ ] Create `internal/api/job_items.go` with all job-item endpoints
- [ ] Create `internal/api/job_items_test.go`
- [ ] Create `internal/scheduler/scheduler.go` with cleanup jobs
- [ ] Create `internal/scheduler/scheduler_test.go`
- [ ] Update `internal/api/router.go` to register job and job-item routes
- [ ] Update `cmd/nexorious/main.go` for pool/scheduler startup and shutdown wiring
- [ ] Add slumber requests for all new endpoints
