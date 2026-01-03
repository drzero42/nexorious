# PSN Playtime Sync Design

## Overview

Add playtime synchronization for PSN games by fetching `title_stats` from the PSNAWP library and enriching game data during library sync.

## Current State

- PSN sync fetches game library via `game_entitlements()` API
- Playtime is hardcoded to `0` in both `PSNGame` and `ExternalGame`
- Steam sync already handles playtime (returns with owned games in single API call)

## Design

### Data Flow

```
PSNService.get_library()
    ├── Fetch title_stats() → build Dict[title_id, hours]
    ├── Fetch game_entitlements() → iterate games
    ├── Look up playtime by titleId
    └── Return List[PSNGame] with playtime_hours populated
        ↓
PSNSyncAdapter.fetch_games()
    └── Use game.playtime_hours (matches Steam adapter pattern)
        ↓
Sync pipeline → UserGamePlatform.hours_played
```

### Changes

#### 1. `backend/app/services/psn.py`

**PSNGame dataclass** - Add field:
```python
playtime_hours: int = 0  # Total playtime in hours
```

**get_library() method** - Fetch and merge playtime:
```python
async def get_library(self) -> List[PSNGame]:
    client = self.psnawp.me()

    # Fetch playtime stats and build lookup by title_id
    playtime_lookup: Dict[str, int] = {}
    for stats in client.title_stats(limit=None):
        if stats.title_id and stats.play_duration:
            hours = int(stats.play_duration.total_seconds() // 3600)
            playtime_lookup[stats.title_id] = hours

    # Fetch game entitlements and enrich with playtime
    for entitlement in client.game_entitlements(limit=None):
        title_id = entitlement.get("titleMeta", {}).get("titleId", "")
        playtime = playtime_lookup.get(title_id, 0)
        # ... create PSNGame with playtime_hours=playtime
```

#### 2. `backend/app/worker/tasks/sync/adapters/psn.py`

**fetch_games() method** - Use playtime from PSNGame:
```python
playtime_hours=game.playtime_hours,  # was hardcoded to 0
```

### Notes

- `title_stats()` only returns games that have been played; unplayed games default to 0 hours (correct behavior)
- `play_duration` is already a `timedelta` (PSNAWP parses ISO 8601 format like "PT243H18M48S")
- Matches Steam adapter pattern where service layer handles API complexity
- Both APIs use `titleId` as the game identifier for matching

### Testing

- Update `test_psn_service.py` to verify playtime is fetched and merged
- Update `test_psn_sync_adapter.py` to verify playtime flows through to ExternalGame
- Mock `title_stats()` responses in tests
