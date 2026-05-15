# River Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the custom `internal/worker/pool.go` and `gocron` scheduler with `riverqueue/river`, deleting ~350 lines of custom queue infrastructure while gaining retry, leader-election, and typed job args.

**Architecture:** Big-bang migration — `pending_tasks` table replaced by River's schema in the existing migration SQL, all 7 task handlers converted to typed River workers, all 7 scheduled jobs converted to River periodic jobs, and the `gocron` dependency removed entirely. The `jobs`/`job_items` tables and their business logic are untouched.

**Tech Stack:** `riverqueue/river`, `riverqueue/riverdriver-go-pgxv5`, `robfig/cron/v3` (kept direct after gocron removed), `jackc/pgx/v5/pgxpool` (new direct dep), `uptrace/bun` (unchanged).

---

## File Map

**Deleted:**
- `internal/worker/pool.go`
- `internal/worker/pool_test.go`
- `internal/scheduler/lifecycle_test.go` (tests gocron Start/Stop — no longer applicable)

**Modified:**
- `internal/db/migrations/20260503000001_initial.up.sql` — remove `pending_tasks`, add River tables, add `last_backup_at` to `backup_config`
- `internal/db/migrations/20260503000001_initial.down.sql` — reverse
- `internal/db/models/jobs.go` — remove `PendingTask` struct
- `internal/db/models/backup_config.go` — add `LastBackupAt *time.Time`
- `internal/worker/tasks/export.go` — `ExportJSONWorker`, `ExportCSVWorker`
- `internal/worker/tasks/export_test.go` — update to River worker pattern
- `internal/worker/tasks/import_item.go` — `ImportItemWorker`
- `internal/worker/tasks/import_item_test.go` — update to River worker pattern
- `internal/worker/tasks/sync.go` — `DispatchSyncWorker`, `ProcessSyncItemWorker`
- `internal/worker/tasks/sync_test.go` — update to River worker pattern
- `internal/worker/tasks/metadata_refresh.go` — `MetadataRefreshDispatchWorker`, `MetadataRefreshItemWorker`
- `internal/worker/tasks/metadata_refresh_test.go` — update to River worker pattern
- `internal/worker/tasks/testmain_test.go` — expose `testConnStr`, remove `makePendingTask`, update `truncateAllTables`
- `internal/scheduler/scheduler.go` — River periodic workers, remove gocron + `*worker.Pool`, remove `RebuildBackupJob`
- `internal/scheduler/stale_jobs.go` — no change
- `internal/api/router.go` — `pool *worker.Pool` → `riverClient *river.Client[pgx.Tx]`
- `internal/api/import.go` — `h.pool` → `h.riverClient`
- `internal/api/job_items.go` — `h.pool` → `h.riverClient`
- `internal/api/jobs.go` — `h.pool` → `h.riverClient`, fix `retryTaskType` bug, add `retryInsert`
- `internal/api/export.go` — `h.pool` → `h.riverClient`
- `internal/api/sync.go` — `h.pool` → `h.riverClient`
- `cmd/nexorious/serve.go` — pgxPool + River client wiring, updated `RebuildServices`

**Created:**
- `internal/scheduler/backup_poll.go` — `CheckScheduledBackupWorker`

---

## Task 1: Add River dependencies

**Files:**
- Modify: `go.mod` / `go.sum` (via `go get`)

- [ ] **Step 1: Add River packages**

```bash
cd /path/to/nexorious-go
go get riverqueue/river@latest
go get riverqueue/riverdriver-go-pgxv5@latest
go get robfig/cron/v3
```

- [ ] **Step 2: Verify go.mod contains new entries**

```bash
grep "riverqueue\|robfig/cron" go.mod
```

Expected output contains:
```
riverqueue/river v0.x.x
riverqueue/riverdriver-go-pgxv5 v0.x.x
robfig/cron/v3 v3.x.x
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add riverqueue/river and robfig/cron/v3 dependencies"
```

---

## Task 2: Update migration SQL and models

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`
- Modify: `internal/db/migrations/20260503000001_initial.down.sql`
- Modify: `internal/db/models/jobs.go`
- Modify: `internal/db/models/backup_config.go`

- [ ] **Step 1: Remove `pending_tasks` from up migration and add River tables**

In `internal/db/migrations/20260503000001_initial.up.sql`, find and remove the entire `pending_tasks` block:

```sql
-- REMOVE this entire block:
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

In its place, add the River schema (insert at the same location):

