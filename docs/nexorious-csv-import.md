# Nexorious CSV Import

This document is the source of truth for how Nexorious re-imports a collection
from its **own CSV export** (`internal/worker/tasks/export.go`). It closes the
"export to CSV → re-import" loop that JSON export/import already has. The
importer is delivered as a `csvmap` preset `Config`
(`internal/services/csvmap/nexorious.go`), selected via the **Format** dropdown
in the CSV import dialog (or auto-detected by its header signature once #1015
lands).

## Identity and matching: IGDB id is exported

Every row carries the **`igdb_id`** column — Nexorious's IGDB-keyed `games.id`.
Re-import uses it for a **direct id match**: the game is hydrated from IGDB by
id, skipping title matching and the `pending_review` surface entirely (the #1022
path). As with every import, **IGDB must be configured** (hydration by id still
calls the IGDB API) or the import is refused up front.

## CSV format

Standard RFC-4180 CSV, UTF-8, one header row, 11 columns:

```
title, igdb_id, play_status, personal_rating, is_loved, hours_played, personal_notes, platforms, tags, created_at, updated_at
```

- **`title`** → game title (fallback only; matching is by id).
- **`igdb_id`** → direct IGDB-id match (#1022).
- **`play_status`** → one of the eight canonical values
  (`not_started`, `in_progress`, `completed`, `mastered`, `dominated`,
  `shelved`, `dropped`, `replay`), round-tripped verbatim via an identity value
  map. An empty or unrecognized value imports as `not_started`.
- **`personal_rating`** → whole `1`–`5` (scale 5).
- **`is_loved`** → `true`/`false`.
- **`hours_played`** → decimal; **the export-summed total across all platforms**.
- **`personal_notes`** → verbatim.
- **`platforms`** → **semicolon-joined platform slugs** (e.g.
  `pc-windows;playstation-5`); split into one ownership entry per slug. Slugs are
  already canonical, so there is no name→slug mapping.
- **`tags`** → semicolon-joined tag names.
- **`created_at`** → RFC3339; stored to date granularity.

## Round-trip caveats

The re-import is faithful for curated scalar fields but not byte-lossless:

- **`updated_at` is dropped** — it has no import-side model home.
- **`hours_played` is the summed total** across platforms; on re-import it
  attaches as the game-level hours (the existing generic behaviour) — the
  per-platform split is not recovered.
- **Platforms come back without storefront, acquired-date, or per-platform
  hours** — the export column carries slugs only, so re-imported ownership
  entries have those fields empty.
- **`created_at`** round-trips to date granularity (the engine normalizes dates
  to `YYYY-MM-DD`); the time component is dropped.
