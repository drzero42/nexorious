# Maintenance start handlers: synchronous `jobs` row + real `job_id`

**Issue:** #890 (follow-up to #884 / #889)
**Date:** 2026-06-09
**Status:** Approved design

## Problem

The two admin "start refresh" POST handlers do not create the `jobs` row. They
enqueue a River *dispatch* job and return `200` with an empty (or absent)
`job_id`:

- `internal/api/games.go` — `HandleStartMetadataRefreshJob` returns `"job_id": ""`.
- `internal/api/games.go` — `HandleStartStoreLinkRefreshJob` returns no `job_id` at all.

The `jobs` row is INSERTed later, asynchronously, by the dispatch worker
(`metadata_refresh.go`, `store_link_refresh.go`). The client therefore has no
concrete id to pin the display to while the worker spins up. This caused the
race fixed in #884 and forced the frontend workaround in #889 (a bounded
eager-poll window in `useJobTypeStatus` plus dismissal bookkeeping in
`maintenance.tsx`).

## Goal

Have the start handlers synchronously create a minimal `jobs` row with a known
id, return that real `job_id`, and pass the id into the dispatch args so the
worker populates the row rather than creating it. Then remove the now-redundant
frontend timing workaround.

## Key constraint

River jobs must be inserted **after** the bun transaction commits:
`riverClient.Insert` uses a separate connection and commits immediately, so a
worker can dequeue and try to load the row before an uncommitted bun tx is
visible. The design honors this for free — the handler commits the `jobs` row,
*then* enqueues the dispatch River job.

## Both dispatch jobs are dual-path

Neither dispatch job is enqueued only by the handler:

- `MetadataRefreshDispatchArgs{}` is also enqueued by the **periodic
  scheduler** (`internal/scheduler/scheduler.go:305`,
  `river.PeriodicInterval(interval)`).
- `StoreLinkRefreshDispatchArgs{...}` is also enqueued by **sync completion**
  (`internal/worker/tasks/sync.go:999`), scoped to one `(user, storefront)`.

So both workers must support **two paths**:

1. **Handler-owned** (`JobID` set): the `jobs` row already exists; use it.
2. **Self-created** (`JobID` empty): create the row as today (periodic metadata
   refresh, sync-triggered store-link refresh).

## Design (Approach A: optional `JobID` in dispatch args)

### Dispatch args

Add an optional field to both args structs:

```go
type MetadataRefreshDispatchArgs struct {
    JobID string `json:"job_id,omitempty"`
}

type StoreLinkRefreshDispatchArgs struct {
    UserID     string `json:"user_id,omitempty"`
    Storefront string `json:"storefront,omitempty"`
    Force      bool   `json:"force,omitempty"`
    JobID      string `json:"job_id,omitempty"`
}
```

### Handlers (`internal/api/games.go`)

Both handlers gain the same shape:

1. In a bun transaction:
   - Run the active-job guard (`status IN ('pending','processing')` for that
     `job_type`; for store-link also `source = JobSourceSystem`, matching the
     admin run's source).
   - If an active job exists → capture its id, **do not insert**, **do not
     enqueue**. Return it with `200` (idempotent re-attach).
   - Otherwise INSERT a minimal row: a fresh UUID, `status='pending'`,
     `total_items=0`, `source` = `JobSourceSystem`, `user_id` = admin id (the
     requesting admin), `priority='low'`, `created_at = now()`.
2. Commit the tx.
3. If a new row was inserted, enqueue `DispatchArgs{JobID: id, ...}`
   (store-link: `Force: true`).
4. Return `{ "success": true, "message": ..., "job_id": id }`.

The store-link handler's response gains the `job_id` field it currently lacks.

> The metadata handler currently accepts a `game_ids` body param the worker
> ignores (it always refreshes all games). That pre-existing quirk is **out of
> scope**; leave the param accepted-and-ignored as today.

### Workers

Both `Work` methods branch on `args.JobID`:

**`JobID` set (handler-owned):**
- Skip the active-job guard entirely (the handler owns dedup).
- Select games / groups.
- In a tx: insert `job_items`, then `UPDATE jobs SET total_items=N,
  status='processing'` for the existing row (instead of INSERT).
- **Empty results** (0 games / 0 groups): finalize the existing row as
  `completed` with `total_items=0` via `finalizeJobCompleted`, in place of the
  `processing` flip. Never leave the pre-created row stuck in `pending`.
- Enqueue item River jobs after commit (unchanged).

**`JobID` empty (self-created — periodic / sync):**
- Behave **exactly as today**: guard, INSERT the row, insert `job_items`,
  enqueue items. No behavior change on these paths.

`reapStuckStoreLinkJobs` and the 2-minute `storeLinkReapMinAge` window are
unaffected: the handler's row is enqueued within the same request, far inside
the window.

## Frontend cleanup

The race the #889 workaround bridged disappears once the POST returns a real id.

- **`src/api/admin.ts`** — `startStoreLinkRefreshJob` gains `job_id` in its
  response type and returns `{ ..., jobId }`, mirroring `startMetadataRefreshJob`.
- **`src/routes/_authenticated/admin/maintenance.tsx`** — start handlers capture
  the returned `jobId` and pin the display to it (`startedJobId` state) instead
  of the dismissal-based resolution. `useJob(startedJobId)` resolves immediately
  because the row exists server-side at POST return.
  - Remove `eagerPoll` state, `eagerTimerRef`, the unmount cleanup effect, and
    `beginEagerPoll`.
  - Simplify display resolution: "show started/active job, else
    most-recent-completed." Keep `candidateDisplayJobId` /
    `mostRecentCompletedJobId` if still needed for the no-active-job fallback;
    prune `resolveDisplayJobId` + `dismissedJobId` bookkeeping if they become
    dead. `npm run knip` catches anything left unreferenced.
  - "Start New" stays as an affordance; it can pin to "nothing shown" rather
    than juggling the resurrection race.
- **`src/hooks/use-jobs.ts`** — remove the `eager` option from
  `useJobTypeStatus` (and its test).

User-visible behavior (card lifecycle) is preserved; only the timing workaround
is removed.

## Testing

**Backend:**
- Handler returns a real, non-empty `job_id`; the row exists immediately
  (`status='pending'`).
- A second start while a job is active returns the **same** id with `200` and
  does **not** insert a second row.
- Worker with `JobID` set populates that row and flips it to `processing`.
- Worker with `JobID` set and empty results finalizes the row `completed`
  (`total_items=0`), not stuck `pending`.
- Worker with `JobID` empty preserves today's guard-and-create path (periodic /
  sync).

**Frontend:**
- `maintenance.test.ts` / `use-jobs.test.ts` updated for the removed `eager`
  option and the pinned-id display.
- Regression: a freshly started job shows immediately; a completed job is not
  resurrected.

## Out of scope

- The ignored `game_ids` param on the metadata refresh endpoint.
- Any change to the periodic schedule or sync-triggered enrichment cadence.
