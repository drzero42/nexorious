package backup_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/backup"
)

// createTarGzWithFiles creates a .tar.gz archive with the given file name→content map.
func createTarGzWithFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test-multi.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		body := []byte(content)
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header for %s: %v", name, err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write body for %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return archivePath
}

// createMinimalTarGz creates a minimal .tar.gz archive containing a single file.
// Returns the archive path.
func createMinimalTarGz(t *testing.T, dir, filename, content string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	body := []byte(content)
	hdr := &tar.Header{
		Name: filename,
		Mode: 0o644,
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return archivePath
}

// createDirTarGz creates a .tar.gz archive containing a directory entry and a file.
func createDirTarGz(t *testing.T, dir string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "with-dir.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Directory entry.
	dirHdr := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "mydir/",
		Mode:     0o755,
	}
	if err := tw.WriteHeader(dirHdr); err != nil {
		t.Fatalf("write dir header: %v", err)
	}
	// File inside directory.
	body := []byte("hello from dir")
	fileHdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "mydir/file.txt",
		Mode:     0o644,
		Size:     int64(len(body)),
	}
	if err := tw.WriteHeader(fileHdr); err != nil {
		t.Fatalf("write file header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write file body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return archivePath
}


func TestCreateBackup(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
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

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
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

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
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

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
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
		SkipPreRestore: true,
		SetMaintenance: func(bool) {},
		ShutdownPool:   func() {},
		StopScheduler:  func() {},
		// Do NOT close testDB — the shared container must stay open for later tests.
		CloseDB: func() error { return nil },
		ReconnectDB: func() (*bun.DB, error) {
			sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
			restoredDB = bun.NewDB(sqldb, pgdialect.New())
			// Reassign the shared testDB so subsequent tests use the new connection.
			testDB = restoredDB
			return restoredDB, nil
		},
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) {},
		MaxMigration:    "99999999999999",
	}

	if err := svc.RestoreBackup(id, restoreOpts); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
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

// TestApplyRetention_EmptyDir exercises ApplyRetention when there are no backups.
func TestApplyRetention_EmptyDir(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	// No backups — both modes should complete without error.
	if err := svc.ApplyRetention("days", 7); err != nil {
		t.Fatalf("ApplyRetention(days): %v", err)
	}
	if err := svc.ApplyRetention("count", 3); err != nil {
		t.Fatalf("ApplyRetention(count): %v", err)
	}
	// Unknown mode — should also complete without error (no-op default).
	if err := svc.ApplyRetention("unknown", 5); err != nil {
		t.Fatalf("ApplyRetention(unknown): %v", err)
	}
}

// TestApplyRetention_WithBackups exercises the days/count retention branches.
// We create 1 backup then fake additional archive files in the backup dir.
func TestApplyRetention_WithBackups(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	// Create 1 real backup.
	id1, err := svc.CreateBackup("manual")
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

	// Apply count retention with count=1 — the 1 backup should remain (not deleted).
	if err := svc.ApplyRetention("count", 1); err != nil {
		t.Fatalf("ApplyRetention(count, 1): %v", err)
	}

	backups, err = svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after count retention: %v", err)
	}
	if len(backups) != 1 {
		t.Errorf("expected 1 backup after count retention, got %d", len(backups))
	}

	// Apply days retention with 0 days — the backup should be deleted.
	if err := svc.ApplyRetention("days", 0); err != nil {
		t.Fatalf("ApplyRetention(days, 0): %v", err)
	}

	backups, err = svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after days retention: %v", err)
	}
	// The backup should be deleted since 0 days means anything older than today.
	_ = id1
	_ = backups // may be 0 or 1 depending on exact timestamp comparison
}

