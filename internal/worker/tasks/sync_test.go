package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/amazongames"
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), LC: newTestLC(t)}

	// With River, args are already typed — test that a job with empty args
	// (no matching sync config) returns nil without panicking.
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{})
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestDispatchSync_FailsJob is a table-driven consolidation of every dispatch
// path that must leave the job in status='failed'. The worker is
// storefront-agnostic (it drives all storefronts through the injected adapter
// factory), so these rows vary only by (storefront, adapter factory): the
// missing-config path, factory/fetch credentials errors, an unknown-storefront
// factory error, and a transport fetch error, across steam, playstation-store,
// and epic-games-store.
func TestDispatchSync_FailsJob(t *testing.T) {
	unknownFactory := func(_ context.Context, _ string, _ models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		return nil, errors.New("unknown storefront: bogus")
	}

	tests := []struct {
		name       string
		storefront string
		// withConfig inserts a user_sync_configs row when true; the no-config
		// row leaves it false to exercise the "no sync config found" path.
		withConfig bool
		factory    func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error)
	}{
		{
			name:       "no_sync_config",
			storefront: "steam",
			withConfig: false,
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return adapterFactory(&fakeStorefrontAdapter{})
			},
		},
		{
			// Was both _NoCredentials and _SteamInvalidCredentials (exact dupes).
			name:       "steam_credentials_error",
			storefront: "steam",
			withConfig: true,
			factory:    credErrFactory,
		},
		{
			name:       "unknown_storefront",
			storefront: "bogus",
			withConfig: true,
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return unknownFactory
			},
		},
		{
			name:       "steam_fetch_error",
			storefront: "steam",
			withConfig: true,
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return fetchErrFactory(errSteamFetch)
			},
		},
		{
			name:       "psn_credentials_error",
			storefront: "playstation-store",
			withConfig: true,
			factory:    credErrFactory,
		},
		{
			name:       "psn_fetch_error",
			storefront: "playstation-store",
			withConfig: true,
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return fetchErrFactory(errors.New("request failed with status 503: service unavailable"))
			},
		},
		{
			// Adapter returns ErrCredentials mid-fetch (auth lost during fetch).
			name:       "psn_fetch_credentials_error",
			storefront: "playstation-store",
			withConfig: true,
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return fetchErrFactory(tasks.ErrCredentials)
			},
		},
		{
			name:       "epic_not_configured",
			storefront: "epic-games-store",
			withConfig: true,
			factory:    credErrFactory,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID := uuid.NewString()
			jobID := uuid.NewString()
			insertTestUser(t, testDB, userID)

			_, _ = testDB.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', ?, 'pending', 'low')`,
				jobID, userID, tc.storefront,
			).Exec(ctx)
			if tc.withConfig {
				_, _ = testDB.NewRaw(
					`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, ?, 'daily')`,
					uuid.NewString(), userID, tc.storefront,
				).Exec(ctx)
			}

			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: tc.factory(), LC: newTestLC(t)}
			job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: tc.storefront})

			if err := w.Work(ctx, job); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var status string
			_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
			if status != "failed" {
				t.Errorf("expected job status=failed, got %q", status)
			}
		})
	}
}

var errSteamFetch = errType("steam fetch failed")

type errType string

func (e errType) Error() string { return string(e) }

// TestDispatchSync_SingleBatchHappyPath consolidates the single-batch success
// paths that were per-storefront tests (_SteamSuccess, _GOGDispatch_CreatesExternalGames,
// _Epic_HappyPath). The worker is storefront-agnostic, so each row varies only
// by (storefront, library batch) and asserts the same invariants: every game is
// upserted into external_games, a representative platform row exists, the job is
// not failed, and last_synced_at is set. The multi-page batch case
// (_PSNSuccess_ItemsDispatchedPerBatch) is kept separate.
func TestDispatchSync_SingleBatchHappyPath(t *testing.T) {
	tests := []struct {
		name       string
		storefront string
		games      []tasks.ExternalGameEntry
		// probeExternalID + probePlatform identify one (game, platform) pair that
		// must have a row in external_game_platforms.
		probeExternalID string
		probePlatform   string
	}{
		{
			name:       "steam",
			storefront: "steam",
			games: []tasks.ExternalGameEntry{
				{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			},
			probeExternalID: "730",
			probePlatform:   "pc-windows",
		},
		{
			name:       "gog",
			storefront: "gog",
			games: []tasks.ExternalGameEntry{
				{ExternalID: "1001", Title: "GOG Game", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			},
			probeExternalID: "1001",
			probePlatform:   "pc-windows",
		},
		{
			name:       "epic",
			storefront: "epic-games-store",
			games: []tasks.ExternalGameEntry{
				{ExternalID: "Fortnite", Title: "Fortnite", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
				{ExternalID: "RocketLeague", Title: "Rocket League", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			},
			probeExternalID: "Fortnite",
			probePlatform:   "pc-windows",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID := uuid.NewString()
			jobID := uuid.NewString()
			insertTestUser(t, testDB, userID)

			_, _ = testDB.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', ?, 'pending', 'low', 0)`,
				jobID, userID, tc.storefront,
			).Exec(ctx)
			configID := uuid.NewString()
			_, _ = testDB.NewRaw(
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, ?, 'daily')`,
				configID, userID, tc.storefront,
			).Exec(ctx)

			fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{tc.games}}
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
			job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: tc.storefront})

			if err := w.Work(ctx, job); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Every game is upserted into external_games.
			var count int
			_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = ?`, userID, tc.storefront).Scan(ctx, &count)
			if count != len(tc.games) {
				t.Errorf("expected %d external_games, got %d", len(tc.games), count)
			}

			// The probed (game, platform) pair has a platform row.
			var platformCount int
			_ = testDB.NewRaw(
				`SELECT COUNT(*) FROM external_game_platforms egp
				 JOIN external_games eg ON eg.id = egp.external_game_id
				 WHERE eg.user_id = ? AND eg.storefront = ? AND eg.external_id = ? AND egp.platform = ?`,
				userID, tc.storefront, tc.probeExternalID, tc.probePlatform,
			).Scan(ctx, &platformCount)
			if platformCount != 1 {
				t.Errorf("expected 1 %s platform row for %s, got %d", tc.probePlatform, tc.probeExternalID, platformCount)
			}

			// Job must not be failed.
			var status string
			_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
			if status == "failed" {
				t.Errorf("expected job not to be failed, got status=%q", status)
			}

			// last_synced_at should be set after a successful sync.
			var lastSynced *time.Time
			_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &lastSynced)
			if lastSynced == nil {
				t.Error("expected last_synced_at to be set after successful sync")
			}
		})
	}
}

