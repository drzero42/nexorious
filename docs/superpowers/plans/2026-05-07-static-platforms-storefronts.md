# Static Platforms & Storefronts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove runtime-configurable platform/storefront complexity — strip extraneous columns from the schema, move reference data into the migration, delete the seed package, remove the seeding call from setup, and copy logo SVG assets from the Python backend into the Go frontend.

**Architecture:** The migration file (`0001_initial.up.sql`) is rewritten to use the lean schema and includes all `INSERT` statements for storefronts, platforms, and associations. The `internal/seed/` package is deleted entirely. `internal/api/setup.go` drops the `SeedAll` call. Logo files are copied from the Python backend (`nexorious/backend/static/logos/`) into `ui/public/logos/` where Vite will serve them as static assets. No sqlc or generated files exist yet, so no generated code needs updating.

**Tech Stack:** Go 1.25, PostgreSQL (via pgx/v5 + golang-migrate), testcontainers-go for integration tests, Vite (serves `ui/public/` verbatim into `ui/dist/`).

---

## File Map

| Action | Path | What changes |
|--------|------|--------------|
| Copy tree | `ui/public/logos/` (new) | Platform and storefront SVGs + attribution files copied from Python backend |
| Modify | `internal/db/migrations/0001_initial.up.sql` | Rewrite `platforms`, `storefronts`, `platform_storefronts` DDL to lean schema; append `INSERT` seed data |
| Delete | `internal/seed/data.go` | Entire file removed |
| Delete | `internal/seed/seeder.go` | Entire file removed |
| Modify | `internal/api/setup.go` | Remove `seed.SeedAll()` call and `internal/seed` import |
| Modify | `internal/api/setup_test.go` | Add integration test asserting migration seed data is present |

---

## Icon filename convention

The Python backend stores files as `{slug}-icon-light.svg` and `{slug}-icon-dark.svg` inside per-slug subdirectories. The `icon` column stores the **light-variant filename only** (e.g. `steam-icon-light.svg`); the frontend derives the dark variant by substituting `-light` → `-dark`.

Three entries have **no logo files** in the Python backend and get `icon = NULL`:
- `playstation-vita`
- `playstation-psp`
- `gamersgate`

---

## Task 1: Copy logo files into ui/public/logos/

**Files:**
- Create: `ui/public/logos/` tree (platforms/ and storefronts/ subtrees)

Logo source: `/home/abo/workspace/home/nexorious/backend/static/logos/`
Logo destination: `ui/public/logos/`

Vite copies everything under `ui/public/` verbatim into `ui/dist/` at build time, so these files will be served at `/logos/...` by the Go binary with no further changes needed.

- [ ] **Step 1: Copy the entire logos tree**

```bash
cp -r /home/abo/workspace/home/nexorious/backend/static/logos /home/abo/workspace/home/nexorious-go/ui/public/logos
```

- [ ] **Step 2: Verify the copy**

```bash
find /home/abo/workspace/home/nexorious-go/ui/public/logos -type f | sort
```

Expected: 92 files (SVGs + SOURCE.txt/ATTRIBUTION.txt files).

Check that the three logo-less entries truly have no files:
```bash
ls /home/abo/workspace/home/nexorious-go/ui/public/logos/platforms/playstation-vita 2>/dev/null || echo "absent (expected)"
ls /home/abo/workspace/home/nexorious-go/ui/public/logos/platforms/playstation-psp 2>/dev/null || echo "absent (expected)"
ls /home/abo/workspace/home/nexorious-go/ui/public/logos/storefronts/gamersgate 2>/dev/null || echo "absent (expected)"
```

Expected: all three print `absent (expected)`.

- [ ] **Step 3: Commit**

```bash
git add ui/public/logos/
git commit -m "feat(ui): copy platform and storefront logo SVGs from Python backend"
```

---

