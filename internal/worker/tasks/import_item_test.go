package tasks_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestImportItem_BasicGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id":         int32(12345),
		"title":           "Test Game",
		"play_status":     "completed",
		"personal_rating": 9.5,
		"hours_played":    42.5,
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Verify Game was created.
	var game models.Game
	err = testDB.NewSelect().Model(&game).Where("id = ?", int32(12345)).Scan(ctx)
	if err != nil {
		t.Fatalf("game not found: %v", err)
	}
	if game.Title != "Test Game" {
		t.Errorf("game title = %q, want %q", game.Title, "Test Game")
	}

	// Verify UserGame was created with correct fields.
	var ug models.UserGame
	err = testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(12345)).Scan(ctx)
	if err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want 'completed'", ug.PlayStatus)
	}
	// 9.5 truncated to 9
	if ug.PersonalRating == nil || *ug.PersonalRating != 9 {
		t.Errorf("personal_rating = %v, want 9", ug.PersonalRating)
	}

	// Verify JobItem is completed.
	var item models.JobItem
	err = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if err != nil {
		t.Fatalf("job_item not found: %v", err)
	}
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("job_item status = %q, want %q", item.Status, models.JobItemStatusCompleted)
	}

	var result map[string]any
	if err := json.Unmarshal(item.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["is_new_addition"] != true {
		t.Errorf("is_new_addition = %v, want true", result["is_new_addition"])
	}
}

func TestImportItem_MissingIGDBID(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// No igdb_id in game data (zero value / missing).
	gameData := map[string]any{
		"title": "No ID Game",
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}})
	if err != nil {
		t.Fatalf("handler should not return error: %v", err)
	}

	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("job_item not found: %v", err)
	}
	if item.Status != models.JobItemStatusFailed {
		t.Errorf("job_item status = %q, want %q", item.Status, models.JobItemStatusFailed)
	}
	if item.ErrorMessage == nil || *item.ErrorMessage != "missing igdb_id" {
		t.Errorf("error_message = %v, want 'missing igdb_id'", item.ErrorMessage)
	}
}

func TestImportItem_DuplicateGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 2)

	gameData := map[string]any{
		"igdb_id": int32(99999),
		"title":   "Dupe Game",
	}

	// First import.
	itemID1 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID1}}); err != nil {
		t.Fatalf("first import error: %v", err)
	}

	// Second import of same game for same user.
	itemID2 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID2}}); err != nil {
		t.Fatalf("second import error: %v", err)
	}

	var item2 models.JobItem
	if err := testDB.NewSelect().Model(&item2).Where("id = ?", itemID2).Scan(ctx); err != nil {
		t.Fatalf("job_item 2 not found: %v", err)
	}
	if item2.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item2.Status)
	}
	var result map[string]any
	if err := json.Unmarshal(item2.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["already_exists"] != true {
		t.Errorf("already_exists = %v, want true", result["already_exists"])
	}

	// Only one UserGame row should exist.
	var count int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?",
		userID, int32(99999),
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("user_game count = %d, want 1", count)
	}
}

func TestImportItem_WithPlatformsAndTags(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Seed platform and storefront.
	_, err := testDB.ExecContext(ctx,
		"INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING",
	)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	_, err = testDB.ExecContext(ctx,
		"INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING",
	)
	if err != nil {
		t.Fatalf("insert storefront: %v", err)
	}

	gameData := map[string]any{
		"igdb_id": int32(77777),
		"title":   "Platform Game",
		"platforms": []any{
			map[string]any{
				"platform_name":    "pc",
				"storefront_name":  "steam",
				"hours_played":     10.0,
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
		"tags": []any{
			map[string]any{
				"name":  "Favorite",
				"color": "#ff0000",
			},
		},
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Verify job item completed.
	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("item not found: %v", err)
	}
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item.Status)
	}

	// Verify UserGame exists.
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(77777)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	// Verify UserGamePlatform.
	var ugp models.UserGamePlatform
	if err := testDB.NewSelect().Model(&ugp).Where("user_game_id = ?", ug.ID).Scan(ctx); err != nil {
		t.Fatalf("user_game_platform not found: %v", err)
	}
	if ugp.Platform == nil || *ugp.Platform != "pc" {
		t.Errorf("platform = %v, want 'pc'", ugp.Platform)
	}
	if ugp.Storefront == nil || *ugp.Storefront != "steam" {
		t.Errorf("storefront = %v, want 'steam'", ugp.Storefront)
	}

	// Verify Tag and UserGameTag.
	var tag models.Tag
	if err := testDB.NewSelect().Model(&tag).Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, "Favorite").Scan(ctx); err != nil {
		t.Fatalf("tag not found: %v", err)
	}
	if tag.Color == nil || *tag.Color != "#ff0000" {
		t.Errorf("tag color = %v, want '#ff0000'", tag.Color)
	}

	var ugt models.UserGameTag
	if err := testDB.NewSelect().Model(&ugt).Where("user_game_id = ? AND tag_id = ?", ug.ID, tag.ID).Scan(ctx); err != nil {
		t.Fatalf("user_game_tag not found: %v", err)
	}
}

