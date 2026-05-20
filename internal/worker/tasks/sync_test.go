package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/ratelimit"
	epicsvc "github.com/drzero42/nexorious/internal/services/epic"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/igdb"
	psnsvc "github.com/drzero42/nexorious/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/worker/tasks"
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

// fakePSNAdapter implements PSNLibraryAdapter for testing.
type fakePSNAdapter struct {
	pages [][]psnsvc.ExternalLibraryEntry // each inner slice is one batch/page
	err   error                           // if non-nil, returned by GetLibrary
}

func (f *fakePSNAdapter) GetLibrary(_ context.Context, _ string, _ int, onBatch func([]psnsvc.ExternalLibraryEntry) error) error {
	if f.err != nil {
		return f.err
	}
	for _, page := range f.pages {
		if err := onBatch(page); err != nil {
			return err
		}
	}
	return nil
}

// fakeEpicAdapter implements EpicLibraryAdapter for testing.
type fakeEpicAdapter struct {
	batches [][]epicsvc.ExternalLibraryEntry
	err     error
}

func (f *fakeEpicAdapter) GetLibrary(_ context.Context, _ string, onBatch func([]epicsvc.ExternalLibraryEntry) error) error {
	if f.err != nil {
		return f.err
	}
	for _, batch := range f.batches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	return nil
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
	if err := w.Work(context.Background(), job); err == nil {
		t.Fatal("expected error when job_item not found so River retries, got nil")
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
	// IGDB returns a wrong-game candidate (above the 0.6 inclusion threshold but
	// below the 0.85 auto-resolve threshold). Verify the item stays pending_review
	// and match_confidence is persisted so the UI can surface it to the user.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	// Search for "Max Payne 3" but IGDB returns "Max Payne 2: The Fall of Max Payne".
	// normalized query "max payne 3" vs normalized candidate "max payne 2 the fall of max payne":
	//   PartialRatio best window = "max payne 2" → ~91% → partial*0.88 ≈ 0.80 (< 0.85)
	// → candidate enters the list (> 0.6) but does not auto-resolve (< 0.85).
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1235, "name": "Max Payne 2: The Fall of Max Payne", "slug": "max-payne-2"},
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
		 VALUES (?, ?, 'steam', '204100', 'Max Payne 3', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '204100', 'Max Payne 3', ?, 'pending', '{}', '[]')`,
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

func TestProcessSyncItem_IGDBPrefixTitle_AutoResolves(t *testing.T) {
	// When the Steam title is a verbatim prefix of the IGDB title (e.g.
	// "Tesla Effect" vs "Tesla Effect: A Tex Murphy Adventure"), the partial ratio
	// is 100% and partial*0.88 = 0.88 >= 0.85, so the item should auto-resolve.
	// This test documents that the fix for subtitle-mismatch cases is working.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

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

	// Use pc-windows (from migration seed) so platform resolution succeeds and
	// the item can reach completed status rather than stopping at failed.
	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "pc-windows"})
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

	// IGDB returned score 0.88 (partial*0.88=1.0*0.88) ≥ 0.85 → auto-resolved.
	var resolvedID *int
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != 261510 {
		t.Errorf("expected resolved_igdb_id=261510 on external_game after prefix-match auto-resolve, got %v", resolvedID)
	}
}

func TestProcessSyncItem_ManualResolution_DoesNotRevertToPendingReview(t *testing.T) {
	// When HandleResolveItem sets job_items.resolved_igdb_id and re-enqueues the
	// item, the worker must apply that choice to external_games rather than
	// re-running IGDB search (which would put the item back to pending_review).
	// The IGDB server deliberately returns an ambiguous tie (two equal-score
	// candidates) to prove the worker is NOT calling SearchGames on this path.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Two equally-scored candidates → tie → would send item back to pending_review
		// if the worker were to call SearchGames instead of honouring item.ResolvedIGDBID.
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 28960, "name": "Eets: Hunger. It's emotional.", "slug": "eets-hunger-its-emotional"},
			{"id": 15312, "name": "Eets Munchies", "slug": "eets-munchies"},
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
		 VALUES (?, ?, 'steam', '28960', 'Eets', false, true, false, 0)`,
		egID, userID,
	)

	// Simulate HandleResolveItem: item has resolved_igdb_id=28960 set by the user,
	// status reset to pending for re-processing.
	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "pc-windows"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, resolved_igdb_id)
		 VALUES (?, ?, ?, '28960', 'Eets', ?, 'pending', '{}', '[]', 28960)`,
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

	// The user's choice (28960) must be on external_games — not pending_review.
	var resolvedID *int
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != 28960 {
		t.Errorf("expected resolved_igdb_id=28960 on external_game after manual resolve, got %v", resolvedID)
	}

	var itemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus)
	if itemStatus == "pending_review" {
		t.Error("item reverted to pending_review despite manual resolution")
	}
}

func TestProcessSyncItem_CrossSKU_InheritsResolutionFromSibling(t *testing.T) {
	// When a PSN sync returns a new SKU (e.g. PPSA/PS5) for a game that was
	// already resolved under a different SKU (e.g. CUSA/PS4), the worker must
	// inherit the resolved_igdb_id from the sibling external_games row instead
	// of running IGDB search and landing in pending_review.
	// The IGDB mock returns an ambiguous tie to prove IGDB search is not called.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unrelated candidates → all score well below 0.85 → pending_review if called.
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 9999, "name": "Counter-Strike 2", "slug": "counter-strike-2"},
			{"id": 9998, "name": "Minecraft", "slug": "minecraft"},
		})
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// games row must exist before external_games.resolved_igdb_id FK is satisfied.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (141544, 'Evil Dead: The Game', now(), now()) ON CONFLICT (id) DO NOTHING`,
	)

	// PS4 SKU — already resolved from a previous sync.
	egPS4ID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, resolved_igdb_id, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'CUSA27708_00', 'Evil Dead: The Game', 141544, false, true, false, 0)`,
		egPS4ID, userID,
	)

	// PS5 SKU — new entry, not yet resolved.
	egPS5ID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'PPSA03521_00', 'Evil Dead: The Game', false, true, false, 0)`,
		egPS5ID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egPS5ID, "raw_platform": "playstation-5"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'PPSA03521_00', 'Evil Dead: The Game', ?, 'pending', '{}', '[]')`,
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

	// The PS4 sibling's resolution must have been applied to the PS5 external_game.
	var resolvedID *int
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egPS5ID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != 141544 {
		t.Errorf("expected resolved_igdb_id=141544 on PS5 external_game after cross-SKU inherit, got %v", resolvedID)
	}

	var ps5ItemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &ps5ItemStatus)
	if ps5ItemStatus == "pending_review" {
		t.Error("PS5 SKU item landed in pending_review despite sibling PS4 SKU already being resolved")
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

// TestProcessSyncItem_AutoRetry_NilRiverClient_MarksItemFailed is the
// regression guard for the wiring bug at cmd/nexorious/serve.go where
// ProcessSyncItemWorker was constructed without RiverClient. Before the fix the
// auto-retry path in syncCheckJobCompletion silently skipped enqueue when
// rc == nil, leaving the reset item in 'pending' with no backing river_job —
// permanently stuck until the cron-based orphan rescue (≥30 min, ≥1 h age).
//
// This test pins the new behaviour: when RiverClient is nil the auto-retry
// loop must mark the reset item 'failed' so it does not get stranded.
func TestProcessSyncItem_AutoRetry_NilRiverClient_MarksItemFailed(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		 VALUES (?, ?, 'steam', '999', 'Stranded Game', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Stranded Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	// RiverClient is deliberately nil — the production bug being guarded against.
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	riverJob := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, riverJob); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var itemStatus string
	var itemErr *string
	_ = testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).
		Scan(ctx, &itemStatus, &itemErr)
	if itemStatus == "pending" {
		t.Fatalf("regression: item stranded in pending without backing river_job")
	}
	if itemStatus != "failed" {
		t.Errorf("expected item status=failed after nil-rc auto-retry, got %q", itemStatus)
	}
	if itemErr == nil || *itemErr == "" {
		t.Errorf("expected diagnostic error_message, got %v", itemErr)
	}

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus == "processing" || jobStatus == "pending" {
		t.Errorf("expected parent job to settle when all reset items fail to enqueue, got %q", jobStatus)
	}
}

func TestDispatchSync_PSNInvalidCredentials(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, "not-valid-json",
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: &fakePSNAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (invalid psn credentials), got %q", status)
	}
}

func TestDispatchSync_PSNTokenNotVerified(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"abc123","is_verified":false}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: &fakePSNAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (token not verified), got %q", status)
	}
}

func TestDispatchSync_PSNAuthError_MarksTokenExpired(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// ErrInvalidNPSSOToken signals that the npsso token is bad → token must be marked expired.
	adapter := &fakePSNAdapter{err: psnsvc.ErrInvalidNPSSOToken}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (auth error), got %q", status)
	}

	// Token must be marked as expired in user_sync_configs.
	var rawCreds string
	_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
	var parsedCreds struct {
		IsVerified bool `json:"is_verified"`
	}
	_ = json.Unmarshal([]byte(rawCreds), &parsedCreds)
	if parsedCreds.IsVerified {
		t.Error("expected is_verified=false after auth error, token still marked verified")
	}
}

func TestDispatchSync_PSNServiceError_PreservesToken(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// A generic (non-auth) error — e.g. 503 from Sony's API — must NOT mark the token expired.
	adapter := &fakePSNAdapter{err: errors.New("request failed with status 503: service unavailable")}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (service error), got %q", status)
	}

	// Token must NOT be marked expired — the token is valid, the service was unavailable.
	var rawCreds string
	_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
	var parsedCreds struct {
		IsVerified bool `json:"is_verified"`
	}
	_ = json.Unmarshal([]byte(rawCreds), &parsedCreds)
	if !parsedCreds.IsVerified {
		t.Error("expected is_verified=true after service error (token not expired), but token was marked expired")
	}
}

func TestDispatchSync_PSNSuccess_ItemsDispatchedPerBatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// Two pages of games — verifies that both pages are processed.
	page1 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
	}
	page2 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00003_00", Title: "Horizon", RawPlatform: "playstation-5", OwnershipStatus: "owned"},
	}
	adapter := &fakePSNAdapter{pages: [][]psnsvc.ExternalLibraryEntry{page1, page2}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 games upserted as external_games.
	var egCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'psn'`, userID).Scan(ctx, &egCount)
	if egCount != 3 {
		t.Errorf("expected 3 external_games, got %d", egCount)
	}

	// 3 job_items created (none pre-skipped).
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 3 {
		t.Errorf("expected 3 job_items, got %d", itemCount)
	}

	// last_synced_at updated.
	var lastSynced *time.Time
	_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &lastSynced)
	if lastSynced == nil {
		t.Error("expected last_synced_at to be set after successful psn sync")
	}
}

