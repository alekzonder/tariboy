# ui-test with one worker: cause report

This report explains why `ui-test` fails with one vitest worker
(`TARIBOY_VITEST_MAX_WORKERS=1`). The earlier report
`docs/superpowers/specs/2026-11-03-ui-test-memory-and-runtime-report.md`
recorded the failure (run W1) and left its cause open.

## Summary

With `pool: "vmThreads"` and `maxWorkers: 1`, vitest 4.1.7 runs all test
files in one VM context. All files share one `jsdom` window and one module
cache. State from earlier files goes into later files, and the tests of the
later files fail. Most of them wait the full `asyncUtilTimeout` of 10 s, so
the run is very slow and the 30-minute `timeout` stops it.

Three kinds of state go from file to file:

1. DOM. `@testing-library/react` registers its automatic
   `afterEach(cleanup)` only in the first file. Rendered trees of later files
   stay in `document.body`, and queries find two elements.
2. Module mocks. A `vi.mock` in a later file does not reach modules that an
   earlier file already loaded. For example, `TasksWorkspace.test.tsx` mocks
   `createApi` of `@/lib/api`, but the cached `ApiProvider` keeps the real
   `createApi`, so the test data never arrives.
3. Fake timers. A test that fails before its `vi.useRealTimers()` leaves fake
   timers on for the next files.

With 2 or 4 workers, every file gets a new context, and this cause does not
apply. The shuffled runs found two separate order dependences between tests
inside one file. They do not show in the default order.

The recommended fix is option A: run at least 2 workers. It is a one-line
change in `ui/vite.config.ts`. It does not change product behavior. The
customer decides about it separately. This task does not make the fix.

| Run | Files done | Failed tests | Time |
| --- | ---: | ---: | ---: |
| Full suite, 1 worker | 10 of 303 | 229 | 1800 s, stopped |
| Full suite, 1 worker, extra `afterEach(cleanup)` | 11 of 303 | 97 | 1800 s, stopped |
| 3 files, 1 worker | 3 of 3 | 3 of 22 | 28.3 s |
| 3 files, 1 worker, extra `afterEach(cleanup)` | 3 of 3 | 0 of 22 | 8.4 s |
| 3 files, 2 workers | 3 of 3 | 0 of 22 | 6.6 s |
| `StoresPage` then `TasksWorkspace`, 1 worker, extra cleanup | 2 of 2 | 25 of 45 | 257.3 s |
| `TasksWorkspace` alone, 1 worker | 1 of 1 | 0 of 25 | 20.7 s |

Each number in this report is a measurement. The sections below give the
command of each run. The only estimate is marked as one in option A.

## Method

| Item | Value |
| --- | --- |
| Date | 2026-11-04 |
| Commit | `main` at `51a7f05e17094a0a00403ea9a9a8b1ad27b5fbe8` |
| vitest | 4.1.7 |
| Node.js | v24.22.2 |
| `nproc`, RAM | 6, 5924 MB, shared by all agents |
| Configuration | `ui/vite.config.ts` of that commit: `pool: "vmThreads"`, `environment: "jsdom"`, `vmMemoryLimit` 336 MiB |

Each run took the host lock alone, with a `timeout`:

```bash
cd ui
# full suite
time timeout 1800 flock -o /tmp/tariboy-check.lock \
  env TARIBOY_VITEST_MAX_WORKERS=1 npm test
# a subset
timeout 600 flock -o /tmp/tariboy-check.lock \
  env TARIBOY_VITEST_MAX_WORKERS=1 npx vitest run --reporter=default FILES...
```

The time in the table is the wall time of `/usr/bin/time` around the command.

For the diagnosis, a temporary setup file `ui/src/test/diag-setup.ts` was
added to `setupFiles` (full text in the appendix). It does not change a test.
Before each test it writes one line to the file in `DIAG_LOG`:

- `win`: a random ID kept on `globalThis`. The same ID means the same window.
- `loads`: how many times the setup files ran in this window.
- `body`: `document.body.children.length`.
- `fake`: `vi.isFakeTimers()`.

