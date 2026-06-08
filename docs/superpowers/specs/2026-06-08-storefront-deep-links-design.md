# Storefront Deep-Links via a Decoupled Enrichment Worker

**Issue:** #831 — _feat: deep-link to each storefront's product page from the game details page_
**Date:** 2026-06-08
**Status:** Approved design

## Summary

On the game details page (`/games/$id`), each storefront a game is associated
with is currently a non-clickable label. This feature turns those into links to
the game's product page on that storefront, opened in a new tab.

The data needed to build a product URL is captured per `external_games` row in a
new uniform `store_link` column. `store_link` is **owned and written exclusively
by a new background enrichment worker** — never by the sync hot path. A single
URL-builder in the API layer turns `(storefront, store_link)` into the product
URL when serving the game. Where no URL can be built, the storefront stays a
plain label.

This design deliberately diverges from the issue's original "resolve inline
during sync" model: resolution is decoupled into a dedicated River worker so
that (a) sync never blocks on per-game store lookups, and (b) an admin can
re-run resolution as a maintenance action to fix stale/broken links.

## Decisions (locked during brainstorming)

- **Architecture:** separate enrichment worker (Approach C), modelled on the
  existing `metadata_refresh` dispatch + item worker pattern.
- **Item granularity:** one `job_item` per `(user_id, storefront)` group — not
  per game. This amortises Epic's bulk-map fetch and PSN authentication once per
  group and keeps River job counts small. Trade-off accepted: retry/progress is
  per-storefront, not per-game.
- **Ownership:** the enrichment worker is the **sole owner** of `store_link`.
  Sync never resolves or writes it.
- **Epic namespace:** sync **persists** Epic's `namespace` (capture only) into a
  new generic `external_games.source_metadata` jsonb column, because Epic's
  resolution input (`namespace`) is not derivable from `external_games` alone
  (its `external_id` is the `app_name`, and `productmapping` is keyed by
  `namespace`). This does not write `store_link`, so ownership is preserved.
- **Triggers (nothing periodic):**
  - **Sync completion** enqueues a **scoped, incremental** enrichment for that
    job's `(user, storefront)` only (`Force=false` → null rows only).
  - **Admin manual** trigger enqueues a **global, force** re-resolve
    (`Force=true` → all resolvable rows refetched). Manual-only.
- **Admin UX:** a single "Refresh store links" button (mirroring the existing
  metadata-refresh button). A manual run implicitly re-resolves everything; the
  word "force" does not appear in the UI.
- **Storefront slugs are canonical** (`storefronts.name`), per the recent
  unification (migration `20260605000003`): `steam`, `gog`,
  `epic-games-store`, `playstation-store`, `humble-bundle`. The URL-builder keys
  on these — **not** the old `epic`/`psn`.

## URL model

A single pure builder lives in the API layer. URL formats live in code, so a
store changing its scheme is a code fix, not a re-sync.

```
buildStoreURL(storefront, store_link):
  store_link == ""              -> no link
  steam                         -> https://store.steampowered.com/app/{store_link}/
  gog                           -> https://www.gog.com/game/{store_link}
  epic-games-store              -> https://store.epicgames.com/en-US/p/{store_link}
  playstation-store             -> https://store.playstation.com/en-us/concept/{store_link}
  humble-bundle / unknown       -> no link
```

A storefront is rendered as a link iff the builder returns a URL.

## What `store_link` holds, and how each store resolves it

| Storefront | `store_link` holds | Resolution (in the enrichment item worker) | Creds needed | Resolvable from `external_games` alone? |
|---|---|---|---|---|
| **steam** | appid | copy `external_id` | none | yes |
| **gog** | product slug | `GET https://api.gog.com/products/{external_id}` → `slug` | refresh token | yes |
| **playstation-store** | concept ID | authenticate (NPSSO) once, then `GET https://m.np.playstation.com/api/catalog/v2/titles/{external_id}/concepts` → concept ID | NPSSO token | yes (+ creds) |
| **epic-games-store** | product slug | look up `source_metadata.namespace` in `productmapping` (`GET https://store-content-ipv4.ak.epicgames.com/api/content/productmapping`, fetched once per item) | none | **no** — needs persisted `namespace` |
| **humble-bundle** | — (always null) | none | — | n/a (excluded) |

