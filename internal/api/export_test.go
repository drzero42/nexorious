package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

)

// ─── Export tests ────────────────────────────────────────────────────────────

func TestExportJSON_NoGames(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "exp-nojson")

	rec := postJSONAuth(t, e, "/api/export/json", nil, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestExportJSON_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "exp-json-ok")

	// Insert a game and user_game so the user has something to export.
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `INSERT INTO games (id, title) VALUES (1, 'Test Game')`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, is_loved) VALUES ('ug1', ?, 1, false)`,
		userID,
	); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/export/json", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["job_id"] == nil || resp["job_id"] == "" {
		t.Fatalf("expected non-empty job_id, got %v", resp["job_id"])
	}
	if resp["status"] != "pending" {
		t.Fatalf("status = %v, want pending", resp["status"])
	}
	estimatedItems, ok := resp["estimated_items"].(float64)
	if !ok || int(estimatedItems) != 1 {
		t.Fatalf("estimated_items = %v, want 1", resp["estimated_items"])
	}
}

func TestExportCSV_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "exp-csv-ok")

	// Insert a game and user_game so the user has something to export.
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `INSERT INTO games (id, title) VALUES (2, 'Test Game 2')`); err != nil {
		t.Fatalf("insert game: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, is_loved) VALUES ('ug2', ?, 2, false)`,
		userID,
	); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/export/csv", nil, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp["job_id"] == nil || resp["job_id"] == "" {
		t.Fatalf("expected non-empty job_id, got %v", resp["job_id"])
	}
	if resp["status"] != "pending" {
		t.Fatalf("status = %v, want pending", resp["status"])
	}
}

// ─── Download tests ───────────────────────────────────────────────────────────

// insertCompletedExportJob inserts a completed export job with an optional file_path and completed_at.
func insertCompletedExportJob(t *testing.T, db *bun.DB, id, userID, jobType, filePath string, completedAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	if filePath != "" && completedAt != nil {
		_, err := db.ExecContext(ctx,
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at, created_at)
			 VALUES (?, ?, ?, 'nexorious', ?, 'normal', ?, ?, now())`,
			id, userID, jobType, "completed", filePath, completedAt,
		)
		if err != nil {
			t.Fatalf("insertCompletedExportJob: %v", err)
		}
	} else if filePath != "" {
		_, err := db.ExecContext(ctx,
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, created_at)
			 VALUES (?, ?, ?, 'nexorious', ?, 'normal', ?, now())`,
			id, userID, jobType, "completed", filePath,
		)
		if err != nil {
			t.Fatalf("insertCompletedExportJob: %v", err)
		}
	} else {
		insertJob(t, db, id, userID, jobType, "nexorious", "pending")
	}
}

func TestDownload_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "dl-notfound")

	rec := getAuth(t, e, "/api/export/nonexistent-id/download", token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body)
	}
}

func TestDownload_NotExportJob(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "dl-notexport")

	// Insert an import job (not an export job).
	insertJob(t, testDB, "dl-import-job", userID, "import", "steam", "completed")

	rec := getAuth(t, e, "/api/export/dl-import-job/download", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestDownload_NotCompleted(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "dl-notcomplete")

	// Insert a pending export job.
	insertJob(t, testDB, "dl-pending-export", userID, "export", "nexorious", "pending")

	rec := getAuth(t, e, "/api/export/dl-pending-export/download", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestDownload_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "dl-success")

	// Create a temp dir with a real JSON file.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "export.json")
	if err := os.WriteFile(filePath, []byte(`{"games":[]}`), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	now := time.Now()
	insertCompletedExportJob(t, testDB, "dl-success-job", userID, "export", filePath, &now)

	rec := getAuth(t, e, "/api/export/dl-success-job/download", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Fatalf("Content-Type = %q, want it to contain 'json'", ct)
	}
}

func TestDownload_WrongOwner(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	// user2 owns the job; user1 makes the request.
	userID2 := "u-dl-owner2"
	insertAuthTestUser(t, testDB, userID2, "dlowner2", "pass123", true, false)
	now := time.Now()
	insertCompletedExportJob(t, testDB, "dl-wrong-owner-job", userID2, "export", "/tmp/fake.json", &now)

	_, token1 := setupTagUser(t, testDB, e, "dl-wrongowner")

	rec := getAuth(t, e, "/api/export/dl-wrong-owner-job/download", token1)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body)
	}
}
