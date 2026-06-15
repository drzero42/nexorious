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

---

# Added scope: manual format selection (Tasks 6–9)

**Goal:** Let a user pick a known CSV format (`Generic CSV` / `Completionator`) in the import dialog, so the Completionator preset is actually reachable. A preset import uses the **server-side `Config`** (preserving the platform/storefront slug maps), not a client-rebuilt mapping. Auto-detection stays #1015; this adds the manual selector + the shared registry #1015 will reuse.

**Spec:** the "Addendum (2026-06-15): manual format selection rolled in" section of the spec.

### Added file structure
- Create `internal/services/csvmap/presets.go` (+ `presets_test.go`) — the preset registry.
- Modify `internal/api/import_csv.go` (+ `import_csv_test.go`) — `format` field on import, presets on inspect.
- Modify `ui/frontend/src/types/import-export.ts` — `CsvInspectResponse.presets`.
- Modify `ui/frontend/src/api/import-export.ts` — `importCsv(file, format, mapping)`.
- Modify `ui/frontend/src/hooks/use-import-export.ts` — `useImportCsv` carries `format`.
- Modify `ui/frontend/src/components/import/csv-mapping-dialog.tsx` (+ `.test.tsx`) — the Format selector + conditional form.
- Modify `ui/frontend/src/routes/_authenticated/import-export.tsx` — `handleCsvImport` wiring.

---

## Task 6: Backend preset registry

**Files:**
- Create: `internal/services/csvmap/presets.go`
- Test: `internal/services/csvmap/presets_test.go`

- [ ] **Step 1: Write the failing test** — Create `internal/services/csvmap/presets_test.go`:

```go
package csvmap

import "testing"

func TestPresets_IncludesCompletionator(t *testing.T) {
	var found *Preset
	for i := range presetList {
		if presetList[i].Slug == "completionator" {
			found = &presetList[i]
		}
	}
	if found == nil {
		t.Fatal("expected a 'completionator' preset in the registry")
	}
	if found.DisplayName != "Completionator" {
		t.Errorf("display name = %q, want Completionator", found.DisplayName)
	}
	if found.Config.Columns.Title != "Name" {
		t.Errorf("preset Config not wired to Completionator() (title col = %q)", found.Config.Columns.Title)
	}
}

func TestPresetBySlug(t *testing.T) {
	cfg, ok := PresetBySlug("completionator")
	if !ok {
		t.Fatal("PresetBySlug(completionator) ok = false, want true")
	}
	if cfg.Columns.Title != "Name" {
		t.Errorf("returned Config is not Completionator (title col = %q)", cfg.Columns.Title)
	}
	if _, ok := PresetBySlug("nope"); ok {
		t.Error("PresetBySlug(nope) ok = true, want false")
	}
	if _, ok := PresetBySlug(""); ok {
		t.Error("PresetBySlug(empty) ok = true, want false")
	}
}

func TestPresets_ReturnsCopy(t *testing.T) {
	got := Presets()
	if len(got) != len(presetList) {
		t.Fatalf("Presets() len = %d, want %d", len(got), len(presetList))
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/services/csvmap/ -run 'TestPreset' -v` (expect: undefined `presetList`/`PresetBySlug`/`Presets`).

- [ ] **Step 3: Write the implementation** — Create `internal/services/csvmap/presets.go`:

```go
package csvmap

// Preset is a named CSV source whose mapping is baked in as a Config plus a
// header signature. Manual format selection (this issue) lists these; auto-detect
// (#1015) will match an upload's header against each Config's Signature.
type Preset struct {
	Slug        string // stable id used on the wire (e.g. "completionator")
	DisplayName string // shown in the import dialog
	Config      Config
}

// presetList is the registry of known CSV source presets.
var presetList = []Preset{
	{Slug: "completionator", DisplayName: "Completionator", Config: Completionator()},
}

// Presets returns the registered presets (a copy; callers must not mutate the registry).
func Presets() []Preset {
	out := make([]Preset, len(presetList))
	copy(out, presetList)
	return out
}

// PresetBySlug returns the Config for a preset slug. ok is false for an unknown
// or empty slug.
func PresetBySlug(slug string) (Config, bool) {
	for i := range presetList {
		if presetList[i].Slug == slug {
			return presetList[i].Config, true
		}
	}
	return Config{}, false
}
```

