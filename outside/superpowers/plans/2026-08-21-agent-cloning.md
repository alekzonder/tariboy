# Agent Cloning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an accessible right-click **Clone** action that opens the complete agent-creation dialog with every persisted agent configuration field copied and editable, then creates the clone atomically on an explicit target host.

**Architecture:** Extend `agent.run` and `registry.RunSpec` so one create request carries the complete agent-row configuration, with HTTP structured environment/plugin inputs and backward-compatible CLI scalar inputs. Build and persist one complete `agent.Agent`, expose the raw configured CWD and message limits on inspect, and make `CreateAgentDialog` own ordinary and clone drafts while `TerminalsPage` owns the source `{hostId, agentName}` and the sidebar owns only the context-menu gesture.

**Tech Stack:** Go 1.26, SQLite, React 19, TypeScript 6, Radix UI through the existing `radix-ui` umbrella, Vitest/Testing Library, Playwright, Tauri/WebKit through `tauri-driver`, Starlight MDX.

**Spec:** `outside/superpowers/specs/2026-08-21-agent-cloning-design.md`

## Global Constraints

- Clone configuration only; do not copy history, iteration/runtime evidence, secrets, subscriptions, messages, scripts, files, workdir contents, retention/evals, budgets, or proxy rules.
- Read the source and all image data from the source/target host explicitly; never fall back from an unresolved remote host to the local daemon.
- Preserve the raw empty configured CWD through `configured_cwd`; never clone the inspect response's effective `cwd` fallback.
- A create request persists one complete row; the UI must not patch configuration after creation.
- A created row remains stopped. **Start now** continues to use the separate retryable start request and defaults from source `enabled` in clone mode.
- Bare images force Interactive on and Autopilot off. Schema-v2 images own their plugin list; schema-v1 plugin overrides remain editable.
- Keep legacy CLI `--env`, `--plugins`, and `--timeout` forms compatible. Reject simultaneous `timeout` and `timeout_s`.
- Do not expose or prefill secret values.
- Do not bump the product version.
- Do not stage ignored `desktop/dist` or `desktop/src-tauri/resources/bin/` output.
- Every test daemon must use isolated base/runtime directories and must not use `127.0.0.1:9990`.

---

### Task 1: Complete HTTP create and inspect contracts

**Files:**
- Modify: `internal/registry/registry.go`
- Modify: `internal/commands/agents.go`
- Modify: `internal/commands/agents_test.go`
- Modify: `internal/api/server_test.go`

**Interfaces:**
- Consumes: existing `registry.Arg.Schema`, `registry.Params`, `api.UserError`, `parseAgentTimeout`, and `agentView`.
- Produces: `registry.RunSpec` fields `IntervalS`, `TimeoutS`, `HardTimeoutS`, `OnTimeout`, `OnError`, `MaxIdleIterations`, `UserPrompt`, `MessagesBatch`, `MessagesMaxQueue`, `Alias`, `Notes`, and `Color`; inspect keys `configured_cwd`, `messages_batch`, and `messages_max_queue`; structured HTTP parsing helpers for environment and plugins.

- [ ] **Step 1: Add failing command tests for the full structured request and inspect projection**

Add table-driven assertions that exercise JSON-decoded shapes, raw/effective CWD separation, message limits, and stable validation codes:

```go
import "reflect"

func TestAgentRunMapsCompleteConfiguration(t *testing.T) {
	c, _, fc := ctxWithStore(t)
	_, err := h(t, "agent.run")(c, registry.Params{
		"image": "basic:latest", "name": "clone", "cwd": "/srv/clone",
		"harness": "codex", "model": "gpt-5", "effort": "high",
		"interactive": true, "loop": false,
		"interval_s": float64(12), "timeout_s": float64(34), "hard_timeout_s": float64(56),
		"on_timeout": "stop", "on_error": "restart", "max_idle_iterations": float64(7),
		"user_prompt": "keep commas, equals=a=b, and\nnewlines",
		"env": map[string]any{"CSV": "a,b", "EQ": "a=b", "LINES": "one\ntwo"},
		"plugins": []any{"context", "custom"},
		"messages_batch": float64(8), "messages_max_queue": float64(900),
		"group": "reviewers", "alias": "Clone", "notes": "all fields", "color": "#123abc",
	})
	if err != nil { t.Fatal(err) }
	want := registry.RunSpec{
		ImageRef: "basic:latest", Name: "clone", Cwd: "/srv/clone",
		Harness: "codex", Model: "gpt-5", Effort: "high", Interactive: true,
		Env: map[string]string{"CSV": "a,b", "EQ": "a=b", "LINES": "one\ntwo"},
		Plugins: []string{"context", "custom"}, Loop: false,
		IntervalS: 12, TimeoutS: 34, HardTimeoutS: 56,
		OnTimeout: "stop", OnError: "restart", MaxIdleIterations: 7,
		UserPrompt: "keep commas, equals=a=b, and\nnewlines",
		MessagesBatch: 8, MessagesMaxQueue: 900,
		Group: "reviewers", Alias: "Clone", Notes: "all fields", Color: "#123abc",
	}
	if !reflect.DeepEqual(want, fc.ran) { t.Fatalf("RunSpec mismatch:\nwant: %#v\n got: %#v", want, fc.ran) }
}

func TestAgentInspectReportsCloneProjection(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "source", MessagesBatch: 17, MessagesMaxQueue: 1234}); err != nil { t.Fatal(err) }
	v, err := h(t, "agent.inspect")(c, registry.Params{"name": "source"})
	if err != nil { t.Fatal(err) }
	got := v.(map[string]any)
	if got["configured_cwd"] != "" || got["cwd"] == "" || got["messages_batch"] != 17 || got["messages_max_queue"] != 1234 {
		t.Fatalf("inspect clone projection = %#v", got)
	}
}
```

