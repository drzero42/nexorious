# Remove Sync Enabled Toggle

## Overview

Remove the `enabled` toggle from sync configuration. The sync frequency setting of `MANUAL` (default) effectively serves as "disabled" - no automatic syncing occurs, but users can still trigger manual syncs.

## Rationale

The current implementation has redundant states:
- `enabled: false` → no automatic sync, manual sync blocked
- `enabled: true` + `frequency: MANUAL` → no automatic sync, manual sync allowed
- `enabled: true` + `frequency: DAILY` → automatic daily sync, manual sync allowed

Simplified states after this change:
- `frequency: MANUAL` → no automatic sync, manual sync allowed
- `frequency: DAILY` → automatic daily sync, manual sync allowed

## Changes Required

### Backend

1. **`app/models/user_sync_config.py`**
   - Remove `enabled` field from `UserSyncConfig` model
   - Update `needs_sync` property to only check `frequency != MANUAL`

2. **`app/api/sync.py`**
   - Remove `enabled` from API schemas and responses
   - Remove any `enabled` checks in endpoint logic

3. **`app/alembic/versions/72d8dfbc57f2_initial_schema.py`**
   - Remove `enabled` column from `user_sync_configs` table creation

### Frontend

1. **`src/types/sync.ts`**
   - Remove `enabled` from `SyncConfig` interface

2. **`src/components/sync/sync-service-card.tsx`**
   - Remove the "Enable sync" toggle switch and related state/handlers
   - Change disabled conditions from `!localEnabled || !config.isConfigured` to just `!config.isConfigured`
   - "Sync Now" button enabled immediately when `config.isConfigured` is true

### UI Behavior After Changes

- **Not configured:** Card shows disabled state, frequency/auto-add controls disabled, "Sync Now" disabled
- **Configured:** Frequency/auto-add controls enabled, "Sync Now" button enabled immediately

### Database

- Reset database after changes (`podman-compose down -v` then `uv run alembic upgrade head`)
- No migration needed (dev only, no releases yet)