- [ ] **Step 4: Run test to verify it passes** — `go test ./internal/services/csvmap/ -run 'TestPreset' -v` (expect PASS).

- [ ] **Step 5: Commit:**

```bash
git add internal/services/csvmap/presets.go internal/services/csvmap/presets_test.go
git commit -m "feat: csvmap preset registry (Completionator)"
```

---

## Task 7: Backend — `format` on import, presets on inspect

**Files:**
- Modify: `internal/api/import_csv.go`
- Test: `internal/api/import_csv_test.go`

- [ ] **Step 1: Write the failing tests** — append to `internal/api/import_csv_test.go`. The `completionatorCSV` fixture is a small valid Completionator export (24 columns, fully quote-wrapped):

```go
const completionatorCSV = `"Name","Edition","Platform","Format","Region","Now Playing","Backlogged","Ownership Status","Progress Status","Est. Value","Amt. Paid","Tags","Box/Case","Cart/Disc","Manual","Extras","Acquisition Type","Acquisition Source","Acquisition Date","Rating","Initial Release Date","Item Release Date","Added On","Genre"
"A Hat in Time","","PC / Windows","Digital (Steam)","EU","No","Yes","Owned","Incomplete","","","","","","","","Purchase","","","10","10/5/2017","","1/17/2022","Platformer"
`

func TestImportCSVInspect_ReturnsPresets(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-presets")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "x.csv", []byte("Name,Status\nA,Beaten\n"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Presets []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"presets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, p := range resp.Presets {
		if p.Slug == "completionator" && p.Name == "Completionator" {
			found = true
		}
	}
	if !found {
		t.Fatalf("presets = %+v, want one {completionator, Completionator}", resp.Presets)
	}
}

func TestImportCSV_PresetFormat_UsesServerConfig(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-preset-import")

	// Post the file with format=completionator and NO mapping field.
	rec := postCSVImportFormat(t, e, "completionator.csv", []byte(completionatorCSV), "completionator", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp ImportJobCreatedFields
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
}

func TestImportCSV_UnknownFormat_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-bad-format")

	rec := postCSVImportFormat(t, e, "x.csv", []byte(completionatorCSV), "bogus", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}

func TestImportCSV_PresetFormat_SignatureMismatch_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-sig-mismatch")

	// A non-Completionator header with format=completionator must be rejected.
	rec := postCSVImportFormat(t, e, "x.csv", []byte("Title,Console\nCeleste,PC\n"), "completionator", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}
```

Add this multipart helper (mirrors `postCSVImport` but sends a `format` field instead of `mapping`) near the existing `postCSVImport` helper in the test file:

```go
// postCSVImportFormat posts a CSV with a "format" form field (preset path), no mapping.
func postCSVImportFormat(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, filename string, fileContent []byte, format, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("createFormFile: %v", err)
	}
	if _, err := fw.Write(fileContent); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.WriteField("format", format); err != nil {
		t.Fatalf("write format: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// ImportJobCreatedFields is the subset of the import response the preset test asserts.
type ImportJobCreatedFields struct {
	JobID string `json:"job_id"`
}
```

(If `bytes` / `mime/multipart` / `net/http/httptest` are not yet imported in the test file, add them — the existing `postCSVImport` helper already uses them, so they should be present.)

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/api/ -run 'TestImportCSV_PresetFormat|TestImportCSV_UnknownFormat|TestImportCSVInspect_ReturnsPresets' -v` (expect FAIL: inspect has no `presets`; the handler ignores `format`).

- [ ] **Step 3a: Add presets to the inspect response.** In `internal/api/import_csv.go`, add a preset DTO and field. Add near `csvInspectResponse`:

```go
// csvPresetInfo is one selectable known CSV format for the import dialog dropdown.
type csvPresetInfo struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}
```

Add `Presets []csvPresetInfo `json:"presets"`` to the `csvInspectResponse` struct. Then, in `HandleImportCSVInspect`, build the list and include it in the returned `csvInspectResponse`. Just before the final `return c.JSON(http.StatusOK, csvInspectResponse{...})`, add:

```go
	presets := make([]csvPresetInfo, 0)
	for _, p := range csvmap.Presets() {
		presets = append(presets, csvPresetInfo{Slug: p.Slug, Name: p.DisplayName})
	}
