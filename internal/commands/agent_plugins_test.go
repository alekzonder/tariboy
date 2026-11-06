package commands

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
)

// pluginsAgentCtx returns a context with agent a1 on the built schema-v2
// image worker:latest, whose plugins are loop, mail and tasks.
func pluginsAgentCtx(t *testing.T) (*registry.Ctx, *agent.Store) {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	source := &imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}
	for name, launcher := range map[string]string{"loop": "scripts/loop.sh", "mail": "", "tasks": "scripts/tasks.sh"} {
		dir := filepath.Join(source.Dir, "skills", name)
		write := func(rel, body string, mode os.FileMode) {
			t.Helper()
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), mode); err != nil {
				t.Fatal(err)
			}
		}
		write("SKILL.md", "---\nname: "+name+"\ndescription: Test "+name+".\n---\n", 0o600)
		if launcher != "" {
			write(launcher, "#!/bin/sh\n", 0o700)
		}
		source.Plugins = append(source.Plugins, imagefile.V2Plugin{Name: name})
		source.Skills = append(source.Skills, imagefile.SkillEntry{Dir: "./skills/" + name})
	}
	sort.Slice(source.Plugins, func(i, j int) bool { return source.Plugins[i].Name < source.Plugins[j].Name })
	ref := image.Ref{Name: "worker", Tag: "latest"}
	manifest, err := image.BuildV2(source, imagefile.ResolveRoots{}, ref, imageStore(c), time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{Name: "a1", ImageRef: ref.String(), ImageDigest: manifest.Digest, Plugins: []string{"loop", "mail", "tasks"}, OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	return c, as
}

func TestAgentPluginsReadsAndReplacesList(t *testing.T) {
	t.Parallel()
	c, as := pluginsAgentCtx(t)
	read, err := h(t, "agent.plugins")(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (AgentPluginsResult{Name: "a1", Plugins: []string{"loop", "mail", "tasks"}}); !reflect.DeepEqual(read, want) {
		t.Fatalf("read = %#v, want %#v", read, want)
	}
	for _, value := range []any{"loop,mail", []any{"loop", "mail"}} {
		set, err := h(t, "agent.plugins")(c, registry.Params{"name": "a1", "value": value})
		if err != nil {
			t.Fatal(err)
		}
		if want := (AgentPluginsResult{Name: "a1", Plugins: []string{"loop", "mail"}}); !reflect.DeepEqual(set, want) {
			t.Fatalf("set %#v = %#v, want %#v", value, set, want)
		}
		if a, err := as.Get("a1"); err != nil || !reflect.DeepEqual(a.Plugins, []string{"loop", "mail"}) {
			t.Fatalf("stored plugins = %v, %v", a.Plugins, err)
		}
	}
	empty, err := h(t, "agent.plugins")(c, registry.Params{"name": "a1", "value": ""})
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.(AgentPluginsResult).Plugins; got == nil || len(got) != 0 {
		t.Fatalf("empty list = %#v, want [] not null", got)
	}
}

func TestAgentPluginsRejectsUnknownAndForeignPlugins(t *testing.T) {
	t.Parallel()
	c, as := pluginsAgentCtx(t)
	for _, test := range []struct{ value, code string }{
		{"loop,nope", "bad_plugins"},
		{"loop,loop", "bad_plugins"},
		{"loop,schedule", "plugin_not_in_image"},
	} {
		_, err := h(t, "agent.plugins")(c, registry.Params{"name": "a1", "value": test.value})
		userErr, ok := err.(api.UserError)
		if !ok || userErr.Code != test.code || userErr.Status != http.StatusBadRequest {
			t.Fatalf("value %q = %#v, want %s", test.value, err, test.code)
		}
	}
	if a, err := as.Get("a1"); err != nil || !reflect.DeepEqual(a.Plugins, []string{"loop", "mail", "tasks"}) {
		t.Fatalf("plugins after rejected values = %v, %v", a.Plugins, err)
	}
	if _, err := h(t, "agent.plugins")(c, registry.Params{"name": "ghost"}); err == nil {
		t.Fatal("unknown agent accepted")
	}
}
