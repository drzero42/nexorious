# Absorb Darkadia into the csvmap engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire the bespoke `internal/services/darkadia` mapper by re-expressing Darkadia as a `csvmap.Config` value on the shared engine (zero behaviour change), register it as a CSV preset, and remove the now-orphaned Darkadia import card / job-source plumbing.

**Architecture:** Implement the four advanced `csvmap` engine features whose `Config` fields already exist from #1014 (`Status.Flags`, `Platform.Tables`, `Notes.Assembly`, `Grouping.CopyRows`) plus `Duration.Format == "h:mm"`, by porting the algorithms from `darkadia.go` into config-driven engine code. Define `Darkadia()` as a preset `Config`, route it through the existing CSV preset path (recorded as `JobSource.CSV`, like Completionator/Grouvee), and delete the bespoke package, its registry entry, the `JobSourceDarkadia` Go constant, and the frontend `JobSource.DARKADIA` enum/label. No backward-compat for in-flight/historical `darkadia`-sourced jobs is kept (decided out-of-band).

**Tech Stack:** Go 1.26 (stdlib `encoding/csv`, Bun, River), Vite/React/TypeScript/Vitest frontend.

---

## Context the engineer needs

- **The byte-for-byte contract** is the existing `internal/services/darkadia/darkadia_test.go` suite. The engine must reproduce its outputs exactly. Those tests call internal functions (`consolidate(rawGame{...})`, column-index constants) and therefore **cannot** survive the deletion verbatim — they are *relocated* into `internal/services/csvmap/` as CSV-driven tests asserting the same outputs (Task 5). "Relocate, don't weaken."
- **Two import paths exist today:**
  - *Registry path* (`importsource` → `internal/api/import.go:handleImportSource`): each source is a card on the import page and produces a job whose `source` is the source slug (e.g. `darkadia`). This is the path being removed for Darkadia.
  - *CSV preset path* (`internal/api/import_csv.go:HandleImportCSV`): the generic "CSV" card with a Format dropdown. Selecting a preset runs `csvmap.Parse(body, preset)` and records the job as `JobSource.CSV` ("csv"). This is the path Darkadia moves to.
- **The frontend import cards and routes are fully data-driven** from the `importsource` registry (`ui/frontend/src/routes/_authenticated/import-export.tsx` maps `importSources`; `internal/api/router.go:412` registers a POST route per registry entry). Deleting the registry entry removes the card and the `/api/import/darkadia` route with **no** other code change.
- **`csvmap` map-key matching is mixed on purpose:** the *simple* engine normalizes (`ToLower`+`TrimSpace`) lookup keys (`ValueMap`/`PlatformMap`/`StorefrontMap`). The Darkadia **platform table** is looked up **case-sensitively** by the raw trimmed platform string (e.g. `"PlayStation 4"`), exactly as `darkadia.platformTable` does — so its `Config` keys stay exact-case. The Darkadia **storefront table** is looked up normalized, so its keys stay lowercased. Do not "fix" either to match the other.
- **`cell()` trims** every value it returns. `darkadia` trims everywhere except the verbatim `Notes` cell, `Name`, and `Added`. The only observable difference is leading/trailing whitespace on verbatim notes, which no test exercises; using `cell()` uniformly is intentional and test-safe.
- **`ReadRecords`** already tolerates ragged rows (`FieldsPerRecord = -1`) and `cell()` returns `""` past a short row's end, so darkadia's ragged-row tolerance is inherited for free.
- Build backend: `make build`. Run a single Go test: `go test ./internal/services/csvmap/... -run TestName -v`. Frontend (from `ui/frontend/`): `npm run test <file>`, `npm run check`, `npm run knip`, `npm run build` (regenerates `routeTree.gen.ts`). Format/lint/build run automatically via hooks; the full suites run at `git push`.
- Commit type: this is a refactor with no behaviour change → use `refactor:` for the squash-merge title. (PR title is what release-please parses.)

## File Structure

**Create:**
- `internal/services/csvmap/advanced.go` — engine code for the advanced features (status flags, h:mm duration, copy-row grouping + table-driven platform consolidation + note assembly). Ports the darkadia algorithms, config-driven.
- `internal/services/csvmap/darkadia.go` — the `Darkadia()` preset `Config` value (tables become data).
- `internal/services/csvmap/advanced_test.go` — focused unit tests for the new engine primitives (`extractStatusFlags`, `parseHMM`).
- `internal/services/csvmap/darkadia_test.go` — the **relocated** Darkadia behaviour suite, CSV-driven against `Darkadia()`.

