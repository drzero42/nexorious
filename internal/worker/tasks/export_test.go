package tasks_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

func TestExportJSON_Task(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	userID := uuid.NewString()
	insertTestUser(t, db, userID)

	// Insert a game.
	releaseDate := time.Date(2020, 3, 15, 0, 0, 0, 0, time.UTC)
	game := &models.Game{
		ID:          42000,
		Title:       "Export Test Game",
		ReleaseDate: &releaseDate,
		LastUpdated: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(game).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}

	// Insert a user_game.
	rating := int32(8)
	hours := float64(55.5)
	status := "completed"
	ug := &models.UserGame{
		ID:             uuid.NewString(),
		UserID:         userID,
		GameID:         42000,
		PlayStatus:     &status,
		PersonalRating: &rating,
		IsLoved:        true,
		HoursPlayed:    &hours,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(ug).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	// Create export job.
	jobID := uuid.NewString()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'export', 'nexorious', 'pending', 'normal', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert export job: %v", err)
	}

	// Build and run the handler.
	handler := tasks.NewExportJSONHandler(db, tmpDir)
	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_json",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	if err := handler(ctx, task); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// ── Verify Job is completed ────────────────────────────────────────────
	var job models.Job
	if err := db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != models.JobStatusCompleted {
		t.Errorf("job.Status = %q, want %q", job.Status, models.JobStatusCompleted)
	}
	if job.CompletedAt == nil {
		t.Error("job.CompletedAt should be set")
	}
	if job.StartedAt == nil {
		t.Error("job.StartedAt should be set")
	}
	if job.FilePath == nil || *job.FilePath == "" {
		t.Fatal("job.FilePath should be set")
	}

	// ── Verify the file exists ─────────────────────────────────────────────
	filePath := *job.FilePath
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("export file not found at %q", filePath)
	}

	// ── Verify JSON content ────────────────────────────────────────────────
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal export JSON: %v", err)
	}

	if payload["export_version"] != "1.2" {
		t.Errorf("export_version = %v, want 1.2", payload["export_version"])
	}
	if payload["user_id"] != userID {
		t.Errorf("user_id = %v, want %v", payload["user_id"], userID)
	}
	totalGames, _ := payload["total_games"].(float64)
	if int(totalGames) != 1 {
		t.Errorf("total_games = %v, want 1", payload["total_games"])
	}
	games, _ := payload["games"].([]any)
	if len(games) != 1 {
		t.Fatalf("games count = %d, want 1", len(games))
	}

	g, _ := games[0].(map[string]any)
	if g["title"] != "Export Test Game" {
		t.Errorf("games[0].title = %v, want 'Export Test Game'", g["title"])
	}
	if g["play_status"] != "completed" {
		t.Errorf("games[0].play_status = %v, want 'completed'", g["play_status"])
	}
	if g["is_loved"] != true {
		t.Errorf("games[0].is_loved = %v, want true", g["is_loved"])
	}
	releaseYear, _ := g["release_year"].(float64)
	if int(releaseYear) != 2020 {
		t.Errorf("games[0].release_year = %v, want 2020", g["release_year"])
	}

	// File must be inside the exports/ subdir of tmpDir.
	if !strings.HasPrefix(filePath, filepath.Join(tmpDir, "exports")) {
		t.Errorf("file path %q not under %q", filePath, filepath.Join(tmpDir, "exports"))
	}
}

