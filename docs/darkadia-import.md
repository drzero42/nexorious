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

The export has a **core of 29 columns**, plus optional columns added when Darkadia features (time-tracking, reviews, completion dates, copy notes, tags) are enabled. Columns are addressed **by header name**, never by fixed position, so the optional columns — which appear interleaved among the core columns — do not shift the import. Game-level fields are only meaningful on the *named* row; copy-level fields are meaningful on every row. The **Fate** column records how each column maps into Nexorious: every column is accounted for, and every drop is a **deliberate** design decision (collected in the accepted-loss ledger under "Out of scope / known limitations"), never a silent omission. The counts below ("0 rows populated") are from the reference export and motivate the drop; the importer's behaviour does not depend on them.

| # | Column | Level | Meaning | Fate |
|---|---|---|---|---|
| 0 | `Name` | game | Game title. Empty on continuation rows. | Title used for IGDB matching. |
| 1 | `Added` | game | Date the game was added to the Darkadia collection. | → `user_games.created_at`. |
| 2 | `Loved` | game | `1` if marked a favorite. | → `is_loved`. |
| 3 | `Owned` | game | `1` if owned (all exported games are owned; no wishlist in this data). | Feeds `play_status` precedence. |
| 4 | `Played` | game | `1` if the user has played it. | Feeds `play_status`. |
| 5 | `Playing` | game | `1` if currently playing. | Feeds `play_status`. |
| 6 | `Finished` | game | `1` if the main game was completed. | Feeds `play_status`. |
| 7 | `Mastered` | game | `1` for full/100% completion. | Feeds `play_status`. |
| 8 | `Dominated` | game | `1` for Darkadia's top completion tier (beyond Mastered). | Feeds `play_status`. |
| 9 | `Shelved` | game | `1` if the user set it aside / gave up on it. | Feeds `play_status` (→ `dropped`). |
| 10 | `Rating` | game | Personal rating on a **0–5 scale with half-steps** (e.g. `4.5`). Empty means unrated. | → `personal_rating`, **truncated** to a whole star. |
| 11 | `Copy label` | copy | Short user label for the copy (e.g. `PS4`, `HB / Steam`). | **Deliberate drop** — a Darkadia data-model artifact, reconstructable from the platform + storefront captured structurally. |
| 12 | `Copy Release` | copy | The specific release/edition title for this copy. | **Deliberate drop** — edition is expressed by *which IGDB id the game matches*, not carried from Darkadia. |
| 13 | `Copy platform` | copy | The platform of this specific copy (e.g. `PlayStation 4`, `PC`, `PlayStation Network (PS3)`). | → consolidated platform list. |
| 14 | `Copy media` | copy | `Digital`, `Physical`, or `N/A`. | Routes storefront resolution (physical-first); not stored as a standalone field. |
| 15 | `Copy media other` | copy | Free-text media type when `Copy media` is "other". | **Deliberate drop** — unused (0 rows populated in the reference export). |
| 16 | `Copy source` | copy | Where the copy was obtained, from a fixed Darkadia list (e.g. `Steam`, `GOG`, `Sony Entertainment Network`, `Humble Bundle`, `GameStop`). `Other` defers to `Copy source other`. | → storefront slug, or provenance note. |
| 17 | `Copy source other` | copy | Free-text source when `Copy source` is `Other` (e.g. `Epic`, `Fanatical`, `WinGameStore`, `Bilka`). | → storefront slug, or provenance note. |
| 18 | `Copy purchase date` | copy | Date the copy was acquired. | → `user_game_platforms.acquired_date`. |
| 19 | `Copy box` | copy | Physical-media flag (auto-filled, set even on digital copies). | **Deliberate drop** — boilerplate; no Nexorious model for physical media. |
| 20 | `Copy box condition` | copy | Physical box condition (almost entirely `N/A`). | **Deliberate drop** — physical condition not modeled. |
| 21 | `Copy box notes` | copy | Free-text box notes. | **Deliberate drop** — unused (0 rows populated). |
| 22 | `Copy manual` | copy | Physical-media flag (auto-filled, set even on digital copies). | **Deliberate drop** — boilerplate. |
| 23 | `Copy manual condition` | copy | Physical manual condition (almost entirely `N/A`). | **Deliberate drop** — physical condition not modeled. |
| 24 | `Copy manual notes` | copy | Free-text manual notes. | **Deliberate drop** — unused (0 rows populated). |
| 25 | `Copy complete` | copy | Physical-completeness flag (auto-filled, set even on digital copies). | **Deliberate drop** — boilerplate. |
| 26 | `Copy complete notes` | copy | Free-text completeness notes. | **Deliberate drop** — unused (0 rows populated). |
| 27 | `Platforms` | game | **Aggregate ownership** — the platforms the user marked owning the game on. Comma-separated. See "Two ways to record platforms". | → consolidated platform list (union with copy platforms). |
| 28 | `Notes` | game | Free-text personal note. | → `personal_notes`, **verbatim**, plus appended provenance lines. |

