# Admin User Management — Design Spec

**Date:** 2026-05-13
**Status:** Approved
**Phase:** 5
**Parent specs:**
- [Nexorious Go Port — Design Spec](2026-05-03-go-port-design.md) (see "Admin User Management" section)
- [Phase 5 — Polish + Production Readiness](2026-05-03-go-port-design-phase-5.md)

## Overview

Implements the seven admin-only `/api/auth/admin/users/*` endpoints called by the frontend at [ui/frontend/src/api/admin.ts](../../../ui/frontend/src/api/admin.ts). The Python reference implementation lives at `backend/app/api/auth.py` in the sibling `nexorious` repository.

The endpoints let an admin create users, list/view them, update role and active state, reset passwords, preview the impact of deletion, and delete users. The deletion impact response is extended beyond the Python version: it adds `total_export_jobs`, `total_sync_jobs`, and `total_sync_configs`, and drops `total_wishlist_items` (wishlist is out of scope in the Go port).

## File Layout

- `internal/api/admin_users.go` — new file. `AdminUsersHandler` struct plus `NewAdminUsersHandler(db *bun.DB) *AdminUsersHandler` and a `RegisterRoutes(g *echo.Group)` method, mirroring [internal/api/sync.go](../../../internal/api/sync.go).
- `internal/api/admin_users_test.go` — companion testcontainers test file, mirroring `internal/api/auth_test.go`.
- [internal/api/router.go](../../../internal/api/router.go) — wire `AdminUsersHandler` onto the existing `adminGroup` (line 267). One added block, no other changes.

The `adminGroup` already chains `auth.JWTMiddleware` + `auth.AdminMiddleware`, so no new middleware is needed.

## Routes

| Method | Path | Handler |
|---|---|---|
| `POST`   | `/api/auth/admin/users`                          | `HandleCreate`           |
| `GET`    | `/api/auth/admin/users`                          | `HandleList`             |
| `GET`    | `/api/auth/admin/users/:id`                      | `HandleGet`              |
| `PUT`    | `/api/auth/admin/users/:id`                      | `HandleUpdate`           |
| `PUT`    | `/api/auth/admin/users/:id/password`             | `HandleResetPassword`    |
| `GET`    | `/api/auth/admin/users/:id/deletion-impact`      | `HandleDeletionImpact`   |
| `DELETE` | `/api/auth/admin/users/:id`                      | `HandleDelete`           |

Static `/password` and `/deletion-impact` routes are registered before the parameterised `/:id` routes (Echo v5 doesn't auto-sort).

## DTOs

All defined as unexported types inside `internal/api/admin_users.go`.

```go
type adminUserResponse struct {
    ID        string    `json:"id"`
    Username  string    `json:"username"`
    IsActive  bool      `json:"is_active"`
    IsAdmin   bool      `json:"is_admin"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type adminCreateUserRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
    IsAdmin  bool   `json:"is_admin"`
}

type adminUpdateUserRequest struct {
    Username *string `json:"username,omitempty"`
    IsActive *bool   `json:"is_active,omitempty"`
    IsAdmin  *bool   `json:"is_admin,omitempty"`
}

type adminResetPasswordRequest struct {
    NewPassword string `json:"new_password"`
}

type adminDeletionImpactResponse struct {
    UserID           string `json:"user_id"`
    Username         string `json:"username"`
    TotalGames       int    `json:"total_games"`         // user_games
    TotalTags        int    `json:"total_tags"`
    TotalImportJobs  int    `json:"total_import_jobs"`   // jobs WHERE job_type='import'
    TotalExportJobs  int    `json:"total_export_jobs"`   // jobs WHERE job_type='export'
    TotalSyncJobs    int    `json:"total_sync_jobs"`     // jobs WHERE job_type='sync'
    TotalSyncConfigs int    `json:"total_sync_configs"`  // user_sync_configs
    TotalSessions    int    `json:"total_sessions"`
    Warning          string `json:"warning"`
}

