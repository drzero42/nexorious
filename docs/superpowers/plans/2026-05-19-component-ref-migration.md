# React.ElementRef → ComponentRef Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate React 19 deprecation hints by renaming all 38 occurrences of `React.ElementRef` to `React.ComponentRef` across 14 shadcn/ui component files.

**Architecture:** Pure mechanical rename. `React.ComponentRef` is the React 19 successor with identical semantics and the same generic signature, so no surrounding edits are required. The existing TypeScript + test suites act as the regression net: if the rename were unsafe, `npm run check` or `npm run test` would fail.

**Tech Stack:** TypeScript, React 19, Vite, Vitest. All work happens under `ui/frontend/`.

**Spec:** [docs/superpowers/specs/2026-05-19-component-ref-migration-design.md](../specs/2026-05-19-component-ref-migration-design.md)
**Issue:** [#538](https://github.com/drzero42/nexorious/issues/538)
**Branch:** `component-ref-migration` (already created and contains the spec commit)

---

## Task 1: Apply the rename and verify

**Files (all under `ui/frontend/src/components/ui/`):**
- Modify: `alert-dialog.tsx` (6 occurrences)
- Modify: `avatar.tsx` (3 occurrences)
- Modify: `checkbox.tsx` (1 occurrence)
- Modify: `command.tsx` (7 occurrences)
- Modify: `dialog.tsx` (4 occurrences)
- Modify: `dropdown-menu.tsx` (2 occurrences)
- Modify: `label.tsx` (1 occurrence)
- Modify: `popover.tsx` (1 occurrence)
- Modify: `progress.tsx` (1 occurrence)
- Modify: `scroll-area.tsx` (2 occurrences)
- Modify: `select.tsx` (5 occurrences)
- Modify: `sheet.tsx` (3 occurrences)
- Modify: `switch.tsx` (1 occurrence)
- Modify: `tooltip.tsx` (1 occurrence)

**Note:** There is no new test to write. This is a refactor whose regression net is the existing TypeScript compiler check (`npm run check`), knip (`npm run knip`), and the existing Vitest suite (`npm run test`). Steps 1 and 2 below establish a clean baseline before changing anything; Steps 4–7 verify nothing broke.

- [ ] **Step 1: Verify the working tree is clean and on the right branch**

Run (from repo root):
```bash
git status --short && git branch --show-current
```
Expected: no output from `git status --short` (working tree clean), and `component-ref-migration` printed by `git branch --show-current`.

If the branch is missing, create it with `git checkout -b component-ref-migration`. If the tree is dirty, stop and resolve before proceeding — the rename commit must be atomic.

- [ ] **Step 2: Capture the baseline occurrence count**

Run (from repo root):
```bash
grep -rn "React.ElementRef" ui/frontend/src
```
Expected: exactly **38 matches across 14 files** under `ui/frontend/src/components/ui/`. The matching files must be the ones listed at the top of this task. If the count or file list differs, **stop** — the codebase has changed since the spec was written and the spec needs to be revisited rather than blindly proceeding.

- [ ] **Step 3: Apply the rename across all matching files**

Run (from repo root):
```bash
grep -rl "React.ElementRef" ui/frontend/src/components/ui/ | xargs sed -i 's/React\.ElementRef/React.ComponentRef/g'
```

What this does:
- `grep -rl` lists every file containing the literal `React.ElementRef`.
- `xargs sed -i 's/.../...g'` rewrites each occurrence in place. The escaped `\.` ensures the dot in the pattern is literal.

There is no import to update — `React.ComponentRef` is a property on the `React` namespace already imported as `import * as React from "react"` at the top of every shadcn file.

- [ ] **Step 4: Verify the rename is complete**

Run (from repo root):
```bash
grep -rn "React.ElementRef" ui/frontend/src; echo "exit=$?"
```
Expected: no matching lines printed, and `exit=1` (grep's "not found" exit code).

Then sanity-check the new symbol shows up in the same places:
```bash
grep -rcn "React.ComponentRef" ui/frontend/src/components/ui | sort
```
Expected: 14 files listed, occurrences summing to 38. The per-file counts should match the table at the top of this task.

- [ ] **Step 5: Run TypeScript check**

Run (from `ui/frontend/`):
```bash
cd ui/frontend && npm run check
```
Expected: exits 0 with no errors. This confirms `React.ComponentRef<typeof PrimitiveX>` accepts the same generic argument shapes that `React.ElementRef` did. If this fails with "Property 'ComponentRef' does not exist on type 'typeof React'", the installed `@types/react` is older than React 19 — stop and investigate; do not attempt to silence the error.

- [ ] **Step 6: Run knip**

Run (from `ui/frontend/`):
```bash
npm run knip
```
Expected: exits 0 with no findings (or no *new* findings — the count should equal the pre-change baseline). The rename does not change any exports, so this should be a no-op for knip.

- [ ] **Step 7: Run the frontend test suite**

Run (from `ui/frontend/`):
```bash
npm run test
```
Expected: exits 0 with all tests passing. This confirms no runtime regression for components whose forwarded-ref types were renamed.

- [ ] **Step 8: Commit the rename**

Run (from repo root):
```bash
git add ui/frontend/src/components/ui/*.tsx
git status --short
```
Expected `git status --short` output: 14 `M` entries, one per file in the table above, and nothing else staged.

Then commit:
```bash
git commit -m "$(cat <<'EOF'
Replace deprecated React.ElementRef with React.ComponentRef

React 19 deprecated React.ElementRef in favor of React.ComponentRef.
Mechanical rename across 14 shadcn/ui components (38 occurrences).
No behavior change; npm run check, knip, and tests pass.

Closes #538
EOF
)"
```

---

## Task 2: Push and open the PR

- [ ] **Step 1: Push the branch**

Run (from repo root):
```bash
git push -u origin component-ref-migration
```
Expected: branch published, upstream tracking set.

- [ ] **Step 2: Open the PR**

Run (from repo root):
```bash
gh pr create --title "Replace React.ElementRef with React.ComponentRef (#538)" --body "$(cat <<'EOF'
## Summary
- Renames `React.ElementRef` → `React.ComponentRef` across 14 shadcn/ui components (38 occurrences) to clear React 19 deprecation hints
- Pure mechanical rename — same generic signature, no behavior change

## Test plan
- [x] `grep -rn "React.ElementRef" ui/frontend/src` returns no matches
- [x] `npm run check` passes
- [x] `npm run knip` passes
- [x] `npm run test` passes

Closes #538

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
Expected: PR URL printed.

- [ ] **Step 3: Report the PR URL back to the user**

Print the PR URL so the user can review and merge. Do **not** merge — the project's branch workflow says merges only happen on explicit user instruction.