> Note the physical column order: `Platforms` and `Notes` are the **last two** columns (27–28), after the copy block — not adjacent to the other game-level fields. A parser must address columns by header name, not by assuming game-level fields come first.

### CSV dialect and parsing

The export is standard RFC 4180 CSV, UTF-8, with a single header row. The facts below were verified against the reference export (`darkadia_export_csv_20251213.csv` — 1,744 lines → 1,740 data rows → 1,474 games):

- **Quoting is partial.** Only some columns are quoted (the `Copy *` block and any header containing a space); a parser must not assume every field is quoted, nor that unquoted fields are safe to split naively.
- **`Notes` can contain embedded newlines.** A free-text `Notes` value may carry a `CRLF` *inside* its surrounding quotes (one such note in the reference export). The file therefore **cannot be parsed line-by-line** — use a real CSV reader that honours quoted multi-line fields. No embedded double-quotes or commas appear in the reference `Notes`, but a conformant parser handles them per RFC 4180 regardless.
- **Rows may be ragged.** Trailing empty columns can be omitted: 5 rows in the reference export carry **fewer than 29 fields**. The parser must tolerate a variable field count (e.g. Go's `encoding/csv` with `FieldsPerRecord = -1`) and treat missing trailing columns as empty, rather than rejecting the row.
- **Dates are ISO-8601.** `Added` and `Copy purchase date` are `YYYY-MM-DD` calendar dates (e.g. `2013-06-05`). Empty means unset.
- **Header validation.** The importer parses the header into a name→position map and requires the 29 core column names to all be present (a strong Darkadia signature). Extra columns are tolerated; a file missing any required column is rejected as non-Darkadia **before** any rows are processed. Reordered or interleaved columns are handled because every field is read by name.

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

- **Playtime and tags are feature-gated.** Exports made with Darkadia's time-tracking and tags features enabled carry a `Time played` column and a `Tags` column; exports without those features omit them. When present, `Time played` → `hours_played` (on the game's first platform entry) and `Tags` → `user_game_tags`. When absent, nothing is lost.
- **No game metadata.** Descriptions, cover art, genres, release dates, etc. are not in the export; Nexorious obtains these from IGDB after matching.

---

## Part 2 — How it maps into Nexorious

### Pipeline shape

The import runs on Nexorious's existing **`jobs` / `job_items` import framework** — the same machinery the Nexorious-JSON importer already uses — **not** the storefront-sync pipeline. It reuses the IGDB matching *primitives* (title search, fuzzy-confidence scoring, the auto-resolve thresholds) and the generic, source-agnostic `job_items` review surface, but it does **not** create `external_games` staging rows, does not masquerade as a storefront, and does not touch the sync connection/config machinery. Each game becomes one `job_item`; the full consolidated per-game payload rides in that item's `source_metadata`. At a high level:

1. **Upload & validate.** The user uploads the Darkadia CSV via a dedicated `darkadia` import source on the Import/Export page. The file is validated as a Darkadia export by its header row; non-Darkadia files are rejected with a clear error. The import is **refused up front** if IGDB is not configured (see below).
2. **Parse & consolidate.** Rows are grouped into games and copies, and each game's owned platforms are consolidated (below). The full per-game payload (status, rating, loved, notes, added-date, and the consolidated platform/storefront/date list, with provenance lines already assembled) is written as one `job_item` per game, carried in `source_metadata`.
3. **Match to IGDB.** Each game's title is matched against IGDB using the shared matching primitives. A confident, unambiguous match auto-resolves; anything low-confidence or tied is stored as candidates on the `job_item` and set to **`pending_review`** — surfaced through the generic `JobItemsDetails` review UI (reusing `IGDBMatchDialog`).
4. **Finalize.** Once a game is resolved (automatically or by the user), it is written into the library: a `user_game` plus its `user_game_platforms`, with the Darkadia-derived fields applied under the additive merge rules below.
5. **Clean up.** Minimal — there is no sync-style staging to prune. The `jobs` / `job_items` rows remain as import history; only the transient `source_metadata` payload may optionally be cleared once an item finalizes.

Because matching and review can take time (the user may resolve ambiguous matches over multiple sessions), the `job_items` persist until the import is fully resolved.

### Prerequisite: IGDB is required

The entire import depends on title→IGDB matching. If IGDB is not configured, **the import is refused up front** with an explanatory error, rather than enqueuing a collection that can never be matched.

### Game identity and matching

The Darkadia `Name` is the title used for matching. The importer reuses the **same matching primitives** as sync — the IGDB title search, the fuzzy-confidence scoring, and the auto-resolve thresholds — rather than inventing a parallel matcher; it simply runs them over its own `job_items` and stores candidates / confidence / resolved id on the item (not on `external_games`). Game metadata (cover, description, release date, genre, …) comes from IGDB on resolution; none of it is sourced from Darkadia.

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
| `PlayStation 2` | `playstation-2` | — |
| `PlayStation Network (PSP)` | `playstation-psp` | `playstation-store` |

This table covers **every** platform string present in the reference export (both the aggregate `Platforms` list and `Copy platform`); each target slug exists in the platform seed. `PC` defaults to `pc-windows` because Darkadia records `Linux` and `Mac` separately.

`playstation-2`'s seed `default_storefront` is `physical`, but per the rule above we deliberately do **not** consult `default_storefront`, so a `PlayStation 2` ownership mark with no matching copy yields `(playstation-2, NULL)`, not a fabricated `physical` storefront.

A platform string outside this table is **not silently dropped**: the original string is preserved as a provenance line in the game's `personal_notes` (the same fallback used for unrecognized storefronts), and the game still imports with whatever other platforms *did* map. No `user_game_platform` row is fabricated for it, and the item does **not** fail. Because the table is complete for the known export and the Darkadia format is frozen, this is a should-never-happen guard, not an expected path.

### Storefront mapping

The source string is taken from `Copy source`, except when that is the literal `Other`, in which case the free-text `Copy source other` is used. Recognition matches **case-insensitively** against this string. Per-copy storefront is then resolved in this order:

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

   Recognition tolerates the spelling variants observed in the reference export: Epic appears as `Epic`, `Epic Games Store`, `Epic Game Store`, and `Epic Gamestore` (and via `Copy source other`); Ubisoft as `Uplay` and `Ubisoft Club`. A recognized source that carries **extra free text** (only `Uplay (coupon w/ GTX 970)` in the reference export) still maps to its slug; the parenthetical annotation is a **deliberate, documented drop** — preserving such one-off annotations is not worth a special case.

2. **Physical media** (`Copy media` is exactly `Physical`) → storefront `physical`; the specific retailer (`GameStop`, `Bilka`, `Proshop`, …) goes into the provenance note. Media is the deciding signal here, **not** the retailer name: `GameStop` appears in the reference export as both digital *and* physical copies (5 digital, 3 physical), so the same retailer routes to `NULL`+note (digital) or `physical`+note (physical) depending on `Copy media`. Only the literal value `Physical` triggers this rule; `Digital`, `N/A`, and empty `Copy media` never do.
3. **Unrecognized digital store** (`Green Man Gaming`, `Fanatical`, `WinGameStore`, `Telltale.com`, `Kickstarter`, `Indie Gala`, `cdon.com`, …) → storefront `NULL`; the source name goes into the provenance note.
4. Otherwise, the platform-string inferred storefront (above) may apply; otherwise `NULL`.

