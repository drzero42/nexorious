# Unify Recent Activity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the two divergent Recent Activity implementations into one component, one hook, and one endpoint; rename `sync_changes` → `changes`; and make the import worker write per-item change rows (`added` / `updated` / `already_in_library`).

**Architecture:** Keep two backing tables — `events` (coarse audit/notify, untouched) and the renamed per-item `changes`. Sync writers are repointed to `changes`; import gains writers. One generalized `GET /api/jobs/recent` endpoint (filters: `source`, `jobType`, `daysBack`, `limit`) returns the grouped per-outcome shape with empty arrays when a job has no change rows. One frontend `RecentActivity` renders the rich breakdown when change rows exist and falls back to the counts + `JobItemsDetails` view otherwise.

**Tech Stack:** Go 1.25, Bun ORM + `pgdriver`, Echo v5, River; Vite/React 19/TypeScript, TanStack Query, Vitest.

**Spec:** `docs/superpowers/specs/2026-06-02-unify-recent-activity-design.md`

**Branch:** `feat/730-unify-recent-activity` (already created and checked out; the spec is already committed on it).

---

## File Structure

**Backend**
- Create: `internal/db/migrations/20260602000002_rename_sync_changes_to_changes.up.sql` / `.down.sql` — rename table + indexes.
- Modify: `internal/db/models/models.go` — `SyncChange` → `JobChange`, `bun:"table:changes"`.
- Modify: `internal/worker/tasks/sync.go` — repoint 6 inserts to `changes`.
- Modify: `internal/worker/tasks/import_item.go` — write one `changes` row per item.
- Modify: `internal/worker/tasks/import_item_test.go` — assert change rows.
- Modify: `internal/api/jobs.go` — generalize `HandleRecentJobs` (query params, `updated_items` bucket).
- Modify: `internal/api/router.go:287` — `GET /jobs/recent` (drop `:source`).
- Modify: `internal/api/jobs_test.go` — update recent tests to the new URL + add filter/`updated` coverage.
- Modify: `slumber.yaml` — recent request uses query params.

**Frontend**
- Modify: `ui/frontend/src/types/jobs.ts` — extend `RecentJobDetail`; add `updatedItems`.
- Modify: `ui/frontend/src/api/jobs.ts` — `getRecentJobs(filters)`, response interface, transform.
- Modify: `ui/frontend/src/hooks/use-jobs.ts` — `jobsKeys.recent(filters)`, `useRecentJobs(filters)`.
- Rewrite: `ui/frontend/src/components/jobs/recent-activity.tsx` — the one unified component.
- Modify: `ui/frontend/src/components/jobs/recent-activity.test.tsx` — breakdown + fallback tests.
- Delete: `ui/frontend/src/components/sync/recent-activity.tsx` + its test + its `index.ts` export.
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` — import unified component, pass `source`.
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx` — pass `jobTypes`.
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — unchanged props still valid (verify).

---

## Phase A — Backend data model & writers

### Task 1: Rename migration `sync_changes` → `changes`

**Files:**
- Create: `internal/db/migrations/20260602000002_rename_sync_changes_to_changes.up.sql`
- Create: `internal/db/migrations/20260602000002_rename_sync_changes_to_changes.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260602000002_rename_sync_changes_to_changes.up.sql`:
```sql
ALTER TABLE sync_changes RENAME TO changes;
ALTER INDEX sync_changes_job_id_idx     RENAME TO changes_job_id_idx;
ALTER INDEX sync_changes_user_id_idx    RENAME TO changes_user_id_idx;
ALTER INDEX sync_changes_created_at_idx RENAME TO changes_created_at_idx;
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260602000002_rename_sync_changes_to_changes.down.sql`:
```sql
ALTER INDEX changes_created_at_idx RENAME TO sync_changes_created_at_idx;
ALTER INDEX changes_user_id_idx    RENAME TO sync_changes_user_id_idx;
ALTER INDEX changes_job_id_idx     RENAME TO sync_changes_job_id_idx;
ALTER TABLE changes RENAME TO sync_changes;
```

- [ ] **Step 3: Verify migrations are discovered and apply**

Run: `go build ./... && ./nexorious migrate status`
Expected: build succeeds; the new `20260602000002_rename_sync_changes_to_changes` migration is listed (pending or applied). If your dev DB is up, `./nexorious migrate` applies it cleanly.