**Modify:**
- `internal/services/csvmap/parse.go` — dispatch to the copy-row grouping path; branch `extractHours` on `h:mm`.
- `internal/services/csvmap/validate.go` — allow the four advanced slots + `h:mm`; remove `notImplemented`; validate `CopyRows`.
- `internal/services/csvmap/presets.go` — register the `darkadia` preset.
- `internal/services/csvmap/presets_test.go` — assert the darkadia preset is registered.
- `internal/services/importsource/registry.go` — delete the Darkadia entry.
- `internal/services/importsource/registry_test.go` — delete darkadia-specific tests; re-point the registry-contract tests to vglist.
- `internal/db/models/jobs.go` — delete the `JobSourceDarkadia` constant.
- `internal/worker/tasks/import_pipeline_test.go` — delete `TestFinalizeArgsForSource` (now a duplicate of `enqueue_test.go`'s CSV coverage).
- `ui/frontend/src/types/jobs.ts` — delete the `JobSource.DARKADIA` enum member and its label-map entry.
- `ui/frontend/src/components/navigation/nav-items.test.tsx` — re-point the two badge tests to `vglist`.
- `ui/frontend/src/routes/_authenticated/import-export.test.tsx` — re-point the registry-source review-surface test to `vglist`.
- `ui/frontend/src/api/jobs.test.ts` — re-point the source-filter fixture to a live source.
- `ui/frontend/src/hooks/use-jobs.test.ts` — re-point the source-filter fixture to a live source.
- `ui/frontend/src/components/jobs/job-card.test.tsx` — delete the `DARKADIA` parameterized case and the `darkadia` mock-label entry.
- `docs/darkadia-import.md` — note the mapping is now a `csvmap.Config` and the import is reached via the CSV card's Format dropdown.

**Delete:**
- `internal/services/darkadia/darkadia.go`
- `internal/services/darkadia/darkadia_test.go`
- (the `internal/services/darkadia/` directory becomes empty and goes away)

---

### Task 1: Status-flags engine (`Status.Flags`)

**Files:**
- Create: `internal/services/csvmap/advanced.go`
- Create: `internal/services/csvmap/advanced_test.go`
- Modify: `internal/services/csvmap/validate.go`

- [ ] **Step 1: Write the failing unit test**

Create `internal/services/csvmap/advanced_test.go`:

```go
package csvmap

import "testing"

func TestExtractStatusFlags_PrecedenceAndDefault(t *testing.T) {
	sf := &StatusFlags{
		Rules: []FlagRule{
			{Column: "Dominated", Truthy: []string{"1"}, Status: "dominated"},
			{Column: "Mastered", Truthy: []string{"1"}, Status: "mastered"},
			{Column: "Finished", Truthy: []string{"1"}, Status: "completed"},
			{Column: "Shelved", Truthy: []string{"1"}, Status: "dropped"},
			{Column: "Playing", Truthy: []string{"1"}, Status: "in_progress"},
			{Column: "Played", Truthy: []string{"1"}, Status: "shelved"},
		},
		Default: "not_started",
	}
	header := []string{"Played", "Playing", "Finished", "Mastered", "Dominated", "Shelved"}
	idx := buildIndex(header)
	set := func(cols ...string) []string {
		rec := make([]string, len(header))
		for _, c := range cols {
			rec[idx[normKey(c)]] = "1"
		}
		return rec
	}
	cases := []struct {
		on   []string
		want string
	}{
		{nil, "not_started"},
		{[]string{"Played"}, "shelved"},
		{[]string{"Played", "Playing"}, "in_progress"},
		{[]string{"Shelved"}, "dropped"},
		{[]string{"Finished"}, "completed"},
		{[]string{"Mastered", "Finished"}, "mastered"},
		{[]string{"Dominated", "Mastered"}, "dominated"},
		{[]string{"Shelved", "Playing"}, "dropped"},
		{[]string{"Finished", "Shelved"}, "completed"},
	}
	for i, c := range cases {
		if got := extractStatusFlags(set(c.on...), idx, sf); got != c.want {
			t.Errorf("case %d (%v): got %q, want %q", i, c.on, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./internal/services/csvmap/ -run TestExtractStatusFlags -v`
Expected: FAIL — `undefined: extractStatusFlags`.

- [ ] **Step 3: Implement `extractStatusFlags`**

Create `internal/services/csvmap/advanced.go` with the package clause and this function (more functions are added in later tasks):

```go
package csvmap

import (
	"sort"
	"strconv"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// extractStatusFlags resolves play_status from ordered boolean-flag columns: the
// first rule (in order) whose column holds a truthy value wins; otherwise Default
// ("not_started" when Default is empty). Replaces darkadia.resolvePlayStatus.
func extractStatusFlags(rec []string, idx map[string]int, sf *StatusFlags) string {
	for _, rule := range sf.Rules {
		v := normKey(cell(rec, idx, rule.Column))
		if v == "" {
			continue
		}
		for _, t := range rule.Truthy {
			if normKey(t) == v {
				return rule.Status
			}
		}
	}
	if sf.Default != "" {
		return sf.Default
	}
	return "not_started"
}
```

> Note: `sort`, `strconv`, `strings`, and `importmodel` are imported here because Tasks 2–3 add functions in this file that use them. If your editor's import pruning runs between tasks, re-add them — the file will not compile standalone until Task 3 lands. To keep each task independently green, you may temporarily add `var _ = sort.Strings; var _ = strconv.Atoi; var _ = strings.TrimSpace; var _ importmodel.Game` and delete those lines in Task 3. (Cleanest: implement Tasks 1–3 before running the package build.)

- [ ] **Step 4: Allow `Status.Flags` in validation**

In `internal/services/csvmap/validate.go`, delete this block:

```go
	if cfg.Status.Flags != nil {
		return notImplemented("Status.Flags")
	}
```

(The mutual-exclusion check `Status.Column != nil && Status.Flags != nil` at the top of `validate` stays.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestExtractStatusFlags -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/csvmap/advanced.go internal/services/csvmap/advanced_test.go internal/services/csvmap/validate.go
git commit -m "refactor: csvmap status-flags engine (Status.Flags)"
```

---

### Task 2: `H:MM` duration

**Files:**
- Modify: `internal/services/csvmap/advanced.go`
- Modify: `internal/services/csvmap/advanced_test.go`
- Modify: `internal/services/csvmap/parse.go`
- Modify: `internal/services/csvmap/validate.go`

- [ ] **Step 1: Write the failing unit test**

Append to `internal/services/csvmap/advanced_test.go`:

```go
func TestParseHMM(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		{"148:00", ptrF(148)},
		{"10:30", ptrF(10.5)},
		{" 5 : 30 ", ptrF(5.5)},
		{"", nil},
		{"abc", nil},
		{"1:2:3", nil},
		{"0:00", nil},
	}
	for _, c := range cases {
		got := parseHMM(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("parseHMM(%q) = %v, want nil", c.in, *got)
		case c.want != nil && (got == nil || *got != *c.want):
			t.Errorf("parseHMM(%q) = %v, want %v", c.in, got, *c.want)
		}
	}
}

func ptrF(f float64) *float64 { return &f }
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestParseHMM -v`
Expected: FAIL — `undefined: parseHMM`.

- [ ] **Step 3: Implement `parseHMM`**

Append to `internal/services/csvmap/advanced.go`:

```go
// parseHMM parses Darkadia "H:MM" playtime into hours. "148:00" -> 148.0,
// "10:30" -> 10.5. Empty, malformed, or non-positive -> nil. Replaces
// darkadia.parseDuration.
func parseHMM(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return nil
	}
	v := float64(h) + float64(m)/60.0
	if v <= 0 {
		return nil
	}
	return &v
}
```

- [ ] **Step 4: Branch `extractHours` on the format**

In `internal/services/csvmap/parse.go`, replace the body of `extractHours` (currently the decimal-only version) with:

```go
// extractHours parses the hours_played cell per cfg.Duration.Format: "decimal"
// (strconv float) or "h:mm" (H:MM clock).
func extractHours(rec []string, idx map[string]int, cfg Config) *float64 {
	if cfg.Duration == nil {
		return nil
	}
	raw := cell(rec, idx, cfg.Columns.HoursPlayed)
	if raw == "" {
		return nil
	}
	if normKey(cfg.Duration.Format) == "h:mm" {
		return parseHMM(raw)
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 {
		return nil
	}
	return &f
}
```

- [ ] **Step 5: Allow `h:mm` in validation**

In `internal/services/csvmap/validate.go`, change the `Duration` switch so `h:mm` is accepted instead of rejected:

```go
	if cfg.Duration != nil {
		switch normKey(cfg.Duration.Format) {
		case "decimal", "h:mm":
		default:
			return fmt.Errorf("csvmap: Duration.Format must be %q or %q, got %q", "decimal", "h:mm", cfg.Duration.Format)
		}
	}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestParseHMM -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/services/csvmap/advanced.go internal/services/csvmap/advanced_test.go internal/services/csvmap/parse.go internal/services/csvmap/validate.go
git commit -m "refactor: csvmap h:mm duration format"
```

---

### Task 3: Copy-row grouping + table-driven platform consolidation + note assembly

This is the core port of `darkadia.consolidate` / `resolveStorefront` / `recognizedStorefront` / `effectiveSource` / `splitAggregate` and the blank-name continuation grouping. It implements `Grouping.CopyRows`, `Platform.Tables`, and `Notes.Assembly` together because consolidation needs all three.

**Files:**
- Modify: `internal/services/csvmap/advanced.go`
- Modify: `internal/services/csvmap/parse.go`
- Modify: `internal/services/csvmap/validate.go`

There is no standalone unit test here — the behaviour is verified end-to-end by the relocated Darkadia suite in Task 5 (the byte-for-byte contract). This task makes the engine code compile and dispatch correctly.

- [ ] **Step 1: Implement the consolidation helpers**

Append to `internal/services/csvmap/advanced.go`:

```go
// splitAggregate splits a comma-separated owned-platform list, trimming and
// dropping empties. Ported from darkadia.splitAggregate.
func splitAggregate(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// effectiveSource returns the source string for a copy row: SourceColumn, unless
// it equals OtherSentinel (case-insensitive), in which case SourceOtherColumn.
func effectiveSource(pt *PlatformTables, rec []string, idx map[string]int) string {
	src := cell(rec, idx, pt.SourceColumn)
	if pt.OtherSentinel != "" && strings.EqualFold(src, pt.OtherSentinel) {
		return cell(rec, idx, pt.SourceOtherColumn)
	}
	return src
}

// recognizedStorefront returns the storefront slug for a recognized digital
// source (normalized exact match, else longest recognized prefix followed by a
// space). Ported from darkadia.recognizedStorefront.
func recognizedStorefront(pt *PlatformTables, eff string) (string, bool) {
	k := normKey(eff)
	if slug, ok := pt.Storefronts[k]; ok {
		return slug, true
	}
	names := make([]string, 0, len(pt.Storefronts))
	for name := range pt.Storefronts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j]) // longer first
		}
		return names[i] < names[j] // lexicographic tie-break
	})
	for _, name := range names {
		if strings.HasPrefix(k, name+" ") {
			return pt.Storefronts[name], true
		}
	}
	return "", false
}

