# OpenTelemetry Tracing (Opt-in OTLP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate opt-in OTLP/HTTP trace export so a sync renders as one waterfall (`river.work` → external-API spans → DB spans), wrap outbound HTTP clients with otelhttp (client metrics always when metrics are on; spans when tracing is on), and bind `trace_id`/`span_id` onto log lines — all gated on `OTEL_EXPORTER_OTLP_ENDPOINT`, with zero change when unset.

**Architecture:** `observability.Init` grows a tracer-provider branch (OTLP/HTTP exporter + batch processor, W3C propagators, slog-backed error handler) and exposes two new seams: `Providers.TracerProvider` (replaces the three `tracenoop` wirings in serve.go for bunotel + both otelriver middlewares) and `HTTPTransport()` (otelhttp wrapping the logging round-tripper, swapped into all 11 outbound-client construction sites). Trace-log correlation is a new slog wrapper handler in `internal/observability` chained in serve.go only when tracing is enabled; `internal/logging` stays OTel-free.

**Tech Stack:** Go 1.26, `go.opentelemetry.io/otel` v1.44.0 + `otel/sdk` v1.43.0 (already present), new `otel/exporters/otlp/otlptrace/otlptracehttp` v1.43.0, `otelhttp` v0.61.0 (promoted from indirect), Echo v5, River v0.39.0, Bun v1.2.18.

**Spec:** `docs/superpowers/specs/2026-06-11-otel-tracing-sync-pipeline-design.md` (approved 2026-06-11, incl. the metrics-only otelhttp amendment). Branch: `feat/911-otel-tracing` (already created; spec committed).

---

## Design decisions (locked during planning — do not re-litigate)

1. **Gate = config field, behavior = standard env vars.** `OTELExporterOTLPEndpoint` (`env:"OTEL_EXPORTER_OTLP_ENDPOINT"`) only decides *whether* to build the exporter. The exporter and provider read the standard OTel env vars natively (`OTEL_EXPORTER_OTLP_*`, `OTEL_TRACES_SAMPLER(_ARG)`, `OTEL_BSP_*`) — verified `sdk@v1.43.0/trace/sampler_env.go` exists and `otlptracehttp` is env-driven. We pass **no** explicit endpoint/sampler/batch options and hard-code no limits.
2. **`otlptracehttp.New` does not dial the collector at construction** — Init works offline; export failures surface asynchronously via the otel error handler (wired to `slog.Warn`).
3. **`HTTPTransport()` wraps with otelhttp when metrics OR tracing is enabled** (user-approved spec amendment): otelhttp's Transport emits `http.client.*` metrics independently of spans (verified in `otelhttp@v0.61.0/transport.go` — `semconv.NewHTTPClient(c.Meter)` + `RecordMetrics`), which #913's external-API alerts need in metrics-only deployments. Both off (or `Init` never called — CLI paths, service-package unit tests) → plain `logging.NewRoundTripper(nil)`, today's behavior.
4. **otelhttp is outermost, logging round-tripper innermost**, so the per-call log line runs inside the client span and inherits its `trace_id`/`span_id`, and `traceparent` is injected outbound.
5. **Trace-log correlation = `observability.NewTraceContextHandler`**, chained between `logging.ContextHandler` and the JSON handler in serve.go only when tracing is enabled. `internal/logging` gains only two plain string constants (`KeyTraceID`, `KeySpanID`).
6. **bunotel stays at default `WithFormattedQueries(false)`** — verified bunotel v1.2.18 falls back to `event.QueryTemplate` (placeholders, no argument values), so `db.statement` carries no secrets. No code change needed; do not add the option.
7. **`playstationstore.profileHTTPClient` becomes `sync.OnceValue`** — it's a package-level var initialized before `observability.Init` runs; lazy init makes it pick up the real transport. Single call site (`client.go:127`).
8. **Dependency hygiene follows the #910 lesson:** `go get` the new module **without** `go mod tidy` (tidy prunes modules no code imports yet); tidy + nix `vendorHash` refresh is a dedicated task after all imports exist.

---

## File structure

