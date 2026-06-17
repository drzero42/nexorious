package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReplaceUserGameTags(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "rtags")

	countLinks := func(t *testing.T, ugID string) int {
		t.Helper()
		n, err := testDB.NewSelect().Table("user_game_tags").
			Where("user_game_id = ?", ugID).Count(context.Background())
		if err != nil {
			t.Fatalf("count links: %v", err)
		}
		return n
	}
	newUG := func(t *testing.T, id, title string) {
		t.Helper()
		gameID := insertTestGame(t, testDB, title)
		insertTestUserGame(t, testDB, id, userID, int(gameID))
	}

	t.Run("creates links from names, auto-creating tags", func(t *testing.T) {
		newUG(t, "ug-rt-1", "RT Game 1")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-1/tags",
			map[string]any{"tags": []string{"RPG", "Backlog"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-1"); got != 2 {
			t.Fatalf("expected 2 links, got %d", got)
		}
		n, err := testDB.NewSelect().Table("tags").Where("user_id = ?", userID).Count(context.Background())
		if err != nil {
			t.Fatalf("count tags: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 tag definitions auto-created, got %d", n)
		}
	})

	t.Run("replace with subset removes surplus, keeps rest", func(t *testing.T) {
		newUG(t, "ug-rt-2", "RT Game 2")
		putJSONAuth(t, e, "/api/user-games/ug-rt-2/tags", map[string]any{"tags": []string{"A", "B", "C"}}, token)
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-2/tags", map[string]any{"tags": []string{"B"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-2"); got != 1 {
			t.Fatalf("expected 1 link, got %d", got)
		}
	})

	t.Run("empty set clears all tags", func(t *testing.T) {
		newUG(t, "ug-rt-3", "RT Game 3")
		putJSONAuth(t, e, "/api/user-games/ug-rt-3/tags", map[string]any{"tags": []string{"X"}}, token)
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-3/tags", map[string]any{"tags": []string{}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-3"); got != 0 {
			t.Fatalf("expected 0 links, got %d", got)
		}
	})

	t.Run("existing tag reused case-insensitively", func(t *testing.T) {
		newUG(t, "ug-rt-4", "RT Game 4")
		insertTag(t, testDB, "tag-existing-rt", userID, "Shooter", nil)
		putJSONAuth(t, e, "/api/user-games/ug-rt-4/tags", map[string]any{"tags": []string{"shooter"}}, token)
		var tagID string
		if err := testDB.NewSelect().Table("user_game_tags").Column("tag_id").
			Where("user_game_id = ?", "ug-rt-4").Scan(context.Background(), &tagID); err != nil {
			t.Fatalf("scan tag_id: %v", err)
		}
		if tagID != "tag-existing-rt" {
			t.Fatalf("expected reuse of tag-existing-rt, got %q", tagID)
		}
	})

	t.Run("duplicate names de-duped", func(t *testing.T) {
		newUG(t, "ug-rt-5", "RT Game 5")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-5/tags",
			map[string]any{"tags": []string{"Dup", "dup", "DUP"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-5"); got != 1 {
			t.Fatalf("expected 1 link, got %d", got)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		newUG(t, "ug-rt-6", "RT Game 6")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-6/tags", map[string]any{"tags": []string{"  "}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("name over 100 chars rejected", func(t *testing.T) {
		newUG(t, "ug-rt-7", "RT Game 7")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-7/tags",
			map[string]any{"tags": []string{strings.Repeat("x", 101)}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("unknown user game returns 404", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/does-not-exist/tags", map[string]any{"tags": []string{"A"}}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("cannot tag another user's game", func(t *testing.T) {
		otherID, _ := setupUserGamesUser(t, testDB, e, "rtags-other")
		gameID := insertTestGame(t, testDB, "RT Game Other")
		insertTestUserGame(t, testDB, "ug-rt-other", otherID, int(gameID))
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-other/tags", map[string]any{"tags": []string{"A"}}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/user-games/ug-rt-1/tags",
			bytes.NewReader([]byte(`{"tags":["A"]}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
