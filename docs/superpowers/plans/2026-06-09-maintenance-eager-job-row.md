# Maintenance Eager Jobs Row Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the admin "start refresh" handlers synchronously create a minimal `jobs` row and return a real `job_id`, with the dispatch workers populating that pre-created row, then remove the now-redundant frontend eager-poll workaround.

**Architecture:** Add an optional `JobID` field to both River dispatch args. When set (the admin HTTP path), the worker uses/finalizes the handler-created row instead of inserting one; when empty (the periodic-scheduler path for metadata, the sync-completion path for store-link) the worker behaves exactly as today. The handler commits the row before enqueuing the dispatch job, honoring the "River insert after bun commit" constraint.

**Tech Stack:** Go (Echo v5, Bun, River, pgx), React 19 + TanStack Query + Vitest.

**Spec:** `docs/superpowers/specs/2026-06-09-maintenance-eager-job-row-design.md`

---

## File Structure

- `internal/worker/tasks/metadata_refresh.go` — add `JobID` to args; branch `Work` on handler-owned vs self-created.
- `internal/worker/tasks/store_link_refresh.go` — add `JobID` to args; branch `Work` likewise.
- `internal/api/games.go` — add a `startMaintenanceRefresh` helper; rewrite both start handlers to create the row + return the id.
- `internal/worker/tasks/metadata_refresh_test.go` — add handler-owned populate + empty-results tests.
- `internal/worker/tasks/store_link_refresh_insert_test.go` — add handler-owned populate + empty-results tests.
- `internal/api/games_test.go` — add metadata-handler test; extend store-link-handler test for the real `job_id` + idempotency.
- `ui/frontend/src/api/admin.ts` — `startStoreLinkRefreshJob` returns `jobId`.
- `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — pin to the returned id; remove eager-poll + dismissal workaround.
- `ui/frontend/src/hooks/use-jobs.ts` — remove the `eager` option from `useJobTypeStatus`.
- `ui/frontend/src/hooks/use-jobs.test.ts`, `.../admin/maintenance.test.ts` — update for the above.

---

## Task 1: Add `JobID` to the dispatch args

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go:25-26`
- Modify: `internal/worker/tasks/store_link_refresh.go:35-39`

- [ ] **Step 1: Add the field to `MetadataRefreshDispatchArgs`**

Replace:

```go
// MetadataRefreshDispatchArgs is the River job args type for "metadata_refresh_dispatch".
type MetadataRefreshDispatchArgs struct{}
```

with:

```go
// MetadataRefreshDispatchArgs is the River job args type for "metadata_refresh_dispatch".
// JobID, when set, names a pre-created (handler-owned) jobs row the worker must
// populate instead of inserting its own; empty means the worker self-creates the
// row (the periodic-scheduler path).
type MetadataRefreshDispatchArgs struct {
	JobID string `json:"job_id,omitempty"`
}
```

- [ ] **Step 2: Add the field to `StoreLinkRefreshDispatchArgs`**

In the `StoreLinkRefreshDispatchArgs` struct (after the `Force` field), add:

```go
	// JobID, when set, names a pre-created (handler-owned) jobs row to populate
	// instead of inserting one; empty means self-create (the sync-completion path).
	JobID string `json:"job_id,omitempty"`
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./internal/worker/tasks/`
Expected: no output (success). Existing tests still compile because the new field is optional.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go internal/worker/tasks/store_link_refresh.go
git commit -m "refactor: add optional JobID to maintenance dispatch args (#890)"
```

---

## Task 2: Metadata dispatch worker — handler-owned populate path (test first)

**Files:**
- Test: `internal/worker/tasks/metadata_refresh_test.go`
- Modify: `internal/worker/tasks/metadata_refresh.go:46-148`

- [ ] **Step 1: Write the failing test for the handler-owned populate path**

Append to `internal/worker/tasks/metadata_refresh_test.go` (in `package tasks_test`):

```go
func TestMetadataRefreshDispatch_HandlerOwned_PopulatesExistingRow(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	now := time.Now().UTC()
	insertTestGame(t, 1001, "Game One", now.Add(-48*time.Hour))
	insertTestGame(t, 1002, "Game Two", now.Add(-24*time.Hour))

	ctx := context.Background()
	// Simulate the handler having created a pending row up front.
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestMetadataRiverClient(t)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{
		Args: tasks.MetadataRefreshDispatchArgs{JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Exactly one job row (no duplicate), flipped to processing with total_items set.
	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Fatalf("expected 1 job row, got %d", count)
	}
	var status string
	var total int
	_ = testDB.NewRaw(`SELECT status, total_items FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status, &total)
	if status != "processing" {
		t.Errorf("status: want processing, got %s", status)
	}
	if total != 2 {
		t.Errorf("total_items: want 2, got %d", total)
	}
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 2 {
		t.Errorf("job_items: want 2, got %d", itemCount)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshDispatch_HandlerOwned_PopulatesExistingRow -v`
Expected: FAIL — the current worker ignores `JobID`, inserts a *second* row, so the count assertion (and/or status on the pre-created row) fails.

- [ ] **Step 3: Rewrite `MetadataRefreshDispatchWorker.Work`**

Replace the entire `Work` method (lines 46-148) with:

```go
func (w *MetadataRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshDispatchArgs]) error {
	if !w.IGDBClient.Configured() {
		slog.Warn("metadata_refresh_dispatch: IGDB not configured, skipping")
		return nil
	}

	jobID := job.Args.JobID
	handlerOwned := jobID != ""

	// Resolve the job owner (used to stamp job_items). Handler-owned rows already
	// carry user_id; the self-created (periodic) path falls back to the first admin.
	var ownerID string
	if handlerOwned {
		err := w.DB.NewRaw(`SELECT user_id FROM jobs WHERE id = ?`, jobID).Scan(ctx, &ownerID)
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("metadata_refresh_dispatch: handler job row missing, skipping", "job_id", jobID)
			return nil
		}
		if err != nil {
			slog.Error("metadata_refresh_dispatch: load job owner", "err", err)
			return nil
		}
	} else {
		err := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &ownerID)
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("metadata_refresh_dispatch: no admin user found, skipping")
			return nil
		}
		if err != nil {
			slog.Error("metadata_refresh_dispatch: query admin user", "err", err)
			return nil
		}

		// Skip if a refresh job is already active. Only the self-created path runs
		// this guard; for handler-owned jobs the handler owns dedup (and the row it
		// created would otherwise match here and make the worker skip its own work).
		var existingJobID string
		err = w.DB.NewRaw(
			`SELECT id FROM jobs WHERE job_type = ? AND status IN ('pending', 'processing') LIMIT 1`,
			models.JobTypeMetadataRefresh,
		).Scan(ctx, &existingJobID)
		if err == nil {
			slog.Info("metadata_refresh_dispatch: job already active, skipping", "existing_job_id", existingJobID)
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("metadata_refresh_dispatch: duplicate check", "err", err)
			return nil
		}

		jobID = uuid.NewString()
	}

	// Oldest-refreshed games first.
	var games []struct {
		ID    int32  `bun:"id"`
		Title string `bun:"title"`
	}
	if err := w.DB.NewRaw(`SELECT id, title FROM games ORDER BY last_updated ASC`).Scan(ctx, &games); err != nil {
		slog.Error("metadata_refresh_dispatch: query games", "err", err)
		return nil
	}
	if len(games) == 0 {
		// A handler-owned row is already pending; finalize it so it never sticks
		// in 'pending' forever. The self-created path created nothing, so no-op.
		if handlerOwned {
			finalizeJobCompleted(ctx, w.DB, jobID, "metadata_refresh_dispatch: finalize empty", false)
		}
		return nil
	}

	// River jobs must be inserted AFTER the transaction commits: riverClient.Insert uses a
	// separate connection and commits immediately, so workers can dequeue and attempt to load
	// job_items before the bun transaction is visible — causing "no rows" errors.
	itemIDs := make([]string, 0, len(games))
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if handlerOwned {
			if _, err := tx.NewRaw(
				`UPDATE jobs SET total_items = ?, status = 'processing' WHERE id = ?`,
				len(games), jobID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("update job: %w", err)
			}
		} else {
			if _, err := tx.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
				 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
				jobID, ownerID, models.JobTypeMetadataRefresh, models.JobSourceSystem, len(games),
			).Exec(ctx); err != nil {
				return fmt.Errorf("insert job: %w", err)
			}
		}

		// Insert job_items only; River jobs are enqueued after commit.
		for _, g := range games {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)

			sourceMeta, _ := json.Marshal(map[string]any{"game_id": g.ID}) //nolint:errcheck // marshaling a fixed map cannot fail

			if _, err := tx.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
				itemID, jobID, ownerID, strconv.Itoa(int(g.ID)), g.Title, json.RawMessage(sourceMeta),
			).Exec(ctx); err != nil {
				return fmt.Errorf("insert job_item for game %d: %w", g.ID, err)
			}
		}

		return nil
	}); err != nil {
		slog.Error("metadata_refresh_dispatch: transaction failed", "err", err)
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "metadata_refresh_dispatch", Error: err.Error()},
		})
		return nil
	}

	// Enqueue River jobs now that job_items are committed and visible.
	for _, itemID := range itemIDs {
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, MetadataRefreshItemArgs{JobItemID: itemID}); err != nil {
			slog.Error("metadata_refresh_dispatch: enqueue item failed", "err", err, "job_id", jobID, "item_id", itemID)
		}
	}

	slog.Info("metadata_refresh_dispatch: job created", "job_id", jobID, "game_count", len(games))
	notify.Emit(ctx, w.DB, notify.EmitParams{
		Type: notify.TypeAdminMaintCompleted, Scope: notify.ScopeAdmin,
		Payload: notify.MaintPayload{Action: "metadata_refresh_dispatch", Count: len(games)},
	})
	return nil
}
```

- [ ] **Step 4: Run the new test and the existing dispatch tests**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshDispatch -v`
Expected: PASS — the new handler-owned test plus all existing `TestMetadataRefreshDispatch_*` (which exercise the empty-`JobID` self-create path) pass.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go internal/worker/tasks/metadata_refresh_test.go
git commit -m "feat: metadata dispatch worker populates handler-created job row (#890)"
```

---

## Task 3: Metadata dispatch worker — empty-results finalization (test first)

**Files:**
- Test: `internal/worker/tasks/metadata_refresh_test.go`

The implementation already landed in Task 2 (the `len(games) == 0 && handlerOwned` branch). This task adds the regression test that locks it in.

- [ ] **Step 1: Write the test**

Append to `internal/worker/tasks/metadata_refresh_test.go`:

```go
func TestMetadataRefreshDispatch_HandlerOwned_EmptyFinalizesCompleted(t *testing.T) {
	truncateAllTables(t)
	adminID := insertMetaRefreshAdminUser(t)
	// No games inserted.

	ctx := context.Background()
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{
		Args: tasks.MetadataRefreshDispatchArgs{JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("status: want completed, got %s", status)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshDispatch_HandlerOwned_EmptyFinalizesCompleted -v`
Expected: PASS (implementation from Task 2 covers it).

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/metadata_refresh_test.go
git commit -m "test: handler-owned metadata refresh with no games finalizes completed (#890)"
```

---

## Task 4: Store-link dispatch worker — handler-owned populate + empty (test first)

**Files:**
- Test: `internal/worker/tasks/store_link_refresh_insert_test.go`
- Modify: `internal/worker/tasks/store_link_refresh.go:97-182`

First inspect the existing insert-test helpers so the new tests reuse them:

- [ ] **Step 1: Confirm the test file's package and imports**

Run: `head -20 internal/worker/tasks/store_link_refresh_insert_test.go`
Confirm it is `package tasks_test`. It does **not** currently import `github.com/google/uuid` — add that import to the file's import block (the shared helpers `insertMetaRefreshAdminUser` and `newTestMetadataRiverClient` from `metadata_refresh_test.go` are in the same package and need no new import). The `external_games` table columns are: `id, user_id, storefront, external_id, title, resolved_igdb_id, is_skipped, is_available, is_subscription, ownership_status, created_at, updated_at` (note the column is `title`, not `name`).

- [ ] **Step 2: Write the handler-owned populate test**

Append to `internal/worker/tasks/store_link_refresh_insert_test.go`:

```go
func TestStoreLinkRefreshDispatch_HandlerOwned_PopulatesExistingRow(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)

	// One resolvable external_games row (steam, no store_link → eligible).
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, created_at, updated_at)
		 VALUES (?, ?, 'steam', '440', 'Team Fortress 2', true, now(), now())`,
		uuid.NewString(), adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert external_game: %v", err)
	}

	// Handler-created pending row.
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'store_link_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	rc := newTestMetadataRiverClient(t)
	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, RiverClient: rc}
	if err := w.Work(ctx, &river.Job[tasks.StoreLinkRefreshDispatchArgs]{
		Args: tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'store_link_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Fatalf("expected 1 job row, got %d", count)
	}
	var status string
	var total int
	_ = testDB.NewRaw(`SELECT status, total_items FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status, &total)
	if status != "processing" {
		t.Errorf("status: want processing, got %s", status)
	}
	if total != 1 {
		t.Errorf("total_items: want 1, got %d", total)
	}
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("job_items: want 1, got %d", itemCount)
	}
}

