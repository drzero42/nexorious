# Page Consolidation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate Jobs, Review, Import/Export, and Sync pages into three focused pages with inline progress and details.

**Architecture:** Remove standalone Jobs and Review pages. Import/Export shows inline progress with expandable item details. Sync becomes overview + per-platform detail pages with inline IGDB review. Add admin-only Maintenance page for seed data and IGDB refresh.

**Tech Stack:** Next.js 16, React 19, TanStack Query, shadcn/ui, FastAPI, SQLModel, TaskIQ

**Design Document:** [2025-12-26-page-consolidation-design.md](2025-12-26-page-consolidation-design.md)

---

## Phase 1: Backend - Job Item Details API

Add endpoints to fetch job items with details (not just counts).

### Task 1.1: Add Job Items List Endpoint Schema

**Files:**
- Modify: `backend/app/schemas/job.py`

**Step 1: Add JobItemResponse schema**

```python
# Add after JobProgress class

class JobItemResponse(BaseModel):
    """Response schema for individual job items."""
    model_config = ConfigDict(from_attributes=True)

    id: str
    job_id: str
    item_key: str
    source_title: str
    status: JobItemStatus
    error_message: str | None = None

    # For completed items - the matched game info
    result_game_title: str | None = None
    result_igdb_id: int | None = None

    created_at: datetime
    processed_at: datetime | None = None


class JobItemListResponse(BaseModel):
    """Paginated list of job items."""
    items: list[JobItemResponse]
    total: int
    page: int
    page_size: int
    pages: int
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: PASS

**Step 3: Commit**

```bash
git add backend/app/schemas/job.py
git commit -m "feat(backend): add JobItemResponse schema for item details"
```

---

### Task 1.2: Add Job Items List Endpoint

**Files:**
- Modify: `backend/app/api/jobs.py`

**Step 1: Add the endpoint**

Add after the `get_job` endpoint:

```python
@router.get("/{job_id}/items", response_model=JobItemListResponse)
async def list_job_items(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    status: JobItemStatus | None = Query(default=None, description="Filter by item status"),
    page: int = Query(default=1, ge=1),
    page_size: int = Query(default=50, ge=1, le=100),
):
    """List items for a job with optional status filter and pagination."""
    # Verify job exists and belongs to user
    job = session.get(Job, job_id)
    if not job:
        raise HTTPException(status_code=404, detail="Job not found")
    if job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    # Build query
    query = select(JobItem).where(JobItem.job_id == job_id)
    if status:
        query = query.where(JobItem.status == status)

    # Count total
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Apply pagination
    query = query.order_by(JobItem.created_at.desc())
    offset = (page - 1) * page_size
    items = session.exec(query.offset(offset).limit(page_size)).all()

    pages = (total + page_size - 1) // page_size if total > 0 else 1

    # Transform items to response
    item_responses = []
    for item in items:
        result_data = json.loads(item.result_json) if item.result_json else {}
        item_responses.append(JobItemResponse(
            id=item.id,
            job_id=item.job_id,
            item_key=item.item_key,
            source_title=item.source_title,
            status=item.status,
            error_message=item.error_message,
            result_game_title=result_data.get("game_title"),
            result_igdb_id=result_data.get("igdb_id"),
            created_at=item.created_at,
            processed_at=item.processed_at,
        ))

    return JobItemListResponse(
        items=item_responses,
        total=total,
        page=page,
        page_size=page_size,
        pages=pages,
    )
