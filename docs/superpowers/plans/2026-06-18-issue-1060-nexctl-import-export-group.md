# `nexctl` Phase 5 — `import` + `export` Implementation Plan (Epic #1060)

Design: [../specs/2026-06-18-issue-1060-nexctl-import-export-group-design.md](../specs/2026-06-18-issue-1060-nexctl-import-export-group-design.md)

One PR, whole phase. Subagent-driven: per-task implementer → controller commit → independent review → whole-branch review → merge on explicit user instruction. **Serialize same-package implementers** (cliclient tasks; cmd/nexctl tasks) — concurrent edits in one Go package collide. Reviewers (read-only) run concurrently.

## Tasks

- **T1 — multipart helper + import client methods.** `internal/cliclient`: add `doBearerMultipart`; `ListImportSources`, `ImportNexorious`, `InspectCSV`, `ImportCSV`, `ImportSource`; types `ImportSource`, `ImportResult`, `CSVInspect`/`CSVColumn`/`CSVPreset`/`CSVSuggestedMapping`. Tests: httptest mux asserting multipart `file` part + `format`/`mapping` fields, envelope decode, error surfacing.
- **T2 — export + review client methods.** `internal/cliclient`: `TriggerExport`, `DownloadExport` (streams to `io.Writer`, no decode), `ResolveJobItem`, `SkipJobItem`; types `ExportResult`. Tests: trigger envelope, download bytes streamed, resolve body `{igdb_id}`, skip 200, error paths. (Runs after T1 — same package.)
- **T3 — `import` group skeleton + sources/nexorious/run.** `cmd/nexctl/import.go`: `newImportCmd()` registering subcommands; `import sources` (table/json/-q), `import nexorious <file>`, `import run <source> <file>` (runtime registry validation). Register on root (`main.go`); add `"import": false` to `main_test.go` want-map. Tests via root + httptest.
- **T4 — `import csv`.** `cmd/nexctl/import_csv.go`: `--inspect`, `--preset`, manual mapping flags → `mapping` JSON (title-required guard, preset/manual mutual-exclusion). Tests: inspect render, preset upload, flag→mapping assembly (incl. `--status-map` repeats + `--rating-scale`).
- **T5 — `import review`/`resolve`/`skip`.** `cmd/nexctl/import_review.go`: interactive walker (TTY-gated, reuse `SearchIGDB` IGDB-pick pattern from sync.go), non-interactive `resolve --igdb-id`, `skip` with persistent `-y` confirm. Tests: resolve forwards igdb_id; skip confirm/abort; off-TTY review errors with hint.
- **T6 — `export` command.** `cmd/nexctl/export.go`: `newExportCmd()`; trigger (`--format` json|csv), `--no-wait` (print job id / `--json`), else poll `GetJob` to terminal then `DownloadExport` to `--out`/`-`/default path. Register on root. Tests: no-wait result, wait→download to a temp file, failed-job surfaces error, `--out -` to stdout.
- **T7 — docs + finalize.** Update `CLAUDE.md` `cmd/nexctl/` bullet (import/export groups, registry-driven sources, CSV presets/flags/inspect, import review vs sync review). Confirm spec/plan committed. Whole-branch review.

## Verification gates (per task + final)

`go build ./...`, `go test ./internal/cliclient/... ./cmd/nexctl/...`, `golangci-lint run`, `make deadcode` (after command wiring changes), `nexctl` REST-boundary import check (no server/DB packages).
