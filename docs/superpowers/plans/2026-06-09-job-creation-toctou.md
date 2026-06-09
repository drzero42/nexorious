# Plan: Close job-creation TOCTOU races (#891)

Follow-up to #890/#892. Issue #891 reports that the maintenance "start refresh"
handlers can create duplicate active jobs under concurrent POSTs: the guard
`SELECT ... WHERE status IN ('pending','processing')` and the `INSERT` race under
PostgreSQL's default READ COMMITTED isolation (two transactions both pass the
guard, neither seeing the other's uncommitted insert, and both insert).

## Verified scope

The same guard-SELECT-then-INSERT shape, unprotected by any DB constraint,
exists at four sites on the `jobs` table:

1. `internal/api/games.go` `startMaintenanceRefresh` — guard+insert in one tx
   (the reported race). Lock key `(job_type, system)`.
2. `internal/worker/tasks/maintenance_dispatch.go` `writeMaintenanceJobInTx`
   self-create path — *wider* window: the worker's guard SELECT
   (`metadata_refresh.go` / `store_link_refresh.go`) runs **outside** the insert
   tx entirely. Key `(job_type, source[, user_id])`.
3. `internal/api/sync.go` `HandleTriggerSync` — guard+insert with **no
   transaction at all** (two separate `db.NewRaw` calls). Key `(sync, storefront,
   user)`.
4. `internal/scheduler/scheduler.go` `CheckPendingSyncs` — same, no tx; races a
   concurrent manual trigger for the same user+storefront. Key `(sync,
   storefront, user)`. (Also silently treated a guard DB error as "no job" and
   inserted anyway — fixed.)

### Rejected fix: partial unique index

The issue floats `CREATE UNIQUE INDEX ... ON jobs (job_type, source) WHERE status
IN ('pending','processing')`. It does not work: the sync-completion path
(`sync.go`) enqueues per-`(user, storefront)` store-link dispatches, so two users
syncing the same storefront legitimately have two active `(store_link_refresh,
steam)` rows differing only by `user_id` — the index would reject the second.
Metadata wants `(job_type, source)`; store-link wants `(job_type, source,
user_id)`; no single index serves both. Use the advisory-lock alternative.

## Fix

Shared helper `tasks.AcquireJobDedupLock(ctx, tx, jobType, source, userID)` takes
a transaction-scoped `pg_advisory_xact_lock(hashtext(jobType|source|userID))`.
Held until the surrounding tx commits, it serializes any two transactions sharing
the dedup key; callers run their guard SELECT and INSERT in the same tx after it,
so the second caller's guard sees the first's committed row and skips.
`userID=""` for non-user-scoped (global) dedup, matching each caller's guard.

Applied at all four sites. The worker self-create path keeps its pre-tx guard as
a cheap early-out but now re-runs the guard inside the locked tx as the
authoritative check; `writeMaintenanceJobInTx` returns `skipped=true` when an
equivalent job is already active so the caller enqueues nothing.

No migration — advisory locks need no schema change.

## Out of scope (noted)

`sync.go` unskip path (~line 1044) inserts a 1-item sync job with **no** guard —
intentional fire-and-forget reprocessing, not a guard race. Left as-is.

## Tests

- `TestHandleStartMetadataRefreshJob` — concurrent starts create exactly one job.
- `TestSyncTrigger_ConcurrentCreatesOneJob` — 16 concurrent triggers → one job;
  existing `TestSyncTrigger_DuplicateReturns409` still green.
- `TestMetadataRefreshDispatch_ConcurrentSelfCreateCreatesOneJob` — concurrent
  self-create dispatches → one job (flaky-RED without the lock, deterministic
  green with it, confirmed 20/20).
- Scheduler race is scheduler-vs-handler across packages; covered by the shared
  lock mechanism + existing `TestCheckPendingSyncs_AlreadyRunning_NotDuplicated`.
