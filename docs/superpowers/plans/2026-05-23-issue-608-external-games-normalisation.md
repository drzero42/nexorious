# external_games Normalisation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Normalise `external_games` to one row per game, moving platform memberships into a new `external_game_platforms` table so sync counts reflect games not platform entries.

**Architecture:** Delete four incremental migrations and rewrite the baseline as a single file. The `ExternalGame` model loses `RawPlatform`; a new `ExternalGamePlatform` model is added. `DispatchSyncWorker` resolves platform slugs at sync time and writes them to `external_game_platforms`; it dispatches one `job_item` per game (not per platform). `ProcessSyncItemWorker` iterates platform rows instead of reading a single platform from metadata.

**Tech Stack:** Go, PostgreSQL, Bun ORM (`uptrace/bun`), River job queue (`riverqueue/river`), testcontainers-go

---

## File Map

| File | Action | Summary |
|---|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Rewrite | Full baseline: folds in all 4 later migrations, adds `external_game_platforms`, removes `raw_platform` from `external_games` |
| `internal/db/migrations/20260503000001_initial.down.sql` | Rewrite | Drop `external_game_platforms` before `external_games` |
| `internal/db/migrations/20260516*` | Delete | Folded into baseline |
| `internal/db/migrations/20260517*` | Delete | Folded into baseline |
| `internal/db/migrations/20260518*` | Delete | Folded into baseline |
| `internal/db/migrations/20260521*` | Delete | Folded into baseline |
| `internal/services/platformresolution/resolution.go` | Fix bug | `playstation-4`→`"ps4"` and `playstation-5`→`"ps5"` must become `"playstation-4"` and `"playstation-5"` to match `platforms.name` |
| `internal/services/platformresolution/resolution_test.go` | Update | Add PSN platform resolution tests |
| `internal/db/models/models.go` | Update | Remove `RawPlatform` from `ExternalGame`; add `ExternalGamePlatform` |
| `internal/worker/tasks/sync.go` | Major refactor | All 4 storefront cases + `ProcessSyncItemWorker` |
| `internal/worker/tasks/sync_test.go` | Major update | Rewrite 3 tests; update all `external_games` INSERTs; update metadata seeds |
| `internal/worker/tasks/main_test.go` | Update | Add `external_game_platforms` to `truncateAllTables` |

---

## Task 1: Create feature branch

- [ ] **Create branch**

```bash
git checkout -b issue-608-normalise-external-games
```

---

## Task 2: Fix platformresolution PSN slug mapping

The function currently returns `"ps4"` / `"ps5"` which do not exist in `platforms.name`. The correct canonical slugs are `"playstation-4"` / `"playstation-5"`.

**Files:**
- Modify: `internal/services/platformresolution/resolution.go`
- Modify: `internal/services/platformresolution/resolution_test.go`

- [ ] **Write failing tests**

Add to `internal/services/platformresolution/resolution_test.go`:

```go
func TestRawPlatformToSlug_PSN4(t *testing.T) {
	slug, ok := platformresolution.RawPlatformToSlug("playstation-4")
	if !ok {
		t.Fatal("expected playstation-4 to resolve")
	}
	if slug != "playstation-4" {
		t.Errorf("got %q, want %q", slug, "playstation-4")
	}
}

func TestRawPlatformToSlug_PSN5(t *testing.T) {
	slug, ok := platformresolution.RawPlatformToSlug("playstation-5")
	if !ok {
		t.Fatal("expected playstation-5 to resolve")
	}
	if slug != "playstation-5" {
		t.Errorf("got %q, want %q", slug, "playstation-5")
	}
}
```

- [ ] **Run tests to verify they fail**

```bash
go test ./internal/services/platformresolution/... -run TestRawPlatformToSlug_PSN -v
```

Expected: FAIL — tests get `"ps4"` / `"ps5"` but want `"playstation-4"` / `"playstation-5"`.

- [ ] **Fix the mapping**

In `internal/services/platformresolution/resolution.go`, change:

```go
	case "playstation-5":
		return "ps5", true
	case "playstation-4":
		return "ps4", true
```

to:

```go
	case "playstation-5":
		return "playstation-5", true
	case "playstation-4":
		return "playstation-4", true
```

- [ ] **Run tests to verify they pass**

```bash
go test ./internal/services/platformresolution/... -v
```

Expected: all PASS.

- [ ] **Commit**

```bash
git add internal/services/platformresolution/
git commit -m "fix(platformresolution): map PSN platforms to correct canonical slugs"
```

---

## Task 3: Collapse migrations into single baseline

Delete four incremental migration pairs and rewrite the baseline to include all accumulated changes plus the new `external_game_platforms` table. The baseline already has the correct `external_games` shape (unique on `user_id, storefront, external_id` without `raw_platform`).

**Files:**
- Rewrite: `internal/db/migrations/20260503000001_initial.up.sql`
- Rewrite: `internal/db/migrations/20260503000001_initial.down.sql`
- Delete: `internal/db/migrations/20260516000001_external_games_raw_platform.{up,down}.sql`
- Delete: `internal/db/migrations/20260517000001_epic_legendary_state.{up,down}.sql`
- Delete: `internal/db/migrations/20260518000001_external_games_platform_unique.{up,down}.sql`
- Delete: `internal/db/migrations/20260521000001_mac_gog_platform_storefront.{up,down}.sql`

- [ ] **Delete the four incremental migration pairs**

```bash
rm internal/db/migrations/20260516000001_external_games_raw_platform.up.sql
rm internal/db/migrations/20260516000001_external_games_raw_platform.down.sql
rm internal/db/migrations/20260517000001_epic_legendary_state.up.sql
rm internal/db/migrations/20260517000001_epic_legendary_state.down.sql
rm internal/db/migrations/20260518000001_external_games_platform_unique.up.sql
rm internal/db/migrations/20260518000001_external_games_platform_unique.down.sql
rm internal/db/migrations/20260521000001_mac_gog_platform_storefront.up.sql
rm internal/db/migrations/20260521000001_mac_gog_platform_storefront.down.sql
```

- [ ] **Rewrite `20260503000001_initial.up.sql`**

Replace the entire file with the full schema below. Key changes vs. the original:
- `external_games`: `raw_platform` column removed; unique constraint is `(user_id, storefront, external_id)`.
- `external_game_platforms`: new table.
- `user_sync_configs`: includes `epic_legendary_state jsonb` (from the deleted migration 20260517).
- `platform_storefronts`: includes `('mac', 'gog')` row (from the deleted migration 20260521).

