package loop

import (
	"os"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

// removeFixture runs one agent with durable history: CONTEXT.md, audit.jsonl,
// an iteration directory and row with a tag, and the seeded side-table rows.
func removeFixture(t *testing.T, name string) (*Manager, *agent.Store, agentdir.Layout, *store.Store) {
	t.Helper()
	r := &fakeRunner{outcomes: []Outcome{{Status: "done", DoneFlag: true}}}
	m, as, agentsDir, s := newManager(t, r)
	t.Cleanup(m.Shutdown)
	if _, err := m.Run(registry.RunSpec{ImageRef: "basic:latest", Name: name,
		Harness: "stub", Plugins: []string{"context"}}); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(agentsDir, name)
	mustWrite(t, l.ContextPath(), "remember me")
	mustWrite(t, l.AuditLog(), "{\"type\":\"status\"}\n")
	if err := os.MkdirAll(l.IterationDir("iter-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := as.CreateIteration(agent.Iteration{ID: "iter-1", Agent: name,
		Trigger: "manual", Status: "done", StartedAt: "2026-07-13T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := as.MutateIterationTags(name, agent.TagOpAdd, []string{"iter-1"}, []string{"good"}); err != nil {
		t.Fatal(err)
	}
	seedLeakedRows(t, s, name)
	return m, as, l, s
}

func countRows(t *testing.T, s *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// TestRemoveDefaultDeletesAgentKeepsHistory is the defect: a plain remove
// must delete the agent row, and keep iterations, their tags, ai_requests,
// CONTEXT.md, audit.jsonl and the iteration directory.
func TestRemoveDefaultDeletesAgentKeepsHistory(t *testing.T) {
	t.Parallel()
	m, as, l, s := removeFixture(t, "gone")

	if err := m.Remove("gone", true, store.RemoveModeRemove); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if ok, _ := as.Exists("gone"); ok {
		t.Fatal("remove kept the agents row")
	}
	if its, _ := as.ListIterations("gone"); len(its) != 1 {
		t.Fatalf("remove dropped iteration rows: got %d, want 1", len(its))
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM iteration_tags WHERE iteration_id = 'iter-1'`); n != 1 {
		t.Fatalf("remove dropped iteration tags: %d", n)
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM ai_requests WHERE agent = 'gone'`); n != 1 {
		t.Fatalf("remove dropped ai_requests: %d", n)
	}
	for _, p := range []string{l.ContextPath(), l.AuditLog(), l.IterationDir("iter-1")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("remove dropped %s: %v", p, err)
		}
	}
	for _, d := range []string{l.ImageDir(), l.BinDir(), l.Workdir()} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Fatalf("remove kept rebuildable dir %s (err=%v)", d, err)
		}
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM schedules WHERE agent = 'gone'`,
		`SELECT COUNT(*) FROM scripts WHERE agent = 'gone'`,
		`SELECT COUNT(*) FROM script_result_outbox WHERE agent = 'gone'`,
		`SELECT COUNT(*) FROM retention_policies WHERE agent = 'gone'`,
		`SELECT COUNT(*) FROM budgets WHERE scope = 'agent:gone'`,
		`SELECT COUNT(*) FROM mail WHERE recipient = 'agent:gone'`,
	} {
		if n := countRows(t, s, q); n != 0 {
			t.Fatalf("%s = %d after remove, want 0", q, n)
		}
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM mail WHERE id = 'm-sent'`); n != 1 {
		t.Fatalf("remove deleted the letter the agent sent: %d", n)
	}
	if st, err := m.LiveState("gone"); err == nil {
		t.Fatalf("LiveState after remove = %q, want an error", st)
	}
}

// TestRemoveKeepLeavesStoppedRow is the compose down mode: the row stays
// stopped with all history, only the rebuildable tree goes.
func TestRemoveKeepLeavesStoppedRow(t *testing.T) {
	t.Parallel()
	m, as, l, s := removeFixture(t, "kept")
	before := leakedRowCount(t, s, "kept")

	if err := m.Remove("kept", true, store.RemoveModeKeep); err != nil {
		t.Fatalf("keep remove: %v", err)
	}
	got, err := as.Get("kept")
	if err != nil || got.Enabled {
		t.Fatalf("keep remove row = %+v, %v; want a stopped row", got, err)
	}
	if n := leakedRowCount(t, s, "kept"); n != before {
		t.Fatalf("keep remove changed side-table rows: %d, want %d", n, before)
	}
	if _, err := os.Stat(l.ContextPath()); err != nil {
		t.Fatalf("keep remove dropped CONTEXT.md: %v", err)
	}
	if _, err := os.Stat(l.ImageDir()); !os.IsNotExist(err) {
		t.Fatalf("keep remove kept the image tree (err=%v)", err)
	}
}