type adminSuccessResponse struct {
    Message string `json:"message"`
}
```

The `User` Bun model is used internally for create/select/update. Responses never serialise it directly — `adminUserResponse` strips `password_hash` and `preferences`.

`adminUpdateUserRequest` uses pointer fields so the handler can distinguish "absent" from "explicit false" / "explicit empty".

## Validation Rules

Performed in the handler before any DB write. On failure return `400 Bad Request` with `{"error": "<message>"}`.

**Create + Update — username (when provided):**
- Trim whitespace; reject empty → `"username is required"`
- Reject `len < 3` after trim → `"username must be at least 3 characters"`

**Create + Reset password — password / new_password:**
- Reject empty → `"password is required"` (or `"new password is required"` for reset)
- Reject `len < 6` → `"password must be at least 6 characters"`

**Uniqueness (create + update where username changes):**
- Reject existing username (case-sensitive, matching Python and the existing `users.username` index) → `"username already taken"`

Validation errors use plain `{"error": "..."}` JSON to match the frontend's existing error-handling pattern (`err instanceof Error ? err.message : ...`).

## Self-Protection Rules

Compare the target `:id` to `c.Get("user_id").(string)` (set by `JWTMiddleware`).

| Handler | Forbidden self-action | Status | Message |
|---|---|---|---|
| `HandleUpdate`         | `is_active == false` on self     | 400 | "Cannot deactivate your own account" |
| `HandleUpdate`         | `is_admin == false` on self      | 400 | "Cannot remove your own admin privileges" |
| `HandleDeletionImpact` | target is self                   | 400 | "Cannot delete your own account" |
| `HandleDelete`         | target is self                   | 400 | "Cannot delete your own account" |

`HandleResetPassword` is **not** restricted to non-self — an admin can reset their own password. Doing so still invalidates all their own sessions (forces re-login).

## Session Invalidation

The `user_sessions` table is wiped for the target user under these conditions:

| Trigger | Reason |
|---|---|
| `HandleResetPassword`            | Refresh tokens otherwise survive the rotation. Mandatory. |
| `HandleUpdate` with `is_active=false` | Belt-and-suspenders: `JWTMiddleware` already rejects inactive users on the next request, but explicit invalidation kills sessions immediately. |
| `HandleDelete`                   | Handled by FK CASCADE — no extra delete needed. |

Role changes (`is_admin` toggle) do **not** invalidate sessions. `JWTMiddleware` re-reads `is_admin` from the `users` table on every request, so demotion takes effect on the very next request and promotion on the next one too.

The wipe is `DELETE FROM user_sessions WHERE user_id = ?` issued via `db.NewDelete()`, executed in the same `bun.Tx` as the parent change.

## Cascade Delete

All schema FKs referencing `users.id` declare `ON DELETE CASCADE` (verified across migrations — seven tables: `user_games`, `tags`, `jobs`, `job_items`, `user_sessions`, `user_sync_configs`, `external_games`). `HandleDelete` executes a single `DELETE FROM users WHERE id = ?`. PostgreSQL cascades the deletion through every dependent row. `user_game_platforms` and `user_game_tags` cascade transitively via their parent `user_games` row.

No manual per-table delete is required (this is where the Go port diverges from the Python reference, which deletes manually because SQLModel does not cascade).

## Handler Behaviour

### `HandleCreate` — `POST /api/auth/admin/users`

1. Bind `adminCreateUserRequest`.
2. Validate username + password.
3. Check uniqueness — if `SELECT 1 FROM users WHERE username = ?` returns a row, reject.
4. `bcrypt.GenerateFromPassword(..., bcryptCost)` where `bcryptCost = 12` is the existing constant in `internal/api/auth.go`.
5. Insert with `IsActive=true`, `IsAdmin=req.IsAdmin`, `Preferences="{}"`, fresh UUID, `CreatedAt`/`UpdatedAt = time.Now().UTC()`.
6. Return `201 Created` with `adminUserResponse`.

### `HandleList` — `GET /api/auth/admin/users`

`SELECT * FROM users ORDER BY created_at DESC` mapped to `[]adminUserResponse`. The frontend's `getAdminStatistics` re-sorts client-side, but newest-first is a sensible default.

### `HandleGet` — `GET /api/auth/admin/users/:id`

Single row lookup. `sql.ErrNoRows` → `404 {"error":"user not found"}`.

### `HandleUpdate` — `PUT /api/auth/admin/users/:id`

1. Bind `adminUpdateUserRequest`.
2. Load target user; 404 if not found.
3. Apply self-protection checks.
4. If `Username != nil` and trimmed value differs from current: validate length, check uniqueness, set.
5. If `IsActive != nil`: set.
6. If `IsAdmin != nil`: set.
7. Bump `UpdatedAt`.
8. In a `bun.Tx`:
   - `UPDATE users SET ...`
   - If `IsActive` was set to false: `DELETE FROM user_sessions WHERE user_id = ?`
9. Return `200 OK` with updated `adminUserResponse`.

If the update body sets no fields (all pointers nil), still bump `UpdatedAt` and return the row — matches the Python no-op behaviour. (Caller error, not server error.)

### `HandleResetPassword` — `PUT /api/auth/admin/users/:id/password`

1. Bind `adminResetPasswordRequest`.
2. Validate password length.
3. Load target user; 404 if not found.
4. Hash new password with bcrypt cost 12.
5. In a `bun.Tx`:
   - `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`
   - `DELETE FROM user_sessions WHERE user_id = ?`
6. Return `200 OK` with `{"message":"Password reset successfully. User will need to log in again."}` (matches Python copy verbatim).

### `HandleDeletionImpact` — `GET /api/auth/admin/users/:id/deletion-impact`

1. Load target user; 404 if not found.
2. Self-check → 400 "Cannot delete your own account".
3. Run seven COUNT queries sequentially (all single-indexed lookups):

```sql
SELECT COUNT(*) FROM user_games         WHERE user_id = ?
SELECT COUNT(*) FROM tags               WHERE user_id = ?
SELECT COUNT(*) FROM jobs               WHERE user_id = ? AND job_type = 'import'
SELECT COUNT(*) FROM jobs               WHERE user_id = ? AND job_type = 'export'
SELECT COUNT(*) FROM jobs               WHERE user_id = ? AND job_type = 'sync'
SELECT COUNT(*) FROM user_sync_configs  WHERE user_id = ?
SELECT COUNT(*) FROM user_sessions      WHERE user_id = ?
```

4. Return `200 OK` with `adminDeletionImpactResponse`. `Warning` is the static string `"This action cannot be undone. All data listed above will be permanently deleted."` (matches Python).

`metadata_refresh` jobs are intentionally not surfaced — they are an admin-initiated, app-global concern and inflate the per-user view without adding meaningful signal.

### `HandleDelete` — `DELETE /api/auth/admin/users/:id`

1. Load target user; 404 if not found.
2. Self-check → 400 "Cannot delete your own account".
3. `DELETE FROM users WHERE id = ?` (FK CASCADE removes the rest).
4. Return `200 OK` with `{"message":"User and all associated data deleted successfully"}` (matches Python copy verbatim).

## Frontend Changes

Required for the deletion-impact extension:

- [ui/frontend/src/types/admin.ts](../../../ui/frontend/src/types/admin.ts) — extend `UserDeletionImpact`:

```ts
export interface UserDeletionImpact {
  user_id: string;
  username: string;
  total_games: number;
  total_tags: number;
  total_import_jobs: number;
  total_export_jobs: number;   // new
  total_sync_jobs: number;     // new
  total_sync_configs: number;  // new
  total_sessions: number;
  warning: string;
}
```

- [ui/frontend/src/routes/_authenticated/admin/users/$id.tsx](../../../ui/frontend/src/routes/_authenticated/admin/users/$id.tsx) — the deletion-impact table is rendered around line 576. Add three new rows for export jobs, sync jobs, and sync configs, matching the existing presentation style.
- [ui/frontend/src/api/admin.test.ts](../../../ui/frontend/src/api/admin.test.ts) — update the mock deletion-impact response (around line 390) to include the three new fields.

No other frontend changes are needed; the existing `getUsers` / `getUserById` / `createUser` / `updateUser` / `resetUserPassword` / `deleteUser` calls in `ui/frontend/src/api/admin.ts` already match the backend contract exactly.

## Tests

`internal/api/admin_users_test.go` uses the same testcontainers helpers as `internal/api/auth_test.go`. For each endpoint:

- 401 with no Authorization header
- 403 with a non-admin JWT
- 404 with an unknown `:id` (where applicable)
- 400 with invalid payloads (validation + self-protection paths)
- Happy-path success

Plus integration tests:

- `DELETE` cascades — verify `user_games`, `tags`, `jobs`, `user_sessions`, `user_sync_configs`, `external_games` are empty for the deleted user.
- `HandleResetPassword` invalidates sessions — old token returns 401 on next request to a protected endpoint.
- `HandleUpdate` with `is_active=false` invalidates sessions and subsequent requests with the old token return 401.
- `HandleUpdate` with `is_admin` toggle does *not* invalidate sessions but the next admin-route request reflects the new role (via `JWTMiddleware` re-read).
- Self-protection: an admin cannot deactivate, demote, or delete themselves.

The tests share the existing testcontainers fixture (one container per package) and use raw `*bun.DB` setup helpers from `auth_test.go`.

## Slumber Collection

Add seven requests to [slumber.yaml](../../../slumber.yaml) under a new `admin_users/` folder (alphabetically right after `admin/` if present, otherwise after the existing `admin` references):

- `create_user` — POST
- `list_users` — GET
- `get_user` — GET
- `update_user` — PUT
- `reset_password` — PUT
- `deletion_impact` — GET
- `delete_user` — DELETE

Each uses `{{base_url}}` and the standard `authentication: type: bearer` block referencing the existing `login` request, matching the pattern in CLAUDE.md.

## Out of Scope

- Pagination on `GET /api/auth/admin/users` (not needed at expected scales; matches Python).
- Audit logging (not in the parent spec).
- Bulk operations.
- Email notifications on password reset.
- Self-service password change is already implemented in [internal/api/auth.go](../../../internal/api/auth.go) (`HandleChangePassword`); this spec only covers admin-initiated resets.
