# Remove Slumber Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Slumber API client, its collection, and all forward-looking references from the project, and close the obsolete enhancement issue.

**Architecture:** Pure deletion/cleanup. No code or tests change — only the collection file, the devenv tooling/lock, and three forward-looking docs. Historical specs/plans are left untouched.

**Tech Stack:** devenv/Nix flake inputs, Markdown docs, `gh` CLI.

**Spec:** `docs/superpowers/specs/2026-06-02-remove-slumber-design.md`

---

### Task 1: Delete the collection file

**Files:**
- Delete: `slumber.yaml`

- [ ] **Step 1: Delete the file**

```bash
git rm slumber.yaml
```

- [ ] **Step 2: Verify it's gone**

Run: `ls slumber.yaml 2>&1`
Expected: `ls: cannot access 'slumber.yaml': No such file or directory`

---

### Task 2: Remove Slumber from the dev environment

**Files:**
- Modify: `devenv.nix` (remove the slumber package line)
- Modify: `devenv.yaml` (remove the `drzero42` input block)
- Modify: `devenv.lock` (regenerated)

- [ ] **Step 1: Remove the package line from `devenv.nix`**

Delete this line (currently line 19) from the package list:

```nix
    inputs.drzero42.packages.${system}.slumber
```

The surrounding list should read (no slumber line between `jq` and `legendary-gl`):

```nix
    jq
    legendary-gl
    librsvg
```

- [ ] **Step 2: Remove the `drzero42` input from `devenv.yaml`**

Delete this block (it is the only consumer of `drzero42`):

```yaml
  drzero42:
    url: github:drzero42/nixpkgs
```

The resulting `inputs:` section should contain only `git-hooks` and `nixpkgs`:

```yaml
inputs:
  git-hooks:
    url: github:cachix/git-hooks.nix
    inputs:
      nixpkgs:
        follows: nixpkgs
  nixpkgs:
    url: github:cachix/devenv-nixpkgs/rolling
```

- [ ] **Step 3: Confirm nothing else references `drzero42`**

Run: `grep -rn drzero42 devenv.nix devenv.yaml`
Expected: no output (exit code 1).

- [ ] **Step 4: Regenerate the lock file**

Run: `devenv update`
Expected: completes without error; `git diff devenv.lock` shows the `drzero42` node removed (and possibly refreshed hashes for remaining inputs).

- [ ] **Step 5: Verify devenv still evaluates**

Run: `devenv info >/dev/null && echo OK`
Expected: `OK` (no error about a missing `drzero42` input).

- [ ] **Step 6: Commit Tasks 1–2 together**

```bash
git add slumber.yaml devenv.nix devenv.yaml devenv.lock
git commit -m "chore: remove slumber from devenv and delete its collection"
```

---

### Task 3: Scrub `README.md`

**Files:**
- Modify: `README.md` (quick-ref table row + API Documentation section)

- [ ] **Step 1: Remove the quick-reference table row**

Delete this row from the commands table:

```markdown
| API client | `slumber` |
```

- [ ] **Step 2: Reword the "API Documentation" section**

Replace the current section body:

```markdown
## API Documentation

The API is self-documenting. With the server running, explore endpoints via `slumber` (included in the devenv shell):

```bash
slumber
```

See `DEV.md` for the full development guide including database reset procedures and the two-terminal Vite dev server workflow.
```

with:

```markdown
## API Documentation

The route handlers in `internal/api/` are the source of truth for available endpoints — each domain (games, user_games, auth, platforms, tags, jobs, import, export, sync, …) has its own handler file with the registered routes.

See `DEV.md` for the full development guide including database reset procedures and the two-terminal Vite dev server workflow.
```

- [ ] **Step 3: Verify no slumber references remain in README**

Run: `grep -ni slumber README.md`
Expected: no output (exit code 1).

---

### Task 4: Scrub `DEV.md`

**Files:**
- Modify: `DEV.md` (remove the `## API Client (Slumber)` section)

- [ ] **Step 1: Remove the entire Slumber section**

Delete the whole `## API Client (Slumber)` section — from the `## API Client (Slumber)` heading through the end of the "Day-to-day use" paragraph (the paragraph ending "…cached credentials from the `local` profile."), including the trailing blank line before the next `##` heading.

- [ ] **Step 2: Verify the surrounding structure is intact**

Run: `grep -n '^## ' DEV.md`
Expected: a clean list of section headings with no `## API Client (Slumber)` entry and no orphaned content where it was.

- [ ] **Step 3: Verify no slumber references remain in DEV.md**

Run: `grep -ni slumber DEV.md`
Expected: no output (exit code 1).

---

### Task 5: Scrub `CLAUDE.md`

**Files:**
- Modify: `CLAUDE.md` (quick-ref table row + Slumber Collection Maintenance section)

- [ ] **Step 1: Remove the quick-reference table row**

Delete this row from the Common Commands table:

```markdown
| Run API client           | `slumber`                                                |
```

- [ ] **Step 2: Remove the Slumber Collection Maintenance subsection**

Delete the entire `### Slumber Collection Maintenance` subsection (the heading and all bullet lines through "…verify the collection loads without errors after any change"), including the trailing blank line before the `## Release Process` heading.

- [ ] **Step 3: Verify no slumber references remain in CLAUDE.md**

Run: `grep -ni slumber CLAUDE.md`
Expected: no output (exit code 1).

- [ ] **Step 4: Commit Tasks 3–5 together**

```bash
git add README.md DEV.md CLAUDE.md
git commit -m "docs: remove slumber references from forward-looking docs"
```

---

### Task 6: Final verification

- [ ] **Step 1: Confirm no slumber references in live files**

Run:

```bash
grep -rin slumber . --exclude-dir=.git --exclude-dir=docs/superpowers --exclude-dir=node_modules
```

Expected: no output. (Historical files under `docs/superpowers/` are intentionally excluded and retained.)

- [ ] **Step 2: Confirm the only remaining mentions are historical**

Run: `grep -rin slumber docs/superpowers | head`
Expected: matches only in dated spec/plan files (e.g. `2026-05-28-slumber-session-auth-*`). These are intentionally kept.

---

### Task 7: Open the PR and close the issue

- [ ] **Step 1: Push the branch and open the PR**

```bash
git push -u origin chore/remove-slumber
gh pr create --title "chore: remove slumber" --body "Removes the Slumber API client and its collection.

- Delete \`slumber.yaml\`
- Remove the slumber package from \`devenv.nix\` and the \`drzero42\` input from \`devenv.yaml\` (slumber was its only consumer); regenerate \`devenv.lock\`
- Scrub forward-looking docs (\`README.md\`, \`DEV.md\`, \`CLAUDE.md\`)
- Historical specs/plans under \`docs/superpowers/\` are intentionally left untouched

Obsoletes #658 (closed as not planned)."
```

Note: deliberately **not** using a `Closes #658` keyword — that would auto-close the issue as *completed* on merge. We close it as *not planned* instead (next step).

Expected: PR created.

- [ ] **Step 2: Close issue #658 as not-planned with an explanatory comment**

```bash
gh issue close 658 --reason "not planned" --comment "Slumber is being removed from the project, so this enhancement is no longer applicable. See the removal PR."
```

Expected: issue #658 shown as closed with reason "not planned".

---

## Notes for the executor

- There are no automated tests for this change — it touches no Go/TS code. The "tests" are the `grep` verification steps.
- The pre-push git hook runs Go/frontend suites only when those files change; this branch changes neither, so the hook is a fast no-op.
- Do **not** edit anything under `docs/superpowers/specs/` or `docs/superpowers/plans/` — those slumber mentions are historical and stay.
