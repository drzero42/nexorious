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
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/observability"
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
		slog.WarnContext(ctx, "dispatch_sync: upsert external_game failed", logging.KeyErr, err, "external_id", e.ExternalID, logging.Cat(logging.CategoryDB))
		return "", false
	}

	if row.IsNew {
		// A new edition of a known game becomes its child and takes its skip
		// (#1339). Only a game a child can inherit from qualifies: top-level and
		// available, matched or skipped. An unmatched, unskipped game that has
		// left the library could never end a child's wait. The oldest wins. One
		// statement, with the parent locked, so a skip committing in between is
		// not missed.
		var inheritedSkip bool
		err := db.NewRaw(`
			UPDATE external_games c
			SET parent_id = p.id, is_skipped = p.is_skipped
			FROM (
			  SELECT id, is_skipped FROM external_games
			  WHERE user_id = ? AND storefront = ? AND title = ?
			    AND id != ? AND parent_id IS NULL
			    AND (is_available OR resolved_igdb_id IS NOT NULL OR is_skipped)
			  ORDER BY created_at, id
			  LIMIT 1
			  FOR SHARE
			) p
			WHERE c.id = ? AND c.parent_id IS NULL
			RETURNING c.is_skipped`,
			p.UserID, p.Storefront, e.Title, row.ID, row.ID,
		).Scan(ctx, &inheritedSkip)
		switch {
		case err == nil:
			row.IsSkipped = inheritedSkip
		case !errors.Is(err, sql.ErrNoRows):
			slog.WarnContext(ctx, "dispatch_sync: set parent_id failed", logging.KeyErr, err, logging.KeyExternalGameID, row.ID, logging.Cat(logging.CategoryDB))
		}
	}

	return row.ID, row.IsSkipped
}

func upsertPlatforms(ctx context.Context, db *bun.DB, egID string, platforms []string, playtimeHours float64, achievementsUnlocked, achievementsTotal *int) {
	for i, platform := range platforms {
		hours := 0.0
		var unlocked, total *int
		if i == 0 {
			hours = playtimeHours
			unlocked = achievementsUnlocked
			total = achievementsTotal
		}
		if _, err := db.NewRaw(`
			INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, achievements_unlocked, achievements_total, created_at)
			VALUES (?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (external_game_id, platform) DO UPDATE SET
				hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played),
				achievements_unlocked = COALESCE(EXCLUDED.achievements_unlocked, external_game_platforms.achievements_unlocked),
				achievements_total = COALESCE(EXCLUDED.achievements_total, external_game_platforms.achievements_total)`,
			uuid.NewString(), egID, platform, hours, unlocked, total,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "dispatch_sync: upsert platform failed", logging.KeyErr, err, logging.KeyExternalGameID, egID, "platform", platform, logging.Cat(logging.CategoryDB))
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
			slog.ErrorContext(ctx, "dispatch_sync: insert job_item failed", logging.KeyErr, err, "external_id", e.ExternalID, logging.Cat(logging.CategoryDB))
		}
		return ""
	}
	return row.ID
}

