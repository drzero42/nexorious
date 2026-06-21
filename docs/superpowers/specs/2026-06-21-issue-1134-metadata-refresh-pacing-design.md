# Pace the metadata-refresh batch to the IGDB rate limit

- **Issue:** #1134
- **Date:** 2026-06-21
- **Status:** Approved (brainstorming)

## Problem

The daily `metadata_refresh_dispatch` job selects the **entire** `games` table
(`SELECT id, title FROM games ORDER BY last_updated ASC`) and enqueues one
`metadata_refresh_item` River job per game with no pacing. Every game's metadata
is re-pulled from IGDB every 24h.

These items run on the **single shared River queue** (`river.QueueDefault`,
`MaxWorkers = WORKER_COUNT`, default 4). For 10–20 minutes per night the refresh
items monopolize those 4 workers, starving user-initiated syncs/imports, which
then queue behind the refresh and contend for the same IGDB token bucket
(4 req/s, shared app-wide). Combined with `metadata_refresh_item`'s
`MaxAttempts: 5`, a single failure (IGDB API slowness, the 30s HTTP timeout, a
transient error) re-enqueues up to 5×, sustaining a retry storm. The result is a
recurring nightly burst of transaction-abort rollbacks that trips the
`NexoriousDBErrorsHigh` alert.

Measured at one homelab batch window: 1357 items / 15 min, 2726 IGDB requests /
15 min (≈3 req/s — note this is *under* the 4 req/s limiter, confirming the
failures are driven by worker starvation + retry amplification, not raw limiter
overflow), ~29% of Postgres transactions rolling back.

## Goal

A nightly refresh that:

1. Never produces a sustained burst of transaction rollbacks / item failures.
2. Keeps the IGDB request rate at/under the configured limiter without mass
   retry exhaustion.
3. Never degrades the user's ability to work with the app while it runs.
4. Still refreshes the whole library on a daily cadence (slow is fine — hours is
   acceptable), and scales sanely for larger libraries.

## Design

Two independent levers plus a safety valve. **No per-run count cap** — the whole
eligible library is still enqueued each run.

### 1. Dedicated low-concurrency River queue (the structural fix)

Move `metadata_refresh_item` jobs off the shared default queue onto their own
`metadata_refresh` queue with **`MaxWorkers` default 1** (configurable).

- **Why a separate queue:** refresh items can no longer occupy the default
  queue's workers, so user-initiated syncs/imports/exports always have their full
  worker pool available. This is what makes the failure burst *structurally*
  impossible regardless of how many items are queued.
- **Why `MaxWorkers = 1`:** the genuinely scarce shared resource is the IGDB
  token bucket (4 req/s), which the queue separation does *not* protect. A single
  serial refresh worker consumes only ~half the IGDB budget (~2 req/s, ~2 calls
  per item), permanently leaving ~2 req/s of headroom so a user's live IGDB
  search waits at most a fraction of a second. It also defangs the retry storm:
  serial execution means a failing item cannot fan out into concurrent retries.
- `MaxWorkers` is a new config knob so a big-library operator who would rather
  finish faster (at the cost of crowding user IGDB traffic) can raise it.

Implementation:

- New config field `MetadataRefreshWorkers int` (`env:"METADATA_REFRESH_WORKERS"`,
  default `1`).
- Register the queue in **both** River clients in `cmd/nexorious/serve.go`
  (the main client ~line 297 and the post-restore re-init ~line 411):
  ```go
  Queues: map[string]river.QueueConfig{
      river.QueueDefault:          {MaxWorkers: cfg.WorkerCount},
      tasks.QueueMetadataRefresh:  {MaxWorkers: cfg.MetadataRefreshWorkers},
  },
  ```
- Route item jobs to the queue via `MetadataRefreshItemArgs.InsertOpts()`
  returning `Queue: tasks.QueueMetadataRefresh` (alongside the existing
  `MaxAttempts: 5, Priority: 3`). The dispatch job itself stays on the default
  queue — it does no IGDB work and is a single quick insert pass.
- Export a `QueueMetadataRefresh = "metadata_refresh"` const from
  `internal/worker/tasks` so the queue name has one source of truth shared by the
  `InsertOpts` and the `serve.go` registration.

> **Guardrail:** a River job inserted to a queue that is not registered in the
> client's `Queues` map will sit unworked. The plan must register the queue in
> *both* client setup sites and confirm via test that an item enqueued to
> `metadata_refresh` is actually executed.

