package migrate_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)

// ---------------------------------------------------------------------------
// AppState.String()
// ---------------------------------------------------------------------------

func TestAppState_String(t *testing.T) {
	cases := []struct {
		state migrate.AppState
		want  string
	}{
		{migrate.AppStateDBUnavailable, "db_unavailable"},
		{migrate.AppStateNeedsMigration, "needs_migration"},
		{migrate.AppStateMigrating, "migrating"},
		{migrate.AppStateReady, "ready"},
		{migrate.AppStateMigrationFailed, "migration_failed"},
		{migrate.AppState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("AppState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// SetLogWriter — exercises the logWriter branch in sendLog
// ---------------------------------------------------------------------------

func TestRunMigrations_SetLogWriter(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	var buf bytes.Buffer
	m.SetLogWriter(&buf)

	ctx := context.Background()
	if err := m.RunMigrations(ctx); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected log output written to logWriter, got empty buffer")
	}
}

// ---------------------------------------------------------------------------
// RunMigrations — AlreadyMigrating branch
// ---------------------------------------------------------------------------

func TestRunMigrations_AlreadyMigrating(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	m.SetStateForTest(migrate.AppStateMigrating)

	err := m.RunMigrations(context.Background())
	if err == nil {
		t.Fatal("expected error when already migrating, got nil")
	}
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

func TestStatus_FreshDB(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)

	ctx := context.Background()
	pending, current, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if pending <= 0 {
		t.Errorf("expected positive pending count on fresh DB, got %d", pending)
	}
	if current != "none" {
		t.Errorf("expected current=none, got %q", current)
	}
}

func TestStatus_AfterMigration(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	ctx := context.Background()
	pending, current, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if pending != 0 {
		t.Errorf("expected 0 pending after migration, got %d", pending)
	}
	if current == "none" {
		t.Error("expected a current migration name, got 'none'")
	}
}

// ---------------------------------------------------------------------------
// HandleMigrateUI
// ---------------------------------------------------------------------------

func TestHandleMigrateUI(t *testing.T) {
	h := newTestHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/migrate", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleMigrateUI(c); err != nil {
		t.Fatalf("HandleMigrateUI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty HTML body from HandleMigrateUI")
	}
}

// ---------------------------------------------------------------------------
// HandleRun — with db set (exercises InitNeedsSetup call path)
// ---------------------------------------------------------------------------

func TestHandleRun_WithDB_202(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// Pass db so HandleRun will call InitNeedsSetup after migration.
	h := migrate.NewHandler(m, db)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleRun(c); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	// Drain the log channel so the goroutine finishes.
	for h.Migrator().LogCh() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	for range h.Migrator().LogCh() {
	}
}

// ---------------------------------------------------------------------------
// StartDBProbe — recovery path (ping fails then succeeds)
// ---------------------------------------------------------------------------

func TestStartDBProbe_RecoveryFromUnavailable(t *testing.T) {
	// Set up a good DB and run migrations so the state is Ready.
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	m.SetStateForTest(migrate.AppStateDBUnavailable)
	m.SetProbeIntervalForTest(30 * time.Millisecond)

	onRecoveryCalled := false
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Probe with the good DB — the probe should detect recovery.
	m.StartDBProbe(ctx, db, func(_ context.Context) error {
		onRecoveryCalled = true
		return nil
	})

	// Give the goroutine time to fire and recover.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Either the onRecovery callback was invoked or the state changed.
	// The probe transitions from DBUnavailable via recoverFromUnavailable.
	if m.State() == migrate.AppStateDBUnavailable && !onRecoveryCalled {
		t.Error("expected recovery from DBUnavailable but state is still DBUnavailable")
	}
}
