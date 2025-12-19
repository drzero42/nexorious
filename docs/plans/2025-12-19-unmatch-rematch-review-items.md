# Unmatch/Rematch Review Items Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to unmatch resolved review items and trigger rematch with different match criteria, enabling correction of incorrect automatic matches.

**Architecture:** Add API endpoints for unmatch/rematch operations, update ReviewItem model to track rematch history, and add UI controls to the review queue for resolved items.

**Tech Stack:** FastAPI, SQLModel, React (frontend-next), TanStack Query

**Related Issues:** nexorious-0ppx

---

## Overview

When review items are auto-matched or manually resolved, users may later realize the match was incorrect. This feature allows users to:
1. **Unmatch** - Clear the current match and return the item to pending status
2. **Rematch** - Trigger a new automatic match attempt with potentially different parameters

---

## Phase 1: Backend API

### Task 1.1: Add Unmatch Endpoint

**Files:**
- Modify: `backend/app/api/routes/review_queue.py`
- Modify: `backend/app/services/review_queue.py`

**Step 1: Add unmatch service method**

Add to `backend/app/services/review_queue.py`:

```python
async def unmatch_review_item(
    session: AsyncSession,
    item_id: str,
    user_id: str,
) -> ReviewItem:
    """
    Clear the match from a resolved review item, returning it to pending status.

    Args:
        session: Database session
        item_id: The review item ID
        user_id: The user performing the action (for audit)

    Returns:
        Updated ReviewItem

    Raises:
        HTTPException: If item not found or not in resolved status
    """
    item = await session.get(ReviewItem, item_id)
    if not item:
        raise HTTPException(status_code=404, detail="Review item not found")

    if item.status != ReviewItemStatus.RESOLVED:
        raise HTTPException(
            status_code=400,
            detail="Can only unmatch resolved items"
        )

    # Clear match data
    item.matched_game_id = None
    item.similarity_score = None
    item.status = ReviewItemStatus.PENDING
    item.resolved_at = None
    item.resolved_by = None

    await session.commit()
    await session.refresh(item)
    return item
```

**Step 2: Add unmatch API endpoint**

Add to `backend/app/api/routes/review_queue.py`:

```python
@router.post("/items/{item_id}/unmatch", response_model=ReviewItemResponse)
async def unmatch_review_item(
    item_id: str,
    session: AsyncSession = Depends(get_session),
    current_user: User = Depends(get_current_user),
) -> ReviewItemResponse:
    """Unmatch a resolved review item, returning it to pending status."""
    item = await review_queue_service.unmatch_review_item(
        session=session,
        item_id=item_id,
        user_id=current_user.id,
    )
    return ReviewItemResponse.model_validate(item)
```

**Verification:**
```bash
uv run pytest backend/app/tests/test_review_queue.py -v -k unmatch
```

---

### Task 1.2: Add Rematch Endpoint

**Files:**
- Modify: `backend/app/api/routes/review_queue.py`
- Modify: `backend/app/services/review_queue.py`
- Modify: `backend/app/services/game_matching.py` (if exists)

**Step 1: Add rematch service method**

Add to `backend/app/services/review_queue.py`:

```python
async def rematch_review_item(
    session: AsyncSession,
    item_id: str,
    user_id: str,
    search_query: str | None = None,
) -> ReviewItem:
    """
    Trigger a new match attempt for a review item.

    Args:
        session: Database session
        item_id: The review item ID
        user_id: The user performing the action
        search_query: Optional custom search query (defaults to original title)

    Returns:
        Updated ReviewItem with new match results
    """
    item = await session.get(ReviewItem, item_id)
    if not item:
        raise HTTPException(status_code=404, detail="Review item not found")

    # Use custom query or fall back to original title
    query = search_query or item.source_title

    # Perform IGDB search and matching
    match_result = await game_matching_service.find_best_match(
        session=session,
        query=query,
        user_id=user_id,
    )

    if match_result:
        item.matched_game_id = match_result.game_id
        item.similarity_score = match_result.similarity_score
        item.status = ReviewItemStatus.PENDING  # User still needs to confirm
    else:
        item.matched_game_id = None
        item.similarity_score = None
        item.status = ReviewItemStatus.NO_MATCH

    await session.commit()
    await session.refresh(item)
    return item
```

**Step 2: Add rematch API endpoint**

