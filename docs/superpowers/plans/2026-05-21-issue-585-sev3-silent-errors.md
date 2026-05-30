# Issue #585: Log + Surface DB and Job-Queue Write Failures (Sev 3) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix ~50 sites across 7 files where DB writes and River inserts are silently discarded — API handlers return 500 on failure, background workers log-only, and critical-path River inserts mark the `job_items` row `failed` via `EnqueueOrFail`.

**Architecture:** For every `_, _ = db.NewRaw(...).Exec(...)` or `_, _ = riverClient.Insert(...)` in an API handler, replace with `if _, err := ...; err != nil { slog.Error(...); return echo.NewHTTPError(500, "...") }`. In workers and schedulers, replace with `if _, err := ...; err != nil { slog.Error(...) }`. The dispatch worker's critical-path River inserts (line 277 in `worker/tasks/sync.go`, line 136 in `metadata_refresh.go`) use the existing `EnqueueOrFail` helper which marks `job_items.status = 'failed'` on error so the item does not get stranded in `pending`.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM, River queue, `log/slog`, testcontainers-go.

**Spec reference:** `docs/superpowers/specs/2026-05-21-issue-534-silent-errors-design.md` — Sev 3 section.

**Dependencies:** Must land after Sev 1 (#583) and Sev 2 (#584/#591) PRs — both already merged.

---

## Task 1: Create feature branch

**Files:** (none)

- [ ] **Step 1: Create branch**

```bash
git checkout -b fix/issue-585-sev3-silent-errors
```

Expected: new branch checked out.

- [ ] **Step 2: Verify starting state**

```bash
go build ./... && go test -timeout 600s ./...
```

Expected: zero build errors, all tests pass.

---

## Task 2: Write the representative test (failing River insert → 500)

**Files:**
- Modify: `internal/api/auth_test.go` — add `newFailingRiverClient` helper
- Modify: `internal/api/sync_test.go` — add `newSyncTestAppWithRiverClient` helper + test

This test documents that `HandleTriggerSync` must return 500 when the River insert fails. It will **fail** on the current code (returns 200) and **pass** after Task 3's fix.

- [ ] **Step 1: Add `newFailingRiverClient` to auth_test.go**

Add after the `newTestRiverClient` function (around line 101):

```go
// newFailingRiverClient builds a River client backed by a pool that has been
// closed, so every Insert call returns an error. Used to test 500 responses on
// River insert failures.
func newFailingRiverClient(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testConnStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	pool.Close() // closed pool causes Insert to fail immediately
	return rc
}
```

- [ ] **Step 2: Add `newSyncTestAppWithRiverClient` helper to sync_test.go**

Add these imports to sync_test.go (alongside the existing ones):
```go
"github.com/jackc/pgx/v5"
"github.com/riverqueue/river"
```

Add the helper after `newSyncTestApp` (around line 50):

```go
func newSyncTestAppWithRiverClient(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient, rc *river.Client[pgx.Tx]) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	e := echo.New()
	ah := api.NewAuthHandler(testDB, cfg)
	e.POST("/api/auth/login", ah.HandleLogin)
	synch := api.NewSyncHandler(db, rc, steam, psn, (api.EpicClient)(nil), (api.GOGClient)(nil))
	g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
	synch.RegisterRoutes(g)
	return e
}
```

- [ ] **Step 3: Write the failing test in sync_test.go**

```go
// TestHandleTriggerSync_RiverInsertFails_Returns500 locks in the contract that
// HandleTriggerSync returns 500 when the River enqueue fails, rather than
// silently succeeding with an orphaned job.
func TestHandleTriggerSync_RiverInsertFails_Returns500(t *testing.T) {
	truncateAllTables(t)
	rc := newFailingRiverClient(t)
	e := newSyncTestAppWithRiverClient(t, testDB, &stubSteamClient{}, &stubPSNClient{}, rc)
	_, token := setupTagUser(t, testDB, e, "trigger-river-fail")

	rec := postJSONAuth(t, e, "/api/sync/steam", nil, token)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when River insert fails, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 4: Run the new test — verify it FAILS**

```bash
go test ./internal/api/... -run TestHandleTriggerSync_RiverInsertFails_Returns500 -v
```

Expected: FAIL — current code returns 200 (ignores River error).

---

## Task 3: Fix `internal/api/sync.go` — all handler Sev 3 sites

**Files:**
- Modify: `internal/api/sync.go`

18 changes: 17 return 500, 1 log-only (line 686 — best-effort Epic cleanup).

The `log/slog` import is already present. Use `slog.Error("sync: <action> failed", "err", err, "<key>", value)`.

- [ ] **Step 1: Fix line 393 — HandleTriggerSync River insert → log + 500**

Replace:
```go
	if h.riverClient != nil {
		_, _ = h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID: jobID, UserID: userID, Storefront: sf,
		}, nil)
	}
```

With:
```go
	if h.riverClient != nil {
		if _, err = h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID: jobID, UserID: userID, Storefront: sf,
		}, nil); err != nil {
			slog.Error("sync: enqueue dispatch failed", "err", err, "job_id", jobID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync job")
		}
	}
```

Note: `err` is already declared earlier in `HandleTriggerSync` (from the job INSERT check), so use `=` not `:=`.

- [ ] **Step 2: Fix line 491 — HandleSteamVerify DB insert → log + 500**

Replace:
```go
	_, _ = h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background())
