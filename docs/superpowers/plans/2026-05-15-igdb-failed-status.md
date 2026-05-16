# IGDB Failed Status + Auto-Retry + UI Retry Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a distinct `igdb_failed` job-item status for IGDB API errors (separate from `pending_review`), trigger a one-time automatic retry on job completion, and surface a "Retry IGDB errors" button in the Recent Activity UI.

**Architecture:** `igdb_failed` is a terminal item status set when `SearchGames` returns an error. When all non-`pending`/`processing`/`pending_review` items are settled, `syncCheckJobCompletion` checks for `igdb_failed` items: if `auto_retry_done=false` it resets them to `pending` and re-enqueues, otherwise it marks the job `completed_with_errors`. The Recent Activity panel is also fixed to return items split by status (was a pre-existing bug — backend returned a flat `items` array but frontend expected split arrays).

**Tech Stack:** Go 1.25 (River, Bun, Echo v5), React 19 (TanStack Query, shadcn/ui), PostgreSQL

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/db/models/jobs.go` | Add `JobItemStatusIGDBFailed` constant |
| Modify | `internal/worker/tasks/sync.go` | `syncMarkItemIGDBFailed`, IGDB error path, `RiverClient` on worker, auto-retry in `syncCheckJobCompletion` |
| Modify | `internal/worker/tasks/sync_test.go` | Tests for igdb_failed marking and auto-retry |
| Modify | `internal/api/jobs.go` | Update `jobItemCounts`, `HandleRetryFailed`, `HandleRecentJobs` |
| Modify | `internal/api/jobs_test.go` | Tests for API changes |
| Modify | `ui/frontend/src/types/jobs.ts` | `IGDB_FAILED` enum value, `igdbFailed` in `JobProgress`, `igdbFailedItems` in `RecentJobDetail` |
| Modify | `ui/frontend/src/api/jobs.ts` | `igdb_failed` in API response types + transform |
| Modify | `ui/frontend/src/components/sync/recent-activity.tsx` | `igdb_failed` item list + retry button |

---

### Task 1: Add `igdb_failed` status constant + worker mark function + IGDB error path

**Files:**
- Modify: `internal/db/models/jobs.go`
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/sync_test.go` after the `TestProcessSyncItem_WithIGDBAutoResolve` test:

```go
func TestProcessSyncItem_IGDBError_MarksItemIGDBFailed(t *testing.T) {
	// IGDB server that returns 503 to trigger a search error.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '123', 'Some Game', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '123', 'Some Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient}
	job := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "igdb_failed" {
		t.Errorf("expected item status=igdb_failed, got %q", status)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/worker/tasks/... -run TestProcessSyncItem_IGDBError_MarksItemIGDBFailed -v -timeout 120s
```

Expected: FAIL — either compilation error (igdb_failed constant missing) or status assertion fails (item stays `pending` because the error path only logs a warning currently).

- [ ] **Step 3: Add `JobItemStatusIGDBFailed` constant**

In `internal/db/models/jobs.go`, add the constant to the existing block (lines ~95-101):

```go
const (
	JobItemStatusPending       = "pending"
	JobItemStatusProcessing    = "processing"
	JobItemStatusCompleted     = "completed"
	JobItemStatusPendingReview = "pending_review"
	JobItemStatusSkipped       = "skipped"
	JobItemStatusFailed        = "failed"
	JobItemStatusIGDBFailed    = "igdb_failed"
)
```

- [ ] **Step 4: Add `syncMarkItemIGDBFailed` and update IGDB error path in `sync.go`**

Add `syncMarkItemIGDBFailed` alongside the other mark functions (after `syncMarkItemFailed`):

```go
// syncMarkItemIGDBFailed sets a job_item to igdb_failed for IGDB API errors.
func syncMarkItemIGDBFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusIGDBFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemIGDBFailed", "id", item.ID, "err", err)
	}
}
```

Also add `RiverClient *river.Client[pgx.Tx]` field to `ProcessSyncItemWorker` (needed in Task 2):

```go
type ProcessSyncItemWorker struct {
	river.WorkerDefaults[ProcessSyncItemArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}
```

Change the IGDB error path inside `ProcessSyncItemWorker.Work` (step 5, IGDB resolution). Currently:

```go
candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
if err != nil {
    slog.Warn("process_sync_item: igdb search failed", "title", eg.Title, "err", err)
} else {
```

Replace with:

```go
candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
if err != nil {
    msg := fmt.Sprintf("igdb search failed: %v", err)
    syncMarkItemIGDBFailed(ctx, w.DB, &item, msg)
    syncCheckJobCompletion(ctx, w.DB, item.JobID)
    return nil
}
```

