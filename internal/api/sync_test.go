package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/externalgame/egtest"
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
	synch := api.NewSyncHandler(api.SyncDeps{Encrypter: testEncrypter, DB: db, River: newTestRiverClient(t), Lifecycle: newTestLifecycle(t), Steam: steam, PlaystationStore: psn})
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
	if resp["total"].(float64) != 6 {
		t.Fatalf("expected total=6, got %v", resp["total"])
	}
	configs := resp["configs"].([]any)
	if len(configs) != 6 {
		t.Fatalf("expected 6 configs, got %d", len(configs))
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

// external_game_count counts games, not editions (ADR 0007, #1438): the sets
// of editions with an available edition.
func TestSyncStatus_CountsSetsOfEditions(t *testing.T) {
	cases := []struct {
		name string
		// setup runs after eg-a (older) and eg-b (newer), both "Astro Bot",
		// both available, are inserted.
		setup string
		want  float64
	}{
		{"two available editions", ``, 1},
		{"only the newer edition available", `UPDATE external_games SET is_available = false WHERE id = 'eg-a'`, 1},
		{"no edition available", `UPDATE external_games SET is_available = false`, 0},
		{"same title on different matches", `UPDATE external_games SET resolved_igdb_id = CASE id WHEN 'eg-a' THEN 9101 ELSE 9102 END`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			userID, token := setupTagUser(t, testDB, e, "stat-editions")
			insertTestGameWithID(t, testDB, 9101, "Astro Bot")
			insertTestGameWithID(t, testDB, 9102, "Astro Bot (2024)")
			egtest.Insert(t, testDB, egtest.Game{ID: "eg-a", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA1", Title: "Astro Bot"})
			egtest.Insert(t, testDB, egtest.Game{ID: "eg-b", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA1", Title: "Astro Bot"})
			ctx := context.Background()
			if _, err := testDB.ExecContext(ctx, `UPDATE external_games SET created_at = now() - interval '1 day' WHERE id = 'eg-a'`); err != nil {
				t.Fatal(err)
			}
			if tc.setup != "" {
				if _, err := testDB.ExecContext(ctx, tc.setup); err != nil {
					t.Fatal(err)
				}
			}

			rec := getAuth(t, e, "/api/sync/playstation-store/status", token)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d %s, want 200", rec.Code, rec.Body.String())
			}
			var status map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatalf("unmarshal status: %v", err)
			}
			if got := status["external_game_count"]; got != tc.want {
				t.Fatalf("external_game_count = %v, want %v", got, tc.want)
			}
		})
	}
}

// A status read that fails must not answer as an idle store: the store page
// would then re-enable Sync Now and treat a running sync as ended.
func TestSyncStatus_DatabaseErrorIs500(t *testing.T) {
	for _, table := range []string{"jobs", "user_sync_configs", "external_games"} {
		t.Run(table, func(t *testing.T) {
			truncateAllTables(t)
			e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			_, token := setupTagUser(t, testDB, e, "stat-err-"+table)
			ctx := context.Background()
			if _, err := testDB.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO "+table+"_r25"); err != nil {
				t.Fatal(err)
			}
			restore := func() { _, _ = testDB.ExecContext(ctx, "ALTER TABLE IF EXISTS "+table+"_r25 RENAME TO "+table) }
			t.Cleanup(restore)
			rec := getAuth(t, e, "/api/sync/steam/status", token)
			restore()
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status with %s unreadable: got %d %s, want 500", table, rec.Code, rec.Body.String())
			}
		})
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-1", UserID: userID, Storefront: "steam", ExternalID: "730", Title: "CS2"})

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

// The ignored list shows a skipped game once, whatever editions it has
// (ADR 0007, #1438): its row is the set's representative, the oldest edition
// still in the storefront library, else the oldest.
func TestIgnored_ListsASetOfEditionsOnce(t *testing.T) {
	for _, skipVia := range []string{"eg-ps4", "eg-ps5"} {
		t.Run("skip via "+skipVia, func(t *testing.T) {
			truncateAllTables(t)
			e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
			userID, token := setupTagUser(t, testDB, e, "ign-editions")
			egtest.Insert(t, testDB, egtest.Game{ID: "eg-ps4", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA1", Title: "Astro Bot"})
			egtest.Insert(t, testDB, egtest.Game{ID: "eg-ps5", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA1", Title: "Astro Bot"})
			// The PS4 edition is older but has left the library, so the PS5
			// edition represents the set.
			if _, err := testDB.ExecContext(context.Background(),
				`UPDATE external_games SET created_at = now() - interval '1 day', is_available = false WHERE id = 'eg-ps4'`,
			); err != nil {
				t.Fatal(err)
			}

			if rec := postJSONAuth(t, e, "/api/sync/ignored/"+skipVia, nil, token); rec.Code != http.StatusNoContent {
				t.Fatalf("skip: got %d %s, want 204", rec.Code, rec.Body.String())
			}

			rec := getAuth(t, e, "/api/sync/ignored", token)
			if rec.Code != http.StatusOK {
				t.Fatalf("list: got %d, want 200", rec.Code)
			}
			var resp []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal resp: %v", err)
			}
			if len(resp) != 1 || resp[0]["id"] != "eg-ps5" {
				t.Fatalf("expected one row, eg-ps5, got %v", resp)
			}
		})
	}
}

func TestSkipGame_MarksJobItemSkippedAndCompletesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-jobitem")
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-skip-ji", UserID: userID, Storefront: "steam", ExternalID: "777", Title: "Skip Me"})
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

func TestSkipGame_SkipsEveryEdition(t *testing.T) {
	// Skipping a game also skips its editions and their open job_items.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-cascade")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-skip-game", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA001", Title: "Horizon"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-skip-edition", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA001", Title: "Horizon"})
	insertJob(t, testDB, "job-skip-cascade", userID, "sync", "playstation-store", "processing")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-skip-edition', 'job-skip-cascade', ?, 'PPSA001', 'Horizon', 'eg-skip-edition', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert edition job_item: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-skip-game", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// The other edition must be skipped.
	var editionSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-skip-edition'`).Scan(ctx, &editionSkipped); err != nil {
		t.Fatalf("scan edition is_skipped: %v", err)
	}
	if !editionSkipped {
		t.Error("expected the other edition to be skipped")
	}

	// The other edition's job_item must be skipped.
	var editionItemStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-skip-edition'`).Scan(ctx, &editionItemStatus); err != nil {
		t.Fatalf("scan edition job_item status: %v", err)
	}
	if editionItemStatus != "skipped" {
		t.Errorf("expected the other edition's job_item status=skipped, got %q", editionItemStatus)
	}
}

func TestUnskipGame_UnskipsEveryEdition(t *testing.T) {
	// Unskipping a game also unskips its editions.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "unskip-cascade")

	// Two editions, both skipped.
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-unskip-game", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA002", Title: "Ratchet", Skipped: true})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-unskip-edition", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA002", Title: "Ratchet", Skipped: true})

	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip-game", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var editionSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-unskip-edition'`).Scan(context.Background(), &editionSkipped); err != nil {
		t.Fatalf("scan edition is_skipped: %v", err)
	}
	if editionSkipped {
		t.Error("expected the other edition to be unskipped")
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
	if resp["total"].(float64) != 6 {
		t.Fatalf("expected total=6, got %v", resp["total"])
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-a1", UserID: userA, Storefront: "steam", ExternalID: "730", Title: "CS2"})

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

func TestListExternalGames_OneEntryPerSetOfEditions(t *testing.T) {
	// Two editions are one entry: the oldest.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-one-entry")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-ed-1", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA001", Title: "Horizon"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-ed-2", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA001", Title: "Horizon"})

	rec := getAuth(t, e, "/api/sync/playstation-store/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result (one entry for both editions), got %d", len(resp))
	}
	if resp[0]["id"] != "eg-ed-1" {
		t.Errorf("expected the oldest edition, got id=%v", resp[0]["id"])
	}
}

func TestListExternalGames_AggregatesEditionPlatforms(t *testing.T) {
	// The entry includes the platform slugs of every edition.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-agg-platforms")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-gow-1", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA002", Title: "God of War"})
	egtest.InsertPlatform(t, testDB, "eg-gow-1", "playstation-4", 0)
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-gow-2", UserID: userID, Storefront: "playstation-store", ExternalID: "PPSA002", Title: "God of War"})
	egtest.InsertPlatform(t, testDB, "eg-gow-2", "playstation-5", 0)

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

func TestListExternalGames_IsSubscriptionFollowsOwnershipStatus(t *testing.T) {
	// is_subscription is not stored (#1495): the response derives it from
	// ownership_status, and a game without one is not a subscription.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-is-sub")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-sub", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA010", Title: "Subscribed"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-owned", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA011", Title: "Bought"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-unset", UserID: userID, Storefront: "playstation-store", ExternalID: "CUSA012", Title: "Unclassified"})
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET ownership_status = CASE id WHEN 'eg-sub' THEN 'subscription' ELSE 'owned' END
		 WHERE id IN ('eg-sub', 'eg-owned')`); err != nil {
		t.Fatalf("set ownership_status: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/playstation-store/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := make(map[any]any, len(resp))
	for _, r := range resp {
		got[r["id"]] = r["is_subscription"]
	}
	want := map[any]any{"eg-sub": true, "eg-owned": false, "eg-unset": false}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: is_subscription = %v, want %v", id, got[id], w)
		}
	}
}

func TestListExternalGames_StoreURL(t *testing.T) {
	// A row with a resolved store_link gets a computed store_url; a row without
	// one (NULL store_link) gets a null store_url.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "list-store-url")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-link", UserID: userID, Storefront: "steam", ExternalID: "2520", Title: "Alone in the Dark"})
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE external_games SET store_link = '2520' WHERE id = 'eg-link'`); err != nil {
		t.Fatalf("set store_link: %v", err)
	}
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-nolink", UserID: userID, Storefront: "steam", ExternalID: "9999", Title: "Mystery Game"})

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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-rm-a", UserID: userA, Storefront: "steam", ExternalID: "42", Title: "Some Game"})

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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-rm-1", UserID: userID, Storefront: "steam", ExternalID: "1", Title: "Game"})
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-rm-2", UserID: userID, Storefront: "steam", ExternalID: "2", Title: "Game Two"})
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-rm-3", UserID: userID, Storefront: "steam", ExternalID: "3", Title: "Game Three"})
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

func TestRematchExternalGame_ActsOnTheUsersEditionsOnly(t *testing.T) {
	// Rematching a game gives its editions the match too. A game with the same
	// title of another user is not an edition and must NOT be affected.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userA, tokenA := setupTagUser(t, testDB, e, "rm-fk-userA")
	userB, _ := setupTagUser(t, testDB, e, "rm-fk-userB")

	// User A: two editions.
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-fk-a", UserID: userA, Storefront: "playstation-store", ExternalID: "CUSA100", Title: "Spider-Man"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-fk-b", UserID: userA, Storefront: "playstation-store", ExternalID: "PPSA100", Title: "Spider-Man"})
	insertJob(t, testDB, "job-fk", userA, "sync", "playstation-store", "processing")
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-fk-b', 'job-fk', ?, 'PPSA100', 'Spider-Man', 'eg-fk-b', '{}', 'pending', '{}', '[]', now())`,
		userA,
	)
	if err != nil {
		t.Fatalf("insert the other edition's job_item: %v", err)
	}

	// User B: unrelated game with same title — must NOT be touched.
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-fk-other", UserID: userB, Storefront: "playstation-store", ExternalID: "CUSA200", Title: "Spider-Man"})

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-fk-a/rematch",
		map[string]any{"igdb_id": 4242}, tokenA)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// The other edition must have the match.
	var editionResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-b'`).Scan(ctx, &editionResolved); err != nil {
		t.Fatalf("scan edition resolved_igdb_id: %v", err)
	}
	if editionResolved == nil || *editionResolved != 4242 {
		t.Errorf("other edition's resolved_igdb_id: want 4242, got %v", editionResolved)
	}

	// User B's game must NOT have been touched.
	var otherResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-other'`).Scan(ctx, &otherResolved); err != nil {
		t.Fatalf("scan other resolved_igdb_id: %v", err)
	}
	if otherResolved != nil {
		t.Errorf("other user's game must not be resolved, got %v", *otherResolved)
	}

	// The other edition's open item: the rematch writes that edition and
	// completes the item itself, queueing no worker (#1338).
	var editionStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-fk-b'`).Scan(ctx, &editionStatus); err != nil {
		t.Fatalf("scan edition item: %v", err)
	}
	if editionStatus != "completed" {
		t.Errorf("other edition's item: want completed, got %q", editionStatus)
	}
	var riverJobs int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM river_job WHERE kind IN ('user_game_write', 'igdb_match')`).Scan(ctx, &riverJobs); err != nil {
		t.Fatalf("count river jobs: %v", err)
	}
	if riverJobs != 0 {
		t.Errorf("river jobs: want 0, got %d", riverJobs)
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-mp-1", UserID: userID, Storefront: "steam", ExternalID: "3000", Title: "Another World"})
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

// TestRematchExternalGame_EditionAlreadyMatchedWrong: rematching one edition
// of a game must not leave the other editions on the wrong match (an old bug:
// the lookup of the other editions filtered resolved_igdb_id IS NULL).
func TestRematchExternalGame_EditionAlreadyMatchedWrong(t *testing.T) {
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-aw-win", UserID: userID, Storefront: "steam", ExternalID: "2100", Title: "Another World"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-aw-lin", UserID: userID, Storefront: "steam", ExternalID: "2101", Title: "Another World"})
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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-reset-1", UserID: userID, Storefront: "steam", ExternalID: "730", Title: "CS2"})
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
	riverID := insertRiverJob(t, testDB, "igdb_match", "available", "ji-reset-active")

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
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-matched", UserID: userID, Storefront: "steam", ExternalID: "1", Title: "Matched Game"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-unmatched", UserID: userID, Storefront: "steam", ExternalID: "2", Title: "Unmatched Game"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-skipped", UserID: userID, Storefront: "steam", ExternalID: "3", Title: "Skipped Game"})

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
	for id, want := range map[string]string{"eg-matched": "matched", "eg-unmatched": "unmatched", "eg-skipped": "skipped"} {
		if got := byID[id]["sync_status"]; got != want {
			t.Errorf("%s sync_status: want %s, got %v", id, want, got)
		}
	}
}

func TestListExternalGames_ExcludesInFlight(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-inflight")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-stable", UserID: userID, Storefront: "steam", ExternalID: "10", Title: "Stable Game"})
	egtest.Insert(t, testDB, egtest.Game{ID: "eg-inflight-1", UserID: userID, Storefront: "steam", ExternalID: "20", Title: "In-Flight Game"})
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

	// A cancelled sync leaves its pending items behind; they are not being
	// worked, so the game is listed again (#1338).
	if _, err := testDB.ExecContext(context.Background(), `UPDATE jobs SET status = 'cancelled' WHERE id = 'job-inflight'`); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	rec = getAuth(t, e, "/api/sync/steam/external-games", token)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 games after the sync was cancelled, got %d", len(resp))
	}
}

func TestListExternalGames_ReturnsPlatforms(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-plat")

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-p1", UserID: userID, Storefront: "steam", ExternalID: "730", Title: "CS2"})

	egtest.InsertPlatform(t, testDB, "eg-p1", "pc-windows", 0)
	egtest.InsertPlatform(t, testDB, "eg-p1", "pc-linux", 0)

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

	egtest.Insert(t, testDB, egtest.Game{ID: "eg-np1", UserID: userID, Storefront: "steam", ExternalID: "999", Title: "Orphan Game"})

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
	synch := api.NewSyncHandler(api.SyncDeps{Encrypter: testEncrypter, DB: db, River: newTestRiverClient(t), Lifecycle: newTestLifecycle(t), Steam: steam, PlaystationStore: psn, EpicGamesStore: epic})
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

// The trigger creates the Job and its dispatch River job in one transaction
// (through the module): the Job records the dispatch as its owner, so there is
// never a Job without a dispatch.
func TestSyncTrigger_CreatesJobWithItsDispatch(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPlaystationStoreClient{})
	_, token := setupTagUser(t, testDB, e, "trigger-dispatch")

	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var job struct {
		Status           string `bun:"status"`
		Priority         string `bun:"priority"`
		DispatchComplete bool   `bun:"dispatch_complete"`
		RiverJobID       *int64 `bun:"river_job_id"`
	}
	if err := testDB.NewRaw(`SELECT status, priority, dispatch_complete, river_job_id FROM jobs WHERE id = ?`, resp.JobID).
		Scan(context.Background(), &job); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if job.Status != "pending" || job.Priority != "high" || job.DispatchComplete || job.RiverJobID == nil {
		t.Fatalf("job = %+v, want pending, high, dispatch not done, with an owner", job)
	}
	var kind, jobArg string
	if err := testDB.NewRaw(`SELECT kind, args->>'job_id' FROM river_job WHERE id = ?`, *job.RiverJobID).
		Scan(context.Background(), &kind, &jobArg); err != nil {
		t.Fatalf("load dispatch: %v", err)
	}
	if kind != "dispatch_sync" || jobArg != resp.JobID {
		t.Errorf("dispatch = %s for %s, want dispatch_sync for %s", kind, jobArg, resp.JobID)
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
	synch := api.NewSyncHandler(api.SyncDeps{Encrypter: testEncrypter, DB: db, River: newTestRiverClient(t), Lifecycle: newTestLifecycle(t), Steam: steam, PlaystationStore: psn, GOG: gog})
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

// ─── Amazon Games connection-handler tests ───────────────────────────────────

type stubAmazonGamesClient struct {
	configured  bool
	loginErr    error
	logins      int
	registerErr error
	gotCode     string
	gotLogin    api.AmazonGamesLogin
	userData    json.RawMessage
	during      func() // runs inside Register, as if nile were still working

	logoutMu     sync.Mutex
	logouts      []string // user data each Logout was given
	logoutErr    error
	logoutHold   chan struct{} // when set, Logout waits for it to close (or its ctx to end)
	logoutCtxErr error         // ctx.Err() when a held Logout returned
	logoutBegan  chan struct{} // when set, closed as Logout starts
	logoutEnded  chan struct{} // when set, closed as Logout returns
}

func (s *stubAmazonGamesClient) Logout(ctx context.Context, userData json.RawMessage) error {
	if s.logoutBegan != nil {
		close(s.logoutBegan)
	}
	if s.logoutEnded != nil {
		defer close(s.logoutEnded)
	}
	s.logoutMu.Lock()
	s.logouts = append(s.logouts, string(userData))
	hold, err := s.logoutHold, s.logoutErr
	s.logoutMu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
		}
		s.logoutMu.Lock()
		s.logoutCtxErr = ctx.Err()
		s.logoutMu.Unlock()
	}
	return err
}

func (s *stubAmazonGamesClient) loggedOut() []string {
	s.logoutMu.Lock()
	defer s.logoutMu.Unlock()
	return append([]string(nil), s.logouts...)
}

func (s *stubAmazonGamesClient) Configured() bool { return s.configured }

func (s *stubAmazonGamesClient) LoginURL(_ context.Context) (*api.AmazonGamesLogin, error) {
	if s.loginErr != nil {
		return nil, s.loginErr
	}
	s.logins++
	n := fmt.Sprint(s.logins)
	return &api.AmazonGamesLogin{
		URL:          "https://amazon.com/ap/signin?attempt=" + n,
		ClientID:     "client-" + n,
		CodeVerifier: "verifier-" + n,
		Serial:       "serial-" + n,
	}, nil
}

func (s *stubAmazonGamesClient) Register(_ context.Context, code string, login api.AmazonGamesLogin) (*api.AmazonGamesAccount, json.RawMessage, error) {
	s.gotCode = code
	s.gotLogin = login
	if s.during != nil {
		s.during()
	}
	if s.registerErr != nil {
		return nil, nil, s.registerErr
	}
	return &api.AmazonGamesAccount{UserID: "amzn1.account.X", Name: "Ama"}, s.userData, nil
}

const amazonTestUserData = `{"tokens":{"bearer":{"refresh_token":"Atnr|R"}},"extensions":{"customer_info":{"user_id":"amzn1.account.X","given_name":"Ama"}}}`

func newSyncTestAppWithAmazonGames(t *testing.T, amazon *stubAmazonGamesClient) (interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, *api.SyncHandler) {
	t.Helper()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, testCfg())
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(api.SyncDeps{Encrypter: testEncrypter, DB: testDB, River: newTestRiverClient(t), Lifecycle: newTestLifecycle(t), Steam: &stubSteamClient{}, PlaystationStore: &stubPlaystationStoreClient{}, AmazonGames: amazon})
	g := e.Group("/api/sync", auth.AuthMiddleware(testDB))
	synch.RegisterRoutes(g)
	return e, synch
}

// startAmazonLogin starts a sign-in and returns its attempt id.
func startAmazonLogin(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, token string) string {
	t.Helper()
	rec := postJSONAuth(t, e, "/api/sync/amazon-games/connection/login", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["attempt_id"] == "" {
		t.Fatalf("start: no attempt_id in %v", resp)
	}
	return resp["attempt_id"]
}

const amazonLanding = "https://www.amazon.com/?openid.oa2.authorization_code=ANcode1&openid.mode=id_res"

func TestAmazonGamesLoginStart_ReturnsURLAndKeepsSecretsServerSide(t *testing.T) {
	truncateAllTables(t)
	e, _ := newSyncTestAppWithAmazonGames(t, &stubAmazonGamesClient{configured: true})
	_, token := setupTagUser(t, testDB, e, "amz-start")

	rec := postJSONAuth(t, e, "/api/sync/amazon-games/connection/login", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["login_url"] != "https://amazon.com/ap/signin?attempt=1" || resp["attempt_id"] == "" {
		t.Errorf("unexpected response: %v", resp)
	}
	for _, secret := range []string{"client-1", "verifier-1", "serial-1"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("response leaks %q: %s", secret, rec.Body.String())
		}
	}
}

func TestAmazonGamesLoginStart_503WhenNileMissing(t *testing.T) {
	truncateAllTables(t)
	e, _ := newSyncTestAppWithAmazonGames(t, &stubAmazonGamesClient{configured: false})
	_, token := setupTagUser(t, testDB, e, "amz-start-503")
	rec := postJSONAuth(t, e, "/api/sync/amazon-games/connection/login", nil, token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAmazonGamesConnect_FinishPersistsEncryptedUserData(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-finish")

	attempt := startAmazonLogin(t, e, token)
	rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["username"] != "Ama" {
		t.Errorf("unexpected response: %v", resp)
	}
	if stub.gotCode != "ANcode1" || stub.gotLogin.ClientID != "client-1" || stub.gotLogin.CodeVerifier != "verifier-1" || stub.gotLogin.Serial != "serial-1" {
		t.Errorf("register got code=%q login=%+v", stub.gotCode, stub.gotLogin)
	}

	var credsRaw string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games'`, userID).
		Scan(context.Background(), &credsRaw); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if strings.Contains(credsRaw, "Atnr|R") {
		t.Fatal("credentials stored in the clear")
	}
	plain, err := testEncrypter.Decrypt(credsRaw)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plain) != amazonTestUserData {
		t.Errorf("stored %s, want nile's user data", plain)
	}

}