func TestImportItem_PlatformsSkippedWithoutSeed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// No platform/storefront seed data — tables are empty.
	gameData := map[string]any{
		"igdb_id": int32(66666),
		"title":   "Unseeded Platform Game",
		"platforms": []any{
			map[string]any{
				"platform_name":    "pc",
				"storefront_name":  "steam",
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// UserGame is still created even when platforms are skipped.
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(66666)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	// Platform entry should NOT exist — seed data must be loaded first.
	var count int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?", ug.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if count != 0 {
		t.Errorf("platform count = %d, want 0 (seed data not loaded)", count)
	}
}

func TestImportItem_ReimportMergesPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 2)

	gameData := map[string]any{
		"igdb_id": int32(88888),
		"title":   "Merge Game",
		"platforms": []any{
			map[string]any{
				"platform_name":    "pc",
				"storefront_name":  "steam",
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
	}

	// First import — no seed, platforms stored via original name.
	itemID1 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID1}}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Seed platform and storefront.
	if _, err := testDB.ExecContext(ctx,
		"INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING",
	); err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		"INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING",
	); err != nil {
		t.Fatalf("insert storefront: %v", err)
	}

	// Second import of same game — should not duplicate platforms.
	itemID2 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID2}}); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var item2 models.JobItem
	if err := testDB.NewSelect().Model(&item2).Where("id = ?", itemID2).Scan(ctx); err != nil {
		t.Fatalf("item2 not found: %v", err)
	}
	if item2.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item2.Status)
	}

	// Still only one UserGame.
	var ugCount int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?",
		userID, int32(88888),
	).Scan(&ugCount); err != nil {
		t.Fatalf("count user_games: %v", err)
	}
	if ugCount != 1 {
		t.Errorf("user_game count = %d, want 1", ugCount)
	}

	// Only one platform entry (no duplicate from re-import).
	var ugpCount int
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(88888)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?", ug.ID,
	).Scan(&ugpCount); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if ugpCount != 1 {
		t.Errorf("user_game_platform count = %d, want 1 (no duplicates)", ugpCount)
	}
}

func TestImportItem_JobCompletion(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id": int32(55555),
		"title":   "Completion Game",
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Job should be completed.
	var job models.Job
	if err := testDB.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx); err != nil {
		t.Fatalf("job not found: %v", err)
	}
	if job.Status != models.JobStatusCompleted {
		t.Errorf("job status = %q, want %q", job.Status, models.JobStatusCompleted)
	}
	if job.CompletedAt == nil {
		t.Error("job completed_at should be set")
	}
}

func TestImportItem_PreservesTimestamps(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	createdAt := time.Date(2023, 1, 15, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2023, 6, 20, 12, 0, 0, 0, time.UTC)

	gameData := map[string]any{
		"igdb_id":    int32(44444),
		"title":      "Timestamped Game",
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(44444)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	if !ug.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", ug.CreatedAt, createdAt)
	}
	if !ug.UpdatedAt.Equal(updatedAt) {
		t.Errorf("updated_at = %v, want %v", ug.UpdatedAt, updatedAt)
	}
}

// TestImportItem_JobItemNotFound exercises the "item not found" path.
func TestImportItem_JobItemNotFound(t *testing.T) {
	truncateAllTables(t)
	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(context.Background(), &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: "non-existent-id"}}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// TestImportItem_BadSourceMetadata exercises the parse-source_metadata failure path (line 98-102).
func TestImportItem_BadSourceMetadata(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Insert a job_item with malformed source_metadata (not a JSON object with "data" key).
	itemID := uuid.NewString()
	_, err := testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, uuid.NewString(), "Bad Meta", json.RawMessage(`"not-an-object"`),
	)
	if err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("item not found: %v", err)
	}
	if item.Status != models.JobItemStatusFailed {
		t.Errorf("status = %q, want failed", item.Status)
	}
}

