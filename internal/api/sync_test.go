package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/auth"
)

type stubSteamClient struct {
	summary *api.SteamPlayerSummary
	err     error
}

func (s *stubSteamClient) GetPlayerSummaries(_ context.Context, _, _ string) (*api.SteamPlayerSummary, error) {
	return s.summary, s.err
}

type stubPlaystationStoreClient struct {
	info *api.PlaystationStoreAccountInfo
	err  error
}

func (s *stubPlaystationStoreClient) GetAccountInfo(_ context.Context, _ string) (*api.PlaystationStoreAccountInfo, error) {
	return s.info, s.err
}

func newSyncTestApp(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PlaystationStoreClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, newTestRiverClient(t), steam, psn, (api.EpicGamesStoreClient)(nil), (api.GOGClient)(nil), (api.HumbleClient)(nil))
	g := e.Group("/api/sync", auth.AuthMiddleware(db))
	synch.RegisterRoutes(g)
	return e
}

// ─── Sync config tests ────────────────────────────────────────────────────────

func TestSyncConfig_ListDefaults(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-list-1")

	rec := getAuth(t, e, "/api/sync/config", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 5 {
		t.Fatalf("expected total=5, got %v", resp["total"])
	}
	configs := resp["configs"].([]any)
	if len(configs) != 5 {
		t.Fatalf("expected 5 configs, got %d", len(configs))
	}
	for _, c := range configs {
		cfg := c.(map[string]any)
		if cfg["is_configured"].(bool) {
			t.Fatalf("expected is_configured=false for virtual default, storefront=%v", cfg["storefront"])
		}
	}
}

func TestSyncConfig_Put_CreatesRow(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-put-1")

	rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
		"frequency": "daily",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["frequency"] != "daily" {
		t.Fatalf("expected frequency=daily, got %v", resp["frequency"])
	}
}

func TestSyncConfig_Put_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-invalid-1")

	rec := putJSONAuth(t, e, "/api/sync/config/battlenet", map[string]any{"frequency": "daily"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSyncConfig_Put_EpicGamesStoreAllowed(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-epic-1")

	rec := putJSONAuth(t, e, "/api/sync/config/epic-games-store", map[string]any{"frequency": "weekly"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for epic-games-store config, got %d", rec.Code)
	}
}

// ─── Sync trigger and status tests ───────────────────────────────────────────

// TestSyncTrigger_CreatesJob verifies that triggering a sync for each supported
// storefront returns 200 with status=queued and a job_id, echoing the storefront.
func TestSyncTrigger_CreatesJob(t *testing.T) {
	tests := []struct {
		name       string
		suffix     string
		storefront string
	}{
		{name: "steam", suffix: "trig-1", storefront: "steam"},
		{name: "epic-games-store", suffix: "trig-epic-1", storefront: "epic-games-store"},
		{name: "playstation-store", suffix: "trig-pss-1", storefront: "playstation-store"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			_, token := setupTagUser(t, testDB, e, tt.suffix)

			rec := postJSONAuth(t, e, "/api/sync/"+tt.storefront, nil, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s trigger, got %d: %s", tt.storefront, rec.Code, rec.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp["storefront"] != tt.storefront {
				t.Fatalf("expected storefront=%s, got %v", tt.storefront, resp["storefront"])
			}
			if resp["status"] != "queued" {
				t.Fatalf("expected status=queued, got %v", resp["status"])
			}
			if resp["job_id"] == nil {
				t.Fatal("expected job_id")
			}
		})
	}
}

func TestSyncTrigger_DuplicateReturns409(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "trig-dup-1")

	postJSONAuth(t, e, "/api/sync/steam", nil, token)
	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate, got %d", rec.Code)
	}
}

func TestSyncTrigger_ConcurrentCreatesOneJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "trig-race-1")

	const n = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			<-start // release together to maximise the TOCTOU race window
			postJSONAuth(t, e, "/api/sync/steam", nil, token)
		}()
	}
	close(start)
	wg.Wait()

	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE job_type = 'sync' AND source = 'steam' AND status IN ('pending','processing')`,
	).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 active steam sync job after %d concurrent triggers, got %d", n, count)
	}
}

func TestSyncTrigger_RejectsOldSlugs(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "trig-oldslug-1")

	// Clean cutover: the pre-rename slugs are no longer valid storefronts.
	for _, sf := range []string{"psn", "epic"} {
		rec := postJSONAuth(t, e, "/api/sync/"+sf, nil, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("old slug %q: got %d, want 400", sf, rec.Code)
		}
	}
}

func TestSyncStatus_ReflectsActiveJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "stat-1")

	rec := getAuth(t, e, "/api/sync/steam/status", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status["is_syncing"].(bool) {
		t.Fatal("expected is_syncing=false before trigger")
	}
	if count, ok := status["external_game_count"].(float64); !ok || count != 0 {
		t.Fatalf("expected external_game_count=0, got %v", status["external_game_count"])
	}

	postJSONAuth(t, e, "/api/sync/steam", nil, token)

	rec2 := getAuth(t, e, "/api/sync/steam/status", token)
	if err := json.Unmarshal(rec2.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal status2: %v", err)
	}
	if !status["is_syncing"].(bool) {
		t.Fatal("expected is_syncing=true after trigger")
	}
	if status["active_job_id"] == nil {
		t.Fatal("expected active_job_id to be set")
	}
}

func TestSteamConnect_BadSteamID(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "sv-bad-id")

	rec := putJSONAuth(t, e, "/api/sync/steam/connection", map[string]any{
		"steam_id":    "12345",
		"web_api_key": "AABBCCDD00112233445566778899AABB",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp["valid"].(bool) {
		t.Fatal("expected valid=false for bad steam_id")
	}
	if resp["error"] != "invalid_steam_id" {
		t.Fatalf("expected error=invalid_steam_id, got %v", resp["error"])
	}
}

func TestSteamConnect_BadAPIKey(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "sv-bad-key")

	rec := putJSONAuth(t, e, "/api/sync/steam/connection", map[string]any{
		"steam_id":    "76561198012345678",
		"web_api_key": "tooshort",
	}, token)
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp["valid"].(bool) {
		t.Fatal("expected valid=false for bad api key")
	}
	if resp["error"] != "invalid_api_key" {
		t.Fatalf("expected error=invalid_api_key, got %v", resp["error"])
	}
}

func TestSteamConnect_StubSuccess(t *testing.T) {
	truncateAllTables(t)
	stub := &stubSteamClient{
		summary: &api.SteamPlayerSummary{PersonaName: "Frostbyte", CommunityVisibilityState: 3},
	}
	e := newSyncTestApp(t, testDB, stub, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "sv-ok")

	rec := putJSONAuth(t, e, "/api/sync/steam/connection", map[string]any{
		"steam_id":    "76561198012345678",
		"web_api_key": "AABBCCDD00112233445566778899AABB",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if !resp["valid"].(bool) {
		t.Fatalf("expected valid=true, got error=%v", resp["error"])
	}
	if resp["steam_username"] != "Frostbyte" {
		t.Fatalf("expected steam_username=Frostbyte, got %v", resp["steam_username"])
	}

	var creds string
	err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`, userID).
		Scan(context.Background(), &creds)
	if err != nil {
		t.Fatalf("credentials not stored: %v", err)
	}
	if creds == "" {
		t.Fatal("expected non-empty credentials")
	}
	// verify stored value is ciphertext, not plaintext
	if strings.Contains(creds, "web_api_key") {
		t.Fatal("stored credentials must be ciphertext, not plaintext")
	}
	if !strings.HasPrefix(creds, "enc:v1:") {
		t.Fatalf("expected enc:v1: prefix, got %q", creds[:min(20, len(creds))])
	}
}

func TestSteamDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "sd-1")

	rec := deleteAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 even with no row, got %d", rec.Code)
	}
}

// --- PSN tests ---------------------------------------------------------------

