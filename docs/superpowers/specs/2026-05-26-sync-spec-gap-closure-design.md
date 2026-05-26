# Sync Spec Gap Closure Design

**Date:** 2026-05-26
**Branch:** issue-608-normalise-external-games

## Overview

A spec-vs-code audit of the in-progress sync refactor against `docs/sync.md` found that the bulk of the refactor is spec-compliant. Three gaps remain:

1. **Stage 3** does not always set `user_game_platforms.external_game_id` on the conflict (UPDATE) path.
2. **Epic adapter** does not chunk its output into batches of ≤10 — it emits the full library in a single `onBatch` call.
3. **PSN adapter** has no inter-page delay; the spec requires a conservative request delay between pages.

This document specifies the fix for each. All three ship together in one PR. `docs/sync.md` is authoritative throughout — quotes below cite the spec verbatim.

No database migrations are required.

---

## G1 — Stage 3: always set `external_game_id` on `user_game_platforms`

### Spec requirement

> "Set `external_game_id` to the specific ExternalGame row that produced this platform entry." (Stage 3, step 4)
>
> "Additionally, a manually added row has no `external_game_id` because it was not produced by a sync. Stage 3 fills this in: `external_game_id` is always set (or updated) to the `external_games` row that produced the platform entry, so the association is linked to the storefront record from that point forward." (Stage 3 → Manually added games)

### Problem

In [`internal/worker/tasks/sync.go`](../../../internal/worker/tasks/sync.go) (`UserGameWorker.Work`, around lines 634–688), the per-platform handling has four sub-cases when a `user_game_platforms` row already exists for `(user_game_id, platform, storefront)`:

| Sub-case | Current behaviour | `external_game_id` written? |
|---|---|---|
| Ownership upgrade (`newRank > existingRank`) | `UPDATE … SET ownership_status, hours_played` | ❌ |
| Hours-only increase (no rank change, `incoming > existing`) | `UPDATE … SET hours_played` | ❌ |
| No-op (no rank change, hours not higher) | no UPDATE issued | ❌ |
| INSERT (no existing row) | `INSERT … external_game_id, …` | ✅ |

The no-op sub-case is the most acute: a manually-added row with `external_game_id = NULL` whose first sync brings the same ownership rank and equal-or-lower playtime is never backfilled and stays NULL forever. The other two UPDATE sub-cases also drop the link silently.

### Fix

Replace the three conditional UPDATE branches in the conflict (`default`) case with a single unconditional UPDATE that runs once per platform iteration. Compute the resolved `ownership_status` and `hours_played` in Go using the existing `ownershipRank` helper and a max-of-incoming-vs-existing comparison; always pass `eg.ID` as `external_game_id`. The INSERT branch is unchanged.

The `status_changed` `sync_changes` row is still written, unchanged, when `newRank > existingRank` — it must be inserted before the UPDATE so that `old_status` reflects the pre-UPDATE value.

Pseudocode for the conflict branch:

```go
// Resolve final values in Go.
// existingOwnership is *string and may be nil; preserve existing nil-safety.
var finalOwnership string
if existingOwnership != nil {
    finalOwnership = *existingOwnership
}
if newRank > existingRank {
    finalOwnership = ownership
    // write status_changed sync_change with old=existingOwnership, new=ownership
    //   (insert BEFORE the UPDATE below so old_status reflects the pre-UPDATE value)
}
finalHours := egp.HoursPlayed
if existingHours != nil && *existingHours > finalHours {
    finalHours = *existingHours
}

// Single UPDATE — always runs, always sets external_game_id.
UPDATE user_game_platforms
SET ownership_status = ?,
    hours_played    = ?,
    external_game_id = ?,
    updated_at = now()
WHERE id = ?
-- args: finalOwnership, finalHours, eg.ID, existingID
```

This makes the "always set" spec invariant structural rather than dependent on which branch runs.

### Testing

Add a Stage 3 test in `internal/worker/tasks/sync_test.go` that:

1. Pre-creates a `user_games` + `user_game_platforms` row with `external_game_id = NULL` (simulates a manually-added game), `ownership_status = 'owned'`, `hours_played = 50`.
2. Runs Stage 3 for an `external_game` representing the same game from a Steam sync with `ownership_status = 'owned'` and `hours_played = 30` (no rank upgrade, no hours increase — the no-op sub-case).
3. Asserts the `user_game_platforms` row now has `external_game_id = <eg.ID>`, `ownership_status = 'owned'` (unchanged), and `hours_played = 50` (unchanged).

Existing tests for the ownership-upgrade and hours-only paths should be reviewed after the refactor to ensure they still pass against the single-UPDATE shape.

---

## G2 — Epic adapter: chunk library into batches of ≤10

### Spec requirement

> "**Library fetch:** `legendary list --json`; DLC entries are filtered out (identified by a non-empty `MainGameAppName`); the adapter chunks the output into batches of ≤10." (Storefront Adapters → Epic Games Store)

### Problem

[`internal/services/epic/client.go`](../../../internal/services/epic/client.go) line 131 calls `onBatch(entries)` once with the full library. [`internal/services/epic/adapter.go`](../../../internal/services/epic/adapter.go) line 50 declares `batchSize` as `_ int` — the parameter is ignored. The dispatcher (`DispatchSyncWorker`) does per-game DB writes and per-game Stage 2 enqueues inside the `onBatch` callback, then continues to the next batch. Skipping chunking means:

