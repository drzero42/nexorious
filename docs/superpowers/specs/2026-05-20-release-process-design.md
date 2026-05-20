# Release Process Design

**Date:** 2026-05-20
**Status:** Draft

## Problem

The project has no release process. No git tags exist, no GitHub Releases have been published, and `deploy/helm/Chart.yaml` is permanently at `version: 0.1.0` / `appVersion: "latest"`. The CI is already partly wired for releases — `build-push.yaml` listens for `release: published`, image tagging uses `type=semver,pattern={{version}}`, and the chart-build step strips a leading `v` (implying tag format `vX.Y.Z`) — but nothing ever triggers it.

Two concrete drivers for changing this now:

1. **A stable channel.** Users currently track the `dev` nightly tag. We want a pinnable release channel that won't break under them when nightly does.
2. **A real 1.0 launch.** We're approaching a milestone where a public install snippet needs a real version baked in.

## Goals

- Define an on-demand release flow that produces tagged image + Helm chart releases.
- Use the project's existing Conventional Commits style as the input to a tooling-driven changelog and version bump.
- Keep the dev nightly channel running unchanged alongside the release channel.
- Auto-update in-repo install pins (`Chart.yaml`, `deploy/docker/docker-compose.yml`) so a fresh clone defaults to the latest released version, not `:dev`.
- Spell out the v1.0.0 promotion ceremony so it doesn't have to be re-invented when the time comes.

## Non-Goals

- Release candidates (`-rc.N`). Deferred. Pre-1.0 we can break things directly; revisit at the 1.0 transition if we want to derisk.
- Hotfix / patch-an-older-release flow. Deferred. No long-lived deployments to patch yet.
- Automated README install-snippet rewrites. Deferred. README can carry `:latest` references and be updated manually on major events.
- Commit-message CI enforcement. Deferred. The single maintainer already follows Conventional Commits; Renovate also emits valid CC.
- Changes to `test.yaml` or to the nightly build/cleanup flow defined in [2026-05-20-nightly-builds-image-retention-design.md](2026-05-20-nightly-builds-image-retention-design.md).

## Design

### 1. High-level flow

```
Conventional-commit PRs ──squash──▶ main
                                    │
                                    ▼
             release-please (push to main + daily schedule)
                                    │
                                    ▼
        Open/update long-running Release PR
        (bumps version, updates CHANGELOG.md,
         Chart.yaml, deploy/docker/docker-compose.yml)
                                    │
              maintainer reviews ───┴──── merge when ready
                                    ▼
        release-please creates git tag vX.Y.Z + GitHub Release
                                    │
                                    ▼
            existing build-push.yaml fires on `release: published`
                                    │
                            ┌───────┴────────┐
                            ▼                ▼
                  push image tags     push chart tags
                  X.Y.Z + latest      X.Y.Z + latest
```

Releases are on-demand. The maintainer cuts one by merging the Release PR; there is no schedule that publishes releases automatically. The nightly `dev` channel continues to run in parallel and is not affected.

### 2. Tooling: release-please

