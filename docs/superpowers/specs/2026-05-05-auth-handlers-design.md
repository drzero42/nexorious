# Auth Handlers — Design Spec

**Date:** 2026-05-05
**Status:** Draft
**Phase:** 1 (Infrastructure Skeleton)
**Depends on:** `internal/auth/jwt.go` (complete), `0001_initial.up.sql` (complete)

## Scope

Implement three auth API handlers in `internal/api/auth.go`:

- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/logout`

These are the minimum endpoints needed for the frontend to authenticate. Profile management (`GET /api/auth/me`, `PUT /api/auth/me`, etc.) and first-run setup are separate tasks.

---

## Existing Infrastructure

The following already exists and must be reused:

| Component | Location | Provides |
|---|---|---|
| JWT generation | `internal/auth/jwt.go` | `GenerateAccessToken`, `GenerateRefreshToken`, `ParseToken`, `HashToken` |
| JWT middleware | `internal/auth/jwt.go` | `JWTMiddleware`, `AdminMiddleware`, `UserIDFromContext`, `IsAdminFromContext`, `AuthUser` |
| Config | `internal/config/config.go` | `SecretKey`, `AccessTokenExpireMinutes`, `RefreshTokenExpireDays` |
| DB schema | `internal/db/migrations/0001_initial.up.sql` | `users` table (bcrypt hash), `user_sessions` table |
| Router | `internal/api/router.go` | Echo instance with middleware stack |

**Password hashing:** The Python version uses bcrypt with 12 rounds (`passlib.CryptContext(schemes=["bcrypt"], bcrypt__rounds=12)`). The Go port must use `golang.org/x/crypto/bcrypt` with cost 12 to produce compatible hashes.

**Schema difference from Python:** The Python `UserSession` model has an `updated_at` column that the refresh handler sets. The Go schema's `user_sessions` table does **not** have `updated_at` — it is unnecessary because the only mutation is updating `token_hash`, and the creation/expiry timestamps are sufficient for operational purposes. The refresh handler updates `token_hash` only.

**`is_admin` is not in the JWT:** The existing `JWTMiddleware` loads `is_admin` from the `users` table on every request rather than embedding it as a JWT claim. This means the login handler does not need to put `is_admin` into the token — it only needs `Subject` (user_id) and `Type` ("access"/"refresh"), which `GenerateAccessToken`/`GenerateRefreshToken` already handle.

---

## New File: `internal/api/auth.go`

### Handler Struct

```go
type AuthHandler struct {
    pool      *pgxpool.Pool
    secretKey string
    cfg       *config.Config
}

