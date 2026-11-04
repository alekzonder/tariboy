# ui-test: two-worker minimum, vmMemoryLimit tuning and memory gate threshold

This report covers three follow-ups to
`docs/superpowers/specs/2026-11-04-ui-test-memory-limit-measurement.md`
(the 336 MiB `vmMemoryLimit` change) and to
`docs/superpowers/specs/2026-11-04-ui-test-single-worker-report.md`:

- `ui/vite.config.ts` now uses at least two vitest workers.
- The report measures why the CI `frontend-check` job became 18 % slower
  after the 336 MiB change, and tests four values of `vmMemoryLimit`.
- `scripts/ui-test-memory-gate.sh` gets a threshold from a measurement.

## Summary

| Item | Result |
| --- | --- |
| Worker minimum | `TARIBOY_VITEST_MAX_WORKERS` below 2 gives 2 workers and a warning on stderr. An empty or non-numeric value gives 4, as before. |
| Cause of the slower CI job | Worker restarts. With 336 MiB and 4 workers, vitest restarts workers 19 times per run. On one runner, `npm test` took 117 s on average with 336 MiB and 101 s with the old limit (0 restarts): about 0.8 s per restart, 16 s per run. Runner variation adds more noise: the attempts of one commit differed by up to 68 s in CI. |
| Chosen `vmMemoryLimit` | 336 MiB, unchanged. No value met both conditions: 512 MiB met the memory condition (1656 MB) but not the time condition (median 175 s). 768 and 1024 MiB failed the memory condition. |
| Memory gate threshold | 1700 MB: peak 1415 MB + 20 % = 1698 MB, rounded up to 100 MB. Before: 3000 MB. |

## Two-worker minimum

`ui/vite.workers.ts` holds `vitestMaxWorkers(raw, warn)`.
`ui/vite.config.ts` calls it with `process.env.TARIBOY_VITEST_MAX_WORKERS` and
`console.warn`. The unit test `ui/vite.workers.test.ts` was written first and
failed because the module did not exist. It covers:

| Value | Workers | Warning |
| --- | ---: | --- |
| unset, `""`, `" "`, `abc` | 4 | no |
| `1`, `0`, `-3`, `1.5` | 2 | once, with the value |
| `2`, `4`, `6` | the value | no |

Run on the host with the variable at 1:

```text
$ cd ui && flock -o /tmp/tariboy-check.lock env TARIBOY_VITEST_MAX_WORKERS=1 npm test
ui-test: TARIBOY_VITEST_MAX_WORKERS=1 is below 2; using 2 workers, because one vmThreads worker runs all test files in one VM context
 Test Files  304 passed (304)
      Tests  2784 passed (2784)
ui-test: vitest finished 304 of 304 test files
real	3m49.551s
```

Before this change, the same command ran one worker, failed tests and took
30 minutes (see the single-worker report).

## Cause of the slower CI job

### Hypothesis

In CI, `TARIBOY_VITEST_MAX_WORKERS` is not set, so vitest runs 4 workers.
After each test file, vitest compares the heap of a `vmThreads` worker with
`vmMemoryLimit`. If the heap is larger, vitest stops the worker and starts a
new thread. The new thread loads `jsdom`, the setup files and the shared
modules again. A smaller limit gives more restarts.

### Counting restarts

A temporary setup file, `ui/src/test/diag-workers.ts`, appended one line per
test file to the file in `DIAG_LOG`: the `threadId` of the worker, the heap in
use and the test file. Every worker is a new thread, so the number of distinct
`threadId` values minus the number of workers is the number of restarts. A
temporary change to `ui/vite.config.ts` read the limit from `DIAG_VM_LIMIT`
(MiB, or `heap60` for the old limit, 60 % of the heap limit). Both were
removed before review; the full text is in the appendix.

### Result in CI, one runner

One job ran `npm test` for each limit, in a fixed order, on one runner
(4 CPUs, Intel Xeon Platinum 8370C, 15983 MB, Node.js v22.23.4, 4 workers).
A second job ran the limits in the reverse order, twice. Every run finished
304 of 304 test files.

| Limit | Restarts | Max worker heap, MB (forward, reverse 1) | `npm test`, forward, s | reverse 1, s | reverse 2, s | Mean, s |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 336 MiB | 19 | 352, 362 | 120 | 115 | 116 | 117.0 |
| 512 MiB | 10, 10, 9 | 519, 527 | 109 | 108 | 109 | 108.7 |
| 768 MiB | 4 | 782, 779 | 107 | 106 | 105 | 106.0 |
| 1024 MiB | 4 | 1032, 1030 | 104 | 103 | 101 | 102.7 |
| old limit (60 % of heap) | 0 | 1364, 1355 | 102 | 101 | 101 | 101.3 |

