package loop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/builtinimages"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imageportable"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/alekzonder/tariboy/internal/registry"
	storedb "github.com/alekzonder/tariboy/internal/store"
)

type imageBoundaryCall struct {
	ref    string
	digest string
}

type imageBoundaryRunner struct {
	calls   chan imageBoundaryCall
	release chan struct{}
}

func TestActiveImageLaunchRejectsDigestDifferentFromPinnedAgentIdentity(t *testing.T) {
	m, _, _, _ := newManager(t, &fakeRunner{})
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("trusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "active", Tag: "latest"}
	manifest, err := image.BuildV2(
		&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}},
		imagefile.ResolveRoots{}, ref, m.cfg.ImgStore, time.Now, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: strings.Repeat("0", len(manifest.Digest))}
	if err := m.cfg.Store.Create(ag); err != nil {
		t.Fatal(err)
	}
	if _, err := m.activatePendingImage(&ag); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("active launch accepted archive outside pinned digest: %v", err)
	}
}

func TestMutableRefActivatesAtNextLaunchGate(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	source := t.TempDir()
	build := func(body string) image.Manifest {
		if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest, err := image.BuildV2Mutable(&imagefile.V2{
			SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}},
		}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := build("first")
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: first.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, ref, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	second := build("second")
	if second.Digest == first.Digest {
		t.Fatal("test setup did not replace the mutable ref")
	}
	stored, err := as.Get(ag.Name)
	if err != nil || stored.ImageDigest != first.Digest {
		t.Fatalf("agent changed before launch gate: %+v err=%v", stored, err)
	}
	localDigest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
	if err != nil || strings.TrimSpace(string(localDigest)) != first.Digest {
		t.Fatalf("unpacked image changed before launch gate: %q err=%v", localDigest, err)
	}
	recorder := &captureRecorder{}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true", AuditFor: func(string) Recorder { return recorder }}}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	if ag.ImageDigest != second.Digest {
		t.Fatalf("activated digest=%s want %s", ag.ImageDigest, second.Digest)
	}
	stored, err = as.Get(ag.Name)
	if err != nil || stored.ImageDigest != second.Digest {
		t.Fatalf("stored activated digest=%s err=%v want %s", stored.ImageDigest, err, second.Digest)
	}
	if _, err := images.InspectPinned(ref, first.Digest); err != nil {
		t.Fatalf("old mutable digest is not pinned-readable: %v", err)
	}
	events := recorder.snapshot()
	if len(events) != 1 || events[0].data["old_digest"] != first.Digest || events[0].data["new_digest"] != second.Digest {
		t.Fatalf("activation audit=%#v", events)
	}
}

func TestMutableRefDiscoveryFailureRecordsRetryablePendingError(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: first.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, ref, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	archive, err := images.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(images.Dir, ref.Name, ref.Tag+".tar.gz")); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	if _, err := m.activatePendingImage(&ag); err == nil {
		t.Fatal("mutable discovery failure did not fail activation")
	}
	pending, err := as.PendingImage(ag.Name)
	if err != nil || pending.Ref != "" || pending.Digest != "" || pending.Error == "" {
		t.Fatalf("discovery failure pending=%+v err=%v", pending, err)
	}
	stored, err := as.Get(ag.Name)
	if err != nil || stored.ImageDigest != first.Digest {
		t.Fatalf("active agent changed after discovery failure: %+v err=%v", stored, err)
	}
	localDigest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
	if err != nil || strings.TrimSpace(string(localDigest)) != first.Digest {
		t.Fatalf("active bytes changed after discovery failure: %q err=%v", localDigest, err)
	}
	if err := os.WriteFile(filepath.Join(images.Dir, ref.Name, ref.Tag+".tar.gz"), archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	pending, err = as.PendingImage(ag.Name)
	if err != nil || pending.Ref != "" || pending.Digest != "" || pending.Error != "" {
		t.Fatalf("successful unchanged retry pending=%+v err=%v", pending, err)
	}
}

