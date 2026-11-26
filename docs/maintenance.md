# Maintenance

Nexorious runs a set of periodic maintenance workers on cron-style schedules,
registered in `scheduler.BuildPeriodicJobs()`. This document is a process
reference — what each task does and why — not an implementation guide.

> For log-based alerting on these maintenance and sync failures, see
> [observability.md](observability.md).

## Sync history pruning

Removes `changes` entries older than the configured retention period
(default: 90 days, configurable via `SYNC_HISTORY_RETENTION_DAYS`) to keep the
sync history table from growing unboundedly while preserving recent history for
the Sync History UI.

## Job pruning

Removes completed, failed, and cancelled jobs (and their associated items, via
cascade) after 30 days.

## Export cleanup

Removes export files from disk and clears their stored path 24 hours after the
export job completed. It also removes files in the exports directory that no Job
refers to and that are more than a day old: the partial file of an export that
was killed, or the file of a run whose Job had been cancelled meanwhile.

## Unreferenced game cleanup

Removes `games` catalogue entries that no longer have any user in their library.
This can occur when a user removes all copies of a game, or after a rematch that
leaves an old IGDB entry with no references.

## Session cleanup

Removes expired login sessions.

## Job recovery

Every 5 minutes, and once at startup before background work starts, applies the
recovery rule of the Job lifecycle ([job-lifecycle.md](job-lifecycle.md#recovery))
to every active Job: metadata refresh, store-link refresh, export, imports and
sync. It has no age thresholds: each `pending` item and each unfinished
dispatch records the River job that owns it, so recovery can tell work that is
still queued or running from work whose River job was lost (it is queued again)
or gave up after its last attempt (it is failed). At startup it first reclaims
the River jobs a stopped process left running, and deletes queued River jobs of
the periodic kinds older versions used for this (the orphaned-item rescue and
the stale-job cleanup). A periodic run that fails or re-queues something sends one
admin maintenance notification (`job_recovery`); startup sends none.

## Metadata refresh

Periodically fetches fresh IGDB metadata for every game in the catalogue,
ordered by staleness (least recently updated first). Covers description, cover
art, genres, release date, developer, publisher, rating, platform names, game
modes, themes, player perspectives, and HowLongToBeat times. The schedule is
configurable via `METADATA_REFRESH_INTERVAL` (default: 24 hours) and can also be
triggered manually by an admin. Two additional knobs tune the refresh behaviour:
`METADATA_REFRESH_WORKERS` (default: `1`) controls how many item jobs run in
parallel on the dedicated refresh queue — keeping it at 1 leaves roughly half the
IGDB rate budget for user-facing requests; and `METADATA_REFRESH_MIN_AGE` (default:
`23h`) sets the minimum age a game must reach before it is re-refreshed, set
slightly below the interval so a game refreshed yesterday is reliably eligible today.

The immediate per-game fetch (triggered at the end of sync Stage 3 — see
[docs/sync.md](sync.md)) is the complement: it handles newly added games so they
do not wait for the next scheduled window. Games whose immediate fetch exhausts
all retries are still picked up by this scheduled refresh.

## Scheduled backup

Checks every minute whether a user-configured backup schedule is due. When a
backup is due, runs it and applies the configured retention policy.

> The scheduled sync trigger (`CheckPendingSyncs`) is documented in
> [docs/sync.md](sync.md) under "Scheduled Sync" and is not duplicated here.
