package platformresolution_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

// testDB is the single shared *bun.DB for all platformresolution_test tests.
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
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

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
			games,
			user_games,
			user_game_platforms,
			tags,
			user_game_tags,
			external_games,
			user_sync_configs,
			jobs,
			job_items
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}

// insertTestUser inserts a minimal user row for testing.
func insertTestUser(t *testing.T, db *bun.DB, userID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO users (id, username, password_hash, is_active, is_admin)
		 VALUES (?, ?, 'testhash', true, false)`,
		userID, "user-"+userID,
	)
	if err != nil {
		t.Fatalf("insertTestUser %q: %v", userID, err)
	}
}

// insertTestExternalGame inserts a minimal external_game row and returns its ID.
func insertTestExternalGame(t *testing.T, db *bun.DB, userID, storefront, title string) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games
		     (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, ?, ?)`,
		id, userID, storefront, uuid.NewString(), title, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestExternalGame %q: %v", title, err)
	}
	return id
}

// insertTestExternalGamePlatform inserts a row in external_game_platforms for
// the given external game and platform slug.
func insertTestExternalGamePlatform(t *testing.T, db *bun.DB, externalGameID, platformSlug string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, ?, 0, now())`,
		uuid.NewString(), externalGameID, platformSlug,
	)
	if err != nil {
		t.Fatalf("insertTestExternalGamePlatform eg=%q platform=%q: %v", externalGameID, platformSlug, err)
	}
}
