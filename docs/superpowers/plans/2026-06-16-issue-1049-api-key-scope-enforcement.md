# API Key Read-Scope Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `read` API-key scope a real security boundary — a `read`-scoped key may perform only safe (non-mutating) requests; any write is rejected with `403`.

**Architecture:** Enforcement is centralized in `auth.AuthMiddleware`, which every API-key-reachable route already passes through. `tryAPIKey` is extended to also read `api_keys.scopes`; session-cookie auth maps to an implicit `write` scope (full access). A small pure helper decides whether a `(scope, method, route)` triple is permitted: a `read` scope allows any non-mutating method, plus an explicit allowlist of read-safe routes that must use a mutating method (only `POST /api/games/search/igdb`, which carries a JSON query body). This is **fail-safe** — new mutating routes are blocked for `read` keys by default; forgetting to allowlist a read-only POST denies access rather than granting writes. The check is orthogonal to and composes with the existing admin gate (`is_admin` is resolved from the user record identically for both auth methods).

**Tech Stack:** Go 1.26, Echo v5 (`c.RouteInfo().Path` / `.Method` give the matched route pattern inside group middleware), Bun + pgx, stdlib `testing` + testcontainers (shared `testDB`).

---

## Background (verified facts)

- `tryAPIKey` (`internal/auth/session.go:202-234`) selects only `user_id` from `api_keys` — scope is never read.
- `AuthMiddleware` (`internal/auth/session.go:122-175`) resolves `user_id` (cookie or key), loads the user, and sets `user_id`/`is_admin`/`user` on the context. It runs **after** routing for every authenticated group, so `c.RouteInfo().Path` returns the registered route pattern (e.g. `/api/games/search/igdb`, `/api/notifications/channels/:id/test`).
- Creation already validates + stores scope (`internal/api/auth.go:477-505`); `read`/`write` are the only legal values, default `write`.
- The only non-GET endpoint that is a pure read with no file upload is `POST /api/games/search/igdb` (`internal/api/games.go:207`, JSON query body). `POST /api/import/csv/inspect` is intentionally **not** allowlisted — it requires a file upload, and a read key must not push files to the server.
- Admin endpoints (`adminGroup := e.Group("", auth.AuthMiddleware(db), auth.AdminMiddleware())`, `internal/api/router.go:424`) keep working for API keys because `is_admin` comes from the user, not the auth method. The scope gate sits inside `AuthMiddleware`, so it applies to the admin group too.

## File Structure

- **Create** `internal/auth/scope.go` — scope constants + pure decision helpers (`isMutatingMethod`, `scopeAllowsRequest`, `readSafeRoutes`).
- **Create** `internal/auth/scope_test.go` — table-driven unit tests for the helpers (no DB).
- **Modify** `internal/auth/session.go` — `tryAPIKey` returns scope; `AuthMiddleware` resolves scope and enforces the gate.
- **Modify** `internal/api/auth_test.go` — integration test helpers + full enforcement matrix against the real router.
- **Modify** `docs/user-guide.md:47` — clarify that a `read` key cannot modify data.

---

## Task 1: Pure scope-decision helpers

**Files:**
- Create: `internal/auth/scope.go`
- Test: `internal/auth/scope_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/auth/scope_test.go`:

```go
package auth

import (
	"net/http"
	"testing"
)

func TestScopeAllowsRequest(t *testing.T) {
	cases := []struct {
		name    string
		scope   string
		method  string
		route   string
		allowed bool
	}{
		{"write scope, write method", scopeWrite, http.MethodDelete, "/api/user-games/:id", true},
		{"write scope, read method", scopeWrite, http.MethodGet, "/api/user-games", true},
		{"read scope, GET", scopeRead, http.MethodGet, "/api/user-games", true},
		{"read scope, HEAD", scopeRead, http.MethodHead, "/api/user-games", true},
		{"read scope, OPTIONS", scopeRead, http.MethodOptions, "/api/user-games", true},
		{"read scope, POST write", scopeRead, http.MethodPost, "/api/tags", false},
		{"read scope, PUT write", scopeRead, http.MethodPut, "/api/tags/:id", false},
		{"read scope, PATCH write", scopeRead, http.MethodPatch, "/api/settings", false},
		{"read scope, DELETE write", scopeRead, http.MethodDelete, "/api/user-games/:id", false},
		{"read scope, allowlisted IGDB search POST", scopeRead, http.MethodPost, "/api/games/search/igdb", true},
		{"read scope, csv inspect POST not allowlisted", scopeRead, http.MethodPost, "/api/import/csv/inspect", false},
		{"unknown scope treated as read", "bogus", http.MethodPost, "/api/tags", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeAllowsRequest(tc.scope, tc.method, tc.route); got != tc.allowed {
				t.Errorf("scopeAllowsRequest(%q,%q,%q) = %v, want %v", tc.scope, tc.method, tc.route, got, tc.allowed)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/auth/ -run TestScopeAllowsRequest -v`
