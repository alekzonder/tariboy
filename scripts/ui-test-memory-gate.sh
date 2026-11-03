#!/bin/sh
# Waits before ui-test until the kernel reports enough available memory, so
# vitest (about 4.1 GB at peak) does not start on a shared host that is
# already short of memory. frontend-check calls it right before ui-test and
# runs ui-test whatever this script returns: the step is never skipped.
#
#   TARIBOY_UI_TEST_MIN_MEM_MB       threshold in MB (default 3000; 0 = off)
#   TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT wait limit in seconds (default 600)
#   TARIBOY_UI_TEST_MEM_POLL         seconds between reads (default 5)
#   TARIBOY_MEMINFO                  meminfo file (default /proc/meminfo; for tests)
set -u

min_mb="${TARIBOY_UI_TEST_MIN_MEM_MB:-3000}"
timeout="${TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT:-600}"
poll="${TARIBOY_UI_TEST_MEM_POLL:-5}"
meminfo="${TARIBOY_MEMINFO:-/proc/meminfo}"

for pair in "TARIBOY_UI_TEST_MIN_MEM_MB=$min_mb" "TARIBOY_UI_TEST_MEM_WAIT_TIMEOUT=$timeout" "TARIBOY_UI_TEST_MEM_POLL=$poll"; do
	case "${pair#*=}" in
	'' | *[!0-9]*)
		echo "memory gate: ${pair%%=*} must be a whole number, got '${pair#*=}'" >&2
		exit 2
		;;
	esac
done
[ "$min_mb" -eq 0 ] && exit 0
[ "$poll" -ge 1 ] || poll=1

available_mb() {
	awk '/^MemAvailable:/ { print int($2 / 1024); exit }' "$meminfo" 2>/dev/null
}

now=$(available_mb)
if [ -z "$now" ]; then
	echo "memory gate: $meminfo has no MemAvailable, not waiting for memory"
	exit 0
fi
[ "$now" -ge "$min_mb" ] && exit 0

echo "memory gate: waiting for MemAvailable >= $min_mb MB before ui-test (now $now MB, limit ${timeout}s)"
start=$(date +%s)
reported=0
while :; do
	sleep "$poll"
	elapsed=$(($(date +%s) - start))
	now=$(available_mb)
	if [ -z "$now" ]; then
		echo "memory gate: $meminfo has no MemAvailable, not waiting for memory"
		exit 0
	fi
	if [ "$now" -ge "$min_mb" ]; then
		echo "memory gate: MemAvailable $now MB after ${elapsed}s, starting ui-test"
		exit 0
	fi
	if [ "$elapsed" -ge "$timeout" ]; then
		echo "memory gate: MemAvailable $now MB is still below $min_mb MB after ${timeout}s; running ui-test anyway" >&2
		exit 0
	fi
	if [ $((elapsed - reported)) -ge 60 ]; then
		reported=$elapsed
		echo "memory gate: still waiting, MemAvailable $now MB, ${elapsed}s of ${timeout}s"
	fi
done
