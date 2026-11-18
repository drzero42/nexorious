package auth

import (
	"net/http"
	"testing"
)

func TestScopeAllowsRequest(t *testing.T) {
	cases := []struct {
		name    string
		scope   string
		method  string
		route   string
		allowed bool
	}{
		{"write scope, write method", ScopeWrite, http.MethodDelete, "/api/user-games/:id", true},
		{"write scope, read method", ScopeWrite, http.MethodGet, "/api/user-games", true},
		{"read scope, GET", ScopeRead, http.MethodGet, "/api/user-games", true},
		{"read scope, HEAD", ScopeRead, http.MethodHead, "/api/user-games", true},
		{"read scope, OPTIONS", ScopeRead, http.MethodOptions, "/api/user-games", true},
		{"read scope, POST write", ScopeRead, http.MethodPost, "/api/tags", false},
		{"read scope, PUT write", ScopeRead, http.MethodPut, "/api/tags/:id", false},
		{"read scope, PATCH write", ScopeRead, http.MethodPatch, "/api/settings", false},
		{"read scope, DELETE write", ScopeRead, http.MethodDelete, "/api/user-games/:id", false},
		{"read scope, allowlisted IGDB search POST", ScopeRead, http.MethodPost, "/api/games/search/igdb", true},
		{"read scope, csv inspect POST not allowlisted", ScopeRead, http.MethodPost, "/api/import/csv/inspect", false},
		{"unknown scope treated as read", "bogus", http.MethodPost, "/api/tags", false},
		// /api/mcp enforces scope per tool itself; every MCP message is a POST.
		{"read scope, self-scoped MCP POST", ScopeRead, http.MethodPost, "/api/mcp", true},
		{"read scope, MCP-like prefix not exempt", ScopeRead, http.MethodPost, "/api/mcp/x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeAllowsRequest(tc.scope, tc.method, tc.route); got != tc.allowed {
				t.Errorf("scopeAllowsRequest(%q,%q,%q) = %v, want %v", tc.scope, tc.method, tc.route, got, tc.allowed)
			}
		})
	}
}
