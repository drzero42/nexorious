# Nightly Dev Builds & Image Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch `build-push.yaml` from push-triggered to nightly-scheduled with a manual-dispatch escape hatch, add a skip-if-already-built pre-check, retain only ~30 dev image and ~30 dev chart versions, and eliminate the duplicate test run that fires on every push to `main`.

**Architecture:** All changes happen inside `.github/workflows/build-push.yaml`. The workflow's `on:` triggers swap `push: main` for `schedule: '0 2 * * *'` + `workflow_dispatch` (release stays). The current `test` job (a `workflow_call` invocation of `test.yaml`) is deleted entirely, removing the second test run — `test.yaml` already triggers on push directly. A new `pre-check` job runs before `build-push` and outputs `should_build`; on the nightly schedule it asks the GitHub Packages REST API whether HEAD's short SHA already has a tagged image and skips the downstream chain if so. A new `cleanup` job runs in parallel with `build-push-chart` after `build-push` succeeds (non-release only) and uses `actions/delete-package-versions@v5` to keep the 30 newest dev image versions (`nexorious`) and 30 newest dev chart versions (`charts/nexorious`), protecting semver versions and the floating `dev` / `0.0.0-dev` tags via the `ignore-versions` regex.

**Tech Stack:** GitHub Actions YAML, GitHub Packages REST API (called via `gh api`), `actions/delete-package-versions@v5`.

**Spec:** [docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md](../specs/2026-05-20-nightly-builds-image-retention-design.md) (committed at `39a7f715`)

**Branch:** `nightly-builds-retention` (create in Task 1, Step 1)

**Verification notes for the executor:**
- Neither `actionlint` nor `yamllint` is in `devenv`. Use `uvx yamllint <file>` to lint (uv is in devenv); this pulls yamllint via the Python-package cache and runs it. As a secondary semantic check, `uvx --from actionlint-py actionlint` runs the actual GitHub Actions linter. If `uvx` cannot fetch a package (offline), fall back to careful manual review of the diff and rely on GitHub's own parser when the PR is opened (a malformed YAML surfaces as a "workflow file issue" annotation on the next push).
- A GitHub Actions workflow cannot be executed locally. The final functional verification is performed AFTER merging to `main`: trigger the workflow manually via `workflow_dispatch` from the Actions tab to confirm the new shape works end-to-end. The plan calls this out as a post-merge step rather than a blocking task.
- `test.yaml` is **not modified** by this plan. The fix for the double test run is entirely a side-effect of removing the `workflow_call` invocation from `build-push.yaml`.

---

## Task 1: Replace triggers and remove the duplicate `test` job

**Files:**
- Modify: `.github/workflows/build-push.yaml` (lines 4-18: the `on:` block and the `test` job + `needs: test`)

**Rationale:** Smallest, most contained change. Removes the push trigger that causes the duplicate test run, adds the new triggers, and removes the now-unused `test` job. This task alone, once merged, fixes the immediate "tests run twice" wastage. Pre-check and cleanup come in later tasks so each task produces a coherent commit.

- [ ] **Step 1: Create the feature branch from latest `main`**

Run (from repo root):
```bash
git checkout main && git pull --ff-only && git checkout -b nightly-builds-retention && git status --short && git branch --show-current
```
Expected: working tree clean, branch printed as `nightly-builds-retention`. If `git pull` fails or the tree is dirty, stop and resolve — every task in this plan assumes a clean tree and the right branch.

- [ ] **Step 2: Record the baseline shape of the file**

Run (from repo root):
```bash
wc -l .github/workflows/build-push.yaml && grep -nE '^(on:|jobs:|  [a-z][a-z-]*:|    name:|    needs:|    uses:|    runs-on:)' .github/workflows/build-push.yaml
```
Expected: 125 lines total, with `on:` at line 4, jobs `test:` at line 11, `build-push:` at line 15 (with `needs: test`), and `build-push-chart:` at line 63 (with `needs: build-push`). If this differs, the file has changed since the spec was written — stop and re-read the spec before proceeding.