// resolveStorefront applies the per-copy storefront precedence: recognized
// source -> physical (with provenance note) -> unrecognized source (note only)
// -> inferred -> none. Ported from darkadia.resolveStorefront.
func resolveStorefront(pt *PlatformTables, inferred *string, eff, media string) (*string, string) {
	if eff != "" {
		if slug, ok := recognizedStorefront(pt, eff); ok {
			s := slug
			return &s, ""
		}
	}
	if pt.MediaPhysicalValue != "" && media == pt.MediaPhysicalValue {
		s := "physical"
		note := ""
		if eff != "" {
			note = "Purchased physically from " + eff + "."
		}
		return &s, note
	}
	if eff != "" {
		return nil, "Purchased from " + eff + "."
	}
	if inferred != nil {
		s := *inferred
		return &s, ""
	}
	return nil, ""
}
```

- [ ] **Step 2: Implement grouping + consolidation**

Append to `internal/services/csvmap/advanced.go`:

```go
// buildGrouped implements blank-continuation grouping (Grouping.CopyRows): a row
// whose ContinuationColumn is non-blank starts a new game; each following blank
// row is a copy of the current game. Each group is consolidated via the platform
// tables / note assembly. Replaces darkadia.Parse's grouping loop.
func buildGrouped(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	contCol := cfg.Grouping.CopyRows.ContinuationColumn
	type group struct {
		named  []string
		copies [][]string
	}
	var groups []group
	for _, rec := range rows {
		if cell(rec, idx, contCol) != "" {
			groups = append(groups, group{named: rec, copies: [][]string{rec}})
			continue
		}
		if len(groups) == 0 {
			continue // continuation before any named row — malformed; skip defensively
		}
		g := &groups[len(groups)-1]
		g.copies = append(g.copies, rec)
	}
	out := make([]importmodel.Game, 0, len(groups))
	for _, grp := range groups {
		out = append(out, consolidateGroup(grp.named, grp.copies, idx, cfg))
	}
	return out
}

// consolidateGroup builds one Game from a named row plus its copy rows, applying
// the platform tables, storefront precedence, (platform, storefront) dedup
// (earliest acquired date kept), and ordered note assembly. Ported from
// darkadia.consolidate.
func consolidateGroup(named []string, copies [][]string, idx map[string]int, cfg Config) importmodel.Game {
	pt := cfg.Platform.Tables
	g := importmodel.Game{
		Title:      cell(named, idx, cfg.Columns.Title),
		PlayStatus: extractStatusFlags(named, idx, cfg.Status.Flags),
		IsLoved:    extractLoved(named, idx, cfg),
		CreatedAt:  extractDate(cell(named, idx, cfg.Columns.CreatedAt), cfg),
	}
	if r := extractRating(cell(named, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
	g.Tags = extractTags(named, idx, cfg)
	if h := extractHours(named, idx, cfg); h != nil {
		g.HoursPlayed = h
	}

	var noteLines []string
	addNote := func(line string) {
		if line == "" {
			return
		}
		for _, e := range noteLines {
			if e == line {
				return
			}
		}
		noteLines = append(noteLines, line)
	}

	// (1) Review line.
	if a := cfg.Notes.Assembly; a != nil {
		if rev := cell(named, idx, a.ReviewColumn); rev != "" {
			if subj := cell(named, idx, a.ReviewSubjectColumn); subj != "" {
				addNote("Review — " + subj + "\n" + rev)
			} else {
				addNote("Review: " + rev)
			}
		}
	}

	// (2) Owned set from aggregate + per-copy platform strings (unmapped -> note).
	owned := map[string]bool{}
	ownedInferred := map[string]*string{}
	markOwned := func(s string) {
		m, ok := pt.Platforms[s]
		if !ok {
			addNote("Owned on " + s + " (no Nexorious platform mapping).")
			return
		}
		owned[m.Slug] = true
		if m.InferredStorefront != nil && ownedInferred[m.Slug] == nil {
			ownedInferred[m.Slug] = m.InferredStorefront
		}
	}
	for _, s := range splitAggregate(cell(named, idx, pt.AggregateColumn)) {
		markOwned(s)
	}
	for _, row := range copies {
		if p := cell(row, idx, pt.PlatformColumn); p != "" {
			markOwned(p)
		}
	}

	// Dedup on (platform, storefront), keeping the earliest acquired date.
	type key struct{ platform, storefront string }
	seen := map[key]int{}
	add := func(slug string, sfp *string, date string) {
		sfKey := ""
		if sfp != nil {
			sfKey = *sfp
		}
		k := key{slug, sfKey}
		if i, ok := seen[k]; ok {
			if date != "" && (g.Platforms[i].AcquiredDate == "" || date < g.Platforms[i].AcquiredDate) {
				g.Platforms[i].AcquiredDate = date
			}
			return
		}
		seen[k] = len(g.Platforms)
		g.Platforms = append(g.Platforms, importmodel.Platform{Platform: slug, Storefront: sfp, AcquiredDate: date})
	}

	// (3) Per-copy storefront resolution + provenance notes.
	slugHasCopy := map[string]bool{}
	for _, row := range copies {
		ps := cell(row, idx, pt.PlatformColumn)
		if ps == "" {
			continue
		}
		m, ok := pt.Platforms[ps]
		if !ok {
			continue // already noted via markOwned
		}
		slugHasCopy[m.Slug] = true
		sfp, note := resolveStorefront(pt, m.InferredStorefront, effectiveSource(pt, row, idx), cell(row, idx, pt.MediaColumn))
		addNote(note)
		add(m.Slug, sfp, cell(row, idx, pt.PurchaseDateColumn))
	}

	// (4) Copy notes.
	if a := cfg.Notes.Assembly; a != nil && a.CopyNoteColumn != "" {
		for _, row := range copies {
			if cn := cell(row, idx, a.CopyNoteColumn); cn != "" {
				addNote("Copy note: " + cn)
			}
		}
	}

	// Owned-but-no-copy slugs, in sorted order, with any inferred storefront.
	slugs := make([]string, 0, len(owned))
	for s := range owned {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		if slugHasCopy[slug] {
			continue
		}
		add(slug, ownedInferred[slug], "")
	}

	// Verbatim notes column + assembled note lines.
	verbatim := cell(named, idx, cfg.Notes.Column)
	var b strings.Builder
	if verbatim != "" {
		b.WriteString(verbatim)
	}
	if len(noteLines) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.Join(noteLines, "\n"))
	}
	if b.Len() > 0 {
		s := b.String()
		g.PersonalNotes = &s
	}
	return g
}
```

- [ ] **Step 3: Dispatch to the grouped path**

In `internal/services/csvmap/parse.go`, change `buildGames` so `CopyRows` takes precedence:

```go
// buildGames dispatches between copy-row grouping, merge-by-title, and one-row.
func buildGames(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	if cfg.Grouping.CopyRows != nil {
		return buildGrouped(rows, idx, cfg)
	}
	if cfg.Grouping.MergeByTitle {
		return buildMerged(rows, idx, cfg)
	}
	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		if g, ok := extractGame(rec, idx, cfg); ok {
			games = append(games, g)
		}
	}
	return games
}
```

- [ ] **Step 4: Allow the advanced slots + validate `CopyRows`**

In `internal/services/csvmap/validate.go`, delete these three blocks:

```go
	if cfg.Platform.Tables != nil {
		return notImplemented("Platform.Tables")
	}
	if cfg.Notes.Assembly != nil {
		return notImplemented("Notes.Assembly")
	}
	if cfg.Grouping.CopyRows != nil {
		return notImplemented("Grouping.CopyRows")
	}