```

With:
```go
	if _, err := h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background()); err != nil {
		slog.Error("sync: persist steam credentials failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Steam connection")
	}
```

- [ ] **Step 3: Fix line 504 — HandleSteamDisconnect DB update → log + 500**

Replace:
```go
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Exec(context.Background())
	return c.NoContent(http.StatusNoContent)
```

With:
```go
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: steam disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Steam")
	}
	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 4: Fix line 608 — HandlePSNDisconnect DB update → log + 500**

Replace:
```go
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
		userID,
	).Exec(context.Background())
	return c.NoContent(http.StatusNoContent)
```

With:
```go
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: psn disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect PSN")
	}
	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 5: Fix lines 681 and 686 — HandleEpicDisconnect (DB update → 500, cleanup → log-only)**

Replace:
```go
	ctx := context.Background()
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, epic_legendary_state = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Exec(ctx)
	if h.epicClient != nil {
		_ = h.epicClient.Cleanup(ctx, userID)
	}
	return c.NoContent(http.StatusNoContent)
```

With:
```go
	ctx := context.Background()
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, epic_legendary_state = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Exec(ctx); err != nil {
		slog.Error("sync: epic disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Epic")
	}
	if h.epicClient != nil {
		if err := h.epicClient.Cleanup(ctx, userID); err != nil {
			slog.Error("sync: epic cleanup failed", "err", err, "user_id", userID)
		}
	}
	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 6: Fix line 774 — HandleSkipGame DB update → log + 500**

Replace:
```go
	_, _ = h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx)
	return c.NoContent(http.StatusNoContent)
```

With:
```go
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: skip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}
	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 7: Fix lines 797, 816, 822 — HandleUnskipGame (unskip update, job_items insert, River insert → all 500)**

Replace:
```go
	_, _ = h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx)

	// Enqueue immediate re-processing. Failure here is non-fatal — the game
	// will be picked up on the next full sync.
	jobID := uuid.NewString()
	now := time.Now().UTC()
	_, jerr := h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'high', 1, ?)`,
		jobID, userID, eg.Storefront, now,
	).Exec(ctx)
	if jerr == nil {
		meta, _ := json.Marshal(map[string]string{
			"external_game_id": eg.ID,
			"raw_platform":     eg.RawPlatform,
		})
		itemID := uuid.NewString()
		_, _ = h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
		).Exec(ctx)
		if h.riverClient != nil {
			_, _ = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil)
		}
	}

	return c.NoContent(http.StatusNoContent)
```

With:
```go
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: unskip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unskip game")
	}

	// Enqueue immediate re-processing. Failure here is non-fatal — the game
	// will be picked up on the next full sync.
	jobID := uuid.NewString()
	now := time.Now().UTC()
	_, jerr := h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'high', 1, ?)`,
		jobID, userID, eg.Storefront, now,
	).Exec(ctx)
	if jerr == nil {
		meta, _ := json.Marshal(map[string]string{
			"external_game_id": eg.ID,
			"raw_platform":     eg.RawPlatform,
		})
		itemID := uuid.NewString()
		if _, err := h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
		).Exec(ctx); err != nil {
			slog.Error("sync: insert job_item for unskip failed", "err", err, "external_game_id", eg.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
				slog.Error("sync: enqueue process_sync_item failed", "err", err, "job_item_id", itemID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
			}
		}
	}

	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 8: Fix lines 897+901 — HandleResetSyncData cancel-active-job block → log + 500**

Replace:
```go
	if err := h.db.NewRaw(
		`SELECT * FROM jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &activeJob); err == nil {
		_, _ = h.db.NewRaw(
			`UPDATE jobs SET status = ?, completed_at = now() WHERE id = ?`,
			models.JobStatusCancelled, activeJob.ID,
		).Exec(ctx)
		_, _ = h.db.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = now()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
			activeJob.ID,
		).Exec(ctx)
	}
```

With:
```go
	if err := h.db.NewRaw(
		`SELECT * FROM jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &activeJob); err == nil {
		if _, err := h.db.NewRaw(
			`UPDATE jobs SET status = ?, completed_at = now() WHERE id = ?`,
			models.JobStatusCancelled, activeJob.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: cancel active job failed", "err", err, "job_id", activeJob.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel active job")
		}
		if _, err := h.db.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = now()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
			activeJob.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: cancel river jobs failed", "err", err, "job_id", activeJob.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel queued tasks")
		}
	}
```

- [ ] **Step 9: Fix lines 984, 988, 993, 999, 1031 — HandleRematchExternalGame orphan+game+River → log + 500**

Replace the entire "Apply orphan decision" through "Enqueue ProcessSyncItem" block:
```go
		// Delete the platform link.
		_, _ = h.db.NewRaw(`DELETE FROM user_game_platforms WHERE id = ?`, ugpID).Exec(ctx)

		// Apply orphan decision.
		if otherCount == 0 && body.OrphanAction == "remove" {
			_, _ = h.db.NewRaw(`DELETE FROM user_games WHERE id = ?`, ugID).Exec(ctx)
		}
	}

	// Ensure the games row exists (FK on external_games.resolved_igdb_id).
	_, _ = h.db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		body.IGDBID, eg.Title,
	).Exec(ctx)

	// Update external_game.
	_, _ = h.db.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id = ?`,
		body.IGDBID, id,
	).Exec(ctx)
