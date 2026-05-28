# Session-based auth + API keys — design

**Issue:** [#625 "Replace JWT auth with server-side sessions and API keys"](https://github.com/Nexorious/nexorious/issues/625)
**Date:** 2026-05-28
**Scope:** Backend + Frontend

## Problem

The current auth system is a JWT + session hybrid that has given up JWT's only real benefit (stateless verification) while retaining all its complexity:

- `JWTMiddleware` in `internal/auth/jwt.go` validates the JWT signature **and** does a DB lookup against `user_sessions` on every request. The DB round-trip means there's no benefit to the JWT layer.
- Two tokens (access + refresh), client-side rotation, a `SECRET_KEY` env var for signing, and non-trivial token management code in the frontend.
- Tokens are stored in `localStorage`, which is accessible to JavaScript and therefore vulnerable to XSS.

## Current state

**Backend:**
- `internal/auth/jwt.go` — `GenerateAccessToken`, `GenerateRefreshToken`, `ParseToken`, `HashToken`, `JWTMiddleware`, `AdminMiddleware`, `UserIDFromContext`, `IsAdminFromContext`
- `internal/api/auth.go` — `HandleLogin` (returns `tokenResponse{access_token, refresh_token}`), `HandleRefresh`, `HandleLogout` (takes `refresh_token` body), `HandleChangePassword` (identifies current session by `Authorization` header hash), plus `issueTokensAndSession` helper used by setup
- `internal/api/setup.go` — calls `issueTokensAndSession` after creating the admin user; returns `access_token`/`refresh_token`
- `internal/db/models/models.go` — `UserSession` has `token_hash`, `refresh_token_hash`, `expires_at`; no `last_used_at`
- `internal/config/config.go` — `SecretKey`, `AccessTokenExpireMinutes` (default 15), `RefreshTokenExpireDays` (default 30)
- `go.mod` — `github.com/golang-jwt/jwt/v5`

**Frontend:**
- `ui/frontend/src/providers/auth-provider.tsx` — reads/writes `localStorage` key `"auth"` with `{accessToken, refreshToken, user}`; manages refresh deduplication via `refreshPromiseRef`; registers `setAuthHandlers` with the API client
- `ui/frontend/src/api/client.ts` — `setAuthHandlers` wires in a token getter, token refresher, and logout handler; injects `Authorization: Bearer <token>` on every fetch; retries on 401 after refresh

## Design

### Two auth paths, one middleware

**Browser SPA** — HttpOnly `SameSite=Strict` session cookie containing an opaque random ID. No `localStorage`, no refresh logic, no `Authorization` header management.

**CLI / MCP / mobile** — `Authorization: Bearer nxr_<hex>` with a named, scoped, revocable API key.

`AuthMiddleware` tries the cookie first, then the Bearer header, then returns 401. Both paths hash the credential, look it up in the DB, load the user, check `is_active`, and set context — same `user_id`/`is_admin`/`user` values on the Echo context as today.

### Database changes

**Migrate `user_sessions`** — drop `token_hash` and `refresh_token_hash`; add `session_id_hash TEXT NOT NULL UNIQUE` and `last_used_at TIMESTAMPTZ`. Keep `user_agent`, `ip_address`, `id`, `user_id`, `created_at`. Replace `expires_at` semantics: it is set at login to `now() + session_expire_days` and is not updated on use (fixed max-age, matching the cookie `Max-Age`). All existing sessions are invalidated on deploy (users log in again — expected, one-time migration cost).

**New `api_keys` table:**
```sql
CREATE TABLE api_keys (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL UNIQUE,
    scopes      TEXT NOT NULL DEFAULT 'write',
    last_used_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ
);
```

The raw API key (`nxr_<32 random bytes hex-encoded>`) is shown exactly once at creation time. Only `key_hash` (SHA-256) is stored.

Migration filenames:
- `20260528000001_sessions_replace_jwt.up.sql` / `.down.sql`
- `20260528000002_api_keys.up.sql` / `.down.sql`

### Config changes

Remove: `SecretKey`, `AccessTokenExpireMinutes`, `RefreshTokenExpireDays`

Add: `SessionExpireDays int` (env `SESSION_EXPIRE_DAYS`, default 30)

`SecretKey` is no longer used for signing; existing deployments must remove it from their env or rename it. The config struct should drop the `required` tag from `SecretKey` and then remove the field entirely once no code references it.

### Backend

**`internal/auth/session.go`** replaces `jwt.go`. Keep `HashToken`, `UserIDFromContext`, `IsAdminFromContext`, `AuthUser`, `AdminMiddleware` unchanged. Replace everything JWT-specific with:

- `GenerateSessionID() (string, error)` — `crypto/rand` 32 bytes, hex-encoded (64-char string)
- `GenerateAPIKey() (string, error)` — `crypto/rand` 32 bytes, `"nxr_" + hex` (68-char string)
- `SetSessionCookie(c *echo.Context, sessionID string, expireDays int)` — sets an HttpOnly, SameSite=Strict, Secure, Path=/, Max-Age cookie named `"session_id"`
- `ClearSessionCookie(c *echo.Context)` — sets same cookie with Max-Age=0
- `AuthMiddleware(db *bun.DB, expireDays int) echo.MiddlewareFunc` — reads `"session_id"` cookie → hashes → queries `user_sessions WHERE session_id_hash = ? AND expires_at > now()`; if not found, reads `Authorization: Bearer` header → hashes → queries `api_keys WHERE key_hash = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`; in both cases loads user and sets context. Also updates `last_used_at` asynchronously (fire-and-forget with `context.Background()`).

Delete `jwt.go` entirely. Remove `golang-jwt/jwt/v5` from `go.mod`.

**`internal/api/auth.go`** changes:

- `HandleLogin` — sets cookie via `auth.SetSessionCookie`, returns `meResponse` (user object) instead of `tokenResponse`. Session is created with a generated session ID stored as its hash.
- Remove `HandleRefresh` entirely.
- `HandleLogout` — no request body; reads current `"session_id"` cookie, deletes the row from `user_sessions`, calls `auth.ClearSessionCookie`. Always returns 200.
- `HandleChangePassword` — identifies current session by reading the `"session_id"` cookie and hashing it (instead of the Authorization header hash). Deletes other sessions `WHERE user_id = ? AND session_id_hash != ?`.
- Remove `issueTokensAndSession` helper; replace with `issueSession(ctx, db, userID, expireDays, userAgent, ip) error` that only creates the session row and sets context — callers set the cookie themselves.

**`internal/api/setup.go`** — after creating the admin user, call `issueSession`, then `auth.SetSessionCookie`. Return the user object (same `meResponse` shape), not tokens.

**New handlers in `internal/api/auth.go`:**

`GET /api/auth/sessions` — list sessions for current user: `[{id, user_agent, ip_address, created_at, last_used_at, is_current}]`. `is_current` is true when `session_id_hash` matches the current cookie.

`DELETE /api/auth/sessions/:id` — revoke one session (must belong to current user); returns 204. If it's the current session, also calls `ClearSessionCookie`.

`DELETE /api/auth/sessions` — revoke all sessions **except** the current one (stolen-credentials response); returns 204.

`GET /api/auth/api-keys` — list API keys for current user: `[{id, name, scopes, last_used_at, created_at, expires_at}]`. Never returns the raw key.

`POST /api/auth/api-keys` — create a key: `{name: string, scopes?: "read"|"write", expires_at?: string}`. Generates key, stores hash. Returns `{id, name, scopes, key, created_at, expires_at}` — `key` is the raw value, shown exactly once.

`DELETE /api/auth/api-keys/:id` — revoke (sets `revoked_at = now()`); returns 204.

**`internal/api/router.go`** changes:

- Replace every `auth.JWTMiddleware(cfg.SecretKey, db)` with `auth.AuthMiddleware(db, cfg.SessionExpireDays)`.
- Remove `e.POST("/api/auth/refresh", ah.HandleRefresh)`.
- Add session management routes under the auth group.
- Add API key routes under the auth group.
- Remove `cfg.SecretKey` from all call sites.

**`internal/db/models/models.go`** changes:

- `UserSession` — remove `TokenHash`, `RefreshTokenHash`; add `SessionIDHash string`, `LastUsedAt *time.Time`.
- New `APIKey` struct:
  ```go
  type APIKey struct {
      bun.BaseModel `bun:"table:api_keys"`
      ID          string     `bun:"id,pk"            json:"id"`
      UserID      string     `bun:"user_id,notnull"  json:"user_id"`
      Name        string     `bun:"name,notnull"     json:"name"`
      KeyHash     string     `bun:"key_hash,notnull" json:"-"`
      Scopes      string     `bun:"scopes,notnull"   json:"scopes"`
      LastUsedAt  *time.Time `bun:"last_used_at"     json:"last_used_at"`
      CreatedAt   time.Time  `bun:"created_at,notnull" json:"created_at"`
      ExpiresAt   *time.Time `bun:"expires_at"       json:"expires_at"`
      RevokedAt   *time.Time `bun:"revoked_at"       json:"revoked_at"`
  }
  ```

### Frontend

**`ui/frontend/src/providers/auth-provider.tsx`** — remove entirely: `StoredAuth`, `getStoredAuth`, `setStoredAuth`, `clearStoredAuth`, `STORAGE_KEY`, `accessToken`/`refreshToken` state, `tokensRef`, `refreshPromiseRef`, `getAccessTokenFn`, `refreshTokensFn`, `setAuthHandlers` call. `isAuthenticated` becomes `!!user`. Initialization simplifies to `GET /api/auth/me` — 200 means logged in (set user), 401 means not (clear user). Login calls the login endpoint and then `GET /api/auth/me` to populate the user. Logout calls the logout endpoint (the server clears the cookie), then clears local user state and navigates to `/login`.

**`ui/frontend/src/api/client.ts`** — remove `setAuthHandlers`, `getAccessToken`, `refreshTokens`, `handleLogout`, `handleTokenRefresh`, `TokenGetter`, `TokenRefresher`, `LogoutHandler`, the token injection block, and the 401-retry-with-refresh block. Add `credentials: 'include'` to all fetch calls. On 401: redirect to `/login` (no retry). Update `apiUploadFile` and `apiDownloadFile` the same way — remove `Authorization` header injection and refresh retry; add `credentials: 'include'`.

**`ui/frontend/src/api/auth.ts`** — remove `refreshToken` function and `TokenResponse` type. Update `login` to expect a `User` response instead of tokens. Keep `logout`, `getMe`.

### Security properties

- HttpOnly cookies are inaccessible to JavaScript — XSS cannot steal sessions.
- `SameSite=Strict` prevents CSRF for same-origin SPAs — no CSRF token needed.
- Revocation is immediate (delete the DB row) for both sessions and API keys.
- API key raw value shown exactly once at creation; only the hash is stored.
- Password change invalidates all other sessions immediately.

## Files to change

| File | Change |
|---|---|
| `internal/auth/jwt.go` | Delete entirely |
| `internal/auth/session.go` | Create — `GenerateSessionID`, `GenerateAPIKey`, `SetSessionCookie`, `ClearSessionCookie`, `AuthMiddleware`; keep `HashToken`, `UserIDFromContext`, `IsAdminFromContext`, `AuthUser`, `AdminMiddleware` |
| `internal/auth/jwt_test.go` | Replace with `session_test.go` covering the new functions |
| `internal/api/auth.go` | Rewrite login/logout/change-password; remove refresh; add session + API key management handlers |
| `internal/api/auth_test.go` | Update all tests; add tests for new endpoints |
| `internal/api/setup.go` | Replace `issueTokensAndSession` call with `issueSession` + `SetSessionCookie`; change response type |
| `internal/api/setup_test.go` | Update login assertions (no longer checks for tokens) |
| `internal/api/router.go` | Replace `JWTMiddleware` with `AuthMiddleware`; remove refresh route; add session/API key routes |
| `internal/db/models/models.go` | Update `UserSession`; add `APIKey` |
| `internal/db/migrations/20260528000001_sessions_replace_jwt.up.sql` | Drop `token_hash`, `refresh_token_hash`; add `session_id_hash`, `last_used_at` |
| `internal/db/migrations/20260528000001_sessions_replace_jwt.down.sql` | Reverse the migration |
| `internal/db/migrations/20260528000002_api_keys.up.sql` | Create `api_keys` table |
| `internal/db/migrations/20260528000002_api_keys.down.sql` | `DROP TABLE api_keys` |
| `internal/config/config.go` | Remove `SecretKey`, `AccessTokenExpireMinutes`, `RefreshTokenExpireDays`; add `SessionExpireDays` |
| `go.mod` / `go.sum` | Remove `github.com/golang-jwt/jwt/v5` |
| `ui/frontend/src/providers/auth-provider.tsx` | Remove all token/localStorage/refresh logic |
| `ui/frontend/src/api/client.ts` | Remove token injection and refresh retry; add `credentials: 'include'` everywhere |
| `ui/frontend/src/api/auth.ts` | Remove `refreshToken`, update `login` response type |

## Out of scope

- API key scopes beyond `read`/`write` — keep `scopes TEXT DEFAULT 'write'` as a future-proof column, but do not enforce scope checks on routes in this PR.
- Session sliding expiry — fixed `Max-Age` is sufficient.
- Slumber collection updates for new session/API key endpoints — follow-on issue per the original spec.
- A UI for managing sessions and API keys — the endpoints will exist; the settings page is a follow-on.
