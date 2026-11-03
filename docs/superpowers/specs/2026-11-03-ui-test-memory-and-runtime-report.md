# ui-test memory and test runtime report

This report finds the cause of the memory peak of the `ui-test` step of
`make check`. It compares vitest pools, the `jsdom` and `happy-dom`
environments, and `bun test`. It also measures the other JavaScript steps of
`frontend-check`. It does not change code, configuration or CI. The base values
come from `2026-11-03-make-check-performance-report.md` (the base report).

## Summary

- The peak comes from the `vmThreads` pool. Each test file runs in a new VM
  context, and the worker keeps what the file loaded. The heap of a worker grows
  by 15.7 MB per file on average. Vitest restarts a worker only when its heap
  passes `vmMemoryLimit`. Thus the peak is close to
  `workers × vmMemoryLimit + 0.6 to 0.9 GB`.
- On the current `main` (`ae321c4d`), `vmMemoryLimit` is 60 % of the worker
  heap limit, 1344 MiB. With 2 workers the peak is 3418 to 3565 MB
  (`memory.peak` of the run cgroup) and the time is 261 to 278 s. The 4.1 GB
  of the base report was measured before `vmMemoryLimit` was set.
- An absolute `vmMemoryLimit` of 336 MiB keeps every file passing and lowers
  the peak to 1566 MB (minus 56 %). The time stays the same: 255 s. This is
  the recommendation.
- The `threads` and `forks` pools lower the peak to 1148 to 1310 MB, but they
  take 2.1 to 2.2 times longer (560 to 587 s).
- `happy-dom` with `vmThreads` is 15 % faster (225 s) at the same peak, but 10
  files (17 tests) fail without changes.
- `bun test` on Bun 1.4.4 passes 135 of 304 files without changes. 130 files
  use `vi.*` functions that Bun does not have. Its peak (3533 MB) is not lower.
  Its memory does not grow steadily, so the Bun claim is confirmed for this
  suite, but short spikes reach 3.1 to 3.4 GB. A move to `bun test` is not
  recommended.
- `ui-lint` (oxlint) and `ui-typecheck` (native `tsc` 7) already use native
  tools. `tsc -b --checkers 1` lowers the `ui-typecheck` peak from 1378 MB to
  888 MB for 1.3 s more. `docs` (1337 MB) and `knip` (402 MB) have no
  suitable native replacement.

## Method

### Host and versions

| Item | Value |
| --- | --- |
| Date | 2026-11-03 |
| Commit | `ae321c4d746d272754f55f2ca08adb3a8dabf5aa` (`main`) |
| `nproc`, RAM | 6, 5924 MB (`MemTotal` 6066680 kB), shared by all agents |
| Node.js | v24.22.2, heap limit of a worker thread 2240 MiB |
| vitest, vite | 4.1.7, 8 |
| jsdom, happy-dom | 29.1.1, 21.0.4 (`happy-dom` and `@happy-dom/global-registrator` installed only in the research worktree) |
| Bun | `bun --version` 1.4.4, `bun --revision` 1.4.4+38afee6c8, the latest release (`bun-v1.4.4`, 2026-10-22), unpacked into the agent workdir |
| TypeScript, oxlint, knip | 7.0.2 (native `tsc`), ^1.81.0, 6.40.0 |
| Docs | Blume 1.0.4 on Astro 7.1.0 and Vite 8.1.5 |

### Measurement of one run

Each run took the host lock alone and ran alone:

```bash
export TARIBOY_VITEST_MAX_WORKERS=2
flock -o /tmp/tariboy-check.lock \
  python3 -I measure.py OUTDIR NAME CWD -- COMMAND...
```

`measure.py` (full text in the appendix) does these steps:

1. It runs `scripts/ui-test-memory-gate.sh`, which waits until `MemAvailable`
   is at least 3000 MB. No run waited.
2. It records `MemAvailable` before the run.
3. It starts the command in its own cgroup with
   `systemd-run --user --scope -- /usr/bin/time -v COMMAND`.
4. Once per second, it writes the sum of RSS of the process tree, the
   `memory.current` of the cgroup and the largest processes to a CSV file.
5. After the run, it reads `memory.peak` of the cgroup (the "peak" column of
   this report), Max RSS of `/usr/bin/time -v`, and the wall time.

