#!/usr/bin/env bash
# Stop hook: when Claude finishes a turn, verify that dirty code still builds.
#
# Runs only the check relevant to what changed (per `git status`):
#   .go dirty            -> go build ./...
#   ui/frontend/ dirty   -> tsc --noEmit
#
# Build / typecheck only — NOT the test suites (too slow for every turn-end; the
# hard gate is the pre-push git hook in devenv.nix). On failure it blocks once
# (exit 2) to nudge a fix; `stop_hook_active` guards against an infinite loop.
set -uo pipefail

root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
[ -n "$root" ] || exit 0
cd "$root" || exit 0

active=$(jq -r '.stop_hook_active // false')

# -uall lists untracked files individually; without it a new file in a new dir
# collapses to "?? dir/" and would slip past the per-language detection below.
dirty=$(git status --porcelain -uall)
[ -n "$dirty" ] || exit 0

fail=0
if printf '%s\n' "$dirty" | grep -qE '\.go$'; then
  if ! out=$(go build ./... 2>&1); then
    { echo "go build ./... failed:"; echo "$out"; } >&2
    fail=1
  fi
fi
if printf '%s\n' "$dirty" | grep -q 'ui/frontend/'; then
  if ! out=$( (cd ui/frontend && ./node_modules/.bin/tsc --noEmit) 2>&1 ); then
    { echo "tsc --noEmit (frontend) failed:"; echo "$out"; } >&2
    fail=1
  fi
fi

[ "$fail" -eq 0 ] && exit 0
[ "$active" = "true" ] && { echo "(checks still failing — not blocking again to avoid a loop)" >&2; exit 0; }
exit 2
