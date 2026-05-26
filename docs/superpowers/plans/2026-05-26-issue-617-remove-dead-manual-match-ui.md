# Issue #617: Remove Dead Manual-Match UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the unreachable `ReviewItemWidget` / resolve / skip code from `JobItemsDetails`, its hooks, API functions, backend handlers, routes, and Slumber collection — without losing the PSN sibling-resolve test coverage that already exists in the sync rematch test suite.

**Architecture:** Pure deletion. No new behaviour is introduced. The sync rematch path (`HandleRematchExternalGame`) is a strict superset of the dead `HandleResolveItem` path and is already covered by `TestRematchExternalGame_ResolvesSiblings`, `TestRematchExternalGame_UpdatesSiblingJobItemStatusToPending`, etc. The sibling-resolve logic tested by `TestResolveItem_PropagatesResolutionToSiblings` and friends maps onto existing sync rematch tests, so those backend tests can be deleted. The frontend has no existing tests for `resolveJobItem`/`skipJobItem` — nothing to port there.

**Tech Stack:** Go (Echo v5, Bun), TypeScript (React 19, TanStack Query)

---

## File Map

| File | Action |
|---|---|
| `ui/frontend/src/components/jobs/job-items-details.tsx` | Delete `SearchResultItem` (lines 65–111), `ReviewItemWidget` (lines 113–312), and the `isPendingReviewSection` branch in `StatusSection` (lines 344, 437–446); remove dead imports |
| `ui/frontend/src/hooks/use-jobs.ts` | Delete `useResolveJobItem` (lines 198–211) and `useSkipJobItem` (lines 213–226) |
| `ui/frontend/src/hooks/index.ts` | Remove `useResolveJobItem` and `useSkipJobItem` from re-export |
| `ui/frontend/src/api/jobs.ts` | Delete `resolveJobItem` (lines 373–382) and `skipJobItem` (lines 384–393) |
| `internal/api/job_items.go` | Delete `HandleResolveItem` (lines 52–155) and `HandleSkipItem` (lines 157–208); remove now-unused imports |
| `internal/api/router.go` | Delete `/:id/resolve` and `/:id/skip` routes (lines 279–280) |
| `internal/api/job_items_test.go` | Delete `TestResolveItem`, `TestResolveItem_PropagatesResolutionToSiblings`, `TestResolveItem_EnqueuesStage3NotStage2`, `TestResolveItem_NotPendingReview`, `TestSkipItem`, `TestSkipItem_TerminatesJobWhenLastItem` |
| `slumber.yaml` | Delete `resolve_item` and `skip_item` request blocks |

---

## Task 1: Create feature branch

**Files:** none (git only)

- [ ] **Step 1: Create branch**

```bash
git checkout -b issue-617-remove-dead-manual-match-ui
```

- [ ] **Step 2: Confirm clean state**

```bash
git status
```

Expected: only `internal/services/gog/library.go` modified (pre-existing, unrelated change). If it's not committed, stash it or leave it — it won't conflict with any file this plan touches.

---

## Task 2: Verify sibling-resolve test coverage exists in sync rematch suite

Before deleting backend tests, confirm the sync rematch suite already covers the behaviour.

**Files:** `internal/api/sync_test.go` (read-only)

- [ ] **Step 1: Run the sync rematch sibling tests**

```bash
go test ./internal/api/... -run "TestRematchExternalGame_ResolvesSiblings|TestRematchExternalGame_UpdatesSiblingJobItemStatusToPending" -v
```

Expected: both tests PASS. If either fails, stop and investigate before continuing.

- [ ] **Step 2: Confirm the three resolve-handler scenarios have rematch equivalents**

The dead resolve handler tests cover:
1. Basic resolve + DB state → `TestRematchExternalGame_UpdatesJobItemStatusToPending` covers this.
2. Sibling propagation (PPSA/CUSA) → `TestRematchExternalGame_ResolvesSiblings` and `TestRematchExternalGame_UpdatesSiblingJobItemStatusToPending` cover this.
3. Stage-3 enqueue (not Stage-2) → `TestRematchExternalGame_UpdatesJobItemStatusToPending` covers the status transition; River enqueue is exercised by the real handler call.

The skip-handler test `TestSkipItem_TerminatesJobWhenLastItem` has no equivalent in the sync suite, but its logic lives in `tasks.SyncCheckJobCompletion` which is tested elsewhere. Confirm:

```bash
go test ./internal/... -run "TestSyncCheckJobCompletion|TestSkipItem_TerminatesJob" -v 2>&1 | head -30
```

Note: this may print "no tests to run" for `TestSyncCheckJobCompletion` — that's fine; the skip-termination path is safe to drop since the same function is exercised by the live sync skip path (via `HandleSkipExternalGame`).

---

## Task 3: Delete backend handlers and routes

**Files:**
- Modify: `internal/api/job_items.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Delete `HandleResolveItem` from `job_items.go`**

Remove lines 52–155 (the entire `HandleResolveItem` function). The file should go from 254 lines to ~155 lines after this and the next deletion.

After deletion, `job_items.go` imports should be audited. The handlers still use `context`, `database/sql`, `errors`, `log/slog`, `net/http`, `time`, `pgx/v5`, `echo/v5`, `river`, `bun`, `auth`, `models`, `tasks`. After removing `HandleResolveItem` and `HandleSkipItem`, check which are still needed:

- `time` — used by `HandleResolveItem` only (`now := time.Now().UTC()`). Remove after deletion.
- `log/slog` — used by `HandleResolveItem` (`slog.Error`) and `HandleSkipItem` (`slog.Error`). Remove after both deletions.
- `github.com/riverqueue/river` and `tasks` — still used by `HandleRetryItem`. Keep.

- [ ] **Step 2: Delete `HandleSkipItem` from `job_items.go`**

Remove lines 157–208 (the entire `HandleSkipItem` function, including its comment header).

- [ ] **Step 3: Clean up unused imports in `job_items.go`**

After both deletions, the imports block should be:

```go
import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)
```

- [ ] **Step 4: Delete resolve and skip routes from `router.go`**

Remove lines 279–280:
```go
jobItemsGroup.POST("/:id/resolve", jih.HandleResolveItem)
jobItemsGroup.POST("/:id/skip", jih.HandleSkipItem)
```

- [ ] **Step 5: Build to confirm no compile errors**

```bash
make build
```

Expected: exits 0, binary produced.

- [ ] **Step 6: Run Go tests**

```bash
go test ./internal/api/... -v -run "TestGetJobItem|TestRetryItem" 2>&1 | tail -20
```

Expected: `TestGetJobItem`, `TestGetJobItem_WrongOwner`, `TestRetryItem`, `TestRetryItem_NotFailed` all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/job_items.go internal/api/router.go
git commit -m "fix: remove HandleResolveItem and HandleSkipItem backend handlers"
```

---

## Task 4: Delete backend tests for resolve and skip

**Files:**
- Modify: `internal/api/job_items_test.go`

- [ ] **Step 1: Delete the six dead test functions**

Delete these complete test functions (with their comment headers and blank lines):
- `TestResolveItem` (lines 62–94)
- `TestResolveItem_PropagatesResolutionToSiblings` (lines 96–181)
- `TestResolveItem_EnqueuesStage3NotStage2` (lines 183–253)
- `TestResolveItem_NotPendingReview` (lines 255–271)
- `TestSkipItem` (lines 273–300)
- `TestSkipItem_TerminatesJobWhenLastItem` (lines 331–373)

Keep: `TestGetJobItem`, `TestGetJobItem_WrongOwner`, `TestRetryItem`, `TestRetryItem_NotFailed`.

- [ ] **Step 2: Run Go tests to confirm survivors pass**

```bash
go test ./internal/api/... -v -run "TestGetJobItem|TestRetryItem" 2>&1 | tail -20
```

Expected: all 4 remaining job-item tests PASS.

- [ ] **Step 3: Run full Go test suite**

```bash
go test -timeout 600s ./... 2>&1 | tail -30
```

Expected: `ok` for all packages, no failures.

- [ ] **Step 4: Commit**

```bash
git add internal/api/job_items_test.go
git commit -m "test: remove dead resolve/skip test cases from job_items_test.go"
```

---

## Task 5: Delete frontend API functions

**Files:**
- Modify: `ui/frontend/src/api/jobs.ts`

- [ ] **Step 1: Delete `resolveJobItem` and `skipJobItem` from `jobs.ts`**

Delete lines 373–393 (both functions and their JSDoc comments):