Each run took its own lock, so the `make check` of other agents ran between
the runs. One run of each variant was made, and two of the base variant A.
The difference between A1 and A2 is 146 MB and 8 s. This report does not treat
a smaller difference as real.

For the cause of the peak, a temporary setup file `measure-hook.ts` ran after
`src/test/setup.ts`. After each test file, it appended one JSON line with the
file name, the worker thread id, and `process.memoryUsage()` before and after
the file. A temporary config `vite.measure.config.ts` added the hook. Both
files are in the appendix. They are not in the repository.

The variants changed only the command line, for example
`npm test -- --pool=threads --environment=happy-dom` or
`npm test -- --vmMemoryLimit=352321536`. No configuration file changed.

The runs A to G had one more trivial test file (a probe of the `vi` object,
one test). This gives 304 files instead of 303. The runs L30 and L15 ran the
303 files of `main`.

## Cause of the peak

### Memory over time

The sum of RSS of the base run grows steadily. It drops only when vitest
restarts a worker. Maximum of the sum of RSS in each tenth of the run (CSV of
the sampler, MB):

| Run | 0–10 % | 10–20 % | 20–30 % | 30–40 % | 40–50 % | 50–60 % | 60–70 % | 70–80 % | 80–90 % | 90–100 % |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A1, `vmThreads` + `jsdom` | 1308 | 1743 | 1932 | 2465 | 2694 | 3082 | 3370 | 3404 | 2433 | 3282 |
| A2 | 1152 | 1576 | 1710 | 2157 | 2417 | 2823 | 3271 | 3500 | 2336 | 3539 |
| L15, `vmMemoryLimit` 336 MiB | 1127 | 1420 | 1461 | 1473 | 1483 | 1440 | 1417 | 1482 | 1474 | 1665 |
| B, `threads` + `jsdom` | 1313 | 1010 | 1044 | 1110 | 1312 | 1387 | 1249 | 1315 | 1319 | 1277 |
| C, `forks` + `jsdom` | 1365 | 1283 | 1132 | 1047 | 1123 | 1004 | 993 | 910 | 886 | 977 |

The slope of A over the middle 80 % of the run is 474 to 589 MB per 100 s.
B grows by 76 MB per 100 s, and C goes down. So the growth belongs to the
`vmThreads` pool. The vitest documentation warns about this pool: it can leak
memory, and `vmMemoryLimit` is the remedy.

### Memory per worker

The hook run with 2 workers (Ahook2: 3565 MB, 278 s, 304 of 304 files pass)
shows four worker threads. Two workers ran first and were restarted once.

| Thread | Files | Time, s | Heap after the last file, MB |
| --- | --- | --- | --- |
| 2 | 44 | 0–191 | 1350 |
| 3 | 41 | 39–190 | 1368 |
| 4 | 110 | 195–263 | 1276 |
| 5 | 109 | 194–263 | 1320 |

`vmMemoryLimit` is `0.6 × 2240 MiB = 1344 MiB`. Each worker grows until it
passes this value, and vitest then starts a new one. The two live workers
held 2689 MB of heap together at 190 s, just before the restart. Both workers
are threads of one Node.js process, so the RSS of that process (3429 MB) holds
both heaps.

The peaks with a different `vmMemoryLimit` follow the same rule:

| `vmMemoryLimit` | 2 × limit | Peak | Peak minus 2 × limit |
| --- | --- | --- | --- |
| 1344 MiB (current, A1) | 2688 MB | 3418 MB | 730 MB |
| 672 MiB (L30) | 1344 MB | 1973 MB | 629 MB |
| 336 MiB (L15) | 672 MB | 1566 MB | 894 MB |

The run with 1 worker (W1, `TARIBOY_VITEST_MAX_WORKERS=1 npm test`) did not
finish. After 1500 s, `timeout` stopped it with 13 of 304 files done. In
those files most tests failed after the 10 s `asyncUtilTimeout` of Testing
Library. The same happened in a second 1-worker run with the hook. The process
stayed between 0.8 and 0.97 GB for the whole run. With 2 workers the same
files pass. This report did not find the cause. It is a separate defect: the
suite does not pass with one worker.

### Memory per file

Heap growth of a worker from the end of one file to the end of the next file
(Ahook2, 300 pairs): median 16.5 MB, 90th percentile 48.3 MB, mean 15.7 MB.
The growth during one file (heap after the file minus heap before it) has a
median of 1.8 MB. So most of the growth is the module graph and the
environment that each new file loads into its new VM context.

