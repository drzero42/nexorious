package auth

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// API-key scope values. These mirror the values validated and stored by the
// key-creation handler (internal/api/auth.go). "write" implies full access;
// "read" is restricted to safe (non-mutating) requests.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// scopeContextKey is the echo context key AuthMiddleware stores the
// credential's scope under; read it with ScopeFromContext.
const scopeContextKey = "api_scope"

// readOnlyMutatingRoutes are routes that use a mutating HTTP method but perform
// no write and accept no file upload, so a read-scoped key may call them.
// Keyed by the Echo matched-route pattern (c.RouteInfo().Path).
//
// Only POST /api/games/search/igdb qualifies: it must be a POST because it
// carries a JSON query body, but it only reads from IGDB. If you add another
// genuinely read-only route that must use POST/PUT/PATCH/DELETE, add its route
// pattern here — and never add a route that writes data or accepts an upload.
var readOnlyMutatingRoutes = map[string]bool{
	"/api/games/search/igdb": true,
}

// selfScopedRoutes are routes whose handler enforces the key scope itself, so
// the method-based check is skipped for them. Unlike readOnlyMutatingRoutes
// they may well lead to writes — but only through the handler's own check.
//
// /api/mcp: every MCP message is a POST, whether it lists tools or runs one.
// The handler serves a read-scoped key only the read tools (and refuses a
// write tool call), and every REST sub-call a tool makes reuses the caller's
// key, so it passes through this method check again on its own route.
var selfScopedRoutes = map[string]bool{
	"/api/mcp": true,
}

// selfRevokeRoute is the key-revocation route. Any API key may revoke itself
// there whatever its scope: revoking only removes access, and it lets
// `nexctl account logout` revoke a read-scoped key with that same key.
const selfRevokeRoute = "/api/auth/api-keys/:id"

// isSelfRevoke reports whether the request revokes the API key that
// authenticated it. keyID is empty for session-cookie auth, which never needs
// the exemption.
func isSelfRevoke(method, routePath, targetID, keyID string) bool {
	return method == http.MethodDelete && routePath == selfRevokeRoute &&
		keyID != "" && targetID == keyID
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
// "write" scope is unrestricted. "read" scope allows non-mutating methods, the
// readOnlyMutatingRoutes allowlist and the selfScopedRoutes; everything else is
// denied. An unknown/empty scope is treated as "read" (deny mutations) so a
// malformed value fails safe.
func scopeAllowsRequest(scope, method, routePath string) bool {
	if scope == ScopeWrite {
		return true
	}
	// "read" scope (and any unknown/malformed scope, which fails safe to read).
	if !isMutatingMethod(method) {
		return true
	}
	return readOnlyMutatingRoutes[routePath] || selfScopedRoutes[routePath]
}

// ScopeFromContext returns the scope of the credential AuthMiddleware (or
// APIKeyMiddleware) authenticated the request with: the API key's scope, or
// ScopeWrite for a session cookie. Anything other than ScopeWrite — including
// a request that never passed the middleware — reports ScopeRead, so a
// missing or malformed value fails safe.
func ScopeFromContext(c *echo.Context) string {
	if s, _ := c.Get(scopeContextKey).(string); s == ScopeWrite {
		return ScopeWrite
	}
	return ScopeRead
}
