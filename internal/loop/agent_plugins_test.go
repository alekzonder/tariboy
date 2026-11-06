package loop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// Catches a plugin list changed between iterations (agent.plugins) that the
// next launch does not apply to the daemon-owned shims.
func TestLaunchGateRewritesShimsForChangedPlugins(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	as := agent.NewStore(storetest.OpenAt(t, filepath.Join(base, "x.db")))
	images := &image.Store{Dir: filepath.Join(base, "images")}
	skills := testSkillsDir(t)
	all := []string{"whoami", "loop", "messages", "context", "tasks"}
	source := &imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}
	for _, name := range all {
		source.Plugins = append(source.Plugins, imagefile.V2Plugin{Name: name})
		source.Skills = append(source.Skills, imagefile.SkillEntry{Dir: filepath.Join(skills, name)})
	}
	ref := image.Ref{Name: "worker", Tag: "latest"}
	manifest, err := image.BuildV2(source, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: manifest.Digest, HarnessType: "stub", Plugins: all}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, ref); err != nil {
		t.Fatal(err)
	}
	if err := agentdir.WriteShims(l, ag); err != nil {
		t.Fatal(err)
	}
	tasksShim, doneShim := filepath.Join(l.BinDir(), "tasks"), filepath.Join(l.BinDir(), "i-am-done")
	if _, err := os.Stat(tasksShim); err != nil {
		t.Fatalf("tasks shim before the change: %v", err)
	}

	stored, err := as.Get(ag.Name)
	if err != nil {
		t.Fatal(err)
	}
	stored.Plugins = []string{"whoami", "loop", "messages", "context"}
	if err := as.Update(stored); err != nil {
		t.Fatal(err)
	}
	next, err := as.Get(ag.Name) // what Engine.reload hands the launch gate
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images}}
	if _, err := m.activatePendingImage(&next); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tasksShim); !os.IsNotExist(err) {
		t.Fatalf("tasks shim after removing the plugin: %v, want removed", err)
	}
	if _, err := os.Stat(doneShim); err != nil {
		t.Fatalf("i-am-done shim: %v", err)
	}
}
