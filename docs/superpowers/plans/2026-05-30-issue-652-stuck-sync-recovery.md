# Stuck Sync Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent `dispatch_sync` River jobs from permanently deadlocking the sync pipeline when a process is killed mid-dispatch.

**Architecture:** Three independent changes: raise `MaxAttempts` so River can retry a failed dispatch, add a startup reconciliation pass to immediately re-queue orphaned River jobs, and extend `CleanupStaleJobs` to mark sync jobs failed once they've been stuck beyond the stale threshold with no active work remaining.

**Tech Stack:** Go, River (`riverqueue/river`), Bun ORM, PostgreSQL, testcontainers-go for integration tests.

---

### Task 1: Raise MaxAttempts for dispatch_sync

**Files:**
- Modify: `internal/worker/tasks/sync.go:47-49`

- [ ] **Step 1: Change MaxAttempts from 1 to 3**

In `internal/worker/tasks/sync.go`, change `InsertOpts()` on `DispatchSyncArgs`:

```go
func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 1}
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
make build
```

Expected: binary builds successfully, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "fix: raise dispatch_sync MaxAttempts to 3 to allow retry on crash"
```

---

### Task 2: Extend CleanupStaleJobs to cover sync jobs

**Files:**
- Modify: `internal/scheduler/stale_jobs.go`
- Modify: `internal/scheduler/stale_jobs_test.go`

The existing `CleanupStaleJobs` function runs one UPDATE for `metadata_refresh`. We add a second UPDATE for `sync` jobs with two extra guards: `dispatch_complete = false` (so jobs where dispatch finished are never touched) and `NOT EXISTS active job_items` (mirrors the existing guard).

- [ ] **Step 1: Write the failing tests**

Append to `internal/scheduler/stale_jobs_test.go`:

```go
// ── sync job cleanup ──────────────────────────────────────────────────────────

func TestCleanupStaleJobs_SyncJob_StuckDispatch_Cleaned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, false, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	var errMsg *string
	if err := testDB.NewRaw(
		`SELECT status, error_message FROM jobs WHERE id = ?`, jobID,
	).Scan(ctx, &status, &errMsg); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status=failed, got %s", status)
	}
	if errMsg == nil || *errMsg != "stale_job_cleaned_up" {
		t.Fatalf("expected error_message=stale_job_cleaned_up, got %v", errMsg)
	}
}

func TestCleanupStaleJobs_SyncJob_DispatchComplete_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, true, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("sync job with dispatch_complete=true should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_SyncJob_WithActivePendingItem_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, false, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'x', '{}', 'pending', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID, "k1",
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert item: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("sync job with active items should not be touched, got %s", status)
	}
}

