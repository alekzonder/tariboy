package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/stores"
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
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" >>\"$NPX_LOG\"\nif test \"$PWD\" = \"${NPX_UPDATE_DIR-}\"; then printf '%s\\n' 'schema_version: 2' 'image_version: 1.2.4' 'plugins: []' 'prompts: []' >Tariboyfile.yaml; fi\nif test \"$PWD\" = \"${NPX_FAIL_DIR-}\"; then exit 7; fi\n"
	if err := os.WriteFile(filepath.Join(tools, "npx"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NPX_LOG", logPath)
	t.Setenv("NPX_UPDATE_DIR", imageDir)

	result, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["name"] != "reviewer" || got["tag"] != "1.2.4" {
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

func TestImageBuildStoreSelectorUpdatesExistingVersionTag(t *testing.T) {
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
	oldSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(oldSource, "prompt.md"), []byte("old image"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldSpec := &imagefile.V2{SchemaVersion: 2, ImageVersion: "1.2.3", Dir: oldSource, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
	ref := image.Ref{Name: "reviewer", Tag: "1.2.3"}
	before, err := image.BuildV2(oldSpec, imagefile.ResolveRoots{}, ref, imageStore(c), time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	latest := image.Ref{Name: "reviewer", Tag: "latest"}
	beforeLatest, err := image.BuildV2(oldSpec, imagefile.ResolveRoots{}, latest, imageStore(c), time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}

	tools := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "npx.log")
	if err := os.WriteFile(filepath.Join(tools, "npx"), []byte("#!/bin/sh\nprintf 'called\\n' >>\"$NPX_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NPX_LOG", logPath)

	result, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	updated := result.(map[string]any)["digest"].(string)
	if updated == "" {
		t.Fatalf("build result = %#v", result)
	}
	// The declared image_version is unchanged, so the store build republishes
	// the same ref and both tags keep naming it with the new content.
	current, err := imageStore(c).Inspect(ref)
	if err != nil || current.Digest != before.Digest || current.Digest != updated {
		t.Fatalf("version ref = %#v, err %v", current, err)
	}
	currentLatest, err := imageStore(c).Inspect(latest)
	if err != nil || currentLatest.Digest != beforeLatest.Digest || currentLatest.Digest != updated {
		t.Fatalf("latest ref = %#v, err %v", currentLatest, err)
	}
	if prompt, err := imageStore(c).RenderPrompt(ref); err != nil || strings.Contains(prompt, "old image") {
		t.Fatalf("republished prompt = %q, %v", prompt, err)
	}
	if raw, err := os.ReadFile(logPath); err != nil || string(raw) != "called\ncalled\n" {
		t.Fatalf("npx log = %q, err %v", raw, err)
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

func TestStoreAutoCommandPersistsPolicy(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	writeImageV2(t, filepath.Join(source, "images", "reviewer"), "3.4.5")
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}

	command, ok := BuildRegistry().Get("store.auto")
	if !ok {
		t.Fatal("missing store.auto")
	}
	if command.HTTP == nil || *command.HTTP != (registry.HTTPRoute{Method: http.MethodPost, Path: "/api/stores/{name}/auto"}) {
		t.Fatalf("store.auto route = %#v", command.HTTP)
	}

	set, err := cmdHandler(t, "store.auto")(c, registry.Params{"name": "team", "interval": 45, "image": []string{"reviewer"}})
	if err != nil {
		t.Fatal(err)
	}
	wantAuto := map[string]any{"interval_minutes": float64(45), "images": []any{"reviewer"}}
	if got := jsonObject(t, set)["auto"]; !reflect.DeepEqual(got, wantAuto) {
		t.Fatalf("store.auto = %#v, want %#v", got, wantAuto)
	}
	shown, err := cmdHandler(t, "store.show")(c, registry.Params{"name": "team"})
	if err != nil {
		t.Fatal(err)
	}
	if got := jsonObject(t, shown)["auto"]; !reflect.DeepEqual(got, wantAuto) {
		t.Fatalf("store.show auto = %#v, want %#v", got, wantAuto)
	}

	if _, err := cmdHandler(t, "store.auto")(c, registry.Params{"name": "team", "interval": 10, "image": []string{"ghost"}}); err == nil {
		t.Fatal("unknown image was accepted")
	} else if userErr, ok := err.(api.UserError); !ok || userErr.Status != http.StatusBadRequest {
		t.Fatalf("unknown image error = %#v", err)
	}
}

func TestStoreErrorReportsRefreshFailureAsConflict(t *testing.T) {
	err := storeError(fmt.Errorf("%w team: git failed: exit status 1: error: Your local changes", stores.ErrRefresh))
	userErr, ok := err.(api.UserError)
	if !ok || userErr.Code != "store_refresh_failed" || userErr.Status != http.StatusConflict || !strings.Contains(userErr.Msg, "Your local changes") {
		t.Fatalf("storeError() = %#v", err)
	}
}

func TestImageBuildStoreSelectorAssemblesExtendsLayerByLayer(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(source, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("skills-lock.json", "root")
	write("images/parent/Tariboyfile.yaml", "schema_version: 2\nplugins: []\nskills: [{dir: ./.agents/skills/demo}]\nprompts: [{file: ./instructions.md}]\n")
	write("images/parent/instructions.md", "parent instructions")
	write("images/parent/skills-lock.json", "parent")
	write("images/child/Tariboyfile.yaml", "schema_version: 2\nimage_version: 1.0.0\nextends: [../parent]\nplugins: []\nskills: [{dir: ./.agents/skills/demo}]\nprompts: [{file: ./instructions.md}]\n")
	write("images/child/instructions.md", "child instructions")
	write("images/child/skills-lock.json", "child")
	write("images/plain/Tariboyfile.yaml", "schema_version: 2\nimage_version: 1.0.0\nextends: [../parent]\nplugins: []\nskills: []\nprompts: []\n")
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "npx.log")
	script := "#!/bin/sh\nlock=$(cat skills-lock.json)\nprintf '%s\\n' \"$lock\" >>\"$NPX_LOG\"\nif test \"$lock\" != root; then mkdir -p .agents/skills/demo && printf -- '---\\nname: demo\\ndescription: %s\\n---\\nbody\\n' \"$lock\" >.agents/skills/demo/SKILL.md; fi\n"
	if err := os.WriteFile(filepath.Join(tools, "npx"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NPX_LOG", logPath)

	demoDescription := func(name string) string {
		t.Helper()
		if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/" + name}); err != nil {
			t.Fatal(err)
		}
		manifest, err := imageStore(c).Inspect(image.Ref{Name: name, Tag: "1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if len(manifest.Skills) != 1 || manifest.Skills[0].Name != "demo" {
			t.Fatalf("%s skills = %#v", name, manifest.Skills)
		}
		return manifest.Skills[0].Description
	}
	if got := demoDescription("child"); got != "child" {
		t.Fatalf("child demo skill = %q, want the child version", got)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "root\nparent\nchild\n"; string(raw) != want {
		t.Fatalf("npx order = %q, want %q", raw, want)
	}
	if got := demoDescription("plain"); got != "parent" {
		t.Fatalf("plain demo skill = %q, want the inherited parent version", got)
	}
	for _, dir := range []string{"images/parent", "images/child"} {
		if _, err := os.Stat(filepath.Join(source, dir, ".agents")); !os.IsNotExist(err) {
			t.Fatalf("assembly wrote into the Store tree %s: %v", dir, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(c.BaseDir, "image-build")); len(entries) != 0 {
		t.Fatalf("build directories survived: %v", entries)
	}
}
