# OpenTelemetry Metrics Foundation + pprof Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bootstrap the OTel SDK, expose Prometheus metrics at `/metrics` (always-on, zero collector dependency), wire the free drop-in metric sources (`bunotel`, `otelriver`), add sync-outcome business metrics, and add a localhost-only pprof endpoint — the scaffolding the rest of the observability epic (#911 tracing, #912 deployment, #913 metrics alerts) builds on.

**Architecture:** A new `internal/observability` package owns the OTel meter provider, a dedicated Prometheus registry, the Prometheus HTTP handler, and the two business-metric instruments. `serve.go` calls `observability.Init(cfg, version)` right after config load, sets it as the global meter provider, attaches a metrics-only `bunotel` query hook to the `*bun.DB`, and registers an `otelriver` middleware on **both** River clients (the primary and the restore-rebuild path). The router mounts `/metrics` via a package accessor (so `api.New`'s signature is unchanged). pprof runs on a separate loopback `http.Server`, gated off by default. Tracing stays a no-op (explicit no-op tracer providers) — #911 swaps in the real exporter later.

**Tech Stack:** Go 1.26, `go.opentelemetry.io/otel` (SDK + Prometheus exporter), `github.com/prometheus/client_golang` (registry + `promhttp`), `github.com/uptrace/bun/extra/bunotel`, `github.com/riverqueue/rivercontrib/otelriver`, Echo v5, River v0.39.0, Bun v1.2.18.

---

## Design decisions (locked during planning)

These were verified against the codebase and the upstream library APIs before writing this plan. Do not re-litigate them mid-execution.

1. **pprof = localhost-only listener** (user decision). A separate `http.Server` bound to `127.0.0.1:6060` by default, **not** mounted on the public router and **not** admin-gated. Access in prod is via `kubectl port-forward` + `go tool pprof`. Default OFF via `PPROF_ENABLED=false`.
2. **`/metrics` is unauthenticated and always-on** when `OTEL_METRICS_ENABLED=true` (default). Per the issue, it carries no secrets and bounded-cardinality labels only. It must **bypass the app-state gates** (like `/health`).
3. **No `api.New` signature change.** `echo.WrapHandler` exists in echo v5 (`echo.go:752`). The router pulls the metrics handler from `observability.MetricsHandler()` (a package accessor set during `Init`). Verified there are 5 `_test.go` callers of `api.New`; leaving the signature alone avoids touching all of them.
4. **Business metrics recorded once at the single finalization chokepoint** `SyncCheckJobCompletion` (`internal/worker/tasks/sync.go:955`), not at each high-fan-out `markItem*` site. Because `pending_review` items block finalization, at that point every item is settled (completed/failed/skipped), so one grouped count query yields the full per-job item outcome distribution. Lower overhead, bounded cardinality.
5. **Instrument names omit the `_total` suffix.** The OTel→Prometheus exporter appends `_total` to monotonic counters automatically. Name the instruments `nexorious_sync` and `nexorious_sync_items` so the scrape shows exactly `nexorious_sync_total` and `nexorious_sync_items_total` (the issue's required names).
6. **OTel core bumps v1.41.0 → v1.43.0.** `otelriver` v0.10.0 (latest) requires `go.opentelemetry.io/otel` v1.43.0. River v0.39.0 already satisfies otelriver's v0.29.0 minimum. `go mod tidy` performs the bump; the nix `vendorHash` must be refreshed afterward.
7. **bunotel and otelriver are metrics-only here.** Both take an explicit `MeterProvider` (ours) and an explicit no-op `TracerProvider` so they emit metrics but create no spans. #911 will replace the no-op tracer providers.

---

## File structure

| File | Responsibility | Action |
|---|---|---|
| `internal/config/config.go` | Add `OTELServiceName`, `OTELMetricsEnabled`, `PprofEnabled`, `PprofAddr` env-backed fields | Modify |
| `internal/config/config_test.go` | Assert defaults for the new fields | Modify |
| `internal/observability/observability.go` | Meter provider init, Prometheus registry + handler, business-metric instruments, record helpers, `Shutdown` | Create |
| `internal/observability/observability_test.go` | Init→handler non-nil; recorded counters appear in scrape with correct names + labels; disabled path is no-op | Create |
| `cmd/nexorious/serve.go` | Call `Init`, set global provider, attach bunotel hook, add otelriver middleware to both clients, start pprof, register `Shutdown` | Modify |
| `cmd/nexorious/pprof.go` | `startPprofServer(addr)` helper (loopback `http.Server` with pprof routes) | Create |
| `internal/api/router.go` | Mount `/metrics` via accessor; add `/metrics` to the 3 gate allowlists | Modify |
| `internal/api/router_test.go` | `/metrics` reachable + bypasses gates; returns Prometheus text | Modify |
| `internal/worker/tasks/sync.go` | Record `nexorious_sync_total` + `nexorious_sync_items_total` at finalization; add `syncJobItemStatusCounts` helper | Modify |
| `internal/worker/tasks/sync_metrics_test.go` | `syncJobItemStatusCounts` returns correct per-status counts | Create |
| `.env.example` | Document the 4 new env vars | Modify |
| `docs/admin-guide.md` | Document `/metrics`, pprof, and the new env vars | Modify |
| `nix/package.nix` | Refresh `vendorHash` after the go.mod change | Modify |

---

## Task 1: Config fields for OTel + pprof

**Files:**
- Modify: `internal/config/config.go` (after line 126, end of struct, before the closing `}` at line 127)
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:

```go
func TestLoad_ObservabilityDefaults(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.OTELServiceName != "nexorious" {
		t.Errorf("OTELServiceName = %q; want %q", cfg.OTELServiceName, "nexorious")
	}
	if !cfg.OTELMetricsEnabled {
		t.Errorf("OTELMetricsEnabled = false; want true (default on)")
	}
	if cfg.PprofEnabled {
		t.Errorf("PprofEnabled = true; want false (default off)")
	}
	if cfg.PprofAddr != "127.0.0.1:6060" {
		t.Errorf("PprofAddr = %q; want %q", cfg.PprofAddr, "127.0.0.1:6060")
	}
}

func TestLoad_ObservabilityOverrides(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Setenv("OTEL_SERVICE_NAME", "nexorious-staging")
	t.Setenv("OTEL_METRICS_ENABLED", "false")
	t.Setenv("PPROF_ENABLED", "true")
	t.Setenv("PPROF_ADDR", "127.0.0.1:7070")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.OTELServiceName != "nexorious-staging" {
		t.Errorf("OTELServiceName = %q; want %q", cfg.OTELServiceName, "nexorious-staging")
	}
	if cfg.OTELMetricsEnabled {
		t.Errorf("OTELMetricsEnabled = true; want false")
	}
	if !cfg.PprofEnabled {
		t.Errorf("PprofEnabled = false; want true")
	}
	if cfg.PprofAddr != "127.0.0.1:7070" {
		t.Errorf("PprofAddr = %q; want %q", cfg.PprofAddr, "127.0.0.1:7070")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_Observability -v`
Expected: FAIL — `cfg.OTELServiceName undefined` (build error).

- [ ] **Step 3: Add the config fields**

In `internal/config/config.go`, insert this block immediately before the closing brace of the `Config` struct (after the `LegendaryWorkDir` field at line 126):

```go
	// -------------------------------------------------------------------------
	// Observability
	// -------------------------------------------------------------------------

	// OTELServiceName is the service.name resource attribute attached to all
	// metrics (and, once #911 lands, traces).
	OTELServiceName string `env:"OTEL_SERVICE_NAME" envDefault:"nexorious"`

	// OTELMetricsEnabled controls whether the OTel meter provider and the
	// always-on Prometheus /metrics endpoint are wired up. Default true.
	OTELMetricsEnabled bool `env:"OTEL_METRICS_ENABLED" envDefault:"true"`

	// PprofEnabled gates a loopback-only net/http/pprof listener for on-demand
	// heap/goroutine/CPU profiling. Default off; never exposed publicly.
	PprofEnabled bool `env:"PPROF_ENABLED" envDefault:"false"`

	// PprofAddr is the bind address for the pprof listener when enabled. Keep it
	// on loopback (127.0.0.1); reach it via `kubectl port-forward` + go tool pprof.
	PprofAddr string `env:"PPROF_ADDR" envDefault:"127.0.0.1:6060"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoad_Observability -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add observability config (OTel service/metrics, pprof)"
```

---

## Task 2: Add dependencies

This task has no unit test of its own; its gate is a clean `go build ./...`. It bumps OTel core 1.41.0 → 1.43.0 (required by otelriver) and adds four direct modules.

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `nix/package.nix` (`vendorHash`)

- [ ] **Step 1: Add the modules**

```bash
go get github.com/uptrace/bun/extra/bunotel@v1.2.18
go get go.opentelemetry.io/otel/exporters/prometheus@v0.66.0
go get go.opentelemetry.io/otel/sdk/metric@v1.43.0
go get github.com/riverqueue/rivercontrib/otelriver@v0.10.0
go mod tidy
```

- [ ] **Step 2: Verify the versions resolved and the tree is consistent**

Run:
```bash
go build ./... 2>&1 | head -20
grep -E "otel|bunotel|otelriver|client_golang" go.mod
```
Expected: build succeeds (no code uses the new deps yet, so this just proves the graph resolves). `go.opentelemetry.io/otel` should now read `v1.43.0`; `bunotel`, `exporters/prometheus`, `sdk/metric`, `rivercontrib/otelriver`, and `prometheus/client_golang` should be present (some still `// indirect` until later tasks import them — that is fine).

> If `go get otelriver@v0.10.0` reports it needs a newer River than v0.39.0, STOP and report — do not bump River in this issue. (Verified during planning: otelriver v0.10.0 requires only river v0.29.0, so this should not happen.)

- [ ] **Step 3: Refresh the nix vendorHash**

Per CLAUDE.md → "Nix Flake Maintenance":
```bash
# In nix/package.nix set: vendorHash = pkgs.lib.fakeHash;
nix build .#nexorious 2>&1 | grep "got:"
# paste the got: hash into nix/package.nix → vendorHash
```
If `nix` is unavailable in this shell, leave a clear `TODO(vendorHash)` note in the PR description and flag it — CI's nix workflow auto-patches `vendorHash` on the PR, but verify it lands.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum nix/package.nix
git commit -m "build: add OTel metrics + bunotel + otelriver dependencies"
```

---

## Task 3: `internal/observability` package

The package owns the meter provider, the Prometheus registry/handler, the two business-metric instruments, the record helpers, and `Shutdown`. When metrics are disabled it installs a no-op meter provider and leaves the handler nil.

**Files:**
- Create: `internal/observability/observability.go`
- Test: `internal/observability/observability_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/observability/observability_test.go`:

```go
package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/observability"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape status = %d; want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestInit_EnabledExposesBusinessMetrics(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: true,
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	h := observability.MetricsHandler()
	if h == nil {
		t.Fatal("MetricsHandler() = nil; want non-nil when metrics enabled")
	}

	observability.RecordSyncOutcome(context.Background(), "steam", "completed")
	observability.RecordSyncItems(context.Background(), "steam", "completed", 3)
	observability.RecordSyncItems(context.Background(), "steam", "failed", 1)

	body := scrape(t, h)

	// Exporter appends _total to the monotonic counters.
	if !strings.Contains(body, "nexorious_sync_total") {
		t.Errorf("scrape missing nexorious_sync_total:\n%s", body)
	}
	if !strings.Contains(body, `source="steam"`) || !strings.Contains(body, `status="completed"`) {
		t.Errorf("scrape missing expected sync_total labels:\n%s", body)
	}
	if !strings.Contains(body, "nexorious_sync_items_total") {
		t.Errorf("scrape missing nexorious_sync_items_total:\n%s", body)
	}
	if !strings.Contains(body, `outcome="failed"`) {
		t.Errorf("scrape missing items outcome label:\n%s", body)
	}
	// Cardinality guard: never label by user_id.
	if strings.Contains(body, "user_id=") {
		t.Errorf("scrape leaked user_id label:\n%s", body)
	}
}

func TestInit_DisabledIsNoop(t *testing.T) {
	prov, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: false,
	}, "1.2.3-test")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	t.Cleanup(func() { _ = prov.Shutdown(context.Background()) })

	if observability.MetricsHandler() != nil {
		t.Error("MetricsHandler() != nil; want nil when metrics disabled")
	}
	// Recording must not panic with the no-op provider.
	observability.RecordSyncOutcome(context.Background(), "steam", "completed")
	observability.RecordSyncItems(context.Background(), "steam", "completed", 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/observability/ -v`
Expected: FAIL — package `observability` does not exist.

- [ ] **Step 3: Write the implementation**

Create `internal/observability/observability.go`:

```go
// Package observability bootstraps the OpenTelemetry metrics pipeline: a meter
// provider backed by a dedicated Prometheus registry, the /metrics HTTP handler,
// and the nexorious sync-outcome business metrics. Tracing is intentionally a
// no-op here; issue #911 adds the OTLP trace exporter on top of this scaffolding.
package observability

import (
	"context"
	"fmt"
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
	syncTotal, _ = m.Int64Counter(
		"nexorious_sync",
		otelmetric.WithDescription("Count of completed sync jobs by source and final status."),
	)
	syncItemsTotal, _ = m.Int64Counter(
		"nexorious_sync_items",
		otelmetric.WithDescription("Count of synced library items by source and per-item outcome."),
	)
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
// "completed", "failed", "skipped". A zero count is a no-op.
func RecordSyncItems(ctx context.Context, source, outcome string, n int64) {
	if syncItemsTotal == nil || n == 0 {
		return
	}
	syncItemsTotal.Add(ctx, n, otelmetric.WithAttributes(
		attribute.String("source", source),
		attribute.String("outcome", outcome),
	))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/observability/ -v`
Expected: PASS (`TestInit_EnabledExposesBusinessMetrics`, `TestInit_DisabledIsNoop`).

> Note: package-global instruments mean these two tests must not run truly in parallel against conflicting providers. They don't call `t.Parallel()`, so the default sequential execution within the package is correct — leave it that way.

- [ ] **Step 5: Commit**

```bash
git add internal/observability/
git commit -m "feat: observability package — OTel meter provider + Prometheus /metrics"
```

---

## Task 4: Wire observability into serve.go (provider, bunotel hook, otelriver middleware, shutdown)

No new unit test — the gate is `go build ./...` plus the existing serve/router tests staying green; the behavioral coverage comes from Task 5 (`/metrics` route) and Task 7 (business metrics).

**Files:**
- Modify: `cmd/nexorious/serve.go`

- [ ] **Step 1: Add imports**

In `cmd/nexorious/serve.go`, add to the import block:
- Standard: nothing new yet (pprof handled in Task 6).
- Third-party:
  ```go
  	"github.com/uptrace/bun/extra/bunotel"
  	"github.com/riverqueue/rivercontrib/otelriver"
  	tracenoop "go.opentelemetry.io/otel/trace/noop"
  ```
- Internal:
  ```go
  	"github.com/drzero42/nexorious/internal/observability"
  ```

- [ ] **Step 2: Initialize the provider right after slog setup**

In `runServe`, immediately after the `slog.SetDefault(...)` block (currently ending at line 63) and **before** the encrypter/DB setup, insert:

```go
	// -------------------------------------------------------------------------
	// Observability (metrics) — must precede DB + River wiring so the bunotel
	// hook and otelriver middleware can bind to the meter provider.
	// -------------------------------------------------------------------------
	obs, err := observability.Init(cfg, version)
	if err != nil {
		slog.Error("observability init failed", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		os.Exit(1)
	}
	noopTracer := tracenoop.NewTracerProvider()
```

- [ ] **Step 3: Attach the bunotel query hook to the DB**

Immediately after `db := openBunDB(resolvedDatabaseURL)` (currently line 77), before `db.SetMaxOpenConns(25)`, add:

```go
	db.AddQueryHook(bunotel.NewQueryHook(
		bunotel.WithMeterProvider(obs.MeterProvider),
		bunotel.WithTracerProvider(noopTracer),
	))
```

- [ ] **Step 4: Add the otelriver middleware to the primary River client**

In the primary `river.NewClient` config (currently `Middleware:` at line 244), prepend the otelriver middleware:

```go
		Middleware: []rivertype.Middleware{
			otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
				MeterProvider:  obs.MeterProvider,
				TracerProvider: noopTracer,
				DurationUnit:   "s",
			}),
			logging.NewWorkerMiddleware(quietJobKinds...),
		},
```

- [ ] **Step 5: Add the otelriver middleware to the restore-rebuild River client**

In the `RebuildServices` closure's `river.NewClient` config (currently `Middleware:` at line 343), apply the identical change. `obs` and `noopTracer` are in lexical scope (the closure is defined inside `runServe`):

```go
			Middleware: []rivertype.Middleware{
				otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
					MeterProvider:  obs.MeterProvider,
					TracerProvider: noopTracer,
					DurationUnit:   "s",
				}),
				logging.NewWorkerMiddleware(quietJobKinds...),
			},
