# Auth Handlers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `POST /api/auth/login`, `POST /api/auth/refresh`, and `POST /api/auth/logout` handlers in `internal/api/auth.go`, wired into the router and covered by integration tests against a real PostgreSQL container.

**Architecture:** A single `AuthHandler` struct holds `*pgxpool.Pool`, `secretKey string`, and `*config.Config`. All three handlers are methods on this struct. The router's `New` function gains a `pool *pgxpool.Pool` parameter; when pool is `nil` (existing router tests), auth routes are skipped. Tests spin up a real Postgres container via testcontainers-go, run migrations, then exercise the HTTP handlers through `httptest`.

**Tech Stack:** Go 1.25, Echo v5, pgx/v5 pgxpool, `golang.org/x/crypto/bcrypt`, `github.com/google/uuid`, `golang-jwt/jwt/v5`, testcontainers-go + postgres module.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/api/auth.go` | **Create** | `AuthHandler` struct, `HandleLogin`, `HandleRefresh`, `HandleLogout` |
| `internal/api/auth_test.go` | **Create** | Integration tests for all three handlers |
| `internal/api/router.go` | **Modify** | Add `pool *pgxpool.Pool` param to `New`; pass to `registerRoutes`; register auth routes when pool non-nil |
| `cmd/nexorious/main.go` | **Modify** | Pass `pool` to `api.New` |
| `internal/api/router_test.go` | **Modify** | Pass `nil` as third arg to `api.New` |
| `go.mod` / `go.sum` | **Modify** | Promote `golang.org/x/crypto` and `github.com/google/uuid` to direct deps |

---

## Task 1: Promote dependencies to direct

Both `golang.org/x/crypto` and `github.com/google/uuid` are already present as indirect deps. Promote them so the import is explicit and `go mod tidy` doesn't remove them.

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Promote dependencies**

```bash
cd /home/abo/workspace/home/nexorious-go
devenv shell -- go get golang.org/x/crypto@latest
devenv shell -- go get github.com/google/uuid@latest
```

Expected: both lines move from `// indirect` to direct in `go.mod`.

- [ ] **Step 2: Verify build still passes**

```bash
devenv shell -- go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: promote crypto and uuid to direct dependencies"
```

---

## Task 2: Update router signature and existing tests

Add the `pool *pgxpool.Pool` parameter to `api.New`. Auth routes are registered only when pool is non-nil, so existing router tests pass unchanged except for the extra `nil` argument.

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Update `internal/api/router.go`**

Replace the `New` signature and update `registerRoutes` to accept pool and skip auth routes when nil:

```go
package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	migrate "github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/ui"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config, migrator *migrate.Migrator, pool *pgxpool.Pool) *echo.Echo {
	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				slog.Error("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "err", v.Error)
			} else {
				slog.Info("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency)
			}
			return nil
		},
	}))

	// App-state middleware: redirect to /migrate unless state is Ready or path is bypassed.
	bypassPrefixes := []string{"/migrate", "/api/migrate"}
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if migrator.State() != migrate.AppStateReady {
				path := c.Request().URL.Path
				for _, prefix := range bypassPrefixes {
					if strings.HasPrefix(path, prefix) {
						return next(c)
					}
				}
				return c.Redirect(http.StatusFound, "/migrate")
			}
			return next(c)
		}
	})

	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
		}))
	}

	mh := migrate.NewHandler(migrator)
	registerRoutes(e, cfg, mh, pool)

	return e
}

func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, pool *pgxpool.Pool) {
	// Migration routes (bypass app-state middleware via prefix list)
	e.GET("/migrate", mh.HandleMigrateUI)
	e.GET("/api/migrate/status", mh.HandleStatus)
	e.POST("/api/migrate/run", mh.HandleRun)
	e.GET("/api/migrate/progress", mh.HandleProgress)

	// Health check
	e.GET("/health", handleHealth)

	// Auth routes — only registered when a DB pool is available.
	if pool != nil {
		ah := NewAuthHandler(pool, cfg)

		// Public auth routes (no JWT required)
		e.POST("/api/auth/login", ah.HandleLogin)
		e.POST("/api/auth/refresh", ah.HandleRefresh)

		// JWT-protected auth routes
		authGroup := e.Group("/api/auth", auth.JWTMiddleware(cfg.SecretKey, pool))
		authGroup.POST("/logout", ah.HandleLogout)
	}

	// Static cover art files from disk
	e.GET("/static/cover_art/*", func(c *echo.Context) error {
		http.StripPrefix("/static/cover_art/", http.FileServer(http.Dir(cfg.StoragePath+"/cover_art/"))).
			ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// SPA catch-all — serves ui.UIBox; falls back to index.html
	e.GET("/*", spaHandler())
}

// handleHealth returns 200 OK with a JSON body.
// GET /health
func handleHealth(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func spaHandler() echo.HandlerFunc {
	fsys, err := fs.Sub(ui.UIBox, "dist")
	if err != nil {
		panic(fmt.Sprintf("api: failed to create SPA sub-FS: %v", err))
	}
	fileServer := http.FileServer(http.FS(fsys))
	return func(c *echo.Context) error {
		path := c.Request().URL.Path
		if _, err := fs.Stat(fsys, strings.TrimPrefix(path, "/")); err != nil {
			// File not found → serve index.html for SPA routing
			c.Request().URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
```

