package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func insertJob(t *testing.T, db *bun.DB, id, userID, jobType, source, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
		 VALUES (?, ?, ?, ?, ?, 'high', now())`,
		id, userID, jobType, source, status,
	)
	if err != nil {
		t.Fatalf("insertJob: %v", err)
	}
}

func insertJobItem(t *testing.T, db *bun.DB, id, jobID, userID, itemKey, sourceTitle, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())`,
		id, jobID, userID, itemKey, sourceTitle, status,
	)
	if err != nil {
		t.Fatalf("insertJobItem: %v", err)
	}
}

// insertRiverJob inserts a minimal river_job row with the given state, whose
// args reference the provided job_item ID. Returns the generated bigserial id.
func insertRiverJob(t *testing.T, db *bun.DB, kind, state, jobItemID string) int64 {
	t.Helper()
	var id int64
	err := db.NewRaw(
		`INSERT INTO river_job (kind, max_attempts, state, args)
		 VALUES (?, 25, ?::river_job_state, jsonb_build_object('job_item_id', ?::text))
		 RETURNING id`,
		kind, state, jobItemID,
	).Scan(context.Background(), &id)
	if err != nil {
		t.Fatalf("insertRiverJob: %v", err)
	}
	return id
}

// riverJobState reads the current state of a river_job row by id.
func riverJobState(t *testing.T, db *bun.DB, id int64) string {
	t.Helper()
	var state string
	if err := db.NewRaw(`SELECT state::text FROM river_job WHERE id = ?`, id).
		Scan(context.Background(), &state); err != nil {
		t.Fatalf("riverJobState: %v", err)
	}
	return state
}

func newTestEchoWithPool(t *testing.T, db *bun.DB) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	return newTestEchoPool(t, db, cfg)
}

// ─── TestListJobs ─────────────────────────────────────────────────────────────

func TestListJobs(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-list")

	insertJob(t, testDB, "job-list-1", userID, "import", "steam", "completed")
	insertJob(t, testDB, "job-list-2", userID, "sync", "psn", "processing")

	rec := getAuth(t, e, "/api/jobs", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	total, ok := resp["total"].(float64)
	if !ok || total != 2 {
		t.Fatalf("expected total=2, got %v", resp["total"])
	}

	items, ok := resp["jobs"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %v", resp["jobs"])
	}

	if resp["page"].(float64) != 1 {
		t.Fatalf("expected page=1, got %v", resp["page"])
	}
}

// ─── TestListJobs_ProgressCounts ─────────────────────────────────────────────

func TestListJobs_ProgressCounts(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-progress")

	jobID := uuid.New().String()
	insertJob(t, testDB, jobID, userID, "import", "steam", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-2", "Game B", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-3", "Game C", "failed")

	rec := getAuth(t, e, "/api/jobs", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %v", resp["jobs"])
	}

	job, ok := jobs[0].(map[string]any)
	if !ok {
		t.Fatalf("job is not an object: %v", jobs[0])
	}

	progress, ok := job["progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress missing or wrong type: %v", job["progress"])
	}

	if got := progress["completed"].(float64); got != 2 {
		t.Errorf("expected progress.completed=2, got %v", got)
	}
	if got := progress["failed"].(float64); got != 1 {
		t.Errorf("expected progress.failed=1, got %v", got)
	}
	if got := progress["total"].(float64); got != 3 {
		t.Errorf("expected progress.total=3, got %v", got)
	}
}

// ─── TestGetJob ───────────────────────────────────────────────────────────────

func TestGetJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-get")

	insertJob(t, testDB, "job-get-1", userID, "import", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/job-get-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["id"] != "job-get-1" {
		t.Fatalf("expected id=job-get-1, got %v", resp["id"])
	}
	if resp["progress"] == nil {
		t.Fatal("expected progress field")
	}
}

// ─── TestGetJob_WrongOwner ────────────────────────────────────────────────────

