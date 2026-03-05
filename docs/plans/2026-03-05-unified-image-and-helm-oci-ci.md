# Design: Unified Image CI + Helm OCI Publishing

**Date:** 2026-03-05
**Status:** Approved

## Context

The container image build workflow (`build-push.yaml`) now produces a single unified image (`ghcr.io/drzero42/nexorious`) that serves both the FastAPI backend and the Vite frontend SPA via FastAPI's `StaticFiles` catch-all. The Helm chart `values.yaml` still references the old placeholder image (`ghcr.io/your-org/nexorious-api`) and hardcodes `tag: latest`. Additionally, the Helm chart has never been published as an OCI artifact, and the workflow trigger is based on git tag pushes rather than GitHub Releases.

## Goals

1. Fix the Helm chart image references to point at the real unified image.
2. Switch the workflow trigger from `push: tags: ["v*"]` to `release: [published]`.
3. Publish the Helm chart as an OCI artifact to `oci://ghcr.io/drzero42/charts` on every main commit and every release.
4. Chart version and appVersion are derived automatically from the triggering event — no manual bumping.

## Design

### 1. Workflow Trigger Change

Remove the `tags: ["v*"]` push trigger and add a `release: [published]` trigger:

```yaml
on:
  push:
    branches: [main]
  release:
    types: [published]
```

The existing `docker/metadata-action` tag rules are compatible: for a `release` event, `github.ref` is the release's underlying tag (`refs/tags/v1.2.3`), so `type=semver` and the `latest` conditional work unchanged.

### 2. Helm Chart Image References (`values.yaml`)

All three application controllers (`api`, `worker`, `scheduler`) get updated:

- `repository`: `ghcr.io/your-org/nexorious-api` → `ghcr.io/drzero42/nexorious`
- `tag`: `latest` → `""` (empty string — the bjw-s common library falls back to `.Chart.AppVersion` when tag is empty)

This means a released chart automatically pulls the matching image version by default, with no user configuration required.

### 3. New `build-push-chart` Job

Added to `build-push.yaml`, with `needs: build-push` (chart is never pushed if the image build fails).

**Version computation:**

| Event | Chart version | appVersion |
|---|---|---|
| `push` to `main` | `0.0.0-dev-YYYYMMDD-sha` | `dev` |
| `release: published` | `X.Y.Z` (tag without `v`) | `X.Y.Z` |

**Steps:**
1. Checkout code
2. Compute chart version and appVersion (bash, conditional on `github.event_name`)
3. `helm dependency update deploy/helm/`
4. `helm package deploy/helm/ --version $VERSION --app-version $APP_VERSION`
5. `echo $TOKEN | helm registry login ghcr.io`
6. `helm push nexorious-*.tgz oci://ghcr.io/drzero42/charts`

**OCI path:** `ghcr.io/drzero42/charts/nexorious` — this is a GitHub Package namespace, not a GitHub repository.

**Installation:**
```
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious --version 1.2.3
```

## Files Changed

| File | Change |
|---|---|
| `.github/workflows/build-push.yaml` | New trigger, new `build-push-chart` job |
| `deploy/helm/values.yaml` | Fix image repo, set `tag: ""` on api/worker/scheduler |
