package backup_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/backup"
)

func setupTestDB(t *testing.T) (*bun.DB, string) {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
				"POSTGRES_DB":       "testdb",
			},
			WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	dsn := "postgres://test:test@" + host + ":" + port.Port() + "/testdb?sslmode=disable"

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS games (id SERIAL PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS tags (id TEXT PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL);
		INSERT INTO schema_migrations (version) VALUES (20260503000001);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db, dsn
}

func TestCreateBackup(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()

	coverDir := filepath.Join(storageDir, "cover_art")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "test.jpg"), []byte("fake image"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty backup ID")
	}

	archivePath := svc.GetBackupPath(id)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive not found at %s: %v", archivePath, err)
	}
}

func TestListBackups(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	_, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].BackupType != "manual" {
		t.Errorf("expected backup_type 'manual', got %q", backups[0].BackupType)
	}
}

func TestDeleteBackup(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if err := svc.DeleteBackup(id); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	if _, err := os.Stat(svc.GetBackupPath(id)); !errors.Is(err, fs.ErrNotExist) {
		t.Error("archive should have been deleted")
	}
}

func TestRestoreBackup(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() || !backup.PsqlAvailable() {
		t.Skip("pg_dump or psql not available")
	}

	db, dsn := setupTestDB(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "INSERT INTO users (id) VALUES ('will-be-restored')")

	var restoredDB *bun.DB
	restoreOpts := backup.RestoreOpts{
		SkipPreRestore:  true,
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { _ = db.Close(); return nil },
		ReconnectDB: func() (*bun.DB, error) {
			sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
			restoredDB = bun.NewDB(sqldb, pgdialect.New())
			return restoredDB, nil
		},
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func() error { return nil },
		SetAppState:     func(s string) {},
		MaxMigration:    "99999999999999",
	}

	if err := svc.RestoreBackup(id, restoreOpts); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if restoredDB != nil {
		t.Cleanup(func() { _ = restoredDB.Close() })
	}
	queryDB := restoredDB
	if queryDB == nil {
		queryDB = db
	}

	var count int
	err = queryDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = 'will-be-restored'").Scan(&count)
	if err != nil {
		t.Fatalf("query after restore: %v", err)
	}
	if count != 0 {
		t.Error("expected 'will-be-restored' row to be absent after restore")
	}
}

func TestDeleteBackupNotFound(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	err := svc.DeleteBackup("nonexistent-id")
	if !errors.Is(err, backup.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}


