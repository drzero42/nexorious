package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

const maxImportBodyBytes = 50 * 1024 * 1024 // 50 MB

// ImportHandler handles import-related endpoints.
type ImportHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

// NewImportHandler returns a new ImportHandler.
func NewImportHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *ImportHandler {
	return &ImportHandler{db: db, riverClient: riverClient}
}

// nexoriousExport is the expected structure of a nexorious export file.
type nexoriousExport struct {
	ExportVersion string            `json:"export_version"`
	Games         []json.RawMessage `json:"games"`
}

// HandleImportNexorious handles POST /api/import/nexorious.
func (h *ImportHandler) HandleImportNexorious(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Parse multipart form (limit to maxImportBodyBytes + some overhead for form fields).
	if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
	}

	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "missing file field")
	}
	defer func() { _ = file.Close() }()

	// Read and enforce 50 MB limit.
	lr := io.LimitReader(file, maxImportBodyBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if len(body) > maxImportBodyBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}

	// Parse JSON.
	var export nexoriousExport
	if err := json.Unmarshal(body, &export); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON")
	}

	// Validate export_version.
	if export.ExportVersion != "1.2" {
		return echo.NewHTTPError(http.StatusBadRequest, "Unsupported export version. Only version 1.2 is supported.")
	}

	// Validate games array.
	if len(export.Games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "games array is missing or empty")
	}

	ctx := context.Background()

	// Check for an active nexorious import job for this user.
	var existing models.Job
	err = h.db.NewSelect().
		Model(&existing).
		Where("user_id = ?", userID).
		Where("job_type = ?", models.JobTypeImport).
		Where("source = ?", models.JobSourceNexorious).
		Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Limit(1).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check active import")
	}
	if err == nil {
		// An active job was found.
		return echo.NewHTTPError(http.StatusConflict, "an active nexorious import is already in progress")
	}

	// Create the Job record.
	job := &models.Job{
		ID:         uuid.NewString(),
		UserID:     userID,
		JobType:    models.JobTypeImport,
		Source:     models.JobSourceNexorious,
		Status:     models.JobStatusPending,
		Priority:   models.JobPriorityHigh,
		TotalItems: len(export.Games),
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	// Create one JobItem per game and enqueue a task.
	for i, raw := range export.Games {
		// Extract igdb_id and title from raw game JSON.
		var gameFields struct {
			IgdbID *int    `json:"igdb_id"`
			Title  *string `json:"title"`
		}
		_ = json.Unmarshal(raw, &gameFields)

		// Build item_key.
		itemKey := fmt.Sprintf("game_%d", i)
		if gameFields.IgdbID != nil {
			itemKey = fmt.Sprintf("igdb_%d", *gameFields.IgdbID)
		}

		// Build source_title.
		sourceTitle := fmt.Sprintf("Game %d", i)
		if gameFields.Title != nil && *gameFields.Title != "" {
			sourceTitle = *gameFields.Title
		}

		// Build source_metadata.
		metadata, err := json.Marshal(map[string]any{
			"item_type": "game",
			"data":      raw,
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to build job item metadata")
		}

		item := &models.JobItem{
			ID:             uuid.NewString(),
			JobID:          job.ID,
			UserID:         userID,
			ItemKey:        itemKey,
			SourceTitle:    sourceTitle,
			SourceMetadata: metadata,
			Status:         models.JobItemStatusPending,
			Result:         json.RawMessage(`{}`),
			IGDBCandidates: json.RawMessage(`[]`),
		}
		if _, err := h.db.NewInsert().Model(item).Exec(ctx); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}

		// Enqueue the task.
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.ImportItemArgs{JobItemID: item.ID}, nil); err != nil {
				slog.Error("import: submit task", "item_id", item.ID, "err", err)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":      job.ID,
		"source":      job.Source,
		"status":      job.Status,
		"message":     fmt.Sprintf("Import job created. Processing %d games.", job.TotalItems),
		"total_items": job.TotalItems,
	})
}
