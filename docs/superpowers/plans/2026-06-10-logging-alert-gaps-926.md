# Logging Gaps for Alert Rules (#926) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the structured-logging gaps that block #908's log-based alert rules: surface recovered panics, migration failures, and fatal startup failures as structured `slog` error lines; standardize the per-source `source` key onto the canonical storefront slug; and fix `category`/leveling on the alert-relevant boundaries.

**Architecture:** Extends the existing `internal/logging` seam established by #907/#924. Adds one new error `Category` (`panic`), a River `ErrorHandler` (in `internal/logging`) and a thin Echo recover-logging middleware (in `internal/api`) that both emit `category=panic` error lines, then makes targeted attribute/level edits at ~13 existing log sites. No schema, no migration, no behavior change other than logging.

**Tech Stack:** Go 1.26, `log/slog`, Echo v5 (`github.com/labstack/echo/v5`), River v0.39 (`github.com/riverqueue/river`), stdlib `testing`.

---

## Context an implementer must know

- **The logging contract** is `docs/logging-conventions.md`. Read it. Key rules:
  - In request/job code use `slog.*Context(ctx, …)` (not bare `slog.*`); correlation ids (`request_id`/`job_id`/`river_job_id`/`user_id`) are injected automatically from `ctx` by `logging.ContextHandler` — **never add them by hand**, except where the handler provably cannot reach the ctx (the River `HandlePanic` boundary — see Task 3).
  - Bare `slog.*` is correct **only** in no-unit-of-work sites: the boot sequence in `serve.go`, config parsing, and the **adapter factory** (the decrypt-failure `slog.Warn` lines in Task 6 are legitimately bare — do not "fix" them to `*Context`).
  - Use the `logging.Key*` constants (`internal/logging/keys.go`), never string literals, for canonical keys. `KeySource` is the canonical key for a storefront slug.
  - Set a `logging.Cat(...)` on error/warn lines at failure boundaries; never on info/debug.
- **Canonical storefront slugs** (the adapter-factory `case` labels and the job `source` value, `cmd/nexorious/serve.go:480`+): `steam`, `playstation-store`, `gog`, `epic-games-store`, `humble-bundle`. The string `"psn"` is **not** canonical — it stays only as a human label inside `msg`, never as a `source` value.
- **Decision (from issue triage):** the new panic category value is `"panic"` (`CategoryPanic`). The stuck-job reaping warn (Task 8) gets **no** category — none of the five fits a handled "jobs reaped" event, and the doc says don't force one; #908 filters it by `msg`/`level`.
- **Testing policy** (`CLAUDE.md`): no coverage gate, no tautological tests. TDD the substantive new units (Tasks 1–3). For the mechanical one-attribute / level edits (Tasks 4–8) a slog-capture test per site would be tautological — verify those by `go build`, `golangci-lint`, and a `grep` assertion instead. This is a deliberate, policy-aligned deviation from blanket per-step TDD.

## File map

| File | Change |
|---|---|
| `internal/logging/category.go` | **Modify** — add `CategoryPanic` |
| `internal/logging/category_test.go` | **Modify** — assert `CategoryPanic` value |
| `internal/logging/error_handler.go` | **Create** — `WorkerErrorHandler` (River `ErrorHandler`) |
| `internal/logging/error_handler_test.go` | **Create** — `HandlePanic` emits a `category=panic` line |
| `internal/api/recover.go` | **Create** — `PanicLogger()` Echo middleware |
| `internal/api/recover_test.go` | **Create** — panic in a handler emits a `category=panic` line |
| `internal/api/router.go` | **Modify** — register `PanicLogger()` before `middleware.Recover()` |
| `cmd/nexorious/serve.go` | **Modify** — wire `ErrorHandler` into both River configs; log both `river.NewClient` failures; add `KeySource` to 5 decrypt warns |
| `internal/migrate/migrator.go` | **Modify** — log 4 migration-failure branches; add `category` to "database unavailable" |
| `internal/worker/tasks/sync.go` | **Modify** — add `KeySource` to library-fetch error; `storefront`→`KeySource` |
| `internal/api/sync.go` | **Modify** — `storefront`→`KeySource` |
| `internal/worker/tasks/store_link_refresh.go` | **Modify** — `storefront`→`KeySource` at the resolve-failed warn |
| `internal/services/playstationstore/client.go` | **Modify** — add `KeySource="playstation-store"` to auth-failed warn |
| `internal/scheduler/scheduler.go` | **Modify** — add `category` to enqueue-dispatch error |
| `internal/api/jobs.go` | **Modify** — add `category` to `retryInsert: enqueue failed` |
| `internal/scheduler/stale_jobs.go` | **Modify** — raise reaping-occurred logs INFO→WARN |
| `docs/logging-conventions.md` | **Modify** — document `panic` category, panic boundaries, canonical source slugs |

