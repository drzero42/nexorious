package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
)

// ─── Admin users test helpers ────────────────────────────────────────────────

// setupAdminUser inserts an admin user, logs them in via the /api/auth/login
// route on `handler`, and returns (userID, accessToken).
func setupAdminUser(t *testing.T, db *bun.DB, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, suffix string) (string, string) {
	t.Helper()
	userID := "u-admin-" + suffix
	username := "adm-" + suffix
	insertAuthTestUser(t, db, userID, username, "password123", true, true)
	token := loginAndGetToken(t, handler, username, "password123")
	return userID, token
}

// setupRegularUser inserts a non-admin user and returns its access token.
func setupRegularUser(t *testing.T, db *bun.DB, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, suffix string) (string, string) {
	t.Helper()
	userID := "u-reg-" + suffix
	username := "reg-" + suffix
	insertAuthTestUser(t, db, userID, username, "password123", true, false)
	token := loginAndGetToken(t, handler, username, "password123")
	return userID, token
}

// ─── HandleCreate tests ──────────────────────────────────────────────────────

func TestAdminCreateUser_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-hp")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "newbie",
		"password": "secret123",
		"is_admin": false,
	}, adminTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["username"] != "newbie" {
		t.Errorf("username = %v, want newbie", resp["username"])
	}
	if resp["is_admin"] != false {
		t.Errorf("is_admin = %v, want false", resp["is_admin"])
	}
	if resp["is_active"] != true {
		t.Errorf("is_active = %v, want true", resp["is_active"])
	}
	if _, ok := resp["password_hash"]; ok {
		t.Error("response leaked password_hash")
	}
	if _, ok := resp["preferences"]; ok {
		t.Error("response leaked preferences")
	}

	// Verify a real row exists with a working bcrypt hash.
	var hash string
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT password_hash FROM users WHERE username = ?", "newbie",
	).Scan(&hash); err != nil {
		t.Fatalf("query hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret123")); err != nil {
		t.Errorf("bcrypt mismatch: %v", err)
	}
}

func TestAdminCreateUser_DuplicateUsername(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-dup")
	insertAuthTestUser(t, testDB, "u-existing-dup", "taken", "pw123456", true, false)

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "taken",
		"password": "secret123",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "username already taken" {
		t.Errorf("error = %q, want %q", resp["error"], "username already taken")
	}
}

func TestAdminCreateUser_ShortUsername(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-shortuser")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "ab",
		"password": "secret123",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "username must be at least 3 characters" {
		t.Errorf("error = %q, want %q", resp["error"], "username must be at least 3 characters")
	}
}

func TestAdminCreateUser_EmptyUsername(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-emptyuser")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "   ",
		"password": "secret123",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "username is required" {
		t.Errorf("error = %q, want %q", resp["error"], "username is required")
	}
}

func TestAdminCreateUser_ShortPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-shortpw")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "validuser",
		"password": "12345",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "password must be at least 8 characters" {
		t.Errorf("error = %q, want %q", resp["error"], "password must be at least 8 characters")
	}
}

func TestAdminCreateUser_EmptyPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "create-emptypw")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "validuser",
		"password": "",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "password is required" {
		t.Errorf("error = %q, want %q", resp["error"], "password is required")
	}
}

func TestAdminCreateUser_RequiresAuth(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	rec := postJSON(t, e, "/api/auth/admin/users", map[string]any{
		"username": "newbie",
		"password": "secret123",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body=%s", rec.Code, rec.Body)
	}
}

func TestAdminCreateUser_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "create-nonadm")

	rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
		"username": "newbie",
		"password": "secret123",
	}, regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", rec.Code, rec.Body)
	}
}

// ─── HandleList tests ────────────────────────────────────────────────────────

func TestAdminListUsers_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "list-hp")
	insertAuthTestUser(t, testDB, "u-list-other", "bob", "pw123456", true, false)

	rec := getAuth(t, e, "/api/auth/admin/users", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var users []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(users) < 2 {
		t.Errorf("got %d users, want >= 2", len(users))
	}
	for _, u := range users {
		if _, ok := u["password_hash"]; ok {
			t.Error("list response leaked password_hash")
		}
	}
}