```ts
/**
 * Resolve a job item to an IGDB game.
 */
export async function resolveJobItem(itemId: string, igdbId: number): Promise<JobItemDetail> {
  ...
}

/**
 * Skip a job item without matching.
 */
export async function skipJobItem(itemId: string, reason?: string): Promise<JobItemDetail> {
  ...
}
```

- [ ] **Step 2: Run type check**

```bash
cd ui/frontend && npm run check 2>&1 | tail -20
```

Expected: 0 errors (the hooks and component are still referencing them — those will be fixed in subsequent tasks).

Note: type errors will appear here because `use-jobs.ts` still imports these functions. That's expected at this stage — they'll resolve in Task 6.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/api/jobs.ts
git commit -m "fix: remove resolveJobItem and skipJobItem from jobs API"
```

---

## Task 6: Delete frontend hooks

**Files:**
- Modify: `ui/frontend/src/hooks/use-jobs.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Delete `useResolveJobItem` from `use-jobs.ts`**

Delete lines 198–211 (the function and its JSDoc comment):

```ts
/**
 * Hook to resolve a job item to an IGDB ID.
 */
export function useResolveJobItem() {
  ...
}
```

- [ ] **Step 2: Delete `useSkipJobItem` from `use-jobs.ts`**

Delete lines 213–226 (the function and its JSDoc comment):

```ts
/**
 * Hook to skip a job item.
 */
export function useSkipJobItem() {
  ...
}
```

- [ ] **Step 3: Remove re-exports from `index.ts`**

In `ui/frontend/src/hooks/index.ts`, remove `useResolveJobItem` and `useSkipJobItem` from the `use-jobs` re-export block (lines 99–100):

```ts
// Before:
  useResolveJobItem,
  useSkipJobItem,
// After: these two lines are gone
```

- [ ] **Step 4: Type check**

```bash
cd ui/frontend && npm run check 2>&1 | tail -20
```

