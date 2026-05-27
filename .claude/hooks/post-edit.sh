#!/usr/bin/env bash
# PostToolUse hook (Edit|Write): format + lint the file Claude just touched.
#
# Reads the tool-call JSON on stdin and acts on `.tool_input.file_path`:
#   *.go            -> gofmt -w, then golangci-lint on the file's package
#   ui/frontend/*   -> prettier --write, then (for js/ts) eslint --fix
#
# A non-zero golangci-lint / eslint result exits 2, which routes the captured
# output back to Claude so it fixes the finding before moving on. Anything else
# (other file types, missing file) is a no-op. Assumes go / golangci-lint / jq
# are on PATH (launch Claude Code from inside `devenv shell`).
set -uo pipefail

root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
[ -n "$root" ] || exit 0

file=$(jq -r '.tool_input.file_path // empty')
[ -n "$file" ] && [ -f "$file" ] || exit 0

case "$file" in
  *.go)
    gofmt -w "$file"
    dir=$(dirname "$file")
    rel="${dir#"$root"/}"
    if ! out=$(cd "$root" && golangci-lint run "./$rel" 2>&1); then
      { echo "golangci-lint findings in ./$rel:"; echo "$out"; } >&2
      exit 2
    fi
    ;;
  "$root"/ui/frontend/*)
    fe="$root/ui/frontend"
    "$fe/node_modules/.bin/prettier" --write "$file" >/dev/null 2>&1 || true
    case "$file" in
      *.ts | *.tsx | *.js | *.jsx)
        if ! out=$(cd "$fe" && ./node_modules/.bin/eslint --fix "$file" 2>&1); then
          { echo "eslint findings in $file:"; echo "$out"; } >&2
          exit 2
        fi
        ;;
    esac
    ;;
esac
exit 0
