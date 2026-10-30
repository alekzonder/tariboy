package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/onboarding"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/stores"
)

// startOnboardingWorkers runs the startup phases up to the workers with the
// default Store pointed at a local directory that holds a basic image.
func startOnboardingWorkers(t *testing.T, mode string) (*managerParts, *process) {
	t.Helper()
	o := phaseOptions(t)
	o.Onboarding = mode
	source := t.TempDir()
	dir := filepath.Join(source, "images", "basic")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte("schema_version: 2\nimage_version: 1.0.0\nplugins: []\nprompts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	o.onboardingSource = source
	proc, c, _ := phaseCore(t, o)
	tk, err := buildTasks(context.Background(), proc, c)
	if err != nil {
		t.Fatal(err)
	}
	px, err := buildProxy(context.Background(), o, proc, c)
	if err != nil {
		t.Fatal(err)
	}
	mg := buildManager(context.Background(), o, proc, c, tk, px, buildPlugins(proc, c))
	var stack shutdownStack
	startWorkers(context.Background(), o, proc, c, tk, px, mg, &stack)
	t.Cleanup(func() { stack.run(proc.log, nil) })
	return mg, proc
}

func TestStartWorkersRegistersDefaultStoreAndBuildsBasic(t *testing.T) {
	t.Parallel()
	mg, proc := startOnboardingWorkers(t, "on")
	deadline := time.Now().Add(60 * time.Second)
	st := mg.cctx.Onboarding.Status(context.Background(), false)
	for st.StarterImage.State != onboarding.StateReady {
		if time.Now().After(deadline) || st.StarterImage.State == onboarding.StateFailed {
			t.Fatalf("basic:latest never built: %#v", st)
		}
		time.Sleep(20 * time.Millisecond)
		st = mg.cctx.Onboarding.Status(context.Background(), false)
	}
	images := &image.Store{Dir: paths.New(proc.paths.Base).ImagesDir()}
	if st.DefaultStore.State != onboarding.StateRegistered || !images.Exists(image.Ref{Name: "basic", Tag: "latest"}) {
		t.Fatalf("status = %#v", st)
	}
}

func TestStartWorkersSkipsOnboardingWhenOff(t *testing.T) {
	t.Parallel()
	mg, proc := startOnboardingWorkers(t, "off")
	list, err := stores.New(mg.cctx.Store, proc.paths.Base).List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("stores = %#v, %v", list, err)
	}
	if st := mg.cctx.Onboarding.Status(context.Background(), false); st.DefaultStore.State != onboarding.StateFailed || !strings.Contains(st.DefaultStore.Error, "off") {
		t.Fatalf("status = %#v", st)
	}
}

func TestOnboardingModeFollowsOptionThenEnvironment(t *testing.T) {
	// IsolateEnv sets TARIBOY_ONBOARDING=off for every daemon test.
	if !onboardingOff(Options{}) || onboardingOff(Options{Onboarding: "on"}) || !onboardingOff(Options{Onboarding: "off"}) {
		t.Fatal("onboarding mode ignores the option or the environment")
	}
}
