# play_status NULL handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `play_status` being NULL on `user_games` rows, which causes the play-status filter to return 0 results for any specific status.

**Architecture:** Three independent changes: (1) a DB migration that backfills NULLs and adds `NOT NULL DEFAULT 'not_started'`; (2) sync worker logic that infers `in_progress` from incoming hours; (3) a docs update in `docs/sync.md`.

**Tech Stack:** Go, PostgreSQL, Bun migrate, `uptrace/bun` raw queries

---

## File Map

| File | Action |
|---|---|
| `internal/db/migrations/20260531000001_play_status_not_null.up.sql` | Create |
| `internal/db/migrations/20260531000001_play_status_not_null.down.sql` | Create |
| `internal/worker/tasks/sync.go` | Modify (lines 620–635 struct + SQL; after line 735 add hours check) |
| `internal/worker/tasks/sync_test.go` | Add tests (after line 2218) |
| `docs/sync.md` | Modify (add Play Status subsection after line 227) |

---

## Task 1: DB migration — backfill NULLs and add NOT NULL DEFAULT

**Files:**
- Create: `internal/db/migrations/20260531000001_play_status_not_null.up.sql`
- Create: `internal/db/migrations/20260531000001_play_status_not_null.down.sql`

- [ ] **Step 1: Create the up migration**

Create `internal/db/migrations/20260531000001_play_status_not_null.up.sql`:

```sql
UPDATE user_games SET play_status = 'not_started' WHERE play_status IS NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET NOT NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET DEFAULT 'not_started';
```

- [ ] **Step 2: Create the down migration**

Create `internal/db/migrations/20260531000001_play_status_not_null.down.sql`:

```sql
ALTER TABLE user_games ALTER COLUMN play_status DROP DEFAULT;
ALTER TABLE user_games ALTER COLUMN play_status DROP NOT NULL;
```

- [ ] **Step 3: Verify the migration is discovered**

The `internal/db/migrations/migrations.go` file uses `//go:embed *.sql` and `Migrations.Discover(FS)` — no registration needed. Confirm the file is picked up by running the test suite (which calls `migrator.Migrate` in `TestMain`):

```bash
go test ./internal/worker/tasks/... -run TestUserGameWorker_CreatesUserGameAndSyncChange -v
```

Expected: `PASS` — the migration runs against the test container and the existing test continues to pass.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260531000001_play_status_not_null.up.sql \
        internal/db/migrations/20260531000001_play_status_not_null.down.sql
git commit -m "feat: add migration to set play_status NOT NULL DEFAULT not_started"
```

---

## Task 2: Sync worker — infer play_status from incoming hours

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Test: `internal/worker/tasks/sync_test.go`

### Step 2a — Write the failing tests

- [ ] **Step 1: Add four tests to `sync_test.go`**

Add the following tests at the end of the `// UserGameWorker — Stage 3 tests` section (after `TestUserGameWorker_AlreadyInLibrary_WritesSyncChange`). Each test calls `UserGameWorker.Work` directly against the test container.

```go
func TestUserGameWorker_PlayStatus_NewGame_WithHours_SetsInProgress(t *testing.T) {
	// New user_games row + incoming hours > 0 → play_status = 'in_progress'.
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
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1001', 'Test Game', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 10.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1001', 'Test Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "in_progress" {
		t.Errorf("play_status: want 'in_progress', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_NewGame_NoHours_SetsNotStarted(t *testing.T) {
	// New user_games row + incoming hours = 0 → play_status = 'not_started' (DB default).
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
	const igdbID = int32(1002)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1002', 'Test Game 2', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1002', 'Test Game 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "not_started" {
		t.Errorf("play_status: want 'not_started', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_ExistingNotStarted_WithHours_PromotesToInProgress(t *testing.T) {
	// Existing row with play_status='not_started' + hours > 0 → promoted to 'in_progress'.
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
	const igdbID = int32(1003)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 3', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, play_status, created_at, updated_at) VALUES (?, ?, ?, 'not_started', now(), now())`,
		ugID, userID, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1003', 'Test Game 3', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 5.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1003', 'Test Game 3', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "in_progress" {
		t.Errorf("play_status: want 'in_progress', got %q", playStatus)
	}
}

