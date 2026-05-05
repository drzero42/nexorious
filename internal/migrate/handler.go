package migrate

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/ui"
)

// Handler holds the Echo handlers for migration routes.
type Handler struct {
	migrator *Migrator
	tmpl     *template.Template
}

// NewHandler creates a Handler. Panics if the migration template cannot be parsed.
func NewHandler(m *Migrator) *Handler {
	tmpl, err := template.ParseFS(ui.MigrateBox, "migrate/migrate.html")
	if err != nil {
		panic(fmt.Sprintf("migrate: failed to parse template: %v", err))
	}
	return &Handler{migrator: m, tmpl: tmpl}
}

// Migrator exposes the underlying Migrator for tests.
func (h *Handler) Migrator() *Migrator { return h.migrator }

// HandleMigrateUI renders the migration UI page.
// GET /migrate
func (h *Handler) HandleMigrateUI(c *echo.Context) error {
	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}
	ver, _, err := h.migrator.CurrentVersion()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get current version")
	}

	data := struct {
		PendingCount   int
		CurrentVersion uint
	}{
		PendingCount:   pending,
		CurrentVersion: ver,
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
	return h.tmpl.Execute(c.Response(), data)
}

// HandleStatus returns migration status as JSON.
// GET /api/migrate/status
func (h *Handler) HandleStatus(c *echo.Context) error {
	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}
	ver, dirty, err := h.migrator.CurrentVersion()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get current version")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"pending_count":   pending,
		"current_version": ver,
		"dirty":           dirty,
		"state":           h.migrator.State().String(),
	})
}

// HandleRun triggers migration asynchronously.
// POST /api/migrate/run
func (h *Handler) HandleRun(c *echo.Context) error {
	switch h.migrator.State() {
	case AppStateMigrating:
		return c.JSON(http.StatusConflict, map[string]string{"error": "migration already in progress"})
	case AppStateReady:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "already up to date"})
	}

	go func() {
		if err := h.migrator.RunMigrations(c.Request().Context()); err != nil {
			// Error already recorded in logCh by RunMigrations.
			_ = err
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "migration started"})
}

// HandleProgress streams migration log lines as Server-Sent Events.
// GET /api/migrate/progress
func (h *Handler) HandleProgress(c *echo.Context) error {
	ch := h.migrator.LogCh()
	if ch == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "no migration in progress"})
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	for line := range ch {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	_, _ = fmt.Fprintf(w, "event: complete\ndata: {}\n\n")
	flusher.Flush()
	return nil
}
