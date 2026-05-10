# Worker Pool & Job Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the database-backed task queue, worker pool, job/job-item tracking API, and scheduler with cleanup jobs.

**Architecture:** A worker pool of goroutines claims tasks from a `pending_tasks` table using `SELECT ... FOR UPDATE SKIP LOCKED`. Jobs and job items provide user-visible tracking. The scheduler runs cleanup cron jobs via gocron v2. All API endpoints are JWT-protected and user-scoped.

**Tech Stack:** Go, Bun ORM, PostgreSQL, Echo v5, gocron v2, testcontainers-go

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Modify | Update `jobs` and `job_items` tables to match spec schema |
| `internal/db/migrations/20260503000001_initial.down.sql` | No change | Already has correct DROP statements |
| `internal/db/models/jobs.go` | Create | Bun model structs + constants for PendingTask, Job, JobItem |
| `internal/worker/pool.go` | Create | Pool type, handler registry, Submit, Start, Shutdown, worker loop |
| `internal/worker/pool_test.go` | Create | Worker pool tests with testcontainers |
| `internal/api/jobs.go` | Create | All job endpoints handler |
| `internal/api/jobs_test.go` | Create | Job endpoint tests |
| `internal/api/job_items.go` | Create | All job-item endpoints handler |
| `internal/api/job_items_test.go` | Create | Job-item endpoint tests |
| `internal/scheduler/scheduler.go` | Create | gocron scheduler with cleanup jobs |
| `internal/scheduler/scheduler_test.go` | Create | Scheduler cleanup query tests |
| `internal/api/router.go` | Modify | Add job/job-item route groups, accept pool parameter |
| `cmd/nexorious/main.go` | Modify | Wire pool + scheduler startup/shutdown |
| `slumber.yaml` | Modify | Add requests for all new endpoints |

---

## Task 1: Update Migration Schema

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`

The existing `jobs` and `job_items` tables don't match the spec. Since no production database exists, we replace them in-place.

- [ ] **Step 1: Replace the `jobs` table definition**

Replace lines 214–233 (the current `CREATE TABLE jobs` block and its indexes) with the spec's schema:

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

- [ ] **Step 2: Replace the `job_items` table definition**

Replace lines 236–253 (the current `CREATE TABLE job_items` block and its indexes) with the spec's schema:

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

- [ ] **Step 3: Verify the migration loads**

Run: `go test ./internal/api/... -run TestAppStateMiddleware_ReadyState -v -count=1`
Expected: PASS — this runs migrations via testcontainers, confirming the SQL is valid.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql
git commit -m "feat: update jobs/job_items schema to match worker pool spec"
```

---

## Task 2: Bun Model Structs

**Files:**
- Create: `internal/db/models/jobs.go`

- [ ] **Step 1: Create the models file with all three structs and constants**

Create `internal/db/models/jobs.go`:

```go
package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// ─── PendingTask ─────────────────────────────────────────────────────────────

type PendingTask struct {
	bun.BaseModel `bun:"table:pending_tasks"`

	ID        string          `bun:"id,pk"                 json:"id"`
	TaskType  string          `bun:"task_type,notnull"      json:"task_type"`
	Payload   json.RawMessage `bun:"payload,notnull"        json:"payload"`
	Priority  int             `bun:"priority,notnull"       json:"priority"`
	Status    string          `bun:"status,notnull"         json:"status"`
	Attempts  int             `bun:"attempts,notnull"       json:"attempts"`
	LastError *string         `bun:"last_error"             json:"last_error"`
	CreatedAt time.Time       `bun:"created_at,notnull"     json:"created_at"`
	ClaimedAt *time.Time      `bun:"claimed_at"             json:"claimed_at"`
	DoneAt    *time.Time      `bun:"done_at"                json:"done_at"`
}

// ─── Job ─────────────────────────────────────────────────────────────────────

// Job type constants.
const (
	JobTypeSync            = "sync"
	JobTypeImport          = "import"
	JobTypeExport          = "export"
	JobTypeMetadataRefresh = "metadata_refresh"
)

// Job source constants.
const (
	JobSourceSteam     = "steam"
	JobSourceEpic      = "epic"
	JobSourcePSN       = "psn"
	JobSourceGOG       = "gog"
	JobSourceManual    = "manual"
	JobSourceNexorious = "nexorious"
	JobSourceCSV       = "csv"
	JobSourceSystem    = "system"
)

// Job status constants.
const (
	JobStatusPending    = "pending"
	JobStatusProcessing = "processing"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
	JobStatusCancelled  = "cancelled"
)

// Job priority constants.
const (
	JobPriorityLow    = "low"
	JobPriorityNormal = "normal"
	JobPriorityHigh   = "high"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs"`

	ID            string     `bun:"id,pk"                    json:"id"`
	UserID        string     `bun:"user_id,notnull"          json:"user_id"`
	JobType       string     `bun:"job_type,notnull"         json:"job_type"`
	Source        string     `bun:"source,notnull"           json:"source"`
	Status        string     `bun:"status,notnull"           json:"status"`
	Priority      string     `bun:"priority,notnull"         json:"priority"`
	FilePath      *string    `bun:"file_path"                json:"file_path"`
	TotalItems    int        `bun:"total_items,notnull"      json:"total_items"`
	ErrorMessage  *string    `bun:"error_message"            json:"error_message"`
	AutoRetryDone bool       `bun:"auto_retry_done,notnull"  json:"auto_retry_done"`
	CreatedAt     time.Time  `bun:"created_at,notnull"       json:"created_at"`
	StartedAt     *time.Time `bun:"started_at"               json:"started_at"`
	CompletedAt   *time.Time `bun:"completed_at"             json:"completed_at"`
}

// IsActive returns true if the job is pending or processing.
func (j *Job) IsActive() bool {
	return j.Status == JobStatusPending || j.Status == JobStatusProcessing
}

// IsTerminal returns true if the job is completed, failed, or cancelled.
func (j *Job) IsTerminal() bool {
	return j.Status == JobStatusCompleted || j.Status == JobStatusFailed || j.Status == JobStatusCancelled
}

// DurationSeconds returns elapsed seconds from StartedAt to CompletedAt
// (or now if still running). Returns nil if StartedAt is not set.
func (j *Job) DurationSeconds() *float64 {
	if j.StartedAt == nil {
		return nil
	}
	end := time.Now()
	if j.CompletedAt != nil {
		end = *j.CompletedAt
	}
	d := end.Sub(*j.StartedAt).Seconds()
	return &d
}

// ─── JobItem ─────────────────────────────────────────────────────────────────

