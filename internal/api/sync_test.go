package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/auth"
)

type stubSteamClient struct {
	summary *api.SteamPlayerSummary
	err     error
}

func (s *stubSteamClient) GetPlayerSummaries(_ context.Context, _, _ string) (*api.SteamPlayerSummary, error) {
	return s.summary, s.err
}

type stubPSNClient struct {
	info *api.PSNAccountInfo
	err  error
}

func (s *stubPSNClient) GetAccountInfo(_ context.Context, _ string) (*api.PSNAccountInfo, error) {
	return s.info, s.err
}

func newSyncTestApp(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(db, nil, steam, psn)
	g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
	synch.RegisterRoutes(g)
	return e
}

// ─── Sync config tests ────────────────────────────────────────────────────────

func TestSyncConfig_ListDefaults(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-list-1")

	rec := getAuth(t, e, "/api/sync/config", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 3 {
		t.Fatalf("expected total=3, got %v", resp["total"])
	}
	configs := resp["configs"].([]any)
	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-put-1")

	rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
		"frequency": "daily",
		"auto_add":  true,
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
	if resp["auto_add"].(bool) != true {
		t.Fatalf("expected auto_add=true")
	}
}

func TestSyncConfig_Put_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-invalid-1")

	rec := putJSONAuth(t, e, "/api/sync/config/gog", map[string]any{"frequency": "daily"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSyncConfig_Put_EpicAllowed(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "cfg-epic-1")

	rec := putJSONAuth(t, e, "/api/sync/config/epic", map[string]any{"frequency": "weekly"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for epic config, got %d", rec.Code)
	}
}

// ─── Sync trigger and status tests ───────────────────────────────────────────

func TestSyncTrigger_CreatesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-1")

	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["storefront"] != "steam" {
		t.Fatalf("expected storefront=steam, got %v", resp["storefront"])
	}
	if resp["status"] != "queued" {
		t.Fatalf("expected status=queued, got %v", resp["status"])
	}
	if resp["job_id"] == nil {
		t.Fatal("expected job_id")
	}
}

func TestSyncTrigger_EpicReturns400(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-epic-1")

	rec := postJSONAuth(t, e, "/api/sync/epic", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for epic trigger, got %d", rec.Code)
	}
}

func TestSyncTrigger_DuplicateReturns409(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-dup-1")

	postJSONAuth(t, e, "/api/sync/steam", nil, token)
	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate, got %d", rec.Code)
	}
}

func TestSyncStatus_ReflectsActiveJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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

func TestSteamVerify_BadSteamID(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "sv-bad-id")

	rec := postJSONAuth(t, e, "/api/sync/steam/verify", map[string]any{
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

func TestSteamVerify_BadAPIKey(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "sv-bad-key")

	rec := postJSONAuth(t, e, "/api/sync/steam/verify", map[string]any{
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

func TestSteamVerify_StubSuccess(t *testing.T) {
	truncateAllTables(t)
	stub := &stubSteamClient{
		summary: &api.SteamPlayerSummary{PersonaName: "Frostbyte", CommunityVisibilityState: 3},
	}
	e := newSyncTestApp(t, testDB, stub, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "sv-ok")

	rec := postJSONAuth(t, e, "/api/sync/steam/verify", map[string]any{
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
}

func TestSteamDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "sd-1")

	rec := deleteAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 even with no row, got %d", rec.Code)
	}
}

// --- PSN tests ---------------------------------------------------------------

func TestPSNConfigure_ShortToken(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "psn-short")

	rec := postJSONAuth(t, e, "/api/sync/psn/configure", map[string]any{
		"npsso_token": "tooshort",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPSNConfigure_StubSuccess(t *testing.T) {
	truncateAllTables(t)
	stub := &stubPSNClient{
		info: &api.PSNAccountInfo{OnlineID: "MyPSNName", AccountID: "123456", Region: "GB"},
	}
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "psn-ok")

	token64 := strings.Repeat("a", 64)
	rec := postJSONAuth(t, e, "/api/sync/psn/configure", map[string]any{
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
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'psn'`, userID).
		Scan(context.Background(), &creds); err != nil {
		t.Fatalf("scan credentials: %v", err)
	}
	if creds == "" {
		t.Fatal("expected credentials stored")
	}
}

func TestPSNStatus_NoRow(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "psn-stat-empty")

	rec := getAuth(t, e, "/api/sync/psn/connection", token)
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
	if resp["token_expired"].(bool) {
		t.Fatal("expected token_expired=false")
	}
	if resp["online_id"] != "" {
		t.Fatalf("expected online_id='', got %v", resp["online_id"])
	}
}

func TestPSNDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "psn-disc-1")

	rec := deleteAuth(t, e, "/api/sync/psn/disconnect", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

// ─── Ignored / skip / unskip tests ───────────────────────────────────────────

func insertExternalGame(t *testing.T, db *bun.DB, id, userID, storefront, extID, title string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, 0, now(), now())`,
		id, userID, storefront, extID, title,
	)
	if err != nil {
		t.Fatalf("insertExternalGame: %v", err)
	}
}

func TestIgnored_EmptyList(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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

func TestIgnored_404ForUnknown(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "ign-404")

	rec := postJSONAuth(t, e, "/api/sync/ignored/nonexistent", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ─── TestHandleGetConfig ──────────────────────────────────────────────────────

func TestSyncListConfig_AfterPut(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "listcfg-afterput")

	// Create a steam config.
	rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
		"frequency": "daily",
		"auto_add":  false,
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
	if resp["total"].(float64) != 3 {
		t.Fatalf("expected total=3, got %v", resp["total"])
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "getcfg-afterput")

	// Create a config first.
	rec := putJSONAuth(t, e, "/api/sync/config/psn", map[string]any{
		"frequency": "weekly",
		"auto_add":  false,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Now GET the same config.
	rec = getAuth(t, e, "/api/sync/config/psn", token)
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "getcfg-invalid")

	rec := getAuth(t, e, "/api/sync/config/gog", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── TestHandleGetPSNStatus with credentials ──────────────────────────────────

func TestPSNStatus_WithCredentials(t *testing.T) {
	truncateAllTables(t)
	stub := &stubPSNClient{
		info: &api.PSNAccountInfo{OnlineID: "MyPSNName", AccountID: "123456", Region: "GB"},
	}
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "psn-stat-cred")

	// Configure PSN first to store credentials.
	token64 := strings.Repeat("b", 64)
	rec := postJSONAuth(t, e, "/api/sync/psn/configure", map[string]any{
		"npsso_token": token64,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("configure expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Now get PSN status — should return configured=true.
	rec = getAuth(t, e, "/api/sync/psn/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp["is_configured"].(bool) {
		t.Error("expected is_configured=true after PSN configure")
	}
}

// ─── TestListExternalGames ────────────────────────────────────────────────────

func TestListExternalGames_EmptyList(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "eg-bad-sf")

	rec := getAuth(t, e, "/api/sync/notaplatform/external-games", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListExternalGames_IsolatedByUser(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "rm-404")

	rec := postJSONAuth(t, e, "/api/sync/external-games/nonexistent/rematch",
		map[string]any{"igdb_id": 1234}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRematchExternalGame_OtherUsersGame(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-keep")
	insertExternalGame(t, testDB, "eg-rm-2", userID, "steam", "2", "Game Two")
	insertUserGameAndPlatform(t, testDB, "ug-rm-2", userID, "2222", "ugp-rm-2", "eg-rm-2")

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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-remove")
	insertExternalGame(t, testDB, "eg-rm-3", userID, "steam", "3", "Game Three")
	insertUserGameAndPlatform(t, testDB, "ug-rm-3", userID, "3333", "ugp-rm-3", "eg-rm-3")

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

// ─── TestUnskipGame_EnqueuesJobItem ───────────────────────────────────────────

func TestUnskipGame_EnqueuesJobItem(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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

// ── TestResetSyncData ─────────────────────────────────────────────────────────

func TestResetSyncData_DeletesDataAndResetsTimestamp(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "reset-data")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, 'steam', 'manual', false, now(), now(), now())`,
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "reset-invalid")

	rec := deleteAuth(t, e, "/api/sync/fakefront/data", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResetSyncData_Unauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})

	req := httptest.NewRequest(http.MethodDelete, "/api/sync/steam/data", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestResetSyncData_EmptyState(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "reset-empty")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on empty reset (idempotent), got %d", rec.Code)
	}
}

func TestListExternalGames_AllStates(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
