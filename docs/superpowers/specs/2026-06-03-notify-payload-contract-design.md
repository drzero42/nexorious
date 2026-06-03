# Notify payload contract: handled decode errors + typed payloads

**Date:** 2026-06-03
**Status:** Design approved, pending implementation
**Closes:** follow-up to #790 (the `//nolint:errcheck` no-op decode sites)

## Problem

PR #790 collapsed eight `json.Unmarshal` error checks in
`internal/notify/formatters.go` into best-effort `_ = json.Unmarshal(...)` lines
carrying `//nolint:errcheck // best-effort decode; falls back to defaults on
error`. The justification was that these are no-ops with a safe fallback. They
are not no-ops:

1. **The error is a real signal, currently discarded.** Notification payloads
   are never untrusted input. They are produced internally by `notify.Emit`
   (`emit.go:46`), marshaled from a `map[string]any`, stored in the `events`
   table, then read back and decoded by `Format` in two callers
   (`worker.go:37` for delivery, `api/events.go:171` for the in-app list). A
   decode failure can therefore only mean **schema drift** (an emitter's payload
   keys/types don't match what `Format` expects — e.g. `"err"` vs `"error"`, or
   a count emitted as a string) or **corruption** of the stored JSON. Both are
   latent bugs worth surfacing; today they vanish silently.

2. **The fallback is not actually safe on partial decodes.** `json.Unmarshal`
   populates the fields it can and errors only on the one it can't, so a partial
   type mismatch renders *misleading* output — e.g. "Sync completed with errors"
   showing "**0** failed item(s)" — not a generic message.

3. **The emit→format contract is implicit and unenforced.** Emit sites pass
   `map[string]any` with stringly-typed keys; `Format` decodes into anonymous
   inline structs. Nothing ties the two together at compile time or in CI.

## Goals

1. **Observability first** — a decode failure is logged with enough context
   (event id, event type, the error) to diagnose drift, instead of being
   swallowed.
2. **Safe fallback** — on decode failure, render a type-appropriate message that
   omits the untrustworthy fields rather than fabricating zero-valued data.
3. **Catch drift before production** — make the emit↔format payload contract
   explicit, enforced at compile time (typed payloads) and in CI (a
   registry-driven round-trip test).

## Non-goals

- **Per-type emit helpers** (`EmitSyncFailed(ctx, db, actorID, payload)`, etc.).
  These would bind `Type`↔payload at compile time but add ~13 functions. The
  registry-driven test (Layer 3) covers the Type↔payload pairing for far less
  surface area. Possible future step; out of scope here.
- **No DB or migration changes.** The payload JSON on the wire is byte-identical
  before and after, so existing stored `events` rows continue to render.

## Design

Three layers, applied in order.

### Layer 1 — `Format` surfaces the decode error; fallback is safe

Change the signature from

```go
func Format(eventType string, payload json.RawMessage) (title, body string)
```

to

```go
func Format(eventType string, payload json.RawMessage) (title, body string, decodeErr error)
```

- Each decoding case keeps an explicit `if err := json.Unmarshal(...); err !=
  nil` check. On error it sets `decodeErr` and builds a **type-appropriate body
  that omits the untrusted fields** rather than using zero-valued struct fields:

  | Event type                     | Body on decode failure                                      |
  |--------------------------------|-------------------------------------------------------------|
  | `sync.completed_with_errors`   | "Your {storefront} sync finished with some failed item(s)." |
  | `sync.needs_review`            | "Your {storefront} sync has item(s) needing review."        |
  | `sync.failed`                  | prefix only: "Sync failed."                                 |
  | `sync.diff`                    | "Your game library changed." (no per-game list)             |
  | `import.failed` / `export.failed` / `admin.backup.failed` | prefix only via `failBody` (already omits the error when empty) |
  | `admin.maintenance.*`          | prefix only via `maintBody`                                 |

  Where a field can still be missing-but-valid (decode succeeds, value empty),
  the existing `fallback(...)` graceful defaults stay — storefront-only cases are
  unaffected.

  Note: `{storefront}` above is only available when the decode failure is a
  *partial* one that populated `storefront` before erroring. When even
  `storefront` is unavailable it degrades through the existing `fallback(...,
  "library")` default, so the body is always well-formed.

- The top-level generic fallback (`"An event occurred: " + eventType`) stays for
  genuinely unknown types and empty results.

- The static-body cases that decode nothing (`import.completed`,
  `export.completed`, `admin.backup.completed`) return `decodeErr == nil`
  unconditionally.

