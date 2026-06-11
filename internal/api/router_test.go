package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/config"
	maint "github.com/drzero42/nexorious/internal/middleware"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/observability"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

func TestAppStateMiddleware_RedirectsToMigrate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/some/page", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/migrate" {
		t.Errorf("expected redirect to /migrate, got %q", loc)
	}
}

func TestAppStateMiddleware_ApiReturnsJSON503(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for /api path, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("expected no redirect for /api path, got Location %q", loc)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["app_state"] != "needs_migration" {
		t.Errorf("expected app_state=needs_migration, got %q", body["app_state"])
	}
}

func TestDBUnavailable_ApiReturnsJSON503(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for /api path, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("expected no redirect for /api path, got Location %q", loc)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["app_state"] != "db_unavailable" {
		t.Errorf("expected app_state=db_unavailable, got %q", body["app_state"])
	}
}

func TestSetupGate_ApiReturnsJSON503(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	m.SetNeedsSetup(true)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for /api path, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("expected no redirect for /api path, got Location %q", loc)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["app_state"] != "needs_setup" {
		t.Errorf("expected app_state=needs_setup, got %q", body["app_state"])
	}
}

func TestAppStateMiddleware_BypassMigrationPaths(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for bypass path, got 302")
	}
}

func TestAppStateMiddleware_ReadyStatePassesThrough(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for ready state, got 302")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestDBUnavailable_RedirectsToErrorPage(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
	req := httptest.NewRequest(http.MethodGet, "/some/page", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/db-error") {
		t.Errorf("expected redirect to /db-error, got %q", loc)
	}
}

func TestDBUnavailable_EncodesFromParam(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
	req := httptest.NewRequest(http.MethodGet, "/user-games?page=2&sort=title", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "from=") {
		t.Errorf("expected encoded from param in Location, got %q", loc)
	}
}

func TestSetupGate_RedirectsArbitraryRoutes(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
	req := httptest.NewRequest(http.MethodGet, "/some/page", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Errorf("expected redirect to /setup, got %q", loc)
	}
}

func TestSetupGate_BypassesHealthEndpoint(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestSetupGate_BypassesMigrateRoutes(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	// Should not redirect to /setup (migrate routes are bypassed).
	if loc := rec.Header().Get("Location"); loc == "/setup" {
		t.Errorf("migrate route should not redirect to /setup")
	}
}

// brandIconPaths are referenced from the static migrate and setup HTML pages
// (and auto-requested by browsers); they must pass through both Gate 2
// (AppStateNeedsMigration) and Gate 3 (NeedsSetup) without redirecting.
var brandIconPaths = []string{
	"/logo.svg",
	"/favicon.svg",
	"/favicon.ico",
	"/apple-touch-icon.png",
}

func TestMigrationGate_BypassesBrandIconAssets(t *testing.T) {
	for _, path := range brandIconPaths {
		t.Run(path, func(t *testing.T) {
			m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
			e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown", nil)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if loc := rec.Header().Get("Location"); loc == "/migrate" {
				t.Errorf("brand asset %s should not redirect to /migrate", path)
			}
		})
	}
}

func TestSetupGate_BypassesBrandIconAssets(t *testing.T) {
	for _, path := range brandIconPaths {
		t.Run(path, func(t *testing.T) {
			migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
			migrator.SetNeedsSetup(true)
			e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if loc := rec.Header().Get("Location"); loc == "/setup" {
				t.Errorf("brand asset %s should not redirect to /setup", path)
			}
		})
	}
}

func TestHealth_Status(t *testing.T) {
	tests := []struct {
		name       string
		state      migrate.AppState
		needsSetup bool
		wantStatus string
	}{
		{name: "ready", state: migrate.AppStateReady, wantStatus: "ok"},
		{name: "setup pending", state: migrate.AppStateReady, needsSetup: true, wantStatus: "ok"},
		{name: "db unavailable", state: migrate.AppStateDBUnavailable, wantStatus: "db_unavailable"},
		{name: "needs migration", state: migrate.AppStateNeedsMigration, wantStatus: "needs_migration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := migrate.NewMigratorForTest(tt.state)
			if tt.needsSetup {
				migrator.SetNeedsSetup(true)
			}
			e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown", nil)
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["status"] != tt.wantStatus {
				t.Errorf("expected status=%q, got %q", tt.wantStatus, body["status"])
			}
		})
	}
}

