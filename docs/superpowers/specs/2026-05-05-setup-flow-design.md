# First-Run Setup Flow — Design Spec

**Date:** 2026-05-05
**Status:** Approved

## Overview

Implements the first-run setup gate for nexorious-go. After migrations complete, if no users exist the server redirects all traffic to `/setup` until an admin is created. This is the same server-driven pattern as the migration gate — the frontend never reaches the authenticated app until setup is complete.

Scope: `needsSetup` middleware flag, `POST /api/auth/setup/admin`, seed data loader, and the minimal frontend change to the `RouteGuard`. `POST /api/auth/setup/restore` is deferred to Phase 3.

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

### 2. App-state middleware (`internal/api/router.go`)

The existing middleware gets a second check added after the migration gate:

```
if migrator.State() != Ready → redirect /migrate (existing)
if migrator.NeedsSetup()    → redirect /setup   (new)
    bypass: /setup, /api/auth/setup/*, /health, /api/migrate/*
```

The bypass list for the setup gate is intentionally narrow. `/static/*` is **not** bypassed — the setup page is a simple form that needs no cover art or logos.

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
    pool    *pgxpool.Pool
    cfg     *config.Config
    migrator *migrate.Migrator  // to call SetNeedsSetup(false) on success
}

// HandleSetupAdmin handles POST /api/auth/setup/admin.
//
// Request:  {"username": string, "password": string}
// Response: 201 {"id", "username", "is_admin", "created_at"}
// Errors:   400 invalid body / validation failure
//           403 users already exist
//           500 internal error
func (h *SetupHandler) HandleSetupAdmin(c *echo.Context) error
```

**Handler logic:**
1. Bind + validate request (`username` non-empty, `password` >= 8 chars)
2. In a transaction:
   a. `SELECT COUNT(*) FROM users FOR UPDATE` — if > 0, return 403
   b. Bcrypt-hash the password (cost 12)
   c. `INSERT INTO users (id, username, password_hash, is_admin, ...)` with `uuid.NewString()`
3. Commit transaction
4. Call `seed.SeedAll(ctx, pool)` — outside the user transaction; log but do not fail if seeding errors (admin can reseed via `POST /api/platforms/seed` later)
5. Call `h.migrator.SetNeedsSetup(false)`
6. Return 201 with user profile

Step 2a uses `FOR UPDATE` to prevent a race between two simultaneous setup requests. The transaction ensures atomicity of the check + insert.

**Response shape** (matches Python `UserProfileResponse`):
```json
{
  "id": "uuid",
  "username": "admin",
  "is_admin": true,
  "is_active": true,
  "created_at": "2026-05-05T00:00:00Z"
}
```

### 5. Router changes (`internal/api/router.go`)

```go
// In registerRoutes:
sh := NewSetupHandler(pool, cfg, migrator)
e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)
// /setup catch-all falls through to spaHandler (serves index.html → React renders /setup route)
```

Setup endpoints are registered outside the JWT middleware group — they are public by design.

### 6. Frontend changes (`ui/src/`)

**Minimal.** The `RouteGuard` component no longer needs to call `GET /api/auth/setup/status`. Remove that check. The server now handles the redirect.

The `/setup` route in TanStack Router renders the setup form unconditionally. If `POST /api/auth/setup/admin` returns 403, the frontend redirects to `/login` (setup already done).

No other frontend changes are needed for this feature.

---

## File Summary

| Action | File |
|--------|------|
| Modify | `internal/migrate/migrator.go` — add `needsSetup` field + `NeedsSetup()`, `SetNeedsSetup()`, `InitNeedsSetup()` |
| Modify | `internal/migrate/migrator_test.go` — tests for `InitNeedsSetup` |
| Modify | `cmd/nexorious/main.go` — call `InitNeedsSetup` after `Ready` |
| Modify | `internal/api/router.go` — add setup gate to middleware; register setup route |
| Create | `internal/seed/data.go` — official seed data |
| Create | `internal/seed/seeder.go` — `SeedAll` function |
| Create | `internal/seed/seeder_test.go` — integration test (testcontainers) |
| Create | `internal/api/setup.go` — `SetupHandler` + `HandleSetupAdmin` |
| Create | `internal/api/setup_test.go` — handler tests |
| Modify | `ui/src/` — remove `setup/status` check from `RouteGuard` |

---

## Error Handling

| Condition | Response |
|-----------|----------|
| Missing/empty username or password | 400 `{"error": "username and password are required"}` |
| Password < 8 chars | 400 `{"error": "password must be at least 8 characters"}` |
| Users already exist | 403 `{"error": "setup already complete"}` |
| DB error | 500 `{"error": "internal server error"}` (logged) |
| Seed error | Logged at WARN level; 201 still returned (admin created, seeding retried via `/api/platforms/seed`) |

---

## Testing

**`internal/seed/seeder_test.go`** — testcontainers integration test:
- `TestSeedAll_EmptyDatabase` — verify all rows created, counts correct
- `TestSeedAll_Idempotent` — call twice, verify no duplicates, counts reflect only changed rows
- `TestSeedAll_PreservesCustomRows` — insert a custom storefront, seed, verify it is untouched

**`internal/api/setup_test.go`** — Echo httptest (real testcontainers DB):
- `TestSetupAdmin_Success` — 201, user in DB, `needsSetup` cleared
- `TestSetupAdmin_AlreadySetup` — 403 when user exists
- `TestSetupAdmin_InvalidBody` — 400 for missing fields
- `TestSetupAdmin_ShortPassword` — 400 for password < 8 chars

**`internal/migrate/migrator_test.go`** — add:
- `TestInitNeedsSetup_NoUsers` — returns true on empty DB
- `TestInitNeedsSetup_UsersExist` — returns false when users present
