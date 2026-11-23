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
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/notify"
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

			w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient}
			if err := w.Work(context.Background(), riverJob(1, 1, tasks.MetadataRefreshDispatchArgs{})); err != nil {
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

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient}
	if err := w.Work(ctx, riverJob(1, 1, tasks.MetadataRefreshDispatchArgs{})); err != nil {
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

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient}
	if err := w.Work(context.Background(), riverJob(1, 1, tasks.MetadataRefreshDispatchArgs{})); err != nil {
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
	var owned int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items ji JOIN river_job rj ON rj.id = ji.river_job_id WHERE ji.job_id = ?`, job.ID).Scan(ctx, &owned)
	if owned != 3 {
		t.Errorf("items owned by their river job: want 3, got %d", owned)
	}
	if n := countEvents(t, notify.TypeAdminMaintCompleted); n != 1 {
		t.Errorf("maint_completed events: want 1, got %d", n)
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
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient}

	const n = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			<-start // release together to maximise the self-create TOCTOU window
			if err := w.Work(context.Background(), riverJob(1, 1, tasks.MetadataRefreshDispatchArgs{})); err != nil {
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

// setupItemTest creates a metadata refresh Job with one item for gameID
// through the lifecycle module and returns the item id and its owning River
// job id.
func setupItemTest(t *testing.T, lc *joblifecycle.Module, adminID string, gameID int32, title string) (string, int64) {
	t.Helper()
	meta, _ := json.Marshal(map[string]any{"game_id": gameID})
	return setupItemWithMeta(t, lc, adminID, strconv.Itoa(int(gameID)), title, meta)
}

func setupItemWithMeta(t *testing.T, lc *joblifecycle.Module, adminID, key, title string, meta json.RawMessage) (string, int64) {
	t.Helper()
	itemID := uuid.NewString()
	if _, err := lc.CreateWithItems(context.Background(), joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow, TotalItems: 1,
	}, []joblifecycle.NewItem{{ID: itemID, UserID: adminID, ItemKey: key, SourceTitle: title, SourceMetadata: meta}}, nil); err != nil {
		t.Fatalf("create metadata refresh job: %v", err)
	}
	return itemID, itemOwner(t, itemID)
}

func TestMetadataRefreshItem_Success(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2001, "Old Title", time.Now().Add(-24*time.Hour))
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 2001, "Old Title")

	gamesJSON := `[{"id":2001,"name":"New Title","slug":"new-title","cover":{"image_id":"co9999"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
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
		setup func(t *testing.T, lc *joblifecycle.Module, adminID string) (string, int64, *igdbsvc.Client)
		// checkJobCompleted asserts the owning job finalized to completed.
		checkJobCompleted bool
	}{
		{
			// IGDB returns [] → no match for the looked-up game.
			name: "igdb_no_match",
			setup: func(t *testing.T, lc *joblifecycle.Module, adminID string) (string, int64, *igdbsvc.Client) {
				insertTestGame(t, 2002, "Some Game", time.Now().Add(-24*time.Hour))
				itemID, rid := setupItemTest(t, lc, adminID, 2002, "Some Game")
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, rid, newTestIGDBClient(t, srv)
			},
			checkJobCompleted: true,
		},
		{
			// Per-item "igdb_not_configured" defensive guard.
			name: "igdb_not_configured_at_item_level",
			setup: func(t *testing.T, lc *joblifecycle.Module, adminID string) (string, int64, *igdbsvc.Client) {
				insertTestGame(t, 2010, "IGDB Guard Game", time.Now().Add(-24*time.Hour))
				itemID, rid := setupItemTest(t, lc, adminID, 2010, "IGDB Guard Game")
				unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
				return itemID, rid, unconfigured
			},
		},
		{
			// source_metadata is not an object with game_id → parse failure.
			name: "bad_source_metadata",
			setup: func(t *testing.T, lc *joblifecycle.Module, adminID string) (string, int64, *igdbsvc.Client) {
				itemID, rid := setupItemWithMeta(t, lc, adminID, "bad-key", "Bad Meta", json.RawMessage(`"not-an-object"`))
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, rid, newTestIGDBClient(t, srv)
			},
		},
		{
			// Backing game row not inserted → "load game" no-rows failure.
			name: "game_not_found",
			setup: func(t *testing.T, lc *joblifecycle.Module, adminID string) (string, int64, *igdbsvc.Client) {
				itemID, rid := setupItemTest(t, lc, adminID, 9999, "Ghost Game")
				srv := igdbTestServer(t, `[]`)
				t.Cleanup(srv.Close)
				return itemID, rid, newTestIGDBClient(t, srv)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			adminID := insertMetaRefreshAdminUser(t)
			lc := newTestLC(t)
			itemID, rid, igdbClient := tc.setup(t, lc, adminID)

			// A first attempt: these failures do not go away by retrying.
			w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: t.TempDir()}
			if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
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
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 2003, "Cover Game")

	gamesJSON := `[{"id":2003,"name":"Cover Game","slug":"cover-game","cover":{"image_id":"co_fail"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: "/dev/null"}
	_ = w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID}))

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
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 2004, "Cover Unchanged")

	gamesJSON := `[{"id":2004,"name":"Cover Unchanged","slug":"cover-unchanged","cover":{"image_id":"co_same"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	_ = w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID}))

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

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(1, 1, tasks.MetadataRefreshItemArgs{JobItemID: "non-existent-item"})); err != nil {
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
	lc := newTestLC(t)
	// The admin handler creates the Job with its dispatch.
	c, err := lc.CreateWithDispatch(ctx, joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	jobID := c.JobID

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: lc, IGDBClient: igdbClient}
	rj := riverJob(dispatchOwner(t, jobID), 1, tasks.MetadataRefreshDispatchArgs{JobID: jobID})
	if err := w.Work(ctx, rj); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	// The dispatch run again (River retry after a crash past its commit) is
	// refused at Start dispatch and sends nothing more.
	if err := w.Work(ctx, rj); err != nil {
		t.Fatalf("re-run: expected nil, got %v", err)
	}
	if n := countEvents(t, notify.TypeAdminMaintCompleted); n != 1 {
		t.Errorf("maint_completed events: want 1 across a re-run dispatch, got %d", n)
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
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 2020, "Cover Update Game")

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

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
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
	lc := newTestLC(t)
	c, err := lc.CreateWithDispatch(ctx, joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	jobID := c.JobID

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: lc, IGDBClient: igdbClient}
	if err := w.Work(ctx, riverJob(dispatchOwner(t, jobID), 1, tasks.MetadataRefreshDispatchArgs{JobID: jobID})); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if n := countEvents(t, notify.TypeAdminMaintCompleted); n != 0 {
		t.Errorf("an empty refresh sent %d maint_completed events", n)
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

	lc := newTestLC(t)
	itemID1, itemID2 := uuid.NewString(), uuid.NewString()
	meta := func(id int32) json.RawMessage { b, _ := json.Marshal(map[string]any{"game_id": id}); return b }
	c, err := lc.CreateWithItems(ctx, joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow, TotalItems: 2,
	}, []joblifecycle.NewItem{
		{ID: itemID1, UserID: adminID, ItemKey: "3001", SourceTitle: "Game A", SourceMetadata: meta(3001)},
		{ID: itemID2, UserID: adminID, ItemKey: "3002", SourceTitle: "Game B", SourceMetadata: meta(3002)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jobID := c.JobID

	gamesResponse := func(id int, name string) string {
		return `[{"id":` + strconv.Itoa(id) + `,"name":"` + name + `","slug":"slug"}]`
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: igdbClient, StoragePath: t.TempDir()}

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
	_ = w.Work(ctx, riverJob(itemOwner(t, itemID1), 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID1}))

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
	_ = w.Work(ctx, riverJob(itemOwner(t, itemID2), 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID2}))

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
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: newTestLC(t), IGDBClient: igdbClient, MinAge: 23 * time.Hour}

	if err := w.Work(ctx, riverJob(1, 1, tasks.MetadataRefreshDispatchArgs{})); err != nil {
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
	if opts.Priority != 3 {
		t.Errorf("Priority = %d; want 3 (unchanged)", opts.Priority)
	}
}

// igdbServerWith serves the token endpoint and runs onGames for /games.
func igdbServerWith(t *testing.T, tokenStatus int, onGames func(w http.ResponseWriter)) *igdbsvc.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			if tokenStatus != http.StatusOK {
				w.WriteHeader(tokenStatus)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			onGames(w)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	return newTestIGDBClient(t, srv)
}

// A refresh an admin started while IGDB is not configured fails its Job at
// once, tells the admin once, and is not run again.
func TestMetadataRefreshDispatch_IGDBNotConfiguredFailsJob(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 4001, "G", time.Now().Add(-48*time.Hour))
	lc := newTestLC(t)
	c, err := lc.CreateWithDispatch(ctx, joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeMetadataRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, LC: lc, IGDBClient: igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))}
	rj := riverJob(dispatchOwner(t, c.JobID), 1, tasks.MetadataRefreshDispatchArgs{JobID: c.JobID})
	for range 2 {
		if err := w.Work(ctx, rj); err != nil {
			t.Fatalf("Work: %v", err)
		}
	}
	if s := jobStatus(t, c.JobID); s != models.JobStatusFailed {
		t.Fatalf("job = %s, want failed", s)
	}
	if n := countEvents(t, notify.TypeAdminMaintFailed); n != 1 {
		t.Errorf("maint_failed events = %d, want 1", n)
	}
	var items int
	_ = testDB.NewRaw(`SELECT count(*) FROM job_items WHERE job_id = ?`, c.JobID).Scan(ctx, &items)
	if items != 0 {
		t.Errorf("a failed dispatch created %d items", items)
	}
}

// IGDB being unreachable is retried; the last attempt fails the item with
// IGDB's message.
func TestMetadataRefreshItem_IGDBUnreachableRetriesThenFails(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 4002, "Flaky", time.Now().Add(-48*time.Hour))
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 4002, "Flaky")
	client := igdbServerWith(t, http.StatusOK, func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })
	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: client, StoragePath: t.TempDir()}

	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err == nil {
		t.Fatal("first attempt returned nil: River would not retry")
	}
	if s, _ := itemStatus(t, itemID); s != models.JobItemStatusPending {
		t.Fatalf("after a retryable error the item is %s, want pending", s)
	}
	if err := w.Work(context.Background(), riverJob(rid, 5, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("last attempt: %v", err)
	}
	s, msg := itemStatus(t, itemID)
	if s != models.JobItemStatusFailed || msg == nil || *msg == "" {
		t.Fatalf("after the last attempt: %s %v", s, msg)
	}
}

