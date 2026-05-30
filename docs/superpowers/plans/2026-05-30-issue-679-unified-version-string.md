# Issue #679: Unified Version String Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every artifact (binary + container image) carries a consistent `BRANCH-DATE-HASH` version string on non-release builds and a bare semver (`0.3.1`, no `v` prefix) on release builds.

**Architecture:** The Makefile becomes the single source of truth for VERSION computation. CI reads the computed version via a `make print-version` target and passes it as a Docker build-arg. The docker/metadata-action tag pattern and the pruning regex in the cleanup job are updated to match the new `main-YYYYMMDD-sha` format.

**Tech Stack:** GNU Make, GitHub Actions (`docker/metadata-action@v6`, `docker/build-push-action@v7`, `vlaurin/action-ghcr-prune@v0.6.0`), Go ldflags injection.

**Decisions recorded here (resolved before planning):**
- Release version string: strip `v` prefix → `0.3.1` not `v0.3.1`
- `dev` floating container image tag: keep as-is
- Helm chart `app_version`: stays `"dev"` for non-release builds (no change)

---

## Files

| File | Change |
|---|---|
| `Makefile` | Replace single-line `VERSION` formula with branch-aware block; change `COMMIT` from `?=` to `:=`; add `print-version` phony target |
| `.github/workflows/build-push.yaml` | (1) Add `Compute VERSION` step in `build-push` job; (2) update `VERSION=` build-arg to use that step; (3) update docker/metadata-action tag pattern to add `main-` prefix; (4) update prune regex |

**Not changed:**
- `Dockerfile` — `ARG VERSION` and `ARG COMMIT` already flow through; no modification needed.
- `cmd/nexorious/` — ldflags injection unchanged; no Go code change.
- Helm chart `Compute chart version` step — `app_version` deliberately stays `"dev"`.

---

## Task 1: Replace Makefile VERSION logic

**Files:**
- Modify: `Makefile` (lines 3–5, and after `coverage` target)

### How the new logic works

- `COMMIT` — short git SHA (7 chars), always evaluated immediately.
- `TAG` — set only if the current HEAD is directly tagged (exact match). Empty otherwise.
- If `TAG` is set: `VERSION` defaults to the tag with any leading `v` stripped (`v0.3.1` → `0.3.1`) using Make's substitution reference `$(TAG:v%=%)`.
- If `TAG` is empty: compute `_RAW_BRANCH` from `git rev-parse --abbrev-ref HEAD`. In a detached-HEAD environment (GitHub Actions after `actions/checkout`), this returns the literal string `HEAD` — in that case fall back to `GITHUB_REF_NAME`, which GitHub Actions always provides. Sanitize the branch name (lowercase, non-alphanumeric → `-`, collapse runs of `-`, strip leading/trailing `-`), then combine with today's date and the short SHA.
- `VERSION ?=` throughout means `VERSION=custom make build` still overrides.

- [ ] **Step 1: Replace lines 3–5 of Makefile**

  Current content to replace:
  ```makefile
  VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
  COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
  LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"
  ```

  New content:
  ```makefile
  COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
  TAG     := $(shell git describe --exact-match HEAD 2>/dev/null)

  ifneq ($(TAG),)
    VERSION ?= $(TAG:v%=%)
  else
    _RAW_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
    ifeq ($(_RAW_BRANCH),HEAD)
      _RAW_BRANCH := $(or $(GITHUB_REF_NAME),unknown)
    endif
    _BRANCH     := $(shell echo "$(_RAW_BRANCH)" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | sed 's/^-//;s/-$$//')
    _DATE       := $(shell date +%Y%m%d)
    VERSION     ?= $(_BRANCH)-$(_DATE)-$(COMMIT)
  endif

  LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"
  ```

- [ ] **Step 2: Add `print-version` phony target**

  Add to the `.PHONY` line at the top of the Makefile:
  ```makefile
  .PHONY: all frontend build docker test test-backend test-frontend coverage print-version
  ```

  Add after the `coverage` target:
  ```makefile
  print-version:
  	@echo $(VERSION)
  ```
  *(The indentation before `@echo` must be a tab character, not spaces.)*

- [ ] **Step 3: Verify on the current branch**

  ```bash
  make print-version
  ```
  Expected output (on `main` branch): `main-20260530-<7-char-hash>`
  Example: `main-20260530-e48c052`

  ```bash
  make build && ./nexorious version
  ```
  Expected: `nexorious main-20260530-<7-char-hash> (<7-char-hash>)`

- [ ] **Step 4: Commit**

  ```bash
  git add Makefile
  git commit -m "build: add branch-aware VERSION logic to Makefile"
  ```

---

## Task 2: Add VERSION computation step to CI workflow

**Files:**
- Modify: `.github/workflows/build-push.yaml` — `build-push` job only

The `build-push` job currently hardcodes `VERSION=${{ github.ref_type == 'tag' && github.ref_name || 'dev' }}` in the `build-args` of `docker/build-push-action`. Replace this with a dedicated step that:
- For release tag builds (`github.ref_type == 'tag'`): strips the `v` prefix from `github.ref_name` directly (reliable; no git-describe needed in shallow clone).
- For all other builds: delegates to `make print-version`, which will use `GITHUB_REF_NAME` (already set by the runner) via the Makefile's `_RAW_BRANCH` fallback.