Cover these invalid inputs in a table and assert `fakeControl.Run` is not called: negative second/count fields, zero message limits, non-integral JSON numbers, policies outside `restart|stop`, malformed color, non-string environment values, non-string plugin array entries, and both `timeout` plus `timeout_s`. Assert codes `bad_interval`, `bad_timeout`, `bad_hard_timeout`, `bad_max_idle`, `bad_messages_batch`, `bad_messages_max_queue`, `bad_on_timeout`, `bad_on_error`, `bad_color`, `bad_env`, `bad_plugins`, and `ambiguous_timeout` respectively.

- [ ] **Step 2: Run focused command tests and observe the expected failures**

Run:

```bash
go test ./internal/commands -run 'TestAgentRunMapsCompleteConfiguration|TestAgentRunRejectsCompleteConfiguration|TestAgentInspectReportsCloneProjection' -count=1
```

Expected: FAIL because `RunSpec` and inspect do not expose the complete fields and `agent.run` still parses only scalar env/plugins and textual timeout.

- [ ] **Step 3: Extend `RunSpec`, parsers, validation, and inspect projection**

Extend the registry contract:

```go
type RunSpec struct {
	ImageRef, Name, Cwd, Harness, Model, Effort string
	Interactive bool
	Env map[string]string
	Plugins []string
	Loop bool
	IntervalS, TimeoutS, HardTimeoutS int
	OnTimeout, OnError string
	MaxIdleIterations int
	UserPrompt string
	MessagesBatch, MessagesMaxQueue int
	Group, Alias, Notes, Color string
}
```

Add parsers that preserve legacy scalar forms and accept structured HTTP values without lossy comma reparsing:

```go
func parseAgentEnv(value any) (map[string]string, error) {
	switch value := value.(type) {
	case nil:
		return map[string]string{}, nil
	case string:
		return parseKV(value), nil
	case map[string]any:
		out := make(map[string]string, len(value))
		for key, raw := range value {
			text, ok := raw.(string)
			if !ok { return nil, fmt.Errorf("env value for %q must be a string", key) }
			out[key] = text
		}
		return out, nil
	default:
		return nil, fmt.Errorf("env must be a K=V string or string-valued object")
	}
}

func parseAgentPlugins(value any) ([]string, error) {
	if text, ok := value.(string); ok { return parseList(text), nil }
	plugins, ok := stringSlice(value)
	if !ok { return nil, fmt.Errorf("plugins must be a comma list or string array") }
	return plugins, nil
}
```

Give `env` and `plugins` `registry.Arg.Schema` values with OpenAPI `oneOf` branches for the legacy string and structured object/array. Add integer and string args for the remaining fields. Detect key presence before assigning defaults so `timeout` plus `timeout_s` is rejected explicitly. Accept only integral JSON numbers, validate all numeric bounds before CWD resolution or `Control.Run`, normalize color to lowercase after matching `^#[0-9a-fA-F]{6}$`, and return the stable `api.UserError` codes named in Step 1.

Add the clone projection before `agentInspect` replaces `cwd` with the effective path:

```go
"cwd": a.Cwd,
"configured_cwd": a.Cwd,
"messages_batch": a.MessagesBatch,
"messages_max_queue": a.MessagesMaxQueue,
```

- [ ] **Step 4: Add a failing OpenAPI contract assertion for union inputs and integer fields**

Add `TestAgentRunOpenAPIContract` to `internal/api/server_test.go`. Build the normal registry-backed test server, fetch `/api/openapi.json`, inspect `POST /api/agents`, and assert:

```go
body := operation["requestBody"].(map[string]any)
schema := body["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
properties := schema["properties"].(map[string]any)
if len(properties["env"].(map[string]any)["oneOf"].([]any)) != 2 { t.Fatal("env union missing") }
if len(properties["plugins"].(map[string]any)["oneOf"].([]any)) != 2 { t.Fatal("plugins union missing") }
if properties["messages_batch"].(map[string]any)["type"] != "integer" { t.Fatal("message limit is not integer") }
```

- [ ] **Step 5: Run the focused command and OpenAPI tests and observe passing results**

Run:

```bash
go test ./internal/commands ./internal/api -run 'TestAgentRun|TestAgentInspect|TestHelpAndOpenAPI|TestAgentRunOpenAPI' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the complete transport contract**

```bash
git add internal/registry/registry.go internal/commands/agents.go internal/commands/agents_test.go internal/api/server_test.go
git commit -m "feat: complete agent creation contract"
```

---

### Task 2: Persist one complete agent row atomically

**Files:**
- Modify: `internal/loop/manager.go`
- Modify: `internal/loop/manager_test.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: the expanded `registry.RunSpec` from Task 1, existing image/plugin resolution, bare-image constraints, `agent.Store.Create`, and `agent.Agent` fields.
- Produces: a fully populated `agent.Agent` before provisioning and one insert that includes `max_idle_iterations`.

- [ ] **Step 1: Add a failing manager persistence test for every requested field**

```go
import "reflect"

func TestRunPersistsCompleteConfiguration(t *testing.T) {
	m, store, _, _ := newManager(t, &fakeRunner{})
	_, err := m.Run(registry.RunSpec{
		ImageRef: "basic:latest", Name: "clone", Cwd: "/srv/clone",
		Harness: "codex", Model: "gpt-5", Effort: "high", Interactive: true, Loop: false,
		IntervalS: 12, TimeoutS: 34, HardTimeoutS: 56,
		OnTimeout: "stop", OnError: "restart", MaxIdleIterations: 7,
		UserPrompt: "standing prompt", Env: map[string]string{"CSV": "a,b"},
		Plugins: []string{"context"}, MessagesBatch: 8, MessagesMaxQueue: 900,
		Alias: "Clone", Notes: "all fields", Color: "#123abc",
	})
	if err != nil { t.Fatal(err) }
	got, err := store.Get("clone")
	if err != nil { t.Fatal(err) }
	want := agent.Agent{
		Name: "clone", ImageRef: got.ImageRef, ImageDigest: got.ImageDigest,
		Cwd: "/srv/clone", HarnessType: "codex", Model: "gpt-5", Effort: "high",
		Interactive: true, LoopEnabled: false, Enabled: false,
		IntervalS: 12, TimeoutS: 34, HardTimeoutS: 56,
		OnTimeout: "stop", OnError: "restart", MaxIdleIterations: 7,
		UserPrompt: "standing prompt", Env: map[string]string{"CSV": "a,b"},
		Plugins: []string{"context"}, MessagesBatch: 8, MessagesMaxQueue: 900,
		Alias: "Clone", Notes: "all fields", Color: "#123abc",
	}
	if got.Cwd != want.Cwd || got.HarnessType != want.HarnessType || got.Model != want.Model ||
		got.Effort != want.Effort || got.Interactive != want.Interactive ||
		got.LoopEnabled != want.LoopEnabled || got.Enabled != want.Enabled ||
		got.IntervalS != want.IntervalS || got.TimeoutS != want.TimeoutS ||
		got.HardTimeoutS != want.HardTimeoutS || got.OnTimeout != want.OnTimeout ||
		got.OnError != want.OnError || got.MaxIdleIterations != want.MaxIdleIterations ||
		got.UserPrompt != want.UserPrompt || !reflect.DeepEqual(got.Env, want.Env) ||
		!reflect.DeepEqual(got.Plugins, want.Plugins) ||
		got.MessagesBatch != want.MessagesBatch || got.MessagesMaxQueue != want.MessagesMaxQueue ||
		got.Alias != want.Alias || got.Notes != want.Notes || got.Color != want.Color {
		t.Fatalf("persisted agent mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}
```

Add `TestRunAppliesCreationDefaults` asserting omitted policies become `restart`, message limits become `10` and `1000`, and all second/count values stay zero. Extend a bare/schema-v2 test to prove the manager still forces Interactive/Autopilot and derives plugins from the image despite supplied overrides.

- [ ] **Step 2: Add a failing store test for `max_idle_iterations` on Create**

```go
func TestCreatePersistsMaximumIdleIterations(t *testing.T) {
	st := openStore(t)
	a := sampleAgent()
	a.MaxIdleIterations = 9
	if err := st.Create(a); err != nil { t.Fatal(err) }
	got, err := st.Get(a.Name)
	if err != nil { t.Fatal(err) }
	if got.MaxIdleIterations != 9 { t.Fatalf("MaxIdleIterations = %d, want 9", got.MaxIdleIterations) }
}
```

