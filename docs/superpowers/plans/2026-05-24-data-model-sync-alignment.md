# Data Model Sync Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the single initial migration and Go model structs with the canonical sync spec in `docs/sync.md`.

**Architecture:** Three focused changes — the migration SQL, `internal/db/models/models.go`, and `internal/db/models/jobs.go` — each committed independently. No worker, service, or API code is touched; those callers will fail to compile after this pass and are updated in a follow-up session.

**Tech Stack:** Go 1.25, Bun ORM (`uptrace/bun`), PostgreSQL via `pgdriver`

> **Important:** Do NOT run `go test ./...` or `go build ./...` during this plan. Worker and API code references fields that are being removed and will fail to compile. Only `go build ./internal/db/models/...` is expected to pass.

---

## Files Changed

| File | Action | Reason |
|---|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Modify | Remove dropped columns, add `hours_played` to `external_game_platforms`, add `external_game_id` to `job_items`, add `sync_changes` table |
| `internal/db/models/models.go` | Modify | Update structs to match new schema; add `SyncChange`; drop `encoding/json` import |
| `internal/db/models/jobs.go` | Modify | Add `ExternalGameID` to `JobItem`; replace `JobItemStatusIGDBFailed` with `JobItemStatusCancelled` |

---

## Task 1: Update the migration SQL

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`

### `external_games` — remove `playtime_hours`

- [ ] **Step 1: Remove `playtime_hours` from the `external_games` CREATE TABLE**

Find this block in the file:

```sql
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
```

Replace it with:

```sql
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
    ownership_status TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront, external_id)
);
```

### `external_game_platforms` — add `hours_played`

- [ ] **Step 2: Add `hours_played` to the `external_game_platforms` CREATE TABLE**

Find this block:

```sql
CREATE TABLE external_game_platforms (
    id               TEXT PRIMARY KEY,
    external_game_id TEXT NOT NULL REFERENCES external_games(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(external_game_id, platform)
);
```

Replace it with:

```sql
CREATE TABLE external_game_platforms (
    id               TEXT PRIMARY KEY,
    external_game_id TEXT NOT NULL REFERENCES external_games(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    hours_played     NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(external_game_id, platform)
);
```

### `user_games` — remove `hours_played`

- [ ] **Step 3: Remove `hours_played` from the `user_games` CREATE TABLE**

Find this block:

```sql
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
```

Replace it with:

```sql
CREATE TABLE user_games (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id         INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    play_status     TEXT,
    personal_rating INTEGER,
    is_loved        BOOLEAN NOT NULL DEFAULT false,
    personal_notes  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, game_id)
);
```

### `user_sync_configs` — remove `epic_legendary_state`

- [ ] **Step 4: Remove `epic_legendary_state` from the `user_sync_configs` CREATE TABLE**

Find this block:

```sql
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
```

Replace it with:

```sql
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
```

### `job_items` — add `external_game_id`

- [ ] **Step 5: Add `external_game_id` to the `job_items` CREATE TABLE**

Find this block:

```sql
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
```

Replace it with:

```sql
CREATE TABLE job_items (
    id                  TEXT PRIMARY KEY,
    job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external_game_id    TEXT REFERENCES external_games(id) ON DELETE SET NULL,
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
CREATE INDEX job_items_external_game_id_idx ON job_items (external_game_id);
```

### `sync_changes` — new table

- [ ] **Step 6: Add the `sync_changes` table immediately after the `job_items` block**

Insert the following SQL after the `job_items` indexes and before the `-- Backup config` comment:

```sql
-- Sync changes: one row per library event per sync run (backs the Sync History UI)
CREATE TABLE sync_changes (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external_game_id TEXT REFERENCES external_games(id) ON DELETE SET NULL,
    change_type      TEXT NOT NULL,
    title            TEXT NOT NULL,
    old_status       TEXT,
    new_status       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sync_changes_job_id_idx    ON sync_changes (job_id);
CREATE INDEX sync_changes_user_id_idx   ON sync_changes (user_id);
CREATE INDEX sync_changes_created_at_idx ON sync_changes (created_at);
```

### `initial.down.sql` — add `sync_changes` drop

- [ ] **Step 7: Add `sync_changes` to the down migration**

The down migration drops tables in reverse dependency order. `sync_changes` depends on `jobs`, `users`, and `external_games`, so it must be dropped before `job_items` and `jobs`.

Find `internal/db/migrations/20260503000001_initial.down.sql`. Replace:

```sql
DROP TABLE IF EXISTS rate_limiter_tokens;
DROP TABLE IF EXISTS backup_config;
DROP TABLE IF EXISTS job_items;
DROP TABLE IF EXISTS jobs;
```

with:

```sql
DROP TABLE IF EXISTS rate_limiter_tokens;
DROP TABLE IF EXISTS backup_config;
DROP TABLE IF EXISTS sync_changes;
DROP TABLE IF EXISTS job_items;
DROP TABLE IF EXISTS jobs;
```

- [ ] **Step 8: Commit the migration changes**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql
git add internal/db/migrations/20260503000001_initial.down.sql
git commit -m "chore(migrations): align initial migration with sync spec"
```

---

## Task 2: Update `models.go`

**Files:**
- Modify: `internal/db/models/models.go`

- [ ] **Step 1: Replace the `ExternalGame` struct**

Find the entire `ExternalGame` struct (lines starting with `// ExternalGame mirrors...` through the closing `}`). Replace it with:

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
	OwnershipStatus *string   `bun:"ownership_status"         json:"ownership_status"`
	CreatedAt       time.Time `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"       json:"updated_at"`

	Platforms []ExternalGamePlatform `bun:"rel:has-many,join:id=external_game_id" json:"-"`
}
```

- [ ] **Step 2: Replace the `ExternalGamePlatform` struct**

Find the entire `ExternalGamePlatform` struct (lines starting with `// ExternalGamePlatform mirrors...` through the closing `}`). Replace it with:

```go
// ExternalGamePlatform mirrors the external_game_platforms table.
// platform holds a canonical slug matching platforms.name.
type ExternalGamePlatform struct {
	bun.BaseModel `bun:"table:external_game_platforms"`

	ID             string    `bun:"id,pk"                    json:"id"`
	ExternalGameID string    `bun:"external_game_id,notnull" json:"external_game_id"`
	Platform       string    `bun:"platform,notnull"         json:"platform"`
	HoursPlayed    float64   `bun:"hours_played,notnull"     json:"hours_played"`
	CreatedAt      time.Time `bun:"created_at,notnull"       json:"created_at"`
}
```

- [ ] **Step 3: Replace the `UserGame` struct**

Find the entire `UserGame` struct. Replace it with:

```go
type UserGame struct {
	bun.BaseModel `bun:"table:user_games"`

	ID             string    `bun:"id,pk"              json:"id"`
	UserID         string    `bun:"user_id,notnull"    json:"user_id"`
	GameID         int32     `bun:"game_id,notnull"    json:"game_id"`
	PlayStatus     *string   `bun:"play_status"        json:"play_status"`
	PersonalRating *int32    `bun:"personal_rating"    json:"personal_rating"`
	IsLoved        bool      `bun:"is_loved,notnull"   json:"is_loved"`
	PersonalNotes  *string   `bun:"personal_notes"     json:"personal_notes"`
	CreatedAt      time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updated_at"`

	Game      *Game              `bun:"rel:belongs-to,join:game_id=id"    json:"game,omitempty"`
	Platforms []UserGamePlatform `bun:"rel:has-many,join:id=user_game_id" json:"platforms"`
	Tags      []UserGameTag      `bun:"rel:has-many,join:id=user_game_id" json:"tags"`
}
```

- [ ] **Step 4: Replace the `UserSyncConfig` struct**

Find the entire `UserSyncConfig` struct (lines starting with `// UserSyncConfig mirrors...` through the closing `}`). Replace it with:

```go
// UserSyncConfig mirrors the user_sync_configs table.
type UserSyncConfig struct {
	bun.BaseModel `bun:"table:user_sync_configs"`

	ID                    string     `bun:"id,pk"                  json:"id"`
	UserID                string     `bun:"user_id,notnull"         json:"user_id"`
	Storefront            string     `bun:"storefront,notnull"      json:"storefront"`
	Frequency             string     `bun:"frequency,notnull"       json:"frequency"`
	AutoAdd               bool       `bun:"auto_add,notnull"        json:"auto_add"`
	StorefrontCredentials *string    `bun:"storefront_credentials"  json:"-"`
	LastSyncedAt          *time.Time `bun:"last_synced_at"          json:"last_synced_at"`
	CreatedAt             time.Time  `bun:"created_at,notnull"      json:"created_at"`
	UpdatedAt             time.Time  `bun:"updated_at,notnull"      json:"updated_at"`
}
```

- [ ] **Step 5: Add the `SyncChange` struct at the end of `models.go`**

Append this after the `UserSyncConfig` struct:

```go

// SyncChange mirrors the sync_changes table — one row per library event per sync run.
type SyncChange struct {
	bun.BaseModel `bun:"table:sync_changes"`

	ID             string    `bun:"id,pk"               json:"id"`
	JobID          string    `bun:"job_id,notnull"      json:"job_id"`
	UserID         string    `bun:"user_id,notnull"     json:"user_id"`
	ExternalGameID *string   `bun:"external_game_id"    json:"external_game_id"`
	ChangeType     string    `bun:"change_type,notnull" json:"change_type"`
	Title          string    `bun:"title,notnull"       json:"title"`
	OldStatus      *string   `bun:"old_status"          json:"old_status"`
	NewStatus      *string   `bun:"new_status"          json:"new_status"`
	CreatedAt      time.Time `bun:"created_at,notnull"  json:"created_at"`
}
```

- [ ] **Step 6: Remove the `encoding/json` import from `models.go`**

`EpicLegendaryState` was the only use of `json.RawMessage` in `models.go`. Update the import block from:

```go
import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)
```

to:

```go
import (
	"time"

	"github.com/uptrace/bun"
)
```

- [ ] **Step 7: Verify the models package compiles**

```bash
go build ./internal/db/models/...
```

Expected: no output, exit 0. If there are errors, they are in `models.go` or `jobs.go` only — fix them before continuing. Do NOT fix errors in other packages.

- [ ] **Step 8: Commit**

```bash
git add internal/db/models/models.go
git commit -m "chore(models): align structs with sync spec"
```

---

## Task 3: Update `jobs.go`

**Files:**
- Modify: `internal/db/models/jobs.go`

- [ ] **Step 1: Replace the `JobItem` status constants block**

Find this block:

```go
// JobItem status constants.
const (
	JobItemStatusPending       = "pending"
	JobItemStatusProcessing    = "processing"
	JobItemStatusCompleted     = "completed"
	JobItemStatusPendingReview = "pending_review"
	JobItemStatusSkipped       = "skipped"
	JobItemStatusFailed        = "failed"
	JobItemStatusIGDBFailed    = "igdb_failed"
)
```

Replace it with:

```go
// JobItem status constants.
const (
	JobItemStatusPending       = "pending"
	JobItemStatusProcessing    = "processing"
	JobItemStatusCompleted     = "completed"
	JobItemStatusPendingReview = "pending_review"
	JobItemStatusSkipped       = "skipped"
	JobItemStatusFailed        = "failed"
	JobItemStatusCancelled     = "cancelled"
)
```

- [ ] **Step 2: Replace the `JobItem` struct**

Find the entire `JobItem` struct. Replace it with:

```go
type JobItem struct {
	bun.BaseModel `bun:"table:job_items"`

	ID              string          `bun:"id,pk"                    json:"id"`
	JobID           string          `bun:"job_id,notnull"           json:"job_id"`
	UserID          string          `bun:"user_id,notnull"          json:"user_id"`
	ExternalGameID  *string         `bun:"external_game_id"         json:"external_game_id"`
	ItemKey         string          `bun:"item_key,notnull"         json:"item_key"`
	SourceTitle     string          `bun:"source_title,notnull"     json:"source_title"`
	SourceMetadata  json.RawMessage `bun:"source_metadata,notnull"  json:"source_metadata"`
	Status          string          `bun:"status,notnull"           json:"status"`
	Result          json.RawMessage `bun:"result,notnull"           json:"result"`
	ErrorMessage    *string         `bun:"error_message"            json:"error_message"`
	IGDBCandidates  json.RawMessage `bun:"igdb_candidates,notnull"  json:"igdb_candidates"`
	ResolvedIGDBID  *int            `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	MatchConfidence *float64        `bun:"match_confidence"         json:"match_confidence"`
	CreatedAt       time.Time       `bun:"created_at,notnull"       json:"created_at"`
	ProcessedAt     *time.Time      `bun:"processed_at"             json:"processed_at"`
	ResolvedAt      *time.Time      `bun:"resolved_at"              json:"resolved_at"`
}
```

- [ ] **Step 3: Verify the models package compiles**

```bash
go build ./internal/db/models/...
```

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/db/models/jobs.go
git commit -m "chore(models): add JobItemStatusCancelled; remove igdb_failed; add ExternalGameID to JobItem"
```

---

## Done

After Task 3 the schema and Go structs are fully aligned with `docs/sync.md`. The overall codebase will not compile (`internal/worker/tasks/sync.go`, `internal/api/sync.go`, and `internal/api/jobs.go` still reference removed fields) — this is expected. A follow-up session updates those callers.