func TestExportCSV_Task(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	tmpDir := t.TempDir()

	userID := uuid.NewString()
	insertTestUser(t, db, userID)

	// Insert a game.
	game := &models.Game{
		ID:          43000,
		Title:       "CSV Export Game",
		LastUpdated: time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(game).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}

	// Insert a user_game.
	rating := int32(7)
	status := "playing"
	ug := &models.UserGame{
		ID:             uuid.NewString(),
		UserID:         userID,
		GameID:         43000,
		PlayStatus:     &status,
		PersonalRating: &rating,
		IsLoved:        false,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(ug).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	// Create export job.
	jobID := uuid.NewString()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'export', 'csv', 'pending', 'normal', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert export job: %v", err)
	}

	// Build and run the handler.
	handler := tasks.NewExportCSVHandler(db, tmpDir)
	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_csv",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	if err := handler(ctx, task); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// ── Verify Job is completed ────────────────────────────────────────────
	var job models.Job
	if err := db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != models.JobStatusCompleted {
		t.Errorf("job.Status = %q, want %q", job.Status, models.JobStatusCompleted)
	}
	if job.CompletedAt == nil {
		t.Error("job.CompletedAt should be set")
	}
	if job.FilePath == nil || *job.FilePath == "" {
		t.Fatal("job.FilePath should be set")
	}

	// ── Verify the CSV file ────────────────────────────────────────────────
	data, err := os.ReadFile(*job.FilePath)
	if err != nil {
		t.Fatalf("read CSV file: %v", err)
	}

	r := csv.NewReader(strings.NewReader(string(data)))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// Should have header + 1 data row.
	if len(records) != 2 {
		t.Fatalf("CSV rows = %d, want 2 (header + 1 data)", len(records))
	}

	// Verify header columns.
	wantHeaders := []string{
		"title", "igdb_id", "play_status", "personal_rating", "is_loved",
		"hours_played", "personal_notes", "platforms", "tags", "release_year",
		"created_at", "updated_at",
	}
	for i, h := range wantHeaders {
		if i >= len(records[0]) {
			t.Errorf("header missing column %q at index %d", h, i)
			continue
		}
		if records[0][i] != h {
			t.Errorf("header[%d] = %q, want %q", i, records[0][i], h)
		}
	}

	// Verify data row.
	row := records[1]
	if row[0] != "CSV Export Game" {
		t.Errorf("title = %q, want 'CSV Export Game'", row[0])
	}
	if row[1] != "43000" {
		t.Errorf("igdb_id = %q, want '43000'", row[1])
	}
	if row[2] != "playing" {
		t.Errorf("play_status = %q, want 'playing'", row[2])
	}
	if row[3] != "7" {
		t.Errorf("personal_rating = %q, want '7'", row[3])
	}
	if row[4] != "false" {
		t.Errorf("is_loved = %q, want 'false'", row[4])
	}
}

func TestExportJSON_MarkJobFailed_OnWriteError(t *testing.T) {
	// Use a path that can't be written to (a file instead of a dir) to force
	// writeJSONExport to fail and trigger markJobFailed.
	db := setupTasksTestDB(t)
	ctx := context.Background()

	// Create a file where the exports dir would be — this causes MkdirAll to fail.
	tmpDir := t.TempDir()
	exportsPath := filepath.Join(tmpDir, "exports")
	if err := os.WriteFile(exportsPath, []byte("block"), 0o444); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	userID := uuid.NewString()
	insertTestUser(t, db, userID)

	jobID := uuid.NewString()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'export', 'nexorious', 'pending', 'normal', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	handler := tasks.NewExportJSONHandler(db, tmpDir)
	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_json",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Job should be marked failed.
	var status string
	if err := db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("read job status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected job status=failed when write fails, got %q", status)
	}
}

// TestExportJSON_InvalidPayload exercises the json.Unmarshal failure path.
func TestExportJSON_InvalidPayload(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewExportJSONHandler(db, t.TempDir())
	task := &models.PendingTask{Payload: json.RawMessage(`not-json`)}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestExportCSV_InvalidPayload exercises the json.Unmarshal failure path.
func TestExportCSV_InvalidPayload(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewExportCSVHandler(db, t.TempDir())
	task := &models.PendingTask{Payload: json.RawMessage(`not-json`)}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestExportJSON_JobNotFound exercises the "load job not found" path.
func TestExportJSON_JobNotFound(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewExportJSONHandler(db, t.TempDir())
	task := &models.PendingTask{Payload: mustMarshal(t, map[string]string{"job_id": "non-existent-job"})}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestExportCSV_JobNotFound exercises the "load job not found" path.
func TestExportCSV_JobNotFound(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewExportCSVHandler(db, t.TempDir())
	task := &models.PendingTask{Payload: mustMarshal(t, map[string]string{"job_id": "non-existent-job"})}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestExportCSV_MarkJobFailed_OnWriteError(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	exportsPath := filepath.Join(tmpDir, "exports")
	if err := os.WriteFile(exportsPath, []byte("block"), 0o444); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	userID := uuid.NewString()
	insertTestUser(t, db, userID)

	jobID := uuid.NewString()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'export', 'nexorious', 'pending', 'normal', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	handler := tasks.NewExportCSVHandler(db, tmpDir)
	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "export_csv",
		Payload:  mustMarshal(t, map[string]string{"job_id": jobID}),
	}
	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	if err := db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("read job status: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected job status=failed when write fails, got %q", status)
	}
}