// TestDispatchSync_MacPlatformPersisted is a regression test for #756: a "mac"
// platform emitted by an adapter must survive end-to-end into
// external_game_platforms. Previously the raw string "mac" was silently dropped
// because the code allowlist only recognised "pc-mac".
func TestDispatchSync_MacPlatformPersisted(t *testing.T) {
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
		{{ExternalID: "1145360", Title: "Hades", PlaytimeHours: 50, Platforms: []string{"mac"}, OwnershipStatus: "owned"}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var platform string
	if err := testDB.NewRaw(
		`SELECT egp.platform FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.external_id = '1145360'`,
		userID,
	).Scan(ctx, &platform); err != nil {
		t.Fatalf("expected a platform row for the mac game, got error: %v", err)
	}
	if platform != "mac" {
		t.Errorf("platform: got %q, want %q", platform, "mac")
	}
}

// TestExternalGamePlatformsFK verifies the external_game_platforms.platform ->
// platforms(name) foreign key rejects an unknown platform slug. This guarantee
// replaces the former code-level PlatformToSlug allowlist.
func TestExternalGamePlatformsFK(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	egID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, ownership_status, created_at, updated_at)
		 VALUES (?, ?, 'steam', '1', 'Test Game', true, 'owned', now(), now())`,
		egID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("failed to insert external_game: %v", err)
	}

	// A canonical slug is accepted.
	if _, err := testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, 'mac', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx); err != nil {
		t.Fatalf("expected canonical platform 'mac' to be accepted, got: %v", err)
	}

	// An unknown raw string (the kind the old allowlist silently dropped) is rejected by the FK.
	if _, err := testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, 'pc-mac', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx); err == nil {
		t.Error("expected FK violation inserting unknown platform 'pc-mac', got nil error")
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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

	fakeAdapter := &fakeStorefrontAdapter{}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', true, true)`,
		egID, userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{
			{ExternalID: "730", Title: "Counter-Strike 2", PlaytimeHours: 100, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
			{ExternalID: "570", Title: "Dota 2", PlaytimeHours: 50, Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
		},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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

	// The pre-skipped CS2 must produce a changes('skipped') row.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM changes WHERE job_id = ? AND external_game_id = ?`,
		jobID, egID,
	).Scan(ctx, &sc); err != nil {
		t.Fatalf("scan change for skipped game: %v", err)
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
	// it; this test fails if anyone later truncates to int in the platform upsert (externalgame.Seen).
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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

func TestDispatchSync_PSNSuccess_ItemsDispatchedPerBatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'playstation-store', 'pending', 'low', 0)`,
		jobID, userID,
	).Exec(ctx)
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'playstation-store', 'daily')`,
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "playstation-store"})
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 games upserted as external_games.
	var egCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'playstation-store'`, userID).Scan(ctx, &egCount)
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

// insertTestExternalGame inserts a minimal external_games row and one
// external_game_platforms row. Returns the external_game id.
func insertTestExternalGame(t *testing.T, userID, storefront, externalID, title, platform string) string {
	t.Helper()
	egID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, ?, ?, ?, false, true)`,
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

