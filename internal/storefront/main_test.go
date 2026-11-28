package storefront

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
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

	"github.com/drzero42/nexorious/internal/crypto"
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
	// CASCADE from users clears user_sync_configs.
	if _, err := testDB.ExecContext(context.Background(), `TRUNCATE users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	if _, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at, updated_at)
		 VALUES (?, ?, 'x', false, ?, ?)`,
		id, "u_"+id[:8], now, now).Exec(context.Background()); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func testEncrypter(t *testing.T) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func seedStoredSignIn(t *testing.T, userID, storefront, ciphertext string) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (gen_random_uuid(), ?, ?, 'daily', ?)`,
		userID, storefront, ciphertext,
	); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func storedSignIn(t *testing.T, userID, storefront string) string {
	t.Helper()
	var ct string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ?`, userID, storefront).
		Scan(context.Background(), &ct); err != nil {
		t.Fatalf("read config: %v", err)
	}
	return ct
}

// syncConfig reads the user's Connection row as the sync worker loads it.
func syncConfig(t *testing.T, userID, storefront string) models.UserSyncConfig {
	t.Helper()
	var cfg models.UserSyncConfig
	if err := testDB.NewSelect().Model(&cfg).Where("user_id = ? AND storefront = ?", userID, storefront).
		Scan(context.Background()); err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
