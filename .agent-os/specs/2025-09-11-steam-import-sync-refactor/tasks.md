# Spec Tasks

## Tasks

- [ ] 1. Database Schema Migration
  - [ ] 1.1 Write tests for schema migration functionality
  - [ ] 1.2 Create Alembic migration to remove `game_id` column from `steam_games` table
  - [ ] 1.3 Update all code references from `steam_games.game_id` to `steam_games.igdb_id`
  - [ ] 1.4 Verify migration rollback functionality works correctly
  - [ ] 1.5 Validate referential integrity is maintained post-migration
  - [ ] 1.6 Test migration in development environment
  - [ ] 1.7 Verify all tests pass after migration changes

- [ ] 2. Generic Sync Function Implementation
  - [ ] 2.1 Write tests for generic `is_game_synced()` function
  - [ ] 2.2 Implement core `is_game_synced(user_id, igdb_id, platform_id, storefront_id)` function
  - [ ] 2.3 Create Steam-specific wrapper `is_steam_game_synced()` function
  - [ ] 2.4 Add comprehensive error handling and logging
  - [ ] 2.5 Optimize database queries with proper JOINs for performance
  - [ ] 2.6 Verify performance meets <50ms criteria for single game queries
  - [ ] 2.7 Verify all tests pass

- [ ] 3. Steam Import Logic Refactoring
  - [ ] 3.1 Write tests for updated Steam import sync behavior
  - [ ] 3.2 Update Steam import service to use generic sync function
  - [ ] 3.3 Replace `game_id` presence checks with platform/storefront association checks
  - [ ] 3.4 Update Steam batch processing logic
  - [ ] 3.5 Update related API endpoints and schemas
  - [ ] 3.6 Ensure backward compatibility with existing workflows
  - [ ] 3.7 Verify all tests pass

- [ ] 4. Comprehensive Testing & Validation
  - [ ] 4.1 Write unit tests for edge cases and error scenarios
  - [ ] 4.2 Create integration tests for end-to-end sync workflows
  - [ ] 4.3 Performance testing for large collections (1000+ games)
  - [ ] 4.4 Regression testing for existing game management features
  - [ ] 4.5 Test concurrent sync operations and database transaction handling
  - [ ] 4.6 Validate CSV import functionality remains intact
  - [ ] 4.7 Run complete test suite with >80% coverage requirement

- [ ] 5. Code Cleanup & Documentation
  - [ ] 5.1 Write tests for any additional helper functions
  - [ ] 5.2 Update related service layer code and utilities
  - [ ] 5.3 Clean up any unused code related to old `game_id` logic
  - [ ] 5.4 Update inline documentation and docstrings
  - [ ] 5.5 Verify code style compliance with ruff
  - [ ] 5.6 Final integration testing and performance validation
  - [ ] 5.7 Ensure all tests pass and coverage requirements met