// TestDispatchSync_DualPlatformCreatesTwoRows consolidates the dual-platform
// dispatch tests (_Steam_MultiPlatform_WindowsAndLinux and
// _GOGDispatch_DualPlatformCreatesTwoRows). A single external_id carrying two
// platforms must yield 1 external_games row, 2 external_game_platforms rows, and
// exactly 1 job_item (one per game, not per platform). The worker is
// storefront-agnostic, so steam and gog differ only by storefront and the
// item_key probe (the Steam case additionally asserted item_key == external_id).
func TestDispatchSync_DualPlatformCreatesTwoRows(t *testing.T) {
	tests := []struct {
		name       string
		storefront string
		externalID string
		platforms  []string
	}{
		{
			name:       "steam_windows_linux",
			storefront: "steam",
			externalID: "730",
			platforms:  []string{"pc-windows", "pc-linux"},
		},
		{
			name:       "gog_windows_linux",
			storefront: "gog",
			externalID: "2001",
			platforms:  []string{"pc-windows", "pc-linux"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID := uuid.NewString()
			jobID := uuid.NewString()
			insertTestUser(t, testDB, userID)

			_, _ = testDB.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', ?, 'pending', 'low', 0)`,
				jobID, userID, tc.storefront,
			).Exec(ctx)
			_, _ = testDB.NewRaw(
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, ?, 'daily')`,
				uuid.NewString(), userID, tc.storefront,
			).Exec(ctx)

			fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
				{{ExternalID: tc.externalID, Title: "Dual Game", PlaytimeHours: 100, Platforms: tc.platforms, OwnershipStatus: "owned"}},
			}}
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
			job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: tc.storefront})
			if err := w.Work(ctx, job); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var egCount int
			_ = testDB.NewRaw(
				`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = ? AND external_id = ?`,
				userID, tc.storefront, tc.externalID,
			).Scan(ctx, &egCount)
			if egCount != 1 {
				t.Errorf("want 1 external_game for dual-platform game, got %d", egCount)
			}

			var egpCount int
			_ = testDB.NewRaw(
				`SELECT COUNT(*) FROM external_game_platforms egp
				 JOIN external_games eg ON eg.id = egp.external_game_id
				 WHERE eg.user_id = ? AND eg.external_id = ?`,
				userID, tc.externalID,
			).Scan(ctx, &egpCount)
			if egpCount != 2 {
				t.Errorf("want 2 external_game_platforms rows, got %d", egpCount)
			}

			var itemCount int
			_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
			if itemCount != 1 {
				t.Errorf("want 1 job_item (one per game, not per platform), got %d", itemCount)
			}

			var itemKey string
			_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
			if itemKey != tc.externalID {
				t.Errorf("expected item_key=%s, got %q", tc.externalID, itemKey)
			}
		})
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available) VALUES (?, ?, 'steam', '1', 'Game', false, true)`,
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
		DB:      testDB,
		Adapter: fetchErrFactory(errors.New("network error")),
		LC:      newTestLC(t),
	}
	_ = w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"}))

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

	// Sync returns a library without game 999 — it is gone. (The fetch must be
	// non-empty: an empty fetch with known games fails the job, see
	// TestDispatchSync_EmptyFetchWithKnownGames_FailsWithoutSweep.)
	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{{
		{ExternalID: "1000", Title: "Kept Game", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"},
	}}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	_ = w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"}))

	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 removed change, got %d", changeCount)
	}
	var changeTitle string
	_ = testDB.NewRaw(
		`SELECT title FROM changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeTitle)
	if changeTitle != "Old Game" {
		t.Errorf("change title: want 'Old Game', got %q", changeTitle)
	}
}

// An empty fetch while games are known must not run the removal sweep (#1324):
// the job fails, every game stays available, and no 'removed' change is written
// (those feed the sync.diff notification).
func TestDispatchSync_EmptyFetchWithKnownGames_FailsWithoutSweep(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'gog', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'gog', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	insertTestExternalGame(t, userID, "gog", "1", "Game One", "pc-windows")
	insertTestExternalGame(t, userID, "gog", "2", "Game Two", "pc-windows")
	// A game on another storefront must not count as "known" for this one.
	insertTestExternalGame(t, userID, "steam", "3", "Steam Game", "pc-windows")

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var status, errMsg string
	var dispatchComplete bool
	if err := testDB.NewRaw(
		`SELECT status, COALESCE(error_message, ''), dispatch_complete FROM jobs WHERE id = ?`, jobID,
	).Scan(ctx, &status, &errMsg, &dispatchComplete); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "failed" {
		t.Errorf("job status: want failed, got %q", status)
	}
	if !strings.Contains(errMsg, "empty library while 2 games are known") {
		t.Errorf("error message should name the known-game count, got %q", errMsg)
	}

	var stillAvailable int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'gog' AND is_available = true`, userID,
	).Scan(ctx, &stillAvailable)
	if stillAvailable != 2 {
		t.Errorf("expected both games still available, got %d", stillAvailable)
	}
	var removed int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &removed)
	if removed != 0 {
		t.Errorf("expected no removed changes, got %d", removed)
	}
	var lastSynced *time.Time
	_ = testDB.NewRaw(
		`SELECT last_synced_at FROM user_sync_configs WHERE user_id = ? AND storefront = 'gog'`, userID,
	).Scan(ctx, &lastSynced)
	if lastSynced != nil {
		t.Errorf("last_synced_at must stay unset on a refused sync, got %v", lastSynced)
	}
}

