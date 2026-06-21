package tasks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/ratelimit"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// ─── DB helpers ──────────────────────────────────────────────────────────────

func insertMetaRefreshAdminUser(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 4)
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_active, is_admin, created_at, updated_at)
		 VALUES (?, 'admin', ?, true, true, now(), now())`, id, string(hash),
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert admin user: %v", err)
	}
	return id
}

func insertTestGame(t *testing.T, igdbID int32, title string, lastUpdated time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, ?, now())`,
		igdbID, title, lastUpdated,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert game %d: %v", igdbID, err)
	}
}

func igdbTestServer(t *testing.T, gamesResponse string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = w.Write([]byte(gamesResponse))
		case "/game_time_to_beats":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestIGDBClient(t *testing.T, srv *httptest.Server) *igdbsvc.Client {
	t.Helper()
	cfg := &config.Config{
		IGDBClientID:          "test-client",
		IGDBClientSecret:      "test-secret",
		IGDBRequestsPerSecond: 100,
		IGDBBurstCapacity:     100,
	}
	client := igdbsvc.NewClientWithTokenURL(cfg, srv.URL+"/oauth2/token", ratelimit.NewLocal(100, 100))
	client.SetAPIURLForTest(srv.URL)
	return client
}

// newTestMetadataRiverClient creates a non-started River client backed by the
// shared test container, suitable for tests that call Insert without a running
// worker loop.
func newTestMetadataRiverClient(t *testing.T) *river.Client[pgx.Tx] {
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

// ─── Dispatch tests ───────────────────────────────────────────────────────────

// TestMetadataRefreshDispatch_PreconditionMissing consolidates the dispatch
// guard tests that each leave 0 metadata_refresh jobs when a precondition is
// missing: IGDB not configured, no admin user, or no stale games to refresh.
func TestMetadataRefreshDispatch_PreconditionMissing(t *testing.T) {
	tests := []struct {
		name string
		// setup configures the precondition and returns the IGDB client to use.
		setup func(t *testing.T) *igdbsvc.Client
	}{
		{
			name: "igdb_not_configured",
			setup: func(t *testing.T) *igdbsvc.Client {
				insertMetaRefreshAdminUser(t)
				return igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
			},
		},
		{
			name: "no_admin_user",
			setup: func(t *testing.T) *igdbsvc.Client {
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return newTestIGDBClient(t, srv)
			},
		},
		{
			name: "no_games",
			setup: func(t *testing.T) *igdbsvc.Client {
				insertMetaRefreshAdminUser(t)
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return newTestIGDBClient(t, srv)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			igdbClient := tc.setup(t)

			w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
			if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
				t.Fatalf("expected nil, got %v", err)
			}

			var count int
			_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
			if count != 0 {
				t.Errorf("expected 0 jobs, got %d", count)
			}
		})
	}
}

func TestMetadataRefreshDispatch_AlreadyRunning(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 1001, "Game One", time.Now().Add(-24*time.Hour))

	ctx := context.Background()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now())`,
		uuid.NewString(), adminID,
	).Exec(ctx)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("expected 1 job (the existing one), got %d", count)
	}
}

func TestMetadataRefreshDispatch_CreatesJobAndItems(t *testing.T) {
	truncateAllTables(t)
	insertMetaRefreshAdminUser(t)
	now := time.Now().UTC()
	insertTestGame(t, 1001, "Game One", now.Add(-72*time.Hour))
	insertTestGame(t, 1002, "Game Two", now.Add(-48*time.Hour))
	insertTestGame(t, 1003, "Game Three", now.Add(-24*time.Hour))

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	rc := newTestMetadataRiverClient(t)
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()

	var job models.Job
	if err := testDB.NewSelect().Model(&job).
		Where("job_type = ?", models.JobTypeMetadataRefresh).
		Scan(ctx); err != nil {
		t.Fatalf("no job found: %v", err)
	}
	if job.Status != models.JobStatusProcessing {
		t.Errorf("job status: want processing, got %s", job.Status)
	}
	if job.TotalItems != 3 {
		t.Errorf("total_items: want 3, got %d", job.TotalItems)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, job.ID).Scan(ctx, &itemCount)
	if itemCount != 3 {
		t.Errorf("job_items: want 3, got %d", itemCount)
	}

	var riverJobCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind = 'metadata_refresh_item'`).Scan(ctx, &riverJobCount)
	if riverJobCount != 3 {
		t.Errorf("river_job (metadata_refresh_item): want 3, got %d", riverJobCount)
	}
}

