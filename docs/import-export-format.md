# Nexorious Library Interchange Format

**Status:** specification (source of truth), implemented as of #828. This
document defines what export produces and import accepts.

This document is the authoritative reference for the **Nexorious JSON
import/export format** (`version` `2.0`). It serves two audiences:

- **Developers** learning what the format looks like and why it is shaped this
  way.
- **Future agents** treating this file as the source of truth when bringing the
  export and import code into compliance.

For the unrelated one-off Darkadia CSV migration, see
[`darkadia-import.md`](./darkadia-import.md). This document covers only the
native Nexorious JSON format.

## Purpose and scope

The interchange format exists to **move a user's library between Nexorious
instances or users without losing user-owned data.** It is deliberately *not* a
full backup — comprehensive, byte-for-byte backups are handled by the separate
backup system. This format carries the user's curation, not the application's
internal state.

The guiding principle:

> **A game's identity in the file is its IGDB ID plus a human-readable title.
> Everything else about the game (cover art, description, genre, developer,
> publisher, release date, ratings, HowLongToBeat, …) is metadata owned by IGDB
> and is re-hydrated from `igdb_id` on import.** The file carries only the IGDB
> ID, the title, and the data the *user* has added.

Consequences of that principle:

- The file never contains IGDB-sourced game metadata. It would be redundant,
  stale, and bloated.