- [ ] **Step 3: Rewrite the `on:` block**

Edit `.github/workflows/build-push.yaml` and replace lines 4-9 (the `on:` block):

Old content to replace:
```yaml
on:
  push:
    branches: [main]
  release:
    types: [published]
```

New content:
```yaml
on:
  schedule:
    # Nightly at 02:00 UTC. The exact minute is intentionally not :00 in GitHub's
    # docs ("we recommend not running on the hour, since traffic spikes there"),
    # but :00 is fine for a low-traffic repo and matches the spec verbatim.
    - cron: '0 2 * * *'
  workflow_dispatch:
  release:
    types: [published]
```

The keys are quoted (`'0 2 * * *'`) because `*` is a YAML reserved character and an unquoted value would be a YAML parse error. `workflow_dispatch:` is intentionally a bare key with no value — that's the correct shape for the manual-trigger escape hatch.

- [ ] **Step 4: Delete the `test` job and the `needs: test` on `build-push`**

In the same file, delete the entire `test` job (the four-line block):
```yaml
  test:
    name: Run Tests
    uses: ./.github/workflows/test.yaml

```
(Note the trailing blank line — delete it so the file has exactly one blank line between `jobs:` and the first remaining job.)

Then, on the `build-push` job, delete the `needs: test` line so the job header looks like:
```yaml
  build-push:
    name: Build and Push
    runs-on: ubuntu-latest

    permissions:
      contents: read
      packages: write
```

Do **not** touch the `build-push-chart` job's `needs: build-push` — that one stays.

- [ ] **Step 5: Lint the workflow file**

Run (from repo root):
```bash
uvx yamllint -d "{extends: default, rules: {line-length: disable, document-start: disable, truthy: {check-keys: false}}}" .github/workflows/build-push.yaml
```
Expected: exit code 0, no output. The disabled rules are noise on GitHub Actions files (long `run:` blocks; missing `---` document-start markers in some files; `on:` parsing as boolean `true` which is harmless).

If yamllint flags real issues (indentation, mapping errors, duplicated keys), fix them before continuing. If `uvx` is offline, skip and rely on careful diff review plus GitHub's own parser on push.

Then run actionlint for the GitHub Actions-aware semantic check:
```bash
uvx --from actionlint-py actionlint .github/workflows/build-push.yaml
```
Expected: exit code 0, no output. If `actionlint-py` cannot be fetched, log the failure and continue — the plan has tasks that won't pass actionlint until Task 2 is also done (the `if:` referencing pre-check), so this check is best-effort.

- [ ] **Step 6: Inspect the diff visually**

Run (from repo root):
```bash
git diff .github/workflows/build-push.yaml
```
Confirm exactly these changes:
1. The `on:` block has `schedule` (with `- cron: '0 2 * * *'`) and `workflow_dispatch:` added, `push:` removed; `release:` retained.
2. The whole `test:` job (4 lines + trailing blank) is gone.
3. `build-push:` no longer has `needs: test`.
4. Nothing else is touched.

If any other line changed (e.g. unrelated whitespace), revert and re-do the edits — task commits must be atomic.

- [ ] **Step 7: Commit**

Run (from repo root):
```bash
git add .github/workflows/build-push.yaml
git commit -m "ci(build-push): switch to nightly schedule, remove duplicate test job

- Replace 'push: main' trigger with 'schedule: 0 2 * * *' and 'workflow_dispatch'
- Release trigger unchanged
- Drop the 'test' job (workflow_call to test.yaml) and 'needs: test' from build-push;
  test.yaml still runs on every push via its own direct trigger, so tests now run
  exactly once per push instead of twice

Refs: docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md"
```

Expected: a single new commit on `nightly-builds-retention`.

---

## Task 2: Add the `pre-check` skip-if-already-built job

