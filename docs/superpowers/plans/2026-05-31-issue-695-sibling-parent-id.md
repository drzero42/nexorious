# Issue #695 — Sibling parent_id refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the sibling relationship between PSN PS4/PS5 external_game rows explicitly at Stage 1 via a `parent_id` FK, eliminating runtime title-search sibling discovery and preventing siblings from appearing independently in the review UI.

**Architecture:** Add a nullable `parent_id` self-referencing FK to `external_games`. Stage 1 detects same-title rows at upsert time and sets `parent_id` on the newer row. Stage 2 inherits `resolved_igdb_id` from a resolved parent or waits (returning nil) until Stage 3 re-enqueues it. All UI list and count queries filter to `parent_id IS NULL`; skip and rematch handlers cascade changes to children via FK.

**Tech Stack:** Go 1.25, PostgreSQL via Bun ORM, River queue, Echo v5, stdlib testing + testcontainers-go.

---

## Files

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/db/migrations/20260531000002_external_games_parent_id.up.sql` | Add column + index + backfill |
| Create | `internal/db/migrations/20260531000002_external_games_parent_id.down.sql` | Drop column |
| Modify | `internal/db/models/models.go` | Add `ParentID` field to `ExternalGame` |
| Modify | `internal/worker/tasks/sync.go` | Stage 1 sibling detection, Stage 2 parent_id lookup, Stage 3 sibling trigger |
| Modify | `internal/worker/tasks/sync_test.go` | Rewrite sibling test; add child inherit, wait, and trigger tests |
| Modify | `internal/api/sync.go` | `HandleListExternalGames` filter + platforms; `HandleSkipGame`/`HandleUnskipGame` cascade; `HandleRematchExternalGame` FK lookup |
| Modify | `internal/api/sync_test.go` | Add child filter, skip cascade, rematch cascade tests |
| Modify | `internal/api/jobs.go` | `HandlePendingReviewCount` LEFT JOIN + parent_id filter |
| Modify | `internal/api/jobs_test.go` | Rewrite dedup test; add child-exclusion test |
| Modify | `docs/sync.md` | Update Siblings, User Interactions, and Data Model sections |

---

## Task 1: Migration — add parent_id column and backfill

**Files:**
- Create: `internal/db/migrations/20260531000002_external_games_parent_id.up.sql`
- Create: `internal/db/migrations/20260531000002_external_games_parent_id.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- internal/db/migrations/20260531000002_external_games_parent_id.up.sql
ALTER TABLE external_games
    ADD COLUMN parent_id TEXT REFERENCES external_games(id) ON DELETE SET NULL;

CREATE INDEX external_games_parent_id_idx
    ON external_games (parent_id)
    WHERE parent_id IS NOT NULL;

-- Backfill existing sibling pairs.
-- The oldest row per (user_id, storefront, title) group is the parent.
-- All later rows get parent_id set to the oldest row's id.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id, storefront, title
                              ORDER BY created_at ASC) AS rn,
           FIRST_VALUE(id) OVER (PARTITION BY user_id, storefront, title
                                 ORDER BY created_at ASC) AS parent_candidate_id
    FROM external_games
)
UPDATE external_games
SET parent_id  = ranked.parent_candidate_id,
    updated_at = now()
FROM ranked
WHERE external_games.id = ranked.id
  AND ranked.rn > 1;
```

- [ ] **Step 2: Create down migration**

```sql
-- internal/db/migrations/20260531000002_external_games_parent_id.down.sql
ALTER TABLE external_games DROP COLUMN parent_id;
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations/20260531000002_external_games_parent_id.up.sql \
        internal/db/migrations/20260531000002_external_games_parent_id.down.sql
