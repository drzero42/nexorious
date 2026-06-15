# Completionator CSV Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Import a Completionator CSV export by adding a tolerant CSV reader (fixes malformed quoting + Windows-1252 encoding) and a reusable `csvmap` preset Config mapping Completionator's 24 columns.

**Architecture:** One new shared reader `csvmap.ReadRecords` (transcode → strict `encoding/csv` → guarded de-quote fallback) replaces the two `csv.NewReader` call sites in `parse.go` and `import_csv.go`. A new `csvmap.Completionator()` returns the preset `Config` (simple subset only). No migration, no schema change, no new dependency (`golang.org/x/text` is already in `go.mod`). The preset registry + auto-detect UX are deferred to #1015; after this plan a Completionator file is importable through the existing manual mapping dialog.

**Tech Stack:** Go, `encoding/csv`, `golang.org/x/text/encoding/charmap`, the existing `internal/services/csvmap` engine, Echo v5 handler `import_csv.go`.

**Spec:** `docs/superpowers/specs/2026-06-15-issue-1003-completionator-csv-import-design.md`

---

## File Structure

- **Create** `internal/services/csvmap/read.go` — `ReadRecords` + its helpers (`toUTF8`, `isQuoteError`, `dequoteSplit`). One responsibility: turn raw bytes into `[][]string` tolerantly.
- **Create** `internal/services/csvmap/read_test.go` — unit tests for the reader.
- **Modify** `internal/services/csvmap/parse.go` — `Parse` uses `ReadRecords` instead of an inline `csv.NewReader` loop.
- **Modify** `internal/services/csvmap/parse_test.go` — add a Parse-level malformed-quote test.
- **Modify** `internal/api/import_csv.go` — `HandleImportCSVInspect` uses `ReadRecords`; drop now-unused `bytes`/`encoding/csv` imports.
- **Modify** `internal/api/import_csv_test.go` — add an inspect test over a malformed Completionator-shaped file.
- **Create** `internal/services/csvmap/completionator.go` — `Completionator() Config`.
- **Create** `internal/services/csvmap/completionator_test.go` — Config behaviour over a representative fixture.
- **Create** `docs/completionator-import.md` — the format spec.

---

## Task 1: The tolerant reader `ReadRecords`

**Files:**
- Create: `internal/services/csvmap/read.go`
- Test: `internal/services/csvmap/read_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/services/csvmap/read_test.go`:

```go
package csvmap

import "testing"

// wrapped is a fully quote-wrapped 3-field header+rows helper for fallback tests.
const malformedQuoted = `"Name","Edition","Note"
"A Hat in Time","","ok"
"Episode 1: "Done Running"","","raw quotes"
`

func TestReadRecords_WellFormed_StrictPath(t *testing.T) {
	raw := []byte("Name,Status\nHalf-Life,Beaten\nPortal,Playing\n")
	recs, err := ReadRecords(raw)
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d: %v", len(recs), recs)
	}
	if recs[0][0] != "Name" || recs[1][0] != "Half-Life" || recs[2][1] != "Playing" {
		t.Fatalf("unexpected records: %v", recs)
	}
}

func TestReadRecords_MalformedQuotes_FallbackRecovers(t *testing.T) {
	recs, err := ReadRecords([]byte(malformedQuoted))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d: %v", len(recs), recs)
	}
	for i, r := range recs {
		if len(r) != 3 {
			t.Fatalf("record %d has %d fields, want 3: %v", i, len(r), r)
		}
	}
	if got := recs[2][0]; got != `Episode 1: "Done Running"` {
		t.Fatalf("recovered title = %q, want `Episode 1: \"Done Running\"`", got)
	}
}

func TestReadRecords_Windows1252_Transcoded(t *testing.T) {
	// 0xF4 is 'ô' in Windows-1252; invalid UTF-8 on its own.
	raw := []byte{'N', 'a', 'm', 'e', '\n', 'O', 'k', 'a', 'm', 'i', 0xF4, '\n'}
	recs, err := ReadRecords(raw)
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if recs[1][0] != "Okamiô" {
		t.Fatalf("transcoded = %q, want %q", recs[1][0], "Okamiô")
	}
}

