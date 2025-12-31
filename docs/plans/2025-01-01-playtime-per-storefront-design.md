# Playtime Per Storefront Design

## Overview

Store playtime per storefront instead of a single value per game. This enables accurate tracking when a user owns the same game on multiple storefronts (e.g., Steam and Epic), and allows automatic sync from services like Steam.

## Current State

- `UserGame.hours_played` stores a single integer for total playtime
- `UserGamePlatform` links games to platforms/storefronts but has no playtime field
- All copies of a game share the same `hours_played` value

## Design

### Data Model Changes

**Add `hours_played` to `UserGamePlatform`:**

```python
class UserGamePlatform(SQLModel, table=True):
    # ... existing fields ...

    hours_played: int = Field(default=0, ge=0)  # Playtime for this storefront
```

**Keep `UserGame.hours_played` as legacy fallback:**

- Existing values preserved for backward compatibility
- Once platform-specific playtime exists, aggregate is computed from platforms
- No migration of existing data (would require guessing which platform)

### Aggregate Calculation

Total playtime = sum of all `UserGamePlatform.hours_played` values.

```python
def get_total_hours_played(user_game: UserGame) -> int:
    platform_hours = sum(p.hours_played for p in user_game.platforms)

    # Fall back to legacy value if no platform-specific playtime
    if platform_hours == 0 and user_game.hours_played > 0:
        return user_game.hours_played

    return platform_hours
```

### API Changes

**UserGamePlatformCreate/Update:**
```python
hours_played: int = 0
```

**UserGamePlatformResponse:**
```python
hours_played: int
storefront_display_name: Optional[str]
```

**UserGameResponse:**
```python
hours_played: int  # Aggregate (sum of all platforms)
platforms: List[UserGamePlatformResponse]  # Each includes its hours_played
```

### Frontend Display

**Game detail view - playtime breakdown:**
```
Total Playtime: 80 hours
├─ Steam: 50 hours
├─ PlayStation Store: 20 hours
└─ Epic Games: 10 hours
```

**Game card/list view:**
- Shows aggregate only (80 hours)
- Breakdown visible in detail view

**Editing playtime:**
- Per-platform editing only (no game-level playtime field)
- Games with no platforms show 0 hours
- User must add a platform to track playtime

### Steam Sync Integration

**Import behavior:**
1. Steam API returns `playtime_forever` (in minutes)
2. Convert to hours, store on `UserGamePlatform` where `storefront = "steam"`
3. Subsequent syncs overwrite with latest Steam value

**Edit permissions:**
- **Steam sync enabled:** `hours_played` is read-only for Steam entries
  - Show "Synced from Steam" indicator or lock icon
  - Tooltip: "Disable Steam sync to edit manually"
- **Steam sync disabled:** User can manually edit Steam playtime
- **Other storefronts:** Always editable

### Collection Stats

- `total_hours_played` uses same aggregate logic across all games
- Future: breakdown by storefront across entire collection

## Migration

1. Add `hours_played` column to `user_game_platforms` table (default 0)
2. Existing `UserGame.hours_played` values remain untouched
3. Aggregate logic handles fallback to legacy values
4. No automatic data migration required

## Future Considerations

- PlayStation, Xbox, GOG sync would follow same pattern as Steam
- Each sync source controls its own storefront's playtime when enabled
