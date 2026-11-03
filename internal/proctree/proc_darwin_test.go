//go:build darwin

package proctree

import "testing"

// macOS /bin/sh is bash, which replaces itself with the last simple command of
// -c; the trailing command keeps the shell alive as the child.
func TestReadFindsLiveShellCommandDarwin(t *testing.T) {
	testReadFindsShellCommand(t, "sleep 30; exit 0")
}
