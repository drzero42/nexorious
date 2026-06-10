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

// insertTestGameWithID inserts a game with an explicit id so tests can match it
// against IGDB candidate ids returned by a mock IGDB server.
func insertTestGameWithID(t *testing.T, db *bun.DB, id int32, title string) {
	t.Helper()
	now := time.Now()
	game := &models.Game{
		ID:          id,
		Title:       title,
		LastUpdated: now,
		CreatedAt:   now,
	}
	if _, err := db.NewInsert().Model(game).Exec(context.Background()); err != nil {
		t.Fatalf("insertTestGameWithID: %v", err)
	}
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

// putAuth issues an authenticated PUT with a JSON body and a session cookie.
func putAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, sessionID string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
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
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown", nil)
}

// TestIGDB_NotConfigured verifies every IGDB-backed endpoint returns 503 when
// the IGDB client is not configured.
func TestIGDB_NotConfigured(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		user   string
		do     func(t *testing.T, e interface {
			ServeHTTP(http.ResponseWriter, *http.Request)
		}, token string) *httptest.ResponseRecorder
	}{
		{
			name:   "search",
			userID: "u-igdb-1",
			user:   "igdbuser",
			do: func(t *testing.T, e interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			}, token string) *httptest.ResponseRecorder {
				return postAuth(t, e, "/api/games/search/igdb", token, `{"query": "Zelda", "limit": 10}`)
			},
		},
		{
			name:   "get game",
			userID: "u-igdb-2",
			user:   "igdbuser2",
			do: func(t *testing.T, e interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			}, token string) *httptest.ResponseRecorder {
				return getAuth(t, e, "/api/games/igdb/12345", token)
			},
		},
		{
			name:   "import",
			userID: "u-igdb-3",
			user:   "igdbuser3",
			do: func(t *testing.T, e interface {
				ServeHTTP(http.ResponseWriter, *http.Request)
			}, token string) *httptest.ResponseRecorder {
				return postAuth(t, e, "/api/games/igdb-import", token, `{"igdb_id": 12345}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := newTestEchoWithIGDB(t, testDB)
			insertAuthTestUser(t, testDB, tt.userID, tt.user, "pass123", true, false)
			token := loginAndGetToken(t, e, tt.user, "pass123")

			rec := tt.do(t, e, token)
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
			}
		})
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

// TestSearchIGDB_AnnotatesLibraryMembership verifies the search response stamps
// user_game_id and user_game_is_wishlisted onto candidates that are already in
// the requesting user's library (issue #856), while leaving other users' library
// entries out.
func TestSearchIGDB_AnnotatesLibraryMembership(t *testing.T) {
	truncateAllTables(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 90101, "name": "Owned Game", "slug": "owned-game"},
			{"id": 90202, "name": "Unowned Game", "slug": "unowned-game"},
			{"id": 90303, "name": "Wishlisted Game", "slug": "wishlisted-game"},
		})
	}))
	defer srv.Close()

	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL)

	insertAuthTestUser(t, testDB, "u-lib-1", "libuser", "pass123", true, false)
	token := loginAndGetToken(t, e, "libuser", "pass123")

	// A different user owns 90202 — it must NOT leak into the caller's results.
	insertAuthTestUser(t, testDB, "u-lib-other", "libother", "pass123", true, false)

	// All games must exist in the games table (user_games.game_id FK).
	insertTestGameWithID(t, testDB, 90101, "Owned Game")
	insertTestGameWithID(t, testDB, 90202, "Unowned Game")
	insertTestGameWithID(t, testDB, 90303, "Wishlisted Game")

	insertTestUserGame(t, testDB, "ug-caller-90101", "u-lib-1", 90101)
	insertTestUserGame(t, testDB, "ug-other-90202", "u-lib-other", 90202)
	// Seed a wishlisted entry for the caller.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id, is_wishlisted) VALUES (?, ?, ?, ?)`,
		"ug-caller-90303", "u-lib-1", 90303, true,
	)
	if err != nil {
		t.Fatalf("insert wishlisted user_game: %v", err)
	}

	body := `{"query": "game", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Games []struct {
			IgdbID               int     `json:"igdb_id"`
			UserGameID           *string `json:"user_game_id"`
			UserGameIsWishlisted *bool   `json:"user_game_is_wishlisted"`
		} `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	type candidate struct {
		userGameID           *string
		userGameIsWishlisted *bool
	}
	byID := make(map[int]candidate, len(resp.Games))
	for _, g := range resp.Games {
		byID[g.IgdbID] = candidate{userGameID: g.UserGameID, userGameIsWishlisted: g.UserGameIsWishlisted}
	}

	// Regular library entry: user_game_id set, is_wishlisted false.
	lib, ok := byID[90101]
	if !ok || lib.userGameID == nil || *lib.userGameID != "ug-caller-90101" {
		t.Fatalf("expected igdb 90101 to carry caller's user_game_id %q, got %v (present=%v)", "ug-caller-90101", lib.userGameID, ok)
	}
	if lib.userGameIsWishlisted == nil || *lib.userGameIsWishlisted != false {
		t.Fatalf("expected igdb 90101 user_game_is_wishlisted=false, got %v", lib.userGameIsWishlisted)
	}

	// Cross-user isolation: other user's entry must not appear for the caller.
	if other, ok := byID[90202]; !ok || other.userGameID != nil {
		t.Fatalf("expected igdb 90202 to have nil user_game_id (owned by another user), got %v", other.userGameID)
	}

	// Wishlisted entry: user_game_id set, is_wishlisted true.
	wish, ok := byID[90303]
	if !ok || wish.userGameID == nil || *wish.userGameID != "ug-caller-90303" {
		t.Fatalf("expected igdb 90303 to carry caller's user_game_id %q, got %v (present=%v)", "ug-caller-90303", wish.userGameID, ok)
	}
	if wish.userGameIsWishlisted == nil || *wish.userGameIsWishlisted != true {
		t.Fatalf("expected igdb 90303 user_game_is_wishlisted=true, got %v", wish.userGameIsWishlisted)
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
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown", nil)
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

// TestSearchIGDB_ExternalGameID_Returns403 verifies that an external_game_id
// the caller does not own — whether owned by another user or non-existent —
// yields 403 before IGDB is ever consulted.
func TestSearchIGDB_ExternalGameID_Returns403(t *testing.T) {
	tests := []struct {
		name      string
		caller    string
		callerUID string
		// egID resolves the external_game_id to send, given the test DB; it may
		// seed another user's row and return its ID, or return a ghost ID.
		egID func(t *testing.T) string
	}{
		{
			name:      "cross-user owned id",
			caller:    "caller",
			callerUID: "u-caller",
			egID: func(t *testing.T) string {
				insertAuthTestUser(t, testDB, "u-owner", "owner", "pass123", true, false)
				return insertTestExternalGameForUser(t, testDB, "u-owner", "steam", "Owner's Game")
			},
		},
		{
			name:      "non-existent id",
			caller:    "caller2",
			callerUID: "u-caller-2",
			egID: func(_ *testing.T) string {
				return "ghost-id-does-not-exist"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			e := newTestEchoWithIGDB(t, testDB) // unconfigured IGDB client; 403 fires before IGDB is called

			extGameID := tt.egID(t)

			insertAuthTestUser(t, testDB, tt.callerUID, tt.caller, "pass123", true, false)
			token := loginAndGetToken(t, e, tt.caller, "pass123")

			body := fmt.Sprintf(`{"query": "x", "limit": 10, "external_game_id": %q}`, extGameID)
			rec := postAuth(t, e, "/api/games/search/igdb", token, body)
			if rec.Code != http.StatusForbidden {
				t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
		})
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

func TestHandleStartStoreLinkRefreshJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	t.Run("non-admin gets 403", func(t *testing.T) {
		_, regTok := setupRegularUser(t, testDB, e, "slr-nonadmin")
		rec := postJSONAuth(t, e, "/api/games/store-links/refresh-job", nil, regTok)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("admin gets 200 with a real job_id, a pending row, and a dispatch", func(t *testing.T) {
		_, adminTok := setupAdminUser(t, testDB, e, "slr-admin")
		rec := postJSONAuth(t, e, "/api/games/store-links/refresh-job", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}
		var body struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.JobID == "" {
			t.Fatalf("expected non-empty job_id")
		}
		var status string
		if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, body.JobID).Scan(context.Background(), &status); err != nil {
			t.Fatalf("job row not found: %v", err)
		}
		if status != "pending" {
			t.Errorf("status: want pending, got %s", status)
		}
		var n int
		if err := testDB.NewRaw(
			`SELECT count(*) FROM river_job WHERE kind = 'store_link_refresh_dispatch'`,
		).Scan(context.Background(), &n); err != nil {
			t.Fatalf("count river_job: %v", err)
		}
		if n < 1 {
			t.Fatalf("expected at least 1 store_link_refresh_dispatch river job, got %d", n)
		}
	})
}

func TestHandleStartMetadataRefreshJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	t.Run("non-admin gets 403", func(t *testing.T) {
		_, regTok := setupRegularUser(t, testDB, e, "mr-nonadmin")
		rec := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, regTok)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("admin gets 200 with a real job_id and a pending row", func(t *testing.T) {
		_, adminTok := setupAdminUser(t, testDB, e, "mr-admin")
		rec := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}
		var body struct {
			Success bool   `json:"success"`
			JobID   string `json:"job_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.JobID == "" {
			t.Fatalf("expected non-empty job_id")
		}
		var status string
		if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, body.JobID).Scan(context.Background(), &status); err != nil {
			t.Fatalf("job row not found for returned id: %v", err)
		}
		if status != "pending" {
			t.Errorf("status: want pending, got %s", status)
		}
	})

	t.Run("second start while active returns the same id and no duplicate", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEchoWithPool(t, testDB)
		_, adminTok := setupAdminUser(t, testDB, e, "mr-admin2")

		first := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		second := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		if second.Code != http.StatusOK {
			t.Fatalf("expected 200 on second call, got %d: %s", second.Code, second.Body)
		}
		idOf := func(rec *httptest.ResponseRecorder) string {
			var b struct {
				JobID string `json:"job_id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &b)
			return b.JobID
		}
		if idOf(first) == "" || idOf(first) != idOf(second) {
			t.Fatalf("expected identical non-empty ids, got %q and %q", idOf(first), idOf(second))
		}
		var count int
		_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
		if count != 1 {
			t.Errorf("expected exactly 1 job row, got %d", count)
		}
	})

	t.Run("concurrent starts create exactly one job (no TOCTOU duplicates)", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEchoWithPool(t, testDB)
		_, adminTok := setupAdminUser(t, testDB, e, "mr-admin-race")

		const n = 16
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(n)
		for range n {
			go func() {
				defer wg.Done()
				<-start // release all goroutines at once to maximise the race window
				rec := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
				if rec.Code != http.StatusOK {
					t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body)
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
			t.Fatalf("expected exactly 1 active metadata_refresh job after %d concurrent starts, got %d", n, count)
		}
	})
}
