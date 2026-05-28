# Session-Based Auth + API Keys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the JWT + localStorage auth hybrid with server-side session cookies for the SPA and Bearer API keys for programmatic access.

**Architecture:** Two auth paths share a single `AuthMiddleware`: a cookie-based session (`session_id` HttpOnly cookie) for the browser, and a Bearer API key (stored as a SHA-256 hash in a new `api_keys` table) for CLI/MCP/mobile. The JWT library and all token-rotation logic are deleted. The frontend drops all localStorage and refresh logic; it just calls `GET /api/auth/me` to check auth and relies on the browser to send cookies automatically.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM, crypto/rand (stdlib), React 19, TanStack Router, TypeScript

**Spec:** `docs/superpowers/specs/2026-05-28-session-auth-design.md`

---

## File Map

**Created:**
- `internal/auth/session.go` — replaces jwt.go
- `internal/auth/session_test.go` — unit tests for session.go

**Edited in-place (no new migration files):**
- `internal/db/migrations/20260503000001_initial.up.sql` — update user_sessions schema, add api_keys table
- `internal/db/migrations/20260503000001_initial.down.sql` — add api_keys drop

**Deleted:**
- `internal/auth/jwt.go`
- `internal/auth/jwt_test.go`

**Modified:**
- `internal/config/config.go`
- `internal/db/models/models.go`
- `internal/api/auth.go` — full rewrite
- `internal/api/auth_test.go` — full rewrite
- `internal/api/setup.go`
- `internal/api/setup_test.go`
- `internal/api/router.go`
- `internal/api/main_test.go` — add `api_keys` to truncate list
- `internal/api/tags_test.go` — update `putJSONAuth`, `deleteAuth` helpers
- `internal/api/platforms_test.go` — update `getAuth` helper
- `internal/api/games_test.go`, `user_games_test.go`, `jobs_test.go`, `job_items_test.go`, `sync_test.go`, `import_test.go`, `export_test.go`, `backup_test.go`, `admin_users_test.go`, `router_test.go`, `setup_test.go` — update JWT calls to session helper
- `ui/frontend/src/types/auth.ts`
- `ui/frontend/src/api/auth.ts`
- `ui/frontend/src/api/client.ts`
- `ui/frontend/src/providers/auth-provider.tsx`

---

### Task 1: Feature branch

**Files:** none

- [ ] **Step 1: Create branch**

```bash
git checkout -b feat/session-auth
```

- [ ] **Step 2: Verify clean state**

```bash
git status
```
Expected: nothing to commit

---

### Task 2: DB migrations

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`
- Modify: `internal/db/migrations/20260503000001_initial.down.sql`
- Modify: `internal/api/main_test.go`

Edit the existing initial migration in-place (no new migration files).

- [ ] **Step 1: Edit `user_sessions` table in the initial up migration**

In `internal/db/migrations/20260503000001_initial.up.sql`, replace the `user_sessions` CREATE TABLE block and its indexes:

```sql
-- User sessions table
CREATE TABLE user_sessions (
    id             TEXT        PRIMARY KEY,
    user_id        TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id_hash TEXT       NOT NULL UNIQUE,
    user_agent     TEXT,
    ip_address     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    last_used_at   TIMESTAMPTZ
);