Expected: FAIL — `undefined: scopeAllowsRequest` (and `scopeWrite`/`scopeRead`).

- [ ] **Step 3: Write the implementation**

Create `internal/auth/scope.go`:

```go
package auth

import "net/http"

// API-key scope values. These mirror the values validated and stored by the
// key-creation handler (internal/api/auth.go). "write" implies full access;
// "read" is restricted to safe (non-mutating) requests.
const (
	scopeRead  = "read"
	scopeWrite = "write"
)

// readSafeRoutes is the allowlist of routes a read-scoped key may call even
// though they use a mutating HTTP method, because they perform no write and
// accept no file upload. Keyed by the Echo matched-route pattern
// (c.RouteInfo().Path).
//
// Only POST /api/games/search/igdb qualifies: it must be a POST because it
// carries a JSON query body, but it only reads from IGDB. If you add another
// genuinely read-only route that must use POST/PUT/PATCH/DELETE, add its route
// pattern here — and never add a route that writes data or accepts an upload.
var readSafeRoutes = map[string]bool{
	"/api/games/search/igdb": true,
}

// isMutatingMethod reports whether m is a write HTTP method.
func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// scopeAllowsRequest reports whether a credential with the given scope may make
// a request with the given HTTP method against the given matched route pattern.
//
// Any scope other than "read" (including session auth, which maps to "write")
// is unrestricted. A "read" scope allows non-mutating methods plus the
// readSafeRoutes allowlist; everything else is denied. An unknown/empty scope
// is treated as "read" (deny mutations) so a malformed value fails safe.
func scopeAllowsRequest(scope, method, routePath string) bool {
	if scope != scopeRead {
		return true
	}
	if !isMutatingMethod(method) {
		return true
	}
	return readSafeRoutes[routePath]
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/auth/ -run TestScopeAllowsRequest -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/scope.go internal/auth/scope_test.go
git commit -m "fix: add API-key scope decision helpers"
```

---

## Task 2: Thread scope through auth and enforce the gate

**Files:**
- Modify: `internal/auth/session.go` (`tryAPIKey` ~202-234, `AuthMiddleware` ~122-175)
- Test: `internal/api/auth_test.go` (new integration matrix)

- [ ] **Step 1: Write the failing integration tests**

Add to the end of `internal/api/auth_test.go`. These helpers reuse the existing `newTestEcho`, `insertAuthTestUser`, `insertAuthTestSession`, `postJSONSession`:

```go
// ─── API key scope enforcement (issue #1049) ───────────────────────────────

// createAPIKeyWithScope creates a key with the given scope via the real
// endpoint and returns the raw key value.
func createAPIKeyWithScope(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, sessionID, scope string) string {
	t.Helper()
	rec := postJSONSession(t, e, "/api/auth/api-keys", map[string]any{
		"name":   "key-" + scope,
		"scopes": scope,
	}, sessionID)
	if rec.Code != http.StatusOK {
		t.Fatalf("create %s key: status = %d; body: %s", scope, rec.Code, rec.Body)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode create-key resp: %v", err)
	}
	key, _ := resp["key"].(string)
	if key == "" {
		t.Fatalf("create %s key: empty key in response", scope)
	}
	return key
}

// bearerReq fires a request authenticated with a Bearer API key and an optional
// JSON body, returning the recorder.
func bearerReq(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, method, path string, body any, key string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestAPIKeyScope_ReadKeyBlockedOnWrite(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u1", "scopeu1", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u1")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")

	rec := bearerReq(t, e, http.MethodPost, "/api/tags", map[string]any{"name": "blocked"}, readKey)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read key POST /api/tags: status = %d, want 403; body: %s", rec.Code, rec.Body)
	}

	// The write must not have happened.
	var count int
	_ = testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM tags WHERE user_id = ?", "scope-u1",
	).Scan(&count)
	if count != 0 {
		t.Errorf("tags created by read key = %d, want 0", count)
	}
}

func TestAPIKeyScope_ReadKeyAllowedOnGet(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u2", "scopeu2", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u2")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")

	rec := bearerReq(t, e, http.MethodGet, "/api/tags", nil, readKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("read key GET /api/tags: status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}

func TestAPIKeyScope_WriteKeyAllowedOnWrite(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u3", "scopeu3", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u3")
	writeKey := createAPIKeyWithScope(t, e, sessionID, "write")

	rec := bearerReq(t, e, http.MethodPost, "/api/tags", map[string]any{"name": "allowed"}, writeKey)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("write key POST /api/tags: got 403, want success; body: %s", rec.Body)
	}
}

func TestAPIKeyScope_DefaultScopeIsWrite(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u4", "scopeu4", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u4")
	// Omit "scopes" entirely → server defaults to "write".
	rec := postJSONSession(t, e, "/api/auth/api-keys", map[string]any{"name": "default-key"}, sessionID)
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	key, _ := resp["key"].(string)

	w := bearerReq(t, e, http.MethodPost, "/api/tags", map[string]any{"name": "ok"}, key)
	if w.Code == http.StatusForbidden {
		t.Fatalf("default-scope key POST /api/tags: got 403, want success; body: %s", w.Body)
	}
}

func TestAPIKeyScope_ReadKeyAllowedOnAllowlistedSearch(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u5", "scopeu5", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u5")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")
	writeKey := createAPIKeyWithScope(t, e, sessionID, "write")

	// POST /api/games/search/igdb is allowlisted: a read key must pass the scope
	// gate exactly like a write key. The handler's own status (IGDB is not
	// configured in this test) is irrelevant — what matters is that the read key
	// is NOT rejected by the scope gate, i.e. it behaves identically to a write key.
	body := map[string]any{"query": "zelda"}
	readRec := bearerReq(t, e, http.MethodPost, "/api/games/search/igdb", body, readKey)
	writeRec := bearerReq(t, e, http.MethodPost, "/api/games/search/igdb", body, writeKey)
	if readRec.Code == http.StatusForbidden {
		t.Fatalf("read key on allowlisted search: got 403, want not-forbidden; body: %s", readRec.Body)
	}
	if readRec.Code != writeRec.Code {
		t.Errorf("allowlisted search: read key status %d != write key status %d", readRec.Code, writeRec.Code)
	}
}

func TestAPIKeyScope_ReadKeyBlockedOnUpload(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u6", "scopeu6", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u6")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")

	// csv/inspect is a POST but is intentionally NOT allowlisted (it requires a
	// file upload). The scope gate fires before any body parsing, so a bodyless
	// POST is enough to assert the rejection.
	rec := bearerReq(t, e, http.MethodPost, "/api/import/csv/inspect", nil, readKey)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read key POST /api/import/csv/inspect: status = %d, want 403; body: %s", rec.Code, rec.Body)
	}
}

func TestAPIKeyScope_SessionCookieImpliesWrite(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-u7", "scopeu7", "pw", true, false)
	sessionID := insertAuthTestSession(t, testDB, "scope-u7")

	rec := postJSONSession(t, e, "/api/tags", map[string]any{"name": "session-write"}, sessionID)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("session cookie POST /api/tags: got 403, want success; body: %s", rec.Body)
	}
}

func TestAPIKeyScope_AdminReadKeyBlockedOnAdminWrite(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-admin1", "scopeadmin1", "pw", true, true) // admin
	sessionID := insertAuthTestSession(t, testDB, "scope-admin1")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")

	// Admin user's READ key on an admin WRITE endpoint → 403 from the scope gate
	// (which runs before AdminMiddleware). Body is irrelevant.
	rec := bearerReq(t, e, http.MethodPut, "/api/admin/backups/config", map[string]any{}, readKey)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin read key PUT admin config: status = %d, want 403; body: %s", rec.Code, rec.Body)
	}
}

func TestAPIKeyScope_AdminReadKeyAllowedOnAdminGet(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-admin2", "scopeadmin2", "pw", true, true) // admin
	sessionID := insertAuthTestSession(t, testDB, "scope-admin2")
	readKey := createAPIKeyWithScope(t, e, sessionID, "read")

	// Admin's read key on an admin GET → allowed (GET passes scope; admin passes).
	rec := bearerReq(t, e, http.MethodGet, "/api/admin/events", nil, readKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin read key GET /api/admin/events: status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
}

func TestAPIKeyScope_NonAdminKeyBlockedOnAdminGet(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	insertAuthTestUser(t, testDB, "scope-nonadmin", "scopenonadmin", "pw", true, false) // not admin
	sessionID := insertAuthTestSession(t, testDB, "scope-nonadmin")
	writeKey := createAPIKeyWithScope(t, e, sessionID, "write")

	// Even a WRITE key from a non-admin must be blocked by the admin gate
	// (admin-guarding is unchanged and auth-method-agnostic).
	rec := bearerReq(t, e, http.MethodGet, "/api/admin/events", nil, writeKey)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin write key GET /api/admin/events: status = %d, want 403; body: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/ -run TestAPIKeyScope -v`
