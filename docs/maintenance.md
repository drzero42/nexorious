# Maintenance

Nexorious runs a set of periodic maintenance workers on cron-style schedules,
registered in `scheduler.BuildPeriodicJobs()`. This document is a process
reference — what each task does and why — not an implementation guide.

## Sync history pruning

Removes `sync_changes` entries older than the configured retention period
(default: 90 days, configurable via `SYNC_HISTORY_RETENTION_DAYS`) to keep the
sync history table from growing unboundedly while preserving recent history for
the Sync History UI.

## Job pruning

Removes completed, failed, and cancelled jobs (and their associated items, via
cascade) after 30 days.

## Export cleanup

Removes export files from disk and clears their stored path 24 hours after the
export job completed.

## Unreferenced game cleanup

Removes `games` catalogue entries that no longer have any user in their library.
This can occur when a user removes all copies of a game, or after a rematch that
leaves an old IGDB entry with no references.

## Session cleanup

Removes expired login sessions.

## Stale job cleanup

Detects `metadata_refresh` jobs stuck in an active state with no remaining
unfinished items (indicating a crash during dispatch) and marks them failed.
This releases the duplicate-run guard so the next scheduled dispatch can proceed.

## Orphaned item rescue

Detects `job_items` stuck in `pending` with no backing River job and re-enqueues
them. This is a safety net for items whose River job was lost due to a crash or
deployment. Only items older than one hour are considered, to avoid racing
freshly-created items.

## Metadata refresh

Periodically fetches fresh IGDB metadata for every game in the catalogue,
ordered by staleness (least recently updated first). Covers description, cover
art, genres, release date, developer, publisher, rating, platform names, game
modes, themes, player perspectives, and HowLongToBeat times. The schedule is
configurable via `METADATA_REFRESH_INTERVAL` (default: 24 hours) and can also be
triggered manually by an admin.

The immediate per-game fetch (triggered at the end of sync Stage 3 — see
[docs/sync.md](sync.md)) is the complement: it handles newly added games so they
do not wait for the next scheduled window. Games whose immediate fetch exhausts
all retries are still picked up by this scheduled refresh.

## Scheduled backup

Checks every minute whether a user-configured backup schedule is due. When a
backup is due, runs it and applies the configured retention policy.

> The scheduled sync trigger (`CheckPendingSyncs`) is documented in
> [docs/sync.md](sync.md) under "Scheduled Sync" and is not duplicated here.
