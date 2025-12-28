# Sync UI Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix sync UI so games needing review don't disappear, and consolidate all sync interaction into the progress box with detailed recent activity.

**Architecture:** Backend change to block job completion on PENDING_REVIEW items. Frontend refactor to embed review widgets inline in the progress box, remove separate review section, and add expandable recent activity with game details.

**Tech Stack:** FastAPI/SQLModel backend, React/TanStack Query frontend, shadcn/ui components

---

## Task 1: Backend - Block Job Completion on PENDING_REVIEW

**Files:**
- Modify: `backend/app/worker/tasks/sync/process_item.py:497-540`
- Test: `backend/app/tests/test_sync_process_item.py` (create if needed)

**Step 1: Write the failing test**

Create test file `backend/app/tests/test_sync_job_completion.py`:

```python
"""Tests for sync job completion logic."""

import pytest
from unittest.mock import MagicMock, patch
from sqlmodel import Session

from app.worker.tasks.sync.process_item import _check_and_update_job_completion
from app.models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus, BackgroundJobType, BackgroundJobSource


class TestJobCompletionBlocking:
    """Test that PENDING_REVIEW items block job completion."""

    def test_job_not_completed_when_pending_review_items_exist(self, db_session: Session):
        """Job should stay PROCESSING when PENDING_REVIEW items exist."""
        # Create a job
        job = Job(
            user_id="test-user",
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=3,
        )
        db_session.add(job)
        db_session.commit()
        db_session.refresh(job)

        # Create items: 1 completed, 1 pending_review, 1 skipped
        items = [
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.PENDING_REVIEW,
            ),
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game3",
                source_title="Game 3",
                status=JobItemStatus.SKIPPED,
            ),
        ]
        for item in items:
            db_session.add(item)
        db_session.commit()

        # Check completion - should NOT complete
        result = _check_and_update_job_completion(db_session, job.id)

        assert result is False
        db_session.refresh(job)
        assert job.status == BackgroundJobStatus.PROCESSING

    def test_job_completed_when_no_pending_review_items(self, db_session: Session):
        """Job should complete when all items are terminal (no PENDING_REVIEW)."""
        # Create a job
        job = Job(
            user_id="test-user",
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=3,
        )
        db_session.add(job)
        db_session.commit()
        db_session.refresh(job)

        # Create items: all terminal (completed, skipped, failed)
        items = [
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.SKIPPED,
            ),
            JobItem(
                job_id=job.id,
                user_id="test-user",
                item_key="game3",
                source_title="Game 3",
                status=JobItemStatus.FAILED,
            ),
        ]
        for item in items:
            db_session.add(item)
        db_session.commit()

        # Check completion - should complete
        with patch.object(
            Job, 'job_type', BackgroundJobType.SYNC
        ):
            result = _check_and_update_job_completion(db_session, job.id)

        assert result is True
        db_session.refresh(job)
        assert job.status == BackgroundJobStatus.COMPLETED
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest app/tests/test_sync_job_completion.py -v`
Expected: FAIL - first test passes (bug!), second test passes

**Step 3: Modify job completion logic**

Edit `backend/app/worker/tasks/sync/process_item.py` - modify `_check_and_update_job_completion`:

