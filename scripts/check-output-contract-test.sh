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
unset TARIBOY_NPM_CI_FORCE
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
if [ "$*" = --version ]; then
  printf '%s\n' "${TEST_NPM_VERSION:-10.0.0}"
  exit 0
fi
printf '%s:npm %s\n' "${PWD##*/}" "$*" >>../events
if [ "$1" = ci ]; then
  [ "${TEST_NPM_FAIL:-0}" != 1 ] || exit 7
  mkdir -p node_modules
fi
EOF
cat >"$fixture/bin/npx" <<'EOF'
#!/bin/sh
printf '%s:npx %s\n' "${PWD##*/}" "$*" >>../events
EOF
cat >"$fixture/bin/node" <<'EOF'
#!/bin/sh
[ "$*" = --version ] || exit 1
printf '%s\n' "${TEST_NODE_VERSION:-v22.0.0}"
EOF
chmod +x "$fixture/bin/go" "$fixture/fake-goroot/bin/gofmt" "$fixture/bin/npm" "$fixture/bin/npx" "$fixture/bin/node"
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

cp "$repo_root/scripts/npm-ci-locked.sh" "$fixture/scripts/"
printf '{}\n' | tee "$fixture/ui/package-lock.json" "$fixture/docs/package-lock.json" "$fixture/ui/package.json" >"$fixture/docs/package.json"
: >"$fixture/events"
PATH="$fixture/bin:$PATH" make --no-print-directory -C "$fixture" frontend-check
test "$(sed -n '1p' "$fixture/events")" = 'ui:npm ci --no-audit --no-fund'
test "$(sed -n '2p' "$fixture/events")" = 'docs:npm ci --no-audit --no-fund'
test "$(sed -n '3p' "$fixture/events")" = 'ui:npx tsc -b'
test "$(sed -n '4p' "$fixture/events")" = 'ui:npm run lint'
test "$(sed -n '5p' "$fixture/events")" = 'ui:npm run lint:unused'
test "$(sed -n '6p' "$fixture/events")" = 'ui:npm test'
echo "Check dependency preparation contract ok"

# The real installer must reuse only successful installs with matching inputs.
run_frontend() {
  PATH="$fixture/bin:$PATH" make --no-print-directory -C "$fixture" frontend-check >"$output" 2>&1
}
test -s "$fixture/ui/node_modules/.tariboy-npm-ci.sha256"
test -s "$fixture/docs/node_modules/.tariboy-npm-ci.sha256"
: >"$fixture/events"
run_frontend
test "$(grep -c 'npm ci' "$fixture/events" || true)" = 0
grep -Fq 'ui: dependencies up to date' "$output"
printf '{"changed": true}\n' >"$fixture/ui/package-lock.json"
: >"$fixture/events"
run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 1
: >"$fixture/events"
TARIBOY_NPM_CI_FORCE=1 run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 2
: >"$fixture/events"
TEST_NPM_VERSION=11.0.0 run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 2
printf '{"dependencies": {"new": "1"}}\n' >"$fixture/ui/package.json"
: >"$fixture/events"
TEST_NPM_VERSION=11.0.0 run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 1
: >"$fixture/events"
TEST_NPM_VERSION=11.0.0 TEST_NODE_VERSION=v24.0.0 run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 2
if TARIBOY_NPM_CI_FORCE=1 TEST_NPM_FAIL=1 run_frontend; then
  echo "FAIL: failed npm ci was ignored" >&2
  exit 1
fi
test ! -e "$fixture/ui/node_modules/.tariboy-npm-ci.sha256"
test "$(grep -c '==> ui-typecheck' "$output" || true)" = 0
: >"$fixture/events"
run_frontend
grep -q 'ui:npm ci' "$fixture/events"
printf 'invalid\n' >"$fixture/ui/node_modules/.tariboy-npm-ci.sha256"
: >"$fixture/events"
run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 1
rm -rf -- "$fixture/ui/node_modules"
: >"$fixture/events"
run_frontend
test "$(grep -c 'npm ci' "$fixture/events")" = 1
echo "Locked npm install cache contract ok"
