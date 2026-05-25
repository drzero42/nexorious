# Sync UI Remaining Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix four spec divergences in the sync UI: add game count to hub cards, remove the per-item drill-down panel shown during sync, wire live polling to ExternalGamesSection, and fix the connection section collapsible initial/auto-close behaviour.

**Architecture:** Three of the four changes are isolated to `$storefront.tsx` + one hook/component each. G-F6 is the only change touching the backend (one field added to the status endpoint response + test). All changes are additive or simplifying — no data model changes, no migrations.

**Tech Stack:** Go + Bun ORM (backend), React 19 + TanStack Query v5 + TypeScript (frontend), Vitest (frontend tests), stdlib testing + testcontainers-go (backend tests).

---

## File Map

| File | Change |
|------|--------|
| `internal/api/sync.go` | Add `ExternalGameCount` to `syncStatusResponse`; query count in `HandleGetSyncStatus` |
| `internal/api/sync_test.go` | Extend `TestSyncStatus_ReflectsActiveJob` to assert `external_game_count` |
| `ui/frontend/src/types/sync.ts` | Add `externalGameCount: number` to `SyncStatus` |
| `ui/frontend/src/api/sync.ts` | Add `external_game_count` to `SyncStatusApiResponse`; map in `transformSyncStatus` |
| `ui/frontend/src/components/sync/sync-service-card.tsx` | Add `externalGameCount?: number` prop; render "N games" line |
| `ui/frontend/src/routes/_authenticated/sync/index.tsx` | Pass `externalGameCount` from status to `SyncServiceCard` |
| `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` | Remove `JobItemsDetails` block + import; add two `useEffect`s for collapsible; pass `isSyncing` to `ExternalGamesSection`; invalidate external games on terminal |
| `ui/frontend/src/hooks/use-sync.ts` | Add optional `refetchInterval` override param to `useExternalGames` |
| `ui/frontend/src/components/sync/external-games-section.tsx` | Add `isSyncing?: boolean` prop; pass `refetchInterval` to hook |

---

## Task 1: Backend — add external_game_count to sync status endpoint

**Files:**
- Modify: `internal/api/sync.go:124-129` (syncStatusResponse) and `:414-447` (HandleGetSyncStatus)
- Modify: `internal/api/sync_test.go:188-217` (TestSyncStatus_ReflectsActiveJob)

- [ ] **Step 1: Extend the test to assert external_game_count**

Open `internal/api/sync_test.go`. Find `TestSyncStatus_ReflectsActiveJob` (line 188). Add an assertion that `external_game_count` is present and is a number in the status response. The check goes right after the first `getAuth` call that asserts `is_syncing=false`:

```go
if _, ok := status["external_game_count"]; !ok {
    t.Fatal("expected external_game_count in status response")
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/api/... -run TestSyncStatus_ReflectsActiveJob -v
```

Expected: FAIL — `external_game_count` key not found in response.

- [ ] **Step 3: Add ExternalGameCount to syncStatusResponse**

In `internal/api/sync.go`, change the `syncStatusResponse` struct (around line 124):

```go
type syncStatusResponse struct {
	Storefront        string     `json:"storefront"`
	IsSyncing         bool       `json:"is_syncing"`
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	ActiveJobID       *string    `json:"active_job_id"`
	ExternalGameCount int        `json:"external_game_count"`
}
```

- [ ] **Step 4: Query the count in HandleGetSyncStatus**

In `internal/api/sync.go`, in the body of `HandleGetSyncStatus` (around line 414), add a count query after the `lastSyncedAt` block and before the `return c.JSON(...)` call:

```go
var externalGameCount int
if err := h.db.NewSelect().
    TableExpr("external_games").
    ColumnExpr("COUNT(*)").
    Where("user_id = ? AND storefront = ? AND is_available = true", userID, sf).
    Scan(ctx, &externalGameCount); err != nil {
    externalGameCount = 0
}
```

Then update the return statement to include the new field:

```go
return c.JSON(http.StatusOK, syncStatusResponse{
    Storefront:        sf,
    IsSyncing:         activeJobID != nil,
    LastSyncedAt:      lastSyncedAt,
    ActiveJobID:       activeJobID,
    ExternalGameCount: externalGameCount,
})
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
go test ./internal/api/... -run TestSyncStatus_ReflectsActiveJob -v
```

Expected: PASS

- [ ] **Step 6: Run all sync tests**

```bash
go test ./internal/api/... -v 2>&1 | tail -20
```