func TestAmazonGamesConnect_BareCodeAccepted(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	_, token := setupTagUser(t, testDB, e, "amz-bare")
	attempt := startAmazonLogin(t, e, token)
	rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": "ANbare"}, token)
	if rec.Code != http.StatusOK || stub.gotCode != "ANbare" {
		t.Errorf("expected 200 with code ANbare, got %d code=%q: %s", rec.Code, stub.gotCode, rec.Body.String())
	}
}

func TestAmazonGamesConnect_400WithoutCode(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	_, token := setupTagUser(t, testDB, e, "amz-nocode")
	attempt := startAmazonLogin(t, e, token)
	for _, landing := range []string{"https://www.amazon.com/?openid.mode=cancel", ""} {
		rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": landing}, token)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("landing %q: expected 400, got %d: %s", landing, rec.Code, rec.Body.String())
		}
	}
	if stub.gotCode != "" {
		t.Error("register ran without a code")
	}
}

// amazonAttemptPayload builds an attempt_id the way the server does, so a test
// can hand the finish an attempt whose fields it controls.
func amazonAttemptPayload(t *testing.T, fields map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := testEncrypter.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	return ct
}

func validAmazonAttemptFields(userID string) map[string]any {
	return map[string]any{
		"kind":          "amazon-games-login",
		"client_id":     "client-x",
		"code_verifier": "verifier-x",
		"serial":        "serial-x",
		"user_id":       userID,
		"expires_at":    time.Now().Add(10 * time.Minute),
	}
}

