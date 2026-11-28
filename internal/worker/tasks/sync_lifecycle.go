package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/observability"
)

// syncKind is the sync Job kind on the lifecycle module (#1268): a dispatch
// that adds the storefront library's games in batches, then match and write
// per game. Whatever the module queues by itself for a sync item (Retry,
// recovery) is an igdb_match, whose fast paths
// send an already matched or skipped game on to the write stage.
func syncKind() *joblifecycle.Kind {
	return &joblifecycle.Kind{
		JobType:            models.JobTypeSync,
		DispatchRiverKinds: []string{DispatchSyncArgs{}.Kind()},
		ItemRiverKinds:     []string{IGDBMatchArgs{}.Kind(), UserGameArgs{}.Kind()},
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return IGDBMatchArgs{JobItemID: it.ID}, nil
		},
		DispatchArgs: func(j *models.Job) (river.JobArgs, error) {
			return DispatchSyncArgs{JobID: j.ID, UserID: j.UserID, Storefront: j.Source}, nil
		},
		// One active sync per user and storefront.
		DuplicateKey: func(j *models.Job) (joblifecycle.DupKey, bool) {
			return joblifecycle.DupKey{Source: j.Source, UserID: j.UserID}, true
		},
		HasEffects: func(joblifecycle.Outcome) bool { return true },
		Effects:    syncEffects,
		Observer:   syncObserver{},
	}
}

// syncFailure is the kind data of a failed sync. A Job failed by recovery
// carries none.
type syncFailure struct {
	// Credentials: the sync failed for its credentials.
	Credentials bool `json:"credentials,omitempty"`
	// Notify: this failure set credentials_error on a store that was
	// healthy, so the user is told. A repeat, or a failure bound to a
	// sign-in the user has since replaced (#1347), notifies nobody.
	Notify bool `json:"notify,omitempty"`
}

// syncObserver records the sync metrics: items as they end, and a Job's
// outcome on its first ending. A cancelled sync records no outcome.
type syncObserver struct{}

func (syncObserver) ItemEnded(ctx context.Context, job models.Job, status models.JobItemStatus) {
	observability.RecordSyncItems(ctx, job.Source, string(status), 1)
}

func (syncObserver) JobEnded(ctx context.Context, job models.Job, outcome models.JobStatus, failedItems int) {
	switch outcome {
	case models.JobStatusFailed:
		observability.RecordSyncOutcome(ctx, job.Source, "failed")
	case models.JobStatusCompleted:
		if failedItems > 0 {
			observability.RecordSyncOutcome(ctx, job.Source, "completed_with_errors")
		} else {
			observability.RecordSyncOutcome(ctx, job.Source, "completed")
		}
	}
}

// syncEffects are the sync kind's ending effects. Each is idempotent for
// its Job: notifications carry a dedup key per Job and type.
func syncEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	switch e.Outcome {
	case joblifecycle.OutcomeCompleted:
		return syncCompletedEffects(ctx, db, e)
	case joblifecycle.OutcomeFailed:
		return syncFailedEffects(ctx, db, e)
	case joblifecycle.OutcomeAwaitingReview:
		var n int
		if err := db.NewRaw(
			`SELECT count(*) FROM job_items WHERE job_id = ? AND status = ?`, e.Job.ID, models.JobItemStatusPendingReview,
		).Scan(ctx, &n); err != nil {
			return fmt.Errorf("count items in review: %w", err)
		}
		return emitAll(ctx, db, notify.EmitParams{
			Type: notify.TypeSyncNeedsReview, Scope: notify.ScopeUser, ActorUserID: e.Job.UserID,
			Payload:  notify.SyncNeedsReviewPayload{Storefront: e.Job.Source, Count: n, JobID: e.Job.ID},
			DedupKey: e.Job.ID + ":" + notify.TypeSyncNeedsReview,
		})
	}
	return nil
}

// syncCompletedEffects sends sync completed (or completed with errors) and the
// sync diff, then queues the store-link refresh for the storefront.
func syncCompletedEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	job := e.Job
	failed, err := joblifecycle.FailedItemCount(ctx, db, job.ID)
	if err != nil {
		return err
	}
	events := make([]notify.EmitParams, 0, 2)
	if failed > 0 {
		events = append(events, notify.EmitParams{
			Type: notify.TypeSyncCompletedWithErrors, Scope: notify.ScopeUser, ActorUserID: job.UserID,
			Payload:  notify.SyncCompletedWithErrorsPayload{Storefront: job.Source, Failed: failed, JobID: job.ID},
			DedupKey: job.ID + ":" + notify.TypeSyncCompletedWithErrors,
		})
	} else {
		events = append(events, notify.EmitParams{
			Type: notify.TypeSyncCompleted, Scope: notify.ScopeUser, ActorUserID: job.UserID,
			Payload:  notify.SyncCompletedPayload{Storefront: job.Source, JobID: job.ID},
			DedupKey: job.ID + ":" + notify.TypeSyncCompleted,
		})
	}
	diff, err := syncDiff(ctx, db, job)
	if err != nil {
		return err
	}
	if diff != nil {
		events = append(events, *diff)
	}
	if err := emitAll(ctx, db, events...); err != nil {
		return err
	}
	// Scoped, incremental store-link enrichment for this storefront. Its
	// dispatch creates its own Job; run again it finds nothing new to do.
	if err := e.Insert(ctx, StoreLinkRefreshDispatchArgs{UserID: job.UserID, Storefront: job.Source}); err != nil {
		return fmt.Errorf("queue store-link refresh: %w", err)
	}
	return nil
}

