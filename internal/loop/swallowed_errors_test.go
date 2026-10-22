package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/script"
)

func TestStartScriptSecretMapFailureDoesNotStartScript(t *testing.T) {
	t.Parallel()
	m, as, _, raw := newManager(t, &fakeRunner{})
	st := script.NewStore(raw, time.Now)
	m.cfg.Scripts = st
	workdir := t.TempDir()
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", Cwd: workdir}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workdir, "started")
	_, r, err := st.CreateOnce("worker", script.CreateOnce{Name: "needs-secrets", Description: "test", Command: "touch " + marker})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.DB.Exec(`DROP TABLE secrets`); err != nil {
		t.Fatal(err)
	}

	m.scripts.startScript(context.Background(), agent.Agent{Name: "worker", Cwd: workdir}, r)
	m.wg.Wait()

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("script started without its secrets (stat err=%v)", err)
	}
	run, err := st.GetRun("worker", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != script.RunFailed {
		t.Fatalf("run status = %q, want %q", run.Status, script.RunFailed)
	}
	body, err := os.ReadFile(run.LogPath)
	if err != nil || !strings.Contains(string(body), "load agent secrets") {
		t.Fatalf("run log = %q (err %v), want the secrets error", body, err)
	}
}

func TestSwapReprovisionedTreeJoinsIdentityRollbackFailure(t *testing.T) {
	t.Parallel()
	m, as, agentsDir, raw := newManager(t, &fakeRunner{})
	defer m.Shutdown()
	name, err := m.Run(registry.RunSpec{ImageRef: "basic:latest", Name: "rollback", Harness: "stub"})
	if err != nil {
		t.Fatal(err)
	}
	ag, err := as.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ag
	candidate.ImageRef = "basic2:latest"
	candidate.GoalWaitCustomerTimeoutS = 0 // Store.Update rejects it after the identity moved.
	// Restoring the original identity must fail too.
	if _, err := raw.DB.Exec(`CREATE TRIGGER block_identity_rollback BEFORE UPDATE OF image_ref ON agents
		WHEN NEW.image_ref = 'basic:latest' BEGIN SELECT RAISE(ABORT, 'rollback blocked'); END`); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "stage")
	l := agentdir.New(agentsDir, name)
	if err := os.CopyFS(stage, os.DirFS(l.ImageDir())); err != nil {
		t.Fatal(err)
	}

	err = m.swapReprovisionedTree(ag, candidate, l, stage)
	if err == nil {
		t.Fatal("swap succeeded, want an error")
	}
	for _, want := range []string{agent.ErrInvalidGoalWaitCustomerTimeout.Error(), "rollback blocked"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}
