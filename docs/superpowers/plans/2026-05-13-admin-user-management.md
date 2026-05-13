# Admin User Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the seven admin `/api/auth/admin/users/*` endpoints required by the frontend, plus the frontend updates needed to render the extended `deletion-impact` response.

**Architecture:** New `AdminUsersHandler` in `internal/api/admin_users.go` mirrors the `SyncHandler` pattern from `internal/api/sync.go` and plugs into the existing `adminGroup` (JWT + admin middleware) in `internal/api/router.go:267`. Cascade delete via existing FK constraints; explicit session wipe on password reset and `is_active=false`. Frontend gets three new fields on `UserDeletionImpact` and three new rendered rows.

**Tech Stack:** Go 1.25, Echo v5, Bun + pgdriver, golang.org/x/crypto/bcrypt, testcontainers-go (postgres:18-alpine); React 19, TanStack Router, Vitest, MSW.

**Spec:** [docs/superpowers/specs/2026-05-13-admin-user-management-design.md](../specs/2026-05-13-admin-user-management-design.md) — every behaviour, error message, and DTO shape is specified there. This plan covers structure, sequencing, and execution mechanics only.

---

## File Map

| Path | Action | Responsibility |
|---|---|---|
| `internal/api/admin_users.go` | Create | `AdminUsersHandler` + DTOs + 7 handler methods + `RegisterRoutes` |
| `internal/api/admin_users_test.go` | Create | Testcontainers-driven integration tests for all endpoints |
| `internal/api/router.go` | Modify (~5 lines after L283) | Construct handler and call `RegisterRoutes(adminGroup)` |
| `slumber.yaml` | Modify | Add `admin_users:` request folder with 7 entries |
| `ui/frontend/src/types/admin.ts` | Modify | Three new fields on `UserDeletionImpact` |
| `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx` | Modify (near L576) | Three new rows in the deletion-impact table |
| `ui/frontend/src/api/admin.test.ts` | Modify (near L390) | Three new fields on the mock response |

---

## Task Graph

```
Task 1 (backend handler + tests + router wiring) ──► Task 2 (slumber collection)
Task 3 (frontend type) ──┬──► Task 4 (frontend UI rows)
                         └──► Task 5 (frontend test mock)
```

Task 1 and Task 3 have no dependencies and may run in parallel. Task 1 already includes the router wiring (Step 1.4) so its routes are reachable; Task 2 only edits `slumber.yaml` but is sequenced after Task 1 so the collection can be exercised against a running server. Tasks 4 and 5 both consume Task 3's extended interface and can run in parallel with each other.

---

## Task 1: Backend handler + tests

**Files:**
- Create: `internal/api/admin_users.go`
- Create: `internal/api/admin_users_test.go`

**Reference patterns:**
- Handler shape: `internal/api/sync.go` (`SyncHandler`, `NewSyncHandler`, `RegisterRoutes`)
- Test helpers: `internal/api/auth_test.go` lines 37–181 (`setupAuthTestDB`, `insertAuthTestUser`, `insertAuthTestSession`, `newTestEcho`, `testCfg`, `postJSON`, `postJSONAuth`)
- bcrypt cost constant: `bcryptCost = 12` in `internal/api/auth.go`
- Session wipe: `auth.HashToken(...)` for matching sessions

The test file lives in `package api_test`. **Define test-local request helpers for GET/PUT/DELETE with Bearer auth** (the existing `auth_test.go` only has POST helpers); place them at the top of `admin_users_test.go` so they're local to this file:

```go
func getAuth(t *testing.T, h interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path, accessToken string) *httptest.ResponseRecorder {
    t.Helper()
    req := httptest.NewRequest(http.MethodGet, path, nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    return rec
}

func putJSONAuth(t *testing.T, h interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path string, body any, accessToken string) *httptest.ResponseRecorder {
    t.Helper()
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+accessToken)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    return rec
}

func deleteAuth(t *testing.T, h interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path, accessToken string) *httptest.ResponseRecorder {
    t.Helper()
    req := httptest.NewRequest(http.MethodDelete, path, nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    return rec
}

// loginAs creates a session and returns (accessToken, refreshToken) for the user.
// The simplest path: hit POST /api/auth/login with the known plaintext password.
func loginAs(t *testing.T, h interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, username, password string) (string, string) {
    t.Helper()
    rec := postJSON(t, h, "/api/auth/login", map[string]string{"username": username, "password": password})
    if rec.Code != http.StatusOK {
        t.Fatalf("login as %s: status %d body %s", username, rec.Code, rec.Body)
    }
    var r struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
    }
    if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
        t.Fatalf("decode login response: %v", err)
    }
    if r.AccessToken == "" {
        t.Fatalf("loginAs: empty access_token; body=%s", rec.Body)
    }
    return r.AccessToken, r.RefreshToken
}
```

