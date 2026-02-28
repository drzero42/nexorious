# Storefront Sync & ExternalGame Model Design

## Overview

Implement the persistent `ExternalGame` model to serve as the source of truth for all storefront sync data. This replaces the transient sync approach (where IGDB resolution is re-computed on every sync) with a durable record per external library entry. It also consolidates `IgnoredExternalGame` into `ExternalGame.is_skipped` and enables correct co-existence of manually added games and synced games.

This document supersedes `2026-01-12-external-game-model-design.md`, which proposed the same core model but was never implemented. The design here incorporates refinements from further discussion, particularly around ownership status precedence and subscription lapse handling.

## Problem Statement

The current sync system has several limitations:

1. **IGDB resolution is re-computed on every sync** — no persistent storage of user-chosen or auto-matched IGDB IDs; a wrong match can't be "remembered"
2. **Removed games have no history** — games removed from a sync source disappear entirely with no record
3. **Skip tracking is a separate model** — `IgnoredExternalGame` is disconnected from the sync flow and requires its own lookup on every run
4. **Manual and synced entries can't coexist cleanly** — if a user manually adds a game and then connects a storefront that has the same game, there is no automatic linking; the game may be duplicated or the sync entry dropped
5. **Subscription lapse is not tracked** — when a PS Plus Extra game is removed from the subscription library, the user's ownership status is not updated automatically
6. **Sync can corrupt ownership status** — if a purchased game appears in a subscription catalog, PSN returns `is_subscription = True` and sync can downgrade `OWNED` to `SUBSCRIPTION`

## Solution

Create a persistent `ExternalGame` model that:

- Stores the IGDB resolution for each external library entry (automatic or user-chosen), so it is never re-computed
- Tracks subscription status and availability from the source, enabling automatic ownership status updates when subscriptions lapse
- Replaces `IgnoredExternalGame` with an `is_skipped` flag
- Persists even when games are removed from the sync source (`is_available = False`)
- Links to `UserGamePlatform` via a nullable FK, enabling seamless auto-linking of manually added games

## Data Models

### ExternalGame (New)

```python
class ExternalGame(SQLModel, table=True):
    __tablename__ = "external_games"

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    storefront: str = Field(foreign_key="storefronts.name", index=True)
    external_id: str = Field(max_length=200)           # Platform-specific ID (Steam AppID, PSN title_id, etc.)
    title: str = Field(max_length=500)                 # Game name as reported by source

    # IGDB resolution state
    resolved_igdb_id: int | None = Field(default=None, foreign_key="games.id")
    is_skipped: bool = Field(default=False)            # User chose to ignore this game

    # Source state — always reflects what the platform last reported
    is_available: bool = Field(default=True)           # Still in user's library on this storefront
    is_subscription: bool = Field(default=False)       # Came from a subscription (PS Plus Extra, etc.)
    playtime_hours: int = Field(default=0)
    ownership_status: OwnershipStatus | None = Field(default=None)
    platform: str | None = Field(default=None, foreign_key="platforms.name")

    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    __table_args__ = (
        UniqueConstraint("user_id", "storefront", "external_id"),
    )

    @property
    def store_url(self) -> str | None:
        """Compute store URL from storefront + external_id."""
        return build_store_url(self.storefront, self.external_id)
```

### UserGamePlatform (Modified)

Two fields added, two removed:

| Change | Field | Notes |
|--------|-------|-------|
| **Add** | `external_game_id: str \| None` | FK → `external_games.id`; `NULL` = manually added entry |
| **Add** | `sync_from_source: bool = True` | When `False`, sync will not overwrite `playtime_hours` or `ownership_status` |
| **Remove** | `store_game_id` | Moves to `ExternalGame.external_id` |
| **Remove** | `store_url` | Moves to `ExternalGame.store_url` (computed property) |

All other `UserGamePlatform` fields are unchanged.

### IgnoredExternalGame (Deprecated)

