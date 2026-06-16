package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// JobsHandler handles job-related endpoints.
type JobsHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
}

// NewJobsHandler returns a new JobsHandler.
func NewJobsHandler(db *bun.DB, riverClient *river.Client[pgx.Tx]) *JobsHandler {
	return &JobsHandler{db: db, riverClient: riverClient}
}

// jobItemCounts fetches aggregated item status counts for a job and returns a
// progress map ready for the API response.
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) (map[string]any, error) {
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	if err := h.db.NewRaw(`
		SELECT status, COUNT(*)::int AS count
		FROM job_items
		WHERE job_id = ?
		GROUP BY status`,
		jobID,
	).Scan(ctx, &counts); err != nil {
		return nil, err
	}

	m := map[string]int{
		"pending": 0, "processing": 0, "completed": 0,
		"pending_review": 0, "skipped": 0, "failed": 0,
	}
	for _, sc := range counts {
		m[sc.Status] = sc.Count
	}
	total := 0
	for _, v := range m {
		total += v
	}
	percent := 0
	if total > 0 {
		percent = (m["completed"] + m["skipped"]) * 100 / total
	}
	return map[string]any{
		"pending": m["pending"], "processing": m["processing"],
		"completed": m["completed"], "pending_review": m["pending_review"],
		"skipped": m["skipped"], "failed": m["failed"],
		"total": total, "percent": percent,
	}, nil
}

// toJobResponse builds the complete job API response DTO including computed fields.
func toJobResponse(job *models.Job, progress map[string]any) map[string]any {
	return map[string]any{
		"id":               job.ID,
		"user_id":          job.UserID,
		"job_type":         job.JobType,
		"source":           job.Source,
		"status":           job.Status,
		"priority":         job.Priority,
		"file_path":        job.FilePath,
		"total_items":      job.TotalItems,
		"error_message":    job.ErrorMessage,
		"auto_retry_done":  job.AutoRetryDone,
		"created_at":       job.CreatedAt,
		"started_at":       job.StartedAt,
		"completed_at":     job.CompletedAt,
		"is_terminal":      job.IsTerminal(),
		"duration_seconds": job.DurationSeconds(),
		"progress":         progress,
	}
}

// HandleListJobs handles GET /api/jobs.
func (h *JobsHandler) HandleListJobs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	sortBy := c.QueryParam("sort_by")
	if sortBy == "" {
		sortBy = "created_at"
	}
	allowedSorts := map[string]bool{
		"created_at":   true,
		"started_at":   true,
		"completed_at": true,
		"job_type":     true,
		"status":       true,
	}
	if !allowedSorts[sortBy] {
		sortBy = "created_at"
	}

	sortOrder := c.QueryParam("sort_order")
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	q := h.db.NewSelect().TableExpr("jobs").Where("user_id = ?", userID)

	if jt := c.QueryParam("job_type"); jt != "" {
		q = q.Where("job_type = ?", jt)
	}
	if src := c.QueryParam("source"); src != "" {
		q = q.Where("source = ?", src)
	}
	if st := c.QueryParam("status"); st != "" {
		q = q.Where("status = ?", st)
	}

	// Count total.
	var total int
	countQ := h.db.NewSelect().TableExpr("jobs").Where("user_id = ?", userID)
	if jt := c.QueryParam("job_type"); jt != "" {
		countQ = countQ.Where("job_type = ?", jt)
	}
	if src := c.QueryParam("source"); src != "" {
		countQ = countQ.Where("source = ?", src)
	}
	if st := c.QueryParam("status"); st != "" {
		countQ = countQ.Where("status = ?", st)
	}
	total, err := countQ.Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count jobs")
	}

	offset := (page - 1) * perPage
	q = q.OrderExpr(fmt.Sprintf("%s %s", sortBy, sortOrder)).
		Limit(perPage).Offset(offset)

	var jobs []models.Job
	err = q.Scan(context.Background(), &jobs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list jobs")
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	ctx := c.Request().Context()
	jobDTOs := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		progress, err := h.jobItemCounts(context.Background(), jobs[i].ID)
		if err != nil {
			slog.ErrorContext(ctx, "jobs: fetch item counts failed", logging.KeyErr, err, logging.KeyJobID, jobs[i].ID, logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
		}
		jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], progress))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"jobs":        jobDTOs,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// HandleJobsSummary handles GET /api/jobs/summary.
func (h *JobsHandler) HandleJobsSummary(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var result struct {
		Running int `bun:"running" json:"running"`
		Failed  int `bun:"failed" json:"failed"`
	}
	err := h.db.NewRaw(`
		SELECT
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing')) AS running,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed
		FROM jobs
		WHERE user_id = ?`,
		userID,
	).Scan(context.Background(), &result)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job summary")
	}

	return c.JSON(http.StatusOK, result)
}

