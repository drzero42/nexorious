package migrate_test

import (
	"context"
	"strings"
	"testing"

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
	ctx := context.Background()

	m, err := migrate.NewMigrator(ctx, connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
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
	ctx := context.Background()

	m, err := migrate.NewMigrator(ctx, connStr)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	}()

	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if m.State() != migrate.AppStateReady {
		t.Errorf("expected Ready after migration, got %v", m.State())
	}

	count, err := m.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount after migration: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pending migrations after run, got %d", count)
	}

	ver, dirty, err := m.CurrentVersion()
	if err != nil {
		t.Fatalf("CurrentVersion after migration: %v", err)
	}
	if ver != 1 || dirty {
		t.Errorf("expected version=1 dirty=false after migration, got ver=%d dirty=%v", ver, dirty)
	}
}

func TestNewMigrator_AlreadyMigrated(t *testing.T) {
	connStr := setupTestDB(t)
	ctx := context.Background()

	// First run — apply migrations.
	m1, err := migrate.NewMigrator(ctx, connStr)
	if err != nil {
		t.Fatalf("NewMigrator first run: %v", err)
	}
	if err := m1.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations first run: %v", err)
	}
	if err := m1.Close(); err != nil {
		t.Logf("close: %v", err)
	}

	// Second run — schema is current; should start Ready.
	m2, err := migrate.NewMigrator(ctx, connStr)
	if err != nil {
		t.Fatalf("NewMigrator second run: %v", err)
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
