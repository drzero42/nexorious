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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/importmodel"
	"github.com/drzero42/nexorious/internal/services/importsource"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

const maxImportBodyBytes = 50 * 1024 * 1024 // 50 MB

// readUploadFile parses the multipart form and reads the "file" field, enforcing
// the 50 MB limit. Returns the bytes or an *echo.HTTPError. Shared by every
// import upload handler so they read identically.
func (h *ImportHandler) readUploadFile(c *echo.Context) ([]byte, error) {
	if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
	}
	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "missing file field")
	}
	defer func() { _ = file.Close() }()
	lr := io.LimitReader(file, maxImportBodyBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if len(body) > maxImportBodyBytes {
		return nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}
	return body, nil
}

// igdbGuard returns a 400 *echo.HTTPError when IGDB is not configured. Every
// import requires IGDB for title matching and metadata; displayName names the
// collection in the error message (e.g. "CSV", "Nexorious", "vglist").
func (h *ImportHandler) igdbGuard(displayName string) error {
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("IGDB must be configured to import a %s collection", displayName))
	}
	return nil
}

// ImportHandler handles import-related endpoints.
type ImportHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	igdbClient  *igdb.Client
}

// NewImportHandler returns a new ImportHandler.
func NewImportHandler(db *bun.DB, riverClient *river.Client[pgx.Tx], igdbClient *igdb.Client) *ImportHandler {
	return &ImportHandler{db: db, riverClient: riverClient, igdbClient: igdbClient}
}

// nexoriousExport is the expected structure of a nexorious export file.
type nexoriousExport struct {
	Version       string            `json:"version"`
	ExportVersion string            `json:"export_version"` // legacy 1.x key, used only for error messages
	Games         []json.RawMessage `json:"games"`
}

// HandleImportNexorious handles POST /api/import/nexorious.
func (h *ImportHandler) HandleImportNexorious(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Prerequisite: IGDB must be configured. Each game is re-hydrated from its
	// igdb_id; with no client an import cannot construct usable games.
	if err := h.igdbGuard("Nexorious"); err != nil {
		return err
	}

	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	var export nexoriousExport
	if err := json.Unmarshal(body, &export); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON")
	}

	if export.Version != "2.0" {
		msg := "Unsupported import file. Only Nexorious library format version 2.0 is supported."
		if export.ExportVersion != "" {
			msg = fmt.Sprintf("Unsupported legacy export (version %s). Only Nexorious library format version 2.0 is supported.", export.ExportVersion)
		}
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}

	if len(export.Games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "games array is missing or empty")
	}

	ctx := context.Background()

	// Check for an active nexorious import job for this user.
	var existing models.Job
	err := h.db.NewSelect().
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
		return echo.NewHTTPError(http.StatusConflict, "an active nexorious import is already in progress")
	}

	now := time.Now().UTC()
	job := &models.Job{
		ID:               uuid.NewString(),
		UserID:           userID,
		JobType:          models.JobTypeImport,
		Source:           models.JobSourceNexorious,
		Status:           models.JobStatusPending,
		Priority:         models.JobPriorityHigh,
		TotalItems:       len(export.Games),
		DispatchComplete: true, // not a streaming sync; the completion gate is N/A
		CreatedAt:        now,
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	reqCtx := c.Request().Context()

	// Create one JobItem per game and enqueue a task.
	var skipCount int
	for i, raw := range export.Games {
		var gameFields struct {
			IgdbID *int    `json:"igdb_id"`
			Title  *string `json:"title"`
		}
		if err := json.Unmarshal(raw, &gameFields); err != nil {
			slog.WarnContext(reqCtx, "import: malformed game record, skipping", "record_index", i, logging.KeyErr, err, logging.Cat(logging.CategoryValidation))
			skipCount++
			continue
		}

		itemKey := fmt.Sprintf("game_%d", i)
		if gameFields.IgdbID != nil {
			itemKey = fmt.Sprintf("igdb_%d", *gameFields.IgdbID)
		}

		sourceTitle := fmt.Sprintf("Game %d", i)
		if gameFields.Title != nil && *gameFields.Title != "" {
			sourceTitle = *gameFields.Title
		}

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

		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.ImportItemArgs{JobItemID: item.ID}, nil); err != nil {
				slog.ErrorContext(reqCtx, "import: submit task", logging.KeyJobItemID, item.ID, logging.KeyErr, err)
			}
		}
	}

	if skipCount > 0 {
		if _, err := h.db.NewRaw(
			`UPDATE jobs SET total_items = total_items - ? WHERE id = ?`,
			skipCount, job.ID,
		).Exec(ctx); err != nil {
			slog.ErrorContext(reqCtx, "import: update total_items failed", logging.KeyErr, err, logging.KeyJobID, job.ID, logging.Cat(logging.CategoryDB))
		} else {
			job.TotalItems -= skipCount
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":        job.ID,
		"source":        job.Source,
		"status":        job.Status,
		"message":       fmt.Sprintf("Import job created. Processing %d games.", job.TotalItems),
		"total_items":   job.TotalItems,
		"skipped_count": skipCount,
	})
}