func TestDispatchSync_PSNSuccess_SkippedGameExcluded(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// Pre-insert God of War as skipped. The ON CONFLICT upsert does not touch
	// is_skipped, so it remains true even when the batch includes this game.
	// raw_platform must match the value in the sync batch so the upsert hits
	// the correct row under the (user_id, storefront, external_id, raw_platform)
	// unique constraint.
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, raw_platform, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'NPWR00001_00', 'God of War', 'playstation-4', true, true, false, 0)`,
		uuid.NewString(), userID,
	).Exec(ctx)

	page1 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
	}
	adapter := &fakePSNAdapter{pages: [][]psnsvc.ExternalLibraryEntry{page1}}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 job_item (Spider-Man); God of War is skipped.
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item (skipped game excluded), got %d", itemCount)
	}

	// Confirm no job_item for God of War.
	var gow int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND item_key = 'NPWR00001_00'`, jobID).Scan(ctx, &gow)
	if gow != 0 {
		t.Error("expected no job_item for skipped God of War")
	}
}

func TestDispatchSync_PSNGraphQLSchemaChanged_PreservesToken(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakePSNAdapter{err: psnsvc.ErrPSNGraphQLSchemaChanged}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed, got %q", status)
	}

	// Token must NOT be marked expired — schema change is not an auth error.
	var rawCreds string
	_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
	var parsedCreds struct {
		IsVerified bool `json:"is_verified"`
	}
	_ = json.Unmarshal([]byte(rawCreds), &parsedCreds)
	if !parsedCreds.IsVerified {
		t.Error("expected is_verified=true after schema-changed error (token not expired)")
	}
}

