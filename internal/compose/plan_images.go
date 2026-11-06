package compose

import (
	"encoding/json"
	"fmt"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// imagesPhase builds every image the file declares, one action per image, so
// a failed build skips only the agents on that image. An image from a Store
// waits for the Store's actions.
type imagesPhase struct{}

func (imagesPhase) name() string { return "images" }

func (imagesPhase) plan(p *planner, f File, live *liveState) error {
	if p.r.skipBuild {
		return nil
	}
	for _, name := range sortedKeys(f.Images) {
		spec := f.Images[name]
		key := "image:" + name
		if f.Version == 2 {
			p.sourceStatus("image", name, spec.SourceSpec, live.sources)
		}
		if spec.Store != "" {
			p.provide(key, p.add(Action{Resource: key, Kind: "build", Fields: p.r.sourceFields(name, spec.SourceSpec),
				run: p.r.buildStoreImage(name, spec.SourceSpec)}, p.storeNeeds(spec.SourceSpec)...))
			continue
		}
		fields := []Field{{Name: "path", To: spec.BuildPath()}}
		if f.Version == 2 {
			fields = p.r.sourceFields(name, spec.SourceSpec)
		}
		p.provide(key, p.add(Action{Resource: key, Kind: "build", Fail: "build image " + name,
			Fields: fields, run: p.r.buildImage(name, spec)}))
	}
	return nil
}

// buildImage publishes NAME:TAG from the local directory of the entry, after
// it checks the image_version of its Tariboyfile against the entry's
// version. compose cannot tell whether the sources changed, so it always
// builds; the build is unchanged when NAME:TAG keeps its digest.
func (r *Runner) buildImage(name string, spec ImageSpec) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		tag := spec.tagOrLatest()
		ref := name + ":" + tag
		ctx := r.absPath(spec.BuildPath())
		if spec.Version != "" {
			v2, err := imagefile.ParseV2(ctx)
			if err != nil {
				return nil, false, fmt.Errorf("image %s: %w", name, err)
			}
			if err := sourceVersionConflict("image", name, spec.Version, v2.ImageVersion); err != nil {
				return nil, false, err
			}
		}
		before, _, err := r.inspectImage(ref)
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		r.logf("building image %s", ref)
		// The build is a daemon operation; wait answers when it has ended.
		raw, err := r.call.Call("POST", "/api/images/build", map[string]any{"name": name, "tag": tag, "path": ctx, "wait": true, "initiator": "compose"})
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		after, _, err := r.inspectImage(ref)
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		if err := sourceVersionConflict("image", name, spec.Version, after.ImageVersion); err != nil {
			return nil, false, err
		}
		return raw, after.Digest == "" || after.Digest != before.Digest, nil
	}
}

// buildStoreImage publishes NAME:TAG from STORE/SOURCE. It reads the version
// the Store gives after the stores phase refreshed it, checks it against the
// entry's version, and builds only when NAME:TAG was built from another
// version, the rule of the Store's automatic builds. A conflict found before
// the build builds nothing, so the tag does not move.
func (r *Runner) buildStoreImage(name string, s SourceSpec) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		selector, ref := s.Store+"/"+s.sourceName(name), name+":"+s.tagOrLatest()
		source, err := r.storeEntryVersion("image", s.Store, s.sourceName(name))
		if err != nil {
			return nil, false, fmt.Errorf("image %s: %w", name, err)
		}
		if err := sourceVersionConflict("image", name, s.Version, source); err != nil {
			return nil, false, err
		}
		before, _, err := r.inspectImage(ref)
		if err != nil {
			return nil, false, fmt.Errorf("image %s: %w", name, err)
		}
		if source != "" && before.ImageVersion == source {
			r.logf("image %s is built from %s %s", ref, selector, source)
			return nil, false, nil
		}
		r.logf("building image %s from %s", ref, selector)
		raw, err := r.call.Call("POST", "/api/images/build", map[string]any{"name": name, "tag": s.tagOrLatest(), "source": selector, "wait": true, "initiator": "compose"})
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		after, _, err := r.inspectImage(ref)
		if err != nil {
			return nil, false, fmt.Errorf("build image %s: %w", name, err)
		}
		if err := sourceVersionConflict("image", name, s.Version, after.ImageVersion); err != nil {
			return nil, false, err
		}
		return raw, after.Digest == "" || after.Digest != before.Digest, nil
	}
}
