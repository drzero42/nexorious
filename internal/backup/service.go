package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
)

// ErrOperationInProgress is returned when a backup or restore operation is already running.
var ErrOperationInProgress = errors.New("a backup or restore operation is already in progress")

// ErrNotFound is returned when a requested backup archive does not exist.
var ErrNotFound = errors.New("backup not found")

// ErrInvalidBackupID is returned when a backup ID is empty or could escape the
// backup directory via path traversal.
var ErrInvalidBackupID = errors.New("invalid backup id")

// Service handles backup create/list/delete/validate/restore operations.
type Service struct {
	db          *bun.DB
	databaseURL string
	backupPath  string
	storagePath string
	appVersion  string
	mu          sync.Mutex
}

// NewService creates a new backup service.
func NewService(db *bun.DB, databaseURL, backupPath, storagePath, appVersion string) *Service {
	return &Service{
		db:          db,
		databaseURL: databaseURL,
		backupPath:  backupPath,
		storagePath: storagePath,
		appVersion:  appVersion,
	}
}

// SetDB updates the database connection (used after restore reconnect).
func (s *Service) SetDB(db *bun.DB) {
	s.db = db
}

// CreateBackup creates a backup archive and returns the backup ID.
// backupType is "manual", "scheduled", or "pre_restore".
func (s *Service) CreateBackup(backupType string) (string, error) {
	if !s.mu.TryLock() {
		return "", ErrOperationInProgress
	}
	defer s.mu.Unlock()

	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}

	id := fmt.Sprintf("nexorious-backup-%s", time.Now().UTC().Format("20060102-150405"))

	tmpDir, err := os.MkdirTemp("", "nexorious-backup-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	backupDir := filepath.Join(tmpDir, id)
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	dbSQLPath := filepath.Join(backupDir, "database.sql")
	if err := RunPgDump(conn, dbSQLPath); err != nil {
		return "", fmt.Errorf("pg_dump: %w", err)
	}

	coverArtSrc := filepath.Join(s.storagePath, "cover_art")
	coverArtDst := filepath.Join(backupDir, "cover_art")
	coverArtCount, coverArtSize, err := copyDir(coverArtSrc, coverArtDst)
	if err != nil {
		return "", fmt.Errorf("copy cover art: %w", err)
	}

	ctx := context.Background()
	var statsUsers, statsGames, statsTags int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&statsUsers)                     //nolint:errcheck // cosmetic stat; zero value acceptable on error
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT game_id) FROM user_games").Scan(&statsGames) //nolint:errcheck // cosmetic stat; zero value acceptable on error
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags").Scan(&statsTags)                       //nolint:errcheck // cosmetic stat; zero value acceptable on error

	var migrationVersion string
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(name), '') FROM bun_migrations").Scan(&migrationVersion) //nolint:errcheck // cosmetic stat; zero value acceptable on error

	dbChecksum, dbSize := checksumFile(dbSQLPath)
	coverArtChecksum := checksumDir(coverArtDst)

	manifest := Manifest{
		Version:           ManifestVersion,
		CreatedAt:         time.Now().UTC(),
		AppVersion:        s.appVersion,
		MigrationVersion:  migrationVersion,
		BackupType:        backupType,
		DatabaseFile:      "database.sql",
		DatabaseSizeBytes: dbSize,
		DatabaseChecksum:  "sha256:" + dbChecksum,
		CoverArtCount:     coverArtCount,
		CoverArtSizeBytes: coverArtSize,
		CoverArtChecksum:  "sha256:" + coverArtChecksum,
		StatsUsers:        statsUsers,
		StatsGames:        statsGames,
		StatsTags:         statsTags,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), manifestBytes, 0o600); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	if err := os.MkdirAll(s.backupPath, 0o750); err != nil {
		return "", fmt.Errorf("create backup path: %w", err)
	}
	archivePath := filepath.Join(s.backupPath, id+".tar.gz")
	if err := createTarGz(archivePath, tmpDir, id); err != nil {
		return "", fmt.Errorf("create archive: %w", err)
	}

	slog.InfoContext(ctx, "backup created", "id", id, "type", backupType, "path", archivePath)
	return id, nil
}

