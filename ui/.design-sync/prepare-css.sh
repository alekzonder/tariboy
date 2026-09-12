#!/usr/bin/env bash
# Tariboy UI ships no static stylesheet: Tailwind v4 is compiled by the Vite
# plugin at app-build time. This produces one for design-sync by running the
# real desktop build and staging its CSS next to the Geist woff2 files it
# references (the @font-face urls are same-dir relative).
set -euo pipefail
cd "$(dirname "$0")/.."
npx vite build -c vite.desktop.config.ts >/dev/null
out=".design-sync/.cache/css"
rm -rf "$out" && mkdir -p "$out"
cp ../desktop/dist/assets/*.woff2 "$out"/
cp "$(ls ../desktop/dist/assets/*.css | head -1)" "$out/styles.css"
# Note: the `/* @kind other */` token annotation is NOT applied here. It runs
# against ds-bundle/_ds_bundle.css from .design-sync/overrides/css.mjs, because
# the converter appends CSS that never passes through this file - see the fork's
# header comment.
echo "staged $out/styles.css ($(wc -c < "$out/styles.css") bytes) + $(ls "$out"/*.woff2 | wc -l) fonts"
