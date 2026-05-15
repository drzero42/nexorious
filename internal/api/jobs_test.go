package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/uptrace/bun"

)

func insertJob(t *testing.T, db *bun.DB, id, userID, jobType, source, status string) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
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
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())`,
		id, jobID, userID, itemKey, sourceTitle, status,
	)
	if err != nil {
		t.Fatalf("insertJobItem: %v", err)
	}
}

func newTestEchoWithPool(t *testing.T, db *bun.DB) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	return newTestEchoPool(t, testDB, cfg)
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
	if resp["count"].(float64) != 0 {
		t.Fatalf("expected count=0, got %v", resp["count"])
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
	if resp["count"].(float64) != 1 {
		t.Fatalf("expected count=1, got %v", resp["count"])
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
	if resp["retried"].(float64) != 0 {
		t.Fatalf("expected retried=0, got %v", resp["retried"])
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
	if resp["retried"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried"])
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
	if resp["retried"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried"])
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
	if resp["retried"].(float64) != 1 {
		t.Fatalf("expected retried=1, got %v", resp["retried"])
	}
}