```

With:
```go
		// Delete the platform link.
		if _, err := h.db.NewRaw(`DELETE FROM user_game_platforms WHERE id = ?`, ugpID).Exec(ctx); err != nil {
			slog.Error("sync: delete user_game_platform failed", "err", err, "ugp_id", ugpID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove platform link")
		}

		// Apply orphan decision.
		if otherCount == 0 && body.OrphanAction == "remove" {
			if _, err := h.db.NewRaw(`DELETE FROM user_games WHERE id = ?`, ugID).Exec(ctx); err != nil {
				slog.Error("sync: delete user_game failed", "err", err, "user_game_id", ugID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove game")
			}
		}
	}

	// Ensure the games row exists (FK on external_games.resolved_igdb_id).
	if _, err := h.db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		body.IGDBID, eg.Title,
	).Exec(ctx); err != nil {
		slog.Error("sync: ensure game row failed", "err", err, "igdb_id", body.IGDBID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve game")
	}

	// Update external_game.
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id = ?`,
		body.IGDBID, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: update external_game resolution failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update external game")
	}
```

Also replace the River insert near line 1031:
```go
	if h.riverClient != nil {
		_, _ = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil)
	}
```

With:
```go
	if h.riverClient != nil {
		if _, err = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
			slog.Error("sync: enqueue process_sync_item failed", "err", err, "job_item_id", itemID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
		}
	}
```

Note: `err` is already declared earlier in `HandleRematchExternalGame`, so use `=` not `:=`.

- [ ] **Step 10: Fix line 1092 — HandleGOGDisconnect DB update → log + 500**

Replace:
```go
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
		userID,
	).Exec(context.Background())
	return c.NoContent(http.StatusNoContent)
```

With:
```go
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: gog disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect GOG")
	}
	return c.NoContent(http.StatusNoContent)
```

- [ ] **Step 11: Build to verify — no compile errors**

```bash
go build ./internal/api/...
```

Expected: exits 0, no output.

- [ ] **Step 12: Run the representative test — verify it now PASSES**

```bash
go test ./internal/api/... -run TestHandleTriggerSync_RiverInsertFails_Returns500 -v
```

Expected: PASS.

- [ ] **Step 13: Run all api tests**

```bash
go test -timeout 600s ./internal/api/...
```

Expected: all pass.

---

## Task 4: Fix `internal/api/jobs.go` — handler Sev 3 sites

**Files:**
- Modify: `internal/api/jobs.go`

2 sites: both return 500. `log/slog` already imported.

- [ ] **Step 1: Fix line 541 — HandleCancelJob River-job cancel → log + 500**

Replace (inside `HandleCancelJob`, after the `jobs` row update that already has error handling):
```go
	// Cancel any queued River jobs for this nexorious job. ImportItemArgs serialises
	// as {"job_item_id": "..."}, so match against the job_items table.
	_, _ = h.db.NewRaw(`
		UPDATE river_job
		SET state = 'cancelled', finalized_at = NOW()
		WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
		  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
		jobID,
	).Exec(context.Background())
