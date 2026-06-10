#!/bin/sh
# Runs before files are unpacked, on both fresh install and upgrade.
# deb: $1 = install | upgrade        rpm: $1 = 1 (install) | 2 (upgrade)
# Create the nexorious system user/group if absent so packaged ownership
# (conffile group, data dir) resolves on both dpkg and rpm.
set -e

if ! getent group nexorious >/dev/null 2>&1; then
    groupadd --system nexorious
fi

if ! getent passwd nexorious >/dev/null 2>&1; then
    useradd --system --gid nexorious \
        --home-dir /var/lib/nexorious \
        --shell /usr/sbin/nologin \
        --comment "Nexorious service account" \
        nexorious
fi
