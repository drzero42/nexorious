package joblifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// River job states, as stored in river_job.state.
const (
	riverAvailable = "available"
	riverScheduled = "scheduled"
	riverRetryable = "retryable"
	riverPending   = "pending"
	riverRunning   = "running"
	riverCompleted = "completed"
	riverCancelled = "cancelled"
	riverDiscarded = "discarded"
)

var liveRiverStates = []string{riverAvailable, riverScheduled, riverRetryable, riverPending, riverRunning}
var queuedRiverStates = []string{riverAvailable, riverScheduled, riverRetryable, riverPending}

// insertMany inserts River jobs in the transaction, in chunks, and returns
// their ids in order. InsertManyTx refuses an empty slice.
func (m *Module) insertMany(ctx context.Context, tx bun.Tx, args []river.JobArgs) ([]int64, error) {
	const chunk = 1000
	ids := make([]int64, 0, len(args))
	for start := 0; start < len(args); start += chunk {
		end := min(start+chunk, len(args))
		params := make([]river.InsertManyParams, 0, end-start)
		for _, a := range args[start:end] {
			params = append(params, river.InsertManyParams{Args: a})
		}
		res, err := m.rc.InsertManyTx(ctx, tx.Tx, params)
		if err != nil {
			return nil, fmt.Errorf("insert river jobs: %w", err)
		}
		for _, r := range res {
			ids = append(ids, r.Job.ID)
		}
	}
	return ids, nil
}

func (m *Module) insertOne(ctx context.Context, tx bun.Tx, args river.JobArgs) (int64, error) {
	res, err := m.rc.InsertTx(ctx, tx.Tx, args, nil)
	if err != nil {
		return 0, fmt.Errorf("insert river job %s: %w", args.Kind(), err)
	}
	return res.Job.ID, nil
}

// NewItem is an item a dispatch or a creator inserts.
type NewItem struct {
	ID             string // generated when empty
	UserID         string
	ItemKey        string
	SourceTitle    string
	SourceMetadata json.RawMessage
	ExternalGameID *string
}

// insertItems inserts pending items for job, each with its River job recorded
// as owner, in the transaction.
func (m *Module) insertItems(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, items []NewItem) error {
	if len(items) == 0 {
		return nil
	}
	now := time.Now().UTC()
	rows := make([]models.JobItem, len(items))
	args := make([]river.JobArgs, len(items))
	for i, it := range items {
		id := it.ID
		if id == "" {
			id = uuid.NewString()
		}
		meta := it.SourceMetadata
		if len(meta) == 0 {
			meta = json.RawMessage(`{}`)
		}
		rows[i] = models.JobItem{
			ID: id, JobID: job.ID, UserID: it.UserID, ExternalGameID: it.ExternalGameID,
			ItemKey: it.ItemKey, SourceTitle: it.SourceTitle, SourceMetadata: meta,
			Status: models.JobItemStatusPending, Result: json.RawMessage(`{}`),
			IGDBCandidates: json.RawMessage(`[]`), CreatedAt: now,
		}
		a, err := k.ItemArgs(job, &rows[i])
		if err != nil {
			return fmt.Errorf("item args: %w", err)
		}
		args[i] = a
	}
	ids, err := m.insertMany(ctx, tx, args)
	if err != nil {
		return err
	}
	for i := range rows {
		rows[i].RiverJobID = &ids[i]
	}
	const chunk = 1000
	for start := 0; start < len(rows); start += chunk {
		part := rows[start:min(start+chunk, len(rows))]
		if _, err := tx.NewInsert().Model(&part).Exec(ctx); err != nil {
			return fmt.Errorf("insert job items: %w", err)
		}
	}
	return nil
}

// queueItem gives a pending item a new River job and records it as owner.
func (m *Module) queueItem(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, item *models.JobItem) error {
	a, err := k.ItemArgs(job, item)
	if err != nil {
		return fmt.Errorf("item args: %w", err)
	}
	id, err := m.insertOne(ctx, tx, a)
	if err != nil {
		return err
	}
	if _, err := tx.NewRaw(`UPDATE job_items SET river_job_id = ? WHERE id = ?`, id, item.ID).Exec(ctx); err != nil {
		return fmt.Errorf("record item owner: %w", err)
	}
	item.RiverJobID = &id
	return nil
}

// jobRiverPredicate selects every River job of the Job: the recorded dispatch
// owner, the recorded owners of its items, and any other River job of the
// kind's River kinds that names the Job or one of its items in its args (rows
// queued before the upgrade and not yet adopted). except is left out.
const jobRiverPredicate = `kind IN (?) AND id <> ? AND (
	id IN (SELECT river_job_id FROM jobs WHERE id = ? AND river_job_id IS NOT NULL)
	OR id IN (SELECT river_job_id FROM job_items WHERE job_id = ? AND river_job_id IS NOT NULL)
	OR args->>'job_id' = ?
	OR args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?))`

// cancelJobRiverJobs cancels the Job's queued River jobs with one set-based
// UPDATE, then signals whatever is still live (the running ones) through
// River. except is the calling River job, which must not signal itself.
func (m *Module) cancelJobRiverJobs(ctx context.Context, tx bun.Tx, k *Kind, jobID string, except int64) error {
	kinds := bun.List(k.riverKinds())
	if _, err := tx.NewRaw(
		`UPDATE river_job SET state = 'cancelled', finalized_at = now()
		 WHERE state IN (?) AND `+jobRiverPredicate,
		bun.List(queuedRiverStates), kinds, except, jobID, jobID, jobID, jobID,
	).Exec(ctx); err != nil {
		return fmt.Errorf("cancel queued river jobs: %w", err)
	}
	var live []int64
	if err := tx.NewRaw(
		`SELECT id FROM river_job WHERE state IN (?) AND `+jobRiverPredicate,
		bun.List(liveRiverStates), kinds, except, jobID, jobID, jobID, jobID,
	).Scan(ctx, &live); err != nil {
		return fmt.Errorf("list live river jobs: %w", err)
	}
	for _, id := range live {
		if _, err := m.rc.JobCancelTx(ctx, tx.Tx, id); err != nil && !errors.Is(err, rivertype.ErrNotFound) {
			return fmt.Errorf("signal river job %d: %w", id, err)
		}
	}
	return nil
}

// riverStates returns the state of each River job that still has a row.
func riverStates(ctx context.Context, tx bun.Tx, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID    int64  `bun:"id"`
		State string `bun:"state"`
	}
	if err := tx.NewRaw(`SELECT id, state FROM river_job WHERE id IN (?)`, bun.List(ids)).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read river job states: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = r.State
	}
	return out, nil
}

// findLive returns the newest live River job of one of kinds whose args name
// id under argKey, or 0.
func findLive(ctx context.Context, tx bun.Tx, kinds []string, argKey, id string) (int64, error) {
	if len(kinds) == 0 {
		return 0, nil
	}
	var rid []int64
	if err := tx.NewRaw(
		`SELECT id FROM river_job WHERE kind IN (?) AND state IN (?) AND args->>? = ? ORDER BY id DESC LIMIT 1`,
		bun.List(kinds), bun.List(liveRiverStates), argKey, id,
	).Scan(ctx, &rid); err != nil {
		return 0, fmt.Errorf("find river job: %w", err)
	}
	if len(rid) == 0 {
		return 0, nil
	}
	return rid[0], nil
}