---

## Task 1: Add the `panic` error category

**Files:**
- Modify: `internal/logging/category.go`
- Test: `internal/logging/category_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/logging/category_test.go`:

```go
func TestCat_EmitsPanicCategory(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "boom", Cat(CategoryPanic))

	m := decode(t, &buf)
	if m[KeyCategory] != "panic" {
		t.Errorf("category = %v, want panic", m[KeyCategory])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestCat_EmitsPanicCategory -v`
Expected: FAIL — `undefined: CategoryPanic`.

- [ ] **Step 3: Add the category constant**

In `internal/logging/category.go`, add `CategoryPanic` to the `const` block (after `CategoryConfig`):

```go
const (
	CategoryExternalAPI Category = "external_api"
	CategoryDB          Category = "db"
	CategoryValidation  Category = "validation"
	CategoryAuth        Category = "auth"
	CategoryConfig      Category = "config"
	CategoryPanic       Category = "panic"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestCat_EmitsPanicCategory -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/category.go internal/logging/category_test.go
git commit -m "feat: add panic error category to logging taxonomy"
```

---

## Task 2: River `ErrorHandler` that logs recovered worker panics

River invokes `ErrorHandler.HandlePanic` when a worker panics. The panic unwinds **past** `WorkerMiddleware` (River handles panics above all middleware), so the middleware's "job finished" line never fires for a panic — `HandlePanic` is the only place to surface it. `HandleError` (normal job errors) is left a no-op because `WorkerMiddleware` already logs failed outcomes at warn; logging there too would double-log. Per River's docs the ctx passed to `HandlePanic` does **not** carry middleware-set values, so `river_job_id` will not be auto-injected — set it explicitly here (the one sanctioned by-hand exception).

**Files:**
- Create: `internal/logging/error_handler.go`
- Test: `internal/logging/error_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/logging/error_handler_test.go`:

```go
package logging

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

func TestWorkerErrorHandler_HandlePanic_EmitsPanicLine(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &WorkerErrorHandler{}
	job := &rivertype.JobRow{ID: 4242, Kind: "sync_steam"}

	res := h.HandlePanic(context.Background(), job, "nil map write", "goroutine 1 [running]:")

	if res != nil {
		t.Errorf("HandlePanic result = %v, want nil (default retry behavior)", res)
	}
	m := decode(t, &buf)
	if m[KeyCategory] != "panic" {
		t.Errorf("category = %v, want panic", m[KeyCategory])
	}
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyRiverJobID] != "4242" {
		t.Errorf("river_job_id = %v, want 4242", m[KeyRiverJobID])
	}
	if m["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", m["level"])
	}
}

func TestWorkerErrorHandler_HandleError_IsNoOp(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &WorkerErrorHandler{}
	job := &rivertype.JobRow{ID: 1, Kind: "sync_steam"}

	res := h.HandleError(context.Background(), job, context.Canceled)

	if res != nil {
		t.Errorf("HandleError result = %v, want nil", res)
	}
	if buf.Len() != 0 {
		t.Errorf("HandleError should not log (WorkerMiddleware already logs failures); got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestWorkerErrorHandler -v`
Expected: FAIL — `undefined: WorkerErrorHandler`.

- [ ] **Step 3: Implement the handler**

Create `internal/logging/error_handler.go`:

```go
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkerErrorHandler implements river.ErrorHandler to surface recovered worker
// panics as structured slog error lines (category=panic). Normal job errors are
// already logged once at warn by WorkerMiddleware, so HandleError is a no-op to
// avoid double-logging; only panics — which unwind past the middleware and so are
// never logged by it — need surfacing here.
type WorkerErrorHandler struct{}

// HandleError is a no-op: WorkerMiddleware already emits the failed-outcome line.
func (h *WorkerErrorHandler) HandleError(_ context.Context, _ *rivertype.JobRow, _ error) *river.ErrorHandlerResult {
	return nil
}

// HandlePanic emits a category=panic error line for a recovered worker panic.
// River calls this above all middleware, so the ctx carries no middleware-set
// correlation ids — river_job_id is added explicitly here (it cannot be injected
// by ContextHandler at this boundary). Returning nil keeps River's default retry
// behavior.
func (h *WorkerErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	slog.ErrorContext(ctx, "worker: recovered panic",
		KeyJobType, job.Kind,
		KeyRiverJobID, strconv.FormatInt(job.ID, 10),
		KeyErr, fmt.Sprintf("%v", panicVal),
		Cat(CategoryPanic),
	)
	return nil
}

// compile-time assertion that the handler satisfies the interface.
var _ river.ErrorHandler = (*WorkerErrorHandler)(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestWorkerErrorHandler -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/logging/error_handler.go internal/logging/error_handler_test.go
git commit -m "feat: add River ErrorHandler that logs recovered worker panics"
```

---

## Task 3: Echo middleware that logs recovered HTTP panics

Echo v5's `middleware.Recover()` has **no** `LogErrorFunc`/`LogLevel` hook (that was v4). Instead it recovers the panic, wraps it as a `*middleware.PanicStackError`, and **returns it as an error** up the chain. So a thin middleware registered *outside* `Recover()` can detect that error type and emit a distinct `category=panic` line, reusing Echo's stack capture. By the time the error propagates back out, `RequestIDMiddleware` has already stamped `request_id` into the shared request ctx, so the panic line is correlated.

**Files:**
- Create: `internal/api/recover.go`
- Test: `internal/api/recover_test.go`
- Modify: `internal/api/router.go:58`

- [ ] **Step 1: Write the failing test**

Create `internal/api/recover_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

func TestPanicLogger_EmitsPanicLine(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(logging.NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e := echo.New()
	e.Use(PanicLogger())
	e.Use(middleware.Recover())
	e.Use(RequestIDMiddleware())
	e.GET("/boom", func(_ *echo.Context) error { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	// Find the panic line among the emitted JSON lines (the access-log line is
	// also emitted; only the panic line carries category=panic).
	var found map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) == nil && m[logging.KeyCategory] == "panic" {
			found = m
		}
	}
	if found == nil {
		t.Fatalf("no category=panic line emitted; got:\n%s", buf.String())
	}
	if found["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", found["level"])
	}
	if found[logging.KeyRequestID] == nil || found[logging.KeyRequestID] == "" {
		t.Errorf("panic line missing request_id correlation; got %v", found[logging.KeyRequestID])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestPanicLogger_EmitsPanicLine -v`
Expected: FAIL — `undefined: PanicLogger`.

- [ ] **Step 3: Implement the middleware**

Create `internal/api/recover.go`:

```go
package api

import (
	"errors"
	"log/slog"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

// PanicLogger emits a structured category=panic error line for panics recovered
// by the downstream middleware.Recover(). Echo v5's Recover converts a panic into
// a *middleware.PanicStackError and returns it up the chain (it exposes no logging
// hook of its own); registering PanicLogger immediately outside Recover lets us
// detect that error and log a distinct panic signal — separate from the HTTP 500
// access-log line — correlated by the request_id already in the request ctx.
func PanicLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			var pse *middleware.PanicStackError
			if errors.As(err, &pse) {
				slog.ErrorContext(c.Request().Context(), "http: recovered panic",
					logging.KeyErr, pse.Err.Error(),
					logging.Cat(logging.CategoryPanic),
				)
			}
			return err
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestPanicLogger_EmitsPanicLine -v`
Expected: PASS.

- [ ] **Step 5: Register the middleware**

In `internal/api/router.go`, register `PanicLogger()` immediately **before** `middleware.Recover()` (so it is the outer wrapper and sees the returned error). Change:

```go
	e.Use(middleware.Recover())
	e.Use(RequestIDMiddleware())
```

to:

```go
	e.Use(PanicLogger())
	e.Use(middleware.Recover())
	e.Use(RequestIDMiddleware())
```

- [ ] **Step 6: Run the api package build + the new test together**