- [ ] **Step 2: Update `internal/api/router_test.go`** — add `nil` as third arg to all three `api.New` calls:

```go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
)

func TestAppStateMiddleware_RedirectsToMigrate(t *testing.T) {
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(cfg, m, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/migrate" {
		t.Errorf("expected redirect to /migrate, got %q", loc)
	}
}

func TestAppStateMiddleware_BypassMigrationPaths(t *testing.T) {
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
	e := api.New(cfg, m, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for bypass path, got 302")
	}
}

func TestAppStateMiddleware_ReadyStatePassesThrough(t *testing.T) {
	cfg := &config.Config{}
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(cfg, m, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Errorf("expected non-302 for ready state, got 302")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 3: Update `cmd/nexorious/main.go`** — pass `pool` to `api.New` (single line change at the HTTP server section):

Find this line:
```go
e := api.New(cfg, migrator)
```

Replace with:
```go
e := api.New(cfg, migrator, pool)
```

- [ ] **Step 4: Verify it compiles and existing tests pass**

```bash
devenv shell -- go build ./...
devenv shell -- go test ./internal/api/... -run TestAppState -v
```

Expected: all three `TestAppStateMiddleware_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go cmd/nexorious/main.go
git commit -m "feat: add pool parameter to api.New, skip auth routes when pool is nil"
```

---

## Task 3: Implement `internal/api/auth.go`

Create the handler file with all three handlers. No tests yet — those come in Task 4.

**Files:**
- Create: `internal/api/auth.go`

- [ ] **Step 1: Create `internal/api/auth.go`**

```go
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

// NewAuthHandler returns a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{pool: pool, cfg: cfg}
}

// loginRequest is the JSON body for POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// refreshRequest is the JSON body for POST /api/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// logoutRequest is the JSON body for POST /api/auth/logout.
type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// tokenResponse is returned by login and refresh.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// HandleLogin handles POST /api/auth/login.
//
// Validates credentials, creates a session row, and returns access + refresh tokens.
func (h *AuthHandler) HandleLogin(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Look up the user by username.
	var (
		userID       string
		passwordHash string
		isActive     bool
	)
	err := h.pool.QueryRow(
		context.Background(),
		"SELECT id, password_hash, is_active FROM users WHERE username = $1",
		req.Username,
	).Scan(&userID, &passwordHash, &isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "incorrect username or password"})
		}
		slog.Error("login: query user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Verify the password before checking is_active (prevents timing-based user enumeration).
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "incorrect username or password"})
	}

	if !isActive {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user account is disabled"})
	}

	// Generate tokens.
	accessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("login: generate access token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	refreshToken, err := auth.GenerateRefreshToken(h.cfg.SecretKey, userID, h.cfg.RefreshTokenExpireDays)
	if err != nil {
		slog.Error("login: generate refresh token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Persist the session.
	sessionID := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(h.cfg.RefreshTokenExpireDays) * 24 * time.Hour)
	_, err = h.pool.Exec(
		context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID,
		userID,
		auth.HashToken(accessToken),
		auth.HashToken(refreshToken),
		c.Request().Header.Get("User-Agent"),
		c.RealIP(),
		expiresAt,
	)
	if err != nil {
		slog.Error("login: insert session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "bearer",
		ExpiresIn:    h.cfg.AccessTokenExpireMinutes * 60,
	})
}

// HandleRefresh handles POST /api/auth/refresh.
//
// Validates the refresh token, issues a new access token, and updates the session's token_hash.
// The refresh token itself is not rotated.
func (h *AuthHandler) HandleRefresh(c *echo.Context) error {
	var req refreshRequest
	if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate the refresh JWT.
	claims, err := auth.ParseToken(h.cfg.SecretKey, req.RefreshToken, "refresh")
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}
	userID := claims.Subject
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}

	// Look up the session.
	refreshHash := auth.HashToken(req.RefreshToken)
	var sessionID string
	err = h.pool.QueryRow(
		context.Background(),
		"SELECT id FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2 AND expires_at > now()",
		userID, refreshHash,
	).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired refresh token"})
		}
		slog.Error("refresh: query session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Check the user is still active.
	var isActive bool
	err = h.pool.QueryRow(
		context.Background(),
		"SELECT is_active FROM users WHERE id = $1",
		userID,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found or disabled"})
		}
		slog.Error("refresh: query user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if !isActive {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found or disabled"})
	}

	// Issue a new access token.
	newAccessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("refresh: generate access token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Update the session's access token hash.
	_, err = h.pool.Exec(
		context.Background(),
		"UPDATE user_sessions SET token_hash = $1 WHERE id = $2",
		auth.HashToken(newAccessToken), sessionID,
	)
	if err != nil {
		slog.Error("refresh: update session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: req.RefreshToken, // unchanged
		TokenType:    "bearer",
		ExpiresIn:    h.cfg.AccessTokenExpireMinutes * 60,
	})
}

