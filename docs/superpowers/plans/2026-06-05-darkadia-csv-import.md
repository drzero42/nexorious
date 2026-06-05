# Darkadia CSV Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a one-off Darkadia CSV import that runs on the existing `jobs`/`job_items` framework — parse + consolidate in the upload handler, match to IGDB asynchronously (auto-resolve or `pending_review`), and finalize into `user_games`/`user_game_platforms` with additive merge.

**Architecture:** Upload handler (IGDB + header guards, synchronous parse via a pure `internal/services/darkadia` package) → `darkadia_match` River worker (reuses an extracted `matching.Decide` primitive) → `darkadia_finalize` River worker (additive DB writes). Manual review uses net-new import-scoped `resolve`/`skip` endpoints on `job_items`, surfaced through the existing `JobItemsDetails` + `IGDBMatchDialog` UI.

**Tech Stack:** Go 1.26, Bun ORM, River queue, Echo v5, `encoding/csv`; React 19 + TanStack Query/Router.

**Spec:** [`docs/superpowers/specs/2026-06-05-issue-822-darkadia-csv-import-design.md`](../specs/2026-06-05-issue-822-darkadia-csv-import-design.md). Format reference: [`docs/darkadia-import.md`](../../darkadia-import.md).

**Conventions for every task:** work on branch `feat/darkadia-csv-import`. Go tests run with `go test ./<pkg>/... -run <Name> -v`. Frontend from `ui/frontend/`. Commit after each task with a Conventional Commit message. The PostToolUse hook runs gofmt+lint on save; don't fight it.

---

## File map

- **Create** `internal/services/darkadia/darkadia.go` — pure parse + consolidation (the heart).
- **Create** `internal/services/darkadia/darkadia_test.go` — unit tests (the bulk of testing).
- **Modify** `internal/services/matching/` — add `decide.go` (extracted auto-resolve decision) + test.
- **Modify** `internal/worker/tasks/sync.go` — refactor `IGDBMatchWorker` to call `matching.Decide`.
- **Create** `internal/worker/tasks/darkadia.go` — `DarkadiaMatchWorker`, `DarkadiaFinalizeWorker`, `darkadiaCheckJobCompletion`.
- **Create** `internal/worker/tasks/darkadia_test.go` — worker tests.
- **Modify** `internal/worker/tasks/enqueue.go` — `ArgsForJobType` becomes source-aware (Darkadia retries route to `darkadia_match`).
- **Modify** `internal/db/models/jobs.go` — add `JobSourceDarkadia = "darkadia"`.
- **Modify** `internal/api/import.go` — `HandleImportDarkadia`; `ImportHandler` gains an IGDB client.
- **Modify** `internal/api/job_items.go` — `HandleResolveItem`, `HandleSkipItem` (import-scoped).
- **Modify** `internal/api/jobs.go` + `internal/api/job_items.go` — pass `job.Source` into `retryInsert`.
- **Modify** `internal/api/router.go` — wire the new routes + pass `igdbClient` to `NewImportHandler`.
- **Modify** `cmd/nexorious/serve.go` — register the two new workers in **both** worker blocks.
- **Modify** `ui/frontend/src/types/import-export.ts`, `ui/frontend/src/api/import-export.ts`, `ui/frontend/src/hooks/use-jobs.ts`, `ui/frontend/src/components/jobs/job-items-details.tsx`, and the import/export route — frontend surface.

---

## Task 1: `darkadia` package — types, header validation, row grouping

**Files:**
- Create: `internal/services/darkadia/darkadia.go`
- Test: `internal/services/darkadia/darkadia_test.go`

- [ ] **Step 1: Write failing tests for header + grouping**

```go
package darkadia

import (
	"strings"
	"testing"
)

// canonicalHeaderLine is the real 29-column header (quoting is incidental).
const canonicalHeaderLine = `Name,Added,Loved,Owned,Played,Playing,Finished,Mastered,Dominated,Shelved,Rating,"Copy label","Copy Release","Copy platform","Copy media","Copy media other","Copy source","Copy source other","Copy purchase date","Copy box","Copy box condition","Copy box notes","Copy manual","Copy manual condition","Copy manual notes","Copy complete","Copy complete notes",Platforms,Notes`

func TestParse_RejectsNonDarkadiaHeader(t *testing.T) {
	_, err := Parse([]byte("foo,bar,baz\n1,2,3\n"))
	if err == nil || !strings.Contains(err.Error(), "Darkadia") {
		t.Fatalf("want Darkadia header error, got %v", err)
	}
}

func TestParse_GroupsRowsIntoGamesAndCopies(t *testing.T) {
	// Two games: first has 2 copies (named row + 1 continuation), second has 1.
	csv := canonicalHeaderLine + "\n" +
		`Game A,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","note A"` + "\n" +
		`,,,,,,,,,,,"","","Mac","Digital","","GOG","","2014-01-01","","","","","","","","","",""` + "\n" +
		`Game B,2015-02-02,1,1,1,0,0,0,0,0,4.5,"","","PlayStation 4","Digital","","Sony Entertainment Network","","2015-02-02","","","","","","","","","PlayStation 4","note B"` + "\n"
	games, err := Parse([]byte(csv))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[0].Title != "Game A" || games[1].Title != "Game B" {
		t.Fatalf("titles = %q, %q", games[0].Title, games[1].Title)
	}
}

