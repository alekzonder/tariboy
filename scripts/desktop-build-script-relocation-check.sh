#!/usr/bin/env bash
# Checks that `make desktop-rust-check` keeps working when its shared cargo
# target directory holds a build script compiled in a worktree that no longer
# exists, and that a changed internal/version/version.go reaches
# TARIBOY_VERSION. Run it by hand when changing desktop/src-tauri/build.rs.
#
# Usage: scripts/desktop-build-script-relocation-check.sh [REV]
#
# REV (default HEAD) is exported with `git archive` into two throwaway trees,
# a and b, which share a temporary TARIBOY_CARGO_CHECK_DIR. The live cache in
# ~/.cache/tariboy/desktop-check-target is never read or changed. The first run
# compiles the desktop crate from scratch: expect several minutes and several
# gigabytes of disk under TMPDIR, all removed on exit.
set -euo pipefail

rev="${1:-HEAD}"
repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
test_version="0.0.0-relocation-check"

# Hold the host-wide check slot that `make check` uses, unless a parent holds it.
if [ -z "${TARIBOY_CHECK_LOCK_HELD:-}" ]; then
	lock="${TARIBOY_CHECK_LOCK:-/tmp/tariboy-check.lock}"
	if command -v flock >/dev/null 2>&1; then
		exec 9>>"$lock"
		if ! flock -n 9; then
			echo "waiting for a check slot ($lock)"
			flock -w "${TARIBOY_CHECK_LOCK_TIMEOUT:-3600}" 9 || {
				echo "no check slot ($lock); the check did not run" >&2
				exit 1
			}
		fi
		export TARIBOY_CHECK_LOCK_HELD=1
	else
		echo "check slot: flock not found, running without a host lock"
	fi
fi

work="$(mktemp -d "${TMPDIR:-/tmp}/tariboy-relocation.XXXXXX")"
trap 'chmod -R u+w -- "$work" 2>/dev/null; rm -rf -- "$work"' EXIT

export TARIBOY_CARGO_CHECK_DIR="$work/target"
# A caller's TARIBOY_VERSION would fail build.rs's assert for the test version.
unset TARIBOY_VERSION

fail() {
	echo "relocation check: FAIL at step: $1" >&2
	exit 1
}

started=$(date +%s)
mkdir -p "$work/a" "$work/b"
git -C "$repo_root" archive "$rev" | tar -x -C "$work/a"
git -C "$repo_root" archive "$rev" | tar -x -C "$work/b"
# Older sources in b make cargo reuse the build script compiled in a.
find "$work/b" -exec touch -h -d '2000-01-01' {} +

echo "== step 1: desktop-rust-check in a"
make -C "$work/a" --no-print-directory desktop-rust-check || fail "build in a"

rm -rf -- "$work/a"

echo "== step 2: desktop-rust-check in b after a was removed"
make -C "$work/b" --no-print-directory desktop-rust-check || fail "build in b after a was removed"

echo "== step 3: desktop-rust-check in b after version.go changed"
version_go="$work/b/internal/version/version.go"
sed -i "s/^const Version = \".*\"$/const Version = \"$test_version\"/" "$version_go"
grep -q "^const Version = \"$test_version\"$" "$version_go" || fail "set test version"
make -C "$work/b" --no-print-directory desktop-rust-check || fail "build in b after version.go changed"

grep -qsx "cargo:rustc-env=TARIBOY_VERSION=$test_version" \
	"$TARIBOY_CARGO_CHECK_DIR"/debug/build/tariboy-desktop-*/output ||
	fail "TARIBOY_VERSION=$test_version in the build script output"

size=$(du -sh "$work" | cut -f1)
echo "relocation check: temporary cache $size, $(($(date +%s) - started))s"
echo "relocation check: ok"
