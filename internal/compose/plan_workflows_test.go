package compose

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/tasks"
)

// teamCaller registers Store team with the image and workflow versions given.
func teamCaller(images, workflows map[string]string) *sourcesCaller {
	fc := newSourcesCaller()
	fc.remote[teamGit] = &sourceState{commit: "c1", images: images, workflows: workflows}
	fc.registered["team"] = &fakeStore{source: teamGit, commit: "c1"}
	return fc
}

func workflowsFile(workflows map[string]WorkflowSpec) File {
	return File{Version: 2, Stores: map[string]StoreSpec{"team": {Git: teamGit}}, Workflows: workflows}
}

func buildBody(fc *sourcesCaller, route string) map[string]any {
	i := callIndex(fc.calls, "POST "+route)
	if i < 0 {
		return nil
	}
	return fc.bodies[i].(map[string]any)
}

// A workflow entry builds from its Store after the Store's refresh, and from
// a path with the absolute directory.
func TestUpBuildsVersion2WorkflowsFromAStoreAndAPath(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review", "0.1.0")
	fc := teamCaller(nil, map[string]string{"development": "1.2.0"})
	var out strings.Builder
	f := workflowsFile(map[string]WorkflowSpec{
		"development": {SourceSpec{Store: "team", Version: "1.2.0"}},
		"review":      {SourceSpec{Path: "./review"}},
	})
	if err := NewRunner(fc, dir, &out).Up(f); err != nil {
		t.Fatalf("up: %v\n%s", err, out.String())
	}
	want := []string{"POST /api/stores/team/refresh", "POST /api/workflow-images/build", "POST /api/workflow-images/build"}
	if got := mutating(fc.calls, 0); !slices.Equal(got, want) {
		t.Fatalf("mutating calls = %v, want %v", got, want)
	}
	var sources []string
	for i, c := range fc.calls {
		if c == "POST /api/workflow-images/build" {
			b := fc.bodies[i].(map[string]any)
			src, _ := b["source"].(string)
			path, _ := b["path"].(string)
			sources = append(sources, src+path)
		}
	}
	if !slices.Equal(sources, []string{"team/development", filepath.Join(dir, "review")}) {
		t.Fatalf("build sources = %v", sources)
	}
	for _, res := range []string{"workflow:development", "workflow:review"} {
		if got := reportOutcome(out.String(), res); got != "changed" {
			t.Errorf("%s = %q\n%s", res, got, out.String())
		}
	}
	// A second up publishes the same versions: created is false, so ok.
	out.Reset()
	if err := NewRunner(fc, dir, &out).Up(f); err != nil {
		t.Fatalf("second up: %v", err)
	}
	if !strings.Contains(out.String(), "up: 3 ok, 0 changed, 0 failed, 0 skipped") {
		t.Fatalf("second up:\n%s", out.String())
	}
}

