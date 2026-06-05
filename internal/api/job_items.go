package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

type JobItemsHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

func NewJobItemsHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *JobItemsHandler {
	return &JobItemsHandler{db: db, riverClient: riverClient}
}

// HandleGetJobItem handles GET /api/job-items/:id.
func (h *JobItemsHandler) HandleGetJobItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var item models.JobItem
	err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}

	return c.JSON(http.StatusOK, item)
}

// HandleRetryItem handles POST /api/job-items/:id/retry.
func (h *JobItemsHandler) HandleRetryItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var item models.JobItem
	err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}

	if item.Status != models.JobItemStatusFailed {
		return echo.NewHTTPError(http.StatusConflict, "item is not failed")
	}

	_, err = h.db.NewRaw(`
		UPDATE job_items
		SET status = ?, error_message = NULL, processed_at = NULL
		WHERE id = ?`,
		models.JobItemStatusPending, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retry item")
	}

	// Get job type to determine task type.
	var job models.Job
	err = h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, item.JobID).
		Scan(context.Background(), &job)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get parent job")
	}

	retryInsert(context.Background(), h.db, h.riverClient, job.JobType, job.Source, itemID)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// isImportSource reports whether a job source uses the generic job-item
// resolve/skip path. Sync sources resolve through the external_games rematch
// endpoints instead and must be rejected here so that flow is untouched.
func isImportSource(source string) bool {
	return source == models.JobSourceDarkadia || source == models.JobSourceNexorious
}

type resolveItemRequest struct {
	IGDBID int `json:"igdb_id"`
}

// HandleResolveItem handles POST /api/job-items/:id/resolve. It records the
// user's chosen IGDB id on a pending_review import item and enqueues the
// Darkadia finalize worker. Scoped to import sources.
func (h *JobItemsHandler) HandleResolveItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	itemID := c.Param("id")

	var req resolveItemRequest
	if err := c.Bind(&req); err != nil || req.IGDBID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	item, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if !isImportSource(job.Source) {
		return echo.NewHTTPError(http.StatusBadRequest, "this item is resolved through the sync flow")
	}
	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	if _, err := h.db.NewRaw(
		`UPDATE job_items SET resolved_igdb_id = ?, status = ?, resolved_at = now() WHERE id = ?`,
		req.IGDBID, models.JobItemStatusProcessing, itemID,
	).Exec(context.Background()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve item")
	}

	if err := tasks.EnqueueOrFail(context.Background(), h.db, h.riverClient, itemID,
		tasks.DarkadiaFinalizeArgs{JobItemID: itemID}); err != nil {
		slog.Error("resolve_item: enqueue finalize", "item_id", itemID, "err", err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleSkipItem handles POST /api/job-items/:id/skip. Scoped to import sources.
func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	itemID := c.Param("id")

	item, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if !isImportSource(job.Source) {
		return echo.NewHTTPError(http.StatusBadRequest, "this item is skipped through the sync flow")
	}
	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	if _, err := h.db.NewRaw(
		`UPDATE job_items SET status = ?, processed_at = now() WHERE id = ?`,
		models.JobItemStatusSkipped, itemID,
	).Exec(context.Background()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip item")
	}
	tasks.DarkadiaCheckJobCompletion(h.db, job.ID)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// loadItemAndJob loads a job_item (scoped to the user) plus its parent job.
func (h *JobItemsHandler) loadItemAndJob(itemID, userID string) (*models.JobItem, *models.Job, error) {
	var item models.JobItem
	if err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}
	var job models.Job
	if err := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, item.JobID).
		Scan(context.Background(), &job); err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to get parent job")
	}
	return &item, &job, nil
}
