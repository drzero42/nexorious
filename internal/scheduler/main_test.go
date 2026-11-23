package scheduler_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/riverqueue/river"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
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
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}
	testConnStr = connStr

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

func truncateAllTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testDB.NewRaw(`TRUNCATE TABLE
		users, user_sessions, games, external_games, platforms, storefronts,
		platform_storefronts, tags, user_games, user_game_tags, user_game_platforms,
		jobs, job_items, river_job, backup_config, user_sync_configs, rate_limiter_tokens,
		events
		RESTART IDENTITY CASCADE`).Exec(ctx)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

func insertUser(t *testing.T, ctx context.Context, _ *bun.DB) string {
	t.Helper()
	id := fmt.Sprintf("user-%d", time.Now().UnixNano())
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)`,
		id, id, "hash",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// newTestLC returns a lifecycle module with the registered kinds, inserting
// River jobs through an insert-only client on the shared test database.
func newTestLC(t *testing.T) *joblifecycle.Module {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(testDB.DB), &river.Config{})
	if err != nil {
		t.Fatalf("insert-only river client: %v", err)
	}
	lc := joblifecycle.New(testDB, rc)
	tasks.RegisterLifecycleKinds(lc)
	return lc
}
