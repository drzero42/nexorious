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
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/observability"
)

// syncKind is the sync Job kind on the lifecycle module (#1268): a dispatch
// that adds the storefront library's games in batches, then match and write
// per game. Whatever the module queues by itself for a sync item (Retry,
// recovery, an item that stops waiting) is an igdb_match, whose fast paths
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
		WaitsOn:    syncWaitsOn,
	}
}

// syncWaitsOn is the sync kind's waiting hook: an item whose game currently
// has a parent (a Child game) waits on the parent game's item in the same
// Job, or on nothing when the parent has none.
func syncWaitsOn(ctx context.Context, tx bun.Tx, q joblifecycle.WaitQuery) ([]joblifecycle.WaitEdge, error) {
	query := `SELECT ji.id AS item_id, pi.id AS awaited_item_id
		FROM job_items ji
		JOIN external_games eg ON eg.id = ji.external_game_id
		LEFT JOIN job_items pi ON pi.job_id = ji.job_id AND pi.external_game_id = eg.parent_id
		WHERE ji.job_id = ? AND eg.parent_id IS NOT NULL`
	args := []any{q.JobID}
	if len(q.Items) > 0 {
		query += ` AND ji.id IN (?)`
		args = append(args, bun.List(q.Items))
	}
	if len(q.Awaited) > 0 {
		query += ` AND pi.id IN (?)`
		args = append(args, bun.List(q.Awaited))
	}
	var edges []joblifecycle.WaitEdge
	if err := tx.NewRaw(query, args...).Scan(ctx, &edges); err != nil {
		return nil, fmt.Errorf("sync waits on: %w", err)
	}
	return edges, nil
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

// syncObserver records the sync metrics: items as they finish, and a Job's
// outcome on its first finish. A cancelled sync records no outcome.
type syncObserver struct{}

func (syncObserver) ItemFinished(ctx context.Context, job models.Job, status string) {
	observability.RecordSyncItems(ctx, job.Source, status, 1)
}

func (syncObserver) JobFinished(ctx context.Context, job models.Job, outcome string, failedItems int) {
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

// syncEffects are the sync kind's finishing effects. Each is idempotent for
// its Job: notifications carry a dedup key per Job and type.
func syncEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	switch e.Outcome {
	case joblifecycle.OutcomeCompleted:
		return syncCompletedEffects(ctx, db, e)
	case joblifecycle.OutcomeFailed:
		return syncFailedEffects(ctx, db, e)
	case joblifecycle.OutcomeAwaitingReview:
		var n int
		if err := db.NewRaw(`SELECT count(*) FROM job_items WHERE job_id = ? AND status = 'pending_review'`, e.Job.ID).Scan(ctx, &n); err != nil {
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
	var failed int
	if err := db.NewRaw(`SELECT count(*) FROM job_items WHERE job_id = ? AND status = 'failed'`, job.ID).Scan(ctx, &failed); err != nil {
		return fmt.Errorf("count failed items: %w", err)
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

// previousSyncFailure reports whether an earlier sync of the same store failed
// since its last successful sync, which means this outage has already been
// notified. The error text need not match: messages often carry varying
// details, so matching would notify on every attempt again. Credential
// failures do not count, since they send their own notice. Comparing
// completion times ignores newer jobs.
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
				AND previous_job.job_type = 'sync' AND previous_job.status = 'failed'
				AND previous_job.error_message IS DISTINCT FROM ?
				AND previous_job.completed_at < current_job.completed_at
				AND (cfg.last_synced_at IS NULL OR previous_job.completed_at > cfg.last_synced_at)
		)`, jobID, credentialsErrorMessage).Scan(ctx, &repeated); err != nil {
		return false, fmt.Errorf("check previous sync failure: %w", err)
	}
	return repeated, nil
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
		job.ID,
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
		if r.ChangeType == "added" {
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
