package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/migrate"
)

// ─── Test helpers ────────────────────────────────────────────────────────────

// insertAuthTestUser inserts a user with a real bcrypt hash (cost 12).
func insertAuthTestUser(t *testing.T, db *bun.DB, id, username, password string, isActive, isAdmin bool) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, ?, ?)",
		id, username, string(hash), isActive, isAdmin,
	)
	if err != nil {
		t.Fatalf("insertAuthTestUser: %v", err)
	}
}

// insertAuthTestSession inserts a user_session for testing.
// expiredDays < 0 means the session is already expired.
func insertAuthTestSession(t *testing.T, db *bun.DB, userID, accessToken, refreshToken string, expiredDays int) {
	t.Helper()
	expiresExpr := "now() + interval '30 days'"
	if expiredDays < 0 {
		expiresExpr = "now() - interval '1 second'"
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at)
		 VALUES (gen_random_uuid()::text, ?, ?, ?, `+expiresExpr+`)`,
		userID, auth.HashToken(accessToken), auth.HashToken(refreshToken),
	)
	if err != nil {
		t.Fatalf("insertAuthTestSession: %v", err)
	}
}

// newTestEcho returns an Echo instance wired with a real db and a ready migrator.
func newTestEcho(t *testing.T, db *bun.DB, cfg *config.Config) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(cfg, m, db, "", nil, nil, nil)
}

// newTestEchoPool returns an Echo instance wired with a real db, ready
// migrator, and a real River client backed by the shared test container so
// handler tests exercise production-realistic enqueue paths.
func newTestEchoPool(t *testing.T, db *bun.DB, cfg *config.Config) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	rc := newTestRiverClient(t)
	return api.New(cfg, m, db, "", nil, nil, nil, rc)
}

// newTestRiverClient builds a non-started River client against the shared
// test container — sufficient for handler tests that only need Insert to
// succeed.
func newTestRiverClient(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	return rc
}

// testCfg returns a minimal config suitable for api_test tests.
func testCfg() *config.Config {
	return &config.Config{
		SecretKey:                "test-secret-key-at-least-32-bytes!",
		AccessTokenExpireMinutes: 15,
		RefreshTokenExpireDays:   30,
		Port:                     8000,
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
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-001", "alice", "password123", true, false)

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
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND token_hash = ?",
		"user-001", auth.HashToken(accessToken),
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
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "incorrect username or password" {
		t.Errorf("error = %q, want %q", resp["message"], "incorrect username or password")
	}
}

func TestHandleLogin_NonExistentUser(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "nobody",
		"password": "irrelevant",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "incorrect username or password" {
		t.Errorf("error = %q, want %q", resp["message"], "incorrect username or password")
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
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "user account is disabled" {
		t.Errorf("error = %q, want %q", resp["message"], "user account is disabled")
	}
}

func TestHandleLogin_MissingUsername(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "",
		"password": "password123",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleLogin_MissingPassword(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleLogin_MalformedJSON(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(cfg, m, testDB, "", nil, nil, nil)

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
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-010", "dave", "pw", true, false)

	oldAccess, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-010", cfg.AccessTokenExpireMinutes)
	refreshToken, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-010", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, testDB, "user-010", oldAccess, refreshToken, 30)

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
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND token_hash = ?",
		"user-010", auth.HashToken(oldAccess),
	).Scan(&count)
	if count != 0 {
		t.Error("old token_hash was not removed from session")
	}
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND token_hash = ?",
		"user-010", auth.HashToken(newAccess),
	).Scan(&count)
	if count != 1 {
		t.Error("new token_hash not found in session")
	}
}

func TestHandleRefresh_ExpiredJWT(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-011", "eve", "pw", true, false)

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
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-012", "frank", "pw", true, false)
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
	if resp["message"] != "invalid or expired refresh token" {
		t.Errorf("error = %q, want %q", resp["message"], "invalid or expired refresh token")
	}
}

func TestHandleRefresh_DisabledUser(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-013", "grace", "pw", false, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-013", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-013", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, testDB, "user-013", access, refresh, 30)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": refresh,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleRefresh_AccessTokenPassedInstead(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-014", "heidi", "pw", true, false)
	accessToken, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-014", cfg.AccessTokenExpireMinutes)

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{
		"refresh_token": accessToken, // wrong type
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHandleRefresh_MissingField(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/refresh", map[string]string{})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// ─── Logout tests ─────────────────────────────────────────────────────────────

func TestHandleLogout_Valid(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-020", "ivan", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-020", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-020", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, testDB, "user-020", access, refresh, 30)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": refresh,
	}, access)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	// Session must be deleted.
	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?",
		"user-020",
	).Scan(&count)
	if count != 0 {
		t.Errorf("session count = %d, want 0 after logout", count)
	}
}

func TestHandleLogout_WrongUserRefreshToken(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-021", "judy", "pw", true, false)
	insertAuthTestUser(t, testDB, "user-022", "ken", "pw", true, false)

	judyAccess, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-021", cfg.AccessTokenExpireMinutes)
	judyRefresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-021", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, testDB, "user-021", judyAccess, judyRefresh, 30)

	kenRefresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-022", cfg.RefreshTokenExpireDays)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": kenRefresh,
	}, judyAccess)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "invalid refresh token for authenticated user" {
		t.Errorf("error = %q, want %q", resp["message"], "invalid refresh token for authenticated user")
	}
}

func TestHandleLogout_MalformedRefreshToken(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-023", "lena", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-023", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, testDB, "user-023", access, "unused-hash-placeholder", 30)

	rec := postJSONAuth(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": "not-a-jwt",
	}, access)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for malformed refresh token", rec.Code)
	}
}

func TestHandleLogout_DoubleLogout(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-024", "mike", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-024", cfg.AccessTokenExpireMinutes)
	refresh, _ := auth.GenerateRefreshToken(cfg.SecretKey, "user-024", cfg.RefreshTokenExpireDays)
	insertAuthTestSession(t, testDB, "user-024", access, refresh, 30)

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
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	rec := postJSON(t, e, "/api/auth/logout", map[string]string{
		"refresh_token": "anything",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// ─── GetMe tests ─────────────────────────────────────────────────────────────

func TestGetMe_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	userID := "user-me-001"
	insertAuthTestUser(t, testDB, userID, "admin", "password123", true, true)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", userID)

	if err := ah.HandleGetMe(c); err != nil {
		t.Fatalf("HandleGetMe: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var body struct {
		ID          string          `json:"id"`
		Username    string          `json:"username"`
		IsAdmin     bool            `json:"is_admin"`
		IsActive    bool            `json:"is_active"`
		Preferences json.RawMessage `json:"preferences"`
		CreatedAt   string          `json:"created_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != userID {
		t.Errorf("id mismatch: got %q want %q", body.ID, userID)
	}
	if body.Username != "admin" {
		t.Errorf("username mismatch: got %q want %q", body.Username, "admin")
	}
	if string(body.Preferences) == "" || string(body.Preferences) == "null" {
		t.Errorf("preferences must not be null, got %q", string(body.Preferences))
	}
}