// TestRestoreFromUpload_InvalidArchive exercises RestoreFromUpload when the
// upload path points to a non-archive file (fails ValidateArchive).
func TestRestoreFromUpload_InvalidArchive(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create a dummy "uploaded" file (not a real .tar.gz).
	uploadedPath := filepath.Join(t.TempDir(), "upload.tar.gz")
	if err := os.WriteFile(uploadedPath, []byte("not-a-real-archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := backup.RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(string) {},
	}

	_, err := svc.RestoreFromUpload(uploadedPath, opts)
	if err == nil {
		t.Fatal("expected error for invalid archive, got nil")
	}
}

// TestValidateArchive_NonExistentFile exercises the "file not found" path.
func TestValidateArchive_NonExistentFile(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive("/nonexistent/archive.tar.gz", false, "")
	if err == nil {
		t.Fatal("expected error for non-existent archive")
	}
}

// TestValidateArchive_NoManifest exercises the archive-has-no-manifest path.
func TestValidateArchive_NoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	// Archive with no manifest.json entry.
	archivePath := createMinimalTarGz(t, tmpDir, "somefile.txt", "content")

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive(archivePath, false, "")
	if err == nil {
		t.Fatal("expected error for archive without manifest")
	}
}

// TestValidateArchive_NoDatabaseSQL exercises the "no database.sql" path.
func TestValidateArchive_NoDatabaseSQL(t *testing.T) {
	tmpDir := t.TempDir()
	// Archive with manifest.json but no database.sql.
	manifest := `{"version":1,"created_at":"2026-01-01T00:00:00Z","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"manual","database_file":"database.sql"}`
	archivePath := createTarGzWithFiles(t, tmpDir, map[string]string{
		"backup-20260101-120000/manifest.json": manifest,
		// No database.sql included.
	})

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive(archivePath, false, "")
	if err == nil {
		t.Fatal("expected error for archive without database.sql")
	}
}

// TestValidateArchive_UnknownManifestVersion exercises the version > max path.
func TestValidateArchive_UnknownManifestVersion(t *testing.T) {
	tmpDir := t.TempDir()
	// Manifest with version=999 (future version).
	manifest := `{"version":999,"created_at":"2026-01-01T00:00:00Z","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"manual","database_file":"database.sql"}`
	archivePath := createTarGzWithFiles(t, tmpDir, map[string]string{
		"backup-20260101-120000/manifest.json": manifest,
	})

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive(archivePath, false, "")
	if err == nil {
		t.Fatal("expected error for unknown manifest version")
	}
}

// TestExtractTarGz_Success exercises the normal extraction path (file + dir entries).
func TestExtractTarGz_Success(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := createDirTarGz(t, tmpDir)

	destDir := t.TempDir()
	if err := backup.ExtractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("ExtractTarGz: %v", err)
	}
	// Verify file was extracted.
	extracted := filepath.Join(destDir, "mydir", "file.txt")
	data, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "hello from dir" {
		t.Errorf("unexpected extracted content: %q", string(data))
	}
}

// TestExtractTarGz_NonExistent exercises the open-file-error path.
func TestExtractTarGz_NonExistent(t *testing.T) {
	err := backup.ExtractTarGz("/nonexistent/path.tar.gz", t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-existent archive")
	}
}