- [ ] **Step 3: Run manager/store tests and observe the expected failures**

Run:

```bash
go test ./internal/loop ./internal/agent -run 'TestRunPersistsCompleteConfiguration|TestRunAppliesCreationDefaults|TestCreatePersistsMaximumIdleIterations' -count=1
```

Expected: FAIL because `Manager.Run` initializes only the current subset and `Store.Create` omits `max_idle_iterations`.

- [ ] **Step 4: Populate the complete `agent.Agent` before provisioning**

Build the row with explicit request values and creation defaults in one place:

```go
onTimeout := pick(spec.OnTimeout, "restart")
onError := pick(spec.OnError, "restart")
messagesBatch := spec.MessagesBatch
if messagesBatch == 0 { messagesBatch = 10 }
messagesMaxQueue := spec.MessagesMaxQueue
if messagesMaxQueue == 0 { messagesMaxQueue = 1000 }

ag := agent.Agent{
	Name: name, ImageRef: ref.String(), ImageDigest: man.Digest,
	Cwd: spec.Cwd, HarnessType: harnessType, Model: model, Effort: effort,
	Interactive: spec.Interactive, LoopEnabled: spec.Loop, Enabled: false,
	IntervalS: spec.IntervalS, TimeoutS: spec.TimeoutS, HardTimeoutS: spec.HardTimeoutS,
	OnTimeout: onTimeout, OnError: onError, MaxIdleIterations: spec.MaxIdleIterations,
	UserPrompt: spec.UserPrompt, Env: env, Plugins: resolvedPlugins,
	MessagesBatch: messagesBatch, MessagesMaxQueue: messagesMaxQueue,
	Alias: spec.Alias, Notes: spec.Notes, Color: spec.Color,
}
```

Keep image/plugin resolution before provisioning, keep `Enabled: false`, apply bare constraints after construction, and apply validated group membership to `ag.Group` before `agentdir.Provision` and `Store.Create`.

- [ ] **Step 5: Include `max_idle_iterations` in the single insert**

Extend the existing `INSERT INTO agents` column list, placeholder list, and arguments:

```go
..., messages_batch, messages_max_queue, "group", alias, notes, color, max_idle_iterations)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
```

and append `a.MaxIdleIterations` to the matching argument list. Do not add a migration; the column already exists.

- [ ] **Step 6: Run focused and package-level backend tests**

Run:

```bash
go test ./internal/agent ./internal/commands ./internal/loop ./internal/api -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit atomic complete-row creation**

```bash
git add internal/loop/manager.go internal/loop/manager_test.go internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat: persist complete agent configuration on create"
```

---

### Task 3: Type and initialize complete ordinary/clone drafts

**Files:**
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/lib/types.ts`
- Create: `ui/src/pages/terminals/agentCreateDraft.ts`
- Create: `ui/src/pages/terminals/agentCreateDraft.test.ts`
- Modify: `ui/src/pages/terminals/CreateAgentDialog.tsx`
- Modify: `ui/src/pages/terminals/CreateAgentDialog.test.tsx`

**Interfaces:**
- Consumes: `AgentView`, `ImageManifest`, `agentGetOn`, explicit `ApiTarget`, the complete create API from Tasks 1-2, and current start retry behavior.
- Produces: `AgentCreateDraft`, `newAgentDraft()`, `cloneAgentDraft(source)`, `CreateAgentDialogProps.cloneSource?: {hostId: string; agentName: string; hostLabel: string}`, and a complete structured `CreateAgentSpec`.

- [ ] **Step 1: Add failing pure draft-mapping tests**

Define a source fixture with distinctive values for every included field, including empty `configured_cwd`, embedded commas/equals/newlines in env, and both schema-v1 plugin names. Assert exact mapping and ordinary defaults:

```ts
expect(newAgentDraft()).toEqual({
  image: "", name: "", cwd: "", harness: "", model: "", effort: "",
  interactive: false, loop: true, startNow: true,
  intervalS: "0", timeoutS: "0", hardTimeoutS: "0",
  onTimeout: "restart", onError: "restart", maxIdleIterations: "0",
  userPrompt: "", envText: "{}", plugins: [],
  messagesBatch: "10", messagesMaxQueue: "1000",
  group: "", alias: "", notes: "", color: "",
})

expect(cloneAgentDraft(source)).toMatchObject({
  image: "worker:v1", name: "", cwd: "", harness: "codex", model: "gpt-5",
  effort: "high", interactive: true, loop: false, startNow: false,
  intervalS: "12", timeoutS: "34", hardTimeoutS: "56",
  onTimeout: "stop", onError: "restart", maxIdleIterations: "7",
  userPrompt: "standing", envText: JSON.stringify(source.env, null, 2),
  plugins: ["context", "custom"], messagesBatch: "8", messagesMaxQueue: "900",
  group: "reviewers", alias: "Clone", notes: "all fields", color: "#123abc",
})
```