The files with the largest growth during the file:

| File | Heap growth, MB |
| --- | --- |
| `src/pages/agents/AgentConfigurationTab.test.tsx` | 279.9 |
| `src/pages/tasks/TasksWorkspace.detail.test.tsx` | 172.7 |
| `src/pages/tasks/TasksWorkspace.test.tsx` | 166.6 |
| `src/App.test.tsx` | 138.3 |
| `src/pages/tasks/TasksWorkspace.realtime.test.tsx` | 92.8 |
| `src/pages/tasks/TasksWorkspace.questions.test.tsx` | 88.6 |
| `src/pages/tasks/TaskDetail.decisions.test.tsx` | 85.1 |
| `src/pages/terminals/TerminalsPage.sidebar.test.tsx` | 66.4 |
| `src/pages/tasks/WorkflowPanel.test.tsx` | 64.8 |
| `src/pages/tasks/TaskMarkdown.test.tsx` | 56.2 |

### Cost of jsdom, setup and the editor

Each micro run had 20 copies of one small test file in 1 worker, with the hook.
"Editor" files import `@tiptap/react`, `@tiptap/starter-kit`,
`@uiw/react-codemirror` and seven `@codemirror/lang-*` packages.

| Run | Environment | `setup.ts` | File | Heap after file 1, MB | Heap after file 20, MB | Max RSS, MB | Time, s |
| --- | --- | --- | --- | --- | --- | --- | --- |
| m1 | `node` | no | empty | 18 | 18 | 171 | 4.1 |
| m6 | `node` | no | editor | 33 | 31 | 199 | 4.1 |
| m2 | `jsdom` | no | empty | 52 | 57 | 266 | 5.1 |
| m3 | `jsdom` | yes | empty | 67 | 73 | 271 | 5.1 |
| m4 | `jsdom` | yes | editor | 71 | 79 | 290 | 7.2 |
| m7 | `happy-dom` | no | empty | 32 | 37 | 195 | 4.1 |
| m5 | `happy-dom` | yes | empty | 38 | 38 | 211 | 5.1 |

A `jsdom` environment costs about 34 MB of heap (m2 minus m1), and
`happy-dom` about 14 MB (m7 minus m1). `setup.ts` adds about 15 MB, and the
editor packages add 4 to 15 MB. Each of these files adds less than 0.5 MB of
heap per file. So jsdom, setup and the editor alone do not explain 15.7 MB per
file. The growth comes from the application modules of each real test file,
with their mocks, that every VM context loads again.

### Why the base report saw 4.1 GB

The base report ran at commit `75a9392c`. At that time `vite.config.ts` did
not set `vmMemoryLimit`. The vitest default is total memory divided by the
number of workers, about 2.9 GB here. That is above the heap limit of a worker
(2240 MiB). So the workers grew until the heap limit, and some runs ended out
of memory. PR #321 set the limit to 60 % of the heap limit. This lowered the
peak to 3.4 to 3.6 GB. It does not remove the growth.

## Variant table

All variants ran on the whole `ui-test` suite with 2 workers, except where
stated. "Peak" is `memory.peak` of the run cgroup. "Max RSS" is the largest
single process. "MemAvailable" is the host value before the run.

