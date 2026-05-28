package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
// Defined as a type alias so service adapter packages can implement StorefrontAdapter
// without importing this (tasks) package and creating an import cycle.
type ExternalGameEntry = storefrontadapter.ExternalGameEntry

// StorefrontAdapter is the interface every storefront adapter must satisfy.
type StorefrontAdapter = storefrontadapter.Adapter

// ErrCredentials is returned by an adapter when credentials are invalid,
// expired, or cannot be decrypted. DispatchSyncWorker marks the job failed on this error.
var ErrCredentials = storefrontadapter.ErrCredentials

// ── DispatchSync ──────────────────────────────────────────────────────────────

// DispatchSyncArgs is the River job args type for "dispatch_sync".
type DispatchSyncArgs struct {
	JobID      string `json:"job_id"`
	UserID     string `json:"user_id"`
	Storefront string `json:"storefront"`
}

func (DispatchSyncArgs) Kind() string { return "dispatch_sync" }

func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 1}
}

// Timeout overrides River's 1-minute default so large libraries (hundreds of
// games needing sequential appdetails calls) can complete in a single run.
func (w *DispatchSyncWorker) Timeout(*river.Job[DispatchSyncArgs]) time.Duration { return -1 }

// DispatchSyncWorker is a River worker that drives a full sync run for one
// (user, storefront) pair using a unified StorefrontAdapter.
type DispatchSyncWorker struct {
	river.WorkerDefaults[DispatchSyncArgs]
	DB          *bun.DB
	Adapter     func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (StorefrontAdapter, error)
	RiverClient *river.Client[pgx.Tx]
}

func resolvePlatforms(platforms []string) []string {
	var resolved []string
	for _, p := range platforms {
		if slug, ok := platformresolution.PlatformToSlug(p); ok {
			resolved = append(resolved, slug)
		}
	}
	if len(resolved) == 0 {
		resolved = []string{"pc-windows"}
	}
	return resolved
}

