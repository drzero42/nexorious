# Clear `lastError` on DBUnavailable Recovery — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clear the stale `mg.lastError` in the `default:` arm of `recoverFromUnavailable` so a `MigrationFailed → DBUnavailable → recovery` cycle leaves no phantom error string behind.

**Architecture:** One-line `mg.lastError.Store("")` at the top of the `default:` recovery arm (restoring symmetry with `RunMigrations`' happy-path clear), plus a deterministic regression test driven through `StartDBProbe` against a good DB. A new `SetPrevStateForTest` helper lets the test stage `prevState = MigrationFailed` without racing bad-DB probe timing.

**Tech Stack:** Go 1.25, stdlib `testing`, `testcontainers-go` (PostgreSQL), Bun migrate.

**Spec:** `docs/superpowers/specs/2026-05-27-issue-589-clear-lasterror-on-recovery-design.md`

**Branch:** `fix/589-clear-lasterror-on-recovery` (already created and checked out)

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/migrate/migrator.go` | Migrator state machine + recovery logic + test seams | Add `SetPrevStateForTest` helper; add one-line `lastError` clear in `default:` arm of `recoverFromUnavailable` |
| `internal/migrate/recover_test.go` | Probe/recovery regression tests (external `migrate_test` pkg) | Add `errors` import; add `TestStartDBProbe_RecoveryFromFailed_ClearsLastError` |

No migrations, no frontend, no slumber routes, no API-surface changes.

---

## Task 1: Clear `lastError` on recovery from `MigrationFailed`

**Files:**
- Modify: `internal/migrate/migrator.go` — add `SetPrevStateForTest` (near the existing `SetStateForTest` at line 265); add one line in the `default:` arm of `recoverFromUnavailable` (line 360)
- Test: `internal/migrate/recover_test.go` — new test + `errors` import

- [ ] **Step 1: Add the `SetPrevStateForTest` test seam**

In `internal/migrate/migrator.go`, immediately after the existing `SetStateForTest` method (currently at lines 265-267):

```go
func (mg *Migrator) SetStateForTest(s AppState) {
	mg.state.Store(int32(s))
}

func (mg *Migrator) SetPrevStateForTest(s AppState) {
	mg.prevState.Store(int32(s))
}
```

This mirrors the existing test seams (`SetStateForTest`, `SetProbeIntervalForTest`, `NewMigratorForTest`). `prevState` is otherwise written only inside the `StartDBProbe` goroutine; this setter lets a test stage the recovery entry point deterministically.

- [ ] **Step 2: Write the failing regression test**

In `internal/migrate/recover_test.go`, add `errors` to the import block (the file currently imports only `context`, `testing`, `time`, and the `migrate` package):

```go
import (
	"context"
	"errors"
	"testing"
	"time"

	migrate "github.com/drzero42/nexorious/internal/migrate"
)
```

Then append this test:

```go
// TestStartDBProbe_RecoveryFromFailed_ClearsLastError exercises the default
// branch of recoverFromUnavailable after a MigrationFailed → DBUnavailable
// transition. The recovery re-determines state from the (now reachable) DB and
// must clear the stale lastError inherited from the earlier failure.
func TestStartDBProbe_RecoveryFromFailed_ClearsLastError(t *testing.T) {
	db := setupTestDB(t)

	// Bring the DB to a known-good, fully-migrated state.
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Record a failure, then stage the recovery entry point deterministically:
	// prevState = MigrationFailed (the default-arm trigger), state = DBUnavailable.
	m.TransitionToFailed(errors.New("boom"))
	if m.LastError() != "boom" {
		t.Fatalf("precondition: expected LastError %q, got %q", "boom", m.LastError())
	}
	m.SetPrevStateForTest(migrate.AppStateMigrationFailed)
	m.SetStateForTest(migrate.AppStateDBUnavailable)

	// Probe against the GOOD db: state is DBUnavailable and the ping succeeds, so
	// the probe recovers via recoverFromUnavailable(prev=MigrationFailed) — the
	// default arm. No bad-DB timing dependency, so the assertion always runs.
	m.SetProbeIntervalForTest(30 * time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.StartDBProbe(ctx, db, func(_ context.Context) error { return nil })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() != migrate.AppStateDBUnavailable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.State() == migrate.AppStateDBUnavailable {
		t.Fatal("expected recovery from DBUnavailable (default branch), but still DBUnavailable")
	}
	if got := m.LastError(); got != "" {
		t.Errorf("expected LastError cleared after recovery, got %q", got)
	}
	if m.State() != migrate.AppStateReady {
		t.Errorf("expected Ready after recovery, got %v", m.State())
	}
}
```

- [ ] **Step 3: Run the test to verify it fails on the regression assertion**

Run: `go test ./internal/migrate/... -run TestStartDBProbe_RecoveryFromFailed_ClearsLastError -v`

Expected: FAIL. The probe recovers (state leaves `DBUnavailable` → reaches `Ready`), but because the production fix is absent, the assertion reports:
`expected LastError cleared after recovery, got "boom"`

If it instead fails to compile, re-check Step 1 (the `SetPrevStateForTest` seam) and the `errors` import in Step 2.

- [ ] **Step 4: Add the one-line production fix**

In `internal/migrate/migrator.go`, in the `default:` arm of `recoverFromUnavailable` (currently line 360), add the clear as the first statement:

```go
	default:
		mg.lastError.Store("") // clear any failure inherited via DBUnavailable
		if mg.bunMig != nil {
			mg.bunMig = nil
		}
		if err := mg.determineState(); err != nil {
			return err
		}
		if prev == AppStateReady && mg.NeedsSetup() {
			if err := mg.InitNeedsSetup(ctx, db); err != nil {
				mg.state.Store(int32(AppStateDBUnavailable))
				return fmt.Errorf("re-check needsSetup: %w", err)
			}
		}
		slog.Info("db probe: recovery complete (re-determined state)", "state", mg.State())
	}
```

Do **not** add the clear to the `AppStateMigrating` arm: `RunMigrations` already clears `lastError` before flipping to `Migrating` (`migrator.go:200`), so that arm can never carry a stale error.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/migrate/... -run TestStartDBProbe_RecoveryFromFailed_ClearsLastError -v`

Expected: PASS.

- [ ] **Step 6: Run the surrounding probe tests to confirm no regression**

Run: `go test ./internal/migrate/... -run TestStartDBProbe -v`

Expected: PASS for all three probe tests (`RecoveryFromMigrating`, `RecoveryFromReady`, `RecoveryFromFailed_ClearsLastError`).

- [ ] **Step 7: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/recover_test.go
git commit -m "chore(migrate): clear lastError in recoverFromUnavailable default branch (#589)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Production one-line clear in `default:` arm → Task 1, Step 4. ✓
- `default:`-only rationale (not `Migrating`) → Step 4 note. ✓
- `SetPrevStateForTest` test seam → Step 1. ✓
- Regression test `MigrationFailed → DBUnavailable → recovery`, asserts `LastError() == ""` → Step 2, deterministic (good-DB only). ✓
- Test fails pre-fix, passes post-fix → Steps 3 & 5. ✓

**Placeholder scan:** No TBD/TODO; all code and commands are concrete. ✓

**Type consistency:** `SetPrevStateForTest(AppState)` defined in Step 1 and called in Step 2 with `migrate.AppStateMigrationFailed`; `TransitionToFailed`, `LastError`, `SetStateForTest`, `SetProbeIntervalForTest`, `StartDBProbe`, `NewMigrator`, `DetermineState`, `RunMigrations` all exist on `*Migrator` (verified against `migrator.go`). ✓
