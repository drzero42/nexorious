package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/restoremarker"
)

// A restore runs in the order #1341 lays down. Before anything is stopped it
// takes the service lock, validates and extracts the archive, and writes the
// restore marker; a failure there refuses the restore and changes nothing. Once
// the marker is written the restore never releases the lock or leaves
// maintenance mode: it always ends by asking the server to restart itself
// (RestoreHooks.Restart), whatever happened. The marker says what the next
// start finds: "cleanup" (the database is whole; work that was in flight is
// cleared before River starts) or "restoring" (public may be half-loaded; the
// server stays in the restore-failed state until an operator steps in).

// Errors that refuse a restore before anything was stopped.
var (
	// ErrPgDumpUnavailable: a pre-restore backup is wanted and pg_dump is missing.
	ErrPgDumpUnavailable = errors.New("pg_dump is not available, so no pre-restore backup can be made; install the PostgreSQL client tools to restore")
	// ErrSetupNotPending: a setup-zone restore found users in the database.
	ErrSetupNotPending = errors.New("restore during setup is only available when no users exist")
	// ErrSetupCheckFailed: a setup-zone restore could not count the users.
	ErrSetupCheckFailed = errors.New("could not verify that setup is still pending")
)

// ErrBackgroundStillRunning: background work did not stop within the limit.
// The database is untouched, but the server restarts.
var ErrBackgroundStillRunning = errors.New("background work did not stop in time; nothing was restored")

// stopTimeout bounds how long a restore waits for River to stop.
const stopTimeout = 30 * time.Second

// Server-side limits on dropping the public schema. They are shorter than the
// 60 s after which RunPsqlCommand gives up, because killing psql does not
// cancel a DROP that waits for a lock: it could commit later, after the server
// has gone back to normal.
const dropPublicSQL = "SET lock_timeout = '20s'; SET statement_timeout = '45s'; " +
	"DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;"

// terminateSQL ends every other session on the database, so nothing holds a
// lock the drop needs.
const terminateSQL = "SELECT pg_terminate_backend(pid) FROM pg_stat_activity " +
	"WHERE datname = current_database() AND pid <> pg_backend_pid();"

// RestoreHooks connect a restore to the rest of the server. Unset hooks do
// nothing.
type RestoreHooks struct {
	// SetMaintenance switches maintenance mode on. Nothing switches it off:
	// the process restarts.
	SetMaintenance func()
	// StopBackground switches off the River start gate and the database probe
	// for good, then stops River, cancelling running work. It returns an error
	// when that has not finished by the deadline of ctx.
	StopBackground func(ctx context.Context) error
	// DropIdleConns drops the server pool's idle connections, which the
	// restore has just terminated.
	DropIdleConns func()
	// ReinitMigrator brings the restored database up to date.
	ReinitMigrator func(ctx context.Context) error
	// Restart asks the server to restart itself once the response has been
	// written. It must not block.
	Restart func()
}

// SetRestoreHooks wires a restore to the server. Call it once, before serving.
func (s *Service) SetRestoreHooks(h RestoreHooks) {
	s.hooks = h
}

func (s *Service) setMaintenance() {
	if s.hooks.SetMaintenance != nil {
		s.hooks.SetMaintenance()
	}
}

func (s *Service) stopBackground(ctx context.Context) error {
	if s.hooks.StopBackground == nil {
		return nil
	}
	return s.hooks.StopBackground(ctx)
}

func (s *Service) dropIdleConns() {
	if s.hooks.DropIdleConns != nil {
		s.hooks.DropIdleConns()
	}
}

func (s *Service) reinitMigrator(ctx context.Context) error {
	if s.hooks.ReinitMigrator == nil {
		return nil
	}
	return s.hooks.ReinitMigrator(ctx)
}

func (s *Service) restart() {
	if s.hooks.Restart != nil {
		s.hooks.Restart()
	}
}

// RestoreOpts says where a restore comes from.
type RestoreOpts struct {
	// SetupZone marks a restore from the setup screen of a fresh install. It
	// makes no pre-restore backup; the restore establishes itself that no
	// users exist, and on a failed load leaves an empty database behind.
	SetupZone bool
}

// RestoreResult describes a restore that got past the point of no return or
// finished. A zero Restarting means the restore was refused and nothing
// changed.
type RestoreResult struct {
	// BackupID is the backup being restored (for an upload, the id it was
	// given in the backup directory).
	BackupID string
	// PreRestoreBackupID is the backup taken before the database was replaced.
	PreRestoreBackupID string
	// Restarting is true once the restore marker was written: the server
	// restarts after the response, on success and on failure.
	Restarting bool
	// RolledBack is true when the load failed and the pre-restore backup was
	// loaded back.
	RolledBack bool
	// MigrationError is set when the restored database could not be migrated.
	// The restore stands; the next start offers the migration again.
	MigrationError error
}

