package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptrace/bun"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

func setupUserGamesUser(t *testing.T, db *bun.DB, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, suffix string) (string, string) {
	t.Helper()
	userID := "u-ug-" + suffix
	username := "uguser-" + suffix
	insertAuthTestUser(t, db, userID, username, "pass123", true, false)
	token := loginAndGetToken(t, handler, username, "pass123")
	return userID, token
}

func insertTestUserGame(t *testing.T, db *bun.DB, id, userID string, gameID int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
		id, userID, gameID,
	)
	if err != nil {
		t.Fatalf("insertTestUserGame: %v", err)
	}
}

func insertTestUserGamePlatform(t *testing.T, db *bun.DB, id, userGameID string, platform, storefront *string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		id, userGameID, platform, storefront,
	)
	if err != nil {
		t.Fatalf("insertTestUserGamePlatform: %v", err)
	}
}

func TestCreateUserGame(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "create")

	t.Run("success with valid status", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Create")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":     gameID,
			"play_status": "not_started",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["game_id"] == nil {
			t.Fatal("expected game_id in response")
		}
		// game relation must be present so the frontend can navigate to the new entry.
		if resp["game"] == nil {
			t.Fatal("expected game relation in response")
		}
	})

	t.Run("success without play_status", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game No Status")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate game", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Dup")
		postJSONAuth(t, e, "/api/user-games", map[string]any{"game_id": gameID}, token)
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{"game_id": gameID}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 for duplicate, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid game_id", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": 999999,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Invalid Status")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":     gameID,
			"play_status": "invalid_status",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("platforms are persisted on create", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game With Platforms")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
			"platforms": []map[string]any{
				{"platform": "pc-windows", "storefront": "steam"},
				{"platform": "pc-windows", "storefront": "epic-games-store"},
			},
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		platforms, ok := resp["platforms"].([]any)
		if !ok {
			t.Fatalf("expected platforms array in response, got: %T", resp["platforms"])
		}
		if len(platforms) != 2 {
			t.Fatalf("expected 2 platform associations, got %d", len(platforms))
		}
	})

	t.Run("platform ownership/acquired/hours are persisted on create", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Platform Details")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
			"platforms": []map[string]any{
				{
					"platform":         "pc-windows",
					"storefront":       "steam",
					"ownership_status": "borrowed",
					"acquired_date":    "2026-05-01",
					"hours_played":     12.5,
				},
			},
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		platforms, ok := resp["platforms"].([]any)
		if !ok || len(platforms) != 1 {
			t.Fatalf("expected 1 platform association, got: %v", resp["platforms"])
		}
		p, ok := platforms[0].(map[string]any)
		if !ok {
			t.Fatalf("expected platform object, got %T", platforms[0])
		}
		if p["ownership_status"] != "borrowed" {
			t.Fatalf("expected ownership_status=borrowed, got %v", p["ownership_status"])
		}
		if p["hours_played"] != 12.5 {
			t.Fatalf("expected hours_played=12.5, got %v", p["hours_played"])
		}
		acquired, _ := p["acquired_date"].(string)
		if acquired == "" {
			t.Fatalf("expected acquired_date to be persisted, got %v", p["acquired_date"])
		}
		if got := acquired[:10]; got != "2026-05-01" {
			t.Fatalf("expected acquired_date 2026-05-01, got %q", got)
		}
	})

	t.Run("invalid acquired_date is rejected", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Bad Acquired")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
			"platforms": []map[string]any{
				{"platform": "pc-windows", "storefront": "steam", "acquired_date": "not-a-date"},
			},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid acquired_date, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid ownership_status is rejected", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Test Game Bad Ownership")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
			"platforms": []map[string]any{
				{"platform": "pc-windows", "storefront": "steam", "ownership_status": "bogus"},
			},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid ownership_status, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestGetUserGame(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "get")
	gameID := insertTestGame(t, testDB, "Test Game Get")
	insertTestUserGame(t, testDB, "ug-get-1", userID, int(gameID))

	t.Run("success", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-get-1", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["id"] != "ug-get-1" {
			t.Fatalf("expected id=ug-get-1, got %v", resp["id"])
		}
		if resp["game"] == nil {
			t.Fatal("expected game relation to be loaded")
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "get-other")
		rec := getAuth(t, e, "/api/user-games/ug-get-1", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateUserGame(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "update")
	gameID := insertTestGame(t, testDB, "Test Game Update")
	insertTestUserGame(t, testDB, "ug-upd-1", userID, int(gameID))

	t.Run("success partial update", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"play_status": "completed",
			"is_loved":    true,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["play_status"] != "completed" {
			t.Fatalf("expected play_status=completed, got %v", resp["play_status"])
		}
		if resp["is_loved"] != true {
			t.Fatalf("expected is_loved=true, got %v", resp["is_loved"])
		}
	})

	t.Run("set field to null", func(t *testing.T) {
		putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"personal_rating": 4}, token)
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"personal_rating": nil}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["personal_rating"] != nil {
			t.Fatalf("expected personal_rating=nil, got %v", resp["personal_rating"])
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"play_status": "invalid"}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rating out of range", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"personal_rating": 6}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for rating > 5, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("reject game_id in update", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"game_id": 999}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/nonexistent", map[string]any{"is_loved": true}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "update-other")
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"is_loved": true}, token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteUserGame(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "delete")
	gameID := insertTestGame(t, testDB, "Test Game Delete")
	insertTestUserGame(t, testDB, "ug-del-1", userID, int(gameID))

	t.Run("success", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games/ug-del-1", token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		gameID2 := insertTestGame(t, testDB, "Test Game Del Other")
		insertTestUserGame(t, testDB, "ug-del-other", userID, int(gameID2))
		_, token2 := setupUserGamesUser(t, testDB, e, "delete-other")
		rec := deleteAuth(t, e, "/api/user-games/ug-del-other", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleGetUserGame_StoreURL(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "store-url")
	gameID := insertTestGame(t, testDB, "Store URL Game")
	insertTestUserGame(t, testDB, "ug-surl-1", userID, int(gameID))

	// Insert an external_game with store_link so buildStoreURL can produce a URL.
	egID := "eg-surl-steam-440"
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, store_link)
		 VALUES (?, ?, 'steam', '440', 'Half-Life 2', false, true, false, '440')`,
		egID, userID,
	)
	if err != nil {
		t.Fatalf("insert external_game: %v", err)
	}

	// Insert a user_game_platform linked to that external_game.
	pcWindows := "pc-windows"
	steam := "steam"
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, external_game_id)
		 VALUES (?, 'ug-surl-1', ?, ?, ?)`,
		"ugp-surl-1", pcWindows, steam, egID,
	)
	if err != nil {
		t.Fatalf("insert user_game_platform with external_game_id: %v", err)
	}

	rec := getAuth(t, e, "/api/user-games/ug-surl-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	platforms, ok := resp["platforms"].([]any)
	if !ok || len(platforms) != 1 {
		t.Fatalf("expected 1 platform, got: %v", resp["platforms"])
	}

	platform := platforms[0].(map[string]any)
	storeURL, exists := platform["store_url"]
	if !exists || storeURL == nil {
		t.Fatalf("expected store_url to be set, got nil (platforms[0]=%v)", platform)
	}

	const wantURL = "https://store.steampowered.com/app/440/"
	if storeURL != wantURL {
		t.Fatalf("expected store_url=%q, got %q", wantURL, storeURL)
	}
}

