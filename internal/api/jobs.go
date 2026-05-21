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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
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
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) map[string]any {
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	_ = h.db.NewRaw(`
		SELECT status, COUNT(*)::int AS count
		FROM job_items
		WHERE job_id = ?
		GROUP BY status`,
		jobID,
	).Scan(ctx, &counts)

	m := map[string]int{
		"pending": 0, "processing": 0, "completed": 0,
		"pending_review": 0, "skipped": 0, "failed": 0, "igdb_failed": 0,
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
		"igdb_failed": m["igdb_failed"],
		"total": total, "percent": percent,
	}
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

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
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

	jobDTOs := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		progress := h.jobItemCounts(context.Background(), jobs[i].ID)
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
		SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		WHERE ji.user_id = ? AND ji.status = ?
		  AND j.status IN ('pending', 'processing')
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

// HandleActiveJob handles GET /api/jobs/active/:job_type.
func (h *JobsHandler) HandleActiveJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	jobType := c.Param("job_type")
	ctx := context.Background()

	// Try in-progress job first.
	var job models.Job
	err := h.db.NewSelect().Model(&job).
		Where("user_id = ? AND job_type = ? AND status IN ('pending', 'processing')", userID, jobType).
		OrderExpr("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get active job")
	}

	// Fall back to most recent terminal job — order by completed_at so the
	// result is deterministic even for old rows that have zero created_at.
	if errors.Is(err, sql.ErrNoRows) {
		err = h.db.NewSelect().Model(&job).
			Where("user_id = ? AND job_type = ?", userID, jobType).
			OrderExpr("completed_at DESC NULLS LAST, created_at DESC NULLS LAST").
			Limit(1).
			Scan(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return c.JSON(http.StatusOK, nil)
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to get active job")
		}
	}

	progress := h.jobItemCounts(ctx, job.ID)
	return c.JSON(http.StatusOK, toJobResponse(&job, progress))
}

// recentJobItem is a summary of a job item for the recent jobs endpoint.
type recentJobItem struct {
	SourceTitle string  `bun:"source_title" json:"source_title"`
	Status      string  `bun:"status" json:"status"`
	GameTitle   *string `bun:"game_title" json:"game_title"`
	IsNewAdd    *bool   `bun:"is_new_addition" json:"is_new_addition"`
	UserGameID  *string `bun:"user_game_id" json:"user_game_id"`
}

// HandleRecentJobs handles GET /api/jobs/recent/:source.
func (h *JobsHandler) HandleRecentJobs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	source := c.Param("source")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	var jobs []models.Job
	err := h.db.NewRaw(`
		SELECT * FROM jobs
		WHERE user_id = ? AND source = ? AND status IN ('completed', 'failed', 'completed_with_errors')
		ORDER BY created_at DESC
		LIMIT ?`,
		userID, source, limit,
	).Scan(context.Background(), &jobs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get recent jobs")
	}
	if jobs == nil {
		jobs = []models.Job{}
	}

	type jobWithItems struct {
		models.Job
		CompletedItems  []recentJobItem `json:"completed_items"`
		SkippedItems    []recentJobItem `json:"skipped_items"`
		FailedItems     []recentJobItem `json:"failed_items"`
		IGDBFailedItems []recentJobItem `json:"igdb_failed_items"`
	}

	result := make([]jobWithItems, 0, len(jobs))
	for _, j := range jobs {
		var allItems []recentJobItem
		err := h.db.NewRaw(`
			SELECT source_title, status,
			       result->>'game_title' AS game_title,
			       (result->>'is_new_addition')::boolean AS is_new_addition,
			       result->>'user_game_id' AS user_game_id
			FROM job_items
			WHERE job_id = ?
			ORDER BY created_at`,
			j.ID,
		).Scan(context.Background(), &allItems)
		if err != nil {
			allItems = nil
		}

		completedItems := []recentJobItem{}
		skippedItems := []recentJobItem{}
		failedItems := []recentJobItem{}
		igdbFailedItems := []recentJobItem{}
		for _, item := range allItems {
			switch item.Status {
			case models.JobItemStatusCompleted:
				completedItems = append(completedItems, item)
			case models.JobItemStatusSkipped:
				skippedItems = append(skippedItems, item)
			case models.JobItemStatusFailed:
				failedItems = append(failedItems, item)
			case models.JobItemStatusIGDBFailed:
				igdbFailedItems = append(igdbFailedItems, item)
			}
		}
		result = append(result, jobWithItems{
			Job:             j,
			CompletedItems:  completedItems,
			SkippedItems:    skippedItems,
			FailedItems:     failedItems,
			IGDBFailedItems: igdbFailedItems,
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

	progress := h.jobItemCounts(ctx, job.ID)
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

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	statusParam := c.QueryParam("status")

	q := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID)
	countQ := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID)

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
		slog.Error("jobs: cancel river jobs failed", "err", err, "job_id", jobID)
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

	// Get failed and igdb_failed items.
	var failedItems []models.JobItem
	err = h.db.NewRaw(`
		SELECT * FROM job_items
		WHERE job_id = ? AND status IN (?, ?)`,
		jobID, models.JobItemStatusFailed, models.JobItemStatusIGDBFailed,
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

	// Reset failed + igdb_failed items to pending.
	_, err = h.db.NewRaw(`
		UPDATE job_items
		SET status = ?, error_message = NULL, processed_at = NULL
		WHERE job_id = ? AND status IN (?, ?)`,
		models.JobItemStatusPending, jobID, models.JobItemStatusFailed, models.JobItemStatusIGDBFailed,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset items")
	}

	// Reset job status to processing and clear auto_retry_done so that a
	// subsequent IGDB failure can trigger another automatic retry cycle.
	if _, err := h.db.NewRaw(`
		UPDATE jobs SET status = ?, auto_retry_done = false WHERE id = ?`,
		models.JobStatusProcessing, jobID,
	).Exec(context.Background()); err != nil {
		slog.Error("jobs: reset job status failed", "err", err, "job_id", jobID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset job status")
	}

	// Submit tasks for each reset item.
	for _, item := range failedItems {
		retryInsert(context.Background(), h.db, h.riverClient, job.JobType, item.ID)
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
func retryInsert(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobType, jobItemID string) {
	args, err := tasks.ArgsForJobType(jobType, jobItemID)
	if err != nil {
		slog.Error("retryInsert: unsupported job_type",
			"job_type", jobType, "job_item_id", jobItemID, "err", err)
		return
	}
	if err := tasks.EnqueueOrFail(ctx, db, rc, jobItemID, args); err != nil {
		slog.Error("retryInsert: enqueue failed",
			"job_type", jobType, "job_item_id", jobItemID, "err", err)
	}
}