func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
	p := job.Args
	// Seed the application job id so every in-job log line (and the helpers below
	// that receive this ctx) is correlated to the user-facing jobs.id.
	ctx = logging.WithJobID(ctx, p.JobID)

	// Mark job as processing — only while it is still active. A dispatch can
	// outlive its Job (cancel and reset leave a queued dispatch in River, and a
	// restart re-runs one): it must never turn a cancelled or failed Job back
	// into a running sync (#1302).
	now := time.Now().UTC()
	res, err := w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ?, dispatch_complete = false
		 WHERE id = ? AND status IN ('pending', 'processing')`,
		now, p.JobID,
	).Exec(ctx)
	if dispatchInterrupted(ctx) {
		return nil
	}
	if err != nil {
		slog.WarnContext(ctx, "dispatch_sync: mark processing failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	} else if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // advisory RowsAffected
		slog.InfoContext(ctx, "dispatch_sync: job is no longer active; nothing to do", logging.KeySource, p.Storefront)
		return nil
	}

	// Child games (#1339): give every child its parent's skip, then promote the
	// children of parents that left in an earlier sync, so a new edition in this
	// fetch finds the promoted game as its parent. Failures are logged: the next
	// dispatch repairs what this one missed, and a child that cannot wait is
	// failed rather than left hanging.
	if err := repairChildSkips(ctx, w.DB, p.UserID, p.Storefront); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: repair child skips failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	promoteOrphanedChildren(ctx, w.DB, p.UserID, p.Storefront)

	// Load sync config.
	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		if dispatchInterrupted(ctx) {
			return nil
		}
		FailSyncJob(ctx, w.DB, p.JobID, "no sync config found")
		return nil
	}

	// Build adapter (credential loading, decryption, and token refresh happen inside).
	adapter, err := w.Adapter(ctx, p.Storefront, cfg)
	if dispatchInterrupted(ctx) {
		return nil
	}
	if errors.Is(err, ErrCredentials) {
		handleCredentialError(ctx, w.DB, p, cfg.CredentialsError)
		return nil
	}
	if err != nil {
		FailSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	fetchedIDs := make(map[string]struct{})
	seenPlatforms := make(map[string][]string)

	// Fetch library; upsert external_games + platforms; insert job_items;
	//    enqueue Stage 2 (IGDBMatch) per batch as each batch completes.
	slog.InfoContext(ctx, "dispatch_sync: starting library fetch", logging.KeyUserID, p.UserID, logging.KeySource, p.Storefront)
	totalProcessed := 0
	fetchErr := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
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
			upsertPlatforms(ctx, w.DB, egID, platforms, e.PlaytimeHours, e.AchievementsUnlocked, e.AchievementsTotal)
			if isSkipped {
				skippedInBatch++
				slog.DebugContext(ctx, "dispatch_sync: game is user-skipped, not enqueuing for matching",
					"external_id", e.ExternalID, "title", e.Title)
				// NOT EXISTS: a dispatch re-run after an interrupt sees the same
				// skipped games again; changes has no unique constraint.
				if _, err := w.DB.NewRaw(
					`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
					 SELECT ?, ?, ?, ?, 'skipped', ?, now()
					 WHERE NOT EXISTS (
					   SELECT 1 FROM changes WHERE job_id = ? AND external_game_id = ? AND change_type = 'skipped')`,
					uuid.NewString(), p.JobID, p.UserID, egID, e.Title, p.JobID, egID,
				).Exec(ctx); err != nil {
					slog.WarnContext(ctx, "dispatch_sync: insert sync_change (skipped)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
				}
			} else {
				if itemID := insertJobItem(ctx, w.DB, egID, e, p); itemID != "" {
					batchItemIDs = append(batchItemIDs, itemID)
				}
			}
		}
		totalProcessed += len(batch)
		slog.DebugContext(ctx, "dispatch_sync: batch complete",
			"batch_size", len(batch),
			"enqueued_for_matching", len(batchItemIDs),
			"skipped_by_user", skippedInBatch,
			"total_processed_so_far", totalProcessed,
		)
		for _, itemID := range batchItemIDs {
			_ = EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, IGDBMatchArgs{JobItemID: itemID}) //nolint:errcheck // EnqueueOrFail records its own failure on the job_item
		}
		return nil
	})
	// An interrupted fetch is neither a failure nor a full fetch, whatever it
	// returned: its batch writes may have failed on the cancelled ctx. Stop
	// before FailSyncJob, the sweeps and the last_synced_at/dispatch_complete
	// writes below (those two use context.Background() and would succeed).
	if dispatchInterrupted(ctx) {
		return nil
	}
	if err := fetchErr; err != nil {
		if errors.Is(err, ErrCredentials) {
			handleCredentialError(ctx, w.DB, p, cfg.CredentialsError)
			return nil
		}
		if _, incomplete := errors.AsType[*storefrontadapter.IncompleteLibraryError](err); incomplete {
			slog.WarnContext(ctx, "dispatch_sync: incomplete library", logging.KeySource, p.Storefront, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
			failSyncJob(ctx, w.DB, p.JobID, err.Error(), true)
			return nil
		}
		slog.ErrorContext(ctx, "dispatch_sync: library fetch failed", logging.KeySource, p.Storefront, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
		FailSyncJob(ctx, w.DB, p.JobID, err.Error())
		return nil
	}

	// Empty-fetch guard (#1324): a fetch that returns nothing while games are
	// known for this storefront is far more likely a privacy setting, an
	// upstream glitch or a changed response shape than a library that really
	// went to zero. Running the sweep would mark every game removed and
	// announce it through sync.diff, so fail the job instead. There is no
	// adapter bypass on purpose: in those failure modes the adapter believes it
	// saw a real, empty library.
	if len(fetchedIDs) == 0 {
		known, err := w.DB.NewSelect().Model((*models.ExternalGame)(nil)).
			Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
			Count(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "dispatch_sync: count available games failed", logging.KeySource, p.Storefront, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			FailSyncJob(ctx, w.DB, p.JobID, "could not verify an empty library fetch; nothing was marked removed")
			return nil
		}
		if known > 0 {
			slog.ErrorContext(ctx, "dispatch_sync: empty library fetch with known games; refusing to mark them removed",
				logging.KeySource, p.Storefront, "known_games", known, logging.Cat(logging.CategoryExternalAPI))
			FailSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf(
				"the storefront returned an empty library while %d games are known; nothing was marked removed. "+
					"If the library really is empty, reset this storefront's sync data or disconnect it.", known))
			return nil
		}
	}

	// Stale platform sweep: remove platform rows no longer present upstream.
	for egID, platforms := range seenPlatforms {
		if _, err := w.DB.NewRaw(`
			DELETE FROM external_game_platforms
			WHERE external_game_id = ? AND platform NOT IN (?)`,
			egID, bun.List(platforms),
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "dispatch_sync: delete stale platforms failed", logging.KeyErr, err, logging.KeyExternalGameID, egID, logging.Cat(logging.CategoryDB))
		}
	}

	// Mark removed games as unavailable and write changes('removed').
	var available []models.ExternalGame
	if err := w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: query available games failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "dispatch_sync: mark game unavailable failed", logging.KeyErr, err, logging.KeyExternalGameID, eg.ID, logging.Cat(logging.CategoryDB))
			}
			if _, err := w.DB.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "dispatch_sync: insert sync_change (removed) failed", logging.KeyErr, err, logging.KeyExternalGameID, eg.ID, logging.Cat(logging.CategoryDB))
			}
		}
	}

	// Parents that left in this sync: promote their children (#1339).
	promoteOrphanedChildren(ctx, w.DB, p.UserID, p.Storefront)

	// The sweeps above ran on ctx: if it was cancelled meanwhile they may be
	// unfinished, so the run must not be marked complete.
	if dispatchInterrupted(ctx) {
		return nil
	}

	// Update last_synced_at and clear any prior credentials_error flag.
	syncedNow := time.Now().UTC()
	if _, err := w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background()); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: update last_synced_at failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
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
		slog.ErrorContext(ctx, "dispatch_sync: mark dispatch_complete failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	// Look again at items with nothing running them: children that waited while
	// the dispatch could still add their parent's item, and games promoted
	// during the sweep. Their igdb_match now matches, inherits or fails them
	// (#1339). The query only selects items of an active Job, so a sync
	// cancelled during its dispatch is not restarted. Background ctx, like the
	// dispatch_complete write above: the shutdown must not cut it off.
	if _, _, err := RequeueOrphanedPendingItems(context.Background(), w.DB, w.RiverClient, p.JobID, 0); err != nil {
		slog.ErrorContext(ctx, "dispatch_sync: re-queue waiting items failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, p.JobID)

	return nil
}

// dispatchInterrupted reports whether the dispatch's ctx was cancelled, which
// (dispatch_sync has no timeout) means the server is shutting down. An
// interrupted dispatch stops without recording anything: the Job stays
// 'processing' with dispatch_complete = false, which is how the startup
// recovery (scheduler.RecoverInterruptedSyncs) finds and re-runs it (#1302).
func dispatchInterrupted(ctx context.Context) bool {
	if ctx.Err() == nil {
		return false
	}
	slog.InfoContext(ctx, "dispatch_sync: interrupted; the sync resumes at the next startup")
	return true
}

// markSyncJobFailed marks a job as failed with the given error message and
// cancels any pending job_items for that job so they are not left orphaned. It
// performs no notification — callers decide which event (if any) to emit.
// msg is scrubbed of URL query strings before persisting: a fetch failure's
// *url.Error embeds the full request URL, query-string credentials included.
//
// The write only applies to a Job that is still active, so a dispatch that
// fails after a cancel does not turn 'cancelled' into 'failed' (#1302). It
// reports whether it changed the Job; when it did not, the caller must skip its
// own side effects (notification, metric, credentials_error flag).
func markSyncJobFailed(ctx context.Context, db *bun.DB, jobID, msg string) bool {
	msg = logging.ScrubURLQueries(msg)
	now := time.Now().UTC()
	res, err := db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ?
		 WHERE id = ? AND status IN ('pending', 'processing')`,
		msg, now, jobID,
	).Exec(ctx)
	if err != nil {
		slog.WarnContext(ctx, "dispatch_sync: fail job update failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return false
	}
	if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // advisory RowsAffected
		slog.InfoContext(ctx, "dispatch_sync: job is no longer active; failure not recorded")
		return false
	}
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = 'cancelled' WHERE job_id = ? AND status = 'pending'`,
		jobID,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: cancel pending items failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	return true
}

// FailSyncJob marks a job as failed (see markSyncJobFailed) and emits the
// generic sync.failed notification. Used for all non-credential failures;
// credential failures go through handleCredentialError instead.
func FailSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	failSyncJob(ctx, db, jobID, msg, false)
}

func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string, notifyOnTransition bool) {
	msg = logging.ScrubURLQueries(msg) // never leak URL query credentials into the event payload
	if !markSyncJobFailed(ctx, db, jobID, msg) {
		return
	}
	userID, storefront := syncJobUserAndStorefront(ctx, db, jobID)
	// Record the hard failure so it surfaces in nexorious_sync_total alongside
	// completed / completed_with_errors; without this a storefront that always
	// hard-fails produces zero metric signal. Guard empty storefront (lookup
	// failure) to avoid a polluting source="" series, matching finalizeJobCompleted.
	if storefront != "" {
		observability.RecordSyncOutcome(ctx, storefront, "failed")
	}
	if notifyOnTransition && previousIncompleteLibraryFailure(ctx, db, jobID, msg) {
		return
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeSyncFailed, Scope: notify.ScopeUser, ActorUserID: userID,
		Payload:  notify.SyncFailedPayload{Storefront: storefront, Error: msg, JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeSyncFailed,
	})
}

// previousIncompleteLibraryFailure checks whether an earlier job failed for
// the same reason since the last successful library fetch. Other failures do
// not reset that interval. Compare completion times to ignore newer jobs.
func previousIncompleteLibraryFailure(ctx context.Context, db *bun.DB, jobID, msg string) bool {
	var repeated bool
	if err := db.NewRaw(`
		SELECT EXISTS (
			SELECT 1
			FROM jobs AS current_job
			JOIN jobs AS previous_job ON previous_job.user_id = current_job.user_id
				AND previous_job.source = current_job.source
			LEFT JOIN user_sync_configs AS cfg ON cfg.user_id = current_job.user_id
				AND cfg.storefront = current_job.source
			WHERE current_job.id = ?
				AND previous_job.job_type = 'sync' AND previous_job.status = 'failed'
				AND previous_job.error_message = ?
				AND previous_job.completed_at < current_job.completed_at
				AND (cfg.last_synced_at IS NULL OR previous_job.completed_at > cfg.last_synced_at)
		)`, jobID, msg).Scan(ctx, &repeated); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: check previous library failure", logging.KeyJobID, jobID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return false
	}
	return repeated
}

// handleCredentialError handles a credentials failure during sync: it fails the
// job, flags credentials_error, and — only on the healthy→expired transition
// (priorErr == false) — notifies the user. Subscribers to sync.auth_expired get
// that actionable event; everyone else falls back to sync.failed so they are
// not left silent. While the storefront stays broken (priorErr == true) no
// notification is sent, so repeated scheduled syncs do not nag.
func handleCredentialError(ctx context.Context, db *bun.DB, p DispatchSyncArgs, priorErr bool) {
	if !markSyncJobFailed(ctx, db, p.JobID, "credentials error") {
		return
	}
	// A credentials failure is a hard sync failure; record it so it is visible
	// in nexorious_sync_total rather than silently absent (see FailSyncJob).
	if p.Storefront != "" {
		observability.RecordSyncOutcome(ctx, p.Storefront, "failed")
	}
	if _, err := db.NewRaw(
		`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		p.UserID, p.Storefront,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "dispatch_sync: flag credentials_error failed", logging.KeyErr, err, logging.KeyUserID, p.UserID, logging.KeySource, p.Storefront, logging.Cat(logging.CategoryDB))
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
		slog.WarnContext(ctx, "dispatch_sync: subscription check failed", logging.KeyErr, err, logging.KeyUserID, userID, "event_type", eventType, logging.Cat(logging.CategoryDB))
		return false
	}
	return subscribed
}