func TestDeleteUserGame_Cascades(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "del-cascade")
	gameID := insertTestGame(t, testDB, "Test Game Cascade Del")
	insertTestUserGame(t, testDB, "ug-cas-del-1", userID, int(gameID))

	pcWindows := "pc-windows"
	steam := "steam"
	insertTestUserGamePlatform(t, testDB, "ugp-cas-1", "ug-cas-del-1", &pcWindows, &steam)
	insertTag(t, testDB, "tag-cas-del-1", userID, "CascadeTag", nil)
	insertUserGameTag(t, testDB, "ugt-cas-del-1", "ug-cas-del-1", "tag-cas-del-1")

	rec := deleteAuth(t, e, "/api/user-games/ug-cas-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-cas-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_platforms to be cascade-deleted")
	}
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_tags WHERE id = 'ugt-cas-del-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_tags to be cascade-deleted")
	}
}

func TestListUserGames(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "list")
	g1 := insertTestGame(t, testDB, "Alpha Game")
	g2 := insertTestGame(t, testDB, "Beta Game")
	g3 := insertTestGame(t, testDB, "Gamma Game")
	insertTestUserGame(t, testDB, "ug-list-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-list-2", userID, int(g2))
	insertTestUserGame(t, testDB, "ug-list-3", userID, int(g3))

	t.Run("basic list", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total := int(resp["total"].(float64))
		if total != 3 {
			t.Fatalf("expected total=3, got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 3 {
			t.Fatalf("expected 3 user_games, got %d", len(games))
		}
		first := games[0].(map[string]any)
		if first["game"] == nil {
			t.Fatal("expected game relation to be loaded")
		}
	})

	t.Run("pagination", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?page=1&per_page=2", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		total := int(resp["total"].(float64))
		if total != 3 {
			t.Fatalf("expected total=3, got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 2 {
			t.Fatalf("expected 2 items on page 1, got %d", len(games))
		}
		pages := int(resp["pages"].(float64))
		if pages != 2 {
			t.Fatalf("expected 2 pages, got %d", pages)
		}
	})

	t.Run("sort by title", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?sort_by=title&sort_order=asc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		first := games[0].(map[string]any)
		game := first["game"].(map[string]any)
		if game["title"] != "Alpha Game" {
			t.Fatalf("expected Alpha Game first, got %v", game["title"])
		}
	})

	t.Run("search by title", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?q=Beta", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		total := int(resp["total"].(float64))
		if total != 1 {
			t.Fatalf("expected total=1 for 'Beta', got %d", total)
		}
	})

	t.Run("multi-value play_status filters OR-within-facet", func(t *testing.T) {
		if _, err := testDB.ExecContext(context.Background(),
			`UPDATE user_games SET play_status = 'not_started' WHERE id = 'ug-list-1';
			 UPDATE user_games SET play_status = 'shelved'     WHERE id = 'ug-list-2';
			 UPDATE user_games SET play_status = 'completed'   WHERE id = 'ug-list-3'`); err != nil {
			t.Fatalf("set statuses: %v", err)
		}

		// Two statuses → a game matches if it is in the selected set.
		rec := getAuth(t, e, "/api/user-games?play_status=not_started&play_status=shelved", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if total := int(resp["total"].(float64)); total != 2 {
			t.Fatalf("expected total=2 for two statuses, got %d", total)
		}

		// A single value still works (back-compat with single-select URLs).
		rec = getAuth(t, e, "/api/user-games?play_status=completed", token)
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if total := int(resp["total"].(float64)); total != 1 {
			t.Fatalf("expected total=1 for single status, got %d", total)
		}

		// Unknown values are dropped, not 400; here it leaves no constraint.
		rec = getAuth(t, e, "/api/user-games?play_status=bogus", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for unknown status, got %d", rec.Code)
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if total := int(resp["total"].(float64)); total != 3 {
			t.Fatalf("expected total=3 (unknown status dropped), got %d", total)
		}
	})

	t.Run("user scoped", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "list-other")
		rec := getAuth(t, e, "/api/user-games", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		total := int(resp["total"].(float64))
		if total != 0 {
			t.Fatalf("expected total=0 for other user, got %d", total)
		}
	})

	t.Run("invalid sort field", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?sort_by=hacked", token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateProgress(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "progress")
	gameID := insertTestGame(t, testDB, "Test Game Progress")
	insertTestUserGame(t, testDB, "ug-prog-1", userID, int(gameID))

	t.Run("success play_status", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"play_status": "in_progress",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty body", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"play_status": "nope",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/nonexistent/progress", map[string]any{
			"play_status": "completed",
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestBulkUpdate(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "bulk-upd")
	g1 := insertTestGame(t, testDB, "Bulk Upd 1")
	g2 := insertTestGame(t, testDB, "Bulk Upd 2")
	insertTestUserGame(t, testDB, "ug-bu-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-bu-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids": []string{"ug-bu-1", "ug-bu-2"},
			"updates": map[string]any{
				"play_status": "completed",
				"is_loved":    true,
			},
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		updated := int(resp["updated"].(float64))
		if updated != 2 {
			t.Fatalf("expected updated=2, got %d", updated)
		}
	})

	t.Run("empty ids", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{},
			"updates": map[string]any{"is_loved": true},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty updates", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bu-1"},
			"updates": map[string]any{},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("skips non-owned ids", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "bulk-upd-other")
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bu-1", "ug-bu-2"},
			"updates": map[string]any{"is_loved": false},
		}, token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		updated := int(resp["updated"].(float64))
		if updated != 0 {
			t.Fatalf("expected updated=0, got %d", updated)
		}
	})
}

func TestBulkDelete(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "bulk-del")
	g1 := insertTestGame(t, testDB, "Bulk Del 1")
	g2 := insertTestGame(t, testDB, "Bulk Del 2")
	insertTestUserGame(t, testDB, "ug-bd-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-bd-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"ids": []string{"ug-bd-1", "ug-bd-2"},
		})
		req := httptest.NewRequest(http.MethodDelete, "/api/user-games/bulk-delete", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		deleted := int(resp["deleted"].(float64))
		if deleted != 2 {
			t.Fatalf("expected deleted=2, got %d", deleted)
		}
	})
}

func TestBulkAddPlatforms(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "bulk-plat-add")
	g1 := insertTestGame(t, testDB, "Bulk Plat Add 1")
	g2 := insertTestGame(t, testDB, "Bulk Plat Add 2")
	insertTestUserGame(t, testDB, "ug-bpa-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-bpa-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/bulk-add-platforms", map[string]any{
			"user_game_ids": []string{"ug-bpa-1", "ug-bpa-2"},
			"platform":      "pc-windows",
			"storefront":    "steam",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		added := int(resp["added"].(float64))
		if added != 2 {
			t.Fatalf("expected added=2, got %d", added)
		}
	})

	t.Run("duplicate skipped", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/bulk-add-platforms", map[string]any{
			"user_game_ids": []string{"ug-bpa-1"},
			"platform":      "pc-windows",
			"storefront":    "steam",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		added := int(resp["added"].(float64))
		if added != 0 {
			t.Fatalf("expected added=0 for duplicate, got %d", added)
		}
	})
}

