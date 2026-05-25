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
	synch := api.NewSyncHandler(testEncrypter, db, nil, steam, psn, (api.EpicClient)(nil), (api.GOGClient)(nil))
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
	if resp["total"].(float64) != 4 {
		t.Fatalf("expected total=4, got %v", resp["total"])
	}
	configs := resp["configs"].([]any)
	if len(configs) != 4 {
		t.Fatalf("expected 4 configs, got %d", len(configs))
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

	rec := putJSONAuth(t, e, "/api/sync/config/battlenet", map[string]any{"frequency": "daily"}, token)
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

func TestSyncTrigger_EpicCreatesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-epic-1")

	rec := postJSONAuth(t, e, "/api/sync/epic", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for epic trigger, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["storefront"] != "epic" {
		t.Fatalf("expected storefront=epic, got %v", resp["storefront"])
	}
	if resp["status"] != "queued" {
		t.Fatalf("expected status=queued, got %v", resp["status"])
	}
	if resp["job_id"] == nil {
		t.Fatal("expected job_id")
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
	if resp["credentials_error"] != nil {
		t.Fatalf("expected credentials_error absent/null, got %v", resp["credentials_error"])
	}
	if resp["online_id"] != nil {
		t.Fatalf("expected online_id absent/null, got %v", resp["online_id"])
	}
}

func TestPSNDisconnect_Idempotent(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "psn-disc-1")

	rec := deleteAuth(t, e, "/api/sync/psn/connection", token)
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

func TestSkipGame_MarksJobItemSkippedAndCompletesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	if resp["total"].(float64) != 4 {
		t.Fatalf("expected total=4, got %v", resp["total"])
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

	rec := getAuth(t, e, "/api/sync/config/battlenet", token)
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
	userID, token := setupTagUser(t, testDB, e, "psn-stat-cred")

	rawCreds := `{"npsso_token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","online_id":"MyPSNName","account_id":"123456","region":"GB","is_verified":true,"token_expired_at":null}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt psn creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'manual', ?, now(), now())`,
		"cfg-psn-cred", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	// Now get PSN status — should return configured=true.
	rec := getAuth(t, e, "/api/sync/psn/connection", token)
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

func TestPSNStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "psn-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "psn")

	// Decrypt failure must surface credentials_error=true without clearing the row.
	rec := getAuth(t, e, "/api/sync/psn/connection", token)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for undecryptable PSN credentials", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["is_configured"] != true {
		t.Errorf("expected is_configured=true (row still exists), got %v", resp["is_configured"])
	}
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true, got %v", resp["credentials_error"])
	}

	// Credentials row must NOT be cleared.
	var creds string
	err := testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'psn'`, userID,
	).Scan(context.Background(), &creds)
	if err != nil {
		t.Fatalf("credentials row missing after decrypt failure: %v", err)
	}
	if creds == "" {
		t.Error("expected credentials to remain non-null after decrypt failure")
	}
}

func TestEpicStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, &stubEpicClient{configured: true})
	userID, token := setupTagUser(t, testDB, e, "epic-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "epic")

	// Decrypt failure must surface credentials_error=true without clearing the row.
	rec := getAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for undecryptable Epic credentials", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["connected"] != true {
		t.Errorf("expected connected=true (row still exists), got %v", resp["connected"])
	}
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true, got %v", resp["credentials_error"])
	}

	// Credentials row must NOT be cleared.
	var creds string
	err := testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`, userID,
	).Scan(context.Background(), &creds)
	if err != nil {
		t.Fatalf("credentials row missing after decrypt failure: %v", err)
	}
	if creds == "" {
		t.Error("expected credentials to remain non-null after decrypt failure")
	}
}

func TestGOGStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, &stubGOGClient{})
	userID, token := setupTagUser(t, testDB, e, "gog-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "gog")

	// Decrypt failure must surface credentials_error=true without clearing the row.
	rec := getAuth(t, e, "/api/sync/gog/connection", token)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for undecryptable GOG credentials", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["connected"] != true {
		t.Errorf("expected connected=true (row still exists), got %v", resp["connected"])
	}
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true, got %v", resp["credentials_error"])
	}

	// Credentials row must NOT be cleared.
	var creds string
	err := testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'gog'`, userID,
	).Scan(context.Background(), &creds)
	if err != nil {
		t.Fatalf("credentials row missing after decrypt failure: %v", err)
	}
	if creds == "" {
		t.Error("expected credentials to remain non-null after decrypt failure")
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-siblings")

	// Two PSN external_games for the same game title (PS4 + PS5 variants).
	insertExternalGame(t, testDB, "eg-ps4", userID, "psn", "PPSA-001", "Spider-Man 2")
	insertExternalGame(t, testDB, "eg-ps5", userID, "psn", "PPSA-002", "Spider-Man 2")

	// A recent sync job for the sibling fallback path.
	insertJob(t, testDB, "job-sib", userID, "sync", "psn", "processing")

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

func TestListExternalGames_ExcludesInFlight(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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

// ─── Epic connection-handler tests ────────────────────────────────────────────

type stubEpicClient struct {
	info       *api.EpicAccountInfo
	snapshot   map[string]string
	authErr    error
	cleanupErr error
	configured bool

	authCalled    bool
	cleanupCalled bool
	cleanupUserID string
}

func (s *stubEpicClient) Authenticate(_ context.Context, _, _ string) (*api.EpicAccountInfo, map[string]string, error) {
	s.authCalled = true
	return s.info, s.snapshot, s.authErr
}

func (s *stubEpicClient) Cleanup(_ context.Context, userID string) error {
	s.cleanupCalled = true
	s.cleanupUserID = userID
	return s.cleanupErr
}

func (s *stubEpicClient) Configured() bool {
	return s.configured
}

// newSyncTestAppWithEpic builds the sync app with a custom EpicClient. Used by
// the Epic connection tests; other tests use newSyncTestApp which passes nil.
func newSyncTestAppWithEpic(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient, epic api.EpicClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, nil, steam, psn, epic, (api.GOGClient)(nil))
	g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
	synch.RegisterRoutes(g)
	return e
}

type stubGOGClient struct {
	authURL string
	token   *api.GOGTokenResponse
	err     error
}

func (s *stubGOGClient) BuildAuthURL() string {
	if s.authURL != "" {
		return s.authURL
	}
	return "https://login.gog.com/auth?test=1"
}

func (s *stubGOGClient) ExchangeCode(_ context.Context, _ string) (*api.GOGTokenResponse, error) {
	return s.token, s.err
}

func newSyncTestAppWithRiverClient(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient, rc *river.Client[pgx.Tx]) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, rc, steam, psn, (api.EpicClient)(nil), (api.GOGClient)(nil))
	g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
	synch.RegisterRoutes(g)
	return e
}

// TestHandleTriggerSync_RiverInsertFails_Returns500 locks in the contract that
// HandleTriggerSync returns 500 when the River enqueue fails, rather than
// silently succeeding with an orphaned job.
func TestHandleTriggerSync_RiverInsertFails_Returns500(t *testing.T) {
	truncateAllTables(t)
	rc := newFailingRiverClient(t)
	e := newSyncTestAppWithRiverClient(t, testDB, &stubSteamClient{}, &stubPSNClient{}, rc)
	_, token := setupTagUser(t, testDB, e, "trigger-river-fail")

	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when River insert fails, got %d: %s", rec.Code, rec.Body.String())
	}
}

func newSyncTestAppWithGOG(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient, gog api.GOGClient) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(testEncrypter, db, nil, steam, psn, (api.EpicClient)(nil), gog)
	g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
	synch.RegisterRoutes(g)
	return e
}

func TestHandleEpicConnect_503WhenNotConfigured(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: false}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-503")

	rec := postJSONAuth(t, e, "/api/sync/epic/connect", map[string]any{"auth_code": "abc"}, token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.authCalled {
		t.Error("Authenticate must not be called when not configured")
	}
}

func TestHandleEpicConnect_400OnMissingAuthCode(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-400")

	rec := postJSONAuth(t, e, "/api/sync/epic/connect", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if stub.authCalled {
		t.Error("Authenticate must not be called when auth_code missing")
	}
}

func TestHandleEpicConnect_400OnAuthError(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true, authErr: fmt.Errorf("legendary: invalid code")}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-auth-fail")

	rec := postJSONAuth(t, e, "/api/sync/epic/connect", map[string]any{"auth_code": "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEpicConnect_500OnNilInfo(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true, info: nil, snapshot: map[string]string{"user.json": "{}"}}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-conn-nil")

	rec := postJSONAuth(t, e, "/api/sync/epic/connect", map[string]any{"auth_code": "ok"}, token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleEpicConnect_HappyPathPersistsConfig(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{
		configured: true,
		info:       &api.EpicAccountInfo{DisplayName: "EpicTester", AccountID: "acct-123"},
		snapshot:   map[string]string{"user.json": `{"displayName":"EpicTester","account_id":"acct-123"}`},
	}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-conn-ok")

	rec := postJSONAuth(t, e, "/api/sync/epic/connect", map[string]any{"auth_code": "ok"}, token)
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
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
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

func TestHandleEpicDisconnect_ClearsCredsSnapshotAndCallsCleanup(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-disc")

	// Pre-populate a connected Epic row: storefront_credentials holds the encrypted snapshot.
	snapJSON := `{"user.json":"{}"}`
	snapCiphertext, err := testEncrypter.Encrypt([]byte(snapJSON))
	if err != nil {
		t.Fatalf("encrypt snap fixture: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'epic', 'manual', ?, now(), now())`,
		"cfg-epic-disc", userID, snapCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	rec := deleteAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if !stub.cleanupCalled || stub.cleanupUserID != userID {
		t.Errorf("expected Cleanup(%q) to be called, got called=%v user=%q", userID, stub.cleanupCalled, stub.cleanupUserID)
	}

	var credsAfter *string
	err = testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Scan(context.Background(), &credsAfter)
	if err != nil {
		t.Fatalf("scan after disconnect: %v", err)
	}
	if credsAfter != nil {
		t.Errorf("expected storefront_credentials cleared, got %v", *credsAfter)
	}
}

