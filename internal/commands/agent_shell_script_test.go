package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentShellScriptCommandsPersistAndRejectInvalidBash(t *testing.T) {
	c, agents, _ := ctxWithStore(t)
	if err := agents.Create(agent.Agent{Name: "a1", OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	valid := "export COLOR=blue\n"
	if _, err := h(t, "agent.shell-script.set")(c, registry.Params{"name": "a1", "script": valid}); err != nil {
		t.Fatal(err)
	}
	if got, err := h(t, "agent.shell-script.get")(c, registry.Params{"name": "a1"}); err != nil || got.(map[string]any)["script"] != valid {
		t.Fatalf("get=%v err=%v", got, err)
	}
	if _, err := h(t, "agent.shell-script.set")(c, registry.Params{"name": "a1", "script": "if then\n"}); err == nil {
		t.Fatal("invalid Bash accepted")
	}
	data, err := os.ReadFile(agentdir.New(agentsDir(c), "a1").ShellScriptPath())
	if err != nil || string(data) != valid {
		t.Fatalf("file=%q err=%v", data, err)
	}
}

func TestGlobalShellScriptCommandsPersistAndRejectInvalidBash(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	valid := "export COLOR=green\n"
	if _, err := h(t, "daemon.agent-shell-script.set")(c, registry.Params{"script": valid}); err != nil {
		t.Fatal(err)
	}
	if got, err := h(t, "daemon.agent-shell-script.get")(c, registry.Params{}); err != nil || got.(map[string]any)["script"] != valid {
		t.Fatalf("get=%v err=%v", got, err)
	}
	if _, err := h(t, "daemon.agent-shell-script.set")(c, registry.Params{"script": "if then\n"}); err == nil {
		t.Fatal("invalid Bash accepted")
	}
	data, err := os.ReadFile(filepath.Join(c.BaseDir, "global-agent-shell.sh"))
	if err != nil || string(data) != valid {
		t.Fatalf("file=%q err=%v", data, err)
	}
}

func TestShellScriptCommandsRegisterRoutes(t *testing.T) {
	reg := BuildRegistry()
	for _, want := range []struct{ path, method, route string }{
		{"daemon.agent-shell-script.get", "GET", "/api/daemon/agent-shell-script"},
		{"daemon.agent-shell-script.set", "POST", "/api/daemon/agent-shell-script"},
		{"agent.shell-script.get", "GET", "/api/agents/{name}/shell-script"},
		{"agent.shell-script.set", "POST", "/api/agents/{name}/shell-script"},
	} {
		cmd, ok := reg.Get(want.path)
		if !ok || cmd.HTTP == nil || cmd.HTTP.Method != want.method || cmd.HTTP.Path != want.route {
			t.Fatalf("%s route = %#v", want.path, cmd.HTTP)
		}
	}
}
