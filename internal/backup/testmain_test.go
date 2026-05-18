package backup_test

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
	"github.com/riverqueue/river/rivermigrate"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

// testDB is the single shared *bun.DB for all backup_test tests.
var testDB *bun.DB

// testDSN is the connection string for the shared postgres container.
// It is needed by backup.NewService which passes it to pg_dump.
var testDSN string

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
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}
	testDSN = connStr

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	testDB = bun.NewDB(sqldb, pgdialect.New())
	defer func() { _ = testDB.Close() }()

	migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrator init: %v\n", err)
		os.Exit(1)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	riverMig, err := rivermigrate.New(riverdatabasesql.New(testDB.DB), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "river migrator init: %v\n", err)
		os.Exit(1)
	}
	if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		fmt.Fprintf(os.Stderr, "river migrate: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// truncateAllTables truncates every user/application data table with RESTART
// IDENTITY CASCADE so each test starts with a clean slate.
func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		TRUNCATE TABLE
			users,
			user_sessions,
			games,
			external_games,
			platforms,
			storefronts,
			platform_storefronts,
			tags,
			user_games,
			user_game_tags,
			user_game_platforms,
			jobs,
			job_items,
			river_job,
			backup_config,
			user_sync_configs,
			rate_limiter_tokens
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}