[release-please](https://github.com/googleapis/release-please) is the engine. It is a good fit here because:

- It parses Conventional Commits (already the style on `main`) to compute the next version and produce the changelog.
- It operates via a long-running "Release PR" the maintainer merges when ready — matches the on-demand cadence and gives a natural review checkpoint.
- It supports updating arbitrary files in the same Release PR via `extra-files`, which is needed for the install-pin auto-bumps.
- It supports a pre-1.0 mode that keeps version bumps inside `0.x` while iterating, and a single-commit promotion path to 1.0.0.

The alternative considered was a hand-rolled `release.yaml` triggered by `workflow_dispatch`. Rejected: would duplicate version-computation and changelog logic release-please already provides, and the maintainer would have to type a version number into a UI for every release.

### 3. Version policy

- **0.x window first**, then promote to 1.0.0 deliberately (see Section 6).
- During 0.x:
  - `bump-minor-pre-major: true` — breaking changes bump minor (`0.4.0` → `0.5.0`), not major. We don't want to burn the `1.0.0` slot before we're ready to commit to stability.
  - `bump-patch-for-minor-pre-major: true` — `feat:` commits bump patch (`0.4.0` → `0.4.1`), not minor. Reserves the minor slot for breaking changes during the stabilization window, so the minor digit communicates "incompatibility step" all the way through 0.x.
  - Net effect during 0.x: minor digit = breakage marker; patch digit = everything else release-worthy.
- After 1.0.0: standard SemVer. `feat:` → minor, `fix:` → patch, `feat!:` / `BREAKING CHANGE:` → major. Both pre-major flags are removed at the promotion moment.

### 4. Chart version coupling

The chart version equals the app version. Every release bumps both. This matches the existing logic in `build-push.yaml`'s `build-push-chart` job (chart_version = release tag minus `v`).

Trade-off: a pure chart change (e.g. a templates-only fix) requires a coupled app-version bump. Acceptable for now; can be split later by switching release-please to manifest-mode with two components if chart-only releases become frequent.

### 5. What the Release PR updates

release-please opens/updates a Release PR that contains the following changes when merged:

| File | Change |
|---|---|
| `CHANGELOG.md` | New section prepended for the version, sourced from `feat:` / `fix:` / `BREAKING CHANGE:` commits since the previous release. Created if it doesn't exist. |
| `.release-please-manifest.json` | Version bumped to the new release version. |
| `release-please-config.json` | Unchanged by the Release PR itself; edited by humans only (e.g. to flip flags at 1.0 promotion). |
| `deploy/helm/Chart.yaml` | `version: X.Y.Z` and `appVersion: "X.Y.Z"` set to the release version. |
| `deploy/docker/docker-compose.yml` | Image reference at line 36 updated from `ghcr.io/drzero42/nexorious:dev` to `ghcr.io/drzero42/nexorious:X.Y.Z`. |

`chore:`, `ci:`, `docs:`, `refactor:`, `test:`, and `chore(deps):` commits do **not** open a Release PR or contribute version bumps. Only `feat:`, `fix:`, and breaking changes do. This prevents Renovate flurries from forcing pointless releases.

The `extra-files` mechanism in `release-please-config.json` uses release-please's `generic` updater with anchor comments (e.g. `# x-release-please-version`) placed next to the lines to update. The exact anchor placement is an implementation detail for the plan to work out; the contract here is "these files end up bumped to the released version."

### 6. CI integration

#### `build-push.yaml`

Structural changes are minor — the `release: published` path already works. Two adjustments:

1. **Image tag `latest` on release.** Add to the `metadata-action` tag list:
   ```yaml
   type=raw,value=latest,enable=${{ github.event_name == 'release' }}
   ```
   This produces both `X.Y.Z` (from the existing semver pattern) and `latest` on a release publish.

2. **Helm chart `latest` floating tag on release.** Mirror the existing dev moving-tag pattern (which currently pushes `0.0.0-dev`) by adding a second `helm push` step for release events that packages and pushes the chart under a floating alias. The exact mechanism (a second `helm package --version` with a known floating string, or an OCI tag alias) is left to the plan.

The dev / nightly path described in [2026-05-20-nightly-builds-image-retention-design.md](2026-05-20-nightly-builds-image-retention-design.md) is unchanged. Release builds are independent of nightly retention — semver-tagged images and charts are explicitly protected from cleanup.

#### `test.yaml`

No changes. CI on the Release PR runs automatically via the existing `pull_request: branches: [main]` trigger, so a green Release PR means tests have passed on the merged main state plus the release-please commits.

#### New: `.github/workflows/release-please.yaml`

A new workflow that runs `googleapis/release-please-action` on:

- `push: branches: [main]` — recomputes the Release PR whenever main moves.
- `schedule:` daily — catches edge cases (e.g. a Release PR that needs reconciliation with a base-branch change that didn't fire the push trigger cleanly).

Permissions: `contents: write` (to push the release tag and the Release PR branch) and `pull-requests: write` (to open/update the Release PR).

### 7. Conventional Commits requirement

Add a "Release Process" section to `CLAUDE.md` that mandates Conventional Commits for everything that lands on `main`. Content sketch:

> ### Release Process
>
> Releases are produced by [release-please](https://github.com/googleapis/release-please) from the Conventional Commits on `main`. All commits on `main` must follow the convention. PRs are squash-merged, so the **PR title** is the commit message release-please parses.
>
> | Prefix | Effect (pre-1.0) | Effect (post-1.0) |
> |---|---|---|
> | `feat:` | patch bump | minor bump |
> | `fix:` | patch bump | patch bump |
> | `feat!:` / `BREAKING CHANGE:` body | **minor bump** | **major bump** |
> | `chore:`, `ci:`, `docs:`, `refactor:`, `test:` | no release | no release |
>
> Cutting a release: merge the open "chore(main): release X.Y.Z" PR from release-please. That creates the tag, the GitHub Release, and triggers the image + chart push.

No CI commit-message linter. If non-conforming commits land on main, release-please simply ignores them — the worst case is a missed bump, which the maintainer can correct via a follow-up commit with `Release-As: X.Y.Z` in the body.

### 8. Bootstrap and 1.0 promotion

#### Bootstrap (first release)

One-time setup, all in a single PR:

1. Add `release-please-config.json` with the pre-1.0 config (release-type `simple`, both pre-major bump flags `true`, `extra-files` entries for the install pins).
2. Add `.release-please-manifest.json` seeded at `0.0.0`. The first Release PR will then propose `0.1.0`.
3. Add `.github/workflows/release-please.yaml`.
4. Modify `.github/workflows/build-push.yaml` to add the `latest` image tag and the floating chart tag on release.
5. Add `CHANGELOG.md` as an empty file (or release-please creates it on first run — implementation detail for the plan).
6. Add the Release Process section to `CLAUDE.md`.

After merging that PR, release-please opens a Release PR proposing `0.1.0`. Merging that PR cuts the inaugural release.

#### 1.0 promotion

When the project is ready to commit to stability:

1. Open a "prepare 1.0" PR with a single commit that:
   - Edits `release-please-config.json` to remove (or set to `false`) `bump-minor-pre-major` and `bump-patch-for-minor-pre-major`.
   - Includes `Release-As: 1.0.0` in the commit body. release-please honors that footer and pins the next release to that exact version.
2. Merge the prep PR. release-please opens a Release PR with version `1.0.0` and a changelog covering everything since the prior 0.x release.
3. Merge the Release PR. `v1.0.0` is tagged, `build-push.yaml` publishes the image and chart.
4. From the next commit onward, post-1.0 SemVer applies automatically.

If we want to derisk before 1.0, that's the natural moment to introduce RC support — use `Release-As: 1.0.0-rc.1` first, validate, then `Release-As: 1.0.0`. Out of scope for this design, but the hook is here.

## Failure Modes

- **Empty Release PR** (only `chore:` / `ci:` since last release): release-please closes the PR or leaves it empty until a `feat:`/`fix:` lands. No release is cut. This is correct behavior — a release with no user-visible delta is noise.
- **Bad commit message lands on main**: release-please ignores it. Maintainer can include a `Release-As: X.Y.Z` footer in a subsequent commit to override the computed version, or amend by hand-editing the Release PR.
- **Image build fails after tag is created**: the tag and GitHub Release exist, but no image was pushed. Re-run `build-push.yaml` via `workflow_dispatch` against the tag, or delete the tag + release and recut. Documented as a runbook item, not a workflow change.
- **Release-please action quota / API failure**: the Release PR simply doesn't update. Maintainer reruns the workflow manually. No data loss.

## Summary of Changes

| File | Change |
|---|---|
| `release-please-config.json` | **New.** Pre-1.0 release-please config with `extra-files` entries. |
| `.release-please-manifest.json` | **New.** Seeded at `0.0.0`. |
| `.github/workflows/release-please.yaml` | **New.** Runs release-please on push to main + daily schedule. |
| `.github/workflows/build-push.yaml` | Add `latest` image tag and floating chart tag for release events. |
| `deploy/helm/Chart.yaml` | Add release-please anchor comments next to `version:` and `appVersion:`. |
| `deploy/docker/docker-compose.yml` | Add release-please anchor comment next to the `image:` line. |
| `CHANGELOG.md` | **New.** Empty file (or created by release-please on first run). |
| `CLAUDE.md` | Add "Release Process" section documenting Conventional Commits requirement and release mechanics. |