// JobItem status constants.
const (
	JobItemStatusPending       = "pending"
	JobItemStatusProcessing    = "processing"
	JobItemStatusCompleted     = "completed"
	JobItemStatusPendingReview = "pending_review"
	JobItemStatusSkipped       = "skipped"
	JobItemStatusFailed        = "failed"
)

type JobItem struct {
	bun.BaseModel `bun:"table:job_items"`

	ID              string          `bun:"id,pk"                    json:"id"`
	JobID           string          `bun:"job_id,notnull"           json:"job_id"`
	UserID          string          `bun:"user_id,notnull"          json:"user_id"`
	ItemKey         string          `bun:"item_key,notnull"         json:"item_key"`
	SourceTitle     string          `bun:"source_title,notnull"     json:"source_title"`
	SourceMetadata  json.RawMessage `bun:"source_metadata,notnull"  json:"source_metadata"`
	Status          string          `bun:"status,notnull"           json:"status"`
	Result          json.RawMessage `bun:"result,notnull"           json:"result"`
	ErrorMessage    *string         `bun:"error_message"            json:"error_message"`
	IGDBCandidates  json.RawMessage `bun:"igdb_candidates,notnull"  json:"igdb_candidates"`
	ResolvedIGDBID  *int            `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	MatchConfidence *float64        `bun:"match_confidence"         json:"match_confidence"`
	CreatedAt       time.Time       `bun:"created_at,notnull"       json:"created_at"`
	ProcessedAt     *time.Time      `bun:"processed_at"             json:"processed_at"`
	ResolvedAt      *time.Time      `bun:"resolved_at"              json:"resolved_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/db/models/...`
Expected: success, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/db/models/jobs.go
git commit -m "feat: add Bun model structs for PendingTask, Job, JobItem"
```

---

## Task 3: Worker Pool Core

**Files:**
- Create: `internal/worker/pool.go`
- Create: `internal/worker/pool_test.go`

- [ ] **Step 1: Write pool_test.go with the first test — submit and process a task**

Create `internal/worker/pool_test.go`:

```go
package worker_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
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
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func TestPool_SubmitAndProcess(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)

	var called atomic.Bool
	done := make(chan struct{})
	pool.Register("test_task", func(ctx context.Context, task *models.PendingTask) error {
		called.Store(true)
		close(done)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)

	err := pool.Submit(context.Background(), "test_task", map[string]string{"key": "value"}, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for task to be processed")
	}

	if !called.Load() {
		t.Fatal("handler was not called")
	}

	// Verify task is marked done in DB.
	var task models.PendingTask
	err = db.NewSelect().Model(&task).Where("task_type = ?", "test_task").Scan(context.Background())
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if task.Status != "done" {
		t.Fatalf("expected status=done, got %s", task.Status)
	}
	if task.DoneAt == nil {
		t.Fatal("expected done_at to be set")
	}

	pool.Shutdown()
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/... -run TestPool_SubmitAndProcess -v -count=1`
Expected: FAIL — `internal/worker` package doesn't exist yet.

- [ ] **Step 3: Create pool.go with minimal implementation**

Create `internal/worker/pool.go`:

```go
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// TaskHandler processes a single claimed task.
type TaskHandler func(ctx context.Context, task *models.PendingTask) error

// Pool is a database-backed worker pool that claims and processes pending tasks.
type Pool struct {
	db       *bun.DB
	handlers map[string]TaskHandler
	notify   chan struct{}
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	started  bool
}

// NewPool creates a new worker pool with an empty handler registry.
func NewPool(db *bun.DB) *Pool {
	return &Pool{
		db:       db,
		handlers: make(map[string]TaskHandler),
		notify:   make(chan struct{}, 1),
	}
}

// Register adds a task handler. Must be called before Start.
func (p *Pool) Register(taskType string, handler TaskHandler) {
	if p.started {
		panic("worker.Pool: Register called after Start")
	}
	p.handlers[taskType] = handler
}

// Submit inserts a pending task and signals workers. Never blocks.
func (p *Pool) Submit(ctx context.Context, taskType string, payload any, priority int) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("worker.Submit: marshal payload: %w", err)
	}

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: taskType,
		Payload:  data,
		Priority: priority,
		Status:   "pending",
	}

	_, err = p.db.NewInsert().Model(task).Exec(ctx)
	if err != nil {
		return fmt.Errorf("worker.Submit: insert: %w", err)
	}

	// Non-blocking signal to workers.
	select {
	case p.notify <- struct{}{}:
	default:
	}

	return nil
}

// Start spawns the given number of worker goroutines.
func (p *Pool) Start(ctx context.Context, workers int) {
	p.started = true
	var workerCtx context.Context
	workerCtx, p.cancel = context.WithCancel(ctx)
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.run(workerCtx, i)
	}
	slog.Info("worker pool started", "workers", workers)
}

// Shutdown cancels the context and waits for all in-flight tasks to complete.
func (p *Pool) Shutdown() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("worker pool shut down")
}

func (p *Pool) run(ctx context.Context, id int) {
	defer p.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.notify:
		case <-ticker.C:
		}

		for {
			if ctx.Err() != nil {
				return
			}
			processed := p.claimAndProcess(ctx)
			if !processed {
				break
			}
		}
	}
}

func (p *Pool) claimAndProcess(ctx context.Context) (processed bool) {
	var task models.PendingTask
	err := p.db.NewRaw(`
		UPDATE pending_tasks
		SET status = 'running', claimed_at = now(), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM pending_tasks
			WHERE status = 'pending'
			ORDER BY priority DESC, created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`,
	).Scan(ctx, &task)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		// No rows or DB error — just return.
		return false
	}

	handler, ok := p.handlers[task.TaskType]
	if !ok {
		errMsg := fmt.Sprintf("unknown task type: %s", task.TaskType)
		p.markFailed(ctx, task.ID, errMsg)
		return true
	}

	// Recover from panics in handlers.
	func() {
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("panic: %v", r)
				slog.Error("worker: handler panicked", "task_id", task.ID, "task_type", task.TaskType, "panic", r)
				p.markFailed(ctx, task.ID, errMsg)
			}
		}()

		if herr := handler(ctx, &task); herr != nil {
			p.markFailed(ctx, task.ID, herr.Error())
		} else {
			p.markDone(ctx, task.ID)
		}
	}()

	return true
}

func (p *Pool) markDone(ctx context.Context, taskID string) {
	_, err := p.db.NewRaw(
		`UPDATE pending_tasks SET status = 'done', done_at = now() WHERE id = ?`, taskID,
	).Exec(ctx)
	if err != nil {
		slog.Error("worker: failed to mark task done", "task_id", taskID, "err", err)
	}
}

