package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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

	rec := getAuth(t, e, "/api/sync/psn/status", token)
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
	_, err := testDB.ExecContext(context.Background(),
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
	rec = getAuth(t, e, "/api/sync/psn/status", token)
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
