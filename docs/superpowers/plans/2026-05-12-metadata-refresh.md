# Metadata Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a scheduled metadata refresh system that periodically re-fetches IGDB metadata for all games and updates their rows in the DB.

**Architecture:** A scheduler job fires a `metadata_refresh_dispatch` worker task on a configurable interval; that task creates a `jobs`/`job_items`/`pending_tasks` batch in a single transaction; individual `metadata_refresh_item` tasks call IGDB and update `games` rows. Estimated playtime (dead field) is removed everywhere as part of this change.

**Tech Stack:** Go, Bun ORM, gocron v2, testcontainers-go, IGDB service client.

---

## File Map

| File | Change |
|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Remove `estimated_playtime_hours` column from `CREATE TABLE games` |
| `internal/db/models/models.go` | Remove `EstimatedPlaytimeHours` field from `Game` struct |
| `internal/services/igdb/models.go` | Remove `EstimatedPlaytimeHours`; add `CoverImageID string` |
| `internal/services/igdb/igdb.go` | Populate `CoverImageID` alongside `CoverArtURL` in `convertToGameMetadata` |
| `internal/worker/tasks/import_item.go` | Fix `igdbMetadataToGame` to use `json.Marshal` for platform IDs/names; use `md.CoverImageID` for cover download |
| `internal/scheduler/scheduler.go` | Add `metadataRefreshInterval` field; add `cfg *config.Config` param to `NewScheduler`; register dispatch job in `Start` |
| `internal/worker/tasks/metadata_refresh.go` | **New file** — dispatch and item handlers + helpers |
| `internal/worker/tasks/metadata_refresh_test.go` | **New file** — 10 tests |
| `cmd/nexorious/main.go` | Register two new worker handlers; pass `cfg` to both `NewScheduler` call sites |
| `ui/frontend/src/api/games.ts` | Remove `estimated_playtime_hours` field and mapper pass-through |
| `ui/frontend/src/types/game.ts` | Remove `estimated_playtime_hours` field |
| `ui/frontend/src/api/games.test.ts` | Remove `estimated_playtime_hours` from mock fixture |
| `ui/frontend/src/hooks/use-games.test.tsx` | Remove `estimated_playtime_hours` from mock fixture |

---

## Task 1: Remove `estimated_playtime_hours` everywhere

This dead column has no data source and is never displayed. Remove it from the DB migration, Go model, and frontend types/tests. No migration file is needed — edit the initial migration directly and recreate the dev DB.

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql` (line 44)
- Modify: `internal/db/models/models.go` (line 22)
- Modify: `ui/frontend/src/api/games.ts` (lines 31, 305)
- Modify: `ui/frontend/src/types/game.ts` (line 98)
- Modify: `ui/frontend/src/api/games.test.ts` (line 40)
- Modify: `ui/frontend/src/hooks/use-games.test.tsx` (line 44)

- [ ] **Step 1: Remove from migration**

In `internal/db/migrations/20260503000001_initial.up.sql`, delete line 44:
```sql
    estimated_playtime_hours    INTEGER,
```

- [ ] **Step 2: Remove from Go model**

In `internal/db/models/models.go`, delete line 22:
```go
	EstimatedPlaytimeHours     *int32     `bun:"estimated_playtime_hours"       json:"estimated_playtime_hours"`
```

- [ ] **Step 3: Remove from frontend API type**

In `ui/frontend/src/api/games.ts`, delete the line:
```typescript
  estimated_playtime_hours?: number;
```
And in the same file, find the mapper function (around line 305) and delete:
```typescript
    estimated_playtime_hours: apiGame.estimated_playtime_hours,
```

- [ ] **Step 4: Remove from domain Game type**

In `ui/frontend/src/types/game.ts` (line 98), delete:
```typescript
  estimated_playtime_hours?: number;
```

- [ ] **Step 5: Remove from test fixtures**

In `ui/frontend/src/api/games.test.ts` (line 40), delete:
```typescript
  estimated_playtime_hours: 40,
```

In `ui/frontend/src/hooks/use-games.test.tsx` (line 44), delete:
```typescript
  estimated_playtime_hours: 20,
```

- [ ] **Step 6: Verify Go builds**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 7: Verify frontend type checks**

```bash
cd ui/frontend && npm run check
```
Expected: no TypeScript errors.

- [ ] **Step 8: Run all tests**

```bash
go test ./...
cd ui/frontend && npm run test
```
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql \
        internal/db/models/models.go \
        ui/frontend/src/api/games.ts \
        ui/frontend/src/types/game.ts \
        ui/frontend/src/api/games.test.ts \
        ui/frontend/src/hooks/use-games.test.tsx
git commit -m "chore: remove dead estimated_playtime_hours field"
```

---

## Task 2: Add `CoverImageID` to IGDB models and fix `convertToGameMetadata`

**Files:**
- Modify: `internal/services/igdb/models.go`
- Modify: `internal/services/igdb/igdb.go`

- [ ] **Step 1: Add `CoverImageID` to `GameMetadata`**

In `internal/services/igdb/models.go`, add the new field to `GameMetadata` after `CoverArtURL`:

```go
type GameMetadata struct {
	IgdbID                     int
	IgdbSlug                   string
	Title                      string
	Description                *string
	Genre                      *string
	Developer                  *string
	Publisher                  *string
	ReleaseDate                *string  // ISO date string "YYYY-MM-DD"
	CoverArtURL                *string
	CoverImageID               string   // IGDB image_id, e.g. "co1wyy". Empty when no cover.
	RatingAverage              *float64
	RatingCount                *int32
	HowlongtobeatMain         *float64
	HowlongtobeatExtra        *float64
	HowlongtobeatCompletionist *float64
	PlatformIDs                []int
	PlatformNames              []string
	GameModes                  *string
	Themes                     *string
	PlayerPerspectives         *string
}
```

- [ ] **Step 2: Populate `CoverImageID` in `convertToGameMetadata`**

In `internal/services/igdb/igdb.go`, find the `convertToGameMetadata` function and replace the cover block (currently ~line 370–373):

```go
// Before:
if g.Cover != nil && g.Cover.ImageID != "" {
    url := igdbImageBaseURL + g.Cover.ImageID + ".jpg"
    md.CoverArtURL = &url
}
```

Replace with:

```go
if g.Cover != nil && g.Cover.ImageID != "" {
    md.CoverImageID = g.Cover.ImageID
    url := igdbImageBaseURL + g.Cover.ImageID + ".jpg"
    md.CoverArtURL = &url
}
```

- [ ] **Step 3: Build to verify**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/services/igdb/models.go internal/services/igdb/igdb.go
git commit -m "feat: add CoverImageID to GameMetadata"
```

---

## Task 3: Fix `import_item.go` — use JSON arrays for platform IDs/names; use `CoverImageID`

The existing `igdbMetadataToGame` function uses `strings.Join` for platform IDs/names (comma-separated), which is inconsistent with the DB schema comment (`-- JSON array as text`). Fix it to use `json.Marshal`. Also fix the cover art download path to use `md.CoverImageID` directly instead of URL-parsing.

**Files:**
- Modify: `internal/worker/tasks/import_item.go`

- [ ] **Step 1: Fix platform ID/name marshalling in `igdbMetadataToGame`**

Find `igdbMetadataToGame` (around line 430). Replace the platform IDs block:

```go
// Before:
if len(md.PlatformIDs) > 0 {
    ids := make([]string, len(md.PlatformIDs))
    for i, id := range md.PlatformIDs {
        ids[i] = strconv.Itoa(id)
    }
    s := strings.Join(ids, ",")
    game.IgdbPlatformIds = &s
}
if len(md.PlatformNames) > 0 {
    s := strings.Join(md.PlatformNames, ",")
    game.IgdbPlatformNames = &s
}
```

Replace with:

```go
if len(md.PlatformIDs) > 0 {
    b, _ := json.Marshal(md.PlatformIDs)
    s := string(b)
    game.IgdbPlatformIds = &s
}
if len(md.PlatformNames) > 0 {
    b, _ := json.Marshal(md.PlatformNames)
    s := string(b)
    game.IgdbPlatformNames = &s
}
```

Also remove the `strconv` and `strings` imports from the file if they are no longer used elsewhere (check first — `strconv` is used by `igdbExtractImageID`... actually `strings` is used in `igdbExtractImageID` too, so only remove if truly unused).

- [ ] **Step 2: Fix cover art download to use `CoverImageID`**

In `NewImportItemHandler`, find the cover download block (inside the `if !gameExists && igdbClient.Configured()` branch):

```go
// Before:
if md.CoverArtURL != nil {
    imageID := igdbExtractImageID(*md.CoverArtURL)
    if imageID != "" {
        localURL, dlErr := igdbClient.DownloadCoverArt(ctx, imageID, storagePath)
        ...
    }
}
```

Replace with:

```go
if md.CoverImageID != "" {
    localURL, dlErr := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
    if dlErr != nil {
        slog.Warn("import_item: cover art download failed", "igdb_id", gd.IGDBID, "err", dlErr)
    } else {
        game.CoverArtUrl = &localURL
    }
}
```

The `igdbExtractImageID` function is now unused — delete it.

- [ ] **Step 3: Build**

```bash
go build ./...
```
Expected: no errors. If `strconv` is now unused, the compiler will tell you — remove the import.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/worker/tasks/...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_item.go
git commit -m "fix: use JSON arrays for platform IDs/names in import_item; use CoverImageID directly"
```

---

## Task 4: Update `scheduler.go` — add `metadataRefreshInterval`, accept `cfg`

**Files:**
- Modify: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Write the failing test (scheduler compile test)**

There are no unit tests for `scheduler.go` in this codebase — the change is verified by `go build`. Move to Step 2.

- [ ] **Step 2: Update `Scheduler` struct and `NewScheduler`**

Replace the current `Scheduler` struct and `NewScheduler` function:

```go
// Before:
type Scheduler struct {
	db        *bun.DB
	pool      *worker.Pool
	backupSvc *backup.Service
	scheduler gocron.Scheduler
	backupJob gocron.Job
}

func NewScheduler(db *bun.DB, pool *worker.Pool, backupSvc *backup.Service) *Scheduler {
	return &Scheduler{db: db, pool: pool, backupSvc: backupSvc}
}
```

Replace with:

```go
type Scheduler struct {
	db                      *bun.DB
	pool                    *worker.Pool
	backupSvc               *backup.Service
	metadataRefreshInterval time.Duration
	scheduler               gocron.Scheduler
	backupJob               gocron.Job
}

func NewScheduler(db *bun.DB, pool *worker.Pool, backupSvc *backup.Service, cfg *config.Config) *Scheduler {
	interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
	if err != nil {
		slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
			"value", cfg.MetadataRefreshInterval, "err", err)
		interval = 24 * time.Hour
	}
	return &Scheduler{
		db:                      db,
		pool:                    pool,
		backupSvc:               backupSvc,
		metadataRefreshInterval: interval,
	}
}
```

Add the `config` import at the top of the file:
```go
"github.com/drzero42/nexorious-go/internal/config"
```

- [ ] **Step 3: Register metadata refresh dispatch job in `Start`**

In `Start`, after the `CheckPendingSyncs` job registration (around line 70), add:

```go
// Metadata refresh dispatch — configurable interval.
_, _ = s.scheduler.NewJob(
    gocron.DurationJob(s.metadataRefreshInterval),
    gocron.NewTask(func() {
        _ = s.pool.Submit(ctx, "metadata_refresh_dispatch", nil, 1)
    }),
)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```
Expected: compiler error about `NewScheduler` call sites in `main.go` — that's expected and will be fixed in Task 6.

Actually, build the scheduler package only to check it compiles:
```bash
go build ./internal/scheduler/...
```
Expected: no errors (scheduler package itself is fine; main.go will break until Task 6).

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go
git commit -m "feat: scheduler accepts cfg, registers metadata_refresh_dispatch job"
```

---

## Task 5: Implement `metadata_refresh.go` — dispatch and item handlers

**Files:**
- Create: `internal/worker/tasks/metadata_refresh.go`

- [ ] **Step 1: Write the failing tests** (see Task 6 for test file — write tests first, then implement)

Actually, tests go in Task 6. Implement the handlers here; the tests in Task 6 will validate them.

- [ ] **Step 2: Create `internal/worker/tasks/metadata_refresh.go`**

```go
package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
	igdbsvc "github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// ─── Dispatch handler ────────────────────────────────────────────────────────

// NewMetadataRefreshDispatchHandler returns a task handler that:
//  1. Guards on IGDB configured and admin user present.
//  2. Checks no active metadata_refresh job.
//  3. Selects all games ordered by last_updated ASC.
//  4. Creates job + job_items + pending_tasks in a single transaction.
func NewMetadataRefreshDispatchHandler(
	db *bun.DB,
	pool *worker.Pool,
	igdbClient *igdbsvc.Client,
) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		// Step 1 — IGDB guard.
		if !igdbClient.Configured() {
			slog.Warn("metadata_refresh_dispatch: IGDB not configured, skipping")
			return nil
		}

		// Step 2 — Find admin user.
		var adminID string
		err := db.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &adminID)
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("metadata_refresh_dispatch: no admin user found, skipping")
			return nil
		}
		if err != nil {
			slog.Error("metadata_refresh_dispatch: query admin user", "err", err)
			return nil
		}

		// Step 3 — Duplicate-run guard.
		var existingJobID string
		err = db.NewRaw(
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

		// Step 4 — Select games ordered by last_updated ASC.
		var games []struct {
			ID    int32  `bun:"id"`
			Title string `bun:"title"`
		}
		err = db.NewRaw(`SELECT id, title FROM games ORDER BY last_updated ASC`).Scan(ctx, &games)
		if err != nil {
			slog.Error("metadata_refresh_dispatch: query games", "err", err)
			return nil
		}
		if len(games) == 0 {
			return nil
		}

		// Step 5 — Create job, items, and tasks in a single transaction.
		jobID := uuid.NewString()
		if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// 5a — Insert job.
			_, err := tx.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
				 VALUES (?, ?, ?, ?, 'pending', 'low', ?, now())`,
				jobID, adminID, models.JobTypeMetadataRefresh, models.JobSourceSystem, len(games),
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("insert job: %w", err)
			}

			// 5b — Insert job_items and pending_tasks.
			for _, g := range games {
				itemID := uuid.NewString()

				sourceMeta, _ := json.Marshal(map[string]any{"game_id": g.ID})

				_, err = tx.NewRaw(
					`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
					 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
					itemID, jobID, adminID, strconv.Itoa(int(g.ID)), g.Title, sourceMeta,
				).Exec(ctx)
				if err != nil {
					return fmt.Errorf("insert job_item for game %d: %w", g.ID, err)
				}

				payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
				_, err = tx.NewRaw(
					`INSERT INTO pending_tasks (id, task_type, payload, priority, status, attempts, created_at)
					 VALUES (?, 'metadata_refresh_item', ?, 1, 'pending', 0, now())`,
					uuid.NewString(), payload,
				).Exec(ctx)
				if err != nil {
					return fmt.Errorf("insert pending_task for game %d: %w", g.ID, err)
				}
			}

			// 5c — Mark job processing.
			_, err = tx.NewRaw(
				`UPDATE jobs SET status = 'processing', started_at = now() WHERE id = ?`,
				jobID,
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("update job to processing: %w", err)
			}

			return nil
		}); err != nil {
			slog.Error("metadata_refresh_dispatch: transaction failed", "err", err)
			return nil
		}

		slog.Info("metadata_refresh_dispatch: job created", "job_id", jobID, "game_count", len(games))
		return nil
	}
}