**Files:**
- Modify: `.github/workflows/build-push.yaml` (insert a new job before `build-push`; add `needs`/`if` to `build-push`)

**Rationale:** Saves a build minute when the schedule fires but no new commits landed since the previous run. The job is a no-op for `workflow_dispatch` and `release` (always builds) and only hits the API on `schedule`.

- [ ] **Step 1: Insert the `pre-check` job**

Edit `.github/workflows/build-push.yaml`. Insert the following block immediately after the `jobs:` line (i.e. as the first job in the file, before `build-push`):

```yaml
  pre-check:
    name: Pre-check
    runs-on: ubuntu-latest
    outputs:
      should_build: ${{ steps.check.outputs.should_build }}
    steps:
      - name: Checkout code
        uses: actions/checkout@v6

      - name: Determine if a build is needed
        id: check
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          set -euo pipefail
          # Non-schedule triggers always build. workflow_dispatch is the manual
          # escape hatch; release builds must always run regardless of API state.
          if [[ "${{ github.event_name }}" != "schedule" ]]; then
            echo "Trigger=${{ github.event_name }} -> always build"
            echo "should_build=true" >> "$GITHUB_OUTPUT"
            exit 0
          fi

          SHORT_SHA=$(git rev-parse --short HEAD)
          echo "HEAD short SHA: $SHORT_SHA"

          # Query GHCR for any existing image version whose tag ends with the
          # current HEAD short SHA. The tag format is YYYYMMDD-<short-sha>, so
          # matching the suffix is sufficient and tolerant of a date rollover.
          MATCHES=$(gh api \
            -H "Accept: application/vnd.github+json" \
            --paginate \
            "/users/drzero42/packages/container/nexorious/versions" \
            --jq "[.[] | (.metadata.container.tags // [])[]] | map(select(endswith(\"-${SHORT_SHA}\"))) | length")

          echo "Found ${MATCHES} existing tag(s) ending in -${SHORT_SHA}"
          if [[ "${MATCHES}" -gt 0 ]]; then
            echo "Image for HEAD already built; skipping downstream jobs."
            echo "should_build=false" >> "$GITHUB_OUTPUT"
          else
            echo "No existing image for HEAD; will build."
            echo "should_build=true" >> "$GITHUB_OUTPUT"
          fi
```

Indentation: two spaces for the job (matching siblings), four spaces for keys inside the job, six spaces for step list items, eight for step keys. Match the indentation style already in the file.

Notes on the implementation choices:
- `set -euo pipefail` — fail fast on any error in the shell snippet.
- `--paginate` — GHCR returns at most 100 versions per page; with ~30 retained dev versions plus ~N release versions we are far under the limit, but pagination future-proofs the query.
- `.metadata.container.tags // []` — guards against a version with no tags (untagged manifests can exist; without this guard `jq` errors out).
- `endswith("-${SHORT_SHA}")` — matches both `YYYYMMDD-<sha>` (dev) and any future tag formats that embed the short SHA at the end; the leading `-` prevents an accidental match against a tag that *contains* the short SHA mid-string.
- `gh api` reads its token from `GH_TOKEN`; the workflow's `GITHUB_TOKEN` has read access to public packages owned by the repo's user without any extra scopes.

- [ ] **Step 2: Wire `build-push` to depend on `pre-check`**

In the same file, change the `build-push` job header so it now reads:
```yaml
  build-push:
    name: Build and Push
    runs-on: ubuntu-latest
    needs: pre-check
    if: needs.pre-check.outputs.should_build == 'true'

    permissions:
      contents: read
      packages: write
```

The string comparison is intentional — GitHub Actions outputs are always strings, so `'true'` (quoted) is required; bare `true` would be a YAML boolean and never match.

- [ ] **Step 3: Confirm `build-push-chart` does NOT need an explicit `if`**