Expected: all PASS, no compilation errors.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): add external_game_count to sync status response"
```

---

## Task 2: Frontend types and API layer — wire external_game_count

**Files:**
- Modify: `ui/frontend/src/types/sync.ts:41-48` (SyncStatus interface)
- Modify: `ui/frontend/src/api/sync.ts:40-45` (SyncStatusApiResponse) and `:80-87` (transformSyncStatus)

- [ ] **Step 1: Add externalGameCount to SyncStatus**

In `ui/frontend/src/types/sync.ts`, update the `SyncStatus` interface (around line 41):

```typescript
export interface SyncStatus {
  storefront: SyncStorefront;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
  requiresReauth?: boolean;
  authExpired?: boolean;
  externalGameCount: number;
}
```

- [ ] **Step 2: Add external_game_count to SyncStatusApiResponse**

In `ui/frontend/src/api/sync.ts`, update `SyncStatusApiResponse` (around line 40):

```typescript
interface SyncStatusApiResponse {
  storefront: string;
  is_syncing: boolean;
  last_synced_at: string | null;
  active_job_id: string | null;
  external_game_count: number;
}
```

- [ ] **Step 3: Map the field in transformSyncStatus**

In `ui/frontend/src/api/sync.ts`, update `transformSyncStatus` (around line 80):

```typescript
function transformSyncStatus(apiStatus: SyncStatusApiResponse): SyncStatus {
  return {
    storefront: apiStatus.storefront as SyncStorefront,
    isSyncing: apiStatus.is_syncing,
    lastSyncedAt: apiStatus.last_synced_at,
    activeJobId: apiStatus.active_job_id,
    externalGameCount: apiStatus.external_game_count ?? 0,
  };
}
```

- [ ] **Step 4: Fix the optimistic update in useTriggerSync**

In `ui/frontend/src/hooks/use-sync.ts`, `useTriggerSync` (around line 128) constructs a `SyncStatus` object inline. Adding `externalGameCount` as a required field will make TypeScript complain. Update the optimistic update callback:

```typescript
queryClient.setQueryData(
  syncKeys.status(platform),
  (old: SyncStatus | undefined) => ({
    storefront: old?.storefront ?? platform,
    isSyncing: true,
    lastSyncedAt: old?.lastSyncedAt ?? null,
    activeJobId: result.jobId,
    externalGameCount: old?.externalGameCount ?? 0,
  })
);
```

- [ ] **Step 5: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/types/sync.ts ui/frontend/src/api/sync.ts \
        ui/frontend/src/hooks/use-sync.ts
git commit -m "feat(sync): add externalGameCount to SyncStatus type and API transform"
```

---

## Task 3: Frontend — display game count on hub card

