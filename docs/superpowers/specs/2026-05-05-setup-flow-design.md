# First-Run Setup Flow — Design Spec

**Date:** 2026-05-05
**Status:** Approved

## Overview

Implements the first-run setup gate for nexorious-go. After migrations complete, if no users exist the server redirects all traffic to `/setup` until an admin is created. This is the same server-driven pattern as the migration gate — the frontend never reaches the authenticated app until setup is complete.

Scope: `needsSetup` middleware flag, `POST /api/auth/setup/admin`, seed data loader, and a standalone static setup page (same pattern as the migration UI). `POST /api/auth/setup/restore` is deferred to Phase 3.

---

## Components

### 1. `needsSetup` flag (`internal/migrate/migrator.go`)

A `needsSetup bool` protected by a `sync.RWMutex` lives on the `Migrator` struct (not a separate package). It is set once at startup and cleared by the setup handler on success.

```go
func (m *Migrator) NeedsSetup() bool          // RLock read
func (m *Migrator) SetNeedsSetup(v bool)       // Lock write
func (m *Migrator) InitNeedsSetup(ctx context.Context, pool *pgxpool.Pool) error
// InitNeedsSetup runs: SELECT COUNT(*) FROM users
// Sets needsSetup = (count == 0)
// Called from main.go immediately after RunMigrations succeeds (or on startup when already Ready)
```

`InitNeedsSetup` is called from `main.go` in two places:
- After `RunMigrations` completes (migration path)
- At startup when state is already `Ready` (already-migrated path)

**DB unavailability at startup:** If the database is unreachable when `InitNeedsSetup` is called, it must not fatally exit. Instead it retries with a backoff loop until the query succeeds or the context is cancelled (SIGTERM). Pattern:

```go
func (m *Migrator) InitNeedsSetup(ctx context.Context, pool *pgxpool.Pool) error {
    for {
        var count int
        err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
        if err == nil {
            m.SetNeedsSetup(count == 0)
            return nil
        }
        // Log at WARN — DB may still be starting up
        log.Warn().Err(err).Msg("InitNeedsSetup: DB unavailable, retrying in 2s")
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(2 * time.Second):
        }
    }
}
```

The caller (`main.go`) checks the returned error; if it is `context.Canceled` or `context.DeadlineExceeded`, it exits cleanly. Any other error is impossible under this implementation since the loop only returns on success or context cancellation.

### 2. App-state middleware (`internal/api/router.go`)

The existing middleware gets a second check added after the migration gate:

```
if migrator.State() != Ready → redirect /migrate (existing)
if migrator.NeedsSetup()    → redirect /setup   (new)
    bypass: /setup, /api/auth/setup/*, /health, /api/migrate/*
```

The bypass list for the setup gate is intentionally narrow. `/static/*` is **not** bypassed — the setup page is a self-contained static HTML file and needs no cover art or logos.

### 3. `internal/seed/` package

New package containing:
- **`data.go`** — Go slice literals for `OfficialStorefronts`, `OfficialPlatforms`, `OfficialAssociations`. These are direct ports of the Python `OFFICIAL_STOREFRONTS`, `OFFICIAL_PLATFORMS`, `PLATFORM_STOREFRONT_ASSOCIATIONS` data structures.
- **`seeder.go`** — `SeedAll(ctx, pool)` function that runs storefronts → platforms → associations in a single transaction.

The SQL uses `INSERT ... ON CONFLICT (name) DO UPDATE SET ... WHERE table.source = 'official'` — this preserves custom rows (user-created) while updating official rows if display_name, icon_url, or base_url changed. Simpler and more correct than Python's row-by-row approach.

```go
// SeedAll seeds storefronts, platforms, and platform-storefront associations.
// Idempotent: safe to call on an already-seeded database.
// Custom rows (source='custom') are never touched.
// Returns counts of rows inserted or updated per table.
func SeedAll(ctx context.Context, pool *pgxpool.Pool) (SeedResult, error)

type SeedResult struct {
    Storefronts  int
    Platforms    int
    Associations int
}
```

