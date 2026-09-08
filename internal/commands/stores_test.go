package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestStoreCommandsRegisteredWithRoutes(t *testing.T) {
	want := map[string]registry.HTTPRoute{
		"store.add":     {Method: http.MethodPost, Path: "/api/stores"},
		"store.list":    {Method: http.MethodGet, Path: "/api/stores"},
		"store.show":    {Method: http.MethodGet, Path: "/api/stores/{name}"},
		"store.refresh": {Method: http.MethodPost, Path: "/api/stores/{name}/refresh"},
		"store.remove":  {Method: http.MethodDelete, Path: "/api/stores/{name}"},
	}
	reg := BuildRegistry()
	for path, route := range want {
		command, ok := reg.Get(path)
		if !ok {
			t.Fatalf("missing %s", path)
		}
		if command.HTTP == nil || *command.HTTP != route {
			t.Fatalf("%s route = %#v, want %#v", path, command.HTTP, route)
		}
	}
	if _, ok := reg.Group("store"); !ok {
		t.Fatal("missing store group")
	}
}

func TestStoreCommandLifecycleReturnsHTTPShapes(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	writeImageV2(t, filepath.Join(source, "images", "reviewer"), "3.4.5")

	added, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source})
	if err != nil {
		t.Fatal(err)
	}
	wantStore := map[string]any{"name": "team", "source": source, "path": source}
	if got := jsonObject(t, added); !reflect.DeepEqual(got, wantStore) {
		t.Fatalf("add = %#v, want %#v", got, wantStore)
	}
	listed, err := cmdHandler(t, "store.list")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	var stores []map[string]any
	roundTripJSON(t, listed, &stores)
	if !reflect.DeepEqual(stores, []map[string]any{wantStore}) {
		t.Fatalf("list = %#v", stores)
	}
	shown, err := cmdHandler(t, "store.show")(c, registry.Params{"name": "team"})
	if err != nil {
		t.Fatal(err)
	}
	detail := jsonObject(t, shown)
	images := detail["images"].([]any)
	if len(images) != 1 || images[0].(map[string]any)["version"] != "3.4.5" {
		t.Fatalf("show = %#v", detail)
	}
	if _, err := cmdHandler(t, "store.refresh")(c, registry.Params{"name": "team"}); err != nil {
		t.Fatal(err)
	}
	removed, err := cmdHandler(t, "store.remove")(c, registry.Params{"name": "team"})
	if err != nil {
		t.Fatal(err)
	}
	if got := jsonObject(t, removed); !reflect.DeepEqual(got, map[string]any{"removed": true}) {
		t.Fatalf("remove = %#v", got)
	}
}

func writeImageV2(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "schema_version: 2\nimage_version: " + version + "\nplugins: []\nprompts: []\n"
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func roundTripJSON(t *testing.T, value any, dst any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatal(err)
	}
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	var result map[string]any
	roundTripJSON(t, value, &result)
	return result
}

func TestImageBuildStoreSelectorInstallsLocksAndDefaultsIdentity(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	imageDir := filepath.Join(source, "images", "reviewer")
	writeImageV2(t, imageDir, "1.2.3")
	for _, dir := range []string{source, imageDir} {
		if err := os.WriteFile(filepath.Join(dir, "skills-lock.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}

	tools := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "npx.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" >>\"$NPX_LOG\"\nif test \"$PWD\" = \"${NPX_FAIL_DIR-}\"; then exit 7; fi\n"
	if err := os.WriteFile(filepath.Join(tools, "npx"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NPX_LOG", logPath)

	result, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["name"] != "reviewer" || got["tag"] != "1.2.3" {
		t.Fatalf("build result = %#v", got)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := source + "\n" + imageDir + "\n"; string(raw) != want {
		t.Fatalf("npx order = %q, want %q", raw, want)
	}

	failedDir := filepath.Join(source, "images", "failed")
	writeImageV2(t, failedDir, "2.0.0")
	if err := os.WriteFile(filepath.Join(failedDir, "skills-lock.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NPX_FAIL_DIR", failedDir)
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/failed"}); err == nil {
		t.Fatal("build succeeded despite image lock installation failure")
	}
	if imageStore(c).Exists(image.Ref{Name: "failed", Tag: "2.0.0"}) {
		t.Fatal("failed lock installation published an image")
	}
}

func TestImageBuildStoreSelectorRejectsTraversalAndExplicitPath(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	writeImageV2(t, filepath.Join(source, "images", "reviewer"), "1.0.0")
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"../reviewer", "team/../reviewer", "team/reviewer/extra", "/team/reviewer", "team//reviewer"} {
		if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": selector}); err == nil {
			t.Fatalf("unsafe selector %q accepted", selector)
		}
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer", "path": filepath.Join(source, "images", "reviewer")}); err == nil {
		t.Fatal("source and path accepted together")
	}
}

func TestImageBuildStoreSelectorRejectsLinkedImagesDirectory(t *testing.T) {
	c := localCtx(t)
	source, outside := t.TempDir(), t.TempDir()
	writeImageV2(t, filepath.Join(outside, "reviewer"), "1.0.0")
	if err := os.Symlink(outside, filepath.Join(source, "images")); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer"}); err == nil {
		t.Fatal("selector followed a linked images directory")
	}
	if imageStore(c).Exists(image.Ref{Name: "reviewer", Tag: "1.0.0"}) {
		t.Fatal("unsafe selector published an image")
	}
}

func TestImageBuildPathCompatibility(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	writeImageV2(t, source, "4.5.6")
	result, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "legacy", "path": source})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.(map[string]any); got["name"] != "legacy" || got["tag"] != "4.5.6" {
		t.Fatalf("path build = %#v", got)
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "missing-path"}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("missing path error = %v", err)
	}
}
