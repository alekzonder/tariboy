package judge

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/paths"
)

func putBundle(t *testing.T, base string, b EvidenceBundle) string {
	t.Helper()
	b.BundleHash = ""
	canonical, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	b.BundleHash = hash
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	dir := paths.New(base).JudgeObjectsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, hash+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	g := gzip.NewWriter(f)
	if _, err := g.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestEvidenceReaderSearchGetAndCorruption(t *testing.T) {
	base := t.TempDir()
	auditRows := make([]map[string]any, 205)
	for i := range auditRows {
		auditRows[i] = map[string]any{"seq": i, "text": "MiXeD needle"}
	}
	hash := putBundle(t, base, EvidenceBundle{SchemaVersion: 1, Prompt: EvidenceArtifact{Locator: "prompt", Content: "needle", Present: true}, Audit: auditRows})
	r := NewEvidenceReader(base)
	p, err := r.Search(hash, EvidenceQuery{Artifacts: []string{"AUDIT"}, Query: "nEeDlE", Limit: 500})
	if err != nil || len(p.Results) != 200 || p.NextCursor != "200" {
		t.Fatalf("bounded case-insensitive page=%+v err=%v", p, err)
	}
	p, err = r.Search(hash, EvidenceQuery{Artifacts: []string{"audit"}, Cursor: p.NextCursor, Limit: 200})
	if err != nil || len(p.Results) != 5 || p.NextCursor != "" {
		t.Fatalf("second page=%+v err=%v", p, err)
	}
	if _, err := r.Get(hash, EvidenceLocator{Artifact: "audit", Locator: "204"}); err != nil {
		t.Fatalf("exact stable locator: %v", err)
	}
	for _, l := range []EvidenceLocator{{Artifact: "audit", Locator: "nope"}, {Artifact: "audit", Locator: "../204"}} {
		if _, err := r.Get(hash, l); !errors.Is(err, ErrBadLocator) {
			t.Fatalf("locator %+v err=%v, want ErrBadLocator", l, err)
		}
	}
	path := filepath.Join(paths.New(base).JudgeObjectsDir(), hash+".json.gz")
	if err := os.WriteFile(path, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Manifest(hash); !errors.Is(err, ErrCorruptEvidence) {
		t.Fatalf("gzip corruption err=%v", err)
	}
}

func TestSnapshotRedactsAndSurvivesSourceDeletion(t *testing.T) {
	base := t.TempDir()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "iter-1", "worker", "done", "2026-07-01T10:00:00Z")
	if _, err := db.DB.Exec(`INSERT INTO secrets(agent,key,value) VALUES(?,?,?)`, "worker", "api", "very-secret"); err != nil {
		t.Fatal(err)
	}
	agentsDir := paths.New(base).AgentsDir()
	l := agentdir.New(agentsDir, "worker")
	if err := l.EnsureIteration("iter-1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.PromptPath("iter-1"), []byte("use very-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	audit.Open(l.AuditLog(), nil).Record("note", "system", "iter-1", map[string]any{"token": "very-secret"})
	run, _, err := js.CreateRun(context.Background(), request("iter-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSnapshotter(SnapshotConfig{Store: js, BaseDir: base, AgentsDir: agentsDir}).BuildRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	targets, err := js.ListTargets(run.ID)
	if err != nil || len(targets) != 1 || targets[0].SnapshotStatus != "ready" {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
	var pins int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM judge_retention_pins WHERE target_id=?`, targets[0].ID).Scan(&pins); err != nil || pins != 0 {
		t.Fatalf("successful snapshot did not release pin: pins=%d err=%v", pins, err)
	}
	if err := os.RemoveAll(l.IterationDir("iter-1")); err != nil {
		t.Fatal(err)
	}
	b, err := NewEvidenceReader(base).Manifest(targets[0].BundleHash)
	if err != nil || strings.Contains(stringMustJSON(t, b), "very-secret") || !strings.Contains(b.Prompt.Content, "[REDACTED]") {
		t.Fatalf("snapshot not immutable/redacted: bundle=%+v err=%v", b, err)
	}
	if b.Completeness[0].Status != "present" {
		t.Fatalf("completeness=%+v", b.Completeness)
	}
}

func TestSnapshotMissingPromptAndFailureReleasePin(t *testing.T) {
	base := t.TempDir()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "missing-prompt", "worker", "done", "2026-07-01T10:00:00Z")
	run, _, err := js.CreateRun(context.Background(), request("missing-prompt"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSnapshotter(SnapshotConfig{Store: js, BaseDir: base, AgentsDir: paths.New(base).AgentsDir()})
	if err := s.BuildRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	targets, _ := js.ListTargets(run.ID)
	b, err := NewEvidenceReader(base).Manifest(targets[0].BundleHash)
	if err != nil || b.Completeness[0].Status != "missing" {
		t.Fatalf("missing artifact bundle=%+v err=%v", b, err)
	}
	// A target whose source row disappeared fails after pinning; the deferred
	// cleanup must still release the temporary pin and record failure.
	seedTarget(t, db.DB, "gone", "worker", "done", "2026-07-02T10:00:00Z")
	run, targets, err = js.CreateRun(context.Background(), request("gone"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`DELETE FROM iterations WHERE id=?`, "gone"); err != nil {
		t.Fatal(err)
	}
	if err := s.BuildRun(context.Background(), run.ID); err == nil {
		t.Fatal("expected deleted source to fail snapshot")
	}
	var pins int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM judge_retention_pins WHERE target_id=?`, targets[0].ID).Scan(&pins); err != nil || pins != 0 {
		t.Fatalf("failed snapshot did not release pin: pins=%d err=%v", pins, err)
	}
	var status string
	if err := db.DB.QueryRow(`SELECT snapshot_status FROM judge_targets WHERE id=?`, targets[0].ID).Scan(&status); err != nil || status != "snapshot_failed" {
		t.Fatalf("failure status=%q err=%v", status, err)
	}
}

func stringMustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
