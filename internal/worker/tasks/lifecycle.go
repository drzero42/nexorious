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

// msgImportUploadLost fails an import whose upload handler died before it had
// created every item. An import's items are created with its Job, so its
// dispatch cannot be run again.
const msgImportUploadLost = "interrupted while the upload was being read; start the import again"

// importDuplicateKey allows one active import per user and source.
func importDuplicateKey(j *models.Job) (joblifecycle.DupKey, bool) {
	return joblifecycle.DupKey{Source: j.Source, UserID: j.UserID}, true
}

func importHasEffects(o joblifecycle.Outcome) bool {
	return o == joblifecycle.OutcomeCompleted || o == joblifecycle.OutcomeFailed
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
	// The nexorious import: one stage per game, plus the pools item.
	m.Register(&joblifecycle.Kind{
		JobType:        models.JobTypeImport,
		Sources:        []string{models.JobSourceNexorious},
		ItemRiverKinds: []string{ImportItemArgs{}.Kind()},
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return ImportItemArgs{JobItemID: it.ID}, nil
		},
		DispatchLostMessage: msgImportUploadLost,
		DuplicateKey:        importDuplicateKey,
		HasEffects:          importHasEffects,
		Effects:             importEffects,
	})
	// The generic import: every other import source, including ones that
	// have since been removed from the registry, so their Jobs are still
	// completed, cancelled and recovered. Match, then finalize.
	m.Register(&joblifecycle.Kind{
		JobType:        models.JobTypeImport,
		ItemRiverKinds: []string{ImportMatchArgs{}.Kind(), ImportFinalizeArgs{}.Kind()},
		// An item that has its IGDB game (Resolved, or matched before it
		// failed) goes to finalize; one that failed in match goes to match.
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			if it.ResolvedIGDBID != nil {
				return ImportFinalizeArgs{JobItemID: it.ID}, nil
			}
			return ImportMatchArgs{JobItemID: it.ID}, nil
		},
		DispatchLostMessage: msgImportUploadLost,
		DuplicateKey:        importDuplicateKey,
		HasEffects:          importHasEffects,
		Effects:             importEffects,
	})
}

// importEffects sends the import notification and, for a nexorious import on
// its Job's first finish, applies the imported pools. The notification goes
// first so a pools error that never goes away cannot hold it back. Pools are
// applied on the first finish only: applying the file's pools again after a
// Retry would recreate pools the user deleted since.
func importEffects(ctx context.Context, db *bun.DB, e joblifecycle.Effect) error {
	p := notify.EmitParams{Scope: notify.ScopeUser, ActorUserID: e.Job.UserID}
	switch e.Outcome {
	case joblifecycle.OutcomeCompleted:
		var failed int
		if err := db.NewRaw(`SELECT count(*) FROM job_items WHERE job_id = ? AND status = 'failed'`, e.Job.ID).Scan(ctx, &failed); err != nil {
			return fmt.Errorf("count failed items: %w", err)
		}
		if failed > 0 {
			p.Type = notify.TypeImportFailed
			p.Payload = notify.ImportFailedPayload{JobID: e.Job.ID, Failed: failed, Error: fmt.Sprintf("%d item(s) failed to import", failed)}
		} else {
			p.Type = notify.TypeImportCompleted
			p.Payload = notify.ImportCompletedPayload{JobID: e.Job.ID}
		}
	case joblifecycle.OutcomeFailed:
		var msg string
		if e.Job.ErrorMessage != nil {
			msg = *e.Job.ErrorMessage
		}
		p.Type = notify.TypeImportFailed
		p.Payload = notify.ImportFailedPayload{JobID: e.Job.ID, Error: msg}
	default:
		return nil
	}
	p.DedupKey = e.Job.ID + ":" + p.Type
	if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return notify.EmitTx(ctx, tx, p)
	}); err != nil {
		return fmt.Errorf("emit %s: %w", p.Type, err)
	}
	if e.Outcome == joblifecycle.OutcomeCompleted && e.FirstFinish && e.Job.Source == models.JobSourceNexorious {
		return applyImportedPools(ctx, db, e.Job.ID, e.Job.UserID)
	}
	return nil
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
