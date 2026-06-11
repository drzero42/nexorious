package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// urlAttrKeys are the span attribute keys that carry a full request URL.
// otelhttp emits url.full (current semconv) or http.url (legacy, via
// OTEL_SEMCONV_STABILITY_OPT_IN); redact both.
var urlAttrKeys = map[attribute.Key]struct{}{
	"url.full": {},
	"http.url": {},
}

// redactingExporter wraps a SpanExporter and strips query strings from URL
// span attributes before export. Outbound APIs put credentials in query
// params (Steam web_api_key, GOG client_secret/refresh_token), so the query
// must never reach the trace backend — same policy as the logging
// round-tripper, which logs the path only.
type redactingExporter struct {
	inner sdktrace.SpanExporter
}

func newRedactingExporter(inner sdktrace.SpanExporter) sdktrace.SpanExporter {
	return &redactingExporter{inner: inner}
}

func (e *redactingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	out := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		out[i] = redactSpan(s)
	}
	return e.inner.ExportSpans(ctx, out)
}

func (e *redactingExporter) Shutdown(ctx context.Context) error {
	return e.inner.Shutdown(ctx)
}

// redactedSpan overrides Attributes() on an embedded ReadOnlySpan. Embedding
// satisfies the interface's unexported method; every other accessor passes
// through to the original span.
type redactedSpan struct {
	sdktrace.ReadOnlySpan
	attrs []attribute.KeyValue
}

func (s redactedSpan) Attributes() []attribute.KeyValue { return s.attrs }

// redactSpan returns the span unchanged when no URL attribute carries a
// query; otherwise a wrapper whose URL attributes are truncated at '?'.
func redactSpan(s sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	attrs := s.Attributes()
	redacted := false
	var out []attribute.KeyValue
	for i, kv := range attrs {
		if _, ok := urlAttrKeys[kv.Key]; !ok {
			continue
		}
		v := kv.Value.AsString()
		q := strings.IndexByte(v, '?')
		if q < 0 {
			continue
		}
		if !redacted {
			out = make([]attribute.KeyValue, len(attrs))
			copy(out, attrs)
			redacted = true
		}
		out[i] = attribute.String(string(kv.Key), v[:q])
	}
	if !redacted {
		return s
	}
	return redactedSpan{ReadOnlySpan: s, attrs: out}
}
