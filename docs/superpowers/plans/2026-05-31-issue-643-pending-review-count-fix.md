# Issue #643 — Pending-Review Count Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the job-status guard from `HandlePendingReviewCount` so the nav badge and service-card badge agree with the detail page — both count `pending_review` job items regardless of the parent job's status.

**Architecture:** Single SQL change in `HandlePendingReviewCount` (`internal/api/jobs.go`). One existing test asserts the old (wrong) behaviour and must be updated; a new test documents the correct behaviour before the fix lands.

**Tech Stack:** Go, `uptrace/bun`, `labstack/echo` v5, `testcontainers-go`

---

### Task 0: Create feature branch

- [ ] **Create and switch to the feature branch**

```bash
git checkout -b fix/643-pending-review-count
```

---

### Task 1: Write a failing test for the correct behaviour

**Files:**
- Modify: `internal/api/jobs_test.go`

- [ ] **Add the new test after `TestPendingReviewCount_Deduplicates`**

```go
func TestPendingReviewCount_IncludesTerminalJobItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "prc-terminal")

	// pending_review item under a cancelled job — must be counted
	insertJob(t, testDB, "job-prc-terminal", userID, "sync", "steam", "cancelled")
	insertJobItem(t, testDB, "ji-prc-terminal-1", "job-prc-terminal", userID, "key-t1", "Game T", "pending_review")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1, got %v", resp["pending_review_count"])
	}
	bySource, ok := resp["counts_by_source"].(map[string]any)
	if !ok {
		t.Fatal("expected counts_by_source to be an object")
	}
	if bySource["steam"].(float64) != 1 {
		t.Fatalf("expected counts_by_source.steam=1, got %v", bySource["steam"])
	}
}
```

- [ ] **Run the new test and confirm it fails**

```bash
go test ./internal/api/... -run TestPendingReviewCount_IncludesTerminalJobItems -v
```

Expected: `FAIL` — the current query filters out the cancelled job, returning `pending_review_count=0`.

---

### Task 2: Remove the job-status filter

**Files:**
- Modify: `internal/api/jobs.go:243-246`

- [ ] **Remove the `AND j.status IN (...)` line**

Find the query in `HandlePendingReviewCount` (around line 243). Change:

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

To:

```go
	err := h.db.NewRaw(`
		SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		WHERE ji.user_id = ? AND ji.status = ?
		GROUP BY j.source`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &rows)
```

- [ ] **Run the new test and confirm it passes**

```bash
go test ./internal/api/... -run TestPendingReviewCount_IncludesTerminalJobItems -v
```

Expected: `PASS`

- [ ] **Run the old `ExcludesCancelledJobs` test and confirm it now fails**

```bash
go test ./internal/api/... -run TestPendingReviewCount_ExcludesCancelledJobs -v
```

Expected: `FAIL` — it expects count=1 but now gets count=2 (both the cancelled-job item and the active-job item are counted). This test was asserting the wrong behaviour.

---

### Task 3: Update the obsolete test

**Files:**
- Modify: `internal/api/jobs_test.go`

- [ ] **Rename and rewrite `TestPendingReviewCount_ExcludesCancelledJobs`**

Replace the entire function with:

```go
func TestPendingReviewCount_AllJobStatuses(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-prc-allstatuses")

	// pending_review item under a cancelled job — must be counted
	insertJob(t, testDB, "job-prc-cancelled", userID, "import", "steam", "cancelled")
	insertJobItem(t, testDB, "ji-prc-c1", "job-prc-cancelled", userID, "key-c1", "Game C", "pending_review")

	// pending_review item under an active job — must also be counted
	insertJob(t, testDB, "job-prc-active", userID, "import", "steam", "processing")
	insertJobItem(t, testDB, "ji-prc-a1", "job-prc-active", userID, "key-a1", "Game A", "pending_review")

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 2 {
		t.Fatalf("expected pending_review_count=2, got %v", resp["pending_review_count"])
	}
	bySource, ok := resp["counts_by_source"].(map[string]any)
	if !ok {
		t.Fatal("expected counts_by_source to be an object")
	}
	if bySource["steam"].(float64) != 2 {
		t.Fatalf("expected counts_by_source.steam=2, got %v", bySource["steam"])
	}
}
```

- [ ] **Run all PendingReviewCount tests**

```bash
go test ./internal/api/... -run TestPendingReviewCount -v
```

Expected: all five tests pass (`Empty`, `WithItems`, `AllJobStatuses`, `IncludesTerminalJobItems`, `Deduplicates`).

---

### Task 4: Commit and open PR

- [ ] **Stage and commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "fix(jobs): count pending_review items regardless of parent job status"
```

- [ ] **Push and open PR**

```bash
git push -u origin fix/643-pending-review-count
gh pr create \
  --title "fix(jobs): count pending_review items regardless of parent job status" \
  --body "Closes #643

Removes the \`j.status IN ('pending', 'processing')\` guard from
\`HandlePendingReviewCount\`. A \`pending_review\` job item means the user
must act on that game; whether the parent job is still running is
irrelevant. The nav badge and service-card badge now agree with the
detail page in all cases.

The existing \`TestPendingReviewCount_ExcludesCancelledJobs\` test was
asserting the wrong behaviour and has been renamed and updated to
\`TestPendingReviewCount_AllJobStatuses\`."
```
