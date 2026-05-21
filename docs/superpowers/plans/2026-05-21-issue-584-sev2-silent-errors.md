# Issue #584 — Stop Swallowing Auth and Credential Errors (Sev 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix five sites where auth and credential errors are silently discarded, causing empty tokens or corrupted credentials to propagate into downstream calls.

**Architecture:** Surgical edits only — no refactoring. Each fix adds a guard at the point of failure and returns the appropriate HTTP error or Go error. Three `json.Unmarshal` sites in `sync.go` return 500. The `c.Bind` swallow in `auth.go` returns 400. Two `psnClient.AccessToken()` call sites in `psn/client.go` get an empty-token guard that returns an error.

**Tech Stack:** Go stdlib, Echo v5, `log/slog`, `github.com/sizovilya/go-psn-api`

---

### Task 1: Create feature branch

**Files:**
- No file changes — branch creation only.

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b fix/issue-584-sev2-silent-errors
```

Expected: switched to new branch.

---

### Task 2: Add failing test — logout handler rejects malformed JSON body

**Files:**
- Modify: `internal/api/auth_test.go`

- [ ] **Step 1: Add the failing test**

Add this function at the end of the logout tests section (after `TestHandleLogout_NoAuthorizationHeader`):

```go
func TestHandleLogout_MalformedBody(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	insertAuthTestUser(t, testDB, "user-030", "zara", "pw", true, false)
	access, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-030", cfg.AccessTokenExpireMinutes)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout",
		strings.NewReader(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed body", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestHandleLogout_MalformedBody -v
```

Expected: FAIL — status is 200, want 400.

---

### Task 3: Fix auth.go — return 400 on malformed logout body

**Files:**
- Modify: `internal/api/auth.go` (around line 222)

- [ ] **Step 1: Replace the swallowed bind error**

Find this block in `HandleLogout` (around line 222):

```go
	var req logoutRequest
	// A missing/malformed body still results in 200 — logout is idempotent.
	_ = c.Bind(&req)
```

Replace with:

```go
	var req logoutRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
```

- [ ] **Step 2: Run the test to confirm it passes**

```bash
go test ./internal/api/... -run TestHandleLogout_MalformedBody -v
```

Expected: PASS.

- [ ] **Step 3: Run the full auth test suite**

```bash
go test ./internal/api/... -run TestHandleLogout -v
```

Expected: all logout tests pass (200 for valid/missing/double-logout scenarios, 400 for malformed body, 401 for missing auth header).

- [ ] **Step 4: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "fix(auth): return 400 on malformed logout request body"
```

---

### Task 4: Add failing tests — sync status handlers reject corrupted credentials

**Files:**
- Modify: `internal/api/sync_test.go`

The tests insert a `user_sync_configs` row directly with corrupted JSON in `storefront_credentials`, then hit the status endpoint.

- [ ] **Step 1: Add helper and three failing tests**

Add this helper and three test functions after `TestPSNStatus_WithCredentials` in `sync_test.go`:

```go
// insertCorruptedSyncConfig inserts a user_sync_configs row with
// deliberately corrupted storefront_credentials to test error handling.
func insertCorruptedSyncConfig(t *testing.T, db *bun.DB, userID, storefront string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (gen_random_uuid()::text, ?, ?, 'manual', 'THIS IS NOT JSON', now(), now())
		 ON CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = 'THIS IS NOT JSON', updated_at = now()`,
		userID, storefront,
	)
	if err != nil {
		t.Fatalf("insertCorruptedSyncConfig: %v", err)
	}
}

func TestPSNStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "psn-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "psn")

	rec := getAuth(t, e, "/api/sync/psn/connection", token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for corrupted PSN credentials", rec.Code)
	}
}

func TestEpicStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "epic-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "epic")

	rec := getAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for corrupted Epic credentials", rec.Code)
	}
}

func TestGOGStatus_CorruptedCredentials(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "gog-corrupt-creds")
	insertCorruptedSyncConfig(t, testDB, userID, "gog")

	rec := getAuth(t, e, "/api/sync/gog/connection", token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for corrupted GOG credentials", rec.Code)
	}
}
```

You also need to ensure `context` is already imported in `sync_test.go`. Check with:

```bash
grep '"context"' internal/api/sync_test.go
```

If missing, add it to the import block.

- [ ] **Step 2: Check which routes Epic and GOG status are registered at**

```bash
grep -n "HandleGetEpicConnection\|HandleGetGOGConnection\|epic/connection\|gog/connection" internal/api/sync.go | head -10
```

Confirm the paths are `/api/sync/epic/connection` and `/api/sync/gog/connection`.

- [ ] **Step 3: Run the three tests to confirm they fail**

```bash
go test ./internal/api/... -run "TestPSNStatus_CorruptedCredentials|TestEpicStatus_CorruptedCredentials|TestGOGStatus_CorruptedCredentials" -v
```

Expected: all three FAIL — status is 200, want 500.

---

### Task 5: Fix sync.go — three json.Unmarshal sites

**Files:**
- Modify: `internal/api/sync.go` (lines 589, 722, 1122)

All three sites follow the same pattern. Handle each one.

- [ ] **Step 1: Fix PSN status handler (line ~589)**

Find this block in `HandleGetPSNStatus`:

```go
	var creds struct {
		OnlineID       string     `json:"online_id"`
		AccountID      string     `json:"account_id"`
		Region         string     `json:"region"`
		IsVerified     bool       `json:"is_verified"`
		TokenExpiredAt *time.Time `json:"token_expired_at"`
	}
	_ = json.Unmarshal([]byte(*row.StorefrontCredentials), &creds)