func TestCleanupStaleJobs_SyncJob_FreshJob_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 0, false, now() - interval '1 hour')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "processing" {
		t.Fatalf("fresh sync job should not be touched, got %s", status)
	}
}
```

- [ ] **Step 2: Run the new tests to confirm they fail**

```bash
go test ./internal/scheduler/... -run "TestCleanupStaleJobs_SyncJob" -v
```

Expected: all four tests FAIL (sync jobs are currently not cleaned up by `CleanupStaleJobs`).

Note: `TestCleanupStaleJobs_NonMetadataRefresh_LeftAlone` already exists and inserts a sync job expecting it to be left alone — that test will need to be updated after implementing this task (see Step 4).

- [ ] **Step 3: Implement the sync UPDATE in CleanupStaleJobs**

In `internal/scheduler/stale_jobs.go`, add a second UPDATE after the existing one:

```go
func CleanupStaleJobs(ctx context.Context, db *bun.DB, threshold time.Duration) {
	result, err := db.NewRaw(
		`UPDATE jobs
		   SET status = 'failed',
		       error_message = 'stale_job_cleaned_up',
		       completed_at = now()
		 WHERE job_type = 'metadata_refresh'
		   AND status IN ('pending', 'processing')
		   AND created_at < now() - (? || ' seconds')::interval
		   AND NOT EXISTS (
		     SELECT 1 FROM job_items
		      WHERE job_items.job_id = jobs.id
		        AND job_items.status NOT IN ('completed', 'failed', 'skipped')
		   )`,
		int64(threshold.Seconds()),
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup_stale_jobs: failed", "err", err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.Info("cleanup_stale_jobs: marked stale jobs failed", "count", rows)
	}

	syncResult, err := db.NewRaw(
		`UPDATE jobs
		   SET status = 'failed',
		       error_message = 'stale_job_cleaned_up',
		       completed_at = now()
		 WHERE job_type = 'sync'
		   AND status IN ('pending', 'processing')
		   AND dispatch_complete = false
		   AND created_at < now() - (? || ' seconds')::interval
		   AND NOT EXISTS (
		     SELECT 1 FROM job_items
		      WHERE job_items.job_id = jobs.id
		        AND job_items.status NOT IN ('completed', 'failed', 'skipped', 'cancelled')
		   )`,
		int64(threshold.Seconds()),
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup_stale_jobs: sync cleanup failed", "err", err)
		return
	}
	syncRows, _ := syncResult.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if syncRows > 0 {
		slog.Info("cleanup_stale_jobs: marked stale sync jobs failed", "count", syncRows)
	}
}
```

- [ ] **Step 4: Update the now-invalidated existing test**

`TestCleanupStaleJobs_NonMetadataRefresh_LeftAlone` in `stale_jobs_test.go` inserts a sync job with no `dispatch_complete` or `created_at` override — by default `dispatch_complete` is `false` and `created_at` is `now() - 5 hours`. After this change it will be cleaned up correctly, so the test name and assertion are now wrong. Update it:

```go
func TestCleanupStaleJobs_NonMetadataRefresh_LeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'import', 'manual', 'pending', 'low', 0, now() - interval '5 hours')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	scheduler.CleanupStaleJobs(ctx, testDB, 4*time.Hour)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("re-read job: %v", err)
	}
	if status != "pending" {
		t.Fatalf("import job should not be touched, got status=%s", status)
	}
}
```

- [ ] **Step 5: Run all stale_jobs tests**

```bash
go test ./internal/scheduler/... -run "TestCleanupStaleJobs" -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/scheduler/stale_jobs.go internal/scheduler/stale_jobs_test.go
git commit -m "fix: extend CleanupStaleJobs to mark stale dispatch-incomplete sync jobs failed"
```

---

### Task 3: Add startup reconciliation for orphaned dispatch_sync River jobs

**Files:**
- Modify: `cmd/nexorious/serve.go`

The River start goroutine (lines 330–346) currently waits for `AppStateReady` then calls `riverClient.Start(ctx)` directly. We add a `reconcileOrphanedDispatchJobs` call immediately before `riverClient.Start`.

There is no meaningful unit test for this function because it operates directly against `river_job` and `river_client` tables which require a full River-migrated schema. The function is simple SQL with no branching logic; the scheduler package's `TestMain` already validates River migrations run cleanly. Manual verification (see Step 4) is sufficient.

- [ ] **Step 1: Add the reconciliation function and call it before River starts**

In `cmd/nexorious/serve.go`, add the function at the bottom of the file (before `buildAdapterFactory`):

```go
// reconcileOrphanedDispatchJobs rescues dispatch_sync River jobs that are
// stuck in 'running' state because the process that claimed them is no longer
// heartbeating. Called once at startup before riverClient.Start so River picks
// them up for retry within seconds.
func reconcileOrphanedDispatchJobs(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(`
		UPDATE river_job
		   SET state = 'retryable',
		       scheduled_at = now(),
		       errors = errors || jsonb_build_array(jsonb_build_object(
		         'at', now(),
		         'error', 'rescued at startup: client no longer heartbeating'
		       ))
		 WHERE kind = 'dispatch_sync'
		   AND state = 'running'
		   AND attempt < max_attempts
		   AND NOT EXISTS (
		     SELECT 1 FROM river_client rc
		      WHERE rc.id = ANY(river_job.attempted_by)
		        AND rc.updated_at > now() - interval '30 seconds'
		   )`,
	).Exec(ctx)
	if err != nil {
		slog.Error("startup: reconcile orphaned dispatch_sync failed", "err", err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.Info("startup: rescued orphaned dispatch_sync jobs", "count", rows)
	}
}
```

Then, in the River start goroutine (around line 337), add the call before `riverClient.Start`:

```go
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
            reconcileOrphanedDispatchJobs(context.Background(), db)
            if err := riverClient.Start(ctx); err != nil {
                slog.Error("failed to start River client", "err", err)
            }
            slog.Info("app ready — River client started")
            return
        }
        time.Sleep(2 * time.Second)
    }
}(shutdownCtx)
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
make build
```

Expected: binary builds successfully, no errors.

- [ ] **Step 3: Run the full Go test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "fix: rescue orphaned dispatch_sync River jobs at startup"
```

---

### Task 4: One-off cleanup of the stuck job (operational)

This task has no code changes. It is an operational step to be performed immediately after deploying.

- [ ] **Step 1: Cancel the stuck job via the API**

```
POST /api/jobs/fc6e7dab-1369-4adc-98a5-a343f4c36da4/cancel
Authorization: Bearer <api-key>
```

This marks the `jobs` row `cancelled` and cancels any queued River items for that job. The next `CheckPendingSyncs` tick (runs every 15 minutes) will schedule a fresh Steam sync.

- [ ] **Step 2: Verify the job is cancelled**

```
GET /api/jobs/fc6e7dab-1369-4adc-98a5-a343f4c36da4
```

Expected: `"status": "cancelled"`.

---

### Task 5: Open and close PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin issue-652-stuck-sync-recovery
```

- [ ] **Step 2: Create the PR**

Title: `fix: prevent dispatch_sync from permanently deadlocking the sync pipeline`

Body should reference issue #652 and note the operational step (cancel stuck job after deploy).

- [ ] **Step 3: After PR is merged, cancel the stuck job (Task 4)**

Perform the operational step from Task 4 against production.
