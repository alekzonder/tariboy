package cli_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/daemon"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

const packWorkflowFile = `schema_version: 1
name: demo
workflow_version: 1.0.0
initial_status: work
statuses:
  - id: work
    owner: { pool: devs }
    transitions:
      - on: done
        to: finished
        checks:
          - script: ./check.sh
  - id: finished
    terminal: true
`

// The secret values must never reach the archive.
const (
	packAgentSecret = "agent-secret-value-7f3a"
	packQueueSecret = "queue-secret-value-91bd"
)

const packComposeFile = `version: 2
stores:
  team:
    path: ./store
workflows:
  demo:
    path: ./wf
images:
  stub:
    path: ./img
groups:
  research:
    lead: scout
agents:
  scout:
    image: stub:latest
    group: research
    loop: { enabled: false }
    secrets:
      API_TOKEN: { from_env: PACK_TEST_AGENT_TOKEN }
task_queues:
  DEV:
    name: Development
    workflow: demo
    pools:
      devs: [scout]
    secrets:
      QUEUE_TOKEN: { from_env: PACK_TEST_QUEUE_TOKEN }
`

// writePackProject writes a compose project: an image with a skill, a
// workflow and the compose file. It returns the compose file path.
func writePackProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"img/Tariboyfile.yaml":                 "schema_version: 2\nplugins: []\nskills: [{dir: ./skills/pack-skill}]\nprompts: []\n",
		"img/skills/pack-skill/SKILL.md":       "---\nname: pack-skill\ndescription: Use for compose pack tests.\n---\n",
		"img/skills/pack-skill/scripts/run.sh": "#!/bin/sh\nexit 0\n",
		"wf/Workflowfile.yaml":                 packWorkflowFile,
		"wf/check.sh":                          "#!/bin/sh\nexit 0\n",
		"store/README.md":                      "an unused path Store\n",
		"Tariboycompose.yaml":                  packComposeFile,
	}
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o600)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0o700
		}
		if err := os.WriteFile(full, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "Tariboycompose.yaml")
}

// isolateEnv sets the secret variables the compose file reads and keeps the
// CLI from starting a daemon.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TARIBOY_NO_AUTOSTART", "1")
	t.Setenv("PACK_TEST_AGENT_TOKEN", packAgentSecret)
	t.Setenv("PACK_TEST_QUEUE_TOKEN", packQueueSecret)
}

// startIsolatedDaemon starts an in-process daemon (unix socket only, no TCP
// listener) whose TARIBOY_BASE_DIR and TARIBOY_RUNTIME_DIR are its own
// temporary directory, set while it starts. The live daemon is never used.
func startIsolatedDaemon(t *testing.T) (sock string, c *client.Client) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("TARIBOY_BASE_DIR", base)
	t.Setenv("TARIBOY_RUNTIME_DIR", base)
	storetest.Seed(t, paths.New(base).DB())
	// Stored workflow images are read-only; make them removable once the
	// daemon stopped (cleanups run in reverse order).
	t.Cleanup(func() {
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if d != nil && d.IsDir() {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		_ = daemon.Run(ctx, inProcessDaemonOptions(daemon.Options{BaseDir: base, Listen: "unix", LogLevel: "error",
			Ready: func() { close(ready) }}))
	}()
	waitReady(t, ready, done)
	sock = paths.New(base).Socket()
	return sock, client.New(sock)
}

// runCompose runs `tariboy compose ARGS` through the CLI registry.
func runCompose(t *testing.T, sock string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), commands.BuildRegistry(), append([]string{"compose"}, args...),
		client.New(sock), &registry.Ctx{Socket: sock}, &out, &errOut)
	return code, out.String(), errOut.String()
}

// packOnSource applies the project to a new daemon with compose up and packs
// it.
func packOnSource(t *testing.T) (archive string, src *client.Client) {
	t.Helper()
	file := writePackProject(t)
	sock, c := startIsolatedDaemon(t)
	if code, out, errOut := runCompose(t, sock, "up", "-f", file); code != 0 {
		t.Fatalf("up exit %d\nout:\n%s\nerr:\n%s", code, out, errOut)
	}
	archive = filepath.Join(t.TempDir(), "team.tariboy.tar.gz")
	if code, out, errOut := runCompose(t, sock, "pack", "-f", file, "-o", archive); code != 0 {
		t.Fatalf("pack exit %d\nout:\n%s\nerr:\n%s", code, out, errOut)
	}
	return archive, c
}

// guardCaller fails the test on every route that builds or touches Stores:
// unpack installs from the archive only.
type guardCaller struct {
	t *testing.T
	c *client.Client
}

func (g guardCaller) check(route string) {
	for _, banned := range []string{"/api/images/build", "/api/workflow-images/build", "/api/stores"} {
		if strings.HasPrefix(route, banned) {
			g.t.Errorf("unpack called %s", route)
		}
	}
}

func (g guardCaller) Call(method, route string, body any) (json.RawMessage, error) {
	g.check(route)
	return g.c.Call(method, route, body)
}

func (g guardCaller) Upload(route string, body io.Reader, size int64) (json.RawMessage, error) {
	g.check(route)
	return g.c.Upload(route, body, size)
}

func (g guardCaller) Download(route string, w io.Writer) (int64, error) {
	g.check(route)
	return g.c.Download(route, w)
}

