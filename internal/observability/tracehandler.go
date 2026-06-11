package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/drzero42/nexorious/internal/logging"
)

// traceContextHandler wraps an inner slog.Handler and, on every record with an
// active span in ctx, injects trace_id and span_id. It lives here (not in
// internal/logging) so the logging package stays OTel-free; serve.go chains it
// only when tracing is enabled, keeping the tracing-off path zero-overhead.
type traceContextHandler struct {
	inner slog.Handler
}

// NewTraceContextHandler wraps inner so that the active span's trace_id and
// span_id are added to each emitted record. Records logged without an active
// span pass through untouched.
func NewTraceContextHandler(inner slog.Handler) slog.Handler {
	return &traceContextHandler{inner: inner}
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String(logging.KeyTraceID, sc.TraceID().String()),
			slog.String(logging.KeySpanID, sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithGroup(name)}
}