// TestImportItem_PartialJobCompletion exercises the checkJobCompletion pendingCount > 0 branch.
// Process only 1 of 2 items — job should remain processing.
func TestImportItem_PartialJobCompletion(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 2) // 2 total items

	gameData1 := map[string]any{
		"igdb_id": int32(33001),
		"title":   "Partial Job Game 1",
	}
	gameData2 := map[string]any{
		"igdb_id": int32(33002),
		"title":   "Partial Job Game 2",
	}
	itemID1 := insertTestJobItem(t, testDB, jobID, userID, gameData1)
	// Insert item2 as pending — not processed.
	_ = insertTestJobItem(t, testDB, jobID, userID, gameData2)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID1}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Job should still be processing since item2 is still pending.
	var jobStatus string
	if err := testDB.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id = ?", jobID).Scan(&jobStatus); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if jobStatus != "processing" {
		t.Errorf("expected job still processing, got %q", jobStatus)
	}
}

// TestImportItem_FailedItemsYieldsCompleted verifies that a job with failed items still reaches "completed".
func TestImportItem_FailedItemsYieldsCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 2) // 2 total items

	// Item1: valid game data — will succeed.
	gameData1 := map[string]any{
		"igdb_id": int32(34001),
		"title":   "Errors Job Game 1",
	}
	itemID1 := insertTestJobItem(t, testDB, jobID, userID, gameData1)

	// Item2: pre-mark as failed so it's already "done" but failed.
	itemID2 := uuid.NewString()
	_, err := testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, ?, ?, ?, 'failed', '{}', '[]')`,
		itemID2, jobID, userID, uuid.NewString(), "Failed Game", json.RawMessage(`{"data":{"igdb_id":0}}`),
	)
	if err != nil {
		t.Fatalf("insert failed item: %v", err)
	}

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID1}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Job should be "completed" — individual item failures are surfaced via job_items.
	var jobStatus string
	if err := testDB.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id = ?", jobID).Scan(&jobStatus); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if jobStatus != "completed" {
		t.Errorf("expected completed, got %q", jobStatus)
	}
}

// TestImportItem_WithReleaseDate exercises the release_date parsing fallback path.
func TestImportItem_WithReleaseDate(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	releaseDate := time.Date(2020, 3, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	gameData := map[string]any{
		"igdb_id":      int32(35001),
		"title":        "Release Date Game",
		"release_date": releaseDate,
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("item not found: %v", err)
	}
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item.Status)
	}

	// Verify the game was created with release_date set.
	var releaseNull bool
	if err := testDB.QueryRowContext(ctx,
		"SELECT release_date IS NULL FROM games WHERE id = ?", int32(35001),
	).Scan(&releaseNull); err != nil {
		t.Fatalf("query release_date: %v", err)
	}
	if releaseNull {
		t.Error("expected release_date to be set, but it was NULL")
	}
}

// TestImportItem_TagError exercises the findOrCreateTag error path via a tag name that causes
// the existing-tag lookup to succeed on re-import (exercising the existingTagIDs skip branch).
func TestImportItem_ReimportTagDedup(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 2)

	gameData := map[string]any{
		"igdb_id": int32(36001),
		"title":   "Tag Dedup Game",
		"tags": []any{
			map[string]any{"name": "indie", "color": "#aabbcc"},
		},
	}

	// First import.
	itemID1 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID1}}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import — tag already exists, existingTagIDs[tagID] should be true → skip.
	itemID2 := insertTestJobItem(t, testDB, jobID, userID, gameData)
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID2}}); err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Still only one UserGameTag.
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(36001)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	var count int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_game_tags WHERE user_game_id = ?", ug.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if count != 1 {
		t.Errorf("tag count = %d, want 1 (no duplicates)", count)
	}
}

// TestImportItem_StorefrontNotFound exercises the storefront-not-found warning path.
func TestImportItem_StorefrontNotFound(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Seed only the platform, not the storefront.
	if _, err := testDB.ExecContext(ctx,
		"INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC Windows') ON CONFLICT DO NOTHING",
	); err != nil {
		t.Fatalf("insert platform: %v", err)
	}

	gameData := map[string]any{
		"igdb_id": int32(37001),
		"title":   "Storefront Missing Game",
		"platforms": []any{
			map[string]any{
				"platform_name":    "pc-windows",
				"storefront_name":  "unknown-store", // not seeded
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("item not found: %v", err)
	}
	// Should still complete — storefront warning is non-fatal.
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item.Status)
	}

	// Platform entry should exist with NULL storefront.
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(37001)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	var sfNull bool
	if err := testDB.QueryRowContext(ctx,
		"SELECT storefront IS NULL FROM user_game_platforms WHERE user_game_id = ?", ug.ID,
	).Scan(&sfNull); err != nil {
		t.Fatalf("query storefront: %v", err)
	}
	if !sfNull {
		t.Error("expected NULL storefront for unknown storefront")
	}
}

// TestImportItem_EmitsImportCompleted verifies that processing the last item of a job
// causes an import.completed event to be written to the events table.
func TestImportItem_EmitsImportCompleted(t *testing.T) {
	truncateAllTables(t)
	notify.SetRiverClient(nil)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id": int32(38001),
		"title":   "Notify Import Game",
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Job must be completed first.
	var jobStatus string
	if err := testDB.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id = ?", jobID).Scan(&jobStatus); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if jobStatus != "completed" {
		t.Fatalf("job status = %q, want completed (prerequisite for event assertion)", jobStatus)
	}

	dedupKey := jobID + ":" + notify.TypeImportCompleted
	var count int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE dedup_key = ?", dedupKey,
	).Scan(&count); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if count != 1 {
		t.Errorf("events count for dedup_key %q = %d, want 1", dedupKey, count)
	}
}

// countChangeRows returns how many `changes` rows of a given change_type exist for a job.
func countChangeRows(t *testing.T, jobID, changeType string) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM changes WHERE job_id = ? AND change_type = ?`, jobID, changeType,
	).Scan(ctx, &n); err != nil {
		t.Fatalf("count changes: %v", err)
	}
	return n
}

