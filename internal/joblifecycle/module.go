// Package joblifecycle is the one owner of Job and Job item status
// (docs/job-lifecycle.md is its specification). It makes every change to
// jobs.status, job_items.status and jobs.dispatch_complete for the Job kinds
// registered with it, and creates their Jobs and items. Workers, HTTP handlers
// and the scheduler ask it for a transition; they never write those columns
// themselves.
//
// Every operation locks the Job row first, checks what it expects, writes,
// and evaluates completion before it commits. River jobs for a Job's dispatch,
// its items and its ending effects are inserted in that same transaction,
// through an insert-only River client on Bun's *sql.DB (ADR 0001), and the
// River job that owns a pending item or a dispatch that is not done is
// recorded on the row (river_job_id).
//
// The module also writes river_job directly in two places: Cancel and Fail
// cancel a Job's queued River jobs with one set-based UPDATE before signalling
// the running ones through River, and the startup step ReclaimRunning moves
// River jobs a dead process left running back to a state River acts on.
//
// Some operations belong to one caller. StartDispatch, AddItems, AddNewItems
// and MarkDispatched are for the dispatch owner; Owns, EndItem, NextStage and
// AwaitReview for the item owner. Resolve and Skip are the review decisions;
// SkipItems and CompleteItems carry a sync's own writes in a Selector. A
// refused transition is typed (IsRefusal), and a worker that gets one exits
// cleanly. Ending effects (notifications) run in EffectsWorker.
//
// Kinds are registered on an instance, not globally, so tests register fake
// kinds. The module does not import the workers: a kind tells it which River
// args run its dispatch and its items.
package joblifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// Outcome is what an ending-effects River job was queued for.
type Outcome string

const (
	OutcomeCompleted      Outcome = "completed"
	OutcomeFailed         Outcome = "failed"
	OutcomeAwaitingReview Outcome = "awaiting_review"
)

// Refusals. Workers treat ErrNotOwner, ErrJobNotActive, ErrGone and
// ErrDispatchDone as a clean exit (see IsRefusal); handlers map them to 404 or
// 409.
var (
	ErrNotOwner        = errors.New("joblifecycle: not the owner")
	ErrJobNotActive    = errors.New("joblifecycle: job is not active")
	ErrGone            = errors.New("joblifecycle: job or item no longer exists")
	ErrDispatchDone    = errors.New("joblifecycle: dispatch already done")
	ErrNotRetryable    = errors.New("joblifecycle: not retryable")
	ErrDuplicateActive = errors.New("joblifecycle: an equivalent job is already active")
	ErrUnknownKind     = errors.New("joblifecycle: job kind is not on the module")
	ErrNotInReview     = errors.New("joblifecycle: item is not in pending review")
)

// IsRefusal reports whether err means the caller no longer has anything to do:
// it is not the owner, the Job has ended, the row is gone, or the dispatch
// is already done. A worker that gets one exits cleanly.
func IsRefusal(err error) bool {
	return errors.Is(err, ErrNotOwner) || errors.Is(err, ErrJobNotActive) ||
		errors.Is(err, ErrGone) || errors.Is(err, ErrDispatchDone)
}

// abortError marks an error returned by a caller's own write on purpose, to
// roll the transition back. It is returned unwrapped-compatible (errors.Is
// matches the cause) and is not logged as a failed transition.
type abortError struct{ err error }

func (e abortError) Error() string { return e.err.Error() }
func (e abortError) Unwrap() error { return e.err }

// Abort wraps an error a caller's own write returns to roll the transition
// back on purpose (a row it expected is gone, for instance). The module does
// not log it as a failed transition; errors.Is still matches err.
func Abort(err error) error { return abortError{err: err} }

// OwnWrite is a caller's own write that commits or rolls back with the
// transition (docs/job-lifecycle.md, "Transactions and locks").
type OwnWrite func(ctx context.Context, tx bun.Tx) error

// DupKey identifies Jobs of which at most one may be active. UserID is "" for
// a key that is not per user.
type DupKey struct {
	Source string
	UserID string
}

