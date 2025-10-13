# API Specification

This is the API specification for the spec detailed in @.agent-os/specs/2025-09-11-steam-import-sync-refactor/spec.md

> Created: 2025-09-11
> Version: 1.0.0

## Purpose

This API specification outlines the changes needed to support the new Steam import sync refactoring approach. The key changes are: 1) Removing the redundant `game_id` field from `steam_games` table and using `igdb_id` instead, and 2) Moving from `game_id` presence-based sync status determination to `user_game_platforms` association-based sync status. This refactoring works within the existing Steam import API structure.

### Endpoint Rationale

The refactored approach modifies the existing Steam import endpoints to:
- Use `igdb_id` instead of `game_id` internally in steam_games references
- Understand sync status based on `user_game_platforms` associations rather than `game_id` presence
- Provide sync/unsync operations through the existing Steam game management endpoints
- Use the generic `is_game_synced()` function with Steam platform/storefront parameters

### Integration Benefits

This API design integrates with the new sync status features by:
- Maintaining existing Steam import API endpoints and workflows
- Using `user_game_platforms` associations for accurate sync determination
- Supporting the planned import framework through reusable sync functions
- Preserving backward compatibility of API contracts

## Existing Steam Import API Endpoints

**Base URL**: `/api/import/sources/steam`

### Status & Configuration Endpoints

#### `GET /api/import/sources/steam/availability`
**Purpose**: Check if Steam import feature is available for the current user
**Refactor Impact**: No changes to API contract, internal logic updated

#### `GET /api/import/sources/steam/status`
**Purpose**: Get Steam import source status
**Refactor Impact**: No changes to API contract, internal logic updated

#### `GET /api/import/sources/steam/config`
**Purpose**: Get current Steam configuration
**Refactor Impact**: No changes to API contract

#### `PUT /api/import/sources/steam/config`
**Purpose**: Update Steam configuration
**Refactor Impact**: No changes to API contract

#### `DELETE /api/import/sources/steam/config`
**Purpose**: Delete Steam configuration
**Refactor Impact**: No changes to API contract

#### `POST /api/import/sources/steam/verify`
**Purpose**: Verify Steam configuration without saving
**Refactor Impact**: No changes to API contract

#### `POST /api/import/sources/steam/resolve-vanity`
**Purpose**: Resolve Steam vanity URL to Steam ID
**Refactor Impact**: No changes to API contract

### Library Operations

#### `GET /api/import/sources/steam/library`
**Purpose**: Get preview of Steam library
**Refactor Impact**: No changes to API contract

#### `GET /api/import/sources/steam/games`
**Purpose**: List imported Steam games with filtering
**Refactor Impact**: Internal sync status determination updated to use `user_game_platforms` associations

**Response Changes**:
```json
{
  "games": [
    {
      "id": "uuid-string",
      "external_id": "123456",
      "name": "Game Name",
      "igdb_id": 1942,
      "igdb_title": "IGDB Game Title", 
      "game_id": null,  // REMOVED - no longer used
      "user_game_id": "uuid-string",
      "ignored": false,
      "created_at": "2025-09-11T10:00:00Z",
      "updated_at": "2025-09-11T10:00:00Z"
    }
  ]
}
```

#### `POST /api/import/sources/steam/games/import`
**Purpose**: Start Steam library import
**Refactor Impact**: Internal sync logic updated to use `user_game_platforms` associations

### Individual Game Operations

#### `PUT /api/import/sources/steam/games/{game_id}/match`
**Purpose**: Match Steam game to IGDB entry
**Refactor Impact**: Uses `igdb_id` instead of `game_id` for steam_games updates

#### `POST /api/import/sources/steam/games/{game_id}/auto-match`
**Purpose**: Automatically match Steam game to IGDB
**Refactor Impact**: Uses `igdb_id` instead of `game_id` for steam_games updates

#### `POST /api/import/sources/steam/games/{game_id}/sync`
**Purpose**: Sync Steam game to main collection
**Refactor Impact**: **Major changes to internal sync logic**

**Internal Logic Changes**:
```python
# Old logic (game_id presence-based)
def sync_game(user_id: str, game_id: str):
    steam_game = get_steam_game(user_id, game_id)
    if steam_game.game_id is None:
        # Create user_game and set game_id
        user_game = create_user_game(user_id, steam_game.igdb_id)
        steam_game.game_id = user_game.game_id
    return steam_game

# New logic (user_game_platforms association-based)
def sync_game(user_id: str, game_id: str):
    steam_game = get_steam_game(user_id, game_id)
    
    # Use generic sync function to check current status
    if not is_game_synced(user_id, steam_game.igdb_id, "pc-windows", "steam"):
        # Create user_game if it doesn't exist
        user_game = get_or_create_user_game(user_id, steam_game.igdb_id)
        
        # Create user_game_platforms association
        create_user_game_platform(
            user_game.id,
            platform_id="pc-windows",
            storefront_id="steam", 
            store_game_id=steam_game.external_id
        )
    
    return steam_game
```

#### `POST /api/import/sources/steam/games/{game_id}/unsync`
**Purpose**: Remove Steam game from main collection
**Refactor Impact**: **Major changes to internal unsync logic**

