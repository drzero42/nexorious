#!/usr/bin/env bash
# Regenerate brand-icon binary assets from the master SVGs.
# Requires librsvg (rsvg-convert) and imagemagick (convert) — provided by devenv.
set -euo pipefail

cd "$(dirname "$0")/.."

LOGO=ui/frontend/public/logo.svg
FAVICON_SVG=ui/frontend/public/favicon.svg

rsvg-convert -w 180 -h 180 "$LOGO" -o ui/frontend/public/apple-touch-icon.png
rsvg-convert -w 256 -h 256 "$LOGO" -o deploy/helm/icon.png

# favicon.ico carries multiple sizes; ImageMagick handles the multi-size pack.
convert -background none "$FAVICON_SVG" \
  -define icon:auto-resize=16,32,48 \
  ui/frontend/public/favicon.ico

echo "Regenerated:"
echo "  ui/frontend/public/apple-touch-icon.png"
echo "  ui/frontend/public/favicon.ico"
echo "  deploy/helm/icon.png"
