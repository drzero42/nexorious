package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	nexdb "github.com/drzero42/nexorious/internal/db"
)

// TestDropIdleConns_AfterTerminateBackend is the #1284 regression test. A
// restore terminates every backend, including the server pool's idle
// connections. Without a flush, the first query through the pool lands on a
// dead connection and fails with 57P01 (which left a setup-zone restore stuck
// in needs_setup). After DropIdleConns the pool dials fresh connections.
func TestDropIdleConns_AfterTerminateBackend(t *testing.T) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	open := func() *bun.DB {
		d := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn))), pgdialect.New())
		d.SetMaxOpenConns(1) // one connection, so the next query must reuse the idle one
		d.SetMaxIdleConns(nexdb.MaxIdleConns)
		t.Cleanup(func() { _ = d.Close() })
		return d
	}
	pool := open()
	admin := open()

	// terminatePool leaves pool holding one idle connection whose backend is dead.
	terminatePool := func() {
		t.Helper()
		var pid int
		if err := pool.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatalf("pool pid: %v", err)
		}
		var ok bool
		if err := admin.QueryRowContext(ctx, "SELECT pg_terminate_backend(?, 5000)", pid).Scan(&ok); err != nil || !ok {
			t.Fatalf("terminate backend %d: ok=%v err=%v", pid, ok, err)
		}
	}

	// Without the flush, the stale connection fails the next query.
	terminatePool()
	var one int
	err = pool.QueryRowContext(ctx, "SELECT 1").Scan(&one)
	if err == nil || !strings.Contains(err.Error(), "57P01") {
		t.Fatalf("query on terminated conn without flush: want 57P01 error, got %v", err)
	}

	// With the flush, the next query gets a fresh connection.
	terminatePool()
	nexdb.DropIdleConns(pool)
	if err := pool.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query after DropIdleConns: %v", err)
	}
	if got := pool.Stats().MaxIdleClosed; got == 0 {
		t.Errorf("MaxIdleClosed = 0; DropIdleConns closed no idle connection")
	}
}