func TestStoreLinkRefreshDispatch_HandlerOwned_EmptyFinalizesCompleted(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)
	// No external_games rows → 0 groups.

	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'store_link_refresh', 'system', 'pending', 'low', 0, now())`,
		jobID, adminID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.StoreLinkRefreshDispatchArgs]{
		Args: tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID},
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("status: want completed, got %s", status)
	}
}
```

> Both tests reuse `insertMetaRefreshAdminUser` (shared helper inserting username `admin`) — fine because each test calls `truncateAllTables` first. They need `uuid` imported in this file (Step 1).

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefreshDispatch_HandlerOwned -v`
Expected: FAIL — current worker ignores `JobID`, inserts a second row, leaves the pending row untouched.

- [ ] **Step 4: Rewrite `StoreLinkRefreshDispatchWorker.Work`**

Replace the `Work` method (lines 97-182) with:

```go
func (w *StoreLinkRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshDispatchArgs]) error {
	args := job.Args

	// Self-heal any predecessor wedged by an orphaned item before evaluating the
	// active-job guard, so a stuck job can never block refreshes permanently.
	reapStuckStoreLinkJobs(ctx, w.DB)

	source := models.JobSourceSystem
	if args.Storefront != "" {
		source = args.Storefront
	}

	jobID := args.JobID
	handlerOwned := jobID != ""

	// Only the self-created path runs the active-job guard; for handler-owned jobs
	// the handler owns dedup (and its own pending row would match this guard).
	if !handlerOwned {
		var existing string
		guard := `SELECT id FROM jobs WHERE job_type = ? AND status IN ('pending','processing') AND source = ?`
		guardArgs := []any{models.JobTypeStoreLinkRefresh, source}
		if args.UserID != "" {
			guard += ` AND user_id = ?`
			guardArgs = append(guardArgs, args.UserID)
		}
		guard += ` LIMIT 1`
		err := w.DB.NewRaw(guard, guardArgs...).Scan(ctx, &existing)
		if err == nil {
			slog.Info("store_link_refresh_dispatch: equivalent job active, skipping", "existing", existing)
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("store_link_refresh_dispatch: guard query", "err", err)
			return nil
		}
	}

	groups, total, err := w.SelectGroups(ctx, args)
	if err != nil {
		slog.Error("store_link_refresh_dispatch: select groups", "err", err)
		return nil
	}
	if len(groups) == 0 {
		// A handler-owned row is already pending; finalize it so it never sticks.
		if handlerOwned {
			finalizeJobCompleted(ctx, w.DB, jobID, "store_link_refresh_dispatch: finalize empty", false)
		}
		return nil
	}

	// jobUserID owns the jobs row on the self-created path. Handler-owned rows
	// already have an owner, so skip the lookup there.
	jobUserID := args.UserID
	if !handlerOwned && jobUserID == "" {
		if e := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &jobUserID); e != nil {
			slog.Error("store_link_refresh_dispatch: no admin user", "err", e)
			return nil
		}
	}

	if !handlerOwned {
		jobID = uuid.NewString()
	}
	itemIDs := make([]string, 0, len(groups))
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if handlerOwned {
			if _, e := tx.NewRaw(
				`UPDATE jobs SET total_items = ?, status = 'processing' WHERE id = ?`,
				len(groups), jobID,
			).Exec(ctx); e != nil {
				return fmt.Errorf("update job: %w", e)
			}
		} else {
			if _, e := tx.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
				 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
				jobID, jobUserID, models.JobTypeStoreLinkRefresh, source, len(groups),
			).Exec(ctx); e != nil {
				return fmt.Errorf("insert job: %w", e)
			}
		}
		for _, g := range groups {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)
			meta, _ := json.Marshal(map[string]any{"storefront": g.Storefront, "force": args.Force}) //nolint:errcheck // fixed map
			if _, e := tx.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
				itemID, jobID, g.UserID, g.Storefront, g.Storefront, json.RawMessage(meta),
			).Exec(ctx); e != nil {
				return fmt.Errorf("insert job_item: %w", e)
			}
		}
		return nil
	}); err != nil {
		slog.Error("store_link_refresh_dispatch: tx failed", "err", err)
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "store_link_refresh_dispatch", Error: err.Error()},
		})
		return nil
	}

	for _, itemID := range itemIDs {
		if e := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, StoreLinkRefreshItemArgs{JobItemID: itemID}); e != nil {
			slog.Error("store_link_refresh_dispatch: enqueue item", "err", e, "item_id", itemID)
		}
	}
	slog.Info("store_link_refresh_dispatch: job created", "job_id", jobID, "groups", len(groups), "rows", total)
	return nil
}
```

- [ ] **Step 5: Run the store-link dispatch tests**

Run: `go test ./internal/worker/tasks/ -run TestStoreLinkRefreshDispatch -v`
Expected: PASS — new handler-owned + empty tests plus all existing store-link dispatch tests (self-create path) pass.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/store_link_refresh.go internal/worker/tasks/store_link_refresh_insert_test.go
git commit -m "feat: store-link dispatch worker populates handler-created job row (#890)"
```

