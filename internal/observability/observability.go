// Package observability bootstraps the OpenTelemetry metrics pipeline: a meter
// provider backed by a dedicated Prometheus registry, the /metrics HTTP handler,
// and the nexorious sync-outcome business metrics. Tracing is intentionally a
// no-op here; issue #911 adds the OTLP trace exporter on top of this scaffolding.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

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
)

// Providers holds the initialized meter provider and a shutdown hook. It is the
// seam #911 extends with a tracer provider.
type Providers struct {
	MeterProvider otelmetric.MeterProvider
	shutdown      func(context.Context) error
}

// Shutdown flushes and stops the meter provider. Safe to call once on exit.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// MetricsHandler returns the Prometheus scrape handler, or nil when metrics are
// disabled. The router uses this to decide whether to mount /metrics.
func MetricsHandler() http.Handler { return metricsHandler }

// Init wires the meter provider. When cfg.OTELMetricsEnabled is false it installs
// a no-op meter provider and leaves MetricsHandler() nil; the record helpers then
// no-op safely. version becomes the service.version resource attribute.
func Init(cfg *config.Config, version string) (*Providers, error) {
	if !cfg.OTELMetricsEnabled {
		mp := metricnoop.NewMeterProvider()
		otel.SetMeterProvider(mp)
		initInstruments(mp)
		metricsHandler = nil
		return &Providers{MeterProvider: mp, shutdown: func(context.Context) error { return nil }}, nil
	}

	reg := prometheus.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, fmt.Errorf("observability: prometheus exporter: %w", err)
	}

	// Resource attributes as plain keys to avoid pinning a semconv version.
	res := resource.NewSchemaless(
		attribute.String("service.name", cfg.OTELServiceName),
		attribute.String("service.version", version),
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	initInstruments(mp)
	metricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	return &Providers{MeterProvider: mp, shutdown: mp.Shutdown}, nil
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