## Task 2: Rewrite the migration — lean DDL + seed INSERTs

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`

The current schema has these columns to drop:
- `platforms`: `is_active`, `source`, `version_added`, `created_at`, `updated_at`; rename `icon_url` → `icon`; remove indexes `platforms_is_active_idx`, `platforms_source_idx`
- `storefronts`: `is_active`, `source`, `version_added`, `created_at`, `updated_at`; rename `icon_url` → `icon`; remove indexes `storefronts_is_active_idx`, `storefronts_source_idx`
- `platform_storefronts`: `created_at`

Also add `igdb_platform_id INTEGER` column to `platforms` (currently absent from schema, required by spec).

> **Important:** This is migration `0001` which has already run on any existing dev databases. For the purposes of this greenfield port you can rewrite it directly — there is no prod database to protect. The down migration already drops the tables, so no additional down-migration changes are needed.

- [ ] **Step 1: Replace the platforms/storefronts/platform_storefronts DDL blocks in the migration**

Replace the three table blocks (lines 66–106 of the current file) plus the default_storefront FK (lines 108–113) with the lean versions. Keep all other tables unchanged.

The new DDL section (replacing `-- Platforms table` through `-- Add FK constraint for platforms.default_storefront`):

```sql
-- Platforms table (TEXT slug as PK)
CREATE TABLE platforms (
    name               TEXT PRIMARY KEY,   -- slug: "pc-windows", "ps5", etc.
    display_name       TEXT NOT NULL,
    icon               TEXT,              -- light-variant filename, e.g. "pc-windows-icon-light.svg"; NULL if no logo
    igdb_platform_id   INTEGER,           -- nullable; IGDB's numeric platform ID
    default_storefront TEXT               -- FK → storefronts.name (added after storefronts)
);

-- Storefronts table (TEXT slug as PK)
CREATE TABLE storefronts (
    name         TEXT PRIMARY KEY,   -- slug: "steam", "epic-games-store", etc.
    display_name TEXT NOT NULL,
    icon         TEXT,              -- light-variant filename, e.g. "steam-icon-light.svg"; NULL if no logo
    base_url     TEXT
);

-- Platform-Storefront many-to-many join table
CREATE TABLE platform_storefronts (
    platform   TEXT NOT NULL REFERENCES platforms(name) ON DELETE CASCADE,
    storefront TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    PRIMARY KEY (platform, storefront)
);

-- Add FK constraint for platforms.default_storefront (deferred until after storefronts exists)
ALTER TABLE platforms
    ADD CONSTRAINT platforms_default_storefront_fkey
    FOREIGN KEY (default_storefront)
    REFERENCES storefronts(name);
```

- [ ] **Step 2: Append INSERT seed data at the end of the migration file**

After all the existing DDL and the `backup_config` INSERT (which already exists at the end of the file), append:

```sql
-- ─── Reference data: storefronts ────────────────────────────────────────────
INSERT INTO storefronts (name, display_name, icon, base_url) VALUES
    ('steam',             'Steam',             'steam-icon-light.svg',             'https://store.steampowered.com'),
    ('epic-games-store',  'Epic Games Store',  'epic-games-store-icon-light.svg',  'https://store.epicgames.com'),
    ('gog',               'GOG',               'gog-icon-light.svg',               'https://www.gog.com'),
    ('playstation-store', 'PlayStation Store', 'playstation-store-icon-light.svg', 'https://store.playstation.com'),
    ('microsoft-store',   'Microsoft Store',   'microsoft-store-icon-light.svg',   'https://www.microsoft.com/store'),
    ('nintendo-eshop',    'Nintendo eShop',    'nintendo-eshop-icon-light.svg',    'https://www.nintendo.com/us/store'),
    ('itch-io',           'Itch.io',           'itch-io-icon-light.svg',           'https://itch.io'),
    ('origin-ea-app',     'Origin/EA App',     'origin-ea-app-icon-light.svg',     'https://www.ea.com/ea-app'),
    ('apple-app-store',   'Apple App Store',   'apple-app-store-icon-light.svg',   'https://apps.apple.com'),
    ('google-play-store', 'Google Play Store', 'google-play-store-icon-light.svg', 'https://play.google.com/store'),
    ('humble-bundle',     'Humble Bundle',     'humble-bundle-icon-light.svg',     'https://www.humblebundle.com'),
    ('physical',          'Physical',          'physical-icon-light.svg',          ''),
    ('uplay',             'UPlay',             'uplay-icon-light.svg',             'https://store.ubi.com'),
    ('gamersgate',        'GamersGate',        NULL,                               'https://www.gamersgate.com');

