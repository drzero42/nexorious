#!/bin/sh
set -e

# Sync node_modules if package.json changed
# This handles the case where the mounted volume has stale dependencies
if [ -f /app/package.json ]; then
    echo "Checking if dependencies need to be updated..."
    npm install --prefer-offline 2>/dev/null || npm install
    echo "Dependencies are up to date."
fi

exec "$@"
