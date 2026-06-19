# CSV import: server-side auto mode (detect-or-guess at import time) — #1089

**Issue:** [#1089](https://github.com/drzero42/nexorious/issues/1089) — applies to the CSV-import track (epic #984) and the `nexctl` epic (#1060).
**Status:** design approved, ready for an implementation plan.
**Builds on:** #1021 (`csvmap.GuessColumns` + `SuggestedMapping`, the data-refined guess returned by `/api/import/csv/inspect`), #1015 (`detectPreset` / `MatchesSignature` signature detection), and #1060 Phase 5 (`nexctl import csv`, PR #1088).

## Problem

The CSV detect-or-guess *building blocks* already live on the server, but only behind the **inspect** endpoint:

- `POST /api/import/csv/inspect` parses the file and returns `detected` (a preset matched by header signature, via `detectPreset`) and `suggested_mapping` (a data-refined `csvmap.GuessColumns` guess).
- `POST /api/import/csv` then requires the caller to send an explicit preset `format` **or** a `mapping`. With neither it returns **400 "missing mapping field"** (`import_csv.go:275-277`). There is no import-time "detect a preset, else guess a mapping" path.

So the *decision* — "if a preset was detected use it; otherwise import with the guessed mapping" — is not a server capability at import time. Each client must call inspect, decide which default to use, then re-submit `POST /api/import/csv` with an explicit `format`/`mapping`. The web UI encodes that decision as the two `useState` initializers in `csv-mapping-dialog.tsx:87-89` (`format = inspect.detected?.slug ?? 'generic'`, `mapping = inspect.suggested_mapping ?? emptyCsvMapping()`) and then has the user confirm. `nexctl import csv <file>` (PR #1088) has no equivalent: with no `--preset` and no `--title-col` it errors `a --title-col is required (or use --preset)` (`import_csv.go:142`).

(Note: this is *not* "the web UI does client-side detection" — detection and guessing run server-side in inspect. What is duplicated/absent across clients is the *import-time application* of the detect-or-guess policy in a single call.)

## Scope

In:

- A **server auto path** on `POST /api/import/csv`: when **no `format` and no `mapping`** are supplied, the handler runs detect-or-guess itself (preset by signature, else a data-refined guessed mapping) and enqueues the import in one call.
- The detect-or-guess + data-refinement logic factored into a **shared helper** that both `HandleImportCSVInspect` and the new auto path call, so the inspect-time guess and the import-time guess cannot drift.
- A **transparent response**: the auto path reports which mode was taken (matched preset, or guessed mapping + the resolved mapping) so callers can show it. Non-auto responses are unchanged.
- A **specific 400** when auto cannot identify a title column, with guidance pointing at `--inspect` / `--preset` / manual mapping.
- **`nexctl`**: the no-flag `import csv <file>` path delegates to auto (instead of erroring) and prints what the server chose before the job confirmation.
- `cliclient.ImportResult` gains the auto-resolution fields; `cliclient.ImportCSV` already sends neither field when `format=="" && mapping==nil`, so no signature change.
- Tests: Go handler tests (preset-detected, guessed, no-title 400, regression of preset/manual/generic-no-mapping); a `cliclient` decode test; a `nexctl` no-flag test against an httptest stub.

Out (explicitly deferred / not in this issue):

- **Web UI changes.** The dialog already applies the policy client-side over the inspect result and lets the user review; a "one-click import with detected settings" button that delegates to the auto endpoint is a possible follow-up, not part of this issue.
- `export`, `import nexorious`, and registry import sources — unaffected.
- Canonical platform/storefront value matching, or any change to `csvmap.Parse` / the job model.
- Auto as an option on the *manual* path — supplying any manual column flag keeps the existing explicit behaviour (and its title-required error).

## Auto-mode trigger (settled with the user)

Auto triggers when **both `format` and `mapping` are empty/absent** on `POST /api/import/csv`. This is the only contract change: the former 400 for that exact case becomes an auto import.

A literal `format=generic` with no mapping **continues to 400** ("missing mapping field"): `generic` is the explicit "I will map columns manually" selection (per the existing handler comment), so it still requires a mapping. Empty-format means "decide for me". Splitting the two is intentional and documented in the handler.

Per the project's no-back-compat-for-solo-user stance, turning the empty/empty 400 into a success is an acceptable breaking change.

## Server design (`internal/api/import_csv.go`)

### Shared detect-or-guess helper

Extract the inspect handler's single-pass column scan + guess refinement into a reusable function, then build the policy on top:

```go
// scanCSV computes, in one pass over the data rows, the per-column distinct
// values (capped at csvDistinctCap) and a data-refined suggested mapping
// (rating scale from the observed max; status value-map from the status
// column's distinct values). records[0] is the header. It is the body of the
// current HandleImportCSVInspect scan, extracted verbatim so inspect and the
// auto import path share one implementation.
func scanCSV(records [][]string) (cols []csvColumnInfo, suggested csvmap.SuggestedMapping)

// csvAutoResolution describes how the auto path mapped a file.
type csvAutoResolution struct {
	Config   csvmap.Config            // the Config to Parse with
	Detected *csvPresetInfo           // non-nil when a preset signature matched
	Mapping  *csvmap.SuggestedMapping // non-nil when guessed (no preset matched)
}

// resolveCSVAuto applies the import-time detect-or-guess policy:
//   1. detectPreset(header) matched  -> that preset's Config (Detected set).
//   2. otherwise GuessColumns + refinement -> Config from the guess (Mapping set).
//   3. a guess with no title column -> errCSVAutoNoTitle.
func resolveCSVAuto(records [][]string) (csvAutoResolution, error)
```

`resolveCSVAuto`:
- `header := records[0]`.
- `if d := detectPreset(header); d != nil { cfg, _ := csvmap.PresetBySlug(d.Slug); return {Config: cfg, Detected: d}, nil }`.
- else: `_, suggested := scanCSV(records)`; if `suggested.Columns.Title == ""` return `errCSVAutoNoTitle`; `cfg, err := buildCSVConfig(suggestedToCSVMapping(suggested))`; return `{Config: cfg, Mapping: &suggested}`.

`suggestedToCSVMapping(csvmap.SuggestedMapping) csvMapping` is a field-for-field copy — both types share the identical column/status/rating/merge JSON shape — so the guessed mapping flows through the existing, tested `buildCSVConfig`.

`HandleImportCSVInspect` is refactored to `cols, suggested := scanCSV(records)` (no behaviour change; the existing inspect test guards this).

### Handler control flow (`HandleImportCSV`)

```
format := trim(FormValue("format"))
mapping := FormValue("mapping")

if format != "" && format != "generic":     // preset path (unchanged)
    cfg = PresetBySlug(format) or 400 "unknown CSV format"
elif format == "" && trim(mapping) == "":    // NEW auto path
    records = ReadRecords(body) (400 on parse error / empty)
    res, err = resolveCSVAuto(records)
        err == errCSVAutoNoTitle -> 400 with guidance
    cfg = res.Config
    autoResolution = res            // carried into the JSON response
else:                                         // manual path (unchanged)
    mapping == "" -> 400 "missing mapping field"   // covers format=generic + no mapping
    cfg = buildCSVConfig(parsed mapping)

games = csvmap.Parse(body, cfg)  ...  enqueueImportJob(...)
```

The no-title 400 message: `could not auto-detect a column mapping (no title column found). Use --inspect to see the headers, then --preset <slug> or column-mapping flags.`

### Response

Non-auto responses keep today's shape exactly. The auto path adds, to the existing `map[string]any` response, the resolution under an `auto` envelope:

```jsonc
{
  "job_id": "...", "source": "csv", "status": "processing",
  "message": "CSV import job created. Matching N games.", "total_items": N,
  "auto": {
    "mode": "preset",                 // or "guessed"
    "preset": { "slug": "grouvee", "name": "Grouvee" },   // mode==preset
    "mapping": { /* csvmap.SuggestedMapping */ }          // mode==guessed
  }
}
```

(`auto` omitted entirely on the preset/manual paths.)

## Client design

### `cliclient` (`internal/cliclient/import.go`)

`ImportResult` gains an optional auto envelope (mirrors the JSON; no new dependency):

```go
type CSVAutoResolution struct {
	Mode    string               `json:"mode"`              // "preset" | "guessed"
	Preset  *CSVPreset           `json:"preset,omitempty"`  // when mode=="preset"
	Mapping *CSVSuggestedMapping `json:"mapping,omitempty"` // when mode=="guessed"
}

type ImportResult struct {
	// ...existing fields...
	Auto *CSVAutoResolution `json:"auto,omitempty"`
}
```

`CSVPreset` and `CSVSuggestedMapping` already exist in this file. `ImportCSV(key, filename, data, "", nil)` already omits both form fields (`import.go:164-171`), so it issues an auto request as-is — no method change.

### `nexctl` (`cmd/nexctl/import_csv.go`)

Insert an auto branch *before* the manual-mapping path, taken when `preset == "" && !anyManual` (and not `--inspect`):

```go
if preset == "" && !anyManual {
	res, err := c.ImportCSV(p.Key, filename, data, "", nil)
	if err != nil {
		return fmt.Errorf("import CSV failed: %w", err)
	}
	if !flagBool(cmd, "json") {
		printCSVAutoResolution(cmd.OutOrStdout(), res.Auto)
	}
	return printImportResult(cmd, res)
}
```

`printCSVAutoResolution` prints nothing for a nil envelope, otherwise:
- `mode=="preset"`: `Auto-detected preset: <slug> (<name>)`.
- `mode=="guessed"`: `No preset matched; guessed column mapping:` then one `  <field>=<header>` line per non-empty assignment (title, igdb_id, platform, storefront, rating, notes, acquired_date, hours_played, tags, loved, status), then `Review the import job before applying.`

The existing manual path keeps its `a --title-col is required` error (reached only when some manual flag is set). Its hint text is updated to mention that running with no flags auto-detects: `a --title-col is required (or use --preset, or run with no flags to auto-detect)`.

## Testing

- **`internal/api/import_csv_test.go`**
  - auto + a Grouvee-signature CSV → 200, `auto.mode=="preset"`, `auto.preset.slug=="grouvee"`, job created.
  - auto + generic headers (`Name,Status,Hours`) with no signature match → 200, `auto.mode=="guessed"`, `auto.mapping.columns.title=="Name"`, job created.
  - auto + a CSV whose headers have no title-like column → 400 containing "no title column".
  - regression: `format=generic` + no mapping still 400 "missing mapping field"; preset path and manual-mapping path unchanged.
  - existing inspect test still green after the `scanCSV` extraction.
- **`internal/cliclient/import_test.go`** — `ImportCSV` against a stub returning the auto envelope decodes `res.Auto` (both `preset` and `guessed` shapes).
- **`cmd/nexctl/import_csv_test.go`** — no-flag `import csv <file>` against an httptest stub: a preset response prints `Auto-detected preset:`; a guessed response prints `No preset matched; guessed column mapping:` and a `title=...` line; `--json` suppresses the preamble.

## Files

- Modify: `internal/api/import_csv.go` — `scanCSV` extraction, `resolveCSVAuto`, `suggestedToCSVMapping`, `errCSVAutoNoTitle`, auto branch + response in `HandleImportCSV`, inspect refactor.
- Modify: `internal/cliclient/import.go` — `CSVAutoResolution`, `ImportResult.Auto`.
- Modify: `cmd/nexctl/import_csv.go` — auto branch, `printCSVAutoResolution`, manual hint text.
- Tests: `internal/api/import_csv_test.go`, `internal/cliclient/import_test.go`, `cmd/nexctl/import_csv_test.go`.

No migration, no new dependency, no web UI change.