func TestParse_ToleratesRaggedRowsAndEmbeddedNewline(t *testing.T) {
	// Row with fewer than 29 fields (trailing columns omitted) + a Notes value
	// carrying an embedded newline inside quotes.
	csv := canonicalHeaderLine + "\n" +
		`Ragged,2013-06-05,0,1,0,0,0,0,0,0` + "\n" + // only 10 fields
		`Multi,2013-06-05,0,1,0,0,0,0,0,0,,"","","PC","Digital","","Steam","","2013-06-05","","","","","","","","","PC","line one` + "\n" + `line two"` + "\n"
	games, err := Parse([]byte(csv))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	if games[1].PersonalNotes == nil || !strings.Contains(*games[1].PersonalNotes, "line one\nline two") {
		t.Fatalf("embedded newline not preserved: %+v", games[1].PersonalNotes)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile (undefined `Parse`)**

Run: `go test ./internal/services/darkadia/... -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Implement types, header, reader, and grouping**

```go
package darkadia

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

// ErrInvalidHeader signals the file is not a Darkadia export. The upload handler
// turns this into a 400 "not a Darkadia export".
var ErrInvalidHeader = errors.New("not a Darkadia export (header mismatch)")

// header is the canonical 29-column Darkadia header, by value (quoting is
// incidental in the real export — only space-containing names are quoted).
var header = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
}

// Column indices (only the ones the importer reads).
const (
	colName            = 0
	colAdded           = 1
	colLoved           = 2
	colOwned           = 3
	colPlayed          = 4
	colPlaying         = 5
	colFinished        = 6
	colMastered        = 7
	colDominated       = 8
	colShelved         = 9
	colRating          = 10
	colCopyPlatform    = 13
	colCopyMedia       = 14
	colCopySource      = 16
	colCopySourceOther = 17
	colCopyPurchase    = 18
	colPlatforms       = 27
	colNotes           = 28
)

// Game is the consolidated, Nexorious-shaped payload for one Darkadia game. It
// is marshalled verbatim into job_item.source_metadata.
type Game struct {
	Title          string     `json:"title"`
	PlayStatus     string     `json:"play_status"`
	IsLoved        bool       `json:"is_loved"`
	PersonalRating *int32     `json:"personal_rating,omitempty"`
	PersonalNotes  *string    `json:"personal_notes,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
	Platforms      []Platform `json:"platforms"`
}

// Platform is one consolidated (platform, storefront, acquired_date) ownership entry.
type Platform struct {
	Platform     string  `json:"platform"`                 // Nexorious slug
	Storefront   *string `json:"storefront,omitempty"`     // slug or nil
	AcquiredDate string  `json:"acquired_date,omitempty"`  // "2006-01-02" or ""
}

// rawGame is one game grouped from the CSV: the named row plus its copy rows.
type rawGame struct {
	named  []string
	copies [][]string // every row (named + continuations), each padded to 29 fields
}

// Parse reads a Darkadia CSV and returns one consolidated Game per title.
func Parse(raw []byte) ([]Game, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	first, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if !headerMatches(first) {
		return nil, ErrInvalidHeader
	}

	var raws []rawGame
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		row := pad(rec, len(header))
		if row[colName] != "" {
			raws = append(raws, rawGame{named: row, copies: [][]string{row}})
			continue
		}
		if len(raws) == 0 {
			// Continuation row before any named row — malformed; skip defensively.
			continue
		}
		last := &raws[len(raws)-1]
		last.copies = append(last.copies, row)
	}

	games := make([]Game, 0, len(raws))
	for _, rg := range raws {
		games = append(games, consolidate(rg))
	}
	return games, nil
}

func headerMatches(got []string) bool {
	if len(got) != len(header) {
		return false
	}
	for i := range header {
		if got[i] != header[i] {
			return false
		}
	}
	return true
}

// pad returns row extended to n fields with empty strings (ragged-row tolerance).
func pad(row []string, n int) []string {
	if len(row) >= n {
		return row
	}
	out := make([]string, n)
	copy(out, row)
	return out
}

// consolidate is filled in across Tasks 2–3; for now it just carries the title.
func consolidate(rg rawGame) Game {
	return Game{Title: rg.named[colName]}
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/services/darkadia/... -v`
Expected: PASS (note: `TestParse_ToleratesRaggedRowsAndEmbeddedNewline` only checks count + the embedded newline; notes are wired in Task 2).

> If the embedded-newline assertion fails because `consolidate` doesn't yet set notes, temporarily relax that assertion to a `len(games)==2` check and restore it in Task 2. Prefer wiring notes now if quick.

- [ ] **Step 5: Commit**

```bash
git add internal/services/darkadia/
git commit -m "feat(import): darkadia CSV parser scaffolding — header validation + row grouping"
```

---

## Task 2: Game-level field mapping (status, rating, loved, notes, created_at)

**Files:**
- Modify: `internal/services/darkadia/darkadia.go`
- Test: `internal/services/darkadia/darkadia_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestConsolidate_PlayStatusPrecedence(t *testing.T) {
	cases := []struct {
		flags map[int]string // column index → "1"
		want  string
	}{
		{map[int]string{colOwned: "1"}, "not_started"},
		{map[int]string{colOwned: "1", colPlayed: "1"}, "shelved"},
		{map[int]string{colOwned: "1", colPlayed: "1", colPlaying: "1"}, "in_progress"},
		{map[int]string{colOwned: "1", colShelved: "1"}, "dropped"},
		{map[int]string{colOwned: "1", colFinished: "1"}, "completed"},
		{map[int]string{colMastered: "1", colFinished: "1"}, "mastered"},
		{map[int]string{colDominated: "1", colMastered: "1"}, "dominated"},
		// Shelved outranks Playing/Played but not Finished:
		{map[int]string{colShelved: "1", colPlaying: "1"}, "dropped"},
		{map[int]string{colFinished: "1", colShelved: "1"}, "completed"},
	}
	for i, c := range cases {
		row := make([]string, len(header))
		row[colName] = "G"
		for idx, v := range c.flags {
			row[idx] = v
		}
		got := consolidate(rawGame{named: row, copies: [][]string{row}})
		if got.PlayStatus != c.want {
			t.Errorf("case %d: play_status = %q, want %q", i, got.PlayStatus, c.want)
		}
	}
}

func TestConsolidate_RatingTruncatedAndLovedAndCreatedAt(t *testing.T) {
	row := make([]string, len(header))
	row[colName] = "G"
	row[colOwned] = "1"
	row[colLoved] = "1"
	row[colRating] = "4.5"
	row[colAdded] = "2013-06-05"
	row[colNotes] = "my note"
	g := consolidate(rawGame{named: row, copies: [][]string{row}})
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

func TestConsolidate_EmptyRatingIsUnrated(t *testing.T) {
	row := make([]string, len(header))
	row[colName] = "G"
	row[colOwned] = "1"
	row[colRating] = "" // and a "0" case
	g := consolidate(rawGame{named: row, copies: [][]string{row}})
	if g.PersonalRating != nil {
		t.Errorf("rating = %v, want nil", g.PersonalRating)
	}
	row[colRating] = "0"
	g = consolidate(rawGame{named: row, copies: [][]string{row}})
	if g.PersonalRating != nil {
		t.Errorf("rating 0 → %v, want nil", g.PersonalRating)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/services/darkadia/... -run TestConsolidate -v`
Expected: FAIL (play_status empty, rating nil, etc.).

- [ ] **Step 3: Implement game-level mapping in `consolidate`**

Replace the placeholder `consolidate` and add helpers:

```go
import "strconv" // add to the import block

func consolidate(rg rawGame) Game {
	n := rg.named
	g := Game{
		Title:      n[colName],
		PlayStatus: resolvePlayStatus(n),
		IsLoved:    n[colLoved] == "1",
		CreatedAt:  n[colAdded],
	}
	if r := parseRating(n[colRating]); r != nil {
		g.PersonalRating = r
	}

	// Notes: verbatim Darkadia Notes; provenance lines appended in Task 3.
	notes := n[colNotes]
	if notes != "" {
		g.PersonalNotes = &notes
	}
	return g
}

// resolvePlayStatus maps Darkadia's cumulative flags to a single Nexorious
// play_status by highest precedence (see docs/darkadia-import.md Part 2).
func resolvePlayStatus(row []string) string {
	switch {
	case row[colDominated] == "1":
		return "dominated"
	case row[colMastered] == "1":
		return "mastered"
	case row[colFinished] == "1":
		return "completed"
	case row[colShelved] == "1":
		return "dropped"
	case row[colPlaying] == "1":
		return "in_progress"
	case row[colPlayed] == "1":
		return "shelved"
	default: // Owned only (or nothing)
		return "not_started"
	}
}

// parseRating truncates a 0–5 half-step rating to a whole 1–5 star. Empty or 0
// means unrated (nil).
func parseRating(s string) *int32 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	v := int32(f) // truncation: 4.5 → 4
	if v <= 0 {
		return nil
	}
	return &v
}
```

Now restore the embedded-newline assertion in `TestParse_ToleratesRaggedRowsAndEmbeddedNewline` if you relaxed it in Task 1.

- [ ] **Step 4: Run, verify pass**

Run: `go test ./internal/services/darkadia/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/darkadia/
git commit -m "feat(import): map darkadia game-level fields (status, rating, loved, notes, added date)"
```

---

## Task 3: Platform consolidation + storefront resolution + provenance

This is the largest unit. Platform string → slug, `Copy source`/media → storefront, platform union/decoration/dedup, and provenance-note assembly (including the unmapped-platform-string → note rule per Decision B).

**Files:**
- Modify: `internal/services/darkadia/darkadia.go`
- Test: `internal/services/darkadia/darkadia_test.go`

- [ ] **Step 1: Write failing tests for the worked examples + rules**

```go
import "encoding/json" // add to test imports if not present

func ptr(s string) *string { return &s }

func mkRow(name string, fields map[int]string) []string {
	row := make([]string, len(header))
	row[colName] = name
	row[colOwned] = "1"
	for i, v := range fields {
		row[i] = v
	}
	return row
}

func TestConsolidate_Anodyne_PCWithGOGCopy_MacNoCopy(t *testing.T) {
	named := mkRow("Anodyne", map[int]string{
		colPlatforms:    "PC, Mac",
		colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "GOG",
		colCopyPurchase: "2014-03-01",
	})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
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

func TestConsolidate_Aaru_PS3andPS4_viaPSN(t *testing.T) {
	named := mkRow("Aaru's Awakening", map[int]string{
		colPlatforms:    "PlayStation Network (PS3), PlayStation 4",
		colCopyPlatform: "PlayStation Network (PS3)", colCopyMedia: "Digital",
		colCopySource: "Sony Entertainment Network", colCopyPurchase: "2015-02-02",
	})
	cont := make([]string, len(header))
	cont[colCopyPlatform] = "PlayStation 4"
	cont[colCopyMedia] = "Digital"
	cont[colCopySource] = "Sony Entertainment Network"
	g := consolidate(rawGame{named: named, copies: [][]string{named, cont}})
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

func TestConsolidate_StorefrontRules(t *testing.T) {
	// Physical media routes to `physical` + retailer note, regardless of retailer.
	phys := mkRow("Phys", map[int]string{
		colPlatforms: "PlayStation 4", colCopyPlatform: "PlayStation 4",
		colCopyMedia: "Physical", colCopySource: "GameStop",
	})
	g := consolidate(rawGame{named: phys, copies: [][]string{phys}})
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "physical" {
		t.Errorf("physical storefront = %v", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "GameStop") {
		t.Errorf("physical retailer not in notes: %v", g.PersonalNotes)
	}

	// Unrecognized digital store → nil storefront + note.
	unrec := mkRow("Unrec", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
		colCopySource: "Fanatical",
	})
	g = consolidate(rawGame{named: unrec, copies: [][]string{unrec}})
	if g.Platforms[0].Storefront != nil {
		t.Errorf("unrecognized digital storefront = %v, want nil", g.Platforms[0].Storefront)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Fanatical") {
		t.Errorf("unrecognized source not in notes: %v", g.PersonalNotes)
	}

	// Empty source → nil storefront, NO note.
	empty := mkRow("Empty", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
	})
	g = consolidate(rawGame{named: empty, copies: [][]string{empty}})
	if g.Platforms[0].Storefront != nil || g.PersonalNotes != nil {
		t.Errorf("empty source: storefront=%v notes=%v, want nil/nil", g.Platforms[0].Storefront, g.PersonalNotes)
	}

	// Epic spelling variant via Copy source other (source == "Other").
	epic := mkRow("Epic", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital",
		colCopySource: "Other", colCopySourceOther: "Epic Game Store",
	})
	g = consolidate(rawGame{named: epic, copies: [][]string{epic}})
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "epic-games-store" {
		t.Errorf("epic variant storefront = %v, want epic-games-store", g.Platforms[0].Storefront)
	}
}

func TestConsolidate_UnmappedPlatform_GoesToNotesNotFailure(t *testing.T) {
	named := mkRow("Weird", map[int]string{
		colPlatforms: "Sega Saturn", // not in the mapping table
	})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none (unmapped → note)", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "Sega Saturn") {
		t.Errorf("unmapped platform not preserved in notes: %v", g.PersonalNotes)
	}
}

func TestConsolidate_NoPlatformGame(t *testing.T) {
	named := mkRow("Bare", nil) // owned, no Platforms, no copies
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	if len(g.Platforms) != 0 {
		t.Errorf("platforms = %+v, want none", g.Platforms)
	}
}

func TestConsolidate_DedupOnPlatformStorefront(t *testing.T) {
	// Two PC/Steam copies collapse to one (platform, storefront) entry.
	named := mkRow("Dup", map[int]string{
		colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "Steam",
		colCopyPurchase: "2013-01-01",
	})
	cont := make([]string, len(header))
	cont[colCopyPlatform] = "PC"
	cont[colCopyMedia] = "Digital"
	cont[colCopySource] = "Steam"
	cont[colCopyPurchase] = "2014-01-01"
	g := consolidate(rawGame{named: named, copies: [][]string{named, cont}})
	if len(g.Platforms) != 1 {
		t.Fatalf("platforms = %+v, want 1 deduped", g.Platforms)
	}
	if g.Platforms[0].AcquiredDate != "2013-01-01" {
		t.Errorf("kept date = %q, want earliest 2013-01-01", g.Platforms[0].AcquiredDate)
	}
}

// SourceMetadata round-trips as JSON (it is stored verbatim).
func TestGame_JSONRoundTrip(t *testing.T) {
	named := mkRow("J", map[int]string{colPlatforms: "PC", colCopyPlatform: "PC", colCopyMedia: "Digital", colCopySource: "Steam"})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var back Game
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Platforms[0].Platform != "pc-windows" {
		t.Errorf("round-trip platform = %q", back.Platforms[0].Platform)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/services/darkadia/... -run TestConsolidate -v`
Expected: FAIL (no platform logic yet).

- [ ] **Step 3: Implement mapping tables, storefront resolution, consolidation, provenance**

Add to `darkadia.go` (and add `"sort"`, `"strings"` to imports):

```go
// platformMapping is a Nexorious platform slug plus an optional inferred
// storefront (used only as a fallback when no copy supplies one).
type platformMapping struct {
	slug     string
	inferred *string // nil unless the platform string itself names a storefront
}

// platformTable maps every Darkadia platform string in the reference export to
// a Nexorious slug. A string absent from this table is preserved in the note
// (Decision B), never dropped and never a failure.
var platformTable = map[string]platformMapping{
	"PC":                          {slug: "pc-windows"},
	"Linux":                       {slug: "pc-linux"},
	"Mac":                         {slug: "mac"},
	"PlayStation 4":               {slug: "playstation-4"},
	"PlayStation 5":               {slug: "playstation-5"},
	"PlayStation 3":               {slug: "playstation-3"},
	"PlayStation Network (PS3)":   {slug: "playstation-3", inferred: ptrStr("playstation-store")},
	"PlayStation Network (Vita)":  {slug: "playstation-vita", inferred: ptrStr("playstation-store")},
	"Nintendo Switch":             {slug: "nintendo-switch"},
	"Wii":                         {slug: "nintendo-wii"},
	"Xbox 360":                    {slug: "xbox-360"},
	"Xbox 360 Games Store":        {slug: "xbox-360", inferred: ptrStr("microsoft-store")},
	"Android":                     {slug: "android"},
	"PlayStation 2":               {slug: "playstation-2"},
	"PlayStation Network (PSP)":   {slug: "playstation-psp", inferred: ptrStr("playstation-store")},
}

func ptrStr(s string) *string { return &s }

// storefrontTable maps a recognized digital source (lowercased) to a Nexorious
// storefront slug. Spelling variants from the reference export are included.
var storefrontTable = map[string]string{
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
	"google play":               "google-play-store",
	"uplay":                      "uplay",
	"ubisoft club":               "uplay",
}

// effectiveSource returns the source string for a copy row: Copy source, unless
// it is the literal "Other", in which case Copy source other.
func effectiveSource(row []string) string {
	src := strings.TrimSpace(row[colCopySource])
	if strings.EqualFold(src, "Other") {
		return strings.TrimSpace(row[colCopySourceOther])
	}
	return src
}

// recognizedStorefront returns the slug for a recognized digital source. It
// tolerates extra free text after a recognized name (e.g. "Uplay (coupon …)").
func recognizedStorefront(eff string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(eff))
	if slug, ok := storefrontTable[key]; ok {
		return slug, true
	}
	// Tolerate trailing annotation: match on the leading recognized token set.
	for name, slug := range storefrontTable {
		if strings.HasPrefix(key, name+" ") {
			return slug, true
		}
	}
	return "", false
}

// resolveStorefront applies the per-copy storefront precedence. It returns the
// storefront slug (or nil) and an optional provenance note line ("" = none).
func resolveStorefront(inferred *string, eff, media string) (*string, string) {
	if eff != "" {
		if slug, ok := recognizedStorefront(eff); ok {
			s := slug
			return &s, ""
		}
	}
	if media == "Physical" {
		s := "physical"
		note := ""
		if eff != "" {
			note = "Purchased physically from " + eff + "."
		}
		return &s, note
	}
	if eff != "" {
		// Unrecognized digital store — preserve in note, no storefront.
		return nil, "Purchased from " + eff + "."
	}
	if inferred != nil {
		s := *inferred
		return &s, ""
	}
	return nil, ""
}

// splitAggregate splits the comma-separated aggregate Platforms list.
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
```

Now replace `consolidate` to assemble platforms + provenance. Keep the game-level mapping from Task 2 and append the platform logic:

```go
func consolidate(rg rawGame) Game {
	n := rg.named
	g := Game{
		Title:      n[colName],
		PlayStatus: resolvePlayStatus(n),
		IsLoved:    n[colLoved] == "1",
		CreatedAt:  n[colAdded],
	}
	if r := parseRating(n[colRating]); r != nil {
		g.PersonalRating = r
	}

	var noteLines []string // provenance lines, appended after verbatim Notes
	addNote := func(line string) {
		if line == "" {
			return
		}
		for _, existing := range noteLines {
			if existing == line {
				return // dedupe identical provenance lines
			}
		}
		noteLines = append(noteLines, line)
	}

	// Build the owned-slug set as a union of aggregate marks and copy platforms.
	// ownedInferred tracks, per slug, any inferred storefront from its source string.
	owned := map[string]bool{}
	ownedInferred := map[string]*string{}
	mapString := func(s string) (mapping platformMapping, ok bool) {
		m, ok := platformTable[s]
		return m, ok
	}
	markOwned := func(s string) {
		m, ok := mapString(s)
		if !ok {
			addNote("Owned on " + s + " (no Nexorious platform mapping).")
			return
		}
		owned[m.slug] = true
		if m.inferred != nil && ownedInferred[m.slug] == nil {
			ownedInferred[m.slug] = m.inferred
		}
	}
	for _, s := range splitAggregate(n[colPlatforms]) {
		markOwned(s)
	}
	for _, row := range rg.copies {
		if p := strings.TrimSpace(row[colCopyPlatform]); p != "" {
			markOwned(p)
		}
	}

	// Decorate: emit one entry per copy that matches a slug; for slugs with no
	// matching copy, emit a single (slug, inferred|nil) entry.
	type key struct {
		platform   string
		storefront string // "" represents NULL for de-dup
	}
	seen := map[key]int{} // key → index into g.Platforms (for earliest-date keep)
	add := func(slug string, sf *string, date string) {
		sfKey := ""
		if sf != nil {
			sfKey = *sf
		}
		k := key{slug, sfKey}
		if idx, ok := seen[k]; ok {
			// Keep the earliest acquired_date.
			if date != "" && (g.Platforms[idx].AcquiredDate == "" || date < g.Platforms[idx].AcquiredDate) {
				g.Platforms[idx].AcquiredDate = date
			}
			return
		}
		seen[k] = len(g.Platforms)
		g.Platforms = append(g.Platforms, Platform{Platform: slug, Storefront: sf, AcquiredDate: date})
	}

	slugHasCopy := map[string]bool{}
	for _, row := range rg.copies {
		ps := strings.TrimSpace(row[colCopyPlatform])
		if ps == "" {
			continue
		}
		m, ok := mapString(ps)
		if !ok {
			continue // already noted via markOwned
		}
		slugHasCopy[m.slug] = true
		sf, note := resolveStorefront(m.inferred, effectiveSource(row), strings.TrimSpace(row[colCopyMedia]))
		addNote(note)
		add(m.slug, sf, strings.TrimSpace(row[colCopyPurchase]))
	}
	// Aggregate-only slugs (no matching copy): single entry, inferred or NULL.
	slugs := make([]string, 0, len(owned))
	for s := range owned {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs) // deterministic output order
	for _, slug := range slugs {
		if slugHasCopy[slug] {
			continue
		}
		add(slug, ownedInferred[slug], "")
	}

	// Assemble notes: verbatim Notes, then a blank line, then provenance lines.
	verbatim := n[colNotes]
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

Note: the duplicate `ptr`/`ptrStr` helpers — keep `ptrStr` in `darkadia.go` and `ptr` in the test file; do not redeclare.

- [ ] **Step 4: Run all darkadia tests, verify pass**

Run: `go test ./internal/services/darkadia/... -v`
Expected: PASS (all Task 1–3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/services/darkadia/
git commit -m "feat(import): consolidate darkadia platforms, storefronts, and provenance notes"
```

---

## Task 4: Extract `matching.Decide` and refactor sync to use it

**Files:**
- Create: `internal/services/matching/decide.go`
- Create: `internal/services/matching/decide_test.go`
- Modify: `internal/worker/tasks/sync.go:520-561`

- [ ] **Step 1: Write failing test for `Decide`**

```go
package matching

import (
	"testing"

	"github.com/drzero42/nexorious/internal/services/igdb"
)

func TestDecide_ConfidentUnambiguous(t *testing.T) {
	cands := []igdb.GameMetadata{
		{IgdbID: 1, Title: "Celeste"},
		{IgdbID: 2, Title: "Some Other Game Entirely"},
	}
	d := Decide("Celeste", cands)
	if !d.Confident {
		t.Fatalf("expected confident, got %+v", d)
	}
	if d.ResolvedIGDBID != 1 {
		t.Errorf("resolved id = %d, want 1", d.ResolvedIGDBID)
	}
}

func TestDecide_TieIsNotConfident(t *testing.T) {
	cands := []igdb.GameMetadata{
		{IgdbID: 1, Title: "Halo"},
		{IgdbID: 2, Title: "Halo"},
	}
	d := Decide("Halo", cands)
	if d.Confident {
		t.Errorf("tie should not be confident: %+v", d)
	}
}

func TestDecide_LowConfidenceNotConfident(t *testing.T) {
	cands := []igdb.GameMetadata{{IgdbID: 1, Title: "Completely Different Title"}}
	d := Decide("xyzzy", cands)
	if d.Confident {
		t.Errorf("low score should not be confident: %+v", d)
	}
}

func TestDecide_NoCandidates(t *testing.T) {
	d := Decide("anything", nil)
	if d.Confident {
		t.Errorf("no candidates should not be confident: %+v", d)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/services/matching/... -run TestDecide -v`
Expected: FAIL — `undefined: Decide`.

- [ ] **Step 3: Implement `Decide`**

```go
package matching

import "github.com/drzero42/nexorious/internal/services/igdb"

// AutoResolveThreshold and TieEpsilon are the auto-resolve gate, shared by the
// sync IGDB-match worker and the Darkadia import match worker.
const (
	AutoResolveThreshold = 0.85
	TieEpsilon           = 0.01
)

// Decision is the result of scoring IGDB candidates against a query title.
type Decision struct {
	BestScore      float64
	SecondBest     float64
	ResolvedIGDBID int32 // best candidate's IGDB id (0 if no candidates)
	// Confident is true when the best score clears the threshold AND beats the
	// runner-up by more than TieEpsilon — i.e. confident and unambiguous.
	Confident bool
}

// Decide scores candidates against query (titles normalized internally) and
// returns the auto-resolve decision. It does no I/O.
func Decide(query string, candidates []igdb.GameMetadata) Decision {
	nq := NormalizeTitle(query)
	var best, second float64
	var bestID int32
	for _, c := range candidates {
		score := FuzzyConfidence(nq, NormalizeTitle(c.Title))
		if score > best {
			second = best
			best = score
			bestID = int32(c.IgdbID) //nolint:gosec // IGDB ids are positive, fit int32
		} else if score > second {
			second = score
		}
	}
	return Decision{
		BestScore:      best,
		SecondBest:     second,
		ResolvedIGDBID: bestID,
		Confident:      best >= AutoResolveThreshold && (best-second) > TieEpsilon,
	}
}
```

- [ ] **Step 4: Run, verify pass**

Run: `go test ./internal/services/matching/... -v`
Expected: PASS.

- [ ] **Step 5: Refactor sync to call `Decide`**

In `internal/worker/tasks/sync.go`, replace the inline scoring + threshold block (the loop computing `bestScore`/`secondBestScore`/`bestID` and the `const autoResolveThreshold … if bestScore >= …` decision, lines ~520-561) with:

```go
		decision := matching.Decide(eg.Title, candidates)
		slog.Debug("igdb_match: search results",
			"item_id", p.JobItemID, "title", eg.Title,
			"candidate_count", len(candidates),
			"best_score", decision.BestScore,
			"second_best_score", decision.SecondBest,
			"best_igdb_id", decision.ResolvedIGDBID,
		)

		if decision.Confident {
			bestID := decision.ResolvedIGDBID
			slog.Debug("igdb_match: auto-resolved",
				"item_id", p.JobItemID, "title", eg.Title, "igdb_id", bestID, "score", decision.BestScore)
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: insert game row (auto-resolve)", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: auto-resolve external_game", "err", err, "external_game_id", eg.ID)
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}

		slog.Debug("igdb_match: low confidence, marking pending_review",
			"item_id", p.JobItemID, "title", eg.Title,
			"best_score", decision.BestScore, "threshold", matching.AutoResolveThreshold,
			"tie_gap", decision.BestScore-decision.SecondBest, "candidate_count", len(candidates))
		candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
		item.IGDBCandidates = candidatesJSON
		bs := decision.BestScore
		item.MatchConfidence = &bs
		syncMarkItemPendingReview(ctx, w.DB, &item)
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
```

Confirm `matching` is already imported in `sync.go` (it is — `matching.NormalizeTitle`/`FuzzyConfidence` were used). Remove the now-unused `matching.NormalizeTitle`/`FuzzyConfidence` calls only if nothing else uses them in the file (search first).

- [ ] **Step 6: Run sync tests + build, verify green**

Run: `go test ./internal/worker/tasks/... -run TestIGDBMatch -v && go build ./...`
Expected: PASS + clean build. If no `TestIGDBMatch*` exists, run the whole package: `go test ./internal/worker/tasks/...`.

- [ ] **Step 7: Commit**

```bash
git add internal/services/matching/ internal/worker/tasks/sync.go
git commit -m "refactor(match): extract shared matching.Decide auto-resolve primitive from sync"
```

---

## Task 5: `JobSourceDarkadia` constant + source-aware retry routing

**Files:**
- Modify: `internal/db/models/jobs.go:29`
- Modify: `internal/worker/tasks/enqueue.go:57-68`
- Modify: `internal/api/jobs.go:727-738`, `internal/api/jobs.go:713`
- Modify: `internal/api/job_items.go:90`

- [ ] **Step 1: Add the source constant**

In `internal/db/models/jobs.go`, in the `JobSource*` const block, add after `JobSourceNexorious`:

```go
	JobSourceDarkadia     = "darkadia"
```

- [ ] **Step 2: Make `ArgsForJobType` source-aware**

In `internal/worker/tasks/enqueue.go`, change the signature and add a Darkadia branch:

```go
func ArgsForJobType(jobType, source, jobItemID string) (river.JobArgs, error) {
	// Darkadia imports run a bespoke match→finalize chain; a retried item
	// re-enters at the match stage regardless of the (shared) "import" job_type.
	if source == models.JobSourceDarkadia {
		return DarkadiaMatchArgs{JobItemID: jobItemID}, nil
	}
	switch jobType {
	case models.JobTypeSync:
		return IGDBMatchArgs{JobItemID: jobItemID}, nil
	case models.JobTypeImport:
		return ImportItemArgs{JobItemID: jobItemID}, nil
	case models.JobTypeMetadataRefresh:
		return MetadataRefreshItemArgs{JobItemID: jobItemID}, nil
	default:
		return nil, fmt.Errorf("unknown job_type %q", jobType)
	}
}
```

> `DarkadiaMatchArgs` is defined in Task 6; this task will not build until Task 6 lands. Implement Tasks 5 and 6 together (single commit at the end of Task 6) — Step 5 below reflects that.

- [ ] **Step 3: Thread `source` through `retryInsert` and callers**

`internal/api/jobs.go`:

```go
func retryInsert(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobType, source, jobItemID string) {
	args, err := tasks.ArgsForJobType(jobType, source, jobItemID)
	if err != nil {
		slog.Error("retryInsert: unsupported job_type",
			"job_type", jobType, "source", source, "job_item_id", jobItemID, "err", err)
		return
	}
	if err := tasks.EnqueueOrFail(ctx, db, rc, jobItemID, args); err != nil {
		slog.Error("retryInsert: enqueue failed",
			"job_type", jobType, "source", source, "job_item_id", jobItemID, "err", err)
	}
}
```

Update the call site at `internal/api/jobs.go:713` to pass `job.Source`:

```go
		retryInsert(context.Background(), h.db, h.riverClient, job.JobType, job.Source, item.ID)
```

Update `internal/api/job_items.go:90`:

```go
	retryInsert(context.Background(), h.db, h.riverClient, job.JobType, job.Source, itemID)
```

- [ ] **Step 4: (Deferred) build** — completes in Task 6.

- [ ] **Step 5: Commit** — combined with Task 6.

---

## Task 6: Darkadia match + finalize workers + completion check

**Files:**
- Create: `internal/worker/tasks/darkadia.go`
- Test: `internal/worker/tasks/darkadia_test.go`

- [ ] **Step 1: Write failing worker tests**

Mirror the existing `internal/worker/tasks` test setup (shared `testDB`, `truncateAllTables(t)`). Reference an existing test file in the package for the exact helper names before writing.

```go
package tasks_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// helper: insert a darkadia job + one job_item with the given source_metadata.
func insertDarkadiaItem(t *testing.T, userID string, payload map[string]any) (jobID, itemID string) {
	t.Helper()
	ctx := context.Background()
	jobID = uuid.NewString()
	itemID = uuid.NewString()
	meta, _ := json.Marshal(payload)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'import', 'darkadia', 'processing', 'high', 1, true, now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, "game_0", payload["title"], meta,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID, itemID
}

func TestDarkadiaFinalize_WritesUserGameAndPlatforms(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertTestUser(t) // use the package's existing user helper
	// Seed the resolved game so finalize does not need live IGDB.
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	sf := "gog"
	payload := map[string]any{
		"title":       "Anodyne",
		"play_status": "completed",
		"is_loved":    true,
		"personal_notes": "great",
		"platforms": []map[string]any{
			{"platform": "pc-windows", "storefront": sf, "acquired_date": "2014-03-01"},
			{"platform": "mac"},
		},
	}
	jobID, itemID := insertDarkadiaItem(t, userID, payload)
	// Mark resolved (auto path sets this; here we set it directly).
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.DarkadiaFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.DarkadiaFinalizeArgs]{Args: tasks.DarkadiaFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 42", userID).Scan(ctx); err != nil {
		t.Fatalf("user_game not written: %v", err)
	}
	if ug.PlayStatus == nil || *ug.PlayStatus != "completed" || !ug.IsLoved {
		t.Errorf("user_game fields wrong: %+v", ug)
	}
	var count int
	testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ug.ID).Scan(ctx, &count)
	if count != 2 {
		t.Errorf("platforms = %d, want 2", count)
	}
	// Job should be completed and item completed.
	var jobStatus string
	testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &jobStatus)
	if jobStatus != "completed" {
		t.Errorf("job status = %q, want completed", jobStatus)
	}
}

func TestDarkadiaFinalize_AdditiveMergeDoesNotOverwrite(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertTestUser(t)
	testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (42, 'Anodyne', now(), now())`).Exec(ctx)
	// Pre-existing curated user_game: rating 5, status mastered, a note.
	ugID := uuid.NewString()
	testDB.NewRaw(`INSERT INTO user_games (id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, created_at, updated_at)
		VALUES (?, ?, 42, 'mastered', 5, true, 'mine', now(), now())`, ugID, userID).Exec(ctx)

	payload := map[string]any{
		"title": "Anodyne", "play_status": "not_started", "is_loved": false,
		"personal_rating": 2, "personal_notes": "imported",
		"platforms": []map[string]any{{"platform": "mac"}},
	}
	_, itemID := insertDarkadiaItem(t, userID, payload)
	testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 42 WHERE id = ?`, itemID).Exec(ctx)

	w := &tasks.DarkadiaFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	w.Work(ctx, &river.Job[tasks.DarkadiaFinalizeArgs]{Args: tasks.DarkadiaFinalizeArgs{JobItemID: itemID}})

	var ug models.UserGame
	testDB.NewSelect().Model(&ug).Where("id = ?", ugID).Scan(ctx)
	if *ug.PlayStatus != "mastered" || *ug.PersonalRating != 5 || *ug.PersonalNotes != "mine" || !ug.IsLoved {
		t.Errorf("curation overwritten: %+v", ug)
	}
	var count int
	testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(ctx, &count)
	if count != 1 { // mac added
		t.Errorf("platforms = %d, want 1 (mac merged in)", count)
	}
}
```

(Use the package's actual user-insertion helper; grep the existing `*_test.go` files in `internal/worker/tasks/` for the right name — e.g. `insertTestUser` or an inline INSERT.)

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/worker/tasks/... -run TestDarkadia -v`
Expected: FAIL — undefined `DarkadiaFinalizeWorker`/`DarkadiaFinalizeArgs`.

- [ ] **Step 3: Implement the workers**

```go
package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/darkadia"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/services/matching"
)

// ── Stage 1: match ───────────────────────────────────────────────────────────

type DarkadiaMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (DarkadiaMatchArgs) Kind() string { return "darkadia_match" }
func (DarkadiaMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 3}
}

