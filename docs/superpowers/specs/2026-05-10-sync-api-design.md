# Sync API Design Spec

## Overview

Phase 4 first task: the full sync configuration and trigger surface for Steam and PSN. Covers Bun models for `external_games` and `user_sync_configs`, the generic config/trigger/status endpoints, and the storefront-specific credential verification and disconnect endpoints.

**Epic is not implemented in the Go port.** The Python version uses the `legendary-gl` library directly (not a CLI shell-out), which has no Go equivalent. Epic sync is deferred to a later task. No Epic sync endpoints are registered (`POST /api/sync/epic`, `GET /api/sync/epic/status`, `POST /api/sync/epic/configure`, `DELETE /api/sync/epic/disconnect` all return 404). The epic storefront remains in the config list (`GET /api/sync/config` always returns all three supported storefronts), but triggering a sync does nothing. The frontend will hit dead endpoints for Epic-related actions, which is acceptable for now.

**GOG is not implemented and has no special handling.** It is not a valid `:storefront` value; any request using `gog` returns 400 like any other unknown storefront. GOG is not in the frontend.

**Storefront identifier vs. collection slug — two distinct namespaces:**
- **Sync-source identifiers** (`"steam"`, `"psn"`, `"epic"`) — used in `user_sync_configs.storefront` and `external_games.storefront`. These identify the sync source.
- **Storefront slugs** (`"steam"`, `"playstation-store"`, `"epic-games-store"`) — used in the `storefronts` table and `user_game_platforms.storefront`. These identify the collection entry's store.

For Steam the values happen to coincide (`"steam"` is both). For PSN they differ: `"psn"` is the sync-source identifier, `"playstation-store"` is the storefronts-table slug used on `UserGamePlatform`. The `platform_resolution.go` service is responsible for mapping sync-source identifiers to the correct collection slugs when creating `UserGamePlatform` rows.

---

## Schema Changes (edit existing migration, no new file)

All changes go in `internal/db/migrations/20260503000001_initial.up.sql` (and the `.down.sql`). There are no live users yet, so no data migration is needed.

**Renames in `user_sync_configs`:**

- Column `platform` → `storefront` (the field stores sync-source identifiers, not platform slugs)
- Column `platform_credentials` → `storefront_credentials`
- Drop the `-- JSON encrypted at rest (AES-GCM)` comment; credentials are stored as plain JSON text
- Update the unique constraint and indexes to reference `storefront`

The resulting table:

```sql
CREATE TABLE user_sync_configs (
    id                   TEXT PRIMARY KEY,
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront           TEXT NOT NULL,            -- 'steam', 'psn', 'epic'
    frequency            TEXT NOT NULL DEFAULT 'manual',  -- 'manual' | 'hourly' | 'daily' | 'weekly'
    auto_add             BOOLEAN NOT NULL DEFAULT false,
    storefront_credentials TEXT,                   -- JSON; shape is storefront-specific (see below)
    last_synced_at       TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront)
);
CREATE INDEX user_sync_configs_user_id_idx    ON user_sync_configs (user_id);
CREATE INDEX user_sync_configs_storefront_idx ON user_sync_configs (storefront);
```

**Changes in `external_games`:**

- Drop the `REFERENCES storefronts(name) ON DELETE CASCADE` FK on `storefront`. External games use the sync-source identifiers (`'steam'`, `'psn'`, `'epic'`) which are not rows in the `storefronts` table (storefronts uses slugs like `'playstation-store'`, `'epic-games-store'`). Make it a plain `TEXT NOT NULL` column.
- Change `playtime_hours` from `NUMERIC(10,2)` (nullable) to `INTEGER NOT NULL DEFAULT 0`.

The affected columns after change:

```sql
storefront       TEXT NOT NULL,                               -- 'steam', 'psn', 'epic'
playtime_hours   INTEGER NOT NULL DEFAULT 0,
```

---

## Bun Models

Add two new structs to `internal/db/models/models.go`.

**`ExternalGame`** — mirrors `external_games`:

```go
type ExternalGame struct {
    bun.BaseModel `bun:"table:external_games"`

    ID              string     `bun:"id,pk"                   json:"id"`
    UserID          string     `bun:"user_id,notnull"          json:"user_id"`
    Storefront      string     `bun:"storefront,notnull"       json:"storefront"`
    ExternalID      string     `bun:"external_id,notnull"      json:"external_id"`
    Title           string     `bun:"title,notnull"            json:"title"`
    ResolvedIGDBID  *int32     `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
    IsSkipped       bool       `bun:"is_skipped,notnull"       json:"is_skipped"`
    IsAvailable     bool       `bun:"is_available,notnull"     json:"is_available"`
    IsSubscription  bool       `bun:"is_subscription,notnull"  json:"is_subscription"`
    PlaytimeHours   int        `bun:"playtime_hours,notnull"   json:"playtime_hours"`
    OwnershipStatus *string    `bun:"ownership_status"         json:"ownership_status"`
    CreatedAt       time.Time  `bun:"created_at,notnull"       json:"created_at"`
    UpdatedAt       time.Time  `bun:"updated_at,notnull"       json:"updated_at"`
}
```

Unique constraint `(user_id, storefront, external_id)` is enforced at the DB level.

**`UserSyncConfig`** — mirrors `user_sync_configs`:

```go
type UserSyncConfig struct {
    bun.BaseModel `bun:"table:user_sync_configs"`

    ID                    string     `bun:"id,pk"                          json:"id"`
    UserID                string     `bun:"user_id,notnull"                json:"user_id"`
    Storefront            string     `bun:"storefront,notnull"             json:"storefront"`
    Frequency             string     `bun:"frequency,notnull"              json:"frequency"`
    AutoAdd               bool       `bun:"auto_add,notnull"               json:"auto_add"`
    StorefrontCredentials *string    `bun:"storefront_credentials"         json:"-"`
    LastSyncedAt          *time.Time `bun:"last_synced_at"                 json:"last_synced_at"`
    CreatedAt             time.Time  `bun:"created_at,notnull"             json:"created_at"`
    UpdatedAt             time.Time  `bun:"updated_at,notnull"             json:"updated_at"`
}
```

`StorefrontCredentials` is tagged `json:"-"` — it is never serialised directly into API responses.

---

## Storefront Credential Shapes

`storefront_credentials` holds a storefront-specific JSON object. The field is written only by the verify/configure endpoints and read only by sync workers.

**Steam:**
```json
{ "web_api_key": "ABC123...", "steam_id": "76561198...", "display_name": "Frostbyte" }
```

**PSN:**
```json
{ "npsso_token": "abc...64chars", "online_id": "MyPSNName", "account_id": "123456", "region": "GB", "is_verified": true, "token_expired_at": null }
```

`is_verified` is set to `true` at configure time. The PSN sync worker sets it to `false` and writes the current UTC timestamp to `token_expired_at` when PSN rejects the token. `token_expired_at` is `null` until the first expiry event.

**Epic:** deferred; row may exist with `storefront_credentials = NULL`.

`is_configured` in all API responses is derived from `storefront_credentials IS NOT NULL`.

---

## API Endpoints

All routes registered under `/api/sync` with JWT middleware in a new handler file `internal/api/sync.go`.

### Generic config

```
GET  /api/sync/config               → SyncConfigListResponse
GET  /api/sync/config/:storefront   → SyncConfigResponse
PUT  /api/sync/config/:storefront   → SyncConfigResponse
```

Valid `:storefront` values for config endpoints: `steam`, `psn`, `epic`. Anything else → 400. (Epic config rows can be created and updated, but there are no Epic sync endpoints to use them.)

`GET /api/sync/config` returns one entry per supported storefront. For storefronts with no DB row, a virtual default is returned (not persisted): `frequency: "manual"`, `auto_add: false`, `is_configured: false`, `id` set to a freshly generated UUID (not stored), `created_at`/`updated_at` set to `time.Now()`.

`PUT` body (all fields optional, partial update):
```json
{ "frequency": "manual|hourly|daily|weekly", "auto_add": true }
```
Upserts the row; creates it if missing. Conflict target is `(user_id, storefront)`.

**`SyncConfigListResponse`** — always a wrapped object, never a bare array (confirmed from Python):
```json
{
  "configs": [
    { "id": "uuid", "storefront": "steam", "frequency": "manual", "auto_add": false, "last_synced_at": null, "is_configured": false, "created_at": "2026-05-10T...", "updated_at": "2026-05-10T..." },
    { "id": "uuid", "storefront": "psn",   "frequency": "manual", "auto_add": false, "last_synced_at": null, "is_configured": false, "created_at": "2026-05-10T...", "updated_at": "2026-05-10T..." },
    { "id": "uuid", "storefront": "epic",  "frequency": "manual", "auto_add": false, "last_synced_at": null, "is_configured": false, "created_at": "2026-05-10T...", "updated_at": "2026-05-10T..." }
  ],
  "total": 3
}
```
`total` is always 3 (one per supported storefront).

**`SyncConfigResponse`:**
```json
{
  "id": "uuid",
  "user_id": "uuid",
  "storefront": "steam",
  "frequency": "manual",
  "auto_add": false,
  "last_synced_at": null,
  "is_configured": false,
  "created_at": "2026-05-10T...",
  "updated_at": "2026-05-10T..."
}
```

### Trigger + status

```
POST /api/sync/:storefront          → ManualSyncTriggerResponse
GET  /api/sync/:storefront/status   → SyncStatusResponse
```

Valid `:storefront` values for trigger and status: `steam`, `psn` only. `epic` and anything else → 400. (Epic endpoints are not registered.)

`POST /api/sync/:storefront` — creates a high-priority sync job. Sequence:

1. Query `jobs` WHERE `user_id = :user AND job_type = 'sync' AND source = :storefront AND status IN ('pending', 'processing')`. If found → 409.
2. Insert a `jobs` row (`status = 'pending'`, `priority = high`). This is the user-visible record; `job_id` in the response is `jobs.id`.
3. Insert a `pending_tasks` row with `payload = {"job_id": "<jobs.id>", "user_id": "...", "storefront": "steam"}` and wake the worker pool.

`GET /api/sync/:storefront/status` queries the same `jobs` table (not `pending_tasks`) for the active job. `active_job_id` is `jobs.id`. `last_synced_at` is read from `user_sync_configs.last_synced_at`.

**`ManualSyncTriggerResponse`:**
```json
{ "message": "Sync job created for steam", "job_id": "uuid", "storefront": "steam", "status": "queued" }
```

**`SyncStatusResponse`:**
```json
{ "storefront": "steam", "is_syncing": false, "last_synced_at": null, "active_job_id": null }
```

### Steam

```
POST   /api/sync/steam/verify       → SteamVerifyResponse
DELETE /api/sync/steam/connection   → 204
```

**`POST /api/sync/steam/verify`** — validates credentials and stores them on success.

Request:
```json
{ "steam_id": "76561198...", "web_api_key": "ABC123..." }
```

Validation order:
1. Steam ID format: 17 digits, starts with `7656119`. Fail fast, no network call.
2. API key format: 32 hex characters. Fail fast, no network call.
3. Call Steam `GetPlayerSummaries` — validates key + retrieves persona name.
4. Check `communityvisibilitystate == 3` (public profile required).

On success: upsert `user_sync_configs` row for `storefront = "steam"`, write credentials JSON, return:
```json
{ "valid": true, "steam_username": "Frostbyte", "error": null }
```

On any failure: always HTTP 200, `valid: false`:
```json
{ "valid": false, "steam_username": null, "error": "invalid_steam_id|invalid_api_key|private_profile|network_error|rate_limited" }
```

**`DELETE /api/sync/steam/connection`** — clears `storefront_credentials` on the steam row. Idempotent: returns 204 even if no row exists.

### PSN

```
POST   /api/sync/psn/configure      → PSNConfigureResponse
GET    /api/sync/psn/status         → PSNStatusResponse
DELETE /api/sync/psn/disconnect     → 204
```

**`POST /api/sync/psn/configure`** — validates NPSSO token and stores credentials.

Request:
```json
{ "npsso_token": "...exactly 64 chars..." }
```

Validation: reject immediately (400) if token length ≠ 64.

On valid token: exchange with PSN to retrieve `online_id`, `account_id`, `region`; upsert `user_sync_configs` row for `storefront = "psn"`; store credentials JSON; return:
```json
{ "success": true, "online_id": "MyPSNName", "account_id": "123456", "region": "GB", "message": "PSN configured successfully" }
```

On PSN rejection: 400 `{ "error": "invalid_npsso_token" }`.

**`GET /api/sync/psn/status`** — reads from the PSN credentials row:
```json
{ "is_configured": true, "online_id": "MyPSNName", "account_id": "123456", "region": "GB", "token_expired": false }
```
`token_expired` is derived as `is_verified == false AND token_expired_at != null` from the stored credentials JSON — this distinguishes "token expired" from "never configured". At configure time `is_verified` is set to `true` and `token_expired_at` to `null`. When the PSN sync worker receives a token rejection, it updates the credentials row: `is_verified = false`, `token_expired_at = <current UTC timestamp>`. Returns zeroed response if no row exists.

**`DELETE /api/sync/psn/disconnect`** — clears `storefront_credentials` on the psn row. 204, idempotent: returns 204 even if no row exists.

### Ignored (skip/un-skip)

```
GET    /api/sync/ignored       → IgnoredExternalGamesResponse
POST   /api/sync/ignored/:id   → 204
DELETE /api/sync/ignored/:id   → 204
```

Operates on `external_games.is_skipped`. When `is_skipped = true` the sync worker silently skips the row on subsequent syncs.

**`GET /api/sync/ignored`** — returns all `external_games` rows for the authenticated user where `is_skipped = true`.

**`POST /api/sync/ignored/:id`** — sets `is_skipped = true` on the external game. `:id` is `external_games.id`. Returns 404 if the row does not exist or belongs to a different user. Idempotent: returns 204 if the game is already skipped.

**`DELETE /api/sync/ignored/:id`** — sets `is_skipped = false`. Returns 404 if not found or wrong user. 204 on success.

**`IgnoredExternalGamesResponse`** — array of `ExternalGame` objects (all public fields, no credentials):
```json
[
  { "id": "uuid", "user_id": "uuid", "storefront": "steam", "external_id": "12345", "title": "Half-Life 2", "resolved_igdb_id": 232, "is_skipped": true, "is_available": true, "is_subscription": false, "playtime_hours": 14, "ownership_status": "owned", "created_at": "...", "updated_at": "..." }
]
```

### Route ordering

Register static-segment routes (`/steam/verify`, `/psn/configure`, `/psn/status`, `/steam/connection`, `/ignored`, `/ignored/:id`) before the `:storefront` parameterised routes. Echo v5 resolves static segments first but explicit ordering avoids surprises.

---

## Error Handling

| Situation | Response |
|---|---|
| Invalid `:storefront` param (config endpoints) | 400 |
| Invalid `:storefront` param (trigger/status — anything not `steam`/`psn`) | 400 |
| `epic` storefront on trigger/status | 400 (not registered) |
| Active sync job exists (trigger) | 409 |
| Steam verify: bad format | 200 `{ valid: false }` |
| Steam verify: API failure | 200 `{ valid: false, error: "network_error" }` |
| PSN configure: token wrong length | 400 |
| PSN configure: PSN rejects token | 400 |
| Disconnect with no row | 204 (idempotent) |
| Skip/un-skip: external game not found or wrong user | 404 |
| Skip: game already skipped | 204 (idempotent) |

---

## Implementation Notes

### Worker Task Algorithms

#### CheckPendingSyncsTask (scheduler, every 15 minutes)

Runs inline in the gocron goroutine (not via the worker pool). Algorithm derived from the Python `check_pending_syncs` task:

1. Query all `user_sync_configs` WHERE `frequency != 'manual'`.
2. For each config, evaluate `needs_sync`:
   - `last_synced_at IS NULL` → `true` (first auto-sync — trigger immediately)
   - `frequency = 'hourly'` → `elapsed >= 3600s`
   - `frequency = 'daily'` → `elapsed >= 86400s`
   - `frequency = 'weekly'` → `elapsed >= 604800s`
   - `frequency = 'manual'` → `false` (already filtered out in step 1)
3. If `needs_sync = true`, check for an existing `jobs` row WHERE `user_id = :uid AND job_type = 'sync' AND source = :storefront AND status IN ('pending', 'processing')`. If found, skip (don't create a duplicate).
4. If no active job, create a **low-priority** `jobs` row and submit a `DispatchSyncTask` to the worker pool. (Manual triggers use high priority; scheduled checks use low priority.)

Only `steam` and `psn` storefronts are dispatched. If a config row exists with `storefront = 'epic'`, skip it — there is no epic sync implementation.

#### DispatchSyncTask (worker pool task)

Called by both the manual trigger endpoint and the scheduler check. Same code path regardless of priority.

Payload: `{ "job_id": "...", "user_id": "...", "storefront": "steam"|"psn" }`

Phases:
1. **Mark job processing.** Set `jobs.status = 'processing'`, `jobs.started_at = now()`.
2. **Read credentials.** Load the `user_sync_configs` row for `(user_id, storefront)`. Parse `storefront_credentials`. If credentials are missing or `is_verified = false` (PSN), fail the job immediately with a descriptive error.
3. **Fetch library from adapter.** Call the storefront-specific adapter (Steam or PSN) to retrieve the current game library. Returns a list of `ExternalLibraryEntry` structs, each with: `external_id`, `title`, `raw_platform` (e.g. `"pc-windows"` for Steam; `"playstation-5"` or `"playstation-4"` for PSN — one entry per platform entitlement), `playtime_hours`, `ownership_status`, `is_subscription`.
4. **Upsert `ExternalGame` rows.** For each entry, upsert by `(user_id, storefront, external_id)`. Always update `title`, `playtime_hours`, `is_subscription`, `ownership_status`, `is_available = true`.
5. **Mark removed games.** Query all `ExternalGame` rows for `(user_id, storefront)` WHERE `is_available = true`. Any row whose `external_id` was not in the fetched set is set to `is_available = false`. If `is_subscription = true`, downgrade linked `UserGamePlatform.ownership_status` to `'no_longer_owned'`.
6. **Dispatch `ProcessSyncItemTask`** for each `ExternalGame` WHERE `is_available = true AND is_skipped = false`. Each job item's `source_metadata_json` carries `{ "external_game_id": "...", "raw_platform": "pc-windows" }` (the `raw_platform` comes from the adapter entry — NOT stored on `ExternalGame`).
7. **Update `last_synced_at`.** After successfully dispatching all items, set `user_sync_configs.last_synced_at = now()`. This ensures the next scheduler check uses the correct elapsed time. (The Python implementation omitted this update, which was a bug. The Go port always writes `last_synced_at` after a successful dispatch.)

**PSN token expiry during fetch:** If the PSN adapter gets a token-rejected error, update the credentials row: `is_verified = false`, `token_expired_at = <current UTC timestamp>`. Then fail the job with `"psn_token_expired"`.

#### ProcessSyncItemTask (worker pool task)

Payload: carried in `job_items.source_metadata_json = { "external_game_id": "...", "raw_platform": "..." }`.

Platform and storefront resolution for `UserGamePlatform`:

- `raw_platform` (from `source_metadata_json`) is passed to `platform_resolution.go` → resolved to a canonical `platforms.name` slug (e.g. `"playstation-5"` → `"ps5"`, `"playstation-4"` → `"ps4"`, `"pc-windows"` → `"pc-windows"`).
- The sync-source storefront identifier (from `ExternalGame.storefront`, e.g. `"psn"`) is mapped to the collection storefront slug (e.g. `"playstation-store"`) — this mapping is also in `platform_resolution.go`. Steam: `"steam"` → `"steam"` (same). PSN: `"psn"` → `"playstation-store"`.

**Why `ExternalGame` has no `platform` field:** The Python model had `ExternalGame.platform` as an FK to `platforms.name`, populated by the sync adapter. This was incorrect — the canonical platform for a collection entry is a property of how the game appears in the user's collection (`UserGamePlatform`), not a permanent attribute of the external game record. The Go port intentionally omits this field. The platform flows through `source_metadata_json` on the job item and is resolved at processing time, not stored redundantly on `ExternalGame`.

Phases:
1. Read `external_game_id` and `raw_platform` from `source_metadata_json`. Load `ExternalGame`.
2. If `eg.is_skipped`, mark item `SKIPPED`.
3. **Phase 4 — IGDB resolution.** If `eg.resolved_igdb_id` is null, run IGDB matching. Score ≥ 0.85 → auto-match, store `resolved_igdb_id`. Score < 0.85 → `PENDING_REVIEW`. No candidates → `PENDING_REVIEW`.
4. **Phase 5 — Collection sync.** Resolve `raw_platform` → platform slug and sync-source storefront → storefront slug via `platform_resolution.go`. Find existing `UserGamePlatform` by `external_game_id`, or by `(user_game_id, platform, storefront)`. Create if missing. Update `hours_played` and `ownership_status` (never downgrade ownership rank: `owned > borrowed/rented > subscription > no_longer_owned`).
5. **Job completion check.** After each item is processed, count remaining non-terminal items (`pending`, `processing`, `pending_review`). If zero, mark `jobs.status = 'completed'`. `PENDING_REVIEW` items block completion — the job stays in `processing` state indefinitely until the user resolves each item via the job-items UI (confirmed from Python `_check_and_update_job_completion`). There is no intermediate job status; `processing` is the correct state while review is pending.

**`auto_add` is not used in sync processing.** The `auto_add` flag on `UserSyncConfig` is stored and exposed via the config API but has no effect on sync task execution — `ProcessSyncItemTask` always creates or updates `UserGamePlatform` rows unconditionally when IGDB resolution succeeds. This matches Python behavior: neither `dispatch_sync_items` nor `process_sync_item` reads `auto_add`. The field is preserved for potential future use.

### PSN Client Library

Use `github.com/sizovilya/go-psn-api` (21 stars, updated March 2026, passing CI — the most maintained Go PSN library). The other Go options (`FrostBreker/go-playstation-api`, `m-nt/go-psn-api`) are forks with no additional adoption.

**Auth flow for `POST /api/sync/psn/configure`:**

```go
opts := &psn.Options{Npsso: npssoToken, Lang: "en", Region: "us"}
client, err := psn.NewClient(opts)
// AuthWithNPSSO hits ca.account.sony.com — no region needed for the auth exchange itself.
// Returns ErrInvalidNPSSOToken (or wraps a PSN rejection) if the token is bad.
if err := client.AuthWithNPSSO(ctx, npssoToken); err != nil {
    return 400 invalid_npsso_token
}
// GetProfile(ctx, "me") fetches the authenticated user's own profile.
profile, err := client.GetProfile(ctx, "me")
// profile.OnlineID → online_id
// profile.NpID     → account_id (Sony's stable per-account identifier)
// profile.Region (ISO alpha-2, e.g. "US", "GB") → region; store whatever PSN returns.
```

The auth exchange (`AuthWithNPSSO`) is purely against `ca.account.sony.com` and does not require a region. The `Options.Region` field is used only for the profile endpoint URL prefix; `"us"` works for the initial profile fetch via the `"me"` identifier regardless of the user's actual region.

The `region` stored in credentials is the ISO 3166-1 alpha-2 code returned by the PSN profile call, **uppercase** (e.g. `"US"`, `"GB"`), confirmed from Python: `client.get_region().alpha_2`. If `go-psn-api`'s `GetProfile` does not expose a region field, store `""` rather than hardcoding a default.

### Handler Interfaces

External HTTP calls are hidden behind interfaces injected into the handler, so test stubs replace them without any HTTP mocking framework.

```go
// SteamClient abstracts the Steam Web API call used during credential verification.
// Steam ID and API key format validation happens before this is called.
type SteamClient interface {
    GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error)
}