func TestPlaystationStoreConnect_ShortToken(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "psn-short")

	rec := putJSONAuth(t, e, "/api/sync/playstation-store/connection", map[string]any{
		"npsso_token": "tooshort",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPlaystationStoreConnect_StubSuccess(t *testing.T) {
	truncateAllTables(t)
	stub := &stubPlaystationStoreClient{
		info: &api.PlaystationStoreAccountInfo{OnlineID: "MyPSNName", AccountID: "123456"},
	}
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "psn-ok")

	token64 := strings.Repeat("a", 64)
	rec := putJSONAuth(t, e, "/api/sync/playstation-store/connection", map[string]any{
		"npsso_token": token64,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if !resp["success"].(bool) {
		t.Fatal("expected success=true")
	}
	if resp["online_id"] != "MyPSNName" {
		t.Fatalf("expected online_id=MyPSNName, got %v", resp["online_id"])
	}

	var creds string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'playstation-store'`, userID).
		Scan(context.Background(), &creds); err != nil {
		t.Fatalf("scan credentials: %v", err)
	}
	if creds == "" {
		t.Fatal("expected credentials stored")
	}
}

func TestPlaystationStoreStatus_NoRow(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "psn-stat-empty")

	rec := getAuth(t, e, "/api/sync/playstation-store/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp["is_configured"].(bool) {
		t.Fatal("expected is_configured=false")
	}
	if resp["credentials_error"] != nil {
		t.Fatalf("expected credentials_error absent/null, got %v", resp["credentials_error"])
	}
	if resp["online_id"] != nil {
		t.Fatalf("expected online_id absent/null, got %v", resp["online_id"])
	}
}

func TestPlaystationStoreDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "psn-disc-1")

	rec := deleteAuth(t, e, "/api/sync/playstation-store/connection", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// ─── Ignored / skip / unskip tests ───────────────────────────────────────────

func insertExternalGame(t *testing.T, db *bun.DB, id, userID, storefront, extID, title string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, now(), now())`,
		id, userID, storefront, extID, title,
	)
	if err != nil {
		t.Fatalf("insertExternalGame: %v", err)
	}
}

// insertChildExternalGame inserts an external_game row with parent_id set.
func insertChildExternalGame(t *testing.T, db *bun.DB, id, userID, storefront, extID, title, parentID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, ?, now(), now())`,
		id, userID, storefront, extID, title, parentID,
	)
	if err != nil {
		t.Fatalf("insertChildExternalGame: %v", err)
	}
}

// insertExternalGamePlatform inserts a platform row for an external_game.
func insertExternalGamePlatform(t *testing.T, db *bun.DB, egID, platform string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (gen_random_uuid()::text, ?, ?, 0, now())`,
		egID, platform,
	)
	if err != nil {
		t.Fatalf("insertExternalGamePlatform: %v", err)
	}
}

func TestIgnored_EmptyList(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "ign-empty")

	rec := getAuth(t, e, "/api/sync/ignored", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp []any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %v", resp)
	}
}

func TestIgnored_SkipAndUnskip(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "ign-roundtrip")
	insertExternalGame(t, testDB, "eg-1", userID, "steam", "730", "CS2")

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-1", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}

	rec2 := postJSONAuth(t, e, "/api/sync/ignored/eg-1", nil, token)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on idempotent skip, got %d", rec2.Code)
	}

	rec3 := getAuth(t, e, "/api/sync/ignored", token)
	var resp []map[string]any
	if err := json.Unmarshal(rec3.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if len(resp) != 1 || resp[0]["id"] != "eg-1" {
		t.Fatalf("expected eg-1 in ignored list, got %v", resp)
	}

	rec4 := deleteAuth(t, e, "/api/sync/ignored/eg-1", token)
	if rec4.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on unskip, got %d", rec4.Code)
	}

	var isSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-1'`).Scan(context.Background(), &isSkipped); err != nil {
		t.Fatalf("scan is_skipped: %v", err)
	}
	if isSkipped {
		t.Fatal("expected is_skipped=false after unskip")
	}
}

func TestSkipGame_MarksJobItemSkippedAndCompletesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-jobitem")
	insertExternalGame(t, testDB, "eg-skip-ji", userID, "steam", "777", "Skip Me")
	insertJob(t, testDB, "job-skip-ji", userID, "sync", "steam", "processing")
	// Insert a pending_review job_item linked to the external_game.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-skip-1', 'job-skip-ji', ?, '777', 'Skip Me', 'eg-skip-ji', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-skip-ji", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	var itemStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-skip-1'`).Scan(ctx, &itemStatus); err != nil {
		t.Fatalf("scan job_item status: %v", err)
	}
	if itemStatus != "skipped" {
		t.Errorf("expected job_item status=skipped, got %q", itemStatus)
	}

	var jobStatus string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = 'job-skip-ji'`).Scan(ctx, &jobStatus); err != nil {
		t.Fatalf("scan job status: %v", err)
	}
	if jobStatus != "completed" {
		t.Errorf("expected job status=completed after last item skipped, got %q", jobStatus)
	}

	// changes('skipped') must be written with the correct title.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM changes WHERE job_id = 'job-skip-ji'`,
	).Scan(context.Background(), &sc); err != nil {
		t.Fatalf("scan change: %v", err)
	}
	if sc.ChangeType != "skipped" {
		t.Errorf("sync_change change_type: want 'skipped', got %q", sc.ChangeType)
	}
	if sc.Title != "Skip Me" {
		t.Errorf("sync_change title: want 'Skip Me', got %q", sc.Title)
	}
}

func TestSkipGame_CascadesToChildren(t *testing.T) {
	// Skipping a parent must also skip its children and their job_items.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-cascade")

	insertExternalGame(t, testDB, "eg-skip-parent", userID, "playstation-store", "CUSA001", "Horizon")
	insertChildExternalGame(t, testDB, "eg-skip-child", userID, "playstation-store", "PPSA001", "Horizon", "eg-skip-parent")
	insertJob(t, testDB, "job-skip-cascade", userID, "sync", "playstation-store", "processing")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-skip-child', 'job-skip-cascade', ?, 'PPSA001', 'Horizon', 'eg-skip-child', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child job_item: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-skip-parent", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Child external_game must be skipped.
	var childSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-skip-child'`).Scan(ctx, &childSkipped); err != nil {
		t.Fatalf("scan child is_skipped: %v", err)
	}
	if !childSkipped {
		t.Error("expected child external_game to be skipped")
	}

	// Child job_item must be skipped.
	var childItemStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-skip-child'`).Scan(ctx, &childItemStatus); err != nil {
		t.Fatalf("scan child job_item status: %v", err)
	}
	if childItemStatus != "skipped" {
		t.Errorf("expected child job_item status=skipped, got %q", childItemStatus)
	}
}

func TestUnskipGame_CascadesToChildren(t *testing.T) {
	// Unskipping a parent must also unskip its children.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "unskip-cascade")

	// Insert parent and child, both pre-skipped.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES ('eg-unskip-parent', ?, 'playstation-store', 'CUSA002', 'Ratchet', true, true, false, now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES ('eg-unskip-child', ?, 'playstation-store', 'PPSA002', 'Ratchet', true, true, false, 'eg-unskip-parent', now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child: %v", err)
	}

	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip-parent", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var childSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-unskip-child'`).Scan(context.Background(), &childSkipped); err != nil {
		t.Fatalf("scan child is_skipped: %v", err)
	}
	if childSkipped {
		t.Error("expected child external_game to be unskipped after parent unskip")
	}
}

