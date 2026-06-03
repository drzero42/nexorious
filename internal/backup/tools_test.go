package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/uptrace/bun"
)

// makeValidArchive creates a minimal valid .tar.gz archive that passes ValidateArchive.
// Used to provide a valid pre-restore backup for handleRestoreFailure tests.
func makeValidArchive(t *testing.T, dir string) string {
	t.Helper()
	sqlContent := "-- minimal sql dump"
	sqlHash := sha256.Sum256([]byte(sqlContent))
	dbChecksum := "sha256:" + fmt.Sprintf("%x", sqlHash)
	emptyHash := sha256.Sum256([]byte(""))
	coverChecksum := "sha256:" + fmt.Sprintf("%x", emptyHash)
	manifest := fmt.Sprintf(
		`{"version":1,"created_at":"2026-01-01T00:00:00Z","app_version":"0.1.0","migration_version":"20260101000001","backup_type":"manual","database_file":"database.sql","database_size_bytes":%d,"database_checksum":"%s","cover_art_count":0,"cover_art_size_bytes":0,"cover_art_checksum":"%s"}`,
		len(sqlContent), dbChecksum, coverChecksum,
	)

	archivePath := filepath.Join(dir, "pre-restore.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for name, content := range map[string]string{
		"backup-20260101-120000/manifest.json": manifest,
		"backup-20260101-120000/database.sql":  sqlContent,
	} {
		body := []byte(content)
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	return archivePath
}

// TestHandleRestoreFailure_ValidArchivePsqlFails exercises the rollback path where
// the pre-restore archive is valid but the psql connection is refused.
func TestHandleRestoreFailure_ValidArchivePsqlFails(t *testing.T) {
	CheckTools()
	if !PsqlAvailable() {
		t.Skip("psql not available")
	}

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create a valid archive as the "pre-restore" backup.
	archivePath := makeValidArchive(t, backupDir)
	preRestoreID := "pre-restore"
	// Rename to the expected backup path.
	expectedPath := filepath.Join(backupDir, preRestoreID+".tar.gz")
	if err := os.Rename(archivePath, expectedPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	origErr := errors.New("simulated restore failure")
	// Use unreachable DB so psql fails.
	conn := DBConnParams{Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "db"}
	appStateValue := ""
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { appStateValue = s },
	}
	err := svc.handleRestoreFailure(origErr, preRestoreID, conn, opts)
	// psql to 127.0.0.1:1 will fail → sets app state to db_unavailable.
	_ = err
	_ = appStateValue
}

// TestExtractTarGz_PathTraversal exercises the path traversal detection path.
func TestExtractTarGz_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tar.gz with a path traversal entry (../../etc/passwd).
	archivePath := filepath.Join(tmpDir, "traversal.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	body := []byte("evil content")
	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "../../evil.txt",
		Mode:     0o644,
		Size:     int64(len(body)),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	destDir := t.TempDir()
	err = ExtractTarGz(archivePath, destDir)
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

// TestCopyFile_Success exercises copyFile copying a file within the same dir.
func TestCopyFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")
	if err := os.WriteFile(src, []byte("hello copyFile"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "hello copyFile" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

// TestCopyFile_SrcNotFound exercises the open-source-error path.
func TestCopyFile_SrcNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	err := copyFile("/nonexistent/src.txt", filepath.Join(tmpDir, "dst.txt"))
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

// TestCopyFile_DstUnwritable exercises the create-destination-error path.
func TestCopyFile_DstUnwritable(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Use a path inside a nonexistent subdirectory so Create fails.
	err := copyFile(src, filepath.Join(tmpDir, "nonexistent", "dst.txt"))
	if err == nil {
		t.Fatal("expected error for unwritable destination")
	}
}

// TestHandleRestoreFailure_NoPreRestore calls handleRestoreFailure directly
// with an empty preRestoreID (no pre-restore backup available).
func TestHandleRestoreFailure_NoPreRestore(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := NewService(nil, "", backupDir, storageDir, "0.1.0")

	origErr := errors.New("simulated restore failure")
	conn := DBConnParams{Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "db"}
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) {},
	}
	err := svc.handleRestoreFailure(origErr, "", conn, opts)
	if err == nil {
		t.Fatal("expected error returned from handleRestoreFailure")
	}
	// Should return the original error.
	if !errors.Is(err, origErr) {
		t.Errorf("expected original error, got: %v", err)
	}
}

// TestHandleRestoreFailure_WithPreRestore exercises the rollback path where
// preRestoreID is set but the archive is invalid (rollback fails at extraction).
func TestHandleRestoreFailure_WithInvalidPreRestore(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create a fake pre-restore archive (not a real archive — rollback will fail at extraction).
	preRestoreID := "backup-20260101-000000"
	fakePath := filepath.Join(backupDir, preRestoreID+".tar.gz")
	if err := os.WriteFile(fakePath, []byte("not-a-real-archive"), 0o644); err != nil {
		t.Fatal(err)
	}

	origErr := errors.New("simulated restore failure")
	conn := DBConnParams{Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "db"}
	appStateCalled := false
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { appStateCalled = true },
	}
	err := svc.handleRestoreFailure(origErr, preRestoreID, conn, opts)
	if err == nil {
		t.Fatal("expected error from rollback failure")
	}
	if !appStateCalled {
		t.Error("expected SetAppState to be called on rollback failure")
	}
}

// makeEmptyTarGz creates a valid but empty .tar.gz archive (no entries).
// When extracted, the destination directory will be empty.
func makeEmptyTarGz(t *testing.T, dir string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "empty-pre-restore.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create empty archive: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()
	return archivePath
}

// TestHandleRestoreFailure_EmptyArchive exercises the path where the pre-restore
// archive extracts to an empty directory (no backup subdirectory found).
func TestHandleRestoreFailure_EmptyArchive(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := NewService(nil, "", backupDir, storageDir, "0.1.0")

	// Create an empty tar.gz as the pre-restore archive.
	archivePath := makeEmptyTarGz(t, backupDir)
	preRestoreID := "empty-pre-restore"
	expectedPath := filepath.Join(backupDir, preRestoreID+".tar.gz")
	if err := os.Rename(archivePath, expectedPath); err != nil {
		t.Fatalf("rename: %v", err)
	}

	origErr := errors.New("simulated restore failure")
	conn := DBConnParams{Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "db"}
	appStateCalled := false
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, nil },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { appStateCalled = true },
	}
	err := svc.handleRestoreFailure(origErr, preRestoreID, conn, opts)
	if err == nil {
		t.Fatal("expected error for empty archive")
	}
	if !appStateCalled {
		t.Error("expected SetAppState to be called for empty archive")
	}
}

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want DBConnParams
	}{
		{
			name: "standard postgres URL",
			url:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
		{
			name: "postgresql scheme",
			url:  "postgresql://admin:secret@db.example.com:5433/proddb",
			want: DBConnParams{Host: "db.example.com", Port: "5433", User: "admin", Password: "secret", DBName: "proddb"},
		},
		{
			name: "default port",
			url:  "postgres://user:pass@localhost/mydb",
			want: DBConnParams{Host: "localhost", Port: "5432", User: "user", Password: "pass", DBName: "mydb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDatabaseURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Host != tt.want.Host || got.Port != tt.want.Port || got.User != tt.want.User || got.Password != tt.want.Password || got.DBName != tt.want.DBName {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestValidateBackupID covers the path-traversal guard applied to backup IDs
// before they are joined into a filesystem path.
func TestValidateBackupID(t *testing.T) {
	valid := []string{
		"nexorious-backup-20260603-150405",
		"pre_restore-20260603-150405",
		"abc123",
	}
	for _, id := range valid {
		if err := validateBackupID(id); err != nil {
			t.Errorf("validateBackupID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",
		"..",
		"../../etc/passwd",
		"foo/bar",
		`foo\bar`,
		"foo/..",
		"with\x00nul",
		" leading",
		"trailing ",
	}
	for _, id := range invalid {
		if err := validateBackupID(id); !errors.Is(err, ErrInvalidBackupID) {
			t.Errorf("validateBackupID(%q) = %v, want ErrInvalidBackupID", id, err)
		}
	}
}

// TestGetBackupPath_RejectsTraversal verifies a traversal ID cannot produce a
// path outside the backup directory, and a valid ID resolves as expected.
func TestGetBackupPath_RejectsTraversal(t *testing.T) {
	s := NewService(nil, "", "/var/backups", "", "0.1.0")

	if _, err := s.GetBackupPath("../../etc/cron.d/evil"); !errors.Is(err, ErrInvalidBackupID) {
		t.Fatalf("GetBackupPath traversal: got %v, want ErrInvalidBackupID", err)
	}

	got, err := s.GetBackupPath("nexorious-backup-1")
	if err != nil {
		t.Fatalf("GetBackupPath valid: %v", err)
	}
	if want := filepath.Join("/var/backups", "nexorious-backup-1.tar.gz"); got != want {
		t.Fatalf("GetBackupPath = %q, want %q", got, want)
	}
}

// TestRestoreBackup_RejectsTraversal and TestDeleteBackup_RejectsTraversal
// confirm the guard fires through the service entry points the admin handlers
// call, before any filesystem or DB access.
func TestRestoreBackup_RejectsTraversal(t *testing.T) {
	s := NewService(nil, "", t.TempDir(), t.TempDir(), "0.1.0")
	if err := s.RestoreBackup("../../etc/passwd", RestoreOpts{}); !errors.Is(err, ErrInvalidBackupID) {
		t.Fatalf("RestoreBackup traversal: got %v, want ErrInvalidBackupID", err)
	}
}

func TestDeleteBackup_RejectsTraversal(t *testing.T) {
	s := NewService(nil, "", t.TempDir(), t.TempDir(), "0.1.0")
	if err := s.DeleteBackup("../../etc/passwd"); !errors.Is(err, ErrInvalidBackupID) {
		t.Fatalf("DeleteBackup traversal: got %v, want ErrInvalidBackupID", err)
	}
}

// TestExtractTarGz_DecompressionBomb verifies extraction aborts once the total
// decompressed size exceeds maxDecompressedBytes, rather than writing unbounded
// output from a small compressed archive.
func TestExtractTarGz_DecompressionBomb(t *testing.T) {
	orig := maxDecompressedBytes
	maxDecompressedBytes = 1 << 10 // 1 KiB
	defer func() { maxDecompressedBytes = orig }()

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "bomb.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	body := make([]byte, 4<<10) // 4 KiB, well over the 1 KiB cap
	hdr := &tar.Header{Typeflag: tar.TypeReg, Name: "big.bin", Mode: 0o600, Size: int64(len(body))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	if err := ExtractTarGz(archivePath, filepath.Join(tmpDir, "out")); err == nil {
		t.Fatal("expected ExtractTarGz to reject a decompression bomb, got nil")
	}
}
