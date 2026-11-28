package migrate_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	migrate "github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/restoremarker"
)

// migratedDB returns a pristine database with every migration applied.
func migratedDB(t *testing.T) *bun.DB {
	t.Helper()
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db
}

func setMarker(t *testing.T, db *bun.DB, phase restoremarker.Phase) {
	t.Helper()
	if err := restoremarker.Set(context.Background(), db, restoremarker.Marker{
		Phase: phase, BackupID: "backup-1", PreRestoreBackupID: "pre-1",
	}); err != nil {
		t.Fatalf("set marker: %v", err)
	}
}

func markerPhase(t *testing.T, db *bun.DB) string {
	t.Helper()
	m, err := restoremarker.Read(context.Background(), db)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if m == nil {
		return ""
	}
	return string(m.Phase)
}

func tableExists(t *testing.T, db *bun.DB, name string) bool {
	t.Helper()
	var reg sql.NullString
	if err := db.QueryRowContext(context.Background(), `SELECT to_regclass(?)::text`, name).Scan(&reg); err != nil {
		t.Fatalf("to_regclass(%s): %v", name, err)
	}
	return reg.Valid
}

func count(t *testing.T, db *bun.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// A marker at restoring means the public schema may be half-loaded: the state
// is restore-failed and nothing is created or migrated, not even bun's own
// tables (#1341).
func TestDetermineState_RestoringMarkerTouchesNothing(t *testing.T) {
	db := setupTestDB(t)
	setMarker(t, db, restoremarker.PhaseRestoring)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateRestoreFailed {
		t.Fatalf("state = %v, want restore_failed", m.State())
	}
	if got := m.RestoreMarker(); got == nil || got.BackupID != "backup-1" || got.PreRestoreBackupID != "pre-1" {
		t.Errorf("RestoreMarker = %+v, want the row's backup ids", got)
	}

	// migrate status, the lazy PendingCount and a migration run must not
	// create anything either.
	if _, _, err := m.Status(context.Background()); err != nil {
		t.Errorf("Status: %v", err)
	}
	if _, err := m.PendingCount(); err == nil {
		t.Error("PendingCount: want an error in restore_failed")
	}
	if err := m.RunMigrations(context.Background()); err == nil {
		t.Error("RunMigrations: want a refusal in restore_failed")
	}
	if tableExists(t, db, "public.bun_migrations") {
		t.Error("bun_migrations was created in a database marked restoring")
	}
}

// A marker written after the state was computed still stops a migration run,
// before anything is migrated.
func TestRunMigrations_RefusesWhenMarkerAppears(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsMigration {
		t.Fatalf("state = %v, want needs_migration", m.State())
	}
	setMarker(t, db, restoremarker.PhaseRestoring)

	if err := m.RunMigrations(context.Background()); err == nil {
		t.Fatal("RunMigrations: want a refusal")
	}
	if m.State() != migrate.AppStateRestoreFailed {
		t.Errorf("state = %v, want restore_failed", m.State())
	}
	if tableExists(t, db, "public.users") {
		t.Error("migrations ran in a database marked restoring")
	}
}

// A phase this version does not know fails closed.
func TestDetermineState_UnknownPhaseIsRestoreFailed(t *testing.T) {
	db := migratedDB(t)
	setMarker(t, db, "some_future_phase")

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateRestoreFailed {
		t.Errorf("state = %v, want restore_failed", m.State())
	}
}

// The probe comes back to restore-failed after a database outage, however
// often the database goes away (#1341).
func TestStartDBProbe_RecoversToRestoreFailed(t *testing.T) {
	db := migratedDB(t)
	setMarker(t, db, restoremarker.PhaseRestoring)

	for _, prev := range []migrate.AppState{migrate.AppStateRestoreFailed, migrate.AppStateReady} {
		m := migrate.NewMigrator(db)
		m.SetPrevStateForTest(prev)
		m.SetStateForTest(migrate.AppStateDBUnavailable)
		m.SetProbeIntervalForTest(20 * time.Millisecond)
		ctx, cancel := context.WithCancel(t.Context())
		m.StartDBProbe(ctx, db, func(context.Context) error { return m.DetermineState() })

		deadline := time.Now().Add(2 * time.Second)
		for m.State() == migrate.AppStateDBUnavailable && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		if m.State() != migrate.AppStateRestoreFailed {
			t.Errorf("prev=%v: state after recovery = %v, want restore_failed", prev, m.State())
		}
	}
}

// seedInFlight creates what a restore leaves behind: queued River jobs, a River
// leader, and Jobs in every status with items in every status.
func seedInFlight(t *testing.T, db *bun.DB) {
	t.Helper()
	ctx := context.Background()
	stmts := []string{
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1', 'admin', 'hash', true)`,
		`INSERT INTO jobs (id, user_id, job_type, source, status) VALUES
			('j-pending', 'u1', 'import', 'nexorious', 'pending'),
			('j-processing', 'u1', 'sync', 'steam', 'processing'),
			('j-done', 'u1', 'export', 'nexorious', 'completed')`,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status) VALUES
			('i-p-pending', 'j-pending', 'u1', 'a', 'A', 'pending'),
			('i-p-review', 'j-pending', 'u1', 'b', 'B', 'pending_review'),
			('i-p-done', 'j-pending', 'u1', 'c', 'C', 'completed'),
			('i-r-pending', 'j-processing', 'u1', 'd', 'D', 'pending'),
			('i-r-failed', 'j-processing', 'u1', 'e', 'E', 'failed'),
			('i-d-pending', 'j-done', 'u1', 'f', 'F', 'pending')`,
		`INSERT INTO river_job (args, kind, max_attempts, queue, state)
			SELECT '{}', 'restore_test', 1, 'default', 'available' FROM generate_series(1, 3)`,
		`INSERT INTO river_leader (leader_id, elected_at, expires_at)
			VALUES ('restore-test', now(), now() + interval '1 hour')`,
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
}

func jobStatus(t *testing.T, db *bun.DB, id string) (status string, msg sql.NullString, completed bool) {
	t.Helper()
	var completedAt sql.NullTime
	if err := db.QueryRowContext(context.Background(),
		`SELECT status, error_message, completed_at FROM jobs WHERE id = ?`, id).Scan(&status, &msg, &completedAt); err != nil {
		t.Fatalf("job %s: %v", id, err)
	}
	return status, msg, completedAt.Valid
}

func itemStatus(t *testing.T, db *bun.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(),
		`SELECT status FROM job_items WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("item %s: %v", id, err)
	}
	return status
}

