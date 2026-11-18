package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listedTools(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	return names
}

func scopedToolNames(want func(Scope) bool) []string {
	var names []string
	for name, s := range toolScopes {
		if want(s) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// TestEveryToolDeclaresScope is the regression guard for scope-aware tools:
// the write server must list exactly the tools in toolScopes. A tool added via
// mcp.AddTool directly (bypassing addTool) or missing from the table, or a
// stale table entry, fails here.
func TestEveryToolDeclaresScope(t *testing.T) {
	got := listedTools(t, mcpSessionScoped(t, "http://unused.invalid", ScopeWrite))
	want := scopedToolNames(func(Scope) bool { return true })
	if !slices.Equal(got, want) {
		t.Errorf("write server tools != toolScopes keys\n got: %v\nwant: %v", got, want)
	}
	for name, s := range toolScopes {
		if s != ScopeRead && s != ScopeWrite {
			t.Errorf("tool %q has invalid scope %q", name, s)
		}
	}
}

func TestReadServerListsOnlyReadTools(t *testing.T) {
	got := listedTools(t, mcpSessionScoped(t, "http://unused.invalid", ScopeRead))
	want := scopedToolNames(func(s Scope) bool { return s == ScopeRead })
	if !slices.Equal(got, want) {
		t.Errorf("read server tools\n got: %v\nwant: %v", got, want)
	}
	if len(want) == 0 {
		t.Fatal("no read tools classified")
	}
}

// A read-scoped server refuses an unlisted write tool with an actionable tool
// error, before any REST call is made.
func TestReadServerRefusesWriteToolCall(t *testing.T) {
	rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected REST call %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	t.Cleanup(rest.Close)
	cs := mcpSessionScoped(t, rest.URL, ScopeRead)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "tag_create", Arguments: map[string]any{"name": "x"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want tool error, got %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "write-scoped key") {
		t.Errorf("error text = %q; want a hint to mint a write-scoped key", text)
	}
}

// An unknown scope value fails safe to read.
func TestScopeAllows(t *testing.T) {
	cases := []struct {
		have, need Scope
		want       bool
	}{
		{ScopeWrite, ScopeWrite, true},
		{ScopeWrite, ScopeRead, true},
		{ScopeRead, ScopeRead, true},
		{ScopeRead, ScopeWrite, false},
		{"bogus", ScopeWrite, false},
		{"bogus", ScopeRead, true},
	}
	for _, tc := range cases {
		if got := tc.have.allows(tc.need); got != tc.want {
			t.Errorf("Scope(%q).allows(%q) = %v; want %v", tc.have, tc.need, got, tc.want)
		}
	}
}
