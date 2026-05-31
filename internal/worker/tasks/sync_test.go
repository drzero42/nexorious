package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// ---------------------------------------------------------------------------
// DispatchSyncWorker — DB-backed tests using testcontainers
// ---------------------------------------------------------------------------

// fakeStorefrontAdapter implements tasks.StorefrontAdapter for testing.
type fakeStorefrontAdapter struct {
	batches [][]tasks.ExternalGameEntry
	err     error
}

func (f *fakeStorefrontAdapter) GetLibrary(_ context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
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

// adapterFactory returns a factory that always yields the given adapter.
func adapterFactory(adapter tasks.StorefrontAdapter) func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
	return func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return adapter, nil
	}
}

// credErrFactory returns a factory that always reports a credentials error.
func credErrFactory() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
	return func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return nil, tasks.ErrCredentials
	}
}

// fetchErrFactory returns a factory whose adapter fails GetLibrary with err.
func fetchErrFactory(err error) func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
	return func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return &fakeStorefrontAdapter{err: err}, nil
	}
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), RiverClient: nil}

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

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), RiverClient: nil}
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

	// Adapter factory reports ErrCredentials when creds are missing.
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
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

	// Factory returns a non-credentials error for unknown storefront.
	unknownFactory := func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return nil, errors.New("unknown storefront: bogus")
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: unknownFactory, RiverClient: nil}
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
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		configID, userID,
	).Exec(ctx)

	// Invalid credentials → factory returns ErrCredentials.
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
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
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		configID, userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: fetchErrFactory(errSteamFetch), RiverClient: nil}
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
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		configID, userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
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

func TestDispatchSync_SetsSiblingParentID(t *testing.T) {
	// When two library entries have the same (storefront, title) but different
	// external_ids, the second row must have parent_id set to the first row's id.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{
			{ExternalID: "CUSA12345_00", Title: "Horizon", Platforms: []string{"playstation-4"}},
			{ExternalID: "PPSA67890_00", Title: "Horizon", Platforms: []string{"playstation-5"}},
		},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type egRow struct {
		ExternalID string  `bun:"external_id"`
		ParentID   *string `bun:"parent_id"`
	}
	var rows []egRow
	if err := testDB.NewRaw(
		`SELECT external_id, parent_id FROM external_games WHERE user_id = ? ORDER BY created_at ASC`,
		userID,
	).Scan(ctx, &rows); err != nil {
		t.Fatalf("scan external_games: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 external_game rows, got %d", len(rows))
	}

	// First row (PS4) must have no parent.
	if rows[0].ParentID != nil {
		t.Errorf("first row should have no parent_id, got %v", *rows[0].ParentID)
	}
	// Second row (PS5) must point to first row.
	if rows[1].ParentID == nil {
		t.Error("second row should have parent_id set")
	}
}

func TestDispatchSync_Steam_MultiPlatform_WindowsAndLinux(t *testing.T) {
	// Adapter yields Windows+Linux for appid 730 →
	// expect 1 external_games row, 2 external_game_platforms rows, 1 job_item keyed "730".
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows", "pc-linux"}, OwnershipStatus: "owned"}},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '730'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 1 {
		t.Errorf("expected 1 external_games row, got %d", egCount)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '730'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("expected 2 external_game_platforms rows (Windows+Linux), got %d", egpCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item, got %d", itemCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "730" {
		t.Errorf("expected item_key=730, got %q", itemKey)
	}
}

func TestDispatchSync_Steam_PlatformUpdate_AddsNewPlatform(t *testing.T) {
	// Pre-seed game 999 with only pc-windows. Second sync returns {Windows, Linux}.
	// Worker must add the pc-linux platform row.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	// Pre-seed with Windows only.
	insertTestExternalGame(t, userID, "steam", "999", "Cached Game", "pc-windows")

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "999", Title: "Cached Game", PlaytimeHours: 5, Platforms: []string{"pc-windows", "pc-linux"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.external_id = '999'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("expected 2 platform rows (windows+linux) after update, got %d", egpCount)
	}
}

