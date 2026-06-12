package api_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/migrations"
)

// testDB is the single shared *bun.DB for all api_test tests.
var testDB *bun.DB

// testEncrypter is the shared Encrypter for all api_test tests.
var testEncrypter *crypto.Encrypter

// testConnStr is the connection string for the shared test container, used by
// helpers that need to build a pgx pool (e.g. for River clients in handler
// tests that exercise the enqueue paths).
var testConnStr string

func TestMain(m *testing.M) {
	// Password hashing at the production cost (12) dominates this package's
	// runtime — the suite creates and logs in hundreds of users. Tests don't
	// exercise the cost factor itself, so drop it to the cheapest setting.
	auth.BcryptCost = bcrypt.MinCost

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

	testEncrypter, err = crypto.NewEncrypter("test-db-encryption-key-32-bytes!!")
	if err != nil {
		fmt.Fprintf(os.Stderr, "crypto encrypter init: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// truncateAllTables truncates every user/application data table with RESTART
// IDENTITY CASCADE so each test starts with a clean slate.
// Reference/seed tables (platforms, storefronts, platform_storefronts,
// backup_config) are intentionally excluded — they are populated by migrations
// and tests rely on them being present.
func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		TRUNCATE TABLE
			users,
			user_sessions,
			api_keys,
			games,
			user_games,
			user_game_platforms,
			tags,
			user_game_tags,
			pools,
			pool_games,
			external_games,
			user_sync_configs,
			jobs,
			job_items,
			river_job,
			rate_limiter_tokens
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}