> Note: the migrations are auto-discovered via `//go:embed *.sql` in `internal/db/migrations/migrations.go` — no registration needed. The package test suites run migrations once at startup, so Task 3's `go test` is the real verification.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260602000002_rename_sync_changes_to_changes.up.sql internal/db/migrations/20260602000002_rename_sync_changes_to_changes.down.sql
git commit -m "feat(db): rename sync_changes table to changes (#730)"
```

### Task 2: Rename the Bun model `SyncChange` → `JobChange`

**Files:**
- Modify: `internal/db/models/models.go:217-230`

- [ ] **Step 1: Rename the struct and table tag**

In `internal/db/models/models.go`, replace the existing block:
```go
// SyncChange mirrors the sync_changes table — one row per library event per sync run.
type SyncChange struct {
	bun.BaseModel `bun:"table:sync_changes"`

	ID             string    `bun:"id,pk"               json:"id"`
	JobID          string    `bun:"job_id,notnull"      json:"job_id"`
	UserID         string    `bun:"user_id,notnull"     json:"user_id"`
	ExternalGameID *string   `bun:"external_game_id"    json:"external_game_id"`
	ChangeType     string    `bun:"change_type,notnull" json:"change_type"`
	Title          string    `bun:"title,notnull"       json:"title"`
	OldStatus      *string   `bun:"old_status"          json:"old_status"`
	NewStatus      *string   `bun:"new_status"          json:"new_status"`
	CreatedAt      time.Time `bun:"created_at,notnull"  json:"created_at"`
}
```
with:
```go
// JobChange mirrors the changes table — one row per library outcome per job run
// (sync, import). The job's type is derived by joining jobs.job_type.
type JobChange struct {
	bun.BaseModel `bun:"table:changes"`

	ID             string    `bun:"id,pk"               json:"id"`
	JobID          string    `bun:"job_id,notnull"      json:"job_id"`
	UserID         string    `bun:"user_id,notnull"     json:"user_id"`
	ExternalGameID *string   `bun:"external_game_id"    json:"external_game_id"`
	ChangeType     string    `bun:"change_type,notnull" json:"change_type"`
	Title          string    `bun:"title,notnull"       json:"title"`
	OldStatus      *string   `bun:"old_status"          json:"old_status"`
	NewStatus      *string   `bun:"new_status"          json:"new_status"`
	CreatedAt      time.Time `bun:"created_at,notnull"  json:"created_at"`
}
```

- [ ] **Step 2: Find any references to the old model name**

Run: `grep -rn "models.SyncChange\|SyncChange{" --include=*.go .`
Expected: no hits outside this struct (the writers use raw SQL, not the model). If any appear, rename them to `JobChange` before continuing.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/db/models/models.go
git commit -m "refactor(models): rename SyncChange model to JobChange (#730)"
```

### Task 3: Repoint all `sync_changes` SQL references to `changes`

**Files:**
- Modify: `internal/worker/tasks/sync.go` (6 inserts at lines ~224, ~291, ~607, ~749, ~790, ~798)
- Modify: any `_test.go` files that insert into `sync_changes`

- [ ] **Step 1: List the remaining references (excluding the migration that does the rename)**

Run: `grep -rn "sync_changes" --include=*.go --include=*.sql . | grep -v "20260602000002_rename_sync_changes_to_changes"`
Expected hits: the 6 `INSERT INTO sync_changes ...` statements in `internal/worker/tasks/sync.go`, plus any test inserts (e.g. in `internal/api/jobs_test.go` helpers). The original `20260503000001_initial.up.sql` *also* contains `sync_changes` (the original CREATE) — **leave it untouched**; the rename migration handles the live schema.

- [ ] **Step 2: Repoint each non-migration reference**

For every hit from Step 1 **except** files under `internal/db/migrations/`, change the table name `sync_changes` → `changes` in the SQL string. The 6 sync.go statements are identical in form; e.g. line ~224:
```go
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
```
Apply the same `sync_changes` → `changes` edit to all six (the `status_changed` one at line ~749 keeps its `old_status, new_status` columns). Repoint any test-file inserts the same way.

- [ ] **Step 3: Confirm no stray non-migration references remain**

Run: `grep -rn "sync_changes" --include=*.go --include=*.sql . | grep -v "internal/db/migrations/"`
Expected: no output.

- [ ] **Step 4: Build and run the worker + api suites**

Run: `go build ./... && go test ./internal/worker/tasks/... ./internal/api/... -timeout 600s`
Expected: PASS. (These packages run migrations at startup, so this also proves the rename migration applies and existing sync-change reads/writes work against `changes`.)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: repoint sync_changes SQL references to changes (#730)"
```

### Task 4: Import worker writes `changes` rows (added / updated / already_in_library)

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (platform loop ~313, tag loop ~348, completion block ~351-360)
- Test: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/worker/tasks/import_item_test.go`:
```go
// countChangeRows returns how many `changes` rows of a given change_type exist for a job.
func countChangeRows(t *testing.T, jobID, changeType string) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM changes WHERE job_id = ? AND change_type = ?`, jobID, changeType,
	).Scan(ctx, &n); err != nil {
		t.Fatalf("count changes: %v", err)
	}
	return n
}

