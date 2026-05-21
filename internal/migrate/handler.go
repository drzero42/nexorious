package migrate

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/ui"
)

type Handler struct {
	migrator *Migrator
	db       *bun.DB
	tmpl     *template.Template
}

func NewHandler(m *Migrator, db *bun.DB) *Handler {
	tmpl, err := template.ParseFS(ui.MigrateBox, "migrate/index.html")
	if err != nil {
		panic(fmt.Sprintf("migrate: failed to parse template: %v", err))
	}
	return &Handler{migrator: m, db: db, tmpl: tmpl}
}

func (h *Handler) Migrator() *Migrator { return h.migrator }

func (h *Handler) HandleMigrateUI(c *echo.Context) error {
	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}

	data := struct {
		PendingCount int
	}{
		PendingCount: pending,
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
	return h.tmpl.Execute(c.Response(), data)
}

func (h *Handler) HandleStatus(c *echo.Context) error {
	state := h.migrator.State()

	// In the failed state PendingCount may itself error (e.g. DB closed),
	// and the UI does not need the pending count to render the failure card.
	if state == AppStateMigrationFailed {
		return c.JSON(http.StatusOK, map[string]any{
			"pending_count": 0,
			"state":         state.String(),
			"error":         h.migrator.LastError(),
		})
	}

	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"pending_count": pending,
		"state":         state.String(),
	})
}

func (h *Handler) HandleRun(c *echo.Context) error {
	switch h.migrator.State() {
	case AppStateMigrating:
		return c.JSON(http.StatusConflict, map[string]string{"error": "migration already in progress"})
	case AppStateReady:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "already up to date"})
	}

	go func() {
		if err := h.migrator.RunMigrations(context.Background()); err != nil {
			_ = err
			return
		}
		if h.db != nil {
			if err := h.migrator.InitNeedsSetup(context.Background(), h.db); err != nil {
				_ = err
			}
		}
		h.migrator.TransitionToReady()
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "migration started"})
}

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
