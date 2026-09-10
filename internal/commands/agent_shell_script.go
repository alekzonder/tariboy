package commands

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func validateBash(script string) error {
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		return api.UserError{Code: "invalid_script", Msg: strings.TrimSpace(string(output)), Status: http.StatusBadRequest}
	}
	return nil
}

func shellScriptGet(path func(*registry.Ctx, registry.Params) (string, error)) registry.HandlerFunc {
	return func(c *registry.Ctx, p registry.Params) (any, error) {
		file, err := path(c, p)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		return map[string]any{"script": string(data)}, nil
	}
}

func shellScriptSet(path func(*registry.Ctx, registry.Params) (string, error)) registry.HandlerFunc {
	return func(c *registry.Ctx, p registry.Params) (any, error) {
		if err := validateBash(str(p, "script")); err != nil {
			return nil, err
		}
		file, err := path(c, p)
		if err != nil {
			return nil, err
		}
		if err := writeFileAtomic(file, []byte(str(p, "script"))); err != nil {
			return nil, err
		}
		return map[string]any{"saved": true}, nil
	}
}

func globalShellScriptPath(c *registry.Ctx, _ registry.Params) (string, error) {
	return filepath.Join(c.BaseDir, "global-agent-shell.sh"), nil
}

func agentShellScriptPath(c *registry.Ctx, p registry.Params) (string, error) {
	a, err := getAgent(c, str(p, "name"))
	if err != nil {
		return "", err
	}
	return agentdir.New(agentsDir(c), a.Name).ShellScriptPath(), nil
}

func globalShellScriptGet() registry.Command {
	return registry.Command{Path: "daemon.agent-shell-script.get", Summary: "Read the global agent shell script", HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/daemon/agent-shell-script"}, Handler: shellScriptGet(globalShellScriptPath)}
}

func globalShellScriptSet() registry.Command {
	return registry.Command{Path: "daemon.agent-shell-script.set", Summary: "Set the global agent shell script", Args: []registry.Arg{{Name: "script", Type: registry.String, Help: "Bash script"}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/daemon/agent-shell-script"}, Handler: shellScriptSet(globalShellScriptPath)}
}

func agentShellScriptGet() registry.Command {
	return registry.Command{Path: "agent.shell-script.get", Summary: "Read an agent shell script", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}}, HTTP: &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/agents/{name}/shell-script"}, Handler: shellScriptGet(agentShellScriptPath)}
}

func agentShellScriptSet() registry.Command {
	return registry.Command{Path: "agent.shell-script.set", Summary: "Set an agent shell script", Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}, {Name: "script", Type: registry.String, Help: "Bash script"}}, HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/shell-script"}, Handler: shellScriptSet(agentShellScriptPath)}
}