| File | Responsibility | Action |
|---|---|---|
| `internal/config/config.go` | Add `OTELExporterOTLPEndpoint` field | Modify |
| `internal/config/config_test.go` | Default-empty + override tests | Modify |
| `internal/observability/observability.go` | Init restructure: resource shared, tracing branch, composed shutdown, package state for `HTTPTransport` | Modify |
| `internal/observability/observability_test.go` | Tracer-provider gating tests | Modify |
| `internal/observability/transport.go` | `HTTPTransport()` accessor | Create |
| `internal/observability/transport_test.go` | Wrap/plain gating + traceparent-injection test | Create |
| `internal/observability/tracehandler.go` | `NewTraceContextHandler` slog wrapper | Create |
| `internal/observability/tracehandler_test.go` | trace_id/span_id present with span, absent without | Create |
| `internal/logging/keys.go` | `KeyTraceID`, `KeySpanID` constants | Modify |
| `cmd/nexorious/serve.go` | Chain trace handler; replace 3 `tracenoop` wirings with `obs.TracerProvider` | Modify |
| `internal/services/{steam,gog,igdb,humble,playstationstore,updatecheck,storelink}` | Swap transports to `observability.HTTPTransport()` (11 sites) | Modify |
| `go.mod` / `go.sum` / `nix/package.nix` | New exporter dep; tidy + vendorHash at the end | Modify |
| `.env.example`, `docs/admin-guide.md`, `DEV.md` | Document tracing | Modify |
| `deploy/helm/values.yaml`, `deploy/helm/README.md` | Commented env example + note | Modify |

---

## Task 1: Config field `OTELExporterOTLPEndpoint`

