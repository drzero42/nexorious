package observability

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
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