### 2. Staleness guard (the safety valve)

Change the dispatch selection from "all games" to "games not refreshed within the
configured window":

```sql
SELECT id, title FROM games
WHERE last_updated < now() - $interval
ORDER BY last_updated ASC
```

`games.last_updated` is `DEFAULT now() NOT NULL`, so it is never NULL: a game
that has never had its metadata refreshed simply carries its creation timestamp
and ages into eligibility on its own. There is no `IS NULL` branch.

- New config field `MetadataRefreshMinAge string` (Go duration,
  `env:"METADATA_REFRESH_MIN_AGE"`, default `"23h"`), documented as "set slightly
  below `METADATA_REFRESH_INTERVAL`."
- **Why 23h (1h below the 24h cadence):** the guard is evaluated once at dispatch
  time. A game refreshed early/mid in the previous run is ~24h old at the next
  dispatch and a 23h guard keeps it eligible despite scheduler jitter; a game
  refreshed late in a long-running batch is only ~21–23h old and simply slips to
  the next cycle (~48h), which is fine for metadata that changes a few times a
  year.
- **Why this does not change the everyday behavior:** with a 24h cadence
  everything is >23h stale by the next run, so in the normal case this still
  refreshes the whole library daily. Its job is purely to prevent a re-dispatch
  or overlapping run from pointlessly re-pulling the entire library and doubling
  IGDB load.
- A game that has never had its metadata refreshed carries its creation
  timestamp in `last_updated` (the column is `NOT NULL`), so it becomes eligible
  once that timestamp is older than the window — no special-casing required.
- Parse the duration at dispatch time; on parse failure, fall back to a safe
  default (mirror the existing `MetadataRefreshInterval` fallback handling in
  `scheduler.go`) rather than refreshing nothing or everything unguarded.

### 3. Retries — unchanged

`MetadataRefreshItemArgs` keeps `MaxAttempts: 5`. With serial (`MaxWorkers = 1`)
execution the retry-storm amplification that drove the burst is gone, so no
change is warranted here.

## Out of scope

- **Per-run count cap** — rejected. It would force multi-day refresh cycles, and
  the user wants the whole library refreshed daily.
- **TTL/staleness as the primary lever for cohort de-synchronization** — a pure
  TTL re-synchronizes an import cohort (all games imported together cross the TTL
  boundary on the same night). The cap-with-re-stamp approach that would solve
  that was rejected with the cap; the staleness guard here is only a re-work
  safety valve, not a cohort spreader.
- **IGDB-limiter priority / refresh-yields-to-user-traffic** — `MaxWorkers = 1`
  already leaves ~half the budget free, which is sufficient. Explicit priority is
  unnecessary complexity.
- **Very large libraries (100k+ games)** — "everything every day" is O(library)
  against a fixed 4 req/s ceiling and cannot complete daily beyond ~20–30k games.
  No real user is expected to hit this; not designed for.

## Acceptance criteria

- A normal metadata-refresh run no longer produces a sustained burst of
  transaction rollbacks / item failures attributable to IGDB rate limiting.
- IGDB request rate during refresh stays at/under the configured limiter without
  mass retry exhaustion.
- User-initiated IGDB operations (search, sync, manual fetch) remain responsive
  while a refresh runs.
- Refresh items execute on the `metadata_refresh` queue at the configured
  concurrency (verified by test).
- Games refreshed within `METADATA_REFRESH_MIN_AGE` are excluded from the next
  dispatch; games whose `last_updated` is older than the window (including
  never-metadata-refreshed games still at their creation timestamp) are included.

## Testing

- **Dispatch selection** (`internal/worker/tasks`, real DB): seed games with
  `last_updated` spanning the boundary (newer than the min-age, just older, and
  very old); assert only the eligible set gets `job_items` rows, ordered oldest
  first.
- **Queue routing**: assert `MetadataRefreshItemArgs.InsertOpts().Queue ==
  QueueMetadataRefresh`, and that the queue is registered in the River client so
  an enqueued item is actually worked (in-process River test).
- **Config defaults**: `MetadataRefreshWorkers` defaults to 1,
  `MetadataRefreshMinAge` defaults to `"23h"`; bad duration falls back safely.
- No new migration (no schema change).
```