```

Then add, just before the final `return nil`:

```go
	if cfg.Grouping.CopyRows != nil {
		if strings.TrimSpace(cfg.Grouping.CopyRows.ContinuationColumn) == "" {
			return errors.New("csvmap: Grouping.CopyRows requires ContinuationColumn")
		}
		if cfg.Grouping.MergeByTitle {
			return errors.New("csvmap: Grouping.CopyRows and MergeByTitle are mutually exclusive")
		}
	}
```

Finally delete the now-unused `notImplemented` function at the bottom of the file:

```go
// notImplemented is returned for an advanced Config slot whose behaviour lands in #1016.
func notImplemented(feature string) error {
	return fmt.Errorf("csvmap: %s is not implemented yet (advanced feature, see #1016)", feature)
}
```

> After deleting `notImplemented`, `fmt` is still used by `validate.go` (the Rating/Duration error messages), so keep the `fmt` import. `errors` and `strings` are already imported. If `go build` reports an unused import, follow its guidance.

- [ ] **Step 5: Build the package**

Run: `go build ./internal/services/csvmap/...`
Expected: builds clean. If the temporary `var _ = ...` lines from Task 1 Step 3 still exist, delete them now (all of `sort`, `strconv`, `strings`, `importmodel` are genuinely used by this file as of this task).

- [ ] **Step 6: Run the existing csvmap tests to confirm no regression**

Run: `go test ./internal/services/csvmap/ -v`
Expected: PASS (existing simple-engine + new unit tests; the relocated darkadia suite lands in Task 5).

- [ ] **Step 7: Commit**

```bash
git add internal/services/csvmap/advanced.go internal/services/csvmap/parse.go internal/services/csvmap/validate.go
git commit -m "refactor: csvmap copy-row grouping, platform tables, note assembly"
```

---

### Task 4: Define the `Darkadia()` preset Config and register it

**Files:**
- Create: `internal/services/csvmap/darkadia.go`
- Modify: `internal/services/csvmap/presets.go`
- Modify: `internal/services/csvmap/presets_test.go`

- [ ] **Step 1: Write the failing preset-registration test**

Read `internal/services/csvmap/presets_test.go` first to match its style, then add:

```go
func TestPresets_IncludesDarkadia(t *testing.T) {
	cfg, ok := PresetBySlug("darkadia")
	if !ok {
		t.Fatal("darkadia preset not registered")
	}
	if cfg.Columns.Title != "Name" {
		t.Errorf("Title column = %q, want Name", cfg.Columns.Title)
	}
	if cfg.Platform.Tables == nil {
		t.Error("darkadia preset must use Platform.Tables")
	}
	found := false
	for _, p := range Presets() {
		if p.Slug == "darkadia" && p.DisplayName == "Darkadia" {
			found = true
		}
	}
	if !found {
		t.Error("Presets() omits the darkadia entry")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestPresets_IncludesDarkadia -v`
Expected: FAIL — `darkadia preset not registered`.

- [ ] **Step 3: Define `Darkadia()`**

Create `internal/services/csvmap/darkadia.go`:

```go
package csvmap

// Darkadia returns the preset Config for a Darkadia CSV export (the now-defunct
// game tracker). Darkadia is a one-row-per-copy format: a game's identity and
// game-level attributes live on its first (named) row; blank-Name rows continue
// the previous game as additional copies. Status comes from cumulative
// achievement flags (Dominated > Mastered > Finished > Shelved > Playing >
// Played); ownership is consolidated from the aggregate "Platforms" column and
// per-copy "Copy platform" rows, with storefront resolved from the copy's source
// / media. See docs/darkadia-import.md. This Config replaces the bespoke
// internal/services/darkadia mapper (#1016) with zero behaviour change.
func Darkadia() Config {
	psn := "playstation-store"
	xboxStore := "microsoft-store"
	return Config{
		// The 29 required columns of a Darkadia export (0–28); optional
		// feature-toggle columns (Tags, Time played, Review…) are read when present.
		Signature: []string{
			"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
			"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
			"Copy platform", "Copy media", "Copy media other", "Copy source",
			"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
			"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
			"Copy complete", "Copy complete notes", "Platforms", "Notes",
		},
		Columns: ColumnMap{
			Title:       "Name",
			Rating:      "Rating",
			CreatedAt:   "Added",
			HoursPlayed: "Time played",
			Tags:        "Tags",
			Loved:       "Loved",
		},
		TruthyValues: []string{"1"},
		Status: StatusConfig{
			Flags: &StatusFlags{
				Rules: []FlagRule{
					{Column: "Dominated", Truthy: []string{"1"}, Status: "dominated"},
					{Column: "Mastered", Truthy: []string{"1"}, Status: "mastered"},
					{Column: "Finished", Truthy: []string{"1"}, Status: "completed"},
					{Column: "Shelved", Truthy: []string{"1"}, Status: "dropped"},
					{Column: "Playing", Truthy: []string{"1"}, Status: "in_progress"},
					{Column: "Played", Truthy: []string{"1"}, Status: "shelved"},
				},
				Default: "not_started",
			},
		},
		Rating:   &RatingConfig{Scale: 5, Truncate: true},
		Duration: &DurationConfig{Format: "h:mm"},
		Grouping: GroupingConfig{
			CopyRows: &CopyRowGrouping{ContinuationColumn: "Name"},
		},
		Notes: NotesConfig{
			Column: "Notes",
			Assembly: &NoteAssembly{
				ReviewSubjectColumn: "Review subject",
				ReviewColumn:        "Review",
				CopyNoteColumn:      "Copy notes",
			},
		},
		Platform: PlatformConfig{
			Tables: &PlatformTables{
				AggregateColumn:    "Platforms",
				PlatformColumn:     "Copy platform",
				SourceColumn:       "Copy source",
				SourceOtherColumn:  "Copy source other",
				OtherSentinel:      "Other",
				MediaColumn:        "Copy media",
				MediaPhysicalValue: "Physical",
				PurchaseDateColumn: "Copy purchase date",
				// Looked up case-sensitively by the raw trimmed platform string.
				Platforms: map[string]PlatformMapping{
					"PC":                         {Slug: "pc-windows"},
					"Linux":                      {Slug: "pc-linux"},
					"Mac":                        {Slug: "mac"},
					"PlayStation 4":              {Slug: "playstation-4"},
					"PlayStation 5":              {Slug: "playstation-5"},
					"PlayStation 3":              {Slug: "playstation-3"},
					"PlayStation Network (PS3)":  {Slug: "playstation-3", InferredStorefront: &psn},
					"PlayStation Network (Vita)": {Slug: "playstation-vita", InferredStorefront: &psn},
					"Nintendo Switch":            {Slug: "nintendo-switch"},
					"Wii":                        {Slug: "nintendo-wii"},
					"Xbox 360":                   {Slug: "xbox-360"},
					"Xbox 360 Games Store":       {Slug: "xbox-360", InferredStorefront: &xboxStore},
					"Android":                    {Slug: "android"},
					"PlayStation 2":              {Slug: "playstation-2"},
					"PlayStation Network (PSP)":  {Slug: "playstation-psp", InferredStorefront: &psn},
				},
				// Looked up normalized (lowercased + trimmed); keys stay lowercased.
				Storefronts: map[string]string{
					"sony entertainment network": "playstation-store",
					"epic games store":           "epic-games-store",
					"epic game store":            "epic-games-store",
					"epic gamestore":             "epic-games-store",
					"epic":                       "epic-games-store",
					"gog":                        "gog",
					"humble bundle":              "humble-bundle",
					"steam":                      "steam",
					"nintendo eshop":             "nintendo-eshop",
					"origin":                     "origin-ea-app",
					"gamersgate":                 "gamersgate",
					"google play":                "google-play-store",
					"uplay":                      "uplay",
					"ubisoft club":               "uplay",
				},
			},
		},
	}
}
```

- [ ] **Step 4: Register the preset**

In `internal/services/csvmap/presets.go`, add the darkadia entry to `presetList`:

```go
var presetList = []Preset{
	{Slug: "completionator", DisplayName: "Completionator", Config: Completionator()},
	{Slug: "grouvee", DisplayName: "Grouvee", Config: Grouvee()},
	{Slug: "darkadia", DisplayName: "Darkadia", Config: Darkadia()},
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestPresets_IncludesDarkadia -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/csvmap/darkadia.go internal/services/csvmap/presets.go internal/services/csvmap/presets_test.go
git commit -m "refactor: add Darkadia csvmap preset"
```

---

### Task 5: Relocate the Darkadia behaviour suite (byte-for-byte contract)

Translate `internal/services/darkadia/darkadia_test.go` into CSV-driven tests against `Darkadia()`. The original asserts the same outputs via internal calls; here we drive real CSV through `csvmap.Parse`. This is the guard that the engine reproduces Darkadia exactly.

**Files:**
- Create: `internal/services/csvmap/darkadia_test.go`

- [ ] **Step 1: Write the relocated suite**

Create `internal/services/csvmap/darkadia_test.go`:

```go
package csvmap

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// dkHeader is the canonical 29-column required Darkadia header.
var dkHeader = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
}

// dkExtHeader appends the 5 optional feature-toggle columns an extended export adds.
var dkExtHeader = append(append([]string{}, dkHeader...),
	"Tags", "Time played", "Review subject", "Review", "Copy notes")

// dkRow builds one CSV record for header h: Name plus column overrides by header name.
func dkRow(h []string, name string, set map[string]string) []string {
	idx := map[string]int{}
	for i, c := range h {
		idx[c] = i
	}
	rec := make([]string, len(h))
	rec[idx["Name"]] = name
	for col, val := range set {
		rec[idx[col]] = val
	}
	return rec
}

// dkCSV renders header h plus rows into RFC-4180 CSV bytes.
func dkCSV(h []string, rows ...[]string) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write(h); err != nil {
		panic(err)
	}
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			panic(err)
		}
	}
	w.Flush()
	return b.Bytes()
}