---

## Task 5: Handler helper `startMaintenanceRefresh`

**Files:**
- Modify: `internal/api/games.go` (imports + new method near the other handlers)

- [ ] **Step 1: Add imports**

In `internal/api/games.go`, add to the stdlib import group:

```go
	"database/sql"
```

and to the third-party group:

```go
	"github.com/google/uuid"
```

(Place `"database/sql"` after `"context"`; place the `uuid` import alongside the other `github.com/...` third-party imports.)

- [ ] **Step 2: Add the helper method**

Add this method to `internal/api/games.go` (just above `HandleStartMetadataRefreshJob`):

```go
// startMaintenanceRefresh runs the "already active?" guard for a maintenance
// refresh and, when none is active, synchronously inserts a minimal pending jobs
// row owned by userID so the caller can return a real job_id immediately. It
// returns the job id to report to the client and created=true only when this
// call inserted the row; created=false means an equivalent job (same job_type +
// source) was already active — its id is returned and the caller must NOT enqueue
// a dispatch. Everything runs in one transaction so the guard and insert are atomic.
func (h *GamesHandler) startMaintenanceRefresh(ctx context.Context, userID, jobType, source string) (jobID string, created bool, err error) {
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var existing string
		e := tx.NewRaw(
			`SELECT id FROM jobs WHERE job_type = ? AND source = ? AND status IN ('pending','processing') LIMIT 1`,
			jobType, source,
		).Scan(ctx, &existing)
		if e == nil {
			jobID, created = existing, false
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		jobID, created = uuid.NewString(), true
		_, e = tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'pending', 'low', 0, now())`,
			jobID, userID, jobType, source,
		).Exec(ctx)
		return e
	})
	return jobID, created, err
}
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./internal/api/`
Expected: no output. (`bun` and `errors` are already imported in games.go.)

- [ ] **Step 4: Commit**

```bash
git add internal/api/games.go
git commit -m "feat: add startMaintenanceRefresh helper for eager job-row creation (#890)"
```

---

## Task 6: Rewrite the two start handlers (test first)

**Files:**
- Test: `internal/api/games_test.go`
- Modify: `internal/api/games.go:363-410`

- [ ] **Step 1: Write the metadata-handler test and extend the store-link test**

Add to `internal/api/games_test.go`:

```go
func TestHandleStartMetadataRefreshJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)

	t.Run("non-admin gets 403", func(t *testing.T) {
		_, regTok := setupRegularUser(t, testDB, e, "mr-nonadmin")
		rec := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, regTok)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("admin gets 200 with a real job_id and a pending row", func(t *testing.T) {
		_, adminTok := setupAdminUser(t, testDB, e, "mr-admin")
		rec := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}
		var body struct {
			Success bool   `json:"success"`
			JobID   string `json:"job_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.JobID == "" {
			t.Fatalf("expected non-empty job_id")
		}
		var status string
		if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, body.JobID).Scan(context.Background(), &status); err != nil {
			t.Fatalf("job row not found for returned id: %v", err)
		}
		if status != "pending" {
			t.Errorf("status: want pending, got %s", status)
		}
	})

	t.Run("second start while active returns the same id and no duplicate", func(t *testing.T) {
		truncateAllTables(t)
		e := newTestEchoWithPool(t, testDB)
		_, adminTok := setupAdminUser(t, testDB, e, "mr-admin2")

		first := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		second := postJSONAuth(t, e, "/api/games/metadata/refresh-job", nil, adminTok)
		if second.Code != http.StatusOK {
			t.Fatalf("expected 200 on second call, got %d: %s", second.Code, second.Body)
		}
		idOf := func(rec *httptest.ResponseRecorder) string {
			var b struct {
				JobID string `json:"job_id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &b)
			return b.JobID
		}
		if idOf(first) == "" || idOf(first) != idOf(second) {
			t.Fatalf("expected identical non-empty ids, got %q and %q", idOf(first), idOf(second))
		}
		var count int
		_ = testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
		if count != 1 {
			t.Errorf("expected exactly 1 job row, got %d", count)
		}
	})
}
```

Then extend `TestHandleStartStoreLinkRefreshJob`'s admin subtest to also assert a real `job_id` and a pending row. Replace the body of its `"admin gets 200 ..."` subtest with:

```go
	t.Run("admin gets 200 with a real job_id, a pending row, and a dispatch", func(t *testing.T) {
		_, adminTok := setupAdminUser(t, testDB, e, "slr-admin")
		rec := postJSONAuth(t, e, "/api/games/store-links/refresh-job", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
		}
		var body struct {
			JobID string `json:"job_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.JobID == "" {
			t.Fatalf("expected non-empty job_id")
		}
		var status string
		if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, body.JobID).Scan(context.Background(), &status); err != nil {
			t.Fatalf("job row not found: %v", err)
		}
		if status != "pending" {
			t.Errorf("status: want pending, got %s", status)
		}
		var n int
		if err := testDB.NewRaw(
			`SELECT count(*) FROM river_job WHERE kind = 'store_link_refresh_dispatch'`,
		).Scan(context.Background(), &n); err != nil {
			t.Fatalf("count river_job: %v", err)
		}
		if n < 1 {
			t.Fatalf("expected at least 1 store_link_refresh_dispatch river job, got %d", n)
		}
	})
```

> If `httptest` is not yet imported in `games_test.go`, add `"net/http/httptest"`. Verify the existing `setupAdminUser`/`setupRegularUser`/`postJSONAuth`/`newTestEchoWithPool` helper names by grepping the test file before running.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/api/ -run 'TestHandleStartMetadataRefreshJob|TestHandleStartStoreLinkRefreshJob' -v`
Expected: FAIL — current handlers return empty/absent `job_id` and create no row.

- [ ] **Step 3: Rewrite `HandleStartMetadataRefreshJob`**

Replace lines 363-387 with:

```go
func (h *GamesHandler) HandleStartMetadataRefreshJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	if !auth.IsAdminFromContext(c) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}

	if h.riverClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "worker not available")
	}

	ctx := c.Request().Context()
	jobID, created, err := h.startMaintenanceRefresh(ctx, userID, models.JobTypeMetadataRefresh, models.JobSourceSystem)
	if err != nil {
		slog.Error("failed to start metadata refresh", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue metadata refresh")
	}

	if created {
		if _, err := h.riverClient.Insert(ctx, tasks.MetadataRefreshDispatchArgs{JobID: jobID}, nil); err != nil {
			slog.Error("failed to enqueue metadata refresh dispatch", "err", err)
			// Roll back the pending row so a failed enqueue leaves no orphan.
			if _, derr := h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(ctx); derr != nil {
				slog.Error("failed to roll back metadata refresh job row", "err", derr, "job_id", jobID)
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue metadata refresh")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Metadata refresh job queued",
		"job_id":  jobID,
	})
}
```

- [ ] **Step 4: Rewrite `HandleStartStoreLinkRefreshJob`**

Replace lines 389-410 with:

```go
func (h *GamesHandler) HandleStartStoreLinkRefreshJob(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !auth.IsAdminFromContext(c) {
		return echo.NewHTTPError(http.StatusForbidden, "admin access required")
	}
	if h.riverClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "worker not available")
	}

	ctx := c.Request().Context()
	jobID, created, err := h.startMaintenanceRefresh(ctx, userID, models.JobTypeStoreLinkRefresh, models.JobSourceSystem)
	if err != nil {
		slog.Error("failed to start store-link refresh", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue store link refresh")
	}

	if created {
		if _, err := h.riverClient.Insert(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: true, JobID: jobID}, nil); err != nil {
			slog.Error("failed to enqueue store-link refresh dispatch", "err", err)
			if _, derr := h.db.NewRaw(`DELETE FROM jobs WHERE id = ?`, jobID).Exec(ctx); derr != nil {
				slog.Error("failed to roll back store-link refresh job row", "err", derr, "job_id", jobID)
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to queue store link refresh")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Store link refresh job queued",
		"job_id":  jobID,
	})
}
```

- [ ] **Step 5: Run the handler tests**

Run: `go test ./internal/api/ -run 'TestHandleStartMetadataRefreshJob|TestHandleStartStoreLinkRefreshJob' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/games.go internal/api/games_test.go
git commit -m "fix: maintenance start handlers create jobs row and return real job_id (#890)"
```

---

## Task 7: Frontend — `startStoreLinkRefreshJob` returns `jobId`

**Files:**
- Modify: `ui/frontend/src/api/admin.ts:129-135`

- [ ] **Step 1: Update the function**

Replace lines 129-135 with:

```ts
/**
 * Start a store-link refresh job (admin only). Re-resolves every storefront
 * product link from upstream. Returns the real job id created server-side.
 */