func (p *Pool) markFailed(ctx context.Context, taskID string, errMsg string) {
	_, err := p.db.NewRaw(
		`UPDATE pending_tasks SET status = 'failed', last_error = ?, done_at = now() WHERE id = ?`,
		errMsg, taskID,
	).Exec(ctx)
	if err != nil {
		slog.Error("worker: failed to mark task failed", "task_id", taskID, "err", err)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/worker/... -run TestPool_SubmitAndProcess -v -count=1`
Expected: PASS

- [ ] **Step 5: Add remaining pool tests**

Append to `internal/worker/pool_test.go`:

```go
func TestPool_UnknownHandler(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	// Register nothing.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)

	err := pool.Submit(context.Background(), "nonexistent", nil, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Wait for processing.
	time.Sleep(3 * time.Second)

	var task models.PendingTask
	err = db.NewSelect().Model(&task).Where("task_type = ?", "nonexistent").Scan(context.Background())
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if task.Status != "failed" {
		t.Fatalf("expected status=failed, got %s", task.Status)
	}
	if task.LastError == nil || !contains(*task.LastError, "unknown task type") {
		t.Fatalf("expected 'unknown task type' error, got %v", task.LastError)
	}

	pool.Shutdown()
}

func TestPool_HandlerError(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)

	done := make(chan struct{})
	pool.Register("fail_task", func(ctx context.Context, task *models.PendingTask) error {
		defer close(done)
		return fmt.Errorf("something went wrong")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)

	err := pool.Submit(context.Background(), "fail_task", nil, 0)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
	// Give the DB update a moment.
	time.Sleep(500 * time.Millisecond)

	var task models.PendingTask
	err = db.NewSelect().Model(&task).Where("task_type = ?", "fail_task").Scan(context.Background())
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if task.Status != "failed" {
		t.Fatalf("expected status=failed, got %s", task.Status)
	}
	if task.LastError == nil || !contains(*task.LastError, "something went wrong") {
		t.Fatalf("expected error message, got %v", task.LastError)
	}

	pool.Shutdown()
}

func TestPool_PriorityOrdering(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)

	var order []string
	var mu sync.Mutex
	allDone := make(chan struct{})

	pool.Register("ordered", func(ctx context.Context, task *models.PendingTask) error {
		mu.Lock()
		order = append(order, string(task.Payload))
		if len(order) == 2 {
			close(allDone)
		}
		mu.Unlock()
		return nil
	})

	// Submit low priority first, then high priority — both before starting the pool.
	if err := pool.Submit(context.Background(), "ordered", "low", 0); err != nil {
		t.Fatalf("submit low: %v", err)
	}
	if err := pool.Submit(context.Background(), "ordered", "high", 10); err != nil {
		t.Fatalf("submit high: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 1)

	select {
	case <-allDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(order))
	}
	// High priority (payload "high") should be processed first.
	if !contains(order[0], "high") {
		t.Fatalf("expected high priority first, got order: %v", order)
	}

	pool.Shutdown()
}

func TestPool_ConcurrentClaim(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)

	var count atomic.Int32
	done := make(chan struct{}, 1)
	pool.Register("once", func(ctx context.Context, task *models.PendingTask) error {
		count.Add(1)
		time.Sleep(100 * time.Millisecond) // Hold the task briefly.
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	})

	if err := pool.Submit(context.Background(), "once", nil, 0); err != nil {
		t.Fatalf("submit: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx, 4) // 4 workers competing for 1 task.

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
	time.Sleep(1 * time.Second) // Let other workers attempt claims.

	if c := count.Load(); c != 1 {
		t.Fatalf("expected exactly 1 execution, got %d", c)
	}

	pool.Shutdown()
}

func TestPool_GracefulShutdown(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)

	started := make(chan struct{})
	pool.Register("slow", func(ctx context.Context, task *models.PendingTask) error {
		close(started)
		time.Sleep(2 * time.Second)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx, 1)

	if err := pool.Submit(context.Background(), "slow", nil, 0); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Wait for the handler to start.
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	// Cancel and shutdown — should wait for the slow task.
	cancel()
	pool.Shutdown()

	// Verify the task completed despite shutdown.
	var task models.PendingTask
	err := db.NewSelect().Model(&task).Where("task_type = ?", "slow").Scan(context.Background())
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if task.Status != "done" {
		t.Fatalf("expected status=done after graceful shutdown, got %s", task.Status)
	}
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

Note: add `"fmt"` and `"sync"` to the imports at the top of the test file.

- [ ] **Step 6: Run all pool tests**

Run: `go test ./internal/worker/... -v -count=1`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/worker/
git commit -m "feat: add worker pool with task claiming and handler dispatch"
```

---

## Task 4: Jobs API Handler

**Files:**
- Create: `internal/api/jobs.go`
- Create: `internal/api/jobs_test.go`

This is a large handler. We'll build it incrementally.

- [ ] **Step 1: Write jobs_test.go with a test for listing jobs**

Create `internal/api/jobs_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
)

// insertJob inserts a job row directly for testing.
func insertJob(t *testing.T, db *bun.DB, id, userID, jobType, source, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
		 VALUES (?, ?, ?, ?, ?, 'high', now())`,
		id, userID, jobType, source, status,
	)
	if err != nil {
		t.Fatalf("insertJob: %v", err)
	}
}

// insertJobItem inserts a job_item row directly for testing.
func insertJobItem(t *testing.T, db *bun.DB, id, jobID, userID, itemKey, sourceTitle, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())`,
		id, jobID, userID, itemKey, sourceTitle, status,
	)
	if err != nil {
		t.Fatalf("insertJobItem: %v", err)
	}
}

// newTestEchoWithPool returns an Echo instance wired with a real db, ready migrator, and worker pool.
func newTestEchoWithPool(t *testing.T, db *bun.DB) (interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, *worker.Pool) {
	t.Helper()
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)
	return e, pool
}

func TestListJobs(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "listjobs")
	insertJob(t, db, "job-1", userID, "sync", "steam", "completed")
	insertJob(t, db, "job-2", userID, "import", "csv", "pending")

	rec := getAuth(t, e, "/api/jobs?per_page=10", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items, ok := resp["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", resp["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(items))
	}
}

func TestGetJob(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "getjob")
	insertJob(t, db, "job-get-1", userID, "sync", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/job-get-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != "job-get-1" {
		t.Fatalf("expected id=job-get-1, got %v", resp["id"])
	}
}

func TestGetJob_WrongOwner(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID1, _ := setupTagUser(t, db, e, "owner1")
	_, token2 := setupTagUser(t, db, e, "owner2")
	insertJob(t, db, "job-other", userID1, "sync", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/job-other", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelJob(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "cancel")
	insertJob(t, db, "job-cancel-1", userID, "sync", "steam", "processing")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancel-1/cancel", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed.
	var status string
	err := db.QueryRowContext(context.Background(),
		"SELECT status FROM jobs WHERE id = 'job-cancel-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "cancelled" {
		t.Fatalf("expected status=cancelled, got %s", status)
	}
}

func TestCancelJob_AlreadyTerminal(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "cancterm")
	insertJob(t, db, "job-cancterm", userID, "sync", "steam", "completed")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancterm/cancel", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for terminal job, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteJob(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "deljob")
	insertJob(t, db, "job-del-1", userID, "sync", "steam", "completed")

	rec := deleteAuth(t, e, "/api/jobs/job-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteJob_ActiveReturns409(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "delactive")
	insertJob(t, db, "job-del-active", userID, "sync", "steam", "processing")

	rec := deleteAuth(t, e, "/api/jobs/job-del-active", token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for active job, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestJobsSummary(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "summary")
	insertJob(t, db, "job-sum-1", userID, "sync", "steam", "processing")
	insertJob(t, db, "job-sum-2", userID, "sync", "steam", "failed")

	rec := getAuth(t, e, "/api/jobs/summary", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["running"] != float64(1) {
		t.Fatalf("expected running=1, got %v", resp["running"])
	}
	if resp["failed"] != float64(1) {
		t.Fatalf("expected failed=1, got %v", resp["failed"])
	}
}
```

Note: The test file uses `newTestEchoPool` which we'll define when we update the router. For now this won't compile — that's expected.

- [ ] **Step 2: Create jobs.go with all endpoints**

Create `internal/api/jobs.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// JobsHandler handles job management endpoints.
type JobsHandler struct {
	db   *bun.DB
	pool *worker.Pool
}

// NewJobsHandler returns a new JobsHandler.
func NewJobsHandler(db *bun.DB, pool *worker.Pool) *JobsHandler {
	return &JobsHandler{db: db, pool: pool}
}

// HandleListJobs handles GET /api/jobs.
func (h *JobsHandler) HandleListJobs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	q := h.db.NewSelect().TableExpr("jobs").Where("user_id = ?", userID)

	if v := c.QueryParam("job_type"); v != "" {
		q = q.Where("job_type = ?", v)
	}
	if v := c.QueryParam("source"); v != "" {
		q = q.Where("source = ?", v)
	}
	if v := c.QueryParam("status"); v != "" {
		q = q.Where("status = ?", v)
	}

	sortBy := c.QueryParam("sort_by")
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := c.QueryParam("sort_order")
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	// Whitelist sort columns.
	allowedSorts := map[string]bool{"created_at": true, "started_at": true, "completed_at": true, "job_type": true, "status": true}
	if !allowedSorts[sortBy] {
		sortBy = "created_at"
	}
	q = q.OrderExpr(fmt.Sprintf("%s %s", sortBy, sortOrder))

	total, err := q.Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count jobs")
	}

	offset := (page - 1) * perPage
	var jobs []models.Job
	err = q.Offset(offset).Limit(perPage).Scan(context.Background(), &jobs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list jobs")
	}
	if jobs == nil {
		jobs = []models.Job{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return c.JSON(http.StatusOK, map[string]any{
		"items":       jobs,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// HandleGetJob handles GET /api/jobs/:id.
func (h *JobsHandler) HandleGetJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")
	var job models.Job
	err := h.db.NewSelect().Model(&job).Where("id = ? AND user_id = ?", jobID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	// Item count breakdowns.
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	err = h.db.NewRaw(
		`SELECT status, COUNT(*)::int AS count FROM job_items WHERE job_id = ? GROUP BY status`, jobID,
	).Scan(context.Background(), &counts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get item counts")
	}

	itemCounts := make(map[string]int)
	for _, sc := range counts {
		itemCounts[sc.Status] = sc.Count
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job":              job,
		"duration_seconds": job.DurationSeconds(),
		"item_counts":      itemCounts,
	})
}

// HandleGetJobItems handles GET /api/jobs/:id/items.
func (h *JobsHandler) HandleGetJobItems(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	// Verify job ownership.
	exists, err := h.db.NewSelect().TableExpr("jobs").Where("id = ? AND user_id = ?", jobID, userID).Exists(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check job")
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	q := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID)
	if v := c.QueryParam("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	q = q.Order("created_at ASC")

	total, err := q.Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count items")
	}

	offset := (page - 1) * perPage
	var items []models.JobItem
	err = q.Offset(offset).Limit(perPage).Scan(context.Background(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list items")
	}
	if items == nil {
		items = []models.JobItem{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return c.JSON(http.StatusOK, map[string]any{
		"items":       items,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// HandleJobsSummary handles GET /api/jobs/summary.
func (h *JobsHandler) HandleJobsSummary(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var result struct {
		Running int `bun:"running"`
		Failed  int `bun:"failed"`
	}
	err := h.db.NewRaw(`
		SELECT
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing'))::int AS running,
			COUNT(*) FILTER (WHERE status = 'failed')::int AS failed
		FROM jobs WHERE user_id = ?`, userID,
	).Scan(context.Background(), &result)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get summary")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"running": result.Running,
		"failed":  result.Failed,
	})
}

// HandlePendingReviewCount handles GET /api/jobs/pending-review-count.
func (h *JobsHandler) HandlePendingReviewCount(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	count, err := h.db.NewSelect().TableExpr("job_items").
		Where("user_id = ? AND status = ?", userID, models.JobItemStatusPendingReview).
		Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count pending reviews")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"count": count,
	})
}

// HandleActiveJob handles GET /api/jobs/active/:job_type.
func (h *JobsHandler) HandleActiveJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobType := c.Param("job_type")

	// Try active job first.
	var job models.Job
	err := h.db.NewSelect().Model(&job).
		Where("user_id = ? AND job_type = ? AND status IN ('pending', 'processing')", userID, jobType).
		Order("created_at DESC").
		Limit(1).
		Scan(context.Background())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query active job")
	}
	if err == nil {
		return c.JSON(http.StatusOK, job)
	}

	// Fallback to most recent completed.
	err = h.db.NewSelect().Model(&job).
		Where("user_id = ? AND job_type = ?", userID, jobType).
		Order("created_at DESC").
		Limit(1).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, nil)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query recent job")
	}

	return c.JSON(http.StatusOK, job)
}

// HandleRecentJobs handles GET /api/jobs/recent/:source.
func (h *JobsHandler) HandleRecentJobs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	source := c.Param("source")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 20 {
		limit = 5
	}

	type jobWithItems struct {
		models.Job
		Items []models.JobItem `json:"items"`
	}

	var jobs []models.Job
	err := h.db.NewSelect().Model(&jobs).
		Where("user_id = ? AND source = ? AND status IN ('completed', 'failed')", userID, source).
		Order("created_at DESC").
		Limit(limit).
		Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query recent jobs")
	}
	if jobs == nil {
		jobs = []models.Job{}
	}

	// Build response with inline item summaries.
	result := make([]map[string]any, len(jobs))
	for i, job := range jobs {
		var items []models.JobItem
		_ = h.db.NewSelect().Model(&items).
			Where("job_id = ?", job.ID).
			Order("created_at ASC").
			Scan(context.Background())

		itemSummaries := make([]map[string]any, 0, len(items))
		for _, item := range items {
			summary := map[string]any{
				"source_title": item.SourceTitle,
				"status":       item.Status,
			}
			// Extract fields from result JSONB if present.
			if len(item.Result) > 2 { // not "{}"
				var resultMap map[string]any
				if json.Unmarshal(item.Result, &resultMap) == nil {
					if v, ok := resultMap["game_title"]; ok {
						summary["game_title"] = v
					}
					if v, ok := resultMap["is_new_addition"]; ok {
						summary["is_new_addition"] = v
					}
					if v, ok := resultMap["user_game_id"]; ok {
						summary["user_game_id"] = v
					}
				}
			}
			itemSummaries = append(itemSummaries, summary)
		}

		result[i] = map[string]any{
			"job":   job,
			"items": itemSummaries,
		}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleCancelJob handles POST /api/jobs/:id/cancel.
func (h *JobsHandler) HandleCancelJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewSelect().Model(&job).Where("id = ? AND user_id = ?", jobID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	if job.IsTerminal() {
		return echo.NewHTTPError(http.StatusConflict, "job is already terminal")
	}

	now := time.Now().UTC()
	_, err = h.db.NewRaw(
		`UPDATE jobs SET status = 'cancelled', completed_at = ? WHERE id = ?`, now, jobID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel job")
	}

	// Delete associated pending tasks.
	_, _ = h.db.NewRaw(
		`DELETE FROM pending_tasks WHERE status = 'pending' AND payload->>'job_id' = ?`, jobID,
	).Exec(context.Background())

	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
}

// HandleDeleteJob handles DELETE /api/jobs/:id.
func (h *JobsHandler) HandleDeleteJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewSelect().Model(&job).Where("id = ? AND user_id = ?", jobID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	if job.IsActive() {
		return echo.NewHTTPError(http.StatusConflict, "cannot delete an active job")
	}

	_, err = h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete job")
	}

	return c.NoContent(http.StatusNoContent)
}

// retryTaskType maps job_type to the task type for pool.Submit.
func retryTaskType(jobType string) string {
	switch jobType {
	case models.JobTypeSync:
		return "process_sync_item"
	case models.JobTypeMetadataRefresh:
		return "metadata_refresh_process"
	default:
		return "process_import_item"
	}
}

// HandleRetryFailed handles POST /api/jobs/:id/retry-failed.
func (h *JobsHandler) HandleRetryFailed(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewSelect().Model(&job).Where("id = ? AND user_id = ?", jobID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	// Reset failed items to pending.
	_, err = h.db.NewRaw(
		`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL
		 WHERE job_id = ? AND status = 'failed'`, jobID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset items")
	}

	// Get the failed items to re-queue.
	var items []models.JobItem
	err = h.db.NewSelect().Model(&items).Where("job_id = ? AND status = 'pending'", jobID).Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query items")
	}

	taskType := retryTaskType(job.JobType)
	for _, item := range items {
		payload := map[string]string{
			"job_id":      jobID,
			"job_item_id": item.ID,
		}
		if submitErr := h.pool.Submit(context.Background(), taskType, payload, 0); submitErr != nil {
			// Log but don't fail the whole operation.
			_ = submitErr
		}
	}

	// Reset job status to processing.
	_, _ = h.db.NewRaw(
		`UPDATE jobs SET status = 'processing' WHERE id = ?`, jobID,
	).Exec(context.Background())

	return c.JSON(http.StatusOK, map[string]any{
		"status":      "retrying",
		"items_count": len(items),
	})
}
```

Add `"encoding/json"` to the imports.

- [ ] **Step 3: Update router.go to accept pool and register job routes**

In `internal/api/router.go`:

1. Update the `New` function signature to accept `*worker.Pool`:

```go
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, pool ...*worker.Pool) *echo.Echo {
```

Use variadic so existing callers (tests without pool) don't break:

```go
	var wp *worker.Pool
	if len(pool) > 0 {
		wp = pool[0]
	}
	registerRoutes(e, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient, wp)
```

2. Update `registerRoutes` to accept the pool and register routes inside the `if db != nil` block:

```go
func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, pool *worker.Pool) {
```

Add at the end of the `if db != nil` block, before the closing brace:

```go
		// Jobs routes (all JWT-protected)
		jh := NewJobsHandler(db, pool)
		jobsGroup := e.Group("/api/jobs", auth.JWTMiddleware(cfg.SecretKey, db))
		jobsGroup.GET("", jh.HandleListJobs)
		jobsGroup.GET("/summary", jh.HandleJobsSummary)
		jobsGroup.GET("/pending-review-count", jh.HandlePendingReviewCount)
		jobsGroup.GET("/active/:job_type", jh.HandleActiveJob)
		jobsGroup.GET("/recent/:source", jh.HandleRecentJobs)
		jobsGroup.GET("/:id", jh.HandleGetJob)
		jobsGroup.GET("/:id/items", jh.HandleGetJobItems)
		jobsGroup.POST("/:id/cancel", jh.HandleCancelJob)
		jobsGroup.DELETE("/:id", jh.HandleDeleteJob)
		jobsGroup.POST("/:id/retry-failed", jh.HandleRetryFailed)

		// Job Items routes (all JWT-protected)
		jih := NewJobItemsHandler(db, pool)
		jobItemsGroup := e.Group("/api/job-items", auth.JWTMiddleware(cfg.SecretKey, db))
		jobItemsGroup.GET("/:id", jih.HandleGetJobItem)
		jobItemsGroup.POST("/:id/resolve", jih.HandleResolveItem)
		jobItemsGroup.POST("/:id/skip", jih.HandleSkipItem)
		jobItemsGroup.POST("/:id/retry", jih.HandleRetryItem)
```

3. Add import for `worker` package:

```go
	"github.com/drzero42/nexorious-go/internal/worker"
```

- [ ] **Step 4: Add `newTestEchoPool` helper to auth_test.go**

Add to `internal/api/auth_test.go` (near `newTestEcho`):

```go
// newTestEchoPool returns an Echo instance wired with a real db, ready migrator, and worker pool.
func newTestEchoPool(t *testing.T, db *bun.DB, cfg *config.Config, pool *worker.Pool) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(cfg, m, db, "", nil, pool)
}
```

Add `"github.com/drzero42/nexorious-go/internal/worker"` to the imports.

- [ ] **Step 5: Verify it compiles**

Run: `go build ./...`
Expected: FAIL — `NewJobItemsHandler` doesn't exist yet. We'll create it in the next task. For now, stub it.

Create a minimal `internal/api/job_items.go` stub:

```go
package api

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
)

// JobItemsHandler handles job item endpoints.
type JobItemsHandler struct {
	db   *bun.DB
	pool *worker.Pool
}

// NewJobItemsHandler returns a new JobItemsHandler.
func NewJobItemsHandler(db *bun.DB, pool *worker.Pool) *JobItemsHandler {
	return &JobItemsHandler{db: db, pool: pool}
}

func (h *JobItemsHandler) HandleGetJobItem(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}

func (h *JobItemsHandler) HandleResolveItem(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}

func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}

func (h *JobItemsHandler) HandleRetryItem(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not implemented")
}
```

- [ ] **Step 6: Verify it compiles**

Run: `go build ./...`
Expected: success

- [ ] **Step 7: Run job tests**

Run: `go test ./internal/api/... -run "TestListJobs|TestGetJob|TestCancelJob|TestDeleteJob|TestJobsSummary" -v -count=1`
Expected: ALL PASS

- [ ] **Step 8: Run all existing tests to check for regressions**

Run: `go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add internal/api/jobs.go internal/api/job_items.go internal/api/jobs_test.go internal/api/router.go internal/api/auth_test.go
git commit -m "feat: add jobs API handler with list, get, cancel, delete, summary endpoints"
```

---

## Task 5: Job Items API Handler

**Files:**
- Modify: `internal/api/job_items.go` (replace stub)
- Create: `internal/api/job_items_test.go`

- [ ] **Step 1: Write job_items_test.go**

Create `internal/api/job_items_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/uptrace/bun"
)

func TestGetJobItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "getitem")
	insertJob(t, db, "job-item-get", userID, "sync", "steam", "processing")
	insertJobItem(t, db, "ji-1", "job-item-get", userID, "app-123", "Test Game", "pending_review")

	rec := getAuth(t, e, "/api/job-items/ji-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != "ji-1" {
		t.Fatalf("expected id=ji-1, got %v", resp["id"])
	}
}

