# Auth Profile Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the four remaining Phase 2 auth endpoints: update preferences, change password, check username availability, and change username.

**Architecture:** All handlers are added to the existing `AuthHandler` in `internal/api/auth.go`. They use raw Bun SQL (not ORM models) matching the established auth pattern. Tests use the existing testcontainers infrastructure in `auth_test.go`.

**Tech Stack:** Go, Echo v5, Bun (raw SQL), bcrypt, testcontainers-go

---

### Task 0: Create Feature Branch

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feat/auth-profile-endpoints
```

---

### Task 1: Test Helpers — Add `putJSONAuth` and `getAuth`

**Files:**
- Modify: `internal/api/auth_test.go`

- [ ] **Step 1: Add `putJSONAuth` helper**

Add after the existing `postJSONAuth` function (around line 160):

```go
// putJSONAuth fires a PUT with a JSON body and a Bearer authorization header.
func putJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// getAuth fires a GET with a Bearer authorization header.
func getAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/api/...`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/api/auth_test.go
git commit -m "test: add putJSONAuth and getAuth test helpers"
```

---

### Task 2: `PUT /api/auth/me` — Update Preferences

**Files:**
- Modify: `internal/api/auth.go` (add types + handler)
- Modify: `internal/api/router.go` (register route)
- Modify: `internal/api/auth_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/auth_test.go`:

```go
// ─── UpdateMe tests ────────────────────────────────────────────────────────

func TestHandleUpdateMe_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-update-me-001"
	insertAuthTestUser(t, db, userID, "testuser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-update-me-002"
	insertAuthTestUser(t, db, userID, "testuser2", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": []string{"not", "an", "object"},
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleUpdateMe_InvalidPreferences_Null(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-update-me-003"
	insertAuthTestUser(t, db, userID, "testuser3", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/me", map[string]any{
		"preferences": nil,
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestHandleUpdateMe -v`
Expected: compilation error (HandleUpdateMe not defined)

- [ ] **Step 3: Add request type and handler**

Add to `internal/api/auth.go`:

```go
type updateMeRequest struct {
	Preferences json.RawMessage `json:"preferences"`
}

type messageResponse struct {
	Message string `json:"message"`
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

	// Validate preferences is a JSON object (not null, array, or scalar).
	if req.Preferences == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	var obj map[string]any
	if err := json.Unmarshal(req.Preferences, &obj); err != nil || obj == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}

	_, err := h.db.ExecContext(context.Background(),
		`UPDATE users SET preferences = ?, updated_at = NOW() WHERE id = ?`,
		string(req.Preferences), userID,
	)
	if err != nil {
		slog.Error("update me: update preferences", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Re-query and return the full profile.
	var resp meResponse
	var prefs []byte
	err = h.db.QueryRowContext(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		slog.Error("update me: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}

	return c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add to the `authGroup` block (after `authGroup.GET("/me", ah.HandleGetMe)`):

```go
authGroup.PUT("/me", ah.HandleUpdateMe)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestHandleUpdateMe -v`
Expected: all 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth.go internal/api/router.go internal/api/auth_test.go
git commit -m "feat: add PUT /api/auth/me endpoint for updating preferences"
```

---

### Task 3: `PUT /api/auth/change-password` — Change Password

**Files:**
- Modify: `internal/api/auth.go` (add type + handler)
- Modify: `internal/api/router.go` (register route)
- Modify: `internal/api/auth_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/auth_test.go`:

```go
// ─── ChangePassword tests ──────────────────────────────────────────────────

func TestHandleChangePassword_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chpwd-001"
	insertAuthTestUser(t, db, userID, "pwduser", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "newpass456",
	}, accessToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
	}

	// Verify new password works by checking bcrypt.
	var hash string
	err = db.QueryRowContext(context.Background(),
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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chpwd-002"
	insertAuthTestUser(t, db, userID, "pwduser2", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "wrongpass",
		"new_password":     "newpass456",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_SamePassword(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chpwd-003"
	insertAuthTestUser(t, db, userID, "pwduser3", "samepass1", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "samepass1",
		"new_password":     "samepass1",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_TooShort(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chpwd-004"
	insertAuthTestUser(t, db, userID, "pwduser4", "oldpass123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/change-password", map[string]any{
		"current_password": "oldpass123",
		"new_password":     "short",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangePassword_InvalidatesOtherSessions(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chpwd-005"
	insertAuthTestUser(t, db, userID, "pwduser5", "oldpass123", true, false)

	// Create the "current" session.
	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	// Create an "other" session (simulating another device).
	otherToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken other: %v", err)
	}
	insertAuthTestSession(t, db, userID, otherToken, "", 30)

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
	err = db.QueryRowContext(context.Background(),
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
	err = db.QueryRowContext(context.Background(),
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestHandleChangePassword -v`
Expected: compilation error (HandleChangePassword not defined)

