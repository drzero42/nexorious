# Pull requests

## Closing issues

A [closing keyword](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue) (`Closes`/`Fixes`/`Resolves`) applies **only to the single issue number immediately following it**. A comma-separated list does **not** work: `Closes #846, #847` auto-closes only #846 and leaves #847 open. Repeat the keyword for every issue — `Closes #846, closes #847` — or put one per line:

```
Closes #846
Closes #847
```

Cross-repo: `Closes owner/repo#123`. The keyword must be in the **PR body**, not only the title: GitHub links and closes issues from the PR description, and the squash commit carries only the title ([releasing.md](releasing.md)).

After merging a multi-issue PR, verify every referenced issue actually closed (`gh issue view <n> --json state`) and close any stragglers manually with a comment linking the PR.

## Merging

Merge with `gh pr merge <n> --squash --delete-branch`, and only when the user says so.

Run from the PR's branch, `gh pr merge` handles the local side too: it switches the checkout to `main`, deletes the local feature branch, **and pulls `main`**, so it already holds the new squash commit. Confirm with `git status` (expect `## main...origin/main`, no divergence); `git checkout main`, `git branch -D`, `git pull` or `git reset --hard origin/main` afterwards would be redundant.

## Required check

The "Protect main" ruleset (require PR, squash-only, no bypass) requires exactly one status check, the `CI Gate` job in `.github/workflows/test.yaml`. That job aggregates the path-gated suites (Go/frontend/Helm) and passes when each either succeeded or was skipped. When editing `test.yaml`, keep `CI Gate` green and reporting — renaming or removing it, or breaking its `needs:`/`if: always()` wiring, blocks every PR from merging.
