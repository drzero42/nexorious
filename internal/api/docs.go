package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/docs"
	"github.com/drzero42/nexorious/internal/auth"
)

// slugPattern restricts doc slugs to lowercase letters, digits, and hyphens.
// This matches the on-disk file naming and guards against path traversal
// ("/", "..", encoded separators) before we touch the embedded filesystem.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// DocsHandler serves the embedded Markdown guides at /api/docs/:slug.
type DocsHandler struct {
	fsys fs.FS
}

// NewDocsHandler builds a DocsHandler over the embedded docs filesystem.
func NewDocsHandler() *DocsHandler {
	return &DocsHandler{fsys: docs.FS}
}

// HandleGetDoc returns raw Markdown for the requested slug. The slug is
// validated against slugPattern; "admin-guide" additionally requires admin.
// Unknown slugs 404 naturally from the embedded FS.
func (h *DocsHandler) HandleGetDoc(c *echo.Context) error {
	slug := c.Param("slug")
	if !slugPattern.MatchString(slug) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "doc not found"})
	}
	if slug == "admin-guide" && !auth.IsAdminFromContext(c) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "admin access required"})
	}
	data, err := fs.ReadFile(h.fsys, slug+".md")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "doc not found"})
		}
		slog.Error("read embedded doc", "slug", slug, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read doc")
	}
	return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", data)
}
