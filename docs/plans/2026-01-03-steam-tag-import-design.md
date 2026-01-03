# Steam Tag Import Design

Import Steam categories as tags during game sync.

## Goals

- Auto-tag games with Steam categories (e.g., "Single-player", "Co-op", "Controller Support")
- Aid in organizing and discovering traits about games
- Tags are just tags - no distinction between imported and manually created

## Data Changes

### UserSyncConfig

Add `import_tags` boolean field (default: `true`):

```python
import_tags: bool = Field(default=True)
```

### ExternalGame

Add optional `tags` field to the dataclass:

```python
@dataclass
class ExternalGame:
    # ... existing fields ...
    tags: list[str] = field(default_factory=list)
```

## Backend Changes

### Steam Store API Integration

**Endpoint:** `https://store.steampowered.com/api/appdetails?appids={appid}`

**Data extracted:** `categories[].description` values only (not genres - IGDB handles those)

**Example categories:** "Single-player", "Multi-player", "Co-op", "Steam Achievements", "Full controller support", "Steam Cloud", "Steam Trading Cards"

### Automatic Import (During Regular Sync)

1. Steam adapter fetches owned games list
2. For each new game (not already in user's library with this appid):
   - Fetch app details from Steam Store API
   - Extract categories into `ExternalGame.tags`
3. In `process_item.py`, after game is matched and `UserGame` created:
   - Check if `import_tags` is enabled in `UserSyncConfig`
   - If enabled, for each tag: `create_or_get_tag()` then `assign_tags_to_game()`

**Key behavior:**
- Only fetches tags for new games (keeps subsequent syncs fast)
- Respects `import_tags` toggle
- Failure to fetch tags doesn't block sync

### Manual Tag Sync

**Endpoint:** `POST /api/sync/steam/sync-tags`

**Flow:**
1. User clicks "Sync Tags" button on Steam settings page
2. Backend creates a parent job
3. Fans out individual `JobItem` tasks for each user game with Steam appid
4. Worker processes items in parallel (respecting Steam Store API rate limits)
5. Each item: fetch categories, create/get tags, assign to game
6. Job progress trackable via existing job infrastructure

**Key behavior:**
- Processes all Steam-linked games (not just new ones)
- Ignores `import_tags` toggle (explicit user action)
- Additive only - never removes existing tags

### Tag Assignment Logic

- Uses existing `TagService.create_or_get_tag()` - finds existing tag by name (case-insensitive) or creates new one
- Uses existing `TagService.assign_tags_to_game()` - handles duplicate prevention
- Tags merge by name - if user has "Co-op" tag, Steam's "Co-op" uses existing tag
- New tags created with default color

## Frontend Changes

### Steam Sync Settings Page

Add two new controls:

1. **"Import tags during sync" toggle**
   - Controls `import_tags` field on `UserSyncConfig`
   - Default: enabled

2. **"Sync Tags" button**
   - Triggers `POST /api/sync/steam/sync-tags`
   - Shows loading state during processing
   - Displays progress using existing job tracking patterns
   - Success/error feedback after completion

### No Other UI Changes

Imported tags automatically appear in:
- Game detail pages (existing tag display)
- Tag filters (existing filter UI)
- Tag management page (existing tag list)

## Behavior Summary

| Scenario | Which games | Respects toggle | When runs |
|----------|-------------|-----------------|-----------|
| Automatic import | New games only | Yes | During regular sync |
| Manual "Sync Tags" | All Steam-linked games | No | On button click |

**Tag handling:**
- Merge by name (case-insensitive)
- Additive only (never removes tags)
- Steam categories only (not genres)

## Future Considerations

- Epic Games tag import (when/if Epic exposes category data)
- Other sync sources as they're added
- Tag caching for Steam Store API responses (if performance becomes an issue)
