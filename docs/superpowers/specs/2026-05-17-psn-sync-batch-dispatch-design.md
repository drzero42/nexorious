# PSN Sync Batch Dispatch Design

**Date:** 2026-05-17  
**Status:** Approved

## Problem

When a PSN sync starts, `DispatchSyncWorker` fetches the entire trophy title library in a single blocking call before creating any `job_items`. During this window the job is in `processing` state but zero items exist, so the progress box on the sync detail page shows all-zero counts and `JobItemsDetails` renders nothing. The user cannot tell whether the sync is working or stuck.

Additionally, the current implementation caps the library at 100 titles (`GetTrophyTitles(ctx, "me", 100, 0)`) — users with larger libraries silently get incomplete syncs.

## Goal

Items appear in the progress box as quickly as possible by switching from a single all-at-once fetch to incremental batch dispatch: fetch a page → create and dispatch those job_items → fetch next page → repeat.

No DB migration. No frontend changes. Steam sync is unchanged.

## Design

### 1. PSNLibraryAdapter interface

**File:** `internal/worker/tasks/sync.go`

Change the interface from returning a full slice to accepting a callback:

```go
type PSNLibraryAdapter interface {
    GetLibrary(
        ctx        context.Context,
        npssoToken string,
        batchSize  int,
        onBatch    func([]psnsvc.ExternalLibraryEntry) error,
    ) error
}
```

The worker passes a callback that is called for each page. If the callback returns an error the loop stops and `GetLibrary` surfaces that error.

Batch size constant in the worker:

```go
const psnLibraryBatchSize = 10
```

10 is chosen so the first items appear in the progress box after approximately one network round-trip (~0.5–1 s), not after the full library is fetched.

### 2. PSN client implementation

**File:** `internal/services/psn/client.go`

`Client.GetLibrary` is rewritten to:

1. Authenticate with PSN once via `AuthWithNPSSO` (token exchange is done a single time).
2. Loop with `offset = 0`:
   - Call `psnClient.GetTrophyTitles(ctx, "me", batchSize, offset)`.
   - Convert results to `[]ExternalLibraryEntry`.
   - Call `onBatch(entries)` — stop and return the error if non-nil.
   - If `len(entries) < batchSize`, no more pages; break.
   - Otherwise `offset += batchSize` and continue.
3. Return `nil` on success.

Token expiry during the loop (auth failure from `GetTrophyTitles`) is propagated as `ErrInvalidNPSSOToken`, same as today.

The `GetTrophyTitles` response includes a `totalItemCount` field. If the SDK exposes it, we can use it as a loop termination condition instead of `len < batchSize` — but `len < batchSize` is sufficient and requires no SDK change.

### 3. DispatchSyncWorker — PSN branch

**File:** `internal/worker/tasks/sync.go`

The PSN branch of `DispatchSyncWorker.Work` is restructured. The existing `entries []syncLibraryEntry` slice and its post-processing loops are replaced with an inline callback:

```
fetchedIDs := map[string]struct{}{}

err := w.PSN.GetLibrary(ctx, creds.NpssoToken, psnLibraryBatchSize,
    func(batch []psnsvc.ExternalLibraryEntry) error {
        // 1. Accumulate IDs and upsert external_games (same ON CONFLICT logic as today).
        batchExtIDs := []string{}
        for each entry in batch:
            fetchedIDs[entry.ExternalID] = {}
            batchExtIDs = append(batchExtIDs, entry.ExternalID)
            upsert external_games

        // 2. Re-query this batch to get DB state (is_skipped, id, etc.).
        //    Games the user previously marked skipped are excluded here.
        var toProcess []models.ExternalGame
        SELECT * FROM external_games
            WHERE user_id = ? AND storefront = ? AND is_available = true
              AND is_skipped = false AND external_id IN (batchExtIDs)

        // 3. Dispatch job_items for non-skipped games in this batch.
        for each eg in toProcess:
            INSERT job_item ON CONFLICT DO NOTHING
            RiverClient.Insert(ProcessSyncItemArgs{...})

        return nil
    },
)
if err != nil:
    handle token-expiry → mark creds expired + failSyncJob
    return nil
```

After the callback loop completes:
- Step 5 (mark removed games unavailable) runs with the fully-accumulated `fetchedIDs` map — unchanged.
- Step 7 (update `last_synced_at`) runs — unchanged.

The Steam branch is untouched.

### 4. Mock / test compatibility

The `PSNLibraryAdapter` interface is used in:

- `internal/worker/tasks/sync.go` — the interface definition (updated).
- Any test file in the `tasks` package that provides a mock implementation — must be updated to match the new callback signature.
- `internal/api/sync_test.go` — uses `PSNClient` (the HTTP-layer interface), **not** `PSNLibraryAdapter`. No change needed.

Test mocks for `PSNLibraryAdapter` should call `onBatch` once with all entries (simulating a single-page response) to keep test behaviour equivalent to today.

## Out of scope

- Phase messaging / heartbeat on the job record — not needed once items appear quickly.
- Steam batch dispatch — apply later if needed.
- Pagination of the unavailability check (step 5) — the existing in-memory approach is fine.
- PSN library pagination beyond trophy titles — the trophy title API remains the library proxy.