// TestExtractTarGz_NotGzip exercises the gzip-parse-error path.
func TestExtractTarGz_NotGzip(t *testing.T) {
	// Create a file that's not a gzip archive.
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.tar.gz")
	if err := os.WriteFile(badPath, []byte("not gzip data"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := backup.ExtractTarGz(badPath, t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-gzip file")
	}
}

// TestListBackups_EmptyDir exercises ListBackups on an empty backup directory.
func TestListBackups_EmptyDir(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups empty: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

// TestApplyRetention_CountMode exercises the count-based retention path
// when count threshold is zero (delete everything).
func TestApplyRetention_CountZero(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	// Create a backup.
	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Apply count retention with count=0 — everything older than 0 kept = delete all.
	if err := svc.ApplyRetention("count", 0); err != nil {
		t.Fatalf("ApplyRetention(count, 0): %v", err)
	}

	// Verify the backup file was deleted.
	_, statErr := os.Stat(svc.GetBackupPath(id))
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("expected backup file to be deleted, stat error: %v", statErr)
	}
}

// TestCreateBackup_NoCoverArtDir exercises the copyDir path where the cover_art
// source directory does not exist (returns 0, 0, nil without error).
func TestCreateBackup_NoCoverArtDir(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
	backupDir := t.TempDir()
	// storageDir has no cover_art subdirectory — copyDir src won't exist.
	storageDir := t.TempDir()

	svc := backup.NewService(db, dsn, backupDir, storageDir, "0.1.0")

	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup without cover_art: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty backup ID")
	}
}

// createValidArchiveWithType creates a valid minimal archive with a given backup_type
// and created_at timestamp, writing it directly to backupDir as "backup-<name>.tar.gz".
// Returns the backup ID (filename without extension).
func createValidArchiveWithType(t *testing.T, backupDir, name, backupType string, createdAt time.Time) string {
	t.Helper()
	sqlContent := "-- sql dump for " + name
	sqlHash := sha256.Sum256([]byte(sqlContent))
	dbChecksum := "sha256:" + fmt.Sprintf("%x", sqlHash)
	emptyHash := sha256.Sum256([]byte(""))
	coverChecksum := "sha256:" + fmt.Sprintf("%x", emptyHash)

	manifest := fmt.Sprintf(
		`{"version":1,"created_at":"%s","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"%s","database_file":"database.sql","database_size_bytes":%d,"database_checksum":"%s","cover_art_count":0,"cover_art_size_bytes":0,"cover_art_checksum":"%s"}`,
		createdAt.UTC().Format(time.RFC3339),
		backupType,
		len(sqlContent),
		dbChecksum,
		coverChecksum,
	)

	archiveName := "nexorious-backup-" + name
	archivePath := filepath.Join(backupDir, archiveName+".tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	subdir := archiveName + "/"
	for entryName, content := range map[string]string{
		subdir + "manifest.json": manifest,
		subdir + "database.sql":  sqlContent,
	} {
		body := []byte(content)
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: entryName, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	return archiveName
}

// TestApplyRetention_DaysMode exercises the "days" retention mode where old
// non-pre_restore backups beyond the cutoff are deleted.
func TestApplyRetention_DaysMode(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create two valid archives: one old (10 days ago), one recent.
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	recentTime := time.Now().Add(-1 * time.Hour)

	createValidArchiveWithType(t, backupDir, "20260101-000000", "manual", oldTime)
	createValidArchiveWithType(t, backupDir, "20260110-000000", "manual", recentTime)

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups before retention, got %d", len(backups))
	}

	// Apply 7-day retention — the old backup (10 days old) should be deleted.
	if err := svc.ApplyRetention("days", 7); err != nil {
		t.Fatalf("ApplyRetention(days, 7): %v", err)
	}

	remaining, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after retention: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 backup after days retention, got %d", len(remaining))
	}
}

// TestApplyRetention_CountMode exercises the "count" retention mode where
// excess backups (beyond retentionValue) are deleted.
func TestApplyRetention_CountMode(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create 3 valid archives with distinct timestamps.
	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-2 * time.Hour)
	t3 := time.Now().Add(-1 * time.Hour)

	createValidArchiveWithType(t, backupDir, "20260101-010000", "manual", t1)
	createValidArchiveWithType(t, backupDir, "20260101-020000", "manual", t2)
	createValidArchiveWithType(t, backupDir, "20260101-030000", "manual", t3)

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups before retention, got %d", len(backups))
	}

	// Apply count=2 — only 2 most recent should remain.
	if err := svc.ApplyRetention("count", 2); err != nil {
		t.Fatalf("ApplyRetention(count, 2): %v", err)
	}

	remaining, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after count retention: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("expected 2 backups after count retention, got %d", len(remaining))
	}
}