// An empty fetch with nothing known is a genuinely empty library (first sync of
// an empty account, or games already marked unavailable): the job completes.
func TestDispatchSync_EmptyFetchNoKnownGames_Completes(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'gog', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'gog', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	// Already-unavailable rows are not "known available" and must not trip the guard.
	insertTestExternalGame(t, userID, "gog", "1", "Gone Game", "pc-windows")
	_, _ = testDB.ExecContext(ctx,
		`UPDATE external_games SET is_available = false WHERE user_id = ? AND storefront = 'gog'`, userID)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("job status: want completed, got %q", status)
	}
}

// ---------------------------------------------------------------------------
// IGDBMatchWorker — Stage 2
// ---------------------------------------------------------------------------

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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '111', 'Unknown Game', false, true)`,
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

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, LC: newTestLC(t)}
	job := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike™ 2', false, true)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike™ 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: igdbClient, LC: newTestLC(t)}
	job := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// The games row exists and has the matched IGDB title, not the
	// storefront's (#1455).
	var gameTitle string
	_ = testDB.NewRaw(`SELECT title FROM games WHERE id = ?`, igdbID).Scan(ctx, &gameTitle)
	if gameTitle != "Counter-Strike 2" {
		t.Errorf("games.title: want the IGDB title %q, got %q", "Counter-Strike 2", gameTitle)
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

	// Two separate batches: batch 1 has game-A, batch 2 has game-B.
	adapter := &fakeStorefrontAdapter{
		batches: [][]tasks.ExternalGameEntry{
			{{ExternalID: "game-A", Title: "Game A", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
			{{ExternalID: "game-B", Title: "Game B", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
		},
	}
	w := &tasks.DispatchSyncWorker{
		DB:      testDB,
		Adapter: adapterFactory(adapter),
		LC:      newTestLC(t),
	}

	job := syncDispatchJob(t, tasks.DispatchSyncArgs{
		JobID: jobID, UserID: userID, Storefront: "steam",
	})
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	job := writeJob(t, tasks.UserGameArgs{JobItemID: itemID})
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
	// changes: added.
	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 added change, got %d", changeCount)
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
	// The playstation-store storefront slug equals storefronts.name (no translation).
	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('playstation-4', 'PlayStation 4') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('playstation-store', 'PlayStation Store') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'playstation-store', 'processing', 'low', 1)`,
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'playstation-store', 'CUSA800', 'PSN Game', false, true, ?, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
	// No change row: game already existed (not a new addition).
	var changeCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'added'`, jobID).Scan(ctx, &changeCount)
	if changeCount != 0 {
		t.Errorf("expected 0 added changes (game pre-existed), got %d", changeCount)
	}
}

func TestUserGameWorker_StatusChangedSyncChange(t *testing.T) {
	// Existing UGP has subscription; new sync says owned (upgrade).
	// Expect a status_changed change row.
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '900', 'Status Change Game', false, true, 'owned', ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc struct {
		ChangeType string  `bun:"change_type"`
		OldStatus  *string `bun:"old_status"`
		NewStatus  *string `bun:"new_status"`
	}
	_ = testDB.NewRaw(
		`SELECT change_type, old_status, new_status FROM changes WHERE job_id = ?`, jobID,
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

// TestDispatchSync_CredentialsError_SetsFlag asserts credentials_error is set
// to true whether ErrCredentials surfaces from the adapter factory (before the
// fetch) or from GetLibrary (mid-fetch) — the worker treats both identically.
func TestDispatchSync_CredentialsError_SetsFlag(t *testing.T) {
	tests := []struct {
		name    string
		factory func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error)
	}{
		{
			name:    "factory_error",
			factory: credErrFactory,
		},
		{
			name: "fetch_error",
			factory: func() func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
				return fetchErrFactory(tasks.ErrCredentials)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: tc.factory(), LC: newTestLC(t)}
			job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})
			if err := w.Work(ctx, job); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var credsErr bool
			_ = testDB.NewRaw(
				`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
				userID,
			).Scan(ctx, &credsErr)
			if !credsErr {
				t.Error("expected credentials_error=true after ErrCredentials")
			}
		})
	}
}

// amazonLibraryStub stands in for nile behind the real Amazon Games adapter.
type amazonLibraryStub struct{ err error }

func (s amazonLibraryStub) Configured() bool { return true }

func (s amazonLibraryStub) Library(context.Context, json.RawMessage) ([]amazongames.Entitlement, json.RawMessage, error) {
	return nil, nil, s.err
}

