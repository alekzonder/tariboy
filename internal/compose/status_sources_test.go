package compose

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
)

func sourcesStatusFile() File {
	return File{Version: 2,
		Stores: map[string]StoreSpec{"team": {Git: teamGit, Ref: "v2"}, "gone": {Git: "https://example.com/gone.git"},
			"moved": {Git: "https://example.com/new.git"}, "local": {Path: "./store"}},
		Workflows: map[string]WorkflowSpec{"development": {SourceSpec{Store: "team"}}, "review": {SourceSpec{Path: "./review"}}},
		Images: map[string]ImageSpec{
			"developer": {SourceSpec: SourceSpec{Store: "team"}},
			"reviewer":  {SourceSpec: SourceSpec{Path: "./reviewer"}},
			"tester":    {SourceSpec: SourceSpec{Store: "team", Version: "1.0.0"}},
			"ops":       {SourceSpec: SourceSpec{Store: "team"}},
		},
	}
}

func sourcesStatusCaller(t *testing.T, dir string) *sourcesCaller {
	writeImageSource(t, filepath.Join(dir, "reviewer"), "2.0.0")
	writeWorkflowSource(t, filepath.Join(dir, "review"), "review", "0.1.0")
	fc := teamCaller(map[string]string{"developer": "1.2.0", "tester": "1.1.0", "ops": "3.0.0"}, map[string]string{"development": "1.0.0"})
	fc.registered["team"].ref = "main"
	fc.remote["https://example.com/old.git"] = &sourceState{}
	fc.registered["moved"] = &fakeStore{source: "https://example.com/old.git"}
	fc.remote[filepath.Join(dir, "store")] = &sourceState{err: &client.APIError{Code: "bad_source_path", Msg: "no directory"}}
	fc.registered["local"] = &fakeStore{source: filepath.Join(dir, "store")}
	fc.manifests["developer:latest"] = map[string]any{"digest": "d1", "image_version": "1.1.0"}
	fc.manifests["reviewer:latest"] = map[string]any{"digest": "d2", "image_version": "2.0.0"}
	fc.manifests["ops:latest"] = map[string]any{"digest": "d3", "image_version": "3.0.0"}
	fc.images["development:latest"] = map[string]any{"name": "development", "version": "0.9.0", "digest": "w1"}
	fc.images["review:latest"] = map[string]any{"name": "review", "version": "0.1.0", "digest": "w2"}
	return fc
}

// status reports the ref drift of a Store and each built version against the
// version its source gives, without a refresh or a build.
func TestStatusReportsStoreRefsAndSourceVersions(t *testing.T) {
	dir := t.TempDir()
	fc := sourcesStatusCaller(t, dir)
	var out strings.Builder
	if err := NewRunner(fc, dir, &out).Status(sourcesStatusFile()); err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{
		"store      gone       MISSING",
		"store      local      UNREADABLE (bad_source_path)",
		"store      moved      source differs: have=https://example.com/old.git want=https://example.com/new.git (up does not change it)",
		"store      team       drift: ref have=main want=v2",
		"workflow   development drift: built=0.9.0 source=1.0.0",
		"workflow   review     ok (version 0.1.0)",
		"image      developer  drift: built=1.1.0 source=1.2.0",
		"image      ops        ok (version 3.0.0)",
		"image      reviewer   ok (version 2.0.0)",
		"image      tester     version conflict: want=1.0.0 source=1.1.0",
		"drift: 7",
	} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("status lacks %q:\n%s", want, out.String())
		}
	}
	if got := mutating(fc.calls, 0); len(got) != 0 {
		t.Fatalf("status mutated: %v", got)
	}
	delete(fc.manifests, "ops:latest")
	out.Reset()
	if err := NewRunner(fc, dir, &out).Status(sourcesStatusFile()); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "image      ops        MISSING (not built; source 3.0.0)\n") {
		t.Fatalf("status:\n%s", out.String())
	}
}

// --dry-run prints the Store and build actions and makes only GET requests.
func TestDryRunPrintsStoreAndSourceActionsWithGETsOnly(t *testing.T) {
	dir := t.TempDir()
	fc := teamCaller(nil, nil)
	fc.registered["team"].ref = "main"
	f := File{Version: 2,
		Stores:    map[string]StoreSpec{"team": {Git: teamGit, Ref: "v2"}, "new": {Git: "https://example.com/new.git"}},
		Workflows: map[string]WorkflowSpec{"review": {SourceSpec{Path: "./review"}}},
		Images:    map[string]ImageSpec{"developer": {SourceSpec: SourceSpec{Store: "team", Version: "1.2.0"}}},
	}
	var out strings.Builder
	if err := NewRunner(fc, dir, &out).DryRun(f); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	for _, want := range []string{
		"phase stores",
		"  create   store:new (source=https://example.com/new.git; ref=(default branch))",
		"  update   store:team (ref main -> v2; refresh)",
		"phase workflows",
		"  build    workflow:review (path=./review; tag=latest)",
		"phase images",
		"  build    image:developer (source=team/developer; tag=latest; version=1.2.0 (checked at apply time))",
	} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("dry-run lacks %q:\n%s", want, out.String())
		}
	}
	for _, c := range fc.calls {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("dry-run made %s", c)
		}
	}
}