// Credentials IGDB rejects fail the item at once.
func TestMetadataRefreshItem_RejectedCredentialsFailAtOnce(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 4003, "Locked", time.Now().Add(-48*time.Hour))
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 4003, "Locked")
	client := igdbServerWith(t, http.StatusUnauthorized, func(w http.ResponseWriter) { _, _ = w.Write([]byte(`[]`)) })
	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: client, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if s, _ := itemStatus(t, itemID); s != models.JobItemStatusFailed {
		t.Fatalf("item = %s, want failed", s)
	}
}

// A game deleted while its metadata was being fetched (the daily cleanup of
// unreferenced games) ends the item failed, not completed.
func TestMetadataRefreshItem_GameDeletedMeanwhileFails(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 4004, "Doomed", time.Now().Add(-48*time.Hour))
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 4004, "Doomed")
	client := igdbServerWith(t, http.StatusOK, func(w http.ResponseWriter) {
		if _, err := testDB.ExecContext(context.Background(), `DELETE FROM games WHERE id = 4004`); err != nil {
			t.Errorf("delete game: %v", err)
		}
		_, _ = w.Write([]byte(`[{"id":4004,"name":"Doomed","slug":"doomed"}]`))
	})
	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: client, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if s, _ := itemStatus(t, itemID); s != models.JobItemStatusFailed {
		t.Fatalf("item = %s, want failed", s)
	}
}