git commit -m "feat: add parent_id column to external_games with sibling backfill"
```

---

## Task 2: Model — add ParentID field

**Files:**
- Modify: `internal/db/models/models.go:173-186`

- [ ] **Step 1: Add ParentID to ExternalGame**

In `internal/db/models/models.go`, add the field after `UpdatedAt`:

```go
type ExternalGame struct {
	bun.BaseModel `bun:"table:external_games"`

	ID              string    `bun:"id,pk"                   json:"id"`
	UserID          string    `bun:"user_id,notnull"          json:"user_id"`
	Storefront      string    `bun:"storefront,notnull"       json:"storefront"`
	ExternalID      string    `bun:"external_id,notnull"      json:"external_id"`
	Title           string    `bun:"title,notnull"            json:"title"`
	ResolvedIGDBID  *int32    `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	IsSkipped       bool      `bun:"is_skipped,notnull"       json:"is_skipped"`
	IsAvailable     bool      `bun:"is_available,notnull"     json:"is_available"`
	IsSubscription  bool      `bun:"is_subscription,notnull"  json:"is_subscription"`
	OwnershipStatus *string   `bun:"ownership_status"         json:"ownership_status"`
	ParentID        *string   `bun:"parent_id"                json:"parent_id,omitempty"`
	CreatedAt       time.Time `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"       json:"updated_at"`

	Platforms []ExternalGamePlatform `bun:"rel:has-many,join:id=external_game_id" json:"-"`
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/db/models/models.go
git commit -m "feat: add ParentID field to ExternalGame model"
```

---

## Task 3: Stage 1 — sibling detection in upsertExternalGame (TDD)

**Files:**
- Modify: `internal/worker/tasks/sync.go:77-99`
- Test: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add after `TestDispatchSync_SteamSuccess` in `internal/worker/tasks/sync_test.go`:

```go
func TestDispatchSync_SetsSiblingParentID(t *testing.T) {
	// When two library entries have the same (storefront, title) but different
	// external_ids, the second row must have parent_id set to the first row's id.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'psn', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{
		{
			{ExternalID: "CUSA12345_00", Title: "Horizon", Platforms: []string{"playstation-4"}},
			{ExternalID: "PPSA67890_00", Title: "Horizon", Platforms: []string{"playstation-5"}},
		},
	}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type egRow struct {
		ExternalID string  `bun:"external_id"`
		ParentID   *string `bun:"parent_id"`
	}
	var rows []egRow
	if err := testDB.NewRaw(
		`SELECT external_id, parent_id FROM external_games WHERE user_id = ? ORDER BY created_at ASC`,
		userID,
	).Scan(ctx, &rows); err != nil {
		t.Fatalf("scan external_games: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 external_game rows, got %d", len(rows))
	}

	// First row (PS4) must have no parent.
	if rows[0].ParentID != nil {
		t.Errorf("first row should have no parent_id, got %v", *rows[0].ParentID)
	}
	// Second row (PS5) must point to first row.
	if rows[1].ParentID == nil {
		t.Error("second row should have parent_id set")
	}

	// Three games with same title — first is parent, second and third are children.
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/worker/tasks/... -run TestDispatchSync_SetsSiblingParentID -v
```

Expected: FAIL — second row has no `parent_id` (column doesn't exist yet in tests or logic not implemented).

- [ ] **Step 3: Implement sibling detection in upsertExternalGame**

Replace the `upsertExternalGame` function in `internal/worker/tasks/sync.go` (lines 77–99):

```go
func upsertExternalGame(ctx context.Context, db *bun.DB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
		IsNew     bool   `bun:"is_new"`
	}
	if err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?, now(), now())
		ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			is_subscription = EXCLUDED.is_subscription,
			ownership_status = EXCLUDED.ownership_status,
			is_available = true,
			updated_at = now()
		RETURNING id, is_skipped, (xmax = 0) AS is_new`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus,
	).Scan(ctx, &row); err != nil {
		slog.Error("dispatch_sync: upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", e.ExternalID)
		return "", false
	}

	if row.IsNew {
		var parentID string
		if err := db.NewRaw(`
			SELECT id FROM external_games
			WHERE user_id = ? AND storefront = ? AND title = ?
			  AND id != ? AND parent_id IS NULL
			LIMIT 1`,
			p.UserID, p.Storefront, e.Title, row.ID,
		).Scan(ctx, &parentID); err == nil && parentID != "" {
			if _, err := db.NewRaw(`
				UPDATE external_games SET parent_id = ? WHERE id = ? AND parent_id IS NULL`,
				parentID, row.ID,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: set parent_id failed", "err", err, "external_game_id", row.ID)
			}
		}
	}

	return row.ID, row.IsSkipped
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/worker/tasks/... -run TestDispatchSync_SetsSiblingParentID -v
```

Expected: PASS.

- [ ] **Step 5: Run full worker task suite to check for regressions**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: detect siblings at Stage 1 upsert and set parent_id"
```

---

## Task 4: Stage 2 — replace title-based sibling check with parent_id lookup (TDD)

**Files:**
- Modify: `internal/worker/tasks/sync.go` (IGDBMatchWorker.Work, lines 402–424)
- Test: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write failing tests**

Replace `TestIGDBMatchWorker_SiblingResolution` and add two new tests in `internal/worker/tasks/sync_test.go`. Find `TestIGDBMatchWorker_SiblingResolution` (line ~1317) and replace the entire function, then add two new ones after it:

```go
func TestIGDBMatchWorker_ChildInheritsFromResolvedParent(t *testing.T) {
	// When a child row's parent already has resolved_igdb_id, Stage 2 must
	// inherit it without calling IGDB and enqueue Stage 3.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'psn', 'processing', 'normal', 2)`,
		jobID, userID,
	)
	const igdbID = int32(7777)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Horizon', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)

	// Parent row: already resolved.
	parentID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'CUSA001', 'Horizon', false, true, false, ?, now(), now())`,
		parentID, userID, igdbID,
	)
	// Child row: points to parent, not yet resolved.
	childID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'PPSA001', 'Horizon', false, true, false, ?, now(), now())`,
		childID, userID, parentID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-5', 0, now())`,
		uuid.NewString(), childID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'PPSA001', 'Horizon', ?, '{}', 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, childID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Child must have inherited resolved_igdb_id.
	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, childID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// Item must still be pending (UserGameWorker handles completion).
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status: want 'pending', got %q", status)
	}
}