Also assert that missing `configured_cwd`, `messages_batch`, or `messages_max_queue` throws the upgrade-required error and never uses `source.cwd`.

- [ ] **Step 2: Run the draft test and observe the expected failure**

Run:

```bash
cd ui && npm test -- src/pages/terminals/agentCreateDraft.test.ts
```

Expected: FAIL because the module and additive `AgentView` fields do not exist.

- [ ] **Step 3: Extend TypeScript API types and implement pure draft mapping**

Add the inspect fields:

```ts
export interface AgentView {
  configured_cwd?: string;
  messages_batch?: number;
  messages_max_queue?: number;
  // existing fields remain unchanged
}
```

Keep them optional in TypeScript for old-daemon detection while current daemons always emit them. Extend create types with structured values and exact integer-second fields:

```ts
export interface CreateAgentSpec {
  image: string;
  name?: string;
  cwd?: string;
  harness?: string;
  model?: string;
  effort?: string;
  interactive?: boolean;
  loop?: boolean;
  env?: string | Record<string, string>;
  plugins?: string | string[];
  timeout?: string;
  interval_s?: number;
  timeout_s?: number;
  hard_timeout_s?: number;
  on_timeout?: "restart" | "stop";
  on_error?: "restart" | "stop";
  max_idle_iterations?: number;
  user_prompt?: string;
  messages_batch?: number;
  messages_max_queue?: number;
  group?: string;
  alias?: string;
  notes?: string;
  color?: string;
}
```

Implement the draft module with no network or React dependencies. `cloneAgentDraft` must require the three complete-clone projection fields, use `configured_cwd`, normalize nullable group to `""`, and leave name blank.

- [ ] **Step 4: Add failing dialog tests for source loading, complete prefill, submission, and retry**

Mock `agentGetOn` in `CreateAgentDialog.test.tsx`. Render with:

```tsx
<CreateAgentDialog
  open
  hostId="d1"
  cloneSource={{ hostId: "d1", agentName: "source", hostLabel: "prod" }}
  hosts={hosts}
  onOpenChange={onOpenChange}
  onCreated={onCreated}
/>
```

Assert the source request is exactly `agentGetOn(targetFor("d1"), "source", "")`, the title is **Clone agent**, the description names `source` and `prod`, the name is blank, every section has representative copied values, raw CWD stays empty, and **Start now** matches `enabled`. Submit and assert one `createAgent` call contains every create field with `env` as an object and `plugins` as an array. Assert source failure shows a **Retry source** action, incomplete old-daemon projections show the update-required message, and neither path calls `createAgent`. Add clone-mode cases proving a target-host change and an image change retain identity/runtime/autopilot drafts while closing the create gate until the new target manifest resolves. Assert schema-v1 plugins remain editable, schema-v2 plugins are the manifest-derived read-only list, and bare images force the rendered Interactive/Autopilot values without erasing the underlying clone draft.

Retain and update the existing ordinary defaults matrix, host switching, bare-image, runtime preset, create failure, and retry-start tests.

- [ ] **Step 5: Run the dialog tests and observe the expected failures**

Run:

```bash
cd ui && npm test -- src/pages/terminals/CreateAgentDialog.test.tsx
```

Expected: FAIL because the dialog has no clone source or full draft fields.

- [ ] **Step 6: Refactor `CreateAgentDialog` around one complete draft**

Use a single draft state and a clone-load state:

```ts
const [draft, setDraft] = useState<AgentCreateDraft>(() => newAgentDraft(imageRef));
const [sourceState, setSourceState] = useState<"idle" | "loading" | "ready" | "error">(
  cloneSource ? "loading" : "idle",
);

const loadSource = useCallback(async () => {
  if (!cloneSource) return;
  setSourceState("loading");
  const sourceTarget = cloneSource.hostId ? await resolveDaemon(cloneSource.hostId) : null;
  if (cloneSource.hostId && !sourceTarget) throw new Error(`host ${cloneSource.hostId} was not found`);
  const source = await agentGetOn<AgentView>(sourceTarget, cloneSource.agentName, "");
  setDraft(cloneAgentDraft(source));
  setHost(cloneSource.hostId);
  setSourceState("ready");
}, [cloneSource]);
```

Keep target image/manifest loading separate from source loading. Host or image changes must update only target readiness and image constraints; they must not reset the other draft fields. Disable create while source, host, image list, or manifest is unresolved.

