package compose

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func imagesFile(images map[string]ImageSpec, agents map[string]AgentSpec) File {
	return File{Version: 2, Stores: map[string]StoreSpec{"team": {Git: teamGit}}, Images: images, Agents: agents}
}

// A Store image builds with the entry's name and tag from STORE/SOURCE; a
// second up does not build it again while the source gives the version it
// was built from.
func TestUpBuildsAStoreImageOnlyWhenItsVersionChanges(t *testing.T) {
	fc := teamCaller(map[string]string{"dev-image": "1.2.0"}, nil)
	f := imagesFile(map[string]ImageSpec{"developer": {SourceSpec: SourceSpec{Store: "team", Source: "dev-image", Tag: "stable"}}}, nil)
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Up(f); err != nil {
		t.Fatalf("up: %v\n%s", err, out.String())
	}
	got := buildBody(fc, "/api/images/build")
	want := map[string]any{"name": "developer", "tag": "stable", "source": "team/dev-image", "wait": true, "initiator": "compose"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("build body = %v, want %v", got, want)
	}
	if reportOutcome(out.String(), "image:developer") != "changed" {
		t.Fatalf("report:\n%s", out.String())
	}
	before := len(fc.calls)
	out.Reset()
	if err := NewRunner(fc, "", &out).Up(f); err != nil {
		t.Fatalf("second up: %v", err)
	}
	if got := mutating(fc.calls, before); !slices.Equal(got, []string{"POST /api/stores/team/refresh"}) {
		t.Fatalf("second up mutating calls = %v", got)
	}
	if reportOutcome(out.String(), "image:developer") != "ok" {
		t.Fatalf("second report:\n%s", out.String())
	}
	// A new version in the Store builds again and moves the tag.
	fc.remote[teamGit].images["dev-image"] = "1.3.0"
	fc.remote[teamGit].commit = "c2"
	out.Reset()
	if err := NewRunner(fc, "", &out).Up(f); err != nil {
		t.Fatalf("third up: %v", err)
	}
	if fc.imageBuilds["developer"] != 2 || fc.manifests["developer:stable"]["image_version"] != "1.3.0" || reportOutcome(out.String(), "image:developer") != "changed" {
		t.Fatalf("builds=%d manifest=%v\n%s", fc.imageBuilds["developer"], fc.manifests["developer:stable"], out.String())
	}
}

// A version the Store does not give fails the image without a build, and the
// agents on the image are skipped.
func TestUpFailsAStoreImageVersionConflictWithoutABuild(t *testing.T) {
	fc := teamCaller(map[string]string{"developer": "1.3.0"}, nil)
	fc.manifests["developer:latest"] = map[string]any{"name": "developer", "digest": "img-developer-1.2.0", "image_version": "1.2.0"}
	f := imagesFile(map[string]ImageSpec{"developer": {SourceSpec: SourceSpec{Store: "team", Version: "1.2.0"}}},
		map[string]AgentSpec{"dev": {Image: "developer:latest"}})
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	if fc.imageBuilds["developer"] != 0 || fc.manifests["developer:latest"]["digest"] != "img-developer-1.2.0" {
		t.Fatalf("the image was built: %v", fc.calls)
	}
	want := "failed   image:developer: image developer: expected version 1.2.0, the source gives 1.3.0; the tag is not moved"
	if !strings.Contains(out.String(), want) || reportOutcome(out.String(), "agent:dev") != "skipped" {
		t.Fatalf("report:\n%s", out.String())
	}
}

