package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
)

func TestAppStateMiddleware_RedirectsToMigrate(t *testing.T) {
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(cfg, m)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
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
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(cfg, m)

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for bypass path, got 302")
	}
}

func TestAppStateMiddleware_ReadyStatePassesThrough(t *testing.T) {
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(cfg, m)

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
