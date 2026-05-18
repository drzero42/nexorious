# GOG Library Sync — Design

**Date:** 2026-05-18
**Status:** Approved
**Issue:** #511
**Research dossier:** [2026-05-18-gog-sync-research.md](./2026-05-18-gog-sync-research.md)

## Summary

Add GOG.com as a third game library sync source alongside Steam and PSN (with Epic in progress). Implementation uses a native Go HTTP client — no subprocess, no Python runtime — mirroring the Steam/PSN service package shape.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Auth UX | Paste-the-code, same as Epic | Public GOG OAuth client has a fixed redirect_uri to embed.gog.com; server-side callback is not possible |
| Platform mapping | Both `pc-windows` and `pc-linux` based on `worksOn` | GOG reports per-platform availability; users may own games on both |
| Playtime | Always 0 (known gap) | `getFilteredProducts` has no playtime hours field; `lastUpdated` is product metadata, not play time |
| OAuth implementation | Raw HTTP, no new dependency | Mirrors PSN/Steam; only two endpoints needed |
| Service structure | Split `auth.go` / `library.go` / `client.go` | Auth lifecycle warrants isolation; clean httptest boundaries |
| Multi-platform external IDs | Schema migration: add `raw_platform` to unique constraint | Real GOG product IDs throughout; one row per (product, platform) |
| Disconnect route naming | `DELETE /:storefront/connection` for all storefronts | Standardise PSN and Epic (currently `/disconnect`) as part of this PR |

## Architecture

### New package: `internal/services/gog/`

Three files with one responsibility each.

**`auth.go`**

- `BuildAuthURL() string` — returns the GOG OAuth login URL:
  ```
  https://login.gog.com/auth?client_id=46899977096215655
    &redirect_uri=https://embed.gog.com/on_login_success?origin=client
    &response_type=code&layout=client2
  ```
- `ExchangeCode(ctx, code string) (*TokenResponse, error)` — POSTs to `https://auth.gog.com/token` with `grant_type=authorization_code`
- `RefreshToken(ctx, refreshToken string) (*TokenResponse, error)` — POSTs with `grant_type=refresh_token`
- `TokenResponse` struct: `AccessToken`, `RefreshToken`, `ExpiresIn int`, `UserID string`, `Username string`
- Client credentials (`client_id=46899977096215655`, `client_secret=9d85c43b1718a5aa`) are package-level constants. These are the well-known public GOG Galaxy credentials used openly by Heroic, Lutris, gogg, gog-backup, and gogdl.

**`library.go`**

- `ExternalLibraryEntry` struct: `ExternalID string`, `Title string`, `RawPlatform string` (`"pc-windows"` or `"pc-linux"`), `PlaytimeHours int` (always 0 — GOG library API has no playtime field), `OwnershipStatus string` (always `"owned"`), `IsSubscription bool` (always false — GOG has no subscription service)
- `GetLibrary(ctx, accessToken string, batchSize int, onBatch func([]ExternalLibraryEntry) error) error` — pages `GET https://embed.gog.com/account/getFilteredProducts?mediaType=1&page=N` (GOG returns max 50 per page). For each product:
  - If `worksOn.Windows` → emit entry with `RawPlatform: "pc-windows"`
  - If `worksOn.Linux` → emit a second entry with the same `ExternalID` and `RawPlatform: "pc-linux"`
  - Calls `onBatch` per page

**`client.go`**

- `Client` struct with `httpClient *http.Client` and overrideable base URLs for testing
- `NewClient() *Client`
- Delegates to `auth.go` and `library.go` functions; owns no business logic
- Error sentinels: `ErrGOGAuthExpired`, `ErrGOGUnauthorized`
- `*Client` is the concrete type that satisfies both the `GOGClient` interface (used by the API handler — `BuildAuthURL` + `ExchangeCode`) and the `GOGLibraryAdapter` interface (used by the worker — `GetLibrary` + `RefreshToken`)

### Schema migration

New migration `20260518000001_external_games_platform_unique`:

```sql
-- Up: widen unique constraint to allow one row per (product, platform).
-- Needed for GOG games that run on both pc-windows and pc-linux.
ALTER TABLE external_games
    DROP CONSTRAINT external_games_user_id_storefront_external_id_key;

ALTER TABLE external_games
    ADD CONSTRAINT external_games_user_id_storefront_external_id_raw_platform_key
    UNIQUE NULLS NOT DISTINCT (user_id, storefront, external_id, raw_platform);
```