Replaced by `ExternalGame.is_skipped = True`. All existing rows are migrated and the table is dropped.

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| ExternalGame scope | Per-user | Each user has their own library state |
| Store URL | Computed from `external_id` + `storefront` | URL patterns rarely change; avoids storing redundant data |
| Relationship direction | `UserGamePlatform` → `ExternalGame` (nullable FK) | Manual entries have `NULL` FK; easy to check if an entry is synced |
| Manual-then-sync matching | Match by `(user_id, IGDB game, platform, storefront)` | IGDB ID is the stable match key |
| Auto-link behaviour | Silent — ExternalGame values win | No user prompt; playtime and ownership_status are overwritten from ExternalGame |
| Ownership status on sync | Never downgrade — precedence: `OWNED > SUBSCRIPTION > BORROWED/RENTED > NO_LONGER_OWNED` | Prevents subscription catalog inclusion from corrupting a purchased game's status |
| Subscription lapse | Auto-downgrade `SUBSCRIPTION` → `NO_LONGER_OWNED` when `is_available` becomes `False` | Only fires when current status is exactly `SUBSCRIPTION`; `OWNED` is never touched |
| Review blocking | Non-blocking — high-confidence matches sync immediately | Low-confidence games queue for async review; the rest of the library is not held back |
| Deleting synced game | Delete `UserGamePlatform` + set `ExternalGame.is_skipped = True` | Reversible; game won't re-import on next sync |

## Sync Flow

Five sequential phases each time a sync runs for a `(user, storefront)` pair.

### Phase 1 — Fetch

The storefront adapter fetches the full current library. Returns a list of transient `ExternalGame` dataclass objects (the existing `adapters/base.py` dataclass; name clash with the new model is resolved by renaming the dataclass to `ExternalLibraryEntry` or similar).

### Phase 2 — Upsert ExternalGame Records

For each game from the source, upsert the persistent `ExternalGame` by `(user_id, storefront, external_id)`:
- If it exists: update `title`, `playtime_hours`, `is_subscription`, `ownership_status`, `platform`. Set `is_available = True`.
- If it does not exist: create a new record.

### Phase 3 — Mark Removed Games & Handle Subscription Lapses

Any `ExternalGame` for this `(user_id, storefront)` that was **not** in the source response:
- Set `is_available = False`.
- If `is_subscription = True` and the linked `UserGamePlatform.ownership_status == SUBSCRIPTION`: set `UserGamePlatform.ownership_status = NO_LONGER_OWNED`.
- If `UserGamePlatform.ownership_status` is anything other than `SUBSCRIPTION` (e.g. `OWNED`): do not touch it.

### Phase 4 — Resolve Unresolved ExternalGames

For each `ExternalGame` where `resolved_igdb_id IS NULL`, `is_skipped = False`, and `is_available = True`:
- Run the IGDB matching service.
- **Confidence ≥ 85%**: set `resolved_igdb_id` immediately. The record proceeds to Phase 5 in this same sync run.
- **Confidence < 85%**: create a `JobItem` in `PENDING_REVIEW` with candidates. User resolves asynchronously; the next sync run (or a retry trigger) picks it up.

### Phase 5 — Sync to Collection

For each `ExternalGame` where `resolved_igdb_id IS NOT NULL`, `is_skipped = False`, and `is_available = True`:

1. Find any linked `UserGamePlatform` via `external_game_id` FK.
2. **No link exists** — look for an existing `UserGamePlatform` matching `(user_id, IGDB game, platform, storefront)` (a manually added entry). If found: link it, then apply the field update rules below. If not found: create `UserGame` (if none exists for this user + IGDB game) + `UserGamePlatform`, then link.
3. **Link exists** and `sync_from_source = True` — apply field update rules below.
4. **Link exists** and `sync_from_source = False` — skip, leave user's values unchanged.

**Field update rules when writing to `UserGamePlatform`:**
- Always overwrite `playtime_hours` from `ExternalGame`.
- Overwrite `ownership_status` only if the incoming value is equal or higher precedence than the current value (`OWNED > SUBSCRIPTION > BORROWED/RENTED > NO_LONGER_OWNED`). Never downgrade.

## User-Facing Behaviours