Then the remaining IGDB logic starts directly (remove the surrounding `else` block, it's no longer needed — the `if err != nil` block returns early):

```go
normalizedQuery := matching.NormalizeTitle(eg.Title)
var bestScore float64
var bestID int32
for _, candidate := range candidates {
    score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(candidate.Title))
    if score > bestScore {
        bestScore = score
        bestID = int32(candidate.IgdbID)
    }
}
if bestScore >= 0.85 {
    // ... auto-resolve path (unchanged)
} else {
    // ... pending_review path (unchanged)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/worker/tasks/... -run TestProcessSyncItem_IGDBError_MarksItemIGDBFailed -v -timeout 120s
```

Expected: PASS

- [ ] **Step 6: Run full tasks package tests to check for regressions**

```bash
go test ./internal/worker/tasks/... -timeout 300s
```

Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add internal/db/models/jobs.go internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(sync): add igdb_failed item status and mark on IGDB API errors"
```

---

### Task 2: Auto-retry in `syncCheckJobCompletion` + `RiverClient` wire-through

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Add two tests to `internal/worker/tasks/sync_test.go`:

```go
func TestSyncCheckJobCompletion_AutoRetry_ResetsIGDBFailed(t *testing.T) {
	// When all items are settled and igdb_failed items exist with auto_retry_done=false,
	// syncCheckJobCompletion should reset them to pending and set auto_retry_done=true.
	// Job should NOT be marked completed yet.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 2, false)`,
		jobID, userID,
	)

	// One completed item, one igdb_failed item.
	itemID1 := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]')`,
		itemID1, jobID, userID,
	)
	itemID2 := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key2', 'Game B', '{}', 'igdb_failed', '{}', '[]')`,
		itemID2, jobID, userID,
	)

	// Call syncCheckJobCompletion via worker (simulate last item completing).
	// We call it directly via the exported wrapper — but syncCheckJobCompletion is unexported.
	// Run the worker on the completed item to trigger the check indirectly,
	// or use a helper that calls it directly. Since the function is unexported,
	// we use the package-internal test file (helpers_test.go is in package tasks).
	// Instead, we trigger it by running ProcessSyncItemWorker on a third item that completes.
	// Simpler: directly manipulate DB and call checkCompletion via running a worker.
	//
	// For this test, we trigger completion by adding a third item in 'pending' state,
	// then running the worker on it (item not found since we didn't wire an external_game),
	// which marks it failed and calls syncCheckJobCompletion.
	//
	// Actually the cleanest approach: make this test internal (package tasks, not tasks_test).
	// See helpers_test.go which is "package tasks". We'll add this test there instead.
	// (See step 3 below — these tests go in a new internal_sync_test.go file.)
	t.Skip("see internal_sync_test.go for the direct test")
}
```

Actually, since `syncCheckJobCompletion` is unexported, the auto-retry test must be in the `package tasks` internal test. Add a new file `internal/worker/tasks/internal_sync_test.go`:

```go
package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSyncCheckJobCompletion_AutoRetry_ResetsIGDBFailed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 2, false)`,
		jobID, userID,
	)

	// One completed item, one igdb_failed item.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]')`,
		uuid.NewString(), jobID, userID,
	)
	igdbFailedItemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key2', 'Game B', '{}', 'igdb_failed', '{}', '[]')`,
		igdbFailedItemID, jobID, userID,
	)

	// First call: auto_retry_done=false → reset igdb_failed items, set auto_retry_done=true.
	syncCheckJobCompletion(ctx, testDB, nil, jobID)

	var igdbItem struct {
		Status string
	}
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, igdbFailedItemID).Scan(ctx, &igdbItem)
	if igdbItem.Status != "pending" {
		t.Errorf("expected igdb_failed item reset to pending after first check, got %q", igdbItem.Status)
	}

	var autoRetryDone bool
	_ = testDB.NewRaw(`SELECT auto_retry_done FROM jobs WHERE id = ?`, jobID).Scan(ctx, &autoRetryDone)
	if !autoRetryDone {
		t.Error("expected auto_retry_done=true after first check")
	}

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "processing" {
		t.Errorf("expected job still processing after reset, got %q", jobStatus)
	}
}

func TestSyncCheckJobCompletion_AutoRetry_CompletesWithErrorsOnSecondFailure(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 2, true)`,
		jobID, userID,
	)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]')`,
		uuid.NewString(), jobID, userID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key2', 'Game B', '{}', 'igdb_failed', '{}', '[]')`,
		uuid.NewString(), jobID, userID,
	)

	// Second call: auto_retry_done=true → mark completed_with_errors.
	syncCheckJobCompletion(ctx, testDB, nil, jobID)

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "completed_with_errors" {
		t.Errorf("expected job completed_with_errors, got %q", jobStatus)
	}
}

func TestSyncCheckJobCompletion_AllCompleted_MarksCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false)`,
		jobID, userID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]')`,
		uuid.NewString(), jobID, userID,
	)

	syncCheckJobCompletion(ctx, testDB, nil, jobID)

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "completed" {
		t.Errorf("expected job completed, got %q", jobStatus)
	}
}
```

Note: `internal_sync_test.go` is in `package tasks` (not `tasks_test`) so it can access unexported `syncCheckJobCompletion` and also uses `testDB` from `testmain_test.go` which is also in `package tasks`.

Wait — `testmain_test.go` is in `package tasks_test`. The `testDB` variable is only visible to external test files. For internal test files (`package tasks`), a separate mechanism is needed.

**Fix:** Put the tests in `helpers_test.go` which is `package tasks` — BUT `testDB` is declared in `testmain_test.go` which is `package tasks_test`. Internal test files cannot access `testDB` from `testmain_test.go`.

**Alternative:** Write a `TestMain` in a new internal test file. But Go only allows one `TestMain` per package.

**Correct approach:** Change the test to use the exported worker `Work()` method as the trigger for `syncCheckJobCompletion`, rather than calling it directly. We simulate the completion check by running a ProcessSyncItemWorker on the last item.

Rewrite as an external test in `sync_test.go` that exercises the full worker flow:

- [ ] **Step 2: Remove the placeholder test added in Step 1, write actual tests in `sync_test.go`**

Remove any placeholder from Step 1. Instead add these to `sync_test.go` (`package tasks_test`), which trigger the check indirectly via the worker:

```go
func TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors(t *testing.T) {
	// Scenario: 1 igdb_failed item, auto_retry_done=false.
	// Running a ProcessSyncItemWorker on the igdb_failed item (which we re-mark as pending
	// and which will fail again due to IGDB error) drives the full auto-retry cycle.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, auto_retry_done)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false)`,
		jobID, userID,
	)

	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '999', 'Retry Game', false, true, false, 0)`,
		egID, userID,
	)

	metaJSON, _ := json.Marshal(map[string]string{"external_game_id": egID, "raw_platform": "PC"})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Retry Game', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	rc := newTestRiverClient(t)
	w := &tasks.ProcessSyncItemWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	riverJob := &river.Job[tasks.ProcessSyncItemArgs]{
		Args: tasks.ProcessSyncItemArgs{JobItemID: itemID},
	}

	// First run: item → igdb_failed, auto_retry triggers (resets to pending, sets auto_retry_done=true).
	if err := w.Work(ctx, riverJob); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}

	var itemStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus)

	var autoRetryDone bool
	_ = testDB.NewRaw(`SELECT auto_retry_done FROM jobs WHERE id = ?`, jobID).Scan(ctx, &autoRetryDone)

	// After first run: item is igdb_failed → completion check → auto_retry resets it to pending,
	// auto_retry_done becomes true, job stays processing.
	if itemStatus != "pending" {
		t.Errorf("expected item reset to pending after auto-retry, got %q", itemStatus)
	}
	if !autoRetryDone {
		t.Error("expected auto_retry_done=true after first completion check")
	}

	var jobStatus string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "processing" {
		t.Errorf("expected job still processing after auto-retry, got %q", jobStatus)
	}

	// Second run: item → igdb_failed again, auto_retry_done=true → job completed_with_errors.
	if err := w.Work(ctx, riverJob); err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "completed_with_errors" {
		t.Errorf("expected job completed_with_errors after retry exhausted, got %q", jobStatus)
	}
}
```

- [ ] **Step 3: Run test to confirm it fails**

```bash
go test ./internal/worker/tasks/... -run TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors -v -timeout 120s
```

Expected: FAIL — compilation error (RiverClient field missing or syncCheckJobCompletion signature mismatch).

- [ ] **Step 4: Update `syncCheckJobCompletion` signature and add auto-retry logic**

Replace the existing `syncCheckJobCompletion` function in `internal/worker/tasks/sync.go`:

```go
// syncCheckJobCompletion counts job_items still in a non-terminal state for the job.
// If none remain, it checks for igdb_failed items:
//   - If igdb_failed items exist and auto_retry_done=false: resets them to pending,
//     re-enqueues River jobs, and sets auto_retry_done=true (job stays processing).
//   - If igdb_failed items exist and auto_retry_done=true: marks job completed_with_errors.
//   - Otherwise: marks job completed.
//
// pending_review items still block completion (they require user action).
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobID string) {
	var remaining int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'processing', 'pending_review')`,
		jobID,
	).Scan(ctx, &remaining); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion count", "job_id", jobID, "err", err)
		return
	}
	if remaining > 0 {
		return
	}

	// Check for igdb_failed items.
	var igdbFailedCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'igdb_failed'`,
		jobID,
	).Scan(ctx, &igdbFailedCount); err != nil {
		slog.Error("process_sync_item: syncCheckJobCompletion igdb_failed count", "job_id", jobID, "err", err)
		return
	}

	if igdbFailedCount > 0 {
		var autoRetryDone bool
		if err := db.NewRaw(`SELECT auto_retry_done FROM jobs WHERE id = ?`, jobID).
			Scan(ctx, &autoRetryDone); err != nil {
			slog.Error("process_sync_item: syncCheckJobCompletion auto_retry_done", "job_id", jobID, "err", err)
			return
		}

		if !autoRetryDone {
			// Reset igdb_failed items to pending.
			type itemID struct{ ID string `bun:"id"` }
			var resetItems []itemID
			if err := db.NewRaw(
				`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL
				 WHERE job_id = ? AND status = 'igdb_failed'
				 RETURNING id`,
				jobID,
			).Scan(ctx, &resetItems); err != nil {
				slog.Error("process_sync_item: syncCheckJobCompletion reset igdb_failed", "job_id", jobID, "err", err)
				return
			}

			// Set auto_retry_done=true on the job.
			_, _ = db.NewRaw(
				`UPDATE jobs SET auto_retry_done = true WHERE id = ?`,
				jobID,
			).Exec(ctx)

			// Re-enqueue River jobs for each reset item.
			if rc != nil {
				for _, item := range resetItems {
					_, _ = rc.Insert(ctx, ProcessSyncItemArgs{JobItemID: item.ID}, nil)
				}
			}
			return
		}

		// auto_retry_done=true — mark completed_with_errors.
		now := time.Now().UTC()
		_, _ = db.NewRaw(
			`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ?`,
			now, jobID,
		).Exec(ctx)
		return
	}

	now := time.Now().UTC()
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ?`,
		now, jobID,
	).Exec(ctx)
}
```

- [ ] **Step 5: Update all `syncCheckJobCompletion` call sites in `sync.go` to pass `w.RiverClient`**

The function is called in 8 places inside `ProcessSyncItemWorker.Work`. Change every call from:

```go
syncCheckJobCompletion(ctx, w.DB, item.JobID)
```

to:

```go
syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
```

(There is no call to this function from `DispatchSyncWorker` — only from `ProcessSyncItemWorker`.)

- [ ] **Step 6: Run the new test**

```bash
go test ./internal/worker/tasks/... -run TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors -v -timeout 120s
```

Expected: PASS

- [ ] **Step 7: Run full tasks package tests**

```bash
go test ./internal/worker/tasks/... -timeout 300s
```

Expected: all pass

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(sync): auto-retry igdb_failed items once on job completion"
```

---

### Task 3: Backend API — `jobItemCounts`, `HandleRetryFailed`, `HandleRecentJobs`

**Files:**
- Modify: `internal/api/jobs.go`
- Modify: `internal/api/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/jobs_test.go`:

```go
// ─── TestJobProgress_IncludesIGDBFailed ───────────────────────────────────────

func TestJobProgress_IncludesIGDBFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "igdb-failed-progress")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k2", "Game B", "igdb_failed")

	rec := getAuth(t, e, "/api/jobs/"+jobID, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	progress, ok := resp["progress"].(map[string]any)
	if !ok {
		t.Fatalf("expected progress map, got %T", resp["progress"])
	}
	if igdbFailed, ok := progress["igdb_failed"].(float64); !ok || igdbFailed != 1 {
		t.Errorf("expected igdb_failed=1 in progress, got %v", progress["igdb_failed"])
	}
}

// ─── TestRetryFailed_IncludesIGDBFailed ──────────────────────────────────────

func TestRetryFailed_IncludesIGDBFailed(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "retry-igdb-failed")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")

	item1ID := uuid.NewString()
	insertJobItem(t, testDB, item1ID, jobID, userID, "k1", "Game A", "igdb_failed")
	item2ID := uuid.NewString()
	insertJobItem(t, testDB, item2ID, jobID, userID, "k2", "Game B", "failed")
	item3ID := uuid.NewString()
	insertJobItem(t, testDB, item3ID, jobID, userID, "k3", "Game C", "completed")

	rec := postAuth(t, e, "/api/jobs/"+jobID+"/retry-failed", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if retried, ok := resp["retried"].(float64); !ok || retried != 2 {
		t.Errorf("expected retried=2 (1 igdb_failed + 1 failed), got %v", resp["retried"])
	}

	var s1, s2 string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item1ID).Scan(context.Background(), &s1)
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item2ID).Scan(context.Background(), &s2)
	if s1 != "pending" {
		t.Errorf("expected igdb_failed item reset to pending, got %q", s1)
	}
	if s2 != "pending" {
		t.Errorf("expected failed item reset to pending, got %q", s2)
	}
}

// ─── TestRecentJobs_IncludesCompletedWithErrors ───────────────────────────────

func TestRecentJobs_IncludesCompletedWithErrors(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "recent-cwe")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}
}

func TestRecentJobs_ReturnsSplitItemArrays(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "recent-split")

	jobID := uuid.NewString()
	insertJob(t, testDB, jobID, userID, "sync", "steam", "completed_with_errors")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k1", "Game A", "completed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k2", "Game B", "igdb_failed")
	insertJobItem(t, testDB, uuid.NewString(), jobID, userID, "k3", "Game C", "skipped")

	rec := getAuth(t, e, "/api/jobs/recent/steam", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	completedItems, _ := job["completed_items"].([]any)
	skippedItems, _ := job["skipped_items"].([]any)
	igdbFailedItems, _ := job["igdb_failed_items"].([]any)
	if len(completedItems) != 1 {
		t.Errorf("expected 1 completed_item, got %d", len(completedItems))
	}
	if len(skippedItems) != 1 {
		t.Errorf("expected 1 skipped_item, got %d", len(skippedItems))
	}
	if len(igdbFailedItems) != 1 {
		t.Errorf("expected 1 igdb_failed_item, got %d", len(igdbFailedItems))
	}
}
```

Note: `postAuth` is a helper that may already exist in `internal/api/jobs_test.go` or `testmain_test.go`. If not, add it:
```go
func postAuth(t *testing.T, handler interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req := httptest.NewRequest(http.MethodPost, path, reqBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

(Check if `postAuth` already exists before adding it; look for existing uses in the file.)

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/api/... -run "TestJobProgress_IncludesIGDBFailed|TestRetryFailed_IncludesIGDBFailed|TestRecentJobs_IncludesCompletedWithErrors|TestRecentJobs_ReturnsSplitItemArrays" -v -timeout 120s
```

Expected: FAIL (compilation errors or assertion failures).

- [ ] **Step 3: Update `jobItemCounts` in `internal/api/jobs.go`**

In the `jobItemCounts` function, add `"igdb_failed": 0` to the map and include it in the returned map:

```go
m := map[string]int{
    "pending": 0, "processing": 0, "completed": 0,
    "pending_review": 0, "skipped": 0, "failed": 0, "igdb_failed": 0,
}
```

And in the returned `map[string]any`:

```go
return map[string]any{
    "pending": m["pending"], "processing": m["processing"],
    "completed": m["completed"], "pending_review": m["pending_review"],
    "skipped": m["skipped"], "failed": m["failed"],
    "igdb_failed": m["igdb_failed"],
    "total": total, "percent": percent,
}
```

Also update the empty progress map in `HandleListJobs`:

```go
emptyProgress := map[string]any{
    "pending": 0, "processing": 0, "completed": 0,
    "pending_review": 0, "skipped": 0, "failed": 0, "igdb_failed": 0,
    "total": 0, "percent": 0,
}
```

- [ ] **Step 4: Update `HandleRetryFailed` in `internal/api/jobs.go`**

Change the query that fetches items to retry to include `igdb_failed`:

```go
// Get failed and igdb_failed items.
var failedItems []models.JobItem
err = h.db.NewRaw(`
    SELECT * FROM job_items
    WHERE job_id = ? AND status IN (?, ?)`,
    jobID, models.JobItemStatusFailed, models.JobItemStatusIGDBFailed,
).Scan(context.Background(), &failedItems)
if err != nil {
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to get failed items")
}

if len(failedItems) == 0 {
    return c.JSON(http.StatusOK, map[string]any{"retried": 0})
}

// Reset failed + igdb_failed items to pending.
_, err = h.db.NewRaw(`
    UPDATE job_items
    SET status = ?, error_message = NULL, processed_at = NULL
    WHERE job_id = ? AND status IN (?, ?)`,
    models.JobItemStatusPending, jobID, models.JobItemStatusFailed, models.JobItemStatusIGDBFailed,
).Exec(context.Background())
```

