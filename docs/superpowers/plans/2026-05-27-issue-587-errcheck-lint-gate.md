# errcheck `check-blank` Lint Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn on `errcheck.check-blank: true` in golangci-lint so every `_ =` / `_, _ =` error discard must be justified, fixing the two genuine swallows it surfaces and annotating the rest.

**Architecture:** errcheck is already enabled (golangci-lint default set) and currently green because it ignores blank assignments. This plan flips on `check-blank`, which surfaces 70 production discards. The `std-error-handling` preset clears the `Close`/`Fprint` family, a one-entry allowlist clears `Tx.Rollback`, test files are excluded, two real swallowed DB writes are fixed with logging, and the remaining 44 acceptable discards get per-site `//nolint:errcheck // reason`.

**Tech Stack:** Go 1.25, golangci-lint v2 (errcheck), `log/slog`.

**Branch:** `issue-587-errcheck-check-blank` (already created).

**Critical ordering constraint:** CLAUDE.md mandates *zero golangci-lint errors before every commit*. Because `//nolint:errcheck` comments and the log fixes are inert while `check-blank` is off, Tasks 1–8 keep `golangci-lint run` green at every commit. **Task 9 enables `check-blank` last**, at which point the pre-placed annotations bring the gate straight to zero. Per-task correctness is verified against a temporary config that has `check-blank` on, so misplaced annotations are caught immediately rather than at the end.

**No Nix/slumber/routeTree impact:** this change touches no `go.mod`, `package-lock.json`, API route, or frontend file, so `vendorHash`, `npmDepsHash`, `slumber.yaml`, and `routeTree.gen.ts` need no updates.

---

## The final `.golangci.yml` (target state — committed in Task 9)

```yaml
version: "2"

linters:
  settings:
    errcheck:
      # Flag `_ =` / `_, _ =` discards — the exact pattern issue #534 set out to
      # eliminate. Without this, errcheck ignores all blank assignments and the
      # regression it guards against can silently return.
      check-blank: true
      exclude-functions:
        # Rollback after a committed tx is a no-op; the deferred
        # `defer func() { _ = tx.Rollback() }()` cleanup idiom is safe.
        - (github.com/uptrace/bun.Tx).Rollback
  exclusions:
    presets:
      # Excludes the conventional no-recovery stdlib cases: (*T).Close,
      # fmt.Fprint/Fprintf/Fprintln, hash.Write, etc.
      - std-error-handling
    rules:
      # Test files are out of scope for the #534 audit — `_ =` in tests is fine.
      - path: _test\.go
        linters:
          - errcheck
    paths:
      # ui/frontend/node_modules is npm's dependency tree. A handful of npm
      # packages (notably `flatted`) ship .go files for cross-language tests.
      # Those files are not part of this module and must not be linted.
      - ui/frontend/node_modules
```

---

## Task 1: Fix the two swallowed `credentials_error` writes

These two `_, _ = ...Exec(ctx)` sites silently drop a state-mutating DB write that drives the UI "credentials error" flag — a Sev-3 defect #585 missed. Fix per the worker convention (log-only; no caller to return to), mirroring the existing pattern at [sync.go:198-204](../../../internal/worker/tasks/sync.go#L198-L204).

**Files:**
- Modify: `internal/worker/tasks/sync.go` (lines 163-168 and 225-230)

- [ ] **Step 1: Edit the first site (inside the `errors.Is(err, ErrCredentials)` guard, 2-tab indent)**

Replace:
```go
		failSyncJob(ctx, w.DB, p.JobID, "credentials error")
		_, _ = w.DB.NewRaw(
			`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
			p.UserID, p.Storefront,
		).Exec(ctx)
		return nil