(Note: `auth_test.go` defines `postJSON` in the same `package api_test`, so it's already in scope.)

Tests should drive through `newTestEcho` so the full middleware chain (JWT + admin) runs — not the handler in isolation. This catches wiring bugs early.

**Steps:**

- [ ] **Step 1.1: Write the skeleton test file**

Create `internal/api/admin_users_test.go` with package declaration, imports, and the four helpers above. No test functions yet.

```bash
go test ./internal/api/... -run TestNothing -v
# Expected: ok with "no tests to run"
```

- [ ] **Step 1.2: Write failing test for `POST /api/auth/admin/users` (happy path)**

```go
func TestAdminCreateUser_HappyPath(t *testing.T) {
    db := setupAuthTestDB(t)
    cfg := testCfg()
    e := newTestEcho(t, db, cfg)

    insertAuthTestUser(t, db, "admin-001", "rootadmin", "password123", true, true)
    adminTok, _ := loginAs(t, e, "rootadmin", "password123")

    rec := postJSONAuth(t, e, "/api/auth/admin/users", map[string]any{
        "username": "newbie",
        "password": "secret123",
        "is_admin": false,
    }, adminTok)

    if rec.Code != http.StatusCreated {
        t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body)
    }
    var resp map[string]any
    _ = json.Unmarshal(rec.Body.Bytes(), &resp)
    if resp["username"] != "newbie" || resp["is_admin"] != false || resp["is_active"] != true {
        t.Fatalf("unexpected response: %v", resp)
    }
    if _, ok := resp["password_hash"]; ok {
        t.Fatal("response leaked password_hash")
    }
}
```

Run: `go test ./internal/api/ -run TestAdminCreateUser_HappyPath -v`
Expected: FAIL with 404 (route not registered).

- [ ] **Step 1.3: Scaffold the handler file with DTOs + constructor + empty handlers + RegisterRoutes**

Create `internal/api/admin_users.go`. Define exactly the DTO types from spec § DTOs. Add:

```go
type AdminUsersHandler struct {
    db *bun.DB
}

func NewAdminUsersHandler(db *bun.DB) *AdminUsersHandler {
    return &AdminUsersHandler{db: db}
}

// RegisterRoutes registers all admin user management routes on the given group.
// The caller must apply JWTMiddleware + AdminMiddleware to the group.
// Static routes are registered before parameterised ones (Echo v5 doesn't auto-sort).
func (h *AdminUsersHandler) RegisterRoutes(g *echo.Group) {
    g.POST("/api/auth/admin/users", h.HandleCreate)
    g.GET("/api/auth/admin/users", h.HandleList)
    g.PUT("/api/auth/admin/users/:id/password", h.HandleResetPassword)
    g.GET("/api/auth/admin/users/:id/deletion-impact", h.HandleDeletionImpact)
    g.GET("/api/auth/admin/users/:id", h.HandleGet)
    g.PUT("/api/auth/admin/users/:id", h.HandleUpdate)
    g.DELETE("/api/auth/admin/users/:id", h.HandleDelete)
}
```

Stub each `Handle*` method as `return c.JSON(http.StatusNotImplemented, map[string]string{"error": "not implemented"})`.

**Important:** this task does *not* modify `router.go`. The handler is unreachable from the running router yet. Tests can still drive it via `newTestEcho` only if the router wires it up; since router wiring is Task 2, this step is intentionally separated. To make tests work in this task, **also include a temporary registration block inside Task 1** — see the next step.

- [ ] **Step 1.4: Temporarily wire the handler into the test path**

This task needs the route to be reachable from `newTestEcho`. The cleanest way is to commit the router wiring as part of Task 1 — but the plan splits backend wiring into Task 2 to keep blast radius small. **Resolution: do the router wiring as Step 1.4 here**, and Task 2 then collapses to "slumber.yaml only".

Modify `internal/api/router.go`. Inside the `if db != nil { ... }` block, after the existing `adminBackups` registrations (~line 277) and before `// Sync routes`, add:

```go
        // Admin user management routes (JWT + admin required)
        auh := NewAdminUsersHandler(db)
        auh.RegisterRoutes(adminGroup)
```

Run the test from Step 1.2:
```bash
go test ./internal/api/ -run TestAdminCreateUser_HappyPath -v
```
Expected: FAIL with 501 ("not implemented") — the route is reachable but stubbed.

- [ ] **Step 1.5: Implement `HandleCreate` per spec § HandleCreate**

Bind → validate (username trim+≥3; password ≥6; "<field> is required" / "<field> must be at least N characters") → uniqueness check (`SELECT 1 FROM users WHERE username = ?` via `db.NewSelect`) → `bcrypt.GenerateFromPassword(..., 12)` → insert with `gen_random_uuid()::text`, `is_active=true`, `preferences="{}"`, `created_at`/`updated_at = time.Now().UTC()` → return 201 + `adminUserResponse`.

Re-run test from Step 1.2. Expected: PASS.

- [ ] **Step 1.6: Write failing tests for `HandleCreate` edge cases**

```go
func TestAdminCreateUser_DuplicateUsername(t *testing.T) { /* 400, "username already taken" */ }
func TestAdminCreateUser_ShortUsername(t *testing.T)      { /* 400, "username must be at least 3 characters" */ }
func TestAdminCreateUser_ShortPassword(t *testing.T)       { /* 400, "password must be at least 6 characters" */ }
func TestAdminCreateUser_RequiresJWT(t *testing.T)         { /* 401 with no Authorization header */ }
func TestAdminCreateUser_RequiresAdmin(t *testing.T)       { /* 403 with non-admin JWT */ }
```

Each test follows the Step 1.2 shape: `setupAuthTestDB` → `insertAuthTestUser` (admin) → `loginAs` → `postJSONAuth`. For `RequiresAdmin`, insert a non-admin user with `isAdmin=false` and use their token. For `RequiresJWT`, use `postJSON` (no Authorization header).

Run them — they should fail (validation not yet implemented). Then complete `HandleCreate` validation. Run again — they should pass.

- [ ] **Step 1.7: Implement `HandleList`, `HandleGet` + tests**

Per spec § HandleList and § HandleGet. Tests: happy-path (returns ≥1 user, no `password_hash` field), 404 unknown id, 401/403 auth.

```go
// HandleList — SELECT * FROM users ORDER BY created_at DESC, mapped to []adminUserResponse.
// HandleGet — SELECT * FROM users WHERE id = ?, 404 on sql.ErrNoRows.
```

- [ ] **Step 1.8: Implement `HandleUpdate` + tests**

Per spec § HandleUpdate. Decode `adminUpdateUserRequest` (pointer fields). Load target user, 404 if missing. Apply self-protection (compare `:id` to `c.Get("user_id").(string)`). For username changes: trim, validate ≥3 chars, uniqueness check against other rows (`WHERE username = ? AND id != ?`). Build the `UPDATE` inside a `db.RunInTx` transaction; if the patch sets `is_active=false`, also `DELETE FROM user_sessions WHERE user_id = ?` inside the same tx.

Tests:
```go
func TestAdminUpdateUser_HappyPath(t *testing.T)              // toggle is_admin
func TestAdminUpdateUser_RenameUsername(t *testing.T)
func TestAdminUpdateUser_DuplicateUsername(t *testing.T)      // 400
func TestAdminUpdateUser_DeactivateSelf_Rejected(t *testing.T) // 400 "Cannot deactivate your own account"
func TestAdminUpdateUser_DemoteSelf_Rejected(t *testing.T)     // 400 "Cannot remove your own admin privileges"
func TestAdminUpdateUser_DeactivateInvalidatesSessions(t *testing.T)
    // Insert target user, give them a session, PUT is_active=false, then GET /api/auth/me with the old token → 401
func TestAdminUpdateUser_PromoteDoesNotInvalidateSessions(t *testing.T)
    // Toggle is_admin and verify the existing session still works on a JWT-protected route
```

- [ ] **Step 1.9: Implement `HandleResetPassword` + tests**

Per spec § HandleResetPassword. Validate `new_password` length, hash with cost 12, transaction wraps the password update and `DELETE FROM user_sessions WHERE user_id = ?`. Return 200 + `{"message":"Password reset successfully. User will need to log in again."}`.

Tests:
```go
func TestAdminResetPassword_HappyPath(t *testing.T)
    // After reset: old token returns 401 on /api/auth/me, login with new password succeeds
func TestAdminResetPassword_ShortPassword_Rejected(t *testing.T)
func TestAdminResetPassword_UnknownUser_NotFound(t *testing.T)
func TestAdminResetPassword_AdminResetsOwnPassword_WipesOwnSessions(t *testing.T)
    // Admin's own session is invalidated; they must re-login.
```

- [ ] **Step 1.10: Implement `HandleDeletionImpact` + tests**

Per spec § HandleDeletionImpact. Load user (404 if missing) → self-protection 400 → run the seven `SELECT COUNT(*)` queries against `user_games`, `tags`, `jobs WHERE job_type='import'`, `jobs WHERE job_type='export'`, `jobs WHERE job_type='sync'`, `user_sync_configs`, `user_sessions`. Return `adminDeletionImpactResponse` with the static `Warning` string from the spec.

Tests:
```go
func TestAdminDeletionImpact_HappyPath(t *testing.T)
    // Seed target user with 2 user_games, 3 tags, 1 import job, 2 export jobs, 1 sync job, 1 sync_config, 1 session
    // Verify the response has the exact counts.
func TestAdminDeletionImpact_Self_Rejected(t *testing.T)   // 400
func TestAdminDeletionImpact_Unknown_NotFound(t *testing.T) // 404
```

Use `db.ExecContext` with raw inserts to seed dependent rows. Reuse `insertAuthTestUser` for the target; the related-row inserts go inline in the test (don't add new helpers — they are one-shot).

- [ ] **Step 1.11: Implement `HandleDelete` + tests**

Per spec § HandleDelete. Self-check → `DELETE FROM users WHERE id = ?` → 200 + success message.

Tests:
```go
func TestAdminDeleteUser_HappyPath(t *testing.T)
    // Verify user row is gone after the call.
func TestAdminDeleteUser_CascadesToRelatedTables(t *testing.T)
    // Seed user_games, tags, jobs, job_items, user_sessions, user_sync_configs, external_games.
    // After DELETE, COUNT(*) is 0 in each table for that user_id.
func TestAdminDeleteUser_Self_Rejected(t *testing.T)        // 400
func TestAdminDeleteUser_Unknown_NotFound(t *testing.T)     // 404
func TestAdminDeleteUser_RequiresAdmin(t *testing.T)        // 403
```

- [ ] **Step 1.12: Run all backend quality gates**

```bash
go test -timeout 300s ./...
golangci-lint run
```
Both must exit zero. Fix anything red before committing.

- [ ] **Step 1.13: Commit**

```bash
git add internal/api/admin_users.go internal/api/admin_users_test.go internal/api/router.go
git commit -m "$(cat <<'EOF'
feat(admin): add admin user management endpoints (phase 5)

Implements POST/GET/PUT/DELETE /api/auth/admin/users/* with
self-protection, session invalidation on password reset and
is_active=false, and cascade delete via existing FKs.

The deletion-impact response is extended beyond the Python
version with total_export_jobs, total_sync_jobs, and
total_sync_configs (frontend changes follow).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Acceptance criteria for Task 1:**
- `internal/api/admin_users.go` exists with all 7 handlers and `RegisterRoutes`.
- `internal/api/admin_users_test.go` covers happy path + 401/403/404/400/self-protection for every endpoint, plus cascade and session-invalidation integration tests.
- `internal/api/router.go` wires `auh := NewAdminUsersHandler(db); auh.RegisterRoutes(adminGroup)` after the existing `adminBackups` registrations.
- `go test -timeout 300s ./...` and `golangci-lint run` both pass.

---

## Task 2: Slumber collection

**Files:**
- Modify: `slumber.yaml`

**Depends on:** Task 1 (the routes must exist on the running server for slumber's collection to be exercisable; this task itself is config-only).

**Steps:**

- [ ] **Step 2.1: Add the `admin_users` request folder**

Insert immediately after the existing `admin:` folder (or just before `bootstrap:`, preserving alphabetical order per CLAUDE.md — `admin_users` sorts after `admin`). The block:

```yaml
  admin_users:
    name: Admin Users
    requests:
      create_user:
        name: Create User
        method: POST
        url: "{{base_url}}/api/auth/admin/users"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            username: "newuser"
            password: "secret123"
            is_admin: false

      list_users:
        name: List Users
        method: GET
        url: "{{base_url}}/api/auth/admin/users"
        $ref: "#/.authenticated"

      get_user:
        name: Get User
        method: GET
        url: "{{base_url}}/api/auth/admin/users/{{response('list_users', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}"
        $ref: "#/.authenticated"

      update_user:
        name: Update User
        method: PUT
        url: "{{base_url}}/api/auth/admin/users/{{response('list_users', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            is_active: "{{select(['true', 'false'])}}"

      reset_password:
        name: Reset Password
        method: PUT
        url: "{{base_url}}/api/auth/admin/users/{{response('list_users', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}/password"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            new_password: "newpassword123"

      deletion_impact:
        name: Deletion Impact
        method: GET
        url: "{{base_url}}/api/auth/admin/users/{{response('list_users', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}/deletion-impact"
        $ref: "#/.authenticated"

      delete_user:
        name: Delete User
        method: DELETE
        url: "{{base_url}}/api/auth/admin/users/{{response('list_users', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}"
        $ref: "#/.authenticated"
```

- [ ] **Step 2.2: Verify slumber loads the collection**

```bash
slumber collection
```
Expected: no parse errors. The new `admin_users` folder appears in the output.

- [ ] **Step 2.3: Commit**

```bash
git add slumber.yaml
git commit -m "chore(slumber): add admin user management requests

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

**Acceptance criteria for Task 2:**
- `slumber collection` exits zero.
- `slumber.yaml` contains an `admin_users:` request folder with all 7 entries, each using `{{base_url}}` and the `#/.authenticated` bearer auth ref.

---

## Task 3: Frontend type extension

**Files:**
- Modify: `ui/frontend/src/types/admin.ts`

**Depends on:** Nothing (DTO shape is fixed by the spec).

**Steps:**

- [ ] **Step 3.1: Add the three new fields to `UserDeletionImpact`**

Read the existing interface (lines 19–27). Add `total_export_jobs`, `total_sync_jobs`, `total_sync_configs` between `total_import_jobs` and `total_sessions` so the order matches the backend response. Final shape:

```typescript
export interface UserDeletionImpact {
  user_id: string;
  username: string;
  total_games: number;
  total_tags: number;
  total_import_jobs: number;
  total_export_jobs: number;
  total_sync_jobs: number;
  total_sync_configs: number;
  total_sessions: number;
  warning: string;
}
```

- [ ] **Step 3.2: Type-check the frontend**

```bash
cd ui/frontend
npm run check
```
Expected: zero TS errors. (Adding optional-looking fields to an interface used in two places — see Tasks 4 and 5 — will likely surface "Property X is missing" errors in mocks. That's expected and gets fixed by Tasks 4 and 5; the goal here is *just* the type extension.)

If `npm run check` fails specifically due to missing fields in the mock at `ui/frontend/src/api/admin.test.ts`, that's the expected failure. Document the failing locations in the commit message and continue.

- [ ] **Step 3.3: Commit**

```bash
git add ui/frontend/src/types/admin.ts
git commit -m "feat(types): extend UserDeletionImpact with export/sync/sync_configs counts

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

**Acceptance criteria for Task 3:**
- `UserDeletionImpact` declares the three new fields.
- Field order matches the backend JSON response (import → export → sync → sync_configs → sessions).

---

## Task 4: Frontend UI rows

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx`

**Depends on:** Task 3.

**Steps:**

- [ ] **Step 4.1: Read the existing deletion-impact table block**

```bash
sed -n '560,610p' ui/frontend/src/routes/_authenticated/admin/users/$id.tsx
```
Locate the row that renders `deletionImpact.total_import_jobs` (near line 576). Note the exact JSX shape used for each existing row (label cell + count cell + class names).

- [ ] **Step 4.2: Add three new rows after the import-jobs row**

Following the existing row pattern verbatim, add:

```tsx
<TableRow>
  <TableCell>Export jobs</TableCell>
  <TableCell className="text-right tabular-nums">
    {deletionImpact.total_export_jobs}
  </TableCell>
</TableRow>
<TableRow>
  <TableCell>Sync jobs</TableCell>
  <TableCell className="text-right tabular-nums">
    {deletionImpact.total_sync_jobs}
  </TableCell>
</TableRow>
<TableRow>
  <TableCell>Sync configs</TableCell>
  <TableCell className="text-right tabular-nums">
    {deletionImpact.total_sync_configs}
  </TableCell>
</TableRow>
```

The exact JSX (component names, classNames) **must match the existing rows** — use whatever shape the file already uses; the snippet above is illustrative. If the existing row uses `<div>` instead of `<TableRow>`, copy that.

- [ ] **Step 4.3: Run frontend type-check + tests**

```bash
cd ui/frontend
npm run check
npm run test -- --run admin
```
Expected: zero TS errors, all admin-related tests pass (Task 5 will update the mock if any fail here for missing fields).

- [ ] **Step 4.4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/admin/users/$id.tsx
git commit -m "feat(admin-ui): show export/sync/sync_configs counts in deletion-impact

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

**Acceptance criteria for Task 4:**
- The deletion-impact display renders three new rows for export jobs, sync jobs, and sync configs.
- Visual style matches the existing rows (same component, same classNames).
- `npm run check` passes.

---

## Task 5: Frontend test mock update

**Files:**
- Modify: `ui/frontend/src/api/admin.test.ts`

**Depends on:** Task 3.

**Steps:**

- [ ] **Step 5.1: Locate the deletion-impact mock**

```bash
sed -n '380,410p' ui/frontend/src/api/admin.test.ts
```
Find the mock at around line 390 (returned from the MSW `http.get('.../deletion-impact')` handler). It returns an object matching the old `UserDeletionImpact` shape.

- [ ] **Step 5.2: Add the three new fields to the mock**

Extend the returned object with sample values; order should match the backend response. Example:

```ts
return HttpResponse.json({
  user_id: 'user-123',
  username: 'testuser',
  total_games: 5,
  total_tags: 3,
  total_import_jobs: 2,
  total_export_jobs: 1,
  total_sync_jobs: 4,
  total_sync_configs: 2,
  total_sessions: 1,
  warning: 'This action cannot be undone. All data listed above will be permanently deleted.',
});
```

If the existing test asserts specific counts, leave the existing values as-is and only add the three new ones with any non-zero plausible values. Don't change the warning text unless the test explicitly references it.

- [ ] **Step 5.3: Run frontend tests**

```bash
cd ui/frontend
npm run check
npm run test
```
Expected: zero TS errors, all tests pass.

- [ ] **Step 5.4: Commit**

```bash
git add ui/frontend/src/api/admin.test.ts
git commit -m "test(admin): include new deletion-impact fields in MSW mock

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

**Acceptance criteria for Task 5:**
- The MSW deletion-impact mock returns all 8 count fields and the warning string.
- `npm run check` and `npm run test` both pass.

---

## Final Verification

After all 5 tasks are complete, from the repo root:

```bash
# Backend
go test -timeout 300s ./...
golangci-lint run

# Frontend
cd ui/frontend
npm run check
npm run test
cd ../..

# Build everything (ensures the embedded SPA still compiles)
make
```

All commands must exit zero. Then push the feature branch:

```bash
git push -u origin phase-5-admin-user-management
```

Optionally open a PR — but per CLAUDE.md, only the user merges PRs, not the agent.
