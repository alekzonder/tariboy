package onboarding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	storedb "github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/stores"
)

type fixture struct {
	svc    *Service
	db     *storedb.Store
	source string
	builds atomic.Int32
}

// newFixture returns a service whose default Store is a local directory, so
// no test reaches the network.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := t.TempDir()
	path := filepath.Join(base, "tariboyd.db")
	storetest.Seed(t, path)
	db, err := storedb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "images", "basic"), 0o700); err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: db, source: source}
	images := &image.Store{Dir: filepath.Join(base, "images")}
	f.svc = &Service{
		DB: db, Catalog: stores.New(db, base), Images: images, Source: source,
		Build: func(_ context.Context, selector string) error {
			f.builds.Add(1)
			if selector != DefaultStoreName+"/"+StarterImage && !strings.HasSuffix(selector, "/"+StarterImage) {
				t.Errorf("selector = %q", selector)
			}
			return plantImage(t, images, image.Ref{Name: StarterImage, Tag: "latest"})
		},
	}
	return f
}

func plantImage(t *testing.T, st *image.Store, ref image.Ref) error {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("hi"), 0o600); err != nil {
		return err
	}
	src := &imagefile.V2{SchemaVersion: 2, Dir: dir, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
	_, err := image.BuildV2(src, imagefile.ResolveRoots{}, ref, st, time.Now, nil)
	return err
}

func (f *fixture) key(t *testing.T) string {
	t.Helper()
	v, _, err := f.db.ConfigGet(ConfigKey)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRunRegistersDefaultStoreAndBuildsBasicOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.svc.Run(ctx)
	f.svc.Run(ctx)
	list, err := f.svc.Catalog.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != DefaultStoreName {
		t.Fatalf("stores = %#v, %v", list, err)
	}
	if got := f.key(t); got != "registered" {
		t.Fatalf("key = %q, want registered", got)
	}
	if n := f.builds.Load(); n != 1 {
		t.Fatalf("builds = %d, want 1", n)
	}
	st := f.svc.Status(ctx, false)
	if st.DefaultStore.State != "registered" || st.StarterImage.State != "ready" || st.StarterImage.Ref != "basic:latest" {
		t.Fatalf("status = %#v", st)
	}
}

func TestRunKeepsExistingStoreAndBasicImage(t *testing.T) {
	for _, name := range []string{DefaultStoreName, "mine"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			// "mine" carries the default source under another name.
			if _, err := f.svc.Catalog.Add(ctx, name, f.source); err != nil {
				t.Fatal(err)
			}
			if err := plantImage(t, f.svc.Images, image.Ref{Name: StarterImage, Tag: "1.0.0"}); err != nil {
				t.Fatal(err)
			}
			f.svc.Run(ctx)
			list, _ := f.svc.Catalog.List(ctx)
			if len(list) != 1 || list[0].Name != name {
				t.Fatalf("stores = %#v", list)
			}
			if n := f.builds.Load(); n != 0 {
				t.Fatalf("builds = %d, want 0", n)
			}
			st := f.svc.Status(ctx, false)
			if st.DefaultStore.State != "registered" || st.DefaultStore.Name != name || st.StarterImage.State != "skipped" || st.StarterImage.Ref != "basic:1.0.0" {
				t.Fatalf("status = %#v", st)
			}
		})
	}
}

func TestRunNeverReaddsARemovedStore(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.svc.Run(ctx)
	if err := f.svc.Catalog.Remove(ctx, DefaultStoreName); err != nil {
		t.Fatal(err)
	}
	if err := MarkRemoved(f.db, stores.Store{Name: DefaultStoreName}, f.source); err != nil {
		t.Fatal(err)
	}
	f.svc.Run(ctx)
	if list, _ := f.svc.Catalog.List(ctx); len(list) != 0 {
		t.Fatalf("stores = %#v, want none", list)
	}
	if st := f.svc.Status(ctx, false); st.DefaultStore.State != "removed" {
		t.Fatalf("status = %#v", st)
	}
}