```
with:
```go
		failSyncJob(ctx, w.DB, p.JobID, "credentials error")
		if _, err := w.DB.NewRaw(
			`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
			p.UserID, p.Storefront,
		).Exec(ctx); err != nil {
			slog.Error("dispatch_sync: flag credentials_error failed", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
		}
		return nil
```

- [ ] **Step 2: Edit the second site (inside the `GetLibrary` error branch, 3-tab indent)**

Replace:
```go
			failSyncJob(ctx, w.DB, p.JobID, "credentials error")
			_, _ = w.DB.NewRaw(
				`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
				p.UserID, p.Storefront,
			).Exec(ctx)
			return nil
```
with:
```go
			failSyncJob(ctx, w.DB, p.JobID, "credentials error")
			if _, err := w.DB.NewRaw(
				`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
				p.UserID, p.Storefront,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: flag credentials_error failed", "err", err, "user_id", p.UserID, "storefront", p.Storefront)
			}
			return nil
```

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Run the worker-tasks tests**

Run: `go test ./internal/worker/tasks/... 2>&1 | tail -5`
Expected: `ok` / PASS (no new test added — this is observability-only, per the testing policy in CLAUDE.md and the spec).

- [ ] **Step 5: Verify lint is still green**

Run: `golangci-lint run ./internal/worker/tasks/...`
Expected: `0 issues.` (check-blank is not yet on.)

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "fix: log swallowed credentials_error writes in dispatch_sync (#534)"
```

---

## Task 2: Create the temporary verification config

A throwaway config (with `check-blank` on) used only to verify that each annotation task's `//nolint` comments actually suppress their target findings. The repo's `.golangci.yml` is **not** changed until Task 9, so commits stay green.

**Files:**
- Create: `/tmp/ec587-check.yml` (not committed)

- [ ] **Step 1: Write the verification config**

```bash
cat > /tmp/ec587-check.yml <<'EOF'
version: "2"
linters:
  settings:
    errcheck:
      check-blank: true
      exclude-functions:
        - (github.com/uptrace/bun.Tx).Rollback
  exclusions:
    presets:
      - std-error-handling
    rules:
      - path: _test\.go
        linters:
          - errcheck
    paths:
      - ui/frontend/node_modules
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
EOF
```

- [ ] **Step 2: Confirm the baseline (46 findings across the repo)**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./... 2>&1 | grep -c errcheck`
Expected: `44`. (Task 1 already fixed the 2 swallowed writes, leaving 44 sites to annotate.) Record the number; each annotation task drives it down, reaching 0 by Task 8.

> Note: if `/tmp/ec587-check.yml` is missing in a later task (fresh shell), recreate it from Step 1.

---

## Task 3: Annotate `cmd/nexorious` (3 sites)

**Files:**
- Modify: `cmd/nexorious/migrate.go:78`, `cmd/nexorious/serve.go:243`, `cmd/nexorious/serve.go:477`

- [ ] **Step 1: Annotate `migrate.go:78`**

Replace:
```go
	configFile, _ := cmd.Root().PersistentFlags().GetString("config")
```
with:
```go
	configFile, _ := cmd.Root().PersistentFlags().GetString("config") //nolint:errcheck // "config" persistent flag is always registered; cannot error
```

- [ ] **Step 2: Annotate `serve.go:243`**

Replace:
```go
			_ = riverClient.Stop(context.Background())
```
with:
```go
			_ = riverClient.Stop(context.Background()) //nolint:errcheck // best-effort stop during DB rebuild; nowhere to surface
```

- [ ] **Step 3: Annotate `serve.go:477`**

Replace:
```go
				newJSON, _ := json.Marshal(s)
```
with:
```go
				newJSON, _ := json.Marshal(s) //nolint:errcheck // marshaling a map[string]string cannot fail
```

- [ ] **Step 4: Verify these sites are now suppressed**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./cmd/... 2>&1 | grep errcheck || echo CLEAN`
Expected: `CLEAN` (no errcheck findings under `cmd/`).

- [ ] **Step 5: Verify the repo lint is still green and build works**

Run: `go build ./... && golangci-lint run ./cmd/...`
Expected: `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/migrate.go cmd/nexorious/serve.go
git commit -m "chore: annotate acceptable errcheck discards in cmd/nexorious (#587)"
```

---

## Task 4: Annotate `internal/api` (13 sites)

All are clamped query-param parses or advisory `RowsAffected`. Identical lines that repeat within a file use `replace_all`.

**Files:**
- Modify: `internal/api/backup.go` (64,65,70), `internal/api/games.go` (90,94), `internal/api/jobs.go` (105,109,322,471,475), `internal/api/user_games.go` (757,801,1052)

- [ ] **Step 1: Annotate `backup.go` (3 distinct lines)**

Replace `	h, _ := strconv.Atoi(parts[1])` with:
```go
	h, _ := strconv.Atoi(parts[1]) //nolint:errcheck // malformed cron field defaults to 0
```
Replace `	m, _ := strconv.Atoi(parts[0])` with:
```go
	m, _ := strconv.Atoi(parts[0]) //nolint:errcheck // malformed cron field defaults to 0
```
Replace `	cronDay, _ := strconv.Atoi(parts[4])` with:
```go
	cronDay, _ := strconv.Atoi(parts[4]) //nolint:errcheck // malformed cron field defaults to 0
```

- [ ] **Step 2: Annotate `games.go` (2 lines)**

Replace `	page, _ := strconv.Atoi(c.QueryParam("page"))` with:
```go
	page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty query param clamped to default below
```
Replace `	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))` with:
```go
	perPage, _ := strconv.Atoi(c.QueryParam("per_page")) //nolint:errcheck // invalid/empty query param clamped to default below
```

- [ ] **Step 3: Annotate `jobs.go` (use `replace_all` for the repeated page/per_page lines)**

`replace_all` `	page, _ := strconv.Atoi(c.QueryParam("page"))` (hits lines 105 and 471) with:
```go
	page, _ := strconv.Atoi(c.QueryParam("page")) //nolint:errcheck // invalid/empty query param clamped to default below
```
`replace_all` `	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))` (hits lines 109 and 475) with:
```go
	perPage, _ := strconv.Atoi(c.QueryParam("per_page")) //nolint:errcheck // invalid/empty query param clamped to default below
```
Replace the single `	limit, _ := strconv.Atoi(c.QueryParam("limit"))` (line 322) with:
```go
	limit, _ := strconv.Atoi(c.QueryParam("limit")) //nolint:errcheck // invalid/empty query param clamped to default below
```

- [ ] **Step 4: Annotate `user_games.go` — all three `result.RowsAffected()` discards (757, 801, 1052)**

These three lines have differing indentation, so edit each individually. For each occurrence of `rows, _ := result.RowsAffected()`, append the same comment, preserving that line's exact leading whitespace:
```go
rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
```
Step 5's verification confirms none remain — if any `result.RowsAffected` still appears there, an occurrence was missed.

- [ ] **Step 5: Verify suppression**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./internal/api/... 2>&1 | grep errcheck || echo CLEAN`
Expected: `CLEAN`.

- [ ] **Step 6: Build + green lint check**

Run: `go build ./... && golangci-lint run ./internal/api/...`
Expected: `0 issues.`

- [ ] **Step 7: Commit**

```bash
git add internal/api/backup.go internal/api/games.go internal/api/jobs.go internal/api/user_games.go
git commit -m "chore: annotate acceptable errcheck discards in internal/api (#587)"
```

---

## Task 5: Annotate `internal/backup/service.go` (12 sites)

All are best-effort filesystem/stat operations or cosmetic stats. Every line below is distinct, so use single-occurrence edits.

**Files:**
- Modify: `internal/backup/service.go` (97,98,99,102,158,421,427,436,453,484,825,857)

- [ ] **Step 1: Annotate the 4 cosmetic stats `Scan` lines (97-102)**

Replace `	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&statsUsers)` with:
```go
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&statsUsers) //nolint:errcheck // cosmetic stat; zero value acceptable on error
```
Replace `	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT game_id) FROM user_games").Scan(&statsGames)` with:
```go
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT game_id) FROM user_games").Scan(&statsGames) //nolint:errcheck // cosmetic stat; zero value acceptable on error
```
Replace `	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags").Scan(&statsTags)` with:
```go
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tags").Scan(&statsTags) //nolint:errcheck // cosmetic stat; zero value acceptable on error
```
Replace `	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(name), '') FROM bun_migrations").Scan(&migrationVersion)` with:
```go
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(name), '') FROM bun_migrations").Scan(&migrationVersion) //nolint:errcheck // cosmetic stat; zero value acceptable on error
```

