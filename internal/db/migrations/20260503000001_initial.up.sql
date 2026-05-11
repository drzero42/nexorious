-- Users table
CREATE TABLE users (
    id            TEXT PRIMARY KEY,               -- UUID v4
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    is_admin      BOOLEAN NOT NULL DEFAULT false,
    preferences   TEXT NOT NULL DEFAULT '{}',     -- JSON
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_username_idx ON users (username);
CREATE INDEX users_is_active_idx ON users (is_active) WHERE is_active = true;

-- User sessions table
CREATE TABLE user_sessions (
    id                 TEXT PRIMARY KEY,           -- UUID v4
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
    id                          INTEGER PRIMARY KEY,  -- IGDB ID used directly
    title                       TEXT NOT NULL,
    description                 TEXT,
    genre                       TEXT,
    developer                   TEXT,
    publisher                   TEXT,
    release_date                DATE,
    cover_art_url               TEXT,
    rating_average              NUMERIC(5,2),
    rating_count                INTEGER,
    estimated_playtime_hours    INTEGER,
    howlongtobeat_main          NUMERIC(6,2),         -- hours (mapped from IGDB 'hastily')
    howlongtobeat_extra         NUMERIC(6,2),         -- hours (mapped from IGDB 'normally')
    howlongtobeat_completionist NUMERIC(6,2),         -- hours (mapped from IGDB 'completely')
    igdb_slug                   TEXT,
    igdb_platform_ids           TEXT,                 -- JSON array as text
    igdb_platform_names         TEXT,                 -- JSON array as text
    game_modes                  TEXT,                 -- comma-separated
    themes                      TEXT,                 -- comma-separated
    player_perspectives         TEXT,                 -- comma-separated
    game_metadata               TEXT,                 -- JSON object as text
    last_updated                TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX games_title_idx ON games (title);
CREATE INDEX games_genre_idx ON games (genre);
CREATE INDEX games_developer_idx ON games (developer);
CREATE INDEX games_publisher_idx ON games (publisher);
CREATE INDEX games_release_date_idx ON games (release_date);

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

-- User games (user's personal collection entries)
CREATE TABLE user_games (
    id              TEXT PRIMARY KEY,              -- UUID v4
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id         INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    play_status     TEXT,                          -- 'not_started', 'playing', 'completed', etc.
    personal_rating INTEGER,                       -- 1-10
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

-- User game platforms (which platforms/storefronts a user owns a game on)
CREATE TABLE user_game_platforms (
    id                       TEXT PRIMARY KEY,     -- UUID v4
    user_game_id             TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    platform                 TEXT NOT NULL REFERENCES platforms(name) ON DELETE RESTRICT,
    storefront               TEXT NOT NULL REFERENCES storefronts(name) ON DELETE RESTRICT,
    store_game_id            TEXT,                 -- external platform's game ID
    store_url                TEXT,
    is_available             BOOLEAN NOT NULL DEFAULT true,
    hours_played             NUMERIC(10,2),
    ownership_status         TEXT,                 -- 'owned', 'subscription', etc.
    acquired_date            DATE,
    original_platform_name   TEXT,                 -- raw name from sync source
    original_storefront_name TEXT,                 -- raw name from sync source
    external_game_id         TEXT,                 -- FK to external_games (added after that table)
    sync_from_source         BOOLEAN NOT NULL DEFAULT false,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_game_id, platform, storefront)
);

CREATE INDEX user_game_platforms_user_game_id_idx ON user_game_platforms (user_game_id);
CREATE INDEX user_game_platforms_platform_idx ON user_game_platforms (platform);
CREATE INDEX user_game_platforms_storefront_idx ON user_game_platforms (storefront);
CREATE INDEX user_game_platforms_external_game_id_idx ON user_game_platforms (external_game_id);

-- Tags (user-created labels for organizing games)
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,                   -- UUID v4
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT,                               -- hex color code
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX tags_user_id_idx ON tags (user_id);

-- User game tags (many-to-many join)
CREATE TABLE user_game_tags (
    id           TEXT PRIMARY KEY,                 -- UUID v4
    user_game_id TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    tag_id       TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_game_id, tag_id)
);

CREATE INDEX user_game_tags_user_game_id_idx ON user_game_tags (user_game_id);
CREATE INDEX user_game_tags_tag_id_idx ON user_game_tags (tag_id);

-- External games (tracks games from sync sources before/after IGDB matching)
CREATE TABLE external_games (
    id               TEXT PRIMARY KEY,             -- UUID v4
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront       TEXT NOT NULL,
    external_id      TEXT NOT NULL,                -- platform's game ID
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

-- Add FK from user_game_platforms to external_games (deferred until after external_games exists)
ALTER TABLE user_game_platforms
    ADD CONSTRAINT user_game_platforms_external_game_id_fkey
    FOREIGN KEY (external_game_id)
    REFERENCES external_games(id)
    ON DELETE SET NULL;

-- User sync configs (per-user, per-platform sync settings and credentials)
CREATE TABLE user_sync_configs (
    id                     TEXT PRIMARY KEY,
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront             TEXT NOT NULL,
    frequency              TEXT NOT NULL DEFAULT 'manual',
    auto_add               BOOLEAN NOT NULL DEFAULT false,
    storefront_credentials TEXT,
    last_synced_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront)
);

CREATE INDEX user_sync_configs_user_id_idx    ON user_sync_configs (user_id);
CREATE INDEX user_sync_configs_storefront_idx ON user_sync_configs (storefront);

-- Jobs (user-visible background task tracking)
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

-- Job items (individual items within a job)
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

-- Pending tasks (database-backed worker queue)
CREATE TABLE pending_tasks (
    id         TEXT PRIMARY KEY,                   -- UUID v4
    task_type  TEXT NOT NULL,                      -- e.g. "sync", "import_item", "export", "metadata_refresh"
    payload    JSONB NOT NULL DEFAULT '{}',
    priority   INTEGER NOT NULL DEFAULT 0,         -- higher = more urgent
    status     TEXT NOT NULL DEFAULT 'pending',    -- 'pending' | 'running' | 'done' | 'failed'
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at TIMESTAMPTZ,
    done_at    TIMESTAMPTZ
);

-- Partial index on pending only: covers the worker claim query efficiently
CREATE INDEX pending_tasks_claim_idx ON pending_tasks (status, priority DESC, created_at)
    WHERE status = 'pending';

-- Backup config (singleton table — always id=1)
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    schedule_cron   TEXT NOT NULL DEFAULT '',      -- standard 5-field cron; empty = disabled
    retention_mode  TEXT NOT NULL DEFAULT 'days',  -- 'days' | 'count'
    retention_value INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the singleton row (daily at 2 AM UTC)
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

-- Reference data: platform-storefront associations
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

-- Rate limiter tokens (used by the PostgreSQL rate-limiter backend)
CREATE TABLE rate_limiter_tokens (
    key         TEXT PRIMARY KEY,
    tokens      DOUBLE PRECISION NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