// Effect is what a kind's ending effects are run with.
type Effect struct {
	Job      *models.Job
	Outcome  Outcome
	FirstEnd bool
	KindData json.RawMessage
	// Insert queues a River job that is not Job work, such as a dispatch
	// that creates its own Job (the store-link refresh after a sync).
	Insert func(ctx context.Context, args river.JobArgs) error
}

// Observer records metrics in-process, right after the transaction of the
// transition it counts has committed.
type Observer interface {
	// ItemEnded is called once per item that ends (completed, failed
	// or skipped; cancelled items are not counted).
	ItemEnded(ctx context.Context, job models.Job, status string)
	// JobEnded is called on a Job's first ending only.
	JobEnded(ctx context.Context, job models.Job, outcome string, failedItems int)
}

// Kind is what a Job kind tells the module about itself.
type Kind struct {
	// JobType is the jobs.job_type of the kind.
	JobType string
	// Sources narrows the kind to these jobs.source values; empty means
	// every source of JobType.
	Sources []string
	// DispatchRiverKinds and ItemRiverKinds are the River kinds the kind
	// uses for its dispatch (args name the Job as job_id) and its items
	// (args name the item as job_item_id).
	DispatchRiverKinds []string
	ItemRiverKinds     []string
	// ItemArgs returns the River args that run an item when the module
	// queues it by itself (Retry, recovery).
	ItemArgs func(job *models.Job, item *models.JobItem) (river.JobArgs, error)
	// DispatchArgs returns the River args that run the Job's dispatch. Nil
	// means the dispatch cannot be run again.
	DispatchArgs func(job *models.Job) (river.JobArgs, error)
	// DispatchLostMessage is the error recovery fails a Job with when its
	// dispatch is not done, has no live owner, and cannot be run again
	// (DispatchArgs is nil). Empty means a generic message.
	DispatchLostMessage string
	// DuplicateKey returns the Job's duplicate key; ok=false means Jobs of
	// the kind are never duplicates.
	DuplicateKey func(job *models.Job) (key DupKey, ok bool)
	// Effects runs the kind's ending effects. HasEffects says for which
	// outcomes there are any; no effects River job is queued for the others.
	Effects    func(ctx context.Context, db *bun.DB, e Effect) error
	HasEffects func(o Outcome) bool
	// Observer is optional.
	Observer Observer
}

func (k *Kind) hasEffects(o Outcome) bool {
	return k.Effects != nil && k.HasEffects != nil && k.HasEffects(o)
}

func (k *Kind) riverKinds() []string {
	return append(slices.Clone(k.DispatchRiverKinds), k.ItemRiverKinds...)
}

func (k *Kind) matches(jobType, source string) bool {
	return k.JobType == jobType && (len(k.Sources) == 0 || slices.Contains(k.Sources, source))
}

// Module owns the lifecycle of the Job kinds registered with it.
type Module struct {
	db    *bun.DB
	rc    *river.Client[*sql.Tx]
	kinds []*Kind
}

// New returns a module that writes through db and inserts River jobs through
// rc, an insert-only client (no queues, no workers) on db's *sql.DB.
func New(db *bun.DB, rc *river.Client[*sql.Tx]) *Module {
	return &Module{db: db, rc: rc}
}

// Register adds a Job kind. Kinds are found by job_type first, then source: a
// kind that names the Job's source wins over one with no Sources, whatever the
// order they were registered in.
func (m *Module) Register(k *Kind) {
	if k.ItemArgs == nil || k.DuplicateKey == nil {
		panic(fmt.Sprintf("joblifecycle: kind %q needs ItemArgs and DuplicateKey", k.JobType))
	}
	m.kinds = append(m.kinds, k)
}

// Handles reports whether the module owns Jobs of this job_type and source.
func (m *Module) Handles(jobType, source string) bool {
	return m.kindFor(jobType, source) != nil
}

