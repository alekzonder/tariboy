//go:build tmux

package loop

import (
	"os"
	"os/exec"
	"testing"
)

func TestTmuxHasSessionMatchesExactName(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	root, err := os.MkdirTemp("/tmp", "tariboy-loop-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", root)
	defer exec.Command("tmux", "kill-server").Run()
	if out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-s", "image-creator-sam", "/bin/sleep 30").CombinedOutput(); err != nil {
		t.Fatalf("decoy: %v: %s", err, out)
	}
	if tmuxHasSession("image-creator") {
		t.Fatal("prefix matched image-creator-sam")
	}
	if !tmuxHasSession("image-creator-sam") {
		t.Fatal("exact live session missing")
	}
}
