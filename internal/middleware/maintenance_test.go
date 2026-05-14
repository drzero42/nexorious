package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestMaintenanceMode_Toggle(t *testing.T) {
	SetMaintenanceMode(true)
	if !IsMaintenanceMode() {
		t.Error("expected maintenance mode on")
	}
	SetMaintenanceMode(false)
	if IsMaintenanceMode() {
		t.Error("expected maintenance mode off")
	}
}

func TestMaintenanceMiddleware_BlocksWhenActive(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/games", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsHealth(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/health", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsBackupEndpoints(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/admin/backups", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaintenanceMiddleware_AllowsAuthMe(t *testing.T) {
	SetMaintenanceMode(true)
	defer SetMaintenanceMode(false)

	e := echo.New()
	e.Use(MaintenanceMiddleware())
	e.GET("/api/auth/me", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
