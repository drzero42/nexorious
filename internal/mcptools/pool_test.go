package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// poolStub is a stateful REST stub for the pool write tools: mutations change
// the counts/positions that GET /api/pools reports, so a tool that projects
// the pre-mutation pool is caught (#1289).
type poolStub struct {
	mu    sync.Mutex
	pools map[string]map[string]any
	// filters records the raw "filter" body of each create/update, in order.
	filters []string
}

func (s *poolStub) list() []map[string]any {
	out := make([]map[string]any, 0, len(s.pools))
	for _, p := range s.pools {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["position"].(int) < out[j]["position"].(int) })
	return out
}

func (s *poolStub) inc(id, field string, by int) {
	s.pools[id][field] = s.pools[id][field].(int) + by
}

func newPoolStub(t *testing.T, games ...string) (*poolStub, string) {
	t.Helper()
	s := &poolStub{pools: map[string]map[string]any{}}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("GET /api/pools", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		write(w, s.list())
	})
	mux.HandleFunc("POST /api/pools", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body struct {
			Name   string          `json:"name"`
			Filter json.RawMessage `json:"filter"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.filters = append(s.filters, string(body.Filter))
		id := "33333333-3333-3333-3333-333333333333"
		s.pools[id] = map[string]any{"id": id, "name": body.Name, "position": len(s.pools) + 1,
			"has_filter": len(body.Filter) > 0, "queue_count": 0, "candidate_count": 0}
		write(w, map[string]any{"id": id, "name": body.Name})
	})
	mux.HandleFunc("PUT /api/pools/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.filters = append(s.filters, string(body["filter"]))
		p := s.pools[r.PathValue("id")]
		var name string
		if json.Unmarshal(body["name"], &name) == nil {
			p["name"] = name
		}
		write(w, map[string]any{"id": p["id"], "name": p["name"]})
	})
	mux.HandleFunc("POST /api/pools/{id}/games", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.inc(r.PathValue("id"), "candidate_count", 1)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/pools/{id}/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body struct {
			IDs []string `json:"user_game_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.inc(r.PathValue("id"), "candidate_count", len(body.IDs))
		write(w, map[string]any{"added": len(body.IDs)})
	})
	mux.HandleFunc("DELETE /api/pools/{id}/games/{gid}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.inc(r.PathValue("id"), "candidate_count", -1)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/pools/{id}/queue", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.pools[r.PathValue("id")]["queue_count"] = len(body.IDs)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/pools/reorder", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for i, id := range body.IDs {
			s.pools[id]["position"] = i + 1
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for i, gid := range games {
		mux.HandleFunc("GET /api/user-games/"+gid, func(w http.ResponseWriter, _ *http.Request) {
			write(w, map[string]any{"id": gid, "game": map[string]any{"id": i + 1, "title": "Game"}})
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func (s *poolStub) add(id, name string, position, queue, candidates int) {
	s.pools[id] = map[string]any{"id": id, "name": name, "position": position,
		"has_filter": false, "queue_count": queue, "candidate_count": candidates}
}

func callPoolTool[T any](t *testing.T, url, name string, args map[string]any) T {
	t.Helper()
	res, err := mcpSession(t, url).CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || res.IsError {
		if res != nil && len(res.Content) > 0 {
			b, _ := json.Marshal(res.Content[0])
			t.Fatalf("%s: err=%v content=%s", name, err, b)
		}
		t.Fatalf("%s: err=%v res=%+v", name, err, res)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s: unmarshal: %v\n%s", name, err, b)
	}
	return out
}

const (
	stubPoolA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	stubPoolB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	stubGame1 = "11111111-1111-1111-1111-111111111111"
	stubGame2 = "22222222-2222-2222-2222-222222222222"
	stubGame3 = "44444444-4444-4444-4444-444444444444"
)

func TestMCPPoolWriteToolsReportPostMutationState(t *testing.T) {
	t.Run("pool_add bulk", func(t *testing.T) {
		s, url := newPoolStub(t, stubGame1, stubGame2, stubGame3)
		s.add(stubPoolA, "Backlog", 1, 0, 2)
		out := callPoolTool[poolGamesOutput](t, url, "pool_add",
			map[string]any{"pool": stubPoolA, "games": []string{stubGame1, stubGame2, stubGame3}})
		if out.Pool == nil || out.Pool.CandidateCount != 5 {
			t.Errorf("pool = %+v; want candidate_count 5", out.Pool)
		}
	})
	t.Run("pool_add single", func(t *testing.T) {
		s, url := newPoolStub(t, stubGame1)
		s.add(stubPoolA, "Backlog", 1, 0, 2)
		out := callPoolTool[poolGamesOutput](t, url, "pool_add",
			map[string]any{"pool": stubPoolA, "games": []string{stubGame1}})
		if out.Pool == nil || out.Pool.CandidateCount != 3 || out.Count != 1 {
			t.Errorf("out = %+v pool = %+v; want count 1, candidate_count 3", out, out.Pool)
		}
	})
	t.Run("pool_remove", func(t *testing.T) {
		s, url := newPoolStub(t, stubGame1, stubGame2)
		s.add(stubPoolA, "Backlog", 1, 0, 5)
		out := callPoolTool[poolGamesOutput](t, url, "pool_remove",
			map[string]any{"pool": stubPoolA, "games": []string{stubGame1, stubGame2}})
		if out.Pool == nil || out.Pool.CandidateCount != 3 {
			t.Errorf("pool = %+v; want candidate_count 3", out.Pool)
		}
	})
	t.Run("pool_queue", func(t *testing.T) {
		s, url := newPoolStub(t, stubGame1, stubGame2)
		s.add(stubPoolA, "Backlog", 1, 0, 0)
		out := callPoolTool[poolGamesOutput](t, url, "pool_queue",
			map[string]any{"pool": stubPoolA, "games": []string{stubGame1, stubGame2}})
		if out.Pool == nil || out.Pool.QueueCount != 2 {
			t.Errorf("pool = %+v; want queue_count 2", out.Pool)
		}
	})
	t.Run("pool_reorder", func(t *testing.T) {
		s, url := newPoolStub(t)
		s.add(stubPoolA, "Backlog", 1, 0, 0)
		s.add(stubPoolB, "Next", 2, 0, 0)
		out := callPoolTool[poolWriteOutput](t, url, "pool_reorder",
			map[string]any{"pools": []string{stubPoolB, stubPoolA}})
		if len(out.Pools) != 2 || out.Pools[0].ID != stubPoolB || out.Pools[0].Position != 1 ||
			out.Pools[1].ID != stubPoolA || out.Pools[1].Position != 2 {
			t.Errorf("pools = %+v; want Next@1, Backlog@2", out.Pools)
		}
	})
	t.Run("pool_edit", func(t *testing.T) {
		s, url := newPoolStub(t)
		s.add(stubPoolA, "Backlog", 2, 4, 7)
		out := callPoolTool[poolWriteOutput](t, url, "pool_edit",
			map[string]any{"ref": stubPoolA, "name": "Renamed"})
		if out.Pool == nil || out.Pool.Name != "Renamed" || out.Pool.Position != 2 ||
			out.Pool.QueueCount != 4 || out.Pool.CandidateCount != 7 {
			t.Errorf("pool = %+v; want Renamed@2 with queue 4, candidates 7", out.Pool)
		}
	})
	t.Run("pool_create", func(t *testing.T) {
		s, url := newPoolStub(t)
		s.add(stubPoolA, "Backlog", 1, 0, 0)
		out := callPoolTool[poolWriteOutput](t, url, "pool_create",
			map[string]any{"name": "RPGs"})
		if out.Pool == nil || out.Pool.ID != "33333333-3333-3333-3333-333333333333" ||
			out.Pool.Position != 2 {
			t.Errorf("pool = %+v; want new pool at position 2", out.Pool)
		}
	})
}

// TestMCPPoolFilterAcceptsObject guards #1293: a json.RawMessage input made
// the SDK infer filter as null|array, so the documented object was rejected.
func TestMCPPoolFilterAcceptsObject(t *testing.T) {
	const filter = `{"filters":[{"genre":["RPG"]}]}`
	s, url := newPoolStub(t)
	s.add(stubPoolA, "Backlog", 1, 0, 0)
	var obj map[string]any
	if err := json.Unmarshal([]byte(filter), &obj); err != nil {
		t.Fatal(err)
	}
	callPoolTool[poolWriteOutput](t, url, "pool_create", map[string]any{"name": "RPGs", "filter": obj})
	callPoolTool[poolWriteOutput](t, url, "pool_edit", map[string]any{"ref": stubPoolA, "filter": obj})
	callPoolTool[poolWriteOutput](t, url, "pool_edit", map[string]any{"ref": stubPoolA, "clear_filter": true})

	s.mu.Lock()
	defer s.mu.Unlock()
	want := []string{filter, filter, "null"}
	if len(s.filters) != len(want) {
		t.Fatalf("filters sent = %q; want %q", s.filters, want)
	}
	for i := range want {
		if s.filters[i] != want[i] {
			t.Errorf("call %d sent filter %s; want %s", i, s.filters[i], want[i])
		}
	}
}
