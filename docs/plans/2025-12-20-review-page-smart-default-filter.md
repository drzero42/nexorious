# Review Page Smart Default Filter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Default the review page status filter to "pending" when there are pending items, otherwise show all statuses.

**Architecture:** Add a `useEffect` hook that sets the status filter to "pending" after the summary loads, but only if there are pending items and no explicit status was set via URL.

**Tech Stack:** React hooks, Next.js useSearchParams

---

### Task 1: Add Smart Default Filter Effect

**Files:**
- Modify: `frontend/src/app/(main)/review/page.tsx:108` (after useReviewSummary hook)

**Step 1: Add the useEffect for smart default**

Add this effect after line 108 (after `const { data: summary } = useReviewSummary();`):

```typescript
// Smart default: show pending items if there are any and no explicit status filter
useEffect(() => {
  const statusFromUrl = searchParams.get('status');
  if (!statusFromUrl && summary && summary.totalPending > 0 && filters.status === undefined) {
    setFilters((prev) => ({ ...prev, status: ReviewItemStatus.PENDING }));
  }
}, [summary?.totalPending, searchParams]);
```

**Step 2: Add useEffect to imports**

Modify line 3 to include `useEffect`:

```typescript
import { useState, useCallback, useEffect } from 'react';
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS with no TypeScript errors

**Step 4: Manual verification**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`

Test cases:
1. Navigate to `/review` with pending items → filter shows "Pending"
2. Navigate to `/review` with no pending items → filter shows "All Statuses"
3. Navigate to `/review?status=all` → filter shows "All Statuses"
4. Click "Clear filters" → filter shows "All Statuses"

**Step 5: Commit**

```bash
git add frontend/src/app/(main)/review/page.tsx
git commit -m "feat(review): default to pending status filter when pending items exist"
```

---

Plan complete and saved to `docs/plans/2025-12-20-review-page-smart-default-filter.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?
