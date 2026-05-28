package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// newTestEcho returns an Echo instance wired with a real db and a ready migrator.
func newTestEcho(t *testing.T, db *bun.DB, cfg *config.Config) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", nil, nil, nil)
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
	return api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, rc)
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

// newFailingRiverClient builds a River client backed by a pool that has been
// closed, so every Insert call returns an error. Used to test 500 responses on
// River insert failures.
func newFailingRiverClient(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	pool.Close() // closed pool causes Insert to fail immediately
	return rc
}

// testCfg returns a minimal config suitable for api_test tests.
func testCfg() *config.Config {
	return &config.Config{
		DBEncryptionKey:   "test-db-encryption-key-32-bytes!!",
		SessionExpireDays: 30,
		Port:              8000,
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

// postJSONSession fires a POST request with a JSON body and a session cookie.
func postJSONSession(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(http.MethodPost, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

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

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["access_token"]; ok {
		t.Error("response must not contain access_token")
	}
	if resp["username"] != "alice" {
		t.Errorf("username = %q, want %q", resp["username"], "alice")
	}

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session_id cookie set")
	}

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

func TestHandleLogin_MalformedJSON(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, cfg, m, testDB, "", nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
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

	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", "user-020",
	).Scan(&count)
	if count != 0 {
		t.Errorf("session count = %d, want 0 after logout", count)
	}

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

// ─── GetMe tests ─────────────────────────────────────────────────────────────

func TestGetMe_Success(t *testing.T) {
	truncateAllTables(t)
	userID := "user-me-001"
	insertAuthTestUser(t, testDB, userID, "admin", "password123", true, true)

	e := echo.New()
	ah := api.NewAuthHandler(testDB, testCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
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
	ah := api.NewAuthHandler(testDB, testCfg())
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

func TestHandleChangePassword_WrongCurrentPassword(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-002", "pwduser2", "oldpass123", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-cp-002")

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "wrongpass",
		"new_password":     "newpass456",
	}, sessionID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_SamePassword(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-003", "pwduser3", "samepass1", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-cp-003")

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "samepass1",
		"new_password":     "samepass1",
	}, sessionID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_TooShort(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-004", "pwduser4", "oldpass123", true, false)
	sessionID := insertAuthTestSession(t, testDB, "user-cp-004")

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "short",
	}, sessionID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_InvalidatesOtherSessions(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "user-cp-005", "pwduser5", "oldpass123", true, false)
	currentSession := insertAuthTestSession(t, testDB, "user-cp-005")
	_ = insertAuthTestSession(t, testDB, "user-cp-005")

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

// ─── UpdateMe tests ────────────────────────────────────────────────────────

func TestHandleUpdateMe_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-update-me-001"
	insertAuthTestUser(t, testDB, userID, "testuser", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": map[string]any{"theme": "dark", "language": "en"},
	}, sessionID)

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
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-update-me-002"
	insertAuthTestUser(t, testDB, userID, "testuser2", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": []string{"not", "an", "object"},
	}, sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleUpdateMe_InvalidPreferences_Null(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-update-me-003"
	insertAuthTestUser(t, testDB, userID, "testuser3", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": nil,
	}, sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ─── CheckUsername tests ───────────────────────────────────────────────────

func TestHandleCheckUsername_Available(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chkusr-001"
	insertAuthTestUser(t, testDB, userID, "existinguser", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := getAuth(t, e, "/api/auth/username/check/newname", sessionID)

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
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chkusr-002"
	insertAuthTestUser(t, testDB, userID, "takenname", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := getAuth(t, e, "/api/auth/username/check/takenname", sessionID)

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
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chkusr-003"
	insertAuthTestUser(t, testDB, userID, "someuser", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := getAuth(t, e, "/api/auth/username/check/ab", sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

// ─── ChangeUsername tests ──────────────────────────────────────────────────

func TestHandleChangeUsername_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chusr-001"
	insertAuthTestUser(t, testDB, userID, "oldname", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "newname",
	}, sessionID)

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
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chusr-002"
	insertAuthTestUser(t, testDB, userID, "samename", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "samename",
	}, sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_AlreadyTaken(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chusr-003"
	insertAuthTestUser(t, testDB, userID, "myname", "password123", true, false)
	insertAuthTestUser(t, testDB, "user-chusr-004", "othername", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "othername",
	}, sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_TooShort(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	userID := "user-chusr-005"
	insertAuthTestUser(t, testDB, userID, "validname", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, userID)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "ab",
	}, sessionID)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}
