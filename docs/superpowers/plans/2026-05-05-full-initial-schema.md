# Full Initial Schema Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete database schema for nexorious-go in the initial migration, creating all 18 tables required for Phase 1.

**Architecture:** Single up/down migration pair (`0001_initial.up.sql` / `0001_initial.down.sql`) that creates all tables, indexes, foreign keys, and constraints. The down migration drops tables in reverse dependency order. Schema matches the Python Alembic models exactly but is written fresh for PostgreSQL 16.

**Tech Stack:** PostgreSQL 16, golang-migrate/v4

---

## File Structure

- **Modify:** `internal/db/migrations/0001_initial.up.sql` — add all 18 tables with constraints and indexes
- **Modify:** `internal/db/migrations/0001_initial.down.sql` — add DROP statements in reverse dependency order
- **Create:** `internal/db/migrations/migration_test.go` — integration test that runs migration up/down against testcontainers PostgreSQL

---

### Task 1: Core User and Auth Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add users table to up migration**

Append to `0001_initial.up.sql` after the existing `schema_info` table:

```sql
-- Users table
CREATE TABLE users (
    id              TEXT PRIMARY KEY,                     -- UUID v4
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    is_admin        BOOLEAN NOT NULL DEFAULT false,
    preferences     TEXT NOT NULL DEFAULT '{}',           -- JSON
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_username_idx ON users (username);
CREATE INDEX users_is_active_idx ON users (is_active) WHERE is_active = true;
```