func TestAdminListUsers_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "list-nonadm")

	rec := getAuth(t, e, "/api/auth/admin/users", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestAdminListUsers_RequiresAuth(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/admin/users", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// ─── HandleGet tests ─────────────────────────────────────────────────────────

func TestAdminGetUser_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "get-hp")
	insertAuthTestUser(t, testDB, "u-get-1", "targetuser", "pw123456", true, false)

	rec := getAuth(t, e, "/api/auth/admin/users/u-get-1", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["id"] != "u-get-1" {
		t.Errorf("id = %v, want u-get-1", resp["id"])
	}
	if resp["username"] != "targetuser" {
		t.Errorf("username = %v, want targetuser", resp["username"])
	}
	if _, ok := resp["password_hash"]; ok {
		t.Error("get response leaked password_hash")
	}
}

func TestAdminGetUser_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "get-nf")

	rec := getAuth(t, e, "/api/auth/admin/users/nonexistent-id", adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "user not found" {
		t.Errorf("error = %q, want %q", resp["error"], "user not found")
	}
}

func TestAdminGetUser_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "get-nonadm")

	rec := getAuth(t, e, "/api/auth/admin/users/any-id", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ─── HandleUpdate tests ──────────────────────────────────────────────────────

func TestAdminUpdateUser_ToggleAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-toggle")
	insertAuthTestUser(t, testDB, "u-upd-1", "targetuser", "pw123456", true, false)

	tru := true
	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-upd-1", map[string]any{
		"is_admin": tru,
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["is_admin"] != true {
		t.Errorf("is_admin = %v, want true", resp["is_admin"])
	}
}

func TestAdminUpdateUser_RenameUsername(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-rename")
	insertAuthTestUser(t, testDB, "u-upd-rn", "oldname", "pw123456", true, false)

	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-upd-rn", map[string]any{
		"username": "newname",
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["username"] != "newname" {
		t.Errorf("username = %v, want newname", resp["username"])
	}
}

func TestAdminUpdateUser_DuplicateUsername(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-dup")
	insertAuthTestUser(t, testDB, "u-upd-a", "alice", "pw123456", true, false)
	insertAuthTestUser(t, testDB, "u-upd-b", "bob", "pw123456", true, false)

	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-upd-a", map[string]any{
		"username": "bob",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "username already taken" {
		t.Errorf("error = %q, want %q", resp["error"], "username already taken")
	}
}

func TestAdminUpdateUser_DeactivateSelf_Rejected(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "upd-self-deact")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/"+adminID, map[string]any{
		"is_active": false,
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "Cannot deactivate your own account" {
		t.Errorf("error = %q, want %q", resp["error"], "Cannot deactivate your own account")
	}
}

func TestAdminUpdateUser_DemoteSelf_Rejected(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "upd-self-demote")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/"+adminID, map[string]any{
		"is_admin": false,
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "Cannot remove your own admin privileges" {
		t.Errorf("error = %q, want %q", resp["error"], "Cannot remove your own admin privileges")
	}
}

func TestAdminUpdateUser_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-404")

	tru := true
	rec := putJSONAuth(t, e, "/api/auth/admin/users/missing-id", map[string]any{
		"is_admin": tru,
	}, adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAdminUpdateUser_DeactivateInvalidatesSessions(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-deact-sess")

	insertAuthTestUser(t, testDB, "u-upd-deact", "target", "pw123456", true, false)
	targetTok := loginAndGetToken(t, e, "target", "pw123456")

	// Confirm target token works.
	rec := getAuth(t, e, "/api/auth/me", targetTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("precondition: target token should work; got %d", rec.Code)
	}

	// Admin deactivates the target user.
	rec = putJSONAuth(t, e, "/api/auth/admin/users/u-upd-deact", map[string]any{
		"is_active": false,
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("deactivate status = %d, want 200; body=%s", rec.Code, rec.Body)
	}

	// Sessions table should be empty for the target.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, "u-upd-deact",
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("session count = %d, want 0 after deactivate", count)
	}

	// Old token must not work any more.
	rec = getAuth(t, e, "/api/auth/me", targetTok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after deactivate, /me with old token status = %d, want 401", rec.Code)
	}
}

func TestAdminUpdateUser_PromoteDoesNotInvalidateSessions(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "upd-promote")

	insertAuthTestUser(t, testDB, "u-upd-promo", "promote-target", "pw123456", true, false)
	targetTok := loginAndGetToken(t, e, "promote-target", "pw123456")

	// Promote.
	tru := true
	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-upd-promo", map[string]any{
		"is_admin": tru,
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("promote status = %d, want 200; body=%s", rec.Code, rec.Body)
	}

	// Sessions for the target should still exist.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, "u-upd-promo",
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count == 0 {
		t.Errorf("expected sessions to be preserved after promotion, got 0")
	}

	// Session still works on an auth-protected route, and the new role is reflected.
	rec = getAuth(t, e, "/api/auth/me", targetTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("after promotion, /me status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var me map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if me["is_admin"] != true {
		t.Errorf("after promotion, /me is_admin = %v, want true", me["is_admin"])
	}
}