func TestGetJobItem_WrongOwner(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID1, _ := setupTagUser(t, db, e, "itemown1")
	_, token2 := setupTagUser(t, db, e, "itemown2")
	insertJob(t, db, "job-own-item", userID1, "sync", "steam", "processing")
	insertJobItem(t, db, "ji-own", "job-own-item", userID1, "app-456", "Other Game", "pending")

	rec := getAuth(t, e, "/api/job-items/ji-own", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong owner, got %d", rec.Code)
	}
}

func TestResolveItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "resolve")
	insertJob(t, db, "job-resolve", userID, "sync", "steam", "processing")
	insertJobItem(t, db, "ji-resolve", "job-resolve", userID, "app-789", "Resolve Game", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/ji-resolve/resolve", map[string]any{
		"igdb_id": 12345,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the item was updated.
	var resolvedID *int
	var status string
	err := db.QueryRowContext(context.Background(),
		"SELECT resolved_igdb_id, status FROM job_items WHERE id = 'ji-resolve'",
	).Scan(&resolvedID, &status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resolvedID == nil || *resolvedID != 12345 {
		t.Fatalf("expected resolved_igdb_id=12345, got %v", resolvedID)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending after resolve, got %s", status)
	}
}

func TestResolveItem_NotPendingReview(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "resolvebad")
	insertJob(t, db, "job-resolvebad", userID, "sync", "steam", "processing")
	insertJobItem(t, db, "ji-resolvebad", "job-resolvebad", userID, "app-000", "Bad Game", "completed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-resolvebad/resolve", map[string]any{
		"igdb_id": 999,
	}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkipItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "skip")
	insertJob(t, db, "job-skip", userID, "sync", "steam", "processing")
	insertJobItem(t, db, "ji-skip", "job-skip", userID, "app-skip", "Skip Game", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/ji-skip/skip", map[string]any{
		"reason": "not interested",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	err := db.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-skip'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "skipped" {
		t.Fatalf("expected status=skipped, got %s", status)
	}
}

func TestRetryItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "retryitem")
	insertJob(t, db, "job-retry-item", userID, "import", "csv", "failed")
	insertJobItem(t, db, "ji-retry", "job-retry-item", userID, "app-retry", "Retry Game", "failed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry/retry", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	err := db.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-retry'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending after retry, got %s", status)
	}
}

func TestRetryItem_NotFailed(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "retrybad")
	insertJob(t, db, "job-retrybad", userID, "import", "csv", "processing")
	insertJobItem(t, db, "ji-retrybad", "job-retrybad", userID, "app-rb", "RB Game", "completed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retrybad/retry", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Replace the job_items.go stub with the full implementation**

Replace `internal/api/job_items.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// JobItemsHandler handles job item endpoints.
type JobItemsHandler struct {
	db   *bun.DB
	pool *worker.Pool
}

// NewJobItemsHandler returns a new JobItemsHandler.
func NewJobItemsHandler(db *bun.DB, pool *worker.Pool) *JobItemsHandler {
	return &JobItemsHandler{db: db, pool: pool}
}

// HandleGetJobItem handles GET /api/job-items/:id.
func (h *JobItemsHandler) HandleGetJobItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")
	var item models.JobItem
	err := h.db.NewSelect().Model(&item).Where("id = ? AND user_id = ?", itemID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get item")
	}

	return c.JSON(http.StatusOK, item)
}

// HandleResolveItem handles POST /api/job-items/:id/resolve.
func (h *JobItemsHandler) HandleResolveItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var req struct {
		IGDBID int `json:"igdb_id"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var item models.JobItem
	err := h.db.NewSelect().Model(&item).Where("id = ? AND user_id = ?", itemID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get item")
	}

	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	now := time.Now().UTC()
	_, err = h.db.NewRaw(
		`UPDATE job_items SET resolved_igdb_id = ?, resolved_at = ?, status = 'pending' WHERE id = ?`,
		req.IGDBID, now, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve item")
	}

	// Re-queue for processing — look up parent job's type for task type mapping.
	var jobType string
	err = h.db.NewRaw(`SELECT job_type FROM jobs WHERE id = ?`, item.JobID).Scan(context.Background(), &jobType)
	if err == nil {
		taskType := retryTaskType(jobType)
		payload := map[string]string{
			"job_id":      item.JobID,
			"job_item_id": item.ID,
		}
		_ = h.pool.Submit(context.Background(), taskType, payload, 0)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "resolved"})
}

