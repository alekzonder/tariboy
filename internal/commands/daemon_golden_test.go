package commands

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/maintenance"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/retention"
)

var updateDaemonGolden = flag.Bool("update-daemon-golden", false, "rewrite testdata/daemon_responses.golden.json")

// daemonGoldenVolatile names keys whose values depend on the wall clock, the
// process, the database schema version or archive bytes.
var daemonGoldenVolatile = map[string]bool{
	"pid": true, "uptime_seconds": true, "schema_version": true,
	"path": true, "sha256": true, "bytes": true, "size_before": true, "size_after": true,
}

// daemonFixedNow is the clock of the fixture's maintenance service.
var daemonFixedNow = time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)

// daemonFixture returns a context with a maintenance service on a fixed
// clock, the retention API, agents bot and old (old has three finished
// iterations), and one proxy transcript entry to reindex. The PATH holds only
// bash, so daemon.status reports the same missing tools on every host.
func daemonFixture(t *testing.T) *registry.Ctx {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	bin := t.TempDir()
	if err := os.Symlink(bash, filepath.Join(bin, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	c, as, _ := ctxWithStore(t)
	c.Version = "v-golden"
	c.HTTPAddr = "127.0.0.1:0"
	c.StartedAt = daemonFixedNow
	c.Maintenance = maintenance.New(c.Store, filepath.Join(c.BaseDir, "maintenance-backups"), func() time.Time { return daemonFixedNow }, nil)
	pol := retention.NewStore(c.Store)
	c.Retention = &retention.RetentionAPI{Policies: pol, Pruner: retention.NewPruner(c.Store, as, pol, agentsDir(c), func() time.Time { return daemonFixedNow }, nil)}
	for _, name := range []string{"bot", "old"} {
		if err := as.Create(agent.Agent{Name: name, ImageRef: "img:1"}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(agentsDir(c), name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"old-1", "old-2", "old-3"} {
		if err := as.CreateIteration(agent.Iteration{ID: id, Agent: "old", Status: "done", StartedAt: daemonFixedNow.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
		if err := agentdir.New(agentsDir(c), "old").EnsureIteration(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := agentdir.New(agentsDir(c), "bot").EnsureIteration("bot-1"); err != nil {
		t.Fatal(err)
	}
	if err := aiproxy.AppendTranscript(agentsDir(c), aiproxy.TranscriptEntry{Meta: aiproxy.AIRequest{ID: "air-1", Agent: "bot", Iteration: "bot-1", TS: "2026-07-14T10:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	return c
}

// Catches a typed daemon, maintenance, retention or backup response changing
// the JSON a client sees.
func TestDaemonResponsesMatchGolden(t *testing.T) {
	c := daemonFixture(t)
	empty, _, _ := ctxWithStore(t)
	emptyPol := retention.NewStore(empty.Store)
	empty.Retention = &retention.RetentionAPI{Policies: emptyPol, Pruner: retention.NewPruner(empty.Store, agent.NewStore(empty.Store), emptyPol, agentsDir(empty), nil, nil)}
	out := t.TempDir()
	type call struct {
		label, path string
		c           *registry.Ctx
		params      registry.Params
	}
	calls := []call{
		{"daemon.status", "daemon.status", c, registry.Params{}},
		{"daemon.config.set", "daemon.config.set", c, registry.Params{"key": "log_level", "value": "debug"}},
		{"daemon.config.get key", "daemon.config.get", c, registry.Params{"key": "log_level"}},
		{"daemon.config.get missing key", "daemon.config.get", c, registry.Params{"key": "absent"}},
		{"daemon.reindex", "daemon.reindex", c, registry.Params{}},
		{"daemon.agent-shell-script.get empty", "daemon.agent-shell-script.get", c, registry.Params{}},
		{"daemon.agent-shell-script.set", "daemon.agent-shell-script.set", c, registry.Params{"script": "export A=1\n"}},
		{"daemon.agent-shell-script.get", "daemon.agent-shell-script.get", c, registry.Params{}},
		{"maintenance.get before run", "maintenance.get", c, registry.Params{}},
		{"maintenance.set", "maintenance.set", c, registry.Params{"retention-days": 120, "time": "04:30"}},
		{"maintenance.run", "maintenance.run", c, registry.Params{}},
		{"maintenance.get after run", "maintenance.get", c, registry.Params{}},
		{"retention.get default", "retention.get", c, registry.Params{"agent": "default"}},
		{"retention.set default", "retention.set", c, registry.Params{"agent": "default", "max-bytes": 500}},
		{"retention.set old", "retention.set", c, registry.Params{"agent": "old", "keep-iterations": 1, "keep-days": 7}},
		{"retention.get old", "retention.get", c, registry.Params{"agent": "old"}},
		{"prune old dry-run", "prune", c, registry.Params{"agent": "old", "dry-run": true}},
		{"prune all dry-run", "prune", c, registry.Params{"agent": "all", "dry-run": true}},
		{"prune all empty", "prune", empty, registry.Params{"agent": "all"}},
		{"backup bot", "backup", c, registry.Params{"target": "bot", "output": filepath.Join(out, "bot.tar.gz")}},
		{"backup bot with secrets", "backup", c, registry.Params{"target": "bot", "output": filepath.Join(out, "bot-secrets.tar.gz"), "include-secrets": true}},
		{"backup all", "backup", c, registry.Params{"target": "all", "output": filepath.Join(out, "all")}},
		{"backup all empty", "backup", empty, registry.Params{"target": "all", "output": filepath.Join(out, "empty")}},
		{"restore new name", "restore", c, registry.Params{"file": filepath.Join(out, "bot.tar.gz"), "name": "clone"}},
		{"restore same name", "restore", c, registry.Params{"file": filepath.Join(out, "bot.tar.gz"), "force": true}},
	}
	recorder := newGoldenRecorder(t, daemonGoldenVolatile, c.BaseDir, "$BASE", empty.BaseDir, "$EMPTY", out, "$OUT")
	for _, call := range calls {
		value, err := h(t, call.path)(call.c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("daemon_responses.golden.json", *updateDaemonGolden)
}
