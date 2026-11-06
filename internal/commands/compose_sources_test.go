package commands

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/compose"
)

// countingCaller counts the requests compose sends, by "METHOD route".
type countingCaller struct {
	compose.Caller
	calls map[string]int
}

func (c *countingCaller) Call(method, route string, body any) (json.RawMessage, error) {
	c.calls[method+" "+route]++
	return c.Caller.Call(method, route, body)
}

// composeSourcesDaemon serves the command registry on a Unix socket in a
// temporary directory, with an isolated base directory, and returns a real
// client for it. No daemon process and no TCP listener of the host is used.
func composeSourcesDaemon(t *testing.T) *countingCaller {
	t.Helper()
	c := workflowCtx(t)
	c.Groups = &fakeGroups{} // compose reads the group list; these files declare no group
	sockDir, err := os.MkdirTemp("", "cs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "api.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(api.NewServer(BuildRegistry(), c).Handler())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return &countingCaller{Caller: client.New(sock), calls: map[string]int{}}
}

func composeGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func writeComposeSource(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func imageSourceBody(version string) string {
	return "schema_version: 2\nimage_version: " + version + "\n"
}

// composeTeamRepo makes a bare git repository whose tag v1 gives image
// developer 1.0.0 and whose branch main gives 1.1.0, both with workflow demo
// 0.1.0. https://store.example/team.git resolves to it through a git URL
// rewrite; any other https://store.example/ URL resolves to a path that does
// not exist, so no test reaches the network.
func composeTeamRepo(t *testing.T) {
	t.Helper()
	seed, remote := t.TempDir(), filepath.Join(t.TempDir(), "team.git")
	composeGit(t, "init", "-q", "-b", "main", seed)
	writeComposeSource(t, filepath.Join(seed, "images", "developer", "Tariboyfile.yaml"), imageSourceBody("1.0.0"))
	writeWorkflowSource(t, filepath.Join(seed, "workflows", "demo"), "0.1.0")
	composeGit(t, "-C", seed, "add", ".")
	composeGit(t, "-C", seed, "commit", "-q", "-m", "v1")
	composeGit(t, "-C", seed, "tag", "v1")
	writeComposeSource(t, filepath.Join(seed, "images", "developer", "Tariboyfile.yaml"), imageSourceBody("1.1.0"))
	composeGit(t, "-C", seed, "commit", "-q", "-am", "1.1.0")
	composeGit(t, "clone", "-q", "--bare", seed, remote)
	config := filepath.Join(t.TempDir(), "gitconfig")
	writeComposeSource(t, config, "[url \"file://"+remote+"\"]\n\tinsteadOf = https://store.example/team.git\n"+
		"[url \"file:///nonexistent-compose-store/\"]\n\tinsteadOf = https://store.example/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func composeUp(t *testing.T, call compose.Caller, dir, doc string) (string, error) {
	t.Helper()
	f, err := compose.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err = compose.NewRunner(call, dir, &out).Up(f)
	return out.String(), err
}

func imageField(t *testing.T, call compose.Caller, ref, field string) string {
	t.Helper()
	raw, err := call.Call("GET", "/api/images/"+ref, map[string]string{})
	if err != nil {
		t.Fatalf("inspect %s: %v", ref, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	s, _ := m[field].(string)
	return s
}

func requireReport(t *testing.T, out string, lines ...string) {
	t.Helper()
	for _, want := range lines {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

const composeSourcesFile = `version: 2
stores:
  team: {git: https://store.example/team.git, ref: REF}
  local: {path: ./local}
workflows:
  demo: {store: team}
  review: {path: ./review}
images:
  developer: {store: team VERSION}
  tester: {store: local}
  reviewer: {path: ./reviewer}
`

func composeSourcesDoc(ref, version string) string {
	if version != "" {
		version = ", version: " + version
	}
	return strings.NewReplacer("REF", ref, " VERSION", version).Replace(composeSourcesFile)
}

// Criterion 1 and 2 against an in-process daemon, a local git repository and
// a local directory: up creates the Stores and builds images and workflows
// from a Store and from a path; a second up rebuilds no Store image and moves
// no tag; a ref change builds the new version; a version conflict leaves the
// tag; an unavailable Store skips what comes from it.
func TestComposeUpAppliesStoresWorkflowsAndImages(t *testing.T) {
	composeTeamRepo(t)
	dir := t.TempDir()
	writeComposeSource(t, filepath.Join(dir, "local", "images", "tester", "Tariboyfile.yaml"), imageSourceBody("3.0.0"))
	writeComposeSource(t, filepath.Join(dir, "reviewer", "Tariboyfile.yaml"), imageSourceBody("2.0.0"))
	writeWorkflowSource(t, filepath.Join(dir, "review"), "0.2.0")
	manifest := filepath.Join(dir, "review", "Workflowfile.yaml")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeComposeSource(t, manifest, strings.Replace(string(body), "name: demo", "name: review", 1))
	call := composeSourcesDaemon(t)

	out, err := composeUp(t, call, dir, composeSourcesDoc("v1", ""))
	if err != nil {
		t.Fatalf("first up: %v\n%s", err, out)
	}
	requireReport(t, out, "changed  store:local", "changed  store:team", "changed  workflow:demo", "changed  workflow:review",
		"changed  image:developer", "changed  image:reviewer", "changed  image:tester")
	if got := imageField(t, call, "developer:latest", "image_version"); got != "1.0.0" {
		t.Fatalf("developer image_version = %q, want 1.0.0 from tag v1", got)
	}
	if got := imageField(t, call, "tester:latest", "image_version"); got != "3.0.0" {
		t.Fatalf("tester image_version = %q", got)
	}
	for _, ref := range []string{"demo/latest", "review/0.2.0"} {
		if _, err := call.Call("GET", "/api/workflow-images/"+ref, map[string]string{}); err != nil {
			t.Fatalf("workflow %s: %v", ref, err)
		}
	}

	digests := map[string]string{}
	for _, ref := range []string{"developer:latest", "tester:latest", "reviewer:latest"} {
		digests[ref] = imageField(t, call, ref, "digest")
	}
	builds := call.calls["POST /api/images/build"]
	out, err = composeUp(t, call, dir, composeSourcesDoc("v1", ""))
	if err != nil {
		t.Fatalf("second up: %v\n%s", err, out)
	}
	requireReport(t, out, "up: 7 ok, 0 changed, 0 failed, 0 skipped")
	if got := call.calls["POST /api/images/build"] - builds; got != 1 {
		t.Fatalf("second up built %d images, want only the path image", got)
	}
	for ref, digest := range digests {
		if got := imageField(t, call, ref, "digest"); got != digest {
			t.Fatalf("second up moved %s: %s -> %s", ref, digest, got)
		}
	}

	out, err = composeUp(t, call, dir, composeSourcesDoc("main", ""))
	if err != nil {
		t.Fatalf("ref up: %v\n%s", err, out)
	}
	requireReport(t, out, "changed  store:team", "changed  image:developer")
	if got := imageField(t, call, "developer:latest", "image_version"); got != "1.1.0" {
		t.Fatalf("developer image_version after the ref change = %q, want 1.1.0", got)
	}

	moved := imageField(t, call, "developer:latest", "digest")
	out, err = composeUp(t, call, dir, composeSourcesDoc("v1", "1.1.0"))
	if err == nil {
		t.Fatalf("up with a version conflict succeeded:\n%s", out)
	}
	requireReport(t, out, "failed   image:developer: image developer: expected version 1.1.0, the source gives 1.0.0; the tag is not moved")
	if got := imageField(t, call, "developer:latest", "digest"); got != moved {
		t.Fatalf("the conflict moved developer:latest: %s -> %s", moved, got)
	}

	out, err = composeUp(t, call, dir, `version: 2
stores:
  broken: {git: https://store.example/missing.git}
  gone: {path: ./gone}
workflows:
  orphan: {store: broken}
images:
  lost: {store: gone}
  reviewer: {path: ./reviewer}
`)
	if err == nil {
		t.Fatalf("up with unavailable Stores succeeded:\n%s", out)
	}
	requireReport(t, out, "skipped  workflow:orphan: an action it depends on failed", "skipped  image:lost: an action it depends on failed",
		"ok       image:reviewer")
	for _, store := range []string{"store:broken", "store:gone"} {
		if !strings.Contains(out, "failed   "+store+": create store") {
			t.Errorf("report lacks a failed %s:\n%s", store, out)
		}
	}
}
