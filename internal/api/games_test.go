package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/ratelimit"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
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
	_, err := testDB.NewInsert().Model(game).Exec(context.Background())
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
	insertAuthTestSession(t, testDB, "u-games-1", "access-games-1", "refresh-games-1", 1)
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
	insertAuthTestSession(t, testDB, "u-games-2", "access-games-2", "refresh-games-2", 1)
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
	insertAuthTestSession(t, testDB, "u-games-3", "access-games-3", "refresh-games-3", 1)
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
	insertAuthTestSession(t, testDB, "u-games-4", "access-games-4", "refresh-games-4", 1)
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
	insertAuthTestSession(t, testDB, "u-games-5", "access-games-5", "refresh-games-5", 1)
	token := loginAndGetToken(t, e, "gamesuser5", "pass123")

	rec := getAuth(t, e, "/api/games?sort_by=invalid_field", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid sort_by, got %d: %s", rec.Code, rec.Body.String())
	}
}

// postAuth issues an authenticated POST with a JSON body.
func postAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, accessToken string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
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
	return api.New(cfg, m, testDB, "", igdbClient, nil, nil)
}

func TestSearchIGDB_NotConfigured(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-igdb-1", "igdbuser", "pass123", true, false)
	insertAuthTestSession(t, testDB, "u-igdb-1", "access-igdb-1", "refresh-igdb-1", 1)
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
	insertAuthTestSession(t, testDB, "u-igdb-2", "access-igdb-2", "refresh-igdb-2", 1)
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
	insertAuthTestSession(t, testDB, "u-igdb-3", "access-igdb-3", "refresh-igdb-3", 1)
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
	insertAuthTestSession(t, testDB, "u-igdb-4", "access-igdb-4", "refresh-igdb-4", 1)
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
	insertAuthTestSession(t, testDB, "u-igdb-5", "access-igdb-5", "refresh-igdb-5", 1)
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
	insertAuthTestSession(t, testDB, "u-igdb-6", "access-igdb-6", "refresh-igdb-6", 1)
	token := loginAndGetToken(t, e, "igdbuser6", "pass123")

	// Missing igdb_id (0 value) should return bad request.
	body := `{"igdb_id": 0}`
	rec := postAuth(t, e, "/api/games/igdb-import", token, body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing igdb_id, got %d: %s", rec.Code, rec.Body.String())
	}
}