**Storefronts SQL:**
```sql
INSERT INTO storefronts (name, display_name, icon_url, base_url, is_active, source, version_added, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'official', $6, now(), now())
ON CONFLICT (name) DO UPDATE SET
    display_name  = EXCLUDED.display_name,
    icon_url      = EXCLUDED.icon_url,
    base_url      = EXCLUDED.base_url,
    version_added = EXCLUDED.version_added,
    updated_at    = now()
WHERE storefronts.source = 'official'
```

**Platforms SQL:** identical pattern; `default_storefront` FK is set in the same INSERT (storefronts are committed first so the FK resolves).

**Associations SQL:**
```sql
INSERT INTO platform_storefronts (platform, storefront, created_at)
VALUES ($1, $2, now())
ON CONFLICT DO NOTHING
```

All three run inside a single `pgxpool` transaction. If the transaction fails, nothing is committed.

### 4. `internal/api/setup.go`

New file alongside `auth.go`. Contains `SetupHandler` with a single method:

```go
type SetupHandler struct {
    pool     *pgxpool.Pool
    cfg      *config.Config
    migrator *migrate.Migrator  // to call SetNeedsSetup(false) on success
}

// HandleSetupAdmin handles POST /api/auth/setup/admin.
//
// Request:  {"username": string, "password": string}
// Response: 201 {"user": {...}, "access_token": "...", "refresh_token": "..."}
// Errors:   400 invalid body / validation failure
//           403 users already exist (including after a serialization-failure retry)
//           500 internal error
func (h *SetupHandler) HandleSetupAdmin(c *echo.Context) error
```

**Handler logic:**
1. Bind + validate request (`username` non-empty, `password` >= 8 chars)
2. In a serializable transaction:
   a. `SELECT COUNT(*) FROM users` — if > 0, return 403
   b. Bcrypt-hash the password (cost 12)
   c. `INSERT INTO users (id, username, password_hash, is_admin, ...)` with `uuid.NewString()`
   d. On `40001` (serialization failure): retry the transaction once. On the retry the `COUNT(*) > 0` check will catch the winner's committed row and return 403 normally.
3. Commit transaction
4. Call `seed.SeedAll(ctx, pool)` — outside the user transaction; log at WARN but do not fail if seeding errors (admin can reseed via `POST /api/platforms/seed` later)
5. Issue access token + refresh token by calling a shared helper `issueTokensAndSession(ctx, pool, cfg, userID, userAgent, ip)` — the same function called by `POST /api/auth/login`. This function generates both tokens, inserts a `user_sessions` row, and returns `(accessToken, refreshToken, error)`. Extracting it avoids duplicating the token issuance + session persistence logic between the login and setup handlers.
6. Call `h.migrator.SetNeedsSetup(false)`
7. Return 201 with user profile + tokens (auto-login — no separate `/login` round-trip needed)

**Why serializable instead of `FOR UPDATE`:** `SELECT COUNT(*) FROM users FOR UPDATE` on an empty table acquires no row locks (there are no rows to lock), so two concurrent requests can both pass the count check and both attempt the INSERT. Using a `SERIALIZABLE` transaction causes one of the two concurrent transactions to fail with a serialization failure (`40001`). The winner commits, and when the loser retries it finds `COUNT(*) > 0` and returns 403 — the correct outcome. Do **not** surface `40001` as a 500; retry the transaction once, then let the retry's 403 propagate normally.

There is no need to handle `23505` (unique violation on `username`) — that case cannot occur here because the endpoint only runs when the user table is empty, so there is no existing username to conflict with.

**Response shape:**
```json
{
  "user": {
    "id": "uuid",
    "username": "admin",
    "is_admin": true,
    "is_active": true,
    "created_at": "2026-05-05T00:00:00Z"
  },
  "access_token": "<jwt>",
  "refresh_token": "<opaque>"
}
```