CREATE INDEX user_sessions_user_id_idx      ON user_sessions (user_id);
CREATE INDEX user_sessions_session_id_hash_idx ON user_sessions (session_id_hash);
CREATE INDEX user_sessions_expires_at_idx   ON user_sessions (expires_at);
```

- [ ] **Step 2: Add `api_keys` table to the initial up migration**

Add after the `user_sessions` block (before the Games table):

```sql
-- API keys table
CREATE TABLE api_keys (
    id           TEXT        PRIMARY KEY,
    user_id      TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    scopes       TEXT        NOT NULL DEFAULT 'write',
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX api_keys_user_id_idx ON api_keys (user_id);
```

- [ ] **Step 3: Update the down migration**

In `internal/db/migrations/20260503000001_initial.down.sql`, add `DROP TABLE IF EXISTS api_keys;` before `DROP TABLE IF EXISTS user_sessions;`:

```sql
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS user_sessions;
```

- [ ] **Step 4: Add `api_keys` to `truncateAllTables` in `internal/api/main_test.go`**

Find the TRUNCATE statement (around line 102) and add `api_keys` to the list:

```go
_, err := testDB.ExecContext(context.Background(), `
    TRUNCATE TABLE
        users,
        user_sessions,
        api_keys,
        games,
        user_games,
        user_game_platforms,
        tags,
        user_game_tags,
        external_games,
        user_sync_configs,
        jobs,
        job_items,
        river_job,
        rate_limiter_tokens
    RESTART IDENTITY CASCADE
`)
```

- [ ] **Step 6: Verify migrations embed**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/20260528000001_sessions_replace_jwt.up.sql \
        internal/db/migrations/20260528000001_sessions_replace_jwt.down.sql \
        internal/db/migrations/20260528000002_api_keys.up.sql \
        internal/db/migrations/20260528000002_api_keys.down.sql \
        internal/api/main_test.go
git commit -m "feat: add session + api_keys migrations"
```

---

### Task 3: Config and model changes

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/db/models/models.go`

- [ ] **Step 1: Update config — remove JWT fields, add `SessionExpireDays`**

In `internal/config/config.go`, replace the Security section:

```go
// -------------------------------------------------------------------------
// Security
// -------------------------------------------------------------------------

// DBEncryptionKey is used for at-rest encryption of storefront credentials.
// Generate with: openssl rand -base64 32
DBEncryptionKey string `env:"DB_ENCRYPTION_KEY,required"`

// SessionExpireDays controls Max-Age of the session cookie and the
// expires_at stored in user_sessions. Default 30 days.
SessionExpireDays int `env:"SESSION_EXPIRE_DAYS" envDefault:"30"`
```

(Remove the `SecretKey`, `AccessTokenExpireMinutes`, and `RefreshTokenExpireDays` fields entirely.)

- [ ] **Step 2: Update `UserSession` model in `internal/db/models/models.go`**

Replace the existing `UserSession` struct:

```go
type UserSession struct {
	bun.BaseModel `bun:"table:user_sessions"`

	ID            string     `bun:"id,pk"                      json:"id"`
	UserID        string     `bun:"user_id,notnull"            json:"user_id"`
	SessionIDHash string     `bun:"session_id_hash,notnull"    json:"-"`
	UserAgent     *string    `bun:"user_agent"                 json:"user_agent"`
	IpAddress     *string    `bun:"ip_address"                 json:"ip_address"`
	CreatedAt     time.Time  `bun:"created_at,notnull"         json:"created_at"`
	ExpiresAt     time.Time  `bun:"expires_at,notnull"         json:"expires_at"`
	LastUsedAt    *time.Time `bun:"last_used_at"               json:"last_used_at"`
}
```

- [ ] **Step 3: Add `APIKey` model after `UserSession`**

```go
type APIKey struct {
	bun.BaseModel `bun:"table:api_keys"`

	ID          string     `bun:"id,pk"              json:"id"`
	UserID      string     `bun:"user_id,notnull"    json:"user_id"`
	Name        string     `bun:"name,notnull"       json:"name"`
	KeyHash     string     `bun:"key_hash,notnull"   json:"-"`
	Scopes      string     `bun:"scopes,notnull"     json:"scopes"`
	LastUsedAt  *time.Time `bun:"last_used_at"       json:"last_used_at"`
	CreatedAt   time.Time  `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt   *time.Time `bun:"expires_at"         json:"expires_at"`
	RevokedAt   *time.Time `bun:"revoked_at"         json:"revoked_at"`
}
```

- [ ] **Step 4: Verify build**

```bash
go build ./...
```
Expected: errors referencing `SecretKey`, `AccessTokenExpireMinutes`, `RefreshTokenExpireDays` — these are expected; they'll be fixed in subsequent tasks.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/db/models/models.go
git commit -m "feat: replace JWT config fields with SessionExpireDays; update session + add APIKey models"
```

---

### Task 4: New `internal/auth/session.go` with unit tests

**Files:**
- Create: `internal/auth/session_test.go`
- Create: `internal/auth/session.go`

`jwt.go` still exists at this point — it will be deleted in Task 12.

- [ ] **Step 1: Write failing tests**

Create `internal/auth/session_test.go`:

```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
)

func TestGenerateSessionID(t *testing.T) {
	id1, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	if len(id1) != 64 {
		t.Errorf("GenerateSessionID() length = %d, want 64", len(id1))
	}
	id2, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	if id1 == id2 {
		t.Error("GenerateSessionID() returned duplicate values")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(key, "nxr_") {
		t.Errorf("GenerateAPIKey() = %q, want prefix %q", key, "nxr_")
	}
	if len(key) != 68 {
		t.Errorf("GenerateAPIKey() length = %d, want 68 (4 prefix + 64 hex)", len(key))
	}
	key2, _ := auth.GenerateAPIKey()
	if key == key2 {
		t.Error("GenerateAPIKey() returned duplicate values")
	}
}

func newSessionTestContext(t *testing.T) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSetSessionCookie(t *testing.T) {
	c, rec := newSessionTestContext(t)
	auth.SetSessionCookie(c, "test-session-id-value", 30)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "session_id" {
		t.Errorf("Name = %q, want %q", cookie.Name, "session_id")
	}
	if cookie.Value != "test-session-id-value" {
		t.Errorf("Value = %q, want %q", cookie.Value, "test-session-id-value")
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.MaxAge != 30*86400 {
		t.Errorf("MaxAge = %d, want %d", cookie.MaxAge, 30*86400)
	}
}

func TestClearSessionCookie(t *testing.T) {
	c, rec := newSessionTestContext(t)
	auth.ClearSessionCookie(c)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session_id" {
		t.Errorf("Name = %q, want %q", cookies[0].Name, "session_id")
	}
	if cookies[0].MaxAge != 0 {
		t.Errorf("MaxAge = %d, want 0", cookies[0].MaxAge)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```bash
go test ./internal/auth/... -run TestGenerateSessionID -v
```
Expected: compile error — `auth.GenerateSessionID` undefined

- [ ] **Step 3: Write `internal/auth/session.go`**

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
)

const sessionCookieName = "session_id"

// GenerateSessionID returns a cryptographically random 64-char hex string.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("GenerateSessionID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateAPIKey returns a cryptographically random API key prefixed with "nxr_".
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("GenerateAPIKey: %w", err)
	}
	return "nxr_" + hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// SetSessionCookie writes an HttpOnly SameSite=Strict session cookie.
func SetSessionCookie(c *echo.Context, sessionID string, expireDays int) {
	cookie := new(http.Cookie)
	cookie.Name = sessionCookieName
	cookie.Value = sessionID
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Secure = true
	cookie.Path = "/"
	cookie.MaxAge = expireDays * 86400
	c.SetCookie(cookie)
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(c *echo.Context) {
	cookie := new(http.Cookie)
	cookie.Name = sessionCookieName
	cookie.Value = ""
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Secure = true
	cookie.Path = "/"
	cookie.MaxAge = 0
	c.SetCookie(cookie)
}

// AuthUser holds user data loaded by AuthMiddleware from the database.
type AuthUser struct {
	ID       string
	Username string
	IsActive bool
	IsAdmin  bool
}

// UserIDFromContext extracts user_id set by AuthMiddleware. Returns "" if unset.
func UserIDFromContext(c *echo.Context) string {
	v := c.Get("user_id")
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// IsAdminFromContext extracts is_admin set by AuthMiddleware. Returns false if unset.
func IsAdminFromContext(c *echo.Context) bool {
	v := c.Get("is_admin")
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// AuthMiddleware tries cookie-based session auth first, then Bearer API key.
func AuthMiddleware(db *bun.DB, expireDays int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			userID, sessionHash, cookieErr := trySessionCookie(c, db)
			if cookieErr != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "session expired or not found"})
			}
			if userID == "" {
				apiUserID, apiErr := tryAPIKey(c, db)
				if apiErr != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired api key"})
				}
				if apiUserID == "" {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing or invalid authorization"})
				}
				userID = apiUserID
			}

			var user AuthUser
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT id, username, is_active, is_admin FROM users WHERE id = ?",
				userID,
			).Scan(&user.ID, &user.Username, &user.IsActive, &user.IsAdmin)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
				}
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
			if !user.IsActive {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user account is disabled"})
			}

			c.Set("user_id", user.ID)
			c.Set("is_admin", user.IsAdmin)
			c.Set("user", &user)
			c.Set("session_hash", sessionHash)

			if sessionHash != "" {
				go func() {
					if _, err := db.ExecContext(context.Background(),
						"UPDATE user_sessions SET last_used_at = now() WHERE session_id_hash = ?",
						sessionHash,
					); err != nil {
						slog.Error("auth: update session last_used_at", "err", err)
					}
				}()
			}

			return next(c)
		}
	}
}

// trySessionCookie checks for a session cookie. Returns ("", "", nil) when no
// cookie is present (fall through to API key). Returns an error when a cookie
// is present but the session is not found or expired.
func trySessionCookie(c *echo.Context, db *bun.DB) (userID, sessionHash string, err error) {
	cookie, cookieErr := c.Cookie(sessionCookieName)
	if cookieErr != nil {
		return "", "", nil
	}
	hash := HashToken(cookie.Value)
	var uid string
	err = db.QueryRowContext(c.Request().Context(),
		"SELECT user_id FROM user_sessions WHERE session_id_hash = ? AND expires_at > now()",
		hash,
	).Scan(&uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.New("session expired or not found")
		}
		return "", "", err
	}
	return uid, hash, nil
}