Resolution is **best-effort**: any failed or missing lookup leaves `store_link`
null and never fails the job. Unresolved cases (delisted titles, namespaces
absent from the mapping, third-party-key titles, PSN titles with no concept,
etc.) simply render no link.

## Components

### 1. Schema (one new migration; new files)

New migration `internal/db/migrations/20260608000001_add_store_link_and_source_metadata_to_external_games.{up,down}.sql`:

```sql
-- up
ALTER TABLE external_games
    ADD COLUMN store_link      TEXT,
    ADD COLUMN source_metadata JSONB;
-- down
ALTER TABLE external_games
    DROP COLUMN IF EXISTS store_link,
    DROP COLUMN IF EXISTS source_metadata;
```

Both columns are nullable. The migration adds them empty; `store_link` populates
as each storefront's enrichment runs (no data backfill).

Model (`internal/db/models/models.go`, `ExternalGame`):

```go
StoreLink      *string         `bun:"store_link"       json:"store_link,omitempty"`
SourceMetadata json.RawMessage `bun:"source_metadata"  json:"source_metadata,omitempty"`
```

### 2. Sync change — capture only, never writes `store_link`

- `storefrontadapter.ExternalGameEntry` gains a generic
  `SourceMetadata map[string]string` field.
- The Epic adapter (`internal/services/epic/adapter.go`) stops discarding
  `Namespace`; it sets `entry.SourceMetadata = map[string]string{"namespace": e.Namespace}`.
  All other adapters leave it nil/empty.
- `upsertExternalGame` (`internal/worker/tasks/sync.go`) includes
  `source_metadata` in the `INSERT` and in `ON CONFLICT DO UPDATE SET
  source_metadata = EXCLUDED.source_metadata`. It does **not** reference
  `store_link`, so re-syncs can never null an already-resolved link.

### 3. Enrichment worker (`internal/worker/tasks/store_link_refresh.go`)

Modelled directly on `metadata_refresh.go`.

**Dispatch worker** — `StoreLinkRefreshDispatchArgs{ UserID, Storefront string; Force bool }`,
kind `store_link_refresh_dispatch`:

1. Scoped active-job guard: skip if an equivalent dispatch (same `UserID` +
   `Storefront` + `Force`) is already `pending`/`processing`. A Steam sync
   finishing never blocks a concurrent GOG enrichment.
2. Select target `(user_id, storefront)` groups from `external_games`:

   ```sql
   SELECT DISTINCT user_id, storefront
   FROM external_games
   WHERE storefront IN ('steam','gog','epic-games-store','playstation-store')
     AND is_available = true
     AND (?::bool OR store_link IS NULL)        -- Force=false → null rows only
     AND (? = '' OR user_id = ?)                 -- optional scope
     AND (? = '' OR storefront = ?)
   ```
3. If no groups, return early (no job created).
4. In one transaction, insert a `jobs` row (`job_type = store_link_refresh`,
   `source = system`, `status = processing`, `priority = low`, `total_items =
   <group count>`) and one `job_item` per group (`item_key = storefront`,
   `user_id = <group user>`, `source_metadata = {"storefront": …, "force": …}`).
5. After commit, enqueue one `StoreLinkRefreshItemArgs` River job per item via
   `EnqueueOrFail`.
6. Emit admin maintenance notifications (`notify.Emit`), mirroring
   `metadata_refresh`.

**Item worker** — `StoreLinkRefreshItemArgs{ JobItemID string }`, kind
`store_link_refresh_item`:

1. Load the `job_item`; read its `user_id`, `storefront` (`item_key`), and
   `force` (from `source_metadata`).
2. Construct a resolver for the storefront via a resolver factory (see §4),
   loading + decrypting that user's `user_sync_configs` creds as needed.
3. Select the group's target rows:
   `WHERE user_id = ? AND storefront = ? AND is_available = true AND (force OR store_link IS NULL)`.
4. Resolve each row's `store_link` (per the table above); shared per-group setup
   (Epic `productmapping` fetch / PSN authentication) happens once. Update each
   resolved row; leave unresolved rows null. Rate-limit using the existing
   client limiters.
5. Mark the `job_item` completed and call a `storeLinkRefreshCheckJobCompletion`
   helper reusing `countJobItems` / `finalizeJobCompleted`.

**New per-storefront client resolution methods:**

- `gog.Client.ResolveSlug(ctx, productID string) (string, error)`
- `psn.Client.ResolveConceptID(ctx, accessToken, titleID string) (string, error)`
- `epic`: a `productmapping` fetcher returning `map[string]string` + a lookup by
  namespace (no auth).
