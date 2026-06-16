# vglist Library Import

This document is the source of truth for how Nexorious imports a game library from a **vglist library export**. It describes the vglist export format and how that data **should** map into Nexorious during import. It is intended for both humans who need to understand the format and coding agents who need to know what to build (or verify) without being tied to any specific implementation.

[vglist](https://github.com/connorshea/vglist) is an open-source, self-hosted game-tracking web app powered by Wikidata. A user can export their entire library from **Settings → Export Library**, which produces a single **JSON** file. This importer exists so a vglist user can bring that library into Nexorious with as little loss as possible. Like Darkadia, it is a **one-off migration path**, not a recurring sync — there is no persistent connection, no incremental re-import, and no state kept "for next time".

It runs on the shared, source-neutral import pipeline (the same `jobs` / `job_items` machinery as Darkadia). The only vglist-specific code is the **mapper**: `Parse(raw []byte) ([]importmodel.Game, error)` plus a signature validator. Everything after the mapper — IGDB matching, `pending_review`, finalising into `user_games` + `user_game_platforms`, additive merge, job history — is identical for every source.

---

## Overview

A vglist export is a JSON object wrapping the user's library entries in a `games` array (see [Part 1](#part-1--the-vglist-export-format)). Each entry carries the game's title, an optional Wikidata id (**not** used for matching — see below), hours played, a completion status, a 0–100 rating, optional start/completion dates, free-text comments, a replay count, and two independent sets: the **platforms** the user owns the game on and the digital **stores** they own it through.

The import is designed to be:

- **Faithful** — it preserves real user data (rating, completion status, comments, playtime, store provenance) rather than discarding it.
- **Non-destructive** — it merges into the existing library; it never clears it and never overwrites curation the user already did.
- **Honest about loss** — where Nexorious cannot represent something (the 0–100 rating granularity, an unmapped Wikidata platform label, an unrecognised store), the behaviour is defined and documented rather than silent.

Identifying games requires IGDB. **The import is blocked unless IGDB is configured**, because every game is matched by **title** and without IGDB every game would land unmatched.

---

## Glossary

| Term | Meaning |
|---|---|
| **Library entry** | One object in the export array: one game plus that user's data for it (vglist calls the underlying record a *GamePurchase*). |
| **Platform** | A platform the user owns the game on. vglist platform names are free-form **Wikidata English labels** (e.g. `"Microsoft Windows"`, `"PlayStation 4"`), not a fixed enum. |
| **Store** | A digital storefront the user owns the game through (e.g. Steam, GOG). vglist store names are **arbitrary admin-entered strings**, not a fixed list. Platforms and stores are two independent sets on the entry — vglist does **not** pair a specific store with a specific platform. |
| **IGDB** | The canonical game database Nexorious uses to identify games. Required for this import. |
| **pending_review** | The state in which a game's IGDB match was ambiguous and the user must pick a candidate (or skip). Shared with the sync system. |

---

## Part 1 — The vglist export format

The export is produced by vglist's `exportLibrary` GraphQL mutation (`app/graphql/mutations/users/export_library.rb`); the frontend downloads the returned JSON verbatim. The root is a **JSON object** with a `user` block and a `games` array — **one object per library entry** under `games`. An empty library exports as `{ "user": …, "games": [] }`.

The parser keys on the first non-space byte: `{` selects the wrapper-object form (the real export); a bare `[` top-level array of entries is also accepted for back-compat (older captures / hand-rolled files). Either way the per-entry shape below is identical.

### Export shape

```json
{
  "user": { "id": 2882, "username": "drzero" },
  "games": [
    {
      "game": { "id": 1, "name": "Half-Life 2", "wikidata_id": 193581 },
      "hours_played": 12.5,
      "completion_status": "completed",
      "rating": 95,
      "start_date": "2024-01-10",
      "completion_date": "2024-02-01",
      "comments": "Great game.",
      "replay_count": 1,
      "platforms": [ { "id": 3, "name": "Microsoft Windows" }, { "id": 7, "name": "Linux" } ],
      "stores": [ { "id": 2, "name": "Steam" } ]
    },
    {
      "game": { "id": 42, "name": "Some Obscure Game", "wikidata_id": null },
      "hours_played": null,
      "completion_status": "unplayed",
      "rating": null,
      "start_date": null,
      "completion_date": null,
      "comments": "",
      "replay_count": 0,
      "platforms": [],
      "stores": []
    }
  ]
}
```

The `user` block is ignored by the importer; only `games` is read.

### Field reference

| JSON key | Type | Notes |
|---|---|---|
| `game.id` | int | vglist-internal id. Unused. |
| `game.name` | string | The game title. Used for IGDB matching. Always present (DB `NOT NULL`). |
| `game.wikidata_id` | int or null | Wikidata QID number. **Not used** for matching (see below). |
| `hours_played` | number or null | DB `numeric(10,1)` — a decimal with one fractional digit. |
| `completion_status` | string or null | One of the seven enum names below, or null. |
| `rating` | int or null | **0–100**, integer only (vglist validates `only_integer`, `0..100`). |
| `start_date` | `YYYY-MM-DD` or null | Date the user started the game. |
| `completion_date` | `YYYY-MM-DD` or null | Date the user completed the game. |
| `comments` | string | DB `text NOT NULL`, defaults to `""`. |
| `replay_count` | int | DB default `0`, `NOT NULL`, `>= 0`. |
| `platforms` | array of `{id, name}` | Possibly empty. `name` is a free-form Wikidata label. |
| `stores` | array of `{id, name}` | Possibly empty. `name` is an arbitrary string. |

### `completion_status` vocabulary

vglist's enum (`app/models/game_purchase.rb`), serialized as the string key:

```
unplayed  in_progress  dropped  completed  fully_completed  not_applicable  paused
```

### What vglist does not provide

- **No IGDB ids.** vglist's only external id is Wikidata, and per the epic, foreign ids are **not** a match signal in v1. Matching is title-based.
- **No "loved"/favourite flag.**
- **No "date added" to the library.** Only `start_date` and `completion_date` exist; there is no field for when the entry was created.
- **No tags.**
- **No per-platform store pairing.** `platforms` and `stores` are two independent sets.

---

## Part 2 — How it maps into Nexorious

### Pipeline shape

The mapper parses the JSON into one canonical `importmodel.Game` per entry. The shared pipeline then matches each title to IGDB (auto-resolve or `pending_review`), and on finalise writes `user_games` + `user_game_platforms` with additive merge semantics. None of that is vglist-specific.

### Prerequisite: IGDB is required

Every entry is identified by title, so the upload is refused up front with a clear error if IGDB is not configured — exactly as Darkadia does.

### Game identity and matching

`game.name` (trimmed) is the title handed to the IGDB matcher. `game.wikidata_id` is ignored. An entry with an empty title is skipped defensively (vglist never emits one).

### Game-level field mapping

| Nexorious field | Source | Rule |
|---|---|---|
| `title` | `game.name` | Trimmed. |
| `play_status` | `completion_status` | See the status table below. |
| `personal_rating` | `rating` | 0–100 → whole 1–5 stars: `round(rating / 20)`, clamped to a minimum of 1 star for any positive rating. `0` or null → unrated (nil). |
| `personal_notes` | `comments` + provenance | `comments` verbatim, then appended provenance lines (see below). |
| `hours_played` | `hours_played` | Passed through as a float; `0`/null → nil. |
| `is_loved` | — | Always false (vglist has no equivalent). |
| `created_at` | — | Left empty: vglist exports no "date added". |
| `tags` | — | None (vglist has no tags). |

#### `completion_status` → `play_status`

| vglist | Nexorious | Rationale |
|---|---|---|
| `unplayed` | `not_started` | |
| `in_progress` | `in_progress` | |
| `paused` | `shelved` | Started, then set aside. |
| `dropped` | `dropped` | |
| `completed` | `completed` | |
| `fully_completed` | `mastered` | 100% / completionist. |
| `not_applicable` | `not_started` | No completion semantics; safest default. |
| null / unknown | `not_started` | |

#### Rating examples

`round(rating / 20)`, min 1 star for any positive rating: `100 → 5`, `95 → 5`, `50 → 3`, `30 → 2`, `10 → 1`, `5 → 1`, `0` or null → unrated.

### Platform and store consolidation

This is the only non-trivial part of the vglist mapper, because vglist gives two **unpaired** sets (`platforms` and `stores`) and Nexorious models ownership as `(platform, storefront)` entries. The mapper consolidates them as follows.

**Platform name mapping.** Each `platforms[].name` (a Wikidata label) is mapped to a Nexorious platform slug via a case-insensitive table with aliases (e.g. `"Microsoft Windows"`/`"Windows"`/`"PC"` → `pc-windows`; `"macOS"`/`"Mac OS X"`/`"OS X"` → `mac`; `"PlayStation Portable"` → `playstation-psp`). A label absent from the table is **preserved as a provenance note** (`Owned on <label> (no Nexorious platform mapping).`), never dropped and never a failure — the same rule as Darkadia. Because vglist platform names are free-form Wikidata labels with no fixed list, the table covers the common families and the note is the safety net for the long tail.

**Store name mapping.** Each `stores[].name` is mapped (case-insensitive, alias-tolerant) to a Nexorious storefront slug. Each store also carries a **default platform** and a **compatibility set** — the platforms that storefront can plausibly sit on:

| Store (and aliases) | Storefront slug | Default platform | Compatible platforms |
|---|---|---|---|
| Steam | `steam` | `pc-windows` | pc-windows, pc-linux, mac |
| GOG / GOG.com | `gog` | `pc-windows` | pc-windows, pc-linux, mac |
| Epic Games Store / Epic | `epic-games-store` | `pc-windows` | pc-windows, pc-linux, mac |
| Humble (Store/Bundle) | `humble-bundle` | `pc-windows` | pc-windows, pc-linux, mac |
| itch.io / itch | `itch-io` | `pc-windows` | pc-windows, pc-linux, mac |
| Origin / EA App | `origin-ea-app` | `pc-windows` | pc-windows |
| Uplay / Ubisoft Connect / Ubisoft Store | `uplay` | `pc-windows` | pc-windows |
| GamersGate | `gamersgate` | `pc-windows` | pc-windows |
| Google Play (Store) | `google-play-store` | `android` | android |
| Apple App Store / App Store | `apple-app-store` | `ios` | ios, mac |
| PlayStation Store / PlayStation Network / PSN | `playstation-store` | _(none)_ | playstation-3/4/5, playstation-vita, playstation-psp |
| Nintendo eShop / eShop | `nintendo-eshop` | _(none)_ | nintendo-switch, nintendo-switch-2, nintendo-wii-u, nintendo-3ds |
| Microsoft Store / Xbox (Games) Store | `microsoft-store` | _(none)_ | xbox-360, xbox-one, xbox-series, pc-windows |

A store name absent from the table is **preserved as a provenance note** (`Store: <name> (no Nexorious storefront mapping).`).

**Pairing algorithm.** For each entry:

1. Map every `platforms[].name` to a slug. Unmapped → note. These slugs are the entry's **game platforms**.
2. Map every `stores[].name` to a storefront (with its default platform + compatibility set). Unmapped → note.
3. For each mapped store, attach its storefront to a platform:
   - Attach it to **every game platform in the store's compatibility set** (e.g. game on `[PlayStation 4]` + store `[PlayStation Network]` → `(playstation-4, playstation-store)`). A store on PC where the entry lists both Windows and Linux attaches to both.
   - If **no** game platform is compatible but the store has a **default platform** (the PC/mobile stores), synthesize a `(defaultPlatform, storefront)` entry (e.g. game with no platforms + store `[Steam]` → `(pc-windows, steam)`).
   - If no game platform is compatible and the store has **no** default platform (the console stores), preserve it as a provenance note (`Store: <name> (no compatible platform to attach).`).
4. Every game platform that received **no** store is emitted as a bare `(platform, nil-storefront)` entry.
5. All entries are de-duplicated on `(platform, storefront)`, so multiple stores on one platform (PC owned on both Steam and GOG) correctly yield two entries.

There is **no storefront pairing data in vglist**, so this is a best-effort reconstruction; it never drops a store and never invents a platform a default doesn't justify. Acquired date is always empty (vglist has no per-copy purchase date).

### Provenance notes

`personal_notes` is built from `comments` (verbatim) followed by de-duplicated provenance lines, in this order: unmapped platform labels, unmapped/unmatched stores, `Started: <start_date>.`, `Completed: <completion_date>.`, and `Replayed <n> time(s).` when `replay_count > 0`. If there is no `comments` text and no provenance lines, notes stay nil.

### Merge semantics

Shared and additive-only, identical to Darkadia: never clear the library, never overwrite existing curation, merge new `(platform, storefront)` entries.

### Review, resolution, and cleanup

Ambiguous IGDB matches flow to the existing generic `pending_review` surface (`JobItemsDetails` + `IGDBMatchDialog`); vglist gets no bespoke review UI.

---

## Out of scope / known limitations

- **Recurring/incremental sync** — vglist import is a one-off migration, like Darkadia.
- **Wikidata ids as a match signal** — title matching only for v1 (an id cross-walk is a possible later enhancement).
- **Per-platform store accuracy** — vglist does not pair stores to platforms, so the consolidation above is a heuristic reconstruction.

### Deliberate field drops (accepted-loss ledger)

| vglist field | Why dropped / where it goes |
|---|---|
| `game.wikidata_id` | Not a match signal in v1; not stored. |
| `game.id` | vglist-internal; meaningless in Nexorious. |
| `start_date` / `completion_date` | Nexorious has no per-game start/completion date field; preserved as provenance notes. |
| `replay_count` | No dedicated field; preserved as a provenance note when > 0. |
| Rating granularity | 0–100 collapses to whole 1–5 stars (documented above). |
