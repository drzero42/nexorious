package tasks

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// This file holds the shared job_item helpers used by every River worker that
// processes job_items (sync, metadata refresh, import). Before consolidation,
// each worker carried its own near-identical copies of these mutators and a
// bespoke "count remaining → finalize job" routine; the only real differences
// were the slog prefix, which columns the completion check counts as "remaining",
// and the per-domain completion notifications. The mutators are unified here; the
// completion checks keep their domain-specific notification policy but build on
// the shared countJobItems / finalizeJobCompleted primitives.

// execItemUpdate persists the given columns of a single job_item and logs (does
// not return) any failure under logPrefix. Callers set the relevant fields on
// item before calling and list exactly the columns to write.
func execItemUpdate(ctx context.Context, db *bun.DB, item *models.JobItem, logPrefix string, columns ...string) {
	_, err := db.NewUpdate().Model(item).
		Column(columns...).
		Where("id = ?", item.ID).
		Exec(ctx)
	if err != nil {
		slog.Error(logPrefix, "id", item.ID, "err", err)
	}
}

// markItemFailed sets a job_item to failed with an error message and processed_at=now.
func markItemFailed(ctx context.Context, db *bun.DB, item *models.JobItem, msg, logPrefix string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusFailed
	item.ErrorMessage = &msg
	item.ProcessedAt = &now
	execItemUpdate(ctx, db, item, logPrefix, "status", "error_message", "processed_at")
}

// markItemCompleted sets a job_item to completed with processed_at=now.
func markItemCompleted(ctx context.Context, db *bun.DB, item *models.JobItem, logPrefix string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusCompleted
	item.ProcessedAt = &now
	execItemUpdate(ctx, db, item, logPrefix, "status", "processed_at")
}

// markItemCompletedWithResult sets a job_item to completed, persisting the
// marshalled result alongside processed_at=now. Used by the import worker, which
// records a per-item result payload.
func markItemCompletedWithResult(ctx context.Context, db *bun.DB, item *models.JobItem, result any, logPrefix string) {
	now := time.Now().UTC()
	resultJSON, _ := json.Marshal(result) //nolint:errcheck // marshaling the job result struct cannot fail
	item.Status = models.JobItemStatusCompleted
	item.Result = resultJSON
	item.ProcessedAt = &now
	execItemUpdate(ctx, db, item, logPrefix, "status", "result", "processed_at")
}

// markItemSkipped sets a job_item to skipped with processed_at=now.
func markItemSkipped(ctx context.Context, db *bun.DB, item *models.JobItem, logPrefix string) {
	now := time.Now().UTC()
	item.Status = models.JobItemStatusSkipped
	item.ProcessedAt = &now
	execItemUpdate(ctx, db, item, logPrefix, "status", "processed_at")
}

// countJobItems counts job_items for the job that match predicate, a constant
// SQL fragment (e.g. "status IN ('pending', 'processing')"). job_id is always
// bound as a parameter. On error it logs under logPrefix and returns ok=false so
// callers can bail without finalizing.
func countJobItems(ctx context.Context, db *bun.DB, jobID, predicate, logPrefix string) (count int, ok bool) {
	query := "SELECT COUNT(*) FROM job_items WHERE job_id = ? AND " + predicate
	if err := db.NewRaw(query, jobID).Scan(ctx, &count); err != nil {
		slog.Error(logPrefix, "job_id", jobID, "err", err)
		return 0, false
	}
	return count, true
}

// finalizeJobCompleted drives a job to 'completed'. The UPDATE is guarded by
// status IN ('pending', 'processing') so it is idempotent and never flips an
// already-terminal job back to completed. When requireDispatchComplete is true
// it additionally refuses to finalize while batches are still being dispatched
// (dispatch_complete=false), the sync worker's "more work may still arrive"
// guard. It returns finalized=true only when this call performed the update —
// callers use that to emit completion notifications exactly once.
func finalizeJobCompleted(ctx context.Context, db *bun.DB, jobID, logPrefix string, requireDispatchComplete bool) (finalized bool) {
	now := time.Now().UTC()
	query := "UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')"
	if requireDispatchComplete {
		query += " AND dispatch_complete = true"
	}
	res, err := db.NewRaw(query, now, jobID).Exec(ctx)
	if err != nil {
		slog.Error(logPrefix, "job_id", jobID, "err", err)
		return false
	}
	n, _ := res.RowsAffected() //nolint:errcheck // advisory RowsAffected
	return n > 0
}