func TestIgnored_404ForUnknown(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "ign-404")

	rec := postJSONAuth(t, e, "/api/sync/ignored/nonexistent", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ─── TestHandleGetConfig ──────────────────────────────────────────────────────

func TestSyncListConfig_AfterPut(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "listcfg-afterput")

	// Create a steam config.
	rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
		"frequency": "daily",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// List configs — steam row should now exist, others default.
	rec = getAuth(t, e, "/api/sync/config", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("LIST expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 5 {
		t.Fatalf("expected total=5, got %v", resp["total"])
	}
	configs := resp["configs"].([]any)
	// Find steam config and verify it has frequency=daily.
	found := false
	for _, c := range configs {
		cfg := c.(map[string]any)
		if cfg["storefront"] == "steam" {
			if cfg["frequency"] != "daily" {
				t.Errorf("expected frequency=daily for steam, got %v", cfg["frequency"])
			}
			found = true
		}
	}
	if !found {
		t.Error("steam config not found in list")
	}
}

func TestSyncGetConfig_NoRow_ReturnsDefault(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "getcfg-norow")

	rec := getAuth(t, e, "/api/sync/config/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// When no row exists the handler returns a synthetic default.
	if resp["is_configured"].(bool) {
		t.Error("expected is_configured=false for missing config")
	}
	if resp["frequency"] != "manual" {
		t.Errorf("expected frequency=manual, got %v", resp["frequency"])
	}
}

func TestSyncGetConfig_AfterPut(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "getcfg-afterput")

	// Create a config first.
	rec := putJSONAuth(t, e, "/api/sync/config/playstation-store", map[string]any{
		"frequency": "weekly",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Now GET the same config.
	rec = getAuth(t, e, "/api/sync/config/playstation-store", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["frequency"] != "weekly" {
		t.Errorf("expected frequency=weekly, got %v", resp["frequency"])
	}
}

func TestSyncGetConfig_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "getcfg-invalid")

	rec := getAuth(t, e, "/api/sync/config/battlenet", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestHandleGetPlaystationStoreConnection with credentials ──────────────────────────────────

func TestPlaystationStoreStatus_WithCredentials(t *testing.T) {
	truncateAllTables(t)
	stub := &stubPlaystationStoreClient{
		info: &api.PlaystationStoreAccountInfo{OnlineID: "MyPSNName", AccountID: "123456"},
	}
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "psn-stat-cred")

	rawCreds := `{"npsso_token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","online_id":"MyPSNName","account_id":"123456","region":"GB","is_verified":true,"token_expired_at":null}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt psn creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'playstation-store', 'manual', ?, now(), now())`,
		"cfg-psn-cred", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	// Now get PSN status — should return configured=true.
	rec := getAuth(t, e, "/api/sync/playstation-store/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp["is_configured"].(bool) {
		t.Error("expected is_configured=true after PSN connect")
	}
}

// insertCorruptedSyncConfig inserts a user_sync_configs row with
// deliberately corrupted storefront_credentials to test error handling.
func insertCorruptedSyncConfig(t *testing.T, db *bun.DB, userID, storefront string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (gen_random_uuid()::text, ?, ?, 'manual', 'THIS IS NOT JSON', now(), now())
		 ON CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = 'THIS IS NOT JSON', updated_at = now()`,
		userID, storefront,
	)
	if err != nil {
		t.Fatalf("insertCorruptedSyncConfig: %v", err)
	}
}

// TestStatus_CorruptedCredentials verifies, for every storefront, that an
// undecryptable storefront_credentials row surfaces credentials_error=true and
// keeps the connected/configured flag true, without clearing the row.
func TestStatus_CorruptedCredentials(t *testing.T) {
	tests := []struct {
		name           string
		suffix         string
		storefront     string
		path           string
		connectedField string // "connected" or "is_configured"
		newApp         func(t *testing.T) interface {
			ServeHTTP(http.ResponseWriter, *http.Request)
		}
	}{
		{
			name:           "playstation-store",
			suffix:         "psn-corrupt-creds",
			storefront:     "playstation-store",
			path:           "/api/sync/playstation-store/connection",
			connectedField: "is_configured",
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			},
		},
		{
			name:           "epic-games-store",
			suffix:         "epic-corrupt-creds",
			storefront:     "epic-games-store",
			path:           "/api/sync/epic-games-store/connection",
			connectedField: "connected",
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, &stubEpicGamesStoreClient{configured: true})
			},
		},
		{
			name:           "gog",
			suffix:         "gog-corrupt-creds",
			storefront:     "gog",
			path:           "/api/sync/gog/connection",
			connectedField: "connected",
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, &stubGOGClient{})
			},
		},
		{
			name:           "steam",
			suffix:         "steam-conn-corrupt",
			storefront:     "steam",
			path:           "/api/sync/steam/connection",
			connectedField: "connected",
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := tt.newApp(t)
			userID, token := setupTagUser(t, testDB, e, tt.suffix)
			insertCorruptedSyncConfig(t, testDB, userID, tt.storefront)

			// Decrypt failure must surface credentials_error=true without clearing the row.
			rec := getAuth(t, e, tt.path, token)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for undecryptable %s credentials", rec.Code, tt.storefront)
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp[tt.connectedField] != true {
				t.Errorf("expected %s=true (row still exists), got %v", tt.connectedField, resp[tt.connectedField])
			}
			if resp["credentials_error"] != true {
				t.Errorf("expected credentials_error=true, got %v", resp["credentials_error"])
			}

			// Credentials row must NOT be cleared.
			var creds string
			err := testDB.NewRaw(
				`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ?`, userID, tt.storefront,
			).Scan(context.Background(), &creds)
			if err != nil {
				t.Fatalf("credentials row missing after decrypt failure: %v", err)
			}
			if creds == "" {
				t.Error("expected credentials to remain non-null after decrypt failure")
			}
		})
	}
}

// ─── TestListExternalGames ────────────────────────────────────────────────────

func TestListExternalGames_EmptyList(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "eg-empty")

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp))
	}
}

func TestListExternalGames_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "eg-bad-sf")

	rec := getAuth(t, e, "/api/sync/notaplatform/external-games", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListExternalGames_IsolatedByUser(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userA, tokenA := setupTagUser(t, testDB, e, "eg-user-a")
	_, tokenB := setupTagUser(t, testDB, e, "eg-user-b")
	insertExternalGame(t, testDB, "eg-a1", userA, "steam", "730", "CS2")

	rec := getAuth(t, e, "/api/sync/steam/external-games", tokenA)
	var respA []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &respA)
	if len(respA) != 1 {
		t.Fatalf("user A should see 1 game, got %d", len(respA))
	}

	rec2 := getAuth(t, e, "/api/sync/steam/external-games", tokenB)
	var respB []map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &respB)
	if len(respB) != 0 {
		t.Fatalf("user B should see 0 games, got %d", len(respB))
	}
}

func TestListExternalGames_ExcludesChildren(t *testing.T) {
	// A child row (parent_id IS NOT NULL) must never appear in the list.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-excludes-children")

	insertExternalGame(t, testDB, "eg-parent-1", userID, "playstation-store", "CUSA001", "Horizon")
	insertChildExternalGame(t, testDB, "eg-child-1", userID, "playstation-store", "PPSA001", "Horizon", "eg-parent-1")

	rec := getAuth(t, e, "/api/sync/playstation-store/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result (parent only), got %d", len(resp))
	}
	if resp[0]["id"] != "eg-parent-1" {
		t.Errorf("expected parent row, got id=%v", resp[0]["id"])
	}
}

func TestListExternalGames_AggregatesChildPlatforms(t *testing.T) {
	// The parent entry must include platform slugs from child rows.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-agg-platforms")

	insertExternalGame(t, testDB, "eg-par-2", userID, "playstation-store", "CUSA002", "God of War")
	insertExternalGamePlatform(t, testDB, "eg-par-2", "playstation-4")
	insertChildExternalGame(t, testDB, "eg-chi-2", userID, "playstation-store", "PPSA002", "God of War", "eg-par-2")
	insertExternalGamePlatform(t, testDB, "eg-chi-2", "playstation-5")

	rec := getAuth(t, e, "/api/sync/playstation-store/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}
	platforms, ok := resp[0]["platforms"].([]any)
	if !ok {
		t.Fatalf("expected platforms array, got %T", resp[0]["platforms"])
	}
	platformSet := make(map[string]bool)
	for _, p := range platforms {
		platformSet[p.(string)] = true
	}
	if !platformSet["playstation-4"] || !platformSet["playstation-5"] {
		t.Errorf("expected both playstation-4 and playstation-5 in platforms, got %v", platforms)
	}
}

func TestListExternalGames_StoreURL(t *testing.T) {
	// A row with a resolved store_link gets a computed store_url; a row without
	// one (NULL store_link) gets a null store_url.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-store-url")

	insertExternalGame(t, testDB, "eg-link", userID, "steam", "2520", "Alone in the Dark")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET store_link = '2520' WHERE id = 'eg-link'`); err != nil {
		t.Fatalf("set store_link: %v", err)
	}
	insertExternalGame(t, testDB, "eg-nolink", userID, "steam", "9999", "Mystery Game")

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byID := make(map[string]map[string]any, len(resp))
	for _, g := range resp {
		byID[g["id"].(string)] = g
	}
	if got := byID["eg-link"]["store_url"]; got != "https://store.steampowered.com/app/2520/" {
		t.Errorf("eg-link store_url = %v, want steam app URL", got)
	}
	if got := byID["eg-nolink"]["store_url"]; got != nil {
		t.Errorf("eg-nolink store_url = %v, want nil", got)
	}
}

// ─── TestRematchExternalGame ──────────────────────────────────────────────────

func insertUserGameAndPlatform(t *testing.T, db *bun.DB, ugID, userID, gameIDInt, ugpID, externalGameID string) {
	t.Helper()
	gameID := 0
	_, _ = fmt.Sscanf(gameIDInt, "%d", &gameID)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT DO NOTHING`,
		gameID)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now()) ON CONFLICT DO NOTHING`,
		ugID, userID, gameID)
	if err != nil {
		t.Fatalf("insert user_game: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, external_game_id, platform, storefront, sync_from_source, is_available, created_at, updated_at)
		 VALUES (?, ?, ?, 'pc-windows', 'steam', true, true, now(), now())`,
		ugpID, ugID, externalGameID)
	if err != nil {
		t.Fatalf("insert user_game_platform: %v", err)
	}
}

func TestRematchExternalGame_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "rm-404")

	rec := postJSONAuth(t, e, "/api/sync/external-games/nonexistent/rematch",
		map[string]any{"igdb_id": 1234}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRematchExternalGame_OtherUsersGame(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userA, _ := setupTagUser(t, testDB, e, "rm-userA")
	_, tokenB := setupTagUser(t, testDB, e, "rm-userB")
	insertExternalGame(t, testDB, "eg-rm-a", userA, "steam", "42", "Some Game")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-a/rematch",
		map[string]any{"igdb_id": 1234}, tokenB)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for other user's game, got %d", rec.Code)
	}
}

func TestRematchExternalGame_OrphanRequiresAction(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-orphan")
	insertExternalGame(t, testDB, "eg-rm-1", userID, "steam", "1", "Game")
	// Link to a user_game that has only this one platform
	insertUserGameAndPlatform(t, testDB, "ug-rm-1", userID, "1111", "ugp-rm-1", "eg-rm-1")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-1/rematch",
		map[string]any{"igdb_id": 9999}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when orphan_action missing, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRematchExternalGame_KeepOrphan(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-keep")
	insertExternalGame(t, testDB, "eg-rm-2", userID, "steam", "2", "Game Two")
	insertUserGameAndPlatform(t, testDB, "ug-rm-2", userID, "2222", "ugp-rm-2", "eg-rm-2")
	// Insert a recent sync job so HandleRematchExternalGame can create a job_item attached to it
	insertJob(t, testDB, "job-rm-keep", userID, "sync", "steam", "pending")

	// Insert the target games row so FK is satisfied
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (8888, 'New IGDB Game', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-2/rematch",
		map[string]any{"igdb_id": 8888, "orphan_action": "keep"}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// user_game_platform should be gone
	var ugpCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-rm-2'`).Scan(context.Background(), &ugpCount)
	if ugpCount != 0 {
		t.Fatal("expected user_game_platform to be deleted")
	}
	// user_game should still exist
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-rm-2'`).Scan(context.Background(), &ugCount)
	if ugCount != 1 {
		t.Fatal("expected user_game to be kept with orphan_action=keep")
	}
	// external_game resolved_igdb_id updated
	var resolvedID int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-rm-2'`).Scan(context.Background(), &resolvedID)
	if resolvedID != 8888 {
		t.Fatalf("expected resolved_igdb_id=8888, got %d", resolvedID)
	}
}

func TestRematchExternalGame_RemoveOrphan(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-remove")
	insertExternalGame(t, testDB, "eg-rm-3", userID, "steam", "3", "Game Three")
	insertUserGameAndPlatform(t, testDB, "ug-rm-3", userID, "3333", "ugp-rm-3", "eg-rm-3")
	// Insert a recent sync job so HandleRematchExternalGame can create a job_item attached to it
	insertJob(t, testDB, "job-rm-remove", userID, "sync", "steam", "pending")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (7777, 'Another Game', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-3/rematch",
		map[string]any{"igdb_id": 7777, "orphan_action": "remove"}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-rm-3'`).Scan(context.Background(), &ugCount)
	if ugCount != 0 {
		t.Fatal("expected user_game to be deleted with orphan_action=remove")
	}
}

func TestRematchExternalGame_ResolvesSiblings(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-siblings")

	// Two PSN external_games for the same game title (PS4 parent + PS5 child variant).
	insertExternalGame(t, testDB, "eg-ps4", userID, "playstation-store", "PPSA-001", "Spider-Man 2")
	insertChildExternalGame(t, testDB, "eg-ps5", userID, "playstation-store", "PPSA-002", "Spider-Man 2", "eg-ps4")

	// A recent sync job for the sibling fallback path.
	insertJob(t, testDB, "job-sib", userID, "sync", "playstation-store", "processing")

	// pending_review job_item for the primary (PS4).
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-ps4', 'job-sib', ?, 'PPSA-001', 'Spider-Man 2', 'eg-ps4', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert primary job_item: %v", err)
	}

	// No job_item for the sibling (PS5) — the fallback path will create one.

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (5555, 'Spider-Man 2', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-ps4/rematch",
		map[string]any{"igdb_id": 5555}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Sibling (PS5) should now have resolved_igdb_id set.
	var sibResolvedID *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-ps5'`).Scan(ctx, &sibResolvedID); err != nil {
		t.Fatalf("scan sibling resolved_igdb_id: %v", err)
	}
	if sibResolvedID == nil || *sibResolvedID != 5555 {
		t.Errorf("expected sibling resolved_igdb_id=5555, got %v", sibResolvedID)
	}

	// A job_item for the sibling should have been created (fallback path).
	var sibItemCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE external_game_id = 'eg-ps5'`,
	).Scan(ctx, &sibItemCount); err != nil {
		t.Fatalf("scan sibling job_item count: %v", err)
	}
	if sibItemCount != 1 {
		t.Errorf("expected 1 job_item for sibling, got %d", sibItemCount)
	}
}

// TestRematchExternalGame_UpdatesJobItemStatusToPending verifies that a
// pending_review job_item is immediately set to pending when rematch is called,
// so the game disappears from the needs-review list without waiting for River.
func TestRematchExternalGame_UpdatesJobItemStatusToPending(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-status")
	insertExternalGame(t, testDB, "eg-status-1", userID, "steam", "111", "Portal 2")
	insertJob(t, testDB, "job-status-1", userID, "sync", "steam", "processing")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-status-1', 'job-status-1', ?, '111', 'Portal 2', 'eg-status-1', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (6001, 'Portal 2', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-status-1/rematch",
		map[string]any{"igdb_id": 6001}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-status-1'`).Scan(context.Background(), &status); err != nil {
		t.Fatalf("scan job_item status: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected job_item status=pending after rematch, got %q", status)
	}
}