func TestBulkRemovePlatforms(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "bulk-plat-rm")
	g1 := insertTestGame(t, testDB, "Bulk Plat Rm 1")
	insertTestUserGame(t, testDB, "ug-bpr-1", userID, int(g1))
	pc := "pc-windows"
	steam := "steam"
	insertTestUserGamePlatform(t, testDB, "ugp-bpr-1", "ug-bpr-1", &pc, &steam)

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"user_game_ids": []string{"ug-bpr-1"},
			"platform":      "pc-windows",
			"storefront":    "steam",
		})
		req := httptest.NewRequest(http.MethodDelete, "/api/user-games/bulk-remove-platforms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		removed := int(resp["removed"].(float64))
		if removed != 1 {
			t.Fatalf("expected removed=1, got %d", removed)
		}
	})
}

func TestPlatformCRUD(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "plat-crud")
	gameID := insertTestGame(t, testDB, "Plat CRUD Game")
	insertTestUserGame(t, testDB, "ug-plat-1", userID, int(gameID))

	t.Run("list empty", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var platforms []any
		_ = json.Unmarshal(rec.Body.Bytes(), &platforms)
		if len(platforms) != 0 {
			t.Fatalf("expected 0 platforms, got %d", len(platforms))
		}
	})

	var platformID string

	t.Run("create", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":         "pc-windows",
			"storefront":       "steam",
			"ownership_status": "owned",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		platformID = resp["id"].(string)
		if platformID == "" {
			t.Fatal("expected non-empty id")
		}
	})

	t.Run("create duplicate", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":   "pc-windows",
			"storefront": "steam",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create invalid ownership_status", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":         "pc-windows",
			"storefront":       "gog",
			"ownership_status": "stolen",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list after create", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var platforms []any
		_ = json.Unmarshal(rec.Body.Bytes(), &platforms)
		if len(platforms) != 1 {
			t.Fatalf("expected 1 platform, got %d", len(platforms))
		}
	})

	t.Run("update", func(t *testing.T) {
		rec := putJSONAuth(t, e,
			fmt.Sprintf("/api/user-games/ug-plat-1/platforms/%s", platformID),
			map[string]any{
				"ownership_status": "subscription",
			}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete", func(t *testing.T) {
		rec := deleteAuth(t, e,
			fmt.Sprintf("/api/user-games/ug-plat-1/platforms/%s", platformID), token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "plat-crud-other")
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ── TestCreatePlatformDefaultsIsAvailable ───────────────────────────────

// TestCreatePlatformDefaultsIsAvailable verifies that POST
// /api/user-games/:id/platforms defaults is_available to true when the field
// is omitted (consistent with create and move-to-library), and still honors an
// explicit value. Regression test for #880.
func TestCreatePlatformDefaultsIsAvailable(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "plat-avail")
	gameID := insertTestGame(t, testDB, "Avail Default Game")
	insertTestUserGame(t, testDB, "ug-avail-1", userID, int(gameID))

	isAvailable := func(rec *httptest.ResponseRecorder) any {
		t.Helper()
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v (body %s)", err, rec.Body.String())
		}
		return resp["is_available"]
	}

	t.Run("omitted defaults to true", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-avail-1/platforms", map[string]any{
			"platform":   "pc-windows",
			"storefront": "steam",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := isAvailable(rec); got != true {
			t.Fatalf("expected is_available true when omitted, got %v", got)
		}
	})

	t.Run("explicit false is honored", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-avail-1/platforms", map[string]any{
			"platform":     "pc-windows",
			"storefront":   "gog",
			"is_available": false,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := isAvailable(rec); got != false {
			t.Fatalf("expected is_available false when explicitly set, got %v", got)
		}
	})
}

// ── TestPlatformAcquiredDate ────────────────────────────────────────────