// ─── Item handler ─────────────────────────────────────────────────────────────

type metadataRefreshItemPayload struct {
	JobItemID string `json:"job_item_id"`
}

type metadataRefreshSourceMeta struct {
	GameID int32 `json:"game_id"`
}

// NewMetadataRefreshItemHandler returns a task handler that fetches fresh IGDB
// metadata for a single game and updates the games row.
func NewMetadataRefreshItemHandler(
	db *bun.DB,
	igdbClient *igdbsvc.Client,
	storagePath string,
) func(ctx context.Context, task *models.PendingTask) error {
	return func(ctx context.Context, task *models.PendingTask) error {
		// Step 1 — Parse payload.
		var payload metadataRefreshItemPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			slog.Error("metadata_refresh_item: unmarshal payload", "err", err)
			return nil
		}

		// Step 2 — Load job_item.
		var item models.JobItem
		if err := db.NewSelect().Model(&item).Where("id = ?", payload.JobItemID).Scan(ctx); err != nil {
			slog.Error("metadata_refresh_item: load job_item", "id", payload.JobItemID, "err", err)
			return nil
		}

		// Step 3 — Parse source_metadata.
		var sourceMeta metadataRefreshSourceMeta
		if err := json.Unmarshal(item.SourceMetadata, &sourceMeta); err != nil {
			metaRefreshMarkItemFailed(ctx, db, &item, fmt.Sprintf("parse source_metadata: %v", err))
			metaRefreshCheckJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// Step 4 — Load game.
		var game struct {
			ID          int32   `bun:"id"`
			Title       string  `bun:"title"`
			CoverArtUrl *string `bun:"cover_art_url"`
		}
		if err := db.NewRaw(
			`SELECT id, title, cover_art_url FROM games WHERE id = ?`, sourceMeta.GameID,
		).Scan(ctx, &game); err != nil {
			metaRefreshMarkItemFailed(ctx, db, &item, fmt.Sprintf("load game: %v", err))
			metaRefreshCheckJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// Step 5 — IGDB guard (defensive).
		if !igdbClient.Configured() {
			metaRefreshMarkItemFailed(ctx, db, &item, "igdb_not_configured")
			metaRefreshCheckJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// Step 6 — Fetch metadata.
		md, err := igdbClient.FetchFullMetadata(ctx, int(sourceMeta.GameID))
		if err != nil {
			metaRefreshMarkItemFailed(ctx, db, &item, err.Error())
			metaRefreshCheckJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// Step 7 — Update games row.
		var releaseDate *time.Time
		if md.ReleaseDate != nil {
			if t, err := time.Parse("2006-01-02", *md.ReleaseDate); err == nil {
				releaseDate = &t
			}
		}

		var igdbSlug *string
		if md.IgdbSlug != "" {
			igdbSlug = &md.IgdbSlug
		}

		var igdbPlatformIds *string
		if len(md.PlatformIDs) > 0 {
			b, _ := json.Marshal(md.PlatformIDs)
			s := string(b)
			igdbPlatformIds = &s
		}

		var igdbPlatformNames *string
		if len(md.PlatformNames) > 0 {
			b, _ := json.Marshal(md.PlatformNames)
			s := string(b)
			igdbPlatformNames = &s
		}

		_, err = db.NewRaw(
			`UPDATE games SET
				title = ?,
				description = ?,
				genre = ?,
				developer = ?,
				publisher = ?,
				release_date = ?,
				rating_average = ?,
				rating_count = ?,
				howlongtobeat_main = ?,
				howlongtobeat_extra = ?,
				howlongtobeat_completionist = ?,
				igdb_slug = ?,
				igdb_platform_ids = ?,
				igdb_platform_names = ?,
				game_modes = ?,
				themes = ?,
				player_perspectives = ?,
				last_updated = now()
			WHERE id = ?`,
			md.Title,
			md.Description,
			md.Genre,
			md.Developer,
			md.Publisher,
			releaseDate,
			md.RatingAverage,
			md.RatingCount,
			md.HowlongtobeatMain,
			md.HowlongtobeatExtra,
			md.HowlongtobeatCompletionist,
			igdbSlug,
			igdbPlatformIds,
			igdbPlatformNames,
			md.GameModes,
			md.Themes,
			md.PlayerPerspectives,
			sourceMeta.GameID,
		).Exec(ctx)
		if err != nil {
			metaRefreshMarkItemFailed(ctx, db, &item, fmt.Sprintf("update games: %v", err))
			metaRefreshCheckJobCompletion(ctx, db, item.JobID)
			return nil
		}

		// Step 8 — Cover art (non-fatal).
		if md.CoverImageID != "" {
			expectedURLPath := "/static/cover_art/" + md.CoverImageID + ".jpg"
			if game.CoverArtUrl == nil || *game.CoverArtUrl != expectedURLPath {
				coverURLPath, err := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
				if err != nil {
					slog.Warn("metadata_refresh_item: cover art download failed",
						"game_id", game.ID, "image_id", md.CoverImageID, "err", err)
				} else if coverURLPath != "" {
					_, _ = db.NewRaw(
						`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
					).Exec(ctx)
				}
			}
		}

		// Step 9 — Mark item completed.
		metaRefreshMarkItemCompleted(ctx, db, &item)

		// Step 10 — Check job completion.
		metaRefreshCheckJobCompletion(ctx, db, item.JobID)

		return nil
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func metaRefreshMarkItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("metadata_refresh: markItemFailed", "id", item.ID, "err", err)
	}
}

func metaRefreshMarkItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusCompleted
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("metadata_refresh: markItemCompleted", "id", item.ID, "err", err)
	}
}

func metaRefreshCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	var pendingCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status NOT IN ('completed', 'failed', 'skipped')`,
		jobID,
	).Scan(ctx, &pendingCount); err != nil {
		slog.Error("metadata_refresh: check job completion", "job_id", jobID, "err", err)
		return
	}
	if pendingCount > 0 {
		return
	}
	now := time.Now().UTC()
	_, err := db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ?`,
		now, jobID,
	).Exec(ctx)
	if err != nil {
		slog.Error("metadata_refresh: update job completed", "job_id", jobID, "err", err)
	}
}
```

- [ ] **Step 3: Build**

```bash
go build ./internal/worker/tasks/...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go
git commit -m "feat: add metadata_refresh_dispatch and metadata_refresh_item worker tasks"
```

---

## Task 6: Wire handlers into `main.go`

**Files:**
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Register worker handlers in the initial pool block**

Find the pool registration block (around line 222–226) and add after `process_sync_item`:

```go
pool.Register("metadata_refresh_dispatch",
    tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient))
pool.Register("metadata_refresh_item",
    tasks.NewMetadataRefreshItemHandler(db, igdbClient, cfg.StoragePath))
```

Also remove the commented-out line:
```go
// pool.Register("metadata_refresh_process", metadataHandler)
```

- [ ] **Step 2: Register worker handlers in `RebuildServices` callback**

Find the `RebuildServices` closure (around line 252–263) and add the same two registrations after `process_sync_item`:

```go
newPool.Register("metadata_refresh_dispatch",
    tasks.NewMetadataRefreshDispatchHandler(newDB, newPool, igdbClient))
newPool.Register("metadata_refresh_item",
    tasks.NewMetadataRefreshItemHandler(newDB, igdbClient, cfg.StoragePath))
```

- [ ] **Step 3: Fix both `scheduler.NewScheduler` call sites to pass `cfg`**

There are two call sites. The first is in `RebuildServices` (around line 265):

```go
// Before:
newSched := scheduler.NewScheduler(newDB, newPool, backupSvc)
```
```go
// After:
newSched := scheduler.NewScheduler(newDB, newPool, backupSvc, cfg)
```

The second is the initial scheduler start (search for `scheduler.NewScheduler` in main.go — it may be a bit further down):

```go
// Before:
sched = scheduler.NewScheduler(db, pool, backupSvc)
```
```go
// After:
sched = scheduler.NewScheduler(db, pool, backupSvc, cfg)
```

- [ ] **Step 4: Build the full binary**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "feat: register metadata_refresh handlers; pass cfg to NewScheduler"
```

---

## Task 7: Write tests for `metadata_refresh.go`

**Files:**
- Create: `internal/worker/tasks/metadata_refresh_test.go`

The test pattern follows `internal/api/auth_test.go`: spin up a real PostgreSQL container, run migrations with `bun/migrate`, and stub IGDB with an `httptest.Server`.

- [ ] **Step 1: Create the test file**

```go
package tasks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/db/models"
	igdbsvc "github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/internal/worker"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// ─── DB helpers ──────────────────────────────────────────────────────────────

func setupMetaRefreshDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func insertMetaRefreshAdminUser(t *testing.T, db *bun.DB) string {
	t.Helper()
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 4)
	id := uuid.NewString()
	_, err := db.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_active, is_admin, preferences, created_at, updated_at)
		 VALUES (?, 'admin', ?, true, true, '{}', now(), now())`, id, string(hash),
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert admin user: %v", err)
	}
	return id
}

func insertTestGame(t *testing.T, db *bun.DB, igdbID int32, title string, lastUpdated time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, ?, now())`,
		igdbID, title, lastUpdated,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert game %d: %v", igdbID, err)
	}
}

// igdbTestServer returns an httptest.Server that handles Twitch token + IGDB games requests.
// gamesResponse is the JSON array to return for /games queries.
func igdbTestServer(t *testing.T, gamesResponse string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(gamesResponse))
		case "/game_time_to_beats":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTestIGDBClient(t *testing.T, srv *httptest.Server) *igdbsvc.Client {
	t.Helper()
	cfg := &config.Config{
		IGDBClientID:          "test-client",
		IGDBClientSecret:      "test-secret",
		IGDBRequestsPerSecond: 100,
		IGDBBurstCapacity:     100,
	}
	client := igdbsvc.NewClientWithTokenURL(cfg, srv.URL+"/oauth2/token")
	// Point the API URL at the test server.
	// NewClientWithTokenURL sets tokenURL; we also need to override apiURL.
	// Use the exported SetAPIURLForTest if available, or just create client normally.
	// The test server handles /games at the same host.
	return client
}

// ─── Dispatch tests ───────────────────────────────────────────────────────────

func TestMetadataRefreshDispatch_IGDBNotConfigured(t *testing.T) {
	db := setupMetaRefreshDB(t)
	insertMetaRefreshAdminUser(t, db)

	pool := worker.NewPool(db)
	unconfigured := igdbsvc.NewClient(&config.Config{}) // no credentials

	handler := tasks.NewMetadataRefreshDispatchHandler(db, pool, unconfigured)
	if err := handler(context.Background(), &models.PendingTask{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
	}
}

func TestMetadataRefreshDispatch_NoAdminUser(t *testing.T) {
	db := setupMetaRefreshDB(t)
	// No admin user inserted.

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	pool := worker.NewPool(db)
	handler := tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient)
	if err := handler(context.Background(), &models.PendingTask{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
	}
}

func TestMetadataRefreshDispatch_AlreadyRunning(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)
	insertTestGame(t, db, 1001, "Game One", time.Now().Add(-24*time.Hour))

	ctx := context.Background()
	// Pre-insert a processing job.
	_, _ = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now())`,
		uuid.NewString(), adminID,
	).Exec(ctx)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	pool := worker.NewPool(db)
	handler := tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient)
	if err := handler(ctx, &models.PendingTask{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(ctx, &count)
	if count != 1 {
		t.Errorf("expected 1 job (the existing one), got %d", count)
	}
}

func TestMetadataRefreshDispatch_NoGames(t *testing.T) {
	db := setupMetaRefreshDB(t)
	insertMetaRefreshAdminUser(t, db)

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	pool := worker.NewPool(db)
	handler := tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient)
	if err := handler(context.Background(), &models.PendingTask{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var count int
	_ = db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE job_type = 'metadata_refresh'`).Scan(context.Background(), &count)
	if count != 0 {
		t.Errorf("expected 0 jobs, got %d", count)
	}
}

func TestMetadataRefreshDispatch_CreatesJobAndItems(t *testing.T) {
	db := setupMetaRefreshDB(t)
	insertMetaRefreshAdminUser(t, db)
	now := time.Now().UTC()
	insertTestGame(t, db, 1001, "Game One", now.Add(-72*time.Hour))
	insertTestGame(t, db, 1002, "Game Two", now.Add(-48*time.Hour))
	insertTestGame(t, db, 1003, "Game Three", now.Add(-24*time.Hour))

	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	pool := worker.NewPool(db)
	handler := tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient)
	if err := handler(context.Background(), &models.PendingTask{}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()

	// Exactly 1 job in processing state.
	var job models.Job
	if err := db.NewSelect().Model(&job).
		Where("job_type = ?", models.JobTypeMetadataRefresh).
		Scan(ctx); err != nil {
		t.Fatalf("no job found: %v", err)
	}
	if job.Status != models.JobStatusProcessing {
		t.Errorf("job status: want processing, got %s", job.Status)
	}
	if job.TotalItems != 3 {
		t.Errorf("total_items: want 3, got %d", job.TotalItems)
	}

	// 3 job_items.
	var itemCount int
	_ = db.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, job.ID).Scan(ctx, &itemCount)
	if itemCount != 3 {
		t.Errorf("job_items: want 3, got %d", itemCount)
	}

	// 3 pending_tasks with task_type = 'metadata_refresh_item'.
	var taskCount int
	_ = db.NewRaw(`SELECT COUNT(*) FROM pending_tasks WHERE task_type = 'metadata_refresh_item'`).Scan(ctx, &taskCount)
	if taskCount != 3 {
		t.Errorf("pending_tasks: want 3, got %d", taskCount)
	}
}

// ─── Item tests ───────────────────────────────────────────────────────────────

// setupItemTest creates a job + job_item for one game and returns the item ID.
func setupItemTest(t *testing.T, db *bun.DB, adminID string, gameID int32, title string) string {
	t.Helper()
	ctx := context.Background()

	jobID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 1, now(), now())`,
		jobID, adminID,
	).Exec(ctx)

	itemID := uuid.NewString()
	sourceMeta, _ := json.Marshal(map[string]any{"game_id": gameID})
	_, _ = db.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, adminID, strconv.Itoa(int(gameID)), title, sourceMeta,
	).Exec(ctx)

	return itemID
}

func TestMetadataRefreshItem_Success(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)
	insertTestGame(t, db, 2001, "Old Title", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, db, adminID, 2001, "Old Title")

	gamesJSON := `[{"id":2001,"name":"New Title","slug":"new-title","cover":{"image_id":"co9999"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	task := &models.PendingTask{Payload: payload}

	handler := tasks.NewMetadataRefreshItemHandler(db, igdbClient, t.TempDir())
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()

	// Game title should be updated.
	var title string
	_ = db.NewRaw(`SELECT title FROM games WHERE id = 2001`).Scan(ctx, &title)
	if title != "New Title" {
		t.Errorf("game title: want 'New Title', got %q", title)
	}

	// Item should be completed.
	var item models.JobItem
	_ = db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}

	// Job should be completed (only 1 item).
	var job models.Job
	_ = db.NewSelect().Model(&job).Where("job_id = (SELECT job_id FROM job_items WHERE id = ?)", itemID).Scan(ctx)
	// Re-fetch job by item's job_id.
	var jobID string
	_ = db.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	var jobStatus string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("job status: want completed, got %s", jobStatus)
	}
}

func TestMetadataRefreshItem_IGDBError(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)
	insertTestGame(t, db, 2002, "Some Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, db, adminID, 2002, "Some Game")

	// IGDB returns empty array → ErrGameNotFound.
	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	handler := tasks.NewMetadataRefreshItemHandler(db, igdbClient, t.TempDir())
	_ = handler(context.Background(), &models.PendingTask{Payload: payload})

	ctx := context.Background()

	var item models.JobItem
	_ = db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusFailed {
		t.Errorf("item status: want failed, got %s", item.Status)
	}

	var jobID string
	_ = db.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	var jobStatus string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("job status: want completed, got %s", jobStatus)
	}
}

func TestMetadataRefreshItem_CoverArtFailureNonFatal(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)
	insertTestGame(t, db, 2003, "Cover Game", time.Now().Add(-24*time.Hour))
	itemID := setupItemTest(t, db, adminID, 2003, "Cover Game")

	// Return a cover image_id but DownloadCoverArt will fail because the URL
	// won't exist (test server doesn't serve cover images from CDN).
	gamesJSON := `[{"id":2003,"name":"Cover Game","slug":"cover-game","cover":{"image_id":"co_fail"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	// Use /dev/null as storage path so file creation will fail.
	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	handler := tasks.NewMetadataRefreshItemHandler(db, igdbClient, "/dev/null")
	_ = handler(context.Background(), &models.PendingTask{Payload: payload})

	ctx := context.Background()
	var item models.JobItem
	_ = db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	// Item should still be completed despite cover art failure.
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}
}

func TestMetadataRefreshItem_CoverArtUnchanged(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)

	// Game already has the correct cover_art_url.
	ctx := context.Background()
	_, _ = db.NewRaw(
		`INSERT INTO games (id, title, cover_art_url, last_updated, created_at)
		 VALUES (2004, 'Cover Unchanged', '/static/cover_art/co_same.jpg', now() - interval '1 day', now())`,
	).Exec(ctx)
	itemID := setupItemTest(t, db, adminID, 2004, "Cover Unchanged")

	gamesJSON := `[{"id":2004,"name":"Cover Unchanged","slug":"cover-unchanged","cover":{"image_id":"co_same"}}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	payload, _ := json.Marshal(map[string]string{"job_item_id": itemID})
	handler := tasks.NewMetadataRefreshItemHandler(db, igdbClient, t.TempDir())
	_ = handler(context.Background(), &models.PendingTask{Payload: payload})

	// Item should complete.
	var item models.JobItem
	_ = db.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx)
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("item status: want completed, got %s", item.Status)
	}
}

