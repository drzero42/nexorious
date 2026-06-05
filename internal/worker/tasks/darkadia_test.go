package tasks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// insertDarkadiaItem inserts a darkadia job + one job_item with the given payload
// as source_metadata. Returns (jobID, itemID).
func insertDarkadiaItem(t *testing.T, userID string, payload map[string]any) (string, string) {
	t.Helper()
	ctx := context.Background()
	jobID := uuid.NewString()
	itemID := uuid.NewString()
	metaBytes, _ := json.Marshal(payload)
	meta := json.RawMessage(metaBytes)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'import', 'darkadia', 'processing', 'high', 1, true, now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	title, _ := payload["title"].(string)
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'game_0', ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, title, meta,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID, itemID
}

func TestDarkadiaMatch_NoIGDBClientMarksPendingReview(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-match"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{"title": "Whatever", "play_status": "not_started", "platforms": []map[string]any{}}
	_, itemID := insertDarkadiaItem(t, userID, payload)

	w := &tasks.DarkadiaMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.DarkadiaMatchArgs]{Args: tasks.DarkadiaMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}
	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_review" {
		t.Errorf("status = %q, want pending_review", status)
	}
}

func TestDarkadiaFinalize_WritesUserGameAndPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-fin1"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Anodyne", "play_status": "completed", "is_loved": true,
		"personal_notes": "great",
		"platforms": []map[string]any{
			{"platform": "pc-windows", "storefront": "gog", "acquired_date": "2014-03-01"},
			{"platform": "mac"},
		},
	}
	jobID, itemID := insertDarkadiaItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.DarkadiaFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.DarkadiaFinalizeArgs]{Args: tasks.DarkadiaFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 42", userID).Scan(ctx); err != nil {
		t.Fatalf("user_game not written: %v", err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" || !ug.IsLoved {
		t.Errorf("user_game fields wrong: %+v", ug)
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ug.ID).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("platforms = %d, want 2", count)
	}
	var jobStatus string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "completed" {
		t.Errorf("job status = %q, want completed", jobStatus)
	}
}

func TestDarkadiaFinalize_AdditiveMergeDoesNotOverwrite(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-fin2"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ugID := uuid.NewString()
	if _, err := testDB.NewRaw(`INSERT INTO user_games (id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, created_at, updated_at)
		VALUES (?, ?, 42, 'mastered', 5, true, 'mine', now(), now())`, ugID, userID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Anodyne", "play_status": "not_started", "is_loved": false,
		"personal_rating": 2, "personal_notes": "imported",
		"platforms": []map[string]any{{"platform": "mac"}},
	}
	_, itemID := insertDarkadiaItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.DarkadiaFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.DarkadiaFinalizeArgs]{Args: tasks.DarkadiaFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", ugID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "mastered" || ug.PersonalRating == nil || *ug.PersonalRating != 5 || ug.PersonalNotes == nil || *ug.PersonalNotes != "mine" || !ug.IsLoved {
		t.Errorf("curation overwritten: %+v", ug)
	}
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("platforms = %d, want 1 (mac merged in)", count)
	}
}
