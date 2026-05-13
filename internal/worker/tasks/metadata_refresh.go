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
				sourceMetaRaw := json.RawMessage(sourceMeta)

				_, err = tx.NewRaw(
					`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
					 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
					itemID, jobID, adminID, strconv.Itoa(int(g.ID)), g.Title, sourceMetaRaw,
				).Exec(ctx)
				if err != nil {
					return fmt.Errorf("insert job_item for game %d: %w", g.ID, err)
				}

				payloadBytes, _ := json.Marshal(map[string]string{"job_item_id": itemID})
				payload := json.RawMessage(payloadBytes)
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
