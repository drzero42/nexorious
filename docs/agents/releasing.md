# Releasing

Releases are produced by [release-please](https://github.com/googleapis/release-please) from the Conventional Commits on `main`.

## How a PR becomes a commit

PRs are squash-merged with `squash_merge_commit_title=PR_TITLE` and `squash_merge_commit_message=BLANK`, so every commit on `main` is exactly `<PR title> (#N)` with no body, and the **PR title** is what release-please parses. The PR description never reaches the commit (#1208: Renovate's release-notes descriptions broke release-please's parser and silently dropped commits from the changelog); follow `(#N)` to the PR for context. The only way to give release-please more than the title is an override block in the PR description (see [Overrides](#overrides)). The `PR Commit Message` workflow parses whichever message release-please will use, and fails a PR with a `Release-As:` / `BREAKING CHANGE:` line outside an override block.

| Prefix | Effect |
|---|---|
| `feat: …` | minor bump |
| `fix: …` | patch bump |
| `feat!: …` (or a `BREAKING CHANGE:` footer inside an override block) | **major bump** |
| `chore:`, `ci:`, `docs:`, `refactor:`, `test:` | no release |

## Release notes

release-please writes the GitHub Release and `CHANGELOG.md` from GitHub's generated release notes (`"changelog-type": "github"`), so each entry is the merged **PR title** with its author, plus a "New Contributors" section. The version bump still comes from the Conventional Commit, but an override block's wording does not reach the notes: to reword an entry, edit the merged PR's title before the release.

Categories are label-driven (`.github/release.yml`), and the `PR Labels` workflow (`pr-labels.yaml`, `pull_request_target` so fork PRs get labelled too) sets them from the commit type: `breaking` (`!` or a `BREAKING CHANGE:` footer), `enhancement` (`feat`), `bug` (`fix`/`perf`/`revert`), `dependencies` (`feat`/`fix` with scope `deps`), and `internal` for everything else, which the notes exclude. Labels are read when the notes are generated, so a wrong label can be fixed on the merged PR any time before the release.

## Cutting a release

1. Wait until there is a `feat:` or `fix:` on `main` since the last release (otherwise release-please's Release PR will be empty).
2. Find the open PR titled `chore(main): release X.Y.Z` (opened by `release-please-action`).
3. Review the proposed `CHANGELOG.md` diff (fix a misfiled entry by correcting the merged PR's label or title, then let release-please re-run), and the `Chart.yaml` / `docker-compose.yml` version bumps.
4. Merge the Release PR. release-please creates the `vX.Y.Z` tag and publishes a GitHub Release; `release-artifacts.yaml` then builds — for amd64 and arm64, from one per-arch binary — the raw binary, `.deb`, `.rpm`, and a multi-arch container image, smoke-tests the packages, uploads the release assets, pushes the image (semver tag + `latest`) and Helm chart, and advances the `release` branch.

The **`nexctl`** client ships the same way minus server-only pieces: per-arch raw binary + `.deb`/`.rpm` (via `deploy/packaging/nfpm-nexctl.yaml`, smoke-tested by the `smoke-test` job's `nexctl` matrix leg using `deploy/packaging/smoke-test-nexctl.sh`), but **no container image or Helm chart**. Both binaries share one repo version. There is no nightly/dev build flow; non-release artifacts do not exist.

## Overrides

To force the next release to a specific version, put an override block in the **PR description** before squash-merging, holding the full commit message — title line, blank line, `Release-As: X.Y.Z`:

```
BEGIN_COMMIT_OVERRIDE
fix: <the PR title>

Release-As: X.Y.Z
END_COMMIT_OVERRIDE
```

A bare `Release-As:` line in the description does nothing (squash commits have no body) and fails the `PR Commit Message` check. An empty override block is ignored by release-please. release-please splits on the bare start token, so a PR description carries it only when an override is meant — even a mention in prose or backticks turns the rest of the description into the commit message (the `PR Commit Message` check catches this). `main` is protected, so an override always travels through a PR.

To skip a release after an unwanted `feat:` / `fix:` lands: close the Release PR; release-please reopens it on the next push to main, so a true skip requires absorbing the change into the next legitimate release.

## Promoting to 1.0.0

A `feat!:` commit (or `BREAKING CHANGE:` footer) produces a `1.0.0` Release PR when the current version is `0.x.y`. Merge that PR to cut 1.0.0.