```sql
CREATE TABLE river_queue (
    name        TEXT        NOT NULL PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata    JSONB       NOT NULL DEFAULT '{}' ::JSONB,
    paused_at   TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE river_job (
    id           BIGSERIAL    PRIMARY KEY,
    args         JSONB        NOT NULL DEFAULT '{}'::JSONB,
    attempt      SMALLINT     NOT NULL DEFAULT 0,
    attempted_at TIMESTAMPTZ,
    attempted_by TEXT[],
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    errors       JSONB[],
    finalized_at TIMESTAMPTZ,
    kind         TEXT         NOT NULL,
    max_attempts SMALLINT     NOT NULL,
    metadata     JSONB        NOT NULL DEFAULT '{}'::JSONB,
    priority     SMALLINT     NOT NULL DEFAULT 1,
    queue        TEXT         NOT NULL DEFAULT 'default'::TEXT,
    scheduled_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    state        TEXT         NOT NULL DEFAULT 'available'::TEXT,
    tags         TEXT[]       NOT NULL DEFAULT '{}'::TEXT[]
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

- [ ] **Step 2: Add `last_backup_at` to `backup_config` in up migration**

Find the `backup_config` table definition and add the column:

```sql
-- Change this:
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY DEFAULT 1,
    schedule_cron   TEXT NOT NULL DEFAULT '',
    retention_mode  TEXT NOT NULL DEFAULT 'count',
    retention_value INTEGER NOT NULL DEFAULT 5,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- To this:
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY DEFAULT 1,
    schedule_cron   TEXT NOT NULL DEFAULT '',
    retention_mode  TEXT NOT NULL DEFAULT 'count',
    retention_value INTEGER NOT NULL DEFAULT 5,
    last_backup_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 3: Update down migration**

In `internal/db/migrations/20260503000001_initial.down.sql`, replace any `DROP TABLE pending_tasks` with drops for River tables (or add them if the down migration drops all tables):

```sql
DROP TABLE IF EXISTS river_leader;
DROP TABLE IF EXISTS river_job;
DROP TABLE IF EXISTS river_queue;
```

- [ ] **Step 4: Remove `PendingTask` struct from `internal/db/models/jobs.go`**

Delete the entire `PendingTask` section — the struct definition and its section comment. Keep everything from `// --- Job ---` onwards. The file starts with:

```go
package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// --- Job ---
```

- [ ] **Step 5: Add `LastBackupAt` to `internal/db/models/backup_config.go`**

```go
type BackupConfig struct {
	bun.BaseModel `bun:"table:backup_config"`

	ID             int        `bun:"id,pk"               json:"id"`
	ScheduleCron   string     `bun:"schedule_cron,notnull" json:"schedule_cron"`
	RetentionMode  string     `bun:"retention_mode,notnull" json:"retention_mode"`
	RetentionValue int        `bun:"retention_value,notnull" json:"retention_value"`
	LastBackupAt   *time.Time `bun:"last_backup_at"        json:"last_backup_at"`
	CreatedAt      time.Time  `bun:"created_at,notnull"    json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull"    json:"updated_at"`
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/ internal/db/models/
git commit -m "chore(db): replace pending_tasks with River schema, add last_backup_at to backup_config"
```

---

## Task 3: Refactor export workers

**Files:**
- Modify: `internal/worker/tasks/export.go`
- Modify: `internal/worker/tasks/export_test.go`

- [ ] **Step 1: Rewrite the top of `export.go` — replace `exportPayload` and handlers**

Replace the `exportPayload` struct and the two `New*Handler` functions. Keep all helper functions (`loadAndStartJob`, `writeJSONExport`, `buildCSVRow`, etc.) unchanged.

Change the package-level types and constructors at the top of the file to:

```go
package tasks

import (
	"context"
	// ... existing imports unchanged ...

	"riverqueue/river"
)

// ── JSON export ──────────────────────────────────────────────────────────────

type ExportJSONArgs struct {
	JobID string `json:"job_id"`
}

func (ExportJSONArgs) Kind() string { return "export_json" }

func (ExportJSONArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ExportJSONWorker struct {
	DB          *bun.DB
	StoragePath string
}

func (w *ExportJSONWorker) Work(ctx context.Context, job *river.Job[ExportJSONArgs]) error {
	j, err := loadAndStartJob(ctx, w.DB, job.Args.JobID)
	if err != nil {
		slog.Error("export_json: load job", "job_id", job.Args.JobID, "err", err)
		return nil
	}
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	outPath, err := writeJSONExport(w.StoragePath, j.UserID, userGames)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("write JSON: %v", err))
		return nil
	}
	markJobCompleted(ctx, w.DB, j, outPath)
	return nil
}

// ── CSV export ───────────────────────────────────────────────────────────────

type ExportCSVArgs struct {
	JobID string `json:"job_id"`
}

func (ExportCSVArgs) Kind() string { return "export_csv" }

