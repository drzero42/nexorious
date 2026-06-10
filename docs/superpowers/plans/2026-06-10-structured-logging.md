# Sharpen Structured Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `slog` output correlatable, consistently keyed, correctly leveled, and secret-safe — and lay a `ctx`-reading handler seam that a future OTel issue (#911) extends with `trace_id`/`span_id`.

**Architecture:** A new `internal/logging` package holds a `ContextHandler` (injects correlation from `ctx` per log record), the canonical attribute-key constants, a fixed `category` enum, a logging `http.RoundTripper`, and a redaction helper. Correlation is seeded into `ctx` at two boundaries — an Echo middleware for HTTP `request_id` and a River `WorkerMiddleware` for `job_id` — and call sites switch to `slog.*Context(ctx, …)`. A conservative, case-by-case audit re-levels handled-and-continued conditions out of `Error`.

**Tech Stack:** Go 1.26, `log/slog`, Echo v5, River v0.39 (`rivertype.WorkerMiddleware`), Bun, testcontainers (existing test harness).

**Spec:** `docs/superpowers/specs/2026-06-10-structured-logging-design.md`

**Branch:** `feat/sharpen-structured-logging` (already created; spec already committed).

---

## File Structure

**Create:**
- `internal/logging/keys.go` — exported attribute-key constants (single source of truth).
- `internal/logging/context.go` — ctx key type + `WithRequestID/WithJobID/WithUserID` setters and matching getters.
- `internal/logging/handler.go` — `ContextHandler` (wraps an inner `slog.Handler`).
- `internal/logging/handler_test.go`
- `internal/logging/category.go` — `Category` enum + `Cat()` attr helper.
- `internal/logging/category_test.go`
- `internal/logging/redact.go` — redaction helper.
- `internal/logging/redact_test.go`
- `internal/logging/roundtripper.go` — logging `http.RoundTripper`.
- `internal/logging/roundtripper_test.go`
- `internal/logging/middleware.go` — River `WorkerMiddleware` (job_id + outcome/duration).
- `internal/logging/middleware_test.go`
- `internal/api/requestid.go` — Echo middleware seeding `request_id` into the request `ctx`.
- `internal/api/requestid_test.go`
- `docs/logging-conventions.md` — reference doc.

**Modify:**
- `cmd/nexorious/serve.go:59-61` — wrap the JSON handler with `ContextHandler`; register the River `WorkerMiddleware` at both `river.NewClient` sites (`:231`, `:327`).
- `internal/api/router.go:58-72` — add the request_id middleware; extend `RequestLoggerWithConfig`.
- `internal/auth/session.go:153` — also seed `user_id` into the request `ctx`.
- Each `internal/services/*/client.go` (+ `igdb/auth.go`, `igdb/igdb.go`) — install the RoundTripper on each client's `http.Client`; replace shared `http.DefaultClient` usage with an owned client.
- Re-leveled call sites across `internal/api`, `internal/worker`, `internal/services`, `internal/scheduler`, `internal/backup`, `internal/notify`, `internal/migrate`, `cmd/nexorious`.
- `CLAUDE.md` — cross-link the new doc; document the `slog.*Context(ctx,…)` contract briefly.

---

## Task 1: Attribute-key constants

**Files:**
- Create: `internal/logging/keys.go`

No test: these are bare constants (project policy — no tests for thin/no-behavior code).

- [ ] **Step 1: Write the constants**

```go
// Package logging provides the structured-logging seam: a ctx-reading slog
// handler, canonical attribute keys, an error-category enum, a logging HTTP
// round-tripper, and redaction helpers.
package logging

// Canonical slog attribute keys. Use these constants instead of string
// literals so keys never drift across call sites.
const (
	KeyRequestID      = "request_id"
	KeyJobID          = "job_id"
	KeyJobType        = "job_type"
	KeyUserID         = "user_id"
	KeySource         = "source"
	KeyOperation      = "operation"
	KeyExternalGameID = "external_game_id"
	KeyDurationMS     = "duration_ms"
	KeyHost           = "host"
	KeyEndpoint       = "endpoint"
	KeyStatus         = "status"
	KeyRoute          = "route"
	KeyLatency        = "latency"
	KeyOutcome        = "outcome"
	KeyCategory       = "category"
	KeyErr            = "err"
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/logging/`
Expected: success (package compiles with just constants).

- [ ] **Step 3: Commit**

```bash
git add internal/logging/keys.go
git commit -m "feat: add canonical slog attribute-key constants"
```

---

## Task 2: Context keys and setters

**Files:**
- Create: `internal/logging/context.go`

No standalone test here; behavior is exercised by the handler test in Task 3.

- [ ] **Step 1: Write context.go**