type SteamPlayerSummary struct {
    PersonaName              string
    CommunityVisibilityState int // 3 = public profile
}

// PSNClient abstracts the PSN NPSSO exchange and account info retrieval.
type PSNClient interface {
    GetAccountInfo(ctx context.Context, npssoToken string) (*PSNAccountInfo, error)
}

type PSNAccountInfo struct {
    OnlineID  string
    AccountID string // Sony NpID — stable account identifier
    Region    string // ISO 3166-1 alpha-2 uppercase, e.g. "US", "GB" — stored for sync worker use
}

// Sentinel errors the handler maps to specific API error codes.
var (
    ErrInvalidNPSSOToken = errors.New("invalid npsso token") // → 400 invalid_npsso_token
    ErrSteamRateLimited  = errors.New("steam rate limited")  // → 200 rate_limited
    ErrSteamNetwork      = errors.New("steam network error") // → 200 network_error
    // Non-sentinel Steam errors map via SteamPlayerSummary fields:
    // communityvisibilitystate != 3 → private_profile
    // empty result → invalid_steam_id
)
```

Production implementations live in `internal/services/steam/` and `internal/services/psn/`, using `sizovilya/go-psn-api` for PSN and direct HTTP to `api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002` for Steam. Test stubs are inline structs in `sync_test.go`.

The handler constructor:

```go
type SyncHandler struct {
    db          *bun.DB
    pool        *worker.Pool
    steamClient SteamClient
    psnClient   PSNClient
}