func TestReadRecords_PartiallyQuoted_ReturnsError(t *testing.T) {
	// Strict parsing fails on the bare quote, and the file is NOT uniformly
	// quote-wrapped (line 2 isn't), so the fallback must not engage.
	raw := []byte("\"Name\",\"Note\"\nUnquoted: \"x\" here,plain\n")
	if _, err := ReadRecords(raw); err == nil {
		t.Fatal("want error for partially-quoted malformed file, got nil")
	}
}

func TestReadRecords_RaggedDequote_ReturnsError(t *testing.T) {
	// Uniformly quote-wrapped but a bare quote trips strict parsing AND the
	// de-quoted field counts differ (2 vs 3), so the guard rejects the fallback.
	raw := []byte("\"a\",\"b\"x\"\n\"c\",\"d\",\"e\"\n")
	if _, err := ReadRecords(raw); err == nil {
		t.Fatal("want error for ragged de-quote, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run TestReadRecords -v`
Expected: FAIL — `undefined: ReadRecords`.

- [ ] **Step 3: Write the implementation**

Create `internal/services/csvmap/read.go`:

```go
package csvmap

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// ReadRecords parses CSV bytes tolerantly and returns every record (header
// included). It (1) transcodes Windows-1252 input to UTF-8 when the bytes are
// not already valid UTF-8, (2) parses strictly with encoding/csv, and (3) only
// when strict parsing fails on a quote error AND the file is uniformly
// quote-wrapped, falls back to a de-quote split. Otherwise it returns the
// strict error rather than risk the silent corruption LazyQuotes would cause.
func ReadRecords(raw []byte) ([][]string, error) {
	text := toUTF8(raw)

	r := csv.NewReader(bytes.NewReader(text))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)
	records, err := r.ReadAll()
	if err == nil {
		return records, nil
	}
	if !isQuoteError(err) {
		return nil, err
	}
	if fallback, ok := dequoteSplit(text); ok {
		return fallback, nil
	}
	return nil, err
}

// toUTF8 returns raw unchanged when it is already valid UTF-8 (including pure
// ASCII); otherwise it decodes the bytes as Windows-1252 (a Latin-1 superset
// covering the smart-quote/dash range that real exports use).
func toUTF8(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}
	decoded, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil {
		return raw // Windows-1252 maps every byte; this path is unreachable in practice
	}
	return decoded
}

// isQuoteError reports whether err is a csv parse error caused by malformed quoting.
func isQuoteError(err error) bool {
	var pe *csv.ParseError
	if !errors.As(err, &pe) {
		return false
	}
	return errors.Is(pe.Err, csv.ErrQuote) || errors.Is(pe.Err, csv.ErrBareQuote)
}

// dequoteSplit recovers a fully quote-wrapped file (every non-empty line
// matching ^"…"$) by stripping the outer quotes and splitting each line on the
// literal separator `","`. It returns ok=false unless every non-empty line is
// quote-wrapped AND yields the same field count — so a partially-quoted file, a
// multi-line quoted field, or a literal `","` inside a field falls through to
// the strict error instead of misaligning columns.
func dequoteSplit(text []byte) ([][]string, bool) {
	lines := strings.Split(string(text), "\n")
	records := make([][]string, 0, len(lines))
	want := -1
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if len(line) < 2 || line[0] != '"' || line[len(line)-1] != '"' {
			return nil, false
		}
		fields := strings.Split(line[1:len(line)-1], `","`)
		if want == -1 {
			want = len(fields)
		} else if len(fields) != want {
			return nil, false
		}
		records = append(records, fields)
	}
	if len(records) == 0 {
		return nil, false
	}
	return records, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run TestReadRecords -v`
Expected: PASS (all five).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/read.go internal/services/csvmap/read_test.go
git commit -m "feat: tolerant csvmap reader (quote-fallback + Windows-1252)"
```

---

## Task 2: Route `Parse` through `ReadRecords`

**Files:**
- Modify: `internal/services/csvmap/parse.go:65-94`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestParse_MalformedQuotes_Recovered(t *testing.T) {
	// A bare-quoted title that strict encoding/csv rejects; Parse must recover
	// it via ReadRecords' fallback.
	csv := "\"Name\",\"Other\"\n" +
		"\"Episode 1: \"Done Running\"\",\"x\"\n" +
		"\"Portal\",\"y\"\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}
	if games[0].Title != `Episode 1: "Done Running"` {
		t.Fatalf("title = %q, want `Episode 1: \"Done Running\"`", games[0].Title)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestParse_MalformedQuotes_Recovered -v`
Expected: FAIL — strict parse currently errors with `parse error … in quoted-field`.

- [ ] **Step 3: Rewrite `Parse`**

In `internal/services/csvmap/parse.go`, replace the whole `Parse` function (lines 65-94) with:

```go
// Parse maps a CSV export into canonical games per cfg. On a wrong-shape file
// (failed Signature) it returns an error wrapping importmodel.ErrInvalidSignature.
func Parse(raw []byte, cfg Config) ([]importmodel.Game, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	records, err := ReadRecords(raw)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, io.EOF
	}

	header := records[0]
	if !MatchesSignature(header, cfg) {
		return nil, fmt.Errorf("csv does not match the expected format: %w", importmodel.ErrInvalidSignature)
	}
	idx := buildIndex(header)

	return buildGames(records[1:], idx, cfg), nil
}
```

The `io`, `fmt`, and `bytes`/`encoding/csv` imports: `io` is still used (`io.EOF`), `fmt` is still used. `bytes` and `encoding/csv` are now unused in `parse.go` — remove them from the import block (lines 4-5). Final import block:

```go
import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)
```

- [ ] **Step 4: Run the csvmap suite to verify pass + no regressions**

Run: `go test ./internal/services/csvmap/ -v`
Expected: PASS (existing tests + the new one).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "refactor: Parse reads via tolerant ReadRecords"
```