**Files:**
- Modify: `ui/frontend/src/components/sync/sync-service-card.tsx:10-17` (props interface) and `:62-76` (name/subtitle area)
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx:66-75` (SyncServiceCardWithStatus)

- [ ] **Step 1: Add externalGameCount prop to SyncServiceCard**

In `ui/frontend/src/components/sync/sync-service-card.tsx`, update the props interface (around line 10):

```typescript
interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  credentialsError?: boolean;
  onTriggerSync: () => Promise<void>;
  isSyncing?: boolean;
  externalGameCount?: number;
}
```

Also destructure it in the function signature (around line 35):

```typescript
export function SyncServiceCard({
  config,
  status,
  pendingReviewCount,
  credentialsError = false,
  onTriggerSync,
  isSyncing = false,
  externalGameCount,
}: SyncServiceCardProps) {
```

- [ ] **Step 2: Render the count below the storefront name**

In the same file, add the count line below the "Last synced" `<p>` (around line 74):

```tsx
<div>
  <CardTitle className="text-lg">
    <Link
      to="/sync/$storefront"
      params={{ storefront: config.storefront }}
      className="hover:underline"
    >
      {platformInfo.name}
    </Link>
  </CardTitle>
  <p className="text-sm text-muted-foreground">
    Last synced: {formatLastSync(config.lastSyncedAt)}
  </p>
  {externalGameCount !== undefined && externalGameCount > 0 && (
    <p className="text-sm text-muted-foreground">{externalGameCount} games</p>
  )}
</div>
```

- [ ] **Step 3: Pass externalGameCount from SyncServiceCardWithStatus**

In `ui/frontend/src/routes/_authenticated/sync/index.tsx`, update the `SyncServiceCardWithStatus` component to pass the count. The `status` data is already fetched via `useSyncStatus(config.storefront)`. Pass it through:

```tsx
return (
  <SyncServiceCard
    config={config}
    status={status}
    pendingReviewCount={pendingReviewCount}
    credentialsError={credentialsError}
    onTriggerSync={handleTriggerSync}
    isSyncing={isSyncing}
    externalGameCount={status?.externalGameCount ?? 0}
  />
);
```

- [ ] **Step 4: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/sync/sync-service-card.tsx \
        ui/frontend/src/routes/_authenticated/sync/index.tsx
git commit -m "feat(sync): show external game count on hub cards"
```

---

## Task 4: Remove JobItemsDetails from sync detail page

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx:30` (import) and `:518-530` (JSX block)

- [ ] **Step 1: Remove the JobItemsDetails import**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`, line 30, change:

```typescript
import { JobProgressCard, JobItemsDetails } from '@/components/jobs';
```

to:

```typescript
import { JobProgressCard } from '@/components/jobs';
```

- [ ] **Step 2: Remove the JobItemsDetails JSX block**

Find the "Active Sync Progress" section (around line 517). Replace:

```tsx
{/* Active Sync Progress */}
{isSyncing && activeJob && (
  <div className="space-y-4">
    <JobProgressCard
      job={activeJob}
      onCancel={handleCancelJob}
      isCancelling={isCancelling}
    />

    {activeJob.progress && (
      <JobItemsDetails jobId={activeJob.id} progress={activeJob.progress} isTerminal={activeJob.isTerminal} />
    )}
  </div>
)}
```

with:

```tsx
{/* Active Sync Progress */}
{isSyncing && activeJob && (
  <JobProgressCard
    job={activeJob}
    onCancel={handleCancelJob}
    isCancelling={isCancelling}
  />
)}
```

- [ ] **Step 3: Type-check and dead-code check**

```bash
cd ui/frontend && npm run check && npm run knip
```

Expected: zero errors, zero knip findings (JobItemsDetails may now be unused — knip will flag it if nothing else imports it; if so, that is the expected correct result, not a problem to suppress).

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/$storefront.tsx
git commit -m "fix(sync): remove JobItemsDetails panel from sync detail page"
```

---

## Task 5: Wire ExternalGamesSection polling during active sync

**Files:**
- Modify: `ui/frontend/src/hooks/use-sync.ts:362-367` (useExternalGames)
- Modify: `ui/frontend/src/components/sync/external-games-section.tsx` (add isSyncing prop)
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` (pass prop + invalidate on terminal)

- [ ] **Step 1: Add refetchInterval option to useExternalGames**

In `ui/frontend/src/hooks/use-sync.ts`, update `useExternalGames` (around line 362):

```typescript
export function useExternalGames(platform: SyncStorefront, options?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: syncKeys.externalGames(platform),
    queryFn: () => syncApi.getExternalGames(platform),
    refetchInterval: options?.refetchInterval,
  });
}
```

- [ ] **Step 2: Add isSyncing prop to ExternalGamesSection**

In `ui/frontend/src/components/sync/external-games-section.tsx`, the props interface is at line 47 and the function signature at line 56. Make these changes:

```typescript
// Props interface (line 47)
interface ExternalGamesSectionProps {
  storefront: SyncStorefront;
  isSyncing?: boolean;
}

// Function signature (line 56)
export function ExternalGamesSection({ storefront, isSyncing = false }: ExternalGamesSectionProps) {
```

Then update the `useExternalGames` call (currently line 57):

```typescript
const { data: games = [], isLoading } = useExternalGames(storefront, {
  refetchInterval: isSyncing ? 5000 : undefined,
});
```

- [ ] **Step 3: Pass isSyncing to ExternalGamesSection in $storefront.tsx and invalidate on terminal**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`:

a) Update the `<ExternalGamesSection>` render call (near the bottom of the JSX):

```tsx
{/* External Games Library */}
<ExternalGamesSection storefront={storefront} isSyncing={!!isSyncing} />
```

b) In the existing `useEffect` that fires when `activeJob` becomes terminal (around line 166), add an `invalidateQueries` call for external games:

```typescript
useEffect(() => {
  if (activeJob?.isTerminal && activeJob.id !== invalidatedJobRef.current) {
    invalidatedJobRef.current = activeJob.id;
    queryClient.invalidateQueries({ queryKey: jobsKeys.recent(storefront) });
    queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
  }
}, [activeJob?.isTerminal, activeJob?.id, storefront, queryClient]);
```

- [ ] **Step 4: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/sync/external-games-section.tsx \
        ui/frontend/src/routes/_authenticated/sync/$storefront.tsx
git commit -m "fix(sync): poll ExternalGamesSection every 5s during active sync"
```

---

## Task 6: Fix connection section collapsible open/close behaviour

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx:204-206` (useState initializer → two effects)

- [ ] **Step 1: Replace the lazy useState initializer with a ref-guarded two-effect pattern**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`, find (around line 204):

```typescript
// Connection section is open by default when not configured or when there's a credentials error
const [connectionSectionOpen, setConnectionSectionOpen] = useState(
  () => !config?.isConfigured || credentialsError
);
```

Replace it with:

```typescript
const [connectionSectionOpen, setConnectionSectionOpen] = useState(false);
const connectionOpenInitialized = useRef(false);

// Set initial state once config data arrives from the server
useEffect(() => {
  if (!connectionOpenInitialized.current && config !== undefined) {
    connectionOpenInitialized.current = true;
    setConnectionSectionOpen(!config.isConfigured || credentialsError);
  }
}, [config, credentialsError]);

// Auto-collapse when the user successfully connects and there is no credentials error
useEffect(() => {
  if (connectionOpenInitialized.current && config?.isConfigured && !credentialsError) {
    setConnectionSectionOpen(false);
  }
}, [config?.isConfigured, credentialsError]);
```

`useRef` is already imported (it is used for `invalidatedJobRef` on line 165). The `useEffect` hook is also already imported. No new imports needed.

- [ ] **Step 2: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/$storefront.tsx
git commit -m "fix(sync): fix connection section open/close on page load and after connect"
```

---

## Task 7: Final quality gates

- [ ] **Step 1: Run all Go tests**

```bash
go test ./... -timeout 600s 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 2: Run all frontend checks**

```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```

Expected: zero TypeScript errors, zero knip findings, all tests pass.

- [ ] **Step 3: Build the frontend**

```bash
cd ui/frontend && npm run build
```

Expected: successful build, no errors.