type DarkadiaMatchWorker struct {
	river.WorkerDefaults[DarkadiaMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *DarkadiaMatchWorker) Work(ctx context.Context, job *river.Job[DarkadiaMatchArgs]) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.Error("darkadia_match: load job_item", "id", job.Args.JobItemID, "err", err)
		return nil
	}

	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
		// Should not happen (upload is guarded), but fail closed to pending_review.
		darkadiaMarkPendingReview(ctx, w.DB, &item, nil, nil)
		darkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	candidates, err := w.IGDBClient.SearchGames(ctx, item.SourceTitle, 10, nil)
	if err != nil {
		if job.Attempt >= job.MaxAttempts {
			slog.Warn("darkadia_match: IGDB failed on final attempt, pending_review", "item_id", item.ID, "err", err)
			darkadiaMarkPendingReview(ctx, w.DB, &item, nil, nil)
			darkadiaCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
		return fmt.Errorf("darkadia_match: search failed (will retry): %w", err)
	}

	decision := matching.Decide(item.SourceTitle, candidates)
	if decision.Confident {
		if _, err := w.DB.NewRaw(
			`UPDATE job_items SET resolved_igdb_id = ?, match_confidence = ? WHERE id = ?`,
			decision.ResolvedIGDBID, decision.BestScore, item.ID,
		).Exec(ctx); err != nil {
			slog.Error("darkadia_match: set resolved id", "err", err, "item_id", item.ID)
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, DarkadiaFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.Error("darkadia_match: enqueue finalize", "err", err, "item_id", item.ID)
			darkadiaCheckJobCompletion(w.DB, item.JobID)
		}
		return nil
	}

	candJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling candidates cannot fail
	bs := decision.BestScore
	darkadiaMarkPendingReview(ctx, w.DB, &item, candJSON, &bs)
	darkadiaCheckJobCompletion(w.DB, item.JobID)
	return nil
}

