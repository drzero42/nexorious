package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
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
	db         *bun.DB
	lc         *joblifecycle.Module
	igdbClient *igdb.Client
}

// NewImportHandler returns a new ImportHandler.
func NewImportHandler(db *bun.DB, lc *joblifecycle.Module, igdbClient *igdb.Client) *ImportHandler {
	return &ImportHandler{db: db, lc: lc, igdbClient: igdbClient}
}

// nexoriousExport is the expected structure of a nexorious export file.
type nexoriousExport struct {
	Version       string            `json:"version"`
	ExportVersion string            `json:"export_version"` // legacy 1.x key, used only for error messages
	Games         []json.RawMessage `json:"games"`
	Pools         json.RawMessage   `json:"pools"`
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

	if export.Version != "2.0" && export.Version != "2.1" {
		msg := "Unsupported import file. Only Nexorious library format versions 2.0 and 2.1 are supported."
		if export.ExportVersion != "" {
			msg = fmt.Sprintf("Unsupported legacy export (version %s). Only Nexorious library format versions 2.0 and 2.1 are supported.", export.ExportVersion)
		}
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}

	if len(export.Games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "games array is missing or empty")
	}

	reqCtx := c.Request().Context()
	now := time.Now().UTC()
	var items []joblifecycle.NewItem

	// Any pools section is kept as an item created already completed, with no
	// River job; the finishing effects apply it once the Job first completes.
	// It is not counted in total_items.
	gameItemStart := now
	if len(export.Pools) > 0 && string(export.Pools) != "null" {
		gameItemStart = now.Add(time.Microsecond)
		poolsMeta, err := json.Marshal(map[string]any{"item_type": "pools", "data": export.Pools})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to build pools item metadata")
		}
		items = append(items, joblifecycle.NewItem{
			UserID: userID, ItemKey: tasks.PoolsItemKey, SourceTitle: "(pools)",
			SourceMetadata: poolsMeta, CreatedAt: now, Completed: true,
		})
	}

	// One item per usable game record. Each item's created_at is offset by its
	// index in the file, so the item list keeps the file's order (#1319).
	var skipCount, games int
	seen := make(map[int]bool, len(export.Games))
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
			// The item key is igdb_<id>, unique per Job.
			if seen[*gameFields.IgdbID] {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("the file lists igdb_id %d more than once", *gameFields.IgdbID))
			}
			seen[*gameFields.IgdbID] = true
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
		items = append(items, joblifecycle.NewItem{
			UserID: userID, ItemKey: itemKey, SourceTitle: sourceTitle, SourceMetadata: metadata,
			CreatedAt: gameItemStart.Add(time.Duration(i) * time.Microsecond),
		})
		games++
	}
	if games == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "the file has no usable game records")
	}

	// Detached from the request: a client that disconnects must not roll back
	// a Job it was about to be told about.
	created, err := h.lc.CreateWithItems(context.WithoutCancel(reqCtx), joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeImport, Source: models.JobSourceNexorious,
		Priority: models.JobPriorityHigh, TotalItems: games,
		CreatedAt: now,
	}, items, nil)
	if err != nil {
		slog.ErrorContext(reqCtx, "import: create job", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}
	if !created.Created {
		return echo.NewHTTPError(http.StatusConflict, "an active nexorious import is already in progress")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":        created.JobID,
		"source":        models.JobSourceNexorious,
		"status":        models.JobStatusProcessing,
		"message":       fmt.Sprintf("Import job created. Processing %d games.", games),
		"total_items":   games,
		"skipped_count": skipCount,
	})
}

// enqueueImportJob runs the shared post-mapping import tail: it creates the
// processing Job with one job_item per game, each with its match River job, in
// one transaction, refusing a second active import for (user, source). It
// returns the job id and item count, or an *echo.HTTPError the caller can
// return directly.
func (h *ImportHandler) enqueueImportJob(reqCtx context.Context, userID, source, displayName string, games []importmodel.Game) (string, int, error) {
	now := time.Now().UTC()
	items := make([]joblifecycle.NewItem, len(games))
	for i, g := range games {
		meta, err := json.Marshal(g)
		if err != nil {
			return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal game payload")
		}
		// Offset by the index, so the item list keeps the file's order (#1319).
		items[i] = joblifecycle.NewItem{
			UserID: userID, ItemKey: fmt.Sprintf("game_%d", i), SourceTitle: g.Title,
			SourceMetadata: meta, CreatedAt: now.Add(time.Duration(i) * time.Microsecond),
		}
	}

	// Detached from the request: a client that disconnects must not roll back
	// a Job it was about to be told about.
	created, err := h.lc.CreateWithItems(context.WithoutCancel(reqCtx), joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeImport, Source: source,
		Priority: models.JobPriorityHigh, TotalItems: len(games),
		CreatedAt: now,
	}, items, nil)
	if err != nil {
		slog.ErrorContext(reqCtx, "import: create job", logging.KeySource, source, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}
	if !created.Created {
		return "", 0, echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("an active %s import is already in progress", displayName))
	}
	return created.JobID, len(games), nil
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