Expected: the read-key write tests FAIL (currently `read` keys can write, so e.g. `TestAPIKeyScope_ReadKeyBlockedOnWrite` gets 200/201 instead of 403). The GET/write-key/admin tests should already pass.

- [ ] **Step 3: Extend `tryAPIKey` to return scope**

In `internal/auth/session.go`, change `tryAPIKey` to also select and return `scopes`:

```go
// tryAPIKey checks for a Bearer API key. Returns ("", "", nil) when no Bearer
// header is present. Returns an error when a key is present but invalid. On
// success it returns the owning user id and the key's scope.
func tryAPIKey(c *echo.Context, db *bun.DB) (userID, scope string, err error) {
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", "", nil
	}
	raw := strings.TrimPrefix(authHeader, "Bearer ")
	if raw == "" {
		return "", "", errors.New("invalid or expired api key")
	}
	hash := HashToken(raw)
	err = db.QueryRowContext(c.Request().Context(),
		`SELECT user_id, scopes FROM api_keys
		 WHERE key_hash = ? AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())`,
		hash,
	).Scan(&userID, &scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.New("invalid or expired api key")
		}
		return "", "", err
	}
	go func() {
		if _, err := db.ExecContext(context.Background(),
			"UPDATE api_keys SET last_used_at = now() WHERE key_hash = ?",
			hash,
		); err != nil {
			slog.WarnContext(context.Background(), "auth: update api key last_used_at", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
	}()
	return userID, scope, nil
}
```

- [ ] **Step 4: Resolve scope and enforce the gate in `AuthMiddleware`**

In `internal/auth/session.go`, update the credential-resolution block and add the gate. Replace the existing block (lines ~125-138):

```go
			userID, sessionHash, cookieErr := trySessionCookie(c, db)
			if cookieErr != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "session expired or not found"})
			}
			// Session-cookie auth implies full (write) access; only API keys
			// can carry a restricted scope.
			scope := scopeWrite
			if userID == "" {
				apiUserID, apiScope, apiErr := tryAPIKey(c, db)
				if apiErr != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired api key"})
				}
				if apiUserID == "" {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing or invalid authorization"})
				}
				userID = apiUserID
				scope = apiScope
			}

			// Enforce the key scope before doing any work. The matched route
			// pattern is available here because AuthMiddleware runs after routing
			// for every authenticated group.
			if !scopeAllowsRequest(scope, c.Request().Method, c.RouteInfo().Path) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "api key has read-only scope; write operations are not permitted"})
			}
```

Leave the rest of `AuthMiddleware` (user load, `is_active` check, context sets, session `last_used_at` update) unchanged.

- [ ] **Step 5: Run the scope tests to verify they pass**

Run: `go test ./internal/api/ -run TestAPIKeyScope -v`
Expected: PASS (all subtests).

- [ ] **Step 6: Run the existing auth + middleware tests to verify no regression**

Run: `go test ./internal/auth/ ./internal/api/ -run 'Test(APIKey|Auth|HandleLogin|HandleLogout|GetMe|Scope)' -v`
Expected: PASS — existing `TestAPIKeyAuth_BearerToken` (a GET) and the create/list/revoke tests still pass.

- [ ] **Step 7: Commit**

