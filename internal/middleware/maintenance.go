package middleware

import (
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v5"
)

var (
	mu              sync.RWMutex
	maintenanceMode bool
)

func SetMaintenanceMode(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	maintenanceMode = enabled
}

func IsMaintenanceMode() bool {
	mu.RLock()
	defer mu.RUnlock()
	return maintenanceMode
}

func MaintenanceMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsMaintenanceMode() {
				return next(c)
			}
			path := c.Request().URL.Path
			if path == "/health" ||
				strings.HasPrefix(path, "/api/admin/backups") ||
				path == "/api/auth/me" {
				return next(c)
			}
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"error":            "Service Unavailable",
				"detail":           "Restore in progress",
				"maintenance_mode": true,
			})
		}
	}
}
