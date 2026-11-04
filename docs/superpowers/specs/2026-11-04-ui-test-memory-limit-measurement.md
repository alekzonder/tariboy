# ui-test memory limit and tsc checkers: measurement report

This report measures two changes to `make check` before and after, on the
shared host and in CI:

- `ui/vite.config.ts`: `vmMemoryLimit` is an absolute 336 MiB
  (`352321536` bytes). Before, it was 60 % of the heap limit of a Node.js
  thread, which is 1344 MiB on the host.
- `Makefile`: the `ui-typecheck` step runs `npx tsc -b --checkers 1`. Before,
  it ran `npx tsc -b`, which uses the default number of parallel checkers.

The recommendation came from
`docs/superpowers/specs/2026-11-03-ui-test-memory-and-runtime-report.md`.
The `build:*` scripts in `ui/package.json` still run `tsc -b`, because they
are not part of `make check`.

## Summary

| KPI | Before | After | Change |
| --- | ---: | ---: | --- |
| Host `ui-test` peak | 3600, 3676 MB | 1523, 1479 MB | -2.1 GB (-59 %) |
| Host `ui-test` time | 204, 207 s | 222, 224 s | +17 s (+8 %) |
| Host `ui-typecheck` peak | 1305, 1270 MB | 849, 828 MB | -0.45 GB (-35 %) |
| Host `ui-typecheck` time | 6.1, 5.1 s | 7.1, 7.1 s | +1.5 s |
| Host `make check` peak (`memory.peak`) | 4115, 3966 MB | 2868 MB | -1.2 GB |
| Host `make check` peak anon memory | 3365 MB | 1317 MB | -2.0 GB |
| Host `make check` time | 452, 441 s | 460 s | +14 s (+3 %) |
| CI `ui-test` peak | 5970 MB | 1972 MB | -4.0 GB (-67 %) |
| CI `ui-test` time | 105 s | 120 s | +15 s (+14 %) |
| CI `ui-typecheck` peak | 1263 MB | 886 MB | -0.38 GB (-30 %) |
| CI `ui-typecheck` time | 4.4 s | 4.0 s | no change |
| CI `make frontend-check` peak | 8073 MB | 3633 MB | -4.4 GB (-55 %) |
| CI `frontend-check` job time | 152 s (median of 3 `main` runs) | 180 s (median of 180, 139, 185 s) | +28 s (+18 %) |

Every `ui-test` run finished all test files:
`ui-test: vitest finished 303 of 303 test files`. The check of an incomplete
run (`scripts/vitest-run.mjs`, `scripts/vitest-file-ledger.mjs`) did not
change.

## Method

### Host

| Item | Value |
| --- | --- |
| Date | 2026-11-04 |
| Before | `main` at `4d516f8ea55d9bfd12d45512d9dbc09f78589a93` |
| After | the branch of this change, the same base, without the report |
| `nproc`, RAM | 6, 5924 MB, shared by all agents |
| Node.js | v24.22.2 |
| vitest workers | 2 (`TARIBOY_VITEST_MAX_WORKERS=2`, as in agent `make check`) |

One `make check` without a measurement warmed the Go and Cargo caches first.
Each measurement then took the host lock alone and ran alone. `measure.py`
(full text in the appendix) runs the command in its own cgroup with
`systemd-run --user --scope`. It reads `memory.peak` of the cgroup after the
run. Each second, it records the sum of RSS of the process tree and
`memory.current`, `anon` and `file` of the cgroup. The time is the wall time.

```bash
D=<agent workdir>
WT=<worktree>
export TARIBOY_VITEST_MAX_WORKERS=2

# ui-test, runs 1 and 2
flock -o /tmp/tariboy-check.lock \
  python3 -I $D/measure.py $D/out ui-test-PHASE-N $WT/ui -- npm test

# ui-typecheck, runs 1 and 2, each without the tsc -b build cache
rm -rf $WT/ui/node_modules/.tmp
flock -o /tmp/tariboy-check.lock \
  python3 -I $D/measure.py $D/out typecheck-before-N $WT/ui -- npx tsc -b
rm -rf $WT/ui/node_modules/.tmp
flock -o /tmp/tariboy-check.lock \
  python3 -I $D/measure.py $D/out typecheck-after-N $WT/ui -- npx tsc -b --checkers 1

# make check; the outer flock holds the host slot, the inner make check
# takes a private lock and does not wait for itself
rm -rf $WT/ui/node_modules/.tmp
TARIBOY_DOCS_REBUILD=1 TARIBOY_CHECK_LOCK=$D/measure-check.lock \
  flock -o /tmp/tariboy-check.lock \
  python3 -I $D/measure.py $D/out check-PHASE $WT -- make check
```

