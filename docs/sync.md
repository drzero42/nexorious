# Sync Process

This document is the source of truth for how Nexorious syncs a user's game library from an external storefront. It describes how the process **should** work. It is kept up to date as the process evolves and is intended for both humans and coding agents working on the sync system.

---

## Overview

Syncing imports a user's game library from an external storefront (Steam, PSN, GOG, or Epic Games Store) into Nexorious. The process fetches the library, matches each game to an entry in the IGDB game database, and creates or updates the user's Nexorious library accordingly.

The sync pipeline is designed to be:

- **Consistent** — the same general process applies to all storefronts; per-storefront differences are isolated to adapter code
- **Resilient** — transient failures are retried automatically; anything the process cannot resolve on its own — whether from an API failure or an ambiguous match — is routed to the user without blocking the rest of the sync
- **Non-destructive** — ownership and playtime are never downgraded; existing data is only improved

---

## Glossary

| Term | Meaning |
|---|---|
| **Storefront** | An external game store: Steam, PSN, GOG, or Epic Games Store |
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
| `external_games` | One row per user + storefront + game; persists across sync runs |
| `external_game_platforms` | Platform slugs for each ExternalGame |
| `jobs` | One row per sync run; tracks status and lifecycle |
| `job_items` | One row per game per sync run; tracks matching progress |
| `games` | IGDB master catalogue; new rows are inserted when a match is found |
| `user_games` | The user's canonical library; one row per user + IGDB game |
| `user_game_platforms` | One row per user + game + platform + storefront combination |

### Key relationships

`user_games` holds one row per user per IGDB game, regardless of how many storefronts or platforms the game was found on. Each `user_game_platforms` row points back to the specific `external_games` row that created it via `external_game_id`. This means a single `user_games` row can have multiple `user_game_platforms` rows pointing to different `external_games` rows — which is expected and correct for storefronts that create multiple ExternalGame rows for the same game.

For example, PSN creates one ExternalGame row per title ID. The PS4 and PS5 versions of the same game are separate title IDs, so they produce separate ExternalGame rows and separate `user_game_platforms` rows — but both point to the same `user_games` row.

### Playtime

Playtime is stored at the `user_game_platforms` level (`hours_played`), not at the `user_games` level. The total playtime for a game is the sum of hours across all its platform rows.

