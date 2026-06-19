# CSV import auto-mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `POST /api/import/csv` a server-side auto mode that, when no `format` and no `mapping` are supplied, detects a preset by header signature (else builds a data-refined guessed mapping), enqueues the import in one call, and reports what it chose — then wire `nexctl import csv <file>` with no flags to use it.

**Architecture:** Extract the inspect handler's single-pass scan into a shared `scanCSV`, build a `resolveCSVAuto` detect-or-guess helper on top of it (reused by inspect and the new auto branch so the two can't drift), add an auto branch + transparent `auto` envelope to `HandleImportCSV`, mirror the envelope in `cliclient.ImportResult`, and replace `nexctl`'s no-flag "title required" error with an auto call that prints the server's chosen mapping.

**Tech Stack:** Go 1.26, Echo v5, Bun, `internal/services/csvmap`, cobra (`nexctl`), stdlib `testing` + testcontainers (server tests) / `httptest` (client + CLI tests).

## Global Constraints

- Auto triggers **only** when `format` is empty **and** `mapping` is empty/absent. `format=generic` with no mapping keeps returning 400 "missing mapping field"; the preset path and manual-mapping path are unchanged.
- No new dependency; `nexctl`/`cliclient` must not import `internal/services/csvmap` (REST-only boundary — mirror shapes locally).
- No migration, no web UI change.
- Go: errors returned not panicked; `errcheck` runs with `check-blank`; `gosec` enabled. Echo handler signature is `func (h *Handler) X(c *echo.Context) error`.
- Reuse the existing test helpers (`postMultipartFile`, `postCSVImport`, `postCSVImportFormat`, `setupTagUser`, `newTestEchoConfiguredIGDB`, `testIGDBClient(true)`, `completionatorCSV` in `internal/api`; `runImport`, `writeTempFile`, `readMultipartFile` in `cmd/nexctl`). Each server test starts with `truncateAllTables(t)`.

---

### Task 1: Extract `scanCSV` from the inspect handler (pure refactor)

Factor the inspect handler's column scan + guess refinement into a reusable function. No behaviour change — the existing inspect tests are the guard.

**Files:**
- Modify: `internal/api/import_csv.go` (`HandleImportCSVInspect`, lines ~159–243)
- Test: `internal/api/import_csv_test.go` (existing inspect tests — no new test)

