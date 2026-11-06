package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alekzonder/tariboy/internal/image"
)

// archiveSkip is the reason the stores phase of compose unpack gives: an
// archive carries no Store, its images and workflows come from the archive.
const archiveSkip = "skipped (archive)"

// planArchiveStores skips every Store of the file; nothing depends on them.
func (p *planner) planArchiveStores(f File) {
	for _, name := range sortedKeys(f.Stores) {
		p.add(Action{Resource: "store:" + name, Kind: "skip", Fields: []Field{{Name: archiveSkip}}, SkipReason: archiveSkip})
	}
}

// planArchiveWorkflows imports each workflow image of the archive. The action
// is named "workflow:NAME" and, for a version 1 queue, by the selector the
// queue bindings look for.
func (p *planner) planArchiveWorkflows(f File) {
	for _, wf := range p.r.archive.meta.Workflows {
		id := "workflow:" + wf.Name
		i := p.add(Action{Resource: id, Kind: "import", Fields: []Field{{Name: "version", To: wf.Version}, {Name: "digest", To: shortDigest(wf.Digest)}},
			run: p.r.importWorkflow(wf)})
		p.provide(id, i)
		for _, prefix := range sortedKeys(f.TaskQueues) {
			ref, declared, err := queueWorkflowRef(f.TaskQueues[prefix], f.Workflows)
			if err == nil && declared && ref.Name == wf.Name && "workflow:"+ref.Selector() != id {
				p.provide("workflow:"+ref.Selector(), i)
			}
		}
	}
}

// planArchiveImages imports each image of the archive, one action per ref,
// named "image:NAME" for the agents on it.
func (p *planner) planArchiveImages() {
	for _, img := range p.r.archive.meta.Images {
		ref, _ := image.ParseRef(img.Ref)
		id := "image:" + ref.Name
		p.provide(id, p.add(Action{Resource: id, Kind: "import", Fields: []Field{{Name: "ref", To: img.Ref}, {Name: "digest", To: shortDigest(img.Digest)}},
			run: p.r.importImage(img)}))
	}
}

// importWorkflow installs a workflow image from the archive unless every tag
// it gets already names its digest. The result has the shape of a build
// result, which the queue binding reads.
func (r *Runner) importWorkflow(wf packWorkflow) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		result, err := json.Marshal(workflowImage{Name: wf.Name, Version: wf.Version, Digest: wf.Digest})
		if err != nil {
			return nil, false, err
		}
		same, err := r.workflowTagsMatch(wf)
		if err != nil || same {
			return result, false, err
		}
		r.logf("importing workflow %s %s", wf.Name, wf.Version)
		raw, err := r.upload("/api/workflow-image-imports", r.archive.member(wf.File))
		if err != nil {
			return nil, false, fmt.Errorf("import workflow %s: %w", wf.Name, err)
		}
		var got workflowImage
		if err := json.Unmarshal(raw, &got); err != nil || got.Digest != wf.Digest {
			return nil, false, fmt.Errorf("import workflow %s: the daemon installed digest %s, want %s", wf.Name, got.Digest, wf.Digest)
		}
		return raw, true, nil
	}
}

func (r *Runner) workflowTagsMatch(wf packWorkflow) (bool, error) {
	for _, tag := range wf.Tags {
		img, found, err := r.inspectWorkflowImage(wf.Name + ":" + tag)
		if err != nil {
			return false, fmt.Errorf("workflow %s: read tag %s: %w", wf.Name, tag, err)
		}
		if !found || img.Digest != wf.Digest {
			return false, nil
		}
	}
	return true, nil
}

// importImage installs an image from the archive unless its ref already has
// the archived digest.
func (r *Runner) importImage(img packImage) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		before, found, err := r.inspectImage(img.Ref)
		if err != nil {
			return nil, false, fmt.Errorf("image %s: %w", img.Ref, err)
		}
		if found && before.Digest == img.Digest {
			return nil, false, nil
		}
		r.logf("importing image %s", img.Ref)
		raw, err := r.upload("/api/image-imports", r.archive.member(img.File))
		if err != nil {
			return nil, false, fmt.Errorf("import image %s: %w", img.Ref, err)
		}
		var preview struct {
			ImportID string `json:"import_id"`
		}
		if err := json.Unmarshal(raw, &preview); err != nil || preview.ImportID == "" {
			return nil, false, fmt.Errorf("import image %s: the preview returned no import id", img.Ref)
		}
		raw, err = r.call.Call("POST", "/api/image-imports/"+preview.ImportID+"/apply", map[string]any{})
		if err != nil {
			return nil, false, fmt.Errorf("import image %s: %w", img.Ref, err)
		}
		return raw, true, nil
	}
}

// uploader is the raw upload of the daemon client (*client.Client).
type uploader interface {
	Upload(route string, body io.Reader, size int64) (json.RawMessage, error)
}

// upload sends a file to a daemon import route.
func (r *Runner) upload(route, file string) (json.RawMessage, error) {
	up, ok := r.call.(uploader)
	if !ok {
		return nil, errors.New("archive upload is not available")
	}
	in, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return nil, err
	}
	return up.Upload(route, in, info.Size())
}
