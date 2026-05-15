package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
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

	emptyProgress := map[string]any{
		"pending": 0, "processing": 0, "completed": 0,
		"pending_review": 0, "skipped": 0, "failed": 0,
		"total": 0, "percent": 0,
	}
	jobDTOs := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], emptyProgress))
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

	var count int
	err := h.db.NewRaw(`
		SELECT COUNT(*) FROM job_items
		WHERE user_id = ? AND status = ?`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &count)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count pending reviews")
	}

	return c.JSON(http.StatusOK, map[string]int{"count": count})
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

	// Fall back to most recent completed/failed/cancelled job.
	if errors.Is(err, sql.ErrNoRows) {
		err = h.db.NewSelect().Model(&job).
			Where("user_id = ? AND job_type = ?", userID, jobType).
			OrderExpr("created_at DESC").
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
	SourceTitle  string  `json:"source_title"`
	Status       string  `json:"status"`
	GameTitle    *string `json:"game_title"`
	IsNewAdd     *bool   `json:"is_new_addition"`
	UserGameID   *string `json:"user_game_id"`
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
		WHERE user_id = ? AND source = ? AND status IN ('completed', 'failed')
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
		Items []recentJobItem `json:"items"`
	}

	result := make([]jobWithItems, 0, len(jobs))
	for _, j := range jobs {
		var items []recentJobItem
		err := h.db.NewRaw(`
			SELECT source_title, status,
			       result->>'game_title' AS game_title,
			       (result->>'is_new_addition')::boolean AS is_new_addition,
			       result->>'user_game_id' AS user_game_id
			FROM job_items
			WHERE job_id = ?
			ORDER BY created_at`,
			j.ID,
		).Scan(context.Background(), &items)
		if err != nil {
			items = []recentJobItem{}
		}
		if items == nil {
			items = []recentJobItem{}
		}
		result = append(result, jobWithItems{Job: j, Items: items})
	}

	return c.JSON(http.StatusOK, result)
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

	q := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID)
	countQ := h.db.NewSelect().TableExpr("job_items").Where("job_id = ?", jobID)

	if st := c.QueryParam("status"); st != "" {
		q = q.Where("status = ?", st)
		countQ = countQ.Where("status = ?", st)
	}

	total, err := countQ.Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
	}

	offset := (page - 1) * perPage
	var items []models.JobItem
	err = q.OrderExpr("created_at ASC").Limit(perPage).Offset(offset).
		Scan(context.Background(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
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

	// Cancel any queued River jobs for this nexorious job.
	_, _ = h.db.NewRaw(`
		UPDATE river_job
		SET state = 'cancelled', finalized_at = NOW()
		WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
		  AND args->>'job_id' = ?`,
		jobID,
	).Exec(context.Background())

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
		return c.JSON(http.StatusOK, map[string]any{"retried": 0})
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
	_, _ = h.db.NewRaw(`
		UPDATE jobs SET status = ? WHERE id = ?`,
		models.JobStatusProcessing, jobID,
	).Exec(context.Background())

	// Submit tasks for each reset item.
	for _, item := range failedItems {
		retryInsert(context.Background(), h.riverClient, job.JobType, item.ID)
	}

	return c.JSON(http.StatusOK, map[string]any{"retried": len(failedItems)})
}

func retryInsert(ctx context.Context, rc *river.Client[pgx.Tx], jobType, jobItemID string) {
	if rc == nil {
		return
	}
	switch jobType {
	case models.JobTypeSync:
		_, _ = rc.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: jobItemID}, nil)
	case models.JobTypeImport:
		_, _ = rc.Insert(ctx, tasks.ImportItemArgs{JobItemID: jobItemID}, nil)
	case models.JobTypeMetadataRefresh:
		_, _ = rc.Insert(ctx, tasks.MetadataRefreshItemArgs{JobItemID: jobItemID}, nil)
	}
}