Render the scrollable sections **Target**, **Identity**, **Runtime**, **Autopilot**, and **Lifecycle**. Use text/number inputs with exact accessible labels, a textarea for notes and standing prompt, an environment JSON textarea, `restart|stop` selects, and the existing badge/input pattern for plugins. For schema v2 render manifest plugin names read-only; for schema v1 edit `draft.plugins`. For bare images render forced switches without erasing the stored draft values.

Parse and validate environment JSON before submission:

```ts
const parsedEnv: unknown = JSON.parse(draft.envText || "{}");
if (!parsedEnv || Array.isArray(parsedEnv) || typeof parsedEnv !== "object" ||
    Object.values(parsedEnv).some((value) => typeof value !== "string")) {
  throw new Error("Environment must be a JSON object whose values are strings");
}
```

Build one complete request using `Number(...)` for validated integer fields and omit schema-v2 `plugins` so the daemon derives them. Preserve current create success, `onCreated`, runtime-preset memory, separate start call, and retry-start behavior.

- [ ] **Step 7: Run all focused UI creation tests**

Run:

```bash
cd ui && npm test -- src/pages/terminals/agentCreateDraft.test.ts src/pages/terminals/CreateAgentDialog.test.tsx src/pages/AgentCreate.test.tsx src/components/GroupWizard.test.tsx
```

Expected: PASS, proving both the new structured dialog and legacy scalar create callers remain typed and compatible.

- [ ] **Step 8: Commit the complete shared dialog**

```bash
git add ui/src/lib/api.ts ui/src/lib/types.ts ui/src/pages/terminals/agentCreateDraft.ts ui/src/pages/terminals/agentCreateDraft.test.ts ui/src/pages/terminals/CreateAgentDialog.tsx ui/src/pages/terminals/CreateAgentDialog.test.tsx
git commit -m "feat: add complete agent create and clone dialog"
```

---

### Task 4: Add the sidebar Clone context menu and explicit-host orchestration

**Files:**
- Create: `ui/src/components/ui/context-menu.tsx`
- Modify: `ui/src/pages/terminals/TerminalsSidebar.tsx`
- Modify: `ui/src/pages/terminals/TerminalsPage.tsx`
- Modify: `ui/src/pages/terminals/TerminalsPage.test.tsx`
- Modify: `ui/tests/workspace-fixture.tsx`

**Interfaces:**
- Consumes: existing `radix-ui` dependency, sidebar agent identity, `CreateAgentDialogProps.cloneSource`, route navigation, and workspace pointer-drag callback.
- Produces: `TerminalsSidebar.onClone(hostId, agentName)`, accessible **Clone** menu items for grouped and individual agents, and `CreateDialogState.cloneSource` in `TerminalsPage`.

- [ ] **Step 1: Add failing sidebar/page interaction tests**

In `TerminalsPage.test.tsx`, right-click both a grouped and individual row:

```ts
fireEvent.contextMenu(screen.getByRole("button", { name: "Open lead" }));
fireEvent.click(await screen.findByRole("menuitem", { name: "Clone" }));
expect(await screen.findByRole("heading", { name: "Clone agent" })).toBeInTheDocument();
expect(agentGetOn).toHaveBeenCalledWith(targetFor(""), "lead", "");
```

Repeat for `solo`. Add assertions that ordinary left click still navigates to `/console`, a workspace pointer down still calls `beginExternalPointerDrag`, and the context-menu gesture does not navigate or begin a drag.

- [ ] **Step 2: Run the page test and observe the expected failure**

Run:

```bash
cd ui && npm test -- src/pages/terminals/TerminalsPage.test.tsx
```

Expected: FAIL because agent rows do not expose a context menu or clone callback.

- [ ] **Step 3: Add the repository-local Radix context-menu wrapper**

Mirror the existing dropdown-menu styling while importing the already-installed umbrella export:

```tsx
import * as React from "react";
import { ContextMenu as ContextMenuPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";

const ContextMenu = ContextMenuPrimitive.Root;
const ContextMenuTrigger = ContextMenuPrimitive.Trigger;

function ContextMenuContent({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.Content>) {
  return <ContextMenuPrimitive.Portal><ContextMenuPrimitive.Content
    data-slot="context-menu-content"
    className={cn("z-50 min-w-36 rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10", className)}
    {...props}
  /></ContextMenuPrimitive.Portal>;
}

function ContextMenuItem({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.Item>) {
  return <ContextMenuPrimitive.Item
    data-slot="context-menu-item"
    className={cn("relative flex cursor-default select-none items-center rounded-md px-1.5 py-1 text-sm outline-hidden focus:bg-accent", className)}
    {...props}
  />;
}

export { ContextMenu, ContextMenuTrigger, ContextMenuContent, ContextMenuItem };
```

Do not change `ui/package.json` or `ui/package-lock.json`; `radix-ui@^1.4.3` already supplies Context Menu and its lockfile entries.