func (m *Module) kindFor(jobType, source string) *Kind {
	var catchAll *Kind
	for _, k := range m.kinds {
		if !k.matches(jobType, source) {
			continue
		}
		if len(k.Sources) > 0 {
			return k
		}
		if catchAll == nil {
			catchAll = k
		}
	}
	return catchAll
}

// JobTypes returns the job_type of every registered kind.
func (m *Module) JobTypes() []string {
	var out []string
	for _, k := range m.kinds {
		if !slices.Contains(out, k.JobType) {
			out = append(out, k.JobType)
		}
	}
	return out
}

// RiverKinds returns every River kind the lifecycle uses: the kinds' dispatch
// and item River kinds, and the ending-effects River kind.
func (m *Module) RiverKinds() []string {
	out := []string{EffectsArgs{}.Kind()}
	for _, k := range m.kinds {
		for _, rk := range k.riverKinds() {
			if !slices.Contains(out, rk) {
				out = append(out, rk)
			}
		}
	}
	return out
}

// txState collects what an operation does after its transaction commits.
type txState struct {
	after []func(context.Context)
}

func (st *txState) onCommit(f func(context.Context)) { st.after = append(st.after, f) }

// run executes fn in one transaction and, once it committed, the work fn
// queued for after the commit. A transition that fails logs one fixed error
// line, or an info line when only its context was cancelled; refusals and
// aborts are not logged.
func (m *Module) run(ctx context.Context, op string, fn func(ctx context.Context, tx bun.Tx, st *txState) error) error {
	st := &txState{}
	err := m.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(ctx, tx, st)
	})
	if err != nil {
		var ab abortError
		switch {
		case IsRefusal(err), errors.Is(err, ErrNotRetryable), errors.Is(err, ErrDuplicateActive),
			errors.Is(err, ErrUnknownKind), errors.Is(err, ErrNotInReview), errors.As(err, &ab):
		case ctx.Err() != nil:
			slog.InfoContext(ctx, "job lifecycle: transition interrupted", logging.KeyOperation, op, logging.KeyErr, err)
		default:
			slog.ErrorContext(ctx, "job lifecycle: transition failed", logging.KeyOperation, op, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
		return err
	}
	for _, f := range st.after {
		f(context.WithoutCancel(ctx))
	}
	return nil
}

// lockJob locks the Job row and returns it, or ErrGone.
func lockJob(ctx context.Context, tx bun.Tx, jobID string) (*models.Job, error) {
	var job models.Job
	err := tx.NewSelect().Model(&job).Where("id = ?", jobID).For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGone
	}
	if err != nil {
		return nil, fmt.Errorf("lock job: %w", err)
	}
	return &job, nil
}

// kindOf returns the kind of a Job, or ErrUnknownKind.
func (m *Module) kindOf(job *models.Job) (*Kind, error) {
	k := m.kindFor(job.JobType, job.Source)
	if k == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrUnknownKind, job.JobType, job.Source)
	}
	return k, nil
}

// acquireDupLock takes the transaction-scoped advisory lock on a duplicate
// key.
func acquireDupLock(ctx context.Context, tx bun.Tx, jobType string, key DupKey) error {
	_, err := tx.NewRaw(`SELECT pg_advisory_xact_lock(hashtext(?))`, jobType+"|"+key.Source+"|"+key.UserID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("duplicate lock: %w", err)
	}
	return nil
}

// activeDuplicate returns the id of another active Job with the same key, or "".
func activeDuplicate(ctx context.Context, tx bun.Tx, jobType string, key DupKey, exceptID string) (string, error) {
	q := tx.NewSelect().Table("jobs").Column("id").
		Where("job_type = ?", jobType).
		Where("source = ?", key.Source).
		Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Limit(1)
	if key.UserID != "" {
		q = q.Where("user_id = ?", key.UserID)
	}
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	var id string
	err := q.Scan(ctx, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("duplicate check: %w", err)
	}
	return id, nil
}

func scrub(msg string) string { return logging.ScrubURLQueries(msg) }
