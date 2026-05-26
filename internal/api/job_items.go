package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
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

	retryInsert(context.Background(), h.db, h.riverClient, job.JobType, itemID)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