```python
def _check_and_update_job_completion(session: Session, job_id: str) -> bool:
    """Check if all job items are processed and update job status.

    A job is considered complete only when ALL items are in terminal states:
    - COMPLETED
    - SKIPPED
    - FAILED

    PENDING_REVIEW items block completion - user must resolve them first.

    Also updates last_synced_at for SYNC jobs when complete.

    Returns:
        True if job was marked complete, False otherwise
    """
    # Count items that are NOT in terminal state (still need work)
    non_terminal_count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(
            JobItem.job_id == job_id,
            col(JobItem.status).in_([
                JobItemStatus.PENDING,
                JobItemStatus.PROCESSING,
                JobItemStatus.PENDING_REVIEW,  # PENDING_REVIEW blocks completion
            ])
        )
    ).one()

    if non_terminal_count > 0:
        return False

    # All items in terminal state - update job
    job = session.get(Job, job_id)
    if not job:
        logger.error(f"Job {job_id} not found when checking completion")
        return False

    # Only update if not already terminal
    if job.status in (BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return False

    # Mark job complete
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)

    # Update last_synced_at for SYNC jobs
    if job.job_type == BackgroundJobType.SYNC:
        _update_sync_config_timestamp(session, job.user_id, job.source)

    session.commit()
    logger.info(f"Job {job_id} marked as COMPLETED")

    return True
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest app/tests/test_sync_job_completion.py -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest -v`
Expected: All tests pass

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add backend/app/worker/tasks/sync/process_item.py backend/app/tests/test_sync_job_completion.py
git commit -m "fix(sync): block job completion when PENDING_REVIEW items exist"
```

---

## Task 2: Frontend - Add Inline Review Widgets to Progress Box

**Files:**
- Modify: `frontend/src/components/jobs/job-items-details.tsx`
- No new test file needed - existing component tests cover this

**Step 1: Update StatusSection to support inline review actions**

The `StatusSection` component in `job-items-details.tsx` needs to render review widgets for PENDING_REVIEW items. Add props for review callbacks and render the match/skip UI inline.

Edit `frontend/src/components/jobs/job-items-details.tsx`:

```tsx
'use client';

