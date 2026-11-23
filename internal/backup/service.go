package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
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

// IncompatibleBackupError indicates a backup cannot be restored by this binary
// because of a version/compatibility mismatch — too old to adopt (predates
// v0.17.1), created by a newer Nexorious, or an unknown archive format. Its
// message is user-facing: restore handlers surface it verbatim with a 400 rather
// than a generic "restore failed", because the operator can act on it (upgrade
// the source, use a different backup, or export/import). Distinct from genuine
// corruption/internal errors, which stay opaque 500s.
type IncompatibleBackupError struct{ Msg string }

func (e *IncompatibleBackupError) Error() string { return e.Msg }

// Service handles backup create/list/delete/validate/restore operations.
//
// mu is the service lock. A backup, a deletion, a retention run and a change of
// the backup schedule each hold it while they run; a restore takes it and, once
// it has written the restore marker, never releases it: the process restarts
// instead (see restore).
type Service struct {
	db          *bun.DB
	databaseURL string
	backupPath  string
	storagePath string
	appVersion  string
	mu          sync.Mutex

	hooks        RestoreHooks
	maxMigration string
	minMigration string
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

// SetMigrationWindow sets the range of migration versions a backup may have to
// be restorable by this binary: not newer than maxMigration, not older than
// minMigration. Empty means unbounded.
func (s *Service) SetMigrationWindow(maxMigration, minMigration string) {
	s.maxMigration = maxMigration
	s.minMigration = minMigration
}

// MigrationWindow returns what SetMigrationWindow set.
func (s *Service) MigrationWindow() (maxMigration, minMigration string) {
	return s.maxMigration, s.minMigration
}

// Exclusive runs fn under the service lock, or returns ErrOperationInProgress
// without running it when a backup, restore or deletion holds the lock. A
// change of the backup schedule goes through it, so none is made during a
// restore.
func (s *Service) Exclusive(fn func() error) error {
	if !s.mu.TryLock() {
		return ErrOperationInProgress
	}
	defer s.mu.Unlock()
	return fn()
}

// CreateBackup creates a backup archive and returns the backup ID.
// backupType is "manual" or "scheduled".
func (s *Service) CreateBackup(backupType string) (string, error) {
	if !s.mu.TryLock() {
		return "", ErrOperationInProgress
	}
	defer s.mu.Unlock()
	return s.createBackupLocked(backupType)
}

// runPgDump is RunPgDump; tests replace it to make a backup fail.
var runPgDump = RunPgDump

// dumpTimeout scales the pg_dump deadline with the database size. The size
// counts indexes and dead rows, which a dump leaves out, so it errs on the
// generous side. When it can't be read, the dump gets the base timeout: a
// database that can't answer this query fails pg_dump quickly anyway.
func (s *Service) dumpTimeout() time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var size int64
	if err := s.db.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		slog.WarnContext(ctx, "backup: could not read the database size, using the base pg_dump timeout",
			logging.KeyErr, err, "timeout", toolTimeoutBase, logging.Cat(logging.CategoryDB))
		return toolTimeoutBase
	}
	return toolTimeout(size)
}

