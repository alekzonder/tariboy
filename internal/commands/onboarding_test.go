package commands

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/onboarding"
	"github.com/alekzonder/tariboy/internal/registry"
)

func init() { typedCallSets["onboarding"] = onboardingTypedCalls }

// onboardingCtx is a command context whose onboarding service registers a
// local directory as the default Store and builds nothing.
func onboardingCtx(t *testing.T) (*registry.Ctx, string) {
	t.Helper()
	c := localCtx(t)
	source := t.TempDir()
	writeImageV2(t, filepath.Join(source, "images", "basic"), "1.0.0")
	c.Onboarding = &onboarding.Service{
		DB: c.Store, Catalog: storeCatalog(c), Images: imageStore(c), Source: source, Off: true,
		Build: func(context.Context, string) error { return nil },
	}
	return c, source
}

func onboardingTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, _ := onboardingCtx(t)
	return c, []typedCall{
		{"GET", "/api/onboarding", "/api/onboarding", nil},
		{"GET", "/api/onboarding", "/api/onboarding?refresh=true", nil},
		{"POST", "/api/onboarding/retry", "/api/onboarding/retry", nil},
	}
}

func TestOnboardingCommandsRegisteredWithRoutes(t *testing.T) {
	t.Parallel()
	reg := BuildRegistry()
	for path, route := range map[string]registry.HTTPRoute{
		"onboarding.status": {Method: http.MethodGet, Path: "/api/onboarding"},
		"onboarding.retry":  {Method: http.MethodPost, Path: "/api/onboarding/retry"},
	} {
		command, ok := reg.Get(path)
		if !ok || command.HTTP == nil || *command.HTTP != route {
			t.Fatalf("%s = %#v, want route %#v", path, command.HTTP, route)
		}
	}
}

func TestOnboardingStatusReportsEveryPart(t *testing.T) {
	t.Parallel()
	c, source := onboardingCtx(t)
	if err := agent.NewStore(c.Store).Create(agent.Agent{Name: "a1", ImageRef: "bare:latest"}); err != nil {
		t.Fatal(err)
	}
	out, err := cmdHandler(t, "onboarding.status")(c, registry.Params{"refresh": true})
	if err != nil {
		t.Fatal(err)
	}
	st := out.(OnboardingStatus)
	if st.DefaultStore.State != "failed" || !strings.Contains(st.DefaultStore.Error, "TARIBOY_ONBOARDING=off") || st.DefaultStore.Source != source {
		t.Fatalf("default_store = %#v", st.DefaultStore)
	}
	if st.StarterImage.State != "pending" || st.Agents != 1 || len(st.Harnesses) != 4 {
		t.Fatalf("status = %#v", st)
	}
	for _, h := range st.Harnesses {
		if h.Type == "stub" {
			t.Fatal("stub is reported as a harness")
		}
	}
	object := jsonObject(t, out)
	for _, key := range []string{"default_store", "starter_image", "harnesses", "agents"} {
		if _, ok := object[key]; !ok {
			t.Fatalf("status JSON lacks %s: %v", key, object)
		}
	}
}

func TestOnboardingRetryRunsTheRoutine(t *testing.T) {
	t.Parallel()
	c, _ := onboardingCtx(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Onboarding.Start(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(10 * time.Second)
	for {
		out, err := cmdHandler(t, "onboarding.retry")(c, registry.Params{})
		if err != nil {
			t.Fatal(err)
		}
		if out.(OnboardingStatus).DefaultStore.State == "registered" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("retry never registered the Store: %#v", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOnboardingCommandsNeedTheDaemon(t *testing.T) {
	t.Parallel()
	c := localCtx(t)
	for _, path := range []string{"onboarding.status", "onboarding.retry"} {
		if _, err := cmdHandler(t, path)(c, registry.Params{}); !isCode(err, "no_onboarding") {
			t.Fatalf("%s error = %v, want no_onboarding", path, err)
		}
	}
}

func TestStoreRemoveMarksOnlyTheDefaultStoreRemoved(t *testing.T) {
	t.Parallel()
	c, source := onboardingCtx(t)
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := storeCatalog(c)
	for name, src := range map[string]string{"team": other, onboarding.DefaultStoreName: source} {
		if _, err := catalog.Add(context.Background(), name, src); err != nil {
			t.Fatal(err)
		}
	}
	key := func() string {
		v, _, err := c.Store.ConfigGet(onboarding.ConfigKey)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if _, err := cmdHandler(t, "store.remove")(c, registry.Params{"name": "team"}); err != nil {
		t.Fatal(err)
	}
	if got := key(); got != "" {
		t.Fatalf("key after removing team = %q", got)
	}
	if _, err := cmdHandler(t, "store.remove")(c, registry.Params{"name": onboarding.DefaultStoreName}); err != nil {
		t.Fatal(err)
	}
	if got := key(); got != onboarding.StateRemoved {
		t.Fatalf("key after removing tariboy = %q", got)
	}
}