func TestDispatchSync_Steam_AdapterEmptyBatch_NoSideEffects(t *testing.T) {
	// When the adapter yields no entries (e.g. it internally decided to skip
	// every game because platform metadata was unavailable), no platform rows
	// and no job_items should be written.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	fakeAdapter := &fakeStorefrontAdapter{}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("expected Work to succeed, got: %v", err)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 0 {
		t.Errorf("expected 0 platform rows for empty adapter batch, got %d", egpCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 0 {
		t.Errorf("expected 0 job_items for empty adapter batch, got %d", itemCount)
	}
}

func TestDispatchSync_Steam_EmptyPlatformsFallback_EmitsWindowsRow(t *testing.T) {
	// Adapter yields a game with an empty Platforms slice →
	// worker falls back to a single pc-windows row.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "777", Title: "No Platform Game", PlaytimeHours: 0, OwnershipStatus: "owned"}},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '777' AND egp.platform = 'pc-windows'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 1 {
		t.Errorf("expected 1 pc-windows platform row for no-platforms fallback, got %d", egpCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "777" {
		t.Errorf("expected item_key=777, got %q", itemKey)
	}
}

func TestDispatchSync_Steam_SkippedGameExcluded(t *testing.T) {
	// Pre-seed game 730 as skipped. The ON CONFLICT upsert must not clear is_skipped,
	// and the worker must not create a job_item for it.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	// Pre-insert CS2 as skipped.
	egID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', true, true, false)`,
		egID, userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{
			{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 50, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
		},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 job_item (Dota 2); CS2 is skipped.
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item (skipped game excluded), got %d", itemCount)
	}

	var cs2Count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND item_key = '730'`, jobID).Scan(ctx, &cs2Count)
	if cs2Count != 0 {
		t.Error("expected no job_item for skipped CS2")
	}

	// The pre-skipped CS2 must produce a sync_changes('skipped') row.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM sync_changes WHERE job_id = ? AND external_game_id = ?`,
		jobID, egID,
	).Scan(ctx, &sc); err != nil {
		t.Fatalf("scan sync_change for skipped game: %v", err)
	}
	if sc.ChangeType != "skipped" {
		t.Errorf("change_type: want 'skipped', got %q", sc.ChangeType)
	}
	if sc.Title != "Counter-Strike 2" {
		t.Errorf("title: want 'Counter-Strike 2', got %q", sc.Title)
	}
}

func TestDispatchSync_Steam_PlaytimeStoredOnPlatform(t *testing.T) {
	// PlaytimeHours=100 on a multi-platform game → primary platform gets 100,
	// secondary platforms get 0. Verifies playtime is on external_game_platforms.hours_played.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows", "pc-linux"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type platformHours struct {
		Platform    string  `bun:"platform"`
		HoursPlayed float64 `bun:"hours_played"`
	}
	var rows []platformHours
	_ = testDB.NewRaw(`
		SELECT egp.platform, egp.hours_played
		FROM external_game_platforms egp
		JOIN external_games eg ON eg.id = egp.external_game_id
		WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '730'
		ORDER BY egp.platform`,
		userID,
	).Scan(ctx, &rows)

	if len(rows) != 2 {
		t.Fatalf("expected 2 platform rows, got %d", len(rows))
	}

	var primaryHours, secondaryHours float64
	for _, r := range rows {
		if r.Platform == "pc-windows" {
			primaryHours = r.HoursPlayed
		} else {
			secondaryHours = r.HoursPlayed
		}
	}
	if primaryHours != 100 {
		t.Errorf("expected primary (pc-windows) hours_played=100, got %f", primaryHours)
	}
	if secondaryHours != 0 {
		t.Errorf("expected secondary (pc-linux) hours_played=0, got %f", secondaryHours)
	}
}

func TestDispatchSync_Steam_FractionalPlaytimeStoredOnPlatform(t *testing.T) {
	// Regression guard: PlaytimeHours=1.5 (the canonical sub-hour case from Steam)
	// must reach external_game_platforms.hours_played intact. NUMERIC(10,2) preserves
	// it; this test fails if anyone later truncates to int inside upsertPlatforms.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 1.5, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hours float64
	if err := testDB.NewRaw(`
		SELECT egp.hours_played
		FROM external_game_platforms egp
		JOIN external_games eg ON eg.id = egp.external_game_id
		WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '570'`,
		userID,
	).Scan(ctx, &hours); err != nil {
		t.Fatalf("scan hours_played: %v", err)
	}
	if hours != 1.5 {
		t.Errorf("hours_played: want 1.5, got %v", hours)
	}
}

