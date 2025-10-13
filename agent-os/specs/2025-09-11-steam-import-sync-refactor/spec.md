# Spec Requirements Document

> Spec: Steam Import Sync Logic Refactor
> Created: 2025-09-11
> Status: Planning

## Overview

Refactor the Steam import sync logic by removing the redundant `game_id` field from `steam_games` table and using proper Steam platform/storefront associations in `user_game_platforms` instead of relying on `game_id` presence for determining which games to import. This change will ensure accurate sync behavior and align with the established data model architecture.

## User Stories

**Story 1: Steam Library Sync**
As a user who has games in my Steam library, I want the import sync to correctly identify which games need to be updated based on my Steam platform/storefront associations in the user_game_platforms table, not just whether a game_id exists in steam_games, so that I get accurate library synchronization.

**Story 2: Multi-Storefront Game Management**
As a user who owns the same game on multiple platforms and storefronts including Steam, I want the sync logic to properly handle Steam-specific updates by checking user_game_platforms associations without affecting my other platform/storefront combinations, so that my game collection remains accurate across all platforms.

**Story 3: Clean Import Logic**
As a developer, I want the Steam import sync logic to use a generic reusable function that can be parameterized with platform and storefront IDs, so that the codebase is maintainable and supports the planned import framework for future storefronts.

## Spec Scope

1. **Schema Migration**: Remove redundant `game_id` column from `steam_games` table and update all references to use `igdb_id`
2. **Generic Sync Function**: Implement reusable `is_game_synced(user_id, igdb_id, platform_id, storefront_id)` function that checks `user_game_platforms` associations
3. **Steam-Specific Logic**: Update Steam import to use generic sync function with Steam platform and storefront parameters
4. **Code Refactoring**: Update all code that currently uses `steam_games.game_id` to use `steam_games.igdb_id` instead
5. **Comprehensive Testing**: Create unit and integration tests for the generic sync function and Steam-specific usage

## Out of Scope

- Changes to other storefront import logic (Epic, GOG, etc.) beyond using the generic sync function
- Modifications to the core Game model or user_game_platforms schema structure
- User interface changes or frontend modifications
- Performance optimizations beyond maintaining current sync speed
- Complex data migrations (only removing redundant column)

## Expected Deliverable

1. **Generic Reusable Sync Function**: Implemented `is_game_synced()` function that works with any platform/storefront combination and Steam-specific wrapper function
2. **Database Schema Migration**: Successfully removed redundant `game_id` column from steam_games table with all code updated to use `igdb_id`
3. **Complete Test Coverage**: Comprehensive test suite covering the generic sync function, Steam-specific usage, migration scenarios, and platform/storefront association-based decision making

## Spec Documentation

- Tasks: @.agent-os/specs/2025-09-11-steam-import-sync-refactor/tasks.md
- Technical Specification: @.agent-os/specs/2025-09-11-steam-import-sync-refactor/sub-specs/technical-spec.md