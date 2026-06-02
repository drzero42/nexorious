package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
)

func TestAppStateMiddleware_RedirectsToMigrate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for bypass path, got 302")
	}
}

func TestAppStateMiddleware_ReadyStatePassesThrough(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
			e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")
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
			e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if loc := rec.Header().Get("Location"); loc == "/setup" {
				t.Errorf("brand asset %s should not redirect to /setup", path)
			}
		})
	}
}

func TestHealth_OKWhenReady(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
}

func TestHealth_OKWhenSetupPending(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	if body["status"] != "ok" {
		t.Errorf("expected status=ok when needsSetup, got %q", body["status"])
	}
}

func TestHealth_DBUnavailableReturns200(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	if body["status"] != "db_unavailable" {
		t.Errorf("expected db_unavailable, got %q", body["status"])
	}
}

func TestHealth_NeedsMigrationReturns200(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
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
	if body["status"] != "needs_migration" {
		t.Errorf("expected needs_migration, got %q", body["status"])
	}
}

func TestHealth_ReportsIGDBStatusOk(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.IGDBClientID = "test-id"
	cfg.IGDBClientSecret = "test-secret"
	igdbClient := igdb.NewClient(cfg, ratelimit.NewLocal(100, 100))
	e := api.New(testEncrypter, cfg, migrator, nil, "", igdbClient, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil, "dev", "unknown")

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
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "1.2.3", "abc1234")

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
	var body map[string]string
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