// TestPlatformAcquiredDate verifies the acquired date is persisted on create
// and update, can be cleared with an empty string, is left untouched when the
// field is omitted, and rejects a malformed value. Regression test for #849.
func TestPlatformAcquiredDate(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "plat-acq")
	gameID := insertTestGame(t, testDB, "Acquired Date Game")
	insertTestUserGame(t, testDB, "ug-acq-1", userID, int(gameID))

	acquiredDate := func(rec *httptest.ResponseRecorder) any {
		t.Helper()
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v (body %s)", err, rec.Body.String())
		}
		return resp["acquired_date"]
	}
	reloadAcquiredDate := func(platformID string) any {
		t.Helper()
		rec := getAuth(t, e, "/api/user-games/ug-acq-1/platforms", token)
		var platforms []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &platforms); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		for _, p := range platforms {
			if p["id"] == platformID {
				return p["acquired_date"]
			}
		}
		t.Fatalf("platform %s not found in list", platformID)
		return nil
	}

	var platformID string

	t.Run("create with acquired_date persists it", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-acq-1/platforms", map[string]any{
			"platform":      "pc-windows",
			"storefront":    "steam",
			"acquired_date": "2024-06-01",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		platformID = resp["id"].(string)

		got, _ := acquiredDate(rec).(string)
		if got == "" || got[:10] != "2024-06-01" {
			t.Fatalf("response acquired_date = %v, want 2024-06-01", resp["acquired_date"])
		}
		if reloaded, _ := reloadAcquiredDate(platformID).(string); reloaded == "" || reloaded[:10] != "2024-06-01" {
			t.Fatalf("reloaded acquired_date = %v, want 2024-06-01", reloaded)
		}
	})

	t.Run("update changes acquired_date", func(t *testing.T) {
		rec := putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-acq-1/platforms/%s", platformID),
			map[string]any{"acquired_date": "2025-01-15"}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if reloaded, _ := reloadAcquiredDate(platformID).(string); reloaded == "" || reloaded[:10] != "2025-01-15" {
			t.Fatalf("reloaded acquired_date = %v, want 2025-01-15", reloaded)
		}
	})

	t.Run("update with omitted field leaves acquired_date unchanged", func(t *testing.T) {
		rec := putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-acq-1/platforms/%s", platformID),
			map[string]any{"hours_played": 5}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if reloaded, _ := reloadAcquiredDate(platformID).(string); reloaded == "" || reloaded[:10] != "2025-01-15" {
			t.Fatalf("reloaded acquired_date = %v, want unchanged 2025-01-15", reloaded)
		}
	})

	t.Run("update with empty string clears acquired_date", func(t *testing.T) {
		rec := putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-acq-1/platforms/%s", platformID),
			map[string]any{"acquired_date": ""}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if reloaded := reloadAcquiredDate(platformID); reloaded != nil {
			t.Fatalf("reloaded acquired_date = %v, want nil after clear", reloaded)
		}
	})

	t.Run("create with malformed acquired_date is rejected", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-acq-1/platforms", map[string]any{
			"platform":      "pc-windows",
			"storefront":    "gog",
			"acquired_date": "not-a-date",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ── TestUpdatePlatform ──────────────────────────────────────────────────

func TestUpdatePlatform_InvalidOwnershipStatus(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "upd-plat-bad-own")
	gameID := insertTestGame(t, testDB, "Update Plat Game")
	insertTestUserGame(t, testDB, "ug-upd-plat", userID, int(gameID))

	// First create a platform entry.
	rec := postJSONAuth(t, e, "/api/user-games/ug-upd-plat/platforms", map[string]any{
		"platform":   "pc-windows",
		"storefront": "steam",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	platformID := createResp["id"].(string)

	// Update with invalid ownership_status.
	rec = putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-upd-plat/platforms/%s", platformID),
		map[string]any{"ownership_status": "stolen"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpdatePlatform_InvalidName verifies that updating a platform row with a
// non-existent platform or storefront name yields 404.
func TestUpdatePlatform_InvalidName(t *testing.T) {
	tests := []struct {
		name    string
		suffix  string
		ugID    string
		title   string
		updates map[string]any
	}{
		{
			name:    "invalid platform name",
			suffix:  "upd-plat-bad-plat",
			ugID:    "ug-upd-plat-b",
			title:   "Update Plat Invalid Game",
			updates: map[string]any{"platform": "nonexistent-platform-xyz"},
		},
		{
			name:    "invalid storefront name",
			suffix:  "upd-plat-bad-sf",
			ugID:    "ug-upd-sf",
			title:   "Update Storefront Game",
			updates: map[string]any{"storefront": "nonexistent-storefront-xyz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateAllTables(t)
			cfg := testCfg()
			e := newTestEcho(t, testDB, cfg)
			userID, token := setupUserGamesUser(t, testDB, e, tt.suffix)
			gameID := insertTestGame(t, testDB, tt.title)
			insertTestUserGame(t, testDB, tt.ugID, userID, int(gameID))

			rec := postJSONAuth(t, e, fmt.Sprintf("/api/user-games/%s/platforms", tt.ugID), map[string]any{
				"platform":   "pc-windows",
				"storefront": "steam",
			}, token)
			if rec.Code != http.StatusCreated {
				t.Fatalf("create expected 201, got %d: %s", rec.Code, rec.Body.String())
			}
			var createResp map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
			platformID := createResp["id"].(string)

			// Update with a non-existent platform/storefront name.
			rec = putJSONAuth(t, e, fmt.Sprintf("/api/user-games/%s/platforms/%s", tt.ugID, platformID),
				tt.updates, token)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUpdatePlatform_PlatformNotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "upd-plat-notfound")
	gameID := insertTestGame(t, testDB, "Update Plat NotFound Game")
	insertTestUserGame(t, testDB, "ug-upd-notfound", userID, int(gameID))

	// Try to update a platform that doesn't exist.
	rec := putJSONAuth(t, e, "/api/user-games/ug-upd-notfound/platforms/nonexistent-platform-id",
		map[string]any{"ownership_status": "owned"}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ── Utility endpoint test helpers ───────────────────────────────────────

func insertTestGameWithGenre(t *testing.T, db *bun.DB, title, genre string) int32 {
	t.Helper()
	gameID := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET genre = ? WHERE id = ?`, genre, gameID)
	if err != nil {
		t.Fatalf("insertTestGameWithGenre: %v", err)
	}
	return gameID
}

func insertTestGameWithMetadata(t *testing.T, db *bun.DB, title, genre, gameModes, themes, playerPerspectives string) int32 {
	t.Helper()
	gameID := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET genre = ?, game_modes = ?, themes = ?, player_perspectives = ? WHERE id = ?`,
		genre, gameModes, themes, playerPerspectives, gameID)
	if err != nil {
		t.Fatalf("insertTestGameWithMetadata: %v", err)
	}
	return gameID
}

// ── TestListUserGameIDs ─────────────────────────────────────────────────

func TestListUserGameIDs(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "ids")

	g1 := insertTestGame(t, testDB, "IDs Game 1")
	g2 := insertTestGame(t, testDB, "IDs Game 2")
	g3 := insertTestGame(t, testDB, "IDs Game 3")
	insertTestUserGame(t, testDB, "ug-ids-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-ids-2", userID, int(g2))
	insertTestUserGame(t, testDB, "ug-ids-3", userID, int(g3))

	// Set play_status on one for filter test
	_, err := testDB.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'completed' WHERE id = 'ug-ids-1'`)
	if err != nil {
		t.Fatalf("update play_status: %v", err)
	}

	t.Run("basic", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ids", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		ids := resp["ids"].([]any)
		if len(ids) != 3 {
			t.Fatalf("expected 3 ids, got %d", len(ids))
		}
	})

	t.Run("with filter", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ids?play_status=completed", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		ids := resp["ids"].([]any)
		if len(ids) != 1 {
			t.Fatalf("expected 1 id, got %d", len(ids))
		}
	})

	t.Run("user scoped", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "ids-other")
		rec := getAuth(t, e, "/api/user-games/ids", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		ids := resp["ids"].([]any)
		if len(ids) != 0 {
			t.Fatalf("expected 0 ids, got %d", len(ids))
		}
	})

	t.Run("empty collection", func(t *testing.T) {
		_, token3 := setupUserGamesUser(t, testDB, e, "ids-empty")
		rec := getAuth(t, e, "/api/user-games/ids", token3)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		// Verify we get an array, not null
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"ids":[]`)) {
			t.Fatalf("expected empty array, got %s", body)
		}
	})
}

// ── TestListGenres ──────────────────────────────────────────────────────

func TestListGenres(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "genres")

	t.Run("basic", func(t *testing.T) {
		g1 := insertTestGameWithGenre(t, testDB, "Genre Game 1", "Action, RPG")
		g2 := insertTestGameWithGenre(t, testDB, "Genre Game 2", "RPG, Simulation")
		insertTestUserGame(t, testDB, "ug-genre-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-genre-2", userID, int(g2))

		rec := getAuth(t, e, "/api/user-games/genres", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		genres := resp["genres"].([]any)
		if len(genres) != 3 {
			t.Fatalf("expected 3 genres, got %d: %v", len(genres), genres)
		}
		// Check alphabetical order
		if genres[0] != "Action" || genres[1] != "RPG" || genres[2] != "Simulation" {
			t.Fatalf("expected [Action, RPG, Simulation], got %v", genres)
		}
	})

	t.Run("comma separation", func(t *testing.T) {
		// "Action, RPG" on g1 should produce both Action and RPG individually
		rec := getAuth(t, e, "/api/user-games/genres", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		found := map[string]bool{}
		for _, g := range resp.Genres {
			found[g] = true
		}
		if !found["Action"] || !found["RPG"] {
			t.Fatalf("expected both Action and RPG from comma-separated genre, got %v", resp.Genres)
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "genres-empty")
		rec := getAuth(t, e, "/api/user-games/genres", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"genres":[]`)) {
			t.Fatalf("expected empty array, got %s", body)
		}
	})

	t.Run("null genres excluded", func(t *testing.T) {
		_, token3 := setupUserGamesUser(t, testDB, e, "genres-null")
		userID3 := "u-ug-genres-null"
		gNull := insertTestGame(t, testDB, "No Genre Game")
		insertTestUserGame(t, testDB, "ug-genre-null", userID3, int(gNull))

		rec := getAuth(t, e, "/api/user-games/genres", token3)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.Genres) != 0 {
			t.Fatalf("expected empty genres for user with only null-genre game, got %v", resp.Genres)
		}
	})
}

// ── TestFilterOptions ───────────────────────────────────────────────────

func TestFilterOptions(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "filteropts")

	t.Run("basic", func(t *testing.T) {
		g1 := insertTestGameWithMetadata(t, testDB, "Filter Game 1", "Action, RPG", "Single player", "Fantasy", "Third person")
		g2 := insertTestGameWithMetadata(t, testDB, "Filter Game 2", "RPG, Strategy", "Multiplayer", "Sci-fi", "First person")
		insertTestUserGame(t, testDB, "ug-fo-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-fo-2", userID, int(g2))

		rec := getAuth(t, e, "/api/user-games/filter-options", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		genres := resp["genres"].([]any)
		if len(genres) != 3 {
			t.Fatalf("expected 3 genres, got %d: %v", len(genres), genres)
		}
		gameModes := resp["game_modes"].([]any)
		if len(gameModes) != 2 {
			t.Fatalf("expected 2 game_modes, got %d", len(gameModes))
		}
		themes := resp["themes"].([]any)
		if len(themes) != 2 {
			t.Fatalf("expected 2 themes, got %d", len(themes))
		}
		perspectives := resp["player_perspectives"].([]any)
		if len(perspectives) != 2 {
			t.Fatalf("expected 2 player_perspectives, got %d", len(perspectives))
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, testDB, e, "filteropts-empty")
		rec := getAuth(t, e, "/api/user-games/filter-options", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp["genres"].([]any)) != 0 {
			t.Fatal("expected empty genres")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		// RPG appears in both games from "basic" subtest but should appear once.
		rec := getAuth(t, e, "/api/user-games/filter-options", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		count := 0
		for _, g := range resp.Genres {
			if g == "RPG" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected RPG to appear once, appeared %d times in %v", count, resp.Genres)
		}
	})
}

// ── TestCollectionStats ─────────────────────────────────────────────────

func TestCollectionStats(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	t.Run("empty collection", func(t *testing.T) {
		_, token := setupUserGamesUser(t, testDB, e, "stats-empty")
		rec := getAuth(t, e, "/api/user-games/stats", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if int(resp["total_games"].(float64)) != 0 {
			t.Fatal("expected total_games=0")
		}
		if resp["completion_rate"].(float64) != 0 {
			t.Fatal("expected completion_rate=0")
		}
		if resp["average_rating"] != nil {
			t.Fatal("expected average_rating=null")
		}
		if resp["total_hours_played"].(float64) != 0 {
			t.Fatal("expected total_hours_played=0")
		}
	})

	t.Run("basic", func(t *testing.T) {
		userID, token := setupUserGamesUser(t, testDB, e, "stats-basic")

		g1 := insertTestGameWithGenre(t, testDB, "Stats Game 1", "RPG")
		g2 := insertTestGameWithGenre(t, testDB, "Stats Game 2", "RPG, Action")
		g3 := insertTestGameWithGenre(t, testDB, "Stats Game 3", "Action")
		insertTestUserGame(t, testDB, "ug-stats-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-stats-2", userID, int(g2))
		insertTestUserGame(t, testDB, "ug-stats-3", userID, int(g3))

		// Set statuses and ratings
		_, _ = testDB.ExecContext(context.Background(),
			`UPDATE user_games SET play_status = 'completed', personal_rating = 4 WHERE id = 'ug-stats-1'`)
		_, _ = testDB.ExecContext(context.Background(),
			`UPDATE user_games SET play_status = 'not_started', personal_rating = 3 WHERE id = 'ug-stats-2'`)
		_, _ = testDB.ExecContext(context.Background(),
			`UPDATE user_games SET play_status = 'in_progress' WHERE id = 'ug-stats-3'`)

		rec := getAuth(t, e, "/api/user-games/stats", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		if int(resp["total_games"].(float64)) != 3 {
			t.Fatalf("expected total_games=3, got %v", resp["total_games"])
		}

		completionStats := resp["completion_stats"].(map[string]any)
		if int(completionStats["completed"].(float64)) != 1 {
			t.Fatalf("expected completed=1, got %v", completionStats["completed"])
		}

		if int(resp["pile_of_shame"].(float64)) != 1 {
			t.Fatalf("expected pile_of_shame=1, got %v", resp["pile_of_shame"])
		}

		cr := resp["completion_rate"].(float64)
		if cr < 33.3 || cr > 33.34 {
			t.Fatalf("expected completion_rate≈33.33, got %v", cr)
		}

		avg := resp["average_rating"].(float64)
		if avg != 3.5 {
			t.Fatalf("expected average_rating=3.5, got %v", avg)
		}

		genreStats := resp["genre_stats"].(map[string]any)
		if int(genreStats["RPG"].(float64)) != 2 {
			t.Fatalf("expected RPG=2, got %v", genreStats["RPG"])
		}
		if int(genreStats["Action"].(float64)) != 2 {
			t.Fatalf("expected Action=2, got %v", genreStats["Action"])
		}
	})

	t.Run("hours from platforms", func(t *testing.T) {
		userID, token := setupUserGamesUser(t, testDB, e, "stats-hours")

		g1 := insertTestGame(t, testDB, "Hours Game 1")
		g2 := insertTestGame(t, testDB, "Hours Game 2")
		insertTestUserGame(t, testDB, "ug-hours-1", userID, int(g1))
		insertTestUserGame(t, testDB, "ug-hours-2", userID, int(g2))

		// Game 1: platform hours = 50.5
		_, _ = testDB.ExecContext(context.Background(),
			`INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING`)
		pc := "pc"
		steam := "steam"
		insertTestUserGamePlatform(t, testDB, "ugp-hours-1", "ug-hours-1", &pc, &steam)
		_, _ = testDB.ExecContext(context.Background(),
			`UPDATE user_game_platforms SET hours_played = 50.5 WHERE id = 'ugp-hours-1'`)

		// Game 2: platform hours = 10.0
		insertTestUserGamePlatform(t, testDB, "ugp-hours-2", "ug-hours-2", &pc, &steam)
		_, _ = testDB.ExecContext(context.Background(),
			`UPDATE user_game_platforms SET hours_played = 10.0 WHERE id = 'ugp-hours-2'`)

		rec := getAuth(t, e, "/api/user-games/stats", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		hours := resp["total_hours_played"].(float64)
		if hours != 60.5 {
			t.Fatalf("expected total_hours_played=60.5, got %v", hours)
		}
	})
}

func TestListUserGamesSortByHours(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "sorthours")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC') ON CONFLICT DO NOTHING`)
	pc := "pc"

	gLow := insertTestGame(t, testDB, "Low Hours")
	gHigh := insertTestGame(t, testDB, "High Hours")
	gZero := insertTestGame(t, testDB, "Zero Hours")
	insertTestUserGame(t, testDB, "ug-sh-low", userID, int(gLow))
	insertTestUserGame(t, testDB, "ug-sh-high", userID, int(gHigh))
	insertTestUserGame(t, testDB, "ug-sh-zero", userID, int(gZero)) // no platforms → 0

	insertTestUserGamePlatform(t, testDB, "ugp-sh-low", "ug-sh-low", &pc, nil)
	insertTestUserGamePlatform(t, testDB, "ugp-sh-high", "ug-sh-high", &pc, nil)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 5 WHERE id = 'ugp-sh-low'`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 100 WHERE id = 'ugp-sh-high'`)

	idsInOrder := func(t *testing.T, order string) []string {
		t.Helper()
		rec := getAuth(t, e, "/api/user-games?sort_by=hours_played&sort_order="+order, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		ids := make([]string, len(games))
		for i, g := range games {
			ids[i] = g.(map[string]any)["id"].(string)
		}
		return ids
	}

	t.Run("desc orders highest hours first, zero last", func(t *testing.T) {
		ids := idsInOrder(t, "desc")
		want := []string{"ug-sh-high", "ug-sh-low", "ug-sh-zero"}
		for i := range want {
			if ids[i] != want[i] {
				t.Fatalf("desc order mismatch: got %v, want %v", ids, want)
			}
		}
	})

	t.Run("asc orders zero first, highest last", func(t *testing.T) {
		ids := idsInOrder(t, "asc")
		want := []string{"ug-sh-zero", "ug-sh-low", "ug-sh-high"}
		for i := range want {
			if ids[i] != want[i] {
				t.Fatalf("asc order mismatch: got %v, want %v", ids, want)
			}
		}
	})
}

// TestListUserGamesSortByGameNumerics is the regression guard for issue #639:
// sort_by=howlongtobeat_main and sort_by=rating_average must return 200 (not
// the prior 400) and order results correctly with NULLs sinking to the bottom
// in both directions.
func TestListUserGamesSortByGameNumerics(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "sortnumerics")

	gLow := insertTestGame(t, testDB, "Low Values")
	gHigh := insertTestGame(t, testDB, "High Values")
	gNull := insertTestGame(t, testDB, "Null Values")

	insertTestUserGame(t, testDB, "ug-low", userID, int(gLow))
	insertTestUserGame(t, testDB, "ug-high", userID, int(gHigh))
	insertTestUserGame(t, testDB, "ug-null", userID, int(gNull))

	// Set both columns with parallel low/high mapping on the same fixture so
	// rating_average and howlongtobeat_main produce the same expected ordering.
	// ug-null is left with both columns NULL (the default).
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE games SET rating_average = 50, howlongtobeat_main = 10 WHERE id = ?`, gLow); err != nil {
		t.Fatalf("update gLow: %v", err)
	}
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE games SET rating_average = 90, howlongtobeat_main = 100 WHERE id = ?`, gHigh); err != nil {
		t.Fatalf("update gHigh: %v", err)
	}

	idsInOrder := func(t *testing.T, field, order string) []string {
		t.Helper()
		rec := getAuth(t, e, "/api/user-games?sort_by="+field+"&sort_order="+order, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		games := resp["user_games"].([]any)
		ids := make([]string, len(games))
		for i, g := range games {
			ids[i] = g.(map[string]any)["id"].(string)
		}
		return ids
	}

	for _, field := range []string{"rating_average", "howlongtobeat_main"} {
		t.Run(field+" desc orders high, low, null", func(t *testing.T) {
			ids := idsInOrder(t, field, "desc")
			want := []string{"ug-high", "ug-low", "ug-null"}
			if len(ids) != len(want) {
				t.Fatalf("got %d ids, want %d: %v", len(ids), len(want), ids)
			}
			for i := range want {
				if ids[i] != want[i] {
					t.Fatalf("desc order mismatch: got %v, want %v", ids, want)
				}
			}
		})
		t.Run(field+" asc orders low, high, null", func(t *testing.T) {
			ids := idsInOrder(t, field, "asc")
			want := []string{"ug-low", "ug-high", "ug-null"}
			if len(ids) != len(want) {
				t.Fatalf("got %d ids, want %d: %v", len(ids), len(want), ids)
			}
			for i := range want {
				if ids[i] != want[i] {
					t.Fatalf("asc order mismatch: got %v, want %v", ids, want)
				}
			}
		})
	}
}

func TestUserGameCalculatedHours(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "calchours")

	// Two platforms on one game: 10 + 25.5 = 35.5
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC'),('ps5','PS5') ON CONFLICT DO NOTHING`)
	g1 := insertTestGame(t, testDB, "Calc Hours Game")
	insertTestUserGame(t, testDB, "ug-calc-1", userID, int(g1))
	pc, ps5 := "pc", "ps5"
	insertTestUserGamePlatform(t, testDB, "ugp-calc-1", "ug-calc-1", &pc, nil)
	insertTestUserGamePlatform(t, testDB, "ugp-calc-2", "ug-calc-1", &ps5, nil)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 10 WHERE id = 'ugp-calc-1'`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 25.5 WHERE id = 'ugp-calc-2'`)

	// A second game with no platform hours → 0
	g2 := insertTestGame(t, testDB, "No Hours Game")
	insertTestUserGame(t, testDB, "ug-calc-2", userID, int(g2))

	t.Run("single GET returns summed hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-calc-1", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["hours_played"].(float64) != 35.5 {
			t.Fatalf("expected hours_played=35.5, got %v", resp["hours_played"])
		}
	})

	t.Run("single GET returns 0 when no platform hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-calc-2", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["hours_played"].(float64) != 0 {
			t.Fatalf("expected hours_played=0, got %v", resp["hours_played"])
		}
	})

	t.Run("list returns summed hours", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		var calc1 map[string]any
		for _, g := range games {
			gm := g.(map[string]any)
			if gm["id"] == "ug-calc-1" {
				calc1 = gm
			}
		}
		if calc1 == nil {
			t.Fatal("ug-calc-1 not found in list response")
		}
		if calc1["hours_played"].(float64) != 35.5 {
			t.Fatalf("expected list hours_played=35.5, got %v", calc1["hours_played"])
		}
	})
}

