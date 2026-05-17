# Reset Sync Data Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Reset sync data" action that wipes all external games, match history, and platform entries for a user+storefront combination without touching credentials or user_games.

**Architecture:** A new `DELETE /api/sync/:storefront/data` endpoint cancels any active sync job, then deletes `user_game_platforms` and `external_games` rows for the user+storefront in a transaction and NULLs `last_synced_at`. A correctness fix to `syncCheckJobCompletion` prevents a mid-flight River worker from flipping a cancelled job back to completed. The frontend adds a guarded reset button with a confirmation dialog to each sync service card.

**Tech Stack:** Go (Echo v5, Bun, River), React 19, TanStack Query, shadcn/ui AlertDialog

---

### Task 1: Guard `syncCheckJobCompletion` against overwriting a cancelled job

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Test: `internal/worker/tasks/sync_test.go`

When a reset cancels a job and deletes `external_games`, any `ProcessSyncItemWorker` already mid-flight will fail to load its game, call `syncMarkItemFailed`, then `syncCheckJobCompletion`. Without a guard, `syncCheckJobCompletion` overwrites `cancelled` with `completed`. Fix: add `AND status IN ('pending', 'processing')` to both terminal UPDATE statements in `syncCheckJobCompletion`.

- [ ] **Step 1: Write the failing test**

Add at the end of `internal/worker/tasks/sync_test.go`:

```go
func TestProcessSyncItem_CancelledJobNotOverwritten(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	// Job is already cancelled — simulates a reset having run.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'steam', 'cancelled', 'low', 1)`,
		jobID, userID,
	)

	// The external_game was deleted by the reset; job_item still references it.
	egID := uuid.NewString()
	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: nil}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "cancelled" {
		t.Errorf("expected job status=cancelled after mid-flight worker, got %q", status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```
go test ./internal/worker/tasks/... -run TestProcessSyncItem_CancelledJobNotOverwritten -v
```

Expected: FAIL — job status will be `completed` instead of `cancelled`.

- [ ] **Step 3: Apply the guard to both terminal UPDATE statements in `syncCheckJobCompletion`**

In `internal/worker/tasks/sync.go`, find the two `UPDATE jobs SET status =` statements at the end of `syncCheckJobCompletion` and add the guard to both.

First (in the `autoRetryDone` branch, sets `completed_with_errors`):
```go
// old
`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ?`
// new
`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`
```

Second (final statement, sets `completed`):
```go
// old
`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ?`
// new
`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`
```

- [ ] **Step 4: Run the test to verify it passes**

```
go test ./internal/worker/tasks/... -run TestProcessSyncItem_CancelledJobNotOverwritten -v
```

Expected: PASS.

- [ ] **Step 5: Run the full tasks test suite**

```
go test ./internal/worker/tasks/... -timeout 600s -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "fix(sync): guard syncCheckJobCompletion against overwriting terminal job status"
```

---

### Task 2: `HandleResetSyncData` backend handler

**Files:**
- Modify: `internal/api/sync.go`
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Add at the end of `internal/api/sync_test.go`:

```go
// ── TestResetSyncData ─────────────────────────────────────────────────────────

func TestResetSyncData_DeletesDataAndResetsTimestamp(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "reset-data")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at, created_at, updated_at)
		 VALUES (gen_random_uuid(), ?, 'steam', 'manual', false, now(), now(), now())`,
		userID,
	)
	insertExternalGame(t, testDB, "eg-reset-1", userID, "steam", "730", "CS2")
	insertUserGameAndPlatform(t, testDB, "ug-reset-1", userID, "12345", "ugp-reset-1", "eg-reset-1")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	var egCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &egCount)
	if egCount != 0 {
		t.Errorf("expected 0 external_games, got %d", egCount)
	}

	var ugpCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-reset-1'`).Scan(ctx, &ugpCount)
	if ugpCount != 0 {
		t.Errorf("expected 0 user_game_platforms, got %d", ugpCount)
	}

	// user_games must survive the reset.
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-reset-1'`).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected user_game to survive reset, got %d rows", ugCount)
	}

	var lastSyncedAt *time.Time
	_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`, userID).Scan(ctx, &lastSyncedAt)
	if lastSyncedAt != nil {
		t.Errorf("expected last_synced_at=NULL after reset, got %v", lastSyncedAt)
	}
}

func TestResetSyncData_CancelsActiveJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "reset-cancel")

	insertJob(t, testDB, "job-reset-active", userID, "sync", "steam", "processing")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = 'job-reset-active'`).Scan(context.Background(), &status)
	if status != "cancelled" {
		t.Errorf("expected active job to be cancelled, got %q", status)
	}
}

