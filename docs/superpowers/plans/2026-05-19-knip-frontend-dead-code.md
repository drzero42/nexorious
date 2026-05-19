# Knip Frontend Dead-Code Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt Knip as the dead-code gate for the frontend, commit `routeTree.gen.ts` so CI no longer needs a vite build, and run eslint in CI.

**Architecture:** Frontend-only tooling change. Three connected pieces ship in one PR: (1) commit the previously-gitignored TanStack Router-generated file so type-check + lint + knip work without a build step, (2) add Knip as a pinned devDependency with a `knip.json` config, then run it and remove every finding, (3) replace the existing `type-check` CI job with a new `frontend-checks` job that runs `npm run check && npm run knip` directly.

**Tech Stack:** Knip 6.x, npm, vitest, ESLint 9, TanStack Router Vite plugin, GitHub Actions.

**Spec:** [docs/superpowers/specs/2026-05-19-knip-frontend-dead-code-design.md](../specs/2026-05-19-knip-frontend-dead-code-design.md)

---

## File-by-file map

| File | Change |
|---|---|
| `.gitignore` (repo root) | Delete line 8: `ui/frontend/src/routeTree.gen.ts` |
| `ui/frontend/src/routeTree.gen.ts` | First commit of this generated file (currently gitignored) |
| `ui/frontend/package.json` | Add `knip` to `devDependencies`; add `"knip": "knip"` to `scripts` |
| `ui/frontend/package-lock.json` | Updated by `npm install` |
| `ui/frontend/knip.json` | **New file** — Knip configuration |
| `.github/workflows/test.yaml` | Rename `type-check` job to `frontend-checks`; replace build step with check + knip |
| `CLAUDE.md` | Replace the routeTree gitignore bullet under "TypeScript (Frontend)" |
| `ui/frontend/src/**` | Various deletions/edits driven by Knip findings (discovered during execution) |

---

## Task 1: Commit `routeTree.gen.ts`

**Files:**
- Modify: `.gitignore` (delete line 8)
- Add to git: `ui/frontend/src/routeTree.gen.ts`

- [ ] **Step 1: Verify the file exists on disk and is current**

From the repo root:

```bash
ls -l ui/frontend/src/routeTree.gen.ts
```

Expected: file exists. If it does not (fresh worktree), generate it first:

```bash
cd ui/frontend && npm ci && npm run build
```

`npm run build` invokes the TanStack Router Vite plugin which writes `src/routeTree.gen.ts`.

- [ ] **Step 2: Remove the file from `.gitignore`**

Delete this line from the repo-root `.gitignore`:

```
ui/frontend/src/routeTree.gen.ts
```

Verify the file is no longer ignored:

```bash
git check-ignore -v ui/frontend/src/routeTree.gen.ts
```

Expected: exit code 1 (no longer ignored).

- [ ] **Step 3: Stage and commit**

```bash
git add .gitignore ui/frontend/src/routeTree.gen.ts
git status
```

Expected: two paths staged, no other changes.

```bash
git commit -m "chore: commit routeTree.gen.ts per TanStack Router FAQ

The TanStack Router FAQ recommends committing routeTree.gen.ts because
it is runtime code, not a build artifact. Stop gitignoring it so
type-check and lint can run without a preceding vite build."
```

---

## Task 2: Add Knip as a dev dependency

**Files:**
- Modify: `ui/frontend/package.json`
- Modify: `ui/frontend/package-lock.json`

- [ ] **Step 1: Install Knip**

```bash
cd ui/frontend
npm install --save-dev knip@^6.14.1
```

This adds `knip` to `devDependencies` and updates `package-lock.json`. Use `^6.14.1` (latest stable at plan time). If a newer 6.x release exists at execution time, that is fine — keep the caret, just update the explicit minor in the commit message.

- [ ] **Step 2: Add the npm script**

Edit `ui/frontend/package.json`. In the `"scripts"` object, add a `"knip"` entry. The final ordering inside `scripts` should be:

```json
"scripts": {
  "dev": "vite",
  "build": "vite build && tsc --noEmit",
  "preview": "vite preview",
  "lint": "eslint .",
  "check": "tsc --noEmit && eslint .",
  "knip": "knip",
  "test": "vitest run",
  "test:watch": "vitest",
  "test:coverage": "vitest run --coverage",
  "test:ui": "vitest --ui"
}
```