// stubOfflineTools puts npx and git stubs first on PATH; each call writes
// the marker file and fails. It also adds a claude stub for the skill probe.
func stubOfflineTools(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "called")
	for _, tool := range []string{"npx", "git"} {
		script := "#!/bin/sh\necho \"" + tool + " $*\" >> " + marker + "\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// The image skill bridge probes `claude --version`; CI has no claude binary.
	claude := "#!/bin/sh\necho '9.9.9 (Claude Code)'\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(claude), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func getJSON(t *testing.T, c *client.Client, route string) map[string]any {
	t.Helper()
	raw, err := c.Call("GET", route, map[string]string{})
	if err != nil {
		t.Fatalf("GET %s: %v", route, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("GET %s: %v", route, err)
	}
	return m
}

// teamState is what must match between the two daemons.
func teamState(t *testing.T, c *client.Client) map[string]any {
	t.Helper()
	agent := getJSON(t, c, "/api/agents/scout")
	group := getJSON(t, c, "/api/groups/research")
	queues := getJSON(t, c, "/api/task-queues")
	pools := map[string]any{}
	rows, _ := getJSON(t, c, "/api/task-queues/DEV/pools")["items"].([]any)
	for _, row := range rows {
		pool := row.(map[string]any)
		pools[pool["name"].(string)] = pool["agents"]
	}
	binding := getJSON(t, c, "/api/task-queues/DEV/workflow")
	return map[string]any{
		"agent.image":     agent["image"],
		"agent.group":     agent["group"],
		"group.lead":      group["lead"],
		"queues":          names(queues, "queues", "prefix"),
		"pools":           pools,
		"workflow.digest": binding["digest"],
		"workflow.tag":    binding["tag"],
		"image.digest":    getJSON(t, c, "/api/images/stub:latest")["digest"],
		"wf.1.0.0":        getJSON(t, c, "/api/workflow-images/demo/1.0.0")["digest"],
		"wf.latest":       getJSON(t, c, "/api/workflow-images/demo/latest")["digest"],
	}
}

// names collects one field of the rows of a list result.
func names(list map[string]any, key, field string) []string {
	var out []string
	rows, _ := list[key].([]any)
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, m[field].(string))
		}
	}
	return out
}

func TestComposePackUnpackOfflineOnSecondDaemon(t *testing.T) {
	isolateEnv(t)
	archive, src := packOnSource(t)
	want := teamState(t, src)

	marker := stubOfflineTools(t)
	_, dst := startIsolatedDaemon(t)
	var out, errOut bytes.Buffer
	code := compose.Run(context.Background(), guardCaller{t: t, c: dst},
		compose.Options{Verb: "unpack", Args: []string{archive}, Yes: true}, &out, &errOut)
	if code != 0 {
		t.Fatalf("unpack exit %d\nout:\n%s\nerr:\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "skipped (archive)") {
		t.Fatalf("unpack report has no skipped stores phase:\n%s", out.String())
	}
	if calls, err := os.ReadFile(marker); err == nil {
		t.Fatalf("unpack ran external tools:\n%s", calls)
	}
	got := teamState(t, dst)
	if mustJSONText(t, got) != mustJSONText(t, want) {
		t.Fatalf("second daemon differs:\n got %s\nwant %s", mustJSONText(t, got), mustJSONText(t, want))
	}

	// A second unpack finds every image with the same digest and installs
	// nothing again.
	out.Reset()
	if code := compose.Run(context.Background(), guardCaller{t: t, c: dst},
		compose.Options{Verb: "unpack", Args: []string{archive}, Yes: true}, &out, &errOut); code != 0 {
		t.Fatalf("second unpack exit %d\n%s\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "ok       image:stub") {
		t.Fatalf("second unpack imported the image again:\n%s", out.String())
	}
}

func mustJSONText(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Neither a secret value nor its base64 form appears in any byte of the
// archive, also after gunzip of the nested image and workflow archives; pack
// prints neither on stdout or stderr.
func TestComposePackKeepsSecretValuesOut(t *testing.T) {
	isolateEnv(t)
	file := writePackProject(t)
	sock, _ := startIsolatedDaemon(t)
	archive := filepath.Join(t.TempDir(), "team.tariboy.tar.gz")
	code, out, errOut := runCompose(t, sock, "pack", "-f", file, "-o", archive)
	if code != 0 {
		t.Fatalf("pack exit %d\n%s\n%s", code, out, errOut)
	}
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	blobs := append([][]byte{raw, []byte(out), []byte(errOut)}, expandAll(t, raw)...)
	for _, secret := range []string{packAgentSecret, packQueueSecret} {
		for _, form := range []string{secret, base64.StdEncoding.EncodeToString([]byte(secret))} {
			for _, b := range blobs {
				if bytes.Contains(b, []byte(form)) {
					t.Fatalf("secret %q found in pack output", form)
				}
			}
		}
	}
}

// expandAll returns every file body of a gzipped tar and, recursively, of the
// gzipped tars inside it.
func expandAll(t *testing.T, gz []byte) [][]byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	out := [][]byte{plain}
	tr := tar.NewReader(bytes.NewReader(plain))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, body)
		if strings.HasSuffix(h.Name, ".gz") {
			out = append(out, expandAll(t, body)...)
		}
	}
	return out
}