**Files:**
- Modify: `internal/config/config.go` (after `PprofAddr` at line 146, before the struct's closing `}`)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go` (same fixture env vars as the existing `TestLoad_Observability*` tests in that file):

```go
func TestLoad_TracingEndpointDefaultEmpty(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		t.Errorf("OTELExporterOTLPEndpoint = %q; want empty (tracing off by default)", cfg.OTELExporterOTLPEndpoint)
	}
}

func TestLoad_TracingEndpointOverride(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OTELExporterOTLPEndpoint != "http://localhost:4318" {
		t.Errorf("OTELExporterOTLPEndpoint = %q; want %q", cfg.OTELExporterOTLPEndpoint, "http://localhost:4318")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_TracingEndpoint -v`
Expected: FAIL — `cfg.OTELExporterOTLPEndpoint undefined` (build error).

- [ ] **Step 3: Add the config field**

In `internal/config/config.go`, insert after the `PprofAddr` field (line 146), inside the Observability block:

```go
	// OTELExporterOTLPEndpoint gates trace export: when set (e.g.
	// "http://collector:4318"), an OTLP/HTTP trace exporter is initialized and
	// the drop-in span sources (otelriver, bunotel, otelhttp) start exporting.
	// Unset = tracing fully off (no-op tracer, zero overhead). The exporter and
	// SDK also honor the other standard OTel env vars natively
	// (OTEL_TRACES_SAMPLER, OTEL_TRACES_SAMPLER_ARG, OTEL_BSP_*,
	// OTEL_EXPORTER_OTLP_HEADERS, ...).
	OTELExporterOTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_TracingEndpoint -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add OTEL_EXPORTER_OTLP_ENDPOINT config gate for tracing"
```

---

## Task 2: Add the otlptracehttp dependency (no tidy yet)

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1: `go get` the exporter, pinned to the sdk's version line**

```bash
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.43.0
```

`otel/sdk` is at v1.43.0; the exporter must match it. Do **not** run `go mod tidy` here — tidy prunes modules no code imports yet (this bit #910; see Design decision 8). Tidy happens in Task 8.

- [ ] **Step 2: Verify the module resolved**

Run: `grep otlptrace go.mod`
Expected: `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0` (probably marked `// indirect` until Task 3's import lands — that's fine). A sibling `otlptrace` module line may appear too.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add otlptracehttp exporter dependency"
```

---

## Task 3: Tracer provider in `observability.Init`

Restructure `Init` so the resource is built once, the metrics branch keeps its current behavior, and a new tracing branch builds the OTLP exporter + SDK tracer provider when the endpoint is set (noop otherwise). `Providers` gains `TracerProvider`; shutdown composes tracer-then-meter. Package vars feed Task 4's `HTTPTransport`.

**Files:**
- Modify: `internal/observability/observability.go`
- Test: `internal/observability/observability_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/observability/observability_test.go`:

```go
func TestInit_TracingEnabledYieldsRealTracerProvider(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:          "nexorious-test",
		OTELMetricsEnabled:       true,
		OTELExporterOTLPEndpoint: "http://127.0.0.1:4318",
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() {
		// No collector is listening; the flush inside Shutdown may time out.
		// Bound it tightly and ignore the error — we only assert construction.
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = prov.Shutdown(ctx)
	})

	if prov.TracerProvider == nil {
		t.Fatal("TracerProvider = nil; want non-nil")
	}
	if _, ok := prov.TracerProvider.(*sdktrace.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T; want *sdktrace.TracerProvider when endpoint set", prov.TracerProvider)
	}
}

func TestInit_TracingDisabledYieldsNoopTracerProvider(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: true,
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	if _, ok := prov.TracerProvider.(tracenoop.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T; want tracenoop.TracerProvider when endpoint unset", prov.TracerProvider)
	}
}
```

Add to the test file's imports:

```go
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/observability/ -run TestInit_Tracing -v`
Expected: FAIL — `prov.TracerProvider undefined` (build error).

- [ ] **Step 3: Restructure `observability.go`**

Replace the package comment (lines 1–4) with:

```go
// Package observability bootstraps the OpenTelemetry pipeline: a meter
// provider backed by a dedicated Prometheus registry, the /metrics HTTP
// handler, the nexorious sync-outcome business metrics, and — when an OTLP
// endpoint is configured (OTEL_EXPORTER_OTLP_ENDPOINT) — an OTLP/HTTP trace
// exporter feeding the drop-in span sources (otelriver, bunotel, otelhttp).
```

Update the import block to:

```go
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
```

Replace the package-level `var` block (lines 33–37) with:

```go
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
```

Replace the `Providers` struct and its `Shutdown` method (lines 39–52) with:

```go
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
```

Replace the whole `Init` function with:

```go
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
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
		)
		tp = sdkTP
		tracerShutdown = sdkTP.Shutdown
		otel.SetTracerProvider(sdkTP)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		))
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			slog.Warn("otel export error", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
		}))
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
```

Leave `instrumentationScope`, `MetricsHandler`, `initInstruments`, `RecordSyncOutcome`, and `RecordSyncItems` unchanged.

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/observability/ -v`
Expected: PASS — the two new tests plus the existing `TestInit_EnabledExposesBusinessMetrics` / `TestInit_DisabledIsNoop` (their behavior is unchanged: same handler gating, `Shutdown` still returns nil when nothing real was built).

- [ ] **Step 5: Commit**

```bash
git add internal/observability/observability.go internal/observability/observability_test.go
git commit -m "feat: opt-in OTLP/HTTP tracer provider in observability.Init"
```

---

## Task 4: `HTTPTransport()` accessor

**Files:**
- Create: `internal/observability/transport.go`
- Test: `internal/observability/transport_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/observability/transport_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/observability/ -run TestHTTPTransport -v`
Expected: FAIL — `observability.HTTPTransport undefined` (build error).

- [ ] **Step 3: Create `transport.go`**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/observability/ -run TestHTTPTransport -v`
Expected: PASS (all three).

> Note: these tests mutate package-global state via repeated `Init` calls. The package's tests don't use `t.Parallel()` (same constraint as #910's) — keep it that way.

- [ ] **Step 5: Commit**

```bash
git add internal/observability/transport.go internal/observability/transport_test.go
git commit -m "feat: observability.HTTPTransport — otelhttp over the logging round-tripper"
```

---

## Task 5: Trace-log correlation handler + logging key constants

**Files:**
- Modify: `internal/logging/keys.go`
- Create: `internal/observability/tracehandler.go`
- Test: `internal/observability/tracehandler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/observability/tracehandler_test.go`:

```go
package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/drzero42/nexorious/internal/observability"
)

func logLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("parse log line %q: %v", buf.String(), err)
	}
	return m
}

func TestTraceContextHandler_ActiveSpanAddsIDs(t *testing.T) {
	tid, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	sid, _ := trace.SpanIDFromHex("0102030405060708")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))

	var buf bytes.Buffer
	logger := slog.New(observability.NewTraceContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(ctx, "hello")

	m := logLine(t, &buf)
	if m["trace_id"] != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("trace_id = %v; want 0102030405060708090a0b0c0d0e0f10", m["trace_id"])
	}
	if m["span_id"] != "0102030405060708" {
		t.Errorf("span_id = %v; want 0102030405060708", m["span_id"])
	}
}

func TestTraceContextHandler_NoSpanAddsNothing(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(observability.NewTraceContextHandler(slog.NewJSONHandler(&buf, nil)))
	logger.InfoContext(context.Background(), "hello")

	m := logLine(t, &buf)
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id present without an active span")
	}
	if _, ok := m["span_id"]; ok {
		t.Error("span_id present without an active span")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/observability/ -run TestTraceContextHandler -v`
Expected: FAIL — `observability.NewTraceContextHandler undefined` (build error).

- [ ] **Step 3: Add the key constants**

In `internal/logging/keys.go`, append to the existing `Key*` const block (after `KeyErr` at line 24):

```go
	KeyTraceID        = "trace_id"
	KeySpanID         = "span_id"
```

- [ ] **Step 4: Create `tracehandler.go`**

```go
package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/drzero42/nexorious/internal/logging"
)

// traceContextHandler wraps an inner slog.Handler and, on every record with an
// active span in ctx, injects trace_id and span_id. It lives here (not in
// internal/logging) so the logging package stays OTel-free; serve.go chains it
// only when tracing is enabled, keeping the tracing-off path zero-overhead.
type traceContextHandler struct {
	inner slog.Handler
}

// NewTraceContextHandler wraps inner so that the active span's trace_id and
// span_id are added to each emitted record. Records logged without an active
// span pass through untouched.
func NewTraceContextHandler(inner slog.Handler) slog.Handler {
	return &traceContextHandler{inner: inner}
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String(logging.KeyTraceID, sc.TraceID().String()),
			slog.String(logging.KeySpanID, sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithGroup(name)}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/observability/ -run TestTraceContextHandler -v`
Expected: PASS (both).

- [ ] **Step 6: Commit**

```bash
git add internal/logging/keys.go internal/observability/tracehandler.go internal/observability/tracehandler_test.go
git commit -m "feat: bind trace_id/span_id onto log lines via a chained slog handler"
```

---

## Task 6: Wire tracing into serve.go

No new unit test — the gate is `go build ./...` plus existing tests staying green; the behavioral coverage came from Tasks 3–5.

**Files:**
- Modify: `cmd/nexorious/serve.go`

- [ ] **Step 1: Chain the trace handler into the slog stack**

Replace (lines 65–67):

```go
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	}))))
```

with:

```go
	var appHandler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})
	if cfg.OTELExporterOTLPEndpoint != "" {
		// Tracing enabled: bind trace_id/span_id from the active span onto
		// every log line. Chained only when tracing is on so the off path
		// stays zero-overhead.
		appHandler = observability.NewTraceContextHandler(appHandler)
	}
	slog.SetDefault(slog.New(logging.NewContextHandler(appHandler)))
```

- [ ] **Step 2: Replace the noop tracer with the real provider**

1. Delete line 78: `noopTracer := tracenoop.NewTracerProvider()` and the import `tracenoop "go.opentelemetry.io/otel/trace/noop"` (line 21).
2. Update the section comment (lines 70–71) from `Observability (metrics) — must precede DB + River wiring so the bunotel
   hook and otelriver middleware can bind to the meter provider.` to `Observability (metrics + opt-in tracing) — must precede DB + River wiring
   so the bunotel hook and otelriver middleware can bind to the providers.`
3. In the bunotel hook (lines 93–96): `bunotel.WithTracerProvider(noopTracer)` → `bunotel.WithTracerProvider(obs.TracerProvider)`.
4. In the primary River client middleware (line 266): `TracerProvider: noopTracer,` → `TracerProvider: obs.TracerProvider,`.
5. In the rebuild River client middleware (line 372): `TracerProvider: noopTracer,` → `TracerProvider: obs.TracerProvider,`.

- [ ] **Step 3: Build and run the cmd tests**

```bash
go build ./...
go test ./cmd/nexorious/ 2>&1 | tail -5
```
Expected: build succeeds; existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat: wire tracer provider into bunotel, otelriver, and the log handler chain"
```