func TestMetadataRefreshDispatch_ConcurrentSelfCreateCreatesOneJob(t *testing.T) {
	truncateAllTables(t)
	insertMetaRefreshAdminUser(t)
	now := time.Now().UTC()
	insertTestGame(t, 2001, "Race Game One", now.Add(-72*time.Hour))
	insertTestGame(t, 2002, "Race Game Two", now.Add(-48*time.Hour))

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestMetadataRiverClient(t)
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}

	const n = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			<-start // release together to maximise the self-create TOCTOU window
			if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
				t.Errorf("Work: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh' AND status IN ('pending','processing')`,
	).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 active metadata_refresh job after %d concurrent self-create dispatches, got %d", n, count)
	}
}

// ─── Item tests ───────────────────────────────────────────────────────────────

func setupItemTest(t *testing.T, adminID string, gameID int32, title string) string {
	t.Helper()
	ctx := context.Background()

	jobID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now(), now())`,
		jobID, adminID,
	).Exec(ctx)

	itemID := uuid.NewString()
	sourceMetaBytes, _ := json.Marshal(map[string]any{"game_id": gameID})
	sourceMeta := json.RawMessage(sourceMetaBytes)
	_, _ = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, adminID, strconv.Itoa(int(gameID)), title, sourceMeta,
	).Exec(ctx)

	return itemID
}

func TestMetadataRefreshItem_Success(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2001, "Old Title", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, adminID, 2001, "Old Title")

	gamesJSON := `[{"id":2001,"name":"New Title","slug":"new-title","cover":{"image_id":"co9999"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()

	var title string
	_ = testDB.NewRaw(`SELECT title FROM games WHERE id = 2001`).Scan(ctx, &title)
	if title != "New Title" {
		t.Errorf("game title: want 'New Title', got %q", title)
	}

	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}

	var jobID string
	_ = testDB.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("job status: want completed, got %s", jobStatus)
	}
}