- [ ] **Step 4: Wrap each agent row without changing its button gesture**

Add `onClone` to `TerminalsSidebar` and wrap the existing row:

```tsx
<ContextMenu key={a.name}>
  <ContextMenuTrigger asChild>
    <div className={cn(
      "flex w-full items-center rounded text-sm hover:bg-accent",
      selected && selected.hostId === h.host.id && selected.agent === a.name && "bg-accent",
    )}>
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center justify-between px-2 py-1 text-left"
        aria-label={`Open ${a.name}`}
        aria-current={selected?.hostId === h.host.id && selected.agent === a.name ? "page" : undefined}
        disabled={Boolean(h.error)}
        onPointerDown={(event) => {
          if (h.error || !workspaceMode || !interactive) return;
          onBeginWorkspaceDrag({ hostId: h.host.id, agentName: a.name }, event);
        }}
        onClick={() => { if (!h.error) onSelect(h.host.id, a.name); }}
      >
        <span className="flex min-w-0 items-center gap-1">
          <span className="truncate">{a.name}</span>
          {attention.has(JSON.stringify([h.host.id, a.name])) && (
            <span role="img" aria-label={`Unread customer question for ${a.name} on ${h.host.label}`}
              title={`Unread customer question for ${a.name} on ${h.host.label}`}
              className="h-2 w-2 shrink-0 rounded-full bg-red-500" />
          )}
          {!interactive && <span className="shrink-0 text-xs text-muted-foreground" title="not interactive (no tty)">non-tty</span>}
        </span>
        <Badge variant={a.state === "running" ? "default" : "secondary"}>{a.state}</Badge>
      </button>
    </div>
  </ContextMenuTrigger>
  <ContextMenuContent>
    <ContextMenuItem disabled={Boolean(h.error)} onSelect={() => onClone(h.host.id, a.name)}>
      Clone
    </ContextMenuItem>
  </ContextMenuContent>
</ContextMenu>
```

Keep the trigger identity outside section-specific rendering so grouped and individual agents share exactly one implementation. Update `ui/tests/workspace-fixture.tsx` with a no-op `onClone` prop.

- [ ] **Step 5: Store clone source identity in `TerminalsPage`**

Extend dialog state and wiring:

```ts
type CloneSource = { hostId: string; agentName: string; hostLabel: string };
type CreateDialogState = { hostId: string; imageRef?: string; cloneSource?: CloneSource };

onClone={(cloneHostId, cloneAgentName) => {
  const hostLabel = sidebarHosts.find((entry) => entry.host.id === cloneHostId)?.host.label
    ?? (cloneHostId === "" ? "This daemon (local)" : cloneHostId);
  setCreateFor({
    hostId: cloneHostId,
    cloneSource: { hostId: cloneHostId, agentName: cloneAgentName, hostLabel },
  });
}}
```

Pass `createFor?.cloneSource` to `CreateAgentDialog`. Keep `onCreated` refreshing the aggregate and navigating to the created agent on the dialog-selected target host.

- [ ] **Step 6: Run sidebar, page, workspace, and dialog tests**

Run:

```bash
cd ui && npm test -- src/pages/terminals/TerminalsPage.test.tsx src/pages/terminals/CreateAgentDialog.test.tsx
npm run test:workspace-browser
```

Expected: PASS. The browser workspace suite proves the new wrapper did not regress real pointer dragging.

- [ ] **Step 7: Commit the context-menu flow**

```bash
git add ui/src/components/ui/context-menu.tsx ui/src/pages/terminals/TerminalsSidebar.tsx ui/src/pages/terminals/TerminalsPage.tsx ui/src/pages/terminals/TerminalsPage.test.tsx ui/tests/workspace-fixture.tsx
git commit -m "feat: open agent cloning from sidebar context menu"
```

---

### Task 5: Prove the production Desktop flow and document the behavior

**Files:**
- Modify: `ui/tests/desktop/create-agent.pw.ts`
- Modify: `docs/docs/architecture/web-ui.mdx`

**Interfaces:**
- Consumes: the isolated Desktop fixture's real bundled daemon, W3C `performActions`, the complete create/inspect endpoints, and the production sidebar/dialog.
- Produces: one `tauri-driver` scenario that clones a fully configured agent through the actual WebView and current architecture documentation for complete create/clone behavior.

- [ ] **Step 1: Add a failing real Desktop clone scenario**

Seed a stopped source through the fixture daemon from WebView JavaScript with representative values from every form section:

```ts
await desktop.execute(`
  return window.__TAURI_INTERNALS__.invoke("daemon_status").then(async (status) => {
    window.__cloneBaseURL = status.base_url;
    const response = await fetch(status.base_url + "/api/agents", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        image: "basic:latest", name: "clone-source", cwd: "",
        harness: "codex", model: "gpt-5", effort: "high",
        interactive: false, loop: false,
        interval_s: 12, timeout_s: 34, hard_timeout_s: 56,
        on_timeout: "stop", on_error: "restart", max_idle_iterations: 7,
        user_prompt: "desktop clone prompt",
        env: { CSV: "a,b", EQ: "a=b", LINES: "one\\ntwo" },
        messages_batch: 8, messages_max_queue: 900,
        group: "desktop-clone-team", alias: "Source alias",
        notes: "desktop clone notes", color: "#123abc"
      })
    });
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  });
`);
```

Wait for the sidebar row, use W3C pointer actions with button `2` over it, choose **Clone**, and assert representative field values from Target, Identity, Runtime, Autopilot, and Lifecycle. Enter `clone-copy` as the blank name, submit with **Start now** off, and inspect `/api/agents/clone-copy` through the same `window.__cloneBaseURL`. Assert raw `configured_cwd`, structured environment, policies/counts, group/metadata, and schema-v2 image-owned plugins match the source.

- [ ] **Step 2: Run the focused Desktop spec and observe the expected failure**

Run:

```bash
. "$HOME/.cargo/env"
make desktop-e2e-build
make desktop-e2e DESKTOP_E2E_ARGS="tests/desktop/create-agent.pw.ts"
```

Expected: FAIL until the production context-menu clone flow and complete form work end to end.

- [ ] **Step 3: Update Web UI architecture documentation**

Replace the current abbreviated New agent paragraph with current behavior:

```mdx
The primary agent dialog has Target, Identity, Runtime, Autopilot, and Lifecycle
sections. Ordinary creation starts with documented defaults and submits one
complete create request; the daemon writes one complete stopped agent row.
Environment is a string-valued JSON object, schema-v1 plugins remain editable,
and schema-v2 plugins are image-owned and read-only.

Right-clicking any grouped or individual sidebar agent exposes **Clone**. Clone
loads the source inspect projection from that agent's explicit host, leaves the
unique name blank, and prefills every persisted agent-row configuration field,
including raw configured CWD and message limits. It does not copy secrets,
history, runtime evidence, messages, subscriptions, scripts, workdir contents,
retention/evals, budgets, or proxy policy. The target host and image remain
editable and every follow-up request stays pinned to that explicit target.
Older daemons that do not expose the complete clone projection must be updated.
**Start now** remains a separate retryable lifecycle request after creation.
```

Keep the existing runtime-preset, bare-image, explicit-host, and generated-output documentation accurate around the expanded paragraph.

- [ ] **Step 4: Run focused Desktop and documentation verification**

Run:

```bash
. "$HOME/.cargo/env"
make desktop-e2e DESKTOP_E2E_ARGS="tests/desktop/create-agent.pw.ts"
cd docs && npm run doctor && npm run build
```

Expected: PASS.

- [ ] **Step 5: Commit production acceptance coverage and docs**

```bash
git add ui/tests/desktop/create-agent.pw.ts docs/docs/architecture/web-ui.mdx
git commit -m "test: cover agent cloning in desktop"
```

---

### Task 6: Verify, review, and prepare integration

**Files:**
- Inspect: every file changed by Tasks 1-5
- Do not modify: version pins or ignored generated Desktop output

**Interfaces:**
- Consumes: all task commits and repository verification entry points.
- Produces: a clean reviewed branch ready for the predetermined local merge into `main`.

- [ ] **Step 1: Run the complete branch verification suite**

Run once for the unchanged final branch state:

```bash
make check
make full-check
git diff --check main...HEAD
```

Expected: all `make check` and `make full-check` summary rows PASS; `git diff --check` exits 0 with no output. `full-check` supplies the required browser and real production Desktop/`tauri-driver` coverage on Linux x86_64.

- [ ] **Step 2: Inspect the complete branch diff and generated-file boundaries**

Run:

```bash
git status --short
git diff --stat main...HEAD
git diff main...HEAD
git status --ignored --short desktop/dist desktop/src-tauri/resources/bin internal/storeui/dist
```

Confirm the diff contains only TARI-7 contract, persistence, UI, tests, docs, spec, and plan changes; no secret values, version changes, unrelated refactors, or staged ignored build output.

- [ ] **Step 3: Request code review and resolve all Critical/Important findings**

Use the required `requesting-code-review` workflow against `main...HEAD`. For each finding, reproduce or verify it, apply focused changes with the applicable TDD workflow, and rerun only the checks invalidated by those changes plus the final relevant verification point.

- [ ] **Step 4: Record the verified branch state on TARI-7**

Add a Native Task comment naming the branch head commit, `make check`, `make full-check`, `git diff --check`, review outcome, and confirmation that no ignored Desktop output is staged. Then invoke `finishing-a-development-branch` for the predetermined local merge flow.
