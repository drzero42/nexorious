#!/bin/sh
# Runs after files are removed.
# deb: $1 = remove | purge | upgrade ...
# rpm: $1 = 0 (uninstall) | 1 (upgrade)
set -e

case "$1" in
    purge)
        # deb purge: true clean uninstall — remove config, ALL data
        # (including backups), and the service account.
        rm -rf /etc/nexorious
        rm -rf /var/lib/nexorious
        if getent passwd nexorious >/dev/null 2>&1; then
            userdel nexorious || true
        fi
        if getent group nexorious >/dev/null 2>&1; then
            groupdel nexorious || true
        fi
        ;;
esac

# deb remove (no purge), rpm uninstall, and any upgrade path: reload only.
# config, key, data, and user remain for a possible reinstall.
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi
