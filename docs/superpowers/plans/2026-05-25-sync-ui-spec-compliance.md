# Sync UI Spec Compliance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close five UI spec gaps (G-F1 through G-F5) identified in `docs/superpowers/specs/2026-05-25-sync-ui-spec-compliance-design.md`, including full removal of `auto_add` from DB, backend, and frontend.

**Architecture:** Tasks are ordered smallest-to-largest blast radius. G-F5 and G-F3 are pure frontend display fixes. G-F1 strips overloaded controls from the hub card. G-F2 removes `auto_add` from eight files across the stack and moves the frequency selector into the Connection & Settings collapsible. G-F4 replaces the raw per-game processing trace in Sync History with a human-readable changelog + progress counts.

**Tech Stack:** Go 1.25 (Bun ORM, Echo v5), React 19 + TypeScript (TanStack Query, shadcn/ui, Tailwind v4), Vitest.

---

## Task 1: G-F5 — Fix External Games section order

**Files:**
- Modify: `ui/frontend/src/components/sync/external-games-section.tsx`

Current render order: Needs Review → Failed → **Skipped → Matched**.
Spec order: Needs Review → Failed → **Matched → Skipped**.

- [ ] **Step 1: Swap Matched and Skipped collapsible blocks**

In `external-games-section.tsx`, move the `{skipped.length > 0 && (...)}` block (lines 175–209) to appear **after** the `{matched.length > 0 && (...)}` block (lines 211–253).

Result — the four sections appear in this order:
```tsx
{/* 1. Needs Review */}
{needsReview.length > 0 && (
  <Card>...</Card>
)}

{/* 2. Failed */}
{failed.length > 0 && (
  <Card>...</Card>
)}

{/* 3. Matched — was last, now third */}
{matched.length > 0 && (
  <Collapsible open={matchedOpen} onOpenChange={setMatchedOpen}>
    <Card>
      <CardHeader className="py-3">
        <CollapsibleTrigger className="flex w-full items-center justify-between">
          <CardTitle className="text-base">Matched ({matched.length})</CardTitle>
          <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', matchedOpen && 'rotate-180')} />
        </CollapsibleTrigger>
      </CardHeader>
      <CollapsibleContent>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Storefront Title</TableHead>
                <TableHead>IGDB Title</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {matched.map((game) => (
                <TableRow key={game.id}>
                  <TableCell>{game.title}</TableCell>
                  <TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setMatchingGame(game)}
                      disabled={isRematching}
                    >
                      Change Match
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </CollapsibleContent>
    </Card>
  </Collapsible>
)}

{/* 4. Skipped — was third, now last */}
{skipped.length > 0 && (
  <Collapsible open={skippedOpen} onOpenChange={setSkippedOpen}>
    <Card>
      <CardHeader className="py-3">
        <CollapsibleTrigger className="flex w-full items-center justify-between">
          <CardTitle className="text-base">Skipped ({skipped.length})</CardTitle>
          <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', skippedOpen && 'rotate-180')} />
        </CollapsibleTrigger>
      </CardHeader>
      <CollapsibleContent>
        <CardContent className="p-0">
          <Table>
            <TableBody>
              {skipped.map((game) => (
                <TableRow key={game.id}>
                  <TableCell>{game.title}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => unskip(game.id)}
                      disabled={isUnskipping}
                    >
                      Unskip
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </CollapsibleContent>
    </Card>
  </Collapsible>
)}
```

- [ ] **Step 2: Type-check and lint**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip
```
Expected: zero errors and zero knip findings.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/sync/external-games-section.tsx
git commit -m "fix(ui): display Matched before Skipped in external games section (G-F5)"
```

---

## Task 2: G-F3 — Fix progress box counts

**Files:**
- Modify: `ui/frontend/src/components/jobs/job-progress-card.tsx`

Current grid: Completed / Failed / Processing / Pending / IGDB Error (conditional).
Spec grid: **Matched** / **Needs Review** / **Skipped** / Failed / **Processing** (pending+processing).
`igdb_failed` is not a valid job item status and must be removed from the UI entirely, including the "Retry IGDB errors" button.

- [ ] **Step 1: Rewrite `job-progress-card.tsx`**

Replace the entire file with:

```tsx
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Loader2, XCircle } from 'lucide-react';
import { useState } from 'react';
import type { Job } from '@/types';
import {
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  isJobInProgress,
} from '@/types';

interface JobProgressCardProps {
  job: Job;
  onCancel: () => Promise<void>;
  isCancelling?: boolean;
}

export function JobProgressCard({ job, onCancel, isCancelling }: JobProgressCardProps) {
  const [confirmCancel, setConfirmCancel] = useState(false);
  const showProgress = isJobInProgress(job);

  const handleCancel = async () => {
    await onCancel();
    setConfirmCancel(false);
  };

  return (
    <>
      <Card>
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">
              {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
            </CardTitle>
            <Badge variant={getJobStatusVariant(job.status)}>
              {isJobInProgress(job) && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
              {getJobStatusLabel(job.status)}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {showProgress && job.progress && (
            <div>
              <div className="mb-2 flex justify-between text-sm text-muted-foreground">
                <span>Progress</span>
                <span>
                  {job.progress.completed + job.progress.failed + job.progress.skipped} /{' '}
                  {job.progress.total} ({job.progress.percent}%)
                </span>
              </div>
              <Progress value={job.progress.percent} />
            </div>
          )}

          {job.progress && (
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-5">
              <div>
                <div className="text-muted-foreground">Matched</div>
                <div className="text-lg font-semibold text-green-600">{job.progress.completed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Needs Review</div>
                <div className="text-lg font-semibold text-yellow-600">{job.progress.pendingReview}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Skipped</div>
                <div className="text-lg font-semibold">{job.progress.skipped}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Failed</div>
                <div className="text-lg font-semibold text-red-600">{job.progress.failed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Processing</div>
                <div className="text-lg font-semibold">{job.progress.pending + job.progress.processing}</div>
              </div>
            </div>
          )}

          {job.errorMessage && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {job.errorMessage}
            </div>
          )}

          {!job.isTerminal && (
            <div className="flex justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setConfirmCancel(true)}
                disabled={isCancelling}
                className="text-amber-600 hover:bg-amber-50 hover:text-amber-700"
              >
                {isCancelling ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <XCircle className="mr-2 h-4 w-4" />
                )}
                Cancel
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <AlertDialog open={confirmCancel} onOpenChange={setConfirmCancel}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel Job</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to cancel this job? This will stop processing and remove the
              job.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep Running</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleCancel}
              disabled={isCancelling}
              className="bg-amber-600 hover:bg-amber-700"
            >
              {isCancelling && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Cancel Job
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
```

