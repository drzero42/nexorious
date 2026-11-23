package backup_test

// Tests for the restore sequence of #1341: what is refused before anything is
// stopped, the marker phases, the rollback, and that every restore past the
// marker keeps the lock and ends in a restart.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/backup"
	nexdb "github.com/drzero42/nexorious/internal/db"
	"github.com/drzero42/nexorious/internal/restoremarker"
)

// restoreRecorder records the hooks a restore calls, in order.
type restoreRecorder struct {
	mu     sync.Mutex
	events []string

	stopErr   error                   // returned by StopBackground
	onStop    func()                  // runs inside StopBackground
	onReinit  func()                  // runs inside ReinitMigrator
	reinitErr error                   // returned by ReinitMigrator
	markers   []*restoremarker.Marker // the marker as ReinitMigrator saw it
}

func (r *restoreRecorder) record(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *restoreRecorder) has(e string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.events {
		if got == e {
			return true
		}
	}
	return false
}

func (r *restoreRecorder) restarted() bool { return r.has("restart") }

func (r *restoreRecorder) hooks() backup.RestoreHooks {
	return backup.RestoreHooks{
		SetMaintenance: func() { r.record("maintenance") },
		StopBackground: func(context.Context) error {
			r.record("stop")
			if r.onStop != nil {
				r.onStop()
			}
			return r.stopErr
		},
		DropIdleConns: func() { nexdb.DropIdleConns(testDB) },
		ReinitMigrator: func(ctx context.Context) error {
			r.record("reinit")
			m, err := restoremarker.Read(ctx, testDB)
			if err != nil {
				return err
			}
			r.markers = append(r.markers, m)
			if r.onReinit != nil {
				r.onReinit()
			}
			return r.reinitErr
		},
		Restart: func() { r.record("restart") },
	}
}

// resetRestoreMarker removes the marker schema, before the test and after it.
func resetRestoreMarker(t *testing.T) {
	t.Helper()
	drop := func() {
		if _, err := testDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS nexorious_restore CASCADE"); err != nil {
			t.Fatalf("drop marker schema: %v", err)
		}
	}
	drop()
	t.Cleanup(drop)
}

func readMarker(t *testing.T) *restoremarker.Marker {
	t.Helper()
	m, err := restoremarker.Read(context.Background(), testDB)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return m
}

// restoreFixture is a database with one user, "in-backup", in a backup, and a
// second user, "live", added after it.
type restoreFixture struct {
	backupDir, storageDir string
	backupID              string
}