// boolptr returns a pointer to b.
func boolptr(b bool) *bool { return &b }

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
		slog.ErrorContext(ctx, "igdb_match: load job_item", "id", p.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return err
	}
	// Correlate every line below to the user-facing job once the item is loaded.
	ctx = logging.WithJobID(ctx, item.JobID)
	// A late or duplicate River job for an item that is no longer pending
	// (finished, in review, or cancelled) must change nothing (#1339).
	if item.Status != models.JobItemStatusPending {
		slog.DebugContext(ctx, "igdb_match: item is not pending, nothing to do", logging.KeyJobItemID, item.ID, "status", item.Status)
		return nil
	}

	if item.ExternalGameID == nil {
		markItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		markItemFailed(ctx, w.DB, &item, "external game not found", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	slog.DebugContext(ctx, "igdb_match: processing",
		logging.KeyJobItemID, p.JobItemID,
		"title", eg.Title,
		logging.KeySource, eg.Storefront,
		logging.KeyExternalGameID, eg.ID,
		"attempt", job.Attempt,
	)

	// Fast-path: skipped games go straight to UserGameWorker.
	if eg.IsSkipped {
		slog.DebugContext(ctx, "igdb_match: game is skipped, fast-path to user_game_write", logging.KeyJobItemID, p.JobItemID, "title", eg.Title)
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Fast-path: already resolved (manual or prior run).
	if eg.ResolvedIGDBID != nil {
		slog.DebugContext(ctx, "igdb_match: already resolved, fast-path to user_game_write",
			logging.KeyJobItemID, p.JobItemID, "title", eg.Title, "igdb_id", *eg.ResolvedIGDBID)
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Child check: if this row has a parent, inherit, wait, or fail.
	if eg.ParentID != nil {
		var parent models.ExternalGame
		err := w.DB.NewSelect().Model(&parent).Where("id = ?", *eg.ParentID).Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("igdb_match: load parent: %w", err)
		}
		if err == nil && parent.ResolvedIGDBID != nil {
			return w.inheritFromParent(ctx, &item, &eg)
		}

		// The parent is unmatched (or gone): wait only if the wait can end.
		res, err := decideChildWait(ctx, w.DB, item.ID, item.JobID, eg.ID, *eg.ParentID)
		if err != nil {
			return err
		}
		switch res.action {
		case childWait:
			slog.DebugContext(ctx, "igdb_match: parent unresolved, waiting",
				logging.KeyJobItemID, p.JobItemID, "parent_id", *eg.ParentID)
			return nil
		case childFailed:
			slog.InfoContext(ctx, "igdb_match: child cannot wait for its parent; item failed",
				logging.KeyJobItemID, p.JobItemID, "parent_id", *eg.ParentID)
			SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
			return nil
		}
		// childProceed: the facts changed while we looked. Carry on with them.
		eg.IsSkipped = res.isSkipped
		eg.ResolvedIGDBID = res.resolvedIGDBID
		eg.ParentID = res.parentID
		if eg.IsSkipped || eg.ResolvedIGDBID != nil {
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}
		if eg.ParentID != nil && res.parentIGDBID != nil {
			return w.inheritFromParent(ctx, &item, &eg)
		}
		// No longer a child (promoted): searched like any other game.
	}

	// IGDB search.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		platformIDs, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, w.DB, eg.ID)
		if perErr != nil {
			slog.DebugContext(ctx, "igdb_match: platform resolution failed, falling back to unfiltered",
				logging.KeyJobItemID, p.JobItemID, logging.KeyExternalGameID, eg.ID, logging.KeyErr, perErr)
			platformIDs = nil
		}
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, platformIDs)
		if err != nil {
			if job.Attempt >= job.MaxAttempts {
				slog.WarnContext(ctx, "igdb_match: IGDB failed on final attempt, marking pending_review",
					logging.KeyJobItemID, p.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
				return w.toPendingReview(ctx, &item, eg.ID)
			}
			return fmt.Errorf("igdb_match: search failed (will retry): %w", err)
		}

		cands := make([]matching.Candidate, len(candidates))
		for i, c := range candidates {
			cands[i] = matching.Candidate{ID: int32(c.IgdbID), Title: c.Title} //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		}
		decision := matching.Decide(eg.Title, cands)

		slog.DebugContext(ctx, "igdb_match: search results",
			logging.KeyJobItemID, p.JobItemID,
			"title", eg.Title,
			"candidate_count", len(candidates),
			"best_score", decision.BestScore,
			"second_best_score", decision.SecondBest,
			"best_igdb_id", decision.ResolvedID,
		)

		if decision.Confident {
			bestID := decision.ResolvedID
			slog.DebugContext(ctx, "igdb_match: auto-resolved",
				logging.KeyJobItemID, p.JobItemID, "title", eg.Title, "igdb_id", bestID, "score", decision.BestScore)
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "igdb_match: insert game row (auto-resolve)", logging.KeyErr, err, "igdb_id", bestID, logging.Cat(logging.CategoryDB))
			}
			// IS NULL: a rematch that committed during the search wins (#1338);
			// either way the write stage writes whatever match the game has.
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ? AND resolved_igdb_id IS NULL`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.WarnContext(ctx, "igdb_match: auto-resolve external_game", logging.KeyErr, err, logging.KeyExternalGameID, eg.ID, logging.Cat(logging.CategoryDB))
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}

		// Low confidence or tie — store candidates, mark pending_review.
		slog.DebugContext(ctx, "igdb_match: low confidence, marking pending_review",
			logging.KeyJobItemID, p.JobItemID,
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
		return w.toPendingReview(ctx, &item, eg.ID)
	}

	// No IGDB client configured — mark pending_review.
	slog.DebugContext(ctx, "igdb_match: no IGDB client configured, marking pending_review", logging.KeyJobItemID, p.JobItemID, "title", eg.Title)
	return w.toPendingReview(ctx, &item, eg.ID)
}

// toPendingReview puts the item in Pending review, unless the game was matched
// meanwhile (a rematch committed during the search): then the item goes on to
// the write stage. The game is read FOR SHARE first, so a rematch in progress
// is waited for (#1338).
func (w *IGDBMatchWorker) toPendingReview(ctx context.Context, item *models.JobItem, externalGameID string) error {
	matched, marked := false, false
	err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var resolved *int32
		if err := tx.NewRaw(
			`SELECT resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, externalGameID,
		).Scan(ctx, &resolved); err != nil {
			return fmt.Errorf("lock external game: %w", err)
		}
		if resolved != nil {
			matched = true
			return nil
		}
		upd := *item
		upd.Status = models.JobItemStatusPendingReview
		res, err := tx.NewUpdate().Model(&upd).
			Column("status", "igdb_candidates", "match_confidence").
			Where("id = ? AND status = ?", item.ID, models.JobItemStatusPending).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("mark pending_review: %w", err)
		}
		n, _ := res.RowsAffected() //nolint:errcheck // pgdriver always reports RowsAffected
		marked = n > 0
		return nil
	})
	if err != nil {
		return fmt.Errorf("igdb_match: %w", err)
	}
	if matched {
		slog.DebugContext(ctx, "igdb_match: game matched meanwhile, going to the write stage", logging.KeyJobItemID, item.ID)
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}
	if marked {
		item.Status = models.JobItemStatusPendingReview
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
	}
	return nil
}