// TestMetadataRefreshItem_Failures consolidates the per-item failure paths that
// must mark the job_item failed: the IGDB lookup returning no match, the
// per-item igdb_not_configured guard, unparseable source_metadata, and the
// backing game row not existing. The igdb_match row additionally asserts the
// owning job finalizes to completed once its sole item settles.
func TestMetadataRefreshItem_Failures(t *testing.T) {
	tests := []struct {
		name string
		// setup configures fixtures and returns (itemID, IGDB client).
		setup func(t *testing.T, adminID string) (string, *igdbsvc.Client)
		// checkJobCompleted asserts the owning job finalized to completed.
		checkJobCompleted bool
	}{
		{
			// IGDB returns [] → no match for the looked-up game.
			name: "igdb_no_match",
			setup: func(t *testing.T, adminID string) (string, *igdbsvc.Client) {
				insertTestGame(t, 2002, "Some Game", time.Now().Add(-24*time.Hour))
				itemID := setupItemTest(t, adminID, 2002, "Some Game")
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, newTestIGDBClient(t, srv)
			},
			checkJobCompleted: true,
		},
		{
			// Per-item "igdb_not_configured" defensive guard.
			name: "igdb_not_configured_at_item_level",
			setup: func(t *testing.T, adminID string) (string, *igdbsvc.Client) {
				insertTestGame(t, 2010, "IGDB Guard Game", time.Now().Add(-24*time.Hour))
				itemID := setupItemTest(t, adminID, 2010, "IGDB Guard Game")
				unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
				return itemID, unconfigured
			},
		},
		{
			// source_metadata is not an object with game_id → parse failure.
			name: "bad_source_metadata",
			setup: func(t *testing.T, adminID string) (string, *igdbsvc.Client) {
				ctx := context.Background()
				jobID := uuid.NewString()
				_, _ = testDB.NewRaw(
					`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
					 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now(), now())`,
					jobID, adminID,
				).Exec(ctx)
				itemID := uuid.NewString()
				_, _ = testDB.NewRaw(
					`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
					 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
					itemID, jobID, adminID, "bad-key", "Bad Meta", json.RawMessage(`"not-an-object"`),
				).Exec(ctx)
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, newTestIGDBClient(t, srv)
			},
		},
		{
			// Backing game row not inserted → "load game" no-rows failure.
			name: "game_not_found",
			setup: func(t *testing.T, adminID string) (string, *igdbsvc.Client) {
				itemID := setupItemTest(t, adminID, 9999, "Ghost Game")
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, newTestIGDBClient(t, srv)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			adminID := insertMetaRefreshAdminUser(t)
			itemID, igdbClient := tc.setup(t, adminID)

			w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
			if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
				Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
			}); err != nil {
				t.Fatalf("expected nil, got %v", err)
			}

			ctx := context.Background()

			var item models.JobItem
			_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
			if item.Status != models.JobItemStatusFailed {
				t.Errorf("item status: want failed, got %s", item.Status)
			}

			if tc.checkJobCompleted {
				var jobID string
				_ = testDB.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
				var jobStatus string
				_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
				if jobStatus != models.JobStatusCompleted {
					t.Errorf("job status: want completed, got %s", jobStatus)
				}
			}
		})
	}
}

func TestMetadataRefreshItem_CoverArtFailureNonFatal(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2003, "Cover Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, adminID, 2003, "Cover Game")

	gamesJSON := `[{"id":2003,"name":"Cover Game","slug":"cover-game","cover":{"image_id":"co_fail"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: "/dev/null"}
	_ = w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	})

	ctx := context.Background()
	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}
}

func TestMetadataRefreshItem_CoverArtUnchanged(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)

	ctx := context.Background()
	_, _ = testDB.NewRaw(
		`INSERT INTO games (id, title, cover_art_url, last_updated, created_at)
		 VALUES (2004, 'Cover Unchanged', '/static/cover_art/co_same.jpg', now() - interval '1 day', now())`,
	).Exec(ctx)
	itemID := setupItemTest(t, adminID, 2004, "Cover Unchanged")

	gamesJSON := `[{"id":2004,"name":"Cover Unchanged","slug":"cover-unchanged","cover":{"image_id":"co_same"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	_ = w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	})

	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}
}

// TestMetadataRefreshItem_JobItemNotFound exercises the "load job_item not found" path.
func TestMetadataRefreshItem_JobItemNotFound(t *testing.T) {
	truncateAllTables(t)
	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: "non-existent-item"},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestMetadataRefreshDispatch_HandlerOwned_PopulatesExistingRow(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	now := time.Now().UTC()
	insertTestGame(t, 1001, "Game One", now.Add(-48*time.Hour))
	insertTestGame(t, 1002, "Game Two", now.Add(-24*time.Hour))

	ctx := context.Background()
	// Simulate the handler having created a pending row up front.
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestMetadataRiverClient(t)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{
		Args: tasks.MetadataRefreshDispatchArgs{JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Exactly one job row (no duplicate), flipped to processing with total_items set.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Fatalf("expected 1 job row, got %d", count)
	}
	var status string
	var total int
	_ = testDB.NewRaw(`SELECT status, total_items FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status, &total)
	if status != "processing" {
		t.Errorf("status: want processing, got %s", status)
	}
	if total != 2 {
		t.Errorf("total_items: want 2, got %d", total)
	}
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 2 {
		t.Errorf("job_items: want 2, got %d", itemCount)
	}
}

// TestMetadataRefreshItem_CoverArtUpdated exercises the cover-art update branch
// when the image ID exists and the stored URL differs from what IGDB would produce.
func TestMetadataRefreshItem_CoverArtUpdated(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2020, "Cover Update Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, adminID, 2020, "Cover Update Game")

	// Serve a cover image that will be downloadable.
	imgData := []byte{0xFF, 0xD8, 0xFF} // minimal JPEG header bytes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = w.Write([]byte(`[{"id":2020,"name":"Cover Update Game","slug":"cover-update","cover":{"image_id":"co_new"}}]`))
		case "/game_time_to_beats":
			_, _ = w.Write([]byte(`[]`))
		default:
			// Serve the cover image download.
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imgData)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		IGDBClientID:          "test-client",
		IGDBClientSecret:      "test-secret",
		IGDBRequestsPerSecond: 100,
		IGDBBurstCapacity:     100,
	}
	igdbClient := igdbsvc.NewClientWithTokenURL(cfg, srv.URL+"/oauth2/token", ratelimit.NewLocal(100, 100))
	igdbClient.SetAPIURLForTest(srv.URL)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()
	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}
}