---

## Task 3: Route the inspect handler through `ReadRecords`

**Files:**
- Modify: `internal/api/import_csv.go:152-247` (the `HandleImportCSVInspect` body and imports)
- Test: `internal/api/import_csv_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/import_csv_test.go`:

```go
func TestImportCSVInspect_MalformedCompletionatorQuotes(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-inspect-malformed")

	// Fully quote-wrapped, with a bare-quoted title strict encoding/csv rejects.
	csvData := []byte("\"Name\",\"Platform\"\n" +
		"\"A Hat in Time\",\"PC / Windows\"\n" +
		"\"Episode 1: \"Done Running\"\",\"PC / Windows\"\n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "completionator.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		RowCount int `json:"row_count"`
		Columns  []struct {
			Name           string   `json:"name"`
			DistinctValues []string `json:"distinct_values"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RowCount != 2 {
		t.Fatalf("row_count = %d, want 2", resp.RowCount)
	}
	var names []string
	for _, c := range resp.Columns {
		if c.Name == "Name" {
			names = c.DistinctValues
		}
	}
	found := false
	for _, n := range names {
		if n == `Episode 1: "Done Running"` {
			found = true
		}
	}
	if !found {
		t.Fatalf("recovered Name values = %v, want one to be `Episode 1: \"Done Running\"`", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestImportCSVInspect_MalformedCompletionatorQuotes -v`
Expected: FAIL — current handler returns 400 "failed to parse CSV" on the bare quote.

- [ ] **Step 3: Rewrite the inspect read loop**

In `internal/api/import_csv.go`, inside `HandleImportCSVInspect`, replace the strict-reader block (currently lines 165-225 — from `r := csv.NewReader(...)` down to the end of the `for { ... }` record loop) with a `ReadRecords` call and a slice walk. The new body from just after `body, herr := h.readUploadFile(c)` / `if herr != nil { return herr }`:

```go
	records, err := csvmap.ReadRecords(body)
	if err != nil || len(records) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
	}
	header := records[0]

	// Guess the column->field mapping from the headers alone so we know which
	// column is the rating column (to track its numeric max below).
	suggested := csvmap.GuessColumns(header)
	ratingIdx := -1
	if suggested.Columns.Rating != "" {
		for i, name := range header {
			if name == suggested.Columns.Rating {
				ratingIdx = i
				break
			}
		}
	}

	cols := make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	rowCount := 0
	var ratingMax float64
	for _, rec := range records[1:] {
		rowCount++
		for i := range header {
			if i >= len(rec) {
				continue
			}
			v := strings.TrimSpace(rec[i])
			if i == ratingIdx && v != "" {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil && f > ratingMax {
					ratingMax = f
				}
			}
			if v == "" || cols[i].DistinctTruncated || seen[i][v] {
				continue
			}
			if len(cols[i].DistinctValues) < csvDistinctCap {
				seen[i][v] = true
				cols[i].DistinctValues = append(cols[i].DistinctValues, v)
			} else {
				cols[i].DistinctTruncated = true
			}
		}
	}
```

Leave everything from `// Refine the suggestion from the data:` to the final `return c.JSON(...)` unchanged.

- [ ] **Step 4: Fix imports**

`encoding/csv` and `bytes` are no longer used in `import_csv.go` (`io` is still used by `readUploadFile`). Remove `bytes` and `encoding/csv` from the import block. Resulting import block:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/csvmap"
)
```

- [ ] **Step 5: Run the test + the existing inspect tests**

Run: `go test ./internal/api/ -run TestImportCSVInspect -v`
Expected: PASS (the new malformed test and all existing inspect tests).

- [ ] **Step 6: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "refactor: CSV inspect reads via tolerant ReadRecords"
```

