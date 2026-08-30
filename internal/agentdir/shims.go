package agentdir

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/agent"
)

// WriteShims (re)writes the agent's bin shims so they exec skill-owned Python
// scripts — nothing
// else: it neither unpacks the image nor creates the tree, so the bin dir must
// already exist (Provision makes it).
//
// The absolute paths select the running daemon's versioned Store. The daemon
// therefore calls this for every stored agent at startup, on top of
// create/reprovision. Writing is skipped when the bytes already match.
func WriteShims(l Layout, a agent.Agent, skillsDir string) error {
	toolsScript := filepath.Join(skillsDir, "agent-tools", "scripts", "tools.py")
	doneScript := filepath.Join(skillsDir, "loop", "scripts", "loop.py")
	tasksScript := filepath.Join(skillsDir, "tasks", "scripts", "tasks.py")
	required := []string{toolsScript}
	if hasCapability(a.Plugins, "loop") {
		required = append(required, doneScript)
	}
	if hasCapability(a.Plugins, "tasks") {
		required = append(required, tasksScript)
	}
	for _, path := range required {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("skill script unavailable: %s", path)
		}
	}
	if err := writeShim(l, "tools", fmt.Sprintf("#!/usr/bin/env bash\nexec python3 -B %q \"$@\"\n", toolsScript)); err != nil {
		return err
	}
	donePath := filepath.Join(l.BinDir(), "i-am-done")
	if hasCapability(a.Plugins, "loop") {
		if err := writeShim(l, "i-am-done", fmt.Sprintf("#!/usr/bin/env bash\nexec python3 -B %q done \"$@\"\n", doneScript)); err != nil {
			return err
		}
	} else if err := os.Remove(donePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	tasksPath := filepath.Join(l.BinDir(), "tasks")
	if hasCapability(a.Plugins, "tasks") {
		return writeShim(l, "tasks", fmt.Sprintf("#!/usr/bin/env bash\nexec python3 -B %q \"$@\"\n", tasksScript))
	}
	if err := os.Remove(tasksPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// writeShim writes body to <bin>/name unless that is already its content.
func writeShim(l Layout, name, body string) error {
	path := filepath.Join(l.BinDir(), name)
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, []byte(body)) {
		return nil
	}
	return os.WriteFile(path, []byte(body), 0o700)
}