> **Note:** `user_games.hours_played` currently exists as a stored column but should be refactored to a calculated sum of `user_game_platforms.hours_played`. See [#613](https://github.com/drzero42/nexorious/issues/613).

Not all storefronts provide playtime. When a storefront does not provide playtime, `hours_played` is 0 for that platform row. Playtime is never decreased — a platform row's `hours_played` is only updated when the incoming value is greater than the stored value.

---

## Architecture

The sync pipeline has three stages. Each stage is implemented as a River worker job. A shared worker library handles the mechanics common to all storefronts — batching, rate limiting, database upserts, and job tracking — while each storefront implements a standard adapter interface that owns its own auth and API communication.

```mermaid
flowchart TD
    A([Trigger: manual or scheduled]) --> B

    subgraph Stage1["Stage 1 — Fetch"]
        B[DispatchSyncWorker\nrecords sync_started_at] --> C
        C[Adapter fetches library\nin batches of ≤10] --> D
        D[Upsert external_games\n+ external_game_platforms] --> E
        E[Enqueue one Stage 2 job\nper game in batch] --> F{More batches?}
        F -->|yes| C
        F -->|no| G[Timestamp sweep:\nmark missing games is_available=false]
    end

    subgraph Stage2["Stage 2 — IGDB Match"]
        H{Already resolved\nor skipped?} -->|yes| L
        H -->|no| I[Search IGDB\nscore candidates]
        I --> J{Clear winner\nscore ≥ 0.85?}
        J -->|yes| K[Set resolved_igdb_id\non external_game]
        K --> L[Enqueue Stage 3]
        J -->|no, or retries exhausted| M([pending_review:\nawait user action])
    end

    subgraph Stage3["Stage 3 — User Game Write"]
        N{is_skipped?} -->|yes| O[Update\nexternal_game.updated_at]
        N -->|no| P[Upsert user_games]
        P --> Q[Upsert user_game_platforms\nper platform\nwith ownership rank guard]
        Q --> O
    end

    subgraph UserAction["User Action"]
        M --> R{User decision}
        R -->|picks IGDB match| S[Set resolved_igdb_id\nenqueue Stage 3]
        R -->|skips game| T[Set is_skipped=true\nmark item skipped]
    end

    E --> H
    L --> N
    S --> N
```

### Shared worker library responsibilities

- Recording `sync_started_at` at the beginning of a sync run
- Calling the adapter's batch callback and iterating until the library is fully fetched
- Applying rate limiting between API calls
- Upserting `external_games` and `external_game_platforms` after each batch
- Enqueuing Stage 2 jobs after each batch
- Running the timestamp sweep at the end of the fetch phase
- Failing the job and cancelling pending items on credential errors

### Storefront adapter responsibilities

- All authentication mechanics (token refresh, CLI state management, credential expiry detection)
- Signalling credential errors to the shared library
- Yielding games in batches of ≤10 via a callback

### Adapter interface

Each game yielded by the adapter provides:

| Field | Type | Notes |
|---|---|---|
| `ExternalID` | string | Storefront-specific game identifier |
| `Title` | string | Game name as reported by the storefront |
| `PlaytimeHours` | int | Hours played; 0 means not provided by this storefront |
| `RawPlatforms` | []string | Platform names in storefront-specific format; resolved to canonical slugs by the library |
| `OwnershipStatus` | string | `owned`, `subscription`, etc. |
| `IsSubscription` | bool | True if the game is accessed via a subscription service |

---

## Stage 1 — Fetch

The `DispatchSyncWorker` runs once per sync job. It:

1. Records `sync_started_at`
2. Calls the storefront adapter which fetches the library and yields games in batches of ≤10
3. After each batch:
   - Upserts each game into `external_games`, always setting `updated_at = now()` and `is_available = true`
   - Upserts platform rows into `external_game_platforms`; removes any platform rows for that game that were not in this batch
   - Enqueues one Stage 2 job per game in the batch
4. After all batches complete, runs a timestamp sweep: any `external_games` row for this user and storefront where `updated_at < sync_started_at` is marked `is_available = false` — these are games that were not seen in this sync run and have been removed from the user's library

If a credential error occurs at any point, the job is marked `failed` and all pending job_items are cancelled. Any `external_games` rows already upserted in this run are kept.

---

## Stage 2 — IGDB Match

One `IGDBMatchWorker` job runs per game. River handles retries with exponential backoff for transient IGDB API failures.

1. **Already resolved or skipped?** If `external_game.resolved_igdb_id` is set, or `is_skipped` is true, route directly to Stage 3 — no IGDB search is performed. On subsequent syncs, most games will take this path.
2. **Search IGDB** for the game title; score each candidate using fuzzy title matching
3. **Auto-resolve** if the best candidate scores ≥ 0.85 and has a clear margin (> 0.01) over the second-best: set `resolved_igdb_id` on the `external_game` and enqueue Stage 3
4. **pending_review** if no clear winner is found, or if IGDB API calls fail after all River retries are exhausted: store the candidates in `job_item.igdb_candidates` and mark the item `pending_review` for the user to resolve

### Title matching

Before searching, titles are normalised (trademark symbols removed, diacritics folded, common suffixes like "GOTY" expanded, etc.). Candidates are scored using a weighted combination of fuzzy matching algorithms. The auto-resolve threshold is 0.85 with a tie-breaking margin of 0.01.

---

## Stage 3 — User Game Write

One `UserGameWorker` job runs per game, enqueued by Stage 2 or by a user action.

1. If `is_skipped` is true: skip steps 2 and 3
2. Upsert `user_games`: one row per user + IGDB game ID
3. For each platform in `external_game_platforms`:
   - Upsert `user_game_platforms` with conflict key `(user_game_id, platform, storefront)`
   - On conflict: apply the ownership rank guard (never downgrade ownership status); update `hours_played` only if the incoming value is greater
   - Set `external_game_id` to the specific ExternalGame row that produced this platform entry
4. Update `external_game.updated_at` — always, whether the game was skipped or not

### Ownership rank guard

Ownership statuses have a fixed rank. A stored status is never replaced by one of lower rank:

```
owned  >  borrowed / rented  >  subscription  >  no_longer_owned
```

---

## Job Lifecycle

```mermaid
stateDiagram-v2
    [*] --> pending : triggered (manual or scheduled)
    pending --> processing : DispatchSyncWorker starts
    processing --> processing : items in pending_review\n(waits indefinitely for user)
    processing --> completed : all items completed or skipped
    processing --> failed : credential error or\nunrecoverable dispatch failure
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

The user can either select a game from the suggested `igdb_candidates` or perform their own IGDB search and choose any result. Once a match is chosen, `resolved_igdb_id` is set on both the job_item and the `external_game`, and a Stage 3 job is enqueued immediately.

### Skipping a game

The user marks a game as ignored. `is_skipped` is set to `true` on the `external_game` and the job_item is marked `skipped`. No Stage 3 job is created. On future syncs, Stage 2 routes the game directly to Stage 3, which updates `external_game.updated_at` and does nothing else.

### Unskipping a game

The user removes the skip. `is_skipped` is cleared. A new job_item is created and a Stage 2 job is enqueued immediately to begin IGDB matching.

### Rematching a game

The user replaces an existing IGDB match with a different one. `external_game.resolved_igdb_id` is updated and a Stage 3 job is enqueued immediately to update the user_game and platform associations.

---

## Credential Errors

All storefronts expose credential problems through a unified `credentials_error` flag in their status response. This covers expired tokens, decryption failures, and any other auth issue.

When a credential error occurs mid-sync, the job is marked `failed` and all pending job_items are cancelled. The user must reconfigure their credentials before triggering a new sync.

Credentials are stored encrypted at rest in `user_sync_configs.storefront_credentials`. Decryption happens in memory during Stage 1 only; plaintext is never persisted. On decryption failure, the encrypted bytes are left untouched in the database — they are never cleared.

---

## Scheduled Sync

A periodic worker checks `user_sync_configs` for all users where the sync frequency is not `manual` and the last sync was more than the configured interval ago (hourly / daily / weekly). For each, it creates a Job and enqueues a Stage 1 run — provided no active job already exists for that user and storefront. All four storefronts support scheduled sync.

---

## Storefront Adapters

All adapters implement the same interface. The differences below are the only places where storefront-specific knowledge lives.

### Steam

- **Auth:** API key + Steam ID; static credentials, no refresh needed
- **Library fetch:** A single API call returns the full library. The adapter chunks the list into batches of ≤10. For each batch, it makes one AppDetails API call per game to resolve platform availability, then yields the enriched batch via the callback
- **Rate limiting:** A token bucket enforces a minimum delay between AppDetails calls. On a 429 response, the adapter backs off and retries. Rate limiting is handled consistently with the shared library's backoff interface
- **Platforms:** `pc-windows`, `mac`, `pc-linux` as reported by AppDetails; mapped to canonical slugs
- **Playtime:** Provided as total playtime across all platforms (not per platform). Written only to the highest-priority platform row in the order `pc-windows` → `mac` → `pc-linux`; all other platform rows for the same game receive 0

### PSN

- **Auth:** NPSSO token exchanged for an access token; token expiry is detected and surfaced as a credential error
- **Library fetch:** Paginated API; the adapter re-chunks pages into batches of ≤10 for the callback
- **Rate limiting:** No published hard limit; the adapter applies a conservative request delay between pages
- **Platforms:** Derived from the `category` field in the API response — `ps4_game` maps to `playstation-4`, `ps5_native_game` maps to `playstation-5`. PSN creates one ExternalGame row per title ID, so the PS4 and PS5 versions of the same game appear as two separate ExternalGame rows, each with their own platform and playtime
- **Playtime:** Provided per title ID as an ISO 8601 duration string, parsed to hours

### GOG

- **Auth:** OAuth2; the adapter refreshes the access token using the stored refresh token before each fetch and saves the new tokens back to `user_sync_configs`
- **Library fetch:** Paginated API; the adapter re-chunks pages into batches of ≤10
- **Rate limiting:** Conservative request delay between pages
- **Platforms:** Reported per entry; mapped to canonical slugs
- **Playtime:** Not provided by the GOG API; always 0

### Epic Games Store

- **Auth:** Managed by the Legendary CLI. The adapter restores an encrypted session state snapshot from `user_sync_configs` to disk, runs the CLI, then captures and re-encrypts the updated snapshot back to the database
- **Library fetch:** `legendary list --json`; DLC entries are filtered out (identified by a non-empty `MainGameAppName`); the adapter chunks the output into batches of ≤10
- **Rate limiting:** Handled internally by the Legendary CLI
- **Platforms:** Epic does not expose per-game platform data; all entries are `pc-windows`
- **Playtime:** Not provided; always 0