// syncFailedEffects sends sync.failed or sync.auth_expired. A credentials
// failure notifies only when it set the flag on a healthy store. Any other
// failure (also one failed by recovery, which carries no kind data) notifies
// once per outage: only the first failure since the store's last successful
// sync (#1344).
func syncFailedEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	job := e.Job
	var data syncFailure
	if len(e.KindData) > 0 {
		if err := json.Unmarshal(e.KindData, &data); err != nil {
			return fmt.Errorf("decode kind data: %w", err)
		}
	}
	if data.Credentials {
		if !data.Notify {
			return nil
		}
		subscribed, err := userSubscribed(ctx, db, job.UserID, notify.TypeSyncAuthExpired)
		if err != nil {
			return err
		}
		if subscribed {
			return emitAll(ctx, db, notify.EmitParams{
				Type: notify.TypeSyncAuthExpired, Scope: notify.ScopeUser, ActorUserID: job.UserID,
				Payload:  notify.SyncAuthExpiredPayload{Storefront: job.Source},
				DedupKey: job.ID + ":" + notify.TypeSyncAuthExpired,
			})
		}
		// Not subscribed to the actionable event: sync.failed, so nobody is
		// left silent.
		return emitAll(ctx, db, syncFailedEvent(job, credentialsErrorMessage))
	}
	repeated, err := previousSyncFailure(ctx, db, job.ID)
	if err != nil {
		return err
	}
	if repeated {
		slog.InfoContext(ctx, "sync: store is still failing; sync.failed already sent for this outage", logging.KeySource, job.Source)
		return nil
	}
	var msg string
	if job.ErrorMessage != nil {
		msg = *job.ErrorMessage
	}
	return emitAll(ctx, db, syncFailedEvent(job, msg))
}

func syncFailedEvent(job *models.Job, msg string) notify.EmitParams {
	return notify.EmitParams{
		Type: notify.TypeSyncFailed, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  notify.SyncFailedPayload{Storefront: job.Source, Error: msg, JobID: job.ID},
		DedupKey: job.ID + ":" + notify.TypeSyncFailed,
	}
}

// emitAll writes the events and their deliveries in one transaction, so a
// retried effect never finds an event without its delivery.
func emitAll(ctx context.Context, db *bun.DB, events ...notify.EmitParams) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, p := range events {
			if err := notify.EmitTx(ctx, tx, p); err != nil {
				return fmt.Errorf("emit %s: %w", p.Type, err)
			}
		}
		return nil
	})
}

// outageFailure is the outage rule that sync.failed and sync.recovered share
// (#1344, #1385): a sync of the store, previous_job, that failed for anything
// but its credentials since the store's last successful fetch (cfg's
// last_fetched_at, which Reset sync data leaves alone, unlike last_synced_at:
// #1405). The error text need not match: messages often carry
// varying details, so matching would notify on every attempt again.
// Credential failures do not count, since they send their own notice. The
// caller joins previous_job (with the store's user_id and source) and cfg, and
// binds credentialsErrorMessage.
const outageFailure = `previous_job.job_type = '` + models.JobTypeSync + `'
	AND previous_job.status = '` + string(models.JobStatusFailed) + `'
	AND previous_job.error_message IS DISTINCT FROM ?
	AND (cfg.last_fetched_at IS NULL OR previous_job.completed_at > cfg.last_fetched_at)`