-- ─── Reference data: platforms ──────────────────────────────────────────────
-- icon is NULL for platforms with no logo file (playstation-vita, playstation-psp)
INSERT INTO platforms (name, display_name, icon, default_storefront) VALUES
    ('pc-windows',        'PC (Windows)',               'pc-windows-icon-light.svg',        'steam'),
    ('playstation-5',     'PlayStation 5',              'playstation-5-icon-light.svg',      'playstation-store'),
    ('playstation-4',     'PlayStation 4',              'playstation-4-icon-light.svg',      'playstation-store'),
    ('playstation-3',     'PlayStation 3',              'playstation-3-icon-light.svg',      'playstation-store'),
    ('playstation-vita',  'PlayStation Vita',           NULL,                                'playstation-store'),
    ('playstation-psp',   'PlayStation Portable (PSP)', NULL,                                'playstation-store'),
    ('xbox-series',       'Xbox Series X/S',            'xbox-series-icon-light.svg',        'microsoft-store'),
    ('xbox-one',          'Xbox One',                   'xbox-one-icon-light.svg',           'microsoft-store'),
    ('xbox-360',          'Xbox 360',                   'xbox-360-icon-light.svg',           'microsoft-store'),
    ('nintendo-switch',   'Nintendo Switch',            'nintendo-switch-icon-light.svg',    'nintendo-eshop'),
    ('nintendo-wii',      'Nintendo Wii',               'nintendo-wii-icon-light.svg',       'nintendo-eshop'),
    ('ios',               'iOS',                        'ios-icon-light.svg',                'apple-app-store'),
    ('android',           'Android',                    'android-icon-light.svg',            'google-play-store'),
    ('playstation-2',     'PlayStation 2',              'playstation-2-icon-light.svg',      'physical'),
    ('playstation',       'PlayStation',                'playstation-icon-light.svg',        'physical'),
    ('nintendo-wii-u',    'Nintendo Wii U',             'nintendo-wii-u-icon-light.svg',     'nintendo-eshop'),
    ('pc-linux',          'PC (Linux)',                 'pc-linux-icon-light.svg',           'steam'),
    ('mac',               'Mac',                        'mac-icon-light.svg',                'steam'),
    ('nintendo-switch-2', 'Nintendo Switch 2',          'nintendo-switch-2-icon-light.svg',  'nintendo-eshop');

-- ─── Reference data: platform-storefront associations ───────────────────────
INSERT INTO platform_storefronts (platform, storefront) VALUES
    -- PC (Windows)
    ('pc-windows',        'steam'),
    ('pc-windows',        'epic-games-store'),
    ('pc-windows',        'gog'),
    ('pc-windows',        'origin-ea-app'),
    ('pc-windows',        'microsoft-store'),
    ('pc-windows',        'itch-io'),
    ('pc-windows',        'gamersgate'),
    ('pc-windows',        'physical'),
    -- PlayStation 5
    ('playstation-5',     'playstation-store'),
    ('playstation-5',     'physical'),
    -- PlayStation 4
    ('playstation-4',     'playstation-store'),
    ('playstation-4',     'physical'),
    -- PlayStation 3
    ('playstation-3',     'playstation-store'),
    ('playstation-3',     'physical'),
    -- PlayStation Vita
    ('playstation-vita',  'playstation-store'),
    ('playstation-vita',  'physical'),
    -- PlayStation Portable (PSP)
    ('playstation-psp',   'playstation-store'),
    ('playstation-psp',   'physical'),
    -- Xbox Series X/S
    ('xbox-series',       'microsoft-store'),
    ('xbox-series',       'physical'),
    -- Xbox One
    ('xbox-one',          'microsoft-store'),
    ('xbox-one',          'physical'),
    -- Xbox 360
    ('xbox-360',          'microsoft-store'),
    ('xbox-360',          'physical'),
    -- Nintendo Switch
    ('nintendo-switch',   'nintendo-eshop'),
    ('nintendo-switch',   'physical'),
    -- Nintendo Wii
    ('nintendo-wii',      'nintendo-eshop'),
    ('nintendo-wii',      'physical'),
    -- iOS
    ('ios',               'apple-app-store'),
    ('ios',               'epic-games-store'),
    -- Android
    ('android',           'google-play-store'),
    ('android',           'epic-games-store'),
    -- PC (Linux)
    ('pc-linux',          'steam'),
    ('pc-linux',          'gog'),
    ('pc-linux',          'humble-bundle'),
    -- PlayStation 2
    ('playstation-2',     'physical'),
    -- PlayStation
    ('playstation',       'physical'),
    -- Nintendo Wii U
    ('nintendo-wii-u',    'nintendo-eshop'),
    ('nintendo-wii-u',    'physical'),
    -- Nintendo Switch 2
    ('nintendo-switch-2', 'nintendo-eshop'),
    ('nintendo-switch-2', 'physical'),
    -- Mac
    ('mac',               'steam');