- [ ] **Step 5: Fix and update `HandleRecentJobs` in `internal/api/jobs.go`**

Change the status filter to include `completed_with_errors`:

```go
err := h.db.NewRaw(`
    SELECT * FROM jobs
    WHERE user_id = ? AND source = ? AND status IN ('completed', 'failed', 'completed_with_errors')
    ORDER BY created_at DESC
    LIMIT ?`,
    userID, source, limit,
).Scan(context.Background(), &jobs)
```

Update `jobWithItems` to use split arrays instead of flat `Items`:

```go
type jobWithItems struct {
    models.Job
    CompletedItems  []recentJobItem `json:"completed_items"`
    SkippedItems    []recentJobItem `json:"skipped_items"`
    FailedItems     []recentJobItem `json:"failed_items"`
    IGDBFailedItems []recentJobItem `json:"igdb_failed_items"`
}
```

Replace the inner loop body that builds `result`:

```go
for _, j := range jobs {
    var allItems []recentJobItem
    err := h.db.NewRaw(`
        SELECT source_title, status,
               result->>'game_title' AS game_title,
               (result->>'is_new_addition')::boolean AS is_new_addition,
               result->>'user_game_id' AS user_game_id
        FROM job_items
        WHERE job_id = ?
        ORDER BY created_at`,
        j.ID,
    ).Scan(context.Background(), &allItems)
    if err != nil {
        allItems = nil
    }

    completedItems := []recentJobItem{}
    skippedItems := []recentJobItem{}
    failedItems := []recentJobItem{}
    igdbFailedItems := []recentJobItem{}
    for _, item := range allItems {
        switch item.Status {
        case models.JobItemStatusCompleted:
            completedItems = append(completedItems, item)
        case models.JobItemStatusSkipped:
            skippedItems = append(skippedItems, item)
        case models.JobItemStatusFailed:
            failedItems = append(failedItems, item)
        case models.JobItemStatusIGDBFailed:
            igdbFailedItems = append(igdbFailedItems, item)
        }
    }
    result = append(result, jobWithItems{
        Job:             j,
        CompletedItems:  completedItems,
        SkippedItems:    skippedItems,
        FailedItems:     failedItems,
        IGDBFailedItems: igdbFailedItems,
    })
}
```

- [ ] **Step 6: Run the new API tests**

```bash
go test ./internal/api/... -run "TestJobProgress_IncludesIGDBFailed|TestRetryFailed_IncludesIGDBFailed|TestRecentJobs_IncludesCompletedWithErrors|TestRecentJobs_ReturnsSplitItemArrays" -v -timeout 120s
```

Expected: PASS

- [ ] **Step 7: Run full API tests**

```bash
go test ./internal/api/... -timeout 300s
```

Expected: all pass

- [ ] **Step 8: Run full test suite**

```bash
go test ./... -timeout 600s
```

Expected: all pass

- [ ] **Step 9: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "feat(api): add igdb_failed to job progress, retry-failed, and recent jobs response"
```

---

### Task 4: Frontend types

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts`

- [ ] **Step 1: Add `IGDB_FAILED` to `JobItemStatus` enum**

```typescript
export enum JobItemStatus {
  PENDING = 'pending',
  PROCESSING = 'processing',
  COMPLETED = 'completed',
  PENDING_REVIEW = 'pending_review',
  SKIPPED = 'skipped',
  FAILED = 'failed',
  IGDB_FAILED = 'igdb_failed',
}
```

- [ ] **Step 2: Add `igdbFailed` to `JobProgress` interface**

```typescript
export interface JobProgress {
  pending: number;
  processing: number;
  completed: number;
  pendingReview: number;
  skipped: number;
  failed: number;
  igdbFailed: number;
  total: number;
  percent: number;
}
```

- [ ] **Step 3: Update `RecentJobDetail` interface**

```typescript
export interface RecentJobDetail {
  id: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  igdbFailedCount: number;
  completedItems: JobItemSummary[];
  skippedItems: JobItemSummary[];
  failedItems: JobItemSummary[];
  igdbFailedItems: JobItemSummary[];
}
```

- [ ] **Step 4: Update `getJobItemStatusLabel`**

