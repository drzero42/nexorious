package migrate_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// newTestHandler creates a Handler backed by a real migrator against a test DB.
func newTestHandler(t *testing.T) *migrate.Handler {
	t.Helper()
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	return migrate.NewHandler(m, nil)
}

func TestHandleStatus_NeedsMigration(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	h := migrate.NewHandler(m, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStatus(c); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body struct {
		PendingCount int    `json:"pending_count"`
		State        string `json:"state"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.State != "needs_migration" {
		t.Errorf("expected state=needs_migration, got %q", body.State)
	}
	direct, err := m.PendingCount()
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if body.PendingCount != direct {
		t.Errorf("HTTP pending_count (%d) != direct PendingCount (%d)", body.PendingCount, direct)
	}
	if body.PendingCount <= 0 {
		t.Errorf("expected positive pending count on fresh DB, got %d", body.PendingCount)
	}
}

func TestHandleRun_202_ThenReady(t *testing.T) {
	h := newTestHandler(t)
	e := echo.New()

	// POST /api/migrate/run — should return 202.
	req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleRun(c); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	// Wait for RunMigrations to start (goroutine may not have run yet).
	var ch <-chan string
	for ch == nil {
		runtime.Gosched()
		ch = h.Migrator().LogCh()
	}
	// Drain the log channel so the migration goroutine is never blocked on a
	// full buffer.
	for range ch {
	}

	// The channel closing only signals that the log stream is done; the state
	// flip to Ready happens afterwards in the HandleRun goroutine (after
	// RunMigrations returns). Poll for the terminal state rather than reading it
	// the instant the channel closes, which races that flip — a window the
	// race detector reliably loses.
	deadline := time.Now().Add(2 * time.Second)
	for h.Migrator().State() != migrate.AppStateReady && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.Migrator().State() != migrate.AppStateReady {
		t.Errorf("expected Ready after migration, got %v", h.Migrator().State())
	}
}

func TestHandleRun_409_WhenMigrating(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	// Manually set state to Migrating.
	m.SetStateForTest(migrate.AppStateMigrating)

	h := migrate.NewHandler(m, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleRun(c); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestHandleRun_400_WhenReady(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	m.SetStateForTest(migrate.AppStateReady)

	h := migrate.NewHandler(m, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleRun(c); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleProgress_409_BeforeRun(t *testing.T) {
	h := newTestHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/progress", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleProgress(c); err != nil {
		t.Fatalf("HandleProgress: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 when logCh is nil, got %d", rec.Code)
	}
}

func TestHandleProgress_SSE_CompletionEvent(t *testing.T) {
	h := newTestHandler(t)
	e := echo.New()

	// Trigger migration to populate logCh.
	runReq := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	runRec := httptest.NewRecorder()
	runCtx := e.NewContext(runReq, runRec)
	if err := h.HandleRun(runCtx); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}

	// Wait for RunMigrations goroutine to create the log channel.
	for h.Migrator().LogCh() == nil {
		runtime.Gosched()
	}

	// Read SSE stream.
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/progress", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleProgress(c); err != nil {
		t.Fatalf("HandleProgress: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: complete") {
		t.Errorf("expected 'event: complete' in SSE response, got:\n%s", body)
	}
}

func TestHandleRun_MigrationFailure_StateAndStatus(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// Close the underlying *sql.DB so RunMigrations fails inside the goroutine.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}
	h := migrate.NewHandler(m, db)

	e := echo.New()
	runReq := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	runRec := httptest.NewRecorder()
	if err := h.HandleRun(e.NewContext(runReq, runRec)); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if runRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", runRec.Code)
	}

	// Wait up to 2s for the goroutine to transition to MigrationFailed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.State() == migrate.AppStateMigrationFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.State() != migrate.AppStateMigrationFailed {
		t.Fatalf("state = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty after failed run")
	}

	// Verify /api/migrate/status reflects the failure.
	statusReq := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	statusRec := httptest.NewRecorder()
	if err := h.HandleStatus(e.NewContext(statusReq, statusRec)); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	var body struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(statusRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.State != "migration_failed" {
		t.Errorf("state = %q, want migration_failed", body.State)
	}
	if body.Error == "" {
		t.Errorf("error field is empty in status payload")
	}
}

func TestHandleStatus_MigrationFailedIncludesError(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	m.TransitionToFailed(errors.New("boom: schema is haunted"))

	h := migrate.NewHandler(m, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStatus(c); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body struct {
		PendingCount int    `json:"pending_count"`
		State        string `json:"state"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.State != "migration_failed" {
		t.Errorf("state = %q, want migration_failed", body.State)
	}
	if body.Error != "boom: schema is haunted" {
		t.Errorf("error = %q, want %q", body.Error, "boom: schema is haunted")
	}
}

func TestHandleRun_409_WhenRefused(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrationRefused)
	m.SetLastErrorForTest("schema predates baseline; manual upgrade required")
	h := migrate.NewHandler(m, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleRun(c); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error == "" {
		t.Errorf("expected non-empty error message in response body")
	}
}

func TestHandleStatus_MigrationRefusedIncludesError(t *testing.T) {
	const refusedMsg = "schema predates baseline; manual upgrade required"
	m := migrate.NewMigratorForTest(migrate.AppStateMigrationRefused)
	m.SetLastErrorForTest(refusedMsg)
	h := migrate.NewHandler(m, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStatus(c); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body struct {
		PendingCount int    `json:"pending_count"`
		State        string `json:"state"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.State != "migration_refused" {
		t.Errorf("state = %q, want migration_refused", body.State)
	}
	if body.Error != refusedMsg {
		t.Errorf("error = %q, want %q", body.Error, refusedMsg)
	}
	if body.PendingCount != 0 {
		t.Errorf("pending_count = %d, want 0", body.PendingCount)
	}
}

func TestHandleStatus_NeedsAdopt_PendingCountOne(t *testing.T) {
	// Set up a DB in adopt state: apply baseline schema, then rewrite
	// bun_migrations to the exact v0.17.1 manifest (23 rows, no baseline row).
	resetPublicSchema(t)
	db := makeAdoptBunDB(t)
	seedV0171(t, db)

	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if m.State() != migrate.AppStateNeedsAdopt {
		t.Fatalf("setup error: state = %v, want NeedsAdopt", m.State())
	}

	h := migrate.NewHandler(m, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStatus(c); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body struct {
		PendingCount int    `json:"pending_count"`
		State        string `json:"state"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.State != "needs_adopt" {
		t.Errorf("state = %q, want needs_adopt", body.State)
	}
	if body.PendingCount != 1 {
		t.Errorf("pending_count = %d, want 1", body.PendingCount)
	}
}