func TestAdminUpdateUser_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "upd-nonadm")

	tru := true
	rec := putJSONAuth(t, e, "/api/auth/admin/users/any-id", map[string]any{
		"is_admin": tru,
	}, regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ─── HandleResetPassword tests ───────────────────────────────────────────────

func TestAdminResetPassword_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "rp-hp")

	insertAuthTestUser(t, testDB, "u-rp-1", "rpuser", "oldpassword", true, false)
	oldTok := loginAndGetToken(t, e, "rpuser", "oldpassword")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-rp-1/password", map[string]any{
		"new_password": "newpassword456",
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["message"] != "Password reset successfully. User will need to log in again." {
		t.Errorf("message = %q", resp["message"])
	}

	// Old session must be gone — next /me call should 401.
	rec = getAuth(t, e, "/api/auth/me", oldTok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after reset, /me with old token status = %d, want 401", rec.Code)
	}

	// Logging in with the new password must succeed.
	newTok := loginAndGetToken(t, e, "rpuser", "newpassword456")
	if newTok == "" {
		t.Error("login with new password failed")
	}
}

func TestAdminResetPassword_ShortPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "rp-short")
	insertAuthTestUser(t, testDB, "u-rp-short", "rpshort", "pw123456", true, false)

	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-rp-short/password", map[string]any{
		"new_password": "12345",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "new password must be at least 8 characters" {
		t.Errorf("error = %q, want %q", resp["error"], "new password must be at least 8 characters")
	}
}

func TestAdminResetPassword_EmptyPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "rp-empty")
	insertAuthTestUser(t, testDB, "u-rp-empty", "rpempty", "pw123456", true, false)

	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-rp-empty/password", map[string]any{
		"new_password": "",
	}, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "new password is required" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestAdminResetPassword_UnknownUser(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "rp-nf")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/missing/password", map[string]any{
		"new_password": "newpassword456",
	}, adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAdminResetPassword_AdminResetsOwnPassword(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "rp-self")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/"+adminID+"/password", map[string]any{
		"new_password": "newadminpass",
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}

	// Admin's own session is invalidated by the reset.
	rec = getAuth(t, e, "/api/auth/me", adminTok)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after self-reset, /me status = %d, want 401 (admin's own session wiped)", rec.Code)
	}
}