func TestHandleGetEpicConnection_DisabledWhenNotConfigured(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: false}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-status-disabled")

	rec := getAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != false || resp["disabled"] != true || resp["reason"] != "legendary_not_configured" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestHandleGetEpicConnection_NotConnectedWhenNoRow(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, e, "epic-status-noconn")

	rec := getAuth(t, e, "/api/sync/epic/connection", token)
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

func TestHandleGetEpicConnection_ConnectedReturnsAccountInfo(t *testing.T) {
	truncateAllTables(t)
	stub := &stubEpicClient{configured: true}
	e := newSyncTestAppWithEpic(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "epic-status-conn")

	rawCreds := `{"display_name":"PlayerOne","account_id":"acct-xyz"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt epic creds: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'epic', 'manual', ?, now(), now())`,
		"cfg-epic-conn", userID, credsCiphertext,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["connected"] != true || resp["disabled"] != false {
		t.Errorf("expected connected=true disabled=false, got: %v", resp)
	}
	if resp["display_name"] != "PlayerOne" || resp["account_id"] != "acct-xyz" {
		t.Errorf("expected display_name/account_id from creds, got: %v", resp)
	}
}

// ─── GOG connection-handler tests ────────────────────────────────────────────

func TestGOGConnect_MissingAuthCode(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-missing")

	rec := postJSONAuth(t, app, "/api/sync/gog/connect", map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGOGConnect_ExchangeFailure(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{err: fmt.Errorf("invalid code")}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-fail")

	rec := postJSONAuth(t, app, "/api/sync/gog/connect", map[string]any{"auth_code": "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGOGConnect_Success(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{
		token: &api.GOGTokenResponse{
			AccessToken:  "acc",
			RefreshToken: "ref",
			UserID:       "u1",
			Username:     "goguser",
		},
	}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-conn-ok")

	rec := postJSONAuth(t, app, "/api/sync/gog/connect", map[string]any{"auth_code": "good"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
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
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-disc")

	req := httptest.NewRequest(http.MethodDelete, "/api/sync/gog/connection", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rec.Code)
	}
}

func TestGOGConnection_NotConnected(t *testing.T) {
	truncateAllTables(t)
	stub := &stubGOGClient{}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
	_, token := setupTagUser(t, testDB, app, "gog-status-notconn")

	req := httptest.NewRequest(http.MethodGet, "/api/sync/gog/connection", nil)
	req.Header.Set("Authorization", "Bearer "+token)
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
		token: &api.GOGTokenResponse{Username: "goguser", UserID: "u1", AccessToken: "a", RefreshToken: "r"},
	}
	app := newSyncTestAppWithGOG(t, testDB, &stubSteamClient{}, &stubPSNClient{}, stub)
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
	req.Header.Set("Authorization", "Bearer "+token)
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
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
	if body["steam_id"] != "76561198012345678" {
		t.Errorf("steam_id: got %v", body["steam_id"])
	}
	if body["username"] != "Frostbyte" {
		t.Errorf("username: got %v", body["username"])
	}
	if body["credentials_error"] != nil {
		t.Errorf("want credentials_error absent, got %v", body["credentials_error"])
	}
}

func TestGetSteamConnection_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "steam-conn-corrupt")
	insertCorruptedSyncConfig(t, testDB, userID, "steam")

	// Decrypt failure must surface credentials_error=true without clearing the row.
	rec := getAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["connected"] != true {
		t.Errorf("want connected=true (row still exists), got %v", body["connected"])
	}
	if body["credentials_error"] != true {
		t.Errorf("want credentials_error=true, got %v", body["credentials_error"])
	}

	// Credentials row must NOT be cleared.
	var creds string
	err := testDB.NewRaw(
		`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`, userID,
	).Scan(context.Background(), &creds)
	if err != nil {
		t.Fatalf("credentials row missing after decrypt failure: %v", err)
	}
	if creds == "" {
		t.Error("expected credentials to remain non-null after decrypt failure")
	}
}