func TestDispatchSync_Steam_JobItemExternalGameIDSet(t *testing.T) {
	// Verifies that job_items.external_game_id is populated directly (not via
	// source_metadata JSON) after a Steam dispatch.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 50, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egID *string
	_ = testDB.NewRaw(
		`SELECT id FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '570'`,
		userID,
	).Scan(ctx, &egID)
	if egID == nil {
		t.Fatal("expected external_game row for appid 570")
	}

	var itemExternalGameID *string
	_ = testDB.NewRaw(
		`SELECT external_game_id FROM job_items WHERE job_id = ? AND item_key = '570'`,
		jobID,
	).Scan(ctx, &itemExternalGameID)
	if itemExternalGameID == nil {
		t.Fatal("expected job_items.external_game_id to be set")
	}
	if *itemExternalGameID != *egID {
		t.Errorf("expected job_items.external_game_id=%q, got %q", *egID, *itemExternalGameID)
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
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		configID, userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
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

func TestDispatchSync_PSNFetchError_FailsJob(t *testing.T) {
	// A library fetch error (auth, transport, schema, etc.) must fail the job.
	// Token-state side effects now live in the psn adapter / factory and are
	// covered by tests in those packages.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     fetchErrFactory(errors.New("request failed with status 503: service unavailable")),
		RiverClient: nil,
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (psn fetch error), got %q", status)
	}
}

func TestDispatchSync_PSNFetchError_CredentialsErrFailsJob(t *testing.T) {
	// When the adapter returns ErrCredentials mid-fetch (e.g. auth lost), the
	// worker must mark the job failed without panicking.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     fetchErrFactory(tasks.ErrCredentials),
		RiverClient: nil,
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (psn credentials error), got %q", status)
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
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		configID, userID,
	).Exec(ctx)

	// Two pages of games — verifies that both pages are processed.
	page1 := []tasks.ExternalGameEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", Platforms: []string{"playstation-4"}, OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", Platforms: []string{"playstation-4"}, OwnershipStatus: "owned"},
	}
	page2 := []tasks.ExternalGameEntry{
		{ExternalID: "NPWR00003_00", Title: "Horizon", Platforms: []string{"playstation-5"}, OwnershipStatus: "owned"},
	}
	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{page1, page2}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
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
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	// Pre-insert God of War as skipped. The ON CONFLICT upsert does not touch
	// is_skipped, so it remains true even when the batch includes this game.
	egID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'psn', 'NPWR00001_00', 'God of War', true, true, false)`,
		egID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-4', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)

	page1 := []tasks.ExternalGameEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", Platforms: []string{"playstation-4"}, OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", Platforms: []string{"playstation-4"}, OwnershipStatus: "owned"},
	}
	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{page1}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
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
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'epic', 'manual')`,
		configID, userID,
	).Exec(ctx)

	// Epic games are always mapped to pc-windows by the Epic adapter.
	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{
			{ExternalID: "Fortnite", Title: "Fortnite", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			{ExternalID: "RocketLeague", Title: "Rocket League", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
		},
	}}
	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     adapterFactory(fakeAdapter),
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

	// Verify pc-windows platform row exists.
	var platformCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'epic' AND eg.external_id = 'Fortnite' AND egp.platform = 'pc-windows'`,
		userID,
	).Scan(ctx, &platformCount)
	if platformCount != 1 {
		t.Errorf("expected 1 pc-windows platform row for Fortnite, got %d", platformCount)
	}

	// Verify job is not failed.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status == "failed" {
		t.Errorf("expected job not to be failed, got status=%q", status)
	}
}

func TestDispatchSync_Epic_NotConfigured_FailsJob(t *testing.T) {
	// When the epic legendary state is missing, the adapter factory returns
	// ErrCredentials and the worker marks the job failed.
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
		DB:      testDB,
		Adapter: credErrFactory(),
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
		t.Errorf("expected job status=failed when Epic adapter is not configured, got %q", status)
	}
}

// insertTestExternalGame inserts a minimal external_games row and one
// external_game_platforms row. Returns the external_game id.
func insertTestExternalGame(t *testing.T, userID, storefront, externalID, title, platform string) string {
	t.Helper()
	egID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, ?, ?, ?, false, true, false)`,
		egID, userID, storefront, externalID, title,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGame: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, ?, 0, now())`,
		uuid.NewString(), egID, platform,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGamePlatform: %v", err)
	}
	return egID
}

// ─── GOG dispatch tests ───────────────────────────────────────────────────────

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
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'gog', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "1001", Title: "GOG Game", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter)}
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
	// Same external_id with two platform entries → 1 external_games row,
	// 2 external_game_platforms rows, 1 job_item.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'gog', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{ExternalID: "2001", Title: "Dual Game", Platforms: []string{"pc-windows", "pc-linux"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter)}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"},
	}
	_ = w.Work(ctx, job)

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'gog' AND external_id = '2001'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 1 {
		t.Errorf("want 1 external_game for dual-platform game, got %d", egCount)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.external_id = '2001'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("want 2 external_game_platforms rows, got %d", egpCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("want 1 job_item (one per game, not per platform), got %d", itemCount)
	}
}

func TestFailSyncJob_CancelsPendingItems(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 2)`,
		jobID, userID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription) VALUES (?, ?, 'steam', '1', 'Game', false, true, false)`,
		egID, userID,
	)
	// Two pending items, one completed item.
	item1 := uuid.NewString()
	item2 := uuid.NewString()
	item3 := uuid.NewString()
	for _, id := range []string{item1, item2} {
		_, _ = testDB.ExecContext(ctx,
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates) VALUES (?, ?, ?, ?, 'Game', ?, '{}', 'pending', '{}', '[]')`,
			id, jobID, userID, id, egID,
		)
	}
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates) VALUES (?, ?, ?, ?, 'Game', ?, '{}', 'completed', '{}', '[]')`,
		item3, jobID, userID, item3, egID,
	)

	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     fetchErrFactory(errors.New("network error")),
		RiverClient: nil,
	}
	_ = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	})

	var cancelledCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'cancelled'`, jobID).Scan(ctx, &cancelledCount)
	if cancelledCount != 2 {
		t.Errorf("expected 2 cancelled items, got %d", cancelledCount)
	}
	var completedStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item3).Scan(ctx, &completedStatus)
	if completedStatus != "completed" {
		t.Errorf("completed item should not be cancelled, got %q", completedStatus)
	}
}

