# Sync Completion Dispatch Gate (#642) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop a storefront sync job from being marked `completed` while `DispatchSyncWorker` is still streaming and enqueuing later batches, which orphans `pending_review` items and hides the progress UI.

**Architecture:** Add a `dispatch_complete` boolean sentinel to `jobs`. `DispatchSyncWorker` sets it `false` while streaming the library and `true` once dispatch finishes, then runs one authoritative completion check. `SyncCheckJobCompletion` refuses to finalize any job whose dispatch is not yet complete. The column defaults `TRUE`, so every other job type and every pre-existing row is unaffected — only `DispatchSyncWorker` ever toggles it.

**Tech Stack:** Go 1.25, Bun ORM + raw SQL (`db.NewRaw`), Bun migrate (single in-place initial migration), River job queue, stdlib `testing` + testcontainers (shared `testDB`).

**Spec:** `docs/superpowers/specs/2026-05-27-sync-completion-dispatch-gate-design.md`

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Schema | Add `dispatch_complete` column to `CREATE TABLE jobs` |
| `internal/db/models/jobs.go` | `Job` struct | Add `DispatchComplete` field (required for `SELECT *` scans) |
| `internal/worker/tasks/sync.go` | Sync workers | Gate `SyncCheckJobCompletion`; toggle the flag in `DispatchSyncWorker` + trailing completion check |
| `internal/worker/tasks/sync_test.go` | Sync tests (`package tasks_test`) | New regression tests for the gate, the streaming flag, and the empty-library finalize |

**Note on editing the migration in place:** the project deliberately edits the single initial migration rather than adding a new one. The test container is created fresh each `go test` run, so tests pick up the new column automatically. Any **already-migrated local/dev database must be recreated** (or have the column added by hand) to gain `dispatch_complete`.

---

## Task 1: Schema groundwork — `dispatch_complete` column + model field

This is prerequisite plumbing for Tasks 2–3 (their tests reference the column). It is non-breaking by construction: the column defaults `TRUE`, so all existing rows and all non-dispatch job paths behave exactly as before. The verification step proves that by running the existing completion test.

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql` (the `CREATE TABLE jobs` block, ~line 222–235)
- Modify: `internal/db/models/jobs.go` (the `Job` struct, ~line 48–64)

- [ ] **Step 1: Add the column to the `jobs` table**

In `internal/db/migrations/20260503000001_initial.up.sql`, insert the new column after `auto_retry_done`. The block changes from:

```sql
    auto_retry_done BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
```

to:

```sql
    auto_retry_done   BOOLEAN NOT NULL DEFAULT FALSE,
    dispatch_complete BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
```

No change to `20260503000001_initial.down.sql` — it already runs `DROP TABLE IF EXISTS jobs`.

- [ ] **Step 2: Add the field to the `Job` model**

In `internal/db/models/jobs.go`, add `DispatchComplete` to the `Job` struct, right after `AutoRetryDone`:

```go
	AutoRetryDone    bool       `bun:"auto_retry_done,notnull"   json:"auto_retry_done"`
	DispatchComplete bool       `bun:"dispatch_complete,notnull" json:"-"`
	CreatedAt        time.Time  `bun:"created_at,notnull"        json:"created_at"`