func TestMarkRemovedIgnoresOtherStores(t *testing.T) {
	f := newFixture(t)
	if err := MarkRemoved(f.db, stores.Store{Name: "other", Source: "/x"}, f.source); err != nil {
		t.Fatal(err)
	}
	if got := f.key(t); got != "" {
		t.Fatalf("key = %q, want absent", got)
	}
	if err := MarkRemoved(f.db, stores.Store{Name: "fork", Source: f.source}, f.source); err != nil {
		t.Fatal(err)
	}
	if got := f.key(t); got != "removed" {
		t.Fatalf("key = %q, want removed", got)
	}
}

func TestRunRecordsFailureAndRetriesOnNextRun(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	good := f.svc.Source
	f.svc.Source = filepath.Join(good, "missing")
	f.svc.Run(ctx)
	st := f.svc.Status(ctx, false)
	if st.DefaultStore.State != "failed" || st.DefaultStore.Error == "" || f.key(t) != "failed" {
		t.Fatalf("status = %#v, key %q", st, f.key(t))
	}
	if st.StarterImage.State != "pending" || st.StarterImage.Error == "" {
		t.Fatalf("starter = %#v, want pending with a reason", st.StarterImage)
	}
	f.svc.Source = good
	f.svc.Run(ctx)
	if st := f.svc.Status(ctx, false); st.DefaultStore.State != "registered" || st.StarterImage.State != "ready" {
		t.Fatalf("status after retry = %#v", st)
	}
}

func TestBuildFailureNamesMissingTool(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PATH", t.TempDir())
	f.svc.Build = func(context.Context, string) error { return errors.New("npx: not found") }
	f.svc.Run(context.Background())
	st := f.svc.Status(context.Background(), false)
	if st.StarterImage.State != "failed" || !strings.HasPrefix(st.StarterImage.Error, "npx is missing: install Node.js") {
		t.Fatalf("starter = %#v", st.StarterImage)
	}
}

func TestStatusReportsOffDaemonAsFailed(t *testing.T) {
	f := newFixture(t)
	f.svc.Off = true
	st := f.svc.Status(context.Background(), false)
	if st.DefaultStore.State != "failed" || !strings.Contains(st.DefaultStore.Error, "TARIBOY_ONBOARDING=off") {
		t.Fatalf("status = %#v", st)
	}
	if st.DefaultStore.Name != DefaultStoreName || st.DefaultStore.Source != f.source {
		t.Fatalf("store identity = %#v", st.DefaultStore)
	}
}

func TestRetryRunsOneRoutineAtATime(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	f.svc.Build = func(context.Context, string) error {
		f.builds.Add(1)
		started <- struct{}{}
		<-release
		return plantImage(t, f.svc.Images, image.Ref{Name: StarterImage, Tag: "latest"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.svc.Off = true
	done := make(chan struct{})
	go func() { f.svc.Start(ctx); close(done) }()
	waitFor(t, func() bool { return f.svc.started() })
	if st := f.svc.Retry(); st.DefaultStore.State == "failed" {
		t.Fatalf("retry status = %#v, want a running state", st)
	}
	<-started
	if st := f.svc.Retry(); st.StarterImage.State != "building" {
		t.Fatalf("second retry status = %#v, want building", st)
	}
	close(release)
	waitFor(t, func() bool { return f.svc.Status(ctx, false).StarterImage.State == "ready" })
	cancel()
	<-done
	if n := f.builds.Load(); n != 1 {
		t.Fatalf("builds = %d, want 1", n)
	}
}

func TestHarnessesAreCachedUntilRefresh(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PATH", t.TempDir())
	first := f.svc.Status(context.Background(), false).Harnesses
	if len(first) != 4 || first[0].Found {
		t.Fatalf("harnesses = %#v", first)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := f.svc.Status(context.Background(), false).Harnesses; got[0].Found {
		t.Fatal("cached harness data changed without refresh")
	}
	if got := f.svc.Status(context.Background(), true).Harnesses; !got[0].Found {
		t.Fatal("refresh did not detect claude")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