`build-push-chart` already has `needs: build-push`. When `build-push` is skipped (because `pre-check` says `should_build=false`), GitHub Actions skips any downstream `needs` job by default. Open the file and confirm the `build-push-chart` job header is unchanged from before this task:

```yaml
  build-push-chart:
    name: Build and Push Helm Chart
    runs-on: ubuntu-latest
    needs: build-push
```

No edit; this step is a verification step only.

- [ ] **Step 4: Lint**

Run (from repo root):
```bash
uvx yamllint -d "{extends: default, rules: {line-length: disable, document-start: disable, truthy: {check-keys: false}}}" .github/workflows/build-push.yaml && \
uvx --from actionlint-py actionlint .github/workflows/build-push.yaml
```
Expected: both exit 0. Common actionlint findings to fix if they appear:
- "shellcheck reported issue" inside the `run:` block — read the message and fix; do not silence.
- "value type should be string but got boolean" — usually means a key like `truthy` needs quoting; should not happen here because the disabled rule list above silences `on: true` parsing.

- [ ] **Step 5: Diff review**

Run (from repo root):
```bash
git diff .github/workflows/build-push.yaml
```
Expected changes only:
1. A new `pre-check` job inserted as the first job under `jobs:`.
2. `build-push` gained `needs: pre-check` and `if: needs.pre-check.outputs.should_build == 'true'`.
3. Nothing else changed (no whitespace drift, no `build-push-chart` edits, no other jobs touched).

- [ ] **Step 6: Commit**

Run (from repo root):
```bash
git add .github/workflows/build-push.yaml
git commit -m "ci(build-push): add pre-check job to skip already-built HEAD

On schedule, query GHCR for any existing image tag ending with the current
HEAD short SHA (the YYYYMMDD-<sha> dev tag format). If found, skip
downstream build-push and build-push-chart jobs. workflow_dispatch and
release events always build.

Refs: docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md"
```

---

## Task 3: Add the `cleanup` job for image and chart retention

**Files:**
- Modify: `.github/workflows/build-push.yaml` (append a new job at the end of the file)

**Rationale:** Enforces the ~30-version retention for dev images and dev charts. Runs in parallel with `build-push-chart` (both depend on `build-push`) per the spec's dependency diagram. Skips on `release` events so release artifacts are never touched.

- [ ] **Step 1: Append the `cleanup` job**

Edit `.github/workflows/build-push.yaml`. Append the following block at the end of the file, after the final step of `build-push-chart`:

```yaml

  cleanup:
    name: Cleanup old dev versions
    runs-on: ubuntu-latest
    needs: build-push
    if: github.event_name != 'release'

    permissions:
      packages: write

    steps:
      - name: Delete old dev container image versions
        uses: actions/delete-package-versions@v5
        with:
          package-name: nexorious
          package-type: container
          min-versions-to-keep: 30
          # Never delete release images (semver: 1.2.3) or the floating 'dev' tag.
          # Everything else (YYYYMMDD-<sha> dev builds) is subject to the
          # min-versions-to-keep cap.
          ignore-versions: '^(\d+\.\d+\.\d+|dev)$'

      - name: Delete old dev Helm chart versions
        uses: actions/delete-package-versions@v5
        with:
          package-name: charts/nexorious
          package-type: container
          min-versions-to-keep: 30
          # Protect release charts (semver) and the moving '0.0.0-dev' tag.
          # 0.0.0-dev-YYYYMMDD-<sha> dev versions are subject to the cap.
          ignore-versions: '^(\d+\.\d+\.\d+|0\.0\.0-dev)$'
```

