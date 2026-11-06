package compose

import (
	"fmt"
	"path/filepath"
	"strings"
)

// liveSources is the daemon state of a version 2 file's stores, workflows
// and images. A failed read of one entry does not stop the plan: its error
// goes to the entry's status line, and up retries the read when it applies.
type liveSources struct {
	stores      map[string]liveStore // every registered Store, by name
	details     map[string]storeDetail
	detailErr   map[string]error
	images      map[string]liveImage // image entry -> its NAME:TAG; absent when not built
	imageErr    map[string]error
	workflows   map[string]workflowImage // workflow entry -> its NAME:TAG; absent when not built
	workflowErr map[string]error
}

// liveStore is a registered Store (store.list).
type liveStore struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

// storeDetail is the part of store.show and store.refresh compose reads.
type storeDetail struct {
	liveStore
	Images    []storeEntry `json:"images"`
	Workflows []storeEntry `json:"workflows"`
}

// storeEntry is an image or workflow source of a Store and the version it
// gives; Error is set when the daemon cannot read the source.
type storeEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

// liveImage is the part of an image manifest compose reads.
type liveImage struct {
	Digest       string `json:"digest"`
	ImageVersion string `json:"image_version"`
}

// readSources reads the Stores, then the detail of each file Store that is
// registered, the image of each images entry and the workflow image of each
// workflows entry. It reads nothing for a version 1 file.
func (r *Runner) readSources(f File) (liveSources, error) {
	live := liveSources{stores: map[string]liveStore{}, details: map[string]storeDetail{}, detailErr: map[string]error{},
		images: map[string]liveImage{}, imageErr: map[string]error{}, workflows: map[string]workflowImage{}, workflowErr: map[string]error{}}
	if f.Version != 2 {
		return live, nil
	}
	if len(f.Stores) > 0 {
		var list []liveStore
		if err := r.get("/api/stores", &list); err != nil {
			return live, fmt.Errorf("stores: %w", err)
		}
		for _, s := range list {
			live.stores[s.Name] = s
		}
	}
	for _, name := range sortedKeys(f.Stores) {
		if _, ok := live.stores[name]; !ok {
			continue
		}
		var d storeDetail
		if err := r.get("/api/stores/"+name, &d); err != nil {
			live.detailErr[name] = err
			continue
		}
		live.details[name] = d
	}
	for _, name := range sortedKeys(f.Images) {
		img, found, err := r.inspectImage(name + ":" + f.Images[name].tagOrLatest())
		switch {
		case err != nil:
			live.imageErr[name] = err
		case found:
			live.images[name] = img
		}
	}
	for _, name := range sortedKeys(f.Workflows) {
		img, found, err := r.inspectWorkflowImage(name + ":" + f.Workflows[name].tagOrLatest())
		switch {
		case err != nil:
			live.workflowErr[name] = err
		case found:
			live.workflows[name] = img
		}
	}
	return live, nil
}

// inspectImage reads an image ref; found is false when it does not exist.
func (r *Runner) inspectImage(ref string) (img liveImage, found bool, err error) {
	err = r.get("/api/images/"+ref, &img)
	if isAPIError(err, "not_found") {
		return liveImage{}, false, nil
	}
	return img, err == nil, err
}

// storeSource is the source compose registers for a Store, as the daemon
// stores it: the trimmed git URL, or the clean absolute path, a relative path
// resolved from the compose file's directory.
func (r *Runner) storeSource(s StoreSpec) string {
	if s.Git != "" {
		return strings.TrimSpace(s.Git)
	}
	return r.absPath(s.Path)
}

// absPath resolves a file path from the compose file's directory.
func (r *Runner) absPath(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.workdir, path)
	}
	return filepath.Clean(path)
}

// entryVersion returns the version a Store gives for its image or workflow
// source; ok is false when the Store has no such source or cannot read it.
func (d storeDetail) entryVersion(kind, source string) (string, error) {
	list := d.Images
	if kind == "workflow" {
		list = d.Workflows
	}
	for _, e := range list {
		if e.Name != source {
			continue
		}
		if e.Error != "" {
			return "", fmt.Errorf("store %s %s %s: %s", d.Name, kind, source, e.Error)
		}
		return e.Version, nil
	}
	return "", fmt.Errorf("store %s has no %s %s", d.Name, kind, source)
}
