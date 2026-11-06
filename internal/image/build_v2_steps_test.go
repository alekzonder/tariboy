package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/imagefile"
)

func writeV2Prompt(t *testing.T, source, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPrepareV2ReportsTheFirstFailingStep pins the text of each error and the
// order of the steps. Later steps are broken on purpose where a case checks
// that an earlier one wins.
func TestPrepareV2ReportsTheFirstFailingStep(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, filepath.Join(source, "skills"), "review")
	writeTestSkill(t, filepath.Join(source, "other"), "review")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	review := imagefile.SkillEntry{Dir: "./skills/review"}
	tests := []struct {
		name   string
		src    *imagefile.V2
		want   string
		prefix bool
	}{
		{name: "nil", src: nil, want: "schema-v2 source is required"},
		{name: "schema", src: &imagefile.V2{SchemaVersion: 1}, want: "schema-v2 source is required"},
		{name: "extends", src: &imagefile.V2{SchemaVersion: 2, Extends: []string{"./base"}, Plugins: []imagefile.V2Plugin{{Name: "nope"}}}, want: imagefile.ExtendsUnassembledMessage},
		{name: "plugin before skills", src: &imagefile.V2{SchemaVersion: 2, Plugins: []imagefile.V2Plugin{{Name: "nope"}}, Skills: []imagefile.SkillEntry{{Dir: "./missing"}}}, want: `unknown plugin "nope"`},
		{name: "skill resolve", src: &imagefile.V2{SchemaVersion: 2, Skills: []imagefile.SkillEntry{review, {Dir: "./missing"}}}, want: "skills[1]: ", prefix: true},
		{name: "skill prepare", src: &imagefile.V2{SchemaVersion: 2, Skills: []imagefile.SkillEntry{{Dir: "./empty"}}}, want: "skills[0]: ", prefix: true},
		{name: "skill set", src: &imagefile.V2{SchemaVersion: 2, Skills: []imagefile.SkillEntry{review, {Dir: "./other/review"}}}, want: `skills[1]: duplicate skill name "review"`},
		{name: "contract before prompts", src: &imagefile.V2{SchemaVersion: 2, Plugins: []imagefile.V2Plugin{{Name: "loop"}}, Prompts: []imagefile.PromptEntry{{File: "./missing.md"}}}, want: `plugin "loop" requires packaged skill "loop"`},
		{name: "runtime contract", src: &imagefile.V2{SchemaVersion: 2, Prompts: []imagefile.PromptEntry{{Runtime: "identity"}}}, want: `runtime placeholder "identity" requires plugin "whoami"`},
		{name: "prompt index counts runtimes", src: &imagefile.V2{SchemaVersion: 2, Prompts: []imagefile.PromptEntry{{Runtime: "user-prompt"}, {File: "./missing.md"}}}, want: "prompts[1]: ", prefix: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			if src != nil {
				src.Dir = source
			}
			_, err := prepareV2(src, imagefile.ResolveRoots{}, nil)
			if err == nil {
				t.Fatal("prepareV2 accepted the source")
			}
			if tc.prefix && !strings.HasPrefix(err.Error(), tc.want) || !tc.prefix && err.Error() != tc.want {
				t.Fatalf("prepareV2 error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestPrepareV2WithoutContentKeepsEmptyLists(t *testing.T) {
	prepared, err := prepareV2(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}, imagefile.ResolveRoots{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.plugins == nil || prepared.skills == nil || prepared.entries == nil || prepared.layers == nil || prepared.template.Entries == nil {
		t.Fatalf("prepared lists are nil: %#v", prepared)
	}
	body, err := json.Marshal(prepared.template)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"entries":[]`) {
		t.Fatalf("template = %s", body)
	}
}

func TestPrepareV2NamesLayersByPromptPosition(t *testing.T) {
	source := t.TempDir()
	writeV2Prompt(t, source, "first draft.md", "one\n")
	writeV2Prompt(t, source, "second.md", "two\n")
	prepared, err := prepareV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{
		{Runtime: "user-prompt"}, {File: "./first draft.md"}, {Runtime: "one-shot"}, {File: "./second.md"},
	}}, imagefile.ResolveRoots{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"", "prompt/layers/001-first-draft.md", "", "prompt/layers/003-second.md"}
	wantKinds := []string{"runtime", "file", "runtime", "file"}
	if len(prepared.entries) != len(wantPaths) {
		t.Fatalf("entries = %#v", prepared.entries)
	}
	for i, entry := range prepared.entries {
		if entry.Kind != wantKinds[i] || entry.ArchivePath != wantPaths[i] {
			t.Fatalf("entry %d = %#v", i, entry)
		}
	}
	if string(prepared.layers[wantPaths[1]]) != "one\n" || string(prepared.layers[wantPaths[3]]) != "two\n" || len(prepared.layers) != 2 {
		t.Fatalf("layers = %#v", prepared.layers)
	}
	wantHash, err := templateHash(prepared.entries)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.template.SHA256 != wantHash || prepared.template.SchemaVersion != 2 || !reflect.DeepEqual(prepared.template.Entries, prepared.entries) {
		t.Fatalf("template = %#v", prepared.template)
	}
}

func TestPrepareV2UsesProvidedSourceSkillsWithoutChangingThem(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "image")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	frozen := writeTestSkill(t, filepath.Join(base, "frozen"), "review")
	writeV2Prompt(t, frozen, "layer.md", "layer\n")
	roots := imagefile.ResolveRoots{SourceSkills: map[string]string{"../skills/review": frozen}}
	src := &imagefile.V2{SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "../skills/review"}}, Prompts: []imagefile.PromptEntry{{File: "../skills/review/layer.md"}}}
	prepared, err := prepareV2(src, roots, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"../skills/review": frozen}; !reflect.DeepEqual(roots.SourceSkills, want) {
		t.Fatalf("source skills changed: %#v", roots.SourceSkills)
	}
	if len(prepared.layers) != 1 || prepared.entries[0].Size != int64(len("layer\n")) {
		t.Fatalf("prepared = %#v", prepared)
	}
}

// v2Members is a valid schema-v2 archive, split into parts a case can change
// before they are written back into validateV2PortableMembers's arguments.
type v2Members struct {
	ref       Ref
	strict    v2PortableManifest
	template  PromptTemplate
	members   map[string]portableImageMember
	keepHash  bool
	rawBody   []byte
	rawPrompt []byte
}

func newV2Members(t *testing.T) *v2Members {
	t.Helper()
	source := t.TempDir()
	skills := filepath.Join(source, "skills")
	loop := writeTestSkill(t, skills, "loop")
	if err := os.WriteFile(filepath.Join(loop, "scripts", "loop.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := writeTestSkill(t, skills, "tasks")
	if err := os.WriteFile(filepath.Join(tasks, "scripts", "tasks.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeV2Prompt(t, source, "a.md", "layer\n")
	ref := Ref{Name: "members", Tag: "v1"}
	archive := builtArchive(t, &imagefile.V2{
		SchemaVersion: 2, ImageVersion: "1.2.3", Dir: source,
		Skills:  []imagefile.SkillEntry{{Dir: "./skills/loop"}, {Dir: "./skills/tasks"}},
		Prompts: []imagefile.PromptEntry{{Runtime: "user-prompt"}, {File: "./a.md"}},
	}, ref)
	members, _, err := readPortableMembers(archive)
	if err != nil {
		t.Fatal(err)
	}
	out := &v2Members{ref: ref, members: members}
	if err := json.Unmarshal(members["manifest.json"].body, &out.strict); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(members["prompt/template.json"].body, &out.template); err != nil {
		t.Fatal(err)
	}
	return out
}

func (m *v2Members) validate(t *testing.T, manifest *Manifest, enforce bool) error {
	t.Helper()
	if !m.keepHash {
		hash, err := templateHash(m.template.Entries)
		if err != nil {
			t.Fatal(err)
		}
		m.template.SHA256 = hash
		m.strict.PromptTemplateSHA256 = hash
	}
	body := m.rawBody
	if body == nil {
		encoded, err := json.Marshal(m.strict)
		if err != nil {
			t.Fatal(err)
		}
		body = encoded
	}
	if member, ok := m.members["prompt/template.json"]; ok {
		member.body = m.rawPrompt
		if member.body == nil {
			encoded, err := json.Marshal(m.template)
			if err != nil {
				t.Fatal(err)
			}
			member.body = encoded
		}
		m.members["prompt/template.json"] = member
	}
	return validateV2PortableMembers(body, m.members, m.ref, manifest, nil, enforce)
}

func setMember(m *v2Members, name string, change func(*portableImageMember)) {
	member := m.members[name]
	change(&member)
	m.members[name] = member
}

func TestValidateV2PortableMembersReportsEachViolation(t *testing.T) {
	const layer = "prompt/layers/001-a.md"
	withLayerSize := func(m *v2Members, size int64) {
		body := strings.Repeat("x", int(size))
		setMember(m, layer, func(member *portableImageMember) { member.body = []byte(body) })
		m.template.Entries[1].Size = size
		m.template.Entries[1].SHA256 = sha256hex([]byte(body))
	}
	tests := []struct {
		name    string
		mutate  func(*v2Members)
		enforce bool
		want    string
		prefix  bool
	}{
		{name: "manifest unknown field", mutate: func(m *v2Members) { m.rawBody = []byte(`{"mystery":1}`) }, want: "invalid schema-v2 manifest: ", prefix: true},
		{name: "manifest trailing", mutate: func(m *v2Members) { m.rawBody = []byte(`{} {}`) }, want: "invalid schema-v2 manifest: multiple JSON values: ", prefix: true},
		{name: "schema", mutate: func(m *v2Members) { m.strict.SchemaVersion = 3 }, want: "invalid schema-v2 manifest identity"},
		{name: "name", mutate: func(m *v2Members) { m.strict.Name = "other" }, want: "invalid schema-v2 manifest identity"},
		{name: "template digest empty", mutate: func(m *v2Members) { m.keepHash, m.strict.PromptTemplateSHA256 = true, "" }, want: "invalid schema-v2 manifest identity"},
		{name: "identity before version", mutate: func(m *v2Members) { m.strict.Name, m.strict.ImageVersion = "other", "bad" }, want: "invalid schema-v2 manifest identity"},
		{name: "image version", mutate: func(m *v2Members) { m.strict.ImageVersion = "bad" }, want: "image_version", prefix: true},
		{name: "version before built_at", mutate: func(m *v2Members) { m.strict.ImageVersion, m.strict.BuiltAt = "bad", "bad" }, want: "image_version", prefix: true},
		{name: "built_at", mutate: func(m *v2Members) { m.strict.BuiltAt = "bad" }, want: "invalid schema-v2 manifest built_at"},
		{name: "plugin name", mutate: func(m *v2Members) { m.strict.Plugins = []v2PortablePlugin{{Name: "BAD NAME"}} }, want: `invalid or duplicate schema-v2 plugin name "BAD NAME"`},
		{name: "plugin duplicate", mutate: func(m *v2Members) { m.strict.Plugins = []v2PortablePlugin{{Name: "loop"}, {Name: "loop"}} }, want: `invalid or duplicate schema-v2 plugin name "loop"`},
		{name: "plugins before template", mutate: func(m *v2Members) {
			m.strict.Plugins = []v2PortablePlugin{{Name: "BAD NAME"}}
			delete(m.members, "prompt/template.json")
		}, want: `invalid or duplicate schema-v2 plugin name "BAD NAME"`},
		{name: "template missing", mutate: func(m *v2Members) { delete(m.members, "prompt/template.json") }, want: "schema-v2 prompt template is missing"},
		{name: "template mode", mutate: func(m *v2Members) {
			setMember(m, "prompt/template.json", func(member *portableImageMember) { member.mode = 0o700 })
		}, want: "schema-v2 prompt template has invalid mode"},
		{name: "template json", mutate: func(m *v2Members) { m.rawPrompt = []byte(`{"mystery":1}`) }, want: "invalid schema-v2 prompt template: ", prefix: true},
		{name: "template rules", mutate: func(m *v2Members) { m.template.SchemaVersion = 3 }, want: "unsupported prompt template schema_version 3"},
		{name: "template digest", mutate: func(m *v2Members) {
			hash, err := templateHash(m.template.Entries)
			if err != nil {
				panic(err)
			}
			m.keepHash, m.template.SHA256, m.strict.PromptTemplateSHA256 = true, hash, strings.Repeat("0", 64)
		}, want: "manifest prompt template digest mismatch"},
		{name: "layer missing counts runtime entries", mutate: func(m *v2Members) { delete(m.members, layer) }, want: "template entry 1 layer is missing"},
		{name: "layer mode", mutate: func(m *v2Members) {
			setMember(m, layer, func(member *portableImageMember) { member.mode = 0o700 })
		}, want: "template entry 1 layer integrity mismatch"},
		{name: "layer size", mutate: func(m *v2Members) { m.template.Entries[1].Size++ }, want: "template entry 1 layer integrity mismatch"},
		{name: "layer digest", mutate: func(m *v2Members) { m.template.Entries[1].SHA256 = strings.Repeat("0", 64) }, want: "template entry 1 layer integrity mismatch"},
		{name: "layer aggregate", mutate: func(m *v2Members) { withLayerSize(m, maxV2PromptBytes+1) }, want: fmt.Sprintf("prompt layers exceed %d aggregate bytes", maxV2PromptBytes)},
		{name: "layers before skills", mutate: func(m *v2Members) {
			delete(m.members, layer)
			m.strict.Skills[0].Source = ""
		}, want: "template entry 1 layer is missing"},
		{name: "skill source", mutate: func(m *v2Members) { m.strict.Skills[1].Source = "" }, want: "invalid schema-v2 skill metadata at index 1"},
		{name: "skill category", mutate: func(m *v2Members) { m.strict.Skills[0].Category = "other" }, want: "invalid schema-v2 skill metadata at index 0"},
		{name: "skill root", mutate: func(m *v2Members) { m.strict.Skills[0].ArchiveRoot = "skills/other" }, want: "invalid schema-v2 skill metadata at index 0"},
		{name: "skill duplicate", mutate: func(m *v2Members) { m.strict.Skills[1] = m.strict.Skills[0] }, want: "invalid schema-v2 skill metadata at index 1"},
		{name: "skill bytes", mutate: func(m *v2Members) {
			setMember(m, "skills/loop/scripts/loop.sh", func(member *portableImageMember) { member.body = []byte("changed") })
		}, want: `invalid schema-v2 skill "loop": `, prefix: true},
		{name: "skill mode", mutate: func(m *v2Members) {
			setMember(m, "skills/loop/scripts/loop.sh", func(member *portableImageMember) { member.mode = 0o600 })
		}, want: `invalid schema-v2 skill "loop": `, prefix: true},
		{name: "skill without files", mutate: func(m *v2Members) {
			for name := range m.members {
				if strings.HasPrefix(name, "skills/tasks/") {
					delete(m.members, name)
				}
			}
		}, want: `invalid schema-v2 skill "tasks": skill "tasks" has invalid file count`},
		{name: "unexpected member", mutate: func(m *v2Members) {
			m.members["skills/unlisted/SKILL.md"] = portableImageMember{name: "skills/unlisted/SKILL.md", mode: 0o600}
		}, want: `unexpected schema-v2 image member "skills/unlisted/SKILL.md"`},
		{name: "skill prefix needs the separator", mutate: func(m *v2Members) {
			m.members["skills/loopier"] = portableImageMember{name: "skills/loopier", mode: 0o600}
		}, want: `unexpected schema-v2 image member "skills/loopier"`},
		{name: "contract skill", enforce: true, mutate: func(m *v2Members) { m.strict.Plugins = []v2PortablePlugin{{Name: "whoami"}} }, want: `plugin "whoami" requires packaged skill "whoami"`},
		{name: "contract launcher mode", enforce: true, mutate: func(m *v2Members) { m.strict.Plugins = []v2PortablePlugin{{Name: "tasks"}} }, want: `plugin "tasks" requires executable launcher "scripts/tasks.sh"`},
		{name: "contract runtime", enforce: true, mutate: func(m *v2Members) {
			m.template.Entries = append(m.template.Entries, TemplateEntry{Kind: "runtime", Runtime: "identity"})
		}, want: `runtime placeholder "identity" requires plugin "whoami"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newV2Members(t)
			tc.mutate(m)
			manifest := Manifest{Name: "untouched"}
			err := m.validate(t, &manifest, tc.enforce)
			if err == nil {
				t.Fatal("accepted invalid members")
			}
			if tc.prefix && !strings.HasPrefix(err.Error(), tc.want) || !tc.prefix && err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
			if !tc.enforce && !reflect.DeepEqual(manifest, Manifest{Name: "untouched"}) {
				t.Fatalf("manifest changed before validation passed: %#v", manifest)
			}
		})
	}
}

func TestValidateV2PortableMembersContractIsOptional(t *testing.T) {
	m := newV2Members(t)
	m.strict.Plugins = []v2PortablePlugin{{Name: "whoami"}}
	var manifest Manifest
	if err := m.validate(t, &manifest, false); err != nil {
		t.Fatalf("contract ran without enforcement: %v", err)
	}
	m.strict.Plugins = []v2PortablePlugin{{Name: "loop"}}
	if err := m.validate(t, &manifest, true); err != nil {
		t.Fatalf("executable launcher rejected: %v", err)
	}
}

func TestValidateV2PortableMembersFillsTheManifest(t *testing.T) {
	m := newV2Members(t)
	m.strict.Plugins = []v2PortablePlugin{{Name: "custom"}, {Name: "other"}}
	wantSkills := append([]agentskills.Metadata(nil), m.strict.Skills...)
	manifest := Manifest{Name: "kept", Plugins: []ManifestPlugin{{Name: "stale"}}}
	if err := m.validate(t, &manifest, false); err != nil {
		t.Fatal(err)
	}
	if want := []ManifestPlugin{{Name: "custom"}, {Name: "other"}}; !reflect.DeepEqual(manifest.Plugins, want) {
		t.Fatalf("plugins = %#v", manifest.Plugins)
	}
	if manifest.ImageVersion != "1.2.3" || manifest.Name != "kept" || !reflect.DeepEqual(manifest.Skills, wantSkills) {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestValidateV2PortableMembersMakesEmptyLists(t *testing.T) {
	m := newV2Members(t)
	m.strict.Skills, m.strict.Plugins = nil, nil
	for name := range m.members {
		if strings.HasPrefix(name, "skills/") {
			delete(m.members, name)
		}
	}
	manifest := Manifest{ImageVersion: "stale"}
	m.strict.ImageVersion = ""
	if err := m.validate(t, &manifest, false); err != nil {
		t.Fatal(err)
	}
	if manifest.Skills == nil || len(manifest.Skills) != 0 || manifest.Plugins == nil || len(manifest.Plugins) != 0 || manifest.ImageVersion != "" {
		t.Fatalf("manifest = %#v", manifest)
	}
}
