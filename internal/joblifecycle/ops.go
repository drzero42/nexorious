package joblifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// NewJob is a Job to create. ID is generated when empty.
type NewJob struct {
	ID         string
	UserID     string
	JobType    string
	Source     string
	Priority   string
	TotalItems int
	// CreatedAt is the Job's created_at; zero means now. A creator that
	// orders items relative to the Job sets both.
	CreatedAt time.Time
}

// Created reports the Job a Create call ended with: the new Job, or the
// active duplicate (Created=false), in which case nothing was inserted.
type Created struct {
	JobID   string
	Created bool
}

func (nj NewJob) row(status string, dispatchComplete bool) *models.Job {
	id := nj.ID
	if id == "" {
		id = uuid.NewString()
	}
	prio := nj.Priority
	if prio == "" {
		prio = models.JobPriorityNormal
	}
	created := nj.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return &models.Job{
		ID: id, UserID: nj.UserID, JobType: nj.JobType, Source: nj.Source,
		Status: status, Priority: prio, TotalItems: nj.TotalItems,
		DispatchComplete: dispatchComplete, CreatedAt: created,
	}
}

// dedup takes the duplicate lock and returns the id of an active duplicate,
// or "" when the Job may be created.
func dedup(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job) (string, error) {
	key, ok := k.DuplicateKey(job)
	if !ok {
		return "", nil
	}
	if err := acquireDupLock(ctx, tx, job.JobType, key); err != nil {
		return "", err
	}
	return activeDuplicate(ctx, tx, job.JobType, key, "")
}

// CreateWithDispatch creates a pending Job and its dispatch River job, which
// is recorded as the dispatch owner. On an active duplicate it returns that
// Job's id and inserts nothing.
func (m *Module) CreateWithDispatch(ctx context.Context, nj NewJob) (Created, error) {
	var out Created
	err := m.run(ctx, "create_with_dispatch", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		k := m.kindFor(nj.JobType, nj.Source)
		if k == nil || k.DispatchArgs == nil {
			return fmt.Errorf("%w: %s/%s has no dispatch", ErrUnknownKind, nj.JobType, nj.Source)
		}
		job := nj.row(models.JobStatusPending, false)
		dup, err := dedup(ctx, tx, k, job)
		if err != nil {
			return err
		}
		if dup != "" {
			out = Created{JobID: dup}
			return nil
		}
		args, err := k.DispatchArgs(job)
		if err != nil {
			return fmt.Errorf("dispatch args: %w", err)
		}
		rid, err := m.insertOne(ctx, tx, args)
		if err != nil {
			return err
		}
		job.RiverJobID = &rid
		if _, err := tx.NewInsert().Model(job).Exec(ctx); err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		out = Created{JobID: job.ID, Created: true}
		return nil
	})
	return out, err
}

// CreateWithItems creates a processing Job with all its items, for a creator
// that inserts every item itself: the dispatch is marked done at once and
// completion is evaluated. own runs in the same transaction. On an active
// duplicate it returns that Job's id and inserts nothing (own is not run).
func (m *Module) CreateWithItems(ctx context.Context, nj NewJob, items []NewItem, own OwnWrite) (Created, error) {
	var out Created
	err := m.run(ctx, "create_with_items", func(ctx context.Context, tx bun.Tx, st *txState) error {
		k := m.kindFor(nj.JobType, nj.Source)
		if k == nil {
			return fmt.Errorf("%w: %s/%s", ErrUnknownKind, nj.JobType, nj.Source)
		}
		job := nj.row(models.JobStatusProcessing, true)
		now := job.CreatedAt
		job.StartedAt = &now
		dup, err := dedup(ctx, tx, k, job)
		if err != nil {
			return err
		}
		if dup != "" {
			out = Created{JobID: dup}
			return nil
		}
		if _, err := tx.NewInsert().Model(job).Exec(ctx); err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		if err := m.insertItems(ctx, tx, k, job, items); err != nil {
			return err
		}
		if own != nil {
			if err := own(ctx, tx); err != nil {
				return err
			}
		}
		if err := m.evaluate(ctx, tx, st, k, job, false); err != nil {
			return err
		}
		out = Created{JobID: job.ID, Created: true}
		return nil
	})
	return out, err
}

