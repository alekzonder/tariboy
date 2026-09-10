# Agent Shell Scripts Design

## Status

Approved by the customer on Native Task `IMPROVE-65` on 2026-09-10.

## Behavior

- Server **Settings → General** exposes a **Global Agent Shell Script** textarea.
- Agent **Configuration** exposes an **Agent Shell Script** textarea.
- The global value is stored at `<base-dir>/global-agent-shell.sh`.
- The agent value is stored at `<base-dir>/agents/<name>/agent-shell.sh`.
- Saving validates the complete draft with `bash -n`. Invalid Bash is returned
  to the UI and never replaces the last valid file.
- Missing and empty files are no-ops.
- Before every harness launch, Bash sources the global script and then the
  agent script in one shell before `exec`-ing the unchanged harness command.
  Exports and filesystem effects are therefore available to that iteration.
- A source-time failure prevents the harness from starting and follows the
  existing harness-error lifecycle.

## Boundaries

- Reads and writes use the route-selected daemon.
- Script files and atomic temporary files are owner-only.
- Invalid UI drafts stay editable after a failed save.
- No database migration, dependency, shell selector, create/clone field, or
  generated Desktop artifact is added.
- Verification uses `make check`; `make full-check` is excluded by customer
  request.
