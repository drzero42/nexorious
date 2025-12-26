# Fix Review API 404s - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate frontend from dead `/api/review/*` endpoints to new `/api/job-items/*` and `/api/jobs/*` endpoints, fixing 404 errors in the sync workflow.

**Architecture:** The backend review API was deleted and replaced with job-items API. Frontend needs to be updated to call the new endpoints. One new backend endpoint is needed for the nav badge pending review count.

**Tech Stack:** FastAPI (Python), Next.js, React Query, TypeScript

---

## Task 1: Backend - Add pending review count endpoint

**Files:**
- Modify: `backend/app/api/jobs.py`
- Create: `backend/app/tests/test_jobs_api.py` (add tests)

**Step 1: Write the failing test**

Add to `backend/app/tests/test_jobs_api.py`:

```python
class TestPendingReviewCount:
    """Tests for GET /api/jobs/pending-review-count endpoint."""

    def test_pending_review_count_empty(self, client, auth_headers, test_user: User):
        """Test pending review count when user has no items."""
        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 0}

    def test_pending_review_count_with_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test pending review count with items needing review."""
        # Create a job with items
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        # Add items with different statuses
        for i, status in enumerate([
            JobItemStatus.PENDING_REVIEW,
            JobItemStatus.PENDING_REVIEW,
            JobItemStatus.COMPLETED,
            JobItemStatus.FAILED,
        ]):
            item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game_{i}",
                source_title=f"Game {i}",
                status=status,
            )
            session.add(item)
        session.commit()

        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 2}

    def test_pending_review_count_excludes_other_users(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that pending review count only includes current user's items."""
        # Create another user
        other_user = User(
            username="other_user",
            hashed_password="hash",
            role="user",
        )
        session.add(other_user)
        session.commit()

        # Create job for other user
        other_job = Job(
            user_id=other_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(other_job)
        session.commit()

        # Add pending review item for other user
        other_item = JobItem(
            job_id=other_job.id,
            user_id=other_user.id,
            item_key="other_game",
            source_title="Other Game",
            status=JobItemStatus.PENDING_REVIEW,
        )
        session.add(other_item)
        session.commit()

        # Current user should see 0
        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 0}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs_api.py::TestPendingReviewCount -v`

