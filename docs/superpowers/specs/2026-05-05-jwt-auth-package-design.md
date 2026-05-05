# JWT Auth Package — Design Spec

**Date:** 2026-05-05
**Status:** Approved
**Scope:** `internal/auth/jwt.go` — token generation, validation, hashing, Echo middleware (DB-backed session check), context helpers

---

## Overview

JWT package for access and refresh tokens using `golang-jwt/jwt/v5`, matching the Python implementation's behavior exactly. Tokens carry minimal claims (`sub` + `type`); the middleware validates tokens against the `user_sessions` table on every request and loads user state from the `users` table. This gives instant invalidation on password change, logout, and account deactivation.

The middleware requires a `*pgxpool.Pool` for DB lookups. Token generation and parsing functions remain pure (no DB).

---

## Claims

Both access and refresh tokens use the same claim shape, differentiated by a `type` field — matching the Python `create_access_token` / `create_refresh_token` exactly:

```go
type Claims struct {
    Type string `json:"type"` // "access" or "refresh"
    jwt.RegisteredClaims
}
```

- `Subject` (`sub`): the user's UUID
- `Type`: `"access"` or `"refresh"` — `verify_token` rejects mismatches
- `ExpiresAt`: access = `now + expireMinutes` (default 15 min); refresh = `now + expireDays` (default 30 days)
- `IssuedAt`: `now`
- Signing method: `HS256`

No `is_admin`, `role`, or `session_id` claims. Admin status is loaded from the DB on every request.

---

## Functions

### Token Generation (pure, no DB)

```go
// GenerateAccessToken creates a short-lived JWT with type="access".
func GenerateAccessToken(secretKey string, userID string, expireMinutes int) (string, error)

// GenerateRefreshToken creates a long-lived JWT with type="refresh".
func GenerateRefreshToken(secretKey string, userID string, expireDays int) (string, error)
```

### Token Parsing (pure, no DB)

```go
// ParseToken validates a JWT string, checks the type claim matches expectedType,
// and returns the claims. Returns an error if malformed, expired, wrong key, or wrong type.
func ParseToken(secretKey string, tokenString string, expectedType string) (*Claims, error)
```

Single function for both token types since the claim shape is identical. The `expectedType` parameter ("access" or "refresh") is checked against the `type` claim.

### Token Hashing

```go
// HashToken returns the SHA-256 hex digest of a token string.
// Used for storing and looking up tokens in user_sessions.
func HashToken(token string) string
```

SHA-256 (not bcrypt) — matching the Python `hash_token()`. Tokens are high-entropy random strings, so SHA-256 is sufficient and fast enough for per-request lookups.

---

## Echo Middleware

### JWTMiddleware

```go
func JWTMiddleware(secretKey string, pool *pgxpool.Pool) echo.MiddlewareFunc
```

Behavior (matches Python `get_current_user` exactly):

1. Read `Authorization` header; expect `Bearer <token>`
2. If missing or malformed → `401 {"error": "missing or invalid authorization header"}`
3. Call `ParseToken(secretKey, token, "access")` → if error → `401 {"error": "invalid or expired token"}`
4. Extract `sub` (user ID) from claims
5. Hash the token with `HashToken` and look up in `user_sessions`:
   ```sql
   SELECT id FROM user_sessions
   WHERE user_id = $1 AND token_hash = $2
   ```
   If no row found → `401 {"error": "session not found or expired"}`
6. Load user from `users` table:
   ```sql
   SELECT id, username, is_active, is_admin FROM users WHERE id = $1
   ```
   If not found → `401 {"error": "user not found"}`
   If `is_active = false` → `401 {"error": "user account is disabled"}`
7. Set on Echo context:
   - `c.Set("user_id", user.ID)`
   - `c.Set("is_admin", user.IsAdmin)`
   - `c.Set("user", user)` (full user struct, for handlers that need it)
8. Call `next(c)`

**Why two queries instead of a JOIN?** Matching the Python implementation's structure. The session check confirms the token hasn't been revoked; the user load confirms the account is still active. Both are indexed lookups on small tables. Could be optimized to a single JOIN later if needed, but correctness and parity first.

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

## Token Response Shape

Login (`POST /api/auth/login`) and refresh (`POST /api/auth/refresh`) return:

```json
{
  "access_token": "eyJhbG...",
  "refresh_token": "eyJhbG...",
  "token_type": "bearer",
  "expires_in": 900
}
```

`expires_in` is the access token lifetime in seconds (e.g. 900 for 15 minutes). This matches the Python `TokenResponse` schema exactly. The actual login/refresh handlers are a follow-up task, but the response shape is documented here since it's determined by the token generation design.

