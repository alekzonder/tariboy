# make check performance report

This report gives the wall time and the peak memory of each step of
`make backend-check` and `make frontend-check`, on the shared agent host and
in GitHub Actions. It names the cause of the most expensive steps, lists the
work that `make check` does again on each run, and gives proposals. It does
not change code. The customer selects the scope of improvements in a separate
task.

## Summary

- Locally, with agent settings (`CHECK_JOBS=2`), a warm run takes 692 s.
  `ui-test` is 56 % of that time (388 s) and `test` is 26 % (180 s).
- `ui-test` is also the memory peak of the whole check: one Node.js process
  grows to 4.1 GB RSS. The host has 5.9 GB of RAM for all agents. This
  agrees with the out-of-memory failures of `ui-test` on 2026-11-02.
- No other step goes above 1.6 GB. The next largest are `desktop-rust-check`
  (cold), `docs` and `ui-typecheck`, each between 1.2 GB and 1.6 GB.
- In CI the job takes a median of 452 s. `Run checks` is 392 s of it:
  `backend-check` 267 s and `frontend-check` 129 s, one after the other.
- A cold local run spends most of its extra time on `desktop-rust-check`
  (306 s), `test` (313 s), `tariboy-tasks-e2e` (126 s), `vet` (110 s) and
  `staticcheck` (72 s). CI repeats the `desktop-rust-check` compile on every
  run because the workflow has no cargo cache.

## Method

### Host and runs

| Item | Value |
| --- | --- |
| Date | 2026-11-03 |
| Commit | `75a9392c07a46db624fef145141334a7a0389979` |
| `nproc` | 6 |
| RAM, swap | 5924 MB, 8191 MB |
| Toolchains | Go 1.26.5, Node.js 22, TypeScript 7.0.2 (native `tsc`), vitest 4.1.7 |
| Agent settings | `TARIBOY_AGENT` set, `CHECK_JOBS=2`, so `GOFLAGS=-p=2`, `TARIBOY_VITEST_MAX_WORKERS=2`, `cargo check --jobs 2` |

| Run | Start and end, UTC | Load average (1, 5, 15 min) before | After | Available memory before, after |
| --- | --- | --- | --- | --- |
| `warm-j2`, all steps | 07:23 to 07:35 | 2.13, 3.81, 6.45 | 5.21, 6.05, 6.48 | 3472 MB, 3585 MB |
| `warm-j2-test`, `test` again | 10:48 to 10:51 | 3.22, 4.41, 2.42 | 5.36, 5.16, 3.09 | 3627 MB, 3081 MB |
| `cold-j2`, all steps up to `ui-dead-code` | 07:47 to 08:02 | 7.80, 6.25, 6.09 | not recorded | 4191 MB |
| `cold-j2-uitest`, `ui-test` | 08:07 to 09:01 | 3.92, 6.16, 6.40 | 8.67, 16.63, 19.42 | 3331 MB, 3810 MB |
| `cold-j2-test`, `test` with an empty `GOCACHE` | 10:52 to 10:58 | 3.96, 4.87, 3.16 | 5.93, 6.06, 4.21 | 2829 MB, 3259 MB |

The host is shared with other agents. The check slot keeps other
`make check` runs out, but not other agent work. Before each run a gate waited
until the host had at least 2500 MB available memory and a 1-minute load
average of 4 or less. Differences under about 15 % are not significant.

### How each step was measured

A script ran each preparation and check step with the same command as the
`Makefile`, one by one. Each run took the check slot with its own
`flock -o /tmp/tariboy-check.lock` call and released it at the end. The `-o`
flag closes the lock descriptor for the measured processes, so no left-over
process can keep the slot. After each step the script recorded the processes
that were still in the step's cgroup. No run left a process.

Each step ran in its own systemd user scope with memory accounting:

```bash
systemd-run --user --scope -p MemoryAccounting=yes -- \
  /usr/bin/time -v -o STEP.time bash -c 'STEP COMMAND; cat /sys/fs/cgroup/<scope>/memory.peak'
```

The report gives four memory numbers for each step:

- Max RSS: `Maximum resident set size` from `/usr/bin/time -v`. It is the
  largest single process of the step.
- Tree peak: `memory.peak` of the scope cgroup. It includes the page cache
  that the step reads, so it is higher than the real need of the step.
- Sum of RSS: the highest sum of `VmRSS` of all processes in the scope, read
  once per second. It misses peaks that last less than one second.
- anon: the highest `anon` value of `memory.stat` of the scope, read once per
  second. It is the memory that the step itself allocates.

To get per-package and per-test times, a `go` wrapper on `PATH` added `-json`
to `go test` in the `test` step. The `Makefile` stayed unchanged. The vitest
per-file times come from the lines of the `default` reporter
(`--reporter=default`).

A warm run used the shared caches of the host: `GOCACHE`, `GOMODCACHE`, the
cargo target in `~/.cache/tariboy/desktop-check-target`, and the
`node_modules` directories of the task worktree. A cold run used a second
worktree without `node_modules` and empty `GOCACHE`, `GOMODCACHE`,
`CARGO_HOME`, `TARIBOY_CARGO_CHECK_DIR` and `npm_config_cache` directories.
No shared cache of the host was removed.

### Limits of the data

- The first two attempts of the `test` step failed because of the measurement
  script, not the code. One `TMPDIR` path was too long for a Unix socket
  (`bind: invalid argument`). One `GO` wrapper path did not match the text
  that `TestDesktopE2EToolsCheckPrecedesEveryExpensiveStep` expects.
  `make test` stops after the first failed `go test`, so those attempts did
  not run `./internal/cli` and `./internal/plugins`. The `test` numbers in
  this report come from the separate runs `warm-j2-test` and `cold-j2-test`.
  Both runs passed.
- `cold-j2-test` started with an empty `GOCACHE`. In a real cold
  `make check`, `vet` and `staticcheck` run first and fill part of that
  cache, so the real cold `test` is somewhat faster than 313 s.
- The cold `ui-test` (3212 s) is not a valid time. During that run the host
  load average went up to 19, because of work from other agents. The memory
  numbers of that run are valid.
- The cold values of `ui-branding`, `ui-bundle-budget` and `docs` were not
  measured. Twice the host ran out of memory and the measurement session was
  stopped, once on `ui-test` and once while the chain waited for the gate.
  These three steps use only `node_modules`, so their cold time is expected
  to be close to the warm time. The cold cost of their dependencies is in the
  `npm-ci-*` rows.
- The optional warm run without `CHECK_JOBS` was not done for the same reason.
  The CI table below shows the same steps without a job limit.

## Local step table

The share is the share of the warm total of 692 s. "n/m" means not measured.
The cold total of the steps that have a valid cold time (all steps except
`ui-test`, `ui-branding`, `ui-bundle-budget` and `docs`) is 1054 s. The same
steps take 269 s warm.

