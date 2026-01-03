# Sync Badge Revival Design

## Overview

Revive the badge feature that shows pending review counts next to the Sync menu item and on individual sync source cards.

## Problem

Refactoring work killed the sync badge feature. Users need visual indication when games are waiting for review across sync sources.

## Solution

1. Extend the existing `/jobs/pending-review-count` endpoint to return per-source breakdowns
2. Wire the total count to the Sync navigation item badge
3. Display per-source counts on each sync service card

## Implementation

### Backend: Extend Pending Review Count Endpoint

**File:** `backend/app/api/jobs.py`

Modify `PendingReviewCountResponse` to include per-source counts:

```python
class PendingReviewCountResponse(BaseModel):
    pending_review_count: int
    counts_by_source: dict[str, int]  # {"steam": 5, "epic": 2, ...}
```

Update the query to join `JobItem` → `Job` and group by `Job.source`:

```python
@router.get("/pending-review-count", response_model=PendingReviewCountResponse)
async def get_pending_review_count(...):
    # Query grouped by source
    results = session.exec(
        select(Job.source, func.count())
        .select_from(JobItem)
        .join(Job, JobItem.job_id == Job.id)
        .where(JobItem.user_id == current_user.id)
        .where(JobItem.status == JobItemStatus.PENDING_REVIEW)
        .group_by(Job.source)
    ).all()

    counts_by_source = {source.value: count for source, count in results}
    total = sum(counts_by_source.values())

    return PendingReviewCountResponse(
        pending_review_count=total,
        counts_by_source=counts_by_source
    )
```

### Frontend: Update Types

**File:** `frontend/src/types/jobs.ts`

```typescript
export interface PendingReviewCountResponse {
  pendingReviewCount: number;
  countsBySource: Record<string, number>;
}
```

### Frontend: Update API Client

**File:** `frontend/src/api/jobs.ts`

Transform snake_case response to camelCase:

```typescript
export async function getPendingReviewCount(): Promise<PendingReviewCountResponse> {
  const response = await api.get<{
    pending_review_count: number;
    counts_by_source: Record<string, number>;
  }>('/jobs/pending-review-count');
  return {
    pendingReviewCount: response.pending_review_count,
    countsBySource: response.counts_by_source,
  };
}
```

### Frontend: Wire Up Navigation Badge

**File:** `frontend/src/components/navigation/nav-items.tsx`

```typescript
import { usePendingReviewCount } from '@/hooks/use-jobs';

export function useNavItems() {
  const { data: reviewData } = usePendingReviewCount();
  const pendingReviewCount = reviewData?.pendingReviewCount ?? 0;

  const mainItems: NavItem[] = [
    // ... other items ...
    {
      href: '/sync',
      label: 'Sync',
      icon: <RefreshCw className="h-4 w-4" />,
      badge: pendingReviewCount,
    },
    // ...
  ];
  // ...
}
```

### Frontend: Add Badge to Sync Service Card

**File:** `frontend/src/components/sync/sync-service-card.tsx`

Add `pendingReviewCount` prop and display badge in header:

```typescript
interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;  // NEW
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
}
```

Display in header alongside the "Connected" badge:

```typescript
<div className="flex items-center gap-2">
  {pendingReviewCount > 0 && (
    <Badge variant="destructive">
      {pendingReviewCount} to review
    </Badge>
  )}
  <Badge variant={...}>
    {!config.isConfigured ? 'Not Configured' : 'Connected'}
  </Badge>
</div>
```

### Frontend: Pass Counts to Cards

**File:** `frontend/src/app/(main)/sync/page.tsx`

Use the hook and pass per-source counts:

```typescript
function SyncServiceCardWithStatus({ config, ... }) {
  const { data: reviewData } = usePendingReviewCount();
  const pendingReviewCount = reviewData?.countsBySource[config.platform] ?? 0;

  return (
    <SyncServiceCard
      config={config}
      pendingReviewCount={pendingReviewCount}
      // ...
    />
  );
}
```

## Files to Modify

| File | Change |
|------|--------|
| `backend/app/api/jobs.py` | Extend endpoint and response model |
| `frontend/src/types/jobs.ts` | Add `countsBySource` field |
| `frontend/src/api/jobs.ts` | Transform response |
| `frontend/src/components/navigation/nav-items.tsx` | Wire up badge |
| `frontend/src/components/sync/sync-service-card.tsx` | Add badge prop & display |
| `frontend/src/app/(main)/sync/page.tsx` | Pass counts to cards |

## Testing

- Backend: Add test for grouped count query
- Frontend: Update existing tests for new prop/response shape