**Interfaces:**
- Produces: `func scanCSV(records [][]string) (cols []csvColumnInfo, suggested csvmap.SuggestedMapping)` — `records[0]` is the header; returns capped per-column distinct values and a data-refined `SuggestedMapping` (rating scale from observed max, status value-map from the status column's distinct values).

- [ ] **Step 1: Add `scanCSV`**

Add this function to `internal/api/import_csv.go` (e.g. just above `HandleImportCSVInspect`). It is the current scan body, lifted verbatim:

```go
// scanCSV computes, in one pass over the data rows, each column's capped set of
// distinct non-empty values and a data-refined suggested mapping (rating scale
// from the observed max; status value-map from the status column's distinct
// values). records[0] is the header. Shared by the inspect handler and the
// auto import path so the inspect-time and import-time guesses cannot drift.
func scanCSV(records [][]string) (cols []csvColumnInfo, suggested csvmap.SuggestedMapping) {
	header := records[0]
	suggested = csvmap.GuessColumns(header)
	ratingIdx := -1
	if suggested.Columns.Rating != "" {
		for i, name := range header {
			if name == suggested.Columns.Rating {
				ratingIdx = i
				break
			}
		}
	}

	cols = make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	var ratingMax float64
	for _, rec := range records[1:] {
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

	if suggested.Columns.Rating != "" {
		suggested.RatingScale = csvmap.GuessRatingScale(ratingMax)
	}
	if suggested.Status.Column != "" {
		for _, col := range cols {
			if col.Name == suggested.Status.Column {
				suggested.Status.ValueMap = csvmap.GuessStatusValueMap(col.DistinctValues)
				break
			}
		}
	}
	return cols, suggested
}
```

- [ ] **Step 2: Rewrite `HandleImportCSVInspect` to call `scanCSV`**

Replace the body from after the `header := records[0]` line through the end of the handler. The new body (keeping the unauthenticated/igdbGuard/readUploadFile/ReadRecords/empty-check preamble exactly as-is):

```go
	records, err := csvmap.ReadRecords(body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
	if len(records) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
	}

	cols, suggested := scanCSV(records)

	presets := make([]csvPresetInfo, 0)
	for _, p := range csvmap.Presets() {
		presets = append(presets, csvPresetInfo{Slug: p.Slug, Name: p.DisplayName})
	}

	return c.JSON(http.StatusOK, csvInspectResponse{
		Headers:          records[0],
		RowCount:         len(records) - 1,
		Columns:          cols,
		SuggestedMapping: suggested,
		Presets:          presets,
		Detected:         detectPreset(records[0]),
	})
```

(`len(records)-1` equals the old per-row `rowCount++` count over `records[1:]`.)

- [ ] **Step 3: Build and run the inspect regression tests**

Run: `go test ./internal/api/... -run 'TestImportCSVInspect' -v`
Expected: PASS (all `TestImportCSVInspect_*` green — `scanCSV` preserved behaviour).

- [ ] **Step 4: Commit**

```bash
git add internal/api/import_csv.go
git commit -m "refactor(api): extract scanCSV from CSV inspect handler"
```

---

### Task 2: Server auto path on `POST /api/import/csv`

Add `resolveCSVAuto` + `suggestedToCSVMapping` + the no-title sentinel, then branch `HandleImportCSV` into auto and emit the transparent `auto` envelope.

**Files:**
- Modify: `internal/api/import_csv.go` (`HandleImportCSV`, add helpers)
- Test: `internal/api/import_csv_test.go`

**Interfaces:**
- Consumes: `scanCSV` (Task 1), `detectPreset`, `buildCSVConfig`, `csvmap.PresetBySlug`, `csvmap.ReadRecords` (all existing).
- Produces:
  - `func resolveCSVAuto(records [][]string) (csvAutoResolution, error)`
  - `type csvAutoResolution struct { Config csvmap.Config; Detected *csvPresetInfo; Mapping *csvmap.SuggestedMapping }`
  - `var errCSVAutoNoTitle error`
  - Response gains optional `"auto"` object: `{ "mode": "preset"|"guessed", "preset": {slug,name}?, "mapping": SuggestedMapping? }`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/import_csv_test.go`:

```go
func TestImportCSV_Auto_DetectsPreset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-preset")

	// completionatorCSV header matches the completionator signature; append a data row.
	data := []byte(completionatorCSV + "\"Celeste\",,,,,Yes,No,Owned,Beaten,,,,,,,,,,2024-01-01,,,,2024-01-02,")
	rec := postMultipartFile(t, e, "/api/import/csv", "c.csv", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		JobID string `json:"job_id"`
		Auto  *struct {
			Mode   string `json:"mode"`
			Preset *struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			} `json:"preset"`
		} `json:"auto"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
	if resp.Auto == nil || resp.Auto.Mode != "preset" || resp.Auto.Preset == nil || resp.Auto.Preset.Slug != "completionator" {
		t.Fatalf("auto = %+v, want preset completionator", resp.Auto)
	}
}

func TestImportCSV_Auto_GuessesMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-guess")

	data := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\n")
	rec := postMultipartFile(t, e, "/api/import/csv", "g.csv", data, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		JobID string `json:"job_id"`
		Auto  *struct {
			Mode    string `json:"mode"`
			Mapping *struct {
				Columns struct {
					Title string `json:"title"`
				} `json:"columns"`
			} `json:"mapping"`
		} `json:"auto"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Error("expected a job_id")
	}
	if resp.Auto == nil || resp.Auto.Mode != "guessed" || resp.Auto.Mapping == nil || resp.Auto.Mapping.Columns.Title != "Name" {
		t.Fatalf("auto = %+v, want guessed title=Name", resp.Auto)
	}
}

func TestImportCSV_Auto_NoTitle_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-auto-notitle")

	// "Console" guesses to platform, no title-like header, no preset signature.
	data := []byte("Console\nPC\nSwitch\n")
	rec := postMultipartFile(t, e, "/api/import/csv", "n.csv", data, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "no title column") {
		t.Errorf("body = %q, want it to mention 'no title column'", rec.Body.String())
	}
}