| Step | Warm, s | Share | Cold, s | Max RSS, MB | Tree peak, MB | Sum of RSS, MB | anon, MB | Cold Max RSS, MB | Cold sum of RSS, MB |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `go-mod-download` | 1.1 | 0.2 % | 6.6 | 13 | 10 | 17 | 3 | 29 | 31 |
| `go-mod-download-tools` | 0.1 | 0.0 % | 2.1 | 12 | 5 | 0 | 0 | 26 | 25 |
| `fmt-check` | 7.2 | 1.0 % | 5.2 | 48 | 100 | 42 | 32 | 47 | 54 |
| `vet` | 5.2 | 0.8 % | 109.5 | 50 | 147 | 59 | 35 | 748 | 930 |
| `staticcheck` | 5.2 | 0.8 % | 71.8 | 59 | 169 | 97 | 66 | 1065 | 1033 |
| `gocyclo` | 1.1 | 0.2 % | 2.2 | 20 | 21 | 27 | 9 | 68 | 92 |
| `test` | 180.4 | 26.1 % | 313.0 | 452 | 937 | 784 | 323 | 723 | 844 |
| `desktop-rust-check` | 9.3 | 1.3 % | 306.5 | 341 | 798 | 676 | 325 | 1114 | 1576 |
| `tariboy-tasks-e2e` | 22.6 | 3.3 % | 125.5 | 467 | 719 | 513 | 178 | 706 | 779 |
| `smoke-contract` | 8.2 | 1.2 % | 7.2 | 7 | 21 | 19 | 2 | 7 | 18 |
| `npm-ci-ui` | 1.1 | 0.2 % | 35.9 | 60 | 19 | 55 | 11 | 566 | 557 |
| `npm-ci-api-types` | 1.1 | 0.2 % | 5.3 | 60 | 19 | 54 | 10 | 146 | 151 |
| `npm-ci-docs` | 1.1 | 0.2 % | 36.9 | 60 | 19 | 55 | 11 | 698 | 700 |
| `ui-fmt-check` | 2.2 | 0.3 % | 2.2 | 92 | 208 | 54 | 12 | 90 | 515 |
| `ui-api-types` | 3.2 | 0.5 % | 3.2 | 210 | 211 | 303 | 165 | 207 | 301 |
| `ui-typecheck` | 8.3 | 1.2 % | 9.4 | 1237 | 1285 | 1326 | 1253 | 1229 | 1283 |
| `ui-lint` | 2.1 | 0.3 % | 2.2 | 109 | 107 | 177 | 75 | 100 | 172 |
| `ui-dead-code` | 9.3 | 1.3 % | 9.3 | 387 | 390 | 450 | 335 | 384 | 394 |
| `ui-test` | 388.3 | 56.1 % | n/m (3212, invalid) | 4150 | 4314 | 4165 | 4116 | 3547 | 3560 |
| `ui-branding` | 2.2 | 0.3 % | n/m | 93 | 74 | 163 | 56 | n/m | n/m |
| `ui-bundle-budget` | 5.2 | 0.8 % | n/m | 737 | 744 | 794 | 666 | n/m | n/m |
| `docs` | 27.7 | 4.0 % | n/m | 1298 | 1486 | 1376 | 1220 | n/m | n/m |

The cold cache sizes after the cold run were: cargo target 1.4 GB,
`GOCACHE` 1006 MB, `GOMODCACHE` 459 MB, npm cache 406 MB, `CARGO_HOME`
273 MB.

### Go tests (warm, `warm-j2-test`)

The `test` step runs 85 packages and 2912 top-level tests. The sum of the
package times is 256 s. With `-p=2` the wall time is 180 s.

| # | Package | Elapsed, s |
|---|---|---:|
| 1 | `internal/commands` | 62.5 |
| 2 | `internal/tasks` | 30.2 |
| 3 | `internal/store` | 14.8 |
| 4 | `internal/loop` | 12.7 |
| 5 | `internal/aiproxy` | 11.3 |
| 6 | `internal/shim` | 10.4 |
| 7 | `internal/taskcli` | 9.7 |
| 8 | `cmd/tariboy` | 6.8 |
| 9 | `internal/daemon` | 5.4 |
| 10 | `internal/auditexport` | 5.2 |

| # | Test | Package | Elapsed, s |
|---|---|---|---:|
| 1 | `TestUpdateTaskRace` | `internal/aiproxy` | 8.8 |
| 2 | `TestTasksResponsesMatchGolden` | `internal/commands` | 8.2 |
| 3 | `TestDetailedHelpIsLocalForEveryCommand` | `internal/taskcli` | 7.5 |
| 4 | `TestAgentLoopResponsesMatchGolden` | `internal/commands` | 7.4 |
| 5 | `TestOpenRemovesJudgeTables` | `internal/store` | 6.5 |
| 6 | `TestListTasksScalesPast32kTasks` | `internal/tasks` | 6.2 |
| 7 | `TestAgentGoalsMigrationPreservesTasksAndAddsReleaseFields` | `internal/store` | 5.0 |
| 8 | `TestWriteZIPPreservesLargeTranscriptOrderAndBodies` | `internal/auditexport` | 4.8 |
| 9 | `TestScriptRunsMigrationPreservesLegacyHistoryAndBusSchedules` | `internal/store` | 4.7 |
| 10 | `TestPricingMigrationAddsSourcesAndGroupSnapshot` | `internal/store` | 4.5 |