```

With:
```go
	// Cancel any queued River jobs for this nexorious job. ImportItemArgs serialises
	// as {"job_item_id": "..."}, so match against the job_items table.
	if _, err := h.db.NewRaw(`
		UPDATE river_job
		SET state = 'cancelled', finalized_at = NOW()
		WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
		  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
		jobID,
	).Exec(context.Background()); err != nil {
		slog.Error("jobs: cancel river jobs failed", "err", err, "job_id", jobID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel queued tasks")
	}
```

- [ ] **Step 2: Fix line 634 — HandleRetryFailedItems job status reset → log + 500**

Replace (inside `HandleRetryFailedItems`, the job status update that uses `_, _`):
```go
	// Reset job status to processing and clear auto_retry_done so that a
	// subsequent IGDB failure can trigger another automatic retry cycle.
	_, _ = h.db.NewRaw(`
		UPDATE jobs SET status = ?, auto_retry_done = false WHERE id = ?`,
		models.JobStatusProcessing, jobID,
	).Exec(context.Background())
```

With:
```go
	// Reset job status to processing and clear auto_retry_done so that a
	// subsequent IGDB failure can trigger another automatic retry cycle.
	if _, err := h.db.NewRaw(`
		UPDATE jobs SET status = ?, auto_retry_done = false WHERE id = ?`,
		models.JobStatusProcessing, jobID,
	).Exec(context.Background()); err != nil {
		slog.Error("jobs: reset job status failed", "err", err, "job_id", jobID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset job status")
	}
```

- [ ] **Step 3: Build + run api tests**

```bash
go build ./internal/api/... && go test -timeout 600s ./internal/api/...
```

Expected: exits 0, all pass.

---

## Task 5: Fix `internal/api/job_items.go` — handler Sev 3 sites

**Files:**
- Modify: `internal/api/job_items.go`

7 sites inside `HandleResolveItem` (lines 100, 105, 111, 117, 123, 128) and `HandleSkipItem` (line 195). All return 500. Need to add `"log/slog"` import.

- [ ] **Step 1: Add `"log/slog"` to the import block**

Change the import block from:
```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"
	...
)
```

To:
```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
	...
)
```

- [ ] **Step 2: Fix lines 100, 105 — HandleResolveItem games + external_games cascade writes → 500**

Inside the `if egErr == nil {` block in `HandleResolveItem`, replace:
```go
			// Ensure the games row exists (FK on external_games.resolved_igdb_id).
			_, _ = h.db.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				body.IGDBID, eg.Title,
			).Exec(context.Background())
			// Resolve the matched external_game immediately so step 3.6 can find it.
			_, _ = h.db.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				body.IGDBID, eg.ID,
			).Exec(context.Background())
```

With:
```go
			// Ensure the games row exists (FK on external_games.resolved_igdb_id).
			if _, err := h.db.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				body.IGDBID, eg.Title,
			).Exec(context.Background()); err != nil {
				slog.Error("job_items: ensure game row failed", "err", err, "igdb_id", body.IGDBID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve game")
			}
			// Resolve the matched external_game immediately so step 3.6 can find it.
			if _, err := h.db.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				body.IGDBID, eg.ID,
			).Exec(context.Background()); err != nil {
				slog.Error("job_items: resolve external_game failed", "err", err, "external_game_id", eg.ID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve external game")
			}
```

- [ ] **Step 3: Fix lines 111, 117, 123, 128 — sibling lookup + cascade → 500**

Replace:
```go
			// Find sibling external_games (same user/storefront/title, different SKU, still unresolved).
			var siblings []models.ExternalGame
			_ = h.db.NewSelect().Model(&siblings).
				Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NULL",
					eg.UserID, eg.Storefront, eg.Title, eg.ID).
				Scan(context.Background())
			for _, sib := range siblings {
				// Resolve the sibling external_game so step 3.6 in the worker skips IGDB search.
				_, _ = h.db.NewRaw(
					`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
					body.IGDBID, sib.ID,
				).Exec(context.Background())
				// Re-queue any pending_review job_items for this sibling.
				var sibItems []models.JobItem
				_ = h.db.NewRaw(
					`SELECT * FROM job_items WHERE user_id = ? AND status = 'pending_review' AND source_metadata->>'external_game_id' = ?`,
					eg.UserID, sib.ID,
				).Scan(context.Background(), &sibItems)
				for _, si := range sibItems {
					_, _ = h.db.NewRaw(
						`UPDATE job_items SET status = 'pending' WHERE id = ?`, si.ID,
					).Exec(context.Background())
					var sibJob models.Job
					if jErr := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, si.JobID).Scan(context.Background(), &sibJob); jErr == nil {
						retryInsert(context.Background(), h.db, h.riverClient, sibJob.JobType, si.ID)
					}
				}
			}
