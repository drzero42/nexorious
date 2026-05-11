# Sync API Design Spec

## Overview

Phase 4 first task: the full sync configuration and trigger surface for Steam, PSN, and (eventually) Epic. Covers Bun models for `external_games` and `user_sync_configs`, the generic config/trigger/status endpoints, and the storefront-specific credential verification and disconnect endpoints.

Epic authentication is deferred — the legendary-gl OAuth flow is unreliable and will be addressed in a later spec.

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
{ "npsso_token": "abc...64chars", "online_id": "MyPSNName", "account_id": "123456", "region": "SIEE", "is_verified": true, "token_expired_at": null }
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

Valid `:storefront` values: `steam`, `psn`, `epic`. Anything else → 400.

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
{ "success": true, "online_id": "MyPSNName", "account_id": "123456", "region": "SIEE", "message": "PSN configured successfully" }
```

On PSN rejection: 400 `{ "error": "invalid_npsso_token" }`.

**`GET /api/sync/psn/status`** — reads from the PSN credentials row:
```json
{ "is_configured": true, "online_id": "MyPSNName", "account_id": "123456", "region": "SIEE", "token_expired": false }
```
`token_expired` is derived as `is_verified == false AND token_expired_at != null` from the stored credentials JSON — this distinguishes "token expired" from "never configured". At configure time `is_verified` is set to `true` and `token_expired_at` to `null`. When the PSN sync worker receives a token rejection, it updates the credentials row: `is_verified = false`, `token_expired_at = <current UTC timestamp>`. Returns zeroed response if no row exists.

**`DELETE /api/sync/psn/disconnect`** — clears `storefront_credentials` on the psn row. 204, idempotent.

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
| Invalid `:storefront` param | 400 |
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

The `region` stored in credentials is the ISO alpha-2 code returned by the PSN profile call (confirmed from Python: `client.get_region().alpha_2`). If `go-psn-api`'s `GetProfile` does not expose a region field, store `""` rather than hardcoding `"us"`.

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
    Region    string // e.g. "us", "gb" — stored for sync worker use
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
- Steam verify: bad Steam ID format → 200 `valid: false`, no network call
- Steam verify: bad API key format → 200 `valid: false`, no network call
- Steam verify: stub returns success → credentials stored, 200 `valid: true`
- PSN configure: 63-char token → 400 immediately
- PSN configure: stub returns success → credentials stored
- Trigger `POST /api/sync/steam` → job created
- Trigger again → 409
- `GET /api/sync/steam/status` → reflects active job
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
