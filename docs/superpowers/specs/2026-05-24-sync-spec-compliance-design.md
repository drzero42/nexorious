# Sync Spec Compliance Design

**Date:** 2026-05-24
**Branch:** issue-608-normalise-external-games
**Related issues:** #608, #613

## Background

`docs/sync.md` is the source of truth for how the sync pipeline should work. After significant work on issues #608 and #613, a gap analysis revealed eight places where the code does not match the spec. This document describes the design for closing all eight gaps in one pass.

---

## Gap Summary

| ID | Category | Description |
|---|---|---|
| A | Bug | `HandleSkipItem` never calls `syncCheckJobCompletion` — job can get stuck in `processing` |
| B | Wrong path | `retryInsert` enqueues Stage 2 for resolved items; spec says Stage 3 directly |
| C | Wrong sequencing | `HandleResolveItem` updates `external_game.resolved_igdb_id` immediately; spec says Stage 3 does it |
| D | Missing feature | No `sync_changes` pruning worker with configurable retention period |
| E | Architecture | No unified `StorefrontAdapter` interface — four separate interfaces and a ~600-line switch in `DispatchSyncWorker` |
| F | Architecture | Steam does not use the batch callback pattern |
| G | Architecture | GOG batch size is 50, not ≤10; GOG emits one entry per platform instead of one entry per game |
| H | Missing feature | Epic excluded from scheduled sync; spec says all four storefronts support it |

---

## Section 1: Functional fixes (A, B, C, D, H)

### A — `HandleSkipItem` missing completion check

After marking the job_item `skipped` and `external_game.is_skipped = true`, call `tasks.SyncCheckJobCompletion(ctx, h.db, item.JobID)`. Export `syncCheckJobCompletion` as `SyncCheckJobCompletion` so the API layer can call it.

### B — Resolve enqueues Stage 3 directly

`retryInsert` maps `job_type=sync` → `IGDBMatchArgs` (Stage 2). This is correct for retrying failed items. Resolving a `pending_review` item must go directly to Stage 3.

In `HandleResolveItem`, replace the `retryInsert` call with:

```go
tasks.EnqueueOrFail(ctx, h.db, h.riverClient, itemID, tasks.UserGameArgs{JobItemID: itemID})
```

For sibling re-queuing, likewise enqueue `tasks.UserGameArgs` directly instead of going through `retryInsert`.

### C — Don't update `external_game` at resolve time for the primary item

Remove the `UPDATE external_games SET resolved_igdb_id = ?` call for the primary item from `HandleResolveItem`. Stage 3's existing propagation step (`if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil`) handles it when it runs.

For **siblings**: per the spec push mechanic, sibling `external_game.resolved_igdb_id` IS updated at resolve time so Stage 3 can proceed. The sibling external_game update in `HandleResolveItem` stays; only the primary item's immediate update is removed.

### D — `sync_changes` pruning worker

Add `SyncHistoryRetentionDays int` to `internal/config/config.go` with env var `SYNC_HISTORY_RETENTION_DAYS` and default `90`.

Add `CleanupSyncChangesWorker` to `internal/scheduler/scheduler.go`:

```sql
DELETE FROM sync_changes WHERE created_at < now() - (? || ' days')::interval
```

Register as a daily periodic job (e.g. `0 2 * * *`) in `BuildPeriodicJobs`.

### H — Epic scheduled sync

Remove the `if cfg.Storefront == "epic" { continue }` guard from `CheckPendingSyncsWorker`. Epic credential errors surface through the normal `failSyncJob` path like other storefronts.

---

## Section 2: Unified adapter interface (E)

Defined in `internal/worker/tasks/sync.go`, replacing the four separate adapter interfaces.

```go
// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
type ExternalGameEntry struct {
    ExternalID      string
    Title           string
    PlaytimeHours   float64  // 0 when storefront does not provide playtime
    Platforms       []string // storefront-specific names; resolved to slugs by the worker
    OwnershipStatus string   // "owned", "subscription", etc.
    IsSubscription  bool
}

// StorefrontAdapter is the interface every storefront adapter must satisfy.
type StorefrontAdapter interface {
    GetLibrary(ctx context.Context, batchSize int, onBatch func([]ExternalGameEntry) error) error
}

// ErrCredentials is returned by an adapter when credentials are invalid,
// expired, or cannot be decrypted. The worker marks the job failed on this error.
var ErrCredentials = errors.New("credentials error")
```

The `DispatchSyncWorker` struct drops its four separate adapter fields and gains one:

```go
type DispatchSyncWorker struct {
    river.WorkerDefaults[DispatchSyncArgs]
    DB          *bun.DB
    Encrypter   *crypto.Encrypter
    Adapter     func(storefront string, cfg models.UserSyncConfig) (StorefrontAdapter, error)
    RiverClient *river.Client[pgx.Tx]
}
```

`Adapter` is a factory function wired at startup in `cmd/nexorious`. It decrypts credentials, validates them, refreshes tokens where needed (GOG), and returns the concrete adapter or `ErrCredentials`. This keeps credential-handling knowledge out of `Work` while allowing the credential switch to remain in one place.

---

## Section 3: Adapter implementations (F, G)

Each service package implements `tasks.StorefrontAdapter`. Credential parameters are constructor arguments — no DB or encrypter dependencies inside the adapters.

