package loop

import (
	"fmt"
	"os"
	"testing"

	"github.com/alekzonder/tariboy/internal/testutil"
)

func TestMain(m *testing.M) {
	cleanup, err := testutil.IsolateEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The stub harness reads its program from the process environment. Every
	// test that does not need another stub shares this one, so those tests do
	// not need t.Setenv and can run in parallel.
	os.Setenv("TARIBOY_STUB_HARNESS", "/usr/bin/true")
	code := m.Run()
	cleanup()
	os.Exit(code)
}