func TestResetSyncData_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "reset-invalid")

	rec := deleteAuth(t, e, "/api/sync/fakefront/data", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResetSyncData_Unauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})

	req := httptest.NewRequest(http.MethodDelete, "/api/sync/steam/data", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestResetSyncData_EmptyState(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "reset-empty")

	rec := deleteAuth(t, e, "/api/sync/steam/data", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on empty reset (idempotent), got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```
go test ./internal/api/... -run "TestResetSyncData" -v
```

Expected: FAIL — route not found (404 or method not allowed).

- [ ] **Step 3: Add the handler and register the route in `internal/api/sync.go`**

Add the route registration inside `RegisterRoutes`, after the `g.PUT("/config/:storefront", ...)` line and before `g.POST("/:storefront", ...)`:

```go
g.DELETE("/:storefront/data", h.HandleResetSyncData)
```

Add the handler method at the end of the file:

```go
// HandleResetSyncData handles DELETE /api/sync/:storefront/data.
// It cancels any active sync job for the storefront, then deletes all
// external_games and user_game_platforms rows for the user+storefront,
// and resets last_synced_at. Credentials are not affected.
func (h *SyncHandler) HandleResetSyncData(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validConfigStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// Cancel any active sync job for this user+storefront.
	var activeJob models.Job
	if err := h.db.NewRaw(
		`SELECT * FROM jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &activeJob); err == nil {
		now := time.Now().UTC()
		_, _ = h.db.NewRaw(
			`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ?`,
			models.JobStatusCancelled, now, activeJob.ID,
		).Exec(ctx)
		_, _ = h.db.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
			activeJob.ID,
		).Exec(ctx)
	}

	if err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(
			`DELETE FROM user_game_platforms
			 WHERE storefront = ? AND user_game_id IN (SELECT id FROM user_games WHERE user_id = ?)`,
			sf, userID,
		).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`DELETE FROM external_games WHERE user_id = ? AND storefront = ?`,
			userID, sf,
		).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(
			`UPDATE user_sync_configs SET last_synced_at = NULL, updated_at = now()
			 WHERE user_id = ? AND storefront = ?`,
			userID, sf,
		).Exec(ctx)
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset sync data")
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```
go test ./internal/api/... -run "TestResetSyncData" -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Run the full API test suite and linter**

```
go test ./internal/api/... -timeout 600s
golangci-lint run ./internal/api/...
```

Expected: all tests pass, zero lint errors.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): add DELETE /:storefront/data reset endpoint"
```

---

### Task 3: Frontend API function and mutation hook

**Files:**
- Modify: `ui/frontend/src/api/sync.ts`
- Modify: `ui/frontend/src/hooks/use-sync.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Add `resetSyncData` to `ui/frontend/src/api/sync.ts`**

Add at the end of the file:

```typescript
export async function resetSyncData(platform: SyncPlatform): Promise<void> {
  const response = await fetch(`/api/sync/${platform}/data`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error((data as { message?: string }).message ?? 'Failed to reset sync data');
  }
}
```

- [ ] **Step 2: Add `useResetSyncData` to `ui/frontend/src/hooks/use-sync.ts`**

Add after the `useDisconnectSteam` function (before the Epic Auth Hooks section comment):

```typescript
export function useResetSyncData() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, SyncPlatform>({
    mutationFn: (platform) => syncApi.resetSyncData(platform),
    onSuccess: (_, platform) => {
      queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(platform) });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}