func TestMetadataRefreshDispatch_HandlerOwned_EmptyFinalizesCompleted(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	// No games inserted.

	ctx := context.Background()
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{
		Args: tasks.MetadataRefreshDispatchArgs{JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("status: want completed, got %s", status)
	}
}

func TestMetadataRefreshItem_JobCompletionPartial(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 3001, "Game A", time.Now().Add(-48*time.Hour))
	insertTestGame(t, 3002, "Game B", time.Now().Add(-24*time.Hour))

	ctx := context.Background()

	jobID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 2, now(), now())`,
		jobID, adminID,
	).Exec(ctx)

	makeItem := func(gameID int32, title string) string {
		itemID := uuid.NewString()
		sourceMetaBytes, _ := json.Marshal(map[string]any{"game_id": gameID})
		sourceMeta := json.RawMessage(sourceMetaBytes)
		_, _ = testDB.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, adminID, strconv.Itoa(int(gameID)), title, sourceMeta,
		).Exec(ctx)
		return itemID
	}

	itemID1 := makeItem(3001, "Game A")
	itemID2 := makeItem(3002, "Game B")

	gamesResponse := func(id int, name string) string {
		return `[{"id":` + strconv.Itoa(id) + `,"name":"` + name + `","slug":"slug"}]`
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}

	// Process first item — job should still be processing.
	srv.Config.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = rw.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = rw.Write([]byte(gamesResponse(3001, "Game A")))
		default:
			_, _ = rw.Write([]byte(`[]`))
		}
	})
	_ = w.Work(ctx, &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID1},
	})

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusProcessing {
		t.Errorf("after first item: job status want processing, got %s", jobStatus)
	}

	// Process second item — job should now be completed.
	srv.Config.Handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = rw.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = rw.Write([]byte(gamesResponse(3002, "Game B")))
		default:
			_, _ = rw.Write([]byte(`[]`))
		}
	})
	_ = w.Work(ctx, &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID2},
	})

	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("after second item: job status want completed, got %s", jobStatus)
	}
}

func TestMetadataRefreshDispatch_StalenessFilter(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Admin owner required by the self-created dispatch path.
	insertMetaRefreshAdminUser(t)

	now := time.Now().UTC()
	insertTestGame(t, 101, "Fresh", now.Add(-1*time.Hour))      // within 23h → excluded
	insertTestGame(t, 102, "Stale", now.Add(-48*time.Hour))     // older than 23h → included
	insertTestGame(t, 103, "Ancient", now.Add(-1000*time.Hour)) // very old (never refreshed) → included

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestMetadataRiverClient(t)
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc, MinAge: 23 * time.Hour}

	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{Args: tasks.MetadataRefreshDispatchArgs{}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var keys []string
	if err := testDB.NewRaw(
		`SELECT item_key FROM job_items ORDER BY item_key`,
	).Scan(ctx, &keys); err != nil {
		t.Fatalf("scan job_items: %v", err)
	}
	// 102 (stale) and 103 (ancient) enqueued; 101 (fresh) excluded.
	want := []string{"102", "103"}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Errorf("enqueued item_keys = %v; want %v", keys, want)
	}
}

func TestMetadataRefreshItemArgs_InsertOptsQueue(t *testing.T) {
	opts := tasks.MetadataRefreshItemArgs{}.InsertOpts()
	if opts.Queue != tasks.QueueMetadataRefresh {
		t.Errorf("Queue = %q; want %q", opts.Queue, tasks.QueueMetadataRefresh)
	}
	if tasks.QueueMetadataRefresh != "metadata_refresh" {
		t.Errorf("QueueMetadataRefresh = %q; want \"metadata_refresh\"", tasks.QueueMetadataRefresh)
	}
	if opts.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d; want 5 (unchanged)", opts.MaxAttempts)
	}
}
