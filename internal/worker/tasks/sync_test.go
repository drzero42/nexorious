package tasks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/ratelimit"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	steamsvc "github.com/drzero42/nexorious-go/internal/services/steam"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// ---------------------------------------------------------------------------
// NewDispatchSyncHandler — DB-backed tests using testcontainers
// ---------------------------------------------------------------------------

// fakeSteamAdapter implements SteamLibraryAdapter for testing.
type fakeSteamAdapter struct {
	games []steamsvc.ExternalLibraryEntry
	err   error
}

func (f *fakeSteamAdapter) GetOwnedGames(_ context.Context, _, _ string) ([]steamsvc.ExternalLibraryEntry, error) {
	return f.games, f.err
}


func TestDispatchSync_InvalidPayload(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewDispatchSyncHandler(db, &fakeSteamAdapter{}, nil)

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: "dispatch_sync",
		Payload:  []byte("not-json"),
	}
	// Should not error — logs and returns nil.
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil error for invalid payload, got %v", err)
	}
}

func TestDispatchSync_NoSyncConfig(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)

	handler := tasks.NewDispatchSyncHandler(db, &fakeSteamAdapter{}, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "steam"})
	task := &models.PendingTask{
		ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload,
	}

	// No sync_config row → fails with "no sync config found".
	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed, got %q", status)
	}
}

func TestDispatchSync_NoCredentials(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	// Insert sync config with NULL credentials.
	configID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		configID, userID,
	).Exec(ctx)

	handler := tasks.NewDispatchSyncHandler(db, &fakeSteamAdapter{}, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "steam"})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (no credentials), got %q", status)
	}
}

func TestDispatchSync_UnknownStorefront(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'bogus', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"key":"val"}`
	configID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'bogus', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	handler := tasks.NewDispatchSyncHandler(db, &fakeSteamAdapter{}, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "bogus"})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (unknown storefront), got %q", status)
	}
}

func TestDispatchSync_SteamInvalidCredentials(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	// Invalid JSON for credentials.
	configID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, "not-valid-json",
	).Exec(ctx)

	handler := tasks.NewDispatchSyncHandler(db, &fakeSteamAdapter{}, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "steam"})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (invalid credentials json), got %q", status)
	}
}

func TestDispatchSync_SteamFetchError(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	configID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{err: errSteamFetch}
	handler := tasks.NewDispatchSyncHandler(db, adapter, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "steam"})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (steam fetch error), got %q", status)
	}
}

var errSteamFetch = errType("steam fetch failed")

type errType string

func (e errType) Error() string { return string(e) }

func TestDispatchSync_SteamSuccess(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	configID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.ExternalLibraryEntry{
			{ExternalID: "730", Title: "Counter-Strike 2", RawPlatform: "PC", PlaytimeHours: 100, OwnershipStatus: "owned"},
		},
	}
	handler := tasks.NewDispatchSyncHandler(db, adapter, nil)
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "user_id": userID, "storefront": "steam"})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "dispatch_sync", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// External game should have been upserted.
	var count int
	_ = db.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("expected 1 external_game, got %d", count)
	}

	// last_synced_at should be updated.
	var lastSynced *time.Time
	_ = db.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &lastSynced)
	if lastSynced == nil {
		t.Error("expected last_synced_at to be set after successful sync")
	}
}

// ---------------------------------------------------------------------------
// NewProcessSyncItemHandler — basic cases
// ---------------------------------------------------------------------------

func newIGDBClientForTests(t *testing.T, tokenURL, apiURL string) *igdb.Client {
	t.Helper()
	cfg := &config.Config{
		IGDBClientID:     "test-id",
		IGDBClientSecret: "test-secret",
	}
	c := igdb.NewClientWithTokenURL(cfg, tokenURL, ratelimit.NewLocal(100, 100))
	c.SetAPIURLForTest(apiURL)
	return c
}

func TestProcessSyncItem_InvalidPayload(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewProcessSyncItemHandler(db, nil)

	task := &models.PendingTask{
		ID: uuid.NewString(), TaskType: "process_sync_item", Payload: []byte("bad"),
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestProcessSyncItem_ItemNotFound(t *testing.T) {
	db := setupTasksTestDB(t)
	handler := tasks.NewProcessSyncItemHandler(db, nil)

	payload, _ := json.Marshal(map[string]string{"job_item_id": uuid.NewString()})
	task := &models.PendingTask{
		ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload,
	}
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}


func TestProcessSyncItem_SkippedExternalGame(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// Insert an external_game marked as skipped.
	egID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '730', 'CS2', true, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	handler := tasks.NewProcessSyncItemHandler(db, nil)
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = db.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "skipped" {
		t.Errorf("expected item status=skipped, got %q", status)
	}
}

func TestProcessSyncItem_NoIGDBID_PendingReview(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// External game with no IGDB ID.
	egID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '440', 'Team Fortress 2', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '440', 'Team Fortress 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	// Pass nil igdbClient so IGDB search is skipped → pending_review.
	handler := tasks.NewProcessSyncItemHandler(db, nil)
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = db.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending_review" {
		t.Errorf("expected item status=pending_review, got %q", status)
	}
}

func TestProcessSyncItem_WithResolvedIGDBID_Completed(t *testing.T) {
	// External game already has a resolved IGDB ID + valid platform/storefront.
	// This exercises the full "create user_game + user_game_platform + completed" path.
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// Pre-insert the games row (required FK from user_games).
	const igdbID = int32(730)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)

	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, 100, ?)`,
		egID, userID, igdbIDVal,
	)

	// Valid platform: pc-windows, valid storefront: steam (both from migration seed).
	metaJSON, _ := json.Marshal(map[string]string{
		"external_game_id": egID,
		"raw_platform":     "pc-windows",
	})
	itemID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	handler := tasks.NewProcessSyncItemHandler(db, nil)
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = db.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("expected item status=completed, got %q", status)
	}

	// user_game should exist.
	var ugCount int
	_ = db.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected 1 user_game, got %d", ugCount)
	}
}

func TestProcessSyncItem_UnresolvedPlatform_Failed(t *testing.T) {
	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(730)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'CS2', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'CS2', false, true, false, 0, ?)`,
		egID, userID, igdbIDVal,
	)
	metaJSON, _ := json.Marshal(map[string]string{
		"external_game_id": egID,
		"raw_platform":     "unknown-platform",
	})
	itemID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	handler := tasks.NewProcessSyncItemHandler(db, nil)
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = db.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected item status=failed (unresolved platform), got %q", status)
	}
}

func TestProcessSyncItem_WithIGDBAutoResolve(t *testing.T) {
	// Set up a mock IGDB server that returns a high-confidence match.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 730, "name": "Counter-Strike 2", "slug": "counter-strike-2"},
		})
	}))
	defer igdbSrv.Close()

	db := setupTasksTestDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, db, userID)

	_, _ = db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, 100)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	handler := tasks.NewProcessSyncItemHandler(db, igdbClient)
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{ID: uuid.NewString(), TaskType: "process_sync_item", Payload: payload}

	if err := handler(ctx, task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Item was resolved or pending_review — just check it's not still pending.
	var status string
	_ = db.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status == "pending" {
		t.Errorf("expected item status to advance from pending, still pending")
	}
}