func TestHealth_ReportsIGDBStatusOk(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.IGDBClientID = "test-id"
	cfg.IGDBClientSecret = "test-secret"
	igdbClient := igdb.NewClient(cfg, ratelimit.NewLocal(100, 100))
	e := api.New(testEncrypter, cfg, migrator, nil, "", igdbClient, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["igdb_status"] != igdb.StatusOK {
		t.Errorf("igdb_status = %v; want %q", body["igdb_status"], igdb.StatusOK)
	}
}

func TestHealth_ReportsIGDBStatusNotConfigured(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	igdbClient := igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["igdb_status"] != igdb.StatusNotConfigured {
		t.Errorf("igdb_status = %v; want %q", body["igdb_status"], igdb.StatusNotConfigured)
	}
}

func TestHealth_ReportsIGDBStatusInvalidCredentials(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	igdbClient := igdb.NewInvalidCredentialsClient(ratelimit.NewLocal(100, 100))
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil, "dev", "unknown", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["igdb_status"] != igdb.StatusInvalidCredentials {
		t.Errorf("igdb_status = %v; want %q", body["igdb_status"], igdb.StatusInvalidCredentials)
	}
}

func TestVersionEndpoint(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "1.2.3", "abc1234", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-store")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Errorf("version = %q, want %q", body["version"], "1.2.3")
	}
	if body["commit"] != "abc1234" {
		t.Errorf("commit = %q, want %q", body["commit"], "abc1234")
	}
}

func getVersion(t *testing.T, e http.Handler) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/version = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestVersionEndpoint_UpdateAvailable(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", st)

	body := getVersion(t, e)
	if body["update_available"] != true {
		t.Errorf("update_available = %v, want true", body["update_available"])
	}
	if body["latest_version"] != "9.9.9" {
		t.Errorf("latest_version = %v, want 9.9.9", body["latest_version"])
	}
	if body["release_url"] != "https://github.com/drzero42/nexorious/releases/tag/v9.9.9" {
		t.Errorf("release_url = %v", body["release_url"])
	}
	if body["update_check_enabled"] != true {
		t.Errorf("update_check_enabled = %v, want true", body["update_check_enabled"])
	}
	if body["version"] != "0.1.0" || body["commit"] != "abc1234" {
		t.Errorf("version/commit = %v/%v", body["version"], body["commit"])
	}
}

func TestVersionEndpoint_DevBuildNeverClaimsUpdate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "dev", "unknown", st)

	body := getVersion(t, e)
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false for dev build", body["update_available"])
	}
	if body["latest_version"] != "" || body["release_url"] != "" {
		t.Errorf("latest_version/release_url = %v/%v, want empty", body["latest_version"], body["release_url"])
	}
}

func TestVersionEndpoint_CheckDisabled(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = false
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", st)

	body := getVersion(t, e)
	if body["update_check_enabled"] != false {
		t.Errorf("update_check_enabled = %v, want false", body["update_check_enabled"])
	}
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false when disabled", body["update_available"])
	}
	if body["latest_version"] != "" || body["release_url"] != "" {
		t.Errorf("latest_version/release_url = %v/%v, want empty", body["latest_version"], body["release_url"])
	}
}

func TestVersionEndpoint_NilStateIsSafe(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", nil)

	body := getVersion(t, e)
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false with nil state", body["update_available"])
	}
}

func TestVersionEndpoint_EmptyStateNoUpdate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", updatecheck.NewState())

	body := getVersion(t, e)
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false with empty state", body["update_available"])
	}
	if body["latest_version"] != "" || body["release_url"] != "" {
		t.Errorf("latest_version/release_url = %v/%v, want empty", body["latest_version"], body["release_url"])
	}
}

func TestMetricsEndpoint_ServedAndBypassesGates(t *testing.T) {
	// Init the observability package so MetricsHandler() is non-nil.
	prov, err := observability.Init(&config.Config{OTELServiceName: "nexorious-test", OTELMetricsEnabled: true}, "test")
	if err != nil {
		t.Fatalf("observability.Init: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	tests := []struct {
		name     string
		migrator func() *migrate.Migrator
	}{
		{
			name: "db_unavailable gate",
			migrator: func() *migrate.Migrator {
				return migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
			},
		},
		{
			name: "needs_migration gate",
			migrator: func() *migrate.Migrator {
				return migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
			},
		},
		{
			name: "setup_required gate",
			migrator: func() *migrate.Migrator {
				m := migrate.NewMigratorForTest(migrate.AppStateReady)
				m.SetNeedsSetup(true)
				return m
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := api.New(testEncrypter, testCfg(), tt.migrator(), nil, "", nil, nil, nil, "dev", "unknown", nil)

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 for /metrics, got %d (gate should be bypassed)", rec.Code)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("expected Content-Type starting with text/plain, got %q", ct)
			}
		})
	}

	// Gate 4: /metrics must also bypass maintenance mode, so a Prometheus scrape
	// does not flap the target to DOWN during a restore window.
	t.Run("maintenance_mode", func(t *testing.T) {
		maint.SetMaintenanceMode(true)
		t.Cleanup(func() { maint.SetMaintenanceMode(false) })

		e := api.New(testEncrypter, testCfg(), migrate.NewMigratorForTest(migrate.AppStateReady), nil, "", nil, nil, nil, "dev", "unknown", nil)
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for /metrics during maintenance, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("expected Content-Type starting with text/plain, got %q", ct)
		}
	})
}
