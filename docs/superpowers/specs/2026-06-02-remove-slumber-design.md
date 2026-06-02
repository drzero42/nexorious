# Remove Slumber — Design

**Date:** 2026-06-02
**Status:** Approved

## Problem

Slumber (a terminal API client) no longer adds enough value to justify its maintenance cost. It is installed via devenv, ships a 28 KB collection file (`slumber.yaml`), is documented across the forward-looking docs, and has an open enhancement issue (#658) proposing to extend it. We want it gone, with the repo left clean.

## Goal

Remove Slumber and its collection entirely from the project's tooling, configuration, and forward-looking documentation. Close the obsolete enhancement issue. Leave historical design records untouched.

## Scope

### 1. Delete the collection

- Remove `slumber.yaml`.

### 2. Remove the tool from the dev environment

- `devenv.nix`: delete the package line `inputs.drzero42.packages.${system}.slumber` (line 19).
- `devenv.yaml`: delete the `drzero42` input block. It exists solely to supply slumber; nothing else in `devenv.nix` references `drzero42`.
- `devenv.lock`: regenerate so the orphaned `drzero42` lock node is dropped and the lock matches the edited `devenv.yaml`. Run `devenv` (or `devenv update`) to refresh, and commit the result.

### 3. Scrub forward-looking docs

- `README.md`:
  - Remove the `| API client | slumber |` quick-reference table row.
  - In the "API Documentation" section, remove the slumber instructions and reword to point readers at the route handlers in `internal/api/` as the source of truth for available endpoints.
- `DEV.md`:
  - Remove the entire `## API Client (Slumber)` section.
- `CLAUDE.md`:
  - Remove the `| Run API client | slumber |` quick-reference table row.
  - Remove the entire `### Slumber Collection Maintenance` subsection.

### 4. Close the issue

- Close GitHub issue #658 ("feat: bootstrap slumber collection with API key creation") as not-planned, with a comment noting slumber has been removed from the project.

### 5. Out of scope (left untouched)

- All files under `docs/superpowers/specs/` and `docs/superpowers/plans/`, including the `2026-05-28-slumber-session-auth` spec + plan pair. These are point-in-time historical records; removing slumber today does not make past history inaccurate.

## Workflow

- Feature branch off `main`.
- Single commit containing all file deletions/edits plus the regenerated `devenv.lock`.
- Open a PR (squash-merge per project convention).
- Close issue #658 separately via `gh`.

## Verification

- `grep -ri slumber` over the working tree, excluding `docs/superpowers/` and `.git`, returns no matches.
- `devenv` evaluates cleanly after the lock regeneration (no reference to a missing `drzero42` input).