| Variant | Command | MemAvailable before, MB | Peak, MB | Max RSS, MB | Time, s | Files done | Files passed | Tests passed | Comparable with A |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A1, `vmThreads` + `jsdom` (current) | `npm test` | 3982 | 3418 | 3329 | 269.5 | 304/304 | 303 | 2771/2771 | base |
| A2, same | `npm test` | 3943 | 3564 | 3422 | 261.3 | 304/304 | 303 | 2771/2771 | base |
| Ahook2, same with the hook | `npm test -- --config vite.measure.config.ts` | 3912 | 3565 | 3429 | 277.7 | 304/304 | 304 | 2774/2774 | yes |
| L30, `vmMemoryLimit` 672 MiB | `npm test -- --vmMemoryLimit=704643072` | 4137 | 1973 | 1845 | 242.4 | 303/303 | 303 | 2773/2773 | yes |
| L15, `vmMemoryLimit` 336 MiB | `npm test -- --vmMemoryLimit=352321536` | 4140 | 1566 | 1544 | 255.4 | 303/303 | 303 | 2773/2773 | yes |
| B, `threads` + `jsdom` | `npm test -- --pool=threads` | 3822 | 1310 | 1267 | 559.8 | 304/304 | 303 | 2771/2771 | yes |
| C, `forks` + `jsdom` | `npm test -- --pool=forks` | 3605 | 1148 | 548 | 587.1 | 304/304 | 303 | 2771/2771 | yes |
| D, `vmThreads` + `happy-dom` | `npm test -- --environment=happy-dom` | 4088 | 3431 | 3370 | 224.9 | 304/304 | 294 | 2757/2774 | yes, 17 tests fail |
| E, `threads` + `happy-dom` | `npm test -- --pool=threads --environment=happy-dom` | 4089 | 1107 | 1026 | 359.4 | 304/304 | 294 | 2757/2774 | yes, 17 tests fail |
| F, `forks` + `happy-dom` | `npm test -- --pool=forks --environment=happy-dom` | 3950 | 1196 | 534 | 394.7 | 304/304 | 294 | 2757/2774 | yes, 17 tests fail |
| G1, `bun test` + `happy-dom` | `bun test --preload ./bun-preload.measure.ts --timeout 20000` | 3481 | 3533 | 3464 | 864.4 | 304/304 | 135 | 1340/2774 | no |
| W1, `vmThreads` + `jsdom`, 1 worker | `TARIBOY_VITEST_MAX_WORKERS=1 npm test` | 3577 | 851 | n/m | 1501.1 (stopped) | 13/304 | n/m | n/m | no |

Notes:

- In A1, A2, B and C, `scripts/api-types.test.mjs` failed because the
  research worktree had no `ui/tools/api-types/node_modules` yet. The file
  finished. Ahook2 ran after the install and passed all files.
- A third base run (A3) started after L15. The Claude Code harness stopped it
  because the host was low on memory. It has no result.
- C has the lowest single process (548 MB), because each worker is a process.
- Vitest reports the sum of the `environment` phase over all files: 11 to 15 s
  for A, 409 s for B and 440 s for C. Outside the VM pool, every file builds a
  new jsdom from the start. This is where the extra time of B and C goes.
- D, E and F fail the same 10 files and 17 tests. The causes are:
  `window.confirm` is not defined in the host and server removal tests of
  `DaemonsPage` and `TerminalsPage.servers` (6 tests), `tests/desktop/w3c.test.ts`
  (4 tests, `happy-dom` blocks the cross-origin request), `navigator.clipboard`
  has only a getter (3 tests), storage that a test blocks (2 tests), the CSS
  minification test of `tasksStyles` (1 test), and a missing error text in
  `ServerDialog` (1 test). The failures are fast, so the time is comparable.
- G1 is not comparable. Many tests wait for the 10 s or 20 s timeouts before
  they fail, and 57 files stop at load and run no test. W1 is not comparable:
  it did not finish.

## Bun compatibility

The run used Bun 1.4.4 (`1.4.4+38afee6c8`). Bun has no `jsdom` environment
like vitest. As the Bun documentation advises, a preload file registered the
`happy-dom` globals with `GlobalRegistrator.register()` and then imported
`src/test/setup.ts`. Bun resolves `import { vi } from "vitest"` to its own
`vi` object.

### What works without changes

A probe test listed the functions of the Bun `vi` object: `fn`, `mock`,
`spyOn`, `restoreAllMocks`, `resetAllMocks`, `clearAllMocks`,
`useFakeTimers`, `useRealTimers`, `advanceTimersToNextTimer`,
`advanceTimersByTime`, `runOnlyPendingTimers`, `runAllTimers`,
`getTimerCount`, `clearAllTimers`, `isFakeTimers`.

- 135 of 304 files pass. They use only `describe`, `it`, `expect`, `vi.fn`,
  `vi.spyOn`, Testing Library and `jest-dom` matchers.
- `describe`, `it` and `expect` work in test files, because every test file
  imports them from `vitest`. No test file relies on vitest globals.
- The `@/` alias resolves: 59 of the 135 passing files import from `@/`.
- `setup.ts` runs from the preload file.

### What must be rewritten

Counts come from `grep -l` over the 303 test files of `main`.

