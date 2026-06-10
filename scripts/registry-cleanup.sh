#!/usr/bin/env bash
# ONE-TIME registry cleanup. Deletes every ghcr version of the nexorious image
# and charts/nexorious that does NOT carry an X.Y.Z release tag.
#
# Run ONCE during rollout (after the workflow PR merges, before the first
# multi-arch release) with the maintainer's `gh auth`. DO NOT wire to CI and
# DO NOT run after a multi-arch release exists — untagged per-platform
# manifests would be deleted, corrupting released images.
#
# Usage:
#   scripts/registry-cleanup.sh           # dry run: print keep/delete lists
#   scripts/registry-cleanup.sh --delete  # actually delete
set -euo pipefail

OWNER=drzero42
DELETE=0
[ "${1:-}" = "--delete" ] && DELETE=1

# A version is KEPT only if it carries a tag matching X.Y.Z (optional -suffix).
RELEASE_TAG_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$'
# ...but never the dev sentinels. Bare `0.0.0-dev` matches RELEASE_TAG_RE
# (its `dev` suffix is valid pre-release), yet it is the moving dev chart tag
# that must be swept. A real release is never 0.0.0, so exclude 0.0.0-* whole.
DEV_SENTINEL_RE='^0\.0\.0(-|$)'

process_package() {
    local kind="$1" encoded="$2" label="$3"
    echo "=== ${label} ==="
    # List "<version-id> <comma-joined-tags>" for every version.
    gh api --paginate \
        "/users/${OWNER}/packages/${kind}/${encoded}/versions" \
        --jq '.[] | "\(.id) \((.metadata.container.tags // []) | join(","))"' \
    | while read -r id tags; do
        keep=0
        IFS=',' read -ra tag_arr <<< "$tags"
        for t in "${tag_arr[@]}"; do
            if [[ "$t" =~ $RELEASE_TAG_RE ]] && ! [[ "$t" =~ $DEV_SENTINEL_RE ]]; then
                keep=1
                break
            fi
        done
        if [ "$keep" = "1" ]; then
            echo "KEEP   $id  [$tags]"
        else
            echo "DELETE $id  [$tags]"
            if [ "$DELETE" = "1" ]; then
                gh api -X DELETE "/users/${OWNER}/packages/${kind}/${encoded}/versions/${id}"
            fi
        fi
    done
}

process_package container "nexorious"          "image: nexorious"
process_package container "charts%2Fnexorious" "chart: charts/nexorious"

if [ "$DELETE" = "0" ]; then
    echo
    echo "Dry run complete. Re-run with --delete to apply."
fi
