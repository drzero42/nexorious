# Issue #590 — Emit SSE Log Line on `bunMig.Lock` Failure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit an SSE log line on the synchronous `bunMig.Lock(ctx)` failure path in `RunMigrations` so the `/migrate` log box is not empty when migration fails at the Lock step.

**Architecture:** `RunMigrations` (in `internal/migrate/migrator.go`) has four internal failure paths; three call `sendLog(ch, ...)` before transitioning to failed, but the Lock path does not. Add the missing `sendLog` call to match the siblings. Drive the change with a failing-test-first extension to the existing Lock-failure test.

**Tech Stack:** Go 1.25, `uptrace/bun/migrate`, stdlib `testing`, `testcontainers-go` (per-test fresh postgres container in this package).

**Spec:** `docs/superpowers/specs/2026-05-27-issue-590-sse-log-on-lock-failure-design.md`

---

### Task 1: Emit the missing log line on the Lock-failure path

**Files:**
- Test: `internal/migrate/migrator_test.go` (extend `TestRunMigrations_FailureTransitionsToFailedWithError`, around lines 333–355; add `strings` import)
- Modify: `internal/migrate/migrator.go` (Lock-failure branch, around lines 203–208)

- [ ] **Step 1: Add the `strings` import to the test file**

Open `internal/migrate/migrator_test.go`. The import block currently is:

```go
import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)
```

Add `"strings"` to the stdlib group so it reads:

```go
import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)
```

- [ ] **Step 2: Add the failing assertion to the existing test**

In `TestRunMigrations_FailureTransitionsToFailedWithError`, the body currently ends:

```go
	if m.State() != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty, want non-empty")
	}
}
```

Append a log-channel drain + assertion before the closing brace, so it reads:

```go
	if m.State() != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty, want non-empty")
	}

	// The Lock-failure path must emit a log line before closing the SSE
	// channel, matching the other failure paths (issue #590). RunMigrations
	// has returned, so the channel is buffered, written, and closed: this
	// range drains and terminates without blocking.
	var logged strings.Builder
	for line := range m.LogCh() {
		logged.WriteString(line)
	}
	if !strings.Contains(logged.String(), "migration failed") {
		t.Errorf("expected a log line emitted on Lock failure, got %q", logged.String())
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/migrate/ -run TestRunMigrations_FailureTransitionsToFailedWithError -v`
Expected: FAIL — the new assertion reports `got ""` because the Lock-failure path currently emits no log line. (The `State`/`LastError` assertions still pass.)

- [ ] **Step 4: Add the `sendLog` call to the Lock-failure path**

In `internal/migrate/migrator.go`, the Lock-failure branch in `RunMigrations` currently is:

```go
	if err := mg.bunMig.Lock(ctx); err != nil {
		wrapped := fmt.Errorf("migrate: acquire lock: %w", err)
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
```

Add the `sendLog` line before `TransitionToFailed`, mirroring the sibling paths:

```go
	if err := mg.bunMig.Lock(ctx); err != nil {
		wrapped := fmt.Errorf("migrate: acquire lock: %w", err)
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", wrapped))
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/migrate/ -run TestRunMigrations_FailureTransitionsToFailedWithError -v`
Expected: PASS.

- [ ] **Step 6: Run the full migrate package tests to confirm no regression**

Run: `go test ./internal/migrate/ -v`
Expected: PASS (all tests). This confirms the added `sendLog` does not affect the success paths, which set `mg.logCh` and drain it elsewhere.

- [ ] **Step 7: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go
git commit -m "fix(migrate): emit SSE log line on synchronous bunMig.Lock failure"
```