---

## Session Lifecycle (for reference)

Not implemented in this package — these are handler-level concerns documented here for context:

- **Login**: creates a `user_sessions` row with `token_hash = HashToken(accessToken)` and `refresh_token_hash = HashToken(refreshToken)`
- **Refresh**: validates the refresh token JWT + looks up `refresh_token_hash` in `user_sessions`; generates a new access token and updates `token_hash` on the session row; returns the same refresh token
- **Logout**: deletes the `user_sessions` row matching `refresh_token_hash`
- **Password change / admin password reset**: deletes ALL `user_sessions` rows for that user
- **Account deactivation (admin)**: deletes ALL `user_sessions` rows for that user

---

## Error Responses

All error responses use a consistent JSON shape:

```json
{"error": "message here"}
```

- `401` — missing/malformed/expired token, revoked session, inactive user
- `403` — valid token but not admin (from `AdminMiddleware`)

---

## DB Queries

The middleware needs two queries. These will be hand-written SQL using `pool.QueryRow` directly (not sqlc) to avoid a circular dependency between `internal/auth` and `internal/db/gen`. The queries are simple indexed lookups:

```sql
-- Session lookup (used by JWTMiddleware step 5)
SELECT id FROM user_sessions WHERE user_id = $1 AND token_hash = $2;

-- User lookup (used by JWTMiddleware step 6)
SELECT id, username, is_active, is_admin FROM users WHERE id = $1;
```

**Why not sqlc?** The auth middleware is imported by `internal/api`, which also imports `internal/db/gen`. Putting auth queries in sqlc would create `internal/auth` → `internal/db/gen` → (shared pool) ← `internal/auth` — not a Go import cycle per se, but it couples the auth package to the generated code. Using `pool.QueryRow` directly keeps `internal/auth` self-contained with only a `*pgxpool.Pool` dependency.

---

## Testing

Unit tests in `internal/auth/jwt_test.go`:

| Test | What it verifies |
|------|-----------------|
| `TestGenerateAndParseAccessToken` | Round-trip: generate → parse with type="access" → claims match |
| `TestGenerateAndParseRefreshToken` | Round-trip: generate → parse with type="refresh" → claims match |
| `TestParseToken_WrongType` | Access token fails parse with expectedType="refresh" and vice versa |
| `TestParseToken_Expired` | Token with past expiry returns error |
| `TestParseToken_WrongKey` | Token signed with key A fails validation with key B |
| `TestParseToken_Malformed` | Garbage string returns error |
| `TestHashToken` | Same input produces same SHA-256 hex output; different inputs differ |
| `TestJWTMiddleware_ValidSession` | DB has matching session + active user → sets context, handler runs (testcontainers) |
| `TestJWTMiddleware_RevokedSession` | Valid JWT but no session row → 401 (testcontainers) |
| `TestJWTMiddleware_InactiveUser` | Valid JWT + session but user is_active=false → 401 (testcontainers) |
| `TestJWTMiddleware_MissingHeader` | Returns 401 (no DB needed) |
| `TestJWTMiddleware_ExpiredToken` | Returns 401 (no DB needed) |
| `TestAdminMiddleware_Admin` | Passes through when is_admin=true in context |
| `TestAdminMiddleware_NonAdmin` | Returns 403 |
| `TestContextHelpers` | `UserIDFromContext` and `IsAdminFromContext` return correct values |

Middleware tests that check DB behavior require testcontainers (real Postgres + migration). Token generation/parsing/hashing tests are pure unit tests.

---

## File Layout

```
internal/auth/
├── jwt.go       # Claims, Generate*, ParseToken, HashToken, middleware, context helpers
└── jwt_test.go  # Unit tests (mix of pure + testcontainers)
```

---

## Dependencies

- `github.com/golang-jwt/jwt/v5` (new — needs `go get`)
- `github.com/labstack/echo/v5` (already in go.mod)
- `github.com/jackc/pgx/v5/pgxpool` (already in go.mod)

---

## Integration Points

After this package is complete, the next tasks can wire it into the router:

1. **Route groups**: `api.New()` applies `JWTMiddleware` to the API route group and `AdminMiddleware` to the admin sub-group
2. **Auth handlers**: `internal/api/auth.go` — login, refresh, logout (requires sqlc queries for `users` and `user_sessions`)
3. **Setup handlers**: `internal/api/auth.go` — setup/status, setup/admin, setup/restore (JWT-exempt, in Setup zone)