func newRestoreFixture(t *testing.T) restoreFixture {
	t.Helper()
	requirePsqlTools(t)
	truncateAllTables(t)
	f := restoreFixture{backupDir: t.TempDir(), storageDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(f.storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}
	insertUser(t, "in-backup")
	id, err := backup.NewService(testDB, testDSN, f.backupDir, f.storageDir, "0.1.0").CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	f.backupID = id
	insertUser(t, "live")
	return f
}

// assertUntouched checks that the live data is still there.
func assertUntouched(t *testing.T) {
	t.Helper()
	if got := userIDs(t); strings.Join(got, ",") != "in-backup,live" {
		t.Errorf("users = %v, want the live database untouched", got)
	}
}

// assertLockHeld checks that the restore kept the service lock: no backup,
// deletion, retention run or schedule change can start.
func assertLockHeld(t *testing.T, svc *backup.Service, someID string) {
	t.Helper()
	if _, err := svc.CreateBackup("manual"); !errors.Is(err, backup.ErrOperationInProgress) {
		t.Errorf("CreateBackup during a restore: %v, want ErrOperationInProgress", err)
	}
	if err := svc.DeleteBackup(someID); !errors.Is(err, backup.ErrOperationInProgress) {
		t.Errorf("DeleteBackup during a restore: %v, want ErrOperationInProgress", err)
	}
	if err := svc.ApplyRetention("count", 1); !errors.Is(err, backup.ErrOperationInProgress) {
		t.Errorf("ApplyRetention during a restore: %v, want ErrOperationInProgress", err)
	}
	if err := svc.Exclusive(func() error { return nil }); !errors.Is(err, backup.ErrOperationInProgress) {
		t.Errorf("Exclusive during a restore: %v, want ErrOperationInProgress", err)
	}
	if _, err := svc.RestoreBackup(someID, backup.RestoreOpts{}); !errors.Is(err, backup.ErrOperationInProgress) {
		t.Errorf("second restore: %v, want ErrOperationInProgress", err)
	}
}

func TestRestore_Success(t *testing.T) {
	f := newRestoreFixture(t)
	svc, rec := newRestoreService(t, f.backupDir, f.storageDir)

	// The marker says restoring while the dump loads.
	var phaseDuringLoad restoremarker.Phase
	orig := backup.RunPsqlRestore
	t.Cleanup(func() { backup.RunPsqlRestore = orig })
	backup.RunPsqlRestore = func(conn backup.DBConnParams, path string) error {
		if m := readMarker(t); m != nil {
			phaseDuringLoad = m.Phase
		}
		return orig(conn, path)
	}
	// Requests made while background work is stopped are refused.
	rec.onStop = func() { assertLockHeld(t, svc, f.backupID) }

	res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if got := userIDs(t); strings.Join(got, ",") != "in-backup" {
		t.Errorf("users after restore = %v, want [in-backup]", got)
	}
	if !res.Restarting || res.BackupID != f.backupID || res.PreRestoreBackupID == "" {
		t.Errorf("result = %+v", res)
	}
	if want := "maintenance,stop,reinit,restart"; strings.Join(rec.events, ",") != want {
		t.Errorf("hooks = %v, want %s", rec.events, want)
	}
	if phaseDuringLoad != restoremarker.PhaseRestoring {
		t.Errorf("marker during the load = %q, want restoring", phaseDuringLoad)
	}
	if len(rec.markers) != 1 || rec.markers[0] == nil || rec.markers[0].Phase != restoremarker.PhaseCleanup ||
		rec.markers[0].BackupID != f.backupID || rec.markers[0].PreRestoreBackupID != res.PreRestoreBackupID {
		t.Errorf("marker while migrating = %+v, want cleanup naming both backups", rec.markers)
	}
	assertLockHeld(t, svc, f.backupID)
}

// A migration that fails after a good load does not undo the restore.
func TestRestore_MigrationFailureKeepsRestore(t *testing.T) {
	f := newRestoreFixture(t)
	svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
	rec.reinitErr = errors.New("migration failed")

	res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if res.MigrationError == nil {
		t.Error("MigrationError not reported")
	}
	if m := readMarker(t); m == nil || m.Phase != restoremarker.PhaseCleanup {
		t.Errorf("marker = %+v, want cleanup", m)
	}
}

// writeDamagedArchive writes an archive whose database.sql does not match the
// checksum in its manifest.
func writeDamagedArchive(t *testing.T, dir string) string {
	t.Helper()
	manifest, err := json.Marshal(map[string]any{
		"version": 1, "created_at": "2026-01-01T00:00:00Z", "migration_version": "20260101000001",
		"backup_type": "manual", "database_file": "database.sql",
		"database_checksum": "sha256:0000", "cover_art_checksum": "sha256:0000",
	})
	if err != nil {
		t.Fatal(err)
	}
	return createTarGzWithFiles(t, dir, map[string]string{
		"b/manifest.json": string(manifest),
		"b/database.sql":  "SELECT 1;\n",
	})
}

// Refusals before the marker change nothing, stop nothing, restart nothing and
// release the lock.
func TestRestore_RefusedBeforeMarker(t *testing.T) {
	requirePsqlTools(t)

	assertRefused := func(t *testing.T, svc *backup.Service, rec *restoreRecorder, res backup.RestoreResult, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("restore succeeded, want it refused")
		}
		if res.Restarting || len(rec.events) != 0 {
			t.Errorf("refused restore: restarting=%v hooks=%v, want neither", res.Restarting, rec.events)
		}
		if m := readMarker(t); m != nil {
			t.Errorf("marker after a refusal = %+v, want none", m)
		}
		if err := svc.Exclusive(func() error { return nil }); err != nil {
			t.Errorf("lock after a refusal: %v, want released", err)
		}
	}

	t.Run("damaged upload", func(t *testing.T) {
		truncateAllTables(t)
		backupDir := t.TempDir()
		svc, rec := newRestoreService(t, backupDir, t.TempDir())
		upload := writeDamagedArchive(t, t.TempDir())
		res, err := svc.RestoreFromUpload(upload, backup.RestoreOpts{})
		assertRefused(t, svc, rec, res, err)
		if !errors.Is(err, backup.ErrDamagedArchive) || !strings.Contains(err.Error(), "checksum") {
			t.Errorf("error = %v, want ErrDamagedArchive for a checksum failure", err)
		}
		if entries, _ := os.ReadDir(backupDir); len(entries) != 0 {
			t.Errorf("backup dir after a refused upload = %v, want empty", entries)
		}
	})

	t.Run("pg_dump missing", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		backup.SetPgDumpAvailableForTest(t, false)
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
		assertRefused(t, svc, rec, res, err)
		if !errors.Is(err, backup.ErrPgDumpUnavailable) {
			t.Errorf("error = %v, want ErrPgDumpUnavailable", err)
		}
		assertUntouched(t)
	})

	t.Run("marker cannot be written", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		// A table the marker write cannot use.
		if _, err := testDB.ExecContext(context.Background(),
			"CREATE SCHEMA nexorious_restore; CREATE TABLE nexorious_restore.marker (x int)"); err != nil {
			t.Fatal(err)
		}
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
		if err == nil || res.Restarting || len(rec.events) != 0 {
			t.Fatalf("restore = %+v, %v, hooks %v; want refused without hooks", res, err, rec.events)
		}
		if err := svc.Exclusive(func() error { return nil }); err != nil {
			t.Errorf("lock after a refusal: %v, want released", err)
		}
		assertUntouched(t)
	})

	t.Run("setup zone with users", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{SetupZone: true})
		assertRefused(t, svc, rec, res, err)
		if !errors.Is(err, backup.ErrSetupNotPending) {
			t.Errorf("error = %v, want ErrSetupNotPending", err)
		}
		assertUntouched(t)
	})
}

