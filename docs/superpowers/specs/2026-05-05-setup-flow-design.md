# First-Run Setup Flow — Design Spec

**Date:** 2026-05-05
**Status:** Approved

## Overview

Implements the first-run setup gate for nexorious-go. After migrations complete, if no users exist the server redirects all traffic to `/setup` until an admin is created. This is the same server-driven pattern as the migration gate — the frontend never reaches the authenticated app until setup is complete.

Scope: `needsSetup` middleware flag, `POST /api/auth/setup/admin`, seed data loader, and a standalone static setup page (same pattern as the migration UI). `POST /api/auth/setup/restore` is deferred to Phase 3.

---

## Components

### 0. DB probe and state additions (`internal/migrate/migrator.go`)

These additions are prerequisites for both the DB-unavailable gate and the setup gate. They belong to `migrator.go` alongside the existing state machine.

**New struct fields:**

```go
type Migrator struct {
    state              atomic.Int32
    prevState          atomic.Int32  // state saved before entering DBUnavailable; restored on recovery
    lastUnavailableAt  atomic.Value  // stores time.Time; zero if never unavailable
    needsSetup         bool
    mu                 sync.RWMutex  // guards needsSetup
    // ... existing fields
}
```

**`StartDBProbe(ctx, pool, onRecovery)`** — polls `pool.Ping()` every 5 seconds in a goroutine:

```go
func (m *Migrator) StartDBProbe(
    ctx context.Context,
    pool *pgxpool.Pool,
    onRecovery func(ctx context.Context) error,
)
```

Behaviour:
- **Ping fails** and state ≠ `DBUnavailable` → save current state to `prevState`, atomically set state to `DBUnavailable`, store `time.Now()` in `lastUnavailableAt`, log WARN.
- **Ping succeeds** and state == `DBUnavailable` → three sub-cases based on `prevState`:
  1. Migrator never initialised (state is still the zero-value `DBUnavailable` with no prior operational state) → call `onRecovery(ctx)` (`initAppState`: runs `determineState()` + `InitNeedsSetup()`); on success the callback sets the correct state; log INFO.
  2. `prevState == Migrating` → call `determineState()` directly on the existing Migrator. Do **not** blindly restore `Migrating`: the migration goroutine that was running when the DB dropped has since failed; `determineState()` re-consults the DB to find the actual current state. Log INFO.
  3. `prevState == NeedsMigration` or `prevState == Ready` → restore `prevState` directly (safe; these are stable states that cannot have changed during the outage). Then: if the restored state is `Ready` **and** `NeedsSetup()` is still `true`, call `InitNeedsSetup()` to re-check the user count. This handles the race where `POST /api/auth/setup/admin` committed the user row but the DB went unavailable before `SetNeedsSetup(false)` was called — without this check `needsSetup` would remain `true` indefinitely, blocking all non-setup routes even though an admin exists. If `InitNeedsSetup()` fails, log ERROR and remain in `DBUnavailable`. Log INFO on success.
  - If the callback or `determineState()` returns an error, log ERROR and remain in `DBUnavailable` — the probe retries on the next successful ping.
- Goroutine exits cleanly when `ctx` is cancelled (SIGTERM path).

**`LastUnavailableAt()`** — accessor read by the `GET /db-error` handler at serve time:

```go
func (m *Migrator) LastUnavailableAt() time.Time
```

Returns the zero `time.Time` if the DB has never been unavailable in this process lifetime.

`onRecovery` is supplied by `main.go` as the `initAppState` closure (see Component 7). This avoids a circular import: `migrator.go` does not need to know about `main.go`'s initialisation logic; `main.go` injects it as a callback.

---

### 1. `needsSetup` flag (`internal/migrate/migrator.go`)

A `needsSetup bool` protected by a `sync.RWMutex` lives on the `Migrator` struct (not a separate package). It is set once at startup and cleared by the setup handler on success.

```go
func (m *Migrator) NeedsSetup() bool          // RLock read
func (m *Migrator) SetNeedsSetup(v bool)       // Lock write
func (m *Migrator) InitNeedsSetup(ctx context.Context, pool *pgxpool.Pool) error
// InitNeedsSetup runs: SELECT COUNT(*) FROM users
// Sets needsSetup = (count == 0)
// Single-attempt: called only when DB is confirmed reachable (from initAppState in main.go)
```