```go
package logging

import "context"

// ctxKey is an unexported type so keys never collide with other packages'.
type ctxKey int

const (
	requestIDKey ctxKey = iota
	jobIDKey
	userIDKey
)

// WithRequestID returns a ctx carrying the HTTP request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// WithJobID returns a ctx carrying the River job id.
func WithJobID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, jobIDKey, id)
}

// WithUserID returns a ctx carrying the authenticated user id.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func requestID(ctx context.Context) string { s, _ := ctx.Value(requestIDKey).(string); return s }
func jobID(ctx context.Context) string     { s, _ := ctx.Value(jobIDKey).(string); return s }
func userID(ctx context.Context) string    { s, _ := ctx.Value(userIDKey).(string); return s }
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/logging/`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/logging/context.go
git commit -m "feat: add ctx setters for logging correlation ids"
```

---

## Task 3: ContextHandler (the seam)

**Files:**
- Create: `internal/logging/handler.go`
- Test: `internal/logging/handler_test.go`

- [ ] **Step 1: Write the failing test**

```go
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// newTestLogger returns a logger writing JSON into buf, wrapped by ContextHandler.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewContextHandler(inner))
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal log line: %v (raw=%q)", err, buf.String())
	}
	return m
}

func TestContextHandler_InjectsCorrelation(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	ctx := WithRequestID(context.Background(), "req-123")
	ctx = WithJobID(ctx, "job-456")
	ctx = WithUserID(ctx, "user-789")

	log.InfoContext(ctx, "hello")

	m := decode(t, &buf)
	if m[KeyRequestID] != "req-123" {
		t.Errorf("request_id = %v, want req-123", m[KeyRequestID])
	}
	if m[KeyJobID] != "job-456" {
		t.Errorf("job_id = %v, want job-456", m[KeyJobID])
	}
	if m[KeyUserID] != "user-789" {
		t.Errorf("user_id = %v, want user-789", m[KeyUserID])
	}
}

func TestContextHandler_OmitsAbsentValues(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.InfoContext(context.Background(), "hello")

	m := decode(t, &buf)
	for _, k := range []string{KeyRequestID, KeyJobID, KeyUserID} {
		if _, present := m[k]; present {
			t.Errorf("key %q should be absent when ctx has no value", k)
		}
	}
}

func TestContextHandler_PreservesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf).With(KeyJobType, "sync_steam")

	log.InfoContext(WithJobID(context.Background(), "j1"), "hello")

	m := decode(t, &buf)
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyJobID] != "j1" {
		t.Errorf("job_id = %v, want j1", m[KeyJobID])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestContextHandler -v`
Expected: FAIL — `NewContextHandler` undefined.

- [ ] **Step 3: Write the handler**

```go
package logging

import (
	"context"
	"log/slog"
)

// ContextHandler wraps an inner slog.Handler and, on every record, injects
// correlation attributes found in the context (request_id, job_id, user_id).
// Reading ctx per-record (rather than binding once) is the OTel seam: a future
// handler adds trace_id/span_id here without touching any call site (#911).
type ContextHandler struct {
	inner slog.Handler
}

// NewContextHandler wraps inner so that ctx-carried correlation ids are added
// to each emitted record.
func NewContextHandler(inner slog.Handler) *ContextHandler {
	return &ContextHandler{inner: inner}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v := requestID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyRequestID, v))
	}
	if v := jobID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyJobID, v))
	}
	if v := userID(ctx); v != "" {
		r.AddAttrs(slog.String(KeyUserID, v))
	}
	return h.inner.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{inner: h.inner.WithGroup(name)}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestContextHandler -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/logging/handler.go internal/logging/handler_test.go
git commit -m "feat: add ctx-reading slog ContextHandler"
```

---

## Task 4: Error-category enum

**Files:**
- Create: `internal/logging/category.go`
- Test: `internal/logging/category_test.go`

- [ ] **Step 1: Write the failing test**

```go
package logging

import (
	"bytes"
	"context"
	"testing"
)