### vitest files (warm, `warm-j2`)

`ui-test` runs 274 files and 2684 tests with `pool: "vmThreads"` and two
workers. The sum of the per-file times is 388 s, which is the same as the wall
time.

| # | Test file | Tests | Time, s |
|---|---|---:|---:|
| 1 | `src/pages/agents/AgentConfigurationTab.test.tsx` | 79 | 49.7 |
| 2 | `src/pages/mailbox/MailboxTaskAnswer.test.tsx` | 2 | 23.5 |
| 3 | `src/pages/tasks/TasksWorkspace.detail.test.tsx` | 23 | 19.6 |
| 4 | `src/pages/tasks/TasksWorkspace.test.tsx` | 25 | 16.8 |
| 5 | `src/pages/tasks/TaskDetail.test.tsx` | 24 | 14.8 |
| 6 | `src/pages/tasks/TaskDetail.decisions.test.tsx` | 13 | 11.7 |
| 7 | `src/App.test.tsx` | 66 | 11.3 |
| 8 | `src/pages/agents/AgentWorkspace.test.tsx` | 28 | 10.8 |
| 9 | `src/pages/tasks/WorkflowPanel.test.tsx` | 34 | 10.5 |
| 10 | `src/pages/tasks/TasksWorkspace.realtime.test.tsx` | 21 | 8.9 |

Worker memory: with `vmThreads`, both workers are threads of one Node.js
process. That process had a Max RSS of 4150 MB in the warm run and 3547 MB in
the cold run, and its anonymous memory was 4116 MB. The two workers cannot be
measured separately with these tools, because they share one process.

## CI table

These are the 12 most recent successful runs of the `make check` job of
`.github/workflows/pull-request.yml` up to 2026-11-03. The step times come
from the `==> STEP ok Ns` and `==> GROUP result: ok (Ns)` lines of each log.
The job step times come from `gh api` job data. CI runs without
`TARIBOY_AGENT`, so `CHECK_JOBS` is not set there: vitest uses 4 workers and
`go test` uses the default `-p`.

Head commits: 41590624019 `4a01911a`, 41586496279 `f77f6803`,
41583437595 `6286763d`, 41582078792 `81be4e93`, 41573508673 `5d146c70`,
41569382401 `8c14c0fa`, 41547237012 `ee038aef`, 41536667848 `b0fdc524`,
41531842379 `42dd7a0d`, 41519021485 `5471254d`, 41500446872 `ad2f9596`,
41480658280 `0a1d052a`.

Times are in seconds.

| Step | 41590624019 | 41586496279 | 41583437595 | 41582078792 | 41573508673 | 41569382401 | 41547237012 | 41536667848 | 41531842379 | 41519021485 | 41500446872 | 41480658280 | median |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `fmt-check` | 1 | 1 | 1 | 1 | 1 | 0 | 1 | 1 | 0 | 1 | 1 | 1 | 1.0 |
| `vet` | 57 | 37 | 43 | 55 | 40 | 54 | 43 | 38 | 34 | 48 | 60 | 55 | 45.5 |
| `staticcheck` | 41 | 28 | 31 | 40 | 28 | 40 | 32 | 29 | 25 | 35 | 43 | 42 | 33.5 |
| `gocyclo` | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0.0 |
| `test` | 72 | 52 | 66 | 69 | 51 | 67 | 67 | 46 | 46 | 59 | 75 | 70 | 66.5 |
| `desktop-rust-check` | 90 | 62 | 73 | 96 | 69 | 85 | 81 | 70 | 55 | 74 | 96 | 91 | 77.5 |
| `tariboy-tasks-e2e` | 46 | 30 | 37 | 44 | 32 | 43 | 35 | 32 | 28 | 39 | 48 | 47 | 38.0 |
| `smoke-contract` | 6 | 5 | 5 | 6 | 5 | 4 | 6 | 4 | 5 | 5 | 6 | 6 | 5.0 |
| `backend-check` (group) | 317 | 218 | 259 | 314 | 230 | 296 | 268 | 223 | 198 | 266 | 332 | 316 | 267.0 |
| `ui-fmt-check` | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | 0.0 |
| `ui-api-types` | 1 | 1 | 1 | 1 | 1 | 1 | 2 | 1 | 1 | 1 | 1 | 1 | 1.0 |
| `ui-typecheck` | 5 | 2 | 4 | 5 | 3 | 4 | 3 | 3 | 2 | 5 | 6 | 5 | 4.0 |
| `ui-lint` | 0 | 1 | 0 | 0 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0.0 |
| `ui-dead-code` | 4 | 2 | 3 | 4 | 3 | 3 | 3 | 2 | 3 | 3 | 4 | 4 | 3.0 |
| `ui-test` | 107 | 65 | 82 | 99 | 70 | 100 | 77 | 72 | 59 | 85 | 109 | 100 | 83.5 |
| `ui-branding` | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0.0 |
| `ui-bundle-budget` | 1 | 1 | 1 | 2 | 1 | 2 | 1 | 1 | 1 | 1 | 2 | 2 | 1.0 |
| `docs` | 11 | 7 | 9 | 11 | 8 | 10 | 9 | 8 | 7 | 9 | 12 | 11 | 9.0 |
| `frontend-check` (group) | 160 | 97 | 126 | 151 | 104 | 148 | 117 | 107 | 89 | 131 | 165 | 153 | 128.5 |