Expected: errors only from `job-items-details.tsx` (it still imports the deleted hooks). This is expected — Task 7 fixes it.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/hooks/use-jobs.ts ui/frontend/src/hooks/index.ts
git commit -m "fix: remove useResolveJobItem and useSkipJobItem hooks"
```

---

## Task 7: Gut the dead UI in `job-items-details.tsx`

**Files:**
- Modify: `ui/frontend/src/components/jobs/job-items-details.tsx`

This is the largest single edit. Work top-to-bottom through the file.

- [ ] **Step 1: Remove dead imports (lines 1–41)**

The cleaned import block should be:

```ts
import { useState, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  ChevronDown,
  ChevronRight,
  AlertCircle,
  CheckCircle,
  Clock,
  Loader2,
  RotateCcw,
} from 'lucide-react';
import { Link } from '@tanstack/react-router';
import {
  useJobItems,
  useRetryFailedItems,
  useRetryJobItem,
} from '@/hooks';
import { JobItemStatus, getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';
import type { JobItem } from '@/types';
```

Removed from imports:
- `useCallback` (only used in `ReviewItemWidget`)
- `Input` (only used in `ReviewItemWidget`)
- `Dialog`, `DialogContent`, `DialogDescription`, `DialogHeader`, `DialogTitle` (only used in `ReviewItemWidget`)
- `Search`, `SkipForward`, `ImageOff` (only used in `ReviewItemWidget` / `SearchResultItem`)
- `useResolveJobItem`, `useSkipJobItem`, `useSearchIGDB` (deleted hooks)
- `IGDBGameCandidate` type (only used by `SearchResultItem` / `ReviewItemWidget`)

- [ ] **Step 2: Delete `SearchResultItem` component (lines 64–111)**

Remove the entire `SearchResultItem` function and its comment header.

- [ ] **Step 3: Delete `ReviewItemWidget` component (lines 113–312)**

Remove the entire `ReviewItemWidget` function and its comment header.

- [ ] **Step 4: Clean `StatusSection` — remove dead state and the `isPendingReviewSection` render branch**

In `StatusSection`, make these targeted removals:

4a. Remove `processingItemId` state and the `isPendingReviewSection` variable:

```ts
// Delete this line:
const [processingItemId, setProcessingItemId] = useState<string | null>(null);
```

```ts
// Delete this line:
const isPendingReviewSection = status === JobItemStatus.PENDING_REVIEW;
```

4b. Replace the map callback that branches on `isPendingReviewSection` with the non-review branch only:

Before (lines 436–486):
```ts
{data?.items.map((item) =>
  isPendingReviewSection ? (
    <ReviewItemWidget
      key={item.id}
      item={item}
      isProcessing={processingItemId === item.id}
      onProcessingChange={(processing) =>
        setProcessingItemId(processing ? item.id : null)
      }
    />
  ) : (
    <div
      key={item.id}
      className="flex items-start justify-between rounded-md border p-3 text-sm"
    >
      ...
    </div>
  )
)}
```

After:
```ts
{data?.items.map((item) => (
  <div
    key={item.id}
    className="flex items-start justify-between rounded-md border p-3 text-sm"
  >
    <div className="min-w-0 flex-1">
      {item.resultUserGameId ? (
        <Link
          to="/games/$id" params={{ id: String(item.resultUserGameId) }}
          className="font-medium truncate hover:underline text-primary block"
        >
          {item.resultGameTitle || item.sourceTitle}
        </Link>
      ) : (
        <div className="font-medium truncate">{item.resultGameTitle || item.sourceTitle}</div>
      )}
      {item.errorMessage && (
        <div className="text-xs mt-1 text-red-600">
          {item.errorMessage}
        </div>
      )}
    </div>
    {canRetryItem && (
      <Button
        variant="ghost"
        size="sm"
        onClick={() => handleRetryItem(item.id)}
        disabled={retryItemMutation.isPending}
        className="ml-2 h-8"
        title="Retry"
      >
        {retryItemMutation.isPending ? (
          <Loader2 className="h-3 w-3 animate-spin" />
        ) : (
          <RotateCcw className="h-3 w-3" />
        )}
      </Button>
    )}
  </div>
))}
```

4c. The `JobItem` type is still referenced by `useJobItems` return type — keep the import. Remove `IGDBGameCandidate` which was only used by `SearchResultItem`.

- [ ] **Step 5: Type-check and knip**

```bash
cd ui/frontend && npm run check 2>&1 | tail -30 && npm run knip 2>&1 | tail -20
```

Expected: 0 type errors, 0 knip findings. If knip flags any shadcn/ui components that are now orphaned (e.g. `Dialog`, `Input`), remove them from the import lines above (they should already be gone from Step 1).

- [ ] **Step 6: Run frontend tests**

```bash
cd ui/frontend && npm run test 2>&1 | tail -30
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/jobs/job-items-details.tsx
git commit -m "fix: remove dead ReviewItemWidget and SearchResultItem from JobItemsDetails"
```

---

## Task 8: Delete Slumber entries

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Delete `resolve_item` and `skip_item` blocks**

In `slumber.yaml`, delete lines 289–307 (the `resolve_item` and `skip_item` request definitions including their trailing blank line):

```yaml
      resolve_item:
        name: Resolve Item
        method: POST
        url: "{{base_url}}/api/job-items/{{prompt(message='Job Item ID')}}/resolve"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            igdb_id: "{{prompt(message='IGDB ID to resolve to') | json_parse()}}"

      skip_item:
        name: Skip Item
        method: POST
        url: "{{base_url}}/api/job-items/{{prompt(message='Job Item ID')}}/skip"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            reason: "{{prompt(message='Skip reason')}}"
```

- [ ] **Step 2: Verify collection loads**

```bash
slumber collection 2>&1 | head -10
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "chore: remove resolve_item and skip_item from slumber collection"
```

---

## Task 9: Final acceptance check

- [ ] **Step 1: Grep for dead symbols**

```bash
grep -r "ReviewItemWidget\|SearchResultItem\|useResolveJobItem\|useSkipJobItem\|resolveJobItem\|skipJobItem\|HandleResolveItem\|HandleSkipItem" \
  --include="*.go" --include="*.ts" --include="*.tsx" .
```

Expected: zero matches.

- [ ] **Step 2: Full Go test suite**

```bash
go test -timeout 600s ./... 2>&1 | tail -20
```

Expected: `ok` for all packages.

- [ ] **Step 3: Full frontend check**

```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```

Expected: 0 errors, 0 knip findings, all tests pass.

- [ ] **Step 4: Lint**

```bash
golangci-lint run 2>&1 | tail -20
```

Expected: no issues.
