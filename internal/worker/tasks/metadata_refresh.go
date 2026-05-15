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
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
	igdbsvc "github.com/drzero42/nexorious-go/internal/services/igdb"
)

// ─── Dispatch worker ─────────────────────────────────────────────────────────

// MetadataRefreshDispatchArgs is the River job args type for "metadata_refresh_dispatch".
type MetadataRefreshDispatchArgs struct{}

func (MetadataRefreshDispatchArgs) Kind() string { return "metadata_refresh_dispatch" }

func (MetadataRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

// MetadataRefreshDispatchWorker is a River worker that:
//  1. Guards on IGDB configured and admin user present.
//  2. Checks no active metadata_refresh job.
//  3. Selects all games ordered by last_updated ASC.
//  4. Creates job + job_items and enqueues metadata_refresh_item River jobs in a single transaction.
type MetadataRefreshDispatchWorker struct {
	river.WorkerDefaults[MetadataRefreshDispatchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *MetadataRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshDispatchArgs]) error {
	// Step 1 — IGDB guard.
	if !w.IGDBClient.Configured() {
		slog.Warn("metadata_refresh_dispatch: IGDB not configured, skipping")
		return nil
	}

	// Step 2 — Find admin user.
	var adminID string
	err := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &adminID)
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

	// Step 4 — Select games ordered by last_updated ASC.
	var games []struct {
		ID    int32  `bun:"id"`
		Title string `bun:"title"`
	}
	err = w.DB.NewRaw(`SELECT id, title FROM games ORDER BY last_updated ASC`).Scan(ctx, &games)
	if err != nil {
		slog.Error("metadata_refresh_dispatch: query games", "err", err)
		return nil
	}
	if len(games) == 0 {
		return nil
	}

	// Step 5 — Create job, items, and enqueue River jobs in a single transaction.
	jobID := uuid.NewString()
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// 5a — Insert job.
		_, err := tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'pending', 'low', ?, now())`,
			jobID, adminID, models.JobTypeMetadataRefresh, models.JobSourceSystem, len(games),
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("insert job: %w", err)
		}

		// 5b — Insert job_items and enqueue River jobs.
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

			_, _ = w.RiverClient.Insert(ctx, MetadataRefreshItemArgs{JobItemID: itemID}, nil)
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

// ─── Item worker ──────────────────────────────────────────────────────────────

// MetadataRefreshItemArgs is the River job args type for "metadata_refresh_item".
type MetadataRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (MetadataRefreshItemArgs) Kind() string { return "metadata_refresh_item" }

func (MetadataRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

// MetadataRefreshItemWorker is a River worker that fetches fresh IGDB
// metadata for a single game and updates the games row.
type MetadataRefreshItemWorker struct {
	river.WorkerDefaults[MetadataRefreshItemArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	StoragePath string
}

type metadataRefreshSourceMeta struct {
	GameID int32 `json:"game_id"`
}

func (w *MetadataRefreshItemWorker) Work(ctx context.Context, job *river.Job[MetadataRefreshItemArgs]) error {
	jobItemID := job.Args.JobItemID

	// Step 1 — Load job_item.
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", jobItemID).Scan(ctx); err != nil {
		slog.Error("metadata_refresh_item: load job_item", "id", jobItemID, "err", err)
		return nil
	}

	// Step 2 — Parse source_metadata.
	var sourceMeta metadataRefreshSourceMeta
	if err := json.Unmarshal(item.SourceMetadata, &sourceMeta); err != nil {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 3 — Load game.
	var game struct {
		ID          int32   `bun:"id"`
		Title       string  `bun:"title"`
		CoverArtUrl *string `bun:"cover_art_url"`
	}
	if err := w.DB.NewRaw(
		`SELECT id, title, cover_art_url FROM games WHERE id = ?`, sourceMeta.GameID,
	).Scan(ctx, &game); err != nil {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("load game: %v", err))
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 4 — IGDB guard (defensive).
	if !w.IGDBClient.Configured() {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, "igdb_not_configured")
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 5 — Fetch metadata.
	md, err := w.IGDBClient.FetchFullMetadata(ctx, int(sourceMeta.GameID))
	if err != nil {
		metaRefreshMarkItemFailed(ctx, w.DB, &item, err.Error())
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 6 — Update games row.
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

	_, err = w.DB.NewRaw(
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
		metaRefreshMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("update games: %v", err))
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 7 — Cover art (non-fatal).
	if md.CoverImageID != "" {
		expectedURLPath := "/static/cover_art/" + md.CoverImageID + ".jpg"
		if game.CoverArtUrl == nil || *game.CoverArtUrl != expectedURLPath {
			coverURLPath, err := w.IGDBClient.DownloadCoverArt(ctx, md.CoverImageID, w.StoragePath)
			if err != nil {
				slog.Warn("metadata_refresh_item: cover art download failed",
					"game_id", game.ID, "image_id", md.CoverImageID, "err", err)
			} else if coverURLPath != "" {
				_, _ = w.DB.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx)
			}
		}
	}

	// Step 8 — Mark item completed.
	metaRefreshMarkItemCompleted(ctx, w.DB, &item)

	// Step 9 — Check job completion.
	metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)

	return nil
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
