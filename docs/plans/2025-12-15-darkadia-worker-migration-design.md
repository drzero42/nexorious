# Darkadia Worker Migration Design

**Date:** 2025-12-15
**Status:** Approved
**Issue:** nexorious-2dc, nexorious-otu

## Overview

Migrate Darkadia CSV import from the legacy staging table system to the worker-based Job/ReviewItem infrastructure, consistent with how Steam sync works.

## Key Decisions

| Decision | Choice |
|----------|--------|
| Data model | Review queue (Job + ReviewItem), not staging tables |
| Platform resolution | Defer to finalization, user maps once per unique string |
| CSV storage | Parse in API, store rows in job.result_summary_json |
| Review UI | Unified with source-specific sections |
| Legacy removal | Full removal in same PR |
| Frontend route | Simple upload page, redirect to unified review |
| Finalization | Auto-resolve known platforms, user resolves rest grouped |

## Architecture

```
┌─────────────────────┐
│  /import/darkadia   │  Simple upload page
│  (Upload CSV)       │
└─────────┬───────────┘
          │ POST /api/import/darkadia
          ▼
┌─────────────────────┐
│  API validates CSV  │  Fast validation in API container
│  Creates Job        │  Store parsed rows in job.result_summary_json
│  Enqueues to worker │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│  Worker processes   │  Runs in worker container
│  Creates ReviewItems│  IGDB matching per game
└─────────┬───────────┘
          │ Redirect user
          ▼
┌─────────────────────┐
│  /review?job_id=X   │  Unified review page
│  - Platform mapping │  Section 1: Map platform/storefront strings
│  - Game matches     │  Section 2: Confirm IGDB matches
└─────────┬───────────┘
          │ User clicks "Finalize"
          ▼
┌─────────────────────┐
│  POST /api/review/  │  Creates UserGame + UserGamePlatform
│  finalize           │  for all confirmed items
└─────────────────────┘
```

## Data Model

### Job Record

```python
Job(
    user_id=current_user.id,
    job_type=BackgroundJobType.IMPORT,
    source=BackgroundJobSource.DARKADIA,
    status=BackgroundJobStatus.PENDING,
    priority=BackgroundJobPriority.HIGH,
    progress_total=len(rows),
)
job.set_result_summary({
    "file_name": "darkadia_export.csv",
    "total_games": 1473,
    "columns": ["Name", "Platform", "Status", ...],
    "_import_rows": [  # Cleared by worker after processing
        {"Name": "Zelda BOTW", "Platform": "Switch", ...},
        {"Name": "Elden Ring", "Platform": "PC|Steam|Digital", ...},
    ]
})
```

### ReviewItem Records

Worker creates one per game:

```python
ReviewItem(
    job_id=job.id,
    user_id=job.user_id,
    source_title="Zelda BOTW",
    status=ReviewItemStatus.MATCHED,  # or PENDING if low confidence
    resolved_igdb_id=7346,  # if auto-matched
)
item.set_source_metadata({
    "source": "darkadia",
    "platforms": ["Switch"],
    "storefronts": [],
    "play_status": "Completed",
    "rating": "9",
    "notes": "Amazing game",
    "hours": "120",
})
item.set_igdb_candidates([...])
```

### Platform/Storefront Mappings

Computed on-the-fly from ReviewItems at review time. Not persisted - passed to finalization API.

## Worker Processing

### Platform String Parsing

Darkadia CSV has platform field like `"PC|Steam|Digital"`:

```python
def parse_darkadia_platform(platform_str: str) -> dict:
    parts = platform_str.split("|")
    return {
        "platform": parts[0] if len(parts) > 0 else None,
        "storefront": parts[1] if len(parts) > 1 else None,
        "media_type": parts[2] if len(parts) > 2 else None,
    }
```

### Multi-Platform Games

Source metadata stores arrays:

```python
source_metadata = {
    "source": "darkadia",
    "platforms": ["PC", "PlayStation 4"],
    "storefronts": ["Steam", "PSN"],
}
```

### Processing Flow

1. Fetch job, update status to PROCESSING
2. Parse `_import_rows` from job.result_summary
3. For each row:
   - Parse platform/storefront strings
   - Run IGDB matching via MatchingService
   - Create ReviewItem with status MATCHED or PENDING
   - Store platforms/storefronts in source_metadata
4. Clear `_import_rows` from job to save space
5. Set job status to AWAITING_REVIEW

## Review UI

### Two Sections

**Section 1: Platform/Storefront Mappings**

- Shows unique platform strings extracted from all ReviewItems
- Shows count of games affected by each
- Auto-suggests matches via fuzzy matching against Platform/Storefront tables
- User selects correct mapping from dropdown
- Each string mapped once, applies to all games

**Section 2: Game Matches**

- Existing ReviewItemCard list
- No changes needed
- Shows each game with IGDB candidates

### Finalize Button

