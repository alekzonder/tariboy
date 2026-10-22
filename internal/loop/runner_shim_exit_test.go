package loop

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
)

// A shim that exits without result.json (for example on an RPC bind failure)
// must end the iteration as harness_error instead of leaving it running.
func TestRunFinishesWhenShimExitsWithoutResult(t *testing.T) {
	t.Parallel()
	r, ag, l, _ := newRunnerForProxyTest(t, &fakeBinder{base: "http://127.0.0.1:5555"})
	r.cfg.ShimBin = falseBin(t)
	r.cfg.Spawner = ExecSpawner{}
	if err := os.MkdirAll(l.Workdir(), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := r.Run(ctx, ag, "manual", "alice-1", "")
	if err != nil {
		t.Fatalf("Run error = %v, want a harness_error outcome", err)
	}
	if out.Status != agent.IterationHarnessError || out.ExitCode != -1 {
		t.Fatalf("outcome = %+v, want harness_error with exit -1", out)
	}
	if !strings.Contains(out.Error, "shim exited before writing result") {
		t.Fatalf("outcome error = %q, want shim exit reason", out.Error)
	}
}

var _ ManagedSpawner = ExecSpawner{}

func falseBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("false")
	if err != nil {
		t.Skipf("false unavailable: %v", err)
	}
	return path
}
