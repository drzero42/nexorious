package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
)

// testDB is the shared bun.DB backed by a single postgres container for the
// entire cmd/nexorious test binary. Tests that touch the database must call
// truncateAllTables(t) at the start to get a clean slate.
var testDB *bun.DB

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
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	testDB = bun.NewDB(sqldb, pgdialect.New())
	defer func() { _ = testDB.Close() }()

	// Run migrations once so the schema is ready for all tests.
	migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrator init: %v\n", err)
		os.Exit(1)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// truncateAllTables resets every application table to an empty state so tests
// start from a known-clean database without restarting the container.
func truncateAllTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		"users",
		"user_sessions",
		"games",
		"external_games",
		"platforms",
		"storefronts",
		"platform_storefronts",
		"tags",
		"user_games",
		"user_game_tags",
		"user_game_platforms",
		"jobs",
		"job_items",
		"pending_tasks",
		"backup_config",
		"user_sync_configs",
		"rate_limiter_tokens",
	}
	for _, table := range tables {
		if _, err := testDB.ExecContext(ctx,
			fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table),
		); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}
