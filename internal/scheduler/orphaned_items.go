package scheduler

import (
	"context"
	"log/slog"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// RescueOrphanedPendingItems finds job_items that are stuck in pending with no
// active River job backing them and re-enqueues a new River job for each one.
//
// The query is tasks.RequeueOrphanedPendingItems (shared with the sync
// dispatch's own end-of-dispatch re-queue); age keeps freshly-created items out.
//
// This is a safety net for the case where the worker's load-job_item query
// fails transiently: the worker returns an error (River retries), but if the
// River job was already finalized before the fix was deployed — or if the
// job was lost for any other reason — this sweeper catches it.
func RescueOrphanedPendingItems(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], age time.Duration) {
	rescueOrphanedPendingItems(ctx, db, rc, age, true)
}

// rescueOrphanedPendingItems is RescueOrphanedPendingItems with the maintenance
// event optional: the startup recovery pass (RecoverInterruptedSyncs) runs it
// with no age limit and emitEvent=false, since a restart must not notify.
func rescueOrphanedPendingItems(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], age time.Duration, emitEvent bool) {
	successCount, failureCount, err := tasks.RequeueOrphanedPendingItems(ctx, db, rc, "", age)
	if err != nil {
		slog.ErrorContext(ctx, "rescue_orphaned_items: query failed", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		if emitEvent {
			emitMaint(ctx, db, true, notify.MaintPayload{Action: "rescue_orphaned_items", Error: err.Error()})
		}
		return
	}
	if emitEvent && successCount+failureCount > 0 {
		emitMaint(ctx, db, false, notify.MaintPayload{Action: "rescue_orphaned_items", Rescued: successCount, Failed: failureCount})
	}
}