```

and add `Presets: presets,` to the `csvInspectResponse{...}` literal.

- [ ] **Step 3b: Handle `format` on import.** In `HandleImportCSV` (in the same file), replace the block that currently reads the `mapping` field and builds `cfg` (from `mappingJSON := c.Request().FormValue("mapping")` through the `cfg, err := buildCSVConfig(mapping)` / its error check) with:

```go
	format := strings.TrimSpace(c.Request().FormValue("format"))
	var cfg csvmap.Config
	if format != "" && format != "generic" {
		preset, ok := csvmap.PresetBySlug(format)
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown CSV format: "+format)
		}
		cfg = preset
	} else {
		mappingJSON := c.Request().FormValue("mapping")
		if mappingJSON == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "missing mapping field")
		}
		var mapping csvMapping
		if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid mapping JSON")
		}
		built, err := buildCSVConfig(mapping)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cfg = built
	}
```

Then, where the handler currently calls `csvmap.Parse(body, cfg)` and checks the error, make the signature-mismatch case a clear message. Replace the existing parse-error check:

```go
	games, err := csvmap.Parse(body, cfg)
	if err != nil {
		if errors.Is(err, importmodel.ErrInvalidSignature) {
			return echo.NewHTTPError(http.StatusBadRequest, "this file does not match the selected format")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
```

Add imports to `internal/api/import_csv.go`: `"errors"` and `"github.com/drzero42/nexorious/internal/services/importmodel"` (keep the existing imports; `strings`, `json`, `csvmap` are already imported).

- [ ] **Step 4: Run the tests** — `go test ./internal/api/ -run 'TestImportCSV' -v` (expect PASS: the new four + all existing CSV import/inspect tests). Requires the testcontainer.

- [ ] **Step 5: Commit:**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "feat: format selection on CSV import + presets on inspect"
```

---

## Task 8: Frontend data layer (types, api, hook)

**Files:**
- Modify: `ui/frontend/src/types/import-export.ts`
- Modify: `ui/frontend/src/api/import-export.ts`
- Modify: `ui/frontend/src/hooks/use-import-export.ts`

- [ ] **Step 1: Extend the inspect type.** In `ui/frontend/src/types/import-export.ts`, add a preset type and field. Add above `CsvInspectResponse`:

```ts
export interface CsvPresetInfo {
  slug: string;
  name: string;
}
```

Add `presets?: CsvPresetInfo[];` to the `CsvInspectResponse` interface (after `suggested_mapping?`).

- [ ] **Step 2: Update the import API fn.** In `ui/frontend/src/api/import-export.ts`, replace the `importCsv` function with:

```ts
/**
 * Import a CSV. For the generic path, `format` is 'generic' and the user-built
 * `mapping` is sent. For a preset (e.g. 'completionator'), the slug is sent and
 * the server applies the preset Config (the mapping is ignored).
 */
export async function importCsv(
  file: File,
  format: string,
  mapping: CsvMapping,
): Promise<ImportJobCreatedResponse> {
  const extraFields =
    format === 'generic'
      ? { mapping: JSON.stringify(mapping) }
      : { format };
  return apiUploadFile<ImportJobCreatedResponse>('/import/csv', file, 'file', extraFields);
}
```

- [ ] **Step 3: Update the hook.** In `ui/frontend/src/hooks/use-import-export.ts`, replace `useImportCsv` with:

```ts
/** Import a CSV with a chosen format ('generic' + a user mapping, or a preset slug). */
export function useImportCsv() {
  const queryClient = useQueryClient();
  return useMutation<
    ImportJobCreatedResponse,
    Error,
    { file: File; format: string; mapping: CsvMapping }
  >({
    mutationFn: ({ file, format, mapping }) => importExportApi.importCsv(file, format, mapping),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}
```

- [ ] **Step 4: Typecheck** — from `ui/frontend/`: `npm run check` (TypeScript will flag the now-stale `importCsv` call in `import-export.tsx` and the dialog `onImport` shape — those are fixed in Task 9; for now just confirm the three files in this task themselves compile by reading the errors, which should all point at `routes/_authenticated/import-export.tsx` and the dialog, not at the three files edited here). Do **not** commit yet if `npm run check` errors only in the Task 9 files — proceed to Task 9, then commit Tasks 8+9 together. If it errors inside the three files edited here, fix them.

- [ ] **Step 5:** Defer the commit to Task 9 (these layers don't typecheck until the dialog/route are updated). No commit in Task 8.

---

## Task 9: Frontend UI (dialog selector + route wiring + tests)

**Files:**
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.tsx`
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx`

- [ ] **Step 1: Update the dialog.** In `csv-mapping-dialog.tsx`:

(a) Change the `onImport` prop type on BOTH `CsvMappingDialogProps` and `CsvMappingFormProps` from `(mapping: CsvMapping) => void` to:

```ts
  onImport: (result: { format: string; mapping: CsvMapping }) => void;
```

(b) In `CsvMappingForm`, add format state at the top (after the existing `const [mapping, setMapping] = ...`):

```tsx
  const [format, setFormat] = useState('generic');
  const isPreset = format !== 'generic';
  const presets = inspect.presets ?? [];
```

(c) Add the Format selector as the first child inside the `<div className="space-y-5">`, before the `1 · Map columns` section:

```tsx
        <div className="grid grid-cols-2 items-center gap-2">
          <Label htmlFor="csv-format">Format</Label>
          <Select value={format} onValueChange={setFormat}>
            <SelectTrigger id="csv-format" aria-label="Format">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="generic">Generic CSV</SelectItem>
              {presets.map((p) => (
                <SelectItem key={p.slug} value={p.slug}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {isPreset && (
          <p className="text-sm text-muted-foreground">
            Columns, play-status, platforms, ratings and dates are mapped automatically by the{' '}
            {presets.find((p) => p.slug === format)?.name ?? format} preset.
          </p>
        )}
```

(d) Wrap the existing `1 · Map columns` section AND the `2 · Map status values` section so they render only for the generic path: change the `<section ...>` for "1 · Map columns" to `{!isPreset && (<section ...>...</section>)}`, and the existing `{mapping.status.column && ...}` status-values block to also require `!isPreset` (i.e. `{!isPreset && mapping.status.column && (...)}`).

(e) Update the Import button: it currently has `onClick={() => onImport(mapping)}` and `disabled={!mapping.columns.title || isImporting}`. Change to:

```tsx
        <Button
          onClick={() => onImport({ format, mapping })}
          disabled={(!isPreset && !mapping.columns.title) || isImporting}
        >
```

(f) Change the `DialogTitle` from `Import CSV — map your columns` to `Import CSV` (the mapping is now conditional).

- [ ] **Step 2: Update the route wiring.** In `ui/frontend/src/routes/_authenticated/import-export.tsx`, change `handleCsvImport` (around line 329) from taking `mapping: CsvMapping` to:

```tsx
  const handleCsvImport = async (result: { format: string; mapping: CsvMapping }) => {
    if (!csvFile) return;
    try {
      const created = await importCsv({ file: csvFile, format: result.format, mapping: result.mapping });
      toast.success(`Import started: ${created.message}`);
      setCsvDialogOpen(false);
      setDismissedJobId(null);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Import failed');
    }
  };
```

(The local variable was named `result` before for the response; it is renamed to `created` here to avoid shadowing the new `result` param.)

- [ ] **Step 3: Update the existing dialog test + add a preset test.** In `csv-mapping-dialog.test.tsx`:

(a) The existing `'imports with the assembled mapping...'` test asserts `onImport` was called with the bare mapping object. Update that assertion to the new shape — wrap the existing expected object as `mapping` under `{ format: 'generic', mapping: {...} }`:

```ts
    expect(onImport).toHaveBeenCalledWith({
      format: 'generic',
      mapping: {
        columns: {
          title: 'Name',
          igdb_id: '',
          platform: '',
          storefront: '',
          rating: '',
          notes: '',
          acquired_date: '',
          hours_played: '',
          tags: '',
          loved: '',
        },
        status: { column: 'Status', value_map: { Beaten: 'not_started', Playing: 'not_started' } },
        rating_scale: 5,
        merge_by_title: true,
      },
    });
```

(b) Add a new test for the preset path. It needs an inspect fixture carrying a preset:

```ts
  it('hides the mapping form and imports with the preset slug when a format is chosen', async () => {
    const user = userEvent.setup();
    const onImport = vi.fn();
    render(
      <CsvMappingDialog
        open
        onOpenChange={vi.fn()}
        inspect={{ ...inspect, presets: [{ slug: 'completionator', name: 'Completionator' }] }}
        isImporting={false}
        onImport={onImport}
      />,
    );

    await user.click(screen.getByRole('combobox', { name: 'Format' }));
    await user.click(screen.getByRole('option', { name: 'Completionator' }));

    // The manual mapping section is gone; Import is enabled without a title.
    expect(screen.queryByText('1 · Map columns')).not.toBeInTheDocument();
    const importBtn = screen.getByRole('button', { name: /import 3 games/i });
    expect(importBtn).toBeEnabled();
    await user.click(importBtn);

    expect(onImport).toHaveBeenCalledTimes(1);
    expect(onImport.mock.calls[0][0].format).toBe('completionator');
  });
```

- [ ] **Step 4: Verify** — from `ui/frontend/`:
  - `npm run check` (typecheck + lint) — expect clean.
  - `npm run test csv-mapping-dialog` — expect all dialog tests pass (updated + new).
  - `npm run knip` — expect no new findings (the `CsvPresetInfo` type is used by the dialog/types).

- [ ] **Step 5: Build to regenerate any route artifacts and confirm** — from `ui/frontend/`: `npm run build` (no route files were added, so `routeTree.gen.ts` should not change; if it does, include it).

- [ ] **Step 6: Commit Tasks 8 + 9 together** (the frontend only typechecks as a unit):

```bash
git add ui/frontend/src/types/import-export.ts ui/frontend/src/api/import-export.ts ui/frontend/src/hooks/use-import-export.ts ui/frontend/src/components/import/csv-mapping-dialog.tsx ui/frontend/src/components/import/csv-mapping-dialog.test.tsx ui/frontend/src/routes/_authenticated/import-export.tsx
git commit -m "feat: choose CSV format (Generic/Completionator) in the import dialog"
```

---

## Added-scope final verification

- [ ] `go test ./internal/services/csvmap/ ./internal/api/` — PASS.
- [ ] From `ui/frontend/`: `npm run check && npm run knip && npm run test` — PASS.
- [ ] Manual sanity (optional): the Format dropdown shows `Generic CSV` + `Completionator`; choosing Completionator collapses the mapping form.

## Added-scope self-review notes
- **Crux honored:** a preset import sends `format=<slug>` and the server runs `csvmap.Parse(body, PresetBySlug(slug))` — the slug maps are applied server-side; the flat DTO is never used for a preset.
- **Signature enforced for manual picks:** `Parse` rejects a non-matching header (`ErrInvalidSignature` → 400 "this file does not match the selected format").
- **Registry is the #1015 seam:** `csvmap.Presets()` / `PresetBySlug` are exactly what auto-detect will consume.
- **Type consistency:** `onImport({format, mapping})` is the shape across the dialog prop, the route handler, and `useImportCsv`'s `{file, format, mapping}`.
