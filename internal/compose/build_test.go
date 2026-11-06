package compose

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The build verb applies only the stores, workflows and images phases: it
// creates no agent or queue.
func TestBuildAppliesOnlyStoresWorkflowsAndImages(t *testing.T) {
	fc := teamCaller(map[string]string{"developer": "1.0.0"}, map[string]string{"development": "1.0.0"})
	f := File{Version: 2, Stores: map[string]StoreSpec{"team": {Git: teamGit}},
		Workflows:  map[string]WorkflowSpec{"development": {SourceSpec{Store: "team"}}},
		Images:     map[string]ImageSpec{"developer": {SourceSpec: SourceSpec{Store: "team"}}},
		Agents:     map[string]AgentSpec{"dev": {Image: "developer:latest"}},
		TaskQueues: map[string]TaskQueueSpec{"DEV": {Name: "Development", Workflow: "development", Pools: map[string][]string{"developers": {"dev"}}}},
	}
	var out strings.Builder
	if err := NewRunner(fc, "", &out).Build(f); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	want := []string{"POST /api/stores/team/refresh", "POST /api/workflow-images/build", "POST /api/images/build"}
	if got := mutating(fc.calls, 0); !slices.Equal(got, want) {
		t.Fatalf("mutating calls = %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "build: 1 ok, 2 changed, 0 failed, 0 skipped") || strings.Contains(out.String(), "agent:dev") {
		t.Fatalf("report:\n%s", out.String())
	}
}

func TestBuildPublishesDeclaredImagesThroughDaemon(t *testing.T) {
	workdir := t.TempDir()
	ctxDir := filepath.Join(workdir, "analyst")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tariboyfile := "schema_version: 2\n"
	if err := os.WriteFile(filepath.Join(ctxDir, "Tariboyfile.yaml"), []byte(tariboyfile), 0o600); err != nil {
		t.Fatal(err)
	}

	fc := newFake()
	r := NewRunner(fc, workdir, io.Discard)
	f, err := Parse([]byte(goodYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Build(f); err != nil {
		t.Fatalf("build: %v", err)
	}

	if countCalls(fc, "POST /api/images/build") != 1 {
		t.Fatalf("build did not publish through daemon: %v", fc.calls)
	}
	body, ok := bodyFor(fc, "POST /api/images/build").(map[string]any)
	if !ok || body["name"] != "analyst" || body["tag"] != "latest" || body["path"] != ctxDir || body["wait"] != true || body["initiator"] != "compose" {
		t.Fatalf("image build body = %#v", body)
	}
}
