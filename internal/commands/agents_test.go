package commands

import (
	"errors"
	"net"
	"path/filepath"
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
func (f *fakeControl) RefreshLoopConfig(string) {}

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
