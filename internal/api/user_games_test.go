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
	insertAuthTestSession(t, db, userID, "access-"+suffix, "refresh-"+suffix, 1)
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
		req.Header.Set("Authorization", "Bearer "+token)
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
		req.Header.Set("Authorization", "Bearer "+token)
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

func TestUpdatePlatform_InvalidPlatform(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "upd-plat-bad-plat")
	gameID := insertTestGame(t, testDB, "Update Plat Invalid Game")
	insertTestUserGame(t, testDB, "ug-upd-plat-b", userID, int(gameID))

	rec := postJSONAuth(t, e, "/api/user-games/ug-upd-plat-b/platforms", map[string]any{
		"platform":   "pc-windows",
		"storefront": "steam",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	platformID := createResp["id"].(string)

	// Update with a non-existent platform name.
	rec = putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-upd-plat-b/platforms/%s", platformID),
		map[string]any{"platform": "nonexistent-platform-xyz"}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdatePlatform_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "upd-plat-bad-sf")
	gameID := insertTestGame(t, testDB, "Update Storefront Game")
	insertTestUserGame(t, testDB, "ug-upd-sf", userID, int(gameID))

	rec := postJSONAuth(t, e, "/api/user-games/ug-upd-sf/platforms", map[string]any{
		"platform":   "pc-windows",
		"storefront": "steam",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	platformID := createResp["id"].(string)

	// Update with a non-existent storefront name.
	rec = putJSONAuth(t, e, fmt.Sprintf("/api/user-games/ug-upd-sf/platforms/%s", platformID),
		map[string]any{"storefront": "nonexistent-storefront-xyz"}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
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
