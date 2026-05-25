# Platform Column and Credentials Persistence Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Overview

Two spec gaps remain after all previous gap-closure work on this branch:

1. **Platform column** — The Matched group in `ExternalGamesSection` must show a Platform column
   sourced from `external_game_platforms`, but neither the API response nor the frontend currently
   carry platform data for external games.

2. **`credentials_error` persistence** — The spec requires a unified `credentials_error` flag in
   all storefront status responses, covering both decryption failure and runtime auth failure
   detected during a sync. Currently:
   - Steam never returns `ErrCredentials` (API key rejection goes through as a generic error).
   - Epic never returns `ErrCredentials` (legendary auth failures go through as generic errors).
   - The flag is not persisted — it is either derived from a decryption attempt at request time
     (GOG, Epic) or from a stale `is_verified` column (PSN). If the user navigates to the status
     page after a failed sync, the error is invisible.

The spec is authoritative. Both gaps require code changes.

---

## Part A — Platform Column in the Matched Group

### Spec requirement

> The **Matched** group displays the external game title alongside a **Platform** column showing
> the platforms associated with that external game (sourced from `external_game_platforms`). Each
> platform is rendered as its canonical slug (e.g. `pc-windows`, `playstation-5`). When an
> external game has multiple platforms, all are shown in the same row.

### Backend — `internal/api/sync.go`

**`externalGameResponse` struct:** add a `Platforms` field. Bun cannot populate a `[]string` from
a raw-SQL scalar column directly; use a string aggregation in SQL and parse it in Go, **or** use
`string_agg` and deserialise. The simplest correct approach is to add to the SELECT:

```sql
COALESCE(
    (SELECT string_agg(egp.platform, ',' ORDER BY egp.platform)
     FROM external_game_platforms egp
     WHERE egp.external_game_id = eg.id),
    ''
) AS platforms_csv
```

Add to the struct:

```go
PlatformsCSV string `bun:"platforms_csv" json:"-"`
```

After `Scan`, convert to the JSON field:

```go
type externalGameWithPlatforms struct {
    externalGameResponse
    Platforms []string `json:"platforms"`
}
```

Alternatively — and more cleanly — add `Platforms []string` directly to `externalGameResponse`
with `json:"platforms"` and populate it in a post-scan loop by splitting on `,`:

```go
for i := range games {
    if games[i].PlatformsCSV != "" {
        games[i].Platforms = strings.Split(games[i].PlatformsCSV, ",")
    } else {
        games[i].Platforms = []string{}
    }
}
```

The `Platforms` field on the struct is `json:"platforms"` so it appears in the API response. The
`PlatformsCSV` field is `json:"-"` — it is only a scan target.

**No change to the SQL JOIN structure** — the subquery is a correlated subquery inside the SELECT
list, which avoids the duplicate-row bug described in prior specs (Steam games with multiple
`external_game_platforms` rows would produce duplicate rows if joined).

### Frontend — `ui/frontend/src/types/sync.ts`

Add `platforms: string[]` to `ExternalGame`. Remove `playtime_hours: number` — it is not returned
by the backend and was never part of the spec.

```ts
export interface ExternalGame {
  // ... existing fields ...
  platforms: string[];
}
```

### Frontend — `ui/frontend/src/components/sync/external-games-section.tsx`

In the Matched `<Collapsible>`, the table currently has three columns: Storefront Title, IGDB
Title, and an action column. Add a **Platform** column between IGDB Title and the action column.

**`<TableHeader>` row:**

```tsx
<TableHead>Storefront Title</TableHead>
<TableHead>IGDB Title</TableHead>
<TableHead>Platform</TableHead>
<TableHead />
```

**`<TableRow>` for each matched game:**

```tsx
<TableCell>{game.title}</TableCell>
<TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
<TableCell className="text-muted-foreground">
  {game.platforms.join(', ')}
</TableCell>
<TableCell className="text-right">
  <Button ...>Change Match</Button>
</TableCell>
```

### Files touched (Part A)

| File | Change |
|---|---|
| `internal/api/sync.go` | Add `PlatformsCSV` scan field; add `Platforms []string`; add `platforms_csv` subquery to SQL; post-scan split |
| `ui/frontend/src/types/sync.ts` | Add `platforms: string[]`; remove `playtime_hours: number` |
| `ui/frontend/src/components/sync/external-games-section.tsx` | Add Platform column header and cell to Matched table |

