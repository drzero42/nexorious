# Issue #689: Clear Library (user) + Reset Database (admin)

## Overview

Two destructive-action features, both requiring explicit typed confirmation before executing.

- **Clear Library**: lets any authenticated user wipe their entire game library in one operation.
- **Reset Database**: lets an admin return the instance to post-admin-creation state (empty library, no non-admin users, no sync config, no jobs, no tags).

---

## Backend

### Feature 1: `DELETE /api/user-games`

**Handler**: `HandleClearLibrary` added to `UserGamesHandler` in `internal/api/user_games.go`.

**Auth**: standard session/API-key auth (same middleware as all other user-games routes).

**Steps** (single `RunInTx`):
1. Cancel non-terminal River jobs for the user:
   ```sql
   UPDATE river_job
   SET state = 'cancelled', finalized_at = NOW()
   WHERE args->>'user_id' = $userID
     AND state NOT IN ('completed', 'cancelled', 'discarded')
   ```
2. `DELETE FROM jobs WHERE user_id = $userID` — FK cascade removes `job_items` and `sync_changes`.
3. `DELETE FROM user_sync_configs WHERE user_id = $userID`.
4. `DELETE FROM user_games WHERE user_id = $userID` — FK cascade removes `user_game_platforms` and `user_game_tags`; capture `RowsAffected` as N.

**Response**: `{"deleted": N}` — 200 on success. Idempotent: empty library → `{"deleted": 0}`, still 200.

**Tags** are intentionally preserved — they are user configuration, not library content. `user_game_tags` rows cascade from the `user_games` delete.

**Route registration** in `router.go` (inside the authenticated `userGamesGroup`):
```go
userGamesGroup.DELETE("", ugh.HandleClearLibrary)
```
Registered before `/:id` (Echo v5 static-before-parameterised rule; `""` is exact so no conflict).

---

### Feature 2: `POST /api/auth/admin/reset`

**New file**: `internal/api/admin_reset.go`, new type `AdminResetHandler { db *bun.DB }`.

**Auth**: `auth.AuthMiddleware(db)` + `auth.AdminMiddleware()` (same as all `adminGroup` routes).

**Steps** (single `RunInTx`):
1. Cancel all non-terminal River jobs (same UPDATE as above, without `user_id` filter).
2. `DELETE FROM user_games` (all rows, all users) — cascades `user_game_platforms`, `user_game_tags`; capture count N.
3. `DELETE FROM jobs` (all) — cascades `job_items`, `sync_changes`.
4. `DELETE FROM user_sync_configs`.
5. `DELETE FROM tags`.
6. `DELETE FROM users WHERE is_admin = false` — cascades `user_sessions`, `api_keys` for non-admin users.

**Preserved**: admin account + their sessions + their API keys; all catalog tables (`games`, `platforms`, `storefronts`, `external_games`, `external_game_platforms`, `platform_storefronts`); `backup_config`.

**Response**: `{"deleted": N}` (count from step 2). Idempotent.

**Route registration** in `router.go` (inside existing `adminGroup`):
```go
adminGroup.POST("/api/auth/admin/reset", arh.HandleReset)
```

`AdminResetHandler` is constructed in `BuildRoutes` alongside other handlers and wired up there.

---

## Frontend

### Profile page — Danger Zone (`/profile`)

Add a "Danger Zone" `Card` at the bottom of `profile.tsx`, below the password section.

**Game count**: read from the existing `useCollectionStats()` hook (`data.totalGames`). The hook result is already available in other pages; add it to the profile component.

**Confirmation dialog**: `Dialog` (from shadcn/ui) rather than `AlertDialog`, because the typed-input confirmation requires controlled state for the confirm button — a pattern already established in the backup-restore dialog.

Flow:
1. "Clear Library" destructive `Button` in a red-bordered Danger Zone card.
2. Dialog opens with message: **"This will permanently remove all N games from your library. This cannot be undone."**
3. `Input` — user must type `DELETE` exactly; confirm button is disabled until the typed value matches.
4. On confirm: call `clearLibrary()`, show `toast.success('Library cleared')`, navigate to `/games`.

**New API function** in `ui/frontend/src/api/games.ts`:
```ts
export async function clearLibrary(): Promise<{ deleted: number }> {
  return api.delete('/user-games');
}
```

No new hook — use `useMutation`-style inline state within the profile component.

---

### Admin Maintenance page — Danger Zone (`/admin/maintenance`)

Add a "Danger Zone" `Card` at the bottom of `maintenance.tsx`, below the existing content.

**Two-step confirmation flow**:
1. "Reset Database" destructive `Button`.
2. First `Dialog`: **"Are you sure? This will delete all users (except you), all libraries, all sync configs, jobs, and tags. This cannot be undone."** → "Yes, continue" advances to step 2.
3. Second `Dialog`: **"Last chance. Type `RESET` to confirm."** — `Input` must equal `RESET` exactly; confirm button disabled until matched.
4. On confirm: call `resetDatabase()`, show `toast.success('Database reset complete')`, navigate to `/dashboard`.

**New API function** in `ui/frontend/src/api/admin.ts`:
```ts
export async function resetDatabase(): Promise<{ deleted: number }> {
  return api.post('/auth/admin/reset', {});
}
```

---

## Slumber collection

Add two new requests to `slumber.yaml`:

- `DELETE /api/user-games` in the `user_games/` folder (with bearer auth).
- `POST /api/auth/admin/reset` in the `admin/` folder (with bearer auth).

---

## Tests

### `TestHandleClearLibrary` (in `user_games_test.go`)

1. Seed: create a user with 3 games, 2 jobs, 1 sync config.
2. `DELETE /api/user-games` as that user → 200, `{"deleted": 3}`.
3. Assert `user_games`, `jobs`, `user_sync_configs` are empty for that user.
4. Call again (idempotent) → 200, `{"deleted": 0}`.
5. Assert another user's data is untouched.

### `TestHandleAdminReset` (in `admin_reset_test.go` — new file)

1. Seed: admin user + 2 non-admin users, each with games, jobs, sync configs; also seed tags and catalog data.
2. `POST /api/auth/admin/reset` as admin → 200.
3. Assert non-admin users are gone; admin account preserved.
4. Assert all `user_games`, `jobs`, `user_sync_configs`, `tags` are empty.
5. Assert catalog tables (`games`, `platforms`, etc.) are untouched.
6. Assert `backup_config` is untouched.
7. Call again (idempotent) → 200, `{"deleted": 0}`.
8. Non-admin user calling the endpoint → 403.

---

## Acceptance criteria (from issue)

- `DELETE /api/user-games` removes the authenticated user's entire library and associated data.
- `POST /api/auth/admin/reset` resets the DB to post-admin-creation state, preserving admin account and catalog data.
- Both return 200 + `{"deleted": N}` on success.
- Both are idempotent.
- UI requires typed confirmation (`DELETE` / `RESET`) before the action executes.
- Warning copy clearly states the action is irreversible.