func TestCat_EmitsCategory(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "db down", Cat(CategoryDB))

	m := decode(t, &buf)
	if m[KeyCategory] != "db" {
		t.Errorf("category = %v, want db", m[KeyCategory])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestCat -v`
Expected: FAIL — `Cat`/`CategoryDB` undefined.

- [ ] **Step 3: Write category.go**

```go
package logging

import "log/slog"

// Category is a fixed, low-cardinality taxonomy for error logs, set at error
// boundaries to make failures aggregatable and greppable.
type Category string

const (
	CategoryExternalAPI Category = "external_api"
	CategoryDB          Category = "db"
	CategoryValidation  Category = "validation"
	CategoryAuth        Category = "auth"
	CategoryConfig      Category = "config"
)

// Cat returns the slog attribute for an error category.
func Cat(c Category) slog.Attr {
	return slog.String(KeyCategory, string(c))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestCat -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/category.go internal/logging/category_test.go
git commit -m "feat: add fixed error-category enum for logs"
```

---

## Task 5: Redaction helper

**Files:**
- Create: `internal/logging/redact.go`
- Test: `internal/logging/redact_test.go`

This helper is for the rare call site that must log a value derived from sensitive
material (e.g. a truncated identifier). It is NOT a blanket scrubber.

- [ ] **Step 1: Write the failing test**

```go
package logging

import "testing"

func TestRedact(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"short", "abcd", "****"},
		{"long", "supersecretvalue", "supe…[redacted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestRedact -v`
Expected: FAIL — `Redact` undefined.

- [ ] **Step 3: Write redact.go**

```go
package logging

import "strings"

// Redact returns a safe, non-reversible rendering of a sensitive string for
// logging. Short values (<=4 runes) become asterisks; longer values keep a
// 4-rune prefix as a weak correlation hint followed by a redaction marker.
func Redact(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 4 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "…[redacted]"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestRedact -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/redact.go internal/logging/redact_test.go
git commit -m "feat: add log redaction helper"
```

---

## Task 6: Logging HTTP RoundTripper

**Files:**
- Create: `internal/logging/roundtripper.go`
- Test: `internal/logging/roundtripper_test.go`

The RoundTripper logs one line per outbound call: `host`, `endpoint` (URL path with
the query **stripped**), `status`, `duration_ms`. Query-stripping bounds cardinality
and prevents leaking query secrets (e.g. Steam's `?key=`).

- [ ] **Step 1: Write the failing test**

```go
package logging

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoundTripper_LogsAndStripsQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	client := &http.Client{Transport: NewRoundTripper(http.DefaultTransport)}
	resp, err := client.Get(srv.URL + "/ISteamUser/GetPlayerSummaries/v2/?key=SECRET&steamids=1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	m := decode(t, &buf)
	if m[KeyStatus] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want 418", m[KeyStatus])
	}
	if got, ok := m[KeyEndpoint].(string); !ok || got != "/ISteamUser/GetPlayerSummaries/v2/" {
		t.Errorf("endpoint = %v, want path without query", m[KeyEndpoint])
	}
	if bytes.Contains(buf.Bytes(), []byte("SECRET")) {
		t.Errorf("log leaked query secret: %s", buf.String())
	}
	if _, ok := m[KeyDurationMS]; !ok {
		t.Errorf("missing duration_ms")
	}
}

func TestRoundTripper_LogsTransportError(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	client := &http.Client{Transport: NewRoundTripper(http.DefaultTransport)}
	// .invalid is a reserved TLD that never resolves.
	_, err := client.Get("http://nonexistent.invalid/x")
	if err == nil {
		t.Fatal("expected transport error")
	}
	m := decode(t, &buf)
	if m[KeyStatus] != float64(0) {
		t.Errorf("status = %v, want 0 on transport error", m[KeyStatus])
	}
	if _, ok := m[KeyErr]; !ok {
		t.Errorf("expected err attr on transport failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestRoundTripper -v`
Expected: FAIL — `NewRoundTripper` undefined.

- [ ] **Step 3: Write roundtripper.go**

```go
package logging

import (
	"log/slog"
	"net/http"
	"time"
)

// roundTripper wraps a base http.RoundTripper and logs one line per call with
// host, endpoint (path only — query stripped), status, and duration_ms. It
// never mutates the response or error it returns.
type roundTripper struct {
	base http.RoundTripper
}

// NewRoundTripper wraps base with request/duration logging. If base is nil,
// http.DefaultTransport is used.
func NewRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &roundTripper{base: base}
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := rt.base.RoundTrip(req)
	dur := time.Since(start).Milliseconds()

	attrs := []any{
		KeyHost, req.URL.Host,
		KeyEndpoint, req.URL.Path, // query intentionally omitted
		KeyDurationMS, dur,
	}
	ctx := req.Context()
	if err != nil {
		attrs = append(attrs, KeyStatus, 0, KeyErr, err.Error(), KeyCategory, string(CategoryExternalAPI))
		slog.WarnContext(ctx, "external api call failed", attrs...)
		return resp, err
	}
	attrs = append(attrs, KeyStatus, resp.StatusCode)
	slog.DebugContext(ctx, "external api call", attrs...)
	return resp, nil
}
```

> Note on levels: a *successful* external call is per-call detail → `debug`. A
> *transport failure* is recoverable/retryable → `warn` with `category=external_api`.
> Non-2xx HTTP statuses are still `debug` here (the caller decides if a 404/500 is
> actionable and logs at its own boundary) — this avoids double-logging.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestRoundTripper -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/logging/roundtripper.go internal/logging/roundtripper_test.go
git commit -m "feat: add logging HTTP round-tripper with query-strip secret guard"
```

---

## Task 7: River WorkerMiddleware (job_id + outcome/duration)

**Files:**
- Create: `internal/logging/middleware.go`
- Test: `internal/logging/middleware_test.go`

- [ ] **Step 1: Write the failing test**

```go
package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
)

func TestWorkerMiddleware_Success(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	mw := NewWorkerMiddleware()
	job := &rivertype.JobRow{ID: 42, Kind: "sync_steam"}

	var sawJobID string
	err := mw.Work(context.Background(), job, func(ctx context.Context) error {
		sawJobID = jobID(ctx)
		return nil
	})
	if err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if sawJobID != "42" {
		t.Errorf("job_id in ctx = %q, want 42", sawJobID)
	}
	m := decode(t, &buf)
	if m[KeyJobType] != "sync_steam" {
		t.Errorf("job_type = %v, want sync_steam", m[KeyJobType])
	}
	if m[KeyOutcome] != "completed" {
		t.Errorf("outcome = %v, want completed", m[KeyOutcome])
	}
	if m[KeyJobID] != "42" {
		t.Errorf("job_id = %v, want 42", m[KeyJobID])
	}
	if _, ok := m[KeyDurationMS]; !ok {
		t.Errorf("missing duration_ms")
	}
}

func TestWorkerMiddleware_Failure(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	mw := NewWorkerMiddleware()
	job := &rivertype.JobRow{ID: 7, Kind: "import_item"}
	want := errors.New("boom")

	err := mw.Work(context.Background(), job, func(ctx context.Context) error {
		time.Sleep(time.Millisecond)
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("Work error = %v, want it to wrap boom", err)
	}
	m := decode(t, &buf)
	if m[KeyOutcome] != "failed" {
		t.Errorf("outcome = %v, want failed", m[KeyOutcome])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestWorkerMiddleware -v`
Expected: FAIL — `NewWorkerMiddleware` undefined.

- [ ] **Step 3: Write middleware.go**

```go
package logging

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WorkerMiddleware binds the River job id into ctx (so every in-job log line is
// correlated via ContextHandler) and emits exactly one outcome line per job with
// job_type, outcome, and duration_ms.
type WorkerMiddleware struct {
	river.MiddlewareDefaults
}

// NewWorkerMiddleware constructs the job logging middleware.
func NewWorkerMiddleware() *WorkerMiddleware { return &WorkerMiddleware{} }

func (m *WorkerMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	id := strconv.FormatInt(job.ID, 10)
	ctx = WithJobID(ctx, id)

	start := time.Now()
	err := doInner(ctx)
	dur := time.Since(start).Milliseconds()

	outcome := "completed"
	level := slog.LevelInfo
	if err != nil {
		outcome = "failed"
		level = slog.LevelWarn
	}
	slog.Log(ctx, level, "job finished",
		KeyJobType, job.Kind,
		KeyOutcome, outcome,
		KeyDurationMS, dur,
	)
	return err
}
```

> The middleware returns `err` unchanged so River's retry semantics are preserved.
> Outcome is `info` on success, `warn` on failure (River will retry; a permanently
> dead job is surfaced elsewhere).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/logging/ -run TestWorkerMiddleware -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/logging/middleware.go internal/logging/middleware_test.go
git commit -m "feat: add River worker logging middleware (job_id + outcome/duration)"
```

---

## Task 8: Install the ContextHandler and River middleware in serve.go

**Files:**
- Modify: `cmd/nexorious/serve.go:59-61` (handler), `:231-234` and `:327-330` (river clients)

- [ ] **Step 1: Wrap the JSON handler**

Replace (`serve.go:59-61`):

```go
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))
```

with:

```go
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	}))))
```

Add `"github.com/drzero42/nexorious/internal/logging"` to the import block.

- [ ] **Step 2: Register the worker middleware at both river.NewClient sites**

At `serve.go:231` add a `Middleware` field to the `river.Config`:

```go
	riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
		Middleware:   []rivertype.Middleware{logging.NewWorkerMiddleware()},
		// ...existing fields (PeriodicJobs, ErrorHandler, etc.) unchanged...
	})
```

Apply the identical `Middleware:` line to the second client in `RebuildServices` (`serve.go:327`).

Add `"github.com/riverqueue/river/rivertype"` to the import block.

- [ ] **Step 3: Build**

Run: `go build ./cmd/... ./internal/...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "feat: install ctx logging handler and River job logging middleware"
```

---

## Task 9: HTTP request_id middleware

**Files:**
- Create: `internal/api/requestid.go`
- Test: `internal/api/requestid_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

func TestRequestIDMiddleware_GeneratesAndPropagates(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seenInCtx, seenHeader string
	e.GET("/x", func(c *echo.Context) error {
		seenInCtx = logging.RequestIDForTest(c.Request().Context())
		seenHeader = c.Response().Header().Get(echo.HeaderXRequestID)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if seenInCtx == "" {
		t.Error("request id not present in request context")
	}
	if seenHeader == "" || seenHeader != seenInCtx {
		t.Errorf("response header %q should match ctx id %q", seenHeader, seenInCtx)
	}
}

func TestRequestIDMiddleware_HonorsInbound(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seen string
	e.GET("/x", func(c *echo.Context) error {
		seen = logging.RequestIDForTest(c.Request().Context())
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(echo.HeaderXRequestID, "inbound-abc")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if seen != "inbound-abc" {
		t.Errorf("ctx request id = %q, want inbound-abc", seen)
	}
}
```

This test needs a tiny test accessor in the logging package. Add to
`internal/logging/context.go`:

```go
// RequestIDForTest exposes the ctx-carried request id for tests in other packages.
func RequestIDForTest(ctx context.Context) string { return requestID(ctx) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestRequestIDMiddleware -v`
Expected: FAIL — `RequestIDMiddleware` undefined.

- [ ] **Step 3: Write requestid.go**

```go
package api

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

// RequestIDMiddleware ensures every request carries an id: it honors an inbound
// X-Request-Id header if present, otherwise generates one. The id is echoed back
// in the response header and seeded into the request context so the slog
// ContextHandler stamps it on every in-request log line.
func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := c.Request().Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = uuid.NewString()
			}
			c.Response().Header().Set(echo.HeaderXRequestID, id)
			ctx := logging.WithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
```

> `github.com/google/uuid v1.6.0` is already a direct dependency (verified in go.mod),
> so `uuid.NewString()` needs no new require.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestRequestIDMiddleware -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/api/requestid.go internal/api/requestid_test.go internal/logging/context.go
git commit -m "feat: add HTTP request_id middleware seeding ctx and response header"
```

---

## Task 10: Wire request_id middleware + extend RequestLogger in router.go

**Files:**
- Modify: `internal/api/router.go:57-72`

- [ ] **Step 1: Register the middleware before RequestLogger**

Immediately after `e.Use(middleware.Recover())` (`router.go:57`), add:

```go
	e.Use(RequestIDMiddleware())
```

- [ ] **Step 2: Extend the RequestLogger config**

Replace the `RequestLoggerWithConfig` block (`router.go:58-72`) with:

```go
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRoutePath: true,
		LogRequestID: true,
		HandleError:  true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()
			attrs := []any{
				"method", v.Method,
				"uri", v.URI,
				logging.KeyRoute, v.RoutePath,
				logging.KeyStatus, v.Status,
				logging.KeyLatency, v.Latency,
				logging.KeyRequestID, v.RequestID,
			}
			if uid := auth.UserIDFromContext(c); uid != "" {
				attrs = append(attrs, logging.KeyUserID, uid)
			}
			if v.Error != nil {
				slog.ErrorContext(ctx, "request", append(attrs, logging.KeyErr, v.Error)...)
			} else {
				slog.InfoContext(ctx, "request", attrs...)
			}
			return nil
		},
	}))
```

Add `"github.com/drzero42/nexorious/internal/logging"` to router.go imports (`auth`
is already imported).

> `LogRoutePath` gives `v.RoutePath` (the matched pattern, e.g. `/api/games/:id`) —
> low cardinality. `LogRequestID` reads the `X-Request-Id` header our middleware set.
> Confirm both fields exist on this Echo version: `grep -n "RoutePath\|RequestID" $(go env GOMODCACHE)/github.com/labstack/echo/v5@*/middleware/request_logger.go`.
> If `LogRoutePath` is unavailable, use `c.Path()` inside LogValuesFunc instead.

- [ ] **Step 3: Build + run existing router tests**

Run: `go build ./internal/api/ && go test ./internal/api/ -run TestRequest -v`
Expected: success; request_id tests still pass.

- [ ] **Step 4: Commit**

```bash
git add internal/api/router.go
git commit -m "feat: emit request_id/route/user_id from Echo request logger"
```

---

## Task 11: Seed user_id into request ctx in AuthMiddleware

**Files:**
- Modify: `internal/auth/session.go:153-156`

- [ ] **Step 1: Add the ctx seed**

After `c.Set("user_id", user.ID)` (`session.go:153`), also seed the Go context so
in-request log lines below the middleware carry `user_id`:

```go
			c.Set("user_id", user.ID)
			c.Set("is_admin", user.IsAdmin)
			c.Set("user", &user)
			c.Set("session_hash", sessionHash)
			c.SetRequest(c.Request().WithContext(logging.WithUserID(c.Request().Context(), user.ID)))
```

Add `"github.com/drzero42/nexorious/internal/logging"` to session.go imports.

> Check for an import cycle: `internal/logging` must not import `internal/auth`.
> It does not (logging only imports stdlib + river). Safe.

- [ ] **Step 2: Build + targeted auth tests**

Run: `go build ./internal/auth/ && go test ./internal/auth/ -v -run TestAuth`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/session.go
git commit -m "feat: seed user_id into request ctx for log correlation"
```

---

## Task 12: Install the RoundTripper on every service HTTP client

Each client must get its **own** `http.Client` with the logging transport — never
mutate the shared `http.DefaultClient`. Replace `http.DefaultClient` usages with an
owned client.

**Files & exact changes:**

- `internal/services/steam/client.go:28` — `http: &http.Client{}` →
  `http: &http.Client{Transport: logging.NewRoundTripper(nil)}`
- `internal/services/igdb/igdb.go:53` — `&http.Client{Timeout: 30 * time.Second}` →
  `&http.Client{Timeout: 30 * time.Second, Transport: logging.NewRoundTripper(nil)}`
- `internal/services/igdb/auth.go:36` — `&http.Client{Timeout: 10 * time.Second}` →
  add `Transport: logging.NewRoundTripper(nil)`
- `internal/services/updatecheck/client.go:33` — `&http.Client{Timeout: 30 * time.Second}` →
  add `Transport: logging.NewRoundTripper(nil)`
- `internal/services/gog/client.go:25,36` — `httpClient: http.DefaultClient` →
  `httpClient: &http.Client{Transport: logging.NewRoundTripper(nil)}`
- `internal/services/humble/client.go:36` — `httpClient: http.DefaultClient` →
  `httpClient: &http.Client{Transport: logging.NewRoundTripper(nil)}`
- `internal/services/playstationstore/client.go:47` — `httpClient: http.DefaultClient` →
  owned client; and `:121` `http.DefaultClient.Do(req)` → use `c.httpClient.Do(req)`
  (the struct field) instead of the global.

> The third-party `psnsdk.NewClient` and the Epic/Legendary subprocess client do
> their own I/O and are NOT wrapped — note this limitation in the doc (Task 16).
> Add `"github.com/drzero42/nexorious/internal/logging"` to each modified file.

- [ ] **Step 1: Apply the transport change to each client listed above**

- [ ] **Step 2: Build all services**

Run: `go build ./internal/services/...`
Expected: success.

- [ ] **Step 3: Run service tests**

Run: `go test ./internal/services/... 2>&1 | tail -30`
Expected: PASS (transport wrapping is transparent to existing tests).

- [ ] **Step 4: Commit**

```bash
git add internal/services/
git commit -m "feat: log external API calls via shared logging round-tripper"
```

---

## Task 13: RequestLogger secret-exclusion test

Prove the request logger never emits auth headers or bodies.

**Files:**
- Create/extend: `internal/api/router_logging_test.go`

- [ ] **Step 1: Write the test**

```go
package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

// buildLoggingEcho mirrors the request-logging middleware wired in router.New,
// writing logs into buf for assertions.
func buildLoggingEcho(buf *bytes.Buffer) *echo.Echo {
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(buf, nil))))
	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogURI: true, LogMethod: true, LogLatency: true,
		LogRoutePath: true, LogRequestID: true, HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			slog.InfoContext(c.Request().Context(), "request",
				"method", v.Method, "uri", v.URI,
				logging.KeyRoute, v.RoutePath, logging.KeyStatus, v.Status,
				logging.KeyLatency, v.Latency, logging.KeyRequestID, v.RequestID)
			return nil
		},
	}))
	e.POST("/login", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func TestRequestLogger_NoSecretLeak(t *testing.T) {
	var buf bytes.Buffer
	e := buildLoggingEcho(&buf)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"hunter2"}`))
	req.Header.Set("Authorization", "Bearer SECRETTOKEN")
	req.Header.Set("Cookie", "session=SECRETSESSION")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	out := buf.String()
	for _, secret := range []string{"hunter2", "SECRETTOKEN", "SECRETSESSION"} {
		if strings.Contains(out, secret) {
			t.Errorf("request log leaked %q: %s", secret, out)
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/api/ -run TestRequestLogger_NoSecretLeak -v`
Expected: PASS (the logger never reads headers/body, so nothing leaks).

- [ ] **Step 3: Commit**

```bash
git add internal/api/router_logging_test.go
git commit -m "test: assert request logger never emits auth headers or bodies"
```

---

## Task 14: Re-leveling + correlation audit (case-by-case)

This is the largest task. It is **review-driven**, not per-site TDD: there is no unit
test per log line. Work **one directory at a time**, committing per directory, so the
diff stays reviewable. For each `slog.*` call site:

**Procedure per site:**
1. **Add ctx.** If the call is in a request/job code path and a `ctx context.Context`
   is in scope, switch `slog.Info(...)` → `slog.InfoContext(ctx, ...)` (same for
   Warn/Error/Debug). If no unit-of-work ctx exists (startup, factory), leave the bare
   call — that is correct.
2. **Re-level** per the policy table:
   - `error` → keep ONLY if the failure is actionable/unhandled (operator must act,
     data loss, job permanently dead).
   - If the code logs then **continues** (retry, skip, fallback, `continue`,
     `return nil`) → demote to `warn` (recoverable) or `debug` (per-item detail).
   - Lifecycle (start/stop/connected/finished) → `info`.
   Only demote when the surrounding code *demonstrably* handles the condition.
3. **Replace string-literal keys** with the `logging.Key*` constants.
4. **Set `category`** on remaining `error`/`warn` logs at error boundaries using
   `logging.Cat(...)` (e.g. DB failure → `logging.Cat(logging.CategoryDB)`).
5. **Remove redundant attrs** now injected by the handler: drop manual
   `"job_id", ...` / `"user_id", ...` / `"request_id", ...` pairs where the ctx now
   carries them (avoids duplicate keys). Keep them only where no ctx flows.

**Worked example** (`internal/worker/tasks/job_item.go:32`):

Before:
```go
func execItemUpdate(ctx context.Context, db *bun.DB, item *models.JobItem, logPrefix string, columns ...string) {
	// ...
	slog.Error(logPrefix, "id", item.ID, "err", err)
}
```
After (DB update failed but the caller continues processing other items → demote to
`warn`, add category, thread ctx, keep id):
```go
	slog.WarnContext(ctx, logPrefix, "id", item.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
```

**Counter-example — keep as Error** (a job that cannot proceed at all, data-loss risk):
leave `slog.ErrorContext(ctx, "export: markJobFailed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))`.

**Directories to sweep (commit one per directory):**

- [ ] **Step 1: `internal/worker/` (largest concentration)**
  Sweep every `slog.*` call. Build: `go build ./internal/worker/...`.
  Commit: `refactor: re-level and correlate logs in worker tasks`

- [ ] **Step 2: `internal/api/`**
  Build: `go build ./internal/api/...`.
  Commit: `refactor: re-level and correlate logs in api handlers`

- [ ] **Step 3: `internal/services/`**
  Demote handled external-API failures to `warn`+`category=external_api`; per-call
  detail to `debug` (most is now covered by the RoundTripper — delete now-redundant
  per-call success logs to honor "log once at the boundary").
  Build: `go build ./internal/services/...`.
  Commit: `refactor: re-level and correlate logs in service clients`

- [ ] **Step 4: `internal/scheduler/`, `internal/backup/`, `internal/notify/`, `internal/migrate/`**
  Build: `go build ./internal/...`.
  Commit: `refactor: re-level and correlate logs in scheduler/backup/notify/migrate`

- [ ] **Step 5: `cmd/nexorious/`**
  Startup logs mostly stay bare `slog.*` (no unit of work) but should use
  `logging.Key*`/`logging.Cat` and correct levels (e.g. `serve.go:475` decrypt
  failures are already `Warn` — verify keys).
  Build: `go build ./...`.
  Commit: `refactor: normalize startup log keys and levels`

- [ ] **Step 6: Verify the reduction**
  Run:
  ```bash
  for lvl in Error Warn Info Debug; do
    printf "%s: " "$lvl"
    grep -rEn "slog\.${lvl}(Context)?\b" --include='*.go' internal cmd | grep -v _test.go | wc -l
  done
  ```
  Expected: `Error` count is far below 260 (driven by code, no hard target); most former
  Errors now Warn/Debug.

---

## Task 15: Secrets/PII grep-audit (documented)

**Files:**
- This produces notes for the doc (Task 16); no code unless a leak is found.

- [ ] **Step 1: Run the audit greps**

```bash
# Auth material, encryption key, session cookies near log calls:
grep -rEn 'slog\.[A-Za-z]+\(' --include='*.go' internal cmd | grep -v _test.go \
  | grep -iE 'cookie|npsso|npsso|password|secret|token|api_key|apikey|encryption|session' || echo "no obvious secret-in-log sites"
# Confirm DB_ENCRYPTION_KEY is never logged as a value:
grep -rn 'DBEncryptionKey\|DB_ENCRYPTION_KEY' --include='*.go' internal cmd | grep -i slog || echo "encryption key not logged"
```

- [ ] **Step 2: Fix any real leak found**
  If a site logs a secret value, redact via `logging.Redact(...)` or drop the attr.
  Commit only if a change was made:
  ```bash
  git commit -am "fix: redact secret value from log output"
  ```

- [ ] **Step 3: Record findings** (carry into Task 16's doc "Secrets audit" section):
  list the sites checked and the conclusion (e.g. "Steam key only ever in query →
  stripped by RoundTripper; npsso never logged; session hash logged only as DB key,
  not value").

---

## Task 16: Logging-conventions doc + CLAUDE.md cross-link

**Files:**
- Create: `docs/logging-conventions.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Write `docs/logging-conventions.md`**

Include these sections (write real prose, not headers-only):
- **Call-site contract:** always `slog.*Context(ctx, …)` when a unit-of-work ctx is in
  scope; bare `slog.*` only in startup/factory code with no ctx.
- **Correlation:** `request_id` (HTTP) and `job_id` (River) are injected automatically
  by `internal/logging.ContextHandler`; never add them by hand. `user_id` likewise when
  authenticated.
- **Leveling policy:** the four-row table (`error`/`warn`/`info`/`debug`) from the spec,
  with the rule "demote anything the code handles and continues."
- **Attribute keys:** the `logging.Key*` constant set is the source of truth; list them.
- **Error taxonomy:** the fixed `category` enum (`external_api|db|validation|auth|config`)
  and when to set it (error boundaries).
- **External calls & jobs:** one line per call (RoundTripper) / per job outcome (worker
  middleware); don't double-log. Note the psnsdk/Legendary wrapping limitation.
- **Secrets:** never log bodies/headers/secrets; RoundTripper strips query; use
  `logging.Redact` for unavoidable sensitive-derived values. Summarize the Task 15 audit.

- [ ] **Step 2: Cross-link from CLAUDE.md**

In the `docs/` bullet (CLAUDE.md line ~78), add `logging-conventions.md` to the list of
reference docs that "stay in the repo for GitHub viewing but are **not** embedded/served."
Add a short line under Code Style → Go:

```
- **Logging:** use `slog.*Context(ctx, …)` and the `internal/logging` key constants; see [docs/logging-conventions.md](docs/logging-conventions.md). Correlation ids (`request_id`/`job_id`/`user_id`) are injected automatically — don't add them by hand.
```

- [ ] **Step 3: Commit**

```bash
git add docs/logging-conventions.md CLAUDE.md
git commit -m "docs: add logging-conventions reference and cross-link from CLAUDE.md"
```

---

## Task 17: Full verification & spec status

- [ ] **Step 1: Full build**
  Run: `go build ./...`
  Expected: success.

- [ ] **Step 2: Lint**
  Run: `golangci-lint run`
  Expected: zero findings. (Watch for `errcheck` on new `_ =` discards and `gosec` on the
  RoundTripper — none expected; add per-site `//nolint` only with justification.)

- [ ] **Step 3: Full test suite**
  Run: `go test -timeout 600s ./...`
  Expected: all pass.

- [ ] **Step 4: Mark spec approved/implemented**
  In `docs/superpowers/specs/2026-06-10-structured-logging-design.md`, change
  `**Status:** approved (brainstorming)` → `**Status:** implemented`.
  Commit:
  ```bash
  git add docs/superpowers/specs/2026-06-10-structured-logging-design.md
  git commit -m "docs: mark structured-logging spec implemented"
  ```

- [ ] **Step 5: Push and open the PR**
  ```bash
  git push -u origin feat/sharpen-structured-logging
  ```
  PR title (`feat:` — adds user-actionable fields):
  `feat: sharpen structured logging (correlation ids, leveling, taxonomy)`
  PR body must include `Closes #907`.

---

## Self-Review notes (coverage check vs spec)

- Seam (handler + ctx) → Tasks 1-3, 8. ✓
- request_id/job_id on every in-request/in-job line → Tasks 7, 9, 10, 11, 14. ✓
- duration_ms on external calls + job outcomes → Tasks 6, 7, 12. ✓
- Shared key constants used consistently → Task 1 + applied throughout 14. ✓
- Leveling audit, Error reserved → Task 14. ✓
- category enum on error logs → Tasks 4, 14. ✓
- Secrets/PII audit + tests → Tasks 6 (query-strip test), 13 (header/body test), 5 +
  15 (redact + grep audit). ✓
- RequestLogger emits request_id/route/status/latency/user_id → Task 10. ✓
- Doc written + cross-linked → Task 16. ✓
