package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestMCPGameEditStatus(t *testing.T) {
	var gotStatus string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "11111111-1111-1111-1111-111111111111",
				"game": map[string]any{"id": 1, "title": "Halo"}})
		case strings.HasSuffix(r.URL.Path, "/progress"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotStatus, _ = body["play_status"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "11111111-1111-1111-1111-111111111111"})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "game_edit",
		Arguments: map[string]any{
			"refs":        []string{"11111111-1111-1111-1111-111111111111"},
			"play_status": "completed",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("edit: err=%v res=%+v", err, res)
	}
	if gotStatus != "completed" {
		t.Fatalf("play_status sent = %q", gotStatus)
	}
}

func TestMCPGameAddAmbiguous(t *testing.T) {
	var importCalled atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/search/igdb", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{
				{"igdb_id": 1, "title": "Halo", "release_date": "2001"},
				{"igdb_id": 2, "title": "Halo 2", "release_date": "2004"},
			},
			"total": 2,
		})
	})
	mux.HandleFunc("/api/games/igdb-import", func(w http.ResponseWriter, _ *http.Request) {
		importCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_add",
		Arguments: map[string]any{"title": "Halo"},
	})
	if err != nil || res.IsError {
		t.Fatalf("game_add: err=%v res=%+v", err, res)
	}
	if importCalled.Load() {
		t.Fatal("import should NOT have been called for ambiguous result")
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out gameAddOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %s", len(out.Candidates), b)
	}
	if out.Candidates[0].IgdbID != 1 || out.Candidates[0].ReleaseDate != "2001" {
		t.Fatalf("candidate[0] = %+v; want igdb_id=1 release_date=2001", out.Candidates[0])
	}
}

func TestMCPGameAcquire(t *testing.T) {
	const id = "33333333-3333-3333-3333-333333333333"
	var moved atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": id, "is_wishlisted": true,
				"game": map[string]any{"id": 3, "title": "Hollow Knight"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/move-to-library"):
			moved.Store(true)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["platforms"]; !ok {
				t.Errorf("move-to-library body missing platforms: %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": id, "game": map[string]any{"id": 3, "title": "Hollow Knight"}})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_acquire",
		Arguments: map[string]any{"ref": id, "platform": "pc-windows/steam"},
	})
	if err != nil || res.IsError {
		t.Fatalf("game_acquire: err=%v res=%+v", err, res)
	}
	if !moved.Load() {
		t.Fatal("expected move-to-library to be called")
	}
}

func TestMCPGameAcquireAmbiguous(t *testing.T) {
	var moved atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}},
				{"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 2, "title": "Halo 2"}},
			},
			"total": 2,
		})
	})
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/move-to-library") {
			moved.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_acquire",
		Arguments: map[string]any{"ref": "halo", "platform": "pc-windows"},
	})
	if err != nil || res.IsError {
		t.Fatalf("game_acquire: err=%v res=%+v", err, res)
	}
	if moved.Load() {
		t.Fatal("move-to-library must NOT be called for an ambiguous ref")
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out gameWriteOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %s", len(out.Candidates), b)
	}
}

func TestMCPGameRm(t *testing.T) {
	var deletedID string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "22222222-2222-2222-2222-222222222222",
				"game": map[string]any{"id": 2, "title": "Doom"}})
		case http.MethodDelete:
			deletedID = strings.TrimPrefix(r.URL.Path, "/api/user-games/")
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_rm",
		Arguments: map[string]any{"refs": []string{"22222222-2222-2222-2222-222222222222"}},
	})
	if err != nil || res.IsError {
		t.Fatalf("game_rm: err=%v res=%+v", err, res)
	}
	if deletedID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("expected delete for id 22222222-..., got %q", deletedID)
	}
}

func TestMCPConfigStanza(t *testing.T) {
	seedProfile(t, "https://example.test")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"mcp", "config"})
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp config: %v\n%s", err, out.String())
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	s, ok := cfg.MCPServers["nexorious"]
	if !ok || s.Command != "nexctl" || len(s.Args) != 2 || s.Args[0] != "mcp" || s.Args[1] != "serve" {
		t.Fatalf("stanza = %+v", cfg)
	}
}

// mcpSession spins up buildMCPServer pointed at restURL and returns a connected
// in-memory client session. Used by tool tests in tasks 4–7.
func mcpSession(t *testing.T, restURL string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := buildMCPServer(clicfg.Profile{URL: restURL, Key: "test-key"})
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPGameList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("play_status = %q", r.URL.Query().Get("play_status"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "play_status": "completed",
					"game": map[string]any{"id": 1, "title": "Halo"}},
			},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_list",
		Arguments: map[string]any{"play_status": "completed"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("Halo")) {
		t.Fatalf("missing game: %s", b)
	}
}

func TestMCPGameShowAmbiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "game": map[string]any{"id": 1, "title": "Metroid Prime"}},
				{"id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "game": map[string]any{"id": 2, "title": "Metroid Prime 2"}},
			},
			"total": 2, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_show",
		Arguments: map[string]any{"ref": "Metroid"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out gameShowOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if out.Game != nil {
		t.Fatalf("expected no Game (ambiguous), got: %+v", out.Game)
	}
	if len(out.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %s", len(out.Candidates), b)
	}
}

func TestMCPGameFilters(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/filter-options", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"genres":              []string{"RPG"},
			"game_modes":          []string{"Single Player"},
			"themes":              []string{"Fantasy"},
			"player_perspectives": []string{"First Person"},
		})
	})
	mux.HandleFunc("/api/platforms/storefronts/simple-list", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "steam", "display_name": "Steam"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "game_filters",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("completed")) {
		t.Fatalf("missing play status 'completed': %s", b)
	}
	if !bytes.Contains(b, []byte("steam")) {
		t.Fatalf("missing storefront 'steam': %s", b)
	}
}

func TestMCPPoolList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "cccccccc-cccc-cccc-cccc-cccccccccccc", "name": "Backlog", "position": 1,
				"has_filter": false, "queue_count": 10, "candidate_count": 5},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pool_list",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("Backlog")) {
		t.Fatalf("missing pool name: %s", b)
	}
}

func TestMCPTagList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "dddddddd-dddd-dddd-dddd-dddddddddddd", "name": "Favourites", "game_count": 7},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "tag_list",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("Favourites")) {
		t.Fatalf("missing tag name: %s", b)
	}
}

func TestMCPPoolCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "33333333-3333-3333-3333-333333333333", "name": "Backlog"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pool_create", Arguments: map[string]any{"name": "Backlog"}})
	if err != nil || res.IsError {
		t.Fatalf("create: err=%v res=%+v", err, res)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out poolWriteOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if out.Pool == nil || out.Pool.ID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("expected pool id 33333333-..., got: %s", b)
	}
}

func TestMCPPoolQueue(t *testing.T) {
	const (
		poolID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		game1ID = "11111111-1111-1111-1111-111111111111"
		game2ID = "22222222-2222-2222-2222-222222222222"
	)
	var bulkCalled, queueCalled atomic.Bool
	mux := http.NewServeMux()
	// pool list for resolution
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": poolID, "name": "Backlog", "position": 1, "has_filter": false, "queue_count": 0, "candidate_count": 0},
		})
	})
	// game fetch by UUID
	mux.HandleFunc("/api/user-games/"+game1ID, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": game1ID, "game": map[string]any{"id": 1, "title": "Halo"},
		})
	})
	mux.HandleFunc("/api/user-games/"+game2ID, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": game2ID, "game": map[string]any{"id": 2, "title": "Doom"},
		})
	})
	// bulk add
	mux.HandleFunc("/api/pools/"+poolID+"/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("bulk-add method = %s", r.Method)
		}
		bulkCalled.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"added": 2})
	})
	// set queue
	mux.HandleFunc("/api/pools/"+poolID+"/queue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("set-queue method = %s", r.Method)
		}
		if !bulkCalled.Load() {
			t.Error("SetQueue called before BulkAddPoolGames")
		}
		queueCalled.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pool_queue",
		Arguments: map[string]any{
			"pool":  poolID,
			"games": []string{game1ID, game2ID},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("pool_queue: err=%v res=%+v", err, res)
	}
	if !bulkCalled.Load() {
		t.Fatal("BulkAddPoolGames was not called")
	}
	if !queueCalled.Load() {
		t.Fatal("SetQueue was not called")
	}
}

func TestMCPTagCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "dddddddd-dddd-dddd-dddd-dddddddddddd", "name": "Favourites", "game_count": 0,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "tag_create", Arguments: map[string]any{"name": "Favourites"},
	})
	if err != nil || res.IsError {
		t.Fatalf("tag_create: err=%v res=%+v", err, res)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out tagWriteOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if out.Tag == nil || out.Tag.Name != "Favourites" {
		t.Fatalf("expected tag name 'Favourites', got: %s", b)
	}
}

func TestMCPSyncStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configs": []map[string]any{
				{"id": "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", "storefront": "steam",
					"frequency": "daily", "is_configured": true,
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
			},
			"total": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "sync_status",
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("steam")) {
		t.Fatalf("missing storefront slug: %s", b)
	}
}
