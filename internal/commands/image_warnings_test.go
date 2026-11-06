package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agentskills"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/plugins"
)

// Catches a warning being lost or changing place: the list is shown in this
// order, prompts first, then skills, then the capability checks.
func TestV2ValidationWarningsOrder(t *testing.T) {
	// Sequential: sets HOME with t.Setenv.
	t.Setenv("HOME", t.TempDir())
	c, _, _ := ctxWithStore(t)
	pluginStore := plugins.NewStore(c.Store, time.Now)
	if err := pluginStore.Upsert(plugins.Record{Name: "loop", Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := pluginStore.SetActiveVersion("loop", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "code-review")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: code-review\ndescription: Review changes safely.\n---\n# Review\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	source := &imagefile.V2{Dir: dir, Plugins: []imagefile.V2Plugin{{Name: "loop"}, {Name: "context"}, {Name: "mail"}}}
	validated := image.ValidationV2{
		Template: image.PromptTemplate{Entries: []image.TemplateEntry{
			{Kind: "runtime", Runtime: "context"},
			{Kind: "file", Source: "/abs/big.md", Size: 128 << 10},
			{Kind: "file", Source: "$PLUGINS/loop/1.0.0/prompt.md"},
			{Kind: "file", Source: "$PLUGINS/loop/2.0.0/prompt.md"},
			{Kind: "file", Source: "$PLUGINS/short/1.0.0"},
			{Kind: "file", Source: "./store/skills/context/guide.md"},
			{Kind: "file", Source: "small.md", Size: 128<<10 - 1},
		}},
		Skills: []agentskills.Metadata{{Name: "first", Source: "/abs/first"}, {Name: "code-review", Source: "./skills/code-review"}},
	}
	got := v2ValidationWarnings(source, validated, pluginStore)
	want := []ImageDiagnostic{
		{Path: "prompts[1]", Message: "absolute prompt path makes the original source host-bound"},
		{Path: "prompts[1]", Message: "large static prompt may exceed an interactive harness argument limit"},
		{Path: "prompts[2]", Message: "static plugin version loop/1.0.0 differs from active runtime version 2.0.0"},
		{Path: "skills[0]", Message: "absolute skill path makes the original source host-bound"},
		{Path: "skills[1]", Message: "skill code-review is also visible in cwd scope; native harness precedence applies"},
		{Path: "prompts", Message: "identity placeholder is omitted"},
		{Path: "prompts", Message: "loop capability has no visible finishing instruction layer"},
		{Path: "plugins", Message: "plugin mail has no obvious static instruction layer"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings:\n got %#v\nwant %#v", got, want)
	}
}

// Catches the conditions that silence a warning, and a nil plugin store.
func TestV2ValidationWarningsSatisfied(t *testing.T) {
	// Sequential: sets HOME with t.Setenv.
	t.Setenv("HOME", t.TempDir())
	source := &imagefile.V2{Dir: t.TempDir(), Plugins: []imagefile.V2Plugin{{Name: "loop"}}}
	validated := image.ValidationV2{Template: image.PromptTemplate{Entries: []image.TemplateEntry{
		{Kind: "runtime", Runtime: "identity"},
		{Kind: "file", Source: "$PLUGINS/loop/1.0.0/loop/finish.md"},
	}}}
	got := v2ValidationWarnings(source, validated, nil)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("warnings = %s, want an empty JSON array", raw)
	}
}

// Catches a missing home directory dropping the other warnings.
func TestV2ValidationWarningsWithoutHome(t *testing.T) {
	// Sequential: sets HOME with t.Setenv.
	t.Setenv("HOME", "")
	got := v2ValidationWarnings(&imagefile.V2{Dir: t.TempDir()}, image.ValidationV2{Skills: []agentskills.Metadata{{Name: "first", Source: "/abs/first"}}}, nil)
	want := []ImageDiagnostic{
		{Path: "skills[0]", Message: "absolute skill path makes the original source host-bound"},
		{Path: "prompts", Message: "identity placeholder is omitted"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings:\n got %#v\nwant %#v", got, want)
	}
}