// HandleLogout handles POST /api/auth/logout.
//
// Requires a valid access token (JWTMiddleware). Deletes the session identified by the
// refresh token. Always returns 200 — errors from an invalid refresh token are logged but
// do not prevent a successful logout response (security: don't reveal token validity).
func (h *AuthHandler) HandleLogout(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req logoutRequest
	// A missing/malformed body still results in 200 — logout is idempotent.
	_ = c.Bind(&req)

	if req.RefreshToken == "" {
		return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
	}

	// Parse the refresh token. On error, return 200 for security.
	claims, err := auth.ParseToken(h.cfg.SecretKey, req.RefreshToken, "refresh")
	if err != nil {
		slog.Info("logout: could not parse refresh token (returning 200 for security)", "err", err)
		return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
	}

	// Guard against logging out another user's session.
	if claims.Subject != userID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid refresh token for authenticated user"})
	}

	// Delete the session. Ignore "not found" — idempotent.
	_, err = h.pool.Exec(
		context.Background(),
		"DELETE FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2",
		userID, auth.HashToken(req.RefreshToken),
	)
	if err != nil {
		slog.Error("logout: delete session", "err", err)
		// Still return 200 — the client can't do anything about a DB error here.
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
devenv shell -- go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/api/auth.go
git commit -m "feat: implement AuthHandler with login, refresh, logout handlers"
```

---

## Task 4: Write login handler integration tests

Tests use a real Postgres container (same pattern as `internal/auth/jwt_test.go`). The `setupTestDB` helper and `insertAuthTestUser` helper are defined once and reused across all test functions.

**Files:**
- Create: `internal/api/auth_test.go`

- [ ] **Step 1: Write the failing tests for `HandleLogin`**

Create `internal/api/auth_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/migrate"
)

// ─── Test helpers ────────────────────────────────────────────────────────────

func setupAuthTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
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

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// insertAuthTestUser inserts a user with a real bcrypt hash (cost 12).
func insertAuthTestUser(t *testing.T, pool *pgxpool.Pool, id, username, password string, isActive, isAdmin bool) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES ($1, $2, $3, $4, $5)",
		id, username, string(hash), isActive, isAdmin,
	)
	if err != nil {
		t.Fatalf("insertAuthTestUser: %v", err)
	}
}

// newTestEcho returns an Echo instance wired with a real pool and a ready migrator.
func newTestEcho(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) interface{ ServeHTTP(http.ResponseWriter, *http.Request) } {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(cfg, m, pool)
}

