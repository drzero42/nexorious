package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkerErrorHandler implements river.ErrorHandler to surface recovered worker
// panics as structured slog error lines (category=panic). Normal job errors are
// already logged once at warn by WorkerMiddleware, so HandleError is a no-op to
// avoid double-logging; only panics — which unwind past the middleware and so are
// never logged by it — need surfacing here.
type WorkerErrorHandler struct{}

// HandleError is a no-op: WorkerMiddleware already emits the failed-outcome line.
func (h *WorkerErrorHandler) HandleError(_ context.Context, _ *rivertype.JobRow, _ error) *river.ErrorHandlerResult {
	return nil
}

// HandlePanic emits a category=panic error line for a recovered worker panic.
// River calls this above all middleware, so the ctx carries no middleware-set
// correlation ids — river_job_id is added explicitly here (it cannot be injected
// by ContextHandler at this boundary). Returning nil keeps River's default retry
// behavior.
func (h *WorkerErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	slog.ErrorContext(ctx, "worker: recovered panic",
		KeyJobType, job.Kind,
		KeyRiverJobID, strconv.FormatInt(job.ID, 10),
		KeyErr, fmt.Sprintf("%v", panicVal),
		Cat(CategoryPanic),
	)
	return nil
}

// compile-time assertion that the handler satisfies the interface.
var _ river.ErrorHandler = (*WorkerErrorHandler)(nil)