// inheritFromParent gives a child its parent's match and hands it to the write
// stage. The parent is read FOR SHARE in the same transaction, so a child never
// takes a match a rematch in progress is replacing; and the child only takes it
// while it has none (a rematch sets the whole family's match itself) (#1338).
func (w *IGDBMatchWorker) inheritFromParent(ctx context.Context, item *models.JobItem, eg *models.ExternalGame) error {
	err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var parentMatch *int32
		if err := tx.NewRaw(
			`SELECT resolved_igdb_id FROM external_games WHERE id = ? FOR SHARE`, *eg.ParentID,
		).Scan(ctx, &parentMatch); err != nil {
			return fmt.Errorf("lock parent: %w", err)
		}
		if parentMatch == nil {
			// Not reachable while rematch never unsets a match; the write
			// stage fails the item if the child has none either.
			return nil
		}
		slog.DebugContext(ctx, "igdb_match: child inheriting from resolved parent",
			logging.KeyJobItemID, item.ID, "title", eg.Title, "igdb_id", *parentMatch, "parent_id", *eg.ParentID)
		if err := EnsureGameRow(ctx, tx, *parentMatch, eg.Title); err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ? AND resolved_igdb_id IS NULL`,
			*parentMatch, eg.ID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("apply child inherit: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("igdb_match: inherit from parent: %w", err)
	}
	return w.enqueueUserGame(ctx, item.ID, item.JobID)
}

func (w *IGDBMatchWorker) enqueueUserGame(ctx context.Context, jobItemID, jobID string) error {
	if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, jobItemID, UserGameArgs{JobItemID: jobItemID}); err != nil {
		slog.WarnContext(ctx, "igdb_match: enqueue user_game_write failed", logging.KeyJobItemID, jobItemID, logging.KeyErr, err)
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, jobID)
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
		slog.ErrorContext(ctx, "user_game_write: load job_item", "id", p.JobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return err
	}
	// Correlate every line below to the user-facing job once the item is loaded.
	ctx = logging.WithJobID(ctx, item.JobID)
	// A late or duplicate River job for an item that is no longer pending
	// (finished, in review, or cancelled) must change nothing (#1339).
	if item.Status != models.JobItemStatusPending {
		slog.DebugContext(ctx, "user_game_write: item is not pending, nothing to do", logging.KeyJobItemID, item.ID, "status", item.Status)
		return nil
	}

	if item.ExternalGameID == nil {
		markItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		markItemFailed(ctx, w.DB, &item, "external game not found", "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Skipped games: record the outcome, mark the item skipped, and check completion.
	if eg.IsSkipped {
		w.finishSkipped(ctx, &item, &eg)
		return nil
	}

	// Manual resolution propagation: job_item has resolved_igdb_id but external_game doesn't.
	// Only an unmatched game takes it: a rematch that committed meanwhile wins.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // IGDB game IDs are positive and fit within int32 (games.id is int32)
		if err := EnsureGameRow(ctx, w.DB, igdbID, eg.Title); err != nil {
			slog.WarnContext(ctx, "user_game_write: insert game row (manual resolve)", logging.KeyErr, err, "igdb_id", igdbID, logging.Cat(logging.CategoryDB))
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ? AND resolved_igdb_id IS NULL`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "user_game_write: apply manual resolution", logging.KeyErr, err, logging.KeyExternalGameID, eg.ID, logging.Cat(logging.CategoryDB))
		}
	}

	// The library write, its diff rows and the item's completion commit
	// together. The write re-reads the match and skip flag under the game's
	// row lock, so a rematch that committed after we loaded eg wins.
	var lw LibraryWrite
	err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		if lw, err = WriteExternalGameToLibrary(ctx, tx, item.UserID, eg.ID); err != nil {
			return err
		}
		if err := InsertLibraryWriteChanges(ctx, tx, item.JobID, item.UserID, lw, lw.Result.Created); err != nil {
			return err
		}
		res, err := tx.NewRaw(
			`UPDATE job_items SET status = 'completed', processed_at = now() WHERE id = ? AND status = 'pending'`, item.ID,
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("complete item: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // pgdriver always reports RowsAffected
			return errItemNotPending
		}
		return nil
	})
	switch {
	case errors.Is(err, errItemNotPending):
		slog.DebugContext(ctx, "user_game_write: item stopped being pending during the write, nothing written", logging.KeyJobItemID, item.ID)
		return nil
	case errors.Is(err, ErrExternalGameSkipped):
		w.finishSkipped(ctx, &item, &eg)
		return nil
	case err != nil:
		markItemFailed(ctx, w.DB, &item, err.Error(), "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Immediate metadata fetch for a game with no description yet. Checked
	// here so an instance without IGDB does not queue a no-op per game.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		EnqueueImmediateMetadataFetch(ctx, w.DB, w.RiverClient, lw.IGDBID)
	}

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
				if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, child.JobItemID, IGDBMatchArgs{JobItemID: child.JobItemID}); err != nil {
					slog.WarnContext(ctx, "user_game_write: enqueue sibling Stage 2",
						logging.KeyErr, err, "child_eg_id", child.ExternalGameID, logging.KeyJobItemID, child.JobItemID)
				}
			}
		}
	}

	SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
	return nil
}