```

With:
```go
			// Find sibling external_games (same user/storefront/title, different SKU, still unresolved).
			var siblings []models.ExternalGame
			if err := h.db.NewSelect().Model(&siblings).
				Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NULL",
					eg.UserID, eg.Storefront, eg.Title, eg.ID).
				Scan(context.Background()); err != nil {
				slog.Error("job_items: query siblings failed", "err", err, "external_game_id", eg.ID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to query sibling games")
			}
			for _, sib := range siblings {
				// Resolve the sibling external_game so step 3.6 in the worker skips IGDB search.
				if _, err := h.db.NewRaw(
					`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
					body.IGDBID, sib.ID,
				).Exec(context.Background()); err != nil {
					slog.Error("job_items: resolve sibling external_game failed", "err", err, "sibling_id", sib.ID)
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve sibling game")
				}
				// Re-queue any pending_review job_items for this sibling.
				var sibItems []models.JobItem
				if err := h.db.NewRaw(
					`SELECT * FROM job_items WHERE user_id = ? AND status = 'pending_review' AND source_metadata->>'external_game_id' = ?`,
					eg.UserID, sib.ID,
				).Scan(context.Background(), &sibItems); err != nil {
					slog.Error("job_items: query sibling job_items failed", "err", err, "sibling_id", sib.ID)
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to query sibling items")
				}
				for _, si := range sibItems {
					if _, err := h.db.NewRaw(
						`UPDATE job_items SET status = 'pending' WHERE id = ?`, si.ID,
					).Exec(context.Background()); err != nil {
						slog.Error("job_items: re-queue sibling job_item failed", "err", err, "job_item_id", si.ID)
						return echo.NewHTTPError(http.StatusInternalServerError, "failed to re-queue sibling item")
					}
					var sibJob models.Job
					if jErr := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, si.JobID).Scan(context.Background(), &sibJob); jErr == nil {
						retryInsert(context.Background(), h.db, h.riverClient, sibJob.JobType, si.ID)
					}
				}
			}
```

- [ ] **Step 4: Fix line 195 — HandleSkipItem external_game mark-skipped → 500**

Replace (inside `HandleSkipItem`, the external_games update):
```go
	if json.Unmarshal(item.SourceMetadata, &meta) == nil && meta.ExternalGameID != "" {
		_, _ = h.db.NewRaw(
			`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ? AND user_id = ?`,
			meta.ExternalGameID, userID,
		).Exec(context.Background())
	}
```

With:
```go
	if json.Unmarshal(item.SourceMetadata, &meta) == nil && meta.ExternalGameID != "" {
		if _, err := h.db.NewRaw(
			`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ? AND user_id = ?`,
			meta.ExternalGameID, userID,
		).Exec(context.Background()); err != nil {
			slog.Error("job_items: mark external_game skipped failed", "err", err, "external_game_id", meta.ExternalGameID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
		}
	}
```

- [ ] **Step 5: Build + run api tests**

```bash
go build ./internal/api/... && go test -timeout 600s ./internal/api/...
```

Expected: exits 0, all pass.

- [ ] **Step 6: Commit progress so far**

```bash
git add internal/api/sync.go internal/api/jobs.go internal/api/job_items.go \
        internal/api/auth_test.go internal/api/sync_test.go
git commit -m "$(cat <<'EOF'
fix: log + surface DB and job-queue write failures in API handlers (issue #534 sev 3)

Fixes ~27 handler sites across sync.go, jobs.go, and job_items.go where DB
writes and River inserts were silently discarded. All handler sites now log
via slog.Error and return 500 on failure. Adds representative test: River
insert failure in HandleTriggerSync returns 500.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Fix `internal/worker/tasks/sync.go` — worker Sev 3 sites

**Files:**
- Modify: `internal/worker/tasks/sync.go`

26 sites: 25 log-only, 1 critical-path River insert (line 277 → `EnqueueOrFail`).

The `log/slog` import is already present. Use `slog.Error("dispatch_sync: <action> failed", "err", err, "job_id", p.JobID)` or `slog.Error("process_sync_item: <action> failed", ...)` depending on the worker.

**txn note:** Lines 270+277 are a pair (INSERT job_items → River insert). `EnqueueOrFail` handles the inconsistency: if line 270 inserts the job_item and line 277 fails, `EnqueueOrFail` marks the item `failed` so it does not sit in `pending` without a backing River job.

- [ ] **Step 1: Fix line 155 — DispatchSyncWorker mark-processing → log-only**

Replace:
```go
	_, _ = w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx)
```

With:
```go
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}
```

- [ ] **Step 2: Fix line 198 — Steam cache query → log-only**

Replace:
```go
		_ = w.DB.NewRaw(
			`SELECT external_id, raw_platform FROM external_games WHERE user_id = ? AND storefront = 'steam'`,
			p.UserID,
		).Scan(ctx, &cachedRows)
```

With:
```go
		if err := w.DB.NewRaw(
			`SELECT external_id, raw_platform FROM external_games WHERE user_id = ? AND storefront = 'steam'`,
			p.UserID,
		).Scan(ctx, &cachedRows); err != nil {
			slog.Error("dispatch_sync: steam load cached rows failed", "err", err, "job_id", p.JobID)
		}
```

- [ ] **Step 3: Fix line 252 — Steam external_game upsert → log-only**

Replace:
```go
				_, _ = w.DB.NewInsert().Model(row).
					On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
					Exec(ctx)
```

With:
```go
				if _, err := w.DB.NewInsert().Model(row).
					On("CONFLICT (user_id, storefront, external_id, raw_platform) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, is_available = true, updated_at = now()").
					Exec(ctx); err != nil {
					slog.Error("dispatch_sync: steam upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", appidStr)
				}
```

- [ ] **Step 4: Fix line 259 — Steam toProcess SELECT → log-only**

Replace:
```go
		_ = w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx)
```

With:
```go
		if err := w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx); err != nil {
			slog.Error("dispatch_sync: steam query to-process failed", "err", err, "job_id", p.JobID)
		}
```

- [ ] **Step 5: Fix lines 270+277 — Steam INSERT job_items (log-only) and River insert (critical path → EnqueueOrFail)**

Replace:
```go
			_, _ = w.DB.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				 ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, itemKey, eg.Title, string(metaJSON),
			).Exec(ctx)
			if w.RiverClient != nil {
				_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
			}
```

With:
```go
			if _, err := w.DB.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				 ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, itemKey, eg.Title, string(metaJSON),
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: steam insert job_item failed", "err", err, "job_id", p.JobID, "external_id", eg.ExternalID)
			}
			if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, ProcessSyncItemArgs{JobItemID: itemID}); err != nil {
				slog.Error("dispatch_sync: steam enqueue failed", "err", err, "job_id", p.JobID, "item_id", itemID)
			}
```

Note: `EnqueueOrFail` is in the same `tasks` package — no import needed.

- [ ] **Step 6: Fix line 375 — PSN token-expiry credential persist → log-only**

Replace:
```go
				if b, merr := json.Marshal(newCreds); merr == nil {
					s := string(b)
					_, _ = w.DB.NewRaw(
						`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
						s, p.UserID,
					).Exec(context.Background())
				}
```

With:
```go
				if b, merr := json.Marshal(newCreds); merr == nil {
					s := string(b)
					if _, err := w.DB.NewRaw(
						`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
						s, p.UserID,
					).Exec(context.Background()); err != nil {
						slog.Error("dispatch_sync: persist expired psn token failed", "err", err, "job_id", p.JobID)
					}
				}