func TestIGDBMatchWorker_ChildWaitsForUnresolvedParent(t *testing.T) {
	// When a child row's parent has no resolved_igdb_id yet, Stage 2 must
	// return nil without advancing the job_item — leaving it pending.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items)
		 VALUES (?, ?, 'sync', 'psn', 'processing', 'normal', 2)`,
		jobID, userID,
	)

	// Parent row: not yet resolved.
	parentID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'CUSA001', 'Horizon', false, true, false, now(), now())`,
		parentID, userID,
	)
	// Child row: points to parent.
	childID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'PPSA001', 'Horizon', false, true, false, ?, now(), now())`,
		childID, userID, parentID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-5', 0, now())`,
		uuid.NewString(), childID,
	).Exec(ctx)

	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'PPSA001', 'Horizon', ?, '{}', 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, childID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.IGDBMatchArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Child must NOT have resolved_igdb_id.
	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, childID).Scan(ctx, &resolvedID)
	if resolvedID != nil {
		t.Errorf("resolved_igdb_id: expected nil for waiting child, got %v", *resolvedID)
	}
	// Job item must remain pending.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status: want 'pending' (waiting), got %q", status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/worker/tasks/... -run "TestIGDBMatchWorker_Child" -v
```

Expected: FAIL — no `parent_id` check in Stage 2 yet.

- [ ] **Step 3: Replace title-based sibling check in IGDBMatchWorker.Work**

In `internal/worker/tasks/sync.go`, find the sibling check block (it starts with `// Sibling check: same user/storefront/title...` around line 402). Replace the entire block from there through the closing `}` of the `if err == nil && sibling.ResolvedIGDBID != nil {` block (approximately lines 402–424) with:

```go
	// Child check: if this row has a parent, inherit or wait.
	if eg.ParentID != nil {
		var parent models.ExternalGame
		if err := w.DB.NewSelect().Model(&parent).
			Where("id = ?", *eg.ParentID).
			Scan(ctx); err == nil && parent.ResolvedIGDBID != nil {
			igdbID := *parent.ResolvedIGDBID
			slog.Debug("igdb_match: child inheriting from resolved parent",
				"item_id", p.JobItemID, "title", eg.Title, "igdb_id", igdbID, "parent_id", *eg.ParentID)
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				igdbID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: insert game row (child inherit)", "err", err, "igdb_id", igdbID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				igdbID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: apply child inherit", "err", err, "external_game_id", eg.ID)
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}
		// Parent not yet resolved — leave job_item in pending.
		// Stage 3 of the parent will re-enqueue Stage 2 for this child.
		slog.Debug("igdb_match: parent unresolved, waiting",
			"item_id", p.JobItemID, "parent_id", *eg.ParentID)
		return nil
	}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/worker/tasks/... -run "TestIGDBMatchWorker_Child" -v
```

Expected: PASS.

