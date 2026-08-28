package commands

import (
	"errors"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
)

type fakePlugins struct {
	installed     string
	removed       string
	restarted     string
	restartErr    error
	routesFn      func(name string) (map[string]any, error)
	actionFn      func(name string, body map[string]any) (map[string]any, error)
	validateFn    func(name, action string, body map[string]any) error
	applied       []map[string]any
	actionCalls   int
	validateCalls int
	contributions []plugins.Contribution
}

func (f *fakePlugins) Contributions() ([]plugins.Contribution, error) {
	return f.contributions, nil
}

func (f *fakePlugins) ValidateOperatorAction(name, action string, body map[string]any) error {
	f.validateCalls++
	if f.validateFn != nil {
		return f.validateFn(name, action, body)
	}
	return nil
}

func (f *fakePlugins) Install(path string) (map[string]any, error) {
	f.installed = path
	return map[string]any{"name": "echo", "installed": true}, nil
}
func (f *fakePlugins) Remove(name string) error { f.removed = name; return nil }
func (f *fakePlugins) Restart(name string) error {
	if f.restartErr != nil {
		return f.restartErr
	}
	f.restarted = name
	return nil
}
func (f *fakePlugins) List() ([]map[string]any, error) {
	return []map[string]any{{"name": "echo", "state": "running", "types": []string{"channel-sink"}}}, nil
}
func (f *fakePlugins) Inspect(name string) (map[string]any, error) {
	if name != "echo" {
		return nil, errors.New("plugin not found")
	}
	return map[string]any{"name": "echo", "version": "0.1.0"}, nil
}
func (f *fakePlugins) Logs(name string, tail int) ([]string, error) {
	return []string{"line1", "line2"}, nil
}
func (f *fakePlugins) PluginRoutes(name string) (map[string]any, error) {
	if f.routesFn != nil {
		return f.routesFn(name)
	}
	return map[string]any{"routes": map[string]any{}, "has_token": true}, nil
}
func (f *fakePlugins) PluginAction(name string, body map[string]any) (map[string]any, error) {
	f.actionCalls++
	if f.actionFn != nil {
		return f.actionFn(name, body)
	}
	return map[string]any{}, nil
}
func (f *fakePlugins) ApplyActionSubscriptions(name string, response map[string]any) error {
	f.applied = append(f.applied, response)
	return nil
}

func TestPluginCommands(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fp := &fakePlugins{}
	c.Plugins = fp

	if _, err := h(t, "plugin.install")(c, registry.Params{"path": "/tmp/echo"}); err != nil {
		t.Fatal(err)
	}
	if fp.installed != "/tmp/echo" {
		t.Fatalf("install path = %q", fp.installed)
	}
	ls, err := h(t, "plugin.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ls = %v", ls)
	}
	if _, err := h(t, "plugin.inspect")(c, registry.Params{"name": "echo"}); err != nil {
		t.Fatal(err)
	}
	logs, _ := h(t, "plugin.logs")(c, registry.Params{"name": "echo", "tail": 50})
	if len(logs.(map[string]any)["lines"].([]string)) != 2 {
		t.Fatalf("logs = %v", logs)
	}
	if _, err := h(t, "plugin.rm")(c, registry.Params{"name": "echo"}); err != nil {
		t.Fatal(err)
	}
	if fp.removed != "echo" {
		t.Fatalf("removed = %q", fp.removed)
	}
}

// wantUserErr asserts err is an api.UserError with the given code.
func wantUserErr(t *testing.T, err error, code string) {
	t.Helper()
	var ue api.UserError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v (%T), want api.UserError{Code:%q}", err, err, code)
	}
	if ue.Code != code {
		t.Fatalf("err code = %q, want %q", ue.Code, code)
	}
}

// TestPluginTraversalNameRejected proves the CLI-layer guard: rm/logs/inspect
// with a traversing name return api.UserError{Code:"bad_name"} and NEVER reach
// the host (fp.removed stays empty).
func TestPluginTraversalNameRejected(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fp := &fakePlugins{}
	c.Plugins = fp

	for _, name := range []string{"../victim", "..", "a/b", "/etc", ""} {
		_, err := h(t, "plugin.rm")(c, registry.Params{"name": name})
		wantUserErr(t, err, "bad_name")
		_, err = h(t, "plugin.logs")(c, registry.Params{"name": name})
		wantUserErr(t, err, "bad_name")
		_, err = h(t, "plugin.inspect")(c, registry.Params{"name": name})
		wantUserErr(t, err, "bad_name")
	}
	if fp.removed != "" {
		t.Fatalf("host Remove reached with a traversing name: %q", fp.removed)
	}
}

// runningPlugins reports "echo" as running from both List and Inspect so the
// rm force-gate can see it.
type runningPlugins struct {
	fakePlugins
}

func (r *runningPlugins) Inspect(name string) (map[string]any, error) {
	return map[string]any{"name": name, "state": "running"}, nil
}

// TestPluginRmForce proves --force wiring: without --force a running plugin is
// refused (Code:"running", host untouched); with --force it is removed.
func TestPluginRmForce(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	rp := &runningPlugins{}
	c.Plugins = rp

	_, err := h(t, "plugin.rm")(c, registry.Params{"name": "echo"})
	wantUserErr(t, err, "running")
	if rp.removed != "" {
		t.Fatalf("running plugin removed without --force: %q", rp.removed)
	}

	if _, err := h(t, "plugin.rm")(c, registry.Params{"name": "echo", "force": true}); err != nil {
		t.Fatal(err)
	}
	if rp.removed != "echo" {
		t.Fatalf("removed = %q, want echo", rp.removed)
	}
}