func TestImportCSV_GenericNoMapping_StillMissingMapping_400(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-generic-nomap")

	rec := postCSVImportFormat(t, e, "x.csv", []byte("Name,Status\nCeleste,Beaten\n"), "generic", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "missing mapping field") {
		t.Errorf("body = %q, want 'missing mapping field'", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run 'TestImportCSV_Auto|TestImportCSV_GenericNoMapping' -v`
Expected: FAIL — `TestImportCSV_Auto_DetectsPreset`/`_GuessesMapping` get the old 400 "missing mapping field" (no auto branch yet); `_NoTitle_400` may pass for the wrong reason; `_GenericNoMapping` passes (existing behaviour).

- [ ] **Step 3: Add the auto helpers**

Add to `internal/api/import_csv.go`:

```go
// errCSVAutoNoTitle is returned by resolveCSVAuto when neither a preset nor the
// header guess can identify a title column, so there is nothing to import.
var errCSVAutoNoTitle = errors.New("could not auto-detect a column mapping: no title column found. Inspect the file and choose a preset or map the columns explicitly")

// csvAutoResolution is the outcome of the import-time detect-or-guess policy:
// either a preset matched by header signature, or a data-refined guessed
// mapping. Config is what csvmap.Parse runs with.
type csvAutoResolution struct {
	Config   csvmap.Config
	Detected *csvPresetInfo           // non-nil when a preset signature matched
	Mapping  *csvmap.SuggestedMapping // non-nil when guessed (no preset matched)
}

// resolveCSVAuto applies the detect-or-guess policy to a parsed CSV:
// a matching preset wins; otherwise the data-refined GuessColumns mapping is
// used, or errCSVAutoNoTitle if that yields no title column.
func resolveCSVAuto(records [][]string) (csvAutoResolution, error) {
	header := records[0]
	if d := detectPreset(header); d != nil {
		cfg, ok := csvmap.PresetBySlug(d.Slug)
		if !ok {
			return csvAutoResolution{}, fmt.Errorf("detected preset %q is not registered", d.Slug)
		}
		return csvAutoResolution{Config: cfg, Detected: d}, nil
	}
	_, suggested := scanCSV(records)
	if strings.TrimSpace(suggested.Columns.Title) == "" {
		return csvAutoResolution{}, errCSVAutoNoTitle
	}
	cfg, err := buildCSVConfig(suggestedToCSVMapping(suggested))
	if err != nil {
		return csvAutoResolution{}, err
	}
	return csvAutoResolution{Config: cfg, Mapping: &suggested}, nil
}

// suggestedToCSVMapping copies a csvmap.SuggestedMapping into the flat csvMapping
// shape buildCSVConfig consumes (the two share an identical JSON layout), so the
// guessed mapping flows through the same translation as a user-submitted one.
func suggestedToCSVMapping(s csvmap.SuggestedMapping) csvMapping {
	var m csvMapping
	m.Columns.Title = s.Columns.Title
	m.Columns.IGDBID = s.Columns.IGDBID
	m.Columns.Platform = s.Columns.Platform
	m.Columns.Storefront = s.Columns.Storefront
	m.Columns.Rating = s.Columns.Rating
	m.Columns.Notes = s.Columns.Notes
	m.Columns.AcquiredDate = s.Columns.AcquiredDate
	m.Columns.HoursPlayed = s.Columns.HoursPlayed
	m.Columns.Tags = s.Columns.Tags
	m.Columns.Loved = s.Columns.Loved
	m.Status.Column = s.Status.Column
	m.Status.ValueMap = s.Status.ValueMap
	m.RatingScale = s.RatingScale
	m.MergeByTitle = s.MergeByTitle
	return m
}
```

- [ ] **Step 4: Branch `HandleImportCSV` into auto + emit the envelope**

Replace the config-selection block (current lines ~261–287, from `format := ...` through the closing brace of the `else`) and the final response (current lines ~304–310) so the handler reads:

```go
	format := strings.TrimSpace(c.Request().FormValue("format"))
	mappingJSON := c.Request().FormValue("mapping")
	var cfg csvmap.Config
	var autoRes *csvAutoResolution
	switch {
	case format != "" && format != "generic":
		// Preset path: server-side Config wins; any "mapping" field is ignored.
		preset, ok := csvmap.PresetBySlug(format)
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown CSV format: "+format)
		}
		cfg = preset
	case format == "" && strings.TrimSpace(mappingJSON) == "":
		// Auto path: no format and no mapping -> detect a preset, else guess.
		records, err := csvmap.ReadRecords(body)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
		}
		if len(records) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
		}
		res, err := resolveCSVAuto(records)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cfg = res.Config
		autoRes = &res
	default:
		// Manual path: a mapping is required (covers format=generic + no mapping).
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

Then the existing `csvmap.Parse(body, cfg)` / empty-check / `enqueueImportJob` stay as-is, and the response becomes:

```go
	resp := map[string]any{
		"job_id":      jobID,
		"source":      models.JobSourceCSV,
		"status":      models.JobStatusProcessing,
		"message":     fmt.Sprintf("CSV import job created. Matching %d games.", total),
		"total_items": total,
	}
	if autoRes != nil {
		auto := map[string]any{}
		if autoRes.Detected != nil {
			auto["mode"] = "preset"
			auto["preset"] = autoRes.Detected
		} else {
			auto["mode"] = "guessed"
			auto["mapping"] = autoRes.Mapping
		}
		resp["auto"] = auto
	}
	return c.JSON(http.StatusOK, resp)
```

(`errors` and `strconv` are already imported in this file.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/... -run 'TestImportCSV' -v`
Expected: PASS — the four new tests plus all existing `TestImportCSV*` (preset, manual, conflict, no-data, missing-title) green.

- [ ] **Step 6: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "feat(api): auto-detect CSV preset/mapping when none is specified"
```

---

### Task 3: `cliclient` auto envelope

Mirror the response's `auto` object on `ImportResult` so clients can read what the server chose. `ImportCSV("", nil)` already issues an auto request, so no method change.

**Files:**
- Modify: `internal/cliclient/import.go`
- Test: `internal/cliclient/import_test.go`

**Interfaces:**
- Produces: `type CSVAutoResolution struct { Mode string; Preset *CSVPreset; Mapping *CSVSuggestedMapping }` and `ImportResult.Auto *CSVAutoResolution`.
- Consumes: existing `CSVPreset`, `CSVSuggestedMapping`.

- [ ] **Step 1: Write the failing test**

Add to `internal/cliclient/import_test.go`:

```go
func TestImportCSV_DecodesAutoEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "job-1",
			"source":      "csv",
			"status":      "processing",
			"total_items": 3,
			"auto": map[string]any{
				"mode":   "preset",
				"preset": map[string]any{"slug": "grouvee", "name": "Grouvee"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ImportCSV("k", "g.csv", []byte("Name\nCeleste\n"), "", nil)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if res.Auto == nil || res.Auto.Mode != "preset" || res.Auto.Preset == nil || res.Auto.Preset.Slug != "grouvee" {
		t.Fatalf("Auto = %+v, want preset grouvee", res.Auto)
	}
}
```

(If `internal/cliclient/import_test.go` lacks the `net/http`/`net/http/httptest`/`encoding/json` imports, add them; check the file first.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cliclient/... -run TestImportCSV_DecodesAutoEnvelope -v`
Expected: FAIL to compile — `res.Auto` undefined.

- [ ] **Step 3: Add the types**

In `internal/cliclient/import.go`, add after the `ImportResult` struct:

```go
// CSVAutoResolution describes how POST /api/import/csv's auto mode mapped the
// file: a preset matched by signature, or a guessed column mapping.
type CSVAutoResolution struct {
	Mode    string               `json:"mode"`              // "preset" | "guessed"
	Preset  *CSVPreset           `json:"preset,omitempty"`  // set when mode=="preset"
	Mapping *CSVSuggestedMapping `json:"mapping,omitempty"` // set when mode=="guessed"
}
```

And add the field to `ImportResult`:

```go
	// Auto is set only by POST /api/import/csv's auto mode (no format, no
	// mapping); nil for preset/manual imports and the other import endpoints.
	Auto *CSVAutoResolution `json:"auto,omitempty"`
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/cliclient/... -run TestImportCSV_DecodesAutoEnvelope -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/import.go internal/cliclient/import_test.go
git commit -m "feat(cliclient): decode CSV import auto-resolution envelope"
```

---

### Task 4: `nexctl import csv` no-flag auto path

Make the no-flag invocation delegate to auto and print what the server chose, instead of erroring that a title column is required.

**Files:**
- Modify: `cmd/nexctl/import_csv.go`
- Test: `cmd/nexctl/import_csv_test.go`

**Interfaces:**
- Consumes: `cliclient.ImportCSV`, `cliclient.ImportResult.Auto`, `cliclient.CSVAutoResolution` (Task 3); existing `printImportResult`, `flagBool`, `anyManual`.
- Produces: `func printCSVAutoResolution(out io.Writer, a *cliclient.CSVAutoResolution)`.

- [ ] **Step 1: Write the failing tests**

Add to `cmd/nexctl/import_csv_test.go`:

```go
func TestImportCSV_AutoPreset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		_ = readMultipartFile(t, r)
		if r.FormValue("format") != "" || r.FormValue("mapping") != "" {
			t.Errorf("auto request should send no format/mapping; got format=%q mapping=%q", r.FormValue("format"), r.FormValue("mapping"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j1", "total_items": 2,
			"auto": map[string]any{"mode": "preset", "preset": map[string]any{"slug": "grouvee", "name": "Grouvee"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name,Status\nPortal,Beat\n")
	out, err := runImport(t, srv.URL, "import", "csv", file)
	if err != nil {
		t.Fatalf("import csv: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Auto-detected preset: grouvee") {
		t.Errorf("output missing detected preset line: %q", out)
	}
}

func TestImportCSV_AutoGuessed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/import/csv", func(w http.ResponseWriter, r *http.Request) {
		_ = readMultipartFile(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": "j2", "total_items": 1,
			"auto": map[string]any{
				"mode":    "guessed",
				"mapping": map[string]any{"columns": map[string]any{"title": "Name"}},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "games.csv", "Name\nPortal\n")
	out, err := runImport(t, srv.URL, "import", "csv", file)
	if err != nil {
		t.Fatalf("import csv: %v\n%s", err, out)
	}
	if !strings.Contains(out, "No preset matched; guessed column mapping:") || !strings.Contains(out, "title=Name") {
		t.Errorf("output missing guessed-mapping lines: %q", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/nexctl/... -run 'TestImportCSV_Auto' -v`
Expected: FAIL — the no-flag path still errors `a --title-col is required` (no auto branch yet).

- [ ] **Step 3: Add the auto branch and printer**

In `cmd/nexctl/import_csv.go`, add `"io"` to the imports. Insert the auto branch between the preset path (after its `return printImportResult(...)` block) and the manual-mapping path (`if strings.TrimSpace(titleCol) == "" {`):

```go
			// Auto path: no preset and no manual flags -> let the server detect
			// a preset or guess the mapping in one call.
			if !anyManual {
				res, err := c.ImportCSV(p.Key, filename, data, "", nil)
				if err != nil {
					return fmt.Errorf("import CSV failed: %w", err)
				}
				if !flagBool(cmd, "json") {
					printCSVAutoResolution(out, res.Auto)
				}
				return printImportResult(cmd, res)
			}
```

Update the manual-path error hint:

```go
			if strings.TrimSpace(titleCol) == "" {
				return fmt.Errorf("a --title-col is required (or use --preset, or run with no flags to auto-detect)")
			}
```

Add the printer (e.g. above `newImportCSVCmd`):

```go
// printCSVAutoResolution reports how the server's auto mode mapped the CSV: the
// detected preset, or the guessed column mapping (so an auto import isn't
// silent about being a guess). A nil envelope prints nothing.
func printCSVAutoResolution(out io.Writer, a *cliclient.CSVAutoResolution) {
	if a == nil {
		return
	}
	switch a.Mode {
	case "preset":
		if a.Preset != nil {
			fmt.Fprintf(out, "Auto-detected preset: %s (%s)\n", a.Preset.Slug, a.Preset.Name)
		}
	case "guessed":
		fmt.Fprintln(out, "No preset matched; guessed column mapping:")
		if a.Mapping != nil {
			m := a.Mapping
			pairs := []struct{ field, val string }{
				{"title", m.Columns.Title}, {"igdb_id", m.Columns.IGDBID},
				{"platform", m.Columns.Platform}, {"storefront", m.Columns.Storefront},
				{"rating", m.Columns.Rating}, {"notes", m.Columns.Notes},
				{"acquired_date", m.Columns.AcquiredDate}, {"hours_played", m.Columns.HoursPlayed},
				{"tags", m.Columns.Tags}, {"loved", m.Columns.Loved},
				{"status", m.Status.Column},
			}
			for _, pr := range pairs {
				if pr.val != "" {
					fmt.Fprintf(out, "  %s=%s\n", pr.field, pr.val)
				}
			}
		}
		fmt.Fprintln(out, "Review the import job before applying.")
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/nexctl/... -run 'TestImportCSV' -v`
Expected: PASS — both new auto tests plus the existing `--inspect`/preset/manual tests green.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/import_csv.go cmd/nexctl/import_csv_test.go
git commit -m "feat(nexctl): auto-detect CSV mapping on flag-less import csv"
```

---

### Task 5: Docs + dead-code reconciliation

**Files:**
- Possibly modify: `docs/import-export-format.md` (if it documents the `POST /api/import/csv` contract)
- Modify: `CLAUDE.md` (the `import csv` description mentions the no-flag error)

- [ ] **Step 1: Check whether the CSV import contract is documented**

Run: `grep -rn "missing mapping\|import/csv\|--inspect\|title column" docs/ CLAUDE.md`
If `docs/import-export-format.md` describes the empty/empty 400, update it to describe auto mode. Update the CLAUDE.md `import csv` sentence so the no-flag path is described as auto-detect (preset else guessed mapping), not an error.

- [ ] **Step 2: Run deadcode (caller graph changed in `import_csv.go`)**

Run: `make deadcode`
Expected: no *new* entries attributable to this diff (the `else`→`switch` refactor and the new helpers are all reachable; `scanCSV`/`resolveCSVAuto`/`suggestedToCSVMapping`/`printCSVAutoResolution` are each called).

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs(import): document CSV auto mode"
```

---

### Task 6: Full verification + PR

- [ ] **Step 1: Targeted suites**

Run: `go test ./internal/api/... ./internal/cliclient/... ./cmd/nexctl/... -v`
Expected: PASS.

- [ ] **Step 2: Push (pre-push hook runs the full Go suite) and open the PR**

```bash
git push -u origin feat/issue-1089-csv-auto-mapping
gh pr create --title "feat: auto-detect CSV format/mapping when none is specified" \
  --label enhancement \
  --body "$(cat <<'EOF'
Adds a server-side auto mode to `POST /api/import/csv`: when no `format` and no `mapping` are supplied, the handler detects a preset by header signature, else builds a data-refined guessed mapping, enqueues the import, and reports its choice in an `auto` envelope. `nexctl import csv <file>` with no flags now delegates to auto and prints the chosen preset/mapping.

- Shared `scanCSV`/`resolveCSVAuto` so inspect-time and import-time guesses can't drift.
- Only contract change: the former empty-format/empty-mapping 400 becomes an auto import. `format=generic` with no mapping still 400s.
- Web UI unchanged.

Design: `docs/superpowers/specs/2026-06-19-issue-1089-csv-auto-mapping-design.md`.

Closes #1089
EOF
)"
```

## Self-Review

- **Spec coverage:** server auto trigger (Task 2) ✓; shared detect-or-guess helper (Tasks 1–2) ✓; transparent response (Task 2) ✓; no-title 400 (Task 2) ✓; nexctl no-flag auto + printing (Task 4) ✓; `cliclient` envelope (Task 3) ✓; `generic`+no-mapping regression (Task 2) ✓; docs (Task 5) ✓. Web UI explicitly out of scope ✓.
- **Type consistency:** `csvAutoResolution{Config, Detected *csvPresetInfo, Mapping *csvmap.SuggestedMapping}` produced in Task 2 and consumed only there; `CSVAutoResolution{Mode, Preset *CSVPreset, Mapping *CSVSuggestedMapping}` produced in Task 3, consumed in Task 4; `scanCSV(records) (cols, suggested)` produced in Task 1, consumed in Tasks 1–2. `printCSVAutoResolution` name consistent across Task 4. `ImportResult.Auto` consistent Tasks 3–4.
- **Placeholder scan:** every code step shows complete code and exact run commands; no TBD/"handle errors"/"similar to".
