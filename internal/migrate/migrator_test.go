package migrate_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// setupTestContainer starts a postgres container and returns the postgres:// connection string.
func setupTestContainer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("nexorious_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return connStr // postgres:// URL
}

// makeBunDB creates a bun.DB from a postgres:// connection string.
func makeBunDB(t *testing.T, connStr string) *bun.DB {
	t.Helper()
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// setupTestDB starts a postgres container and returns a ready *bun.DB.
func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	return makeBunDB(t, setupTestContainer(t))
}

func TestNewMigrator_FreshDatabase(t *testing.T) {
	db := setupTestDB(t)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	if m.State() != migrate.AppStateNeedsMigration {
		t.Errorf("expected NeedsMigration, got %v", m.State())
	}

	count, err := m.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count <= 0 {
		t.Errorf("expected positive pending count on fresh DB, got %d", count)
	}
}

func TestRunMigrations_TransitionsToReady(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	ctx := context.Background()
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	// RunMigrations must NOT set Ready — leaves state as Migrating.
	if m.State() != migrate.AppStateMigrating {
		t.Errorf("expected Migrating after RunMigrations, got %v", m.State())
	}
	// Handler goroutine calls TransitionToReady after InitNeedsSetup.
	m.TransitionToReady()
	if m.State() != migrate.AppStateReady {
		t.Errorf("expected Ready after TransitionToReady, got %v", m.State())
	}
}

func TestNewMigrator_AlreadyMigrated(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// First run — apply migrations.
	m1 := migrate.NewMigrator(db)
	if err := m1.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m1.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations first run: %v", err)
	}

	// Second migrator on same DB — schema is current; should start Ready.
	m2 := migrate.NewMigrator(db)
	if err := m2.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	if m2.State() != migrate.AppStateReady {
		t.Errorf("expected Ready on already-migrated DB, got %v", m2.State())
	}
}

func TestRunMigrations_AllTablesExist(t *testing.T) {
	ctx := context.Background()
	connStr := setupTestContainer(t)
	db := makeBunDB(t, connStr)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	expectedTables := []string{
		"users",
		"user_sessions",
		"games",
		"platforms",
		"storefronts",
		"platform_storefronts",
		"user_games",
		"user_game_platforms",
		"tags",
		"user_game_tags",
		"external_games",
		"user_sync_configs",
		"jobs",
		"job_items",
		"river_queue",
		"river_job",
		"river_leader",
		"river_client",
		"river_client_queue",
		"backup_config",
		"rate_limiter_tokens",
	}

	for _, table := range expectedTables {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = ?
			)`, table).Scan(&exists); err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created by the migration", table)
		}
	}

	// Verify backup_config singleton row seeded correctly.
	var schedCron string
	if err := db.QueryRowContext(ctx,
		"SELECT schedule_cron FROM backup_config WHERE id = 1").Scan(&schedCron); err != nil {
		t.Fatalf("reading backup_config: %v", err)
	}
	if schedCron != "0 2 * * *" {
		t.Errorf("backup_config.schedule_cron = %q, want %q", schedCron, "0 2 * * *")
	}
}

func badBunDB(t *testing.T) *bun.DB {
	t.Helper()
	// A db pointed at a non-existent host — all pings will fail immediately.
	sqldb := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithDSN("postgres://bad:bad@127.0.0.1:19999/x?sslmode=disable&connect_timeout=1"),
	))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewMigrator_SucceedsWhenDBUnreachable(t *testing.T) {
	// NewMigrator should succeed even with a bad DB — no DB contact at construction.
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://bad-host:5432/nope?sslmode=disable")))
	db := bun.NewDB(sqldb, pgdialect.New())
	defer func() { _ = db.Close() }()

	m := migrate.NewMigrator(db)
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("expected DBUnavailable, got %v", m.State())
	}
}

func TestInitNeedsSetup_NoUsers(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("determineState: %v", err)
	}
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if err := m.InitNeedsSetup(ctx, db); err != nil {
		t.Fatalf("InitNeedsSetup: %v", err)
	}
	if !m.NeedsSetup() {
		t.Error("expected NeedsSetup=true on empty users table")
	}
}

func TestInitNeedsSetup_UsersExist(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("determineState: %v", err)
	}
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','admin','hash',true)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := m.InitNeedsSetup(ctx, db); err != nil {
		t.Fatalf("InitNeedsSetup: %v", err)
	}
	if m.NeedsSetup() {
		t.Error("expected NeedsSetup=false when users exist")
	}
}

func TestStartDBProbe_SetsUnavailableOnPingFail(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	m.SetStateForTest(migrate.AppStateReady)
	m.SetProbeIntervalForTest(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	badDB := badBunDB(t)
	m.StartDBProbe(ctx, badDB, func(_ context.Context) error { return nil })

	time.Sleep(150 * time.Millisecond)
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("expected DBUnavailable after ping fail, got %v", m.State())
	}
	if m.LastUnavailableAt().IsZero() {
		t.Error("expected LastUnavailableAt to be set")
	}
}

func TestStartDBProbe_RespectsContext(t *testing.T) {
	badDB := badBunDB(t)
	sqldb2 := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN("postgres://bad:5432/x?sslmode=disable")))
	db2 := bun.NewDB(sqldb2, pgdialect.New())
	defer func() { _ = db2.Close() }()

	m := migrate.NewMigrator(db2)
	m.SetProbeIntervalForTest(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	m.StartDBProbe(ctx, badDB, func(_ context.Context) error { return nil })

	cancel() // should cause goroutine to exit cleanly
	time.Sleep(100 * time.Millisecond)
	// No assertion needed — if the goroutine leaks, the race detector will catch it.
}
