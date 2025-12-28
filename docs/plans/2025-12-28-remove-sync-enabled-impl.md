# Remove Sync Enabled Toggle - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the redundant `enabled` toggle from sync configuration, using `frequency: MANUAL` as the "disabled" state.

**Architecture:** Remove the `enabled` field from backend model, schema, API responses, and migration. Update frontend to remove the toggle UI and change disabled conditions from `!enabled` to `!isConfigured`.

**Tech Stack:** Python/FastAPI/SQLModel (backend), TypeScript/React/Next.js (frontend)

---

## Task 1: Backend Model - Remove `enabled` Field

**Files:**
- Modify: `backend/app/models/user_sync_config.py:53` (remove enabled field)
- Modify: `backend/app/models/user_sync_config.py:69-80` (update needs_sync property)

**Step 1: Remove the `enabled` field from UserSyncConfig model**

In `backend/app/models/user_sync_config.py`, remove line 53:

```python
# DELETE THIS LINE:
enabled: bool = Field(default=False, description="Whether sync is enabled for this platform")
```

**Step 2: Update the `needs_sync` property docstring and logic**

Replace lines 69-80 with:

```python
    @property
    def needs_sync(self) -> bool:
        """
        Check if this config needs syncing based on frequency and last sync time.

        Returns True if:
        - frequency is not MANUAL AND
        - (last_synced_at is None OR enough time has passed based on frequency)
        """
        if self.frequency == SyncFrequency.MANUAL:
            return False
```

**Step 3: Run type checker to verify model changes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pyrefly check app/models/user_sync_config.py`
Expected: No errors (or unrelated warnings only)

**Step 4: Commit**

```bash
git add backend/app/models/user_sync_config.py
git commit -m "refactor(backend): remove enabled field from UserSyncConfig model"
```

---

## Task 2: Backend Schema - Remove `enabled` from API Schemas

**Files:**
- Modify: `backend/app/schemas/sync.py:43` (remove from SyncConfigResponse)
- Modify: `backend/app/schemas/sync.py:72-74` (remove from SyncConfigUpdateRequest)
- Modify: `backend/app/schemas/sync.py:88` (remove from SyncConfigCreateRequest)

**Step 1: Remove `enabled` from SyncConfigResponse**

In `backend/app/schemas/sync.py`, remove line 43:

```python
# DELETE THIS LINE:
enabled: bool = Field(description="Whether sync is enabled for this platform")
```

**Step 2: Remove `enabled` from SyncConfigUpdateRequest**

Remove lines 72-74:

```python
# DELETE THESE LINES:
enabled: Optional[bool] = Field(
    default=None, description="Whether sync is enabled for this platform"
)
```

**Step 3: Remove `enabled` from SyncConfigCreateRequest**

Remove line 88:

```python
# DELETE THIS LINE:
enabled: bool = Field(default=False, description="Whether sync is enabled")
```

**Step 4: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pyrefly check app/schemas/sync.py`
Expected: No errors

**Step 5: Commit**

```bash
git add backend/app/schemas/sync.py
git commit -m "refactor(backend): remove enabled field from sync API schemas"
```

---

## Task 3: Backend API - Remove `enabled` from Endpoints

**Files:**
- Modify: `backend/app/api/sync.py:80` (remove from _config_to_response)
- Modify: `backend/app/api/sync.py:121` (remove from default config creation)
- Modify: `backend/app/api/sync.py:160` (remove from default config creation)
- Modify: `backend/app/api/sync.py:205-206` (remove enabled update logic)
- Modify: `backend/app/api/sync.py:215` (remove from log message)
- Modify: `backend/app/api/sync.py:454-456` (remove from disconnect_steam)

**Step 1: Remove `enabled` from _config_to_response function**

In `backend/app/api/sync.py`, remove line 80:

```python
# DELETE THIS LINE:
enabled=config.enabled,
```

**Step 2: Remove `enabled` from default config in get_sync_configs**

Remove line 121:

```python
# DELETE THIS LINE:
enabled=False,
```

**Step 3: Remove `enabled` from default config in get_sync_config**

Remove line 160:

```python
# DELETE THIS LINE:
enabled=False,
```