```

- [ ] **Step 6: Register provider shutdown in the graceful-shutdown sequence**

After the `riverClient.Stop` block (currently lines 422–425), before `slog.Info("shutdown complete")`, add (use a fresh timeout context — `shutdownCtx` is already cancelled by the signal):

```go
	// Flush and stop the meter provider last so in-flight job/DB metrics from the
	// River drain above are exported.
	obsShutdownCtx, obsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer obsCancel()
	if err := obs.Shutdown(obsShutdownCtx); err != nil {
		slog.Warn("observability shutdown", logging.KeyErr, err)
	}
```

- [ ] **Step 7: Build and run the existing cmd tests**

Run:
```bash
go build ./...
go test ./cmd/nexorious/ -v 2>&1 | tail -30
```
Expected: build succeeds; existing `cmd/nexorious` tests still pass.

- [ ] **Step 8: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat: wire OTel meter provider, bunotel hook, otelriver middleware"
```

---

## Task 5: Mount `/metrics` and bypass the app-state gates

**Files:**
- Modify: `internal/api/router.go`
- Test: `internal/api/router_test.go`

- [ ] **Step 1: Write the failing test**

First inspect an existing test in `internal/api/router_test.go` to copy the harness's router-construction pattern (how it builds the `*echo.Echo` and a `*config.Config`). Then add a test that initializes observability with metrics enabled, builds the router, and asserts `/metrics` returns Prometheus text. Add to `internal/api/router_test.go`:

```go
func TestMetricsEndpoint(t *testing.T) {
	if _, err := observability.Init(&config.Config{
		OTELServiceName:    "nexorious-test",
		OTELMetricsEnabled: true,
	}, "test"); err != nil {
		t.Fatalf("observability.Init: %v", err)
	}

	e := newTestRouter(t) // use the existing helper this file uses to build api.New

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d; want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("/metrics Content-Type = %q; want text/plain*", ct)
	}
}
```

> Replace `newTestRouter(t)` with whatever constructor `router_test.go` already uses (e.g. a `setupRouter`/`testServer` helper, or a direct `api.New(...)` call with the package's test fixtures). Add `httptest`, `strings`, `net/http`, the `config` and `observability` imports as needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestMetricsEndpoint -v`
Expected: FAIL — `/metrics` returns 404 (route not yet mounted) or a redirect from a gate.

- [ ] **Step 3: Mount the route**

In `internal/api/router.go`, add the `observability` import:
```go
	"github.com/drzero42/nexorious/internal/observability"
```

In `registerRoutes`, immediately after the `/health` handler block (after line 191), add:

```go
	// Prometheus metrics — unauthenticated and always-on when enabled. Mounted
	// only if the meter provider produced a handler (OTEL_METRICS_ENABLED=true).
	// Carries no secrets; labels are bounded (source/status/outcome, never user_id).
	if h := observability.MetricsHandler(); h != nil {
		e.GET("/metrics", echo.WrapHandler(h))
	}
```

- [ ] **Step 4: Add `/metrics` to the three gate allowlists**

So `/metrics` is scrapable even before the app reaches `Ready`:

- Gate 1 (line 95): change
  ```go
  				if path == "/db-error" || path == "/health" || path == "/static/app.css" {
  ```
  to
  ```go
  				if path == "/db-error" || path == "/health" || path == "/metrics" || path == "/static/app.css" {
  ```

- Gate 2 (line 115): change
  ```go
  					path == "/health" || path == "/static/app.css" ||
  ```
  to
  ```go
  					path == "/health" || path == "/metrics" || path == "/static/app.css" ||
  ```

- Gate 3 (line 135): change
  ```go
  					path == "/health" || strings.HasPrefix(path, "/api/migrate") ||
  ```
  to
  ```go
  					path == "/health" || path == "/metrics" || strings.HasPrefix(path, "/api/migrate") ||
  ```

Also update the three gate comments (lines 89, 108, 129) to mention `/metrics` alongside `/health`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestMetricsEndpoint -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go
git commit -m "feat: serve Prometheus /metrics, bypassing app-state gates"
```

---

## Task 6: Localhost-only pprof listener

**Files:**
- Create: `cmd/nexorious/pprof.go`
- Modify: `cmd/nexorious/serve.go` (start the listener when enabled)

- [ ] **Step 1: Create the pprof helper**

Create `cmd/nexorious/pprof.go`:

```go
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

// startPprofServer launches a loopback-only net/http/pprof listener in a
// background goroutine. It is gated by PPROF_ENABLED (default off) and must bind
// a loopback address (PPROF_ADDR, default 127.0.0.1:6060) — profiling is never
// exposed publicly; reach it via `kubectl port-forward` + `go tool pprof`.
//
// A dedicated ServeMux (not DefaultServeMux) keeps the pprof handlers off any
// other server. ReadHeaderTimeout is set to satisfy gosec G112; no write timeout
// is set because /debug/pprof/profile streams for its full duration.
func startPprofServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("pprof listener starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof listener failed", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		}
	}()
}
```

> If gosec flags `srv.ListenAndServe()` (G114) despite the `ReadHeaderTimeout`, add `//nolint:gosec // loopback-only pprof listener, ReadHeaderTimeout set, default off` on that line. If it flags the bind address (G102), the loopback default makes it a false positive — annotate similarly.