---

## Task 7: Swap the 11 outbound-client transport sites

Mechanical: replace `logging.NewRoundTripper(nil)` with `observability.HTTPTransport()` at every construction site; `playstationstore.profileHTTPClient` additionally becomes lazy. No new tests — the pre-`Init` fallback in `HTTPTransport()` returns the plain logging round-tripper, so every existing service test behaves identically.

In each file: add the import `"github.com/drzero42/nexorious/internal/observability"`, and **remove the `logging` import only if nothing else in the file uses it** (the build/lint hook will flag an unused import; several of these files use `logging.Key*` elsewhere).

**Files (all Modify):**

- [ ] **Step 1: `internal/services/steam/client.go:30`**

```go
		http:           &http.Client{Transport: observability.HTTPTransport()},
```

- [ ] **Step 2: `internal/services/gog/client.go:27` and `:38`** (both constructors)

```go
		httpClient: &http.Client{Transport: observability.HTTPTransport()},
```

- [ ] **Step 3: `internal/services/igdb/igdb.go:54`**

```go
		httpClient:    &http.Client{Timeout: 30 * time.Second, Transport: observability.HTTPTransport()},
```

- [ ] **Step 4: `internal/services/igdb/auth.go:38`**

```go
		httpClient:   &http.Client{Timeout: 10 * time.Second, Transport: observability.HTTPTransport()},
```

- [ ] **Step 5: `internal/services/humble/client.go:38`**

```go
		httpClient: &http.Client{Transport: observability.HTTPTransport()},
```

- [ ] **Step 6: `internal/services/playstationstore/client.go`** — three edits:

Replace the package-level var (lines 35–37):

```go
// profileHTTPClient lazily builds the package-level client used by the
// package-level fetchMyProfile helper, which has no access to a *Client
// receiver. Lazy (sync.OnceValue) so it picks up the otel transport wired by
// observability.Init, which runs after package init.
var profileHTTPClient = sync.OnceValue(func() *http.Client {
	return &http.Client{Transport: observability.HTTPTransport()}
})
```

Update its single call site (line 127): `profileHTTPClient.Do(req)` → `profileHTTPClient().Do(req)`.

Replace in `NewClient` (line 53):

```go
		httpClient:      &http.Client{Transport: observability.HTTPTransport()},
```

Add `"sync"` to the imports.

- [ ] **Step 7: `internal/services/updatecheck/client.go:35`**

```go
		httpClient: &http.Client{Timeout: 30 * time.Second, Transport: observability.HTTPTransport()},
```

- [ ] **Step 8: `internal/services/storelink/resolver.go:42` and `:90`** (the nil-client fallbacks in `NewGOGResolver` / `NewEpicResolver`)

```go
		httpClient = &http.Client{Transport: observability.HTTPTransport()}
```

- [ ] **Step 9: Build and run the services tests**

