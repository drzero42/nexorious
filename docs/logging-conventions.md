# Logging conventions

Nexorious emits structured JSON logs via `log/slog` (configured in
`cmd/nexorious/serve.go`). This document is the contract for how log lines are
produced so they stay correlatable, consistently keyed, correctly leveled, and
free of secrets. The seam lives in `internal/logging`.

## The call-site contract

**When a unit of work is in scope, use the `*Context` slog variants and pass the
context:**

```go
slog.InfoContext(ctx, "sync complete", logging.KeySource, src)
slog.ErrorContext(ctx, "load job_item", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
```

`internal/logging.ContextHandler` wraps the JSON handler and, **per log record**,
pulls correlation ids out of `ctx` and stamps them on the line. This only works
when `ctx` reaches the handler — and slog passes `ctx` to the handler **only** for
the `*Context` methods. A bare `slog.Info(...)` hands the handler
`context.Background()` and loses correlation.

Bare `slog.*` is correct **only** where there is genuinely no unit of work — the
startup/boot sequence in `serve.go`, config parsing, and the adapter factory. Those
lines carry no correlation id by design, and that is fine. (~56 such sites remain;
~340 in-request/in-job sites use the `*Context` form.)

To bind extra static attributes for a scope, use `.With` and still log with a
`*Context` method:

```go
l := slog.Default().With(logging.KeyJobType, jobType)
l.InfoContext(ctx, "starting")
```

## Correlation ids — injected automatically, never by hand

The `ContextHandler` injects these from `ctx`; **do not** add them as explicit
attributes (doing so duplicates the JSON key):

| Key             | Meaning                                            | Seeded by |
|-----------------|----------------------------------------------------|-----------|
| `request_id`    | One HTTP request                                   | `RequestIDMiddleware` (honors inbound `X-Request-Id`, else generates) |
| `user_id`       | Authenticated user                                 | `auth.AuthMiddleware` (on authenticated routes) |
| `job_id`        | The application job (`jobs.id`, a UUID) — the user-facing job | the worker that owns it, via `logging.WithJobID(ctx, id)` |
| `river_job_id`  | River's internal job id (a Postgres sequence int)  | `logging.WorkerMiddleware` (every River job) |

### `job_id` vs `river_job_id` — they are different values

River assigns every job an internal **int64** id (`river_job.id`). The application
has its **own** `jobs` table keyed by a **UUID** (`jobs.id`), which is what the API,
the `job_items` table, and users see. These are not the same number.

- `river_job_id` is seeded automatically by `WorkerMiddleware` onto **every** in-job
  line — use it to correlate one execution.
- `job_id` (the UUID) is the business id. Workers that know it seed it into `ctx`
  near their entrypoint (e.g. `ctx = logging.WithJobID(ctx, p.JobID)`), so every line
  below carries it. Item-scoped workers seed it after loading the job_item
  (`item.JobID`). Id-bearing helpers (`countJobItems`, `finalizeJobCompleted`,
  `SyncCheckJobCompletion`) seed their `jobID` parameter rather than logging it
  explicitly — `context.WithValue` is idempotent per key, so re-seeding the same
  value never duplicates the attribute.

If you log an id for a **different** entity than the request's user/job (e.g. an
admin creating another user), use a distinct key like `target_user_id` — never reuse
`user_id`/`job_id`, or the line will carry two conflicting values for one key.

## Leveling policy

Re-level by what the code does with the condition, not by how bad the message reads.

| Level   | Reserved for                                                                   |
|---------|--------------------------------------------------------------------------------|
| `error` | Actionable failure — an operator must act: a DB failure that aborts the request (HTTP 500) or the job, data loss, a stuck/dead job, fatal startup failure. |
| `warn`  | Recoverable / handled — the code logs and continues: retries, best-effort side-effects (a non-critical `changes`/notification insert), per-item skips, transient external-API failures, expired/invalid credentials. |
| `info`  | Lifecycle — start/stop, connected, job started/finished, a completed action.   |
| `debug` | Per-item detail and verbose tracing.                                           |

The governing rule: **if the surrounding code handles the condition and continues
(loop `continue`, fall-through, `return nil`, a best-effort write), it is not
`error`.** When unsure between `warn` and `error`, keep `error`; between `warn` and
`debug`, pick `warn`.

## Attribute keys

`internal/logging/keys.go` is the single source of truth. Use the `Key*` constants
instead of string literals so keys never drift:

`KeyRequestID`, `KeyJobID`, `KeyRiverJobID`, `KeyJobType`, `KeyUserID`, `KeySource`,
`KeyOperation`, `KeyExternalGameID`, `KeyDurationMS`, `KeyHost`, `KeyEndpoint`,
`KeyStatus`, `KeyRoute`, `KeyOutcome`, `KeyCategory`, `KeyErr`.

Attributes that have no constant (e.g. `"item_id"`, `"appid"`, `"backup_id"`,
`"path"`, `"storefront"`) stay as string literals. Add a constant only when a key is
used widely enough to be worth pinning.

## Error taxonomy — the `category` field

Set a fixed, low-cardinality `category` on `error`/`warn` lines at failure
boundaries, so failures are aggregatable and greppable:

```go
slog.ErrorContext(ctx, "upsert external_game failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
```

| Category               | Use for |
|------------------------|---------|
| `logging.CategoryDB`           | database operations (queries, inserts, transactions, restore) |
| `logging.CategoryExternalAPI`  | outbound HTTP / third-party services (Steam, IGDB, PSN, GOG, Epic, shoutrrr) |
| `logging.CategoryValidation`   | bad input / unparseable data |
| `logging.CategoryAuth`         | credential / token / permission failures |
| `logging.CategoryConfig`       | misconfiguration (missing/invalid env, bad durations) |

Don't force a category where none fits (e.g. plain file/process failures in the
backup service, or `json.Marshal` of a fixed struct). Never put a category on
`info`/`debug` lines.

## External API calls and job outcomes — log once at the boundary

- **External HTTP calls** are logged automatically by the shared logging
  `http.RoundTripper` installed on every service client: one line per call with
  `host`, `endpoint` (URL path, **query stripped**), `status`, `duration_ms`. Do
  **not** add your own "calling X" / "got response" logs — that double-logs the
  boundary. (Limitation: the third-party `psnsdk` HTTP stack and the Epic/Legendary
  subprocess are not wrapped, so their calls aren't auto-logged.)
- **River job outcomes** are logged once by `WorkerMiddleware`: `job_type`,
  `outcome` (`completed`/`failed`), `duration_ms`. Don't re-log "job done" yourself.
  Routine high-frequency kinds (periodic maintenance — see
  `scheduler.MaintenanceJobKinds()` — plus `prune_events`) are passed to
  `NewWorkerMiddleware` as *quiet* kinds: their **successful** completion logs at
  `debug` so it doesn't bury user-initiated job outcomes. Failures always log at
  `warn`.

## The HTTP access log

One line per request, emitted by the Echo `RequestLogger` (`internal/api/router.go`),
keyed `method`, `uri`, `route` (the matched pattern, low cardinality), `status`,
`duration_ms`, plus `request_id`/`user_id` injected from ctx. Its **level is chosen by
status and route** (`requestLogLevel`, `internal/api/request_log_level.go`):

| Condition                                   | Level   |
|---------------------------------------------|---------|
| `status >= 500` (or handler error)          | `error` |
| `status >= 400`                             | `warn`  |
| successful asset/SPA/poll route (`isQuietRequestRoute`) | `debug` |
| everything else (meaningful API traffic)    | `info`  |

Quiet routes are static assets (`/static/*`, `/logos/*`, the SPA shell `/*`,
`/static/app.css`) and the timer-driven UI poll endpoints
(`/api/jobs/pending-review-count`, `/api/jobs/status/:job_type`). Add a route here when
it generates high-volume, no-signal access lines.

## Secrets and PII

Never log a credential or PII value. Specifically:

- No passwords, API keys, session tokens/cookies, `npsso`, OAuth/refresh tokens, the
  DB encryption key, or `DATABASE_URL` (it contains a password). Log the **error** and
  a non-sensitive identifier (`user_id`, `username`, `storefront`) — never the secret.
- Never log full request or response bodies, or `Authorization`/`Cookie` headers. The
  Echo `RequestLogger` emits only `request_id`, `route`, `status`, `duration_ms`,
  `method`, `uri`, and `user_id` (test-asserted to exclude headers/bodies). When a
  bounded snippet of an upstream error body is genuinely useful, cap it (e.g.
  `io.LimitReader(body, 256)`) — never log the whole body.
- The logging `RoundTripper` strips the URL query string before logging — this both
  bounds cardinality and prevents leaking secrets that ride in the query (notably the
  Steam API key, `...?key=...`). This is test-asserted.
- For the rare case where a value derived from sensitive material must be logged, use
  `logging.Redact(v)`, which returns a non-reversible masked string.

A grep audit (issue #907) confirmed no slog call passes the encryption key,
`DATABASE_URL`, decrypted credentials, tokens, or session values. Re-run it when
touching auth/sync/backup code.
