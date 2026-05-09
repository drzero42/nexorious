# IGDB Optional Credentials Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` optional so the app starts without them, gating only IGDB-dependent endpoints.

**Architecture:** Remove `required` tags from IGDB config fields. Add credential validation in `main.go` with auth-vs-network error distinction. The IGDB client already handles the unconfigured state internally (`configured: false` + `ErrIGDBNotConfigured`), so handler-level gating is already in place — we just need to stop crashing on startup and add health reporting.

**Tech Stack:** Go, Echo v5, caarlos0/env, stdlib testing

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/config/config.go` | Modify | Remove `required` tag from IGDB fields |
| `internal/config/config_test.go` | Modify | Update tests for optional IGDB vars |
| `internal/services/igdb/errors.go` | Create | Extract `IsAuthError` helper (new file for clarity) |
| `internal/services/igdb/igdb.go` | Modify | Add `ValidateCredentials` method, export `Configured()` |
| `internal/services/igdb/igdb_test.go` | Modify | Add tests for `ValidateCredentials` and `IsAuthError` |
| `cmd/nexorious/main.go` | Modify | Add credential validation after config load |
| `internal/api/router.go` | Modify | Add `igdb_configured` to `/health` response |
| `internal/api/router_test.go` | Modify | Add health tests for `igdb_configured` field |
| `internal/api/games_test.go` | Modify | Add tests for IGDB-not-configured handler path |

---

### Task 1: Make IGDB Config Fields Optional

**Files:**
- Modify: `internal/config/config.go:46-47`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test — config loads without IGDB vars**

In `internal/config/config_test.go`, add a test that loads config with only `SECRET_KEY` set (no IGDB vars):

```go
func TestLoad_SucceedsWithoutIGDBVars(t *testing.T) {
	t.Setenv("SECRET_KEY", "testsecretkey")
	// Explicitly unset IGDB vars to ensure they're not inherited.
	os.Unsetenv("IGDB_CLIENT_ID")
	os.Unsetenv("IGDB_CLIENT_SECRET")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() should succeed without IGDB vars, got error: %v", err)
	}
	if cfg.IGDBClientID != "" {
		t.Errorf("IGDBClientID = %q; want empty string", cfg.IGDBClientID)
	}
	if cfg.IGDBClientSecret != "" {
		t.Errorf("IGDBClientSecret = %q; want empty string", cfg.IGDBClientSecret)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -run TestLoad_SucceedsWithoutIGDBVars -v`
Expected: FAIL — `config.Load()` returns an error because `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` are `required`.

- [ ] **Step 3: Remove `required` tag from IGDB fields**

In `internal/config/config.go`, change lines 46–47:

```go
// Before
IGDBClientID          string  `env:"IGDB_CLIENT_ID,required"`
IGDBClientSecret      string  `env:"IGDB_CLIENT_SECRET,required"`

// After
IGDBClientID          string  `env:"IGDB_CLIENT_ID"`
IGDBClientSecret      string  `env:"IGDB_CLIENT_SECRET"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -run TestLoad_SucceedsWithoutIGDBVars -v`
Expected: PASS

- [ ] **Step 5: Update `TestLoad_RequiredFieldsMissing` — remove IGDB vars from required set**

The existing test unsets `SECRET_KEY`, `IGDB_CLIENT_ID`, and `IGDB_CLIENT_SECRET` and expects `Load()` to fail. Now only `SECRET_KEY` is required. Update the test:

```go
func TestLoad_RequiredFieldsMissing(t *testing.T) {
	// Only SECRET_KEY is required now — IGDB vars are optional.
	saved := os.Getenv("SECRET_KEY")
	os.Unsetenv("SECRET_KEY") //nolint:errcheck
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("SECRET_KEY", saved) //nolint:errcheck
		} else {
			os.Unsetenv("SECRET_KEY") //nolint:errcheck
		}
	})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when SECRET_KEY is missing, got nil")
	}
}
```

- [ ] **Step 6: Run all config tests**

Run: `go test ./internal/config/... -v`
Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: make IGDB_CLIENT_ID and IGDB_CLIENT_SECRET optional in config"
```

---

### Task 2: Add `IsAuthError` Helper and `ValidateCredentials` Method

**Files:**
- Create: `internal/services/igdb/errors.go`
- Modify: `internal/services/igdb/models.go` (move sentinel errors to errors.go)
- Modify: `internal/services/igdb/igdb.go` (add `ValidateCredentials`, `Configured`)
- Modify: `internal/services/igdb/auth.go` (change `fetchToken` to wrap HTTP status in error)

- [ ] **Step 1: Write the failing test — `IsAuthError` distinguishes auth from network errors**

In `internal/services/igdb/igdb_test.go`, add:

```go
func TestIsAuthError_TrueForAuthFailure(t *testing.T) {
	err := fmt.Errorf("%w: Twitch returned status 403", igdb.ErrTwitchAuth)
	if !igdb.IsAuthError(err) {
		t.Error("IsAuthError should return true for ErrTwitchAuth wrapping an HTTP status")
	}
}

func TestIsAuthError_FalseForNetworkError(t *testing.T) {
	err := fmt.Errorf("connection refused")
	if igdb.IsAuthError(err) {
		t.Error("IsAuthError should return false for non-auth errors")
	}
}

func TestIsAuthError_FalseForNil(t *testing.T) {
	if igdb.IsAuthError(nil) {
		t.Error("IsAuthError should return false for nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/igdb/... -run TestIsAuthError -v`
Expected: FAIL — `IsAuthError` is not defined.

- [ ] **Step 3: Create `internal/services/igdb/errors.go` with `IsAuthError`**

Move the sentinel errors from `models.go` to `errors.go` and add the helper:

```go
package igdb

import "errors"

// Sentinel errors for IGDB service operations.
var (
	ErrIGDBNotConfigured = errors.New("IGDB credentials not configured")
	ErrGameNotFound      = errors.New("game not found in IGDB")
	ErrTwitchAuth        = errors.New("Twitch authentication failed")
)

// IsAuthError reports whether err is an authentication failure (invalid credentials),
// as opposed to a transient network or server error. It returns true when the error
// wraps ErrTwitchAuth — which fetchToken produces for HTTP 4xx responses from Twitch.
func IsAuthError(err error) bool {
	return err != nil && errors.Is(err, ErrTwitchAuth)
}
```

Remove the three sentinel `var` lines from `internal/services/igdb/models.go` (the `import "errors"` line too, if no longer needed).

- [ ] **Step 4: Run `IsAuthError` tests**

Run: `go test ./internal/services/igdb/... -run TestIsAuthError -v`
Expected: PASS

- [ ] **Step 5: Distinguish auth errors from network errors in `fetchToken`**

Currently `auth.go:fetchToken` wraps **all** errors with `ErrTwitchAuth`, including network failures. We need network errors to NOT be `ErrTwitchAuth` so `IsAuthError` can distinguish them. Change `fetchToken` in `internal/services/igdb/auth.go`:

```go
func (am *AuthManager) fetchToken(ctx context.Context) error {
	data := url.Values{
		"client_id":     {am.clientID},
		"client_secret": {am.clientSecret},
		"grant_type":    {"client_credentials"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, am.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		// Network/DNS/timeout — NOT an auth error.
		return fmt.Errorf("Twitch token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// HTTP error from Twitch — this IS an auth error (bad credentials, forbidden, etc.)
		return fmt.Errorf("%w: Twitch returned status %d", ErrTwitchAuth, resp.StatusCode)
	}

	var tokenResp twitchTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode Twitch token response: %w", err)
	}

	am.accessToken = tokenResp.AccessToken
	am.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return nil
}
```

Key change: network errors (the `am.httpClient.Do(req)` failure path) no longer wrap `ErrTwitchAuth`. Only HTTP status errors do.

- [ ] **Step 6: Write the failing test — `ValidateCredentials`**

In `internal/services/igdb/igdb_test.go`, add:

```go
func TestValidateCredentials_NotConfigured(t *testing.T) {
	client := igdb.NewClient(&config.Config{}) // no IGDB creds
	err := client.ValidateCredentials(context.Background())
	if !errors.Is(err, igdb.ErrIGDBNotConfigured) {
		t.Errorf("expected ErrIGDBNotConfigured, got %v", err)
	}
}

func TestValidateCredentials_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer ts.Close()

	cfg := &config.Config{
		IGDBClientID:          "test-id",
		IGDBClientSecret:      "test-secret",
		IGDBRequestsPerSecond: 4.0,
		IGDBBurstCapacity:     8,
	}
	client := igdb.NewClientWithTokenURL(cfg, ts.URL)
	err := client.ValidateCredentials(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateCredentials_AuthError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	cfg := &config.Config{
		IGDBClientID:          "bad-id",
		IGDBClientSecret:      "bad-secret",
		IGDBRequestsPerSecond: 4.0,
		IGDBBurstCapacity:     8,
	}
	client := igdb.NewClientWithTokenURL(cfg, ts.URL)
	err := client.ValidateCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
	if !igdb.IsAuthError(err) {
		t.Errorf("expected auth error, got %v", err)
	}
}
```

- [ ] **Step 7: Run tests to verify they fail**

Run: `go test ./internal/services/igdb/... -run TestValidateCredentials -v`
Expected: FAIL — `ValidateCredentials` and `NewClientWithTokenURL` don't exist.

- [ ] **Step 8: Add `ValidateCredentials`, `Configured`, and `NewClientWithTokenURL` to `igdb.go`**

In `internal/services/igdb/igdb.go`, add:

```go
// Configured reports whether the client has IGDB credentials.
func (c *Client) Configured() bool {
	return c.configured
}

// ValidateCredentials attempts a Twitch OAuth token fetch to verify that the
// configured client ID and secret are valid. Returns nil on success.
// Returns ErrIGDBNotConfigured if the client has no credentials.
func (c *Client) ValidateCredentials(ctx context.Context) error {
	if !c.configured {
		return ErrIGDBNotConfigured
	}
	_, err := c.auth.GetAccessToken(ctx)
	return err
}

// NewClientWithTokenURL creates a client with a custom Twitch token URL (for testing).
func NewClientWithTokenURL(cfg *config.Config, tokenURL string) *Client {
	c := NewClient(cfg)
	if c.auth != nil {
		c.auth.tokenURL = tokenURL
	}
	return c
}
```

- [ ] **Step 9: Run tests**

Run: `go test ./internal/services/igdb/... -run "TestIsAuthError|TestValidateCredentials" -v`
Expected: All PASS.

- [ ] **Step 10: Run all IGDB tests to check for regressions**

Run: `go test ./internal/services/igdb/... -v`
Expected: All pass.

- [ ] **Step 11: Commit**

```bash
git add internal/services/igdb/errors.go internal/services/igdb/models.go internal/services/igdb/igdb.go internal/services/igdb/auth.go internal/services/igdb/igdb_test.go
git commit -m "feat: add IsAuthError helper and ValidateCredentials method to IGDB client"
```

---

### Task 3: Add IGDB Credential Validation in `main.go`

**Files:**
- Modify: `cmd/nexorious/main.go:176-177` (the IGDB client creation block)

- [ ] **Step 1: Replace unconditional `igdb.NewClient` with validation logic**

In `cmd/nexorious/main.go`, replace the current line:

```go
igdbClient := igdb.NewClient(cfg)
```

With the validation block:

```go
// -------------------------------------------------------------------------
// IGDB client (optional)
// -------------------------------------------------------------------------
igdbClient := igdb.NewClient(cfg)

if !igdbClient.Configured() {
	slog.Warn("IGDB credentials not configured — game search, import, and metadata features will be unavailable")
} else {
	validateCtx, validateCancel := context.WithTimeout(ctx, 10*time.Second)
	err := igdbClient.ValidateCredentials(validateCtx)
	validateCancel()
	if err != nil {
		if igdb.IsAuthError(err) {
			slog.Warn("IGDB credentials are invalid — disabling IGDB features", "err", err)
			igdbClient = igdb.NewClient(&config.Config{}) // unconfigured client
		} else {
			slog.Warn("IGDB credential probe failed (network/transient) — IGDB client kept", "err", err)
		}
	} else {
		slog.Info("IGDB credentials validated successfully")
	}
}
```

Note: On auth failure we create a new unconfigured client instead of setting `nil` — this matches the existing codebase pattern where handlers call `h.igdb.SearchGames(...)` without nil checks, relying on the `configured` bool to return `ErrIGDBNotConfigured`.

- [ ] **Step 2: Verify the build compiles**

Run: `go build ./cmd/nexorious/...`
Expected: Compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat: add IGDB credential validation at startup"
```

---

### Task 4: Add `igdb_configured` to `/health` Response

**Files:**
- Modify: `internal/api/router.go:119-125` (health handler)
- Modify: `internal/api/router_test.go`

- [ ] **Step 1: Write the failing test — health reports `igdb_configured: true`**

In `internal/api/router_test.go`, add:

```go
func TestHealth_ReportsIGDBConfiguredTrue(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.IGDBClientID = "test-id"
	cfg.IGDBClientSecret = "test-secret"
	igdbClient := igdb.NewClient(cfg)
	e := api.New(cfg, migrator, nil, "", igdbClient)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	igdbConfigured, ok := body["igdb_configured"]
	if !ok {
		t.Fatal("response missing igdb_configured field")
	}
	if igdbConfigured != true {
		t.Errorf("igdb_configured = %v; want true", igdbConfigured)
	}
}

func TestHealth_ReportsIGDBConfiguredFalse(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testCfg(), migrator, nil, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	igdbConfigured, ok := body["igdb_configured"]
	if !ok {
		t.Fatal("response missing igdb_configured field")
	}
	if igdbConfigured != false {
		t.Errorf("igdb_configured = %v; want false", igdbConfigured)
	}
}
```

You'll need to add `"github.com/drzero42/nexorious-go/internal/services/igdb"` to the imports of `router_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestHealth_ReportsIGDB -v`
Expected: FAIL — health response doesn't include `igdb_configured`.

- [ ] **Step 3: Update the health handler in `router.go`**

Replace the health handler in `registerRoutes` (around line 119):

```go
// Health check — bypassed by all gates
igdbConfigured := igdbClient != nil && igdbClient.Configured()
e.GET("/health", func(c *echo.Context) error {
	state := migrator.State()
	status := "ok"
	if state != migrate.AppStateReady {
		status = state.String()
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status":          status,
		"igdb_configured": igdbConfigured,
	})
})
```

- [ ] **Step 4: Run health tests**

Run: `go test ./internal/api/... -run TestHealth -v`
Expected: All pass (including the existing ones — they pass `nil` for igdbClient so `igdb_configured` is `false`).

- [ ] **Step 5: Update existing health tests to expect the new field**

The existing `TestHealth_OKWhenReady` etc. tests decode into `map[string]string`. Since the response now includes a bool (`igdb_configured`), they need to decode into `map[string]any` or just ignore the extra field. Update them to use `map[string]any`:

```go
// In TestHealth_OKWhenReady, TestHealth_OKWhenSetupPending,
// TestHealth_DBUnavailableReturns200, TestHealth_NeedsMigrationReturns200:
// Change:
var body map[string]string
// To:
var body map[string]any
// And change status checks from:
if body["status"] != "ok" {
// To:
if body["status"] != "ok" {
```

The string comparison still works because `json.Decoder` decodes JSON strings as `string` even into `any`.

- [ ] **Step 6: Run all health tests**

Run: `go test ./internal/api/... -run TestHealth -v`
Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go
git commit -m "feat: add igdb_configured field to /health response"
```

---

### Task 5: Add Games Handler Tests for IGDB-Not-Configured Path

**Files:**
- Modify: `internal/api/games_test.go`

- [ ] **Step 1: Write the tests**

In `internal/api/games_test.go`, add tests that exercise the IGDB handlers when the client is unconfigured. These tests verify the existing `mapIGDBError` → 503 path works end-to-end:

```go
func TestSearchIGDB_NotConfigured(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	// Don't set IGDB credentials — NewClient returns unconfigured client
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "u-igdb-1", "igdbuser", "pass123", true, false)
	insertAuthTestSession(t, db, "u-igdb-1", "access-igdb-1", "refresh-igdb-1", 1)
	token := loginAndGetToken(t, e, "igdbuser", "pass123")

	body := `{"query": "Zelda", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGetIGDBGame_NotConfigured(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "u-igdb-2", "igdbuser2", "pass123", true, false)
	insertAuthTestSession(t, db, "u-igdb-2", "access-igdb-2", "refresh-igdb-2", 1)
	token := loginAndGetToken(t, e, "igdbuser2", "pass123")

	rec := getAuth(t, e, "/api/games/igdb/12345", token)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestImportFromIGDB_NotConfigured(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "u-igdb-3", "igdbuser3", "pass123", true, false)
	insertAuthTestSession(t, db, "u-igdb-3", "access-igdb-3", "refresh-igdb-3", 1)
	token := loginAndGetToken(t, e, "igdbuser3", "pass123")

	body := `{"igdb_id": 12345}`
	rec := postAuth(t, e, "/api/games/igdb-import", token, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
```

Note: These tests rely on `testCfg()` not setting IGDB credentials, so `NewClient` returns an unconfigured client. Check that `testCfg()` doesn't set `IGDBClientID`/`IGDBClientSecret` — if it does, clear them in these tests.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/api/... -run "TestSearchIGDB_NotConfigured|TestGetIGDBGame_NotConfigured|TestImportFromIGDB_NotConfigured" -v`
Expected: All PASS (the 503 path already works via `ErrIGDBNotConfigured` → `mapIGDBError`).

- [ ] **Step 3: Verify non-IGDB endpoints still work with unconfigured IGDB**

Run: `go test ./internal/api/... -run "TestGamesList|TestGamesGet" -v`
Expected: All pass — these don't touch IGDB.

- [ ] **Step 4: Commit**

```bash
git add internal/api/games_test.go
git commit -m "test: add games handler tests for IGDB-not-configured path"
```

---

### Task 6: Full Test Suite Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test ./... -count=1`
Expected: All pass.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run`
Expected: No errors.

- [ ] **Step 3: Build the binary**

Run: `make build`
Expected: Compiles cleanly.

- [ ] **Step 4: Commit any fixups if needed**

If any tests or lint issues surfaced, fix and commit.