Notes on the implementation choices:
- `package-name: charts/nexorious` — the Helm chart is pushed to `oci://ghcr.io/drzero42/charts` with a chart name of `nexorious` (see existing line ~115 of the file), so the GHCR package path is `charts/nexorious`. `package-type` is still `container` for OCI Helm charts — GHCR exposes them under the container API.
- `min-versions-to-keep: 30` — the action interprets this against the non-ignored versions; semver releases and the floating tags are excluded from the count by `ignore-versions`.
- `ignore-versions` is an anchored regex: `^(\d+\.\d+\.\d+|dev)$` matches exactly a semver triple OR the literal string `dev`. The pipe is regex alternation; the surrounding `^...$` ensures `dev-extra` would NOT be ignored.
- The cleanup job runs in **parallel** with `build-push-chart` (both `needs: build-push`). This matches the dependency graph in the spec. A consequence: on a given run the chart cleanup may execute before the new chart is pushed, in which case the new push adds a 31st version after cleanup leaves 30. This drift is bounded at +1 and is acceptable per the spec ("~30 versions").

- [ ] **Step 2: Lint**

Run (from repo root):
```bash
uvx yamllint -d "{extends: default, rules: {line-length: disable, document-start: disable, truthy: {check-keys: false}}}" .github/workflows/build-push.yaml && \
uvx --from actionlint-py actionlint .github/workflows/build-push.yaml
```
Expected: both exit 0.

- [ ] **Step 3: Verify the final job graph matches the spec**

Run (from repo root):
```bash
grep -nE '^  [a-z][a-z-]*:|^    name:|^    needs:|^    if:' .github/workflows/build-push.yaml
```
Expected (line numbers approximate, ordering is what matters): four jobs in this order with these `needs`/`if` lines:
1. `pre-check:` — no `needs`, no `if`
2. `build-push:` — `needs: pre-check`, `if: needs.pre-check.outputs.should_build == 'true'`
3. `build-push-chart:` — `needs: build-push`, no `if`
4. `cleanup:` — `needs: build-push`, `if: github.event_name != 'release'`

This mirrors the spec's dependency graph:
```
pre-check
    └── build-push (if should_build == true)
            ├── cleanup (non-release only)
            └── build-push-chart
```

- [ ] **Step 4: Diff review of this task only**

Run (from repo root):
```bash
git diff -- .github/workflows/build-push.yaml
```
This shows uncommitted changes since Task 2's commit. Confirm the only change is the appended `cleanup` job. No earlier sections changed.

- [ ] **Step 5: Commit**

Run (from repo root):
```bash
git add .github/workflows/build-push.yaml
git commit -m "ci(build-push): add cleanup job to retain ~30 dev image and chart versions

Adds a 'cleanup' job that runs after build-push (non-release only, in parallel
with build-push-chart) and uses actions/delete-package-versions@v5 to keep
the 30 newest dev versions of:

- ghcr.io/drzero42/nexorious (container image)
- ghcr.io/drzero42/charts/nexorious (Helm chart)

Semver release versions and the floating 'dev' / '0.0.0-dev' tags are
protected via the ignore-versions regex.

Refs: docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md"
```

---

## Task 4: Commit the plan, open the PR, post-merge verification

**Files:**
- Add: this plan file at `docs/superpowers/plans/2026-05-20-nightly-builds-image-retention.md` (already created by the planning step; commit it here)

**Rationale:** Per `CLAUDE.md`: "Plan files: docs/superpowers/plans/ is tracked — always commit the plan file on the feature branch". The PR is opened after the plan is committed so reviewers have all the context.

- [ ] **Step 1: Commit the plan file**

Run (from repo root):
```bash
git add docs/superpowers/plans/2026-05-20-nightly-builds-image-retention.md
git commit -m "docs: add nightly builds & image retention implementation plan"
```

If `git status` says the file is already tracked and unmodified (i.e. it was somehow committed earlier), skip this step.

- [ ] **Step 2: Push the branch**

Run (from repo root):
```bash
git push -u origin nightly-builds-retention
```

- [ ] **Step 3: Open the PR**

