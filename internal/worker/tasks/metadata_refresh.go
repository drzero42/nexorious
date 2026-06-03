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

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
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

	// Step 5 — Create job and items in a transaction, then enqueue River jobs after commit.
	// River jobs must be inserted AFTER the transaction commits: riverClient.Insert uses a
	// separate connection and commits immediately, so workers can dequeue and attempt to load
	// job_items before the bun transaction is visible — causing "no rows" errors.
	jobID := uuid.NewString()
	itemIDs := make([]string, 0, len(games))
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// 5a — Insert job.
		_, err := tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
			jobID, adminID, models.JobTypeMetadataRefresh, models.JobSourceSystem, len(games),
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("insert job: %w", err)
		}

		// 5b — Insert job_items only; River jobs are enqueued after commit.
		for _, g := range games {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)

			sourceMeta, _ := json.Marshal(map[string]any{"game_id": g.ID}) //nolint:errcheck // marshaling a fixed map cannot fail
			sourceMetaRaw := json.RawMessage(sourceMeta)

			_, err = tx.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
				itemID, jobID, adminID, strconv.Itoa(int(g.ID)), g.Title, sourceMetaRaw,
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("insert job_item for game %d: %w", g.ID, err)
			}
		}

		return nil
	}); err != nil {
		slog.Error("metadata_refresh_dispatch: transaction failed", "err", err)
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: map[string]any{"action": "metadata_refresh_dispatch", "error": err.Error()},
		})
		return nil
	}

	// Step 6 — Enqueue River jobs now that job_items are committed and visible.
	for _, itemID := range itemIDs {
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, MetadataRefreshItemArgs{JobItemID: itemID}); err != nil {
			slog.Error("metadata_refresh_dispatch: enqueue item failed", "err", err, "job_id", jobID, "item_id", itemID)
		}
	}

	slog.Info("metadata_refresh_dispatch: job created", "job_id", jobID, "game_count", len(games))
	notify.Emit(ctx, w.DB, notify.EmitParams{
		Type: notify.TypeAdminMaintCompleted, Scope: notify.ScopeAdmin,
		Payload: map[string]any{"action": "metadata_refresh_dispatch", "count": len(games)},
	})
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
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err), "metadata_refresh: markItemFailed")
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 3 — IGDB guard (defensive; preserves the per-item failure message).
	if !w.IGDBClient.Configured() {
		markItemFailed(ctx, w.DB, &item, "igdb_not_configured", "metadata_refresh: markItemFailed")
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 4 — Fetch IGDB metadata and update the games row (shared helper).
	if err := fetchAndStoreMetadata(ctx, w.DB, w.IGDBClient, w.StoragePath, sourceMeta.GameID); err != nil {
		markItemFailed(ctx, w.DB, &item, err.Error(), "metadata_refresh: markItemFailed")
		metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Step 5 — Mark item completed.
	markItemCompleted(ctx, w.DB, &item, "metadata_refresh: markItemCompleted")

	// Step 6 — Check job completion.
	metaRefreshCheckJobCompletion(ctx, w.DB, item.JobID)

	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// fetchAndStoreMetadata fetches fresh IGDB metadata for a single game and writes
// it to the games row, including cover art (cover-art failure is non-fatal). It
// is the shared core used by both MetadataRefreshItemWorker (which layers
// job_items tracking on top) and the fire-and-forget MetadataFetchWorker. The
// caller must verify IGDB is configured before calling.
func fetchAndStoreMetadata(ctx context.Context, db *bun.DB, igdbClient *igdbsvc.Client, storagePath string, gameID int32) error {
	// Load the current games row; cover_art_url drives the cover re-download decision.
	var game struct {
		ID          int32   `bun:"id"`
		Title       string  `bun:"title"`
		CoverArtUrl *string `bun:"cover_art_url"`
	}
	if err := db.NewRaw(
		`SELECT id, title, cover_art_url FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &game); err != nil {
		return fmt.Errorf("load game: %w", err)
	}

	md, err := igdbClient.FetchFullMetadata(ctx, int(gameID))
	if err != nil {
		return err
	}

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
		b, _ := json.Marshal(md.PlatformIDs) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		igdbPlatformIds = &s
	}

	var igdbPlatformNames *string
	if len(md.PlatformNames) > 0 {
		b, _ := json.Marshal(md.PlatformNames) //nolint:errcheck // marshaling a fixed slice cannot fail
		s := string(b)
		igdbPlatformNames = &s
	}

	if _, err := db.NewRaw(
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
		gameID,
	).Exec(ctx); err != nil {
		return fmt.Errorf("update games: %w", err)
	}

	// Cover art (non-fatal).
	if md.CoverImageID != "" {
		expectedURLPath := "/static/cover_art/" + md.CoverImageID + ".jpg"
		if game.CoverArtUrl == nil || *game.CoverArtUrl != expectedURLPath {
			coverURLPath, err := igdbClient.DownloadCoverArt(ctx, md.CoverImageID, storagePath)
			if err != nil {
				slog.Warn("metadata fetch: cover art download failed",
					"game_id", game.ID, "image_id", md.CoverImageID, "err", err)
			} else if coverURLPath != "" {
				if _, err := db.NewRaw(
					`UPDATE games SET cover_art_url = ? WHERE id = ?`, coverURLPath, game.ID,
				).Exec(ctx); err != nil {
					slog.Error("metadata fetch: update cover_art_url failed", "err", err, "game_id", game.ID)
				}
			}
		}
	}

	return nil
}

func metaRefreshCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	remaining, ok := countJobItems(ctx, db, jobID, "status NOT IN ('completed', 'failed', 'skipped')", "metadata_refresh: check job completion")
	if !ok || remaining > 0 {
		return
	}
	finalizeJobCompleted(ctx, db, jobID, "metadata_refresh: update job completed", false)
}
