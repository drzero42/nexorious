# Discard Import Feature Design

**Date:** 2025-12-16
**Status:** Approved

## Overview

Allow users to completely discard a Darkadia CSV import that's awaiting review, deleting the job and all associated ReviewItems as if the import never happened.

## Key Decisions

| Decision | Choice |
|----------|--------|
| Action type | Single atomic operation (discard = cancel + delete) |
| API endpoint | `POST /api/jobs/{job_id}/discard` |
| Allowed job types | Import jobs only |
| Allowed job states | `AWAITING_REVIEW` only |
| Error responses | 404 (not found/not owned), 409 (wrong type/state) |
| UI location | Review Queue page, in finalize banner |
| Confirmation | Simple dialog with item count |
| Post-discard redirect | `/import/darkadia` |

## API Design

### Endpoint

**`POST /api/jobs/{job_id}/discard`**

Atomically discards an import job and all associated review items.

### Request

No request body required.

### Response (200 OK)

```json
{
    "success": true,
    "message": "Import discarded successfully",
    "deleted_job_id": "abc-123",
    "deleted_review_items": 142
}
```

### Error Responses

**404 Not Found**
- Job does not exist
- Job not owned by current user

**409 Conflict**
- Job is not an import job (`job_type != IMPORT`)
- Job is not in `AWAITING_REVIEW` status

## Frontend Design

### UI Location

Review Queue page (`/review?job_id=X`), in the existing "Ready to finalize?" banner.

### Layout

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Ready to finalize?                                                     │
│  Once you've reviewed all items and mapped platforms...                 │
│                                                                         │
│                                    [Discard Import]  [Finalize Import]  │
└─────────────────────────────────────────────────────────────────────────┘
```

### Button Styling

- "Discard Import" - outlined/secondary style with red/destructive color
- "Finalize Import" - solid primary style (unchanged)

### Confirmation Dialog

- **Title:** "Discard Import?"
- **Body:** "Are you sure you want to discard this import? This will permanently delete all X review items."
- **Buttons:** "Cancel" (secondary), "Discard" (red/destructive)

### Post-Discard Behavior

Redirect to `/import/darkadia` (the upload page).

## Implementation

### Files to Modify

| File | Change |
|------|--------|
| `backend/app/api/jobs.py` | Add `POST /{job_id}/discard` endpoint |
| `backend/app/schemas/job.py` | Add `JobDiscardResponse` schema |
| `frontend/src/lib/stores/review.svelte.ts` | Add `discardImport(jobId)` method |
| `frontend/src/routes/review/+page.svelte` | Add button, confirmation dialog, handler |

### Backend Implementation

```python
# backend/app/schemas/job.py
class JobDiscardResponse(BaseModel):
    success: bool
    message: str
    deleted_job_id: str
    deleted_review_items: int
```

```python
# backend/app/api/jobs.py
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
    job = session.get(Job, job_id)

    if not job or job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.job_type != BackgroundJobType.IMPORT:
        raise HTTPException(
            status_code=409,
            detail="Only import jobs can be discarded"
        )

    if job.status != BackgroundJobStatus.AWAITING_REVIEW:
        raise HTTPException(
            status_code=409,
            detail=f"Cannot discard job - must be awaiting review. Current status: {job.status.value}"
        )

    # Count review items before deletion
    review_item_count = session.exec(
        select(func.count()).where(ReviewItem.job_id == job_id)
    ).one()

    # Delete job (cascade will delete review items)
    session.delete(job)
    session.commit()

    return JobDiscardResponse(
        success=True,
        message="Import discarded successfully",
        deleted_job_id=job_id,
        deleted_review_items=review_item_count,
    )
```

### Frontend Implementation

```typescript
// frontend/src/lib/stores/review.svelte.ts
async discardImport(jobId: string): Promise<{ success: boolean; deleted_review_items: number }> {
    const response = await fetch(`${API_BASE}/jobs/${jobId}/discard`, {
        method: 'POST',
        headers: getAuthHeaders(),
    });

    if (!response.ok) {
        const error = await response.json();
        throw new Error(error.detail || 'Failed to discard import');
    }

    return response.json();
}
```

## Testing

### Backend Tests

1. **Success case** - Discard import job in AWAITING_REVIEW status
2. **404 - Job not found** - Non-existent job ID
3. **404 - Not owned** - Job owned by different user
4. **409 - Wrong job type** - Attempt to discard sync job
5. **409 - Wrong status** - Attempt to discard PENDING/PROCESSING/COMPLETED job
6. **Cascade deletion** - Verify ReviewItems are deleted with job

### Frontend Tests

1. **Button visibility** - Only shown for Darkadia import jobs
2. **Confirmation dialog** - Opens on button click, shows correct item count
3. **Cancel flow** - Dialog closes, no action taken
4. **Discard flow** - API called, redirects to /import/darkadia
5. **Error handling** - Shows error message on API failure