`NULLS NOT DISTINCT` ensures a NULL `raw_platform` still forms a unique triple with `(user_id, storefront, external_id)`, preventing accidental duplicates for storefronts that don't set `raw_platform`.

The down migration drops the new constraint and reinstates the original.

All existing `ON CONFLICT (user_id, storefront, external_id)` upserts in `DispatchSyncWorker` (Steam, PSN, Epic cases) must be updated to `ON CONFLICT (user_id, storefront, external_id, raw_platform)`.

### API handlers (`internal/api/sync.go`)

**New interface:**

```go
type GOGClient interface {
    BuildAuthURL() string
    ExchangeCode(ctx context.Context, code string) (*GOGTokenResponse, error)
}

type GOGTokenResponse struct {
    AccessToken  string
    RefreshToken string
    UserID       string
    Username     string
}
```

`SyncHandler` gets a `gogClient GOGClient` field. `NewSyncHandler` gains a `gog GOGClient` parameter.

`validConfigStorefronts`, `validTriggerStorefronts`, and `supportedStorefronts` all get `"gog"` added.

**New routes registered in `RegisterRoutes`:**

```
POST   /sync/gog/connect     HandleGOGConnect
GET    /sync/gog/connection  HandleGetGOGConnection
DELETE /sync/gog/connection  HandleGOGDisconnect
```

The generic parameterised routes (`POST /:storefront`, `GET /:storefront/status`, `GET /:storefront/external-games`, `DELETE /:storefront/data`) work for GOG automatically once `"gog"` is in `validConfigStorefronts`.

**Disconnect route rename (part of this PR):**

| Old | New |
|-----|-----|
| `DELETE /psn/disconnect` | `DELETE /psn/connection` |
| `DELETE /epic/disconnect` | `DELETE /epic/connection` |

Handler implementations (`HandlePSNDisconnect`, `HandleEpicDisconnect`) are unchanged; only the registered path changes.

**`HandleGOGConnect`** — binds `{ auth_code }`, calls `gogClient.ExchangeCode`, stores `{ access_token, refresh_token, user_id, username }` as JSON in `storefront_credentials` via upsert on `(user_id, storefront='gog')`. Returns `{ username, user_id }`.

**`HandleGOGDisconnect`** — NULLs `storefront_credentials` for the user's GOG config row. Returns 204.

**`HandleGetGOGConnection`** — returns `{ connected: bool, username: string, user_id: string, auth_url: string }`. `auth_url` is the result of `gogClient.BuildAuthURL()`, surfaced here so the frontend doesn't hardcode it.

**Token credential shape stored in `storefront_credentials`:**

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "user_id": "...",
  "username": "..."
}
```

No new DB column needed — `storefront_credentials TEXT` already exists and holds JSON for all storefronts.

### Worker (`internal/worker/tasks/sync.go`)

**New interface:**

```go
type GOGLibraryAdapter interface {
    GetLibrary(ctx context.Context, accessToken string, batchSize int,
        onBatch func([]gogsvc.ExternalLibraryEntry) error) error
    RefreshToken(ctx context.Context, refreshToken string) (*gogsvc.TokenResponse, error)
}
```

`DispatchSyncWorker` struct gets a `GOG GOGLibraryAdapter` field.

**New `case "gog":` in `Work()`:**

1. Unmarshal `storefront_credentials` → `{ access_token, refresh_token, user_id, username }`
2. Call `w.GOG.RefreshToken(ctx, refreshToken)` upfront; persist the new `access_token` and `refresh_token` back to `storefront_credentials` (GOG refresh tokens may rotate — always re-store after refresh)
3. Call `w.GOG.GetLibrary(ctx, accessToken, batchSize, onBatch)` — for each page, upsert into `external_games` using `ON CONFLICT (user_id, storefront, external_id, raw_platform)`, dispatch `ProcessSyncItemArgs` River jobs
4. Dual-platform games (Windows + Linux) produce two `ExternalLibraryEntry` rows with the same `ExternalID` but different `RawPlatform`; the new unique constraint handles both without synthetic ID suffixes
5. After fetch, mark unavailable any `external_games` rows for this user+storefront whose `external_id` was not in the fetched set

**`platformresolution/resolution.go` additions:**

```go
// In RawPlatformToSlug:
case "pc-linux":
    return "pc-linux", true