- **Import requires IGDB to be configured.** Because the game record is
  re-hydrated from IGDB by ID, an import with no IGDB client cannot construct a
  usable game and is rejected with a clear error (mirroring the Darkadia
  importer's hard IGDB requirement). `title` is retained only as a
  human-readable label and disambiguation aid; it is not a substitute for IGDB
  metadata.

## Document structure

A library export is a single JSON document with a minimal envelope and a flat
array of game entries.

```json
{
  "format": "nexorious-library",
  "version": "2.0",
  "exported_at": "2026-06-05T14:30:45Z",
  "games": [
    {
      "igdb_id": 1234,
      "title": "Hollow Knight",
      "play_status": "completed",
      "personal_rating": 5,
      "is_loved": true,
      "personal_notes": "One of the best metroidvanias ever made.",
      "created_at": "2025-01-02T10:00:00Z",
      "updated_at": "2025-06-01T12:00:00Z",
      "platforms": [
        {
          "platform": "pc-windows",
          "storefront": "steam",
          "ownership_status": "owned",
          "acquired_date": "2024-12-25",
          "hours_played": 42.5
        }
      ],
      "tags": [
        { "name": "metroidvania", "color": "#7C3AED" }
      ]
    }
  ]
}
```

### Envelope

The envelope is intentionally minimal. Everything that was derivable,
instance-specific, or vestigial in the older `1.2` format has been removed.

| Field         | Type   | Required | Notes                                                                 |
|---------------|--------|----------|-----------------------------------------------------------------------|
| `format`      | string | yes      | Constant `"nexorious-library"`. Identifies the document kind.          |
| `version`     | string | yes      | Constant `"2.0"`. See [Versioning](#versioning).                       |
| `exported_at` | string | yes      | RFC 3339 timestamp (UTC) of when the file was produced. Informational. |
| `games`       | array  | yes      | Array of [game entries](#game-entry). May be empty.                   |

Removed from the previous (`1.2`) envelope and **not** present in `2.0`:

- `user_id` — instance/user-specific; ignored on import; a privacy leak in a
  portable file.
- `total_games`, `total_wishlist` — derivable from the arrays.
- `export_stats` — fully derivable; belongs in the UI, not the interchange file.
- `wishlist` — a dead field (always empty; no wishlist feature exists).

### Game entry

| Field             | Type           | Required | Notes                                                                                 |
|-------------------|----------------|----------|---------------------------------------------------------------------------------------|
| `igdb_id`         | integer        | **yes**  | Non-zero IGDB game ID. The canonical game identity; used to re-hydrate metadata.       |
| `title`           | string         | **yes**  | Human-readable label / disambiguation aid. Not a metadata source.                     |
| `play_status`     | string \| null | no       | One of the [play status values](#play-status-values). Omitted/null = unset.            |
| `personal_rating` | integer \| null| no       | **Integer 1–5.** Omitted/null = unrated.                                               |
| `is_loved`        | boolean        | no       | Defaults to `false` when absent.                                                       |
| `personal_notes`  | string \| null | no       | Free-text user notes.                                                                  |
| `created_at`      | string \| null | no       | RFC 3339. When the user added the game. Preserved on import for new games.             |
| `updated_at`      | string \| null | no       | RFC 3339. Last user modification. Preserved on import for new games.                   |
| `platforms`       | array          | no       | Array of [platform entries](#platform-entry). May be empty/absent.                     |
| `tags`            | array          | no       | Array of [tag entries](#tag-entry). May be empty/absent.                               |

Removed from the previous (`1.2`) game entry and **not** present in `2.0`:

- `release_year` — IGDB-sourced (re-hydrated from `igdb_id`). It was also the
  source of a real round-trip bug: export wrote `release_year` (an integer)
  while import only read `release_date` (an RFC 3339 string), so the value was
  silently dropped on re-import.
- game-level `hours_played` — redundant. Per-platform `hours_played` is the
  source of truth; the game-level total is derivable by summation.
- `description`, `genre`, `developer`, `publisher`, `cover_art_url`,
  `rating_average` — IGDB-sourced metadata that the old import accepted as a
  fallback but the old export never actually produced. Re-hydrated from IGDB.

### Platform entry

A platform entry records that the user owns the game on a given
`(platform, storefront)` combination, plus the user-supplied facts about that
copy.

| Field              | Type            | Required | Notes                                                                              |
|--------------------|-----------------|----------|------------------------------------------------------------------------------------|
| `platform`         | string          | **yes**  | Canonical platform slug (e.g. `pc-windows`). Must exist in destination seed data.   |
| `storefront`       | string \| null  | no       | Canonical storefront slug (e.g. `steam`). See handling of unknown values below.     |
| `ownership_status` | string \| null  | no       | One of the [ownership status values](#ownership-status-values). Default `owned`.    |
| `acquired_date`    | string \| null  | no       | Date-only `YYYY-MM-DD`. When the user acquired this copy.                            |
| `hours_played`     | number \| null  | no       | Hours played on this platform (float).                                              |

Slug resolution on import:

- **Unknown `platform`** → the platform entry is skipped (the destination must be
  seeded with the platform first).
- **Unknown or null `storefront`** → the entry is stored with a null storefront
  rather than being rejected.

Removed from the previous (`1.2`) platform entry and **not** present in `2.0`:

- `platform_id` / `storefront_id` — pure duplicates of the slug (`platform_id`
  equalled `platform_name`, `storefront_id` equalled `storefront_name`). The
  format now uses a single canonical key each: `platform` and `storefront`.
- `store_game_id`, `store_url` — denormalized store-linkage fields that were only
  ever written by the manual import path and never by sync. Store linkage
  authoritatively lives in the `external_games` table (the sync domain), not on
  the ownership record. These columns have been dropped from the schema
  (issue #827).
- `is_available` — sync-managed availability *state*; re-derived on the next
  sync run. Imported rows default to available.
- `external_game_id` — a foreign key into the *per-instance* `external_games`
  table. Its value is meaningless on a different instance/user, so it must never
  be serialized. Sync re-establishes this link automatically (see
  [Relationship to sync](#relationship-to-sync)).
- `sync_from_source` — instance-local provenance with no meaning on the
  destination.

### Tag entry

| Field   | Type           | Required | Notes                                         |
|---------|----------------|----------|-----------------------------------------------|
| `name`  | string         | **yes**  | Tag name. Matched case-insensitively.          |
| `color` | string \| null | no       | Hex color (e.g. `#7C3AED`).                    |

On import, tags are **found-or-created per importing user**, matched
case-insensitively by `name`. The `color` is applied only when a new tag is
created; an existing tag's color is left untouched.

## Enumerations

These values are the canonical enums defined in
[`internal/enum/enum.go`](../internal/enum/enum.go). Importers must validate
against them; exporters only ever emit them.

### Play status values

`not_started`, `in_progress`, `completed`, `mastered`, `dominated`, `shelved`,
`dropped`, `replay`

### Ownership status values

`owned`, `borrowed`, `rented`, `subscription`, `no_longer_owned`

## Date and number formats

- **Timestamps** (`exported_at`, `created_at`, `updated_at`): RFC 3339 in UTC,
  e.g. `2026-06-05T14:30:45Z`.
- **Dates** (`acquired_date`): date-only `YYYY-MM-DD`, e.g. `2024-12-25`.
- **`personal_rating`**: integer in the inclusive range 1–5.
- **`hours_played`**: non-negative number (float allowed), e.g. `42.5`.

## Import semantics

Import is **non-destructive and additive (merge)**. The goal is "move a library
without losing data," not "replace the destination library."

Per game entry:

1. **Validation.** `igdb_id` must be present and non-zero, and an IGDB client
   must be configured. Otherwise the item fails (or, for the IGDB requirement,
   the whole import is rejected up front).
2. **Game record.** The game is re-hydrated from IGDB by `igdb_id` (cover art,
   metadata, etc.). `title` from the file is used only as a label/fallback for
   display and matching; it does not override IGDB.
3. **Existing user game.** If the importing user already owns the game
   (`user_id` + `game_id`), the existing `user_games` row's user fields
   (`play_status`, `personal_rating`, `is_loved`, `personal_notes`) are **left
   unchanged** — they are never overwritten by an import.
4. **New user game.** If the user does not yet own the game, a new `user_games`
   row is created with the file's user fields, preserving `created_at` /
   `updated_at` when present (otherwise stamped at import time).
5. **Platforms.** Merged by `(platform, storefront)`. Existing pairs are left
   as-is; new pairs are inserted. Unknown platforms are skipped; unknown
   storefronts are stored as null.
6. **Tags.** Merged by name (case-insensitive), found-or-created per user, then
   linked to the game if not already linked.

Unknown/extra fields in the input are ignored (forward tolerance). Missing
optional fields take their documented defaults.

## Relationship to sync

Dropping store-linkage and sync-state fields is safe because storefront sync is
self-healing:

- Sync reconciles `user_game_platforms` rows purely on the
  `(user_game_id, platform, storefront)` tuple — it does not match on
  `store_game_id` or `external_game_id`.
- On the first sync run after an import, sync performs an unconditional update
  that **backfills `external_game_id`** to the correct local `external_games`
  row for any matching platform entry, with no duplicate rows (guaranteed by a
  unique index plus `ON CONFLICT DO NOTHING`).

So an imported library that records only `(platform, storefront, ownership,
acquired_date, hours)` is fully re-linked to the destination instance's
storefront records on the next sync, without carrying any instance-local
identifiers in the file. See [`sync.md`](./sync.md) § "Manually added games".

## CSV export (separate convenience format)

Alongside the JSON interchange format, Nexorious can export a **CSV** file. CSV
is a **human-oriented, export-only convenience format**: it is *not* governed by
this specification, has **no import counterpart**, and is therefore **not
round-trippable**. Its columns (`title`, `igdb_id`, `play_status`,
`personal_rating`, `is_loved`, `hours_played`, `personal_notes`, `platforms`,
`tags`, `created_at`, `updated_at`) flatten per-platform data into
semicolon-joined cells and a game-level `hours_played` total for readability. The
`release_year` column was removed (release info is IGDB-sourced). Use the JSON
format for moving a library between instances.

## Versioning

- The current and only supported version is **`2.0`**.
- Import **accepts only `version` `"2.0"`** and rejects any `1.x` file with a
  clear error message. The legacy `1.2` format is intentionally unsupported:
  it carried different (and partly broken) fields, and durable backups are the
  backup system's responsibility, not this format's.
- Future backward-incompatible changes increment the major version; additive,
  backward-compatible changes may increment a minor version.

## Differences from the legacy 1.2 format (summary)

| Area      | `1.2` (legacy)                                                                 | `2.0` (this spec)                                  |
|-----------|-------------------------------------------------------------------------------|----------------------------------------------------|
| Envelope  | `export_version`, `export_date`, `user_id`, `total_games`, `total_wishlist`, `export_stats`, `games`, `wishlist` | `format`, `version`, `exported_at`, `games`        |
| Game id   | `igdb_id` + `title` + IGDB metadata fields                                     | `igdb_id` + `title` only (IGDB re-hydrated)         |
| Release   | `release_year` (broken round-trip vs. import's `release_date`)                 | removed (IGDB-sourced)                              |
| Hours     | game-level `hours_played` **and** per-platform                                 | per-platform only                                  |
| Rating    | exported int, imported float (drift)                                           | integer 1–5                                        |
| Platform  | `platform_id`+`platform_name`, `storefront_id`+`storefront_name`, `store_game_id`, `store_url`, `is_available`, … | `platform`, `storefront`, `ownership_status`, `acquired_date`, `hours_played` |
| Tags      | `name`, `color`                                                                | `name`, `color` (unchanged)                        |

## Related issues

- **#827** — dropped the denormalized `store_game_id` and `store_url` columns from
  `user_game_platforms` (schema change implied by this format).