// errItemNotPending rolls back a write whose item a Skip or a cancel finished
// meanwhile.
var errItemNotPending = errors.New("job item is no longer pending")

// finishSkipped records a skipped game in the Job's diff, marks the item
// skipped, and checks completion.
func (w *UserGameWorker) finishSkipped(ctx context.Context, item *models.JobItem, eg *models.ExternalGame) {
	if _, err := w.DB.NewRaw(
		`UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "user_game_write: update external_game updated_at (skipped)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	// NOT EXISTS: this item can run twice across a restart (see the dispatch's
	// 'skipped' insert).
	if _, err := w.DB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 SELECT ?, ?, ?, ?, 'skipped', ?, now()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM changes WHERE job_id = ? AND external_game_id = ? AND change_type = 'skipped')`,
		uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title, item.JobID, eg.ID,
	).Exec(ctx); err != nil {
		slog.WarnContext(ctx, "user_game_write: insert sync_change (skipped)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	markItemSkipped(ctx, w.DB, item, "process_sync_item: markItemSkipped")
	SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
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
func SyncCheckJobCompletion(ctx context.Context, db *bun.DB, riverClient *river.Client[pgx.Tx], jobID string) {
	ctx = logging.WithJobID(ctx, jobID)
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

	// The Job is finalized: nothing will run this check for it again, so the
	// notifications, diff, metrics and store-link trigger below must not be
	// lost to a shutdown that cancels ctx right here (#1302).
	ctx = context.WithoutCancel(ctx)

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
	emitSyncDiff(ctx, db, jobID, userID, storefrontDisplayName(ctx, db, storefront))

	// Sync-outcome metrics (best-effort, bounded cardinality: source + status/outcome).
	// Skip when storefront is empty (lookup failure) so we never create a polluting
	// source="" time series.
	if storefront != "" {
		outcomeStatus := "completed"
		if failedCount > 0 {
			outcomeStatus = "completed_with_errors"
		}
		observability.RecordSyncOutcome(ctx, storefront, outcomeStatus)
		if completed, failed, skipped, ok := syncJobItemStatusCounts(ctx, db, jobID); ok {
			observability.RecordSyncItems(ctx, storefront, "completed", completed)
			observability.RecordSyncItems(ctx, storefront, "failed", failed)
			observability.RecordSyncItems(ctx, storefront, "skipped", skipped)
		}
	}

	// Kick off scoped, incremental store-link enrichment for this storefront.
	// Best-effort: a missing River client (e.g. in tests) simply skips it.
	if riverClient != nil && userID != "" && storefront != "" {
		if _, err := riverClient.Insert(ctx, StoreLinkRefreshDispatchArgs{
			UserID: userID, Storefront: storefront, Force: false,
		}, nil); err != nil {
			slog.WarnContext(ctx, "sync: enqueue store_link_refresh", logging.KeyErr, err)
		}
	}
}

// syncJobItemStatusCounts returns the per-status item counts for a finalized job.
// Used for the nexorious_sync_items_total business metric. Returns ok=false on a
// query error so callers can skip metrics (best-effort, never blocks the job).
func syncJobItemStatusCounts(ctx context.Context, db *bun.DB, jobID string) (completed, failed, skipped int64, ok bool) {
	var rows []struct {
		Status string `bun:"status"`
		N      int64  `bun:"n"`
	}
	if err := db.NewRaw(
		`SELECT status, COUNT(*) AS n FROM job_items WHERE job_id = ? GROUP BY status`,
		jobID,
	).Scan(ctx, &rows); err != nil {
		slog.WarnContext(ctx, "sync: count job item statuses for metrics", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return 0, 0, 0, false
	}
	for _, r := range rows {
		switch r.Status {
		case "completed":
			completed = r.N
		case "failed":
			failed = r.N
		case "skipped":
			skipped = r.N
		}
	}
	return completed, failed, skipped, true
}

// syncJobUserAndStorefront fetches the owning user_id and storefront (source)
// for a job. Returns ("","") on error.
func syncJobUserAndStorefront(ctx context.Context, db *bun.DB, jobID string) (userID, storefront string) {
	var row struct {
		UserID string `bun:"user_id"`
		Source string `bun:"source"`
	}
	if err := db.NewRaw(`SELECT user_id, source FROM jobs WHERE id = ?`, jobID).Scan(ctx, &row); err != nil {
		slog.WarnContext(ctx, "sync: lookup job user/storefront", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return "", ""
	}
	return row.UserID, row.Source
}

// storefrontDisplayName resolves a storefront source slug (e.g. "steam",
// "playstation-store") to its human-readable display name from the storefronts
// reference table. Returns "" when the slug is empty, unknown, or on error —
// callers fall back to a generic label.
func storefrontDisplayName(ctx context.Context, db *bun.DB, slug string) string {
	if slug == "" {
		return ""
	}
	var name string
	if err := db.NewRaw(`SELECT display_name FROM storefronts WHERE name = ?`, slug).Scan(ctx, &name); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.WarnContext(ctx, "sync: lookup storefront display_name", "slug", slug, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
		return ""
	}
	return name
}

// emitSyncDiff emits sync.diff iff changes rows exist for the job. storefront is
// the resolved storefront display name (may be empty) carried in the payload so
// the notification names its source.
func emitSyncDiff(ctx context.Context, db *bun.DB, jobID, userID, storefront string) {
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
		slog.WarnContext(ctx, "sync: load changes for diff notify", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
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
		Payload:  notify.SyncDiffPayload{Storefront: storefront, Added: added, Removed: removed, JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeSyncDiff,
	})
}