---

## Part B — `credentials_error` Persistence

### Spec requirement

> All storefronts expose credential problems through a unified `credentials_error` flag in their
> status response.
>
> | Storefront | Detection mechanism |
> |---|---|
> | Steam | Decryption failure, or API key rejected by the Steam API |
> | PSN | NPSSO token exchange failure (token expires ~every 2 months) |
> | GOG | OAuth2 refresh token failure |
> | Epic | Decryption failure, or Legendary CLI reports an authentication failure |

The flag must survive page navigation — it must be readable from the database, not recomputed on
every status request.

### B1 — Schema: add `credentials_error` column to `user_sync_configs`

**Edit** `internal/db/migrations/20260503000001_initial.up.sql` (no new migration file):

Add one column to the `user_sync_configs` CREATE TABLE statement:

```sql
credentials_error BOOLEAN NOT NULL DEFAULT false,
```

**`internal/db/models/models.go`** — add to `UserSyncConfig`:

```go
CredentialsError bool `bun:"credentials_error"`
```

### B2 — Worker: write `credentials_error` on auth failure and on success

`DispatchSyncWorker` in `internal/worker/tasks/sync.go` has **two** `ErrCredentials` check sites:

1. After `w.Adapter(...)` — decryption failure detected at factory time.
2. After `adapter.GetLibrary(...)` — runtime auth failure detected during the library fetch.

Add the `credentials_error = true` update at **both** sites:

```go
if errors.Is(err, ErrCredentials) {
    failSyncJob(ctx, w.DB, p.JobID, "credentials error")
    _, _ = w.DB.NewUpdate().
        TableExpr("user_sync_configs").
        Set("credentials_error = true, updated_at = now()").
        Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
        Exec(ctx)
    return nil
}
```

On successful job completion, extend the existing `last_synced_at` UPDATE to also clear the flag:

```go
_, err = w.DB.NewRaw(
    `UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now()
     WHERE user_id = ? AND storefront = ?`,
    now, p.UserID, p.Storefront,
).Exec(ctx)
```

### B3 — Credential-save handlers: clear `credentials_error` on new credential submission

When the user submits new credentials (regardless of whether a sync runs), clear the flag
optimistically so the UI reflects the new state. Update the following handlers in
`internal/api/sync.go`:

- `HandleConnectSteam` (or `HandleSaveSteamCredentials`) — after persisting credentials
- `HandleConfigurePSN` — after a successful NPSSO token exchange
- `HandleConnectGOG` — after persisting the OAuth tokens
- `HandleConnectEpic` — after persisting the legendary snapshot

In each, after the DB write:

```go
_, _ = h.db.NewUpdate().
    TableExpr("user_sync_configs").
    Set("credentials_error = false, updated_at = now()").
    Where("user_id = ? AND storefront = ?", userID, storefront).
    Exec(ctx)
```

### B4 — Connection status endpoints: read from `credentials_error` column

Replace all current `credentials_error` derivation logic in the four connection status endpoints
with a direct read from `user_sync_configs.credentials_error`:

**`HandleGetSteamConnection`** — currently sets `credentials_error: true` only on decryption
failure. Change to also read from `user_sync_configs`:

```go
var cfg models.UserSyncConfig
_ = h.db.NewSelect().Model(&cfg).
    Where("user_id = ? AND storefront = 'steam'", userID).
    Scan(ctx)
// credentialsError is true if decryption fails OR the column is true
credentialsError := cfg.CredentialsError || decryptErr != nil
```

Apply the same pattern to `HandleGetPSNStatus`, `HandleGetGOGConnection`, and
`HandleGetEpicConnection`.

For PSN specifically, remove the `is_verified` → `credentials_error` derivation that currently
conflates "verified once" with "credentials are currently good".

### B5 — Steam adapter: detect API key rejection as `ErrCredentials`

In `internal/services/steam/client.go`, define a sentinel:

```go
var ErrAPIKeyRejected = errors.New("steam: API key rejected")
```

In `GetOwnedGames`, return this sentinel when the API responds with HTTP 401 or 403:

```go
if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
    return nil, ErrAPIKeyRejected
}
```