```

**Step 2: Add missing imports at top of file**

```python
import json
from app.schemas.job import JobItemResponse, JobItemListResponse, JobItemStatus
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs.py -v`
Expected: PASS (existing tests still pass)

**Step 4: Commit**

```bash
git add backend/app/api/jobs.py
git commit -m "feat(backend): add GET /jobs/{id}/items endpoint for item details"
```

---

### Task 1.3: Simplify Job Cancellation (Remove Delete Step)

Currently: cancel → cancelled state → delete. New: cancel immediately removes job.

**Files:**
- Modify: `backend/app/api/jobs.py`

**Step 1: Modify cancel endpoint to delete job**

Replace the existing `cancel_job` endpoint:

```python
@router.post("/{job_id}/cancel", response_model=JobCancelResponse)
async def cancel_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Cancel and remove a job that is not in terminal state."""
    job = session.get(Job, job_id)
    if not job:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.is_terminal:
        raise HTTPException(
            status_code=400,
            detail=f"Cannot cancel job - already in terminal state: {job.status.value}"
        )

    # Delete the job (cascade deletes items)
    session.delete(job)
    session.commit()

    logger.info(f"Job {job_id} cancelled and deleted by user {current_user.id}")

    return JobCancelResponse(
        success=True,
        message="Job cancelled and removed",
        job=None,  # Job no longer exists
    )
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs.py -v`
Expected: Some tests may need updating if they expect cancelled state

**Step 3: Update any failing tests**

If tests fail because they expect the job to exist after cancel, update them to expect 404 on subsequent fetch.

**Step 4: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/
git commit -m "feat(backend): cancel job now immediately deletes instead of marking cancelled"
```

---

### Task 1.4: Add Active Job Check Endpoint

**Files:**
- Modify: `backend/app/api/jobs.py`

**Step 1: Add endpoint to check for active job by type**

```python
@router.get("/active/{job_type}", response_model=JobResponse | None)
async def get_active_job(
    job_type: JobType,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """Get the active (non-terminal) job for a specific type, if any."""
    job = session.exec(
        select(Job)
        .where(Job.user_id == current_user.id)
        .where(Job.job_type == BackgroundJobType(job_type.value))
        .where(Job.status.not_in([
            BackgroundJobStatus.COMPLETED,
            BackgroundJobStatus.FAILED,
            BackgroundJobStatus.CANCELLED,
        ]))
        .order_by(Job.created_at.desc())
    ).first()

    if not job:
        return None

    return _job_to_response(job, session)
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: PASS

**Step 3: Commit**

```bash
git add backend/app/api/jobs.py
git commit -m "feat(backend): add GET /jobs/active/{job_type} to check for running job"
```

---

## Phase 2: Frontend - API and Hooks for Job Items

### Task 2.1: Add Job Items API Functions

**Files:**
- Modify: `frontend/src/api/jobs.ts`

**Step 1: Add types for job items API response**

```typescript
// Add after existing API response types

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
}

interface JobItemListApiResponse {
  items: JobItemApiResponse[];
  total: number;
  page: number;
  page_size: number;
  pages: number;
}
```

**Step 2: Add transformation function**

```typescript
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
  };
}
```

**Step 3: Add API functions**

```typescript
export async function getJobItems(
  jobId: string,
  status?: JobItemStatus,
  page: number = 1,
  pageSize: number = 50
): Promise<JobItemListResponse> {
  const params: Record<string, string | number> = { page, page_size: pageSize };
  if (status) params.status = status;

  const response = await api.get<JobItemListApiResponse>(
    `/jobs/${jobId}/items`,
    { params }
  );

  return {
    items: response.items.map(transformJobItem),
    total: response.total,
    page: response.page,
    pageSize: response.page_size,
    pages: response.pages,
  };
}

export async function getActiveJob(jobType: JobType): Promise<Job | null> {
  const response = await api.get<JobApiResponse | null>(`/jobs/active/${jobType}`);
  return response ? transformJob(response) : null;
}
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: FAIL (types not defined yet)

---

### Task 2.2: Add Job Item Types

**Files:**
- Modify: `frontend/src/types/jobs.ts`

**Step 1: Add JobItemStatus enum**

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

**Step 2: Add JobItem interface**

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
}

export interface JobItemListResponse {
  items: JobItem[];
  total: number;
  page: number;
  pageSize: number;
  pages: number;
}
```

**Step 3: Add helper function for item status display**

```typescript
export function getJobItemStatusLabel(status: JobItemStatus): string {
  const labels: Record<JobItemStatus, string> = {
    [JobItemStatus.PENDING]: 'Pending',
    [JobItemStatus.PROCESSING]: 'Processing',
    [JobItemStatus.COMPLETED]: 'Completed',
    [JobItemStatus.PENDING_REVIEW]: 'Needs Review',
    [JobItemStatus.SKIPPED]: 'Skipped',
    [JobItemStatus.FAILED]: 'Failed',
  };
  return labels[status];
}

export function getJobItemStatusVariant(
  status: JobItemStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case JobItemStatus.COMPLETED:
      return 'default';
    case JobItemStatus.FAILED:
      return 'destructive';
    case JobItemStatus.PENDING_REVIEW:
      return 'secondary';
    default:
      return 'outline';
  }
}
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/types/jobs.ts frontend/src/api/jobs.ts
git commit -m "feat(frontend): add job item types and API functions"
```

---

### Task 2.3: Add Job Items Hook

**Files:**
- Modify: `frontend/src/hooks/use-jobs.ts`

**Step 1: Add query key for items**

```typescript
// Update jobsKeys object
export const jobsKeys = {
  all: ['jobs'] as const,
  lists: () => [...jobsKeys.all, 'list'] as const,
  list: (filters?: JobFilters, page?: number, perPage?: number) =>
    [...jobsKeys.lists(), { filters, page, perPage }] as const,
  details: () => [...jobsKeys.all, 'detail'] as const,
  detail: (id: string) => [...jobsKeys.details(), id] as const,
  items: (jobId: string, status?: JobItemStatus, page?: number) =>
    [...jobsKeys.detail(jobId), 'items', { status, page }] as const,
  active: (jobType: JobType) => [...jobsKeys.all, 'active', jobType] as const,
};
```

**Step 2: Add useJobItems hook**

```typescript
export function useJobItems(
  jobId: string,
  status?: JobItemStatus,
  page: number = 1,
  pageSize: number = 50,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: jobsKeys.items(jobId, status, page),
    queryFn: () => jobsApi.getJobItems(jobId, status, page, pageSize),
    enabled: options?.enabled !== false && !!jobId,
  });
}
```

**Step 3: Add useActiveJob hook**

```typescript
export function useActiveJob(jobType: JobType, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.active(jobType),
    queryFn: () => jobsApi.getActiveJob(jobType),
    enabled: options?.enabled !== false,
    refetchInterval: (query) => {
      // Poll every 3 seconds if there's an active job
      const job = query.state.data as Job | null;
      return job && !job.isTerminal ? 3000 : false;
    },
  });
}
```

**Step 4: Export from hooks index**

Ensure `useJobItems` and `useActiveJob` are exported from `frontend/src/hooks/index.ts`.

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 6: Commit**

```bash
git add frontend/src/hooks/use-jobs.ts frontend/src/hooks/index.ts
git commit -m "feat(frontend): add useJobItems and useActiveJob hooks"
```

---

## Phase 3: Import/Export Page Redesign

### Task 3.1: Create Job Progress Component

**Files:**
- Create: `frontend/src/components/jobs/job-progress-card.tsx`
- Modify: `frontend/src/components/jobs/index.ts`

**Step 1: Create the component**

```typescript
'use client';

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
                  {job.progress.completed + job.progress.failed + job.progress.skipped} / {job.progress.total} ({job.progress.percent}%)
                </span>
              </div>
              <Progress value={job.progress.percent} />
            </div>
          )}

          {job.progress && (
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
              <div>
                <div className="text-muted-foreground">Completed</div>
                <div className="text-lg font-semibold text-green-600">{job.progress.completed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Failed</div>
                <div className="text-lg font-semibold text-red-600">{job.progress.failed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Processing</div>
                <div className="text-lg font-semibold">{job.progress.processing}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Pending</div>
                <div className="text-lg font-semibold">{job.progress.pending}</div>
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
              Are you sure you want to cancel this job? This will stop processing and remove the job.
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

**Step 2: Export from index**

Add to `frontend/src/components/jobs/index.ts`:

```typescript
export { JobProgressCard } from './job-progress-card';
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/components/jobs/
git commit -m "feat(frontend): add JobProgressCard component for inline progress display"
```

---

### Task 3.2: Create Job Items Details Component

**Files:**
- Create: `frontend/src/components/jobs/job-items-details.tsx`
- Modify: `frontend/src/components/jobs/index.ts`

**Step 1: Create the component**

```typescript
'use client';

import { useState } from 'react';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { ChevronDown, ChevronRight, AlertCircle, CheckCircle, Clock, Loader2 } from 'lucide-react';
import { useJobItems } from '@/hooks';
import type { JobItemStatus } from '@/types';
import { getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';

interface JobItemsDetailsProps {
  jobId: string;
  defaultExpandedStatus?: JobItemStatus;
}

interface StatusSectionProps {
  jobId: string;
  status: JobItemStatus;
  count: number;
  defaultOpen?: boolean;
}

function StatusSection({ jobId, status, count, defaultOpen = false }: StatusSectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const [page, setPage] = useState(1);
  const { data, isLoading } = useJobItems(jobId, status, page, 20, { enabled: isOpen });

  if (count === 0) return null;

  const icon = {
    pending: <Clock className="h-4 w-4" />,
    processing: <Loader2 className="h-4 w-4 animate-spin" />,
    completed: <CheckCircle className="h-4 w-4 text-green-600" />,
    pending_review: <AlertCircle className="h-4 w-4 text-yellow-600" />,
    skipped: <Clock className="h-4 w-4 text-muted-foreground" />,
    failed: <AlertCircle className="h-4 w-4 text-red-600" />,
  }[status];

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            {icon}
            <span>{getJobItemStatusLabel(status)}</span>
          </div>
          <Badge variant={getJobItemStatusVariant(status)}>{count}</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-l-2 border-muted ml-6 pl-4 py-2 space-y-2">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          ) : (
            <>
              {data?.items.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center justify-between rounded-md border p-2 text-sm"
                >
                  <div className="min-w-0 flex-1">
                    <div className="font-medium truncate">{item.sourceTitle}</div>
                    {item.resultGameTitle && (
                      <div className="text-muted-foreground truncate">
                        → {item.resultGameTitle}
                      </div>
                    )}
                    {item.errorMessage && (
                      <div className="text-red-600 text-xs mt-1">{item.errorMessage}</div>
                    )}
                  </div>
                </div>
              ))}
              {data && data.pages > 1 && (
                <div className="flex justify-center gap-2 pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    Previous
                  </Button>
                  <span className="flex items-center text-sm text-muted-foreground">
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

export function JobItemsDetails({ jobId, defaultExpandedStatus }: JobItemsDetailsProps) {
  // We need the job progress to know counts per status
  // This component expects to be rendered alongside JobProgressCard which has job data
  // For now, we'll fetch items for each status section independently

  const statuses: JobItemStatus[] = [
    'failed' as JobItemStatus,
    'processing' as JobItemStatus,
    'pending' as JobItemStatus,
    'completed' as JobItemStatus,
    'skipped' as JobItemStatus,
  ];

  return (
    <div className="rounded-lg border">
      <div className="border-b p-3">
        <h3 className="font-medium">Item Details</h3>
      </div>
      <div className="divide-y">
        {statuses.map((status) => (
          <StatusSection
            key={status}
            jobId={jobId}
            status={status}
            count={0} // Will need to pass from parent
            defaultOpen={status === defaultExpandedStatus || status === ('failed' as JobItemStatus)}
          />
        ))}
      </div>
    </div>
  );
}
```

**Note:** This component needs refinement - it requires job progress counts to be passed in. We'll improve this in the next task.

**Step 2: Export from index**

Add to `frontend/src/components/jobs/index.ts`:

```typescript
export { JobItemsDetails } from './job-items-details';
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/components/jobs/
git commit -m "feat(frontend): add JobItemsDetails component with expandable status sections"
```

---

### Task 3.3: Refactor JobItemsDetails to Accept Progress

**Files:**
- Modify: `frontend/src/components/jobs/job-items-details.tsx`

**Step 1: Update component to accept progress prop**

```typescript
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
}

export function JobItemsDetails({ jobId, progress }: JobItemsDetailsProps) {
  const sections = [
    { status: 'failed' as JobItemStatus, count: progress.failed, defaultOpen: progress.failed > 0 },
    { status: 'processing' as JobItemStatus, count: progress.processing, defaultOpen: false },
    { status: 'pending' as JobItemStatus, count: progress.pending, defaultOpen: false },
    { status: 'completed' as JobItemStatus, count: progress.completed, defaultOpen: false },
    { status: 'pending_review' as JobItemStatus, count: progress.pendingReview, defaultOpen: false },
    { status: 'skipped' as JobItemStatus, count: progress.skipped, defaultOpen: false },
  ];

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
          />
        ))}
      </div>
    </div>
  );
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/components/jobs/job-items-details.tsx
git commit -m "refactor(frontend): JobItemsDetails now receives progress counts as prop"
```

---

### Task 3.4: Redesign Import/Export Page

**Files:**
- Modify: `frontend/src/app/(main)/import-export/page.tsx`

**Step 1: Rewrite the page with new structure**

This is a significant rewrite. The new page has three states: idle, active, completed.

```typescript
'use client';

import { useRef, useState } from 'react';
import Link from 'next/link';
import { toast } from 'sonner';
import {
  useActiveJob,
  useImportNexorious,
  useExportCollection,
  useCancelJob,
  useDownloadExport,
} from '@/hooks';
import {
  ImportSource,
  ExportFormat,
  JobType,
  JobStatus,
  getImportSourceDisplayInfo,
  getExportFormatDisplayInfo,
} from '@/types';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import {
  AlertCircle,
  Upload,
  Download,
  FileJson,
  FileSpreadsheet,
  Check,
  Loader2,
  RefreshCw,
  CheckCircle,
  XCircle,
} from 'lucide-react';
import { JobProgressCard, JobItemsDetails } from '@/components/jobs';

// ... (keep existing ImportCard and ExportCard components but add disabled prop)

interface ImportCardProps {
  source: ImportSource;
  onFileSelect: (file: File) => void;
  isUploading: boolean;
  disabled?: boolean;
}

// Update ImportCard to handle disabled state
function ImportCard({ source, onFileSelect, isUploading, disabled }: ImportCardProps) {
  // ... existing implementation with disabled handling
  const fileInputRef = useRef<HTMLInputElement>(null);
  const info = getImportSourceDisplayInfo(source);

  const handleButtonClick = () => {
    if (!disabled) fileInputRef.current?.click();
  };

  // ... rest of implementation, add disabled to button
}

interface ExportCardProps {
  format: ExportFormat;
  onExport: () => void;
  isExporting: boolean;
  disabled?: boolean;
}

// Update ExportCard similarly

export default function ImportExportPage() {
  const [isUploading, setIsUploading] = useState(false);
  const [exportingFormat, setExportingFormat] = useState<ExportFormat | null>(null);
  const [showCompleted, setShowCompleted] = useState(false);
  const [completedJobId, setCompletedJobId] = useState<string | null>(null);

  // Check for active import or export job
  const { data: activeImportJob, isLoading: loadingImport } = useActiveJob(JobType.IMPORT);
  const { data: activeExportJob, isLoading: loadingExport } = useActiveJob(JobType.EXPORT);

  const activeJob = activeImportJob || activeExportJob;
  const isLoading = loadingImport || loadingExport;

  const { mutateAsync: importNexorious } = useImportNexorious();
  const { mutateAsync: exportCollection } = useExportCollection();
  const cancelMutation = useCancelJob();
  const downloadMutation = useDownloadExport();

  const hasActiveJob = !!activeJob && !activeJob.isTerminal;
  const isJobCompleted = activeJob?.isTerminal;

  const handleImportFile = async (file: File) => {
    setIsUploading(true);
    try {
      await importNexorious(file);
      toast.success('Import started');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Import failed');
    } finally {
      setIsUploading(false);
    }
  };

  const handleExport = async (format: ExportFormat) => {
    setExportingFormat(format);
    try {
      await exportCollection(format);
      toast.success('Export started');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Export failed');
    } finally {
      setExportingFormat(null);
    }
  };

  const handleCancel = async () => {
    if (!activeJob) return;
    try {
      await cancelMutation.mutateAsync(activeJob.id);
      toast.success('Job cancelled');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to cancel');
    }
  };

  const handleDownload = async () => {
    if (!activeJob) return;
    try {
      await downloadMutation.mutateAsync(activeJob.id);
      toast.success('Download started');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Download failed');
    }
  };

  const handleDismissCompleted = () => {
    // Clear the completed job from view
    setShowCompleted(false);
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Import / Export</span>
        </nav>
        <h1 className="text-2xl font-bold">Import / Export</h1>
        <p className="text-muted-foreground">
          Import your game collection or export your data for backup.
        </p>
      </div>

      {/* Active Job Progress */}
      {activeJob && (
        <section className="mb-8 space-y-4">
          <JobProgressCard
            job={activeJob}
            onCancel={handleCancel}
            isCancelling={cancelMutation.isPending}
          />

          {activeJob.progress && (
            <JobItemsDetails
              jobId={activeJob.id}
              progress={activeJob.progress}
            />
          )}

          {/* Completed job actions */}
          {activeJob.isTerminal && (
            <div className="flex gap-2">
              {activeJob.jobType === JobType.EXPORT &&
               activeJob.status === JobStatus.COMPLETED && (
                <Button onClick={handleDownload} disabled={downloadMutation.isPending}>
                  {downloadMutation.isPending ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Download className="mr-2 h-4 w-4" />
                  )}
                  Download Export
                </Button>
              )}
              <Button variant="outline" onClick={handleDismissCompleted}>
                <RefreshCw className="mr-2 h-4 w-4" />
                Start New
              </Button>
            </div>
          )}
        </section>
      )}

      {/* Import/Export Cards - only show when no active job */}
      {!activeJob && (
        <>
          {/* Import Section */}
          <section className="mb-8">
            <h2 className="mb-4 text-lg font-semibold">Import Games</h2>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <ImportCard
                source={ImportSource.NEXORIOUS}
                onFileSelect={handleImportFile}
                isUploading={isUploading}
                disabled={hasActiveJob}
              />
            </div>
          </section>

          {/* Export Section */}
          <section className="mb-8">
            <h2 className="mb-4 text-lg font-semibold">Export</h2>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <ExportCard
                format={ExportFormat.JSON}
                onExport={() => handleExport(ExportFormat.JSON)}
                isExporting={exportingFormat === ExportFormat.JSON}
                disabled={hasActiveJob}
              />
              <ExportCard
                format={ExportFormat.CSV}
                onExport={() => handleExport(ExportFormat.CSV)}
                isExporting={exportingFormat === ExportFormat.CSV}
                disabled={hasActiveJob}
              />
            </div>
          </section>

          {/* Info Alert */}
          <Alert className="mb-6">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>About Import / Export</AlertTitle>
            <AlertDescription>
              <p className="mb-2">
                <strong>Nexorious JSON</strong> is the recommended format for backups. It preserves all
                metadata including IGDB IDs, ratings, notes, and platform associations.
              </p>
              <p>
                <strong>CSV exports</strong> are useful for spreadsheet analysis but are not
                recommended for re-import due to potential data loss.
              </p>
            </AlertDescription>
          </Alert>
        </>
      )}

      {/* TODO: Add Recent Activity section showing last 7 days of jobs */}
    </div>
  );
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS or minor fixes needed

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: Some tests may need updating

**Step 4: Commit**

```bash
git add frontend/src/app/(main)/import-export/page.tsx
git commit -m "feat(frontend): redesign Import/Export page with inline progress and details"
```

---

## Phase 4: Sync Pages Redesign

### Task 4.1: Create Sync Overview Page

Refactor `/sync` to be an overview with cards linking to detail pages.

**Files:**
- Modify: `frontend/src/app/(main)/sync/page.tsx`

(Implementation details similar to Phase 3 - show storefront cards with status, link to detail pages)

---

### Task 4.2: Create Sync Detail Page

**Files:**
- Create: `frontend/src/app/(main)/sync/[platform]/page.tsx`

(Platform-specific configuration, progress, review items)

---

### Task 4.3: Move Review Items to Sync Detail Page

Integrate review functionality from the Review page into the Sync detail page.

---

## Phase 5: Maintenance Page (Admin)

### Task 5.1: Create Maintenance Page

**Files:**
- Create: `frontend/src/app/(main)/admin/maintenance/page.tsx`

---

### Task 5.2: Add Seed Data API Endpoint

**Files:**
- Create: `backend/app/api/admin/maintenance.py`

---

### Task 5.3: Add IGDB Refresh Job Type

**Files:**
- Modify: `backend/app/models/job.py`
- Create: `backend/app/tasks/igdb_refresh.py`

---

## Phase 6: Navigation & Cleanup

### Task 6.1: Update Navigation Items

**Files:**
- Modify: `frontend/src/components/navigation/nav-items.tsx`

Remove Jobs and Review from Manage section. Add Maintenance to Admin section.

---

### Task 6.2: Add Redirect Pages

**Files:**
- Create: `frontend/src/app/(main)/jobs/page.tsx` (redirect to /import-export)
- Create: `frontend/src/app/(main)/review/page.tsx` (redirect to /sync)

---

### Task 6.3: Remove Old Pages

**Files:**
- Delete: `frontend/src/app/(main)/jobs/[id]/page.tsx`
- Delete: Old review page components

---

### Task 6.4: Update Tests

Update all affected tests to reflect new page structure and behavior.

---

## Summary

This plan is organized into 6 phases:

1. **Backend - Job Item Details API** (Tasks 1.1-1.4)
2. **Frontend - API and Hooks** (Tasks 2.1-2.3)
3. **Import/Export Page Redesign** (Tasks 3.1-3.4)
4. **Sync Pages Redesign** (Tasks 4.1-4.3)
5. **Maintenance Page** (Tasks 5.1-5.3)
6. **Navigation & Cleanup** (Tasks 6.1-6.4)

Each task follows TDD where applicable, with explicit file paths, code snippets, and commit points.
