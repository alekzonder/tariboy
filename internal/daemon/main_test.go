package daemon

import (
	"fmt"
	"os"
	"testing"

	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/alekzonder/tariboy/internal/testutil"
)

func TestMain(m *testing.M) {
	// The test binary stands in for tariboy-shim when a script run starts.
	if len(os.Args) >= 3 && os.Args[1] == shim.ScriptRunMode && os.Args[2] == "--" {
		if shim.RunScript(os.Args[3:]) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	cleanup, err := testutil.IsolateEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
