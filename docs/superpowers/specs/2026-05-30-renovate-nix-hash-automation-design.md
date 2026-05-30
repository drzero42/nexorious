# Renovate Nix Hash Automation Design

**Date:** 2026-05-30
**Status:** Approved

## Problem

Renovatebot PRs that update `ui/frontend/package-lock.json` or `go.sum` always fail the `Nix Build` CI check. The Nix package definitions in `nix/frontend.nix` (`npmDepsHash`) and `nix/package.nix` (`vendorHash`) are content-addressed hashes that must be kept in sync with those lock files. Renovate has no awareness of these Nix-specific hashes and never updates them.

This creates two related failure modes:

**PR-level failure.** Every renovate PR that touches a lock file has an incorrect hash in the nix files. The Nix Build check fails on the PR branch, blocking merge.

**Post-merge main corruption.** When two renovate PRs both update `package-lock.json` (touching different packages), each gets its hash fixed in isolation. If PR#1 merges first and PR#2 is not rebased, a squash-merge of PR#2 applies PR#2's lock file diff on top of main's already-updated lockfile. The result on main is a lockfile that contains both sets of changes, but the `npmDepsHash` was only computed for PR#2's isolated change — so main's Nix Build breaks after the merge.

Additionally, when both PRs run the self-healing workflow, they both overwrite the same `npmDepsHash` line in `nix/frontend.nix` (from the original hash to their respective computed hash). This means a subsequent rebase of PR#2 onto main will hit a **git conflict** on that line — GitHub's `update-branch` API rejects conflicts, so a git-level merge with explicit conflict resolution is required.

## Solution: Two Complementary Workflows

### Workflow 1 — Self-healing Nix Build

Modify the existing `.github/workflows/nix.yaml` to attempt a hash fix when `nix build` fails with a hash mismatch, then re-run the build in the same job.

**Trigger:** `pull_request` on `main`, filtered to paths: `go.sum`, `ui/frontend/package-lock.json`, `nix/**`, `flake.nix`, `flake.lock`. (Existing push-to-main trigger remains unchanged — the build just fails normally on main since we cannot commit directly to a protected branch.)

**Job permissions:** `contents: write` (needed to push the hash-fix commit back to the PR branch).

**Steps:**

1. Checkout code (with `persist-credentials: true` so git push works).
2. Configure git identity for the fix commit.
3. Install Nix (existing step).
4. Run `nix build .#nexorious 2>&1 | tee /tmp/nix-build.log`; capture exit code.
5. If exit code is non-zero and `/tmp/nix-build.log` contains `hash mismatch`:
   a. For each `hash mismatch` block, extract the `specified:` hash and the `got:` hash from the log.
   b. For each pair, run `sed -i "s|<specified>|<got>|g"` across `nix/frontend.nix` and `nix/package.nix`.
   c. Stage and commit the updated files with message `chore(nix): update hashes for <branch-name>`.
   d. Push to the PR branch (`git push origin HEAD:<branch>`).
   e. Re-run `nix build .#nexorious`; exit with the new result code.
6. Otherwise exit with the original code.

The re-run after the fix happens within the same CI job — no new workflow trigger is needed, and `GITHUB_TOKEN` is sufficient for the push since we don't need to trigger other workflows from it.

**Hash extraction:** Nix prints hash mismatches in a stable format:

```
error: hash mismatch in fixed-output derivation '...':
         specified: sha256-<OLD>
            got:    sha256-<NEW>
```

Both `npmDepsHash` (frontend npm deps) and `vendorHash` (Go vendor directory) produce this format when wrong. The sed substitution replaces the wrong hash with the correct one in whichever `.nix` file contains it.

**Edge cases:**

- If the build fails for a reason other than hash mismatch (e.g., a compile error), the workflow exits with the original non-zero code after the hash-extraction step finds nothing to substitute. No spurious commit is made.
- If both `npmDepsHash` and `vendorHash` are wrong simultaneously (e.g., a PR updates both Go and npm deps), both mismatches appear in the log and both are fixed in the same commit.
- The push uses `GITHUB_TOKEN` which will not trigger a new separate workflow run. The re-run of `nix build` happens immediately in the same job step, so no external trigger is needed.

### Workflow 2 — Post-merge Renovate Rebase

New file `.github/workflows/rebase-renovate-prs.yaml`. After any merge to main that changes a lock file, finds all open renovate PRs and rebases them onto the new main so their lock files (and therefore required Nix hashes) are up to date.

