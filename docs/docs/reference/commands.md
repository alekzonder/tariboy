---
title: Command reference
description: The full command reference — operator commands and agent tools, generated from the binary registry.
sidebar:
  label: Commands
  icon: terminal
---

# tariboy command reference

tariboy has three command surfaces:

1. **Operator commands** — `tariboy <group> <command>` (`sa` below is a
   convenience alias you define yourself, e.g. `alias sa=tariboy`; the build
   ships no such symlink), run by a
   human or CI against the daemon. Global flags: `--json`, `--help`,
   `--help-json`, `--version`. This is the authoritative list, generated from the
   binary's registry (`tariboy --help-json`).
2. **Agent tools** — `tools <group> <command>`, run *inside* an agent (the
   `tools` shim talks to the per-agent socket `$TARIBOY_TOOLS_SOCKET`).
3. **Native Tasks** — optional `tasks <verb>`, run inside an enabled agent
   against daemon-owned work.

## Operator commands

| Command | Summary |
| --- | --- |
| `tariboy agent exec` | Run a manual iteration now, with an optional one-shot prompt |
| `tariboy agent inspect` | Show one agent's full config |
| `tariboy agent kill` | Kill the current iteration via its shim |
| `tariboy agent ps` | List agents |
| `tariboy agent pull` | Read a base64 file from an agent's cwd (used by 'cp') |
| `tariboy agent push` | Write a base64 file into an agent's cwd (used by 'cp') |
| `tariboy agent restart` | Restart an agent and run one iteration now |
| `tariboy agent rm` | Remove an agent (stop it first, or --force) |
| `tariboy agent run` | Create and start an agent from an image (image:tag) |
| `tariboy agent screen` | Capture the interactive screen (tmux capture-pane) |
| `tariboy agent send-keys` | Send keys into the interactive session (tmux send-keys) |
| `tariboy agent start` | Start (enable) an agent's loop |
| `tariboy agent status history` | List newest-first status messages from the audit log, including each event's iteration ID when present |
| `tariboy agent status show` | Show one agent's runtime status |
| `tariboy agent stop` | Stop an agent's loop (current iteration lives) |
| `tariboy backup` | Back up an agent (or 'all') to a portable tar.gz |
| `tariboy budget ls` | List configured budgets |
| `tariboy budget set` | Set a cost budget (scope agent:\<name\>\|group:\<g\>\|global) |
| `tariboy budget status` | Show current spend vs limit for each budget |
| `tariboy channel inspect` | Show a channel's kind and message count |
| `tariboy channel ls` | List channels |
| `tariboy channel tail` | Print recent messages on a channel (-f to follow) |
| `tariboy compose archive` | Create a compose-only portable team archive; transfer runnable images separately |
| `tariboy compose import` | Preview and import compose-only team/runtime configuration |
| `tariboy cp` | Copy files to/from an agent cwd: cp SRC AGENT:DST \| AGENT:SRC DST |
| `tariboy daemon config get` | Read daemon config (all keys, or one with --key) |
| `tariboy daemon config set` | Set a daemon config key (runtime-mutable) |
| `tariboy daemon reindex` | Rebuild ai_requests metadata from proxy-transcript.jsonl files |
| `tariboy daemon status` | Show daemon version, uptime and base directory |
| `tariboy eval inspect` | Show all eval results for an iteration (with the image version) |
| `tariboy eval ls` | List recent eval results (verdict/score per iteration + image version) |
| `tariboy group assign` | Assign an agent to a group (empty group leaves) |
| `tariboy group create` | Create (or update) a group with an optional lead |
| `tariboy group inspect` | Show a group's lead, members, channels and shared dir |
| `tariboy group ls` | List groups (name/lead/member count) |
| `tariboy group rm` | Remove a group (detach members, delete channels; --volumes drops the shared dir) |
| `tariboy image build --path DIR --name NAME [--tag TAG] [--repository-id ID --git-commit SHA]` | Build an immutable image and source snapshot; optional Git provenance must be provided as a pair |
| `tariboy image validate --path DIR --name NAME [--tag TAG]` | Validate the source and target ref without publishing; tag defaults to `latest` |
| `tariboy image inspect` | Show an image manifest |
| `tariboy image ls` | List built agent images |
| `tariboy image prompt` | Print an image's assembled prompt |
| `tariboy image template` | Show the ordered schema-v2 static/runtime template |
| `tariboy image rm` | Remove a built image |
| `tariboy image-release inspect` | Show immutable image release provenance |
| `tariboy image-release rollback` | Stage the prior immutable image from a completed rollout |
| `tariboy image-release rollout approve` | Approve an exact image release rollout |
| `tariboy image-release rollout reject` | Reject an exact image release rollout |
| `tariboy image-release rollout stage` | Stage an approved release for one agent |
| `tariboy improvement inspect` | Show an agent improvement proposal |
| `tariboy improvement ls` | List agent improvement proposals |
| `tariboy improvement plan approve` | Approve an exact improvement plan revision |
| `tariboy improvement plan reject` | Reject an exact improvement plan revision |
| `tariboy judge cancel` | Cancel an LLM-as-Judge run while preserving immutable artifacts |
| `tariboy judge evidence` | Read immutable judge evidence by stable locator |
| `tariboy judge inspect` | Show an LLM-as-Judge run, targets, analyses, summaries and target usage |
| `tariboy judge ls` | List LLM-as-Judge runs |
| `tariboy judge retry` | Retry failed assignments in an LLM-as-Judge run |
| `tariboy judge automation get` | Read the active Judge automation revision |
| `tariboy judge automation validate --json JSON` | Validate raw JSON in `tariboyd` without applying it |
| `tariboy judge automation apply --json JSON` | Apply JSON, create `JUDGE`/`IMPROVE`, and reconcile the recurring schedule without starting a review |
| `tariboy judge automation run-once --limit N` | Queue one immediate cycle through the existing scheduler |
| `tariboy iteration inspect` | Show one iteration |
| `tariboy iteration logs` | Print an iteration's harness logs |
| `tariboy iteration ls` | List an agent's iterations |
| `tariboy logs` | Stream or print an agent's events (-f to follow) |
| `tariboy loop disable` | Turn the agent loop disable |
| `tariboy loop enable` | Turn the agent loop enable |
| `tariboy loop hard-timeout` | Get or set loop hard-timeout (seconds); omit value to read |
| `tariboy loop interval` | Get or set loop interval (seconds); omit value to read |
| `tariboy loop on-error` | Get or set loop on-error policy; omit value to read |
| `tariboy loop on-timeout` | Get or set loop on-timeout policy; omit value to read |
| `tariboy loop timeout` | Get or set loop timeout (seconds); omit value to read |
| `tariboy message send` | Publish a message to a channel (operator) |
| `tariboy plugin inspect` | Show one plugin's manifest, state and socket |
| `tariboy plugin install` | Install and start a plugin from a directory with plugin.json |
| `tariboy plugin logs` | Show captured plugin stdout/stderr |
| `tariboy plugin ls` | List installed plugins (name/version/types/state/health) |
| `tariboy plugin rm` | Stop and remove a plugin |
| `tariboy prune` | Prune old iterations for an agent now (or 'all'); --dry-run lists victims |
| `tariboy restore` | Restore an agent from a backup tar.gz (optionally under a new name) |
| `tariboy retention get` | Show the effective retention policy for an agent (or 'default') |
| `tariboy retention set` | Set the retention policy for an agent (or 'default') |
| `tariboy rule ls` | List proxy policy rules (evaluation order) |
| `tariboy rule rm` | Remove a proxy policy rule by id |
| `tariboy rule set` | Set a proxy policy rule (kind rate-limit\|model-policy, scope global\|agent:\<n\>\|group:\<g\>) |
| `tariboy schedule ls` | List an agent's schedules (read-only) |
| `tariboy secret ls` | List secret keys (values are never shown) |
| `tariboy secret rm` | Remove a secret |
| `tariboy secret set` | Set a secret; value from --value or stdin |
| `tariboy usage` | Aggregate AI usage and cost from ai_requests |
| `tariboy user-prompt get` | Read the agent's standing user-prompt |
| `tariboy user-prompt set` | Set the agent's standing user-prompt |
| `tariboy version` | Print the Tariboy version locally without a daemon |

