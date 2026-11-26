#!/usr/bin/env bash
# Regenerate src/routeTree.gen.ts with the TanStack Router generator from the
# installed (lockfile) versions.
#
# Renovate runs this as a postUpgradeTask on TanStack Router updates, so a
# router-plugin bump that changes the generator's output arrives with the
# regenerated file in Renovate's own commit (the "Route tree is up to date" CI
# step fails otherwise). It also works by hand from anywhere in the repo.
#
# The generator runs inside the Vite plugin, so this does a `vite build`, into a
# throwaway directory: dist/ is left alone and only the route tree changes.
# Dependencies are installed with --ignore-scripts so a freshly bumped package
# gets no install-time code execution in the Renovate runner.
set -euo pipefail

cd "$(dirname "$0")/.."

npm ci --ignore-scripts --no-audit --no-fund

out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

npx --no-install vite build --outDir "$out" --emptyOutDir --logLevel warn