func TestProcessSyncItem_PlaytimeHoursWrittenToUserGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'low', 1)`,
		jobID, userID,
	)

	const igdbID = int32(9999)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)

	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'PPSA09999_00', 'Test Game', false, true, false, 47, ?)`,
		egID, userID, igdbIDVal,
	)

	// source_metadata includes playtime_hours=47
	metaJSON, _ := json.Marshal(map[string]any{
		"external_game_id": egID,
		"raw_platform":     "playstation-4",
		"playtime_hours":   47,
	})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'PPSA09999_00', 'Test Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var itemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus)
	if itemStatus != "completed" {
		t.Errorf("expected item completed, got %q", itemStatus)
	}

	// user_games.hours_played should be set from meta.PlaytimeHours
	var hoursPlayed *float64
	_ = testDB.NewRaw(
		`SELECT hours_played FROM user_games WHERE user_id = ? AND game_id = ?`,
		userID, igdbID,
	).Scan(ctx, &hoursPlayed)
	if hoursPlayed == nil {
		t.Fatal("expected user_games.hours_played to be set, got nil")
	}
	if *hoursPlayed != 47.0 {
		t.Errorf("expected user_games.hours_played=47.0, got %f", *hoursPlayed)
	}
}

func TestDispatchSync_Epic_HappyPath(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'epic', 'pending', 'high')`,
		jobID, userID,
	)
	// Epic uses epic_legendary_state not storefront_credentials — insert row with NULL creds.
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'epic', 'manual')`,
		configID, userID,
	).Exec(ctx)

	epicGames := []epicsvc.ExternalLibraryEntry{
		{ExternalID: "Fortnite", Title: "Fortnite", OwnershipStatus: "owned"},
		{ExternalID: "RocketLeague", Title: "Rocket League", OwnershipStatus: "owned"},
	}

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Steam:       &fakeSteamAdapter{},
		PSN:         &fakePSNAdapter{},
		Epic:        &fakeEpicAdapter{batches: [][]epicsvc.ExternalLibraryEntry{epicGames}},
		RiverClient: nil,
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "epic"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify external_games rows were created.
	var count int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Scan(ctx, &count)
	if count != 2 {
		t.Errorf("expected 2 external_games rows, got %d", count)
	}

	// Verify raw_platform is pc-windows.
	var rawPlatform string
	_ = testDB.NewRaw(
		`SELECT raw_platform FROM external_games WHERE user_id = ? AND storefront = 'epic' AND external_id = 'Fortnite'`,
		userID,
	).Scan(ctx, &rawPlatform)
	if rawPlatform != "pc-windows" {
		t.Errorf("expected raw_platform=pc-windows, got %q", rawPlatform)
	}

	// Verify job is not failed.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status == "failed" {
		t.Errorf("expected job not to be failed, got status=%q", status)
	}
}

