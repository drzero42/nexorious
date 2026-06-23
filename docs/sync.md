# Sync Process

This document is the source of truth for how Nexorious syncs a user's game library from an external storefront. It describes how the process **should** work. It is kept up to date as the process evolves and is intended for both humans and coding agents working on the sync system.

---

## Overview

Syncing imports a user's game library from an external storefront (Steam, PSN, GOG, Epic Games Store, or Humble Bundle) into Nexorious. The process fetches the library, matches each game to an entry in the IGDB game database, and creates or updates the user's Nexorious library accordingly.

The sync pipeline is designed to be:

- **Consistent** — the same general process applies to all storefronts; per-storefront differences are isolated to adapter code
- **Resilient** — transient failures are retried automatically; anything the process cannot resolve on its own — whether from an API failure or an ambiguous match — is routed to the user without blocking the rest of the sync
- **Non-destructive** — ownership and playtime are never downgraded; existing data is only improved

---

## Glossary

| Term | Meaning |
|---|---|
| **Storefront** | An external game store: Steam, PSN, GOG, Epic Games Store, or Humble Bundle |
| **ExternalGame** | A game record fetched from a storefront; persisted across sync runs |
| **ExternalGamePlatform** | A platform slug (e.g. `pc-windows`, `playstation-5`) that an ExternalGame is available on |
| **Job** | A sync run for one user and one storefront; tracks overall progress and status |
| **JobItem** | One game within a Job; tracks per-game matching progress |
| **IGDB** | The canonical game database used to identify and deduplicate games across storefronts |
| **pending_review** | A JobItem state where the user must manually pick an IGDB match or skip the game |
| **Ownership rank** | A hierarchy that prevents ownership downgrades: `owned` > `borrowed` / `rented` > `subscription` > `no_longer_owned` |

---

## Data Model

The sync system reads and writes these core tables:

| Table | Role |
|---|---|
| `user_sync_configs` | Credentials, sync frequency, and last sync timestamp per user and storefront |
| `external_games` | One row per user + storefront + game; persists across sync runs. `parent_id` (nullable FK to self) marks duplicate-SKU siblings established at Stage 1. `store_link` (nullable) holds the product-page identifier written by store-link enrichment; `source_metadata` (jsonb, nullable) captures per-source resolution inputs (e.g. Epic's namespace) at Stage 1 |
| `external_game_platforms` | Platform slugs and per-platform playtime for each ExternalGame |
| `jobs` | One row per sync run; tracks status and lifecycle |
| `job_items` | One row per game per sync run; tracks matching progress |
| `sync_changes` | Changelog entries written by Stage 1 and Stage 3; backs the Sync History UI |
| `games` | IGDB master catalogue; new rows are inserted when a match is found |
| `user_games` | The user's canonical library; one row per user + IGDB game |
| `user_game_platforms` | One row per user + game + platform + storefront combination |

### Key relationships

`user_games` holds one row per user per IGDB game, regardless of how many storefronts or platforms the game was found on. Each `user_game_platforms` row points back to the specific `external_games` row that created it via `external_game_id`. This means a single `user_games` row can have multiple `user_game_platforms` rows pointing to different `external_games` rows — which is expected and correct for storefronts that create multiple ExternalGame rows for the same game.

For example, PSN creates one ExternalGame row per title ID. The PS4 and PS5 versions of the same game are separate title IDs, so they produce separate ExternalGame rows and separate `user_game_platforms` rows — but both point to the same `user_games` row.

### Playtime

Playtime is stored at the `external_game_platforms` level (`hours_played`). Stage 1 writes the correct value to each platform row as part of the upsert; Stage 3 reads from there when writing to `user_game_platforms`. The total playtime for a game in the user's library is the sum of `hours_played` across all its `user_game_platforms` rows.

Not all storefronts provide playtime. When a storefront does not provide playtime, `hours_played` is 0 for all platform rows. Playtime is never decreased — a `user_game_platforms` row's `hours_played` is only updated when the incoming value is greater than the stored value.

### Sync Changes

`sync_changes` records what happened to a user's library during each sync run. Each row captures one event:

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT | Primary key |
| `job_id` | TEXT | The sync job that produced this change; FK to `jobs` |
| `user_id` | TEXT | FK to `users` |
| `external_game_id` | TEXT | FK to `external_games`; nullable (SET NULL on delete) |
| `change_type` | TEXT | `added`, `removed`, or `status_changed` |
| `title` | TEXT | Game title at the time of the event; denormalised for display |
| `old_status` | TEXT | Previous ownership status; only set for `status_changed` |
| `new_status` | TEXT | New ownership status; only set for `status_changed` |
| `created_at` | TIMESTAMPTZ | When the event was recorded |

Writers:
- **Stage 1** writes a `removed` entry for each game marked `is_available = false` during the availability sweep
- **Stage 3** writes an `added` entry when a new `user_games` row is inserted, and a `status_changed` entry when the ownership rank guard replaces an existing status

Old entries are pruned by a periodic maintenance job (see Maintenance).

---

## Architecture

The sync pipeline has three stages. Each stage is implemented as a River worker job in the `tasks` package. The `DispatchSyncWorker` defines a standard adapter interface; each storefront implements that interface in its own `services/` package (`services/steam`, `services/playstationstore`, `services/gog`, `services/epicgamesstore`, `services/humble`). Storefront-specific knowledge — auth, API communication, credential lifecycle — never crosses into the workers.

```mermaid
flowchart TD
    A([Trigger: manual or scheduled]) --> B

    subgraph Stage1["Stage 1 — Fetch"]
        B[DispatchSyncWorker<br/>records sync_started_at] --> C
        C[Adapter fetches library<br/>in batches of ≤10] --> D
        D[Upsert external_games<br/>+ external_game_platforms] --> E
        E[Enqueue one Stage 2 job<br/>per game in batch] --> F{More batches?}
        F -->|yes| C
        F -->|no| G[Availability sweep:<br/>mark missing games is_available=false]
    end

    subgraph Stage2["Stage 2 — IGDB Match"]
        H{is_skipped?} -->|yes| L
        H -->|no| I{Sibling resolved?}
        I -->|yes| J[Inherit resolved_igdb_id]
        J --> K{resolved_igdb_id set?}
        I -->|no| K
        K -->|yes| L[Enqueue Stage 3]
        K -->|no| N[Search IGDB<br/>score candidates]
        N --> O{Clear winner<br/>score ≥ 0.85?}
        O -->|yes| P[Set resolved_igdb_id<br/>on external_game]
        P --> L
        O -->|no, or retries exhausted| M([pending_review:<br/>await user action])
    end

    subgraph Stage3["Stage 3 — User Game Write"]
        S3A{is_skipped?} -->|yes| S3D[Update<br/>external_game.updated_at]
        S3A -->|no| S3B[Upsert user_games]
        S3B --> S3C[Upsert user_game_platforms<br/>per platform<br/>with ownership rank guard]
        S3C --> S3D
    end

    subgraph UserAction["User Action"]
        M --> R{User decision}
        R -->|picks IGDB match| S[Set resolved_igdb_id<br/>enqueue Stage 3]
        R -->|skips game| T[Set is_skipped=true<br/>mark item skipped]
    end

    E --> H
    L --> S3A
    S --> S3A
```

### DispatchSyncWorker responsibilities

- Recording `sync_started_at` at the beginning of a sync run
- Calling the adapter's batch callback and iterating until the library is fully fetched
- Applying rate limiting between API calls
- Upserting `external_games` and `external_game_platforms` after each batch
- Enqueuing Stage 2 jobs after each batch
- Running the availability sweep at the end of the fetch phase
- Failing the job and cancelling pending items on credential errors

### Storefront adapter responsibilities

Each adapter lives in its own `services/` package and is responsible for:

- All authentication mechanics (token refresh, CLI state management, credential expiry detection)
- Signalling credential errors to the worker
- Yielding games in batches of ≤10 via a callback

### Adapter interface

The `tasks` package defines a concrete Go interface (`StorefrontAdapter`) that every storefront adapter must implement. Each `services/` package implements this interface; the `DispatchSyncWorker` depends only on the interface, never on a concrete adapter type.

The interface requires a `GetLibrary` method that accepts a context, a batch size, and a callback. The adapter calls the callback once per batch of ≤10 games, each represented by a `GameEntry` value with the following fields:

| Field | Type | Notes |
|---|---|---|
| `ExternalID` | string | Storefront-specific game identifier |
| `Title` | string | Game name as reported by the storefront |
| `PlaytimeHours` | int | Hours played; 0 means not provided by this storefront |
| `Platforms` | []string | Platform names in storefront-specific format; resolved to canonical slugs by the worker |
| `OwnershipStatus` | string | `owned`, `subscription`, etc. |
| `IsSubscription` | bool | True if the game is accessed via a subscription service |
| `SourceMetadata` | map[string]string | Optional per-source resolution inputs captured for later store-link enrichment (currently only Epic sets `{"namespace": …}`); persisted to `external_games.source_metadata`. Never used to build the link directly |

---

## Stage 1 — Fetch

The `DispatchSyncWorker` runs once per sync job. It:

1. Records `sync_started_at`
2. Calls the storefront adapter which fetches the library and yields games in batches of ≤10
3. After each batch:
   - Upserts each game into `external_games`, always setting `updated_at = now()` and `is_available = true`
   - Accumulates each game's `external_id` into an in-memory set of fetched IDs
   - Upserts platform rows into `external_game_platforms`; removes any platform rows for that game that were not in this batch
   - Enqueues one Stage 2 job per game in the batch
4. After all batches complete, runs a sweep: queries all `external_games` rows for this user and storefront where `is_available = true`, and marks any whose `external_id` is not in the fetched ID set as `is_available = false` — these are games that were not seen in this sync run and have been removed from the user's library. For each game marked unavailable, writes a `removed` entry to `sync_changes`

If a credential error occurs at any point, the job is marked `failed` and all pending job_items are cancelled. Any `external_games` rows already upserted in this run are kept.

---

## Stage 2 — IGDB Match

One `IGDBMatchWorker` job runs per game. River handles retries with exponential backoff for transient IGDB API failures.

1. **Skipped?** If `is_skipped` is true, route directly to Stage 3 — no matching is ever performed for skipped games
2. **Sibling check:** Look for another `external_games` row for the same user, storefront, and title that already has `resolved_igdb_id` set. If found, inherit its `resolved_igdb_id`. This avoids an unnecessary IGDB search when a related entry has already been matched
3. **Already resolved?** If `resolved_igdb_id` is now set — either from a previous sync run or just inherited from a sibling — route directly to Stage 3. On subsequent syncs, most games will take this path
4. **Search IGDB** for the game title; score each candidate using fuzzy title matching
5. **Auto-resolve** if the best candidate scores ≥ 0.85 and has a clear margin (> 0.01) over the second-best: set `resolved_igdb_id` on the `external_game` and enqueue Stage 3
6. **pending_review** if no clear winner is found, or if IGDB API calls fail after all River retries are exhausted: mark the item `pending_review` for the user to resolve

### Title matching

Before searching, titles are normalised (trademark symbols removed, diacritics folded, common suffixes like "GOTY" expanded, etc.). Candidates are scored using a weighted combination of fuzzy matching algorithms. The auto-resolve threshold is 0.85 with a tie-breaking margin of 0.01.

### Siblings

A sibling is another `external_games` row for the same user, storefront, and title. This occurs on storefronts that assign separate identifiers to different platform releases of the same game — for example, PSN assigns distinct title IDs to the PS4 and PS5 versions of a game.

The sibling relationship is established explicitly during **Stage 1**: when a new `external_games` row is inserted with the same `(user_id, storefront, title)` as an existing row that has no `parent_id` itself, the new row's `parent_id` is set to the existing row's `id`. This produces a flat tree — all siblings point to one parent; no chaining.

The sibling relationship is acted on in three places:

- **Stage 2 (child):** if the external game has `parent_id IS NOT NULL`, check whether the parent already has `resolved_igdb_id`. If yes, inherit it and proceed to Stage 3. If no (parent still in flight or in `pending_review`), return without advancing the job item — the child waits in `pending` state.
- **Stage 3 (sibling trigger):** after writing the parent's library entries, query for child rows with `parent_id = eg.id` that are not yet resolved and have a `pending` job item. Re-enqueue Stage 2 for each. This handles the case where the child's Stage 2 ran before the parent was resolved, and also handles siblings that arrive in the library after the parent was already matched.
- **Manual match / skip (cascade):** `HandleRematchExternalGame` and `HandleSkipGame` look up children via `parent_id = eg.id` and propagate the resolution or skip flag to each child, then enqueue Stage 3 (rematch) or mark job items skipped (skip).

Child rows (`parent_id IS NOT NULL`) are filtered from all UI lists and counts — only the parent row is visible and actionable by the user.

---

## Stage 3 — User Game Write

One `UserGameWorker` job runs per game, enqueued by Stage 2 or by a user action.

1. If `is_skipped` is true: skip steps 2, 3, and 4
2. If `external_game.resolved_igdb_id` is not already set (i.e. Stage 3 was triggered by a manual user resolution, not auto-resolve), propagate `job_item.resolved_igdb_id` to `external_game.resolved_igdb_id` — this durably records the match for future sync runs
3. Upsert `user_games`: one row per user + IGDB game ID. If this is an INSERT (new game), write an `added` entry to `sync_changes`
4. For each platform row in `external_game_platforms`:
   - Upsert `user_game_platforms` with conflict key `(user_game_id, platform, storefront)`
   - On conflict: apply the ownership rank guard (never downgrade ownership status); update `hours_played` only if the incoming value from `external_game_platforms.hours_played` is greater; if the ownership status changed, write a `status_changed` entry to `sync_changes`
   - Set `external_game_id` to the specific ExternalGame row that produced this platform entry
5. Update `external_game.updated_at` — always, whether the game was skipped or not
6. After writing all platform rows, if IGDB is configured and the `games` row has no description, an immediate metadata fetch is enqueued for that game. This ensures newly added games have cover art and full IGDB data within seconds rather than waiting for the next scheduled bulk refresh. The enqueue is fire-and-forget and non-fatal — the periodic bulk refresh (see [docs/maintenance.md](maintenance.md) § "Metadata refresh") remains the safety net.

### Play Status

`user_games.play_status` defaults to `'not_started'`. Sync infers an initial status from the incoming hours:

- If total `hours_played` across all `external_game_platforms` rows for the game is **> 0**, and the current `play_status` is `'not_started'` (either because the row is new, or because it was previously unplayed), sync sets `play_status = 'in_progress'`.
- If total hours = 0, the DB default (`'not_started'`) applies and nothing is changed.

Sync can only auto-promote `not_started → in_progress`. Any other status the user has explicitly set (e.g. `'completed'`, `'on_hold'`) is never touched by sync.

Manually added games that omit `play_status` default to `'not_started'` via the DB default. The sync worker applies the same inference when it later processes that game from a storefront.

### Ownership rank guard

Ownership statuses have a fixed rank. A stored status is never replaced by one of lower rank:

```
owned  >  borrowed / rented  >  subscription  >  no_longer_owned
```

### Manually added games

A user may add a game to their library by hand and associate it with a platform and storefront before that storefront has ever been synced. When Stage 3 later processes the same game from a sync run, it encounters an existing `user_game_platforms` row with a matching `(user_game_id, platform, storefront)` key. The behaviour is identical to any other conflict: the ownership rank guard applies and playtime is updated only if the incoming value is higher.

Additionally, a manually added row has no `external_game_id` because it was not produced by a sync. Stage 3 fills this in: `external_game_id` is always set (or updated) to the `external_games` row that produced the platform entry, so the association is linked to the storefront record from that point forward.

---

## Job Lifecycle

```mermaid
stateDiagram-v2
    [*] --> pending : triggered (manual or scheduled)
    pending --> processing : DispatchSyncWorker starts
    processing --> processing : pending_review items remain
    processing --> completed : all items completed or skipped
    processing --> failed : credential error or fatal dispatch failure
    failed --> [*]
    completed --> [*]
```

A job is complete only when every job_item is either `completed` or `skipped`. Items in `pending_review` hold the job in `processing` indefinitely — the job does not time out waiting for the user.

### Job item statuses

| Status | Meaning |
|---|---|
| `pending` | Waiting to be picked up by a Stage 2 or Stage 3 worker |
| `processing` | Currently being worked on |
| `completed` | Successfully written to the user's library |
| `skipped` | Game is marked `is_skipped`; no user_game entry was created |
| `pending_review` | Awaiting the user to pick an IGDB match or skip the game |
| `cancelled` | Job failed mid-run; this item will not be processed |
| `failed` | Permanent failure (e.g. the external_game record is missing) |

---

## User Interactions

### Resolving a pending_review item

The user searches IGDB and selects a match. Once a match is chosen, the resolve endpoint (`HandleRematchExternalGame`) sets `resolved_igdb_id` on the parent `external_game` and enqueues Stage 3 immediately. Any children (`parent_id = eg.id`) are resolved with the same IGDB ID and also enqueued for Stage 3 at the time of the user's action. Siblings never appear in the Needs Review list — only the parent does.

### Skipping a game

The user marks a game as ignored. `is_skipped` is set to `true` on the `external_game` and the job_item is marked `skipped`. No Stage 3 job is created. On future syncs, Stage 2 routes the game directly to Stage 3, which updates `external_game.updated_at` and does nothing else.

### Unskipping a game

The user removes the skip. `is_skipped` is cleared. A new job_item is created and a Stage 2 job is enqueued immediately to begin IGDB matching.

### Rematching a game

The user replaces an existing IGDB match with a different one. `external_game.resolved_igdb_id` is updated and a Stage 3 job is enqueued immediately to update the user_game and platform associations.

---

## Credential Errors

All storefronts expose credential problems through a unified `credentials_error` flag in their status response. Each storefront detects errors differently, but all surface them the same way to the worker:

| Storefront | Detection mechanism |
|---|---|
| **Steam** | Decryption failure of `storefront_credentials`, or API key rejected by the Steam API |
| **PSN** | Authentication failure when exchanging the NPSSO token for an access token (token expires approximately every 2 months) |
| **GOG** | OAuth2 refresh token failure (refresh token expired or revoked) |
| **Epic** | Decryption failure of `storefront_credentials`, or Legendary CLI reports an authentication failure |
| **Humble Bundle** | Decryption failure of `storefront_credentials`, or the order API rejects the `_simpleauth_sess` session cookie (401/403); the cookie expires periodically and must be re-pasted |

When a credential error occurs mid-sync, the job is marked `failed` and all pending job_items are cancelled. The user must reconfigure their credentials before triggering a new sync.

Credentials are stored encrypted at rest in `user_sync_configs.storefront_credentials`. Decryption happens in memory during Stage 1 only; plaintext is never persisted. On decryption failure, the encrypted bytes are left untouched in the database — they are never cleared.

---

## Store-Link Enrichment

The game details page renders each storefront a game is associated with as a deep-link to that game's product page on that store. The link target is resolved by a dedicated background worker, decoupled from the sync pipeline so sync never blocks on per-game store lookups.

### Storage & URL model

A single `external_games.store_link` column holds whatever a store needs to build its product URL — always the same column for every storefront, holding a slug or an ID depending on the store. A pure URL-builder in the API layer (`internal/api/store_url.go`, `buildStoreURL`) turns `(storefront, store_link)` into the product URL when serving the game details endpoint, reading only `store_link`. URL formats live in code, so a store changing its scheme is a code fix, not a re-sync. A storefront is rendered as a link iff the builder returns a URL; otherwise it stays a plain label.

| Storefront | `store_link` holds | URL |
|---|---|---|
| `steam` | appid | `https://store.steampowered.com/app/{store_link}/` |
| `gog` | product slug | `https://www.gog.com/game/{store_link}` |
| `epic-games-store` | product slug | `https://store.epicgames.com/en-US/p/{store_link}` |
| `playstation-store` | concept ID | `https://store.playstation.com/en-us/concept/{store_link}` |
| `humble-bundle` | — (always null) | no link |

### Ownership: enrichment is the sole writer

`store_link` is written **only** by the enrichment worker; the sync upsert (Stage 1) never touches it, so a re-sync can never null a resolved link. Sync's only contribution is capturing resolution *inputs*: the Epic adapter records its `namespace` into `external_games.source_metadata` (Epic's `productmapping` is keyed by namespace, which is not derivable from `external_id` alone). The other stores resolve from `external_id` directly.

