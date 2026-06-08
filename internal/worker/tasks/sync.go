package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
	"github.com/drzero42/nexorious/internal/services/platformresolution"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
	"github.com/drzero42/nexorious/internal/usergame"
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
	return river.InsertOpts{MaxAttempts: 3, Priority: 1}
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

// nullableJSON returns nil when b is empty so the column is written as SQL NULL.
// When non-empty it returns a json.RawMessage so pgdriver encodes it as JSONB.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

func upsertExternalGame(ctx context.Context, db *bun.DB, e ExternalGameEntry, p DispatchSyncArgs) (egID string, isSkipped bool) {
	var row struct {
		ID        string `bun:"id"`
		IsSkipped bool   `bun:"is_skipped"`
		IsNew     bool   `bun:"is_new"`
	}
	var sourceMetaJSON []byte
	if len(e.SourceMetadata) > 0 {
		sourceMetaJSON, _ = json.Marshal(e.SourceMetadata) //nolint:errcheck // marshaling a map[string]string cannot fail
	}
	if err := db.NewRaw(`
		INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, source_metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, now(), now())
		ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
			title = EXCLUDED.title,
			is_subscription = EXCLUDED.is_subscription,
			ownership_status = EXCLUDED.ownership_status,
			source_metadata = COALESCE(EXCLUDED.source_metadata, external_games.source_metadata),
			is_available = true,
			updated_at = now()
		RETURNING id, is_skipped, (xmax = 0) AS is_new`,
		uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
		e.IsSubscription, e.OwnershipStatus, nullableJSON(sourceMetaJSON),
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

	// Mark job as processing.
	now := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ?, dispatch_complete = false WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: mark processing failed", "err", err, "job_id", p.JobID)
	}

	// Load sync config.
	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		failSyncJob(ctx, w.DB, p.JobID, "no sync config found")
		return nil
	}

	// Build adapter (credential loading, decryption, and token refresh happen inside).
	adapter, err := w.Adapter(ctx, p.Storefront, cfg)
	if errors.Is(err, ErrCredentials) {
		handleCredentialError(ctx, w.DB, p, cfg.CredentialsError)
		return nil
	}
	if err != nil {
		failSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	fetchedIDs := make(map[string]struct{})
	seenPlatforms := make(map[string][]string)

	// Fetch library; upsert external_games + platforms; insert job_items;
	//    enqueue Stage 2 (IGDBMatch) per batch as each batch completes.
	slog.Info("dispatch_sync: starting library fetch", "job_id", p.JobID, "user_id", p.UserID, "storefront", p.Storefront)
	totalProcessed := 0
	if err := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
		var batchItemIDs []string
		skippedInBatch := 0
		for _, e := range batch {
			fetchedIDs[e.ExternalID] = struct{}{}
			// Every external game must have at least one platform; default to PC
			// (Windows) when the adapter reports none. Platform-slug validity is
			// enforced by the external_game_platforms -> platforms(name) foreign key,
			// so adapters must emit canonical platforms.name slugs.
			platforms := e.Platforms
			if len(platforms) == 0 {
				platforms = []string{"pc-windows"}
			}
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
					`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
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
			handleCredentialError(ctx, w.DB, p, cfg.CredentialsError)
			return nil
		}
		slog.Error("dispatch_sync: library fetch failed", "job_id", p.JobID, "err", err)
		failSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	// Stale platform sweep: remove platform rows no longer present upstream.
	for egID, platforms := range seenPlatforms {
		if _, err := w.DB.NewRaw(`
			DELETE FROM external_game_platforms
			WHERE external_game_id = ? AND platform NOT IN (?)`,
			egID, bun.List(platforms),
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: delete stale platforms failed", "err", err, "external_game_id", egID)
		}
	}

	// Mark removed games as unavailable and write changes('removed').
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
				`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: insert sync_change (removed) failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
		}
	}

	// Update last_synced_at and clear any prior credentials_error flag.
	syncedNow := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background()); err != nil {
		slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
	}

	// Dispatch is fully complete — every batch has been streamed and enqueued.
	//    Open the completion gate and run the authoritative check: this finalizes
	//    the job when all items already drained during dispatch (including an
	//    empty library), and lets per-item checks finalize it from here on.
	if _, err := w.DB.NewRaw(
		`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, p.JobID,
	).Exec(context.Background()); err != nil {
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

// markSyncJobFailed marks a job as failed with the given error message and
// cancels any pending job_items for that job so they are not left orphaned. It
// performs no notification — callers decide which event (if any) to emit.
func markSyncJobFailed(ctx context.Context, db *bun.DB, jobID, msg string) {
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

// failSyncJob marks a job as failed (see markSyncJobFailed) and emits the
// generic sync.failed notification. Used for all non-credential failures;
// credential failures go through handleCredentialError instead.
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	markSyncJobFailed(ctx, db, jobID, msg)
	userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncFailed, Scope: notify.ScopeUser, ActorUserID: userID,
		Payload:  notify.SyncFailedPayload{Storefront: storefront, Error: msg, JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeSyncFailed,
	})
}

// handleCredentialError handles a credentials failure during sync: it fails the
// job, flags credentials_error, and — only on the healthy→expired transition
// (priorErr == false) — notifies the user. Subscribers to sync.auth_expired get
// that actionable event; everyone else falls back to sync.failed so they are
// not left silent. While the storefront stays broken (priorErr == true) no
// notification is sent, so repeated scheduled syncs do not nag.
func handleCredentialError(ctx context.Context, db *bun.DB, p DispatchSyncArgs, priorErr bool) {
	markSyncJobFailed(ctx, db, p.JobID, "credentials error")
	if _, err := db.NewRaw(
		`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		p.UserID, p.Storefront,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: flag credentials_error failed", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
	}

	if priorErr {
		return // already in error state; notify only on transition
	}

	if userSubscribed(ctx, db, p.UserID, notify.TypeSyncAuthExpired) {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncAuthExpired, Scope: notify.ScopeUser, ActorUserID: p.UserID,
			Payload:  notify.SyncAuthExpiredPayload{Storefront: p.Storefront},
			DedupKey: p.JobID + ":" + notify.TypeSyncAuthExpired,
		})
		return
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncFailed, Scope: notify.ScopeUser, ActorUserID: p.UserID,
		Payload:  notify.SyncFailedPayload{Storefront: p.Storefront, Error: "credentials error", JobID: p.JobID},
		DedupKey: p.JobID + ":" + notify.TypeSyncFailed,
	})
}