- [ ] **Step 2: Start it from runServe when enabled**

In `cmd/nexorious/serve.go`, in the HTTP-server section (just before `addr := fmt.Sprintf(...)` at line 409), add:

```go
	if cfg.PprofEnabled {
		startPprofServer(cfg.PprofAddr)
	}
```

- [ ] **Step 3: Build and verify lint**

Run:
```bash
go build ./...
golangci-lint run ./cmd/nexorious/ 2>&1 | tail -20
```
Expected: build succeeds; no lint errors (add the documented `//nolint:gosec` only if gosec actually fires).

- [ ] **Step 4: Manual smoke test (optional but recommended)**

```bash
make build
PPROF_ENABLED=true DB_ENCRYPTION_KEY=test-db-encryption-key-32-bytes!! \
  DATABASE_URL="$DATABASE_URL" ./nexorious serve &
SERVER_PID=$!
sleep 3
curl -s http://127.0.0.1:6060/debug/pprof/ | head -5     # expect the pprof index HTML
go tool pprof -top -seconds 1 http://127.0.0.1:6060/debug/pprof/heap 2>&1 | head -10
# confirm it is NOT reachable off-loopback if you have another interface IP
kill $SERVER_PID
```
Expected: index reachable on 127.0.0.1; heap profile obtainable.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/pprof.go cmd/nexorious/serve.go
git commit -m "feat: add loopback-only pprof listener gated by PPROF_ENABLED"
```

---

## Task 7: Sync-outcome business metrics at finalization

Record `nexorious_sync_total{source,status}` (job-level) and `nexorious_sync_items_total{source,outcome}` (item-level) inside `SyncCheckJobCompletion`, using one grouped count query. The query helper is the unit-tested piece; the thin recording calls are covered by the observability package test from Task 3.

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Test: `internal/worker/tasks/sync_metrics_test.go`

- [ ] **Step 1: Write the failing test for the count helper**

First check `internal/worker/tasks/` for the shared test DB pattern (the package-level `testDB` var and `truncateAllTables(t)` helper described in CLAUDE.md → Testing → Performance). Use them — do **not** spin a new container. Create `internal/worker/tasks/sync_metrics_test.go`:

```go
package tasks

