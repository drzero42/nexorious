package joblifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// FailAllAfterRestore fails every active Job, of every kind, in a database a
// backup restore has just put in place (docs/job-lifecycle.md, "Outside the
// lifecycle"): nothing that was in flight when the backup was taken can be
// resumed, because the River queue is emptied with it. Each active Job is
// failed with msg, its open items are cancelled, and every owner
// recorded on them is cleared. No ending effects are queued, so a restore
// sends no notifications.
//
// It runs in the caller's transaction (the restore marker's, which also
// empties the queue) and needs no River client and no registered kinds: it
// may run in a process that has neither, such as `nexorious migrate`. Run it
// only before River starts.
func FailAllAfterRestore(ctx context.Context, tx bun.Tx, msg string) error {
	now := time.Now().UTC()
	if _, err := tx.NewRaw(`
		UPDATE job_items SET status = ?, processed_at = ?, river_job_id = NULL
		WHERE status IN (?, ?, ?)
		  AND job_id IN (SELECT id FROM jobs WHERE status IN (?))`,
		models.JobItemStatusCancelled, now,
		models.JobItemStatusPending, models.JobItemStatusProcessing, models.JobItemStatusPendingReview,
		bun.List(models.ActiveJobStatuses()),
	).Exec(ctx); err != nil {
		return fmt.Errorf("cancel open items: %w", err)
	}
	if _, err := tx.NewRaw(`
		UPDATE jobs SET status = ?, error_message = ?, completed_at = ?, river_job_id = NULL
		WHERE status IN (?)`,
		models.JobStatusFailed, scrub(msg), now, bun.List(models.ActiveJobStatuses()),
	).Exec(ctx); err != nil {
		return fmt.Errorf("fail active jobs: %w", err)
	}
	return nil
}