// TestRemoveDefaultDeletesRowLeftByOldVersion covers an agent that an older
// version "removed": the row is stopped and the image tree is gone.
func TestRemoveDefaultDeletesRowLeftByOldVersion(t *testing.T) {
	t.Parallel()
	m, as, l, _ := removeFixture(t, "legacy")
	if err := m.Remove("legacy", true, store.RemoveModeKeep); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("legacy", true, store.RemoveModeRemove); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if ok, _ := as.Exists("legacy"); ok {
		t.Fatal("second remove kept the agents row")
	}
	if _, err := os.Stat(l.ContextPath()); err != nil {
		t.Fatalf("second remove dropped CONTEXT.md: %v", err)
	}
}

// TestRemoveDefaultRejectsPoolMemberBeforeStopping proves the plain remove
// checks pool membership before it touches the agent.
func TestRemoveDefaultRejectsPoolMemberBeforeStopping(t *testing.T) {
	t.Parallel()
	m, as, l, s := removeFixture(t, "pooled")
	if err := m.Start("pooled"); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES ('DAO','DAO','t','t')`,
		`INSERT INTO task_agent_pools(id,queue_prefix,name,created_at,updated_at) VALUES (1,'DAO','developers','t','t')`,
		`INSERT INTO task_agent_pool_members(pool_id,agent,position) VALUES (1,'pooled',0)`,
	} {
		if _, err := s.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	err := m.Remove("pooled", true, store.RemoveModeRemove)
	if err == nil || !strings.Contains(err.Error(), "agent pool developers in queue DAO") {
		t.Fatalf("remove of a pool member = %v, want a pool error", err)
	}
	if got, _ := as.Get("pooled"); !got.Enabled {
		t.Fatal("rejected remove stopped the agent")
	}
	if _, err := os.Stat(l.Workdir()); err != nil {
		t.Fatalf("rejected remove dropped the workdir: %v", err)
	}
	if m.runtimes.runtime("pooled") == nil {
		t.Fatal("rejected remove tore down the runtime")
	}
}

// TestRemovePurgeDeletesLeftoversOfRemovedAgent proves --purge still deletes
// the history a plain remove kept, although the agents row is gone.
func TestRemovePurgeDeletesLeftoversOfRemovedAgent(t *testing.T) {
	t.Parallel()
	m, as, l, s := removeFixture(t, "rest")
	if err := m.Remove("rest", true, store.RemoveModeRemove); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("rest", true, store.RemoveModePurge); err != nil {
		t.Fatalf("purge of leftovers: %v", err)
	}
	if its, _ := as.ListIterations("rest"); len(its) != 0 {
		t.Fatalf("purge kept iteration rows: %d", len(its))
	}
	if n := countRows(t, s, `SELECT COUNT(*) FROM ai_requests WHERE agent = 'rest'`); n != 0 {
		t.Fatalf("purge kept ai_requests: %d", n)
	}
	if _, err := os.Stat(l.Root); !os.IsNotExist(err) {
		t.Fatalf("purge kept the agent directory (err=%v)", err)
	}
	if err := m.Remove("rest", true, store.RemoveModePurge); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("purge with nothing left = %v, want not found", err)
	}
}

// TestRunAfterRemoveReusesHistory proves an agent created under the name of a
// removed one starts and inherits the kept history.
func TestRunAfterRemoveReusesHistory(t *testing.T) {
	t.Parallel()
	m, as, l, _ := removeFixture(t, "again")
	if err := m.Remove("again", true, store.RemoveModeRemove); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(registry.RunSpec{ImageRef: "basic:latest", Name: "again",
		Harness: "stub", Plugins: []string{"context"}}); err != nil {
		t.Fatalf("run after remove: %v", err)
	}
	if ok, _ := as.Exists("again"); !ok {
		t.Fatal("run after remove did not create the agent")
	}
	if b, err := os.ReadFile(l.ContextPath()); err != nil || string(b) != "remember me" {
		t.Fatalf("CONTEXT.md after run = %q, %v; want the kept one", b, err)
	}
	if its, _ := as.ListIterations("again"); len(its) != 1 {
		t.Fatalf("iterations after run = %d, want the kept one", len(its))
	}
}