// Every attempt_id the finish cannot trust is a 409, nile never runs, and
// nothing is stored.
func TestAmazonGamesConnect_409ForUnusableAttempt(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-409")
	otherID, _ := setupTagUser(t, testDB, e, "amz-409-other")

	real := startAmazonLogin(t, e, token)
	// One base64 character flipped in the middle of the attempt.
	mid := len(real) / 2
	flip := byte('A')
	if real[mid] == 'A' {
		flip = 'B'
	}
	tampered := real[:mid] + string(flip) + real[mid+1:]
	storedCredential, err := testEncrypter.Encrypt([]byte(amazonTestUserData))
	if err != nil {
		t.Fatal(err)
	}
	with := func(key string, value any) string {
		f := validAmazonAttemptFields(userID)
		f[key] = value
		return amazonAttemptPayload(t, f)
	}
	without := func(key string) string {
		f := validAmazonAttemptFields(userID)
		delete(f, key)
		return amazonAttemptPayload(t, f)
	}

	for _, tc := range []struct{ name, attempt string }{
		{"garbage", "no-such-attempt"},
		{"over the length cap, otherwise valid", with("client_id", strings.Repeat("c", 2100))},
		{"tampered", tampered},
		{"another ciphertext from the same key (a stored credential)", storedCredential},
		{"wrong kind", with("kind", "gog-login")},
		{"another user's", with("user_id", otherID)},
		{"expired", with("expires_at", time.Now().Add(-time.Second))},
		{"no client id", with("client_id", "")},
		{"no code verifier", without("code_verifier")},
		{"no serial", with("serial", "")},
		{"no expiry", without("expires_at")},
	} {
		rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": tc.attempt, "redirect_url": amazonLanding}, token)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: expected 409, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	if stub.gotCode != "" {
		t.Error("register ran for an unusable attempt")
	}
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_sync_configs WHERE storefront = 'amazon-games' AND storefront_credentials IS NOT NULL`).
		Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d sign-ins stored from unusable attempts", n)
	}

	// The control: the same fields, unaltered, finish, and a real attempt_id
	// fits well inside the length cap.
	if len(real) > 1024 {
		t.Errorf("a real attempt_id is %d characters; the cap assumes about 450", len(real))
	}
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": amazonAttemptPayload(t, validAmazonAttemptFields(userID)), "redirect_url": amazonLanding}, token); rec.Code != http.StatusOK {
		t.Errorf("a well-formed attempt: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// An attempt expires on the server's clock: fifteen minutes after it started.
func TestAmazonGamesConnect_AttemptExpiresAfterFifteenMinutes(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	e, h := newSyncTestAppWithAmazonGames(t, stub)
	_, token := setupTagUser(t, testDB, e, "amz-expiry")
	attempt := startAmazonLogin(t, e, token)

	h.SetAmazonLoginClockForTest(func() time.Time { return time.Now().Add(16 * time.Minute) })
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token); rec.Code != http.StatusConflict {
		t.Errorf("after 16 minutes: expected 409, got %d", rec.Code)
	}
	h.SetAmazonLoginClockForTest(func() time.Time { return time.Now().Add(14 * time.Minute) })
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token); rec.Code != http.StatusOK {
		t.Errorf("after 14 minutes: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A sign-in started on one server finishes on another, or after a restart:
// everything the finish needs travels in the attempt_id.
func TestAmazonGamesConnect_SurvivesRestart(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	first, _ := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, first, "amz-restart")
	attempt := startAmazonLogin(t, first, token)

	restarted, _ := newSyncTestAppWithAmazonGames(t, stub)
	rec := putJSONAuth(t, restarted, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("finishing on a new handler: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotLogin.ClientID != "client-1" || stub.gotLogin.CodeVerifier != "verifier-1" || stub.gotLogin.Serial != "serial-1" {
		t.Errorf("register got login=%+v, want the started attempt's values", stub.gotLogin)
	}
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games' AND storefront_credentials IS NOT NULL`, userID).
		Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("stored sign-ins = %d, want 1", n)
	}
}