```python
class RematchRequest(BaseModel):
    search_query: str | None = None

@router.post("/items/{item_id}/rematch", response_model=ReviewItemResponse)
async def rematch_review_item(
    item_id: str,
    request: RematchRequest | None = None,
    session: AsyncSession = Depends(get_session),
    current_user: User = Depends(get_current_user),
) -> ReviewItemResponse:
    """Trigger a new match attempt for a review item."""
    item = await review_queue_service.rematch_review_item(
        session=session,
        item_id=item_id,
        user_id=current_user.id,
        search_query=request.search_query if request else None,
    )
    return ReviewItemResponse.model_validate(item)
```

**Verification:**
```bash
uv run pytest backend/app/tests/test_review_queue.py -v -k rematch
```

---

## Phase 2: Frontend API Layer

### Task 2.1: Add API Functions

**Files:**
- Modify: `frontend-next/src/api/review.ts` (or create if needed)

**Step 1: Add unmatch/rematch API calls**

```typescript
export async function unmatchReviewItem(itemId: string): Promise<ReviewItem> {
  const response = await apiClient.post<ReviewItem>(
    `/api/review/items/${itemId}/unmatch`
  );
  return response.data;
}

export interface RematchRequest {
  search_query?: string;
}

export async function rematchReviewItem(
  itemId: string,
  request?: RematchRequest
): Promise<ReviewItem> {
  const response = await apiClient.post<ReviewItem>(
    `/api/review/items/${itemId}/rematch`,
    request
  );
  return response.data;
}
```

---

### Task 2.2: Add React Query Mutations

**Files:**
- Modify: `frontend-next/src/hooks/use-review-queue.ts` (or create)

**Step 1: Add mutation hooks**

```typescript
export function useUnmatchReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: unmatchReviewItem,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reviewItems'] });
    },
  });
}

export function useRematchReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, request }: { itemId: string; request?: RematchRequest }) =>
      rematchReviewItem(itemId, request),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reviewItems'] });
    },
  });
}
```

---

## Phase 3: Frontend UI

### Task 3.1: Add Unmatch/Rematch Controls to Review Item Card

**Files:**
- Modify: `frontend-next/src/app/(main)/review/page.tsx` (or component)

**Step 1: Add action buttons for resolved items**

Add to the review item card/row component:

```tsx
{item.status === 'resolved' && (
  <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button variant="ghost" size="icon">
        <MoreHorizontal className="h-4 w-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem onClick={() => handleUnmatch(item.id)}>
        <Undo className="mr-2 h-4 w-4" />
        Unmatch
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => openRematchDialog(item)}>
        <RefreshCw className="mr-2 h-4 w-4" />
        Rematch with different search
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
)}
```

**Step 2: Add rematch dialog**

```tsx
<Dialog open={rematchDialogOpen} onOpenChange={setRematchDialogOpen}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Rematch "{selectedItem?.source_title}"</DialogTitle>
      <DialogDescription>
        Enter a custom search query or leave blank to use the original title.
      </DialogDescription>
    </DialogHeader>
    <div className="space-y-4">
      <Input
        placeholder="Custom search query (optional)"
        value={rematchQuery}
        onChange={(e) => setRematchQuery(e.target.value)}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={() => setRematchDialogOpen(false)}>
          Cancel
        </Button>
        <Button onClick={handleRematch} disabled={rematchMutation.isPending}>
          {rematchMutation.isPending ? 'Searching...' : 'Search'}
        </Button>
      </div>
    </div>
  </DialogContent>
</Dialog>
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Phase 4: Testing

### Task 4.1: Backend Unit Tests

**Files:**
- Create/Modify: `backend/app/tests/test_review_queue.py`

**Tests to add:**
- `test_unmatch_resolved_item` - Verify unmatch clears match and resets status
- `test_unmatch_pending_item_fails` - Verify can't unmatch non-resolved items
- `test_unmatch_not_found` - Verify 404 for invalid item
- `test_rematch_with_custom_query` - Verify custom search works
- `test_rematch_uses_original_title` - Verify fallback to original title

### Task 4.2: Frontend Component Tests

**Files:**
- Create: `frontend-next/src/app/(main)/review/__tests__/unmatch-rematch.test.tsx`

**Tests to add:**
- Unmatch button only shows for resolved items
- Clicking unmatch calls API and invalidates cache
- Rematch dialog opens with correct item
- Rematch with custom query calls API correctly

---

## Acceptance Criteria

- [ ] Users can unmatch resolved review items
- [ ] Unmatched items return to pending status
- [ ] Users can trigger rematch with optional custom search query
- [ ] UI shows unmatch/rematch options only for resolved items
- [ ] All backend tests pass
- [ ] All frontend tests pass
- [ ] `uv run pytest` passes
- [ ] `npm run check && npm run test` passes