| Job step | 41590624019 | 41586496279 | 41583437595 | 41582078792 | 41573508673 | 41569382401 | 41547237012 | 41536667848 | 41531842379 | 41519021485 | 41500446872 | 41480658280 | median |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Set up job | 1 | 1 | 2 | 1 | 1 | 1 | 1 | 2 | 1 | 1 | 1 | 1 | 1.0 |
| Check out repository | 1 | 2 | 2 | 1 | 1 | 2 | 2 | 2 | 2 | 1 | 2 | 2 | 2.0 |
| Set up Go | 8 | 7 | 7 | 8 | 6 | 7 | 8 | 5 | 6 | 7 | 8 | 8 | 7.0 |
| Set up Node.js | 1 | 0 | 1 | 3 | 1 | 1 | 0 | 4 | 1 | 1 | 1 | 1 | 1.0 |
| Provide Codex test harness | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0 |
| Install check dependencies | 21 | 27 | 34 | 35 | 25 | 47 | 42 | 72 | 26 | 33 | 36 | 1254 | 34.5 |
| Run checks | 477 | 315 | 385 | 465 | 334 | 444 | 385 | 330 | 287 | 398 | 497 | 469 | 391.5 |
| Post Set up Node.js | 3 | 3 | 4 | 3 | 2 | 4 | 3 | 2 | 3 | 3 | 4 | 4 | 3.0 |
| Post Set up Go | 5 | 5 | 5 | 5 | 5 | 7 | 5 | 4 | 5 | 5 | 7 | 6 | 5.0 |
| Post Check out repository | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0.0 |
| Complete job | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0 |
| Job total | 521 | 364 | 443 | 524 | 378 | 516 | 451 | 424 | 334 | 453 | 566 | 1749 | 452.0 |

In CI, the `npm-ci-locked.sh` preparation of `frontend-check` is included in
the group time but has no step line. The median group time minus the sum of
the median step times is about 27 s. Run 41480658280 spent 1254 s in `apt-get` and is an
outlier.

## Causes of the most expensive steps

### Top 5 by time

The local warm order is `ui-test`, `test`, `docs`, `tariboy-tasks-e2e`,
`ui-dead-code`. The cold run adds `desktop-rust-check` and `vet`.

1. `ui-test`, 388 s warm, 84 s in CI. Vitest runs all 274 files with
   `pool: "vmThreads"`. Each file gets a new VM context, so each file imports
   React, jsdom and the page modules again. Ten files take 178 s, 46 % of the
   step. `AgentConfigurationTab.test.tsx` alone takes 50 s for 79 tests.
   `MailboxTaskAnswer.test.tsx` takes 23.5 s for only 2 tests. Its tests click
   the "Markdown" button, which probably loads the lazy rich-text editor
   module. Locally the step is 4.6 times slower than in CI because it has
   2 workers instead of 4 and shares the host with other agents.