func upsertExternalGame(ctx context.Context, db *bun.DB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
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
		RETURNING id, is_skipped`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus,
	).Scan(ctx, &row); err != nil {
		slog.Error("dispatch_sync: upsert external_game failed", "err", err, "job_id", p.JobID, "external_id", e.ExternalID)
		return "", false
	}
	return row.ID, row.IsSkipped
}

func upsertPlatforms(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64) {
	for i, platform := range platforms {
		hours := 0.0
		if i == 0 {
			hours = playtimeHours
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
			VALUES (?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
			uuid.NewString(), egID, platform, hours,
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: upsert platform failed", "err", err, "external_game_id", egID, "platform", platform)
		}
	}
}

func insertJobItem(ctx context.Context, db *bun.DB, egID string, e ExternalGameEntry, p DispatchSyncArgs) string {
	var row struct {
		ID string `bun:"id"`
	}
	if err := db.NewRaw(`
		INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
		ON CONFLICT (job_id, item_key) DO NOTHING
		RETURNING id`,
		uuid.NewString(), p.JobID, p.UserID, e.ExternalID, e.Title, egID,
	).Scan(ctx, &row); err != nil {
		// sql.ErrNoRows means ON CONFLICT fired — item already exists; skip silently.
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("dispatch_sync: insert job_item failed", "err", err, "job_id", p.JobID, "external_id", e.ExternalID)
		}
		return ""
	}
	return row.ID
}

func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
	p := job.Args

	// 1. Mark job as processing.
	now := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ?, dispatch_complete = false WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}

	// 2. Load sync config.
	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		failSyncJob(ctx, w.DB, p.JobID, "no sync config found")
		return nil
	}

	// 3. Build adapter (credential loading, decryption, and token refresh happen inside).
	adapter, err := w.Adapter(ctx, p.Storefront, cfg)
	if errors.Is(err, ErrCredentials) {
		failSyncJob(ctx, w.DB, p.JobID, "credentials error")
		if _, err := w.DB.NewRaw(
			`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
			p.UserID, p.Storefront,
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: flag credentials_error failed", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
		}
		return nil
	}
	if err != nil {
		failSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	fetchedIDs := make(map[string]struct{})
	seenPlatforms := make(map[string][]string)

	// 4. Fetch library; upsert external_games + platforms; insert job_items;
	//    enqueue Stage 2 (IGDBMatch) per batch as each batch completes.
	slog.Info("dispatch_sync: starting library fetch", "job_id", p.JobID, "user_id", p.UserID, "storefront", p.Storefront)
	totalProcessed := 0
	if err := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
		var batchItemIDs []string
		skippedInBatch := 0
		for _, e := range batch {
			fetchedIDs[e.ExternalID] = struct{}{}
			platforms := resolvePlatforms(e.Platforms)
			egID, isSkipped := upsertExternalGame(ctx, w.DB, e, p)
			if egID == "" {
				continue
			}
			seenPlatforms[egID] = append(seenPlatforms[egID], platforms...)
			upsertPlatforms(ctx, w.DB, egID, platforms, e.PlaytimeHours)
			if isSkipped {
				skippedInBatch++
				slog.Debug("dispatch_sync: game is user-skipped, not enqueuing for matching",
					"job_id", p.JobID, "external_id", e.ExternalID, "title", e.Title)
				if _, err := w.DB.NewRaw(
					`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
					 VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
					uuid.NewString(), p.JobID, p.UserID, egID, e.Title,
				).Exec(ctx); err != nil {
					slog.Error("dispatch_sync: insert sync_change (skipped)", "err", err)
				}
			} else {
				if itemID := insertJobItem(ctx, w.DB, egID, e, p); itemID != "" {
					batchItemIDs = append(batchItemIDs, itemID)
				}
			}
		}
		totalProcessed += len(batch)
		slog.Debug("dispatch_sync: batch complete",
			"job_id", p.JobID,
			"batch_size", len(batch),
			"enqueued_for_matching", len(batchItemIDs),
			"skipped_by_user", skippedInBatch,
			"total_processed_so_far", totalProcessed,
		)
		for _, itemID := range batchItemIDs {
			_ = EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, IGDBMatchArgs{JobItemID: itemID}) //nolint:errcheck // EnqueueOrFail records its own failure on the job_item
		}
		return nil
	}); err != nil {
		if errors.Is(err, ErrCredentials) {
			failSyncJob(ctx, w.DB, p.JobID, "credentials error")
			if _, err := w.DB.NewRaw(
				`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
				p.UserID, p.Storefront,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: flag credentials_error failed", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
			}
			return nil
		}
		slog.Error("dispatch_sync: library fetch failed", "job_id", p.JobID, "err", err)
		failSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	// 6. Stale platform sweep: remove platform rows no longer present upstream.
	for egID, platforms := range seenPlatforms {
		if _, err := w.DB.NewRaw(`
			DELETE FROM external_game_platforms
			WHERE external_game_id = ? AND platform NOT IN (?)`,
			egID, bun.List(platforms),
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: delete stale platforms failed", "err", err, "external_game_id", egID)
		}
	}

	// 7. Mark removed games as unavailable and write sync_changes('removed').
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
			if _, err := w.DB.NewRaw(
				`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: insert sync_change (removed) failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
		}
	}

	// 8. Update last_synced_at and clear any prior credentials_error flag.
	syncedNow := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background()); err != nil {
		slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
	}

	// 9. Dispatch is fully complete — every batch has been streamed and enqueued.
	//    Open the completion gate and run the authoritative check: this finalizes
	//    the job when all items already drained during dispatch (including an
	//    empty library), and lets per-item checks finalize it from here on.
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, p.JobID,
	).Exec(ctx); err != nil {
		// The gate write failed, so the gate stays closed and the completion
		// check below would be a guaranteed no-op — skip it. The job stays in
		// 'processing'; the user can cancel the stuck sync (recovery of such
		// jobs is out of scope for #642).
		slog.Error("dispatch_sync: mark dispatch_complete failed", "err", err, "job_id", p.JobID)
		return nil
	}
	SyncCheckJobCompletion(ctx, w.DB, p.JobID)

	return nil
}

// failSyncJob marks a job as failed with the given error message and cancels
// any pending job_items for that job so they are not left orphaned.
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: fail job update failed", "err", err, "job_id", jobID)
	}
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = 'cancelled' WHERE job_id = ? AND status = 'pending'`,
		jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: cancel pending items failed", "err", err, "job_id", jobID)
	}
}

// ownershipRank returns a numeric rank for an ownership status string.
// Higher = better. Used to avoid downgrading ownership on update.
func ownershipRank(status string) int {
	switch status {
	case "owned":
		return 4
	case "borrowed", "rented":
		return 3
	case "subscription":
		return 2
	case "no_longer_owned":
		return 1
	default:
		return 0
	}
}

// ── IGDBMatchWorker — Stage 2 ─────────────────────────────────────────────────

type IGDBMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (IGDBMatchArgs) Kind() string { return "igdb_match" }

func (IGDBMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

type IGDBMatchWorker struct {
	river.WorkerDefaults[IGDBMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *IGDBMatchWorker) Work(ctx context.Context, job *river.Job[IGDBMatchArgs]) error {
	p := job.Args

	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("igdb_match: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	slog.Debug("igdb_match: processing",
		"item_id", p.JobItemID,
		"title", eg.Title,
		"storefront", eg.Storefront,
		"external_game_id", eg.ID,
		"attempt", job.Attempt,
	)

	// Fast-path: skipped games go straight to UserGameWorker.
	if eg.IsSkipped {
		slog.Debug("igdb_match: game is skipped, fast-path to user_game_write", "item_id", p.JobItemID, "title", eg.Title)
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Fast-path: already resolved (manual or prior run).
	if eg.ResolvedIGDBID != nil {
		slog.Debug("igdb_match: already resolved, fast-path to user_game_write",
			"item_id", p.JobItemID, "title", eg.Title, "igdb_id", *eg.ResolvedIGDBID)
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Sibling check: same user/storefront/title already resolved by another SKU.
	var sibling models.ExternalGame
	if err := w.DB.NewSelect().Model(&sibling).
		Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NOT NULL",
			eg.UserID, eg.Storefront, eg.Title, eg.ID).
		Limit(1).
		Scan(ctx); err == nil && sibling.ResolvedIGDBID != nil {
		igdbID := *sibling.ResolvedIGDBID
		slog.Debug("igdb_match: sibling match, inheriting resolution",
			"item_id", p.JobItemID, "title", eg.Title, "igdb_id", igdbID, "sibling_id", sibling.ID)
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: insert game row (sibling)", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: apply sibling resolution", "err", err, "external_game_id", eg.ID)
		}
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// IGDB search.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		platformIDs, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, w.DB, eg.ID)
		if perErr != nil {
			slog.Debug("igdb_match: platform resolution failed, falling back to unfiltered",
				"item_id", p.JobItemID, "external_game_id", eg.ID, "err", perErr)
			platformIDs = nil
		}
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, platformIDs)
		if err != nil {
			if job.Attempt >= job.MaxAttempts {
				slog.Warn("igdb_match: IGDB failed on final attempt, marking pending_review",
					"item_id", p.JobItemID, "err", err)
				syncMarkItemPendingReview(ctx, w.DB, &item)
				SyncCheckJobCompletion(ctx, w.DB, item.JobID)
				return nil
			}
			return fmt.Errorf("igdb_match: search failed (will retry): %w", err)
		}

		normalizedQuery := matching.NormalizeTitle(eg.Title)
		var bestScore, secondBestScore float64
		var bestID int32
		for _, c := range candidates {
			score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(c.Title))
			if score > bestScore {
				secondBestScore = bestScore
				bestScore = score
				bestID = int32(c.IgdbID)
			} else if score > secondBestScore {
				secondBestScore = score
			}
		}

		slog.Debug("igdb_match: search results",
			"item_id", p.JobItemID,
			"title", eg.Title,
			"candidate_count", len(candidates),
			"best_score", bestScore,
			"second_best_score", secondBestScore,
			"best_igdb_id", bestID,
		)

		const autoResolveThreshold = 0.85
		const tieEpsilon = 0.01
		if bestScore >= autoResolveThreshold && (bestScore-secondBestScore) > tieEpsilon {
			slog.Debug("igdb_match: auto-resolved",
				"item_id", p.JobItemID, "title", eg.Title, "igdb_id", bestID, "score", bestScore)
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: insert game row (auto-resolve)", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: auto-resolve external_game", "err", err, "external_game_id", eg.ID)
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}

		// Low confidence or tie — store candidates, mark pending_review.
		slog.Debug("igdb_match: low confidence, marking pending_review",
			"item_id", p.JobItemID,
			"title", eg.Title,
			"best_score", bestScore,
			"threshold", autoResolveThreshold,
			"tie_gap", bestScore-secondBestScore,
			"candidate_count", len(candidates),
		)
		candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
		item.IGDBCandidates = candidatesJSON
		item.MatchConfidence = &bestScore
		syncMarkItemPendingReview(ctx, w.DB, &item)
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// No IGDB client configured — mark pending_review.
	slog.Debug("igdb_match: no IGDB client configured, marking pending_review", "item_id", p.JobItemID, "title", eg.Title)
	syncMarkItemPendingReview(ctx, w.DB, &item)
	SyncCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
}

