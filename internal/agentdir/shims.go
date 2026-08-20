package agentdir

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alekzonder/tariboy/internal/agent"
)

// WriteShims (re)writes the agent's bin shims so they exec toolsBin — nothing
// else: it neither unpacks the image nor creates the tree, so the bin dir must
// already exist (Provision makes it).
//
// The shims embed an absolute path to one release's tariboy-tools, so an
// agent provisioned by an older daemon keeps calling that old client forever
// unless someone repoints it. The daemon therefore calls this for every stored
// agent at startup, on top of create/reprovision. Writing is skipped when the
// file already holds exactly the wanted bytes, so a restart on the same version
// is a true no-op.
func WriteShims(l Layout, a agent.Agent, toolsBin string) error {
	if err := writeShim(l, "tools", fmt.Sprintf("#!/usr/bin/env bash\nexec %q \"$@\"\n", toolsBin)); err != nil {
		return err
	}
	donePath := filepath.Join(l.BinDir(), "i-am-done")
	if hasCapability(a.Plugins, "loop") {
		if err := writeShim(l, "i-am-done", fmt.Sprintf("#!/usr/bin/env bash\nexec %q loop done \"$@\"\n", toolsBin)); err != nil {
			return err
		}
	} else if err := os.Remove(donePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	tasksPath := filepath.Join(l.BinDir(), "tasks")
	if hasCapability(a.Plugins, "tasks") {
		return writeShim(l, "tasks", fmt.Sprintf("#!/usr/bin/env bash\nexec %q tasks \"$@\"\n", toolsBin))
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