---

## Task 4: The Completionator preset Config

**Files:**
- Create: `internal/services/csvmap/completionator.go`
- Test: `internal/services/csvmap/completionator_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/services/csvmap/completionator_test.go`:

```go
package csvmap

import "testing"

// completionatorFixture is a fully quote-wrapped 24-column Completionator export
// (the real malformed-quote shape) with three games: a rated/Incomplete PC
// title, the bare-quoted "Done Running" title, and a Finished PlayStation 5/GOG
// title. Column order matches a real export.
const completionatorFixture = `"Name","Edition","Platform","Format","Region","Now Playing","Backlogged","Ownership Status","Progress Status","Est. Value","Amt. Paid","Tags","Box/Case","Cart/Disc","Manual","Extras","Acquisition Type","Acquisition Source","Acquisition Date","Rating","Initial Release Date","Item Release Date","Added On","Genre"
"A Hat in Time","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","10","10/5/2017","","1/17/2022","Platformer"
"The Walking Dead: The Final Season - Episode 1: "Done Running"","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","","","","1/17/2022",""
"Batman: Arkham Asylum - Game of the Year Edition","","PlayStation 5","Digital (GOG)","EU","No","No","Owned","Finished","","","","","","","","Purchase","","","3","","","1/17/2022","Action"
`

func TestCompletionator_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(completionatorFixture), Completionator())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d: %+v", len(games), games)
	}

	// Game 1: A Hat in Time — Incomplete -> not_started, rating 10/10 -> 5 stars,
	// pc-windows / steam, Added On -> CreatedAt.
	g := games[0]
	if g.Title != "A Hat in Time" {
		t.Fatalf("g0 title = %q", g.Title)
	}
	if g.PlayStatus != "not_started" {
		t.Errorf("g0 status = %q, want not_started", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 5 {
		t.Errorf("g0 rating = %v, want 5", g.PersonalRating)
	}
	if g.CreatedAt != "2022-01-17" {
		t.Errorf("g0 created = %q, want 2022-01-17", g.CreatedAt)
	}
	if len(g.Platforms) != 1 || g.Platforms[0].Platform != "pc-windows" {
		t.Fatalf("g0 platforms = %+v", g.Platforms)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "steam" {
		t.Errorf("g0 storefront = %v, want steam", g.Platforms[0].Storefront)
	}
	if len(g.Tags) != 0 {
		t.Errorf("g0 tags = %v, want none (Genre is not mapped)", g.Tags)
	}

	// Game 2: bare-quoted title recovered exactly; no rating.
	g = games[1]
	if g.Title != `The Walking Dead: The Final Season - Episode 1: "Done Running"` {
		t.Fatalf("g1 title = %q", g.Title)
	}
	if g.PersonalRating != nil {
		t.Errorf("g1 rating = %v, want nil", g.PersonalRating)
	}

	// Game 3: Finished -> completed, rating 3/10 -> 2 stars, playstation-5 / gog.
	g = games[2]
	if g.PlayStatus != "completed" {
		t.Errorf("g2 status = %q, want completed", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 2 {
		t.Errorf("g2 rating = %v, want 2", g.PersonalRating)
	}
	if len(g.Platforms) != 1 || g.Platforms[0].Platform != "playstation-5" {
		t.Fatalf("g2 platforms = %+v", g.Platforms)
	}
	if g.Platforms[0].Storefront == nil || *g.Platforms[0].Storefront != "gog" {
		t.Errorf("g2 storefront = %v, want gog", g.Platforms[0].Storefront)
	}
}

func TestCompletionator_Signature(t *testing.T) {
	cfg := Completionator()
	completionatorHeader := []string{
		"Name", "Edition", "Platform", "Format", "Region", "Now Playing",
		"Backlogged", "Ownership Status", "Progress Status", "Added On",
	}
	if !MatchesSignature(completionatorHeader, cfg) {
		t.Error("Completionator signature should match a real header")
	}
	if MatchesSignature([]string{"Title", "Console", "Status"}, cfg) {
		t.Error("Completionator signature should not match an unrelated header")
	}
}
```

