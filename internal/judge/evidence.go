package judge

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/paths"
)

var ErrBadLocator = errors.New("judge: bad evidence locator")
var ErrCorruptEvidence = errors.New("judge: corrupt evidence")

type EvidenceArtifact struct {
	Locator string `json:"locator"`
	Content string `json:"content"`
	Present bool   `json:"present"`
}
type ArtifactStatus struct {
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	SHA256   string `json:"sha256,omitempty"`
}
type UsageTotal struct {
	Requests, InputTokens, OutputTokens int
	CostUSD                             float64
}
type TargetMetadata struct{ Iteration, Agent, Status, StartedAt string }
type SubjectEvidence struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	ExternalID   string   `json:"external_id"`
	SnapshotHash string   `json:"snapshot_hash"`
	Status       string   `json:"status"`
	Group        string   `json:"group,omitempty"`
	Artifacts    []string `json:"artifacts"`
}
type RuntimeEvidence struct {
	Agent        string `json:"agent"`
	Group        string `json:"group,omitempty"`
	ImageRef     string `json:"image_ref,omitempty"`
	ImageDigest  string `json:"image_digest,omitempty"`
	SourceDigest string `json:"source_digest,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	GitCommit    string `json:"git_commit,omitempty"`
	LockDigest   string `json:"lock_digest,omitempty"`
}
type ConfigurationEvidence struct {
	PromptTemplateSHA256 string                 `json:"prompt_template_sha256,omitempty"`
	Plugins              []image.ManifestPlugin `json:"plugins"`
	Skills               []image.ManifestSkill  `json:"skills"`
}
type SourceEvidence struct {
	Name         string `json:"name,omitempty"`
	Digest       string `json:"digest,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
	GitCommit    string `json:"git_commit,omitempty"`
	LockDigest   string `json:"lock_digest,omitempty"`
}
type EvidenceBundle struct {
	SchemaVersion int                   `json:"schema_version"`
	BundleHash    string                `json:"bundle_hash"`
	Target        TargetMetadata        `json:"target"`
	Subject       SubjectEvidence       `json:"subject,omitempty"`
	Runtime       RuntimeEvidence       `json:"runtime,omitempty"`
	Configuration ConfigurationEvidence `json:"configuration,omitempty"`
	Source        SourceEvidence        `json:"source,omitempty"`
	Prompt        EvidenceArtifact      `json:"prompt"`
	Audit         []map[string]any      `json:"audit"`
	Transcript    []map[string]any      `json:"transcript"`
	Usage         UsageTotal            `json:"usage"`
	Completeness  []ArtifactStatus      `json:"completeness"`
}
type EvidenceQuery struct {
	Artifacts     []string
	Query, Cursor string
	Limit         int
}
type EvidenceLocator struct{ Artifact, Locator string }
type EvidencePage struct {
	Results    []map[string]any `json:"results"`
	NextCursor string           `json:"next_cursor,omitempty"`
}
type EvidenceReader struct{ objects string }

func NewEvidenceReader(base string) *EvidenceReader {
	return &EvidenceReader{objects: paths.New(base).JudgeObjectsDir()}
}

func (r *EvidenceReader) Manifest(hash string) (EvidenceBundle, error) { return r.load(hash) }
func (r *EvidenceReader) load(hash string) (EvidenceBundle, error) {
	if len(hash) != 64 || strings.ContainsAny(hash, `/\\`) {
		return EvidenceBundle{}, ErrBadLocator
	}
	f, err := os.Open(filepath.Join(r.objects, hash+".json.gz"))
	if err != nil {
		return EvidenceBundle{}, err
	}
	defer f.Close()
	g, err := gzip.NewReader(f)
	if err != nil {
		return EvidenceBundle{}, fmt.Errorf("%w: gzip", ErrCorruptEvidence)
	}
	defer g.Close()
	b, err := io.ReadAll(g)
	if err != nil {
		return EvidenceBundle{}, fmt.Errorf("%w: gzip", ErrCorruptEvidence)
	}
	var out EvidenceBundle
	if err = json.Unmarshal(b, &out); err != nil {
		return EvidenceBundle{}, fmt.Errorf("%w: json", ErrCorruptEvidence)
	}
	// The envelope carries its own hash, so calculate the CAS key over the
	// canonical representation with that self-referential field blank.
	declared := out.BundleHash
	out.BundleHash = ""
	canonical, _ := json.Marshal(out)
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != hash || declared != hash {
		return EvidenceBundle{}, fmt.Errorf("%w: hash", ErrCorruptEvidence)
	}
	out.BundleHash = declared
	return out, nil
}
func (r *EvidenceReader) Search(hash string, q EvidenceQuery) (EvidencePage, error) {
	b, err := r.load(hash)
	if err != nil {
		return EvidencePage{}, err
	}
	lim := q.Limit
	if lim <= 0 || lim > 200 {
		lim = 200
	}
	start := 0
	if q.Cursor != "" {
		_, e := fmt.Sscanf(q.Cursor, "%d", &start)
		if e != nil || start < 0 {
			return EvidencePage{}, ErrBadLocator
		}
	}
	allowed := map[string]bool{}
	for _, x := range q.Artifacts {
		allowed[strings.ToLower(x)] = true
	}
	want := strings.ToLower(q.Query)
	all := []map[string]any{}
	add := func(kind, loc string, v any) {
		if len(allowed) > 0 && !allowed[kind] {
			return
		}
		raw, _ := json.Marshal(v)
		if want != "" && !strings.Contains(strings.ToLower(string(raw)), want) {
			return
		}
		all = append(all, map[string]any{"artifact": kind, "locator": loc, "value": v})
	}
	if b.Prompt.Present {
		add("prompt", "prompt", b.Prompt)
	}
	if b.SchemaVersion >= 2 {
		add("task", "subject", b.Subject)
		add("image", "runtime", b.Runtime)
		add("image", "configuration", b.Configuration)
		if b.Source.Digest != "" {
			add("source", "metadata", b.Source)
		}
	}
	add("metadata", "metadata", b.Target)
	add("usage", "usage", b.Usage)
	for _, x := range b.Audit {
		loc := fmt.Sprint(x["seq"])
		add("audit", loc, x)
	}
	for i, x := range b.Transcript {
		loc := fmt.Sprint(x["request_id"])
		if loc == "" || loc == "<nil>" {
			loc = fmt.Sprintf("%d", i)
		}
		add("transcript", loc, x)
	}
	if start >= len(all) {
		return EvidencePage{Results: []map[string]any{}}, nil
	}
	end := start + lim
	if end > len(all) {
		end = len(all)
	}
	p := EvidencePage{Results: all[start:end]}
	if end < len(all) {
		p.NextCursor = fmt.Sprint(end)
	}
	return p, nil
}
func (r *EvidenceReader) Get(hash string, l EvidenceLocator) (map[string]any, error) {
	if l.Locator == "" || strings.ContainsAny(l.Locator, "/\\") {
		return nil, ErrBadLocator
	}
	q := EvidenceQuery{Artifacts: []string{strings.ToLower(l.Artifact)}, Limit: 200}
	for {
		p, e := r.Search(hash, q)
		if e != nil {
			return nil, e
		}
		for _, x := range p.Results {
			if x["locator"] == l.Locator {
				return x, nil
			}
		}
		if p.NextCursor == "" {
			return nil, ErrBadLocator
		}
		q.Cursor = p.NextCursor
	}
}
