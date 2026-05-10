package worker_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
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

	pool.Start(t.Context(), 1)

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

func TestPool_UnknownHandler(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	// Register nothing.

	pool.Start(t.Context(), 1)

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
	if task.LastError == nil || !strings.Contains(*task.LastError, "unknown task type") {
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

	pool.Start(t.Context(), 1)

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
	if task.LastError == nil || !strings.Contains(*task.LastError, "something went wrong") {
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

	pool.Start(t.Context(), 1)

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
	if !strings.Contains(order[0], "high") {
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

	pool.Start(t.Context(), 4) // 4 workers competing for 1 task.

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
