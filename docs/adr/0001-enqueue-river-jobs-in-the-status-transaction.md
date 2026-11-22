# River jobs are enqueued in the same transaction as the status change

A Job item that waits on a worker must always have a River job behind it. The server's River client runs on its own pgx pool, so an insert through it cannot join a Bun transaction: the status was written first and the River job inserted afterwards, and every failure between the two left an item `pending` with nothing to run it. That gap is what the orphan rescue, `EnqueueOrFail`, the store-link reaper and the stale-job sweep existed to repair.

We decided that the Job lifecycle module (`internal/joblifecycle`) inserts River jobs through a second, insert-only River client built on `riverdatabasesql` over Bun's `*sql.DB`, using `InsertTx` with the Bun transaction's `*sql.Tx`. The status change, the River insert, and the record of which River job owns the item commit or roll back together. The working client stays on `riverpgxv5`; both use the same `river_job` table.

## Considered options

- **Insert after commit and repair the gap** (the previous design). Rejected: every repair needs a heuristic (item age, job age), and each heuristic was a source of stuck or wrongly failed Jobs.
- **Move the whole server to one driver.** Not needed: River supports an insert-only client next to a working one.

## Consequences

- Code that changes a Job or item status never calls the River client directly. It asks the module for a transition.
- Cancelling River jobs for a Job also happens in the transaction (`JobCancelTx`), so a cancelled Job and its River jobs cannot disagree.
- Recovery needs no time thresholds: an item's recorded River job is either alive or it is not. See `docs/job-lifecycle.md`.
- River inserts made outside the lifecycle (notifications, periodic jobs, the immediate metadata fetch) are unaffected.
