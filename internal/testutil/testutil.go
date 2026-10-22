// Package testutil holds helpers that tests of several packages share.
package testutil

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

var realHome string

// RealHome returns the HOME captured by IsolateEnv, for nested Go builds only.
func RealHome() string { return realHome }

// IsolateEnv removes every TARIBOY_ variable and points HOME at a new
// temporary directory, so no default path of the code under test can reach
// the live ~/.tariboy, ~/.tariboyd or daemon socket. Call it from TestMain
// before m.Run; cleanup removes the temporary HOME.
func IsolateEnv() (cleanup func(), err error) {
	realHome = os.Getenv("HOME")
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

// WaitFor checks cond until it returns true or timeout passes, and reports
// whether it returned true. The caller decides how to fail, so its message
// can show the state it waited for.
func WaitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		// The one polling interval of the suite: state owned by another
		// goroutine or process has no event a test could wait on instead.
		time.Sleep(5 * time.Millisecond)
	}
}

// WriteExecutable writes a script that a test runs next. It holds
// syscall.ForkLock while the file is open, so a parallel test cannot fork a
// child that keeps the write descriptor and makes the exec fail with "text
// file busy" (golang.org/issue/22315).
func WriteExecutable(path string, data []byte, perm os.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(path, data, perm)
}
