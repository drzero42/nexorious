# Sync Remaining Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three remaining spec gaps in the sync system: job_item marking on skip, sibling push on rematch, and in-flight game filtering on the external games list.

**Architecture:** All three changes are isolated to `internal/api/sync.go` (production code) and `internal/api/sync_test.go` (tests). No new files, no new packages. G1 calls `tasks.SyncCheckJobCompletion` directly from the HTTP handler — the API package already imports `tasks` and this is the only way to drive job completion after a skip (no River worker fires). G2 inserts a sibling-resolution loop immediately after the existing primary-resolution block. G3 adds a single NOT EXISTS subquery to the WHERE clause.

**Tech Stack:** Go 1.25, Bun ORM, River (riverqueue/river), Echo v5, stdlib `testing`, testcontainers-go.

**Spec:** `docs/superpowers/specs/2026-05-25-sync-remaining-gaps-design.md`

---

## File Map

| File | Change |
|------|--------|
| `internal/api/sync.go` | G1: add job_item marking + SyncCheckJobCompletion call in HandleSkipGame; G2: add sibling loop in HandleRematchExternalGame; G3: add NOT EXISTS filter in HandleListExternalGames |
| `internal/api/sync_test.go` | New tests for G1, G2, G3 |

Test helpers available (defined across `internal/api/*_test.go`):
- `insertExternalGame(t, db, id, userID, storefront, extID, title)` — inserts an external_game row
- `insertJob(t, db, id, userID, jobType, source, status)` — inserts a jobs row
- `insertJobItem(t, db, id, jobID, userID, itemKey, sourceTitle, status)` — inserts job_items (no external_game_id column)
- `insertUserGameAndPlatform(t, db, ugID, userID, gameIDStr, ugpID, externalGameID)` — inserts games + user_games + user_game_platforms
- `newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})` — returns an Echo handler

For job_items that need `external_game_id` set, use raw SQL inline in the test (the shared helper omits that column).

---

## Task 1: G1 — HandleSkipGame marks job_item skipped and calls SyncCheckJobCompletion

**Files:**
- Modify: `internal/api/sync.go` (~line 863, the `HandleSkipGame` function)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add after `TestIgnored_SkipAndUnskip` in `internal/api/sync_test.go`:

```go
func TestSkipGame_MarksJobItemSkippedAndCompletesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "skip-jobitem")
	insertExternalGame(t, testDB, "eg-skip-ji", userID, "steam", "777", "Skip Me")
	insertJob(t, testDB, "job-skip-ji", userID, "sync", "steam", "processing")
	// Insert a pending_review job_item linked to the external_game.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-skip-1', 'job-skip-ji', ?, '777', 'Skip Me', 'eg-skip-ji', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert job_item: %v", err)
	}

	rec := postJSONAuth(t, e, "/api/sync/ignored/eg-skip-ji", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	var itemStatus string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = 'ji-skip-1'`).Scan(ctx, &itemStatus); err != nil {
		t.Fatalf("scan job_item status: %v", err)
	}
	if itemStatus != "skipped" {
		t.Errorf("expected job_item status=skipped, got %q", itemStatus)
	}

	var jobStatus string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = 'job-skip-ji'`).Scan(ctx, &jobStatus); err != nil {
		t.Fatalf("scan job status: %v", err)
	}
	if jobStatus != "completed" {
		t.Errorf("expected job status=completed after last item skipped, got %q", jobStatus)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestSkipGame_MarksJobItemSkippedAndCompletesJob -v -timeout 600s 2>&1 | tail -15
```

Expected: FAIL — `expected job_item status=skipped, got "pending_review"`.

- [ ] **Step 3: Add job_item marking and SyncCheckJobCompletion to HandleSkipGame**

In `internal/api/sync.go`, replace the `HandleSkipGame` function body. Find the return statement at the end (currently just `return c.NoContent(http.StatusNoContent)`) and add the new block before it:

```go
func (h *SyncHandler) HandleSkipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var ownerID string
	err := h.db.NewRaw(`SELECT user_id FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerID)
	if errors.Is(err, sql.ErrNoRows) || ownerID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: skip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}

	// Mark the most recent pending_review or pending job_item for this game as skipped,
	// then check whether the job can now complete.
	var jobItemRow struct {
		ID    string `bun:"id"`
		JobID string `bun:"job_id"`
	}
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

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
go test ./internal/api/... -run TestSkipGame_MarksJobItemSkippedAndCompletesJob -v -timeout 600s 2>&1 | tail -15
```

Expected: PASS.

- [ ] **Step 5: Run the full sync test suite**

```bash
go test ./internal/api/... -run TestIgnored -v -timeout 600s 2>&1 | tail -20
```

Expected: all existing `TestIgnored_*` tests still PASS (the new code only fires when a job_item exists; existing tests have no job_item so the block is skipped silently).

- [ ] **Step 6: Build**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "$(cat <<'EOF'
fix(sync): HandleSkipGame marks job_item skipped and calls SyncCheckJobCompletion

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: G2 — HandleRematchExternalGame resolves sibling external games

**Files:**
- Modify: `internal/api/sync.go` (end of `HandleRematchExternalGame`, after the existing Stage 3 enqueue)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add after `TestRematchExternalGame_RemoveOrphan` in `internal/api/sync_test.go`:

```go
func TestRematchExternalGame_ResolvesSiblings(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-siblings")

	// Two PSN external_games for the same game title (PS4 + PS5 variants).
	insertExternalGame(t, testDB, "eg-ps4", userID, "psn", "PPSA-001", "Spider-Man 2")
	insertExternalGame(t, testDB, "eg-ps5", userID, "psn", "PPSA-002", "Spider-Man 2")

	// A recent sync job for the sibling fallback path.
	insertJob(t, testDB, "job-sib", userID, "sync", "psn", "processing")

	// pending_review job_item for the primary (PS4).
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-ps4', 'job-sib', ?, 'PPSA-001', 'Spider-Man 2', 'eg-ps4', '{}', 'pending_review', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert primary job_item: %v", err)
	}

	// No job_item for the sibling (PS5) — the fallback path will create one.

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (5555, 'Spider-Man 2', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-ps4/rematch",
		map[string]any{"igdb_id": 5555}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()

	// Sibling (PS5) should now have resolved_igdb_id set.
	var sibResolvedID *int32
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-ps5'`).Scan(ctx, &sibResolvedID); err != nil {
		t.Fatalf("scan sibling resolved_igdb_id: %v", err)
	}
	if sibResolvedID == nil || *sibResolvedID != 5555 {
		t.Errorf("expected sibling resolved_igdb_id=5555, got %v", sibResolvedID)
	}

	// A job_item for the sibling should have been created (fallback path).
	var sibItemCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE external_game_id = 'eg-ps5'`,
	).Scan(ctx, &sibItemCount); err != nil {
		t.Fatalf("scan sibling job_item count: %v", err)
	}
	if sibItemCount != 1 {
		t.Errorf("expected 1 job_item for sibling, got %d", sibItemCount)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestRematchExternalGame_ResolvesSiblings -v -timeout 600s 2>&1 | tail -15
```

Expected: FAIL — `expected sibling resolved_igdb_id=5555, got <nil>`.

- [ ] **Step 3: Add sibling resolution loop to HandleRematchExternalGame**

In `internal/api/sync.go`, find `HandleRematchExternalGame`. Locate the final block that enqueues Stage 3 for the primary item:

```go
if h.riverClient != nil {
    if _, err := h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: jobItemID}, nil); err != nil {
        slog.Error("sync: enqueue user_game_write failed", "err", err, "job_item_id", jobItemID)
        return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
    }
}

return c.NoContent(http.StatusNoContent)
```

Replace it with:

```go
if h.riverClient != nil {
    if _, err := h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: jobItemID}, nil); err != nil {
        slog.Error("sync: enqueue user_game_write failed", "err", err, "job_item_id", jobItemID)
        return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
    }
}

// Resolve siblings: other external_games for the same (user, storefront, title) that are
// still unresolved. Each gets the same IGDB ID and its own Stage 3 job.
var siblings []struct {
    ID         string `bun:"id"`
    ExternalID string `bun:"external_id"`
    Title      string `bun:"title"`
}
if err := h.db.NewRaw(`
    SELECT id, external_id, title FROM external_games
    WHERE user_id = ? AND storefront = ? AND title = ?
      AND id != ? AND resolved_igdb_id IS NULL AND is_skipped = false`,
    userID, eg.Storefront, eg.Title, id,
).Scan(ctx, &siblings); err == nil {
    for _, sib := range siblings {
        if _, err := h.db.NewRaw(
            `UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
            body.IGDBID, sib.ID,
        ).Exec(ctx); err != nil {
            slog.Error("sync: rematch: resolve sibling", "err", err, "sibling_id", sib.ID)
            continue
        }
        var sibItemID string
        if err := h.db.NewRaw(`
            SELECT id FROM job_items
            WHERE external_game_id = ? AND status = 'pending_review'
            ORDER BY created_at DESC LIMIT 1`, sib.ID,
        ).Scan(ctx, &sibItemID); err != nil || sibItemID == "" {
            var recentJobID string
            if err2 := h.db.NewRaw(`
                SELECT id FROM jobs
                WHERE user_id = ? AND source = ? AND job_type = 'sync'
                ORDER BY created_at DESC LIMIT 1`,
                userID, eg.Storefront,
            ).Scan(ctx, &recentJobID); err2 != nil {
                slog.Error("sync: rematch: sibling no recent job", "sibling_id", sib.ID, "err", err2)
                continue
            }
            sibItemID = uuid.NewString()
            if _, err3 := h.db.NewRaw(`
                INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
                VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
                sibItemID, recentJobID, userID, sib.ExternalID, sib.Title, sib.ID,
            ).Exec(ctx); err3 != nil {
                slog.Error("sync: rematch: create sibling job_item", "sibling_id", sib.ID, "err", err3)
                continue
            }
        }
        if h.riverClient != nil {
            if _, err := h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: sibItemID}, nil); err != nil {
                slog.Error("sync: rematch: enqueue sibling Stage 3", "sibling_id", sib.ID, "err", err)
            }
        }
    }
}

return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 4: Run the failing test to confirm it now passes**

```bash
go test ./internal/api/... -run TestRematchExternalGame_ResolvesSiblings -v -timeout 600s 2>&1 | tail -15
```

Expected: PASS.

- [ ] **Step 5: Run the full rematch test suite**

```bash
go test ./internal/api/... -run TestRematchExternalGame -v -timeout 600s 2>&1 | tail -20
```

Expected: all `TestRematchExternalGame_*` tests PASS (the sibling loop only fires when there are matching rows; existing tests have no siblings).

- [ ] **Step 6: Build**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "$(cat <<'EOF'
fix(sync): HandleRematchExternalGame resolves sibling external games

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: G3 — HandleListExternalGames excludes in-flight games

**Files:**
- Modify: `internal/api/sync.go` (`HandleListExternalGames` SQL query, ~line 938)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add after `TestListExternalGames_AllStates` in `internal/api/sync_test.go`:

```go
func TestListExternalGames_ExcludesInFlight(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-inflight")

	insertExternalGame(t, testDB, "eg-stable", userID, "steam", "10", "Stable Game")
	insertExternalGame(t, testDB, "eg-inflight-1", userID, "steam", "20", "In-Flight Game")
	insertJob(t, testDB, "job-inflight", userID, "sync", "steam", "processing")

	// pending job_item links eg-inflight-1 to the active job.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES ('ji-inflight', 'job-inflight', ?, '20', 'In-Flight Game', 'eg-inflight-1', '{}', 'pending', '{}', '[]', now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert in-flight job_item: %v", err)
	}

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 game (in-flight excluded), got %d", len(resp))
	}
	if resp[0]["id"] != "eg-stable" {
		t.Errorf("expected eg-stable in response, got %v", resp[0]["id"])
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestListExternalGames_ExcludesInFlight -v -timeout 600s 2>&1 | tail -15
```

Expected: FAIL — `expected 1 game (in-flight excluded), got 2`.

- [ ] **Step 3: Add NOT EXISTS filter to HandleListExternalGames**

In `internal/api/sync.go`, find `HandleListExternalGames`. The WHERE clause currently reads:

```go
WHERE eg.user_id = ? AND eg.storefront = ?
ORDER BY eg.title ASC`,
```

Change it to:

```go
WHERE eg.user_id = ? AND eg.storefront = ?
  AND NOT EXISTS (
      SELECT 1 FROM job_items ji
      WHERE ji.external_game_id = eg.id
        AND ji.status IN ('pending', 'processing')
  )
ORDER BY eg.title ASC`,
```

- [ ] **Step 4: Run the failing test to confirm it now passes**

```bash
go test ./internal/api/... -run TestListExternalGames_ExcludesInFlight -v -timeout 600s 2>&1 | tail -15
```

Expected: PASS.

- [ ] **Step 5: Run the full external games test suite**

```bash
go test ./internal/api/... -run TestListExternalGames -v -timeout 600s 2>&1 | tail -20
```

Expected: all `TestListExternalGames_*` tests PASS. (`TestListExternalGames_AllStates` still expects 3 games — none of those have active job_items, so the filter does not affect them.)

- [ ] **Step 6: Build and full API test suite**

```bash
go build ./...
go test ./internal/api/... -timeout 600s 2>&1 | tail -10
```

Expected: clean build, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "$(cat <<'EOF'
fix(sync): HandleListExternalGames excludes in-flight games per spec

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Final Verification

- [ ] **Full backend test run**

```bash
go test -timeout 600s ./... 2>&1 | tail -20
```

Expected: all packages PASS.

- [ ] **Lint**

```bash
golangci-lint run 2>&1 | head -20
```

Expected: no findings.