// RestoreBackup restores the stored backup backupID.
func (s *Service) RestoreBackup(backupID string, opts RestoreOpts) (RestoreResult, error) {
	archivePath, err := s.GetBackupPath(backupID)
	if err != nil {
		return RestoreResult{}, err
	}
	return s.restore(archivePath, backupID, false, opts)
}

// RestoreFromUpload restores an uploaded archive. The archive is moved into the
// backup directory, under a new id, only once the restore has passed the point
// where it can still be refused; a refused upload is left where it was.
func (s *Service) RestoreFromUpload(uploadedPath string, opts RestoreOpts) (RestoreResult, error) {
	return s.restore(uploadedPath, "", true, opts)
}

// RestoreFromArchive restores an archive that already lives at its final
// location, without renaming or moving it. The setup-zone disk restore uses it
// so the operator's curated on-disk backup is not mutated. The backup id is
// the file name without .tar.gz.
func (s *Service) RestoreFromArchive(archivePath string, opts RestoreOpts) (RestoreResult, error) {
	id := strings.TrimSuffix(filepath.Base(archivePath), ".tar.gz")
	return s.restore(archivePath, id, false, opts)
}

// restore runs a restore. archivePath is read; backupID names it in the marker;
// upload moves it into the backup directory under a new id.
func (s *Service) restore(archivePath, backupID string, upload bool, opts RestoreOpts) (res RestoreResult, err error) {
	if !s.mu.TryLock() {
		return res, ErrOperationInProgress
	}
	// Released only when the restore is refused before the marker is written.
	// From then on the process restarts holding it, so no backup, deletion or
	// second restore can start before the process is replaced.
	pastPointOfNoReturn := false
	defer func() {
		if !pastPointOfNoReturn {
			s.mu.Unlock()
		}
	}()
	ctx := context.Background()

	// 1. Validate the archive and extract it, verifying its checksums.
	conn, err := ParseDatabaseURL(s.databaseURL)
	if err != nil {
		return res, fmt.Errorf("restore: %w", err)
	}
	if _, err := os.Stat(archivePath); errors.Is(err, fs.ErrNotExist) {
		return res, fmt.Errorf("restore %s: %w", backupID, ErrNotFound)
	}
	manifest, err := s.ValidateArchive(archivePath, false, s.maxMigration, s.minMigration)
	if err != nil {
		return res, fmt.Errorf("restore: validate: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "nexorious-restore-*")
	if err != nil {
		return res, fmt.Errorf("restore: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	extracted, err := extractBackup(archivePath, filepath.Join(tmpDir, "restore"))
	if err != nil {
		return res, fmt.Errorf("restore: %w", err)
	}
	if err := verifyExtracted(extracted, manifest); err != nil {
		return res, fmt.Errorf("restore: checksum verification failed: %w", err)
	}

	// A setup-zone restore is unauthenticated: establish here, under the lock,
	// that there are no users. A count that fails counts as "users exist".
	if opts.SetupZone {
		var users int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
			slog.ErrorContext(ctx, "restore: count users", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			return res, ErrSetupCheckFailed
		}
		if users > 0 {
			return res, ErrSetupNotPending
		}
	}

	// 2. Every restore except from the setup zone needs a pre-restore backup.
	if !opts.SetupZone && !PgDumpAvailable() {
		return res, ErrPgDumpUnavailable
	}

	if upload {
		if backupID, err = newBackupID(); err != nil {
			return res, err
		}
	}

	// 3. Write the marker. A database user that may not create the schema
	// cannot restore.
	marker := restoremarker.Marker{Phase: restoremarker.PhaseCleanup, BackupID: backupID}
	if err := restoremarker.Set(ctx, s.db, marker); err != nil {
		return res, fmt.Errorf("restore refused: %w", err)
	}

	// From here on the process always ends by restarting itself, also after a
	// panic (deferred calls run while it unwinds).
	pastPointOfNoReturn = true
	res.Restarting = true
	res.BackupID = backupID
	defer s.restart()
	slog.InfoContext(ctx, "restore started", "backup_id", backupID, "setup_zone", opts.SetupZone)

	if upload {
		if err := os.MkdirAll(s.backupPath, 0o750); err != nil {
			return res, fmt.Errorf("create backup dir: %w", err)
		}
		if err := moveNoReplace(archivePath, filepath.Join(s.backupPath, backupID+".tar.gz")); err != nil {
			return res, fmt.Errorf("move uploaded archive: %w", err)
		}
	}

	// 4. Maintenance mode.
	s.setMaintenance()

	// 5. Stop the start gate, the probe and River. No graceful drain.
	stopCtx, cancel := context.WithTimeout(ctx, stopTimeout)
	err = s.stopBackground(stopCtx)
	cancel()
	if err != nil {
		return res, fmt.Errorf("%w: %v", ErrBackgroundStillRunning, err)
	}

	// 6. Pre-restore backup.
	if !opts.SetupZone {
		pid, err := s.createBackupLocked("pre_restore")
		if err != nil {
			return res, fmt.Errorf("pre-restore backup failed, nothing was restored: %w", err)
		}
		res.PreRestoreBackupID = pid
		marker.PreRestoreBackupID = pid
		if err := restoremarker.Set(ctx, s.db, marker); err != nil {
			return res, fmt.Errorf("nothing was restored: %w", err)
		}
	}

	// 7a. Terminate the other connections, and drop the idle ones of our own
	// pool: they were just killed.
	if err := RunPsqlCommand(conn, terminateSQL); err != nil {
		return res, fmt.Errorf("terminate connections, nothing was restored: %w", err)
	}
	s.dropIdleConns()

	// 7b. From here until a load succeeds, public may be half-loaded.
	marker.Phase = restoremarker.PhaseRestoring
	if err := restoremarker.Set(ctx, s.db, marker); err != nil {
		return res, fmt.Errorf("nothing was restored: %w", err)
	}

	// 7c. Drop and recreate public.
	var loadErr error
	if err := RunPsqlCommand(conn, dropPublicSQL); err != nil {
		intact, checkErr := s.publicIntact(ctx)
		switch {
		case checkErr != nil:
			slog.ErrorContext(ctx, "restore: dropping the schema failed and whether it is still there cannot be checked; the restore marker stays at restoring",
				logging.KeyErr, err, "check_err", checkErr, logging.Cat(logging.CategoryDB))
			return res, fmt.Errorf("drop schema: %w", err)
		case intact:
			marker.Phase = restoremarker.PhaseCleanup
			if setErr := s.setMarkerRetrying(ctx, marker); setErr != nil {
				slog.ErrorContext(ctx, "restore: the schema was not dropped, but the restore marker could not be set back to cleanup",
					logging.KeyErr, setErr, logging.Cat(logging.CategoryDB))
			}
			return res, fmt.Errorf("drop schema, nothing was restored: %w", err)
		default:
			loadErr = fmt.Errorf("drop schema: %w", err)
		}
	}

	// 7d, 7e. Load the dump and mark the database whole.
	if loadErr == nil {
		loadErr = s.loadAndSettle(ctx, conn, extracted, marker)
	}
	if loadErr == nil {
		// 8. Migrate. A failure does not undo the restore.
		res.MigrationError = s.reinitMigrator(ctx)
		if res.MigrationError != nil {
			slog.ErrorContext(ctx, "restore: migrating the restored database failed; the next start offers the migration again",
				logging.KeyErr, res.MigrationError, logging.Cat(logging.CategoryDB))
		}
		slog.InfoContext(ctx, "restore completed", "backup_id", backupID)
		return res, nil
	}

	// 9. The load failed.
	slog.ErrorContext(ctx, "restore: loading the backup failed", logging.KeyErr, loadErr, logging.Cat(logging.CategoryDB))
	switch {
	case res.PreRestoreBackupID != "":
		if err := s.rollback(ctx, conn, res.PreRestoreBackupID, tmpDir, marker, &res); err != nil {
			slog.ErrorContext(ctx, "restore: rolling back to the pre-restore backup failed; the restore marker stays at restoring",
				logging.KeyErr, err, "pre_restore_backup_id", res.PreRestoreBackupID, logging.Cat(logging.CategoryDB))
			return res, fmt.Errorf("restore failed: %w; rolling back to pre-restore backup %s failed too: %v", loadErr, res.PreRestoreBackupID, err)
		}
		res.RolledBack = true
		return res, fmt.Errorf("restore failed: %w (rolled back to pre-restore backup %s)", loadErr, res.PreRestoreBackupID)
	case opts.SetupZone:
		if err := s.resetToEmpty(ctx, conn, marker, &res); err != nil {
			slog.ErrorContext(ctx, "restore: emptying the database after a failed setup restore failed; the restore marker stays at restoring",
				logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			return res, fmt.Errorf("restore failed: %w; emptying the database failed too: %v", loadErr, err)
		}
		return res, fmt.Errorf("restore failed: %w (the database was left empty)", loadErr)
	default:
		return res, fmt.Errorf("restore failed with nothing to roll back to: %w", loadErr)
	}
}

// loadAndSettle loads the dump in dir into the freshly created public schema,
// replaces the cover art, and sets the marker to cleanup (7d and 7e).
//
// Cover-art copy failures are logged, not returned: the database is the source
// of truth, and a missing image must not undo an otherwise good restore.
func (s *Service) loadAndSettle(ctx context.Context, conn DBConnParams, dir string, marker restoremarker.Marker) error {
	if err := RunPsqlRestore(conn, filepath.Join(dir, "database.sql")); err != nil {
		return fmt.Errorf("psql restore: %w", err)
	}
	coverArtSrc := filepath.Join(dir, "cover_art")
	coverArtDst := filepath.Join(s.storagePath, "cover_art")
	if err := os.RemoveAll(coverArtDst); err != nil {
		slog.WarnContext(ctx, "restore: failed to remove old cover_art", logging.KeyErr, err)
	}
	if _, err := os.Stat(coverArtSrc); err == nil {
		if _, _, err := copyDir(coverArtSrc, coverArtDst); err != nil {
			slog.WarnContext(ctx, "restore: failed to restore cover art (best-effort)", logging.KeyErr, err)
		}
	}
	s.dropIdleConns()

	marker.Phase = restoremarker.PhaseCleanup
	if err := s.setMarkerRetrying(ctx, marker); err != nil {
		return fmt.Errorf("mark the restored database whole: %w", err)
	}
	return nil
}

// rollback loads the pre-restore backup back (7c to 7e and 8 with it). Any
// failure before the marker is back at cleanup leaves it at restoring.
func (s *Service) rollback(ctx context.Context, conn DBConnParams, preRestoreID, tmpDir string, marker restoremarker.Marker, res *RestoreResult) error {
	slog.WarnContext(ctx, "restore: rolling back to the pre-restore backup", "pre_restore_backup_id", preRestoreID)
	archivePath, err := s.GetBackupPath(preRestoreID)
	if err != nil {
		return err
	}
	extracted, err := extractBackup(archivePath, filepath.Join(tmpDir, "rollback"))
	if err != nil {
		return err
	}
	if err := RunPsqlCommand(conn, dropPublicSQL); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	if err := s.loadAndSettle(ctx, conn, extracted, marker); err != nil {
		return err
	}
	res.MigrationError = s.reinitMigrator(ctx)
	return nil
}

// resetToEmpty leaves an empty, migrated database after a failed setup-zone
// restore, so the server comes back at the setup screen. The database was
// empty to begin with, so nothing is lost.
//
// The marker goes to cleanup before the migration, not after: while it says
// restoring, the migrator refuses to touch the database. An empty schema is a
// whole database; if the migration fails, the next start offers it again.
func (s *Service) resetToEmpty(ctx context.Context, conn DBConnParams, marker restoremarker.Marker, res *RestoreResult) error {
	if err := RunPsqlCommand(conn, dropPublicSQL); err != nil {
		return fmt.Errorf("drop schema: %w", err)
	}
	s.dropIdleConns()
	marker.Phase = restoremarker.PhaseCleanup
	if err := s.setMarkerRetrying(ctx, marker); err != nil {
		return err
	}
	res.MigrationError = s.reinitMigrator(ctx)
	return nil
}

// setMarkerRetrying writes the marker, retrying after dropping idle
// connections: it is the write that turns a good load into a good restore.
func (s *Service) setMarkerRetrying(ctx context.Context, m restoremarker.Marker) error {
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			s.dropIdleConns()
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		if err = restoremarker.Set(ctx, s.db, m); err == nil {
			return nil
		}
		slog.WarnContext(ctx, "restore: writing the restore marker failed", "attempt", attempt+1, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	return err
}

// publicIntact reports whether the public schema still holds the app's tables
// after a drop reported an error.
func (s *Service) publicIntact(ctx context.Context) (bool, error) {
	var present bool
	err := s.db.QueryRowContext(ctx, "SELECT to_regclass('public.bun_migrations') IS NOT NULL").Scan(&present)
	return present, err
}

// extractBackup extracts archivePath into destDir and returns the backup
// directory inside it.
func extractBackup(archivePath, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}
	if err := ExtractTarGz(archivePath, destDir); err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return "", fmt.Errorf("read extracted archive: %w", err)
	}
	if len(entries) == 0 {
		return "", errors.New("empty archive")
	}
	return filepath.Join(destDir, entries[0].Name()), nil
}