func TestDispatchSync_RemovedGames_WritesSyncChange(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	// Pre-seed a game that is NOT returned by the sync (should be marked removed).
	insertTestExternalGame(t, userID, "steam", "999", "Old Game", "pc-windows")

	// Sync returns an empty library — game 999 is gone.
	fakeAdapter := &fakeStorefrontAdapter{}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: nil}
	_ = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	})

	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 removed sync_change, got %d", changeCount)
	}
	var changeTitle string
	_ = testDB.NewRaw(
		`SELECT title FROM sync_changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeTitle)
	if changeTitle != "Old Game" {
		t.Errorf("sync_change title: want 'Old Game', got %q", changeTitle)
	}
}

// ---------------------------------------------------------------------------
// IGDBMatchWorker — Stage 2
// ---------------------------------------------------------------------------

func TestIGDBMatchWorker_SiblingResolution(t *testing.T) {
	// A sibling external_game row already has resolved_igdb_id set.
	// IGDBMatchWorker must inherit it and enqueue UserGameArgs without calling IGDB.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'normal', 1)`,
		jobID, userID,
	)
	const igdbID = int32(7777)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Sibling Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Sibling: same user/storefront/title, different external_id, already resolved.
	siblingID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'CUSA001', 'Sibling Game', false, true, false, ?)`,
		siblingID, userID, igdbID,
	)
	// Target: same title, unresolved.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'psn', 'PPSA001', 'Sibling Game', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-5', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'PPSA001', 'Sibling Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// external_game must have inherited resolved_igdb_id.
	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// Item must still be pending (UserGameWorker handles completion).
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status after sibling resolution: want 'pending', got %q", status)
	}
}

func TestIGDBMatchWorker_NoIGDBClient_PendingReview(t *testing.T) {
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '111', 'Unknown Game', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '111', 'Unknown Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending_review" {
		t.Errorf("expected pending_review, got %q", status)
	}
}

func TestIGDBMatchWorker_AutoResolve(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	const igdbID = int32(500)
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": igdbID, "name": "Counter-Strike 2", "slug": "counter-strike-2"},
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// games row must exist.
	var gameTitle string
	_ = testDB.NewRaw(`SELECT title FROM games WHERE id = ?`, igdbID).Scan(ctx, &gameTitle)
	if gameTitle == "" {
		t.Error("games row should have been inserted by IGDBMatchWorker")
	}
	// Item stays pending (UserGameWorker handles the transition).
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status: want 'pending', got %q (UserGameWorker transitions it)", status)
	}
}

// NOTE: TestGOGDispatch_TokenRefreshPersisted was removed: GOG token-refresh
// persistence now lives in the adapter factory (cmd/nexorious/serve.go) and is
// covered by tests in the factory's package, not by DispatchSyncWorker tests.

func TestDispatchSyncWorker_EnqueuesPerBatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	rc := newTestRiverClient(t)

	// Two separate batches: batch 1 has game-A, batch 2 has game-B.
	adapter := &fakeStorefrontAdapter{
		batches: [][]tasks.ExternalGameEntry{
			{{ExternalID: "game-A", Title: "Game A", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
			{{ExternalID: "game-B", Title: "Game B", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
		},
	}
	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     adapterFactory(adapter),
		RiverClient: rc,
	}

	job := &river.Job[tasks.DispatchSyncArgs]{Args: tasks.DispatchSyncArgs{
		JobID: jobID, UserID: userID, Storefront: "steam",
	}}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	// Both games' job_items should have been enqueued (river_job rows exist).
	var riverCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'igdb_match'`,
	).Scan(ctx, &riverCount); err != nil {
		t.Fatalf("query river_job: %v", err)
	}
	if riverCount != 2 {
		t.Errorf("expected 2 igdb_match river jobs, got %d", riverCount)
	}
}