Note: the test asserts positionally through `games[i]` and constructs no `Game` literal, so `completionator_test.go` needs only the `testing` import (no `importmodel` import).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestCompletionator -v`
Expected: FAIL — `undefined: Completionator`.

- [ ] **Step 3: Write the Config**

Create `internal/services/csvmap/completionator.go`:

```go
package csvmap

// Completionator returns the preset Config for a Completionator CSV export.
// Completionator quote-wraps every field (with unescaped embedded quotes) and
// exports Windows-1252; ReadRecords handles both. Play-status comes from the
// single Progress Status column (the Now Playing / Backlogged flags are not
// honoured — that would need the advanced StatusFlags, #1016). Ratings are a
// 1-10 scale. See docs/completionator-import.md.
func Completionator() Config {
	return Config{
		Signature: []string{
			"Now Playing", "Backlogged", "Ownership Status",
			"Acquisition Source", "Added On",
		},
		Columns: ColumnMap{
			Title:     "Name",
			Rating:    "Rating",
			CreatedAt: "Added On",
			Tags:      "Tags",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column: "Progress Status",
				ValueMap: map[string]string{
					"finished":   "completed",
					"incomplete": "not_started",
				},
				Default: "not_started",
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn:     "Platform",
				StorefrontColumn:   "Format",
				AcquiredDateColumn: "Acquisition Date",
				PlatformMap: map[string]string{
					"pc / windows":  "pc-windows",
					"playstation 5": "playstation-5",
				},
				StorefrontMap: map[string]string{
					"digital (steam)":       "steam",
					"digital (gog)":         "gog",
					"physical (unassigned)": "physical",
					"physical (new)":        "physical",
					"physical (used)":       "physical",
				},
			},
		},
		Rating:     &RatingConfig{Scale: 10, Truncate: false},
		DateLayout: "1/2/2006",
		Grouping:   GroupingConfig{MergeByTitle: true},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestCompletionator -v`
Expected: PASS (both tests).

- [ ] **Step 5: Run the whole csvmap package**

Run: `go test ./internal/services/csvmap/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/csvmap/completionator.go internal/services/csvmap/completionator_test.go
git commit -m "feat: Completionator csvmap preset Config (#1003)"
```

---

## Task 5: Format documentation

**Files:**
- Create: `docs/completionator-import.md`

- [ ] **Step 1: Write the doc**

Create `docs/completionator-import.md` (modelled on `docs/darkadia-import.md`):

```markdown
# Completionator CSV Import

This document is the source of truth for how Nexorious imports a game collection
from a **Completionator CSV export**. Completionator is an active game-tracking
service; this is a **one-off migration path**, not a recurring sync.

Completionator is supported as a `csvmap` **preset `Config`** (`csvmap.Completionator()`),
not a bespoke mapper. Identifying games requires IGDB; the import is blocked
unless IGDB is configured.

## The export format

A Completionator CSV is **one row per game** with a 24-column header:

```
Name, Edition, Platform, Format, Region, Now Playing, Backlogged,
Ownership Status, Progress Status, Est. Value, Amt. Paid, Tags, Box/Case,
Cart/Disc, Manual, Extras, Acquisition Type, Acquisition Source,
Acquisition Date, Rating, Initial Release Date, Item Release Date, Added On, Genre
```

### Two format quirks (handled by `csvmap.ReadRecords`)

1. **Malformed quoting.** Every field is quote-wrapped, but embedded quotes in
   titles are **not** RFC-4180-escaped — e.g.
   `"...Episode 1: "Done Running"",""...`. Strict `encoding/csv` rejects this.
   `ReadRecords` falls back to stripping the outer quotes and splitting each line
   on the literal `","` — but only when every line is uniformly quote-wrapped, so
   a genuinely malformed file errors rather than corrupting silently.
2. **Windows-1252 encoding.** Exports are Windows-1252, not UTF-8. `ReadRecords`
   transcodes to UTF-8 when the bytes are not already valid UTF-8, so accented
   titles import correctly.

## Mapping into Nexorious

| Nexorious field | Completionator column | Notes |
|---|---|---|
| Title | `Name` | required |
| Play status | `Progress Status` | `Finished` → completed, `Incomplete` → not_started (default) |
| Platform | `Platform` | `PC / Windows` → `pc-windows`, `PlayStation 5` → `playstation-5` |
| Storefront | `Format` | `Digital (Steam)` → `steam`, `Digital (GOG)` → `gog`, `Physical (*)` → `physical` |
| Acquired date | `Acquisition Date` | `M/D/YYYY` |
| Added date | `Added On` | `M/D/YYYY` |
| Rating | `Rating` | 1–10 scale → 1–5 stars, round-to-nearest |
| Tags | `Tags` | comma-separated |

Rows sharing a title are merged into one game (platform entries unioned).

### Deliberately not mapped

`Edition`, `Region`, `Now Playing` / `Backlogged` (see status note below),
`Est. Value`, `Amt. Paid`, `Box/Case`, `Cart/Disc`, `Manual`, `Extras`,
`Acquisition Type`, `Acquisition Source`, `Initial Release Date`,
`Item Release Date`, and `Genre` (IGDB re-supplies genre on match).

### Known limitations

- **Play status uses `Progress Status` only.** Completionator also has `Now
  Playing` and `Backlogged` flags; honouring their precedence would need the
  advanced `StatusFlags` engine feature (#1016). Until then, a "Now Playing"
  game imports as `not_started` rather than `in_progress`.
- **Platform / `Format` value maps are derived from observed exports.** An
  unmapped platform value imports the game without that platform (logged); an
  unmapped `Format` records the platform without a storefront. The maps are
  extensible as new values are confirmed.
```

- [ ] **Step 2: Commit**

```bash
git add docs/completionator-import.md
git commit -m "docs: Completionator CSV import format reference (#1003)"
```

---

## Final verification

- [ ] **Run the full affected suites**

Run: `go test ./internal/services/csvmap/ ./internal/api/`
Expected: PASS.

- [ ] **Build + vet the whole module** (mirrors the Stop hook)

Run: `go build ./... && go vet ./internal/services/csvmap/ ./internal/api/`
Expected: no output, exit 0.

- [ ] **Confirm scope:** no migration was added, no frontend changed, `Completionator()` is exported but not yet registered anywhere (that is #1015). `git status` should show only the files listed in this plan.

---

## Self-review notes (for the implementer)

- **Spec coverage:** tolerant reader (Task 1) ✔, both call sites rewired (Tasks 2–3) ✔, Completionator Config + signature (Task 4) ✔, doc (Task 5) ✔, tests at each layer ✔, rating 1–10 mapped ✔, #1015 boundary respected (no registry) ✔.
- **Type consistency:** `ReadRecords([]byte) ([][]string, error)` is used identically in `parse.go` and `import_csv.go`; `Completionator() Config` matches the `Config` shape in `config.go` (simple subset only — passes `validate`).
- Task 4's test asserts positionally (`games[0..2]`) — `Parse` preserves first-seen order, and the three fixture titles are distinct so `MergeByTitle` produces exactly three games in fixture order.
```
