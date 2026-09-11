#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd -P)"
output="$(mktemp)"
trap 'rm -f -- "$output"' EXIT

if make --no-print-directory -C "$repo_root" check-output-contract-fixture >"$output" 2>&1; then
  echo "FAIL: output fixture unexpectedly passed" >&2
  exit 1
fi

if grep -Fq 'SUCCESS_DETAIL' "$output" || grep -Fq 'AFTER_DETAIL' "$output"; then
  echo "FAIL: successful step output was not suppressed" >&2
  cat "$output" >&2
  exit 1
fi

grep -Fq 'success-step' "$output"
grep -Fq '==> success-step' "$output"
grep -Eq 'success-step +ok +[0-9]+s' "$output"
grep -Fq 'failure-step' "$output"
grep -Eq 'failure-step +FAIL +[0-9]+s' "$output"
grep -Fq 'command: echo FAILURE_DETAIL; exit 7' "$output"
grep -Fq 'FAILURE_DETAIL' "$output"
grep -Fq 'after-step' "$output"
grep -Eq 'after-step +ok +[0-9]+s' "$output"
grep -Fq 'check-output-contract-fixture FAILED' "$output"

echo "check output contract ok"

# Fast checks delegate Tasks E2E without invoking the workspace build target.
# Its script owns isolated builds; here a fixture observes the Make dependency.
fixture="$(mktemp -d)"
trap 'rm -f -- "$output"; rm -rf -- "$fixture"' EXIT
cp "$repo_root/Makefile" "$fixture/Makefile"
mkdir "$fixture/scripts"
mkdir -p "$fixture/internal/version"
cp "$repo_root/internal/version/version.go" "$fixture/internal/version/version.go"
printf '#!/bin/sh\ntouch ran-e2e\n' >"$fixture/scripts/tariboy-tasks-e2e.sh"
chmod +x "$fixture/scripts/tariboy-tasks-e2e.sh"
make --no-print-directory -C "$fixture" GO=false VERSION=test tariboy-tasks-e2e
test -f "$fixture/ran-e2e"
test ! -e "$fixture/bin"
echo "Tasks check build isolation contract ok"

# Dependency preparation must finish before either fast check starts.
mkdir -p "$fixture/bin" "$fixture/fake-goroot/bin" "$fixture/empty-go" "$fixture/ui" "$fixture/docs"
cat >"$fixture/bin/go" <<'EOF'
#!/bin/sh
printf 'go:%s\n' "$*" >>events
case "$*" in
  'list -f {{.Dir}} ./...') printf '%s\n' "$PWD/empty-go" ;;
  'env GOROOT') printf '%s\n' "$PWD/fake-goroot" ;;
  'list ./...') printf '%s\n' example.invalid/pkg ;;
esac
EOF
cat >"$fixture/fake-goroot/bin/gofmt" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$fixture/bin/npm" <<'EOF'
#!/bin/sh
printf '%s:npm %s\n' "${PWD##*/}" "$*" >>../events
EOF
cat >"$fixture/bin/npx" <<'EOF'
#!/bin/sh
printf '%s:npx %s\n' "${PWD##*/}" "$*" >>../events
EOF
chmod +x "$fixture/bin/go" "$fixture/fake-goroot/bin/gofmt" "$fixture/bin/npm" "$fixture/bin/npx"
for script in tariboy-tasks-e2e.sh tariboy-smoke-contract-test.sh package-alpha-contract-test.sh tariboy-branding-contract-test.sh make-clean-contract-test.sh server-install-contract-test.sh publish-docs-contract-test.sh; do
  printf '#!/bin/sh\nexit 0\n' >"$fixture/scripts/$script"
  chmod +x "$fixture/scripts/$script"
done
cat >"$fixture/scripts/check-output-contract-test.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$fixture/scripts/docs-build-contract-test.sh" <<'EOF'
#!/bin/sh
printf 'docs:contract\n' >>../events
EOF
chmod +x "$fixture/scripts/check-output-contract-test.sh" "$fixture/scripts/docs-build-contract-test.sh"

: >"$fixture/events"
PATH="$fixture/bin:$PATH" make --no-print-directory -C "$fixture" GO=go backend-check
test "$(sed -n '1p' "$fixture/events")" = 'go:mod download'

: >"$fixture/events"
PATH="$fixture/bin:$PATH" make --no-print-directory -C "$fixture" frontend-check
test "$(sed -n '1p' "$fixture/events")" = 'ui:npm ci'
test "$(sed -n '2p' "$fixture/events")" = 'docs:npm ci'
test "$(sed -n '3p' "$fixture/events")" = 'ui:npx tsc -b'
echo "Check dependency preparation contract ok"
