package joblifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"
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
		UPDATE job_items SET status = 'cancelled', processed_at = ?, river_job_id = NULL
		WHERE status IN ('pending', 'processing', 'pending_review')
		  AND job_id IN (SELECT id FROM jobs WHERE status IN ('pending', 'processing'))`,
		now,
	).Exec(ctx); err != nil {
		return fmt.Errorf("cancel open items: %w", err)
	}
	if _, err := tx.NewRaw(`
		UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ?, river_job_id = NULL
		WHERE status IN ('pending', 'processing')`,
		scrub(msg), now,
	).Exec(ctx); err != nil {
		return fmt.Errorf("fail active jobs: %w", err)
	}
	return nil
}