`InitNeedsSetup` is a single-attempt call — it does **not** contain an internal retry loop. DB unavailability is handled at the state-machine level by `StartDBProbe` (Component 0). `InitNeedsSetup` is called from `initAppState()` in `main.go` in two situations:
- At startup, if `pool.Ping()` succeeds and `determineState()` resolves to `Ready` (already-migrated path)
- By the probe's `onRecovery` callback on first DB recovery, after `determineState()` runs on the existing Migrator and the state is `Ready`

**`initAppState()` pseudocode** (for clarity — lives in `main.go` as a closure over `migrator` and `pool`):

```go
func initAppState(ctx context.Context) error {
    if err := migrator.determineState(ctx, pool); err != nil {
        return err
    }
    // Only call InitNeedsSetup when migrations are already applied.
    // NeedsMigration and Migrating states have no users table yet (or it is
    // being modified), so a COUNT(*) query would fail or be meaningless.
    if migrator.State() == AppStateReady {
        if err := migrator.InitNeedsSetup(ctx, pool); err != nil {
            return err
        }
    }
    return nil
}
```

---

### 2. App-state middleware (`internal/api/router.go`)

The middleware has three sequential checks. Each gate has its own bypass list. The setup gate is the third (innermost) check:

```
// Gate 1 — DB unavailable
if migrator.State() == DBUnavailable → redirect /db-error?from=<url-encoded original path+query>
    bypass: /db-error, /health

// Gate 2 — migrations pending
if migrator.State() != Ready        → redirect /migrate
    bypass: /migrate, /api/migrate/*, /health

// Gate 3 — setup required (new)
if migrator.NeedsSetup()            → redirect /setup
    bypass: /setup, /api/auth/setup/*, /health, /api/migrate/*
```

The bypass list for the setup gate is intentionally narrow. `/static/*` is **not** bypassed — the setup page is a self-contained static HTML file and needs no cover art or logos.

`/health` is bypassed by all three gates so liveness and readiness probes always receive a machine-readable JSON response regardless of app state. The health handler inspects `migrator.State()` and returns the appropriate body (see `/health` Response Contract below).

> **`?from=` encoding (Gate 1):** The `from` value in the `/db-error` redirect must be the full original request URI (path + query string) percent-encoded as a single query parameter value. Use `url.QueryEscape(c.Request().RequestURI())` to construct the redirect target. Echo's `c.QueryParam("from")` automatically percent-decodes it when `HandleDBError` reads it back. Without encoding, a request to `/user-games?page=2&sort=title` would produce `/db-error?from=/user-games?page=2&sort=title`, where `page` and `sort` are misinterpreted as top-level query params on the `/db-error` URL — `c.QueryParam("from")` would return only `/user-games`, silently discarding the original query.

> **Gate ordering note:** A request to `/setup` while state is `NeedsMigration` hits Gate 2 first and is redirected to `/migrate` — `/setup` is not in Gate 2's bypass list. This is intentional: migrations must complete before setup can run. The user will be sent through `/migrate` → state becomes `Ready` → subsequent requests to `/` hit Gate 3 → redirected to `/setup`.

---

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

**Platforms SQL:** identical pattern; `default_storefront` FK is set in the same INSERT (storefronts are inserted earlier within the same open transaction, so the FK resolves without a separate commit — PostgreSQL resolves FKs to uncommitted rows within the same transaction).

**Associations SQL:**
```sql
INSERT INTO platform_storefronts (platform, storefront, created_at)
VALUES ($1, $2, now())
ON CONFLICT DO NOTHING
```

All three run inside a single `pgxpool` transaction. If the transaction fails, nothing is committed.

> **`SeedResult.Associations` count note:** `ON CONFLICT DO NOTHING` returns `RowsAffected() == 0` for rows that conflict and are skipped. The `Associations int` field therefore counts only newly inserted rows, not pre-existing ones. This is intentional — the field reports what changed, not what exists.

