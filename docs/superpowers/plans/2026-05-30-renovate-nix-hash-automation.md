# Renovate Nix Hash Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two GitHub Actions workflows — one that self-heals stale Nix hashes on renovate PR branches, and one that rebases open renovate PRs after a lock-file-changing merge to main — so that `Nix Build` always passes on both PRs and main.

**Architecture:** `nix.yaml` is modified to detect `hash mismatch` errors in `nix build` output, extract the correct `got:` hashes, patch `nix/frontend.nix` and `nix/package.nix`, commit back to the PR branch, and re-run the build in the same job. A new `rebase-renovate-prs.yaml` workflow triggers on lock-file pushes to main, merges main into every open `renovate/*` PR branch using `-X theirs` (to resolve nix-file conflicts), and pushes with `RELEASE_PLEASE_TOKEN` so the push triggers the self-healing workflow on each updated branch.

**Tech Stack:** GitHub Actions, Bash, Nix, `gh` CLI (pre-installed on ubuntu-latest), `jq` (pre-installed on ubuntu-latest), `RELEASE_PLEASE_TOKEN` secret (already present in repo).

---

## Files

| File | Action |
|---|---|
| `.github/workflows/nix.yaml` | Modify — add `permissions`, change checkout `ref`, replace `Build package` step with self-healing script |
| `.github/workflows/rebase-renovate-prs.yaml` | Create — post-merge rebase workflow |

---

### Task 1: Create feature branch

- [ ] **Create and switch to feature branch**

```bash
git checkout -b feat/renovate-nix-hash-automation
```

---

### Task 2: Implement self-healing Nix Build (W1)

`actions/checkout` for a `pull_request` event checks out a synthetic merge commit (`refs/pull/N/merge`), not the real PR branch HEAD. Committing on top of that and pushing to the PR branch would fail with a non-fast-forward error. The fix is to check out the actual PR branch HEAD (`github.head_ref`) on PR events, and fall back to the pushed SHA on push-to-main events.

**File:** `.github/workflows/nix.yaml`

- [ ] **Replace the entire contents of `.github/workflows/nix.yaml` with the following**

```yaml
---
name: Nix Build

on:
  pull_request:
    branches: [main]
    paths:
      - 'go.sum'
      - 'ui/frontend/package-lock.json'
      - 'nix/**'
      - 'flake.nix'
      - 'flake.lock'
  push:
    branches: [main]
    paths:
      - 'go.sum'
      - 'ui/frontend/package-lock.json'
      - 'nix/**'
      - 'flake.nix'
      - 'flake.lock'

jobs:
  nix-build:
    name: Nix Build
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v6
        with:
          # On pull_request events, check out the actual PR branch HEAD (not the
          # synthetic merge commit) so we can commit a hash fix and push it back.
          ref: ${{ github.event_name == 'pull_request' && github.head_ref || github.sha }}

      - name: Install Nix
        uses: cachix/install-nix-action@v31
        with:
          github_access_token: ${{ secrets.GITHUB_TOKEN }}

      - name: Build package
        run: |
          set +e
          nix build .#nexorious 2>&1 | tee /tmp/nix-build.log
          BUILD_EXIT=${PIPESTATUS[0]}
          set -e

          if [ "$BUILD_EXIT" -eq 0 ]; then
            exit 0
          fi

          # On push-to-main or non-hash failures, fail immediately — we cannot
          # commit directly to a protected branch.
          if [ "${{ github.event_name }}" != "pull_request" ] || ! grep -q "hash mismatch" /tmp/nix-build.log; then
            echo "Build failed (not a hash mismatch or not a PR)."
            exit "$BUILD_EXIT"
          fi

          echo "Hash mismatch detected — patching nix files..."

          mapfile -t old_hashes < <(grep "specified:" /tmp/nix-build.log | awk '{print $NF}')
          mapfile -t new_hashes < <(grep "got:" /tmp/nix-build.log | awk '{print $NF}')

          for i in "${!old_hashes[@]}"; do
            echo "  ${old_hashes[$i]} → ${new_hashes[$i]}"
            sed -i "s|${old_hashes[$i]}|${new_hashes[$i]}|g" nix/frontend.nix nix/package.nix
          done

          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add nix/frontend.nix nix/package.nix
          git diff --cached --quiet && { echo "No hash changes to commit."; exit 1; }
          git commit -m "chore(nix): update hashes for ${{ github.head_ref }}"
          git push origin HEAD:${{ github.head_ref }}

          echo "Hashes fixed. Re-running build..."
          nix build .#nexorious
```

- [ ] **Commit**

```bash
git add .github/workflows/nix.yaml
git commit -m "ci: self-healing nix hash fix on renovate PRs"
```

---

### Task 3: Implement post-merge renovate rebase (W2)

**File:** `.github/workflows/rebase-renovate-prs.yaml` (new)

When PR#1 and PR#2 both run the self-healing workflow, they each write a different value to the same `npmDepsHash` line in `nix/frontend.nix`. After PR#1 merges, merging main into PR#2's branch will hit a git conflict on that line. `git merge -X theirs` resolves it by taking main's version, which is intentionally stale — W1 then immediately re-runs on the pushed branch and replaces it with the correct value for the combined lockfile.

The push in this workflow must use `RELEASE_PLEASE_TOKEN` (not `GITHUB_TOKEN`). Pushes made with `GITHUB_TOKEN` do not trigger downstream workflow runs; pushes made with a PAT do, which is what causes W1 to self-heal each branch.