// Two attempts started by one user can both finish; the last one stored wins.
func TestAmazonGamesConnect_LastFinishWins(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-last")
	firstAttempt := startAmazonLogin(t, e, token)
	secondAttempt := startAmazonLogin(t, e, token)

	second := strings.Replace(amazonTestUserData, "Atnr|R", "Atnr|second", 1)
	stub.userData = json.RawMessage(second)
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": secondAttempt, "redirect_url": amazonLanding}, token); rec.Code != http.StatusOK {
		t.Fatalf("second attempt: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	firstData := strings.Replace(amazonTestUserData, "Atnr|R", "Atnr|first", 1)
	stub.userData = json.RawMessage(firstData)
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": firstAttempt, "redirect_url": amazonLanding}, token); rec.Code != http.StatusOK {
		t.Fatalf("first attempt, finished last: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.gotLogin.ClientID != "client-1" {
		t.Errorf("the last finish registered %q, want client-1", stub.gotLogin.ClientID)
	}
	var credsRaw string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games'`, userID).
		Scan(context.Background(), &credsRaw); err != nil {
		t.Fatal(err)
	}
	plain, err := testEncrypter.Decrypt(credsRaw)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != firstData {
		t.Errorf("stored the earlier finish; want the last one")
	}
}

func TestAmazonGamesConnect_AttemptsAreUserScoped(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, userData: json.RawMessage(amazonTestUserData)}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	_, tokenA := setupTagUser(t, testDB, e, "amz-user-a")
	_, tokenB := setupTagUser(t, testDB, e, "amz-user-b")
	attemptA := startAmazonLogin(t, e, tokenA)
	rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attemptA, "redirect_url": amazonLanding}, tokenB)
	if rec.Code != http.StatusConflict {
		t.Errorf("another user's attempt: expected 409, got %d", rec.Code)
	}
}