func (w *IGDBMatchWorker) enqueueUserGame(ctx context.Context, jobItemID, jobID string) error {
	if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, jobItemID, UserGameArgs{JobItemID: jobItemID}); err != nil {
		slog.Error("igdb_match: enqueue user_game_write failed", "item_id", jobItemID, "err", err)
		SyncCheckJobCompletion(ctx, w.DB, jobID)
	}
	return nil
}

// ── UserGameWorker — Stage 3 ──────────────────────────────────────────────────

type UserGameArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (UserGameArgs) Kind() string { return "user_game_write" }

func (UserGameArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// UserGameWorker writes the user_game and user_game_platform rows for a resolved sync item.
type UserGameWorker struct {
	river.WorkerDefaults[UserGameArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *UserGameWorker) Work(ctx context.Context, job *river.Job[UserGameArgs]) error {
	p := job.Args

	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("user_game_write: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

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

	// Manual resolution propagation: job_item has resolved_igdb_id but external_game doesn't.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID)
		eg.ResolvedIGDBID = &igdbID
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert game row (manual resolve)", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: apply manual resolution", "err", err, "external_game_id", eg.ID)
		}
	}

	if eg.ResolvedIGDBID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "no resolved_igdb_id on external_game")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Ensure games row exists.
	if _, err := w.DB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		*eg.ResolvedIGDBID, eg.Title,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: ensure game row", "err", err)
	}

	// (xmax = 0) detects new vs. updated row; relies on ON CONFLICT DO UPDATE (not DO NOTHING).
	// NOTE: sync_changes('added') is written after the platform loop to avoid orphans when
	// platform load fails after the user_games row has already been committed.
	ugID := uuid.NewString()
	now := time.Now().UTC()
	var isNewRow struct {
		ID    string `bun:"id"`
		IsNew bool   `bun:"is_new"`
	}
	if err := w.DB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
		 RETURNING id, (xmax = 0) AS is_new`,
		ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
	).Scan(ctx, &isNewRow); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("upsert user_game: %v", err))
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
	ugID = isNewRow.ID

	// Load platform rows from external_game_platforms.
	var egPlatforms []models.ExternalGamePlatform
	if err := w.DB.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("load platforms: %v", err))
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
	if len(egPlatforms) == 0 {
		syncMarkItemFailed(ctx, w.DB, &item, "external game has no platform rows")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	storefrontSlug, ok := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !ok {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved storefront=%s", eg.Storefront))
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	ownership := "owned"
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	}

	var platformUpgraded bool
	for _, egp := range egPlatforms {
		var existingID string
		var existingOwnership *string
		var existingHours *float64
		err := w.DB.NewRaw(
			`SELECT id, ownership_status, hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
			ugID, egp.Platform, storefrontSlug,
		).Scan(ctx, &existingID, &existingOwnership, &existingHours)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			// No existing row — insert new platform.
			ugpID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO user_game_platforms
				(id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
				 original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
				VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
				ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				ugpID, ugID, egp.Platform, storefrontSlug, egp.HoursPlayed, ownership,
				egp.Platform, eg.Storefront, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("user_game_write: insert user_game_platform", "err", err, "item_id", p.JobItemID)
			}
		case err != nil:
			slog.Error("user_game_write: select existing ugp", "err", err, "item_id", p.JobItemID)
		default:
			// Resolve final ownership and hours in Go, then write a single
			// unconditional UPDATE. This collapses the previous three branches
			// (ownership upgrade / hours-only / no-op) and guarantees that
			// external_game_id is always backfilled — see docs/sync.md
			// § "Manually added games".
			existingRank := 0
			if existingOwnership != nil {
				existingRank = ownershipRank(*existingOwnership)
			}
			newRank := ownershipRank(ownership)

			finalOwnership := ownership
			if existingOwnership != nil {
				finalOwnership = *existingOwnership
			}
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

			finalHours := egp.HoursPlayed
			if existingHours != nil && *existingHours > finalHours {
				finalHours = *existingHours
			}

			if _, err := w.DB.NewRaw(
				`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, external_game_id = ?, updated_at = now() WHERE id = ?`,
				finalOwnership, finalHours, eg.ID, existingID,
			).Exec(ctx); err != nil {
				slog.Error("user_game_write: update ugp", "err", err, "item_id", p.JobItemID)
			}
		}
	}

	if _, err := w.DB.NewRaw(
		`UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: update external_game updated_at", "err", err)
	}

	// Write sync_changes('added') only after platforms are confirmed written,
	// preventing orphan user_games + sync_changes rows on platform-load failure.
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

	// Immediate metadata fetch: if IGDB is configured and the games row has no
	// description yet, enqueue a fire-and-forget fetch so newly added games get
	// cover art and full IGDB data within seconds rather than waiting for the
	// next scheduled bulk refresh. Non-fatal — the bulk refresh is the safety net.
	w.maybeEnqueueImmediateMetadataFetch(ctx, *eg.ResolvedIGDBID)

	syncMarkItemCompleted(ctx, w.DB, &item)
	SyncCheckJobCompletion(ctx, w.DB, item.JobID)
	return nil
}

// maybeEnqueueImmediateMetadataFetch enqueues a fire-and-forget metadata_fetch
// job for gameID when IGDB is configured and the games row has no description
// yet. Every failure mode is non-fatal — the periodic bulk refresh is the
// safety net — so we log at warn and move on rather than failing the job_item.
func (w *UserGameWorker) maybeEnqueueImmediateMetadataFetch(ctx context.Context, gameID int32) {
	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		return
	}

	var descriptionIsNull bool
	if err := w.DB.NewRaw(
		`SELECT description IS NULL FROM games WHERE id = ?`, gameID,
	).Scan(ctx, &descriptionIsNull); err != nil {
		slog.Warn("user_game_write: check game description for immediate metadata fetch",
			"err", err, "game_id", gameID)
		return
	}
	if !descriptionIsNull {
		return // already has metadata
	}

	if w.RiverClient == nil {
		slog.Warn("user_game_write: river client unavailable, skipping immediate metadata fetch",
			"game_id", gameID)
		return
	}
	if _, err := w.RiverClient.Insert(ctx, MetadataFetchArgs{GameID: gameID}, nil); err != nil {
		slog.Warn("user_game_write: enqueue immediate metadata fetch failed",
			"err", err, "game_id", gameID)
	}
}

// syncMarkItemFailed sets a job_item to failed with an error message.
func syncMarkItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "error_message", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemFailed", "id", item.ID, "err", err)
	}
}

// syncMarkItemCompleted sets a job_item to completed.
func syncMarkItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusCompleted
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemCompleted", "id", item.ID, "err", err)
	}
}

// syncMarkItemSkipped sets a job_item to skipped.
func syncMarkItemSkipped(ctx context.Context, db *bun.DB, item *models.JobItem) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusSkipped
	item.ProcessedAt = &now
	_, err := db.NewUpdate().Model(item).
		Column("status", "processed_at").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemSkipped", "id", item.ID, "err", err)
	}
}

// syncMarkItemPendingReview sets a job_item to pending_review and persists any IGDB candidates.
func syncMarkItemPendingReview(ctx context.Context, db *bun.DB, item *models.JobItem) {
	item.Status = models.JobItemStatusPendingReview
	_, err := db.NewUpdate().Model(item).
		Column("status", "igdb_candidates", "match_confidence").
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error("process_sync_item: syncMarkItemPendingReview", "id", item.ID, "err", err)
	}
}

// SyncCheckJobCompletion checks whether active processing of job_items is complete and
// drives the job to its terminal state.
//
// "Active" means pending or processing. pending_review items require user action and
// do not count as active, but they DO block job termination — the job stays in
// 'processing' until every item has been resolved by the user, auto-matched, or failed.
//
// Once no active items remain:
//   - pending_review items still exist: job stays processing (user must review).
//   - No pending_review items remain: marks job completed (individual item failures are
//     surfaced via the job_items table, not the job status).
//
// In addition, a sync job is never finalized while its dispatch is still
// streaming batches: DispatchSyncWorker sets jobs.dispatch_complete=false on
// entry and true only after the full library has been dispatched, so the
// completion check below treats dispatch_complete=false as "more work may
// still arrive" and refuses to finalize.
func SyncCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	var activeRemaining int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'processing')`,
		jobID,
	).Scan(ctx, &activeRemaining); err != nil {
		slog.Error("sync: SyncCheckJobCompletion count", "job_id", jobID, "err", err)
		return
	}
	if activeRemaining > 0 {
		return
	}

	// If pending_review items still exist the job stays processing — user must resolve them.
	var pendingReviewCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'pending_review'`,
		jobID,
	).Scan(ctx, &pendingReviewCount); err != nil {
		slog.Error("sync: SyncCheckJobCompletion pending_review count", "job_id", jobID, "err", err)
		return
	}
	if pendingReviewCount > 0 {
		return
	}

	now := time.Now().UTC()
	finalStatus := "completed"
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ? WHERE id = ? AND status IN ('pending', 'processing') AND dispatch_complete = true`,
		finalStatus, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("sync: SyncCheckJobCompletion finalize job failed", "err", err, "job_id", jobID, "final_status", finalStatus)
	}
}