func darkadiaMarkPendingReview(ctx context.Context, db *bun.DB, item *models.JobItem, candidates json.RawMessage, confidence *float64) {
	item.Status = models.JobItemStatusPendingReview
	if candidates != nil {
		item.IGDBCandidates = candidates
	}
	item.MatchConfidence = confidence
	if _, err := db.NewUpdate().Model(item).
		Column("status", "igdb_candidates", "match_confidence").
		Where("id = ?", item.ID).Exec(ctx); err != nil {
		slog.Error("darkadia_match: mark pending_review", "id", item.ID, "err", err)
	}
}

// ── Stage 2: finalize ────────────────────────────────────────────────────────

type DarkadiaFinalizeArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (DarkadiaFinalizeArgs) Kind() string { return "darkadia_finalize" }
func (DarkadiaFinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type DarkadiaFinalizeWorker struct {
	river.WorkerDefaults[DarkadiaFinalizeArgs]
	DB          *bun.DB
	IGDBClient  *igdb.Client
	StoragePath string
}

func (w *DarkadiaFinalizeWorker) Work(ctx context.Context, job *river.Job[DarkadiaFinalizeArgs]) error {
	bg := context.Background() // writes survive shutdown cancellation
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", job.Args.JobItemID).Scan(ctx); err != nil {
		slog.Error("darkadia_finalize: load job_item", "id", job.Args.JobItemID, "err", err)
		return nil
	}
	if item.ResolvedIGDBID == nil {
		markItemFailed(bg, w.DB, &item, "no resolved IGDB id", "darkadia_finalize: markItemFailed")
		darkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}
	igdbID := int32(*item.ResolvedIGDBID) //nolint:gosec // resolved id fits int32

	var payload darkadia.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("parse payload: %v", err), "darkadia_finalize: markItemFailed")
		darkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	// Ensure games row (reuse the import worker's IGDB fetch + cover download).
	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, igdbID, payload.Title); err != nil {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("ensure game: %v", err), "darkadia_finalize: markItemFailed")
		darkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}

	// Upsert user_game (additive — never overwrite existing curation).
	var ug models.UserGame
	err := w.DB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", item.UserID, igdbID).Scan(ctx)
	alreadyExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		markItemFailed(bg, w.DB, &item, fmt.Sprintf("load user_game: %v", err), "darkadia_finalize: markItemFailed")
		darkadiaCheckJobCompletion(w.DB, item.JobID)
		return nil
	}
	now := time.Now().UTC()
	if !alreadyExists {
		created := now
		if payload.CreatedAt != "" {
			if t, perr := time.Parse("2006-01-02", payload.CreatedAt); perr == nil {
				created = t.UTC()
			}
		}
		ps := payload.PlayStatus
		ug = models.UserGame{
			ID: uuid.NewString(), UserID: item.UserID, GameID: igdbID,
			PlayStatus: &ps, PersonalRating: payload.PersonalRating, IsLoved: payload.IsLoved,
			PersonalNotes: payload.PersonalNotes, CreatedAt: created, UpdatedAt: now,
		}
		if _, ierr := w.DB.NewInsert().Model(&ug).Exec(ctx); ierr != nil {
			markItemFailed(bg, w.DB, &item, fmt.Sprintf("insert user_game: %v", ierr), "darkadia_finalize: markItemFailed")
			darkadiaCheckJobCompletion(w.DB, item.JobID)
			return nil
		}
	}

	// Merge platforms (skip existing (platform, storefront); ownership owned).
	existing := map[[2]string]bool{}
	if alreadyExists {
		var ugps []models.UserGamePlatform
		w.DB.NewSelect().Model(&ugps).Where("user_game_id = ?", ug.ID).Scan(ctx)
		for _, p := range ugps {
			existing[[2]string{deref(p.Platform), deref(p.Storefront)}] = true
		}
	}
	owned := "owned"
	newPlatforms := 0
	for _, pl := range payload.Platforms {
		var sf *string
		if pl.Storefront != nil {
			sf = pl.Storefront
		}
		if existing[[2]string{pl.Platform, deref(sf)}] {
			continue
		}
		platform := pl.Platform
		ugp := models.UserGamePlatform{
			ID: uuid.NewString(), UserGameID: ug.ID, Platform: &platform, Storefront: sf,
			OwnershipStatus: &owned, AcquiredDate: parseDateOnly(pl.AcquiredDate),
			CreatedAt: now, UpdatedAt: now,
		}
		if _, ierr := w.DB.NewInsert().Model(&ugp).Exec(ctx); ierr != nil {
			slog.Error("darkadia_finalize: insert platform", "err", ierr)
		} else {
			newPlatforms++
		}
	}

	changeType := "added"
	if alreadyExists {
		if newPlatforms > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
	}
	if _, err := w.DB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
		 VALUES (?, ?, ?, NULL, ?, ?, now())`,
		uuid.NewString(), item.JobID, item.UserID, changeType, item.SourceTitle,
	).Exec(ctx); err != nil {
		slog.Error("darkadia_finalize: insert change", "err", err)
	}

	markItemCompletedWithResult(bg, w.DB, &item, map[string]any{
		"game_id": igdbID, "user_game_id": ug.ID, "is_new_addition": !alreadyExists,
	}, "darkadia_finalize: markItemCompleted")
	darkadiaCheckJobCompletion(w.DB, item.JobID)
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseDateOnly(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}

// ensureGameRow inserts the games row for igdbID if absent, fetching full IGDB
// metadata + cover when the client is configured, else a title-only stub.
func ensureGameRow(ctx context.Context, db *bun.DB, client *igdb.Client, storagePath string, igdbID int32, title string) error {
	var existing models.Game
	if db.NewSelect().Model(&existing).Where("id = ?", igdbID).Scan(ctx) == nil {
		return nil
	}
	if client != nil && client.Configured() {
		if md, err := client.FetchFullMetadata(ctx, int(igdbID)); err == nil {
			g := igdbMetadataToGame(md)
			if md.CoverImageID != "" {
				if localURL, derr := client.DownloadCoverArt(ctx, md.CoverImageID, storagePath); derr == nil {
					g.CoverArtUrl = &localURL
				}
			}
			_, err = db.NewInsert().Model(g).On("CONFLICT (id) DO NOTHING").Exec(ctx)
			return err
		}
	}
	_, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		igdbID, title,
	).Exec(ctx)
	return err
}

// darkadiaCheckJobCompletion finalizes a Darkadia import job once no active or
// pending_review items remain. pending_review blocks termination (the job stays
// processing until the user resolves/skips every such item).
func darkadiaCheckJobCompletion(db *bun.DB, jobID string) {
	ctx := context.Background()
	active, ok := countJobItems(ctx, db, jobID, "status IN ('pending', 'processing')", "darkadia: count active")
	if !ok || active > 0 {
		return
	}
	review, ok := countJobItems(ctx, db, jobID, "status = 'pending_review'", "darkadia: count pending_review")
	if !ok || review > 0 {
		return
	}
	if !finalizeJobCompleted(ctx, db, jobID, "darkadia: finalize job", false) {
		return
	}
	uid, _ := syncJobUserAndStorefront(ctx, db, jobID)
	failed, ok := countJobItems(ctx, db, jobID, "status = 'failed'", "darkadia: count failed")
	if !ok {
		return
	}
	if failed > 0 {
		notify.Emit(ctx, db, notify.EmitParams{
			Type: notify.TypeImportFailed, Scope: notify.ScopeUser, ActorUserID: uid,
			Payload:  notify.ImportFailedPayload{JobID: jobID, Failed: failed, Error: fmt.Sprintf("%d item(s) failed to import", failed)},
			DedupKey: jobID + ":" + notify.TypeImportFailed,
		})
		return
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeImportCompleted, Scope: notify.ScopeUser, ActorUserID: uid,
		Payload:  notify.ImportCompletedPayload{JobID: jobID},
		DedupKey: jobID + ":" + notify.TypeImportCompleted,
	})
}
```

