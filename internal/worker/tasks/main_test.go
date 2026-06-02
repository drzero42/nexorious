package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
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

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/migrations"
)

// testDB is the shared database connection for all external package tests.
var testDB *bun.DB

// testEncrypter is the shared Encrypter for all tasks_test tests.
var testEncrypter *crypto.Encrypter

// testConnStr is the DSN of the shared test container, exposed for tests that
// need a pgxpool (e.g. to create a non-started River client).
var testConnStr string

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

	riverMig, err := rivermigrate.New(riverdatabasesql.New(testDB.DB), nil)
	if err != nil {
		_ = testDB.Close()
		_ = ctr.Terminate(ctx)
		panic("river migrator init: " + err.Error())
	}
	if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		_ = testDB.Close()
		_ = ctr.Terminate(ctx)
		panic("river migrate: " + err.Error())
	}

	var encErr error
	testEncrypter, encErr = crypto.NewEncrypter("test-db-encryption-key-32-bytes!!")
	if encErr != nil {
		_ = testDB.Close()
		_ = ctr.Terminate(ctx)
		panic("crypto encrypter init: " + encErr.Error())
	}

	code := m.Run()

	_ = testDB.Close()
	_ = ctr.Terminate(ctx)
	os.Exit(code)
}

// truncateAllTables resets all application tables between tests.
//
// Reference/seed tables (platforms, storefronts, platform_storefronts) are
// intentionally excluded — they are populated by migrations and are referenced
// by foreign keys (e.g. external_game_platforms.platform -> platforms.name), so
// wiping them would leave the schema unable to satisfy those FKs. This mirrors
// the convention in internal/api's truncateAllTables.
func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		TRUNCATE TABLE
			users, user_sessions, games, external_games, external_game_platforms,
			tags, user_games, user_game_tags, user_game_platforms,
			jobs, job_items, river_job, backup_config,
			user_sync_configs, rate_limiter_tokens,
			events, changes
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}

// ─── Shared test helpers ──────────────────────────────────────────────────────

func insertTestUser(t *testing.T, db *bun.DB, userID string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, true, false)",
		userID, "user_"+userID, string(hash),
	)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
}

func insertTestJob(t *testing.T, db *bun.DB, jobID, userID string, totalItems int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'import', 'nexorious', 'processing', 'normal', ?)`,
		jobID, userID, totalItems,
	)
	if err != nil {
		t.Fatalf("insertTestJob: %v", err)
	}
}

func insertTestJobItem(t *testing.T, db *bun.DB, jobID, userID string, gameData map[string]any) string {
	t.Helper()
	itemID := uuid.NewString()
	sourceMetadata := mustMarshal(t, map[string]any{"data": gameData})
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, uuid.NewString(), "Test Game", sourceMetadata,
	)
	if err != nil {
		t.Fatalf("insertTestJobItem: %v", err)
	}
	return itemID
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}