The order does not change the result. The time falls with the number of
restarts: (117.0 − 101.3) s / 19 restarts = 0.83 s per restart. The 336 MiB
limit adds about 16 s (15 %) to `npm test` against the old limit. One run
(768 MiB, reverse 2) exited 1 with 304 of 304 files finished: one test failed.
That run is in the table, because the failure does not change the time.

### Result on the host

The host (6 CPUs, 5924 MB shared by all agents, Node.js v24.22.2) ran
2 workers. The heap limit of a worker thread on the host is larger, so the
old limit still restarts twice.

| Limit | Restarts | Max worker heap, MB | `npm test` time, s |
| --- | ---: | ---: | --- |
| 336 MiB | 21, 21 | 373, 358 | 231.9, 236.1 |
| 512 MiB | 12, 12 | 533, 537 | 222.1, 218.6 |
| 768 MiB | 7, 7 | 790, 797 | 217.5, 223.6 |
| 1024 MiB | 4, 4 | 1048, 1036 | 220.9, 217.9 |
| old limit | 2 | 1345 | 217.8 |

336 MiB took 234 s on average, the other values 220 s: 14 s more for about
13 more restarts, about 1 s each, close to the cost in CI.

### Runner variation

The same commit gave very different job times. The `ui-test` step falls into
two groups: 71 to 91 s and 101 to 125 s. Both groups occur for every limit
(see the CI table below). One commit, 336 MiB, took 136 to 194 s; one commit,
768 MiB, took 111 to 179 s. The runner of the diagnostic jobs (Xeon 8370C)
was in the slow group: its `ui-test` step took 118 and 124 s. The CPU model of
the other runners was not recorded.

### Conclusion

The proved cause is worker restarts: 19 restarts at 336 MiB against 0 with
the old limit, about 0.8 s each, 16 s per CI run. The rest of the 28 s
difference between the medians in the earlier report (152 s and 180 s, three
runs each) is runner variation, which is up to 68 s for one commit. The
earlier report already showed this: one run after the change took 139 s, and
the `ui-test` step took 78, 99, 98 s on `main` and 122, 90, 126 s after the
change.

## vmMemoryLimit values

### Conditions

- The peak anon memory of `make check` on the host is at most 1800 MB.
- The median time of the CI `frontend-check` job is at most 160 s.

### Table

| Limit | Host `make check` peak anon, MB (runs) | Host `make check` `memory.peak`, MB | Host `npm test` peak (`memory.peak`), MB | Host `npm test` peak anon, MB | CI job times, s | CI job median of 5, s (of first 3) | CI `ui-test` step, s | Restarts host / CI |
| --- | --- | ---: | --- | --- | --- | ---: | --- | --- |
| 336 MiB | 1290 (1 run) | 2377 | 1415, 1330 | 1336, 1272 | 136, 194, 182, 182, 187 | 182 (182) | 91, 124, 125, 124, 125 | 21 / 19 |
| 512 MiB | 1656 (1 run) | 2287 | 1628, 1729 | 1550, 1627 | 112, 179, 178, 175, 124 | 175 (178) | 71, 119, 117, 114, 73 | 12 / 10 |
| 768 MiB | 2032 (1 run) | 3095 | 2180, 2101 | 2058, 2005 | 179, 111, 123, 152, 131 | 131 (123) | 115, 71, 73, 101, 82 | 7 / 4 |
| 1024 MiB | 2665 (1 run) | 3584 | 2713, 2873 | 2581, 2751 | 111, 129, 173, 137, 162 | 137 (129) | 71, 77, 109, 87, 106 | 4 / 4 |

`make check` ran once for each limit, so no spread is given. The second run
for a value near 1800 MB was not needed: 512 MiB was 144 MB below the limit
and failed the time condition, and 768 MiB was 232 MB above it.
`memory.peak` of `make check` includes the page cache of the Go `test` step,
which is outside this task (see the earlier report); the condition uses anon
memory.

### Choice

| Limit | Memory | Time | Result |
| --- | --- | --- | --- |
| 336 MiB | 1290 MB, pass | 182 s, fail | no |
| 512 MiB | 1656 MB, pass | 175 s, fail | no |
| 768 MiB | 2032 MB, fail | 131 s, pass | no |
| 1024 MiB | 2665 MB, fail | 137 s, pass | no |

No value meets both conditions, so the limit stays at 336 MiB, as the task
requires (host memory first). A limit only for CI was out of scope.

The medians of 768 and 1024 MiB are low because more of their runs landed on
fast runners: their `ui-test` steps of 71 to 82 s match the fast group of
336 and 512 MiB. The one-runner measurement gives the effect of the limit
alone: 768 MiB saves 11 s and 512 MiB saves 8 s against 336 MiB. On a slow
runner, even the old limit (0 restarts) would give about 182 − 16 = 166 s, so
the 160 s condition depends on the runner more than on the limit.

