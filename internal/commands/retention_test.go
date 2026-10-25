package commands

import (
	"encoding/json"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/retention"
)

func TestRetentionCommands(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t) // returns (*registry.Ctx, *agent.Store, *fakeControl)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	pol := retention.NewStore(c.Store)
	c.Retention = &retention.RetentionAPI{
		Policies: pol,
		Pruner:   retention.NewPruner(c.Store, as, pol, t.TempDir(), nil, nil),
	}

	// set per-agent policy
	if _, err := h(t, "retention.set")(c, registry.Params{
		"agent": "bot", "keep-iterations": 3, "keep-days": 7,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := h(t, "retention.get")(c, registry.Params{"agent": "bot"})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(RetentionView)
	if m.KeepIterations != 3 || m.KeepDays != 7 {
		t.Fatalf("get = %+v", m)
	}
	// set the daemon default
	if _, err := h(t, "retention.set")(c, registry.Params{"agent": "default", "keep-iterations": 10, "max-bytes": 500}); err != nil {
		t.Fatal(err)
	}
	def, _ := pol.Default()
	if def.KeepIterations != 10 {
		t.Fatalf("default = %+v", def)
	}
	// The agent's non-zero fields override the default; its zero max_bytes
	// inherits. The agent's own record and the default are reported too.
	got, _ = h(t, "retention.get")(c, registry.Params{"agent": "bot"})
	m = got.(RetentionView)
	if m.KeepIterations != 3 || m.MaxBytes != 500 {
		t.Fatalf("effective = %+v", m)
	}
	if o := m.Override; o == nil || o.KeepIterations != 3 || o.MaxBytes != 0 {
		t.Fatalf("override = %+v", o)
	}
	if d := m.Default; d == nil || d.KeepIterations != 10 {
		t.Fatalf("default = %+v", d)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["archive"]; ok {
		t.Fatalf("archive flag still reported: %s", data)
	}
	// prune (empty agent tree -> zero victims, no error)
	pr, err := h(t, "prune")(c, registry.Params{"agent": "bot", "dry-run": true})
	if err != nil {
		t.Fatal(err)
	}
	if !pr.(PruneResult).DryRun {
		t.Fatalf("prune = %+v", pr)
	}
}

func TestRetentionSetRejectsNegative(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	pol := retention.NewStore(c.Store)
	c.Retention = &retention.RetentionAPI{
		Policies: pol,
		Pruner:   retention.NewPruner(c.Store, as, pol, t.TempDir(), nil, nil),
	}

	cases := []registry.Params{
		{"agent": "bot", "keep-iterations": -5},
		{"agent": "bot", "keep-days": -1},
		{"agent": "bot", "max-bytes": -100},
	}
	for _, p := range cases {
		if _, err := h(t, "retention.set")(c, p); err == nil {
			t.Fatalf("retention.set(%v): expected error for negative value, got nil", p)
		}
	}
	// Nothing was persisted for the agent.
	if _, ok, _ := pol.Get("bot"); ok {
		t.Fatal("negative retention values were persisted")
	}
}

func TestRetentionBadAgentName(t *testing.T) {
	t.Parallel()
	c, as, _ := ctxWithStore(t)
	pol := retention.NewStore(c.Store)
	c.Retention = &retention.RetentionAPI{
		Policies: pol,
		Pruner:   retention.NewPruner(c.Store, as, pol, t.TempDir(), nil, nil),
	}

	for _, path := range []string{"retention.get", "retention.set", "prune"} {
		if _, err := h(t, path)(c, registry.Params{"agent": "../../etc/passwd"}); err == nil {
			t.Fatalf("%s: expected error for path-traversal agent name, got nil", path)
		}
	}
}