- [ ] **Step 2: Type-check and lint**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip
```
Expected: zero errors and zero knip findings. If TypeScript errors about `onRetry`/`isRetrying` props passed from callers, fix those callers now (see step 3).

- [ ] **Step 3: Remove `onRetry`/`isRetrying` from the call site in `$storefront.tsx`**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`:

Remove the `isRetrying` state declaration:
```tsx
// DELETE:
const [isRetrying, setIsRetrying] = useState(false);
```

Remove `handleRetryIGDBErrors` entirely:
```tsx
// DELETE the entire function:
const handleRetryIGDBErrors = async () => {
  if (!activeJob) return;
  setIsRetrying(true);
  try {
    await retryFailedItems(activeJob.id);
    await queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    toast.success('IGDB errors re-queued for retry');
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to retry IGDB errors';
    toast.error(message);
  } finally {
    setIsRetrying(false);
  }
};
```

Remove `onRetry` and `isRetrying` props from the `JobProgressCard` call:
```tsx
<JobProgressCard
  job={activeJob}
  onCancel={handleCancelJob}
  isCancelling={isCancelling}
/>
```

Remove `retryFailedItems` from the import at the top of the file:
```tsx
// BEFORE:
import { retryFailedItems } from '@/api/jobs';
// AFTER: delete this import entirely (retryFailedItems is no longer used here)
```

- [ ] **Step 4: Remove `igdbFailed` from the `JobProgress` type**

In `ui/frontend/src/types/jobs.ts`, remove `igdbFailed: number;` from `JobProgress`:
```typescript
export interface JobProgress {
  pending: number;
  processing: number;
  completed: number;
  pendingReview: number;
  skipped: number;
  failed: number;
  total: number;
  percent: number;
}
```

Also remove `IGDB_FAILED` from `JobItemStatus` enum and its entries in `getJobItemStatusLabel` / `getJobItemStatusVariant`:
```typescript
export enum JobItemStatus {
  PENDING = 'pending',
  PROCESSING = 'processing',
  COMPLETED = 'completed',
  PENDING_REVIEW = 'pending_review',
  SKIPPED = 'skipped',
  FAILED = 'failed',
}
```

In `getJobItemStatusLabel`, remove the `[JobItemStatus.IGDB_FAILED]: 'IGDB Error'` entry.
In `getJobItemStatusVariant`, remove the `case JobItemStatus.IGDB_FAILED:` branch.

- [ ] **Step 5: Remove `igdb_failed` from `api/jobs.ts`**

In `ui/frontend/src/api/jobs.ts`, remove `igdb_failed?: number` (or `igdb_failed: number`) from `JobProgressApiResponse`, and remove `igdbFailed: apiProgress.igdb_failed ?? 0` from `transformProgress`:

```typescript
function transformProgress(apiProgress: JobProgressApiResponse): JobProgress {
  return {
    pending: apiProgress.pending,
    processing: apiProgress.processing,
    completed: apiProgress.completed,
    pendingReview: apiProgress.pending_review,
    skipped: apiProgress.skipped,
    failed: apiProgress.failed,
    total: apiProgress.total,
    percent: apiProgress.percent,
  };
}
```

- [ ] **Step 6: Type-check, knip, and run frontend tests**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip && npm run test
```
Expected: zero errors, zero knip findings, all tests pass. If any component still references `igdbFailed` (e.g. `JobItemsDetails`), trace it and remove the reference.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/jobs/job-progress-card.tsx \
        ui/frontend/src/routes/_authenticated/sync/'$storefront.tsx' \
        ui/frontend/src/types/jobs.ts \
        ui/frontend/src/api/jobs.ts
git commit -m "fix(ui): align progress box with spec; remove igdb_failed from UI (G-F3)"
```

---

## Task 3: G-F1 — Strip overloaded controls from hub cards

**Files:**
- Modify: `ui/frontend/src/components/sync/sync-service-card.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx`

Hub cards currently show: frequency selector, auto-add toggle, reset button.
Spec says hub cards show only: icon, name, status badge, last-synced, pending-review badge, Sync Now button, "View details" link.

- [ ] **Step 1: Rewrite `sync-service-card.tsx`**

Replace the entire file with:

```tsx
import { Card, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { config as envConfig } from '@/lib/env';
import type { SyncConfig, SyncStatus } from '@/types';
import { getStorefrontDisplayInfo } from '@/types';

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  credentialsError?: boolean;
  onTriggerSync: () => Promise<void>;
  isSyncing?: boolean;
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
  credentialsError = false,
  onTriggerSync,
  isSyncing = false,
}: SyncServiceCardProps) {
  const platformInfo = getStorefrontDisplayInfo(config.storefront);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

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
            </div>
          </div>
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Link
                to="/sync/$storefront"
                params={{ storefront: config.storefront }}
                hash="needs-review"
              >
                <Badge variant="destructive">
                  {pendingReviewCount}
                </Badge>
              </Link>
            )}
            {credentialsError ? (
              <Badge variant="destructive">Credentials Error</Badge>
            ) : config.isConfigured ? (
              <Badge
                variant="outline"
                className="bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
              >
                Connected
              </Badge>
            ) : (
              <Badge variant="outline" className="bg-muted text-muted-foreground">
                Not Configured
              </Badge>
            )}
          </div>
        </div>
      </CardHeader>

      <CardFooter className="flex items-center justify-end border-t bg-muted/50 px-6 py-4">
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
      </CardFooter>
    </Card>
  );
}
```

- [ ] **Step 2: Update `sync/index.tsx`**

Remove all references to `onUpdate`, `isUpdating`, `isResetting`, `onReset`, and `useResetSyncData`. Also remove `autoAdd: false` from the placeholder config and update the info alert text.

The updated file:

```tsx
import { createFileRoute, Link } from '@tanstack/react-router';
import { useSyncConfigs, useTriggerSync, useSyncStatus, usePendingReviewCount, useSteamConnection, usePSNStatus, useEpicConnection, useGOGConnection } from '@/hooks';
import { SyncServiceCard } from '@/components/sync';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AlertCircle, Info, ArrowRight } from 'lucide-react';
import { toast } from 'sonner';
import { SUPPORTED_SYNC_STOREFRONTS, SyncStorefront, SyncFrequency } from '@/types';
import type { SyncConfig } from '@/types';

export const Route = createFileRoute('/_authenticated/sync/')({
  component: SyncPage,
});

function SyncPageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <Skeleton className="h-12 w-12 rounded-lg" />
              <div className="flex-1">
                <Skeleton className="mb-2 h-5 w-24" />
                <Skeleton className="h-4 w-32" />
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function SyncServiceCardWithStatus({
  config,
  onTriggerSync,
}: {
  config: SyncConfig;
  onTriggerSync: (storefront: SyncStorefront) => Promise<void>;
}) {
  const { data: status } = useSyncStatus(config.storefront);
  const { data: reviewData } = usePendingReviewCount();
  const { isPending: isSyncing } = useTriggerSync();

  const { data: steamConnection } = useSteamConnection();
  const { data: psnStatus } = usePSNStatus();
  const { data: epicConnection } = useEpicConnection();
  const { data: gogConnection } = useGOGConnection();

  const pendingReviewCount = reviewData?.countsBySource?.[config.storefront] ?? 0;

  const credentialsError =
    (config.storefront === SyncStorefront.STEAM && (steamConnection?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.PSN && (psnStatus?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.EPIC && (epicConnection?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.GOG && (gogConnection?.credentialsError ?? false));

  const handleTriggerSync = async () => {
    await onTriggerSync(config.storefront);
  };

  return (
    <SyncServiceCard
      config={config}
      status={status}
      pendingReviewCount={pendingReviewCount}
      credentialsError={credentialsError}
      onTriggerSync={handleTriggerSync}
      isSyncing={isSyncing}
    />
  );
}

function SyncPage() {
  const { data: configs, isLoading, error } = useSyncConfigs();
  const { mutateAsync: triggerSync } = useTriggerSync();

  const configsByStorefront = new Map<SyncStorefront, SyncConfig>();
  configs?.configs.forEach(config => {
    configsByStorefront.set(config.storefront, config);
  });

  const allStorefrontConfigs = SUPPORTED_SYNC_STOREFRONTS.map(storefront => {
    return configsByStorefront.get(storefront) || {
      id: `placeholder-${storefront}`,
      userId: '',
      storefront,
      frequency: SyncFrequency.MANUAL,
      lastSyncedAt: null,
      createdAt: '',
      updatedAt: '',
      isConfigured: false,
    };
  });

  const handleTriggerSync = async (storefront: SyncStorefront) => {
    try {
      await triggerSync(storefront);
      toast.success(`${storefront} sync started successfully`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to trigger sync';
      toast.error(message);
      throw err;
    }
  };

  return (
    <div>
      <div className="mb-6">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link to="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Sync</span>
        </nav>
        <h1 className="text-2xl font-bold">Sync</h1>
        <p className="text-muted-foreground">
          Sync your Steam, Epic Games, and PlayStation Network libraries with Nexorious.
        </p>
      </div>

      {isLoading && <SyncPageSkeleton />}

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load sync configurations. Please try again later.
          </AlertDescription>
        </Alert>
      )}

      {!isLoading && !error && (
        <>
          <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {allStorefrontConfigs.map((config: SyncConfig) => (
              <SyncServiceCardWithStatus
                key={config.id}
                config={config}
                onTriggerSync={handleTriggerSync}
              />
            ))}
          </div>

          <Alert className="mb-6">
            <Info className="h-4 w-4" />
            <AlertTitle>About Platform Syncing</AlertTitle>
            <AlertDescription>
              <p className="mb-2">
                Connect your gaming platforms to automatically sync your game libraries. New games
                will appear in your collection, and you can review pending items before they&apos;re
                added.
              </p>
              <p>
                Configure sync frequency for each platform individually from the platform detail
                page. Manual sync is always available regardless of your settings.
              </p>
            </AlertDescription>
          </Alert>

          <Card>
            <CardHeader>
              <CardTitle>Quick Links</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <Link
                to="/import-export"
                className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
              >
                <div>
                  <div className="font-medium">Import/Export</div>
                  <div className="text-sm text-muted-foreground">
                    Bulk import or export your collection
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </Link>
              <Link
                to="/games"
                className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
              >
                <div>
                  <div className="font-medium">View Collection</div>
                  <div className="text-sm text-muted-foreground">
                    Browse and manage your game library
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </Link>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Type-check, knip, and run frontend tests**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip && npm run test
```
Expected: zero errors, zero knip findings, all tests pass. Note: `isRetrying` state and `handleRetryIGDBErrors` were already removed in Task 2 step 3.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/sync/sync-service-card.tsx \
        ui/frontend/src/routes/_authenticated/sync/index.tsx