// tryAPIKey checks for a Bearer API key. Returns ("", nil) when no Bearer
// header is present. Returns an error when a key is present but invalid.
func tryAPIKey(c *echo.Context, db *bun.DB) (string, error) {
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", nil
	}
	raw := strings.TrimPrefix(authHeader, "Bearer ")
	hash := HashToken(raw)
	var userID string
	err := db.QueryRowContext(c.Request().Context(),
		`SELECT user_id FROM api_keys
		 WHERE key_hash = ? AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("invalid or expired api key")
		}
		return "", err
	}
	go func() {
		if _, err := db.ExecContext(context.Background(),
			"UPDATE api_keys SET last_used_at = now() WHERE key_hash = ?",
			hash,
		); err != nil {
			slog.Error("auth: update api key last_used_at", "err", err)
		}
	}()
	return userID, nil
}

// AdminMiddleware requires is_admin=true on the context. Must follow AuthMiddleware.
func AdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsAdminFromContext(c) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "admin access required"})
			}
			return next(c)
		}
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/auth/... -run "TestGenerateSessionID|TestGenerateAPIKey|TestSetSessionCookie|TestClearSessionCookie" -v
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/session.go internal/auth/session_test.go
git commit -m "feat: add session auth layer (GenerateSessionID, GenerateAPIKey, AuthMiddleware)"
```

---

### Task 5: Rewrite `auth.go` core handlers + update test helpers

**Files:**
- Modify: `internal/api/auth.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/tags_test.go`
- Modify: `internal/api/platforms_test.go`

This task replaces HandleLogin/HandleLogout/HandleRefresh/HandleChangePassword and updates the test infrastructure to use session cookies instead of JWT tokens.

- [ ] **Step 1: Update test helpers — `testCfg`, `insertAuthTestSession`, `postJSONAuth` in `auth_test.go`**

These helpers are shared across all `api_test` files. Replace in `internal/api/auth_test.go`:

```go
// testCfg returns a minimal config suitable for api_test tests.
func testCfg() *config.Config {
	return &config.Config{
		DBEncryptionKey:  "test-db-encryption-key-32-bytes!!",
		SessionExpireDays: 30,
		Port:             8000,
	}
}

// insertAuthTestSession inserts a session for userID and returns the raw session ID.
func insertAuthTestSession(t *testing.T, db *bun.DB, userID string) string {
	t.Helper()
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES (gen_random_uuid()::text, ?, ?, now() + interval '30 days')`,
		userID, auth.HashToken(sessionID),
	)
	if err != nil {
		t.Fatalf("insertAuthTestSession: %v", err)
	}
	return sessionID
}

