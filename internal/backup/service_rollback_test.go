package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uptrace/bun"
)

func createRollbackTestArchive(t *testing.T, backupDir, id string) {
	t.Helper()
	archivePath := filepath.Join(backupDir, id+".tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, entry := range []struct {
		name  string
		isDir bool
	}{
		{id + "/", true},
		{id + "/database.sql", false},
	} {
		hdr := &tar.Header{
			Name: entry.name,
			Mode: 0o755,
		}
		if entry.isDir {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

func TestHandleRestoreFailure_RollbackDropSchemaFails(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()

	preRestoreID := "nexorious-backup-20260101-000000"
	createRollbackTestArchive(t, backupDir, preRestoreID)

	orig := RunPsqlCommand
	t.Cleanup(func() { RunPsqlCommand = orig })
	RunPsqlCommand = func(_ DBConnParams, cmd string) error {
		if strings.Contains(cmd, "DROP SCHEMA") {
			return errors.New("simulated drop schema failure")
		}
		return nil
	}

	var capturedState string
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, errors.New("should not reach reconnect") },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { capturedState = s },
		MaxMigration:    "99999999999999",
	}

	originalErr := errors.New("primary restore failed")
	svc := &Service{backupPath: backupDir, storagePath: storageDir}

	err := svc.handleRestoreFailure(originalErr, preRestoreID, DBConnParams{}, opts)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rollback failed") {
		t.Errorf("error %q should contain 'rollback failed'", err.Error())
	}
	if !errors.Is(err, originalErr) {
		t.Errorf("error should wrap originalErr; got: %v", err)
	}
	if capturedState != "db_unavailable" {
		t.Errorf("SetAppState got %q, want 'db_unavailable'", capturedState)
	}
}