- [ ] **Step 2: Add user_sessions table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- User sessions table
CREATE TABLE user_sessions (
    id                   TEXT PRIMARY KEY,                -- UUID v4
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash           TEXT NOT NULL,
    refresh_token_hash   TEXT NOT NULL,
    user_agent           TEXT,
    ip_address           TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX user_sessions_user_id_idx ON user_sessions (user_id);
CREATE INDEX user_sessions_token_hash_idx ON user_sessions (token_hash);
CREATE INDEX user_sessions_expires_at_idx ON user_sessions (expires_at);
```

- [ ] **Step 3: Update down migration for user tables**

Prepend to `0001_initial.down.sql` before the existing `schema_info` drop (reverse dependency order):

```sql
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 4: Verify SQL syntax**

Run:
```bash
devenv shell
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Tables created successfully (will error on schema_info already exists — that's fine)

- [ ] **Step 5: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS user_sessions, users, schema_info CASCADE;"
```

Expected: Tables dropped

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add users and user_sessions tables"
```

---

### Task 2: Games Catalog Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add games table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Games table (IGDB catalog)
CREATE TABLE games (
    id                             INTEGER PRIMARY KEY,  -- IGDB ID used directly
    title                          TEXT NOT NULL,
    description                    TEXT,
    genre                          TEXT,
    developer                      TEXT,
    publisher                      TEXT,
    release_date                   DATE,
    cover_art_url                  TEXT,
    rating_average                 NUMERIC(5,2),
    rating_count                   INTEGER,
    estimated_playtime_hours       INTEGER,
    howlongtobeat_main             NUMERIC(6,2),         -- hours
    howlongtobeat_extra            NUMERIC(6,2),         -- hours
    howlongtobeat_completionist    NUMERIC(6,2),         -- hours
    igdb_slug                      TEXT,
    igdb_platform_ids              TEXT,                 -- JSON array as text
    igdb_platform_names            TEXT,                 -- JSON array as text
    game_modes                     TEXT,                 -- comma-separated
    themes                         TEXT,                 -- comma-separated
    player_perspectives            TEXT,                 -- comma-separated
    game_metadata                  TEXT,                 -- JSON object as text
    last_updated                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX games_title_idx ON games (title);
CREATE INDEX games_genre_idx ON games (genre);
CREATE INDEX games_developer_idx ON games (developer);
CREATE INDEX games_publisher_idx ON games (publisher);
CREATE INDEX games_release_date_idx ON games (release_date);
```

- [ ] **Step 2: Update down migration for games**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS games;
```

- [ ] **Step 3: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Games table created (may error on existing tables — that's fine)

- [ ] **Step 4: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS games CASCADE;"
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add games table"
```

---

### Task 3: Platform and Storefront Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add platforms table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Platforms table (TEXT slug as PK)
CREATE TABLE platforms (
    name                TEXT PRIMARY KEY,                 -- slug: "pc-windows", "ps5", etc.
    display_name        TEXT NOT NULL,
    icon_url            TEXT,
    default_storefront  TEXT,                             -- FK added after storefronts table
    is_active           BOOLEAN NOT NULL DEFAULT true,
    source              TEXT NOT NULL,                    -- 'official' | 'custom'
    version_added       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX platforms_is_active_idx ON platforms (is_active) WHERE is_active = true;
CREATE INDEX platforms_source_idx ON platforms (source);
```

- [ ] **Step 2: Add storefronts table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Storefronts table (TEXT slug as PK)
CREATE TABLE storefronts (
    name          TEXT PRIMARY KEY,                      -- slug: "steam", "epic", etc.
    display_name  TEXT NOT NULL,
    icon_url      TEXT,
    base_url      TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    source        TEXT NOT NULL,                         -- 'official' | 'custom'
    version_added TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX storefronts_is_active_idx ON storefronts (is_active) WHERE is_active = true;
CREATE INDEX storefronts_source_idx ON storefronts (source);
```

- [ ] **Step 3: Add platform_storefronts join table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Platform-Storefront many-to-many join table
CREATE TABLE platform_storefronts (
    platform    TEXT NOT NULL REFERENCES platforms(name) ON DELETE CASCADE,
    storefront  TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (platform, storefront)
);

CREATE INDEX platform_storefronts_platform_idx ON platform_storefronts (platform);
CREATE INDEX platform_storefronts_storefront_idx ON platform_storefronts (storefront);
```

- [ ] **Step 4: Add FK constraint from platforms to storefronts**

Append to `0001_initial.up.sql`:

```sql
-- Add FK constraint for platforms.default_storefront (deferred until after storefronts table exists)
ALTER TABLE platforms
    ADD CONSTRAINT platforms_default_storefront_fkey
    FOREIGN KEY (default_storefront)
    REFERENCES storefronts(name)
    ON DELETE SET NULL;
```

- [ ] **Step 5: Update down migration for platform tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS platform_storefronts;
DROP TABLE IF EXISTS storefronts;
DROP TABLE IF EXISTS platforms;
```

- [ ] **Step 6: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Platform tables created

- [ ] **Step 7: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS platform_storefronts, storefronts, platforms CASCADE;"
```

- [ ] **Step 8: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add platforms, storefronts, and join table"
```

---

### Task 4: User Game Collection Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add user_games table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- User games (user's personal collection entries)
CREATE TABLE user_games (
    id              TEXT PRIMARY KEY,                    -- UUID v4
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id         INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    play_status     TEXT,                                 -- 'not_started', 'playing', 'completed', etc.
    personal_rating INTEGER,                              -- 1-10
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
```

- [ ] **Step 2: Add user_game_platforms table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- User game platforms (which platforms/storefronts a user owns a game on)
CREATE TABLE user_game_platforms (
    id                         TEXT PRIMARY KEY,         -- UUID v4
    user_game_id               TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    platform                   TEXT NOT NULL REFERENCES platforms(name) ON DELETE RESTRICT,
    storefront                 TEXT NOT NULL REFERENCES storefronts(name) ON DELETE RESTRICT,
    store_game_id              TEXT,                      -- external platform's game ID
    store_url                  TEXT,
    is_available               BOOLEAN NOT NULL DEFAULT true,
    hours_played               NUMERIC(10,2),
    ownership_status           TEXT,                      -- 'owned', 'subscription', etc.
    acquired_date              DATE,
    original_platform_name     TEXT,                      -- raw name from sync source
    original_storefront_name   TEXT,                      -- raw name from sync source
    external_game_id           TEXT,                      -- FK to external_games (added later)
    sync_from_source           BOOLEAN NOT NULL DEFAULT false,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_game_id, platform, storefront)
);

CREATE INDEX user_game_platforms_user_game_id_idx ON user_game_platforms (user_game_id);
CREATE INDEX user_game_platforms_platform_idx ON user_game_platforms (platform);
CREATE INDEX user_game_platforms_storefront_idx ON user_game_platforms (storefront);
CREATE INDEX user_game_platforms_external_game_id_idx ON user_game_platforms (external_game_id);
```

- [ ] **Step 3: Update down migration for user game tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS user_game_platforms;
DROP TABLE IF EXISTS user_games;
```

- [ ] **Step 4: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: User game tables created

- [ ] **Step 5: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS user_game_platforms, user_games CASCADE;"
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add user_games and user_game_platforms tables"
```

---

### Task 5: Tags Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add tags table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Tags (user-created tags for organizing games)
CREATE TABLE tags (
    id          TEXT PRIMARY KEY,                        -- UUID v4
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    color       TEXT,                                     -- hex color code
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX tags_user_id_idx ON tags (user_id);
```

- [ ] **Step 2: Add user_game_tags join table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- User game tags (many-to-many join)
CREATE TABLE user_game_tags (
    id            TEXT PRIMARY KEY,                      -- UUID v4
    user_game_id  TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    tag_id        TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_game_id, tag_id)
);

CREATE INDEX user_game_tags_user_game_id_idx ON user_game_tags (user_game_id);
CREATE INDEX user_game_tags_tag_id_idx ON user_game_tags (tag_id);
```

- [ ] **Step 3: Update down migration for tag tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS user_game_tags;
DROP TABLE IF EXISTS tags;
```

- [ ] **Step 4: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Tag tables created

- [ ] **Step 5: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS user_game_tags, tags CASCADE;"
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add tags and user_game_tags tables"
```

---

### Task 6: External Sync Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add external_games table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- External games (tracks games from sync sources before IGDB matching)
CREATE TABLE external_games (
    id                TEXT PRIMARY KEY,                   -- UUID v4
    user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront        TEXT NOT NULL REFERENCES storefronts(name) ON DELETE CASCADE,
    external_id       TEXT NOT NULL,                      -- platform's game ID
    title             TEXT NOT NULL,
    resolved_igdb_id  INTEGER REFERENCES games(id) ON DELETE SET NULL,
    is_skipped        BOOLEAN NOT NULL DEFAULT false,
    is_available      BOOLEAN NOT NULL DEFAULT true,
    is_subscription   BOOLEAN NOT NULL DEFAULT false,
    playtime_hours    NUMERIC(10,2),
    ownership_status  TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront, external_id)
);

CREATE INDEX external_games_user_id_idx ON external_games (user_id);
CREATE INDEX external_games_storefront_idx ON external_games (storefront);
CREATE INDEX external_games_resolved_igdb_id_idx ON external_games (resolved_igdb_id);
CREATE INDEX external_games_is_skipped_idx ON external_games (is_skipped) WHERE is_skipped = true;
```

- [ ] **Step 2: Add FK constraint from user_game_platforms to external_games**

Append to `0001_initial.up.sql`:

```sql
-- Add FK constraint from user_game_platforms to external_games (deferred until after external_games exists)
ALTER TABLE user_game_platforms
    ADD CONSTRAINT user_game_platforms_external_game_id_fkey
    FOREIGN KEY (external_game_id)
    REFERENCES external_games(id)
    ON DELETE SET NULL;
```

- [ ] **Step 3: Add user_sync_configs table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- User sync configs (per-user, per-platform sync settings and credentials)
CREATE TABLE user_sync_configs (
    id                     TEXT PRIMARY KEY,              -- UUID v4
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform               TEXT NOT NULL,                  -- 'steam', 'psn', 'epic'
    frequency              TEXT NOT NULL DEFAULT 'manual', -- 'manual' | 'hourly' | 'daily' | 'weekly'
    auto_add               BOOLEAN NOT NULL DEFAULT false,
    platform_credentials   TEXT,                           -- JSON (encrypted at rest)
    last_synced_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, platform)
);

CREATE INDEX user_sync_configs_user_id_idx ON user_sync_configs (user_id);
CREATE INDEX user_sync_configs_platform_idx ON user_sync_configs (platform);
```

- [ ] **Step 4: Update down migration for sync tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS user_sync_configs;
DROP TABLE IF EXISTS external_games;
```

- [ ] **Step 5: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Sync tables created

- [ ] **Step 6: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS user_sync_configs, external_games CASCADE;"
```

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add external_games and user_sync_configs tables"
```

---

### Task 7: Job System Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add jobs table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Jobs (user-visible background tasks)
CREATE TABLE jobs (
    id            TEXT PRIMARY KEY,                      -- UUID v4
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type      TEXT NOT NULL,                         -- 'sync', 'import', 'export', 'metadata_refresh'
    status        TEXT NOT NULL DEFAULT 'pending',       -- 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
    total_items   INTEGER NOT NULL DEFAULT 0,
    processed     INTEGER NOT NULL DEFAULT 0,
    succeeded     INTEGER NOT NULL DEFAULT 0,
    failed        INTEGER NOT NULL DEFAULT 0,
    source        TEXT,                                   -- platform/source name for sync/import jobs
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);

CREATE INDEX jobs_user_id_idx ON jobs (user_id);
CREATE INDEX jobs_status_idx ON jobs (status);
CREATE INDEX jobs_job_type_idx ON jobs (job_type);
CREATE INDEX jobs_created_at_idx ON jobs (created_at DESC);
```

- [ ] **Step 2: Add job_items table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Job items (individual items within a job)
CREATE TABLE job_items (
    id             TEXT PRIMARY KEY,                     -- UUID v4
    job_id         TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    external_id    TEXT,                                  -- platform's game ID
    external_title TEXT,
    igdb_id        INTEGER REFERENCES games(id) ON DELETE SET NULL,
    igdb_title     TEXT,
    status         TEXT NOT NULL DEFAULT 'pending',      -- 'pending' | 'matched' | 'pending_review' | 'skipped' | 'failed' | 'no_match'
    confidence     NUMERIC(4,3),                          -- fuzzy match confidence 0.0-1.0
    error_message  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at   TIMESTAMPTZ
);

CREATE INDEX job_items_job_id_idx ON job_items (job_id);
CREATE INDEX job_items_status_idx ON job_items (status);
CREATE INDEX job_items_igdb_id_idx ON job_items (igdb_id);
```

- [ ] **Step 3: Update down migration for job tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS job_items;
DROP TABLE IF EXISTS jobs;
```

- [ ] **Step 4: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Job tables created

- [ ] **Step 5: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS job_items, jobs CASCADE;"
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add jobs and job_items tables"
```

---

### Task 8: Worker Queue and Config Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Add pending_tasks table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Pending tasks (database-backed worker queue)
CREATE TABLE pending_tasks (
    id          TEXT PRIMARY KEY,                        -- UUID v4
    task_type   TEXT NOT NULL,                           -- e.g. "sync", "import_item", "export", "metadata_refresh"
    payload     JSONB NOT NULL DEFAULT '{}',
    priority    INTEGER NOT NULL DEFAULT 0,              -- higher = more urgent
    status      TEXT NOT NULL DEFAULT 'pending',         -- 'pending' | 'running' | 'done' | 'failed'
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at  TIMESTAMPTZ,
    done_at     TIMESTAMPTZ
);

CREATE INDEX pending_tasks_claim_idx ON pending_tasks (status, priority DESC, created_at)
    WHERE status = 'pending';
```

- [ ] **Step 2: Add backup_config singleton table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Backup config (singleton table, always id=1)
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    schedule_cron   TEXT NOT NULL DEFAULT '',             -- standard 5-field cron, empty = disabled
    retention_mode  TEXT NOT NULL DEFAULT 'days',         -- 'days' | 'count'
    retention_value INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Insert default row
INSERT INTO backup_config (id, schedule_cron, retention_mode, retention_value)
VALUES (1, '0 2 * * *', 'days', 30);
```

- [ ] **Step 3: Add rate_limiter_tokens table to up migration**

Append to `0001_initial.up.sql`:

```sql
-- Rate limiter tokens (for PostgreSQL rate limiter backend)
CREATE TABLE rate_limiter_tokens (
    key         TEXT PRIMARY KEY,
    tokens      DOUBLE PRECISION NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 4: Update down migration for worker/config tables**

Prepend to `0001_initial.down.sql`:

```sql
DROP TABLE IF EXISTS rate_limiter_tokens;
DROP TABLE IF EXISTS backup_config;
DROP TABLE IF EXISTS pending_tasks;
```

- [ ] **Step 5: Verify SQL syntax**

Run:
```bash
psql $DATABASE_URL -c "\i internal/db/migrations/0001_initial.up.sql"
```

Expected: Worker/config tables created, backup_config row inserted

- [ ] **Step 6: Clean up test database**

Run:
```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS rate_limiter_tokens, backup_config, pending_tasks CASCADE;"
```

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/0001_initial.up.sql internal/db/migrations/0001_initial.down.sql
git commit -m "feat(schema): add pending_tasks, backup_config, and rate_limiter_tokens tables"
```

---

### Task 9: Integration Test

**Files:**
- Create: `internal/db/migrations/migration_test.go`

- [ ] **Step 1: Write failing migration test**

Create `internal/db/migrations/migration_test.go`:

```go
package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"nexorious-go/internal/db/migrations"
)

func TestMigrationUpDown(t *testing.T) {
	ctx := context.Background()

	// Start PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2)),
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, pgContainer.Terminate(ctx))
	}()

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Run migrations up
	m, err := newMigrator(connStr)
	require.NoError(t, err)
	defer m.Close()

	err = m.Up()
	require.NoError(t, err)

	// Verify all tables exist
	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer db.Close()

	expectedTables := []string{
		"schema_info",
		"users",
		"user_sessions",
		"games",
		"platforms",
		"storefronts",
		"platform_storefronts",
		"user_games",
		"user_game_platforms",
		"tags",
		"user_game_tags",
		"external_games",
		"user_sync_configs",
		"jobs",
		"job_items",
		"pending_tasks",
		"backup_config",
		"rate_limiter_tokens",
	}

	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = $1
			)
		`, table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist", table)
	}

	// Verify backup_config singleton row exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM backup_config").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "backup_config should have exactly one row")

	// Run migrations down
	err = m.Down()
	require.NoError(t, err)

	// Verify all tables dropped
	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public'
				AND table_name = $1
			)
		`, table).Scan(&exists)
		require.NoError(t, err)
		assert.False(t, exists, "table %s should be dropped", table)
	}
}