func (ExportCSVArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ExportCSVWorker struct {
	DB          *bun.DB
	StoragePath string
}

func (w *ExportCSVWorker) Work(ctx context.Context, job *river.Job[ExportCSVArgs]) error {
	j, err := loadAndStartJob(ctx, w.DB, job.Args.JobID)
	if err != nil {
		slog.Error("export_csv: load job", "job_id", job.Args.JobID, "err", err)
		return nil
	}
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	outPath, err := writeCSVExport(w.StoragePath, j.UserID, userGames)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("write CSV: %v", err))
		return nil
	}
	markJobCompleted(ctx, w.DB, j, outPath)
	return nil
}
```

- [ ] **Step 2: Update `export_test.go` to use River worker pattern**

Replace all uses of `tasks.NewExportJSONHandler` / `tasks.NewExportCSVHandler` and `*models.PendingTask` with the River worker pattern. Example for JSON:

```go
func TestExportJSON_Task(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	jobID := insertExportJob(t, testDB, userID, "json")

	w := &tasks.ExportJSONWorker{DB: testDB, StoragePath: t.TempDir()}
	err := w.Work(ctx, &river.Job[tasks.ExportJSONArgs]{Args: tasks.ExportJSONArgs{JobID: jobID}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// assert job status = completed
}
```

Apply the same pattern to all export test functions (replacing `handler(ctx, task)` with `w.Work(ctx, &river.Job[tasks.ExportXxxArgs]{Args: ...})`).

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/export_test.go
git commit -m "refactor(worker): convert export handlers to River workers"
```

---

## Task 4: Refactor import worker

**Files:**
- Modify: `internal/worker/tasks/import_item.go`
- Modify: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Replace `importPayload` and `NewImportItemHandler` in `import_item.go`**

Replace the `importPayload` struct and `NewImportItemHandler` function. All helper functions (`findOrCreateTag`, `markItemFailed`, `markItemCompleted`, `igdbMetadataToGame`, `checkJobCompletion`, `parseFlexibleDate`) stay unchanged.

```go
type ImportItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ImportItemArgs) Kind() string { return "import_item" }

func (ImportItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ImportItemWorker struct {
	DB          *bun.DB
	IGDBClient  *igdb.Client
	StoragePath string
}

func (w *ImportItemWorker) Work(ctx context.Context, job *river.Job[ImportItemArgs]) error {
	// ── 1. Load JobItem ───────────────────────────────────────────────────
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.Error("import_item: load job_item", "id", job.Args.JobItemID, "err", err)
		return nil
	}
	// ... rest of the existing handler body unchanged,
	// replacing w.DB for db, w.IGDBClient for igdbClient, w.StoragePath for storagePath
```

The handler body is identical to the existing closure body — just substitute the captured variables with struct fields: `db` → `w.DB`, `igdbClient` → `w.IGDBClient`, `storagePath` → `w.StoragePath`.

- [ ] **Step 2: Update `import_item_test.go`**

Replace `makePendingTask` + `tasks.NewImportItemHandler(...)` pattern with River worker pattern:

```go
// Before:
handler := tasks.NewImportItemHandler(testDB, igdbClient, storagePath)
task := makePendingTask(t, itemID)
err := handler(ctx, task)

// After:
w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: storagePath}
err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}})
```

Apply this substitution throughout all import test functions.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go
git commit -m "refactor(worker): convert import_item handler to River worker"
```

---

## Task 5: Refactor sync workers

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Add args types and convert `DispatchSyncWorker` in `sync.go`**

Replace `dispatchSyncPayload` and `NewDispatchSyncHandler` with:

```go
import (
	// existing imports +
	"riverqueue/river"
	riverpgxv5 "riverqueue/riverdriver-go-pgxv5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DispatchSyncArgs struct {
	JobID      string `json:"job_id"`
	UserID     string `json:"user_id"`
	Storefront string `json:"storefront"`
}

func (DispatchSyncArgs) Kind() string { return "dispatch_sync" }

func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 1}
}

type DispatchSyncWorker struct {
	DB          *bun.DB
	Steam       SteamLibraryAdapter
	PSN         PSNLibraryAdapter
	RiverClient *river.Client[pgx.Tx]
}

func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
	p := job.Args
	// ... identical to the existing closure body ...
	// Replace: task.Payload unmarshal → use p.JobID, p.UserID, p.Storefront directly
	// Replace db.NewRaw INSERT INTO pending_tasks with:
	//   _, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
```

The raw SQL `INSERT INTO pending_tasks` for each sync item (around line 225 in the original) becomes:

```go
_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
```

- [ ] **Step 2: Add args type and convert `ProcessSyncItemWorker` in `sync.go`**

Replace `processSyncItemPayload` and `NewProcessSyncItemHandler` with:

```go
type ProcessSyncItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (ProcessSyncItemArgs) Kind() string { return "process_sync_item" }

func (ProcessSyncItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

type ProcessSyncItemWorker struct {
	DB         *bun.DB
	IGDBClient *igdbsvc.Client
}

func (w *ProcessSyncItemWorker) Work(ctx context.Context, job *river.Job[ProcessSyncItemArgs]) error {
	// identical to existing closure body
	// Replace: json.Unmarshal(task.Payload, &p) → p := job.Args
	// Replace: db → w.DB, igdbClient → w.IGDBClient
```

- [ ] **Step 3: Update `sync_test.go`**

Add a `testPgxPool` package-level var used for creating a non-started River client in tests that exercise the full dispatch path. For tests that return before the River insert (invalid payload, no config, etc.), pass `nil` as `RiverClient`.

```go
// In tests that reach the river.Insert call, construct:
pgxPool, err := pgxpool.New(ctx, testConnStr)
if err != nil {
    t.Fatalf("pgxpool.New: %v", err)
}
defer pgxPool.Close()
rc, _ := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{})

w := &tasks.DispatchSyncWorker{
    DB:          testDB,
    Steam:       &fakeSteamAdapter{games: steamGames},
    PSN:         nil,
    RiverClient: rc,
}
err = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
    Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
})
```

For tests that hit early-return paths (invalid payload, no sync config), set `RiverClient: nil` — the nil client is never reached.

Replace `tasks.NewDispatchSyncHandler(testDB, ...)` calls throughout and replace `tasks.NewProcessSyncItemHandler(testDB, ...)` calls with `&tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: ...}`.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "refactor(worker): convert sync handlers to River workers"
```