### Steam (`services/steam`)

`*Client` gains `GetLibrary(ctx, batchSize, onBatch)`. It:
1. Calls existing `GetOwnedGames` to fetch the full library in one call
2. Processes games in batches of `batchSize`, calling `GetAppDetailsPlatforms` per game — rate limiting and backoff stay internal
3. Maps each game to `ExternalGameEntry` with `Platforms` set to whichever of `["windows", "mac", "linux"]` apply, and `PlaytimeHours` carrying the single total

`OwnedGame` is renamed to `ExternalGameEntry`.

Playtime assignment (the first platform row gets the hours, others get 0) happens in the worker when upserting `external_game_platforms`, not in the adapter.

### PSN (`services/psn`)

`*Client.GetLibrary` already uses the batch callback pattern. Changes:
- Rename internal entry type to `ExternalGameEntry`
- Change `RawPlatform string` → `Platforms []string` (single element per entry, since PSN creates one ExternalGame per title ID)
- Map to `tasks.ExternalGameEntry` before calling `onBatch`
- NPSSO token is a constructor argument

### GOG (`services/gog`)

`*Client.GetLibrary` renames the entry type to `ExternalGameEntry` and consolidates per-platform duplicates: `fetchPage` groups by product ID and emits a single `ExternalGameEntry` with all applicable platforms in `Platforms []string`. This eliminates the `dispatchedInBatch` deduplication workaround in the current DispatchSyncWorker.

The access token is a constructor argument. `ErrGOGAuthExpired` wraps `tasks.ErrCredentials`.

### Epic (`tasks` package — `EpicClientAdapter`)

`EpicClientAdapter` already uses the batch callback pattern. Changes:
- Rename `epicsvc.ExternalGameEntry` → `epicsvc.ExternalGameEntry`
- Map to `tasks.ExternalGameEntry{Platforms: []string{"pc-windows"}}`
- Decrypt failure wraps `tasks.ErrCredentials`

### `platformresolution` rename

`RawPlatformToSlug` is renamed to `PlatformToSlug` to remove "raw" from the API surface, consistent with the agreed naming.

---

## Section 4: DispatchSyncWorker redesign

`Work` becomes storefront-agnostic after adapter construction:

```go
func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
    // mark processing, load config — unchanged

    adapter, err := w.Adapter(p.Storefront, cfg)
    if errors.Is(err, ErrCredentials) {
        failSyncJob(ctx, w.DB, p.JobID, "credentials error")
        return nil
    }
    if err != nil {
        failSyncJob(ctx, w.DB, p.JobID, err.Error())
        return nil
    }

    fetchedIDs := make(map[string]struct{})

    if err := adapter.GetLibrary(ctx, 10, func(batch []ExternalGameEntry) error {
        for _, e := range batch {
            fetchedIDs[e.ExternalID] = struct{}{}
            platforms := resolvePlatforms(e.Platforms) // platformresolution.PlatformToSlug per entry
            egID, isSkipped := upsertExternalGame(ctx, e, p)
            upsertPlatforms(ctx, egID, platforms, e.PlaytimeHours)
            if !isSkipped {
                insertJobItem(ctx, egID, e, p)
            }
        }
        enqueueBatch(ctx, p.JobID) // enqueue Stage 2 for all 'pending' items in this batch
        return nil
    }); err != nil {
        if errors.Is(err, ErrCredentials) {
            failSyncJob(ctx, w.DB, p.JobID, "credentials error")
            return nil
        }
        failSyncJob(ctx, w.DB, p.JobID, err.Error())
        return nil
    }

    // availability sweep + sync_changes('removed') — unchanged
    // update last_synced_at — unchanged
    return nil
}
```

`enqueueBatch` queries `job_items WHERE job_id = ? AND status = 'pending'` and enqueues `IGDBMatchArgs` for each. This is the same deferred pattern Steam uses today, now applied uniformly to all storefronts. PSN, GOG, and Epic currently enqueue inline per-batch, which can race with `syncCheckJobCompletion`; the unified pattern closes that race.

---

## Section 5: Testing

**A — `HandleSkipItem` completion check:** Add a test that skips the last `pending_review` item on a job and asserts the job reaches a terminal state.

**B+C — Resolve path:** Update existing resolve tests to assert that a `UserGameArgs` River job is inserted (not `IGDBMatchArgs`), and that `external_game.resolved_igdb_id` is `NULL` after the handler returns.

**D — `CleanupSyncChangesWorker`:** Insert `sync_changes` rows at varying ages, run the worker, assert only rows older than the retention period are deleted.

**E–G — Adapter interface:** Update per-adapter tests to use the new `GetLibrary(ctx, batchSize, onBatch)` signature. Key cases:
- Batch size is respected (≤10 entries per callback invocation)
- GOG consolidates multi-platform games into a single `ExternalGameEntry`
- Steam assigns playtime to the first platform only (worker-side, not adapter-side)

**E–G — DispatchSyncWorker:** Existing `sync_test.go` fake adapters collapse into a single `StorefrontAdapter` interface fake. No new test cases needed beyond what the existing suite covers.

**H — Epic scheduled sync:** No new tests needed; one-line guard removal. Existing scheduler tests cover the happy path.

**`PlatformToSlug` rename:** Existing `platformresolution_test.go` updated for the new function name; no logic changes.