func TestGetJob_WrongOwner(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID1 := "u-jobs-owner1"
	insertAuthTestUser(t, testDB, userID1, "jobowner1", "pass123", true, false)
	insertJob(t, testDB, "job-wrong-owner", userID1, "import", "steam", "completed")

	_, token2 := setupTagUser(t, testDB, e, "jobs-wrong")

	rec := getAuth(t, e, "/api/jobs/job-wrong-owner", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestCancelJob ────────────────────────────────────────────────────────────

func TestCancelJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-cancel")

	insertJob(t, testDB, "job-cancel-1", userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-cancel-1", "job-cancel-1", userID, "key-1", "Game 1", "pending")
	riverID := insertRiverJob(t, testDB, "import_item", "available", "ji-cancel-1")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancel-1/cancel", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed.
	var status string
	err := testDB.QueryRowContext(context.Background(),
		"SELECT status FROM jobs WHERE id = 'job-cancel-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "cancelled" {
		t.Fatalf("expected status=cancelled, got %s", status)
	}

	// Verify the queued river_job was cancelled too.
	if state := riverJobState(t, testDB, riverID); state != "cancelled" {
		t.Errorf("expected river_job state=cancelled, got %q", state)
	}
}

// ─── TestCancelJob_AlreadyTerminal ────────────────────────────────────────────

func TestCancelJob_AlreadyTerminal(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-cancel-term")

	insertJob(t, testDB, "job-cancel-term", userID, "sync", "steam", "completed")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancel-term/cancel", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestDeleteJob ────────────────────────────────────────────────────────────

func TestDeleteJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-delete")

	insertJob(t, testDB, "job-del-1", userID, "import", "steam", "completed")

	rec := deleteAuth(t, e, "/api/jobs/job-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted.
	var count int
	err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM jobs WHERE id = 'job-del-1'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected job to be deleted, count=%d", count)
	}
}

// ─── TestDeleteJob_ActiveReturns409 ───────────────────────────────────────────

func TestDeleteJob_ActiveReturns409(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-del-active")

	insertJob(t, testDB, "job-del-active", userID, "sync", "steam", "processing")

	rec := deleteAuth(t, e, "/api/jobs/job-del-active", token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestJobsSummary ──────────────────────────────────────────────────────────

func TestJobsSummary(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-summary")

	insertJob(t, testDB, "job-sum-1", userID, "sync", "steam", "processing")
	insertJob(t, testDB, "job-sum-2", userID, "import", "csv", "pending")
	insertJob(t, testDB, "job-sum-3", userID, "sync", "psn", "failed")
	insertJob(t, testDB, "job-sum-4", userID, "import", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/summary", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	running := resp["running"].(float64)
	failed := resp["failed"].(float64)

	if running != 2 {
		t.Fatalf("expected running=2, got %v", running)
	}
	if failed != 1 {
		t.Fatalf("expected failed=1, got %v", failed)
	}
}

// ─── TestPendingReviewCount ───────────────────────────────────────────────────

func TestPendingReviewCount_Empty(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-prc-empty")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 0 {
		t.Fatalf("expected pending_review_count=0, got %v", resp["pending_review_count"])
	}
	if _, ok := resp["counts_by_source"]; !ok {
		t.Fatal("expected counts_by_source key in response")
	}
}

func TestPendingReviewCount_WithItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-prc-items")

	insertJob(t, testDB, "job-prc-1", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-prc-1", "job-prc-1", userID, "key-prc-1", "Game A", "pending_review")
	insertJobItem(t, testDB, "ji-prc-2", "job-prc-1", userID, "key-prc-2", "Game B", "completed")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1, got %v", resp["pending_review_count"])
	}
	bySource, ok := resp["counts_by_source"].(map[string]any)
	if !ok {
		t.Fatal("expected counts_by_source to be an object")
	}
	if bySource["steam"].(float64) != 1 {
		t.Fatalf("expected counts_by_source.steam=1, got %v", bySource["steam"])
	}
}

func TestPendingReviewCount_ExcludesCancelledJobs(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-prc-cancelled")

	// cancelled job — its pending_review items must NOT inflate the badge
	insertJob(t, testDB, "job-prc-cancelled", userID, "import", "steam", "cancelled")
	insertJobItem(t, testDB, "ji-prc-c1", "job-prc-cancelled", userID, "key-c1", "Game C", "pending_review")

	// active job — its pending_review item SHOULD be counted
	insertJob(t, testDB, "job-prc-active", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-prc-a1", "job-prc-active", userID, "key-a1", "Game A", "pending_review")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1 (cancelled job excluded), got %v", resp["pending_review_count"])
	}
}

func TestPendingReviewCount_Deduplicates(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "prc-dedup")

	insertJob(t, testDB, "job-prc-dedup", userID, "sync", "psn", "processing")
	// Same source_title, different item_key (PS4 and PS5 SKUs of the same game).
	insertJobItem(t, testDB, "ji-prc-dedup-1", "job-prc-dedup", userID, "CUSA12345_00", "Call of Duty", "pending_review")
	insertJobItem(t, testDB, "ji-prc-dedup-2", "job-prc-dedup", userID, "PPSA07890_00", "Call of Duty", "pending_review")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1 (deduplicated), got %v", resp["pending_review_count"])
	}
	bySource := resp["counts_by_source"].(map[string]any)
	if bySource["psn"].(float64) != 1 {
		t.Fatalf("expected counts_by_source.psn=1 (deduplicated), got %v", bySource["psn"])
	}
}