- [ ] **Step 5: Run full worker task suite**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: Stage 2 inherits from parent_id instead of title search"
```

---

## Task 5: Stage 3 — sibling trigger after writing parent (TDD)

**Files:**
- Modify: `internal/worker/tasks/sync.go` (UserGameWorker.Work, after line ~782)
- Test: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add after `TestIGDBMatchWorker_ChildWaitsForUnresolvedParent` in `internal/worker/tasks/sync_test.go`:

```go
func TestUserGameWorker_SiblingTrigger(t *testing.T) {
	// After Stage 3 completes for a parent, it must re-enqueue Stage 2 for
	// any children that have pending job_items and are not yet resolved.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete)
		 VALUES (?, ?, 'sync', 'psn', 'processing', 'normal', 2, true)`,
		jobID, userID,
	)
	const igdbID = int32(9999)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Elden Ring', now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID,
	)
	// Seed platform and storefront for user_game_platforms FK constraints.
	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('playstation-4', 'PlayStation 4') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('playstation-store', 'PlayStation Store') ON CONFLICT DO NOTHING`)

	// Parent: already resolved.
	parentID := insertTestExternalGame(t, userID, "psn", "CUSA001", "Elden Ring", "playstation-4")
	_, _ = testDB.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ? WHERE id = ?`, igdbID, parentID,
	).Exec(ctx)

	// Child: parent_id set, not yet resolved, has a pending job_item.
	childID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'PPSA001', 'Elden Ring', false, true, false, ?, now(), now())`,
		childID, userID, parentID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-5', 0, now())`,
		uuid.NewString(), childID,
	).Exec(ctx)
	childItemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'PPSA001', 'Elden Ring', ?, '{}', 'pending', '{}', '[]', now())`,
		childItemID, jobID, userID, childID,
	)

	// Parent job_item.
	parentItemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, resolved_igdb_id, created_at)
		 VALUES (?, ?, ?, 'CUSA001', 'Elden Ring', ?, '{}', 'pending', '{}', '[]', ?, now())`,
		parentItemID, jobID, userID, parentID, igdbID,
	)

	rc := newTestRiverClient(t)
	w := &tasks.UserGameWorker{DB: testDB, IGDBClient: nil, RiverClient: rc}
	rj := &river.Job[tasks.UserGameArgs]{
		JobRow: &rivertype.JobRow{MaxAttempts: 5},
		Args:   tasks.UserGameArgs{JobItemID: parentItemID},
	}
	if err := w.Work(ctx, rj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify a River job was enqueued for the child's job_item (Stage 2).
	var enqueuedCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'igdb_match' AND args->>'job_item_id' = ?`,
		childItemID,
	).Scan(ctx, &enqueuedCount); err != nil {
		t.Fatalf("scan river_job count: %v", err)
	}
	if enqueuedCount < 1 {
		t.Errorf("expected at least one igdb_match river job for child item, got %d", enqueuedCount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_SiblingTrigger -v
```

Expected: FAIL — no sibling trigger in Stage 3 yet.

- [ ] **Step 3: Add sibling trigger to UserGameWorker.Work**

In `internal/worker/tasks/sync.go`, find the lines `syncMarkItemCompleted(ctx, w.DB, &item)` and `SyncCheckJobCompletion(ctx, w.DB, item.JobID)` near the end of `UserGameWorker.Work` (around line 782). Insert the sibling trigger between them:

```go
	syncMarkItemCompleted(ctx, w.DB, &item)

	// Sibling trigger: re-enqueue Stage 2 for children waiting on this parent.
	if w.RiverClient != nil {
		var childItems []struct {
			JobItemID      string `bun:"job_item_id"`
			ExternalGameID string `bun:"external_game_id"`
		}
		if err := w.DB.NewRaw(`
			SELECT ji.id AS job_item_id, eg.id AS external_game_id
			FROM external_games eg
			JOIN job_items ji ON ji.external_game_id = eg.id
			WHERE eg.parent_id = ?
			  AND eg.resolved_igdb_id IS NULL
			  AND NOT eg.is_skipped
			  AND ji.status = 'pending'
			ORDER BY ji.created_at DESC`,
			eg.ID,
		).Scan(ctx, &childItems); err == nil {
			for _, child := range childItems {
				if _, err := w.RiverClient.Insert(ctx, IGDBMatchArgs{JobItemID: child.JobItemID}, nil); err != nil {
					slog.Error("user_game_write: enqueue sibling Stage 2",
						"err", err, "child_eg_id", child.ExternalGameID, "job_item_id", child.JobItemID)
				}
			}
		}
	}

	SyncCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_SiblingTrigger -v
```

Expected: PASS.

- [ ] **Step 5: Run full worker task suite**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: Stage 3 re-enqueues Stage 2 for unresolved children (sibling trigger)"
```

---

## Task 6: HandleListExternalGames — filter + platform aggregation (TDD)

**Files:**
- Modify: `internal/api/sync.go` (HandleListExternalGames, lines ~978–1040)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write failing tests**

Add a helper and two tests in `internal/api/sync_test.go`, after `TestListExternalGames_IsolatedByUser`:

```go
// insertChildExternalGame inserts an external_game row with parent_id set.
func insertChildExternalGame(t *testing.T, db *bun.DB, id, userID, storefront, extID, title, parentID string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, ?, now(), now())`,
		id, userID, storefront, extID, title, parentID,
	)
	if err != nil {
		t.Fatalf("insertChildExternalGame: %v", err)
	}
}

// insertExternalGamePlatform inserts a platform row for an external_game.
func insertExternalGamePlatform(t *testing.T, db *bun.DB, egID, platform string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (gen_random_uuid()::text, ?, ?, 0, now())`,
		egID, platform,
	)
	if err != nil {
		t.Fatalf("insertExternalGamePlatform: %v", err)
	}
}

func TestListExternalGames_ExcludesChildren(t *testing.T) {
	// A child row (parent_id IS NOT NULL) must never appear in the list.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "list-excludes-children")

	insertExternalGame(t, testDB, "eg-parent-1", userID, "psn", "CUSA001", "Horizon")
	insertChildExternalGame(t, testDB, "eg-child-1", userID, "psn", "PPSA001", "Horizon", "eg-parent-1")

	rec := getAuth(t, e, "/api/sync/psn/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result (parent only), got %d", len(resp))
	}
	if resp[0]["id"] != "eg-parent-1" {
		t.Errorf("expected parent row, got id=%v", resp[0]["id"])
	}
}