If `DIAG_CLEANUP=1`, the file also registers `afterEach(cleanup)`. All
temporary changes were removed after the measurements. The branch of this
report contains only this file.

## Reproduction

The full suite with one worker, without the diagnostic file:

```bash
cd ui
/usr/bin/time -f "wall=%es" timeout 1800 flock -o /tmp/tariboy-check.lock \
  env TARIBOY_VITEST_MAX_WORKERS=1 npm test
```

Result:

```text
Command exited with non-zero status 124
wall=1800.00s
ui-test: vitest finished 10 of 303 test files; 293 did not run
```

Eight of the 10 files failed. The `agent` reporter showed 229 failed tests.
119 of them took between 10 and 20 s, the length of `asyncUtilTimeout` and
`testTimeout`. The first failed files:

```text
 ❯ src/pages/StoresPage.test.tsx (20 tests | 19 failed) 120592ms
 ❯ src/components/TuiScreen.test.tsx (90 tests | 56 failed) 42966ms
 ❯ src/pages/tasks/TasksWorkspace.test.tsx (25 tests | 25 failed) 249354ms
 ❯ src/pages/tasks/TasksWorkspace.detail.test.tsx (23 tests | 23 failed) 254874ms
 ❯ src/pages/tasks/WorkflowPanel.test.tsx (34 tests | 31 failed) 346204ms
```

This agrees with run W1 of the earlier report: 13 of 304 files in 1500 s.

### Minimal example

Three files reproduce the failure in less than 30 s:

```bash
cd ui
TARIBOY_VITEST_MAX_WORKERS=1 npx vitest run --reporter=default \
  src/components/SendFilesButton.test.tsx \
  src/hooks/useApiQuery.test.tsx \
  src/pages/AuditLogPage.test.tsx
```

```text
 ❯ src/components/SendFilesButton.test.tsx (4 tests | 3 failed) 20206ms
     × renders a Send files button 28ms
     × uploads every picked file and reports their absolute paths 10047ms
     × uploads through the client of the surrounding host 10062ms
TestingLibraryElementError: Found multiple elements with the role "button" and name `/send files/i`
 Test Files  1 failed | 2 passed (3)
      Tests  3 failed | 19 passed (22)
```

Each of the three files passes alone. The three files pass together with
`TARIBOY_VITEST_MAX_WORKERS=2` (22 of 22, 6.6 s).

## Cause

### Measurement: one window for all files

The diagnostic log of the three-file run with one worker. vitest ran the
files in the order `AuditLogPage`, `useApiQuery`, `SendFilesButton`:

```text
count file                     win     loads  body
   10 AuditLogPage.test.tsx    26trk9  1      0
    8 useApiQuery.test.tsx     26trk9  2      0, 1, 2, ... 7
    4 SendFilesButton.test.tsx 26trk9  3      8, 9, 10, 11
```

All three files have the same `win`. The setup files ran again for each file
(`loads` 1, 2, 3), but in the same window. `body` stays 0 in the first file
and grows by one for each test after it. The trees of earlier tests stay in
the DOM, so `SendFilesButton` finds two "Send files" buttons.

With two workers, each file has its own `win`, `loads` is always 1 and
`body` is always 0.

### Before and after: the DOM cleanup

The same three files with one worker and `DIAG_CLEANUP=1`:

```text
 Test Files  3 passed (3)
      Tests  22 passed (22)
```

`body` is 0 before every test. This proves the DOM leak for these three
files. It is not the only state that goes between files. The full suite with
one worker and `DIAG_CLEANUP=1` still failed:

```bash
cd ui
/usr/bin/time -f "wall=%es" timeout 1800 flock -o /tmp/tariboy-check.lock \
  env TARIBOY_VITEST_MAX_WORKERS=1 DIAG_CLEANUP=1 DIAG_LOG=... npm test
```