```sql
-- Users table
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    is_admin      BOOLEAN NOT NULL DEFAULT false,
    preferences   TEXT NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_username_idx ON users (username);
CREATE INDEX users_is_active_idx ON users (is_active) WHERE is_active = true;

-- User sessions table
CREATE TABLE user_sessions (
    id                 TEXT PRIMARY KEY,
    user_id            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash         TEXT NOT NULL,
    refresh_token_hash TEXT NOT NULL,
    user_agent         TEXT,
    ip_address         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at         TIMESTAMPTZ NOT NULL
);

CREATE INDEX user_sessions_user_id_idx ON user_sessions (user_id);
CREATE INDEX user_sessions_token_hash_idx ON user_sessions (token_hash);
CREATE INDEX user_sessions_expires_at_idx ON user_sessions (expires_at);

-- Games table (IGDB catalog)
CREATE TABLE games (
    id                          INTEGER PRIMARY KEY,
    title                       TEXT NOT NULL,
    description                 TEXT,
    genre                       TEXT,
    developer                   TEXT,
    publisher                   TEXT,
    release_date                DATE,
    cover_art_url               TEXT,
    rating_average              NUMERIC(5,2),
    rating_count                INTEGER,
    howlongtobeat_main          NUMERIC(6,2),
    howlongtobeat_extra         NUMERIC(6,2),
    howlongtobeat_completionist NUMERIC(6,2),
    igdb_slug                   TEXT,
    igdb_platform_ids           TEXT,
    igdb_platform_names         TEXT,
    game_modes                  TEXT,
    themes                      TEXT,
    player_perspectives         TEXT,
    game_metadata               TEXT,
    last_updated                TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX games_title_idx ON games (title);
CREATE INDEX games_genre_idx ON games (genre);
CREATE INDEX games_developer_idx ON games (developer);
CREATE INDEX games_publisher_idx ON games (publisher);
CREATE INDEX games_release_date_idx ON games (release_date);

-- Platforms table
CREATE TABLE platforms (
    name               TEXT PRIMARY KEY,
    display_name       TEXT NOT NULL,
    icon               TEXT,
    igdb_platform_id   INTEGER,
    default_storefront TEXT
);

-- Storefronts table
CREATE TABLE storefronts (
    name         TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    icon         TEXT,
    base_url     TEXT
);

-- Platform-Storefront many-to-many join table
CREATE TABLE platform_storefronts (
    platform   TEXT NOT NULL REFERENCES platforms(name) ON DELETE CASCADE,
    storefront TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    PRIMARY KEY (platform, storefront)
);

ALTER TABLE platforms
    ADD CONSTRAINT platforms_default_storefront_fkey
    FOREIGN KEY (default_storefront)
    REFERENCES storefronts(name);

-- User games
CREATE TABLE user_games (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id         INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    play_status     TEXT,
    personal_rating INTEGER,
    is_loved        BOOLEAN NOT NULL DEFAULT false,
    hours_played    NUMERIC(10,2),
    personal_notes  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, game_id)
);

CREATE INDEX user_games_user_id_idx ON user_games (user_id);
CREATE INDEX user_games_game_id_idx ON user_games (game_id);
CREATE INDEX user_games_play_status_idx ON user_games (play_status);
CREATE INDEX user_games_is_loved_idx ON user_games (is_loved) WHERE is_loved = true;

-- User game platforms
CREATE TABLE user_game_platforms (
    id                       TEXT PRIMARY KEY,
    user_game_id             TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    platform                 TEXT NOT NULL REFERENCES platforms(name) ON DELETE RESTRICT,
    storefront               TEXT REFERENCES storefronts(name) ON DELETE RESTRICT,
    store_game_id            TEXT,
    store_url                TEXT,
    is_available             BOOLEAN NOT NULL DEFAULT true,
    hours_played             NUMERIC(10,2),
    ownership_status         TEXT,
    acquired_date            DATE,
    original_platform_name   TEXT,
    original_storefront_name TEXT,
    external_game_id         TEXT,
    sync_from_source         BOOLEAN NOT NULL DEFAULT false,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_game_platforms_uniq
    ON user_game_platforms (user_game_id, platform, storefront) NULLS NOT DISTINCT;

CREATE INDEX user_game_platforms_user_game_id_idx ON user_game_platforms (user_game_id);
CREATE INDEX user_game_platforms_platform_idx ON user_game_platforms (platform);
CREATE INDEX user_game_platforms_storefront_idx ON user_game_platforms (storefront);
CREATE INDEX user_game_platforms_external_game_id_idx ON user_game_platforms (external_game_id);

-- Tags
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX tags_user_id_idx ON tags (user_id);

-- User game tags
CREATE TABLE user_game_tags (
    id           TEXT PRIMARY KEY,
    user_game_id TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    tag_id       TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_game_id, tag_id)
);

CREATE INDEX user_game_tags_user_game_id_idx ON user_game_tags (user_game_id);
CREATE INDEX user_game_tags_tag_id_idx ON user_game_tags (tag_id);

-- External games: one row per (user_id, storefront, external_id)
CREATE TABLE external_games (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront       TEXT NOT NULL,
    external_id      TEXT NOT NULL,
    title            TEXT NOT NULL,
    resolved_igdb_id INTEGER REFERENCES games(id) ON DELETE SET NULL,
    is_skipped       BOOLEAN NOT NULL DEFAULT false,
    is_available     BOOLEAN NOT NULL DEFAULT true,
    is_subscription  BOOLEAN NOT NULL DEFAULT false,
    playtime_hours   INTEGER NOT NULL DEFAULT 0,
    ownership_status TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront, external_id)
);

CREATE INDEX external_games_user_id_idx ON external_games (user_id);
CREATE INDEX external_games_storefront_idx ON external_games (storefront);
CREATE INDEX external_games_resolved_igdb_id_idx ON external_games (resolved_igdb_id);
CREATE INDEX external_games_is_skipped_idx ON external_games (is_skipped) WHERE is_skipped = true;

-- External game platforms: one row per resolved canonical platform per game
CREATE TABLE external_game_platforms (
    id               TEXT PRIMARY KEY,
    external_game_id TEXT NOT NULL REFERENCES external_games(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(external_game_id, platform)
);

CREATE INDEX external_game_platforms_external_game_id_idx ON external_game_platforms (external_game_id);

-- Add FK from user_game_platforms to external_games
ALTER TABLE user_game_platforms
    ADD CONSTRAINT user_game_platforms_external_game_id_fkey
    FOREIGN KEY (external_game_id)
    REFERENCES external_games(id)
    ON DELETE SET NULL;

-- User sync configs
CREATE TABLE user_sync_configs (
    id                     TEXT PRIMARY KEY,
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront             TEXT NOT NULL,
    frequency              TEXT NOT NULL DEFAULT 'manual',
    auto_add               BOOLEAN NOT NULL DEFAULT false,
    storefront_credentials TEXT,
    epic_legendary_state   JSONB,
    last_synced_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront)
);

CREATE INDEX user_sync_configs_user_id_idx    ON user_sync_configs (user_id);
CREATE INDEX user_sync_configs_storefront_idx ON user_sync_configs (storefront);

-- Jobs
CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type        TEXT NOT NULL,
    source          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    priority        TEXT NOT NULL DEFAULT 'high',
    file_path       TEXT,
    total_items     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    auto_retry_done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);
CREATE INDEX jobs_user_id_idx ON jobs (user_id);
CREATE INDEX jobs_job_type_idx ON jobs (job_type);
CREATE INDEX jobs_source_idx ON jobs (source);
CREATE INDEX jobs_status_idx ON jobs (status);

-- Job items
CREATE TABLE job_items (
    id                  TEXT PRIMARY KEY,
    job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_key            TEXT NOT NULL,
    source_title        TEXT NOT NULL,
    source_metadata     JSONB NOT NULL DEFAULT '{}',
    status              TEXT NOT NULL DEFAULT 'pending',
    result              JSONB NOT NULL DEFAULT '{}',
    error_message       TEXT,
    igdb_candidates     JSONB NOT NULL DEFAULT '[]',
    resolved_igdb_id    INTEGER,
    match_confidence    NUMERIC(5,4),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at        TIMESTAMPTZ,
    resolved_at         TIMESTAMPTZ,
    UNIQUE(job_id, item_key)
);
CREATE INDEX job_items_job_id_idx ON job_items (job_id);
CREATE INDEX job_items_user_id_idx ON job_items (user_id);
CREATE INDEX job_items_status_idx ON job_items (status);

-- Backup config
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    schedule_cron   TEXT NOT NULL DEFAULT '',
    retention_mode  TEXT NOT NULL DEFAULT 'days',
    retention_value INTEGER NOT NULL DEFAULT 30,
    last_backup_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO backup_config (id, schedule_cron, retention_mode, retention_value)
VALUES (1, '0 2 * * *', 'days', 30);

-- Reference data: storefronts
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

-- Reference data: platforms
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

-- Reference data: platform-storefront associations
INSERT INTO platform_storefronts (platform, storefront) VALUES
    ('pc-windows',        'steam'),
    ('pc-windows',        'epic-games-store'),
    ('pc-windows',        'gog'),
    ('pc-windows',        'origin-ea-app'),
    ('pc-windows',        'microsoft-store'),
    ('pc-windows',        'itch-io'),
    ('pc-windows',        'gamersgate'),
    ('pc-windows',        'physical'),
    ('playstation-5',     'playstation-store'),
    ('playstation-5',     'physical'),
    ('playstation-4',     'playstation-store'),
    ('playstation-4',     'physical'),
    ('playstation-3',     'playstation-store'),
    ('playstation-3',     'physical'),
    ('playstation-vita',  'playstation-store'),
    ('playstation-vita',  'physical'),
    ('playstation-psp',   'playstation-store'),
    ('playstation-psp',   'physical'),
    ('xbox-series',       'microsoft-store'),
    ('xbox-series',       'physical'),
    ('xbox-one',          'microsoft-store'),
    ('xbox-one',          'physical'),
    ('xbox-360',          'microsoft-store'),
    ('xbox-360',          'physical'),
    ('nintendo-switch',   'nintendo-eshop'),
    ('nintendo-switch',   'physical'),
    ('nintendo-wii',      'nintendo-eshop'),
    ('nintendo-wii',      'physical'),
    ('ios',               'apple-app-store'),
    ('ios',               'epic-games-store'),
    ('android',           'google-play-store'),
    ('android',           'epic-games-store'),
    ('pc-linux',          'steam'),
    ('pc-linux',          'gog'),
    ('pc-linux',          'humble-bundle'),
    ('playstation-2',     'physical'),
    ('playstation',       'physical'),
    ('nintendo-wii-u',    'nintendo-eshop'),
    ('nintendo-wii-u',    'physical'),
    ('nintendo-switch-2', 'nintendo-eshop'),
    ('nintendo-switch-2', 'physical'),
    ('mac',               'steam'),
    ('mac',               'gog');

-- Rate limiter tokens
CREATE TABLE rate_limiter_tokens (
    key         TEXT PRIMARY KEY,
    tokens      DOUBLE PRECISION NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Rewrite `20260503000001_initial.down.sql`**

```sql
-- Drop in reverse dependency order
DROP TABLE IF EXISTS rate_limiter_tokens;
DROP TABLE IF EXISTS backup_config;
DROP TABLE IF EXISTS job_items;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS user_sync_configs;
DROP TABLE IF EXISTS user_game_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS user_game_platforms;
DROP TABLE IF EXISTS external_game_platforms;
DROP TABLE IF EXISTS external_games;
DROP TABLE IF EXISTS user_games;
DROP TABLE IF EXISTS platform_storefronts;
DROP TABLE IF EXISTS platforms;
DROP TABLE IF EXISTS storefronts;
DROP TABLE IF EXISTS games;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
```

- [ ] **Verify migrations compile (Bun auto-discovers via embed)**

```bash
go build ./...
```

Expected: no errors. (The Go code still references `RawPlatform` — it will compile because the model hasn't changed yet. The SQL file change only affects runtime.)

- [ ] **Commit**

```bash
git add internal/db/migrations/
git commit -m "chore(migrations): collapse to single baseline with external_game_platforms"
```

---

## Task 4: Update models

**Files:**
- Modify: `internal/db/models/models.go`

- [ ] **Remove `RawPlatform` from `ExternalGame` and add `ExternalGamePlatform`**

In `internal/db/models/models.go`, replace the `ExternalGame` struct and add `ExternalGamePlatform` after it:

```go
// ExternalGame mirrors the external_games table — one row per (user_id, storefront, external_id).
type ExternalGame struct {
	bun.BaseModel `bun:"table:external_games"`

	ID              string    `bun:"id,pk"                   json:"id"`
	UserID          string    `bun:"user_id,notnull"          json:"user_id"`
	Storefront      string    `bun:"storefront,notnull"       json:"storefront"`
	ExternalID      string    `bun:"external_id,notnull"      json:"external_id"`
	Title           string    `bun:"title,notnull"            json:"title"`
	ResolvedIGDBID  *int32    `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	IsSkipped       bool      `bun:"is_skipped,notnull"       json:"is_skipped"`
	IsAvailable     bool      `bun:"is_available,notnull"     json:"is_available"`
	IsSubscription  bool      `bun:"is_subscription,notnull"  json:"is_subscription"`
	PlaytimeHours   int       `bun:"playtime_hours,notnull"   json:"playtime_hours"`
	OwnershipStatus *string   `bun:"ownership_status"         json:"ownership_status"`
	CreatedAt       time.Time `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"       json:"updated_at"`

	Platforms []ExternalGamePlatform `bun:"rel:has-many,join:id=external_game_id" json:"-"`
}

