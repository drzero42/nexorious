# PSN Library API v2 Design

**Date:** 2026-05-17
**Status:** Draft

## Problem

The current PSN sync uses `GetTrophyTitles` from the `go-psn-api` SDK as a proxy for the user's game library. This has three fundamental problems:

1. **Regional endpoint failure.** The SDK calls `https://{region}-tpy.np.community.playstation.net/...` — a legacy Sony endpoint with regional subdomains. Users outside the hardcoded `us` region get 503/DNS failures.
2. **Trophy proxy is wrong.** A game only appears if the user has earned at least one trophy. Games played without earning any trophy, and games never launched, are silently absent.
3. **PS5 games are missing entirely.** The SDK URL hardcodes `platform=PS3,PS4,PSVITA` — PS5 is not in the filter.

## Goal

Replace the trophy-title proxy with two purpose-built Sony endpoints that together give the most complete and accurate picture of the user's PS4/PS5 game library:

- **Play history endpoint** (`gamelist/v2`) — all PS4/PS5 games launched at least once, with playtime.
- **Purchased games endpoint** (GraphQL `getPurchasedGameList`) — all digitally purchased and PS Plus–redeemed titles, including games never launched.

Both endpoints are called on every sync. Their results are merged. Failures are surfaced individually. No PS3 support. No PlayStation PC (`pspc_game`) games.

The `PSNLibraryAdapter` interface and `DispatchSyncWorker` are unchanged. Only `psn.Client.GetLibrary` is replaced.

## Auth

Unchanged. The existing `AuthWithNPSSO` flow produces a Bearer access token. Both new endpoints accept `Authorization: Bearer <access_token>` — the same token the current code already obtains. No region configuration needed; both endpoints use `m.np.playstation.com` and `web.np.playstation.com`, which are global hosts.

## Design

### 1. Play history endpoint (`gamelist/v2`)

**Request:**
```
GET https://m.np.playstation.com/api/gamelist/v2/users/me/titles
    ?categories=ps4_game,ps5_native_game
    &limit=200
    &offset=<N>
Authorization: Bearer <access_token>
```

`limit=200` is the documented maximum. Paginate with `offset += 200` until `offset >= totalItemCount`.

**Response shape (relevant fields):**
```json
{
  "titles": [
    {
      "titleId": "PPSA07950_00",
      "name": "Call of Duty®",
      "category": "ps5_native_game",
      "service": "none(purchased)",
      "playDuration": "PT340H46M13S",
      "playCount": 392
    }
  ],
  "nextOffset": 200,
  "totalItemCount": 347
}
```

**Platform mapping:**

| `category` | `RawPlatform` |
|---|---|
| `ps4_game` | `playstation-4` |
| `ps5_native_game` | `playstation-5` |
| anything else | skip entry |

**Playtime:** `playDuration` is an ISO 8601 duration (e.g. `PT340H46M13S`, `PT0S`). Parse to integer hours, truncating (340h 46m → 340). A small dedicated parser handles the `PTxHxMxS` format; no third-party library needed.

**Ownership:** `service` field. `"none(purchased)"` → `OwnershipStatus: "owned"`, `IsSubscription: false`. Any value beginning with `"ps_plus"` → `OwnershipStatus: "subscription"`, `IsSubscription: true`.

**Error handling:** Any HTTP error or response-shape mismatch → return a non-nil error from `GetLibrary` (not `ErrInvalidNPSSOToken`). The worker will fail the sync job cleanly without marking the token expired.

### 2. Purchased games endpoint (GraphQL)

**Request:**
```
GET https://web.np.playstation.com/api/graphql/v1/op
    ?operationName=getPurchasedGameList
    &variables={"platform":["ps4","ps5"],"size":200,"start":0,"sortBy":"ACTIVE_DATE","sortDirection":"desc"}
    &extensions={"persistedQuery":{"version":1,"sha256Hash":"827a423f6a8ddca4107ac01395af2ec0eafd8396fc7fa204aaf9b7ed2eefa168"}}
Authorization: Bearer <access_token>
```

Note: `isActive` is omitted from variables so that lapsed PS Plus games remain visible. Paginate with `start += 200` until fewer than `size` games are returned. Max `size` per call: 200 (confirmed in practice up to 379 games in one call; use pagination defensively).

**Response shape (relevant fields):**
```json
{
  "data": {
    "purchasedTitlesRetrieve": {
      "games": [
        {
          "titleId": "CUSA10410_00",
          "name": "CODE VEIN",
          "platform": "PS4",
          "subscriptionService": "PS_PLUS",
          "isActive": true
        }
      ]
    }
  }
}
```

**Platform mapping:**

| `platform` | `RawPlatform` |
|---|---|
| `"PS4"` | `playstation-4` |
| `"PS5"` | `playstation-5` |
| anything else | skip entry |