The setup page JavaScript writes the tokens to `localStorage` under the key `'auth'`, matching the exact storage format used by the React SPA's `AuthProvider` (`frontend/src/providers/auth-provider.tsx`). The stored object shape must be:

```json
{
  "accessToken": "<jwt>",
  "refreshToken": "<opaque>",
  "user": {
    "id": "uuid",
    "username": "admin",
    "isAdmin": true,
    "preferences": {}
  }
}
```

Key details from the SPA source:
- Storage key: `'auth'` (constant `STORAGE_KEY` in `auth-provider.tsx`)
- All fields are **camelCase** (`accessToken`, `refreshToken`, `isAdmin`) — the SPA transforms snake_case API responses before storing
- The `user` object is the transformed shape, not the raw API response
- On page load, `AuthProvider` reads `localStorage.getItem('auth')`, validates the token via `GET /api/auth/me`, and sets up the auth context — if the key is present and valid, the user is immediately authenticated without a login prompt

The setup page must construct this object from the 201 response (transforming `is_admin` → `isAdmin`) and write it before redirecting to `/`.

### 5. Router changes (`internal/api/router.go`)

```go
// In registerRoutes:
sh := NewSetupHandler(pool, cfg, migrator)
e.GET("/setup", func(c echo.Context) error {
    // If setup is already done, redirect to the app root.
    // Mirrors the middleware logic in the other direction.
    if !migrator.NeedsSetup() {
        return c.Redirect(http.StatusFound, "/")
    }
    f, err := setupBox.Open("setup/index.html")
    if err != nil {
        return err
    }
    defer f.Close()
    return c.Stream(http.StatusOK, "text/html; charset=utf-8", f)
})
e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)
```

`ui/setup/` is a standalone directory (not part of `ui/dist/`) containing a single self-contained `index.html` with inlined CSS and JavaScript — the same approach as the migration UI (`ui/migrate/`). It is embedded via:

```go
// In ui/ui.go:
//go:embed setup
var SetupBox embed.FS
```

This avoids the problem of the React SPA's Vite-bundled assets (`/assets/*.js`) being blocked by the setup gate — the setup page has no external asset dependencies.

Setup endpoints are registered outside the JWT middleware group — they are public by design.

### 6. Static setup page (`ui/setup/index.html`)

Self-contained HTML/CSS/JS file (no build step, no external dependencies). Mirrors the migration page in structure. Behaviour:

1. Renders a username + password form.
2. On submit, `POST /api/auth/setup/admin`.
3. On **201**: constructs the `localStorage` auth object (transforming `is_admin` → `isAdmin`), writes it to `localStorage` under key `'auth'`, then redirects to `/`. The SPA's `AuthProvider` reads this on load and the user is immediately authenticated.
4. On **400**: displays the error message inline.

No changes to `ui/src/` (the React SPA) are required. The `RouteGuard` change from the original spec (removing the `GET /api/auth/setup/status` call) is still needed if that call exists in the Python frontend code being ported — confirm during Phase 2 frontend work.

### 7. Worker and scheduler startup (`cmd/nexorious/main.go`)

Workers and the gocron scheduler run in the same process as the HTTP server. They must not begin processing jobs before the database is ready and setup is complete, because scheduled tasks (metadata refresh, backup) assume at least one user exists and seed data is loaded.

**Startup gate loop:**

```go
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return // SIGTERM received before setup completed — exit cleanly
        default:
        }
        if migrator.State() == Ready && !migrator.NeedsSetup() {
            pool.Start()
            scheduler.Start()
            return
        }
        time.Sleep(2 * time.Second)
    }
}(shutdownCtx)
```

`shutdownCtx` is the same context cancelled on `SIGTERM`/`SIGINT` that the HTTP server's graceful shutdown already uses. If the operator shuts the process down before setup completes, the goroutine exits without starting workers or the scheduler — no goroutine leak.