// ExternalGamePlatform mirrors the external_game_platforms table.
// platform holds a canonical slug matching platforms.name.
type ExternalGamePlatform struct {
	bun.BaseModel `bun:"table:external_game_platforms"`

	ID             string    `bun:"id,pk"                    json:"id"`
	ExternalGameID string    `bun:"external_game_id,notnull" json:"external_game_id"`
	Platform       string    `bun:"platform,notnull"         json:"platform"`
	CreatedAt      time.Time `bun:"created_at,notnull"       json:"created_at"`
}
```

- [ ] **Verify compilation fails as expected (sync.go uses RawPlatform)**

```bash
go build ./... 2>&1 | grep "RawPlatform" | head -20
```

Expected: multiple errors referencing `eg.RawPlatform`, `row.RawPlatform`, etc. in `sync.go`. This is expected — Tasks 5–9 fix them.

---

## Task 5: Refactor DispatchSyncWorker — Steam case

**Files:**
- Modify: `internal/worker/tasks/sync.go`

Replace the entire `case "steam":` block. The new code:
1. Calls `GetAppDetailsPlatforms` for every game (no cache).
2. Upserts one `external_games` row (RETURNING id).
3. Upserts each resolved platform into `external_game_platforms`.
4. Deletes stale platform rows no longer returned by appdetails.
5. Dispatches one `job_item` per game, `item_key = external_id`, metadata without `raw_platform`.

- [ ] **Replace the `case "steam":` block**

```go
	case "steam":
		plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: steam credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var creds struct {
			WebAPIKey string `json:"web_api_key"`
			SteamID   string `json:"steam_id"`
		}
		if err := json.Unmarshal(plainCreds, &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
			return nil
		}
		owned, err := w.Steam.GetOwnedGames(ctx, creds.WebAPIKey, creds.SteamID)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("fetch steam library: %v", err))
			return nil
		}
		slog.Debug("dispatch_sync: steam owned games fetched", "count", len(owned), "user_id", p.UserID)

		for _, og := range owned {
			appidStr := fmt.Sprintf("%d", og.AppID)
			fetchedIDs[appidStr] = struct{}{}

			// Always fetch platforms from appdetails to keep data current.
			pl, detErr := w.Steam.GetAppDetailsPlatforms(ctx, og.AppID)
			if detErr != nil {
				if ctx.Err() != nil {
					slog.Warn("dispatch_sync: steam loop exiting early — context cancelled", "ctx_err", ctx.Err(), "appid", og.AppID, "appdetails_err", detErr, "job_id", p.JobID)
					failSyncJob(context.Background(), w.DB, p.JobID, fmt.Sprintf("sync cancelled: %v", ctx.Err()))
					return ctx.Err()
				}
				slog.Warn("steam appdetails failed, using pc-windows fallback", "appid", og.AppID, "err", detErr)
				pl = steamsvc.Platforms{Windows: true}
			}

			var resolvedPlatforms []string
			if pl.Windows {
				resolvedPlatforms = append(resolvedPlatforms, "pc-windows")
			}
			if pl.Mac {
				resolvedPlatforms = append(resolvedPlatforms, "mac")
			}
			if pl.Linux {
				resolvedPlatforms = append(resolvedPlatforms, "pc-linux")
			}
			if len(resolvedPlatforms) == 0 {
				resolvedPlatforms = []string{"pc-windows"}
			}

			ownership := "owned"
			upsertNow := time.Now().UTC()

			var egID string
			if err := w.DB.NewRaw(`
				INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, true, false, ?, ?, ?, ?)
				ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
					title = EXCLUDED.title,
					playtime_hours = EXCLUDED.playtime_hours,
					is_subscription = EXCLUDED.is_subscription,
					ownership_status = EXCLUDED.ownership_status,
					is_available = true,
					updated_at = now()
				RETURNING id`,
				uuid.NewString(), p.UserID, p.Storefront, appidStr, og.Title,
				og.PlaytimeHours, &ownership, upsertNow, upsertNow,
			).Scan(ctx, &egID); err != nil {
				slog.Error("dispatch_sync: steam upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
				continue
			}

			for _, platform := range resolvedPlatforms {
				if _, err := w.DB.NewRaw(`
					INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
					VALUES (?, ?, ?, now())
					ON CONFLICT (external_game_id, platform) DO NOTHING`,
					uuid.NewString(), egID, platform,
				).Exec(ctx); err != nil {
					slog.Error("dispatch_sync: steam upsert platform failed", "err", err, "job_id", p.JobID, "external_id", appidStr, "platform", platform)
				}
			}

			if _, err := w.DB.NewRaw(`
				DELETE FROM external_game_platforms
				WHERE external_game_id = ? AND platform NOT IN (?)`,
				egID, bun.In(resolvedPlatforms),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: steam delete stale platforms failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
			}
		}

		var toProcess []models.ExternalGame
		if err := w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx); err != nil {
			slog.Error("dispatch_sync: steam query to-process failed", "err", err, "job_id", p.JobID)
		}
		slog.Debug("dispatch_sync: steam to-process count", "count", len(toProcess), "job_id", p.JobID)
		for _, eg := range toProcess {
			metaJSON, _ := json.Marshal(map[string]any{
				"external_game_id": eg.ID,
				"playtime_hours":   eg.PlaytimeHours,
			})
			itemID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: steam insert job_item failed", "err", err, "job_id", p.JobID, "external_id", eg.ExternalID)
			}
			if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, ProcessSyncItemArgs{JobItemID: itemID}); err != nil {
				slog.Error("dispatch_sync: steam enqueue failed", "err", err, "job_id", p.JobID, "item_id", itemID)
			}
		}