// Failures after the marker but before the drop leave the database untouched
// and the marker at cleanup, keep the lock, and end in a restart.
func TestRestore_FailsBeforeDrop(t *testing.T) {
	assertCleanRestart := func(t *testing.T, svc *backup.Service, rec *restoreRecorder, f restoreFixture, res backup.RestoreResult, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("restore succeeded, want a failure")
		}
		if !res.Restarting || !rec.restarted() {
			t.Errorf("restarting=%v hooks=%v, want a restart", res.Restarting, rec.events)
		}
		if rec.has("reinit") {
			t.Error("the migrator ran although nothing was restored")
		}
		if m := readMarker(t); m == nil || m.Phase != restoremarker.PhaseCleanup {
			t.Errorf("marker = %+v, want cleanup", m)
		}
		assertUntouched(t)
		assertLockHeld(t, svc, f.backupID)
	}

	t.Run("River does not stop", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		rec.stopErr = context.DeadlineExceeded
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
		if !errors.Is(err, backup.ErrBackgroundStillRunning) {
			t.Errorf("error = %v, want ErrBackgroundStillRunning", err)
		}
		if res.PreRestoreBackupID != "" {
			t.Error("a pre-restore backup was made although River had not stopped")
		}
		assertCleanRestart(t, svc, rec, f, res, err)
	})

	t.Run("pre-restore backup fails", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		backup.SetRunPgDumpForTest(t, func(backup.DBConnParams, string) error { return errors.New("pg_dump: timed out") })
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
		assertCleanRestart(t, svc, rec, f, res, err)
	})

	t.Run("drop fails with public intact", func(t *testing.T) {
		f := newRestoreFixture(t)
		svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
		orig := backup.RunPsqlCommand
		t.Cleanup(func() { backup.RunPsqlCommand = orig })
		backup.RunPsqlCommand = func(conn backup.DBConnParams, cmd string) error {
			if strings.Contains(cmd, "DROP SCHEMA") {
				return errors.New("must be owner of schema public")
			}
			return orig(conn, cmd)
		}
		res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
		if res.RolledBack {
			t.Error("rolled back although nothing was dropped")
		}
		assertCleanRestart(t, svc, rec, f, res, err)
	})
}

// fakeDrops makes RunPsqlCommand pretend to drop public (so the shared
// database survives) and fail the drops listed in failAt (1-based). Other
// commands run for real.
func fakeDrops(t *testing.T, failAt ...int) *int {
	t.Helper()
	orig := backup.RunPsqlCommand
	t.Cleanup(func() { backup.RunPsqlCommand = orig })
	drops := new(int)
	backup.RunPsqlCommand = func(conn backup.DBConnParams, cmd string) error {
		if !strings.Contains(cmd, "DROP SCHEMA") {
			return orig(conn, cmd)
		}
		*drops++
		for _, n := range failAt {
			if *drops == n {
				return errors.New("simulated drop failure")
			}
		}
		return nil
	}
	return drops
}

