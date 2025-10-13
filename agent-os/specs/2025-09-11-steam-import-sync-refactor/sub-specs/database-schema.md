# Database Schema

This is the database schema implementation for the spec detailed in @.agent-os/specs/2025-09-11-steam-import-sync-refactor/spec.md

> Created: 2025-09-11
> Version: 1.0.0

## Schema Changes

### Core Schema Requirements

The Steam import sync refactor requires one schema change: removing the `game_id` field from the `steam_games` table, since it now contains the same value as `igdb_id`. The key change is shifting from Game ID presence detection to proper Steam storefront association queries via the `user_game_platforms` table.

### Existing Schema Elements Used

#### steam_games Table Changes

The `steam_games` table requires one schema change to remove redundant data:

```sql
-- Remove game_id column (redundant with igdb_id)
ALTER TABLE steam_games DROP COLUMN game_id;
```

The existing unique constraint remains and supports the new sync logic:

```sql
-- Existing unique constraint ensures one entry per user per Steam app
UNIQUE(user_id, steam_appid)
```

This constraint ensures:
- Each user can only have one Steam game entry per Steam app ID
- Duplicate Steam apps cannot exist for the same user
- The sync process can reliably query using `igdb_id` instead of `game_id`

#### Platform/Storefront Association Schema

The existing association tables provide the foundation for proper sync status determination:

```sql
-- games table - core game entity (uses IGDB ID as primary key)
games (
    id, -- IGDB ID as primary key
    title,
    ...
)

-- platforms table - platform definitions
platforms (
    id,
    name, -- 'pc-windows', 'playstation-5', etc.
    ...
)

-- storefronts table - storefront definitions  
storefronts (
    id,
    name, -- 'Steam', 'Epic', 'GOG', etc.
    ...
)

-- user_games table - user's game ownership
user_games (
    id,
    user_id REFERENCES users(id),
    game_id REFERENCES games(id), -- References IGDB ID
    ...
)

-- user_game_platforms association table - tracks platform/storefront ownership
user_game_platforms (
    id,
    user_game_id REFERENCES user_games(id),
    platform_id REFERENCES platforms(id),
    storefront_id REFERENCES storefronts(id),
    store_game_id, -- Steam app ID, Epic catalog ID, etc.
    ...
)
```

### Performance Optimization Indexes

#### Existing Indexes Supporting New Logic

The current schema already includes indexes that support efficient platform/storefront-based queries:

```sql
-- Index on user_game_platforms for storefront-based lookups
CREATE INDEX idx_user_game_platforms_storefront_id ON user_game_platforms(storefront_id);

-- Index on user_game_platforms for platform-based lookups  
CREATE INDEX idx_user_game_platforms_platform_id ON user_game_platforms(platform_id);

-- Index on user_games for user-based queries
CREATE INDEX idx_user_games_user_id ON user_games(user_id);

-- Index on user_games for game-based queries
CREATE INDEX idx_user_games_game_id ON user_games(game_id);
```

These indexes enable efficient queries for:
- Finding all games for a user with Steam platform/storefront associations
- Determining if a specific Steam app already exists in a user's collection
- Cross-referencing games with their platform/storefront associations

#### Recommended Additional Indexes

While not required for functionality, these indexes could optimize the new sync logic:

```sql
-- Composite index for efficient user_game + platform + storefront queries
CREATE INDEX idx_user_game_platforms_composite 
ON user_game_platforms(user_game_id, platform_id, storefront_id);

-- Index on store_game_id for reverse lookups (Steam app ID -> game)
CREATE INDEX idx_user_game_platforms_store_game_id ON user_game_platforms(store_game_id);
```

### Database Constraints and Foreign Key Relationships

#### Existing Constraints Supporting Sync Logic

The current schema constraints already provide the data integrity needed for the refactored sync logic:

```sql
-- Foreign key ensuring user_games exist for platform associations
user_game_platforms.user_game_id -> user_games.id (CASCADE DELETE)

-- Foreign key ensuring valid platforms
user_game_platforms.platform_id -> platforms.id (RESTRICT)

-- Foreign key ensuring valid storefronts
user_game_platforms.storefront_id -> storefronts.id (RESTRICT)

-- User ownership validation
user_games.user_id -> users.id (CASCADE DELETE)

-- Game reference validation
user_games.game_id -> games.id (RESTRICT)
```