**Trigger:** `push` to `main`, filtered to paths: `go.sum`, `ui/frontend/package-lock.json`.

**Token:** Uses `secrets.RELEASE_PLEASE_TOKEN` (the existing fine-grained PAT, which has `contents: write` and `pull-requests: write`). A push made with a PAT — unlike one made with `GITHUB_TOKEN` — triggers downstream workflows on the target branch, so the push to each renovate branch causes Workflow 1 to run and fix that branch's hash.

**Steps:**

1. Checkout main (full history not required; `fetch-depth: 1`).
2. Configure git with the same identity used for the fix commits.
3. List all open PRs authored by `app/renovate` via `gh pr list --author app/renovate --state open --json number,headRefName,headRefOid`.
4. For each PR:
   a. Fetch the PR branch: `git fetch origin <branch>`.
   b. Check whether the branch is already up to date with main (`git merge-base --is-ancestor origin/main origin/<branch>`). Skip if already up to date.
   c. Create a local tracking branch: `git checkout -B <branch> origin/<branch>`.
   d. Merge main using the "theirs" strategy for conflicts: `git merge origin/main -X theirs --no-edit -m "chore: merge main into <branch>"`.
      - `-X theirs` resolves any git conflict (including the `npmDepsHash` / `vendorHash` line that both the PR and main modified) by taking main's version. This is intentional — the hash will be stale after the merge but Workflow 1 will immediately correct it.
   e. Push the branch with the PAT: configure the remote origin URL to embed the token (`https://x-access-token:<PAT>@github.com/<repo>.git`) before pushing, then push via `git push origin <branch>`. The URL is set only for this job and never printed to the log.
5. The PAT-sourced push to each renovate branch triggers Workflow 1, which fixes the hash and re-runs `nix build`.

**Why `-X theirs` for the nix files is safe:** After the merge, the branch's `npmDepsHash` (or `vendorHash`) is set to main's current value, which was computed against main's current lockfile. The merged branch now has the *combined* lockfile (both PRs' dep changes). Workflow 1 then runs `nix build`, sees a hash mismatch for the combined lockfile, extracts the correct hash, and commits it. The final state on the branch is: combined lockfile + hash that matches the combined lockfile.

**Scope:** Only renovate PRs are rebased. Non-renovate PRs (feature branches, etc.) are the author's responsibility. Filtering by `app/renovate` keeps the blast radius minimal.

**Failure handling:** If a merge fails for a reason other than a resolvable conflict (e.g., file deletion conflict), the step logs the error and continues to the next PR. The failed PR is left for manual intervention; it will be retried on the next main push that changes a lock file.

## Interaction Between the Two Workflows

```
[renovate opens PR]
        │
        ▼
[Nix Build (W1) runs on PR branch]
        │ nix build fails: hash mismatch
        ▼
[W1: extract got: hashes, patch .nix, commit, push, re-run nix build]
        │ nix build succeeds
        ▼
[PR passes Nix Build ✓]

[PR#1 merges to main]
        │ push to main changes package-lock.json
        ▼
[W2: finds open renovate PRs]
        │ for each PR: git merge -X theirs origin/main, push with PAT
        ▼
[Nix Build (W1) re-triggered on each renovate PR branch]
        │ nix build fails: hash mismatch (merged lockfile ≠ stale hash)
        ▼
[W1: extract got: hashes, patch .nix, commit, push, re-run nix build]
        │ nix build succeeds
        ▼
[All renovate PRs pass Nix Build ✓, up-to-date with main]
```

## What Is Not Covered

- **Branch protection / merge requirements:** This design does not enforce "require up-to-date branches" in GitHub branch protection. The combination of W1 + W2 ensures that renovate PRs are kept current and their hashes are correct, but it does not block a stale non-renovate PR from being merged. That is a deliberate scope limit.
- **Go dep + npm dep in same PR:** Handled naturally — both hash mismatch lines appear in the `nix build` log and both are fixed by W1.
- **Nix flake input updates:** `flake.lock` changes are already in the W1 trigger path. However, flake input updates do not affect `npmDepsHash` or `vendorHash`, so W1 would pass through without modification.

## Files Changed

| File | Change |
|---|---|
| `.github/workflows/nix.yaml` | Add git config, hash-fix step, and re-run logic after failed build |
| `.github/workflows/rebase-renovate-prs.yaml` | New workflow |