// TestRematchExternalGame_UpdatesSiblingJobItemStatusToPending verifies that
// pending_review job_items for sibling external_games are also immediately set
// to pending when a rematch resolves the whole sibling group.
func TestRematchExternalGame_UpdatesSiblingJobItemStatusToPending(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-sib-status")

	insertExternalGame(t, testDB, "eg-sib-s1", userID, "playstation-store", "PPSA-101", "God of War")
	insertChildExternalGame(t, testDB, "eg-sib-s2", userID, "playstation-store", "PPSA-102", "God of War", "eg-sib-s1")
	insertJob(t, testDB, "job-sib-s", userID, "sync", "playstation-store", "processing")

	for _, row := range []struct{ id, egID, extID string }{
		{"ji-sib-s1", "eg-sib-s1", "PPSA-101"},
		{"ji-sib-s2", "eg-sib-s2", "PPSA-102"},
	} {
		_, err := testDB.ExecContext(context.Background(),
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, 'job-sib-s', ?, ?, 'God of War', ?, '{}', 'pending_review', '{}', '[]', now())`,
			row.id, userID, row.extID, row.egID,
		)
		if err != nil {
			t.Fatalf("insert job_item %s: %v", row.id, err)
		}
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (6002, 'God of War', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-sib-s1/rematch",
		map[string]any{"igdb_id": 6002}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE id IN ('ji-sib-s1', 'ji-sib-s2') AND status = 'pending'`,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("scan job_item status count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected both job_items to have status=pending after rematch, got count=%d", count)
	}
}

