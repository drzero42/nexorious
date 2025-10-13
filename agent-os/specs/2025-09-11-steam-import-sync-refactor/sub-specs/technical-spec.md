# Technical Specification

This is the technical specification for the spec detailed in @.agent-os/specs/2025-09-11-steam-import-sync-refactor/spec.md

> Created: 2025-09-11
> Version: 1.0.0

## Technical Requirements

### Core Functionality Changes

**Schema Migration**
- Remove `game_id` column from `steam_games` table (redundant with `igdb_id`)
- Update all code references from `steam_games.game_id` to `steam_games.igdb_id`
- Ensure referential integrity during migration process

**Generic Sync Status Determination Logic**
- Implement reusable `is_game_synced(user_id, igdb_id, platform_id, storefront_id)` function
- Function checks `user_game_platforms` table for platform/storefront associations
- Steam-specific usage: call generic function with Steam platform and storefront IDs
- Replace `game_id` presence checks with proper association-based queries
- Design for reusability across future import sources (Epic, GOG, PlayStation, etc.)

**Database Query Optimization**
- Update queries to check `user_game_platforms` table instead of `game_id` presence
- Implement efficient JOINs between `user_games`, `user_game_platforms`, `platforms`, and `storefronts`
- Leverage existing indexes on foreign key columns for optimal performance
- Optimize query performance for large game collections with proper JOIN strategies

**Error Handling and Logging**
- Implement comprehensive error handling for database connectivity issues
- Add detailed logging for sync decision points and storefront association checks
- Include debug-level logging for troubleshooting sync behavior
- Handle edge cases where storefront associations may be incomplete or corrupted

### Integration Requirements

**Existing System Compatibility**
- Maintain backward compatibility with current game import workflow
- Ensure integration with existing IGDB API calls and rate limiting
- Preserve current CSV import functionality and batch processing capabilities
- Maintain compatibility with existing game collection management features

**Database Schema Integration**
- Migrate `steam_games` table to remove redundant `game_id` column
- Leverage existing `user_game_platforms` table structure for sync determination
- Ensure proper foreign key relationships are maintained during migration
- Work with existing platform/storefront seed data (Steam platform: pc-windows, Steam storefront: steam)
- Integrate migration with current SQLModel/Alembic migration system

**API Endpoint Compatibility**
- Maintain existing REST API endpoints for game management
- Ensure import/sync endpoints continue to function with refactored logic
- Preserve current authentication and authorization mechanisms
- Maintain consistent response formats and error codes

### Performance Criteria

**Query Performance**
- Storefront association checks must complete in <50ms for single game queries
- Batch sync operations should process ≥100 games per second
- Database connection pooling must support concurrent sync operations
- Memory usage should remain within current application limits

**Sync Efficiency**
- Reduce unnecessary IGDB API calls by accurate sync status determination
- Maintain current rate limiting compliance (4 requests/second max)
- Ensure sync operations complete within existing timeout constraints
- Minimize database transaction overhead during bulk operations

**System Resource Usage**
- CPU usage should not exceed current baseline during sync operations
- Database connection count should remain within configured limits
- File I/O for cover art downloads should maintain current performance levels
- Memory allocation patterns should not introduce leaks or excessive usage

### Implementation Design

**Generic Sync Function Architecture**

```python
def is_game_synced(user_id: str, igdb_id: int, platform_id: str, storefront_id: str) -> bool:
    """
    Generic function to check if a game is synced for a specific platform/storefront combination.
    
    Args:
        user_id: User's unique identifier
        igdb_id: Game's IGDB ID (primary key in games table)
        platform_id: Platform identifier (e.g., 'pc-windows', 'playstation-5')
        storefront_id: Storefront identifier (e.g., 'steam', 'epic', 'gog')
    
    Returns:
        True if user_game_platforms association exists, False otherwise
    """
    # Query user_game_platforms for the specific combination
    # JOIN user_games ON user_game_id
    # WHERE user_id = ? AND game_id = ? AND platform_id = ? AND storefront_id = ?
    pass

# Steam-specific usage
def is_steam_game_synced(user_id: str, igdb_id: int) -> bool:
    """Check if a Steam game is synced using the generic function."""
    steam_platform_id = get_platform_id("pc-windows")  # or lookup Steam platform
    steam_storefront_id = get_storefront_id("steam")    # or lookup Steam storefront
    return is_game_synced(user_id, igdb_id, steam_platform_id, steam_storefront_id)

# Usage in Steam import logic
steam_game = get_steam_game(user_id, steam_appid)
if is_steam_game_synced(user_id, steam_game.igdb_id):
    # Skip import - already synced
    pass
else:
    # Proceed with import/sync
    pass
```

**Migration Strategy**
- Create Alembic migration to drop `steam_games.game_id` column
- Update all code references to use `steam_games.igdb_id` instead
- Update foreign key relationships and constraints as needed
- Test migration in development environment before deployment

**Framework Design Benefits**
- Reusable across all future import sources (Epic, GOG, PlayStation, Xbox, Nintendo)
- Consistent sync logic and behavior across platforms
- Easier testing with parameterized test cases
- Centralized sync determination logic for maintenance
- Supports multi-platform game ownership scenarios

### Testing Requirements

**Unit Test Coverage**
- Test generic `is_game_synced()` function with various platform/storefront combinations
- Test Steam-specific sync logic using generic function with Steam parameters
- Mock database queries for `user_game_platforms` association checks
- Test error handling for database connectivity failures
- Verify correct behavior when Steam platform/storefront associations exist vs. don't exist
- Test edge cases with malformed or incomplete association data
- Test `game_id` to `igdb_id` migration logic

**Integration Test Scenarios**
- End-to-end sync workflow testing with real `user_game_platforms` database interactions
- Test Steam import with mixed game collections (some with/without platform/storefront associations)
- Test generic sync function with multiple platform/storefront combinations
- Verify IGDB API integration continues to function correctly with `igdb_id` usage
- Test concurrent sync operations and database transaction handling
- Validate cover art download and storage functionality remains intact
- Test migration from `game_id` to `igdb_id` in steam_games table

**Performance Testing**
- Benchmark sync performance with large game collections (1000+ games)
- Load test concurrent sync operations
- Measure database query performance under various load conditions
- Validate memory usage patterns during extended sync operations
- Test system behavior under IGDB API rate limiting conditions

**Regression Testing**
- Ensure existing game management functionality remains unaffected
- Verify backward compatibility with previously imported games
- Test all existing CSV import scenarios continue to work
- Validate API endpoints maintain expected behavior and response formats
- Confirm authentication and authorization mechanisms remain functional