func NewAuthHandler(pool *pgxpool.Pool, cfg *config.Config) *AuthHandler
```

The handler uses raw `pool.QueryRow` / `pool.Exec` (not sqlc) — matching the pattern established in `internal/auth/jwt.go` to avoid coupling auth to `internal/db/gen`.

### Dependencies

New Go dependency: `golang.org/x/crypto/bcrypt`

---

## Endpoint Specifications

### `POST /api/auth/login`

**Auth:** None (public endpoint, no JWT required)

**Request body:**
```json
{
  "username": "string",
  "password": "string"
}
```

**Success response (200):**
```json
{
  "access_token": "string",
  "refresh_token": "string",
  "token_type": "bearer",
  "expires_in": 900
}
```

`expires_in` is the access token lifetime in seconds (derived from `cfg.AccessTokenExpireMinutes * 60`).

**Error responses:**
- `400` — `{"message": "invalid request body"}` — malformed JSON or missing required fields
- `401` — `{"message": "incorrect username or password"}` — wrong credentials OR user not found (same message to prevent user enumeration)
- `401` — `{"message": "user account is disabled"}` — `is_active = false`

**Logic:**

1. Parse and validate request body (both fields required, non-empty)
2. `SELECT id, username, password_hash, is_active, is_admin FROM users WHERE username = $1`
3. If no row → 401 "incorrect username or password"
4. `bcrypt.CompareHashAndPassword(hash, password)` → if mismatch → 401 "incorrect username or password"
5. If `!is_active` → 401 "user account is disabled"
6. Generate access token via `auth.GenerateAccessToken(secretKey, userID, cfg.AccessTokenExpireMinutes)`
7. Generate refresh token via `auth.GenerateRefreshToken(secretKey, userID, cfg.RefreshTokenExpireDays)`
8. Generate UUID for session ID
9. Insert `user_sessions` row:
   - `id` = generated UUID
   - `user_id` = user.ID
   - `token_hash` = `auth.HashToken(accessToken)`
   - `refresh_token_hash` = `auth.HashToken(refreshToken)`
   - `user_agent` = `c.Request().Header.Get("User-Agent")`
   - `ip_address` = `c.RealIP()`
   - `expires_at` = `now() + RefreshTokenExpireDays`
10. Return token response

**UUID generation:** Use `github.com/google/uuid` (`uuid.NewString()`). This is already an indirect dependency via testcontainers — add it as a direct dependency.

---

### `POST /api/auth/refresh`

**Auth:** None (public endpoint — the refresh token itself is the credential)

**Request body:**
```json
{
  "refresh_token": "string"
}
```

**Success response (200):** Same shape as login:
```json
{
  "access_token": "string (new)",
  "refresh_token": "string (same as request — unchanged)",
  "token_type": "bearer",
  "expires_in": 900
}
```

The refresh token is **not rotated** — the same refresh token is echoed back. Only the access token is new. This matches the Python implementation.

**Error responses:**
- `400` — `{"message": "invalid request body"}` — malformed JSON or missing refresh_token
- `401` — `{"message": "invalid refresh token"}` — malformed, expired, wrong type, or missing subject
- `401` — `{"message": "invalid or expired refresh token"}` — no matching `user_sessions` row (deleted session or expired)
- `401` — `{"message": "user not found or disabled"}` — user row missing or `is_active = false`

**Logic:**

1. Parse request body (refresh_token required, non-empty)
2. `auth.ParseToken(secretKey, refreshToken, "refresh")` → if error → 401 "invalid refresh token"
3. Extract `userID` from claims.Subject → if empty → 401 "invalid refresh token"
4. Hash the refresh token: `auth.HashToken(refreshToken)`
5. `SELECT id FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2 AND expires_at > now()` → if no row → 401 "invalid or expired refresh token"
6. `SELECT id, username, is_active, is_admin FROM users WHERE id = $1` → if no row or `!is_active` → 401 "user not found or disabled"
7. Generate new access token
8. Update session: `UPDATE user_sessions SET token_hash = $1 WHERE id = $2` (the session ID from step 5)
9. Return token response with new access token and **same** refresh token

---

### `POST /api/auth/logout`

**Auth:** JWT required (uses `JWTMiddleware`)

**Request body:**
```json
{
  "refresh_token": "string"
}
```

**Success response (200):**
```json
{
  "message": "Successfully logged out"
}
```

**Error responses:**
- `401` — from JWTMiddleware (missing/invalid/expired access token)
- `400` — `{"message": "invalid refresh token for authenticated user"}` — refresh token's subject doesn't match the authenticated user

**Logic:**

1. Get authenticated user ID from context: `auth.UserIDFromContext(c)`
2. Parse request body (refresh_token required)
3. `auth.ParseToken(secretKey, refreshToken, "refresh")` — if error, log it but still return 200 (security: don't leak token validity on logout)
4. If token is valid: check `claims.Subject == userID` → if mismatch → 400 "invalid refresh token for authenticated user"
5. Hash the refresh token: `auth.HashToken(refreshToken)`
6. `DELETE FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2`
7. Return success regardless of whether a row was actually deleted (idempotent)

**Note on error handling:** The Python version catches parse errors and still returns 200 for security (a logout request should not reveal whether a refresh token is valid). The Go port matches this: if the refresh token fails to parse, skip the user-mismatch check and the delete, and return 200.

**Frontend note:** The current frontend (`auth-provider.tsx`) performs logout entirely client-side — it clears localStorage and navigates to `/login` without calling `POST /api/auth/logout`. No frontend code calls this endpoint. It is still implemented for API completeness (other clients, curl, future frontend changes) and to match the Python backend, but the initial frontend will not exercise it.

---

## Route Registration

In `internal/api/router.go`, add to `registerRoutes`:

```go
ah := NewAuthHandler(pool, cfg)

