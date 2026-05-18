# Needs Review Deduplication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** De-duplicate the Needs Review list and badge count so the same game title appears only once, regardless of how many platform SKUs (e.g. PS4 + PS5) are in `pending_review`.

**Architecture:** Two SQL-only changes in `internal/api/jobs.go`. For `HandleGetJobItems`, the `pending_review` list uses `DISTINCT ON (source_title)` and `COUNT(DISTINCT source_title)` instead of the generic Bun query builder path. For `HandlePendingReviewCount`, `COUNT(*)` becomes `COUNT(DISTINCT ji.source_title)`. No schema changes. No frontend changes. No dispatch changes.

**Tech Stack:** Go, PostgreSQL (Bun ORM for non-pending_review paths, raw SQL for pending_review paths), Echo v5, testcontainers-go for tests.

---

### Task 1: Deduplicate the Needs Review list (`HandleGetJobItems`)

**Files:**
- Modify: `internal/api/jobs.go` (the `HandleGetJobItems` method — count query and list query for `pending_review`)
- Test: `internal/api/jobs_test.go`

- [ ] **Step 1: Create feature branch**

```bash
git checkout -b fix/needs-review-deduplication
```

- [ ] **Step 2: Write the failing test**

Open `internal/api/jobs_test.go` and add this function at the end of the file (after `TestHandleGetJobItems_SortsPendingReviewAlphabetically`):

```go
func TestHandleGetJobItems_DeduplicatesPendingReview(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "items-dedup")

	insertJob(t, testDB, "job-dedup", userID, "sync", "psn", "processing")
	// Same source_title, different item_key (PS4 and PS5 SKUs of the same game).
	insertJobItem(t, testDB, "ji-dedup-ps4", "job-dedup", userID, "CUSA12345_00", "Call of Duty", "pending_review")
	insertJobItem(t, testDB, "ji-dedup-ps5", "job-dedup", userID, "PPSA07890_00", "Call of Duty", "pending_review")
	// A distinct title to verify other items still appear.
	insertJobItem(t, testDB, "ji-dedup-other", "job-dedup", userID, "CUSA99999_00", "Other Game", "pending_review")

	rec := getAuth(t, e, "/api/jobs/job-dedup/items?status=pending_review", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total"].(float64) != 2 {
		t.Fatalf("expected total=2 (deduplicated), got %v", resp["total"])
	}
	items := resp["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items (deduplicated), got %d", len(items))
	}
	// Items must be sorted alphabetically.
	titles := []string{
		items[0].(map[string]any)["source_title"].(string),
		items[1].(map[string]any)["source_title"].(string),
	}
	if titles[0] != "Call of Duty" || titles[1] != "Other Game" {
		t.Errorf("expected [Call of Duty, Other Game], got %v", titles)
	}
}
```

- [ ] **Step 3: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestHandleGetJobItems_DeduplicatesPendingReview -v
```

Expected: FAIL — `expected total=2 (deduplicated), got 3` (current query returns all 3 items).

- [ ] **Step 4: Implement the fix in `HandleGetJobItems`**

In `internal/api/jobs.go`, find the block inside `HandleGetJobItems` that starts with:

```go
	total, err := countQ.Count(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
	}

	offset := (page - 1) * perPage
	var items []models.JobItem
	sortExpr := "created_at ASC"
	if c.QueryParam("status") == "pending_review" {
		sortExpr = "source_title ASC"
	}
	err = q.OrderExpr(sortExpr).Limit(perPage).Offset(offset).
		Scan(context.Background(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
	}
```

Replace it with:

```go
	var total int
	if c.QueryParam("status") == "pending_review" {
		if err := h.db.NewRaw(
			`SELECT COUNT(DISTINCT source_title) FROM job_items WHERE job_id = ? AND status = 'pending_review'`,
			jobID,
		).Scan(context.Background(), &total); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
		}
	} else {
		var err error
		total, err = countQ.Count(context.Background())
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to count job items")
		}
	}

	offset := (page - 1) * perPage
	var items []models.JobItem
	if c.QueryParam("status") == "pending_review" {
		if err := h.db.NewRaw(
			`SELECT DISTINCT ON (source_title) * FROM job_items WHERE job_id = ? AND status = 'pending_review' ORDER BY source_title ASC LIMIT ? OFFSET ?`,
			jobID, perPage, offset,
		).Scan(context.Background(), &items); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
		}
	} else {
		if err := q.OrderExpr("created_at ASC").Limit(perPage).Offset(offset).Scan(context.Background(), &items); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list job items")
		}
	}
```

- [ ] **Step 5: Run the full `api` test suite to confirm everything passes**

```bash
go test ./internal/api/... -v -timeout 600s
```

Expected: all tests pass, including `TestHandleGetJobItems_DeduplicatesPendingReview` and the existing `TestHandleGetJobItems_SortsPendingReviewAlphabetically`.

- [ ] **Step 6: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "fix(api): deduplicate pending_review list by source_title"
```

---

### Task 2: Deduplicate the badge count (`HandlePendingReviewCount`)

**Files:**
- Modify: `internal/api/jobs.go` (the `HandlePendingReviewCount` method — one word change in the raw SQL)
- Test: `internal/api/jobs_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/api/jobs_test.go` and add this function after `TestPendingReviewCount_ExcludesCancelledJobs`:

```go
func TestPendingReviewCount_Deduplicates(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "prc-dedup")

	insertJob(t, testDB, "job-prc-dedup", userID, "sync", "psn", "processing")
	// Same source_title, different item_key (PS4 and PS5 SKUs of the same game).
	insertJobItem(t, testDB, "ji-prc-dedup-1", "job-prc-dedup", userID, "CUSA12345_00", "Call of Duty", "pending_review")
	insertJobItem(t, testDB, "ji-prc-dedup-2", "job-prc-dedup", userID, "PPSA07890_00", "Call of Duty", "pending_review")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1 (deduplicated), got %v", resp["pending_review_count"])
	}
	bySource := resp["counts_by_source"].(map[string]any)
	if bySource["psn"].(float64) != 1 {
		t.Fatalf("expected counts_by_source.psn=1 (deduplicated), got %v", bySource["psn"])
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestPendingReviewCount_Deduplicates -v
```

Expected: FAIL — `expected pending_review_count=1 (deduplicated), got 2`.

- [ ] **Step 3: Implement the fix in `HandlePendingReviewCount`**

In `internal/api/jobs.go`, inside `HandlePendingReviewCount`, find the raw SQL string:

```go
	err := h.db.NewRaw(`
		SELECT j.source, COUNT(*) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		WHERE ji.user_id = ? AND ji.status = ?
		  AND j.status IN ('pending', 'processing')
		GROUP BY j.source`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &rows)
```

Change `COUNT(*)` to `COUNT(DISTINCT ji.source_title)`:

```go
	err := h.db.NewRaw(`
		SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		WHERE ji.user_id = ? AND ji.status = ?
		  AND j.status IN ('pending', 'processing')
		GROUP BY j.source`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &rows)
```

- [ ] **Step 4: Run the full `api` test suite to confirm everything passes**

```bash
go test ./internal/api/... -v -timeout 600s
```

Expected: all tests pass, including `TestPendingReviewCount_Deduplicates` and the existing `TestPendingReviewCount_WithItems`.

- [ ] **Step 5: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "fix(api): deduplicate pending_review badge count by source_title"
```