// HandleSkipItem handles POST /api/job-items/:id/skip.
func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var req struct {
		Reason string `json:"reason"`
	}
	// Bind is optional — body may be empty.
	_ = c.Bind(&req)

	var item models.JobItem
	err := h.db.NewSelect().Model(&item).Where("id = ? AND user_id = ?", itemID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get item")
	}

	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	_, err = h.db.NewRaw(
		`UPDATE job_items SET status = 'skipped' WHERE id = ?`, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip item")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "skipped"})
}

// HandleRetryItem handles POST /api/job-items/:id/retry.
func (h *JobItemsHandler) HandleRetryItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var item models.JobItem
	err := h.db.NewSelect().Model(&item).Where("id = ? AND user_id = ?", itemID, userID).Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get item")
	}

	if item.Status != models.JobItemStatusFailed {
		return echo.NewHTTPError(http.StatusConflict, "item is not failed")
	}

	_, err = h.db.NewRaw(
		`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL WHERE id = ?`, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset item")
	}

	// Re-queue.
	var jobType string
	err = h.db.NewRaw(`SELECT job_type FROM jobs WHERE id = ?`, item.JobID).Scan(context.Background(), &jobType)
	if err == nil {
		taskType := retryTaskType(jobType)
		payload := map[string]string{
			"job_id":      item.JobID,
			"job_item_id": item.ID,
		}
		_ = h.pool.Submit(context.Background(), taskType, payload, 0)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "retrying"})
}
```

- [ ] **Step 3: Run job item tests**

Run: `go test ./internal/api/... -run "TestGetJobItem|TestResolveItem|TestSkipItem|TestRetryItem" -v -count=1`
Expected: ALL PASS

- [ ] **Step 4: Run all tests for regressions**

Run: `go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/job_items.go internal/api/job_items_test.go
git commit -m "feat: add job items API handler with get, resolve, skip, retry endpoints"
```

---

## Task 6: Scheduler with Cleanup Jobs

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Add gocron dependency**

Run: `go get github.com/go-co-op/gocron/v2`

- [ ] **Step 2: Write scheduler_test.go**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/scheduler"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
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
		t.Fatalf("failed to start postgres: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func TestCleanupOldJobs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Insert a user for FK.
	_, _ = db.ExecContext(ctx,
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES ('u-sched', 'scheduser', 'hash', true, false)")

	// Old completed job (31 days ago).
	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at, created_at)
		 VALUES ('job-old', 'u-sched', 'sync', 'steam', 'completed', now() - interval '31 days', now() - interval '31 days')`)

	// Recent completed job (1 day ago).
	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, completed_at, created_at)
		 VALUES ('job-recent', 'u-sched', 'sync', 'steam', 'completed', now() - interval '1 day', now() - interval '1 day')`)

	// Active job (should not be deleted).
	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, created_at)
		 VALUES ('job-active', 'u-sched', 'sync', 'steam', 'processing', now())`)

	scheduler.CleanupOldJobs(ctx, db)

	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 jobs remaining (recent + active), got %d", count)
	}

	// Verify the old one was deleted.
	var exists bool
	_ = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE id = 'job-old')").Scan(&exists)
	if exists {
		t.Fatal("expected old job to be deleted")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, _ = db.ExecContext(ctx,
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES ('u-sess', 'sessuser', 'hash', true, false)")

	// Expired session.
	_, _ = db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES ('sess-exp', 'u-sess', 'h1', 'h2', now() - interval '1 hour')`)

	// Valid session.
	_, _ = db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES ('sess-valid', 'u-sess', 'h3', 'h4', now() + interval '30 days')`)

	scheduler.CleanupExpiredSessions(ctx, db)

	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_sessions").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 session remaining, got %d", count)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/scheduler/... -v -count=1`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 4: Create scheduler.go**

Create `internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
)

