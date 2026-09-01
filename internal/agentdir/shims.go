package agentdir

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
	if err := RequirePython3(); err != nil {
		return err
	}
	doneScript := filepath.Join(skillsDir, "loop", "scripts", "loop.sh")
	tasksScript := filepath.Join(skillsDir, "tasks", "scripts", "tasks.sh")
	var required []string
	if hasCapability(a.Plugins, "loop") {
		required = append(required, doneScript)
	}
	if hasCapability(a.Plugins, "tasks") {
		required = append(required, tasksScript)
	}
	for _, path := range required {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("skill script unavailable: %s", path)
		}
	}
	if err := removeManagedLegacyToolsShim(l); err != nil {
		return err
	}
	donePath := filepath.Join(l.BinDir(), "i-am-done")
	if hasCapability(a.Plugins, "loop") {
		if err := writeShim(l, "i-am-done", fmt.Sprintf("#!/usr/bin/env bash\nexec %q done \"$@\"\n", doneScript)); err != nil {
			return err
		}
	} else if err := os.Remove(donePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	tasksPath := filepath.Join(l.BinDir(), "tasks")
	if hasCapability(a.Plugins, "tasks") {
		return writeShim(l, "tasks", fmt.Sprintf("#!/usr/bin/env bash\nexec %q \"$@\"\n", tasksScript))
	}
	if err := os.Remove(tasksPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// removeManagedLegacyToolsShim removes only the regular-file dispatcher shim
// written by pre-TARI-41 releases. A user-owned bin/tools remains untouched.
func removeManagedLegacyToolsShim(l Layout) error {
	path := filepath.Join(l.BinDir(), "tools")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Contains(body, []byte("agent-tools/scripts/tools.py")) && bytes.Contains(body, []byte("exec python3")) {
		return os.Remove(path)
	}
	return nil
}

func RequirePython3() error {
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("python3 is required for agent tool scripts: %w", err)
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
