#!/bin/sh
# Runs before files are removed.
# deb: $1 = remove (final) | upgrade | deconfigure ...
# rpm: $1 = 0 (final removal) | 1 (upgrade)
set -e

final_removal=0
case "$1" in
    remove) final_removal=1 ;;   # deb final removal
    0)      final_removal=1 ;;   # rpm final removal
esac

if [ "$final_removal" = "1" ] && [ -d /run/systemd/system ]; then
    systemctl stop nexorious.service || true
    systemctl disable nexorious.service || true
fi
