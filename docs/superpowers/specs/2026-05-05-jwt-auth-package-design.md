# JWT Auth Package — Design Spec

**Date:** 2026-05-05
**Status:** Approved
**Scope:** `internal/auth/jwt.go` — token generation, validation, Echo middleware, context helpers

---

## Overview

Self-contained JWT package for access and refresh tokens using `golang-jwt/jwt/v5`. No database dependencies — login/refresh/logout route handlers (which need sqlc queries for `users` and `user_sessions`) are a separate follow-up task.

---

## Claims

### Access Token Claims

```go
type Claims struct {
    UserID  string `json:"user_id"`
    IsAdmin bool   `json:"is_admin"`
    jwt.RegisteredClaims
}
```

- `Subject` (`sub`): set to `userID` (mirrors `UserID` field for JWT standard compliance)
- `ExpiresAt`: `now + expireMinutes` (default 15 min, from `Config.AccessTokenExpireMinutes`)
- `IssuedAt`: `now`
- Signing method: `HS256`

### Refresh Token Claims

```go
type RefreshClaims struct {
    UserID    string `json:"user_id"`
    SessionID string `json:"session_id"`
    jwt.RegisteredClaims
}
```

- `Subject`: set to `userID`
- `ExpiresAt`: `now + expireDays` (default 30 days, from `Config.RefreshTokenExpireDays`)
- `IssuedAt`: `now`
- `SessionID`: references `user_sessions.id` — used by the refresh handler to look up the session row and verify the refresh token hash
- Signing method: `HS256`

---

## Functions

```go
// GenerateAccessToken creates a short-lived JWT carrying user_id and is_admin.
func GenerateAccessToken(secretKey string, userID string, isAdmin bool, expireMinutes int) (string, error)

// GenerateRefreshToken creates a long-lived JWT carrying user_id and session_id.
func GenerateRefreshToken(secretKey string, userID string, sessionID string, expireDays int) (string, error)

// ParseAccessToken validates an access token string and returns its claims.
// Returns an error if the token is malformed, expired, or signed with a different key.
func ParseAccessToken(secretKey string, tokenString string) (*Claims, error)

// ParseRefreshToken validates a refresh token string and returns its claims.
func ParseRefreshToken(secretKey string, tokenString string) (*RefreshClaims, error)
```

All functions are pure — no side effects, no DB calls. `secretKey` is `Config.SecretKey`.

---

## Echo Middleware

### JWTMiddleware

```go
func JWTMiddleware(secretKey string) echo.MiddlewareFunc
```

Behavior:
1. Read `Authorization` header; expect `Bearer <token>`
2. If missing or malformed → `401 {"error": "missing or invalid authorization header"}`
3. Call `ParseAccessToken` → if error → `401 {"error": "invalid or expired token"}`
4. Set claims on Echo context:
   - `c.Set("user_id", claims.UserID)`
   - `c.Set("is_admin", claims.IsAdmin)`
5. Call `next(c)`

### AdminMiddleware

```go
func AdminMiddleware() echo.MiddlewareFunc
```

Stacks after `JWTMiddleware`. Reads `is_admin` from context → `403 {"error": "admin access required"}` if false or absent.

### Context Helpers

```go
// UserIDFromContext extracts user_id set by JWTMiddleware. Returns "" if unset.
func UserIDFromContext(c *echo.Context) string

// IsAdminFromContext extracts is_admin set by JWTMiddleware. Returns false if unset.
func IsAdminFromContext(c *echo.Context) bool
```

---

## Error Responses

All error responses use a consistent JSON shape:

```json
{"error": "message here"}
```

- `401` — missing/malformed/expired token
- `403` — valid token but not admin (from `AdminMiddleware`)

---

## Testing

Unit tests in `internal/auth/jwt_test.go`:

| Test | What it verifies |
|------|-----------------|
| `TestGenerateAndParseAccessToken` | Round-trip: generate → parse → claims match |
| `TestGenerateAndParseRefreshToken` | Round-trip: generate → parse → claims match |
| `TestParseAccessToken_Expired` | Token with past expiry returns error |
| `TestParseAccessToken_WrongKey` | Token signed with key A fails validation with key B |
| `TestParseAccessToken_Malformed` | Garbage string returns error |
| `TestJWTMiddleware_ValidToken` | Sets user_id and is_admin on context, handler runs |
| `TestJWTMiddleware_MissingHeader` | Returns 401 |
| `TestJWTMiddleware_ExpiredToken` | Returns 401 |
| `TestAdminMiddleware_Admin` | Passes through when is_admin=true |
| `TestAdminMiddleware_NonAdmin` | Returns 403 |
| `TestContextHelpers` | `UserIDFromContext` and `IsAdminFromContext` return correct values |

No database or testcontainers needed — all tests are pure unit tests using `echo.New()` + `httptest`.

---

## File Layout

```
internal/auth/
├── jwt.go       # Claims, Generate*, Parse*, middleware, context helpers
└── jwt_test.go  # Unit tests
```

Single file is sufficient — the package is small and cohesive.

---

## Dependencies

- `github.com/golang-jwt/jwt/v5` (new — needs `go get`)
- `github.com/labstack/echo/v5` (already in go.mod)

---

## Integration Points

After this package is complete, the next tasks can wire it into the router:

1. **Route groups**: `api.New()` applies `JWTMiddleware` to the API route group and `AdminMiddleware` to the admin sub-group
2. **Auth handlers**: `internal/api/auth.go` — login, refresh, logout (requires sqlc queries for `users` and `user_sessions`)
3. **Setup handlers**: `internal/api/auth.go` — setup/status, setup/admin, setup/restore (JWT-exempt, in Setup zone)
