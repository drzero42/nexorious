package clientops

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// smellStub serves a check flagging n games and enforces the server's
// 200-ids-per-request cap on apply/ignore/restore. failOnBatch (1-based, 0 =
// never) makes that apply batch fail with a 500. It records each batch size.
type smellStub struct {
	n           int
	failOnBatch int
	batches     []int
}

func (s *smellStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/library/smells/big", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		var items []map[string]any
		for i := (page - 1) * perPage; i < min(page*perPage, s.n); i++ {
			items = append(items, map[string]any{"user_game_id": fmt.Sprintf("ug-%d", i), "game_id": i, "title": "g"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": items, "total": s.n, "page": page, "per_page": perPage,
			"pages": (s.n + perPage - 1) / perPage,
		})
	})
	idsHandler := func(key string, count func(ids []string) int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				IDs []string `json:"user_game_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode: %v", err)
			}
			if len(body.IDs) > 200 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "too many user_game_ids (max 200)"})
				return
			}
			s.batches = append(s.batches, len(body.IDs))
			if s.failOnBatch == len(s.batches) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "apply failed"})
				return
			}
			n := count(body.IDs)
			if key == "applied" {
				_ = json.NewEncoder(w).Encode(map[string]int{"applied": n, "skipped": len(body.IDs) - n})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int{key: n})
		}
	}
	// Apply skips one game per batch so the summed skipped count is checked too.
	mux.HandleFunc("POST /api/library/smells/big/apply", idsHandler("applied", func(ids []string) int { return len(ids) - 1 }))
	mux.HandleFunc("POST /api/library/smells/big/ignore", idsHandler("ignored", func(ids []string) int { return len(ids) }))
	mux.HandleFunc("DELETE /api/library/smells/big/ignore", idsHandler("restored", func(ids []string) int { return len(ids) }))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Regression for #1408: "apply all" on a check flagging more than 200 games
// sent every id in one request and the server refused it.
func TestApplySmellAllBatchesOverServerCap(t *testing.T) {
	stub := &smellStub{n: 450}
	c := cliclient.New(stub.server(t).URL)

	ids, err := CollectFlaggedIDs(t.Context(), c, "k", "big")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(ids) != 450 {
		t.Fatalf("collected %d ids, want 450", len(ids))
	}
	res, err := ApplySmell(t.Context(), c, "k", "big", ids)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if fmt.Sprint(stub.batches) != "[200 200 50]" {
		t.Fatalf("batches = %v, want [200 200 50]", stub.batches)
	}
	if res.Applied != 447 || res.Skipped != 3 {
		t.Fatalf("result = %+v, want applied 447 skipped 3", res)
	}
}

func TestIgnoreAndRestoreSmellBatch(t *testing.T) {
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("ug-%d", i)
	}

	stub := &smellStub{}
	c := cliclient.New(stub.server(t).URL)
	n, err := IgnoreSmell(t.Context(), c, "k", "big", ids)
	if err != nil || n != 201 {
		t.Fatalf("ignore = %d, %v; want 201", n, err)
	}
	n, err = RestoreSmell(t.Context(), c, "k", "big", ids)
	if err != nil || n != 201 {
		t.Fatalf("restore = %d, %v; want 201", n, err)
	}
	if fmt.Sprint(stub.batches) != "[200 1 200 1]" {
		t.Fatalf("batches = %v, want [200 1 200 1]", stub.batches)
	}
}

// A failure after a batch went through must say how far it got; a failure on
// the first batch must return the server's error unwrapped, so
// mcptools.toolError can still rewrite it.
func TestApplySmellBatchFailure(t *testing.T) {
	ids := make([]string, 450)
	for i := range ids {
		ids[i] = fmt.Sprintf("ug-%d", i)
	}

	t.Run("part way", func(t *testing.T) {
		stub := &smellStub{failOnBatch: 2}
		_, err := ApplySmell(t.Context(), cliclient.New(stub.server(t).URL), "k", "big", ids)
		if err == nil {
			t.Fatal("want error")
		}
		if !strings.Contains(err.Error(), "stopped after 200 of 450 game(s) (applied 199, skipped 1)") {
			t.Fatalf("error lacks progress: %v", err)
		}
		var he *cliclient.HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusInternalServerError {
			t.Fatalf("error does not wrap the HTTPError: %v", err)
		}
		if fmt.Sprint(stub.batches) != "[200 200]" {
			t.Fatalf("batches = %v; must stop at the failed batch", stub.batches)
		}
	})

	t.Run("first batch", func(t *testing.T) {
		stub := &smellStub{failOnBatch: 1}
		_, err := ApplySmell(t.Context(), cliclient.New(stub.server(t).URL), "k", "big", ids)
		if _, ok := err.(*cliclient.HTTPError); !ok {
			t.Fatalf("err = %T %v, want a bare *cliclient.HTTPError", err, err)
		}
	})
}