func TestMetadataRefreshItem_JobCompletionPartial(t *testing.T) {
	db := setupMetaRefreshDB(t)
	adminID := insertMetaRefreshAdminUser(t, db)
	insertTestGame(t, db, 3001, "Game A", time.Now().Add(-48*time.Hour))
	insertTestGame(t, db, 3002, "Game B", time.Now().Add(-24*time.Hour))

	ctx := context.Background()

	// Create a job with 2 items manually.
	jobID := uuid.NewString()
	_, _ = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at, started_at)
		 VALUES (?, ?, 'metadata_refresh', 'system', 'processing', 'low', 2, now(), now())`,
		jobID, adminID,
	).Exec(ctx)

	makeItem := func(gameID int32, title string) string {
		itemID := uuid.NewString()
		sourceMeta, _ := json.Marshal(map[string]any{"game_id": gameID})
		_, _ = db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, adminID, strconv.Itoa(int(gameID)), title, sourceMeta,
		).Exec(ctx)
		return itemID
	}

	itemID1 := makeItem(3001, "Game A")
	itemID2 := makeItem(3002, "Game B")

	gamesResponse := func(id int, name string) string {
		return `[{"id":` + strconv.Itoa(id) + `,"name":"` + name + `","slug":"slug"}]`
	}

	srv := igdbTestServer(t, `[]`) // will be overridden per call
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	handler := tasks.NewMetadataRefreshItemHandler(db, igdbClient, t.TempDir())

	// Process first item — job should still be processing.
	// Re-point test server to return game 3001.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = w.Write([]byte(gamesResponse(3001, "Game A")))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	payload1, _ := json.Marshal(map[string]string{"job_item_id": itemID1})
	_ = handler(ctx, &models.PendingTask{Payload: payload1})

	var jobStatus string
	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusProcessing {
		t.Errorf("after first item: job status want processing, got %s", jobStatus)
	}

	// Process second item — job should now be completed.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600,"token_type":"bearer"}`))
		case "/games":
			_, _ = w.Write([]byte(gamesResponse(3002, "Game B")))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	payload2, _ := json.Marshal(map[string]string{"job_item_id": itemID2})
	_ = handler(ctx, &models.PendingTask{Payload: payload2})

	_ = db.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != models.JobStatusCompleted {
		t.Errorf("after second item: job status want completed, got %s", jobStatus)
	}
}
```

Note: The test file uses `strconv` — add `"strconv"` to imports.

- [ ] **Step 2: Run failing tests first**

```bash
go test ./internal/worker/tasks/... -run TestMetadataRefreshDispatch -v
```
Expected: tests compile and most should pass (the DB/transaction logic is straightforward). Some may reveal minor issues — fix them.

- [ ] **Step 3: Run all metadata refresh tests**

```bash
go test ./internal/worker/tasks/... -run TestMetadataRefresh -v -timeout 300s
```
Expected: all 10 tests pass.

> **Note on `newTestIGDBClient`:** The `igdb.NewClientWithTokenURL` sets the token URL but the API URL still points to `api.igdb.com`. You'll need a way to override `apiURL` for tests. Check if `igdb.Client` has a test-accessible field or add an exported `SetAPIURLForTest(url string)` method to `igdb.go` if not. If the cover download test uses a real HTTP call to `images.igdb.com`, use `/dev/null` storage to force failure (already done above).
>
> If `Client.apiURL` is unexported and there's no setter, add one to `internal/services/igdb/igdb.go`:
> ```go
> // SetAPIURLForTest overrides the IGDB API URL. For use in tests only.
> func (c *Client) SetAPIURLForTest(url string) {
>     c.apiURL = url
> }
> ```
> Then call `igdbClient.SetAPIURLForTest(srv.URL)` in `newTestIGDBClient`.

- [ ] **Step 4: Run all Go tests**

```bash
go test -timeout 300s ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/metadata_refresh_test.go
# If SetAPIURLForTest was added:
git add internal/services/igdb/igdb.go
git commit -m "test: add metadata_refresh dispatch and item handler tests"
```

---

## Task 8: Slumber collection note + final verification

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add comment in slumber.yaml**

Find the `jobs/` folder section in `slumber.yaml` and add a comment noting that metadata refresh has no HTTP trigger:

```yaml
# jobs/ — job and job_item management endpoints
# Note: metadata_refresh has no HTTP trigger endpoint (scheduler-only).
#       To trigger manually, POST a pending_task directly to the DB or
#       wait for the METADATA_REFRESH_INTERVAL scheduler tick.
```

- [ ] **Step 2: Verify slumber collection loads**

```bash
slumber collection
```
Expected: no errors.

- [ ] **Step 3: Run full test suite**

```bash
go test -timeout 300s ./...
cd ui/frontend && npm run check && npm run test
```
Expected: all pass, no TypeScript errors.

- [ ] **Step 4: Final commit**

```bash
git add slumber.yaml
git commit -m "docs: note metadata_refresh is scheduler-only in slumber.yaml"
```

---

## Self-Review Checklist

- [x] **`CoverImageID` field added** — Task 2
- [x] **`convertToGameMetadata` populates `CoverImageID`** — Task 2
- [x] **`EstimatedPlaytimeHours` removed from all locations** — Task 1 (migration, Go model, TS types, TS tests)
- [x] **`import_item.go` uses JSON arrays for platform IDs/names** — Task 3
- [x] **`import_item.go` cover art uses `CoverImageID` directly** — Task 3
- [x] **`Scheduler` struct gains `metadataRefreshInterval`** — Task 4
- [x] **`NewScheduler` accepts `cfg *config.Config`** — Task 4
- [x] **Dispatch job registered in `Start`** — Task 4
- [x] **`metadata_refresh_dispatch` handler** — Task 5 (IGDB guard, admin guard, duplicate guard, game selection, tx)
- [x] **`metadata_refresh_item` handler** — Task 5 (all 10 columns, cover art idempotent skip, non-fatal failure)
- [x] **Helper functions `metaRefreshMark*` and `metaRefreshCheckJobCompletion`** — Task 5
- [x] **Both `NewScheduler` call sites in `main.go` updated** — Task 6
- [x] **Both handler `pool.Register` call sites updated (initial + `RebuildServices`)** — Task 6
- [x] **All 10 tests** — Task 7
- [x] **Slumber note** — Task 8
- [x] **Spec type consistency**: `metadataRefreshItemPayload`, `metadataRefreshSourceMeta`, helper names all consistent across Tasks 5 and 7
