package logging

import (
	"context"
	"log/slog"
)

// ContextHandler wraps an inner slog.Handler and, on every record, injects
// correlation attributes found in the context (request_id, job_id, user_id).
// Reading ctx per-record (rather than binding once) is the OTel seam: a future
// handler adds trace_id/span_id here without touching any call site (#911).
//
// It is also the log-side credential-redaction choke point (#937): URL query
// strings are scrubbed from the record message and from string/error attribute
// values before the record reaches the inner handler, so a *url.Error carrying
// a credential-bearing query (Steam web_api_key, GOG client_secret) can never
// leak into a log line regardless of which call site logged it — the same
// per-output-channel policy as the trace exporter in internal/observability.
type ContextHandler struct {
	inner slog.Handler
	// groups are the open group names, outermost first. The ContextHandler tracks
	// them itself instead of delegating WithGroup to inner so the correlation ids
	// stay flat at the root (request_id, never sync.request_id) while the record's
	// own attributes remain nested under the groups. See WithGroup.
	groups []string
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
	r = scrubRecord(r)
	corr := correlationAttrs(ctx)

	if len(h.groups) == 0 {
		// Fast path: no open group, so the correlation ids and the record's own
		// attrs are all at the root.
		if len(corr) > 0 {
			r.AddAttrs(corr...)
		}
		return h.inner.Handle(ctx, r)
	}

	// A group is open. The record's attributes belong under it, but the
	// correlation ids must stay flat — so rebuild the record with the correlation
	// ids at the root and the record's attrs nested under the group chain, then
	// hand it to inner (which carries no group of its own).
	var recAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		recAttrs = append(recAttrs, a)
		return true
	})
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(corr...)
	nr.AddAttrs(nestUnderGroups(h.groups, recAttrs)...)
	return h.inner.Handle(ctx, nr)
}

// correlationAttrs collects the ctx-carried correlation ids present on ctx.
func correlationAttrs(ctx context.Context) []slog.Attr {
	var a []slog.Attr
	if v := requestID(ctx); v != "" {
		a = append(a, slog.String(KeyRequestID, v))
	}
	if v := jobID(ctx); v != "" {
		a = append(a, slog.String(KeyJobID, v))
	}
	if v := riverJobID(ctx); v != "" {
		a = append(a, slog.String(KeyRiverJobID, v))
	}
	if v := userID(ctx); v != "" {
		a = append(a, slog.String(KeyUserID, v))
	}
	return a
}

// nestUnderGroups wraps attrs in the group chain (outermost first), so
// ["a","b"] turns attrs into a single {a: {b: attrs}} group attribute.
func nestUnderGroups(groups []string, attrs []slog.Attr) []slog.Attr {
	for i := len(groups) - 1; i >= 0; i-- {
		attrs = []slog.Attr{{Key: groups[i], Value: slog.GroupValue(attrs...)}}
	}
	return attrs
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i], _ = scrubAttr(a)
	}
	// With a group open, pre-bound attrs belong under it too. (No call site
	// chains WithGroup before WithAttrs today, but keep the nesting correct.)
	if len(h.groups) > 0 {
		out = nestUnderGroups(h.groups, out)
	}
	return &ContextHandler{inner: h.inner.WithAttrs(out), groups: h.groups}
}

// scrubRecord returns r with URL query strings stripped from its message and
// attribute values. Records that carry no query-bearing URL — the overwhelming
// majority — are returned unmodified; only a record that needs scrubbing is
// rebuilt (slog.Record attributes cannot be mutated in place).
func scrubRecord(r slog.Record) slog.Record {
	msg := ScrubURLQueries(r.Message)
	changed := msg != r.Message
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		sa, c := scrubAttr(a)
		changed = changed || c
		attrs = append(attrs, sa)
		return true
	})
	if !changed {
		return r
	}
	nr := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	nr.AddAttrs(attrs...)
	return nr
}

// scrubAttr strips URL queries from string and error attribute values,
// recursing into groups. An error value that needs scrubbing is replaced by a
// string attribute holding the scrubbed err.Error() — identical to how
// slog.JSONHandler/TextHandler would have rendered the error anyway. The bool
// reports whether anything changed; an untouched attr is returned as-is.
func scrubAttr(a slog.Attr) (slog.Attr, bool) {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if scrubbed := ScrubURLQueries(s); scrubbed != s {
			return slog.String(a.Key, scrubbed), true
		}
	case slog.KindGroup:
		group := v.Group()
		changed := false
		out := make([]slog.Attr, len(group))
		for i, ga := range group {
			var c bool
			out[i], c = scrubAttr(ga)
			changed = changed || c
		}
		if changed {
			return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}, true
		}
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			s := err.Error()
			if scrubbed := ScrubURLQueries(s); scrubbed != s {
				return slog.String(a.Key, scrubbed), true
			}
		}
	default:
	}
	return a, false
}

// WithGroup records the group locally rather than delegating to inner, so that
// Handle can keep the correlation ids flat at the root while still nesting the
// record's own attributes under the group chain.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, len(h.groups)+1)
	copy(groups, h.groups)
	groups[len(h.groups)] = name
	return &ContextHandler{inner: h.inner, groups: groups}
}