export async function startStoreLinkRefreshJob(): Promise<{
  success: boolean;
  message: string;
  jobId: string;
}> {
  const response = await api.post<{
    success: boolean;
    message: string;
    job_id: string;
  }>('/games/store-links/refresh-job', {});
  return {
    success: response.success,
    message: response.message,
    jobId: response.job_id,
  };
}
```

- [ ] **Step 2: Typecheck**

Run: `cd ui/frontend && npm run check`
Expected: passes (no type errors). `maintenance.tsx` does not yet read the return value, so nothing breaks.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/api/admin.ts
git commit -m "feat: startStoreLinkRefreshJob returns the real job id (#890)"
```

---

## Task 8: Frontend — remove the `eager` option from `useJobTypeStatus`

**Files:**
- Modify: `ui/frontend/src/hooks/use-jobs.ts:108-130` (the `useJobTypeStatus` definition)
- Modify: `ui/frontend/src/hooks/use-jobs.test.ts`

- [ ] **Step 1: Remove the `eager` option**

The current hook (use-jobs.ts) reads:

```ts
/**
 * Polls every 30 s at baseline and every 3 s while a job is active — the
 * baseline poll catches background jobs and reliably detects completion.
 *
 * `eager` forces the fast 3 s cadence even while no job is active yet. The
 * maintenance page sets it for a bounded window right after starting a job:
 * the server-side `jobs` row is created asynchronously by the dispatch worker,
 * so without this the new job wouldn't be detected until the next 30 s poll.
 */
export function useJobTypeStatus(
  jobType: JobType,
  options?: { enabled?: boolean; eager?: boolean },
) {
  return useQuery({
    queryKey: jobsKeys.typeStatus(jobType),
    queryFn: () => jobsApi.getJobTypeStatus(jobType),
    enabled: options?.enabled !== false,
    refetchInterval: (query) => {
      if (options?.eager) return 3000;
      const data = query.state.data as JobTypeStatus | undefined;
      return data?.isActive ? 3000 : 30000;
    },
  });
}
```

