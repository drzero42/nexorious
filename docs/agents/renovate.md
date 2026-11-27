# Renovate PRs

Renovate is self-hosted (`cloudzero-renovate-operator`, a few runs a day), not the Mend app, so `@renovatebot` comments do nothing.

## Grouping

Non-major npm and Go updates arrive **pre-grouped** — one "npm non-major" PR (`package-lock.json`) and one "go non-major" PR (`go.sum` + `goVendorHash` in `flake.nix`), configured via `packageRules` in `.github/renovate.json`. These two group PRs touch disjoint files, so they do **not** conflict with each other — merge them independently. `flake.lock` updates come from Renovate's nix manager as their own weekly lockfile-maintenance PR (Monday morning), never bundled into an unrelated PR.

## Post-upgrade tasks

Renovate runs two scripts as `postUpgradeTasks` (both are in the self-hosted `allowedCommands`, and the runner needs `nix`):

- `nix/update-go-vendor-hash.sh` on every gomod update, so a Go PR (including a rebase) arrives with `goVendorHash` recomputed in the same commit. See [nix.md](nix.md).
- `ui/frontend/scripts/regen-route-tree.sh` on TanStack Router updates, so a router bump's PR arrives with the regenerated `routeTree.gen.ts`. The script also works by hand.

No workflow pushes to Renovate branches; `nix.yaml` only verifies.

## Same-ecosystem conflicts

A **major** PR and its same-ecosystem **group** PR both touch the same lockfile (and, for Go, `goVendorHash`), so merging one invalidates the other. Nothing in CI rebases Renovate branches — Renovate rebases a conflicted branch itself on its next run. Branches don't need to be up to date with `main` to merge (the ruleset's required check is not strict).

To merge two same-ecosystem PRs (major + group) by hand:

1. Verify all CI checks pass on the target PR (use `gh run list --branch <branch>` — the GitHub API's `statusCheckRollup` field lags and often shows stale/empty data).
2. Merge **one PR** with `gh pr merge <n> --squash --delete-branch`.
3. The other PR now conflicts and is fixed by Renovate's rebase on its next run. Ask the user to trigger a run rather than wait for the schedule (ticking the "rebase/retry" checkbox also only takes effect on the next run).
4. Wait for the rebase to land and CI to pass (~4 min) before merging it.