2. `test`, 180 s warm, 313 s cold, 67 s in CI. `internal/commands` (62.5 s)
   and `internal/tasks` (30.2 s) are 36 % of the package time. With `-p=2`
   these two packages take most of one test slot. The slowest tests are golden
   response tests in `internal/commands`, a race test in `internal/aiproxy`,
   `TestDetailedHelpIsLocalForEveryCommand`, the 32k task list test, and store
   migration tests. The recipe runs `go test` four times: all packages, then
   `./internal/cli`, then `./internal/plugins` twice, once with each `TMPDIR`.
3. `desktop-rust-check`, 9 s warm, 306 s cold, 78 s in CI.
   `cargo check --locked --all-targets` compiles the Tauri crate and all its
   dependencies. The local warm run reuses
   `~/.cache/tariboy/desktop-check-target`. CI has no cargo cache, so every CI
   run compiles the dependencies again.
4. `tariboy-tasks-e2e`, 23 s warm, 126 s cold, 38 s in CI.
   `scripts/tariboy-tasks-e2e.sh` runs `go build` six times, once for each of
   `tariboyd`, `tariboy`, `tariboy-tasks`, `tariboy-mail`, `tariboy-shim` and
   `tariboy-plugin-telegram`, one after the other. Then it starts an isolated
   daemon. The compile output is cached, but the link of six binaries runs on
   each run.
5. `vet`, 5 s warm, 110 s cold, 46 s in CI. `go vet -tags=tmux` type-checks
   every package. In a cold run it is the first step that compiles the module.
   `staticcheck` (72 s cold, 34 s in CI) loads the same packages again with
   its own loader and its own cache.
6. `docs`, 28 s warm, 9 s in CI. `blume doctor` and `blume build` build the
   whole documentation site on each run, and nothing is cached.

### Top 5 by memory

The order uses the higher sum of RSS of the warm and the cold run.

1. `ui-test`, 4165 MB. The `vmThreads` pool runs both workers as threads of
   one Node.js process, so one process holds the jsdom environments and the
   module graphs of both workers. `vite.config.ts` does not set
   `vmMemoryLimit`, the limit at which vitest recycles a worker. Anonymous
   memory is 4116 MB, so the memory belongs to the test process and not to
   the page cache.
2. `desktop-rust-check`, 1576 MB cold, 676 MB warm. Two `rustc` processes
   (`--jobs 2`) compile large dependency crates. The largest single process
   had 1114 MB.
3. `docs`, 1376 MB. The Blume (Astro) build of the site runs in one Node.js
   process of 1298 MB.
4. `ui-typecheck`, 1326 MB. TypeScript 7 is the native `tsc`. One process
   checks all projects of `tsc -b` and uses 1237 MB. Node.js flags such as
   `--max-old-space-size` do not apply to it. Its own options `--checkers`,
   `--builders` and `--singleThreaded` control its threads.
5. `staticcheck`, 1033 MB cold, 97 MB warm. On an empty cache it analyzes all
   packages and their dependencies in one process of 1065 MB. `vet` (930 MB
   cold) is close.

`test` is not in this list: its largest process is 452 MB and its sum of RSS
is 784 MB warm.

## Repeated work

### On every run

- `ui-bundle-budget` runs a full production `vite build` into a temporary
  directory (`ui/scripts/bundle-budget.mjs`). It takes 5 s and 737 MB locally
  and builds the UI again on every run.
- `docs` builds the whole documentation site on every run (28 s, 1.3 GB).
- `tariboy-tasks-e2e` links six binaries on every run. `make build` in
  `full-check` links the same binaries again.
- `ui-test` runs all 274 files on every run, also for a change that touches
  one component.
- In CI, `Install check dependencies` runs `apt-get update` and installs the
  WebKit and GTK development packages on every run (median 35 s).
- In CI, `desktop-rust-check` compiles all crates on every run (median 78 s),
  because no cargo cache exists. `actions/setup-go` keeps `GOCACHE` and
  `GOMODCACHE`, and `actions/setup-node` keeps the npm download cache.