- [ ] **Step 2: Annotate `os.Stat` (158)**

Replace `		info, _ := os.Stat(archivePath)` with:
```go
		info, _ := os.Stat(archivePath) //nolint:errcheck // best-effort modtime for display; zero value acceptable
```

- [ ] **Step 3: Annotate the two `io.Copy`-to-hash lines (421, 436)**

Replace `	size, _ := io.Copy(h, f)` with:
```go
	size, _ := io.Copy(h, f) //nolint:errcheck // hashing a file; hash.Hash.Write never errors
```
Replace `		_, _ = io.Copy(h, f) // hash.Hash.Write never returns an error` with:
```go
		_, _ = io.Copy(h, f) //nolint:errcheck // hash.Hash.Write never returns an error
```

- [ ] **Step 4: Annotate the checksum `WalkDir` (427, multi-line call — comment goes on the opening line)**

Replace `	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {` with:
```go
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error { //nolint:errcheck // best-effort directory checksum; non-fatal
```

- [ ] **Step 5: Annotate the two `filepath.Rel` lines (453, 484)**

Replace `		relPath, _ := filepath.Rel(src, path)` with:
```go
		relPath, _ := filepath.Rel(src, path) //nolint:errcheck // path is always under src; cannot fail here
```
Replace `		relPath, _ := filepath.Rel(baseDir, path)` with:
```go
		relPath, _ := filepath.Rel(baseDir, path) //nolint:errcheck // path is always under baseDir; cannot fail here
```