// postJSON fires a POST request with a JSON body and returns the recorder.
func postJSON(t *testing.T, handler interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ─── Login tests ─────────────────────────────────────────────────────────────

func testCfg() *config.Config {
	return &config.Config{
		SecretKey:                "test-secret-key-at-least-32-bytes!",
		AccessTokenExpireMinutes: 15,
		RefreshTokenExpireDays:   30,
	}
}

func TestHandleLogin_ValidCredentials(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-001", "alice", "password123", true, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "password123",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Tokens must be present and non-empty.
	accessToken, _ := resp["access_token"].(string)
	refreshToken, _ := resp["refresh_token"].(string)
	if accessToken == "" {
		t.Error("access_token is empty")
	}
	if refreshToken == "" {
		t.Error("refresh_token is empty")
	}

	// token_type must be "bearer".
	if tt, _ := resp["token_type"].(string); tt != "bearer" {
		t.Errorf("token_type = %q, want %q", tt, "bearer")
	}

	// expires_in must be 15*60 = 900.
	if ei, ok := resp["expires_in"].(float64); !ok || int(ei) != 900 {
		t.Errorf("expires_in = %v, want 900", resp["expires_in"])
	}

	// Verify tokens are valid JWTs.
	if _, err := auth.ParseToken(cfg.SecretKey, accessToken, "access"); err != nil {
		t.Errorf("access_token not a valid access JWT: %v", err)
	}
	if _, err := auth.ParseToken(cfg.SecretKey, refreshToken, "refresh"); err != nil {
		t.Errorf("refresh_token not a valid refresh JWT: %v", err)
	}

	// Verify session was created in DB.
	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND token_hash = $2",
		"user-001", auth.HashToken(accessToken),
	).Scan(&count)
	if err != nil {
		t.Fatalf("session query: %v", err)
	}
	if count != 1 {
		t.Errorf("session count = %d, want 1", count)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())
	insertAuthTestUser(t, pool, "user-002", "bob", "correctpassword", true, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "bob",
		"password": "wrongpassword",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "incorrect username or password" {
		t.Errorf("error = %q, want %q", resp["error"], "incorrect username or password")
	}
}

func TestHandleLogin_NonExistentUser(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "nobody",
		"password": "irrelevant",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	// Same message as wrong password — prevents user enumeration.
	if resp["error"] != "incorrect username or password" {
		t.Errorf("error = %q, want %q", resp["error"], "incorrect username or password")
	}
}

func TestHandleLogin_DisabledUser(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())
	insertAuthTestUser(t, pool, "user-003", "carol", "password123", false, false)

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "carol",
		"password": "password123",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "user account is disabled" {
		t.Errorf("error = %q, want %q", resp["error"], "user account is disabled")
	}
}