- Locally, `npm-ci-locked.sh` costs about 1 s for each directory when its
  fingerprint matches. When the fingerprint differs, it costs 78 s for the
  three directories.

### Inside one run

- The Go code is type-checked three times, by `go vet -tags=tmux`,
  `staticcheck -tags=tmux` and `go test -tags=tmux`. `go test ./internal/cli`
  runs without `-tags=tmux`, so it builds a second variant of its
  dependencies when tmux is installed. `go vet` and `go test` share the
  build cache. `staticcheck` keeps a cache of its own.
- `go list ./...` runs in `fmt-check`, `vet`, `staticcheck` and `test`. Each
  call takes under 1 s warm.
- The UI sources are parsed by `tsc -b`, by `oxlint`, twice by `knip`
  (`--production --include files`, then the full run), by vitest, and by the
  `vite build` of `ui-bundle-budget`.
- In `full-check`, `make ui` runs `tsc -b` and `vite build` again for the
  desktop bundle. This is outside `make check`.

## Proposals without loss of coverage

The time and memory gains are estimates from the numbers above. Each change
must be measured with the commands in "How to repeat the measurement".

| # | Proposal | Expected time gain | Expected memory gain | Work | Coverage risk |
|---|---|---|---|---|---|
| 1 | Limit the memory of `ui-test`: set `vmMemoryLimit` so vitest recycles a worker, or move to `pool: "forks"` or `"threads"` and compare. | Unknown, possibly faster with `threads` | The local peak of 4.1 GB is expected to go down to about 1.5 to 2.5 GB | S | None. A pool change can show tests that depend on the VM isolation. Fix those tests. |
| 2 | In agent mode, run `ui-test` as two sequential vitest processes with `--shard=1/2` and `--shard=2/2`. | 0 to 10 % slower | The peak is expected to drop to about half, because the first process exits before the second starts | S | None. Both shards together run every file. |
| 3 | Add a memory gate to the check slot. Before `ui-test`, wait until `MemAvailable` is at least a set value. | None, can add waiting | Prevents the out-of-memory failures, does not lower the peak | S | None |
| 4 | Make the slowest vitest files faster: `AgentConfigurationTab` (50 s), `MailboxTaskAnswer` (23.5 s for 2 tests, probably the lazy editor import), `TasksWorkspace*` and `TaskDetail*`. Load the heavy lazy module once, or mock it in tests that do not test the editor. | 60 to 100 s of 388 s locally | Small | M | None, if the editor keeps its own tests |
| 5 | Split `internal/commands` (62.5 s) and `internal/tasks` (30.2 s), or run more of their tests with `t.Parallel()`. | Up to 30 to 40 s with `-p=2` | Small | M | None |
| 6 | Cache the cargo target directory of `desktop-rust-check` in CI with `actions/cache`. Key it on `Cargo.lock` and the toolchain. | About 60 to 70 s of each CI run | None | S | None |
| 7 | Split the CI job into a `backend-check` job and a `frontend-check` job that run in parallel. | The job goes from 452 s to about 330 s, limited by `backend-check` | None | S | None. It costs more runner minutes. |
| 8 | Cache the `apt` packages in CI, or use a runner image that already has them. | About 25 to 30 s of each CI run | None | S | None |
| 9 | Build the six binaries of `tariboy-tasks-e2e` with one `go build -o DIR ./cmd/...` call, so the links run in parallel. | A few seconds of 23 s warm | Higher peak during the link | S | None |
| 10 | Run `vet` and `staticcheck` as one `staticcheck` run that includes the vet checks, or run vet through the same loader. | Up to 46 s in CI, up to 110 s cold | Lower cold peak | M | Low. Make sure that every `go vet` analyzer stays enabled. |
| 11 | Limit the threads of `tsc -b` in agent mode with `--checkers` or `--singleThreaded`. | Slower by an unknown amount | Unknown. It must be measured. | S | None |
| 12 | Cache the `docs` build output between local runs and rebuild only when `docs/` changes. | 28 s locally when `docs/` did not change | 1.3 GB peak skipped | M | Low. The cache key must include every input of the site. |