// In StorefrontToCollectionSlug:
case "gog":
    return "gog", true
```

### Sibling matching for dual-platform games

The existing cross-SKU sibling logic in `ProcessSyncItemWorker` (step 3.6) matches on `(user_id, storefront, title)`. A GOG game available on both Windows and Linux produces two `external_games` rows with identical titles. When the Windows entry is processed first and auto-resolves to an IGDB ID, the Linux entry's worker run finds the resolved sibling and inherits the IGDB ID — skipping the IGDB search entirely. If both land in pending-review simultaneously, resolving one causes the other to auto-complete on retry. This is identical to how PSN PS4/PS5 sibling pairs work.

### Frontend

Most scaffolding already exists (`SyncPlatform.GOG`, `JobSource.GOG`, `getPlatformDisplayInfo` for GOG, logos at `public/logos/storefronts/gog/`).

**`types/sync.ts`**
- Add `GOG_AUTH_URL` constant (fixed public URL — same pattern as `EPIC_AUTH_URL`)
- Add `GOGConnectRequest`, `GOGConnectResponse`, `GOGConnectionResponse` interfaces
- Add `SyncPlatform.GOG` to `SUPPORTED_SYNC_PLATFORMS`

**`api/sync.ts`**
- Add `connectGOG`, `getGOGConnection`, `disconnectGOG`
- Update `disconnectPSN` → `DELETE /sync/psn/connection`
- Update `disconnectEpic` → `DELETE /sync/epic/connection`

**`hooks/use-sync.ts`** + **`hooks/index.ts`**
- Add `useGOGConnection`, `useConnectGOG`, `useDisconnectGOG`

**`components/sync/gog-connection-card.tsx`** (new)
- Structure mirrors `epic-connection-card.tsx`
- Shows GOG auth URL link, auth code input, connected state (username), disconnect button

**`routes/_authenticated/sync/$platform.tsx`**
- Add `{platform === SyncPlatform.GOG && <GOGConnectionCard ... />}` block

**`components/sync/index.ts`**
- Export `GOGConnectionCard`

### Slumber (`slumber.yaml`)

New `gog/` folder with requests: `connect`, `connection` (GET), `disconnect` (DELETE), `external-games`, `reset`, `status`, `trigger` — all with bearer auth.

Update `psn/disconnect` and `epic/disconnect` entries to reflect renamed routes.

## Testing

**`internal/services/gog/`** — httptest unit tests:
- `auth_test.go`: `ExchangeCode` success/failure, `RefreshToken` success/expired
- `library_test.go`: single-page, multi-page paging, dual-platform game emits two entries, Windows-only emits one, `PlaytimeHours` is always 0

**`internal/api/sync_test.go`** — integration tests against real DB:
- `HandleGOGConnect`: success, missing auth code, exchange failure
- `HandleGOGDisconnect`: clears credentials
- `HandleGetGOGConnection`: not connected, connected

**`internal/worker/tasks/sync_test.go`** — extend existing suite:
- GOG dispatch: credentials read, token refresh persisted, batch upsert, dual-platform entries both created
- Existing Steam/PSN/Epic upsert tests implicitly cover the ON CONFLICT column rename

**Frontend** — `gog-connection-card.test.tsx` mirroring `epic-connection-card.test.tsx`

## Known gaps (v1)

- **Playtime hours** — GOG library API has no playtime field; all GOG entries sync with `playtime_hours = 0`. A future issue could explore GOG's per-game achievement/stats endpoints.
- **Achievements** — out of scope for v1.
- **Unofficial API risk** — GOG has not documented or versioned these endpoints, but they have been stable since ~2017 and are used openly by Heroic, Lutris, and gog-backup. All GOG HTTP knowledge is isolated in `internal/services/gog/` to minimise the blast radius of any future changes.
- **Refresh token rotation** — GOG's behavior on rotation is undocumented; we always re-store the token returned by the refresh call, which is the convention followed by all known third-party clients.