- [ ] **Step 3: Add request type and handler**

Add to `internal/api/auth.go`:

```go
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

	// Fetch the stored password hash.
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

	// Verify current password.
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.CurrentPassword)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is incorrect")
	}

	// Hash and store new password.
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		slog.Error("change password: bcrypt", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	_, err = h.db.ExecContext(context.Background(),
		`UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?`,
		string(newHash), userID,
	)
	if err != nil {
		slog.Error("change password: update hash", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Invalidate other sessions (keep the current one).
	authHeader := c.Request().Header.Get("Authorization")
	currentTokenHash := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		currentTokenHash = auth.HashToken(authHeader[7:])
	}

	_, err = h.db.ExecContext(context.Background(),
		`DELETE FROM user_sessions WHERE user_id = ? AND token_hash != ?`,
		userID, currentTokenHash,
	)
	if err != nil {
		slog.Error("change password: invalidate sessions", "err", err)
		// Non-fatal: password is already changed.
	}

	return c.JSON(http.StatusOK, messageResponse{Message: "Password changed successfully."})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add to the `authGroup` block:

```go
authGroup.PUT("/change-password", ah.HandleChangePassword)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestHandleChangePassword -v`
Expected: all 5 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth.go internal/api/router.go internal/api/auth_test.go
git commit -m "feat: add PUT /api/auth/change-password endpoint"
```

---

### Task 4: `GET /api/auth/username/check/:username` — Check Username Availability

**Files:**
- Modify: `internal/api/auth.go` (add type + handler)
- Modify: `internal/api/router.go` (register route)
- Modify: `internal/api/auth_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/auth_test.go`:

```go
// ─── CheckUsername tests ───────────────────────────────────────────────────

func TestHandleCheckUsername_Available(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chkusr-001"
	insertAuthTestUser(t, db, userID, "existinguser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chkusr-002"
	insertAuthTestUser(t, db, userID, "takenname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chkusr-003"
	insertAuthTestUser(t, db, userID, "someuser", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := getAuth(t, e, "/api/auth/username/check/ab", accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestHandleCheckUsername -v`
Expected: compilation error (HandleCheckUsername not defined)

- [ ] **Step 3: Add response type and handler**

Add to `internal/api/auth.go`:

```go
type usernameAvailabilityResponse struct {
	Available bool   `json:"available"`
	Username  string `json:"username"`
}

// HandleCheckUsername handles GET /api/auth/username/check/:username.
func (h *AuthHandler) HandleCheckUsername(c *echo.Context) error {
	username := c.PathParam("username")

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

	return c.JSON(http.StatusOK, usernameAvailabilityResponse{
		Available: available,
		Username:  username,
	})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add to the `authGroup` block:

```go
authGroup.GET("/username/check/:username", ah.HandleCheckUsername)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestHandleCheckUsername -v`
Expected: all 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth.go internal/api/router.go internal/api/auth_test.go
git commit -m "feat: add GET /api/auth/username/check/:username endpoint"
```

---

### Task 5: `PUT /api/auth/username` — Change Username

**Files:**
- Modify: `internal/api/auth.go` (add type + handler)
- Modify: `internal/api/router.go` (register route)
- Modify: `internal/api/auth_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/auth_test.go`:

```go
// ─── ChangeUsername tests ──────────────────────────────────────────────────

func TestHandleChangeUsername_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chusr-001"
	insertAuthTestUser(t, db, userID, "oldname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

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
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chusr-002"
	insertAuthTestUser(t, db, userID, "samename", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "samename",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_AlreadyTaken(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chusr-003"
	insertAuthTestUser(t, db, userID, "myname", "password123", true, false)
	insertAuthTestUser(t, db, "user-chusr-004", "othername", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "othername",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}

func TestHandleChangeUsername_TooShort(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID := "user-chusr-005"
	insertAuthTestUser(t, db, userID, "validname", "password123", true, false)

	accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	insertAuthTestSession(t, db, userID, accessToken, "", 30)

	rec := putJSONAuth(t, e, "/api/auth/username", map[string]any{
		"new_username": "ab",
	}, accessToken)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestHandleChangeUsername -v`