Replace it with (drop the `eager` doc paragraph, the `eager` option, and the `if (options?.eager)` line; **keep `enabled: options?.enabled !== false` exactly**):

```ts
/**
 * Polls every 30 s at baseline and every 3 s while a job is active — the
 * baseline poll catches background jobs and reliably detects completion.
 */
export function useJobTypeStatus(jobType: JobType, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.typeStatus(jobType),
    queryFn: () => jobsApi.getJobTypeStatus(jobType),
    enabled: options?.enabled !== false,
    refetchInterval: (query) => {
      const data = query.state.data as JobTypeStatus | undefined;
      return data?.isActive ? 3000 : 30000;
    },
  });
}
```

- [ ] **Step 2: Update `use-jobs.test.ts`**

Run: `grep -n "eager" ui/frontend/src/hooks/use-jobs.test.ts`
Remove the test that asserts `eager` drives the 3 s cadence (added in #889). Keep the other `useJobTypeStatus` cadence tests.

- [ ] **Step 3: Typecheck (will fail until Task 9)**

Run: `cd ui/frontend && npm run check`
Expected: FAIL — `maintenance.tsx` still passes `{ eager: eagerPoll }`. That is fixed in Task 9; do Tasks 8 and 9 back-to-back and commit together at the end of Task 9.

---

## Task 9: Frontend — pin display to returned id, remove the workaround

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.test.ts`

- [ ] **Step 1: Replace the eager-poll + dismissal machinery with a pinned started id**

In `maintenance.tsx`:

- Remove state/refs: `dismissedJobId`, `eagerPoll`, `eagerTimerRef`, and the unmount cleanup `useEffect` that clears `eagerTimerRef`.
- Add: `const [startedJobId, setStartedJobId] = useState<string | null>(null);`
- Remove `beginEagerPoll`.
- Change the two `useJobTypeStatus` calls to drop the `{ eager }` option:

```ts
  const { data: refreshStatus } = useJobTypeStatus(JobType.METADATA_REFRESH);
  const { data: storeLinkStatus } = useJobTypeStatus(JobType.STORE_LINK_REFRESH);
```

- Replace `displayJobId` resolution. The display id is: the just-started job if set, else any active job, else most-recent-completed:

```ts
  const candidateJobId = candidateDisplayJobId(refreshStatus, storeLinkStatus);
  const displayJobId = startedJobId ?? candidateJobId;
  const { data: activeMaintenanceJob } = useJob(displayJobId ?? undefined);
```

- In `handleStartMetadataRefresh`, capture the returned id and pin it:

```ts
  const handleStartMetadataRefresh = async () => {
    try {
      setIsRefreshLoading(true);
      const { jobId } = await adminApi.startMetadataRefreshJob();
      setStartedJobId(jobId);
      toast.success('Metadata refresh job started');
      queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(JobType.METADATA_REFRESH) });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start metadata refresh';
      toast.error(message);
    } finally {
      setIsRefreshLoading(false);
    }
  };