import (
	"context"
	"testing"
)

func TestSyncJobItemStatusCounts(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Seed a job and items spanning the settled statuses.
	const jobID = "job-metrics-1"
	const userID = "user-metrics-1"
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'high')`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	seed := func(key, status string) {
		if _, err := testDB.NewRaw(
			`INSERT INTO job_items (job_id, user_id, item_key, source_title, status)
			 VALUES (?, ?, ?, ?, ?)`,
			jobID, userID, key, key, status,
		).Exec(ctx); err != nil {
			t.Fatalf("seed item %s: %v", key, err)
		}
	}
	seed("a", "completed")
	seed("b", "completed")
	seed("c", "failed")
	seed("d", "skipped")
	seed("e", "skipped")
	seed("f", "skipped")

	completed, failed, skipped, ok := syncJobItemStatusCounts(ctx, testDB, jobID)
	if !ok {
		t.Fatal("syncJobItemStatusCounts ok = false; want true")
	}
	if completed != 2 || failed != 1 || skipped != 3 {
		t.Errorf("counts = (completed=%d failed=%d skipped=%d); want (2 1 3)", completed, failed, skipped)
	}
}
```

> Adjust the seed column lists if the `jobs` / `job_items` schema in this package's test fixtures requires more NOT NULL columns — consult CLAUDE.md → Known Gotchas for the authoritative column lists (`jobs` has no `updated_at`; `job_items` uses `error_message`, not `error`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestSyncJobItemStatusCounts -v`
Expected: FAIL — `syncJobItemStatusCounts` undefined.