```text
Command exited with non-zero status 124
wall=1800.00s
ui-test: vitest finished 11 of 303 test files; 292 did not run
 ❯ src/pages/tasks/TasksWorkspace.test.tsx (25 tests | 25 failed) 241774ms
 ❯ src/pages/tasks/TasksWorkspace.detail.test.tsx (23 tests | 23 failed) 230919ms
 ❯ src/pages/tasks/TasksWorkspace.realtime.test.tsx (21 tests | 21 failed) 191246ms
 ❯ src/components/CustomerQuestionNotifications.test.tsx (18 tests | 18 failed) 250231ms
 ❯ src/pages/images/BuiltImages.test.tsx (10 tests | 10 failed) 200221ms
```

`body` was 0 before all 439 tests that ran, and 97 tests failed.
`StoresPage` and `TuiScreen`, which failed without the cleanup, passed.

### Before and after: module mocks

`TasksWorkspace.test.tsx` passes alone with one worker (25 of 25, 20.7 s).
After `StoresPage.test.tsx` in the same worker, with `DIAG_CLEANUP=1`, all 25
tests fail (257.3 s). The first error:

```text
TestingLibraryElementError: Unable to find an accessible element with the role "tab" and name "Pools"
```

`TasksWorkspace.test.tsx` mocks `@/lib/api` and replaces `createApi` with a
wrapper that sends calls to its `api` spies. A temporary counter in the mock
factory and in the wrapper showed this, with `--sequence.shuffle.files` to
set the file order and `--bail=1`:

| Order | Mock factory ran | Wrapped `createApi` calls | Result |
| --- | ---: | ---: | --- |
| `StoresPage`, then `TasksWorkspace` (seeds 1 and 2) | 1 | 0 | `TasksWorkspace` fails |
| `TasksWorkspace`, then `StoresPage` (seed 3) | 1 | one or more for each test | 45 of 45 pass |

`StoresPage.test.tsx` imports `@/components/ApiProvider`, which imports
`createApi` from `@/lib/api`. vitest keeps that evaluated module. When
`TasksWorkspace.test.tsx` runs later, its `vi.mock` applies to new imports
only. The cached `ApiProvider` keeps the real `createApi`, the page calls the
real client, and the spies get no call. The other order shows the reverse
leak: the mock of `TasksWorkspace` stayed in effect for `StoresPage`. In the
run with seed 3, the wrapper was called 58 times during the 20 tests of
`StoresPage`. 21 test files in `ui/src`
call `vi.mock("@/lib/api", ...)`.

### Fake timers

In the full run with the cleanup, `vi.isFakeTimers()` was `true` before the
last 8 tests of `CustomerQuestionNotifications.test.tsx` and before all tests
of `BuiltImages.test.tsx` and `App.test.tsx`, 52 tests in all. Some tests of
that file, for example "treats recovery after an initial failure as a new
silent baseline", call `vi.useFakeTimers()` and call `vi.useRealTimers()` at
the end of the test body, not in a `finally` block or an `afterEach`. If such a test fails, fake timers
stay on. Between files vitest calls only `vi.resetConfig()` and
`vi.restoreAllMocks()`, which do not restore timers. In a new context per
file, as with 2 or more workers, the next file starts with real timers.

### Mechanism in vitest 4.1.7

The names are functions in `ui/node_modules/vitest/dist`. The line numbers
are for 4.1.7.

1. `resolveConfig` (`chunks/coverage.DM_a_rWm.js`, line 379) sets
   `isolate = false` for the pools `vmForks`, `vmThreads` and `typescript`:

   ```js
   if (resolved.pool === "vmForks" || resolved.pool === "vmThreads" || resolved.pool === "typescript") resolved.isolate = false;
   ```

2. `groupSpecs` (`chunks/cli-api.C6CiCDM3.js`, line 3812) puts all files of
   a project into one worker task when `isolate` is false and `maxWorkers`
   is 1:

   ```js
   // Non-isolated single worker can receive all files at once
   if (isolate === false && maxWorkers === 1) {
     const previous = groups[order].specs[0]?.[0];
     if (previous && previous.project.name === spec.project.name && isEqualEnvironments(spec, previous)) return groups[order].specs[0].push(spec);
   }
   ```

   With `maxWorkers` above 1, each file is its own task.

