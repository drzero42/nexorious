package migrate_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	migrate "github.com/drzero42/nexorious-go/internal/migrate"
)

func setupTestDB(t *testing.T) string {
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
	// golang-migrate's pgx/v5 driver registers under the "pgx5" scheme.
	connStr = "pgx5" + strings.TrimPrefix(connStr, "postgres")
	return connStr
}

func TestNewMigrator_FreshDatabase(t *testing.T) {
	connStr := setupTestDB(t)

	m, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.DetermineStateForTest(); err != nil {
		t.Fatalf("DetermineStateForTest: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	}()

	if m.State() != migrate.AppStateNeedsMigration {
		t.Errorf("expected NeedsMigration, got %v", m.State())
	}

	count, err := m.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending migration, got %d", count)
	}

	ver, dirty, err := m.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if ver != 0 || dirty {
		t.Errorf("expected version=0 dirty=false on fresh DB, got ver=%d dirty=%v", ver, dirty)
	}
}

func TestRunMigrations_TransitionsToReady(t *testing.T) {
	connStr := setupTestDB(t)
	m, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.DetermineStateForTest(); err != nil {
		t.Fatalf("DetermineStateForTest: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

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
	connStr := setupTestDB(t)
	ctx := context.Background()

	// First run — apply migrations.
	m1, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator first run: %v", err)
	}
	if err := m1.DetermineStateForTest(); err != nil {
		t.Fatalf("DetermineStateForTest: %v", err)
	}
	if err := m1.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations first run: %v", err)
	}
	if err := m1.Close(); err != nil {
		t.Logf("close: %v", err)
	}

	// Second run — schema is current; should start Ready.
	m2, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator second run: %v", err)
	}
	if err := m2.DetermineStateForTest(); err != nil {
		t.Fatalf("DetermineStateForTest: %v", err)
	}
	defer func() {
		if err := m2.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	}()

	if m2.State() != migrate.AppStateReady {
		t.Errorf("expected Ready on already-migrated DB, got %v", m2.State())
	}
}

func TestRunMigrations_AllTablesExist(t *testing.T) {
	ctx := context.Background()

	// Need a raw postgres:// URL for database/sql; spin up a second container.
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

	rawConn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Apply migrations via the Migrator (uses pgx5:// scheme internally).
	pgx5Conn := "pgx5" + strings.TrimPrefix(rawConn, "postgres")
	m, err := migrate.NewMigrator(pgx5Conn)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.DetermineStateForTest(); err != nil {
		t.Fatalf("DetermineStateForTest: %v", err)
	}
	defer func() { _ = m.Close() }()

	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Inspect tables via database/sql + pgx stdlib driver.
	db, err := openStdlib(rawConn)
	if err != nil {
		t.Fatalf("open stdlib db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close stdlib db: %v", err)
		}
	}()

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
		"pending_tasks",
		"backup_config",
		"rate_limiter_tokens",
	}

	for _, table := range expectedTables {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
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

// openStdlib opens a *sql.DB using the pgx stdlib driver.
// rawConn must be a standard postgres:// URL (not pgx5://).
func openStdlib(rawConn string) (*sql.DB, error) {
	return sql.Open("pgx", rawConn)
}

func setupPool(t *testing.T, connStr string) *pgxpool.Pool {
	t.Helper()
	// connStr from setupTestDB is pgx5://, but pgxpool needs postgres://
	pgxConnStr := "postgres" + strings.TrimPrefix(connStr, "pgx5")
	pool, err := pgxpool.New(context.Background(), pgxConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func badPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// A pool pointed at a non-existent host — all Pings will fail immediately.
	pool, err := pgxpool.New(context.Background(), "postgres://bad:bad@127.0.0.1:19999/x?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("badPool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestNewMigrator_SucceedsWhenDBUnreachable(t *testing.T) {
	// NewMigrator should succeed even with a bad URL — no DB contact at construction.
	m, err := migrate.NewMigrator("postgres://bad-host:5432/nope?sslmode=disable")
	if err != nil {
		t.Fatalf("NewMigrator with unreachable DB: %v", err)
	}
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("expected DBUnavailable, got %v", m.State())
	}
}

func TestInitNeedsSetup_NoUsers(t *testing.T) {
	connStr := setupTestDB(t)
	ctx := context.Background()
	m, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.DetermineStateForTest(); err != nil {
		t.Fatalf("determineState: %v", err)
	}
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	pool := setupPool(t, connStr)
	if err := m.InitNeedsSetup(ctx, pool); err != nil {
		t.Fatalf("InitNeedsSetup: %v", err)
	}
	if !m.NeedsSetup() {
		t.Error("expected NeedsSetup=true on empty users table")
	}
}

func TestInitNeedsSetup_UsersExist(t *testing.T) {
	connStr := setupTestDB(t)
	ctx := context.Background()
	m, err := migrate.NewMigrator(connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.DetermineStateForTest(); err != nil {
		t.Fatalf("determineState: %v", err)
	}
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	pool := setupPool(t, connStr)
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','admin','hash',true)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := m.InitNeedsSetup(ctx, pool); err != nil {
		t.Fatalf("InitNeedsSetup: %v", err)
	}
	if m.NeedsSetup() {
		t.Error("expected NeedsSetup=false when users exist")
	}
}

func TestStartDBProbe_SetsUnavailableOnPingFail(t *testing.T) {
	m, err := migrate.NewMigrator("postgres://bad:5432/x?sslmode=disable")
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	m.SetStateForTest(migrate.AppStateReady)
	m.SetProbeIntervalForTest(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := badPool(t)
	m.StartDBProbe(ctx, pool, func(_ context.Context) error { return nil })

	time.Sleep(150 * time.Millisecond)
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("expected DBUnavailable after ping fail, got %v", m.State())
	}
	if m.LastUnavailableAt().IsZero() {
		t.Error("expected LastUnavailableAt to be set")
	}
}

func TestStartDBProbe_RespectsContext(t *testing.T) {
	m, err := migrate.NewMigrator("postgres://bad:5432/x?sslmode=disable")
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	m.SetProbeIntervalForTest(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	pool := badPool(t)
	m.StartDBProbe(ctx, pool, func(_ context.Context) error { return nil })

	cancel() // should cause goroutine to exit cleanly
	time.Sleep(100 * time.Millisecond)
	// No assertion needed — if the goroutine leaks, the race detector will catch it.
}
