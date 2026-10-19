#!/usr/bin/env bash
# Print the one signing certificate SHA-256 in `apksigner verify --print-certs`
# output on stdin. Older build-tools print "Signer #1 certificate SHA-256
# digest:", current ones a line per scheme such as "V2 Signer: certificate
# SHA-256 digest:". No digest, or more than one certificate, fails.
set -euo pipefail

digests="$(sed -nE 's/^(Signer #[0-9]+|V[0-9.]+ Signer)[^:]*:? certificate SHA-256 digest: ([0-9A-Fa-f]+)[[:space:]]*$/\2/p' |
  tr '[:upper:]' '[:lower:]' | sort -u)"
count="$(grep -c . <<<"$digests" || true)"
if [ "$count" -ne 1 ]; then
  echo "expected one APK signing certificate SHA-256, found $count" >&2
  exit 1
fi
printf '%s\n' "$digests"
