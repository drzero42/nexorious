# Nightly Dev Builds & Image Retention Design

**Date:** 2026-05-20
**Status:** Approved

## Problem

The current `build-push.yaml` workflow fires on every push to `main`, running a full Docker build, image push, and Helm chart push for each commit. Two concrete issues:

1. **Wasteful build minutes** — a Docker build + Helm push runs for every commit, even trivial ones. For a dev image that changes infrequently in meaningful ways, this is excess spend.
2. **Unbounded registry storage** — `YYYYMMDD-<sha>` image tags and `0.0.0-dev-YYYYMMDD-<sha>` Helm chart versions accumulate indefinitely in GHCR with no retention policy.

A silent third issue discovered during analysis: the test suite runs **twice** on every push to `main` — once from `test.yaml`'s own direct trigger, and once via `workflow_call` from `build-push.yaml`. This wastes more minutes than the Docker build itself.

## Goals

- Reduce build minutes by switching to a nightly schedule for dev image builds
- Keep the `dev` floating tag reasonably fresh (max ~24h lag)
- Provide an escape hatch for immediate builds when needed
- Retain ~30 dev image versions and ~30 dev Helm chart versions; delete the rest
- Eliminate the duplicate test run

## Non-Goals

- Changes to release builds (triggered by `release: published`) — those remain untouched
- Changes to `test.yaml` job definitions or what tests run
- Any changes to image tagging conventions

## Design

### Trigger Restructure

**`build-push.yaml`** trigger changes:

| Trigger | Before | After |
|---|---|---|
| `push: branches: [main]` | yes | **removed** |
| `schedule: '0 2 * * *'` | no | **added** (2 AM UTC nightly) |
| `workflow_dispatch` | no | **added** (manual escape hatch) |
| `release: types: [published]` | yes | unchanged |

**`test.yaml`** — no changes. It already triggers on `push: branches: [main]`, `pull_request`, and `workflow_call`. Tests continue running on every push to `main` for fast feedback.

**Fix for the double test run:** Removing the `push: main` trigger from `build-push.yaml` means it no longer calls `test.yaml` via `workflow_call` on push. The `test` job and `needs: test` in `build-push.yaml` are removed entirely. Tests still fire on every push via `test.yaml`'s own direct trigger — exactly once.

For scheduled and dispatch builds, tests are not re-run; the code has already been tested when it was committed.

### Pre-Check Job (Skip-If-Already-Built)

A `pre-check` job runs before `build-push` and outputs `should_build: true|false`.

- **`workflow_dispatch` or `release`**: always `should_build=true`, no API call needed.
- **`schedule`**: query the GitHub Packages REST API (`GET /users/drzero42/packages/container/nexorious/versions`) to check whether any existing image version is tagged with the current HEAD's short SHA. If a matching tag is found, `should_build=false`; all downstream jobs are skipped via `if: needs.pre-check.outputs.should_build == 'true'`.

This covers the common case where no commits were pushed since the previous night's build, avoiding a wasted nightly run.

### Job Dependency Graph

```
pre-check
    └── build-push (if should_build == true)
            ├── cleanup (non-release only)
            └── build-push-chart
```

### Image & Chart Cleanup

A `cleanup` job runs after `build-push` succeeds, only on non-release triggers (`if: github.event_name != 'release'`).

It runs two steps using `actions/delete-package-versions@v5`:

**Step 1 — Container image (`nexorious` package)**
- Protect versions tagged with a semver pattern (release images: `1.2.3`, etc.)
- Protect the `dev` floating tag
- Among remaining `YYYYMMDD-<sha>` dev versions: keep the 30 newest, delete the rest

**Step 2 — Helm chart (`charts/nexorious` package)**
- Protect versions tagged with a semver chart version (release charts)
- Protect the `0.0.0-dev` moving tag
- Among remaining `0.0.0-dev-YYYYMMDD-<sha>` dev versions: keep the 30 newest, delete the rest

The `dev` floating tag and `0.0.0-dev` moving tag are never explicitly deleted — they naturally re-point to the latest build on each push.

At 1 build/day, 30 versions ≈ 30 days of history.

## Summary of Changes

| File | Change |
|---|---|
| `.github/workflows/build-push.yaml` | Replace `push: main` with `schedule` + `workflow_dispatch`; remove `test` job and `needs: test`; add `pre-check` job; add `cleanup` job |
| `.github/workflows/test.yaml` | No changes |
