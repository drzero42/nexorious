# OpenTelemetry Tracing (Opt-in OTLP) for the Sync Pipeline — Design

**Issue:** #911 (part of the #909 observability epic). Builds on #910 (metrics foundation).

## Goal

Activate trace export so a sync becomes a readable waterfall — `river.work` →
outbound external-API calls → DB queries — and bind `trace_id`/`span_id` onto
log lines so logs and traces cross-reference. Tracing is **off unless an OTLP
endpoint is configured**; when off, behavior and overhead are identical to
today.

## Corrected premise (deviation from the issue text)

Issue #911 prescribes extending "#907's `logger(ctx)` accessor" and argues the
slog handler never sees the ctx. That premise is outdated: #907 actually landed
`logging.ContextHandler` (`internal/logging/handler.go`), which reads
correlation IDs from ctx **per record**, and the project convention is
`slog.*Context(ctx, …)` — which does pass ctx to `Handler.Handle`. The handler's
own comment designates it as the #911 seam. This design therefore does
trace-log correlation via a chained handler, not an accessor.

## Decisions (settled during brainstorming, 2026-06-11)

1. **Trace-log correlation lives in `internal/observability`** as a wrapper
   slog handler (`NewTraceContextHandler`). `internal/logging` stays OTel-free
   and untouched except for two plain string key constants (`KeyTraceID`,
   `KeySpanID`). Rationale: `observability` already imports `logging` (so the
   reverse import would cycle), and chaining the handler only when tracing is
   enabled gives literally zero overhead when off.
2. **Outbound HTTP instrumentation via an `observability.HTTPTransport()`
   accessor** returning the full transport stack. No service constructor
   signatures change. *Amended after spec approval (user-approved
   2026-06-11):* the otelhttp transport wraps whenever metrics **or** tracing
   is enabled — not only when tracing is on. The otelhttp `Transport` emits
   client metrics (`http.client.request.duration` etc., per host)
   independently of spans, and #913's external-API alert rules need those
   metrics in metrics-only deployments. Spans still appear only when the OTLP
   endpoint is set (noop tracer provider otherwise).
3. **OTLP over HTTP/protobuf only** (`otlptracehttp`, default port 4318).
   Matches #912's dev-stack assumption; avoids the `google.golang.org/grpc`
   dependency tree. gRPC can be added later if a real need appears.
4. **SDK-default sampling (sample everything) and no artificial limits.** The
   user explicitly does not want hard-coded conservative bounds; the 256MB
   prod cap is a local choice, not a product constraint. Operators tune via the
   standard `OTEL_TRACES_SAMPLER(_ARG)` / `OTEL_BSP_*` env vars, which the SDK
   honors natively.
5. **Helm support = commented example + docs**, not a values block.
   `OTEL_EXPORTER_OTLP_ENDPOINT` has no sensible default and the standard env
   vars are the interface; operators set it via the normal bjw-s values merge.

## 1. Gating & architecture

- New optional config field `OTELExporterOTLPEndpoint`
  (`env:"OTEL_EXPORTER_OTLP_ENDPOINT"`, no default). It is used **only as the
  on/off gate** — the exporter itself reads the standard OTel env vars
  natively (endpoint, path, headers, TLS, timeouts, compression), so we pass
  no explicit exporter options.
- **Endpoint set:** `observability.Init` builds an `otlptracehttp` exporter +
  `sdktrace.TracerProvider` with a batch span processor and the same resource
  attributes as the meter provider (`service.name`, `service.version`). It
  registers the provider globally (`otel.SetTracerProvider`) and sets the W3C
  propagators (`propagation.TraceContext{}` + `propagation.Baggage{}`).
- **Endpoint unset:** exactly today's behavior — `tracenoop` provider, no
  exporter, no extra handler in the slog chain, plain logging round-tripper.
- Sampling/batching: SDK defaults (`parentbased_always_on`, default BSP
  bounds). No custom sampler, no hard-coded limits.

## 2. Components

### `internal/observability`

- `Providers.TracerProvider trace.TracerProvider` — real SDK provider when
  gated on, `tracenoop.NewTracerProvider()` otherwise. `Providers.Shutdown`
  composes shutdown: tracer provider first (flushes pending spans), then meter
  provider.
