# Surface migration failures via app state (issue #583)

Date: 2026-05-21
Tracking issue: [#583](https://github.com/drzero42/nexorious/issues/583)
Parent: [#534](https://github.com/drzero42/nexorious/issues/534) (Sev 1)
Parent spec: [2026-05-21-issue-534-silent-errors-design.md](2026-05-21-issue-534-silent-errors-design.md)
Milestone: 0.1.0

## Background

`internal/migrate/handler.go:67-78` runs migrations in a goroutine that swallows both `RunMigrations` and `InitNeedsSetup` errors via `_ = err`. Two failure modes result:

1. `RunMigrations` fails internally and rolls itself back to `AppStateNeedsMigration` (`migrator.go:179,188,201,208`). The error is written to the SSE log channel, but `HandleProgress` then emits `event: complete` unconditionally (`handler.go:105`). The UI JS treats `complete` as success and redirects to `/`. Gate 2 sees `NeedsMigration` and bounces back to `/migrate`. The operator sees a brief log flash and then the migration prompt again, with no clear failure message.
2. `RunMigrations` succeeds but `InitNeedsSetup` fails. The error is fully silent. The handler still calls `TransitionToReady()`, so the app proceeds into `Ready` with whatever `needsSetup` value was last in memory — typically `false`, which lets a no-users DB through the setup gate.

Both modes leave the operator with no actionable signal. This spec adds a first-class `MigrationFailed` state so the failure is visible in the status payload and the migration UI.

## Goals

1. A migration failure deterministically transitions the app to a `MigrationFailed` state with the error message stored on the migrator.
2. `/api/migrate/status` returns the state and the error message; `/migrate` renders both and offers a Retry button.
3. The route gates continue to block app routes while permitting `/migrate*` and `/api/migrate*` in the failed state.
4. Retry from the failed state runs migrations again without restarting the process.

## Non-goals

- Fixes for Sev 2/3/4 sites enumerated in the parent spec — those are child issues B–D.
- Flipping `errcheck` on in CI — that is child issue E.
- New error types or restructuring of `recoverFromUnavailable`.

## Design

### State machine

Add `AppStateMigrationFailed` to the `AppState` enum in `internal/migrate/migrator.go`. Append it after `AppStateReady` so existing iota values stay stable (they are not serialized as ints anywhere). Extend `String()` with a `migration_failed` case.

The transitions become:

```
NeedsMigration → Migrating → Ready                            (success)
NeedsMigration → Migrating → MigrationFailed → Migrating → …  (retry)
```

`recoverFromUnavailable` is unchanged: it only special-cases `AppStateMigrating`; `MigrationFailed` falls into the default branch which re-runs `determineState`, which is correct (the DB is back, the migrator should re-evaluate from scratch).

### Error storage

Add to `Migrator`:

```go
lastError atomic.Value // stores string; empty when not in MigrationFailed
```

And the two access methods:

```go
func (mg *Migrator) TransitionToFailed(err error) {
    mg.lastError.Store(err.Error())
    mg.state.Store(int32(AppStateMigrationFailed))
}

func (mg *Migrator) LastError() string {
    v := mg.lastError.Load()
    if v == nil { return "" }
    return v.(string)
}
```

`RunMigrations` clears `lastError` (stores `""`) when it transitions to `Migrating`, so a successful retry does not leave the prior error visible in the status payload during the run.

### `RunMigrations` rollback paths

The four `mg.state.Store(int32(AppStateNeedsMigration))` calls inside `RunMigrations` (`migrator.go:179, 188, 201, 208`) become `mg.TransitionToFailed(wrappedErr)`. The function still returns the wrapped error so the handler can `slog.Error` it. The internal log-channel writes (`mg.sendLog(ch, "...failed: %v")`) stay — the SSE stream remains a useful live log.

### Handler goroutine

`HandleRun` becomes:

```go
go func() {
    ctx := context.Background()
    if err := h.migrator.RunMigrations(ctx); err != nil {
        slog.Error("migrate: run migrations failed", "err", err)
        // RunMigrations already called TransitionToFailed; nothing else to do.
        return
    }
    if h.db != nil {
        if err := h.migrator.InitNeedsSetup(ctx, h.db); err != nil {
            slog.Error("migrate: init needs-setup failed", "err", err)
            h.migrator.TransitionToFailed(err)
            return
        }
    }
    h.migrator.TransitionToReady()
}()
```

The switch at the top of `HandleRun` lists the two states that are allowed to start a run for clarity:

```go
switch h.migrator.State() {
case AppStateMigrating:
    return c.JSON(http.StatusConflict, …)
case AppStateReady:
    return c.JSON(http.StatusBadRequest, …)
}
// Allowed: NeedsMigration, MigrationFailed, DBUnavailable (gate 1 already redirects).
```

### Status payload

`HandleStatus` adds an `error` field when state is `MigrationFailed`:

```json
{ "pending_count": 0, "state": "migration_failed", "error": "migrate: acquire lock: <details>" }
```

The field is omitted in any other state. The error string is the wrapped `error.Error()` value — exposing it is acceptable because `/api/migrate/status` is already reachable only when the app is in a pre-Ready state and is used exclusively by the operator-facing migration page.

### Route gates

`internal/api/router.go:80` already reads `state != AppStateReady && state != AppStateDBUnavailable`, which transparently treats `MigrationFailed` like `Migrating`/`NeedsMigration` (blocks app routes, allows `/migrate*` and `/api/migrate*`). Gate 1 (`AppStateDBUnavailable`) and the health-check status mapping (`router.go:142`) also work correctly without change.

### UI changes — `ui/migrate/index.html`

Two JS changes:

1. **After SSE `event: complete`**, call `/api/migrate/status` and branch:
   - `state === 'ready'` → keep the existing redirect to `/`.
   - `state === 'migration_failed'` → render `data.error` in the status area with the error styling, and turn the button into "Retry".
   - Anything else (typically `migrating` if `InitNeedsSetup` is still running — see Risks) → leave the status text as "Running migrations…" and restart the 5s poll. The next poll tick will catch the eventual `ready` or `migration_failed` and act on it.
2. **Initial-load poll**: when the poll sees `state === 'migration_failed'`, render the error message and the Retry button immediately. This covers the case where the operator navigates to `/migrate` after a failed run from an earlier browser session.

The Retry button reuses `runMigrations()` (calling `POST /api/migrate/run` + opening the SSE stream). Reset the status area, hide the prior error, and clear the log before kicking off the new run.

No server change is required for retry — `HandleRun` falls through for `MigrationFailed` after the switch reorganization above.

## Testing

- **Migrator unit test** (no DB): construct a `Migrator`, call `TransitionToFailed(errors.New("boom"))`, assert `State() == AppStateMigrationFailed` and `LastError() == "boom"`. Then set state back to `Migrating` (via `SetStateForTest`) and `lastError` to `""` and confirm `LastError()` is empty.
- **Handler integration test** (test DB): set up DB → `DetermineState()` (initializes `bunMig`) → close the underlying `*sql.DB` via `db.DB.Close()` → call `HandleRun` over `httptest` → spin until `Migrator().State() == AppStateMigrationFailed` → GET `/api/migrate/status` and assert the JSON has `state: "migration_failed"` and a non-empty `error` field. Closing the connection makes `bunMig.Lock` fail naturally inside the goroutine; no test seams needed.

No UI test is added — the existing migrate page has no test harness and the JS change is a small branch on top of existing fetch/EventSource logic. Manual smoke-testing is captured in the implementation checklist.

## Risks

- **SSE/state race on the success path**: `RunMigrations` closes the SSE log channel before `InitNeedsSetup` runs, so the JS can receive `event: complete` while state is still `Migrating`. The UI mitigation above (resume the 5s poll on a non-terminal state) handles this without retries or sleeps — the poll picks up the eventual `Ready` or `MigrationFailed` and acts on it. The operator sees "Running migrations…" for at most one poll interval longer than today.
- **Test flakiness on goroutine completion**: the handler test polls for state change. Bound the poll loop with a short timeout (e.g., 2 seconds) and fail loudly rather than hang.
- **`atomic.Value` panic on type change**: `lastError.Store("")` followed by `Store(err.Error())` is safe because both are `string`. Document the invariant in a comment so a future refactor doesn't accidentally store an `error` value and panic.

## Out of scope

- Restructuring `recoverFromUnavailable` to also clear `lastError` on recovery: the next `RunMigrations` call clears it, and the failed state is only ever seen on `/migrate`, not on the recovered-app path.
- Improving the `slog.Error` format for migration failures beyond the `"migrate: <action> failed"` convention defined in the parent spec.
- Surfacing a structured error code in the status payload — the raw message is sufficient for the operator UI.
