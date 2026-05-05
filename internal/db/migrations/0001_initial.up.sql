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
    name               TEXT PRIMARY KEY,           -- slug: "pc-windows", "ps5", etc.
    display_name       TEXT NOT NULL,
    icon_url           TEXT,
    default_storefront TEXT,                       -- FK added after storefronts table
    is_active          BOOLEAN NOT NULL DEFAULT true,
    source             TEXT NOT NULL,              -- 'official' | 'custom'
    version_added      TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX platforms_is_active_idx ON platforms (is_active) WHERE is_active = true;
CREATE INDEX platforms_source_idx ON platforms (source);

-- Storefronts table (TEXT slug as PK)
CREATE TABLE storefronts (
    name          TEXT PRIMARY KEY,                -- slug: "steam", "epic", etc.
    display_name  TEXT NOT NULL,
    icon_url      TEXT,
    base_url      TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    source        TEXT NOT NULL,                   -- 'official' | 'custom'
    version_added TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX storefronts_is_active_idx ON storefronts (is_active) WHERE is_active = true;
CREATE INDEX storefronts_source_idx ON storefronts (source);

-- Platform-Storefront many-to-many join table
CREATE TABLE platform_storefronts (
    platform   TEXT NOT NULL REFERENCES platforms(name) ON DELETE CASCADE,
    storefront TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (platform, storefront)
);

CREATE INDEX platform_storefronts_platform_idx ON platform_storefronts (platform);
CREATE INDEX platform_storefronts_storefront_idx ON platform_storefronts (storefront);

-- Add FK constraint for platforms.default_storefront (deferred until after storefronts exists)
ALTER TABLE platforms
    ADD CONSTRAINT platforms_default_storefront_fkey
    FOREIGN KEY (default_storefront)
    REFERENCES storefronts(name)
    ON DELETE SET NULL;

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
    storefront       TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    external_id      TEXT NOT NULL,                -- platform's game ID
    title            TEXT NOT NULL,
    resolved_igdb_id INTEGER REFERENCES games(id) ON DELETE SET NULL,
    is_skipped       BOOLEAN NOT NULL DEFAULT false,
    is_available     BOOLEAN NOT NULL DEFAULT true,
    is_subscription  BOOLEAN NOT NULL DEFAULT false,
    playtime_hours   NUMERIC(10,2),
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
    id                   TEXT PRIMARY KEY,         -- UUID v4
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform             TEXT NOT NULL,            -- 'steam', 'psn', 'epic'
    frequency            TEXT NOT NULL DEFAULT 'manual',  -- 'manual' | 'hourly' | 'daily' | 'weekly'
    auto_add             BOOLEAN NOT NULL DEFAULT false,
    platform_credentials TEXT,                     -- JSON encrypted at rest (AES-GCM)
    last_synced_at       TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, platform)
);

CREATE INDEX user_sync_configs_user_id_idx ON user_sync_configs (user_id);
CREATE INDEX user_sync_configs_platform_idx ON user_sync_configs (platform);

-- Jobs (user-visible background task tracking)
CREATE TABLE jobs (
    id            TEXT PRIMARY KEY,                -- UUID v4
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type      TEXT NOT NULL,                   -- 'sync', 'import', 'export', 'metadata_refresh'
    status        TEXT NOT NULL DEFAULT 'pending', -- 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
    total_items   INTEGER NOT NULL DEFAULT 0,
    processed     INTEGER NOT NULL DEFAULT 0,
    succeeded     INTEGER NOT NULL DEFAULT 0,
    failed        INTEGER NOT NULL DEFAULT 0,
    source        TEXT,                            -- platform/source name for sync/import jobs
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);

CREATE INDEX jobs_user_id_idx ON jobs (user_id);
CREATE INDEX jobs_status_idx ON jobs (status);
CREATE INDEX jobs_job_type_idx ON jobs (job_type);
CREATE INDEX jobs_created_at_idx ON jobs (created_at DESC);

-- Job items (individual items within a job)
CREATE TABLE job_items (
    id             TEXT PRIMARY KEY,               -- UUID v4
    job_id         TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    external_id    TEXT,                           -- platform's game ID
    external_title TEXT,
    igdb_id        INTEGER REFERENCES games(id) ON DELETE SET NULL,
    igdb_title     TEXT,
    status         TEXT NOT NULL DEFAULT 'pending',  -- 'pending' | 'matched' | 'pending_review' | 'skipped' | 'failed' | 'no_match'
    confidence     NUMERIC(4,3),                   -- fuzzy match confidence 0.0–1.0
    error_message  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at   TIMESTAMPTZ
);

CREATE INDEX job_items_job_id_idx ON job_items (job_id);
CREATE INDEX job_items_status_idx ON job_items (status);
CREATE INDEX job_items_igdb_id_idx ON job_items (igdb_id);

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

-- Rate limiter tokens (used by the PostgreSQL rate-limiter backend)
CREATE TABLE rate_limiter_tokens (
    key         TEXT PRIMARY KEY,
    tokens      DOUBLE PRECISION NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
