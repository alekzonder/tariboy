// Package proctree reads a snapshot of the host process table and decides
// whether a harness process waits for a live agent command.
package proctree

import "strings"

// Proc is one process of a snapshot. Args is set only for the direct
// children of the pids passed to Read.
type Proc struct {
	PID    int
	PPID   int
	Zombie bool
	Comm   string
	Args   []string
}

// AgentCommand reports whether the harness harnessPID runs a live agent
// command: a direct child that is not a zombie and that excluded does not
// exclude. Harnesses run tool commands through a shell, and a live shell is
// itself a child of the harness, so the direct children decide. When such a
// shell exits, its children are reparented away from the harness.
func AgentCommand(procs []Proc, harnessPID int, hookCommands []string) bool {
	for _, p := range procs {
		if p.PPID == harnessPID && p.PID != harnessPID && !p.Zombie && !excluded(p, hookCommands) {
			return true
		}
	}
	return false
}

// excluded holds every exclusion of the rule. A direct child of the harness is
// excluded when:
//   - it is not a shell (bash, sh, zsh, dash, fish): MCP servers such as node,
//     npx, uvx, python or the tariboy binary, and harness plugins;
//   - it is a shell whose -c script is an instant mail hook command the daemon
//     wrote for this harness, or starts with one followed by a space.
//
// The tmux server, the pane supervisor, the shim and the tariboy tool bridges
// are ancestors or siblings of the harness, never its direct children, so the
// rule never sees them.
func excluded(p Proc, hookCommands []string) bool {
	switch strings.TrimPrefix(p.Comm, "-") {
	case "bash", "sh", "zsh", "dash", "fish":
	default:
		return true
	}
	script, ok := shellScript(p.Args)
	if !ok {
		return false
	}
	for _, hook := range hookCommands {
		if hook != "" && (script == hook || strings.HasPrefix(script, hook+" ")) {
			return true
		}
	}
	return false
}

func isChildOf(p Proc, roots []int) bool {
	for _, r := range roots {
		if p.PPID == r && p.PID != r {
			return true
		}
	}
	return false
}

// shellScript returns the argument after the first flag that holds c, such as
// -c, -lc or -ec.
func shellScript(args []string) (string, bool) {
	for i := 1; i < len(args)-1; i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") {
			return args[i+1], true
		}
	}
	return "", false
}