func TestUserGamesNoStoredHoursColumn(t *testing.T) {
	truncateAllTables(t)
	var count int
	err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'user_games' AND column_name = 'hours_played'`).Scan(&count)
	if err != nil {
		t.Fatalf("information_schema query: %v", err)
	}
	if count != 0 {
		t.Fatalf("user_games.hours_played must not be a stored column; found %d", count)
	}
}

func TestHandleClearLibrary(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "clear")

	// Seed 3 games + user games.
	g1 := insertTestGame(t, testDB, "Clear Game 1")
	g2 := insertTestGame(t, testDB, "Clear Game 2")
	g3 := insertTestGame(t, testDB, "Clear Game 3")
	insertTestUserGame(t, testDB, "ug-cl-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-cl-2", userID, int(g2))
	insertTestUserGame(t, testDB, "ug-cl-3", userID, int(g3))

	// Seed a job + job item + active river job.
	insertJob(t, testDB, "job-cl-1", userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-cl-1", "job-cl-1", userID, "key-1", "Game 1", "pending")
	riverID := insertRiverJob(t, testDB, "sync_item", "available", "ji-cl-1")

	// Seed a sync config.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront) VALUES (?, ?, 'steam')`,
		"sc-cl-1", userID,
	)
	if err != nil {
		t.Fatalf("seed sync_config: %v", err)
	}

	t.Run("deletes library and returns count", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}

		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(3) {
			t.Errorf("deleted = %v, want 3", resp["deleted"])
		}
	})

	t.Run("clears user_games", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("user_games count = %d, want 0", count)
		}
	})

	t.Run("clears jobs", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("jobs count = %d, want 0", count)
		}
	})

	t.Run("preserves sync configs", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sync_configs WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("sync_configs count = %d, want 1 (sync configs must survive library clear)", count)
		}
	})

	t.Run("cancels active river jobs", func(t *testing.T) {
		var state string
		if err := testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, riverID).
			Scan(context.Background(), &state); err != nil {
			t.Fatalf("river state: %v", err)
		}
		if state != "cancelled" {
			t.Errorf("river state = %q, want cancelled", state)
		}
	})

	t.Run("idempotent on empty library", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(0) {
			t.Errorf("deleted = %v, want 0", resp["deleted"])
		}
	})

	t.Run("does not touch other users", func(t *testing.T) {
		otherID, _ := setupUserGamesUser(t, testDB, e, "clear-other")
		otherGame := insertTestGame(t, testDB, "Other User Game")
		insertTestUserGame(t, testDB, "ug-cl-other", otherID, int(otherGame))

		// Clear the original user again (already empty).
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}

		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, otherID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count other: %v", err)
		}
		if count != 1 {
			t.Errorf("other user_games count = %d, want 1", count)
		}
	})
}

