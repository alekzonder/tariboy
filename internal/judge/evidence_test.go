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
	"github.com/alekzonder/tariboy/internal/aiproxy"
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

func putLegacyBundle(t *testing.T, base, canonical string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(canonical))
	hash := hex.EncodeToString(sum[:])
	raw := strings.Replace(canonical, `"bundle_hash":""`, `"bundle_hash":"`+hash+`"`, 1)
	dir := paths.New(base).JudgeObjectsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, hash+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	g := gzip.NewWriter(f)
	if _, err := g.Write([]byte(raw)); err != nil {
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

func transcriptMap(t *testing.T, entry aiproxy.TranscriptEntry) map[string]any {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	out["request_id"] = entry.Meta.ID
	return out
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

func TestSnapshotBuildsEvidenceV2WithTaskAndImageProvenance(t *testing.T) {
	base := t.TempDir()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "iter-v2", "worker", "done", "2026-07-01T10:00:00Z")
	if _, err := db.DB.Exec(`UPDATE iterations SET image_ref='worker:v7',image_digest='sha256:image',prompt_template_sha256='sha256:prompt' WHERE id='iter-v2'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES('TARI','Tariboy','2026-07-01T09:00:00Z','2026-07-01T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO tasks(task_key,queue_prefix,position,title,status,author,customer,group_name,created_at,updated_at,completed_at) VALUES('TARI-42','TARI',1,'Review','done','user:operator','user:operator','dev-team','2026-07-01T09:00:00Z','2026-07-01T11:00:00Z','2026-07-01T11:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO ai_requests(id,ts,agent,iteration,task_id) VALUES('req-v2','2026-07-01T10:30:00Z','worker','iter-v2','TARI-42')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO image_source_snapshots(image_ref,image_digest,source_name,source_digest,relative_dir,created_at,repository_id,git_commit,lock_digest) VALUES('worker:v7','sha256:image','worker','sha256:source','source-dir','2026-07-01T09:00:00Z','production-agent-images','91ab820','sha256:lock')`); err != nil {
		t.Fatal(err)
	}
	agentsDir := paths.New(base).AgentsDir()
	l := agentdir.New(agentsDir, "worker")
	if err := l.EnsureIteration("iter-v2"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.PromptPath("iter-v2"), []byte("review the task"), 0o600); err != nil {
		t.Fatal(err)
	}

	run, _, err := js.CreateRun(context.Background(), request("iter-v2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSnapshotter(SnapshotConfig{Store: js, BaseDir: base, AgentsDir: agentsDir}).BuildRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	targets, err := js.ListTargets(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewEvidenceReader(base).Manifest(targets[0].BundleHash)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != 2 || bundle.Subject.Type != "task" || bundle.Subject.ExternalID != "TARI-42" || bundle.Subject.Status != "done" || bundle.Subject.Group != "dev-team" {
		t.Fatalf("subject evidence = %+v", bundle.Subject)
	}
	if bundle.Runtime.ImageRef != "worker:v7" || bundle.Runtime.ImageDigest != "sha256:image" || bundle.Runtime.RepositoryID != "production-agent-images" || bundle.Runtime.GitCommit != "91ab820" || bundle.Runtime.SourceDigest != "sha256:source" || bundle.Runtime.LockDigest != "sha256:lock" {
		t.Fatalf("runtime evidence = %+v", bundle.Runtime)
	}
	if bundle.Configuration.PromptTemplateSHA256 != "sha256:prompt" {
		t.Fatalf("configuration evidence = %+v", bundle.Configuration)
	}
	page, err := NewEvidenceReader(base).Search(targets[0].BundleHash, EvidenceQuery{Artifacts: []string{"task", "image", "source"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 4 {
		t.Fatalf("v2 evidence results = %+v", page.Results)
	}
}

func TestSnapshotUsageAndEmptyEvidenceCompleteness(t *testing.T) {
	base := t.TempDir()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "iter-usage", "worker", "done", "2026-07-01T10:00:00Z")
	for _, row := range []struct {
		id                  string
		input, output, cost any
	}{
		{"req-1", 11, 7, 0.125},
		{"req-2", 13, 5, 0.375},
	} {
		if _, err := db.DB.Exec(`INSERT INTO ai_requests(id,ts,agent,iteration,input_tokens,output_tokens,cost_usd) VALUES(?,?,?,?,?,?,?)`, row.id, "2026-07-01T10:30:00Z", "worker", "iter-usage", row.input, row.output, row.cost); err != nil {
			t.Fatal(err)
		}
	}
	run, _, err := js.CreateRun(context.Background(), request("iter-usage"))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSnapshotter(SnapshotConfig{Store: js, BaseDir: base, AgentsDir: paths.New(base).AgentsDir()}).BuildRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	targets, err := js.ListTargets(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	reader := NewEvidenceReader(base)
	bundle, err := reader.Manifest(targets[0].BundleHash)
	if err != nil {
		t.Fatal(err)
	}
	if want := (UsageTotal{Requests: 2, InputTokens: 24, OutputTokens: 12, CostUSD: 0.5}); bundle.Usage != want {
		t.Fatalf("usage=%+v, want %+v", bundle.Usage, want)
	}
	wantStatus := map[string]string{"prompt": "missing", "audit": "empty", "transcript": "empty", "task": "present", "image": "missing"}
	if len(bundle.Completeness) != len(wantStatus) {
		t.Fatalf("completeness=%+v, want exactly %+v", bundle.Completeness, wantStatus)
	}
	for _, status := range bundle.Completeness {
		want, ok := wantStatus[status.Artifact]
		if !ok || want != status.Status {
			t.Fatalf("completeness=%+v", bundle.Completeness)
		}
		item, err := reader.Get(targets[0].BundleHash, EvidenceLocator{Artifact: "completeness", Locator: status.Artifact})
		if err != nil {
			t.Fatalf("get completeness %q: %v", status.Artifact, err)
		}
		if item["value"].(ArtifactStatus) != status {
			t.Fatalf("completeness item=%+v, want %+v", item, status)
		}
		delete(wantStatus, status.Artifact)
	}
	if len(wantStatus) != 0 {
		t.Fatalf("missing completeness statuses: %+v", wantStatus)
	}
	page, err := reader.Search(targets[0].BundleHash, EvidenceQuery{Artifacts: []string{"completeness"}})
	if err != nil || len(page.Results) != len(bundle.Completeness) {
		t.Fatalf("completeness search=%+v err=%v", page, err)
	}
}

func TestEvidenceReaderPreservesLegacyBundleHashes(t *testing.T) {
	base := t.TempDir()
	bundles := []string{
		`{"schema_version":1,"bundle_hash":"","target":{"Iteration":"iter-v1","Agent":"worker","Status":"done","StartedAt":"2026-07-01T10:00:00Z"},"prompt":{"locator":"prompt","content":"","present":false},"audit":[],"transcript":[],"usage":{"Requests":0,"InputTokens":0,"OutputTokens":0,"CostUSD":0},"completeness":[]}`,
		`{"schema_version":2,"bundle_hash":"","target":{"Iteration":"iter-v2","Agent":"worker","Status":"done","StartedAt":"2026-07-01T10:00:00Z"},"subject":{"id":"subject","type":"iteration","external_id":"iter-v2","snapshot_hash":"sha256:subject","status":"done","artifacts":[]},"runtime":{"agent":"worker"},"configuration":{"plugins":[],"skills":[]},"source":{},"prompt":{"locator":"prompt","content":"","present":false},"audit":[],"transcript":[],"usage":{"Requests":0,"InputTokens":0,"OutputTokens":0,"CostUSD":0},"completeness":[]}`,
	}
	reader := NewEvidenceReader(base)
	for _, canonical := range bundles {
		hash := putLegacyBundle(t, base, canonical)
		bundle, err := reader.Manifest(hash)
		if err != nil || bundle.BundleHash != hash {
			t.Fatalf("legacy bundle hash=%q bundle=%+v err=%v", hash, bundle, err)
		}
	}
}

func TestEvidenceReaderExposesReadableTranscriptCalls(t *testing.T) {
	base := t.TempDir()
	entry := aiproxy.TranscriptEntry{
		Meta:     aiproxy.AIRequest{ID: "req-action", Provider: "openai", Model: "gpt-test"},
		Request:  []byte(`{"instructions":"work safely","input":"run verification"}`),
		Response: []byte(`{"status":"completed","output":[{"type":"local_shell_call","call_id":"shell-1","action":{"type":"exec","command":["bash","-lc","make check"]}}]}`),
	}
	hash := putBundle(t, base, EvidenceBundle{SchemaVersion: 1, Transcript: []map[string]any{transcriptMap(t, entry)}})
	reader := NewEvidenceReader(base)
	page, err := reader.Search(hash, EvidenceQuery{Artifacts: []string{"transcript"}, Query: "make check"})
	if err != nil || len(page.Results) != 1 || page.Results[0]["locator"] != "req-action" {
		t.Fatalf("readable transcript search=%+v err=%v", page, err)
	}
	got, err := reader.Get(hash, EvidenceLocator{Artifact: "transcript", Locator: "req-action"})
	if err != nil || stringMustJSON(t, got) != stringMustJSON(t, page.Results[0]) {
		t.Fatalf("get=%+v search=%+v err=%v", got, page.Results[0], err)
	}
	if strings.Contains(stringMustJSON(t, got), `"request":"`) {
		t.Fatalf("readable call leaked base64 envelope: %+v", got)
	}
}

func TestEvidenceReaderHidesRepeatedInstructionsWithoutChangingTranscriptLocators(t *testing.T) {
	base := t.TempDir()
	entries := []aiproxy.TranscriptEntry{
		{Meta: aiproxy.AIRequest{ID: "req-first", Provider: "openai"}, Request: []byte(`{"instructions":"follow the rubric","input":"start"}`), Response: []byte(`{"status":"completed","output":[]}`)},
		{Meta: aiproxy.AIRequest{ID: "req-second", Provider: "openai"}, Request: []byte(`{"instructions":"follow the rubric","input":"verify"}`), Response: []byte(`{"status":"completed","output":[{"type":"local_shell_call","call_id":"shell-2","action":{"type":"exec","command":["bash","-lc","make check"]}}]}`)},
	}
	hash := putBundle(t, base, EvidenceBundle{SchemaVersion: 1, Transcript: []map[string]any{transcriptMap(t, entries[0]), transcriptMap(t, entries[1])}})
	page, err := NewEvidenceReader(base).Search(hash, EvidenceQuery{Artifacts: []string{"transcript"}, Query: "make check"})
	if err != nil || len(page.Results) != 1 || page.Results[0]["locator"] != "req-second" {
		t.Fatalf("action search = %+v, err = %v", page, err)
	}
	first, err := NewEvidenceReader(base).Get(hash, EvidenceLocator{Artifact: "transcript", Locator: "req-first"})
	if err != nil || first["value"].(map[string]any)["instructions"] != "follow the rubric" {
		t.Fatalf("first call = %+v, err = %v", first, err)
	}
	second, err := NewEvidenceReader(base).Get(hash, EvidenceLocator{Artifact: "transcript", Locator: "req-second"})
	if err != nil || second["value"].(map[string]any)["instructions"] != "" {
		t.Fatalf("second call = %+v, err = %v", second, err)
	}
	page, err = NewEvidenceReader(base).Search(hash, EvidenceQuery{Artifacts: []string{"transcript"}, Query: "verify"})
	if err != nil || len(page.Results) != 1 || page.Results[0]["locator"] != "req-second" {
		t.Fatalf("rewritten request search = %+v, err = %v", page, err)
	}
}

func TestEvidenceReaderProjectionErrorRemainsCitable(t *testing.T) {
	base := t.TempDir()
	entry := aiproxy.TranscriptEntry{
		Meta:     aiproxy.AIRequest{ID: "req-truncated", Provider: "openai"},
		Request:  []byte(`{"messages":[{"role":"assistant","content":"","tool_calls":[{"id":"call-1","function":{"name":"broken","arguments":"{"}}]}]}`),
		Response: []byte(`{"choices":[{"message":{"content":"ok"}}]}`),
	}
	hash := putBundle(t, base, EvidenceBundle{SchemaVersion: 1, Transcript: []map[string]any{transcriptMap(t, entry)}})
	reader := NewEvidenceReader(base)
	page, err := reader.Search(hash, EvidenceQuery{Artifacts: []string{"transcript"}, Query: "projection error"})
	if err != nil || len(page.Results) != 1 || page.Results[0]["locator"] != "req-truncated" {
		t.Fatalf("projection gap search = %+v, err = %v", page, err)
	}
	got, err := reader.Get(hash, EvidenceLocator{Artifact: "transcript", Locator: "req-truncated"})
	if err != nil || got["locator"] != "req-truncated" {
		t.Fatalf("projection gap get = %+v, err = %v", got, err)
	}
}

func TestEvidenceReaderLegacyAndMalformedTranscriptRemainVisible(t *testing.T) {
	base := t.TempDir()
	entry := aiproxy.TranscriptEntry{Meta: aiproxy.AIRequest{ID: "legacy-request", Provider: "openai"}, Request: []byte(`not json`), Response: []byte(`also not json`)}
	encoded := stringMustJSON(t, transcriptMap(t, entry))
	canonical := `{"schema_version":1,"bundle_hash":"","target":{"Iteration":"iter-v1","Agent":"worker","Status":"done","StartedAt":"2026-07-01T10:00:00Z"},"prompt":{"locator":"prompt","content":"","present":false},"audit":[],"transcript":[` + encoded + `],"usage":{"Requests":0,"InputTokens":0,"OutputTokens":0,"CostUSD":0},"completeness":[]}`
	hash := putLegacyBundle(t, base, canonical)
	reader := NewEvidenceReader(base)
	page, err := reader.Search(hash, EvidenceQuery{Artifacts: []string{"transcript"}, Query: "parse_error"})
	if err != nil || len(page.Results) != 1 || page.Results[0]["locator"] != "legacy-request" {
		t.Fatalf("malformed legacy transcript=%+v err=%v", page, err)
	}
	if _, err := reader.Manifest(hash); err != nil {
		t.Fatalf("readable projection changed legacy hash: %v", err)
	}
}

func TestSnapshotRedactsDecodedTranscriptPayloads(t *testing.T) {
	base := t.TempDir()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "iter-secret", "worker", "done", "2026-07-01T10:00:00Z")
	if _, err := db.DB.Exec(`INSERT INTO secrets(agent,key,value) VALUES(?,?,?)`, "worker", "api", "very-secret"); err != nil {
		t.Fatal(err)
	}
	agentsDir := paths.New(base).AgentsDir()
	if err := agentdir.New(agentsDir, "worker").EnsureIteration("iter-secret"); err != nil {
		t.Fatal(err)
	}
	if err := aiproxy.AppendTranscript(agentsDir, aiproxy.TranscriptEntry{
		Meta:     aiproxy.AIRequest{ID: "req-secret", Agent: "worker", Iteration: "iter-secret", Provider: "openai"},
		Request:  []byte(`{"instructions":"use very-secret","input":"work"}`),
		Response: []byte(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"very-secret"}]}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	run, _, err := js.CreateRun(context.Background(), request("iter-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSnapshotter(SnapshotConfig{Store: js, BaseDir: base, AgentsDir: agentsDir}).BuildRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	targets, err := js.ListTargets(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewEvidenceReader(base).Manifest(targets[0].BundleHash)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle.Transcript[0])
	if err != nil {
		t.Fatal(err)
	}
	var captured aiproxy.TranscriptEntry
	if err := json.Unmarshal(raw, &captured); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(captured.Request)+string(captured.Response), "very-secret") || !strings.Contains(string(captured.Request)+string(captured.Response), "[REDACTED]") {
		t.Fatalf("decoded payloads not redacted: request=%s response=%s", captured.Request, captured.Response)
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
