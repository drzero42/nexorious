package migrate_test

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
)

// testDSN is the postgres:// connection string for the single shared container
// used by every test in this package.
var testDSN string

// controlDB is a long-lived handle used solely to reset the schema between
// tests; tests open their own handles via makeBunDB so they can close them
// mid-test without disturbing the reset machinery.
var controlDB *bun.DB

// TestMain starts ONE postgres container for the whole package. Unlike the
// data-layer packages it does NOT run migrations here: the migrate package
// tests the migrator itself, so most tests need a pristine, un-migrated
// database. resetPublicSchema gives each test that fresh slate cheaply, without
// paying for a new container per test.
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

	controlDB = bun.NewDB(
		sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr))),
		pgdialect.New(),
	)
	defer func() { _ = controlDB.Close() }()

	os.Exit(m.Run())
}

// resetPublicSchema drops and recreates the public schema on the shared
// container, returning a pristine, un-migrated database to the caller. All
// nexorious objects live in public (no extensions or extra schemas), so this is
// a complete reset. Tests run sequentially, so the prior test's handle is
// already closed by the time this runs.
func resetPublicSchema(t *testing.T) {
	t.Helper()
	if _, err := controlDB.ExecContext(context.Background(),
		`DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO public;`); err != nil {
		t.Fatalf("resetPublicSchema: %v", err)
	}
}
