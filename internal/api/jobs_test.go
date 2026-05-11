package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
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

func newTestEchoWithPool(t *testing.T, db *bun.DB) (interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, *worker.Pool) {
	t.Helper()
	pool := worker.NewPool(db)
	cfg := testCfg()
	e := newTestEchoPool(t, db, cfg, pool)
	return e, pool
}

// ─── TestListJobs ─────────────────────────────────────────────────────────────

func TestListJobs(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-list")

	insertJob(t, db, "job-list-1", userID, "import", "steam", "completed")
	insertJob(t, db, "job-list-2", userID, "sync", "psn", "processing")

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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-get")

	insertJob(t, db, "job-get-1", userID, "import", "steam", "completed")

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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID1 := "u-jobs-owner1"
	insertAuthTestUser(t, db, userID1, "jobowner1", "pass123", true, false)
	insertJob(t, db, "job-wrong-owner", userID1, "import", "steam", "completed")

	_, token2 := setupTagUser(t, db, e, "jobs-wrong")

	rec := getAuth(t, e, "/api/jobs/job-wrong-owner", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestCancelJob ────────────────────────────────────────────────────────────

func TestCancelJob(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-cancel")

	insertJob(t, db, "job-cancel-1", userID, "sync", "steam", "processing")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancel-1/cancel", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status changed.
	var status string
	err := db.QueryRowContext(context.Background(),
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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-cancel-term")

	insertJob(t, db, "job-cancel-term", userID, "sync", "steam", "completed")

	rec := postJSONAuth(t, e, "/api/jobs/job-cancel-term/cancel", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestDeleteJob ────────────────────────────────────────────────────────────

func TestDeleteJob(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-delete")

	insertJob(t, db, "job-del-1", userID, "import", "steam", "completed")

	rec := deleteAuth(t, e, "/api/jobs/job-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify deleted.
	var count int
	err := db.QueryRowContext(context.Background(),
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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-del-active")

	insertJob(t, db, "job-del-active", userID, "sync", "steam", "processing")

	rec := deleteAuth(t, e, "/api/jobs/job-del-active", token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestJobsSummary ──────────────────────────────────────────────────────────

func TestJobsSummary(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "jobs-summary")

	insertJob(t, db, "job-sum-1", userID, "sync", "steam", "processing")
	insertJob(t, db, "job-sum-2", userID, "import", "csv", "pending")
	insertJob(t, db, "job-sum-3", userID, "sync", "psn", "failed")
	insertJob(t, db, "job-sum-4", userID, "import", "steam", "completed")

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