```

- [ ] **Verify the file compiles (other cases still have RawPlatform errors)**

```bash
go build ./internal/worker/tasks/... 2>&1 | grep -v "RawPlatform\|raw_platform" | grep "error:" | head -10
```

Expected: only `RawPlatform`-related errors remain from the PSN, Epic, GOG cases.

---

## Task 6: Refactor DispatchSyncWorker — PSN case

**Files:**
- Modify: `internal/worker/tasks/sync.go`

Replace the entire `case "psn":` block. PSN entries each have exactly one platform. The code upserts one `external_games` row per TitleID and one `external_game_platforms` row. No platform reconciliation needed (each TitleID is tied to one platform permanently). `item_key = external_id` (unchanged from before).

- [ ] **Replace the `case "psn":` block**

```go
	case "psn":
		plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: psn credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var psnCreds struct {
			NpssoToken string `json:"npsso_token"`
			IsVerified bool   `json:"is_verified"`
		}
		if err := json.Unmarshal(plainCreds, &psnCreds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
			return nil
		}
		if !psnCreds.IsVerified {
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}

		slog.Info("dispatch_sync: starting psn library fetch", "job_id", p.JobID, "user_id", p.UserID)
		if err := w.PSN.GetLibrary(ctx, psnCreds.NpssoToken, psnLibraryBatchSize,
			func(batch []psnsvc.ExternalGameEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: psn batch received", "job_id", p.JobID, "batch_size", len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)

					platform, ok := platformresolution.RawPlatformToSlug(e.RawPlatform)
					if !ok {
						slog.Error("dispatch_sync: psn unknown platform, using default", "raw_platform", e.RawPlatform, "external_id", e.ExternalID)
						platform = "playstation-4"
					}

					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							playtime_hours = EXCLUDED.playtime_hours,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, e.PlaytimeHours, &ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: psn upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
						VALUES (?, ?, ?, now())
						ON CONFLICT (external_game_id, platform) DO NOTHING`,
						uuid.NewString(), egID, platform,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: psn upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: psn re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: psn batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess), "batch_size", len(batch))

				for _, eg := range toProcess {
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"playtime_hours":   eg.PlaytimeHours,
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: psn insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: psn river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			if errors.Is(err, psnsvc.ErrInvalidNPSSOToken) {
				expiredAt := time.Now().UTC()
				newCreds := map[string]any{
					"npsso_token":      psnCreds.NpssoToken,
					"is_verified":      false,
					"token_expired_at": expiredAt,
				}
				if b, merr := json.Marshal(newCreds); merr == nil {
					enc, encErr := w.Encrypter.Encrypt(b)
					if encErr != nil {
						slog.Error("dispatch_sync: encrypt expired psn token failed", "err", encErr, "job_id", p.JobID)
					} else if _, err := w.DB.NewRaw(
						`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
						enc, p.UserID,
					).Exec(context.Background()); err != nil {
						slog.Error("dispatch_sync: persist expired psn token failed", "err", err, "job_id", p.JobID)
					}
				}
				failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			} else {
				slog.Error("dispatch_sync: psn library fetch failed", "job_id", p.JobID, "err", err)
				failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("psn_fetch_error: %v", err))
			}
			return nil
		}
