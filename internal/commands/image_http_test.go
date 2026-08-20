package commands

import (
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
)

// routed returns a now-HTTP-routed command's handler (mirrors handler() but for
// commands that DO carry an HTTPRoute).
func routed(t *testing.T, path string) registry.HandlerFunc {
	t.Helper()
	cmd, ok := BuildRegistry().Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	if cmd.HTTP == nil {
		t.Fatalf("%s should now carry an HTTPRoute", path)
	}
	return cmd.Handler
}

func TestImageRoutesRegistered(t *testing.T) {
	for _, tc := range []struct{ path, method, route string }{
		{"image.ls", "GET", "/api/images"},
		{"image.inspect", "GET", "/api/images/{ref}"},
		{"image.prompt", "GET", "/api/images/{ref}/prompt"},
		{"image.files", "GET", "/api/images/{ref}/files"},
		{"image.file", "GET", "/api/images/{ref}/files/{path...}"},
		{"image.rm", "DELETE", "/api/images/{ref}"},
	} {
		cmd, ok := BuildRegistry().Get(tc.path)
		if !ok {
			t.Fatalf("%s not registered", tc.path)
		}
		if cmd.HTTP == nil || cmd.HTTP.Method != tc.method || cmd.HTTP.Path != tc.route {
			t.Fatalf("%s route = %+v, want %s %s", tc.path, cmd.HTTP, tc.method, tc.route)
		}
	}
}

func TestImageLsInspectRmViaHandler(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t) // from image_test.go: writes a Tariboyfile + task.md

	// Call the build handler directly to seed one image.
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": "demo:latest", "path": src}); err != nil {
		t.Fatal(err)
	}

	ls, err := routed(t, "image.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ls: %v", ls)
	}
	images := ls.(map[string]any)["images"].([]map[string]any)
	if images[0]["schema_version"] != 1 {
		t.Fatalf("ls image schema_version = %v, want 1", images[0]["schema_version"])
	}

	if _, err := routed(t, "image.inspect")(c, registry.Params{"ref": "demo:latest"}); err != nil {
		t.Fatal(err)
	}

	if _, err := routed(t, "image.rm")(c, registry.Params{"ref": "demo:latest"}); err != nil {
		t.Fatal(err)
	}
	ls, _ = routed(t, "image.ls")(c, registry.Params{})
	if ls.(map[string]any)["count"].(int) != 0 {
		t.Fatalf("image not removed: %v", ls)
	}
}

func TestImagePromptFilesReadViaHandler(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": "demo:latest", "path": src}); err != nil {
		t.Fatal(err)
	}

	// prompt route returns {"prompt": ...}
	pr, err := routed(t, "image.prompt")(c, registry.Params{"ref": "demo:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := pr.(map[string]any)["prompt"].(string); !strings.Contains(s, "BE A TEST AGENT") {
		t.Fatalf("prompt route body: %v", pr)
	}

	// files route lists the packed members
	fl, err := routed(t, "image.files")(c, registry.Params{"ref": "demo:latest"})
	if err != nil {
		t.Fatal(err)
	}
	m := fl.(map[string]any)
	if m["count"].(int) < 4 {
		t.Fatalf("files count too low: %v", m)
	}
	files := m["files"].([]image.FileEntry)
	var haveManifest bool
	for _, e := range files {
		if e.Path == "manifest.json" {
			haveManifest = true
		}
	}
	if !haveManifest {
		t.Fatalf("files route missing manifest.json: %+v", files)
	}

	// file read route returns raw content
	rd, err := routed(t, "image.file")(c, registry.Params{"ref": "demo:latest", "path": "manifest.json"})
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := rd.(map[string]any)["content"].(string); !strings.Contains(content, "\"name\"") {
		t.Fatalf("file read content: %v", rd)
	}

	// traversal is rejected as a user error
	if _, err := routed(t, "image.file")(c, registry.Params{"ref": "demo:latest", "path": "../etc/passwd"}); err == nil {
		t.Fatal("traversal path should be rejected")
	}

	// absent image surfaces not_found
	if _, err := routed(t, "image.files")(c, registry.Params{"ref": "ghost:latest"}); err == nil {
		t.Fatal("files of absent image should error")
	}
}
