# Steam Multi-Platform Sync — Design

**Date:** 2026-05-20
**Status:** Approved
**Issue:** #526

## Summary

Steam sync currently hardcodes `raw_platform = "pc-windows"` for every game. Steam's `IPlayerService/GetOwnedGames` endpoint does not include platform data, but the public store endpoint `store.steampowered.com/api/appdetails` does. This change calls `appdetails` per game to detect Windows/Mac/Linux availability, then emits one `ExternalLibraryEntry` per supported platform — mirroring the GOG pattern already implemented for `worksOn`. As a parity fix bundled into the same change, the GOG client also begins emitting Mac entries (it already parses `worksOn.Mac` but discards it), and `platformresolution` learns the `pc-mac → mac` mapping.

`external_games` already supports per-platform rows via the unique key `(user_id, storefront, external_id, raw_platform)`, so no schema change is required. `external_games` itself serves as the per-user cache: when rows already exist for a given (user, steam, appid), the `appdetails` call is skipped on subsequent syncs.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Platform detection source | `store.steampowered.com/api/appdetails?appids=<id>&filters=basics` | Only endpoint that exposes Windows/Mac/Linux availability; `filters=basics` keeps response small |
| Cache | Existing `external_games` rows, per-user | No new table; row existence for (user, appid) means platforms have already been detected for that user |
| Cache write policy | Only on successful detection | Failed API calls leave no trace; next sync retries naturally |
| Windows-only fallback trigger | Successful HTTP response with `platforms` absent or all-false | Real `appdetails` result that says "no specific platform info" — treat as Windows |
| API failure handling | Skip the game for this sync; do not emit entries; add appid to `fetchedIDs` | Prevents permanent mis-labelling from transient failures; preserves any rows from prior successful detection |
| Rate limit | 5 req/s, burst 1, via `golang.org/x/time/rate` on the `steam.Client` struct | Endpoint is undocumented; matches the ~200 ms gap suggested in the issue; same pattern as IGDB client |
| Steam client API shape | Two methods — `GetOwnedGames` returns `[]OwnedGame`; `GetAppDetailsPlatforms` per appid | Clean layering: client is dumb HTTP, worker orchestrates cache + fan-out |
| `item_key` for Steam job_items | `external_id + ":" + raw_platform` | Matches the GOG pattern at `sync.go:507`; required for uniqueness when a single appid emits multiple platform entries |
| GOG Mac emission | Bundled into the same change | GOG already parses `worksOn.Mac`; natural parity with adding `pc-mac` to `platformresolution` |
| Schema change | None | `external_games` already has `raw_platform` in its unique key |

## Architecture

### Data flow (Steam dispatch case)

```
DispatchSyncWorker (storefront = "steam"):
  1. owned, _ := client.GetOwnedGames(ctx, key, steamID)         // []OwnedGame
  2. existingPlatformsByAppID := SELECT external_id, raw_platform
                                  FROM external_games
                                  WHERE user_id = ? AND storefront = 'steam'
                                  GROUP BY external_id
                                  → map[appidStr][]string
  3. for each og := range owned:
       appidStr := strconv.Itoa(og.AppID)
       fetchedIDs[appidStr] = struct{}{}
       platforms := existingPlatformsByAppID[appidStr]
       if len(platforms) == 0 {
         pl, err := client.GetAppDetailsPlatforms(ctx, og.AppID)
         if err != nil {
           slog.Warn("appdetails failed", "appid", og.AppID, "err", err)
           continue  // no rows written; no job_item dispatched; retry next sync
         }
         platforms = []string{}
         if pl.Windows { platforms = append(platforms, "pc-windows") }
         if pl.Mac     { platforms = append(platforms, "pc-mac") }
         if pl.Linux   { platforms = append(platforms, "pc-linux") }
         if len(platforms) == 0 { platforms = []string{"pc-windows"} }  // explicit all-false / absent
       }
       for _, raw := range platforms {
         upsert external_games row with raw_platform = raw
       }
  4. (existing flow) SELECT toProcess, dispatch ProcessSyncItem jobs
     with item_key = eg.ExternalID + ":" + eg.RawPlatform
  5. (existing flow) Mark rows with external_id not in fetchedIDs as is_available = false
```

The cache lookup at step 2 is a single batched query, not N queries.