### Workers

Modelled on the metadata-refresh pattern (a dispatch + item worker pair, in `internal/worker/tasks/store_link_refresh.go`):

- **`StoreLinkRefreshDispatchWorker`** selects the distinct `(user, storefront)` groups needing work from `external_games` (restricted to the resolvable storefronts), creates a `jobs` row plus **one `job_item` per group**, and enqueues a per-group item job. A scoped active-job guard prevents a duplicate pass for the same scope (a Steam sync finishing never blocks a GOG enrichment).
  - `Force = false` (incremental): only rows where `store_link IS NULL`.
  - `Force = true`: re-resolves every row from upstream (the "fix stale/broken links" pass).
- **`StoreLinkRefreshItemWorker`** resolves the rows for its one `(user, storefront)` group, amortising shared setup once per group (Epic's `productmapping` fetch; PSN authentication). Resolution is **best-effort**: a failed or empty lookup leaves `store_link` null and never fails the job. Per-storefront resolvers live in `internal/services/storelink` (Steam copies the appid; GOG hits the public `api.gog.com/products/{id}`; Epic looks up the captured namespace in `productmapping`; PSN resolves a concept ID via the catalog API using the user's NPSSO).

### Triggers

Enrichment is **event-driven, never periodic**:

- **Sync completion** — when a sync job finalizes, `SyncCheckJobCompletion` enqueues a **scoped, incremental** dispatch (`Force = false`) for that job's storefront only. New/changed games get links shortly after their sync.
- **Admin maintenance** — `POST /api/games/store-links/refresh-job` (admin-only; the "Refresh store links" button on the maintenance page) enqueues a **global, forced** dispatch (`Force = true`) that re-resolves every resolvable row from upstream.