**Step 4: Remove `enabled` update logic in update_sync_config**

Remove lines 205-206:

```python
# DELETE THESE LINES:
if request.enabled is not None:
    config.enabled = request.enabled
```

**Step 5: Update log message in update_sync_config**

Change line 215 from:

```python
f"frequency={config.frequency}, auto_add={config.auto_add}, enabled={config.enabled}"
```

To:

```python
f"frequency={config.frequency}, auto_add={config.auto_add}"
```

**Step 6: Remove enabled update from disconnect_steam**

Remove lines 454-456:

```python
# DELETE THESE LINES:
if config:
    config.enabled = False
    config.updated_at = datetime.now(timezone.utc)
```

Note: Keep the config lookup (lines 448-452) for potential future use, or remove if not needed. Since we're removing the enabled field entirely, we can simplify by removing the whole config update block.

**Step 7: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pyrefly check app/api/sync.py`
Expected: No errors

**Step 8: Commit**

```bash
git add backend/app/api/sync.py
git commit -m "refactor(backend): remove enabled field from sync API endpoints"
```

---

## Task 4: Backend Migration - Remove `enabled` Column

**Files:**
- Modify: `backend/app/alembic/versions/72d8dfbc57f2_initial_schema.py:193` (remove enabled column)

**Step 1: Remove `enabled` column from user_sync_configs table**

In `backend/app/alembic/versions/72d8dfbc57f2_initial_schema.py`, remove line 193:

```python
# DELETE THIS LINE:
sa.Column('enabled', sa.Boolean(), nullable=False),
```

**Step 2: Commit**

```bash
git add backend/app/alembic/versions/72d8dfbc57f2_initial_schema.py
git commit -m "refactor(backend): remove enabled column from user_sync_configs migration"
```

---

## Task 5: Backend Tests - Update Sync API Tests

**Files:**
- Modify: `backend/app/tests/test_sync_api.py` (multiple locations)
- Modify: `backend/app/tests/test_sync_steam_verify.py` (lines 560-578)

**Step 1: Update test_sync_api.py**

Remove all `enabled` references:

Line 38: Remove assertion `assert config["enabled"] is False`
Line 50: Remove `enabled=True,` from UserSyncConfig creation
Line 82: Remove assertion `assert data["enabled"] is False`
Line 134: Remove `enabled=True,` from UserSyncConfig creation
Line 141: Change `json={"frequency": "hourly", "auto_add": True, "enabled": False}` to `json={"frequency": "hourly", "auto_add": True}`
Line 150: Remove assertion `assert data["enabled"] is False`
Line 162: Remove `enabled=True,` from UserSyncConfig creation
Line 179: Remove assertion `assert data["enabled"] is True`

**Step 2: Update test_sync_steam_verify.py**

Lines 560-578: Remove the sync config creation and assertion about enabled field. The test creates a sync config with `enabled=True` and then asserts `config.enabled is False` after disconnect. Since we're removing `enabled`, remove these lines:

```python
# DELETE: Lines 560-564 (sync config creation)
# DELETE: Lines 576-578 (assertion about enabled)
```

**Step 3: Run backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pytest app/tests/test_sync_api.py app/tests/test_sync_steam_verify.py -v`
Expected: All tests pass

**Step 4: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pytest -q`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/tests/test_sync_api.py backend/app/tests/test_sync_steam_verify.py
git commit -m "test(backend): update sync tests to remove enabled field references"
```

---

## Task 6: Frontend Types - Remove `enabled` from SyncConfig

**Files:**
- Modify: `frontend/src/types/sync.ts:26` (remove from SyncConfig interface)
- Modify: `frontend/src/types/sync.ts:36` (remove from SyncConfigUpdateData)

**Step 1: Remove `enabled` from SyncConfig interface**

In `frontend/src/types/sync.ts`, remove line 26:

```typescript
// DELETE THIS LINE:
enabled: boolean;
```

**Step 2: Remove `enabled` from SyncConfigUpdateData interface**

Remove line 36:

```typescript
// DELETE THIS LINE:
enabled?: boolean;
```

