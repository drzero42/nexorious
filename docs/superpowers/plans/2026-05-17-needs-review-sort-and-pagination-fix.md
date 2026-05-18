# Needs Review: Alphabetical Sort + Pagination Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sort "Needs Review" job items alphabetically by title and fix broken pagination that prevented users from seeing items beyond the first 20.

**Architecture:** One backend change (conditional sort order in `HandleGetJobItems`) and one frontend change (fix mismatched JSON field names and query param between the API client and the backend). Re-sorting on new arrivals is already handled by the existing `useEffect` refetch-on-count-change in `StatusSection`.

**Tech Stack:** Go + Bun ORM (backend), TypeScript + TanStack Query (frontend)

---

## Files

- Modify: `internal/api/jobs.go` — change sort to `source_title ASC` for `pending_review`
- Modify: `internal/api/jobs_test.go` — add test for alphabetical sort
- Modify: `ui/frontend/src/api/jobs.ts` — fix `JobItemListApiResponse` field names and query param

---

### Task 1: Create feature branch

- [ ] **Step 1: Create and switch to branch**

```bash
git checkout -b fix/needs-review-sort-pagination
```

---

### Task 2: Backend — sort pending_review items alphabetically (TDD)

**Files:**
- Test: `internal/api/jobs_test.go`
- Modify: `internal/api/jobs.go:469`

- [ ] **Step 1: Write the failing test**

Open `internal/api/jobs_test.go` and add this function after the existing `TestHandleGetJobItems_*` tests:

```go
func TestHandleGetJobItems_SortsPendingReviewAlphabetically(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "items-sort")

	insertJob(t, testDB, "job-sort", userID, "import", "steam", "processing")
	// Insert in reverse alphabetical order to prove sorting isn't by insertion order.
	insertJobItem(t, testDB, "ji-sort-z", "job-sort", userID, "key-z", "Zebra Game", "pending_review")
	insertJobItem(t, testDB, "ji-sort-a", "job-sort", userID, "key-a", "Apple Game", "pending_review")
	insertJobItem(t, testDB, "ji-sort-m", "job-sort", userID, "key-m", "Mango Game", "pending_review")

	rec := getAuth(t, e, "/api/jobs/job-sort/items?status=pending_review", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	items := resp["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	want := []string{"Apple Game", "Mango Game", "Zebra Game"}
	for i, title := range want {
		got := items[i].(map[string]any)["source_title"].(string)
		if got != title {
			t.Errorf("position %d: got %q, want %q", i, got, title)
		}
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestHandleGetJobItems_SortsPendingReviewAlphabetically -v
```

Expected: FAIL — items will be in insertion order (Zebra, Apple, Mango), not alphabetical.

- [ ] **Step 3: Implement the sort**

In `internal/api/jobs.go`, find the `HandleGetJobItems` handler. The relevant section currently reads:

```go
err = q.OrderExpr("created_at ASC").Limit(perPage).Offset(offset).
    Scan(context.Background(), &items)
```

Replace that single statement with:

```go
sortExpr := "created_at ASC"
if c.QueryParam("status") == "pending_review" {
    sortExpr = "source_title ASC"
}
err = q.OrderExpr(sortExpr).Limit(perPage).Offset(offset).
    Scan(context.Background(), &items)
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
go test ./internal/api/... -run TestHandleGetJobItems_SortsPendingReviewAlphabetically -v
```

Expected: PASS

- [ ] **Step 5: Run the full Go test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "fix(sync): sort pending_review job items alphabetically by title"
```

---

### Task 3: Frontend — fix API field name mismatches

**Files:**
- Modify: `ui/frontend/src/api/jobs.ts` — three changes: interface, query param, mapping

There are two mismatches between what the backend sends and what the frontend expects:

| Direction | Backend | Frontend (wrong) | Fix |
|-----------|---------|-----------------|-----|
| Response field | `total_pages` | `pages` in `JobItemListApiResponse` | rename to `total_pages` |
| Response field | `per_page` | `page_size` in `JobItemListApiResponse` | rename to `per_page` |
| Query param sent | backend reads `per_page` | frontend sends `page_size` | send `per_page` |

Because `pages` is always `undefined`, the pagination buttons (`data.pages > 1`) never render. No component changes are needed — the UI is already correct once real data arrives.

- [ ] **Step 1: Fix `JobItemListApiResponse` interface**

In `ui/frontend/src/api/jobs.ts`, find the `JobItemListApiResponse` interface (around line 99) which currently reads:

```ts
interface JobItemListApiResponse {
  items: JobItemApiResponse[];
  total: number;
  page: number;
  page_size: number;
  pages: number;
}
```

Change it to:

```ts
interface JobItemListApiResponse {
  items: JobItemApiResponse[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}
```

- [ ] **Step 2: Fix the query param and response mapping in `getJobItems`**

Find the `getJobItems` function (around line 329). The current params and return value read:

```ts
const params: Record<string, string | number> = { page, page_size: pageSize };
// ...
return {
  items: response.items.map(transformJobItem),
  total: response.total,
  page: response.page,
  pageSize: response.page_size,
  pages: response.pages,
};
```

Change to:

```ts
const params: Record<string, string | number> = { page, per_page: pageSize };
// ...
return {
  items: response.items.map(transformJobItem),
  total: response.total,
  page: response.page,
  pageSize: response.per_page,
  pages: response.total_pages,
};
```

- [ ] **Step 3: Type-check the frontend**

```bash
cd ui/frontend && npm run check
```

Expected: zero TypeScript errors.

- [ ] **Step 4: Run frontend tests**

```bash
cd ui/frontend && npm run test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/jobs.ts
git commit -m "fix(sync): fix job items API field name and query param mismatches

Backend sends total_pages/per_page; frontend was reading pages/page_size.
Pagination buttons were never rendered because data.pages was always undefined."
```

---

### Task 4: Open PR

- [ ] **Step 1: Push branch and open PR**

```bash
git push -u origin fix/needs-review-sort-pagination
gh pr create \
  --title "fix(sync): sort Needs Review items alphabetically + fix pagination" \
  --body "$(cat <<'EOF'
## Summary

- Sort `pending_review` job items by `source_title ASC` (was `created_at ASC`)
- Fix mismatched field names in frontend API client (`total_pages`/`per_page` vs `pages`/`page_size`) that caused pagination buttons to never render
- Re-sorting on new arrivals is free — existing `useEffect` refetch-on-count-change in `StatusSection` handles it

## Test plan

- [ ] Verify Go tests pass: `go test ./internal/api/... -run TestHandleGetJobItems`
- [ ] Verify full suite passes: `go test ./...`
- [ ] Verify TypeScript: `cd ui/frontend && npm run check`
- [ ] Manually: trigger a sync with >20 unmatched games; confirm Needs Review shows Next/Previous buttons and items are alphabetically ordered across pages

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
