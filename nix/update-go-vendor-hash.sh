#!/usr/bin/env bash
# Recompute goVendorHash in flake.nix after a go.mod/go.sum change.
#
# Renovate runs this as a postUpgradeTask on gomod updates, so Go dependency
# PRs arrive with the correct hash in Renovate's own commit. It also works by
# hand from the repo root.
#
# Only the vendored Go modules (.#nexctl.goModules) are built, not the
# frontend or the binaries: that is the derivation goVendorHash pins, and it is
# shared by .#nexorious and .#nexctl.
set -euo pipefail

cd "$(dirname "$0")/.."

FAKE_HASH="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
HASH_RE='goVendorHash = "sha256-[A-Za-z0-9+/=]*";'

if ! grep -qE "$HASH_RE" flake.nix; then
  echo "goVendorHash line not found in flake.nix" >&2
  exit 1
fi

set_hash() {
  sed -i -E "s|${HASH_RE}|goVendorHash = \"$1\";|" flake.nix
}

# Renovate's -full runner image (containerbase): its nix wrapper keeps the store
# under /tmp/containerbase, which cache.nixos.org can't substitute into, so every
# dependency would be built from source. When /nix is writable (an emptyDir on
# the RenovateJob, drzero42/cloudzero#895), call the real binary instead so it
# uses /nix/store. The wrapper still runs once: its first call sets up the
# containerbase home dirs (~/.cache etc.) that nix needs. Outside that image
# (local dev, CI) there is no /opt/containerbase and plain `nix` is used.
# Done before flake.nix is touched, so a failure here leaves it unchanged.
nix_cmd=(nix)
for b in /opt/containerbase/tools/nix/*/bin/nix; do
  if [ -x "$b" ] && [ -w /nix ]; then
    nix --version >/dev/null
    nix_cmd=("$b" --store local)
  fi
done

old_hash="$(grep -oE "$HASH_RE" flake.nix | grep -oE 'sha256-[A-Za-z0-9+/=]*')"
set_hash "$FAKE_HASH"

log="$(mktemp)"
trap 'rm -f "$log"' EXIT

if "${nix_cmd[@]}" --extra-experimental-features 'nix-command flakes' \
  build --no-link .#nexctl.goModules >"$log" 2>&1; then
  set_hash "$old_hash"
  echo "Build succeeded with a fake hash; refusing to guess." >&2
  exit 1
fi

new_hash="$(grep -E '^\s+got:' "$log" | awk '{print $NF}' | head -n1 || true)"
if [ -z "$new_hash" ]; then
  set_hash "$old_hash"
  echo "No 'got:' hash in the build output:" >&2
  cat "$log" >&2
  exit 1
fi

set_hash "$new_hash"
if [ "$new_hash" = "$old_hash" ]; then
  echo "goVendorHash unchanged: $new_hash"
else
  echo "goVendorHash: $old_hash -> $new_hash"
fi