// lockDispatch locks the Job and checks the caller owns its dispatch.
func (m *Module) lockDispatch(ctx context.Context, tx bun.Tx, jobID string, riverJobID int64) (*models.Job, *Kind, error) {
	job, err := lockJob(ctx, tx, jobID)
	if err != nil {
		return nil, nil, err
	}
	k, err := m.kindOf(job)
	if err != nil {
		return nil, nil, err
	}
	if !job.IsActive() {
		return nil, nil, ErrJobNotActive
	}
	if job.DispatchComplete {
		return nil, nil, ErrDispatchDone
	}
	if job.RiverJobID == nil || *job.RiverJobID != riverJobID {
		return nil, nil, ErrNotOwner
	}
	return job, k, nil
}

// StartDispatch is called by the dispatch owner before it starts work. A
// pending Job becomes processing; a dispatch run again finds it processing
// and continues. It returns the Job.
func (m *Module) StartDispatch(ctx context.Context, jobID string, riverJobID int64) (*models.Job, error) {
	var out *models.Job
	err := m.run(ctx, "start_dispatch", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		job, _, err := m.lockDispatch(ctx, tx, jobID, riverJobID)
		if err != nil {
			return err
		}
		if job.Status == models.JobStatusPending {
			now := time.Now().UTC()
			if _, err := tx.NewRaw(
				`UPDATE jobs SET status = 'processing', started_at = COALESCE(started_at, ?) WHERE id = ? AND status = 'pending'`,
				now, job.ID,
			).Exec(ctx); err != nil {
				return fmt.Errorf("start dispatch: %w", err)
			}
			job.Status = models.JobStatusProcessing
			if job.StartedAt == nil {
				job.StartedAt = &now
			}
		}
		out = job
		return nil
	})
	return out, err
}

// AddItems inserts items, each with its River job, for the dispatch owner.
func (m *Module) AddItems(ctx context.Context, jobID string, riverJobID int64, items []NewItem) error {
	return m.run(ctx, "add_items", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		job, k, err := m.lockDispatch(ctx, tx, jobID, riverJobID)
		if err != nil {
			return err
		}
		if job.Status != models.JobStatusProcessing {
			return ErrJobNotActive
		}
		return m.insertItems(ctx, tx, k, job, items)
	})
}

// AddNewItems is AddItems for a dispatch that adds in batches and can be run
// again: an item whose item_key the Job already has, or that comes twice in
// items, is left out, so no existing item gets a second River job. It returns
// how many items it created. The Job row lock serialises it with every other
// insert into the Job.
func (m *Module) AddNewItems(ctx context.Context, jobID string, riverJobID int64, items []NewItem) (int, error) {
	var n int
	err := m.run(ctx, "add_new_items", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		n = 0
		job, k, err := m.lockDispatch(ctx, tx, jobID, riverJobID)
		if err != nil {
			return err
		}
		if job.Status != models.JobStatusProcessing {
			return ErrJobNotActive
		}
		if len(items) == 0 {
			return nil
		}
		keys := make([]string, len(items))
		for i, it := range items {
			keys[i] = it.ItemKey
		}
		var existing []string
		if err := tx.NewRaw(`SELECT item_key FROM job_items WHERE job_id = ? AND item_key IN (?)`, job.ID, bun.List(keys)).Scan(ctx, &existing); err != nil {
			return fmt.Errorf("load existing items: %w", err)
		}
		seen := make(map[string]bool, len(existing)+len(items))
		for _, key := range existing {
			seen[key] = true
		}
		fresh := make([]NewItem, 0, len(items))
		for _, it := range items {
			if seen[it.ItemKey] {
				continue
			}
			seen[it.ItemKey] = true
			fresh = append(fresh, it)
		}
		if err := m.insertItems(ctx, tx, k, job, fresh); err != nil {
			return err
		}
		n = len(fresh)
		return nil
	})
	return n, err
}

// Dispatched is what the dispatch owner records when it marks the dispatch
// done.
type Dispatched struct {
	Items      []NewItem // inserted in the same transaction
	TotalItems *int
	FilePath   *string
	Own        OwnWrite
}