```

- [ ] **Step 3: Verify the build passes**

```bash
cd /home/abo/workspace/home/nexorious-go
go build ./...
```

Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql
git commit -m "feat(db): lean platform/storefront schema with static reference data in migration"
```

---

## Task 3: Delete internal/seed/

**Files:**
- Delete: `internal/seed/data.go`
- Delete: `internal/seed/seeder.go`

- [ ] **Step 1: Verify the seed package is only imported from setup.go**

```bash
grep -r '"github.com/drzero42/nexorious-go/internal/seed"' --include="*.go" /home/abo/workspace/home/nexorious-go
```

Expected output (only one line):
```
internal/api/setup.go:	"github.com/drzero42/nexorious-go/internal/seed"
```

If other files appear, update them in this task before deleting.

- [ ] **Step 2: Remove the seed package import and SeedAll call from setup.go**

In `internal/api/setup.go`:

Remove the import line:
```go
	"github.com/drzero42/nexorious-go/internal/seed"
```

Remove the seeding block (currently lines 100-102):
```go
	if _, seedErr := seed.SeedAll(context.Background(), h.pool); seedErr != nil {
		slog.Warn("setup admin: seed failed", "err", seedErr)
	}
```

The result: `HandleSetupAdmin` goes straight from the `tryCreateAdmin` call to `issueTokensAndSession`. No other changes to the file.

- [ ] **Step 3: Delete the seed package files**

```bash
rm /home/abo/workspace/home/nexorious-go/internal/seed/data.go
rm /home/abo/workspace/home/nexorious-go/internal/seed/seeder.go
```

- [ ] **Step 4: Verify the build passes**

```bash
cd /home/abo/workspace/home/nexorious-go
go build ./...
```

Expected: no output (success).

- [ ] **Step 5: Run all tests**

```bash
cd /home/abo/workspace/home/nexorious-go
go test ./...
```

