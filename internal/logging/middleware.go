package logging

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkerMiddleware binds River's internal job id into ctx as river_job_id (so
// every in-job log line is correlated via ContextHandler) and emits exactly one
// outcome line per job with job_type, outcome, and duration_ms. The user-facing
// application job id (jobs.id) is distinct and is seeded by the worker that owns
// it via WithJobID — River's id is a Postgres sequence, not the jobs UUID.
//
// Routine, high-frequency job kinds (periodic maintenance, per-item fan-out work)
// can be passed to NewWorkerMiddleware as "quiet" kinds: their *successful*
// completion logs at Debug instead of Info so they don't drown out user-facing
// job outcomes. Failures always log at Warn regardless of kind.
type WorkerMiddleware struct {
	river.MiddlewareDefaults
	quiet map[string]bool
}

// NewWorkerMiddleware constructs the job logging middleware. Any job kinds passed
// in quietKinds log successful completion at Debug rather than Info.
func NewWorkerMiddleware(quietKinds ...string) *WorkerMiddleware {
	quiet := make(map[string]bool, len(quietKinds))
	for _, k := range quietKinds {
		quiet[k] = true
	}
	return &WorkerMiddleware{quiet: quiet}
}

func (m *WorkerMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	id := strconv.FormatInt(job.ID, 10)
	ctx = WithRiverJobID(ctx, id)

	start := time.Now()
	err := doInner(ctx)
	dur := time.Since(start).Milliseconds()

	outcome := "completed"
	level := slog.LevelInfo
	switch {
	case err != nil:
		outcome = "failed"
		level = slog.LevelWarn
	case m.quiet[job.Kind]:
		level = slog.LevelDebug
	}
	slog.Log(ctx, level, "job finished",
		KeyJobType, job.Kind,
		KeyOutcome, outcome,
		KeyDurationMS, dur,
	)
	return err
}
