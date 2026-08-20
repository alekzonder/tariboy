package registry

import "testing"

func cmd(path, method, route string) Command {
	return Command{
		Path:    path,
		Summary: "s",
		HTTP:    &HTTPRoute{Method: method, Path: route},
		Handler: func(c *Ctx, p Params) (any, error) { return nil, nil },
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := New()
	if err := r.Register(cmd("daemon.status", "GET", "/api/daemon/status")); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("daemon.status"); !ok {
		t.Fatal("registered command not found")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown command found")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	r := New()
	if err := r.Register(cmd("a.b", "GET", "/api/a/b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(cmd("a.b", "GET", "/api/other")); err == nil {
		t.Fatal("duplicate path accepted")
	}
	if err := r.Register(cmd("a.c", "GET", "/api/a/b")); err == nil {
		t.Fatal("duplicate route accepted")
	}
}

func TestRegisterValidates(t *testing.T) {
	r := New()
	if err := r.Register(Command{Path: "", Summary: "s"}); err == nil {
		t.Fatal("empty path accepted")
	}
	if err := r.Register(Command{Path: "x", Summary: ""}); err == nil {
		t.Fatal("empty summary accepted")
	}
	if err := r.Register(Command{Path: "x", Summary: "s", Handler: nil}); err == nil {
		t.Fatal("nil handler accepted")
	}
}

func TestCommandsSortedAndTree(t *testing.T) {
	r := New()
	r.Register(cmd("b.x", "GET", "/api/b/x"))
	r.Register(cmd("a.y", "GET", "/api/a/y"))
	cs := r.Commands()
	if len(cs) != 2 || cs[0].Path != "a.y" || cs[1].Path != "b.x" {
		t.Fatalf("Commands() not sorted: %+v", cs)
	}
	tree := r.Tree()
	if _, ok := tree["a"]; !ok {
		t.Fatalf("Tree missing group a: %v", tree)
	}
}

func TestRegisterGroupAndGet(t *testing.T) {
	r := New()
	if err := r.RegisterGroup("agent", "Manage agents"); err != nil {
		t.Fatalf("RegisterGroup: %v", err)
	}
	s, ok := r.Group("agent")
	if !ok || s != "Manage agents" {
		t.Fatalf("Group(agent) = %q, %v", s, ok)
	}
	if err := r.RegisterGroup("agent", "dup"); err == nil {
		t.Fatal("duplicate group should error")
	}
	if err := r.RegisterGroup("", "x"); err == nil {
		t.Fatal("empty path should error")
	}
	if err := r.RegisterGroup("x", ""); err == nil {
		t.Fatal("empty summary should error")
	}
}

func TestRegisterGroupCollidesWithCommand(t *testing.T) {
	r := New()
	mustCmd(t, r, "agent.ps", "List agents")
	if err := r.RegisterGroup("agent.ps", "collide"); err == nil {
		t.Fatal("group colliding with command path should error")
	}
}

func TestValidateRequiresGroupSummaries(t *testing.T) {
	r := New()
	mustCmd(t, r, "agent.status.show", "Show status")
	// Missing groups "agent" and "agent.status".
	if err := r.Validate(); err == nil {
		t.Fatal("Validate should fail when ancestor group summary is missing")
	}
	if err := r.RegisterGroup("agent", "Manage agents"); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate should still fail: agent.status missing")
	}
	if err := r.RegisterGroup("agent.status", "Runtime status"); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate should pass once all groups described: %v", err)
	}
}

func TestValidateRejectsCommandEqualToGroup(t *testing.T) {
	r := New()
	mustCmd(t, r, "agent.status", "runnable")
	mustCmd(t, r, "agent.status.history", "child")
	r.RegisterGroup("agent", "Manage agents")
	r.RegisterGroup("agent.status", "Runtime status")
	if err := r.Validate(); err == nil {
		t.Fatal("command path equal to a group path must fail validation")
	}
}

func TestValidateRejectsOrphanGroup(t *testing.T) {
	r := New()
	// "a.b" is a group whose parent "a" is not a registered group: reachable
	// only if "a" exists as a tree node, so Validate must reject it.
	if err := r.RegisterGroup("a.b", "Orphan child"); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate should fail: group a.b has no parent group a")
	}
	// Registering the missing ancestor closes the gap.
	if err := r.RegisterGroup("a", "Parent"); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate should pass once parent group a exists: %v", err)
	}
}

func TestTreeSkipsCLIHidden(t *testing.T) {
	r := New()
	mustCmd(t, r, "daemon.up", "Start daemon")
	if err := r.Register(Command{
		Path: "daemon.status", Summary: "Daemon status", CLIHidden: true,
		Handler: func(c *Ctx, p Params) (any, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	r.RegisterGroup("daemon", "Daemon control")
	tree := r.Tree()
	daemon, ok := tree["daemon"].(map[string]any)
	if !ok {
		t.Fatal("daemon node missing")
	}
	if _, hidden := daemon["status"]; hidden {
		t.Fatal("Tree should skip CLIHidden command daemon.status")
	}
	if _, shown := daemon["up"]; !shown {
		t.Fatal("Tree should still carry visible command daemon.up")
	}
}

func TestTreeCarriesGroupSummary(t *testing.T) {
	r := New()
	mustCmd(t, r, "agent.ps", "List agents")
	r.RegisterGroup("agent", "Manage agents")
	tree := r.Tree()
	agent, ok := tree["agent"].(map[string]any)
	if !ok {
		t.Fatal("agent node missing")
	}
	if agent["summary"] != "Manage agents" {
		t.Fatalf("group summary = %v", agent["summary"])
	}
}

// mustCmd registers a minimal handler-only command for tests.
func mustCmd(t *testing.T, r *Registry, path, summary string) {
	t.Helper()
	if err := r.Register(Command{
		Path: path, Summary: summary,
		Handler: func(c *Ctx, p Params) (any, error) { return nil, nil },
	}); err != nil {
		t.Fatalf("Register(%s): %v", path, err)
	}
}
