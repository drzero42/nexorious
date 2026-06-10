#!/usr/bin/env bash
# Install a nexorious package, assert its shape, then reinstall and assert
# that the env conffile edits and the generated key are preserved.
#
# Usage: smoke-test.sh <deb|rpm> <path-to-package>
# Intended to run as root inside a clean debian:13 or rockylinux:9 container.
set -euo pipefail

PKG_TYPE="$1"
case "$PKG_TYPE" in
    deb|rpm) ;;
    *) echo "Usage: smoke-test.sh <deb|rpm> <path-to-package>" >&2; exit 2 ;;
esac
PKG_PATH="$2"
# apt requires a path-like argument (leading ./ or /).
case "$PKG_PATH" in
    /*|./*) ;;
    *) PKG_PATH="./$PKG_PATH" ;;
esac

ENV_FILE=/etc/nexorious/nexorious.env
UNIT=/usr/lib/systemd/system/nexorious.service

install_systemd_and_pkg() {
    if [ "$PKG_TYPE" = "deb" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update
        apt-get install -y systemd
        apt-get install -y "$PKG_PATH"
    else
        dnf install -y systemd
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
install_systemd_and_pkg

echo "=== Asserting package shape ==="
nexorious version
getent passwd nexorious
test -d /var/lib/nexorious
test -f "$ENV_FILE"
systemd-analyze verify "$UNIT"

# The packaged file_info must survive install on both dpkg and rpm.
DIR_PERMS=$(stat -c '%U:%G %a' /var/lib/nexorious)
if [ "$DIR_PERMS" != "nexorious:nexorious 750" ]; then
    echo "FAIL: /var/lib/nexorious perms = '$DIR_PERMS', expected 'nexorious:nexorious 750'" >&2
    exit 1
fi
ENV_PERMS=$(stat -c '%U:%G %a' "$ENV_FILE")
if [ "$ENV_PERMS" != "root:nexorious 640" ]; then
    echo "FAIL: $ENV_FILE perms = '$ENV_PERMS', expected 'root:nexorious 640'" >&2
    exit 1
fi
echo "Permissions OK (dir=$DIR_PERMS env=$ENV_PERMS)"

KEY1=$(grep '^DB_ENCRYPTION_KEY=' "$ENV_FILE" | cut -d= -f2-)
if [ -z "$KEY1" ]; then
    echo "FAIL: DB_ENCRYPTION_KEY was not generated" >&2
    exit 1
fi
echo "Generated key present (len=${#KEY1})"

echo "=== Editing env file, then reinstalling ==="
sed -i 's|^DATABASE_URL=.*|DATABASE_URL=postgres://smoke@example|' "$ENV_FILE"
reinstall_pkg

echo "=== Asserting preservation ==="
KEY2=$(grep '^DB_ENCRYPTION_KEY=' "$ENV_FILE" | cut -d= -f2-)
if [ "$KEY1" != "$KEY2" ]; then
    echo "FAIL: key changed across reinstall ($KEY1 -> $KEY2)" >&2
    exit 1
fi
if ! grep -q '^DATABASE_URL=postgres://smoke@example' "$ENV_FILE"; then
    echo "FAIL: DATABASE_URL edit was not preserved across reinstall" >&2
    exit 1
fi

echo "=== SMOKE TEST PASSED ($PKG_TYPE) ==="