| Cause | Files | Error in G1 |
| --- | --- | --- |
| `vi.stubGlobal`, `vi.unstubAllGlobals` | 69 | `vi.stubGlobal is not a function` |
| `vi.hoisted` | 54 | `vi.hoisted is not a function`, and the file stops at load |
| `vi.mock` factories with `importOriginal` | 25 | no original module is passed |
| `vi.mocked` | 23 | `vi.mocked is not a function` |
| `vi.advanceTimersByTimeAsync` | 13 | not a function |
| `vi.stubEnv`, `vi.unstubAllEnvs` | 10 | not a function |
| `vi.waitFor` | 3 | not a function |
| `vi.importActual` | 2 | not a function |
| `vi.setSystemTime` | 1 | not a function |
| `?raw` and `?url` imports | 3 | Vite import suffixes |
| `import.meta.glob` | 1 | Vite only |
| `spyOn` of an accessor property | 4 errors | `does not support accessor properties yet` |

Together, 130 files contain one of these APIs, with 513 call sites. All 73
files with `vi.mock` also need review, because Bun does not hoist
`vi.mock` above the imports the way vitest does. After the API rewrite, the
tests that pass only in `jsdom` need the same fixes as variants D to F.

Estimate of the work: 130 files at 15 to 30 minutes each is 4 to 8
person-days (estimate). Fixing the remaining failures and the module mocks
adds 2 to 4 person-days (estimate). Total: 6 to 12 person-days (estimate).

### Memory of `bun test` over time

The Bun claim (bun.com/blog/bun-in-rust) is that memory stops growing in the
Rust version. Two runs checked it:

- G1, every file once (864 s): the sum of RSS was 1735 MB at 161 s and
  1848 MB at 692 s. That is 113 MB over 530 s.
- G2, every file twice in one process with `--rerun-each 2` (1632 s): the
  maximum of the sum of RSS in each tenth of the run was 708, 644, 730, 635,
  639, 738, 725, 852, 3150, 861 MB.

The slope over the middle 80 % is 43 MB per 100 s (G1) and 41 MB per 100 s
(G2). G2 ran 608 file runs in 1632 s, so this is about 110 MB per 100 files
(estimate, it assumes an even rate of files). The `vmThreads` pool grows by
about 1570 MB of heap per 100 files (15.7 MB per file).

Result: confirmed for this suite. There is no steady growth. But short spikes
of 2.5 to 3.4 GB happen at a few files (G1 at 117 to 119 s and 730 to 745 s,
G2 at 1387 to 1391 s). So the peak of `bun test` (3533 MB) is not lower than
the peak of A. The 169 files that fail do less work, so a fully working suite
can use more memory.

### Risks

- Incompatibility: 130 files use `vi.*` functions that Bun does not have. Bun
  has no `jsdom`. Module mocks follow different hoisting rules.
- Maturity: the Bun `vi` object has 15 functions. The suite uses 20 different
  `vi.*` functions, and 10 of them are missing in Bun. Bun 1.4 is a new Rust implementation, so its behavior can
  still change between releases.
- Support: the vitest API is the API that the tests target. A Bun gap needs a
  rewrite or a wait for Bun, not a configuration change.

### What a move away from vitest loses

- The pool choice and the isolation of each file. `bun test` ran all files in
  one process (the sampler saw one `bun test` process). Module mocks and
  globals of one file can reach the next file.
- The worker restart at `vmMemoryLimit`. Bun has no restart. One process must
  hold the whole suite.
- The `maxWorkers` limit through `TARIBOY_VITEST_MAX_WORKERS`.
- The check in `scripts/vitest-run.mjs` that fails a run with fewer finished
  files than found. It reads the ledger of the vitest reporter
  `scripts/vitest-file-ledger.mjs`. Bun has no such reporter API. The check
  would need a new source, for example the JUnit file of Bun.
- The Vite transform: the React and Tailwind plugins, `import.meta.glob`, and
  the `?raw` and `?url` imports.

## Other JavaScript steps

All runs used the same lock and `measure.py`. `ui-typecheck` runs started
without `ui/node_modules/.tmp`, so `tsc -b` did a full build.

