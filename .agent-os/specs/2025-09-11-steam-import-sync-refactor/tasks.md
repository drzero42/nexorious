# Spec Tasks

## Tasks

- [x] 1. Database Schema Migration
  - [x] 1.1 Write tests for schema migration functionality
  - [x] 1.2 Create Alembic migration to remove `game_id` column from `steam_games` table
  - [x] 1.3 Update all code references from `steam_games.game_id` to `steam_games.igdb_id`
  - [x] 1.4 Verify migration rollback functionality works correctly
  - [x] 1.5 Validate referential integrity is maintained post-migration
  - [x] 1.6 Test migration in development environment
  - [x] 1.7 Verify all tests pass after migration changes

- [x] 2. Generic Sync Function Implementation
  - [x] 2.1 Write tests for generic `is_game_synced()` function
  - [x] 2.2 Implement core `is_game_synced(user_id, igdb_id, platform_id, storefront_id)` function
  - [x] 2.3 Create Steam-specific wrapper `is_steam_game_synced()` function
  - [x] 2.4 Add comprehensive error handling and logging
  - [x] 2.5 Optimize database queries with proper JOINs for performance
  - [x] 2.6 Verify performance meets <50ms criteria for single game queries
  - [x] 2.7 Verify all tests pass

- [x] 3. Steam Import Logic Refactoring
  - [x] 3.1 Write tests for updated Steam import sync behavior
  - [x] 3.2 Update Steam import service to use generic sync function
  - [x] 3.3 Replace `game_id` presence checks with platform/storefront association checks
  - [x] 3.4 Update Steam batch processing logic
  - [x] 3.5 Update related API endpoints and schemas
  - [x] 3.6 Ensure backward compatibility with existing workflows
  - [x] 3.7 Verify all tests pass

- [x] 4. Comprehensive Testing & Validation
  - [x] 4.1 Write unit tests for edge cases and error scenarios
  - [x] 4.2 Create integration tests for end-to-end sync workflows
  - [x] 4.3 Performance testing for large collections (1000+ games)
  - [x] 4.4 Regression testing for existing game management features
  - [x] 4.5 Test concurrent sync operations and database transaction handling
  - [x] 4.6 Validate CSV import functionality remains intact
  - [x] 4.7 Run complete test suite with >80% coverage requirement

- [x] 5. Code Cleanup & Documentation
  - [x] 5.1 Write tests for any additional helper functions
  - [x] 5.2 Update related service layer code and utilities
  - [x] 5.3 Clean up any unused code related to old `game_id` logic
  - [x] 5.4 Update inline documentation and docstrings
  - [x] 5.5 Verify code style compliance with ruff
  - [x] 5.6 Final integration testing and performance validation
  - [x] 5.7 Ensure all tests pass and coverage requirements met