```typescript
export function getJobItemStatusLabel(status: JobItemStatus): string {
  const labels: Record<JobItemStatus, string> = {
    [JobItemStatus.PENDING]: 'Pending',
    [JobItemStatus.PROCESSING]: 'Processing',
    [JobItemStatus.COMPLETED]: 'Completed',
    [JobItemStatus.PENDING_REVIEW]: 'Needs Review',
    [JobItemStatus.SKIPPED]: 'Skipped',
    [JobItemStatus.FAILED]: 'Failed',
    [JobItemStatus.IGDB_FAILED]: 'IGDB Error',
  };
  return labels[status];
}
```

- [ ] **Step 5: Update `getJobItemStatusVariant`**

```typescript
export function getJobItemStatusVariant(
  status: JobItemStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case JobItemStatus.COMPLETED:
      return 'default';
    case JobItemStatus.FAILED:
      return 'destructive';
    case JobItemStatus.IGDB_FAILED:
      return 'destructive';
    case JobItemStatus.PENDING_REVIEW:
      return 'secondary';
    default:
      return 'outline';
  }
}
```

- [ ] **Step 6: Run TypeScript check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors (new fields may show errors in `api/jobs.ts` — that's expected and fixed in Task 5).

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/types/jobs.ts
git commit -m "feat(frontend/types): add igdb_failed status and igdbFailed progress field"
```

---

### Task 5: Frontend API transforms

**Files:**
- Modify: `ui/frontend/src/api/jobs.ts`

- [ ] **Step 1: Update `JobProgressApiResponse` interface**

```typescript
interface JobProgressApiResponse {
  pending: number;
  processing: number;
  completed: number;
  pending_review: number;
  skipped: number;
  failed: number;
  igdb_failed: number;
  total: number;
  percent: number;
}
```

- [ ] **Step 2: Update `transformProgress`**

```typescript
function transformProgress(apiProgress: JobProgressApiResponse): JobProgress {
  return {
    pending: apiProgress.pending,
    processing: apiProgress.processing,
    completed: apiProgress.completed,
    pendingReview: apiProgress.pending_review,
    skipped: apiProgress.skipped,
    failed: apiProgress.failed,
    igdbFailed: apiProgress.igdb_failed ?? 0,
    total: apiProgress.total,
    percent: apiProgress.percent,
  };
}
```

- [ ] **Step 3: Update `RecentJobDetailApiResponse` interface**

```typescript
interface RecentJobDetailApiResponse {
  id: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  completed_items: JobItemSummaryApiResponse[];
  skipped_items: JobItemSummaryApiResponse[];
  failed_items: JobItemSummaryApiResponse[];
  igdb_failed_items: JobItemSummaryApiResponse[];
}
```

(Remove `completed_count`, `skipped_count`, `failed_count` — backend no longer returns these separately; the counts are derivable from the arrays' lengths in the frontend.)

- [ ] **Step 4: Update `transformRecentJob`**

```typescript
function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  const completedItems = (api.completed_items ?? []).map(transformJobItemSummary);
  const skippedItems = (api.skipped_items ?? []).map(transformJobItemSummary);
  const failedItems = (api.failed_items ?? []).map(transformJobItemSummary);
  const igdbFailedItems = (api.igdb_failed_items ?? []).map(transformJobItemSummary);
  return {
    id: api.id,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    totalItems: api.total_items,
    completedCount: completedItems.length,
    skippedCount: skippedItems.length,
    failedCount: failedItems.length,
    igdbFailedCount: igdbFailedItems.length,
    completedItems,
    skippedItems,
    failedItems,
    igdbFailedItems,
  };
}
```

- [ ] **Step 5: Run TypeScript check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/jobs.ts
git commit -m "feat(frontend/api): add igdb_failed to progress transform and recent jobs transform"
```

---

### Task 6: Frontend UI — IGDB error items list + retry button

**Files:**
- Modify: `ui/frontend/src/components/sync/recent-activity.tsx`

- [ ] **Step 1: Run TypeScript check to see current state**

```bash
cd ui/frontend && npm run check
```

- [ ] **Step 2: Add `igdb_failed` support to `ItemsList`**

Update the `type` prop union and the icon/label/render logic:

```tsx
function ItemsList({
  items,
  type,
}: {
  items: JobItemSummary[];
  type: 'completed' | 'skipped' | 'failed' | 'igdb_failed';
}) {
  const [isOpen, setIsOpen] = useState(false);

  if (items.length === 0) return null;

  const iconMap = {
    completed: <CheckCircle className="h-4 w-4 text-green-600" />,
    skipped: <SkipForward className="h-4 w-4 text-muted-foreground" />,
    failed: <AlertCircle className="h-4 w-4 text-red-600" />,
    igdb_failed: <AlertCircle className="h-4 w-4 text-orange-500" />,
  };

  const labelMap = {
    completed: 'Completed',
    skipped: 'Skipped',
    failed: 'Failed',
    igdb_failed: 'IGDB Error',
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
                  {item.resultGameTitle && item.resultUserGameId && (
                    <>
                      <span className="mx-1">&rarr;</span>
                      <Link
                        to="/games/$id" params={{ id: String(item.resultUserGameId) }}
                        className="font-medium hover:underline"
                      >
                        {item.resultGameTitle}
                      </Link>
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
                    <span className="text-red-600 text-xs ml-2">- {item.errorMessage}</span>
                  )}
                </div>
              )}
              {type === 'igdb_failed' && (
                <div>
                  <span>{item.sourceTitle}</span>
                  {item.errorMessage && (
                    <span className="text-orange-600 text-xs ml-2">- {item.errorMessage}</span>
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
```

- [ ] **Step 3: Update `JobCard` to show retry button and `igdb_failed` items**

Add required imports at top of file:

```tsx
import { useQueryClient } from '@tanstack/react-query';
import { retryFailedItems } from '@/api';
import { jobsKeys } from '@/hooks';
```

Replace the `JobCard` function:

```tsx
function JobCard({ job }: { job: RecentJobDetail }) {
  const [isOpen, setIsOpen] = useState(false);
  const [isRetrying, setIsRetrying] = useState(false);
  const queryClient = useQueryClient();

  const handleRetry = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsRetrying(true);
    try {
      await retryFailedItems(job.id);
      await queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    } finally {
      setIsRetrying(false);
    }
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-3 h-auto">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <span>{job.completedAt ? formatDate(job.completedAt) : 'In progress'}</span>
          </div>
          <div className="flex items-center gap-2">
            {job.igdbFailedCount > 0 && (
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-xs"
                onClick={handleRetry}
                disabled={isRetrying}
              >
                {isRetrying
                  ? 'Retrying…'
                  : `Retry ${job.igdbFailedCount} IGDB ${job.igdbFailedCount === 1 ? 'error' : 'errors'}`}
              </Button>
            )}
            <Badge variant="outline">{job.totalItems} games processed</Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-4 pb-4 space-y-1">
          <ItemsList items={job.completedItems} type="completed" />
          <ItemsList items={job.skippedItems} type="skipped" />
          <ItemsList items={job.failedItems} type="failed" />
          <ItemsList items={job.igdbFailedItems} type="igdb_failed" />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
```

- [ ] **Step 4: Run TypeScript check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors

- [ ] **Step 5: Run frontend tests**

```bash
cd ui/frontend && npm run test
```

Expected: all pass

- [ ] **Step 6: Run full Go test suite one final time**

```bash
go test ./... -timeout 600s
```

Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/sync/recent-activity.tsx
git commit -m "feat(frontend/ui): add igdb_failed items list and retry button in recent activity"
```

---

## Self-Review

**Spec coverage:**
- ✅ `igdb_failed` status constant + `syncMarkItemIGDBFailed` (Task 1)
- ✅ IGDB error path marks `igdb_failed` instead of silently continuing (Task 1)
- ✅ One-time auto-retry on job completion (Task 2)
- ✅ `completed_with_errors` on retry exhaustion (Task 2)
- ✅ `igdb_failed` in `jobItemCounts` progress (Task 3)
- ✅ `HandleRetryFailed` includes `igdb_failed` items (Task 3)
- ✅ `HandleRecentJobs` includes `completed_with_errors` jobs (Task 3)
- ✅ `HandleRecentJobs` returns split arrays by status — fixes pre-existing bug (Task 3)
- ✅ Frontend types for `IGDB_FAILED`, `igdbFailed`, `igdbFailedItems` (Task 4)
- ✅ Frontend API transforms (Task 5)
- ✅ Frontend UI: `igdb_failed` items list + retry button (Task 6)

**Type consistency:**
- `JobItemStatusIGDBFailed = "igdb_failed"` (Go) matches `JobItemStatus.IGDB_FAILED = 'igdb_failed'` (TS)
- `igdb_failed` (API JSON) maps to `igdbFailed` (TS camelCase) in `transformProgress`
- `igdb_failed_items` (API JSON) maps to `igdbFailedItems` (TS) in `transformRecentJob`
- `RecentJobDetail.igdbFailedCount` is computed from `igdbFailedItems.length` in transform

**Placeholder scan:** No TBDs, no unimplemented steps, all code shown in full.
