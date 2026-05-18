package tasks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		`INSERT INTO users (id, username, password_hash, is_active, is_admin, preferences, created_at, updated_at)
		 VALUES (?, 'admin', ?, true, true, '{}', now(), now())`, id, string(hash),
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

func TestMetadataRefreshDispatch_IGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	insertMetaRefreshAdminUser(t)

	unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: unconfigured, RiverClient: nil}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
	}
}

func TestMetadataRefreshDispatch_NoAdminUser(t *testing.T) {
	truncateAllTables(t)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
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

func TestMetadataRefreshDispatch_NoGames(t *testing.T) {
	truncateAllTables(t)
	insertMetaRefreshAdminUser(t)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	if err := w.Work(context.Background(), &river.Job[tasks.MetadataRefreshDispatchArgs]{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
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

func TestMetadataRefreshItem_IGDBError(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2002, "Some Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, adminID, 2002, "Some Game")

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	_ = w.Work(context.Background(), &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	})

	ctx := context.Background()

	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusFailed {
		t.Errorf("item status: want failed, got %s", item.Status)
	}

	var jobID string
	_ = testDB.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("job status: want completed, got %s", jobStatus)
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

// TestMetadataRefreshItem_IGDBNotConfiguredAtItemLevel exercises the per-item
// "igdb_not_configured" defensive guard.
func TestMetadataRefreshItem_IGDBNotConfiguredAtItemLevel(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	insertTestGame(t, 2010, "IGDB Guard Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, adminID, 2010, "IGDB Guard Game")

	// Use an unconfigured IGDB client — triggers the per-item guard.
	unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: unconfigured, StoragePath: t.TempDir()}
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
}

// TestMetadataRefreshItem_BadSourceMetadata exercises the "parse source_metadata" failure path.
func TestMetadataRefreshItem_BadSourceMetadata(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)

	ctx := context.Background()
	jobID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now(), now())`,
		jobID, adminID,
	).Exec(ctx)

	// Insert item with invalid source_metadata (not an object with game_id).
	itemID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, adminID, "bad-key", "Bad Meta", json.RawMessage(`"not-an-object"`),
	).Exec(ctx)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshItemWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshItemArgs]{
		Args: tasks.MetadataRefreshItemArgs{JobItemID: itemID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var item models.JobItem
	_ = testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusFailed {
		t.Errorf("item status: want failed, got %s", item.Status)
	}
}

// TestMetadataRefreshItem_GameNotFound exercises the "load game" failure path (game not in DB).
func TestMetadataRefreshItem_GameNotFound(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	// Do NOT insert game 9999 — so the SELECT will fail with "no rows".
	itemID := setupItemTest(t, adminID, 9999, "Ghost Game")

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

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