func TestAmazonGamesConnect_RegisterFailure(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true, registerErr: fmt.Errorf("amazon: device registration failed: [redacted]")}
	e, _ := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-regfail")
	attempt := startAmazonLogin(t, e, token)
	rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games' AND storefront_credentials IS NOT NULL`, userID).
		Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("a failed registration stored credentials")
	}
	// The attempt survives a failed paste, so the user can paste again.
	stub.registerErr = nil
	stub.userData = json.RawMessage(amazonTestUserData)
	rec = putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token)
	if rec.Code != http.StatusOK {
		t.Errorf("retry on the same attempt: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func seedAmazonConnection(t *testing.T, userID string, credentialsError bool) {
	t.Helper()
	ct, err := testEncrypter.Encrypt([]byte(amazonTestUserData))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'amazon-games', 'manual', ?, ?, now(), now())`,
		uuid.NewString(), userID, ct, credentialsError,
	).Exec(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestGetAmazonGamesConnection(t *testing.T) {
	truncateAllTables(t)
	e, _ := newSyncTestAppWithAmazonGames(t, &stubAmazonGamesClient{configured: true})
	userID, token := setupTagUser(t, testDB, e, "amz-status")
	get := func() map[string]any {
		rec := getAuth(t, e, "/api/sync/amazon-games/connection", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp
	}
	if resp := get(); resp["connected"] != false || resp["disabled"] != false {
		t.Errorf("not connected: unexpected %v", resp)
	}
	seedAmazonConnection(t, userID, true)
	resp := get()
	if resp["connected"] != true || resp["username"] != "Ama" || resp["credentials_error"] != true {
		t.Errorf("connected with credentials error: unexpected %v", resp)
	}
}

func TestGetAmazonGamesConnection_DisabledWhenNileMissing(t *testing.T) {
	truncateAllTables(t)
	e, _ := newSyncTestAppWithAmazonGames(t, &stubAmazonGamesClient{configured: false})
	_, token := setupTagUser(t, testDB, e, "amz-status-disabled")
	rec := getAuth(t, e, "/api/sync/amazon-games/connection", token)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != false || resp["disabled"] != true || resp["reason"] != "nile_not_installed" {
		t.Errorf("unexpected response: %v", resp)
	}
}

// Without nile a stored sign-in is still reported, so the page can offer to
// remove it.
func TestGetAmazonGamesConnection_DisabledStillReportsStoredSignIn(t *testing.T) {
	truncateAllTables(t)
	e, _ := newSyncTestAppWithAmazonGames(t, &stubAmazonGamesClient{configured: false})
	userID, token := setupTagUser(t, testDB, e, "amz-status-disabled-stored")
	seedAmazonConnection(t, userID, false)
	rec := getAuth(t, e, "/api/sync/amazon-games/connection", token)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != true || resp["disabled"] != true || resp["reason"] != "nile_not_installed" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func storedAmazonUserData(t *testing.T, userID string) *string {
	t.Helper()
	var creds *string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games'`, userID).
		Scan(context.Background(), &creds); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if creds == nil {
		return nil
	}
	plain, err := testEncrypter.Decrypt(*creds)
	if err != nil {
		t.Fatal(err)
	}
	out := string(plain)
	return &out
}

