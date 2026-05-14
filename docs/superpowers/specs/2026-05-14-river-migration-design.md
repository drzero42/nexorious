# River Migration — Design Spec

## Scope

Replace the custom `internal/worker/pool.go` (PostgreSQL SKIP LOCKED job queue) and the `gocron`-based scheduler with [`riverqueue/river`](https://github.com/riverqueue/river). This is a big-bang migration — no incremental transition period. The existing migration SQL is updated in place (no new migration file, since no production database exists yet).

**Out of scope:** The `jobs` and `job_items` tables and all business logic that manipulates them. They are unchanged.

---

## Dependencies

**Added:**
- `riverqueue/river` — queue engine, periodic job scheduler, leader election
- `riverqueue/riverdriver-go-pgxv5` — pgx/v5 adapter for River
- `jackc/pgx/v5/pgxpool` — already a transitive dependency; now used directly

**Removed:**
- `go-co-op/gocron/v2`

---

## Database Changes

Updated in the existing migration SQL (`internal/db/migrations/`). No new migration file.

**Removed:**
```sql
-- pending_tasks table and its index are dropped entirely
DROP TABLE IF EXISTS pending_tasks;
```

**Added (River's own schema):**
```sql
CREATE TABLE river_queue (
    name        TEXT        NOT NULL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata    JSONB       NOT NULL DEFAULT '{}',
    paused_at   TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE river_job (
    id           BIGSERIAL   PRIMARY KEY,
    args         JSONB       NOT NULL DEFAULT '{}',
    attempt      SMALLINT    NOT NULL DEFAULT 0,
    attempted_at TIMESTAMPTZ,
    attempted_by TEXT[],
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    errors       JSONB[],
    finalized_at TIMESTAMPTZ,
    kind         TEXT        NOT NULL,
    max_attempts SMALLINT    NOT NULL,
    metadata     JSONB       NOT NULL DEFAULT '{}',
    priority     SMALLINT    NOT NULL DEFAULT 1,
    queue        TEXT        NOT NULL DEFAULT 'default',
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    state        TEXT        NOT NULL DEFAULT 'available',
    tags         TEXT[]      NOT NULL DEFAULT '{}'
);
CREATE INDEX river_job_kind        ON river_job (kind);
CREATE INDEX river_job_state_queue ON river_job (state, queue, priority, scheduled_at, id)
    WHERE state IN ('available', 'retryable', 'scheduled');

CREATE TABLE river_leader (
    elected_at  TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    leader_id   TEXT        NOT NULL,
    name        TEXT        NOT NULL PRIMARY KEY,
    updated_at  TIMESTAMPTZ NOT NULL
);
```

**`models.PendingTask` struct** in `internal/db/models/` is deleted. River owns job persistence.

---

## Priority Mapping

River uses 1–4 (1 = highest, 4 = lowest).

| Task type | Priority | Rationale |
|---|---|---|
| `process_sync_item` | 1 | User-triggered sync, latency-sensitive |
| `dispatch_sync` | 1 | Kicks off user-visible sync job |
| `import_item` | 3 | Batch background work |
| `export_json` / `export_csv` | 3 | Background export |
| `metadata_refresh_dispatch` | 3 | Background maintenance |
| `metadata_refresh_item` | 3 | Background maintenance |
| Scheduler-submitted jobs | 3 | Periodic maintenance |

---

## Worker Client

River requires a `*pgxpool.Pool` directly (not `*bun.DB`). A second connection is created at startup alongside the existing Bun connection, sharing the same `DATABASE_URL`:

```go
// cmd/nexorious/serve.go
pgxPool, err := pgxpool.New(ctx, resolvedDatabaseURL)
// ... error handling

workers := river.NewWorkers()
river.AddWorker(workers, &tasks.ImportItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &tasks.ExportJSONWorker{DB: db, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &tasks.ExportCSVWorker{DB: db, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &tasks.DispatchSyncWorker{DB: db, Steam: steamsvc.NewClient(), PSN: psnsvc.NewClient()})
river.AddWorker(workers, &tasks.ProcessSyncItemWorker{DB: db, IGDBClient: igdbClient})
river.AddWorker(workers, &tasks.MetadataRefreshDispatchWorker{DB: db})
river.AddWorker(workers, &tasks.MetadataRefreshItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})

// Scheduler workers — see Scheduler section
river.AddWorker(workers, &scheduler.CleanupOldJobsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupExportsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupUnreferencedGamesWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupExpiredSessionsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupStaleJobsWorker{DB: db, Threshold: staleThreshold})
river.AddWorker(workers, &scheduler.MetadataRefreshDispatchTriggerWorker{RiverClient: riverClient})
river.AddWorker(workers, &scheduler.CheckPendingSyncsWorker{DB: db, RiverClient: riverClient})
river.AddWorker(workers, &scheduler.CheckScheduledBackupWorker{DB: db, BackupSvc: backupSvc})

riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{
    Workers: workers,
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
    PeriodicJobs: buildPeriodicJobs(cfg, staleThreshold),
})
```

`internal/worker/pool.go` and `internal/worker/pool_test.go` are deleted.

---

## Task Handler Pattern

Each task type becomes a typed River worker. The handler body is unchanged; only the outer signature changes.

**Args type:**
```go
// internal/worker/tasks/import_item.go
type ImportItemArgs struct {
    JobItemID string `json:"job_item_id"`
}
func (ImportItemArgs) Kind() string { return "import_item" }
```

**Worker type:**
```go
type ImportItemWorker struct {
    DB          *bun.DB
    IGDBClient  *igdb.Client
    StoragePath string
}

func (w *ImportItemWorker) Work(ctx context.Context, job *river.Job[ImportItemArgs]) error {
    // existing handler body — access payload via job.Args.JobItemID
}
```

**Submitting jobs** — all `pool.Submit(...)` calls and the two raw SQL `INSERT INTO pending_tasks` in `sync.go` become:
```go
_, err = riverClient.Insert(ctx, ImportItemArgs{JobItemID: itemID}, &river.InsertOpts{
    Priority: 3,
})
```

---

## Retry Policy

River's default is 25 attempts. We override per worker.

**`MaxAttempts: 1` (no retry) — 5 task types:**

| Task type | Reason |
|---|---|
| `import_item` | Partial DB writes; blind retry risks duplicate data |
| `export_json` / `export_csv` | Creates files on disk; retry produces duplicate exports |
| `dispatch_sync` | Fans out to many sub-tasks; retry after partial fan-out double-dispatches |
| `metadata_refresh_dispatch` | Same fan-out concern |

**`MaxAttempts: 5` with exponential backoff — 2 task types:**

| Task type | Reason |
|---|---|
| `process_sync_item` | Network-bound IGDB calls; all DB writes use `ON CONFLICT DO NOTHING` |
| `metadata_refresh_item` | Same: network-bound, all writes are upserts |

Retry policy is co-located with the job definition via `river.JobArgsWithInsertOpts`:
```go
func (ProcessSyncItemArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}
```

---

## Scheduler Migration

`go-co-op/gocron/v2` is removed entirely. The `internal/scheduler/` package stays but the `Scheduler` struct drops its `gocron.Scheduler` and `*worker.Pool` fields. Periodic jobs are defined as `river.PeriodicJob` entries in the River client config.

### Fixed-schedule periodic jobs

| Job | Schedule | Implementation |
|---|---|---|
| `CleanupOldJobs` | `0 3 * * *` | `river.NewCronSchedule("0 3 * * *")` |
| `CleanupExports` | `0 4 * * *` | `river.NewCronSchedule("0 4 * * *")` |
| `CleanupUnreferencedGames` | `0 5 * * *` | `river.NewCronSchedule("0 5 * * *")` |
| `CleanupExpiredSessions` | `*/30 * * * *` | `river.NewCronSchedule("*/30 * * * *")` |
| `CleanupStaleJobs` | `0 * * * *` | `river.NewCronSchedule("0 * * * *")` |
| `metadata_refresh_dispatch` | configurable duration | `river.PeriodicInterval(metadataRefreshInterval)` |
| `CheckPendingSyncs` | `*/15 * * * *` | `river.NewCronSchedule("*/15 * * * *")` |

Each becomes a thin worker whose `Work` method calls the existing function (e.g. `CleanupOldJobs(ctx, db)`). The cleanup function bodies are unchanged.

River elects a leader across instances — periodic jobs fire **exactly once** cluster-wide, unlike gocron which fires on every instance.

### Dynamic backup job — polling pattern

`RebuildBackupJob` is **removed** from the `Scheduler` struct.

A `CheckScheduledBackup` periodic job fires every minute:

```go
// internal/scheduler/backup_poll.go
type CheckScheduledBackupArgs struct{}
func (CheckScheduledBackupArgs) Kind() string { return "check_scheduled_backup" }

type CheckScheduledBackupWorker struct {
    DB        *bun.DB
    BackupSvc *backup.Service
}

func (w *CheckScheduledBackupWorker) Work(ctx context.Context, job *river.Job[CheckScheduledBackupArgs]) error {
    if !backup.PgDumpAvailable() {
        return nil
    }
    var cfg models.BackupConfig
    if err := w.DB.NewSelect().Model(&cfg).Where("id = 1").Scan(ctx); err != nil || cfg.ScheduleCron == "" {
        return nil
    }
    sched, err := cron.ParseStandard(cfg.ScheduleCron)
    if err != nil {
        return nil
    }
    // Determine last expected fire time and compare against last_backup_at
    now := time.Now().UTC()
    prev := sched.Prev(now)
    if cfg.LastBackupAt != nil && !cfg.LastBackupAt.Before(prev) {
        return nil // already ran this window
    }
    id, err := w.BackupSvc.CreateBackup("scheduled")
    if err != nil {
        slog.Error("scheduled backup failed", "err", err)
        return err
    }
    slog.Info("scheduled backup created", "id", id)
    _ = w.BackupSvc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue)
    return nil
}
```

`BackupConfig` gains a `last_backup_at TIMESTAMPTZ` column (updated by `CreateBackup`) so the poller can detect whether the current cron window has already been covered.

The cron expression is parsed using `robfig/cron/v3` (already a transitive dependency via gocron; stays as a direct dependency after gocron is removed).

---

## Restore-After-Backup Rebuild

`RebuildServices` in `serve.go` tears down and rebuilds the River client after a DB restore. Pattern mirrors the current pool rebuild:

```go
// Stop existing client and close pgxpool
riverClient.Stop(ctx)
pgxPool.Close()

// Re-create both with the same connection string
pgxPool, _ = pgxpool.New(ctx, resolvedDatabaseURL)
riverClient, _ = river.NewClient(riverpgxv5.New(pgxPool), buildRiverConfig(workers, cfg))
riverClient.Start(ctx)
```

The `Scheduler` struct is removed entirely — periodic jobs live in the River client config, so there is nothing separate to rebuild.

---

## Testing

**Task handler tests** — instantiate the worker struct directly and call `Work`:
```go
w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}})
```

River provides `rivertest` helpers for test client setup; not required for unit-testing individual workers.

**Scheduler tests** — the `CleanupOldJobs(ctx, db)` etc. functions are called directly in tests. No change required to scheduler test files.

**Pool tests** — `internal/worker/pool_test.go` is deleted. River's own test suite covers claim loop and dispatch behaviour.

**Integration** — River migrations run as part of the updated migration SQL, picked up by the shared `TestMain` PostgreSQL container already used across packages.