### Component changes

**`internal/services/steam/client.go`**

- `Client` struct gains a `limiter *rate.Limiter` field. `NewClient()` constructs it as `rate.NewLimiter(rate.Every(200*time.Millisecond), 1)`.
- New type:
  ```go
  type OwnedGame struct {
      AppID         int
      Title         string
      PlaytimeHours int
  }
  ```
- New type:
  ```go
  type Platforms struct {
      Windows bool
      Mac     bool
      Linux   bool
  }
  ```
- `GetOwnedGames(ctx, apiKey, steamID string) ([]OwnedGame, error)` — the previous return type `[]ExternalLibraryEntry` is removed from this method. The Steam-specific platform fan-out moves to the worker.
- `GetAppDetailsPlatforms(ctx context.Context, appID int) (Platforms, error)`:
  - `limiter.Wait(ctx)` at the top.
  - GET `https://store.steampowered.com/api/appdetails?appids=<appID>&filters=basics`.
  - Decode into `map[string]struct{ Success bool; Data struct{ Platforms Platforms } }`.
  - Return `Platforms{}, error` for non-200, `Success: false`, JSON decode error, or missing `<appID>` key.
  - Return parsed `Platforms{}` for `Success: true` with `Data.Platforms` zero-valued (caller decides fallback).
- `ExternalLibraryEntry` is removed from this package — it now lives only in the worker layer for Steam (consumed inline; no public type needed since the worker assembles the entries directly from `OwnedGame` + detected platforms).

**`internal/worker/tasks/sync.go`**

- `SteamLibraryAdapter` interface gains the new method and changes the existing one:
  ```go
  type SteamLibraryAdapter interface {
      GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]steamsvc.OwnedGame, error)
      GetAppDetailsPlatforms(ctx context.Context, appID int) (steamsvc.Platforms, error)
  }
  ```
- The Steam case in `Work()` is rewritten per the data flow above. The cache lookup is implemented as:
  ```go
  var rows []struct {
      ExternalID  string `bun:"external_id"`
      RawPlatform string `bun:"raw_platform"`
  }
  _ = w.DB.NewRaw(
      `SELECT external_id, raw_platform FROM external_games
       WHERE user_id = ? AND storefront = 'steam'`,
      p.UserID,
  ).Scan(ctx, &rows)
  existing := make(map[string][]string)
  for _, r := range rows {
      existing[r.ExternalID] = append(existing[r.ExternalID], r.RawPlatform)
  }
  ```
- Steam `item_key` changes from `eg.ExternalID` to `eg.ExternalID + ":" + eg.RawPlatform` (matching the GOG block at `sync.go:507`).
- The `rawPlatformByExtID` map used by the current Steam block is removed. The metadata blob takes `raw_platform` directly from `eg.RawPlatform` (the `ExternalGame` model field), again matching the GOG pattern. The `ON CONFLICT` clause on the upsert is unchanged.

**`internal/services/platformresolution/resolution.go`**

Add one case to `RawPlatformToSlug`:
```go
case "pc-mac":
    return "mac", true
```

The `mac` platform row and its `platform_storefronts` association with Steam already exist from the initial migration.

**`internal/services/gog/library.go`**

Add a third branch alongside the existing Windows and Linux blocks:
```go
if p.WorksOn.Mac {
    entries = append(entries, ExternalLibraryEntry{
        ExternalID:      id,
        Title:           p.Title,
        RawPlatform:     "pc-mac",
        PlaytimeHours:   0,
        OwnershipStatus: "owned",
        IsSubscription:  false,
    })
}
```

The doc comment on `ExternalLibraryEntry.RawPlatform` is updated from `"pc-windows" or "pc-linux"` to `"pc-windows", "pc-mac", or "pc-linux"`.

**`internal/api/router.go`** — no change required. The API-side `SteamClient` interface only declares `GetPlayerSummaries`; the worker takes a `*steamsvc.Client` directly (no adapter on that side). The `SteamLibraryAdapter` interface change is purely internal to `internal/worker/tasks` and the concrete client satisfies it after the method-signature edits.

## Failure handling

