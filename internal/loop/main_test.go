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
	code := m.Run()
	cleanup()
	os.Exit(code)
}
