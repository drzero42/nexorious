package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/services/importsource"
)

// HandleListImportSources returns the registered mapper-based import sources so
// the frontend picker is data-driven from the registry. It does not require
// IGDB to be configured — that guard stays on the upload itself.
func (h *ImportHandler) HandleListImportSources(c *echo.Context) error {
	return c.JSON(http.StatusOK, importsource.All())
}