- Both callers log the returned error with IDs they already hold, at **Warn**
  level (a live decode failure most plausibly means a historical event row
  written before a payload shape changed, not an active code bug — Layer 3 is
  the loud signal for the latter):

  ```go
  // worker.go
  title, body, derr := Format(ev.Type, ev.Payload)
  if derr != nil {
      slog.Warn("notify: payload decode failed", "event_id", ev.ID, "type", ev.Type, "err", derr)
  }
  ```
  ```go
  // api/events.go
  title, body, derr := notify.Format(r.Type, r.Payload)
  if derr != nil {
      slog.Warn("notify: payload decode failed", "event_id", r.ID, "type", r.Type, "err", derr)
  }
  ```

- All seven `//nolint:errcheck` annotations in `formatters.go` are deleted —
  five inline in the `Format` switch plus the two in `failBody`/`maintBody`. The
  error is now handled. (PR #790 described these as "8 no-op blocks" by counting
  decode call sites, since `failBody`/`maintBody` each serve multiple event
  types.)

### Layer 2 — typed payloads as the single source of truth

New file `internal/notify/payloads.go` defines one exported struct per event
shape. `DiffGame` moves here from `formatters.go`. Structs include
emitted-but-not-rendered fields (`JobID`, `BackupID`, `FilePath`) so the stored
contract is captured in full.

```go
type SyncCompletedPayload struct {
    Storefront string `json:"storefront"`
    JobID      string `json:"job_id"`
}
type SyncCompletedWithErrorsPayload struct {
    Storefront string `json:"storefront"`
    Failed     int    `json:"failed"`
    JobID      string `json:"job_id"`
}
type SyncFailedPayload struct {
    Storefront string `json:"storefront"`
    Error      string `json:"error"`
    JobID      string `json:"job_id"`
}
type SyncNeedsReviewPayload struct {
    Storefront string `json:"storefront"`
    Count      int    `json:"count"`
    JobID      string `json:"job_id"`
}
type SyncDiffPayload struct {
    Added   []DiffGame `json:"added"`
    Removed []DiffGame `json:"removed"`
    JobID   string     `json:"job_id"`
}
type ImportCompletedPayload struct {
    JobID string `json:"job_id"`
}
type ImportFailedPayload struct {
    JobID  string `json:"job_id"`
    Failed int    `json:"failed"`
    Error  string `json:"error"`
}
type ExportCompletedPayload struct {
    JobID    string `json:"job_id"`
    FilePath string `json:"file_path"`
}
type ExportFailedPayload struct {
    JobID string `json:"job_id"`
    Error string `json:"error"`
}
type BackupCompletedPayload struct {
    BackupID string `json:"backup_id"`
}
type BackupFailedPayload struct {
    Error string `json:"error"`
}
// Shared by admin.maintenance.completed and admin.maintenance.failed.
type MaintPayload struct {
    Action string `json:"action"`
    Error  string `json:"error,omitempty"`
    Count  int    `json:"count,omitempty"`
}
```

Changes that follow:

- `EmitParams.Payload` changes type `map[string]any` → `any`. Backward
  compatible: maps still satisfy `any`. `Emit` continues to `json.Marshal` it and
  keeps the existing nil→`{}` guard (a nil `any` and a nil map both fall through
  to `"{}"`).
- All emit sites switch from `map[string]any{...}` literals to the typed structs.
  Complete list (≈16 sites):

  | File:func                              | Type                          | Struct                           |
  |----------------------------------------|-------------------------------|----------------------------------|
  | `worker/tasks/sync.go` (sync failed)   | `sync.failed`                 | `SyncFailedPayload`              |
  | `worker/tasks/sync.go` (needs review)  | `sync.needs_review`           | `SyncNeedsReviewPayload`         |
  | `worker/tasks/sync.go` (with errors)   | `sync.completed_with_errors`  | `SyncCompletedWithErrorsPayload` |
  | `worker/tasks/sync.go` (completed)     | `sync.completed`              | `SyncCompletedPayload`           |
  | `worker/tasks/sync.go` (`emitSyncDiff`)| `sync.diff`                   | `SyncDiffPayload`                |
  | `worker/tasks/import_item.go` (failed) | `import.failed`               | `ImportFailedPayload`            |
  | `worker/tasks/import_item.go` (done)   | `import.completed`            | `ImportCompletedPayload`         |
  | `worker/tasks/export.go` (failed)      | `export.failed`               | `ExportFailedPayload`            |
  | `worker/tasks/export.go` (completed)   | `export.completed`            | `ExportCompletedPayload`         |
  | `scheduler/backup_poll.go` (failed)    | `admin.backup.failed`         | `BackupFailedPayload`            |
  | `scheduler/backup_poll.go` (completed) | `admin.backup.completed`      | `BackupCompletedPayload`         |
  | `worker/tasks/metadata_refresh.go` ×2  | `admin.maintenance.{failed,completed}` | `MaintPayload`          |
  | `notify/prune.go` ×2                   | `admin.maintenance.{failed,completed}` | `MaintPayload`          |
  | `scheduler/scheduler.go` (`emitMaint`) | `admin.maintenance.{failed,completed}` | `MaintPayload`          |

