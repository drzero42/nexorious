package observability

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"github.com/drzero42/nexorious/internal/logging"
)

// HTTPTransport returns the outbound HTTP transport stack for service clients:
// the logging round-tripper, wrapped by otelhttp when metrics or tracing is
// enabled. otelhttp is outermost so the per-call log line runs inside the
// client span (inheriting its trace_id/span_id) and traceparent is injected
// into outbound requests; its http.client.* metrics flow even when tracing is
// off (noop tracer). Before Init runs — CLI paths, service-package unit
// tests — it falls back to the plain logging round-tripper.
func HTTPTransport() http.RoundTripper {
	base := logging.NewRoundTripper(nil)
	if !instrumentHTTP {
		return base
	}
	return otelhttp.NewTransport(base,
		otelhttp.WithTracerProvider(tracerProvider),
		otelhttp.WithMeterProvider(meterProvider),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
	)
}