// Scheduler manages recurring cleanup jobs via gocron.
type Scheduler struct {
	db        *bun.DB
	pool      *worker.Pool
	scheduler gocron.Scheduler
}

// NewScheduler creates a new scheduler.
func NewScheduler(db *bun.DB, pool *worker.Pool) *Scheduler {
	return &Scheduler{db: db, pool: pool}
}

// Start registers all cleanup jobs and starts the gocron scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	sched, err := gocron.NewScheduler()
	if err != nil {
		return err
	}
	s.scheduler = sched

	// Cleanup old job results — daily at 3:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 3 * * *", false),
		gocron.NewTask(func() {
			CleanupOldJobs(ctx, s.db)
		}),
	)

	// Cleanup exports — daily at 4:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 4 * * *", false),
		gocron.NewTask(func() {
			CleanupExports(ctx, s.db)
		}),
	)

	// Cleanup unreferenced games — daily at 5:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 5 * * *", false),
		gocron.NewTask(func() {
			CleanupUnreferencedGames(ctx, s.db)
		}),
	)

	// Cleanup expired sessions — every 30 minutes.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("*/30 * * * *", false),
		gocron.NewTask(func() {
			CleanupExpiredSessions(ctx, s.db)
		}),
	)

	s.scheduler.Start()
	slog.Info("scheduler started")
	return nil
}

// Stop shuts down the gocron scheduler.
func (s *Scheduler) Stop() {
	if s.scheduler != nil {
		_ = s.scheduler.Shutdown()
		slog.Info("scheduler stopped")
	}
}