func TestManualPlatformHoursReflectedInSum(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "manualhours")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc','PC') ON CONFLICT DO NOTHING`)
	g := insertTestGame(t, testDB, "Manual Hours Game")
	insertTestUserGame(t, testDB, "ug-mh-1", userID, int(g))
	pc := "pc"
	insertTestUserGamePlatform(t, testDB, "ugp-mh-1", "ug-mh-1", &pc, nil)

	// Manually set hours via the platform-update endpoint.
	rec := putJSONAuth(t, e, "/api/user-games/ug-mh-1/platforms/ugp-mh-1", map[string]any{
		"hours_played": 42.5,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from platform update, got %d: %s", rec.Code, rec.Body.String())
	}

	// The calculated game-level value reflects the manual entry — proving it is derived,
	// not stored.
	rec = getAuth(t, e, "/api/user-games/ug-mh-1", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["hours_played"].(float64) != 42.5 {
		t.Fatalf("expected calculated hours_played=42.5, got %v", resp["hours_played"])
	}
}

// readPlayStatus returns the play_status for a user game directly from the DB.
func readPlayStatus(t *testing.T, userGameID string) string {
	t.Helper()
	var status string
	err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE id = ?`, userGameID,
	).Scan(context.Background(), &status)
	if err != nil {
		t.Fatalf("readPlayStatus: %v", err)
	}
	return status
}

// TestAutoPromotePlayStatus covers the not_started → in_progress auto-promotion
// triggered from the manual edit paths (issue #713).
func TestAutoPromotePlayStatus(t *testing.T) {
	cfg := testCfg()

	t.Run("update platform hours promotes not_started", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEcho(t, testDB, cfg)
		userID, token := setupUserGamesUser(t, testDB, e, "promote-upd")
		g := insertTestGame(t, testDB, "Promote Update Game")
		insertTestUserGame(t, testDB, "ug-pr-1", userID, int(g))
		pc := "pc-windows"
		insertTestUserGamePlatform(t, testDB, "ugp-pr-1", "ug-pr-1", &pc, nil)

		if got := readPlayStatus(t, "ug-pr-1"); got != "not_started" {
			t.Fatalf("precondition: expected not_started, got %q", got)
		}

		rec := putJSONAuth(t, e, "/api/user-games/ug-pr-1/platforms/ugp-pr-1", map[string]any{
			"hours_played": 3.0,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := readPlayStatus(t, "ug-pr-1"); got != "in_progress" {
			t.Fatalf("expected in_progress after adding hours, got %q", got)
		}
	})

	t.Run("create platform with hours promotes not_started", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEcho(t, testDB, cfg)
		userID, token := setupUserGamesUser(t, testDB, e, "promote-create-plat")
		g := insertTestGame(t, testDB, "Promote Create Plat Game")
		insertTestUserGame(t, testDB, "ug-pr-2", userID, int(g))

		rec := postJSONAuth(t, e, "/api/user-games/ug-pr-2/platforms", map[string]any{
			"platform":     "pc-windows",
			"storefront":   "steam",
			"hours_played": 1.5,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := readPlayStatus(t, "ug-pr-2"); got != "in_progress" {
			t.Fatalf("expected in_progress, got %q", got)
		}
	})

	t.Run("create user game with played platform promotes", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEcho(t, testDB, cfg)
		_, token := setupUserGamesUser(t, testDB, e, "promote-create-ug")
		g := insertTestGame(t, testDB, "Promote Create UG Game")

		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": g,
			"platforms": []map[string]any{
				{"platform": "pc-windows", "storefront": "steam", "hours_played": 5.0},
			},
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		ugID := resp["id"].(string)
		if got := readPlayStatus(t, ugID); got != "in_progress" {
			t.Fatalf("expected in_progress, got %q", got)
		}
	})

	t.Run("does not clobber a chosen status", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEcho(t, testDB, cfg)
		userID, token := setupUserGamesUser(t, testDB, e, "promote-guard")
		g := insertTestGame(t, testDB, "Promote Guard Game")
		insertTestUserGame(t, testDB, "ug-pr-3", userID, int(g))
		if _, err := testDB.ExecContext(context.Background(),
			`UPDATE user_games SET play_status = 'completed' WHERE id = 'ug-pr-3'`); err != nil {
			t.Fatalf("set completed: %v", err)
		}
		pc := "pc-windows"
		insertTestUserGamePlatform(t, testDB, "ugp-pr-3", "ug-pr-3", &pc, nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-pr-3/platforms/ugp-pr-3", map[string]any{
			"hours_played": 10.0,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := readPlayStatus(t, "ug-pr-3"); got != "completed" {
			t.Fatalf("expected completed to be preserved, got %q", got)
		}
	})

	t.Run("no promotion when hours stay zero", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEcho(t, testDB, cfg)
		userID, token := setupUserGamesUser(t, testDB, e, "promote-zero")
		g := insertTestGame(t, testDB, "Promote Zero Game")
		insertTestUserGame(t, testDB, "ug-pr-4", userID, int(g))
		pc := "pc-windows"
		insertTestUserGamePlatform(t, testDB, "ugp-pr-4", "ug-pr-4", &pc, nil)

		rec := putJSONAuth(t, e, "/api/user-games/ug-pr-4/platforms/ugp-pr-4", map[string]any{
			"ownership_status": "owned",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := readPlayStatus(t, "ug-pr-4"); got != "not_started" {
			t.Fatalf("expected not_started to be preserved, got %q", got)
		}
	})
}

func TestCreateUserGame_Wishlist(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "wl-create")

	t.Run("wishlist create with no platforms succeeds and sets the flag", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Hades Wishlist")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":       gameID,
			"is_wishlisted": true,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("want 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["is_wishlisted"] != true {
			t.Fatalf("want is_wishlisted=true, got %v", resp["is_wishlisted"])
		}
		plats, _ := resp["platforms"].([]any)
		if len(plats) != 0 {
			t.Fatalf("want 0 platforms, got %d", len(plats))
		}
	})

	t.Run("wishlist create with platforms is rejected with 422", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Celeste Wishlist")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":       gameID,
			"is_wishlisted": true,
			"platforms": []map[string]any{
				{"platform": "pc-windows"},
			},
		}, token)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("want 422 for wishlist+platforms, got %d: %s", rec.Code, rec.Body.String())
		}
		var errResp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("unmarshal error body: %v", err)
		}
		if msg, _ := errResp["message"].(string); msg != "a wishlisted game cannot have platforms" {
			t.Fatalf("want specific error message, got %q", msg)
		}
	})
}

func TestListUserGames_WishlistExclusion(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "wl-list")

	// One library entry (with a platform).
	libGameID := insertTestGame(t, testDB, "Library Game")
	rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
		"game_id": libGameID,
		"platforms": []map[string]any{
			{"platform": "pc-windows"},
		},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create library game: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// One wishlist entry (no platforms).
	wishGameID := insertTestGame(t, testDB, "Wishlist Game")
	rec = postJSONAuth(t, e, "/api/user-games", map[string]any{
		"game_id":       wishGameID,
		"is_wishlisted": true,
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create wishlist game: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	t.Run("default list excludes wishlisted", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total := int(resp["total"].(float64))
		if total != 1 {
			t.Fatalf("default list want total=1 (library only), got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 1 {
			t.Fatalf("default list want 1 user_game, got %d", len(games))
		}
		first := games[0].(map[string]any)
		if first["is_wishlisted"] != false {
			t.Fatalf("default list entry should not be wishlisted, got %v", first["is_wishlisted"])
		}
	})

	t.Run("wishlist=true returns only wishlisted", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?wishlist=true", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total := int(resp["total"].(float64))
		if total != 1 {
			t.Fatalf("wishlist list want total=1, got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 1 {
			t.Fatalf("wishlist list want 1 user_game, got %d", len(games))
		}
		first := games[0].(map[string]any)
		if first["is_wishlisted"] != true {
			t.Fatalf("wishlist list entry should be wishlisted, got %v", first["is_wishlisted"])
		}
	})
}

// decodeID unmarshals a recorder body and returns the "id" string field.
func decodeID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decodeID unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	id, ok := resp["id"].(string)
	if !ok || id == "" {
		t.Fatalf("decodeID: expected non-empty string id, got %v", resp["id"])
	}
	return id
}

// doMoveToLibrary posts to POST /api/user-games/:id/move-to-library.
func doMoveToLibrary(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, token, userGameID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/user-games/"+userGameID+"/move-to-library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMoveToLibrary(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, token := setupUserGamesUser(t, testDB, e, "mtl")

	t.Run("happy path: platform attached, flag cleared, notes preserved", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Hades MTL")
		created := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":        gameID,
			"is_wishlisted":  true,
			"personal_notes": "want this",
		}, token)
		if created.Code != http.StatusCreated {
			t.Fatalf("create wishlist: want 201, got %d: %s", created.Code, created.Body.String())
		}
		ugID := decodeID(t, created)

		rec := doMoveToLibrary(t, e, token, ugID, `{"platforms":[{"platform":"pc-windows","ownership_status":"owned"}]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("move-to-library: want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["is_wishlisted"] != false {
			t.Fatalf("want is_wishlisted=false after move, got %v", resp["is_wishlisted"])
		}
		if resp["personal_notes"] != "want this" {
			t.Fatalf("personal_notes should carry over, got %v", resp["personal_notes"])
		}
		plats, _ := resp["platforms"].([]any)
		if len(plats) != 1 {
			t.Fatalf("want 1 platform, got %d", len(plats))
		}
	})

	t.Run("moving a library entry again returns 422", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Hades MTL Already Library")
		created := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":       gameID,
			"is_wishlisted": true,
		}, token)
		if created.Code != http.StatusCreated {
			t.Fatalf("create wishlist: want 201, got %d: %s", created.Code, created.Body.String())
		}
		ugID := decodeID(t, created)

		// First move succeeds.
		rec := doMoveToLibrary(t, e, token, ugID, `{"platforms":[{"platform":"pc-windows","ownership_status":"owned"}]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("first move: want 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Second move on a now-library entry is rejected.
		rec = doMoveToLibrary(t, e, token, ugID, `{"platforms":[{"platform":"ps5"}]}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("want 422 moving non-wishlisted, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty platforms returns 422", func(t *testing.T) {
		gameID := insertTestGame(t, testDB, "Celeste MTL")
		created := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":       gameID,
			"is_wishlisted": true,
		}, token)
		if created.Code != http.StatusCreated {
			t.Fatalf("create wishlist: want 201, got %d: %s", created.Code, created.Body.String())
		}
		ugID := decodeID(t, created)

		rec := doMoveToLibrary(t, e, token, ugID, `{"platforms":[]}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("want 422 for empty platforms, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("another user cannot move-to-library an entry they do not own", func(t *testing.T) {
		// Create a wishlist entry owned by user A (the existing "mtl" user).
		gameID := insertTestGame(t, testDB, "Hollow Knight MTL Cross")
		created := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":       gameID,
			"is_wishlisted": true,
		}, token)
		if created.Code != http.StatusCreated {
			t.Fatalf("create wishlist: want 201, got %d: %s", created.Code, created.Body.String())
		}
		ugID := decodeID(t, created)

		// User B tries to move user A's entry.
		_, tokenB := setupUserGamesUser(t, testDB, e, "mtl-other")
		rec := doMoveToLibrary(t, e, tokenB, ugID, `{"platforms":[{"platform":"pc-windows","ownership_status":"owned"}]}`)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404 when moving another user's wishlist entry, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func floatPtr(f float64) *float64 { return &f }

func insertTestGameWithHLTB(t *testing.T, db *bun.DB, title string, hltbMain *float64) int32 {
	t.Helper()
	id := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET howlongtobeat_main = ? WHERE id = ?`, hltbMain, id)
	if err != nil {
		t.Fatalf("insertTestGameWithHLTB: %v", err)
	}
	return id
}

