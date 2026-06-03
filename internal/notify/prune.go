package notify

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"
)

// PruneEventsArgs is the River periodic job payload.
type PruneEventsArgs struct {
	RetentionDays int `json:"retention_days"`
}

// Kind implements river.JobArgs.
func (PruneEventsArgs) Kind() string { return "prune_events" }

// PruneEventsWorker deletes events older than the retention window. On
// completion it emits admin.maintenance.completed (or .failed on error).
type PruneEventsWorker struct {
	river.WorkerDefaults[PruneEventsArgs]
	DB *bun.DB
}

// Work runs the prune and emits a maintenance event.
func (w *PruneEventsWorker) Work(ctx context.Context, job *river.Job[PruneEventsArgs]) error {
	days := job.Args.RetentionDays
	if days <= 0 {
		days = 90
	}
	PruneEvents(ctx, w.DB, days)
	return nil
}

// PruneEvents deletes events older than retentionDays and emits a maintenance
// event describing the outcome.
func PruneEvents(ctx context.Context, db *bun.DB, retentionDays int) {
	res, err := db.NewRaw(
		`DELETE FROM events WHERE occurred_at < now() - (? || ' days')::interval`,
		retentionDays,
	).Exec(ctx)
	if err != nil {
		slog.Error("notify: prune events", "err", err)
		Emit(ctx, db, EmitParams{
			Type:    TypeAdminMaintFailed,
			Scope:   ScopeAdmin,
			Payload: MaintPayload{Action: "prune_events", Error: err.Error()},
		})
		return
	}
	rows, _ := res.RowsAffected() //nolint:errcheck // advisory count only
	slog.Info("notify: pruned events", "count", rows)
	Emit(ctx, db, EmitParams{
		Type:    TypeAdminMaintCompleted,
		Scope:   ScopeAdmin,
		Payload: MaintPayload{Action: "prune_events", Count: int(rows)},
	})
}
