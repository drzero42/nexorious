# External Game Model Design

## Overview

Promote `ExternalGame` from a transient dataclass (used only during sync) to a persistent SQLModel that serves as the source of truth for sync data. This consolidates `IgnoredExternalGame` and enables better tracking of sync state, IGDB resolution, and subscription status.

## Problem Statement

Current sync system limitations:

1. **IGDB resolution is re-computed** - No persistent storage of user-chosen or auto-matched IGDB IDs
2. **Re-mapping is difficult** - Changing a wrong IGDB match requires manual intervention
3. **Subscription tracking is transient** - No way to track which games are part of subscriptions over time
4. **Removed games have no history** - Games removed from sync source disappear entirely
5. **Skip tracking is separate** - `IgnoredExternalGame` model exists separately from sync flow

## Solution

Create a persistent `ExternalGame` model that:

- Stores IGDB ID resolution (automatic or user-chosen)
- Tracks subscription status from source
- Replaces `IgnoredExternalGame` with an `is_skipped` flag
- Persists even when games are removed from sync source (via `is_available` flag)
- Links to `UserGamePlatform` to enable seamless re-mapping

## Data Models

### ExternalGame (New Model)

```python
class ExternalGame(SQLModel, table=True):
    __tablename__ = "external_games"

    id: str                                    # UUID PK
    user_id: str                               # FK to users.id
    storefront: str                            # FK to storefronts.name
    external_id: str                           # Platform-specific ID
    title: str                                 # Game name from source

    # Resolution state
    resolved_igdb_id: int | None = None        # FK to games.igdb_id
    is_skipped: bool = False                   # User chose to ignore

    # Source state (always reflects what platform reports)
    is_available: bool = True                  # Still in user's library
    is_subscription: bool = False              # Subscription game
    playtime_hours: int = 0                    # Playtime from source
    ownership_status: OwnershipStatus | None = None

    # Platform info
    platform: str | None = None                # e.g., "pc-windows"

    # Timestamps
    created_at: datetime
    updated_at: datetime

    # Constraints
    __table_args__ = (
        UniqueConstraint("user_id", "storefront", "external_id"),
    )

    # Computed property for store URL
    @property
    def store_url(self) -> str | None:
        return build_store_url(self.storefront, self.external_id)
```

### UserGamePlatform (Modified)

```python
class UserGamePlatform(SQLModel, table=True):
    # ... existing fields stay ...

    # New fields
    external_game_id: str | None = None        # FK to external_games.id
    sync_from_source: bool = True              # Controls sync propagation

    # Removed fields
    # store_game_id - no longer needed, lives on ExternalGame
```

### IgnoredExternalGame

Deprecated - replaced by `ExternalGame.is_skipped`. Migration will convert existing records.

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| ExternalGame scope | Per-user (not global) | Each user has their own library state |
| Store URL | Computed from external_id + storefront | URL patterns rarely change, less storage |
| Relationship direction | UserGamePlatform → ExternalGame (nullable FK) | Manual entries have null FK, easy to check if synced |
| Manual entries | `external_game_id = NULL` | Unaffected by sync, no fake ExternalGame needed |
| Manual-then-sync matching | Link by same user + IGDB + platform + storefront | IGDB ID is the match key, no duplicates |
| Sync update control | `sync_from_source` flag per UserGamePlatform | User can stop sync updates without deleting |
| Re-mapping | Automatic - move UserGamePlatform to new UserGame | Seamless, no user confirmation needed |
| Deleting synced game | Set `ExternalGame.is_skipped = True` | Reversible, game won't re-import |
| Review candidates | Stay on JobItem (transient) | Resolution stored on ExternalGame after review |

## Sync Flow