| Step | Command | Peak, MB | Max RSS, MB | Time, s | Native today | Native analog | Suitable |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ui-typecheck` | `npx tsc -b` | 1378 | 1297 | 6.1 | yes, `tsc` 7 | already native | yes |
| `ui-typecheck` | `npx tsc -b --checkers 1` | 888 | 861 | 8.1 | yes | same tool, one checker | yes |
| `ui-typecheck` | `npx tsc -b --singleThreaded` | 822 | 795 | 13.3 | yes | same tool, one thread | yes, but 2.2 times slower |
| `ui-lint` | `npm run lint` | 122 | 129 | 2.0 | yes, oxlint | already native | yes |
| `ui-dead-code` | `npm run knip` | 402 | 395 | 9.2 | no | none known | no |
| `docs` | `npm run doctor` | 380 | 354 | 8.2 | no | none | no |
| `docs` | `npm run build` | 1337 | 1212 | 23.4 | partly | Hugo, Zola, mdBook | no |

- `ui-typecheck`: `tsc` 7 is the native Go compiler. Its own `--checkers`
  option controls its parallel checkers. One checker saves 490 MB for 2 s.
- `ui-lint`: oxlint is already the native linter.
- `ui-dead-code`: knip finds unused files, exports, types and dependencies.
  This report knows no native tool with the same checks. oxlint finds unused
  variables and imports only inside one file. At 402 MB, knip is not a peak
  of `make check`.
- `docs`: Blume builds the site with Astro 7, which already bundles with
  Vite 8 and the Rust bundler rolldown. The cost that remains is the
  rendering of the MDX pages in Node.js. Hugo, Zola and mdBook are native site
  generators, but they cannot render MDX with React components. A move means
  a rewrite of the site and of Blume.

## Recommendation and KPI effect

Recommendation: keep `vmThreads` and `jsdom`, and set an absolute
`vmMemoryLimit` of 336 MiB (`352321536` bytes) in `ui/vite.config.ts`.
Optionally, run `ui-typecheck` with `--checkers 1`. A separate task makes
these changes. This report changes no configuration.

Do not move to `threads`, `forks` or `bun test`. Do not move to `happy-dom`
before its 10 failing files are fixed in a separate task.

### Measured effect on the shared host

These values are measured with 2 workers (`make check` agent settings) as
shown in the method.

| KPI | Now | With the recommendation | Measurement |
| --- | --- | --- | --- |
| `ui-test` peak | 3418 to 3565 MB | 1566 MB | `memory.peak`, runs A1, A2, Ahook2 against L15 |
| `ui-test` time | 261 to 278 s | 255 s | wall time, same runs. The difference is inside the host noise. |
| `ui-typecheck` peak | 1378 MB | 888 MB | `memory.peak`, `tsc -b` against `tsc -b --checkers 1` |
| `ui-typecheck` time | 6.1 s | 8.1 s | wall time, same runs |
| `make check` peak | 3418 to 3565 MB (`ui-test`) | about 1.6 GB | the largest step peak: `ui-test` 1566 MB from L15, `docs` build 1337 MB from this report, cold `desktop-rust-check` 1576 MB from the base report |
| `make check` time | no change | no change | the steps above change by less than the host noise. `make check` was not run as a whole. |

### Estimate for CI (estimate)

CI runs `ui-test` with 4 workers on a larger runner. The current limit is 60 %
of the heap limit, and the heap limit grows with the memory of the runner. So
4 workers can hold several GB each (estimate). An absolute limit of 336 MiB
gives about `4 × 336 MiB + 0.6 to 0.9 GB`, which is 2.0 to 2.3 GB for
`ui-test` in CI (estimate). The time in CI is expected to stay the same,
because the host runs showed no time change (estimate). No CI run measured
this.

## Limits

- The host is shared. Other agents ran between the runs, and `MemAvailable`
  before a run was 3481 to 4140 MB. Each variant ran once, except A.
- The cgroup peak includes page cache. Max RSS is one process only. The CSV
  sum of RSS misses peaks shorter than 1 s.
- The heap values of the hook include garbage that the collector did not yet
  free.
- The cause of the 1-worker failure (W1) is not known.
- Bun was run only with `happy-dom` and without changes to the tests. Its time
  includes the timeouts of failing tests.
- CI was not measured. Every CI value is an estimate.

## Appendix: measurement files

These files lived outside the repository or were removed before the commit.

`measure.py`:

```python
#!/usr/bin/env python3
"""Measure one command: wall time, /usr/bin/time Max RSS, cgroup memory.peak,
and RSS of the process tree sampled every second.

Usage (always under the host lock, one run at a time):
  flock -o /tmp/tariboy-check.lock python3 -I measure.py OUTDIR NAME CWD -- CMD...
"""
import json
import os
import subprocess
import sys
import time