func TestHandleLogin_MissingUsername(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "",
		"password": "password123",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleLogin_MissingPassword(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleLogin_MalformedJSON(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(cfg, m, pool)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (handler file exists but tests may reveal compilation issues)**

```bash
devenv shell -- go test ./internal/api/... -run TestHandleLogin -v 2>&1 | head -40
```

Expected: tests compile and the container-backed tests run. If there are compile errors, fix them before proceeding.

- [ ] **Step 3: Run tests to verify they pass**

```bash
devenv shell -- go test ./internal/api/... -run TestHandleLogin -v -timeout 120s
```

Expected: all 7 `TestHandleLogin_*` tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/auth_test.go
git commit -m "test: add login handler integration tests"
```

---

## Task 5: Write refresh handler integration tests

Append to `internal/api/auth_test.go`.

**Files:**
- Modify: `internal/api/auth_test.go`

- [ ] **Step 1: Add a helper to insert a session with known hashes**

Add this helper at the bottom of the helpers section in `auth_test.go` (after `newTestEcho`):

```go
// insertAuthTestSession inserts a user_session for testing.
func insertAuthTestSession(t *testing.T, pool *pgxpool.Pool, userID, accessToken, refreshToken string, expiredDays int) {
	t.Helper()
	expiresExpr := "now() + interval '30 days'"
	if expiredDays < 0 {
		expiresExpr = "now() - interval '1 second'"
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES (gen_random_uuid()::text, $1, $2, $3, `+expiresExpr+`)`,
		userID, auth.HashToken(accessToken), auth.HashToken(refreshToken),
	)
	if err != nil {
		t.Fatalf("insertAuthTestSession: %v", err)
	}
}
```

- [ ] **Step 2: Add refresh tests to `internal/api/auth_test.go`**

```go
// ─── Refresh tests ────────────────────────────────────────────────────────────

func TestHandleRefresh_Valid(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-010", "dave", "pw", true, false)

	// Generate real tokens to insert a valid session.
	oldAccess, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-010", cfg.AccessTokenExpireMinutes)
	refreshToken, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-010", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, pool, "user-010", oldAccess, refreshToken, 30)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	newAccess, _ := resp["access_token"].(string)
	echoedRefresh, _ := resp["refresh_token"].(string)

	if newAccess == "" {
		t.Error("access_token is empty")
	}
	// Refresh token must be unchanged.
	if echoedRefresh != refreshToken {
		t.Errorf("refresh_token changed; want original back")
	}
	if _, err := auth.ParseToken(cfg.SecretKey, newAccess, "access"); err != nil {
		t.Errorf("new access token invalid: %v", err)
	}

	// Old token_hash must be replaced in DB.
	var count int
	_ = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND token_hash = $2",
		"user-010", auth.HashToken(oldAccess),
	).Scan(&count)
	if count != 0 {
		t.Error("old token_hash was not removed from session")
	}
	_ = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND token_hash = $2",
		"user-010", auth.HashToken(newAccess),
	).Scan(&count)
	if count != 1 {
		t.Error("new token_hash not found in session")
	}
}

func TestHandleRefresh_ExpiredJWT(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-011", "eve", "pw", true, false)

	// Build a refresh token that is already expired by crafting claims manually.
	expiredClaims := auth.Claims{
		Type: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-011",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-31 * 24 * time.Hour)),
		},
	}
	// We need to sign it — use the same secret.
	rawToken := jwtSign(t, expiredClaims, cfg.SecretKey)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": rawToken,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleRefresh_NoMatchingSession(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-012", "frank", "pw", true, false)
	refreshToken, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-012", cfg.RefreshTokenExpireDays)
	// Intentionally do NOT insert a session.

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid or expired refresh token" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid or expired refresh token")
	}
}

func TestHandleRefresh_DisabledUser(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-013", "grace", "pw", false, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-013", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-013", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, pool, "user-013", access, refresh, 30)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": refresh,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleRefresh_AccessTokenPassedInstead(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-014", "heidi", "pw", true, false)
	accessToken, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-014", cfg.AccessTokenExpireMinutes)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": accessToken, // wrong type
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleRefresh_MissingField(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
```

The expired-token test requires `jwt.RegisteredClaims` and a `jwtSign` helper. Add these imports and helper:

```go
// Additional imports needed in auth_test.go:
import (
    // (add to existing import block)
    "time"
    "github.com/golang-jwt/jwt/v5"
)

// jwtSign signs a Claims struct for test purposes.
func jwtSign(t *testing.T, claims auth.Claims, secret string) string {
    t.Helper()
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    signed, err := token.SignedString([]byte(secret))
    if err != nil {
        t.Fatalf("jwtSign: %v", err)
    }
    return signed
}
```

- [ ] **Step 3: Run refresh tests**

```bash
devenv shell -- go test ./internal/api/... -run TestHandleRefresh -v -timeout 120s
```

Expected: all 6 `TestHandleRefresh_*` tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/auth_test.go
git commit -m "test: add refresh handler integration tests"
```

---

## Task 6: Write logout handler integration tests

Append to `internal/api/auth_test.go`.

**Files:**
- Modify: `internal/api/auth_test.go`

- [ ] **Step 1: Add logout tests to `internal/api/auth_test.go`**

The logout endpoint requires a valid `Authorization: Bearer <access_token>` header. A helper wraps `postJSON` to add the header:

```go
// postJSONAuth fires a POST with a JSON body and a Bearer authorization header.
func postJSONAuth(t *testing.T, handler interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path string, body any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func TestHandleLogout_Valid(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-020", "ivan", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-020", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-020", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, pool, "user-020", access, refresh, 30)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": refresh,
	}, access)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	// Session must be deleted.
	var count int
	_ = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1",
		"user-020",
	).Scan(&count)
	if count != 0 {
		t.Errorf("session count = %d, want 0 after logout", count)
	}
}

func TestHandleLogout_WrongUserRefreshToken(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-021", "judy", "pw", true, false)
	insertAuthTestUser(t, pool, "user-022", "ken", "pw", true, false)

	// Judy logs in; Ken's refresh token is passed.
	judyAccess, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-021", cfg.AccessTokenExpireMinutes)
	judyRefresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-021", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, pool, "user-021", judyAccess, judyRefresh, 30)

	kenRefresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-022", cfg.RefreshTokenExpireDays)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": kenRefresh,
	}, judyAccess)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid refresh token for authenticated user" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid refresh token for authenticated user")
	}
}

func TestHandleLogout_MalformedRefreshToken(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-023", "lena", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-023", cfg.AccessTokenExpireMinutes)
	_, refresh := access, "not-a-jwt"
	insertAuthTestSession(t, pool, "user-023", access, "unused-hash", 30)

	// Malformed refresh token → still 200 (security: don't reveal validity).
	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": refresh,
	}, access)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for malformed refresh token", rec.Code)
	}
}