**Step 3: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run check`
Expected: Type errors will appear - this is expected. We'll fix them in subsequent tasks.

**Step 4: Commit**

```bash
git add frontend/src/types/sync.ts
git commit -m "refactor(frontend): remove enabled field from sync types"
```

---

## Task 7: Frontend SyncServiceCard - Remove Enable Toggle

**Files:**
- Modify: `frontend/src/components/sync/sync-service-card.tsx`

**Step 1: Remove localEnabled state and handler**

Remove line 57:

```typescript
// DELETE THIS LINE:
const [localEnabled, setLocalEnabled] = useState(config.enabled);
```

Remove lines 64-67 (handleEnabledChange function):

```typescript
// DELETE THESE LINES:
const handleEnabledChange = async (enabled: boolean) => {
  setLocalEnabled(enabled);
  await onUpdate({ enabled });
};
```

**Step 2: Update badge logic**

Replace lines 102-119 with:

```typescript
        <Badge
          variant={!config.isConfigured ? 'outline' : 'default'}
          className={
            !config.isConfigured
              ? 'bg-muted text-muted-foreground'
              : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
          }
        >
          {!config.isConfigured ? 'Not Configured' : 'Connected'}
        </Badge>
```

**Step 3: Remove the Enable Toggle section**

Delete lines 123-132 (the entire Enable Toggle div):

```typescript
// DELETE THIS ENTIRE BLOCK:
{/* Enable Toggle */}
<div className="flex items-center justify-between">
  <span className="text-sm font-medium">Enable sync</span>
  <Switch
    checked={localEnabled}
    onCheckedChange={handleEnabledChange}
    disabled={isUpdating || !config.isConfigured}
  />
</div>
```

**Step 4: Update frequency select disabled condition**

Change line 140 from:

```typescript
disabled={!localEnabled || isUpdating || !config.isConfigured}
```

To:

```typescript
disabled={isUpdating || !config.isConfigured}
```

**Step 5: Update auto-add toggle disabled condition**

Change line 161 from:

```typescript
disabled={!localEnabled || isUpdating || !config.isConfigured}
```

To:

```typescript
disabled={isUpdating || !config.isConfigured}
```

**Step 6: Update Sync Now button disabled condition**

Change line 176 from:

```typescript
disabled={!localEnabled || isCurrentlySyncing || !config.isConfigured}
```

To:

```typescript
disabled={isCurrentlySyncing || !config.isConfigured}
```

**Step 7: Remove Switch import if no longer used**

Check if Switch is still used. If not, remove it from imports (line 6).

**Step 8: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run check`
Expected: Fewer type errors than before

**Step 9: Commit**

```bash
git add frontend/src/components/sync/sync-service-card.tsx
git commit -m "refactor(frontend): remove enable toggle from SyncServiceCard"
```

---

## Task 8: Frontend SteamConnectionCard - Remove `enabled` Prop

**Files:**
- Modify: `frontend/src/components/sync/steam-connection-card.tsx`

**Step 1: Remove `enabled` from props interface**

Remove line 51:

```typescript
// DELETE THIS LINE:
enabled: boolean;
```

**Step 2: Remove `enabled` from destructured props**

Change line 59 from:

```typescript
enabled,
```

Remove it entirely.

**Step 3: Update getBadgeState function**

Replace lines 136-140 with:

```typescript
const getBadgeState = () => {
  if (!isConfigured) return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
  return { label: 'Connected', className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' };
};
```

**Step 4: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run check`
Expected: Fewer type errors - may still have errors from components using SteamConnectionCard

**Step 5: Commit**

```bash
git add frontend/src/components/sync/steam-connection-card.tsx
git commit -m "refactor(frontend): remove enabled prop from SteamConnectionCard"
```

---

## Task 9: Frontend - Update Components Using SteamConnectionCard

**Files:**
- Search for and modify any files that use `<SteamConnectionCard` with `enabled` prop

**Step 1: Find usages**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && grep -r "enabled=" --include="*.tsx" src/ | grep -i steam`

**Step 2: Update each usage**

Remove the `enabled={...}` prop from each `<SteamConnectionCard` usage found.

**Step 3: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run check`
Expected: No type errors related to enabled

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor(frontend): remove enabled prop from SteamConnectionCard usages"
```