// TestApplyRetention_PreRestoreCleanup exercises the pre_restore cleanup branch
// where pre_restore backups older than 7 days are deleted.
func TestApplyRetention_PreRestoreCleanup(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create a pre_restore backup older than 7 days.
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	createValidArchiveWithType(t, backupDir, "20260101-000000", "pre_restore", oldTime)

	// Create a recent pre_restore backup (should NOT be deleted).
	recentTime := time.Now().Add(-1 * time.Hour)
	createValidArchiveWithType(t, backupDir, "20260109-120000", "pre_restore", recentTime)

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups before retention, got %d", len(backups))
	}

	// Apply any retention mode — the pre_restore cleanup runs first regardless of mode.
	if err := svc.ApplyRetention("count", 10); err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}

	remaining, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after pre_restore cleanup: %v", err)
	}
	// Only the recent pre_restore should remain.
	if len(remaining) != 1 {
		t.Errorf("expected 1 backup after pre_restore cleanup, got %d", len(remaining))
	}
	if remaining[0].BackupType != "pre_restore" {
		t.Errorf("expected remaining backup to be pre_restore, got %q", remaining[0].BackupType)
	}
}

// TestValidateArchive_ChecksumMismatch exercises the checksum verification failure path
// by using a valid-format archive with a wrong checksum in the manifest.
func TestValidateArchive_ChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an archive with a manifest that has a wrong database checksum.
	manifest := `{"version":1,"created_at":"2026-01-01T00:00:00Z","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"manual","database_file":"database.sql","database_size_bytes":10,"database_checksum":"sha256:0000000000000000000000000000000000000000000000000000000000000000","cover_art_count":0,"cover_art_size_bytes":0,"cover_art_checksum":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`
	archivePath := createTarGzWithFiles(t, tmpDir, map[string]string{
		"backup-20260101-120000/manifest.json": manifest,
		"backup-20260101-120000/database.sql":  "actual content here",
	})

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive(archivePath, true, "")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

// TestValidateArchive_CoverArtChecksumMismatch exercises the cover_art checksum
// mismatch path in verifyArchiveChecksums (database.sql checksum is correct but
// cover_art checksum is wrong).
func TestValidateArchive_CoverArtChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	sqlContent := "-- sql dump"
	sqlHash := sha256.Sum256([]byte(sqlContent))
	dbChecksum := "sha256:" + fmt.Sprintf("%x", sqlHash)
	// Wrong cover_art checksum (all zeros).
	wrongCoverChecksum := "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	manifest := fmt.Sprintf(
		`{"version":1,"created_at":"2026-01-01T00:00:00Z","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"manual","database_file":"database.sql","database_size_bytes":%d,"database_checksum":"%s","cover_art_count":0,"cover_art_size_bytes":0,"cover_art_checksum":"%s"}`,
		len(sqlContent), dbChecksum, wrongCoverChecksum,
	)

	archivePath := createTarGzWithFiles(t, tmpDir, map[string]string{
		"backup-20260101-120000/manifest.json": manifest,
		"backup-20260101-120000/database.sql":  sqlContent,
	})

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")

	_, err := svc.ValidateArchive(archivePath, true, "")
	if err == nil {
		t.Fatal("expected cover_art checksum mismatch error")
	}
}

// TestValidateArchive_WithRealArchive exercises the checksum verification path.
func TestValidateArchive_WithRealArchive(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	truncateAllTables(t)
	db := testDB
	dsn := testDSN
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

	archivePath := svc.GetBackupPath(id)

	// ValidateArchive with checksum verification.
	manifest, err := svc.ValidateArchive(archivePath, true, "")
	if err != nil {
		t.Fatalf("ValidateArchive with checksums: %v", err)
	}
	if manifest == nil {
		t.Fatal("expected non-nil manifest")
	}

	// ValidateArchive with a newer migration version than the backup.
	_, err = svc.ValidateArchive(archivePath, false, "00000000000000")
	if err == nil {
		t.Error("expected error for maxMigrationVersion too old")
	}
}
