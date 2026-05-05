# JWT Auth Package Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement JWT token generation, parsing, hashing, DB-backed session middleware, admin middleware, and context helpers in `internal/auth/`.

**Architecture:** Single package with pure token functions (no DB) and Echo middleware that validates every request against `user_sessions` and `users` tables via `pgxpool.Pool.QueryRow`. Matches Python `security.py` + `get_current_user` behavior exactly.

**Tech Stack:** `golang-jwt/jwt/v5`, `pgx/v5/pgxpool`, `echo/v5`, `crypto/sha256`, `testcontainers-go`

---

### Task 1: Add `golang-jwt/jwt/v5` dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Install the dependency**

```bash
go get github.com/golang-jwt/jwt/v5
```

- [ ] **Step 2: Verify it's in go.mod**

```bash
grep 'golang-jwt' go.mod
```

Expected: `github.com/golang-jwt/jwt/v5 v5.x.x`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add golang-jwt/jwt/v5"
```

---

### Task 2: Token generation and parsing — tests first

**Files:**
- Create: `internal/auth/jwt.go`
- Create: `internal/auth/jwt_test.go`

- [ ] **Step 1: Create `jwt.go` with the `Claims` struct and function stubs**

```go
package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT payload for both access and refresh tokens.
// Differentiated by the Type field ("access" or "refresh").
type Claims struct {
	Type string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a short-lived JWT with type="access".
func GenerateAccessToken(secretKey string, userID string, expireMinutes int) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// GenerateRefreshToken creates a long-lived JWT with type="refresh".
func GenerateRefreshToken(secretKey string, userID string, expireDays int) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// ParseToken validates a JWT string, checks the type claim matches expectedType,
// and returns the claims. Returns an error if malformed, expired, wrong key, or wrong type.
func ParseToken(secretKey string, tokenString string, expectedType string) (*Claims, error) {
	return nil, fmt.Errorf("not implemented")
}

// HashToken returns the SHA-256 hex digest of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 2: Write tests for token generation, parsing, and hashing**

```go
package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/drzero42/nexorious-go/internal/auth"
)

func TestGenerateAndParseAccessToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	token, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ParseToken(secret, token, "access")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID)
	}
	if claims.Type != "access" {
		t.Errorf("Type = %q, want %q", claims.Type, "access")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	// Expiry should be ~15 minutes from now (allow 5s tolerance).
	expectedExpiry := time.Now().Add(15 * time.Minute)
	diff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt off by %v", diff)
	}
}

func TestGenerateAndParseRefreshToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	token, err := auth.GenerateRefreshToken(secret, userID, 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ParseToken(secret, token, "refresh")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID)
	}
	if claims.Type != "refresh" {
		t.Errorf("Type = %q, want %q", claims.Type, "refresh")
	}
	// Expiry should be ~30 days from now (allow 5s tolerance).
	expectedExpiry := time.Now().Add(30 * 24 * time.Hour)
	diff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt off by %v", diff)
	}
}

func TestParseToken_WrongType(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	accessToken, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Parsing an access token as refresh should fail.
	_, err = auth.ParseToken(secret, accessToken, "refresh")
	if err == nil {
		t.Fatal("expected error when parsing access token as refresh")
	}

	refreshToken, err := auth.GenerateRefreshToken(secret, userID, 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	// Parsing a refresh token as access should fail.
	_, err = auth.ParseToken(secret, refreshToken, "access")
	if err == nil {
		t.Fatal("expected error when parsing refresh token as access")
	}
}

func TestParseToken_Expired(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a token that expired 1 minute ago by signing manually.
	claims := auth.Claims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-16 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = auth.ParseToken(secret, signed, "access")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseToken_WrongKey(t *testing.T) {
	token, err := auth.GenerateAccessToken("key-A-at-least-32-bytes-long!!!!", "user1", 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	_, err = auth.ParseToken("key-B-at-least-32-bytes-long!!!!", token, "access")
	if err == nil {
		t.Fatal("expected error when verifying with wrong key")
	}
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := auth.ParseToken("secret-key-at-least-32-bytes!!!!!", "not.a.jwt.token", "access")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestHashToken(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiJ9.test"
	hash1 := auth.HashToken(token)
	hash2 := auth.HashToken(token)

	if hash1 != hash2 {
		t.Errorf("HashToken not deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got length %d", len(hash1))
	}

	// Different input → different hash.
	hash3 := auth.HashToken("different-token")
	if hash1 == hash3 {
		t.Error("different inputs produced the same hash")
	}
}
```

- [ ] **Step 3: Run tests — expect failures for generate/parse (HashToken should pass)**

```bash
go test ./internal/auth/... -v
```

Expected: `TestHashToken` PASS; all `Generate`/`Parse` tests FAIL with "not implemented".

- [ ] **Step 4: Implement `GenerateAccessToken`**

Replace the stub in `jwt.go`:

```go
func GenerateAccessToken(secretKey string, userID string, expireMinutes int) (string, error) {
	now := time.Now()
	claims := Claims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}
```

- [ ] **Step 5: Implement `GenerateRefreshToken`**

Replace the stub in `jwt.go`:

```go
func GenerateRefreshToken(secretKey string, userID string, expireDays int) (string, error) {
	now := time.Now()
	claims := Claims{
		Type: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireDays) * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}
```

- [ ] **Step 6: Implement `ParseToken`**

Replace the stub in `jwt.go`:

```go
func ParseToken(secretKey string, tokenString string, expectedType string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.Type != expectedType {
		return nil, fmt.Errorf("token type %q does not match expected %q", claims.Type, expectedType)
	}
	return claims, nil
}
```

- [ ] **Step 7: Run all tests — all should pass**

```bash
go test ./internal/auth/... -v
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/auth/jwt.go internal/auth/jwt_test.go
git commit -m "feat(auth): add JWT token generation, parsing, and hashing"
```

---

### Task 3: Context helpers and admin middleware

**Files:**
- Modify: `internal/auth/jwt.go`
- Modify: `internal/auth/jwt_test.go`

- [ ] **Step 1: Write tests for context helpers and admin middleware**

Append to `jwt_test.go`:

```go
import (
	// Add to existing imports:
	"net/http"
	"net/http/httptest"

	"github.com/labstack/echo/v5"
)

func TestContextHelpers(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Before setting — defaults.
	if got := auth.UserIDFromContext(c); got != "" {
		t.Errorf("UserIDFromContext before Set = %q, want empty", got)
	}
	if got := auth.IsAdminFromContext(c); got != false {
		t.Errorf("IsAdminFromContext before Set = %v, want false", got)
	}

	// After setting.
	c.Set("user_id", "user-123")
	c.Set("is_admin", true)

	if got := auth.UserIDFromContext(c); got != "user-123" {
		t.Errorf("UserIDFromContext = %q, want %q", got, "user-123")
	}
	if got := auth.IsAdminFromContext(c); got != true {
		t.Errorf("IsAdminFromContext = %v, want true", got)
	}
}

func TestAdminMiddleware_Admin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("is_admin", true)

	handlerCalled := false
	handler := func(c *echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	}

	mw := auth.AdminMiddleware()(handler)
	err := mw(c)
	if err != nil {
		t.Fatalf("AdminMiddleware returned error: %v", err)
	}
	if !handlerCalled {
		t.Error("handler was not called for admin user")
	}
}

func TestAdminMiddleware_NonAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("is_admin", false)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called for non-admin")
		return nil
	}

	mw := auth.AdminMiddleware()(handler)
	_ = mw(c)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAdminMiddleware_NoAdminKey(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Don't set is_admin at all.
	handler := func(c *echo.Context) error {
		t.Error("handler should not be called when is_admin is absent")
		return nil
	}

	mw := auth.AdminMiddleware()(handler)
	_ = mw(c)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
```

- [ ] **Step 2: Run tests — expect failures for helpers/middleware (not implemented)**

```bash
go test ./internal/auth/... -v -run "TestContextHelpers|TestAdminMiddleware"
```

Expected: compile error — functions don't exist yet.

- [ ] **Step 3: Implement context helpers and admin middleware**

Append to `jwt.go`:

```go
import (
	// Add to existing imports:
	"net/http"

	"github.com/labstack/echo/v5"
)

// UserIDFromContext extracts user_id set by JWTMiddleware. Returns "" if unset.
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

// IsAdminFromContext extracts is_admin set by JWTMiddleware. Returns false if unset.
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

// AdminMiddleware returns middleware that requires is_admin=true on the context.
// Must be applied after JWTMiddleware.
func AdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsAdminFromContext(c) {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "admin access required",
				})
			}
			return next(c)
		}
	}
}
```

- [ ] **Step 4: Run tests — all should pass**

```bash
go test ./internal/auth/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/jwt.go internal/auth/jwt_test.go
git commit -m "feat(auth): add context helpers and admin middleware"
```

---

### Task 4: JWT middleware with DB-backed session validation

**Files:**
- Modify: `internal/auth/jwt.go`
- Modify: `internal/auth/jwt_test.go`

- [ ] **Step 1: Add `AuthUser` struct and `JWTMiddleware` signature to `jwt.go`**

Add above the existing functions:

```go
import (
	// Add to existing imports:
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthUser holds the user data loaded by JWTMiddleware from the database.
type AuthUser struct {
	ID       string
	Username string
	IsActive bool
	IsAdmin  bool
}

// JWTMiddleware returns middleware that validates the access token JWT,
// checks the session exists in user_sessions, loads the user from users,
// and sets user_id, is_admin, and user on the Echo context.
func JWTMiddleware(secretKey string, pool *pgxpool.Pool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "not implemented",
			})
		}
	}
}
```

- [ ] **Step 2: Write tests for JWT middleware — no-DB cases**

Append to `jwt_test.go`:

```go
func TestJWTMiddleware_MissingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called without auth header")
		return nil
	}

	// pool is nil — we should never reach the DB lookup.
	mw := auth.JWTMiddleware("secret", nil)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer some-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called with malformed header")
		return nil
	}

	mw := auth.JWTMiddleware("secret", nil)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	claims := auth.Claims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-16 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called with expired token")
		return nil
	}

	mw := auth.JWTMiddleware(secret, nil)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_RefreshTokenRejected(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"

	// Generate a refresh token and try to use it as an access token.
	refreshToken, err := auth.GenerateRefreshToken(secret, "user-1", 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called with refresh token")
		return nil
	}

	mw := auth.JWTMiddleware(secret, nil)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
```

- [ ] **Step 3: Run no-DB tests — expect failures (stub returns 401 for all, so some may pass accidentally; verify the right error messages once implemented)**

```bash
go test ./internal/auth/... -v -run "TestJWTMiddleware_Missing|TestJWTMiddleware_Malformed|TestJWTMiddleware_Expired|TestJWTMiddleware_Refresh"
```

- [ ] **Step 4: Implement the header-parsing and JWT-validation part of `JWTMiddleware`**

Replace the `JWTMiddleware` stub:

```go
func JWTMiddleware(secretKey string, pool *pgxpool.Pool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// Step 1-2: Read and validate Authorization header.
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "missing or invalid authorization header",
				})
			}
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			// Step 3: Parse and validate JWT.
			claims, err := ParseToken(secretKey, tokenString, "access")
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "invalid or expired token",
				})
			}

			// Step 4: Extract user ID.
			userID := claims.Subject
			if userID == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "invalid token: missing subject",
				})
			}

			// Step 5: Check session in DB.
			tokenHash := HashToken(tokenString)
			var sessionID string
			err = pool.QueryRow(context.Background(),
				"SELECT id FROM user_sessions WHERE user_id = $1 AND token_hash = $2",
				userID, tokenHash,
			).Scan(&sessionID)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "session not found or expired",
				})
			}

			// Step 6: Load user from DB.
			var user AuthUser
			err = pool.QueryRow(context.Background(),
				"SELECT id, username, is_active, is_admin FROM users WHERE id = $1",
				userID,
			).Scan(&user.ID, &user.Username, &user.IsActive, &user.IsAdmin)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "user not found",
				})
			}
			if !user.IsActive {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "user account is disabled",
				})
			}

			// Step 7: Set on context.
			c.Set("user_id", user.ID)
			c.Set("is_admin", user.IsAdmin)
			c.Set("user", &user)

			return next(c)
		}
	}
}
```

- [ ] **Step 5: Run no-DB tests — all should pass**

```bash
go test ./internal/auth/... -v -run "TestJWTMiddleware_Missing|TestJWTMiddleware_Malformed|TestJWTMiddleware_Expired|TestJWTMiddleware_Refresh"
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/jwt.go internal/auth/jwt_test.go
git commit -m "feat(auth): add JWTMiddleware with header parsing and JWT validation"
```

---

### Task 5: JWT middleware DB tests (testcontainers)

**Files:**
- Modify: `internal/auth/jwt_test.go`

- [ ] **Step 1: Write DB-backed middleware tests**

Append to `jwt_test.go`:

```go
import (
	// Add to existing imports:
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	gmigrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
)

// setupTestPool spins up a Postgres container, runs migrations, and returns a pgxpool.Pool.
func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("nexorious_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Run migrations.
	migrateConnStr := "pgx5" + strings.TrimPrefix(connStr, "postgres")
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	m, err := gmigrate.NewWithSourceInstance("iofs", src, migrateConnStr)
	if err != nil {
		t.Fatalf("NewWithSourceInstance: %v", err)
	}
	if err := m.Up(); err != nil && err != gmigrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		t.Fatalf("migrate close source: %v", srcErr)
	}
	if dbErr != nil {
		t.Fatalf("migrate close db: %v", dbErr)
	}

	// Create pgxpool.
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// insertTestUser inserts a user row and returns the user ID.
func insertTestUser(t *testing.T, pool *pgxpool.Pool, userID, username string, isActive, isAdmin bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, password_hash, is_active, is_admin)
		 VALUES ($1, $2, '$2a$12$dummy', $3, $4)`,
		userID, username, isActive, isAdmin,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
}