Storefront associations created outside sync (e.g. CSV import onto non-sync stores) have no resolver and render no link. Existing rows show no link until their storefront re-syncs (or an admin runs the manual refresh); there is no migration-time backfill.

---

## Scheduled Sync

A periodic worker checks `user_sync_configs` for all users where the sync frequency is not `manual` and the last sync was more than the configured interval ago (hourly / daily / weekly). For each, it creates a Job and enqueues a Stage 1 run — provided no active job already exists for that user and storefront. All five storefronts support scheduled sync.

---

## Maintenance

Maintenance tasks that support the sync system — sync history pruning, orphaned item rescue, and stale job cleanup — are documented in [docs/maintenance.md](maintenance.md).

An admin can also trigger a global store-link refresh (re-resolving every storefront product link from upstream) from the maintenance page; see [Store-Link Enrichment](#store-link-enrichment).

---

## Storefront Adapters

All adapters implement the same interface. The differences below are the only places where storefront-specific knowledge lives.

### Steam

- **Auth:** API key + Steam ID; static credentials, no refresh needed
- **Library fetch:** A single API call returns the full library. The adapter then makes one AppDetails API call per game to resolve platform availability, and chunks the enriched results into batches of ≤10 before yielding them via the callback. The batching is adapter-side; the Steam API itself is not paginated
- **Rate limiting:** A token bucket enforces a minimum delay between AppDetails calls. On a 429 response, the adapter backs off and retries. Rate limiting is handled consistently with the shared library's backoff interface
- **Platforms:** `pc-windows`, `mac`, `pc-linux` as reported by AppDetails; all supported platforms are recorded as separate `external_game_platforms` rows
- **Playtime:** Provided as a single total across all platforms. The adapter assigns this value to the `hours_played` of the highest-priority platform row in the order `pc-windows` → `mac` → `pc-linux`; all other platform rows for the same game receive 0
- **Achievements:** For games with playtime > 0 the adapter fetches per-game achievement stats and records `achievements_unlocked` and `achievements_total` on the `user_game_platforms` row; games with no playtime or no achievement schema leave both fields null
- **Store link:** `store_link` is the appid (the same value as `external_id`); enrichment copies it directly, no API call

### PSN

- **Auth:** NPSSO token exchanged for an access token; token expiry is detected and surfaced as a credential error
- **Library fetch:** Paginated API; the adapter re-chunks pages into batches of ≤10 for the callback
- **Rate limiting:** No published hard limit; the adapter applies a conservative request delay between pages
- **Platforms:** Derived from the `category` field in the API response — `ps4_game` maps to `playstation-4`, `ps5_native_game` maps to `playstation-5`. PSN creates one ExternalGame row per title ID, so the PS4 and PS5 versions of the same game appear as two separate ExternalGame rows, each with their own platform and playtime
- **Playtime:** Provided per title ID as an ISO 8601 duration string, parsed to hours
- **Store link:** `store_link` is a store concept ID; enrichment resolves it from the title ID via the catalog API (`GET /api/catalog/v2/titles/{titleId}/concepts`) using the user's NPSSO. Titles with no resolvable concept stay null

### GOG

- **Auth:** OAuth2; the adapter refreshes the access token using the stored refresh token before each fetch and saves the new tokens back to `user_sync_configs`
- **Library fetch:** Paginated API; the adapter re-chunks pages into batches of ≤10
- **Rate limiting:** Conservative request delay between pages
- **Platforms:** Reported per entry; mapped to canonical slugs
- **Playtime:** Not provided by the GOG API; always 0
- **Store link:** `store_link` is the product slug; enrichment resolves it from the numeric product id via the public `GET https://api.gog.com/products/{id}` endpoint (no auth — separate from the OAuth library fetch)

### Epic Games Store

- **Prerequisites:** Requires the `LEGENDARY_WORK_DIR` environment variable to be set to a writable directory. If unset, Epic sync is disabled entirely — the adapter returns an error immediately and the storefront is unavailable in the UI
- **Auth:** Managed by the Legendary CLI. The adapter restores an encrypted session state snapshot from `user_sync_configs.storefront_credentials` to disk, runs the CLI, then captures and re-encrypts the updated snapshot back to `storefront_credentials`
- **Library fetch:** `legendary list --json`; DLC entries are filtered out (identified by a non-empty `MainGameAppName`); the adapter chunks the output into batches of ≤10
- **Rate limiting:** Handled internally by the Legendary CLI
- **Platforms:** Epic does not expose per-game platform data; all entries are `pc-windows`
- **Playtime:** Not provided; always 0
- **Store link:** `store_link` is the product slug. The adapter captures each entry's `namespace` into `source_metadata` at Stage 1; enrichment looks that namespace up in the public `productmapping` dictionary (`GET https://store-content-ipv4.ak.epicgames.com/api/content/productmapping`, fetched once per group). Namespaces absent from the mapping stay null

### Humble Bundle

- **Auth:** A `_simpleauth_sess` session cookie, pasted by the user (programmatic login is gated by reCAPTCHA + 2FA, so server-side login is not viable). The cookie is verified on save against the order API and stored encrypted; expiry is surfaced as a credential error and prompts a re-paste
- **Library fetch:** Lists order gamekeys (`GET /api/v1/user/order`), then fetches each order's detail (`GET /api/v1/order/{gamekey}?all_tpkds=true`). A single failing order is logged and skipped so one bad order doesn't sink the sync. Qualifying subproducts are yielded in batches of ≤10
- **DRM-free only:** Only games the user can download directly from Humble are imported. A subproduct is a game iff it has a download whose platform is in the whitelist `{windows, mac, linux, android}` with a non-empty `download_struct[0].url.web`, and its `machine_name` is not in the launcher blocklist (`uplayclient`). Ebooks, audio, video, `asmjs`, and promo/info stubs are excluded by absence of a game-platform download. **Third-party keys (`tpkd_dict`) are never read**, so Steam-key-only titles are not imported
- **Rate limiting:** Conservative request delay (5 req/sec) between API calls
- **Platforms:** Mapped in-adapter — `windows`→`pc-windows`, `mac`→`mac`, `linux`→`pc-linux`, `android`→`android` (union across a subproduct's qualifying downloads). PC and Android editions are separate subproducts (distinct `machine_name`, shared title); the adapter emits both and the pipeline collapses them into one library entry with the union of platforms
- **Playtime:** Not provided by Humble; always 0
- **Store link:** none — there is no reliable public per-game store URL for the items ingested; `store_link` stays null and no link is rendered

### Microsoft Store (Xbox)

> **Status:** not yet implemented. The notes below record what a spike (issue #752) validated end-to-end against a live Microsoft account, including the auth chain, the ownership/play-data endpoints, and the platform model. Storefront key is `microsoft-store` (already seeded; display name "Microsoft Store"). Treat the residual unknowns at the end as implementation to-dos.

- **Auth:** OAuth2 + the Xbox Live token chain. No Azure/Microsoft app registration is required (a public client is used), consistent with the issue's constraint.
  - **Connect flow (GOG-style paste-the-URL):** use a public client with the **standard desktop (OOB) redirect** (`https://login.live.com/oauth20_desktop.srf`) and the **v2 Xbox scope** `XboxLive.signin offline_access`. After sign-in the auth code lands in the browser address bar (`…/oauth20_desktop.srf?code=…`), which the user pastes back — the same UX as GOG. **Do not** use the Xbox-app client's `ms-xal://` redirect (the SISU flow): browsers silently swallow that custom-scheme redirect, so it is unusable for a copy-paste connect. The deprecated legacy client `0000000048093EE3` + `MBI_SSL` scope no longer works (`/user/authenticate` returns 403 `X-Err=00000005`).
  - **Token chain (all Xbox calls are request-signed):** generate an ECDSA P-256 **proof key**; mint a **device token** (`device.auth.xboxlive.com`, `ProofOfPossession`); exchange the OAuth access token at `user.auth.xboxlive.com/user/authenticate` (**`RpsTicket` prefix `d=`** for the v2-scope token) for a user token; then mint an **XSTS** token per relying party at `xsts.auth.xboxlive.com/xsts/authorize` (include the device token). Every Xbox/Store request carries a `Signature` header (proof-of-possession): the byte layout is `version(4 BE) 0x00 ts(8 BE) 0x00 METHOD 0x00 path+query 0x00 authorization 0x00 body 0x00`, SHA-256'd, ECDSA-P256-signed, with the header = `base64(version(4) ‖ ts(8) ‖ r(32) ‖ s(32))` and `ts` a Windows FILETIME. The `Authorization` header for data calls is `XBL3.0 x=<uhs>;<xsts-token>`.
  - **Credential storage / rotation:** store the **refresh token and the proof key** (encrypted, in `user_sync_configs.storefront_credentials`). Refresh the access token and re-run the (cheap) token chain on each sync, persisting the rotated refresh token back — no recurring manual re-paste.
- **Library fetch:** two sources answer different questions; the front half merges them, the shared back half (external_games / IGDB / job pipeline) is unchanged.
  - **Ownership — `collections.mp.microsoft.com/v8.0/collections/b2bLicensePreview`** (relying party `http://licensing.xboxlive.com`). Authenticate with the delegated XSTS as `XBL3.0` **plus the `Signature` header**; **omit `beneficiaries`** (the X-token identifies the user). Body uses `entitlementFilters: ["*:Game", "*:Durable", "*:Pass", …]` (the format is `*:<ProductType>`, **not** `*:*:*`), `market: "neutral"`, `validityType: "All"`; omit `productSkuIds` to enumerate the whole library. Returns directly-owned **modern** Store entitlements with `productId`, `acquisitionType` (`Single` = purchase/redeem, `Recurring` = subscription/Game Pass, `Conditional` = Games-with-Gold) and `status`. The v9 `publisherQuery` endpoint is **partner-only** (returns 401 `PartnerXTokenNotProvided` to a consumer token) — do not use it.
    - **Caveat — Xbox 360-era purchases are not in `collections`.** Legacy Xbox 360 Marketplace purchases live in the retired Xbox Inventory service, so a 360-only (or Game Pass / free-only) account returns an empty `collections` result (a valid `200` with `{"items":[]}`). A collections-*only* "owned" definition would give such users an empty Xbox library, so titlehub is needed as the practical library source for legacy libraries.
  - **Play data / legacy library — `titlehub.xboxlive.com/users/xuid(<XUID>)/titles/titlehistory/decoration/detail,image,scid`** (relying party `http://xboxlive.com`). This is *play history* (titles the user has launched, including Xbox 360 back-compat titles), not ownership. The `detail` decoration is rich: title, `devices[]`, genres, publisher, release date, art (`images[]`), `titleHistory.lastTimePlayed`, and — critically — **`detail.availabilities[].ProductId` and `.Platforms`**. That ProductId **is the `titleId ↔ ProductId` bridge** the issue worried about: no separate bridging step is needed.
- **Catalog resolution — `displaycatalog.mp.microsoft.com/v7.0/products?bigIds=<ids>&market=…&languages=…&fieldsTemplate=Details`** (anonymous, no auth). Resolves a ProductId to title, art, and platform availability via `DisplaySkuAvailabilities[].Sku.Properties.Packages[].PlatformDependencies[].PlatformName`.
- **Platforms:** `Windows.Desktop` → `pc-windows`; `Windows.Xbox` plus the titlehub `devices[]` generations (`Xbox360`/`XboxOne`/`XboxSeries`) → `xbox-360` / `xbox-one` / `xbox-series`. All four already map to `microsoft-store` in the seed. Multi-platform titles are real (e.g. back-compat titles list `["Xbox360","XboxOne","XboxSeries"]`); record each as a separate `external_game_platforms` row.
- **Playtime:** titlehub provides `lastTimePlayed` (a timestamp) but **not** total hours, so `hours_played` is unavailable from Xbox; only a last-played signal is.
- **Store link:** the Store product page is derivable from the ProductId (`https://www.microsoft.com/store/productId/<ProductId>`); ProductId comes from collections (owned) or titlehub `detail.availabilities` (played).
- **Residual unknowns to validate during implementation (need an account with modern Store purchases):** the non-empty `b2bLicensePreview` item shape was only confirmed to a successful *empty* `200`; whether Game Pass entitlements appear in `collections` and with which `acquisitionType` (the issue flagged they may be reported as `Single`, i.e. indistinguishable from purchases); and refresh-token rotation across real time. The product decision of whether the Xbox "library" is owned-only (collections), played (titlehub), or a union also remains open — note that owned-only yields nothing for legacy/360 accounts.

---

## User Interface

The sync UI has two levels: a hub page that shows all storefronts at a glance, and a per-storefront detail page where the user can configure, monitor, and act on sync results.

### Navigation

The global navigation shows an aggregate count of `pending_review` items across all storefronts. Tapping it navigates to the sync hub page.

### Sync Hub Page

A grid of storefront cards — one per supported storefront. Each card shows:

- Platform name and icon; the name links to that storefront's detail page
- Connection status (Connected / Credentials Error / Not Configured)
- Last synced timestamp
- Count of external games currently available on that storefront (`is_available = true`), including skipped games; shown as a simple number (e.g. "482 games")
- Pending review count badge; clicking it navigates to that storefront's detail page, anchoring to the Needs Review section where possible
- A Sync Now button to trigger a manual sync without navigating into the detail page

### Platform Detail Page

#### Header

The storefront name and a connection status badge. The badge is one of: **Connected**, **Credentials Error**, or **Not Configured**. Clicking the badge toggles the Connection & Settings section open or closed.

#### Connection & Settings Section

Collapsible. Collapsed by default when the connection is working; expanded by default when the status is Credentials Error or Not Configured.

Contains:
- Storefront-specific credential input (e.g. API key for Steam, NPSSO token for PSN, OAuth flow for GOG and Epic)
- Sync frequency setting (Manual / Hourly / Daily / Weekly)
- Disconnect action

#### Progress Box

Shown only while a sync job is active. Displays:

- Total number of games found on the storefront so far; during Stage 1 this count grows as games are fetched; once Stage 1 completes it is fixed for that run
- Live counts per state: matched, needs review, skipped, failed, and still processing

When the job reaches a terminal state the progress box collapses to a single summary line showing the outcome and timestamp, or disappears entirely.

Games that are currently in-flight (being processed by Stage 2 or Stage 3) do not appear in the External Games section — only the counts in the progress box reflect their existence until they settle into a stable state.

#### External Games Section

A permanent view of all external games for this storefront, organised into four groups. Only games in stable states are shown here.

| Group | Condition | Default | Actions |
|---|---|---|---|
| **Needs Review** | `pending_review` job item | Expanded | Pick IGDB match, Skip |
| **Failed** | Permanent Stage 2 or Stage 3 failure | Expanded | Retry (per game), Retry All |
| **Matched** | `resolved_igdb_id` set, Stage 3 complete | Collapsed | Change match |
| **Skipped** | `is_skipped = true` | Collapsed | Unskip |

The Needs Review group is the most prominent — these games are blocking the job from completing and require user action.

The **Matched** group displays the external game title alongside a **Platform** column showing the platforms associated with that external game (sourced from `external_game_platforms`). Each platform is rendered as its canonical slug (e.g. `pc-windows`, `playstation-5`). When an external game has multiple platforms, all are shown in the same row.

#### Sync History

A log of past sync runs for this storefront. Each entry shows:

- Timestamp and outcome (`completed` or `failed`)
- A summary: counts of games added, removed, and status-changed for that run
- An expandable changelog: the individual game titles behind those counts, grouped by change type (added, removed, status changed)

The changelog is collapsed by default — the summary counts are visible immediately. Expanding reveals the per-game detail. The history does not reproduce the full per-game processing trace — it is a human-readable record of what changed in the user's library as a result of each sync run.
