package tasks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/ratelimit"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	steamsvc "github.com/drzero42/nexorious-go/internal/services/steam"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// ---------------------------------------------------------------------------
// DispatchSyncWorker — DB-backed tests using testcontainers
// ---------------------------------------------------------------------------

// fakeSteamAdapter implements SteamLibraryAdapter for testing.
type fakeSteamAdapter struct {
	games []steamsvc.ExternalLibraryEntry
	err   error
}

func (f *fakeSteamAdapter) GetOwnedGames(_ context.Context, _, _ string) ([]steamsvc.ExternalLibraryEntry, error) {
	return f.games, f.err
}

// newTestRiverClient creates a non-started River client backed by the shared
// test container. It is suitable for tests that call Insert but do not need a
// running worker loop.
func newTestRiverClient(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	return rc
}

func TestDispatchSync_InvalidPayload(t *testing.T) {
	truncateAllTables(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}

	// With River, args are already typed — test that a job with empty args
	// (no matching sync config) returns nil without panicking.
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDispatchSync_NoSyncConfig(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)

	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	// No sync_config row → fails with "no sync config found".
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed, got %q", status)
	}
}

func TestDispatchSync_NoCredentials(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	// Insert sync config with NULL credentials.
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		configID, userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (no credentials), got %q", status)
	}
}

func TestDispatchSync_UnknownStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'bogus', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"key":"val"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'bogus', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "bogus"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (unknown storefront), got %q", status)
	}
}

func TestDispatchSync_SteamInvalidCredentials(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	// Invalid JSON for credentials.
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, "not-valid-json",
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (invalid credentials json), got %q", status)
	}
}

func TestDispatchSync_SteamFetchError(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{err: errSteamFetch}
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (steam fetch error), got %q", status)
	}
}

var errSteamFetch = errType("steam fetch failed")

type errType string

func (e errType) Error() string { return string(e) }

func TestDispatchSync_SteamSuccess(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"web_api_key":"k","steam_id":"s"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.ExternalLibraryEntry{
			{ExternalID: "730", Title: "Counter-Strike 2", RawPlatform: "PC", PlaytimeHours: 100, OwnershipStatus: "owned"},
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Steam: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// External game should have been upserted.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("expected 1 external_game, got %d", count)
	}

	// last_synced_at should be updated.
	var lastSynced *time.Time
	_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &lastSynced)
	if lastSynced == nil {
		t.Error("expected last_synced_at to be set after successful sync")
	}
}

// ---------------------------------------------------------------------------
// ProcessSyncItemWorker — basic cases
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

func TestProcessSyncItem_ItemNotFound(t *testing.T) {
	truncateAllTables(t)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}

	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: uuid.NewString()},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestProcessSyncItem_SkippedExternalGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// Insert an external_game marked as skipped.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '730', 'CS2', true, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "skipped" {
		t.Errorf("expected item status=skipped, got %q", status)
	}
}

func TestProcessSyncItem_NoIGDBID_PendingReview(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// External game with no IGDB ID.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '440', 'Team Fortress 2', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '440', 'Team Fortress 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	// Pass nil igdbClient so IGDB search is skipped → pending_review.
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending_review" {
		t.Errorf("expected item status=pending_review, got %q", status)
	}
}

func TestProcessSyncItem_WithResolvedIGDBID_Completed(t *testing.T) {
	// External game already has a resolved IGDB ID + valid platform/storefront.
	// This exercises the full "create user_game + user_game_platform + completed" path.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// Pre-insert the games row (required FK from user_games).
	const igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)

	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
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
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("expected item status=completed, got %q", status)
	}

	// user_game should exist.
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected 1 user_game, got %d", ugCount)
	}
}

func TestProcessSyncItem_UnresolvedPlatform_Failed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'CS2', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'CS2', false, true, false, 0, ?)`,
		egID, userID, igdbIDVal,
	)
	metaJSON, _ := json.Marshal(map[string]string{
		"external_game_id": egID,
		"raw_platform":     "unknown-platform",
	})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
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

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, 100)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Item was resolved or pending_review — just check it's not still pending.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status == "pending" {
		t.Errorf("expected item status to advance from pending, still pending")
	}
}

func TestProcessSyncItem_LowConfidenceIGDB_StoresMatchConfidence(t *testing.T) {
	// IGDB returns a close-but-not-exact match (score > 0.6, < 0.85) so the item
	// goes to pending_review. We verify that match_confidence is stored so the UI
	// can surface it to the user.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	// Return "Tesla Effect: A Tex Murphy Adventure" when searching for "Tesla Effect".
	// After normalization: "tesla effect" vs "tesla effect a tex murphy adventure".
	// PartialRatio = 100% → score = 0.80, which is > 0.6 (in candidates) but < 0.85
	// (below auto-resolve threshold), so the item stays pending_review.
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 261510, "name": "Tesla Effect: A Tex Murphy Adventure", "slug": "tesla-effect-a-tex-murphy-adventure"},
		})
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '261510', 'Tesla Effect', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '261510', 'Tesla Effect', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	var matchConfidence *float64
	_ = testDB.NewRaw(`SELECT status, match_confidence FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status, &matchConfidence)
	if status != "pending_review" {
		t.Errorf("expected status=pending_review, got %q", status)
	}
	if matchConfidence == nil {
		t.Error("expected match_confidence to be stored, got nil")
	} else if *matchConfidence <= 0 || *matchConfidence >= 1 {
		t.Errorf("expected match_confidence in (0, 1), got %f", *matchConfidence)
	}
}

func TestProcessSyncItem_IGDBError_MarksItemIGDBFailed(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// auto_retry_done=true so the item stays igdb_failed (no reset) and job becomes completed_with_errors.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, true)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '123', 'Some Game', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '123', 'Some Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "igdb_failed" {
		t.Errorf("expected item status=igdb_failed, got %q", status)
	}
}

func TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '999', 'Retry Game', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Retry Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	rc := newTestRiverClient(t)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	riverJob := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	// First run: item → igdb_failed, auto_retry triggers (resets to pending, sets auto_retry_done=true).
	if err := w.Work(ctx, riverJob); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}

	var itemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus)

	var autoRetryDone bool
	_ = testDB.NewRaw(`SELECT auto_retry_done FROM jobs WHERE id = ?`, jobID).Scan(ctx, &autoRetryDone)

	if itemStatus != "pending" {
		t.Errorf("expected item reset to pending after auto-retry, got %q", itemStatus)
	}
	if !autoRetryDone {
		t.Error("expected auto_retry_done=true after first completion check")
	}

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "processing" {
		t.Errorf("expected job still processing after auto-retry, got %q", jobStatus)
	}

	// Second run: item → igdb_failed again, auto_retry_done=true → job completed_with_errors.
	if err := w.Work(ctx, riverJob); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "completed_with_errors" {
		t.Errorf("expected job completed_with_errors after retry exhausted, got %q", jobStatus)
	}
}