---

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
1. Bind + validate request. Validation rules (matching the Python frontend's `setup.tsx`):
   - `username`: non-empty, minimum 3 characters (the `users.username` column is `TEXT NOT NULL UNIQUE` with no DB-level length cap; the 3-char minimum is enforced by the handler)
   - `password`: minimum 8 characters
2. In a serializable transaction:
   a. `SELECT COUNT(*) FROM users` — if > 0, return 403
   b. Bcrypt-hash the password (cost 12)
   c. `INSERT INTO users (id, username, password_hash, is_admin, ...)` with `uuid.NewString()`
   d. On `40001` (serialization failure): retry the transaction once. On the retry the `COUNT(*) > 0` check will catch the winner's committed row and return 403 normally.
3. Commit transaction
4. Call `seed.SeedAll(ctx, pool)` — outside the user transaction; log at WARN but do not fail if seeding errors (admin can reseed via `POST /api/platforms/seed` later)
5. Issue access token + refresh token by calling `issueTokensAndSession(ctx, pool, cfg, userID, userAgent, ip)` — a package-level function in `internal/api/auth.go`, extracted from the `HandleLogin` body and shared with `HandleSetupAdmin`. It generates both tokens, inserts a `user_sessions` row, and returns `(accessToken, refreshToken string, error)`. Extracting it avoids duplicating the token issuance + session persistence logic between the two handlers.
   - `userAgent`: `c.Request().Header.Get("User-Agent")`
   - `ip`: `c.RealIP()` (Echo v5 helper; respects `X-Real-IP` / `X-Forwarded-For` headers). Note: `c.RealIP()` returns the correct client IP only when Echo is configured to trust the proxy's forwarding headers (e.g. `echo.IPExtractor` set to `echo.ExtractIPFromXForwardedForHeader()` or similar). On a direct connection with no proxy, it falls back to `RemoteAddr`. The `ip` field in `user_sessions` is informational; if it resolves incorrectly in a given deployment it does not affect authentication correctness.
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

`is_active` is not set explicitly in the INSERT — the `users.is_active` column defaults to `true` at the DB level. The handler relies on this default; do not set it explicitly in the INSERT statement. Return `is_active: true` as a **hardcoded literal** in the 201 response body — do not read it back from the DB via `RETURNING is_active` (the default is guaranteed for a freshly created row and the extra round-trip is unnecessary).

`preferences` is **not included** in this response — the column defaults to `'{}'::jsonb` for a newly created user, but the handler does not need to read it back from the DB and include it in the 201 body. The setup page supplies the `{}` fallback directly (see transformation below).

The setup page JavaScript writes the tokens to `localStorage` under the key `'auth'`, matching the exact storage format used by the React SPA's `AuthProvider` (`ui/src/providers/auth-provider.tsx`). The stored object shape must be:

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

Key details from the SPA source (`ui/src/providers/auth-provider.tsx`, `ui/src/types/auth.ts`):
- Storage key: `'auth'` (constant `STORAGE_KEY` in `auth-provider.tsx`)
- All fields are **camelCase**: `access_token` → `accessToken`, `refresh_token` → `refreshToken`, `is_admin` → `isAdmin`
- `is_active` and `created_at` from the API response are **not stored** — the `User` type (`ui/src/types/auth.ts`) only has `id`, `username`, `isAdmin`, and optional `preferences`
- `preferences` is a **client-side field**: `AuthProvider` stores it in auth state and the SPA reads it when rendering user settings. It is not returned by `POST /api/auth/setup/admin` (see above); the setup page must supply `preferences: {}` as an explicit constant — this is load-bearing, not just defensive. On the next page load `AuthProvider` calls `GET /api/auth/me` and overwrites the stored user with the live response (which does include `preferences`), so the empty fallback is only observed for the duration of the initial redirect.
- The `user` object is the transformed shape, not the raw API response
- On page load, `AuthProvider` reads `localStorage.getItem('auth')`, validates the token via `GET /api/auth/me`, and updates the stored user with the fresh response — if the key is present and valid, the user is immediately authenticated without a login prompt

**Complete transformation the setup page must perform:**

```js
// API 201 response (snake_case)
const { user, access_token, refresh_token } = response;

// Object written to localStorage under key 'auth'
const storedAuth = {
  accessToken: access_token,
  refreshToken: refresh_token,
  user: {
    id: user.id,
    username: user.username,
    isAdmin: user.is_admin,
    preferences: {},  // not in 201 response; GET /api/auth/me overwrites on next page load
  },
};
localStorage.setItem('auth', JSON.stringify(storedAuth));
```

The setup page must construct this object from the 201 response and write it before redirecting to `/`.

---

### 5. Router changes (`internal/api/router.go`)

The embed vars are defined in `ui/ui.go` as exported package-level vars (`ui.SetupBox`, `ui.DBErrorBox`) and referenced by the router package as `ui.SetupBox` / `ui.DBErrorBox`.

**Setup route:**

```go
// In registerRoutes (internal/api/router.go):
sh := NewSetupHandler(pool, cfg, migrator)

e.GET("/setup", func(c echo.Context) error {
    // If setup is already done, redirect to the app root.
    // Mirrors the middleware logic in the other direction.
    if !migrator.NeedsSetup() {
        return c.Redirect(http.StatusFound, "/")
    }
    f, err := ui.SetupBox.Open("setup/index.html")
    if err != nil {
        return err
    }
    defer f.Close()
    return c.Stream(http.StatusOK, "text/html; charset=utf-8", f)
})
e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)

// Phase 3 — deferred; registered now so callers get a deterministic 501 instead of 404
e.POST("/api/auth/setup/restore", func(c echo.Context) error {
    return c.JSON(http.StatusNotImplemented, map[string]string{
        "error": "not implemented — deferred to Phase 3",
    })
})
```

**DB-error route:**

```go
dh := NewDBErrorHandler(cfg, migrator)
e.GET("/db-error", dh.HandleDBError)
```

**Health route** (update existing handler):

```go
e.GET("/health", func(c echo.Context) error {
    switch migrator.State() {
    case migrate.AppStateReady:
        return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
    default:
        return c.JSON(http.StatusOK, map[string]string{"status": migrator.State().String()})
    }
})
```

All states return `200`. The `status` field carries the machine-readable state string for monitoring. See `/health` Response Contract for the full rationale.
```

The `State().String()` method returns the following strings (already implemented in `internal/migrate/migrator.go`):

| `AppState` constant | `String()` return value |
|---|---|
| `AppStateDBUnavailable` | `"db_unavailable"` |
| `AppStateNeedsMigration` | `"needs_migration"` |
| `AppStateMigrating` | `"migrating"` |
| `AppStateReady` | `"ready"` |

**Embed declarations (`ui/ui.go`):**

```go
//go:embed setup
var SetupBox embed.FS

//go:embed db-error
var DBErrorBox embed.FS
```

`ui/setup/` and `ui/db-error/` are standalone directories (not part of `ui/dist/`), each containing a single self-contained `index.html` with inlined CSS and JavaScript — the same approach as the migration UI (`ui/migrate/`). This avoids the problem of the React SPA's Vite-bundled assets (`/assets/*.js`) being blocked by the setup gate.

Setup and DB-error endpoints are registered **outside** the JWT middleware group — they are public by design.

---

### 6. `GET /db-error` handler (`internal/api/db_error.go`)

New file. Contains `DBErrorHandler`:

```go
type DBErrorHandler struct {
    migrator    *migrate.Migrator
    redactedDSN string  // computed once at construction time, reused on every serve
}

func NewDBErrorHandler(cfg *config.Config, migrator *migrate.Migrator) *DBErrorHandler
func (h *DBErrorHandler) HandleDBError(c echo.Context) error
```

**DSN redaction** — computed once in `NewDBErrorHandler` via `net/url.Parse(cfg.DatabaseURL)`:
- Keep: scheme, username, host, port, database name, non-sensitive query params (e.g. `sslmode`).
- Redact password → `***`.
- Redact any query param whose name contains `password`, `secret`, or `key` (case-insensitive) → `***`.
- Store the resulting string as `h.redactedDSN`.

Example output: `postgres://myuser:***@db.example.com:5432/nexorious?sslmode=require`

**Handler logic:**

1. If `migrator.State() != DBUnavailable` → redirect to the `?from=` parameter, or `/` if absent. **Validate that `from` starts with `/` before using it** — if it is empty, absent, or does not start with `/`, redirect to `/` instead. This prevents an open-redirect attack where a crafted link such as `/db-error?from=https://evil.com` would forward users to an external site on DB recovery.
2. Otherwise serve `ui.DBErrorBox`'s `db-error/index.html`, injecting two values at serve time (not at registration time) via `html/template` or `strings.ReplaceAll` on placeholder tokens:
   - `{{.RedactedDSN}}` → `h.redactedDSN`
   - `{{.LastUnavailableAt}}` → `h.migrator.LastUnavailableAt().UTC().Format(time.RFC3339)` (or `"unknown"` if zero)

**Auto-reload** — the static page includes:
```js
setTimeout(() => location.reload(), 5000)
```
When the auto-reload fires and the DB has recovered, the server-side redirect in step 1 sends the user back to the original path supplied in the `?from=` query parameter.

---

### 7. Static setup page (`ui/setup/index.html`)

Self-contained HTML/CSS/JS file (no build step, no external dependencies). Mirrors the migration page in structure.

**Form fields:** username, password, confirm password. Client-side validation (matching the Python frontend) before submit:
- Username ≥ 3 characters
- Password ≥ 8 characters
- Password and confirm-password must match

**Behaviour:**

1. Renders username + password + confirm-password form.
2. Validates fields client-side; displays error inline on failure (no network call made).
3. On submit, `POST /api/auth/setup/admin` with `{"username": ..., "password": ...}`.
4. On **201**: constructs the `localStorage` auth object (see transformation in Component 4), writes it under key `'auth'`, then redirects to `/`. The SPA's `AuthProvider` reads this on load and the user is immediately authenticated.
5. On **400**: displays the server error message inline.
6. On **403** (`"setup already complete"`): redirects to `/login`. This handles the edge case where a user manually navigates to `/setup` after setup is done and the middleware redirect somehow didn't fire (e.g. a direct API call).
7. On any other error: displays a generic "Setup failed, please try again" message inline.

No changes to `ui/src/` (the React SPA) are required for the setup page itself. The `RouteGuard` simplification (removing the `GET /api/auth/setup/status` call) is still needed if that call exists in the Python frontend code being ported — confirm during Phase 2 frontend work.

**`POST /api/auth/setup/restore` placeholder:** Phase 1 registers this route in Component 5 (router changes) returning `501 Not Implemented`. The route is in the setup zone (JWT-exempt, only accessible while `needsSetup=true`). This matches the pattern used by the GOG sync route and ensures the endpoint returns a deterministic 501 rather than a 404 if anything calls it before Phase 3.

---

### 8. Static DB-error page (`ui/db-error/index.html`)

Self-contained HTML/CSS/JS file (no build step, no external dependencies). Same approach as `ui/setup/index.html` and `ui/migrate/migrate.html`.

**Content:** A minimal error page communicating that the database is unreachable and the server is retrying. Two server-injected values are inserted via `html/template` placeholder tokens at serve time (not embedded literally in the file — see Component 6):

- A redacted database connection string, shown so an operator can verify the DSN without leaking the password. Displayed verbatim as pre-formatted text.
- The UTC timestamp of the last failed connection attempt, formatted as RFC 3339.

**Copy:**
```
Nexorious — Database Unavailable

The server cannot reach the database.

Connection: <redacted-dsn>
Last failed: <timestamp>

The page will automatically refresh every 5 seconds.
If this problem persists, check that your database is running and the connection
string is correct.
```

**Behaviour:**
- Displays the two injected values clearly (e.g. in a `<pre>` or labelled `<code>` block).
- Auto-reloads every 5 seconds via `setTimeout(() => location.reload(), 5000)`. When the DB recovers, the server-side redirect in `HandleDBError` (step 1 of Component 6) sends the user back to the path stored in the `?from=` query parameter — they do not land back on the error page.
- No form, no user input.

**Placeholder tokens** (replaced by `HandleDBError` at serve time):
- `{{.RedactedDSN}}` — the pre-computed redacted DSN string.
- `{{.LastUnavailableAt}}` — UTC RFC 3339 timestamp, or the literal string `"unknown"` if the DB has never been reported unavailable in this process lifetime.

---

### 9. Worker and scheduler startup (`cmd/nexorious/main.go`)

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
            workerPool.Start()   // internal/worker.Pool — not the pgxpool DB connection pool
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

`NewMigrator()` is called **before** the ping so the struct always exists when the HTTP server starts. `initAppState()` receives the already-created Migrator and only calls `determineState()` + `InitNeedsSetup()` on it — it does not create a new instance. This guarantees the middleware can safely dereference the Migrator from the first request onwards, even if the DB was unavailable at startup.

```
pgxpool.New()              ← fatal only on DSN parse error
NewMigrator()              ← struct created; state zero-values to DBUnavailable
pool.Ping():
  success → initAppState() → determineState() + InitNeedsSetup() on existing Migrator
  failure → leave state as DBUnavailable (no exit)
Start HTTP server          ← Migrator always exists; middleware is safe
StartDBProbe(shutdownCtx, pool, initAppState)  ← goroutine; calls initAppState on first recovery
Spawn worker/scheduler gate-loop goroutine(shutdownCtx)
```

---

## `/health` Response Contract

The health endpoint is bypassed by all three middleware gates — it is always reachable regardless of DB availability, migration state, or setup state. It always returns `200`:

| App state | HTTP status | Body |
|-----------|-------------|------|
| `Ready` (setup complete) | `200` | `{"status": "ok"}` |
| `Ready` + `needsSetup=true` | `200` | `{"status": "ok"}` |
| `DBUnavailable` | `200` | `{"status": "db_unavailable"}` |
| `NeedsMigration` | `200` | `{"status": "needs_migration"}` |
| `Migrating` | `200` | `{"status": "migrating"}` |

**Why always `200`:** The HTTP server is always able to serve meaningful content to the user regardless of app state — the db-error page, the migration UI, the setup page, or the authenticated app. Traffic must always be routable to the instance. A non-`200` response would cause a Kubernetes readiness probe to stop routing traffic, or a liveness probe to kill the process — both of which defeat the purpose of server-driven state pages. For example, returning `503` when the DB is unavailable would prevent users from ever seeing the `/db-error` page that explains what is wrong and auto-recovers when the DB comes back.

The `status` field in the body provides the actual state for monitoring and observability tooling without affecting traffic routing.

---

## File Summary

| Action | File |
|--------|------|
| Modify | `internal/migrate/migrator.go` — add `needsSetup` + `NeedsSetup()`, `SetNeedsSetup()`, `InitNeedsSetup()`; add `prevState atomic.Int32`, `lastUnavailableAt atomic.Value`, `LastUnavailableAt()`, `StartDBProbe()` |
| Modify | `internal/migrate/migrator_test.go` — tests for `InitNeedsSetup` and `StartDBProbe` |
| Modify | `cmd/nexorious/main.go` — `initAppState()` helper; remove fatal ping exit; `StartDBProbe` goroutine; worker/scheduler gate-loop |
| Modify | `internal/api/router.go` — add all three middleware gates; register `GET /setup`, `GET /db-error`, `POST /api/auth/setup/admin`, `POST /api/auth/setup/restore` (501 placeholder); update `/health` response contract |
| Create | `internal/seed/data.go` — official seed data |
| Create | `internal/seed/seeder.go` — `SeedAll` function |
| Create | `internal/seed/seeder_test.go` — integration test (testcontainers) |
| Create | `internal/api/setup.go` — `SetupHandler` + `HandleSetupAdmin` |
| Create | `internal/api/db_error.go` — `DBErrorHandler` + `HandleDBError`; DSN redaction + timestamp injection |
| Modify | `internal/api/auth.go` — extract `issueTokensAndSession(ctx, pool, cfg, userID, userAgent, ip) (accessToken, refreshToken string, err error)` as a package-level function shared by the login handler and `HandleSetupAdmin` |
| Create | `internal/api/setup_test.go` — handler tests |
| Create | `internal/api/db_error_test.go` — DB-error handler tests |
| Create | `ui/setup/index.html` — standalone static setup page (no build step) |
| Create | `ui/db-error/index.html` — standalone DB-unavailable error page; displays redacted DSN + last-failed-at timestamp (injected by the Go handler at serve time via `html/template`); auto-reloads every 5 s; see Component 8 for copy and placeholder tokens |
| Modify | `ui/ui.go` — add `//go:embed setup` + `var SetupBox embed.FS`; add `//go:embed db-error` + `var DBErrorBox embed.FS` |

---

## Error Handling

| Condition | Response |
|-----------|----------|
| Missing/empty username or password | 400 `{"error": "username and password are required"}` |
| Username < 3 chars | 400 `{"error": "username must be at least 3 characters"}` |
| Password < 8 chars | 400 `{"error": "password must be at least 8 characters"}` |
| Users already exist | 403 `{"error": "setup already complete"}` |
| PG `40001` — serialization failure (concurrent setup race), after one retry | 403 `{"error": "setup already complete"}` — the retry's count check catches the winner's row |
| DB error (other) | 500 `{"error": "internal server error"}` (logged) |
| Seed error | Logged at WARN level; 201 still returned (admin created, seeding retried via `POST /api/platforms/seed`) |

**Rate limiting:** Setup endpoints are **not** rate-limited. Once any user exists the endpoints return 403 permanently — they are not a viable brute-force surface. Adding rate limiting would require bootstrapping the limiter before setup is complete, which adds complexity for no practical benefit.

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
- `TestSetupAdmin_ShortUsername` — 400 for username < 3 chars
- `TestSetupAdmin_ShortPassword` — 400 for password < 8 chars
- `TestSetupAdmin_ConcurrentRace` — fire two simultaneous `POST /api/auth/setup/admin` requests; assert exactly one 201 and one 403, and exactly one user row in the DB. Verifies the SERIALIZABLE transaction correctly serializes concurrent setup attempts.
- `TestSetupPage_ServesPage` — GET /setup returns 200 with `text/html` content-type when `needsSetup=true`
- `TestSetupPage_RedirectsWhenDone` — GET /setup returns 302 to `/` when `needsSetup=false`

**`internal/api/db_error_test.go`**:
- `TestDBErrorPage_ServesHTML` — `GET /db-error` returns 200 with `text/html` when state is `DBUnavailable`; body contains redacted DSN placeholder text and a timestamp
- `TestDBErrorPage_RedirectsOnRecovery` — `GET /db-error?from=/foo` returns 302 to `/foo` when state ≠ `DBUnavailable`
- `TestDBErrorPage_RedirectsToRootWithNoFrom` — `GET /db-error` (no `?from=`) returns 302 to `/` when state ≠ `DBUnavailable`
- `TestDBErrorPage_RejectsExternalFrom` — `GET /db-error?from=https://evil.com` returns 302 to `/` (not to the external URL) when state ≠ `DBUnavailable`
- `TestDBErrorHandler_RedactsDSN` — unit test: verifies password and sensitive query params are replaced with `***`

**`internal/migrate/migrator_test.go`** — add:
- `TestInitNeedsSetup_NoUsers` — returns true on empty DB
- `TestInitNeedsSetup_UsersExist` — returns false when users present
- `TestStartDBProbe_SetsUnavailableOnPingFail` — probe sets `AppStateDBUnavailable` and saves `prevState` when ping fails
- `TestStartDBProbe_RestoresPrevStateOnRecovery` — probe restores previous state when ping succeeds after unavailability
- `TestStartDBProbe_RechecksNeedsSetupOnReadyRecovery` — when `prevState == Ready` and `needsSetup` is still `true` at recovery time, probe calls `InitNeedsSetup()`; if a user now exists (race scenario), `NeedsSetup()` returns `false` after recovery
- `TestStartDBProbe_RespectsContext` — probe goroutine exits cleanly when context is cancelled

**`internal/api/router_test.go`** (or `internal/api/setup_test.go`) — add:
- `TestDBUnavailable_RedirectsToErrorPage` — middleware returns 302 to `/db-error?from=<path>` when state is `DBUnavailable`
- `TestDBUnavailable_EncodesFromParam` — request to `/user-games?page=2&sort=title` while `DBUnavailable` produces a redirect `Location` of `/db-error?from=%2Fuser-games%3Fpage%3D2%26sort%3Dtitle`; verifies `url.QueryEscape(RequestURI())` is used rather than the bare path
- `TestHealth_OKWhenReady` — `GET /health` returns 200 + `{"status":"ok"}` when state is `Ready` and `needsSetup=false`
- `TestHealth_OKWhenSetupPending` — `GET /health` returns 200 + `{"status":"ok"}` when state is `Ready` but `needsSetup=true` (setup gate is active; health still reports `ok` because the HTTP server is fully functional)
- `TestHealth_DBUnavailableReturns200` — `GET /health` returns 200 + `{"status":"db_unavailable"}` when state is `DBUnavailable`
- `TestHealth_NeedsMigrationReturns200` — `GET /health` returns 200 + `{"status":"needs_migration"}` when state is `NeedsMigration`