## Memory gate threshold

| Item | Value |
| --- | --- |
| Peak | 1415 MB: the larger `memory.peak` of two host runs of `npm test` with 2 workers and 336 MiB (1415 and 1330 MB) |
| Margin | 20 %: 1415 × 1.2 = 1698 MB |
| Threshold | 1700 MB, rounded up to 100 MB |
| Source | `ui-test-336-1` and `ui-test-336-2` in this report, `measure.py`, 304 test files |
| Before | 3000 MB, sized for the old peak of about 4.1 GB |

The earlier report measured 1523 and 1479 MB for the same limit with 303 test
files. 1700 MB is 12 % above 1523 MB.

`scripts/ui-test-memory-gate-test.sh` was changed first to expect 1700 MB and
failed against the old script; the script then passed it.

## Before and after

The limit did not change, so `ui-test` with 2 or more workers runs as before.
The two-worker minimum changes only runs with a value below 2.

| Measurement | Before (`main`, earlier report) | After (this branch) |
| --- | --- | --- |
| Host `npm test` peak, 2 workers | 1523, 1479 MB | 1415, 1330 MB |
| Host `npm test` time, 2 workers | 222, 224 s | 232, 236 s |
| Host `make check` peak anon | 1317 MB | 1290 MB |
| Host `ui-test` with `TARIBOY_VITEST_MAX_WORKERS=1` | 10 of 303 files after 1800 s, timeout | 304 of 304 files, 229 s |
| CI `frontend-check` job | 180 s (median of 180, 139, 185) | 182 s (median of 136, 194, 182, 182, 187) |
| Memory gate threshold | 3000 MB | 1700 MB |

The host times include a temporary setup file that wrote one line per test
file. The CI times have no measurement inside the job.

## Method

### Host

| Item | Value |
| --- | --- |
| Date | 2026-11-04 |
| Base | `main` at `96d51c89` and the two-worker commit |
| vitest workers | 2 (`TARIBOY_VITEST_MAX_WORKERS=2`) |

One `make check` without a measurement warmed the caches first. Each
measurement then took the host lock alone. `measure.py` is the same script as
in the earlier report (its appendix has the full text): it runs the command in
its own cgroup with `systemd-run --user --scope`, reads `memory.peak`, and
samples `anon` every second.

```bash
D=<agent workdir>
WT=<worktree>
export TARIBOY_VITEST_MAX_WORKERS=2
for L in 336 512 768 1024; do
  for N in 1 2; do
    DIAG_VM_LIMIT=$L DIAG_LOG=$D/out/diag-$L-$N.log flock -o /tmp/tariboy-check.lock \
      python3 -I $D/measure.py $D/out ui-test-$L-$N $WT/ui -- npm test
  done
  rm -rf $WT/ui/node_modules/.tmp
  DIAG_VM_LIMIT=$L TARIBOY_DOCS_REBUILD=1 TARIBOY_CHECK_LOCK=$D/measure-check.lock \
    flock -o /tmp/tariboy-check.lock \
    python3 -I $D/measure.py $D/out check-$L-1 $WT -- make check
done
DIAG_VM_LIMIT=heap60 DIAG_LOG=$D/out/diag-heap60-1.log flock -o /tmp/tariboy-check.lock \
  python3 -I $D/measure.py $D/out ui-test-heap60-1 $WT/ui -- npm test
# restarts: distinct thread IDs minus 2 workers
cut -d' ' -f1 $D/out/diag-$L-$N.log | sort -u | wc -l
```

The measured `make check` runs ended with exit code 2: the Go test
`TestPullRequestsRunCheckGroupsAsParallelCachedJobs` failed because the
temporary CI step added a fourth step to the `frontend-check` job. That test
reads only the workflow file, so it does not change the memory or the time
of `ui-test`. All frontend steps passed.

`make check` step times on the host, from its summary table:

| Step | 336 MiB | 512 MiB | 768 MiB | 1024 MiB |
| --- | ---: | ---: | ---: | ---: |
| `ui-test` | 236 s | 222 s | 225 s | 214 s |
| `frontend-check` | 284 s | 271 s | 274 s | 265 s |
| total | 460 s | 446 s | 455 s | 441 s |

### CI

Each limit was one temporary commit that changed only the number in
`vmMemoryLimit`, so its job ran without a measurement. The first run of each
commit and four reruns of its `frontend-check` job
(`gh run rerun RUN --job JOB`) gave five attempts. The job time is
`completed_at - started_at` of the job of each attempt
(`gh api repos/OWNER/REPO/actions/runs/RUN/attempts/N/jobs`). The `ui-test`
time is from the summary table of `make frontend-check`
(`gh run view RUN --attempt N --log`).