// enqueueImportJob runs the shared post-mapping import tail: it refuses a second
// active import for (user, source), inserts the processing job, creates one
// job_item per game with an ImportMatch task, marks dispatch complete, and
// triggers the completion check. It returns the job id and item count, or an
// *echo.HTTPError the caller can return directly.
func (h *ImportHandler) enqueueImportJob(reqCtx context.Context, userID, source, displayName string, games []importmodel.Game) (string, int, error) {
	ctx := context.Background()

	var existing models.Job
	err := h.db.NewSelect().Model(&existing).
		Where("user_id = ?", userID).
		Where("job_type = ?", models.JobTypeImport).
		Where("source = ?", source).
		Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Limit(1).Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to check active import")
	}
	if err == nil {
		return "", 0, echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("an active %s import is already in progress", displayName))
	}

	now := time.Now().UTC()
	job := &models.Job{
		ID:               uuid.NewString(),
		UserID:           userID,
		JobType:          models.JobTypeImport,
		Source:           source,
		Status:           models.JobStatusProcessing,
		Priority:         models.JobPriorityHigh,
		TotalItems:       len(games),
		DispatchComplete: false,
		CreatedAt:        now,
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	for i, g := range games {
		meta, err := json.Marshal(g)
		if err != nil {
			return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal game payload")
		}
		item := &models.JobItem{
			ID:             uuid.NewString(),
			JobID:          job.ID,
			UserID:         userID,
			ItemKey:        fmt.Sprintf("game_%d", i),
			SourceTitle:    g.Title,
			SourceMetadata: meta,
			Status:         models.JobItemStatusPending,
			Result:         json.RawMessage(`{}`),
			IGDBCandidates: json.RawMessage(`[]`),
		}
		if _, err := h.db.NewInsert().Model(item).Exec(ctx); err != nil {
			return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.ImportMatchArgs{JobItemID: item.ID}, nil); err != nil {
				slog.ErrorContext(reqCtx, "import: submit import_match", logging.KeyJobItemID, item.ID, logging.KeyErr, err)
			}
		}
	}

	// Dispatch is complete only now that every item exists and is enqueued.
	// Flipping the flag (and re-checking completion) here closes the window where
	// an early item could finish and finalize the job before later items were
	// inserted — the completion check refuses to finalize while dispatch is in
	// flight (dispatch_complete=false), mirroring the sync dispatch worker.
	if _, err := h.db.NewRaw(`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, job.ID).Exec(ctx); err != nil {
		slog.ErrorContext(reqCtx, "import: mark dispatch complete", logging.KeyJobID, job.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	tasks.ImportCheckJobCompletion(h.db, job.ID)

	return job.ID, len(games), nil
}

// handleImportSource returns an Echo handler for a registered import source.
// It validates IGDB is configured, parses the upload via the source's mapper,
// creates the job + one job_item per game, and enqueues the match stage. One
// active import per (user, source) is allowed at a time.
func (h *ImportHandler) handleImportSource(src importsource.Source) echo.HandlerFunc {
	return func(c *echo.Context) error {
		userID := auth.UserIDFromContext(c)
		if userID == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		if err := h.igdbGuard(src.DisplayName); err != nil {
			return err
		}

		body, herr := h.readUploadFile(c)
		if herr != nil {
			return herr
		}

		games, err := src.Mapper.Parse(body)
		if err != nil {
			if errors.Is(err, importmodel.ErrInvalidSignature) {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("not a %s export", src.DisplayName))
			}
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse file: "+err.Error())
		}
		if len(games) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no games found in file")
		}

		jobID, total, err := h.enqueueImportJob(c.Request().Context(), userID, src.Slug, src.DisplayName, games)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"job_id":      jobID,
			"source":      src.Slug,
			"status":      models.JobStatusProcessing,
			"message":     fmt.Sprintf("%s import job created. Matching %d games.", src.DisplayName, total),
			"total_items": total,
		})
	}
}