- [ ] **Step 6: Annotate `os.ReadDir` (825) and `copyDir` (857)**

Replace `	entries, _ := os.ReadDir(tmpDir)` with:
```go
	entries, _ := os.ReadDir(tmpDir) //nolint:errcheck // already in restore-failure path; empty result handled below
```
Replace `		_, _, _ = copyDir(coverArtSrc, coverArtDst)` with:
```go
		_, _, _ = copyDir(coverArtSrc, coverArtDst) //nolint:errcheck // best-effort cover-art restore; DB already restored
```

- [ ] **Step 7: Verify suppression**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./internal/backup/... 2>&1 | grep errcheck || echo CLEAN`
Expected: `CLEAN`.

- [ ] **Step 8: Build + green lint check**

Run: `go build ./... && golangci-lint run ./internal/backup/...`
Expected: `0 issues.`

- [ ] **Step 9: Commit**

```bash
git add internal/backup/service.go
git commit -m "chore: annotate acceptable errcheck discards in internal/backup (#587)"
```

---

## Task 6: Annotate `internal/scheduler` (5 sites)

All are advisory `RowsAffected` after a logged-and-returned DELETE.

**Files:**
- Modify: `internal/scheduler/scheduler.go` (220,319,375,390), `internal/scheduler/stale_jobs.go` (44)

- [ ] **Step 1: Annotate `scheduler.go` (use `replace_all`)**

`replace_all` `	rows, _ := result.RowsAffected()` (hits 220, 319, 375, 390) with:
```go
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
```

- [ ] **Step 2: Annotate `stale_jobs.go:44`**

Replace `	rows, _ := result.RowsAffected()` with:
```go
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
```

- [ ] **Step 3: Verify suppression**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./internal/scheduler/... 2>&1 | grep errcheck || echo CLEAN`
Expected: `CLEAN`.

- [ ] **Step 4: Build + green lint check**

Run: `go build ./... && golangci-lint run ./internal/scheduler/...`
Expected: `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/stale_jobs.go
git commit -m "chore: annotate acceptable errcheck discards in internal/scheduler (#587)"
```