func newMigrator(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("iofs.New: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrate.NewWithSourceInstance: %w", err)
	}

	return m, nil
}
```

- [ ] **Step 2: Add testify/assert dependency**

Run:
```bash
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
go mod tidy
```

Expected: Dependencies added

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
go test ./internal/db/migrations/... -v
```

Expected: Test runs but may have compilation errors or assertion failures (we haven't completed the migration SQL yet)

- [ ] **Step 4: Run test until it passes**

After all migration SQL is complete, run:
```bash
go test ./internal/db/migrations/... -v
```

Expected: PASS - all tables created and dropped successfully

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/migration_test.go go.mod go.sum
git commit -m "test(schema): add migration up/down integration test"
```

---

### Task 10: Final Verification and Cleanup

**Files:**
- Modify: `internal/db/migrations/0001_initial.up.sql`
- Modify: `internal/db/migrations/0001_initial.down.sql`

- [ ] **Step 1: Run complete migration against clean database**

Run:
```bash
devenv shell
# Drop all existing tables
psql $DATABASE_URL -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
# Run migration
go run ./cmd/nexorious
```

Expected: Server starts, migration UI appears, all tables created successfully

- [ ] **Step 2: Verify table count**

Run:
```bash
psql $DATABASE_URL -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';"
```

Expected: 18 tables

- [ ] **Step 3: Run full test suite**

Run:
```bash
go test ./... -v
```

Expected: All tests pass

- [ ] **Step 4: Verify down migration**

Run:
```bash
psql $DATABASE_URL -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
# Start server, run migration via UI
# Then manually test down migration
migrate -database $DATABASE_URL -path internal/db/migrations down
psql $DATABASE_URL -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';"
```

Expected: 0 tables (all dropped)

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(schema): complete initial database schema with all 18 tables"
```

---

## Self-Review Checklist

### Spec Coverage

✅ **Users and sessions** (Task 1)
- `users` table with UUID PK, username, password_hash, is_active, is_admin, preferences JSON
- `user_sessions` table with token hashes, user_agent, ip_address

✅ **Games catalog** (Task 2)
- `games` table with INT PK (IGDB ID), all IGDB metadata fields
- HowLongToBeat fields as NUMERIC hours (note about conversion from seconds in spec)
- JSON fields stored as TEXT

✅ **Platforms and storefronts** (Task 3)
- `platforms` table with TEXT slug PK, default_storefront FK (deferred)
- `storefronts` table with TEXT slug PK
- `platform_storefronts` many-to-many join table
- Both have source field ('official' | 'custom') and version_added

✅ **User collection** (Task 4)
- `user_games` table with UNIQUE(user_id, game_id)
- `user_game_platforms` table with UNIQUE(user_game_id, platform, storefront)
- FK to external_games (deferred until Task 6)

✅ **Tags** (Task 5)
- `tags` table with UNIQUE(user_id, name)
- `user_game_tags` join table with UNIQUE(user_game_id, tag_id)

✅ **Sync system** (Task 6)
- `external_games` table with UNIQUE(user_id, storefront, external_id), is_skipped flag
- `user_sync_configs` table with UNIQUE(user_id, platform), encrypted credentials

✅ **Job system** (Task 7)
- `jobs` table with job_type, status, counters
- `job_items` table with FK to jobs

✅ **Worker queue and config** (Task 8)
- `pending_tasks` table with partial index on status='pending'
- `backup_config` singleton with id=1 CHECK constraint, default row inserted
- `rate_limiter_tokens` table

✅ **Testing** (Task 9)
- Integration test using testcontainers
- Verifies all 18 tables created and dropped

### No Placeholders

✅ All SQL is complete and executable
✅ All test code is complete
✅ All commands have expected output documented
✅ No "TBD", "TODO", or "implement later"

### Type Consistency

✅ UUID fields consistently TEXT
✅ IGDB ID consistently INTEGER
✅ Platform/storefront slugs consistently TEXT
✅ Timestamps consistently TIMESTAMPTZ
✅ JSON fields consistently TEXT (not JSONB except pending_tasks.payload)
✅ Foreign key references match PK types

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-05-full-initial-schema.md`.

**Two execution options:**

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
