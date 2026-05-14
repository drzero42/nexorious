package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/ratelimit"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
)

func TestAppStateMiddleware_RedirectsToMigrate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testCfg(), m, nil, "", nil, nil, nil)

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

func TestAppStateMiddleware_BypassMigrationPaths(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(testCfg(), m, nil, "", nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for bypass path, got 302")
	}
}

func TestAppStateMiddleware_ReadyStatePassesThrough(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testCfg(), m, nil, "", nil, nil, nil)

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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	// Should not redirect to /setup (migrate routes are bypassed).
	if loc := rec.Header().Get("Location"); loc == "/setup" {
		t.Errorf("migrate route should not redirect to /setup")
	}
}

func TestHealth_OKWhenReady(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)
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

func TestHealth_ReportsIGDBConfiguredTrue(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.IGDBClientID = "test-id"
	cfg.IGDBClientSecret = "test-secret"
	igdbClient := igdb.NewClient(cfg, ratelimit.NewLocal(100, 100))
	e := api.New(cfg, migrator, nil, "", igdbClient, nil, nil)

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
	igdbConfigured, ok := body["igdb_configured"]
	if !ok {
		t.Fatal("response missing igdb_configured field")
	}
	if igdbConfigured != true {
		t.Errorf("igdb_configured = %v; want true", igdbConfigured)
	}
}

func TestHealth_ReportsIGDBConfiguredFalse(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testCfg(), migrator, nil, "", nil, nil, nil)

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
	igdbConfigured, ok := body["igdb_configured"]
	if !ok {
		t.Fatal("response missing igdb_configured field")
	}
	if igdbConfigured != false {
		t.Errorf("igdb_configured = %v; want false", igdbConfigured)
	}
}
