package observability

import (
	"context"
	"regexp"
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

// urlQueryRe matches an http(s) URL up to its query string. Used to scrub
// queries from free-text span fields (status descriptions, exception-event
// messages): Go's *url.Error embeds the full request URL in its message, and
// otelriver/bunotel copy err.Error() into the span status, so a failed
// storefront call would otherwise carry its query-string credentials there.
var urlQueryRe = regexp.MustCompile(`(https?://[^?\s"']+)\?[^\s"']*`)

// scrubText strips the query string from any URL embedded in free text.
func scrubText(s string) string {
	return urlQueryRe.ReplaceAllString(s, "$1")
}

// redactingExporter wraps a SpanExporter and strips query strings from URL
// span attributes, status descriptions, and event attributes before export.
// Outbound APIs put credentials in query params (Steam web_api_key, GOG
// client_secret/refresh_token), so the query must never reach the trace
// backend — same policy as the logging round-tripper, which logs the path
// only.
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

// redactedSpan overrides Attributes(), Status(), and Events() on an embedded
// ReadOnlySpan. Embedding satisfies the interface's unexported method; every
// other accessor passes through to the original span.
type redactedSpan struct {
	sdktrace.ReadOnlySpan
	attrs  []attribute.KeyValue
	status sdktrace.Status
	events []sdktrace.Event
}

func (s redactedSpan) Attributes() []attribute.KeyValue { return s.attrs }
func (s redactedSpan) Status() sdktrace.Status          { return s.status }
func (s redactedSpan) Events() []sdktrace.Event         { return s.events }

// redactSpan returns the span unchanged when nothing needs redacting;
// otherwise a wrapper whose URL attributes are truncated at '?' and whose
// status description / event attributes have URL queries scrubbed.
func redactSpan(s sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	attrs, attrsChanged := redactURLAttrs(s.Attributes())
	status, statusChanged := redactStatus(s.Status())
	events, eventsChanged := redactEvents(s.Events())
	if !attrsChanged && !statusChanged && !eventsChanged {
		return s
	}
	return redactedSpan{ReadOnlySpan: s, attrs: attrs, status: status, events: events}
}

// redactURLAttrs truncates URL-carrying attributes at '?'. Returns the input
// slice unchanged (and false) when no attribute carries a query.
func redactURLAttrs(attrs []attribute.KeyValue) ([]attribute.KeyValue, bool) {
	changed := false
	out := attrs
	for i, kv := range attrs {
		if _, ok := urlAttrKeys[kv.Key]; !ok {
			continue
		}
		v := kv.Value.AsString()
		q := strings.IndexByte(v, '?')
		if q < 0 {
			continue
		}
		if !changed {
			out = make([]attribute.KeyValue, len(attrs))
			copy(out, attrs)
			changed = true
		}
		out[i] = attribute.String(string(kv.Key), v[:q])
	}
	return out, changed
}

// redactStatus scrubs URL queries from the status description (otelriver and
// bunotel set it to err.Error(), which may embed a full request URL).
func redactStatus(st sdktrace.Status) (sdktrace.Status, bool) {
	scrubbed := scrubText(st.Description)
	if scrubbed == st.Description {
		return st, false
	}
	st.Description = scrubbed
	return st, true
}

// redactEvents scrubs URL queries from string event attributes (notably
// exception.message written by span.RecordError).
func redactEvents(events []sdktrace.Event) ([]sdktrace.Event, bool) {
	changed := false
	out := events
	for i, ev := range events {
		var newAttrs []attribute.KeyValue
		for j, kv := range ev.Attributes {
			if kv.Value.Type() != attribute.STRING {
				continue
			}
			v := kv.Value.AsString()
			scrubbed := scrubText(v)
			if scrubbed == v {
				continue
			}
			if newAttrs == nil {
				newAttrs = make([]attribute.KeyValue, len(ev.Attributes))
				copy(newAttrs, ev.Attributes)
			}
			newAttrs[j] = attribute.String(string(kv.Key), scrubbed)
		}
		if newAttrs == nil {
			continue
		}
		if !changed {
			out = make([]sdktrace.Event, len(events))
			copy(out, events)
			changed = true
		}
		out[i].Attributes = newAttrs
	}
	return out, changed
}