In `internal/services/steam/adapter.go`, wrap the sentinel as `ErrCredentials`:

```go
owned, err := a.client.GetOwnedGames(ctx, a.apiKey, a.steamID)
if errors.Is(err, ErrAPIKeyRejected) {
    return fmt.Errorf("%w: steam API key rejected", storefrontadapter.ErrCredentials)
}
if err != nil {
    return fmt.Errorf("steam: fetch owned games: %w", err)
}
```

### B6 — Epic adapter: detect legendary auth failure as `ErrCredentials`

Legendary exits non-zero and writes to stderr when session authentication fails. Known patterns
include `"Not logged in"`, `"Login session invalidated"`, and `"Refreshing token failed"`.

In `internal/services/epic/client.go`, define a sentinel:

```go
var ErrAuthFailed = errors.New("epic: legendary auth failed")
```

In `GetLibrary`, after `runLegendary` returns an error, check the error message for auth-failure
substrings:

```go
out, err := c.runLegendary(ctx, userID, "list", "--json")
if err != nil {
    if isAuthError(err) {
        return ErrAuthFailed
    }
    return err
}
```

```go
func isAuthError(err error) bool {
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "not logged in") ||
        strings.Contains(msg, "login session") ||
        strings.Contains(msg, "token failed") ||
        strings.Contains(msg, "please login")
}
```

In `internal/services/epic/adapter.go`, wrap the sentinel:

```go
if errors.Is(fetchErr, ErrAuthFailed) {
    return fmt.Errorf("%w: epic legendary auth failure", storefrontadapter.ErrCredentials)
}
return fetchErr
```

### Files touched (Part B)

| File | Change |
|---|---|
| `internal/db/migrations/20260503000001_initial.up.sql` | Add `credentials_error BOOLEAN NOT NULL DEFAULT false` to `user_sync_configs` |
| `internal/db/models/models.go` | Add `CredentialsError bool` to `UserSyncConfig` |
| `internal/worker/tasks/sync.go` | Set `credentials_error = true` on ErrCredentials; clear on success |
| `internal/api/sync.go` | 4 credential-save handlers clear the flag; 4 status endpoints read from column |
| `internal/services/steam/client.go` | Add `ErrAPIKeyRejected`; return it on 401/403 in `GetOwnedGames` |
| `internal/services/steam/adapter.go` | Wrap `ErrAPIKeyRejected` as `storefrontadapter.ErrCredentials` |
| `internal/services/epic/client.go` | Add `ErrAuthFailed`; return it on legendary auth errors; add `isAuthError` |
| `internal/services/epic/adapter.go` | Wrap `ErrAuthFailed` as `storefrontadapter.ErrCredentials` |

---

## Testing

**Part A:**
- `TestHandleListExternalGames` in `internal/api/sync_test.go`: assert `platforms` array is
  present in matched-game responses; verify empty array is returned when no platforms exist.

**Part B:**
- `TestHandleListExternalGames` already covers the endpoint; add a case that seeds
  `credentials_error = true` in `user_sync_configs` and asserts the status endpoint returns
  `"credentials_error": true`.
- Steam adapter test: add a case where `GetOwnedGames` returns `ErrAPIKeyRejected`; assert
  `GetLibrary` returns a `storefrontadapter.ErrCredentials`-wrapped error.
- Epic adapter test: add a case where `GetLibrary` returns `ErrAuthFailed`; assert `GetLibrary`
  returns a `storefrontadapter.ErrCredentials`-wrapped error.
- `TestDispatchSyncWorker` (or equivalent): add a case where the adapter returns
  `ErrCredentials`; assert `user_sync_configs.credentials_error` is set to `true`.

---

## Out of Scope

- No frontend change for Part B: the `credentials_error` flag is already consumed correctly by
  the UI (hub card badge, detail page badge) after previous gap-closure work.
- PSN `is_verified` column: once `credentials_error` is the source of truth, `is_verified` can
  eventually be removed. Not in scope for this branch.
- Legendary stderr pattern list is not exhaustive. New legendary versions may introduce different
  messages; the `isAuthError` function should be treated as a best-effort heuristic. A false
  negative (missed auth error) degrades to a generic sync failure rather than a credentials
  warning — acceptable behaviour.