func TestUserGameWorker_PlayStatus_ExistingUserSet_NeverOverwritten(t *testing.T) {
	// Existing row with play_status='completed' (user-set) must never be overwritten by sync,
	// even when incoming hours > 0.
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
	const igdbID = int32(1004)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game 4', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, play_status, created_at, updated_at) VALUES (?, ?, ?, 'completed', now(), now())`,
		ugID, userID, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', 'ext-1004', 'Test Game 4', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 50.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'ext-1004', 'Test Game 4', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var playStatus string
	if err := testDB.NewRaw(
		`SELECT play_status FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID,
	).Scan(ctx, &playStatus); err != nil {
		t.Fatalf("scan play_status: %v", err)
	}
	if playStatus != "completed" {
		t.Errorf("play_status: want 'completed' (unchanged), got %q", playStatus)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
go test ./internal/worker/tasks/... -run "TestUserGameWorker_PlayStatus" -v
```

Expected: `FAIL` — the four new tests fail because the sync worker doesn't yet set `play_status`.

### Step 2b — Implement the sync worker changes

- [ ] **Step 3: Extend the `isNewRow` struct in `sync.go` (around line 620)**

Change:
```go
var isNewRow struct {
	ID    string `bun:"id"`
	IsNew bool   `bun:"is_new"`
}
```

To:
```go
var isNewRow struct {
	ID         string  `bun:"id"`
	IsNew      bool    `bun:"is_new"`
	PlayStatus *string `bun:"play_status"`
}
```

- [ ] **Step 4: Extend the RETURNING clause in the upsert SQL (around line 625)**

Change:
```go
if err := w.DB.NewRaw(
	`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
	 VALUES (?, ?, ?, ?, ?)
	 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
	 RETURNING id, (xmax = 0) AS is_new`,
	ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
).Scan(ctx, &isNewRow); err != nil {
```

To:
```go
if err := w.DB.NewRaw(
	`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
	 VALUES (?, ?, ?, ?, ?)
	 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
	 RETURNING id, (xmax = 0) AS is_new, play_status`,
	ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
).Scan(ctx, &isNewRow); err != nil {
```

- [ ] **Step 5: Add play_status inference after the platform loop (around line 735)**

After the closing `}` of the `for _, egp := range egPlatforms` loop and before the `UPDATE external_games SET updated_at` statement, insert:

```go
var totalHours float64
for _, egp := range egPlatforms {
	totalHours += egp.HoursPlayed
}
if totalHours > 0 && (isNewRow.IsNew || (isNewRow.PlayStatus != nil && *isNewRow.PlayStatus == "not_started")) {
	if _, err := w.DB.NewRaw(
		`UPDATE user_games SET play_status = 'in_progress' WHERE id = ?`, ugID,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: update play_status", "err", err, "item_id", p.JobItemID)
	}
}
```

- [ ] **Step 6: Run the new tests to confirm they pass**

```bash
go test ./internal/worker/tasks/... -run "TestUserGameWorker_PlayStatus" -v
```

Expected: all four `PASS`.

- [ ] **Step 7: Run the full tasks test suite to confirm no regressions**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -20
```

Expected: all existing tests continue to `PASS`.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat: infer play_status from hours in sync worker (#706)"
```

---

## Task 3: Update docs/sync.md — add Play Status subsection

**Files:**
- Modify: `docs/sync.md`

- [ ] **Step 1: Add Play Status subsection after line 227 (after the Stage 3 numbered list)**

After the paragraph ending with `...the periodic bulk refresh (see [docs/maintenance.md](maintenance.md) § "Metadata refresh") remains the safety net.` (line 227), and before `### Ownership rank guard`, insert:

```markdown
### Play Status

`user_games.play_status` defaults to `'not_started'`. Sync infers an initial status from the incoming hours:

- If total `hours_played` across all `external_game_platforms` rows for the game is **> 0**, and the current `play_status` is `'not_started'` (either because the row is new, or because it was previously unplayed), sync sets `play_status = 'in_progress'`.
- If total hours = 0, the DB default (`'not_started'`) applies and nothing is changed.

Sync can only auto-promote `not_started → in_progress`. Any other status the user has explicitly set (e.g. `'completed'`, `'on_hold'`) is never touched by sync.

Manually added games that omit `play_status` default to `'not_started'` via the DB default. The sync worker applies the same inference when it later processes that game from a storefront.
```

- [ ] **Step 2: Verify the doc reads correctly**

```bash
grep -A 20 "### Play Status" docs/sync.md
```

Expected: the new section text is visible.

- [ ] **Step 3: Commit**

```bash
git add docs/sync.md
git commit -m "docs: add play_status section to sync.md (#706)"
```