// insertTestSession inserts a user_sessions row with the given token hash.
func insertTestSession(t *testing.T, pool *pgxpool.Pool, userID, tokenHash string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES (gen_random_uuid()::text, $1, $2, 'unused', now() + interval '30 days')`,
		userID, tokenHash,
	)
	if err != nil {
		t.Fatalf("insert test session: %v", err)
	}
}

func TestJWTMiddleware_ValidSession(t *testing.T) {
	pool := setupTestPool(t)
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	insertTestUser(t, pool, userID, "testuser", true, true)

	accessToken, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertTestSession(t, pool, userID, auth.HashToken(accessToken))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handlerCalled := false
	handler := func(c *echo.Context) error {
		handlerCalled = true

		if got := auth.UserIDFromContext(c); got != userID {
			t.Errorf("UserIDFromContext = %q, want %q", got, userID)
		}
		if got := auth.IsAdminFromContext(c); got != true {
			t.Errorf("IsAdminFromContext = %v, want true", got)
		}
		user := c.Get("user")
		if user == nil {
			t.Fatal("user not set on context")
		}
		au, ok := user.(*auth.AuthUser)
		if !ok {
			t.Fatalf("user is %T, want *auth.AuthUser", user)
		}
		if au.Username != "testuser" {
			t.Errorf("Username = %q, want %q", au.Username, "testuser")
		}
		return c.String(http.StatusOK, "ok")
	}

	mw := auth.JWTMiddleware(secret, pool)(handler)
	err = mw(c)
	if err != nil {
		t.Fatalf("middleware error: %v", err)
	}
	if !handlerCalled {
		t.Error("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestJWTMiddleware_RevokedSession(t *testing.T) {
	pool := setupTestPool(t)
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440001"

	insertTestUser(t, pool, userID, "revokeduser", true, false)
	// Do NOT insert a session — simulates revocation.

	accessToken, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called for revoked session")
		return nil
	}

	mw := auth.JWTMiddleware(secret, pool)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_InactiveUser(t *testing.T) {
	pool := setupTestPool(t)
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440002"

	insertTestUser(t, pool, userID, "inactiveuser", false, false)

	accessToken, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertTestSession(t, pool, userID, auth.HashToken(accessToken))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called for inactive user")
		return nil
	}

	mw := auth.JWTMiddleware(secret, pool)(handler)
	_ = mw(c)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/auth/... -v -count=1
```

Expected: all PASS (including testcontainers tests — may take 10-15s for container startup).

- [ ] **Step 3: Commit**

```bash
git add internal/auth/jwt_test.go
git commit -m "test(auth): add DB-backed JWT middleware tests with testcontainers"
```

---

### Task 6: Verify full test suite and lint

**Files:** none (verification only)

- [ ] **Step 1: Run the full project test suite**

```bash
go test ./... -count=1
```

Expected: all PASS across all packages.

- [ ] **Step 2: Run the linter**

```bash
golangci-lint run ./...
```

Expected: no errors.

- [ ] **Step 3: Verify the build**

```bash
go build ./cmd/nexorious
```

Expected: builds successfully (the auth package is not wired into the router yet, but it should compile).
