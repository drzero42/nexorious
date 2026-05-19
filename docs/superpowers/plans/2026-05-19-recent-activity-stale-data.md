# Recent Activity Stale Data Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix three bugs that cause Recent Activity sections on the Sync, Import/Export, and Maintenance pages to show empty or stale data after a background job completes.

**Architecture:** Three independent, targeted edits — no new files, no new abstractions, no API shape changes. Bug 1 wires a `useEffect` in the sync page to invalidate the recent-jobs query when the active job turns terminal. Bug 2 corrects a one-line prop on the import/export page. Bug 3 replaces a hardcoded zero-progress map in the Go handler with actual per-job item counts.

**Tech Stack:** React 19 / TanStack Query (frontend), Go / Bun / Echo v5 (backend), testcontainers-go + stdlib testing (backend tests).

---

## Files Modified

| File | Change |
|------|--------|
| `ui/frontend/src/routes/_authenticated/sync/$platform.tsx` | Add `invalidatedJobRef` + `useEffect` after `activeJob` declaration |
| `ui/frontend/src/routes/_authenticated/import-export.tsx` | Fix `excludeJobIds` prop (line ~440) |
| `internal/api/jobs.go` | Replace `emptyProgress` loop in `HandleListJobs` with `jobItemCounts` calls |
| `internal/api/jobs_test.go` | Add `TestListJobs_ProgressCounts` test |

---

## Task 1: Bug 3 — Write the failing test for HandleListJobs progress counts

**Files:**
- Modify: `internal/api/jobs_test.go`

- [ ] **Step 1: Add the test at the bottom of the `TestListJobs` block area**

Open `internal/api/jobs_test.go`. After the closing brace of `TestListJobs` (around line 109), add the following test. It inserts a completed job with two completed items and one failed item, then asserts that `HandleListJobs` returns non-zero counts in the `progress` field.

```go
// ─── TestListJobs_ProgressCounts ─────────────────────────────────────────────

func TestListJobs_ProgressCounts(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	userID, token := setupTagUser(t, testDB, e, "jobs-progress")

	jobID := uuid.New().String()
	insertJob(t, testDB, jobID, userID, "import", "steam", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-2", "Game B", "completed")
	insertJobItem(t, testDB, uuid.New().String(), jobID, userID, "key-3", "Game C", "failed")

	rec := getAuth(t, e, "/api/jobs", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	jobs, ok := resp["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %v", resp["jobs"])
	}

	job, ok := jobs[0].(map[string]any)
	if !ok {
		t.Fatalf("job is not an object: %v", jobs[0])
	}

	progress, ok := job["progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress missing or wrong type: %v", job["progress"])
	}

	if got := progress["completed"].(float64); got != 2 {
		t.Errorf("expected progress.completed=2, got %v", got)
	}
	if got := progress["failed"].(float64); got != 1 {
		t.Errorf("expected progress.failed=1, got %v", got)
	}
	if got := progress["total"].(float64); got != 3 {
		t.Errorf("expected progress.total=3, got %v", got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestListJobs_ProgressCounts -v -timeout 120s
```

Expected: FAIL — `expected progress.completed=2, got 0` (the current code returns zeros).

---

## Task 2: Bug 3 — Fix HandleListJobs to return real progress counts

**Files:**
- Modify: `internal/api/jobs.go` (lines ~174–186)

- [ ] **Step 1: Replace the `emptyProgress` block**

In `internal/api/jobs.go`, find the following block near the end of `HandleListJobs`:

```go
	emptyProgress := map[string]any{
		"pending": 0, "processing": 0, "completed": 0,
		"pending_review": 0, "skipped": 0, "failed": 0, "igdb_failed": 0,
		"total": 0, "percent": 0,
	}
	jobDTOs := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], emptyProgress))
	}
```

Replace it with:

```go
	jobDTOs := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		progress := h.jobItemCounts(context.Background(), jobs[i].ID)
		jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], progress))
	}
```

- [ ] **Step 2: Run the test to confirm it passes**

```bash
go test ./internal/api/... -run TestListJobs_ProgressCounts -v -timeout 120s
```

Expected: PASS.

- [ ] **Step 3: Run the full test suite to check for regressions**

```bash
go test ./internal/api/... -timeout 600s
```

Expected: all tests pass.

- [ ] **Step 4: Build to confirm no compile errors**

```bash
make build
```

Expected: binary produced with no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "fix: populate progress counts in HandleListJobs response"
```

---

## Task 3: Bug 2 — Fix excludeJobIds on import/export page

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx` (line ~440)

- [ ] **Step 1: Find and replace the excludeJobIds prop**

In `ui/frontend/src/routes/_authenticated/import-export.tsx`, find the `<RecentActivity>` call. It currently passes:

```tsx
excludeJobIds={[activeImportJob?.id, activeExportJob?.id].filter((id): id is string => !!id)}
```

Replace with:

```tsx
excludeJobIds={activeJob ? [activeJob.id] : []}
```

`activeJob` is already computed from the `getActiveJob()` function earlier in the component (around line 233). It returns `null` when the active job has been dismissed, so this expression produces an empty array after dismissal — matching the pattern used on the maintenance page.

- [ ] **Step 2: Type-check the frontend**

From `ui/frontend/`:

```bash
npm run check
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/import-export.tsx
git commit -m "fix: exclude dismissed job from recent activity on import/export page"
```

---

## Task 4: Bug 1 — Invalidate recent jobs query when sync job goes terminal

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$platform.tsx` (after line ~161)

- [ ] **Step 1: Add the ref and effect after the activeJob declaration**

In `$platform.tsx`, find this block (around line 158):

```tsx
  // Fetch job details if there's an active job
  const { data: activeJob } = useJob(status?.activeJobId ?? undefined, {
    enabled: !!status?.activeJobId,
  });
```

Immediately after it, add:

```tsx
  const invalidatedJobRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (activeJob?.isTerminal && activeJob.id !== invalidatedJobRef.current) {
      invalidatedJobRef.current = activeJob.id;
      queryClient.invalidateQueries({ queryKey: jobsKeys.recent(platform) });
    }
  }, [activeJob?.isTerminal, activeJob?.id, platform, queryClient]);
```

`useRef`, `useEffect`, `queryClient`, `jobsKeys`, and `platform` are all already available in scope (`useRef` and `useEffect` are imported at line 1; `queryClient` is already used in the file; `jobsKeys` is imported from `@/hooks`; `platform` comes from the route params).

The `invalidatedJobRef` prevents the effect from firing more than once per job ID — safe if the component re-renders while the job is already terminal (e.g., from cache on mount).

- [ ] **Step 2: Type-check and lint**

From `ui/frontend/`:

```bash
npm run check
```

Expected: no errors.

- [ ] **Step 3: Run dead-code check**

```bash
npm run knip
```

Expected: no new findings.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/\$platform.tsx
git commit -m "fix: invalidate recent activity when sync job becomes terminal"
```

---

## Task 5: Final verification

- [ ] **Step 1: Run Go tests**

```bash
go test -timeout 600s ./...
```

Expected: all pass.

- [ ] **Step 2: Run frontend checks**

From `ui/frontend/`:

```bash
npm run check && npm run knip && npm run test
```

Expected: no errors, no knip findings, all tests pass.

- [ ] **Step 3: Run Go linter**

```bash
golangci-lint run
```

Expected: no new lint errors.
