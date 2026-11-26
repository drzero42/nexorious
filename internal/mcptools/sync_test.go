package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	syncRepID     = "11111111-1111-1111-1111-111111111111"
	syncEditionID = "22222222-2222-2222-2222-222222222222"
)

// syncDecisionServer serves a steam list with one "Spider-Man" entry of two
// editions and records each decision request as "METHOD path?query".
func syncDecisionServer(t *testing.T, requests *[]string) *mcp.ClientSession {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"configs": []map[string]any{{"storefront": "steam"}}, "total": 1})
	})
	mux.HandleFunc("/api/sync/steam/external-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": syncRepID, "title": "Spider-Man", "external_id": "100", "sync_status": "matched",
			"platforms": []string{"pc-linux", "pc-windows"},
			"editions": []map[string]any{
				{"id": syncRepID, "external_id": "100", "platforms": []string{"pc-windows"}},
				{"id": syncEditionID, "external_id": "200", "platforms": []string{"pc-linux"}},
			},
		}})
	})
	record := func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}
	mux.HandleFunc("/api/sync/external-games/", record)
	mux.HandleFunc("/api/sync/ignored/", record)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return mcpSession(t, srv.URL)
}

// decisionArgs are the arguments of tool on ref with only_this_edition.
func decisionArgs(tool, ref string) map[string]any {
	args := map[string]any{"storefront": "steam", "ref": ref, "only_this_edition": true}
	if tool == "sync_resolve" {
		args["igdb_id"] = 5
	}
	return args
}

// sync_resolve and sync_skip with only_this_edition send the request to the
// edition's id as given, never the entry's.
func TestMCPSyncOnlyThisEdition_SendsTheEditionsID(t *testing.T) {
	cases := []struct {
		tool string
		ref  string
		want string
	}{
		{"sync_resolve", syncEditionID, "POST /api/sync/external-games/" + syncEditionID + "/rematch?only_this_edition=true"},
		{"sync_resolve", syncRepID, "POST /api/sync/external-games/" + syncRepID + "/rematch?only_this_edition=true"},
		{"sync_skip", syncEditionID, "POST /api/sync/ignored/" + syncEditionID + "?only_this_edition=true"},
	}
	for _, tc := range cases {
		t.Run(tc.tool+" "+tc.ref, func(t *testing.T) {
			var reqs []string
			cs := syncDecisionServer(t, &reqs)
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: decisionArgs(tc.tool, tc.ref)})
			if err != nil || res.IsError {
				t.Fatalf("%s: err=%v res=%+v", tc.tool, err, res)
			}
			if len(reqs) != 1 || reqs[0] != tc.want {
				t.Fatalf("requests = %q, want [%q]", reqs, tc.want)
			}
		})
	}
}

// A title of an entry with more than one edition returns the entry with its
// editions as candidates and changes nothing.
func TestMCPSyncOnlyThisEdition_AmbiguousTitle_ReturnsEditions(t *testing.T) {
	for _, tool := range []string{"sync_resolve", "sync_skip"} {
		t.Run(tool, func(t *testing.T) {
			var reqs []string
			cs := syncDecisionServer(t, &reqs)
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: decisionArgs(tool, "spider-man")})
			if err != nil || res.IsError {
				t.Fatalf("err=%v res=%+v", err, res)
			}
			if len(reqs) != 0 {
				t.Fatalf("requests sent: %q", reqs)
			}
			b, _ := json.Marshal(res.StructuredContent)
			var out syncMatchOutput
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Candidates) != 1 || len(out.Candidates[0].Editions) != 2 {
				t.Fatalf("candidates: want one entry with two editions, got %s", b)
			}
			e := out.Candidates[0].Editions[1]
			if e.ID != syncEditionID || e.ExternalID != "200" || strings.Join(e.Platforms, ",") != "pc-linux" {
				t.Errorf("edition candidate: %+v", e)
			}
		})
	}
}

// Without the option, a title of one entry goes to the entry's id and acts on
// the set.
func TestMCPSyncResolve_Title_ActsOnTheSet(t *testing.T) {
	var reqs []string
	cs := syncDecisionServer(t, &reqs)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "sync_resolve", Arguments: map[string]any{
		"storefront": "steam", "ref": "spider-man", "igdb_id": 5,
	}})
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	want := "POST /api/sync/external-games/" + syncRepID + "/rematch?"
	if len(reqs) != 1 || reqs[0] != want {
		t.Fatalf("requests = %q, want [%q]", reqs, want)
	}
}