`TARIBOY_DOCS_REBUILD=1` makes the `docs` step build the site in every run.
The first `make check` before the change ran with an older `measure.py` that
did not sample `anon` and `file`, so the run `check-before-2` repeated it with
the sampling.

### CI

A temporary commit ran the `frontend-check` job under a measurement and was
removed before the review. The script (full text in the appendix) runs a
command in its own cgroup with `sudo systemd-run --scope` as the `runner` user
and prints `memory.peak` and the wall time. The job ran three measurements in
order:

1. `make frontend-check`, the normal job step.
2. `ui-typecheck` alone, with the command from the `Makefile` and without
   `ui/node_modules/.tmp`.
3. `npm test` alone in `ui/`.

The runner is `ubuntu-latest` with 4 CPUs and 15983 MB of RAM, Node.js
v22.23.4 and 4 vitest workers.

| Run | Commit | Link |
| --- | --- | --- |
| Before, measured | `db007c3b` (`main` and the measurement) | [run 41772270457, job `frontend-check`](https://github.com/alekzonder/tariboy/actions/runs/41772270457/job/126384459712) |
| After, measured | `5362225b` (the change and the measurement) | [run 41775402409, job `frontend-check`](https://github.com/alekzonder/tariboy/actions/runs/41775402409/job/126394461463) |
| Before, job time | `4d516f8e` | [run 41735888229](https://github.com/alekzonder/tariboy/actions/runs/41735888229/job/126264857136): 152 s |
| Before, job time | `2fe12895` | [run 41726212209](https://github.com/alekzonder/tariboy/actions/runs/41726212209/job/126231532916): 163 s |
| Before, job time | `1f11b165` | [run 41721412649](https://github.com/alekzonder/tariboy/actions/runs/41721412649/job/126214852327): 136 s |
| After, job time | `454d0d11` (the change without the measurement) | [run 41776977786, attempt 1](https://github.com/alekzonder/tariboy/actions/runs/41776977786/job/126399499106): 180 s |
| After, job time | `454d0d11`, the job run again | [run 41776977786, attempt 2](https://github.com/alekzonder/tariboy/actions/runs/41776977786/job/126400584714): 139 s |
| After, job time | `3d63d2e3` (the change and the first version of this report) | [run 41777646420](https://github.com/alekzonder/tariboy/actions/runs/41777646420/job/126401616416): 185 s |

The job time is `completedAt - startedAt` of the job from
`gh run view RUN --json jobs`. The measured runs are slower than a normal run,
because they run `ui-typecheck` and `ui-test` a second time.

## Results

### Host

| Run | Peak, MB | Peak anon, MB | Max RSS sum, MB | Time, s |
| --- | ---: | ---: | ---: | ---: |
| `ui-test-before-1` | 3600 | not sampled | 3617 | 204.2 |
| `ui-test-before-2` | 3676 | not sampled | 3703 | 207.2 |
| `ui-test-after-1` | 1523 | 1371 | 1611 | 221.9 |
| `ui-test-after-2` | 1479 | 1385 | 1599 | 223.8 |
| `typecheck-before-1` | 1305 | not sampled | 1175 | 6.1 |
| `typecheck-before-2` | 1270 | not sampled | 1329 | 5.1 |
| `typecheck-after-1` | 849 | 792 | 863 | 7.1 |
| `typecheck-after-2` | 828 | 820 | 891 | 7.1 |
| `check-before` | 4115 | not sampled | 3376 | 451.7 |
| `check-before-2` | 3966 | 3365 | 3544 | 440.7 |
| `check-after` | 2868 | 1317 | 1524 | 459.8 |

Step times in `make check`, from its summary table:

| Step | `check-before` | `check-before-2` | `check-after` |
| --- | ---: | ---: | ---: |
| `test` | 128 s | 123 s | 121 s |
| `ui-typecheck` | 6 s | 6 s | 7 s |
| `ui-test` | 211 s | 209 s | 230 s |
| `docs` | 25 s | 22 s | 21 s |
| `backend-check` | 190 s | 184 s | 184 s |
| `frontend-check` | 261 s | 255 s | 275 s |

### CI

| Measurement | Peak before, MB | Peak after, MB | Max RSS before, MB | Max RSS after, MB | Time before, s | Time after, s |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `make frontend-check` | 8073 | 3633 | 6366 | 2088 | 154.3 | 171.0 |
| `ui-typecheck` alone | 1263 | 886 | 1242 | 865 | 4.4 | 4.0 |
| `npm test` alone | 5970 | 1972 | 5884 | 1976 | 105.0 | 119.8 |
| `ui-test` step in `make frontend-check` | | | | | 104 | 121 |

In the measured run before the change, `npm test` alone failed one test,
`TerminalsPage.sections.test.tsx` ("Found multiple elements with the text:
a1"). The same test passed in `make frontend-check` of the same job and in
every other run. The change does not touch this test.

## Peak of `make check` on the host (criterion: at most 1.8 GB)

The target is not met for `memory.peak`: 2868 MB after the change. The
target is met for the memory that the processes use: the peak anon memory
of the cgroup is 1317 MB and the peak sum of RSS is 1524 MB.

The peak step is `ui-test` in both runs. The CSV of `check-after` shows the
cause:

| Time from the start | Running step | anon, MB | file, MB | `memory.current`, MB |
| --- | --- | ---: | ---: | ---: |
| 120 to 180 s | `test` (Go) | 168 to 346 | 1553 to 1711 | 1691 to 1846 |
| 210 to 270 s | `ui-test` | 1268 to 1317 | 1486 to 1491 | 2815 to 2864 |

The Go `test` step reads the Go build and test caches. That page cache
(`file`, about 1.5 GB) stays charged to the cgroup of the run, because the
kernel reclaims it only under memory pressure. `ui-test` then adds its own
1.3 GB of anon memory, and `memory.peak` counts both. Before the change, the
anon memory of `ui-test` (3365 MB) pushed the page cache down to 480 MB, so
`memory.peak` was mostly anon memory. The page cache is free for other
processes when they need memory, so the real demand of `make check` is now
about 1.3 to 1.5 GB.

## Time (criterion: at most +5 %)

Host `make check`: 460 s against 452 and 441 s, which is +2 % and +4 %. The
`ui-test` step is 19 to 21 s slower. The `test` and `docs` steps are 2 to
4 s faster, inside the host noise.

CI `frontend-check` job: three runs after the change took 180, 139 and
185 s, median 180 s. The three `main` runs took 136, 163 and 152 s, median
152 s. The job is 28 s (18 %) slower, which is more than the 5 % limit. The
`ui-test` step explains the difference: 122, 90 and 126 s after the change
(121 s in the measured run) against 78, 99 and 98 s on `main`. The other
steps, `npm ci` and `ui-typecheck` (3 to 4 s) did not change. One attempt
(139 s, `ui-test` 90 s) was faster than `main`, so the runner noise is large,
but two of three runs and the measured run show the slowdown.

On the host, `ui-test` is slower in all four runs after the change (222 to
230 s against 204 to 211 s). The cause on the host and in CI is the smaller
limit. Vitest restarts
a `vmThreads` worker each time its heap passes 336 MiB, and every new worker
loads `jsdom`, the setup file and the shared modules again. In CI, with 4
workers, the `ui-test` time grows by 20 to 25 s. On the host, with 2
workers, it grows by 17 to 21 s. This report did not count the restarts.
The task keeps 336 MiB. A larger value trades memory for time.

## Appendix: measurement files

`measure.py`, in the agent workdir, not in the repository:

```python
#!/usr/bin/env python3
"""Measure one command: wall time, /usr/bin/time Max RSS, cgroup memory.peak
and memory.stat, and RSS of the process tree sampled every second.

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


def cgroup_dir(pid):
    try:
        with open(f"/proc/{pid}/cgroup") as f:
            return "/sys/fs/cgroup" + f.read().strip().split(":", 2)[2]
    except OSError:
        return None


def cg_sample(cg):
    """memory.current, anon and file of the cgroup in MB."""
    try:
        cur = int(open(cg + "/memory.current").read()) // 1048576
        st = dict(line.split() for line in open(cg + "/memory.stat"))
        return cur, int(st["anon"]) // 1048576, int(st["file"]) // 1048576
    except (OSError, KeyError, ValueError, TypeError):
        return None, None, None


def main():
    outdir, name, cwd = sys.argv[1], sys.argv[2], sys.argv[3]
    cmd = sys.argv[5:]
    base = os.path.join(outdir, name)
    subprocess.run([GATE], check=False)
    before = memavail_mb()
    inner = ('/usr/bin/time -v -o "$T" "$@"; rc=$?; '
             'cg="/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup)"; '
             'cat "$cg/memory.peak" > "$P"; cat "$cg/memory.stat" > "$S"; exit $rc')
    env = dict(os.environ, T=base + ".time", P=base + ".peak", S=base + ".stat")
    csv = open(base + ".csv", "w")
    csv.write("t,sum_rss_mb,memavail_mb,cg_current_mb,cg_anon_mb,cg_file_mb\n")
    t0 = time.time()
    p = subprocess.Popen(["systemd-run", "--user", "--scope", "-q", "--", "sh", "-c", inner, "sh", *cmd],
                         cwd=cwd, stdout=open(base + ".log", "w"), stderr=subprocess.STDOUT, env=env)
    max_rss_sum = max_anon = 0
    at_max_anon = None
    cg = None
    while p.poll() is None:
        pids = tree(p.pid)
        total = sum(rss_kb(x) for x in pids) // 1024
        max_rss_sum = max(max_rss_sum, total)
        if cg is None or cg.endswith("/"):
            for x in pids[1:]:
                c = cgroup_dir(x)
                if c and "scope" in c:
                    cg = c
                    break
        cur, anon, fil = cg_sample(cg) if cg else (None, None, None)
        if anon is not None and anon > max_anon:
            max_anon, at_max_anon = anon, dict(t=round(time.time() - t0, 1), current=cur, file=fil)
        csv.write(f"{time.time() - t0:.1f},{total},{memavail_mb()},{cur},{anon},{fil}\n")
        csv.flush()
        time.sleep(1)
    wall = round(time.time() - t0, 1)
    peak = stat = None
    try:
        peak = int(open(base + ".peak").read()) // (1024 * 1024)
        stat = {k: int(v) // (1024 * 1024) for k, v in
                (line.split() for line in open(base + ".stat")) if k in ("anon", "file")}
    except (OSError, ValueError):
        pass
    print(json.dumps(dict(name=name, exit=p.returncode, wall_s=wall, peak_mb=peak, stat_mb=stat,
                          max_rss_sum_mb=max_rss_sum, max_anon_mb=max_anon,
                          at_max_anon=at_max_anon, memavail_before_mb=before)))


if __name__ == "__main__":
    main()
```

The temporary CI step in `.github/workflows/pull-request.yml`, job
`frontend-check`:

```yaml
# TEMPORARY memory measurement. Remove this commit before merge.
- name: Run frontend checks
  run: |
    cat > /tmp/measure.sh <<'EOF'
    #!/usr/bin/env bash
    # measure.sh NAME CMD...: run CMD in its own cgroup, print wall time,
    # memory.peak and memory.stat of that cgroup.
    name=$1; shift
    inner='start=$(date +%s.%N); /usr/bin/time -v -o /tmp/$N.time "$@"; rc=$?; end=$(date +%s.%N);
    cg=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup);
    echo "MEASURE $N wall_s=$(awk "BEGIN{print $end - $start}") peak_mb=$(( $(cat $cg/memory.peak) / 1048576 )) rc=$rc";
    grep -E "^(anon|file) " $cg/memory.stat | sed "s/^/MEASURE $N stat /";
    grep -E "Maximum resident|Elapsed" /tmp/$N.time | sed "s/^/MEASURE $N time /"; exit $rc'
    sudo env "PATH=$PATH" "HOME=$HOME" "CI=${CI:-}" "N=$name" \
      systemd-run --scope -q --uid="$(id -u)" --gid="$(id -g)" \
      -- bash -c "$inner" bash "$@"
    EOF
    chmod +x /tmp/measure.sh
    nproc; free -m; node --version
    /tmp/measure.sh frontend-check make frontend-check
    typecheck=$(grep -o "npx tsc -b[^']*" Makefile)
    echo "typecheck command: $typecheck"
    (cd ui && rm -rf node_modules/.tmp && /tmp/measure.sh ui-typecheck $typecheck)
    (cd ui && /tmp/measure.sh ui-test npm test)
```