---

## Task 10: Frontend Tests - Update SyncServiceCard Tests

**Files:**
- Modify: `frontend/src/components/sync/sync-service-card.test.tsx`

**Step 1: Update mock config factory**

Remove `enabled: true` from line 21 in createMockConfig.

**Step 2: Remove/update tests that reference enabled**

Tests to remove entirely:
- "shows Enabled badge when configured and enabled" (line 87)
- "shows Disabled badge when configured but not enabled" (line 100)
- Tests related to enable toggle (lines 130-165)
- "disables frequency select when not enabled" (line 223)
- "disables auto-add toggle when not enabled" (line 308)
- "disables sync button when not enabled" (line 375)

Tests to update:
- Update remaining tests to not pass `enabled` to createMockConfig
- Update badge tests to check for "Connected" instead of "Enabled"/"Disabled"

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run test -- src/components/sync/sync-service-card.test.tsx`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/components/sync/sync-service-card.test.tsx
git commit -m "test(frontend): update SyncServiceCard tests for removed enabled toggle"
```

---

## Task 11: Frontend Tests - Update SteamConnectionCard Tests

**Files:**
- Modify: `frontend/src/components/sync/steam-connection-card.test.tsx`

**Step 1: Remove all `enabled` prop references**

Remove `enabled={true}` or `enabled={false}` from all test renders.

**Step 2: Update badge-related tests**

- Remove test "renders 'Enabled' badge when configured and enabled"
- Remove test "renders 'Disabled' badge when configured but not enabled"
- Update to test for "Connected" badge when configured

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run test -- src/components/sync/steam-connection-card.test.tsx`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/components/sync/steam-connection-card.test.tsx
git commit -m "test(frontend): update SteamConnectionCard tests for removed enabled prop"
```

---

## Task 12: Frontend Tests - Update Remaining Test Files

**Files:**
- Modify: `frontend/src/app/(main)/sync/page.test.tsx`
- Modify: `frontend/src/hooks/use-sync.test.ts`
- Modify: `frontend/src/api/sync.test.ts`

**Step 1: Update sync page tests**

Remove references to `enabled` in mock data and assertions.

**Step 2: Update use-sync hook tests**

Remove `enabled` from mock configs and update assertions.

**Step 3: Update sync API tests**

Remove `enabled` from mock responses.

**Step 4: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run test`
Expected: All tests pass

**Step 5: Commit**

```bash
git add frontend/src/app/(main)/sync/page.test.tsx frontend/src/hooks/use-sync.test.ts frontend/src/api/sync.test.ts
git commit -m "test(frontend): update remaining tests for removed enabled field"
```

---

## Task 13: Final Verification

**Step 1: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pytest -q`
Expected: All tests pass

**Step 2: Run backend type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Run full frontend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run test`
Expected: All tests pass

**Step 4: Run frontend type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/frontend && npm run check`
Expected: No errors

**Step 5: Reset database and verify migrations work**

Run: `podman-compose down -v && podman-compose up -d postgres && sleep 3 && cd /home/abo/workspace/home/nexorious/.worktrees/remove-sync-enabled/backend && uv run alembic upgrade head`
Expected: Migrations apply successfully

**Step 6: Commit any final fixes if needed**

If any issues were found and fixed, commit them.

---

## Summary

This plan removes the `enabled` field from:
1. Backend model (`UserSyncConfig`)
2. Backend schemas (`SyncConfigResponse`, `SyncConfigUpdateRequest`, `SyncConfigCreateRequest`)
3. Backend API endpoints (response builders, update logic, disconnect logic)
4. Backend migration (database schema)
5. Backend tests
6. Frontend types (`SyncConfig`, `SyncConfigUpdateData`)
7. Frontend components (`SyncServiceCard`, `SteamConnectionCard`)
8. Frontend tests

After implementation:
- Sync frequency `MANUAL` serves as "disabled" (no automatic sync)
- UI controls are disabled when `!isConfigured` (instead of `!enabled`)
- "Sync Now" is available immediately when configured
- Badge shows "Connected" when configured (instead of "Enabled"/"Disabled")
