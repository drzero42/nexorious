package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uptrace/bun"
)

// ─── Extra HTTP helpers ───────────────────────────────────────────────────────

// postJSONAuth fires a POST request with a JSON body and a session cookie.
func postJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ─── DB helpers ───────────────────────────────────────────────────────────────

// insertTag inserts a tag row directly and returns its ID.
func insertTag(t *testing.T, db *bun.DB, id, userID, name string, color *string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO tags (id, user_id, name, color)
		 VALUES (?, ?, ?, ?)`,
		id, userID, name, color,
	)
	if err != nil {
		t.Fatalf("insertTag: %v", err)
	}
}

// insertGame inserts a minimal game row with the given id.
func insertGame(t *testing.T, db *bun.DB, id int, title string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO games (id, title) VALUES (?, ?)`, id, title,
	)
	if err != nil {
		t.Fatalf("insertGame: %v", err)
	}
}

// insertUserGame inserts a user_game row.
func insertUserGame(t *testing.T, db *bun.DB, id, userID string, gameID int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
		id, userID, gameID,
	)
	if err != nil {
		t.Fatalf("insertUserGame: %v", err)
	}
}

// insertUserGameTag inserts a user_game_tag row.
func insertUserGameTag(t *testing.T, db *bun.DB, id, userGameID, tagID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_game_tags (id, user_game_id, tag_id) VALUES (?, ?, ?)`,
		id, userGameID, tagID,
	)
	if err != nil {
		t.Fatalf("insertUserGameTag: %v", err)
	}
}

// setupTagUser inserts a user and session, logs in, and returns (userID, token).
func setupTagUser(t *testing.T, db *bun.DB, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, suffix string) (string, string) {
	t.Helper()
	userID := "u-tags-" + suffix
	username := "taguser-" + suffix
	insertAuthTestUser(t, db, userID, username, "pass123", true, false)
	token := loginAndGetToken(t, handler, username, "pass123")
	return userID, token
}

// ─── TestListTags_Empty ───────────────────────────────────────────────────────

func TestListTags_Empty(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "empty")

	rec := getAuth(t, e, "/api/tags", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tags []any
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tags == nil {
		t.Fatal("expected non-null empty array, got null")
	}
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
	}
}

// ─── TestListTags_WithTags ────────────────────────────────────────────────────

func TestListTags_WithTags(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "withtags")
	red := "red"
	insertTag(t, testDB, "tag-wt-1", userID, "Zebra", &red)
	insertTag(t, testDB, "tag-wt-2", userID, "Alpha", nil)

	rec := getAuth(t, e, "/api/tags", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tags []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	// Sorted by name: Alpha first, Zebra second.
	if tags[0]["name"] != "Alpha" {
		t.Fatalf("expected Alpha first, got %v", tags[0]["name"])
	}
	if tags[1]["name"] != "Zebra" {
		t.Fatalf("expected Zebra second, got %v", tags[1]["name"])
	}
	// game_count should be present and 0
	if gc, ok := tags[0]["game_count"]; !ok {
		t.Fatal("expected game_count field")
	} else if gc.(float64) != 0 {
		t.Fatalf("expected game_count=0, got %v", gc)
	}
}

// ─── TestListTags_UserScoped ──────────────────────────────────────────────────

func TestListTags_UserScoped(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID1, token1 := setupTagUser(t, testDB, e, "scoped1")
	_, token2 := setupTagUser(t, testDB, e, "scoped2")

	insertTag(t, testDB, "tag-scope-1", userID1, "User1Tag", nil)

	// user1 sees their tag
	rec := getAuth(t, e, "/api/tags", token1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var tags1 []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tags1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags1) != 1 {
		t.Fatalf("expected 1 tag for user1, got %d", len(tags1))
	}

	// user2 sees empty list
	rec2 := getAuth(t, e, "/api/tags", token2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	var tags2 []any
	if err := json.Unmarshal(rec2.Body.Bytes(), &tags2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags2) != 0 {
		t.Fatalf("expected 0 tags for user2, got %d", len(tags2))
	}
}

// ─── TestCreateTag ────────────────────────────────────────────────────────────

func TestCreateTag(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupTagUser(t, testDB, e, "create")

	t.Run("success", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]any{
			"name":  "My Tag",
			"color": "#ff0000",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["name"] != "My Tag" {
			t.Fatalf("expected name=My Tag, got %v", resp["name"])
		}
		if resp["color"] != "#ff0000" {
			t.Fatalf("expected color=#ff0000, got %v", resp["color"])
		}
		if resp["id"] == nil || resp["id"] == "" {
			t.Fatal("expected non-empty id")
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		// Ensure "My Tag" exists first (success subtest may have run, but be explicit).
		_, _ = testDB.ExecContext(context.Background(),
			`INSERT INTO tags (id, user_id, name) VALUES ('tag-dup-seed', ?, 'My Tag') ON CONFLICT DO NOTHING`,
			userID,
		)
		// Create again with same name
		rec := postJSONAuth(t, e, "/api/tags", map[string]any{
			"name": "My Tag",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		msg := resp["message"]
		if msg == "" {
			msg = resp["message"]
		}
		if !strings.Contains(msg, "already exists") {
			t.Fatalf("expected 'already exists' error, got %q (body: %s)", msg, rec.Body.String())
		}
	})

	t.Run("missing name", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]any{
			"color": "blue",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("name too long", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]any{
			"name": strings.Repeat("x", 101),
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestUpdateTag ────────────────────────────────────────────────────────────

func TestUpdateTag(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "update")
	insertTag(t, testDB, "tag-upd-1", userID, "Original", nil)
	insertTag(t, testDB, "tag-upd-2", userID, "Other", nil)

	t.Run("success full update", func(t *testing.T) {
		newColor := "#aabbcc"
		rec := putJSONAuth(t, e, "/api/tags/tag-upd-1", map[string]any{
			"name":  "Updated",
			"color": newColor,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["name"] != "Updated" {
			t.Fatalf("expected name=Updated, got %v", resp["name"])
		}
		if resp["color"] != newColor {
			t.Fatalf("expected color=%s, got %v", newColor, resp["color"])
		}
	})

	t.Run("partial update name only", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/tag-upd-1", map[string]any{
			"name": "Renamed",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["name"] != "Renamed" {
			t.Fatalf("expected name=Renamed, got %v", resp["name"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/nonexistent-tag-id", map[string]any{
			"name": "Whatever",
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupTagUser(t, testDB, e, "update-other")
		rec := putJSONAuth(t, e, "/api/tags/tag-upd-2", map[string]any{
			"name": "Stolen",
		}, token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 (wrong owner), got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		// try to rename tag-upd-1 to "Other" (tag-upd-2's name)
		rec := putJSONAuth(t, e, "/api/tags/tag-upd-1", map[string]any{
			"name": "Other",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestUpdateTag_PresenceDetection covers the absent-vs-null-vs-value distinction
// the raw-map decoding gives the tag update handler: an explicit null clears the
// color, an absent color leaves it unchanged, and a null name is rejected.
func TestUpdateTag_PresenceDetection(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "presence")
	color := "#abcdef"
	insertTag(t, testDB, "tag-pres-1", userID, "Colored", &color)

	t.Run("explicit null clears color", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/tag-pres-1", map[string]any{
			"color": nil,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["color"] != nil {
			t.Fatalf("expected color cleared to null, got %v", resp["color"])
		}
	})

	t.Run("absent color leaves it unchanged", func(t *testing.T) {
		// Set a color first, then send a name-only update.
		newColor := "#123456"
		if _, err := testDB.ExecContext(context.Background(),
			`UPDATE tags SET color = ? WHERE id = ?`, newColor, "tag-pres-1"); err != nil {
			t.Fatalf("seed color: %v", err)
		}
		rec := putJSONAuth(t, e, "/api/tags/tag-pres-1", map[string]any{
			"name": "Renamed",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["color"] != newColor {
			t.Fatalf("expected color preserved as %s, got %v", newColor, resp["color"])
		}
	})

	t.Run("explicit null name is rejected", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/tag-pres-1", map[string]any{
			"name": nil,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("no fields to update", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/tag-pres-1", map[string]any{}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestDeleteTag ────────────────────────────────────────────────────────────

func TestDeleteTag(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "delete")
	insertTag(t, testDB, "tag-del-1", userID, "ToDelete", nil)

	t.Run("success", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/tags/tag-del-1", token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/tags/nonexistent-tag-id", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		// Insert a tag owned by a different user.
		userID2 := "u-tags-del-other"
		insertAuthTestUser(t, testDB, userID2, "taguser-del-other", "pass123", true, false)
		insertTag(t, testDB, "tag-del-other", userID2, "OtherUserTag", nil)

		rec := deleteAuth(t, e, "/api/tags/tag-del-other", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 (wrong owner), got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestDeleteTag_CascadesUserGameTags ───────────────────────────────────────

func TestDeleteTag_CascadesUserGameTags(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID, token := setupTagUser(t, testDB, e, "cascade")
	insertTag(t, testDB, "tag-cas-1", userID, "CascadeTag", nil)

	insertGame(t, testDB, 999001, "Test Game")
	insertUserGame(t, testDB, "ug-cas-1", userID, 999001)
	insertUserGameTag(t, testDB, "ugt-cas-1", "ug-cas-1", "tag-cas-1")

	// Verify the user_game_tag exists before deletion.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_tags WHERE id = 'ugt-cas-1'",
	).Scan(&count); err != nil {
		t.Fatalf("pre-check query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected user_game_tag to exist before deletion, count=%d", count)
	}

	rec := deleteAuth(t, e, "/api/tags/tag-cas-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the user_game_tag was cascade-deleted.
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_tags WHERE id = 'ugt-cas-1'",
	).Scan(&count); err != nil {
		t.Fatalf("post-check query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected user_game_tag to be cascade-deleted, count=%d", count)
	}
}

// ─── TestTags_Unauthorized ────────────────────────────────────────────────────

func TestTags_Unauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list", http.MethodGet, "/api/tags"},
		{"create", http.MethodPost, "/api/tags"},
		{"update", http.MethodPut, "/api/tags/some-id"},
		{"delete", http.MethodDelete, "/api/tags/some-id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})
	}
}
