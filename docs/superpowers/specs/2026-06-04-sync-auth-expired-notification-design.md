# Sync authentication-expired notification (#751)

## Problem

When a storefront sync token or connection expires, the user wants a push
notification telling them to go reconnect — distinct from a generic "sync
failed" alert.

Today, an expired token *is* already detected and surfaced in-app
(`user_sync_configs.credentials_error` is set, the Sync page shows a Reconnect
prompt), and a credential failure already emits a notification — but it is the
generic `sync.failed` event with the message `"credentials error"`, which is
not actionable and not separately subscribable.

This change adds a distinct, actionable, separately-subscribable event for
expired credentials.

## Scope

- Backend only. **No DB migration** — `user_sync_configs.credentials_error`
  already exists and already records the prior state needed for transition
  detection.
- The frontend notification settings UI renders the event registry dynamically
  (`GET /api/notifications/event-types`), so the new toggle appears with no
  frontend change.

## Non-goals

- Changing any in-app behavior. The sync job still fails, `credentials_error`
  is still set, and the Sync settings page still shows the Reconnect prompt
  exactly as today. "Silence" in this document refers **only** to whether an
  outbound push notification (Shoutrrr) is sent.
- Adding a periodic re-reminder while a storefront stays disconnected.

## Design

### Event semantics

| Decision | Choice |
|---|---|
| New event type | `sync.auth_expired`, user-scoped, category `Sync`, label `Storefront needs reconnect`, **default-on** (consistent with other failure events) |
| Interaction with `sync.failed` | On a credential failure, emit the new event **only if the user is subscribed to it**; otherwise fall back to emitting `sync.failed`. Never both. This avoids a silent gap for users who haven't opted into the new event. |
| Repeat behavior | **Once on transition** only. Emit when the storefront flips from healthy (`credentials_error = false`) to expired. While it stays expired, subsequent scheduled *and manual* syncs send no notification. |

Two accepted consequences:

1. A storefront stuck in the expired state goes fully silent on subsequent
   retries (scheduled or manual) until reconnected. The user still sees the
   error in the Sync UI; only the push notification is suppressed.
2. After a DB restore with the wrong encryption key, every configured
   storefront hits `ErrCredentials` and each fires one transition
   notification — a small burst (one per storefront). This is intended: each
   connection genuinely needs attention.

### Components

**1. Registry — `internal/notify/registry.go`**

- Add constant `TypeSyncAuthExpired = "sync.auth_expired"`.
- Add registry entry:
  `{TypeSyncAuthExpired, ScopeUser, "Sync", "Storefront needs reconnect", true}`.
  Placed adjacent to the other `sync.*` entries to keep UI ordering sensible.

**2. Payload — `internal/notify/payloads.go`**

```go
type SyncAuthExpiredPayload struct {
    Storefront string `json:"storefront"`
}
```

No raw error string — it is always "credentials error", which carries no
useful information for the user.

**3. Formatter — `internal/notify/formatters.go`**

Render an actionable message, e.g.:
- title: `"Steam needs reconnecting"`
- body: `"Your Steam connection has expired. Open Sync settings to reconnect."`

Storefront name is title-cased / mapped to its display label consistently with
how existing sync formatters render the storefront.

**4. Worker logic — `internal/worker/tasks/sync.go`**

The two credential-error sites (`DispatchSyncWorker.Work`, currently around the
adapter-build branch and the mid-fetch branch) each currently call
`failSyncJob` (which emits `sync.failed`) and set the flag. Refactor:

- Extract `markSyncJobFailed(ctx, db, jobID, msg)` containing the DB work
  currently in `failSyncJob` (mark job failed + cancel pending job_items).
- `failSyncJob` keeps calling `markSyncJobFailed` then emitting `sync.failed`,
  so all existing non-credential callers are unchanged.
- Add `handleCredentialError(ctx, db, p, priorErr bool)` called from both
  credential-error sites:
  1. `markSyncJobFailed(ctx, db, p.JobID, "credentials error")`.
  2. Set `user_sync_configs.credentials_error = true` (existing UPDATE).
  3. If `priorErr == false` (healthy → expired transition):
     - If the user is subscribed to `sync.auth_expired`
       (`SELECT 1 FROM notification_subscriptions WHERE user_id = ? AND event_type = ?`)
       → `notify.Emit` the `sync.auth_expired` event.
     - Else → `notify.Emit` the `sync.failed` event (fallback).
  4. If `priorErr == true` (already broken) → emit nothing.

`priorErr` is read from the `cfg` already loaded at the top of `Work`, before
the flag is written, so it reflects the pre-run state at both call sites.

Dedup key for the new event follows the existing convention:
`jobID + ":" + TypeSyncAuthExpired`.

### Recipient resolution

No change. Delivery still flows through `notify.Emit` → `NotifyWorker`, which
resolves recipients via `notification_subscriptions`. The subscription check
added in the worker is only to decide *which* event to emit (auth vs. fallback),
so an unsubscribed user is never left silent.

## Testing

Per repo policy this logic is non-obvious and notification-adjacent, so it
earns targeted tests in `internal/worker/tasks`:

- Transition (prior `credentials_error = false`), user subscribed to
  `sync.auth_expired` → a `sync.auth_expired` event row is written; no
  `sync.failed` row.
- Transition, user **not** subscribed to `sync.auth_expired` → a `sync.failed`
  event row is written (fallback); no `sync.auth_expired` row.
- Repeat (prior `credentials_error = true`) → no new event row of either type.
- Existing non-credential `failSyncJob` callers still emit `sync.failed`
  (regression guard for the refactor).

Assertions are made against the `events` table (the source of truth `Emit`
writes), which keeps the tests independent of channel delivery.

## Files touched

- `internal/notify/registry.go` — new constant + registry entry
- `internal/notify/payloads.go` — `SyncAuthExpiredPayload`
- `internal/notify/formatters.go` — formatter case
- `internal/worker/tasks/sync.go` — `markSyncJobFailed`, `handleCredentialError`,
  wire both credential-error sites
- `internal/worker/tasks/sync_test.go` (or sibling) — tests