// A version the source does not give fails the entry before any build.
func TestUpFailsAWorkflowVersionConflictWithoutABuild(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review", "0.2.0")
	fc := teamCaller(nil, map[string]string{"development": "1.3.0"})
	var out strings.Builder
	f := workflowsFile(map[string]WorkflowSpec{
		"development": {SourceSpec{Store: "team", Version: "1.2.0"}},
		"review":      {SourceSpec{Path: "./review", Version: "0.1.0"}},
	})
	if err := NewRunner(fc, dir, &out).Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	if callIndex(fc.calls, "POST /api/workflow-images/build") >= 0 {
		t.Fatalf("a workflow was built: %v", fc.calls)
	}
	for _, want := range []string{
		"failed   workflow:development: workflow development: expected version 1.2.0, the source gives 1.3.0; the tag is not moved",
		"failed   workflow:review: workflow review: expected version 0.1.0, the source gives 0.2.0; the tag is not moved",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

// The workflow a source builds must have the entry's name.
func TestUpFailsAWorkflowWhoseNameDiffersFromItsEntry(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review-flow", "0.1.0")
	fc := teamCaller(nil, map[string]string{"dev-flow": "1.0.0"})
	var out strings.Builder
	f := workflowsFile(map[string]WorkflowSpec{
		"development": {SourceSpec{Store: "team", Source: "dev-flow"}},
		"review":      {SourceSpec{Path: "./review"}},
	})
	if err := NewRunner(fc, dir, &out).Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	for _, want := range []string{
		"failed   workflow:development: workflow development: the source builds workflow dev-flow; the entry key must equal the workflow name",
		"failed   workflow:review: workflow review: the source builds workflow review-flow; the entry key must equal the workflow name",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
	if got := countCallsSince(fc.calls, 0, "POST /api/workflow-images/build"); got != 0 {
		t.Fatalf("builds = %d, want none: %v", got, fc.calls)
	}
}

// A version tag of a workflow is the version the source must give: another
// version fails before the build, so neither tag moves.
func TestUpFailsAWorkflowWhoseVersionTagTheSourceDoesNotGive(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review", "0.4.0")
	fc := teamCaller(nil, nil)
	var out strings.Builder
	f := workflowsFile(map[string]WorkflowSpec{"review": {SourceSpec{Path: "./review", Tag: "0.3.0"}}})
	if err := NewRunner(fc, dir, &out).Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	if callIndex(fc.calls, "POST /api/workflow-images/build") >= 0 ||
		!strings.Contains(out.String(), "workflow review: expected version 0.3.0, the source gives 0.4.0; the tag is not moved") {
		t.Fatalf("calls %v\n%s", fc.calls, out.String())
	}
	out.Reset()
	if err := NewRunner(fc, dir, &out).Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "workflow   review     version conflict: want=0.3.0 source=0.4.0\n") {
		t.Fatalf("status:\n%s", out.String())
	}
}

func queueWorkflowFile(workflow string, entry WorkflowSpec) File {
	f := taskQueueComposeFile()
	f.Version = 2
	f.Stores = map[string]StoreSpec{"team": {Git: teamGit}}
	f.Workflows = map[string]WorkflowSpec{"development": entry}
	f.Images = map[string]ImageSpec{"basic": {SourceSpec: SourceSpec{Path: "./basic"}}} // not built: the tests skip image builds
	q := f.TaskQueues["DEV"]
	q.Workflow = workflow
	f.TaskQueues["DEV"] = q
	return f
}

// A queue that names the entry follows the entry's tag; STORE/NAME pins the
// digest of the build.
func TestUpBindsAVersion2QueueByTagOrPinnedDigest(t *testing.T) {
	for _, tc := range []struct {
		workflow, tag, wantTag string
	}{
		{"development", "", "latest"},
		{"development", "1.2.0", "1.2.0"},
		{"team/development", "", ""},
		{"development:1.2.0", "", "1.2.0"},
	} {
		fc := teamCaller(nil, map[string]string{"development": "1.2.0"})
		var out strings.Builder
		f := queueWorkflowFile(tc.workflow, WorkflowSpec{SourceSpec{Store: "team", Tag: tc.tag}})
		upNoBuild(t, NewRunner(fc, "", &out), f)
		got := fc.bound["DEV"]
		if got.Digest != "wf-development-1.2.0" || got.Tag != tc.wantTag {
			t.Errorf("%s tag %q: bound = %+v, want tag %q", tc.workflow, tc.tag, got, tc.wantTag)
		}
		build, bind := callIndex(fc.calls, "POST /api/workflow-images/build"), callIndex(fc.calls, "PUT /api/task-queues/DEV/workflow")
		if build < 0 || bind < build {
			t.Errorf("%s: build=%d bind=%d", tc.workflow, build, bind)
		}
	}
}

// status compares a queue that names an entry with the entry's tag.
func TestStatusResolvesAVersion2QueueThroughTheEntryTag(t *testing.T) {
	fc := teamCaller(nil, map[string]string{"development": "1.2.0"})
	fc.images["development:1.2.0"] = map[string]any{"name": "development", "version": "1.2.0", "digest": "wf-development-1.2.0"}
	fc.queues["DEV"] = tasks.Queue{Prefix: "DEV", Name: "Development", Revision: 1}
	fc.bound["DEV"] = tasks.QueueWorkflow{Queue: "DEV", Name: "development", Version: "1.2.0", Digest: "wf-development-1.2.0", Tag: "1.2.0"}
	var out strings.Builder
	f := queueWorkflowFile("development", WorkflowSpec{SourceSpec{Store: "team", Tag: "1.2.0"}})
	if err := NewRunner(fc, "", &out).Status(f); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "task queue DEV        workflow development:1.2.0 wf-developme ok") {
		t.Fatalf("status:\n%s", out.String())
	}
}

// A failed workflow build skips the binding; the queue keeps its workflow.
func TestUpSkipsTheBindingOfAFailedVersion2Workflow(t *testing.T) {
	fc := teamCaller(nil, map[string]string{"development": "1.2.0"})
	fc.buildErr["team/development"] = errors.New("invalid workflow")
	var out strings.Builder
	f := queueWorkflowFile("development", WorkflowSpec{SourceSpec{Store: "team"}})
	r := NewRunner(fc, "", &out)
	r.skipBuild = true
	if err := r.Up(f); err == nil {
		t.Fatal("up succeeded")
	}
	if callIndex(fc.calls, "PUT /api/task-queues/DEV/workflow") >= 0 {
		t.Fatalf("the queue was bound: %v", fc.calls)
	}
	if got := reportOutcome(out.String(), "queue:DEV"); got != "skipped" {
		t.Fatalf("queue:DEV = %q\n%s", got, out.String())
	}
}
