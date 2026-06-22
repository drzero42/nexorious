package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// deleteJSONAuth fires a DELETE request with a JSON body and a session cookie.
func deleteJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func setupSmellsUser(t *testing.T, suffix string) (string, string, interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}) {
	t.Helper()
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID := "u-smell-" + suffix
	insertAuthTestUser(t, testDB, userID, "smelluser-"+suffix, "pass123", true, false)
	token := loginAndGetToken(t, e, "smelluser-"+suffix, "pass123")
	return userID, token, e
}

func TestSmellsSummary(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "summary")

	// One orphan game (no platform, not wishlisted).
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	rec := getAuth(t, e, "/api/library/smells", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var summary []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summary) != 10 {
		t.Fatalf("expected 10 checks, got %d", len(summary))
	}
	for _, s := range summary {
		if s["id"] == "orphan-game" {
			if int(s["count"].(float64)) != 1 {
				t.Fatalf("expected orphan count 1, got %v", s["count"])
			}
			return
		}
	}
	t.Fatal("orphan-game check missing from summary")
}

func TestSmellsListAndUnknownCheck(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "list")
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	rec := getAuth(t, e, "/api/library/smells/orphan-game", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got total=%d len=%d", resp.Total, len(resp.Items))
	}
	if resp.Items == nil {
		t.Fatal("items must be [] not null")
	}

	// Unknown check → 404.
	rec404 := getAuth(t, e, "/api/library/smells/not-a-check", token)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown check, got %d", rec404.Code)
	}
}

func TestSmellsApplyNotAutoFixable(t *testing.T) {
	truncateAllTables(t)
	_, token, e := setupSmellsUser(t, "apply422")
	rec := postJSONAuth(t, e, "/api/library/smells/orphan-game/apply",
		map[string]any{"user_game_ids": []string{"x"}}, token)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-auto-fixable check, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSmellsIgnoreRestoreAndListDismissed(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "ignore")
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	// Ignore it → listing drops to 0, dismissed listing shows 1.
	recIg := postJSONAuth(t, e, "/api/library/smells/orphan-game/ignore",
		map[string]any{"user_game_ids": []string{"ug-orphan"}}, token)
	if recIg.Code != http.StatusOK {
		t.Fatalf("ignore: expected 200, got %d: %s", recIg.Code, recIg.Body.String())
	}

	recList := getAuth(t, e, "/api/library/smells/orphan-game", token)
	var listResp struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	if listResp.Total != 0 {
		t.Fatalf("expected 0 flagged after ignore, got %d", listResp.Total)
	}

	recDismissed := getAuth(t, e, "/api/library/smells/orphan-game/ignored", token)
	var dis struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recDismissed.Body.Bytes(), &dis)
	if dis.Total != 1 {
		t.Fatalf("expected 1 dismissed, got %d", dis.Total)
	}

	// Restore → flagged again.
	recRestore := deleteJSONAuth(t, e, "/api/library/smells/orphan-game/ignore",
		map[string]any{"user_game_ids": []string{"ug-orphan"}}, token)
	if recRestore.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", recRestore.Code, recRestore.Body.String())
	}
	recList2 := getAuth(t, e, "/api/library/smells/orphan-game", token)
	var l2 struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recList2.Body.Bytes(), &l2)
	if l2.Total != 1 {
		t.Fatalf("expected 1 flagged after restore, got %d", l2.Total)
	}
	_ = context.Background
}