git commit -m "fix(ui): strip frequency/auto-add/reset from hub sync cards (G-F1)"
```

---

## Task 4: G-F2 — Remove `auto_add` everywhere + move frequency into collapsible

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`
- Modify: `internal/db/models/models.go`
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`
- Modify: `internal/scheduler/cleanup_test.go`
- Modify: `ui/frontend/src/types/sync.ts`
- Modify: `ui/frontend/src/api/sync.ts`
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

- [ ] **Step 1: Write the failing Go tests**

Run the current tests to confirm they pass (baseline):
```bash
go test ./internal/api/... -run TestSyncConfig -v 2>&1 | tail -20
go test ./internal/scheduler/... -v 2>&1 | tail -20
```
Expected: all pass. After the changes below they must still pass (the test edits remove `auto_add` from SQL INSERTs and PUT bodies, not from assertions about real behaviour).

- [ ] **Step 2: Remove `auto_add` from the initial migration**

In `internal/db/migrations/20260503000001_initial.up.sql`, delete this line from the `user_sync_configs` CREATE TABLE statement (around line 210):
```sql
    auto_add               BOOLEAN NOT NULL DEFAULT false,
```

The table definition after the change:
```sql
CREATE TABLE user_sync_configs (
    id                     TEXT PRIMARY KEY,
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront             TEXT NOT NULL,
    frequency              TEXT NOT NULL DEFAULT 'manual',
    storefront_credentials TEXT,
    last_synced_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront)
);
```

- [ ] **Step 3: Remove `AutoAdd` from the Go model**

In `internal/db/models/models.go`, delete this line from `UserSyncConfig` (around line 195):
```go
	AutoAdd               bool       `bun:"auto_add,notnull"        json:"auto_add"`
```

- [ ] **Step 4: Remove `auto_add` from `sync.go`**

**4a.** In `syncConfigItem` (around line 96), remove:
```go
	AutoAdd      bool       `json:"auto_add"`
```

**4b.** In `syncConfigResponse` (around line 107), remove:
```go
	AutoAdd      bool       `json:"auto_add"`
```

**4c.** In `HandleListSyncConfigs` (around lines 251–272), remove `AutoAdd: row.AutoAdd,` and `AutoAdd: false,` from both config builder branches:

```go
// When row exists:
configs = append(configs, syncConfigItem{
    ID:           row.ID,
    Storefront:   row.Storefront,
    Frequency:    row.Frequency,
    LastSyncedAt: row.LastSyncedAt,
    IsConfigured: row.StorefrontCredentials != nil,
    CreatedAt:    row.CreatedAt,
    UpdatedAt:    row.UpdatedAt,
})
// When no row (default):
configs = append(configs, syncConfigItem{
    ID:           uuid.NewString(),
    Storefront:   sf,
    Frequency:    "manual",
    LastSyncedAt: nil,
    IsConfigured: false,
    CreatedAt:    now,
    UpdatedAt:    now,
})
```

**4d.** In `HandleGetConfig` (around lines 292–307), remove `AutoAdd: false,` and `AutoAdd: row.AutoAdd,` from both response builders:

```go
// When no row:
return c.JSON(http.StatusOK, syncConfigResponse{
    ID: uuid.NewString(), UserID: userID, Storefront: sf,
    Frequency: "manual", IsConfigured: false,
    CreatedAt: now, UpdatedAt: now,
})
// When row exists:
return c.JSON(http.StatusOK, syncConfigResponse{
    ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
    Frequency: row.Frequency,
    LastSyncedAt: row.LastSyncedAt,
    IsConfigured: row.StorefrontCredentials != nil,
    CreatedAt:    row.CreatedAt, UpdatedAt: row.UpdatedAt,
})
```

**4e.** In `HandleUpdateConfig` (around lines 310–369):

Remove `AutoAdd *bool \`json:"auto_add"\`` from the body struct:
```go
var body struct {
    Frequency *string `json:"frequency"`
}
```

Remove `AutoAdd: false,` from the new-row initialization:
```go
row = models.UserSyncConfig{
    ID:         uuid.NewString(),
    UserID:     userID,
    Storefront: sf,
    Frequency:  "manual",
    CreatedAt:  now,
    UpdatedAt:  now,
}
```

Remove the `if body.AutoAdd != nil` block entirely.

Change the UPSERT conflict clause to remove `auto_add = EXCLUDED.auto_add, `:
```go
_, err = h.db.NewInsert().Model(&row).
    On("CONFLICT (user_id, storefront) DO UPDATE SET frequency = EXCLUDED.frequency, updated_at = EXCLUDED.updated_at").
    Exec(ctx)
```

