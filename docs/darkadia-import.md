# Darkadia CSV Import

This document is the source of truth for how Nexorious imports a game collection from a **Darkadia CSV export**. It describes the Darkadia export format and how that data **should** map into Nexorious during import. It is intended for both humans who need to understand the format and coding agents who need to know what to build (or verify) without being tied to any specific implementation.

Darkadia was a game-collection tracking service that shut down. Before it closed, users could export their collection to CSV. This importer exists so a Darkadia refugee can bring that collection into Nexorious with as little loss as possible. It is a **one-off migration path**, not a recurring sync — there is no persistent connection, no incremental re-import, and no state kept "for next time".

---

## Overview

A Darkadia CSV holds one user's whole collection: every game, the platforms they own it on, where each copy was purchased, a personal rating, a "loved" flag, free-text notes, and a set of progress/achievement flags. Nexorious does not store Darkadia's exact shape, so the import **translates and consolidates** Darkadia's model into Nexorious's `user_games` + `user_game_platforms` model.

The import is designed to be:

- **Faithful** — it preserves real user data (ratings, loved, notes, purchase provenance, the date a game was added) rather than discarding or overwriting it.
- **Non-destructive** — it merges into the existing library; it never clears it and never overwrites curation the user already did.
- **Honest about loss** — where Nexorious genuinely cannot represent something (half-star ratings), the behavior is defined and documented rather than silent.

Identifying games requires IGDB. **The import is blocked unless IGDB is configured**, because without it every game would land unmatched.

---

## Glossary

| Term | Meaning |
|---|---|
| **Game** | One title in the Darkadia collection. In the CSV it is a *named row* plus zero or more continuation rows. |
| **Copy** | One concrete acquisition of a game: a specific platform, where it was purchased, the media (digital/physical), and a purchase date. A game can have several copies. |
| **Aggregate platforms** | Darkadia's game-level "which platforms do I own this on" marks. The CSV `Platforms` column. This is **ownership**, not availability. |
| **Status flags** | Darkadia's cumulative progress/achievement markers: Owned, Played, Playing, Finished, Mastered, Dominated, Shelved (plus the separate Loved flag). |
| **IGDB** | The canonical game database Nexorious uses to identify games. Required for this import. |
| **pending_review** | The state in which a game's IGDB match was ambiguous and the user must pick a candidate (or skip). Shared with the sync system. |

---

## Part 1 — The Darkadia CSV format

### Row structure: games and copies

A Darkadia CSV is **not** one row per game. It is one row per **copy**, with the game's identity and game-level attributes living on the **first** row of each game (the *named row*). Additional copies of the same game follow as continuation rows whose `Name` column is **empty**.

```
Name          | ... game-level fields ... | ... copy fields ...
"Aaru's Awakening"  Owned, Rating, Notes…    PS4 copy: source, purchase date…
""                  (blank — same game)       PS3 copy: source, purchase date…
```

To reconstruct games, group rows so that **each non-empty `Name` starts a new game and every following empty-`Name` row attaches to it as another copy.** An empty-`Name` row never appears before the first named row.

Game-level fields (status flags, rating, loved, notes, the "Added" date, the aggregate `Platforms` list) are only meaningful on the named row. Copy-level fields (`Copy *` columns) are meaningful on every row, named and continuation alike.

> In a representative real export, ~1,740 rows collapsed to ~1,474 games: most games had a single copy, but many had two to five.

### Column reference

