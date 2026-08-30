## Finishing this iteration
The Python script lives inside this skill directory under `scripts/` and calls
the identity-bound daemon through `TARIBOY_TOOLS_SOCKET`.

Only the root iteration owner may run `i-am-done` (with or without `--idle`).
A subagent must never run `i-am-done`: when its assigned work is complete, it
must return its result to its parent. This remains true even if the parent is
unavailable, the subagent has no active work, or the task asks it to finish.

You run in iterations inside a repeating loop. One iteration is a single pass
over your task — not the whole loop. When you have done everything this iteration
requires, your very last action MUST be to run, exactly as written:
    i-am-done
`i-am-done` is a bare command already on your $PATH. Run it as that one word —
do NOT prefix it with a directory, and do NOT `find`/`ls`/`which` for a file first;
those paths do not exist. If it ever reports "command not found", just run the
bare word again — that is always the fix.

It closes the current iteration; it does NOT stop the loop, and stopping the loop
is not your job. If you exit without it the iteration is recorded as incomplete
(no_i_am_done) and the loop can get stuck. If there is no work this iteration, run
it anyway to close the empty iteration. Calling it more than once is safe — if
unsure whether you already did, run it again.

If this iteration did no useful work — you woke, found nothing to do, and ran only
routine checks — close it with `i-am-done --idle` instead. Plain `i-am-done` is the
default and declares the iteration productive; `--idle` is your honest signal that
it was not. Report it truthfully: the loop counts consecutive idle iterations and
can auto-stop after enough of them, so an empty iteration mislabelled productive
keeps a loop running with nothing left to do. Getting it wrong is cheap — forget
`--idle` and the iteration merely counts as productive — so reach for it only when
the work was genuinely empty, and keep bare `i-am-done` for any run where you
changed, checked, or advanced something real.

Before either form of i-am-done, account for every active subagent (parallel
task/worker) and every background command/process launched through a terminal
tool, regardless of when it started. Do NOT run i-am-done while any remains
active. A terminal tool returning a live session ID, "still running", or only
partial output means that command is still active; starting it is not completing
it. Expected success, an approaching timeout, or deciding the result is probably
unimportant are not exceptions.

Wait for every subagent and background terminal command/process to finish. Read
each final result, including terminal output and exit status, resolve any failure,
and fold the result into your work. i-am-done begins closing the current
iteration and can take its in-flight work down with it, so never assume a
terminal job will safely continue afterward. Run i-am-done only once none
remains active and all work is fully complete.

Manager-owned durable scripts started with `tools script run` or
`tools script schedule` are different:
their result is intentionally delivered as an inbox message in a later iteration,
so they do not block i-am-done. This exception applies only to that durable tool,
not to a command launched through a terminal.