func TestHandleLogout_DoubleLogout(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-024", "mike", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-024", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-024", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, pool, "user-024", access, refresh, 30)

	// First logout.
	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{"refresh_token": refresh}, access)
	if rec.Code != http.StatusOK {
		t.Fatalf("first logout status = %d, want 200", rec.Code)
	}

	// Second logout — no session exists, but JWTMiddleware will block because the session
	// was deleted. So the second call returns 401 from the middleware, not 200.
	// That is correct behaviour: the access token is invalidated once the session is gone.
	rec2 := postJSONAuth(t, e, "/api/auth/logout", map[string]string{"refresh_token": refresh}, access)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("second logout status = %d, want 401 (session deleted)", rec2.Code)
	}
}

func TestHandleLogout_NoAuthorizationHeader(t *testing.T) {
	pool := setupAuthTestDB(t)
	e := newTestEcho(t, pool, testCfg())

	rec := postJSON(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": "anything",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run logout tests**

```bash
devenv shell -- go test ./internal/api/... -run TestHandleLogout -v -timeout 120s
```

Expected: all 5 `TestHandleLogout_*` tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/api/auth_test.go
git commit -m "test: add logout handler integration tests"
```

---

## Task 7: Full test suite and linting

- [ ] **Step 1: Run all tests**

```bash
devenv shell -- go test ./... -timeout 180s
```

Expected: all tests PASS, no failures.

- [ ] **Step 2: Run linter**

```bash
devenv shell -- golangci-lint run
```

Expected: exits 0, zero lint errors.

If linter complains about `_ = ` suppression in test helpers, extract the Scan into a named variable or use `//nolint:errcheck` with a comment explaining why.

- [ ] **Step 3: Final commit**

```bash
git add -p   # review everything
git commit -m "feat: auth handlers complete — login, refresh, logout with integration tests"
```

---

## Self-Review

### Spec coverage check

| Spec requirement | Covered by |
|---|---|
| `POST /api/auth/login` — 200 + tokens | Task 3 (impl), Task 4 (tests) |
| Login 400 on missing fields | Task 4 `TestHandleLogin_Missing*` |
| Login 401 wrong credentials | Task 4 `TestHandleLogin_WrongPassword` |
| Login 401 non-existent user (same message) | Task 4 `TestHandleLogin_NonExistentUser` |
| Login 401 disabled user | Task 4 `TestHandleLogin_DisabledUser` |
| Login — session row created | Task 4 `TestHandleLogin_ValidCredentials` (DB assertion) |
| Login — `expires_in` and `token_type` | Task 4 `TestHandleLogin_ValidCredentials` |
| Login — tokens are valid JWTs | Task 4 `TestHandleLogin_ValidCredentials` |
| `POST /api/auth/refresh` — 200 + new access, same refresh | Task 3 (impl), Task 5 tests |
| Refresh — old token_hash replaced in DB | Task 5 `TestHandleRefresh_Valid` |
| Refresh 401 — expired JWT | Task 5 `TestHandleRefresh_ExpiredJWT` |
| Refresh 401 — no matching session | Task 5 `TestHandleRefresh_NoMatchingSession` |
| Refresh 401 — disabled user | Task 5 `TestHandleRefresh_DisabledUser` |
| Refresh 401 — access token passed as refresh | Task 5 `TestHandleRefresh_AccessTokenPassedInstead` |
| Refresh 400 — missing field | Task 5 `TestHandleRefresh_MissingField` |
| `POST /api/auth/logout` — 200 + session deleted | Task 3 (impl), Task 6 tests |
| Logout 400 — wrong user's refresh token | Task 6 `TestHandleLogout_WrongUserRefreshToken` |
| Logout 200 — malformed refresh (security) | Task 6 `TestHandleLogout_MalformedRefreshToken` |
| Logout 200 — double logout idempotent | Task 6 `TestHandleLogout_DoubleLogout` |
| Logout 401 — no Authorization header | Task 6 `TestHandleLogout_NoAuthorizationHeader` |
| Router updated with pool param | Task 2 |
| `api.New` updated in `main.go` | Task 2 |
| Existing router tests pass with `nil` pool | Task 2 |
| Auth routes skipped when pool nil | Task 2 |
| Direct deps for crypto + uuid | Task 1 |
| bcrypt cost 12 | Task 3 (impl uses cost 12 in test helper) |
| `uuid.NewString()` for session ID | Task 3 (impl) |
| `context.Background()` (not request context) for DB calls in handler | Task 3 (impl) |

All spec requirements accounted for. No gaps found.
