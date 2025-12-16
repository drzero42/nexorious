# Discard Import Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to completely discard a Darkadia CSV import awaiting review, deleting the job and all ReviewItems.

**Architecture:** Add `POST /api/jobs/{job_id}/discard` endpoint that validates the job is an import in AWAITING_REVIEW status, then atomically deletes the job and cascaded ReviewItems. Frontend adds a "Discard Import" button with confirmation dialog to the Review Queue page.

**Tech Stack:** FastAPI, SQLModel, Svelte 5, TypeScript

---

## Task 1: Add JobDiscardResponse Schema

**Files:**
- Modify: `backend/app/schemas/job.py:129` (after JobConfirmResponse)

**Step 1: Add the schema**

Add after line 129 (after `JobConfirmResponse`):

```python
class JobDiscardResponse(BaseModel):
    """Response model for discarding an import job."""

    success: bool
    message: str
    deleted_job_id: str
    deleted_review_items: int
```

**Step 2: Verify no syntax errors**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.schemas.job import JobDiscardResponse; print('OK')"`

Expected: `OK`

**Step 3: Commit**

```bash
git add backend/app/schemas/job.py
git commit -m "feat(api): add JobDiscardResponse schema"
```

---

## Task 2: Write Failing Tests for Discard Endpoint

**Files:**
- Modify: `backend/app/tests/test_jobs_api.py` (add new test class at end)

**Step 1: Add test class**

Add at the end of the file:

```python
class TestDiscardImport:
    """Tests for POST /api/jobs/{job_id}/discard endpoint."""

    def test_discard_import_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test successfully discarding an import job awaiting review."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Add some review items
        for i in range(3):
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Test Game {i}",
                status=ReviewItemStatus.PENDING,
            )
            session.add(review_item)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["deleted_job_id"] == job.id
        assert data["deleted_review_items"] == 3
        assert "discarded" in data["message"].lower()

        # Verify job is deleted
        deleted_job = session.get(Job, job.id)
        assert deleted_job is None

        # Verify review items are deleted (cascade)
        remaining_items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job.id)
        ).all()
        assert len(remaining_items) == 0

    def test_discard_job_not_found(self, client, auth_headers):
        """Test discarding non-existent job returns 404."""
        response = client.post(
            "/api/jobs/non-existent-id/discard", headers=auth_headers
        )
        assert response.status_code == 404

    def test_discard_other_users_job(
        self, client, auth_headers, session: Session
    ):
        """Test cannot discard another user's job."""
        # Create a job for a different user
        other_user = User(
            username="otheruser",
            password_hash="hash",
        )
        session.add(other_user)
        session.commit()

        job = Job(
            user_id=other_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 404

    def test_discard_non_import_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cannot discard a sync job (only imports allowed)."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 409
        assert "import" in response.json()["detail"].lower()

    def test_discard_wrong_status_pending(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cannot discard a job in PENDING status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 409
        assert "awaiting" in response.json()["detail"].lower()

    def test_discard_wrong_status_completed(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cannot discard a completed job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 409

    def test_discard_wrong_status_processing(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cannot discard a job still processing."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/discard", headers=auth_headers)
        assert response.status_code == 409
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs_api.py::TestDiscardImport -v`

