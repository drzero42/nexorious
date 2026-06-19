#!/usr/bin/env bash
# Install a nexctl package, assert its shape (a single CLI binary), then
# reinstall and re-assert. nexctl is a pure client — it must NOT bring along
# any of the nexorious server's user/systemd/conffile footprint.
#
# Usage: smoke-test-nexctl.sh <deb|rpm> <path-to-package>
# Intended to run as root inside a clean debian:13 or rockylinux:10 container.
set -euo pipefail

PKG_TYPE="$1"
case "$PKG_TYPE" in
    deb|rpm) ;;
    *) echo "Usage: smoke-test-nexctl.sh <deb|rpm> <path-to-package>" >&2; exit 2 ;;
esac
PKG_PATH="$2"
# apt requires a path-like argument (leading ./ or /).
case "$PKG_PATH" in
    /*|./*) ;;
    *) PKG_PATH="./$PKG_PATH" ;;
esac

install_pkg() {
    if [ "$PKG_TYPE" = "deb" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update
        apt-get install -y "$PKG_PATH"
    else
        dnf install -y "$PKG_PATH"
    fi
}

reinstall_pkg() {
    if [ "$PKG_TYPE" = "deb" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get install -y --reinstall "$PKG_PATH"
    else
        dnf reinstall -y "$PKG_PATH"
    fi
}

echo "=== Installing package ==="
install_pkg

echo "=== Asserting package shape ==="
test -x /usr/bin/nexctl
VERSION_OUT=$(nexctl version)
echo "nexctl version => $VERSION_OUT"
if [ -z "$VERSION_OUT" ]; then
    echo "FAIL: nexctl version produced no output" >&2
    exit 1
fi

# The client package must stay minimal: it must not create the server's system
# user or install a systemd unit. Guard against accidentally inheriting server
# content from the nexorious nfpm config.
if getent passwd nexorious >/dev/null 2>&1; then
    echo "FAIL: nexctl package created a 'nexorious' system user" >&2
    exit 1
fi
if [ -e /usr/lib/systemd/system/nexorious.service ] \
   || [ -e /usr/lib/systemd/system/nexctl.service ]; then
    echo "FAIL: nexctl package installed a systemd unit" >&2
    exit 1
fi
echo "Minimal client footprint OK (no server user, no systemd unit)"

echo "=== Reinstalling (idempotency) ==="
reinstall_pkg
nexctl version >/dev/null

echo "=== SMOKE TEST PASSED ($PKG_TYPE) ==="
