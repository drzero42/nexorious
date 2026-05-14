package ratelimit_test

import (
	"context"
	"database/sql"
	"sync"
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
	"github.com/drzero42/nexorious-go/internal/ratelimit"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
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

func TestPostgres_ConcurrentGoroutinesEachGetToken(t *testing.T) {
	db := setupTestDB(t)
	n := 5
	// Burst = n so all goroutines can get a token without waiting for refill.
	l := ratelimit.NewPostgres(db, "test-concurrent", 100, float64(n))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = l.Wait(ctx)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}

func TestPostgres_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	// Burst=1, rate=0.01/s so after consuming the first token there's essentially
	// no refill within the test window.
	l := ratelimit.NewPostgres(db, "test-cancel", 0.01, 1)

	ctx := context.Background()
	// Consume the single burst token.
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	// The next Wait should block; cancel it quickly.
	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := l.Wait(cancelCtx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// TestPostgres_ZeroRPSClamped verifies that rps<=0 is clamped to 1.
func TestPostgres_ZeroRPSClamped(t *testing.T) {
	db := setupTestDB(t)
	// rps=0 should be clamped to 1; burst=5 so first call succeeds.
	l := ratelimit.NewPostgres(db, "test-zero-rps", 0, 5)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPostgres_LowBurstClamped verifies that burst<1 is clamped to 1.
func TestPostgres_LowBurstClamped(t *testing.T) {
	db := setupTestDB(t)
	// burst=0.5 should be clamped to 1.
	l := ratelimit.NewPostgres(db, "test-low-burst", 10, 0.5)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPostgres_AlreadyCancelledContext verifies Wait returns immediately when context is already done.
func TestPostgres_AlreadyCancelledContext(t *testing.T) {
	db := setupTestDB(t)
	l := ratelimit.NewPostgres(db, "test-pre-cancel", 10, 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Wait
	err := l.Wait(ctx)
	if err == nil {
		t.Fatal("expected error from already-cancelled context, got nil")
	}
}