// Auth routes (public — no JWT)
e.POST("/api/auth/login", ah.HandleLogin)
e.POST("/api/auth/refresh", ah.HandleRefresh)

// Auth routes (JWT required)
authGroup := e.Group("/api/auth", auth.JWTMiddleware(cfg.SecretKey, pool))
authGroup.POST("/logout", ah.HandleLogout)
```

This requires `router.go`'s `New` function to accept `*pgxpool.Pool` in addition to its current parameters. Update the signature:

```go
func New(cfg *config.Config, migrator *migrate.Migrator, pool *pgxpool.Pool) *echo.Echo
```

And update the call site in `cmd/nexorious/main.go` accordingly.

**Existing test impact:** `internal/api/router_test.go` calls `api.New(cfg, m)` without a pool. These tests must be updated to pass `nil` for the pool parameter (the tests exercise the app-state middleware and SPA handler, which don't touch the DB). The auth routes won't be registered when pool is `nil` — `NewAuthHandler` should guard against this, or `registerRoutes` should skip auth route registration when pool is nil. The simplest approach: `registerRoutes` checks `pool != nil` before registering auth routes. This keeps the existing router tests working with minimal changes (just adding `, nil` to the `api.New` call).

### App-state middleware update

Login, refresh, and logout are in the **API zone** — they must be gated by the app-state middleware (require `Ready` state). The existing app-state middleware already blocks all non-migration routes when state is not `Ready`, so no changes are needed for these endpoints. They will naturally return a redirect to `/migrate` if the app isn't ready.

---

## Testing: `internal/api/auth_test.go`

Tests use `testcontainers-go` with a real PostgreSQL instance, matching the pattern in `internal/auth/jwt_test.go`.

### Test helper

A shared helper function creates a test user with a known bcrypt-hashed password:

```go
func insertTestUser(t *testing.T, pool *pgxpool.Pool, id, username, password string, isActive, isAdmin bool) {
    hash, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
    pool.Exec(ctx, "INSERT INTO users (...) VALUES (...)", id, username, string(hash), isActive, isAdmin)
}
```

### Test cases

**Login:**
- ✅ Valid credentials → 200 + tokens returned, session row created in DB
- ✅ Wrong password → 401 "incorrect username or password"
- ✅ Non-existent username → 401 "incorrect username or password" (same error)
- ✅ Disabled user (`is_active=false`) → 401 "user account is disabled"
- ✅ Missing/empty username or password → 400
- ✅ Malformed JSON body → 400
- ✅ Verify `expires_in` matches config
- ✅ Verify `token_type` is "bearer"
- ✅ Verify returned tokens are valid JWTs (parse with `auth.ParseToken`)

**Refresh:**
- ✅ Valid refresh token + session exists → 200 + new access token, same refresh token
- ✅ Verify old access token hash is replaced in DB
- ✅ Expired refresh token (JWT expired) → 401
- ✅ Refresh token with no matching session (logged out) → 401
- ✅ Disabled user → 401
- ✅ Access token passed instead of refresh token (wrong type) → 401
- ✅ Missing refresh_token field → 400

**Logout:**
- ✅ Valid access + refresh token → 200, session row deleted
- ✅ Refresh token belonging to different user → 400
- ✅ Malformed refresh token → 200 (still succeeds for security)
- ✅ Session already deleted (double logout) → 200 (idempotent)
- ✅ No Authorization header → 401 (from middleware)

---

## Checklist

1. Add `golang.org/x/crypto` dependency (`go get golang.org/x/crypto`)
2. Add `github.com/google/uuid` as direct dependency (`go get github.com/google/uuid`)
3. Create `internal/api/auth.go` with `AuthHandler`, `HandleLogin`, `HandleRefresh`, `HandleLogout`
4. Update `router.go`: add `pool` parameter to `New`, register auth routes
5. Update `cmd/nexorious/main.go`: pass pool to `api.New`
6. Create `internal/api/auth_test.go` with test cases above
7. Verify: `go test ./internal/api/... -v`
8. Verify: `golangci-lint run`
