package logging

import (
	"context"
	"log/slog"
)

// ContextHandler wraps an inner slog.Handler and, on every record, injects
// correlation attributes found in the context (request_id, job_id, user_id).
// Reading ctx per-record (rather than binding once) is the OTel seam: a future
// handler adds trace_id/span_id here without touching any call site (#911).
type ContextHandler struct {
	inner slog.Handler
}

// NewContextHandler wraps inner so that ctx-carried correlation ids are added
// to each emitted record.
func NewContextHandler(inner slog.Handler) *ContextHandler {
	return &ContextHandler{inner: inner}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v := requestID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyRequestID, v))
	}
	if v := jobID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyJobID, v))
	}
	if v := userID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyUserID, v))
	}
	return h.inner.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{inner: h.inner.WithGroup(name)}
}