Expected: FAIL with "404 Not Found" (endpoint doesn't exist yet)

**Step 3: Write minimal implementation**

Add to `backend/app/api/jobs.py` (after the imports, add the schema):

```python
from pydantic import BaseModel

class PendingReviewCountResponse(BaseModel):
    """Response for pending review count endpoint."""
    pending_review_count: int
```

Add the endpoint (IMPORTANT: must be placed BEFORE `/{job_id}` route to avoid path conflicts):

```python
@router.get("/pending-review-count", response_model=PendingReviewCountResponse)
async def get_pending_review_count(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> PendingReviewCountResponse:
    """
    Get total count of items needing review across all jobs.

    Used by the frontend navigation to show a badge with pending items.
    """
    count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(JobItem.user_id == current_user.id)
        .where(JobItem.status == JobItemStatus.PENDING_REVIEW)
    ).one()

    return PendingReviewCountResponse(pending_review_count=count)
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs_api.py::TestPendingReviewCount -v`

Expected: PASS

**Step 5: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`

Expected: All tests pass

**Step 6: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/test_jobs_api.py
git commit -m "feat(backend): add pending review count endpoint for nav badge"
```

---

## Task 2: Frontend - Add job item API functions

**Files:**
- Modify: `frontend/src/api/jobs.ts`
- Modify: `frontend/src/types/jobs.ts`

**Step 1: Add types to `frontend/src/types/jobs.ts`**

Add at the end of the interfaces section:

```typescript
export interface PendingReviewCountResponse {
  pendingReviewCount: number;
}

export interface JobItemDetail extends JobItem {
  sourceMetadataJson: string;
  resultJson: string;
  igdbCandidatesJson: string;
  resolvedIgdbId: number | null;
  resolvedAt: string | null;
}
```

**Step 2: Add API response types to `frontend/src/api/jobs.ts`**

Add after the existing API response interfaces:

```typescript
interface PendingReviewCountApiResponse {
  pending_review_count: number;
}

interface JobItemDetailApiResponse extends JobItemApiResponse {
  source_metadata_json: string;
  result_json: string;
  igdb_candidates_json: string;
  resolved_igdb_id: number | null;
  resolved_at: string | null;
}
```

**Step 3: Add transformation function**

Add after `transformJobItem`:

```typescript
function transformJobItemDetail(apiItem: JobItemDetailApiResponse): JobItemDetail {
  return {
    ...transformJobItem(apiItem),
    sourceMetadataJson: apiItem.source_metadata_json,
    resultJson: apiItem.result_json,
    igdbCandidatesJson: apiItem.igdb_candidates_json,
    resolvedIgdbId: apiItem.resolved_igdb_id,
    resolvedAt: apiItem.resolved_at,
  };
}
```

**Step 4: Add API functions**

Add at the end of the file:

```typescript
/**
 * Get total count of items needing review across all jobs.
 * Used for nav badge display.
 */
export async function getPendingReviewCount(): Promise<PendingReviewCountResponse> {
  const response = await api.get<PendingReviewCountApiResponse>('/jobs/pending-review-count');
  return { pendingReviewCount: response.pending_review_count };
}

/**
 * Resolve a job item to an IGDB game.
 */
export async function resolveJobItem(itemId: string, igdbId: number): Promise<JobItemDetail> {
  const response = await api.post<JobItemDetailApiResponse>(
    `/job-items/${itemId}/resolve`,
    { igdb_id: igdbId }
  );
  return transformJobItemDetail(response);
}

/**
 * Skip a job item without matching.
 */
export async function skipJobItem(itemId: string, reason?: string): Promise<JobItemDetail> {
  const response = await api.post<JobItemDetailApiResponse>(
    `/job-items/${itemId}/skip`,
    { reason }
  );
  return transformJobItemDetail(response);
}
```

**Step 5: Update exports in `frontend/src/api/index.ts`**

Change the jobs export line:

```typescript
// Re-export jobs API functions
export { getJobs, getJob, cancelJob, deleteJob, getJobsSummary, getJobItems, getActiveJob, getPendingReviewCount, resolveJobItem, skipJobItem } from './jobs';
```

**Step 6: Add type exports in `frontend/src/types/index.ts`**

Check if `PendingReviewCountResponse` and `JobItemDetail` need to be exported. If the types file has explicit exports, add them.

**Step 7: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS

**Step 8: Commit**

```bash
git add frontend/src/api/jobs.ts frontend/src/api/index.ts frontend/src/types/jobs.ts
git commit -m "feat(frontend): add job item resolve/skip API functions and pending review count"
```

---

## Task 3: Frontend - Add job item hooks

**Files:**
- Modify: `frontend/src/hooks/use-jobs.ts`
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Add hooks to `frontend/src/hooks/use-jobs.ts`**

Add imports at top:

```typescript
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobItemStatus,
  JobType,
  PendingReviewCountResponse,
  JobItemDetail,
} from '@/types';
```

Add after `useActiveJob`:

```typescript
/**
 * Hook to fetch total count of items needing review.
 * Polls every 30 seconds for badge updates.
 */
export function usePendingReviewCount() {
  return useQuery({
    queryKey: [...jobsKeys.all, 'pendingReviewCount'] as const,
    queryFn: () => jobsApi.getPendingReviewCount(),
    refetchInterval: 30000,
  });
}

/**
 * Hook to resolve a job item to an IGDB ID.
 */
export function useResolveJobItem() {
  const queryClient = useQueryClient();

  return useMutation<JobItemDetail, Error, { itemId: string; igdbId: number }>({
    mutationFn: ({ itemId, igdbId }) => jobsApi.resolveJobItem(itemId, igdbId),
    onSuccess: () => {
      // Invalidate job queries to refresh progress counts
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}

/**
 * Hook to skip a job item.
 */
export function useSkipJobItem() {
  const queryClient = useQueryClient();

  return useMutation<JobItemDetail, Error, { itemId: string; reason?: string }>({
    mutationFn: ({ itemId, reason }) => jobsApi.skipJobItem(itemId, reason),
    onSuccess: () => {
      // Invalidate job queries to refresh progress counts
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}
```

**Step 2: Update exports in `frontend/src/hooks/index.ts`**

Update the jobs hooks export:

```typescript
// Jobs hooks
export {
  jobsKeys,
  useJobs,
  useJob,
  useJobsSummary,
  useJobItems,
  useActiveJob,
  useCancelJob,
  useDeleteJob,
  usePendingReviewCount,
  useResolveJobItem,
  useSkipJobItem,
} from './use-jobs';
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/hooks/use-jobs.ts frontend/src/hooks/index.ts
git commit -m "feat(frontend): add usePendingReviewCount, useResolveJobItem, useSkipJobItem hooks"
```

---

## Task 4: Frontend - Update nav badge

**Files:**
- Modify: `frontend/src/components/navigation/nav-items.tsx`

**Step 1: Update imports**

Replace:
```typescript
import { useReviewSummary, useJobsSummary } from '@/hooks';
```

With:
```typescript
import { usePendingReviewCount, useJobsSummary } from '@/hooks';
```

**Step 2: Update hook usage**

Replace:
```typescript
const { data: reviewSummary } = useReviewSummary();
const { data: jobsSummary } = useJobsSummary();

const pendingReviews = reviewSummary?.totalPending ?? 0;
const failedJobs = jobsSummary?.failedCount ?? 0;
```

With:
```typescript
const { data: pendingReviewData } = usePendingReviewCount();
const { data: jobsSummary } = useJobsSummary();

const pendingReviews = pendingReviewData?.pendingReviewCount ?? 0;
const failedJobs = jobsSummary?.failedCount ?? 0;
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/components/navigation/nav-items.tsx
git commit -m "fix(frontend): update nav badge to use new pending review count endpoint"
```

---

## Task 5: Frontend - Update sync page

**Files:**
- Modify: `frontend/src/app/(main)/sync/page.tsx`

**Step 1: Remove dead hook import**

Remove `useReviewCountsByType` from imports:

```typescript
import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, useSyncStatus } from '@/hooks';
```

**Step 2: Remove dead hook usage**

Remove this line:
```typescript
const { data: reviewCounts } = useReviewCountsByType();
```

**Step 3: Update `SyncServiceCardWithStatus` prop**

Remove the `pendingReviewCount` prop:

```typescript
<SyncServiceCardWithStatus
  key={config.id}
  config={config}
  onUpdate={handleUpdateConfig}
  onTriggerSync={handleTriggerSync}
/>
```

**Step 4: Update `SyncServiceCardWithStatus` component**

Remove `pendingReviewCount` from the props interface and component:

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
  // ... rest of component, remove pendingReviewCount prop from SyncServiceCard
```

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS (or errors about SyncServiceCard prop - fix those)

**Step 6: Commit**

```bash
git add frontend/src/app/\(main\)/sync/page.tsx
git commit -m "fix(frontend): remove dead useReviewCountsByType from sync page"
```

---

## Task 6: Frontend - Update sync detail page

**Files:**
- Modify: `frontend/src/app/(main)/sync/[platform]/page.tsx`

**Step 1: Update imports**

Replace review hooks with job hooks:

```typescript
import {
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useJob,
  useCancelJob,
  useJobItems,
  useResolveJobItem,
  useSkipJobItem,
  useSearchIGDB,
} from '@/hooks';
```

Remove these imports:
- `useReviewItems`
- `useMatchReviewItem`
- `useSkipReviewItem`
- `useKeepReviewItem`
- `useRemoveReviewItem`

**Step 2: Update type imports**

```typescript
import {
  SyncPlatform,
  SyncFrequency,
  SUPPORTED_SYNC_PLATFORMS,
  getPlatformDisplayInfo,
  getSyncFrequencyLabel,
  JobItemStatus,
  formatReleaseYear,
} from '@/types';
import type { SyncConfigUpdateData, JobItem, IGDBCandidate, IGDBGameCandidate } from '@/types';
```

Remove:
- `ReviewItemStatus`
- `ReviewSource`
- `ReviewItem`
- `ReviewFilters`

**Step 3: Replace state types**

Replace:
```typescript
const [selectedItem, setSelectedItem] = useState<ReviewItem | null>(null);
```

With:
```typescript
const [selectedItem, setSelectedItem] = useState<JobItem | null>(null);
```

**Step 4: Replace review hooks with job hooks**

Remove:
```typescript
const reviewFilters: ReviewFilters = { source: ReviewSource.SYNC, status: ReviewItemStatus.PENDING };
const { data: reviewData, isLoading: reviewLoading } = useReviewItems(reviewFilters, 1, 20);
const pendingReviewCount = reviewData?.total ?? 0;

const matchMutation = useMatchReviewItem();
const skipMutation = useSkipReviewItem();
const keepMutation = useKeepReviewItem();
const removeMutation = useRemoveReviewItem();
```

Add:
```typescript
// Fetch items needing review for the active job
const { data: reviewData, isLoading: reviewLoading } = useJobItems(
  activeJob?.id ?? '',
  JobItemStatus.PENDING_REVIEW,
  1,
  20,
  { enabled: !!activeJob?.id }
);
const pendingReviewCount = reviewData?.total ?? 0;

const resolveMutation = useResolveJobItem();
const skipMutation = useSkipJobItem();
```

**Step 5: Update handlers**

Replace `handleMatch`:
```typescript
const handleMatch = useCallback(
  async (item: JobItem, igdbId: number) => {
    setProcessingItemId(item.id);
    try {
      await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
      toast.success(`Matched "${item.sourceTitle}" to IGDB`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to match item');
    } finally {
      setProcessingItemId(null);
    }
  },
  [resolveMutation]
);
```

Replace `handleSkip`:
```typescript
const handleSkip = useCallback(
  async (item: JobItem) => {
    setProcessingItemId(item.id);
    try {
      await skipMutation.mutateAsync({ itemId: item.id });
      toast.success(`Skipped "${item.sourceTitle}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to skip item');
    } finally {
      setProcessingItemId(null);
    }
  },
  [skipMutation]
);
```

Remove `handleKeep` and `handleRemove` functions entirely.

**Step 6: Update `handleModalMatch`**

```typescript
const handleModalMatch = useCallback(
  async (igdbId: number) => {
    if (!selectedItem) return;
    setProcessingItemId(selectedItem.id);
    try {
      await resolveMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
      toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
      setSelectedItem(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to match item');
    } finally {
      setProcessingItemId(null);
    }
  },
  [selectedItem, resolveMutation]
);
```

**Step 7: Update `handleModalSkip`**

```typescript
const handleModalSkip = useCallback(async () => {
  if (!selectedItem) return;
  setProcessingItemId(selectedItem.id);
  try {
    await skipMutation.mutateAsync({ itemId: selectedItem.id });
    toast.success(`Skipped "${selectedItem.sourceTitle}"`);
    setSelectedItem(null);
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to skip item');
  } finally {
    setProcessingItemId(null);
  }
}, [selectedItem, skipMutation]);
```

**Step 8: Update `handleSearchResultMatch`**

```typescript
const handleSearchResultMatch = useCallback(
  async (igdbId: number) => {
    if (!selectedItem) return;
    setProcessingItemId(selectedItem.id);
    try {
      await resolveMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
      toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
      setSelectedItem(null);
      setSearchQuery('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to match item');
    } finally {
      setProcessingItemId(null);
    }
  },
  [selectedItem, resolveMutation]
);
```

**Step 9: Update `handleView`**

```typescript
const handleView = useCallback((item: JobItem) => {
  setSelectedItem(item);
}, []);
```

**Step 10: Update ReviewItemCard usage**

The `ReviewItemCard` component expects `ReviewItem` but we now have `JobItem`. We need to either:
1. Update the ReviewItemCard to accept JobItem
2. Create an adapter

For now, we'll need to check the ReviewItemCard component and update it. This may require creating a new JobItemCard component or updating the existing one.

First, check if there's a simple mapping possible. The key differences:
- `ReviewItem.igdbCandidates` vs `JobItem` doesn't have this (it's in `igdbCandidatesJson`)
- Remove `onKeep` and `onRemove` props

**Step 11: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: Errors about type mismatches. Fix them as needed.

**Step 12: Commit**

```bash
git add frontend/src/app/\(main\)/sync/\[platform\]/page.tsx
git commit -m "fix(frontend): update sync detail page to use job-items API"
```

---

## Task 7: Frontend - Update or remove ReviewItemCard

**Files:**
- Check: `frontend/src/components/review/review-item-card.tsx`
- Possibly modify or delete

**Step 1: Check the component**

Read the ReviewItemCard component to understand its props and usage.

**Step 2: Decide approach**

Option A: If the component is only used in sync detail page, update it to work with JobItem
Option B: If it's used elsewhere, create a new JobItemCard component
Option C: If it's complex, simplify by inlining the UI in the sync detail page

**Step 3: Implement the chosen approach**

This will depend on what we find in Step 1.

**Step 4: Run type check and tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test`

**Step 5: Commit**

```bash
git add frontend/src/components/review/
git commit -m "fix(frontend): update ReviewItemCard for JobItem compatibility"
```

---

## Task 8: Frontend - Delete dead review code

**Files:**
- Delete: `frontend/src/api/review.ts`
- Delete: `frontend/src/api/review.test.ts`
- Delete: `frontend/src/hooks/use-review.ts`
- Delete: `frontend/src/hooks/use-review.test.tsx`
- Modify: `frontend/src/api/index.ts`
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Remove exports from `frontend/src/api/index.ts`**

Delete these lines:
```typescript
// Re-export review API functions
export {
  getReviewItems,
  getReviewItem,
  getReviewSummary,
  getReviewCountsByType,
  matchReviewItem,
  skipReviewItem,
  keepReviewItem,
  removeReviewItem,
  getPlatformSummary,
  finalizeImport,
} from './review';
```

**Step 2: Remove exports from `frontend/src/hooks/index.ts`**

Delete these lines:
```typescript
// Review hooks
export {
  reviewKeys,
  useReviewItems,
  useReviewItem,
  useReviewSummary,
  useReviewCountsByType,
  usePlatformSummary,
  useMatchReviewItem,
  useSkipReviewItem,
  useKeepReviewItem,
  useRemoveReviewItem,
  useFinalizeImport,
} from './use-review';
```

**Step 3: Delete the files**

```bash
rm frontend/src/api/review.ts
rm frontend/src/api/review.test.ts
rm frontend/src/hooks/use-review.ts
rm frontend/src/hooks/use-review.test.tsx
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: PASS (if any errors, there are still references to delete)

**Step 5: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: PASS

**Step 6: Commit**

```bash
git add -A
git commit -m "chore(frontend): remove dead review API code"
```

---

## Task 9: Final verification

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`

Expected: All tests pass

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: All tests pass

**Step 3: Run frontend type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: No errors

**Step 4: Manual testing with podman-compose**

```bash
podman-compose up --build
```

Test:
1. Check browser console for 404 errors - should be gone
2. Navigate to sync page
3. Trigger a sync
4. Verify items needing review appear
5. Test resolve and skip actions
6. Check nav badge shows correct count

**Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: final adjustments for review API migration"
```

---

## Notes

- The `keep` and `remove` actions are intentionally not migrated - they were for handling removed games which is now handled differently
- Platform/storefront mapping is not needed - the sync source determines the platform
- The job-items API returns `igdb_candidates_json` as a JSON string that needs parsing if we want to show candidates
- Consider adding a future improvement to have the backend return parsed candidates
