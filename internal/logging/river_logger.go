package logging

import (
	"context"
	"log/slog"
)

// RiverLogger returns the *slog.Logger to hand to river.Config.Logger so River's
// own operational lines (queue warnings, the "panic recovery; possible bug with
// Worker" error) flow through the app's JSON pipeline instead of River's default
// text-on-stdout handler.
//
// It also reconciles a key-name conflict: River tags every internal line with
// slog.Int64("job_id", <river job id>), but the logging-conventions contract
// reserves job_id for the jobs.id UUID and uses river_job_id for River's int64.
// keyRenameHandler rewrites that one key so a log shipper keying on job_id never
// sees River's int mixed in with our UUIDs. River never sets our UUID job_id, so
// the rename is unambiguous.
func RiverLogger() *slog.Logger {
	return slog.New(&keyRenameHandler{inner: slog.Default().Handler(), from: KeyJobID, to: KeyRiverJobID})
}

// keyRenameHandler wraps an inner slog.Handler and renames any top-level
// attribute whose key == from to to, on both per-record attrs and attrs bound
// via WithAttrs. It does not recurse into groups (River emits none).
type keyRenameHandler struct {
	inner    slog.Handler
	from, to string
}

func (h *keyRenameHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *keyRenameHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.rename(a))
		return true
	})
	return h.inner.Handle(ctx, nr)
}

func (h *keyRenameHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = h.rename(a)
	}
	return &keyRenameHandler{inner: h.inner.WithAttrs(out), from: h.from, to: h.to}
}

func (h *keyRenameHandler) WithGroup(name string) slog.Handler {
	return &keyRenameHandler{inner: h.inner.WithGroup(name), from: h.from, to: h.to}
}

func (h *keyRenameHandler) rename(a slog.Attr) slog.Attr {
	if a.Key == h.from {
		a.Key = h.to
	}
	return a
}