| Column | Level | Meaning |
|---|---|---|
| `Name` | game | Game title. Empty on continuation rows. |
| `Added` | game | Date the game was added to the Darkadia collection. The genuine "date added", worth preserving. |
| `Loved` | game | `1` if marked a favorite. |
| `Owned` | game | `1` if owned. In practice all exported games are owned (Darkadia had no separate wishlist in this data). |
| `Played` | game | `1` if the user has played it. |
| `Playing` | game | `1` if currently playing. |
| `Finished` | game | `1` if the main game was completed. |
| `Mastered` | game | `1` for full/100% completion. |
| `Dominated` | game | `1` for Darkadia's top completion tier (beyond Mastered). |
| `Shelved` | game | `1` if the user set it aside / gave up on it. |
| `Rating` | game | Personal rating on a **0–5 scale with half-steps** (e.g. `4.5`). Empty/`0` means unrated. |
| `Platforms` | game | **Aggregate ownership** — the platforms the user marked owning the game on. Comma-separated. See "Two ways to record platforms". |
| `Notes` | game | Free-text personal note. |
| `Copy label` | copy | Short user label for the copy (e.g. `PS4`, `HB / Steam`). |
| `Copy Release` | copy | The specific release/edition title for this copy. |
| `Copy platform` | copy | The platform of this specific copy (e.g. `PlayStation 4`, `PC`, `PlayStation Network (PS3)`). |
| `Copy media` | copy | `Digital`, `Physical`, or `N/A`. |
| `Copy source` | copy | Where the copy was obtained, from a fixed Darkadia list (e.g. `Steam`, `GOG`, `Sony Entertainment Network`, `Humble Bundle`, `GameStop`). `Other` defers to `Copy source other`. |
| `Copy source other` | copy | Free-text source when `Copy source` is `Other` (e.g. `Epic`, `Fanatical`, `Green Man Gaming`, `Bilka`). |
| `Copy purchase date` | copy | Date the copy was acquired. |
| `Copy box*`, `Copy manual*`, `Copy complete*` | copy | Physical-media condition fields (box/manual/completeness and notes). Not imported. |

### Status flags are cumulative

The progress flags are **cumulative tiers**, not independent toggles. A `Dominated` game is also `Mastered`, `Finished`, and `Played`. The meaningful state of a game is therefore the **highest tier reached**, combined with the orthogonal `Shelved`, `Playing`, `Loved` markers. The mapping in Part 2 resolves these to a single Nexorious status by precedence.

### Two ways to record platforms (the key subtlety)

Darkadia let the user record platform information **two different ways**, and a single game can use either or both:

1. **Aggregate ownership marks** — the game-level `Platforms` list. This is the authoritative "which platforms do I own this on". A game like *Anodyne* can be marked as owned on `PC, Mac`.
2. **Per-copy detail** — the `Copy *` columns, which additionally capture *where* a copy was bought and *when*. Users often filled this in for only **some** of the platforms they owned (e.g. recorded the PC copy's store but never bothered with the Mac copy).

Consequences that the importer must respect:

- The aggregate `Platforms` list is **ownership, not availability**. If it lists `Mac, PC`, the user owns it on both — even if only the PC copy has purchase detail.
- The per-copy platforms are a (usually partial) **subset** of the owned platforms, enriched with storefront and date.
- **Darkadia was lossy.** Platform associations sometimes disappeared, so neither source is fully reliable on its own. A copy can name a platform that is missing from the aggregate, and vice-versa.
- Some games have **no copy detail at all** — only the aggregate list. For those, the aggregate is the only platform signal, and there is no storefront or purchase date to recover.
- A few games have an **empty aggregate and no copies** — no platform signal at all.

The import therefore **consolidates** the two sources rather than choosing one (see Part 2).

### What Darkadia does not provide

- **No playtime.** There is no hours-played data.
- **No tags.** Darkadia has no tag concept.
- **No game metadata.** Descriptions, cover art, genres, release dates, etc. are not in the export; Nexorious obtains these from IGDB after matching.

---

## Part 2 — How it maps into Nexorious

### Pipeline shape

The import is modeled as a file-based, sync-like operation that reuses Nexorious's existing IGDB matching and manual-review experience, because identifying games by title is exactly what sync already does. At a high level:

1. **Upload & validate.** The user uploads the Darkadia CSV as a dedicated import source. The file is validated as a Darkadia export by its header row; non-Darkadia files are rejected with a clear error.
2. **Parse & consolidate.** Rows are grouped into games and copies, and each game's owned platforms are consolidated (below). The full per-game payload (status, rating, loved, notes, added-date, and the consolidated platform/storefront/date list) is carried as staging data through matching.
3. **Match to IGDB.** Each game's title is matched against IGDB. A confident, unambiguous match auto-resolves; anything low-confidence or tied is routed to **pending_review** — the same review surface used by sync.
4. **Finalize.** Once a game is resolved (automatically or by the user), it is written into the library: a `user_game` plus its `user_game_platforms`, with the Darkadia-derived fields applied.
5. **Clean up.** The transient staging records created only to drive matching/review are pruned once the import has no work left. Nothing about the import lingers as connectable "sync state".

Because matching and review can take time (the user may resolve ambiguous matches over multiple sessions), staging data must survive until the import is fully resolved, then be removed.

### Prerequisite: IGDB is required

The entire import depends on title→IGDB matching. If IGDB is not configured, **the import is refused up front** with an explanatory error, rather than enqueuing a collection that can never be matched.

### Game identity and matching

The Darkadia `Name` is the title used for matching. Matching, confidence thresholds, auto-resolution, candidate storage, and the pending_review experience are **the same** as the sync system's IGDB matching stage — this importer does not invent a parallel matching mechanism. Game metadata (cover, description, release date, genre, …) comes from IGDB on resolution; none of it is sourced from Darkadia.

### Game-level field mapping

| Nexorious field | From Darkadia | Rule |
|---|---|---|
| `play_status` | status flags | Highest-precedence tier (table below). |
| `is_loved` | `Loved` | Direct boolean. |
| `personal_rating` | `Rating` | **Truncated** to a whole 1–5 star (`4.5` → `4`). Empty/`0` → unrated. |
| `personal_notes` | `Notes` + provenance | The user's real note **verbatim**, with a purchase-provenance line appended (below). |
| `created_at` | `Added` | The collection-added date is preserved as the game's created timestamp. |

#### play_status precedence

Nexorious `shelved` means "progress is paused"; Darkadia `Shelved` means "set aside / given up". The flags map by **highest precedence wins**:

| Precedence | Darkadia flag | → Nexorious `play_status` |
|---|---|---|
| 1 | Dominated | `dominated` |
| 2 | Mastered | `mastered` |
| 3 | Finished | `completed` |
| 4 | Shelved | `dropped` |
| 5 | Playing | `in_progress` |
| 6 | Played (only) | `shelved` |
| 7 | Owned (only) | `not_started` |

#### Rating

Nexorious ratings are whole 1–5 stars; the model cannot represent half-stars. Darkadia half-star ratings are **truncated** (`4.5` → `4`, `3.5` → `3`). This is a defined, accepted loss: Nexorious has no half-star representation, and truncation matches how a rating is otherwise coerced to an integer.

#### Notes and purchase provenance

`personal_notes` does double duty:

- The user's **real Darkadia `Notes`** are preserved verbatim. They must never be dropped or overwritten (an earlier ad-hoc conversion lost ~60% of real notes by repurposing the field — this importer must not).
- Purchase locations that **cannot** be represented as a Nexorious storefront (physical retailers, and digital stores Nexorious does not model) are appended as a short provenance line, so the "where did I buy this" curiosity is not lost. Recognized digital storefronts do **not** need a note — they are captured structurally on the platform.

### Platform consolidation

Owned platforms for a game are the **union** of:

- the aggregate `Platforms` list (ownership marks), and
- every `Copy platform` (guards against Darkadia having dropped an aggregate association).

Then each owned platform is **decorated**:

- If one or more copies match that platform, emit one `(platform, storefront)` entry **per copy** — taking storefront, acquired-date, and media from that copy.
- If no copy matches, emit a single `(platform, NULL-storefront)` entry.

Entries are de-duplicated on `(platform, storefront)`. All imported platforms are owned (`ownership_status = owned`). There is no playtime to set.

Worked examples:

- **Anodyne** — aggregate `PC, Mac`; one PC copy bought on GOG. → owns `pc-windows` (storefront `gog`, with purchase date) **and** `mac` (no storefront). Both owned.
- **Aaru's Awakening** — aggregate `PlayStation Network (PS3), PlayStation 4`; PS3 and PS4 copies via PSN. → owns `playstation-3` and `playstation-4`, each with `playstation-store`.
- **A no-copy game** — aggregate `Mac, PC`, no copy detail. → owns `mac` and `pc-windows`, both without storefront. This is genuine ownership, not over-reporting.
- **A no-platform game** — empty aggregate, no copies. → imported as a platform-less `user_game`.

### Platform string mapping

Darkadia platform strings (used in both the aggregate list and `Copy platform`) map to Nexorious platform slugs. Strings that **explicitly name a storefront** also carry an inferred storefront, used only as a fallback when no copy supplies one. Nexorious's broad per-platform `default_storefront` is deliberately **not** used for this inference, because it would fabricate storefronts (e.g. stamping every PC game as Steam).

| Darkadia platform string | Platform slug | Inferred storefront |
|---|---|---|
| `PC` | `pc-windows` | — |
| `Linux` | `pc-linux` | — |
| `Mac` | `mac` | — |
| `PlayStation 4` | `playstation-4` | — |
| `PlayStation 5` | `playstation-5` | — |
| `PlayStation 3` | `playstation-3` | — |
| `PlayStation Network (PS3)` | `playstation-3` | `playstation-store` |
| `PlayStation Network (Vita)` | `playstation-vita` | `playstation-store` |
| `Nintendo Switch` | `nintendo-switch` | — |
| `Wii` | `nintendo-wii` | — |
| `Xbox 360` | `xbox-360` | — |
| `Xbox 360 Games Store` | `xbox-360` | `microsoft-store` |
| `Android` | `android` | — |

`PC` defaults to `pc-windows` because Darkadia records `Linux` and `Mac` separately. A platform string not covered by the table is **flagged, not silently dropped** — unmapped platforms should surface in the import outcome so the data is not quietly lost.

### Storefront mapping

Per-copy storefront is resolved in this order:

1. **Recognized digital source** → the canonical Nexorious storefront slug (icon and filtering work):

   | Darkadia source | Storefront |
   |---|---|
   | `Sony Entertainment Network` | `playstation-store` |
   | `Epic Games Store` / `Epic` (and spelling variants) | `epic-games-store` |
   | `GOG` | `gog` |
   | `Humble Bundle` | `humble-bundle` |
   | `Steam` | `steam` |
   | `Nintendo eShop` | `nintendo-eshop` |
   | `Origin` | `origin-ea-app` |
   | `GamersGate` | `gamersgate` |
   | `Google Play` | `google-play-store` |
   | `Uplay` / `Ubisoft Club` | `uplay` |

2. **Physical media** (`Copy media = Physical`) → storefront `physical`; the specific retailer (`GameStop`, `Bilka`, …) goes into the provenance note.
3. **Unrecognized digital store** (`Green Man Gaming`, `Fanatical`, `WinGameStore`, `Telltale.com`, `Kickstarter`, …) → storefront `NULL`; the source name goes into the provenance note.
4. Otherwise, the platform-string inferred storefront (above) may apply; otherwise `NULL`.

**No new storefronts are seeded** to accommodate the long tail of stores. Nexorious's `original_storefront_name` field is not used for this, because it is stored but never displayed — the visible, durable home for "where did I buy this" is the note.

### Merge semantics

The import **merges into the existing library; it never clears it.** When a game is already present:

- **Game-level fields are additive-only** — existing `play_status`, `personal_rating`, `is_loved`, and `personal_notes` are left untouched. The import never overwrites curation the user already did.
- **Platforms and storefronts are merged in** — new `(platform, storefront)` entries are added; existing ones are not duplicated.

A fresh import into an empty library is simply the degenerate case of this rule.

### Review, resolution, and cleanup

Ambiguous matches use the **existing pending_review experience** — the user picks an IGDB candidate or skips the game, exactly as with sync. Resolution (automatic or manual) triggers finalization into the library. Once the import has no remaining pending or in-review work, the transient staging records that existed only to drive matching/review are pruned. Pruning is safe by construction: the real library (`user_games`, `user_game_platforms`) does not depend on the staging records, and an imported game ends up indistinguishable from a manually added one.

---

## Out of scope / known limitations

- **One-off only.** No incremental re-import, no saved mapping for "next time", no persistent Darkadia connection.
- **Half-star ratings are truncated** — Nexorious has no half-star representation.
- **No playtime, no tags** — Darkadia provides neither.
- **No new storefronts** — the long tail of small/physical stores is preserved in notes, not as first-class storefronts.
- **No-copy multi-platform games** are imported as owned on every aggregate platform; because the aggregate is ownership, this is intentional and not flagged for review.
- **Wishlist is not handled** — the observed exports contain only owned games.
- **Darkadia's own lossiness** (dropped platform associations) cannot be recovered beyond the union heuristic.
