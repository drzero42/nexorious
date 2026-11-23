package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
)

type JobItemsHandler struct {
	db *bun.DB
	lc *joblifecycle.Module
}

func NewJobItemsHandler(db *bun.DB, lc *joblifecycle.Module) *JobItemsHandler {
	return &JobItemsHandler{db: db, lc: lc}
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
	ctx := c.Request().Context()

	// Authorised on the Job's owner: in an admin-triggered refresh the Job is
	// the admin's while each item belongs to the storefront's user.
	var owner struct {
		JobID     string `bun:"job_id"`
		JobUserID string `bun:"job_user_id"`
	}
	err := h.db.NewRaw(
		`SELECT ji.job_id, j.user_id AS job_user_id
		 FROM job_items ji JOIN jobs j ON j.id = ji.job_id WHERE ji.id = ?`, itemID,
	).Scan(ctx, &owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}
	if errors.Is(err, sql.ErrNoRows) || owner.JobUserID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if _, err := h.lc.Retry(ctx, owner.JobID, []string{itemID}); err != nil {
		if errors.Is(err, joblifecycle.ErrNotRetryable) {
			return echo.NewHTTPError(http.StatusConflict, "item is not failed, or its job is failed or cancelled")
		}
		return lifecycleHTTPError(ctx, err, "retry item")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

type resolveItemRequest struct {
	IGDBID int `json:"igdb_id"`
}

// reviewableImport returns the 400 for an item whose Job has no import review
// stage: a sync Job (resolved through the external-games flow), the nexorious
// import (one stage, no review), or any other kind. A sync Job is told by its
// job_type, not by the import source registry, so an import from a source
// that has since been removed can still be resolved.
func reviewableImport(job *models.Job, verb string) error {
	switch {
	case job.JobType == models.JobTypeSync:
		return echo.NewHTTPError(http.StatusBadRequest, "this item is "+verb+" through the sync flow")
	case job.JobType != models.JobTypeImport || job.Source == models.JobSourceNexorious:
		return echo.NewHTTPError(http.StatusBadRequest, "manual resolution is not supported for this import source")
	}
	return nil
}

// reviewHTTPError maps a Resolve or Skip refusal to its HTTP error.
func reviewHTTPError(ctx context.Context, err error, what string) error {
	if errors.Is(err, joblifecycle.ErrNotInReview) {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}
	return lifecycleHTTPError(ctx, err, what)
}

// HandleResolveItem handles POST /api/job-items/:id/resolve. It records the
// user's chosen IGDB id on an import item in Pending review; the item goes to
// finalize. Scoped to import sources with a review stage.
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

	_, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if err := reviewableImport(job, "resolved"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.lc.Resolve(ctx, itemID, req.IGDBID); err != nil {
		return reviewHTTPError(ctx, err, "resolve item")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleSkipItem handles POST /api/job-items/:id/skip. Scoped to import
// sources with a review stage.
func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	itemID := c.Param("id")

	_, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if err := reviewableImport(job, "skipped"); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.lc.Skip(ctx, itemID); err != nil {
		return reviewHTTPError(ctx, err, "skip item")
	}
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
