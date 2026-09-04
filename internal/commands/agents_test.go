package commands

import (
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/store"
)

type fakeControl struct {
	agents        *agent.Store
	ran           registry.RunSpec
	stopped       string
	removed       string
	purged        bool
	reprovisioned string
	reprovImage   string
	itemsSent     []shim.KeyItem
	extendResult  registry.IterationTimeoutExtension
	extendErr     error
	extendName    string
	extendID      string
	live          string // LiveState return; defaults to "idle"
	loopUpdates   []bool
	refreshed     []string
}

func (f *fakeControl) Run(s registry.RunSpec) (string, error) {
	f.ran = s
	name := s.Name
	if name == "" {
		name = "gen-name"
	}
	return name, nil
}
func (f *fakeControl) Start(string) error   { return nil }
func (f *fakeControl) Stop(n string) error  { f.stopped = n; return nil }
func (f *fakeControl) Restart(string) error { return nil }
func (f *fakeControl) Kill(string) error    { return nil }
func (f *fakeControl) Remove(n string, _, purge bool) error {
	f.removed = n
	f.purged = purge
	return nil
}
func (f *fakeControl) Reprovision(n, img string) error {
	f.reprovisioned = n
	f.reprovImage = img
	return nil
}
func (f *fakeControl) Exec(string, string) (string, error) { return "queued", nil }
func (f *fakeControl) LiveState(string) (string, error) {
	if f.live != "" {
		return f.live, nil
	}
	return "idle", nil
}
func (f *fakeControl) Screen(string) (string, error) { return "", nil }
func (f *fakeControl) SendKeys(string, string) error { return nil }
func (f *fakeControl) SendKeysItems(_ string, items []shim.KeyItem) error {
	f.itemsSent = items
	return nil
}
func (f *fakeControl) Attach(string, int, int) (net.Conn, error) {
	return nil, errors.New("no session")
}
func (f *fakeControl) Resize(string, int, int) error { return nil }
func (f *fakeControl) ExtendIterationTimeout(name, id string) (registry.IterationTimeoutExtension, error) {
	f.extendName, f.extendID = name, id
	return f.extendResult, f.extendErr
}
func (f *fakeControl) SetLoopEnabled(name string, enabled bool) error {
	f.loopUpdates = append(f.loopUpdates, enabled)
	a, err := f.agents.Get(name)
	if err != nil {
		return err
	}
	a.LoopEnabled = enabled
	return f.agents.Update(a)
}
func (f *fakeControl) RefreshLoopConfig(name string) { f.refreshed = append(f.refreshed, name) }

func ctxWithStore(t *testing.T) (*registry.Ctx, *agent.Store, *fakeControl) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	fc := &fakeControl{agents: as}
	return &registry.Ctx{Store: s, Control: fc, BaseDir: t.TempDir()}, as, fc
}

func h(t *testing.T, path string) registry.HandlerFunc {
	t.Helper()
	cmd, ok := BuildRegistry().Get(path)
	if !ok {
		t.Fatalf("%s not registered", path)
	}
	return cmd.Handler
}