func TestAdminResetPassword_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "rp-nonadm")

	rec := putJSONAuth(t, e, "/api/auth/admin/users/any-id/password", map[string]any{
		"new_password": "newpassword456",
	}, regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ─── HandleDeletionImpact tests ──────────────────────────────────────────────

func TestAdminDeletionImpact_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "di-hp")
	insertAuthTestUser(t, testDB, "u-di-1", "ditarget", "pw123456", true, false)

	seedDeletionImpactRows(t, testDB, "u-di-1")

	rec := getAuth(t, e, "/api/auth/admin/users/u-di-1/deletion-impact", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	expect := map[string]float64{
		"total_games":        2,
		"total_tags":         3,
		"total_import_jobs":  1,
		"total_export_jobs":  2,
		"total_sync_jobs":    1,
		"total_sync_configs": 1,
		"total_sessions":     1,
	}
	for k, want := range expect {
		got, ok := resp[k].(float64)
		if !ok {
			t.Errorf("%s missing or not numeric: %v", k, resp[k])
			continue
		}
		if got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	if resp["user_id"] != "u-di-1" {
		t.Errorf("user_id = %v", resp["user_id"])
	}
	if resp["username"] != "ditarget" {
		t.Errorf("username = %v", resp["username"])
	}
	if resp["warning"] != "This action cannot be undone. All data listed above will be permanently deleted." {
		t.Errorf("warning = %v", resp["warning"])
	}
}

func TestAdminDeletionImpact_Self_Rejected(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "di-self")

	rec := getAuth(t, e, "/api/auth/admin/users/"+adminID+"/deletion-impact", adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "Cannot delete your own account" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestAdminDeletionImpact_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "di-nf")

	rec := getAuth(t, e, "/api/auth/admin/users/missing-id/deletion-impact", adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAdminDeletionImpact_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "di-nonadm")

	rec := getAuth(t, e, "/api/auth/admin/users/any-id/deletion-impact", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ─── HandleDelete tests ──────────────────────────────────────────────────────

func TestAdminDeleteUser_HappyPath(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "del-hp")
	insertAuthTestUser(t, testDB, "u-del-1", "deltarget", "pw123456", true, false)

	rec := deleteAuth(t, e, "/api/auth/admin/users/u-del-1", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["message"] != "User and all associated data deleted successfully" {
		t.Errorf("message = %q", resp["message"])
	}

	// Row gone.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM users WHERE id = ?`, "u-del-1",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("user count = %d, want 0", count)
	}
}

func TestAdminDeleteUser_CascadesToRelatedTables(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "del-cascade")
	insertAuthTestUser(t, testDB, "u-del-cascade", "cascadetarget", "pw123456", true, false)
	seedDeletionImpactRows(t, testDB, "u-del-cascade")

	// Sanity-check seeding before delete.
	for _, table := range []string{"user_games", "tags", "jobs", "user_sessions", "user_sync_configs", "external_games", "job_items"} {
		var c int
		if err := testDB.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", "u-del-cascade",
		).Scan(&c); err != nil {
			t.Fatalf("pre-count %s: %v", table, err)
		}
		if c == 0 {
			t.Fatalf("pre-condition: %s should have rows for u-del-cascade", table)
		}
	}

	rec := deleteAuth(t, e, "/api/auth/admin/users/u-del-cascade", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}

	// All per-user tables must be empty afterwards.
	for _, table := range []string{"user_games", "tags", "jobs", "user_sessions", "user_sync_configs", "external_games", "job_items"} {
		var c int
		if err := testDB.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", "u-del-cascade",
		).Scan(&c); err != nil {
			t.Fatalf("post-count %s: %v", table, err)
		}
		if c != 0 {
			t.Errorf("after delete, %s rows for u-del-cascade = %d, want 0", table, c)
		}
	}
}

