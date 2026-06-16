# Grouvee CSV Import

This document is the source of truth for how Nexorious imports a game collection
from a **Grouvee CSV export**. Grouvee is a web-based game tracker built around a
"shelves" model; users export their collection to CSV from account settings
(delivered by email). This importer is a **one-off migration path** — no
persistent connection, no incremental re-import.

Grouvee is delivered as a `csvmap` preset `Config` (see
`internal/services/csvmap/grouvee.go`), not a bespoke mapper. A Grouvee file is
selected via the **Format** dropdown in the CSV import dialog (or, once #1015
lands, auto-detected by its header signature).

## Identity and matching: IGDB id is exported

Every Grouvee row carries a populated **`igdb_id`** column. Nexorious uses it for
a **direct id match** — the game is hydrated from IGDB by id, skipping title
matching and the `pending_review` surface entirely (the #1022 path). A row that
somehow lacks the id falls back to title matching. As with every import, **IGDB
must be configured** (hydration by id still calls the IGDB API), or the import is
refused up front.

## CSV format

Standard RFC-4180 CSV, UTF-8, one header row, 20 columns. Three columns embed
JSON:

- **`shelves`** — a JSON object keyed by shelf name, e.g.
  `{"Played": {"date_added": "...", "url": "..."}}`. The **keys** are the shelves.
- **`platforms`** — a JSON object keyed by IGDB platform name, e.g.
  `{"PC (Microsoft Windows)": {"url": "..."}}` (often `{}`).
- **`dates`** — a JSON array of play-session objects, e.g.
  `[{"seconds_played": 36300, "level_of_completion": "100% Completion", ...}]`.

## Column reference

| Column | Fate |
|---|---|
| `id` | Grouvee's own game id — dropped (Nexorious keys on IGDB). |
| `name` | Title (fallback matching for any row missing `igdb_id`). |
| `shelves` | JSON object → `play_status` (baseline) + `is_wishlisted`. |
| `platforms` | JSON object → owned-platform entries. |
| `rating` | → `personal_rating`, 1–5 scale, **rounded to nearest** (`4.5` → `5`). |
| `review_title` | → note heading (`**title**`). |
| `review` | → note body. |
| `review_platform` | Dropped (no model home; part of the signature). |
| `dates` | JSON play-log → `hours_played` (`seconds_played`) and a completion-tier `play_status` override (`level_of_completion`). Start/finish dates are not read. |
| `statuses` | Dropped — **verified empty** in real exports even after setting statuses in-app. |
| `genres`, `franchises`, `series`, `developers`, `publishers`, `release_date` | Dropped — editorial metadata IGDB resupplies on match. |
| `date_added_to_collection` | → `user_games.created_at`. |
| `url` | Grouvee page URL — dropped. |
| `giantbomb_id` | Dropped (part of the signature). |
| `igdb_id` | → direct IGDB-id match. |

## Shelves → play_status and wishlist

The shelf keys map as follows. When a game sits on several shelves, precedence is
`Playing > Played > Backlog`. An unrecognized/custom shelf falls back to
`not_started`.

| Shelf | Result |
|---|---|
| `Playing` | `play_status = in_progress` |
| `Played` | `play_status = completed` |
| `Backlog` | `play_status = not_started` |
| `Wish List` | `is_wishlisted = true` (orthogonal to play_status) |

A `Wish List` game with no other shelf imports as a wishlisted, **unowned** entry
(`is_wishlisted = true`, no platforms). If it is also owned (has a platform), the
wishlist flag is cleared on acquire, consistent with the rest of Nexorious.

## Play-log → playtime and completion tier

From the `dates` array:

- **`seconds_played`** is summed across entries and divided by 3600 →
  `hours_played` (0 / unset → no playtime). It lands on the game's first platform
  entry (no platform → playtime is dropped, as elsewhere in import).
- **`level_of_completion`** maps to a completion tier that **overrides** the
  shelf-derived `play_status` whenever present:

  | `level_of_completion` | `play_status` |
  |---|---|
  | `Main Story` | `completed` |
  | `Main Story + Extras` | `mastered` |
  | `100% Completion` | `dominated` |

  Across multiple entries the **highest** tier wins. Unrecognized tiers are
  ignored (the shelf status stands). `date_started` / `date_finished` are not
  used.

## Platform mapping

Grouvee platform names are IGDB platform names. They map to Nexorious platform
slugs (`PC (Microsoft Windows)` → `pc-windows`, `PlayStation 4` →
`playstation-4`, …). The map is **fixture-derived and extensible**; an unrecognized
platform name passes through unchanged. Grouvee's platform objects carry no
storefront or purchase date, so imported platforms have neither.

## Merge semantics

The import merges into the existing library; it never clears it. Game-level
fields are additive-only — an already-present game is not re-wishlisted and its
curation is not overwritten. New `(platform, storefront)` entries are added;
existing ones are not duplicated.

## Known limitations

- **One-off only** — no incremental re-import or saved connection.
- **Half-star ratings** round to the nearest whole star.
- **A populated completion tier always overrides the shelf** (even on a Playing or
  Backlog shelf). In practice a tier is only set on games actually completed.
- **Custom shelves** map to `not_started`.
- **Play-session dates and the `statuses` column are not imported** — Nexorious
  has no model for them.
- **Platform vocabulary is fixture-derived** — unseen IGDB platform names pass
  through unmapped.
