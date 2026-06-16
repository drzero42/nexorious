# Auto-detect known CSV formats on import — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a user uploads a CSV to the generic import path, recognise it as a known registered preset (Completionator, Grouvee, Darkadia, Nexorious) by its header signature and pre-select that preset in the mapping dialog, so the user can confirm-and-import in one click instead of mapping columns by hand.

**Architecture:** Add server-side detection to `POST /api/import/csv/inspect`: iterate `csvmap.Presets()`, skip any preset with an empty signature (those match anything), and return the first preset whose `csvmap.MatchesSignature` passes against the uploaded header as a new `detected` field. The frontend `CsvMappingForm` initialises its existing `format` state from `inspect.detected?.slug` (falling back to `generic`), reusing the already-built collapsed preset view and the Generic-CSV override. No new endpoint, no route-component change, no migration.

**Tech Stack:** Go (Echo v5 handler, `internal/services/csvmap`), React 19 + TypeScript (Vitest + Testing Library).

---

## Background (verified against the codebase, 2026-06-16)

- `POST /api/import/csv/inspect` is `HandleImportCSVInspect` in `internal/api/import_csv.go:159`. It already returns `presets []csvPresetInfo` (the *list* of registered formats). This plan adds a `detected` field (the matched preset, or absent).
- `csvmap.Presets()` (`internal/services/csvmap/presets.go:21`) returns `[]Preset{Slug, DisplayName, Config}`. `Preset.Config.Signature` (`internal/services/csvmap/config.go:20`) is the exported `[]string` of required headers.
- `csvmap.MatchesSignature(headers, cfg)` (`internal/services/csvmap/parse.go:71`) reports whether every signature header is present (normalised). **It returns `true` for an empty/nil signature** — so the detection loop MUST skip empty-signature presets, or a future signature-less preset would falsely auto-match every upload. All 4 currently-registered presets have distinctive, non-overlapping signatures, so "first match in registry order wins" is safe.
- The frontend dialog `CsvMappingForm` (`ui/frontend/src/components/import/csv-mapping-dialog.tsx:85`) already holds `const [format, setFormat] = useState('generic')`, renders a Format `<Select>` listing `inspect.presets`, shows a collapsed "mapped automatically by the X preset" view when a preset is selected (`isPreset`), and lets the user switch back to `Generic CSV` to map by hand. The form is remounted on every dialog open (the `{open && <CsvMappingForm/>}` guard in `CsvMappingDialog`), so a `useState` initializer reading `inspect.detected` runs fresh each open — no effect needed (honours the project's "no setState in useEffect" rule).
- The route component `import-export.tsx` (`handleCsvSelect`) just passes the inspect result through to the dialog. It needs **no change**: the dialog reads `inspect.detected` itself.

---

## File Structure

- **Modify** `internal/api/import_csv.go` — add `Detected *csvPresetInfo` to `csvInspectResponse`; add a `detectPreset(header []string) *csvPresetInfo` helper; populate it in `HandleImportCSVInspect`.
- **Modify** `internal/api/import_csv_internal_test.go` — unit test for `detectPreset` (pure, no DB).
- **Modify** `internal/api/import_csv_test.go` — API tests: detected-on-inspect, no-detection-for-unknown.
- **Modify** `ui/frontend/src/types/import-export.ts` — add `detected?: CsvPresetInfo | null` to `CsvInspectResponse`.
- **Modify** `ui/frontend/src/components/import/csv-mapping-dialog.tsx` — initialise `format` from `inspect.detected?.slug`; add a muted "detected automatically" hint.
- **Modify** `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx` — test auto-selection from `inspect.detected`.

---

## Task 1: Backend — `detectPreset` helper (unit, no DB)

**Files:**
- Modify: `internal/api/import_csv.go` (add helper near `csvPresetInfo` at `import_csv.go:112`)
- Test: `internal/api/import_csv_internal_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/import_csv_internal_test.go`:

```go
func TestDetectPreset_MatchesCompletionatorSignature(t *testing.T) {
	header := []string{
		"Name", "Now Playing", "Backlogged", "Ownership Status",
		"Progress Status", "Added On",
	}
	got := detectPreset(header)
	if got == nil || got.Slug != "completionator" {
		t.Fatalf("detectPreset = %+v, want completionator", got)
	}
	if got.Name != "Completionator" {
		t.Errorf("name = %q, want Completionator", got.Name)
	}
}

func TestDetectPreset_MatchesGrouveeSignature(t *testing.T) {
	header := []string{
		"name", "shelves", "date_added_to_collection",
		"review_platform", "giantbomb_id", "igdb_id",
	}
	if got := detectPreset(header); got == nil || got.Slug != "grouvee" {
		t.Fatalf("detectPreset = %+v, want grouvee", got)
	}
}

func TestDetectPreset_NoMatchReturnsNil(t *testing.T) {
	header := []string{"Title", "Console", "Hours"}
	if got := detectPreset(header); got != nil {
		t.Fatalf("detectPreset = %+v, want nil for an unrecognised header", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestDetectPreset -v`
Expected: FAIL — `undefined: detectPreset`.

- [ ] **Step 3: Write minimal implementation**

In `internal/api/import_csv.go`, immediately after the `csvPresetInfo` struct (ends at `import_csv.go:116`), add:

```go
// detectPreset returns the first registered preset whose signature matches the
// uploaded header, or nil if none match. Presets with an empty signature are
// skipped: csvmap.MatchesSignature treats an empty signature as "matches
// anything", which is correct for the manual/generic path but must never
// auto-match here. First match in registry order wins.
func detectPreset(header []string) *csvPresetInfo {
	for _, p := range csvmap.Presets() {
		if len(p.Config.Signature) == 0 {
			continue
		}
		if csvmap.MatchesSignature(header, p.Config) {
			return &csvPresetInfo{Slug: p.Slug, Name: p.DisplayName}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestDetectPreset -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_internal_test.go
git commit -m "feat: detect registered CSV preset by header signature"
```

---

## Task 2: Backend — return `detected` from `inspect`

**Files:**
- Modify: `internal/api/import_csv.go` (`csvInspectResponse` at `import_csv.go:118`; `HandleImportCSVInspect` return at `import_csv.go:248`)
- Test: `internal/api/import_csv_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/import_csv_test.go` (the `completionatorCSV` fixture at `import_csv_test.go:353` carries the full Completionator signature; the helpers `newTestEchoConfiguredIGDB`, `testIGDBClient`, `setupTagUser`, `postMultipartFile` are already used by the sibling inspect tests):

```go
func TestImportCSVInspect_DetectsRegisteredPreset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-detect")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "c.csv",
		[]byte(completionatorCSV+"\n\"Celeste\",,,,,Yes,No,Owned,Beaten,,,,,,,,,,2024-01-01,,,,2024-01-02,"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Detected *struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"detected"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Detected == nil {
		t.Fatal("detected = nil, want the completionator preset")
	}
	if resp.Detected.Slug != "completionator" || resp.Detected.Name != "Completionator" {
		t.Fatalf("detected = %+v, want {completionator, Completionator}", resp.Detected)
	}
}

func TestImportCSVInspect_NoDetectionForUnknownHeader(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-no-detect")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "x.csv",
		[]byte("Title,Console\nCeleste,PC\n"), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Detected *struct {
			Slug string `json:"slug"`
		} `json:"detected"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Detected != nil {
		t.Fatalf("detected = %+v, want nil for an unrecognised header", resp.Detected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestImportCSVInspect_Detects -v` and `go test ./internal/api/ -run TestImportCSVInspect_NoDetection -v`
Expected: FAIL — the detect test fails because `detected` is absent from the JSON (decodes to nil).

- [ ] **Step 3: Add the field and populate it**

In `internal/api/import_csv.go`, add the field to `csvInspectResponse` (currently `import_csv.go:118-124`):

```go
type csvInspectResponse struct {
	Headers          []string                `json:"headers"`
	RowCount         int                     `json:"row_count"`
	Columns          []csvColumnInfo         `json:"columns"`
	SuggestedMapping csvmap.SuggestedMapping `json:"suggested_mapping"`
	Presets          []csvPresetInfo         `json:"presets"`
	Detected         *csvPresetInfo          `json:"detected,omitempty"`
}
```

Then in `HandleImportCSVInspect`, change the return (currently `import_csv.go:248-254`) to set `Detected`:

```go
	return c.JSON(http.StatusOK, csvInspectResponse{
		Headers:          header,
		RowCount:         rowCount,
		Columns:          cols,
		SuggestedMapping: suggested,
		Presets:          presets,
		Detected:         detectPreset(header),
	})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestImportCSVInspect -v`
Expected: PASS (the new two plus the existing `ReturnsPresets`/`SuggestsMapping` etc., which use header `Name,Status` that matches no signature).

- [ ] **Step 5: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "feat: return detected CSV preset from inspect endpoint"
```

---

## Task 3: Frontend — type + auto-select detected preset

**Files:**
- Modify: `ui/frontend/src/types/import-export.ts` (`CsvInspectResponse` at line 44)
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.tsx` (`CsvMappingForm` at line 85)
- Test: `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx`

- [ ] **Step 1: Write the failing test**

Add to `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx` inside the `describe('CsvMappingDialog', ...)` block:

```tsx
  it('pre-selects the detected preset and imports with its slug', async () => {
    const user = userEvent.setup();
    const onImport = vi.fn();
    render(
      <CsvMappingDialog
        open
        onOpenChange={vi.fn()}
        inspect={{
          ...inspect,
          presets: [{ slug: 'completionator', name: 'Completionator' }],
          detected: { slug: 'completionator', name: 'Completionator' },
        }}
        isImporting={false}
        onImport={onImport}
      />,
    );

    // Opened straight into the collapsed preset view (no manual mapping form).
    expect(screen.queryByText('1 · Map columns')).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Format' })).toHaveTextContent('Completionator');

    const importBtn = screen.getByRole('button', { name: /import 3 games/i });
    expect(importBtn).toBeEnabled();
    await user.click(importBtn);

    expect(onImport).toHaveBeenCalledTimes(1);
    expect(onImport.mock.calls[0][0].format).toBe('completionator');
  });

  it('defaults to Generic CSV when nothing is detected', () => {
    render(
      <CsvMappingDialog
        open
        onOpenChange={vi.fn()}
        inspect={inspect}
        isImporting={false}
        onImport={vi.fn()}
      />,
    );
    expect(screen.getByText('1 · Map columns')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Format' })).toHaveTextContent('Generic CSV');
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test csv-mapping-dialog.test.tsx`
Expected: FAIL — the detected preset is ignored, form opens on Generic CSV showing "1 · Map columns".

- [ ] **Step 3: Add the type field**

In `ui/frontend/src/types/import-export.ts`, add to `CsvInspectResponse` (line 44):

```ts
export interface CsvInspectResponse {
  headers: string[];
  row_count: number;
  columns: CsvColumnInfo[];
  suggested_mapping?: CsvMapping;
  presets?: CsvPresetInfo[];
  detected?: CsvPresetInfo | null;
}
```

- [ ] **Step 4: Initialise `format` from the detection + add a hint**

In `ui/frontend/src/components/import/csv-mapping-dialog.tsx`, change the `format` initialiser (line 89):

```tsx
  const [format, setFormat] = useState(() => inspect.detected?.slug ?? 'generic');
```

Then, inside the existing `{isPreset && (...)}` block (line 164-169), append a muted auto-detected hint so the recognition is visible. Replace that block with:

```tsx
        {isPreset && (
          <p className="text-sm text-muted-foreground">
            {format === inspect.detected?.slug && 'Detected automatically. '}
            Columns, play-status, platforms, ratings and dates are mapped automatically by the{' '}
            {presets.find((p) => p.slug === format)?.name ?? format} preset.
            {format === inspect.detected?.slug && ' Switch the Format above to map columns yourself.'}
          </p>
        )}
```

- [ ] **Step 5: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test csv-mapping-dialog.test.tsx`
Expected: PASS — including the existing tests (default-Generic path unchanged; the preset/switch-back tests still pass because `inspect.detected` is undefined in their fixtures).

- [ ] **Step 6: Type-check and dead-code check**

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: zero errors, zero findings.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/types/import-export.ts ui/frontend/src/components/import/csv-mapping-dialog.tsx ui/frontend/src/components/import/csv-mapping-dialog.test.tsx
git commit -m "feat: pre-select auto-detected CSV format in import dialog"
```

---

## Task 4: Full-suite verification + PR

- [ ] **Step 1: Run the Go suite for the touched package**

Run: `go test ./internal/api/... ./internal/services/csvmap/...`
Expected: PASS.

- [ ] **Step 2: Build**

Run: `make frontend && make build`
Expected: clean build (the pre-push hook also runs `go test ./...` + frontend `check`/`knip`/`test`).

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin feat/1015-autodetect-csv-format
gh pr create --title "feat: auto-detect known CSV formats on import" \
  --label enhancement \
  --body "$(cat <<'EOF'
Auto-detects a known CSV preset (Completionator, Grouvee, Darkadia, Nexorious) by its
header signature on `POST /api/import/csv/inspect` and pre-selects it in the mapping
dialog so the user can confirm-and-import in one click. Unrecognised CSVs open the
manual mapping dialog exactly as before; the user can always switch the Format dropdown
back to Generic CSV to map columns by hand.

- Backend: `detectPreset` skips empty-signature presets and returns the first
  `MatchesSignature` hit as a new `detected` field on the inspect response.
- Frontend: the dialog initialises its `format` state from `inspect.detected` and shows
  a "detected automatically" hint; no route-component change.

Closes #1015
EOF
)"
```

Expected: PR opens with the `CI Gate` check running.

---

## Self-Review

- **Spec coverage:**
  - *Detection on `inspect` iterating `csvmap.Presets()` + `MatchesSignature`, first match wins* → Task 1 (`detectPreset`) + Task 2 (wired into the response).
  - *Returns matched `{slug, name}` or none* → `Detected *csvPresetInfo` with `omitempty` (absent ⇒ frontend treats as "none").
  - *Frontend: matched ⇒ confirm-and-import affordance with manual override; no match ⇒ manual dialog unchanged* → Task 3 (pre-select + collapsed preset view + Generic override; default-Generic test proves the unmatched path is unchanged).
  - *Tests: registered signature ⇒ detected; no-match ⇒ no detection* → Task 1 unit tests + Task 2 API tests + Task 3 frontend tests.
- **Out of scope (not touched):** authoring the Nexorious own-CSV format (#1033, already landed); the source presets themselves (#1002/#1003/#1016). This plan only *detects* what is already registered.
- **Placeholder scan:** none — every code/test step shows full code and an exact command with expected output.
- **Type consistency:** `detectPreset` returns `*csvPresetInfo` (same struct already used for `Presets`); response field `Detected *csvPresetInfo` `json:"detected,omitempty"` ⇄ TS `detected?: CsvPresetInfo | null` ⇄ dialog reads `inspect.detected?.slug`. Names line up across Go and TS.
- **Empty-signature guard:** explicitly handled in `detectPreset` (Task 1) and called out in Background — the one real correctness trap.
