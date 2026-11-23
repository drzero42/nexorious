package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
)

// RequeueOrphanedPendingItems gives a new River job to every job_item that is
// stuck in pending with no River job backing it, and returns how many were
// re-queued and how many could not be.
//
// An item is orphaned when ALL of:
//   - status = 'pending'
//   - created_at older than age (0 = no age limit)
//   - its Job is still active (status IN ('pending', 'processing'))
//   - no river_job exists for it in a non-ended state
//   - its Job's kind is not on internal/joblifecycle (its recovery rule owns
//     those)
//
// jobID restricts the query to one Job; "" means every Job. The scheduled
// orphan rescue and the startup recovery call it for every Job; the sync
// dispatch calls it for its own Job once the dispatch is done, so children that
// waited on a parent that never got an item are looked at again (#1339).
func RequeueOrphanedPendingItems(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobID string, age time.Duration) (requeued, failed int, err error) {
	var orphans []struct {
		ID      string `bun:"id"`
		JobType string `bun:"job_type"`
		Source  string `bun:"source"`
	}

	q := `
		SELECT ji.id, j.job_type, j.source
		FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		WHERE ji.status = 'pending'
		  AND ji.created_at < now() - (?::text || ' seconds')::interval
		  AND j.status IN ('pending', 'processing')
		  AND j.job_type NOT IN (?)
		  AND NOT EXISTS (
		    SELECT 1 FROM river_job rj
		    WHERE rj.args->>'job_item_id' = ji.id::text
		      AND rj.state NOT IN ('completed', 'discarded', 'cancelled')
		  )`
	args := []any{int64(age.Seconds()), bun.List(lifecycleJobTypes)}
	if jobID != "" {
		q += ` AND ji.job_id = ?`
		args = append(args, jobID)
	}
	if err := db.NewRaw(q, args...).Scan(ctx, &orphans); err != nil {
		return 0, 0, fmt.Errorf("query orphaned items: %w", err)
	}

	for _, o := range orphans {
		// Route by job_type AND source so a generic-pipeline import (e.g. CSV,
		// which shares the "import" job_type but needs its own match→finalize
		// chain) re-enters at the right worker rather than the JSON importer.
		jobArgs, err := ArgsForJobType(o.JobType, o.Source, o.ID)
		if err != nil {
			slog.WarnContext(ctx, "rescue_orphaned_items: unknown job_type, skipping", logging.KeyJobItemID, o.ID, logging.KeyJobType, o.JobType, logging.KeySource, o.Source)
			continue
		}
		if err := EnqueueOrFail(ctx, db, rc, o.ID, jobArgs); err != nil {
			slog.WarnContext(ctx, "rescue_orphaned_items: re-enqueue failed", logging.KeyJobItemID, o.ID, logging.KeyErr, err)
			failed++
		} else {
			slog.InfoContext(ctx, "rescue_orphaned_items: re-enqueued orphaned item", logging.KeyJobItemID, o.ID, logging.KeyJobType, o.JobType)
			requeued++
		}
	}
	return requeued, failed, nil
}