// ─── TestHandleActiveJob ──────────────────────────────────────────────────────

func TestHandleActiveJob_NoJobs(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-active-none")

	rec := getAuth(t, e, "/api/jobs/active/import", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleActiveJob_ActiveJobExists(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-active-exists")

	insertJob(t, testDB, "job-active-1", userID, "import", "steam", "processing")

	rec := getAuth(t, e, "/api/jobs/active/import", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != "job-active-1" {
		t.Fatalf("expected id=job-active-1, got %v", resp["id"])
	}
}

func TestHandleActiveJob_FallbackToCompleted(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-active-fallback")

	// No active job, but there is a completed one.
	insertJob(t, testDB, "job-fallback-1", userID, "sync", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/active/sync", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["id"] != "job-fallback-1" {
		t.Fatalf("expected id=job-fallback-1, got %v", resp["id"])
	}
}

// ─── TestHandleRecentJobs ─────────────────────────────────────────────────────

func TestHandleRecentJobs_Empty(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-recent-empty")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d", len(resp))
	}
}

func TestHandleRecentJobs_WithJobs(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-recent-with")

	insertJob(t, testDB, "job-recent-1", userID, "sync", "steam", "completed")
	insertJobItem(t, testDB, "ji-recent-1", "job-recent-1", userID, "key-r1", "Recent Game", "completed")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 job, got %d", len(resp))
	}
}

// ─── TestHandleGetJobItems ────────────────────────────────────────────────────

func TestHandleGetJobItems_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-items-ok")

	insertJob(t, testDB, "job-items-1", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-items-1", "job-items-1", userID, "key-i1", "Game 1", "pending_review")
	insertJobItem(t, testDB, "ji-items-2", "job-items-1", userID, "key-i2", "Game 2", "completed")

	rec := getAuth(t, e, "/api/jobs/job-items-1/items", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Fatalf("expected total=2, got %v", resp["total"])
	}
}

func TestHandleGetJobItems_WithStatusFilter(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-items-filter")

	insertJob(t, testDB, "job-items-f", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-items-f1", "job-items-f", userID, "key-f1", "Game X", "pending_review")
	insertJobItem(t, testDB, "ji-items-f2", "job-items-f", userID, "key-f2", "Game Y", "completed")

	rec := getAuth(t, e, "/api/jobs/job-items-f/items?status=pending_review", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 1 {
		t.Fatalf("expected filtered total=1, got %v", resp["total"])
	}
}

func TestHandleGetJobItems_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-items-notfound")

	rec := getAuth(t, e, "/api/jobs/nonexistent-job/items", token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestHandleRetryFailed ────────────────────────────────────────────────────

func TestHandleRetryFailed_NoFailedItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-retry-nofail")

	insertJob(t, testDB, "job-retry-nf", userID, "import", "steam", "processing")
	// No failed items.

	rec := postJSONAuth(t, e, "/api/jobs/job-retry-nf/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["retried_count"].(float64) != 0 {
		t.Fatalf("expected retried_count=0, got %v", resp["retried_count"])
	}
	if resp["success"] != true {
		t.Fatalf("expected success=true, got %v", resp["success"])
	}
}

func TestHandleRetryFailed_WithFailedItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-retry-withfail")

	insertJob(t, testDB, "job-retry-wf", userID, "import", "steam", "failed")
	insertJobItem(t, testDB, "ji-retry-wf1", "job-retry-wf", userID, "key-rf1", "Failed Game", "failed")

	rec := postJSONAuth(t, e, "/api/jobs/job-retry-wf/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["retried_count"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried_count"])
	}
}

