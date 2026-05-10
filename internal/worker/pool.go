package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// TaskHandler processes a single claimed task.
type TaskHandler func(ctx context.Context, task *models.PendingTask) error

// Pool is a database-backed worker pool that claims and processes pending tasks.
type Pool struct {
	db       *bun.DB
	handlers map[string]TaskHandler
	notify   chan struct{}
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	started  bool
}

// NewPool creates a new worker pool with an empty handler registry.
func NewPool(db *bun.DB) *Pool {
	return &Pool{
		db:       db,
		handlers: make(map[string]TaskHandler),
		notify:   make(chan struct{}, 1),
	}
}

// Register adds a task handler. Must be called before Start.
func (p *Pool) Register(taskType string, handler TaskHandler) {
	if p.started {
		panic("worker.Pool: Register called after Start")
	}
	p.handlers[taskType] = handler
}

// Submit inserts a pending task and signals workers. Never blocks.
func (p *Pool) Submit(ctx context.Context, taskType string, payload any, priority int) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("worker.Submit: marshal payload: %w", err)
	}

	task := &models.PendingTask{
		ID:       uuid.NewString(),
		TaskType: taskType,
		Payload:  data,
		Priority: priority,
		Status:   "pending",
	}

	_, err = p.db.NewInsert().Model(task).Exec(ctx)
	if err != nil {
		return fmt.Errorf("worker.Submit: insert: %w", err)
	}

	// Non-blocking signal to workers.
	select {
	case p.notify <- struct{}{}:
	default:
	}

	return nil
}

// Start spawns the given number of worker goroutines.
func (p *Pool) Start(ctx context.Context, workers int) {
	p.started = true
	var workerCtx context.Context
	workerCtx, p.cancel = context.WithCancel(ctx)
	for i := range workers {
		p.wg.Add(1)
		go p.run(workerCtx, i)
	}
	slog.Info("worker pool started", "workers", workers)
}

// Shutdown cancels the context and waits for all in-flight tasks to complete.
func (p *Pool) Shutdown() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("worker pool shut down")
}

func (p *Pool) run(ctx context.Context, _ int) {
	defer p.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.notify:
		case <-ticker.C:
		}

		for {
			if ctx.Err() != nil {
				return
			}
			processed := p.claimAndProcess(ctx)
			if !processed {
				break
			}
		}
	}
}

func (p *Pool) claimAndProcess(ctx context.Context) (processed bool) {
	var task models.PendingTask
	err := p.db.NewRaw(`
		UPDATE pending_tasks
		SET status = 'running', claimed_at = now(), attempts = attempts + 1
		WHERE id = (
			SELECT id FROM pending_tasks
			WHERE status = 'pending'
			ORDER BY priority DESC, created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`,
	).Scan(ctx, &task)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		// No rows or DB error — just return.
		return false
	}

	handler, ok := p.handlers[task.TaskType]
	if !ok {
		errMsg := fmt.Sprintf("unknown task type: %s", task.TaskType)
		p.markFailed(context.Background(), task.ID, errMsg)
		return true
	}

	// Recover from panics in handlers.
	// Use context.Background() for mark operations so they survive shutdown cancellation.
	func() {
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("panic: %v", r)
				slog.Error("worker: handler panicked", "task_id", task.ID, "task_type", task.TaskType, "panic", r)
				p.markFailed(context.Background(), task.ID, errMsg)
			}
		}()

		if herr := handler(ctx, &task); herr != nil {
			p.markFailed(context.Background(), task.ID, herr.Error())
		} else {
			p.markDone(context.Background(), task.ID)
		}
	}()

	return true
}

func (p *Pool) markDone(ctx context.Context, taskID string) {
	_, err := p.db.NewRaw(
		`UPDATE pending_tasks SET status = 'done', done_at = now() WHERE id = ?`, taskID,
	).Exec(ctx)
	if err != nil {
		slog.Error("worker: failed to mark task done", "task_id", taskID, "err", err)
	}
}

func (p *Pool) markFailed(ctx context.Context, taskID string, errMsg string) {
	_, err := p.db.NewRaw(
		`UPDATE pending_tasks SET status = 'failed', last_error = ?, done_at = now() WHERE id = ?`,
		errMsg, taskID,
	).Exec(ctx)
	if err != nil {
		slog.Error("worker: failed to mark task failed", "task_id", taskID, "err", err)
	}
}