```

- [ ] **Step 7: Fix line 490 — GOG token refresh persist → log-only**

Replace:
```go
		if newCredsJSON, merr := json.Marshal(creds); merr == nil {
			_, _ = w.DB.NewRaw(
				`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
				string(newCredsJSON), p.UserID,
			).Exec(context.Background())
		}
```

With:
```go
		if newCredsJSON, merr := json.Marshal(creds); merr == nil {
			if _, err := w.DB.NewRaw(
				`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
				string(newCredsJSON), p.UserID,
			).Exec(context.Background()); err != nil {
				slog.Error("dispatch_sync: persist refreshed gog token failed", "err", err, "job_id", p.JobID)
			}
		}
```

- [ ] **Step 8: Fix lines 585+590 — available-games SELECT and mark-unavailable UPDATE → log-only**

Replace:
```go
	var available []models.ExternalGame
	_ = w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx)
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx)
		}
	}
```

With:
```go
	var available []models.ExternalGame
	if err := w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		slog.Error("dispatch_sync: query available games failed", "err", err, "job_id", p.JobID)
	}
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: mark game unavailable failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
		}
	}
```

- [ ] **Step 9: Fix lines 599+610 — last_synced_at update and failSyncJob → log-only**

Replace:
```go
	syncedNow := time.Now().UTC()
	_, _ = w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background())
```

With:
```go
	syncedNow := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background()); err != nil {
		slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
	}
```

And in `failSyncJob`, replace:
```go
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx)
}
```

With:
```go
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: fail job update failed", "err", err, "job_id", jobID)
	}
}
```

- [ ] **Step 10: Fix lines 693+697 — ProcessSyncItem step 3.5 games+external_games writes → log-only**

Replace:
```go
		_, _ = w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx)
		_, _ = w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx)
	}

	// ── 3.6. Cross-SKU IGDB resolution
```

With:
```go
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert game row (step 3.5) failed", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: apply manual resolution failed", "err", err, "external_game_id", eg.ID)
		}
	}

	// ── 3.6. Cross-SKU IGDB resolution
```

- [ ] **Step 11: Fix lines 716+720 — ProcessSyncItem step 3.6 games+external_games writes → log-only**

Replace (inside the `if err == nil && sibling.ResolvedIGDBID != nil {` block):
```go
		_, _ = w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx)
		_, _ = w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx)
```

With:
```go
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert game row (step 3.6) failed", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: cross-sku resolution failed", "err", err, "external_game_id", eg.ID)
		}
```

- [ ] **Step 12: Fix lines 764+768 — IGDB auto-resolve games+external_games writes → log-only**

Replace (inside the `if bestScore >= autoResolveThreshold && ...` block):
```go
			_, _ = w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx)
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx)
```

With:
```go
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: insert game row (auto-resolve) failed", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: auto-resolve external_game failed", "err", err, "external_game_id", eg.ID)
			}
```

- [ ] **Step 13: Fix lines 809+826 — user_game insert + playtime update → log-only**

Replace (inside the `if err != nil { // Not found — insert }` block):
```go
		_, _ = w.DB.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, hours_played, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, float64(meta.PlaytimeHours), now, now,
		).Exec(ctx)
```

With:
```go
		if _, err := w.DB.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, hours_played, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, float64(meta.PlaytimeHours), now, now,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert user_game failed", "err", err, "job_item_id", p.JobItemID)
		}
```

And the playtime update:
```go
	if meta.PlaytimeHours > 0 {
		_, _ = w.DB.NewRaw(
			`UPDATE user_games SET hours_played = ?, updated_at = now() WHERE id = ? AND (hours_played IS NULL OR hours_played < ?)`,
			float64(meta.PlaytimeHours), ugID, float64(meta.PlaytimeHours),
		).Exec(ctx)
	}
```

With:
```go
	if meta.PlaytimeHours > 0 {
		if _, err := w.DB.NewRaw(
			`UPDATE user_games SET hours_played = ?, updated_at = now() WHERE id = ? AND (hours_played IS NULL OR hours_played < ?)`,
			float64(meta.PlaytimeHours), ugID, float64(meta.PlaytimeHours),
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: update playtime failed", "err", err, "job_item_id", p.JobItemID)
		}
	}
```

- [ ] **Step 14: Fix lines 855, 871, 877 — user_game_platform writes → log-only**

These are three separate writes in the `if errors.Is(ugpErr, sql.ErrNoRows) || ugpErr != nil` and `else` branches.

Replace the `_, _ = w.DB.NewRaw('INSERT INTO user_game_platforms...').Exec(ctx)` (line 855):
```go
		_, _ = w.DB.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
			  original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			ugpID, ugID, platformSlug, storefrontSlug, hoursPlayed, ownership,
			meta.RawPlatform, eg.Storefront, extGameID,
		).Exec(ctx)
```

With:
```go
		if _, err := w.DB.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
			  original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
			 VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			ugpID, ugID, platformSlug, storefrontSlug, hoursPlayed, ownership,
			meta.RawPlatform, eg.Storefront, extGameID,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: insert user_game_platform failed", "err", err, "job_item_id", p.JobItemID)
		}
```

Replace the ownership-rank update (line 871):
```go
			_, _ = w.DB.NewRaw(
				`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
				ownership, hoursPlayed, existingUGPID,
			).Exec(ctx)
```

With:
```go
			if _, err := w.DB.NewRaw(
				`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
				ownership, hoursPlayed, existingUGPID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: update ugp ownership failed", "err", err, "job_item_id", p.JobItemID)
			}
```

Replace the hours-only update (line 877):
```go
			_, _ = w.DB.NewRaw(
				`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
				hoursPlayed, existingUGPID,
			).Exec(ctx)