func TestDispatchSync_Epic_NilAdapter(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'epic', 'pending', 'high')`,
		jobID, userID,
	)
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'epic', 'manual')`,
		configID, userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:    testDB,
		Steam: &fakeSteamAdapter{},
		PSN:   &fakePSNAdapter{},
		Epic:  nil, // not configured
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "epic"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed when Epic adapter is nil, got %q", status)
	}
}

func TestProcessSyncItem_CancelledJobNotOverwritten(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// Job is already cancelled — simulates a reset having run.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'steam', 'cancelled', 'low', 1)`,
		jobID, userID,
	)

	// The external_game was deleted by the reset; job_item still references it.
	egID := uuid.NewString()
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
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "cancelled" {
		t.Errorf("expected job status=cancelled after mid-flight worker, got %q", status)
	}
}

// ---------------------------------------------------------------------------
// EpicClientAdapter — DB↔disk snapshot round-trip
// ---------------------------------------------------------------------------

// fakeEpicSubprocessClient satisfies the unexported epicSubprocessClient
// interface that EpicClientAdapter depends on. It records calls and returns
// canned values so the adapter can be tested without invoking legendary.
type fakeEpicSubprocessClient struct {
	configured       bool
	restoreErr       error
	getLibraryErr    error
	captureSnapshot  map[string]string
	captureErr       error
	libraryBatches   [][]epicsvc.ExternalLibraryEntry
	restoredSnapshot map[string]string

	restoreCalled    bool
	getLibraryCalled bool
	captureCalled    bool
}

func (f *fakeEpicSubprocessClient) Configured() bool { return f.configured }

func (f *fakeEpicSubprocessClient) RestoreSnapshot(_ string, snapshot map[string]string) error {
	f.restoreCalled = true
	f.restoredSnapshot = snapshot
	return f.restoreErr
}