---

## Task 6: Refactor metadata refresh workers

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go`
- Modify: `internal/worker/tasks/metadata_refresh_test.go`

- [ ] **Step 1: Add args types and convert `MetadataRefreshDispatchWorker`**

Replace `NewMetadataRefreshDispatchHandler`. The dispatch worker creates a bun transaction for job/job_items, then inserts River jobs after commit (non-transactional — acceptable for a background maintenance job):

```go
type MetadataRefreshDispatchArgs struct{}

func (MetadataRefreshDispatchArgs) Kind() string { return "metadata_refresh_dispatch" }

func (MetadataRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type MetadataRefreshDispatchWorker struct {
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *MetadataRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshDispatchArgs]) error {
	// Steps 1-4 unchanged (IGDB guard, admin user, duplicate check, load games)
	// ...

	// Step 5 — Create job and items in a bun transaction (no River inserts here)
	var itemIDs []string
	jobID := uuid.NewString()
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// 5a — Insert job (unchanged)
		// 5b — Insert job_items, collect itemIDs
		for _, g := range games {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)
			// insert job_item (unchanged SQL)
		}
		// 5c — Mark job processing (unchanged)
		return nil
	}); err != nil {
		slog.Error("metadata_refresh_dispatch: transaction failed", "err", err)
		return nil
	}

	// Step 6 — Insert River jobs after transaction (non-transactional; acceptable for maintenance)
	for _, itemID := range itemIDs {
		_, _ = w.RiverClient.Insert(ctx, MetadataRefreshItemArgs{JobItemID: itemID}, nil)
	}

	slog.Info("metadata_refresh_dispatch: job created", "job_id", jobID, "game_count", len(games))
	return nil
}
```

- [ ] **Step 2: Convert `MetadataRefreshItemWorker`**

Replace `metadataRefreshItemPayload` and `NewMetadataRefreshItemHandler`:

```go
type MetadataRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (MetadataRefreshItemArgs) Kind() string { return "metadata_refresh_item" }

func (MetadataRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type MetadataRefreshItemWorker struct {
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	StoragePath string
}

func (w *MetadataRefreshItemWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshItemArgs]) error {
	// identical to existing closure body
	// Replace: json.Unmarshal(task.Payload, &payload) → payload := job.Args
	// Replace: db → w.DB, igdbClient → w.IGDBClient, storagePath → w.StoragePath
```

- [ ] **Step 3: Update `metadata_refresh_test.go`**

For the dispatch worker test, create a minimal River client (same pattern as Task 5):

```go
pgxPool, _ := pgxpool.New(ctx, testConnStr)
defer pgxPool.Close()
rc, _ := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{})

w := &tasks.MetadataRefreshDispatchWorker{
    DB:          testDB,
    IGDBClient:  igdbClient,
    RiverClient: rc,
}
err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{
    Args: tasks.MetadataRefreshDispatchArgs{},
})
```

For the item worker:
```go
w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
err := w.Work(ctx, &river.Job[tasks.MetadataRefreshItemArgs]{
    Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
})
```

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go internal/worker/tasks/metadata_refresh_test.go
git commit -m "refactor(worker): convert metadata_refresh handlers to River workers"
```

---

## Task 7: Update testmain — expose connStr, remove makePendingTask

**Files:**
- Modify: `internal/worker/tasks/testmain_test.go`

- [ ] **Step 1: Expose `testConnStr` and clean up `testmain_test.go`**

```go
package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/db/models"
)

var (
	testDB      *bun.DB
	testConnStr string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		panic("get connection string: " + err.Error())
	}
	testConnStr = connStr

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	testDB = bun.NewDB(sqldb, pgdialect.New())

	migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		_ = testDB.Close()
		_ = ctr.Terminate(ctx)
		panic("migrator init: " + err.Error())
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		_ = testDB.Close()
		_ = ctr.Terminate(ctx)
		panic("migrate: " + err.Error())
	}

	code := m.Run()

	_ = testDB.Close()
	_ = ctr.Terminate(ctx)
	os.Exit(code)
}

func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		TRUNCATE TABLE
			users, user_sessions, games, external_games,
			platforms, storefronts, platform_storefronts,
			tags, user_games, user_game_tags, user_game_platforms,
			jobs, job_items, river_job, backup_config,
			user_sync_configs, rate_limiter_tokens
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}

// insertTestUser, insertTestJob, insertTestJobItem, mustMarshal remain unchanged.
// makePendingTask is deleted — no longer needed.
```

- [ ] **Step 2: Commit**

```bash
git add internal/worker/tasks/testmain_test.go
git commit -m "test(worker): expose testConnStr, remove makePendingTask, update truncateAllTables"
```