```

With:
```go
			if _, err := w.DB.NewRaw(
				`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
				hoursPlayed, existingUGPID,
			).Exec(ctx); err != nil {
				slog.Error("process_sync_item: update ugp playtime failed", "err", err, "job_item_id", p.JobItemID)
			}
```

- [ ] **Step 15: Fix lines 1018, 1036, 1046, 1068 — syncCheckJobCompletion terminal updates → log-only**

Line 1018 (`auto_retry_done = true`):
```go
			_, _ = db.NewRaw(`UPDATE jobs SET auto_retry_done = true WHERE id = ?`, jobID).Exec(ctx)
```
→
```go
			if _, err := db.NewRaw(`UPDATE jobs SET auto_retry_done = true WHERE id = ?`, jobID).Exec(ctx); err != nil {
				slog.Error("process_sync_item: set auto_retry_done failed", "err", err, "job_id", jobID)
			}
```

Line 1036 (`completed_with_errors` after all enqueue failures):
```go
				_, _ = db.NewRaw(
					`UPDATE jobs SET status = 'completed_with_errors', completed_at = ?
					 WHERE id = ? AND status IN ('pending', 'processing')`,
					now, jobID,
				).Exec(ctx)
```
→
```go
				if _, err := db.NewRaw(
					`UPDATE jobs SET status = 'completed_with_errors', completed_at = ?
					 WHERE id = ? AND status IN ('pending', 'processing')`,
					now, jobID,
				).Exec(ctx); err != nil {
					slog.Error("process_sync_item: finalize job (all-enqueue-failed) failed", "err", err, "job_id", jobID)
				}
```

Line 1046 (`completed_with_errors` after auto_retry_done):
```go
		_, _ = db.NewRaw(
			`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
			now, jobID,
		).Exec(ctx)
```
→
```go
		if _, err := db.NewRaw(
			`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
			now, jobID,
		).Exec(ctx); err != nil {
			slog.Error("process_sync_item: finalize job completed_with_errors failed", "err", err, "job_id", jobID)
		}
```

Line 1068 (`completed` terminal state):
```go
	_, _ = db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		now, jobID,
	).Exec(ctx)
```
→
```go
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("process_sync_item: finalize job completed failed", "err", err, "job_id", jobID)
	}
```

- [ ] **Step 16: Build + run worker tests**

```bash
go build ./internal/worker/... && go test -timeout 600s ./internal/worker/...
```

Expected: exits 0, all pass.

---

## Task 7: Fix `internal/worker/tasks/metadata_refresh.go` and `internal/scheduler/`

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/backup_poll.go`

`log/slog` already imported in all three files.

### `metadata_refresh.go`

- [ ] **Step 1: Fix line 136 — MetadataRefreshDispatchWorker River insert → EnqueueOrFail (critical path)**

Replace:
```go
	// Step 6 — Enqueue River jobs now that job_items are committed and visible.
	for _, itemID := range itemIDs {
		_, _ = w.RiverClient.Insert(ctx, MetadataRefreshItemArgs{JobItemID: itemID}, nil)
	}
```

With:
```go
	// Step 6 — Enqueue River jobs now that job_items are committed and visible.
	for _, itemID := range itemIDs {
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, MetadataRefreshItemArgs{JobItemID: itemID}); err != nil {
			slog.Error("metadata_refresh_dispatch: enqueue item failed", "err", err, "job_id", jobID, "item_id", itemID)
		}
	}
```

- [ ] **Step 2: Fix line 298 — MetadataRefreshItemWorker cover_art_url update → log-only**

Replace:
```go
			} else if coverURLPath != "" {
				_, _ = w.DB.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx)
			}
```

With:
```go
			} else if coverURLPath != "" {
				if _, err := w.DB.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx); err != nil {
					slog.Error("metadata_refresh_item: update cover_art_url failed", "err", err, "game_id", game.ID)
				}
			}
```

### `scheduler/scheduler.go`

- [ ] **Step 3: Fix line 185 — CheckPendingSyncs River insert → log-only (cron self-heals)**

Replace:
```go
		_, _ = w.RiverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID:      jobID,
			UserID:     cfg.UserID,
			Storefront: cfg.Storefront,
		}, nil)
```

With:
```go
		if _, err := w.RiverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID:      jobID,
			UserID:     cfg.UserID,
			Storefront: cfg.Storefront,
		}, nil); err != nil {
			slog.Error("CheckPendingSyncs: enqueue dispatch failed", "err", err, "job_id", jobID, "user_id", cfg.UserID)
		}
```

- [ ] **Step 4: Fix line 317 — CleanupExpiredExports DB update → log-only**

Replace:
```go
	_, _ = db.NewRaw(
		`UPDATE jobs SET file_path = NULL WHERE id IN (?)`,
		bun.List(ids),
	).Exec(ctx)
```

With:
```go
	if _, err := db.NewRaw(
		`UPDATE jobs SET file_path = NULL WHERE id IN (?)`,
		bun.List(ids),
	).Exec(ctx); err != nil {
		slog.Error("cleanup: clear expired export file_paths failed", "err", err)
	}
