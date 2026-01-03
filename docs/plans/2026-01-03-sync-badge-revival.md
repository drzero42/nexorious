# Sync Badge Revival Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show pending review counts on the Sync nav item badge and individual sync service cards.

**Architecture:** Extend the existing `/jobs/pending-review-count` endpoint to return per-source breakdowns. Wire the total count to navigation, and per-source counts to sync service cards.

**Tech Stack:** FastAPI (Python), React/Next.js, TanStack Query, SQLModel

---

## Task 1: Extend Backend Endpoint

**Files:**
- Modify: `backend/app/api/jobs.py:49-53` (response model)
- Modify: `backend/app/api/jobs.py:192-209` (endpoint)
- Modify: `backend/app/tests/test_jobs_api.py:1059-1138` (tests)

**Step 1: Update existing tests to expect the new response shape**

In `backend/app/tests/test_jobs_api.py`, update the `TestPendingReviewCount` class:

```python
class TestPendingReviewCount:
    """Tests for GET /api/jobs/pending-review-count endpoint."""

    def test_pending_review_count_empty(self, client, auth_headers, test_user: User):
        """Test pending review count when user has no items."""
        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 0, "counts_by_source": {}}

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
        data = response.json()
        assert data["pending_review_count"] == 2
        assert data["counts_by_source"] == {"steam": 2}

    def test_pending_review_count_multiple_sources(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test pending review count aggregates across multiple sources."""
        # Create Steam job
        steam_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(steam_job)
        session.commit()

        # Create Epic job
        epic_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.EPIC,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(epic_job)
        session.commit()

        # Add pending review items to Steam
        for i in range(3):
            item = JobItem(
                job_id=steam_job.id,
                user_id=test_user.id,
                item_key=f"steam_game_{i}",
                source_title=f"Steam Game {i}",
                status=JobItemStatus.PENDING_REVIEW,
            )
            session.add(item)

        # Add pending review items to Epic
        for i in range(2):
            item = JobItem(
                job_id=epic_job.id,
                user_id=test_user.id,
                item_key=f"epic_game_{i}",
                source_title=f"Epic Game {i}",
                status=JobItemStatus.PENDING_REVIEW,
            )
            session.add(item)

        session.commit()

        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["pending_review_count"] == 5
        assert data["counts_by_source"] == {"steam": 3, "epic": 2}

    def test_pending_review_count_excludes_other_users(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that pending review count only includes current user's items."""
        # Create another user
        other_user = User(
            username="other_user",
            password_hash="$2b$12$test_hash",
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
        assert response.json() == {"pending_review_count": 0, "counts_by_source": {}}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/backend && uv run pytest app/tests/test_jobs_api.py::TestPendingReviewCount -v`

Expected: FAIL - response missing `counts_by_source`

**Step 3: Update the response model**

In `backend/app/api/jobs.py`, update `PendingReviewCountResponse`:

```python
class PendingReviewCountResponse(BaseModel):
    """Response for pending review count endpoint."""

    pending_review_count: int
    counts_by_source: dict[str, int]
```

**Step 4: Update the endpoint implementation**

In `backend/app/api/jobs.py`, replace the `get_pending_review_count` function:

```python
@router.get("/pending-review-count", response_model=PendingReviewCountResponse)
async def get_pending_review_count(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> PendingReviewCountResponse:
    """
    Get total count of items needing review across all jobs.

    Used by the frontend navigation to show a badge with pending items.
    Returns both total count and per-source breakdown.
    """
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
        counts_by_source=counts_by_source,
    )
```

**Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/backend && uv run pytest app/tests/test_jobs_api.py::TestPendingReviewCount -v`

Expected: PASS (all 4 tests)

**Step 6: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/backend && uv run pytest -q`

Expected: All tests pass

**Step 7: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/test_jobs_api.py
git commit -m "feat(api): add per-source breakdown to pending review count endpoint"
```

---

## Task 2: Update Frontend Types and API Client

**Files:**
- Modify: `frontend/src/types/jobs.ts:141-143`
- Modify: `frontend/src/api/jobs.ts:106-108`
- Modify: `frontend/src/api/jobs.ts:358-361`

**Step 1: Update the TypeScript type**

In `frontend/src/types/jobs.ts`, update `PendingReviewCountResponse`:

```typescript
export interface PendingReviewCountResponse {
  pendingReviewCount: number;
  countsBySource: Record<string, number>;
}
```

**Step 2: Update the API response interface**

In `frontend/src/api/jobs.ts`, update `PendingReviewCountApiResponse`:

```typescript
interface PendingReviewCountApiResponse {
  pending_review_count: number;
  counts_by_source: Record<string, number>;
}
```

**Step 3: Update the API function**

In `frontend/src/api/jobs.ts`, update `getPendingReviewCount`:

```typescript
export async function getPendingReviewCount(): Promise<PendingReviewCountResponse> {
  const response = await api.get<PendingReviewCountApiResponse>('/jobs/pending-review-count');
  return {
    pendingReviewCount: response.pending_review_count,
    countsBySource: response.counts_by_source,
  };
}
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run check`

Expected: No errors (warnings OK)

**Step 5: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run test`

Expected: All tests pass

**Step 6: Commit**

```bash
git add frontend/src/types/jobs.ts frontend/src/api/jobs.ts
git commit -m "feat(frontend): update types for per-source pending review counts"
```

---

## Task 3: Wire Badge to Navigation

**Files:**
- Modify: `frontend/src/components/navigation/nav-items.tsx`

**Step 1: Import the hook and wire up the badge**

Replace the entire `frontend/src/components/navigation/nav-items.tsx` file:

```typescript
// frontend/src/components/navigation/nav-items.tsx
'use client';

import {
  LayoutDashboard,
  Library,
  Plus,
  RefreshCw,
  Tag,
  Users,
  Layers,
  Shield,
  Wrench,
  DatabaseBackup,
} from 'lucide-react';
import type { NavItem, NavSection } from './types';
import { usePendingReviewCount } from '@/hooks/use-jobs';

export function useNavItems() {
  const { data: reviewData } = usePendingReviewCount();
  const pendingReviewCount = reviewData?.pendingReviewCount ?? 0;

  const mainItems: NavItem[] = [
    {
      href: '/dashboard',
      label: 'Dashboard',
      icon: <LayoutDashboard className="h-4 w-4" />,
    },
    {
      href: '/games',
      label: 'Library',
      icon: <Library className="h-4 w-4" />,
    },
    {
      href: '/games/add',
      label: 'Add Game',
      icon: <Plus className="h-4 w-4" />,
    },
    {
      href: '/sync',
      label: 'Sync',
      icon: <RefreshCw className="h-4 w-4" />,
      badge: pendingReviewCount,
    },
    {
      href: '/tags',
      label: 'Tags',
      icon: <Tag className="h-4 w-4" />,
    },
  ];

  const adminSection: NavSection = {
    label: 'Administration',
    icon: <Shield className="h-4 w-4" />,
    items: [
      {
        href: '/admin',
        label: 'Admin Dashboard',
        icon: <LayoutDashboard className="h-4 w-4" />,
      },
      {
        href: '/admin/users',
        label: 'User Management',
        icon: <Users className="h-4 w-4" />,
      },
      {
        href: '/admin/platforms',
        label: 'Platforms',
        icon: <Layers className="h-4 w-4" />,
      },
      {
        href: '/admin/maintenance',
        label: 'Maintenance',
        icon: <Wrench className="h-4 w-4" />,
      },
      {
        href: '/admin/backups',
        label: 'Backup / Restore',
        icon: <DatabaseBackup className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  return { mainItems, adminSection };
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run check`

Expected: No errors

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run test`

Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/components/navigation/nav-items.tsx
git commit -m "feat(frontend): add pending review badge to Sync nav item"
```

---

## Task 4: Add Badge to Sync Service Card

**Files:**
- Modify: `frontend/src/components/sync/sync-service-card.tsx`
- Modify: `frontend/src/components/sync/sync-service-card.test.tsx`

**Step 1: Add test for the new badge prop**

In `frontend/src/components/sync/sync-service-card.test.tsx`, add a new describe block after the existing tests (before the final closing braces):

```typescript
  describe('pending review badge', () => {
    it('shows pending review badge when count > 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          pendingReviewCount={5}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('5 to review')).toBeInTheDocument();
    });

    it('does not show pending review badge when count is 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          pendingReviewCount={0}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.queryByText(/to review/)).not.toBeInTheDocument();
    });

    it('does not show pending review badge when count is undefined', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.queryByText(/to review/)).not.toBeInTheDocument();
    });
  });
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run test -- src/components/sync/sync-service-card.test.tsx`

Expected: FAIL - pendingReviewCount prop doesn't exist

**Step 3: Add the prop and badge to the component**

In `frontend/src/components/sync/sync-service-card.tsx`:

First, update the interface (around line 24):

```typescript
interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
}
```

Then update the function signature (around line 49):

```typescript
export function SyncServiceCard({
  config,
  status,
  pendingReviewCount,
  onUpdate,
  onTriggerSync,
  isUpdating = false,
  isSyncing = false,
}: SyncServiceCardProps) {
```

Finally, update the badge section in the header (around line 96). Replace the single Badge with a div containing both badges:

```typescript
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Badge variant="destructive">
                {pendingReviewCount} to review
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
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run test -- src/components/sync/sync-service-card.test.tsx`

Expected: PASS (all tests including new ones)

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run check`

Expected: No errors

**Step 6: Commit**

```bash
git add frontend/src/components/sync/sync-service-card.tsx frontend/src/components/sync/sync-service-card.test.tsx
git commit -m "feat(frontend): add pending review badge to sync service card"
```

---

## Task 5: Pass Counts to Sync Service Cards

**Files:**
- Modify: `frontend/src/app/(main)/sync/page.tsx`

**Step 1: Import the hook and pass counts to cards**

In `frontend/src/app/(main)/sync/page.tsx`:

First, add the import at the top (after the existing hook imports on line 3):

```typescript
import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, useSyncStatus, usePendingReviewCount } from '@/hooks';
```

Then update `SyncServiceCardWithStatus` function to use the hook (around line 40):

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

  const pendingReviewCount = reviewData?.countsBySource[config.platform] ?? 0;

  const handleUpdate = async (data: SyncConfigUpdateData) => {
    await onUpdate(config.platform, data);
  };

  const handleTriggerSync = async () => {
    await onTriggerSync(config.platform);
  };

  return (
    <SyncServiceCard
      config={config}
      status={status}
      pendingReviewCount={pendingReviewCount}
      onUpdate={handleUpdate}
      onTriggerSync={handleTriggerSync}
      isUpdating={isUpdating}
      isSyncing={isSyncing}
    />
  );
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run check`

Expected: No errors

**Step 3: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run test`

Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/app/(main)/sync/page.tsx
git commit -m "feat(frontend): pass per-source review counts to sync cards"
```

---

## Task 6: Final Verification

**Step 1: Run backend type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/backend && uv run pyrefly check`

Expected: No errors

**Step 2: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/backend && uv run pytest -q`

Expected: All tests pass

**Step 3: Run full frontend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-badge-revival/frontend && npm run check && npm run test`

Expected: No errors, all tests pass

**Step 4: Mark feature complete**

Update the IDEAS.md to remove or mark the completed item if appropriate.
