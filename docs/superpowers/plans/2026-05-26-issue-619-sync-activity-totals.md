# Sync Activity Totals Reconciliation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every game processed by a sync job has exactly one `sync_changes` row describing its outcome, so the Recent Activity UI totals reconcile to the full count of matched + skipped games.

**Architecture:** Two new `change_type` values (`already_in_library`, `skipped`) are written by the sync worker and the skip API handler. `HandleRecentJobs` surfaces both as new arrays in its response. The frontend renders them as collapsible lists alongside the existing `added`, `removed`, and `status_changed` buckets. No DB migration required — `change_type` is a free-form TEXT column.

**Tech Stack:** Go (Bun ORM, Echo v5, River), TypeScript (React 19, TanStack Query, Vitest, MSW), PostgreSQL.

---

## File Map

| File | Change |
|---|---|
| `internal/worker/tasks/sync.go` | Add `platformUpgraded` flag; write `skipped` and `already_in_library` sync_changes rows |
| `internal/api/sync.go` | Extend `HandleSkipGame` to fetch title and write `sync_changes('skipped')` |
| `internal/api/jobs.go` | Add `SkippedItems` and `AlreadyInLibraryItems` to `jobWithChanges`; handle new cases in switch |
| `ui/frontend/src/api/jobs.ts` | Add `skipped_items` and `already_in_library_items` to `RecentJobDetailApiResponse`; map in `transformRecentJob` |
| `ui/frontend/src/types/jobs.ts` | Add `skippedItems` and `alreadyInLibraryItems` to `RecentJobDetail` |
| `ui/frontend/src/components/sync/recent-activity.tsx` | Add two new `SyncChangeList` rows; update `formatSummary` |
| `internal/worker/tasks/sync_test.go` | Add two worker tests: `already_in_library` and `skipped` outcomes |
| `internal/api/sync_test.go` | Extend `TestSkipGame_MarksJobItemSkippedAndCompletesJob` to assert sync_changes row |
| `ui/frontend/src/components/sync/recent-activity.test.tsx` | New: unit tests for `formatSummary` and `SyncChangeList` rendering |

---

## Task 1: Worker — `already_in_library` sync_change

**Files:**
- Modify: `internal/worker/tasks/sync_test.go`
- Modify: `internal/worker/tasks/sync.go`

- [ ] **Step 1: Write the failing test**

Add this test to `internal/worker/tasks/sync_test.go` after `TestUserGameWorker_OwnershipRankGuard`:

```go
func TestUserGameWorker_AlreadyInLibrary_WritesSyncChange(t *testing.T) {
	// A game whose user_games row already exists with no ownership upgrade
	// must produce a sync_changes('already_in_library') row and no 'added' row.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1001)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Existing Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Pre-seed user_games and user_game_platforms so the game is already in library.
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 10.0, 'owned', true, now(), now())`,
		uuid.NewString(), ugID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1001', 'Existing Game', false, true, false, 'owned', ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 10.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '1001', 'Existing Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have exactly one already_in_library sync_change.
	var alreadyCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'already_in_library'`, jobID,
	).Scan(ctx, &alreadyCount)
	if alreadyCount != 1 {
		t.Errorf("expected 1 already_in_library sync_change, got %d", alreadyCount)
	}

	// Must have zero 'added' rows.
	var addedCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &addedCount)
	if addedCount != 0 {
		t.Errorf("expected 0 added sync_changes, got %d", addedCount)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_AlreadyInLibrary_WritesSyncChange -v -timeout 120s
```

Expected: FAIL — `expected 1 already_in_library sync_change, got 0`

- [ ] **Step 3: Implement `platformUpgraded` flag and `already_in_library` insert**

In `internal/worker/tasks/sync.go`, locate the `egPlatforms` loop (starts just after the `storefrontSlug` resolution). Add a `var platformUpgraded bool` declaration immediately before the loop:

```go
var platformUpgraded bool
for _, egp := range egPlatforms {
```

Inside the loop, in the `default:` branch where `newRank > existingRank` is checked (just before the `status_changed` INSERT), set the flag:

```go
if newRank > existingRank {
    platformUpgraded = true
    // Insert the status_changed sync_change BEFORE the UPDATE so
    // that old_status reflects the pre-UPDATE value.
    if _, err := w.DB.NewRaw(
        `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, old_status, new_status, created_at)
         VALUES (?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title, existingOwnership, &ownership,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (status_changed)", "err", err)
    }
    finalOwnership = ownership
}
```

Then extend the post-loop block. The current code at the end of `Work()` looks like:

```go
if isNewRow.IsNew {
    if _, err := w.DB.NewRaw(
        `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
         VALUES (?, ?, ?, ?, 'added', ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (added)", "err", err)
    }
}

syncMarkItemCompleted(ctx, w.DB, &item)
```

Change it to:

```go
if isNewRow.IsNew {
    if _, err := w.DB.NewRaw(
        `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
         VALUES (?, ?, ?, ?, 'added', ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (added)", "err", err)
    }
} else if !platformUpgraded {
    if _, err := w.DB.NewRaw(
        `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
         VALUES (?, ?, ?, ?, 'already_in_library', ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (already_in_library)", "err", err)
    }
}

syncMarkItemCompleted(ctx, w.DB, &item)
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_AlreadyInLibrary_WritesSyncChange -v -timeout 120s
```

Expected: PASS

- [ ] **Step 5: Run full worker test suite to check for regressions**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all pass. Note: `TestUserGameWorker_OwnershipRankGuard` previously asserted `0 added sync_changes` — it should still pass because `platformUpgraded` will be `false` (no rank upgrade in that test) but `isNewRow.IsNew` is also `false`, so an `already_in_library` row will now be written instead. The test only checks that no `added` row is written, which remains true. If the test also implicitly checks that no sync_change at all is written, update it to assert `change_type='already_in_library'` instead of zero rows total.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "fix(sync): write already_in_library sync_change for games already in library"
```

---

## Task 2: Worker — `skipped` sync_change

**Files:**
- Modify: `internal/worker/tasks/sync_test.go`
- Modify: `internal/worker/tasks/sync.go`

- [ ] **Step 1: Write the failing test**

Add this test to `internal/worker/tasks/sync_test.go` after `TestUserGameWorker_AlreadyInLibrary_WritesSyncChange`:

```go
func TestUserGameWorker_WorkerAutoSkip_WritesSyncChange(t *testing.T) {
	// When eg.IsSkipped=true, the worker must write sync_changes('skipped')
	// before marking the job_item skipped.
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '999', 'Skipped Game', true, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '999', 'Skipped Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// sync_changes('skipped') must exist with the correct title.
	var sc struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
	}
	if err := testDB.NewRaw(
		`SELECT change_type, title FROM sync_changes WHERE job_id = ?`, jobID,
	).Scan(ctx, &sc); err != nil {
		t.Fatalf("scan sync_change: %v", err)
	}
	if sc.ChangeType != "skipped" {
		t.Errorf("change_type: want 'skipped', got %q", sc.ChangeType)
	}
	if sc.Title != "Skipped Game" {
		t.Errorf("title: want 'Skipped Game', got %q", sc.Title)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_WorkerAutoSkip_WritesSyncChange -v -timeout 120s
```

Expected: FAIL — scan returns no rows or wrong change_type

- [ ] **Step 3: Add `skipped` insert in the `eg.IsSkipped` early-return path**

In `internal/worker/tasks/sync.go`, locate the `eg.IsSkipped` block:

```go
// Skipped games: mark the item skipped and check completion.
if eg.IsSkipped {
    if _, err := w.DB.NewRaw(
        `UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: update external_game updated_at (skipped)", "err", err)
    }
    syncMarkItemSkipped(ctx, w.DB, &item)
    SyncCheckJobCompletion(ctx, w.DB, item.JobID)
    return nil
}
```

Change it to:

```go
// Skipped games: record the outcome, mark the item skipped, and check completion.
if eg.IsSkipped {
    if _, err := w.DB.NewRaw(
        `UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: update external_game updated_at (skipped)", "err", err)
    }
    if _, err := w.DB.NewRaw(
        `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
         VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (skipped)", "err", err)
    }
    syncMarkItemSkipped(ctx, w.DB, &item)
    SyncCheckJobCompletion(ctx, w.DB, item.JobID)
    return nil
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_WorkerAutoSkip_WritesSyncChange -v -timeout 120s
```

Expected: PASS

- [ ] **Step 5: Run full worker test suite**

```bash
go test ./internal/worker/tasks/... -timeout 300s 2>&1 | tail -10
```

Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "fix(sync): write skipped sync_change in worker auto-skip path"
```

---

## Task 3: API skip handler — `skipped` sync_change

**Files:**
- Modify: `internal/api/sync_test.go`
- Modify: `internal/api/sync.go`

- [ ] **Step 1: Extend the existing skip test to assert sync_changes**

In `internal/api/sync_test.go`, find `TestSkipGame_MarksJobItemSkippedAndCompletesJob`. After the existing assertions for `itemStatus` and `jobStatus`, add:

```go
// sync_changes('skipped') must be written with the correct title.
var sc struct {
    ChangeType string `bun:"change_type"`
    Title      string `bun:"title"`
}
if err := testDB.NewRaw(
    `SELECT change_type, title FROM sync_changes WHERE job_id = 'job-skip-ji'`,
).Scan(context.Background(), &sc); err != nil {
    t.Fatalf("scan sync_change: %v", err)
}
if sc.ChangeType != "skipped" {
    t.Errorf("sync_change change_type: want 'skipped', got %q", sc.ChangeType)
}
if sc.Title != "Skip Me" {
    t.Errorf("sync_change title: want 'Skip Me', got %q", sc.Title)
}
```

- [ ] **Step 2: Run test to confirm it now fails**

```bash
go test ./internal/api/... -run TestSkipGame_MarksJobItemSkippedAndCompletesJob -v -timeout 120s
```

Expected: FAIL — sync_change not found

- [ ] **Step 3: Update `HandleSkipGame` to fetch title and write sync_change**

In `internal/api/sync.go`, locate `HandleSkipGame`. The current ownership check is:

```go
var ownerID string
err := h.db.NewRaw(`SELECT user_id FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerID)
if errors.Is(err, sql.ErrNoRows) || ownerID != userID {
    return echo.NewHTTPError(http.StatusNotFound, "not found")
}
if err != nil {
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
}
```

Replace it with:

```go
var ownerRow struct {
    UserID string `bun:"user_id"`
    Title  string `bun:"title"`
}
err := h.db.NewRaw(`SELECT user_id, title FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerRow)
if errors.Is(err, sql.ErrNoRows) || ownerRow.UserID != userID {
    return echo.NewHTTPError(http.StatusNotFound, "not found")
}
if err != nil {
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
}
```

Then find the block that marks the job_item skipped and calls `SyncCheckJobCompletion`:

```go
if err := h.db.NewRaw(`
    SELECT id, job_id FROM job_items
    WHERE external_game_id = ? AND status IN ('pending_review', 'pending')
    ORDER BY created_at DESC
    LIMIT 1`, id,
).Scan(ctx, &jobItemRow); err == nil {
    if _, err := h.db.NewRaw(
        `UPDATE job_items SET status = 'skipped', processed_at = now() WHERE id = ?`,
        jobItemRow.ID,
    ).Exec(ctx); err != nil {
        slog.Error("sync: skip game: mark job_item skipped", "err", err, "job_item_id", jobItemRow.ID)
    } else {
        tasks.SyncCheckJobCompletion(ctx, h.db, jobItemRow.JobID)
    }
}
```

Change it to:

```go
if err := h.db.NewRaw(`
    SELECT id, job_id FROM job_items
    WHERE external_game_id = ? AND status IN ('pending_review', 'pending')
    ORDER BY created_at DESC
    LIMIT 1`, id,
).Scan(ctx, &jobItemRow); err == nil {
    if _, err := h.db.NewRaw(
        `UPDATE job_items SET status = 'skipped', processed_at = now() WHERE id = ?`,
        jobItemRow.ID,
    ).Exec(ctx); err != nil {
        slog.Error("sync: skip game: mark job_item skipped", "err", err, "job_item_id", jobItemRow.ID)
    } else {
        if _, err := h.db.NewRaw(
            `INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
             VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
            uuid.NewString(), jobItemRow.JobID, userID, id, ownerRow.Title,
        ).Exec(ctx); err != nil {
            slog.Error("sync: skip game: insert sync_change (skipped)", "err", err)
        }
        tasks.SyncCheckJobCompletion(ctx, h.db, jobItemRow.JobID)
    }
}
```

Also update any remaining references to `ownerID` in the function to use `ownerRow.UserID`. Check the full function body — there should be none after the ownership check, but verify.

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./internal/api/... -run TestSkipGame_MarksJobItemSkippedAndCompletesJob -v -timeout 120s
```

Expected: PASS

- [ ] **Step 5: Run full API test suite**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "fix(sync): write skipped sync_change in HandleSkipGame"
```

---

## Task 4: API — extend `HandleRecentJobs` response

**Files:**
- Modify: `internal/api/jobs.go`

No new test needed — the SQL query already fetches all `sync_changes` rows for the job without filtering by `change_type`. The existing jobs API tests cover the handler at a structural level; the new fields will appear as empty arrays in old test data (no regression).

- [ ] **Step 1: Extend `jobWithChanges` and the switch**

In `internal/api/jobs.go`, find the `HandleRecentJobs` function. The local `jobWithChanges` struct currently is:

```go
type jobWithChanges struct {
    models.Job
    Progress           map[string]any   `json:"progress"`
    AddedItems         []syncChangeItem `json:"added_items"`
    RemovedItems       []syncChangeItem `json:"removed_items"`
    StatusChangedItems []syncChangeItem `json:"status_changed_items"`
}
```

Change it to:

```go
type jobWithChanges struct {
    models.Job
    Progress                map[string]any   `json:"progress"`
    AddedItems              []syncChangeItem `json:"added_items"`
    RemovedItems            []syncChangeItem `json:"removed_items"`
    StatusChangedItems      []syncChangeItem `json:"status_changed_items"`
    SkippedItems            []syncChangeItem `json:"skipped_items"`
    AlreadyInLibraryItems   []syncChangeItem `json:"already_in_library_items"`
}
```

Find the slice initialisations:

```go
addedItems := []syncChangeItem{}
removedItems := []syncChangeItem{}
statusChangedItems := []syncChangeItem{}
```

Change to:

```go
addedItems := []syncChangeItem{}
removedItems := []syncChangeItem{}
statusChangedItems := []syncChangeItem{}
skippedItems := []syncChangeItem{}
alreadyInLibraryItems := []syncChangeItem{}
```

Find the switch block:

```go
switch sc.ChangeType {
case "added":
    addedItems = append(addedItems, syncChangeItem{Title: sc.Title})
case "removed":
    removedItems = append(removedItems, syncChangeItem{Title: sc.Title})
case "status_changed":
    statusChangedItems = append(statusChangedItems, syncChangeItem{
        Title: sc.Title, OldStatus: sc.OldStatus, NewStatus: sc.NewStatus,
    })
}
```

Change to:

```go
switch sc.ChangeType {
case "added":
    addedItems = append(addedItems, syncChangeItem{Title: sc.Title})
case "removed":
    removedItems = append(removedItems, syncChangeItem{Title: sc.Title})
case "status_changed":
    statusChangedItems = append(statusChangedItems, syncChangeItem{
        Title: sc.Title, OldStatus: sc.OldStatus, NewStatus: sc.NewStatus,
    })
case "skipped":
    skippedItems = append(skippedItems, syncChangeItem{Title: sc.Title})
case "already_in_library":
    alreadyInLibraryItems = append(alreadyInLibraryItems, syncChangeItem{Title: sc.Title})
}
```

Find the `result = append(result, jobWithChanges{...})` call and add the two new fields:

```go
result = append(result, jobWithChanges{
    Job:                   j,
    Progress:              progress,
    AddedItems:            addedItems,
    RemovedItems:          removedItems,
    StatusChangedItems:    statusChangedItems,
    SkippedItems:          skippedItems,
    AlreadyInLibraryItems: alreadyInLibraryItems,
})
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors

- [ ] **Step 3: Run API tests**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add internal/api/jobs.go
git commit -m "fix(sync): surface skipped and already_in_library items in recent jobs API"
```

---

## Task 5: Frontend — types and API transform

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts`
- Modify: `ui/frontend/src/api/jobs.ts`

- [ ] **Step 1: Add fields to `RecentJobDetail` in types.ts**

In `ui/frontend/src/types/jobs.ts`, find the `RecentJobDetail` interface:

```typescript
export interface RecentJobDetail {
  id: string;
  status: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  addedItems: SyncChangeItem[];
  removedItems: SyncChangeItem[];
  statusChangedItems: SyncChangeItem[];
}
```

Change to:

```typescript
export interface RecentJobDetail {
  id: string;
  status: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  addedItems: SyncChangeItem[];
  removedItems: SyncChangeItem[];
  statusChangedItems: SyncChangeItem[];
  skippedItems: SyncChangeItem[];
  alreadyInLibraryItems: SyncChangeItem[];
}
```

- [ ] **Step 2: Extend `RecentJobDetailApiResponse` and `transformRecentJob` in api/jobs.ts**

In `ui/frontend/src/api/jobs.ts`, find `RecentJobDetailApiResponse`:

```typescript
interface RecentJobDetailApiResponse {
  id: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  progress: { ... };
  added_items: SyncChangeItemApiResponse[];
  removed_items: SyncChangeItemApiResponse[];
  status_changed_items: SyncChangeItemApiResponse[];
}
```

Add the two new optional fields:

```typescript
interface RecentJobDetailApiResponse {
  id: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  progress: {
    completed: number;
    skipped: number;
    failed: number;
    pending: number;
    processing: number;
    pending_review: number;
    total: number;
    percent: number;
  };
  added_items: SyncChangeItemApiResponse[];
  removed_items: SyncChangeItemApiResponse[];
  status_changed_items: SyncChangeItemApiResponse[];
  skipped_items?: SyncChangeItemApiResponse[];
  already_in_library_items?: SyncChangeItemApiResponse[];
}
```

In `transformRecentJob`, add the two new fields:

```typescript
function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  const p = api.progress ?? { completed: 0, skipped: 0, failed: 0, pending: 0, processing: 0, pending_review: 0, total: 0, percent: 0 };
  return {
    id: api.id,
    status: api.status,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    totalItems: api.total_items,
    completedCount: p.completed,
    skippedCount: p.skipped,
    failedCount: p.failed,
    addedItems: (api.added_items ?? []).map(transformSyncChangeItem),
    removedItems: (api.removed_items ?? []).map(transformSyncChangeItem),
    statusChangedItems: (api.status_changed_items ?? []).map(transformSyncChangeItem),
    skippedItems: (api.skipped_items ?? []).map(transformSyncChangeItem),
    alreadyInLibraryItems: (api.already_in_library_items ?? []).map(transformSyncChangeItem),
  };
}
```

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/types/jobs.ts ui/frontend/src/api/jobs.ts
git commit -m "fix(sync): add skippedItems and alreadyInLibraryItems to RecentJobDetail types"
```

---

## Task 6: Frontend — component update and tests

**Files:**
- Modify: `ui/frontend/src/components/sync/recent-activity.tsx`
- Create: `ui/frontend/src/components/sync/recent-activity.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `ui/frontend/src/components/sync/recent-activity.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RecentActivity } from './recent-activity';
import type { RecentJobDetail, SyncChangeItem } from '@/types';

// Suppress TanStack Query console errors in tests.
vi.mock('@/hooks', () => ({
  useRecentJobs: vi.fn(),
}));
import { useRecentJobs } from '@/hooks';

const makeItem = (title: string): SyncChangeItem => ({ title });

const baseJob: RecentJobDetail = {
  id: 'j1',
  status: 'completed',
  createdAt: '2026-01-01T00:00:00Z',
  completedAt: '2026-01-01T01:00:00Z',
  totalItems: 10,
  completedCount: 8,
  skippedCount: 2,
  failedCount: 0,
  addedItems: [makeItem('New Game A'), makeItem('New Game B')],
  removedItems: [],
  statusChangedItems: [],
  skippedItems: [],
  alreadyInLibraryItems: [],
};

describe('RecentActivity component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders skippedItems section when items are present', async () => {
    const job: RecentJobDetail = {
      ...baseJob,
      skippedItems: [makeItem('Skipped Game 1'), makeItem('Skipped Game 2')],
    };
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [job] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    // Expand the job card first.
    const trigger = screen.getByRole('button', { name: /completed/i });
    await userEvent.click(trigger);

    expect(screen.getByText('Skipped')).toBeInTheDocument();
  });

  it('renders alreadyInLibraryItems section when items are present', async () => {
    const job: RecentJobDetail = {
      ...baseJob,
      alreadyInLibraryItems: [makeItem('Old Game A'), makeItem('Old Game B'), makeItem('Old Game C')],
    };
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [job] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    const trigger = screen.getByRole('button', { name: /completed/i });
    await userEvent.click(trigger);

    expect(screen.getByText('Already in library')).toBeInTheDocument();
  });

  it('does not render skippedItems section when array is empty', () => {
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [baseJob] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    expect(screen.queryByText('Skipped')).not.toBeInTheDocument();
  });

  it('does not render alreadyInLibraryItems section when array is empty', () => {
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [baseJob] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    expect(screen.queryByText('Already in library')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd ui/frontend && npm run test -- recent-activity.test.tsx
```

Expected: FAIL — `Skipped` and `Already in library` not found in DOM

- [ ] **Step 3: Update the component**

In `ui/frontend/src/components/sync/recent-activity.tsx`:

**Update the lucide-react import** (currently imports `Clock, ChevronDown, ChevronRight, CheckCircle, XCircle, ArrowRight`) to also include `SkipForward` and `BookMarked`:

```typescript
import {
  Clock,
  ChevronDown,
  ChevronRight,
  CheckCircle,
  XCircle,
  ArrowRight,
  SkipForward,
  BookMarked,
} from 'lucide-react';
```

**Update `formatSummary`** (currently only checks addedItems, removedItems, statusChangedItems):

```typescript
function formatSummary(job: RecentJobDetail): string {
  const parts: string[] = [];
  if (job.addedItems.length > 0) parts.push(`${job.addedItems.length} added`);
  if (job.removedItems.length > 0) parts.push(`${job.removedItems.length} removed`);
  if (job.statusChangedItems.length > 0) parts.push(`${job.statusChangedItems.length} status changed`);
  if (job.alreadyInLibraryItems.length > 0) parts.push(`${job.alreadyInLibraryItems.length} already in library`);
  if (job.skippedItems.length > 0) parts.push(`${job.skippedItems.length} skipped`);
  return parts.join(' · ');
}
```

**Update the `JobCard` body** (inside `CollapsibleContent`, after the `statusChangedItems` SyncChangeList) to add two new lists:

```tsx
<SyncChangeList
  items={job.alreadyInLibraryItems}
  label="Already in library"
  icon={<BookMarked className="h-4 w-4 text-muted-foreground" />}
/>
<SyncChangeList
  items={job.skippedItems}
  label="Skipped"
  icon={<SkipForward className="h-4 w-4 text-muted-foreground" />}
/>
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd ui/frontend && npm run test -- recent-activity.test.tsx
```

Expected: PASS

- [ ] **Step 5: Type-check and knip**

```bash
cd ui/frontend && npm run check && npm run knip
```

Expected: no errors, no unused exports

- [ ] **Step 6: Run full frontend test suite**

```bash
cd ui/frontend && npm run test
```

Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/sync/recent-activity.tsx ui/frontend/src/components/sync/recent-activity.test.tsx
git commit -m "fix(sync): render skipped and already_in_library buckets in Recent Activity"
```

---

## Task 7: Final quality gates

- [ ] **Step 1: Run full Go test suite**

```bash
go test -timeout 600s ./...
```

Expected: all pass

- [ ] **Step 2: Run Go linter**

```bash
golangci-lint run
```

Expected: no new issues

- [ ] **Step 3: Run full frontend suite (check + knip + test)**

```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```

Expected: all pass

- [ ] **Step 4: Close the loop — push branch and open PR**

```bash
git push -u origin issue-619-sync-activity-totals
```

Open a PR targeting `main` with title: `fix(sync): reconcile activity totals — record skipped and already_in_library outcomes`