```

### `scheduler/backup_poll.go`

- [ ] **Step 5: Fix line 52 — CheckScheduledBackupWorker DB update → log-only**

Replace:
```go
	_, _ = w.DB.NewRaw(
		`UPDATE backup_config SET last_backup_at = now(), updated_at = now() WHERE id = 1`,
	).Exec(context.Background())
```

With:
```go
	if _, err := w.DB.NewRaw(
		`UPDATE backup_config SET last_backup_at = now(), updated_at = now() WHERE id = 1`,
	).Exec(context.Background()); err != nil {
		slog.Error("check_scheduled_backup: update last_backup_at failed", "err", err)
	}
```

- [ ] **Step 6: Build everything**

```bash
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 7: Run all tests**

```bash
go test -timeout 600s ./...
```

Expected: all pass.

---

## Task 8: Final verification and commit

- [ ] **Step 1: Confirm zero `_, _ = ` remain at any Sev 3 site**

```bash
grep -n "_, _ = " \
  internal/api/sync.go \
  internal/api/jobs.go \
  internal/api/job_items.go \
  internal/worker/tasks/sync.go \
  internal/worker/tasks/metadata_refresh.go \
  internal/scheduler/scheduler.go \
  internal/scheduler/backup_poll.go
```

Expected output: only lines that are **Acceptable** (these files should be clean — all Acceptable patterns are `defer ...Close()` or live in other files). If any unexpected `_, _ =` remain, fix them before continuing.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```

Expected: zero errors.

- [ ] **Step 3: Commit worker + scheduler fixes**

```bash
git add internal/worker/tasks/sync.go \
        internal/worker/tasks/metadata_refresh.go \
        internal/scheduler/scheduler.go \
        internal/scheduler/backup_poll.go
git commit -m "$(cat <<'EOF'
fix: log DB and job-queue failures in workers and scheduler (issue #534 sev 3)

Adds slog.Error to ~26 log-only sites in dispatch_sync, process_sync_item,
metadata_refresh, and scheduler workers where DB/River failures were silently
discarded. Critical-path River inserts in dispatch_sync (steam) and
metadata_refresh_dispatch now use EnqueueOrFail so job_items do not get
stranded in 'pending' without a backing River job.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Open a PR**

```bash
gh pr create \
  --title "fix: log + surface DB and job-queue write failures (issue #534 sev 3) (#585)" \
  --body "$(cat <<'EOF'
## Summary

- Fixes ~50 Sev 3 sites across `sync.go`, `jobs.go`, `job_items.go`, `worker/tasks/sync.go`, `metadata_refresh.go`, `scheduler.go`, and `backup_poll.go`
- API handler sites: replaced `_, _ = ...Exec()` / `_, _ = riverClient.Insert()` with log + 500
- Background worker sites: replaced `_, _ = ...Exec()` with `slog.Error` log-only
- Critical-path River inserts in Steam dispatch and metadata_refresh dispatch now use `EnqueueOrFail`, which marks `job_items.status='failed'` on error so items are not stranded in `pending`
- One representative test: `TestHandleTriggerSync_RiverInsertFails_Returns500`

## Test plan

- [ ] `go test -timeout 600s ./...` passes
- [ ] `golangci-lint run` reports zero errors
- [ ] Manually smoke-test: Steam disconnect, PSN disconnect, trigger sync, skip/unskip game, rematch external game — all should return 200 on success and 500 on DB error

Closes #585

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review against spec

### Spec coverage check

| Spec section | Plan task |
|---|---|
| `sync.go` lines 393, 491, 504, 605, 678, 768, 791, 810, 816, 891, 895, 978, 982, 987, 993, 1025, 1086 → 500 | Task 3, Steps 1–10 |
| `sync.go` line 683 → log-only (epic cleanup) | Task 3, Step 5 |
| `jobs.go` lines 541, 634 → 500 | Task 4, Steps 1–2 |
| `job_items.go` lines 100, 105, 111, 117, 123, 128, 195 → 500 | Task 5, Steps 2–4 |
| `worker/tasks/sync.go` 25 log-only + line 277 critical path | Task 6, Steps 1–15 |
| `metadata_refresh.go` line 136 (critical path), line 298 (log-only) | Task 7, Steps 1–2 |
| `scheduler.go` lines 185, 317 (log-only) | Task 7, Steps 3–4 |
| `backup_poll.go` line 52 (log-only) | Task 7, Step 5 |
| Representative test | Task 2 |

All spec requirements covered. No gaps found.

### Notes for the reviewer

- Line numbers in the plan match the **current** file state (post Sev 2 fixes), which is ~3–8 lines higher than the spec's `447e3daa` reference point.
- `HandleUnskipGame` has a `jerr` guard around the job_items insert — lines 816/822 fixes live inside the `if jerr == nil` block. The UPDATE at line 797 (unskip) returns 500 on failure; if it succeeds but the inner inserts fail, the game is unskipped but the client sees a 500. This is the documented "Behavior change in handlers" risk from the spec.
- The Steam dispatch critical path (lines 270+277) uses `EnqueueOrFail` rather than a transaction because `EnqueueOrFail` already provides the atomic "River insert failed → mark job_item failed" guarantee the spec requires.
- `job_items.go` requires adding `"log/slog"` to its import block — it is the only file among the 7 that does not already import it.