func TestAdminDeleteUser_Self_Rejected(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "del-self")

	rec := deleteAuth(t, e, "/api/auth/admin/users/"+adminID, adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "Cannot delete your own account" {
		t.Errorf("error = %q", resp["error"])
	}

	// Self row still exists.
	var count int
	_ = testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM users WHERE id = ?`, adminID,
	).Scan(&count)
	if count != 1 {
		t.Errorf("admin should still exist after self-delete attempt, count = %d", count)
	}
}

func TestAdminDeleteUser_NotFound(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "del-nf")

	rec := deleteAuth(t, e, "/api/auth/admin/users/missing", adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAdminDeleteUser_RequiresAdmin(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "del-nonadm")

	rec := deleteAuth(t, e, "/api/auth/admin/users/any-id", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ─── Route ordering test ─────────────────────────────────────────────────────

func TestAdminUsers_RouteOrdering_PasswordVsID(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "ro")
	insertAuthTestUser(t, testDB, "u-ro-1", "rotarget", "pw123456", true, false)

	// PUT /:id/password must not collide with PUT /:id — it should still reset
	// the password rather than running a partial-update.
	rec := putJSONAuth(t, e, "/api/auth/admin/users/u-ro-1/password", map[string]any{
		"new_password": "freshpassword",
	}, adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (route ordering bug?); body=%s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["message"] != "Password reset successfully. User will need to log in again." {
		t.Errorf("message = %q (route ordering wrong?)", resp["message"])
	}

	// GET /:id/deletion-impact must hit the deletion-impact handler, not GET /:id.
	rec = getAuth(t, e, "/api/auth/admin/users/u-ro-1/deletion-impact", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var di map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &di)
	if _, ok := di["total_games"]; !ok {
		t.Error("deletion-impact response missing total_games — likely matched GET /:id")
	}
}

// ─── Seeding helper ──────────────────────────────────────────────────────────

// seedDeletionImpactRows inserts the dependent rows used by the deletion-impact
// and cascade-delete tests. Counts:
//
//	user_games        : 2
//	tags              : 3
//	jobs (import)     : 1
//	jobs (export)     : 2
//	jobs (sync)       : 1
//	user_sync_configs : 1
//	user_sessions     : 1
//	external_games    : 1 (cascade-only)
//	job_items         : 1 (cascade-only, attached to the import job)
func seedDeletionImpactRows(t *testing.T, db *bun.DB, userID string) {
	t.Helper()
	ctx := context.Background()

	// Insert a game row that user_games can reference.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO games (id, title) VALUES (10001, 'TestGame1'), (10002, 'TestGame2')
		 ON CONFLICT DO NOTHING`,
	); err != nil {
		t.Fatalf("seed games: %v", err)
	}

	// user_games (2)
	for i, gameID := range []int{10001, 10002} {
		ugID := userID + "-ug-" + intToStr(i)
		if _, err := db.ExecContext(ctx,
			`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
			ugID, userID, gameID,
		); err != nil {
			t.Fatalf("seed user_games: %v", err)
		}
	}

	// tags (3)
	for i := range 3 {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tags (id, user_id, name) VALUES (?, ?, ?)`,
			userID+"-tag-"+intToStr(i), userID, "tag"+intToStr(i),
		); err != nil {
			t.Fatalf("seed tags: %v", err)
		}
	}

	// jobs: 1 import, 2 export, 1 sync
	importJobID := userID + "-job-import"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source) VALUES (?, ?, 'import', 'nexorious')`,
		importJobID, userID,
	); err != nil {
		t.Fatalf("seed import job: %v", err)
	}
	for i := range 2 {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO jobs (id, user_id, job_type, source) VALUES (?, ?, 'export', 'csv')`,
			userID+"-job-export-"+intToStr(i), userID,
		); err != nil {
			t.Fatalf("seed export job: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source) VALUES (?, ?, 'sync', 'steam')`,
		userID+"-job-sync", userID,
	); err != nil {
		t.Fatalf("seed sync job: %v", err)
	}

	// job_items (1) attached to the import job.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title)
		 VALUES (?, ?, ?, 'k1', 'TestItem')`,
		userID+"-ji-1", importJobID, userID,
	); err != nil {
		t.Fatalf("seed job_items: %v", err)
	}

	// user_sync_configs (1)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO user_sync_configs (id, user_id, storefront) VALUES (?, ?, 'steam')`,
		userID+"-cfg", userID,
	); err != nil {
		t.Fatalf("seed user_sync_configs: %v", err)
	}

	// user_sessions (1) — use a unique session hash so it doesn't collide with
	// any session issued by setupAdminUser/loginAndGetToken in this test.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES (gen_random_uuid()::text, ?, ?, now() + interval '30 days')`,
		userID, auth.HashToken(userID+"-seed-session"),
	); err != nil {
		t.Fatalf("seed user_sessions: %v", err)
	}

	// external_games (1)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title)
		 VALUES (?, ?, 'steam', 'ext-1', 'External')`,
		userID+"-eg", userID,
	); err != nil {
		t.Fatalf("seed external_games: %v", err)
	}
}

// intToStr is a tiny zero-dep stringifier for unique ID suffixes in seeds.
func intToStr(i int) string {
	switch i {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	case 4:
		return "4"
	}
	// fallback for larger values via strconv-like manual formatting
	digits := ""
	for i > 0 {
		digits = string('0'+byte(i%10)) + digits
		i /= 10
	}
	if digits == "" {
		return "0"
	}
	return digits
}