Run: `go build ./internal/api/ && go test ./internal/api/ -run TestPanicLogger -v`
Expected: build OK, PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/recover.go internal/api/recover_test.go internal/api/router.go
git commit -m "feat: log recovered HTTP panics as structured error lines"
```

---

## Task 4: Wire the River `ErrorHandler` + log fatal `river.NewClient` failures

**Files:**
- Modify: `cmd/nexorious/serve.go` (two `river.NewClient` sites: ~`:245` primary, ~`:337` `RebuildServices`)

- [ ] **Step 1: Wire ErrorHandler into the primary River config**

In `cmd/nexorious/serve.go`, the primary `river.NewClient(...)` `&river.Config{...}` (around line 245), add the `ErrorHandler` field:

```go
	riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
		PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
		Middleware:   []rivertype.Middleware{logging.NewWorkerMiddleware(quietJobKinds...)},
		ErrorHandler: &logging.WorkerErrorHandler{},
	})
	if err != nil {
		slog.ErrorContext(ctx, "serve: river client init failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return fmt.Errorf("river.NewClient: %w", err)
	}
```

(`ctx` is the `context.Background()` declared near the top of the function; it is in scope here.)

- [ ] **Step 2: Wire ErrorHandler into the RebuildServices River config**

In the `RebuildServices` closure's `river.NewClient(...)` (around line 337), add the same field and failure log:

```go
			newClient, err := river.NewClient(riverpgxv5.New(newPgxPool), &river.Config{
				Workers:      newWorkers,
				Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
				PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
				Middleware:   []rivertype.Middleware{logging.NewWorkerMiddleware(quietJobKinds...)},
				ErrorHandler: &logging.WorkerErrorHandler{},
			})
			if err != nil {
				slog.ErrorContext(ctx, "serve: river client init failed (rebuild)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
				return fmt.Errorf("RebuildServices: river.NewClient: %w", err)
			}
```

(The outer `ctx` from the enclosing `serve` function is captured by this closure, so it is in scope. If a linter flags it as unused-shadow, fall back to `context.Background()`.)

- [ ] **Step 3: Build**

Run: `go build ./cmd/... ./internal/...`
Expected: OK (no unused-import error — `slog` and `logging` are already imported in `serve.go`).

- [ ] **Step 4: Verify wiring by grep**

Run: `grep -n "ErrorHandler: &logging.WorkerErrorHandler{}" cmd/nexorious/serve.go`
Expected: 2 matches.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat: wire River panic ErrorHandler and log fatal river client init failures"
```

---

## Task 5: Log migration-failure branches + add category to "database unavailable"

**Files:**
- Modify: `internal/migrate/migrator.go` (4 failure branches in `RunMigrations` ~`:220–250`; the "database unavailable" warn ~`:349`)

`slog` and `logging` are already imported in this file; `RunMigrations(ctx context.Context)` has `ctx`.

- [ ] **Step 1: Add a slog.ErrorContext to each of the 4 migration-failure branches**

In `RunMigrations`, each failure branch currently does `sendLog` + `TransitionToFailed(wrapped)` + `close(ch)` + `return wrapped`. Add a `slog.ErrorContext(ctx, ..., logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))` line immediately after the `wrapped :=` assignment in each branch. The four branches and their messages:

```go
	// lock branch
		wrapped := fmt.Errorf("migrate: acquire lock: %w", err)
		slog.ErrorContext(ctx, "migrate: acquire lock failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", wrapped))
```

```go
	// bun migrate branch
		wrapped := fmt.Errorf("migrate: bun: %w", err)
		slog.ErrorContext(ctx, "migrate: bun migration failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", err))
```

```go
	// River migrator setup branch
		wrapped := fmt.Errorf("migrate: River migrator: %w", err)
		slog.ErrorContext(ctx, "migrate: River migrator setup failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
		mg.sendLog(ch, fmt.Sprintf("River migration setup failed: %v\n", err))
```

```go
	// River migrate branch
		wrapped := fmt.Errorf("migrate: River: %w", err)
		slog.ErrorContext(ctx, "migrate: River migration failed", logging.KeyErr, wrapped, logging.Cat(logging.CategoryDB))
		mg.sendLog(ch, fmt.Sprintf("River migration failed: %v\n", err))
```

- [ ] **Step 2: Add category to the "database unavailable" warn**

At the existing line (~`:349`):

```go
					slog.WarnContext(ctx, "database unavailable", logging.KeyErr, err)
```

change to:

```go
					slog.WarnContext(ctx, "database unavailable", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
```

- [ ] **Step 3: Build + run any existing migrate tests**

Run: `go build ./internal/migrate/ && go test ./internal/migrate/ -count=1`
Expected: OK / PASS (no behavior change; existing tests unaffected).

- [ ] **Step 4: Verify by grep**

Run: `grep -c "logging.Cat(logging.CategoryDB)" internal/migrate/migrator.go`
Expected: at least 5 (4 new failure branches + the database-unavailable warn; plus any pre-existing).

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/migrator.go
git commit -m "fix: emit structured error logs on migration failure and tag db-unavailable warn"
```

---

## Task 6: Standardize the per-source `source` key onto the canonical slug

Five adapter-factory decrypt warns carry the source only in `msg`; four other boundaries use a custom `"storefront"` literal or omit `source`; the PSN client uses no `source`. Standardize all onto `logging.KeySource` with the canonical slug. The adapter-factory lines are legitimately bare `slog.Warn` (no unit of work) — keep them bare, just add the key.

**Files:**
- Modify: `cmd/nexorious/serve.go` (5 decrypt warns, ~`:486`–`:582`)
- Modify: `internal/worker/tasks/sync.go` (`:258` add source; `:371` rename key)
- Modify: `internal/api/sync.go` (`:339` rename key)
- Modify: `internal/worker/tasks/store_link_refresh.go` (`:316` rename key)
- Modify: `internal/services/playstationstore/client.go` (`:463` add source)

- [ ] **Step 1: Add `KeySource` to the 5 adapter-factory decrypt warns in `serve.go`**

For each `case`, append `logging.KeySource, "<slug>"` to the existing `slog.Warn(...)`:

```go
		// case "steam":
			slog.Warn("adapter factory: steam decrypt failed", logging.KeyUserID, cfg.UserID, logging.KeyErr, err, logging.KeySource, "steam", logging.Cat(logging.CategoryAuth))
		// case "playstation-store":
			slog.Warn("adapter factory: psn decrypt failed", logging.KeyUserID, cfg.UserID, logging.KeyErr, err, logging.KeySource, "playstation-store", logging.Cat(logging.CategoryAuth))
		// case "gog":
			slog.Warn("adapter factory: gog decrypt failed", logging.KeyUserID, cfg.UserID, logging.KeyErr, err, logging.KeySource, "gog", logging.Cat(logging.CategoryAuth))
		// case "epic-games-store":
			slog.Warn("adapter factory: epic decrypt failed", logging.KeyUserID, cfg.UserID, logging.KeyErr, err, logging.KeySource, "epic-games-store", logging.Cat(logging.CategoryAuth))
		// case "humble-bundle":
			slog.Warn("adapter factory: humble-bundle decrypt failed", logging.KeyUserID, cfg.UserID, logging.KeyErr, err, logging.KeySource, "humble-bundle", logging.Cat(logging.CategoryAuth))
```

- [ ] **Step 2: `sync.go` — add source to the library-fetch error; rename the credential-flag key**

At `internal/worker/tasks/sync.go:258`:

```go
		slog.ErrorContext(ctx, "dispatch_sync: library fetch failed", logging.KeySource, p.Storefront, logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
```

At `internal/worker/tasks/sync.go:371` (rename `"storefront"` → `logging.KeySource`):

```go
		slog.WarnContext(ctx, "dispatch_sync: flag credentials_error failed", logging.KeyErr, err, logging.KeyUserID, p.UserID, logging.KeySource, p.Storefront, logging.Cat(logging.CategoryDB))
```

- [ ] **Step 3: `api/sync.go:339` — rename `"storefront"` → `logging.KeySource`**

```go
		slog.WarnContext(ctx, "sync: credentials decrypt failed", logging.KeySource, sf, logging.KeyUserID, userID, logging.KeyErr, err, logging.Cat(logging.CategoryAuth))
```

- [ ] **Step 4: `store_link_refresh.go:316` — rename `"storefront"` → `logging.KeySource`**

The resolve-failed warn at line ~325 already uses `logging.KeySource`. The remaining custom-key site is the per-item warn around line 316:

```go
				"storefront", meta.Storefront, "item_id", jobItemID)
```

Change the `"storefront"` literal to `logging.KeySource`:

```go
				logging.KeySource, meta.Storefront, "item_id", jobItemID)
```

(Verify the full `slog.*Context(...)` call this line belongs to and edit only the key literal; leave `"item_id"` as-is — it has no constant.)

- [ ] **Step 5: `playstationstore/client.go:463` — add canonical source to the auth-failed warn**

```go
			slog.WarnContext(ctx, "psn: auth failed",
				logging.KeySource, "playstation-store",
				logging.KeyErr, err, logging.Cat(logging.CategoryAuth))
```

(The `"psn:"` prefix in `msg` is a human label and stays; the canonical `source` **value** is `playstation-store`.)

- [ ] **Step 6: Build the touched packages**

Run: `go build ./cmd/... ./internal/worker/... ./internal/api/... ./internal/services/playstationstore/...`
Expected: OK.

- [ ] **Step 7: Verify no stray `"storefront"` slog key remains at the four renamed sites**

Run: `grep -rn '"storefront"' internal/worker/tasks/sync.go internal/api/sync.go internal/worker/tasks/store_link_refresh.go`
Expected: no matches inside an `slog.*` attribute position. (Non-slog uses — SQL, struct tags, JSON keys, the `meta` map literal — are unrelated and stay. Inspect each hit to confirm it is not a slog key.)

- [ ] **Step 8: Commit**

```bash
git add cmd/nexorious/serve.go internal/worker/tasks/sync.go internal/api/sync.go internal/worker/tasks/store_link_refresh.go internal/services/playstationstore/client.go
git commit -m "fix: standardize per-source logging onto canonical KeySource slug"
```

---

## Task 7: Add `category` to the two remaining alert-relevant error lines

**Files:**
- Modify: `internal/scheduler/scheduler.go:219`
- Modify: `internal/api/jobs.go:744`

Both files already import `logging` and use `logging.Key*`.

- [ ] **Step 1: `scheduler.go:219` — add db category to the enqueue-dispatch error**

```go
			slog.ErrorContext(ctx, "CheckPendingSyncs: enqueue dispatch failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.KeyUserID, cfg.UserID, logging.Cat(logging.CategoryDB))
```

- [ ] **Step 2: `jobs.go:744` — add db category to the enqueue-failed error**

```go
		slog.ErrorContext(ctx, "retryInsert: enqueue failed",
			"job_type", jobType, logging.KeySource, source, "job_item_id", jobItemID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
```

- [ ] **Step 3: Build**

Run: `go build ./internal/scheduler/ ./internal/api/`
Expected: OK.

- [ ] **Step 4: Commit**

```bash
git add internal/scheduler/scheduler.go internal/api/jobs.go
git commit -m "fix: tag enqueue-failure error logs with db category for alerting"
```

---

## Task 8: Raise stuck-job reaping logs from INFO to WARN

The leveling policy puts a stuck/dead job under `error`, but the cleanup **handles** it (marks the jobs failed and continues), so the reaping-occurred event is `warn`, not `error`. Only the two `count > 0` reaping lines change; the query-failure `slog.ErrorContext` lines above them are already correct and stay. No category is added — none of the five fits a handled "jobs reaped" event, and forcing one violates the doc; #908 filters this by `msg`/`level`.

**Files:**
- Modify: `internal/scheduler/stale_jobs.go:65` and `:91`

- [ ] **Step 1: Change the two reaping-occurred logs to WarnContext**

At `:65`:

```go
	if rows > 0 {
		slog.WarnContext(ctx, "cleanup_stale_jobs: marked stale jobs failed", "count", rows)
	}
```

At `:91`:

```go
	if syncRows > 0 {
		slog.WarnContext(ctx, "cleanup_stale_jobs: marked stale sync jobs failed", "count", syncRows)
	}
```

- [ ] **Step 2: Build + run scheduler tests**

Run: `go build ./internal/scheduler/ && go test ./internal/scheduler/ -count=1`
Expected: OK / PASS.

- [ ] **Step 3: Verify by grep**

Run: `grep -n "marked stale" internal/scheduler/stale_jobs.go`
Expected: both lines now `slog.WarnContext`.

- [ ] **Step 4: Commit**

```bash
git add internal/scheduler/stale_jobs.go
git commit -m "fix: log stuck-job reaping at warn so alerts can fire on it"
```

---

## Task 9: Update `docs/logging-conventions.md`

**Files:**
- Modify: `docs/logging-conventions.md`

- [ ] **Step 1: Add the `panic` category to the taxonomy table**

In the "Error taxonomy — the `category` field" section table, add a row:

```
| `logging.CategoryPanic`        | recovered panics — the Echo recover boundary and the River worker ErrorHandler |
```

- [ ] **Step 2: Document the panic-logging boundaries**

Append a short paragraph after the taxonomy table (or under "External API calls and job outcomes"):

```markdown
## Recovered panics

A recovered panic is surfaced as a distinct `level=error`, `category=panic` line
(separate from the resulting HTTP 500 access-log line / River failed-outcome line):

- **HTTP:** `api.PanicLogger()` (registered just outside `middleware.Recover()`)
  detects the `*middleware.PanicStackError` that Echo's recover returns and logs
  `"http: recovered panic"`, correlated by `request_id`.
- **Workers:** `logging.WorkerErrorHandler` (wired as River's `ErrorHandler`) logs
  `"worker: recovered panic"` from `HandlePanic` with `job_type` and `river_job_id`.
  River calls `HandlePanic` above all middleware, so `river_job_id` is set
  explicitly there — the one place a correlation id is added by hand, because the
  `ContextHandler` cannot reach that boundary's ctx.
```

- [ ] **Step 3: Document the canonical source slugs**

In the "Attribute keys" section (near the mention of `KeySource`/`"storefront"`), add:

```markdown
`KeySource` carries the **canonical storefront slug** — the same value used as the
job `source` and the adapter-factory `case` label: `steam`, `playstation-store`,
`gog`, `epic-games-store`, `humble-bundle`. Use `KeySource` (never a custom
`"storefront"` key) and never the legacy `"psn"` value — `"psn"` may appear only as
a human label inside a `msg`, never as a `source` value.
```

- [ ] **Step 4: Commit**

```bash
git add docs/logging-conventions.md
git commit -m "docs: document panic category, panic boundaries, and canonical source slugs"
```

---

## Final verification (before opening the PR)

- [ ] **Full Go test suite**

Run: `go test -timeout 600s ./...`
Expected: PASS.

- [ ] **Lint**

Run: `golangci-lint run`
Expected: zero findings. (Watch for: `errcheck` on the new code — none expected, no `_ =` introduced; `gosec` — none; an unused `ctx` in the RebuildServices site — if flagged, switch that one call to `context.Background()`.)

- [ ] **Acceptance-criteria self-check (map each box in #926 to a task)**

  - Recovered panics (Echo + River) → Tasks 2, 3, 4.
  - Migration failures `category=db` → Task 5.
  - Fatal startup (`river.NewClient`) failures → Task 4.
  - Per-source `KeySource` canonical slug; `"storefront"` removed; `psn` resolved → Task 6.
  - `category` on `migrator.go:349`, `scheduler.go:219`, `jobs.go:744` → Tasks 5, 7.
  - Stuck-job reaping at `warn` when count > 0 → Task 8.
  - `docs/logging-conventions.md` updated → Task 9.

- [ ] **Open the PR**

```bash
git push -u origin fix/logging-alert-gaps-926
gh pr create --title "fix: close logging gaps blocking log-based alert rules" --body "$(cat <<'EOF'
Closes #926

Follow-up to #907/#924; unblocks #908. Surfaces recovered panics (Echo + River),
migration failures, and fatal River-client startup failures as structured slog
error lines; standardizes per-source logging onto the canonical `KeySource` slug
(resolving the `psn` vs `playstation-store` drift); and fixes `category`/leveling
on the alert-relevant boundaries. Adds a `panic` error category. Logging-only — no
schema, migration, or behavior change. `docs/logging-conventions.md` updated.
EOF
)"
```

---

## Notes / out-of-scope findings surfaced while planning

- `internal/api/jobs.go:738` (`"retryInsert: unsupported job_type"`, ERROR) also lacks a `category` and would fit `CategoryValidation`. #926 lists only `:744`, so it is **not** in scope here — flag to the user as a possible one-line follow-up rather than fixing silently.
