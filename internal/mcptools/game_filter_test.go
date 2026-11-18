package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// filterTargetStub serves a filtered GET /api/user-games that returns one game
// but reports total, and records every request so a test can assert that no
// mutation (or no lookup at all) reached the REST API (#1252).
type filterTargetStub struct {
	mu   sync.Mutex
	reqs []string
}

func (s *filterTargetStub) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.reqs...)
}

func newFilterTargetStub(t *testing.T, total int) (*filterTargetStub, string) {
	t.Helper()
	s := &filterTargetStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.reqs = append(s.reqs, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		if r.Method == http.MethodGet && r.URL.Path == "/api/user-games" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_games": []map[string]any{{
					"id":          stubGame1,
					"play_status": "not_started",
					"game":        map[string]any{"id": 1, "title": "Doom"},
				}},
				"total": total,
			})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func callGameTool(t *testing.T, url, name string, args map[string]any) (*mcp.CallToolResult, gameWriteOutput, string) {
	t.Helper()
	res, err := mcpSession(t, url).CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text string
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	var out gameWriteOutput
	if res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(b, &out)
	}
	return res, out, text
}

func TestMCPGameFilterTargets(t *testing.T) {
	writeArgs := map[string]map[string]any{
		"game_edit": {"play_status": "completed"},
		"game_rm":   {},
	}
	for tool, extra := range writeArgs {
		withArgs := func(m map[string]any) map[string]any {
			for k, v := range extra {
				m[k] = v
			}
			return m
		}

		t.Run(tool+" rejects refs with filter", func(t *testing.T) {
			s, url := newFilterTargetStub(t, 1)
			res, _, text := callGameTool(t, url, tool, withArgs(map[string]any{
				"refs":   []string{stubGame1},
				"filter": map[string]any{},
			}))
			if !res.IsError || !strings.Contains(text, "not both") {
				t.Errorf("want a 'not both' error; IsError=%v text=%q", res.IsError, text)
			}
			if reqs := s.requests(); len(reqs) != 0 {
				t.Errorf("no REST call expected; got %v", reqs)
			}
		})

		t.Run(tool+" refuses a filter over one page", func(t *testing.T) {
			s, url := newFilterTargetStub(t, 201)
			res, out, text := callGameTool(t, url, tool, withArgs(map[string]any{
				"filter": map[string]any{"play_status": "not_started"},
			}))
			if res.IsError {
				t.Fatalf("want a message, not an error: %s", text)
			}
			if !strings.Contains(out.Message, "201") || len(out.Updated)+len(out.Removed) != 0 {
				t.Errorf("want a refusal naming 201 with nothing changed; got %+v", out)
			}
			for _, r := range s.requests() {
				if !strings.HasPrefix(r, http.MethodGet) {
					t.Errorf("no mutation expected; got %s", r)
				}
			}
		})

		t.Run(tool+" applies a filter within one page", func(t *testing.T) {
			s, url := newFilterTargetStub(t, 1)
			res, _, text := callGameTool(t, url, tool, withArgs(map[string]any{
				"filter": map[string]any{"play_status": "not_started"},
			}))
			if res.IsError {
				t.Fatalf("%s: %s", tool, text)
			}
			mutated := false
			for _, r := range s.requests() {
				if !strings.HasPrefix(r, http.MethodGet) {
					mutated = true
				}
			}
			if !mutated {
				t.Errorf("want a mutation; got %v", s.requests())
			}
		})
	}
}