```

Note: `continue` inside a `range` over a closure is not valid Go. Replace `continue` with `return` inside the batch closure. Actually, the `continue` above is inside a `for _, e := range batch` loop within the closure — that is valid. The compiler will accept it.

- [ ] **Check compilation progress**

```bash
go build ./internal/worker/tasks/... 2>&1 | grep "error:" | head -10
```

Expected: only GOG and Epic `RawPlatform` errors remain.

---

## Task 7: Refactor DispatchSyncWorker — GOG case

**Files:**
- Modify: `internal/worker/tasks/sync.go`

GOG sends one entry per platform per game (same external_id can appear twice — once for Windows, once for Linux). The new code:
1. Accumulates `seenEGPlatforms map[string][]string` (external_game_id → platforms seen) across all batches.
2. After the stream, reconciles each game's platform rows (deletes stale ones).
3. `item_key = external_id` (was `external_id + ":" + raw_platform`).

- [ ] **Replace the `case "gog":` block**

```go
	case "gog":
		if w.GOG == nil {
			failSyncJob(ctx, w.DB, p.JobID, "GOG sync not available")
			return nil
		}

		plainGOGCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
		if err != nil {
			slog.Warn("dispatch_sync: gog credentials decrypt failed", "user_id", p.UserID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
			return nil
		}
		var creds struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			UserID       string `json:"user_id"`
			Username     string `json:"username"`
		}
		if err := json.Unmarshal(plainGOGCreds, &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid gog credentials")
			return nil
		}

		newTok, err := w.GOG.RefreshToken(ctx, creds.RefreshToken)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("gog token refresh failed: %v", err))
			return nil
		}
		creds.AccessToken = newTok.AccessToken
		creds.RefreshToken = newTok.RefreshToken
		if newCredsJSON, merr := json.Marshal(creds); merr == nil {
			enc, encErr := w.Encrypter.Encrypt(newCredsJSON)
			if encErr != nil {
				slog.Error("dispatch_sync: encrypt refreshed gog token failed", "err", encErr, "job_id", p.JobID)
			} else {
				if _, err := w.DB.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
					enc, p.UserID,
				).Exec(context.Background()); err != nil {
					slog.Error("dispatch_sync: persist refreshed gog token failed", "err", err, "job_id", p.JobID)
				}
			}
		}

		// seenEGPlatforms tracks which canonical platform slugs were seen per
		// external_game_id across all batches, for end-of-stream reconciliation.
		seenEGPlatforms := make(map[string][]string)

		slog.Info("dispatch_sync: starting gog library fetch", "job_id", p.JobID, "user_id", p.UserID)
		const gogBatchSize = 50
		if err := w.GOG.GetLibrary(ctx, creds.AccessToken, gogBatchSize,
			func(batch []gogsvc.ExternalGameEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: gog batch received", "job_id", p.JobID, "batch_size", len(batch))

				// dispatchedInBatch deduplicates job_item dispatch within this batch.
				dispatchedInBatch := make(map[string]struct{})

				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}

					platform, ok := platformresolution.RawPlatformToSlug(e.RawPlatform)
					if !ok {
						slog.Error("dispatch_sync: gog unknown platform, using default", "raw_platform", e.RawPlatform, "external_id", e.ExternalID)
						platform = "pc-windows"
					}

					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							playtime_hours = EXCLUDED.playtime_hours,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, e.PlaytimeHours, &ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: gog upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					seenEGPlatforms[egID] = append(seenEGPlatforms[egID], platform)

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
						VALUES (?, ?, ?, now())
						ON CONFLICT (external_game_id, platform) DO NOTHING`,
						uuid.NewString(), egID, platform,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: gog upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}

					if _, alreadyDispatched := dispatchedInBatch[e.ExternalID]; alreadyDispatched {
						continue
					}
					dispatchedInBatch[e.ExternalID] = struct{}{}
				}

				// Re-query this batch's unique external_ids to get DB state (is_skipped, id).
				batchExtIDs := make([]string, 0, len(dispatchedInBatch))
				for extID := range dispatchedInBatch {
					batchExtIDs = append(batchExtIDs, extID)
				}
				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: gog re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: gog batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess))

				for _, eg := range toProcess {
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"playtime_hours":   eg.PlaytimeHours,
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: gog insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: gog river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			slog.Error("dispatch_sync: gog library fetch failed", "job_id", p.JobID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("gog_fetch_error: %v", err))
			return nil
		}

		// Reconcile: delete platform rows no longer present in the upstream library.
		for egID, platforms := range seenEGPlatforms {
			if _, err := w.DB.NewRaw(`
				DELETE FROM external_game_platforms
				WHERE external_game_id = ? AND platform NOT IN (?)`,
				egID, bun.In(platforms),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: gog delete stale platforms failed", "err", err, "job_id", p.JobID, "external_game_id", egID)
			}
		}
```

---

## Task 8: Refactor DispatchSyncWorker — Epic case

**Files:**
- Modify: `internal/worker/tasks/sync.go`

Epic always uses `pc-windows`. The upsert pattern is the same as the other storefronts. No platform reconciliation needed.

- [ ] **Replace the `case "epic":` block**

```go
	case "epic":
		if w.Epic == nil {
			failSyncJob(ctx, w.DB, p.JobID, "Epic sync not configured (LEGENDARY_WORK_DIR unset)")
			return nil
		}
		slog.Info("dispatch_sync: starting epic library fetch", "job_id", p.JobID, "user_id", p.UserID)
		if err := w.Epic.GetLibrary(ctx, p.UserID,
			func(batch []epicsvc.ExternalGameEntry) error {
				if len(batch) == 0 {
					return nil
				}
				slog.Info("dispatch_sync: epic batch received", "job_id", p.JobID, "batch_size", len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)

					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()

					var egID string
					if err := w.DB.NewRaw(`
						INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, false, 0, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						&ownership, upsertNow, upsertNow,
					).Scan(ctx, &egID); err != nil {
						slog.Error("dispatch_sync: epic upsert external_game failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
						continue
					}

					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
						VALUES (?, ?, 'pc-windows', now())
						ON CONFLICT (external_game_id, platform) DO NOTHING`,
						uuid.NewString(), egID,
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: epic upsert platform failed", "job_id", p.JobID, "external_id", e.ExternalID, "err", err)
					}
				}

				var toProcess []models.ExternalGame
				if err := w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.List(batchExtIDs)).
					Scan(ctx); err != nil {
					slog.Error("dispatch_sync: epic re-query batch failed", "job_id", p.JobID, "err", err)
				}
				slog.Info("dispatch_sync: epic batch to dispatch", "job_id", p.JobID, "to_process", len(toProcess), "batch_size", len(batch))

				for _, eg := range toProcess {
					metaJSON, _ := json.Marshal(map[string]any{
						"external_game_id": eg.ID,
						"playtime_hours":   eg.PlaytimeHours,
					})
					itemID := uuid.NewString()
					if _, err := w.DB.NewRaw(`
						INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx); err != nil {
						slog.Error("dispatch_sync: epic insert job_item failed", "job_id", p.JobID, "external_id", eg.ExternalID, "err", err)
					}
					if w.RiverClient != nil {
						if _, err := w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
							slog.Error("dispatch_sync: epic river insert failed", "job_id", p.JobID, "item_id", itemID, "err", err)
						}
					}
				}
				return nil
			},
		); err != nil {
			slog.Error("dispatch_sync: epic library fetch failed", "job_id", p.JobID, "err", err)
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("epic_fetch_error: %v", err))
			return nil
		}
