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

func TestMaintenanceMiddleware_AllowsExemptEndpoints(t *testing.T) {
	paths := []string{"/health", "/api/admin/backups", "/api/auth/me"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			SetMaintenanceMode(true)
			defer SetMaintenanceMode(false)

			e := echo.New()
			e.Use(MaintenanceMiddleware())
			e.GET(path, func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 for %s, got %d", path, rec.Code)
			}
		})
	}
}

func TestMaintenanceMiddleware_AllowsBrandIconAssets(t *testing.T) {
	paths := []string{"/logo.svg", "/favicon.svg", "/favicon.ico", "/apple-touch-icon.png"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			SetMaintenanceMode(true)
			defer SetMaintenanceMode(false)

			e := echo.New()
			e.Use(MaintenanceMiddleware())
			e.GET(path, func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 for %s, got %d", path, rec.Code)
			}
		})
	}
}