```

Replace with:

```go
	var creds struct {
		OnlineID       string     `json:"online_id"`
		AccountID      string     `json:"account_id"`
		Region         string     `json:"region"`
		IsVerified     bool       `json:"is_verified"`
		TokenExpiredAt *time.Time `json:"token_expired_at"`
	}
	if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
		slog.Error("psn: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
```

- [ ] **Step 2: Fix Epic status handler (line ~722)**

Find this block in the Epic status handler:

```go
	var creds struct {
		DisplayName string `json:"display_name"`
		AccountID   string `json:"account_id"`
	}
	_ = json.Unmarshal([]byte(*row.StorefrontCredentials), &creds)
```

Replace with:

```go
	var creds struct {
		DisplayName string `json:"display_name"`
		AccountID   string `json:"account_id"`
	}
	if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
		slog.Error("epic: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
```

- [ ] **Step 3: Fix GOG status handler (line ~1122)**

Find this block in the GOG status handler:

```go
	var creds struct {
		Username string `json:"username"`
		UserID   string `json:"user_id"`
	}
	_ = json.Unmarshal([]byte(*row.StorefrontCredentials), &creds)
```

Replace with:

```go
	var creds struct {
		Username string `json:"username"`
		UserID   string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
		slog.Error("gog: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
```

- [ ] **Step 4: Run the three tests to confirm they pass**

```bash
go test ./internal/api/... -run "TestPSNStatus_CorruptedCredentials|TestEpicStatus_CorruptedCredentials|TestGOGStatus_CorruptedCredentials" -v
```

Expected: all three PASS.

- [ ] **Step 5: Run the full sync test suite**

```bash
go test ./internal/api/... -run TestPSNStatus -v
go test ./internal/api/... -run TestEpicStatus -v
go test ./internal/api/... -run TestGOGStatus -v
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "fix(sync): return 500 on corrupted stored credentials"
```

---

### Task 6: Fix psn/client.go — guard against empty access token

**Files:**
- Modify: `internal/services/psn/client.go` (lines 77 and 368)

`psnClient.AccessToken()` returns `(string, int32)` where the second value is the expiry timestamp. After a successful `AuthWithNPSSO`, an empty token indicates something went wrong internally. Guard against it.

- [ ] **Step 1: Fix GetAccountInfo (line ~77)**

Find this in `GetAccountInfo`:

```go
	// Fetch the authenticated user's own profile using the "me" alias supported
	// by Sony's profile API.
	accessToken, _ := psnClient.AccessToken()
	profile, err := fetchMyProfile(ctx, accessToken)
```

Replace with:

```go
	// Fetch the authenticated user's own profile using the "me" alias supported
	// by Sony's profile API.
	accessToken, _ := psnClient.AccessToken()
	if accessToken == "" {
		return nil, fmt.Errorf("psn: access token unavailable after authentication")
	}
	profile, err := fetchMyProfile(ctx, accessToken)
```

- [ ] **Step 2: Fix GetLibrary (line ~368)**

Find this in `GetLibrary`, inside the `else` branch:

```go
		if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
			slog.Error("psn: auth failed", "err", err)
			return ErrInvalidNPSSOToken
		}
		accessToken, _ = psnClient.AccessToken()
	}
	slog.Info("psn: auth succeeded")
```

Replace with:

```go
		if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
			slog.Error("psn: auth failed", "err", err)
			return ErrInvalidNPSSOToken
		}
		accessToken, _ = psnClient.AccessToken()
		if accessToken == "" {
			return fmt.Errorf("psn: access token unavailable after authentication")
		}
	}
	slog.Info("psn: auth succeeded")
```

Note: No unit test is added for these guards. The `psnClient.AccessToken()` call is inside the real `psnsdk` auth path; tests use the `authFn` override which bypasses this branch entirely. The guard is a defensive check for a condition that cannot occur in normal operation but would otherwise cause silent downstream failures.

- [ ] **Step 3: Build to verify no compilation errors**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 4: Run the PSN service tests**

```bash
go test ./internal/services/psn/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/services/psn/client.go
git commit -m "fix(psn): guard against empty access token after authentication"
```

---

### Task 7: Full verification

- [ ] **Step 1: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all pass, no failures.

- [ ] **Step 2: Run linter**

```bash
golangci-lint run
```

Expected: no errors.

- [ ] **Step 3: Confirm all five fix sites are addressed**

```bash
grep -n "_ = json.Unmarshal" internal/api/sync.go
grep -n "_ = c.Bind" internal/api/auth.go
grep -n "accessToken, _ =" internal/services/psn/client.go
```

Expected: zero output from all three commands.