#### Data Integrity Benefits

These constraints support the new sync logic by:
- Preventing orphaned platform/storefront associations
- Ensuring all user games have valid user ownership and game references
- Maintaining referential integrity during sync operations
- Enabling reliable cascade deletes when user games are removed
- Preventing invalid platform or storefront references

### Rationale for Schema Support

#### Why Current Schema Supports New Logic

The existing schema design already embodies the correct architectural pattern that the refactor aims to implement:

1. **Separation of Concerns**: User games and platform/storefront associations are properly separated, allowing for independent sync logic per platform/storefront

2. **Normalized Relationships**: The `user_game_platforms` table provides a clean many-to-many relationship that naturally supports multi-platform, multi-storefront ownership

3. **Unique Constraints**: Existing constraints prevent duplicate associations, eliminating edge cases in sync logic

4. **Indexed Performance**: Current indexes support efficient queries for platform/storefront-based operations

#### How Schema Enables Refactor Goals

The schema design supports the shift from `game_id` presence to Steam platform/storefront associations by:

**Before (game_id Presence in steam_games)**:
```sql
-- Old logic: Check if steam_games.game_id is not null
SELECT game_id FROM steam_games WHERE user_id = ? AND steam_appid = ? AND game_id IS NOT NULL
```

**After (Platform/Storefront Association)**:
```sql
-- New logic: Check if Steam platform/storefront association exists
SELECT ug.id FROM user_games ug
JOIN user_game_platforms ugp ON ug.id = ugp.user_game_id
JOIN platforms p ON ugp.platform_id = p.id
JOIN storefronts s ON ugp.storefront_id = s.id
WHERE ug.user_id = ? AND ug.game_id = ? -- game_id is now IGDB ID
  AND p.name = 'pc-windows' AND s.name = 'steam'
```

This approach:
- Provides accurate sync status for Steam-specific games
- Supports multi-storefront ownership without conflicts
- Follows established data model patterns
- Enables precise update targeting

## Migrations

### Required Migration

This refactor requires **one database migration** to remove the redundant `game_id` column from `steam_games`:

```sql
-- Remove game_id column from steam_games table
ALTER TABLE steam_games DROP COLUMN game_id;
```

The migration is safe because:
- `game_id` and `igdb_id` currently contain the same values
- All existing logic can be updated to use `igdb_id` instead
- No data loss occurs during the migration

### Validation Queries

To validate the schema supports the new logic, these queries can be used during testing:

```sql
-- Verify Steam platform and storefront exist
SELECT id FROM platforms WHERE name = 'pc-windows';
SELECT id FROM storefronts WHERE name = 'steam';

-- Count user games with Steam platform/storefront associations
SELECT COUNT(*) FROM user_games ug
JOIN user_game_platforms ugp ON ug.id = ugp.user_game_id
JOIN platforms p ON ugp.platform_id = p.id
JOIN storefronts s ON ugp.storefront_id = s.id  
WHERE ug.user_id = ? AND p.name = 'pc-windows' AND s.name = 'steam';

-- Find Steam games that are synced (have platform/storefront associations)
SELECT sg.steam_appid, sg.igdb_id FROM steam_games sg
JOIN user_games ug ON sg.igdb_id = ug.game_id AND sg.user_id = ug.user_id
JOIN user_game_platforms ugp ON ug.id = ugp.user_game_id
JOIN platforms p ON ugp.platform_id = p.id
JOIN storefronts s ON ugp.storefront_id = s.id
WHERE sg.user_id = ? AND p.name = 'pc-windows' AND s.name = 'steam';
```

### Data Consistency Verification

The existing schema constraints ensure data consistency during the transition:
- No orphaned user_game records can exist
- No invalid platform/storefront associations can be created  
- User ownership is always maintained through user_games table
- Duplicate Steam apps per user are prevented by steam_games unique constraint
- Game references remain valid through foreign key to games table

This makes the refactor safe to deploy with minimal migration concerns (only dropping the redundant column).