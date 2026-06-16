package auth

import "net/http"

// API-key scope values. These mirror the values validated and stored by the
// key-creation handler (internal/api/auth.go). "write" implies full access;
// "read" is restricted to safe (non-mutating) requests.
const (
	scopeRead  = "read"
	scopeWrite = "write"
)

// readSafeRoutes is the allowlist of routes a read-scoped key may call even
// though they use a mutating HTTP method, because they perform no write and
// accept no file upload. Keyed by the Echo matched-route pattern
// (c.RouteInfo().Path).
//
// Only POST /api/games/search/igdb qualifies: it must be a POST because it
// carries a JSON query body, but it only reads from IGDB. If you add another
// genuinely read-only route that must use POST/PUT/PATCH/DELETE, add its route
// pattern here — and never add a route that writes data or accepts an upload.
var readSafeRoutes = map[string]bool{
	"/api/games/search/igdb": true,
}

// isMutatingMethod reports whether m is a write HTTP method.
func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// scopeAllowsRequest reports whether a credential with the given scope may make
// a request with the given HTTP method against the given matched route pattern.
//
// "write" scope is unrestricted. "read" scope allows non-mutating methods plus
// the readSafeRoutes allowlist; everything else is denied. An unknown/empty
// scope is treated as "read" (deny mutations) so a malformed value fails safe.
func scopeAllowsRequest(scope, method, routePath string) bool {
	if scope == scopeWrite {
		return true
	}
	// "read" scope (and any unknown/malformed scope, which fails safe to read).
	if !isMutatingMethod(method) {
		return true
	}
	return readSafeRoutes[routePath]
}