- The Progress Box "count grows as games are fetched" UX (spec § "Progress Box") doesn't grow at all — the count jumps from 0 to N at end-of-fetch.
- All Stage 2 enqueues happen in one tight synchronous loop after Legendary returns, rather than interleaving with subsequent fetch batches.

### Fix

Chunking lives in the adapter — that is what the spec says, and it matches the location of chunking for Steam (also adapter-side). The Legendary client continues to emit a single slice (it has no native pagination — `legendary list --json` returns the whole library at once).

In [`internal/services/epic/adapter.go`](../../../internal/services/epic/adapter.go) `GetLibrary`, change the callback passed to `a.client.GetLibrary` to slice its input into chunks of `min(batchSize, 10)` and invoke the outer `onBatch` once per chunk. Use the passed-in `batchSize` (no longer `_`), defaulting to 10 if it is `<= 0` or `> 10`.

```go
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
    // … existing configured / restore-snapshot logic …

    chunkSize := batchSize
    if chunkSize <= 0 || chunkSize > 10 {
        chunkSize = 10
    }

    fetchErr := a.client.GetLibrary(ctx, a.userID, func(batch []ExternalGameEntry) error {
        for start := 0; start < len(batch); start += chunkSize {
            end := start + chunkSize
            if end > len(batch) {
                end = len(batch)
            }
            mapped := make([]storefrontadapter.ExternalGameEntry, 0, end-start)
            for _, e := range batch[start:end] {
                mapped = append(mapped, storefrontadapter.ExternalGameEntry{
                    ExternalID:      e.ExternalID,
                    Title:           e.Title,
                    PlaytimeHours:   0,
                    Platforms:       []string{"pc-windows"},
                    OwnershipStatus: e.OwnershipStatus,
                    IsSubscription:  false,
                })
            }
            if err := onBatch(mapped); err != nil {
                return err
            }
        }
        return nil
    })

    // … existing capture-snapshot and error-handling logic …
}
```

The client's behaviour does not change. Its existing test (single onBatch call with all entries) remains valid.

### Testing

Add an Epic adapter test in `internal/services/epic/adapter_test.go` (or extend an existing one) that:

1. Wires a fake `clientInterface` whose `GetLibrary` invokes `onBatch` once with 25 `ExternalGameEntry` values.
2. Calls `adapter.GetLibrary(ctx, 10, recorder)` where `recorder` appends each received slice to a list.
3. Asserts the recorder received exactly 3 slices, of sizes 10, 10, 5, in that order, with every entry mapped to `Platforms: ["pc-windows"]`.

---

## G3 — PSN adapter: inter-page request delay

### Spec requirement

> "**Rate limiting:** No published hard limit; the adapter applies a conservative request delay between pages." (Storefront Adapters → PSN)

### Problem

[`internal/services/psn/client.go`](../../../internal/services/psn/client.go) has no throttling between paginated fetches in `fetchPlayHistory` or `fetchPurchasedGames`. The spec requires a request delay; the current code makes pages as fast as the network allows.

### Fix

Add a `*rate.Limiter` field to the PSN client struct, initialized to `rate.NewLimiter(rate.Every(200*time.Millisecond), 1)` — 5 requests/second, matching Steam's adapter pattern in [`internal/services/steam/client.go`](../../../internal/services/steam/client.go) line 29. Call `limiter.Wait(ctx)` at the top of each iteration of the paginated fetch loops, before issuing the HTTP request.

Justification for matching Steam's cadence rather than a slower value: PSN has no published limit, and per-user page counts are in the single digits even for large libraries, so the absolute time difference between 200ms and a more conservative value (e.g. 500ms or 1s) is small. A 429 from PSN would surface as a fetch error and fail the job — not silent — so we'll notice and can tune down. Matching Steam's cadence keeps a single mental model across adapters.

Add a `NewClientForTests(httpClient, limiter, …)` constructor that accepts a custom limiter so tests can inject `rate.NewLimiter(rate.Inf, 1)` and run synchronously. This mirrors the Steam client's testing pattern.

### Testing

Two adjustments:

1. Existing PSN client tests should switch to `NewClientForTests` with an unlimited limiter so they continue to run instantly.
2. Add one new test that constructs a client with `rate.NewLimiter(rate.Every(50*time.Millisecond), 1)` (deliberately slowed), records `time.Now()` before and after a 3-page fetch using a fake HTTP server, and asserts the elapsed time is `>= 2 * 50ms` (allowing the first call to pass through immediately because the bucket starts full).

---

## Out of Scope

The following are intentionally not addressed by this design:

- Wider refactor of Stage 3 (e.g. extracting the per-platform logic into a separate function). The G1 fix is the smallest change that closes the spec gap; deeper restructuring belongs in its own PR.
- Wider refactor of the Epic client or PSN client beyond the changes named here.
- Adding 429 backoff handling to PSN. The spec only asks for an inter-page delay; backoff becomes relevant only if PSN starts returning 429s.
- Per-storefront tuning of the PSN delay value. 200ms matches Steam; revisit if PSN behaviour requires it.

## Acceptance Criteria

The branch satisfies this design when:

- `internal/worker/tasks/sync_test.go` includes the new G1 test described above and it passes.
- `internal/services/epic/adapter_test.go` includes the new G2 test described above and it passes.
- The PSN client test suite includes the new G3 timing test described above and it passes.
- A fresh run of `go test ./... && cd ui/frontend && npm run check && npm run knip && npm run test` is clean.
- A re-audit of the code against the three spec sections cited above shows MATCH for each.
