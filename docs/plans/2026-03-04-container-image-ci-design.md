# Container Image CI/CD Design

**Date:** 2026-03-04
**Status:** Approved

## Goal

Publish Nexorious as a container image to the GitHub Container Registry (ghcr.io), with automated test and build/push workflows. The repository is being made public to unlock free GitHub-hosted runner minutes.

## Sequence of Work

Work is split into two phases to ensure the repo is in a good state before it goes public:

1. **Phase 1 — Main branch** (before going public)
   - Update README: add WIP banner and AI disclosure
   - Make the repository public via `gh repo edit --visibility public`

2. **Phase 2 — Feature branch** (after going public)
   - Update Dockerfile: `postgresql-client-16` → `postgresql-client-18`
   - Add `.dockerignore`
   - Add `test.yaml` reusable workflow
   - Add `build-push.yaml` workflow
   - Delete `.github/workflows/test-pr.yaml.disabled`

## Phase 1: README Updates

Two additions to `README.md`:

### WIP Banner (top of file)
A prominent `> [!WARNING]` admonition at the very top of the file, before the description, stating that Nexorious is a work in progress, not ready for production use, and that users should expect breaking changes, missing features, and rough edges.

### AI Disclosure Section (near bottom, before License)
A dedicated section disclosing that AI tooling — specifically Claude Code by Anthropic — was used extensively during development. States that this was an intentional choice to explore AI-assisted development. Users who object to AI-assisted code should avoid using Nexorious.

## Phase 2: Dockerfile Changes

### postgresql-client version
Both `backend/Dockerfile` and the root `Dockerfile` install `postgresql-client-16` for backup/restore functionality. Update both to `postgresql-client-18` to match the PostgreSQL version used in CI and production.

### `.dockerignore`
New file at repo root to keep the build context lean. Excludes:
- `backend/app/tests/`
- `frontend/node_modules/`
- `**/__pycache__/`
- `**/.pytest_cache/`
- `.git/`
- `docs/`
- `deploy/`
- `.github/`
- `*.md` (except root-level is fine, the key is test/dev artifacts)

## Phase 2: GitHub Actions Workflows

### Workflow: `test.yaml`

**File:** `.github/workflows/test.yaml`

**Triggers:**
- `pull_request` → `main`
- `push` → `main`
- `workflow_call` (so it can be called from `build-push.yaml`)

**Jobs** (all run in parallel, no dependencies):

#### `backend-tests`
- Runner: `ubuntu-latest`
- Service container: `postgres:18` with health check
- Steps:
  1. `actions/checkout@v4`
  2. `astral-sh/setup-uv@v5` (installs uv + Python 3.13, no Nix needed)
  3. `cd backend && uv sync --dev`
  4. `uv run pytest --cov=app --cov-report=term-missing --cov-fail-under=80`
- Environment: `DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nexorious_test`

#### `frontend-tests`
- Runner: `ubuntu-latest`
- Steps:
  1. `actions/checkout@v4`
  2. `actions/setup-node@v4` with Node 22, npm cache keyed on `frontend/package-lock.json`
  3. `cd frontend && npm ci`
  4. `npm run test:coverage -- --coverage.thresholds.lines=70 --coverage.thresholds.functions=70 --coverage.thresholds.branches=70 --coverage.thresholds.statements=70`

#### `type-check`
- Runner: `ubuntu-latest`
- Steps:
  1. `actions/checkout@v4`
  2. `actions/setup-node@v4` with Node 22, npm cache
  3. `cd frontend && npm ci`
  4. `npm run check` (tsc --noEmit + eslint)

**Note:** The old `test-pr.yaml.disabled` had a SQLite matrix and a "Svelte type check" step — both stale. This workflow replaces it entirely with PostgreSQL-only tests and a proper TypeScript/ESLint check.

### Workflow: `build-push.yaml`

**File:** `.github/workflows/build-push.yaml`

**Triggers:**
- `push` → `main`
- `push` of tags matching `v*`

**Jobs:**

#### `test`
```yaml
uses: ./.github/workflows/test.yaml
```
Reuses `test.yaml` via `workflow_call`. Build does not proceed if any test job fails.

#### `build-push` (needs: test)
- Runner: `ubuntu-latest`
- Steps:
  1. `actions/checkout@v4`
  2. `docker/login-action@v3` → `ghcr.io`, username `${{ github.actor }}`, password `${{ secrets.GITHUB_TOKEN }}`
  3. `docker/setup-buildx-action@v3`
  4. `docker/metadata-action@v5` — generates tags:
     - On push to `main`: `ghcr.io/drzero42/nexorious:YYYYMMDD-<short-sha>` and `ghcr.io/drzero42/nexorious:dev`
     - On push of `v1.2.3` tag: `ghcr.io/drzero42/nexorious:1.2.3` and `ghcr.io/drzero42/nexorious:latest`
  5. `docker/build-push-action@v6` — builds root `Dockerfile`, pushes with generated tags, uses GitHub Actions cache for layer caching

**Authentication:** Uses the built-in `GITHUB_TOKEN` — no additional secrets required on a public repo with ghcr.io.

## Cleanup

- Delete `.github/workflows/test-pr.yaml.disabled` — fully superseded by `test.yaml`

## What Is Not In Scope

- Re-enabling full Helm chart CI (separate task)
- Fixing remaining stale Next.js references in `README.md` (separate cleanup task)
- Multi-arch builds (arm64) — can be added later
