# Fix Review API 404s - Design Document

## Problem

After the job/task system migration (commit `b48bae5`), the backend review API was removed and replaced with the job-items API. However, the frontend was never updated to use the new endpoints, causing 404 errors:

```
GET /api/review/summary HTTP/1.1" 404 Not Found
```

The sync detail page's inline review functionality is completely broken.

## Root Cause

The migration deleted:
- `backend/app/api/review.py`
- `backend/app/schemas/review.py`
- `backend/app/tests/test_review_api.py`

But the frontend still calls these dead endpoints via:
- `frontend/src/api/review.ts`
- `frontend/src/hooks/use-review.ts`

## What Already Works

The new job-items API provides most of the needed functionality:

| Functionality | Endpoint | Status |
|--------------|----------|--------|
| List items for a job | `GET /api/jobs/{job_id}/items?status=pending_review` | ✅ Works |
| Get single item | `GET /api/job-items/{item_id}` | ✅ Works |
| Match item to IGDB | `POST /api/job-items/{item_id}/resolve` | ✅ Works |
| Skip item | `POST /api/job-items/{item_id}/skip` | ✅ Works |
| Job progress with pendingReview count | `GET /api/jobs/{job_id}` | ✅ Works |

Frontend hooks that already work:
- `useJobItems(jobId, status, page, pageSize)` - fetches job items
- `useJob(jobId)` - fetches job with progress including `pendingReview` count
- `useActiveJob(jobType)` - gets active job for a type (sync/import/export)

## What's Missing

### Backend

One new endpoint needed for the nav badge:

```
GET /api/jobs/pending-review-count
Response: { pending_review_count: number }
```

Returns total count of items in `PENDING_REVIEW` status across all jobs for the current user.

### Frontend

Mutations for resolving/skipping job items need to be added to `use-jobs.ts`:
- `useResolveJobItem()` - calls `POST /api/job-items/{item_id}/resolve`
- `useSkipJobItem()` - calls `POST /api/job-items/{item_id}/skip`

## Implementation Plan

### Phase 1: Backend - Add pending review count endpoint

**File: `backend/app/api/jobs.py`**

Add endpoint:
```python
@router.get("/pending-review-count")
async def get_pending_review_count(
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
) -> dict:
    """Get total count of items needing review across all jobs."""
    count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .join(Job)
        .where(Job.user_id == current_user.id)
        .where(JobItem.status == JobItemStatus.PENDING_REVIEW)
    ).one()
    return {"pending_review_count": count}
```

### Phase 2: Frontend - Add job item mutations

**File: `frontend/src/api/jobs.ts`**

Add API functions:
```typescript
export async function resolveJobItem(itemId: string, igdbId: number): Promise<JobItem> {
  return api.post(`/job-items/${itemId}/resolve`, { igdb_id: igdbId });
}

export async function skipJobItem(itemId: string, reason?: string): Promise<JobItem> {
  return api.post(`/job-items/${itemId}/skip`, { reason });
}

export async function getPendingReviewCount(): Promise<{ pendingReviewCount: number }> {
  const response = await api.get<{ pending_review_count: number }>('/jobs/pending-review-count');
  return { pendingReviewCount: response.pending_review_count };
}
```

**File: `frontend/src/hooks/use-jobs.ts`**

Add hooks:
```typescript
export function usePendingReviewCount() {
  return useQuery({
    queryKey: [...jobsKeys.all, 'pendingReviewCount'],
    queryFn: () => jobsApi.getPendingReviewCount(),
    refetchInterval: 30000, // Poll every 30 seconds
  });
}

export function useResolveJobItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ itemId, igdbId }: { itemId: string; igdbId: number }) =>
      jobsApi.resolveJobItem(itemId, igdbId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}

export function useSkipJobItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ itemId, reason }: { itemId: string; reason?: string }) =>
      jobsApi.skipJobItem(itemId, reason),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}
```

### Phase 3: Frontend - Update nav badge

**File: `frontend/src/components/navigation/nav-items.tsx`**

Replace:
```typescript
// Before
const { data: reviewSummary } = useReviewSummary();
const pendingReviews = reviewSummary?.totalPending ?? 0;

// After
const { data: pendingReviewData } = usePendingReviewCount();
const pendingReviews = pendingReviewData?.pendingReviewCount ?? 0;
```

### Phase 4: Frontend - Update sync detail page

**File: `frontend/src/app/(main)/sync/[platform]/page.tsx`**

Key changes:
1. Remove imports from `use-review.ts`
2. Use `useJobItems(activeJob?.id, JobItemStatus.PENDING_REVIEW)` for items needing review
3. Replace `useMatchReviewItem()` with `useResolveJobItem()`
4. Replace `useSkipReviewItem()` with `useSkipJobItem()`
5. Remove `useKeepReviewItem()` and `useRemoveReviewItem()` (not needed for sync)
6. Update the review section to only show when there's an active job with pending items

### Phase 5: Frontend - Update sync page

**File: `frontend/src/app/(main)/sync/page.tsx`**

Remove `useReviewCountsByType()` - no longer needed. The pending review count per platform can come from active job progress if needed.

### Phase 6: Frontend - Delete dead code

Delete files:
- `frontend/src/api/review.ts`
- `frontend/src/api/review.test.ts`
- `frontend/src/hooks/use-review.ts`
- `frontend/src/hooks/use-review.test.tsx`

Update exports:
- `frontend/src/api/index.ts` - remove review exports
- `frontend/src/hooks/index.ts` - remove review exports

Also remove related types if they become unused:
- Review-related types in `frontend/src/types/`

## Testing

1. Run backend tests: `uv run pytest`
2. Run frontend tests: `npm run test`
3. Manual testing:
   - Verify nav badge shows correct pending review count
   - Trigger a sync, verify items needing review appear
   - Test resolve and skip actions
   - Verify 404 errors are gone from console

## Notes

- The `keep` and `remove` actions from the old review API are not needed for sync (those were for handling games removed from source, which is now handled differently)
- Platform/storefront mapping is not needed - we know the platform/storefront based on the sync source
- The sync detail page should show job history when no active job is running (using existing `RecentActivity` component pattern from import page)
