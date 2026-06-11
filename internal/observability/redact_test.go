package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestRedactingExporter_StripsQueryFromURLAttrs verifies end-to-end that the
// redacting exporter removes query strings from url.full and http.url span
// attributes, leaving all other attributes untouched.
func TestRedactingExporter_StripsQueryFromURLAttrs(t *testing.T) {
	rec := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(newRedactingExporter(rec)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.SetAttributes(
		attribute.String("url.full", "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=SECRET&steamid=123"),
		attribute.String("http.url", "https://auth.gog.com/token?client_secret=SECRET2"),
		attribute.String("http.request.method", "GET"),
	)
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	s := spans[0]

	attrMap := make(map[attribute.Key]string)
	for _, kv := range s.Attributes {
		if kv.Value.Type() == attribute.STRING {
			attrMap[kv.Key] = kv.Value.AsString()
		}
	}

	if got, want := attrMap["url.full"], "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/"; got != want {
		t.Errorf("url.full = %q; want %q", got, want)
	}
	if got, want := attrMap["http.url"], "https://auth.gog.com/token"; got != want {
		t.Errorf("http.url = %q; want %q", got, want)
	}
	if got, want := attrMap["http.request.method"], "GET"; got != want {
		t.Errorf("http.request.method = %q; want %q (unrelated attr must be untouched)", got, want)
	}

	// Absolute guard: no attribute value may contain either secret.
	for _, kv := range s.Attributes {
		v := kv.Value.AsString()
		if strings.Contains(v, "SECRET") {
			t.Errorf("attribute %q still contains a secret: %q", kv.Key, v)
		}
	}
}

// TestRedactingExporter_NoQueryPassesThrough verifies that a url.full without a
// query string is exported unchanged (zero allocation path).
func TestRedactingExporter_NoQueryPassesThrough(t *testing.T) {
	rec := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(newRedactingExporter(rec)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	const noQueryURL = "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/"

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "no-query-span")
	span.SetAttributes(attribute.String("url.full", noQueryURL))
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	s := spans[0]

	for _, kv := range s.Attributes {
		if kv.Key == "url.full" {
			if got := kv.Value.AsString(); got != noQueryURL {
				t.Errorf("url.full = %q; want %q (no-query URL must pass through unchanged)", got, noQueryURL)
			}
			return
		}
	}
	t.Error("url.full attribute not found in recorded span")
}

// TestRedactingExporter_ScrubsStatusDescription verifies that URL query strings
// embedded in a span's status description are stripped. Go's *url.Error embeds
// the full request URL in its message, and otelriver/bunotel copy err.Error()
// into the status description — so a failed Steam/GOG call would otherwise
// carry credentials there.
func TestRedactingExporter_ScrubsStatusDescription(t *testing.T) {
	rec := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(newRedactingExporter(rec)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "failed-job")
	span.SetStatus(codes.Error, `Get "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=SECRET&steamid=123": connection refused`)
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	got := spans[0].Status.Description
	want := `Get "https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/": connection refused`
	if got != want {
		t.Errorf("status description = %q; want %q", got, want)
	}
	if strings.Contains(got, "SECRET") {
		t.Errorf("status description still contains a secret: %q", got)
	}
}

// TestRedactingExporter_ScrubsExceptionEvents verifies that URL query strings in
// span event attributes (exception.message from span.RecordError) are stripped.
func TestRedactingExporter_ScrubsExceptionEvents(t *testing.T) {
	rec := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(newRedactingExporter(rec)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "failed-call")
	span.RecordError(errors.New(`Get "https://auth.gog.com/token?client_secret=SECRET&refresh_token=SECRET2": EOF`))
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	if len(spans[0].Events) == 0 {
		t.Fatal("no events recorded")
	}
	found := false
	for _, ev := range spans[0].Events {
		for _, kv := range ev.Attributes {
			v := kv.Value.AsString()
			if strings.Contains(v, "SECRET") {
				t.Errorf("event attribute %q still contains a secret: %q", kv.Key, v)
			}
			if kv.Key == "exception.message" {
				found = true
				if want := `Get "https://auth.gog.com/token": EOF`; v != want {
					t.Errorf("exception.message = %q; want %q", v, want)
				}
			}
		}
	}
	if !found {
		t.Error("exception.message attribute not found in recorded events")
	}
}

// TestRedactingExporter_PlainErrorTextPassesThrough verifies that error text
// without an embedded URL is exported unchanged.
func TestRedactingExporter_PlainErrorTextPassesThrough(t *testing.T) {
	rec := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(newRedactingExporter(rec)),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	const plain = "dial tcp 10.0.0.1:443: connect: connection refused"

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "plain-failure")
	span.SetStatus(codes.Error, plain)
	span.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans recorded")
	}
	if got := spans[0].Status.Description; got != plain {
		t.Errorf("status description = %q; want %q (plain text must pass through)", got, plain)
	}
}
