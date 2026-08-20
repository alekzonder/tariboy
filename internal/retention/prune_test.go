package retention

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

func mkIter(t *testing.T, as *agent.Store, agentsDir, name, id, started string) {
	t.Helper()
	if err := as.CreateIteration(agent.Iteration{ID: id, Agent: name, Trigger: "manual", Status: "done", StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(agentsDir, name)
	if err := l.EnsureIteration(id); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.ResultPath(id), []byte(`{"exit_code":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPruneKeepsNewestByCount(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	if err := as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"}); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(dir, "agents")
	// Three iterations oldest->newest.
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-02T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-3", "2026-07-03T10:00:00Z")

	ps := NewStore(s)
	ps.Set("bot", Policy{KeepIterations: 1, Archive: true})
	clk := func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }
	pr := NewPruner(s, as, ps, agentsDir, clk, discardLog())

	// Dry-run: reports victims, changes nothing.
	rep, err := pr.PruneAgent("bot", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 2 || !rep.DryRun {
		t.Fatalf("dry-run report = %+v", rep)
	}
	if its, _ := as.ListIterations("bot"); len(its) != 3 {
		t.Fatal("dry-run deleted rows")
	}

	// Real prune: keeps only the newest, archives the two victims.
	rep, err = pr.PruneAgent("bot", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 2 || len(rep.Archived) != 2 {
		t.Fatalf("prune report = %+v", rep)
	}
	its, _ := as.ListIterations("bot")
	if len(its) != 1 || its[0].ID != "bot-1-3" {
		t.Fatalf("remaining iterations = %+v", its)
	}
	// The victim dirs are gone; archives exist.
	if _, err := os.Stat(agentdir.New(agentsDir, "bot").IterationDir("bot-1-1")); !os.IsNotExist(err) {
		t.Fatal("victim dir not removed")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "bot", "archive", "bot-1-1.tar.gz")); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}

func TestPruneNeverRemovesRunning(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(filepath.Join(dir, "x.db"))
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	// Mark a second one running (old but active).
	as.CreateIteration(agent.Iteration{ID: "bot-1-2", Agent: "bot", Trigger: "manual", Status: "running", StartedAt: "2026-07-01T11:00:00Z"})
	agentdir.New(agentsDir, "bot").EnsureIteration("bot-1-2")
	mkIter(t, as, agentsDir, "bot", "bot-1-3", "2026-07-03T10:00:00Z")

	ps := NewStore(s)
	ps.Set("bot", Policy{KeepIterations: 1, Archive: false})
	pr := NewPruner(s, as, ps, agentsDir, func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }, discardLog())
	rep, err := pr.PruneAgent("bot", false)
	if err != nil {
		t.Fatal(err)
	}
	// Only bot-1-1 is a victim: bot-1-3 is newest (protected), bot-1-2 is running.
	if len(rep.Pruned) != 1 || rep.Pruned[0] != "bot-1-1" {
		t.Fatalf("report = %+v", rep)
	}
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// untarGz reads the archive at path and returns rel-path -> content.
func untarGz(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[hdr.Name] = string(data)
	}
	return out
}

// TestArchiveRoundTrips: the tar.gz untars to the original iteration contents.
func TestArchiveRoundTrips(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-02T10:00:00Z")
	// Write an extra nested log file into the victim to exercise dir capture.
	l := agentdir.New(agentsDir, "bot")
	if err := os.WriteFile(l.HarnessStdout("bot-1-1"), []byte("hello-log"), 0o600); err != nil {
		t.Fatal(err)
	}

	ps := NewStore(s)
	ps.Set("bot", Policy{KeepIterations: 1, Archive: true})
	clk := func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }
	pr := NewPruner(s, as, ps, agentsDir, clk, discardLog())
	if _, err := pr.PruneAgent("bot", false); err != nil {
		t.Fatal(err)
	}

	got := untarGz(t, filepath.Join(agentsDir, "bot", "archive", "bot-1-1.tar.gz"))
	if got["result.json"] != `{"exit_code":0}` {
		t.Fatalf("result.json content = %q", got["result.json"])
	}
	if got["logs/harness.stdout.log"] != "hello-log" {
		t.Fatalf("nested log content = %q, entries=%v", got["logs/harness.stdout.log"], got)
	}
}

// TestPruneDeletesAIRequestRowsConsistently: victim's ai_requests rows are
// removed with the iterations row; a kept iteration's rows survive.
func TestPruneDeletesAIRequestRowsConsistently(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-02T10:00:00Z")
	// One ai_requests row per iteration.
	for _, r := range []struct{ id, iter string }{{"r1", "bot-1-1"}, {"r2", "bot-1-2"}} {
		if _, err := s.DB.Exec(`INSERT INTO ai_requests(id, ts, agent, iteration) VALUES(?,?,?,?)`,
			r.id, "2026-07-01T10:00:00Z", "bot", r.iter); err != nil {
			t.Fatal(err)
		}
	}

	ps := NewStore(s)
	ps.Set("bot", Policy{KeepIterations: 1, Archive: false})
	pr := NewPruner(s, as, ps, agentsDir, func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }, discardLog())
	if _, err := pr.PruneAgent("bot", false); err != nil {
		t.Fatal(err)
	}

	count := func(iter string) int {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ai_requests WHERE iteration=?`, iter).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("bot-1-1") != 0 {
		t.Fatal("victim ai_requests rows not deleted")
	}
	if count("bot-1-2") != 1 {
		t.Fatal("kept ai_requests rows were removed")
	}
}

// TestPruneKeepDays: iterations older than the cutoff are victims; newest and
// in-window iterations survive.
func TestPruneKeepDays(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-06-01T10:00:00Z") // old
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-04T10:00:00Z") // within 7d
	mkIter(t, as, agentsDir, "bot", "bot-1-3", "2026-07-05T10:00:00Z") // newest

	ps := NewStore(s)
	ps.Set("bot", Policy{KeepDays: 7, Archive: false})
	clk := func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }
	pr := NewPruner(s, as, ps, agentsDir, clk, discardLog())
	rep, err := pr.PruneAgent("bot", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 1 || rep.Pruned[0] != "bot-1-1" {
		t.Fatalf("keep_days report = %+v", rep)
	}
}

// TestPruneMaxBytes: once cumulative size (newest-first) exceeds max_bytes,
// older iterations are victims; newest is always protected.
func TestPruneMaxBytes(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	agentsDir := filepath.Join(dir, "agents")
	// Each mkIter writes result.json = 15 bytes.
	mkIter(t, as, agentsDir, "bot", "bot-1-1", "2026-07-01T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-2", "2026-07-02T10:00:00Z")
	mkIter(t, as, agentsDir, "bot", "bot-1-3", "2026-07-03T10:00:00Z")

	ps := NewStore(s)
	// Budget 10 bytes over prunable (non-protected) iterations: the newest is
	// always kept and not counted; bot-1-2 (15) and bot-1-1 (cum 30) both
	// exceed 10, so both are victims.
	ps.Set("bot", Policy{MaxBytes: 10, Archive: false})
	clk := func() time.Time { return time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC) }
	pr := NewPruner(s, as, ps, agentsDir, clk, discardLog())
	rep, err := pr.PruneAgent("bot", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Pruned) != 2 || rep.Pruned[0] != "bot-1-1" || rep.Pruned[1] != "bot-1-2" {
		t.Fatalf("max_bytes report = %+v", rep)
	}
}