Enabled when:
- All platforms/storefronts mapped (or explicitly left unmapped)
- At least one game has status MATCHED

## API Endpoints

### Existing (no changes)

- `GET /api/review/` - List ReviewItems
- `POST /api/review/{id}/match` - Confirm IGDB match
- `POST /api/review/{id}/skip` - Skip game
- `GET /api/review/summary` - Get counts

### Modified

- `POST /api/import/darkadia` - Minor tweaks to platform parsing

### New

**GET /api/review/platform-summary?job_id=X**

```json
{
    "platforms": [
        {"original": "PS4", "count": 23, "suggested_id": "6ac2f1c7-...", "suggested_name": "PlayStation 4"},
        {"original": "PC", "count": 142, "suggested_id": "07a28e0a-...", "suggested_name": "PC (Windows)"},
        {"original": "Switch", "count": 15, "suggested_id": null, "suggested_name": null}
    ],
    "storefronts": [
        {"original": "Steam", "count": 89, "suggested_id": "steam", "suggested_name": "Steam"},
        {"original": "PSN", "count": 23, "suggested_id": "bb948c01-...", "suggested_name": "PlayStation Store"}
    ],
    "all_resolved": false
}
```

**POST /api/review/finalize**

```json
// Request
{
    "job_id": "abc-123",
    "platform_mappings": {
        "PS4": "6ac2f1c7-...",
        "PC": "07a28e0a-...",
        "Switch": "a1b2c3d4-..."
    },
    "storefront_mappings": {
        "PSN": "bb948c01-...",
        "Steam": "steam"
    }
}

// Response
{
    "success": true,
    "games_imported": 142,
    "games_skipped": 12,
    "errors": []
}
```

### Finalization Logic

```python
for item in job.review_items:
    if item.status != ReviewItemStatus.MATCHED:
        continue

    if not item.resolved_igdb_id:
        continue

    # 1. Ensure Game exists
    game = await game_service.create_or_update_game_from_igdb(item.resolved_igdb_id)

    # 2. Create UserGame
    user_game = UserGame(user_id=item.user_id, game_id=game.id)

    # 3. Create UserGamePlatform for each platform/storefront
    metadata = item.get_source_metadata()
    platforms = metadata.get("platforms", [])
    storefronts = metadata.get("storefronts", [])

    for i, platform_str in enumerate(platforms):
        platform_id = platform_mappings.get(platform_str)
        storefront_str = storefronts[i] if i < len(storefronts) else None
        storefront_id = storefront_mappings.get(storefront_str) if storefront_str else None

        UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=platform_id,
            storefront_id=storefront_id,
        )

job.status = BackgroundJobStatus.COMPLETED
```

## Legacy Code Removal

### Backend Files to Delete

| Path | Lines |
|------|-------|
| `app/api/import_api/sources/darkadia.py` | ~1,760 |
| `app/api/import_api/sources/darkadia_batch.py` | ~44 |
| `app/services/import_sources/darkadia.py` | ~2,191 |
| `app/models/darkadia_game.py` | ~150 |
| `app/models/darkadia_import.py` | ~150 |

### Frontend Files to Delete

| Path | Lines |
|------|-------|
| `src/lib/stores/darkadia.svelte.ts` | ~1,241 |
| `src/lib/types/darkadia.ts` | ~200 |
| `src/lib/components/DarkadiaGamesTable.svelte` | TBD |
| `src/lib/components/DarkadiaGameCard.svelte` | TBD |
| `src/lib/components/DarkadiaManualMatchModal.svelte` | TBD |
| Related test files | TBD |

### Frontend Files to Rewrite

| Path | Change |
|------|--------|
| `src/routes/import/darkadia/+page.svelte` | Simple upload form only |

### Files to Modify

| Path | Change |
|------|--------|
| `app/api/import_api/sources/__init__.py` | Remove darkadia router |
| `app/models/__init__.py` | Remove model exports |
| `src/lib/stores/index.ts` | Remove darkadia export |
| `src/routes/review/+page.svelte` | Add platform mapping section |
| `src/lib/stores/review.svelte.ts` | Add finalize, platform helpers |

### Database Migration

```sql
DROP TABLE IF EXISTS darkadia_imports;
DROP TABLE IF EXISTS darkadia_games;
```

## New Frontend Components

| Path | Description |
|------|-------------|
| `src/routes/import/darkadia/+page.svelte` | Rewrite - simple upload |
| `src/lib/components/PlatformMappingSection.svelte` | New - mapping UI |
| `src/routes/review/+page.svelte` | Modify - add mapping section |
| `src/lib/stores/review.svelte.ts` | Modify - add finalization |

## Testing Strategy

1. **Worker task tests** - Verify CSV parsing, IGDB matching, ReviewItem creation
2. **API tests** - Test upload, platform-summary, finalize endpoints
3. **Frontend tests** - Test upload flow, platform mapping UI, finalization
4. **Integration test** - Full flow from CSV upload to games in collection
