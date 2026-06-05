# Prune dead account-ID fields from sync GET responses

**Issue**: #800 (follow-up from #799)
**Date**: 2026-06-04

## Problem

After #799 removed the opaque account-ID line from the connected-state display,
the account-ID fields in the sync status/connection **GET** responses are no
longer read by any frontend component. They are dead end-to-end, and keeping
them on the wire perpetuates the "looks sensitive" concern #799 addressed only
in the UI.

## Scope

Remove the account-ID fields from the four sync GET (status/connection)
response shapes, end-to-end:

| Endpoint | Wire field | Go shape | TS domain type |
|---|---|---|---|
| `GET /sync/steam/connection` | `steam_id` | `steamConnectionResponse.SteamID` | `SteamConnectionData.steamId` |
| `GET /sync/psn/connection` | `account_id` | `psnStatusResponse.AccountID` | `PSNStatusResponse.accountId` |
| `GET /sync/epic/connection` | `account_id` | ad-hoc map in `HandleGetEpicConnection` | `EpicConnectionResponse.accountId` |
| `GET /sync/gog/connection` | `user_id` | ad-hoc map in `HandleGetGOGConnection` | `GOGConnectionResponse.userId` |

**Out of scope** (verified consumers; intentionally untouched):

- POST/connect/configure response shapes (`psnConfigureResponse`,
  `EpicConnectApiResponse`, `GOGConnectApiResponse`, Steam verify) — they may
  legitimately surface the ID immediately after connecting.
- Stored encrypted credentials (`user_sync_configs.storefront_credentials`) —
  the PSN sync worker reads `account_id` from there; the storage format does
  not change.

## Changes

### Go — `internal/api/sync.go`

1. `steamConnectionResponse`: drop `SteamID`; in `HandleGetSteamConnection`,
   drop `SteamID` from the local creds-decode struct and the response literal.
2. `psnStatusResponse`: drop `AccountID`; in `HandleGetPSNStatus`, drop
   `AccountID` from the creds-decode struct and the response literal.
   `OnlineID` and `Region` stay.
3. `HandleGetEpicConnection`: drop `"account_id"` from the response map and
   `AccountID` from the user.json decode struct; update the comment that
   mentions `account_id`.
4. `HandleGetGOGConnection`: drop `"user_id"` from the response map and
   `UserID` from the creds-decode struct.

### Go tests — `internal/api/sync_test.go`

Drop the now-stale assertions only (no absence assertions):

- `body["steam_id"]` check in `TestGetSteamConnection_Connected`
- `resp["account_id"]` checks in the Epic GET connection tests

Stored-credential fixtures keep their ID fields — they exercise the storage
format, which is unchanged.

### Frontend — `ui/frontend/src/api/sync.ts`

Drop `steam_id`, `account_id` (Epic + PSN), and `user_id` (GOG) from the four
`*ApiResponse` wire interfaces and the corresponding mapping lines in
`getSteamConnection`, `getEpicConnection`, `getPSNStatus`, `getGOGConnection`.

### Frontend — `ui/frontend/src/types/sync.ts`

Drop `SteamConnectionData.steamId`, `EpicConnectionResponse.accountId`,
`GOGConnectionResponse.userId`, `PSNStatusResponse.accountId`.

### Frontend tests

Remove the pruned fields from mock objects (`epic-connection-card.test.tsx`,
`psn-connection-card.test.tsx`, and any `api/sync.test.ts` / `use-sync*` mocks
that include them). TypeScript excess-property checks flag every leftover, so
`npm run check` is the completeness gate.

## Compatibility

Pure field removal from JSON responses. An older frontend reading a missing key
gets `undefined`, which nothing uses anymore. No migration, no API version
bump.

## Verification

- Hooks handle format/lint/build/typecheck.
- Targeted Go runs: `go test ./internal/api/... -run
  'SteamConnection|PSNStatus|EpicConnection|GOGConnection' -v`
- Frontend: `npm run test` for the touched test files; `npm run check` and
  `npm run knip` confirm no dangling references.