// Disconnecting removes the stored sign-in and answers 204 as before, then
// signs that device out of Amazon with the sign-in it removed. Whether the
// sign-out works, fails or hangs past its limit, the disconnect has succeeded.
func TestAmazonGamesDisconnect_SignsTheDeviceOutBestEffort(t *testing.T) {
	for _, tc := range []struct {
		name      string
		logoutErr error
		hang      bool
	}{
		{"sign-out works", nil, false},
		{"sign-out fails", fmt.Errorf("amazon: nile could not deregister the device: [redacted]"), false},
		{"sign-out hangs past its limit", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			stub := &stubAmazonGamesClient{configured: true, logoutErr: tc.logoutErr}
			if tc.hang {
				stub.logoutHold = make(chan struct{}) // never closed
				stub.logoutEnded = make(chan struct{})
			}
			e, h := newSyncTestAppWithAmazonGames(t, stub)
			h.SetAmazonSignOutTimeoutForTest(100 * time.Millisecond)
			userID, token := setupTagUser(t, testDB, e, "amz-disc")
			seedAmazonConnection(t, userID, false)

			rec := deleteAuth(t, e, "/api/sync/amazon-games/connection", token)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
			if tc.hang {
				select {
				case <-stub.logoutEnded:
					t.Error("the disconnect waited for the hung sign-out before answering")
				default:
				}
			}
			if storedAmazonUserData(t, userID) != nil {
				t.Error("credentials not cleared")
			}
			h.WaitForAmazonSignOutsForTest()
			if got := stub.loggedOut(); len(got) != 1 || got[0] != amazonTestUserData {
				t.Errorf("sign-outs = %v, want one with the removed sign-in", got)
			}
			if tc.hang && !errors.Is(stub.logoutCtxErr, context.DeadlineExceeded) {
				t.Errorf("hung sign-out ended with %v, want the time limit", stub.logoutCtxErr)
			}
		})
	}
}

