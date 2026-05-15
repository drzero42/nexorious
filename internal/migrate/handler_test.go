package migrate_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	migrate "github.com/drzero42/nexorious-go/internal/migrate"
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
	h := newTestHandler(t)
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
	if body.PendingCount != 7 {
		t.Errorf("expected pending_count=7, got %d", body.PendingCount)
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
	// Wait for migration to finish.
	for range ch {
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
