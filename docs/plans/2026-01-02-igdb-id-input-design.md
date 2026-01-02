# IGDB ID Input Support for Search Fields

## Overview

Allow users to input IGDB IDs directly in search fields using the format `igdb:12345` (case-insensitive) instead of searching by game name.

## Problem

Users who already know the exact IGDB ID (from the IGDB website or another source) currently have to search by name and hope the correct game appears in results. Direct ID input provides a faster, more accurate path.

## Solution

Detect the `igdb:` prefix pattern in search inputs and route to a direct ID lookup instead of name search.

## Design

### Input Detection

Frontend detects the pattern using regex:

```
/^igdb:(\d+)$/i
```

- If matched: Extract numeric ID, call direct lookup endpoint
- If not matched: Proceed with existing search-by-name behavior

**Examples:**
- `igdb:12345` → lookup ID 12345
- `IGDB:999` → lookup ID 999
- `igdb:abc` → no match, search as query "igdb:abc"
- `The Witcher 3` → no match, search as normal

### Backend: New Endpoint

```
GET /games/igdb/{igdb_id}
```

**Response:** Same `IGDBSearchResponse` structure used by search:

```json
{
  "games": [{ "igdb_id": 12345, "title": "...", ... }],
  "total": 1
}
```

**Error handling:**
- Game not found → `{ "games": [], "total": 0 }` (matches search behavior)
- Invalid ID format → 400 Bad Request
- IGDB service unavailable → 503

### Frontend Changes

**`useSearchIGDB` hook:**
- Add detection for `igdb:` prefix pattern
- If detected: call `getGameByIGDBId()` API function
- If not detected: existing search behavior unchanged
- No minimum character requirement for ID lookup
- No debounce for ID lookup (immediate)

**New API function:**
```typescript
async function getGameByIGDBId(id: number): Promise<IGDBGameCandidate[]>
```

### Affected Locations

Both IGDB search locations work automatically via the shared hook:

1. Add Game page (`/games/add`)
2. Job Item Review modal (import conflict resolution)

## Implementation

| Layer | File | Change |
|-------|------|--------|
| Backend | `app/api/games.py` | Add `GET /games/igdb/{igdb_id}` endpoint |
| Frontend | `src/api/games.ts` | Add `getGameByIGDBId()` function |
| Frontend | `src/hooks/use-games.ts` | Update `useSearchIGDB` to detect prefix and route |

## Testing

- Backend: Unit test for new endpoint (found, not found, invalid ID)
- Frontend: Hook tests for prefix detection and routing logic
