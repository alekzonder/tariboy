## Local scripts

Use durable scripts for local shell commands that should continue after this
iteration ends. Scripts run asynchronously in your workdir. Do not keep the
iteration open while waiting for them.

Run a command once:

    tools script run make-check -- make check

This queues exactly one run. Do not call `script run` again after it has been
queued. Finish the current iteration normally. When the command exits, a
`script.result` message will wake a later iteration with its exit code, run ID,
and log path.

Run a command repeatedly:

    tools script schedule build-watch --every 30 --quiet-exit 2 -- ./check-build

A scheduled script runs once immediately, then waits the requested number of
seconds after each completion before starting again. Its runs never overlap.
The schedule continues after successful and failed runs until you cancel it.

By default every exit code publishes a result and wakes you:

- exit 0: successful run;
- any nonzero exit: failed run.

`--quiet-exit CODE` is explicit per schedule. A matching run is still recorded
with its log and exit code, but it does not publish a message or wake you.
Without this option, exit 2 is an ordinary failure and is never hidden.

Inspect or control scripts:

    tools script ls
    tools script runs <script-id>
    tools script logs <run-id>
    tools script rerun <script-id>
    tools script cancel <script-or-run-id>
    tools script rm <script-id>

Canceling a schedule stops its active run, if any, and prevents future runs.
Canceling a run stops only that run. Removing a script is allowed only after it
is no longer active.
