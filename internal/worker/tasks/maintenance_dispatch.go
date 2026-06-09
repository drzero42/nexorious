package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// maintenanceJobParams carries the fields the shared dispatch helper needs to
// write a maintenance refresh jobs row.
type maintenanceJobParams struct {
	HandlerOwned bool   // true when the row was pre-created by the HTTP handler
	JobID        string // the row id (pre-created when HandlerOwned, else freshly generated)
	OwnerID      string // user_id for a self-created jobs row; the handler-owned UPDATE leaves user_id untouched (item ownership is the caller's concern, in insertItems)
	JobType      string
	Source       string
	GuardUserID  string // user_id discriminant for the self-create dedup guard/lock; "" = global (not user-scoped), matching the caller's pre-tx guard
	TotalItems   int
}

// writeMaintenanceJobInTx writes the jobs row for a maintenance refresh dispatch
// in a single transaction: for a handler-owned row it flips the pre-created
// 'pending' row to 'processing' and sets total_items; otherwise it inserts a new
// 'processing' row owned by OwnerID. It then runs insertItems within the same
// transaction to create the job_items. Shared by the metadata and store-link
// dispatch workers so the handler-owned vs self-created branching lives in one place.
//
// On the self-create path the transaction is fronted by an advisory lock on the
// (JobType, Source, GuardUserID) dedup key and re-runs the active-job guard
// inside the lock: the worker's own pre-tx guard is only a cheap early-out and
// races (its SELECT runs outside this tx), so the authoritative dedup happens
// here. When the in-tx guard finds an equivalent job already active, the helper
// inserts nothing and returns skipped=true; the caller must then NOT enqueue any
// items. (A handler-owned row needs no guard — the handler already created it.)
//
// Callers must enqueue the River item jobs only AFTER this returns (the bun tx
// must commit before riverClient.Insert, which uses a separate connection).
func writeMaintenanceJobInTx(ctx context.Context, db *bun.DB, p maintenanceJobParams, insertItems func(context.Context, bun.Tx) error) (skipped bool, err error) {
	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if p.HandlerOwned {
			if _, err := tx.NewRaw(
				`UPDATE jobs SET total_items = ?, status = 'processing' WHERE id = ?`,
				p.TotalItems, p.JobID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("update job: %w", err)
			}
			return insertItems(ctx, tx)
		}

		if e := AcquireJobDedupLock(ctx, tx, p.JobType, p.Source, p.GuardUserID); e != nil {
			return fmt.Errorf("acquire dedup lock: %w", e)
		}
		guard := `SELECT 1 FROM jobs WHERE job_type = ? AND source = ? AND status IN ('pending','processing')`
		guardArgs := []any{p.JobType, p.Source}
		if p.GuardUserID != "" {
			guard += ` AND user_id = ?`
			guardArgs = append(guardArgs, p.GuardUserID)
		}
		guard += ` LIMIT 1`
		var one int
		switch ge := tx.NewRaw(guard, guardArgs...).Scan(ctx, &one); {
		case ge == nil:
			skipped = true
			return nil
		case !errors.Is(ge, sql.ErrNoRows):
			return fmt.Errorf("active-job guard: %w", ge)
		}

		if _, err := tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
			p.JobID, p.OwnerID, p.JobType, p.Source, p.TotalItems,
		).Exec(ctx); err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		return insertItems(ctx, tx)
	})
	return skipped, err
}