GATE = "<worktree>/scripts/ui-test-memory-gate.sh"


def memavail_mb():
    with open("/proc/meminfo") as f:
        for line in f:
            if line.startswith("MemAvailable:"):
                return int(line.split()[1]) // 1024
    return None


def children_map():
    kids = {}
    for d in os.listdir("/proc"):
        if not d.isdigit():
            continue
        try:
            with open(f"/proc/{d}/stat") as f:
                s = f.read()
            ppid = int(s[s.rfind(")") + 2:].split()[1])
            kids.setdefault(ppid, []).append(int(d))
        except OSError:
            pass
    return kids


def rss_kb(pid):
    try:
        with open(f"/proc/{pid}/status") as f:
            for line in f:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except OSError:
        pass
    return 0


def tree(root):
    kids = children_map()
    out, stack = [], [root]
    while stack:
        p = stack.pop()
        out.append(p)
        stack.extend(kids.get(p, []))
    return out


def main():
    outdir, name, cwd = sys.argv[1], sys.argv[2], sys.argv[3]
    cmd = sys.argv[5:]
    base = os.path.join(outdir, name)
    subprocess.run([GATE], check=False)
    before = memavail_mb()
    inner = ('/usr/bin/time -v -o "$T" "$@"; rc=$?; '
             'cat "/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup)/memory.peak" > "$P"; exit $rc')
    env = dict(os.environ, T=base + ".time", P=base + ".peak")
    csv = open(base + ".csv", "w")
    csv.write("t,sum_rss_mb,memavail_mb\n")
    t0 = time.time()
    p = subprocess.Popen(["systemd-run", "--user", "--scope", "-q", "--", "sh", "-c", inner, "sh", *cmd],
                         cwd=cwd, stdout=open(base + ".log", "w"), stderr=subprocess.STDOUT, env=env)
    while p.poll() is None:
        total = sum(rss_kb(x) for x in tree(p.pid)) // 1024
        csv.write(f"{time.time() - t0:.1f},{total},{memavail_mb()}\n")
        csv.flush()
        time.sleep(1)
    print(json.dumps(dict(name=name, exit=p.returncode, wall_s=round(time.time() - t0, 1),
                          memavail_before_mb=before)))


if __name__ == "__main__":
    main()
```

`ui/measure-hook.ts`:

```ts
import fs from "node:fs";
import { threadId } from "node:worker_threads";

const out = process.env.MEASURE_HOOK_OUT;
let before: NodeJS.MemoryUsage | undefined;
let file: string | undefined;

beforeAll(() => {
  before = process.memoryUsage();
  file = expect.getState().testPath;
});

afterAll(() => {
  if (!out) return;
  const m = process.memoryUsage();
  fs.appendFileSync(out, JSON.stringify({ t: Date.now(), tid: threadId, file,
    rss0: before?.rss, heap0: before?.heapUsed, rss: m.rss, heap: m.heapUsed }) + "\n");
});
```

`ui/vite.measure.config.ts`:

```ts
import { mergeConfig } from "vite";
import base from "./vite.config";

const cfg = mergeConfig(base, { test: { setupFiles: ["./measure-hook.ts"] } });
if (process.env.MEASURE_NO_SETUP === "1") cfg.test.setupFiles = ["./measure-hook.ts"];
if (process.env.MEASURE_INCLUDE) cfg.test.include = [process.env.MEASURE_INCLUDE];
export default cfg;
```

`ui/bun-preload.measure.ts`:

```ts
import { GlobalRegistrator } from "@happy-dom/global-registrator";

GlobalRegistrator.register({ url: "http://localhost:3000/" });
await import("./src/test/setup.ts");
```

Micro runs, one per row of the micro table:

```bash
TARIBOY_VITEST_MAX_WORKERS=1 MEASURE_NO_SETUP=1 \
  MEASURE_INCLUDE='src/__measure__/empty-*.mtest.ts' MEASURE_HOOK_OUT=m2.jsonl \
  npx vitest run --config vite.measure.config.ts --environment jsdom
```