// A store with nothing stored, or a server without nile, disconnects without
// a sign-out.
func TestAmazonGamesDisconnect_NoSignOutWithoutSignInOrNile(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured bool
		seed       bool
	}{
		{"nothing stored", true, false},
		{"nile not installed", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			stub := &stubAmazonGamesClient{configured: tc.configured}
			e, h := newSyncTestAppWithAmazonGames(t, stub)
			userID, token := setupTagUser(t, testDB, e, "amz-disc-none")
			if tc.seed {
				seedAmazonConnection(t, userID, false)
			}
			if rec := deleteAuth(t, e, "/api/sync/amazon-games/connection", token); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
			h.WaitForAmazonSignOutsForTest()
			if storedAmazonUserData(t, userID) != nil {
				t.Error("credentials not cleared")
			}
			if got := stub.loggedOut(); len(got) != 0 {
				t.Errorf("signed out %d times, want none", len(got))
			}
		})
	}
}

// A disconnect whose removal fails signs nothing out: the connection is still
// stored and must stay usable, not revoked while the page shows it connected.
func TestAmazonGamesDisconnect_FailedRemovalSignsNothingOut(t *testing.T) {
	truncateAllTables(t)
	stub := &stubAmazonGamesClient{configured: true}
	e, h := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-disc-fail")
	seedAmazonConnection(t, userID, false)
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `CREATE FUNCTION r25_refuse_update() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''forced failure''; END'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(ctx, `DROP TRIGGER IF EXISTS r25_refuse_update ON user_sync_configs`)
		_, _ = testDB.ExecContext(ctx, `DROP FUNCTION IF EXISTS r25_refuse_update()`)
	})
	if _, err := testDB.ExecContext(ctx, `CREATE TRIGGER r25_refuse_update BEFORE UPDATE ON user_sync_configs FOR EACH ROW EXECUTE FUNCTION r25_refuse_update()`); err != nil {
		t.Fatal(err)
	}

	rec := deleteAuth(t, e, "/api/sync/amazon-games/connection", token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	h.WaitForAmazonSignOutsForTest()
	if got := stub.loggedOut(); len(got) != 0 {
		t.Errorf("signed out %d times after a failed removal, want none", len(got))
	}
	if storedAmazonUserData(t, userID) == nil {
		t.Error("the connection is gone after a failed removal")
	}
}

// A reconnect stored while the sign-out still runs is untouched: the sign-out
// uses only the sign-in the disconnect removed.
func TestAmazonGamesDisconnect_ReconnectDuringSignOutIsKept(t *testing.T) {
	truncateAllTables(t)
	newer := strings.Replace(amazonTestUserData, "Atnr|R", "Atnr|newer", 1)
	stub := &stubAmazonGamesClient{
		configured: true, userData: json.RawMessage(newer),
		logoutHold: make(chan struct{}), logoutBegan: make(chan struct{}), logoutEnded: make(chan struct{}),
	}
	e, h := newSyncTestAppWithAmazonGames(t, stub)
	userID, token := setupTagUser(t, testDB, e, "amz-disc-reconnect")
	seedAmazonConnection(t, userID, false)
	stillRunning := func(when string) {
		t.Helper()
		select {
		case <-stub.logoutEnded:
			t.Fatalf("%s: the sign-out had already finished", when)
		default:
		}
	}

	if rec := deleteAuth(t, e, "/api/sync/amazon-games/connection", token); rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case <-stub.logoutBegan:
	case <-time.After(5 * time.Second):
		t.Fatal("the sign-out never started")
	}
	stillRunning("after the disconnect answered")
	attempt := startAmazonLogin(t, e, token)
	if rec := putJSONAuth(t, e, "/api/sync/amazon-games/connection", map[string]any{"attempt_id": attempt, "redirect_url": amazonLanding}, token); rec.Code != http.StatusOK {
		t.Fatalf("reconnect: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	stillRunning("after the reconnect committed")
	close(stub.logoutHold)
	h.WaitForAmazonSignOutsForTest()

	if got := stub.loggedOut(); len(got) != 1 || got[0] != amazonTestUserData {
		t.Errorf("sign-outs = %v, want one with the removed (older) sign-in", got)
	}
	if got := storedAmazonUserData(t, userID); got == nil || *got != newer {
		t.Error("the reconnected sign-in was not kept")
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