// TestRematchExternalGame_AfterSyncCompleted reproduces the bug where
// HandleRematchExternalGame returns 500 "failed to create job item" after a
// sync has completed. The job_item for the external game is already in
// 'completed' status; the handler must update it instead of inserting a
// duplicate that would violate the UNIQUE(job_id, item_key) constraint.
func TestRematchExternalGame_AfterSyncCompleted(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-postcomplete")
	insertExternalGame(t, testDB, "eg-pc-1", userID, "steam", "555", "Half-Life 3")
	insertJob(t, testDB, "job-pc-1", userID, "sync", "steam", "completed")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-pc-1', 'job-pc-1', ?, '555', 'Half-Life 3', 'eg-pc-1', '{}', 'completed', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert completed job_item: %v", err)
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (9001, 'Half-Life 3', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-pc-1/rematch",
		map[string]any{"igdb_id": 9001}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-pc-1'`).Scan(context.Background(), &status); err != nil {
		t.Fatalf("scan job_item status: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected existing job_item reset to pending, got %q", status)
	}
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE external_game_id = 'eg-pc-1'`).Scan(context.Background(), &count)
	if count != 1 {
		t.Errorf("expected exactly 1 job_item for the external game, got %d", count)
	}
}

// TestRematchExternalGame_SiblingAfterSyncCompleted verifies that sibling
// external_games with 'completed' job_items are also reset to 'pending'
// without triggering a duplicate-key error.
func TestRematchExternalGame_SiblingAfterSyncCompleted(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-sib-postcomplete")

	insertExternalGame(t, testDB, "eg-sib-pc1", userID, "playstation-store", "CUSA-001", "Horizon")
	insertChildExternalGame(t, testDB, "eg-sib-pc2", userID, "playstation-store", "CUSA-002", "Horizon", "eg-sib-pc1")
	insertJob(t, testDB, "job-sib-pc", userID, "sync", "playstation-store", "completed")

	for _, row := range []struct{ id, egID, extID string }{
		{"ji-sib-pc1", "eg-sib-pc1", "CUSA-001"},
		{"ji-sib-pc2", "eg-sib-pc2", "CUSA-002"},
	} {
		_, err := testDB.ExecContext(context.Background(),
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, 'job-sib-pc', ?, ?, 'Horizon', ?, '{}', 'completed', '{}', '[]', now())`,
			row.id, userID, row.extID, row.egID,
		)
		if err != nil {
			t.Fatalf("insert job_item %s: %v", row.id, err)
		}
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (9002, 'Horizon', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-sib-pc1/rematch",
		map[string]any{"igdb_id": 9002}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE id IN ('ji-sib-pc1', 'ji-sib-pc2') AND status = 'pending'`,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("scan job_item status count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected both sibling job_items reset to pending, got count=%d", count)
	}
}

func TestRematchExternalGame_CascadesToChildrenViaParentID(t *testing.T) {
	// Rematching a parent must cascade resolved_igdb_id to children via parent_id,
	// not title search. A second game with the same title but different user must NOT
	// be affected.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userA, tokenA := setupTagUser(t, testDB, e, "rm-fk-userA")
	userB, _ := setupTagUser(t, testDB, e, "rm-fk-userB")

	// User A: parent + child, same title.
	insertExternalGame(t, testDB, "eg-fk-parent", userA, "playstation-store", "CUSA100", "Spider-Man")
	insertChildExternalGame(t, testDB, "eg-fk-child", userA, "playstation-store", "PPSA100", "Spider-Man", "eg-fk-parent")
	insertJob(t, testDB, "job-fk", userA, "sync", "playstation-store", "processing")
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-fk-child', 'job-fk', ?, 'PPSA100', 'Spider-Man', 'eg-fk-child', '{}', 'pending', '{}', '[]', now())`,
		userA,
	)
	if err != nil {
		t.Fatalf("insert child job_item: %v", err)
	}

	// User B: unrelated game with same title — must NOT be touched.
	insertExternalGame(t, testDB, "eg-fk-other", userB, "playstation-store", "CUSA200", "Spider-Man")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-fk-parent/rematch",
		map[string]any{"igdb_id": 4242}, tokenA)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Child must have inherited resolved_igdb_id.
	var childResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-child'`).Scan(ctx, &childResolved); err != nil {
		t.Fatalf("scan child resolved_igdb_id: %v", err)
	}
	if childResolved == nil || *childResolved != 4242 {
		t.Errorf("child resolved_igdb_id: want 4242, got %v", childResolved)
	}

	// User B's game must NOT have been touched.
	var otherResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-other'`).Scan(ctx, &otherResolved); err != nil {
		t.Fatalf("scan other resolved_igdb_id: %v", err)
	}
	if otherResolved != nil {
		t.Errorf("other user's game must not be resolved, got %v", *otherResolved)
	}
}

// TestRematchExternalGame_MultiPlatformOrphanRemove reproduces the bug where
// a Steam game with both Windows and Linux platform rows only had one UGP
// deleted (LIMIT 1), leaving the user_game alive despite orphan_action=remove.
// The backend otherCount also disagreed with the frontend's count because it
// used id != ugpID instead of external_game_id IS DISTINCT FROM eg.id.
func TestRematchExternalGame_MultiPlatformOrphanRemove(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-mp-orphan")

	// Wrong IGDB match — external_game must exist before UGPs (FK constraint).
	insertExternalGame(t, testDB, "eg-mp-1", userID, "steam", "3000", "Another World")
	insertUserGameAndPlatform(t, testDB, "ug-mp-wrong", userID, "1111", "ugp-mp-win", "eg-mp-1")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 1111 WHERE id = 'eg-mp-1'`); err != nil {
		t.Fatalf("set wrong match: %v", err)
	}
	// Second platform (Linux) on the same user_game and same external_game.
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, external_game_id, platform, storefront, sync_from_source, is_available, created_at, updated_at)
		 VALUES ('ugp-mp-lin', 'ug-mp-wrong', 'eg-mp-1', 'pc-linux', 'steam', true, true, now(), now())`); err != nil {
		t.Fatalf("insert linux ugp: %v", err)
	}

	insertJob(t, testDB, "job-mp", userID, "sync", "steam", "completed")
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-mp-1', 'job-mp', ?, '3000', 'Another World', 'eg-mp-1', '{}', 'completed', '{}', '[]', now())`,
		userID); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (2222, 'Another World', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-mp-1/rematch",
		map[string]any{"igdb_id": 2222, "orphan_action": "remove"}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Both Windows and Linux UGPs must be gone.
	var ugpCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = 'ug-mp-wrong'`,
	).Scan(ctx, &ugpCount); err != nil {
		t.Fatalf("scan ugp count: %v", err)
	}
	if ugpCount != 0 {
		t.Errorf("expected all UGPs deleted, got %d remaining", ugpCount)
	}

	// The wrong user_game must be deleted.
	var ugCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM user_games WHERE id = 'ug-mp-wrong'`,
	).Scan(ctx, &ugCount); err != nil {
		t.Fatalf("scan user_game count: %v", err)
	}
	if ugCount != 0 {
		t.Errorf("expected wrong user_game deleted, got %d rows", ugCount)
	}
}

// TestRematchExternalGame_SiblingAlreadyMatchedWrong reproduces the bug where
// rematching one platform variant of a game leaves sibling variants on the
// wrong match because the sibling query filtered resolved_igdb_id IS NULL.
func TestRematchExternalGame_SiblingAlreadyMatchedWrong(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-sib-wrong")

	// Two Steam entries for "Another World" — both previously matched to the wrong IGDB game (id 1111).
	// The games row must exist before setting resolved_igdb_id (FK constraint).
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (1111, 'Wrong Game', now(), now()) ON CONFLICT DO NOTHING`)
	if err != nil {
		t.Fatalf("insert wrong game: %v", err)
	}
	insertExternalGame(t, testDB, "eg-aw-win", userID, "steam", "2100", "Another World")
	insertChildExternalGame(t, testDB, "eg-aw-lin", userID, "steam", "2101", "Another World", "eg-aw-win")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 1111 WHERE id IN ('eg-aw-win', 'eg-aw-lin')`); err != nil {
		t.Fatalf("set wrong match: %v", err)
	}

	insertJob(t, testDB, "job-aw", userID, "sync", "steam", "completed")
	for _, row := range []struct{ id, egID, key string }{
		{"ji-aw-win", "eg-aw-win", "2100"},
		{"ji-aw-lin", "eg-aw-lin", "2101"},
	} {
		_, err := testDB.ExecContext(context.Background(),
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, 'job-aw', ?, ?, 'Another World', ?, '{}', 'completed', '{}', '[]', now())`,
			row.id, userID, row.key, row.egID,
		)
		if err != nil {
			t.Fatalf("insert job_item %s: %v", row.id, err)
		}
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (2222, 'Another World', now(), now()) ON CONFLICT DO NOTHING`)

	// Rematch the Windows entry to the correct game.
	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-aw-win/rematch",
		map[string]any{"igdb_id": 2222}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Both Windows and Linux should now point to the correct IGDB game.
	ctx := context.Background()
	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE id IN ('eg-aw-win', 'eg-aw-lin') AND resolved_igdb_id = 2222`,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("scan resolved count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected both platform variants resolved to 2222, got count=%d", count)
	}
}

// ─── TestUnskipGame_EnqueuesJobItem ───────────────────────────────────────────

func TestUnskipGame_EnqueuesJobItem(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "unskip-enqueue")
	insertExternalGame(t, testDB, "eg-unskip", userID, "steam", "42", "Half-Life 3")

	// Skip first
	postJSONAuth(t, e, "/api/sync/ignored/eg-unskip", nil, token)

	// Unskip
	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// is_skipped should be false
	var isSkipped bool
	_ = testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-unskip'`).
		Scan(context.Background(), &isSkipped)
	if isSkipped {
		t.Fatal("expected is_skipped=false after unskip")
	}

	// A job and job_item should exist
	var jobCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = 'steam'`, userID,
	).Scan(context.Background(), &jobCount)
	if jobCount != 1 {
		t.Fatalf("expected 1 sync job created by unskip, got %d", jobCount)
	}

	var itemCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE item_key = '42' AND user_id = ?`, userID,
	).Scan(context.Background(), &itemCount)
	if itemCount != 1 {
		t.Fatalf("expected 1 job_item with item_key='42', got %d", itemCount)
	}
}