// TestDispatchSync_AmazonGames_FailureChangesNoGames runs the real Amazon
// Games adapter through the worker: a rejected sign-in takes the shared
// credentials path (credentials_error set) and any failure leaves the library
// as it was — nothing is marked removed.
func TestDispatchSync_AmazonGames_FailureChangesNoGames(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantFlag  bool
		wantState string
	}{
		{"sign-in rejected", amazongames.ErrAuthFailed, true, "failed"},
		{"library read failed", errors.New("amazon: nile library sync failed: KeyError"), false, "failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			ctx := context.Background()
			userID := uuid.NewString()
			jobID := uuid.NewString()
			insertTestUser(t, testDB, userID)
			_, _ = testDB.ExecContext(ctx,
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'amazon-games', 'pending', 'low')`,
				jobID, userID,
			)
			_, _ = testDB.NewRaw(
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'amazon-games', 'daily')`,
				uuid.NewString(), userID,
			).Exec(ctx)
			insertTestExternalGame(t, userID, "amazon-games", "amzn1.adg.product.1", "Owned Game", "pc-windows")

			adapter := amazongames.NewAdapter(amazonLibraryStub{err: tc.err}, json.RawMessage(`{}`), nil)
			w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}
			if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "amazon-games"})); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var credsErr bool
			_ = testDB.NewRaw(`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games'`, userID).Scan(ctx, &credsErr)
			if credsErr != tc.wantFlag {
				t.Errorf("credentials_error = %v, want %v", credsErr, tc.wantFlag)
			}
			var status string
			_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
			if status != tc.wantState {
				t.Errorf("job status = %q, want %q", status, tc.wantState)
			}
			var available bool
			_ = testDB.NewRaw(`SELECT is_available FROM external_games WHERE user_id = ? AND external_id = 'amzn1.adg.product.1'`, userID).Scan(ctx, &available)
			if !available {
				t.Error("an owned game was marked unavailable by a failed sync")
			}
			var changes int
			_ = testDB.NewRaw(`SELECT COUNT(*) FROM changes WHERE job_id = ?`, jobID).Scan(ctx, &changes)
			if changes != 0 {
				t.Errorf("a failed sync wrote %d changes", changes)
			}
		})
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})
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

// syncCompletionFixture inserts an active sync Job for storefront with its
// dispatch done and one pending item per status in pending (owned by a fake
// River job) plus the given ended or in-review items. It returns the Job id
// and the pending items' ids.
func syncCompletionFixture(t *testing.T, storefront string, dispatchComplete bool, pending int, others ...models.JobItemStatus) (userID, jobID string, pendingIDs []string) {
	t.Helper()
	ctx := context.Background()
	userID, jobID = uuid.NewString(), uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, started_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'low', 1, ?, now())`,
		jobID, userID, storefront, dispatchComplete,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	insert := func(status models.JobItemStatus, owner any) string {
		id := uuid.NewString()
		if _, err := testDB.ExecContext(ctx,
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, river_job_id, created_at)
			 VALUES (?, ?, ?, ?, 'Game', '{}', ?, '{}', '[]', ?, now())`,
			id, jobID, userID, id, status, owner,
		); err != nil {
			t.Fatalf("insert item: %v", err)
		}
		return id
	}
	for range pending {
		pendingIDs = append(pendingIDs, insert("pending", nextFakeOwner()))
	}
	for _, st := range others {
		insert(st, nil)
	}
	return userID, jobID, pendingIDs
}

func endSyncItem(t *testing.T, lc *joblifecycle.Module, itemID string, status models.JobItemStatus) {
	t.Helper()
	if err := lc.EndItem(context.Background(), itemID, itemOwner(t, itemID), joblifecycle.ItemResult{Status: status, Message: "boom"}); err != nil {
		t.Fatalf("end item: %v", err)
	}
}

// The last item ending completes the sync, even when items failed: a
// failed item is surfaced through the completed-with-errors notification,
// not the Job status.
func TestSyncCompletion_FailedItemYieldsCompletedWithErrors(t *testing.T) {
	truncateAllTables(t)
	lc := newTestLC(t)
	userID, jobID, items := syncCompletionFixture(t, "steam", true, 2)

	endSyncItem(t, lc, items[0], models.JobItemStatusFailed)
	if got := jobStatus(t, jobID); got != models.JobStatusProcessing {
		t.Fatalf("job with a pending item left: want processing, got %q", got)
	}
	endSyncItem(t, lc, items[1], models.JobItemStatusCompleted)
	if got := jobStatus(t, jobID); got != models.JobStatusCompleted {
		t.Fatalf("job: want completed, got %q", got)
	}
	runSyncEffects(t, lc)
	if n := eventCount(t, jobID+":"+notify.TypeSyncCompletedWithErrors); n != 1 {
		t.Errorf("sync.completed_with_errors events: want 1, got %d", n)
	}
	if n := eventCount(t, jobID+":"+notify.TypeSyncCompleted); n != 0 {
		t.Errorf("sync.completed must not be sent with a failed item, got %d", n)
	}
	_ = userID
}

