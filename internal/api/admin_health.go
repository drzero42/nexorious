package api

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbrole"
	"github.com/drzero42/nexorious/internal/logging"
)

// Health-warning codes returned by GET /api/admin/health-warnings.
const (
	// healthWarningDBRolePrivileged: the app's database role is a superuser
	// or can run programs / write files on the database host, so restoring a
	// crafted backup could reach the host (#1357).
	healthWarningDBRolePrivileged = "db_role_privileged"
)

// HealthWarning is one deployment problem an admin should fix.
type HealthWarning struct {
	Code string `json:"code"`
	// Role is set for db_role_privileged: the role the app connects as.
	Role string `json:"role,omitempty"`
}

// AdminHealthHandler serves deployment warnings for admins. Unlike /health it
// is admin-only: these warnings describe the server's security posture.
type AdminHealthHandler struct {
	db *bun.DB
}

func NewAdminHealthHandler(db *bun.DB) *AdminHealthHandler {
	return &AdminHealthHandler{db: db}
}

// HandleList returns the current warnings, computed live so a warning clears
// as soon as its cause is fixed.
func (h *AdminHealthHandler) HandleList(c *echo.Context) error {
	ctx := c.Request().Context()
	warnings := []HealthWarning{}

	check, err := dbrole.CheckConnectedRole(ctx, h.db)
	if err != nil {
		slog.ErrorContext(ctx, "admin health: check database role", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check database role")
	}
	if check.Privileged() {
		warnings = append(warnings, HealthWarning{Code: healthWarningDBRolePrivileged, Role: check.Role})
	}

	return c.JSON(http.StatusOK, map[string]any{"warnings": warnings})
}
