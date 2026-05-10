package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// ─── TestGetJobItem ────────────────────────────────────────────────────────────

func TestGetJobItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-get")

	insertJob(t, db, "job-ji-get", userID, "import", "steam", "processing")
	insertJobItem(t, db, "ji-get-1", "job-ji-get", userID, "key1", "My Game", "pending_review")

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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	ownerID := "u-ji-owner"
	insertAuthTestUser(t, db, ownerID, "jiowner", "pass123", true, false)

	insertJob(t, db, "job-ji-wrong", ownerID, "import", "steam", "processing")
	insertJobItem(t, db, "ji-wrong-1", "job-ji-wrong", ownerID, "key2", "Other Game", "pending_review")

	_, token2 := setupTagUser(t, db, e, "ji-other")

	rec := getAuth(t, e, "/api/job-items/ji-wrong-1", token2)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestResolveItem ──────────────────────────────────────────────────────────

func TestResolveItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-resolve")

	insertJob(t, db, "job-ji-resolve", userID, "import", "steam", "processing")
	insertJobItem(t, db, "ji-resolve-1", "job-ji-resolve", userID, "key3", "Resolve Game", "pending_review")

	body := map[string]any{"igdb_id": 99999}
	rec := postJSONAuth(t, e, "/api/job-items/ji-resolve-1/resolve", body, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	var resolvedIGDBID *int
	err := db.QueryRowContext(context.Background(),
		"SELECT status, resolved_igdb_id FROM job_items WHERE id = 'ji-resolve-1'",
	).Scan(&status, &resolvedIGDBID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "pending" {
		t.Fatalf("expected status=pending, got %s", status)
	}
	if resolvedIGDBID == nil || *resolvedIGDBID != 99999 {
		t.Fatalf("expected resolved_igdb_id=99999, got %v", resolvedIGDBID)
	}
}

// ─── TestResolveItem_NotPendingReview ─────────────────────────────────────────

func TestResolveItem_NotPendingReview(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-resolve-409")

	insertJob(t, db, "job-ji-resolve-409", userID, "import", "steam", "completed")
	insertJobItem(t, db, "ji-resolve-409-1", "job-ji-resolve-409", userID, "key4", "Done Game", "completed")

	body := map[string]any{"igdb_id": 12345}
	rec := postJSONAuth(t, e, "/api/job-items/ji-resolve-409-1/resolve", body, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestSkipItem ─────────────────────────────────────────────────────────────

func TestSkipItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-skip")

	insertJob(t, db, "job-ji-skip", userID, "import", "steam", "processing")
	insertJobItem(t, db, "ji-skip-1", "job-ji-skip", userID, "key5", "Skip Game", "pending_review")

	rec := postJSONAuth(t, e, "/api/job-items/ji-skip-1/skip", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	err := db.QueryRowContext(context.Background(),
		"SELECT status FROM job_items WHERE id = 'ji-skip-1'",
	).Scan(&status)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "skipped" {
		t.Fatalf("expected status=skipped, got %s", status)
	}
}

// ─── TestRetryItem ────────────────────────────────────────────────────────────

func TestRetryItem(t *testing.T) {
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-retry")

	insertJob(t, db, "job-ji-retry", userID, "import", "steam", "failed")
	insertJobItem(t, db, "ji-retry-1", "job-ji-retry", userID, "key6", "Retry Game", "failed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-1/retry", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify DB state.
	var status string
	err := db.QueryRowContext(context.Background(),
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
	db := setupAuthTestDB(t)
	e, _ := newTestEchoWithPool(t, db)

	userID, token := setupTagUser(t, db, e, "ji-retry-409")

	insertJob(t, db, "job-ji-retry-409", userID, "import", "steam", "completed")
	insertJobItem(t, db, "ji-retry-409-1", "job-ji-retry-409", userID, "key7", "Complete Game", "completed")

	rec := postJSONAuth(t, e, "/api/job-items/ji-retry-409-1/retry", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