// A path image builds with the entry's tag; its version is checked against
// the Tariboyfile before the build.
func TestUpBuildsAPathImageWithTagAndChecksItsVersion(t *testing.T) {
	dir := t.TempDir()
	writeImageSource(t, filepath.Join(dir, "reviewer"), "2.0.0")
	fc := teamCaller(nil, nil)
	f := imagesFile(map[string]ImageSpec{"reviewer": {SourceSpec: SourceSpec{Path: "./reviewer", Tag: "v2", Version: "2.0.0"}}}, nil)
	var out strings.Builder
	if err := NewRunner(fc, dir, &out).Up(f); err != nil {
		t.Fatalf("up: %v\n%s", err, out.String())
	}
	got := buildBody(fc, "/api/images/build")
	want := map[string]any{"name": "reviewer", "tag": "v2", "path": filepath.Join(dir, "reviewer"), "wait": true, "initiator": "compose"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("build body = %v, want %v", got, want)
	}
	writeImageSource(t, filepath.Join(dir, "reviewer"), "2.1.0")
	out.Reset()
	if err := NewRunner(fc, dir, &out).Up(f); err == nil {
		t.Fatal("up with a version conflict succeeded")
	}
	if fc.imageBuilds["reviewer"] != 1 || !strings.Contains(out.String(), "image reviewer: expected version 2.0.0, the source gives 2.1.0; the tag is not moved") {
		t.Fatalf("builds=%d\n%s", fc.imageBuilds["reviewer"], out.String())
	}
}

// Criterion 3: a version 1 image with context makes the calls it made before.
func TestUpBuildsAVersion1ContextImageAsBefore(t *testing.T) {
	fc := newFake()
	f := File{Version: 1, Images: map[string]ImageSpec{"dev": {Context: "dev"}}}
	if err := NewRunner(fc, "/work", &strings.Builder{}).Up(f); err != nil {
		t.Fatalf("up: %v", err)
	}
	want := []string{"GET /api/groups", "GET /api/agents", "GET /api/budgets", "GET /api/images/dev:latest", "POST /api/images/build", "GET /api/images/dev:latest"}
	if !slices.Equal(fc.calls, want) {
		t.Fatalf("calls = %v, want %v", fc.calls, want)
	}
	body := fc.bodies[4].(map[string]any)
	if !reflect.DeepEqual(body, map[string]any{"name": "dev", "tag": "latest", "path": "/work/dev", "wait": true, "initiator": "compose"}) {
		t.Fatalf("build body = %v", body)
	}
}

// A Store that fails its refresh skips every image and workflow from it; the
// entries from a path still apply, and up exits non-zero.
func TestUpSkipsTheBuildsOfAFailedStore(t *testing.T) {
	dir := t.TempDir()
	writeImageSource(t, filepath.Join(dir, "reviewer"), "1.0.0")
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review", "0.1.0")
	fc := teamCaller(map[string]string{"developer": "1.0.0"}, map[string]string{"development": "1.0.0"})
	fc.remote[teamGit].err = errors.New("could not resolve host")
	f := File{Version: 2, Stores: map[string]StoreSpec{"team": {Git: teamGit}},
		Workflows: map[string]WorkflowSpec{"development": {SourceSpec{Store: "team"}}, "review": {SourceSpec{Path: "./review"}}},
		Images:    map[string]ImageSpec{"developer": {SourceSpec: SourceSpec{Store: "team"}}, "reviewer": {SourceSpec: SourceSpec{Path: "./reviewer"}}},
		Agents:    map[string]AgentSpec{"dev": {Image: "developer:latest"}, "rev": {Image: "reviewer:latest"}},
	}
	var out strings.Builder
	if err := NewRunner(fc, dir, &out).Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	for res, want := range map[string]string{"store:team": "failed", "workflow:development": "skipped", "image:developer": "skipped",
		"agent:dev": "skipped", "workflow:review": "changed", "image:reviewer": "changed", "agent:rev": "changed"} {
		if got := reportOutcome(out.String(), res); got != want {
			t.Errorf("%s = %q, want %q", res, got, want)
		}
	}
	if !strings.Contains(out.String(), "failed   store:team: refresh store team: could not resolve host") {
		t.Fatalf("report:\n%s", out.String())
	}
}
