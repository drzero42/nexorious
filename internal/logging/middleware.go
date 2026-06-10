package logging

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkerMiddleware binds the River job id into ctx (so every in-job log line is
// correlated via ContextHandler) and emits exactly one outcome line per job with
// job_type, outcome, and duration_ms.
type WorkerMiddleware struct {
	river.MiddlewareDefaults
}

// NewWorkerMiddleware constructs the job logging middleware.
func NewWorkerMiddleware() *WorkerMiddleware { return &WorkerMiddleware{} }

func (m *WorkerMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	id := strconv.FormatInt(job.ID, 10)
	ctx = WithJobID(ctx, id)

	start := time.Now()
	err := doInner(ctx)
	dur := time.Since(start).Milliseconds()

	outcome := "completed"
	level := slog.LevelInfo
	if err != nil {
		outcome = "failed"
		level = slog.LevelWarn
	}
	slog.Log(ctx, level, "job finished",
		KeyJobType, job.Kind,
		KeyOutcome, outcome,
		KeyDurationMS, dur,
	)
	return err
}
