package librarysmells

import (
	"context"
	"database/sql"
	"fmt"
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

	"github.com/drzero42/nexorious/internal/db/migrations"
)

var testDB *bun.DB

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "conn string: %v\n", err)
		os.Exit(1)
	}
	testDB = bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr))), pgdialect.New())
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
		fmt.Fprintf(os.Stderr, "river migrator: %v\n", err)
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
	_, err := testDB.ExecContext(context.Background(),
		`TRUNCATE users, games, user_games, user_game_platforms, smell_ignores CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at, updated_at)
		 VALUES (?, ?, 'x', false, now(), now())`,
		id, "u_"+id[:8]).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedGame(t *testing.T, gameID int32, title string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now())
		 ON CONFLICT (id) DO NOTHING`, gameID, title).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed game: %v", err)
	}
}

// seedUserGame inserts a user_games row (no platforms) and returns its id.
func seedUserGame(t *testing.T, userID string, gameID int32) string {
	t.Helper()
	seedGame(t, gameID, fmt.Sprintf("Game %d", gameID))
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
		id, userID, gameID).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_game: %v", err)
	}
	return id
}

// seedPlatform inserts a user_game_platforms row. Pass "" for a NULL platform or
// storefront. Use seeded names (pc-windows, steam) for non-null FK values.
func seedPlatform(t *testing.T, ugID, platform, storefront string) string {
	t.Helper()
	id := uuid.NewString()
	var p, s any
	if platform != "" {
		p = platform
	}
	if storefront != "" {
		s = storefront
	}
	_, err := testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		id, ugID, p, s).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	return id
}

func ignore(t *testing.T, userID, ugID, checkID string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO smell_ignores (id, user_id, user_game_id, check_id) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), userID, ugID, checkID).Exec(context.Background())
	if err != nil {
		t.Fatalf("ignore: %v", err)
	}
}

func flaggedIDs(items []FlaggedItem) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it.UserGameID] = true
	}
	return m
}
