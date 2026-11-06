package compose

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// imagesPhase builds every image the file declares, one action per image, so
// a failed build skips only the agents on that image.
type imagesPhase struct{}

func (imagesPhase) name() string { return "images" }

func (imagesPhase) plan(p *planner, f File, _ *liveState) error {
	if p.r.skipBuild {
		return nil
	}
	for _, name := range sortedKeys(f.Images) {
		path := f.Images[name].BuildPath()
		key := "image:" + name
		p.provide(key, p.add(Action{Resource: key, Kind: "build", Fail: "build image " + name,
			Fields: []Field{{Name: "path", To: path}}, run: p.r.buildImage(name, path)}))
	}
	return nil
}

// buildImage publishes NAME:latest from the local directory path. compose
// cannot tell whether the sources changed, so it always builds; the build is
// unchanged when NAME:latest keeps its digest.
func (r *Runner) buildImage(name, path string) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		ref := name + ":latest"
		before, err := r.imageDigest(ref)
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		ctx := path
		if !filepath.IsAbs(ctx) {
			ctx = filepath.Join(r.workdir, ctx)
		}
		r.logf("building image %s", ref)
		// The build is a daemon operation; wait answers when it has ended.
		raw, err := r.call.Call("POST", "/api/images/build", map[string]any{"name": name, "tag": "latest", "path": ctx, "wait": true, "initiator": "compose"})
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		after, err := r.imageDigest(ref)
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		return raw, after == "" || after != before, nil
	}
}

// imageDigest returns the digest of an image ref, or "" when it does not
// exist.
func (r *Runner) imageDigest(ref string) (string, error) {
	var img struct {
		Digest string `json:"digest"`
	}
	err := r.get("/api/images/"+ref, &img)
	if isAPIError(err, "not_found") {
		return "", nil
	}
	return img.Digest, err
}
