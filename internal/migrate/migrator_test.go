package migrate_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// setupTestContainer resets the shared container to a pristine, un-migrated
// state and returns its postgres:// connection string.
func setupTestContainer(t *testing.T) string {
	t.Helper()
	resetPublicSchema(t)
	return testDSN
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

// TestDetermineState_RiverDriftAlone verifies that River drift alone (bun
// migrations fully applied, but River missing a version) flips state to
// NeedsMigration. On a fresh DB the bun check short-circuits before River is
// consulted, so this is the only path that exercises riverNeedsMigration in the
// "needs migration" direction — guarding the Validate-based detection.
func TestDetermineState_RiverDriftAlone(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Apply everything (bun + River) so the DB starts fully migrated.
	m1 := migrate.NewMigrator(db)
	if err := m1.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m1.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Roll River back a single version, leaving bun untouched. The single-step
	// default keeps the river_migration table (only a version-1 downmigration
	// would drop it), so River now reports one unapplied version.
	riverMig, err := rivermigrate.New(riverdatabasesql.New(db.DB), nil)
	if err != nil {
		t.Fatalf("rivermigrate.New: %v", err)
	}
	if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionDown, nil); err != nil {
		t.Fatalf("River down-migrate: %v", err)
	}

	// bun is current, but River drift must drive the decision to NeedsMigration.
	m2 := migrate.NewMigrator(db)
	if err := m2.DetermineState(); err != nil {
		t.Fatalf("DetermineState after River drift: %v", err)
	}
	if m2.State() != migrate.AppStateNeedsMigration {
		t.Errorf("expected NeedsMigration from River drift, got %v", m2.State())
	}
	count, err := m2.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count <= 0 {
		t.Errorf("expected positive pending count from River drift, got %d", count)
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

func TestTransitionToFailed_SetsStateAndStoresError(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrating)

	if got := m.LastError(); got != "" {
		t.Fatalf("LastError before transition = %q, want empty", got)
	}

	m.TransitionToFailed(errors.New("boom"))

	if got := m.State(); got != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", got)
	}
	if got := m.LastError(); got != "boom" {
		t.Errorf("LastError = %q, want %q", got, "boom")
	}
}

func TestTransitionToReady_ClearsLastError(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrating)
	m.TransitionToFailed(errors.New("boom"))
	m.TransitionToReady()
	if got := m.LastError(); got != "" {
		t.Errorf("LastError after TransitionToReady = %q, want empty", got)
	}
	if m.State() != migrate.AppStateReady {
		t.Errorf("State = %v, want AppStateReady", m.State())
	}
}

func TestTransitionToFailed_OverwritesPreviousError(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrating)
	m.TransitionToFailed(errors.New("first"))
	m.TransitionToFailed(errors.New("second"))
	if got := m.LastError(); got != "second" {
		t.Errorf("LastError = %q, want %q", got, "second")
	}
}

func TestRunMigrations_FailureTransitionsToFailedWithError(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	// Close the underlying *sql.DB so bunMig.Lock fails inside RunMigrations.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	err := m.RunMigrations(context.Background())
	if err == nil {
		t.Fatal("RunMigrations: expected error from closed DB, got nil")
	}
	if m.State() != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty, want non-empty")
	}

	// The Lock-failure path must emit a log line before closing the SSE
	// channel, matching the other failure paths (issue #590). RunMigrations
	// has returned, so the channel is buffered, written, and closed: this
	// range drains and terminates without blocking.
	var logged strings.Builder
	for line := range m.LogCh() {
		logged.WriteString(line)
	}
	if !strings.Contains(logged.String(), "migration failed") {
		t.Errorf("expected a log line emitted on Lock failure, got %q", logged.String())
	}
}

func TestRunMigrations_ClearsLastErrorOnStart(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// Seed a previous failure.
	m.TransitionToFailed(errors.New("previous run failed"))

	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if got := m.LastError(); got != "" {
		t.Errorf("LastError after successful run = %q, want empty", got)
	}
}