// postJSONSession fires a POST request with a JSON body and a session cookie.
func postJSONSession(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

Also remove the old `postJSONAuth`, `jwtSign`, and `insertAuthTestSession` (old signature) from `auth_test.go`. Remove the `jwt` import.

- [ ] **Step 2: Update `putJSONAuth` in `internal/api/tags_test.go`**

```go
func putJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func deleteAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 3: Update `getAuth` in `internal/api/platforms_test.go`**

```go
func getAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 4: Write failing tests for new login behavior in `auth_test.go`**

Add these new tests (replacing the old login/logout/refresh/change-password tests):

```go
// ─── Login tests ─────────────────────────────────────────────────────────────

func TestHandleLogin_ValidCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-001", "alice", "password123", true, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "password123",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	// Response must be a user object, not tokens.
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["access_token"]; ok {
		t.Error("response must not contain access_token")
	}
	if resp["id"] == "" || resp["id"] == nil {
		t.Error("response must contain id")
	}
	if resp["username"] != "alice" {
		t.Errorf("username = %q, want %q", resp["username"], "alice")
	}

	// Must set a session cookie.
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session_id cookie set")
	}
	if sessionCookie.Value == "" {
		t.Error("session_id cookie value is empty")
	}

	// Session must exist in DB.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND session_id_hash = ?",
		"user-001", auth.HashToken(sessionCookie.Value),
	).Scan(&count); err != nil {
		t.Fatalf("session query: %v", err)
	}
	if count != 1 {
		t.Errorf("session count = %d, want 1", count)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-002", "bob", "correctpassword", true, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "bob",
		"password": "wrongpassword",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleLogin_DisabledUser(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-003", "carol", "password123", false, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "carol",
		"password": "password123",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleLogin_MissingFields(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{"username": "", "password": "pw"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing username: status = %d, want 400", rec.Code)
	}
	rec = postJSON(t, e, "/api/auth/login", map[string]string{"username": "alice", "password": ""})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing password: status = %d, want 400", rec.Code)
	}
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func TestHandleLogout_Valid(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-020", "ivan", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-020")

	rec := postJSONSession(t, e, "/api/auth/logout", nil, sessionID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	// Session must be deleted.
	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", "user-020",
	).Scan(&count)
	if count != 0 {
		t.Errorf("session count = %d, want 0 after logout", count)
	}

	// Cookie must be cleared (MaxAge=0).
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" && c.MaxAge == 0 {
			return
		}
	}
	t.Error("expected session_id cookie with MaxAge=0")
}

func TestHandleLogout_NoSession(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no session cookie)", rec.Code)
	}
}

// ─── ChangePassword tests ──────────────────────────────────────────────────

func TestHandleChangePassword_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-001", "pwduser", "oldpass123", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-cp-001")

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	}, sessionID)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var hash string
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT password_hash FROM users WHERE id = ?", "user-cp-001",
	).Scan(&hash)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpass456")); err != nil {
		t.Error("new password does not match stored hash")
	}
}

func TestHandleChangePassword_InvalidatesOtherSessions(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-005", "pwduser5", "oldpass123", true, false)
	currentSession := insertAuthTestSession(t, testDB, "user-cp-005")
	_ = insertAuthTestSession(t, testDB, "user-cp-005") // other session

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	}, currentSession)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", "user-cp-005",
	).Scan(&count)
	if count != 1 {
		t.Errorf("session count = %d, want 1 (current session preserved)", count)
	}
}
```

- [ ] **Step 5: Run tests — expect compile errors**

```bash
go test ./internal/api/... -run "TestHandleLogin|TestHandleLogout|TestHandleChangePassword" -v 2>&1 | head -20
```
Expected: compile errors (old JWT fields referenced)

- [ ] **Step 6: Rewrite `internal/api/auth.go`**

Replace the file with this content (keep `HandleGetMe`, `HandleUpdateMe`, `HandleCheckUsername`, `HandleChangeUsername` unchanged):

```go
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
)

const bcryptCost = 12

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db  *bun.DB
	cfg *config.Config
}

// NewAuthHandler returns a new AuthHandler.
func NewAuthHandler(db *bun.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type messageResponse struct {
	Message string `json:"message"`
}

// issueSession creates a session row and returns the raw session ID for use as
// a cookie value. Always uses context.Background() so client disconnect cannot
// abort the DB write.
func issueSession(ctx context.Context, db *bun.DB, expireDays int, userID, userAgent, ip string) (string, error) {
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		return "", fmt.Errorf("issueSession: %w", err)
	}
	var uaVal, ipVal any
	if userAgent != "" {
		uaVal = userAgent
	}
	if ip != "" {
		ipVal = ip
	}
	expiresAt := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
	_, err = db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, session_id_hash, user_agent, ip_address, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(),
		userID,
		auth.HashToken(sessionID),
		uaVal,
		ipVal,
		expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("issueSession: insert: %w", err)
	}
	return sessionID, nil
}

// meResponse is returned by login, GET /api/auth/me, and setup.
type meResponse struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	IsAdmin     bool            `json:"is_admin"`
	IsActive    bool            `json:"is_active"`
	Preferences json.RawMessage `json:"preferences"`
	CreatedAt   time.Time       `json:"created_at"`
}

func loadMeResponse(ctx context.Context, db *bun.DB, userID string) (*meResponse, error) {
	var resp meResponse
	var prefs []byte
	err := db.QueryRowContext(ctx,
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}
	return &resp, nil
}

// HandleLogin handles POST /api/auth/login.
func (h *AuthHandler) HandleLogin(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var userID, passwordHash string
	var isActive bool
	err := h.db.QueryRowContext(context.Background(),
		"SELECT id, password_hash, is_active FROM users WHERE username = ?",
		req.Username,
	).Scan(&userID, &passwordHash, &isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
		}
		slog.Error("login: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
	}
	if !isActive {
		return echo.NewHTTPError(http.StatusUnauthorized, "user account is disabled")
	}

	sessionID, err := issueSession(context.Background(), h.db, h.cfg.SessionExpireDays,
		userID, c.Request().Header.Get("User-Agent"), c.RealIP())
	if err != nil {
		slog.Error("login: issue session", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	auth.SetSessionCookie(c, sessionID, h.cfg.SessionExpireDays)

	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("login: load user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleLogout handles POST /api/auth/logout.
func (h *AuthHandler) HandleLogout(c *echo.Context) error {
	cookie, err := c.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		hash := auth.HashToken(cookie.Value)
		if _, err := h.db.ExecContext(context.Background(),
			"DELETE FROM user_sessions WHERE session_id_hash = ?", hash,
		); err != nil {
			slog.Error("logout: delete session", "err", err)
		}
	}
	auth.ClearSessionCookie(c)
	return c.JSON(http.StatusOK, messageResponse{Message: "Successfully logged out"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleChangePassword handles PUT /api/auth/change-password.
func (h *AuthHandler) HandleChangePassword(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.CurrentPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is required")
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be between 8 and 128 characters")
	}
	if req.CurrentPassword == req.NewPassword {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be different from current password")
	}

	var storedHash string
	err := h.db.QueryRowContext(context.Background(),
		"SELECT password_hash FROM users WHERE id = ?", userID,
	).Scan(&storedHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		slog.Error("change password: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.CurrentPassword)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is incorrect")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		slog.Error("change password: bcrypt", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?",
		string(newHash), userID,
	); err != nil {
		slog.Error("change password: update hash", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Invalidate other sessions; keep the current one.
	currentHash, _ := c.Get("session_hash").(string)
	if _, err := h.db.ExecContext(context.Background(),
		"DELETE FROM user_sessions WHERE user_id = ? AND session_id_hash != ?",
		userID, currentHash,
	); err != nil {
		slog.Error("change password: invalidate sessions", "err", err)
	}

	return c.JSON(http.StatusOK, messageResponse{Message: "Password changed successfully."})
}

// HandleGetMe handles GET /api/auth/me.
func (h *AuthHandler) HandleGetMe(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	resp, err := loadMeResponse(c.Request().Context(), h.db, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
		}
		slog.Error("get me: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

type updateMeRequest struct {
	Preferences json.RawMessage `json:"preferences"`
}

// HandleUpdateMe handles PUT /api/auth/me.
func (h *AuthHandler) HandleUpdateMe(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req updateMeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Preferences == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	var obj map[string]any
	if err := json.Unmarshal(req.Preferences, &obj); err != nil || obj == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET preferences = ?, updated_at = NOW() WHERE id = ?",
		string(req.Preferences), userID,
	); err != nil {
		slog.Error("update me: update preferences", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("update me: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

type usernameAvailabilityResponse struct {
	Available bool   `json:"available"`
	Username  string `json:"username"`
}

// HandleCheckUsername handles GET /api/auth/username/check/:username.
func (h *AuthHandler) HandleCheckUsername(c *echo.Context) error {
	username := c.Param("username")
	if len(username) < 3 || len(username) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be between 3 and 100 characters")
	}
	var exists int
	err := h.db.QueryRowContext(context.Background(),
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", username,
	).Scan(&exists)
	available := errors.Is(err, sql.ErrNoRows)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check username: query", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, usernameAvailabilityResponse{Available: available, Username: username})
}

type changeUsernameRequest struct {
	NewUsername string `json:"new_username"`
}

// HandleChangeUsername handles PUT /api/auth/username.
func (h *AuthHandler) HandleChangeUsername(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req changeUsernameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.NewUsername) < 3 || len(req.NewUsername) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be between 3 and 100 characters")
	}
	var currentUsername string
	err := h.db.QueryRowContext(context.Background(),
		"SELECT username FROM users WHERE id = ?", userID,
	).Scan(&currentUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		slog.Error("change username: query current", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if req.NewUsername == currentUsername {
		return echo.NewHTTPError(http.StatusBadRequest, "new username must be different from current username")
	}
	var usernameExists int
	err = h.db.QueryRowContext(context.Background(),
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", req.NewUsername,
	).Scan(&usernameExists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("change username: check availability", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if err == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "username already taken")
	}
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET username = ?, updated_at = NOW() WHERE id = ?",
		req.NewUsername, userID,
	); err != nil {
		slog.Error("change username: update", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("change username: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 7: Run the targeted tests**

```bash
go test ./internal/api/... -run "TestHandleLogin|TestHandleLogout|TestHandleChangePassword" -v 2>&1 | tail -20
```
Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go \
        internal/api/tags_test.go internal/api/platforms_test.go
git commit -m "feat: rewrite auth handlers to use session cookies; update test helpers"
```

---

### Task 6: Session management endpoints

**Files:**
- Modify: `internal/api/auth.go` (append handlers)
- Modify: `internal/api/auth_test.go` (append tests)

- [ ] **Step 1: Write failing tests**

Append to `internal/api/auth_test.go`:

```go
// ─── Session management tests ─────────────────────────────────────────────

func TestHandleListSessions_ReturnsSessions(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-sess-001", "sesuser", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-sess-001")
	_ = insertAuthTestSession(t, testDB, "user-sess-001")

	rec := getAuth(t, e, "/api/auth/sessions", sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var sessions []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("len(sessions) = %d, want 2", len(sessions))
	}
	// Exactly one session should be marked current.
	currentCount := 0
	for _, s := range sessions {
		if s["is_current"] == true {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Errorf("is_current count = %d, want 1", currentCount)
	}
}

func TestHandleRevokeSession_DeletesSession(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-sess-002", "sesuser2", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-sess-002")
	otherSession := insertAuthTestSession(t, testDB, "user-sess-002")

	// Get the ID of the other session.
	var otherID string
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT id FROM user_sessions WHERE session_id_hash = ?",
		auth.HashToken(otherSession),
	).Scan(&otherID)

	rec := deleteAuth(t, e, "/api/auth/sessions/"+otherID, sessionID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body)
	}

	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", "user-sess-002",
	).Scan(&count)
	if count != 1 {
		t.Errorf("session count = %d, want 1 after revoke", count)
	}
}

func TestHandleRevokeSession_OtherUserForbidden(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-sess-003", "sesuser3", "pw", true, false)
	insertAuthTestUser(t, testDB, "user-sess-004", "sesuser4", "pw", true, false)
	mySession := insertAuthTestSession(t, testDB, "user-sess-003")
	theirSession := insertAuthTestSession(t, testDB, "user-sess-004")

	var theirID string
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT id FROM user_sessions WHERE session_id_hash = ?",
		auth.HashToken(theirSession),
	).Scan(&theirID)

	rec := deleteAuth(t, e, "/api/auth/sessions/"+theirID, mySession)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (other user's session)", rec.Code)
	}
}

func TestHandleRevokeAllOtherSessions(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-sess-010", "sesuser10", "pw", true, false)
	currentSession := insertAuthTestSession(t, testDB, "user-sess-010")
	_ = insertAuthTestSession(t, testDB, "user-sess-010")
	_ = insertAuthTestSession(t, testDB, "user-sess-010")

	rec := deleteAuth(t, e, "/api/auth/sessions", currentSession)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body)
	}

	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", "user-sess-010",
	).Scan(&count)
	if count != 1 {
		t.Errorf("session count = %d, want 1 (current session preserved)", count)
	}
}
```

- [ ] **Step 2: Run tests — expect fail (handlers don't exist)**

```bash
go test ./internal/api/... -run "TestHandleListSessions|TestHandleRevokeSession|TestHandleRevokeAllOther" -v 2>&1 | head -10
```
Expected: FAIL (404 or compile error)

- [ ] **Step 3: Add session management handlers to `internal/api/auth.go`**

Append after `HandleChangeUsername`:

```go
// ─── Session management ───────────────────────────────────────────────────────

type sessionItem struct {
	ID         string     `json:"id"`
	UserAgent  *string    `json:"user_agent"`
	IPAddress  *string    `json:"ip_address"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	IsCurrent  bool       `json:"is_current"`
}

// HandleListSessions handles GET /api/auth/sessions.
func (h *AuthHandler) HandleListSessions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	currentHash, _ := c.Get("session_hash").(string)

	rows, err := h.db.QueryContext(context.Background(),
		`SELECT id, user_agent, ip_address, created_at, last_used_at, session_id_hash
		 FROM user_sessions WHERE user_id = ? AND expires_at > now() ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("list sessions: query", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	defer func() { //nolint:errcheck
		_ = rows.Close()
	}()

	var items []sessionItem
	for rows.Next() {
		var item sessionItem
		var hash string
		if err := rows.Scan(&item.ID, &item.UserAgent, &item.IPAddress,
			&item.CreatedAt, &item.LastUsedAt, &hash); err != nil {
			slog.Error("list sessions: scan", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
		}
		item.IsCurrent = hash == currentHash
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.Error("list sessions: rows.Err", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if items == nil {
		items = []sessionItem{}
	}
	return c.JSON(http.StatusOK, items)
}

// HandleRevokeSession handles DELETE /api/auth/sessions/:id.
func (h *AuthHandler) HandleRevokeSession(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	sessionRowID := c.Param("id")

	var sessionHash string
	err := h.db.QueryRowContext(context.Background(),
		"SELECT session_id_hash FROM user_sessions WHERE id = ? AND user_id = ?",
		sessionRowID, userID,
	).Scan(&sessionHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		slog.Error("revoke session: query", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if _, err := h.db.ExecContext(context.Background(),
		"DELETE FROM user_sessions WHERE id = ? AND user_id = ?",
		sessionRowID, userID,
	); err != nil {
		slog.Error("revoke session: delete", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	currentHash, _ := c.Get("session_hash").(string)
	if sessionHash == currentHash {
		auth.ClearSessionCookie(c)
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleRevokeAllOtherSessions handles DELETE /api/auth/sessions.
func (h *AuthHandler) HandleRevokeAllOtherSessions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	currentHash, _ := c.Get("session_hash").(string)

	if _, err := h.db.ExecContext(context.Background(),
		"DELETE FROM user_sessions WHERE user_id = ? AND session_id_hash != ?",
		userID, currentHash,
	); err != nil {
		slog.Error("revoke other sessions: delete", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/api/... -run "TestHandleListSessions|TestHandleRevokeSession|TestHandleRevokeAllOther" -v 2>&1 | tail -15
```
Expected: all PASS (routes not wired yet, but handler-level tests via `newTestEcho` work if the router is updated — if not, wire in Task 9 and re-run)

- [ ] **Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat: add session management endpoints (list, revoke, revoke-all-others)"
```

---

### Task 7: API key endpoints

**Files:**
- Modify: `internal/api/auth.go` (append handlers)
- Modify: `internal/api/auth_test.go` (append tests)

- [ ] **Step 1: Write failing tests**

Append to `internal/api/auth_test.go`:

```go
// ─── API key tests ────────────────────────────────────────────────────────────

func TestHandleCreateAPIKey_ReturnsRawKey(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-key-001", "keyuser", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-key-001")

	rec := postJSONSession(t, e, "/api/auth/api-keys", map[string]any{
		"name": "my-cli-key",
	}, sessionID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rawKey, _ := resp["key"].(string)
	if !strings.HasPrefix(rawKey, "nxr_") {
		t.Errorf("key = %q, want prefix nxr_", rawKey)
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("response must contain id")
	}

	// DB stores hash, not raw key.
	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM api_keys WHERE user_id = ? AND key_hash = ?",
		"user-key-001", auth.HashToken(rawKey),
	).Scan(&count)
	if count != 1 {
		t.Errorf("api_keys count = %d, want 1", count)
	}
}

func TestHandleListAPIKeys_HidesRawKey(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-key-002", "keyuser2", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-key-002")

	// Create a key first.
	postJSONSession(t, e, "/api/auth/api-keys", map[string]any{"name": "test-key"}, sessionID)

	rec := getAuth(t, e, "/api/auth/api-keys", sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var keys []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&keys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("len(keys) = %d, want 1", len(keys))
	}
	if _, ok := keys[0]["key"]; ok {
		t.Error("list must not include raw key")
	}
	if _, ok := keys[0]["key_hash"]; ok {
		t.Error("list must not include key_hash")
	}
}

