package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
)

// ExportHandler handles export-related endpoints.
type ExportHandler struct {
	db  *bun.DB
	lc  *joblifecycle.Module
	cfg *config.Config
}

// NewExportHandler returns a new ExportHandler.
func NewExportHandler(db *bun.DB, lc *joblifecycle.Module, cfg *config.Config) *ExportHandler {
	return &ExportHandler{db: db, lc: lc, cfg: cfg}
}

// handleExport is the shared logic for JSON and CSV export initiation.
func (h *ExportHandler) handleExport(c *echo.Context, source string) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	count, err := h.db.NewSelect().
		TableExpr("user_games").
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count games")
	}
	if count == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games to export")
	}

	// The export River job is the Job's dispatch: created with it, in one
	// transaction. Exports have no duplicate key.
	created, err := h.lc.CreateWithDispatch(ctx, joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeExport, Source: source,
		Priority: models.JobPriorityNormal, TotalItems: int(count),
	})
	if err != nil {
		slog.ErrorContext(ctx, "export: create job", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create export job")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":          created.JobID,
		"status":          models.JobStatusPending,
		"message":         fmt.Sprintf("Export job created. Exporting %d games.", count),
		"estimated_items": count,
	})
}

// HandleExportJSON handles POST /api/export/json.
func (h *ExportHandler) HandleExportJSON(c *echo.Context) error {
	return h.handleExport(c, models.JobSourceNexorious)
}

// HandleExportCSV handles POST /api/export/csv.
func (h *ExportHandler) HandleExportCSV(c *echo.Context) error {
	return h.handleExport(c, models.JobSourceCSV)
}

// HandleDownload handles GET /api/export/:id/download.
func (h *ExportHandler) HandleDownload(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")
	ctx := context.Background()

	// Load the job, scoped to the requesting user.
	job := &models.Job{}
	err := h.db.NewSelect().
		Model(job).
		Where("id = ? AND user_id = ?", jobID, userID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "job not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job")
	}

	if job.JobType != models.JobTypeExport {
		return echo.NewHTTPError(http.StatusBadRequest, "not an export job")
	}

	if job.Status != models.JobStatusCompleted {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("export job is not completed (status: %s)", job.Status))
	}

	if job.FilePath == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "export file path not set")
	}

	if _, err := os.Stat(*job.FilePath); err != nil {
		return echo.NewHTTPError(http.StatusGone, "export file no longer available")
	}

	// Check expiration: completed more than 24 hours ago.
	if job.CompletedAt != nil && time.Now().After(job.CompletedAt.Add(24*time.Hour)) {
		_ = os.Remove(*job.FilePath)
		return echo.NewHTTPError(http.StatusGone, "export file has expired")
	}

	ext := strings.ToLower(filepath.Ext(*job.FilePath))
	var contentType, filename string
	switch ext {
	case ".csv":
		contentType = "text/csv"
		filename = "nexorious_export_" + time.Now().Format("20060102_150405") + ".csv"
	default:
		contentType = "application/json"
		filename = "nexorious_export_" + time.Now().Format("20060102_150405") + ".json"
	}

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	f, err := os.Open(*job.FilePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusGone, "export file no longer available")
	}
	defer func() { _ = f.Close() }()
	return c.Stream(http.StatusOK, contentType, f)
}