func (f *fakeEpicSubprocessClient) GetLibrary(_ context.Context, _ string, onBatch func([]epicsvc.ExternalLibraryEntry) error) error {
	f.getLibraryCalled = true
	if f.getLibraryErr != nil {
		return f.getLibraryErr
	}
	for _, batch := range f.libraryBatches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeEpicSubprocessClient) CaptureSnapshot(_ string) (map[string]string, error) {
	f.captureCalled = true
	return f.captureSnapshot, f.captureErr
}

func seedEpicConfig(t *testing.T, userID string, snapshot map[string]string) {
	t.Helper()
	snapJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, epic_legendary_state, created_at, updated_at)
		 VALUES (?, ?, 'epic', 'manual', ?::jsonb, now(), now())`,
		uuid.NewString(), userID, string(snapJSON),
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}
}

func readEpicSnapshot(t *testing.T, userID string) map[string]string {
	t.Helper()
	var snapJSON []byte
	err := testDB.NewRaw(
		`SELECT epic_legendary_state FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Scan(context.Background(), &snapJSON)
	if err != nil {
		t.Fatalf("read epic_legendary_state: %v", err)
	}
	if len(snapJSON) == 0 {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal(snapJSON, &out); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return out
}

func TestEpicClientAdapter_NotConfigured_ReturnsErrorWithoutTouchingDB(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	fake := &fakeEpicSubprocessClient{configured: false}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when not configured, got nil")
	}
	if fake.restoreCalled || fake.getLibraryCalled || fake.captureCalled {
		t.Errorf("no Client methods should be invoked when not configured, got restore=%v getLib=%v capture=%v",
			fake.restoreCalled, fake.getLibraryCalled, fake.captureCalled)
	}
}

func TestEpicClientAdapter_NoSnapshotInDB_ReturnsError(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	fake := &fakeEpicSubprocessClient{configured: true}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when no snapshot in DB, got nil")
	}
	if fake.restoreCalled || fake.getLibraryCalled {
		t.Errorf("should not restore/fetch when DB has no snapshot, got restore=%v getLib=%v",
			fake.restoreCalled, fake.getLibraryCalled)
	}
}

func TestEpicClientAdapter_RestoresSnapshotFromDB(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	original := map[string]string{
		"user.json":           `{"displayName":"Tester","account_id":"abc"}`,
		"metadata/Game1.json": `{"title":"Game 1"}`,
	}
	seedEpicConfig(t, userID, original)

	fake := &fakeEpicSubprocessClient{configured: true, captureSnapshot: original}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.restoreCalled {
		t.Fatal("expected RestoreSnapshot to be called")
	}
	if len(fake.restoredSnapshot) != len(original) {
		t.Errorf("restored snapshot size mismatch: got %d, want %d", len(fake.restoredSnapshot), len(original))
	}
	for k, v := range original {
		if fake.restoredSnapshot[k] != v {
			t.Errorf("restored snapshot %q: got %q, want %q", k, fake.restoredSnapshot[k], v)
		}
	}
}