// TestPluginRestart proves the restart handler forwards the name to the host,
// rejects a traversing name before the host is touched, and maps ErrNotFound to
// a clean not_found UserError.
func TestPluginRestart(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fp := &fakePlugins{}
	c.Plugins = fp

	res, err := h(t, "plugin.restart")(c, registry.Params{"name": "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if fp.restarted != "echo" {
		t.Fatalf("restarted = %q, want echo", fp.restarted)
	}
	if res.(map[string]any)["restarted"] != "echo" {
		t.Fatalf("res = %v", res)
	}

	// A traversing name is refused at the CLI guard, never reaching the host.
	fp.restarted = ""
	_, err = h(t, "plugin.restart")(c, registry.Params{"name": "../victim"})
	wantUserErr(t, err, "bad_name")
	if fp.restarted != "" {
		t.Fatalf("host Restart reached with a traversing name: %q", fp.restarted)
	}

	// ErrNotFound from the host becomes a clean not_found UserError.
	c.Plugins = &fakePlugins{restartErr: plugins.ErrNotFound}
	_, err = h(t, "plugin.restart")(c, registry.Params{"name": "ghost"})
	wantUserErr(t, err, "not_found")

	// Any other host error becomes restart_failed.
	c.Plugins = &fakePlugins{restartErr: errors.New("boom")}
	_, err = h(t, "plugin.restart")(c, registry.Params{"name": "echo"})
	wantUserErr(t, err, "restart_failed")
}

// TestPluginActionForwards proves --data is forwarded as an opaque object with
// only the positional action added by core.
func TestPluginActionForwards(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	var got map[string]any
	c.Plugins = &fakePlugins{
		actionFn: func(name string, body map[string]any) (map[string]any, error) {
			if name != "messenger-provider" {
				t.Fatalf("forwarded to %q", name)
			}
			got = body
			return map[string]any{
				"result":        map[string]any{"channel": "chat:orders"},
				"subscriptions": map[string]any{"add": []any{"chat:orders"}},
			}, nil
		},
	}
	res, err := h(t, "plugin.action")(c, registry.Params{
		"name": "messenger-provider", "action": "bind",
		"data": `{"channel":"chat:orders","external_id":"c-1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["action"] != "bind" || got["external_id"] != "c-1" || got["channel"] != "chat:orders" {
		t.Fatalf("forwarded body = %v", got)
	}
	if res.(map[string]any)["channel"] != "chat:orders" {
		t.Fatalf("res = %v", res)
	}
}

func TestPluginActionRejectsMalformedDataBeforeCallingPlugin(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fp := &fakePlugins{}
	c.Plugins = fp
	for _, data := range []string{`{`, `[]`, `{"action":"override"}`} {
		_, err := h(t, "plugin.action")(c, registry.Params{
			"name": "messenger-provider", "action": "bind", "data": data,
		})
		wantUserErr(t, err, "bad_data")
	}
	if fp.actionCalls != 0 {
		t.Fatalf("plugin called %d times", fp.actionCalls)
	}
}

func TestPluginActionRejectsManifestInvalidDataBeforeCallingPlugin(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	fp := &fakePlugins{validateFn: func(_, _ string, body map[string]any) error {
		if _, found := body["admin"]; found {
			return errors.New("field admin is not declared")
		}
		return nil
	}}
	c.Plugins = fp
	_, err := h(t, "plugin.action")(c, registry.Params{
		"name": "telegram", "action": "configure", "data": `{"admin":true}`,
	})
	wantUserErr(t, err, "bad_action_data")
	if fp.validateCalls != 1 || fp.actionCalls != 0 {
		t.Fatalf("validate calls=%d action calls=%d", fp.validateCalls, fp.actionCalls)
	}
}

// TestPluginActionMapsErrors proves host/plugin errors become clean UserErrors.
func TestPluginActionMapsErrors(t *testing.T) {
	c, _, _ := ctxWithStore(t)

	c.Plugins = &fakePlugins{actionFn: func(string, map[string]any) (map[string]any, error) {
		return nil, plugins.ErrNotRunning
	}}
	_, err := h(t, "plugin.action")(c, registry.Params{"name": "messenger", "action": "bind"})
	wantUserErr(t, err, "not_running")

	c.Plugins = &fakePlugins{actionFn: func(string, map[string]any) (map[string]any, error) {
		return nil, &plugins.ActionError{Status: 409, Code: "no_token"}
	}}
	_, err = h(t, "plugin.action")(c, registry.Params{"name": "messenger", "action": "create"})
	wantUserErr(t, err, "no_token")
}

// TestPluginRoutesForwards proves the routes handler forwards to the host.
func TestPluginRoutesForwards(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	c.Plugins = &fakePlugins{routesFn: func(name string) (map[string]any, error) {
		return map[string]any{"routes": map[string]any{"c1": "chat:orders"}, "has_token": false}, nil
	}}
	res, err := h(t, "plugin.routes")(c, registry.Params{"name": "messenger"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["has_token"] != false {
		t.Fatalf("res = %v", res)
	}
}

func TestPluginContributionsForwardsSafeManifestData(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	c.Plugins = &fakePlugins{contributions: []plugins.Contribution{{
		Name:     "telegram",
		Commands: []plugins.OperatorCommand{{Path: "status", Summary: "Show status", Action: "status"}},
		Settings: &plugins.SettingsContribution{Title: "Telegram"},
	}}}
	res, err := h(t, "plugin.contributions")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	got := res.(map[string]any)["plugins"].([]plugins.Contribution)
	if len(got) != 1 || got[0].Name != "telegram" || got[0].Commands[0].Action != "status" {
		t.Fatalf("contributions = %+v", got)
	}
}
