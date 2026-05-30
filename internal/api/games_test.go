package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
)

var gameIDCounter int32

func insertTestGame(t *testing.T, db *bun.DB, title string) int32 {
	t.Helper()
	now := time.Now()
	id := atomic.AddInt32(&gameIDCounter, 1)
	game := &models.Game{
		ID:          id,
		Title:       title,
		LastUpdated: now,
		CreatedAt:   now,
	}
	_, err := db.NewInsert().Model(game).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestGame: %v", err)
	}
	return game.ID
}

func TestGamesList(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "u-games-1", "gamesuser", "pass123", true, false)
	token := loginAndGetToken(t, e, "gamesuser", "pass123")

	insertTestGame(t, testDB, "The Witcher 3")
	insertTestGame(t, testDB, "Elden Ring")
	insertTestGame(t, testDB, "Hollow Knight")

	rec := getAuth(t, e, "/api/games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Games   []map[string]any `json:"games"`
		Total   int              `json:"total"`
		Page    int              `json:"page"`
		PerPage int              `json:"per_page"`
		Pages   int              `json:"pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Total)
	}
	if resp.Page != 1 {
		t.Fatalf("expected page=1, got %d", resp.Page)
	}
}

func TestGamesList_Search(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "u-games-2", "gamesuser2", "pass123", true, false)
	token := loginAndGetToken(t, e, "gamesuser2", "pass123")

	insertTestGame(t, testDB, "The Witcher 3")
	insertTestGame(t, testDB, "Elden Ring")

	rec := getAuth(t, e, "/api/games?q=witcher", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected total=1 for 'witcher' search, got %d", resp.Total)
	}
}

func TestGamesGet(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "u-games-3", "gamesuser3", "pass123", true, false)
	token := loginAndGetToken(t, e, "gamesuser3", "pass123")

	gameID := insertTestGame(t, testDB, "Hollow Knight")

	rec := getAuth(t, e, fmt.Sprintf("/api/games/%d", gameID), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var game map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &game); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if game["title"] != "Hollow Knight" {
		t.Fatalf("expected 'Hollow Knight', got %v", game["title"])
	}
}

func TestGamesGet_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "u-games-4", "gamesuser4", "pass123", true, false)
	token := loginAndGetToken(t, e, "gamesuser4", "pass123")

	rec := getAuth(t, e, "/api/games/99999", token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGamesList_InvalidSort(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "u-games-5", "gamesuser5", "pass123", true, false)
	token := loginAndGetToken(t, e, "gamesuser5", "pass123")

	rec := getAuth(t, e, "/api/games?sort_by=invalid_field", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid sort_by, got %d: %s", rec.Code, rec.Body.String())
	}
}

// postAuth issues an authenticated POST with a JSON body and a session cookie.
func postAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, sessionID string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// newTestEchoWithIGDB returns an Echo instance wired with an unconfigured IGDB
// client (credentials absent → configured=false). Use this for tests that
// exercise the IGDB-not-configured → 503 path.
func newTestEchoWithIGDB(t *testing.T, db *bun.DB) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg() // no IGDB credentials
	igdbClient := igdb.NewClient(cfg, ratelimit.NewLocal(100, 100))
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown")
}

func TestSearchIGDB_NotConfigured(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-1", "igdbuser", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser", "pass123")

	body := `{"query": "Zelda", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetIGDBGame_NotConfigured(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-2", "igdbuser2", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser2", "pass123")

	rec := getAuth(t, e, "/api/games/igdb/12345", token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportFromIGDB_NotConfigured(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-3", "igdbuser3", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser3", "pass123")

	body := `{"igdb_id": 12345}`
	rec := postAuth(t, e, "/api/games/igdb-import", token, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── IGDB input validation tests ───────────────────────────────────────────

func TestSearchIGDB_EmptyQuery(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-4", "igdbuser4", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser4", "pass123")

	body := `{"query": "", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty query, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetIGDBGame_InvalidID(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-5", "igdbuser5", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser5", "pass123")

	rec := getAuth(t, e, "/api/games/igdb/not-a-number", token)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid IGDB ID, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestImportFromIGDB_MissingIGDBID(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-6", "igdbuser6", "pass123", true, false)
	token := loginAndGetToken(t, e, "igdbuser6", "pass123")

	// Missing igdb_id (0 value) should return bad request.
	body := `{"igdb_id": 0}`
	rec := postAuth(t, e, "/api/games/igdb-import", token, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing igdb_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── newTestEchoWithLiveIGDB / EG helpers ────────────────────────────────────

// newTestEchoWithLiveIGDB builds a test Echo instance with a configured IGDB
// client pointing at srvURL (both token endpoint and API endpoint). Use this
// when tests need to intercept outbound IGDB calls.
func newTestEchoWithLiveIGDB(t *testing.T, db *bun.DB, srvURL string) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := &config.Config{
		DBEncryptionKey:   "test-db-encryption-key-32-bytes!!",
		SessionExpireDays: 30,
		Port:              8000,
		IGDBClientID:      "test-client-id",
		IGDBClientSecret:  "test-client-secret",
		IGDBAccessToken:   "test-access-token",
	}
	igdbClient := igdb.NewClientWithTokenURL(cfg, srvURL+"/oauth2/token", ratelimit.NewLocal(100, 100))
	igdbClient.SetAPIURLForTest(srvURL)
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown")
}

// insertTestExternalGameForUser inserts a minimal external_game row owned by
// userID and returns its generated ID.
func insertTestExternalGameForUser(t *testing.T, db *bun.DB, userID, storefront, title string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, ?, ?, ?, false, true, false)`,
		id, userID, storefront, uuid.NewString(), title,
	)
	if err != nil {
		t.Fatalf("insertTestExternalGameForUser: %v", err)
	}
	return id
}

// insertTestExternalGamePlatformForUser inserts an external_game_platforms row
// linking externalGameID to platformSlug (must match a platforms.name seed value).
func insertTestExternalGamePlatformForUser(t *testing.T, db *bun.DB, externalGameID, platformSlug string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played)
		 VALUES (?, ?, ?, 0)`,
		uuid.NewString(), externalGameID, platformSlug,
	)
	if err != nil {
		t.Fatalf("insertTestExternalGamePlatformForUser: %v", err)
	}
}

// ─── ExternalGameID ownership / filter tests ─────────────────────────────────

func TestSearchIGDB_ExternalGameID_CrossUserReturns403(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB) // unconfigured IGDB client; 403 fires before IGDB is called

	// Owning user.
	insertAuthTestUser(t, testDB, "u-owner", "owner", "pass123", true, false)
	otherEGID := insertTestExternalGameForUser(t, testDB, "u-owner", "steam", "Owner's Game")

	// Calling user.
	insertAuthTestUser(t, testDB, "u-caller", "caller", "pass123", true, false)
	token := loginAndGetToken(t, e, "caller", "pass123")

	body := fmt.Sprintf(`{"query": "x", "limit": 10, "external_game_id": %q}`, otherEGID)
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-user external_game_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchIGDB_ExternalGameID_NonExistentReturns403(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-caller-2", "caller2", "pass123", true, false)
	token := loginAndGetToken(t, e, "caller2", "pass123")

	body := `{"query": "x", "limit": 10, "external_game_id": "ghost-id-does-not-exist"}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-existent external_game_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchIGDB_ExternalGameID_OwnedPassesPlatformIDs(t *testing.T) {
	truncateAllTables(t)

	var (
		capturedBodies []string
		mu             sync.Mutex
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		capturedBodies = append(capturedBodies, string(body))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]map[string]any{}) // empty result; we only assert on the request body
	}))
	defer srv.Close()

	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL)

	insertAuthTestUser(t, testDB, "u-caller-3", "caller3", "pass123", true, false)
	token := loginAndGetToken(t, e, "caller3", "pass123")
	egID := insertTestExternalGameForUser(t, testDB, "u-caller-3", "steam", "Owned Title")
	insertTestExternalGamePlatformForUser(t, testDB, egID, "pc-windows")

	body := fmt.Sprintf(`{"query": "x", "limit": 10, "external_game_id": %q}`, egID)
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// At least one captured IGDB request body must contain the platform clause for pc-windows (igdb_platform_id=6).
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, b := range capturedBodies {
		if strings.Contains(b, "platforms = (6)") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one IGDB request body to contain 'platforms = (6)'; got %v", capturedBodies)
	}
}

func TestSearchIGDB_NoExternalGameID_UnfilteredCall(t *testing.T) {
	truncateAllTables(t)
	var (
		capturedBodies []string
		mu             sync.Mutex
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		capturedBodies = append(capturedBodies, string(body))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL)
	insertAuthTestUser(t, testDB, "u-caller-4", "caller4", "pass123", true, false)
	token := loginAndGetToken(t, e, "caller4", "pass123")

	body := `{"query": "x", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, b := range capturedBodies {
		if strings.Contains(b, "platforms = (") {
			t.Fatalf("body without external_game_id must produce unfiltered IGDB calls; got %q", b)
		}
	}
}
