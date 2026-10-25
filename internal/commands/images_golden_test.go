package commands

import (
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

var updateImagesGolden = flag.Bool("update-images-golden", false, "rewrite testdata/images_responses.golden.json")

// imagesGoldenVolatile names keys whose values depend on the wall clock or on
// archive bytes that embed it.
var imagesGoldenVolatile = map[string]bool{
	"built_at": true, "created_at": true, "updated_at": true,
	"digest": true, "source_digest": true,
}

// imagesGoldenCommit is the Git commit recorded as explicit build provenance.
const imagesGoldenCommit = "97cf20ec0c8542f54b68904521be6a2ca85552a1"

// imagesFixtureDirs are the temporary directories of an images fixture.
type imagesFixtureDirs struct {
	root, versioned, plain, store string
}

// imagesFixture returns a context with an isolated base directory and the
// sources the images and stores commands read: a versioned source with a skill
// and a cwd-scope duplicate of it, an unversioned source, and a local Store
// with a valid image, a broken image and a broken workflow.
func imagesFixture(t *testing.T) (*registry.Ctx, imagesFixtureDirs) {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "base")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	c := &registry.Ctx{
		Store:   storetest.OpenAt(t, filepath.Join(base, "state.db")),
		BaseDir: base,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	dirs := imagesFixtureDirs{
		root:      root,
		versioned: filepath.Join(root, "versioned"),
		plain:     filepath.Join(root, "plain"),
		store:     filepath.Join(root, "store"),
	}
	skill := "---\nname: golden-review\ndescription: Review golden changes.\n---\n# Review\n"
	imagesWriteFiles(t, dirs.versioned, map[string]string{
		"Tariboyfile.yaml":                        "schema_version: 2\nimage_version: 1.2.0\nplugins: []\nskills:\n  - dir: ./skills/golden-review\nprompts:\n  - file: ./task.md\n",
		"task.md":                                 "REVIEW THE CHANGE",
		"skills/golden-review/SKILL.md":           skill,
		".agents/skills/golden-review/SKILL.md":   skill,
		"skills/golden-review/notes/checklist.md": "check",
	})
	imagesWriteFiles(t, dirs.plain, map[string]string{
		"Tariboyfile.yaml": "schema_version: 2\nprompts:\n  - file: ./task.md\n",
		"task.md":          "BE A TEST AGENT",
	})
	writeImageV2(t, filepath.Join(dirs.store, "images", "reviewer"), "3.4.5")
	imagesWriteFiles(t, dirs.store, map[string]string{
		"images/broken/README.md":    "no Tariboyfile",
		"workflows/broken/README.md": "no workflow file",
	})
	return c, dirs
}

func imagesWriteFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// imagesSetupAgents stores agents that use built refs, so image.ls reports
// current and pending agents.
func imagesSetupAgents(t *testing.T, c *registry.Ctx) {
	t.Helper()
	if _, err := c.Store.DB.Exec(`INSERT INTO agents(name, image_ref, image_digest, pending_image_ref) VALUES
		('dev-a', 'reviewer:latest', 'digest', ''),
		('dev-b', 'reviewer:latest', 'digest', 'demo:v1')`); err != nil {
		t.Fatal(err)
	}
}

// imagesGoldenCall is one handler call of the images golden.
type imagesGoldenCall struct {
	label, path string
	params      registry.Params
}

func imagesGoldenCalls(dirs imagesFixtureDirs) []imagesGoldenCall {
	p := func(kv ...any) registry.Params {
		params := registry.Params{}
		for i := 0; i+1 < len(kv); i += 2 {
			params[kv[i].(string)] = kv[i+1]
		}
		return params
	}
	return []imagesGoldenCall{
		{"image.build default tags", "image.build", p("name", "reviewer", "path", dirs.versioned)},
		{"image.build two tags", "image.build", p("name", "demo", "tag", []string{"v1", "v2"}, "path", dirs.plain, "repository-id", "tariboy", "git-commit", imagesGoldenCommit)},
		{"image.build one tag", "image.build", p("name", "solo", "tag", []string{"latest"}, "path", dirs.plain)},
		{"image.validate valid", "image.validate", p("name", "reviewer", "path", dirs.versioned)},
		{"image.validate bad name", "image.validate", p("name", "", "path", dirs.versioned)},
		{"image.validate bad path", "image.validate", p("name", "reviewer", "path", filepath.Join(dirs.root, "missing"))},
		{"image.source.create", "image.source.create", p("name", "drafter", "prompt", "Draft the reply.")},
		{"image.source.create broken", "image.source.create", p("name", "broken")},
		{"image.source.file.put", "image.source.file.put", p("name", "broken", "path", "Tariboyfile.yaml", "content", "schema_version: [")},
		{"image.source.validate valid", "image.source.validate", p("name", "drafter")},
		{"image.source.validate invalid", "image.source.validate", p("name", "broken")},
		{"image.source.build", "image.source.build", p("name", "drafter")},
		{"image.source.ls", "image.source.ls", p()},
		{"image.source.inspect built", "image.source.inspect", p("name", "drafter")},
		{"image.source.inspect unbuilt", "image.source.inspect", p("name", "broken")},
		{"image.source.files", "image.source.files", p("name", "drafter")},
		{"image.source.file.get", "image.source.file.get", p("name", "drafter", "path", "PROMPT.md")},
		{"image.ls", "image.ls", p()},
		{"image.inspect versioned", "image.inspect", p("ref", "reviewer:latest")},
		{"image.inspect plain", "image.inspect", p("ref", "demo:v1")},
		{"image.template", "image.template", p("ref", "reviewer:latest")},
		{"image.prompt", "image.prompt", p("ref", "reviewer:latest")},
		{"image.files", "image.files", p("ref", "reviewer:latest")},
		{"image.file", "image.file", p("ref", "reviewer:latest", "path", "prompt/layers/000-task.md")},
		{"image.provenance with snapshot", "image.provenance", p("ref", "demo:v1")},
		{"image.provenance versioned", "image.provenance", p("ref", "reviewer:1.2.0")},
		{"image.provenance absent", "image.provenance", p("ref", "drafter:latest")},
		{"image.rm", "image.rm", p("ref", "solo:latest")},
		{"image.source.rm", "image.source.rm", p("name", "broken")},
	}
}

// imagesStableSizes masks the size of each listed manifest.json: the manifest
// embeds built_at, whose encoded length varies with its trailing zeros.
func imagesStableSizes(t *testing.T, value any) any {
	t.Helper()
	object := jsonObject(t, value)
	files, _ := object["files"].([]any)
	for _, file := range files {
		if entry, ok := file.(map[string]any); ok && entry["path"] == "manifest.json" {
			entry["size"] = "$VOLATILE"
		}
	}
	if files == nil {
		return value
	}
	return object
}

// Catches a typed images response changing the JSON a client sees:
// the set of keys, their JSON types, and their values must match the recorded
// responses.
func TestImagesResponsesMatchGolden(t *testing.T) {
	c, dirs := imagesFixture(t)
	imagesSetupAgents(t, c)
	resolvedRoot, err := filepath.EvalSymlinks(dirs.root)
	if err != nil {
		t.Fatal(err)
	}
	recorder := newGoldenRecorder(t, imagesGoldenVolatile, resolvedRoot, "$ROOT", dirs.root, "$ROOT")
	for _, call := range imagesGoldenCalls(dirs) {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, imagesStableSizes(t, value))
	}
	recorder.check("images_responses.golden.json", *updateImagesGolden)
}
