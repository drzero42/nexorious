# Release Process Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire up release-please to drive on-demand, Conventional-Commits-driven releases that publish versioned + `latest` container images and Helm charts, with auto-updated install pins in `Chart.yaml` and `docker-compose.yml`.

**Architecture:** A new `release-please.yaml` workflow runs the `googleapis/release-please-action` on every push to `main` and on a daily schedule. It maintains a long-running "Release PR" that bumps the version in `.release-please-manifest.json`, prepends a `CHANGELOG.md` entry, and updates anchor-commented version strings in `deploy/helm/Chart.yaml` and `deploy/docker/docker-compose.yml`. Merging the Release PR creates a `vX.Y.Z` tag and GitHub Release, which fires the existing `build-push.yaml` `release: published` path. That path also gets two small additions: a `latest` image tag and a floating-`latest` Helm chart push.

**Tech Stack:** GitHub Actions, [release-please](https://github.com/googleapis/release-please) v4, Helm 3 (OCI), Docker Buildx.

**Spec:** [2026-05-20-release-process-design.md](../specs/2026-05-20-release-process-design.md)

**Branch:** `release-process-design` (already created; spec already committed there).

---

## Notes for the Implementer

- All file edits in tasks 1–6 land on the same feature branch as the spec doc.
- The full bootstrap PR is opened in Task 7. Until that PR is merged to `main`, nothing changes in CI behavior.
- After merging the bootstrap PR, release-please opens its first Release PR proposing `0.1.0` (because the bootstrap commit body carries a `Release-As: 0.1.0` footer — see Task 7). Merging that second PR cuts the inaugural release.
- The temporary state introduced by Task 2 — `Chart.yaml` and `docker-compose.yml` referencing the placeholder version `0.0.0` (no such image exists in GHCR) — exists only on the bootstrap feature branch and resolves the moment the first Release PR is merged. We accept this brief window because nobody deploys from feature branches.
- release-please's `generic` updater finds the first SemVer-shaped substring (`\d+\.\d+\.\d+(?:-\S+)?`) on a line marked with `# x-release-please-version` and replaces it. The exact comment syntax matters; use it verbatim.

---

## Task 1: Add release-please configuration

**Files:**
- Create: `release-please-config.json`
- Create: `.release-please-manifest.json`

- [ ] **Step 1: Create `release-please-config.json`**

Write to `release-please-config.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "release-type": "simple",
  "bump-minor-pre-major": true,
  "bump-patch-for-minor-pre-major": true,
  "include-component-in-tag": false,
  "separate-pull-requests": false,
  "packages": {
    ".": {
      "release-type": "simple",
      "package-name": "nexorious",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [
        {
          "type": "generic",
          "path": "deploy/helm/Chart.yaml"
        },
        {
          "type": "generic",
          "path": "deploy/docker/docker-compose.yml"
        }
      ]
    }
  }
}
```

What each setting does:
- `release-type: simple` — the version is tracked in the manifest, not in a language-specific file like `package.json`.
- `bump-minor-pre-major: true` — breaking changes during 0.x bump the minor digit (0.4.x → 0.5.0) rather than going to 1.0.
- `bump-patch-for-minor-pre-major: true` — `feat:` commits during 0.x bump patch (0.4.0 → 0.4.1) instead of minor. Reserves the minor digit for breaking changes during stabilization.
- `include-component-in-tag: false` + `separate-pull-requests: false` — single-package mode; tags are plain `vX.Y.Z` with no component prefix and a single Release PR.
- `extra-files` with `type: generic` — release-please will scan these files for `# x-release-please-version` anchor comments (added in Task 2) and update the version-looking substring on each anchored line.

- [ ] **Step 2: Create `.release-please-manifest.json`**

Write to `.release-please-manifest.json`:

```json
{
  ".": "0.0.0"
}
```

The manifest is seeded at `0.0.0`. The bootstrap PR's `Release-As: 0.1.0` footer (added in Task 7) overrides the next computed version, so the first Release PR will propose `0.1.0` regardless of what release-please would otherwise compute from commit history.

- [ ] **Step 3: Validate JSON syntax**

Run:
```bash
jq . release-please-config.json > /dev/null
jq . .release-please-manifest.json > /dev/null
echo "JSON OK"
```
Expected: `JSON OK` with no errors.

- [ ] **Step 4: Commit**

```bash
git add release-please-config.json .release-please-manifest.json
git commit -m "ci: add release-please configuration and manifest"
```

---

## Task 2: Add anchor comments and version placeholders to install-pin files

**Files:**
- Modify: `deploy/helm/Chart.yaml` (lines 5 and 6–10)
- Modify: `deploy/docker/docker-compose.yml` (line 36)

- [ ] **Step 1: Modify `deploy/helm/Chart.yaml`**

Replace lines 5–10 (the `version:` and `appVersion:` block including its existing comment) with this exact content:

```yaml
version: 0.0.0 # x-release-please-version
# appVersion is kept in sync with the application's release version by
# release-please. Defaults to the released image tag; real deployments may
# override controllers.nexorious.containers.main.image.tag (and the matching
# initContainers.migrate.image.tag) to pin a specific release explicitly.
appVersion: "0.0.0" # x-release-please-version
```

Rationale: `version` was `0.1.0` and `appVersion` was `"latest"`. release-please's generic updater needs an existing SemVer-shaped substring on the line to replace, so both are reset to `0.0.0`. The comment text was rewritten because the old comment referred to `"latest"` (no longer applicable). The `# x-release-please-version` marker comment must appear on the *same line* as the value to update; release-please ignores it otherwise.

- [ ] **Step 2: Modify `deploy/docker/docker-compose.yml` line 36**

Replace:
```yaml
    image: ghcr.io/drzero42/nexorious:dev
```
with:
```yaml
    image: ghcr.io/drzero42/nexorious:0.0.0 # x-release-please-version
```

- [ ] **Step 3: Run helm lint to verify Chart.yaml still parses**

Run:
```bash
helm repo add bjw-s https://bjw-s-labs.github.io/helm-charts/ 2>/dev/null || true
helm dependency build deploy/helm/
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```
Expected: `1 chart(s) linted, 0 chart(s) failed`. A warning about icon is fine; failures are not. If `helm` is not on PATH in your environment, install it first (`devenv shell` provides it, or `nix run nixpkgs#kubernetes-helm --`).

- [ ] **Step 4: Verify docker-compose.yml still parses**

Run:
```bash
docker compose -f deploy/docker/docker-compose.yml config --quiet
echo "compose OK"
```
Expected: `compose OK`. It will complain about missing env vars — that's fine; we only need YAML validity. If `docker compose config` fails on env vars even with `--quiet`, run instead:
```bash
POSTGRES_PASSWORD=x SECRET_KEY=x IGDB_CLIENT_ID=x IGDB_CLIENT_SECRET=x \
  docker compose -f deploy/docker/docker-compose.yml config --quiet && echo "compose OK"
```

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/Chart.yaml deploy/docker/docker-compose.yml
git commit -m "ci: add release-please anchor comments to chart and compose pins"
```

---

## Task 3: Add the release-please workflow

**Files:**
- Create: `.github/workflows/release-please.yaml`

- [ ] **Step 1: Create the workflow**

Write to `.github/workflows/release-please.yaml`:

```yaml
---
name: Release Please

on:
  push:
    branches: [main]
  schedule:
    # Daily reconciliation at 06:00 UTC. Catches edge cases where a push to
    # main didn't trigger the action (e.g. concurrent merges). The Release PR
    # is updated in-place; this is idempotent.
    - cron: '0 6 * * *'
  workflow_dispatch:

jobs:
  release-please:
    name: Release Please
    runs-on: ubuntu-latest

    permissions:
      contents: write
      pull-requests: write

    steps:
      - name: Run release-please
        uses: googleapis/release-please-action@v4
        with:
          config-file: release-please-config.json
          manifest-file: .release-please-manifest.json
```

What this does:
- `contents: write` is required to push the version-bump branch and create the release tag.
- `pull-requests: write` is required to open and update the Release PR.
- No checkout step is needed — `release-please-action` uses the GitHub API directly.

- [ ] **Step 2: Verify YAML syntax**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release-please.yaml'))" && echo "YAML OK"
```
Expected: `YAML OK`.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release-please.yaml
git commit -m "ci: add release-please workflow"
```

---

## Task 4: Update `build-push.yaml` for release `latest` tags

**Files:**
- Modify: `.github/workflows/build-push.yaml`

- [ ] **Step 1: Add `latest` image tag on release events**

In `.github/workflows/build-push.yaml`, find the `Extract image metadata` step (currently lines 89–99). Replace the `tags:` block:

```yaml
          tags: |
            # Release tags: v1.2.3 → 1.2.3
            type=semver,pattern={{version}}
            # Main branch builds: YYYYMMDD-<short-sha> + dev
            type=raw,value={{date 'YYYYMMDD'}}-{{sha}},enable=${{ github.ref == 'refs/heads/main' }}
            type=raw,value=dev,enable=${{ github.ref == 'refs/heads/main' }}
```

with:

```yaml
          tags: |
            # Release tags: v1.2.3 → 1.2.3
            type=semver,pattern={{version}}
            # Release floating tag
            type=raw,value=latest,enable=${{ github.event_name == 'release' }}
            # Main branch builds: YYYYMMDD-<short-sha> + dev
            type=raw,value={{date 'YYYYMMDD'}}-{{sha}},enable=${{ github.ref == 'refs/heads/main' }}
            type=raw,value=dev,enable=${{ github.ref == 'refs/heads/main' }}
```

- [ ] **Step 2: Add a floating-`latest` Helm chart push on release**

In the same file, find the existing `Push Helm chart (moving dev tag)` step (currently the last step in the `build-push-chart` job). After that step, append a new step:

```yaml
      - name: Push Helm chart (moving latest tag)
        if: github.event_name == 'release'
        run: |
          PKG=$(helm package deploy/helm/ \
            --version "0.0.0-latest" \
            --app-version "${{ steps.chart-version.outputs.app_version }}" \
            | awk '{print $NF}')
          helm push "$PKG" oci://ghcr.io/drzero42/charts
```

Why `0.0.0-latest`: Helm chart versions must be valid SemVer, and `latest` alone is not. Mirroring the existing `0.0.0-dev` convention, `0.0.0-latest` is a valid SemVer prerelease identifier that sorts below all real release versions but always points at the latest release's `appVersion`. Users opt in with `helm install --version 0.0.0-latest`.

- [ ] **Step 3: Verify YAML syntax**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-push.yaml'))" && echo "YAML OK"
```
Expected: `YAML OK`.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/build-push.yaml
git commit -m "ci(build-push): publish latest tag for image and chart on release"
```

---

## Task 5: Create an empty `CHANGELOG.md`

**Files:**
- Create: `CHANGELOG.md`

- [ ] **Step 1: Create the file**

Write to `CHANGELOG.md`:

```markdown
# Changelog

All notable changes to this project will be documented in this file. The
format and version bumping are managed by [release-please](https://github.com/googleapis/release-please);
do not edit this file by hand.
```

release-please prepends entries above this header on each Release PR. The file must exist for the first Release PR to update it (release-please's generic updater for the changelog requires a preexisting file).

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: seed empty CHANGELOG.md for release-please"
```

---

## Task 6: Add Release Process section to `CLAUDE.md`

**Files:**
- Modify: `CLAUDE.md` (insert new section after the "Slumber Collection Maintenance" subsection, before the "Known Gotchas" H2)

- [ ] **Step 1: Insert the new section**

In `CLAUDE.md`, locate the blank line between the end of the `### Slumber Collection Maintenance` subsection (ends around line 202) and the `## Known Gotchas` header (around line 204). Insert this new H2 section there:

```markdown
## Release Process

Releases are produced by [release-please](https://github.com/googleapis/release-please) from the Conventional Commits on `main`. The full design is in [docs/superpowers/specs/2026-05-20-release-process-design.md](docs/superpowers/specs/2026-05-20-release-process-design.md).

### Commit message convention (MANDATORY)

All commits on `main` must follow [Conventional Commits](https://www.conventionalcommits.org/). PRs are squash-merged, so the **PR title** is the commit message release-please parses.

| Prefix | Effect (pre-1.0) | Effect (post-1.0) |
|---|---|---|
| `feat: …` | patch bump | minor bump |
| `fix: …` | patch bump | patch bump |
| `feat!: …` or `BREAKING CHANGE:` footer | **minor bump** | **major bump** |
| `chore:`, `ci:`, `docs:`, `refactor:`, `test:` | no release | no release |

During the 0.x window the minor digit is reserved for breaking changes; everything else bumps patch.

### Cutting a release

1. Wait until there is a `feat:` or `fix:` on `main` since the last release (otherwise release-please's Release PR will be empty).
2. Find the open PR titled `chore(main): release X.Y.Z` (opened by `release-please-action`).
3. Review the proposed `CHANGELOG.md` diff, `Chart.yaml` / `docker-compose.yml` version bumps.
4. Merge the Release PR. release-please creates the `vX.Y.Z` tag, publishes a GitHub Release, and `build-push.yaml` pushes the image and chart (semver tag + `latest`).

### Overrides

- Force the next release version: include `Release-As: X.Y.Z` in any commit body on `main`. release-please honors that footer.
- Skip a release after a `feat:` / `fix:` lands by mistake: close the Release PR; release-please reopens it on the next push, so to actually skip you must absorb the change into the next legitimate release.

### Promoting to 1.0.0

When ready, open a single PR that:
1. Edits `release-please-config.json` to remove (or set to `false`) `bump-minor-pre-major` and `bump-patch-for-minor-pre-major`.
2. Includes `Release-As: 1.0.0` in the commit body.

After merging that PR, release-please opens a Release PR for `1.0.0`. From the next commit onward, post-1.0 SemVer applies automatically.
```

- [ ] **Step 2: Verify the section landed in the right place**

Run:
```bash
grep -n "^## " CLAUDE.md
```
Expected output (order matters):
```
## Quick Reference
## Setup & Development
## Project Structure
## Architecture
## Testing
## Development Rules
## Release Process
## Known Gotchas
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude-md): add Release Process section"
```

---

## Task 7: Push the branch and open the bootstrap PR

**Files:** none (git + GitHub only).

- [ ] **Step 1: Push the branch**

```bash
git push -u origin release-process-design
```

- [ ] **Step 2: Open the PR with the required `Release-As` footer**

The PR description must end with a `Release-As:` footer. release-please scans the body of the squash merge commit (which by default uses the PR description), so the footer needs to be in the **PR body**, on its own line, with no extra text after it.

Run:
```bash
gh pr create --title "ci: introduce release-please release process" --body "$(cat <<'EOF'
## Summary

- Add release-please config, manifest, and workflow.
- Add `latest` image tag and floating `0.0.0-latest` chart tag on release.
- Add anchor comments + `0.0.0` placeholders in `Chart.yaml` and `docker-compose.yml` so release-please can update them.
- Seed an empty `CHANGELOG.md`.
- Document the Release Process in `CLAUDE.md`.

Design: docs/superpowers/specs/2026-05-20-release-process-design.md
Plan: docs/superpowers/plans/2026-05-20-release-process.md

## Notes for the maintainer

After this PR is squash-merged, release-please opens a "chore(main): release 0.1.0" PR. Merging that second PR cuts the inaugural release.

The `Release-As: 0.1.0` footer below forces the first release version regardless of what release-please would otherwise compute. Do **not** remove the footer before merging.

## Test plan

- [ ] After merge, `Release Please` workflow runs successfully on `main`.
- [ ] release-please opens a PR titled `chore(main): release 0.1.0`.
- [ ] The Release PR diff includes `CHANGELOG.md` entry, `.release-please-manifest.json` bumped to `0.1.0`, `Chart.yaml` `version`/`appVersion` set to `0.1.0`, `docker-compose.yml` image tag bumped to `:0.1.0`.
- [ ] Merging the Release PR creates tag `v0.1.0` and a published GitHub Release.
- [ ] `Build and Push Container Image` workflow fires on the release event and publishes `ghcr.io/drzero42/nexorious:0.1.0` and `:latest`, plus chart `0.1.0` and `0.0.0-latest`.

Release-As: 0.1.0
EOF
)"
```

The literal final line `Release-As: 0.1.0` is the part that matters; everything else is summary and test plan. Verify the rendered PR body shows the footer on its own line.

- [ ] **Step 3: Verify the PR opened correctly**

Run:
```bash
gh pr view --json url,title,body --jq '"URL: \(.url)\nTitle: \(.title)\nFooter present: \(.body | contains("Release-As: 0.1.0"))"'
```
Expected: `Footer present: true`.

The PR URL is the deliverable. Hand it to the maintainer to review.

---

## Manual smoke-test after the bootstrap PR is merged

This is what the *maintainer* does after merging — not the implementer, but documented here so the test plan is complete.

1. **Wait for the `Release Please` workflow to run on main.** Visit the Actions tab; the run should be green and produce a new PR.
2. **Inspect the Release PR.** Confirm it touches the four expected files (`CHANGELOG.md`, `.release-please-manifest.json`, `deploy/helm/Chart.yaml`, `deploy/docker/docker-compose.yml`) with `0.1.0` everywhere appropriate.
3. **Merge the Release PR via the GitHub UI.** Use the default "Squash and merge"; release-please's auto-generated body already contains the right metadata.
4. **Confirm `v0.1.0` tag and GitHub Release exist.** `git fetch --tags && git tag -l v0.1.0` locally, or check the Releases page.
5. **Confirm `build-push.yaml` ran on the release event.** Actions tab → most recent `Build and Push Container Image` run should show `release` as trigger, and both `build-push` and `build-push-chart` jobs should be green.
6. **Confirm published artifacts.**
   ```bash
   # Image: should show tags 0.1.0 + latest + the YYYYMMDD-sha + dev from the last nightly
   gh api -H "Accept: application/vnd.github+json" \
     "/users/drzero42/packages/container/nexorious/versions" \
     --jq '.[].metadata.container.tags' | head -20

   # Chart: should show 0.1.0 + 0.0.0-latest + 0.0.0-dev
   gh api -H "Accept: application/vnd.github+json" \
     "/users/drzero42/packages/container/charts%2Fnexorious/versions" \
     --jq '.[].metadata.container.tags' | head -10
   ```

If any of these fail, the runbook is in the spec's "Failure Modes" section.
