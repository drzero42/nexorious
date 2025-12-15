# Import/Sync Refactor Design

## Overview

This document describes the refactored import and sync architecture for Nexorious. The key distinction:

- **Import** = One-time operation from a file (CSV, JSON)
- **Sync** = Recurring operation from an external API (Steam, potentially Epic/GOG)

## Data Model Changes

### Tables to Remove

1. **`SteamGame`** - No longer needed. Steam library data flows directly through `Job` → `ReviewItem` → `UserGamePlatform`.

2. **`ImportJob`** - Already replaced by unified `Job` model.

### Tables to Add

**`IgnoredExternalGame`** - Tracks games user explicitly doesn't want synced:

```python
class IgnoredExternalGame(SQLModel, table=True):
    id: str  # UUID, primary key
    user_id: str  # foreign key to users
    source: BackgroundJobSource  # STEAM, EPIC, GOG
    external_id: str  # Steam AppID, Epic ID, etc.
    title: str  # for display purposes
    created_at: datetime

    # Unique constraint: (user_id, source, external_id)
```

### Fields to Add

**`ReviewItem.match_confidence`** - Float 0.0-1.0:

| Value | Meaning |
|-------|---------|
| `1.0` | Exact title match OR single IGDB result (definite match) |
| `>= 0.85` | High confidence auto-match |
| `< 0.85` | Low confidence, needs user input |
| `null` or `0.0` | No IGDB results found (user must search manually) |

## Steam Sync Flow

### Trigger
- Manual: User clicks "Sync" button
- Scheduled: Based on `UserSyncConfig.frequency` (hourly, daily, weekly)

### Process Steps

1. **Create Job** - `job_type=SYNC`, `source=STEAM`, `status=PROCESSING`

2. **Fetch Steam Library** - Call Steam Web API to get user's games (AppID + title)

3. **Filter Already Synced** - For each Steam game:
   - Query: Does `UserGamePlatform` exist with `storefront_id='steam'` AND `store_game_id=<AppID>`?
   - If yes → Skip (already synced)
   - If no → Continue to matching

4. **Filter Ignored** - Check `IgnoredExternalGame` for `(user_id, STEAM, AppID)`
   - If found → Skip
   - If not → Continue to matching

5. **Match to IGDB** - For remaining games:
   - Search IGDB by title
   - Calculate confidence score
   - If confidence = 1.0 AND IGDB ID already in collection → Auto-link storefront (no ReviewItem)
   - Otherwise → Create `ReviewItem` with candidates and confidence

6. **Complete** - Set job `status=AWAITING_REVIEW` (or `COMPLETED` if no ReviewItems created)

### "Already Synced" Definition

A game is considered synced when `UserGamePlatform` exists with:
- `storefront_id = 'steam'`
- `store_game_id = <Steam AppID>`

## Review Flow

### Review States (per ReviewItem)

| State | Condition | User Action |
|-------|-----------|-------------|
| **Auto-matched** | `match_confidence >= 0.85`, has recommended IGDB ID | Approve, Edit match, or Ignore |
| **Needs input** | `match_confidence < 0.85` OR multiple candidates | Select match, Search manually, or Ignore |
| **No results** | `igdb_candidates` is empty | Search manually or Ignore |

### User Actions

1. **Approve** (single or batch)
   - Sets `ReviewItem.status = MATCHED`
   - Sets `ReviewItem.resolved_igdb_id` (from candidate or user selection)
   - Triggers sync for that game

2. **Edit Match**
   - User searches IGDB for different game
   - Updates `igdb_candidates` with new selection
   - Then user approves

3. **Ignore**
   - Sets `ReviewItem.status = SKIPPED`
   - Creates `IgnoredExternalGame` record
   - Game won't appear in future syncs

### Sync a Single Game (after approval)

1. Check if IGDB ID exists in `UserGame` for this user
   - If no → Add game via normal IGDB procedure (creates `Game` if needed, creates `UserGame`)
   - If yes → Skip to step 2
2. Add `UserGamePlatform` with:
   - `platform_id = 'pc-windows'`
   - `storefront_id = 'steam'`
   - `store_game_id = <Steam AppID>`

### Batch Actions

- **Approve All** - Approves all auto-matched games (confidence >= threshold)
- **Sync All Approved** - Processes all `MATCHED` ReviewItems