Remove `AutoAdd: row.AutoAdd,` from the response:
```go
return c.JSON(http.StatusOK, syncConfigResponse{
    ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
    Frequency: row.Frequency,
    LastSyncedAt: row.LastSyncedAt,
    IsConfigured: row.StorefrontCredentials != nil,
    CreatedAt:    row.CreatedAt, UpdatedAt: row.UpdatedAt,
})
```

- [ ] **Step 5: Update `sync_test.go`**

**5a.** `TestSyncConfig_Put_CreatesRow` (around lines 84–106): Remove `"auto_add": true,` from the PUT body and remove the `auto_add` assertion:
```go
rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
    "frequency": "daily",
}, token)
// ... existing assertions ...
if resp["frequency"] != "daily" {
    t.Fatalf("expected frequency=daily, got %v", resp["frequency"])
}
// (delete the auto_add assertion entirely)
```

**5b.** `TestSyncListConfig_AfterPut` (around line 539): Remove `"auto_add": false,` from the PUT body:
```go
rec := putJSONAuth(t, e, "/api/sync/config/steam", map[string]any{
    "frequency": "daily",
}, token)
```

**5c.** `TestSyncGetConfig_AfterPut` (around line 604): Remove `"auto_add": false,` from the PUT body:
```go
rec := putJSONAuth(t, e, "/api/sync/config/psn", map[string]any{
    "frequency": "weekly",
}, token)
```

**5d.** `TestResetSyncData_DeletesDataAndResetsTimestamp` (around line 1086): Update raw INSERT to remove `auto_add`:
```go
_, _ = testDB.ExecContext(context.Background(),
    `INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at, created_at, updated_at)
     VALUES (gen_random_uuid(), ?, 'steam', 'manual', now(), now(), now())`,
    userID,
)
```

- [ ] **Step 6: Update `cleanup_test.go`**

There are five raw INSERTs that include `auto_add`. Change each one by removing the `auto_add` column and `false` value.

The pattern to change (appears five times with slight variations):

Before:
```go
`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at)
 VALUES (?, ?, 'steam', 'daily', false, ?)`,
```
After:
```go
`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at)
 VALUES (?, ?, 'steam', 'daily', ?)`,
```

Before (no last_synced_at):
```go
`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add)
 VALUES (?, ?, 'steam', 'weekly', false)`,
```
After:
```go
`INSERT INTO user_sync_configs (id, user_id, storefront, frequency)
 VALUES (?, ?, 'steam', 'weekly')`,
```

Apply this change to all five INSERTs in the file (search for `auto_add` to find them all).

- [ ] **Step 7: Run Go tests**

```bash
go test ./internal/api/... -run TestSync -v 2>&1 | tail -30
go test ./internal/scheduler/... -v 2>&1 | tail -20
```
Expected: all tests pass. If any fail because of missing `auto_add` column in SQL, you missed an INSERT — grep for `auto_add` in both test files to find remaining occurrences:
```bash
grep -n "auto_add" internal/api/sync_test.go internal/scheduler/cleanup_test.go
```

- [ ] **Step 8: Run full Go test suite**

```bash
go test -timeout 600s ./...
```
Expected: all pass.

- [ ] **Step 9: Remove `autoAdd` from frontend types**

In `ui/frontend/src/types/sync.ts`:

Remove `autoAdd: boolean;` from `SyncConfig`:
```typescript
export interface SyncConfig {
  id: string;
  userId: string;
  storefront: SyncStorefront;
  frequency: SyncFrequency;
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
  isConfigured: boolean;
}
```

Remove `autoAdd?: boolean;` from `SyncConfigUpdateData`:
```typescript
export interface SyncConfigUpdateData {
  frequency?: SyncFrequency;
}
```

- [ ] **Step 10: Remove `auto_add` from `api/sync.ts`**

In `ui/frontend/src/api/sync.ts`:

Remove `auto_add: boolean;` from `SyncConfigApiResponse`:
```typescript
interface SyncConfigApiResponse {
  id: string;
  user_id: string;
  storefront: string;
  frequency: string;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
  is_configured: boolean;
}
```

Remove `autoAdd: apiConfig.auto_add,` from `transformSyncConfig`:
```typescript
function transformSyncConfig(apiConfig: SyncConfigApiResponse): SyncConfig {
  return {
    id: apiConfig.id,
    userId: apiConfig.user_id,
    storefront: apiConfig.storefront as SyncStorefront,
    frequency: apiConfig.frequency as SyncFrequency,
    lastSyncedAt: apiConfig.last_synced_at,
    createdAt: apiConfig.created_at,
    updatedAt: apiConfig.updated_at,
    isConfigured: apiConfig.is_configured,
  };
}
```

Remove the `if (data.autoAdd !== undefined)` block from `updateSyncConfig`:
```typescript
export async function updateSyncConfig(
  platform: SyncStorefront,
  data: SyncConfigUpdateData
): Promise<SyncConfig> {
  const requestBody: Record<string, unknown> = {};

  if (data.frequency !== undefined) {
    requestBody.frequency = data.frequency;
  }

  const response = await api.put<SyncConfigApiResponse>(
    `/sync/config/${platform}`,
    requestBody
  );
  return transformSyncConfig(response);
}
```

- [ ] **Step 11: Rewrite `$storefront.tsx` to remove `auto_add` and move frequency into collapsible**

Make the following changes to `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`:

