package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

func TestNewManagerIgnoresRuntimeDirEnvironment(t *testing.T) {
	envRuntime := filepath.Join(t.TempDir(), "env-runtime")
	t.Setenv("TARIBOY_RUNTIME_DIR", envRuntime)
	agentsDir := filepath.Join(t.TempDir(), "agents")

	m := NewManager(ManagerConfig{AgentsDir: agentsDir})

	if m.cfg.RuntimeDir != "" {
		t.Fatalf("RuntimeDir = %q, want empty: sockets stay in the agent directory", m.cfg.RuntimeDir)
	}
	if _, err := os.Stat(envRuntime); !os.IsNotExist(err) {
		t.Fatalf("NewManager created the TARIBOY_RUNTIME_DIR directory: %v", err)
	}
}

func TestScriptRuntimeUsesConfiguredBaseEnv(t *testing.T) {
	m, as, _, _ := newManager(t, &fakeRunner{})
	m.cfg.BaseEnv = []string{"PATH=/usr/bin:/bin", "BASE_ENV_MARKER=config"}
	t.Setenv("PROCESS_ENV_MARKER", "process")
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	_, env, err := m.ScriptRuntime("worker")
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	if !strings.Contains(joined, "\nBASE_ENV_MARKER=config\n") {
		t.Fatalf("script env lacks the configured base: %v", env)
	}
	if strings.Contains(joined, "PROCESS_ENV_MARKER") {
		t.Fatalf("script env carries the process environment: %v", env)
	}
}