```

- [ ] **Step 3: Export `useResetSyncData` from `ui/frontend/src/hooks/index.ts`**

Find the existing `export { ... } from './use-sync'` block and add `useResetSyncData` to it.

- [ ] **Step 4: Type-check**

```
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/sync.ts ui/frontend/src/hooks/use-sync.ts ui/frontend/src/hooks/index.ts
git commit -m "feat(sync): add resetSyncData API function and useResetSyncData hook"
```

---

### Task 4: Reset button with confirmation dialog

**Files:**
- Modify: `ui/frontend/src/components/sync/sync-service-card.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx`

- [ ] **Step 1: Verify AlertDialog is available**

```
grep -r "AlertDialog" ui/frontend/src/components/ui/
```

If the file `ui/frontend/src/components/ui/alert-dialog.tsx` exists, proceed. If not, add it:

```
cd ui/frontend && npx shadcn@latest add alert-dialog
```

- [ ] **Step 2: Add `onReset` prop and reset button to `ui/frontend/src/components/sync/sync-service-card.tsx`**

Replace the file content with the following (all existing code preserved, additions marked):

```typescript
import { useState } from 'react';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Loader2, RefreshCw, History } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { config as envConfig } from '@/lib/env';
import type { SyncConfig, SyncStatus, SyncConfigUpdateData } from '@/types';
import { SyncFrequency, getSyncFrequencyLabel, getPlatformDisplayInfo } from '@/types';

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  onReset?: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
  isResetting?: boolean;
}