**Internal Logic Changes**:
```python
# Old logic (remove game_id)
def unsync_game(user_id: str, game_id: str):
    steam_game = get_steam_game(user_id, game_id)
    steam_game.game_id = None
    return steam_game

# New logic (remove user_game_platforms association)
def unsync_game(user_id: str, game_id: str):
    steam_game = get_steam_game(user_id, game_id)
    
    # Remove Steam platform/storefront association
    remove_user_game_platform(
        user_id,
        steam_game.igdb_id,
        platform_id="pc-windows",
        storefront_id="steam"
    )
    
    return steam_game
```

#### `PUT /api/import/sources/steam/games/{game_id}/ignore`
**Purpose**: Toggle ignore status of Steam game
**Refactor Impact**: No changes to API contract

### Bulk Operations

#### `POST /api/import/sources/steam/games/auto-match`
**Purpose**: Auto-match all unmatched Steam games
**Refactor Impact**: Uses `igdb_id` instead of `game_id` for steam_games updates

#### `POST /api/import/sources/steam/games/sync`
**Purpose**: Sync all matched Steam games to collection
**Refactor Impact**: **Major changes to bulk sync logic**

**Internal Logic Changes**:
- Uses generic `is_game_synced()` function for each game
- Creates `user_game_platforms` associations instead of setting `game_id`

#### `POST /api/import/sources/steam/games/unsync`
**Purpose**: Remove all synced Steam games from collection
**Refactor Impact**: **Major changes to bulk unsync logic**

**Internal Logic Changes**:
- Removes `user_game_platforms` associations instead of clearing `game_id`

#### `PUT /api/import/sources/steam/games/unignore-all`
**Purpose**: Unignore all ignored Steam games
**Refactor Impact**: No changes to API contract

#### `PUT /api/import/sources/steam/games/unmatch-all`
**Purpose**: Remove IGDB matches from all Steam games
**Refactor Impact**: Clears `igdb_id` instead of `game_id`

## Controllers

### Steam Import Service Changes

#### Core Logic Refactoring

**Generic Sync Function Implementation**:
```python
def is_game_synced(user_id: str, igdb_id: int, platform_id: str, storefront_id: str) -> bool:
    """
    Generic function to check if a game is synced for a specific platform/storefront combination.
    
    Returns True if user_game_platforms association exists, False otherwise.
    """
    return session.query(UserGamePlatform)\
        .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)\
        .filter(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,  # igdb_id is game PK
            UserGamePlatform.platform_id == platform_id,
            UserGamePlatform.storefront_id == storefront_id
        ).first() is not None

# Steam-specific wrapper
def is_steam_game_synced(user_id: str, igdb_id: int) -> bool:
    """Check if a Steam game is synced using the generic function."""
    steam_platform_id = get_platform_id("pc-windows")
    steam_storefront_id = get_storefront_id("steam")
    return is_game_synced(user_id, igdb_id, steam_platform_id, steam_storefront_id)
```

#### Database Migration Impact

**SteamGame Model Changes**:
```python
# Before refactor
class SteamGame(SQLModel, table=True):
    igdb_id: Optional[int] = Field(default=None)
    game_id: Optional[int] = Field(default=None)  # REMOVE THIS

# After refactor  
class SteamGame(SQLModel, table=True):
    igdb_id: Optional[int] = Field(default=None)
    # game_id field removed entirely
```

#### Error Handling Updates

**Sync Operation Error Handling**:
1. **Platform/Storefront Lookup Errors**:
   ```python
   steam_platform = get_platform_by_name("pc-windows")
   steam_storefront = get_storefront_by_name("steam")
   if not steam_platform or not steam_storefront:
       raise ConfigurationError("Steam platform/storefront not found in database")
   ```

2. **Association Conflicts**:
   ```python
   existing_ugp = get_user_game_platform_by_store_id(user_id, platform_id, storefront_id, steam_app_id)
   if existing_ugp and existing_ugp.user_game.game_id != igdb_id:
       raise ConflictError(f"Steam app {steam_app_id} already associated with different game")
   ```

3. **Migration Compatibility**:
   ```python
   # Handle cases where steam_games still references old game_id during transition
   try:
       sync_status = is_steam_game_synced(user_id, steam_game.igdb_id)
   except AttributeError:
       # Fallback for incomplete migration
       logger.warning(f"Steam game {steam_game.id} missing igdb_id during migration")
       sync_status = False
   ```

## Summary of Changes

### API Contract Changes:
- **Minimal external changes** - existing endpoints preserved
- **Response model updates** - remove `game_id` field from Steam game responses
- **Internal logic overhaul** - sync determination based on `user_game_platforms`

### Internal Implementation Changes:
- **Generic sync function** - reusable across all import sources
- **Database migration** - remove `steam_games.game_id` column
- **Association-based sync** - use `user_game_platforms` instead of `game_id` presence
- **Error handling updates** - handle new association conflicts and edge cases

### Benefits:
- **Framework foundation** - supports planned import framework for other storefronts
- **Accurate sync status** - based on actual platform/storefront associations
- **Maintainable code** - centralized sync logic with consistent patterns
- **Minimal breaking changes** - preserves existing API contracts where possible