```

- [ ] **Verify full compilation**

```bash
go build ./...
```

Expected: no errors. All `RawPlatform` references have been removed from the worker switch statement.

- [ ] **Commit tasks 4–8**

```bash
git add internal/db/models/models.go internal/worker/tasks/sync.go
git commit -m "refactor(sync): normalise external_games — one row per game, platform rows in external_game_platforms"
```

---

## Task 9: Update ProcessSyncItemWorker

**Files:**
- Modify: `internal/worker/tasks/sync.go`

Changes:
- `meta` struct: remove `RawPlatform`.
- Step 7: load `external_game_platforms`, iterate them; no `RawPlatformToSlug` call needed.

- [ ] **Update the meta struct and step 7 in `ProcessSyncItemWorker.Work`**

Find and replace the `meta` struct and step 7. The full updated `Work` method keeps steps 1–6 and 8–10 identical to before; only the meta struct and step 7 change.

**Replace the meta struct (step 2):**

```go
	// ── 2. Parse source_metadata ──────────────────────────────────────────
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
		PlaytimeHours  int    `json:"playtime_hours"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
```

**Replace step 7 (was "Resolve platform and storefront slugs") with the new "Resolve storefront + iterate platforms":**

Delete the old step 7:
```go
	// ── 7. Resolve platform and storefront slugs ──────────────────────────
	platformSlug, platformOK := platformresolution.RawPlatformToSlug(meta.RawPlatform)
	storefrontSlug, storefrontOK := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !platformOK || !storefrontOK {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved platform=%s storefront=%s", meta.RawPlatform, eg.Storefront))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
```

And delete the old step 9 ("Find or create user_game_platform") which references `platformSlug` and `meta.RawPlatform`.

Replace both with:

```go
	// ── 7. Load platform rows ─────────────────────────────────────────────
	var egPlatforms []models.ExternalGamePlatform
	if err := w.DB.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("load platforms: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
	if len(egPlatforms) == 0 {
		syncMarkItemFailed(ctx, w.DB, &item, "external game has no platform rows")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	storefrontSlug, storefrontOK := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !storefrontOK {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved storefront=%s", eg.Storefront))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
```

**Replace step 8 ("Find or create user_game")** — keep it exactly as before but update `meta.PlaytimeHours` reference (it still exists in the new meta struct).

**Replace step 9 ("Find or create user_game_platform")** — now loops over platforms:

```go
	// ── 9. Find or create user_game_platform for each platform ───────────
	ownership := ""
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	} else {
		ownership = "owned"
	}
	hoursPlayed := float64(eg.PlaytimeHours)

	for _, egp := range egPlatforms {
		var existingUGPID string
		var existingOwnership *string
		ugpErr := w.DB.NewRaw(
			`SELECT id, ownership_status FROM user_game_platforms WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
			ugID, egp.Platform, storefrontSlug,
		).Scan(ctx, &existingUGPID, &existingOwnership)

		if errors.Is(ugpErr, sql.ErrNoRows) || ugpErr != nil {
			ugpID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO user_game_platforms
				(id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
				 original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
				VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
				ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				ugpID, ugID, egp.Platform, storefrontSlug, hoursPlayed, ownership,
				egp.Platform, eg.Storefront, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: insert user_game_platform failed", "err", err, "job_item_id", p.JobItemID, "platform", egp.Platform)
			}
		} else {
			existingRank := 0
			if existingOwnership != nil {
				existingRank = ownershipRank(*existingOwnership)
			}
			if ownershipRank(ownership) > existingRank {
				if _, err := w.DB.NewRaw(
					`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
					ownership, hoursPlayed, existingUGPID,
				).Exec(ctx); err != nil {
					slog.Error("process_sync_item: update ugp ownership failed", "err", err, "job_item_id", p.JobItemID)
				}
			} else {
				if _, err := w.DB.NewRaw(
					`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
					hoursPlayed, existingUGPID,
				).Exec(ctx); err != nil {
					slog.Error("process_sync_item: update ugp playtime failed", "err", err, "job_item_id", p.JobItemID)
				}
			}
		}
	}
```

- [ ] **Verify compilation**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "refactor(sync): ProcessSyncItemWorker iterates external_game_platforms rows"
```

---

## Task 10: Update tests

**Files:**
- Modify: `internal/worker/tasks/main_test.go`
- Modify: `internal/worker/tasks/sync_test.go`

### Step 10a — Add `external_game_platforms` to `truncateAllTables`

- [ ] **Update `truncateAllTables` in `main_test.go`**

```go
func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		TRUNCATE TABLE
			users, user_sessions, games, external_games, external_game_platforms,
			platforms, storefronts, platform_storefronts,
			tags, user_games, user_game_tags, user_game_platforms,
			jobs, job_items, river_job, backup_config,
			user_sync_configs, rate_limiter_tokens
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncateAllTables: %v", err)
	}
}
```

### Step 10b — Helper for seeding an external_game_platforms row

Many tests need to seed both tables. Add a helper at the bottom of `sync_test.go`:

- [ ] **Add helper `insertTestExternalGame` to `sync_test.go`**

```go
// insertTestExternalGame inserts a minimal external_games row and one
// external_game_platforms row. Returns the external_game id.
func insertTestExternalGame(t *testing.T, userID, storefront, externalID, title, platform string) string {
	t.Helper()
	egID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, ?, ?, ?, false, true, false, 0)`,
		egID, userID, storefront, externalID, title,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGame: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
		 VALUES (?, ?, ?, now())`,
		uuid.NewString(), egID, platform,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGamePlatform: %v", err)
	}
	return egID
}
```

### Step 10c — Rewrite DispatchSync Steam tests

- [ ] **Rewrite `TestDispatchSync_Steam_MultiPlatform_WindowsAndLinux`**

```go
func TestDispatchSync_Steam_MultiPlatform_WindowsAndLinux(t *testing.T) {
	// appdetails reports {Windows, Linux} for appid 730 →
	// expect 1 external_games row, 2 external_game_platforms rows, 1 job_item keyed "730".
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 730, Title: "Counter-Strike 2", PlaytimeHours: 100},
		},
		platformsByAppID: map[int]steamsvc.Platforms{
			730: {Windows: true, Linux: true},
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND external_id = '730'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 1 {
		t.Errorf("expected 1 external_games row, got %d", egCount)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '730'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("expected 2 external_game_platforms rows (Windows+Linux), got %d", egpCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item, got %d", itemCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "730" {
		t.Errorf("expected item_key=730, got %q", itemKey)
	}
}
```

- [ ] **Replace `TestDispatchSync_Steam_CacheHit_SkipsAppDetails` with `TestDispatchSync_Steam_PlatformUpdate_AddsNewPlatform`**

```go
func TestDispatchSync_Steam_PlatformUpdate_AddsNewPlatform(t *testing.T) {
	// Pre-seed game 999 with only pc-windows. Second sync returns {Windows, Linux}.
	// Worker must add the pc-linux platform row.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	// Pre-seed with Windows only.
	insertTestExternalGame(t, userID, "steam", "999", "Cached Game", "pc-windows")

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 999, Title: "Cached Game", PlaytimeHours: 5},
		},
		platformsByAppID: map[int]steamsvc.Platforms{
			999: {Windows: true, Linux: true},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Appdetails must have been called.
	if len(adapter.queriedAppIDs) == 0 || adapter.queriedAppIDs[0] != 999 {
		t.Errorf("expected GetAppDetailsPlatforms to be called for appid 999, got %v", adapter.queriedAppIDs)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.external_id = '999'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("expected 2 platform rows (windows+linux) after update, got %d", egpCount)
	}
}
```

- [ ] **Rewrite `TestDispatchSync_Steam_AppDetailsFailure_FallsBackToWindows`**

```go
func TestDispatchSync_Steam_AppDetailsFailure_FallsBackToWindows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 888, Title: "Rate Limited Game", PlaytimeHours: 0},
		},
		platformErrByAppID: map[int]error{
			888: errors.New("steam appdetails HTTP 429 for appid 888"),
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var platform string
	_ = testDB.NewRaw(
		`SELECT egp.platform FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '888'`,
		userID,
	).Scan(ctx, &platform)
	if platform != "pc-windows" {
		t.Errorf("expected pc-windows fallback platform, got %q", platform)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item after appdetails fallback, got %d", itemCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "888" {
		t.Errorf("expected item_key=888, got %q", itemKey)
	}
}
```

- [ ] **Update `TestDispatchSync_Steam_NoPlatformsFallback_EmitsWindowsRow`**

The `raw_platform` column query must change to query `external_game_platforms`:

```go
	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.storefront = 'steam' AND eg.external_id = '777' AND egp.platform = 'pc-windows'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 1 {
		t.Errorf("expected 1 pc-windows platform row for no-platforms fallback, got %d", egpCount)
	}

	var itemKey string
	_ = testDB.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemKey)
	if itemKey != "777" {
		t.Errorf("expected item_key=777, got %q", itemKey)
	}
```

### Step 10d — Rewrite GOG tests

- [ ] **Rewrite `TestGOGDispatch_DualPlatformCreatesTwoRows`**