func TestAgentRunAndPs(t *testing.T) {
	c, as, fc := ctxWithStore(t)
	res, err := h(t, "agent.run")(c, registry.Params{
		"image": "basic:latest", "name": "smoke", "harness": "stub",
		"env": "A=1,B=2", "plugins": "context,status", "loop": true, "interactive": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["name"] != "smoke" {
		t.Fatalf("run result: %v", res)
	}
	if fc.ran.Env["A"] != "1" || len(fc.ran.Plugins) != 2 || !fc.ran.Loop {
		t.Fatalf("spec not parsed: %+v", fc.ran)
	}
	// seed for ps
	as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest", HarnessType: "stub", Interactive: true})
	ps, err := h(t, "agent.ps")(c, registry.Params{})
	if err != nil || ps.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ps: %v err=%v", ps, err)
	}
	rows := ps.(map[string]any)["agents"].([]map[string]any)
	if len(rows) != 1 || rows[0]["interactive"] != true {
		t.Fatalf("ps rows missing interactive=true: %#v", rows)
	}
}

// Catches the create handler dropping a clone field or lossy reparsing of
// structured HTTP environment/plugin values before they reach the manager.
func TestAgentRunMapsCompleteConfiguration(t *testing.T) {
	c, _, fc := ctxWithStore(t)
	cwd := t.TempDir()
	_, err := h(t, "agent.run")(c, registry.Params{
		"image": "basic:latest", "name": "clone", "cwd": cwd,
		"harness": "codex", "model": "gpt-5", "effort": "high",
		"interactive": true, "loop": false,
		"interval_s": float64(12), "timeout_s": float64(34), "hard_timeout_s": float64(56),
		"on_timeout": "stop", "on_error": "restart", "max_idle_iterations": float64(7),
		"user_prompt":    "keep commas, equals=a=b, and\nnewlines",
		"env":            map[string]any{"CSV": "a,b", "EQ": "a=b", "LINES": "one\ntwo"},
		"plugins":        []any{"context", "custom"},
		"messages_batch": float64(8), "messages_max_queue": float64(900),
		"group": "reviewers", "alias": "Clone", "notes": "all fields", "color": "#123ABC",
	})
	if err != nil {
		t.Fatal(err)
	}
	goalEnabled := true
	want := registry.RunSpec{
		ImageRef: "basic:latest", Name: "clone", Cwd: cwd,
		Harness: "codex", Model: "gpt-5", Effort: "high", Interactive: true,
		Env:     map[string]string{"CSV": "a,b", "EQ": "a=b", "LINES": "one\ntwo"},
		Plugins: []string{"context", "custom"}, Loop: false,
		IntervalS: 12, TimeoutS: 34, HardTimeoutS: 56,
		OnTimeout: "stop", OnError: "restart", MaxIdleIterations: 7,
		UserPrompt:    "keep commas, equals=a=b, and\nnewlines",
		MessagesBatch: 8, MessagesMaxQueue: 900,
		GoalEnabled: &goalEnabled, GoalWaitCustomerTimeoutS: 300, GoalDeliveryCooldownS: 60,
		Group: "reviewers", Alias: "Clone", Notes: "all fields", Color: "#123abc",
	}
	if !reflect.DeepEqual(fc.ran, want) {
		t.Fatalf("RunSpec mismatch:\nwant: %#v\n got: %#v", want, fc.ran)
	}
}

// Catches the create surface accepting Goal settings without carrying them to
// the manager or returning the canonical create projection.
func TestAgentGoalSettingsRoundTrip(t *testing.T) {
	c, _, fc := ctxWithStore(t)
	result, err := h(t, "agent.run")(c, registry.Params{
		"image": "basic:latest", "name": "worker",
		"goal_enabled": false, "goal_wait_customer_timeout_s": float64(120),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["goal_enabled"] != false || got["goal_wait_customer_timeout_s"] != 120 {
		t.Fatalf("create result = %#v", got)
	}
	if fc.ran.GoalEnabled == nil || *fc.ran.GoalEnabled || fc.ran.GoalWaitCustomerTimeoutS != 120 {
		t.Fatalf("RunSpec did not carry Goal settings: %#v", fc.ran)
	}
}

// Catches Goal updates bypassing Store.Update (which owns sticky-key clearing)
// or failing to use the existing configuration refresh/goal signal hook.
func TestAgentGoalSettingsUpdateClearsCurrentKeyAndSignals(t *testing.T) {
	c, agents, fc := ctxWithStore(t)
	if err := agents.Create(agent.Agent{Name: "worker", GoalEnabled: true, GoalWaitCustomerTimeoutS: 300}); err != nil {
		t.Fatal(err)
	}
	if err := agents.SetCurrentGoal("worker", "TARI-43"); err != nil {
		t.Fatal(err)
	}

	result, err := h(t, "agent.goal-enabled")(c, registry.Params{"name": "worker", "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["goal_enabled"] != false || got["goal_wait_customer_timeout_s"] != 300 {
		t.Fatalf("update result = %#v", got)
	}
	if _, editable := got["current_goal_task_key"]; editable {
		t.Fatalf("update exposed daemon-owned key: %#v", got)
	}
	stored, err := agents.Get("worker")
	if err != nil {
		t.Fatal(err)
	}
	if stored.GoalEnabled || stored.CurrentGoalTaskKey != "" {
		t.Fatalf("disabled Goal = %#v", stored)
	}
	if !reflect.DeepEqual(fc.refreshed, []string{"worker"}) {
		t.Fatalf("configuration refreshes = %v", fc.refreshed)
	}

	if _, err := h(t, "agent.goal-wait-customer-timeout")(c, registry.Params{"name": "worker", "seconds": float64(120)}); err != nil {
		t.Fatal(err)
	}
	stored, err = agents.Get("worker")
	if err != nil || stored.GoalWaitCustomerTimeoutS != 120 {
		t.Fatalf("stored timeout = %#v err=%v", stored, err)
	}
	if len(fc.refreshed) != 2 {
		t.Fatalf("configuration refreshes = %v", fc.refreshed)
	}
	if _, err := h(t, "agent.goal-delivery-cooldown")(c, registry.Params{"name": "worker", "seconds": float64(120)}); err != nil {
		t.Fatal(err)
	}
	stored, err = agents.Get("worker")
	if err != nil || stored.GoalDeliveryCooldownS != 120 {
		t.Fatalf("stored cooldown = %#v err=%v", stored, err)
	}
	if len(fc.refreshed) != 3 {
		t.Fatalf("configuration refreshes = %v", fc.refreshed)
	}

	for _, params := range []registry.Params{
		{"name": "worker", "seconds": float64(0)},
		{"name": "worker"},
	} {
		if _, err := h(t, "agent.goal-wait-customer-timeout")(c, params); !isCode(err, "bad_goal_wait_customer_timeout") {
			t.Fatalf("invalid timeout error = %v", err)
		}
	}
	if len(fc.refreshed) != 3 {
		t.Fatalf("rejected update signaled Goal: %v", fc.refreshed)
	}
}

// Catches inspect/list omitting either editable Goal setting or the read-only
// daemon-owned sticky key.
func TestAgentGoalSettingsInspectAndListProjection(t *testing.T) {
	c, agents, _ := ctxWithStore(t)
	if err := agents.Create(agent.Agent{Name: "worker", GoalEnabled: true, GoalWaitCustomerTimeoutS: 120}); err != nil {
		t.Fatal(err)
	}
	if err := agents.SetCurrentGoal("worker", "TARI-43"); err != nil {
		t.Fatal(err)
	}
	inspect, err := h(t, "agent.inspect")(c, registry.Params{"name": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := h(t, "agent.ps")(c, registry.Params{})
	for _, got := range []map[string]any{
		inspect.(map[string]any),
		mustAgentRows(t, ps, err)[0],
	} {
		if got["goal_enabled"] != true || got["goal_wait_customer_timeout_s"] != 120 || got["current_goal_task_key"] != "TARI-43" {
			t.Fatalf("Goal projection = %#v", got)
		}
	}
}

func mustAgentRows(t *testing.T, value any, err error) []map[string]any {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any)["agents"].([]map[string]any)
}

// Each case catches a malformed clone request reaching provisioning or
// persistence instead of failing at the agent.run boundary with a stable code.
func TestAgentRunRejectsInvalidCompleteConfiguration(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value any
		extra registry.Params
		code  string
	}{
		{name: "negative interval", key: "interval_s", value: float64(-1), code: "bad_interval"},
		{name: "fractional interval", key: "interval_s", value: 1.5, code: "bad_interval"},
		{name: "negative timeout", key: "timeout_s", value: float64(-1), code: "bad_timeout"},
		{name: "negative hard timeout", key: "hard_timeout_s", value: float64(-1), code: "bad_hard_timeout"},
		{name: "negative idle limit", key: "max_idle_iterations", value: float64(-1), code: "bad_max_idle"},
		{name: "zero message batch", key: "messages_batch", value: float64(0), code: "bad_messages_batch"},
		{name: "zero message queue", key: "messages_max_queue", value: float64(0), code: "bad_messages_max_queue"},
		{name: "timeout policy", key: "on_timeout", value: "continue", code: "bad_on_timeout"},
		{name: "error policy", key: "on_error", value: "continue", code: "bad_on_error"},
		{name: "color", key: "color", value: "#12345g", code: "bad_color"},
		{name: "environment value", key: "env", value: map[string]any{"COUNT": float64(3)}, code: "bad_env"},
		{name: "plugin value", key: "plugins", value: []any{"context", float64(3)}, code: "bad_plugins"},
		{name: "ambiguous timeout", key: "timeout", value: "1m", extra: registry.Params{"timeout_s": float64(60)}, code: "ambiguous_timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _, fc := ctxWithStore(t)
			params := registry.Params{"image": "basic:latest", tc.key: tc.value}
			for key, value := range tc.extra {
				params[key] = value
			}
			_, err := h(t, "agent.run")(c, params)
			var userErr api.UserError
			if !errors.As(err, &userErr) || userErr.Code != tc.code {
				t.Fatalf("error = %#v, want UserError code %q", err, tc.code)
			}
			if fc.ran.ImageRef != "" {
				t.Fatalf("Control.Run called for invalid request: %#v", fc.ran)
			}
		})
	}
}

// Catches generated OpenAPI advertising the legacy comma strings after the
// HTTP create route learned lossless object/array inputs.
func TestAgentRunDeclaresStructuredHTTPInputs(t *testing.T) {
	args := map[string]registry.Arg{}
	for _, arg := range agentRun().Args {
		args[arg.Name] = arg
	}
	for _, name := range []string{"env", "plugins"} {
		oneOf, ok := args[name].Schema["oneOf"].([]any)
		if !ok || len(oneOf) != 2 {
			t.Fatalf("%s schema = %#v, want two-form oneOf", name, args[name].Schema)
		}
	}
	envObject := args["env"].Schema["oneOf"].([]any)[1].(map[string]any)
	if envObject["type"] != "object" || envObject["additionalProperties"].(map[string]any)["type"] != "string" {
		t.Fatalf("env object schema = %#v", envObject)
	}
	pluginArray := args["plugins"].Schema["oneOf"].([]any)[1].(map[string]any)
	if pluginArray["type"] != "array" || pluginArray["items"].(map[string]any)["type"] != "string" {
		t.Fatalf("plugins array schema = %#v", pluginArray)
	}
	for _, name := range []string{"interval_s", "timeout_s", "hard_timeout_s", "max_idle_iterations", "messages_batch", "messages_max_queue"} {
		if args[name].Type != registry.Int {
			t.Fatalf("%s type = %q, want int", name, args[name].Type)
		}
	}
}

func TestAgentStatusIncludesActiveIterationDeadlines(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := as.CreateIteration(agent.Iteration{ID: "smoke-1", Agent: "smoke", Status: "running", StartedAt: "2026-07-14T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	if err := as.InitializeIterationTimeout("smoke-1", 60, 120, now); err != nil {
		t.Fatal(err)
	}
	soft := now.Add(time.Minute).Format(time.RFC3339Nano)
	res, err := h(t, "agent.status.show")(c, registry.Params{"name": "smoke"})
	if err != nil {
		t.Fatal(err)
	}
	out := res.(map[string]any)
	active, ok := out["active_iteration"].(map[string]any)
	if !ok || active["id"] != "smoke-1" || active["effective_deadline"] != soft || active["timeout_period_s"] != 60 || out["server_now"] == "" {
		t.Fatalf("status = %#v", out)
	}
}

func TestIterationExtendTimeoutCommand(t *testing.T) {
	c, as, fc := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "smoke", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	fc.extendResult = registry.IterationTimeoutExtension{TimeoutDeadline: "soft", HardTimeoutDeadline: "hard", TimeoutExtensions: 2, ShimSync: "pending"}
	res, err := h(t, "iteration.extend-timeout")(c, registry.Params{"name": "smoke", "id": "smoke-1"})
	if err != nil || fc.extendName != "smoke" || fc.extendID != "smoke-1" {
		t.Fatalf("res=%#v err=%v control=%+v", res, err, fc)
	}
	out := res.(map[string]any)
	if out["shim_sync"] != "pending" || out["timeout_extensions"] != 2 {
		t.Fatalf("result = %#v", out)
	}
	fc.extendErr = agent.ErrNoIterationTimeout
	_, err = h(t, "iteration.extend-timeout")(c, registry.Params{"name": "smoke", "id": "smoke-1"})
	var ue api.UserError
	if !errors.As(err, &ue) || ue.Status != 422 {
		t.Fatalf("error = %#v", err)
	}
}

func TestAgentStopAndRm(t *testing.T) {
	c, as, fc := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke"})
	if _, err := h(t, "agent.stop")(c, registry.Params{"name": "smoke"}); err != nil {
		t.Fatal(err)
	}
	if fc.stopped != "smoke" {
		t.Fatalf("stop not routed: %q", fc.stopped)
	}
	if _, err := h(t, "agent.rm")(c, registry.Params{"name": "smoke", "force": true}); err != nil {
		t.Fatal(err)
	}
	if fc.removed != "smoke" {
		t.Fatalf("rm not routed: %q", fc.removed)
	}
	// Default rm preserves data (purge=false).
	if fc.purged {
		t.Fatalf("rm defaulted to purge; must preserve unless --purge given")
	}
	// --purge routes through as a hard delete.
	if _, err := h(t, "agent.rm")(c, registry.Params{"name": "smoke", "force": true, "purge": true}); err != nil {
		t.Fatal(err)
	}
	if !fc.purged {
		t.Fatalf("rm --purge did not route purge=true")
	}
	// reprovision routes name + image through to Control.Reprovision.
	if _, err := h(t, "agent.reprovision")(c, registry.Params{"name": "smoke", "image": "img:v2"}); err != nil {
		t.Fatal(err)
	}
	if fc.reprovisioned != "smoke" || fc.reprovImage != "img:v2" {
		t.Fatalf("reprovision not routed: name=%q image=%q", fc.reprovisioned, fc.reprovImage)
	}
}

func TestAgentInspectNotFound(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	if _, err := h(t, "agent.inspect")(c, registry.Params{"name": "ghost"}); err == nil {
		t.Fatal("inspect of missing agent should error")
	}
}

func TestAgentInspectReportsEffectiveCwd(t *testing.T) {
	c, as, _ := ctxWithStore(t)

	// Empty Cwd -> effective cwd is the agent's workdir, never blank.
	as.Create(agent.Agent{Name: "nocwd"})
	v, err := h(t, "agent.inspect")(c, registry.Params{"name": "nocwd"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(c.BaseDir, "agents", "nocwd", "workdir")
	if got := v.(map[string]any)["cwd"]; got != want {
		t.Fatalf("effective cwd: got %q want %q", got, want)
	}

	// Explicit Cwd is reported verbatim.
	as.Create(agent.Agent{Name: "setcwd", Cwd: "/srv/work"})
	v, err = h(t, "agent.inspect")(c, registry.Params{"name": "setcwd"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.(map[string]any)["cwd"]; got != "/srv/work" {
		t.Fatalf("explicit cwd: got %q want %q", got, "/srv/work")
	}
}

// The four calendar limits are an atomic agent configuration: malformed input
// must not partially replace the last accepted limits, and every agent view
// must expose the same derived budget projection.
func TestAgentBudgetSetIsAtomicAndProjected(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "alice", ImageRef: "basic:latest"}); err != nil {
		t.Fatal(err)
	}
	set := h(t, "agent.budget.set")
	params := registry.Params{"name": "alice", "hour_usd": "1.25", "day_usd": "2.50", "week_usd": "3.75", "month_usd": "5.00"}
	value, err := set(c, params)
	if err != nil {
		t.Fatal(err)
	}
	budget := value.(map[string]any)
	if budget["hour_usd"] != 1.25 || budget["day_usd"] != 2.50 || budget["week_usd"] != 3.75 || budget["month_usd"] != 5.00 {
		t.Fatalf("saved budget = %#v", budget)
	}

	bad := registry.Params{"name": "alice", "hour_usd": "9", "day_usd": "-1", "week_usd": "9", "month_usd": "9"}
	if _, err := set(c, bad); !isCode(err, "bad_budget") {
		t.Fatalf("invalid budget error = %v, want bad_budget", err)
	}
	inspect, err := h(t, "agent.inspect")(c, registry.Params{"name": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	projected := inspect.(map[string]any)["budget"].(map[string]any)
	if projected["hour_usd"] != 1.25 || projected["day_usd"] != 2.50 || projected["week_usd"] != 3.75 || projected["month_usd"] != 5.00 {
		t.Fatalf("budget changed after rejected update: %#v", projected)
	}
}

// Catches regressions where clone initialization sees the effective managed
// workdir instead of the raw configured CWD, or silently guesses message
// delivery defaults that belong to the persisted source agent.
func TestAgentInspectReportsCompleteCloneProjection(t *testing.T) {
	c, as, _ := ctxWithStore(t)
	if err := as.Create(agent.Agent{
		Name: "source", MessagesBatch: 17, MessagesMaxQueue: 1234,
	}); err != nil {
		t.Fatal(err)
	}

	value, err := h(t, "agent.inspect")(c, registry.Params{"name": "source"})
	if err != nil {
		t.Fatal(err)
	}
	got := value.(map[string]any)
	if got["configured_cwd"] != "" {
		t.Fatalf("configured_cwd = %#v, want raw empty configured value", got["configured_cwd"])
	}
	if got["cwd"] == "" {
		t.Fatal("effective cwd is empty, want managed-workdir fallback")
	}
	if got["messages_batch"] != 17 || got["messages_max_queue"] != 1234 {
		t.Fatalf("message limits = %#v/%#v, want 17/1234", got["messages_batch"], got["messages_max_queue"])
	}
}
