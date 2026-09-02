package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/registry"
	storedb "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/teamportable"
)

type fakeGroups struct{}

func (*fakeGroups) Create(string, string) (map[string]any, error) { return nil, nil }
func (*fakeGroups) List() ([]map[string]any, error)               { return nil, nil }
func (*fakeGroups) Inspect(string) (map[string]any, error)        { return nil, nil }
func (*fakeGroups) Remove(string, bool) error                     { return nil }
func (*fakeGroups) Assign(string, string) error                   { return nil }
func (*fakeGroups) Rename(string, string) error                   { return nil }
func (*fakeGroups) ChangeLead(string, string) error               { return nil }

func TestTeamEditRoutesRegistered(t *testing.T) {
	r := BuildRegistry()
	for _, tc := range []struct{ path, method, route string }{
		{"group.rename", "PATCH", "/api/groups/{name}/name"},
		{"group.lead.set", "PATCH", "/api/groups/{name}/lead"},
		{"group.member.rm", "DELETE", "/api/groups/{name}/members/{agent}"},
		{"team.compose", "GET", "/api/groups/{name}/compose"},
		{"team.import.yaml", "POST", "/api/team-imports/preview-yaml"},
		{"team.import.archive.apply", "POST", "/api/team-imports/{id}/apply"},
		{"team.import.archive.status", "GET", "/api/team-imports/{id}"},
	} {
		command, ok := r.Get(tc.path)
		if !ok {
			t.Fatalf("%s is not registered", tc.path)
		}
		if command.HTTP == nil || command.HTTP.Method != tc.method || command.HTTP.Path != tc.route {
			t.Fatalf("%s route = %+v", tc.path, command.HTTP)
		}
	}
}

func TestBuiltInTeamTemplateRoutesAreAbsent(t *testing.T) {
	r := BuildRegistry()
	for _, path := range []string{"team.template.ls", "team.template.create"} {
		if _, ok := r.Get(path); ok {
			t.Fatalf("%s remains registered", path)
		}
	}
}

func TestTeamSourceImportPlanReusesSourceWithinOneArchive(t *testing.T) {
	imported := map[string]string{}
	first := teamportable.Image{SourceName: "shared", SourceDigest: "digest"}
	if reuse, err := planTeamSource(imported, first); err != nil || reuse {
		t.Fatalf("first plan reuse=%v err=%v", reuse, err)
	}
	if reuse, err := planTeamSource(imported, first); err != nil || !reuse {
		t.Fatalf("second plan reuse=%v err=%v", reuse, err)
	}
	if _, err := planTeamSource(imported, teamportable.Image{SourceName: "shared", SourceDigest: "other"}); err == nil {
		t.Fatal("expected source-name conflict")
	}
}

func TestAgentTimeoutOverrideRequiresDurationAndReturnsSeconds(t *testing.T) {
	if seconds, err := parseAgentTimeout("2h"); err != nil || seconds != 7200 {
		t.Fatalf("2h = %d, %v", seconds, err)
	}
	if _, err := parseAgentTimeout("60"); err == nil {
		t.Fatal("accepted timeout without unit")
	}
}

func TestRewriteTeamComposeImageRefsForConflictResolution(t *testing.T) {
	input := []byte("version: 1\nagents:\n  worker:\n    image: demo:v1\n    group: team\n")
	output, err := rewriteTeamComposeRefs(input, map[string]string{"demo:v1": "demo:imported"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := compose.Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if file.Agents["worker"].Image != "demo:imported" {
		t.Fatalf("compose = %s", output)
	}
}

func TestYAMLPreviewReportsMissingAgentImageBeforeApply(t *testing.T) {
	c, _, _ := ctxWithStore(t)
	c.BaseDir = t.TempDir()
	c.Groups = &fakeGroups{}
	preview, err := teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}.PreviewYAML("team", []byte("version: 1\ngroups:\n  team: {}\nagents:\n  worker:\n    image: missing:v1\n    group: team\n"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := planTeamImport(c, preview)
	if err != nil {
		t.Fatal(err)
	}
	agents := result.(map[string]any)["agents"].([]map[string]any)
	if len(agents) != 1 || agents[0]["action"] != "blocked" || agents[0]["conflict"] != true {
		t.Fatalf("agent plans = %#v", agents)
	}
}

func TestApplyTeamImageBuildsTwoRefsFromOneImportedSource(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	source := filepath.Join(base, "team-imports", "id", "images", "shared")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 1\nprompts: [PROMPT.md]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "PROMPT.md"), []byte("shared\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshots := imagesnapshot.Store{DB: db.DB, Root: filepath.Join(base, "image-source-snapshots")}
	seed, err := snapshots.Capture(context.Background(), "seed:v1", "seed-built", "seed", source)
	if err != nil {
		t.Fatal(err)
	}
	c := &registry.Ctx{Store: db, BaseDir: base}
	preview := teamportable.Preview{StagedDir: filepath.Join(base, "team-imports", "id")}
	for index, ref := range []string{"shared:v1", "shared:v2"} {
		planned := teamportable.Image{Ref: ref, SourceName: "shared", SourceDigest: seed.SourceDigest}
		if err := applyTeamImage(c, preview, planned, index > 0, func() {}); err != nil {
			t.Fatalf("apply %s: %v", ref, err)
		}
		parsed, _ := image.ParseRef(ref)
		if !imageStore(c).Exists(parsed) {
			t.Fatalf("image %s was not built", ref)
		}
	}
	if err := os.RemoveAll(filepath.Join(base, "image-sources", "shared")); err != nil {
		t.Fatal(err)
	}
	for index, ref := range []string{"shared:v1", "shared:v4"} {
		planned := teamportable.Image{Ref: ref, SourceName: "shared", SourceDigest: seed.SourceDigest}
		if err := applyTeamImage(c, preview, planned, index > 0, func() {}); err != nil {
			t.Fatalf("reuse existing then apply %s: %v", ref, err)
		}
	}
	if err := applyTeamImage(c, preview, teamportable.Image{Ref: "shared:v3", SourceName: "shared", SourceDigest: "sha256:wrong"}, true, func() {}); err == nil {
		t.Fatal("accepted staged source whose digest did not match archive metadata")
	}
}

func TestApplyTeamImageWaitsForPublicationGate(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	source := filepath.Join(base, "team-imports", "id", "images", "shared")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshots := imagesnapshot.Store{DB: db.DB, Root: filepath.Join(base, "image-source-snapshots")}
	seed, err := snapshots.Capture(context.Background(), "seed:v1", "seed-built", "seed", source)
	if err != nil {
		t.Fatal(err)
	}
	c := &registry.Ctx{Store: db, BaseDir: base}
	preview := teamportable.Preview{StagedDir: filepath.Join(base, "team-imports", "id")}
	planned := teamportable.Image{Ref: "shared:v1", SourceName: "shared", SourceDigest: seed.SourceDigest}
	entered, release, locked := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		locked <- image.WithPublicationGate(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	done := make(chan error, 1)
	go func() { done <- applyTeamImage(c, preview, planned, false, func() {}) }()
	select {
	case err := <-done:
		close(release)
		<-locked
		t.Fatalf("team publisher ignored publication gate: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-locked; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
