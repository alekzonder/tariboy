package commands

import (
	"encoding/json"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/registry"
)

// compose up reads GET /api/agents into its own typed rows. Pin that every
// field it reads is present in agent.ps output and decodes into its type, so a
// renamed or dropped field fails here instead of reading as drift on every up.
func TestAgentPsOutputSatisfiesComposeLiveView(t *testing.T) {
	t.Parallel()
	c, as, fc := ctxWithStore(t)
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", HarnessType: "stub", Enabled: true,
		Group: "", Cwd: t.TempDir(), TimeoutS: 60, IntervalS: 30, OnTimeout: "restart", OnError: "stop", MaxIdleIterations: 2}); err != nil {
		t.Fatal(err)
	}
	fc.live = "running"
	res, err := h(t, "agent.ps")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := compose.CheckAgentListContract(raw); err != nil {
		t.Fatal(err)
	}
}