> Regenerate after adding/removing a command: `make build && ./bin/tariboy --help-json`.

## Agent tools (`tools …`)

Run inside an agent; the socket comes from `$TARIBOY_TOOLS_SOCKET`.

| Tool | Purpose |
| --- | --- |
| `tools whoami` | Print agent, cwd and current iteration |
| `tools status` | Print the agent status |
| `tools loop done` | Signal this iteration is finished (i-am-done) |
| `tools context get` | Print the durable working memory (CONTEXT.md) |
| `tools context set <text>` | Overwrite the durable working memory |
| `tools message send --channel C [--type T] [--subject k=v,…] [--text … \| --data JSON]` | Publish a message to a channel |
| `tools channel subscribe C [--matcher JSON] [--type globs]` | Subscribe to a channel |
| `tools channel unsubscribe ID` | Remove a subscription |
| `tools channel ls` | List your subscriptions |
| `tools sources` | List available channels |
| `tools schedule add --kind cron\|oneshot --spec S [--channel C] [--message JSON]` | Schedule a future wake-up |
| `tools schedule ls` | List your schedules |
| `tools schedule cancel ID` | Cancel a schedule |
| `tools script ls` | List your scripts |
| `tools script run NAME [--description TEXT] -- COMMAND` | Queue exactly one local run |
| `tools script schedule NAME --every SECONDS [--quiet-exit CODE] -- COMMAND` | Run now and repeat after each completion |
| `tools script runs SCRIPT_ID` / `logs RUN_ID` | Inspect run history and bounded logs |
| `tools script rerun SCRIPT_ID` | Rerun a completed one-shot definition |
| `tools script cancel SCRIPT_OR_RUN_ID` / `rm SCRIPT_ID` | Cancel work or remove inactive history |
| `tools image build --name NAME [--tag TAG] --path DIR` | Build a schema-v1 or schema-v2 image from an agent-confined source directory (`image-creator` only) |
| `tools help` | Show tool help |