// The sync.diff payload carries the storefront's display name (looked up from
// the storefronts table by source slug), so the notification can name its
// source.
func TestSyncCompletion_SyncDiffNamesStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	lc := newTestLC(t)
	// playstation-store → "PlayStation Store" exercises a multi-word display
	// name that a naive slug-titlecase would mangle.
	userID, jobID, items := syncCompletionFixture(t, "playstation-store", true, 1)
	if _, err := testDB.NewRaw(`
		INSERT INTO changes (id, job_id, user_id, change_type, title, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'added', 'Bloodborne', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert change: %v", err)
	}

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	runSyncEffects(t, lc)

	var payload []byte
	if err := testDB.NewRaw(
		`SELECT payload FROM events WHERE type = ? AND actor_user_id = ?`,
		notify.TypeSyncDiff, userID,
	).Scan(ctx, &payload); err != nil {
		t.Fatalf("query sync.diff event: %v", err)
	}
	var p notify.SyncDiffPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Storefront != "PlayStation Store" {
		t.Errorf("payload storefront = %q, want %q", p.Storefront, "PlayStation Store")
	}
}

// #642: while the dispatch is still adding batches, the last item ending
// must not complete the sync, or later batches' items would be orphaned under
// an ended Job.
func TestSyncCompletion_DispatchNotDone_StaysProcessing(t *testing.T) {
	truncateAllTables(t)
	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", false, 1)

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	if got := jobStatus(t, jobID); got != models.JobStatusProcessing {
		t.Errorf("dispatch not done: want processing, got %q", got)
	}
}

// Items in Pending review keep the sync processing once nothing is left for a
// worker, and the user is told once.
func TestSyncCompletion_PendingReviewBlocksAndNotifiesOnce(t *testing.T) {
	truncateAllTables(t)
	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", true, 2, models.JobItemStatusPendingReview)

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	endSyncItem(t, lc, items[1], models.JobItemStatusCompleted)
	if got := jobStatus(t, jobID); got != models.JobStatusProcessing {
		t.Errorf("pending_review present: want processing, got %q", got)
	}
	runSyncEffects(t, lc)
	runSyncEffects(t, lc)
	if n := eventCount(t, jobID+":"+notify.TypeSyncNeedsReview); n != 1 {
		t.Errorf("sync.needs_review events: want 1, got %d", n)
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1145360', 'Hades', false, true, 'owned', ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '999', 'Test Game', false, true)`,
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

	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Test Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: igdbClient, LC: newTestLC(t)}
	job := matchJob(t, tasks.IGDBMatchArgs{JobItemID: itemID})
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
	// must produce a changes('already_in_library') row and no 'added' row.
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1001', 'Existing Game', false, true, 'owned', ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have exactly one already_in_library change row.
	var alreadyCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'already_in_library'`, jobID,
	).Scan(ctx, &alreadyCount)
	if alreadyCount != 1 {
		t.Errorf("expected 1 already_in_library change, got %d", alreadyCount)
	}

	// Must have zero 'added' rows.
	var addedCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &addedCount)
	if addedCount != 0 {
		t.Errorf("expected 0 added changes, got %d", addedCount)
	}
}

func TestUserGameWorker_WorkerAutoSkip_WritesSyncChange(t *testing.T) {
	// When eg.IsSkipped=true, the worker must write changes('skipped')
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '999', 'Skipped Game', true, true)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// changes('skipped') must exist with the correct title.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM changes WHERE job_id = ?`, jobID,
	).Scan(ctx, &sc); err != nil {
		t.Fatalf("scan change: %v", err)
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t), IGDBClient: unconfigured, RiverClient: rc}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
// the dispatch is done.
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
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
		t.Error("expected dispatch_complete=true after dispatch was done")
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

	// Adapter yields no games at all.
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})

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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1001', 'Test Game', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1002', 'Test Game 2', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1003', 'Test Game 3', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1004', 'Test Game 4', false, true, ?)`,
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

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
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

// TestUserGameWorker_PromotesWishlistedGame is a regression guard for #867: a
// wishlisted user_games row (is_wishlisted=true, no platforms) must be
// auto-promoted to the library (is_wishlisted cleared) when the sync worker
// attaches a platform to the same (user, game).
func TestUserGameWorker_PromotesWishlistedGame(t *testing.T) {
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
	const igdbID = int32(1005)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Wishlist Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)

	// Pre-create a wishlisted user_games row for this (user, game) — no platforms yet.
	wishlistUGID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, is_wishlisted, created_at, updated_at) VALUES (?, ?, ?, true, now(), now())`,
		wishlistUGID, userID, igdbID,
	)

	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1005', 'Wishlist Game', false, true, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 5.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1005', 'Wishlist Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, LC: newTestLC(t)}
	if err := w.Work(ctx, writeJob(t, tasks.UserGameArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The wishlisted game must be auto-promoted to the library (is_wishlisted cleared).
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).
		Where("user_id = ?", userID).
		Where("game_id = ?", igdbID).
		Scan(ctx); err != nil {
		t.Fatalf("load user_game: %v", err)
	}
	if ug.IsWishlisted {
		t.Fatal("sync should have cleared is_wishlisted (auto-promote to library)")
	}

	// The game must also have at least one platform after the sync.
	var ugpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ug.ID,
	).Scan(ctx, &ugpCount)
	if ugpCount < 1 {
		t.Errorf("expected ≥1 platform after sync, got %d", ugpCount)
	}
}

// ---------------------------------------------------------------------------
// Ending effects — notifications
// ---------------------------------------------------------------------------

func TestSyncCompletion_EmitsCompleted(t *testing.T) {
	truncateAllTables(t)
	notify.SetRiverClient(nil)
	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", true, 1)

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	runSyncEffects(t, lc)
	// Run again (a River retry): the dedup key keeps it to one event.
	if _, err := testDB.ExecContext(context.Background(), `UPDATE river_job SET state = 'available', finalized_at = NULL WHERE kind = ?`, joblifecycle.EffectsArgs{}.Kind()); err != nil {
		t.Fatal(err)
	}
	runSyncEffects(t, lc)

	if n := eventCount(t, jobID+":sync.completed"); n != 1 {
		t.Fatalf("sync.completed events: want 1, got %d", n)
	}
}

