package compose

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/safefs"
	"github.com/alekzonder/tariboy/internal/tidy"
	"github.com/alekzonder/tariboy/internal/version"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// A compose archive is a tariboy-portable archive of kind "compose": the
// compose file, unchanged, and the image and workflow image archives the
// daemon exports for it. It never holds Stores or secret values.
const (
	packKind       = "compose"
	packSchema     = 1
	packComposeRel = "compose.yaml"
)

// packMeta is the manifest metadata of a compose archive.
type packMeta struct {
	Schema          int            `json:"schema"`
	ProducerVersion string         `json:"producer_version"`
	ComposeFile     string         `json:"compose_file"`
	Images          []packImage    `json:"images"`
	Workflows       []packWorkflow `json:"workflows"`
}

// packImage is one image archive: the image-artifact export of Ref.
type packImage struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	File   string `json:"file"`
}

// packWorkflow is one workflow image archive; import tags it Tags.
type packWorkflow struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Digest  string   `json:"digest"`
	Tags    []string `json:"tags"`
	File    string   `json:"file"`
}

// packed is a staged compose archive that passed every check.
type packed struct {
	dir  string // staged root: compose.yaml, images/, workflows/
	meta packMeta
	file File
	// plugins lists the plugins each image ref needs.
	plugins map[string][]string
}

// member is the absolute path of a staged member file.
func (p *packed) member(rel string) string { return filepath.Join(p.dir, filepath.FromSlash(rel)) }

func imageMemberPath(ref string) string {
	r, _ := image.ParseRef(ref)
	return path.Join("images", r.Name, r.Tag+".tar.gz")
}

func workflowMemberPath(name, version string) string {
	return path.Join("workflows", name, version+".tar.gz")
}

