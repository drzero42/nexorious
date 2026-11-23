package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/restoremarker"
)

// restoringDB returns a pristine database whose restore marker says restoring,
// and its DSN.
func restoringDB(t *testing.T, name string) (*bun.DB, string) {
	t.Helper()
	db := freshDB(t, name)
	if err := restoremarker.Set(context.Background(), db, restoremarker.Marker{
		Phase: restoremarker.PhaseRestoring, BackupID: "backup-1",
	}); err != nil {
		t.Fatalf("set marker: %v", err)
	}
	return db, strings.Replace(testDSN, "/nexorious_test?", "/"+name+"?", 1)
}

func assertNothingMigrated(t *testing.T, db *bun.DB) {
	t.Helper()
	var reg sql.NullString
	if err := db.QueryRowContext(context.Background(), `SELECT to_regclass('public.bun_migrations')::text`).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	if reg.Valid {
		t.Error("bun_migrations was created in a database marked restoring")
	}
}

// serve --migrate does not migrate a database a restore left marked restoring,
// but does start, so the server can show the restore-failed page (#1341).
func TestRunStartupMigrations_RestoreFailedDoesNotMigrate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	db, _ := restoringDB(t, "serve_migrate_restore_failed")
	migrator := migrate.NewMigrator(db)
	if err := runStartupMigrations(context.Background(), db, migrator, 10*time.Second); err != nil {
		t.Fatalf("runStartupMigrations: %v, want nil (serve the restore-failed page)", err)
	}
	if migrator.State() != migrate.AppStateRestoreFailed {
		t.Errorf("state = %v, want restore_failed", migrator.State())
	}
	assertNothingMigrated(t, db)
}

// `nexorious migrate` exits non-zero naming the marker, and `migrate status`
// reports the state without creating anything.
func TestMigrateCmd_RestoreFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-backed test in -short mode")
	}
	db, dsn := restoringDB(t, "migrate_cmd_restore_failed")
	envFile := filepath.Join(t.TempDir(), "test.env")
	if err := os.WriteFile(envFile, []byte("DB_ENCRYPTION_KEY=test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", dsn)

	run := func(args ...string) (string, error) {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"--config", envFile}, args...))
		err := root.Execute()
		return out.String(), err
	}

	if _, err := run("migrate"); err == nil || !strings.Contains(err.Error(), "nexorious_restore.marker") {
		t.Errorf("migrate: err = %v, want a refusal naming the restore marker", err)
	}
	out, err := run("migrate", "status")
	if err != nil {
		t.Fatalf("migrate status: %v", err)
	}
	if !strings.Contains(out, "state=restore_failed") {
		t.Errorf("migrate status output = %q, want state=restore_failed", out)
	}
	assertNothingMigrated(t, db)
}