3. `runVmTests` (`chunks/vm.DVLYObm9.js`, line 708) sets up the environment
   once for each task: one `jsdom` window and one VM context. It then calls
   `run` with all files of the task.

4. `run` (`chunks/base.RR7zL1h0.js`, line 66) runs the files in a loop. It
   resets the module cache and the mocker only `if (config.isolate)`. After
   each file it calls `vi.resetConfig()` and `vi.restoreAllMocks()`, and
   nothing more.

5. `@testing-library/react` (`dist/index.js`, line 26) registers
   `afterEach(cleanup)` when its module is evaluated. With a shared module
   cache, that happens once, in the first file.

The vm pools are always non-isolated, so the condition in step 2 is true
exactly when `maxWorkers` is 1.

### The hypotheses of the task

1. "vitest changes its mode with `maxWorkers: 1`": confirmed in another form.
   The order of files and the parallelism are as before. The grouping
   changes: all files go into one task and one context. Fake timers, mocks
   and other global state then go between files.
2. "The first file leaves `vi.useFakeTimers()` or another global patch": not
   the main cause. The first file does not leave fake timers. The DOM leak
   starts at the second file, and the mock leak starts at the first file that
   mocks an already cached module. A fake-timer leak exists, but only after
   a test failed before `vi.useRealTimers()`.
3. "`vmMemoryLimit` or a worker restart": not confirmed. The pool checks
   `vmMemoryLimit` when a task ends (`testfileFinished`), and with one worker
   the run has only one task. The worker does not restart during the run.
   The failure happens without a restart.

## Runs with 2 and 4 workers

The cause above does not apply with 2 or 4 workers. Each file is its own
task, so it gets its own window and module cache.

The check used 26 files: every fifth file of the 114 test files that use
`findBy` or `waitFor`, sorted by path, and the three files of the minimal
example. `--sequence.shuffle` shuffles the order of files and the order of
tests inside each file.

```bash
cd ui
timeout 900 flock -o /tmp/tariboy-check.lock \
  env TARIBOY_VITEST_MAX_WORKERS=W DIAG_LOG=... \
  npx vitest run --reporter=default --sequence.shuffle --sequence.seed=SEED FILES...
```

| Workers | Seed | Files | Failed tests | Windows | Time |
| ---: | ---: | --- | ---: | ---: | ---: |
| 2 | 11 | 25 of 26 pass | 3 of 397 | 26 | 53.8 s |
| 2 | 42 | 26 of 26 pass | 0 of 397 | 26 | 54.3 s |
| 4 | 11 | 24 of 26 pass | 4 of 397 | 26 | 31.3 s |
| 4 | 42 | 25 of 26 pass | 2 of 397 | 26 | 33.1 s |

In each run, the 26 files had 26 different `win` IDs, `loads` was always 1,
and `vi.isFakeTimers()` was `false` before every test. No state went between
files.

The failures come from the order of tests inside one file:

- `src/pages/tasks/TasksWorkspace.questions.test.tsx`: 2 or 3 tests fail with
  `Found a label with the text of: Comment, however no form control was found
  associated to that label`. With the two files of these failures and 2
  workers, the result was:

  | Option | Result |
  | --- | --- |
  | no shuffle | 20 of 20 pass |
  | `--sequence.shuffle.files --sequence.seed=11` | 20 of 20 pass |
  | `--sequence.shuffle.tests --sequence.seed=11` | 3 of 20 fail |

  The tests of this file depend on the order of the tests in the file.

- `src/pages/terminals/TerminalsPage.sections.test.tsx`: one test failed
  once (4 workers, seed 11) with `Found multiple elements with the text: a1`.
  In every run, `body` grew from 0 to 7 across the 8 tests of this file.
  Nodes outside the Testing Library containers stay between tests. The cause
  of this growth was not examined further.