- [ ] **Step 1: Add the `Compute VERSION` step**

  In the `build-push` job, after the `Set up Docker Buildx` step and before the `Extract image metadata` step, insert:

  ```yaml
        - name: Compute VERSION
          id: build-version
          run: |
            if [[ "${{ github.ref_type }}" == "tag" ]]; then
              TAG="${{ github.ref_name }}"
              echo "value=${TAG#v}" >> "$GITHUB_OUTPUT"
            else
              echo "value=$(make print-version)" >> "$GITHUB_OUTPUT"
            fi
  ```

- [ ] **Step 2: Update build-args to use the new step output**

  In the `Build and push image` step, change:
  ```yaml
            build-args: |
              VERSION=${{ github.ref_type == 'tag' && github.ref_name || 'dev' }}
              COMMIT=${{ github.sha }}
  ```
  To:
  ```yaml
            build-args: |
              VERSION=${{ steps.build-version.outputs.value }}
              COMMIT=${{ github.sha }}
  ```

- [ ] **Step 3: Commit**

  ```bash
  git add .github/workflows/build-push.yaml
  git commit -m "ci: compute VERSION via Makefile for Docker build"
  ```

---

## Task 3: Update container image tag pattern and prune regex

**Files:**
- Modify: `.github/workflows/build-push.yaml` — `build-push` job (metadata-action) and `cleanup` job (prune step)

Two independent changes in the same file:

### 3a — docker/metadata-action tag pattern

The current main-branch tag `{{date 'YYYYMMDD'}}-{{sha}}` (e.g. `20260530-e48c052`) must gain a `main-` prefix to match the binary version string.

- [ ] **Step 1: Update the metadata-action tag**

  In the `Extract image metadata` step, change:
  ```yaml
              # Main branch builds: YYYYMMDD-<short-sha> + dev
              type=raw,value={{date 'YYYYMMDD'}}-{{sha}},enable=${{ github.ref == 'refs/heads/main' }}
              type=raw,value=dev,enable=${{ github.ref == 'refs/heads/main' }}
  ```
  To:
  ```yaml
              # Main branch builds: main-YYYYMMDD-<short-sha> + dev
              type=raw,value=main-{{date 'YYYYMMDD'}}-{{sha}},enable=${{ github.ref == 'refs/heads/main' }}
              type=raw,value=dev,enable=${{ github.ref == 'refs/heads/main' }}
  ```

### 3b — cleanup prune regex

The `cleanup` job prunes old dev-build container image tags. The current regex `^\d{8}-[a-f0-9]+$` matches the old format. Update it to match the new `main-YYYYMMDD-sha` format. Include the old pattern as a second regex to clean up any pre-existing tags from before this change.

- [ ] **Step 2: Update the prune regex**

  In the `Prune old dev container image versions` step, change:
  ```yaml
            prune-tags-regexes: |
              ^\d{8}-[a-f0-9]+$
  ```
  To:
  ```yaml
            prune-tags-regexes: |
              ^main-\d{8}-[a-f0-9]+$
              ^\d{8}-[a-f0-9]+$
  ```

  Note: The `keep-tags-regexes` block (`^\d+\.\d+\.\d+(-[A-Za-z0-9.-]+)?$` and `^dev$`) needs no change — release semver tags and the `dev` floating tag remain protected.

- [ ] **Step 3: Commit**

  ```bash
  git add .github/workflows/build-push.yaml
  git commit -m "ci: use main-YYYYMMDD-sha image tag; update prune regex"
  ```

---

## Task 4: Open PR

- [ ] **Step 1: Push the branch and open PR**

  ```bash
  git push -u origin HEAD
  gh pr create \
    --title "build: unify version string as BRANCH-DATE-HASH across binary and container image tags" \
    --body "Closes #679"
  ```

  PR title must follow the Conventional Commits convention (`build:` prefix) so release-please ignores it (no version bump for a build-system change).

---

## Self-Review Checklist (completed before saving plan)

| Spec requirement | Covered by |
|---|---|
| Non-release binary: `BRANCH-DATE-HASH` | Task 1 |
| Release binary: bare semver, no `v` | Task 1 (`TAG:v%=%`) |
| Branch sanitization (lowercase, non-alphanumeric → `-`, collapse `-`) | Task 1 (`_BRANCH` sed pipeline) |
| Detached-HEAD fallback via `GITHUB_REF_NAME` | Task 1 (`ifeq ($(_RAW_BRANCH),HEAD)`) |
| `make build` locally shows new format | Task 1 step 3 |
| Container image tag gains `main-` prefix | Task 3a |
| `dev` floating tag preserved | Task 3a (unchanged line) |
| Helm `app_version` stays `"dev"` | No change needed — explicitly out of scope |
| Prune regex updated | Task 3b |
| Old-format tags cleaned up | Task 3b (second regex) |
| `GET /api/version` returns new format | No Go change needed — ldflags inject at build |
| Sidebar shows new format | No frontend change needed — fetched from API |
