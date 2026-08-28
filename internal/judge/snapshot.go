package judge

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy/session"
	"github.com/alekzonder/tariboy/internal/audit"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/paths"
)

type SnapshotConfig struct {
	Store     *Store
	BaseDir   string
	AgentsDir string
}
type Snapshotter struct {
	store        *Store
	base, agents string
	reader       *EvidenceReader
}

func NewSnapshotter(c SnapshotConfig) *Snapshotter {
	return &Snapshotter{store: c.Store, base: c.BaseDir, agents: c.AgentsDir, reader: NewEvidenceReader(c.BaseDir)}
}
func (s *Snapshotter) BuildRun(ctx context.Context, runID string) error {
	ts, e := s.store.ListTargets(runID)
	if e != nil {
		return e
	}
	for _, t := range ts {
		if t.SnapshotStatus == "ready" {
			continue
		}
		if e = s.build(ctx, t); e != nil {
			return e
		}
	}
	_, e = s.store.db.ExecContext(ctx, `UPDATE judge_runs SET status='running',targets_ready=(SELECT COUNT(*) FROM judge_targets WHERE run_id=? AND snapshot_status='ready'),updated_at=? WHERE id=?`, runID, s.store.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), runID)
	return e
}
func (s *Snapshotter) build(ctx context.Context, t Target) (err error) {
	now := s.store.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	tx, e := s.store.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO judge_retention_pins(run_id,target_id,iteration_id,created_at) VALUES(?,?,?,?)`, t.RunID, t.ID, t.Iteration, now)
	if e == nil {
		_, e = tx.ExecContext(ctx, `UPDATE judge_targets SET snapshot_status='snapshotting' WHERE id=?`, t.ID)
	}
	if e != nil {
		tx.Rollback()
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	defer func() {
		_, _ = s.store.db.Exec(`DELETE FROM judge_retention_pins WHERE run_id=? AND target_id=?`, t.RunID, t.ID)
		if err != nil {
			_, _ = s.store.db.Exec(`UPDATE judge_targets SET snapshot_status='snapshot_failed' WHERE id=?`, t.ID)
		}
	}()
	var status, started, imageRef, imageDigest, promptTemplateSHA string
	if e = s.store.db.QueryRowContext(ctx, `SELECT status,started_at,image_ref,image_digest,prompt_template_sha256 FROM iterations WHERE id=?`, t.Iteration).Scan(&status, &started, &imageRef, &imageDigest, &promptTemplateSHA); e != nil {
		return fmt.Errorf("snapshot source: %w", e)
	}
	subject, e := s.store.SubjectForTarget(t.ID)
	if e != nil {
		return e
	}
	secrets := []string{}
	rows, e := s.store.db.QueryContext(ctx, `SELECT value FROM secrets WHERE agent=?`, t.Agent)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil && v != "" {
			secrets = append(secrets, v)
		}
	}
	rows.Close()
	redact := func(v string) string {
		for _, x := range secrets {
			v = strings.ReplaceAll(v, x, "[REDACTED]")
		}
		return v
	}
	l := agentdir.New(s.agents, t.Agent)
	prompt, e := os.ReadFile(l.PromptPath(t.Iteration))
	present := e == nil
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	evs, e := audit.ReadByIteration(l.AuditLog(), t.Iteration)
	if e != nil {
		return e
	}
	aud := []map[string]any{}
	for _, x := range evs {
		raw, _ := json.Marshal(x)
		var m map[string]any
		_ = json.Unmarshal([]byte(redact(string(raw))), &m)
		aud = append(aud, m)
	}
	trs, e := session.ReadEntries(s.agents, t.Agent, t.Iteration)
	if e != nil {
		return e
	}
	tr := []map[string]any{}
	for i, x := range trs {
		raw, _ := json.Marshal(x)
		var m map[string]any
		_ = json.Unmarshal([]byte(redact(string(raw))), &m)
		m["request_id"] = x.Meta.ID
		if m["request_id"] == "" {
			m["request_id"] = fmt.Sprint(i)
		}
		tr = append(tr, m)
	}
	runtime := RuntimeEvidence{Agent: t.Agent, Group: subject.Snapshot.Group, ImageRef: imageRef, ImageDigest: imageDigest}
	configuration := ConfigurationEvidence{PromptTemplateSHA256: promptTemplateSHA, Plugins: []image.ManifestPlugin{}, Skills: []image.ManifestSkill{}}
	sourceEvidence := SourceEvidence{}
	completeness := []ArtifactStatus{{Artifact: "prompt", Status: map[bool]string{true: "present", false: "missing"}[present]}, {Artifact: "audit", Status: "present"}, {Artifact: "transcript", Status: "present"}, {Artifact: "task", Status: "present"}}
	if imageDigest != "" {
		snapshots := imagesnapshot.Store{DB: s.store.db, Root: filepath.Join(s.base, "image-source-snapshots")}
		snapshot, ok, lookupErr := snapshots.LookupDigest(ctx, imageDigest)
		if lookupErr != nil {
			return lookupErr
		}
		if ok {
			runtime.SourceDigest, runtime.RepositoryID, runtime.GitCommit, runtime.LockDigest = snapshot.SourceDigest, snapshot.RepositoryID, snapshot.GitCommit, snapshot.LockDigest
			sourceEvidence = SourceEvidence{Name: snapshot.SourceName, Digest: snapshot.SourceDigest, RepositoryID: snapshot.RepositoryID, GitCommit: snapshot.GitCommit, LockDigest: snapshot.LockDigest}
			completeness = append(completeness, ArtifactStatus{Artifact: "source", Status: "present", SHA256: snapshot.SourceDigest})
		} else {
			completeness = append(completeness, ArtifactStatus{Artifact: "source", Status: "missing"})
		}
	}
	imageStatus := "missing"
	if ref, parseErr := image.ParseRef(imageRef); parseErr == nil && imageDigest != "" {
		manifest, inspectErr := (&image.Store{Dir: paths.New(s.base).ImagesDir()}).InspectPinned(ref, imageDigest)
		if inspectErr == nil {
			configuration.Plugins, configuration.Skills = manifest.Plugins, manifest.Skills
			imageStatus = "present"
		}
	}
	completeness = append(completeness, ArtifactStatus{Artifact: "image", Status: imageStatus, SHA256: imageDigest})
	b := EvidenceBundle{
		SchemaVersion: 2,
		Target:        TargetMetadata{Iteration: t.Iteration, Agent: t.Agent, Status: status, StartedAt: started},
		Subject:       SubjectEvidence{ID: subject.ID, Type: subject.Type, ExternalID: subject.ExternalID, SnapshotHash: subject.SnapshotHash, Status: subject.Snapshot.Status, Group: subject.Snapshot.Group, Artifacts: subject.Snapshot.Artifacts},
		Runtime:       runtime,
		Configuration: configuration,
		Source:        sourceEvidence,
		Prompt:        EvidenceArtifact{Locator: "prompt", Content: redact(string(prompt)), Present: present},
		Audit:         aud,
		Transcript:    tr,
		Completeness:  completeness,
	}
	raw, e := json.Marshal(b)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	b.BundleHash = hash
	raw, e = json.Marshal(b)
	if e != nil {
		return e
	} // hash excludes self-referential envelope; reader accepts canonical content hash after removing it.
	// Store a hash of the canonical envelope with its hash field cleared, matching verification below.
	if e = s.write(hash, raw); e != nil {
		return e
	}
	if _, e = s.reader.load(hash); e != nil {
		return e
	}
	_, err = s.store.db.ExecContext(ctx, `UPDATE judge_targets SET bundle_path=?,bundle_hash=?,bundle_bytes=?,snapshot_status='ready',target_state='ready' WHERE id=?`, filepath.Join(paths.New(s.base).JudgeObjectsDir(), hash+".json.gz"), hash, len(raw), t.ID)
	return err
}
func (s *Snapshotter) write(hash string, b []byte) error {
	dir := paths.New(s.base).JudgeObjectsDir()
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	p := filepath.Join(dir, hash+".json.gz")
	if _, e := os.Stat(p); e == nil {
		return nil
	}
	f, e := os.CreateTemp(dir, ".tmp-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	g := gzip.NewWriter(f)
	if _, e = g.Write(b); e == nil {
		e = g.Close()
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, p)
}
