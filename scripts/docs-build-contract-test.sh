#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
build_root="${1:-$repo_root/docs/dist}"
site_root="https://alekzonder.github.io/tariboy"

if [ ! -f "$build_root/index.html" ]; then
  echo "FAIL: documentation build is missing index.html" >&2
  exit 1
fi

canonical="$(sed -n 's/.*<link rel="canonical" href="\([^"]*\)".*/\1/p' "$build_root/index.html")"
if [ "$canonical" != "$site_root" ]; then
  echo "FAIL: documentation root canonical is $canonical, expected $site_root" >&2
  exit 1
fi

if ! grep -Fq "<loc>$site_root/architecture</loc>" "$build_root/sitemap.xml"; then
  echo "FAIL: documentation sitemap does not use the GitHub Pages project URL" >&2
  exit 1
fi

if ! grep -Fq 'href="/tariboy/_astro/' "$build_root/index.html"; then
  echo "FAIL: documentation assets do not use the /tariboy base path" >&2
  exit 1
fi

echo "docs build contract ok"
