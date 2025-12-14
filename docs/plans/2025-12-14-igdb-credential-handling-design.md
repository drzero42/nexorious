# IGDB Credential Handling Design

**Date:** 2025-12-14
**Status:** Ready for implementation

## Problem

IGDB API credentials (`IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET`) are required for core functionality (game search, import, metadata enrichment), but:

1. No documentation explains how to obtain credentials
2. Backend starts silently without credentials
3. Frontend shows no indication that IGDB is misconfigured
4. Users only discover the problem when they try to add a game and get an opaque error

## Design Decisions

| Decision | Choice |
|----------|--------|
| Target users | Both technical and non-technical self-hosters |
| App usability without IGDB | Unusable for adding games; viewing existing games works |
| Frontend notification style | Banner on affected pages only |
| Backend behavior | Log warning at startup, fail gracefully at runtime (HTTP 503) |
| User instructions | Link to documentation (not inline steps) |
| Backend-to-frontend communication | Dedicated `/api/status` endpoint |
| Documentation location | README section + detailed `docs/igdb-setup.md` |
| Frontend caching | Check once on app load, store globally |

## Implementation

### 1. Backend: Status Endpoint

**File:** `backend/app/api/status.py` (new)

Create a new router with a public endpoint:

```python
GET /api/status
Response: {"igdb_configured": boolean}
```

- No authentication required
- Returns `true` only if both `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` are set and non-empty

**File:** `backend/app/main.py`

- Register the new status router

### 2. Backend: Startup Warning

**File:** `backend/app/main.py`

In the `lifespan` function, after startup initialization:

```python
if not settings.igdb_client_id or not settings.igdb_client_secret:
    logger.warning(
        "IGDB credentials not configured. Game search and import features "
        "will be unavailable. See docs/igdb-setup.md for setup instructions."
    )
```

### 3. Backend: Improved IGDB Error Response

**File:** `backend/app/services/igdb/auth.py`

Update `TwitchAuthError` handling to return HTTP 503 with helpful message:

```python
{
    "error": "IGDB not configured",
    "detail": "IGDB API credentials are required for this feature. See documentation for setup instructions."
}
```

This may require updating exception handlers or the service layer to catch and re-raise appropriately.

### 4. Frontend: Status Store

**File:** `frontend/src/lib/stores/app-status.svelte.ts` (new)

```typescript
// Svelte 5 runes-based store
// Fetches /api/status once on initialization
// Exposes: igdbConfigured (boolean), loading (boolean), error (string | null)
```

### 5. Frontend: Warning Banner Component

**File:** `frontend/src/lib/components/IgdbWarningBanner.svelte` (new)

- Yellow/amber warning styling (consistent with existing design system)
- Message: "IGDB API credentials are not configured. Game search and import features are unavailable."
- Link: "Setup Guide" pointing to documentation
- Only renders when `igdbConfigured === false`

### 6. Frontend: Add Banner to Affected Pages

Add the `IgdbWarningBanner` component to:

- `frontend/src/routes/games/add/+page.svelte`
- `frontend/src/routes/import/steam/+page.svelte`
- `frontend/src/routes/import/darkadia/+page.svelte`

Banner appears at top of content area, below navigation.

### 7. Documentation: README Update

**File:** `README.md`

Add a "Prerequisites" or "Required Configuration" section near the top:

```markdown
## Prerequisites

### IGDB API Credentials (Required)

Game search and import features require IGDB API access. See the [IGDB Setup Guide](docs/igdb-setup.md) for instructions on obtaining credentials.
```

### 8. Documentation: IGDB Setup Guide

**File:** `docs/igdb-setup.md` (new)

Complete setup guide including:

1. **Overview** - What IGDB is and why it's required
2. **Step-by-step instructions** for obtaining Twitch/IGDB credentials:
   - Navigate to Twitch Developer Console
   - Register an application
   - Generate Client ID and Secret
   - Add to `.env` file
   - Restart backend
3. **Verification** - How to confirm it's working
4. **Troubleshooting** - Common issues and solutions

## Files to Create/Modify

| File | Action |
|------|--------|
| `backend/app/api/status.py` | Create |
| `backend/app/main.py` | Modify (add router, startup warning) |
| `backend/app/services/igdb/auth.py` | Modify (improve error messages) |
| `frontend/src/lib/stores/app-status.svelte.ts` | Create |
| `frontend/src/lib/components/IgdbWarningBanner.svelte` | Create |
| `frontend/src/routes/games/add/+page.svelte` | Modify |
| `frontend/src/routes/import/steam/+page.svelte` | Modify |
| `frontend/src/routes/import/darkadia/+page.svelte` | Modify |
| `README.md` | Modify |
| `docs/igdb-setup.md` | Create |

## Testing

- Backend: Test `/api/status` returns correct value with and without credentials
- Backend: Test startup logs warning when credentials missing
- Backend: Test IGDB endpoints return 503 with helpful message when unconfigured
- Frontend: Test banner appears on affected pages when `igdbConfigured === false`
- Frontend: Test banner does not appear when IGDB is configured
- Frontend: Test status store fetches once and caches result