// TestUnskipGame_RiverInsertFails_MarksItemFailed locks in the dual-write
// contract enforced by tasks.EnqueueOrFail: when the River insert fails, the
// just-created job_item must be marked 'failed' rather than stranded in
// 'pending' with no backing river_job (the stuck-item class this consolidation
// closes — #1058). The handler also surfaces a 500 to the caller.
func TestUnskipGame_RiverInsertFails_MarksItemFailed(t *testing.T) {
	truncateAllTables(t)
	rc := newFailingRiverClient(t)
	e := newSyncTestAppWithRiverClient(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, rc)
	userID, token := setupTagUser(t, testDB, e, "unskip-river-fail")
	insertExternalGame(t, testDB, "eg-unskip-fail", userID, "steam", "99", "Portal 3")

	postJSONAuth(t, e, "/api/sync/ignored/eg-unskip-fail", nil, token)

	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip-fail", token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when River insert fails, got %d: %s", rec.Code, rec.Body.String())
	}

	// The job_item must not be left stuck in 'pending'.
	var status string
	if err := testDB.NewRaw(
		`SELECT status FROM job_items WHERE item_key = '99' AND user_id = ?`, userID,
	).Scan(context.Background(), &status); err != nil {
		t.Fatalf("query job_item status: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected job_item status='failed' after River insert failure, got %q", status)
	}
}

// ── TestResetSyncData ─────────────────────────────────────────────────────────

func TestResetSyncData_DeletesDataAndResetsTimestamp(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "reset-data")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, 'steam', 'manual', now(), now(), now())`,
		userID,
	)
	insertExternalGame(t, testDB, "eg-reset-1", userID, "steam", "730", "CS2")
	insertUserGameAndPlatform(t, testDB, "ug-reset-1", userID, "12345", "ugp-reset-1", "eg-reset-1")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	var egCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &egCount)
	if egCount != 0 {
		t.Errorf("expected 0 external_games, got %d", egCount)
	}

	var ugpCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-reset-1'`).Scan(ctx, &ugpCount)
	if ugpCount != 0 {
		t.Errorf("expected 0 user_game_platforms, got %d", ugpCount)
	}

	// user_games must survive the reset.
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-reset-1'`).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected user_game to survive reset, got %d rows", ugCount)
	}

	var lastSyncedAt *time.Time
	_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &lastSyncedAt)
	if lastSyncedAt != nil {
		t.Errorf("expected last_synced_at=NULL after reset, got %v", lastSyncedAt)
	}
}

func TestResetSyncData_CancelsActiveJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "reset-cancel")

	insertJob(t, testDB, "job-reset-active", userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-reset-active", "job-reset-active", userID, "key-1", "Game 1", "pending")
	riverID := insertRiverJob(t, testDB, "import_item", "available", "ji-reset-active")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = 'job-reset-active'`).Scan(context.Background(), &status)
	if status != "cancelled" {
		t.Errorf("expected active job to be cancelled, got %q", status)
	}

	if state := riverJobState(t, testDB, riverID); state != "cancelled" {
		t.Errorf("expected river_job state=cancelled, got %q", state)
	}
}

func TestResetSyncData_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "reset-invalid")

	rec := deleteAuth(t, e, "/api/sync/fakefront/data", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResetSyncData_Unauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})

	req := httptest.NewRequest(http.MethodDelete, "/api/sync/steam/data", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestResetSyncData_EmptyState(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "reset-empty")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on empty reset (idempotent), got %d", rec.Code)
	}
}

func TestListExternalGames_AllStates(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-states")
	insertExternalGame(t, testDB, "eg-matched", userID, "steam", "1", "Matched Game")
	insertExternalGame(t, testDB, "eg-unmatched", userID, "steam", "2", "Unmatched Game")
	insertExternalGame(t, testDB, "eg-skipped", userID, "steam", "3", "Skipped Game")

	// Mark skipped
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET is_skipped = true WHERE id = 'eg-skipped'`)

	// Set resolved IGDB ID (insert games row first to satisfy FK)
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (9999, 'IGDB Title', now(), now()) ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 9999 WHERE id = 'eg-matched'`)

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 3 {
		t.Fatalf("expected 3 games, got %d", len(resp))
	}
	byID := make(map[string]map[string]any)
	for _, g := range resp {
		byID[g["id"].(string)] = g
	}
	if byID["eg-matched"]["igdb_title"] != "IGDB Title" {
		t.Errorf("expected igdb_title='IGDB Title', got %v", byID["eg-matched"]["igdb_title"])
	}
	if byID["eg-unmatched"]["igdb_title"] != nil {
		t.Errorf("expected igdb_title=nil for unmatched, got %v", byID["eg-unmatched"]["igdb_title"])
	}
	if byID["eg-skipped"]["is_skipped"] != true {
		t.Errorf("expected is_skipped=true for skipped game")
	}
}

func TestListExternalGames_ExcludesInFlight(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-inflight")

	insertExternalGame(t, testDB, "eg-stable", userID, "steam", "10", "Stable Game")
	insertExternalGame(t, testDB, "eg-inflight-1", userID, "steam", "20", "In-Flight Game")
	insertJob(t, testDB, "job-inflight", userID, "sync", "steam", "processing")

	// pending job_item links eg-inflight-1 to the active job.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-inflight', 'job-inflight', ?, '20', 'In-Flight Game', 'eg-inflight-1', '{}', 'pending', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert in-flight job_item: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 game (in-flight excluded), got %d", len(resp))
	}
	if resp[0]["id"] != "eg-stable" {
		t.Errorf("expected eg-stable in response, got %v", resp[0]["id"])
	}
}

func TestListExternalGames_ReturnsPlatforms(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-plat")

	insertExternalGame(t, testDB, "eg-p1", userID, "steam", "730", "CS2")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES ('egp-1', 'eg-p1', 'pc-windows', 0, now()),
		        ('egp-2', 'eg-p1', 'pc-linux', 0, now())`)
	if err != nil {
		t.Fatalf("insert platforms: %v", err)
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (8888, 'CS2', now(), now()) ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 8888 WHERE id = 'eg-p1'`)

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 game, got %d", len(resp))
	}
	plRaw, ok := resp[0]["platforms"]
	if !ok {
		t.Fatal("expected 'platforms' field in response")
	}
	platforms, _ := plRaw.([]any)
	if len(platforms) != 2 {
		t.Errorf("expected 2 platforms, got %v", plRaw)
	}
	platformStrs := make(map[string]bool)
	for _, p := range platforms {
		platformStrs[p.(string)] = true
	}
	if !platformStrs["pc-windows"] || !platformStrs["pc-linux"] {
		t.Errorf("expected pc-windows and pc-linux, got %v", platforms)
	}
}

func TestListExternalGames_NoPlatforms_ReturnsEmptyArray(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-noplat")

	insertExternalGame(t, testDB, "eg-np1", userID, "steam", "999", "Orphan Game")

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	var resp []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 game, got %d", len(resp))
	}
	plRaw, ok := resp[0]["platforms"]
	if !ok {
		t.Fatal("expected 'platforms' field in response")
	}
	if plRaw == nil {
		t.Fatal("expected empty array [], got JSON null")
	}
	platforms, _ := plRaw.([]any)
	if len(platforms) != 0 {
		t.Errorf("expected empty platforms array, got %v", plRaw)
	}
}

// ─── Epic connection-handler tests ────────────────────────────────────────────

type stubEpicGamesStoreClient struct {
	info       *api.EpicGamesStoreAccountInfo
	snapshot   map[string]string
	authErr    error
	cleanupErr error
	configured bool

	authCalled    bool
	cleanupCalled bool
	cleanupUserID string
}

func (s *stubEpicGamesStoreClient) Authenticate(_ context.Context, _, _ string) (*api.EpicGamesStoreAccountInfo, map[string]string, error) {
	s.authCalled = true
	return s.info, s.snapshot, s.authErr
}

func (s *stubEpicGamesStoreClient) Cleanup(_ context.Context, userID string) error {
	s.cleanupCalled = true
	s.cleanupUserID = userID
	return s.cleanupErr
}

func (s *stubEpicGamesStoreClient) Configured() bool {
	return s.configured
}

// newSyncTestAppWithEpicGamesStore builds the sync app with a custom EpicGamesStoreClient. Used by
// the Epic connection tests.
func newSyncTestAppWithEpicGamesStore(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PlaystationStoreClient, epic api.EpicGamesStoreClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, newTestRiverClient(t), steam, psn, epic, (api.GOGClient)(nil), (api.HumbleClient)(nil))
	g := e.Group("/api/sync", auth.AuthMiddleware(db))
	synch.RegisterRoutes(g)
	return e
}

type stubGOGClient struct {
	authURL string
	token   *api.GOGTokenResponse
	err     error
	gotCode string
}

func (s *stubGOGClient) BuildAuthURL() string {
	if s.authURL != "" {
		return s.authURL
	}
	return "https://login.gog.com/auth?test=1"
}

func (s *stubGOGClient) ExchangeCode(_ context.Context, code string) (*api.GOGTokenResponse, error) {
	s.gotCode = code
	return s.token, s.err
}

func newSyncTestAppWithRiverClient(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PlaystationStoreClient, rc *river.Client[pgx.Tx]) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, rc, steam, psn, (api.EpicGamesStoreClient)(nil), (api.GOGClient)(nil), (api.HumbleClient)(nil))
	g := e.Group("/api/sync", auth.AuthMiddleware(db))
	synch.RegisterRoutes(g)
	return e
}

// TestHandleTriggerSync_RiverInsertFails_Returns500 locks in the contract that
// HandleTriggerSync returns 500 when the River enqueue fails, rather than
// silently succeeding with an orphaned job.
func TestHandleTriggerSync_RiverInsertFails_Returns500(t *testing.T) {
	truncateAllTables(t)
	rc := newFailingRiverClient(t)
	e := newSyncTestAppWithRiverClient(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, rc)
	_, token := setupTagUser(t, testDB, e, "trigger-river-fail")

	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when River insert fails, got %d: %s", rec.Code, rec.Body.String())
	}
}

func newSyncTestAppWithGOG(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PlaystationStoreClient, gog api.GOGClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, newTestRiverClient(t), steam, psn, (api.EpicGamesStoreClient)(nil), gog, (api.HumbleClient)(nil))
	g := e.Group("/api/sync", auth.AuthMiddleware(db))
	synch.RegisterRoutes(g)
	return e
}

func TestHandleEpicGamesStoreConnect_503WhenNotConfigured(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: false}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-503")

	rec := putJSONAuth(t, e, "/api/sync/epic-games-store/connection", map[string]any{"auth_code": "abc"}, token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.authCalled {
		t.Error("Authenticate must not be called when not configured")
	}
}

func TestHandleEpicGamesStoreConnect_400OnMissingAuthCode(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-400")

	rec := putJSONAuth(t, e, "/api/sync/epic-games-store/connection", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if stub.authCalled {
		t.Error("Authenticate must not be called when auth_code missing")
	}
}

func TestHandleEpicGamesStoreConnect_400OnAuthError(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true, authErr: fmt.Errorf("legendary: invalid code")}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-auth-fail")

	rec := putJSONAuth(t, e, "/api/sync/epic-games-store/connection", map[string]any{"auth_code": "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEpicGamesStoreConnect_500OnNilInfo(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true, info: nil, snapshot: map[string]string{"user.json": "{}"}}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-nil")

	rec := putJSONAuth(t, e, "/api/sync/epic-games-store/connection", map[string]any{"auth_code": "ok"}, token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleEpicGamesStoreConnect_HappyPathPersistsConfig(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{
		configured: true,
		info:       &api.EpicGamesStoreAccountInfo{DisplayName: "EpicTester", AccountID: "acct-123"},
		snapshot:   map[string]string{"user.json": `{"displayName":"EpicTester","account_id":"acct-123"}`},
	}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-conn-ok")

	rec := putJSONAuth(t, e, "/api/sync/epic-games-store/connection", map[string]any{"auth_code": "ok"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["display_name"] != "EpicTester" || resp["account_id"] != "acct-123" {
		t.Errorf("unexpected response body: %v", resp)
	}

	var credsRaw string
	err := testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic-games-store'`,
		userID,
	).Scan(context.Background(), &credsRaw)
	if err != nil {
		t.Fatalf("scan user_sync_configs: %v", err)
	}
	// storefront_credentials holds the encrypted legendary snapshot.
	if !strings.HasPrefix(credsRaw, "enc:v1:") {
		t.Fatalf("expected enc:v1: prefix for storefront_credentials, got %q", credsRaw[:min(20, len(credsRaw))])
	}
	decryptedState, err := testEncrypter.Decrypt(credsRaw)
	if err != nil {
		t.Fatalf("decrypt storefront_credentials: %v", err)
	}
	var stateMap map[string]string
	if err := json.Unmarshal(decryptedState, &stateMap); err != nil {
		t.Fatalf("unmarshal decrypted storefront_credentials: %v", err)
	}
	if _, ok := stateMap["user.json"]; !ok {
		t.Errorf("decrypted storefront_credentials missing user.json key: %v", stateMap)
	}
}