// parseDK parses CSV bytes with the Darkadia preset, failing the test on error.
func parseDK(t *testing.T, raw []byte) []importmodel.Game {
	t.Helper()
	games, err := Parse(raw, Darkadia())
	if err != nil {
		t.Fatalf("Parse(Darkadia): %v", err)
	}
	return games
}

func TestDarkadia_AcceptsExtendedHeaderAndStatusByName(t *testing.T) {
	// Played=1, not finished -> "shelved"; PC copy bought on Steam, 148h, tagged.
	row := dkRow(dkExtHeader, "Game X", map[string]string{
		"Owned": "1", "Played": "1", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Steam", "Copy purchase date": "2013-06-05",
		"Platforms": "PC", "Time played": "148:00", "Tags": "Co-op, VR", "Notes": "my note",
	})
	games := parseDK(t, dkCSV(dkExtHeader, row))
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Title != "Game X" {
		t.Errorf("title = %q", g.Title)
	}
	if len(g.Platforms) == 0 || g.Platforms[0].Platform != "pc-windows" {
		t.Errorf("platforms = %+v", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "my note") {
		t.Errorf("notes = %v", g.PersonalNotes)
	}
	if g.PlayStatus != "shelved" {
		t.Errorf("play_status = %q, want shelved", g.PlayStatus)
	}
}

func TestDarkadia_RejectsNonDarkadiaHeader(t *testing.T) {
	_, err := Parse([]byte("foo,bar,baz\n1,2,3\n"), Darkadia())
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatalf("err = %v, want wrapping ErrInvalidSignature", err)
	}
}

func TestDarkadia_GroupsRowsIntoGamesAndCopies(t *testing.T) {
	gameA := dkRow(dkHeader, "Game A", map[string]string{
		"Owned": "1", "Copy platform": "PC", "Copy media": "Digital", "Copy source": "Steam",
		"Copy purchase date": "2013-06-05", "Platforms": "PC", "Notes": "note A",
	})
	contA := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "Mac", "Copy media": "Digital", "Copy source": "GOG",
		"Copy purchase date": "2014-01-01",
	})
	gameB := dkRow(dkHeader, "Game B", map[string]string{
		"Owned": "1", "Played": "1", "Loved": "1", "Rating": "4.5",
		"Copy platform": "PlayStation 4", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network", "Copy purchase date": "2015-02-02",
		"Platforms": "PlayStation 4", "Notes": "note B",
	})
	games := parseDK(t, dkCSV(dkHeader, gameA, contA, gameB))
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[0].Title != "Game A" || games[1].Title != "Game B" {
		t.Fatalf("titles = %q, %q", games[0].Title, games[1].Title)
	}
}

func TestDarkadia_ToleratesRaggedRowsAndEmbeddedNewline(t *testing.T) {
	// A ragged short row (10 fields) and a row with an embedded newline in Notes.
	raw := []byte(strings.Join(dkHeader, ",") + "\n" +
		`Ragged,2013-06-05,0,1,0,0,0,0,0,0` + "\n" +
		`Multi,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","line one` + "\n" + `line two"` + "\n")
	games := parseDK(t, raw)
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[1].PersonalNotes == nil || !strings.Contains(*games[1].PersonalNotes, "line one\nline two") {
		t.Fatalf("embedded newline not preserved: %+v", games[1].PersonalNotes)
	}
}

func TestDarkadia_PlayStatusPrecedence(t *testing.T) {
	cases := []struct {
		on   map[string]string
		want string
	}{
		{map[string]string{"Owned": "1"}, "not_started"},
		{map[string]string{"Owned": "1", "Played": "1"}, "shelved"},
		{map[string]string{"Owned": "1", "Played": "1", "Playing": "1"}, "in_progress"},
		{map[string]string{"Owned": "1", "Shelved": "1"}, "dropped"},
		{map[string]string{"Owned": "1", "Finished": "1"}, "completed"},
		{map[string]string{"Mastered": "1", "Finished": "1"}, "mastered"},
		{map[string]string{"Dominated": "1", "Mastered": "1"}, "dominated"},
		{map[string]string{"Shelved": "1", "Playing": "1"}, "dropped"},
		{map[string]string{"Finished": "1", "Shelved": "1"}, "completed"},
	}
	for i, c := range cases {
		row := dkRow(dkHeader, "G", c.on)
		g := parseDK(t, dkCSV(dkHeader, row))[0]
		if g.PlayStatus != c.want {
			t.Errorf("case %d: play_status = %q, want %q", i, g.PlayStatus, c.want)
		}
	}
}

