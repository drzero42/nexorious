package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/observability"
)

func TestHTTPTransport_AllDisabledIsPlain(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: false,
	}, "test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	if _, ok := observability.HTTPTransport().(*otelhttp.Transport); ok {
		t.Error("HTTPTransport() = *otelhttp.Transport; want plain logging round-tripper when metrics and tracing are both off")
	}
}

func TestHTTPTransport_MetricsOnlyWrapsOtelhttp(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: true,
	}, "test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	if _, ok := observability.HTTPTransport().(*otelhttp.Transport); !ok {
		t.Error("HTTPTransport() not *otelhttp.Transport; want otelhttp wrapping in metrics-only mode (client metrics feed #913)")
	}
}

func TestHTTPTransport_TracingInjectsTraceparent(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:          "nexorious-test",
		OTELMetricsEnabled:       true,
		OTELExporterOTLPEndpoint: "http://127.0.0.1:4318",
	}, "test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = prov.Shutdown(ctx) // no collector listening; flush error is fine
	})

	var gotTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
	}))
	defer srv.Close()

	client := &http.Client{Transport: observability.HTTPTransport()}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()

	// parentbased_always_on (SDK default) samples the root client span, so the
	// W3C propagator must inject a traceparent header. This verifies the whole
	// chain: real provider → otelhttp transport → propagator.
	if gotTraceparent == "" {
		t.Error("traceparent header not injected; otelhttp transport not wired to the real tracer provider")
	}
}
