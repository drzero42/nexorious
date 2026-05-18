# Repo Migration Design: nexorious-go → nexorious

**Date:** 2026-05-18  
**Status:** Approved

## Overview

The Go port of Nexorious has reached sufficient maturity to become the canonical codebase. This spec covers migrating everything from `github.com/drzero42/nexorious-go` into `github.com/drzero42/nexorious`, preserving the Python version on a `python` branch, and bringing all open issues across.

## Approach

**Prep-then-push:** All changes are made locally in the `nexorious-go` repo first, committed to a clean state, then force-pushed as `nexorious`'s new `main`. The canonical repo is never in a broken state.

Git history strategy: force-push the Go repo's `main` as `nexorious` main. Python history is preserved on the `python` branch.

## Phase 1: Branch rename on nexorious

Rename the Python `main` → `python` on GitHub. When the default branch is renamed, GitHub automatically:
- Makes `python` the new default branch
- Redirects all URLs from `/main/` to `/python/` permanently
- Retargets any open PRs (none currently)
- Transfers branch protection rules

Steps:
1. `gh api -X POST /repos/drzero42/nexorious/branches/main/rename -f new_name=python`
   — `python` becomes the default branch automatically

## Phase 2: Local prep in nexorious-go

All of the following are committed locally before anything touches the `nexorious` repo.

### 2.1 Go module path
- Update `go.mod`: `module github.com/drzero42/nexorious-go` → `module github.com/drzero42/nexorious`
- Replace all occurrences of `github.com/drzero42/nexorious-go` in every `.go` file with `github.com/drzero42/nexorious`
- Run `go mod tidy` to regenerate `go.sum`

### 2.2 Helm chart relocation
- Move `charts/nexorious-go/` → `deploy/helm/`
- In `Chart.yaml`: rename `name:` from `nexorious-go` to `nexorious`
- In `values.yaml` and `values.schema.json`: replace any `nexorious-go` references
- Update Helm chart templates for any hardcoded `nexorious-go` strings

### 2.3 Docker Compose
- Create `deploy/docker/docker-compose.yml` for the Go version
- Services: postgres + nexorious app binary
- Uses the `ghcr.io/drzero42/nexorious` image

### 2.4 GitHub Actions workflows
Create `.github/workflows/` with two files, adapted from the Python repo:

**`test.yaml`** — triggered on push/PR to `main` and via `workflow_call`:
- Go tests: `go test -timeout 600s ./...` with `DOCKER_HOST: unix:///var/run/docker.sock` (testcontainers)
- Go lint: `golangci-lint run`
- Frontend build + type-check: `cd ui/frontend && npm run build` (generates `routeTree.gen.ts`, then tsc)
- Frontend tests: `cd ui/frontend && npm run test`
- Helm lint: `helm lint --strict deploy/helm/` with required `--set` flags for required values

**`build-push.yaml`** — triggered on push to `main` and on published releases:
- Calls `test.yaml` via `workflow_call`
- Builds and pushes container image to `ghcr.io/drzero42/nexorious` with tags:
  - On release: semver tag (`1.2.3`)
  - On main: `YYYYMMDD-<short-sha>` + `dev`
  - Passes `VERSION` and `COMMIT` build args (used by Go `-ldflags`)
- Packages and pushes Helm chart to `oci://ghcr.io/drzero42/charts`
  - Release: chart version = semver tag, app_version = same
  - Main: chart version = `0.0.0-dev-YYYYMMDD-<sha>`, app_version = `dev`

### 2.5 README.md
Rewrite `README.md` for the Go version based on the Python README structure:
- Remove all Python/FastAPI/Next.js/uv content
- Correct stack: Go + React/Vite, single binary, devenv
- Correct quick-start, env vars, deployment, testing commands
- Keep the "Work in Progress" warning, Darkadia inspiration, AI-assisted development note, trademarks section, and MIT license footer

### 2.6 renovate.json
Copy `renovate.json` from the Python repo as-is — just `"extends": ["config:recommended"]`. No changes needed; the preset auto-detects Go, npm, Docker, and Helm.

### 2.7 .env.example
Create `.env.example` at the repo root with Go-specific env vars:
- DATABASE_URL
- SECRET_KEY (JWT signing)
- IGDB_CLIENT_ID / IGDB_CLIENT_SECRET
- Reference `docs/igdb-setup.md` in a comment

### 2.8 docs/igdb-setup.md
Port from the Python repo. Update:
- Remove references to `backend/` directory paths
- Update env var config examples to use `.env` at the repo root (or system env)
- Remove Docker Compose examples that reference Python services

### 2.9 CLAUDE.md and DEV.md
- `CLAUDE.md`: replace `nexorious-go` in module path examples, repo URL references, and any directory structure descriptions
- `DEV.md`: update Docker build/run examples that reference `nexorious-go:local` → `nexorious:local`

## Phase 3: Push and restore

1. Add nexorious as a remote in the local nexorious-go clone:
   `git remote add nexorious git@github.com:drzero42/nexorious.git`
2. Force-push the prepared branch as nexorious main:
   `git push nexorious HEAD:main --force`
3. Restore main as the default branch:
   `gh repo edit drzero42/nexorious --default-branch main`

Renovate's existing branches on nexorious will be closed and cleaned up automatically once Renovate processes the new main.

## Phase 4: Issues migration

Transfer 8 open issues from `drzero42/nexorious-go` to `drzero42/nexorious`:

| # | Title | Labels |
|---|-------|--------|
| 47 | Add GOG library sync | enhancement |
| 46 | Add LEGENDARY_WORK_DIR env + emptyDir volume to helm chart | enhancement |
| 45 | Add legendary-gl to runtime container image for Epic Games Store sync | enhancement |
| 36 | Add notifications | — |
| 34 | Add nix build | — |
| 33 | Use knip to declutter frontend | — |
| 31 | Replace containsString with slices.Contains in igdb/keywords.go | — |
| 25 | Add MCP support | enhancement |

For each issue:
1. Check if the label exists on nexorious; create it if not
2. Create the issue on nexorious with the same title, body, and labels
3. Close the original on nexorious-go with a comment: "Moved to drzero42/nexorious#<new-number>"

## What stays in nexorious-go

After migration, `nexorious-go` remains as-is — no archiving or deletion required. The repo serves as a historical record of the Go port development.