**File:** `.github/workflows/rebase-renovate-prs.yaml`

- [ ] **Create `.github/workflows/rebase-renovate-prs.yaml` with the following contents**

```yaml
---
name: Rebase Renovate PRs

on:
  push:
    branches: [main]
    paths:
      - 'go.sum'
      - 'ui/frontend/package-lock.json'

jobs:
  rebase:
    name: Rebase open renovate PRs onto main
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read

    steps:
      - name: Checkout main
        uses: actions/checkout@v6
        with:
          # Full history is required so git can compute merge bases when merging
          # PR branches that diverged from an older main commit.
          fetch-depth: 0

      - name: Rebase open renovate PRs
        env:
          GH_TOKEN: ${{ secrets.RELEASE_PLEASE_TOKEN }}
        run: |
          set -euo pipefail

          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"

          # Inject the PAT into the remote URL so git push triggers downstream
          # workflows (GITHUB_TOKEN pushes do not trigger workflow runs).
          git remote set-url origin \
            "https://x-access-token:${GH_TOKEN}@github.com/${{ github.repository }}.git"

          # Find open PRs whose branch names start with renovate/.
          mapfile -t pr_entries < <(
            gh pr list --state open --json number,headRefName --limit 100 |
            jq -r '.[] | select(.headRefName | startswith("renovate/")) | "\(.number) \(.headRefName)"'
          )

          if [ "${#pr_entries[@]}" -eq 0 ]; then
            echo "No open renovate PRs found."
            exit 0
          fi

          for entry in "${pr_entries[@]}"; do
            pr_number="${entry%% *}"
            branch="${entry#* }"
            echo "--- PR #${pr_number}: ${branch} ---"

            # Fetch the tip of the PR branch.
            if ! git fetch origin "${branch}"; then
              echo "  Could not fetch ${branch}; skipping."
              continue
            fi

            # Skip if the branch already contains the latest main commit.
            if git merge-base --is-ancestor origin/main "origin/${branch}"; then
              echo "  Already up to date; skipping."
              continue
            fi

            # Check out a local copy of the PR branch.
            git checkout -B "${branch}" "origin/${branch}"

            # Merge main. -X theirs resolves any conflict (including the
            # npmDepsHash / vendorHash line both this PR and main modified) by
            # taking main's version. W1 will replace it with the correct hash
            # for the combined lockfile immediately after this push.
            if git merge origin/main -X theirs --no-edit \
                -m "chore: merge main into ${branch}"; then
              git push origin "${branch}:${branch}"
              echo "  Pushed updated branch for PR #${pr_number}."
            else
              echo "  Unresolvable conflict for PR #${pr_number}; skipping."
              git merge --abort 2>/dev/null || true
            fi

            # Return to main before processing the next PR.
            git checkout main
          done
```

- [ ] **Commit**

```bash
git add .github/workflows/rebase-renovate-prs.yaml
git commit -m "ci: rebase open renovate PRs after lock-file merge to main"
```

---

### Task 4: Commit plan file, push, open PR

- [ ] **Commit the plan file on the feature branch**

```bash
git add docs/superpowers/plans/2026-05-30-renovate-nix-hash-automation.md
git commit -m "docs: add renovate nix hash automation plan"
```

- [ ] **Push and open PR**

```bash
git push -u origin feat/renovate-nix-hash-automation
gh pr create \
  --title "ci: automate nix hash updates for renovate PRs" \
  --body "Adds two workflows:

- **W1 (nix.yaml):** Self-heals stale \`npmDepsHash\`/\`vendorHash\` on renovate PR branches by parsing \`nix build\` error output, patching the hash, committing back to the branch, and re-running the build in the same job.
- **W2 (rebase-renovate-prs.yaml):** After any lock-file-changing merge to main, merges main into every open \`renovate/*\` PR branch (\`-X theirs\` to resolve nix-file conflicts) and pushes with \`RELEASE_PLEASE_TOKEN\` so W1 re-triggers on each branch.

Closes the scenario where two concurrent renovate PRs each fix their hash in isolation and the second one corrupts main after merging onto an already-advanced lockfile.

Spec: \`docs/superpowers/specs/2026-05-30-renovate-nix-hash-automation-design.md\`"
```

---

## Self-review notes

- **W1 checkout `ref`:** Without the `ref` override, `actions/checkout` on a `pull_request` event checks out the synthetic merge commit, and `git push origin HEAD:${{ github.head_ref }}` would fail (non-fast-forward). The `ref` expression fixes this.
- **`git diff --cached --quiet && exit 1`:** If no hashes were actually substituted (shouldn't happen on a genuine hash-mismatch failure, but defensive), the job fails rather than re-running an unchanged build.
- **W2 PAT vs GITHUB_TOKEN:** Explicitly noted in the task why `RELEASE_PLEASE_TOKEN` is required; forgetting this is the most likely implementation mistake.
- **W2 `-X theirs` safety:** Explained in the task body — the stale hash left by the merge is intentional and immediately corrected by W1 triggering on the pushed branch.
- **Both workflows tested in CI** — the real integration test is opening a renovate-style PR and verifying the Nix Build check self-heals. No unit tests are warranted for these workflow scripts beyond inspecting the bash logic manually, which the task steps support.