Run (from repo root):
```bash
gh pr create --title "ci: nightly dev builds + image retention" --body "$(cat <<'EOF'
## Summary

Replaces the per-push Docker/Helm build with a nightly schedule, adds a manual-dispatch escape hatch, and introduces 30-version retention for dev images and dev Helm charts. Also eliminates the duplicate test run that was firing on every push to `main`.

See [docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md](docs/superpowers/specs/2026-05-20-nightly-builds-image-retention-design.md) for the full design and [docs/superpowers/plans/2026-05-20-nightly-builds-image-retention.md](docs/superpowers/plans/2026-05-20-nightly-builds-image-retention.md) for the per-task breakdown.

### Job graph after this change
```
pre-check
    └── build-push (if should_build == true)
            ├── cleanup (non-release only)
            └── build-push-chart
```

## Test plan

- [ ] CI on this PR is green (test.yaml runs once via its own push trigger; build-push.yaml does not run on PRs)
- [ ] After merge, trigger the workflow manually via `gh workflow run build-push.yaml --ref main` and confirm: pre-check outputs `should_build=true`, the image and chart are pushed, and the cleanup job exits 0.
- [ ] Re-trigger immediately via `workflow_dispatch` — the manual dispatch still forces a build (does NOT skip), confirming the pre-check short-circuit is `schedule`-only.
- [ ] Wait for the first nightly run at 02:00 UTC (after the workflow has been on `main` long enough for a scheduled trigger). If HEAD already has an image from the manual dispatch, confirm `pre-check` outputs `should_build=false` and downstream jobs are skipped.
- [ ] After ~30 dev builds have accumulated (or via a manual test push of a dummy commit and re-dispatch a few times), confirm the GHCR UI shows no more than ~31 dev image and ~31 dev chart versions, with semver and the floating tags untouched.
EOF
)"
```

Note: `gh pr create` will report the PR URL; capture it and report it to the user.

- [ ] **Step 4: Post-merge verification (manual; record in a comment on the PR or in the merge-back-to-`main` notes)**

This step cannot be automated and is intentionally a checklist for the human merging the PR. Do not block the PR review on it.

1. Merge the PR to `main` (squash + delete branch, per CLAUDE.md).
2. From `main`, trigger the workflow manually:
   ```bash
   gh workflow run build-push.yaml --ref main
   gh run watch
   ```
   Confirm: `pre-check` runs, outputs `should_build=true`, `build-push` and `build-push-chart` succeed, and `cleanup` succeeds.
3. Re-trigger via `gh workflow run build-push.yaml --ref main` — confirm `pre-check` still outputs `should_build=true` (because `workflow_dispatch` is the escape hatch and never skips).
4. After the nightly schedule fires (at 02:00 UTC the night after merge), confirm via the Actions tab that either (a) the build ran because new commits landed, or (b) `pre-check` reported `should_build=false` and downstream jobs were skipped.
5. After a few dev builds have accumulated, open the GHCR package pages for `nexorious` and `charts/nexorious` and confirm the version counts have stabilised near 30 and the semver / floating-tag versions are intact.

If any of these checks fail, open a follow-up issue rather than reverting — the workflow is recoverable forward (e.g. by adjusting the regex or the API query) without taking nightly builds offline.

---

## Out of scope (explicit non-changes)

These were considered and intentionally deferred or rejected during the spec phase. Do not implement them in this plan:

- **Changes to `test.yaml`** — the duplicate-test fix is achieved entirely by removing the `workflow_call` invocation from `build-push.yaml`. `test.yaml` itself is correct as-is.
- **Changes to release builds** — the `release: types: [published]` trigger is preserved verbatim, including the chart-side `if: github.event_name != 'release'` guards that keep the moving dev tags out of release pushes.
- **Changes to image tagging conventions** — `YYYYMMDD-<sha>` for dev images and `0.0.0-dev-YYYYMMDD-<sha>` for dev charts remain unchanged. The cleanup regex is built around these existing conventions.
- **Auto-deletion of untagged manifest blobs** — `actions/delete-package-versions` operates on versions, not blobs. Untagged-blob garbage collection is a GHCR-side concern outside this plan.
