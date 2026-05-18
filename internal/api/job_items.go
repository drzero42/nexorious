package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

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

// HandleResolveItem handles POST /api/job-items/:id/resolve.
func (h *JobItemsHandler) HandleResolveItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var body struct {
		IGDBID int `json:"igdb_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var item models.JobItem
	err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}

	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	now := time.Now().UTC()
	_, err = h.db.NewRaw(`
		UPDATE job_items
		SET resolved_igdb_id = ?, resolved_at = ?, status = ?
		WHERE id = ?`,
		body.IGDBID, now, models.JobItemStatusPending, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve item")
	}

	// Propagate the resolution to external_games and to same-title sibling SKUs.
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
	}
	if json.Unmarshal(item.SourceMetadata, &meta) == nil && meta.ExternalGameID != "" {
		var eg models.ExternalGame
		if egErr := h.db.NewSelect().Model(&eg).Where("id = ?", meta.ExternalGameID).Scan(context.Background()); egErr == nil {
			// Ensure the games row exists (FK on external_games.resolved_igdb_id).
			_, _ = h.db.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				body.IGDBID, eg.Title,
			).Exec(context.Background())
			// Resolve the matched external_game immediately so step 3.6 can find it.
			_, _ = h.db.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				body.IGDBID, eg.ID,
			).Exec(context.Background())
			// Find sibling external_games (same user/storefront/title, different SKU, still unresolved).
			var siblings []models.ExternalGame
			_ = h.db.NewSelect().Model(&siblings).
				Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NULL",
					eg.UserID, eg.Storefront, eg.Title, eg.ID).
				Scan(context.Background())
			for _, sib := range siblings {
				// Resolve the sibling external_game so step 3.6 in the worker skips IGDB search.
				_, _ = h.db.NewRaw(
					`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
					body.IGDBID, sib.ID,
				).Exec(context.Background())
				// Re-queue any pending_review job_items for this sibling.
				var sibItems []models.JobItem
				_ = h.db.NewRaw(
					`SELECT * FROM job_items WHERE user_id = ? AND status = 'pending_review' AND source_metadata->>'external_game_id' = ?`,
					eg.UserID, sib.ID,
				).Scan(context.Background(), &sibItems)
				for _, si := range sibItems {
					_, _ = h.db.NewRaw(
						`UPDATE job_items SET status = 'pending' WHERE id = ?`, si.ID,
					).Exec(context.Background())
					var sibJob models.Job
					if jErr := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, si.JobID).Scan(context.Background(), &sibJob); jErr == nil {
						retryInsert(context.Background(), h.riverClient, sibJob.JobType, si.ID)
					}
				}
			}
		}
	}

	// Get job type to determine task type.
	var job models.Job
	err = h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, item.JobID).
		Scan(context.Background(), &job)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get parent job")
	}

	retryInsert(context.Background(), h.riverClient, job.JobType, itemID)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleSkipItem handles POST /api/job-items/:id/skip.
func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	itemID := c.Param("id")

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.Bind(&body) // optional body

	var item models.JobItem
	err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}

	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	_, err = h.db.NewRaw(`
		UPDATE job_items SET status = ? WHERE id = ?`,
		models.JobItemStatusSkipped, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip item")
	}

	// For sync items, mark the external game as skipped so it won't be
	// re-queued on the next sync run.
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
	}
	if json.Unmarshal(item.SourceMetadata, &meta) == nil && meta.ExternalGameID != "" {
		_, _ = h.db.NewRaw(
			`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ? AND user_id = ?`,
			meta.ExternalGameID, userID,
		).Exec(context.Background())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "skipped"})
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

	if item.Status != models.JobItemStatusFailed && item.Status != models.JobItemStatusIGDBFailed {
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

	retryInsert(context.Background(), h.riverClient, job.JobType, itemID)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
