package commands

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

var updateStoresGolden = flag.Bool("update-stores-golden", false, "rewrite testdata/stores_responses.golden.json")

// storesFixture returns the images fixture with
// reviewer:1.2.0 built, so the Store image reviewer 3.4.5 reports an update.
func storesFixture(t *testing.T) (*registry.Ctx, imagesFixtureDirs) {
	t.Helper()
	c, dirs := imagesFixture(t)
	if _, err := h(t, "image.build")(c, registry.Params{"name": "reviewer", "path": dirs.versioned}); err != nil {
		t.Fatal(err)
	}
	return c, dirs
}

// Catches a typed stores response changing the JSON a client sees: the set of
// keys, their JSON types, and their values must match the recorded responses.
func TestStoresResponsesMatchGolden(t *testing.T) {
	c, dirs := storesFixture(t)
	calls := []struct {
		label, path string
		params      registry.Params
	}{
		{"store.list empty", "store.list", registry.Params{}},
		{"store.add", "store.add", registry.Params{"name": "team", "source": dirs.store}},
		{"store.list", "store.list", registry.Params{}},
		{"store.show", "store.show", registry.Params{"name": "team"}},
		{"store.refresh", "store.refresh", registry.Params{"name": "team"}},
		{"store.auto", "store.auto", registry.Params{"name": "team", "interval": 45, "image": []string{"reviewer"}}},
		{"store.auto off", "store.auto", registry.Params{"name": "team", "interval": 0}},
		{"store.remove", "store.remove", registry.Params{"name": "team"}},
	}
	resolvedRoot, err := filepath.EvalSymlinks(dirs.root)
	if err != nil {
		t.Fatal(err)
	}
	recorder := newGoldenRecorder(t, nil, resolvedRoot, "$ROOT", dirs.root, "$ROOT")
	for _, call := range calls {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		recorder.record(call.label, value)
	}
	recorder.check("stores_responses.golden.json", *updateStoresGolden)
}

func init() { typedCallSets["stores"] = storesTypedCalls }

// storesTypedCalls drives every typed stores operation over HTTP, including
// an empty list, a Store image and workflow with an error, and an automatic
// build policy that is set and then disabled.
func storesTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, dirs := storesFixture(t)
	return c, []typedCall{
		{"GET", "/api/stores", "/api/stores", nil},
		{"POST", "/api/stores", "/api/stores", map[string]any{"name": "team", "source": dirs.store}},
		{"GET", "/api/stores", "/api/stores", nil},
		{"GET", "/api/stores/{name}", "/api/stores/team", nil},
		{"POST", "/api/stores/{name}/refresh", "/api/stores/team/refresh", nil},
		{"POST", "/api/stores/{name}/auto", "/api/stores/team/auto", map[string]any{"interval": 45, "image": []string{"reviewer"}}},
		{"POST", "/api/stores/{name}/auto", "/api/stores/team/auto", map[string]any{"interval": 0}},
		{"DELETE", "/api/stores/{name}", "/api/stores/team", nil},
	}
}
