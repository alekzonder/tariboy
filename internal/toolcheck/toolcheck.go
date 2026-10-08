// Package toolcheck reports host commands the daemon and its agents need but
// cannot find on the daemon PATH, which is the PATH iterations start from.
package toolcheck

import (
	"os/exec"
	"strings"
)

// Tool names one missing command, what needs it and how to get it.
type Tool struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Install string `json:"install"`
}

// Result is "ready" when every tool resolves and "error" otherwise.
type Result struct {
	State        string `json:"state"`
	MissingTools []Tool `json:"missing_tools"`
	Message      string `json:"message,omitempty"`
}

var required = []Tool{
	{"tmux", "agent terminal sessions", "install tmux with the system package manager"},
	{"bash", "running iterations", "install bash with the system package manager"},
	{"python3", "agent tool scripts", "install Python 3 with the system package manager"},
	{"git", "adding and refreshing Stores", "install git with the system package manager"},
	{"npx", "building images that carry skills-lock.json", "install Node.js (npm ships npx)"},
	{"tariboy", "the tariboy command inside agents", "install the Tariboy CLI and put ~/.local/bin on PATH"},
	{"ttasks", "the ttasks command inside agents", "install the Tariboy CLI and put ~/.local/bin on PATH"},
}

var harnesses = []string{"claude", "codex", "opencode", "agent"}

// Check resolves every tool on the current PATH.
func Check() Result {
	var missing []Tool
	for _, tool := range required {
		if _, err := exec.LookPath(tool.Name); err != nil {
			missing = append(missing, tool)
		}
	}
	if !anyFound(harnesses) {
		missing = append(missing, Tool{
			Name:    strings.Join(harnesses, " | "),
			Purpose: "at least one agent harness",
			Install: "install Claude Code, Codex, OpenCode or Cursor Agent",
		})
	}
	if len(missing) == 0 {
		return Result{State: "ready", MissingTools: []Tool{}}
	}
	names := make([]string, len(missing))
	for i, tool := range missing {
		names[i] = tool.Name
	}
	return Result{
		State:        "error",
		MissingTools: missing,
		Message:      "missing required tools: " + strings.Join(names, ", ") + "; install them on the daemon host",
	}
}

func anyFound(names []string) bool {
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}