The loop runs in a goroutine so it does not block the HTTP server (the migration and setup UI must remain responsive while the gate is spinning). The HTTP server starts immediately; only workers and scheduler are deferred.

**Startup ordering in `main.go`:**

`InitNeedsSetup` must be called **before** the HTTP server starts accepting requests. The `needsSetup` bool has a Go zero value of `false`, so if the server starts before `InitNeedsSetup` runs, the window between server start and the `SELECT COUNT(*) FROM users` completing incorrectly passes all requests through the setup gate on a fresh install.

```
Call InitNeedsSetup   ← before HTTP server starts
Start HTTP server
Spawn gate-loop goroutine(shutdownCtx) → on condition met, start workers + scheduler
```

`InitNeedsSetup` is a single fast query against a local DB and will not meaningfully delay startup.

## File Summary

| Action | File |
|--------|------|
| Modify | `internal/migrate/migrator.go` — add `needsSetup` field + `NeedsSetup()`, `SetNeedsSetup()`, `InitNeedsSetup()` |
| Modify | `internal/migrate/migrator_test.go` — tests for `InitNeedsSetup` |
| Modify | `cmd/nexorious/main.go` — call `InitNeedsSetup` after `Ready`; add gate-loop goroutine for workers + scheduler |
| Modify | `internal/api/router.go` — add setup gate to middleware; register `GET /setup` + `POST /api/auth/setup/admin` |
| Create | `internal/seed/data.go` — official seed data |
| Create | `internal/seed/seeder.go` — `SeedAll` function |
| Create | `internal/seed/seeder_test.go` — integration test (testcontainers) |
| Create | `internal/api/setup.go` — `SetupHandler` + `HandleSetupAdmin` |
| Create | `internal/api/setup_test.go` — handler tests |
| Create | `ui/setup/index.html` — standalone static setup page (no build step) |
| Modify | `ui/ui.go` — add `//go:embed setup` + `SetupBox embed.FS` |

---

## Error Handling

| Condition | Response |
|-----------|----------|
| Missing/empty username or password | 400 `{"error": "username and password are required"}` |
| Password < 8 chars | 400 `{"error": "password must be at least 8 characters"}` |
| Users already exist | 403 `{"error": "setup already complete"}` |
| PG `40001` — serialization failure (concurrent setup race), after one retry | 403 `{"error": "setup already complete"}` — the retry's count check catches the winner's row |
| DB error (other) | 500 `{"error": "internal server error"}` (logged) |
| Seed error | Logged at WARN level; 201 still returned (admin created, seeding retried via `POST /api/platforms/seed`) |

---

## Testing

**`internal/seed/seeder_test.go`** — testcontainers integration test:
- `TestSeedAll_EmptyDatabase` — verify all rows created, counts correct
- `TestSeedAll_Idempotent` — call twice, verify no duplicates, counts reflect only changed rows
- `TestSeedAll_PreservesCustomRows` — insert a custom storefront, seed, verify it is untouched

**`internal/api/setup_test.go`** — Echo httptest (real testcontainers DB):
- `TestSetupAdmin_Success` — 201, user in DB, tokens in response, `needsSetup` cleared
- `TestSetupAdmin_AlreadySetup` — 403 when user exists
- `TestSetupAdmin_InvalidBody` — 400 for missing fields
- `TestSetupAdmin_ShortPassword` — 400 for password < 8 chars
- `TestSetupPage_ServesPage` — GET /setup returns 200 with `text/html` content-type when `needsSetup=true`
- `TestSetupPage_RedirectsWhenDone` — GET /setup returns 302 to `/` when `needsSetup=false`

**`internal/migrate/migrator_test.go`** — add:
- `TestInitNeedsSetup_NoUsers` — returns true on empty DB
- `TestInitNeedsSetup_UsersExist` — returns false when users present