- [ ] **Step 3: Add the count helper**

In `internal/worker/tasks/sync.go`, add (near `syncJobUserAndStorefront`, after line 1027):

```go
// syncJobItemStatusCounts returns the per-status item counts for a finalized job.
// Used for the nexorious_sync_items_total business metric. Returns ok=false on a
// query error so callers can skip metrics (best-effort, never blocks the job).
func syncJobItemStatusCounts(ctx context.Context, db *bun.DB, jobID string) (completed, failed, skipped int64, ok bool) {
	var rows []struct {
		Status string `bun:"status"`
		N      int64  `bun:"n"`
	}
	if err := db.NewRaw(
		`SELECT status, COUNT(*) AS n FROM job_items WHERE job_id = ? GROUP BY status`,
		jobID,
	).Scan(ctx, &rows); err != nil {
		slog.WarnContext(ctx, "sync: count job item statuses for metrics", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return 0, 0, 0, false
	}
	for _, r := range rows {
		switch r.Status {
		case "completed":
			completed = r.N
		case "failed":
			failed = r.N
		case "skipped":
			skipped = r.N
		}
	}
	return completed, failed, skipped, true
}
```

- [ ] **Step 4: Record the metrics at finalization**

Add the `observability` import to `internal/worker/tasks/sync.go`:
```go
	"github.com/drzero42/nexorious/internal/observability"
```