func TestHandleRetryFailed_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-retry-notfound")

	rec := postJSONAuth(t, e, "/api/jobs/nonexistent-job/retry-failed", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestListJobs_WithFilters ─────────────────────────────────────────────────

func TestListJobs_WithFilters(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-list-filters")

	insertJob(t, testDB, "job-filter-1", userID, "sync", "steam", "completed")
	insertJob(t, testDB, "job-filter-2", userID, "import", "csv", "failed")
	insertJob(t, testDB, "job-filter-3", userID, "sync", "psn", "processing")

	t.Run("filter by job_type", func(t *testing.T) {
		rec := getAuth(t, e, "/api/jobs?job_type=sync", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["total"].(float64) != 2 {
			t.Fatalf("expected 2 sync jobs, got %v", resp["total"])
		}
	})

	t.Run("filter by source", func(t *testing.T) {
		rec := getAuth(t, e, "/api/jobs?source=steam", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["total"].(float64) != 1 {
			t.Fatalf("expected 1 steam job, got %v", resp["total"])
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		rec := getAuth(t, e, "/api/jobs?status=failed", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["total"].(float64) != 1 {
			t.Fatalf("expected 1 failed job, got %v", resp["total"])
		}
	})

	t.Run("sort by job_type asc", func(t *testing.T) {
		rec := getAuth(t, e, "/api/jobs?sort_by=job_type&sort_order=asc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestCancelJob_NotFound ───────────────────────────────────────────────────

func TestCancelJob_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-cancel-notfound")

	rec := postJSONAuth(t, e, "/api/jobs/nonexistent-job/cancel", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestDeleteJob_NotFound ───────────────────────────────────────────────────

func TestDeleteJob_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-del-notfound")

	rec := deleteAuth(t, e, "/api/jobs/nonexistent-job", token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRetryFailed_SyncJobType(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-retry-sync")

	insertJob(t, testDB, "job-retry-sync", userID, "sync", "steam", "failed")
	insertJobItem(t, testDB, "ji-retry-sync1", "job-retry-sync", userID, "key-rs1", "Sync Game", "failed")

	rec := postJSONAuth(t, e, "/api/jobs/job-retry-sync/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["retried_count"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried_count"])
	}
}

func TestHandleRetryFailed_MetadataRefreshJobType(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-retry-meta")

	insertJob(t, testDB, "job-retry-meta", userID, "metadata_refresh", "system", "failed")
	insertJobItem(t, testDB, "ji-retry-meta1", "job-retry-meta", userID, "key-rm1", "Meta Game", "failed")

	rec := postJSONAuth(t, e, "/api/jobs/job-retry-meta/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["retried_count"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried_count"])
	}
}

func TestJobProgress_IncludesIGDBFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "igdb-failed-progress")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k2", "Game B", "igdb_failed")

	rec := getAuth(t, e, "/api/jobs/"+jobID, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	progress, ok := resp["progress"].(map[string]any)
	if !ok {
		t.Fatalf("expected progress map, got %T", resp["progress"])
	}
	if igdbFailed, ok := progress["igdb_failed"].(float64); !ok || igdbFailed != 1 {
		t.Errorf("expected igdb_failed=1 in progress, got %v", progress["igdb_failed"])
	}
}

func TestRetryFailed_IncludesIGDBFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "retry-igdb-failed")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")

	item1ID := uuid.NewString()
	insertJobItem(t, testDB, item1ID, jobID, userID, "k1", "Game A", "igdb_failed")
	item2ID := uuid.NewString()
	insertJobItem(t, testDB, item2ID, jobID, userID, "k2", "Game B", "failed")
	item3ID := uuid.NewString()
	insertJobItem(t, testDB, item3ID, jobID, userID, "k3", "Game C", "completed")

	rec := postAuth(t, e, "/api/jobs/"+jobID+"/retry-failed", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if retried, ok := resp["retried_count"].(float64); !ok || retried != 2 {
		t.Errorf("expected retried=2 (1 igdb_failed + 1 failed), got %v", resp["retried_count"])
	}

	var s1, s2 string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item1ID).Scan(context.Background(), &s1)
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item2ID).Scan(context.Background(), &s2)
	if s1 != "pending" {
		t.Errorf("expected igdb_failed item reset to pending, got %q", s1)
	}
	if s2 != "pending" {
		t.Errorf("expected failed item reset to pending, got %q", s2)
	}
}

func TestRecentJobs_IncludesCompletedWithErrors(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "recent-cwe")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}
}

func TestRecentJobs_ReturnsSplitItemArrays(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "recent-split")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k2", "Game B", "igdb_failed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k3", "Game C", "skipped")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	completedItems, _ := job["completed_items"].([]any)
	skippedItems, _ := job["skipped_items"].([]any)
	igdbFailedItems, _ := job["igdb_failed_items"].([]any)
	if len(completedItems) != 1 {
		t.Errorf("expected 1 completed_item, got %d", len(completedItems))
	}
	if len(skippedItems) != 1 {
		t.Errorf("expected 1 skipped_item, got %d", len(skippedItems))
	}
	if len(igdbFailedItems) != 1 {
		t.Errorf("expected 1 igdb_failed_item, got %d", len(igdbFailedItems))
	}
}

