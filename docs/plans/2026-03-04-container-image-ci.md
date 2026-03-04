# Container Image CI/CD Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Publish Nexorious as a container image to ghcr.io with automated test and build/push GitHub Actions workflows, gated behind the repository being made public.

**Architecture:** Phase 1 updates the README and makes the repo public on `main`. Phase 2 creates a feature branch to add `.dockerignore`, update PostgreSQL client versions, add a reusable `test.yaml` workflow, and a `build-push.yaml` that calls it before building.

**Tech Stack:** GitHub Actions, `docker/build-push-action`, `docker/metadata-action`, `astral-sh/setup-uv`, ghcr.io, PostgreSQL 18.

---

## PHASE 1 — Main Branch (before going public)

### Task 1: Add WIP banner to README

**Files:**
- Modify: `README.md` (line 1, after the `# Nexorious` heading)

**Step 1: Insert the WIP warning admonition**

Add immediately after the `# Nexorious` heading, before the description paragraph:

```markdown
> [!WARNING]
> **Work in Progress — Not Ready for Use**
> Nexorious is under active development and is not ready for production use or general adoption. Expect breaking changes, missing features, incomplete documentation, and rough edges. Use at your own risk.
```

**Step 2: Verify it renders correctly**

Open `README.md` and confirm the `> [!WARNING]` block appears before the feature list.

---

### Task 2: Add AI disclosure section to README

**Files:**
- Modify: `README.md` (before the `## License` section)

**Step 1: Insert the AI disclosure section**

Add before the `## Trademarks and Copyright` section:

```markdown
## AI-Assisted Development

Nexorious was built with extensive use of AI tooling, specifically [Claude Code](https://claude.ai/code) by Anthropic. AI assistance was used throughout the project for code generation, architecture decisions, debugging, and documentation.

This is an intentional choice — Nexorious is partly an experiment in what AI-assisted software development can produce. If you have strong objections to AI-generated or AI-assisted code, Nexorious may not be the right project for you.
```

**Step 2: Verify placement**

Confirm the section appears before `## Trademarks and Copyright` and after the main content.

---

### Task 3: Commit and push README changes to main

**Step 1: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add README.md
git commit -m "docs: add WIP warning and AI disclosure to README"
```

**Step 2: Push**

```bash
git push origin main
```

---

### Task 4: Make the repository public

**Step 1: Run the gh command**

```bash
gh repo edit drzero42/nexorious --visibility public
```

Confirm the prompt when asked. This action is irreversible without contacting GitHub support.

**Step 2: Verify**

```bash
gh repo view drzero42/nexorious --json visibility -q .visibility
```

Expected output: `PUBLIC`

---

## PHASE 2 — Feature Branch (after going public)

### Task 5: Create feature branch

**Step 1: Create and switch to branch**

```bash
cd /home/abo/workspace/home/nexorious
git checkout -b feat/container-image-ci
```

---

### Task 6: Update postgresql-client in both Dockerfiles

**Files:**
- Modify: `Dockerfile` (root)
- Modify: `backend/Dockerfile`

**Step 1: Update root Dockerfile**

In `Dockerfile`, find the line:
```
    && apt-get install -y --no-install-recommends postgresql-client-16 \
```
Change to:
```
    && apt-get install -y --no-install-recommends postgresql-client-18 \
```

**Step 2: Update backend/Dockerfile**

In `backend/Dockerfile`, find the same line and apply the same change:
```
    && apt-get install -y --no-install-recommends postgresql-client-18 \
```

**Step 3: Commit**

```bash
git add Dockerfile backend/Dockerfile
git commit -m "chore: upgrade postgresql-client from 16 to 18"
```

---

### Task 7: Add .dockerignore

**Files:**
- Create: `.dockerignore`

**Step 1: Create the file**

```
# Version control
.git/
.github/

# Documentation and planning
docs/
*.md

# Deployment configs (not needed in image)
deploy/

# Frontend build artifacts and deps (rebuilt inside Docker)
frontend/node_modules/
frontend/dist/
frontend/coverage/
frontend/.vite/

# Backend virtualenv and test artifacts (rebuilt inside Docker)
backend/.venv/
backend/app/tests/
backend/htmlcov/
backend/.pytest_cache/

# Python bytecode
**/__pycache__/
**/*.pyc
**/*.pyo

# Local development
*.env
.env*

# Runtime storage (populated at runtime, not baked in)
storage/
```

**Step 2: Verify the build context is still complete**

The Dockerfile copies `frontend/` (for `npm ci` and `npm run build`) and `backend/` (for `uv sync`). None of those excluded paths are needed for the build — the Dockerfile installs dependencies fresh inside the image.

**Step 3: Commit**

```bash
git add .dockerignore
git commit -m "chore: add .dockerignore to slim build context"
```

---

### Task 8: Delete the disabled test workflow

**Files:**
- Delete: `.github/workflows/test-pr.yaml.disabled`

**Step 1: Remove the file**

```bash
git rm .github/workflows/test-pr.yaml.disabled
git commit -m "chore: remove stale disabled test workflow"
```

---

### Task 9: Create the reusable test workflow

**Files:**
- Create: `.github/workflows/test.yaml`

**Step 1: Create the workflow**

```yaml
---
name: Test

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
  workflow_call:

jobs:
  backend-tests:
    name: Backend Tests
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:18
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: nexorious_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Install uv
        uses: astral-sh/setup-uv@v5
        with:
          python-version: "3.13"

      - name: Install dependencies
        run: |
          cd backend
          uv sync --dev

      - name: Run backend tests
        env:
          DATABASE_URL: postgresql://postgres:postgres@localhost:5432/nexorious_test
        run: |
          cd backend
          uv run pytest --cov=app --cov-report=term-missing --cov-fail-under=80

  frontend-tests:
    name: Frontend Tests
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: cd frontend && npm ci

      - name: Run frontend tests with coverage
        run: |
          cd frontend
          npm run test:coverage -- \
            --coverage.thresholds.lines=70 \
            --coverage.thresholds.functions=70 \
            --coverage.thresholds.branches=70 \
            --coverage.thresholds.statements=70

  type-check:
    name: Type Check
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "22"
          cache: "npm"
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        run: cd frontend && npm ci

      - name: Run type check
        run: cd frontend && npm run check
```

**Step 2: Commit**

```bash
git add .github/workflows/test.yaml
git commit -m "ci: add reusable test workflow (backend, frontend, type-check)"
```

---

### Task 10: Create the build-push workflow

**Files:**
- Create: `.github/workflows/build-push.yaml`

**Step 1: Create the workflow**

```yaml
---
name: Build and Push Container Image

on:
  push:
    branches: [main]
    tags: ["v*"]

jobs:
  test:
    name: Run Tests
    uses: ./.github/workflows/test.yaml

  build-push:
    name: Build and Push
    runs-on: ubuntu-latest
    needs: test

    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Log in to ghcr.io
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Extract image metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/drzero42/nexorious
          tags: |
            # Release tags: v1.2.3 → 1.2.3 + latest
            type=semver,pattern={{version}}
            type=raw,value=latest,enable=${{ startsWith(github.ref, 'refs/tags/v') }}
            # Main branch builds: YYYYMMDD-<short-sha> + dev
            type=raw,value={{date 'YYYYMMDD'}}-{{sha}},enable=${{ github.ref == 'refs/heads/main' }}
            type=raw,value=dev,enable=${{ github.ref == 'refs/heads/main' }}

      - name: Build and push image
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Step 2: Commit**

```bash
git add .github/workflows/build-push.yaml
git commit -m "ci: add build-push workflow for ghcr.io container image"
```

---

### Task 11: Push branch and open PR

**Step 1: Push the branch**

```bash
git push -u origin feat/container-image-ci
```

**Step 2: Open a PR**

```bash
gh pr create \
  --title "ci: container image build and push to ghcr.io" \
  --body "$(cat <<'EOF'
## Summary

- Updates `postgresql-client-16` → `postgresql-client-18` in both Dockerfiles
- Adds `.dockerignore` to slim the build context
- Adds reusable `test.yaml` workflow (backend tests with PostgreSQL 18, frontend tests, type-check)
- Adds `build-push.yaml` that runs tests then builds and pushes to `ghcr.io/drzero42/nexorious`
- Deletes stale `test-pr.yaml.disabled`

## Tagging strategy

| Trigger | Tags |
|---|---|
| Push to `main` | `YYYYMMDD-<short-sha>`, `dev` |
| Push `v*` tag | `1.2.3`, `latest` |

## Test plan

- [ ] All three test jobs pass in the PR check
- [ ] After merge, verify `build-push.yaml` triggers and pushes `dev` tag to ghcr.io
- [ ] Verify image is visible at `ghcr.io/drzero42/nexorious`

🤖 Generated with [Claude Code](https://claude.ai/code)
EOF
)"
```

---

### Task 12: Review and merge PR

**Step 1: Check the PR diff**

```bash
gh pr diff
```

Verify:
- Both Dockerfiles changed from `postgresql-client-16` to `postgresql-client-18`
- `.dockerignore` is present and excludes dev artifacts
- `test.yaml` has three jobs: `backend-tests`, `frontend-tests`, `type-check`
- `build-push.yaml` calls `test.yaml` via `workflow_call` before `build-push`
- `test-pr.yaml.disabled` is deleted

**Step 2: Wait for PR checks to pass**

```bash
gh pr checks --watch
```

All three test jobs must be green before merging.

**Step 3: Merge with squash**

```bash
gh pr merge --squash --delete-branch
```