func TestListExternalGames_AggregatesChildPlatforms(t *testing.T) {
	// The parent entry must include platform slugs from child rows.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "list-agg-platforms")

	insertExternalGame(t, testDB, "eg-par-2", userID, "psn", "CUSA002", "God of War")
	insertExternalGamePlatform(t, testDB, "eg-par-2", "playstation-4")
	insertChildExternalGame(t, testDB, "eg-chi-2", userID, "psn", "PPSA002", "God of War", "eg-par-2")
	insertExternalGamePlatform(t, testDB, "eg-chi-2", "playstation-5")

	rec := getAuth(t, e, "/api/sync/psn/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp))
	}
	platforms, ok := resp[0]["platforms"].([]any)
	if !ok {
		t.Fatalf("expected platforms array, got %T", resp[0]["platforms"])
	}
	platformSet := make(map[string]bool)
	for _, p := range platforms {
		platformSet[p.(string)] = true
	}
	if !platformSet["playstation-4"] || !platformSet["playstation-5"] {
		t.Errorf("expected both playstation-4 and playstation-5 in platforms, got %v", platforms)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/... -run "TestListExternalGames_Excludes|TestListExternalGames_Aggregates" -v
```

Expected: `ExcludesChildren` returns 2 items (child not filtered); `AggregatesChildPlatforms` shows only the parent's own platform.

- [ ] **Step 3: Update the WHERE clause in HandleListExternalGames**

In `internal/api/sync.go`, find the query in `HandleListExternalGames`. Update it in two places:

**WHERE clause** — add `AND eg.parent_id IS NULL` after `eg.storefront = ?`:

```sql
		WHERE eg.user_id = ? AND eg.storefront = ?
		  AND eg.parent_id IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM job_items ji
		      WHERE ji.external_game_id = eg.id
		        AND ji.status IN ('pending', 'processing')
		  )
```

**platforms_csv subquery** — replace:

```sql
			COALESCE(
				(SELECT string_agg(egp.platform, ',' ORDER BY egp.platform)
				 FROM external_game_platforms egp
				 WHERE egp.external_game_id = eg.id),
				''
			) AS platforms_csv
```

with:

```sql
			COALESCE(
				(SELECT string_agg(DISTINCT egp.platform, ',' ORDER BY egp.platform)
				 FROM external_game_platforms egp
				 WHERE egp.external_game_id = eg.id
				    OR egp.external_game_id IN (
				        SELECT id FROM external_games WHERE parent_id = eg.id
				    )
				),
				''
			) AS platforms_csv
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/api/... -run "TestListExternalGames_Excludes|TestListExternalGames_Aggregates" -v
```

Expected: PASS.

- [ ] **Step 5: Run full API suite**

```bash
go test ./internal/api/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: HandleListExternalGames filters children and aggregates child platforms"
```

---

## Task 7: HandlePendingReviewCount — LEFT JOIN parent filter (TDD)

**Files:**
- Modify: `internal/api/jobs.go` (HandlePendingReviewCount, lines ~236–243)
- Test: `internal/api/jobs_test.go`

- [ ] **Step 1: Rewrite TestPendingReviewCount_Deduplicates and add TestPendingReviewCount_ExcludesChildren**

In `internal/api/jobs_test.go`, replace `TestPendingReviewCount_Deduplicates` and add a new test after it:

```go
func TestPendingReviewCount_Deduplicates(t *testing.T) {
	// A child row with a pending_review job_item must not be counted — only the
	// parent counts. Dedup now comes from parent_id IS NULL, not DISTINCT title.
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "prc-dedup")

	insertJob(t, testDB, "job-prc-dedup", userID, "sync", "psn", "processing")

	// Insert parent external_game and link its job_item.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES ('eg-prc-parent', ?, 'psn', 'CUSA12345_00', 'Call of Duty', false, true, false, now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert parent eg: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-prc-dedup-1', 'job-prc-dedup', ?, 'CUSA12345_00', 'Call of Duty', 'eg-prc-parent', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert parent job_item: %v", err)
	}

	// Insert child external_game and link its job_item.
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES ('eg-prc-child', ?, 'psn', 'PPSA07890_00', 'Call of Duty', false, true, false, 'eg-prc-parent', now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child eg: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-prc-dedup-2', 'job-prc-dedup', ?, 'PPSA07890_00', 'Call of Duty', 'eg-prc-child', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child job_item: %v", err)
	}

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 1 {
		t.Fatalf("expected pending_review_count=1 (child excluded), got %v", resp["pending_review_count"])
	}
	bySource := resp["counts_by_source"].(map[string]any)
	if bySource["psn"].(float64) != 1 {
		t.Fatalf("expected counts_by_source.psn=1, got %v", bySource["psn"])
	}
}