// writePack writes the compose archive of the members staged under dir.
func writePack(w io.Writer, dir string, meta packMeta) error {
	rels := []string{packComposeRel}
	for _, img := range meta.Images {
		rels = append(rels, img.File)
	}
	for _, wf := range meta.Workflows {
		rels = append(rels, wf.File)
	}
	files := make([]portablearchive.File, 0, len(rels))
	for _, rel := range rels {
		f, err := hashFile(dir, rel)
		if err != nil {
			return err
		}
		files = append(files, f)
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return portablearchive.Write(w, dir, portablearchive.Manifest{Format: "tariboy-portable", Version: 1, Kind: packKind, Files: files, Metadata: raw}, rels)
}

func hashFile(dir, rel string) (portablearchive.File, error) {
	in, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return portablearchive.File{}, err
	}
	defer in.Close()
	h := sha256.New()
	n, err := io.Copy(h, in)
	if err != nil {
		return portablearchive.File{}, err
	}
	return portablearchive.File{Path: rel, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

// stagePack unpacks an archive file into dest, which must not exist, and
// checks the sha256 and size of every file its manifest lists.
func stagePack(file, dest string) (portablearchive.Manifest, error) {
	in, err := os.Open(file)
	if err != nil {
		return portablearchive.Manifest{}, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return portablearchive.Manifest{}, err
	}
	return portablearchive.Stage(in, info.Size(), dest, portablearchive.ComposeLimits())
}

// verifyPack checks a staged compose archive as a whole: its metadata, the
// compose file, the content digest of every image and workflow image, and
// that the archive holds every image and workflow the file needs. It reads
// no daemon and returns every problem it finds.
func verifyPack(dir string, m portablearchive.Manifest) (*packed, []error) {
	p := &packed{dir: dir, plugins: map[string][]string{}}
	if err := safefs.DecodeStrictJSON(bytes.NewReader(m.Metadata), &p.meta); err != nil {
		return nil, []error{fmt.Errorf("archive metadata: %v", err)}
	}
	if p.meta.Schema != packSchema {
		return nil, []error{fmt.Errorf("archive schema %d is not supported (want %d)", p.meta.Schema, packSchema)}
	}
	var problems []error
	if _, err := version.ParseSemVer(p.meta.ProducerVersion); err != nil {
		problems = append(problems, fmt.Errorf("archive producer_version: %w", err))
	}
	problems = append(problems, p.readFile()...)
	for _, img := range p.meta.Images {
		if err := p.verifyImage(img); err != nil {
			problems = append(problems, fmt.Errorf("image %s: %w", img.Ref, err))
		}
	}
	for _, wf := range p.meta.Workflows {
		if err := p.verifyWorkflow(wf); err != nil {
			problems = append(problems, fmt.Errorf("workflow %s %s: %w", wf.Name, wf.Version, err))
		}
	}
	problems = append(problems, p.coverage()...)
	return p, problems
}

// readFile parses and validates the compose file of the archive. Local paths
// are not checked: they name directories of the machine that packed it.
func (p *packed) readFile() []error {
	b, err := os.ReadFile(p.member(packComposeRel))
	if err != nil {
		return []error{fmt.Errorf("archive has no %s", packComposeRel)}
	}
	if p.file, err = Parse(b); err != nil {
		return []error{err}
	}
	return p.file.problems()
}

// memberFile opens a member the metadata names and checks the member limit.
func (p *packed) memberFile(rel string) (*os.File, int64, error) {
	clean, err := safefs.CleanRel(rel)
	if err != nil || clean != rel {
		return nil, 0, fmt.Errorf("invalid member path %q", rel)
	}
	in, err := os.Open(p.member(rel))
	if err != nil {
		return nil, 0, fmt.Errorf("member %s is missing", rel)
	}
	info, err := in.Stat()
	if err != nil {
		in.Close()
		return nil, 0, err
	}
	if info.Size() > portablearchive.ComposeMemberBytes {
		in.Close()
		return nil, 0, fmt.Errorf("member %s is %d bytes, over the %d byte limit", rel, info.Size(), portablearchive.ComposeMemberBytes)
	}
	return in, info.Size(), nil
}

// verifyImage proves that the image archive holds the runnable image the
// metadata names: the content digest of the image is recomputed.
func (p *packed) verifyImage(img packImage) error {
	in, size, err := p.memberFile(img.File)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.MkdirTemp(p.dir, ".verify-")
	if err != nil {
		return err
	}
	defer tidy.RemoveAll(nil, tmp)
	stage := filepath.Join(tmp, "image")
	m, err := portablearchive.Stage(in, size, stage, portablearchive.DefaultLimits())
	if err != nil {
		return err
	}
	if m.Kind != "image-artifact" || len(m.Files) != 1 || m.Files[0].Path != "image.tar.gz" {
		return errors.New("member is not a runnable image artifact")
	}
	var meta struct {
		Ref    string `json:"ref"`
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(m.Metadata, &meta); err != nil {
		return fmt.Errorf("image metadata: %v", err)
	}
	ref, err := image.ParseRef(img.Ref)
	if err != nil || meta.Ref != ref.String() {
		return fmt.Errorf("the member holds image %s", meta.Ref)
	}
	body, err := os.ReadFile(filepath.Join(stage, "image.tar.gz"))
	if err != nil {
		return err
	}
	inner, err := image.ValidatePortableArchive(body, ref)
	if err != nil {
		return err
	}
	if inner.Digest != img.Digest || meta.Digest != img.Digest {
		return fmt.Errorf("content digest %s does not match %s", inner.Digest, img.Digest)
	}
	for _, plugin := range inner.Plugins {
		p.plugins[img.Ref] = append(p.plugins[img.Ref], plugin.Name)
	}
	return nil
}

// verifyWorkflow recomputes the digest of a workflow image archive.
func (p *packed) verifyWorkflow(wf packWorkflow) error {
	in, size, err := p.memberFile(wf.File)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.MkdirTemp(p.dir, ".verify-")
	if err != nil {
		return err
	}
	defer tidy.RemoveAll(nil, tmp)
	info, err := workflowimage.VerifyArchive(in, size, tmp)
	if err != nil {
		return err
	}
	if info.Name != wf.Name || info.Version != wf.Version || info.Digest != wf.Digest {
		return fmt.Errorf("the member holds %s %s %s", info.Name, info.Version, info.Digest)
	}
	return nil
}

// coverage requires an image for every agent and a workflow image for every
// workflow the file binds.
func (p *packed) coverage() []error {
	var out []error
	images := map[string]bool{}
	for _, img := range p.meta.Images {
		images[img.Ref] = true
	}
	for _, ref := range packImageRefs(p.file) {
		if !images[ref] {
			out = append(out, fmt.Errorf("the archive has no image %s", ref))
		}
	}
	workflows := map[string]bool{}
	for _, wf := range p.meta.Workflows {
		workflows[wf.Name] = true
	}
	for _, ref := range packWorkflowRefs(p.file) {
		if name, _, _ := strings.Cut(ref, ":"); !workflows[name] {
			out = append(out, fmt.Errorf("the archive has no workflow %s", ref))
		}
	}
	return out
}

// packImageRefs are the image refs of the file's agents, as NAME:TAG, sorted
// and without duplicates.
func packImageRefs(f File) []string {
	seen := map[string]bool{}
	for _, name := range sortedKeys(f.Agents) {
		if ref := normalizeImageRef(f.Agents[name].Image); ref != "" {
			seen[ref] = true
		}
	}
	return sortedKeys(seen)
}

// packWorkflowRefs are the workflow refs NAME:TAG of the file: every
// workflows entry and every queue workflow, sorted and without duplicates.
func packWorkflowRefs(f File) []string {
	seen := map[string]bool{}
	for _, name := range sortedKeys(f.Workflows) {
		seen[name+":"+f.Workflows[name].tagOrLatest()] = true
	}
	for _, prefix := range sortedKeys(f.TaskQueues) {
		ref, declared, err := queueWorkflowRef(f.TaskQueues[prefix], f.Workflows)
		if err == nil && declared {
			seen[ref.Ref()] = true
		}
	}
	return sortedKeys(seen)
}