## Native Tasks (`tasks …`)

An image enables this command with `plugins: [{name: tasks}]`. All mutations
derive the agent identity from its socket.

| Command | Purpose |
| --- | --- |
| `tasks mine` | List tasks visible/assigned to this agent |
| `tasks ready [--queue Q] [--claim]` | List ready work or atomically claim one |
| `tasks show KEY` | Show task detail, comments, waits, and relations |
| `tasks create --queue Q --title T [--assignee A]` | Create a queue-root task; in a queue the agent does not run, an omitted assignee files it for triage while an explicit assignee owns only that task tree |
| `tasks create --parent KEY --title T` | Create a child inheriting queue/context |
| `tasks update KEY [--title T] [--description D] [--status S]` | Update fields with optimistic revision |
| `tasks assign KEY ASSIGNEE` | Hand work to a known or arbitrary agent name |
| `tasks comment KEY TEXT` | Add a task comment |
| `tasks ask KEY agent:name\|user:login TEXT` | Mention a principal and record an open answer wait |
| `tasks move KEY [--parent KEY] [--before KEY] [--to-root]` | Reparent/reorder in the same queue, or detach into a root with `--to-root` |
| `tasks block KEY BLOCKER` | Add a directed, cycle-checked blocking relation |
| `tasks relate KEY OTHER` | Add a symmetric related link |
| `tasks done KEY [--complete-anyway]` | Complete, optionally overriding active descendants |

Workflow-managed queues add an assignment-scoped surface:

| Command | Purpose |
| --- | --- |
| `tasks work next [--queue Q] --idempotency-key K` | Atomically claim eligible work and return its least-context packet |
| `tasks work show ASSIGNMENT` | Refresh the current packet and revisions |
| `tasks work complete ASSIGNMENT --outcome O ...` | Submit one declared outcome |
| `tasks work release ASSIGNMENT ...` | Release a leased attempt |
| `tasks artifacts add ASSIGNMENT --name N --type T ...` | Attach a required typed output |
| `tasks artifacts show ASSIGNMENT ARTIFACT --task KEY` | Read one packet-visible artifact |
| `tasks ask ASSIGNMENT --question Q --context C --blocking-scope S ...` | Ask a universal workflow question and optionally hold work |
| `tasks questions ASSIGNMENT` | List questions visible in the packet |
| `tasks answer QUESTION --assignment ASSIGNMENT --answer TEXT ...` | Answer a routed question assignment |
| `tasks observe subscribe ASSIGNMENT PATTERN ...` | Create a policy-bounded observation subscription |
| `tasks observe list ASSIGNMENT` | List its workflow subscriptions |
| `tasks observe cancel ASSIGNMENT SUBSCRIPTION ...` | Cancel one subscription |

Mutations represented by `...` require current task/assignment revisions and a
stable idempotency key. Exact semantics and operator REST routes are in
[Configurable task workflows](/docs/task-workflows).