- `HTTPTransport() http.RoundTripper` — package accessor (same pattern as
  `MetricsHandler()`):
  - metrics or tracing enabled → `otelhttp.NewTransport(logging.NewRoundTripper(nil))`
    with the package's meter + tracer providers (each real or noop per its own
    gate). otelhttp is **outermost**, so the per-call log line emitted by the
    logging round-tripper inherits the HTTP client span's `trace_id`/`span_id`,
    and `traceparent` is injected into outbound requests. With tracing off the
    spans are noops but the client metrics still flow (feeds #913).
  - both disabled, or `Init` not yet called → `logging.NewRoundTripper(nil)`
    (today's behavior; the pre-`Init` fallback keeps service-package unit
    tests and CLI paths working unchanged).
- `NewTraceContextHandler(inner slog.Handler) slog.Handler` — adds
  `trace_id`/`span_id` from `trace.SpanContextFromContext(ctx)` when the span
  context is valid; passes through untouched otherwise. Mirrors
  `ContextHandler`'s `WithAttrs`/`WithGroup` plumbing.

### `internal/logging`

- Two new key constants only: `KeyTraceID = "trace_id"`,
  `KeySpanID = "span_id"`. No OTel imports; package stays OTel-free.

### `cmd/nexorious/serve.go`

- Replace the three `tracenoop` wirings with `obs.TracerProvider`: the
  `bunotel` query hook and the `otelriver` middleware on **both** River
  clients (primary + restore-rebuild closure).
- Chain `observability.NewTraceContextHandler` into the slog handler stack
  only when tracing is enabled (the gate is known from `cfg` at slog-setup
  time; ordering with `observability.Init` is an implementation detail for the
  plan).
- `otel.SetErrorHandler` → `slog.Warn` so runtime export failures surface in
  our logs instead of stderr.

### Service HTTP clients

One-line swap to `Transport: observability.HTTPTransport()` at each
construction site:

| Package | Sites |
|---|---|
| `internal/services/steam` | `client.go:30` |
| `internal/services/gog` | `client.go:27`, `client.go:38` |
| `internal/services/igdb` | `igdb.go:54`, `auth.go:38` |
| `internal/services/humble` | `client.go:38` |
| `internal/services/playstationstore` | `client.go:37` (see note) |
| `internal/services/updatecheck` | `client.go:35` |
| `internal/services/storelink` | `resolver.go:42`, `resolver.go:90` |

- **Note:** `playstationstore`'s `profileHTTPClient` is a package-level var
  initialized at package-init time — **before** `observability.Init` runs. It
  becomes lazily initialized (e.g. `sync.OnceValue`) so it picks up the real
  transport.
- `epicgamesstore` is CLI-driven (Legendary) — no HTTP client, excluded.

## 3. Resulting trace shape

`river.work:<kind>` (otelriver root span) → `HTTP GET <external host>` spans
(otelhttp) → DB query spans (bunotel), all in one trace per job.

**Known, accepted noise:** DB queries and outbound calls made from HTTP
handlers become standalone single-span root traces, because there is
deliberately no HTTP-server span middleware (`otelecho` is echo/v4-only; see
#909). Documented, filterable in the backend; not worth a custom sampler.

## 4. Error handling, lifecycle, secrets

- Exporter construction failure → `Init` returns an error; serve fails fast
  (same as metrics init).
- Runtime export failures → logged at warn via the otel error handler.
- Shutdown: tracer provider shuts down inside the existing 5s observability
  shutdown window in `serve.go`, after the River drain — pending spans from
  in-flight jobs flush before exit (acceptance: no dropped spans on SIGTERM).
- **Secrets:** `bunotel` stays at its default `WithFormattedQueries(false)`.
  Verified against bunotel v1.2.18 source: the `db.statement` attribute then
  carries the query **template** (placeholders, no argument values) — the
  fallback is `event.QueryTemplate`, including for `internal/auth`'s raw
  `QueryRowContext` calls. otelhttp records method/URL, never bodies or
  `Authorization` headers.

## 5. Testing

- `observability_test.go`: with the endpoint set, `Init` yields a non-noop
  `TracerProvider`; without it, the noop provider. `HTTPTransport()` returns
  the otelhttp transport when metrics or tracing is enabled and the plain
  logging round-tripper when both are disabled.
- Trace-handler tests: a record logged with an active span ctx carries
  `trace_id`/`span_id` matching the span; without an active span, neither key
  appears.
- Transport test with the SDK's in-memory `tracetest` exporter: a request
  through `HTTPTransport()` produces a client span (verifies stack order and
  provider wiring).
- Config test: `OTELExporterOTLPEndpoint` defaults to empty.

## 6. Documentation

- `.env.example`: `OTEL_EXPORTER_OTLP_ENDPOINT` (commented, with the 4318
  example) + a pointer to the standard `OTEL_TRACES_SAMPLER(_ARG)` /
  `OTEL_BSP_*` vars.
- `docs/admin-guide.md`: extend the Observability section — how to enable
  tracing, what a sync trace shows, the trace-log correlation fields, the
  root-span noise caveat.
- `DEV.md`: replace the "tracing is a no-op" note; how to enable locally
  (full local viewing stack arrives with #912).

## 7. Helm chart support

- Commented-out example entries in the `values.yaml` env block beside the
  #910 observability vars: `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`.
- Chart README + admin-guide note showing how to point at an in-cluster
  collector via the values merge.
- No `values.schema.json` changes (no new `nexorious.*` fields), no
  `test.yaml` lint flags.

## Out of scope

- HTTP-server span middleware (epic decision; `otelecho` is echo/v4-only).
- gRPC OTLP transport (add on demand).
- Custom samplers / span filtering.
- Local trace-viewing stack and dashboards (#912); metrics alert rules (#913).
- Context propagation from HTTP requests into enqueued River jobs (job traces
  are rooted at `river.work`; linking enqueue-site request traces to job
  traces is a possible future enhancement, not needed for the sync waterfall).
