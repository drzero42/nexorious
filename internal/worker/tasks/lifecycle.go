package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
)

// lifecycleJobTypes are the job types whose Jobs internal/joblifecycle owns.
// The old recovery paths (the orphan rescue, the stale sweep) leave them
// alone. Stages 2 and 3 of #1265 add the import and sync kinds.
var lifecycleJobTypes = []string{
	models.JobTypeMetadataRefresh,
	models.JobTypeStoreLinkRefresh,
	models.JobTypeExport,
}

// RegisterLifecycleKinds registers the Job kinds that are on the lifecycle
// module.
func RegisterLifecycleKinds(m *joblifecycle.Module) {
	m.Register(&joblifecycle.Kind{
		JobType:            models.JobTypeMetadataRefresh,
		DispatchRiverKinds: []string{MetadataRefreshDispatchArgs{}.Kind()},
		ItemRiverKinds:     []string{MetadataRefreshItemArgs{}.Kind()},
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return MetadataRefreshItemArgs{JobItemID: it.ID}, nil
		},
		DispatchArgs: func(j *models.Job) (river.JobArgs, error) {
			return MetadataRefreshDispatchArgs{JobID: j.ID}, nil
		},
		// One active metadata refresh in all.
		DuplicateKey: func(*models.Job) (joblifecycle.DupKey, bool) {
			return joblifecycle.DupKey{Source: models.JobSourceSystem}, true
		},
	})
	m.Register(&joblifecycle.Kind{
		JobType:            models.JobTypeStoreLinkRefresh,
		DispatchRiverKinds: []string{StoreLinkRefreshDispatchArgs{}.Kind()},
		ItemRiverKinds:     []string{StoreLinkRefreshItemArgs{}.Kind()},
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return StoreLinkRefreshItemArgs{JobItemID: it.ID}, nil
		},
		// Source system is a forced, all-users refresh; a storefront source
		// is that user's storefront, not forced. Those are the only two
		// combinations ever created.
		DispatchArgs: func(j *models.Job) (river.JobArgs, error) {
			if j.Source == models.JobSourceSystem {
				return StoreLinkRefreshDispatchArgs{JobID: j.ID, Force: true}, nil
			}
			return StoreLinkRefreshDispatchArgs{JobID: j.ID, UserID: j.UserID, Storefront: j.Source}, nil
		},
		// Source system: one in all. A storefront source: one per user.
		DuplicateKey: func(j *models.Job) (joblifecycle.DupKey, bool) {
			if j.Source == models.JobSourceSystem {
				return joblifecycle.DupKey{Source: j.Source}, true
			}
			return joblifecycle.DupKey{Source: j.Source, UserID: j.UserID}, true
		},
	})
	m.Register(&joblifecycle.Kind{
		JobType:            models.JobTypeExport,
		DispatchRiverKinds: []string{ExportJSONArgs{}.Kind(), ExportCSVArgs{}.Kind()},
		ItemArgs: func(*models.Job, *models.JobItem) (river.JobArgs, error) {
			return nil, errors.New("an export has no items")
		},
		DispatchArgs: func(j *models.Job) (river.JobArgs, error) {
			switch j.Source {
			case models.JobSourceNexorious:
				return ExportJSONArgs{JobID: j.ID}, nil
			case models.JobSourceCSV:
				return ExportCSVArgs{JobID: j.ID}, nil
			}
			return nil, fmt.Errorf("unknown export source %q", j.Source)
		},
		// Starting a second export is allowed.
		DuplicateKey: func(*models.Job) (joblifecycle.DupKey, bool) { return joblifecycle.DupKey{}, false },
		HasEffects: func(o joblifecycle.Outcome) bool {
			return o == joblifecycle.OutcomeCompleted || o == joblifecycle.OutcomeFailed
		},
		Effects: exportEffects,
	})
}

// exportEffects sends the export completed / failed notification. The event
// and its delivery are written in one transaction, so a retried effect never
// finds the event without its delivery; the dedup key makes it idempotent.
func exportEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	p := notify.EmitParams{Scope: notify.ScopeUser, ActorUserID: e.Job.UserID}
	switch e.Outcome {
	case joblifecycle.OutcomeCompleted:
		var path string
		if e.Job.FilePath != nil {
			path = *e.Job.FilePath
		}
		p.Type = notify.TypeExportCompleted
		p.Payload = notify.ExportCompletedPayload{JobID: e.Job.ID, FilePath: path}
	case joblifecycle.OutcomeFailed:
		var msg string
		if e.Job.ErrorMessage != nil {
			msg = *e.Job.ErrorMessage
		}
		p.Type = notify.TypeExportFailed
		p.Payload = notify.ExportFailedPayload{JobID: e.Job.ID, Error: msg}
	default:
		return nil
	}
	p.DedupKey = e.Job.ID + ":" + p.Type
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return notify.EmitTx(ctx, tx, p)
	})
}

// exitOrRetry turns the error of a lifecycle call into what a worker
// returns: its own cancelled context first (River puts the job back or ends
// it), nil for a refusal (nothing left to do), else the error, so River
// retries.
func exitOrRetry(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if joblifecycle.IsRefusal(err) {
		slog.InfoContext(ctx, "job lifecycle: refused, nothing to do", logging.KeyErr, err)
		return nil
	}
	return err
}

// lastAttempt reports whether River will not run this job again on error.
func lastAttempt(rj *rivertype.JobRow) bool { return rj.Attempt >= rj.MaxAttempts }

// loadOwnedItem loads the item a lifecycle item worker was queued for and
// checks the worker still owns it. done=true means it has nothing to do (the
// item is gone or owned by someone else); err is a database error to return.
func loadOwnedItem(ctx context.Context, db *bun.DB, lc *joblifecycle.Module, itemID string, riverJobID int64) (item *models.JobItem, done bool, err error) {
	var it models.JobItem
	if err := db.NewSelect().Model(&it).Where("id = ?", itemID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, true, nil
		}
		return nil, false, exitOrRetry(ctx, fmt.Errorf("load job_item: %w", err))
	}
	if err := lc.Owns(ctx, itemID, riverJobID); err != nil {
		return nil, true, exitOrRetry(ctx, err)
	}
	return &it, false, nil
}

// failItem fails an item its worker owns, with msg.
func failItem(ctx context.Context, lc *joblifecycle.Module, itemID string, riverJobID int64, msg string) error {
	return exitOrRetry(ctx, lc.FinishItem(ctx, itemID, riverJobID, joblifecycle.ItemResult{
		Status: models.JobItemStatusFailed, Message: msg,
	}))
}

// maintDedupKey keys an admin maintenance notification so a dispatch that is
// run again does not send it twice. Keys are unique across all events.
func maintDedupKey(id any, typ string) string { return fmt.Sprintf("%v:%s", id, typ) }