func TestListUserGamesTimeToBeatFilter(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "ttb")

	// Three games with distinct howlongtobeat_main, plus one with NULL.
	shortID := insertTestGameWithHLTB(t, testDB, "Short Game", floatPtr(5))
	midID := insertTestGameWithHLTB(t, testDB, "Mid Game", floatPtr(20))
	longID := insertTestGameWithHLTB(t, testDB, "Long Game", floatPtr(80))
	nullID := insertTestGameWithHLTB(t, testDB, "Unknown Game", nil)

	insertTestUserGame(t, testDB, "ug-ttb-short", userID, int(shortID))
	insertTestUserGame(t, testDB, "ug-ttb-mid", userID, int(midID))
	insertTestUserGame(t, testDB, "ug-ttb-long", userID, int(longID))
	insertTestUserGame(t, testDB, "ug-ttb-null", userID, int(nullID))

	titlesFor := func(query string) map[string]bool {
		rec := getAuth(t, e, "/api/user-games?"+query, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			UserGames []struct {
				Game struct {
					Title string `json:"title"`
				} `json:"game"`
			} `json:"user_games"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got := map[string]bool{}
		for _, ug := range resp.UserGames {
			got[ug.Game.Title] = true
		}
		return got
	}

	t.Run("max only excludes longer and NULL", func(t *testing.T) {
		got := titlesFor("time_to_beat_max=25")
		if !got["Short Game"] || !got["Mid Game"] {
			t.Fatalf("expected Short+Mid, got %v", got)
		}
		if got["Long Game"] || got["Unknown Game"] {
			t.Fatalf("did not expect Long/Unknown, got %v", got)
		}
	})

	t.Run("min only excludes shorter and NULL", func(t *testing.T) {
		got := titlesFor("time_to_beat_min=10")
		if !got["Mid Game"] || !got["Long Game"] {
			t.Fatalf("expected Mid+Long, got %v", got)
		}
		if got["Short Game"] || got["Unknown Game"] {
			t.Fatalf("did not expect Short/Unknown, got %v", got)
		}
	})

	t.Run("range", func(t *testing.T) {
		got := titlesFor("time_to_beat_min=10&time_to_beat_max=25")
		if !got["Mid Game"] || len(got) != 1 {
			t.Fatalf("expected only Mid Game, got %v", got)
		}
	})
}