### UI Notification

The sync menu item shows a badge count of pending ReviewItems awaiting user input. No other notifications are sent.

## Import Flows

### Nexorious JSON Import (Trusted)

**No matching, no review** - This is our own export format with IGDB IDs.

1. **Create Job** - `job_type=IMPORT`, `source=NEXORIOUS`, `status=PROCESSING`

2. **Parse JSON** - Validate schema version, extract games

3. **For each game:**
   - Check if IGDB ID exists in user's `UserGame`
     - If yes → Add any missing platform/storefront associations
     - If no → Add game via normal IGDB procedure, then add platform/storefront associations

4. **Complete** - Set job `status=COMPLETED` with statistics

### Darkadia CSV Import (Needs Matching)

**Two-phase review:** First resolve platforms/storefronts, then review game matches.

1. **Create Job** - `job_type=IMPORT`, `source=DARKADIA`, `status=PROCESSING`

2. **Parse CSV** - Map columns flexibly, extract unique platform/storefront names

3. **Resolve Platforms & Storefronts**
   - Fuzzy match each unique name against Nexorious tables
   - High confidence → Auto-map
   - Low confidence or no match → Flag for user review

4. **Platform/Storefront Review** (if needed)
   - Present unresolved platforms/storefronts to user
   - User selects correct Nexorious equivalent for each
   - All must be resolved before proceeding to game matching

5. **Match Games to IGDB** - For each row:
   - Match title to IGDB
   - Calculate confidence score
   - If high confidence AND IGDB ID already in collection → Auto-add platform/storefront associations
   - Otherwise → Create `ReviewItem`

6. **Complete** - Set job `status=AWAITING_REVIEW` (or `COMPLETED` if no ReviewItems)

7. **Game Review & Finalize** - Same as Steam sync, but platform/storefront uses resolved IDs

**Note:** Darkadia CSV specifies storefronts but has no platform-specific IDs (like Steam AppID), so `UserGamePlatform.store_game_id` will be null for Darkadia imports.

## API Changes

### Endpoints to Remove

- `POST /import/steam` - Steam only has sync, no import

### Sync Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/sync/{platform}` | Trigger sync |
| GET | `/sync/{platform}/status` | Get sync status |
| GET | `/sync/config/{platform}` | Get sync config |
| PUT | `/sync/config/{platform}` | Update sync config |
| GET | `/sync/ignored` | List ignored external games |
| DELETE | `/sync/ignored/{id}` | Un-ignore a game |

### Import Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/import/nexorious` | Nexorious JSON import (no review) |
| POST | `/import/darkadia` | Darkadia CSV import (two-phase review) |

### Job/Review Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/jobs/{job_id}/review-items` | List ReviewItems for a job |
| PUT | `/review-items/{id}` | Update single ReviewItem |
| POST | `/jobs/{job_id}/review-items/batch` | Batch actions |
| GET | `/jobs/{job_id}/unresolved-mappings` | Get unresolved platform/storefront names (Darkadia) |
| PUT | `/jobs/{job_id}/resolve-mappings` | Submit platform/storefront mapping decisions |

## Code Cleanup

### Backend Files to Remove

- `backend/app/models/steam_game.py` - SteamGame model
- `backend/app/models/import_job.py` - ImportJob model (if still exists)
- `backend/app/worker/tasks/import_export/import_steam.py` - Steam import task
- `backend/app/api/import_api/sources/steam_batch.py` - Steam batch import API
- `backend/app/services/steam_games/` - Most of this service (refactor remaining into sync service)
- All references to SteamGame throughout codebase

### Frontend Files to Remove

- `frontend/src/routes/import/steam/` - Steam import page
- `frontend/src/lib/stores/steam-games.svelte.ts` - SteamGame store
- `frontend/src/lib/components/SteamGameCard.svelte` - SteamGame-specific component
- `frontend/src/lib/components/SteamGamesTable.svelte` - SteamGame-specific component
- `frontend/src/lib/adapters/SteamImportServiceAdapter.ts` - Steam import adapter

### Database

- Reset database after schema changes
- Alembic auto-generates migrations for:
  - New `IgnoredExternalGame` table
  - New `match_confidence` column on `ReviewItem`
  - Dropped `steam_games` table
  - Dropped `import_jobs` table (if exists)
