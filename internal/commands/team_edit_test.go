package commands

import (
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/compose"
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
	t.Parallel()
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
	t.Parallel()
	r := BuildRegistry()
	for _, path := range []string{"team.template.ls", "team.template.create"} {
		if _, ok := r.Get(path); ok {
			t.Fatalf("%s remains registered", path)
		}
	}
}

func TestAgentTimeoutOverrideRequiresDurationAndReturnsSeconds(t *testing.T) {
	t.Parallel()
	if seconds, err := parseAgentTimeout("2h"); err != nil || seconds != 7200 {
		t.Fatalf("2h = %d, %v", seconds, err)
	}
	if _, err := parseAgentTimeout("60"); err == nil {
		t.Fatal("accepted timeout without unit")
	}
}

func TestRewriteTeamComposeImageRefsForConflictResolution(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