// A refresh cancelled while an item's metadata is being fetched updates no
// game afterwards.
func TestMetadataRefreshItem_CancelledRefreshUpdatesNoGame(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 4005, "Original", time.Now().Add(-48*time.Hour))
	lc := newTestLC(t)
	itemID, rid := setupItemTest(t, lc, adminID, 4005, "Original")
	var jobID string
	_ = testDB.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &jobID)
	client := igdbServerWith(t, http.StatusOK, func(w http.ResponseWriter) {
		if err := lc.Cancel(context.Background(), jobID); err != nil {
			t.Errorf("cancel: %v", err)
		}
		_, _ = w.Write([]byte(`[{"id":4005,"name":"Renamed","slug":"renamed"}]`))
	})
	w := &tasks.MetadataRefreshItemWorker{DB: testDB, LC: lc, IGDBClient: client, StoragePath: t.TempDir()}
	if err := w.Work(context.Background(), riverJob(rid, 1, tasks.MetadataRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("Work: %v", err)
	}
	var title string
	_ = testDB.NewRaw(`SELECT title FROM games WHERE id = 4005`).Scan(context.Background(), &title)
	if title != "Original" {
		t.Errorf("a cancelled refresh updated the game: title %q", title)
	}
	if s, _ := itemStatus(t, itemID); s != models.JobItemStatusCancelled {
		t.Errorf("item = %s, want cancelled", s)
	}
}
