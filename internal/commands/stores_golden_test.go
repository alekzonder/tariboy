package commands

import (
	"flag"
	"os/exec"
	"path/filepath"
	"strings"
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

// gitStoreCommits are the commits of the Git Store gitStoreFixture places.
type gitStoreCommits struct{ v1, main string }

// gitStoreFixture registers the Git Store "repo" with its managed clone in
// place. The clone's origin is a local bare repository, so a fetch needs no
// network and no Git configuration. Commit v1 carries tag v1; main is one
// commit later.
func gitStoreFixture(t *testing.T, c *registry.Ctx) gitStoreCommits {
	t.Helper()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	seed, remote := t.TempDir(), filepath.Join(t.TempDir(), "remote.git")
	git("init", "-q", "-b", "main", seed)
	commit := func(message string) string {
		git("-C", seed, "commit", "-q", "--allow-empty", "-m", message)
		return git("-C", seed, "rev-parse", "HEAD")
	}
	commits := gitStoreCommits{v1: commit("v1")}
	git("-C", seed, "tag", "v1")
	commits.main = commit("main")
	git("clone", "-q", "--bare", seed, remote)
	git("clone", "-q", remote, filepath.Join(c.BaseDir, "stores", "repo"))
	if _, err := c.Store.DB.Exec(`INSERT INTO image_stores(name,source) VALUES('repo','https://store.example/repo.git')`); err != nil {
		t.Fatal(err)
	}
	return commits
}

// Catches a typed stores response changing the JSON a client sees: the set of
// keys, their JSON types, and their values must match the recorded responses.
func TestStoresResponsesMatchGolden(t *testing.T) {
	t.Parallel()
	c, dirs := storesFixture(t)
	type call struct {
		label, path string
		params      registry.Params
	}
	calls := []call{
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
	run := func(calls []call) {
		for _, call := range calls {
			value, err := h(t, call.path)(c, call.params)
			if err != nil {
				t.Fatalf("%s: %v", call.label, err)
			}
			recorder.record(call.label, value)
		}
	}
	run(calls)
	// The Git Store comes after "store.list empty"; its commit ids vary by run.
	commits := gitStoreFixture(t, c)
	recorder.replacements = append(recorder.replacements, commits.v1, "$COMMIT_V1", commits.main, "$COMMIT_MAIN")
	run([]call{
		{"store.ref tag", "store.ref", registry.Params{"name": "repo", "ref": "v1"}},
		{"store.ref default", "store.ref", registry.Params{"name": "repo", "ref": ""}},
		{"store.list git", "store.list", registry.Params{}},
	})
	recorder.check("stores_responses.golden.json", *updateStoresGolden)
}

func init() { typedCallSets["stores"] = storesTypedCalls }

// storesTypedCalls drives every typed stores operation over HTTP, including
// a Git Store set to a tag, a Store image and workflow with an error, and an
// automatic build policy that is set and then disabled.
func storesTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, dirs := storesFixture(t)
	gitStoreFixture(t, c)
	return c, []typedCall{
		{"POST", "/api/stores/{name}/ref", "/api/stores/repo/ref", map[string]any{"ref": "v1"}},
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
