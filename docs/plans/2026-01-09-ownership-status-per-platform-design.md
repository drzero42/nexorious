# Ownership Status Per Platform Design

**Date:** 2026-01-09
**Status:** Approved

## Problem

Currently, `ownership_status` and `acquired_date` are stored at the `UserGame` level. This means a user can only have ONE ownership status per game, regardless of how many platforms/storefronts they own it on.

**Example of the problem:**
- User owns Elden Ring on Steam (should be "owned")
- User has access to Elden Ring via PS Plus on PSN (should be "subscription")
- Current model forces picking ONE status for the entire game

## Solution

Move `ownership_status` and `acquired_date` from `UserGame` to `UserGamePlatform`, allowing each platform/storefront entry to have its own ownership status.

## Data Model Changes

### UserGamePlatform (gains fields)

```python
class UserGamePlatform(SQLModel, table=True):
    # ... existing fields ...
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED)
    acquired_date: Optional[date] = Field(default=None)
```

### UserGame (loses fields)

Remove:
- `ownership_status`
- `acquired_date`

## Migration Strategy

The Alembic migration will:

1. Add `ownership_status` column to `user_game_platforms` with default `'owned'`
2. Add `acquired_date` column to `user_game_platforms` (nullable)
3. Copy existing values from `user_games` to all related `user_game_platforms` records
4. Drop `ownership_status` column from `user_games`
5. Drop `acquired_date` column from `user_games`

This preserves existing data by propagating current values to all platform entries.

## Schema/API Changes

### UserGamePlatformCreate (input)

Add:
- `ownership_status: OwnershipStatus = OwnershipStatus.OWNED`
- `acquired_date: Optional[date] = None`

### UserGamePlatformRead (output)

Add:
- `ownership_status: OwnershipStatus`
- `acquired_date: Optional[date]`

### UserGameCreate (input)

Remove:
- `ownership_status`
- `acquired_date`

### UserGameRead (output)

Remove:
- `ownership_status`
- `acquired_date`

## Frontend Impact

### Game detail/edit views
- Ownership status selector moves from main game form to each platform entry
- Acquired date picker moves to each platform entry
- Each platform row displays its own ownership status badge

### Game list/grid views
- If displaying ownership status, show primary platform's status or multiple badges
- Alternative: omit aggregate status from list view

### Filtering
- "Filter by ownership status" queries across platform entries
- A game appears in "Subscription" filter if ANY of its platforms have subscription status

### Forms
- When adding a platform, user selects ownership status and optionally acquired date
- Default remains "owned" for convenience

## Implementation Order

### Phase 1: Backend model & migration
1. Update `UserGamePlatform` model with new fields
2. Generate Alembic migration (data-preserving)
3. Update `UserGame` model to remove fields

### Phase 2: Backend schemas & API
1. Update `UserGamePlatformCreate` and `UserGamePlatformRead` schemas
2. Update `UserGameCreate` and `UserGameRead` schemas
3. Update service logic referencing these fields
4. Update backend tests

### Phase 3: Frontend
1. Update TypeScript types
2. Move ownership status UI to platform entries
3. Update filtering logic
4. Update forms and tests

## Testing Considerations

- Migration must be tested with existing data
- API tests need updating for new field locations
- Frontend tests for platform-level ownership editing