An **empty `Copy source`** (no purchase source recorded — 406 copies in the reference export) yields a `NULL` storefront and **no** provenance note: there is nothing to record. A provenance note is appended only for cases 2 and 3 (a physical retailer, or an unrecognized digital store).

**No new storefronts are seeded** to accommodate the long tail of stores. The visible, durable home for "where did I buy this" is the provenance note.

### Merge semantics

The import **merges into the existing library; it never clears it.** When a game is already present:

- **Game-level fields are additive-only** — existing `play_status`, `personal_rating`, `is_loved`, and `personal_notes` are left untouched. The import never overwrites curation the user already did.
- **Platforms and storefronts are merged in** — new `(platform, storefront)` entries are added; existing ones are not duplicated.

A fresh import into an empty library is simply the degenerate case of this rule.

### Review, resolution, and cleanup

Ambiguous matches use the **existing `pending_review` experience**, but on the `job_items` framework — the user picks an IGDB candidate or skips the game through the generic, source-agnostic `JobItemsDetails` review surface (reusing the existing `IGDBMatchDialog`), not the sync page. Resolve/skip act on the `job_item` and are scoped to import sources so the sync `external_games` flow is untouched. As with sync, a `pending_review` item keeps the job in `processing` until it is resolved or skipped; resolution (automatic or manual) triggers finalization into the library.

Cleanup is minimal because nothing sync-like is created: there are **no `external_games` staging rows** to prune. The `jobs` / `job_items` rows are the import's history and remain (they drive Recent Activity); the only transient payload is each item's `source_metadata`, which may optionally be cleared after the item finalizes. An imported game ends up indistinguishable from a manually added one.

---

## Out of scope / known limitations

- **One-off only.** No incremental re-import, no saved mapping for "next time", no persistent Darkadia connection.
- **Half-star ratings are truncated** — Nexorious has no half-star representation.
- **No playtime, no tags** — Darkadia provides neither.
- **No new storefronts** — the long tail of small/physical stores is preserved in notes, not as first-class storefronts.
- **No-copy multi-platform games** are imported as owned on every aggregate platform; because the aggregate is ownership, this is intentional and not flagged for review.
- **Wishlist is not handled** — the observed exports contain only owned games.
- **Darkadia's own lossiness** (dropped platform associations) cannot be recovered beyond the union heuristic.

### Deliberate field drops (accepted-loss ledger)

These Darkadia columns are dropped **on purpose** because Nexorious has no representation for them. Each is a cleared design decision, not a silent omission (see the per-column **Fate** in the column reference):

- **`Copy label`** — a Darkadia data-model artifact (e.g. `HB / Steam`); reconstructable from the platform + storefront captured structurally.
- **`Copy Release` (edition title)** — edition is expressed by which IGDB id the game matches, not carried from Darkadia.
- **Half-star precision in `Rating`** — truncated to a whole star (`4.5` → `4`).
- **Physical-condition block** — `Copy box`, `Copy box condition`, `Copy box notes`, `Copy manual`, `Copy manual condition`, `Copy manual notes`, `Copy complete`, `Copy complete notes`: no physical-media model; in the reference export these are boilerplate/auto-filled and the free-text `*notes` columns are entirely empty.
- **`Copy media other`** — empty in the reference export; no separate media-type field.
- **Extra free text on a recognized source** — e.g. the `(coupon w/ GTX 970)` annotation on a `Uplay` source: the storefront is captured, the annotation is dropped.
- **Completion dates** — `Date completed`, `Date mastered`, `Date dominated`: Nexorious has no per-status completion-date field.
- **Milestone times** — `Time to complete`, `Time to master`, `Time to dominate`: `games.howlongtobeat_*` is IGDB community data, not per-user; there is no per-user milestone-time field. (The user's actual `Time played` *is* mapped, to `hours_played`.)