- `Format`'s anonymous inline structs are replaced by these named types, so both
  sides of the contract reference the same definition. A renamed field or changed
  type now moves emit and format together — drift cannot compile.

- `scheduler.emitMaint(ctx, db, failed bool, action string, extra map[string]any)`
  is narrowed. The only `extra` ever passed is a count, so the signature becomes
  `emitMaint(ctx, db, failed bool, p MaintPayload)` (or
  `emitMaint(ctx, db, failed bool, action string, count int)`), removing the
  open-ended map. Implementation picks whichever reads cleaner at the call sites.

### Layer 3 — registry-driven round-trip test

Add to `internal/notify/formatters_test.go` a test that locks the contract and
forces coverage:

```go
var samplePayloads = map[string]any{
    TypeSyncCompleted:           SyncCompletedPayload{Storefront: "Steam", JobID: "j1"},
    TypeSyncCompletedWithErrors: SyncCompletedWithErrorsPayload{Storefront: "Steam", Failed: 3, JobID: "j1"},
    // ... one per registered type
}

func TestFormat_AllRegisteredTypesRoundTrip(t *testing.T) {
    for _, et := range Registry() {
        t.Run(et.Type, func(t *testing.T) {
            sample, ok := samplePayloads[et.Type]
            if !ok {
                t.Fatalf("no sample payload for registered type %q — add one", et.Type)
            }
            raw, err := json.Marshal(sample)
            // require no marshal err
            title, body, derr := Format(et.Type, raw)
            // require derr == nil
            // require body != "An event occurred: " + et.Type  (not the generic fallback)
            // require title != "" && body != ""
            // assert exact rendered (title, body) against a golden expectation per type
        })
    }
}
```

This catches what compile-time typing cannot:

- A `Type` accidentally paired with the wrong payload struct at an emit site
  (types are strings; `Payload` is `any`, so the compiler can't bind them).
- A newly registered event type with no `Format` case or no sample payload — the
  test fails until the author wires both up.
- Accidental changes to the rendered copy — the golden assertions lock it.

A separate negative test asserts `Format` returns a non-nil `decodeErr` and the
safe (non-misleading) fallback body for a deliberately malformed payload on each
decoding type (e.g. `{"failed":"not-a-number"}` for
`sync.completed_with_errors` must not render "0").

## Files touched

- `internal/notify/formatters.go` — signature change, safe fallbacks, drop nolints, use named structs.
- `internal/notify/payloads.go` — **new**, typed payload structs + `DiffGame`.
- `internal/notify/emit.go` — `EmitParams.Payload` `map[string]any` → `any`.
- `internal/notify/worker.go` — log `decodeErr`.
- `internal/notify/prune.go` — typed `MaintPayload`.
- `internal/notify/formatters_test.go` — registry round-trip + malformed-payload tests.
- `internal/api/events.go` — log `decodeErr`.
- `internal/scheduler/scheduler.go` — narrow `emitMaint`; typed payload.
- `internal/scheduler/backup_poll.go` — typed payloads.
- `internal/worker/tasks/sync.go` — typed payloads (5 sites).
- `internal/worker/tasks/export.go` — typed payloads (2 sites).
- `internal/worker/tasks/import_item.go` — typed payloads (2 sites).
- `internal/worker/tasks/metadata_refresh.go` — typed `MaintPayload` (2 sites).

## Testing

- New `formatters_test.go` cases (Layer 3) are the primary gate: registry
  round-trip with golden render assertions + malformed-payload safe-fallback
  assertions.
- Existing `formatters_test.go` cases update for the 3-value return signature.
- Full `go test ./...` via the pre-push hook covers the emit-site refactors
  (their existing worker/scheduler tests must still pass with typed payloads).

## Risks & mitigations

- **Missed emit site** → leftover `map[string]any` still compiles (Payload is
  `any`) and still round-trips correctly, so behavior is unchanged; it just
  doesn't gain compile-time protection. The migration table above is the
  checklist; a `grep` for `Payload: map[string]any` in emit callers confirms
  completion.
- **Golden test brittleness** → assertions live next to the `Format` source;
  intentional copy changes update both in one edit. Acceptable: locking copy is a
  stated goal.