func TestImportItem_WritesChangeRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	game := func(extraTag string) map[string]any {
		tags := []map[string]any{}
		if extraTag != "" {
			tags = append(tags, map[string]any{"name": extraTag})
		}
		return map[string]any{
			"igdb_id": int32(55501),
			"title":   "Change Row Game",
			"tags":    tags,
		}
	}
	runImport := func(jobID string, gd map[string]any) {
		insertTestJob(t, testDB, jobID, userID, 1)
		itemID := insertTestJobItem(t, testDB, jobID, userID, gd)
		w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
			t.Fatalf("work: %v", err)
		}
	}

	// 1) New game → 'added'.
	job1 := uuid.NewString()
	runImport(job1, game("RPG"))
	if got := countChangeRows(t, job1, "added"); got != 1 {
		t.Fatalf("added rows = %d, want 1", got)
	}

	// 2) Same game, nothing new merged → 'already_in_library'.
	job2 := uuid.NewString()
	runImport(job2, game("RPG"))
	if got := countChangeRows(t, job2, "already_in_library"); got != 1 {
		t.Fatalf("already_in_library rows = %d, want 1", got)
	}

	// 3) Same game, a new tag merged in → 'updated'.
	job3 := uuid.NewString()
	runImport(job3, game("Action"))
	if got := countChangeRows(t, job3, "updated"); got != 1 {
		t.Fatalf("updated rows = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/... -run TestImportItem_WritesChangeRows -v`
Expected: FAIL — no `changes` rows are written yet (counts are 0).

- [ ] **Step 3: Track newly-merged platforms/tags in `import_item.go`**

In the platform loop, the existing insert is:
```go
		if _, err := w.DB.NewInsert().Model(ugp).Exec(ctx); err != nil {
			slog.Error("import_item: insert user_game_platform", "err", err)
		}
```
Add a counter. Just before the platform loop (after `existingPlatforms` is built, near the `gameHoursApplied := false` line), declare:
```go
	newPlatformCount := 0
	newTagCount := 0
```
and change the platform insert to:
```go
		if _, err := w.DB.NewInsert().Model(ugp).Exec(ctx); err != nil {
			slog.Error("import_item: insert user_game_platform", "err", err)
		} else {
			newPlatformCount++
		}
```
In the tag loop, change:
```go
		if _, err := w.DB.NewInsert().Model(ugt).Exec(ctx); err != nil {
			slog.Error("import_item: insert user_game_tag", "err", err)
		}
```
to:
```go
		if _, err := w.DB.NewInsert().Model(ugt).Exec(ctx); err != nil {
			slog.Error("import_item: insert user_game_tag", "err", err)
		} else {
			newTagCount++
		}
```

- [ ] **Step 4: Write the change row before marking the item completed**

In the `── 9. Mark item completed ──` block, immediately **before** the `result := map[string]any{` line, insert:
```go
	// Record a per-item change row mirroring the sync worker's `changes` writes.
	changeType := "added"
	if alreadyExists {
		if newPlatformCount+newTagCount > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
	}
	if _, err := w.DB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, NULL, ?, ?, now())`,
		uuid.NewString(), item.JobID, item.UserID, changeType, item.SourceTitle,
	).Exec(ctx); err != nil {
		slog.Error("import_item: insert change", "err", err)
	}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/worker/tasks/... -run TestImportItem_WritesChangeRows -v`
Expected: PASS.

- [ ] **Step 6: Run the full worker suite to confirm no regressions**

Run: `go test ./internal/worker/tasks/... -timeout 600s`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go
git commit -m "feat(import): write per-item change rows (added/updated/already_in_library) (#730)"
```

---

## Phase B — Backend API

### Task 5: Generalize the recent-jobs endpoint

**Files:**
- Modify: `internal/api/jobs.go` — `HandleRecentJobs` (lines ~321-424) + the `jobWithChanges` struct.
- Modify: `internal/api/router.go:287`
- Modify: `internal/api/jobs_test.go` — existing recent tests + new ones.
- Modify: `slumber.yaml`

- [ ] **Step 1: Write the failing tests (new behaviour)**

In `internal/api/jobs_test.go`, append:
```go
func TestHandleRecentJobs_FiltersByJobType(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-recent-type")

	insertJob(t, testDB, "job-imp-1", userID, "import", "nexorious", "completed")
	insertJobItem(t, testDB, "ji-imp-1", "job-imp-1", userID, "key-imp", "Imp Game", "completed")
	insertJob(t, testDB, "job-exp-1", userID, "export", "nexorious", "completed")
	insertJobItem(t, testDB, "ji-exp-1", "job-exp-1", userID, "key-exp", "Exp Game", "completed")
	insertJob(t, testDB, "job-syn-1", userID, "sync", "steam", "completed")

	rec := getAuth(t, e, "/api/jobs/recent?jobType=import,export", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	jobs, _ := resp["jobs"].([]any)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs (import+export), got %d", len(jobs))
	}
}

func TestHandleRecentJobs_GroupsUpdatedItems(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-recent-updated")

	insertJob(t, testDB, "job-upd-1", userID, "import", "nexorious", "completed")
	if _, err := testDB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, NULL, 'updated', 'Updated Game', now())`,
		"chg-upd-1", "job-upd-1", userID,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert change: %v", err)
	}

	rec := getAuth(t, e, "/api/jobs/recent?source=nexorious", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Jobs []struct {
			UpdatedItems []map[string]any `json:"updated_items"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Jobs) != 1 || len(resp.Jobs[0].UpdatedItems) != 1 {
		t.Fatalf("expected 1 job with 1 updated item, got %+v", resp.Jobs)
	}
}
```
Also update the three existing recent tests to the new URL: in `internal/api/jobs_test.go`, change every `"/api/jobs/recent/steam"` to `"/api/jobs/recent?source=steam"` (occurrences around lines 688, 710, 1041, 1075).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run 'TestHandleRecentJobs|TestRecentJobs' -v`
Expected: FAIL — the new endpoint shape/route doesn't exist yet (404 on `/api/jobs/recent`, no `updated_items`).

- [ ] **Step 3: Change the route**

In `internal/api/router.go`, change line 287 from:
```go
		jobsGroup.GET("/recent/:source", jh.HandleRecentJobs)
```
to:
```go
		jobsGroup.GET("/recent", jh.HandleRecentJobs)
```
(Keep it above any parameterised `/:id`-style jobs routes — Echo v5 does not auto-sort.)

- [ ] **Step 4: Rewrite the handler**

Ensure `internal/api/jobs.go` imports `"github.com/uptrace/bun"` and `"strings"` (add if missing). Replace the body of `HandleRecentJobs` (from `userID := auth.UserIDFromContext(c)` through the final `return c.JSON(...)`) with:
```go
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	source := c.QueryParam("source")

	// jobType: accept repeated params and/or comma-separated values.
	var jobTypes []string
	for _, raw := range c.QueryParams()["jobType"] {
		for _, t := range strings.Split(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				jobTypes = append(jobTypes, t)
			}
		}
	}

	daysBack, _ := strconv.Atoi(c.QueryParam("daysBack")) //nolint:errcheck // invalid/empty query param clamped to default below
	if daysBack < 1 {
		daysBack = 7
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit")) //nolint:errcheck // invalid/empty query param clamped to default below
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	var jobs []models.Job
	q := h.db.NewSelect().Model(&jobs).
		Where("user_id = ?", userID).
		Where("status IN ('completed', 'failed')").
		Where("created_at >= now() - make_interval(days => ?)", daysBack).
		Order("created_at DESC").
		Limit(limit)
	if source != "" {
		q = q.Where("source = ?", source)
	}
	if len(jobTypes) > 0 {
		q = q.Where("job_type IN (?)", bun.In(jobTypes))
	}
	if err := q.Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get recent jobs")
	}
	if jobs == nil {
		jobs = []models.Job{}
	}

	type jobWithChanges struct {
		models.Job
		Progress              map[string]any   `json:"progress"`
		AddedItems            []syncChangeItem `json:"added_items"`
		UpdatedItems          []syncChangeItem `json:"updated_items"`
		RemovedItems          []syncChangeItem `json:"removed_items"`
		StatusChangedItems    []syncChangeItem `json:"status_changed_items"`
		SkippedItems          []syncChangeItem `json:"skipped_items"`
		AlreadyInLibraryItems []syncChangeItem `json:"already_in_library_items"`
	}

	result := make([]jobWithChanges, 0, len(jobs))
	for _, j := range jobs {
		progress, err := h.jobItemCounts(ctx, j.ID)
		if err != nil {
			slog.Error("HandleRecentJobs: failed to count job items", "job_id", j.ID, "err", err)
			progress = map[string]any{
				"pending": 0, "processing": 0, "completed": 0, "pending_review": 0,
				"skipped": 0, "failed": 0, "total": 0, "percent": 0,
			}
		}

		var allChanges []struct {
			ChangeType string  `bun:"change_type"`
			Title      string  `bun:"title"`
			OldStatus  *string `bun:"old_status"`
			NewStatus  *string `bun:"new_status"`
		}
		if err := h.db.NewRaw(`
			SELECT change_type, title, old_status, new_status
			FROM changes
			WHERE job_id = ?
			ORDER BY created_at`,
			j.ID,
		).Scan(ctx, &allChanges); err != nil {
			slog.Error("HandleRecentJobs: failed to query changes", "job_id", j.ID, "err", err)
			allChanges = nil
		}

		addedItems := []syncChangeItem{}
		updatedItems := []syncChangeItem{}
		removedItems := []syncChangeItem{}
		statusChangedItems := []syncChangeItem{}
		skippedItems := []syncChangeItem{}
		alreadyInLibraryItems := []syncChangeItem{}
		for _, sc := range allChanges {
			switch sc.ChangeType {
			case "added":
				addedItems = append(addedItems, syncChangeItem{Title: sc.Title})
			case "updated":
				updatedItems = append(updatedItems, syncChangeItem{Title: sc.Title})
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
		}

		result = append(result, jobWithChanges{
			Job:                   j,
			Progress:              progress,
			AddedItems:            addedItems,
			UpdatedItems:          updatedItems,
			RemovedItems:          removedItems,
			StatusChangedItems:    statusChangedItems,
			SkippedItems:          skippedItems,
			AlreadyInLibraryItems: alreadyInLibraryItems,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"jobs": result})
```
Also update the handler doc comment above the function from `// HandleRecentJobs handles GET /api/jobs/recent/:source.` to `// HandleRecentJobs handles GET /api/jobs/recent (filters: source, jobType, daysBack, limit).`

- [ ] **Step 5: Run the recent tests to verify they pass**

Run: `go test ./internal/api/... -run 'TestHandleRecentJobs|TestRecentJobs' -v`
Expected: PASS (existing + the two new tests).

- [ ] **Step 6: Update slumber.yaml**

Find the recent-jobs request (search `recent/` in `slumber.yaml`) and change its URL from the path-param form to the query form, e.g.:
```yaml
        url: "{{base_url}}/api/jobs/recent"
        query:
          - source: "steam"
```
(Keep the existing `authentication: type: bearer` block.) Then verify the collection still loads:
Run: `slumber collection`
Expected: loads without errors.

- [ ] **Step 7: Build + full api suite**

Run: `go build ./... && go test ./internal/api/... -timeout 600s`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/jobs.go internal/api/router.go internal/api/jobs_test.go slumber.yaml
git commit -m "feat(api): generalize GET /api/jobs/recent with source/jobType/daysBack filters (#730)"
```

---

## Phase C — Frontend

### Task 6: Extend the recent-jobs API client, types, and hook

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts:166-190`
- Modify: `ui/frontend/src/api/jobs.ts` (interfaces ~134-165, transform ~233-266, `getRecentJobs` ~418-430)
- Modify: `ui/frontend/src/hooks/use-jobs.ts` (`jobsKeys.recent` ~31-34, `useRecentJobs` ~148-154)

- [ ] **Step 1: Extend the `RecentJobDetail` type**

In `ui/frontend/src/types/jobs.ts`, replace the `RecentJobDetail` interface (lines ~172-186) with:
```ts
export interface RecentJobDetail {
  id: string;
  jobType: JobType;
  source: JobSource;
  status: string;
  createdAt: string;
  completedAt: string | null;
  errorMessage: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  progress: JobProgress;
  addedItems: SyncChangeItem[];
  updatedItems: SyncChangeItem[];
  removedItems: SyncChangeItem[];
  statusChangedItems: SyncChangeItem[];
  skippedItems: SyncChangeItem[];
  alreadyInLibraryItems: SyncChangeItem[];
}
```
(`JobType`, `JobSource`, and `JobProgress` are already declared earlier in this file.)

- [ ] **Step 2: Extend the API response interface and `RecentJobsFilters`**

In `ui/frontend/src/api/jobs.ts`, replace the `RecentJobDetailApiResponse` interface (lines ~140-161) with:
```ts
interface RecentJobDetailApiResponse {
  id: string;
  job_type: string;
  source: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  error_message: string | null;
  started_at: string | null;
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
  updated_items?: SyncChangeItemApiResponse[];
  removed_items: SyncChangeItemApiResponse[];
  status_changed_items: SyncChangeItemApiResponse[];
  skipped_items?: SyncChangeItemApiResponse[];
  already_in_library_items?: SyncChangeItemApiResponse[];
}
```
Add a filters type near the other exported request types (e.g. just above `getRecentJobs`):
```ts
export interface RecentJobsFilters {
  source?: string;
  jobTypes?: JobType[];
  daysBack?: number;
  limit?: number;
}
```
Make sure `JobType` and `JobProgress`/`JobSource` are imported in `ui/frontend/src/api/jobs.ts` (the file already imports types from `@/types`; add `JobType` to that import list if not present).

- [ ] **Step 3: Update the transform to populate the new fields**

In `ui/frontend/src/api/jobs.ts`, replace the `transformRecentJob` function (lines ~241-266) with:
```ts
function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  const p = api.progress ?? {
    completed: 0,
    skipped: 0,
    failed: 0,
    pending: 0,
    processing: 0,
    pending_review: 0,
    total: 0,
    percent: 0,
  };
  return {
    id: api.id,
    jobType: api.job_type as JobType,
    source: api.source as JobSource,
    status: api.status,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    errorMessage: api.error_message,
    totalItems: api.total_items,
    completedCount: p.completed,
    skippedCount: p.skipped,
    failedCount: p.failed,
    progress: {
      pending: p.pending,
      processing: p.processing,
      completed: p.completed,
      pendingReview: p.pending_review,
      skipped: p.skipped,
      failed: p.failed,
      total: p.total,
      percent: p.percent,
    },
    addedItems: (api.added_items ?? []).map(transformSyncChangeItem),
    updatedItems: (api.updated_items ?? []).map(transformSyncChangeItem),
    removedItems: (api.removed_items ?? []).map(transformSyncChangeItem),
    statusChangedItems: (api.status_changed_items ?? []).map(transformSyncChangeItem),
    skippedItems: (api.skipped_items ?? []).map(transformSyncChangeItem),
    alreadyInLibraryItems: (api.already_in_library_items ?? []).map(transformSyncChangeItem),
  };
}
```
Ensure `JobSource` is imported alongside `JobType` from `@/types` in this file.

- [ ] **Step 4: Update `getRecentJobs` to take filters**

In `ui/frontend/src/api/jobs.ts`, replace the `getRecentJobs` function (lines ~418-430) with:
```ts
export async function getRecentJobs(filters: RecentJobsFilters = {}): Promise<RecentJobsResponse> {
  const params: Record<string, string | number> = {
    limit: filters.limit ?? 5,
    days_back: filters.daysBack ?? 7,
  };
  if (filters.source) {
    params.source = filters.source;
  }
  if (filters.jobTypes && filters.jobTypes.length > 0) {
    params.job_type = filters.jobTypes.join(',');
  }
  const response = await api.get<RecentJobsApiResponse>('/jobs/recent', { params });
  return {
    jobs: response.jobs.map(transformRecentJob),
  };
}
```

- [ ] **Step 5: Update the query key and hook**

In `ui/frontend/src/hooks/use-jobs.ts`, replace the `recent` key (lines ~31-34) with:
```ts
  recent: (filters: RecentJobsFilters) => [...jobsKeys.all, 'recent', filters] as const,
```
and replace `useRecentJobs` (lines ~148-154) with:
```ts
/**
 * Hook to fetch recent completed/failed jobs (any type) with per-item change details.
 */
export function useRecentJobs(filters: RecentJobsFilters = {}, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.recent(filters),
    queryFn: () => jobsApi.getRecentJobs(filters),
    enabled: options?.enabled !== false,
  });
}
```
Add `RecentJobsFilters` to the type import from `../api/jobs` (or `@/api/jobs`) at the top of `use-jobs.ts` — match the existing import path used for `JobFilters`/`jobsApi`.

- [ ] **Step 6: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: no errors. (Component consumers update in Tasks 7-8; if `check` flags the not-yet-updated component, that is expected and resolved there — but the api/types/hook files themselves must typecheck.)

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/types/jobs.ts ui/frontend/src/api/jobs.ts ui/frontend/src/hooks/use-jobs.ts
git commit -m "feat(ui): recent-jobs API/types/hook take filters and updated_items (#730)"
```

### Task 7: Rewrite the unified `RecentActivity` component

**Files:**
- Rewrite: `ui/frontend/src/components/jobs/recent-activity.tsx`

- [ ] **Step 1: Replace the file contents**

Overwrite `ui/frontend/src/components/jobs/recent-activity.tsx` with:
```tsx
import { useState } from 'react';
import type { ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { useRecentJobs } from '@/hooks';
import { JobItemsDetails } from './job-items-details';
import type { RecentJobDetail, SyncChangeItem, JobType as JobTypeEnum } from '@/types';
import {
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  formatRelativeTime,
} from '@/types';
import {
  ChevronDown,
  ChevronRight,
  History,
  Inbox,
  CheckCircle,
  XCircle,
  ArrowRight,
  SkipForward,
  BookMarked,
  RefreshCw,
} from 'lucide-react';

interface RecentActivityProps {
  /** Sync: the storefront to filter by. */
  source?: string;
  /** Import/Export, Maintenance: job types to include. */
  jobTypes?: JobTypeEnum[];
  /** Job IDs to hide (e.g. the currently-displayed job). */
  excludeJobIds?: string[];
  /** Look-back window in days (default 7). */
  daysBack?: number;
  /** Max jobs (default 5). */
  limit?: number;
}

function hasChangeRows(job: RecentJobDetail): boolean {
  return (
    job.addedItems.length > 0 ||
    job.updatedItems.length > 0 ||
    job.removedItems.length > 0 ||
    job.statusChangedItems.length > 0 ||
    job.skippedItems.length > 0 ||
    job.alreadyInLibraryItems.length > 0
  );
}

function ChangeList({
  items,
  label,
  icon,
}: {
  items: SyncChangeItem[];
  label: string;
  icon: ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(false);
  if (items.length === 0) return null;
  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between h-8 px-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            {icon}
            <span className="text-sm">{label}</span>
          </div>
          <Badge variant="secondary" className="h-5 text-xs">
            {items.length}
          </Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-6 pl-2 border-l space-y-1 py-1">
          {items.map((item, idx) => (
            <div key={idx} className="text-sm py-1 text-muted-foreground">
              {item.title}
              {item.oldStatus && item.newStatus && (
                <span className="ml-2 text-xs">
                  {item.oldStatus} → {item.newStatus}
                </span>
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ChangeBreakdown({ job }: { job: RecentJobDetail }) {
  return (
    <div className="space-y-1">
      <ChangeList
        items={job.addedItems}
        label="Added to library"
        icon={<CheckCircle className="h-4 w-4 text-green-600" />}
      />
      <ChangeList
        items={job.updatedItems}
        label="Updated"
        icon={<RefreshCw className="h-4 w-4 text-blue-500" />}
      />
      <ChangeList
        items={job.removedItems}
        label="Removed from storefront"
        icon={<XCircle className="h-4 w-4 text-muted-foreground" />}
      />
      <ChangeList
        items={job.statusChangedItems}
        label="Status changed"
        icon={<ArrowRight className="h-4 w-4 text-blue-500" />}
      />
      <ChangeList
        items={job.alreadyInLibraryItems}
        label="Already in library"
        icon={<BookMarked className="h-4 w-4 text-muted-foreground" />}
      />
      <ChangeList
        items={job.skippedItems}
        label="Skipped"
        icon={<SkipForward className="h-4 w-4 text-muted-foreground" />}
      />
    </div>
  );
}

function JobActivityItem({
  job,
  isExpanded,
  onToggle,
}: {
  job: RecentJobDetail;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const showBreakdown = hasChangeRows(job);
  return (
    <Collapsible open={isExpanded} onOpenChange={onToggle}>
      <div className="rounded-lg border">
        <CollapsibleTrigger asChild>
          <button
            className="w-full p-3 flex items-center justify-between hover:bg-muted/50 transition-colors text-left"
            type="button"
          >
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <Badge variant={getJobStatusVariant(job.status)} className="shrink-0">
                {getJobStatusLabel(job.status)}
              </Badge>
              <span className="font-medium truncate">
                {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
              </span>
            </div>
            <div className="flex items-center gap-4 text-sm text-muted-foreground shrink-0 ml-4">
              <div className="hidden sm:flex items-center gap-2">
                <span className="text-green-600">{job.completedCount} completed</span>
                {job.failedCount > 0 && (
                  <span className="text-red-600">{job.failedCount} failed</span>
                )}
              </div>
              <span className="text-xs">
                {formatRelativeTime(job.completedAt || job.createdAt)}
              </span>
              {isExpanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </div>
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="border-t p-3 bg-muted/30">
            {job.errorMessage && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive mb-3">
                {job.errorMessage}
              </div>
            )}
            {showBreakdown ? (
              <ChangeBreakdown job={job} />
            ) : (
              <JobItemsDetails jobId={job.id} progress={job.progress} isTerminal />
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}

function EmptyState({ daysBack }: { daysBack: number }) {
  return (
    <div className="flex flex-col items-center justify-center py-8 text-center">
      <div className="rounded-full bg-muted p-3 mb-4">
        <Inbox className="h-6 w-6 text-muted-foreground" />
      </div>
      <h3 className="font-medium text-muted-foreground mb-1">No recent activity</h3>
      <p className="text-sm text-muted-foreground">
        Completed activity from the last {daysBack} days will appear here.
      </p>
    </div>
  );
}

/**
 * Recent Activity over completed/failed jobs. Renders a rich per-outcome
 * breakdown when the job has change rows (sync, import); otherwise falls back to
 * aggregate counts + per-item details (export, metadata-refresh).
 */
export function RecentActivity({
  source,
  jobTypes,
  excludeJobIds = [],
  daysBack = 7,
  limit = 5,
}: RecentActivityProps) {
  const [isOpen, setIsOpen] = useState(true);
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);
  const { data, isLoading } = useRecentJobs({ source, jobTypes, daysBack, limit });

  if (isLoading) return null;

  const jobs = (data?.jobs ?? []).filter((j) => !excludeJobIds.includes(j.id));

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <Card>
        <CollapsibleTrigger asChild>
          <CardHeader className="cursor-pointer hover:bg-muted/50 transition-colors">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <History className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                {jobs.length > 0 && (
                  <Badge variant="secondary" className="ml-2">
                    {jobs.length}
                  </Badge>
                )}
              </div>
              <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                {isOpen ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </Button>
            </div>
          </CardHeader>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="pt-0">
            {jobs.length === 0 ? (
              <EmptyState daysBack={daysBack} />
            ) : (
              <div className="space-y-2">
                {jobs.map((job) => (
                  <JobActivityItem
                    key={job.id}
                    job={job}
                    isExpanded={expandedJobId === job.id}
                    onToggle={() =>
                      setExpandedJobId((current) => (current === job.id ? null : job.id))
                    }
                  />
                ))}
              </div>
            )}
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: the component itself typechecks. The sync route still imports the old `@/components/sync` `RecentActivity` — that is rewired in Task 8; an error there is expected until then.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/jobs/recent-activity.tsx
git commit -m "feat(ui): unified RecentActivity with breakdown + counts fallback (#730)"
```

### Task 8: Rewire call sites and delete the duplicate component

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` (import ~20-29, usage ~550)
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx` (usage ~461)
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` (usage ~249 — verify only)
- Delete: `ui/frontend/src/components/sync/recent-activity.tsx`
- Delete: `ui/frontend/src/components/sync/recent-activity.test.tsx`
- Modify: `ui/frontend/src/components/sync/index.ts` (remove the export)

- [ ] **Step 1: Point the sync route at the unified component**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`, remove `RecentActivity` from the `@/components/sync` import block (lines ~22-29) and add a separate import:
```tsx
import { RecentActivity } from '@/components/jobs';
```
Change the usage (line ~550) from:
```tsx
      <RecentActivity platform={storefront} />
```
to:
```tsx
      <RecentActivity source={storefront} />
```

- [ ] **Step 2: Pass explicit job types on the import/export page**

In `ui/frontend/src/routes/_authenticated/import-export.tsx`, change the usage (line ~461) from:
```tsx
        <RecentActivity excludeJobIds={excludeJobIds} />
```
to:
```tsx
        <RecentActivity jobTypes={[JobType.IMPORT, JobType.EXPORT]} excludeJobIds={excludeJobIds} />
```
`JobType` is already imported in this file (it is used elsewhere in import-export.tsx); if `npm run check` says otherwise, add it to the `@/types` import.

- [ ] **Step 3: Verify the maintenance page still compiles unchanged**

`ui/frontend/src/routes/_authenticated/admin/maintenance.tsx:249` already passes `jobTypes={[JobType.METADATA_REFRESH]}` and `excludeJobIds`. These props are still valid on the unified component — no edit needed. Confirm in Step 6's typecheck.

- [ ] **Step 4: Delete the duplicate component, its test, and its export**

```bash
git rm ui/frontend/src/components/sync/recent-activity.tsx ui/frontend/src/components/sync/recent-activity.test.tsx
```
In `ui/frontend/src/components/sync/index.ts`, delete the line:
```ts
export { RecentActivity } from './recent-activity';
```

- [ ] **Step 5: Check for stragglers**

Run: `grep -rn "components/sync/recent-activity\|from '@/components/sync'" ui/frontend/src | grep -i recent`
Expected: no remaining import of `RecentActivity` from `@/components/sync`.

- [ ] **Step 6: Typecheck + dead-code check**

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: both clean. (knip would flag the old `useJobs`-based path or unused exports if anything was missed.)

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(ui): rewire Recent Activity call sites, delete sync duplicate (#730)"
```

### Task 9: Frontend tests for the unified component

**Files:**
- Modify/Create: `ui/frontend/src/components/jobs/recent-activity.test.tsx`

- [ ] **Step 1: Write the tests**

Replace the contents of `ui/frontend/src/components/jobs/recent-activity.test.tsx` with a suite that mocks the hook and asserts both render modes. Match the existing test conventions in the repo (look at the just-deleted `sync/recent-activity.test.tsx` in git history via `git show HEAD~1:ui/frontend/src/components/sync/recent-activity.test.tsx` for the mocking style, and at other `*.test.tsx` files for the render/query-client wrapper). The two behaviours to cover:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecentActivity } from './recent-activity';
import type { RecentJobDetail } from '@/types';
import { JobType } from '@/types';

// Mock the data hook so the component renders synchronously.
vi.mock('@/hooks', () => ({
  useRecentJobs: vi.fn(),
}));
// JobItemsDetails fetches on mount; stub it to a marker for the fallback case.
vi.mock('./job-items-details', () => ({
  JobItemsDetails: () => <div data-testid="job-items-details" />,
}));

import { useRecentJobs } from '@/hooks';

const baseJob = (over: Partial<RecentJobDetail>): RecentJobDetail => ({
  id: 'j1',
  jobType: JobType.SYNC,
  source: 'steam' as RecentJobDetail['source'],
  status: 'completed',
  createdAt: '2026-06-01T00:00:00Z',
  completedAt: '2026-06-01T00:01:00Z',
  errorMessage: null,
  totalItems: 1,
  completedCount: 1,
  skippedCount: 0,
  failedCount: 0,
  progress: {
    pending: 0,
    processing: 0,
    completed: 1,
    pendingReview: 0,
    skipped: 0,
    failed: 0,
    total: 1,
    percent: 100,
  },
  addedItems: [],
  updatedItems: [],
  removedItems: [],
  statusChangedItems: [],
  skippedItems: [],
  alreadyInLibraryItems: [],
  ...over,
});

describe('RecentActivity', () => {
  it('renders the rich breakdown when change rows exist', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: { jobs: [baseJob({ addedItems: [{ title: 'Portal', oldStatus: null, newStatus: null }] })] },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity source="steam" />);
    expect(screen.getByText('Added to library')).toBeInTheDocument();
    expect(screen.queryByTestId('job-items-details')).not.toBeInTheDocument();
  });

  it('falls back to per-item details when there are no change rows', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: { jobs: [baseJob({ jobType: JobType.EXPORT, source: 'nexorious' as RecentJobDetail['source'] })] },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity jobTypes={[JobType.EXPORT]} />);
    // The expandable row is collapsed by default; the breakdown labels must be absent.
    expect(screen.queryByText('Added to library')).not.toBeInTheDocument();
  });
});
```
If the collapsed-by-default state hides the fallback marker, either assert on the always-visible header ("Recent Activity", the count badge) or expand the row with `fireEvent.click` on the job button before asserting `job-items-details` — follow whichever pattern the existing `*.test.tsx` files use. The non-negotiable assertions: breakdown mode shows "Added to library"; fallback mode does not.

- [ ] **Step 2: Run the frontend tests**

Run (from `ui/frontend/`): `npm run test recent-activity`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/jobs/recent-activity.test.tsx
git commit -m "test(ui): cover unified RecentActivity breakdown + fallback (#730)"
```

### Task 10: Full verification

- [ ] **Step 1: Backend build + targeted suites**

Run: `go build ./... && go test ./internal/worker/tasks/... ./internal/api/... -timeout 600s`
Expected: PASS.

- [ ] **Step 2: Frontend gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all clean/PASS.

- [ ] **Step 3: Slumber loads**

Run: `slumber collection`
Expected: loads without errors.

- [ ] **Step 4: Confirm clean tree**

Run: `git status`
Expected: clean (everything committed across Tasks 1-9).

---

## Notes for the implementer

- **No new writers for export / metadata-refresh** — they intentionally render via the counts fallback. Do not add `changes` inserts to `export.go` or the metadata-refresh worker.
- **Import failures are not change rows** — a failed item is already marked `failed` on `job_items` (and surfaces in progress); the change-row insert only runs on the success path (it sits in the `── 9. Mark item completed ──` block).
- **`external_game_id` is NULL for import rows** — import works against `games`/IGDB ids, not `external_games`; the column is nullable. Sync rows keep populating it.
- **Echo v5 route order** — keep `GET /jobs/recent` registered before any parameterised jobs route.
- **Do not touch** the `events` table, its prune job, `internal/notify/`, or `/admin/activity`.
- **`routeTree.gen.ts`** — no routes are added/removed/renamed here, so it does not need regeneration; `npm run check` will tell you if that assumption is wrong.
