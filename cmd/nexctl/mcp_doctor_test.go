package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPSmellsListAndDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/library/smells" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "tier": "inconsistency", "auto_fixable": false, "count": 1},
		})
	})
	mux.HandleFunc("/api/library/smells/orphan-game", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u1", "game_id": 1, "title": "Tetris"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "library_smells_list"})
	if err != nil || res.IsError {
		t.Fatalf("list: err=%v res=%+v", err, res)
	}
	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_detail", Arguments: map[string]any{"check_id": "orphan-game"}})
	if err != nil || res.IsError {
		t.Fatalf("detail: err=%v res=%+v", err, res)
	}
}

func TestMCPSmellsApplyByRef(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 1, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_apply",
		Arguments: map[string]any{
			"check_id": "wishlisted-yet-owned",
			"refs":     []string{"11111111-1111-1111-1111-111111111111"},
		}})
	if err != nil || res.IsError {
		t.Fatalf("apply: err=%v res=%+v", err, res)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("applied ids = %v", gotIDs)
	}
}

func TestMCPSmellsIgnoreRestore(t *testing.T) {
	var ignored, restored bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 1, "title": "Doom"}})
	})
	mux.HandleFunc("/api/library/smells/orphan-game/ignore", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ignored = true
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			restored = true
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	ref := "22222222-2222-2222-2222-222222222222"

	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_ignore", Arguments: map[string]any{"check_id": "orphan-game", "refs": []string{ref}}}); err != nil || res.IsError {
		t.Fatalf("ignore: err=%v res=%+v", err, res)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_restore", Arguments: map[string]any{"check_id": "orphan-game", "refs": []string{ref}}}); err != nil || res.IsError {
		t.Fatalf("restore: err=%v res=%+v", err, res)
	}
	if !ignored || !restored {
		t.Fatalf("ignored=%v restored=%v", ignored, restored)
	}
}
