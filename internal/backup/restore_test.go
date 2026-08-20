package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestRestoreUnderNewName(t *testing.T) {
	// Source host: seed + backup.
	src, agentsDir := seed(t)
	as := agent.NewStore(src)
	as.CreateIteration(agent.Iteration{ID: "bot-1-1", Agent: "bot", Trigger: "manual", Status: "done", StartedAt: "2026-07-01T10:00:00Z"})
	l := agentdir.New(agentsDir, "bot")
	l.EnsureIteration("bot-1-1")
	os.WriteFile(l.ResultPath("bot-1-1"), []byte(`{"exit_code":0}`), 0o600)
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}

	// Target host: fresh store + dir.
	dstDir := t.TempDir()
	dst, err := store.Open(filepath.Join(dstDir, "y.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dst.Close() })
	dstAgents := filepath.Join(dstDir, "agents")

	man, err := Restore(bytes.NewReader(arc.Bytes()), dst, dstAgents, RestoreOptions{NewName: "clone"})
	if err != nil {
		t.Fatal(err)
	}
	if man.Agent != "bot" {
		t.Fatalf("manifest agent = %q", man.Agent)
	}
	// The agent exists under the new name.
	das := agent.NewStore(dst)
	if _, err := das.Get("clone"); err != nil {
		t.Fatalf("restored agent missing: %v", err)
	}
	// The iteration id was re-prefixed.
	its, _ := das.ListIterations("clone")
	if len(its) != 1 || its[0].ID != "clone-1-1" {
		t.Fatalf("iterations = %+v", its)
	}
	// The iteration dir was unpacked under the new id.
	if _, err := os.Stat(agentdir.New(dstAgents, "clone").ResultPath("clone-1-1")); err != nil {
		t.Fatalf("iteration file not restored: %v", err)
	}
	// Agent-scoped bus rows were remapped consistently under the new name.
	assertRemappedBus(t, dst, "clone")
	wantScriptLog := filepath.Join(agentdir.New(dstAgents, "clone").Root, "scripts", "clone-run-1.log")
	var restoredScriptLog string
	if err := dst.DB.QueryRow(`SELECT log_path FROM script_runs WHERE agent='clone'`).Scan(&restoredScriptLog); err != nil {
		t.Fatalf("query restored script log path: %v", err)
	}
	if restoredScriptLog != wantScriptLog {
		t.Fatalf("restored script log path=%q, want %q", restoredScriptLog, wantScriptLog)
	}
	if data, err := os.ReadFile(wantScriptLog); err != nil || string(data) != "script output\n" {
		t.Fatalf("renamed script run log not restored: data=%q err=%v", data, err)
	}
	// Restoring again without --force refuses.
	if _, err := Restore(bytes.NewReader(arc.Bytes()), dst, dstAgents, RestoreOptions{NewName: "clone"}); err == nil {
		t.Fatal("restore over existing agent should refuse without --force")
	}
	// With --force it overwrites idempotently.
	if _, err := Restore(bytes.NewReader(arc.Bytes()), dst, dstAgents, RestoreOptions{NewName: "clone", Force: true}); err != nil {
		t.Fatalf("force restore should succeed: %v", err)
	}
	its, _ = das.ListIterations("clone")
	if len(its) != 1 {
		t.Fatalf("force restore should be idempotent, iterations = %+v", its)
	}
}

func TestRestoreUnderNewNameRemapsOpaqueIDsOnSameHost(t *testing.T) {
	s, agentsDir := seed(t)
	if err := agent.NewStore(s).Create(agent.Agent{Name: "opaque", ImageRef: "img:1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO subscriptions(id, agent, channel) VALUES('sub-opaque-1','opaque','opaque.inbox')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO messages(id, channel, ts, produced_by_agent) VALUES('msg-opaque-1','opaque.inbox','2026-01-01T00:00:00Z','opaque')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO deliveries(subscription_id, message_id) VALUES('sub-opaque-1','msg-opaque-1')`); err != nil {
		t.Fatal(err)
	}
	var arc bytes.Buffer
	if _, err := Build(&arc, s, agentsDir, "opaque", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(bytes.NewReader(arc.Bytes()), s, agentsDir, RestoreOptions{NewName: "clone"}); err != nil {
		t.Fatal(err)
	}
	var subID, messageID string
	if err := s.DB.QueryRow(`SELECT id FROM subscriptions WHERE agent='clone'`).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT id FROM messages WHERE produced_by_agent='clone'`).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	if subID == "sub-opaque-1" || messageID == "msg-opaque-1" {
		t.Fatalf("opaque ids were not remapped: subscription=%q message=%q", subID, messageID)
	}
	var deliverySub, deliveryMessage string
	if err := s.DB.QueryRow(`SELECT subscription_id, message_id FROM deliveries WHERE subscription_id=?`, subID).Scan(&deliverySub, &deliveryMessage); err != nil {
		t.Fatal(err)
	}
	if deliverySub != subID || deliveryMessage != messageID {
		t.Fatalf("delivery refs=%q/%q, want %q/%q", deliverySub, deliveryMessage, subID, messageID)
	}
}

func TestRestoreRemapsOutboxThatOutlivedRemovedScript(t *testing.T) {
	src, agentsDir := seed(t)
	if _, err := src.DB.Exec(`DELETE FROM scripts WHERE agent='bot'`); err != nil {
		t.Fatal(err)
	}
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	dst, err := store.Open(filepath.Join(dstDir, "restored.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dst.Close() })
	if _, err := Restore(bytes.NewReader(arc.Bytes()), dst, filepath.Join(dstDir, "agents"), RestoreOptions{NewName: "clone"}); err != nil {
		t.Fatal(err)
	}
	var agentName, runID, key, payload string
	if err := dst.DB.QueryRow(`SELECT agent,run_id,idempotency_key,payload FROM script_result_outbox`).Scan(&agentName, &runID, &key, &payload); err != nil {
		t.Fatal(err)
	}
	if agentName != "clone" || runID == "bot-run-1" || key != "script-result:"+runID || !strings.Contains(payload, `"run_id":"`+runID+`"`) {
		t.Fatalf("restored orphan outbox agent=%q run=%q key=%q payload=%q", agentName, runID, key, payload)
	}
}

func TestRestoreRenameMapsOwnedIDsButPreservesExternalReferencesAndEmptyValues(t *testing.T) {
	s, agentsDir := seed(t)
	if err := agent.NewStore(s).Create(agent.Agent{Name: "opaque", ImageRef: "img:1"}); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO subscriptions(id,agent,channel) VALUES('sub-owned','opaque','opaque.inbox')`)
	exec(`INSERT INTO messages(id,channel,ts,produced_by_agent,produced_in_iteration) VALUES('msg-owned','opaque.inbox','t','opaque','')`)
	exec(`INSERT INTO messages(id,channel,ts,produced_by_agent) VALUES('shared-msg','shared','t','other')`)
	// Occupy the first deterministic rename candidate to force collision-safe
	// allocation for the owned message without rewriting the shared row.
	exec(`INSERT INTO messages(id,channel,ts,produced_by_agent) VALUES('clone-msg-owned','shared','t','other')`)
	exec(`INSERT INTO deliveries(subscription_id,message_id) VALUES('sub-owned','msg-owned')`)
	exec(`INSERT INTO deliveries(subscription_id,message_id) VALUES('sub-owned','shared-msg')`)
	exec(`INSERT INTO ai_requests(id,ts,agent,iteration) VALUES('request-empty','t','opaque','')`)

	var arc bytes.Buffer
	if _, err := Build(&arc, s, agentsDir, "opaque", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(bytes.NewReader(arc.Bytes()), s, agentsDir, RestoreOptions{NewName: "clone"}); err != nil {
		t.Fatal(err)
	}

	var clonedSub, clonedMessage, producedIteration, requestIteration string
	if err := s.DB.QueryRow(`SELECT id FROM subscriptions WHERE agent='clone'`).Scan(&clonedSub); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT id,produced_in_iteration FROM messages WHERE produced_by_agent='clone'`).Scan(&clonedMessage, &producedIteration); err != nil {
		t.Fatal(err)
	}
	if clonedMessage == "msg-owned" || clonedMessage == "clone-msg-owned" {
		t.Fatalf("owned message collision was not remapped safely: %q", clonedMessage)
	}
	if producedIteration != "" {
		t.Fatalf("empty produced_in_iteration remapped to %q", producedIteration)
	}
	if err := s.DB.QueryRow(`SELECT iteration FROM ai_requests WHERE agent='clone'`).Scan(&requestIteration); err != nil {
		t.Fatal(err)
	}
	if requestIteration != "" {
		t.Fatalf("empty ai request iteration remapped to %q", requestIteration)
	}
	rows, err := s.DB.Query(`SELECT message_id FROM deliveries WHERE subscription_id=? ORDER BY message_id`, clonedSub)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var delivered []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		delivered = append(delivered, id)
	}
	if len(delivered) != 2 || !containsTestString(delivered, clonedMessage) || !containsTestString(delivered, "shared-msg") {
		t.Fatalf("cloned deliveries=%v, want owned %q plus shared-msg", delivered, clonedMessage)
	}
	var fkProblems int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&fkProblems); err != nil || fkProblems != 0 {
		t.Fatalf("foreign key check=%d, err=%v", fkProblems, err)
	}
}

func containsTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// assertRemappedBus checks that every agent-scoped bus id was consistently
// re-prefixed old->new and that foreign refs (deliveries) still line up.
func assertRemappedBus(t *testing.T, s *store.Store, newName string) {
	t.Helper()
	q := func(query string, args ...any) string {
		var v string
		if err := s.DB.QueryRow(query, args...).Scan(&v); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return v
	}
	if got := q(`SELECT id FROM subscriptions WHERE agent=?`, newName); got != newName+"-sub-1" {
		t.Fatalf("subscription id not remapped: %q", got)
	}
	if got := q(`SELECT id FROM messages WHERE produced_by_agent=?`, newName); got != newName+"-msg-1" {
		t.Fatalf("message id not remapped: %q", got)
	}
	// The delivery's FK refs must match the remapped subscription/message ids.
	sub := q(`SELECT subscription_id FROM deliveries`)
	msg := q(`SELECT message_id FROM deliveries`)
	if sub != newName+"-sub-1" || msg != newName+"-msg-1" {
		t.Fatalf("delivery refs not remapped consistently: sub=%q msg=%q", sub, msg)
	}
	if got := q(`SELECT id FROM schedules WHERE agent=? ORDER BY id LIMIT 1`, newName); got != newName+"-sch-a" {
		t.Fatalf("schedule id not remapped: %q", got)
	}
	if got := q(`SELECT id FROM scripts WHERE agent=?`, newName); got != newName+"-scr-1" {
		t.Fatalf("script id not remapped: %q", got)
	}
	var runID, scriptID, logPath string
	if err := s.DB.QueryRow(`SELECT id,script_id,log_path FROM script_runs WHERE agent=?`, newName).Scan(&runID, &scriptID, &logPath); err != nil {
		t.Fatalf("query script run: %v", err)
	}
	if runID != newName+"-run-1" || scriptID != newName+"-scr-1" {
		t.Fatalf("script run not remapped: id=%q script_id=%q", runID, scriptID)
	}
	if filepath.Base(logPath) != runID+".log" {
		t.Fatalf("script run log path not remapped: %q", logPath)
	}
	var key, outboxScriptID, outboxRunID, payload string
	if err := s.DB.QueryRow(`SELECT idempotency_key,script_id,run_id,payload FROM script_result_outbox WHERE agent=?`, newName).Scan(&key, &outboxScriptID, &outboxRunID, &payload); err != nil {
		t.Fatalf("query script result outbox: %v", err)
	}
	if key != "script-result:"+runID || outboxScriptID != scriptID || outboxRunID != runID {
		t.Fatalf("script result outbox not remapped: key=%q script_id=%q run_id=%q", key, outboxScriptID, outboxRunID)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decode script result payload: %v", err)
	}
	decodedLogPath, _ := decoded["log_path"].(string)
	if decoded["script_id"] != scriptID || decoded["run_id"] != runID || filepath.Base(decodedLogPath) != runID+".log" {
		t.Fatalf("script result payload not remapped: %+v", decoded)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	// Backup an agent then restore it (same name) onto a fresh host.
	src, agentsDir := seed(t)
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	dst, err := store.Open(filepath.Join(dstDir, "y.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dst.Close() })
	dstAgents := filepath.Join(dstDir, "agents")

	man, err := Restore(bytes.NewReader(arc.Bytes()), dst, dstAgents, RestoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if man.Agent != "bot" {
		t.Fatalf("manifest agent = %q", man.Agent)
	}
	das := agent.NewStore(dst)
	a, err := das.Get("bot")
	if err != nil {
		t.Fatalf("restored agent missing: %v", err)
	}
	if a.ImageRef != "img:1" {
		t.Fatalf("agent config not restored: %+v", a)
	}
	// All bus rows are present with their original ids.
	var n int
	dst.DB.QueryRow(`SELECT COUNT(*) FROM schedules WHERE agent='bot'`).Scan(&n)
	if n != 2 {
		t.Fatalf("schedules not restored: %d", n)
	}
	dst.DB.QueryRow(`SELECT COUNT(*) FROM deliveries`).Scan(&n)
	if n != 1 {
		t.Fatalf("deliveries not restored: %d", n)
	}
	var runLogPath string
	if err := dst.DB.QueryRow(`SELECT log_path FROM script_runs WHERE agent='bot'`).Scan(&runLogPath); err != nil {
		t.Fatalf("script run not restored: %v", err)
	}
	wantRunLogPath := filepath.Join(agentdir.New(dstAgents, "bot").Root, "scripts", "bot-run-1.log")
	if runLogPath != wantRunLogPath {
		t.Fatalf("script run log path=%q, want %q", runLogPath, wantRunLogPath)
	}
	if data, err := os.ReadFile(wantRunLogPath); err != nil || string(data) != "script output\n" {
		t.Fatalf("script run log not restored: data=%q err=%v", data, err)
	}
	// The on-disk agent Root files were unpacked.
	if _, err := os.Stat(filepath.Join(agentdir.New(dstAgents, "bot").Root, "meta.txt")); err != nil {
		t.Fatalf("agent Root file not restored: %v", err)
	}
}

func TestRestoreExcludedSecretAbsent(t *testing.T) {
	// Backup without IncludeSecrets -> secret value must be blank after restore.
	src, agentsDir := seed(t)
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	dstDir := t.TempDir()
	dst, _ := store.Open(filepath.Join(dstDir, "y.db"))
	t.Cleanup(func() { dst.Close() })
	if _, err := Restore(bytes.NewReader(arc.Bytes()), dst, filepath.Join(dstDir, "agents"), RestoreOptions{}); err != nil {
		t.Fatal(err)
	}
	var val string
	if err := dst.DB.QueryRow(`SELECT value FROM secrets WHERE agent='bot' AND key='API_KEY'`).Scan(&val); err != nil {
		t.Fatalf("secret row missing: %v", err)
	}
	if val != "" {
		t.Fatalf("excluded secret value leaked into restore: %q", val)
	}
}

func TestRestoreRejectsNewerSchema(t *testing.T) {
	dstDir := t.TempDir()
	dst, _ := store.Open(filepath.Join(dstDir, "y.db"))
	t.Cleanup(func() { dst.Close() })
	sv, _ := dst.SchemaVersion()
	arc := archiveWith(t, Meta{
		Manifest: Manifest{FormatVersion: FormatVersion, SchemaVersion: sv + 1, Agent: "x"},
		Tables:   map[string][]map[string]any{},
	}, nil)
	if _, err := Restore(bytes.NewReader(arc), dst, filepath.Join(dstDir, "agents"), RestoreOptions{}); err == nil {
		t.Fatal("archive with newer schema must be refused")
	}
}

func TestRestoreRejectsInvalidNewName(t *testing.T) {
	src, agentsDir := seed(t)
	var arc bytes.Buffer
	if _, err := Build(&arc, src, agentsDir, "bot", Options{}, clk); err != nil {
		t.Fatal(err)
	}
	dstDir := t.TempDir()
	dst, _ := store.Open(filepath.Join(dstDir, "y.db"))
	t.Cleanup(func() { dst.Close() })
	if _, err := Restore(bytes.NewReader(arc.Bytes()), dst, filepath.Join(dstDir, "agents"), RestoreOptions{NewName: "../evil"}); err == nil {
		t.Fatal("traversing --name must be rejected")
	}
}

func TestRestoreRejectsTraversal(t *testing.T) {
	dstDir := t.TempDir()
	dst, _ := store.Open(filepath.Join(dstDir, "y.db"))
	t.Cleanup(func() { dst.Close() })
	// A hand-built archive whose file entry escapes the destination.
	arc := maliciousArchive(t) // helper below writes files/../../evil
	if _, err := Restore(bytes.NewReader(arc), dst, filepath.Join(dstDir, "agents"), RestoreOptions{}); err == nil {
		t.Fatal("path-traversal entry must be rejected")
	}
}

func TestRestoreRejectsSymlink(t *testing.T) {
	dstDir := t.TempDir()
	dst, _ := store.Open(filepath.Join(dstDir, "y.db"))
	t.Cleanup(func() { dst.Close() })
	arc := symlinkArchive(t)
	if _, err := Restore(bytes.NewReader(arc), dst, filepath.Join(dstDir, "agents"), RestoreOptions{}); err == nil {
		t.Fatal("symlink entry must be rejected")
	}
}

// --- archive helpers ---------------------------------------------------------

// archiveWith builds a tar.gz with the given meta and extra raw entries.
func archiveWith(t *testing.T, meta Meta, extra []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	write := func(e tarEntry) {
		hdr := &tar.Header{Name: e.name, Mode: 0o600, Size: int64(len(e.data)), ModTime: time.Unix(0, 0), Typeflag: e.typ, Linkname: e.link}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	write(tarEntry{name: "meta.json", data: metaBytes, typ: tar.TypeReg})
	for _, e := range extra {
		write(e)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type tarEntry struct {
	name string
	data []byte
	typ  byte
	link string
}

// maliciousArchive builds a valid meta plus a files/ entry that escapes root.
func maliciousArchive(t *testing.T) []byte {
	t.Helper()
	meta := Meta{
		Manifest: Manifest{FormatVersion: FormatVersion, SchemaVersion: 0, Agent: "x"},
		Tables:   map[string][]map[string]any{},
	}
	return archiveWith(t, meta, []tarEntry{
		{name: "files/../../evil", data: []byte("pwned"), typ: tar.TypeReg},
	})
}

// symlinkArchive builds a valid meta plus a symlink entry (must be rejected).
func symlinkArchive(t *testing.T) []byte {
	t.Helper()
	meta := Meta{
		Manifest: Manifest{FormatVersion: FormatVersion, SchemaVersion: 0, Agent: "x"},
		Tables:   map[string][]map[string]any{},
	}
	return archiveWith(t, meta, []tarEntry{
		{name: "files/link", typ: tar.TypeSymlink, link: "/etc/passwd"},
	})
}
