#!/bin/sh
# Download one published server archive into a staging directory created by
# the Desktop app, verify it against the release SHA256SUMS and unpack it.
# remote-install.sh then verifies the unpacked files and switches the links.
set -eu
: "${HOME:?HOME is required}"

version=${1-}
staging=${2-}
archive=${3-}
base_url=${4-}

case "$version" in
  ""|*[!A-Za-z0-9._-]*|.*|*/*) echo "invalid version" >&2; exit 64 ;;
esac
case "$staging" in
  .stage-*) ;;
  *) echo "invalid staging basename" >&2; exit 64 ;;
esac
case "${staging#.stage-}" in
  ""|*[!A-Za-z0-9-]*) echo "invalid staging basename" >&2; exit 64 ;;
esac
case "$archive" in
  "tariboy_${version}_linux-x86_64.tar.gz"|"tariboy_${version}_linux-aarch64.tar.gz"|"tariboy_${version}_darwin-arm64.tar.gz") ;;
  *) echo "invalid archive name" >&2; exit 64 ;;
esac
# Plain HTTP is accepted only from loopback, for a local mirror in tests.
case "$base_url" in
  https://?*|http://127.0.0.1:?*) ;;
  *) echo "invalid release base URL" >&2; exit 64 ;;
esac
case "$base_url" in
  *[!A-Za-z0-9._~:/-]*) echo "invalid release base URL" >&2; exit 64 ;;
esac

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --proto =https,http --proto-redir =https --retry 2 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  echo "curl or wget is required to download $archive" >&2
  exit 69
fi
if command -v sha256sum >/dev/null 2>&1; then
  digest() { sha256sum "$1" | awk '{print $1}'; }
else
  digest() { shasum -a 256 "$1" | awk '{print $1}'; }
fi

stage=$HOME/.local/lib/tariboy/$staging
test -d "$stage"
download=$stage/.download
trap 'rm -rf "$download"' EXIT
rm -rf "$download"
mkdir "$download"

release_url=$base_url/download/v$version
for name in SHA256SUMS "$archive"; do
  echo "download $release_url/$name" >&2
  fetch "$release_url/$name" "$download/$name" || {
    echo "download failed: $release_url/$name" >&2
    exit 1
  }
done

expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name {print $1}' "$download/SHA256SUMS")
case "$expected" in
  "") echo "release SHA256SUMS has no checksum for $archive" >&2; exit 1 ;;
  *[!0-9a-f]*) echo "release SHA256SUMS has an invalid checksum for $archive" >&2; exit 1 ;;
esac
actual=$(digest "$download/$archive")
test "$actual" = "$expected" || {
  echo "checksum mismatch for $archive: expected $expected, got $actual" >&2
  exit 1
}

# Only regular names at the archive root may reach the stage.
tar -tzf "$download/$archive" > "$download/entries"
while IFS= read -r entry; do
  case "$entry" in
    ""|.*|*/*) echo "unsafe archive entry: $entry" >&2; exit 65 ;;
  esac
done < "$download/entries"
tar -xzf "$download/$archive" -C "$stage"
for path in "$stage"/*; do
  if test -L "$path" || ! test -f "$path"; then
    echo "unsafe archive entry: ${path##*/}" >&2
    exit 65
  fi
done
echo "staged $archive" >&2
