package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/clicfg"
)

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
