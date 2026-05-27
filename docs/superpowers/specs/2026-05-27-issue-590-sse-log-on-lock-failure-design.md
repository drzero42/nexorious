# Issue #590 — Emit SSE log line on synchronous `bunMig.Lock` failure

**Date:** 2026-05-27
**Issue:** #590 (`chore(migrate): emit SSE log line on synchronous bunMig.Lock failure`)
**Type:** Bug fix (cosmetic)
**Follow-up from:** #583 review feedback

## Problem

In `internal/migrate/migrator.go`, `RunMigrations` has four internal failure paths.
Three of them — `bunMig.Migrate`, River migrator setup, and River migrate — emit a
`sendLog(ch, ...)` line to the SSE channel before transitioning to
`AppStateMigrationFailed` and closing the channel. The fourth, the synchronous
`bunMig.Lock(ctx)` failure path, is the odd one out: it transitions and closes the
channel without ever emitting a log line.

### User-visible symptom

When migration fails at the Lock step, the `/migrate` page renders the failure card
correctly (`Migration failed: migrate: acquire lock: ...`), but the streaming log box
above it stays empty — operators see a `<div class="log visible">` with no content.
Purely cosmetic, but inconsistent with the other failure paths.

## Change 1 — Code

In `internal/migrate/migrator.go` (the Lock-failure branch, currently around lines
203–208), add one `sendLog` call before `TransitionToFailed`, mirroring the sibling
paths:

```go
if err := mg.bunMig.Lock(ctx); err != nil {
    wrapped := fmt.Errorf("migrate: acquire lock: %w", err)
    mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", wrapped))
    mg.TransitionToFailed(wrapped)
    close(ch)
    return wrapped
}
```

### Decision: log `wrapped`, not raw `err`

The three sibling paths log the raw `err` with a path-specific prefix
(`"River migration failed: %v\n"`, etc.). This fix logs the `wrapped` error instead,
producing `"migration failed: migrate: acquire lock: <err>\n"`. That matches exactly
what the failure card displays (`mg.LastError()` stores `wrapped.Error()`), giving the
operator a single consistent message in both places. The mild asymmetry with the
sibling prefixes is harmless. This follows the fix proposed and reviewed in the issue.

## Change 2 — Test

Extend the existing `TestRunMigrations_FailureTransitionsToFailedWithError`
(`internal/migrate/migrator_test.go`). That test already forces a Lock failure by
closing the underlying `*sql.DB` before calling `RunMigrations`. After the existing
`State` and `LastError` assertions, drain the log channel and assert the line was
emitted:

```go
var logged strings.Builder
for line := range m.LogCh() {
    logged.WriteString(line)
}
if !strings.Contains(logged.String(), "migration failed") {
    t.Errorf("expected a log line emitted on Lock failure, got %q", logged.String())
}
```

This is safe because by the time `RunMigrations` returns, `sendLog` has written to the
buffered channel and `close(ch)` has run, so the `range` drains and terminates without
blocking. `LogCh()` returns the channel only after `RunMigrations` releases
`migrateMu`. Adds a `strings` import to the test file.

## Scope & non-goals

- One line of production code plus a small test extension.
- No change to architecture, data flow, state machine, or the other three failure
  paths.
- Not refactoring this package's per-test fresh-container pattern
  (`setupTestDB(t)` deliberately starts a new container per test here, because several
  tests close the DB to force errors — this departs from the shared-`testDB` guidance
  in CLAUDE.md and stays as-is).
