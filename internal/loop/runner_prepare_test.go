package loop

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// urlOnlyBinder hides fakeBinder's ProxyBaseURLForToken so prepare falls back
// to the plain ProxyBaseURL, which an empty base leaves unavailable.
type urlOnlyBinder struct{ ProxyBinder }

// TestPrepareRevokesMintedTokenExactlyOnceOnFailure drives prepare failures
// that happen after the proxy token was minted. prepare still owns the token on
// those paths, so it must revoke it exactly once before returning the error.
func TestPrepareRevokesMintedTokenExactlyOnceOnFailure(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *ShimRunner, binder *fakeBinder)
	}{
		{
			name: "goal attribution not persisted",
			setup: func(t *testing.T, r *ShimRunner, binder *fakeBinder) {
				binder.failUpdate = true
				r.cfg.CurrentGoal = func(string, time.Time) (tasks.Task, bool, error) {
					return tasks.Task{Key: "SUPER-1"}, true, nil
				}
			},
		},
		{
			name: "empty proxy base URL",
			setup: func(t *testing.T, r *ShimRunner, binder *fakeBinder) {
				binder.base = ""
				r.cfg.Proxy = urlOnlyBinder{binder}
			},
		},
		{
			name: "harness executable not found",
			setup: func(t *testing.T, r *ShimRunner, binder *fakeBinder) {
				t.Setenv("TARIBOY_STUB_HARNESS", filepath.Join(t.TempDir(), "missing-stub"))
			},
		},
		{
			name: "python3 not found",
			setup: func(t *testing.T, r *ShimRunner, binder *fakeBinder) {
				dir := t.TempDir()
				writeHarnessExecutable(t, filepath.Join(dir, "bash"))
				t.Setenv("PATH", dir)
			},
		},
		{
			name: "bash not found",
			setup: func(t *testing.T, r *ShimRunner, binder *fakeBinder) {
				dir := t.TempDir()
				writeHarnessExecutable(t, filepath.Join(dir, "python3"))
				t.Setenv("PATH", dir)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			binder := &fakeBinder{base: "http://127.0.0.1:5555"}
			r, ag, l, _ := newRunnerForProxyTest(t, binder)
			preparePath(t)
			tc.setup(t, r, binder)
			if err := l.EnsureIteration("alice-1"); err != nil {
				t.Fatal(err)
			}

			if _, err := r.prepare(context.Background(), otel.Tracer("test"), ag, l, "alice-1", ""); err == nil {
				t.Fatal("prepare succeeded, want failure after token mint")
			}
			if len(binder.minted) != 1 {
				t.Fatalf("minted tokens = %v, want exactly one", binder.minted)
			}
			if got := revokeCount(binder, binder.minted[0]); got != 1 || len(binder.revoked) != 1 {
				t.Fatalf("revoked = %v, want minted token %q revoked exactly once", binder.revoked, binder.minted[0])
			}
		})
	}
}

// TestPrepareLeavesMintedTokenToCallerOnSuccess asserts that a successful
// prepare hands the live token to Run without revoking it.
func TestPrepareLeavesMintedTokenToCallerOnSuccess(t *testing.T) {
	binder := &fakeBinder{base: "http://127.0.0.1:5555"}
	r, ag, l, _ := newRunnerForProxyTest(t, binder)
	preparePath(t)
	if err := l.EnsureIteration("alice-1"); err != nil {
		t.Fatal(err)
	}

	prep, err := r.prepare(context.Background(), otel.Tracer("test"), ag, l, "alice-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(binder.minted) != 1 || prep.proxyToken != binder.minted[0] {
		t.Fatalf("prepared token = %q, minted = %v", prep.proxyToken, binder.minted)
	}
	if len(binder.revoked) != 0 {
		t.Fatalf("prepare revoked %v on success, want none", binder.revoked)
	}
}

// preparePath points PATH at a directory holding stub python3 and bash so the
// preflight does not depend on the host.
func preparePath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeHarnessExecutable(t, filepath.Join(dir, "python3"))
	writeHarnessExecutable(t, filepath.Join(dir, "bash"))
	t.Setenv("PATH", dir)
}

func revokeCount(binder *fakeBinder, token string) int {
	n := 0
	for _, tok := range binder.revoked {
		if tok == token {
			n++
		}
	}
	return n
}