func TestHandleEpicGamesStoreDisconnect_ClearsCredsSnapshotAndCallsCleanup(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-disc")

	// Pre-populate a connected Epic row: storefront_credentials holds the encrypted snapshot.
	snapJSON := `{"user.json":"{}"}`
	snapCiphertext, err := testEncrypter.Encrypt([]byte(snapJSON))
	if err != nil {
		t.Fatalf("encrypt snap fixture: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'epic-games-store', 'manual', ?, now(), now())`,
		"cfg-epic-disc", userID, snapCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	rec := deleteAuth(t, e, "/api/sync/epic-games-store/connection", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if !stub.cleanupCalled || stub.cleanupUserID != userID {
		t.Errorf("expected Cleanup(%q) to be called, got called=%v user=%q", userID, stub.cleanupCalled, stub.cleanupUserID)
	}

	var credsAfter *string
	err = testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic-games-store'`,
		userID,
	).Scan(context.Background(), &credsAfter)
	if err != nil {
		t.Fatalf("scan after disconnect: %v", err)
	}
	if credsAfter != nil {
		t.Errorf("expected storefront_credentials cleared, got %v", *credsAfter)
	}
}

func TestHandleGetEpicGamesStoreConnection_DisabledWhenNotConfigured(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: false}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-status-disabled")

	rec := getAuth(t, e, "/api/sync/epic-games-store/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != false || resp["disabled"] != true || resp["reason"] != "legendary_not_configured" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestHandleGetEpicGamesStoreConnection_NotConnectedWhenNoRow(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-status-noconn")

	rec := getAuth(t, e, "/api/sync/epic-games-store/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != false || resp["disabled"] != false {
		t.Errorf("unexpected response: %v", resp)
	}
	if _, hasReason := resp["reason"]; hasReason {
		t.Errorf("response should not include reason when not disabled, got: %v", resp)
	}
}

func TestHandleGetEpicGamesStoreConnection_ConnectedReturnsDisplayName(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicGamesStoreClient{configured: true}
	e := newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-status-conn")

	// Seed the realistic legendary snapshot shape the connect flow actually
	// persists: a map[relPath]content where user.json holds displayName/account_id.
	rawCreds := `{"user.json":"{\"displayName\":\"PlayerOne\",\"account_id\":\"acct-xyz\"}","installed.json":"{}"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt epic creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'epic-games-store', 'manual', ?, now(), now())`,
		"cfg-epic-conn", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/epic-games-store/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != true || resp["disabled"] != false {
		t.Errorf("expected connected=true disabled=false, got: %v", resp)
	}
	if resp["display_name"] != "PlayerOne" {
		t.Errorf("expected display_name from creds, got: %v", resp)
	}
}

// ─── GOG connection-handler tests ────────────────────────────────────────────

func TestGOGConnect_MissingAuthCode(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-missing")

	rec := putJSONAuth(t, app, "/api/sync/gog/connection", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGOGConnect_ExchangeFailure(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{err: fmt.Errorf("invalid code")}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-fail")

	rec := putJSONAuth(t, app, "/api/sync/gog/connection", map[string]any{"auth_code": "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestGOGConnect_FullURL verifies a GOG connect with a full login-success URL:
// the bare code is extracted and passed to ExchangeCode, and the response
// carries the resolved username.
func TestGOGConnect_FullURL(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{
		token: &api.GOGTokenResponse{
			RefreshToken: "ref",
			Username:     "goguser",
		},
	}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-url")

	rec := putJSONAuth(t, app, "/api/sync/gog/connection", map[string]any{
		"auth_code": "https://embed.gog.com/on_login_success?origin=client&code=XXX",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotCode != "XXX" {
		t.Errorf("ExchangeCode received %q, want extracted code %q", stub.gotCode, "XXX")
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["username"] != "goguser" {
		t.Errorf("username: got %q", body["username"])
	}
}

func TestGOGDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-disc")

	req := httptest.NewRequest(http.MethodDelete, "/api/sync/gog/connection", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rec.Code)
	}
}

func TestGOGConnection_NotConnected(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-status-notconn")

	req := httptest.NewRequest(http.MethodGet, "/api/sync/gog/connection", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["connected"] != false {
		t.Errorf("want connected=false, got %v", body["connected"])
	}
}

func TestGOGConnection_Connected(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{
		token: &api.GOGTokenResponse{Username: "goguser", RefreshToken: "r"},
	}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, stub)
	userID, token := setupTagUser(t, testDB, app, "gog-status-conn")

	rawCreds := `{"access_token":"a","refresh_token":"r","user_id":"u1","username":"goguser"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt gog creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'gog', 'manual', ?, now(), now())`,
		"cfg-gog-conn", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sync/gog/connection", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["connected"] != true {
		t.Errorf("want connected=true, got %v", body["connected"])
	}
	if body["username"] != "goguser" {
		t.Errorf("username: got %v", body["username"])
	}
}

// ─── Steam connection-handler tests ──────────────────────────────────────────

func TestGetSteamConnection_NotConnected(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "steam-conn-notconn")

	rec := getAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["connected"] != false {
		t.Errorf("want connected=false, got %v", body["connected"])
	}
	if body["credentials_error"] != nil {
		t.Errorf("want credentials_error absent, got %v", body["credentials_error"])
	}
}

func TestGetSteamConnection_Connected(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "steam-conn-ok")

	rawCreds := `{"web_api_key":"AABBCCDD00112233445566778899AABB","steam_id":"76561198012345678","display_name":"Frostbyte"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt steam creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'steam', 'manual', ?, now(), now())`,
		"cfg-steam-conn", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["connected"] != true {
		t.Errorf("want connected=true, got %v", body["connected"])
	}
	if body["username"] != "Frostbyte" {
		t.Errorf("username: got %v", body["username"])
	}
	if body["credentials_error"] != nil {
		t.Errorf("want credentials_error absent, got %v", body["credentials_error"])
	}
}

// TestGetConnection_DBCredentialsErrorFlag verifies that a persisted
// credentials_error=true flag is surfaced in the connection status response for
// every storefront, even when the stored credentials are otherwise valid.
func TestGetConnection_DBCredentialsErrorFlag(t *testing.T) {
	tests := []struct {
		name       string
		suffix     string
		storefront string
		path       string
		rawCreds   string
		newApp     func(t *testing.T) interface {
			ServeHTTP(http.ResponseWriter, *http.Request)
		}
	}{
		{
			name:       "steam",
			suffix:     "sc-db-cred-err",
			storefront: "steam",
			path:       "/api/sync/steam/connection",
			rawCreds:   `{"steam_id":"76561198000000001","web_api_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","display_name":"TestUser"}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			},
		},
		{
			name:       "playstation-store",
			suffix:     "psn-db-cred-err",
			storefront: "playstation-store",
			path:       "/api/sync/playstation-store/connection",
			rawCreds:   `{"npsso_token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","online_id":"MyPSN","account_id":"123","region":"GB","is_verified":true,"token_expired_at":null}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			},
		},
		{
			name:       "gog",
			suffix:     "gog-db-cred-err",
			storefront: "gog",
			path:       "/api/sync/gog/connection",
			rawCreds:   `{"access_token":"aaa","refresh_token":"bbb","user_id":"u1","username":"GogUser"}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, &stubGOGClient{})
			},
		},
		{
			name:       "epic-games-store",
			suffix:     "epic-db-cred-err",
			storefront: "epic-games-store",
			path:       "/api/sync/epic-games-store/connection",
			rawCreds:   `{"user.json":"{\"displayName\":\"EpicUser\",\"account_id\":\"abc123\"}"}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				return newSyncTestAppWithEpicGamesStore(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{}, &stubEpicGamesStoreClient{configured: true})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := tt.newApp(t)
			userID, token := setupTagUser(t, testDB, e, tt.suffix)

			ciphertext, err := testEncrypter.Encrypt([]byte(tt.rawCreds))
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			_, _ = testDB.ExecContext(context.Background(),
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
				 VALUES (?, ?, ?, 'manual', ?, true, now(), now())`,
				uuid.NewString(), userID, tt.storefront, ciphertext,
			)

			rec := getAuth(t, e, tt.path, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if resp["credentials_error"] != true {
				t.Errorf("expected credentials_error=true from DB flag, got %v", resp["credentials_error"])
			}
		})
	}
}

// TestConnect_ClearsCredentialsErrorFlag verifies a successful connect clears a
// previously-set credentials_error=true flag, for both Steam and PSN.
func TestConnect_ClearsCredentialsErrorFlag(t *testing.T) {
	tests := []struct {
		name       string
		suffix     string
		storefront string
		path       string
		body       string
		newApp     func(t *testing.T) interface {
			ServeHTTP(http.ResponseWriter, *http.Request)
		}
	}{
		{
			name:       "steam",
			suffix:     "sv-clear-cred",
			storefront: "steam",
			path:       "/api/sync/steam/connection",
			body:       `{"steam_id":"76561198000000001","web_api_key":"AABBCCDD00112233445566778899AABB"}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				stub := &stubSteamClient{
					summary: &api.SteamPlayerSummary{PersonaName: "TestUser", CommunityVisibilityState: 3},
				}
				return newSyncTestApp(t, testDB, stub, &stubPlaystationStoreClient{})
			},
		},
		{
			name:       "playstation-store",
			suffix:     "psn-clear-cred",
			storefront: "playstation-store",
			path:       "/api/sync/playstation-store/connection",
			body:       `{"npsso_token":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`,
			newApp: func(t *testing.T) interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			} {
				stub := &stubPlaystationStoreClient{
					info: &api.PlaystationStoreAccountInfo{OnlineID: "MyPSN", AccountID: "123"},
				}
				return newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := tt.newApp(t)
			userID, token := setupTagUser(t, testDB, e, tt.suffix)

			// Seed a pre-existing row with credentials_error=true.
			_, _ = testDB.ExecContext(context.Background(),
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error, created_at, updated_at)
				 VALUES (?, ?, ?, 'manual', true, now(), now())`,
				uuid.NewString(), userID, tt.storefront,
			)

			rec := putAuth(t, e, tt.path, token, tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var credsErr bool
			_ = testDB.NewRaw(
				`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = ?`,
				userID, tt.storefront,
			).Scan(context.Background(), &credsErr)
			if credsErr {
				t.Errorf("expected credentials_error=false after successful %s connect, got true", tt.storefront)
			}
		})
	}
}