import { useState, useCallback } from 'react';
import { toast } from 'sonner';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  ChevronDown,
  ChevronRight,
  AlertCircle,
  CheckCircle,
  Clock,
  Loader2,
  RotateCcw,
  SkipForward,
  Search,
  ImageOff,
} from 'lucide-react';
import {
  useJobItems,
  useRetryFailedItems,
  useRetryJobItem,
  useResolveJobItem,
  useSkipJobItem,
  useSearchIGDB,
} from '@/hooks';
import { JobItemStatus, getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';
import type { JobItem, IGDBGameCandidate } from '@/types';

interface JobItemsDetailsProps {
  jobId: string;
  progress: {
    pending: number;
    processing: number;
    completed: number;
    pendingReview: number;
    skipped: number;
    failed: number;
  };
  isTerminal: boolean;
}

interface StatusSectionProps {
  jobId: string;
  status: JobItemStatus;
  count: number;
  defaultOpen?: boolean;
  isTerminal: boolean;
}

// Search result item component for the modal
function SearchResultItem({
  result,
  isProcessing,
  onSelect,
}: {
  result: IGDBGameCandidate;
  isProcessing: boolean;
  onSelect: () => void;
}) {
  const releaseYear = result.release_date
    ? new Date(result.release_date).getFullYear()
    : null;

  return (
    <button
      className="flex w-full items-center gap-3 rounded-md p-2 text-left transition-colors hover:bg-muted disabled:opacity-50"
      onClick={onSelect}
      disabled={isProcessing}
    >
      {result.cover_art_url ? (
        <img
          src={result.cover_art_url}
          alt={result.title}
          className="h-12 w-9 rounded object-cover"
        />
      ) : (
        <div className="flex h-12 w-9 items-center justify-center rounded bg-muted">
          <ImageOff className="h-4 w-4 text-muted-foreground" />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">
          {result.title}
          {releaseYear && (
            <span className="ml-1 text-muted-foreground">({releaseYear})</span>
          )}
        </p>
        {result.platforms.length > 0 && (
          <p className="truncate text-xs text-muted-foreground">
            {result.platforms.slice(0, 3).join(', ')}
            {result.platforms.length > 3 && ` +${result.platforms.length - 3}`}
          </p>
        )}
      </div>
    </button>
  );
}

// Inline review widget for a single PENDING_REVIEW item
function ReviewItemWidget({
  item,
  isProcessing,
  onProcessingChange,
}: {
  item: JobItem;
  isProcessing: boolean;
  onProcessingChange: (processing: boolean) => void;
}) {
  const [showSearchModal, setShowSearchModal] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const resolveMutation = useResolveJobItem();
  const skipMutation = useSkipJobItem();
  const { data: searchResults, isLoading: isSearching } = useSearchIGDB(searchQuery);

  // Parse IGDB candidates from the item
  const candidates: IGDBGameCandidate[] = (() => {
    try {
      const parsed = JSON.parse(item.igdbCandidatesJson || '[]');
      return parsed.map((c: Record<string, unknown>) => ({
        igdb_id: c.igdb_id as number,
        title: (c.name || c.title) as string,
        release_date: c.release_date as string | null,
        cover_art_url: c.cover_art_url as string | null,
        platforms: (c.platforms || []) as string[],
      }));
    } catch {
      return [];
    }
  })();

  const handleMatch = useCallback(
    async (igdbId: number) => {
      onProcessingChange(true);
      try {
        await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}"`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match');
      } finally {
        onProcessingChange(false);
      }
    },
    [item.id, item.sourceTitle, resolveMutation, onProcessingChange]
  );

  const handleSkip = useCallback(async () => {
    onProcessingChange(true);
    try {
      await skipMutation.mutateAsync({ itemId: item.id });
      toast.success(`Skipped "${item.sourceTitle}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to skip');
    } finally {
      onProcessingChange(false);
    }
  }, [item.id, item.sourceTitle, skipMutation, onProcessingChange]);

  const handleSearchMatch = useCallback(
    async (igdbId: number) => {
      onProcessingChange(true);
      try {
        await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}"`);
        setShowSearchModal(false);
        setSearchQuery('');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match');
      } finally {
        onProcessingChange(false);
      }
    },
    [item.id, item.sourceTitle, resolveMutation, onProcessingChange]
  );

  return (
    <div className="rounded-md border p-3 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <div className="font-medium truncate">{item.sourceTitle}</div>
        </div>
        <div className="flex gap-1 shrink-0">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setSearchQuery(item.sourceTitle);
              setShowSearchModal(true);
            }}
            disabled={isProcessing}
            className="h-7"
          >
            <Search className="h-3 w-3 mr-1" />
            Search
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleSkip}
            disabled={isProcessing}
            className="h-7"
          >
            <SkipForward className="h-3 w-3 mr-1" />
            Skip
          </Button>
        </div>
      </div>

      {/* Suggested matches */}
      {candidates.length > 0 && (
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">Suggested matches:</div>
          <div className="flex flex-wrap gap-1">
            {candidates.slice(0, 3).map((candidate) => (
              <Button
                key={candidate.igdb_id}
                variant="secondary"
                size="sm"
                onClick={() => handleMatch(candidate.igdb_id)}
                disabled={isProcessing}
                className="h-auto py-1 px-2 text-xs"
              >
                {candidate.title}
                <span className="ml-1 text-muted-foreground">
                  (ID: {candidate.igdb_id})
                </span>
              </Button>
            ))}
          </div>
        </div>
      )}

      {/* Search Modal */}
      <Dialog open={showSearchModal} onOpenChange={setShowSearchModal}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Search: {item.sourceTitle}</DialogTitle>
            <DialogDescription>
              Search IGDB to find the correct match
            </DialogDescription>
          </DialogHeader>

          <div className="pt-2">
            <div className="relative">
              <Input
                placeholder="Search IGDB..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
              {searchQuery.length >= 3 && (
                <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-64 overflow-y-auto rounded-md border bg-popover shadow-lg">
                  {isSearching ? (
                    <div className="flex items-center justify-center p-4">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span className="ml-2 text-sm text-muted-foreground">
                        Searching...
                      </span>
                    </div>
                  ) : searchResults && searchResults.length > 0 ? (
                    <div className="p-1">
                      {searchResults.map((result) => (
                        <SearchResultItem
                          key={result.igdb_id}
                          result={result}
                          isProcessing={isProcessing}
                          onSelect={() => handleSearchMatch(result.igdb_id)}
                        />
                      ))}
                    </div>
                  ) : (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      No games found for &ldquo;{searchQuery}&rdquo;
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>

          <div className="flex justify-end gap-2 border-t pt-4">
            <Button
              variant="outline"
              onClick={handleSkip}
              disabled={isProcessing}
            >
              Skip
            </Button>
            <Button variant="ghost" onClick={() => setShowSearchModal(false)}>
              Cancel
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function StatusSection({
  jobId,
  status,
  count,
  defaultOpen = false,
  isTerminal,
}: StatusSectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const [page, setPage] = useState(1);
  const [processingItemId, setProcessingItemId] = useState<string | null>(null);
  const { data, isLoading, refetch } = useJobItems(jobId, status, page, 20, {
    enabled: isOpen && count > 0,
  });

  // Retry mutations
  const retryAllMutation = useRetryFailedItems();
  const retryItemMutation = useRetryJobItem();

  // Determine section behavior
  const isFailedSection = status === JobItemStatus.FAILED;
  const isPendingReviewSection = status === JobItemStatus.PENDING_REVIEW;
  const canRetry = isFailedSection && isTerminal;

  const handleRetryAll = async () => {
    try {
      const result = await retryAllMutation.mutateAsync(jobId);
      toast.success(result.message);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry items');
    }
  };

  const handleRetryItem = async (itemId: string) => {
    try {
      await retryItemMutation.mutateAsync(itemId);
      toast.success('Item queued for retry');
      refetch();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry item');
    }
  };

  if (count === 0) return null;

  const iconMap: Record<JobItemStatus, React.ReactNode> = {
    [JobItemStatus.PENDING]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.PROCESSING]: (
      <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
    ),
    [JobItemStatus.COMPLETED]: <CheckCircle className="h-4 w-4 text-green-600" />,
    [JobItemStatus.PENDING_REVIEW]: (
      <AlertCircle className="h-4 w-4 text-yellow-600" />
    ),
    [JobItemStatus.SKIPPED]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.FAILED]: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="w-full justify-between px-4 py-2 h-auto"
        >
          <div className="flex items-center gap-2">
            {isOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
            {iconMap[status]}
            <span>{getJobItemStatusLabel(status)}</span>
          </div>
          <div className="flex items-center gap-2">
            {canRetry && (
              <Button
                variant="outline"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  handleRetryAll();
                }}
                disabled={retryAllMutation.isPending}
                className="h-7"
              >
                {retryAllMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <RotateCcw className="h-3 w-3 mr-1" />
                )}
                Retry All
              </Button>
            )}
            <Badge variant={getJobItemStatusVariant(status)}>{count}</Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-l-2 border-muted ml-6 pl-4 py-2 space-y-2">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : (
            <>
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
                    <div className="min-w-0 flex-1">
                      <div className="font-medium truncate">{item.sourceTitle}</div>
                      {item.resultGameTitle && (
                        <div className="text-muted-foreground truncate">
                          &rarr; {item.resultGameTitle}
                        </div>
                      )}
                      {item.errorMessage && (
                        <div className="text-red-600 text-xs mt-1">
                          {item.errorMessage}
                        </div>
                      )}
                    </div>
                    {canRetry && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRetryItem(item.id)}
                        disabled={retryItemMutation.isPending}
                        className="ml-2 h-8"
                      >
                        {retryItemMutation.isPending ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : (
                          <RotateCcw className="h-3 w-3" />
                        )}
                      </Button>
                    )}
                  </div>
                )
              )}
              {data && data.pages > 1 && (
                <div className="flex items-center justify-center gap-2 pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {page} of {data.pages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.min(data.pages, p + 1))}
                    disabled={page >= data.pages}
                  >
                    Next
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function JobItemsDetails({ jobId, progress, isTerminal }: JobItemsDetailsProps) {
  // Order sections: needs review first (action required), then failed, then others
  const sections = [
    {
      status: JobItemStatus.PENDING_REVIEW,
      count: progress.pendingReview,
      defaultOpen: progress.pendingReview > 0,  // Auto-expand when items need review
    },
    { status: JobItemStatus.FAILED, count: progress.failed, defaultOpen: progress.failed > 0 },
    { status: JobItemStatus.PROCESSING, count: progress.processing, defaultOpen: false },
    { status: JobItemStatus.PENDING, count: progress.pending, defaultOpen: false },
    { status: JobItemStatus.COMPLETED, count: progress.completed, defaultOpen: false },
    { status: JobItemStatus.SKIPPED, count: progress.skipped, defaultOpen: false },
  ];

  const hasItems = sections.some((s) => s.count > 0);

  if (!hasItems) {
    return null;
  }

  return (
    <div className="rounded-lg border">
      <div className="border-b p-3">
        <h3 className="font-medium">Item Details</h3>
      </div>
      <div className="divide-y">
        {sections.map(({ status, count, defaultOpen }) => (
          <StatusSection
            key={status}
            jobId={jobId}
            status={status}
            count={count}
            defaultOpen={defaultOpen}
            isTerminal={isTerminal}
          />
        ))}
      </div>
    </div>
  );
}
```

**Step 2: Add missing type for igdbCandidatesJson to JobItem**

Edit `frontend/src/types/job.ts` (or wherever JobItem is defined) to include:

```typescript
export interface JobItem {
  id: string;
  jobId: string;
  itemKey: string;
  sourceTitle: string;
  status: JobItemStatus;
  errorMessage: string | null;
  resultGameTitle: string | null;
  resultIgdbId: number | null;
  createdAt: string;
  processedAt: string | null;
  igdbCandidatesJson?: string;  // Add this field
}
```

**Step 3: Update API transformation to include igdbCandidatesJson**

Edit `frontend/src/api/jobs.ts` - update `JobItemApiResponse` and `transformJobItem`:

```typescript
interface JobItemApiResponse {
  id: string;
  job_id: string;
  item_key: string;
  source_title: string;
  status: string;
  error_message: string | null;
  result_game_title: string | null;
  result_igdb_id: number | null;
  created_at: string;
  processed_at: string | null;
  igdb_candidates_json?: string;  // Add this
}

function transformJobItem(apiItem: JobItemApiResponse): JobItem {
  return {
    id: apiItem.id,
    jobId: apiItem.job_id,
    itemKey: apiItem.item_key,
    sourceTitle: apiItem.source_title,
    status: apiItem.status as JobItemStatus,
    errorMessage: apiItem.error_message,
    resultGameTitle: apiItem.result_game_title,
    resultIgdbId: apiItem.result_igdb_id,
    createdAt: apiItem.created_at,
    processedAt: apiItem.processed_at,
    igdbCandidatesJson: apiItem.igdb_candidates_json,  // Add this
  };
}
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run check`
Expected: PASS (0 errors)

**Step 5: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run test`
Expected: PASS

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add frontend/src/components/jobs/job-items-details.tsx frontend/src/types/job.ts frontend/src/api/jobs.ts
git commit -m "feat(sync): add inline review widgets to progress box"
```

---

## Task 3: Frontend - Remove Separate Review Section from Sync Page

**Files:**
- Modify: `frontend/src/app/(main)/sync/[platform]/page.tsx`

**Step 1: Remove the "Items Needing Review" card**

Edit `frontend/src/app/(main)/sync/[platform]/page.tsx`:

1. Remove the import for `JobItemCard` (no longer needed)
2. Remove state for review functionality: `selectedItem`, `processingItemId`, `searchQuery`
3. Remove the `useJobItems` query for review data (lines 171-178)
4. Remove review handler functions: `handleMatch`, `handleSkip`, `handleView`, `handleModalSkip`, `handleSearchResultMatch`
5. Remove the entire "Review Items Section" card (lines 507-552)
6. Remove the IGDB Candidates Modal (lines 554-623)
7. Remove the `SearchResultItem` helper component (lines 661-706)

The sync page should now only show:
- Navigation header
- Platform header card
- Steam connection card (for Steam)
- Active sync progress (with JobProgressCard + JobItemsDetails)
- Configuration card
- Recent activity card

**Step 2: Clean up unused imports**

Remove imports that are no longer needed after removing the review section.

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run check`
Expected: PASS (0 errors)

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run test`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add frontend/src/app/(main)/sync/[platform]/page.tsx
git commit -m "refactor(sync): remove separate review section, use progress box for all interactions"
```

---

## Task 4: Backend - Add Endpoint to Fetch Recent Completed Jobs with Items

**Files:**
- Modify: `backend/app/api/jobs.py`
- Modify: `backend/app/schemas/job.py`
- Test: `backend/app/tests/test_jobs_api.py`

**Step 1: Add schema for recent jobs with item details**

Edit `backend/app/schemas/job.py` - add new response models:

```python
class JobItemSummary(BaseModel):
    """Summary of a job item for recent activity display."""

    source_title: str
    result_game_title: Optional[str] = None
    result_igdb_id: Optional[int] = None
    error_message: Optional[str] = None
    is_new_addition: bool = False  # True if game was newly added, False if already in library


class RecentJobDetail(BaseModel):
    """Detailed job info for recent activity."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    created_at: datetime
    completed_at: Optional[datetime]
    total_items: int

    # Item counts
    completed_count: int
    skipped_count: int
    failed_count: int

    # Item details by status
    completed_items: List[JobItemSummary]
    skipped_items: List[JobItemSummary]
    failed_items: List[JobItemSummary]


class RecentJobsResponse(BaseModel):
    """Response for recent completed jobs."""

    jobs: List[RecentJobDetail]
```

**Step 2: Add API endpoint for recent jobs**

Edit `backend/app/api/jobs.py` - add endpoint:

```python
@router.get("/recent/{source}", response_model=RecentJobsResponse)
async def get_recent_jobs(
    source: str,
    limit: int = Query(default=5, ge=1, le=20),
    db: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """
    Get recent completed sync jobs for a platform with item details.

    Returns the most recent completed jobs with their items grouped by status.
    """
    # Map source string to enum
    try:
        job_source = BackgroundJobSource(source)
    except ValueError:
        raise HTTPException(status_code=400, detail=f"Invalid source: {source}")

    # Fetch recent completed jobs
    jobs = db.exec(
        select(Job)
        .where(
            Job.user_id == current_user.id,
            Job.source == job_source,
            Job.job_type == BackgroundJobType.SYNC,
            Job.status == BackgroundJobStatus.COMPLETED,
        )
        .order_by(col(Job.completed_at).desc())
        .limit(limit)
    ).all()

    result = []
    for job in jobs:
        # Fetch items grouped by status
        completed_items = db.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.COMPLETED)
        ).all()

        skipped_items = db.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.SKIPPED)
        ).all()

        failed_items = db.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.FAILED)
        ).all()

        result.append(RecentJobDetail(
            id=job.id,
            created_at=job.created_at,
            completed_at=job.completed_at,
            total_items=job.total_items,
            completed_count=len(completed_items),
            skipped_count=len(skipped_items),
            failed_count=len(failed_items),
            completed_items=[
                JobItemSummary(
                    source_title=item.source_title,
                    result_game_title=_get_result_game_title(item),
                    result_igdb_id=item.resolved_igdb_id,
                    is_new_addition=_check_is_new_addition(item),
                )
                for item in completed_items
            ],
            skipped_items=[
                JobItemSummary(source_title=item.source_title)
                for item in skipped_items
            ],
            failed_items=[
                JobItemSummary(
                    source_title=item.source_title,
                    error_message=item.error_message,
                )
                for item in failed_items
            ],
        ))

    return RecentJobsResponse(jobs=result)


def _get_result_game_title(item: JobItem) -> Optional[str]:
    """Extract game title from result JSON."""
    try:
        result = json.loads(item.result_json) if item.result_json else {}
        return result.get("igdb_title") or result.get("game_title")
    except json.JSONDecodeError:
        return None


def _check_is_new_addition(item: JobItem) -> bool:
    """Check if the item was a new addition vs already in library."""
    try:
        result = json.loads(item.result_json) if item.result_json else {}
        # Check result type from processing
        result_type = result.get("result_type", "")
        return result_type in ("auto_imported", "imported_new", "linked_new")
    except json.JSONDecodeError:
        return False
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pyrefly check`
Expected: PASS

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add backend/app/api/jobs.py backend/app/schemas/job.py
git commit -m "feat(api): add endpoint for recent completed jobs with item details"
```

---

## Task 5: Frontend - Add Recent Activity with Expandable Job Details

**Files:**
- Create: `frontend/src/components/sync/recent-activity.tsx`
- Modify: `frontend/src/api/jobs.ts`
- Modify: `frontend/src/hooks/use-jobs.ts`
- Modify: `frontend/src/types/job.ts`
- Modify: `frontend/src/app/(main)/sync/[platform]/page.tsx`

**Step 1: Add types for recent jobs**

Edit `frontend/src/types/job.ts`:

```typescript
export interface JobItemSummary {
  sourceTitle: string;
  resultGameTitle: string | null;
  resultIgdbId: number | null;
  errorMessage: string | null;
  isNewAddition: boolean;
}

export interface RecentJobDetail {
  id: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  completedItems: JobItemSummary[];
  skippedItems: JobItemSummary[];
  failedItems: JobItemSummary[];
}

export interface RecentJobsResponse {
  jobs: RecentJobDetail[];
}
```

**Step 2: Add API function**

Edit `frontend/src/api/jobs.ts`:

```typescript
// Add API response types
interface JobItemSummaryApiResponse {
  source_title: string;
  result_game_title: string | null;
  result_igdb_id: number | null;
  error_message: string | null;
  is_new_addition: boolean;
}

interface RecentJobDetailApiResponse {
  id: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  completed_count: number;
  skipped_count: number;
  failed_count: number;
  completed_items: JobItemSummaryApiResponse[];
  skipped_items: JobItemSummaryApiResponse[];
  failed_items: JobItemSummaryApiResponse[];
}

interface RecentJobsApiResponse {
  jobs: RecentJobDetailApiResponse[];
}

// Add transformation
function transformJobItemSummary(api: JobItemSummaryApiResponse): JobItemSummary {
  return {
    sourceTitle: api.source_title,
    resultGameTitle: api.result_game_title,
    resultIgdbId: api.result_igdb_id,
    errorMessage: api.error_message,
    isNewAddition: api.is_new_addition,
  };
}

function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  return {
    id: api.id,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    totalItems: api.total_items,
    completedCount: api.completed_count,
    skippedCount: api.skipped_count,
    failedCount: api.failed_count,
    completedItems: api.completed_items.map(transformJobItemSummary),
    skippedItems: api.skipped_items.map(transformJobItemSummary),
    failedItems: api.failed_items.map(transformJobItemSummary),
  };
}

// Add API function
export async function getRecentJobs(source: string, limit: number = 5): Promise<RecentJobsResponse> {
  const response = await api.get<RecentJobsApiResponse>(`/jobs/recent/${source}`, {
    params: { limit },
  });
  return {
    jobs: response.jobs.map(transformRecentJob),
  };
}
```

**Step 3: Add hook**

Edit `frontend/src/hooks/use-jobs.ts`:

```typescript
// Add to query keys
export const jobsKeys = {
  // ... existing keys
  recent: (source: string, limit?: number) => [...jobsKeys.all, 'recent', source, limit] as const,
};

// Add hook
export function useRecentJobs(source: string, limit: number = 5, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.recent(source, limit),
    queryFn: () => jobsApi.getRecentJobs(source, limit),
    enabled: options?.enabled !== false,
  });
}
```

**Step 4: Create RecentActivity component**

Create `frontend/src/components/sync/recent-activity.tsx`:

```tsx
'use client';