// MarkDispatched marks the dispatch done for its owner, with the Job fields
// set at that point and the caller's own writes, clears the dispatch owner
// and evaluates completion.
func (m *Module) MarkDispatched(ctx context.Context, jobID string, riverJobID int64, d Dispatched) error {
	return m.run(ctx, "mark_dispatched", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, k, err := m.lockDispatch(ctx, tx, jobID, riverJobID)
		if err != nil {
			return err
		}
		if job.Status != models.JobStatusProcessing {
			return ErrJobNotActive
		}
		if err := m.insertItems(ctx, tx, k, job, d.Items); err != nil {
			return err
		}
		if d.TotalItems != nil {
			job.TotalItems = *d.TotalItems
		}
		if d.FilePath != nil {
			job.FilePath = d.FilePath
		}
		job.DispatchComplete = true
		job.RiverJobID = nil
		if _, err := tx.NewUpdate().Model(job).
			Column("total_items", "file_path", "dispatch_complete", "river_job_id").
			Where("id = ? AND status = 'processing'", job.ID).
			Exec(ctx); err != nil {
			return fmt.Errorf("mark dispatched: %w", err)
		}
		if d.Own != nil {
			if err := d.Own(ctx, tx); err != nil {
				return err
			}
		}
		// Items that waited on an item the dispatch never added, and items
		// the kind no longer names as waiting, are queued now.
		if err := m.queueUnwaitable(ctx, tx, k, job); err != nil {
			return err
		}
		return m.evaluate(ctx, tx, st, k, job, false)
	})
}

// Owns reports, without locking, whether riverJobID still owns the item: nil,
// or the refusal it would get. A worker can stop before expensive work.
func (m *Module) Owns(ctx context.Context, itemID string, riverJobID int64) error {
	var row struct {
		ItemStatus string `bun:"item_status"`
		RiverJobID *int64 `bun:"river_job_id"`
		JobStatus  string `bun:"job_status"`
	}
	err := m.db.NewRaw(
		`SELECT ji.status AS item_status, ji.river_job_id, j.status AS job_status
		 FROM job_items ji JOIN jobs j ON j.id = ji.job_id WHERE ji.id = ?`, itemID,
	).Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGone
	}
	if err != nil {
		return fmt.Errorf("owns: %w", err)
	}
	if row.JobStatus != models.JobStatusPending && row.JobStatus != models.JobStatusProcessing {
		return ErrJobNotActive
	}
	if row.ItemStatus != models.JobItemStatusPending || row.RiverJobID == nil || *row.RiverJobID != riverJobID {
		return ErrNotOwner
	}
	return nil
}

// ItemResult is how an item finished.
type ItemResult struct {
	Status  string // completed, failed or skipped
	Message string // the error message of a failed item
	// Own is the caller's own write recording the outcome (invariant 8). A
	// non-nil result it returns is stored as the item's result.
	Own func(ctx context.Context, tx bun.Tx) (result json.RawMessage, err error)
}

// lockItem locks the item's Job and returns both, for its owner.
func (m *Module) lockItem(ctx context.Context, tx bun.Tx, itemID string, riverJobID int64) (*models.Job, *Kind, *models.JobItem, error) {
	var jobID string
	err := tx.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, ErrGone
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load item: %w", err)
	}
	job, err := lockJob(ctx, tx, jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	k, err := m.kindOf(job)
	if err != nil {
		return nil, nil, nil, err
	}
	var item models.JobItem
	if err := tx.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, ErrGone
		}
		return nil, nil, nil, fmt.Errorf("load item: %w", err)
	}
	if !job.IsActive() {
		return nil, nil, nil, ErrJobNotActive
	}
	if item.Status != models.JobItemStatusPending || item.RiverJobID == nil || *item.RiverJobID != riverJobID {
		return nil, nil, nil, ErrNotOwner
	}
	return job, k, &item, nil
}

// FinishItem moves an item its caller owns to completed, failed or skipped,
// with the caller's own write, and evaluates completion.
func (m *Module) FinishItem(ctx context.Context, itemID string, riverJobID int64, r ItemResult) error {
	switch r.Status {
	case models.JobItemStatusCompleted, models.JobItemStatusFailed, models.JobItemStatusSkipped:
	default:
		return fmt.Errorf("joblifecycle: FinishItem with status %q", r.Status)
	}
	return m.run(ctx, "finish_item", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, k, item, err := m.lockItem(ctx, tx, itemID, riverJobID)
		if err != nil {
			return err
		}
		var result json.RawMessage
		if r.Own != nil {
			if result, err = r.Own(ctx, tx); err != nil {
				return err
			}
		}
		if err := m.finishItem(ctx, tx, st, k, job, item, r.Status, r.Message, result); err != nil {
			return err
		}
		return m.evaluate(ctx, tx, st, k, job, false)
	})
}