---

## Task 8: Scheduler — River periodic workers and backup poller

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/backup_poll.go`
- Delete: `internal/scheduler/lifecycle_test.go`

- [ ] **Step 1: Rewrite `scheduler.go`**

Replace the entire file. The `CleanupOldJobs`, `CleanupExports`, `CleanupUnreferencedGames`, `CleanupExpiredSessions`, `CleanupStaleJobs`, and `CheckPendingSyncs` standalone functions are unchanged and stay at the bottom of the file. The `Scheduler` struct, `NewScheduler`, `Start`, `Stop`, and `RebuildBackupJob` are replaced:

```go
package scheduler

import (
	"context"
	"log/slog"
	"os"
	"time"

	"riverqueue/river"
	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// BuildPeriodicJobs returns the River PeriodicJob list. Called from serve.go
// when constructing the River client config.
func BuildPeriodicJobs(cfg *config.Config, staleThreshold time.Duration) []*river.PeriodicJob {
	interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
	if err != nil {
		slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
			"value", cfg.MetadataRefreshInterval, "err", err)
		interval = 24 * time.Hour
	}

	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.NewCronSchedule("0 3 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupOldJobsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NewCronSchedule("0 4 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupExportsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NewCronSchedule("0 5 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupUnreferencedGamesArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NewCronSchedule("*/30 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupExpiredSessionsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NewCronSchedule("0 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) {
				return CleanupStaleJobsArgs{Threshold: staleThreshold.String()}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.NewCronSchedule("*/15 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CheckPendingSyncsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) { return tasks.MetadataRefreshDispatchArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return CheckScheduledBackupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}
}

// ── Periodic worker types ─────────────────────────────────────────────────

type CleanupOldJobsArgs struct{}
func (CleanupOldJobsArgs) Kind() string { return "cleanup_old_jobs" }
type CleanupOldJobsWorker struct{ DB *bun.DB }
func (w *CleanupOldJobsWorker) Work(ctx context.Context, _ *river.Job[CleanupOldJobsArgs]) error {
	CleanupOldJobs(ctx, w.DB)
	return nil
}

type CleanupExportsArgs struct{}
func (CleanupExportsArgs) Kind() string { return "cleanup_exports" }
type CleanupExportsWorker struct{ DB *bun.DB }
func (w *CleanupExportsWorker) Work(ctx context.Context, _ *river.Job[CleanupExportsArgs]) error {
	CleanupExports(ctx, w.DB)
	return nil
}

type CleanupUnreferencedGamesArgs struct{}
func (CleanupUnreferencedGamesArgs) Kind() string { return "cleanup_unreferenced_games" }
type CleanupUnreferencedGamesWorker struct{ DB *bun.DB }
func (w *CleanupUnreferencedGamesWorker) Work(ctx context.Context, _ *river.Job[CleanupUnreferencedGamesArgs]) error {
	CleanupUnreferencedGames(ctx, w.DB)
	return nil
}

type CleanupExpiredSessionsArgs struct{}
func (CleanupExpiredSessionsArgs) Kind() string { return "cleanup_expired_sessions" }
type CleanupExpiredSessionsWorker struct{ DB *bun.DB }
func (w *CleanupExpiredSessionsWorker) Work(ctx context.Context, _ *river.Job[CleanupExpiredSessionsArgs]) error {
	CleanupExpiredSessions(ctx, w.DB)
	return nil
}

type CleanupStaleJobsArgs struct{ Threshold string `json:"threshold"` }
func (CleanupStaleJobsArgs) Kind() string { return "cleanup_stale_jobs" }
type CleanupStaleJobsWorker struct{ DB *bun.DB }
func (w *CleanupStaleJobsWorker) Work(ctx context.Context, job *river.Job[CleanupStaleJobsArgs]) error {
	d, err := time.ParseDuration(job.Args.Threshold)
	if err != nil {
		return nil
	}
	CleanupStaleJobs(ctx, w.DB, d)
	return nil
}

type CheckPendingSyncsArgs struct{}
func (CheckPendingSyncsArgs) Kind() string { return "check_pending_syncs" }
type CheckPendingSyncsWorker struct {
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}
func (w *CheckPendingSyncsWorker) Work(ctx context.Context, _ *river.Job[CheckPendingSyncsArgs]) error {
	CheckPendingSyncs(ctx, w.DB, w.RiverClient)
	return nil
}
```

Also update the `CheckPendingSyncs` function signature at the bottom of the file — replace `pool *worker.Pool` with `riverClient *river.Client[pgx.Tx]`, and replace `pool.Submit(...)` with:

```go
_, _ = riverClient.Insert(ctx, tasks.DispatchSyncArgs{
    JobID: jobID, UserID: cfg.UserID, Storefront: cfg.Storefront,
}, nil)
```

- [ ] **Step 2: Create `internal/scheduler/backup_poll.go`**

```go
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"riverqueue/river"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/db/models"
)

type CheckScheduledBackupArgs struct{}

func (CheckScheduledBackupArgs) Kind() string { return "check_scheduled_backup" }

type CheckScheduledBackupWorker struct {
	DB        *bun.DB
	BackupSvc *backup.Service
}

func (w *CheckScheduledBackupWorker) Work(ctx context.Context, _ *river.Job[CheckScheduledBackupArgs]) error {
	if !backup.PgDumpAvailable() {
		return nil
	}
	var cfg models.BackupConfig
	if err := w.DB.NewSelect().Model(&cfg).Where("id = 1").Scan(ctx); err != nil || cfg.ScheduleCron == "" {
		return nil
	}
	sched, err := cron.ParseStandard(cfg.ScheduleCron)
	if err != nil {
		slog.Warn("check_scheduled_backup: invalid cron expression", "cron", cfg.ScheduleCron, "err", err)
		return nil
	}
	now := time.Now().UTC()
	prev := sched.Prev(now)
	if cfg.LastBackupAt != nil && !cfg.LastBackupAt.Before(prev) {
		return nil
	}
	id, err := w.BackupSvc.CreateBackup("scheduled")
	if err != nil {
		slog.Error("scheduled backup failed", "err", err)
		return err
	}
	slog.Info("scheduled backup created", "id", id)
	if err := w.BackupSvc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue); err != nil {
		slog.Warn("scheduled backup retention cleanup failed", "err", err)
	}
	// Update last_backup_at so the next poll knows this window was covered.
	_, _ = w.DB.NewRaw(
		`UPDATE backup_config SET last_backup_at = now(), updated_at = now() WHERE id = 1`,
	).Exec(context.Background())
	return nil
}
```

- [ ] **Step 3: Delete `lifecycle_test.go`**

```bash
rm internal/scheduler/lifecycle_test.go
```

- [ ] **Step 4: Commit**

```bash
git add internal/scheduler/
git commit -m "refactor(scheduler): replace gocron with River periodic workers"
```

---

## Task 9: Update API handlers — pool → riverClient

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/export.go`
- Modify: `internal/api/import.go`
- Modify: `internal/api/job_items.go`
- Modify: `internal/api/jobs.go`
- Modify: `internal/api/sync.go`

- [ ] **Step 1: Update `router.go` — change `New` and `registerRoutes` signatures**

```go
// Change:
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, pool ...*worker.Pool) *echo.Echo {
	var wp *worker.Pool
	if len(pool) > 0 {
		wp = pool[0]
	}

// To:
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
	var rc *river.Client[pgx.Tx]
	if len(riverClient) > 0 {
		rc = riverClient[0]
	}
```

Update `registerRoutes` similarly, replacing `pool *worker.Pool` with `riverClient *river.Client[pgx.Tx]` and updating the 5 handler constructor calls:

```go
jh  := NewJobsHandler(db, rc)
jih := NewJobItemsHandler(db, rc)
imh := NewImportHandler(db, rc)
exh := NewExportHandler(db, rc, cfg)
synch := NewSyncHandler(db, rc, &steamClientAdapter{c: steamSvc}, &psnClientAdapter{c: psnSvc})
```

- [ ] **Step 2: Update `export.go` handler struct**

```go
// Change handler struct field:
type ExportHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	cfg         *config.Config
}

func NewExportHandler(db *bun.DB, riverClient *river.Client[pgx.Tx], cfg *config.Config) *ExportHandler {
	return &ExportHandler{db: db, riverClient: riverClient, cfg: cfg}
}
```

Replace the `h.pool.Submit(ctx, taskType, ...)` call with:

```go
taskType := "export_json"
if format == "csv" {
    taskType = "export_csv"
}
if taskType == "export_json" {
    _, err = h.riverClient.Insert(ctx, tasks.ExportJSONArgs{JobID: job.ID}, nil)
} else {
    _, err = h.riverClient.Insert(ctx, tasks.ExportCSVArgs{JobID: job.ID}, nil)
}
if err != nil {
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to submit export task")
}
```

- [ ] **Step 3: Update `import.go` handler struct**

```go
type ImportHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

func NewImportHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *ImportHandler {
	return &ImportHandler{db: db, riverClient: riverClient}
}
```

Replace `h.pool.Submit(ctx, "import_item", map[string]string{"job_item_id": item.ID}, 5)` with:

```go
if _, err := h.riverClient.Insert(ctx, tasks.ImportItemArgs{JobItemID: item.ID}, nil); err != nil {
    slog.Error("import: submit task", "item_id", item.ID, "err", err)
}
```

- [ ] **Step 4: Update `job_items.go` handler struct**

```go
type JobItemsHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

func NewJobItemsHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *JobItemsHandler {
	return &JobItemsHandler{db: db, riverClient: riverClient}
}
```

The two `h.pool.Submit` calls in `job_items.go` retry specific job item types. Inspect each call site to determine which args type to use:
- Where `taskType` is `"import_item"` or `"metadata_refresh_item"`, use the corresponding args type:

```go
// Replace h.pool.Submit(context.Background(), taskType, payload, 5) with:
switch taskType {
case "import_item":
    _, _ = h.riverClient.Insert(context.Background(), tasks.ImportItemArgs{JobItemID: item.ID}, nil)
case "metadata_refresh_item":
    _, _ = h.riverClient.Insert(context.Background(), tasks.MetadataRefreshItemArgs{JobItemID: item.ID}, nil)
case "process_sync_item":
    _, _ = h.riverClient.Insert(context.Background(), tasks.ProcessSyncItemArgs{JobItemID: item.ID}, nil)
}
```

- [ ] **Step 5: Update `jobs.go` handler struct + fix `retryTaskType` bug**

```go
type JobsHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

func NewJobsHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *JobsHandler {
	return &JobsHandler{db: db, riverClient: riverClient}
}
```

Replace `retryTaskType` and the `h.pool.Submit` loop with `retryInsert`:

```go
func retryInsert(ctx context.Context, rc *river.Client[pgx.Tx], jobType, jobItemID string) {
	switch jobType {
	case models.JobTypeSync:
		_, _ = rc.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: jobItemID}, nil)
	case models.JobTypeImport:
		_, _ = rc.Insert(ctx, tasks.ImportItemArgs{JobItemID: jobItemID}, nil)
	case models.JobTypeMetadataRefresh:
		_, _ = rc.Insert(ctx, tasks.MetadataRefreshItemArgs{JobItemID: jobItemID}, nil)
	}
}
```

Replace the `for _, item := range failedItems` loop:

```go
for _, item := range failedItems {
    retryInsert(context.Background(), h.riverClient, job.JobType, item.ID)
}
```

Delete `retryTaskType`.

- [ ] **Step 6: Update `sync.go` handler struct**

```go
type SyncHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	// ... other fields unchanged
}

func NewSyncHandler(db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamAdapter, psn PSNAdapter) *SyncHandler {
	return &SyncHandler{db: db, riverClient: riverClient, steam: steam, psn: psn}
}
```

Replace `h.pool.Submit(ctx, "dispatch_sync", ...)` with:

```go
_, _ = h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
    JobID: jobID, UserID: userID, Storefront: storefront,
}, nil)
```

- [ ] **Step 7: Commit**

```bash
git add internal/api/
git commit -m "refactor(api): replace worker.Pool with river.Client in all API handlers"
```

---

## Task 10: Rewire serve.go and delete pool files

**Files:**
- Modify: `cmd/nexorious/serve.go`
- Delete: `internal/worker/pool.go`
- Delete: `internal/worker/pool_test.go`

- [ ] **Step 1: Rewrite the worker/scheduler section of `runServe`**

Replace the pool construction block (after the IGDB client setup) with:

```go
import (
	// add:
	"riverqueue/river"
	riverpgxv5 "riverqueue/riverdriver-go-pgxv5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── pgxPool for River ──────────────────────────────────────────────────────
pgxPool, err := pgxpool.New(ctx, resolvedDatabaseURL)
if err != nil {
    return fmt.Errorf("pgxpool.New: %w", err)
}
defer pgxPool.Close()

// ── River workers ──────────────────────────────────────────────────────────
staleThreshold, err := time.ParseDuration(cfg.StaleJobThreshold)
if err != nil {
    slog.Warn("invalid STALE_JOB_THRESHOLD, defaulting to 4h", "value", cfg.StaleJobThreshold)
    staleThreshold = 4 * time.Hour
}

// Workers that need the River client are wired after client creation.
dispatchSyncWorker     := &tasks.DispatchSyncWorker{DB: db, Steam: steamsvc.NewClient(), PSN: psnsvc.NewClient()}
metaDispatchWorker     := &tasks.MetadataRefreshDispatchWorker{DB: db, IGDBClient: igdbClient}
checkPendingSyncsWorker := &scheduler.CheckPendingSyncsWorker{DB: db}

workers := river.NewWorkers()
river.AddWorker(workers, &tasks.ImportItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &tasks.ExportJSONWorker{DB: db, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &tasks.ExportCSVWorker{DB: db, StoragePath: cfg.StoragePath})
river.AddWorker(workers, dispatchSyncWorker)
river.AddWorker(workers, &tasks.ProcessSyncItemWorker{DB: db, IGDBClient: igdbClient})
river.AddWorker(workers, metaDispatchWorker)
river.AddWorker(workers, &tasks.MetadataRefreshItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
river.AddWorker(workers, &scheduler.CleanupOldJobsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupExportsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupUnreferencedGamesWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupExpiredSessionsWorker{DB: db})
river.AddWorker(workers, &scheduler.CleanupStaleJobsWorker{DB: db})
river.AddWorker(workers, checkPendingSyncsWorker)
river.AddWorker(workers, &scheduler.CheckScheduledBackupWorker{DB: db, BackupSvc: backupSvc})

riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{
    Workers: workers,
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
    PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
})
if err != nil {
    return fmt.Errorf("river.NewClient: %w", err)
}

// Wire River client into workers that submit sub-jobs.
dispatchSyncWorker.RiverClient = riverClient
metaDispatchWorker.RiverClient = riverClient
checkPendingSyncsWorker.RiverClient = riverClient
```

- [ ] **Step 2: Update `RebuildServices` callback**

Replace the entire `RebuildServices` function body (pool teardown/rebuild) with:

```go
RebuildServices: func(newDB *bun.DB) error {
    riverClient.Stop(shutdownCtx)
    pgxPool.Close()

    newPgxPool, err := pgxpool.New(ctx, resolvedDatabaseURL)
    if err != nil {
        return fmt.Errorf("pgxpool.New after restore: %w", err)
    }
    pgxPool = newPgxPool

    newDispatchSync     := &tasks.DispatchSyncWorker{DB: newDB, Steam: steamsvc.NewClient(), PSN: psnsvc.NewClient()}
    newMetaDispatch     := &tasks.MetadataRefreshDispatchWorker{DB: newDB, IGDBClient: igdbClient}
    newCheckPendingSyncs := &scheduler.CheckPendingSyncsWorker{DB: newDB}

    newWorkers := river.NewWorkers()
    river.AddWorker(newWorkers, &tasks.ImportItemWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
    river.AddWorker(newWorkers, &tasks.ExportJSONWorker{DB: newDB, StoragePath: cfg.StoragePath})
    river.AddWorker(newWorkers, &tasks.ExportCSVWorker{DB: newDB, StoragePath: cfg.StoragePath})
    river.AddWorker(newWorkers, newDispatchSync)
    river.AddWorker(newWorkers, &tasks.ProcessSyncItemWorker{DB: newDB, IGDBClient: igdbClient})
    river.AddWorker(newWorkers, newMetaDispatch)
    river.AddWorker(newWorkers, &tasks.MetadataRefreshItemWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
    river.AddWorker(newWorkers, &scheduler.CleanupOldJobsWorker{DB: newDB})
    river.AddWorker(newWorkers, &scheduler.CleanupExportsWorker{DB: newDB})
    river.AddWorker(newWorkers, &scheduler.CleanupUnreferencedGamesWorker{DB: newDB})
    river.AddWorker(newWorkers, &scheduler.CleanupExpiredSessionsWorker{DB: newDB})
    river.AddWorker(newWorkers, &scheduler.CleanupStaleJobsWorker{DB: newDB})
    river.AddWorker(newWorkers, newCheckPendingSyncs)
    river.AddWorker(newWorkers, &scheduler.CheckScheduledBackupWorker{DB: newDB, BackupSvc: backupSvc})

    newClient, err := river.NewClient(riverpgxv5.New(newPgxPool), &river.Config{
        Workers:      newWorkers,
        Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
        PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
    })
    if err != nil {
        return fmt.Errorf("river.NewClient after restore: %w", err)
    }
    newDispatchSync.RiverClient = newClient
    newMetaDispatch.RiverClient = newClient
    newCheckPendingSyncs.RiverClient = newClient

    if err := newClient.Start(shutdownCtx); err != nil {
        return fmt.Errorf("river client start after restore: %w", err)
    }
    riverClient = newClient
    backupSvc.SetDB(newDB)
    slog.Info("River client restarted after restore")
    return nil
},
```

- [ ] **Step 3: Update the worker/scheduler start gate**

Replace the `pool.Start(ctx, cfg.WorkerCount)` + `sched = scheduler.NewScheduler(...)` goroutine with:

```go
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
            if err := riverClient.Start(ctx); err != nil {
                slog.Error("failed to start River client", "err", err)
            }
            slog.Info("app ready — River client started")
            return
        }
        time.Sleep(2 * time.Second)
    }
}(shutdownCtx)
```

- [ ] **Step 4: Update graceful shutdown**

Replace `sched.Stop()` / `pool.Shutdown()` with:

```go
if err := riverClient.Stop(shutdownCtx); err != nil {
    slog.Warn("River client stop", "err", err)
}
```

- [ ] **Step 5: Update `api.New` call in `runServe`**

```go
e := api.New(cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, riverClient)
```

- [ ] **Step 6: Remove `go-co-op/gocron/v2` import from serve.go and run tidy**

```bash
go mod tidy
```

Verify gocron is no longer in go.mod:

```bash
grep gocron go.mod
# Expected: no output
```

- [ ] **Step 7: Delete pool files**

```bash
rm internal/worker/pool.go internal/worker/pool_test.go
```

- [ ] **Step 8: Commit**

```bash
git add cmd/nexorious/serve.go go.mod go.sum
git rm internal/worker/pool.go internal/worker/pool_test.go
git commit -m "feat: migrate worker pool and scheduler to River, remove gocron"
```

---

## Task 11: Build and test

- [ ] **Step 1: Build**

```bash
go build ./...
```

Expected: zero errors. Fix any remaining import or type errors before continuing.

- [ ] **Step 2: Run Go tests**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass. The most likely failure points:
- `truncateAllTables` referencing old table names → fixed in Task 7
- Tests that constructed `*models.PendingTask` directly → fixed in Tasks 3–6
- `lifecycle_test.go` — deleted in Task 8

- [ ] **Step 3: Run linter**

```bash
golangci-lint run
```

Expected: zero errors.

- [ ] **Step 4: Final commit if any lint fixes were needed**

```bash
git add -p
git commit -m "fix: address golangci-lint findings after River migration"
```

- [ ] **Step 5: Push**

```bash
bd dolt push
git pull --rebase
git push
```
