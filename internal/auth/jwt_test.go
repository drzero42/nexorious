package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
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
	expectedExpiry := time.Now().Add(15 * time.Minute)
	diff := claims.ExpiresAt.Sub(expectedExpiry)
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
	expectedExpiry := time.Now().Add(30 * 24 * time.Hour)
	diff := claims.ExpiresAt.Sub(expectedExpiry)
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

	_, err = auth.ParseToken(secret, accessToken, "refresh")
	if err == nil {
		t.Fatal("expected error when parsing access token as refresh")
	}

	refreshToken, err := auth.GenerateRefreshToken(secret, userID, 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	_, err = auth.ParseToken(secret, refreshToken, "access")
	if err == nil {
		t.Fatal("expected error when parsing refresh token as access")
	}
}

func TestParseToken_Expired(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

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

	hash3 := auth.HashToken("different-token")
	if hash1 == hash3 {
		t.Error("different inputs produced the same hash")
	}

	const knownInput = "hello"
	const knownHash = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if auth.HashToken(knownInput) != knownHash {
		t.Errorf("HashToken(%q) = %s, want %s", knownInput, auth.HashToken(knownInput), knownHash)
	}
}

func TestGenerateAccessToken_EmptyKey(t *testing.T) {
	_, err := auth.GenerateAccessToken("", "user1", 15)
	if err == nil {
		t.Fatal("expected error when secretKey is empty")
	}
}

func TestGenerateRefreshToken_EmptyKey(t *testing.T) {
	_, err := auth.GenerateRefreshToken("", "user1", 30)
	if err == nil {
		t.Fatal("expected error when secretKey is empty")
	}
}

func TestParseToken_EmptyKey(t *testing.T) {
	_, err := auth.ParseToken("", "some.token.string", "access")
	if err == nil {
		t.Fatal("expected error when secretKey is empty")
	}
}

func TestGenerateAccessToken_ZeroExpiry(t *testing.T) {
	_, err := auth.GenerateAccessToken("test-secret-key-at-least-32-bytes!", "user1", 0)
	if err == nil {
		t.Fatal("expected error when expireMinutes is 0")
	}
	_, err = auth.GenerateAccessToken("test-secret-key-at-least-32-bytes!", "user1", -1)
	if err == nil {
		t.Fatal("expected error when expireMinutes is negative")
	}
}

func TestGenerateRefreshToken_ZeroDays(t *testing.T) {
	_, err := auth.GenerateRefreshToken("test-secret-key-at-least-32-bytes!", "user1", 0)
	if err == nil {
		t.Fatal("expected error when expireDays is 0")
	}
	_, err = auth.GenerateRefreshToken("test-secret-key-at-least-32-bytes!", "user1", -1)
	if err == nil {
		t.Fatal("expected error when expireDays is negative")
	}
}

func TestContextHelpers(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Before setting — defaults.
	if got := auth.UserIDFromContext(c); got != "" {
		t.Errorf("UserIDFromContext before Set = %q, want empty", got)
	}
	if auth.IsAdminFromContext(c) {
		t.Error("IsAdminFromContext before Set should be false")
	}

	// After setting.
	c.Set("user_id", "user-123")
	c.Set("is_admin", true)

	if got := auth.UserIDFromContext(c); got != "user-123" {
		t.Errorf("UserIDFromContext = %q, want %q", got, "user-123")
	}
	if !auth.IsAdminFromContext(c) {
		t.Error("IsAdminFromContext after Set should be true")
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
	if err := mw(c); err != nil {
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
	if err := mw(c); err != nil {
		t.Fatalf("AdminMiddleware returned unexpected error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAdminMiddleware_NoAdminKey(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called when is_admin is absent")
		return nil
	}

	mw := auth.AdminMiddleware()(handler)
	if err := mw(c); err != nil {
		t.Fatalf("AdminMiddleware returned unexpected error: %v", err)
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// --- JWTMiddleware no-DB tests ---

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := func(c *echo.Context) error {
		t.Error("handler should not be called without auth header")
		return nil
	}

	mw := auth.JWTMiddleware("secret", nil)(handler)
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJWTMiddleware_RefreshTokenRejected(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
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
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// --- JWTMiddleware DB-backed tests ---

func setupTestPool(t *testing.T) *pgxpool.Pool {
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

	// Run migrations using the same pgx5:// scheme as the rest of the project.
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

func insertTestUser(t *testing.T, pool *pgxpool.Pool, userID, username string, isActive, isAdmin bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, password_hash, is_active, is_admin)
		 VALUES ($1, $2, '$2a$12$dummyhashvalue000000000000000000000000000000000000000000', $3, $4)`,
		userID, username, isActive, isAdmin,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
}

func insertTestSession(t *testing.T, pool *pgxpool.Pool, userID, tokenHash string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES (gen_random_uuid()::text, $1, $2, 'unused_refresh_hash', now() + interval '30 days')`,
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
		if !auth.IsAdminFromContext(c) {
			t.Error("IsAdminFromContext should be true")
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
	if err := mw(c); err != nil {
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
	// No session inserted — simulates a revoked/logged-out token.

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
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
