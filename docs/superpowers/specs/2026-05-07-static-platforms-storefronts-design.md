# Static Platforms & Storefronts Design Spec

## Problem

The current implementation treats platforms and storefronts as runtime-configurable data: they live in DB tables populated by a Go `internal/seed` package called at first-run setup, and the original design called for a Phase 2 admin CRUD API to manage them at runtime. This is unnecessary complexity for data that changes at most a handful of times over the lifetime of the application.

## Decision

Platforms and storefronts are **static reference data**. They belong in migration `INSERT` statements, not in a seed package or admin API. To add a platform or storefront, open a PR. To retire one (rare), write a migration that cleans up `user_game_platforms` rows first.

## Schema Changes

### `platforms` — remove columns

**Remove:** `is_active`, `source`, `version_added`, `created_at`, `updated_at`

**Remove indexes:** `platforms_is_active_idx`, `platforms_source_idx`

**Final shape:**
```sql
CREATE TABLE platforms (
    name               TEXT PRIMARY KEY,   -- slug: "pc-windows", "ps5", etc.
    display_name       TEXT NOT NULL,
    icon_url           TEXT,
    default_storefront TEXT               -- FK → storefronts.name, nullable; added after storefronts
);
```

### `storefronts` — remove columns

**Remove:** `is_active`, `source`, `version_added`, `created_at`, `updated_at`

**Remove indexes:** `storefronts_is_active_idx`, `storefronts_source_idx`

**Final shape:**
```sql
CREATE TABLE storefronts (
    name         TEXT PRIMARY KEY,   -- slug: "steam", "epic-games-store", etc.
    display_name TEXT NOT NULL,
    icon_url     TEXT,
    base_url     TEXT
);
```

### `platform_storefronts` — remove `created_at`

**Remove:** `created_at`

**Final shape:**
```sql
CREATE TABLE platform_storefronts (
    platform   TEXT NOT NULL REFERENCES platforms(name) ON DELETE CASCADE,
    storefront TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    PRIMARY KEY (platform, storefront)
);
```

### Reference data goes into the migration

The `INSERT` statements for all official storefronts, platforms, and associations (currently in `internal/seed/data.go`) move into `0001_initial.up.sql` (after the table DDL). The down migration (`0001_initial.down.sql`) already drops the tables so no additional down steps are needed.

## Code Deletions

### Delete `internal/seed/` entirely

- `internal/seed/data.go`
- `internal/seed/seeder.go`

### Update `internal/api/setup.go`

Remove the `seed.SeedAll()` call and the `internal/seed` import. The `HandleSetupAdmin` function simply creates the user and issues tokens — no seeding.

### sqlc query files — remove mutation queries

From `internal/db/queries/platform_storefronts.sql`, remove:
```sql
-- name: AddPlatformStorefront :exec
-- name: RemovePlatformStorefront :exec
```
Keep only the read query:
```sql
-- name: ListStorefrontsForPlatform :many
```

From `internal/db/queries/platforms.sql`, the existing read queries are fine. No mutations were defined there.

From `internal/db/queries/storefronts.sql`, the existing read queries are fine. No mutations were defined there.

After removing queries, run `make sqlc` to regenerate `internal/db/gen/`.

### sqlc query column references

The generated sqlc types for `Platform` and `Storefront` will no longer include the removed columns. Scan any usages across the codebase and remove references to `IsActive`, `Source`, `VersionAdded`, `CreatedAt`, `UpdatedAt` on these types.

## Phase 2 API — what doesn't get built

The following admin endpoints from the original port design are **cancelled** — never implement them:

```
POST   /api/platforms
PUT    /api/platforms/:platform
DELETE /api/platforms/:platform
POST   /api/platforms/:platform/storefronts/:storefront
DELETE /api/platforms/:platform/storefronts/:storefront
PUT    /api/platforms/:platform/default-storefront
POST   /api/platforms/:platform/logo
DELETE /api/platforms/:platform/logo
GET    /api/platforms/:platform/logos
POST   /api/platforms/storefronts/
PUT    /api/platforms/storefronts/:storefront
DELETE /api/platforms/storefronts/:storefront
POST   /api/platforms/storefronts/:storefront/logo
DELETE /api/platforms/storefronts/:storefront/logo
GET    /api/platforms/storefronts/:storefront/logos
POST   /api/platforms/seed
```

The read endpoints remain (they're part of Phase 2):
```
GET  /api/platforms
GET  /api/platforms/simple-list
GET  /api/platforms/:platform
GET  /api/platforms/:platform/storefronts
GET  /api/platforms/:platform/default-storefront
GET  /api/platforms/storefronts/
GET  /api/platforms/storefronts/simple-list
GET  /api/platforms/storefronts/:storefront
GET  /api/platforms/stats
GET  /api/platforms/storefronts/stats
```

## Adding / Retiring Platforms and Storefronts

**Adding:** Create a new numbered migration (e.g. `0010_add_sega_platform.up.sql`) with the `INSERT` statement. The down migration is `DELETE FROM platforms WHERE name = 'sega-genesis'`.

**Retiring a storefront** (rare): Write a migration that:
1. Updates any `user_game_platforms` rows referencing the storefront (e.g. set `storefront = 'physical'` or use the platform's `default_storefront`)
2. Deletes from `platform_storefronts` where `storefront = 'retiring-storefront'`
3. Deletes from `storefronts` where `name = 'retiring-storefront'`

The FK constraint (`ON DELETE CASCADE` on `platform_storefronts`, `ON DELETE RESTRICT` on `user_game_platforms`) enforces the correct ordering.

**Retiring a platform** is expected to be vanishingly rare but follows the same pattern — migrate user data first, then delete.

## Affected Specs and Plans

The following existing docs were written against the old design and contain stale references to seed/admin concepts. They should not be implemented as written — the implementation spec for this change supersedes them for the platform/storefront portions:

- `docs/superpowers/plans/2026-05-05-full-initial-schema.md` — Task 3 schema and Task 5 (seed) are superseded
- `docs/superpowers/specs/2026-05-05-setup-flow-design.md` — the `internal/seed/` package section is superseded
- `docs/superpowers/plans/2026-05-06-setup-flow.md` — Task 5 (seed package) is superseded
