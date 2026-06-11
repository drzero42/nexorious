// Package observability bootstraps the OpenTelemetry pipeline: a meter
// provider backed by a dedicated Prometheus registry, the /metrics HTTP
// handler, the nexorious sync-outcome business metrics, and — when an OTLP
// endpoint is configured (OTEL_EXPORTER_OTLP_ENDPOINT) — an OTLP/HTTP trace
// exporter feeding the drop-in span sources (otelriver, bunotel, otelhttp).
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/logging"
)

// instrumentationScope names the meter used for business metrics.
const instrumentationScope = "github.com/drzero42/nexorious/internal/observability"

// Package-level state set once by Init. The metrics handler and instruments are
// process-global by design (a single registry per process, mirroring how the
// Prometheus client treats its default registry).
var (
	metricsHandler http.Handler
	syncTotal      otelmetric.Int64Counter
	syncItemsTotal otelmetric.Int64Counter

	// Set by Init; read by HTTPTransport (transport.go). instrumentHTTP is
	// true when metrics or tracing is enabled — otelhttp emits client metrics
	// even with a noop tracer, so the transport wraps in metrics-only mode too.
	meterProvider  otelmetric.MeterProvider
	tracerProvider trace.TracerProvider
	instrumentHTTP bool
)

// Providers holds the initialized meter + tracer providers and a composed
// shutdown hook.
type Providers struct {
	MeterProvider  otelmetric.MeterProvider
	TracerProvider trace.TracerProvider
	shutdown       func(context.Context) error
}

// Shutdown flushes and stops the tracer provider (when tracing is enabled)
// and then the meter provider. Safe to call once on exit.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// MetricsHandler returns the Prometheus scrape handler, or nil when metrics are
// disabled. The router uses this to decide whether to mount /metrics.
func MetricsHandler() http.Handler { return metricsHandler }

// Init wires the meter provider and, when cfg.OTELExporterOTLPEndpoint is set,
// an OTLP/HTTP trace exporter + SDK tracer provider. With metrics disabled it
// installs a no-op meter provider and leaves MetricsHandler() nil; with the
// endpoint unset it installs a no-op tracer provider (tracing fully off).
// version becomes the service.version resource attribute.
func Init(cfg *config.Config, version string) (*Providers, error) {
	// Resource attributes as plain keys to avoid pinning a semconv version.
	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.OTELServiceName),
		attribute.String("service.version", version),
	)

	// --- Metrics ---
	var mp otelmetric.MeterProvider
	var meterShutdown func(context.Context) error
	if cfg.OTELMetricsEnabled {
		reg := prometheus.NewRegistry()
		exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
		if err != nil {
			return nil, fmt.Errorf("observability: prometheus exporter: %w", err)
		}
		sdkMP := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(exporter),
			sdkmetric.WithResource(res),
		)
		mp = sdkMP
		meterShutdown = sdkMP.Shutdown
		metricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	} else {
		mp = metricnoop.NewMeterProvider()
		metricsHandler = nil
	}
	otel.SetMeterProvider(mp)
	initInstruments(mp)

	// Route OTel-internal errors (async export failures etc.) through slog
	// instead of the SDK's unstructured stderr default.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("otel export error", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
	}))

	// --- Tracing (opt-in: only when an OTLP endpoint is configured) ---
	var tp trace.TracerProvider
	var tracerShutdown func(context.Context) error
	if cfg.OTELExporterOTLPEndpoint != "" {
		// No explicit options: the exporter reads the standard
		// OTEL_EXPORTER_OTLP_* env vars natively (endpoint, headers, TLS,
		// timeout, compression) and the provider reads
		// OTEL_TRACES_SAMPLER(_ARG) and OTEL_BSP_*. Construction does not
		// dial the collector; export failures surface via the error handler.
		exporter, err := otlptracehttp.New(context.Background())
		if err != nil {
			return nil, fmt.Errorf("observability: otlp trace exporter: %w", err)
		}
		sdkTP := sdktrace.NewTracerProvider(
			// The redacting wrapper strips query strings from URL attributes —
			// Steam/GOG put credentials in query params (see redact.go).
			sdktrace.WithBatcher(newRedactingExporter(exporter)),
			sdktrace.WithResource(res),
		)
		tp = sdkTP
		tracerShutdown = sdkTP.Shutdown
		otel.SetTracerProvider(sdkTP)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		))
	} else {
		tp = tracenoop.NewTracerProvider()
	}

	meterProvider = mp
	tracerProvider = tp
	instrumentHTTP = cfg.OTELMetricsEnabled || cfg.OTELExporterOTLPEndpoint != ""

	shutdown := func(ctx context.Context) error {
		var errs []error
		// Tracer first so pending spans flush before anything else stops.
		if tracerShutdown != nil {
			errs = append(errs, tracerShutdown(ctx))
		}
		if meterShutdown != nil {
			errs = append(errs, meterShutdown(ctx))
		}
		return errors.Join(errs...)
	}
	return &Providers{MeterProvider: mp, TracerProvider: tp, shutdown: shutdown}, nil
}

// initInstruments (re)creates the business-metric counters from the given
// provider. Instrument names omit _total; the Prometheus exporter appends it for
// monotonic counters, yielding nexorious_sync_total / nexorious_sync_items_total.
func initInstruments(mp otelmetric.MeterProvider) {
	m := mp.Meter(instrumentationScope)
	var err error
	syncTotal, err = m.Int64Counter(
		"nexorious_sync",
		otelmetric.WithDescription("Count of completed sync jobs by source and final status."),
	)
	if err != nil {
		slog.Error("observability: failed to create nexorious_sync counter", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
	}
	syncItemsTotal, err = m.Int64Counter(
		"nexorious_sync_items",
		otelmetric.WithDescription("Count of synced library items by source and per-item outcome."),
	)
	if err != nil {
		slog.Error("observability: failed to create nexorious_sync_items counter", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
	}
}

// RecordSyncOutcome records one completed sync job. source is a storefront slug
// (e.g. "steam"); status is "completed" or "completed_with_errors". Never label
// by user_id — cardinality must stay bounded.
func RecordSyncOutcome(ctx context.Context, source, status string) {
	if syncTotal == nil {
		return
	}
	syncTotal.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("source", source),
		attribute.String("status", status),
	))
}

// RecordSyncItems records n items for a (source, outcome) pair. outcome is one of
// "completed", "failed", "skipped". A non-positive count is a no-op (counters are
// monotonic; n must be positive).
func RecordSyncItems(ctx context.Context, source, outcome string, n int64) {
	if syncItemsTotal == nil || n <= 0 {
		return
	}
	syncItemsTotal.Add(ctx, n, otelmetric.WithAttributes(
		attribute.String("source", source),
		attribute.String("outcome", outcome),
	))
}