In `SyncCheckJobCompletion`, immediately after the `emitSyncDiff(...)` call (currently line 1002) and before the store-link enrichment block (line 1004), insert:

```go
	// Sync-outcome metrics (best-effort, bounded cardinality: source + status/outcome).
	status := "completed"
	if failedCount > 0 {
		status = "completed_with_errors"
	}
	observability.RecordSyncOutcome(ctx, storefront, status)
	if completed, failed, skipped, ok := syncJobItemStatusCounts(ctx, db, jobID); ok {
		observability.RecordSyncItems(ctx, storefront, "completed", completed)
		observability.RecordSyncItems(ctx, storefront, "failed", failed)
		observability.RecordSyncItems(ctx, storefront, "skipped", skipped)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go test ./internal/worker/tasks/ -run TestSyncJobItemStatusCounts -v
go build ./...
```
Expected: PASS; build clean. (`RecordSync*` are safe no-ops in this test since `observability.Init` was not called in this package's test setup — `syncTotal`/`syncItemsTotal` are nil and the helpers guard on nil.)

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_metrics_test.go
git commit -m "feat: record sync-outcome metrics at job finalization"
```

---

## Task 8: Documentation

**Files:**
- Modify: `.env.example`
- Modify: `docs/admin-guide.md`

- [ ] **Step 1: Document the env vars in `.env.example`**

Append (match the file's existing comment style):

```bash
# --- Observability ---
# Prometheus metrics are exposed at /metrics (unauthenticated, bounded labels).
OTEL_SERVICE_NAME=nexorious
OTEL_METRICS_ENABLED=true
# Loopback-only pprof for on-demand profiling (off by default). Reach it via
# `kubectl port-forward` + `go tool pprof`; never expose it publicly.
PPROF_ENABLED=false
PPROF_ADDR=127.0.0.1:6060
```

- [ ] **Step 2: Document operations in `docs/admin-guide.md`**

Add an "Observability" section. Inspect the current heading structure first and match it. Cover:
- `GET /metrics` — Prometheus exposition; always-on unless `OTEL_METRICS_ENABLED=false`; lists the notable metric families (`nexorious_sync_total`, `nexorious_sync_items_total`, `river.work_count`, `river.work_duration`, Bun DB query metrics); note it is unauthenticated and safe to scrape (no secrets, bounded cardinality).
- pprof — how to enable (`PPROF_ENABLED=true`), why it stays on loopback, and the `kubectl port-forward 6060:6060` + `go tool pprof http://localhost:6060/debug/pprof/heap` workflow; tie it to the 256 MB cap / OOM-diagnosis motivation.
- The four env vars and their defaults.

> Do not add an in-app `/help` link to any non-embedded doc. `admin-guide.md` **is** embedded (per CLAUDE.md), so an admin-guide section is fine. Do not cross-link to non-embedded reference docs.

- [ ] **Step 3: Verify the docs build/render path is unaffected**

Run: `go build ./...`
Expected: clean (admin-guide is embedded via `docs/embed.go` — confirm the file still compiles into the binary).

- [ ] **Step 4: Commit**

```bash
git add .env.example docs/admin-guide.md
git commit -m "docs: document /metrics, pprof, and observability env vars"
```

---

## Task 9: Full verification

- [ ] **Step 1: Full build + suites**

Run:
```bash
go build ./...
go test -timeout 600s ./... 2>&1 | tail -40
golangci-lint run 2>&1 | tail -40
```
Expected: build clean; all tests pass; zero lint findings.

- [ ] **Step 2: End-to-end `/metrics` smoke test**

```bash
make build
DB_ENCRYPTION_KEY=test-db-encryption-key-32-bytes!! DATABASE_URL="$DATABASE_URL" ./nexorious serve &
SERVER_PID=$!
sleep 4
curl -s http://localhost:8000/metrics | grep -E "nexorious_sync|river_work|go_goroutines" | head
kill $SERVER_PID
```
Expected: Prometheus text including `river_work_*` and Go runtime metrics. (`nexorious_sync_*` series appear only after at least one sync completes — note this; they are registered lazily on first record.)

- [ ] **Step 3: Confirm graceful shutdown is clean**

With the server running, send SIGTERM and confirm the logs show `River client stop` (or success), then `observability shutdown` only if it errors, then `shutdown complete`, with no panic and no goroutine-leak warning.

- [ ] **Step 4: Verify acceptance criteria against the issue**

- [ ] `GET /metrics` returns Prometheus metrics incl. DB (bunotel), `river.work_*` (otelriver), and `nexorious_sync_*` (after a sync).
- [ ] pprof reachable only when `PPROF_ENABLED=true` + on loopback; heap/goroutine profiles obtainable.
- [ ] Meter provider shuts down cleanly on SIGTERM/SIGINT; no leaked goroutines.
- [ ] No secrets in labels; cardinality bounded (`source`/`status`/`outcome`, never `user_id`) — asserted by the observability test.

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin feat/910-otel-metrics-pprof
gh pr create --title "feat: OpenTelemetry metrics foundation + pprof endpoint" --body "$(cat <<'EOF'
Implements #910 (part of the #909 observability epic).

## What
- New `internal/observability` package: OTel meter provider + dedicated Prometheus registry + `/metrics` handler.
- Always-on `GET /metrics` (zero collector dependency), bypassing the app-state gates like `/health`.
- Drop-in metric sources: `bunotel` query hook (DB) and `otelriver` middleware on both River clients (`river.work_*`).
- Business metrics `nexorious_sync_total{source,status}` and `nexorious_sync_items_total{source,outcome}`, recorded once at the `SyncCheckJobCompletion` finalization chokepoint.
- Loopback-only `net/http/pprof` listener gated by `PPROF_ENABLED` (default off) — diagnoses the 256 MB-cap OOM motivation in #909.
- Config: `OTEL_SERVICE_NAME`, `OTEL_METRICS_ENABLED`, `PPROF_ENABLED`, `PPROF_ADDR`.
- Tracing stays a no-op (explicit no-op tracer providers); #911 swaps in the OTLP exporter.

## Dependency note
Adds `otelriver` v0.10.0, which requires OTel core v1.43.0 — this bumps `go.opentelemetry.io/otel` 1.41.0 → 1.43.0 (River v0.39.0 already satisfies otelriver's minimum). `nix/package.nix` vendorHash refreshed.

Closes #910
EOF
)"
```

---

## Self-review

**Spec coverage (issue #910 tasks):**
- Provider lifecycle (init after config, shutdown after River stop, no-op tracer stub) → Tasks 3, 4.
- `GET /metrics` via OTel Prometheus exporter near `/health` → Task 5.
- bunotel query hook → Task 4. otelriver middleware (both clients) → Task 4.
- Business metrics at sync completion → Task 7.
- pprof, admin/localhost-gated, default off via `PPROF_ENABLED` → Task 6 (localhost variant, per user decision).
- Config `OTEL_SERVICE_NAME` / `OTEL_METRICS_ENABLED` / `PPROF_ENABLED`; service version from build var → Tasks 1, 3.
- Acceptance criteria (metrics present, pprof gated, clean shutdown, bounded cardinality) → Task 9.
- Risk: pin otelriver / confirm builds against river v0.39.0 → Task 2 (pinned v0.10.0; compatibility verified in planning).

**Type/name consistency:** `observability.Init(cfg, version) (*Providers, error)`, `Providers.MeterProvider` / `Providers.Shutdown(ctx)`, `MetricsHandler() http.Handler`, `RecordSyncOutcome(ctx, source, status)`, `RecordSyncItems(ctx, source, outcome, n)`, `syncJobItemStatusCounts(ctx, db, jobID) (completed, failed, skipped int64, ok bool)`, `startPprofServer(addr string)` — used identically across Tasks 3–7. Instrument names `nexorious_sync` / `nexorious_sync_items` → scraped as `*_total` consistently.

**Open items to confirm during execution (flagged inline, not placeholders):**
- The exact router test helper name in Task 5 (copy the one `router_test.go` already uses).
- Whether gosec fires on the pprof `ListenAndServe` despite `ReadHeaderTimeout` (Task 6 documents the precise nolint to add only if it does).
- The seed column lists in Task 7 if the test fixtures require additional NOT NULL columns.