// ---------------------------------------------------------------------------
// UserGameWorker — Stage 3 tests
// ---------------------------------------------------------------------------

func TestUserGameWorker_CreatesUserGameAndSyncChange(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// Seed platform and storefront required by user_game_platforms FKs.
	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 42.5, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	job := &river.Job[tasks.UserGameArgs]{
		Args: tasks.UserGameArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user_games row must exist.
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected 1 user_game, got %d", ugCount)
	}
	// user_game_platforms row with hours_played=42.5.
	var ugpHours *float64
	_ = testDB.NewRaw(
		`SELECT ugp.hours_played FROM user_game_platforms ugp
		 JOIN user_games ug ON ug.id = ugp.user_game_id
		 WHERE ug.user_id = ? AND ug.game_id = ? AND ugp.platform = 'pc-windows' AND ugp.storefront = 'steam'`,
		userID, igdbID,
	).Scan(ctx, &ugpHours)
	if ugpHours == nil || *ugpHours != 42.5 {
		t.Errorf("hours_played: want 42.5, got %v", ugpHours)
	}
	// sync_changes: added.
	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 added sync_change, got %d", changeCount)
	}
	// item status: completed.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("item status: want 'completed', got %q", status)
	}
}

func TestUserGameWorker_OwnershipRankGuard(t *testing.T) {
	// existing UGP has ownership=owned (rank 4). New sync says subscription (rank 2).
	// The existing ownership must NOT be downgraded.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// Seed platform and storefront required by user_game_platforms FKs.
	// psn storefront maps to 'playstation-store' via StorefrontToCollectionSlug.
	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('playstation-4', 'PlayStation 4') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('playstation-store', 'PlayStation Store') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(800)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'PSN Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	ownership := "owned"
	existingHours := 10.0
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'playstation-4', 'playstation-store', true, ?, ?, true, now(), now())`,
		uuid.NewString(), ugID, existingHours, ownership,
	)
	egID := uuid.NewString()
	subOwnership := "subscription"
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'CUSA800', 'PSN Game', false, true, true, ?, ?)`,
		egID, userID, subOwnership, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-4', 20.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'CUSA800', 'PSN Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ownership must not be downgraded from 'owned' to 'subscription'.
	var resultOwnership string
	_ = testDB.NewRaw(
		`SELECT ownership_status FROM user_game_platforms WHERE user_game_id = ? AND platform = 'playstation-4' AND storefront = 'playstation-store'`,
		ugID,
	).Scan(ctx, &resultOwnership)
	if resultOwnership != "owned" {
		t.Errorf("ownership should not be downgraded: want 'owned', got %q", resultOwnership)
	}
	// hours_played should be updated to the higher value (20.0 > 10.0).
	var resultHours float64
	_ = testDB.NewRaw(
		`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'playstation-4' AND storefront = 'playstation-store'`,
		ugID,
	).Scan(ctx, &resultHours)
	if resultHours != 20.0 {
		t.Errorf("hours_played should update to higher value: want 20.0, got %v", resultHours)
	}
	// No sync_change: game already existed (not a new addition).
	var changeCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID).Scan(ctx, &changeCount)
	if changeCount != 0 {
		t.Errorf("expected 0 added sync_changes (game pre-existed), got %d", changeCount)
	}
}