| Action | What happens |
|--------|-------------|
| **Skip a game during review** | `ExternalGame.is_skipped = True`. No `UserGamePlatform` created. Game won't reappear in future reviews. |
| **Un-skip a game** | `ExternalGame.is_skipped = False`. If already resolved → `UserGamePlatform` created on next sync. If unresolved → re-enters review queue. |
| **Re-map to different IGDB game** | Update `ExternalGame.resolved_igdb_id`. Move linked `UserGamePlatform` to the correct `UserGame`. Old `UserGame` is retained (may have other platforms). |
| **Delete a synced game from collection** | Delete `UserGamePlatform`. Set `ExternalGame.is_skipped = True`. Game will not re-import on next sync. |
| **Stop sync overwriting values** | Set `UserGamePlatform.sync_from_source = False`. ExternalGame still receives updates from source; collection values are frozen. |
| **Manually add a game also in a sync source** | Manual entry created normally (`external_game_id = NULL`). On next sync Phase 5 finds the match by `(user_id, IGDB game, platform, storefront)`, links them silently, and ExternalGame values win per the field update rules. |
| **View store page** | URL computed from `ExternalGame.store_url`. Surfaced wherever the platform entry is displayed in the UI. |

## Migration Strategy

Four Alembic migrations applied in order. All generated via `--autogenerate`.

### Migration 1 — Create `external_games` table
New table with all fields defined above. No data changes.

### Migration 2 — Add columns to `user_game_platforms`
Add `external_game_id` (nullable FK → `external_games.id`) and `sync_from_source` (bool, default `True`). No data changes.

### Migration 3 — Migrate `ignored_external_games` to `ExternalGame`
For each `IgnoredExternalGame` row, create an `ExternalGame` with:
- `is_skipped = True`
- `resolved_igdb_id = NULL`
- `is_available = True`
- `storefront` mapped from `BackgroundJobSource` → storefront name
- Other source fields set to defaults

Drop the `ignored_external_games` table.

### Migration 4 — Drop `store_game_id` and `store_url` from `user_game_platforms`
These columns move to `ExternalGame`. Existing `UserGamePlatform` rows that had a `store_game_id` do **not** get `ExternalGame` records backfilled by migration — they will be created naturally on first sync (Phase 2 upserts them, Phase 5 links them). During the window between migration and first sync, affected rows temporarily have no store URL or subscription status. This is acceptable.

### Post-migration first sync
Existing `UserGamePlatform` rows have `external_game_id = NULL`. When Phase 2 creates `ExternalGame` records and Phase 5 runs, it finds the existing `UserGamePlatform` by `(user_id, IGDB game, platform, storefront)` and links them automatically. After the first sync completes all synced entries are linked.

## API Changes

### New endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/external-games` | List user's ExternalGames. Filterable by `storefront`, `is_skipped`, `is_available`, `resolved` (bool). Includes computed `store_url`. |
| `PATCH` | `/external-games/{id}` | Update `resolved_igdb_id` or `is_skipped`. Used by review UI and library management. |

### Modified endpoints

- `UserGamePlatform` response schemas gain `external_game_id`, `sync_from_source`, and a nested or linked `store_url` from `ExternalGame`.
- `PATCH /user-game-platforms/{id}` gains `sync_from_source` as a writable field.
- `store_game_id` and `store_url` removed from `UserGamePlatform` request/response schemas.

### Removed

- Endpoints that read/write `IgnoredExternalGame` (ignore/unignore during review) are replaced by `PATCH /external-games/{id}` with `{ "is_skipped": true/false }`.

### Unchanged

- Sync trigger endpoints (`POST /sync/...`) — changes are internal to the worker.
- Review flow endpoints — `JobItem` candidates are surfaced the same way; resolving a `JobItem` now writes `ExternalGame.resolved_igdb_id` rather than queuing another IGDB lookup.

## Files to Change

| Change | Path |
|--------|------|
| **New** | `backend/app/models/external_game.py` |
| **Modified** | `backend/app/models/user_game.py` — add `external_game_id`, `sync_from_source`; remove `store_game_id`, `store_url` |
| **Deprecated** | `backend/app/models/ignored_external_game.py` — deleted after migration |
| **Modified** | `backend/app/worker/tasks/sync/adapters/base.py` — rename transient dataclass to `ExternalLibraryEntry` |
| **Modified** | `backend/app/worker/tasks/sync/dispatch.py` — upsert ExternalGame in Phase 2 |
| **Modified** | `backend/app/worker/tasks/sync/process_item.py` — rewrite to use ExternalGame; remove IgnoredExternalGame lookups |
| **New** | `backend/app/api/external_games.py` — new router |
| **Modified** | `backend/app/schemas/` — add ExternalGame schemas; update UserGamePlatform schemas |
| **New** | Alembic migration files (4, via `--autogenerate`) |
| **Modified** | `backend/app/tests/` — update and extend sync tests |