func TestRecoveredSwapShimFailureRecordsPendingImageError(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	build := func(name string) (image.Ref, image.Manifest) {
		ref := image.Ref{Name: name, Tag: "latest"}
		manifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref, manifest
	}
	activeRef, active := build("active")
	pendingRef, pendingManifest := build("pending")
	ag := agent.Agent{Name: "worker", ImageRef: activeRef.String(), ImageDigest: active.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, activeRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage(ag.Name, pendingRef.String(), pendingManifest.Digest); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(l.ImageDir(), filepath.Join(l.Root, ".image-backup")); err != nil {
		t.Fatal(err)
	}
	if err := images.Unpack(pendingRef, l.ImageDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(l.BinDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.BinDir(), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	if _, err := m.activatePendingImage(&ag); err == nil {
		t.Fatal("recovered swap shim failure did not fail activation")
	}
	pending, err := as.PendingImage(ag.Name)
	if err != nil || pending.Ref != pendingRef.String() || pending.Digest != pendingManifest.Digest || pending.Error == "" {
		t.Fatalf("recovered swap pending=%+v err=%v", pending, err)
	}
	stored, err := as.Get(ag.Name)
	if err != nil || stored.ImageDigest != active.Digest {
		t.Fatalf("active agent changed after recovered swap failure: %+v err=%v", stored, err)
	}
}

func TestMutableRefreshLosesToExplicitPendingDuringStaging(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	mutableRef := image.Ref{Name: "reviewer", Tag: "latest"}
	source := t.TempDir()
	buildMutable := func(body string, plugins []imagefile.V2Plugin) image.Manifest {
		if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Dir: source, Plugins: plugins, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}, imagefile.ResolveRoots{}, mutableRef, images, time.Now, func(string) (plugincaps.ResolvedPlugin, error) {
			return plugincaps.ResolvedPlugin{Installed: true}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := buildMutable("first", nil)
	explicitRef := image.Ref{Name: "explicit", Tag: "v1"}
	explicit, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, explicitRef, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: mutableRef.String(), ImageDigest: first.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, mutableRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	second := buildMutable("second", []imagefile.V2Plugin{{Name: "external-widget"}})
	if second.Digest == first.Digest {
		t.Fatal("test setup did not replace mutable ref")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	recorder := &captureRecorder{}
	m := &Manager{cfg: ManagerConfig{
		AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true", AuditFor: func(string) Recorder { return recorder },
		ExternalPlugins: func(string) (plugincaps.ResolvedPlugin, error) {
			enteredOnce.Do(func() { close(entered) })
			<-release
			return plugincaps.ResolvedPlugin{Installed: true}, nil
		},
	}}
	done := make(chan error, 1)
	finished := make(chan struct{})
	releaseResolver := func() { releaseOnce.Do(func() { close(release) }) }
	go func() {
		defer close(finished)
		_, err := m.activatePendingImage(&ag)
		done <- err
	}()
	t.Cleanup(func() {
		releaseResolver()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("blocked activation did not finish")
		}
	})
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("automatic activation did not reach plugin validation")
	}
	if err := as.SetPendingImage(ag.Name, explicitRef.String(), explicit.Digest); err != nil {
		t.Fatal(err)
	}
	releaseResolver()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("automatic activation won after explicit pending replacement")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("automatic activation did not finish after explicit replacement")
	}
	stored, err := as.Get(ag.Name)
	if err != nil || stored.ImageDigest != first.Digest {
		t.Fatalf("active agent changed after conditional promotion loss: %+v err=%v", stored, err)
	}
	localDigest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
	if err != nil || strings.TrimSpace(string(localDigest)) != first.Digest {
		t.Fatalf("active bytes changed after conditional promotion loss: %q err=%v", localDigest, err)
	}
	pending, err := as.PendingImage(ag.Name)
	if err != nil || pending.Ref != explicitRef.String() || pending.Digest != explicit.Digest || pending.Error != "" {
		t.Fatalf("explicit pending after conditional promotion loss=%+v err=%v", pending, err)
	}
	if events := recorder.snapshot(); len(events) != 0 {
		t.Fatalf("automatic activation audited despite rollback: %#v", events)
	}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	if ag.ImageRef != explicitRef.String() || ag.ImageDigest != explicit.Digest {
		t.Fatalf("explicit pending retry did not activate: %+v", ag)
	}
	events := recorder.snapshot()
	if len(events) != 1 || events[0].data["old_digest"] != first.Digest || events[0].data["new_digest"] != explicit.Digest {
		t.Fatalf("explicit retry audit=%#v", events)
	}
}

func TestImmutableRefRemainsPinned(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	ref := image.Ref{Name: "immutable", Tag: "v1"}
	manifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: manifest.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, ref, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	if ag.ImageDigest != manifest.Digest {
		t.Fatalf("immutable ref advanced to %s", ag.ImageDigest)
	}
}

func TestExplicitPendingImageBeatsMutableRefresh(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	mutableRef := image.Ref{Name: "reviewer", Tag: "latest"}
	source := t.TempDir()
	buildMutable := func(body string) image.Manifest {
		if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}, imagefile.ResolveRoots{}, mutableRef, images, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := buildMutable("first")
	explicitRef := image.Ref{Name: "explicit", Tag: "v1"}
	explicit, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, explicitRef, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: mutableRef.String(), ImageDigest: first.Digest}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, mutableRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	_ = buildMutable("second")
	if err := as.SetPendingImage(ag.Name, explicitRef.String(), explicit.Digest); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	if ag.ImageRef != explicitRef.String() || ag.ImageDigest != explicit.Digest {
		t.Fatalf("explicit pending assignment lost to mutable refresh: %+v", ag)
	}
}

func (r *imageBoundaryRunner) Run(_ context.Context, ag agent.Agent, _, _, _ string) (Outcome, error) {
	r.calls <- imageBoundaryCall{ref: ag.ImageRef, digest: ag.ImageDigest}
	if ag.ImageRef == "image-a:latest" {
		<-r.release
	}
	return Outcome{Status: "done", DoneFlag: true}, nil
}

func TestPendingImageActivationWaitsForActiveIterationAndSnapshotsNextImage(t *testing.T) {
	runner := &imageBoundaryRunner{calls: make(chan imageBoundaryCall, 2), release: make(chan struct{})}
	m, as, _, _ := newManager(t, runner)
	sources := make(map[string]string)
	build := func(name, body string) image.Manifest {
		source := t.TempDir()
		sources[name] = source
		if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest, err := image.BuildV2(
			&imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}},
			imagefile.ResolveRoots{}, image.Ref{Name: name, Tag: "latest"}, m.cfg.ImgStore, time.Now, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	aManifest := build("image-a", "prompt A")
	bManifest := build("image-b", "prompt B")
	if _, err := m.Run(registry.RunSpec{ImageRef: "image-a:latest", Name: "worker", Harness: "stub"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Shutdown)
	if _, err := m.Exec("worker", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case first := <-runner.calls:
		if first.ref != "image-a:latest" || first.digest != aManifest.Digest {
			t.Fatalf("active iteration image = %#v", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("image A iteration did not start")
	}
	if err := as.SetPendingImage("worker", "image-b:latest", bManifest.Digest); err != nil {
		t.Fatal(err)
	}
	if current, err := as.Get("worker"); err != nil || current.ImageRef != "image-a:latest" {
		t.Fatalf("active image changed during iteration: %#v, %v", current, err)
	}
	close(runner.release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		iterations, err := as.ListIterations("worker")
		if err == nil && len(iterations) == 1 && iterations[0].Status == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("image A iteration did not finish: %#v, %v", iterations, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.Exec("worker", "next"); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-runner.calls:
		if second.ref != "image-b:latest" || second.digest != bManifest.Digest {
			t.Fatalf("next iteration image = %#v", second)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("image B iteration did not start")
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		iterations, err := as.ListIterations("worker")
		if err == nil && len(iterations) == 2 && iterations[1].Status == "done" {
			if iterations[0].ImageRef != "image-a:latest" || iterations[0].ImageDigest != aManifest.Digest {
				t.Fatalf("first iteration snapshot = %#v", iterations[0])
			}
			if iterations[1].ImageRef != "image-b:latest" || iterations[1].ImageDigest != bManifest.Digest || iterations[1].PromptTemplateSHA256 != bManifest.PromptTemplateSHA256 {
				t.Fatalf("second iteration snapshot = %#v", iterations[1])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("image B iteration did not finish: %#v, %v", iterations, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The same lifecycle continues through runnable-only portability: remove
	// B's original source, export it, import into a distinct daemon base, and
	// launch an iteration there using only the portable artifact.
	if err := os.RemoveAll(sources["image-b"]); err != nil {
		t.Fatal(err)
	}
	originBase := filepath.Dir(m.cfg.ImgStore.Dir)
	portable := imageportable.Service{BaseDir: originBase, StagingRoot: filepath.Join(originBase, "image-imports")}
	var archive bytes.Buffer
	if err := portable.Export(context.Background(), "image-b:latest", &archive); err != nil {
		t.Fatal(err)
	}
	targetRunner := &imageBoundaryRunner{calls: make(chan imageBoundaryCall, 1), release: make(chan struct{})}
	targetManager, targetAgents, _, _ := newManager(t, targetRunner)
	t.Cleanup(targetManager.Shutdown)
	targetBase := filepath.Dir(targetManager.cfg.ImgStore.Dir)
	importer := imageportable.Service{BaseDir: targetBase, StagingRoot: filepath.Join(targetBase, "image-imports")}
	preview, err := importer.Preview(context.Background(), bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importer.Apply(context.Background(), preview.ImportID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targetManager.Run(registry.RunSpec{ImageRef: imported.Ref, Name: "imported-worker", Harness: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := targetManager.Exec("imported-worker", "portable run"); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-targetRunner.calls:
		if call.ref != imported.Ref || call.digest != imported.Digest {
			t.Fatalf("imported iteration image = %#v, want %s@%s", call, imported.Ref, imported.Digest)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("imported runnable image did not launch")
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		iterations, listErr := targetAgents.ListIterations("imported-worker")
		if listErr == nil && len(iterations) == 1 && iterations[0].Status == "done" {
			if iterations[0].ImageRef != imported.Ref || iterations[0].ImageDigest != imported.Digest || iterations[0].PromptTemplateSHA256 != bManifest.PromptTemplateSHA256 {
				t.Fatalf("imported iteration snapshot = %#v", iterations[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("imported iteration did not finish: %#v, %v", iterations, listErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPendingImageActivationSwapsOnlyImageAndPromotes(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	build := func(name, body string, pluginNames ...string) (image.Ref, image.Manifest) {
		source := t.TempDir()
		if err := os.WriteFile(filepath.Join(source, "p.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		ref := image.Ref{Name: name, Tag: "latest"}
		declared := make([]imagefile.V2Plugin, 0, len(pluginNames))
		for _, pluginName := range pluginNames {
			declared = append(declared, imagefile.V2Plugin{Name: pluginName})
		}
		man, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: source, Plugins: declared, Prompts: []imagefile.PromptEntry{{File: "./p.md"}}}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref, man
	}
	aRef, aMan := build("a", "A")
	bRef, bMan := build("b", "B", "tasks")
	ag := agent.Agent{Name: "worker", ImageRef: aRef.String(), ImageDigest: aMan.Digest, HarnessType: "codex", Model: "runtime-model", OnTimeout: "restart", OnError: "restart"}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, aRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.ContextPath(), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage(ag.Name, bRef.String(), bMan.Digest); err != nil {
		t.Fatal(err)
	}
	recorder := &captureRecorder{}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), AuditFor: func(string) Recorder { return recorder }}}
	sha, err := m.activatePendingImage(&ag)
	if err != nil {
		t.Fatal(err)
	}
	if sha.PromptTemplateSHA256 != bMan.PromptTemplateSHA256 || ag.ImageRef != bRef.String() || ag.Model != "runtime-model" {
		t.Fatalf("agent=%#v sha=%s", ag, sha)
	}
	if body, err := os.ReadFile(filepath.Join(l.ImageDir(), "prompt", "layers", "000-p.md")); err != nil || string(body) != "B" {
		t.Fatalf("layer=%q err=%v", body, err)
	}
	if body, err := os.ReadFile(l.ContextPath()); err != nil || string(body) != "keep" {
		t.Fatalf("context=%q err=%v", body, err)
	}
	pending, _ := as.PendingImage(ag.Name)
	if pending.Ref != "" {
		t.Fatalf("pending=%#v", pending)
	}
	if len(ag.Plugins) != 1 || ag.Plugins[0] != "tasks" {
		t.Fatalf("activated plugins=%v", ag.Plugins)
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "tasks")); err != nil {
		t.Fatalf("tasks shim was not reconciled: %v", err)
	}
	events := recorder.snapshot()
	if len(events) != 1 || events[0].typ != "agent_image_activated" || events[0].source != "system" || events[0].iterID != "" || events[0].data["old_ref"] != aRef.String() || events[0].data["new_ref"] != bRef.String() {
		t.Fatalf("activation audit = %#v", events)
	}
}

func TestRecoverImageSwapRestoresDatabaseActiveImage(t *testing.T) {
	root := t.TempDir()
	l := agentdir.New(root, "worker")
	if err := os.MkdirAll(l.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeManifest := func(dir, name string) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schema_version":2,"name":"`+name+`","tag":"latest"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest(l.ImageDir(), "candidate")
	writeManifest(filepath.Join(l.Root, ".image-backup"), "active")
	stage := filepath.Join(l.Root, ".image-stage-abandoned")
	writeManifest(stage, "staged")
	if err := os.WriteFile(filepath.Join(l.ImageDir(), ".image-digest"), []byte("candidate-digest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root, ".image-backup", ".image-digest"), []byte("active-digest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if recovered, err := recoverImageSwap(l, "active:latest", "active-digest"); err != nil {
		t.Fatal(err)
	} else if !recovered {
		t.Fatal("swap recovery was not reported")
	}
	data, err := os.ReadFile(filepath.Join(l.ImageDir(), "manifest.json"))
	if err != nil || !strings.Contains(string(data), `"name":"active"`) {
		t.Fatalf("restored manifest=%q err=%v", data, err)
	}
	for _, path := range []string{filepath.Join(l.Root, ".image-backup"), stage} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("recovery marker survived: %s err=%v", path, err)
		}
	}
}

func TestRecoverImageSwapUsesDigestWhenManagedRefIsUnchanged(t *testing.T) {
	root := t.TempDir()
	l := agentdir.New(root, "worker")
	writeImage := func(dir, digest string) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schema_version":2,"name":"basic","tag":"latest"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".image-digest"), []byte(digest+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeImage(l.ImageDir(), "candidate-v2")
	writeImage(filepath.Join(l.Root, ".image-backup"), "active-v1")

	recovered, err := recoverImageSwap(l, "basic:latest", "active-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("same-ref swap recovery was not reported")
	}
	digest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
	if err != nil || strings.TrimSpace(string(digest)) != "active-v1" {
		t.Fatalf("restored digest=%q err=%v", digest, err)
	}
}

func TestImageActivationCrashRecoveryReconcilesActiveShimsAfterCancel(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	build := func(name string, plugins ...string) (image.Ref, image.Manifest) {
		declared := make([]imagefile.V2Plugin, 0, len(plugins))
		for _, plugin := range plugins {
			declared = append(declared, imagefile.V2Plugin{Name: plugin})
		}
		ref := image.Ref{Name: name, Tag: "latest"}
		manifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Plugins: declared}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref, manifest
	}
	aRef, aManifest := build("active", "loop")
	bRef, _ := build("candidate")
	ag := agent.Agent{Name: "worker", ImageRef: aRef.String(), ImageDigest: aManifest.Digest, Plugins: []string{"loop"}}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, aRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(l.Root, ".image-backup")
	if err := os.Rename(l.ImageDir(), backup); err != nil {
		t.Fatal(err)
	}
	if err := images.Unpack(bRef, l.ImageDir()); err != nil {
		t.Fatal(err)
	}
	candidate := ag
	candidate.Plugins = nil
	if err := agentdir.WriteShims(l, candidate, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "i-am-done")); !os.IsNotExist(err) {
		t.Fatalf("candidate loop shim survived simulated crash setup: %v", err)
	}

	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	if _, err := m.activatePendingImage(&ag); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(l.BinDir(), "i-am-done")); err != nil {
		t.Fatalf("active loop shim was not restored after crash recovery: %v", err)
	}
}

func TestPendingManagedImageSurvivesDaemonUpgradeBeforeActivation(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	managedArchive := func(version string) []byte {
		source, err := filepath.Abs(filepath.Join("..", "builtinimages", "source"))
		if err != nil {
			t.Fatal(err)
		}
		out := t.TempDir()
		if err := builtinimages.Generate(source, out, version); err != nil {
			t.Fatal(err)
		}
		archive, err := os.ReadFile(filepath.Join(out, "basic.tar.gz"))
		if err != nil {
			t.Fatal(err)
		}
		return archive
	}
	basicRef := image.Ref{Name: "basic", Tag: "latest"}
	if err := images.InstallManagedArchive(basicRef, managedArchive("1.0.0")); err != nil {
		t.Fatal(err)
	}
	pendingManifest, err := images.Inspect(basicRef)
	if err != nil {
		t.Fatal(err)
	}
	activeRef := image.Ref{Name: "active", Tag: "latest"}
	activeManifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, activeRef, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: activeRef.String(), ImageDigest: activeManifest.Digest, HarnessType: "codex"}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, activeRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage(ag.Name, basicRef.String(), pendingManifest.Digest); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 10*time.Millisecond)
	if err := images.InstallManagedArchive(basicRef, managedArchive("1.1.0")); err != nil {
		t.Fatal(err)
	}
	current, err := images.Inspect(basicRef)
	if err != nil {
		t.Fatal(err)
	}
	if current.Digest == pendingManifest.Digest {
		t.Fatal("test setup did not replace managed basic image")
	}
	m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
	sha, err := m.activatePendingImage(&ag)
	if err != nil {
		t.Fatalf("pre-upgrade pending image did not activate: %v", err)
	}
	if ag.ImageRef != basicRef.String() || ag.ImageDigest != pendingManifest.Digest || sha.PromptTemplateSHA256 != pendingManifest.PromptTemplateSHA256 {
		t.Fatalf("activated agent=%+v sha=%s; want pinned digest %s", ag, sha, pendingManifest.Digest)
	}
}

func TestPendingImageActivationFailuresPreserveActiveImageAndRecordError(t *testing.T) {
	tests := []struct {
		name          string
		candidate     []imagefile.V2Plugin
		corruptDigest bool
		blockStaging  bool
	}{
		{name: "missing plugin", candidate: []imagefile.V2Plugin{{Name: "external-widget"}}},
		{name: "corrupted digest", corruptDigest: true},
		{name: "staging failure", blockStaging: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			db, err := storedb.Open(filepath.Join(base, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			as := agent.NewStore(db)
			images := &image.Store{Dir: filepath.Join(base, "images")}
			activeRef := image.Ref{Name: "active", Tag: "latest"}
			activeManifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2}, imagefile.ResolveRoots{}, activeRef, images, time.Now, nil)
			if err != nil {
				t.Fatal(err)
			}
			candidateRef := image.Ref{Name: "candidate", Tag: "latest"}
			candidateManifest, err := image.BuildV2(
				&imagefile.V2{SchemaVersion: 2, Plugins: tc.candidate}, imagefile.ResolveRoots{}, candidateRef, images, time.Now,
				func(string) (plugincaps.ResolvedPlugin, error) {
					return plugincaps.ResolvedPlugin{Installed: true}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			ag := agent.Agent{Name: "worker", ImageRef: activeRef.String(), ImageDigest: activeManifest.Digest}
			if err := as.Create(ag); err != nil {
				t.Fatal(err)
			}
			l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
			if err := agentdir.Provision(l, ag, images, activeRef, "/bin/true"); err != nil {
				t.Fatal(err)
			}
			pendingDigest := candidateManifest.Digest
			if tc.corruptDigest {
				pendingDigest = strings.Repeat("0", len(pendingDigest))
			}
			if err := as.SetPendingImage(ag.Name, candidateRef.String(), pendingDigest); err != nil {
				t.Fatal(err)
			}
			if tc.blockStaging {
				if err := os.Chmod(l.Root, 0o500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(l.Root, 0o700)
			}
			m := &Manager{cfg: ManagerConfig{AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true"}}
			if _, err := m.activatePendingImage(&ag); err == nil {
				t.Fatal("invalid pending image activated")
			}
			stored, err := as.Get(ag.Name)
			if err != nil {
				t.Fatal(err)
			}
			if stored.ImageRef != activeRef.String() || stored.ImageDigest != activeManifest.Digest {
				t.Fatalf("active image changed after activation failure: %+v", stored)
			}
			pending, err := as.PendingImage(ag.Name)
			if err != nil || pending.Ref != candidateRef.String() || pending.Error == "" {
				t.Fatalf("pending failure was not retained: %+v err=%v", pending, err)
			}
			localDigest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
			if err != nil || strings.TrimSpace(string(localDigest)) != activeManifest.Digest {
				t.Fatalf("active local image changed: digest=%q err=%v", localDigest, err)
			}
		})
	}
}

func TestPendingImageActivationBridgeFailurePreservesActiveImage(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	activeRef := image.Ref{Name: "active", Tag: "latest"}
	activeManifest, err := image.BuildV2(&imagefile.V2{SchemaVersion: 2, Dir: t.TempDir()}, imagefile.ResolveRoots{}, activeRef, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	skillDir := filepath.Join(source, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes.\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pendingRef := image.Ref{Name: "pending", Tag: "latest"}
	pendingManifest, err := image.BuildV2(&imagefile.V2{
		SchemaVersion: 2, Dir: source,
		Skills: []imagefile.SkillEntry{{Dir: "./skills/review"}},
	}, imagefile.ResolveRoots{}, pendingRef, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: activeRef.String(), ImageDigest: activeManifest.Digest, HarnessType: "claude"}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(filepath.Join(base, "agents"), ag.Name)
	if err := agentdir.Provision(l, ag, images, activeRef, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage(ag.Name, pendingRef.String(), pendingManifest.Digest); err != nil {
		t.Fatal(err)
	}
	bridgeErr := errors.New("bridge publication failed")
	bridgeCalled := false
	m := &Manager{cfg: ManagerConfig{
		AgentsDir: filepath.Join(base, "agents"), Store: as, ImgStore: images, ToolsBin: "/bin/true",
		PrepareImageBridge: func(sourceDir, finalDir string, skills []image.ManifestSkill, plan agentdir.BridgePlan) error {
			bridgeCalled = true
			if sourceDir == filepath.Join(l.ImageDir(), "skills") {
				t.Fatal("pending bridge was prepared from the active image directory")
			}
			if len(skills) != 1 || plan.SkillDestination == "" || !strings.Contains(finalDir, pendingManifest.Digest) {
				t.Fatalf("bridge request source=%q final=%q skills=%#v plan=%#v", sourceDir, finalDir, skills, plan)
			}
			return bridgeErr
		},
	}}
	if _, err := m.activatePendingImage(&ag); !errors.Is(err, bridgeErr) {
		t.Fatalf("activation error = %v, want %v", err, bridgeErr)
	}
	if !bridgeCalled {
		t.Fatal("bridge preparer was not called")
	}
	stored, err := as.Get(ag.Name)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ImageRef != activeRef.String() || stored.ImageDigest != activeManifest.Digest {
		t.Fatalf("active identity changed: %+v", stored)
	}
	localDigest, err := os.ReadFile(filepath.Join(l.ImageDir(), ".image-digest"))
	if err != nil || strings.TrimSpace(string(localDigest)) != activeManifest.Digest {
		t.Fatalf("active image bytes changed: %q, %v", localDigest, err)
	}
	pending, err := as.PendingImage(ag.Name)
	if err != nil || !strings.Contains(pending.Error, bridgeErr.Error()) {
		t.Fatalf("pending error = %+v, %v", pending, err)
	}
}

func TestImageBridgeRestartReusesPublishedBridge(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	source := t.TempDir()
	skillDir := filepath.Join(source, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes.\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	manifest, err := image.BuildV2(&imagefile.V2{
		SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/review"}},
	}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	harnessBin := t.TempDir()
	claudePath := filepath.Join(harnessBin, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/bin/sh\necho '2.1.227 (Claude Code)'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: manifest.Digest, HarnessType: "claude", Env: map[string]string{"PATH": harnessBin}}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(base, "agents")
	l := agentdir.New(agentsDir, ag.Name)
	if err := agentdir.Provision(l, ag, images, ref, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	config := ManagerConfig{AgentsDir: agentsDir, Store: as, ImgStore: images, ToolsBin: "/bin/true"}
	first, err := (&Manager{cfg: config}).activatePendingImage(&ag)
	if err != nil {
		t.Fatal(err)
	}
	bridgeDir, err := l.ImageBridgeDir(manifest.Digest, harness.SkillAdapterContractVersion, "claude")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(bridgeDir, "bridge-manifest.json")
	before, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (&Manager{cfg: config}).activatePendingImage(&ag)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"--plugin-dir", bridgeDir}
	if !slices.Equal(first.Skills.Args, wantArgs) || !slices.Equal(second.Skills.Args, wantArgs) {
		t.Fatalf("launch configs = %#v / %#v, want args %#v", first.Skills, second.Skills, wantArgs)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("valid bridge was republished: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestStubImageSkillsNeedNoBridge(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/bin/true")
	called := false
	m := &Manager{cfg: ManagerConfig{PrepareImageBridge: func(string, string, []image.ManifestSkill, agentdir.BridgePlan) error {
		called = true
		return nil
	}}}
	got, err := m.prepareImageSkillBridge(agent.Agent{Name: "worker", HarnessType: "stub"}, image.Manifest{
		SchemaVersion: 2,
		Skills:        []image.ManifestSkill{{Name: "review"}},
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if called || len(got.Args) != 0 || len(got.Env) != 0 || got.PromptPrefix != "" {
		t.Fatalf("stub bridge called=%v launch=%#v", called, got)
	}
}

func TestActiveCodexImageSkillBridgeNeedsNoPluginProbe(t *testing.T) {
	base := t.TempDir()
	db, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := agent.NewStore(db)
	images := &image.Store{Dir: filepath.Join(base, "images")}
	source := t.TempDir()
	skillDir := filepath.Join(source, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: review\ndescription: Review changes.\n---\nPROOF BODY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	manifest, err := image.BuildV2(&imagefile.V2{
		SchemaVersion: 2, Dir: source, Skills: []imagefile.SkillEntry{{Dir: "./skills/review"}},
	}, imagefile.ResolveRoots{}, ref, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Agent{
		Name: "worker", ImageRef: ref.String(), ImageDigest: manifest.Digest, HarnessType: "codex",
		Env: map[string]string{"PATH": t.TempDir()},
	}
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(base, "agents")
	l := agentdir.New(agentsDir, ag.Name)
	if err := agentdir.Provision(l, ag, images, ref, "/bin/true"); err != nil {
		t.Fatal(err)
	}
	activated, err := (&Manager{cfg: ManagerConfig{
		AgentsDir: agentsDir, Store: as, ImgStore: images, ToolsBin: "/bin/true",
	}}).activatePendingImage(&ag)
	if err != nil {
		t.Fatalf("Codex bridge required an executable/plugin probe: %v", err)
	}
	if activated.Skills.PromptPrefix == "" || strings.Contains(activated.Skills.PromptPrefix, "PROOF BODY") {
		t.Fatalf("Codex prompt catalog = %q", activated.Skills.PromptPrefix)
	}
	bridgeDir, err := l.ImageBridgeDir(manifest.Digest, harness.SkillAdapterContractVersion, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bridgeDir, string(filepath.Separator)+"2"+string(filepath.Separator)+"codex") {
		t.Fatalf("Codex bridge did not use contract 2: %s", bridgeDir)
	}
	if body, err := os.ReadFile(filepath.Join(bridgeDir, "skills", "review", "SKILL.md")); err != nil || !strings.Contains(string(body), "PROOF BODY") {
		t.Fatalf("copied Codex skill = %q, %v", body, err)
	}
	for _, unwanted := range []string{filepath.Join(bridgeDir, "marketplace"), filepath.Join(bridgeDir, ".codex-plugin")} {
		if _, err := os.Stat(unwanted); !os.IsNotExist(err) {
			t.Fatalf("Codex bridge contains %s: %v", unwanted, err)
		}
	}
}