func TestUserGameWorker_StatusChangedSyncChange(t *testing.T) {
	// Existing UGP has subscription; new sync says owned (upgrade).
	// Expect a status_changed sync_change.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// Seed platform and storefront required by user_game_platforms FKs.
	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(900)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Status Change Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 0, 'subscription', true, now(), now())`,
		uuid.NewString(), ugID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '900', 'Status Change Game', false, true, false, 'owned', ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 5.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '900', 'Status Change Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc struct {
		ChangeType string  `bun:"change_type"`
		OldStatus  *string `bun:"old_status"`
		NewStatus  *string `bun:"new_status"`
	}
	_ = testDB.NewRaw(
		`SELECT change_type, old_status, new_status FROM sync_changes WHERE job_id = ?`, jobID,
	).Scan(ctx, &sc)
	if sc.ChangeType != "status_changed" {
		t.Errorf("change_type: want 'status_changed', got %q", sc.ChangeType)
	}
	if sc.OldStatus == nil || *sc.OldStatus != "subscription" {
		t.Errorf("old_status: want 'subscription', got %v", sc.OldStatus)
	}
	if sc.NewStatus == nil || *sc.NewStatus != "owned" {
		t.Errorf("new_status: want 'owned', got %v", sc.NewStatus)
	}
}

func TestDispatchSync_FactoryCredentialsError_SetsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if !credsErr {
		t.Error("expected credentials_error=true after factory ErrCredentials")
	}
}

func TestDispatchSync_FetchCredentialsError_SetsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     fetchErrFactory(tasks.ErrCredentials),
		RiverClient: nil,
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if !credsErr {
		t.Error("expected credentials_error=true after fetch ErrCredentials")
	}
}

func TestDispatchSync_Success_ClearsCredentialsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error)
		 VALUES (?, ?, 'steam', 'daily', true)`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if credsErr {
		t.Error("expected credentials_error=false after successful sync")
	}
}

func TestSyncCheckJobCompletion_FailedItemsYieldsCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Insert one failed item.
	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'failed', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "completed" {
		t.Errorf("expected status=completed, got %q", status)
	}
}

// TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing is the #642
// regression guard: while DispatchSyncWorker is still streaming batches
// (dispatch_complete=false), a transiently-empty active set must NOT finalize
// the job, or items from later batches get orphaned under a terminal job.
func TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Dispatch is still streaming batches.
	if _, err := testDB.NewRaw(`UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID).Exec(ctx); err != nil {
		t.Fatalf("set dispatch_complete=false: %v", err)
	}

	// One fully-resolved item, none pending_review — the empty-active set that
	// previously tripped premature completion.
	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "processing" {
		t.Errorf("dispatch incomplete: expected job to stay 'processing', got %q", status)
	}
}

// TestSyncCheckJobCompletion_DispatchComplete_Finalizes confirms the happy
// path: once dispatch is complete and no active/pending_review items remain,
// the job finalizes to 'completed'.
func TestSyncCheckJobCompletion_DispatchComplete_Finalizes(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1) // dispatch_complete defaults TRUE

	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "completed" {
		t.Errorf("dispatch complete, no active/review items: expected 'completed', got %q", status)
	}
}

// TestSyncCheckJobCompletion_PendingReviewBlocks confirms pending_review items
// keep the job processing even when dispatch is complete (existing invariant).
func TestSyncCheckJobCompletion_PendingReviewBlocks(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1) // dispatch_complete defaults TRUE

	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'pending_review', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "processing" {
		t.Errorf("pending_review present: expected job to stay 'processing', got %q", status)
	}
}

// TestUserGameWorker_BackfillsExternalGameID covers the spec invariant from
// docs/sync.md § "Manually added games": Stage 3 must always set
// external_game_id on user_game_platforms, even when the row pre-existed
// (e.g. manually added by the user) and the incoming sync brings neither an
// ownership rank upgrade nor a higher playtime. This is the no-op sub-case
// of the conflict branch — the one most prone to silently leaving
// external_game_id NULL forever.
func TestUserGameWorker_BackfillsExternalGameID(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1100)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Hades', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Pre-create a "manually added" user_game + user_game_platforms row:
	// external_game_id = NULL, ownership = 'owned', hours_played = 50.
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	ugpID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 50.0, 'owned', false, now(), now())`,
		ugpID, ugID,
	)

	// Incoming sync: same ownership rank ('owned'), lower hours (30 < 50) — the no-op sub-case.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1145360', 'Hades', false, true, false, 'owned', ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 30.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '1145360', 'Hades', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// external_game_id must now point at the synced external_games row.
	var gotEGID *string
	_ = testDB.NewRaw(
		`SELECT external_game_id FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotEGID)
	if gotEGID == nil || *gotEGID != egID {
		t.Errorf("external_game_id: want %q, got %v", egID, gotEGID)
	}

	// ownership_status unchanged (no upgrade).
	var gotOwnership string
	_ = testDB.NewRaw(
		`SELECT ownership_status FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotOwnership)
	if gotOwnership != "owned" {
		t.Errorf("ownership_status: want 'owned', got %q", gotOwnership)
	}

	// hours_played unchanged (incoming was lower).
	var gotHours float64
	_ = testDB.NewRaw(
		`SELECT hours_played FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotHours)
	if gotHours != 50.0 {
		t.Errorf("hours_played: want 50.0 (preserved), got %v", gotHours)
	}
}

// TestSync_IGDBMatch_PassesPlatformIDsFromExternalGame verifies that the
// IGDBMatchWorker resolves the external_game's platforms to IGDB IDs and passes
// them to SearchGames. The IGDB httptest server captures every request body; the
// test asserts that at least one body contains both platform IDs 6 (pc-windows)
// and 3 (pc-linux) in a "platforms = (...)" clause.
func TestSync_IGDBMatch_PassesPlatformIDsFromExternalGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Capture every Apicalypse body the IGDB client sends.
	var mu sync.Mutex
	var capturedBodies []string

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	const igdbID = int32(777)
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		capturedBodies = append(capturedBodies, string(raw))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": igdbID, "name": "Test Game", "slug": "test-game"},
		})
	}))
	defer igdbSrv.Close()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	// Seed platforms with their IGDB IDs (truncateAllTables clears the platforms
	// table, so we can't rely on the migration seed data).
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name, igdb_platform_id) VALUES ('pc-windows', 'PC (Windows)', 6) ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name, igdb_platform_id) VALUES ('pc-linux', 'PC (Linux)', 3) ON CONFLICT DO NOTHING`)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '999', 'Test Game', false, true, false)`,
		egID, userID,
	)
	// Attach two platforms whose igdb_platform_id values were seeded above.
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-linux', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Test Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At least one request body must contain both IGDB platform IDs 6 and 3.
	mu.Lock()
	bodies := capturedBodies
	mu.Unlock()

	if len(bodies) == 0 {
		t.Fatal("no requests reached the IGDB mock server")
	}

	// With the resolver's ORDER BY p.igdb_platform_id, the clause is built as
	// platforms = (3,6) (ascending). Assert on the exact clause to avoid
	// substring collisions with e.g. `limit 6;`.
	foundPlatformFilter := false
	for _, b := range bodies {
		if strings.Contains(b, "platforms = (3,6)") {
			foundPlatformFilter = true
			break
		}
	}
	if !foundPlatformFilter {
		t.Errorf("expected at least one IGDB request body to contain 'platforms = (3,6)'; got bodies: %v", bodies)
	}
}

func TestUserGameWorker_AlreadyInLibrary_WritesSyncChange(t *testing.T) {
	// A game whose user_games row already exists with no ownership upgrade
	// must produce a sync_changes('already_in_library') row and no 'added' row.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1001)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Existing Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Pre-seed user_games and user_game_platforms so the game is already in library.
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 10.0, 'owned', true, now(), now())`,
		uuid.NewString(), ugID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1001', 'Existing Game', false, true, false, 'owned', ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 10.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '1001', 'Existing Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have exactly one already_in_library sync_change.
	var alreadyCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'already_in_library'`, jobID,
	).Scan(ctx, &alreadyCount)
	if alreadyCount != 1 {
		t.Errorf("expected 1 already_in_library sync_change, got %d", alreadyCount)
	}

	// Must have zero 'added' rows.
	var addedCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &addedCount)
	if addedCount != 0 {
		t.Errorf("expected 0 added sync_changes, got %d", addedCount)
	}
}

func TestUserGameWorker_WorkerAutoSkip_WritesSyncChange(t *testing.T) {
	// When eg.IsSkipped=true, the worker must write sync_changes('skipped')
	// before marking the job_item skipped.
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '999', 'Skipped Game', true, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Skipped Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// sync_changes('skipped') must exist with the correct title.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM sync_changes WHERE job_id = ?`, jobID,
	).Scan(ctx, &sc); err != nil {
		t.Fatalf("scan sync_change: %v", err)
	}
	if sc.ChangeType != "skipped" {
		t.Errorf("change_type: want 'skipped', got %q", sc.ChangeType)
	}
	if sc.Title != "Skipped Game" {
		t.Errorf("title: want 'Skipped Game', got %q", sc.Title)
	}
}

// userGameStage3Fixture seeds the rows a UserGameWorker run needs: user,
// platform, storefront, games row (description NULL), external_game (resolved),
// one external_game_platform, and a pending sync job_item. It returns the
// job_item ID and the IGDB game ID.
func userGameStage3Fixture(t *testing.T) (itemID string, igdbID int32) {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 42.5, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID = uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)
	return itemID, igdbID
}

func TestUserGameWorker_EnqueuesImmediateMetadataFetch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, igdbID := userGameStage3Fixture(t)

	srv := igdbTestServer(t, `[]`) // never actually called — UserGameWorker only checks Configured()
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch' AND (args->>'game_id')::int = ?`, igdbID,
	).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("metadata_fetch river_job for game %d: want 1, got %d", igdbID, count)
	}
}