Expected: compilation error (HandleChangeUsername not defined)

- [ ] **Step 3: Add request type and handler**

Add to `internal/api/auth.go`:

```go
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

	// Check if same as current.
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

	// Check availability.
	var exists int
	err = h.db.QueryRowContext(context.Background(),
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", req.NewUsername,
	).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("change username: check availability", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if err == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "username already taken")
	}

	// Update username.
	_, err = h.db.ExecContext(context.Background(),
		`UPDATE users SET username = ?, updated_at = NOW() WHERE id = ?`,
		req.NewUsername, userID,
	)
	if err != nil {
		slog.Error("change username: update", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Re-query and return profile.
	var resp meResponse
	var prefs []byte
	err = h.db.QueryRowContext(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		slog.Error("change username: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}

	return c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add to the `authGroup` block:

```go
authGroup.PUT("/username", ah.HandleChangeUsername)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestHandleChangeUsername -v`
Expected: all 4 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth.go internal/api/router.go internal/api/auth_test.go
git commit -m "feat: add PUT /api/auth/username endpoint"
```

---

### Task 6: Slumber Collection Entries

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add auth profile requests to slumber.yaml**

Add the following entries inside the `auth.requests` section (after the existing `me` entry):

```yaml
      update_me:
        name: Update Preferences
        method: PUT
        url: "{{base_url}}/api/auth/me"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            preferences:
              theme: dark
              language: en

      change_password:
        name: Change Password
        method: PUT
        url: "{{base_url}}/api/auth/change-password"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            current_password: "{{password}}"
            new_password: newpass1234

      check_username:
        name: Check Username
        method: GET
        url: "{{base_url}}/api/auth/username/check/newname"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      change_username:
        name: Change Username
        method: PUT
        url: "{{base_url}}/api/auth/username"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            new_username: newadmin
```

- [ ] **Step 2: Verify collection loads**

Run: `slumber show collection`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "chore: add auth profile endpoint entries to slumber collection"
```

---

### Task 7: Full Test Suite + Lint

- [ ] **Step 1: Run all Go tests**

Run: `go test ./...`
Expected: all tests PASS

- [ ] **Step 2: Run linter**

Run: `golangci-lint run`
Expected: no errors

- [ ] **Step 3: Fix any issues and commit if needed**

---

### Task 8: Push and Create Pull Request

- [ ] **Step 1: Push the feature branch**

```bash
git push -u origin feat/auth-profile-endpoints
```

- [ ] **Step 2: Create pull request**

```bash
gh pr create --title "feat: auth profile endpoints (Phase 2)" --body "$(cat <<'EOF'
## Summary
- Add `PUT /api/auth/me` — update user preferences
- Add `PUT /api/auth/change-password` — change password, invalidates other sessions
- Add `GET /api/auth/username/check/:username` — username availability check
- Add `PUT /api/auth/username` — change username

## Test plan
- [ ] `go test ./internal/api/... -run TestHandleUpdateMe -v`
- [ ] `go test ./internal/api/... -run TestHandleChangePassword -v`
- [ ] `go test ./internal/api/... -run TestHandleCheckUsername -v`
- [ ] `go test ./internal/api/... -run TestHandleChangeUsername -v`
- [ ] `golangci-lint run`
- [ ] `slumber show collection`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review: Spec Coverage

| Spec requirement | Task |
|-----------------|------|
| `PUT /api/auth/me` — update preferences | Task 2 |
| `PUT /api/auth/change-password` — change password, invalidate other sessions | Task 3 |
| `GET /api/auth/username/check/:username` — availability check | Task 4 |
| `PUT /api/auth/username` — change username | Task 5 |
| Route registration in `authGroup` | Tasks 2–5 |
| Tests for all endpoints | Tasks 2–5 |
| Slumber collection entries | Task 6 |
| Lint clean | Task 7 |