```
1. FETCH FROM SOURCE
   - Adapter fetches games from platform (Steam, PSN, etc.)
   - Returns list of raw game data (external_id, title, playtime, etc.)

2. CREATE/UPDATE EXTERNAL GAMES
   - For each game from source:
     - Find ExternalGame by (user_id, storefront, external_id)
     - If exists: update source fields (playtime, is_subscription, is_available=True)
     - If not exists: create new ExternalGame

3. MARK REMOVED GAMES
   - Any ExternalGame for this user+storefront NOT in source fetch:
     - Set is_available = False

4. PROCESS UNRESOLVED EXTERNAL GAMES
   - For each ExternalGame where resolved_igdb_id IS NULL and is_skipped = FALSE:
     - Run matching service (platform lookup -> IGDB search)
     - If high confidence: set resolved_igdb_id
     - If low confidence: mark for review (JobItem.PENDING_REVIEW with candidates)

5. SYNC TO USER COLLECTION
   - For each ExternalGame where resolved_igdb_id IS NOT NULL and is_skipped = FALSE:
     - Find linked UserGamePlatform (via external_game_id FK)
     - If no link exists:
       - Check for existing UserGamePlatform (same user + IGDB + platform + storefront)
       - If found: link ExternalGame to it
       - If not found: create UserGame (if needed) + UserGamePlatform, link to ExternalGame
     - If link exists and sync_from_source = True:
       - Update UserGamePlatform with ExternalGame's playtime, ownership_status
```

## User Actions & Behaviors

### Skip a game during review

- Set `ExternalGame.is_skipped = True`
- No UserGamePlatform created
- Game won't appear in future reviews

### Un-skip a game

- Set `ExternalGame.is_skipped = False`
- If `resolved_igdb_id` is set: sync creates UserGamePlatform on next run
- If `resolved_igdb_id` is null: game enters review flow again

### Re-map to different IGDB game

- Update `ExternalGame.resolved_igdb_id` to new IGDB ID
- Find/create UserGame for new IGDB ID
- Move linked UserGamePlatform to new UserGame
- Old UserGame remains (may have other platforms, or gets cleaned up if empty)

### Delete synced game from collection

- Delete UserGamePlatform
- Set `ExternalGame.is_skipped = True`
- ExternalGame persists, won't re-import on next sync

### Stop syncing but keep in collection

- Set `UserGamePlatform.sync_from_source = False`
- ExternalGame still updates from source
- UserGamePlatform values no longer overwritten

### Manually add game (no sync source)

- Create UserGame + UserGamePlatform as today
- `UserGamePlatform.external_game_id = None`
- Not affected by sync

### View store page

- Compute URL from `ExternalGame.storefront + external_id`
- Display as clickable link in UI

## Migration Strategy

### Database migrations:

1. **Create `external_games` table**
   - New table with all fields from the model

2. **Add columns to `user_game_platforms`**
   - `external_game_id` (nullable FK)
   - `sync_from_source` (bool, default True)

3. **Migrate IgnoredExternalGame data**
   - For each `IgnoredExternalGame`:
     - Create `ExternalGame` with `is_skipped = True`, `resolved_igdb_id = NULL`
   - Drop `ignored_external_games` table

4. **Drop store_game_id from user_game_platforms**
   - Remove the column entirely

### First sync after migration:

- Existing UserGamePlatforms have `external_game_id = NULL`
- Sync creates ExternalGame records from source
- Sync matches ExternalGame to existing UserGamePlatform (same user + IGDB + platform + storefront)
- Links are established automatically

Until first sync runs, existing synced UserGamePlatforms won't have store URL or subscription status - this is acceptable since data will populate on next sync.

## API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /external-games` | List user's ExternalGames (filterable by storefront, is_skipped, is_available, resolved status) |
| `PATCH /external-games/{id}` | Update resolved_igdb_id, is_skipped |
| `GET /external-games/{id}/store-url` | Get computed store URL (or include in model response) |

## UI Considerations

### Review Flow Changes

- Review UI queries ExternalGames where `resolved_igdb_id IS NULL` and `is_skipped = FALSE`
- Candidates still come from JobItem during active sync
- User resolves -> updates `ExternalGame.resolved_igdb_id`

### Collection UI Changes

- UserGamePlatform display can show "synced" badge if `external_game_id` is set
- "View in store" button uses ExternalGame's computed URL
- "Stop syncing" toggle maps to `sync_from_source` flag
- Subscription games can show badge based on `ExternalGame.is_subscription`

### Library Management UI (New)

- View all ExternalGames for a storefront
- See which are: resolved, unresolved, skipped, unavailable
- Bulk actions: skip multiple, re-resolve, un-skip

## Files to Change

- **New:** `models/external_game.py`
- **Modified:** `models/user_game.py` (UserGamePlatform)
- **Modified:** `worker/tasks/sync/` (dispatch, process_item, adapters)
- **Modified:** `services/matching/` (update to work with ExternalGame)
- **Deprecated:** `models/ignored_external_game.py`
- **New:** Migration files (via alembic autogenerate)
- **New/Modified:** API routes for ExternalGame management
