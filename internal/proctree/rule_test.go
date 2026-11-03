package proctree

import "testing"

const harnessPID = 100

// claudeHookCommand is a hook command in the form harness.InstantMailHookCommand
// builds it; the harness runs it as `/bin/sh -c COMMAND`.
const claudeHookCommand = "'/base/bin/tmail' instant --format claude-post-tool-use"

var hookCommands = []string{
	claudeHookCommand,
	"'/base/bin/tmail' instant --format claude-stop",
}

// tree is the live layout under a tmux pane: the tmux server, the pane
// supervisor and the shim are ancestors and siblings of the harness.
func tree(children ...Proc) []Proc {
	procs := []Proc{
		{PID: 1, PPID: 0, Comm: "systemd"},
		{PID: 10, PPID: 1, Comm: "tmux: server", Args: []string{"tmux", "new-session"}},
		{PID: 20, PPID: 1, Comm: "tariboy-shim", Args: []string{"tariboy-shim", "--iteration-dir", "/x"}},
		{PID: 30, PPID: 10, Comm: "tariboy-shim", Args: []string{"tariboy-shim", "__tmux-supervisor"}},
		{PID: harnessPID, PPID: 30, Comm: "claude", Args: []string{"claude"}},
	}
	return append(procs, children...)
}

func child(pid int, comm string, args ...string) Proc {
	return Proc{PID: pid, PPID: harnessPID, Comm: comm, Args: args}
}

func TestAgentCommandFindsLiveShellCommand(t *testing.T) {
	procs := tree(child(200, "bash", "/bin/bash", "-c", "source /snap && eval 'make check'"))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("live bash command under the harness is not an agent command")
	}
}

func TestAgentCommandAcceptsLoginShellName(t *testing.T) {
	procs := tree(child(200, "-bash", "-bash"))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("-bash is not an agent command")
	}
}

func TestAgentCommandAcceptsEveryShell(t *testing.T) {
	for _, sh := range []string{"bash", "sh", "zsh", "dash", "fish"} {
		if !AgentCommand(tree(child(200, sh, sh, "-c", "sleep 30")), harnessPID, nil) {
			t.Errorf("%s is not an agent command", sh)
		}
	}
}

func TestAgentCommandIgnoresZombieShell(t *testing.T) {
	zombie := child(200, "bash", "/bin/bash", "-c", "make check")
	zombie.Zombie = true
	if AgentCommand(tree(zombie), harnessPID, hookCommands) {
		t.Fatal("zombie shell counted as an agent command")
	}
}

func TestAgentCommandWithoutChildren(t *testing.T) {
	if AgentCommand(tree(), harnessPID, hookCommands) {
		t.Fatal("harness without children has an agent command")
	}
}

// MCP servers and harness plugins are direct non-shell children.
func TestAgentCommandExcludesNonShellChildren(t *testing.T) {
	cases := map[string]Proc{
		"node":    child(200, "node", "node", "/srv/mcp/index.js"),
		"npx":     child(200, "npx", "npx", "-y", "@acme/mcp"),
		"uvx":     child(200, "uvx", "uvx", "mcp-server-git"),
		"python":  child(200, "python3", "python3", "-m", "mcp_server"),
		"tariboy": child(200, "tariboy", "/base/bin/tariboy", "mcp"),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if AgentCommand(tree(p), harnessPID, hookCommands) {
				t.Fatalf("%s child counted as an agent command", name)
			}
		})
	}
}

// A shell under the tmux server, the pane supervisor or the shim is not under
// the harness, so it never counts.
func TestAgentCommandIgnoresShellsOutsideTheHarness(t *testing.T) {
	cases := map[string]int{"tmux server": 10, "pane supervisor": 30, "shim": 20}
	for name, parent := range cases {
		t.Run(name, func(t *testing.T) {
			procs := tree(Proc{PID: 200, PPID: parent, Comm: "bash", Args: []string{"bash", "-c", "sleep 30"}})
			if AgentCommand(procs, harnessPID, hookCommands) {
				t.Fatalf("shell under the %s counted as an agent command", name)
			}
		})
	}
}

// The harness ancestors themselves are never children of the harness.
func TestAgentCommandIgnoresHarnessAncestors(t *testing.T) {
	if AgentCommand(tree(), 30, hookCommands) {
		t.Fatal("harness as the pane supervisor child counted as an agent command")
	}
}

// A grandchild whose shell exited was reparented away from the harness.
func TestAgentCommandIgnoresReparentedGrandchild(t *testing.T) {
	procs := tree(Proc{PID: 300, PPID: 1, Comm: "bash", Args: []string{"bash", "-c", "sleep 30"}})
	if AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("reparented shell counted as an agent command")
	}
}

func TestAgentCommandExcludesInstantMailHook(t *testing.T) {
	procs := tree(child(200, "sh", "/bin/sh", "-c", claudeHookCommand))
	if AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("instant mail hook counted as an agent command")
	}
}

func TestAgentCommandExcludesHookWithArguments(t *testing.T) {
	procs := tree(child(200, "sh", "/bin/sh", "-c", claudeHookCommand+" --verbose"))
	if AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("hook command with an argument counted as an agent command")
	}
}

func TestAgentCommandKeepsBashToolRunningTmailInstant(t *testing.T) {
	script := "source /home/a/.claude/shell-snapshots/snap.sh && eval '" + claudeHookCommand + " --format claude-stop' < /dev/null"
	procs := tree(child(200, "bash", "/bin/bash", "-c", script))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("bash tool running tmail instant is not an agent command")
	}
}

func TestAgentCommandKeepsImageBridgeScript(t *testing.T) {
	script := "source /snap && eval '/home/a/.tariboy/agents/x/image-bridges/759033ed/2/claude/skills/scripts/scripts.sh run'"
	procs := tree(child(200, "bash", "/bin/bash", "-c", script))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("bash tool running an image bridge script is not an agent command")
	}
}

func TestAgentCommandHookMatchStopsAtWordBoundary(t *testing.T) {
	procs := tree(child(200, "sh", "/bin/sh", "-c", "'/base/bin/tmail' instant --format claude-stopper"))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("claude-stopper matched the claude-stop hook")
	}
}

func TestAgentCommandShellWithoutScript(t *testing.T) {
	procs := tree(child(200, "bash", "/bin/bash", "/tmp/run.sh"))
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("shell running a script file is not an agent command")
	}
}

func TestAgentCommandFindsScriptAfterCombinedFlag(t *testing.T) {
	procs := tree(child(200, "bash", "/bin/bash", "-lc", claudeHookCommand))
	if AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("hook after -lc counted as an agent command")
	}
}

func TestAgentCommandOneLiveCommandAmongExcluded(t *testing.T) {
	procs := tree(
		child(200, "node", "node", "mcp.js"),
		child(201, "sh", "/bin/sh", "-c", claudeHookCommand),
		child(202, "bash", "/bin/bash", "-c", "make check"),
	)
	if !AgentCommand(procs, harnessPID, hookCommands) {
		t.Fatal("live command next to excluded children is not found")
	}
}
