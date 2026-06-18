# Plan: Consolidate job-item enqueue dual-write (#1058)

## Context

Enqueuing a job item is an implicit dual-write: a `river_job` row (so River
processes it) and the `job_items` tracking row must stay in lockstep. If a
caller inserts one without keeping the other consistent, the item is left
permanently stuck in `pending` with no backing River job (CLAUDE.md → Known
Gotchas: "River queue is independent of `job_items`").

A canonical abstraction for this **already exists**: `tasks.EnqueueOrFail`
(`internal/worker/tasks/enqueue.go`). It inserts the `river_job` and, if the
insert fails (nil client or River error), marks the `job_item` `failed` with a
diagnostic message so the two never drift. It is already covered by
`enqueue_test.go` and already used by the main dispatch worker, the retry
handlers (`retryInsert`), the resolve handler, and several task workers.

The remaining work is **finishing the migration**: a handful of call sites still
call `riverClient.Insert(args{JobItemID})` directly and silently leave the item
stuck-`pending` on failure. Route them all through `EnqueueOrFail`.

Note: a true single-transaction write of both rows is not feasible — River uses
a `pgx` tx and the app's `bun.DB` uses `pgdriver`; they cannot share a tx. That
is why `EnqueueOrFail` uses compensation (fail the item if the River insert
fails) rather than a 2-table transaction. "Write together" is satisfied by this
lockstep-via-compensation contract.

## Decisions

- Keep the name `EnqueueOrFail` (descriptive, already tested) — no rename.
- Include the periodic orphan rescuer in the consolidation.

## Sites to convert (direct `rc.Insert({JobItemID})` → `EnqueueOrFail`)

1. `api/sync.go` ~1074 (unskip, `IGDBMatchArgs`) — on error keep the 500.
2. `api/sync.go` ~1377 (rematch, `UserGameArgs`) — on error keep the 500.
3. `api/sync.go` ~1434 (rematch sibling, `UserGameArgs`) — on error log + continue.
4. `api/sync.go` ~1479 (retry-failed, `IGDBMatchArgs`) — on error log + continue.
5. `api/import.go` ~229 (`ImportItemArgs`) — on error log.
6. `api/import.go` ~314 (`ImportMatchArgs`) — on error log.
7. `worker/tasks/sync.go` ~811 (sibling Stage 2, `IGDBMatchArgs`) — on error log.
8. `scheduler/orphaned_items.go` ~66 (rescuer) — use the returned error for `failureCount`.

## Deliberate behaviour shifts (both eliminate the stuck-`pending` class)

- The API handler `if riverClient != nil` guards are **removed** — `EnqueueOrFail`
  is called unconditionally, matching the dispatch worker (`tasks/sync.go:251`).
  In production the client is always wired (`cmd/nexorious/serve.go` →
  `router.go`; handlers only start in the `Ready` state), so the guard was
  pure test-scaffolding. On a genuine River insert failure the item is now
  marked `failed` (via `EnqueueOrFail`) instead of left stuck-`pending`.
- The rescuer marks an item `failed` on re-enqueue failure instead of leaving it
  for the next 30-min cycle.

## Tests

The api sync tests previously built the handler with a `nil` River client (via
`newSyncTestApp`), so they never exercised the enqueue at all. Removing the
production guard exposed this: those tests must now provide a client.

- `newSyncTestApp` (and the Epic/GOG variants) now inject `newTestRiverClient`
  (a real, non-started client whose `Insert` writes the `river_job` row; no
  worker runs it, so item statuses are unaffected). The 65 sync tests now
  actually cover the enqueue path. Import tests already used a real client.
- New `TestUnskipGame_RiverInsertFails_MarksItemFailed` uses `newFailingRiverClient`
  to lock the dual-write contract: a failed River insert leaves the job_item
  `failed` (not `pending`) and surfaces a 500. Mirrors the existing
  `TestHandleTriggerSync_RiverInsertFails_Returns500`.

## Out of scope

- Job-level dispatch inserts that carry `JobID` (not `JobItemID`):
  `DispatchSyncArgs`, `Export*Args`, `MetadataRefreshDispatchArgs`,
  `StoreLinkRefreshDispatchArgs`. These are not job-item dual-writes.
- `notify/emit.go` `NotifyArgs`, `tasks/sync.go` `MetadataFetchArgs{GameID}` —
  not job-item enqueues.
- Broader sync / jobs / job_items handler extraction (explicitly dropped in the
  issue: single in-process REST consumer; CLI/MCP drive over REST).

## Verification

- `go test ./internal/api/... ./internal/worker/tasks/... ./internal/scheduler/...`
- `make deadcode` (removes direct-insert code paths; confirm no new orphans).
- Grep check: no `riverClient.Insert(` / `rc.Insert(` / `RiverClient.Insert(`
  call with a `JobItemID` arg remains outside `EnqueueOrFail`.
