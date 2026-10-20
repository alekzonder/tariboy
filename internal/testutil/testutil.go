// Package testutil holds helpers that tests of several packages share.
package testutil

import (
	"os"
	"strings"
	"testing"
)

// IsolateEnv removes every TARIBOY_ variable and points HOME at a new
// temporary directory, so no default path of the code under test can reach
// the live ~/.tariboy, ~/.tariboyd or daemon socket. Call it from TestMain
// before m.Run; cleanup removes the temporary HOME.
func IsolateEnv() (cleanup func(), err error) {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "TARIBOY_") {
			os.Unsetenv(name)
		}
	}
	home, err := os.MkdirTemp("", "tariboy-test-home-")
	if err != nil {
		return nil, err
	}
	os.Setenv("HOME", home)
	return func() { os.RemoveAll(home) }, nil
}

// ShortDir returns a new directory under /tmp, removed when the test ends.
// t.TempDir paths carry the test name and can make a unix socket path longer
// than the OS limit.
func ShortDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tb-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