func TestHandleRevokeAPIKey(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-key-003", "keyuser3", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-key-003")

	createRec := postJSONSession(t, e, "/api/auth/api-keys", map[string]any{"name": "revoke-me"}, sessionID)
	var createResp map[string]any
	_ = json.NewDecoder(createRec.Body).Decode(&createResp)
	keyID, _ := createResp["id"].(string)

	rec := deleteAuth(t, e, "/api/auth/api-keys/"+keyID, sessionID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body)
	}

	// Key should now appear revoked (revoked_at IS NOT NULL).
	var revokedAt *time.Time
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT revoked_at FROM api_keys WHERE id = ?", keyID,
	).Scan(&revokedAt)
	if revokedAt == nil {
		t.Error("revoked_at is nil, want a timestamp")
	}
}

func TestAPIKeyAuth_BearerToken(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-key-010", "keyuser10", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-key-010")

	// Create an API key via session auth.
	createRec := postJSONSession(t, e, "/api/auth/api-keys", map[string]any{"name": "bearer-test"}, sessionID)
	var createResp map[string]any
	_ = json.NewDecoder(createRec.Body).Decode(&createResp)
	rawKey, _ := createResp["key"].(string)

	// Use the raw API key as a Bearer token.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("API key auth: status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests — expect fail**

