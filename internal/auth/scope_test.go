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
		{"write scope, write method", scopeWrite, http.MethodDelete, "/api/user-games/:id", true},
		{"write scope, read method", scopeWrite, http.MethodGet, "/api/user-games", true},
		{"read scope, GET", scopeRead, http.MethodGet, "/api/user-games", true},
		{"read scope, HEAD", scopeRead, http.MethodHead, "/api/user-games", true},
		{"read scope, OPTIONS", scopeRead, http.MethodOptions, "/api/user-games", true},
		{"read scope, POST write", scopeRead, http.MethodPost, "/api/tags", false},
		{"read scope, PUT write", scopeRead, http.MethodPut, "/api/tags/:id", false},
		{"read scope, PATCH write", scopeRead, http.MethodPatch, "/api/settings", false},
		{"read scope, DELETE write", scopeRead, http.MethodDelete, "/api/user-games/:id", false},
		{"read scope, allowlisted IGDB search POST", scopeRead, http.MethodPost, "/api/games/search/igdb", true},
		{"read scope, csv inspect POST not allowlisted", scopeRead, http.MethodPost, "/api/import/csv/inspect", false},
		{"unknown scope treated as read", "bogus", http.MethodPost, "/api/tags", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeAllowsRequest(tc.scope, tc.method, tc.route); got != tc.allowed {
				t.Errorf("scopeAllowsRequest(%q,%q,%q) = %v, want %v", tc.scope, tc.method, tc.route, got, tc.allowed)
			}
		})
	}
}
