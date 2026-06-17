package usergame

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

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
	"github.com/drzero42/nexorious/internal/db/models"
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
	// Truncate the tables these operation tests touch. CASCADE clears dependents.
	_, err := testDB.ExecContext(context.Background(),
		`TRUNCATE users, games, user_games, user_game_platforms, tags, user_game_tags, pools, pool_games, changes CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at, updated_at)
		 VALUES (?, ?, 'x', false, ?, ?)`,
		id, "u_"+id[:8], now, now).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

//nolint:unused // consumed by later tasks in this package
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
//
//nolint:unused // consumed by later tasks in this package
func seedUserGame(t *testing.T, userID string, gameID int32) string {
	t.Helper()
	seedGame(t, gameID, fmt.Sprintf("Game %d", gameID))
	ug := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: gameID}
	if _, err := testDB.NewInsert().Model(ug).Exec(context.Background()); err != nil {
		t.Fatalf("seed user_game: %v", err)
	}
	return ug.ID
}

func TestHarnessUp(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	if u == "" {
		t.Fatal("expected user id")
	}
}