func TestImportItem_WritesChangeRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	game := func(extraTag string) map[string]any {
		tags := []map[string]any{}
		if extraTag != "" {
			tags = append(tags, map[string]any{"name": extraTag})
		}
		return map[string]any{
			"igdb_id": int32(55501),
			"title":   "Change Row Game",
			"tags":    tags,
		}
	}
	runImport := func(jobID string, gd map[string]any) {
		insertTestJob(t, testDB, jobID, userID, 1)
		itemID := insertTestJobItem(t, testDB, jobID, userID, gd)
		w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
			t.Fatalf("work: %v", err)
		}
	}

	// 1) New game → 'added'.
	job1 := uuid.NewString()
	runImport(job1, game("RPG"))
	if got := countChangeRows(t, job1, "added"); got != 1 {
		t.Fatalf("added rows = %d, want 1", got)
	}

	// 2) Same game, nothing new merged → 'already_in_library'.
	job2 := uuid.NewString()
	runImport(job2, game("RPG"))
	if got := countChangeRows(t, job2, "already_in_library"); got != 1 {
		t.Fatalf("already_in_library rows = %d, want 1", got)
	}

	// 3) Same game, a new tag merged in → 'updated'.
	job3 := uuid.NewString()
	runImport(job3, game("Action"))
	if got := countChangeRows(t, job3, "updated"); got != 1 {
		t.Fatalf("updated rows = %d, want 1", got)
	}
}

// TestImportItem_EmitsImportFailedWhenItemsFail verifies that when the last item
// of a job fails (here: missing igdb_id), the job finalizes with an import.failed
// event and no import.completed event.
func TestImportItem_EmitsImportFailedWhenItemsFail(t *testing.T) {
	truncateAllTables(t)
	notify.SetRiverClient(nil)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Missing igdb_id forces the worker to mark the item failed (no handler error).
	gameData := map[string]any{
		"title": "Failing Import Game",
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Item must have failed.
	var itemStatus string
	if err := testDB.QueryRowContext(ctx, "SELECT status FROM job_items WHERE id = ?", itemID).Scan(&itemStatus); err != nil {
		t.Fatalf("query item status: %v", err)
	}
	if itemStatus != string(models.JobItemStatusFailed) {
		t.Fatalf("item status = %q, want failed (prerequisite)", itemStatus)
	}

	failedKey := jobID + ":" + notify.TypeImportFailed
	var failedCount int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE dedup_key = ?", failedKey,
	).Scan(&failedCount); err != nil {
		t.Fatalf("query failed events: %v", err)
	}
	if failedCount != 1 {
		t.Errorf("events count for dedup_key %q = %d, want 1", failedKey, failedCount)
	}

	completedKey := jobID + ":" + notify.TypeImportCompleted
	var completedCount int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE dedup_key = ?", completedKey,
	).Scan(&completedCount); err != nil {
		t.Fatalf("query completed events: %v", err)
	}
	if completedCount != 0 {
		t.Errorf("events count for dedup_key %q = %d, want 0", completedKey, completedCount)
	}
}