func TestFailSyncJobEmitsFailed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	jobID := uuid.NewString()

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user_sync_config: %v", err)
	}

	// Drive the real worker with a fetch error — this triggers FailSyncJob internally.
	w := &tasks.DispatchSyncWorker{
		DB:      testDB,
		Adapter: fetchErrFactory(errors.New("network error")),
		LC:      newTestLC(t),
	}
	_ = w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"}))
	runSyncEffects(t, w.LC)

	// sync.failed event must exist with the correct dedup_key.
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.failed").Scan(ctx, &count); err != nil {
		t.Fatalf("count failed events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 sync.failed event, got %d", count)
	}
}

// A fetch failure whose error chain embeds a credential-bearing URL (Go's
// *url.Error includes the full request URL — Steam carries the web_api_key in
// the query string) must not leak the query into jobs.error_message or the
// sync.failed event payload (#937).
func TestFailSyncJobScrubsCredentialURLQuery(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	jobID := uuid.NewString()

	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user_sync_config: %v", err)
	}

	fetchErr := fmt.Errorf("get owned games: %w", &url.Error{
		Op:  "Get",
		URL: "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=SECRET&steamid=123",
		Err: errors.New("connection refused"),
	})
	w := &tasks.DispatchSyncWorker{
		DB:      testDB,
		Adapter: fetchErrFactory(fetchErr),
		LC:      newTestLC(t),
	}
	_ = w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"}))
	runSyncEffects(t, w.LC)

	var errMsg string
	if err := testDB.NewRaw(`SELECT error_message FROM jobs WHERE id = ?`, jobID).Scan(ctx, &errMsg); err != nil {
		t.Fatalf("read jobs.error_message: %v", err)
	}
	if strings.Contains(errMsg, "key=SECRET") {
		t.Errorf("jobs.error_message still contains credential query: %q", errMsg)
	}
	if !strings.Contains(errMsg, "api.steampowered.com/IPlayerService/GetOwnedGames/v0001/") {
		t.Errorf("jobs.error_message lost the URL host/path: %q", errMsg)
	}

	var payload string
	if err := testDB.NewRaw(`SELECT payload::text FROM events WHERE dedup_key = ?`, jobID+":sync.failed").Scan(ctx, &payload); err != nil {
		t.Fatalf("read sync.failed payload: %v", err)
	}
	if strings.Contains(payload, "key=SECRET") {
		t.Errorf("sync.failed payload still contains credential query: %q", payload)
	}
}

func TestEmitsSyncDiffWhenChangesExist(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	lc := newTestLC(t)
	userID, jobID, items := syncCompletionFixture(t, "steam", true, 1)

	// Insert two changes rows: one added (with platform rows), one removed.
	// The added game has explicit platform rows so we can assert they appear in the payload.
	addedEgID := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available)
		 VALUES (?, ?, 'steam', '12345', 'Hades', false, true)`,
		addedEgID, userID,
	); err != nil {
		t.Fatalf("insert external_game for added: %v", err)
	}
	for _, plat := range []string{"mac", "pc-windows"} {
		if _, err := testDB.ExecContext(ctx,
			`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
			 VALUES (?, ?, ?, 0, now())`,
			uuid.NewString(), addedEgID, plat,
		); err != nil {
			t.Fatalf("insert external_game_platform %s: %v", plat, err)
		}
	}
	egID := insertTestExternalGame(t, userID, "steam", "999", "Old Game", "pc-windows")
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, ?, 'added', 'Hades', now())`,
		uuid.NewString(), jobID, userID, addedEgID,
	); err != nil {
		t.Fatalf("insert change added: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, ?, 'removed', 'Old Game', now())`,
		uuid.NewString(), jobID, userID, egID,
	); err != nil {
		t.Fatalf("insert change removed: %v", err)
	}

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	runSyncEffects(t, lc)

	// sync.diff event must have been emitted.
	var diffCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.diff").Scan(ctx, &diffCount); err != nil {
		t.Fatalf("count diff events: %v", err)
	}
	if diffCount != 1 {
		t.Fatalf("expected 1 sync.diff event, got %d", diffCount)
	}

	// Payload must mention both game titles and the platforms for the added game.
	var payloadText string
	if err := testDB.NewRaw(`SELECT payload::text FROM events WHERE dedup_key = ?`, jobID+":sync.diff").Scan(ctx, &payloadText); err != nil {
		t.Fatalf("scan diff payload: %v", err)
	}
	if !strings.Contains(payloadText, "Hades") {
		t.Fatalf("expected diff payload to contain 'Hades', got: %s", payloadText)
	}
	if !strings.Contains(payloadText, "Old Game") {
		t.Fatalf("expected diff payload to contain 'Old Game', got: %s", payloadText)
	}
	if !strings.Contains(payloadText, "pc-windows") {
		t.Fatalf("expected diff payload to contain 'pc-windows', got: %s", payloadText)
	}
	if !strings.Contains(payloadText, "mac") {
		t.Fatalf("expected diff payload to contain 'mac', got: %s", payloadText)
	}
}