> Verify `models.JobItem.ResolvedIGDBID` is `*int` (it is, per the model) — adjust the `int32(...)` conversion accordingly. Verify `bun`'s `.On("CONFLICT (id) DO NOTHING")` form against an existing usage; if the codebase uses raw SQL for upserts, use `NewRaw` instead.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/worker/tasks/... -run TestDarkadia -v && go build ./...`
Expected: PASS + clean build (Task 5's `ArgsForJobType` change now compiles).

- [ ] **Step 5: Commit (Tasks 5 + 6 together)**

```bash
git add internal/db/models/jobs.go internal/worker/tasks/ internal/api/jobs.go internal/api/job_items.go
git commit -m "feat(import): darkadia match + finalize workers with additive merge and source-aware retry"
```

---

## Task 7: Upload handler + worker registration + route wiring

**Files:**
- Modify: `internal/api/import.go`
- Modify: `internal/api/router.go:320-322`, `:152` call to `NewImportHandler`
- Modify: `cmd/nexorious/serve.go` (both worker blocks: ~191 and ~271)
- Test: `internal/api/import_test.go` (add Darkadia cases)

- [ ] **Step 1: Write a failing handler test (IGDB guard + header reject + happy path)**

Add to the existing import test file (reuse its harness — shared `testDB`, an authed request helper). Reference existing tests for the exact request/auth helpers before writing.

```go
func TestImportDarkadia_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	// Build an ImportHandler whose igdbClient.Configured() == false.
	// Expect 400 mentioning IGDB when posting any file.
	// (Use the package's existing multipart-upload request helper.)
}

func TestImportDarkadia_RejectsNonDarkadiaHeader(t *testing.T) {
	truncateAllTables(t)
	// IGDB configured; upload a CSV whose header != canonical → 400 "not a Darkadia export".
}

func TestImportDarkadia_CreatesJobAndItems(t *testing.T) {
	truncateAllTables(t)
	// IGDB configured; upload a 2-game Darkadia CSV → 200, job source=darkadia,
	// total_items=2, two job_items in 'pending'.
}
```

Flesh these out using the patterns already in `import_test.go` (it already exercises `HandleImportNexorious`). The key assertions: status code, `job.Source == "darkadia"`, `COUNT(job_items) == 2`.

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run TestImportDarkadia -v`
Expected: FAIL — `HandleImportDarkadia` undefined.

- [ ] **Step 3: Add the IGDB client to `ImportHandler` and implement the handler**

In `internal/api/import.go`, extend the struct + constructor:

```go
type ImportHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	igdbClient  *igdb.Client
}