- Steam needs no method (copy `external_id`).

### 4. Resolver factory & worker wiring

- A `buildStoreLinkResolverFactory(db, encrypter, epicClient)` helper in
  `cmd/nexorious/serve.go`, mirroring `buildAdapterFactory`, returns a
  per-storefront resolver with decrypted creds. (GOG = refresh token; PSN =
  NPSSO; Epic/Steam = no creds.)
- Register the two new workers in **both** `river.AddWorker` blocks in
  `serve.go` (the primary block and the DB-reconnect re-init block ~L276).
  The dispatch worker takes `DB` + `RiverClient`; the item worker takes `DB` +
  the resolver factory.

### 5. Triggers (nothing periodic)

- **Sync completion (scoped, incremental):** in the success branch of
  `SyncCheckJobCompletion` (`internal/worker/tasks/sync.go`, after
  `finalizeJobCompleted` returns true), enqueue
  `StoreLinkRefreshDispatchArgs{UserID, Storefront, Force:false}` for that job's
  storefront. This requires threading `RiverClient` into
  `SyncCheckJobCompletion` — its worker callers already hold one. The scoped
  dispatch yields exactly one `(user, storefront)` `job_item`; a group with no
  null rows produces no job.
- **Admin manual (global, force):** `POST /api/games/store-links/refresh-job`,
  admin-only, mirrors `HandleStartMetadataRefreshJob` (`internal/api/games.go`).
  Enqueues `StoreLinkRefreshDispatchArgs{Force:true}` (no scope → all groups).
  Register the route alongside the existing metadata-refresh route.
- **No `BuildPeriodicJobs` entry and no new config.**
- New constant `models.JobTypeStoreLinkRefresh = "store_link_refresh"` in
  `internal/db/models/jobs.go`.

### 6. API read path (game details only)

- A pure `buildStoreURL(storefront, storeLink string) (string, bool)` in the API
  layer — the single place URL formats live.
- Add an `ExternalGame` relation to the `UserGamePlatform` model
  (`bun:"rel:belongs-to,join:external_game_id=id"`).
- `HandleGetUserGame` (`internal/api/user_games.go`) loads that relation so each
  platform has its `external_games.store_link` + `storefront`.
- `toUserGamePlatformResponse` sets a new
  `StoreURL *string json:"store_url,omitempty"` on `userGamePlatformResponse`
  when `buildStoreURL` returns a URL. Other endpoints that reuse the conversion
  without loading the relation simply omit `store_url` (harmless).

### 7. Frontend

- `UserGamePlatform` type (`ui/frontend/src/types/game.ts`) gains
  `store_url?: string`.
- `$id.index.tsx` (Platforms & Ownership section): when `p.store_url` is
  present, render the storefront indicator as
  `<a href={p.store_url} target="_blank" rel="noopener noreferrer">`; otherwise
  keep the plain `({display_name})` label.
- Admin maintenance UI: add a single "Refresh store links" button next to the
  existing metadata-refresh control, calling the new admin endpoint.

## Testing

- **Go unit:** `buildStoreURL` for every storefront + null + humble-bundle +
  unknown.
- **Go worker:** dispatch worker — scoped vs global selection, `Force` vs
  incremental row selection, scoped active-job guard, empty-result no-op,
  group fan-out. Item worker — per-storefront resolution, best-effort null on
  failure, completion tracking. Use the shared test DB container per
  package-level `TestMain` (no per-test container).
- **Go integration:** `HandleGetUserGame` returns `store_url` when a resolvable
  `store_link` exists and omits it otherwise.
- **Frontend:** component test asserting link vs plain-label rendering based on
  `store_url`.

## Out of scope

- **humble-bundle** — no reliable public per-game store URL; `store_link` stays
  null, no link rendered.
- **Non-sync storefront associations** — storefronts created outside sync (CSV
  import onto Microsoft Store, Itch.io, Nintendo eShop, etc.) have no sync path
  to populate `store_link`, so no link.
- **Data backfill** — columns are added empty; `store_link` populates as each
  storefront re-syncs (which enqueues scoped enrichment) or via a manual admin
  refresh. No migration-time backfill.
- **Periodic enrichment** — explicitly excluded; resolution is event-driven
  (sync completion) plus the manual admin action.