```

- Do the same in `handleStartStoreLinkRefresh` (capture `jobId`, `setStartedJobId(jobId)`, invalidate the store-link type-status key).
- Replace `handleDismissJob` to clear the pinned id and dismiss via candidate. Since `dismissedJobId` is gone, "Start New" now just unpins so the cards reappear:

```ts
  const handleDismissJob = () => {
    setStartedJobId(null);
  };
```

> Note: clearing `startedJobId` falls back to `candidateDisplayJobId`, which returns the most-recent-completed job — re-showing the card the user just dismissed. To avoid that, gate the completed-job display on the pinned id. Simplest correct behavior: when the user presses "Start New", hide the terminal card until a new job is started. Implement by tracking whether the displayed job is the pinned one:

```ts
  // After "Start New", suppress the completed-fallback until a new start pins an id.
  const [showCompletedFallback, setShowCompletedFallback] = useState(true);
  const displayJobId = startedJobId ?? (showCompletedFallback ? candidateJobId : undefined);
```

and in `handleDismissJob`: `setStartedJobId(null); setShowCompletedFallback(false);`
and at the top of both start handlers (before the await): `setShowCompletedFallback(true);`

- [ ] **Step 2: Prune now-dead helpers**

`resolveDisplayJobId` is no longer referenced. Remove it. Keep `candidateDisplayJobId` and `mostRecentCompletedJobId` (still used). Run knip to confirm nothing else is orphaned:

Run: `cd ui/frontend && npm run knip`
Expected: no findings. If `resolveDisplayJobId` or any removed symbol is still flagged, delete it and its test.

- [ ] **Step 3: Update `maintenance.test.ts`**

Run: `grep -n "resolveDisplayJobId\|dismissedJobId\|eager" ui/frontend/src/routes/_authenticated/admin/maintenance.test.ts`
- Remove tests for `resolveDisplayJobId` (deleted).
- Keep/adjust `candidateDisplayJobId` and `mostRecentCompletedJobId` unit tests.
- The #884 regression intent ("a dismissed completed job is not resurrected") now maps to: pinning a started id shows it immediately, and after "Start New" the completed fallback is suppressed. Add a small unit test for the new resolution if the helpers expose it; otherwise rely on the component-level behavior covered by the existing suite.

- [ ] **Step 4: Typecheck, knip, and frontend tests**

Run: `cd ui/frontend && npm run check && npm run knip && npm run test`
Expected: all pass.

- [ ] **Step 5: Commit (Tasks 8 + 9 together)**

```bash
git add ui/frontend/src/hooks/use-jobs.ts ui/frontend/src/hooks/use-jobs.test.ts ui/frontend/src/routes/_authenticated/admin/maintenance.tsx ui/frontend/src/routes/_authenticated/admin/maintenance.test.ts
git commit -m "refactor: pin maintenance display to returned job id, drop eager-poll workaround (#890)"
```

---

## Task 10: Full verification

- [ ] **Step 1: Backend build + targeted tests**

Run: `go build ./... && go test ./internal/worker/tasks/ ./internal/api/ -count=1`
Expected: PASS.

- [ ] **Step 2: Lint**

Run: `golangci-lint run ./internal/api/... ./internal/worker/...`
Expected: no findings. (Watch for `errcheck` on any new `_ =` discards — the plan's handler rollback handles its error explicitly; worker `finalizeJobCompleted` calls are log-only helpers.)

- [ ] **Step 3: Frontend gates**

Run: `cd ui/frontend && npm run check && npm run knip && npm run test`
Expected: all pass.

- [ ] **Step 4: Push (runs the pre-push hard gate)**

```bash
git push -u origin fix/890-eager-maintenance-job-row
```

Expected: pre-push runs full `go test ./...` and the frontend suite; both pass.

---

## Notes for the implementer

- **River-after-commit constraint:** the handler's `RunInTx` commits the pending `jobs` row before `riverClient.Insert` runs (separate connection), so the dispatch worker always sees the row. Do not move the Insert inside the tx.
- **Why the worker guard is conditional:** if a handler-owned worker re-ran the active-job guard, it would find the very row the handler just created (status `pending`) and skip, doing nothing. The handler owns dedup for that path.
- **Out of scope:** the ignored `game_ids` param on the metadata endpoint; any change to periodic-schedule or sync cadence.
- **Existing tests are the self-create regression net:** every pre-existing `TestMetadataRefreshDispatch_*` / store-link dispatch test runs with empty `JobID` and must keep passing unchanged.
