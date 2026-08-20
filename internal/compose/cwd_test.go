package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCwd(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := "/some/compose/dir"
	cases := map[string]string{
		"":          "",
		"$CWD":      dir,
		"$CWD/sub":  filepath.Join(dir, "sub"),
		"$HOME/x":   filepath.Join(home, "x"),
		"${HOME}/x": filepath.Join(home, "x"),
		"~/x":       filepath.Join(home, "x"),
		"/abs/path": "/abs/path",
	}
	for in, want := range cases {
		if got := resolveCwd(in, dir); got != want {
			t.Errorf("resolveCwd(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEffectiveCwd(t *testing.T) {
	dir := "/some/compose/dir"
	cases := map[string]string{
		"":         dir,                       // default: the compose file's dir
		"$CWD":     dir,                       // explicit token, same result
		"$CWD/sub": filepath.Join(dir, "sub"), // explicit still resolves
		"/abs":     "/abs",
	}
	for in, want := range cases {
		if got := effectiveCwd(in, dir); got != want {
			t.Errorf("effectiveCwd(%q)=%q want %q", in, got, want)
		}
	}
	// No compose dir -> no default to impose.
	if got := effectiveCwd("", ""); got != "" {
		t.Errorf("effectiveCwd(\"\", \"\")=%q want \"\"", got)
	}
}
