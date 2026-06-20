package migrate_test

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
	"github.com/drzero42/nexorious/internal/migrate"
)

// v0171Timestamps is the frozen set of 23 bun_migrations.name timestamps for
// the v0.17.1 release. Intentionally duplicates baseline.go's v0171Manifest so
// the test independently pins the values and acts as a canary if they diverge.
var v0171Timestamps = []string{
	"20260503000001", "20260531000001", "20260531000002", "20260601000001",
	"20260601000002", "20260601000003", "20260601000004", "20260602000001",
	"20260602000002", "20260602000003", "20260604000001", "20260604000002",
	"20260604000003", "20260604000004", "20260605000001", "20260605000002",
	"20260605000003", "20260608000001", "20260608000002", "20260608000003",
	"20260608000004", "20260609000001", "20260612000001",
}

// makeAdoptBunDB opens a *bun.DB from the shared testDSN. It is named
// differently from migrator_test.go's makeBunDB (which takes a connStr arg) to
// avoid a compile-time duplicate.
func makeAdoptBunDB(t *testing.T) *bun.DB {
	t.Helper()
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(testDSN))), pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedV0171 builds the v0.17.1 end-state: baseline schema (applied via the
// embedded baseline migration), then bun_migrations rewritten to exactly the 23
// manifest rows (no baseline row).
func seedV0171(t *testing.T, db *bun.DB) {
	t.Helper()
	ctx := context.Background()
	m := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := m.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil { // applies baseline
		t.Fatalf("apply baseline: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM bun_migrations"); err != nil {
		t.Fatalf("clear bun_migrations: %v", err)
	}
	for _, ts := range v0171Timestamps {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES (?, 1, now())", ts); err != nil {
			t.Fatalf("seed row %s: %v", ts, err)
		}
	}
}

// adoptNames returns all bun_migrations.name values in ascending order.
func adoptNames(t *testing.T, db *bun.DB) []string {
	t.Helper()
	var out []string
	if err := db.NewSelect().ColumnExpr("name").
		ModelTableExpr("bun_migrations").OrderExpr("name").
		Scan(context.Background(), &out); err != nil {
		t.Fatalf("read names: %v", err)
	}
	return out
}

func TestDetermineState_AdoptPending(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsAdopt {
		t.Fatalf("state = %v, want NeedsAdopt", m.State())
	}
}

func TestRunMigrations_AdoptRewritesHistory(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	got := adoptNames(t, db)
	if len(got) != 1 || got[0] != "20260620000001" {
		t.Fatalf("bun_migrations = %v, want exactly [20260620000001]", got)
	}
}

func TestDetermineState_BaselinePresentIsReady(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	// Fresh baseline install (baseline row present, no post-baseline migration).
	m0 := migrate.NewMigrator(db)
	if err := m0.DetermineState(); err != nil {
		t.Fatalf("DetermineState(fresh): %v", err)
	}
	if err := m0.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations(fresh): %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateReady {
		t.Fatalf("state = %v, want Ready", m.State())
	}
}

func TestDetermineState_PartialIsRefused(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)
	// Drop one manifest row → no longer the exact set.
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM bun_migrations WHERE name = ?", "20260612000001"); err != nil {
		t.Fatalf("delete row: %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateMigrationRefused {
		t.Fatalf("state = %v, want MigrationRefused", m.State())
	}
}

func TestRunMigrations_RefusedReturnsError(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES (?, 1, now())", "29990101000001"); err != nil {
		t.Fatalf("insert stranger: %v", err)
	}
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err == nil {
		t.Fatal("RunMigrations: want error on refuse, got nil")
	}
	if m.State() != migrate.AppStateMigrationRefused {
		t.Fatalf("state = %v, want MigrationRefused", m.State())
	}
}

func TestRunMigrations_AdoptThenCatchUp(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)

	// Build a migration set = baseline (from FS) + a synthetic post-baseline one.
	set := bunmigrate.NewMigrations()
	if err := set.Discover(migrations.FS); err != nil {
		t.Fatalf("discover baseline: %v", err)
	}
	synth := fstest.MapFS{
		"20260621000001_test_addcol.up.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE platforms ADD COLUMN test_adopt_marker text;"),
		},
		"20260621000001_test_addcol.down.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE platforms DROP COLUMN test_adopt_marker;"),
		},
	}
	if err := set.Discover(synth); err != nil {
		t.Fatalf("discover synth: %v", err)
	}

	m := migrate.NewMigratorWithMigrations(db, set)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsAdopt {
		t.Fatalf("pre-run state = %v, want NeedsAdopt", m.State())
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// bun_migrations is now exactly [baseline, synthetic].
	got := adoptNames(t, db)
	want := []string{"20260620000001", "20260621000001"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("bun_migrations = %v, want %v", got, want)
	}
	// And the synthetic migration's column actually exists.
	var n int
	if err := db.NewSelect().ColumnExpr("count(*)").
		TableExpr("information_schema.columns").
		Where("table_name = 'platforms' AND column_name = 'test_adopt_marker'").
		Scan(context.Background(), &n); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if n != 1 {
		t.Fatalf("synthetic column present = %d, want 1", n)
	}
}

func TestBaseline_SeedDataPresent(t *testing.T) {
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	for _, tbl := range []string{"platforms", "storefronts", "platform_storefronts", "backup_config"} {
		var n int
		if err := db.NewSelect().ColumnExpr("count(*)").TableExpr(tbl).
			Scan(context.Background(), &n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n == 0 {
			t.Fatalf("seed table %s is empty after baseline", tbl)
		}
	}
}