func TestEpicClientAdapter_PersistsNewSnapshotAfterSuccess(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	original := map[string]string{"user.json": `{"v":1}`}
	updated := map[string]string{"user.json": `{"v":2}`, "metadata/NewGame.json": `{"title":"New"}`}
	seedEpicConfig(t, userID, original)

	fake := &fakeEpicSubprocessClient{configured: true, captureSnapshot: updated}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	if err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readEpicSnapshot(t, userID)
	if len(got) != len(updated) {
		t.Errorf("persisted snapshot size mismatch: got %d, want %d", len(got), len(updated))
	}
	for k, v := range updated {
		if got[k] != v {
			t.Errorf("persisted %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestEpicClientAdapter_PersistsSnapshotEvenOnFetchError(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	original := map[string]string{"user.json": `{"v":1}`}
	updatedAfterFailedFetch := map[string]string{"user.json": `{"v":2,"refreshed_token":"x"}`}
	seedEpicConfig(t, userID, original)

	fetchErr := errors.New("legendary list failed: connection reset")
	fake := &fakeEpicSubprocessClient{
		configured:      true,
		getLibraryErr:   fetchErr,
		captureSnapshot: updatedAfterFailedFetch,
	}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil })
	if err == nil || err.Error() != fetchErr.Error() {
		t.Fatalf("expected fetch error to propagate, got %v", err)
	}
	if !fake.captureCalled {
		t.Error("expected CaptureSnapshot to run even after fetch failure (refreshed tokens must survive)")
	}

	got := readEpicSnapshot(t, userID)
	if got["user.json"] != `{"v":2,"refreshed_token":"x"}` {
		t.Errorf("snapshot was not persisted after fetch failure: got %v", got)
	}
}

func TestEpicClientAdapter_SkipsPersistWhenSnapshotEmpty(t *testing.T) {
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	original := map[string]string{"user.json": `{"v":1}`}
	seedEpicConfig(t, userID, original)

	fake := &fakeEpicSubprocessClient{configured: true, captureSnapshot: map[string]string{}}
	a := &tasks.EpicClientAdapter{Client: fake, DB: testDB}

	if err := a.GetLibrary(context.Background(), userID, func([]epicsvc.ExternalLibraryEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readEpicSnapshot(t, userID)
	if got["user.json"] != `{"v":1}` {
		t.Errorf("snapshot should be unchanged when CaptureSnapshot returns empty, got %v", got)
	}
}

// ─── GOG dispatch tests ───────────────────────────────────────────────────────

type fakeGOGAdapter struct {
	entries     []gogsvc.ExternalLibraryEntry
	refreshedTo *gogsvc.TokenResponse
	refreshErr  error
	libraryErr  error
}

func (f *fakeGOGAdapter) GetLibrary(_ context.Context, _ string, _ int, onBatch func([]gogsvc.ExternalLibraryEntry) error) error {
	if f.libraryErr != nil {
		return f.libraryErr
	}
	return onBatch(f.entries)
}

func (f *fakeGOGAdapter) RefreshToken(_ context.Context, _ string) (*gogsvc.TokenResponse, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	if f.refreshedTo != nil {
		return f.refreshedTo, nil
	}
	return &gogsvc.TokenResponse{AccessToken: "new-acc", RefreshToken: "new-ref", UserID: "u1", Username: "user"}, nil
}

func TestGOGDispatch_CreatesExternalGames(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"access_token":"acc","refresh_token":"ref","user_id":"u1","username":"user"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'gog', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeGOGAdapter{
		entries: []gogsvc.ExternalLibraryEntry{
			{ExternalID: "1001", Title: "GOG Game", RawPlatform: "pc-windows", OwnershipStatus: "owned"},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, GOG: adapter}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'gog'`, userID).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("want 1 external_game, got %d", count)
	}
}

func TestGOGDispatch_DualPlatformCreatesTwoRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"access_token":"acc","refresh_token":"ref","user_id":"u1","username":"user"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'gog', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeGOGAdapter{
		entries: []gogsvc.ExternalLibraryEntry{
			{ExternalID: "2001", Title: "Dual Game", RawPlatform: "pc-windows", OwnershipStatus: "owned"},
			{ExternalID: "2001", Title: "Dual Game", RawPlatform: "pc-linux", OwnershipStatus: "owned"},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, GOG: adapter}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"},
	}
	_ = w.Work(ctx, job)

	var count int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'gog' AND external_id = '2001'`,
		userID,
	).Scan(ctx, &count)
	if count != 2 {
		t.Errorf("want 2 external_games for dual-platform game, got %d", count)
	}

	// Verify both job_items have distinct item_keys.
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 2 {
		t.Errorf("want 2 job_items (one per platform), got %d", itemCount)
	}
}

func TestGOGDispatch_TokenRefreshPersisted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	creds := `{"access_token":"old-acc","refresh_token":"old-ref","user_id":"u1","username":"user"}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'gog', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakeGOGAdapter{
		refreshedTo: &gogsvc.TokenResponse{AccessToken: "new-acc", RefreshToken: "new-ref", UserID: "u1", Username: "user"},
		entries:     []gogsvc.ExternalLibraryEntry{},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, GOG: adapter}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"},
	}
	_ = w.Work(ctx, job)

	var storedCreds string
	_ = testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'gog'`,
		userID,
	).Scan(ctx, &storedCreds)
	var parsed map[string]string
	_ = json.Unmarshal([]byte(storedCreds), &parsed)
	if parsed["access_token"] != "new-acc" {
		t.Errorf("refreshed access_token not persisted, got %q", parsed["access_token"])
	}
	if parsed["refresh_token"] != "new-ref" {
		t.Errorf("refreshed refresh_token not persisted, got %q", parsed["refresh_token"])
	}
}