func assertCleared(t *testing.T, db *bun.DB) {
	t.Helper()
	for _, table := range []string{"river_job", "river_leader"} {
		if n := count(t, db, "SELECT count(*) FROM "+table); n != 0 {
			t.Errorf("%s has %d rows after the clear, want 0", table, n)
		}
	}
	for _, id := range []string{"j-pending", "j-processing"} {
		status, msg, completed := jobStatus(t, db, id)
		if status != "failed" || msg.String != restoremarker.InterruptedMessage || !completed {
			t.Errorf("job %s = (%s, %q, completed=%v), want failed with %q and completed_at set",
				id, status, msg.String, completed, restoremarker.InterruptedMessage)
		}
	}
	if status, _, _ := jobStatus(t, db, "j-done"); status != "completed" {
		t.Errorf("finished job j-done = %s, want completed (untouched)", status)
	}
	want := map[string]string{
		"i-p-pending": "cancelled",
		"i-p-review":  "cancelled",
		"i-p-done":    "completed",
		"i-r-pending": "cancelled",
		"i-r-failed":  "failed",
		"i-d-pending": "pending", // belongs to a finished Job
	}
	for id, w := range want {
		if got := itemStatus(t, db, id); got != w {
			t.Errorf("item %s = %s, want %s", id, got, w)
		}
	}
	if phase := markerPhase(t, db); phase != "" {
		t.Errorf("marker still present (phase %q) after the clear", phase)
	}
}