```bash
go test ./internal/api/... -run "TestHandleCreateAPIKey|TestHandleListAPIKeys|TestHandleRevokeAPIKey|TestAPIKeyAuth" -v 2>&1 | head -10
```
Expected: FAIL (404 — routes not wired)

- [ ] **Step 3: Add API key handlers to `internal/api/auth.go`**

Append after the session management handlers:

```go
// ─── API keys ────────────────────────────────────────────────────────────────

type apiKeyItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

type createAPIKeyRequest struct {
	Name      string  `json:"name"`
	Scopes    string  `json:"scopes"`
	ExpiresAt *string `json:"expires_at"`
}

type createAPIKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    string     `json:"scopes"`
	Key       string     `json:"key"` // raw value, shown exactly once
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// HandleListAPIKeys handles GET /api/auth/api-keys.
func (h *AuthHandler) HandleListAPIKeys(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	rows, err := h.db.QueryContext(context.Background(),
		`SELECT id, name, scopes, last_used_at, created_at, expires_at
		 FROM api_keys WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		slog.Error("list api keys: query", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var items []apiKeyItem
	for rows.Next() {
		var item apiKeyItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Scopes,
			&item.LastUsedAt, &item.CreatedAt, &item.ExpiresAt); err != nil {
			slog.Error("list api keys: scan", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		slog.Error("list api keys: rows.Err", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if items == nil {
		items = []apiKeyItem{}
	}
	return c.JSON(http.StatusOK, items)
}

// HandleCreateAPIKey handles POST /api/auth/api-keys.
func (h *AuthHandler) HandleCreateAPIKey(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req createAPIKeyRequest
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	scopes := req.Scopes
	if scopes == "" {
		scopes = "write"
	}
	if scopes != "read" && scopes != "write" {
		return echo.NewHTTPError(http.StatusBadRequest, "scopes must be 'read' or 'write'")
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expires_at must be RFC3339")
		}
		expiresAt = &parsed
	}

	rawKey, err := auth.GenerateAPIKey()
	if err != nil {
		slog.Error("create api key: generate", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	keyID := uuid.NewString()
	now := time.Now()
	if _, err := h.db.ExecContext(context.Background(),
		`INSERT INTO api_keys (id, user_id, name, key_hash, scopes, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		keyID, userID, req.Name, auth.HashToken(rawKey), scopes, now, expiresAt,
	); err != nil {
		slog.Error("create api key: insert", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	return c.JSON(http.StatusOK, createAPIKeyResponse{
		ID:        keyID,
		Name:      req.Name,
		Scopes:    scopes,
		Key:       rawKey,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	})
}

// HandleRevokeAPIKey handles DELETE /api/auth/api-keys/:id.
func (h *AuthHandler) HandleRevokeAPIKey(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	keyID := c.Param("id")

	result, err := h.db.ExecContext(context.Background(),
		"UPDATE api_keys SET revoked_at = now() WHERE id = ? AND user_id = ? AND revoked_at IS NULL",
		keyID, userID,
	)
	if err != nil {
		slog.Error("revoke api key: update", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		slog.Error("revoke api key: rows affected", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if affected == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "api key not found")
	}
	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests — expect pass (after router is wired in Task 9)**

Store this note — rerun after Task 9:
```bash
go test ./internal/api/... -run "TestHandleCreateAPIKey|TestHandleListAPIKeys|TestHandleRevokeAPIKey|TestAPIKeyAuth" -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat: add API key endpoints (list, create, revoke) and Bearer API key auth test"
```

---

### Task 8: Update `setup.go` and `setup_test.go`

**Files:**
- Modify: `internal/api/setup.go`
- Modify: `internal/api/setup_test.go`

- [ ] **Step 1: Update `setup.go`**

Replace the token-issuing section. Find the call to `issueTokensAndSession` (around line 95) and replace it with:

```go
sessionID, tokenErr := issueSession(context.Background(), sh.db, sh.cfg.SessionExpireDays,
    userID, c.Request().Header.Get("User-Agent"), c.RealIP())
if tokenErr != nil {
    slog.Error("setup admin: issue session", "err", tokenErr)
    return c.JSON(http.StatusOK, map[string]string{
        "message": "setup succeeded but session could not be created — please log in",
    })
}
auth.SetSessionCookie(c, sessionID, sh.cfg.SessionExpireDays)

resp, err := loadMeResponse(context.Background(), sh.db, userID)
if err != nil {
    slog.Error("setup admin: load user", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
}
return c.JSON(http.StatusOK, resp)
```

Also remove the `setupAdminResponse` type (or `AccessToken`/`RefreshToken` fields) that was returned from setup — the response is now `*meResponse` from `auth.go` (same package, no import needed).

- [ ] **Step 2: Update `setup_test.go` assertions**

Find assertions that check `access_token` and `refresh_token` (lines ~63–66) and replace with:

```go
if body.ID == "" {
    t.Error("expected user id in response")
}
if body.Username == "" {
    t.Error("expected username in response")
}

// Must set a session cookie.
var sessionCookie *http.Cookie
for _, c := range rec.Result().Cookies() {
    if c.Name == "session_id" {
        sessionCookie = c
    }
}
if sessionCookie == nil {
    t.Fatal("no session_id cookie set after setup")
}
```

Update the response struct used in `setup_test.go` from `AccessToken`/`RefreshToken` to user fields:

```go
var body struct {
    ID       string `json:"id"`
    Username string `json:"username"`
    IsAdmin  bool   `json:"is_admin"`
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/api/setup.go internal/api/setup_test.go
git commit -m "feat: update setup to issue session cookie instead of JWT tokens"
```

---

### Task 9: Update `router.go`

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Replace `JWTMiddleware` with `AuthMiddleware` everywhere**

In `internal/api/router.go`, every occurrence of `auth.JWTMiddleware(cfg.SecretKey, db)` becomes `auth.AuthMiddleware(db)`.

There are ~8 occurrences. Run this to find them all:
```bash
grep -n "JWTMiddleware" internal/api/router.go
```

Replace each one.

- [ ] **Step 2: Remove the refresh route**

Delete the line:
```go
e.POST("/api/auth/refresh", ah.HandleRefresh)
```

- [ ] **Step 3: Add session and API key routes**

In the `authGroup` block (after the existing auth routes), add:

```go
authGroup.GET("/sessions", ah.HandleListSessions)
authGroup.DELETE("/sessions", ah.HandleRevokeAllOtherSessions)
authGroup.DELETE("/sessions/:id", ah.HandleRevokeSession)
authGroup.GET("/api-keys", ah.HandleListAPIKeys)
authGroup.POST("/api-keys", ah.HandleCreateAPIKey)
authGroup.DELETE("/api-keys/:id", ah.HandleRevokeAPIKey)
```

- [ ] **Step 4: Add `AllowCredentials: true` to CORS config**

Replace the CORS block:
```go
if len(cfg.CORSOrigins) > 0 {
    e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
        AllowOrigins:     cfg.CORSOrigins,
        AllowCredentials: true,
        AllowHeaders:     []string{"Content-Type", "Authorization"},
        AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
    }))
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```
Expected: errors mentioning `HandleRefresh` still referenced (if any) — fix them. Otherwise no errors.

- [ ] **Step 6: Run all auth + session + API key tests**

```bash
go test ./internal/api/... -run "TestHandle" -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL)" | head -40
```
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api/router.go
git commit -m "feat: wire AuthMiddleware, session/api-key routes; add CORS AllowCredentials"
```

---

### Task 10: Update all non-auth integration test files

**Files:**
- Modify: `internal/api/games_test.go`, `user_games_test.go`, `jobs_test.go`, `job_items_test.go`, `sync_test.go`, `import_test.go`, `export_test.go`, `backup_test.go`, `admin_users_test.go`, `router_test.go`

Every one of these files follows the same pattern. For each file:

1. Remove `auth.GenerateAccessToken(...)` calls
2. Remove `auth.GenerateRefreshToken(...)` calls
3. Replace `insertAuthTestSession(t, testDB, userID, accessToken, refreshToken, 30)` with `sessionID := insertAuthTestSession(t, testDB, userID)`
4. Rename `accessToken` variables to `sessionID` where passed to `getAuth`/`putJSONAuth`/`deleteAuth`/`postJSONAuth`/`postJSONSession`
5. Remove `cfg.SecretKey` references from `testCfg()` callsites (none needed since `testCfg()` is already updated)
6. Remove the `auth` import if only JWT functions were using it (but `auth.HashToken` may still be used — keep the import if so)

- [ ] **Step 1: Update `internal/api/games_test.go`**

Find all patterns like:
```go
accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
// ...
insertAuthTestSession(t, testDB, userID, accessToken, "", 30)
```
Replace with:
```go
sessionID := insertAuthTestSession(t, testDB, userID)
```
Then replace `accessToken` with `sessionID` in `getAuth`/`putJSONAuth`/`deleteAuth` calls.

- [ ] **Step 2: Update remaining test files with the same pattern**

Repeat for: `user_games_test.go`, `jobs_test.go`, `job_items_test.go`, `sync_test.go`, `import_test.go`, `export_test.go`, `backup_test.go`, `admin_users_test.go`, `router_test.go`.

The exact pattern to replace in each file:
```go
// OLD:
cfg := testCfg()
accessToken, _ := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
insertAuthTestSession(t, testDB, userID, accessToken, "", 30)
// use: getAuth(t, e, path, accessToken)

// NEW:
sessionID := insertAuthTestSession(t, testDB, userID)
// use: getAuth(t, e, path, sessionID)
```

In files where `cfg` is used only for `testCfg()` setup and nothing else, keep the call but drop the JWT field references.

- [ ] **Step 3: Run the full test suite**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -20
```
Expected: all packages PASS

- [ ] **Step 4: Commit**

```bash
git add internal/api/
git commit -m "test: update all api integration tests to use session cookies"
```

---

### Task 11: Delete `jwt.go` and remove the JWT dependency

**Files:**
- Delete: `internal/auth/jwt.go`
- Delete: `internal/auth/jwt_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Delete JWT files**

```bash
rm internal/auth/jwt.go internal/auth/jwt_test.go
```

- [ ] **Step 2: Remove the JWT module**

```bash
go get github.com/golang-jwt/jwt/v5@none
go mod tidy
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```
Expected: no errors

- [ ] **Step 4: Run full Go test suite**

```bash
go test -timeout 600s ./...
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: delete jwt.go; remove golang-jwt/jwt/v5 dependency"
```

---

### Task 12: Frontend — types and API client

**Files:**
- Modify: `ui/frontend/src/types/auth.ts`
- Modify: `ui/frontend/src/api/auth.ts`

- [ ] **Step 1: Update `ui/frontend/src/types/auth.ts`**

Remove `LoginResponse`. The file becomes:

```typescript
export interface User {
  id: string;
  username: string;
  isAdmin: boolean;
  preferences?: Record<string, unknown>;
}
```

- [ ] **Step 2: Update `ui/frontend/src/api/auth.ts`**

```typescript
import { api } from './client';
import type { User } from '@/types';

interface UserApiResponse {
  id: string;
  username: string;
  is_admin: boolean;
  preferences?: Record<string, unknown>;
}

interface UsernameAvailabilityResponse {
  available: boolean;
  username: string;
}

function transformUser(apiUser: UserApiResponse): User {
  return {
    id: apiUser.id,
    username: apiUser.username,
    isAdmin: apiUser.is_admin,
    preferences: apiUser.preferences,
  };
}

export async function login(username: string, password: string): Promise<User> {
  const response = await api.post<UserApiResponse>('/auth/login', { username, password });
  return transformUser(response);
}

export async function logout(): Promise<void> {
  await api.post('/auth/logout');
}

export async function getMe(): Promise<User> {
  const response = await api.get<UserApiResponse>('/auth/me');
  return transformUser(response);
}

export async function changeUsername(newUsername: string): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/username', {
    new_username: newUsername,
  });
  return transformUser(response);
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await api.put('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
  });
}

export async function checkUsernameAvailability(
  username: string,
): Promise<UsernameAvailabilityResponse> {
  return api.get<UsernameAvailabilityResponse>(
    `/auth/username/check/${encodeURIComponent(username)}`,
  );
}

export async function updatePreferences(preferences: Record<string, unknown>): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/me', { preferences });
  return transformUser(response);
}
```

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```
Expected: no errors (or only errors from files not yet updated — fix those)

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/types/auth.ts ui/frontend/src/api/auth.ts
git commit -m "feat: remove LoginResponse/refreshToken; login now returns User"
```

---

### Task 13: Frontend — `client.ts`

**Files:**
- Modify: `ui/frontend/src/api/client.ts`

- [ ] **Step 1: Rewrite `ui/frontend/src/api/client.ts`**

```typescript
import { config } from '@/lib/env';

export class ApiErrorException extends Error {
  constructor(
    public override message: string,
    public status: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = 'ApiErrorException';
  }
}

export interface ApiCallOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>;
}

function buildUrl(
  path: string,
  params?: Record<string, string | number | boolean | undefined>,
): string {
  const baseUrl = `${config.apiUrl}${path.startsWith('/') ? path : `/${path}`}`;
  if (!params) return baseUrl;
  const searchParams = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined) {
      searchParams.append(key, String(value));
    }
  });
  const queryString = searchParams.toString();
  return queryString ? `${baseUrl}?${queryString}` : baseUrl;
}

async function handleApiError(response: Response): Promise<never> {
  let errorDetails: unknown;
  let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
  try {
    errorDetails = await response.json();
    if (typeof errorDetails === 'object' && errorDetails !== null) {
      const details = errorDetails as Record<string, unknown>;
      if (typeof details.detail === 'string') errorMessage = details.detail;
      else if (typeof details.error === 'string') errorMessage = details.error;
      else if (typeof details.message === 'string') errorMessage = details.message;
    }
  } catch {
    // use default message
  }
  throw new ApiErrorException(errorMessage, response.status, errorDetails);
}

export async function apiCall(path: string, options: ApiCallOptions = {}): Promise<Response> {
  const { params, ...fetchOptions } = options;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(fetchOptions.headers as Record<string, string>),
  };

  const url = buildUrl(path, params);
  const response = await fetch(url, {
    ...fetchOptions,
    headers,
    credentials: 'include',
  });

  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await handleApiError(response);
  }

  return response;
}

export const api = {
  get: <T = unknown>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'GET' }).then((r) => r.json()),

  post: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  put: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  patch: <T = unknown>(
    path: string,
    data?: unknown,
    options?: Omit<ApiCallOptions, 'method' | 'body'>,
  ): Promise<T> =>
    apiCall(path, {
      ...options,
      method: 'PATCH',
      body: data ? JSON.stringify(data) : undefined,
    }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),

  delete: <T = void>(path: string, options?: Omit<ApiCallOptions, 'method'>): Promise<T> =>
    apiCall(path, { ...options, method: 'DELETE' }).then((r) => {
      if (r.status === 204) return undefined as T;
      return r.json();
    }),
};

export async function apiUploadFile<T>(
  path: string,
  file: File,
  fieldName: string = 'file',
): Promise<T> {
  const formData = new FormData();
  formData.append(fieldName, file);
  const url = buildUrl(path);
  const response = await fetch(url, {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await handleApiError(response);
  }
  return response.json();
}

export async function apiDownloadFile(path: string): Promise<{ blob: Blob; filename: string }> {
  const url = buildUrl(path);
  const response = await fetch(url, {
    method: 'GET',
    credentials: 'include',
  });
  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== '/login') {
      window.location.replace('/login');
    }
    await handleApiError(response);
  }
  const contentDisposition = response.headers.get('Content-Disposition');
  let filename = 'download';
  if (contentDisposition) {
    const filenameMatch = contentDisposition.match(/filename="?([^";\n]+)"?/);
    if (filenameMatch) filename = filenameMatch[1];
  }
  const blob = await response.blob();
  return { blob, filename };
}
```

- [ ] **Step 2: Fix any callers that passed `skipAuth: true`**

Search for remaining `skipAuth` usage:
```bash
grep -r "skipAuth" ui/frontend/src/
```
Remove any `skipAuth` option from all call sites (the option no longer exists).

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/api/client.ts
git commit -m "feat: replace token-based client with cookie-based; add credentials:include"
```

---

### Task 14: Frontend — `auth-provider.tsx`

**Files:**
- Modify: `ui/frontend/src/providers/auth-provider.tsx`

- [ ] **Step 1: Rewrite `ui/frontend/src/providers/auth-provider.tsx`**

```typescript
import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react';
import { useNavigate } from '@tanstack/react-router';
import type { User } from '@/types';
import * as authApi from '@/api/auth';

interface AuthContextValue {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const isAuthenticated = !!user;

  // Initialize: check if a session exists by calling GET /api/auth/me.
  useEffect(() => {
    authApi
      .getMe()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setIsLoading(false));
  }, []);

  const login = useCallback(async (username: string, password: string): Promise<void> => {
    setIsLoading(true);
    setError(null);
    try {
      const loggedInUser = await authApi.login(username, password);
      setUser(loggedInUser);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
      setUser(null);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  const logout = useCallback(async (): Promise<void> => {
    try {
      await authApi.logout();
    } catch {
      // Ignore errors — the server clears the cookie; we clear local state regardless.
    }
    setUser(null);
    setError(null);
    await navigate({ to: '/login' });
  }, [navigate]);

  const clearError = useCallback(() => setError(null), []);

  const value: AuthContextValue = {
    user,
    isAuthenticated,
    isLoading,
    error,
    login,
    logout,
    clearError,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
```

- [ ] **Step 2: Fix any call sites that call `logout()` synchronously**

```bash
grep -r "logout()" ui/frontend/src/ --include="*.tsx" --include="*.ts"
```
`logout` is now async. Any call site that calls `logout()` without `await` should add `void logout()` or `await logout()` as appropriate.

- [ ] **Step 3: Fix any call sites that read `accessToken` from auth context**

```bash
grep -r "accessToken\|refreshToken\|setAuthHandlers" ui/frontend/src/
```
Remove/replace any references.

- [ ] **Step 4: Type-check**

```bash
cd ui/frontend && npm run check
```
Expected: no errors

- [ ] **Step 5: Run frontend tests**

```bash
cd ui/frontend && npm run test
```
Expected: all PASS (update any test mocks that referenced `LoginResponse` or token fields)

- [ ] **Step 6: Run dead-code check**

```bash
cd ui/frontend && npm run knip
```
Expected: no unused exports (remove any that knip flags)

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/providers/auth-provider.tsx
git commit -m "feat: simplify AuthProvider to cookie-based session; drop localStorage + refresh logic"
```

---

### Task 15: Final verification

- [ ] **Step 1: Full Go test suite**

```bash
go test -timeout 600s ./...
```
Expected: all PASS

- [ ] **Step 2: Full frontend check**

```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```
Expected: all PASS

- [ ] **Step 3: Build everything**

```bash
make
```
Expected: frontend builds, Go binary compiles, no errors

- [ ] **Step 4: Verify spec coverage**

Re-read `docs/superpowers/specs/2026-05-28-session-auth-design.md` and confirm all items are implemented.

- [ ] **Step 5: Commit spec and plan**

```bash
git add docs/superpowers/specs/2026-05-28-session-auth-design.md \
        docs/superpowers/plans/2026-05-28-session-auth.md
git commit -m "docs: add spec and plan for session-based auth + API keys"
```