func TestDarkadia_RatingTruncatedLovedCreatedAtNotes(t *testing.T) {
	row := dkRow(dkHeader, "G", map[string]string{
		"Owned": "1", "Loved": "1", "Rating": "4.5", "Added": "2013-06-05", "Notes": "my note",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if g.PersonalRating == nil || *g.PersonalRating != 4 {
		t.Errorf("rating = %v, want 4", g.PersonalRating)
	}
	if !g.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if g.CreatedAt != "2013-06-05" {
		t.Errorf("created_at = %q, want 2013-06-05", g.CreatedAt)
	}
	if g.PersonalNotes == nil || *g.PersonalNotes != "my note" {
		t.Errorf("notes = %v, want verbatim", g.PersonalNotes)
	}
}

func TestDarkadia_EmptyAndZeroRatingUnrated(t *testing.T) {
	for _, rating := range []string{"", "0"} {
		row := dkRow(dkHeader, "G", map[string]string{"Owned": "1", "Rating": rating})
		g := parseDK(t, dkCSV(dkHeader, row))[0]
		if g.PersonalRating != nil {
			t.Errorf("rating %q -> %v, want nil", rating, g.PersonalRating)
		}
	}
}

func TestDarkadia_PCWithGOGCopy_MacNoCopy(t *testing.T) {
	row := dkRow(dkHeader, "Anodyne", map[string]string{
		"Owned": "1", "Platforms": "PC, Mac",
		"Copy platform": "PC", "Copy media": "Digital", "Copy source": "GOG",
		"Copy purchase date": "2014-03-01",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	got := map[string]*string{}
	dates := map[string]string{}
	for _, p := range g.Platforms {
		got[p.Platform] = p.Storefront
		dates[p.Platform] = p.AcquiredDate
	}
	if len(g.Platforms) != 2 {
		t.Fatalf("platforms = %+v, want pc-windows+mac", g.Platforms)
	}
	if got["pc-windows"] == nil || *got["pc-windows"] != "gog" || dates["pc-windows"] != "2014-03-01" {
		t.Errorf("pc-windows = %v (%q), want gog/2014-03-01", got["pc-windows"], dates["pc-windows"])
	}
	if sf, ok := got["mac"]; !ok || sf != nil {
		t.Errorf("mac = %v, want present with nil storefront", sf)
	}
}

func TestDarkadia_PS3andPS4_viaPSN(t *testing.T) {
	named := dkRow(dkHeader, "Aaru's Awakening", map[string]string{
		"Owned": "1", "Platforms": "PlayStation Network (PS3), PlayStation 4",
		"Copy platform": "PlayStation Network (PS3)", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network", "Copy purchase date": "2015-02-02",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PlayStation 4", "Copy media": "Digital",
		"Copy source": "Sony Entertainment Network",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
	got := map[string]*string{}
	for _, p := range g.Platforms {
		got[p.Platform] = p.Storefront
	}
	if got["playstation-3"] == nil || *got["playstation-3"] != "playstation-store" {
		t.Errorf("ps3 = %v, want playstation-store", got["playstation-3"])
	}
	if got["playstation-4"] == nil || *got["playstation-4"] != "playstation-store" {
		t.Errorf("ps4 = %v, want playstation-store", got["playstation-4"])
	}
}

func TestDarkadia_StorefrontRules(t *testing.T) {
	// Physical media -> "physical" storefront + retailer provenance note.
	phys := dkRow(dkHeader, "Phys", map[string]string{
		"Owned": "1", "Platforms": "PlayStation 4", "Copy platform": "PlayStation 4",
		"Copy media": "Physical", "Copy source": "GameStop",
	})
	g := parseDK(t, dkCSV(dkHeader, phys))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "physical" {
		t.Errorf("physical storefront = %v", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "GameStop") {
		t.Errorf("physical retailer not in notes: %v", g.PersonalNotes)
	}

	// Unrecognized digital source -> nil storefront + provenance note.
	unrec := dkRow(dkHeader, "Unrec", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Fanatical",
	})
	g = parseDK(t, dkCSV(dkHeader, unrec))[0]
	if g.Platforms[0].Storefront != nil {
		t.Errorf("unrecognized digital storefront = %v, want nil", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Fanatical") {
		t.Errorf("unrecognized source not in notes: %v", g.PersonalNotes)
	}

	// Empty source, digital -> nil storefront, no note.
	empty := dkRow(dkHeader, "Empty", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
	})
	g = parseDK(t, dkCSV(dkHeader, empty))[0]
	if g.Platforms[0].Storefront != nil || g.PersonalNotes != nil {
		t.Errorf("empty source: storefront=%v notes=%v, want nil/nil", g.Platforms[0].Storefront, g.PersonalNotes)
	}

	// "Other" sentinel -> Copy source other; spelling variant maps to epic.
	epic := dkRow(dkHeader, "Epic", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Other", "Copy source other": "Epic Game Store",
	})
	g = parseDK(t, dkCSV(dkHeader, epic))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "epic-games-store" {
		t.Errorf("epic variant storefront = %v, want epic-games-store", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_UnmappedPlatformGoesToNotes(t *testing.T) {
	row := dkRow(dkHeader, "Weird", map[string]string{"Owned": "1", "Platforms": "Sega Saturn"})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none (unmapped -> note)", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Sega Saturn") {
		t.Errorf("unmapped platform not preserved in notes: %v", g.PersonalNotes)
	}
}

func TestDarkadia_NoPlatformGame(t *testing.T) {
	row := dkRow(dkHeader, "Bare", map[string]string{"Owned": "1"})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none", g.Platforms)
	}
}

func TestDarkadia_DedupOnPlatformStorefrontKeepsEarliest(t *testing.T) {
	named := dkRow(dkHeader, "Dup", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Steam", "Copy purchase date": "2013-01-01",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PC", "Copy media": "Digital", "Copy source": "Steam",
		"Copy purchase date": "2014-01-01",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 deduped", g.Platforms)
	}
	if g.Platforms[0].AcquiredDate != "2013-01-01" {
		t.Errorf("kept date = %q, want earliest 2013-01-01", g.Platforms[0].AcquiredDate)
	}
}

func TestDarkadia_RecognizedSourceWithTrailingFreeText(t *testing.T) {
	row := dkRow(dkHeader, "Uplay", map[string]string{
		"Owned": "1", "Platforms": "PC", "Copy platform": "PC", "Copy media": "Digital",
		"Copy source": "Uplay (coupon w/ GTX 970)",
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "uplay" {
		t.Errorf("storefront = %v, want uplay", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_NoCopyAggregateUsesInferredStorefront(t *testing.T) {
	row := dkRow(dkHeader, "NoCopyPSP", map[string]string{
		"Owned": "1", "Platforms": "PlayStation Network (PSP)", // no Copy platform
	})
	g := parseDK(t, dkCSV(dkHeader, row))[0]
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1", g.Platforms)
	}
	if g.Platforms[0].Platform != "playstation-psp" {
		t.Errorf("platform = %q, want playstation-psp", g.Platforms[0].Platform)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "playstation-store" {
		t.Errorf("storefront = %v, want playstation-store (inferred)", g.Platforms[0].Storefront)
	}
}

func TestDarkadia_DuplicateProvenanceNoteDeduped(t *testing.T) {
	named := dkRow(dkHeader, "DupNote", map[string]string{
		"Owned": "1", "Platforms": "PlayStation 4", "Copy platform": "PlayStation 4",
		"Copy media": "Physical", "Copy source": "GameStop",
	})
	cont := dkRow(dkHeader, "", map[string]string{
		"Copy platform": "PlayStation 4", "Copy media": "Physical", "Copy source": "GameStop",
	})
	g := parseDK(t, dkCSV(dkHeader, named, cont))[0]
	if g.PersonalNotes == nil {
		t.Fatalf("expected a provenance note")
	}
	if strings.Count(*g.PersonalNotes, "GameStop") != 1 {
		t.Errorf("GameStop mentioned %d times, want 1 (deduped): %q",
			strings.Count(*g.PersonalNotes, "GameStop"), *g.PersonalNotes)
	}
}

func TestDarkadia_TagsPlaytimeReviewCopyNotes(t *testing.T) {
	row := dkRow(dkExtHeader, "G", map[string]string{
		"Owned": "1", "Tags": "Co-op, VR", "Time played": "10:30",
		"Review subject": "Loved it", "Review": "Best game ever", "Notes": "my note",
		"Copy platform": "PC", "Copy notes": "PS Plus",
	})
	g := parseDK(t, dkCSV(dkExtHeader, row))[0]
	if len(g.Tags) != 2 || g.Tags[0] != "Co-op" || g.Tags[1] != "VR" {
		t.Fatalf("tags = %v, want [Co-op VR]", g.Tags)
	}
	if g.HoursPlayed == nil || *g.HoursPlayed != 10.5 {
		t.Errorf("hours = %v, want 10.5", g.HoursPlayed)
	}
	if g.PersonalNotes == nil {
		t.Fatal("notes nil")
	}
	for _, want := range []string{"my note", "Loved it", "Best game ever", "PS Plus"} {
		if !strings.Contains(*g.PersonalNotes, want) {
			t.Errorf("notes missing %q in: %s", want, *g.PersonalNotes)
		}
	}
}
```

> `TestDarkadia_TagsPlaytimeReviewCopyNotes` uses `Copy platform: PC` with no `Platforms` aggregate. In darkadia, a copy-row platform is marked owned, so PC becomes an owned platform with a copy — confirming the copy note ("PS Plus") and the PC platform both surface. This mirrors the original `TestConsolidate_TagsPlaytimeReviewCopyNotes`.

- [ ] **Step 2: Run the relocated suite**

Run: `go test ./internal/services/csvmap/ -run TestDarkadia -v`
Expected: PASS for every `TestDarkadia_*`. If any fails, the engine port diverges from darkadia — fix the engine (Task 1–3), not the test, since these encode the byte-for-byte contract.

- [ ] **Step 3: Run the whole csvmap package**

Run: `go test ./internal/services/csvmap/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/services/csvmap/darkadia_test.go
git commit -m "test: relocate Darkadia behaviour suite onto csvmap"
```

---

### Task 6: Delete the bespoke darkadia package, registry entry, and Go constant

Now that Darkadia runs through `csvmap`, remove the old mapper, its registry entry (which drops the import card + `/api/import/darkadia` route), the `JobSourceDarkadia` Go constant, and the duplicate pipeline test. No in-flight `darkadia`-sourced job compatibility is kept.

**Files:**
- Delete: `internal/services/darkadia/darkadia.go`, `internal/services/darkadia/darkadia_test.go`
- Modify: `internal/services/importsource/registry.go`
- Modify: `internal/services/importsource/registry_test.go`
- Modify: `internal/db/models/jobs.go`
- Modify: `internal/worker/tasks/import_pipeline_test.go`

- [ ] **Step 1: Delete the bespoke package**

```bash
git rm internal/services/darkadia/darkadia.go internal/services/darkadia/darkadia_test.go
```

- [ ] **Step 2: Remove the registry entry and its import**

In `internal/services/importsource/registry.go`, delete the `darkadia` import line:

```go
	"github.com/drzero42/nexorious/internal/services/darkadia"
```

and delete the entire Darkadia `Source` literal from `registry` (the block from `{ Slug: models.JobSourceDarkadia,` through its closing `},` — lines 38–49), leaving vglist as the first entry.

- [ ] **Step 3: Update the registry tests**

In `internal/services/importsource/registry_test.go`:

Delete `TestLookup_Darkadia` and `TestDarkadiaMapper_RejectsWrongFile` outright (their vglist twins `TestLookup_Vglist` / `TestVglistMapper_RejectsWrongFile` already cover the contract).

Re-point `TestIsRegistered` to vglist:

```go
func TestIsRegistered(t *testing.T) {
	if !importsource.IsRegistered(models.JobSourceVglist) {
		t.Error("vglist should be registered")
	}
	if importsource.IsRegistered(models.JobSourceNexorious) {
		t.Error("nexorious is not a mapper-based migration source")
	}
}
```

Rename `TestAll_IncludesDarkadia` to `TestAll_IncludesVglist`:

```go
func TestAll_IncludesVglist(t *testing.T) {
	found := false
	for _, s := range importsource.All() {
		if s.Slug == models.JobSourceVglist {
			found = true
		}
	}
	if !found {
		t.Error("All() omits vglist")
	}
}
```

- [ ] **Step 4: Delete the `JobSourceDarkadia` constant**

In `internal/db/models/jobs.go`, delete the line:

```go
	JobSourceDarkadia         = "darkadia"
```

- [ ] **Step 5: Delete the now-duplicate pipeline test**

In `internal/worker/tasks/import_pipeline_test.go`, delete the whole `TestFinalizeArgsForSource` function (lines ~317–331). Its csv-positive + nexorious/steam-negative coverage already lives in `enqueue_test.go` (`TestFinalizeArgsForSource_CSV`, `TestUsesGenericImportPipeline`).

> Do NOT touch `enqueue.go:UsesGenericImportPipeline` — it already returns true for `JobSourceCSV` (new Darkadia imports) and we are intentionally not preserving `darkadia` there.

- [ ] **Step 6: Build and test the affected packages**

Run:
```bash
go build ./...
go test ./internal/services/importsource/... ./internal/worker/tasks/... -v
```
Expected: build clean; tests PASS. A build error naming `JobSourceDarkadia` or `darkadia` means a reference was missed — grep `grep -rn "JobSourceDarkadia\|services/darkadia" --include=*.go` and remove it.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: remove bespoke darkadia mapper, registry entry, and JobSourceDarkadia"
```

---

### Task 7: Frontend — remove the Darkadia enum/label and update tests

The import card and route are already gone (data-driven from the registry). Remove the dead `JobSource.DARKADIA` enum member + label, and re-point/trim the tests per the test evaluation.

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts`
- Modify: `ui/frontend/src/components/navigation/nav-items.test.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/import-export.test.tsx`
- Modify: `ui/frontend/src/api/jobs.test.ts`
- Modify: `ui/frontend/src/hooks/use-jobs.test.ts`
- Modify: `ui/frontend/src/components/jobs/job-card.test.tsx`

Work from `ui/frontend/`.

- [ ] **Step 1: Remove the enum member and label entry**

In `ui/frontend/src/types/jobs.ts`, delete the enum line:

```ts
  DARKADIA = 'darkadia',
```

and delete its label-map entry (around line 235):

```ts
  [JobSource.DARKADIA]: 'Darkadia',
```

- [ ] **Step 2: Re-point the nav-items badge tests to vglist**

In `ui/frontend/src/components/navigation/nav-items.test.tsx`, in the first test change the source key and the import-source mock from darkadia to vglist, and update the title/comments:

```ts
  it('Sync badge excludes import-source reviews; Import badge shows the import count', () => {
    mockReview.mockReturnValue({
      data: { pendingReviewCount: 30, countsBySource: { steam: 4, psn: 1, vglist: 25 } },
    });
    mockImportSources.mockReturnValue({
      data: [
        {
          slug: 'vglist',
          display_name: 'vglist',
          description: '',
          features: [],
          accept: ['.json'],
        },
      ],
    });
    const { result } = renderHook(() => useNavItems());
    const sync = result.current.mainItems.find((i) => i.href === '/sync');
    const imp = result.current.mainItems.find((i) => i.href === '/import-export');
    expect(sync?.badge).toBe(5); // 30 total - 25 vglist
    expect(imp?.badge).toBe(25);
  });
```

The second test (`no Import badge when there are no … reviews`) does not reference darkadia and needs no change beyond an optional wording tweak.

- [ ] **Step 3: Re-point the import-export review-surface test to vglist**

In `ui/frontend/src/routes/_authenticated/import-export.test.tsx`:

Change the `useImportSources` mock to return vglist:

```ts
  useImportSources: () => ({
    data: [
      {
        slug: 'vglist',
        display_name: 'vglist',
        description: 'desc',
        features: [],
        accept: ['.json'],
      },
    ],
  }),
```

Change the registry-source review-surface test to use `JobSource.VGLIST`:

```ts
  it('renders the per-item review surface for an active vglist import', () => {
    h.job = makeJob(JobSource.VGLIST);
    render(<ImportExportPage />);
    expect(screen.getByTestId('import-review')).toBeInTheDocument();
  });
```

(The CSV and Nexorious sibling tests are unchanged.)

- [ ] **Step 4: Re-point the source-filter fixtures**

In `ui/frontend/src/api/jobs.test.ts`, change `source: JobSource.DARKADIA` to `source: JobSource.VGLIST` and the expected param `source: 'darkadia'` to `source: 'vglist'`.

In `ui/frontend/src/hooks/use-jobs.test.ts`, change `source: JobSource.DARKADIA` to `source: JobSource.VGLIST` and the expected `capturedParams!.get('source')` assertion from `'darkadia'` to `'vglist'`.

- [ ] **Step 5: Trim the job-card test**

In `ui/frontend/src/components/jobs/job-card.test.tsx`:

Delete the `darkadia: 'Darkadia',` line from the mock label map (around line 19), and delete the parameterized case:

```ts
      [{ source: JobSource.DARKADIA }, 'Sync - Darkadia'],
```

(The Steam/Epic/GOG cases keep the `"Sync - {label}"` path covered.)

- [ ] **Step 6: Typecheck, knip, and run the affected tests**

Run (from `ui/frontend/`):
```bash
npm run check
npm run knip
npm run test nav-items.test.tsx import-export.test.tsx jobs.test.ts use-jobs.test.ts job-card.test.tsx
```
Expected: `check` clean, `knip` clean (no leftover `DARKADIA` reference), tests PASS. A knip "unused export" or a tsc error naming `DARKADIA` means a reference was missed — grep `grep -rn "DARKADIA\|darkadia" ui/frontend/src` and update it.

- [ ] **Step 7: Regenerate the route tree and commit**

No routes changed, but run the build to be certain `routeTree.gen.ts` is current:

```bash
npm run build
cd ..
git add ui/frontend/src ui/frontend/src/routeTree.gen.ts
git commit -m "refactor: remove Darkadia job-source enum/label and update tests"
```

---

### Task 8: Update the Darkadia import doc

**Files:**
- Modify: `docs/darkadia-import.md`

- [ ] **Step 1: Note the new implementation and entry point**

`docs/darkadia-import.md` is intentionally implementation-agnostic, so the change is light. Add a short note near the top of **Part 2 — How it maps into Nexorious** (after the existing first paragraph that begins "The import runs on Nexorious's existing …"):

```markdown
> **Implementation note (#1016):** As of this refactor the Darkadia mapping is no
> longer a bespoke parser — it is expressed as a `csvmap.Config` value
> (`internal/services/csvmap/darkadia.go`) on the shared CSV engine, with **zero
> behaviour change**. The import is reached through the generic **CSV** card on
> the Import / Export page by selecting **Darkadia** in the Format dropdown (there
> is no longer a dedicated Darkadia card), and the resulting job is recorded with
> source `csv`. Everything this document describes about the format and the
> resulting Nexorious data is unchanged.
```

- [ ] **Step 2: Commit**

```bash
git add docs/darkadia-import.md
git commit -m "docs: note Darkadia mapping is now a csvmap.Config"
```

---

### Task 9: Full verification and PR

**Files:** none (verification only).

- [ ] **Step 1: Confirm no dangling references**

Run:
```bash
grep -rn "services/darkadia\|JobSourceDarkadia" --include='*.go' .
grep -rni "darkadia" ui/frontend/src | grep -v node_modules
```
Expected: the Go grep returns nothing. The frontend grep should return **no** `DARKADIA` enum/label references; any remaining hits should only be intentional (e.g. a comment) — there should be none after Task 7.

- [ ] **Step 2: Full backend suite**

Run: `go test -timeout 600s ./...`
Expected: PASS.

- [ ] **Step 3: Full frontend gate**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all green.

- [ ] **Step 4: Build everything**

Run: `make`
Expected: frontend builds, Go binary compiles.

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin refactor/1016-absorb-darkadia-csvmap
gh pr create --title "refactor: absorb Darkadia into the csvmap engine and delete the bespoke mapper" --body "$(cat <<'EOF'
Re-expresses Darkadia as a `csvmap.Config` value on the shared CSV engine (zero
behaviour change), implementing the advanced engine features whose `Config`
fields were declared in #1014 (`Status.Flags`, `Platform.Tables`, `Notes.Assembly`,
`Grouping.CopyRows`, `Duration.Format="h:mm"`).

- Darkadia now runs through `csvmap` + a preset `Config`; the bespoke
  `internal/services/darkadia` package is deleted.
- The Darkadia behaviour suite is relocated onto `csvmap` as CSV-driven tests
  (the byte-for-byte contract) and passes unchanged in spirit.
- The dedicated Darkadia import card and `/api/import/darkadia` route are removed
  (Darkadia is now selected in the CSV card's Format dropdown; jobs record as
  `csv`). The `JobSourceDarkadia` Go constant and the frontend `JobSource.DARKADIA`
  enum/label are removed; no in-flight `darkadia`-sourced job compatibility is kept.
- `docs/darkadia-import.md` notes the mapping is now a `csvmap.Config`.

Closes #1016
EOF
)"
```

Expected: PR created against `main`. Do not merge — wait for explicit instruction.

---

## Self-Review

**Spec coverage (issue #1016 acceptance criteria):**
- [x] *csvmap implements the advanced features, unit-tested* — Tasks 1 (`Status.Flags`), 2 (`h:mm`), 3 (`Platform.Tables`/`Notes.Assembly`/`Grouping.CopyRows`); unit tests for status-flags and h:mm; consolidation verified end-to-end by Task 5.
- [x] *Darkadia runs entirely through csvmap + a Config; bespoke logic removed* — Tasks 4 (Config) + 6 (deletion).
- [x] *Existing Darkadia + roundtrip tests pass* — Task 5 relocates the Darkadia suite (driven through `csvmap.Parse`); the JSON roundtrip test (`import_roundtrip_test.go`) is untouched and unaffected.
- [x] *docs/darkadia-import.md updated* — Task 8.
- [x] *Remove the separate Darkadia import card* (user instruction) — Task 6 Step 2 (registry entry deletion cascades to card + route) and Task 7 (enum/label cleanup).

**Placeholder scan:** No TBD/TODO; every code step shows full code; every command has an expected result.

**Type consistency:** `extractStatusFlags(rec, idx, *StatusFlags)`, `parseHMM(string) *float64`, `buildGrouped`/`consolidateGroup`, `effectiveSource(*PlatformTables, []string, map)`, `recognizedStorefront(*PlatformTables, string)`, `resolveStorefront(*PlatformTables, *string, string, string)` are used consistently across Tasks 1–5. `Darkadia()` returns `Config`; `PlatformMapping{Slug, InferredStorefront}` and `FlagRule{Column, Truthy, Status}` match the `config.go` definitions. Preset slug `"darkadia"` / display `"Darkadia"` consistent across Tasks 4, 7.

**Note on "tests pass unchanged":** the issue says the Darkadia tests must pass *unchanged*. They cannot literally — they call deleted internals (`consolidate`, column-index constants). Task 5 relocates them as behaviour-equivalent CSV-driven tests (the issue's "relocate, don't weaken"). This was confirmed with the user. The genuinely-unchanged test is the JSON `import_roundtrip_test.go`.