In the default order, which `make check` uses, these two files passed (the
"no shuffle" row above). These two order dependences are separate findings. They do not explain the failure with
one worker.

## Fix options

The fix is small and does not change product behavior. The customer decides
about it separately. This task changes no code.

### A. Run at least 2 workers (recommended)

In `ui/vite.config.ts`, raise `maxWorkers` to at least 2 and give the reason
in a comment:

```ts
maxWorkers: Math.max(2, Number(process.env.TARIBOY_VITEST_MAX_WORKERS) || 4),
```

- Scope: one line and a comment in `ui/vite.config.ts`. Optionally one
  sentence in `docs/docs/development.mdx` that 1 is raised to 2.
- Effect: the condition in `groupSpecs` is never true, so each file gets its
  own context. With 2 workers, the full suite finished 303 of 303 files in
  the earlier measurement report.
- Risk: a one-worker mode no longer exists. The memory peak of `ui-test` is
  that of 2 workers. The earlier report
  `docs/superpowers/specs/2026-11-04-ui-test-memory-limit-measurement.md`
  measured 1523 and 1479 MB on this host with 2 workers. The config comment
  gives about workers × 336 MiB + 0.6 to 0.9 GB (estimate), about 1.3 to
  1.6 GB for 2 workers.
- Risk: the value 1 is silently changed. The comment must say why.

### B. Add `afterEach(cleanup)` to `ui/src/test/setup.ts`

- Scope: two lines in `ui/src/test/setup.ts`.
- Effect: the DOM leak stops. The three-file example passes.
- Result of the full run: it does not fix the failure. With this cleanup, the
  full suite with one worker finished 11 of 303 files in 1800 s, with 97
  failed tests. Module mocks and fake timers still go between files.
- Risk: none for 2 or more workers. The cleanup then runs twice, which is
  harmless.

Option B alone is not a fix. A full fix on this path also needs changes to
the test files that use `vi.mock` or fake timers. That is a large change and
its end is not known, so it is not recommended.

### C. Report the behavior to vitest

- Scope: an issue in the vitest repository with the three-file example.
- Content: vm pools are always non-isolated, and `groupSpecs` then puts all
  files of a single worker into one VM context. This differs from runs with
  more workers, where each file gets a new context.
- Risk: none. It does not fix the suite by itself. It adds to option A.

### Separate findings

These do not affect the default `make check` run. Fix them only if shuffled
runs are wanted:

- `TasksWorkspace.questions.test.tsx` depends on the order of its tests.
- `TerminalsPage.sections.test.tsx` leaves nodes in `document.body` between
  its tests.
- `CustomerQuestionNotifications.test.tsx` restores real timers at the end of
  the test body. A `finally` block or an `afterEach(() => vi.useRealTimers())`
  restores them also when the test fails.

## Appendix: diagnostic setup file

This file was temporary and was not committed.

```ts
// TEMPORARY diagnostic setup file. Not for commit.
import { appendFileSync } from "node:fs";
import { afterEach, beforeEach, expect, vi } from "vitest";
import { cleanup } from "@testing-library/react";

const g = globalThis as unknown as { __diagLoads?: number; __diagWin?: string };
g.__diagLoads = (g.__diagLoads ?? 0) + 1;
g.__diagWin ??= Math.random().toString(36).slice(2, 8);

const log = process.env.DIAG_LOG;

beforeEach(() => {
  if (!log) return;
  const file = (expect.getState().testPath ?? "").split("/").pop();
  appendFileSync(
    log,
    `${file}\twin=${g.__diagWin}\tloads=${g.__diagLoads}\tbody=${document.body.children.length}\tfake=${vi.isFakeTimers()}\n`,
  );
});

if (process.env.DIAG_CLEANUP) afterEach(() => cleanup());
```

The counter in `TasksWorkspace.test.tsx` added two lines to the `vi.mock`
factory of `@/lib/api`: one `appendFileSync` when the factory runs, and one
at the start of the wrapped `createApi`.