func TestPendingReviewCount_ExcludesChildren(t *testing.T) {
	// A pending_review item linked to a child external_game must be excluded
	// from the count even when the parent has no pending_review item.
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "prc-exclude-child")

	insertJob(t, testDB, "job-prc-child", userID, "sync", "psn", "processing")

	// Parent: no pending_review item.
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES ('eg-prc-ex-parent', ?, 'psn', 'CUSA999', 'Ratchet', false, true, false, now(), now())`,
		userID,
	)
	// Child: has a pending_review item — must NOT be counted.
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES ('eg-prc-ex-child', ?, 'psn', 'PPSA999', 'Ratchet', false, true, false, 'eg-prc-ex-parent', now(), now())`,
		userID,
	)
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-prc-ex-child', 'job-prc-child', ?, 'PPSA999', 'Ratchet', 'eg-prc-ex-child', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)

	rec := getAuth(t, e, "/api/jobs/pending-review-count", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["pending_review_count"].(float64) != 0 {
		t.Fatalf("expected pending_review_count=0 (child excluded), got %v", resp["pending_review_count"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/... -run "TestPendingReviewCount_Deduplicates|TestPendingReviewCount_ExcludesChildren" -v
```

Expected: FAIL — child items are still counted.

- [ ] **Step 3: Update HandlePendingReviewCount query**

In `internal/api/jobs.go`, replace the query in `HandlePendingReviewCount` (the `db.NewRaw` call around line 236):

```go
	err := h.db.NewRaw(`
		SELECT j.source, COUNT(*) AS count
		FROM job_items ji
		JOIN jobs j ON ji.job_id = j.id
		LEFT JOIN external_games eg ON eg.id = ji.external_game_id
		WHERE ji.user_id = ? AND ji.status = ?
		  AND (eg.id IS NULL OR eg.parent_id IS NULL)
		GROUP BY j.source`,
		userID, models.JobItemStatusPendingReview,
	).Scan(context.Background(), &rows)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/api/... -run "TestPendingReviewCount" -v
```

Expected: all `TestPendingReviewCount_*` tests pass.

- [ ] **Step 5: Run full API suite**

```bash
go test ./internal/api/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/jobs.go internal/api/jobs_test.go
git commit -m "feat: HandlePendingReviewCount excludes child rows via parent_id LEFT JOIN"
```

---

## Task 8: HandleSkipGame / HandleUnskipGame — cascade to children (TDD)

**Files:**
- Modify: `internal/api/sync.go` (HandleSkipGame ~line 874; HandleUnskipGame ~line 913)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write failing tests**

Add after `TestSkipGame_MarksJobItemSkippedAndCompletesJob` in `internal/api/sync_test.go`:

```go
func TestSkipGame_CascadesToChildren(t *testing.T) {
	// Skipping a parent must also skip its children and their job_items.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-cascade")

	insertExternalGame(t, testDB, "eg-skip-parent", userID, "psn", "CUSA001", "Horizon")
	insertChildExternalGame(t, testDB, "eg-skip-child", userID, "psn", "PPSA001", "Horizon", "eg-skip-parent")
	insertJob(t, testDB, "job-skip-cascade", userID, "sync", "psn", "processing")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-skip-child', 'job-skip-cascade', ?, 'PPSA001', 'Horizon', 'eg-skip-child', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child job_item: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-skip-parent", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Child external_game must be skipped.
	var childSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-skip-child'`).Scan(ctx, &childSkipped); err != nil {
		t.Fatalf("scan child is_skipped: %v", err)
	}
	if !childSkipped {
		t.Error("expected child external_game to be skipped")
	}

	// Child job_item must be skipped.
	var childItemStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-skip-child'`).Scan(ctx, &childItemStatus); err != nil {
		t.Fatalf("scan child job_item status: %v", err)
	}
	if childItemStatus != "skipped" {
		t.Errorf("expected child job_item status=skipped, got %q", childItemStatus)
	}
}

func TestUnskipGame_CascadesToChildren(t *testing.T) {
	// Unskipping a parent must also unskip its children.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "unskip-cascade")

	// Insert parent and child, both pre-skipped.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES ('eg-unskip-parent', ?, 'psn', 'CUSA002', 'Ratchet', true, true, false, now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, parent_id, created_at, updated_at)
		 VALUES ('eg-unskip-child', ?, 'psn', 'PPSA002', 'Ratchet', true, true, false, 'eg-unskip-parent', now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert child: %v", err)
	}

	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip-parent", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var childSkipped bool
	if err := testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-unskip-child'`).Scan(context.Background(), &childSkipped); err != nil {
		t.Fatalf("scan child is_skipped: %v", err)
	}
	if childSkipped {
		t.Error("expected child external_game to be unskipped after parent unskip")
	}
}
```

The test uses `deleteAuth` — check whether that helper already exists in the test package:

```bash
grep -n "func deleteAuth\|func delete" internal/api/sync_test.go internal/api/main_test.go 2>/dev/null | head -10
```

If it doesn't exist, add to `internal/api/main_test.go`:

```go
func deleteAuth(t *testing.T, handler interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/api/... -run "TestSkipGame_CascadesToChildren|TestUnskipGame_CascadesToChildren" -v
```

Expected: FAIL — no cascade in handlers yet.

- [ ] **Step 3: Add skip cascade to HandleSkipGame**

In `internal/api/sync.go`, find `HandleSkipGame`. After the block that updates the parent's `external_games` row (`UPDATE external_games SET is_skipped = true ...`), add the cascade before the job_item block:

```go
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: skip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}

	// Cascade skip to children.
	var childIDs []string
	if err := h.db.NewRaw(
		`SELECT id FROM external_games WHERE parent_id = ?`, id,
	).Scan(ctx, &childIDs); err == nil {
		for _, childID := range childIDs {
			if _, err := h.db.NewRaw(
				`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, childID,
			).Exec(ctx); err != nil {
				slog.Error("sync: skip game: cascade child failed", "err", err, "child_id", childID)
				continue
			}
			var childItem struct {
				ID    string `bun:"id"`
				JobID string `bun:"job_id"`
			}
			if err := h.db.NewRaw(`
				SELECT id, job_id FROM job_items
				WHERE external_game_id = ? AND status IN ('pending_review', 'pending')
				ORDER BY created_at DESC
				LIMIT 1`, childID,
			).Scan(ctx, &childItem); err == nil {
				if _, err := h.db.NewRaw(
					`UPDATE job_items SET status = 'skipped', processed_at = now() WHERE id = ?`,
					childItem.ID,
				).Exec(ctx); err != nil {
					slog.Error("sync: skip game: cascade child job_item", "err", err, "job_item_id", childItem.ID)
				} else {
					tasks.SyncCheckJobCompletion(ctx, h.db, childItem.JobID)
				}
			}
		}
	}
```

- [ ] **Step 4: Add unskip cascade to HandleUnskipGame**

In `internal/api/sync.go`, find `HandleUnskipGame`. After the block that updates the parent's `external_games` row (`UPDATE external_games SET is_skipped = false ...`), add:

```go
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: unskip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unskip game")
	}

	// Cascade unskip to children (job_items unchanged; children re-process on next sync).
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE parent_id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: unskip game: cascade children failed", "err", err, "parent_id", id)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/api/... -run "TestSkipGame_CascadesToChildren|TestUnskipGame_CascadesToChildren" -v
```

Expected: PASS.

- [ ] **Step 6: Run full API suite**

```bash
go test ./internal/api/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: skip/unskip cascades to children via parent_id"
```

---

## Task 9: HandleRematchExternalGame — replace title search with parent_id (TDD)

**Files:**
- Modify: `internal/api/sync.go` (HandleRematchExternalGame, lines ~1261–1272)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write failing test**

Find `TestRematchExternalGame_SiblingAfterSyncCompleted` in `internal/api/sync_test.go`. Add a new test after it:

```go
func TestRematchExternalGame_CascadesToChildrenViaParentID(t *testing.T) {
	// Rematching a parent must cascade resolved_igdb_id to children via parent_id,
	// not title search. A second game with the same title but different user must NOT
	// be affected.
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userA, tokenA := setupTagUser(t, testDB, e, "rm-fk-userA")
	userB, _ := setupTagUser(t, testDB, e, "rm-fk-userB")

	// User A: parent + child, same title.
	insertExternalGame(t, testDB, "eg-fk-parent", userA, "psn", "CUSA100", "Spider-Man")
	insertChildExternalGame(t, testDB, "eg-fk-child", userA, "psn", "PPSA100", "Spider-Man", "eg-fk-parent")
	insertJob(t, testDB, "job-fk", userA, "sync", "psn", "processing")
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-fk-child', 'job-fk', ?, 'PPSA100', 'Spider-Man', 'eg-fk-child', '{}', 'pending', '{}', '[]', now())`,
		userA,
	)
	if err != nil {
		t.Fatalf("insert child job_item: %v", err)
	}

	// User B: unrelated game with same title — must NOT be touched.
	insertExternalGame(t, testDB, "eg-fk-other", userB, "psn", "CUSA200", "Spider-Man")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-fk-parent/rematch",
		map[string]any{"igdb_id": 4242}, tokenA)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Child must have inherited resolved_igdb_id.
	var childResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-child'`).Scan(ctx, &childResolved); err != nil {
		t.Fatalf("scan child resolved_igdb_id: %v", err)
	}
	if childResolved == nil || *childResolved != 4242 {
		t.Errorf("child resolved_igdb_id: want 4242, got %v", childResolved)
	}

	// User B's game must NOT have been touched.
	var otherResolved *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-fk-other'`).Scan(ctx, &otherResolved); err != nil {
		t.Fatalf("scan other resolved_igdb_id: %v", err)
	}
	if otherResolved != nil {
		t.Errorf("other user's game must not be resolved, got %v", *otherResolved)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -run TestRematchExternalGame_CascadesToChildrenViaParentID -v
```

Expected: test may pass accidentally if user B's game would have been caught by the old title+user+storefront query. But it fails because `eg-fk-child` has `parent_id` set and the old query won't find it without `AND id != eg.id` matching the child — or rather, the old query is `WHERE user_id = ? AND storefront = ? AND title = ? AND id != ?` which WILL find `eg-fk-child`. So the test may actually pass. The key test is the negative case: `eg-fk-other` (User B) is a different user so old query won't touch it. However, the purpose of this task is to replace the title search with a cleaner FK-based lookup. Run the suite to verify the existing rematch sibling tests pass after the change.

- [ ] **Step 3: Replace title-based sibling query in HandleRematchExternalGame**

In `internal/api/sync.go`, find the `// Resolve siblings` block near the end of `HandleRematchExternalGame` (around line 1261). Replace just the query:

```go
	// Resolve children: update resolved_igdb_id for all children of this parent
	// via FK rather than title search.
	var siblings []struct {
		ID         string `bun:"id"`
		ExternalID string `bun:"external_id"`
		Title      string `bun:"title"`
	}
	if err := h.db.NewRaw(`
		SELECT id, external_id, title FROM external_games
		WHERE parent_id = ? AND is_skipped = false`, id,
	).Scan(ctx, &siblings); err == nil {
```

Leave the rest of the loop body unchanged.

Also update the comment above the block from "Resolve siblings: other external_games for the same (user, storefront, title)..." to "Resolve children: external_games with parent_id pointing to this row."

- [ ] **Step 4: Run the new test and existing rematch tests**

```bash
go test ./internal/api/... -run "TestRematchExternalGame" -v 2>&1 | tail -40
```

Expected: all `TestRematchExternalGame_*` tests pass including the new one.

- [ ] **Step 5: Run full API suite**

```bash
go test ./internal/api/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: HandleRematchExternalGame cascades to children via parent_id FK"
```

---

## Task 10: Update docs/sync.md

**Files:**
- Modify: `docs/sync.md`

- [ ] **Step 1: Update the Data Model table**

Find the `external_games` row in the Data Model table and update it to mention `parent_id`:

```markdown
| `external_games` | One row per user + storefront + game; persists across sync runs. `parent_id` (nullable FK to self) marks duplicate-SKU siblings established at Stage 1 |
```

- [ ] **Step 2: Update the Siblings section**

Find the `### Siblings` section (around line 200) and replace its content:

```markdown
### Siblings

A sibling is another `external_games` row for the same user, storefront, and title. This occurs on storefronts that assign separate identifiers to different platform releases of the same game — for example, PSN assigns distinct title IDs to the PS4 and PS5 versions of a game.

The sibling relationship is established explicitly during **Stage 1**: when a new `external_games` row is inserted with the same `(user_id, storefront, title)` as an existing row that has no `parent_id` itself, the new row's `parent_id` is set to the existing row's `id`. This produces a flat tree — all siblings point to one parent; no chaining.

The sibling relationship is acted on in three places:

- **Stage 2 (child):** if the external game has `parent_id IS NOT NULL`, check whether the parent already has `resolved_igdb_id`. If yes, inherit it and proceed to Stage 3. If no (parent still in flight or in `pending_review`), return without advancing the job item — the child waits in `pending` state.
- **Stage 3 (sibling trigger):** after writing the parent's library entries, query for child rows with `parent_id = eg.id` that are not yet resolved and have a `pending` job item. Re-enqueue Stage 2 for each. This handles the case where the child's Stage 2 ran before the parent was resolved, and also handles siblings that arrive in the library after the parent was already matched.
- **Manual match / skip (cascade):** `HandleRematchExternalGame` and `HandleSkipGame` look up children via `parent_id = eg.id` and propagate the resolution or skip flag to each child, then enqueue Stage 3 (rematch) or mark job items skipped (skip).

Child rows (`parent_id IS NOT NULL`) are filtered from all UI lists and counts — only the parent row is visible and actionable by the user.
```

- [ ] **Step 3: Update the User Interactions section**

Find the paragraph in the User Interactions section describing manual resolution cascade. Update:

```markdown
Once a match is chosen, the resolve endpoint (`HandleRematchExternalGame`) sets `resolved_igdb_id` on the parent `external_game` and enqueues Stage 3 immediately. Any children (`parent_id = eg.id`) are resolved with the same IGDB ID and also enqueued for Stage 3 at the time of the user's action. Siblings never appear in the Needs Review list — only the parent does.
```

- [ ] **Step 4: Commit**

```bash
git add docs/sync.md
git commit -m "docs: update sync.md siblings section for parent_id model"
```

---

## Final verification

- [ ] **Run full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Build**

```bash
make build
```

Expected: binary compiles without errors.
