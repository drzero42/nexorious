# Source-neutral import pipeline (#1000)

**Issue:** [#1000](https://github.com/drzero42/nexorious/issues/1000) — the blocking refactor of the multi-source import epic [#984](https://github.com/drzero42/nexorious/issues/984).
**Status:** design approved, ready for an implementation plan.
**Lands first.** Every other sub-issue of #984 (vglist #1001, Grouvee #1002, Completionator #1003, generic CSV #1004) depends on this and is blocked until it lands.

## Problem

Nexorious supports exactly one external-tracker migration source: Darkadia. Its importer already runs on the generic `jobs` / `job_items` framework, and most of its code is *already* source-agnostic — but everything is named and packaged as if Darkadia-specific. Adding a new source today would mean copy-pasting the match worker, the finalize worker, the completion check, and the upload handler, and changing only a handful of strings.

The goal is to extract a **source-neutral import pipeline** so that adding a source (vglist, Grouvee, Completionator, generic CSV) requires only:

1. a small per-source **mapper** (parse the source file → canonical structs, plus a signature validator), and
2. one line of **registration**.

Nothing else.

## Scope

In:

- A shared canonical game model (`internal/services/importmodel`).
- A `Mapper` interface + a **source registry** (`internal/services/importsource`).
- Generalising the Darkadia match/finalize/completion workers into source-neutral workers keyed by `source`, with the River job kinds renamed `darkadia_match`/`darkadia_finalize` → `import_match`/`import_finalize`.
- Collapsing `HandleImportDarkadia` into a parameterised, registry-driven upload handler.
- A new `GET /api/import/sources` endpoint so the frontend source picker is data-driven from the registry.
- Re-expressing Darkadia as one registered mapper, with **zero behaviour change** for the actual import.

Out (this issue):

- vglist / Grouvee / Completionator / generic-CSV mappers — those are #1001–#1004, each depending only on this.
- Any change to the Nexorious JSON importer (`HandleImportNexorious`) — it trusts IGDB ids in the file and is not a title-matching migration; it is a separate path and stays as-is.
- Using a source's foreign ids (Wikidata/GiantBomb) as a match signal — title matching only, per the epic.

## Verified facts (current code)

- `internal/services/darkadia/darkadia.go` — `Parse([]byte) ([]darkadia.Game, error)`; `Game`/`Platform` structs; `consolidate`, `platformTable`, `storefrontTable`, `resolvePlayStatus`, `parseRating`; `ErrInvalidHeader` is the signature-mismatch sentinel.
- `internal/worker/tasks/darkadia.go` — `DarkadiaMatchWorker` (kind `darkadia_match`), `DarkadiaFinalizeWorker` (kind `darkadia_finalize`), `DarkadiaCheckJobCompletion`. The finalize worker unmarshals `darkadia.Game` from `job_item.source_metadata` and writes `user_games` + `user_game_platforms` with additive-only merge. All of this logic is already source-agnostic.
- `internal/api/import.go` — `HandleImportDarkadia` (validate IGDB → parse → create job + one `job_item` per game → enqueue `DarkadiaMatchArgs` → flip `dispatch_complete`). `HandleImportNexorious` is a separate, unrelated handler.
- `internal/worker/tasks/enqueue.go` — `ArgsForJobType` and `FinalizeArgsForSource` already switch on `JobSourceDarkadia` to route the bespoke match→finalize chain (used by the retry handlers and the generic job-item resolve endpoint).
- `cmd/nexorious/serve.go` — registers `DarkadiaMatchWorker` + `DarkadiaFinalizeWorker` with River in two places (initial boot and the post-migration re-init).
- Frontend: `ui/frontend/src/routes/_authenticated/import-export.tsx` hardcodes two `<ImportCard>`s; `ui/frontend/src/types/import-export.ts` has an `ImportSource` enum + a hardcoded `getImportSourceDisplayInfo` map; `JobItemsDetails` is gated on `source === JobSource.DARKADIA`; `ui/frontend/src/api/import-export.ts` has `importDarkadiaCsv`.

## Architecture

### Package layout

Three layers, chosen so the dependency graph stays acyclic:

```
importmodel  (leaf: Game, Platform)
    ▲                 ▲
    │                 │
 mappers          importsource ──► mappers
 (darkadia, …)    (registry + Mapper interface)
    ▲                 ▲
    │                 │
  tasks ──────────────┘   (tasks → importmodel, and → importsource for a membership predicate)
    ▲
    │
   api ──► importsource + tasks
```

- **`internal/services/importmodel`** (new, leaf) — the canonical `Game` and `Platform` structs, moved verbatim from `darkadia` (identical JSON tags, so existing `source_metadata` payloads still unmarshal). No dependencies.

  ```go
  package importmodel

  import "errors"

  // ErrInvalidSignature is the shared sentinel a mapper returns (wrapped) when a
  // file is the wrong shape for that source. It lives in this leaf package so
  // both the mappers and the generic upload handler can reference it without an
  // import cycle (the handler does errors.Is(err, importmodel.ErrInvalidSignature)).
  var ErrInvalidSignature = errors.New("file does not match the expected source format")

  // Game is the canonical, source-neutral payload for one imported game,
  // marshalled verbatim into job_item.source_metadata.
  type Game struct {
      Title          string     `json:"title"`
      PlayStatus     string     `json:"play_status"`
      IsLoved        bool       `json:"is_loved"`
      PersonalRating *int32     `json:"personal_rating,omitempty"`
      PersonalNotes  *string    `json:"personal_notes,omitempty"`
      CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
      Platforms      []Platform `json:"platforms"`
      Tags           []string   `json:"tags,omitempty"`
      HoursPlayed    *float64   `json:"hours_played,omitempty"`
  }

  // Platform is one consolidated (platform, storefront, acquired_date) ownership entry.
  type Platform struct {
      Platform     string  `json:"platform"`                // Nexorious slug
      Storefront   *string `json:"storefront,omitempty"`    // slug or nil
      AcquiredDate string  `json:"acquired_date,omitempty"` // "2006-01-02" or ""
  }
  ```

- **mapper packages** (e.g. `internal/services/darkadia`, location unchanged) — import `importmodel`, expose `Parse(raw []byte) ([]importmodel.Game, error)`. A wrong-shape file returns an error wrapping `importmodel.ErrInvalidSignature`; `darkadia.ErrInvalidHeader` is kept but redefined to wrap that shared sentinel (`fmt.Errorf("not a Darkadia export (header mismatch): %w", importmodel.ErrInvalidSignature)`), so `TestParse_RejectsNonDarkadiaHeader` (which does `errors.Is(err, darkadia.ErrInvalidHeader)`) still passes *and* the generic handler can match on the shared sentinel. A mapper does **not** import `importsource`, so it stays independently testable. `darkadia.Parse` returns `[]importmodel.Game` directly — **no `type Game = importmodel.Game` alias**; the Darkadia code and its tests reference `importmodel.Game` where they need the type.

- **`internal/services/importsource`** (new, registry) — imports `importmodel` and every mapper; defines the `Mapper` interface and the registry table. Registration is **explicit** (the registry package imports each mapper and lists it), not `init()`-based blank-import magic.

  ```go
  package importsource

  type Mapper interface {
      // Parse maps a source export into canonical games. On a wrong-shape file it
      // returns an error wrapping importmodel.ErrInvalidSignature, which the
      // generic handler turns into a 400 "not a <DisplayName> export".
      Parse(raw []byte) ([]importmodel.Game, error)
  }

  type Source struct {
      Slug        string   // JobSource* value, e.g. "darkadia"
      DisplayName string   // "Darkadia"
      Description string   // picker blurb
      Features    []string // picker bullet list
      Accept      []string // file-input accept hints, e.g. [".csv", "text/csv"]
      Mapper      Mapper
  }

  func Lookup(slug string) (Source, bool)
  func All() []Source            // stable order for the picker
  func IsRegistered(slug string) bool
  ```

  The `Mapper` interface is satisfied structurally — `darkadia.Parse` is a package function, so the registry wraps it in a tiny adapter (`mapperFunc(darkadia.Parse)`) rather than requiring the `darkadia` package to know the interface exists.

### Canonical model & the multi-line invariant

`importmodel.Game.Platforms` is a **slice** because one-game-many-platforms is a first-class part of the canonical format. How a source *encodes* that multiplicity is a parsing concern owned entirely by its mapper.

**Cross-cutting invariant: multi-row consolidation lives in the mapper, never the pipeline.** Darkadia's file emits one row per *copy* — a named row plus zero or more continuation rows with an empty `Name`. `darkadia.Parse` groups those into `rawGame{named, copies}`, and `darkadia.consolidate` collapses each group into a single `importmodel.Game` with a `Platforms[]` that unions the aggregate `Platforms` column and the per-copy `Copy platform` rows, de-duped on `(platform, storefront)` (earliest acquired date wins). **All of this grouping logic moves verbatim into the refactor and keeps its existing tests** — the only change is the return type (`importmodel.Game` instead of `darkadia.Game`).

The generic pipeline never sees a "row" or a "copy". Its contract is `[]importmodel.Game`, one struct per game, each already carrying its full consolidated `Platforms[]`. A future source with a similar multi-row encoding does its own grouping in its own mapper; the pipeline stays oblivious. Most other sources emit multiplicity trivially (vglist: a JSON array field; Grouvee/Completionator/generic CSV: typically one row per game → a single- or zero-element `Platforms[]`).

### Generic workers

`internal/worker/tasks/darkadia.go` → `internal/worker/tasks/import_pipeline.go`, with the types renamed and the logic unchanged:

| Today | After |
|---|---|
| `DarkadiaMatchArgs` / `DarkadiaMatchWorker` (kind `darkadia_match`) | `ImportMatchArgs` / `ImportMatchWorker` (kind `import_match`) |
| `DarkadiaFinalizeArgs` / `DarkadiaFinalizeWorker` (kind `darkadia_finalize`) | `ImportFinalizeArgs` / `ImportFinalizeWorker` (kind `import_finalize`) |
| `DarkadiaCheckJobCompletion` | `ImportCheckJobCompletion` |
| `darkadiaMarkPendingReview` | `importMarkPendingReview` |

The finalize worker unmarshals `importmodel.Game`. **Zero logic change** — match (IGDB search on `item.SourceTitle` → confident auto-resolve or `pending_review`), finalize (ensure game row → additive merge of `user_games` + `user_game_platforms` + tags + playtime-on-first-platform → `changes` row → completion check), and the completion gate (`pending_review` blocks termination; `dispatch_complete` guard) are all carried over byte-for-byte except for the renames.

`cmd/nexorious/serve.go` registers the renamed workers with River in both registration sites.

### Source-routing helpers

`ArgsForJobType` and `FinalizeArgsForSource` in `enqueue.go` generalise from a hardcoded `JobSourceDarkadia` switch to a registry-membership check:

- `ArgsForJobType`: a job whose `source` is `importsource.IsRegistered(source)` routes a retried item back to `ImportMatchArgs` (re-enters at the match stage), regardless of the shared `import` job_type.
- `FinalizeArgsForSource`: any registered import source returns `ImportFinalizeArgs`; an unregistered source returns the existing "no interactive finalize stage" error.

`tasks` importing `importsource` for this predicate introduces no cycle (`importsource` does not import `tasks`).

### Parameterised upload handler

`HandleImportDarkadia` becomes one generic `handleImportSource(src importsource.Source)`:

1. require IGDB configured (same 400 as today);
2. read the upload (same 50 MB limit, same multipart handling);
3. `src.Mapper.Parse(body)` — on `errors.Is(err, importmodel.ErrInvalidSignature)`, return `400 "not a <DisplayName> export"`; on other parse errors, `400 "failed to parse: …"`; on zero games, `400 "no games found"`;
4. reject if an active import job for this `(user, source)` already exists (same 409);
5. create the job (`source = src.Slug`, `status = processing`, `dispatch_complete = false`), one `job_item` per game (payload = the `importmodel.Game` JSON), enqueue `ImportMatchArgs`;
6. flip `dispatch_complete = true`, then `ImportCheckJobCompletion`.

Routes are registered in a **loop over the registry** rather than a `:source` path param — this avoids colliding with the separate static `POST /import/nexorious` and keeps each source's route explicit:

```go
imh := NewImportHandler(db, riverClient, igdbClient)
importGroup.POST("/nexorious", imh.HandleImportNexorious) // unchanged, separate path
for _, src := range importsource.All() {
    importGroup.POST("/"+src.Slug, imh.handleImportSource(src))
}
```

### `GET /api/import/sources`

A new endpoint returns the registry (`slug`, `display_name`, `description`, `features[]`, `accept[]`) as JSON. It does not require IGDB to be configured — it only describes available sources; the IGDB-required guard stays on the upload itself. The Nexorious JSON importer is **not** in this list (it is a separate, non-migration path with its own card).

### Frontend

- `api/import-export.ts` — replace `importDarkadiaCsv` with a generic `importFromSource(slug, file)` posting to `/import/${slug}`; add `fetchImportSources()` hitting `/import/sources`.
- `import-export.tsx` — fetch the source list and render one `<ImportCard>` per registry entry (its `accept`, title, description, and features come from the response). Delete the hardcoded two-card block and the `getImportSourceDisplayInfo` map. The Nexorious JSON card stays (its own card, separate endpoint).
- `JobItemsDetails` gating switches from `source === JobSource.DARKADIA` to "source is a registry import source" (any source returned by `/import/sources`), so the manual-match box appears for every migration source.
- `types/import-export.ts` — the `ImportSource` enum / display-info map is removed in favour of the fetched shape.

## Testing

Tests follow the code: logic that becomes generic gets **generic** tests; the `darkadia` package keeps only the tests for genuinely Darkadia-specific behaviour.

- **`internal/services/importmodel`** — struct-only, no test.
- **`internal/services/darkadia`** — keeps its existing Parse / consolidation / mapping-table / status / rating tests (these *are* the Darkadia-specific behaviour), with type references updated to `importmodel.Game`. This is the only place a "Darkadia" test name still makes sense.
- **`internal/worker/tasks/import_pipeline_test.go`** (was `darkadia_test.go`) — the former worker tests, renamed source-neutral (`TestImportFinalize_*`, `TestImportMatch_*`, `TestImportCheckJobCompletion_*`) and `TestFinalizeArgsForSource` rewritten for the registry-membership routing. The generic workers never call a mapper (parsing happened at upload), so these tests construct `importmodel.Game` payloads directly. Same scenarios and coverage as today: no-IGDB → pending_review, confident finalize writes game+platforms, invalid play-status coerced to null, concurrent duplicate-game no failure, completion blocked until dispatch complete, additive merge does not overwrite, tags/playtime on first platform.
- **`internal/worker/tasks/import_roundtrip_test.go`** — must pass; type-reference renames only.
- **`internal/services/importsource`** — registry test: `Lookup` hit/miss, `IsRegistered`, `All()` includes Darkadia, unknown-source error.
- **`internal/api`** — generic handler: unknown/unregistered source rejected; signature-mismatch → 400 with the source display name; `GET /import/sources` returns the registry shape (and Darkadia is present).
- **Frontend** — `import-export.test.tsx` updated for the fetched-sources list; a registry response drives the rendered cards and the manual-match gating.

## Acceptance criteria

- [ ] `internal/services/importmodel.Game` exists; `darkadia.Parse` returns `[]importmodel.Game`.
- [ ] `internal/services/importsource` exists with the `Mapper` interface, the registry, and Darkadia registered as one entry.
- [ ] Workers, the upload handler, and the source-routing helpers are source-neutral and keyed by `source`; River job kinds are `import_match` / `import_finalize`; both `serve.go` registration sites updated.
- [ ] `GET /api/import/sources` returns the registry; the frontend picker renders from it; the manual-match box is gated on registry membership, not a hardcoded Darkadia check.
- [ ] **Darkadia import behaviour is unchanged** — every existing behavioural assertion still holds (reorganised under generic test names where the code became generic).

## Deliberate deviation from #1000's wording

#1000's acceptance criteria say the Darkadia tests "pass unchanged." This spec interprets that as **behavioural coverage preserved, tests reorganised to match the generic code** — not byte-identical files. The match/finalize/completion logic is becoming generic, so its tests become generic (renamed, source-neutral) rather than being pinned under the `Darkadia` name behind aliases kept only to avoid edits. The genuinely Darkadia-specific tests (parsing/consolidation) stay in the `darkadia` package. This will be noted on the PR that closes #1000.

## Out of scope

- vglist / Grouvee / Completionator / generic-CSV mappers (#1001–#1004).
- Recurring/incremental sync from any migration source — all are one-off migrations.
- Foreign-id (Wikidata/GiantBomb) cross-walk as a match signal — title matching only for v1.
- New storefront seeds for stores Nexorious doesn't model — preserved as provenance notes, per Darkadia.
- Any change to `HandleImportNexorious`.