// previousSyncFailure reports whether an earlier sync of the same store failed
// in this outage (outageFailure), which means the outage has already been
// notified. Comparing completion times ignores newer jobs.
func previousSyncFailure(ctx context.Context, db bun.IDB, jobID string) (bool, error) {
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
				AND previous_job.completed_at < current_job.completed_at
				AND `+outageFailure+`
		)`, jobID, credentialsErrorMessage).Scan(ctx, &repeated); err != nil {
		return false, fmt.Errorf("check previous sync failure: %w", err)
	}
	return repeated, nil
}

// emitSyncRecovered sends sync.recovered when the store is in an outage that
// sent sync.failed (outageFailure), so the user learns it is over (#1385). It
// runs in the transaction that records the successful fetch, before
// last_fetched_at moves past the outage. A store that failed only for its
// credentials sent no sync.failed, and the user's reconnect ended it, so it
// sends nothing.
func emitSyncRecovered(ctx context.Context, tx bun.Tx, p DispatchSyncArgs) error {
	var outage bool
	if err := tx.NewRaw(`
		SELECT EXISTS (
			SELECT 1
			FROM jobs AS previous_job
			LEFT JOIN user_sync_configs AS cfg ON cfg.user_id = previous_job.user_id
				AND cfg.storefront = previous_job.source
			WHERE previous_job.user_id = ? AND previous_job.source = ?
				AND previous_job.id <> ?
				AND `+outageFailure+`
		)`, p.UserID, p.Storefront, p.JobID, credentialsErrorMessage).Scan(ctx, &outage); err != nil {
		return fmt.Errorf("check sync outage: %w", err)
	}
	if !outage {
		return nil
	}
	if err := notify.EmitTx(ctx, tx, notify.EmitParams{
		Type: notify.TypeSyncRecovered, Scope: notify.ScopeUser, ActorUserID: p.UserID,
		Payload:  notify.SyncRecoveredPayload{Storefront: p.Storefront, JobID: p.JobID},
		DedupKey: p.JobID + ":" + notify.TypeSyncRecovered,
	}); err != nil {
		return fmt.Errorf("emit %s: %w", notify.TypeSyncRecovered, err)
	}
	return nil
}

// userSubscribed reports whether the user is subscribed to eventType.
func userSubscribed(ctx context.Context, db bun.IDB, userID, eventType string) (bool, error) {
	var subscribed bool
	if err := db.NewRaw(
		`SELECT EXISTS(SELECT 1 FROM notification_subscriptions WHERE user_id = ? AND event_type = ?)`,
		userID, eventType,
	).Scan(ctx, &subscribed); err != nil {
		return false, fmt.Errorf("subscription check: %w", err)
	}
	return subscribed, nil
}

// storefrontDisplayName resolves a storefront slug (e.g. "steam",
// "playstation-store") to its display name from the storefronts reference
// table, or "" when the slug is unknown.
func storefrontDisplayName(ctx context.Context, db bun.IDB, slug string) (string, error) {
	if slug == "" {
		return "", nil
	}
	var name string
	if err := db.NewRaw(`SELECT display_name FROM storefronts WHERE name = ?`, slug).Scan(ctx, &name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("lookup storefront display_name: %w", err)
	}
	return name, nil
}

// syncDiff returns the sync.diff event for the Job's added and removed
// games, or nil when there are none.
func syncDiff(ctx context.Context, db bun.IDB, job *models.Job) (*notify.EmitParams, error) {
	var rows []struct {
		ChangeType enum.ChangeType `bun:"change_type"`
		Title      string          `bun:"title"`
		Platforms  string          `bun:"platforms"`
	}
	// One line per game (#1443). An 'added' row is one per library entry; its
	// line shows the library entry's title and the platforms of every edition
	// the Job wrote to that entry. A 'removed' row is one per set of editions
	// (the sweep writes it for the last edition to leave); its line shows the
	// platforms of every edition of the set as they are now. The set, not
	// externalgame.SameGame: the sweep counts a same-title game without a
	// decision as the same game only to hold the row back while it is still
	// available; it is not yet one of the game's editions.
	if err := db.NewRaw(
		`SELECT sc.change_type,
		        COALESCE(g.title, sc.title) AS title,
		        COALESCE((
		          SELECT string_agg(DISTINCT egp.platform, ',' ORDER BY egp.platform)
		            FROM external_game_platforms egp
		           WHERE egp.external_game_id = sc.external_game_id
		              OR (sc.change_type = ? AND egp.external_game_id IN (
		                    SELECT l.external_game_id FROM changes l
		                     WHERE l.job_id = sc.job_id AND l.user_game_id = sc.user_game_id
		                       AND l.change_type IN (?)))
		              OR (sc.change_type = ? AND egp.external_game_id IN (
		                    SELECT o.id FROM external_games me
		                      JOIN external_games o ON `+externalgame.SameEdition("o", "me")+`
		                     WHERE me.id = sc.external_game_id))
		        ), '') AS platforms
		   FROM changes sc
		   LEFT JOIN user_games ug ON ug.id = sc.user_game_id AND sc.change_type = ?
		   LEFT JOIN games g ON g.id = ug.game_id
		  WHERE sc.job_id = ? AND sc.change_type IN (?, ?)
		  ORDER BY sc.created_at, sc.id`,
		enum.ChangeAdded, bun.List(enum.LibraryChangeTypeStrings()), enum.ChangeRemoved,
		enum.ChangeAdded, job.ID, enum.ChangeAdded, enum.ChangeRemoved,
	).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("load changes for diff: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	name, err := storefrontDisplayName(ctx, db, job.Source)
	if err != nil {
		return nil, err
	}
	added := []notify.DiffGame{}
	removed := []notify.DiffGame{}
	for _, r := range rows {
		platforms := []string{}
		if r.Platforms != "" {
			platforms = strings.Split(r.Platforms, ",")
		}
		entry := notify.DiffGame{Title: r.Title, Platforms: platforms}
		if r.ChangeType == enum.ChangeAdded {
			added = append(added, entry)
		} else {
			removed = append(removed, entry)
		}
	}
	return &notify.EmitParams{
		Type: notify.TypeSyncDiff, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  notify.SyncDiffPayload{Storefront: name, Added: added, Removed: removed, JobID: job.ID},
		DedupKey: job.ID + ":" + notify.TypeSyncDiff,
	}, nil
}