| Condition | Behaviour |
|-----------|-----------|
| `appdetails` returns 200 with `platforms.{windows,mac,linux}` having ≥1 true | Real detection; emit one row per `true` platform |
| `appdetails` returns 200 with `platforms` absent or all-false | Real detection meaning "no platform info"; emit single `pc-windows` row |
| `appdetails` returns 200 with `success: false` for this appid | Treated as failure; skip game this sync, log `slog.Warn` |
| `appdetails` returns non-200 (incl. 429, 5xx) | Failure; skip game this sync, log `slog.Warn` |
| Network error / decode error | Failure; skip game this sync, log `slog.Warn` |
| Context cancelled (sync job aborting) | `limiter.Wait` returns; the surrounding loop checks ctx and exits cleanly |

The `appid` is always added to `fetchedIDs` regardless of success, so a transient failure does not flip an existing successfully-detected row's `is_available` to false in the step that marks removed games unavailable. A brand-new appid whose first detection fails simply doesn't appear in the user's library until the next sync succeeds — preferred over permanently mis-labelling its platform.

## Testing

**`internal/services/steam/client_test.go`** (new file; none exists today)

- `GetOwnedGames` returns `OwnedGame` slice with appid/title/playtime parsed correctly from an `httptest.Server` fixture.
- `GetAppDetailsPlatforms` against `httptest.Server`:
  - Happy path with mixed platforms returns the expected `Platforms` struct.
  - `success: false` returns an error.
  - HTTP 429 returns an error (not silently treated as Windows-only).
  - HTTP 500 returns an error.
  - Decode failure / missing appid key in response returns an error.
  - All-false `platforms` returns a zero `Platforms{}` with no error (caller's job to apply Windows-only fallback).
- Rate limiter: tests construct the `Client` with `rate.NewLimiter(rate.Inf, 1)` to avoid 200 ms sleeps; mirrors the IGDB test pattern.

**`internal/worker/tasks/sync_test.go`** (extend existing)

- Extend `fakeSteamAdapter` to also implement `GetAppDetailsPlatforms(appID int) (Platforms, error)` returning a configurable per-appid map, and track which appids were queried for cache-hit assertions.
- New test: `GetAppDetailsPlatforms` reports `{Windows, Linux}` for appid 730 → expect two `external_games` rows (`pc-windows`, `pc-linux`) and two `job_items` rows with item_keys `730:pc-windows` and `730:pc-linux`.
- New test (cache hit): pre-seed `external_games` with an existing `(user, 'steam', '999', 'pc-linux')` row, then run sync. Assert `GetAppDetailsPlatforms` was NOT called for appid 999 and the existing row's `is_available` stays true.
- New test (api failure): `GetAppDetailsPlatforms` returns an error for appid X → no new `external_games` row for X, no `job_item` for X, but a pre-existing successfully-detected row for X remains `is_available = true`.
- New test (no-platforms fallback): `GetAppDetailsPlatforms` returns `Platforms{}` (no error) → exactly one `pc-windows` row + `pc-windows`-keyed job_item.

**`internal/services/gog/library_test.go`** (extend existing)

- Add a test case where the fixture's `worksOn.Mac` is true; assert an additional `pc-mac` entry is emitted alongside Windows/Linux.

**`internal/services/platformresolution`**

- No test file currently exists for this package; one is not added for a one-line switch case addition.

**Slumber collection**

- No new HTTP routes are introduced; `slumber.yaml` does not need changes.

## Out of scope / known limitations

- **Removed platforms aren't detected.** The existing `fetchedIDs` logic at `sync.go:547` is keyed by `external_id` only, not by `(external_id, raw_platform)`. If Steam drops Linux support from a game the user owns, the Linux row stays `is_available = true`. This affects GOG too and is not addressed here; out of scope.
- **No cross-user cache.** Each user's first Steam sync pays for `appdetails` calls for every appid they own (~3 min per 1000 games at 5 req/s). A future optimisation could add a global cache, but it is not part of this change.
- **No periodic re-detection.** Once an appid has rows in `external_games` for a user, those platforms are reused indefinitely. Acceptable because Steam rarely changes a game's platform support post-release; a manual "reset sync data" already exists if the user wants a clean re-fetch.
- **Mid-sync rate-limiter blocking.** A 1000-game first sync sequentially blocks on the limiter for roughly 3 minutes inside `DispatchSyncWorker.Work`. The job is already a background River job and this is consistent with the issue's stated expectation; no progress reporting is added.
