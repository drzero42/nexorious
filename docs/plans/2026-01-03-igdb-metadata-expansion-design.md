# IGDB Metadata Expansion Design

## Overview

Expand IGDB integration to pull in Game Modes, Themes, and Player Perspectives as separate metadata fields. Add filterable dropdowns for these fields in an expandable filter section on the games page.

## Decisions

- **Storage approach:** Comma-separated strings (consistent with existing `genre` field)
- **Relationship to tags:** Completely separate - these are system-level metadata, not user-curated tags
- **Filter dropdown population:** Only show values that exist in the user's collection
- **Filter UX:** Expandable section with primary filters always visible, secondary filters in collapsible area
- **Existing games:** New fields will be null until metadata is refreshed via maintenance job

## Data Model Changes

### New Game Model Fields

Add to `backend/app/models/game.py`:

```python
game_modes: Optional[str] = Field(default=None, max_length=500)
themes: Optional[str] = Field(default=None, max_length=500)
player_perspectives: Optional[str] = Field(default=None, max_length=500)
```

Each stores comma-separated values like `"Single player, Multiplayer, Co-operative"`.

### Database Migration

Run `alembic revision --autogenerate -m "add game modes themes perspectives"` to create migration adding three nullable string columns to the `games` table.

### IGDB Query Update

Add to the IGDB query fields:

```
game_modes.name, themes.name, player_perspectives.name
```

### Parser Update

Extract names from each array and join with comma separator, matching the existing genre handling pattern.

## Backend API Changes

### Schema Updates

Add the three new fields to `GameResponse` and `GameBase` schemas in `backend/app/schemas/game.py`.

### Filter Parameters

Add filter parameters to the user-games list endpoint for substring matching:

```python
game_modes: Optional[str] = None
themes: Optional[str] = None
player_perspectives: Optional[str] = None
```

Filter using: `WHERE game_modes LIKE '%Co-operative%'`

### Filter Options Endpoint

New endpoint to populate filter dropdowns:

```
GET /api/user-games/filter-options
```

Response:

```json
{
  "game_modes": ["Single player", "Multiplayer", "Co-operative"],
  "themes": ["Horror", "Sci-fi", "Fantasy"],
  "player_perspectives": ["First person", "Third person"],
  "genres": ["Action", "RPG", "Adventure"]
}
```

This endpoint queries the user's games, splits comma-separated values, deduplicates, and returns sorted lists.

## Frontend Filter UX

### Layout Structure

```
+---------------------------------------------------------------------+
| Primary row (always visible):                                       |
| [Status v]  [Platform v]  [Search...          ]  [More filters (6)] |
+---------------------------------------------------------------------+
| Expanded section (when toggled open):                               |
| [Genre v]  [Game Modes v]  [Themes v]  [Player Perspectives v]      |
| [Storefront v]  [Tags v]                                            |
+---------------------------------------------------------------------+
```

### Primary Filters (Always Visible)

- Status
- Platform
- Search

### Secondary Filters (In Expandable Section)

- Genre
- Game Modes
- Themes
- Player Perspectives
- Storefront
- Tags

### "More Filters" Button Behavior

- Shows count of hidden filters: "More filters (6)"
- When any hidden filter is active, shows: "More filters (2 active)"
- Click toggles expanded section visibility
- Uses chevron icon (down when collapsed, up when expanded)

### Expanded Section Behavior

- Animates open/closed with subtle slide down
- Filters wrap to multiple rows based on viewport width
- Collapsed state persisted in localStorage

### Dropdown Population

- On page load, fetch `/api/user-games/filter-options`
- Populate each dropdown with only values that exist in user's collection
- Cache this data, invalidate when games are added/removed

## Metadata Refresh Job

### Job Type Addition

Add to `BackgroundJobType` enum: `MAINTENANCE`

### Fan-Out Pattern

1. **Endpoint:** `POST /api/admin/jobs/refresh-game-metadata`
   - Creates a Job with type `MAINTENANCE`, source `SYSTEM`
   - Enqueues `maintenance.dispatch_metadata_refresh` task
   - Returns job ID immediately for polling

2. **Dispatch task:** `maintenance.dispatch_metadata_refresh`
   - Fetches all games from the database
   - Creates a JobItem for each game (item_key = game ID)
   - Enqueues `maintenance.process_metadata_refresh` for each item
   - Updates `job.total_items`

3. **Process task:** `maintenance.process_metadata_refresh`
   - Fetches fresh IGDB data for the single game using existing `igdb_id`
   - Updates all IGDB-sourced fields (including new ones)
   - Marks JobItem as `COMPLETED` or `FAILED`
   - Checks if all items done, marks Job complete

### Files to Create

- `backend/app/worker/tasks/maintenance/dispatch_metadata_refresh.py`
- `backend/app/worker/tasks/maintenance/process_metadata_refresh.py`

## Implementation Summary

### Backend Changes

1. Add three fields to Game model (`game_modes`, `themes`, `player_perspectives`)
2. Run `alembic revision --autogenerate` to create migration
3. Update IGDB query to fetch `game_modes.name`, `themes.name`, `player_perspectives.name`
4. Update IGDB parser to extract and comma-join these fields
5. Update game schemas to include new fields in responses
6. Add filter parameters to user-games list endpoint
7. Create `/api/user-games/filter-options` endpoint
8. Add `MAINTENANCE` to BackgroundJobType enum
9. Create metadata refresh dispatch and process tasks
10. Create admin endpoint to trigger metadata refresh job

### Frontend Changes

1. Update game types to include new fields
2. Create expandable filter section component
3. Refactor filter bar to separate primary/secondary filters
4. Add new filter dropdowns for game modes, themes, player perspectives
5. Fetch and cache filter options from new endpoint
6. Persist expand/collapse state in localStorage
7. Show active filter count on "More filters" button