// finishItem writes an item's finished status and clears its owner.
func (m *Module) finishItem(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, item *models.JobItem, status, msg string, result json.RawMessage) error {
	now := time.Now().UTC()
	var errMsg *string
	if status == models.JobItemStatusFailed {
		s := scrub(msg)
		errMsg = &s
	}
	q := tx.NewUpdate().Table("job_items").
		Set("status = ?", status).
		Set("processed_at = ?", now).
		Set("error_message = ?", errMsg).
		Set("river_job_id = NULL").
		Where("id = ? AND status = 'pending'", item.ID)
	if result != nil {
		q = q.Set("result = ?", result)
	}
	if _, err := q.Exec(ctx); err != nil {
		return fmt.Errorf("finish item: %w", err)
	}
	if k.Observer != nil {
		jobCopy := *job
		st.onCommit(func(ctx context.Context) { k.Observer.ItemFinished(ctx, jobCopy, status) })
	}
	return m.releaseWaiters(ctx, tx, k, job, []string{item.ID})
}

// Failure is how a Job is failed.
type Failure struct {
	Message string
	// CallerRiverJobID is the dispatch owner failing its Job. Its own River
	// job is not cancelled.
	CallerRiverJobID int64
	KindData         json.RawMessage
	// Own is the caller's own write, in the same transaction, after the
	// ownership check. A non-nil result replaces KindData: what the effects
	// need to know may only be known once the write ran.
	Own func(ctx context.Context, tx bun.Tx) (json.RawMessage, error)
}

// Fail fails an active Job for its dispatch owner, with the caller's own
// writes: its unfinished items are cancelled, its River jobs cancelled or
// signalled (except the caller's), and its finishing effects queued.
func (m *Module) Fail(ctx context.Context, jobID string, f Failure) error {
	return m.run(ctx, "fail", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, k, err := m.lockDispatch(ctx, tx, jobID, f.CallerRiverJobID)
		if err != nil {
			return err
		}
		kindData := f.KindData
		if f.Own != nil {
			data, err := f.Own(ctx, tx)
			if err != nil {
				return err
			}
			if data != nil {
				kindData = data
			}
		}
		return m.fail(ctx, tx, st, k, job, f.Message, kindData, f.CallerRiverJobID)
	})
}

// fail finishes an active Job as failed. The Job row is locked.
func (m *Module) fail(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, msg string, kindData json.RawMessage, except int64) error {
	first, err := m.finishJob(ctx, tx, st, k, job, models.JobStatusFailed, scrub(msg), except)
	if err != nil {
		return err
	}
	if k.hasEffects(OutcomeFailed) {
		if err := m.queueEffects(ctx, tx, job.ID, OutcomeFailed, first, kindData); err != nil {
			return err
		}
	}
	return nil
}

// Cancel finishes an active Job as cancelled: its unfinished items are
// cancelled and its River jobs cancelled or signalled. It has no finishing
// effects. Authorisation is the caller's.
func (m *Module) Cancel(ctx context.Context, jobID string) error {
	return m.run(ctx, "cancel", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, err := lockJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		k, err := m.kindOf(job)
		if err != nil {
			return err
		}
		if !job.IsActive() {
			return ErrJobNotActive
		}
		_, err = m.finishJob(ctx, tx, st, k, job, models.JobStatusCancelled, "", 0)
		return err
	})
}

// finishJob moves an active Job to failed or cancelled: River jobs first (so
// the item owners are still recorded), then the items, then the Job. It
// reports whether this is the Job's first finish.
func (m *Module) finishJob(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, status, msg string, except int64) (bool, error) {
	if err := m.cancelJobRiverJobs(ctx, tx, k, job.ID, except); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	if _, err := tx.NewRaw(
		`UPDATE job_items SET status = 'cancelled', processed_at = ?, river_job_id = NULL
		 WHERE job_id = ? AND status IN ('pending', 'pending_review', 'processing')`,
		now, job.ID,
	).Exec(ctx); err != nil {
		return false, fmt.Errorf("cancel items: %w", err)
	}
	first := job.CompletedAt == nil
	var errMsg *string
	if msg != "" {
		errMsg = &msg
	}
	if _, err := tx.NewRaw(
		`UPDATE jobs SET status = ?, completed_at = ?, error_message = COALESCE(?, error_message), river_job_id = NULL
		 WHERE id = ? AND status IN ('pending', 'processing')`,
		status, now, errMsg, job.ID,
	).Exec(ctx); err != nil {
		return false, fmt.Errorf("finish job: %w", err)
	}
	job.Status, job.CompletedAt, job.RiverJobID = status, &now, nil
	if errMsg != nil {
		job.ErrorMessage = errMsg
	}
	if first {
		if err := m.observeJob(ctx, tx, st, k, job, status); err != nil {
			return false, err
		}
	}
	return first, nil
}