## Proposals that reduce coverage

These proposals make the check faster, but they skip work that the check
does today. Each one needs a decision by the customer.

| # | Proposal | Expected time gain | Expected memory gain | Work | Coverage risk |
|---|---|---|---|---|---|
| A | Incremental mode: run `go test` only for the changed packages and the packages that depend on them (`go list -deps -test`), and run `vitest --changed`. | Most of `test` and `ui-test` for a small change | Most of the `ui-test` peak for a small change | M | Medium. Tests that read non-Go inputs, such as `Makefile`, `testdata`, scripts or the OpenAPI snapshot, can be skipped by mistake. |
| B | Run `docs`, `ui-bundle-budget`, `smoke-contract` and `desktop-rust-check` only in CI, not in the agent `make check`. | About 50 s warm locally, 306 s cold | Removes the 1.3 to 1.6 GB peaks of `docs` and cold `desktop-rust-check` locally | S | Medium. Agents find these failures only in CI. |
| C | Run `ui-test` only in CI for agents, and run only the files that are related to the change locally. | Up to 388 s locally | Removes the 4.1 GB peak locally | S | High. Most UI failures then appear only in CI. |
| D | Do not run the `./internal/plugins` long-socket tests in the agent `make check`. | A few seconds | Small | S | Low, but the long-socket path then runs only in CI. |

## How to repeat the measurement

The scripts below are kept in the agent workdir and are not part of the
repository. They are small. A future measurement can write them again from
this description.

1. Make sure that no other check runs and that the host has at least 2500 MB
   available memory and a 1-minute load average of 4 or less.
2. Run one measured step, for example `ui-test`, in a worktree that already
   has `node_modules`:

   ```bash
   cd WORKTREE
   export TARIBOY_AGENT=me CHECK_JOBS=2 GOFLAGS=-p=2 TARIBOY_VITEST_MAX_WORKERS=2 \
     TARIBOY_CHECK_LOCK_HELD=1 CGO_ENABLED=0
   export TMPDIR=$(mktemp -d /tmp/tbm.XXXXXX)
   flock -o /tmp/tariboy-check.lock \
     systemd-run --user --scope -q -p MemoryAccounting=yes -- \
     /usr/bin/time -v -o ui-test.time \
     bash -c 'cd ui && npm test -- --reporter=default; cat /sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup)/memory.peak'
   ```

3. Read `Elapsed (wall clock) time` and `Maximum resident set size` in
   `ui-test.time`. The last output line is `memory.peak` in bytes.
4. To record the sum of RSS, read once per second the `cgroup.procs` file of
   the scope and add the `VmRSS` values from `/proc/PID/status`.
5. For the other steps, use the commands of `backend-check` and
   `frontend-check` in the `Makefile`. Use `make --no-print-directory STEP`
   for a `Makefile` target, and the `cd ui && npm run ...` command for a UI
   step.
6. For per-package Go times, put a `go` wrapper first on `PATH` that runs
   `go test -json "$@"` for `go test` and passes every other command to the
   real `go`. Do not set `GO` to the wrapper: the
   `TestDesktopE2EToolsCheckPrecedesEveryExpensiveStep` test reads `$(GO)`
   from a `make -n` dry run.
7. For a cold run, use a new worktree without `node_modules` and set empty
   directories in `GOCACHE`, `GOMODCACHE`, `CARGO_HOME`,
   `TARIBOY_CARGO_CHECK_DIR` and `npm_config_cache`. Do not delete the shared
   caches of the host.
8. Keep `TMPDIR` short, for example `/tmp/tbm.XXXXXX`. A long `TMPDIR` makes
   the Unix socket tests fail with `bind: invalid argument`.
9. For the CI table, list the runs with
   `gh run list --workflow pull-request.yml --status success`. Read
   `gh run view RUN --log` and the jobs of the run with
   `gh api repos/OWNER/REPO/actions/runs/RUN/jobs`. Take the step times from
   the `==> STEP ok Ns` and `==> GROUP result: ok (Ns)` lines.