Expected: all tests pass. The existing `TestSetupAdmin_Success` test will pass without any seeding — it makes no assertions about platform/storefront rows.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(setup): remove internal/seed package and SeedAll call — data lives in migration"
```

---

## Task 4: Verify migration data via integration test

The existing tests spin up a real PostgreSQL container via testcontainers-go and run all migrations. Add a test that asserts the migration INSERT rows actually land.

**Files:**
- Modify: `internal/api/setup_test.go`

- [ ] **Step 1: Write a new integration test**

Add this test to `internal/api/setup_test.go` (after the existing tests):

```go
func TestMigration_PlatformStorefrontSeedData(t *testing.T) {
	pool := setupAuthTestDB(t)

	var sfCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM storefronts").Scan(&sfCount); err != nil {
		t.Fatalf("count storefronts: %v", err)
	}
	if sfCount != 14 {
		t.Errorf("expected 14 storefronts from migration, got %d", sfCount)
	}

	var pfCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM platforms").Scan(&pfCount); err != nil {
		t.Fatalf("count platforms: %v", err)
	}
	if pfCount != 19 {
		t.Errorf("expected 19 platforms from migration, got %d", pfCount)
	}

	var assocCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM platform_storefronts").Scan(&assocCount); err != nil {
		t.Fatalf("count platform_storefronts: %v", err)
	}
	if assocCount != 46 {
		t.Errorf("expected 46 platform-storefront associations from migration, got %d", assocCount)
	}

	// Spot-check: pc-windows default_storefront
	var defaultSF *string
	if err := pool.QueryRow(context.Background(),
		"SELECT default_storefront FROM platforms WHERE name = 'pc-windows'").Scan(&defaultSF); err != nil {
		t.Fatalf("query pc-windows default_storefront: %v", err)
	}
	if defaultSF == nil || *defaultSF != "steam" {
		t.Errorf("expected pc-windows default_storefront='steam', got %v", defaultSF)
	}

	// Spot-check: steam icon uses light-variant filename, no path prefix
	var icon *string
	if err := pool.QueryRow(context.Background(),
		"SELECT icon FROM storefronts WHERE name = 'steam'").Scan(&icon); err != nil {
		t.Fatalf("query steam icon: %v", err)
	}
	if icon == nil || *icon != "steam-icon-light.svg" {
		t.Errorf("expected steam icon='steam-icon-light.svg', got %v", icon)
	}

	// Spot-check: platforms with no logo have NULL icon
	var vitaIcon *string
	if err := pool.QueryRow(context.Background(),
		"SELECT icon FROM platforms WHERE name = 'playstation-vita'").Scan(&vitaIcon); err != nil {
		t.Fatalf("query playstation-vita icon: %v", err)
	}
	if vitaIcon != nil {
		t.Errorf("expected playstation-vita icon=NULL, got %q", *vitaIcon)
	}
}
```

- [ ] **Step 2: Run the test**

```bash
cd /home/abo/workspace/home/nexorious-go
go test ./internal/api/... -run TestMigration_PlatformStorefrontSeedData -v
```

Expected:
```
--- PASS: TestMigration_PlatformStorefrontSeedData
```

If counts don't match, verify against the INSERT blocks in `0001_initial.up.sql`:
- Storefronts: 14 rows (steam, epic-games-store, gog, playstation-store, microsoft-store, nintendo-eshop, itch-io, origin-ea-app, apple-app-store, google-play-store, humble-bundle, physical, uplay, gamersgate)
- Platforms: 19 rows (pc-windows, playstation-5, playstation-4, playstation-3, playstation-vita, playstation-psp, xbox-series, xbox-one, xbox-360, nintendo-switch, nintendo-wii, ios, android, playstation-2, playstation, nintendo-wii-u, pc-linux, mac, nintendo-switch-2)
- Associations: 46 rows

- [ ] **Step 3: Run the full test suite**

```bash
cd /home/abo/workspace/home/nexorious-go
go test ./...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/api/setup_test.go
git commit -m "test(api): assert migration seed data for platforms and storefronts"
```

---

## Self-Review Notes

**Spec coverage check:**

| Spec requirement | Covered by |
|-----------------|-----------|
| Drop `is_active`, `source`, `version_added`, `created_at`, `updated_at` from platforms | Task 2 Step 1 |
| Rename `icon_url` → `icon` on platforms | Task 2 Step 1 |
| Remove `platforms_is_active_idx`, `platforms_source_idx` | Task 2 Step 1 (not in new DDL = absent) |
| Add `igdb_platform_id INTEGER` to platforms | Task 2 Step 1 |
| `default_storefront` FK with no explicit ON DELETE (defaults to RESTRICT) | Task 2 Step 1 |
| Drop `is_active`, `source`, `version_added`, `created_at`, `updated_at` from storefronts | Task 2 Step 1 |
| Rename `icon_url` → `icon` on storefronts | Task 2 Step 1 |
| Remove `storefronts_is_active_idx`, `storefronts_source_idx` | Task 2 Step 1 |
| Drop `created_at` from `platform_storefronts` | Task 2 Step 1 |
| Reference data (storefronts, platforms, associations) in migration INSERTs | Task 2 Step 2 |
| Logo files in `ui/public/logos/` (served by frontend) | Task 1 |
| Delete `internal/seed/data.go` and `internal/seed/seeder.go` | Task 3 Step 3 |
| Remove `seed.SeedAll()` call from setup.go | Task 3 Step 2 |
| Remove `internal/seed` import from setup.go | Task 3 Step 2 |
| No mutation sqlc queries to remove (queries dir doesn't exist yet) | n/a |
| Cancelled admin API endpoints — do not implement | n/a |
| Stats endpoints — do not implement | n/a |

**Out of scope (per spec):** Frontend React component work, `docs/superpowers/specs/` updates, sqlc query renaming. The sqlc queries directory (`internal/db/queries/`) doesn't exist yet so there is nothing to rename.