**Playtime:** not available from this endpoint; always 0.

**Ownership:** `subscriptionService == "PS_PLUS"` → `IsSubscription: true`; otherwise `IsSubscription: false`.

**Error handling:**

- HTTP 4xx with a shape that looks like a GraphQL error (missing `data.purchasedTitlesRetrieve`) → the persisted query hash has changed. Return a named sentinel: `ErrPSNGraphQLSchemaChanged`. The worker maps this to a job failure message that tells the user exactly what broke: `"psn_graphql_schema_changed: the PSN purchases endpoint requires a code update"`.
- Any other HTTP error → return a generic non-`ErrInvalidNPSSOToken` error. Worker fails the sync cleanly.

### 3. Merge logic

`GetLibrary` fetches both endpoints, then merges into a single `map[string]ExternalLibraryEntry` keyed by `titleId`. Merge rules:

1. **Insert from `gamelist/v2`** — all entries, with playtime, platform, and ownership from that endpoint.
2. **Upsert from `getPurchasedGames`** — for each game:
   - If `titleId` already in map: update `IsSubscription` if the purchased endpoint marks it as PS Plus (trust the purchased endpoint for subscription status); keep playtime from `gamelist/v2`.
   - If `titleId` not in map: insert with `PlaytimeHours: 0`.

The result is the union of both sources. Disc games (in play history but not purchased list) are included from step 1.

After merging, call `onBatch` with slices of `batchSize` entries so the worker can dispatch job items progressively as before.

Both endpoints are **required**. If either fails, `GetLibrary` returns an error immediately. There is no partial-success fallback — the sync fails cleanly so the user sees an actionable message rather than a silently incomplete library.

### 4. New error sentinels in `internal/services/psn/`

```go
var ErrInvalidNPSSOToken     = errors.New("invalid npsso token")    // existing
var ErrPSNGraphQLSchemaChanged = errors.New("psn graphql schema changed") // new
```

`ErrPSNGraphQLSchemaChanged` is returned when the GraphQL response is missing the expected `data.purchasedTitlesRetrieve` field (indicating the persisted query hash no longer works). The worker already distinguishes `ErrInvalidNPSSOToken` from other errors; `ErrPSNGraphQLSchemaChanged` is handled in the same `else` branch with a specific job failure message.

### 5. Worker error handling

`DispatchSyncWorker.Work` already handles two error classes:

- `errors.Is(err, psnsvc.ErrInvalidNPSSOToken)` → mark token expired, `failSyncJob(..., "psn_token_expired")`
- Anything else → `failSyncJob(..., fmt.Sprintf("psn_fetch_error: %v", err))`

`ErrPSNGraphQLSchemaChanged` falls into the second class. No worker changes are needed beyond a test for the new error path.

### 6. `PSNLibraryAdapter` interface

Unchanged:

```go
type PSNLibraryAdapter interface {
    GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]psnsvc.ExternalLibraryEntry) error) error
}
```

The batch size constant `psnLibraryBatchSize = 10` in the worker is also unchanged — it controls how many merged entries are dispatched per `onBatch` call.

### 7. `ExternalLibraryEntry`

Unchanged:

```go
type ExternalLibraryEntry struct {
    ExternalID      string
    Title           string
    RawPlatform     string
    PlaytimeHours   int
    OwnershipStatus string
    IsSubscription  bool
}
```

### 8. Playtime propagation to `user_games`

The `ProcessSyncItemWorker` creates or updates a `user_games` row when a sync item completes. Currently `playtime_hours` is set from `source_metadata` at dispatch time. The dispatch code in `DispatchSyncWorker.Work` already writes `source_metadata` as a JSON object with `external_game_id` and `raw_platform`.

To propagate playtime, `source_metadata` must also carry `playtime_hours`. `ProcessSyncItemWorker` will read it and write it to `user_games.playtime_hours` when creating or updating the row. This requires:
- Dispatch (in `sync.go`): add `"playtime_hours"` key to the `meta` map.
- `ProcessSyncItemWorker`: read `playtime_hours` from metadata and write to `user_games`.
- Migration: `user_games.playtime_hours` column must exist. **Check whether this column already exists before including a migration in the plan.**

## Out of scope

- PS3 games — no longer supported; the legacy endpoint is broken.
- PlayStation PC (`pspc_game`) — PC ports are PC games; excluded from sync.
- Removing the `go-psn-api` SDK entirely — `AuthWithNPSSO` is still used for authentication; only `GetTrophyTitles` is replaced.
- UI changes — sync detail page is unchanged.
- Steam sync — unchanged.
- Refresh tokens / token renewal — out of scope; existing token expiry handling is unchanged.