func TestNoSyncDiffWhenNoChanges(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	lc := newTestLC(t)
	_, jobID, items := syncCompletionFixture(t, "steam", true, 1)
	// No changes rows inserted.

	endSyncItem(t, lc, items[0], models.JobItemStatusCompleted)
	runSyncEffects(t, lc)

	// No sync.diff event should have been emitted.
	var diffCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.diff").Scan(ctx, &diffCount); err != nil {
		t.Fatalf("count diff events: %v", err)
	}
	if diffCount != 0 {
		t.Fatalf("expected no sync.diff event when no changes, got %d", diffCount)
	}
}

// ─── sync.auth_expired notification (#751) ──────────────────────────────────

// seedSubscription subscribes a user to one event type.
func seedSubscription(t *testing.T, userID, eventType string) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO notification_subscriptions (user_id, event_type, created_at) VALUES (?, ?, now())`,
		userID, eventType,
	); err != nil {
		t.Fatalf("seedSubscription: %v", err)
	}
}

// insertCredErrJobAndConfig sets up a user, a processing sync job, and a steam
// sync config with the given prior credentials_error state.
func insertCredErrJobAndConfig(t *testing.T, userID, jobID string, priorErr bool) {
	t.Helper()
	ctx := context.Background()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0)`,
		jobID, userID,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, priorErr,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user_sync_config: %v", err)
	}
}

func eventCount(t *testing.T, dedupKey string) int {
	t.Helper()
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, dedupKey).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count events %q: %v", dedupKey, err)
	}
	return count
}

// On the healthy→expired transition, a user subscribed to sync.auth_expired
// gets that actionable event and NOT the generic sync.failed.
func TestDispatchSync_CredentialsError_Transition_Subscribed_EmitsAuthExpired(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertCredErrJobAndConfig(t, userID, jobID, false)
	seedSubscription(t, userID, "sync.auth_expired")

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runSyncEffects(t, w.LC)

	if got := eventCount(t, jobID+":sync.auth_expired"); got != 1 {
		t.Errorf("expected 1 sync.auth_expired event, got %d", got)
	}
	if got := eventCount(t, jobID+":sync.failed"); got != 0 {
		t.Errorf("expected 0 sync.failed events (auth event replaces it), got %d", got)
	}
}

// On the transition, a user NOT subscribed to sync.auth_expired still hears
// about it via the fallback sync.failed event.
func TestDispatchSync_CredentialsError_Transition_Unsubscribed_EmitsSyncFailed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertCredErrJobAndConfig(t, userID, jobID, false)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runSyncEffects(t, w.LC)

	if got := eventCount(t, jobID+":sync.failed"); got != 1 {
		t.Errorf("expected 1 fallback sync.failed event, got %d", got)
	}
	if got := eventCount(t, jobID+":sync.auth_expired"); got != 0 {
		t.Errorf("expected 0 sync.auth_expired events for unsubscribed user, got %d", got)
	}
}

// When the storefront is already in the credentials_error state, a repeat
// failure emits nothing — even for a subscribed user (notify only on transition).
func TestDispatchSync_CredentialsError_Repeat_EmitsNothing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	notify.SetRiverClient(nil)

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertCredErrJobAndConfig(t, userID, jobID, true)
	seedSubscription(t, userID, "sync.auth_expired")

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), LC: newTestLC(t)}
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runSyncEffects(t, w.LC)

	if got := eventCount(t, jobID+":sync.auth_expired"); got != 0 {
		t.Errorf("expected 0 sync.auth_expired events on repeat, got %d", got)
	}
	if got := eventCount(t, jobID+":sync.failed"); got != 0 {
		t.Errorf("expected 0 sync.failed events on repeat, got %d", got)
	}
}

// TestDispatchSync_PersistsSourceMetadata verifies that when an
// ExternalGameEntry carries SourceMetadata (e.g. Epic's namespace), the
// key/value pair is written to external_games.source_metadata as JSONB.
func TestDispatchSync_PersistsSourceMetadata(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	jobID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'epic-games-store', 'pending', 'low', 0)`,
		jobID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'epic-games-store', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{{
			ExternalID:      "abc123",
			Title:           "Some Epic Game",
			Platforms:       []string{"pc-windows"},
			OwnershipStatus: "owned",
			SourceMetadata:  map[string]string{"namespace": "ns-xyz"},
		}},
	}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), LC: newTestLC(t)}
	job := syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "epic-games-store"})

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var ns string
	if err := testDB.NewRaw(
		`SELECT source_metadata->>'namespace' FROM external_games WHERE user_id = ? AND external_id = 'abc123'`,
		userID,
	).Scan(ctx, &ns); err != nil {
		t.Fatalf("scan source_metadata->>'namespace': %v", err)
	}
	if ns != "ns-xyz" {
		t.Fatalf("namespace = %q, want %q", ns, "ns-xyz")
	}
}