```bash
go build ./...
go test ./internal/services/... 2>&1 | tail -10
```
Expected: build clean; all service tests pass unchanged (their clients get the plain fallback since `Init` isn't called there).

- [ ] **Step 10: Commit**

```bash
git add internal/services/
git commit -m "feat: route outbound service HTTP clients through observability.HTTPTransport"
```

---

## Task 8: `go mod tidy` + nix vendorHash

**Files:** `go.mod`, `go.sum`, `nix/package.nix`

- [ ] **Step 1: Tidy (now that imports exist) and build**

```bash
go mod tidy
go build ./...
```
Expected: `otlptracehttp` (and `otelhttp`) reclassified as direct deps; build clean. Verify with `grep -E "otlptracehttp|otelhttp" go.mod` — neither should say `// indirect` anymore.

- [ ] **Step 2: Refresh `vendorHash`**

```bash
# In nix/package.nix set: vendorHash = pkgs.lib.fakeHash; then:
nix build .#nexorious 2>&1 | grep "got:"
# paste the got: hash into nix/package.nix → vendorHash
```
If `nix` is unavailable in this shell, leave `vendorHash` untouched and note it in the PR description — the CI nix workflow auto-patches it on the PR; verify it lands.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum nix/package.nix
git commit -m "build: tidy go.mod and refresh nix vendorHash for tracing deps"
```

---

## Task 9: Documentation (.env.example, admin-guide, DEV.md)

**Files:**
- Modify: `.env.example` (Observability block, after `PPROF_ADDR` at line 23)
- Modify: `docs/admin-guide.md` (env reference table ~line 235; Monitoring and operations section ~line 351)
- Modify: `DEV.md` (`## Observability` section at line 182)

- [ ] **Step 1: `.env.example`** — append to the Observability block:

```bash
# Opt-in OTLP trace export: set to your collector's OTLP/HTTP endpoint to turn
# on tracing (spans from River jobs, outbound API calls, and DB queries).
# Unset = tracing fully off. Sampling/batching follow the standard OTel env
# vars (OTEL_TRACES_SAMPLER, OTEL_TRACES_SAMPLER_ARG, OTEL_BSP_*).
#OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

- [ ] **Step 2: admin-guide env table** — add a row to the **Observability** table (after the `PPROF_ADDR` row):

```markdown
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Set to an OTLP/HTTP collector endpoint (e.g. `http://collector:4318`) to enable trace export. Unset = tracing off entirely, with no overhead. Sampling and batching follow the standard OTel env vars (`OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`, `OTEL_BSP_*`). |
```

- [ ] **Step 3: admin-guide Monitoring section** — insert a **Tracing** bullet between the **Metrics** and **Profiling (pprof)** bullets:

```markdown
- **Tracing** — set `OTEL_EXPORTER_OTLP_ENDPOINT` to an OTLP/HTTP collector (e.g. `http://collector:4318`) and Nexorious exports traces: each background job becomes one waterfall — the River job at the root, outbound storefront/IGDB API calls beneath it, and the individual database queries under those. Outbound requests also carry a `traceparent` header, and every log line written inside a traced job or request includes matching `trace_id`/`span_id` fields, so you can jump from a log line to the exact trace. Everything is sampled by default; dial it down with the standard `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` env vars. Two things to know: queries and API calls made while serving ordinary HTTP requests appear as small standalone traces (there is deliberately no HTTP-server span), and span attributes carry no secrets — SQL statements are recorded as templates with placeholders, never argument values. Leave the variable unset and tracing is off entirely.
```

- [ ] **Step 4: DEV.md** — two edits in `## Observability`:

Replace the intro sentence:

```markdown
Nexorious ships an OpenTelemetry metrics pipeline plus an opt-in pprof endpoint. Tracing is currently a **no-op** — the SDK seams are wired with no-op tracer providers; issue #911 adds the OTLP trace exporter on top.
```

with:

```markdown
Nexorious ships an OpenTelemetry metrics pipeline, opt-in OTLP tracing, and an opt-in pprof endpoint.
```

Then append a new subsection after the **Profiling with pprof** block (end of the section, before `## Test Coverage`):

```markdown
**Tracing:**

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to an OTLP/HTTP endpoint and the drop-in span sources start exporting: `otelriver` (one root span per River job), `otelhttp` (outbound API calls — wired via `observability.HTTPTransport()`, which every service client uses), and `bunotel` (DB queries). A sync renders as one waterfall: `river.work` → external API spans → query spans.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 ./nexorious serve
```

With tracing on, log lines emitted inside a traced job/request carry `trace_id`/`span_id` (added by `observability.NewTraceContextHandler`, chained in `serve.go`). Unset the variable and tracing is fully off — no exporter, noop tracer providers, plain log lines. Sampling and batching follow the standard OTel env vars (`OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`, `OTEL_BSP_*`); the default samples everything. Until the local Grafana stack lands (#912), view traces with any OTLP backend — e.g. `docker run --rm -p 3000:3000 -p 4318:4318 grafana/otel-lgtm` and open Grafana at `http://localhost:3000`.
```

- [ ] **Step 5: Verify the embedded guide still builds**

Run: `go build ./...`
Expected: clean (`admin-guide.md` is embedded via `docs/embed.go`).

- [ ] **Step 6: Commit**

```bash
git add .env.example docs/admin-guide.md DEV.md
git commit -m "docs: document opt-in OTLP tracing"
```

---

## Task 10: Helm chart — commented env example + README note

**Files:**
- Modify: `deploy/helm/values.yaml` (env block, after `PPROF_ADDR: "127.0.0.1:6060"` at line 429)
- Modify: `deploy/helm/README.md`

- [ ] **Step 1: values.yaml** — append to the observability env entries (keeping the existing comment style):

```yaml
          # Opt-in OTLP trace export — uncomment and point at your collector's
          # OTLP/HTTP endpoint to enable tracing (see the admin guide). Tune
          # sampling with the standard OTEL_TRACES_SAMPLER(_ARG) vars.
          # OTEL_EXPORTER_OTLP_ENDPOINT: http://collector.monitoring:4318
          # OTEL_TRACES_SAMPLER: parentbased_traceidratio
          # OTEL_TRACES_SAMPLER_ARG: "0.25"
```

- [ ] **Step 2: README** — first inspect `deploy/helm/README.md`'s heading structure (`grep -n "^## " deploy/helm/README.md`), then add a short `## Observability` section (place it after the configuration/secrets material, matching the file's tone):

```markdown
## Observability

Metrics are exposed at `/metrics` (on by default — see the admin guide). To
also export traces, set the OTLP endpoint on the main container via the env
block:

```yaml
controllers:
  main:
    containers:
      main:
        env:
          OTEL_EXPORTER_OTLP_ENDPOINT: http://collector.monitoring:4318
```

Tracing is off unless this variable is set. Sampling follows the standard
`OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` env vars (default: sample
everything). ServiceMonitor and dashboard delivery are tracked in #912.
```

> Adjust the `controllers.main.containers.main.env` path to match the chart's actual values layout — copy the nesting used by the existing env block in `values.yaml` (it sits under the bjw-s common-library values; verify the exact key path with `yq eval '.controllers' deploy/helm/values.yaml` or by reading the surrounding structure).

- [ ] **Step 3: Lint the chart**

```bash
helm lint --strict deploy/helm
```
Expected: green — comments and README changes can't affect templates, but verify anyway.

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/values.yaml deploy/helm/README.md
git commit -m "docs: helm — commented OTLP tracing env example"
```

---

## Task 11: Full verification + PR

- [ ] **Step 1: Full build + suites + lint**

```bash
go build ./...
go test -timeout 600s ./... 2>&1 | tail -20
golangci-lint run 2>&1 | tail -20
```
Expected: build clean; all tests pass; zero lint findings.

- [ ] **Step 2: Smoke test — tracing OFF (default) is unchanged**

```bash
make build
DB_ENCRYPTION_KEY=test-db-encryption-key-32-bytes!! DATABASE_URL="$DATABASE_URL" ./nexorious serve &
SERVER_PID=$!
sleep 4
curl -s http://localhost:8000/health
curl -s http://localhost:8000/metrics | head -3
kill -TERM $SERVER_PID
wait $SERVER_PID
```
Expected: health OK, metrics scrape OK, logs show **no** `trace_id` fields, clean `shutdown complete`.

- [ ] **Step 3: Smoke test — tracing ON (no collector needed)**

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
DB_ENCRYPTION_KEY=test-db-encryption-key-32-bytes!! DATABASE_URL="$DATABASE_URL" ./nexorious serve &
SERVER_PID=$!
sleep 8
curl -s http://localhost:8000/health
kill -TERM $SERVER_PID
wait $SERVER_PID
```
Expected: server runs normally; once the batch processor first tries to flush (or on shutdown), a `otel export error` **warn** line appears (nothing listens on 4318) — that is the error handler working, not a failure. Shutdown completes within the timeout. Optionally repeat with `docker run --rm -p 4318:4318 -p 3000:3000 grafana/otel-lgtm` running and trigger a sync to view a real waterfall.

- [ ] **Step 4: Verify acceptance criteria from issue #911**

- [ ] With `OTEL_EXPORTER_OTLP_ENDPOINT` set, a sync produces a trace `river.work` → external-API spans → DB spans (verified live with otel-lgtm, or accepted via the Task 3–5 wiring tests if no Docker available — note which in the PR).
- [ ] Unset ⇒ no trace exporter initialized, noop providers, no extra log handler (Tasks 3/6 + Step 2 smoke).
- [ ] With tracing enabled, log lines inside jobs/requests carry `trace_id`/`span_id` matching the spans; off ⇒ no such fields (Task 5 tests + smoke tests).
- [ ] Tracer provider flushes/shuts down cleanly on SIGTERM (Step 3 smoke; shutdown composes tracer-then-meter).
- [ ] No secrets in span attributes (bunotel template-only statements — Design decision 6; otelhttp records URL/method only).

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin feat/911-otel-tracing
gh pr create --title "feat: opt-in OTLP tracing for the sync pipeline" --body "$(cat <<'EOF'
Implements #911 (part of the #909 observability epic). Design spec: docs/superpowers/specs/2026-06-11-otel-tracing-sync-pipeline-design.md.

## What
- Opt-in OTLP/HTTP trace exporter, gated on `OTEL_EXPORTER_OTLP_ENDPOINT` (unset = today's no-op behavior, zero overhead). Sampling/batching/headers follow the standard OTel env vars; no hard-coded limits.
- `obs.TracerProvider` replaces the no-op tracer in the bunotel hook and both otelriver middlewares — a sync now renders as one waterfall: `river.work` → external-API spans → DB query spans.
- All outbound service HTTP clients route through `observability.HTTPTransport()`: otelhttp (client spans + `traceparent` injection + `http.client.*` metrics) over the existing logging round-tripper. Per the spec amendment, otelhttp wraps in metrics-only mode too, so its client metrics feed #913's external-API alert rules without requiring tracing.
- Trace-log correlation: `observability.NewTraceContextHandler` adds `trace_id`/`span_id` to log lines from the active span; chained only when tracing is enabled. `internal/logging` stays OTel-free.
- Docs (`.env.example`, admin guide, DEV.md) and a commented Helm env example.

## Notes
- The issue prescribed a `logger(ctx)` accessor for correlation; #907 actually landed the per-record `ContextHandler` seam, so this uses a chained handler instead (see the spec's "Corrected premise").
- `db.statement` span attributes carry query templates only (verified bunotel default) — no argument values, no secrets.

Closes #911
EOF
)"
```

---

## Self-review

**Spec coverage:** gating + SDK-env behavior → Tasks 1, 3. Tracer provider + propagators + error handler + composed shutdown → Task 3. `HTTPTransport` incl. metrics-only amendment + pre-Init fallback → Task 4. Trace-log handler + key constants → Task 5. serve.go (3 noop replacements, conditional handler chain) → Task 6. 11 client sites incl. lazy `profileHTTPClient` → Task 7. Deps + vendorHash → Tasks 2, 8. Docs → Task 9. Helm → Task 10. Acceptance criteria → Task 11.

**Type consistency:** `Providers.TracerProvider trace.TracerProvider`; package vars `meterProvider`/`tracerProvider`/`instrumentHTTP` set in Task 3, read in Task 4; `NewTraceContextHandler(inner slog.Handler) slog.Handler` used identically in Tasks 5, 6, 9; `HTTPTransport() http.RoundTripper` used identically in Tasks 4, 7; `logging.KeyTraceID`/`KeySpanID` defined Task 5, referenced in tracehandler.go.

**Open items flagged inline (not placeholders):** the exact bjw-s values nesting for the README example (Task 10 Step 2 — copy from values.yaml); whether `nix` is available for the vendorHash refresh (Task 8 — CI fallback documented); live-waterfall verification needs Docker (Task 11 Step 4 — fallback documented).