function formatLastSync(dateStr: string | null): string {
  if (!dateStr) return 'Never';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

export function SyncServiceCard({
  config,
  status,
  pendingReviewCount,
  onUpdate,
  onTriggerSync,
  onReset,
  isUpdating = false,
  isSyncing = false,
  isResetting = false,
}: SyncServiceCardProps) {
  const [localFrequency, setLocalFrequency] = useState(config.frequency);
  const [localAutoAdd, setLocalAutoAdd] = useState(config.autoAdd);

  const platformInfo = getPlatformDisplayInfo(config.platform);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

  const handleFrequencyChange = async (frequency: SyncFrequency) => {
    setLocalFrequency(frequency);
    await onUpdate({ frequency });
  };

  const handleAutoAddChange = async (autoAdd: boolean) => {
    setLocalAutoAdd(autoAdd);
    await onUpdate({ autoAdd });
  };

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`flex h-12 w-12 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <img
                src={`${envConfig.staticUrl}${platformInfo.iconUrl}`}
                alt={`${platformInfo.name} icon`}
                width={28}
                height={28}
                className="h-7 w-7"
                loading="lazy"
              />
            </div>
            <div>
              <CardTitle className="text-lg">{platformInfo.name}</CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Badge variant="destructive">
                {pendingReviewCount}
              </Badge>
            )}
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
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Frequency Select */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Sync frequency</span>
          <Select
            value={localFrequency}
            onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
            disabled={isUpdating || !config.isConfigured}
          >
            <SelectTrigger className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.values(SyncFrequency).map((freq) => (
                <SelectItem key={freq} value={freq}>
                  {getSyncFrequencyLabel(freq)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Auto-add Toggle */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Auto-add games</span>
          <Switch
            checked={localAutoAdd}
            onCheckedChange={handleAutoAddChange}
            disabled={isUpdating || !config.isConfigured}
          />
        </div>
      </CardContent>

      <CardFooter className="flex items-center justify-between border-t bg-muted/50 px-6 py-4">
        <Link
          to="/sync/$platform" params={{ platform: config.platform }}
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          <History className="h-4 w-4" />
          View details
        </Link>
        <div className="flex items-center gap-2">
          {onReset && config.isConfigured && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={isCurrentlySyncing || isResetting}
                >
                  {isResetting ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Reset'}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Reset sync data?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will remove all imported games and match history for {platformInfo.name}.
                    Your game library entries will not be deleted. This cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={onReset}>Reset</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
          <Button
            onClick={onTriggerSync}
            disabled={isCurrentlySyncing || !config.isConfigured}
            size="sm"
          >
            {isCurrentlySyncing ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Syncing...
              </>
            ) : (
              <>
                <RefreshCw className="mr-2 h-4 w-4" />
                Sync Now
              </>
            )}
          </Button>
        </div>
      </CardFooter>
    </Card>
  );
}
```

- [ ] **Step 3: Wire up `useResetSyncData` in `ui/frontend/src/routes/_authenticated/sync/index.tsx`**

Add `useResetSyncData` to the hook import line at the top:

```typescript
import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, useSyncStatus, usePendingReviewCount, useResetSyncData } from '@/hooks';
```

In `SyncServiceCardWithStatus`, add the mutation and handler:

```typescript
function SyncServiceCardWithStatus({
  config,
  onUpdate,
  onTriggerSync,
}: {
  config: SyncConfig;
  onUpdate: (platform: SyncPlatform, data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: (platform: SyncPlatform) => Promise<void>;
}) {
  const { data: status } = useSyncStatus(config.platform);
  const { data: reviewData } = usePendingReviewCount();
  const { isPending: isUpdating } = useUpdateSyncConfig();
  const { isPending: isSyncing } = useTriggerSync();
  const { mutateAsync: resetSync, isPending: isResetting } = useResetSyncData();

  const pendingReviewCount = reviewData?.countsBySource?.[config.platform] ?? 0;

  const handleUpdate = async (data: SyncConfigUpdateData) => {
    await onUpdate(config.platform, data);
  };

  const handleTriggerSync = async () => {
    await onTriggerSync(config.platform);
  };

  const handleReset = async () => {
    await resetSync(config.platform);
  };

  return (
    <SyncServiceCard
      config={config}
      status={status}
      pendingReviewCount={pendingReviewCount}
      onUpdate={handleUpdate}
      onTriggerSync={handleTriggerSync}
      onReset={handleReset}
      isUpdating={isUpdating}
      isSyncing={isSyncing}
      isResetting={isResetting}
    />
  );
}
```

Also add a `handleReset` in `SyncPage` that wraps the mutation with a toast (add after `handleTriggerSync`):

```typescript
const { mutateAsync: resetSync } = useResetSyncData();

const handleResetSync = async (platform: SyncPlatform) => {
  try {
    await resetSync(platform);
    toast.success(`${platform} sync data reset successfully`);
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to reset sync data';
    toast.error(message);
    throw err;
  }
};
```

Wait — looking at the component structure again: `SyncServiceCardWithStatus` handles its own mutation directly (it already calls `useTriggerSync` internally). The `handleReset` in `SyncServiceCardWithStatus` handles both the call and the toast. Remove `handleResetSync` from `SyncPage` — the reset mutation and toast live entirely in `SyncServiceCardWithStatus`. Update `handleReset` in `SyncServiceCardWithStatus` to include the toast:

```typescript
const handleReset = async () => {
  try {
    await resetSync(config.platform);
    toast.success(`${config.platform} sync data reset successfully`);
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to reset sync data';
    toast.error(message);
    throw err;
  }
};
```

- [ ] **Step 4: Type-check and run frontend tests**

```
cd ui/frontend && npm run check && npm run test
```

Expected: zero type errors, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/sync/sync-service-card.tsx \
        ui/frontend/src/routes/_authenticated/sync/index.tsx
git commit -m "feat(sync): add reset sync data button with confirmation dialog"
```

---

## Self-Review

**Spec coverage:**
- ✅ `external_games` deleted per user+storefront — Task 2
- ✅ `user_game_platforms` deleted for that storefront — Task 2
- ✅ `user_games` untouched — verified in `TestResetSyncData_DeletesDataAndResetsTimestamp`
- ✅ `last_synced_at` reset to NULL — Task 2
- ✅ Credentials untouched — handler touches no credential columns
- ✅ Active job cancelled before delete — Task 2, `TestResetSyncData_CancelsActiveJob`
- ✅ `syncCheckJobCompletion` guard — Task 1
- ✅ 204 idempotent on empty state — Task 2, `TestResetSyncData_EmptyState`
- ✅ 400 on invalid storefront, 401 on no auth — Task 2
- ✅ `resetSyncData` API function — Task 3
- ✅ `useResetSyncData` hook with query invalidation — Task 3
- ✅ Reset button disabled while syncing — Task 4 (`disabled={isCurrentlySyncing || isResetting}`)
- ✅ Confirmation dialog with correct copy — Task 4
- ✅ Button only shown when `config.isConfigured` — Task 4