func NewSyncHandler(db *bun.DB, pool *worker.Pool, steam SteamClient, psn PSNClient) *SyncHandler
```

---

## Testing

One test file `internal/api/sync_test.go` using testcontainers (same pattern as the rest of the test suite). External HTTP calls to Steam and PSN are stubbed via an interface injected into the handler.

Coverage:
- `GET /api/sync/config` returns virtual defaults for all three storefronts when no rows exist
- `PUT /api/sync/config/steam` creates row; second PUT updates it
- `PUT /api/sync/config/invalid` → 400
- `PUT /api/sync/config/epic` → 200 (config can be set, no trigger available)
- Steam verify: bad Steam ID format → 200 `valid: false`, no network call
- Steam verify: bad API key format → 200 `valid: false`, no network call
- Steam verify: stub returns success → credentials stored, 200 `valid: true`
- PSN configure: 63-char token → 400 immediately
- PSN configure: stub returns success → credentials stored
- Trigger `POST /api/sync/steam` → job created (high priority)
- Trigger `POST /api/sync/epic` → 400
- Trigger again (steam) → 409
- `GET /api/sync/steam/status` → reflects active job
- `GET /api/sync/psn/status` → zeroed response (`is_configured: false, token_expired: false, online_id: "", account_id: "", region: ""`) when no row exists
- `DELETE /api/sync/steam/connection` → 204 even with no row
- `DELETE /api/sync/psn/disconnect` → 204 even with no row
- `GET /api/sync/ignored` → empty list when no rows skipped
- `POST /api/sync/ignored/:id` → 404 for unknown ID; 204 on success; second POST is idempotent
- `DELETE /api/sync/ignored/:id` → 404 for unknown ID; 204 on success
- Skip/un-skip round-trip: POST then DELETE then verify `is_skipped = false`

---

## Frontend Changes (when porting `nexorious` frontend to `nexorious-go`)

The sync API surface is otherwise identical to Python; only field names change:

**In `src/api/sync.ts`:**
- `SyncConfigApiResponse.platform` → `storefront`
- `SyncStatusApiResponse.platform` → `storefront`
- `ManualSyncApiResponse.platform` → `storefront`
- Update all three `transform*` functions to read from `storefront` instead of `platform`

**In `src/types` (wherever `SyncConfig`, `SyncStatus`, `ManualSyncResponse` are defined):**
- `SyncConfig.platform` → `storefront`
- `SyncStatus.platform` → `storefront`
- `ManualSyncResponse.platform` → `storefront`
- Rename type `SyncPlatform` → `SyncStorefront` (the enum values `steam`, `psn`, `epic` are unchanged)

All component files that reference `config.platform`, `status.platform`, or the `SyncPlatform` type will need the corresponding rename. The rename is mechanical — no logic changes.
