package commands

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
)

var updateGroupsGolden = flag.Bool("update-groups-golden", false, "rewrite testdata/groups_responses.golden.json")

// groupsGoldenVolatile names keys whose values depend on the wall clock.
var groupsGoldenVolatile = map[string]bool{"updated_at": true}

// groupsFixture serves groups from a real provisioner and stores the built
// image worker:latest and the ungrouped agents lead1, dev1 and dev2.
func groupsFixture(t *testing.T) *registry.Ctx {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	fixed := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "prompt.md"), []byte("prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := &imagefile.V2{SchemaVersion: 2, ImageVersion: "1.0.0", Dir: src, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
	ref := image.Ref{Name: "worker", Tag: "latest"}
	if _, err := image.BuildV2(spec, imagefile.ResolveRoots{}, ref, imageStore(c), func() time.Time { return fixed }, nil); err != nil {
		t.Fatal(err)
	}
	c.Groups = groups.NewProvisioner(groups.ProvisionerConfig{
		Groups: groups.NewStore(c.Store, time.Now), Agents: as,
		GroupsDir: filepath.Join(c.BaseDir, "groups"), Clock: time.Now,
	})
	for _, name := range []string{"lead1", "dev1", "dev2"} {
		if err := as.Create(agent.Agent{
			Name: name, ImageRef: ref.String(), Cwd: "/srv/" + name, HarnessType: "stub", Model: "m1",
			Effort: "high", Interactive: true, Enabled: name != "dev2", TimeoutS: 90, Plugins: []string{"context"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

// importID reads the import_id of a team import result, whatever its Go type.
func importID(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ImportID string `json:"import_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.ImportID == "" {
		t.Fatalf("no import_id in %s: %v", data, err)
	}
	return result.ImportID
}

// Catches a typed groups or teams response changing the JSON a client sees:
// the set of keys, their JSON types, and their values must match the recorded
// responses.
func TestGroupsResponsesMatchGolden(t *testing.T) {
	c := groupsFixture(t)
	recorder := newGoldenRecorder(t, groupsGoldenVolatile, c.BaseDir, "$BASE")
	call := func(label, path string, params registry.Params) any {
		t.Helper()
		value, err := h(t, path)(c, params)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		recorder.record(label, value)
		return value
	}
	call("group.create core", "group.create", registry.Params{"name": "core"})
	call("group.assign lead1", "group.assign", registry.Params{"name": "core", "agent": "lead1"})
	call("group.assign dev1", "group.assign", registry.Params{"name": "core", "agent": "dev1"})
	call("group.assign dev2", "group.assign", registry.Params{"name": "core", "agent": "dev2"})
	call("group.create core lead", "group.create", registry.Params{"name": "core", "lead": "lead1"})
	call("group.create empty", "group.create", registry.Params{"name": "empty"})
	call("group.ls", "group.ls", registry.Params{})
	call("group.inspect core", "group.inspect", registry.Params{"name": "core"})
	call("group.inspect empty", "group.inspect", registry.Params{"name": "empty"})
	call("group.lead.set core", "group.lead.set", registry.Params{"name": "core", "lead": "dev1"})
	call("group.member.rm core dev2", "group.member.rm", registry.Params{"name": "core", "agent": "dev2"})
	call("team.compose core", "team.compose", registry.Params{"name": "core"})
	call("group.rename core", "group.rename", registry.Params{"name": "core", "new_name": "crew"})

	// Each preview names one agent: plan rows follow map order.
	previews := []struct{ label, yaml string }{
		{"team.import.yaml blocked", "version: 1\ngroups:\n  fresh: {lead: newbie}\nagents:\n  newbie:\n    image: ghost:1\n    group: fresh\n"},
		{"team.import.yaml reuse", "version: 1\ngroups:\n  crew: {}\nagents:\n  lead1:\n    image: worker:latest\n    group: crew\n"},
		{"team.import.yaml rename", "version: 1\ngroups:\n  other: {}\nagents:\n  dev2:\n    image: worker:latest\n    group: other\n"},
	}
	ids := map[string]string{}
	for i, preview := range previews {
		value, err := h(t, "team.import.yaml")(c, registry.Params{"yaml": preview.yaml})
		if err != nil {
			t.Fatalf("%s: %v", preview.label, err)
		}
		ids[preview.label] = importID(t, value)
		recorder.replacements = append(recorder.replacements, ids[preview.label], "$IMPORT"+string(rune('A'+i)))
		recorder.record(preview.label, value)
	}
	call("team.import.archive.status pending", "team.import.archive.status", registry.Params{"id": ids["team.import.yaml blocked"]})
	call("team.import.archive.apply refs", "team.import.archive.apply", registry.Params{"id": ids["team.import.yaml blocked"], "refs": map[string]any{"ghost:1": "worker:latest"}})
	call("team.import.archive.status complete", "team.import.archive.status", registry.Params{"id": ids["team.import.yaml blocked"]})
	call("team.import.archive.apply update existing", "team.import.archive.apply", registry.Params{"id": ids["team.import.yaml reuse"], "update_existing": true})
	call("group.rm crew volumes", "group.rm", registry.Params{"name": "crew", "volumes": true})
	call("group.rm empty", "group.rm", registry.Params{"name": "empty"})
	recorder.check("groups_responses.golden.json", *updateGroupsGolden)
}

// Catches the team export route losing the compose YAML of the typed
// team.compose result: the archive must carry the YAML team.compose renders.
func TestTeamExportArchivesTypedComposeYAML(t *testing.T) {
	c := groupsFixture(t)
	c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := h(t, "group.assign")(c, registry.Params{"name": "core", "agent": "lead1"}); err != nil {
		t.Fatal(err)
	}
	rendered, err := h(t, "team.compose")(c, registry.Params{"name": "core"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	api.NewServer(BuildRegistry(), c).Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/groups/core/export", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", w.Code, w.Body.String())
	}
	gz, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if err != nil {
			t.Fatalf("archive has no tariboy-compose.yaml: %v", err)
		}
		if filepath.Base(header.Name) != "tariboy-compose.yaml" {
			continue
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		// The archive is compose-only: export drops the images section.
		if !strings.Contains(rendered.(TeamCompose).YAML, "cwd: /srv/lead1") || !strings.Contains(string(data), "cwd: /srv/lead1") {
			t.Fatalf("archived compose = %q, rendered %#v", data, rendered)
		}
		return
	}
}

func init() { typedCallSets["groups"] = groupsTypedCalls }

// groupsTypedCalls serves the groups fixture with one staged team import
// whose agent image is resolved through refs at apply time.
func groupsTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c := groupsFixture(t)
	staged, err := h(t, "team.import.yaml")(c, registry.Params{"yaml": "version: 1\ngroups:\n  fresh: {lead: newbie}\nagents:\n  newbie:\n    image: ghost:1\n    group: fresh\n"})
	if err != nil {
		t.Fatal(err)
	}
	id := importID(t, staged)
	return c, []typedCall{
		{"POST", "/api/groups", "/api/groups", map[string]any{"name": "core"}},
		{"POST", "/api/groups/{name}/assign", "/api/groups/core/assign", map[string]any{"agent": "lead1"}},
		{"POST", "/api/groups/{name}/assign", "/api/groups/core/assign", map[string]any{"agent": "dev1"}},
		{"POST", "/api/groups/{name}/assign", "/api/groups/core/assign", map[string]any{"agent": "dev2"}},
		{"POST", "/api/groups", "/api/groups", map[string]any{"name": "core", "lead": "lead1"}},
		{"GET", "/api/groups", "/api/groups", nil},
		{"GET", "/api/groups/{name}", "/api/groups/core", nil},
		{"PATCH", "/api/groups/{name}/lead", "/api/groups/core/lead", map[string]any{"lead": "dev1"}},
		{"DELETE", "/api/groups/{name}/members/{agent}", "/api/groups/core/members/dev2", nil},
		{"GET", "/api/groups/{name}/compose", "/api/groups/core/compose", nil},
		{"PATCH", "/api/groups/{name}/name", "/api/groups/core/name", map[string]any{"new_name": "crew"}},
		{"POST", "/api/team-imports/preview-yaml", "/api/team-imports/preview-yaml", map[string]any{"yaml": "version: 1\ngroups:\n  crew: {}\nagents:\n  lead1:\n    image: worker:latest\n    group: crew\n"}},
		{"GET", "/api/team-imports/{id}", "/api/team-imports/" + id, nil},
		{"POST", "/api/team-imports/{id}/apply", "/api/team-imports/" + id + "/apply", map[string]any{"refs": map[string]any{"ghost:1": "worker:latest"}}},
		{"GET", "/api/team-imports/{id}", "/api/team-imports/" + id, nil},
		{"DELETE", "/api/groups/{name}", "/api/groups/crew?volumes=true", nil},
	}
}