// HandlePendingReviewCount handles GET /api/jobs/pending-review-count.
func (h *JobsHandler) HandlePendingReviewCount(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	type sourceCount struct {
		Source string `bun:"source"`
		Count  int    `bun:"count"`
	}

	var rows []sourceCount
	err := h.db.NewRaw(`
		SELECT j.source, COUNT(*) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		LEFT JOIN external_games eg ON eg.id = ji.external_game_id
		WHERE ji.user_id = ? AND ji.status = ?
		  AND (eg.id IS NULL OR eg.parent_id IS NULL)
		GROUP BY j.source`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count pending reviews")
	}

	countsBySource := make(map[string]int)
	total := 0
	for _, row := range rows {
		countsBySource[row.Source] = row.Count
		total += row.Count
	}

	return c.JSON(http.StatusOK, map[string]any{
		"pending_review_count": total,
		"counts_by_source":     countsBySource,
	})
}

// HandleJobTypeStatus handles GET /api/jobs/status/:job_type.
// Lightweight status for any job type: the current active job (if any) plus the
// most recent terminal job, so the UI can poll continuously and detect
// completion via the active_job_id non-null → null transition.
func (h *JobsHandler) HandleJobTypeStatus(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	jobType := c.Param("job_type")
	ctx := context.Background()

	var activeJobID *string
	var activeID string
	err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = ? AND status IN ('pending', 'processing') ORDER BY created_at DESC LIMIT 1`,
		userID, jobType,
	).Scan(ctx, &activeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job status")
	}
	if err == nil {
		activeJobID = &activeID
	}

	var lastCompletedJobID *string
	var lastCompletedAt *time.Time
	var last struct {
		ID          string     `bun:"id"`
		CompletedAt *time.Time `bun:"completed_at"`
	}
	err = h.db.NewRaw(
		`SELECT id, completed_at FROM jobs WHERE user_id = ? AND job_type = ? AND status IN ('completed', 'failed', 'cancelled') ORDER BY completed_at DESC NULLS LAST, created_at DESC LIMIT 1`,
		userID, jobType,
	).Scan(ctx, &last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job status")
	}
	if err == nil {
		lastCompletedJobID = &last.ID
		lastCompletedAt = last.CompletedAt
	}

	return c.JSON(http.StatusOK, map[string]any{
		"is_active":             activeJobID != nil,
		"active_job_id":         activeJobID,
		"last_completed_job_id": lastCompletedJobID,
		"last_completed_at":     lastCompletedAt,
	})
}

// syncChangeItem is a summary of a changes row for the recent jobs endpoint.
// UserGameID is populated for changes that resulted in a library entry (added,
// updated, status_changed, already_in_library); the frontend renders the title
// as a link to /games/$id when present and as plain text otherwise.
type syncChangeItem struct {
	Title      string  `bun:"title"        json:"title"`
	UserGameID *string `bun:"user_game_id" json:"user_game_id,omitempty"`
	OldStatus  *string `bun:"old_status"   json:"old_status,omitempty"`
	NewStatus  *string `bun:"new_status"   json:"new_status,omitempty"`
}

// HandleRecentJobs handles GET /api/jobs/recent (filters: source, job_type, days_back, limit).
func (h *JobsHandler) HandleRecentJobs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	source := c.QueryParam("source")

	// job_type: accept repeated params and/or comma-separated values.
	var jobTypes []string
	for _, raw := range c.QueryParams()["job_type"] {
		for t := range strings.SplitSeq(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				jobTypes = append(jobTypes, t)
			}
		}
	}

	daysBack, _ := strconv.Atoi(c.QueryParam("days_back")) //nolint:errcheck // invalid/empty query param clamped to default below
	if daysBack < 1 {
		daysBack = 7
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit")) //nolint:errcheck // invalid/empty query param clamped to default below
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	var jobs []models.Job
	q := h.db.NewSelect().Model(&jobs).
		Where("user_id = ?", userID).
		Where("status IN ('completed', 'failed')").
		Where("created_at >= now() - make_interval(days => ?)", daysBack).
		Order("created_at DESC").
		Limit(limit)
	if source != "" {
		q = q.Where("source = ?", source)
	}
	if len(jobTypes) > 0 {
		q = q.Where("job_type IN (?)", bun.List(jobTypes))
	}
	if err := q.Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get recent jobs")
	}
	if jobs == nil {
		jobs = []models.Job{}
	}

	reqCtx := c.Request().Context()

	type jobWithChanges struct {
		models.Job
		Progress              map[string]any   `json:"progress"`
		AddedItems            []syncChangeItem `json:"added_items"`
		UpdatedItems          []syncChangeItem `json:"updated_items"`
		RemovedItems          []syncChangeItem `json:"removed_items"`
		StatusChangedItems    []syncChangeItem `json:"status_changed_items"`
		SkippedItems          []syncChangeItem `json:"skipped_items"`
		AlreadyInLibraryItems []syncChangeItem `json:"already_in_library_items"`
	}

	result := make([]jobWithChanges, 0, len(jobs))
	for _, j := range jobs {
		progress, err := h.jobItemCounts(ctx, j.ID)
		if err != nil {
			slog.ErrorContext(reqCtx, "HandleRecentJobs: failed to count job items", logging.KeyJobID, j.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			progress = map[string]any{
				"pending": 0, "processing": 0, "completed": 0, "pending_review": 0,
				"skipped": 0, "failed": 0, "total": 0, "percent": 0,
			}
		}

		var allChanges []struct {
			ChangeType string  `bun:"change_type"`
			Title      string  `bun:"title"`
			UserGameID *string `bun:"user_game_id"`
			OldStatus  *string `bun:"old_status"`
			NewStatus  *string `bun:"new_status"`
		}
		if err := h.db.NewRaw(`
			SELECT change_type, title, user_game_id, old_status, new_status
			FROM changes
			WHERE job_id = ?
			ORDER BY created_at`,
			j.ID,
		).Scan(ctx, &allChanges); err != nil {
			slog.ErrorContext(reqCtx, "HandleRecentJobs: failed to query changes", logging.KeyJobID, j.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			allChanges = nil
		}

		addedItems := []syncChangeItem{}
		updatedItems := []syncChangeItem{}
		removedItems := []syncChangeItem{}
		statusChangedItems := []syncChangeItem{}
		skippedItems := []syncChangeItem{}
		alreadyInLibraryItems := []syncChangeItem{}
		for _, sc := range allChanges {
			switch sc.ChangeType {
			case "added":
				addedItems = append(addedItems, syncChangeItem{Title: sc.Title, UserGameID: sc.UserGameID})
			case "updated":
				updatedItems = append(updatedItems, syncChangeItem{Title: sc.Title, UserGameID: sc.UserGameID})
			case "removed":
				removedItems = append(removedItems, syncChangeItem{Title: sc.Title})
			case "status_changed":
				statusChangedItems = append(statusChangedItems, syncChangeItem{
					Title: sc.Title, UserGameID: sc.UserGameID, OldStatus: sc.OldStatus, NewStatus: sc.NewStatus,
				})
			case "skipped":
				skippedItems = append(skippedItems, syncChangeItem{Title: sc.Title})
			case "already_in_library":
				alreadyInLibraryItems = append(alreadyInLibraryItems, syncChangeItem{Title: sc.Title, UserGameID: sc.UserGameID})
			}
		}

		result = append(result, jobWithChanges{
			Job:                   j,
			Progress:              progress,
			AddedItems:            addedItems,
			UpdatedItems:          updatedItems,
			RemovedItems:          removedItems,
			StatusChangedItems:    statusChangedItems,
			SkippedItems:          skippedItems,
			AlreadyInLibraryItems: alreadyInLibraryItems,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"jobs": result})
}

// HandleGetJob handles GET /api/jobs/:id.
func (h *JobsHandler) HandleGetJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")
	ctx := context.Background()

	var job models.Job
	err := h.db.NewSelect().Model(&job).
		Where("id = ? AND user_id = ?", jobID, userID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	progress, err := h.jobItemCounts(ctx, job.ID)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "jobs: fetch item counts failed", logging.KeyErr, err, logging.KeyJobID, job.ID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
	}
	return c.JSON(http.StatusOK, toJobResponse(&job, progress))
}

// HandleGetJobItems handles GET /api/jobs/:id/items.
func (h *JobsHandler) HandleGetJobItems(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	// Verify job ownership.
	var ownerID string
	err := h.db.NewRaw(`SELECT user_id FROM jobs WHERE id = ?`, jobID).
		Scan(context.Background(), &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}
	if ownerID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page")) //nolint:errcheck // invalid/empty query param clamped to default below
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	statusParam := c.QueryParam("status")

	q := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID).Where("item_key <> ?", tasks.PoolsItemKey)
	countQ := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID).Where("item_key <> ?", tasks.PoolsItemKey)

	if statusParam != "" && statusParam != "pending_review" {
		q = q.Where("status = ?", statusParam)
		countQ = countQ.Where("status = ?", statusParam)
	}

	var total int
	if statusParam == "pending_review" {
		if err := h.db.NewRaw(
			`SELECT COUNT(DISTINCT source_title) FROM job_items WHERE job_id = ? AND status = 'pending_review'`,
			jobID,
		).Scan(context.Background(), &total); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
		}
	} else {
		var err error
		total, err = countQ.Count(context.Background())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
		}
	}

	offset := (page - 1) * perPage
	var items []models.JobItem
	if statusParam == "pending_review" {
		if err := h.db.NewRaw(
			`SELECT DISTINCT ON (source_title) * FROM job_items WHERE job_id = ? AND status = 'pending_review' ORDER BY source_title ASC LIMIT ? OFFSET ?`,
			jobID, perPage, offset,
		).Scan(context.Background(), &items); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
		}
	} else {
		if err := q.OrderExpr("created_at ASC").Limit(perPage).Offset(offset).Scan(context.Background(), &items); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
		}
	}
	if items == nil {
		items = []models.JobItem{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	return c.JSON(http.StatusOK, map[string]any{
		"items":       items,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// HandleCancelJob handles POST /api/jobs/:id/cancel.
func (h *JobsHandler) HandleCancelJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ? AND user_id = ?`, jobID, userID).
		Scan(context.Background(), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	if job.IsTerminal() {
		return echo.NewHTTPError(http.StatusConflict, "job is already terminal")
	}

	now := time.Now().UTC()
	_, err = h.db.NewRaw(`
		UPDATE jobs SET status = ?, completed_at = ?
		WHERE id = ?`,
		models.JobStatusCancelled, now, jobID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel job")
	}

	// Cancel any queued River jobs for this nexorious job. ImportItemArgs serialises
	// as {"job_item_id": "..."}, so match against the job_items table.
	if _, err := h.db.NewRaw(`
		UPDATE river_job
		SET state = 'cancelled', finalized_at = NOW()
		WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
		  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
		jobID,
	).Exec(context.Background()); err != nil {
		slog.ErrorContext(c.Request().Context(), "jobs: cancel river jobs failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel queued tasks")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
}

// HandleDeleteJob handles DELETE /api/jobs/:id.
func (h *JobsHandler) HandleDeleteJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ? AND user_id = ?`, jobID, userID).
		Scan(context.Background(), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	if job.IsActive() {
		return echo.NewHTTPError(http.StatusConflict, "cannot delete an active job")
	}

	_, err = h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete job")
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleRetryFailed handles POST /api/jobs/:id/retry-failed.
func (h *JobsHandler) HandleRetryFailed(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobID := c.Param("id")

	var job models.Job
	err := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ? AND user_id = ?`, jobID, userID).
		Scan(context.Background(), &job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job")
	}

	// Get failed items.
	var failedItems []models.JobItem
	err = h.db.NewRaw(`
		SELECT * FROM job_items
		WHERE job_id = ? AND status = ?`,
		jobID, models.JobItemStatusFailed,
	).Scan(context.Background(), &failedItems)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get failed items")
	}

	if len(failedItems) == 0 {
		return c.JSON(http.StatusOK, map[string]any{
			"success":       true,
			"message":       "No failed items to retry",
			"retried_count": 0,
		})
	}

	// Reset failed items to pending.
	_, err = h.db.NewRaw(`
		UPDATE job_items
		SET status = ?, error_message = NULL, processed_at = NULL
		WHERE job_id = ? AND status = ?`,
		models.JobItemStatusPending, jobID, models.JobItemStatusFailed,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset items")
	}

	// Reset job status to processing.
	if _, err := h.db.NewRaw(`
		UPDATE jobs SET status = ?, auto_retry_done = false WHERE id = ?`,
		models.JobStatusProcessing, jobID,
	).Exec(context.Background()); err != nil {
		slog.ErrorContext(c.Request().Context(), "jobs: reset job status failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset job status")
	}

	// Submit tasks for each reset item.
	for _, item := range failedItems {
		retryInsert(context.Background(), h.db, h.riverClient, job.JobType, job.Source, item.ID)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":       true,
		"message":       fmt.Sprintf("Retrying %d failed item(s)", len(failedItems)),
		"retried_count": len(failedItems),
	})
}

// retryInsert enqueues a River job for the given job_item. On any failure
// (nil client, unknown job_type, River error) the job_item is marked 'failed'
// by EnqueueOrFail so it does not get stranded in 'pending' with no backing
// river_job.
func retryInsert(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobType, source, jobItemID string) {
	args, err := tasks.ArgsForJobType(jobType, source, jobItemID)
	if err != nil {
		slog.ErrorContext(ctx, "retryInsert: unsupported job_type",
			"job_type", jobType, logging.KeySource, source, "job_item_id", jobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryValidation))
		return
	}
	if err := tasks.EnqueueOrFail(ctx, db, rc, jobItemID, args); err != nil {
		slog.ErrorContext(ctx, "retryInsert: enqueue failed",
			"job_type", jobType, logging.KeySource, source, "job_item_id", jobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}
