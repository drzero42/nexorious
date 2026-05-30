package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// ─── TestGetJobItem ────────────────────────────────────────────────────────────

func TestGetJobItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-get")

	insertJob(t, testDB, "job-ji-get", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-get-1", "job-ji-get", userID, "key1", "My Game", "pending_review")

	rec := getAuth(t, e, "/api/job-items/ji-get-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["id"] != "ji-get-1" {
		t.Fatalf("expected id=ji-get-1, got %v", resp["id"])
	}
	if resp["source_title"] != "My Game" {
		t.Fatalf("expected source_title=My Game, got %v", resp["source_title"])
	}
	if resp["status"] != "pending_review" {
		t.Fatalf("expected status=pending_review, got %v", resp["status"])
	}
}

// ─── TestGetJobItem_WrongOwner ────────────────────────────────────────────────

func TestGetJobItem_WrongOwner(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	ownerID := "u-ji-owner"
	insertAuthTestUser(t, testDB, ownerID, "jiowner", "pass123", true, false)

	insertJob(t, testDB, "job-ji-wrong", ownerID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-wrong-1", "job-ji-wrong", ownerID, "key2", "Other Game", "pending_review")

	_, token2 := setupTagUser(t, testDB, e, "ji-other")

	rec := getAuth(t, e, "/api/job-items/ji-wrong-1", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestRetryItem ────────────────────────────────────────────────────────────

func TestRetryItem(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-retry")

	insertJob(t, testDB, "job-ji-retry", userID, "import", "steam", "failed")
	insertJobItem(t, testDB, "ji-retry-1", "job-ji-retry", userID, "key6", "Retry Game", "failed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-1/retry", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	err := testDB.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-retry-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending, got %s", status)
	}
}

// ─── TestRetryItem_NotFailed ──────────────────────────────────────────────────

func TestRetryItem_NotFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "ji-retry-409")

	insertJob(t, testDB, "job-ji-retry-409", userID, "import", "steam", "completed")
	insertJobItem(t, testDB, "ji-retry-409-1", "job-ji-retry-409", userID, "key7", "Complete Game", "completed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-409-1/retry", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
