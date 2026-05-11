package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// ─── Test DB setup ────────────────────────────────────────────────────────────

func setupTasksTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func insertTestUser(t *testing.T, db *bun.DB, userID string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, true, false)",
		userID, "user_"+userID, string(hash),
	)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
}

func insertTestJob(t *testing.T, db *bun.DB, jobID, userID string, totalItems int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'import', 'nexorious', 'processing', 'normal', ?)`,
		jobID, userID, totalItems,
	)
	if err != nil {
		t.Fatalf("insertTestJob: %v", err)
	}
}

func insertTestJobItem(t *testing.T, db *bun.DB, jobID, userID string, gameData map[string]any) string {
	t.Helper()
	itemID := uuid.NewString()
	sourceMetadata := mustMarshal(t, map[string]any{"data": gameData})
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, uuid.NewString(), "Test Game", sourceMetadata,
	)
	if err != nil {
		t.Fatalf("insertTestJobItem: %v", err)
	}
	return itemID
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}

func makePendingTask(t *testing.T, itemID string) *models.PendingTask {
	t.Helper()
	return &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "import_item",
		Payload:  mustMarshal(t, map[string]string{"job_item_id": itemID}),
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestImportItem_BasicGame(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id":     int32(12345),
		"title":       "Test Game",
		"play_status": "completed",
		"personal_rating": 9.5,
		"hours_played": 42.5,
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	task := makePendingTask(t, itemID)
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Verify Game was created.
	var game models.Game
	err = db.NewSelect().Model(&game).Where("id = ?", int32(12345)).Scan(ctx)
	if err != nil {
		t.Fatalf("game not found: %v", err)
	}
	if game.Title != "Test Game" {
		t.Errorf("game title = %q, want %q", game.Title, "Test Game")
	}

	// Verify UserGame was created with correct fields.
	var ug models.UserGame
	err = db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(12345)).Scan(ctx)
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
	if ug.HoursPlayed == nil || *ug.HoursPlayed != 42.5 {
		t.Errorf("hours_played = %v, want 42.5", ug.HoursPlayed)
	}

	// Verify JobItem is completed.
	var item models.JobItem
	err = db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
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
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	// No igdb_id in game data (zero value / missing).
	gameData := map[string]any{
		"title": "No ID Game",
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	task := makePendingTask(t, itemID)
	err := handler(ctx, task)
	if err != nil {
		t.Fatalf("handler should not return error: %v", err)
	}

	var item models.JobItem
	if err := db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
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
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 2)

	gameData := map[string]any{
		"igdb_id": int32(99999),
		"title":   "Dupe Game",
	}

	// First import.
	itemID1 := insertTestJobItem(t, db, jobID, userID, gameData)
	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID1)); err != nil {
		t.Fatalf("first import error: %v", err)
	}

	// Second import of same game for same user.
	itemID2 := insertTestJobItem(t, db, jobID, userID, gameData)
	if err := handler(ctx, makePendingTask(t, itemID2)); err != nil {
		t.Fatalf("second import error: %v", err)
	}

	var item2 models.JobItem
	if err := db.NewSelect().Model(&item2).Where("id = ?", itemID2).Scan(ctx); err != nil {
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
	if err := db.QueryRowContext(ctx,
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
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	// Seed platform and storefront.
	_, err := db.ExecContext(ctx,
		"INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING",
	)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	_, err = db.ExecContext(ctx,
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
				"platform_name":      "pc",
				"storefront_name":    "steam",
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
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Verify job item completed.
	var item models.JobItem
	if err := db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("item not found: %v", err)
	}
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item.Status)
	}

	// Verify UserGame exists.
	var ug models.UserGame
	if err := db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(77777)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	// Verify UserGamePlatform.
	var ugp models.UserGamePlatform
	if err := db.NewSelect().Model(&ugp).Where("user_game_id = ?", ug.ID).Scan(ctx); err != nil {
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
	if err := db.NewSelect().Model(&tag).Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, "Favorite").Scan(ctx); err != nil {
		t.Fatalf("tag not found: %v", err)
	}
	if tag.Color == nil || *tag.Color != "#ff0000" {
		t.Errorf("tag color = %v, want '#ff0000'", tag.Color)
	}

	var ugt models.UserGameTag
	if err := db.NewSelect().Model(&ugt).Where("user_game_id = ? AND tag_id = ?", ug.ID, tag.ID).Scan(ctx); err != nil {
		t.Fatalf("user_game_tag not found: %v", err)
	}
}

func TestImportItem_PlatformsSkippedWithoutSeed(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	// No platform/storefront seed data — tables are empty.
	gameData := map[string]any{
		"igdb_id": int32(66666),
		"title":   "Unseeded Platform Game",
		"platforms": []any{
			map[string]any{
				"platform_name":      "pc",
				"storefront_name":    "steam",
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// UserGame is still created even when platforms are skipped.
	var ug models.UserGame
	if err := db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(66666)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	// Platform entry should NOT exist — seed data must be loaded first.
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?", ug.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if count != 0 {
		t.Errorf("platform count = %d, want 0 (seed data not loaded)", count)
	}
}

func TestImportItem_ReimportMergesPlatforms(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 2)

	gameData := map[string]any{
		"igdb_id": int32(88888),
		"title":   "Merge Game",
		"platforms": []any{
			map[string]any{
				"platform_name":      "pc",
				"storefront_name":    "steam",
				"ownership_status": "owned",
				"is_available":     true,
			},
		},
	}

	// First import — no seed, platforms stored via original name.
	itemID1 := insertTestJobItem(t, db, jobID, userID, gameData)
	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID1)); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Seed platform and storefront.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING",
	); err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING",
	); err != nil {
		t.Fatalf("insert storefront: %v", err)
	}

	// Second import of same game — should not duplicate platforms.
	itemID2 := insertTestJobItem(t, db, jobID, userID, gameData)
	if err := handler(ctx, makePendingTask(t, itemID2)); err != nil {
		t.Fatalf("second import: %v", err)
	}

	var item2 models.JobItem
	if err := db.NewSelect().Model(&item2).Where("id = ?", itemID2).Scan(ctx); err != nil {
		t.Fatalf("item2 not found: %v", err)
	}
	if item2.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed", item2.Status)
	}

	// Still only one UserGame.
	var ugCount int
	if err := db.QueryRowContext(ctx,
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
	if err := db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(88888)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?", ug.ID,
	).Scan(&ugpCount); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if ugpCount != 1 {
		t.Errorf("user_game_platform count = %d, want 1 (no duplicates)", ugpCount)
	}
}

func TestImportItem_JobCompletion(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id": int32(55555),
		"title":   "Completion Game",
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Job should be completed.
	var job models.Job
	if err := db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx); err != nil {
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
	db := setupTasksTestDB(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)
	insertTestJob(t, db, jobID, userID, 1)

	createdAt := time.Date(2023, 1, 15, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2023, 6, 20, 12, 0, 0, 0, time.UTC)

	gameData := map[string]any{
		"igdb_id":    int32(44444),
		"title":      "Timestamped Game",
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	}
	itemID := insertTestJobItem(t, db, jobID, userID, gameData)

	handler := tasks.NewImportItemHandler(db, igdb.NewClient(&config.Config{}), "")
	if err := handler(ctx, makePendingTask(t, itemID)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	var ug models.UserGame
	if err := db.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(44444)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}

	if !ug.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", ug.CreatedAt, createdAt)
	}
	if !ug.UpdatedAt.Equal(updatedAt) {
		t.Errorf("updated_at = %v, want %v", ug.UpdatedAt, updatedAt)
	}
}
