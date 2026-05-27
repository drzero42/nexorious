# Issue #589 — Clear `lastError` in `recoverFromUnavailable` default branch

**Date:** 2026-05-27
**Issue:** [#589](https://github.com/drzero42/nexorious/issues/589) (label: bug, milestone 0.1.0)
**Type:** Latent-bug fix + regression test
**Scope:** ~2 LOC production + one unit test, all within `internal/migrate/`

## Problem

`internal/migrate/migrator.go` — `recoverFromUnavailable` handles DB recovery from
`AppStateDBUnavailable`. The `default:` arm covers every prior state except
`DBUnavailable` and `Migrating`, which includes `MigrationFailed`.

When recovery runs after a failed migration, the `default:` arm nils `bunMig` and
re-runs `determineState()`, correctly transitioning to `NeedsMigration` or `Ready`.
But `mg.lastError` is never cleared, so a stale error string lingers in memory after
a successful recovery.

This is invisible today because `HandleStatus` (`handler.go`) only includes the
`error` field in its response when state is `MigrationFailed`. The stale
`LastError()` value sits behind that state guard. It becomes a real bug the moment a
future feature reads `LastError()` outside the failed-state guard and trusts a value
that no longer reflects reality.

## Fix

### Production change

Add one line at the top of the `default:` arm in `recoverFromUnavailable`:

```go
default:
    mg.lastError.Store("") // clear any failure inherited via DBUnavailable
    if mg.bunMig != nil {
        mg.bunMig = nil
    }
    // ... existing code unchanged
```

**Why `default:` only — not the `Migrating` arm.** `RunMigrations` clears
`lastError` before flipping state to `Migrating` (`migrator.go:200`). Therefore a
`Migrating → DBUnavailable → recover` path can never carry a stale error into the
`Migrating` recovery arm. Only the `default:` arm can be reached with a non-empty
`lastError` (via `MigrationFailed`). Placing the clear in `default:` matches both the
issue's instruction and the actual invariant, and restores symmetry with
`RunMigrations`' happy-path clear.

**Observable behavior:** none for current callers. `HandleStatus` still gates the
`error` field on `AppStateMigrationFailed`, so no API response changes. The fix
removes a latent phantom-error footgun, nothing more.

### Test seam

Add one exported test helper to `migrator.go`, alongside the existing
`SetStateForTest` / `SetProbeIntervalForTest` / `NewMigratorForTest` seams:

```go
func (mg *Migrator) SetPrevStateForTest(s AppState) {
    mg.prevState.Store(int32(s))
}
```

`prevState` is otherwise set only inside the `StartDBProbe` goroutine when it detects
unavailability. Exposing a deterministic setter lets the regression test reach the
`default:` recovery arm without racing the bad-DB probe timing. This follows the
package's established test-seam convention.

### Regression test

New test `TestStartDBProbe_RecoveryFromFailed_ClearsLastError` in
`internal/migrate/recover_test.go` (external `migrate_test` package, matching the
existing probe tests):

1. `setupTestDB(t)`, then `DetermineState()` + `RunMigrations(ctx)` so the DB is in a
   known-good, fully-migrated state.
2. `m.TransitionToFailed(errors.New("boom"))` — sets `lastError = "boom"` and state
   `MigrationFailed`. Assert `m.LastError() == "boom"` as a precondition.
3. `m.SetPrevStateForTest(migrate.AppStateMigrationFailed)` and
   `m.SetStateForTest(migrate.AppStateDBUnavailable)` to stage the recovery entry
   point deterministically.
4. `m.SetProbeIntervalForTest(30 * time.Millisecond)` and start a single probe against
   the **good** DB. The probe sees `DBUnavailable`, the ping succeeds, and it calls
   `recoverFromUnavailable` with `prev = MigrationFailed` → the `default:` arm.
5. Poll (bounded deadline, ~500ms) until state leaves `DBUnavailable`.
6. Assert `m.LastError() == ""` (the regression assertion) and, as a sanity check,
   that state is `AppStateReady`.

This is deterministic: it uses the good DB only, so there is no bad-DB timing
dependency and no `t.Skip` fallback path. The regression assertion always runs.

## Files touched

| File | Change |
|---|---|
| `internal/migrate/migrator.go` | One-line `lastError.Store("")` in `default:` arm; add `SetPrevStateForTest` helper |
| `internal/migrate/recover_test.go` | Add `TestStartDBProbe_RecoveryFromFailed_ClearsLastError` |

## Out of scope

- No migrations, no frontend, no slumber routes, no API-surface changes.
- No change to `HandleStatus`' state-gated error reporting (the masking guard stays).
- No change to the `Migrating` recovery arm.

## Verification

- `go test ./internal/migrate/... -run TestStartDBProbe -v` (targeted, includes the new test).
- The new test fails before the production one-line fix and passes after — confirming
  it is a genuine regression test.