```

`json:"-"` keeps the field out of the public API — the jobs endpoints build their JSON response maps by hand, so nothing serializes the `Job` struct directly in a way the API contract depends on. The field is required because `SELECT * FROM jobs` is scanned into `models.Job` in several places (`internal/api/job_items.go:84`, `internal/api/sync.go:1077`, `internal/api/jobs.go`) — Bun errors on an unmapped column. (gofmt will re-align the struct tags; that is expected and applied automatically by the post-edit hook.)

- [ ] **Step 3: Verify the build compiles**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 4: Verify existing completion behavior is unchanged**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletion_FailedItemsYieldsCompleted -v`
Expected: PASS. (This existing test seeds a job via `insertTestJob`, which now gets `dispatch_complete = TRUE` by default, so it still finalizes to `completed` — proving the column is non-breaking.)

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql internal/db/models/jobs.go
git commit -m "fix(db): add dispatch_complete gate column to jobs"
```

---

## Task 2: Gate `SyncCheckJobCompletion` on `dispatch_complete`

The completion check must never finalize a job whose dispatch is still in flight. We fold the gate into the atomic finalize `UPDATE` so there is no read-then-write race.

**Files:**
- Test: `internal/worker/tasks/sync_test.go` (append near the existing `TestSyncCheckJobCompletion_FailedItemsYieldsCompleted`, ~after line 1877; `package tasks_test`)
- Modify: `internal/worker/tasks/sync.go` (`SyncCheckJobCompletion`, ~line 845–890)

- [ ] **Step 1: Write the failing regression test**

Append to `internal/worker/tasks/sync_test.go`:

```go
// TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing is the #642
// regression guard: while DispatchSyncWorker is still streaming batches
// (dispatch_complete=false), a transiently-empty active set must NOT finalize
// the job, or items from later batches get orphaned under a terminal job.
func TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Dispatch is still streaming batches.
	if _, err := testDB.NewRaw(`UPDATE jobs SET dispatch_complete = false WHERE id = ?`, jobID).Exec(ctx); err != nil {
		t.Fatalf("set dispatch_complete=false: %v", err)
	}

	// One fully-resolved item, none pending_review — the empty-active set that
	// previously tripped premature completion.
	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "processing" {
		t.Errorf("dispatch incomplete: expected job to stay 'processing', got %q", status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing -v`
Expected: FAIL — got `"completed"`, want `"processing"` (the ungated `UPDATE` finalizes the job).

- [ ] **Step 3: Add the gate to the finalize UPDATE**

In `internal/worker/tasks/sync.go`, the finalizing `UPDATE` in `SyncCheckJobCompletion` (~line 884) changes from:

```go
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		finalStatus, now, jobID,
	).Exec(ctx); err != nil {
```

to:

```go
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ? AND status IN ('pending', 'processing') AND dispatch_complete = true`,
		finalStatus, now, jobID,
	).Exec(ctx); err != nil {
```

- [ ] **Step 4: Update the doc comment to record the invariant**

In `internal/worker/tasks/sync.go`, extend the doc comment above `SyncCheckJobCompletion` (~line 845) by adding this sentence to it (place it after the existing "Active" paragraph):

```go
// In addition, a sync job is never finalized while its dispatch is still
// streaming batches: DispatchSyncWorker sets jobs.dispatch_complete=false on
// entry and true only after the full library has been dispatched, so the
// completion check below treats dispatch_complete=false as "more work may
// still arrive" and refuses to finalize.
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletion_DispatchIncomplete_StaysProcessing -v`
Expected: PASS.

- [ ] **Step 6: Add the two companion guard tests**

Append to `internal/worker/tasks/sync_test.go`:

```go
// TestSyncCheckJobCompletion_DispatchComplete_Finalizes confirms the happy
// path: once dispatch is complete and no active/pending_review items remain,
// the job finalizes to 'completed'.
func TestSyncCheckJobCompletion_DispatchComplete_Finalizes(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1) // dispatch_complete defaults TRUE

	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'completed', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "completed" {
		t.Errorf("dispatch complete, no active/review items: expected 'completed', got %q", status)
	}
}

// TestSyncCheckJobCompletion_PendingReviewBlocks confirms pending_review items
// keep the job processing even when dispatch is complete (existing invariant).
func TestSyncCheckJobCompletion_PendingReviewBlocks(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1) // dispatch_complete defaults TRUE

	if _, err := testDB.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (gen_random_uuid(), ?, ?, 'key1', 'Game A', '{}', 'pending_review', '{}', '[]', now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	tasks.SyncCheckJobCompletion(ctx, testDB, jobID)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "processing" {
		t.Errorf("pending_review present: expected job to stay 'processing', got %q", status)
	}
}
```

- [ ] **Step 7: Run all three completion tests**

Run: `go test ./internal/worker/tasks/ -run TestSyncCheckJobCompletion -v`
Expected: PASS (all of `_DispatchIncomplete_StaysProcessing`, `_DispatchComplete_Finalizes`, `_PendingReviewBlocks`, and the pre-existing `_FailedItemsYieldsCompleted`).

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "fix(sync): gate job completion on dispatch_complete"
```

---

## Task 3: Toggle the flag in `DispatchSyncWorker` + final completion check

`DispatchSyncWorker` must (a) set `dispatch_complete = false` when it begins streaming so per-item completion checks during inter-batch gaps are gated off, and (b) set it `true` after the full library is dispatched, then run one authoritative completion check (which finalizes the job when all items already drained — including an empty library, which currently never completes because no item worker ever fires).

**Files:**
- Test: `internal/worker/tasks/sync_test.go` (append; `package tasks_test`)
- Modify: `internal/worker/tasks/sync.go` (`DispatchSyncWorker.Work`: step-1 UPDATE ~line 144, and end of `Work` before `return nil` ~line 286)

- [ ] **Step 1: Add a streaming-probe adapter and write the two failing tests**

Append to `internal/worker/tasks/sync_test.go`:

```go
// streamProbeAdapter yields batches and invokes probe() after the first batch
// (i.e. while dispatch is still mid-stream) so a test can observe the job's
// dispatch_complete flag during streaming.
type streamProbeAdapter struct {
	batches [][]tasks.ExternalGameEntry
	probe   func()
}

func (a *streamProbeAdapter) GetLibrary(_ context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
	for i, batch := range a.batches {
		if err := onBatch(batch); err != nil {
			return err
		}
		if i == 0 && a.probe != nil {
			a.probe()
		}
	}
	return nil
}

// TestDispatchSync_FlagFalseWhileStreaming proves DispatchSyncWorker sets
// dispatch_complete=false while it is still streaming batches, and true once
// dispatch finishes.
func TestDispatchSync_FlagFalseWhileStreaming(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	var midStreamFlag bool
	adapter := &streamProbeAdapter{
		batches: [][]tasks.ExternalGameEntry{
			{{ExternalID: "1", Title: "Game A", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
			{{ExternalID: "2", Title: "Game B", Platforms: []string{"pc-windows"}, OwnershipStatus: "owned"}},
		},
		probe: func() {
			_ = testDB.NewRaw(`SELECT dispatch_complete FROM jobs WHERE id = ?`, jobID).Scan(ctx, &midStreamFlag)
		},
	}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if midStreamFlag {
		t.Error("expected dispatch_complete=false while dispatch is still streaming batches")
	}

	var finalFlag bool
	if err := testDB.NewRaw(`SELECT dispatch_complete FROM jobs WHERE id = ?`, jobID).Scan(ctx, &finalFlag); err != nil {
		t.Fatalf("query dispatch_complete: %v", err)
	}
	if !finalFlag {
		t.Error("expected dispatch_complete=true after dispatch finished")
	}
}

// TestDispatchSync_EmptyLibrary_Finalizes proves an empty library finalizes the
// job: dispatch sets dispatch_complete=true and runs the authoritative
// completion check, which finds no active/pending_review items.
func TestDispatchSync_EmptyLibrary_Finalizes(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	// Adapter yields no games at all.
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&fakeStorefrontAdapter{}), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}

	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "completed" {
		t.Errorf("empty library: expected job 'completed', got %q", status)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/worker/tasks/ -run 'TestDispatchSync_FlagFalseWhileStreaming|TestDispatchSync_EmptyLibrary_Finalizes' -v`
Expected: FAIL —
- `FlagFalseWhileStreaming`: `midStreamFlag` reads `true` (the worker never sets the flag `false`, so it stays at the column default `TRUE`).
- `EmptyLibrary_Finalizes`: job status is `"processing"`, not `"completed"` (the worker never runs a completion check).

- [ ] **Step 3: Set `dispatch_complete = false` when dispatch begins**

In `internal/worker/tasks/sync.go`, the step-1 "mark processing" `UPDATE` (~line 144) changes from:

```go
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}
```

to:

```go
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ?, dispatch_complete = false WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}
```

- [ ] **Step 4: Mark dispatch complete and run the authoritative completion check**

In `internal/worker/tasks/sync.go`, at the end of `DispatchSyncWorker.Work`, replace the final `return nil` (~line 286, immediately after step 8's `last_synced_at` update) with:

```go
	// 9. Dispatch is fully complete — every batch has been streamed and enqueued.
	//    Open the completion gate and run the authoritative check: this finalizes
	//    the job when all items already drained during dispatch (including an
	//    empty library), and lets per-item checks finalize it from here on.
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark dispatch_complete failed", "err", err, "job_id", p.JobID)
	}
	SyncCheckJobCompletion(ctx, w.DB, p.JobID)

	return nil
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/worker/tasks/ -run 'TestDispatchSync_FlagFalseWhileStreaming|TestDispatchSync_EmptyLibrary_Finalizes' -v`
Expected: PASS.

- [ ] **Step 6: Run the full sync test package to confirm no regressions**

Run: `go test ./internal/worker/tasks/ -v`
Expected: PASS. In particular the existing `TestDispatchSync_Steam*`/`TestDispatchSync_PSN*` success tests stay green — their items are enqueued but never processed (no running worker), so the trailing `SyncCheckJobCompletion` sees active items and does not finalize; those tests assert `external_games`/platforms/`last_synced_at`, not job status.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "fix(sync): set dispatch_complete around library dispatch"
```

---

## Final verification

- [ ] **Run the affected backend packages**

Run: `go test ./internal/worker/tasks/ ./internal/api/ -count=1`
Expected: PASS. The API package's `TestSkipGame_MarksJobItemSkippedAndCompletesJob` (`internal/api/sync_test.go:480`) seeds its job without `dispatch_complete`, so it gets `TRUE` by default and still finalizes to `completed` via the review-resolution path — confirming the gate does not block the manual review flow.

- [ ] **Confirm the full suite passes at push time**

The pre-push git hook runs `go test ./...`. Push the branch and let it run, or run `go test ./...` manually before opening the PR.

---

## Out of scope (per spec)

- **Recovery of already-orphaned jobs** — prevention only; jobs already wrongly finalized with outstanding `pending_review` items are not repaired.
- **#643 dual-count unification** — the nav-badge vs. detail-page count divergence remains its own follow-up.
