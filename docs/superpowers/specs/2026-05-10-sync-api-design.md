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
    Platform        *string    `bun:"platform"                 json:"platform"`
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

`GET /api/sync/config` returns one entry per supported storefront. For storefronts with no DB row, a virtual default is returned (not persisted): `frequency: "manual"`, `auto_add: false`, `is_configured: false`, `id` set to a freshly generated UUID (not stored).

`PUT` body (all fields optional, partial update):
```json
{ "frequency": "manual|hourly|daily|weekly", "auto_add": true }
```
Upserts the row; creates it if missing.

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

`POST /api/sync/:storefront` — creates a high-priority sync job. Returns 409 if a `pending` or `processing` job already exists for this user+storefront. Dispatch to the worker pool via `pool.Submit`.

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

### Route ordering

Register storefront-specific routes (`/steam/verify`, `/psn/configure`, `/psn/status`, `/steam/connection`) before the `:storefront` parameterised routes. Echo v5 resolves static segments first but explicit ordering avoids surprises.

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