func NewImportHandler(db *bun.DB, riverClient *river.Client[pgx.Tx], igdbClient *igdb.Client) *ImportHandler {
	return &ImportHandler{db: db, riverClient: riverClient, igdbClient: igdbClient}
}
```

Add the import for `"github.com/drzero42/nexorious/internal/services/igdb"` and `"github.com/drzero42/nexorious/internal/services/darkadia"`. Then:

```go
// HandleImportDarkadia handles POST /api/import/darkadia.
func (h *ImportHandler) HandleImportDarkadia(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Prerequisite: IGDB must be configured, else every game lands unmatched.
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return echo.NewHTTPError(http.StatusBadRequest, "IGDB must be configured to import a Darkadia collection")
	}

	if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
	}
	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "missing file field")
	}
	defer func() { _ = file.Close() }()

	lr := io.LimitReader(file, maxImportBodyBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if len(body) > maxImportBodyBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}

	games, err := darkadia.Parse(body)
	if err != nil {
		if errors.Is(err, darkadia.ErrInvalidHeader) {
			return echo.NewHTTPError(http.StatusBadRequest, "not a Darkadia export")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
	if len(games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games found in CSV")
	}

	ctx := context.Background()

	// Refuse a duplicate in-progress Darkadia import.
	var existing models.Job
	err = h.db.NewSelect().Model(&existing).
		Where("user_id = ?", userID).
		Where("job_type = ?", models.JobTypeImport).
		Where("source = ?", models.JobSourceDarkadia).
		Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Limit(1).Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check active import")
	}
	if err == nil {
		return echo.NewHTTPError(http.StatusConflict, "an active Darkadia import is already in progress")
	}

	now := time.Now().UTC()
	job := &models.Job{
		ID: uuid.NewString(), UserID: userID, JobType: models.JobTypeImport,
		Source: models.JobSourceDarkadia, Status: models.JobStatusProcessing,
		Priority: models.JobPriorityHigh, TotalItems: len(games),
		DispatchComplete: true, CreatedAt: now,
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	for i, g := range games {
		meta, err := json.Marshal(g)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal game payload")
		}
		item := &models.JobItem{
			ID: uuid.NewString(), JobID: job.ID, UserID: userID,
			ItemKey: fmt.Sprintf("game_%d", i), SourceTitle: g.Title,
			SourceMetadata: meta, Status: models.JobItemStatusPending,
			Result: json.RawMessage(`{}`), IGDBCandidates: json.RawMessage(`[]`),
		}
		if _, err := h.db.NewInsert().Model(item).Exec(ctx); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.DarkadiaMatchArgs{JobItemID: item.ID}, nil); err != nil {
				slog.Error("import: submit darkadia_match", "item_id", item.ID, "err", err)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id": job.ID, "source": job.Source, "status": job.Status,
		"message":     fmt.Sprintf("Darkadia import job created. Matching %d games.", len(games)),
		"total_items": len(games),
	})
}
```

> Job starts in `processing` (not `pending`) because the items immediately enter matching and `darkadiaCheckJobCompletion` drives it to `completed`/keeps it `processing` for review. `finalizeJobCompleted`'s guard accepts both `pending` and `processing`.

- [ ] **Step 4: Wire the route + constructor**

In `internal/api/router.go`: update the `NewImportHandler` call to pass `igdbClient`, and add the route:

```go
		imh := NewImportHandler(db, riverClient, igdbClient)
		importGroup := e.Group("/api/import", auth.AuthMiddleware(db))
		importGroup.POST("/nexorious", imh.HandleImportNexorious)
		importGroup.POST("/darkadia", imh.HandleImportDarkadia)