import { useState } from 'react';
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
  SkipForward,
  AlertCircle,
} from 'lucide-react';
import { useRecentJobs } from '@/hooks';
import type { RecentJobDetail, JobItemSummary } from '@/types';

interface RecentActivityProps {
  platform: string;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function ItemsList({
  items,
  type,
}: {
  items: JobItemSummary[];
  type: 'completed' | 'skipped' | 'failed';
}) {
  const [isOpen, setIsOpen] = useState(false);

  if (items.length === 0) return null;

  const iconMap = {
    completed: <CheckCircle className="h-4 w-4 text-green-600" />,
    skipped: <SkipForward className="h-4 w-4 text-muted-foreground" />,
    failed: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  const labelMap = {
    completed: 'Completed',
    skipped: 'Skipped',
    failed: 'Failed',
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between h-8 px-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            {iconMap[type]}
            <span className="text-sm">{labelMap[type]}</span>
          </div>
          <Badge variant="secondary" className="h-5 text-xs">
            {items.length}
          </Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-6 pl-2 border-l space-y-1 py-1">
          {items.map((item, idx) => (
            <div key={idx} className="text-sm py-1">
              {type === 'completed' && (
                <div>
                  <span className="text-muted-foreground">{item.sourceTitle}</span>
                  {item.resultGameTitle && (
                    <>
                      <span className="mx-1">&rarr;</span>
                      <span className="font-medium">{item.resultGameTitle}</span>
                      {item.resultIgdbId && (
                        <span className="text-muted-foreground ml-1">
                          (IGDB: {item.resultIgdbId})
                        </span>
                      )}
                      <span className="ml-2 text-xs">
                        {item.isNewAddition ? (
                          <Badge variant="outline" className="h-4 text-[10px]">Added</Badge>
                        ) : (
                          <Badge variant="secondary" className="h-4 text-[10px]">Already in library</Badge>
                        )}
                      </span>
                    </>
                  )}
                </div>
              )}
              {type === 'skipped' && (
                <span className="text-muted-foreground">{item.sourceTitle}</span>
              )}
              {type === 'failed' && (
                <div>
                  <span>{item.sourceTitle}</span>
                  {item.errorMessage && (
                    <span className="text-red-600 text-xs ml-2">— {item.errorMessage}</span>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
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
          <Badge variant="outline">{job.totalItems} games processed</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-4 pb-4 space-y-1">
          <ItemsList items={job.completedItems} type="completed" />
          <ItemsList items={job.skippedItems} type="skipped" />
          <ItemsList items={job.failedItems} type="failed" />
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

**Step 5: Update sync page to use RecentActivity**

Edit `frontend/src/app/(main)/sync/[platform]/page.tsx`:

1. Import `RecentActivity` component
2. Replace the old "Recent Activity" card with `<RecentActivity platform={platform} />`

**Step 6: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run check`
Expected: PASS

**Step 7: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run test`
Expected: PASS

**Step 8: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add frontend/src/components/sync/recent-activity.tsx frontend/src/api/jobs.ts frontend/src/hooks/use-jobs.ts frontend/src/types/job.ts frontend/src/app/(main)/sync/[platform]/page.tsx
git commit -m "feat(sync): add expandable recent activity with game details"
```

---

## Task 6: Backend - Store Result Data for Recent Activity Display

**Files:**
- Modify: `backend/app/worker/tasks/sync/process_item.py`

The `result_json` field needs to store the data required for recent activity display:
- `igdb_title`: The matched IGDB game title
- `result_type`: Whether it was a new addition or already in library

**Step 1: Update _complete_job_item to store result data**

Edit `backend/app/worker/tasks/sync/process_item.py`:

In `_auto_import_game` and `_process_with_resolved_id`, update the job item with result data before completing:

```python
async def _auto_import_game(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    igdb_id: int,
    platform_id: str,
    storefront_id: str,
    external_id: str,
    match_result: MatchResult,
    confidence: float,
) -> Dict[str, Any]:
    """Auto-import a high-confidence match."""
    logger.info(f"Auto-importing {match_result.igdb_title} (confidence: {confidence:.2f})")

    # ... existing logic ...

    # Update JobItem with match info AND result data
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.resolved_igdb_id = igdb_id
        job_item.match_confidence = confidence
        # Store result data for recent activity display
        job_item.result_json = json.dumps({
            "igdb_title": match_result.igdb_title,
            "igdb_id": igdb_id,
            "result_type": result,  # "auto_imported" or "auto_linked"
        })
        session.add(job_item)

    return await _complete_job_item(
        session, job_item_id, job_id,
        JobItemStatus.COMPLETED, result
    )
```

Similar update for `_process_with_resolved_id`.

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest -v`
Expected: PASS

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add backend/app/worker/tasks/sync/process_item.py
git commit -m "fix(sync): store result data for recent activity display"
```

---

## Task 7: Final Testing and Cleanup

**Step 1: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass, >80% coverage

**Step 2: Run full frontend test suite**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign/frontend && npm run check && npm run test`
Expected: 0 errors, all tests pass

**Step 3: Manual testing checklist**

- [ ] Start a sync job
- [ ] Verify progress box shows all status categories
- [ ] Verify "Needs Review" section is expanded by default when items exist
- [ ] Verify inline review widgets work (match, search, skip)
- [ ] Verify job does NOT complete until all PENDING_REVIEW items are resolved
- [ ] Verify recent activity shows past jobs with expandable details
- [ ] Verify completed items show Steam title → IGDB title (ID) — Added/Already in library
- [ ] Verify skipped items show Steam title only
- [ ] Verify failed items show Steam title — Error reason

**Step 4: Commit any remaining fixes**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-ui-redesign
git add -A
git commit -m "test: add manual testing verification"
```
