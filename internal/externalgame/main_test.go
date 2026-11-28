package externalgame

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
	"github.com/drzero42/nexorious/internal/externalgame/egtest"
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
	// CASCADE from users clears external_games, their platform rows and the
	// user games.
	_, err := testDB.ExecContext(context.Background(), `TRUNCATE users, games CASCADE`)
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
// extGame describes an external_games row for seedExternalGame.
type extGame struct {
	skipped   bool
	resolved  int32 // 0 = unmatched; otherwise a games row is seeded too
	platforms bool  // whether to seed a pc-windows platform row
}

// seedExternalGame inserts a steam external game for userID and returns its id.
func seedExternalGame(t *testing.T, userID string, g extGame) string {
	t.Helper()
	eg := egtest.Game{UserID: userID, Title: "Horizon", Skipped: g.skipped}
	if g.resolved != 0 {
		seedGame(t, g.resolved, "Horizon")
		eg.ResolvedIGDBID = g.resolved
	}
	if g.platforms {
		eg.Platforms = []string{"pc-windows"}
	}
	return egtest.Insert(t, testDB, eg)
}

func userGameCount(t *testing.T, userID string, igdbID int32) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID).
		Scan(context.Background(), &n); err != nil {
		t.Fatalf("count user games: %v", err)
	}
	return n
}