- [ ] **Step 3: Verify the binary is reachable**

```bash
cd ui/frontend
npx --no-install knip --version
```

Expected: prints a 6.x version. (`--no-install` ensures we're using the project-local install, not a download.)

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/package.json ui/frontend/package-lock.json
git commit -m "build(frontend): add knip 6.x as devDependency

Adds a pinned knip devDep and an npm run knip script. No config yet —
that lands in the next commit."
```

---

## Task 3: Add Knip configuration

**Files:**
- Create: `ui/frontend/knip.json`

- [ ] **Step 1: Write a minimal Knip config**

Create `ui/frontend/knip.json` with this content:

```json
{
  "$schema": "https://unpkg.com/knip@6/schema.json",
  "ignore": [
    "src/routeTree.gen.ts"
  ]
}
```

Why so small: Knip ships built-in plugins for Vite, Vitest, TanStack Router, ESLint, PostCSS, Tailwind, and MSW. It auto-detects entry points and config files via `package.json` and the configs already in the repo. We only ignore `routeTree.gen.ts` because it is auto-regenerated and should not be analysed for unused exports.

- [ ] **Step 2: Run Knip and observe the output**

```bash
cd ui/frontend
npm run knip
```

Expected: command runs to completion. Exit code is 1 if findings exist (which is fine — we'll fix them in Task 4) and 0 if the codebase is already clean.

Sanity-check the output:

- Confirm the report is not riddled with false positives caused by misconfiguration (e.g., every test file flagged as unused, or every entry point flagged).
- If a whole category looks like a misconfiguration (e.g., `vitest` config not detected), add the relevant plugin override to `knip.json` before moving on. Reference: https://knip.dev/reference/plugins.

If the only "issues" are real findings (unused files, exports, deps), the config is good.

- [ ] **Step 3: Commit the config**

```bash
git add ui/frontend/knip.json
git commit -m "build(frontend): add knip configuration

Minimal config relying on knip's built-in plugins. Only carve-out is
ignoring the auto-generated routeTree.gen.ts."
```

---

## Task 4: Resolve Knip findings

This task is iterative. Knip categorises findings; we address one category at a time and commit each category. After every category, re-run `npm run knip` to confirm that category is gone before moving on.

**Files:** Discovered during execution. Will likely touch files under `ui/frontend/src/` and `ui/frontend/package.json`.

- [ ] **Step 1: Capture the initial findings**

```bash
cd ui/frontend
npm run knip 2>&1 | tee /tmp/knip-initial.txt
```

This is for reference only — re-run knip after each fix.

- [ ] **Step 2: Fix "Unused files"**

For each file listed under `Unused files`:

1. Open the file. Confirm with `grep` from `ui/frontend/src` that nothing imports it (knip is generally correct, but a sanity check costs nothing):

   ```bash
   grep -rn "from .*<filename-without-extension>" ui/frontend/src
   ```

2. Delete the file:

   ```bash
   rm ui/frontend/src/path/to/file.tsx
   ```

3. If it was a test file (`*.test.ts(x)`) and its target file is also unused, delete the target as part of this category too.

Re-run knip to confirm the category is gone:

```bash
cd ui/frontend && npm run knip
```

Expected: zero entries under `Unused files`.

Run the rest of the local checks to be sure nothing regressed:

```bash
cd ui/frontend && npm run check && npm run test
```

Expected: both pass.

Commit:

```bash
git add -A
git commit -m "refactor(frontend): remove files unused per knip"
```

If knip reported no unused files, skip the commit and continue.

- [ ] **Step 3: Fix "Unused dependencies" and "Unused devDependencies"**

For each entry:

1. Confirm with a repo-wide grep that no source file imports the package and no config file references it:

   ```bash
   grep -rn "<package-name>" ui/frontend/src ui/frontend/*.ts ui/frontend/*.mjs ui/frontend/*.json
   ```

   Pay special attention to devDependencies — some are referenced only by config files (`postcss.config.mjs`, `eslint.config.mjs`, `tailwind.config.*`, etc.) and Knip's plugins should already account for those. If grep finds a config-file reference Knip missed, it's a false positive — add a plugin override or `ignoreDependencies` entry to `knip.json` instead of removing the dep.

2. Remove confirmed unused deps:

   ```bash
   cd ui/frontend && npm uninstall <package-name>
   ```

Re-run knip and confirm:

```bash
cd ui/frontend && npm run knip
```

Expected: zero entries under `Unused dependencies` and `Unused devDependencies`.

Verify nothing breaks:

```bash
cd ui/frontend && npm run check && npm run test
```

Commit:

```bash
git add ui/frontend/package.json ui/frontend/package-lock.json
git commit -m "build(frontend): remove dependencies unused per knip"
```

If knip reported nothing here, skip the commit.

- [ ] **Step 4: Fix "Unused exports" and "Unused exported types"**

For each `(<file>:<line>) <symbol>` entry:

1. Open the file. Search for the symbol's usages:

   ```bash
   grep -rn "\\b<SymbolName>\\b" ui/frontend/src
   ```

2. If the symbol is used only inside its own file: remove the `export` keyword.
3. If the symbol is genuinely unused anywhere: delete the declaration entirely (and any imports it pulls in that become unused as a result).

Be especially careful with re-exports in barrel files like `ui/frontend/src/api/index.ts`. Knip already understands barrel files, but if it flags a re-export that the bundle truly needs (e.g., a public-facing module reachable only via dynamic import), prefer adding to `knip.json` `ignore` over deleting the export. If you need to document a reason, rename the file to `knip.jsonc` (Knip supports both `.json` and `.jsonc`) and add comments there — don't put `//` comments in a strict `.json` file.

Re-run knip:

```bash
cd ui/frontend && npm run knip
```

Expected: zero entries under `Unused exports` and `Unused exported types`.

Verify:

```bash
cd ui/frontend && npm run check && npm run test
```

Commit:

```bash
git add -A
git commit -m "refactor(frontend): remove exports unused per knip"
```

If knip reported nothing here, skip the commit.

- [ ] **Step 5: Fix remaining categories**

Knip may also report:

- `Unused enum members`
- `Unused class members`
- `Duplicate exports`
- `Unlisted dependencies` (deps used but missing from package.json — should be added, not removed)
- `Unresolved imports`

Walk through each category. Treat each as a separate commit if there is more than one category to address. For each finding:

1. Open the offending file.
2. Verify with `grep` that knip is right.
3. Apply the obvious fix: delete unused enum/class members, dedupe exports, add missing deps, fix broken imports.

Re-run knip after each commit:

```bash
cd ui/frontend && npm run knip
```

- [ ] **Step 6: Final clean-state check**

After all categories are addressed:

```bash
cd ui/frontend
npm run knip
echo "knip exit code: $?"
npm run check
npm run test
```

Expected: `knip exit code: 0`. No knip findings. `check` clean. Tests pass.

From the repo root, confirm the Go binary still builds against the trimmed frontend:

```bash
make
```

Expected: builds the frontend, then the Go binary, with no errors.

---

## Task 5: Switch CI to `frontend-checks`

**Files:**
- Modify: `.github/workflows/test.yaml` (the `type-check` job, currently at lines ~80–103)

- [ ] **Step 1: Replace the `type-check` job with `frontend-checks`**

In `.github/workflows/test.yaml`, find this block:

```yaml
  type-check:
    name: Type Check
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v6

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "npm"
          cache-dependency-path: ui/frontend/package-lock.json

      - name: Install dependencies
        run: cd ui/frontend && npm ci

      - name: Build (generates routeTree.gen.ts) and type check
        # `npm run build` runs `vite build && tsc --noEmit`. The vite build step triggers
        # the TanStack Router plugin to generate src/routeTree.gen.ts (gitignored),
        # which tsc requires. Running `npm run check` alone would fail without it.
        run: cd ui/frontend && npm run build
```

Replace it with:

```yaml
  frontend-checks:
    name: Frontend Checks
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v6

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "npm"
          cache-dependency-path: ui/frontend/package-lock.json

      - name: Install dependencies
        run: cd ui/frontend && npm ci

      - name: Type check and lint
        run: cd ui/frontend && npm run check

      - name: Knip (dead code)
        run: cd ui/frontend && npm run knip
```

Notes:

- Job ID changed from `type-check` to `frontend-checks`. This will be a required-check name change if branch protection lists the old name; flag this in the PR description so the repo admin can update branch protection.
- `npm run check` now works directly because `routeTree.gen.ts` is committed.
- No vite build runs in this job. `build-push.yaml` (the workflow that builds release artifacts) is unchanged.

- [ ] **Step 2: Sanity-check the YAML**

```bash
cd /home/abo/workspace/home/nexorious
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yaml'))"
```

Expected: no output (YAML is valid).

If `python3` is unavailable, use any YAML validator. Or, if you have `act` installed, dry-run the workflow:

```bash
act -W .github/workflows/test.yaml -j frontend-checks -n
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/test.yaml
git commit -m "ci: replace type-check job with frontend-checks

Renames the job and replaces the build step with npm run check
(tsc + eslint) and npm run knip. No vite build needed in CI now
that routeTree.gen.ts is committed. Branch-protection required-check
name changes from \"Type Check\" to \"Frontend Checks\"."
```

---

## Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md` line 187 (under "TypeScript (Frontend)")

- [ ] **Step 1: Replace the bullet**

Find this line in `CLAUDE.md`:

```
- `routeTree.gen.ts` is gitignored — run `npm run build` once in a fresh worktree before type-checking
```

Replace it with:

```
- `routeTree.gen.ts` is generated by the TanStack Router Vite plugin and committed to git. After adding, moving, renaming, or removing a route, run `npm run dev` or `npm run build` to regenerate it, then commit the change alongside the route edit — CI fails if the committed file drifts from the route definitions.
```

- [ ] **Step 2: Verify**

```bash
grep -n routeTree CLAUDE.md
```

Expected: exactly one match, with the new wording. No mention of "gitignored".

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md routeTree.gen.ts instruction

The file is now committed to git per the TanStack Router FAQ.
Devs must regenerate (via npm run dev/build) and commit it
alongside route edits."
```

---

## Task 7: End-to-end verification

**Files:** None — verification only.

- [ ] **Step 1: Confirm clean working tree**

```bash
git status
```

Expected: `nothing to commit, working tree clean`.

- [ ] **Step 2: Run every relevant gate locally**

```bash
cd /home/abo/workspace/home/nexorious/ui/frontend
npm ci
npm run check
npm run knip
npm run test
```

Expected: all four commands exit 0. `npm run knip` reports zero findings.

- [ ] **Step 3: Confirm the Go binary builds with the trimmed frontend**

```bash
cd /home/abo/workspace/home/nexorious
make
ls -l nexorious
```

Expected: `make` succeeds; `nexorious` binary is produced.

- [ ] **Step 4: Confirm the commit log**

```bash
git log --oneline main..HEAD
```

Expected: roughly these commits, in order (some of the knip cleanup commits may be skipped if there were no findings in that category):

1. `docs: spec for knip frontend dead-code detection (#516)` — already on branch from brainstorming
2. `docs: add CLAUDE.md instruction for regenerating routeTree.gen.ts` — already on branch
3. `chore: commit routeTree.gen.ts per TanStack Router FAQ`
4. `build(frontend): add knip 6.x as devDependency`
5. `build(frontend): add knip configuration`
6. Zero or more `refactor(frontend): remove ... unused per knip` / `build(frontend): remove dependencies unused per knip` commits
7. `ci: replace type-check job with frontend-checks`
8. `docs: update CLAUDE.md routeTree.gen.ts instruction`

- [ ] **Step 5: Hand off**

The branch is ready for PR. The PR description should:

- Link to issue #516 and the design doc at `docs/superpowers/specs/2026-05-19-knip-frontend-dead-code-design.md`.
- Note the branch-protection required-check name change from `Type Check` → `Frontend Checks` so the repo admin can update it.
- Summarise what was removed (files / exports / deps).

Do **not** open the PR until the user explicitly requests it (per CLAUDE.md: "Never merge a PR on your own initiative — only when the user explicitly instructs"; same principle applies to opening one without direction).