// userSubscribed reports whether the user is subscribed to eventType. On query
// error it returns false, so a credential failure still falls back to the
// default-on sync.failed event rather than going silent.
func userSubscribed(ctx context.Context, db *bun.DB, userID, eventType string) bool {
	var subscribed bool
	if err := db.NewRaw(
		`SELECT EXISTS(SELECT 1 FROM notification_subscriptions WHERE user_id = ? AND event_type = ?)`,
		userID, eventType,
	).Scan(ctx, &subscribed); err != nil {
		slog.Error("dispatch_sync: subscription check failed", "err", err, "user_id", userID, "event_type", eventType)
		return false
	}
	return subscribed
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
		markItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		markItemFailed(ctx, w.DB, &item, "external game not found", "process_sync_item: markItemFailed")
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

		cands := make([]matching.Candidate, len(candidates))
		for i, c := range candidates {
			cands[i] = matching.Candidate{ID: int32(c.IgdbID), Title: c.Title} //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		}
		decision := matching.Decide(eg.Title, cands)

		slog.Debug("igdb_match: search results",
			"item_id", p.JobItemID,
			"title", eg.Title,
			"candidate_count", len(candidates),
			"best_score", decision.BestScore,
			"second_best_score", decision.SecondBest,
			"best_igdb_id", decision.ResolvedID,
		)

		if decision.Confident {
			bestID := decision.ResolvedID
			slog.Debug("igdb_match: auto-resolved",
				"item_id", p.JobItemID, "title", eg.Title, "igdb_id", bestID, "score", decision.BestScore)
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
			"best_score", decision.BestScore,
			"threshold", matching.AutoResolveThreshold,
			"tie_gap", decision.BestScore-decision.SecondBest,
			"candidate_count", len(candidates),
		)
		candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
		item.IGDBCandidates = candidatesJSON
		bs := decision.BestScore
		item.MatchConfidence = &bs
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
		markItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		markItemFailed(ctx, w.DB, &item, "external game not found", "process_sync_item: markItemFailed")
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
			`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
			 VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
			uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert sync_change (skipped)", "err", err)
		}
		markItemSkipped(ctx, w.DB, &item, "process_sync_item: markItemSkipped")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	// Manual resolution propagation: job_item has resolved_igdb_id but external_game doesn't.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
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
		markItemFailed(ctx, w.DB, &item, "no resolved_igdb_id on external_game", "process_sync_item: markItemFailed")
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
	// NOTE: changes('added') is written after the platform loop to avoid orphans when
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
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("upsert user_game: %v", err), "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
	ugID = isNewRow.ID

	// Load platform rows from external_game_platforms.
	var egPlatforms []models.ExternalGamePlatform
	if err := w.DB.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		Scan(ctx); err != nil {
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("load platforms: %v", err), "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
	if len(egPlatforms) == 0 {
		markItemFailed(ctx, w.DB, &item, "external game has no platform rows", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}

	storefrontSlug := eg.Storefront

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
				 external_game_id, sync_from_source, created_at, updated_at)
				VALUES (?, ?, ?, ?, true, ?, ?, ?, true, now(), now())
				ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				ugpID, ugID, egp.Platform, storefrontSlug, egp.HoursPlayed, ownership,
				eg.ID,
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
					`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, old_status, new_status, created_at)
					 VALUES (?, ?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
					uuid.NewString(), item.JobID, item.UserID, eg.ID, ugID, eg.Title, existingOwnership, &ownership,
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

	// Auto-promote not_started → in_progress when the game has any played
	// hours. The shared helper keys off the SUM of all the game's platforms in
	// the DB and its play_status = 'not_started' guard covers freshly-upserted
	// rows (which default to not_started), so no separate isNew check is needed.
	if err := usergame.PromoteToInProgressIfPlayed(ctx, w.DB, ugID); err != nil {
		slog.Error("user_game_write: auto-promote play_status", "err", err, "item_id", p.JobItemID)
	}

	if _, err := w.DB.NewRaw(
		`UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: update external_game updated_at", "err", err)
	}

	// Write changes('added') only after platforms are confirmed written,
	// preventing orphan user_games + changes rows on platform-load failure.
	if isNewRow.IsNew {
		if _, err := w.DB.NewRaw(
			`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
			 VALUES (?, ?, ?, ?, ?, 'added', ?, now())`,
			uuid.NewString(), item.JobID, item.UserID, eg.ID, ugID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert sync_change (added)", "err", err)
		}
	} else if !platformUpgraded {
		if _, err := w.DB.NewRaw(
			`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, created_at)
			 VALUES (?, ?, ?, ?, ?, 'already_in_library', ?, now())`,
			uuid.NewString(), item.JobID, item.UserID, eg.ID, ugID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert sync_change (already_in_library)", "err", err)
		}
	}

	// Immediate metadata fetch: if IGDB is configured and the games row has no
	// description yet, enqueue a fire-and-forget fetch so newly added games get
	// cover art and full IGDB data within seconds rather than waiting for the
	// next scheduled bulk refresh. Non-fatal — the bulk refresh is the safety net.
	w.maybeEnqueueImmediateMetadataFetch(ctx, *eg.ResolvedIGDBID)

	markItemCompleted(ctx, w.DB, &item, "process_sync_item: markItemCompleted")

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
	activeRemaining, ok := countJobItems(ctx, db, jobID, "status IN ('pending', 'processing')", "sync: SyncCheckJobCompletion count")
	if !ok || activeRemaining > 0 {
		return
	}

	// If pending_review items still exist the job stays processing — user must resolve them.
	pendingReviewCount, ok := countJobItems(ctx, db, jobID, "status = 'pending_review'", "sync: SyncCheckJobCompletion pending_review count")
	if !ok {
		return
	}
	if pendingReviewCount > 0 {
		userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncNeedsReview, Scope: notify.ScopeUser, ActorUserID: userID,
			Payload:  notify.SyncNeedsReviewPayload{Storefront: storefront, Count: pendingReviewCount, JobID: jobID},
			DedupKey: jobID + ":" + notify.TypeSyncNeedsReview,
		})
		return
	}

	// Finalize only when dispatch has finished streaming batches. finalized=false
	// means dispatch is still in flight or the job is already terminal — in either
	// case we must not emit completion notifications.
	if !finalizeJobCompleted(ctx, db, jobID, "sync: SyncCheckJobCompletion finalize job failed", true) {
		return
	}

	userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
	failedCount, ok := countJobItems(ctx, db, jobID, "status = 'failed'", "sync: count failed items for notify")
	if !ok {
		return
	}
	if failedCount > 0 {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncCompletedWithErrors, Scope: notify.ScopeUser, ActorUserID: userID,
			Payload:  notify.SyncCompletedWithErrorsPayload{Storefront: storefront, Failed: failedCount, JobID: jobID},
			DedupKey: jobID + ":" + notify.TypeSyncCompletedWithErrors,
		})
	} else {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncCompleted, Scope: notify.ScopeUser, ActorUserID: userID,
			Payload:  notify.SyncCompletedPayload{Storefront: storefront, JobID: jobID},
			DedupKey: jobID + ":" + notify.TypeSyncCompleted,
		})
	}
	emitSyncDiff(ctx, db, jobID, userID)
}

// syncJobUserAndStorefront fetches the owning user_id and storefront (source)
// for a job. Returns ("","") on error.
func syncJobUserAndStorefront(ctx context.Context, db *bun.DB, jobID string) (userID, storefront string) {
	var row struct {
		UserID string `bun:"user_id"`
		Source string `bun:"source"`
	}
	if err := db.NewRaw(`SELECT user_id, source FROM jobs WHERE id = ?`, jobID).Scan(ctx, &row); err != nil {
		slog.Error("sync: lookup job user/storefront", "job_id", jobID, "err", err)
		return "", ""
	}
	return row.UserID, row.Source
}

// emitSyncDiff emits sync.diff iff changes rows exist for the job.
func emitSyncDiff(ctx context.Context, db *bun.DB, jobID, userID string) {
	var rows []struct {
		ChangeType string `bun:"change_type"`
		Title      string `bun:"title"`
		Platforms  string `bun:"platforms"`
	}
	if err := db.NewRaw(
		`SELECT sc.change_type,
		        sc.title,
		        COALESCE(string_agg(egp.platform, ',' ORDER BY egp.platform), '') AS platforms
		   FROM changes sc
		   LEFT JOIN external_game_platforms egp ON egp.external_game_id = sc.external_game_id
		  WHERE sc.job_id = ? AND sc.change_type IN ('added','removed')
		  GROUP BY sc.id, sc.change_type, sc.title, sc.created_at
		  ORDER BY sc.created_at`,
		jobID,
	).Scan(ctx, &rows); err != nil {
		slog.Error("sync: load changes for diff notify", "job_id", jobID, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	added := []notify.DiffGame{}
	removed := []notify.DiffGame{}
	for _, r := range rows {
		platforms := []string{}
		if r.Platforms != "" {
			platforms = strings.Split(r.Platforms, ",")
		}
		entry := notify.DiffGame{Title: r.Title, Platforms: platforms}
		if r.ChangeType == "added" {
			added = append(added, entry)
		} else {
			removed = append(removed, entry)
		}
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncDiff, Scope: notify.ScopeUser, ActorUserID: userID,
		Payload:  notify.SyncDiffPayload{Added: added, Removed: removed, JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeSyncDiff,
	})
}
