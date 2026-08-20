package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

func clk() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) }

// seedBus inserts one row of each bus table (schedules, script definitions and
// runs, subscriptions, messages produced by the agent, and deliveries for its
// subscription) scoped to ag.
// IDs are prefixed with the agent name so tests can assert agent-scoping. Rows
// are inserted in reverse-id order to exercise the dump's stable ORDER BY.
func seedBus(t *testing.T, s *store.Store, ag string) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatalf("seedBus %s: %v", ag, err)
		}
	}
	// Two schedules inserted b-then-a so ORDER BY id must reorder them.
	exec(`INSERT INTO schedules(id, agent, kind, spec, channel, next_fire_at) VALUES(?,?,?,?,?,?)`,
		ag+"-sch-b", ag, "cron", "* * * * *", ag+".inbox", "2026-01-01T00:00:00Z")
	exec(`INSERT INTO schedules(id, agent, kind, spec, channel, next_fire_at) VALUES(?,?,?,?,?,?)`,
		ag+"-sch-a", ag, "oneshot", "2026-01-01T00:00:00Z", ag+".inbox", "2026-01-01T00:00:00Z")
	exec(`INSERT INTO scripts(id, agent, name, description, command, mode, state, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		ag+"-scr-1", ag, "deploy", "", "echo hi", "once", "completed", "2026-01-01T00:00:00Z")
	exec(`INSERT INTO script_runs(id, script_id, agent, status, exit_code, created_at, started_at, finished_at, log_path) VALUES(?,?,?,?,?,?,?,?,?)`,
		ag+"-run-1", ag+"-scr-1", ag, "succeeded", 0, "2026-01-01T00:00:00Z", "2026-01-01T00:00:01Z", "2026-01-01T00:00:02Z", "/old/agents/"+ag+"/scripts/"+ag+"-run-1.log")
	exec(`INSERT INTO script_result_outbox(idempotency_key, script_id, run_id, agent, payload, next_attempt_at) VALUES(?,?,?,?,?,?)`,
		"script-result:"+ag+"-run-1", ag+"-scr-1", ag+"-run-1", ag, `{"script_id":"`+ag+`-scr-1","run_id":"`+ag+`-run-1","log_path":"/old/agents/`+ag+`/scripts/`+ag+`-run-1.log"}`, "2026-01-01T00:00:02Z")
	exec(`INSERT INTO subscriptions(id, agent, channel) VALUES(?,?,?)`,
		ag+"-sub-1", ag, ag+".inbox")
	exec(`INSERT INTO messages(id, channel, ts, produced_by_agent) VALUES(?,?,?,?)`,
		ag+"-msg-1", ag+".inbox", "2026-01-01T00:00:00Z", ag)
	exec(`INSERT INTO deliveries(subscription_id, message_id) VALUES(?,?)`,
		ag+"-sub-1", ag+"-msg-1")
}

func seed(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "bot", ImageRef: "img:1"})
	as.SecretSet("bot", "API_KEY", "supersecret")
	seedBus(t, s, "bot")
	agentsDir := filepath.Join(dir, "agents")
	l := agentdir.New(agentsDir, "bot")
	os.MkdirAll(l.Workdir(), 0o700)
	os.MkdirAll(filepath.Join(l.Root, "scripts"), 0o700)
	// A file at the agent Root (not workdir) is archived; workdir is excluded.
	os.WriteFile(filepath.Join(l.Root, "meta.txt"), []byte("meta"), 0o600)
	os.WriteFile(filepath.Join(l.Root, "scripts", "bot-run-1.log"), []byte("script output\n"), 0o600)
	os.WriteFile(filepath.Join(l.Workdir(), "huge.bin"), bytes.Repeat([]byte{0}, 1024), 0o600)
	return s, agentsDir
}

func readMeta(t *testing.T, archive []byte) Meta {
	t.Helper()
	gz, _ := gzip.NewReader(bytes.NewReader(archive))
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "meta.json" {
			var m Meta
			if err := json.NewDecoder(tr).Decode(&m); err != nil {
				t.Fatal(err)
			}
			return m
		}
	}
	t.Fatal("meta.json not in archive")
	return Meta{}
}

func TestBuildMasksSecretsExcludesWorkdir(t *testing.T) {
	s, agentsDir := seed(t)
	var buf bytes.Buffer
	man, err := Build(&buf, s, agentsDir, "bot", Options{}, clk)
	if err != nil {
		t.Fatal(err)
	}
	if man.Agent != "bot" || man.SchemaVersion == 0 {
		t.Fatalf("manifest = %+v", man)
	}
	m := readMeta(t, buf.Bytes())
	// secrets row present but value masked
	secs := m.Tables["secrets"]
	if len(secs) != 1 || secs[0]["value"] != "" || secs[0]["key"] != "API_KEY" {
		t.Fatalf("secrets not masked: %+v", secs)
	}
	// workdir file NOT in the archive
	gz, _ := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(h.Name) == "huge.bin" {
			t.Fatal("workdir file leaked into backup")
		}
	}
}

func TestBuildIncludeSecrets(t *testing.T) {
	s, agentsDir := seed(t)
	var buf bytes.Buffer
	if _, err := Build(&buf, s, agentsDir, "bot", Options{IncludeSecrets: true}, clk); err != nil {
		t.Fatal(err)
	}
	m := readMeta(t, buf.Bytes())
	if m.Tables["secrets"][0]["value"] != "supersecret" {
		t.Fatalf("secret value not included: %+v", m.Tables["secrets"])
	}
}

func TestBuildDumpsAllBusTables(t *testing.T) {
	s, agentsDir := seed(t)
	var buf bytes.Buffer
	if _, err := Build(&buf, s, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	m := readMeta(t, buf.Bytes())

	// All agent-scoped durable tables must be present as keys.
	for _, tbl := range []string{"agents", "iterations", "ai_requests", "secrets",
		"subscriptions", "schedules", "scripts", "script_runs", "script_result_outbox", "messages", "deliveries"} {
		if _, ok := m.Tables[tbl]; !ok {
			t.Fatalf("table %q missing from dump", tbl)
		}
	}

	// schedules: both bot rows present, ordered by id (a before b).
	sch := m.Tables["schedules"]
	if len(sch) != 2 || sch[0]["id"] != "bot-sch-a" || sch[1]["id"] != "bot-sch-b" {
		t.Fatalf("schedules not present/ordered: %+v", sch)
	}
	// scripts: the deploy script present.
	scr := m.Tables["scripts"]
	if len(scr) != 1 || scr[0]["name"] != "deploy" || scr[0]["id"] != "bot-scr-1" {
		t.Fatalf("scripts not present: %+v", scr)
	}
	if got := m.Tables["script_runs"]; len(got) != 1 || got[0]["script_id"] != "bot-scr-1" || got[0]["id"] != "bot-run-1" {
		t.Fatalf("script runs not present: %+v", got)
	}
	if got := m.Tables["script_result_outbox"]; len(got) != 1 || got[0]["run_id"] != "bot-run-1" {
		t.Fatalf("script result outbox not present: %+v", got)
	}
	// messages: the message bot produced present.
	msg := m.Tables["messages"]
	if len(msg) != 1 || msg[0]["id"] != "bot-msg-1" || msg[0]["produced_by_agent"] != "bot" {
		t.Fatalf("produced message not present: %+v", msg)
	}
	// deliveries: the delivery for bot's subscription present.
	del := m.Tables["deliveries"]
	if len(del) != 1 || del[0]["subscription_id"] != "bot-sub-1" || del[0]["message_id"] != "bot-msg-1" {
		t.Fatalf("delivery not present: %+v", del)
	}
}

func TestBuildAgentScoping(t *testing.T) {
	s, agentsDir := seed(t)
	// A different agent with its own bus rows must NOT appear in bot's backup.
	as := agent.NewStore(s)
	as.Create(agent.Agent{Name: "other", ImageRef: "img:1"})
	seedBus(t, s, "other")

	var buf bytes.Buffer
	if _, err := Build(&buf, s, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	m := readMeta(t, buf.Bytes())

	has := func(tbl, col, val string) bool {
		for _, r := range m.Tables[tbl] {
			if r[col] == val {
				return true
			}
		}
		return false
	}
	// bot's rows present.
	if !has("schedules", "id", "bot-sch-a") || !has("scripts", "id", "bot-scr-1") ||
		!has("script_runs", "id", "bot-run-1") || !has("script_result_outbox", "run_id", "bot-run-1") ||
		!has("messages", "id", "bot-msg-1") || !has("deliveries", "message_id", "bot-msg-1") {
		t.Fatalf("bot rows missing under scoping: %+v", m.Tables)
	}
	// other's rows absent.
	if has("schedules", "id", "other-sch-a") || has("scripts", "id", "other-scr-1") ||
		has("script_runs", "id", "other-run-1") || has("script_result_outbox", "run_id", "other-run-1") ||
		has("messages", "id", "other-msg-1") || has("deliveries", "message_id", "other-msg-1") {
		t.Fatalf("other agent's rows leaked into bot backup: %+v", m.Tables)
	}
}

// TestBuildSkipsNonRegularFiles reproduces the live-CLI failure where backup
// of an agent with an in-flight iteration failed with "no such device or
// address": the walk read iterations/<id>/shim.sock (a real unix socket) with
// os.ReadFile, which returns ENXIO for sockets. Build must skip non-regular
// files instead of trying to read them.
func TestBuildSkipsNonRegularFiles(t *testing.T) {
	s, agentsDir := seed(t)
	l := agentdir.New(agentsDir, "bot")

	const iterID = "bot-iter-1"
	if err := l.EnsureIteration(iterID); err != nil {
		t.Fatal(err)
	}
	// A regular file alongside the socket, inside the iteration dir.
	promptPath := l.PromptPath(iterID)
	if err := os.WriteFile(promptPath, []byte("do the thing"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A real unix socket at the shim.sock path, simulating a running iteration.
	sockPath := l.ShimSock()
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var buf bytes.Buffer
	if _, err := Build(&buf, s, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatalf("Build failed with a live socket present: %v", err)
	}

	var sawPrompt, sawSock bool
	gz, _ := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch filepath.Base(h.Name) {
		case "PROMPT.md":
			sawPrompt = true
		case "shim.sock":
			sawSock = true
		}
	}
	if !sawPrompt {
		t.Fatal("regular file in iteration dir missing from archive")
	}
	if sawSock {
		t.Fatal("unix socket leaked into backup archive")
	}
}

func TestBuildDeterministic(t *testing.T) {
	s, agentsDir := seed(t)
	var a, b bytes.Buffer
	Build(&a, s, agentsDir, "bot", Options{}, clk)
	Build(&b, s, agentsDir, "bot", Options{}, clk)
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("backup is not byte-deterministic")
	}
}
