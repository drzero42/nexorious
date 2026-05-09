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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	_, token := setupUserGamesUser(t, db, e, "create")

	t.Run("success with valid status", func(t *testing.T) {
		gameID := insertTestGame(t, db, "Test Game Create")
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
	})

	t.Run("success without play_status", func(t *testing.T) {
		gameID := insertTestGame(t, db, "Test Game No Status")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate game", func(t *testing.T) {
		gameID := insertTestGame(t, db, "Test Game Dup")
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
		gameID := insertTestGame(t, db, "Test Game Invalid Status")
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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "get")
	gameID := insertTestGame(t, db, "Test Game Get")
	insertTestUserGame(t, db, "ug-get-1", userID, int(gameID))

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
		_, token2 := setupUserGamesUser(t, db, e, "get-other")
		rec := getAuth(t, e, "/api/user-games/ug-get-1", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "update")
	gameID := insertTestGame(t, db, "Test Game Update")
	insertTestUserGame(t, db, "ug-upd-1", userID, int(gameID))

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
		_, token2 := setupUserGamesUser(t, db, e, "update-other")
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{"is_loved": true}, token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "delete")
	gameID := insertTestGame(t, db, "Test Game Delete")
	insertTestUserGame(t, db, "ug-del-1", userID, int(gameID))

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
		gameID2 := insertTestGame(t, db, "Test Game Del Other")
		insertTestUserGame(t, db, "ug-del-other", userID, int(gameID2))
		_, token2 := setupUserGamesUser(t, db, e, "delete-other")
		rec := deleteAuth(t, e, "/api/user-games/ug-del-other", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteUserGame_Cascades(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "del-cascade")
	gameID := insertTestGame(t, db, "Test Game Cascade Del")
	insertTestUserGame(t, db, "ug-cas-del-1", userID, int(gameID))

	pcWindows := "pc-windows"
	steam := "steam"
	insertTestUserGamePlatform(t, db, "ugp-cas-1", "ug-cas-del-1", &pcWindows, &steam)
	insertTag(t, db, "tag-cas-del-1", userID, "CascadeTag", nil)
	insertUserGameTag(t, db, "ugt-cas-del-1", "ug-cas-del-1", "tag-cas-del-1")

	rec := deleteAuth(t, e, "/api/user-games/ug-cas-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	_ = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-cas-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_platforms to be cascade-deleted")
	}
	_ = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_tags WHERE id = 'ugt-cas-del-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_tags to be cascade-deleted")
	}
}

func TestListUserGames(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "list")
	g1 := insertTestGame(t, db, "Alpha Game")
	g2 := insertTestGame(t, db, "Beta Game")
	g3 := insertTestGame(t, db, "Gamma Game")
	insertTestUserGame(t, db, "ug-list-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-list-2", userID, int(g2))
	insertTestUserGame(t, db, "ug-list-3", userID, int(g3))

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
		_, token2 := setupUserGamesUser(t, db, e, "list-other")
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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "progress")
	gameID := insertTestGame(t, db, "Test Game Progress")
	insertTestUserGame(t, db, "ug-prog-1", userID, int(gameID))

	t.Run("success hours only", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"hours_played": 12.5,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success both fields", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"hours_played": 25.0,
			"play_status":  "in_progress",
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
			"hours_played": 1.0,
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestBulkUpdate(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "bulk-upd")
	g1 := insertTestGame(t, db, "Bulk Upd 1")
	g2 := insertTestGame(t, db, "Bulk Upd 2")
	insertTestUserGame(t, db, "ug-bu-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bu-2", userID, int(g2))

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
		_, token2 := setupUserGamesUser(t, db, e, "bulk-upd-other")
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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "bulk-del")
	g1 := insertTestGame(t, db, "Bulk Del 1")
	g2 := insertTestGame(t, db, "Bulk Del 2")
	insertTestUserGame(t, db, "ug-bd-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bd-2", userID, int(g2))

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "bulk-plat-add")
	g1 := insertTestGame(t, db, "Bulk Plat Add 1")
	g2 := insertTestGame(t, db, "Bulk Plat Add 2")
	insertTestUserGame(t, db, "ug-bpa-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bpa-2", userID, int(g2))

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "bulk-plat-rm")
	g1 := insertTestGame(t, db, "Bulk Plat Rm 1")
	insertTestUserGame(t, db, "ug-bpr-1", userID, int(g1))
	pc := "pc-windows"
	steam := "steam"
	insertTestUserGamePlatform(t, db, "ugp-bpr-1", "ug-bpr-1", &pc, &steam)

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "plat-crud")
	gameID := insertTestGame(t, db, "Plat CRUD Game")
	insertTestUserGame(t, db, "ug-plat-1", userID, int(gameID))

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
		_, token2 := setupUserGamesUser(t, db, e, "plat-crud-other")
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
