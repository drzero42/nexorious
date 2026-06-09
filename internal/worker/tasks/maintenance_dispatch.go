package tasks

import (
	"context"
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
	TotalItems   int
}

// writeMaintenanceJobInTx writes the jobs row for a maintenance refresh dispatch
// in a single transaction: for a handler-owned row it flips the pre-created
// 'pending' row to 'processing' and sets total_items; otherwise it inserts a new
// 'processing' row owned by OwnerID. It then runs insertItems within the same
// transaction to create the job_items. Shared by the metadata and store-link
// dispatch workers so the handler-owned vs self-created branching lives in one place.
//
// Callers must enqueue the River item jobs only AFTER this returns (the bun tx
// must commit before riverClient.Insert, which uses a separate connection).
func writeMaintenanceJobInTx(ctx context.Context, db *bun.DB, p maintenanceJobParams, insertItems func(context.Context, bun.Tx) error) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if p.HandlerOwned {
			if _, err := tx.NewRaw(
				`UPDATE jobs SET total_items = ?, status = 'processing' WHERE id = ?`,
				p.TotalItems, p.JobID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("update job: %w", err)
			}
		} else {
			if _, err := tx.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
				 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
				p.JobID, p.OwnerID, p.JobType, p.Source, p.TotalItems,
			).Exec(ctx); err != nil {
				return fmt.Errorf("insert job: %w", err)
			}
		}
		return insertItems(ctx, tx)
	})
}
