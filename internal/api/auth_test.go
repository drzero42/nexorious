package api_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// insertAuthTestSession inserts a user_session for testing.
// expiredDays < 0 means the session is already expired.
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

// newTestEcho returns an Echo instance wired with a real pool and a ready migrator.
func newTestEcho(t *testing.T, pool *pgxpool.Pool, cfg *config.Config) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(cfg, m, pool)
}

// testCfg returns a minimal config suitable for auth tests.
func testCfg() *config.Config {
	return &config.Config{
		SecretKey:                "test-secret-key-at-least-32-bytes!",
		AccessTokenExpireMinutes: 15,
		RefreshTokenExpireDays:   30,
	}
}

// postJSON fires a POST request with a JSON body and returns the recorder.
func postJSON(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any) *httptest.ResponseRecorder {
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

// postJSONAuth fires a POST with a JSON body and a Bearer authorization header.
func postJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, accessToken string) *httptest.ResponseRecorder {
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

// jwtSign signs a Claims struct for test purposes (e.g. to build expired tokens).
func jwtSign(t *testing.T, claims auth.Claims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("jwtSign: %v", err)
	}
	return signed
}

// ─── Login tests ─────────────────────────────────────────────────────────────

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

	accessToken, _ := resp["access_token"].(string)
	refreshToken, _ := resp["refresh_token"].(string)
	if accessToken == "" {
		t.Error("access_token is empty")
	}
	if refreshToken == "" {
		t.Error("refresh_token is empty")
	}
	if tt, _ := resp["token_type"].(string); tt != "bearer" {
		t.Errorf("token_type = %q, want %q", tt, "bearer")
	}
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
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = $1 AND token_hash = $2",
		"user-001", auth.HashToken(accessToken),
	).Scan(&count); err != nil {
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

// ─── Refresh tests ────────────────────────────────────────────────────────────

func TestHandleRefresh_Valid(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, pool, cfg)

	insertAuthTestUser(t, pool, "user-010", "dave", "pw", true, false)

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
	if echoedRefresh != refreshToken {
		t.Error("refresh_token changed; want original back")
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

	expiredClaims := auth.Claims{
		Type: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-011",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-31 * 24 * time.Hour)),
		},
	}
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
	insertAuthTestSession(t, pool, "user-023", access, "unused-hash-placeholder", 30)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": "not-a-jwt",
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

	// Second logout — session deleted, so JWTMiddleware blocks with 401.
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