// Retry retries failed items of a Job that is active or completed: all of
// them when itemIDs is empty, otherwise the given ones. A completed Job is
// reopened, unless another Job with its duplicate key is active. It returns
// how many items it retried; with none failed it changes nothing.
func (m *Module) Retry(ctx context.Context, jobID string, itemIDs []string) (int, error) {
	var n int
	err := m.run(ctx, "retry", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		job, err := lockJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		k, err := m.kindOf(job)
		if err != nil {
			return err
		}
		if job.Status == models.JobStatusFailed || job.Status == models.JobStatusCancelled {
			return ErrNotRetryable
		}
		var items []models.JobItem
		q := tx.NewSelect().Model(&items).Where("job_id = ? AND status = 'failed'", job.ID).Order("created_at", "id")
		if len(itemIDs) > 0 {
			q = q.Where("id IN (?)", bun.List(itemIDs))
		}
		if err := q.Scan(ctx); err != nil {
			return fmt.Errorf("load failed items: %w", err)
		}
		if len(items) == 0 {
			if len(itemIDs) > 0 {
				return ErrNotRetryable
			}
			return nil
		}
		// Retrying an item retries the failed items that wait on it too.
		if len(itemIDs) > 0 && k.WaitsOn != nil {
			if items, err = m.withFailedWaiters(ctx, tx, k, job, items); err != nil {
				return err
			}
		}
		if job.Status == models.JobStatusCompleted {
			if key, ok := k.DuplicateKey(job); ok {
				if err := acquireDupLock(ctx, tx, job.JobType, key); err != nil {
					return err
				}
				dup, err := activeDuplicate(ctx, tx, job.JobType, key, job.ID)
				if err != nil {
					return err
				}
				if dup != "" {
					return ErrDuplicateActive
				}
			}
			if _, err := tx.NewRaw(`UPDATE jobs SET status = 'processing' WHERE id = ? AND status = 'completed'`, job.ID).Exec(ctx); err != nil {
				return fmt.Errorf("reopen job: %w", err)
			}
			job.Status = models.JobStatusProcessing
		}
		ids := make([]string, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		if _, err := tx.NewRaw(
			`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL
			 WHERE id IN (?) AND status = 'failed'`, bun.List(ids),
		).Exec(ctx); err != nil {
			return fmt.Errorf("reset items: %w", err)
		}
		for i := range items {
			items[i].Status = models.JobItemStatusPending
			if err := m.queueItem(ctx, tx, k, job, &items[i]); err != nil {
				return err
			}
		}
		n = len(items)
		return nil
	})
	return n, err
}

// withFailedWaiters adds to items the failed items of the Job that, by the
// kind's rule, wait on one of them.
func (m *Module) withFailedWaiters(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, items []models.JobItem) ([]models.JobItem, error) {
	in := make(map[string]bool, len(items))
	ids := make([]string, len(items))
	for i, it := range items {
		in[it.ID] = true
		ids[i] = it.ID
	}
	edges, err := k.WaitsOn(ctx, tx, WaitQuery{JobID: job.ID, Awaited: ids})
	if err != nil {
		return nil, fmt.Errorf("waits on: %w", err)
	}
	var more []string
	for _, e := range edges {
		if !in[e.ItemID] {
			in[e.ItemID] = true
			more = append(more, e.ItemID)
		}
	}
	if len(more) == 0 {
		return items, nil
	}
	var waiters []models.JobItem
	if err := tx.NewSelect().Model(&waiters).
		Where("job_id = ? AND id IN (?) AND status = 'failed'", job.ID, bun.List(more)).
		Order("created_at", "id").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("load failed waiters: %w", err)
	}
	return append(items, waiters...), nil
}
