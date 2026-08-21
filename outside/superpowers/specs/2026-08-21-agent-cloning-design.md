# Agent Cloning Design

## Summary

Tariboy will add a **Clone** action to every agent row in the Agents sidebar.
The action opens the existing agent-creation dialog in clone mode, loads the
source agent from its explicit host, and prefills every editable agent-row
configuration field. The clone keeps the source configuration but requires a
new name; its target host, image, and every copied field remain editable before
submission.

The create API will accept the complete persisted agent configuration in one
request. `loop.Manager.Run` will validate the request, construct one complete
`agent.Agent`, and persist those values in the existing single-row insert. The
UI will not create a partial agent and then patch its configuration through a
sequence of endpoints.

## Goals

- Put a **Clone** item in a native, accessible context menu opened by
  right-clicking an agent in either the **Teams** or **Individual agents**
  section.
- Reuse one create dialog for ordinary creation and cloning.
- Prefill every persisted, user-controlled field on the agent row.
- Keep every prefilled value editable, except values constrained by the image
  contract, such as schema-v2 plugin capabilities and bare-image runtime mode.
- Preserve the current explicit-host boundary for source reads, image loading,
  creation, and start requests.
- Persist the complete configuration in the create request and one agent-row
  insert.
- Preserve the existing create-then-start recovery behavior: a clone can be
  created successfully even when its requested immediate start fails, and the
  dialog can retry that start.

## Non-goals and clone boundary

The clone copies configuration, not runtime or historical resources.

Included:

- active image ref;
- configured CWD (including the distinction between an empty configured CWD
  and the source agent's effective managed workdir);
- harness, model, effort, interactive mode;
- loop enabled state, interval, soft timeout, hard timeout, timeout policy,
  error policy, and maximum idle iterations;
- standing user prompt;
- environment;
- persisted plugin list, subject to image-schema ownership rules;
- message batch and maximum queue limits;
- group, alias, notes, and color;
- master enabled state, represented by **Start now**.

Excluded:

- the source name, because agent names are unique; the clone name starts blank;
- image digest and pending image ref/digest/error; the target daemon resolves
  the selected active image ref and its current immutable digest;
- `created_at`, computed live state, current iteration, iteration history,
  status message, status timestamp, error/halt evidence, and idle-reset row id;
- secret values, because Tariboy deliberately exposes only secret keys and
  cannot safely prefill values;
- subscriptions, queued/delivered messages, scripts, context files, workdir or
  CWD contents, audit/history files, retention policy, eval configuration,
  budgets, and proxy rules; these are separate resources rather than fields of
  the create form.

This boundary prevents cloning from replaying work, copying sensitive values,
or silently duplicating resources with independent lifecycle and security
rules.

## Current limitations

`CreateAgentDialog` currently submits only image, name, CWD, runtime selection,
interactive mode, loop intent, environment, and the separate **Start now**
choice. The command layer's `registry.RunSpec` additionally carries plugins,
group, and a soft timeout, but `loop.Manager.Run` still initializes most loop
and metadata fields to defaults.

`GET /api/agents/{name}` exposes most configuration, but its `cwd` value is the
effective CWD. When the stored CWD is empty, that value is the source agent's
managed workdir and must not be copied to another agent. The response also
omits `messages_batch` and `messages_max_queue`. A clone therefore needs an
additive raw-CWD projection and the two missing message limits.

## User experience

### Opening the clone dialog

Each agent row becomes a Radix context-menu trigger without changing its
existing left-click navigation or Workspace pointer-drag behavior. The menu
contains one item, **Clone**. Radix supplies keyboard context-menu behavior and
focus management in addition to pointer right-click.

Selecting **Clone** records the source identity as `{hostId, agentName}` and
opens `CreateAgentDialog` in clone mode. The dialog resolves that exact host and
loads `GET /api/agents/{name}` from it. It shows a loading state until both the
source configuration and the source host's image list/manifest are current.
The create button remains disabled during that load. A source-read failure is
shown inside the dialog with a retry action; no request falls back to the local
daemon.

Clone mode requires the additive `configured_cwd`, `messages_batch`, and
`messages_max_queue` fields. If an older source daemon omits any of them, the
dialog reports that the host must be updated before a complete clone can be
made. It never falls back to effective `cwd` or guessed message defaults,
because either would violate the complete-copy guarantee.

The title is **Clone agent** and the description names the source agent and
host. Ordinary creation retains **New agent** and its existing description.

### Prefilled fields

The dialog is organized into focused sections so the complete configuration
remains usable in its scrollable Desktop bounds:

1. **Target**: host and image.
2. **Identity**: blank name, alias, group, color, and notes.
3. **Runtime**: harness, model, effort, configured CWD, interactive mode,
   environment JSON, and plugins.
4. **Autopilot**: loop enabled, interval seconds, soft timeout seconds, hard
   timeout seconds, timeout/error policies, maximum idle iterations, standing
   user prompt, message batch size, and maximum queued messages.
5. **Lifecycle**: **Start now**.

Ordinary creation uses the same sections with current defaults:

- name, alias, group, color, notes, CWD, model, effort, prompt, environment,
  and plugins start empty;
- interactive starts false, loop enabled starts true, and **Start now** starts
  true;
- interval, soft timeout, hard timeout, and maximum idle iterations start at
  zero;
- timeout and error policies start at `restart`;
- message batch starts at 10 and maximum queue starts at 1000.

Clone mode applies the source configuration exactly after the source read:

| Source projection | Dialog field | Create request |
| --- | --- | --- |
| `image` | image | `image` |
| no copied value | blank name | `name` |
| `configured_cwd` | CWD | `cwd` |
| `harness` | harness | `harness` |
| `model` | model | `model` |
| `effort` | effort | `effort` |
| `interactive` | Interactive | `interactive` |
| `loop_enabled` | Autopilot | `loop` |
| `enabled` | Start now | separate start request after create |
| `interval_s` | interval seconds | `interval_s` |
| `timeout_s` | soft timeout seconds | `timeout_s` |
| `hard_timeout_s` | hard timeout seconds | `hard_timeout_s` |
| `on_timeout` | timeout policy | `on_timeout` |
| `on_error` | error policy | `on_error` |
| `max_idle_iterations` | maximum idle iterations | `max_idle_iterations` |
| `user_prompt` | standing user prompt | `user_prompt` |
| `env` | environment entries | `env` |
| `plugins` | plugin entries | `plugins` |
| `messages_batch` | message batch | `messages_batch` |
| `messages_max_queue` | maximum queue | `messages_max_queue` |
| `group` | group | `group` |
| `alias` | alias | `alias` |
| `notes` | notes | `notes` |
| `color` | color | `color` |

The source's `enabled` value defaults **Start now**. The create endpoint still
creates a stopped row; after creation the existing explicit start request sets
the master enabled state. This keeps the current retryable failure boundary and
avoids treating process startup as part of the configuration insert.

### Host and image changes

The source host is the initial target, but the operator may select another
host. Host changes reload images from the new explicit target while retaining
the current draft. Submission stays disabled until the selected image ref is
present and its manifest is loaded from that target. An absolute configured
CWD or plugin selection that is invalid on the new target remains visible and
produces the target daemon's existing actionable validation error; it is never
silently rewritten. A valid named group retains the existing create behavior:
the target daemon ensures that group and reconciles its membership.

Changing the selected image does not reset the other cloned fields. A bare
image still enforces Interactive on and Autopilot off. For a schema-v2 image,
plugins are image-owned: the dialog displays the resolved list read-only and
the daemon derives the persisted list from the selected image. For schema v1,
the plugin override remains editable and is sent as configured. This prevents
the form from presenting an override the runtime contract would ignore.

### Success and failure

Configuration validation errors leave the complete draft in place. A
successful create immediately refreshes the selected host's agent list and
navigates to the new agent as today. When **Start now** is off, the dialog
closes after creation. When it is on, the existing start call runs; a start
failure leaves the successfully created agent visible and offers **Retry
start** without resubmitting the create request.

## API and persistence design

### Read projection

`agentView` remains backward compatible and adds:

- `configured_cwd`: the raw `agents.cwd` value;
- `messages_batch`;
- `messages_max_queue`.

The existing `cwd` key remains the effective CWD used by current consumers.
The TypeScript `AgentView` type gains the three fields. Clone initialization
uses `configured_cwd`, never `cwd`.

### Create contract

`POST /api/agents` and `registry.RunSpec` gain optional values for:

- `interval_s`, `timeout_s`, `hard_timeout_s`, and
  `max_idle_iterations`;
- `on_timeout` and `on_error`;
- `user_prompt`;
- `messages_batch` and `messages_max_queue`;
- `alias`, `notes`, and `color`.

Existing image, name, CWD, harness, model, effort, interactive, loop,
environment, plugins, and group inputs remain compatible. The current textual
`timeout` CLI flag remains accepted; when both representations are present,
the request is rejected as ambiguous rather than choosing one silently. The
Desktop uses the exact integer-second fields.

For exact HTTP round-tripping, `env` accepts either the existing serialized
`K=V,K=V` string or a JSON object whose values are strings, and `plugins`
accepts either the existing comma-separated string or a JSON string array.
Their OpenAPI schemas describe both representations. The operator CLI retains
its scalar flags. The Desktop clone path uses the structured forms, so commas,
equals signs, whitespace, and newlines inside environment values are preserved
instead of being reparsed through the legacy comma encoding. The form displays
environment as an editable JSON object and rejects malformed or non-string
values before submission.

Before calling `Control.Run`, `agent.run` validates:

- all second counts and `max_idle_iterations` are non-negative;
- `messages_batch` and `messages_max_queue` are positive;
- timeout and error policy are `restart` or `stop`;
- color is empty or a six-digit `#rrggbb` value;
- CWD resolution and existence through the existing target-host rules;
- existing image, name, group, harness, and plugin invariants through their
  current owners.

Validation errors use stable user-error codes and happen before provisioning
or persistence.

### Manager and store

`loop.Manager.Run` builds the full `agent.Agent` before provisioning. Defaults
are applied only to omitted create values; explicit clone values win. Bare and
schema-v2 image constraints remain authoritative. Schema-v2 plugins are
resolved from the image, while schema-v1 plugin overrides retain existing
resolution and validation.

`agent.Store.Create` already inserts most agent configuration in one SQL
statement. Its insert will add the currently omitted
`max_idle_iterations` value and continue to include the new request's alias,
notes, color, message limits, prompt, policies, environment, plugins, and
group in the same row write. No database migration is required because all
columns already exist.

No post-create configuration endpoint is called by the UI. If request
validation, image/plugin resolution, or provisioning fails, no agent row is
written. Once the row is written, existing inbox subscription and group
reconciliation behavior remains unchanged.

## Components and files

- `internal/registry/registry.go`: expand `RunSpec`.
- `internal/commands/agents.go`: extend the create args, validation, read
  projection, and request mapping.
- `internal/loop/manager.go`: apply the complete spec to the new `agent.Agent`.
- `internal/agent/agent.go`: include `max_idle_iterations` in the create insert.
- `ui/src/lib/api.ts` and `ui/src/lib/types.ts`: type the complete create and
  inspect contracts, including structured environment and plugin inputs.
- `ui/src/pages/terminals/CreateAgentDialog.tsx`: share the full form between
  ordinary create and clone modes.
- `ui/src/pages/terminals/TerminalsPage.tsx`: own clone-dialog source identity
  and explicit-host configuration loading.
- `ui/src/pages/terminals/TerminalsSidebar.tsx`: expose the agent context-menu
  action without changing navigation or drag behavior.
- `ui/src/components/ui/context-menu.tsx`, `ui/package.json`, and
  `ui/package-lock.json`: add the repository-local Radix context-menu wrapper
  and pinned dependency.
- `docs/docs/architecture/web-ui.mdx`: document complete creation and cloning
  behavior.

## Testing and verification

Backend tests will prove:

- `agent.run` maps every new HTTP field into `RunSpec` and preserves old
  defaults;
- invalid numbers, policies, color, and ambiguous timeout inputs fail before
  `Control.Run`;
- `agent.inspect` reports effective and configured CWD separately plus both
  message limits;
- `loop.Manager.Run` persists every requested configuration field, including
  zero values and `max_idle_iterations`, in one created row;
- schema-v2 and bare-image constraints remain authoritative.

Frontend tests will prove:

- ordinary creation keeps its current defaults while submitting the complete
  contract;
- clone mode loads from the source's explicit host, leaves name blank, and
  prefills every included field;
- raw empty CWD remains empty rather than becoming the source workdir;
- changing target host or image retains the clone draft and fails closed while
  the target image is unresolved;
- schema-v2 plugins are read-only, schema-v1 plugins remain editable, and bare
  images enforce their runtime settings;
- right-click **Clone** works for grouped and individual rows without changing
  ordinary click navigation or Workspace drag behavior;
- source-load, create, and start failures retain the appropriate retry state.

The production Desktop Playwright scenario will create a source agent through
the isolated daemon, right-click it, verify representative values from every
form section, submit a clone, and inspect the clone through the daemon API. It
will run through `tauri-driver`, never against the live listener or live data
directories.

Final verification is:

```bash
make check
make full-check
git diff --check
```

The complete diff will be reviewed before integration. Generated
`desktop/dist` and `desktop/src-tauri/resources/bin/` remain ignored and will
not be staged. This ordinary feature task does not bump the product version.