// A marker at cleanup is cleared before Ready is stored: the queue is emptied,
// active Jobs are failed, their unfinished items cancelled, and the marker row
// deleted, and the River job id sequence is not reset (#1341).
func TestDetermineState_CleanupClearsBeforeReady(t *testing.T) {
	db := migratedDB(t)
	seedInFlight(t, db)
	maxID := count(t, db, `SELECT max(id) FROM river_job`)
	setMarker(t, db, restoremarker.PhaseCleanup)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateReady {
		t.Fatalf("state = %v, want ready", m.State())
	}
	assertCleared(t, db)

	var next int
	if err := db.QueryRowContext(context.Background(),
		`INSERT INTO river_job (args, kind, max_attempts, queue, state)
		 VALUES ('{}', 'restore_test', 1, 'default', 'available') RETURNING id`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next <= maxID {
		t.Errorf("river job id after the clear = %d, want > %d (sequence was reset)", next, maxID)
	}
}

// The clear runs only when the state would be Ready: with migrations pending it
// waits, and runs once the migration page has migrated (HandleRun), before
// Ready is reported.
func TestHandleRun_ClearsAfterMigratingBeforeReady(t *testing.T) {
	db := migratedDB(t)
	seedInFlight(t, db)
	setMarker(t, db, restoremarker.PhaseCleanup)
	// Make a River migration pending again, as a restored older backup would.
	if _, err := db.ExecContext(context.Background(),
		`DELETE FROM river_migration WHERE version = (SELECT max(version) FROM river_migration)`); err != nil {
		t.Fatal(err)
	}

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsMigration {
		t.Fatalf("state = %v, want needs_migration", m.State())
	}
	if phase := markerPhase(t, db); phase != string(restoremarker.PhaseCleanup) {
		t.Fatalf("marker phase = %q before migrating, want cleanup (no clear yet)", phase)
	}
	if n := count(t, db, `SELECT count(*) FROM river_job`); n == 0 {
		t.Fatal("queue emptied before the state could be Ready")
	}

	h := migrate.NewHandler(m, db)
	rec := httptest.NewRecorder()
	if err := h.HandleRun(echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil), rec)); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("HandleRun = %d, want 202", rec.Code)
	}
	var ch <-chan string
	for ch == nil {
		runtime.Gosched()
		ch = m.LogCh()
	}
	for range ch {
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.State() != migrate.AppStateReady && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.State() != migrate.AppStateReady {
		t.Fatalf("state = %v, want ready", m.State())
	}
	assertCleared(t, db)
}

// A clear that fails leaves the state unavailable with the reason, changes
// nothing, and is tried again on the next computation.
func TestDetermineState_FailingClearIsUnavailable(t *testing.T) {
	db := migratedDB(t)
	seedInFlight(t, db)
	setMarker(t, db, restoremarker.PhaseCleanup)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE FUNCTION fail_jobs_update() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN RAISE EXCEPTION 'jobs are read-only'; END $$;
		CREATE TRIGGER fail_jobs_update BEFORE UPDATE ON jobs
			FOR EACH ROW EXECUTE FUNCTION fail_jobs_update()`); err != nil {
		t.Fatal(err)
	}

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err == nil {
		t.Fatal("DetermineState: want an error from the failing clear")
	}
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("state = %v, want db_unavailable", m.State())
	}
	if !strings.Contains(m.UnavailableReason(), "jobs are read-only") {
		t.Errorf("UnavailableReason = %q, want the clear's error", m.UnavailableReason())
	}
	if n := count(t, db, `SELECT count(*) FROM river_job`); n != 3 {
		t.Errorf("river_job has %d rows, want 3 (the clear must roll back)", n)
	}
	if phase := markerPhase(t, db); phase != string(restoremarker.PhaseCleanup) {
		t.Errorf("marker phase = %q, want cleanup", phase)
	}

	if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_jobs_update ON jobs`); err != nil {
		t.Fatal(err)
	}
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState on retry: %v", err)
	}
	if m.State() != migrate.AppStateReady || m.UnavailableReason() != "" {
		t.Errorf("after retry: state = %v, reason = %q; want ready and no reason", m.State(), m.UnavailableReason())
	}
	assertCleared(t, db)
}

// A marker that cannot be read is "database unavailable", never "no marker".
func TestDetermineState_UnreadableMarkerIsUnavailable(t *testing.T) {
	db := migratedDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.DetermineState(); err == nil {
		t.Fatal("DetermineState on a closed database: want an error")
	}
	if m.State() != migrate.AppStateDBUnavailable {
		t.Errorf("state = %v, want db_unavailable", m.State())
	}
}