| Limit | Commit | Attempts |
| --- | --- | --- |
| 336 MiB | `412ea14a` (the two-worker commit) | [1](https://github.com/alekzonder/tariboy/actions/runs/41791777941/job/126446572993), [2](https://github.com/alekzonder/tariboy/actions/runs/41791777941/job/126447671164), [3](https://github.com/alekzonder/tariboy/actions/runs/41791777941/job/126449036153), [4](https://github.com/alekzonder/tariboy/actions/runs/41791777941/job/126454766245), [5](https://github.com/alekzonder/tariboy/actions/runs/41791777941/job/126455966217) |
| 512 MiB | `227e43d1` (temporary) | [1](https://github.com/alekzonder/tariboy/actions/runs/41791834921/job/126446752838), [2](https://github.com/alekzonder/tariboy/actions/runs/41791834921/job/126449230035), [3](https://github.com/alekzonder/tariboy/actions/runs/41791834921/job/126450403629), [4](https://github.com/alekzonder/tariboy/actions/runs/41791834921/job/126456176662), [5](https://github.com/alekzonder/tariboy/actions/runs/41791834921/job/126457351290) |
| 768 MiB | `2233f23d` (temporary) | [1](https://github.com/alekzonder/tariboy/actions/runs/41791874494/job/126446883290), [2](https://github.com/alekzonder/tariboy/actions/runs/41791874494/job/126450604395), [3](https://github.com/alekzonder/tariboy/actions/runs/41791874494/job/126451412815), [4](https://github.com/alekzonder/tariboy/actions/runs/41791874494/job/126457554917), [5](https://github.com/alekzonder/tariboy/actions/runs/41791874494/job/126458537704) |
| 1024 MiB | `9172fc1a` (temporary) | [1](https://github.com/alekzonder/tariboy/actions/runs/41791914762/job/126447007646), [2](https://github.com/alekzonder/tariboy/actions/runs/41791914762/job/126451626636), [3](https://github.com/alekzonder/tariboy/actions/runs/41791914762/job/126452617260), [4](https://github.com/alekzonder/tariboy/actions/runs/41791914762/job/126458743206), [5](https://github.com/alekzonder/tariboy/actions/runs/41791914762/job/126459730893) |
| Restarts, forward | `9025cea2` (temporary) | [run 41792055838](https://github.com/alekzonder/tariboy/actions/runs/41792055838/job/126447455478) |
| Restarts, reverse, twice | `a000ed04` (temporary) | [run 41794327355](https://github.com/alekzonder/tariboy/actions/runs/41794327355/job/126454725180) |

The plan asked for three attempts. Two more were added because the attempts
of one commit differed by up to 68 s. The medians of the first three attempts
are in the table above and lead to the same choice.

## Appendix: temporary files

`ui/src/test/diag-workers.ts`, added to `setupFiles`:

```ts
import { appendFileSync } from "node:fs";
import { threadId } from "node:worker_threads";
import { beforeAll, expect } from "vitest";

const log = process.env.DIAG_LOG;
if (log) {
  beforeAll(() => {
    const heapMb = Math.round(process.memoryUsage().heapUsed / 1048576);
    appendFileSync(log, `${threadId} ${heapMb} ${expect.getState().testPath ?? "?"}\n`);
  });
}
```

`ui/vite.config.ts`, the limit:

```ts
vmMemoryLimit:
  process.env.DIAG_VM_LIMIT === "heap60"
    ? Math.floor(v8.getHeapStatistics().heap_size_limit * 0.6)
    : Number(process.env.DIAG_VM_LIMIT || 336) * 1024 * 1024,
```

The step after `make frontend-check` in the `frontend-check` job
(the reverse version; the forward version had
`for limit in 336 512 768 1024 heap60`):

```yaml
- name: Count vitest worker restarts
  if: always()
  working-directory: ui
  run: |
    nproc; free -m; node --version; lscpu | grep "Model name"
    for limit in heap60 1024 768 512 336 heap60 1024 768 512 336; do
      log=/tmp/diag-$limit.log; rm -f "$log"
      start=$(date +%s); rc=0
      DIAG_VM_LIMIT=$limit DIAG_LOG=$log npm test >/tmp/test-$limit.out 2>&1 || rc=$?
      wall=$(( $(date +%s) - start ))
      threads=$(cut -d' ' -f1 "$log" | sort -u | wc -l)
      echo "DIAG limit=$limit rc=$rc wall_s=$wall files=$(wc -l <"$log") threads=$threads restarts=$((threads - 4))"
      grep -E "ui-test:|Test Files" /tmp/test-$limit.out
    done
```
