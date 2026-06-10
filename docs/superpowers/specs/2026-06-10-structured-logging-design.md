# Sharpen structured logging — design

**Issue:** #907 — leveling, correlation IDs, consistent keys, error taxonomy
**Status:** approved (brainstorming)
**Date:** 2026-06-10
**Coordination:** OTel `trace_id`/`span_id` injection is **out of scope** here; tracked in
#911 (epic #909). This issue builds the `ctx → logger` seam that #911 extends.

## Motivation

Logs are structured JSON (`slog` → stdout, configured in `cmd/nexorious/serve.go`) but
inconsistent: **260 `Error` / 57 `Warn` / 44 `Info` / 34 `Debug`** across ~400 sites, no
correlation id threading lines within a request or job, ad-hoc attribute keys, and no error
categorization. This is a focused logging-quality pass — **no metrics/tracing/OTel** — that
also lays a clean seam a future OTel handler hooks into.

## The slog constraint that shapes the seam

`slog` only passes a `context.Context` to a `Handler` when the **`*Context` method variant**
is used:

```go
slog.Info(msg, args...)              // Handler.Handle receives context.Background() — ctx is LOST
slog.InfoContext(ctx, msg, args...)  // Handler.Handle receives YOUR ctx
```

Therefore correlation injected by a *handler* requires call sites to pass `ctx`. We choose this
(over a `.With()`-bound logger) because it is the most OTel-ready: `span_id` changes within a
request (parent → child spans), and a handler reads ctx **per log record**, so the *current*
active span is always reflected. #911 then becomes ~3 lines added to `Handle()` with **zero
call-site changes**.

## Architecture

### 1. New `internal/logging` package — the seam

A cohesive home for all logging plumbing:

- **`ContextHandler`** — wraps the JSON handler installed in `serve.go`. Its
  `Handle(ctx, r)` pulls correlation values out of `ctx` *per record* and `AddAttrs` them:
  `request_id`, `job_id`, and `user_id` (when present). This is the OTel seam — #911 adds
  `trace_id`/`span_id` extraction here, no call-site changes.

  ```go
  func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
      if v, ok := ctx.Value(requestIDKey).(string); ok && v != "" {
          r.AddAttrs(slog.String(KeyRequestID, v))
      }
      if v, ok := ctx.Value(jobIDKey).(string); ok && v != "" {
          r.AddAttrs(slog.String(KeyJobID, v))
      }
      if v, ok := ctx.Value(userIDKey).(string); ok && v != "" {
          r.AddAttrs(slog.String(KeyUserID, v))
      }
      return h.inner.Handle(ctx, r)
  }
  // #911 will add only: span := trace.SpanFromContext(ctx); if valid → trace_id/span_id
  ```

- **ctx setters / keys** — an unexported key type plus `WithRequestID(ctx, id)`,
  `WithJobID(ctx, id)`, `WithUserID(ctx, id)`.

- **`keys.go`** — exported attribute-key constants, the single source of truth for keys.
  Call sites use these, not string literals:
  `KeyRequestID`, `KeyJobID`, `KeyJobType`, `KeyUserID`, `KeySource`, `KeyOperation`,
  `KeyExternalGameID`, `KeyDurationMS`, `KeyHost`, `KeyEndpoint`, `KeyStatus`, `KeyRoute`,
  `KeyLatency`, `KeyCategory`, `KeyOutcome`, `KeyErr`.

- **`category.go`** — fixed `Category` enum and a helper returning the `slog.Attr`:
  `external_api | db | validation | auth | config`. Lives here (not `internal/enum`) because it
  is logging-specific. Set at error boundaries to make failures aggregatable/greppable.

- **`roundtripper.go`** — the logging `http.RoundTripper` (see §3).

- **`redact.go`** — redaction helper for known-sensitive attribute keys, with tests.

**Call-site contract** everywhere a unit of work exists: `slog.InfoContext(ctx, msg, attrs…)`
(and `ErrorContext`/`WarnContext`/`DebugContext`). Scopes wanting bound static attrs do:
`l := slog.Default().With(logging.KeyJobType, t); l.InfoContext(ctx, …)`.

Startup code with no unit of work (`serve.go` boot sequence, adapter factory) legitimately stays
on bare `slog.*` and carries no correlation — that is correct, not a gap.

### 2. Correlation wiring at the two boundaries

- **HTTP** — a small middleware in `internal/api` generates a request id (honoring an inbound
  `X-Request-Id` if present, else generating one) and stores it via
  `logging.WithRequestID` onto `c.Request().Context()`, so it propagates to every downstream
  call that threads ctx. `AuthMiddleware` additionally stashes `user_id` into the Go context
  (it already sets the echo-context copy via `c.Set("user_id", …)`) so in-request log lines
  carry `user_id`.

- **Jobs** — one global River `WorkerMiddleware` (River v0.39 supports
  `rivertype.WorkerMiddleware`) binds `logging.WithJobID(ctx, job.ID)` and emits exactly one
  outcome line wrapping `doInner`: `job_type`, `outcome` (`completed`/`failed`),
  `duration_ms`. This single middleware satisfies both job-correlation and job-duration
  criteria. Registered in the River `Config.Middleware` where the client is constructed.

### 3. External API duration — shared RoundTripper

Each `internal/services/*/client.go` `http.Client` gets its `Transport` wrapped with the shared
logging `RoundTripper`. One log line per call: `host`, `endpoint` (URL **path, query
stripped**), `status`, `duration_ms`.

