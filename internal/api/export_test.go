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

func TestExport_Success(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		suffix        string
		gameID        int
		title         string
		ugID          string
		assertEstimat bool
	}{
		{
			name:          "json",
			endpoint:      "/api/export/json",
			suffix:        "exp-json-ok",
			gameID:        1,
			title:         "Test Game",
			ugID:          "ug1",
			assertEstimat: true,
		},
		{
			name:     "csv",
			endpoint: "/api/export/csv",
			suffix:   "exp-csv-ok",
			gameID:   2,
			title:    "Test Game 2",
			ugID:     "ug2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			cfg := testCfg()
			e := newTestEchoPool(t, testDB, cfg)

			userID, token := setupTagUser(t, testDB, e, tt.suffix)

			// Insert a game and user_game so the user has something to export.
			ctx := context.Background()
			if _, err := testDB.ExecContext(ctx, `INSERT INTO games (id, title) VALUES (?, ?)`, tt.gameID, tt.title); err != nil {
				t.Fatalf("insert game: %v", err)
			}
			if _, err := testDB.ExecContext(ctx,
				`INSERT INTO user_games (id, user_id, game_id, is_loved) VALUES (?, ?, ?, false)`,
				tt.ugID, userID, tt.gameID,
			); err != nil {
				t.Fatalf("insert user_game: %v", err)
			}

			rec := postJSONAuth(t, e, tt.endpoint, nil, token)

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
			if tt.assertEstimat {
				estimatedItems, ok := resp["estimated_items"].(float64)
				if !ok || int(estimatedItems) != 1 {
					t.Fatalf("estimated_items = %v, want 1", resp["estimated_items"])
				}
			}
		})
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

// TestDownload_BadJobState verifies a 400 for jobs that are not a completed
// export: a non-export (import) job, and an export job that hasn't completed.
func TestDownload_BadJobState(t *testing.T) {
	tests := []struct {
		name    string
		suffix  string
		jobID   string
		jobType string
		source  string
		status  string
	}{
		{
			name:    "not an export job",
			suffix:  "dl-notexport",
			jobID:   "dl-import-job",
			jobType: "import",
			source:  "steam",
			status:  "completed",
		},
		{
			name:    "export job not completed",
			suffix:  "dl-notcomplete",
			jobID:   "dl-pending-export",
			jobType: "export",
			source:  "nexorious",
			status:  "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			cfg := testCfg()
			e := newTestEchoPool(t, testDB, cfg)

			userID, token := setupTagUser(t, testDB, e, tt.suffix)
			insertJob(t, testDB, tt.jobID, userID, tt.jobType, tt.source, tt.status)

			rec := getAuth(t, e, "/api/export/"+tt.jobID+"/download", token)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
			}
		})
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