func failLoads(t *testing.T) {
	t.Helper()
	orig := backup.RunPsqlRestore
	t.Cleanup(func() { backup.RunPsqlRestore = orig })
	backup.RunPsqlRestore = func(backup.DBConnParams, string) error { return errors.New("simulated load failure") }
}

// A rollback that fails leaves the marker at restoring.
func TestRestore_RollbackDropFailsLeavesRestoring(t *testing.T) {
	f := newRestoreFixture(t)
	svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
	drops := fakeDrops(t, 2) // the forward drop "works", the rollback's fails
	failLoads(t)

	res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
	if err == nil || res.RolledBack {
		t.Fatalf("restore = %+v, %v; want a failed rollback", res, err)
	}
	if *drops != 2 {
		t.Errorf("drops = %d, want 2 (forward and rollback)", *drops)
	}
	if m := readMarker(t); m == nil || m.Phase != restoremarker.PhaseRestoring || m.PreRestoreBackupID != res.PreRestoreBackupID {
		t.Errorf("marker = %+v, want restoring naming the pre-restore backup", m)
	}
	if !rec.restarted() || rec.has("reinit") {
		t.Errorf("hooks = %v, want a restart and no migration", rec.events)
	}
}

// A failed load from the setup zone leaves an empty, migrated database with the
// marker at cleanup, so the server comes back at the setup screen.
func TestRestore_SetupZoneFailedLoadLeavesEmptyDatabase(t *testing.T) {
	f := newRestoreFixture(t)
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
	drops := fakeDrops(t)
	failLoads(t)

	res, err := svc.RestoreBackup(f.backupID, backup.RestoreOpts{SetupZone: true})
	if err == nil {
		t.Fatal("restore succeeded, want the load failure reported")
	}
	if res.PreRestoreBackupID != "" {
		t.Error("a setup-zone restore made a pre-restore backup")
	}
	if *drops != 2 {
		t.Errorf("drops = %d, want 2 (forward and reset to empty)", *drops)
	}
	if len(rec.markers) != 1 || rec.markers[0] == nil || rec.markers[0].Phase != restoremarker.PhaseCleanup {
		t.Errorf("marker while migrating the empty schema = %+v, want cleanup", rec.markers)
	}
	if !rec.restarted() {
		t.Error("no restart")
	}
}

// A panic after the marker still ends in a restart and keeps the lock.
func TestRestore_PanicStillRestarts(t *testing.T) {
	f := newRestoreFixture(t)
	svc, rec := newRestoreService(t, f.backupDir, f.storageDir)
	rec.onStop = func() { panic("boom") }

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic did not reach the caller")
			}
		}()
		_, _ = svc.RestoreBackup(f.backupID, backup.RestoreOpts{})
	}()
	if !rec.restarted() {
		t.Error("no restart after a panic")
	}
	assertLockHeld(t, svc, f.backupID)
}

// A drop that waits for a lock gives up on the server side, before psql's own
// limit, and does not commit when the lock is released afterwards: killing psql
// alone would not cancel it (#1341).
func TestDropPublic_LockWaitTimesOutAndNeverCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the server-side lock timeout")
	}
	conn := requirePsqlTools(t)
	truncateAllTables(t)
	insertUser(t, "survivor")
	ctx := context.Background()

	// Another session holds a lock the drop needs.
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "LOCK TABLE users IN ACCESS SHARE MODE"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err = backup.RunPsqlCommand(conn, backup.DropPublicSQL)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("drop succeeded while a lock was held")
	}
	if !strings.Contains(err.Error(), "lock timeout") {
		t.Errorf("error = %v, want the server's lock timeout", err)
	}
	if elapsed >= 60*time.Second {
		t.Errorf("drop gave up after %s, want before psql's 60 s limit", elapsed)
	}

	// Release the lock; nothing may be left waiting to commit.
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	var waiting int
	if err := testDB.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_stat_activity WHERE query LIKE '%DROP SCHEMA%' AND pid <> pg_backend_pid()").Scan(&waiting); err != nil {
		t.Fatal(err)
	}
	if waiting != 0 {
		t.Errorf("%d sessions still running the drop", waiting)
	}
	if got := userIDs(t); len(got) != 1 || got[0] != "survivor" {
		t.Errorf("users = %v, want the schema untouched", got)
	}
}