**11a.** Remove from the imports:
- `Switch` from `@/components/ui/switch`
- `Settings` from the lucide-react import line (keep `RefreshCw, Loader2, AlertCircle, Clock, ChevronDown`)
- `CardDescription` stays (still used in Platform Header)

The lucide-react import becomes:
```tsx
import { RefreshCw, Loader2, AlertCircle, Clock, ChevronDown } from 'lucide-react';
```

**11b.** Remove `localAutoAdd` state and `effectiveAutoAdd`:
```tsx
// Remove this line:
const [localAutoAdd, setLocalAutoAdd] = useState<boolean | null>(null);
// Remove this line:
const effectiveAutoAdd = localAutoAdd ?? config?.autoAdd ?? false;
```

**11c.** Remove `handleAutoAddChange`:
```tsx
// Delete entirely:
const handleAutoAddChange = async (autoAdd: boolean) => {
  setLocalAutoAdd(autoAdd);
  await handleUpdateConfig({ autoAdd });
};
```

**11d.** In `handleUpdateConfig`, remove the `setLocalAutoAdd(null)` error reset line:
```tsx
const handleUpdateConfig = async (data: SyncConfigUpdateData) => {
  try {
    await updateConfig({ storefront, data });
    toast.success('Settings updated');
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to update settings';
    toast.error(message);
    if (data.frequency !== undefined) setLocalFrequency(null);
    // (the autoAdd reset line is gone)
  }
};
```

**11e.** Add `className="space-y-4"` to `CollapsibleContent` and append the frequency row after the connection cards, guarded by `config.isConfigured`:

```tsx
<Collapsible open={connectionSectionOpen} onOpenChange={setConnectionSectionOpen}>
  <CollapsibleContent className="space-y-4">
    {storefront === SyncStorefront.STEAM && (
      <SteamConnectionCard
        isConfigured={config.isConfigured}
        credentialsError={steamConnection?.credentialsError ?? false}
        steamId={steamConnection?.steamId}
        steamUsername={steamConnection?.username}
        onConnectionChange={() => {
          queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
          queryClient.invalidateQueries({ queryKey: syncKeys.steamConnection() });
          queryClient.invalidateQueries({ queryKey: authKeys.me() });
        }}
      />
    )}

    {storefront === SyncStorefront.EPIC && (
      <EpicConnectionCard
        isConfigured={config.isConfigured}
        onConnectionChange={() => {
          queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
          queryClient.invalidateQueries({ queryKey: authKeys.me() });
        }}
      />
    )}

    {storefront === SyncStorefront.GOG && (
      <GOGConnectionCard
        isConfigured={!!config?.isConfigured}
        onConnectionChange={() => {
          queryClient.invalidateQueries({ queryKey: syncKeys.gogConnection() });
        }}
      />
    )}

    {storefront === SyncStorefront.PSN && (
      <PSNConnectionCard
        isConfigured={config.isConfigured}
        credentialsError={psnStatus?.credentialsError ?? false}
        onlineId={psnPrefs?.online_id}
        accountId={psnPrefs?.account_id}
        onConnectionChange={() => {
          queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
          queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
          queryClient.invalidateQueries({ queryKey: authKeys.me() });
        }}
      />
    )}

    {config.isConfigured && (
      <div className="flex items-center justify-between px-1">
        <div>
          <div className="font-medium">Sync Frequency</div>
          <div className="text-sm text-muted-foreground">How often to automatically sync</div>
        </div>
        <Select
          value={effectiveFrequency}
          onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
          disabled={isUpdating}
        >
          <SelectTrigger className="w-[160px]">
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
    )}
  </CollapsibleContent>
</Collapsible>
```

**11f.** Delete the entire "Configuration Section" card (from `{/* Configuration Section */}` comment through the closing `</Card>` of the card that contains the Settings icon header). This removes the standalone frequency select and auto-add switch.

- [ ] **Step 12: Type-check, knip, and run frontend tests**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip && npm run test
```
Expected: zero errors, zero knip findings, all tests pass. If knip complains about unused `SyncConfigUpdateData` import somewhere, trace it and remove the import.

- [ ] **Step 13: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql \
        internal/db/models/models.go \
        internal/api/sync.go \
        internal/api/sync_test.go \
        internal/scheduler/cleanup_test.go \
        ui/frontend/src/types/sync.ts \
        ui/frontend/src/api/sync.ts \
        ui/frontend/src/routes/_authenticated/sync/'$storefront.tsx'
git commit -m "fix: remove auto_add from DB, backend, and frontend; move frequency into collapsible (G-F2)"
```

---

## Task 5: G-F4 — Sync History: replace processing trace with changelog + counts

**Files:**
- Modify: `internal/api/jobs.go`
- Modify: `internal/api/jobs_test.go`
- Modify: `ui/frontend/src/types/jobs.ts`
- Modify: `ui/frontend/src/api/jobs.ts`
- Modify: `ui/frontend/src/components/sync/recent-activity.tsx`

Current: history shows per-game completed/skipped/failed/igdb_failed item lists.
Spec: history shows progress counts one-line summary + `added`/`removed`/`status_changed` sync_changes changelog.

### Backend

- [ ] **Step 1: Write the failing Go test**

In `internal/api/jobs_test.go`, replace the entire `TestRecentJobs_ReturnsSplitItemArrays` function (lines 867–903) with:

```go
func TestRecentJobs_ReturnsProgressAndAddedItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "recent-progress")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k2", "Game B", "failed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k3", "Game C", "skipped")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO sync_changes (id, job_id, user_id, title, change_type, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, 'Game A', 'added', now(), now())`,
		jobID, userID,
	)
	if err != nil {
		t.Fatalf("insert sync_changes: %v", err)
	}

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rawJobs, _ := resp["jobs"].([]any)
	if len(rawJobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(rawJobs))
	}
	job, _ := rawJobs[0].(map[string]any)

	progress, _ := job["progress"].(map[string]any)
	if progress == nil {
		t.Fatal("expected progress object in response")
	}
	if progress["completed"].(float64) != 1 {
		t.Errorf("expected completed=1, got %v", progress["completed"])
	}
	if progress["failed"].(float64) != 1 {
		t.Errorf("expected failed=1, got %v", progress["failed"])
	}
	if progress["skipped"].(float64) != 1 {
		t.Errorf("expected skipped=1, got %v", progress["skipped"])
	}

	addedItems, _ := job["added_items"].([]any)
	if len(addedItems) != 1 {
		t.Errorf("expected 1 added_item, got %d", len(addedItems))
	}
	if len(addedItems) == 1 {
		item, _ := addedItems[0].(map[string]any)
		if item["title"] != "Game A" {
			t.Errorf("expected title=Game A, got %v", item["title"])
		}
	}

	if _, ok := job["completed_items"]; ok {
		t.Error("completed_items should not be present in response")
	}
	if _, ok := job["skipped_items"]; ok {
		t.Error("skipped_items should not be present in response")
	}
	if _, ok := job["failed_items"]; ok {
		t.Error("failed_items should not be present in response")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/api/... -run TestRecentJobs_ReturnsProgressAndAddedItems -v
```
Expected: FAIL — because the response still contains `completed_items` / `skipped_items` / `failed_items` and lacks a `progress` object.

- [ ] **Step 3: Rewrite `HandleRecentJobs` in `jobs.go`**

Replace the `type jobWithItems struct { ... }` definition and the entire loop body (lines 354–431) inside `HandleRecentJobs` with:

```go
type jobWithChanges struct {
    models.Job
    Progress           map[string]any   `json:"progress"`
    AddedItems         []syncChangeItem `json:"added_items"`
    RemovedItems       []syncChangeItem `json:"removed_items"`
    StatusChangedItems []syncChangeItem `json:"status_changed_items"`
}

result := make([]jobWithChanges, 0, len(jobs))
for _, j := range jobs {
    progress, err := h.jobItemCounts(context.Background(), j.ID)
    if err != nil {
        progress = map[string]any{
            "pending": 0, "processing": 0, "completed": 0, "pending_review": 0,
            "skipped": 0, "failed": 0, "total": 0, "percent": 0,
        }
    }

    var allChanges []struct {
        ChangeType string  `bun:"change_type"`
        Title      string  `bun:"title"`
        OldStatus  *string `bun:"old_status"`
        NewStatus  *string `bun:"new_status"`
    }
    if err := h.db.NewRaw(`
        SELECT change_type, title, old_status, new_status
        FROM sync_changes
        WHERE job_id = ?
        ORDER BY created_at`,
        j.ID,
    ).Scan(context.Background(), &allChanges); err != nil {
        allChanges = nil
    }

    addedItems := []syncChangeItem{}
    removedItems := []syncChangeItem{}
    statusChangedItems := []syncChangeItem{}
    for _, sc := range allChanges {
        switch sc.ChangeType {
        case "added":
            addedItems = append(addedItems, syncChangeItem{Title: sc.Title})
        case "removed":
            removedItems = append(removedItems, syncChangeItem{Title: sc.Title})
        case "status_changed":
            statusChangedItems = append(statusChangedItems, syncChangeItem{
                Title: sc.Title, OldStatus: sc.OldStatus, NewStatus: sc.NewStatus,
            })
        }
    }

    result = append(result, jobWithChanges{
        Job:                j,
        Progress:           progress,
        AddedItems:         addedItems,
        RemovedItems:       removedItems,
        StatusChangedItems: statusChangedItems,
    })
}
```

Also delete the now-unused `recentJobItem` struct (the `type recentJobItem struct { ... }` block, lines 307–314) since it is no longer referenced.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/api/... -run TestRecentJobs_ReturnsProgressAndAddedItems -v
```
Expected: PASS.

- [ ] **Step 5: Run the full Go test suite**

```bash
go test -timeout 600s ./...
```
Expected: all pass.

### Frontend

- [ ] **Step 6: Update `types/jobs.ts`**

In `ui/frontend/src/types/jobs.ts`, replace the `RecentJobDetail` interface (lines 175–191). Note: `JobProgress.igdbFailed` was already removed in Task 2 step 4.

```typescript
export interface RecentJobDetail {
  id: string;
  status: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  addedItems: SyncChangeItem[];
  removedItems: SyncChangeItem[];
  statusChangedItems: SyncChangeItem[];
}
```

Also delete the `JobItemSummary` interface — it will no longer be used:
```typescript
// DELETE this entire interface:
export interface JobItemSummary {
  sourceTitle: string;
  resultGameTitle: string | null;
  resultIgdbId: number | null;
  resultUserGameId: string | null;
  errorMessage: string | null;
  isNewAddition: boolean;
}
```

- [ ] **Step 7: Update `api/jobs.ts`**

**7a.** Delete the `JobItemSummaryApiResponse` interface (only used by the per-game item arrays being removed). Search for `interface JobItemSummaryApiResponse` and delete it.

**7b.** Replace `RecentJobDetailApiResponse` with (note: no `igdb_failed` in progress since that status is gone):
```typescript
interface RecentJobDetailApiResponse {
  id: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  progress: {
    completed: number;
    skipped: number;
    failed: number;
    pending: number;
    processing: number;
    pending_review: number;
    total: number;
    percent: number;
  };
  added_items?: SyncChangeItemApiResponse[];
  removed_items?: SyncChangeItemApiResponse[];
  status_changed_items?: SyncChangeItemApiResponse[];
}
```

**7c.** Delete `transformJobItemSummary` (no longer called):
```typescript
// DELETE this entire function:
function transformJobItemSummary(api: JobItemSummaryApiResponse): JobItemSummary {
  return { ... };
}
```

**7d.** Replace `transformRecentJob` with:
```typescript
function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  const p = api.progress ?? { completed: 0, skipped: 0, failed: 0, pending: 0, processing: 0, pending_review: 0, total: 0, percent: 0 };
  return {
    id: api.id,
    status: api.status,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    totalItems: api.total_items,
    completedCount: p.completed,
    skippedCount: p.skipped,
    failedCount: p.failed,
    addedItems: (api.added_items ?? []).map(transformSyncChangeItem),
    removedItems: (api.removed_items ?? []).map(transformSyncChangeItem),
    statusChangedItems: (api.status_changed_items ?? []).map(transformSyncChangeItem),
  };
}
```

- [ ] **Step 8: Rewrite `recent-activity.tsx`**

Replace the entire file with:

```tsx
import { useState } from 'react';
import type { ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Clock,
  ChevronDown,
  ChevronRight,
  CheckCircle,
  XCircle,
  ArrowRight,
} from 'lucide-react';
import { useRecentJobs } from '@/hooks';
import type { RecentJobDetail, SyncChangeItem } from '@/types';

interface RecentActivityProps {
  platform: string;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function SyncChangeList({
  items,
  label,
  icon,
}: {
  items: SyncChangeItem[];
  label: string;
  icon: ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(false);
  if (items.length === 0) return null;
  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between h-8 px-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            {icon}
            <span className="text-sm">{label}</span>
          </div>
          <Badge variant="secondary" className="h-5 text-xs">{items.length}</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-6 pl-2 border-l space-y-1 py-1">
          {items.map((item, idx) => (
            <div key={idx} className="text-sm py-1 text-muted-foreground">
              {item.title}
              {item.oldStatus && item.newStatus && (
                <span className="ml-2 text-xs">
                  {item.oldStatus} → {item.newStatus}
                </span>
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function formatSummary(job: RecentJobDetail): string {
  const parts: string[] = [];
  if (job.completedCount > 0) parts.push(`${job.completedCount} matched`);
  if (job.skippedCount > 0) parts.push(`${job.skippedCount} skipped`);
  if (job.failedCount > 0) parts.push(`${job.failedCount} failed`);
  return parts.join(' · ');
}

function JobCard({ job }: { job: RecentJobDetail }) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-3 h-auto">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <span>{job.completedAt ? formatDate(job.completedAt) : 'In progress'}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{formatSummary(job)}</span>
            <Badge
              variant={job.status === 'completed' ? 'outline' : 'destructive'}
              className={
                job.status === 'completed'
                  ? 'h-5 text-xs bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                  : 'h-5 text-xs'
              }
            >
              {job.status === 'completed' ? 'Completed' : 'Failed'}
            </Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-4 pb-4 space-y-1">
          <SyncChangeList
            items={job.addedItems}
            label="Added to library"
            icon={<CheckCircle className="h-4 w-4 text-green-600" />}
          />
          <SyncChangeList
            items={job.removedItems}
            label="Removed from storefront"
            icon={<XCircle className="h-4 w-4 text-muted-foreground" />}
          />
          <SyncChangeList
            items={job.statusChangedItems}
            label="Status changed"
            icon={<ArrowRight className="h-4 w-4 text-blue-500" />}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function RecentActivity({ platform }: RecentActivityProps) {
  const { data, isLoading, error } = useRecentJobs(platform, 5);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Clock className="h-5 w-5" />
          Recent Activity
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12" />
            <Skeleton className="h-12" />
          </div>
        ) : error ? (
          <div className="text-center py-8 text-muted-foreground">
            Failed to load recent activity
          </div>
        ) : !data || data.jobs.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 text-center text-muted-foreground">
            <Clock className="h-12 w-12 mb-4 opacity-50" />
            <p>No sync history yet</p>
            <p className="text-sm mt-1">Start your first sync to see activity here</p>
          </div>
        ) : (
          <div className="divide-y">
            {data.jobs.map((job) => (
              <JobCard key={job.id} job={job} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 9: Type-check, knip, and run frontend tests**

Run from `ui/frontend/`:
```bash
npm run check && npm run knip && npm run test
```
Expected: zero errors, zero knip findings, all tests pass.

If knip flags `JobItemSummary` as still exported from `types/jobs.ts`, verify step 6 removed the interface. If knip flags `transformJobItemSummary` as dead code in `api/jobs.ts`, verify step 7c deleted the function.

- [ ] **Step 10: Commit**

```bash
git add internal/api/jobs.go \
        internal/api/jobs_test.go \
        ui/frontend/src/types/jobs.ts \
        ui/frontend/src/api/jobs.ts \
        ui/frontend/src/components/sync/recent-activity.tsx
git commit -m "fix: replace per-game history trace with changelog and progress counts (G-F4)"
```

---

## Final verification

- [ ] **Run complete test suites**

```bash
go test -timeout 600s ./...
```
```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```
Expected: all pass with zero errors.