```bash
git add internal/auth/session.go internal/api/auth_test.go
git commit -m "fix: enforce API-key read scope on mutating requests

A read-scoped API key could call every write/delete endpoint. AuthMiddleware
now resolves the key scope and rejects mutating requests from read keys with
403, allowlisting only the read-only IGDB search POST. Session-cookie auth
implies write. Closes #1049"
```

---

## Task 3: Documentation

**Files:**
- Modify: `docs/user-guide.md:47`

- [ ] **Step 1: Clarify what a read scope means**

In `docs/user-guide.md`, update the API-keys bullet (line 47). Replace:

```markdown
- **API keys** — create keys here if you want to talk to Nexorious from a script or the command-line tool. Each key has a name, a scope (read or write), and an optional expiry. A key's full value is shown only once when you create it, so copy it then. You can revoke a key at any time.
```

with:

```markdown
- **API keys** — create keys here if you want to talk to Nexorious from a script or the command-line tool. Each key has a name, a scope, and an optional expiry. A **read** key can only read your data — any attempt to create, change, or delete is rejected — which makes it safe to hand to a script or third-party tool that only needs to look. A **write** key has full access. A key's full value is shown only once when you create it, so copy it then. You can revoke a key at any time.
```

- [ ] **Step 2: Commit**

```bash
git add docs/user-guide.md
git commit -m "docs: clarify API-key read scope is read-only"
```

---

## Task 4: Final verification

- [ ] **Step 1: Dead-code check**

This change widens an existing function's return signature and adds a self-contained file with internal helpers; no callers are removed. Run a dead-code check anyway since the helper file adds new symbols:

Run: `make deadcode`
Expected: no *new* entries referencing `scopeAllowsRequest`, `isMutatingMethod`, or `readSafeRoutes` (all are referenced — by tests and by `AuthMiddleware`).

- [ ] **Step 2: Full backend test suite**

Run: `go test -timeout 600s ./internal/auth/ ./internal/api/`
Expected: PASS.

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./internal/auth/... ./internal/api/...`
Expected: no findings.

- [ ] **Step 4: Push and open PR**

```bash
git push -u origin fix/1049-api-key-scope-enforcement
gh pr create --title "fix: enforce API-key read scope on mutating requests" \
  --label bug --label security \
  --body "$(cat <<'EOF'
## Summary

API keys carried a `read`/`write` scope that was stored but never enforced — a `read` key could call every write/delete endpoint. This makes `read` a real boundary.

- `AuthMiddleware` now resolves the key scope (`tryAPIKey` selects `scopes`); a `read` key is rejected with `403` on any mutating method (POST/PUT/PATCH/DELETE).
- Session-cookie auth implies `write` (full access).
- Read-safe allowlist: only `POST /api/games/search/igdb` (a read that must POST a JSON query). Upload-bearing reads like `csv/inspect` are intentionally blocked for read keys.
- Fail-safe by design: new mutating routes are denied to read keys by default.
- Admin-guarding is unchanged and composes — an admin's read key still gets `403` on admin writes; non-admin keys still get `403` on admin endpoints.

## Tests

Full matrix in `internal/api/auth_test.go` (read→write=403, read→GET=200, write→write=ok, session→write=ok, allowlisted search behaves identically for read/write, upload blocked, admin read key blocked on admin write, admin gate still enforced for keys) plus pure-helper unit tests in `internal/auth/scope_test.go`.

Closes #1049
EOF
)"
```

---

## Self-Review

- **Spec coverage:** Issue's proposed fix points (1) surface scope from `tryAPIKey` → Task 2 Step 3; (2) scope-gate rejecting read keys on mutating methods with a route allowlist → Task 1 + Task 2 Step 4; (3) test matrix (read→write=403, read→read=200, write→write=200, session→write=200) → Task 2 Step 1. The issue's optional `auth.ScopeFromContext` is deliberately omitted (YAGNI — no consumer; would be dead code). Borderline endpoints resolved per discussion: only `search/igdb` allowlisted; export, notification-test, and `csv/inspect` treated as writes.
- **Placeholder scan:** none — every code/step is concrete.
- **Type consistency:** `scopeRead`/`scopeWrite` (Task 1) reused in `session.go` (Task 2). `scopeAllowsRequest(scope, method, routePath)` signature matches both the unit test (Task 1) and the call site (Task 2 Step 4). `tryAPIKey` new signature `(userID, scope string, err error)` matches its single caller in `AuthMiddleware`.