```go
func TestGOGDispatch_DualPlatformCreatesTwoRows(t *testing.T) {
	// Same external_id with two platform entries → 1 external_games row,
	// 2 external_game_platforms rows, 1 job_item.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'pending', 'low')`,
		jobID, userID,
	).Exec(ctx)
	rawCreds := `{"access_token":"acc","refresh_token":"ref","user_id":"u1","username":"user"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'gog', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	adapter := &fakeGOGAdapter{
		entries: []gogsvc.ExternalGameEntry{
			{ExternalID: "2001", Title: "Dual Game", RawPlatform: "pc-windows", OwnershipStatus: "owned"},
			{ExternalID: "2001", Title: "Dual Game", RawPlatform: "pc-linux", OwnershipStatus: "owned"},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, GOG: adapter}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"},
	}
	_ = w.Work(ctx, job)

	var egCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'gog' AND external_id = '2001'`,
		userID,
	).Scan(ctx, &egCount)
	if egCount != 1 {
		t.Errorf("want 1 external_game for dual-platform game, got %d", egCount)
	}

	var egpCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM external_game_platforms egp
		 JOIN external_games eg ON eg.id = egp.external_game_id
		 WHERE eg.user_id = ? AND eg.external_id = '2001'`,
		userID,
	).Scan(ctx, &egpCount)
	if egpCount != 2 {
		t.Errorf("want 2 external_game_platforms rows, got %d", egpCount)
	}

	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("want 1 job_item (one per game, not per platform), got %d", itemCount)
	}
}
```

### Step 10e — Update PSNSuccess_SkippedGameExcluded test

- [ ] **Remove `raw_platform` from the pre-seeded `external_games` INSERT and add a platform row**

Replace:
```go
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, raw_platform, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'NPWR00001_00', 'God of War', 'playstation-4', true, true, false, 0)`,
		uuid.NewString(), userID,
	).Exec(ctx)
```

With:
```go
	egID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'NPWR00001_00', 'God of War', true, true, false, 0)`,
		egID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, created_at) VALUES (?, ?, 'playstation-4', now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
```

### Step 10f — Update all ProcessSyncItem tests

Each of these tests inserts an `external_games` row with `raw_platform` and seeds a `job_items` row with metadata containing `"raw_platform"`. Both must be updated.

The pattern for every affected test:

**Old external_games INSERT (remove `raw_platform`):**
```go
// OLD:
`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
 VALUES (?, ?, 'steam', 'app1', 'Test Game', false, true, false, 0)`
// (raw_platform column removed — no change needed if it wasn't in the INSERT)
// If it WAS included:
`INSERT INTO external_games (id, user_id, storefront, external_id, title, raw_platform, is_skipped, is_available, is_subscription, playtime_hours)
 VALUES (?, ?, ...)` → remove `raw_platform` and its value
```

**Add external_game_platforms row after each external_games INSERT:**
```go
_, _ = testDB.NewRaw(
    `INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
     VALUES (?, ?, 'pc-windows', now())`,
    uuid.NewString(), egID,
).Exec(ctx)
```

**Old job_item metadata (remove `raw_platform`):**
```go
// OLD:
metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "pc-windows"})
// NEW:
metaJSON, _ := json.Marshal(map[string]any{"external_game_id": egID, "playtime_hours": 0})
```

- [ ] **Apply the pattern to every test that seeds external_games + job_items directly**

Affected tests (search for `INSERT INTO external_games` in sync_test.go to find them all):
- `TestProcessSyncItem_SkippedExternalGame` — add platform row, update metadata
- `TestProcessSyncItem_NoIGDBID_PendingReview` — add platform row, update metadata
- `TestProcessSyncItem_WithResolvedIGDBID_Completed` — add platform row (`pc-windows`), update metadata
- `TestProcessSyncItem_UnresolvedPlatform_Failed` — rewrite: instead of seeding `raw_platform = "unknown-platform"`, do NOT seed any `external_game_platforms` row, and change assertion to expect `status = 'failed'` with message about no platform rows. Also rename to `TestProcessSyncItem_NoPlatforms_Failed`.
- `TestProcessSyncItem_WithIGDBAutoResolve` — add platform row, update metadata
- `TestProcessSyncItem_LowConfidenceIGDB_StoresMatchConfidence` — add platform row, update metadata
- `TestProcessSyncItem_IGDBPrefixTitle_AutoResolves` — add platform row (`pc-windows`), update metadata
- `TestProcessSyncItem_ManualResolution_DoesNotRevertToPendingReview` — add platform row (`pc-windows`), update metadata
- `TestProcessSyncItem_CrossSKU_InheritsResolutionFromSibling` — add platform rows for BOTH external_games inserts, update metadata for both job_items
- `TestProcessSyncItem_IGDBError_MarksItemIGDBFailed` — add platform row, update metadata
- `TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors` — add platform row (`pc-windows`), update metadata
- `TestProcessSyncItem_AutoRetry_NilRiverClient_MarksItemFailed` — add platform row, update metadata
- `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame` — add platform row (`pc-windows`), update metadata
- `TestProcessSyncItem_CancelledJobNotOverwritten` — add platform row, update metadata

For `TestProcessSyncItem_UnresolvedPlatform_Failed`, replace entirely:

```go
func TestProcessSyncItem_NoPlatforms_Failed(t *testing.T) {
	// An external_games row with no external_game_platforms rows is a bug.
	// The worker must mark the item failed.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	egID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'app-noplatform', 'No Platform Game', false, true, false, 0, 12345)`,
		egID, userID,
	).Exec(ctx)
	_, _ = testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (12345, 'No Platform Game', now(), now()) ON CONFLICT DO NOTHING`,
	).Exec(ctx)
	// No external_game_platforms row inserted — this is the bug scenario.
	metaJSON, _ := json.Marshal(map[string]any{"external_game_id": egID, "playtime_hours": 0})
	itemID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'app-noplatform', 'No Platform Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	).Exec(ctx)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected status=failed for game with no platform rows, got %q", status)
	}
}
```

### Step 10g — Run all tests

- [ ] **Run full test suite**

```bash
go test -timeout 600s ./... 2>&1 | tail -30
```

Expected: all PASS. Fix any remaining compilation errors or assertion mismatches before proceeding.

- [ ] **Commit**

```bash
git add internal/worker/tasks/
git commit -m "test(sync): update tests for normalised external_games schema"
```

---

## Task 11: Final verification and lint

- [ ] **Run tests with coverage**

```bash
go test -timeout 600s -cover ./...
```

Expected: all PASS, no failures.

- [ ] **Run linter**

```bash
golangci-lint run
```

Expected: zero issues.

- [ ] **Verify frontend is unaffected**

```bash
cd ui/frontend && npm run check && npm run knip
```

Expected: zero errors.

- [ ] **Commit if any lint fixes were needed, then push**

```bash
git push -u origin issue-608-normalise-external-games
```

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-23-issue-608-external-games-normalisation.md`.**

Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using `executing-plans`, batch execution with checkpoints.

Which approach?
