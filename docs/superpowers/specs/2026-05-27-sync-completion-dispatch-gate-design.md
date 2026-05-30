# Sync completion dispatch gate (#642)

**Date:** 2026-05-27
**Issue:** [#642](https://github.com/drzero42/nexorious/issues/642) — *Sync job marked completed while batches are still being enqueued — orphans pending_review items and hides progress UI*
**Scope:** Prevention only. No recovery of already-orphaned jobs; no #643 dual-count unification.

## Problem

A storefront sync can mark its `jobs` row `completed` **while `DispatchSyncWorker` is still streaming and enqueuing later batches of the library**. Items enqueued after that point are processed against an already-terminal job, so their completion checks are no-ops and they are orphaned. Any `pending_review` items produced by those later batches never surface: the sync progress card and the nav "needs review" badge both key off the job being `pending`/`processing`, so they vanish even though the user has games waiting to review.

This violates the documented invariant: *"Never mark a job `completed` while `pending_review` items exist."*

### Root cause

`DispatchSyncWorker.Work` (`internal/worker/tasks/sync.go:139`) enqueues matching work **incrementally, one batch at a time, as the library streams in**. It never finalizes the job itself — the terminal transition is driven entirely by per-item completion checks.

Each `IGDBMatchWorker` / `UserGameWorker` calls `SyncCheckJobCompletion` (`sync.go:856`) when it finishes its item. That function finalizes the job as soon as it observes **0 active (`pending`/`processing`) and 0 `pending_review`** items.

There is **no sentinel separating "dispatch is still streaming batches" from "all enqueued work is done."** When a storefront is rate-limited, the library arrives in waves with multi-minute gaps. If one wave fully drains (all items terminal, none `pending_review`) **before the next batch has been created**, the completion check sees a transiently-empty active set and finalizes the job. Later waves — including their `pending_review` items — are then orphaned under a terminal job.

Both sync entry points reach this code path:
- Manual trigger — `internal/api/sync.go:394` → enqueues `DispatchSyncArgs`.
- Scheduled auto-sync — `internal/scheduler/scheduler.go:173` → enqueues `DispatchSyncArgs`.

The only sync-job path that does **not** go through `DispatchSyncWorker` is the single-item "unskip" path (`internal/api/sync.go:942`), which inserts its one item synchronously before any worker runs.

## Approach

Introduce an explicit **dispatch-completion sentinel** on the job: a `dispatch_complete` boolean. `DispatchSyncWorker` sets it `false` while it is streaming and `true` once the full library has been dispatched. `SyncCheckJobCompletion` refuses to finalize a job whose dispatch is not yet complete.

The column defaults to `TRUE`, so every other job type (import, metadata refresh, the single-item unskip sync) and every pre-existing row is unaffected — only `DispatchSyncWorker` ever toggles it.

### Alternatives considered

- **`total_items` count-match** — record the expected item count after dispatch and finalize only when the resolved count equals it. Reuses an existing column, but `total_items = 0` is ambiguous (empty library vs. not-yet-dispatched) and it forces an exact, drift-prone count across upserts, dedup, and skips. Rejected as fragile.
- **River introspection** — have the completion check query `river_job` for a live `dispatch_sync` job before finalizing. No schema change, but it couples completion logic to River's internal table and brittle `args->>'job_id'` JSON matching — the same fragile pattern the orphan-rescuer is already forced into. Rejected.

## Design

### 1. Migration (edit in place)

Per project direction, **edit the existing initial migration** rather than adding a new one.

In `internal/db/migrations/20260503000001_initial.up.sql`, add to the `CREATE TABLE jobs` block (after `auto_retry_done`):

```sql
dispatch_complete BOOLEAN NOT NULL DEFAULT TRUE,
```

- `DEFAULT TRUE` means existing rows and all non-dispatch job paths are unaffected; only `DispatchSyncWorker` sets it `false`.
- No down-migration change — `20260503000001_initial.down.sql` already drops the `jobs` table.

### 2. Model

`internal/db/models/jobs.go` — add to the `Job` struct:

```go
DispatchComplete bool `bun:"dispatch_complete,notnull" json:"-"`
```

Required so the existing `SELECT * FROM jobs` scans into `models.Job` (`internal/api/job_items.go:84`, `internal/api/sync.go:1077`, `internal/api/jobs.go:332/548/597/628`) do not break on the unmapped column. `json:"-"` keeps it internal — the jobs endpoints build their response maps by hand, so the public API contract is unchanged.

### 3. `DispatchSyncWorker.Work` (`internal/worker/tasks/sync.go`)

- **Start of work (step 1, "mark processing"):** also set `dispatch_complete = false`:

  ```sql
  UPDATE jobs SET status = 'processing', started_at = ?, dispatch_complete = false WHERE id = ?
  ```

- **End of work (after step 8 — stale-platform sweep, removed-game detection, `last_synced_at`):** set `dispatch_complete = true`, then call `SyncCheckJobCompletion(ctx, w.DB, p.JobID)` exactly once.

  The trailing completion check is essential: if every enqueued item already drained while dispatch was still running, no further item worker will fire, so dispatch must perform the finalizing check itself. This also finalizes an **empty library** (zero items enqueued), which currently never completes because no item worker ever calls the check.

- **Error paths unchanged:** `failSyncJob` still marks the job `failed` directly. The gate only ever blocks the *complete* transition, so a job that errors during dispatch (with `dispatch_complete` still `false`) is unaffected.

### 4. `SyncCheckJobCompletion` (`internal/worker/tasks/sync.go:856`)

Add `AND dispatch_complete = true` to the finalizing `UPDATE`:

```sql
UPDATE jobs SET status = ?, completed_at = ?
WHERE id = ? AND status IN ('pending','processing') AND dispatch_complete = true
```

- Folding the gate into the atomic `UPDATE` avoids any read-then-write race; the function's early `active`/`pending_review` count returns stay as-is.
- Update the doc comment to record the new invariant: *a sync job is never finalized while its dispatch is still streaming batches (`dispatch_complete = false`).*

## Testing

Go integration tests on the shared `testDB`, asserting the invariant directly against `SyncCheckJobCompletion`:

1. **#642 regression guard:** `dispatch_complete = false`, all items terminal (`completed`/`skipped`/`failed`), none `pending_review` → job stays `processing` (not finalized).
2. **Happy path:** `dispatch_complete = true`, active = 0, `pending_review` = 0 → job finalizes to `completed`.
3. **Review still blocks:** `pending_review > 0` → job stays `processing` regardless of the flag (existing behavior preserved).

## Out of scope

- **Recovery of already-orphaned jobs** — per direction, this change is prevention-only; jobs already wrongly finalized with outstanding `pending_review` items are not repaired.
- **#643 dual-count unification** — re-opening is not part of this fix; the nav badge / detail-page count divergence remains its own follow-up. (With prevention in place, the divergence no longer arises from this defect going forward.)