func TestHandleGetJobItems_SortsPendingReviewAlphabetically(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "items-sort")

	insertJob(t, testDB, "job-sort", userID, "import", "steam", "processing")
	// Insert in reverse alphabetical order to prove sorting isn't by insertion order.
	insertJobItem(t, testDB, "ji-sort-z", "job-sort", userID, "key-z", "Zebra Game", "pending_review")
	insertJobItem(t, testDB, "ji-sort-a", "job-sort", userID, "key-a", "Apple Game", "pending_review")
	insertJobItem(t, testDB, "ji-sort-m", "job-sort", userID, "key-m", "Mango Game", "pending_review")

	rec := getAuth(t, e, "/api/jobs/job-sort/items?status=pending_review", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	items := resp["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	want := []string{"Apple Game", "Mango Game", "Zebra Game"}
	for i, title := range want {
		got := items[i].(map[string]any)["source_title"].(string)
		if got != title {
			t.Errorf("position %d: got %q, want %q", i, got, title)
		}
	}
}

func TestHandleGetJobItems_DeduplicatesPendingReview(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "items-dedup")

	insertJob(t, testDB, "job-dedup", userID, "sync", "psn", "processing")
	// Same source_title, different item_key (PS4 and PS5 SKUs of the same game).
	insertJobItem(t, testDB, "ji-dedup-ps4", "job-dedup", userID, "CUSA12345_00", "Call of Duty", "pending_review")
	insertJobItem(t, testDB, "ji-dedup-ps5", "job-dedup", userID, "PPSA07890_00", "Call of Duty", "pending_review")
	// A distinct title to verify other items still appear.
	insertJobItem(t, testDB, "ji-dedup-other", "job-dedup", userID, "CUSA99999_00", "Other Game", "pending_review")

	rec := getAuth(t, e, "/api/jobs/job-dedup/items?status=pending_review", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Fatalf("expected total=2 (deduplicated), got %v", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items (deduplicated), got %d", len(items))
	}
	// Items must be sorted alphabetically.
	titles := []string{
		items[0].(map[string]any)["source_title"].(string),
		items[1].(map[string]any)["source_title"].(string),
	}
	if titles[0] != "Call of Duty" || titles[1] != "Other Game" {
		t.Errorf("expected [Call of Duty, Other Game], got %v", titles)
	}
}