```

- [ ] **Step 5: Register both workers in both serve.go blocks**

In `cmd/nexorious/serve.go`, after the `ImportItemWorker` registration in the **first** block (~line 191):

```go
	river.AddWorker(workers, &tasks.DarkadiaMatchWorker{DB: db, IGDBClient: igdbClient, RiverClient: nil})
	river.AddWorker(workers, &tasks.DarkadiaFinalizeWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
```

The `RiverClient` is set after the client is constructed (mirror how `igdbMatchWorker`/`userGameWorker` receive `RiverClient` — they are struct vars assigned the client later; follow that exact pattern). If the existing code assigns `igdbMatchWorker.RiverClient = riverClient` after client creation, create `darkadiaMatchWorker := &tasks.DarkadiaMatchWorker{...}` as a var and assign its `RiverClient` the same way. Repeat the same two registrations in the **second** block (~line 271) using `newDB`/`newWorkers`.

> Check exactly how `IGDBMatchWorker.RiverClient` gets the client in serve.go before/after `river.NewClient`, and replicate it for `DarkadiaMatchWorker` so finalize tasks can be enqueued.

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/api/... -run TestImportDarkadia -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 7: Commit**

```bash
git add internal/api/import.go internal/api/router.go cmd/nexorious/serve.go internal/api/import_test.go
git commit -m "feat(import): darkadia CSV upload endpoint, IGDB guard, and worker registration"
```

---

## Task 8: Import-scoped resolve/skip endpoints

**Files:**
- Modify: `internal/api/job_items.go`
- Modify: `internal/api/router.go:315-317`
- Test: `internal/api/job_items_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestResolveItem_ImportScopedRejectsSyncJob(t *testing.T) {
	truncateAllTables(t)
	// Insert a SYNC job + pending_review job_item. POST /api/job-items/:id/resolve
	// {igdb_id:1} → expect 4xx (sync items use the external_games rematch path).
}

func TestResolveItem_SetsResolvedAndEnqueuesFinalize(t *testing.T) {
	truncateAllTables(t)
	// Insert a darkadia job + pending_review item. POST resolve {igdb_id:42}
	// → 200, job_item.resolved_igdb_id == 42, status moved out of pending_review.
}

func TestSkipItem_ImportScoped(t *testing.T) {
	truncateAllTables(t)
	// darkadia job + pending_review item. POST /api/job-items/:id/skip
	// → 200, status == 'skipped'.
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/api/... -run 'TestResolveItem|TestSkipItem' -v`
Expected: FAIL — handlers undefined.

- [ ] **Step 3: Implement the handlers**

Add to `internal/api/job_items.go`. Define the set of import sources that may use the generic resolve/skip:

```go
// importSources are the job sources whose job_items use the generic job-item
// resolve/skip path. Sync sources resolve through the external_games rematch
// endpoints instead and must be rejected here.
func isImportSource(source string) bool {
	return source == models.JobSourceDarkadia || source == models.JobSourceNexorious
}

type resolveItemRequest struct {
	IGDBID int `json:"igdb_id"`
}

// HandleResolveItem handles POST /api/job-items/:id/resolve.
func (h *JobItemsHandler) HandleResolveItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	itemID := c.Param("id")

	var req resolveItemRequest
	if err := c.Bind(&req); err != nil || req.IGDBID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	item, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if !isImportSource(job.Source) {
		return echo.NewHTTPError(http.StatusBadRequest, "this item is resolved through the sync flow")
	}
	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	_, err = h.db.NewRaw(
		`UPDATE job_items SET resolved_igdb_id = ?, status = ?, resolved_at = now() WHERE id = ?`,
		req.IGDBID, models.JobItemStatusProcessing, itemID,
	).Exec(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve item")
	}

	if err := tasks.EnqueueOrFail(context.Background(), h.db, h.riverClient, itemID,
		tasks.DarkadiaFinalizeArgs{JobItemID: itemID}); err != nil {
		slog.Error("resolve_item: enqueue finalize", "item_id", itemID, "err", err)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleSkipItem handles POST /api/job-items/:id/skip.
func (h *JobItemsHandler) HandleSkipItem(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	itemID := c.Param("id")

	item, job, err := h.loadItemAndJob(itemID, userID)
	if err != nil {
		return err
	}
	if !isImportSource(job.Source) {
		return echo.NewHTTPError(http.StatusBadRequest, "this item is skipped through the sync flow")
	}
	if item.Status != models.JobItemStatusPendingReview {
		return echo.NewHTTPError(http.StatusConflict, "item is not pending review")
	}

	if _, err := h.db.NewRaw(
		`UPDATE job_items SET status = ?, processed_at = now() WHERE id = ?`,
		models.JobItemStatusSkipped, itemID,
	).Exec(context.Background()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip item")
	}
	tasks.DarkadiaCheckJobCompletion(h.db, job.ID)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// loadItemAndJob loads a job_item (scoped to the user) plus its parent job.
func (h *JobItemsHandler) loadItemAndJob(itemID, userID string) (*models.JobItem, *models.Job, error) {
	var item models.JobItem
	if err := h.db.NewRaw(`SELECT * FROM job_items WHERE id = ? AND user_id = ?`, itemID, userID).
		Scan(context.Background(), &item); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to get job item")
	}
	var job models.Job
	if err := h.db.NewRaw(`SELECT * FROM jobs WHERE id = ?`, item.JobID).
		Scan(context.Background(), &job); err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to get parent job")
	}
	return &item, &job, nil
}
```

Add imports to `job_items.go`: `"log/slog"`, `"github.com/drzero42/nexorious/internal/worker/tasks"`. Export the completion checker by renaming `darkadiaCheckJobCompletion` → `DarkadiaCheckJobCompletion` in `internal/worker/tasks/darkadia.go` (update its internal call sites), so the API package can call it from the skip handler.

- [ ] **Step 4: Wire routes**

In `internal/api/router.go`, in the `jobItemsGroup` block:

```go
		jobItemsGroup.POST("/:id/retry", jih.HandleRetryItem)
		jobItemsGroup.POST("/:id/resolve", jih.HandleResolveItem)
		jobItemsGroup.POST("/:id/skip", jih.HandleSkipItem)
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/api/... -run 'TestResolveItem|TestSkipItem' -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/api/job_items.go internal/api/router.go internal/worker/tasks/darkadia.go
git commit -m "feat(import): import-scoped resolve/skip endpoints for job items"
```

---

## Task 9: Frontend — source registration, API, upload card

**Files:**
- Modify: `ui/frontend/src/types/import-export.ts`
- Modify: `ui/frontend/src/api/import-export.ts`
- Modify: the import/export route (`ui/frontend/src/routes/_authenticated/import-export.tsx`)

- [ ] **Step 1: Add the source enum + display info**

In `ui/frontend/src/types/import-export.ts`:

```ts
export enum ImportSource {
  NEXORIOUS = 'nexorious',
  DARKADIA = 'darkadia',
}
```

Widen the `color` union to include the new card's colour and add the entry to `getImportSourceDisplayInfo`'s `info` map:

```ts
    [ImportSource.DARKADIA]: {
      title: 'Darkadia CSV',
      description:
        'Migrate a Darkadia collection export. Games are matched to IGDB; ambiguous matches go to review. Requires IGDB to be configured.',
      icon: '🗄️',
      features: ['Preserves ratings, notes & added date', 'Matches to IGDB', 'Interactive review'],
      color: 'purple',
    },
```

- [ ] **Step 2: Add the API function**

In `ui/frontend/src/api/import-export.ts`:

```ts
/**
 * Import a Darkadia CSV export. Parsing/matching happens server-side; the
 * returned job drives matching + interactive review.
 */
export async function importDarkadiaCsv(file: File): Promise<ImportJobCreatedResponse> {
  const response = await apiUploadFile<ImportJobApiResponse>('/import/darkadia', file);
  return transformImportJobResponse(response);
}
```

- [ ] **Step 3: Add the card to the import/export page**

Read `ui/frontend/src/routes/_authenticated/import-export.tsx` first. The page already maps over import sources / renders an `ImportCard` per source and dispatches the upload by `selectedSource`. Add `ImportSource.DARKADIA` to the rendered source list and route its upload to `importDarkadiaCsv` (mirror the `importNexoriousJson` wiring; accept `.csv`).

- [ ] **Step 4: Verify build + typecheck**

Run (from `ui/frontend/`): `npm run build && npm run check`
Expected: clean (routeTree regenerates if a route changed — commit it if so).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/import-export.ts ui/frontend/src/api/import-export.ts ui/frontend/src/routes/_authenticated/import-export.tsx
git commit -m "feat(import): darkadia CSV upload card and API client"
```

---

## Task 10: Frontend — resolve/skip hooks + Find-Match/Skip in JobItemsDetails

**Files:**
- Modify: `ui/frontend/src/hooks/use-jobs.ts`
- Modify: `ui/frontend/src/components/jobs/job-items-details.tsx`

- [ ] **Step 1: Add resolve/skip mutation hooks**

In `ui/frontend/src/hooks/use-jobs.ts`, mirror `useRetryJobItem` to add (use the existing api client + query invalidation pattern in that file):

```ts
export function useResolveJobItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ itemId, igdbId }: { itemId: string; igdbId: number }) =>
      api.post(`/job-items/${itemId}/resolve`, { igdb_id: igdbId }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['jobs'] }),
  });
}

export function useSkipJobItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (itemId: string) => api.post(`/job-items/${itemId}/skip`, {}),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['jobs'] }),
  });
}
```

(Match the exact `api` import, query-key shape, and invalidation already used by `useRetryJobItem` in that file.)

- [ ] **Step 2: Add Find-Match + Skip actions to the pending_review section**

In `job-items-details.tsx`, extend `StatusSection`: when `status === JobItemStatus.PENDING_REVIEW`, render per-item **Find Match** and **Skip** buttons (alongside the existing item row). Find Match opens `IGDBMatchDialog` (`import { IGDBMatchDialog } from '@/components/sync/igdb-match-dialog'`) with `initialQuery={item.sourceTitle}` and no `externalGameId`; its `onSelect` calls `resolveMutation.mutateAsync({ itemId: item.id, igdbId: candidate.igdb_id })` then `refetch()`. Skip calls `skipMutation.mutateAsync(item.id)` then `refetch()`.

```tsx
// near the other mutations in StatusSection:
const resolveMutation = useResolveJobItem();
const skipMutation = useSkipJobItem();
const [matchItem, setMatchItem] = useState<{ id: string; title: string } | null>(null);
const isReview = status === JobItemStatus.PENDING_REVIEW;
```

Render (inside the item row, augmenting the retry button area, gated by `isReview`):

```tsx
{isReview && (
  <div className="flex gap-1 ml-2">
    <Button variant="outline" size="sm" className="h-8"
      onClick={() => setMatchItem({ id: item.id, title: item.sourceTitle })}>
      Find Match
    </Button>
    <Button variant="ghost" size="sm" className="h-8"
      disabled={skipMutation.isPending}
      onClick={async () => {
        try { await skipMutation.mutateAsync(item.id); toast.success('Skipped'); refetch(); }
        catch (e) { toast.error(e instanceof Error ? e.message : 'Failed to skip'); }
      }}>
      Skip
    </Button>
  </div>
)}
```

And once, after the items list:

```tsx
{matchItem && (
  <IGDBMatchDialog
    open
    initialQuery={matchItem.title}
    onClose={() => setMatchItem(null)}
    onSelect={async (candidate) => {
      try {
        await resolveMutation.mutateAsync({ itemId: matchItem.id, igdbId: candidate.igdb_id });
        toast.success('Match applied');
        setMatchItem(null);
        refetch();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : 'Failed to apply match');
      }
    }}
  />
)}
```

Import the new hooks at the top: `import { useJobItems, useRetryFailedItems, useRetryJobItem, useResolveJobItem, useSkipJobItem } from '@/hooks';` (confirm `@/hooks` re-exports them; if hooks are imported from a specific path, match it).

- [ ] **Step 3: Verify build, typecheck, knip, tests**

Run (from `ui/frontend/`): `npm run build && npm run check && npm run knip && npm run test`
Expected: clean. Fix any knip findings (e.g. if `IGDBGameCandidate` type import is needed).

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-jobs.ts ui/frontend/src/components/jobs/job-items-details.tsx
git commit -m "feat(import): find-match and skip actions for pending-review import items"
```

---

## Task 11: Full backend suite + end-to-end verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full Go suite**

Run: `go test -timeout 600s ./...`
Expected: PASS. Pay attention to `internal/services/darkadia`, `internal/services/matching`, `internal/worker/tasks`, `internal/api`.

- [ ] **Step 2: Run golangci-lint**

Run: `golangci-lint run`
Expected: zero errors. (Watch for `errcheck` on discarded errors and `gosec` on the int conversions — annotate per the CLAUDE.md guidance if flagged.)

- [ ] **Step 3: End-to-end against the real export (manual)**

Bring up a fresh dev DB with IGDB configured, build, and run migrations. Then upload the maintainer's real CSV via the UI (or `curl`), e.g.:

```bash
curl -sS -b "<session-cookie>" -F file=@/storage/filebase/download/darkadia_export_csv_20251213.csv \
  http://localhost:8000/api/import/darkadia
```

Verify:
- Response 200; job `source=darkadia`, `total_items ≈ 1474`.
- After matching settles: a mix of auto-resolved (completed) and `pending_review` items; the job stays `processing` while review items remain.
- Spot-check in the DB: a known multi-copy game (e.g. Anodyne → `pc-windows`+`mac`), a no-copy multi-platform game, a no-platform game, and a game with a physical/unrecognized source → provenance line present in `personal_notes`.
- Resolve one `pending_review` item via the UI → it finalizes into the library; skip another → it goes to `skipped`; the job completes once none remain.
- Re-run the same import → existing curation (rating/status/notes) is unchanged; no duplicate `(platform, storefront)` rows.

The CSV is read-only and must never be committed.

- [ ] **Step 4: Final commit (if any lint/test fixups were needed)**

```bash
git add -A
git commit -m "test(import): darkadia end-to-end verification fixups"
```

---

## Self-review notes (spec coverage)

- Upload guard (IGDB + header) → Task 7. Parse/consolidate (translate-at-parse) → Tasks 1–3. Match reuse via `matching.Decide` → Tasks 4, 6. Finalize additive merge → Task 6. Import-scoped resolve/skip → Task 8. `pending_review` blocks completion → Task 6 (`darkadiaCheckJobCompletion`). Unmapped platform → note (Decision B) → Task 3. Keep `JobSourceCSV`, add `JobSourceDarkadia` (Decision C) → Task 5. Frontend reuse (no new components) → Tasks 9–10. End-to-end real CSV → Task 11.
- **Type consistency:** `DarkadiaMatchArgs`/`DarkadiaFinalizeArgs`, `darkadia.Game`/`darkadia.Platform`, `matching.Decide`/`Decision`, `DarkadiaCheckJobCompletion` (exported for the API skip handler) are used consistently across tasks.
- **Verify-before-coding flags embedded in the plan:** confirm `models.JobItem.ResolvedIGDBID` pointer type; confirm the bun upsert idiom; confirm how `IGDBMatchWorker.RiverClient` is assigned in `serve.go` and replicate for `DarkadiaMatchWorker`; confirm the import/export page's source-rendering pattern; confirm `@/hooks` re-exports.
