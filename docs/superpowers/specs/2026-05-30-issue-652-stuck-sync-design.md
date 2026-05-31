# Stuck sync job recovery (issue #652)

Date: 2026-05-30
Tracking issue: [#652](https://github.com/drzero42/nexorious/issues/652)

## Background

A `dispatch_sync` River job killed mid-stream (process crash, OOM, eviction) leaves the corresponding `jobs` row permanently stuck in `processing`. Four weaknesses combine to make this a deadlock:

1. `MaxAttempts: 1` means River discards the job instead of retrying it.
2. `dispatch_complete` is set only at the very end of `Work` — if `Work` exits early the gate is never opened and `SyncCheckJobCompletion` becomes a no-op forever.
3. `CleanupStaleJobs` only targets `metadata_refresh`, so stuck sync jobs are never cleaned up.
4. `CheckPendingSyncs` skips any `(user, storefront)` with an active sync, so all future scheduled syncs for that storefront are silently blocked.

An instance of this was observed on 2026-05-28: job `fc6e7dab-1369-4adc-98a5-a343f4c36da4` stuck at 380/457 games with `dispatch_complete=false`, the River job in `discarded` state.

## Goals

1. Allow `dispatch_sync` to be retried automatically after a transient crash.
2. Rescue orphaned running River jobs at startup before new work begins.
3. Guarantee a clean failure state (not permanent `processing`) if all retries are exhausted.
4. Clear the existing stuck job without manual DB surgery.

## Non-goals

- Surfacing `dispatch_state` (River job state) in the API or UI — deferred.
- A high-frequency periodic River rescue job — River's built-in `RescueStuckJobsAfter` (1h) combined with `MaxAttempts: 3` is sufficient.
- A resumable cursor in dispatch — retries re-walk the full library, which is acceptable (idempotent upserts, extra minutes of work at most).
- Job-level progress heartbeating to detect application-level hangs.

## Design

### Piece 1 — `MaxAttempts: 3`

**File:** `internal/worker/tasks/sync.go`

Change `InsertOpts()` on `DispatchSyncArgs`:

```go
func (DispatchSyncArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{MaxAttempts: 3, Priority: 1}
}
```

This gives River the ability to retry transient failures and lets `JobRescuer` move stuck dispatches to `retryable` instead of `discarded`.

A retry is safe because the dispatch worker is already idempotent:
- `external_games`: `ON CONFLICT (user_id, storefront, external_id) DO UPDATE`
- `job_items`: `ON CONFLICT (job_id, item_key) DO NOTHING`
- Step 1 of `Work` resets `dispatch_complete = false` on entry — already false from the previous failed attempt, so this is a no-op.

### Piece 2 — Startup reconciliation

**File:** `cmd/nexorious/serve.go`

Before `riverClient.Start()` (inside the goroutine that waits for `AppStateReady`), call a `reconcileOrphanedDispatchJobs(ctx, db)` function. This runs once at startup and flips any orphaned `dispatch_sync` River jobs from `running` → `retryable` so River picks them up within seconds.

```go
func reconcileOrphanedDispatchJobs(ctx context.Context, db *bun.DB) {
    result, err := db.NewRaw(`
        UPDATE river_job
           SET state = 'retryable',
               scheduled_at = now(),
               errors = errors || jsonb_build_array(jsonb_build_object(
                 'at', now(),
                 'error', 'rescued at startup: client no longer heartbeating'
               ))
         WHERE kind = 'dispatch_sync'
           AND state = 'running'
           AND attempt < max_attempts
           AND NOT EXISTS (
             SELECT 1 FROM river_client rc
              WHERE rc.id = ANY(river_job.attempted_by)
                AND rc.updated_at > now() - interval '30 seconds'
           )`,
    ).Exec(ctx)
    if err != nil {
        slog.Error("startup: reconcile orphaned dispatch_sync failed", "err", err)
        return
    }
    rows, _ := result.RowsAffected() //nolint:errcheck // advisory
    if rows > 0 {
        slog.Info("startup: rescued orphaned dispatch_sync jobs", "count", rows)
    }
}
```

Key decisions:
- **`attempt < max_attempts`**: only rescue if retries remain. Jobs with exhausted attempts go to `discarded` naturally; CleanupStaleJobs handles the `jobs` row for those.
- **Heartbeat predicate** (`rc.updated_at > now() - 30s`): same liveness signal River uses internally — safe in multi-replica deployments.
- **`errors` append**: records why the job was rescued so the River error history is accurate.
- **Runs before `riverClient.Start()`**: no race with River's own rescue loop.
- **`context.Background()`**: avoids racing with any startup deadline context.
- **Non-fatal**: logs and returns on error; a failed reconciliation is not a reason to abort startup.

### Piece 3 — Extend `CleanupStaleJobs` to cover sync

**File:** `internal/scheduler/stale_jobs.go`

Add a second UPDATE statement in `CleanupStaleJobs`, run after the existing `metadata_refresh` UPDATE:

```sql
UPDATE jobs
   SET status = 'failed',
       error_message = 'stale_job_cleaned_up',
       completed_at = now()
 WHERE job_type = 'sync'
   AND status IN ('pending', 'processing')
   AND dispatch_complete = false
   AND created_at < now() - (? || ' seconds')::interval
   AND NOT EXISTS (
     SELECT 1 FROM job_items
      WHERE job_items.job_id = jobs.id
        AND job_items.status NOT IN ('completed', 'failed', 'skipped', 'cancelled')
   )
```

Key decisions:
- **`dispatch_complete = false`**: protects sync jobs where dispatch completed successfully. Even if items are still draining, the job is not touched.
- **`NOT EXISTS active job_items`**: mirrors the existing `metadata_refresh` guard. Ensures jobs with real item-side work still in flight are not cleaned up.
- **Same threshold**: reuses `STALE_JOB_THRESHOLD` (default 4h). Dispatch for any supported storefront completes in minutes; 4h is a conservative safety margin.
- **Same schedule**: CleanupStaleJobs already runs hourly (`0 * * * *`); no scheduler change.
- **Same `error_message`**: `stale_job_cleaned_up` requires no new frontend or monitoring handling.

### Piece 4 — One-off cleanup of the stuck job

No code change. Operational step at deploy time:

```
POST /api/jobs/fc6e7dab-1369-4adc-98a5-a343f4c36da4/cancel
```

This marks the `jobs` row `cancelled` and clears any queued River items. The next `CheckPendingSyncs` tick schedules a fresh sync for that storefront. The new `CleanupStaleJobs` extension would eventually catch it anyway, but cancelling immediately avoids waiting up to 4h.

## Files touched

| File | Change |
|---|---|
| `internal/worker/tasks/sync.go` | `MaxAttempts: 1` → `MaxAttempts: 3` |
| `cmd/nexorious/serve.go` | Add `reconcileOrphanedDispatchJobs` call before River start |
| `internal/scheduler/stale_jobs.go` | Second UPDATE for `sync` job type |

No migrations required. No frontend changes.

## Recovery flow after this change

```
Process killed mid-dispatch
  └─ River JobRescuer fires (≤1h) → dispatch_sync moved to retryable
       └─ River retries (up to 2 more times)
            ├─ Success → dispatch_complete=true → job completes normally
            └─ All retries exhausted → River discards
                 └─ CleanupStaleJobs fires (≤4h after threshold) → jobs row set to failed
                      └─ CheckPendingSyncs unblocked → fresh sync scheduled

Process restarted before River JobRescuer fires
  └─ reconcileOrphanedDispatchJobs at startup → dispatch_sync moved to retryable immediately
       └─ (same branch as above)
```