---

## Task 7: Annotate `internal/services` (3 sites)

**Files:**
- Modify: `internal/services/igdb/igdb.go:530`, `internal/services/psn/client.go:280`, `internal/services/psn/duration.go:19`

- [ ] **Step 1: Annotate `igdb.go:530` (body drain)**

Replace `	_, _ = io.Copy(io.Discard, body)` with:
```go
	_, _ = io.Copy(io.Discard, body) //nolint:errcheck // draining body for connection reuse
```

- [ ] **Step 2: Annotate `psn/client.go:280`**

Replace `			body, _ := io.ReadAll(resp.Body)` with:
```go
			body, _ := io.ReadAll(resp.Body) //nolint:errcheck // body read only to enrich the error log line
```

- [ ] **Step 3: Annotate `psn/duration.go:19`**

Replace `	h, _ := strconv.Atoi(m[1])` with:
```go
	h, _ := strconv.Atoi(m[1]) //nolint:errcheck // m[1] is a regex \d+ capture; always numeric
```

- [ ] **Step 4: Verify suppression**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./internal/services/... 2>&1 | grep errcheck || echo CLEAN`
Expected: `CLEAN`.

- [ ] **Step 5: Build + green lint check**

Run: `go build ./... && golangci-lint run ./internal/services/...`
Expected: `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/services/igdb/igdb.go internal/services/psn/client.go internal/services/psn/duration.go
git commit -m "chore: annotate acceptable errcheck discards in internal/services (#587)"
```

---

## Task 8: Annotate `internal/worker/tasks` (8 sites)

`json.Marshal` of fixed structs, plus `EnqueueOrFail` (which records its own failure).

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (415,457,462), `internal/worker/tasks/metadata_refresh.go` (115,251,258), `internal/worker/tasks/sync.go` (220,479)

- [ ] **Step 1: Annotate `import_item.go` (3 distinct lines)**

Replace `	resultJSON, _ := json.Marshal(result)` with:
```go
	resultJSON, _ := json.Marshal(result) //nolint:errcheck // marshaling the job result struct cannot fail
```
Replace `		b, _ := json.Marshal(md.PlatformIDs)` with:
```go
		b, _ := json.Marshal(md.PlatformIDs) //nolint:errcheck // marshaling a fixed slice cannot fail
```
Replace `		b, _ := json.Marshal(md.PlatformNames)` with:
```go
		b, _ := json.Marshal(md.PlatformNames) //nolint:errcheck // marshaling a fixed slice cannot fail
```

- [ ] **Step 2: Annotate `metadata_refresh.go` (3 distinct lines)**

Replace `			sourceMeta, _ := json.Marshal(map[string]any{"game_id": g.ID})` with:
```go
			sourceMeta, _ := json.Marshal(map[string]any{"game_id": g.ID}) //nolint:errcheck // marshaling a fixed map cannot fail
```
Replace `		b, _ := json.Marshal(md.PlatformIDs)` with:
```go
		b, _ := json.Marshal(md.PlatformIDs) //nolint:errcheck // marshaling a fixed slice cannot fail
```
Replace `		b, _ := json.Marshal(md.PlatformNames)` with:
```go
		b, _ := json.Marshal(md.PlatformNames) //nolint:errcheck // marshaling a fixed slice cannot fail
```

- [ ] **Step 3: Annotate `sync.go` (220 EnqueueOrFail, 479 json.Marshal)**

Replace `			_ = EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, IGDBMatchArgs{JobItemID: itemID})` with:
```go
			_ = EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, IGDBMatchArgs{JobItemID: itemID}) //nolint:errcheck // EnqueueOrFail records its own failure on the job_item
```
Replace `		candidatesJSON, _ := json.Marshal(candidates)` with:
```go
		candidatesJSON, _ := json.Marshal(candidates) //nolint:errcheck // marshaling the candidates slice cannot fail
