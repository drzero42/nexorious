#!/bin/sh
# Runs after files are unpacked.
# deb: $1 = configure, $2 = previous version (empty on fresh install)
# rpm: $1 = 1 (fresh install) | 2 (upgrade)
set -e

ENV_FILE=/etc/nexorious/nexorious.env

# 1. Generate DB_ENCRYPTION_KEY only if the line is present and empty.
#    A non-empty value is never touched -> upgrade-safe by construction.
if [ -f "$ENV_FILE" ] && grep -q '^DB_ENCRYPTION_KEY=$' "$ENV_FILE"; then
    KEY=$(head -c 32 /dev/urandom | base64)
    # '|' is a safe sed delimiter: it never appears in base64 output.
    sed -i "s|^DB_ENCRYPTION_KEY=\$|DB_ENCRYPTION_KEY=${KEY}|" "$ENV_FILE"
fi

# Determine whether this is an upgrade.
is_upgrade=0
case "$1" in
    configure) if [ -n "${2:-}" ]; then is_upgrade=1; fi ;;  # deb: $2 set => upgrade
    2)         is_upgrade=1 ;;                                # rpm upgrade
esac

# 2. daemon-reload (only when systemd is actually running).
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
    # 3. Upgrade: restart only if already running.
    if [ "$is_upgrade" = "1" ]; then
        systemctl try-restart nexorious.service || true
    fi
fi

# 4. Fresh install: do NOT auto-enable/start; print next steps.
if [ "$is_upgrade" = "0" ]; then
    cat <<'EOF'

Nexorious installed.

Next steps:
  1. Edit /etc/nexorious/nexorious.env and set DATABASE_URL.
     A DB_ENCRYPTION_KEY has been generated for you — do not change it.
  2. Enable and start the service:
       systemctl enable --now nexorious

EOF
fi
