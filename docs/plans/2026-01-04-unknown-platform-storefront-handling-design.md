# Unknown Platform/Storefront Handling Design

## Overview

When importing Darkadia CSV data, missing or unmapped platforms and storefronts should be stored as "unknown" associations in the database. Users can then filter for these unknowns and manually resolve them through the existing game edit UI.

## Problem Statement

Currently:
- The Darkadia CSV converter fails fast if it encounters unmapped platforms/storefronts
- Users cannot easily find games with unresolved platform/storefront associations
- The `original_storefront_name` field is missing (only `original_platform_name` exists)

## Solution

### Database Changes

Add `original_storefront_name` field to `UserGamePlatform` model:

```python
class UserGamePlatform(SQLModel, table=True):
    # ... existing fields ...
    platform: Optional[str]  # NULL = unknown
    storefront: Optional[str]  # NULL = unknown
    original_platform_name: Optional[str]  # Already exists
    original_storefront_name: Optional[str]  # NEW FIELD
```

**Behavior:**
- When platform/storefront resolves successfully: `original_*_name` stays NULL
- When platform/storefront is unmapped: `original_*_name` stores the source value
- When platform/storefront is missing from CSV: both the FK and `original_*_name` are NULL

### CSV Converter Changes

**File:** `backend/scripts/darkadia_to_nexorious.py`

**New behavior:**
1. Passthrough unmapped values - If a platform or storefront isn't in the mapping, pass through the original value as-is to the JSON output
2. Handle empty/missing values - If CSV has no platform/storefront data, output `null` in the JSON
3. Remove validation gate - Delete the code that fails on unmapped values
4. Print warnings - Log unmapped values so user is aware, but continue processing

**JSON output format:**
```json
{
  "platforms": [
    {
      "platform_name": "playstation-5",
      "storefront_name": "playstation-store"
    },
    {
      "platform_name": "PlayStation 6",
      "storefront_name": null
    }
  ]
}
```

### Backend Import Helper Changes

**File:** `backend/app/worker/tasks/import_export/import_nexorious_helpers.py`

Store `original_storefront_name` when storefront doesn't resolve:

```python
user_game_platform = UserGamePlatform(
    user_game_id=user_game.id,
    platform=platform_slug,
    storefront=storefront_slug,
    original_platform_name=platform_name if not platform_slug else None,
    original_storefront_name=storefront_name if not storefront_slug else None,
    # ... other fields
)
```

Handle empty strings as equivalent to `None` (both mean unknown).

### Backend API Changes

**File:** `backend/app/api/games.py` (or user_games endpoint)

Accept special value `"unknown"` for platform and storefront filter parameters:

- `platform=playstation-5` → `WHERE user_game_platforms.platform = 'playstation-5'`
- `platform=unknown` → `WHERE user_game_platforms.platform IS NULL`
- Same pattern for storefronts

### Frontend Changes

**File:** `frontend/src/components/games/game-filters.tsx`

Add "Unknown" option to both platform and storefront filter dropdowns:
- Append `{ value: "unknown", label: "Unknown" }` to the options list
- Sorts alphabetically with other options (appears near end under "U")

No changes to game display - already shows "-" for null platforms.

## Implementation Order

1. Database migration - Add `original_storefront_name` column
2. Backend import helper - Store `original_storefront_name` when storefront doesn't resolve
3. Backend API - Handle `platform=unknown` and `storefront=unknown` filter values
4. Frontend filters - Add "Unknown" option to platform and storefront dropdowns
5. CSV converter - Remove fail-fast validation, passthrough unmapped values

## Files to Modify

| Layer | File |
|-------|------|
| Database | `backend/app/models/user_game.py` |
| Migration | `backend/app/alembic/versions/xxx_add_original_storefront_name.py` (generated) |
| Import | `backend/app/worker/tasks/import_export/import_nexorious_helpers.py` |
| API | `backend/app/api/games.py` (or user_games endpoint) |
| Frontend | `frontend/src/components/games/game-filters.tsx` |
| Script | `backend/scripts/darkadia_to_nexorious.py` |

## Testing

- Backend: Unit tests for import helper with unknown platforms/storefronts
- Backend: API tests for `?platform=unknown` and `?storefront=unknown` filters
- Frontend: Component tests for filter dropdown with Unknown option