// ListBackups returns all backups sorted by created_at descending.
func (s *Service) ListBackups() ([]BackupInfo, error) {
	pattern := filepath.Join(s.backupPath, "nexorious-backup-*.tar.gz")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob backups: %w", err)
	}

	var backups []BackupInfo
	for _, archivePath := range matches {
		manifest, err := readManifestFromArchive(archivePath)
		if err != nil {
			slog.Warn("skipping invalid backup archive", "path", archivePath, logging.KeyErr, err)
			continue
		}
		var sizeBytes int64
		if info, statErr := os.Stat(archivePath); statErr == nil {
			sizeBytes = info.Size()
		}
		bi := BackupInfo{
			ID:         strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz"),
			CreatedAt:  manifest.CreatedAt,
			BackupType: manifest.BackupType,
			SizeBytes:  sizeBytes,
		}
		bi.Stats.Users = manifest.StatsUsers
		bi.Stats.Games = manifest.StatsGames
		bi.Stats.Tags = manifest.StatsTags
		backups = append(backups, bi)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// validateBackupID rejects backup IDs that could escape the backup directory
// via path traversal. A valid ID is a single path component with no separators,
// "..", NUL bytes, or surrounding whitespace.
func validateBackupID(backupID string) error {
	if backupID == "" {
		return fmt.Errorf("%w: empty", ErrInvalidBackupID)
	}
	if strings.ContainsAny(backupID, `/\`) || strings.Contains(backupID, "..") ||
		strings.ContainsRune(backupID, 0) || strings.TrimSpace(backupID) != backupID ||
		filepath.Base(backupID) != backupID {
		return fmt.Errorf("%w: %q", ErrInvalidBackupID, backupID)
	}
	return nil
}

// GetBackupPath returns the full filesystem path for a backup archive after
// validating backupID against path traversal.
func (s *Service) GetBackupPath(backupID string) (string, error) {
	if err := validateBackupID(backupID); err != nil {
		return "", err
	}
	return filepath.Join(s.backupPath, backupID+".tar.gz"), nil
}

// DeleteBackup removes a backup archive file.
func (s *Service) DeleteBackup(backupID string) error {
	path, err := s.GetBackupPath(backupID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("delete backup %s: %w", backupID, ErrNotFound)
		}
		return fmt.Errorf("delete backup %s: %w", backupID, err)
	}
	slog.Info("backup deleted", "id", backupID)
	return nil
}

// ValidateArchive opens an archive, reads the manifest, checks database.sql exists,
// and optionally verifies SHA-256 checksums.
func (s *Service) ValidateArchive(archivePath string, verifyChecksums bool, maxMigrationVersion string) (*Manifest, error) {
	manifest, err := readManifestFromArchive(archivePath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	if manifest.Version > MaxManifestVersion {
		return nil, fmt.Errorf("unknown manifest version %d (max supported: %d)", manifest.Version, MaxManifestVersion)
	}

	if maxMigrationVersion != "" && manifest.MigrationVersion > maxMigrationVersion {
		return nil, fmt.Errorf(
			"backup was created by a newer version of Nexorious (migration %s); this binary only supports up to migration %s — upgrade before restoring",
			manifest.MigrationVersion, maxMigrationVersion,
		)
	}

	// Always verify database.sql is present
	found, err := archiveContainsFile(archivePath, "database.sql")
	if err != nil {
		return nil, fmt.Errorf("check archive contents: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("archive does not contain database.sql")
	}

	if verifyChecksums {
		if err := verifyArchiveChecksums(archivePath, manifest); err != nil {
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	return manifest, nil
}

// ArchiveInfo summarizes one candidate backup archive found in the backup
// directory. Files that fail to validate end-to-end (corrupt manifest,
// migration version newer than this binary supports, etc.) are still returned
// with Restorable=false and a human-readable Reason so the UI can show them.
type ArchiveInfo struct {
	Filename   string    `json:"filename"` // base name only
	SizeBytes  int64     `json:"size_bytes"`
	ModTime    time.Time `json:"mtime"`
	Manifest   *Manifest `json:"manifest,omitempty"`
	Restorable bool      `json:"restorable"`
	Reason     string    `json:"reason,omitempty"`
}

// BackupPath returns the configured backup directory path. Exposed so handlers
// can safely resolve a user-supplied filename to a full path under it.
func (s *Service) BackupPath() string {
	return s.backupPath
}

// ListAvailableArchives scans the configured backup directory (top-level only)
// for *.tar.gz files and returns metadata for each. Files appear regardless of
// whether they validate so callers can show non-restorable files with an
// explanation. Sorted newest mtime first.
//
// Returns an empty slice (not an error) when the directory is empty,
// unreadable, or doesn't exist — listing is best-effort discovery.
func (s *Service) ListAvailableArchives(ctx context.Context, maxMigrationVersion string) ([]ArchiveInfo, error) {
	if s.backupPath == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.backupPath)
	if err != nil {
		// Missing dir / permission error is not fatal — listing is best-effort.
		slog.DebugContext(ctx, "ListAvailableArchives: ReadDir failed", "path", s.backupPath, logging.KeyErr, err)
		return nil, nil
	}

	infos := make([]ArchiveInfo, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		fullPath := filepath.Join(s.backupPath, name)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		// Only regular files. Skip symlinks, sockets, devices.
		if !fi.Mode().IsRegular() {
			continue
		}

		info := ArchiveInfo{
			Filename:  name,
			SizeBytes: fi.Size(),
			ModTime:   fi.ModTime().UTC(),
		}

		manifest, mErr := readManifestFromArchive(fullPath)
		switch {
		case mErr != nil:
			info.Restorable = false
			info.Reason = "unreadable manifest"
		case manifest.Version > MaxManifestVersion:
			info.Restorable = false
			info.Reason = fmt.Sprintf("unknown manifest version %d (max supported: %d)", manifest.Version, MaxManifestVersion)
			info.Manifest = manifest
		case maxMigrationVersion != "" && manifest.MigrationVersion > maxMigrationVersion:
			info.Restorable = false
			info.Reason = fmt.Sprintf(
				"backup was created by a newer version of Nexorious (migration %s); this binary only supports up to migration %s — upgrade before restoring",
				manifest.MigrationVersion, maxMigrationVersion,
			)
			info.Manifest = manifest
		default:
			// Final restorability check: database.sql must be present in the
			// archive. Mirrors the assertion ValidateArchive makes at restore
			// time, so a Restorable=true entry is a real promise.
			found, fErr := archiveContainsFile(fullPath, "database.sql")
			if fErr != nil || !found {
				info.Restorable = false
				info.Reason = "archive is missing database.sql"
			} else {
				info.Restorable = true
			}
			info.Manifest = manifest
		}

		infos = append(infos, info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ModTime.After(infos[j].ModTime)
	})
	return infos, nil
}

// archiveContainsFile returns true if the .tar.gz archive contains an entry
// whose base name matches filename.
func archiveContainsFile(archivePath, filename string) (bool, error) {
	f, err := os.Open(archivePath) //nolint:gosec // archivePath is an internally-derived backup path, not user input
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return false, err
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if filepath.Base(hdr.Name) == filename {
			return true, nil
		}
	}
}

// ApplyRetention deletes backups exceeding the retention policy.
func (s *Service) ApplyRetention(retentionMode string, retentionValue int) error {
	backups, err := s.ListBackups()
	if err != nil {
		return err
	}

	now := time.Now()

	for _, b := range backups {
		if b.BackupType == "pre_restore" && now.Sub(b.CreatedAt) > 7*24*time.Hour {
			if err := s.DeleteBackup(b.ID); err != nil {
				slog.Warn("retention: failed to delete old pre-restore backup", "id", b.ID, logging.KeyErr, err)
			}
		}
	}

	backups, err = s.ListBackups()
	if err != nil {
		return err
	}

	switch retentionMode {
	case "days":
		cutoff := now.AddDate(0, 0, -retentionValue)
		for _, b := range backups {
			if b.BackupType != "pre_restore" && b.CreatedAt.Before(cutoff) {
				if err := s.DeleteBackup(b.ID); err != nil {
					slog.Warn("retention: failed to delete old backup", "id", b.ID, logging.KeyErr, err)
				}
			}
		}
	case "count":
		nonPreRestore := 0
		for _, b := range backups {
			if b.BackupType == "pre_restore" {
				continue
			}
			nonPreRestore++
			if nonPreRestore > retentionValue {
				if err := s.DeleteBackup(b.ID); err != nil {
					slog.Warn("retention: failed to delete excess backup", "id", b.ID, logging.KeyErr, err)
				}
			}
		}
	}

	return nil
}

// --- Helper functions ---

func checksumFile(path string) (string, int64) {
	f, err := os.Open(path) //nolint:gosec // path is an internally-derived backup file path, not user input
	if err != nil {
		return "", 0
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	size, _ := io.Copy(h, f) //nolint:errcheck // hashing a file; hash.Hash.Write never errors
	return hex.EncodeToString(h.Sum(nil)), size
}

func checksumDir(dir string) string {
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error { //nolint:errcheck // best-effort directory checksum; non-fatal
		if err != nil || d.IsDir() {
			return err
		}
		f, err := os.Open(path) //nolint:gosec // path comes from WalkDir over an internal backup dir, not user input
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, _ = io.Copy(h, f) //nolint:errcheck // hash.Hash.Write never returns an error
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func copyDir(src, dst string) (fileCount int, totalSize int64, err error) {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return 0, 0, err
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return 0, 0, nil
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path) //nolint:errcheck // path is always under src; cannot fail here
		dstPath := filepath.Join(dst, relPath)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir over an internal source dir, not user input
		if err != nil {
			return err
		}
		fileCount++
		totalSize += int64(len(data))
		return os.WriteFile(dstPath, data, 0o600) //nolint:gosec // dstPath is filepath.Join(dst, relPath) from WalkDir over an internal dir, not user input
	})
	return fileCount, totalSize, err
}

func createTarGz(archivePath, baseDir, dirName string) error {
	f, err := os.Create(archivePath) //nolint:gosec // archivePath is an internally-derived backup destination under s.backupPath, not user input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	srcDir := filepath.Join(baseDir, dirName)
	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(baseDir, path) //nolint:errcheck // path is always under baseDir; cannot fail here
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		file, err := os.Open(path) //nolint:gosec // path comes from WalkDir over an internal source dir, not user input
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		_, err = io.Copy(tw, file)
		return err
	}); err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gw.Close()
}

func readManifestFromArchive(archivePath string) (*Manifest, error) {
	f, err := os.Open(archivePath) //nolint:gosec // archivePath is from an internal backup-dir listing or validated by GetBackupPath, not user input
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("manifest.json not found in archive")
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "manifest.json" {
			var m Manifest
			if err := json.NewDecoder(tr).Decode(&m); err != nil {
				return nil, fmt.Errorf("decode manifest: %w", err)
			}
			return &m, nil
		}
	}
}

func verifyArchiveChecksums(archivePath string, manifest *Manifest) error {
	tmpDir, err := os.MkdirTemp("", "nexorious-verify-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := ExtractTarGz(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extract for verification: %w", err)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("empty archive")
	}
	backupDir := filepath.Join(tmpDir, entries[0].Name())

	dbChecksum, _ := checksumFile(filepath.Join(backupDir, "database.sql"))
	expectedDB := strings.TrimPrefix(manifest.DatabaseChecksum, "sha256:")
	if dbChecksum != expectedDB {
		return fmt.Errorf("database.sql checksum mismatch: got %s, expected %s", dbChecksum, expectedDB)
	}
	coverChecksum := checksumDir(filepath.Join(backupDir, "cover_art"))
	expectedCover := strings.TrimPrefix(manifest.CoverArtChecksum, "sha256:")
	if coverChecksum != expectedCover {
		return fmt.Errorf("cover_art checksum mismatch: got %s, expected %s", coverChecksum, expectedCover)
	}
	return nil
}

// maxDecompressedBytes caps the total uncompressed size ExtractTarGz will write,
// guarding against decompression-bomb archives whose small compressed form
// expands to exhaust disk. It is a var so tests can lower it.
var maxDecompressedBytes int64 = 20 << 30 // 20 GiB

// ExtractTarGz extracts a .tar.gz archive to destDir. It rejects entries whose
// path would escape destDir and aborts if the total decompressed size exceeds
// maxDecompressedBytes.
func ExtractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath) //nolint:gosec // archivePath is validated against traversal by GetBackupPath / the setup handler, or is an internal temp file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	var totalWritten int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name) //nolint:gosec // guarded immediately below: rejected unless the cleaned target stays within destDir
		cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("invalid tar path: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			outFile, err := os.Create(target) //nolint:gosec // target validated against destDir above
			if err != nil {
				return err
			}
			// Bound the copy to the remaining budget (+1 to detect overflow),
			// so a decompression bomb cannot write past maxDecompressedBytes.
			n, err := io.Copy(outFile, io.LimitReader(tr, maxDecompressedBytes-totalWritten+1))
			totalWritten += n
			if err != nil {
				_ = outFile.Close()
				return err
			}
			if err := outFile.Close(); err != nil {
				return err
			}
			if totalWritten > maxDecompressedBytes {
				return fmt.Errorf("archive exceeds maximum decompressed size of %d bytes", maxDecompressedBytes)
			}
		}
	}
}

// RestoreOpts holds callbacks for restore orchestration.
type RestoreOpts struct {
	SkipPreRestore  bool
	SetMaintenance  func(bool)
	ShutdownPool    func()
	StopScheduler   func()
	CloseDB         func() error
	ReconnectDB     func() (*bun.DB, error)
	RebuildServices func(db *bun.DB) error
	ReinitMigrator  func(db *bun.DB) error
	SetAppState     func(state string)
	MaxMigration    string
}

// RestoreBackup restores from a stored backup archive.
func (s *Service) RestoreBackup(backupID string, opts RestoreOpts) error {
	if !s.mu.TryLock() {
		return ErrOperationInProgress
	}
	defer s.mu.Unlock()

	archivePath, err := s.GetBackupPath(backupID)
	if err != nil {
		return err
	}
	return s.doRestore(archivePath, backupID, opts)
}

// RestoreFromUpload validates an uploaded archive, moves it to the backup dir,
// then restores it. Returns the backup ID assigned to the uploaded archive.
func (s *Service) RestoreFromUpload(uploadedPath string, opts RestoreOpts) (string, error) {
	if !s.mu.TryLock() {
		return "", ErrOperationInProgress
	}
	defer s.mu.Unlock()

	_, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
	if err != nil {
		return "", fmt.Errorf("validate uploaded archive: %w", err)
	}

	id := fmt.Sprintf("nexorious-backup-%s", time.Now().UTC().Format("20060102-150405"))
	destPath := filepath.Join(s.backupPath, id+".tar.gz")
	if err := os.MkdirAll(s.backupPath, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	if err := os.Rename(uploadedPath, destPath); err != nil {
		if err := copyFile(uploadedPath, destPath); err != nil {
			return "", fmt.Errorf("move uploaded archive: %w", err)
		}
		_ = os.Remove(uploadedPath)
	}

	return id, s.doRestore(destPath, id, opts)
}

// RestoreFromArchive restores from an archive that already lives at its final
// location (typically inside the configured backup directory) without renaming
// or moving it. Unlike RestoreFromUpload — which "promotes" a temp upload to
// a timestamped name inside the backup dir — this method preserves the input
// path. Used by the setup-zone disk-restore handler so the operator's curated
// on-disk backup is not mutated by the restore operation.
//
// Returns a derived backup ID (the archive's filename without the .tar.gz
// suffix) and any error from the underlying restore.
func (s *Service) RestoreFromArchive(archivePath string, opts RestoreOpts) (string, error) {
	if !s.mu.TryLock() {
		return "", ErrOperationInProgress
	}
	defer s.mu.Unlock()

	if _, err := s.ValidateArchive(archivePath, true, opts.MaxMigration); err != nil {
		return "", fmt.Errorf("validate archive: %w", err)
	}

	// Derive an ID from the filename for logging/return value. doRestore reads
	// directly from archivePath, so the file stays where it is.
	id := strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz")
	return id, s.doRestore(archivePath, id, opts)
}

// applyRestoreFromDir applies an already-extracted backup directory to the live
// database: it terminates existing connections, recreates the public schema,
// loads the SQL dump, restores cover art, then reconnects the pool and rebuilds
// dependent services. On success s.db points at the new pool.
//
// Cover-art copy failures are best-effort (logged, not returned): the database
// is the source of truth and is fully restored before cover art, which is
// re-derivable, so a missing image must not undo an otherwise-good restore.
//
// Errors from the DB-critical steps are returned with the failing step named in
// the wrap; each caller decides how to react — the forward restore path rolls
// back via handleRestoreFailure, while the rollback path escalates to
// db_unavailable.
func (s *Service) applyRestoreFromDir(extractedDir string, conn DBConnParams, opts RestoreOpts) error {
	terminateCmd := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid();"
	if err := RunPsqlCommand(conn, terminateCmd); err != nil {
		return fmt.Errorf("terminate connections: %w", err)
	}

	if err := RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return fmt.Errorf("drop/recreate schema: %w", err)
	}

	sqlFile := filepath.Join(extractedDir, "database.sql")
	if err := RunPsqlFile(conn, sqlFile); err != nil {
		return fmt.Errorf("psql restore: %w", err)
	}

	coverArtSrc := filepath.Join(extractedDir, "cover_art")
	coverArtDst := filepath.Join(s.storagePath, "cover_art")
	if err := os.RemoveAll(coverArtDst); err != nil {
		slog.Warn("restore: failed to remove old cover_art", logging.KeyErr, err)
	}
	if _, err := os.Stat(coverArtSrc); err == nil {
		if _, _, err := copyDir(coverArtSrc, coverArtDst); err != nil {
			slog.Warn("restore: failed to restore cover art (best-effort)", logging.KeyErr, err)
		}
	}

	newDB, err := opts.ReconnectDB()
	if err != nil {
		return fmt.Errorf("reconnect DB: %w", err)
	}
	s.db = newDB

	if err := opts.RebuildServices(newDB); err != nil {
		slog.Error("restore: rebuild services", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	if err := opts.ReinitMigrator(newDB); err != nil {
		slog.Error("restore: reinit migrator", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	return nil
}

func (s *Service) doRestore(archivePath, backupID string, opts RestoreOpts) error {
	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return fmt.Errorf("restore: parse DB URL: %w", err)
	}

	_, err = s.ValidateArchive(archivePath, false, opts.MaxMigration)
	if err != nil {
		return fmt.Errorf("restore: validate: %w", err)
	}

	opts.SetMaintenance(true)
	opts.ShutdownPool()
	opts.StopScheduler()

	var preRestoreID string
	if !opts.SkipPreRestore && PgDumpAvailable() {
		s.mu.Unlock()
		pid, err := s.CreateBackup("pre_restore")
		s.mu.Lock()
		if err != nil {
			slog.Error("restore: failed to create pre-restore backup", logging.KeyErr, err)
		} else {
			preRestoreID = pid
		}
	}

	if err := opts.CloseDB(); err != nil {
		slog.Error("restore: close DB", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}

	tmpDir, err := os.MkdirTemp("", "nexorious-restore-*")
	if err != nil {
		return s.handleRestoreFailure(fmt.Errorf("create temp dir: %w", err), preRestoreID, conn, opts)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := ExtractTarGz(archivePath, tmpDir); err != nil {
		return s.handleRestoreFailure(fmt.Errorf("extract archive: %w", err), preRestoreID, conn, opts)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		return s.handleRestoreFailure(fmt.Errorf("empty or unreadable archive"), preRestoreID, conn, opts)
	}
	extractedDir := filepath.Join(tmpDir, entries[0].Name())

	if err := s.applyRestoreFromDir(extractedDir, conn, opts); err != nil {
		return s.handleRestoreFailure(err, preRestoreID, conn, opts)
	}

	opts.SetMaintenance(false)

	slog.Info("restore completed", "backup_id", backupID)
	return nil
}

func (s *Service) handleRestoreFailure(originalErr error, preRestoreID string, conn DBConnParams, opts RestoreOpts) error {
	slog.Error("restore failed", logging.KeyErr, originalErr)

	if preRestoreID == "" {
		slog.Error("restore failed with no pre-restore backup — database may be inconsistent. Manual intervention required.",
			logging.KeyErr, originalErr)
		opts.SetAppState("db_unavailable")
		return originalErr
	}

	slog.Warn("attempting rollback to pre-restore backup", "pre_restore_id", preRestoreID)

	archivePath, err := s.GetBackupPath(preRestoreID)
	if err != nil {
		slog.Error("rollback failed: invalid pre-restore backup id", logging.KeyErr, err, "original_err", originalErr)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed (invalid pre-restore id). Original: %w. Rollback: %v", originalErr, err)
	}
	tmpDir, err := os.MkdirTemp("", "nexorious-rollback-*")
	if err != nil {
		slog.Error("rollback failed: create temp dir", logging.KeyErr, err, "original_err", originalErr)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v", originalErr, err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := ExtractTarGz(archivePath, tmpDir); err != nil {
		slog.Error("rollback failed: extract archive", logging.KeyErr, err, "original_err", originalErr)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v", originalErr, err)
	}

	entries, _ := os.ReadDir(tmpDir) //nolint:errcheck // already in restore-failure path; empty result handled below
	if len(entries) == 0 {
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed (empty archive). Original: %w", originalErr)
	}
	extractedDir := filepath.Join(tmpDir, entries[0].Name())

	if err := s.applyRestoreFromDir(extractedDir, conn, opts); err != nil {
		slog.Error("FATAL: rollback restore also failed", "rollback_err", err, "original_err", originalErr,
			"pre_restore_path", archivePath)
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed. Original: %w. Rollback: %v. Pre-restore backup at: %s",
			originalErr, err, archivePath)
	}

	opts.SetMaintenance(false)
	slog.Warn("restore failed but successfully rolled back", "original_err", originalErr, "pre_restore_id", preRestoreID)
	return fmt.Errorf("restore failed: %w (rolled back to pre-restore backup)", originalErr)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src/dst are internally-derived cover-art paths, not user input
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // src/dst are internally-derived cover-art paths, not user input
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