Stripping the query string both bounds cardinality (these APIs put ids/keys in the query, not the
path) and prevents leaking secrets — notably Steam's API key, which rides in the query
(`...?key=%s&steamids=...`, `internal/services/steam/client.go`). The query-strip is unit-tested
as a secret guard.

### 4. Echo RequestLogger (`internal/api/router.go`)

Extend `RequestLoggerWithConfig` to consistently emit `request_id`, `route` (the matched route
**pattern**, e.g. `/api/games/:id`, not the raw URI — low cardinality), `status`, `latency`, and
`user_id` (when authenticated). It must never log `Authorization`/`Cookie` headers or request/
response bodies.

### 5. Re-leveling audit — conservative, case-by-case

Walk every `Error` site. Demote only where the code **provably handles the condition and
continues** (retry, skip, fallback) — judged individually against the policy table below. `error`
stays reserved for actionable/unhandled failures. Set `category` at error boundaries. No hard
numeric target; the reduction is driven by the code, expected to land `error` well below half of
260.

| Level   | Reserved for                                                              | Typical demotions                                            |
|---------|---------------------------------------------------------------------------|-------------------------------------------------------------|
| `error` | Actionable failure — operator must act; data loss, unhandled, job dead     | (keep only these)                                           |
| `warn`  | Recoverable/retryable — handled, will retry, degraded but continuing       | decrypt failures that skip a source, transient probe fails  |
| `info`  | Lifecycle — start/stop, connected, job started/finished                    | —                                                           |
| `debug` | Per-item detail, verbose tracing                                           | per-item progress, 429 backoff notes                        |

### 6. Secrets/PII — focused unit guards

1. RoundTripper strips query/secrets from logged URLs (test).
2. RequestLogger never logs `Authorization`/`Cookie` headers or bodies (test).
3. Redaction helper for known-sensitive keys (test).

Plus a documented manual grep-audit confirming sync cookie-paste material, API keys,
`DB_ENCRYPTION_KEY`, and session cookies are never logged. Field cardinality is bounded; full
request/response bodies are never logged.

### 7. Docs

New `docs/logging-conventions.md` documenting: the leveling policy, the attribute-key set, the
`category` taxonomy, and the `slog.InfoContext(ctx, …)` call-site contract. Cross-linked from
`CLAUDE.md`. This is a **reference doc** (like `sync.md`/`maintenance.md`) — kept in the repo for
GitHub viewing, **not** embedded/served in-app.

## Data flow

```
HTTP request
  → RequestID middleware  (ctx ← request_id)
  → AuthMiddleware        (ctx ← user_id)
  → handler: slog.InfoContext(ctx, …)
        → ContextHandler.Handle reads ctx → adds request_id/user_id → JSON handler → stdout
  → service client.Do(ctx)
        → logging RoundTripper → one line: host/endpoint/status/duration_ms (+ ctx correlation)
  → RequestLogger (one line: request_id/route/status/latency/user_id)

River job
  → WorkerMiddleware (ctx ← job_id; times doInner)
  → Work(ctx): slog.InfoContext(ctx, …)  → ContextHandler adds job_id
  → WorkerMiddleware emits one outcome line: job_type/outcome/duration_ms
```

## Error handling

- `ContextHandler` adds attrs only when ctx values are present and non-empty; absence is silent
  and correct (e.g. startup logs).
- RoundTripper logs the outcome of every call including transport errors (status 0 + err);
  it never alters the response/error returned to the caller.
- WorkerMiddleware records `outcome=failed` and `duration_ms` even when `doInner` returns an
  error, then returns that error unchanged so River retry semantics are unaffected.

## Testing

- `ContextHandler`: attrs injected from ctx; absent ctx values omitted.
- RoundTripper: query stripped (secret guard); host/endpoint/status/duration_ms emitted; error
  path logged without mutating the returned error.
- WorkerMiddleware: outcome/duration on success and failure; error propagated unchanged.
- RequestLogger: emits request_id/route/status/latency/user_id; excludes auth headers/bodies.
- Redaction helper: sensitive keys redacted.
- Re-leveling is verified by review against the policy table (not unit-tested per site).

## Delivery

Single PR closing the full acceptance checklist. Commit type mixed (`feat:` for net-new
actionable fields — duration_ms, request_id, category; `refactor:` for re-leveling); the PR
title is a `feat:` because it adds user-actionable structured fields.

## Acceptance criteria (from #907)

> Note: the issue's first criterion names a `logger(ctx)` accessor returning a pre-bound
> logger. We deliberately reframe it to a **ctx-reading handler** + the `slog.InfoContext(ctx, …)`
> contract — same intent (correlation on every in-request/in-job line) via a mechanism that is
> strictly more OTel-ready (live `span_id` for #911 with zero call-site changes). A thin
> `logging` helper still exists for scopes that bind extra static attrs.

- [ ] `ctx`-based logging seam; `request_id`/`job_id` on every in-request/in-job log line
- [ ] `duration_ms` on every external API call and River job outcome
- [ ] Shared attribute-key constants; keys used consistently
- [ ] Leveling audit complete; `Error` reserved for actionable failures (large reduction from 260)
- [ ] `category` field on error logs, fixed enum
- [ ] Secrets/PII audit done, with tests asserting no secret leakage
- [ ] Echo RequestLogger emits `request_id`/`route`/`status`/`latency`/`user_id`
- [ ] Logging-conventions doc written, cross-linked from `CLAUDE.md`

## Anti-goals

- No double-logging the same event at every layer — log once at the boundary.
- No tracing/metrics/OTel (that is #911).
- No logging of bodies or secrets.