func TestGetMe_Unauthorized_NoUserID(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	ah := api.NewAuthHandler(testDB, cfg)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := ah.HandleGetMe(c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", he.Code)
	}
}

// ─── ChangePassword tests ──────────────────────────────────────────────────

func TestHandleChangePassword_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chpwd-001"
	insertAuthTestUser(t, testDB, userID, "pwduser", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	}, accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify new password works by checking bcrypt.
	var hash string
	err = testDB.QueryRowContext(context.Background(),
		"SELECT password_hash FROM users WHERE id = ?", userID,
	).Scan(&hash)
	if err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpass456")); err != nil {
		t.Error("new password does not match stored hash")
	}
}

func TestHandleChangePassword_WrongCurrentPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chpwd-002"
	insertAuthTestUser(t, testDB, userID, "pwduser2", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "wrongpass",
		"new_password":     "newpass456",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_SamePassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chpwd-003"
	insertAuthTestUser(t, testDB, userID, "pwduser3", "samepass1", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "samepass1",
		"new_password":     "samepass1",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_TooShort(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chpwd-004"
	insertAuthTestUser(t, testDB, userID, "pwduser4", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "short",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_InvalidatesOtherSessions(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chpwd-005"
	insertAuthTestUser(t, testDB, userID, "pwduser5", "oldpass123", true, false)

	// Create the "current" session.
	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	// Create an "other" session (simulating another device).
	otherToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken other: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, otherToken, "", 30)

	// Change password using the first token.
	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	}, accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	// Current session should still exist.
	var currentCount int
	err = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND token_hash = ?",
		userID, auth.HashToken(accessToken),
	).Scan(&currentCount)
	if err != nil {
		t.Fatalf("count current session: %v", err)
	}
	if currentCount != 1 {
		t.Errorf("current session should be preserved, got count=%d", currentCount)
	}

	// Other session should be deleted.
	var otherCount int
	err = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND token_hash = ?",
		userID, auth.HashToken(otherToken),
	).Scan(&otherCount)
	if err != nil {
		t.Fatalf("count other session: %v", err)
	}
	if otherCount != 0 {
		t.Errorf("other session should be deleted, got count=%d", otherCount)
	}
}