func TestUserGameWorker_SkipsMetadataFetchWhenDescriptionPresent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, igdbID := userGameStage3Fixture(t)
	// Game already has metadata.
	_, _ = testDB.NewRaw(`UPDATE games SET description = 'already here' WHERE id = ?`, igdbID).Exec(ctx)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch'`).Scan(ctx, &count)
	if count != 0 {
		t.Errorf("metadata_fetch river_job: want 0 (description present), got %d", count)
	}
}

func TestUserGameWorker_SkipsMetadataFetchWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	itemID, _ := userGameStage3Fixture(t)

	unconfigured := igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	rc := newTestRiverClient(t)

	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: unconfigured, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_fetch'`).Scan(ctx, &count)
	if count != 0 {
		t.Errorf("metadata_fetch river_job: want 0 (IGDB not configured), got %d", count)
	}
}

// streamProbeAdapter yields batches and invokes probe() after the first batch
// (i.e. while dispatch is still mid-stream) so a test can observe the job's
// dispatch_complete flag during streaming.
type streamProbeAdapter struct {
	batches [][]tasks.ExternalGameEntry
	probe   func()
}

func (a *streamProbeAdapter) GetLibrary(_ context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
	for i, batch := range a.batches {
		if err := onBatch(batch); err != nil {
			return err
		}
		if i == 0 && a.probe != nil {
			a.probe()
		}
	}
	return nil
}

// TestDispatchSync_FlagFalseWhileStreaming proves DispatchSyncWorker sets
// dispatch_complete=false while it is still streaming batches, and true once
// dispatch finishes.
func TestDispatchSync_FlagFalseWhileStreaming(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	var midStreamFlag bool
	adapter := &streamProbeAdapter{
		batches: [][]tasks.ExternalGameEntry{
			{{ExternalID: "1", Title: "Game A", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
			{{ExternalID: "2", Title: "Game B", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
		},
		probe: func() {
			_ = testDB.NewRaw(`SELECT dispatch_complete FROM jobs WHERE id = ?`, jobID).Scan(ctx, &midStreamFlag)
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if midStreamFlag {
		t.Error("expected dispatch_complete=false while dispatch is still streaming batches")
	}

	var finalFlag bool
	if err := testDB.NewRaw(`SELECT dispatch_complete FROM jobs WHERE id = ?`, jobID).Scan(ctx, &finalFlag); err != nil {
		t.Fatalf("query dispatch_complete: %v", err)
	}
	if !finalFlag {
		t.Error("expected dispatch_complete=true after dispatch finished")
	}
}

// TestDispatchSync_EmptyLibrary_Finalizes proves an empty library finalizes the
// job: dispatch sets dispatch_complete=true and runs the authoritative
// completion check, which finds no active/pending_review items.
func TestDispatchSync_EmptyLibrary_Finalizes(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	// Adapter yields no games at all.
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "completed" {
		t.Errorf("empty library: expected job 'completed', got %q", status)
	}
}

func TestUserGameWorker_PlayStatus_NewGame_WithHours_SetsInProgress(t *testing.T) {
	// New user_games row + incoming hours > 0 → play_status = 'in_progress'.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1001)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1001', 'Test Game', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 10.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1001', 'Test Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "in_progress" {
		t.Errorf("play_status: want 'in_progress', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_NewGame_NoHours_SetsNotStarted(t *testing.T) {
	// New user_games row + incoming hours = 0 → play_status = 'not_started' (DB default).
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1002)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1002', 'Test Game 2', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1002', 'Test Game 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "not_started" {
		t.Errorf("play_status: want 'not_started', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_ExistingNotStarted_WithHours_PromotesToInProgress(t *testing.T) {
	// Existing row with play_status='not_started' + hours > 0 → promoted to 'in_progress'.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1003)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 3', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, play_status, created_at, updated_at) VALUES (?, ?, ?, 'not_started', now(), now())`,
		ugID, userID, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1003', 'Test Game 3', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 5.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1003', 'Test Game 3', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "in_progress" {
		t.Errorf("play_status: want 'in_progress', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_ExistingUserSet_NeverOverwritten(t *testing.T) {
	// Existing row with play_status='completed' (user-set) must never be overwritten by sync,
	// even when incoming hours > 0.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1004)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 4', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, play_status, created_at, updated_at) VALUES (?, ?, ?, 'completed', now(), now())`,
		ugID, userID, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1004', 'Test Game 4', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 50.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1004', 'Test Game 4', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "completed" {
		t.Errorf("play_status: want 'completed' (unchanged), got %q", playStatus)
	}
}