// createBackupLocked is CreateBackup for a caller that holds the lock: the
// restore makes its pre-restore backup with it.
func (s *Service) createBackupLocked(backupType string) (string, error) {
	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}

	id, err := newBackupID()
	if err != nil {
		return "", err
	}

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
	if err := runPgDump(conn, dbSQLPath, s.dumpTimeout()); err != nil {
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

// newBackupID returns a fresh backup id: the UTC time to the second plus a
// random suffix. Two backups made in the same second (an uploaded archive and
// its pre-restore backup, #1349) must never share an id, because the id names
// the archive file. Nothing parses the id; older archives without the suffix
// stay valid.
func newBackupID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate backup id: %w", err)
	}
	return fmt.Sprintf("nexorious-backup-%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:])), nil
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

// DeleteBackup removes a backup archive file. It is refused with
// ErrOperationInProgress while a backup or restore holds the lock, so the
// backup a restore rolls back to cannot disappear under it.
func (s *Service) DeleteBackup(backupID string) error {
	if !s.mu.TryLock() {
		return ErrOperationInProgress
	}
	defer s.mu.Unlock()
	return s.deleteBackupLocked(backupID)
}

func (s *Service) deleteBackupLocked(backupID string) error {
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
func (s *Service) ValidateArchive(archivePath string, verifyChecksums bool, maxMigrationVersion, minMigrationVersion string) (*Manifest, error) {
	manifest, err := readManifestFromArchive(archivePath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	if manifest.Version > MaxManifestVersion {
		return nil, &IncompatibleBackupError{Msg: fmt.Sprintf("unknown manifest version %d (max supported: %d)", manifest.Version, MaxManifestVersion)}
	}

	if maxMigrationVersion != "" && manifest.MigrationVersion > maxMigrationVersion {
		return nil, &IncompatibleBackupError{Msg: fmt.Sprintf(
			"backup was created by a newer version of Nexorious (migration %s); this binary only supports up to migration %s — upgrade before restoring",
			manifest.MigrationVersion, maxMigrationVersion,
		)}
	}

	// Migration floor: a backup older than the v0.17.1 stepping stone cannot be
	// adopted by this binary. Restoring it would replace the database with an
	// un-adoptable schema and strand the instance in the refuse state, so reject
	// it here — before any database mutation. The source must first be upgraded
	// to v0.17.1 (or use export/import).
	if minMigrationVersion != "" && manifest.MigrationVersion < minMigrationVersion {
		return nil, &IncompatibleBackupError{Msg: fmt.Sprintf(
			"backup predates v0.17.1 (migration %s); restore requires a backup at migration %s or later — upgrade the source instance to v0.17.1 first, or use export/import",
			manifest.MigrationVersion, minMigrationVersion,
		)}
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
func (s *Service) ListAvailableArchives(ctx context.Context, maxMigrationVersion, minMigrationVersion string) ([]ArchiveInfo, error) {
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
		case minMigrationVersion != "" && manifest.MigrationVersion < minMigrationVersion:
			info.Restorable = false
			info.Reason = fmt.Sprintf(
				"backup predates v0.17.1 (migration %s); upgrade the source instance to v0.17.1 before restoring, or use export/import",
				manifest.MigrationVersion,
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

// ApplyRetention deletes backups exceeding the retention policy. Like
// DeleteBackup it is refused with ErrOperationInProgress while the lock is
// held.
func (s *Service) ApplyRetention(retentionMode string, retentionValue int) error {
	if !s.mu.TryLock() {
		return ErrOperationInProgress
	}
	defer s.mu.Unlock()

	backups, err := s.ListBackups()
	if err != nil {
		return err
	}

	now := time.Now()

	for _, b := range backups {
		if b.BackupType == "pre_restore" && now.Sub(b.CreatedAt) > 7*24*time.Hour {
			if err := s.deleteBackupLocked(b.ID); err != nil {
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
				if err := s.deleteBackupLocked(b.ID); err != nil {
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
				if err := s.deleteBackupLocked(b.ID); err != nil {
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

// writeTarFile writes the regular file at srcPath into tw under the given
// archive name (tarName).
func writeTarFile(tw *tar.Writer, srcPath, tarName string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = tarName
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(srcPath) //nolint:gosec // srcPath is an internally-derived staged backup file, not user input
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = io.Copy(tw, file)
	return err
}

func createTarGz(archivePath, baseDir, dirName string) error {
	// O_EXCL: never overwrite an existing backup (#1349).
	f, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // archivePath is an internally-derived backup destination under s.backupPath, not user input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	srcDir := filepath.Join(baseDir, dirName)

	// Write manifest.json first so it is the leading entry in the (non-seekable)
	// gzip stream. readManifestFromArchive stops at the first manifest.json, so
	// listing a backup then decompresses only the tiny manifest instead of
	// advancing past the entire database.sql + cover art. See #983.
	manifestPath := filepath.Join(srcDir, "manifest.json")
	if err := writeTarFile(tw, manifestPath, filepath.Join(dirName, "manifest.json")); err != nil {
		return err
	}

	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == manifestPath {
			return nil // already written first; don't duplicate it
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
	return verifyExtracted(filepath.Join(tmpDir, entries[0].Name()), manifest)
}

// verifyExtracted checks an extracted backup directory against the checksums
// its manifest records.
func verifyExtracted(backupDir string, manifest *Manifest) error {
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

// moveNoReplace moves src to dst, refusing to overwrite an existing dst.
// A rename across filesystems (the upload staging dir is often on another one)
// falls back to an exclusive copy.
func moveNoReplace(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s: %w", dst, fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		slog.Warn("remove staged upload after copy", "path", src, logging.KeyErr, err)
	}
	return nil
}

// copyFile copies src to a new file dst. It never overwrites: an existing dst
// is an error. A partially written dst is removed.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is an internally-staged upload path, not user input
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // dst is an internally-derived path under the backup dir, not user input
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