// CleanupOldJobs deletes terminal jobs older than 30 days and their items (CASCADE).
func CleanupOldJobs(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM jobs
		 WHERE status IN ('completed', 'failed', 'cancelled')
		   AND completed_at < now() - interval '30 days'`,
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup: failed to delete old jobs", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted old jobs", "count", rows)
	}
}

// CleanupExports deletes expired export files and their job rows.
// Placeholder — export jobs use file_path; full implementation comes in the export consumer spec.
func CleanupExports(ctx context.Context, db *bun.DB) {
	// Export cleanup will be implemented when the export consumer is built.
	// For now, this is a no-op that logs it ran.
	slog.Debug("cleanup: export cleanup ran (no-op until export consumer is implemented)")
}

// CleanupUnreferencedGames deletes games with no user_games rows.
func CleanupUnreferencedGames(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM games
		 WHERE id NOT IN (SELECT DISTINCT game_id FROM user_games)`,
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup: failed to delete unreferenced games", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted unreferenced games", "count", rows)
	}
}

// CleanupExpiredSessions deletes expired user_sessions rows.
func CleanupExpiredSessions(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM user_sessions WHERE expires_at < now()`,
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup: failed to delete expired sessions", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted expired sessions", "count", rows)
	}
}
```

- [ ] **Step 5: Run scheduler tests**

Run: `go test ./internal/scheduler/... -v -count=1`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/scheduler/
git commit -m "feat: add gocron scheduler with cleanup jobs for jobs, sessions, and games"
```

---

## Task 7: Startup & Shutdown Wiring

**Files:**
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Update main.go to wire pool and scheduler**

In `cmd/nexorious/main.go`, add imports:

```go
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/worker"
```

Replace the worker/scheduler gate goroutine (the `go func(ctx context.Context) {` block near line 199) with:

```go
	// Worker pool — created early so the Echo server can reference it.
	pool := worker.NewPool(db)
	// Register handlers here when consumer specs are implemented:
	// pool.Register("process_sync_item", syncHandler)
	// pool.Register("process_import_item", importHandler)
	// pool.Register("metadata_refresh_process", metadataHandler)

	var sched *scheduler.Scheduler

	// Worker/scheduler gate — starts after Ready && !NeedsSetup.
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
				pool.Start(ctx, cfg.WorkerCount)
				sched = scheduler.NewScheduler(db, pool)
				if err := sched.Start(ctx); err != nil {
					slog.Error("failed to start scheduler", "err", err)
				}
				slog.Info("app ready — workers and scheduler started")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}(shutdownCtx)
```

Update the `api.New` call to pass the pool:

```go
	e := api.New(cfg, migrator, db, resolvedDatabaseURL, igdbClient, pool)
```

Add graceful shutdown after the `sc.Start` call, before the final log:

```go
	if err := sc.Start(shutdownCtx, e); err != nil {
		slog.Info("server stopped", "err", err)
	}

	// Graceful shutdown sequence.
	if sched != nil {
		sched.Stop()
	}
	pool.Shutdown()

	slog.Info("shutdown complete")
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/nexorious/...`
Expected: success

- [ ] **Step 3: Run all tests**

Run: `go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat: wire worker pool and scheduler into startup/shutdown sequence"
```

---

## Task 8: Slumber Collection Requests

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add jobs and job-items request folders to slumber.yaml**

Add the following under the top-level `requests:` key (after existing request groups, maintaining alphabetical order — `jobs` goes after `health` or `games`):

```yaml
  jobs:
    name: Jobs
    requests:
      list_jobs:
        name: List Jobs
        method: GET
        url: "{{base_url}}/api/jobs?per_page=10"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      jobs_summary:
        name: Jobs Summary
        method: GET
        url: "{{base_url}}/api/jobs/summary"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      pending_review_count:
        name: Pending Review Count
        method: GET
        url: "{{base_url}}/api/jobs/pending-review-count"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      active_job:
        name: Active Job
        method: GET
        url: "{{base_url}}/api/jobs/active/sync"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      recent_jobs:
        name: Recent Jobs
        method: GET
        url: "{{base_url}}/api/jobs/recent/steam?limit=5"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      get_job:
        name: Get Job
        method: GET
        url: "{{base_url}}/api/jobs/REPLACE_WITH_JOB_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      get_job_items:
        name: Get Job Items
        method: GET
        url: "{{base_url}}/api/jobs/REPLACE_WITH_JOB_ID/items?per_page=10"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      cancel_job:
        name: Cancel Job
        method: POST
        url: "{{base_url}}/api/jobs/REPLACE_WITH_JOB_ID/cancel"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      delete_job:
        name: Delete Job
        method: DELETE
        url: "{{base_url}}/api/jobs/REPLACE_WITH_JOB_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      retry_failed:
        name: Retry Failed Items
        method: POST
        url: "{{base_url}}/api/jobs/REPLACE_WITH_JOB_ID/retry-failed"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

  job_items:
    name: Job Items
    requests:
      get_job_item:
        name: Get Job Item
        method: GET
        url: "{{base_url}}/api/job-items/REPLACE_WITH_ITEM_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      resolve_item:
        name: Resolve Item
        method: POST
        url: "{{base_url}}/api/job-items/REPLACE_WITH_ITEM_ID/resolve"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            igdb_id: 12345

      skip_item:
        name: Skip Item
        method: POST
        url: "{{base_url}}/api/job-items/REPLACE_WITH_ITEM_ID/skip"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            reason: "not interested"

      retry_item:
        name: Retry Item
        method: POST
        url: "{{base_url}}/api/job-items/REPLACE_WITH_ITEM_ID/retry"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
```

- [ ] **Step 2: Verify the collection loads**

Run: `slumber show collection`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "feat: add slumber requests for jobs and job-items endpoints"
```

---

## Task 9: Final Verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -count=1`
Expected: ALL PASS

- [ ] **Step 2: Run the linter**

Run: `golangci-lint run`
Expected: no errors

- [ ] **Step 3: Build the binary**

Run: `make build`
Expected: success

- [ ] **Step 4: Verify all spec checklist items are addressed**

Cross-reference with the spec's checklist:
- ✅ `pending_tasks`, `jobs`, `job_items` tables in migration (already existed, updated to match spec)
- ✅ DROP statements in down migration (already existed)
- ✅ `internal/db/models/jobs.go` with Bun structs and constants
- ✅ `internal/worker/pool.go` with Pool type, registry, worker loop
- ✅ `internal/worker/pool_test.go`
- ✅ `internal/api/jobs.go` with all job endpoints
- ✅ `internal/api/jobs_test.go`
- ✅ `internal/api/job_items.go` with all job-item endpoints
- ✅ `internal/api/job_items_test.go`
- ✅ `internal/scheduler/scheduler.go` with cleanup jobs
- ✅ `internal/scheduler/scheduler_test.go`
- ✅ `internal/api/router.go` updated with job and job-item routes
- ✅ `cmd/nexorious/main.go` updated for pool/scheduler startup and shutdown
- ✅ Slumber requests for all new endpoints