// ─── UpdateMe tests ────────────────────────────────────────────────────────

func TestHandleUpdateMe_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-update-me-001"
	insertAuthTestUser(t, testDB, userID, "testuser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": map[string]any{"theme": "dark", "language": "en"},
	}, accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var body struct {
		ID          string          `json:"id"`
		Username    string          `json:"username"`
		Preferences json.RawMessage `json:"preferences"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != userID {
		t.Errorf("id mismatch: got %q want %q", body.ID, userID)
	}

	var prefs map[string]any
	if err := json.Unmarshal(body.Preferences, &prefs); err != nil {
		t.Fatalf("unmarshal preferences: %v", err)
	}
	if prefs["theme"] != "dark" {
		t.Errorf("theme: got %q want %q", prefs["theme"], "dark")
	}
}

func TestHandleUpdateMe_InvalidPreferences_Array(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-update-me-002"
	insertAuthTestUser(t, testDB, userID, "testuser2", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": []string{"not", "an", "object"},
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleUpdateMe_InvalidPreferences_Null(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-update-me-003"
	insertAuthTestUser(t, testDB, userID, "testuser3", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": nil,
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ─── CheckUsername tests ───────────────────────────────────────────────────

func TestHandleCheckUsername_Available(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chkusr-001"
	insertAuthTestUser(t, testDB, userID, "existinguser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := getAuth(t, e, "/api/auth/username/check/newname", accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var body struct {
		Available bool   `json:"available"`
		Username  string `json:"username"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Available {
		t.Error("expected available=true")
	}
	if body.Username != "newname" {
		t.Errorf("username: got %q want %q", body.Username, "newname")
	}
}

func TestHandleCheckUsername_Taken(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chkusr-002"
	insertAuthTestUser(t, testDB, userID, "takenname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := getAuth(t, e, "/api/auth/username/check/takenname", accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var body struct {
		Available bool `json:"available"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Available {
		t.Error("expected available=false for taken username")
	}
}

func TestHandleCheckUsername_TooShort(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chkusr-003"
	insertAuthTestUser(t, testDB, userID, "someuser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := getAuth(t, e, "/api/auth/username/check/ab", accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ─── ChangeUsername tests ──────────────────────────────────────────────────

func TestHandleChangeUsername_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chusr-001"
	insertAuthTestUser(t, testDB, userID, "oldname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "newname",
	}, accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var body struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Username != "newname" {
		t.Errorf("username: got %q want %q", body.Username, "newname")
	}
}

func TestHandleChangeUsername_SameAsCurrent(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chusr-002"
	insertAuthTestUser(t, testDB, userID, "samename", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "samename",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_AlreadyTaken(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chusr-003"
	insertAuthTestUser(t, testDB, userID, "myname", "password123", true, false)
	insertAuthTestUser(t, testDB, "user-chusr-004", "othername", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "othername",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_TooShort(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "user-chusr-005"
	insertAuthTestUser(t, testDB, userID, "validname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, testDB, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "ab",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}
