package scheduler

import (
	"context"
	"log/slog"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// RescueOrphanedPendingItems finds job_items that are stuck in pending with no
// active River job backing them and re-enqueues a new River job for each one.
//
// An item is considered orphaned when ALL of:
//   - status = 'pending'
//   - created_at older than age (so freshly-created items are not touched)
//   - parent job is still active (status IN ('pending', 'processing'))
//   - no river_job exists for this item in a non-terminal state
//
// This is a safety net for the case where the worker's load-job_item query
// fails transiently: the worker returns an error (River retries), but if the
// River job was already finalized before the fix was deployed — or if the
// job was lost for any other reason — this sweeper catches it.
func RescueOrphanedPendingItems(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], age time.Duration) {
	var orphans []struct {
		ID      string `bun:"id"`
		JobType string `bun:"job_type"`
	}

	if err := db.NewRaw(`
		SELECT ji.id, j.job_type
		FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		WHERE ji.status = 'pending'
		  AND ji.created_at < now() - (?::text || ' seconds')::interval
		  AND j.status IN ('pending', 'processing')
		  AND NOT EXISTS (
		    SELECT 1 FROM river_job rj
		    WHERE rj.args->>'job_item_id' = ji.id
		      AND rj.state NOT IN ('completed', 'discarded', 'cancelled')
		  )`,
		int64(age.Seconds()),
	).Scan(ctx, &orphans); err != nil {
		slog.Error("rescue_orphaned_items: query failed", "err", err)
		emitMaint(ctx, db, true, notify.MaintPayload{Action: "rescue_orphaned_items", Error: err.Error()})
		return
	}

	var successCount, failureCount int
	for _, o := range orphans {
		var args river.JobArgs
		switch o.JobType {
		case "sync":
			args = tasks.IGDBMatchArgs{JobItemID: o.ID}
		case "import":
			args = tasks.ImportItemArgs{JobItemID: o.ID}
		case "metadata_refresh":
			args = tasks.MetadataRefreshItemArgs{JobItemID: o.ID}
		default:
			slog.Warn("rescue_orphaned_items: unknown job_type, skipping", "item_id", o.ID, "job_type", o.JobType)
			continue
		}
		if _, err := rc.Insert(ctx, args, nil); err != nil {
			slog.Error("rescue_orphaned_items: re-enqueue failed", "item_id", o.ID, "err", err)
			failureCount++
		} else {
			slog.Info("rescue_orphaned_items: re-enqueued orphaned item", "item_id", o.ID, "job_type", o.JobType)
			successCount++
		}
	}
	if successCount+failureCount > 0 {
		emitMaint(ctx, db, false, notify.MaintPayload{Action: "rescue_orphaned_items", Rescued: successCount, Failed: failureCount})
	}
}