Expected: All tests FAIL with 404 (endpoint doesn't exist yet)

**Step 3: Commit**

```bash
git add backend/app/tests/test_jobs_api.py
git commit -m "test(api): add failing tests for discard import endpoint"
```

---

## Task 3: Implement Discard Endpoint

**Files:**
- Modify: `backend/app/api/jobs.py:25-34` (add import)
- Modify: `backend/app/api/jobs.py:312` (add endpoint after delete_job)

**Step 1: Update imports**

At line 30, add `JobDiscardResponse` to the imports:

```python
from ..schemas.job import (
    JobResponse,
    JobListResponse,
    JobCancelResponse,
    JobDeleteResponse,
    JobConfirmResponse,
    JobDiscardResponse,
    JobType,
    JobSource,
    JobStatus,
)
```

**Step 2: Add the endpoint**

Add after the `delete_job` function (after line 311):

```python
@router.post("/{job_id}/discard", response_model=JobDiscardResponse)
async def discard_import(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Discard an import job and all associated review items.

    Only works for import jobs in AWAITING_REVIEW status.
    Completely removes the job and all review items from the database.
    """
    logger.info(f"User {current_user.id} requesting to discard import job {job_id}")

    job = session.get(Job, job_id)

    if not job or job.user_id != current_user.id:
        logger.warning(f"Job {job_id} not found for discard")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    if job.job_type != BackgroundJobType.IMPORT:
        logger.warning(f"Cannot discard job {job_id} - not an import job")
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Only import jobs can be discarded",
        )

    if job.status != BackgroundJobStatus.AWAITING_REVIEW:
        logger.warning(
            f"Cannot discard job {job_id} - not awaiting review (status: {job.status})"
        )
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"Cannot discard job - must be awaiting review. Current status: {job.status.value}",
        )

    # Count review items before deletion
    review_item_count = session.exec(
        select(func.count()).select_from(ReviewItem).where(ReviewItem.job_id == job_id)
    ).one()

    # Delete job (cascade will delete review items)
    session.delete(job)
    session.commit()

    logger.info(
        f"Import job {job_id} discarded by user {current_user.id} "
        f"({review_item_count} review items deleted)"
    )

    return JobDiscardResponse(
        success=True,
        message="Import discarded successfully",
        deleted_job_id=job_id,
        deleted_review_items=review_item_count,
    )
```

**Step 3: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_jobs_api.py::TestDiscardImport -v`

Expected: All 7 tests PASS

**Step 4: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/api/jobs.py
git commit -m "feat(api): implement POST /jobs/{id}/discard endpoint"
```

---

## Task 4: Add Frontend Type for Discard Response

**Files:**
- Modify: `frontend/src/lib/types/jobs.ts` (add interface)

**Step 1: Add the interface**

Find the job-related interfaces section and add:

```typescript
export interface JobDiscardResponse {
  success: boolean;
  message: string;
  deleted_job_id: string;
  deleted_review_items: number;
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/lib/types/jobs.ts
git commit -m "feat(frontend): add JobDiscardResponse type"
```

---

## Task 5: Add discardImport Method to Review Store

**Files:**
- Modify: `frontend/src/lib/stores/review.svelte.ts:24` (add export)
- Modify: `frontend/src/lib/stores/review.svelte.ts:450` (add method)

**Step 1: Update type imports and exports**

At the imports (around line 10-20), add `JobDiscardResponse` to the import:

```typescript
import type {
  ReviewItem,
  ReviewItemDetail,
  ReviewListResponse,
  ReviewSummary,
  ReviewCountsByType,
  MatchResponse,
  ReviewFilters,
  PlatformSummaryResponse,
  FinalizeImportResponse,
  JobDiscardResponse
} from '$lib/types/jobs';
```

Update the re-export line (around line 24) to include it:

```typescript
export type { ReviewItem, ReviewItemDetail, ReviewSummary, ReviewCountsByType, ReviewFilters, PlatformSummaryResponse, FinalizeImportResponse, JobDiscardResponse };
```

**Step 2: Add the discardImport method**

Add after the `finalizeImport` method (after line 450, before the closing `};`):

```typescript
    /**
     * Discard an import job and all its review items.
     */
    discardImport: async (jobId: string): Promise<JobDiscardResponse> => {
      try {
        const response = await api.post(`${config.apiUrl}/jobs/${jobId}/discard`, {});
        const data: JobDiscardResponse = await response.json();
        return data;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to discard import';
        state.error = errorMessage;
        throw error;
      }
    },
```

Note: Add a comma after the closing brace of `finalizeImport` if not already present.

**Step 3: Verify TypeScript compiles**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/lib/stores/review.svelte.ts
git commit -m "feat(frontend): add discardImport method to review store"
```

---

## Task 6: Add Discard Button and Confirmation Dialog to Review Page

**Files:**
- Modify: `frontend/src/routes/review/+page.svelte`

**Step 1: Add state variables**

After line 27 (after `isFinalizingImport`), add:

```typescript
	let isDiscardingImport = $state(false);
	let showDiscardConfirmation = $state(false);
```

**Step 2: Add the discard handler function**

After the `handleFinalizeImport` function (around line 233), add:

```typescript
	async function handleDiscardImport() {
		if (!jobIdFromUrl) return;

		isDiscardingImport = true;
		try {
			await review.discardImport(jobIdFromUrl);
			goto('/import/darkadia');
		} catch (e) {
			console.error('Failed to discard import:', e);
		} finally {
			isDiscardingImport = false;
			showDiscardConfirmation = false;
		}
	}
```

**Step 3: Update the finalize banner section**

Find the "Ready to finalize?" section (around lines 459-487). Replace it with:

```svelte
		<!-- Finalize/Discard Import Section (for Darkadia imports) -->
		{#if isDarkadiaImport && jobIdFromUrl}
			<div class="mb-6 flex items-center justify-between bg-indigo-50 dark:bg-indigo-900/20 rounded-lg p-4 border border-indigo-200 dark:border-indigo-800">
				<div>
					<h3 class="text-sm font-medium text-indigo-800 dark:text-indigo-300">
						Ready to finalize?
					</h3>
					<p class="text-sm text-indigo-600 dark:text-indigo-400 mt-1">
						Once you've reviewed all items and mapped platforms, click to add games to your collection.
					</p>
				</div>
				<div class="flex items-center gap-3">
					<button
						type="button"
						class="inline-flex items-center px-4 py-2 border border-red-300 dark:border-red-700 text-sm font-medium rounded-md text-red-700 dark:text-red-400 bg-white dark:bg-gray-800 hover:bg-red-50 dark:hover:bg-red-900/20 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
						disabled={isDiscardingImport || isFinalizingImport}
						onclick={() => (showDiscardConfirmation = true)}
					>
						Discard Import
					</button>
					<button
						type="button"
						class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
						disabled={!canFinalize || isDiscardingImport}
						onclick={handleFinalizeImport}
					>
						{#if isFinalizingImport}
							<svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
								<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
								<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
							</svg>
							Finalizing...
						{:else}
							Finalize Import
						{/if}
					</button>
				</div>
			</div>
		{/if}
```

**Step 4: Add the confirmation dialog**

Add before the closing `</RouteGuard>` tag (around line 715):

```svelte
	<!-- Discard Import Confirmation Dialog -->
	{#if showDiscardConfirmation}
		<div
			class="fixed inset-0 z-50 overflow-y-auto"
			aria-labelledby="discard-modal-title"
			role="dialog"
			aria-modal="true"
		>
			<div class="flex items-end justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
				<div
					class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity"
					aria-hidden="true"
					onclick={() => (showDiscardConfirmation = false)}
					onkeydown={(e) => e.key === 'Escape' && (showDiscardConfirmation = false)}
					role="button"
					tabindex="-1"
				></div>
				<span class="hidden sm:inline-block sm:align-middle sm:h-screen" aria-hidden="true">&#8203;</span>
				<div class="inline-block align-bottom bg-white dark:bg-gray-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-lg sm:w-full">
					<div class="bg-white dark:bg-gray-800 px-4 pt-5 pb-4 sm:p-6">
						<div class="sm:flex sm:items-start">
							<div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 dark:bg-red-900/30 sm:mx-0 sm:h-10 sm:w-10">
								<svg class="h-6 w-6 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true">
									<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
								</svg>
							</div>
							<div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
								<h3 class="text-lg leading-6 font-medium text-gray-900 dark:text-white" id="discard-modal-title">
									Discard Import?
								</h3>
								<div class="mt-2">
									<p class="text-sm text-gray-500 dark:text-gray-400">
										Are you sure you want to discard this import? This will permanently delete all {summary?.total_pending ?? 0} review items. This action cannot be undone.
									</p>
								</div>
							</div>
						</div>
					</div>
					<div class="bg-gray-50 dark:bg-gray-800/50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse gap-3">
						<button
							type="button"
							class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
							onclick={handleDiscardImport}
							disabled={isDiscardingImport}
						>
							{#if isDiscardingImport}
								<svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
									<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
									<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
								</svg>
								Discarding...
							{:else}
								Discard
							{/if}
						</button>
						<button
							type="button"
							class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 dark:border-gray-600 shadow-sm px-4 py-2 bg-white dark:bg-gray-700 text-base font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 sm:mt-0 sm:w-auto sm:text-sm"
							onclick={() => (showDiscardConfirmation = false)}
							disabled={isDiscardingImport}
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}
```

**Step 5: Verify TypeScript and Svelte checks pass**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: No errors

**Step 6: Run frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: All tests pass

**Step 7: Commit**

```bash
git add frontend/src/routes/review/+page.svelte
git commit -m "feat(frontend): add discard import button with confirmation dialog"
```

---

## Task 7: Final Verification

**Step 1: Run full backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing -q`

Expected: All tests pass, coverage >80%

**Step 2: Run full frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test`

Expected: All checks pass, all tests pass

**Step 3: Manual testing (optional)**

1. Start backend: `cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main`
2. Start frontend: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`
3. Upload a Darkadia CSV
4. Navigate to Review Queue with job_id parameter
5. Click "Discard Import" button
6. Verify confirmation dialog appears with item count
7. Click "Discard" and verify redirect to /import/darkadia
8. Verify job and review items are deleted

**Step 4: Sync beads and push**

```bash
bd sync
git push -u origin <branch-name>
```