```

- [ ] **Step 4: Verify suppression (whole repo should now be clean under check-blank)**

Run: `golangci-lint run -c /tmp/ec587-check.yml ./... 2>&1 | grep -c errcheck`
Expected: `0`. (All 44 annotated + 2 fixed = the gate is fully satisfied.)

- [ ] **Step 5: Build + green lint check**

Run: `go build ./... && golangci-lint run ./internal/worker/...`
Expected: `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/metadata_refresh.go internal/worker/tasks/sync.go
git commit -m "chore: annotate acceptable errcheck discards in internal/worker/tasks (#587)"
```

---

## Task 9: Enable `check-blank` in the real config

With all annotations and fixes already in place, flip the gate on. The repo lint goes straight to zero.

**Files:**
- Modify: `.golangci.yml`

- [ ] **Step 1: Replace `.golangci.yml` with the target config**

Overwrite the file with the exact "final `.golangci.yml`" block shown at the top of this plan.

- [ ] **Step 2: Run the full lint — the acceptance test**

Run: `golangci-lint run ./...`
Expected: `0 issues.`

- [ ] **Step 3: Run the full test suite**

Run: `go test -timeout 600s ./... 2>&1 | tail -20`
Expected: all packages `ok` / PASS.

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml
git commit -m "chore: enable errcheck check-blank lint gate (#587)"
```

- [ ] **Step 5: Clean up the temp config**

```bash
rm -f /tmp/ec587-check.yml
```

> The gate is proven live by the Task 2 → Task 8 progression (44 findings → 0 as annotations land), so no separate regression sanity-check is needed.

---

## Task 10: Open the pull request

- [ ] **Step 1: Push the branch**

Run: `git push -u origin issue-587-errcheck-check-blank`

- [ ] **Step 2: Open the PR**

The PR contains both the two genuine fixes and the lint gate, so its title is `fix:` (per the spec; produces a patch release):

```bash
gh pr create \
  --title "fix: enable errcheck check-blank gate and surface swallowed sync writes (#587)" \
  --body "$(cat <<'EOF'
Closes #587 (final child of #534).

Enables `errcheck.check-blank: true` so every `_ =` / `_, _ =` discard must be justified — the regression-prevention goal of #534, which was previously unmet because errcheck (already enabled) ignores blank assignments by default.

- `std-error-handling` preset clears the `Close`/`Fprint` family; one-entry allowlist clears `Tx.Rollback`; test files excluded.
- Fixes 2 genuinely-swallowed `credentials_error` DB writes in `dispatch_sync` (Sev-3 class #585 missed).
- 44 acceptable discards annotated with per-site `//nolint:errcheck // reason`.

Design: docs/superpowers/specs/2026-05-27-issue-587-errcheck-lint-gate-design.md

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Post-merge follow-up (separate task, do NOT do now)**

After this PR merges, run the `claude-md-management:revise-claude-md` skill to document the `//nolint:errcheck` policy in CLAUDE.md (recorded in the spec's "Post-merge follow-up" section).

---

## Self-review notes

- **Spec coverage:** config change (Task 9), `check-blank` (Task 9), `std-error-handling` preset (Task 9), `Tx.Rollback` allowlist (Task 9), test-file exclusion (Task 9), 44 `//nolint` sites (Tasks 3–8, counts: 3+13+12+5+3+8 = 44 ✓), 2 fixes (Task 1), verification = `golangci-lint run` == 0 (Tasks 8–9), `fix:` PR title (Task 10), post-merge CLAUDE.md follow-up (Task 10 Step 3) — all covered.
- **No tests added** by design (observability-only fixes; spec + testing policy). The acceptance test is CI lint.
- **Edit uniqueness:** repeated identical lines (`page`/`per_page` Atoi in jobs.go; `RowsAffected` in scheduler.go/user_games.go) use `replace_all` with a fallback note; all other target lines are textually distinct within their file.
- **Type/name consistency:** the log fix uses `slog.Error` with the file's existing `dispatch_sync:` prefix and `p.UserID`/`p.Storefront` (confirmed in scope at the edit sites); `log/slog` is already imported in `sync.go`.
