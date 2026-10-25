package commands

import (
	"encoding/base64"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/harness"
	"github.com/alekzonder/tariboy/internal/plugins"
	"github.com/alekzonder/tariboy/internal/registry"
)

var updatePluginsGolden = flag.Bool("update-plugins-golden", false, "rewrite testdata/plugin_responses.golden.json")

// silentPluginRunner starts a plugin process that never sends its handshake,
// so a started plugin stays in the starting state for the test's duration.
type silentPluginRunner struct{}

func (silentPluginRunner) Start(plugins.SpawnSpec, io.Writer) (plugins.ProcHandle, error) {
	return &silentPluginHandle{done: make(chan struct{})}, nil
}

type silentPluginHandle struct{ done chan struct{} }

func (h *silentPluginHandle) Handshake() <-chan plugins.HandshakeResult { return nil }
func (h *silentPluginHandle) Done() <-chan struct{}                     { return h.done }
func (h *silentPluginHandle) Pid() int                                  { return 4242 }
func (h *silentPluginHandle) Stop(time.Duration) error {
	select {
	case <-h.done:
	default:
		close(h.done)
	}
	return nil
}

// pluginsMiscFixture is the state the plugin, harness, secret, fs.list and
// files.upload responses cover. A real plugin host serves two stored
// plugins: echo, enabled with channels, a provided channel, operator commands,
// settings and a log, and quiet, disabled with no types or channels and an
// empty log. widgetSrc is an installable plugin directory. Agent smoke holds
// one secret; agent bare holds none. fsRoot is the TARIBOY_FS_ROOT the
// caller sets, holding directories proj/sub and docs and one file.
type pluginsMiscFixture struct {
	c         *registry.Ctx
	widgetSrc string
	fsRoot    string
}

func newPluginsMiscFixture(t *testing.T) pluginsMiscFixture {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := func() time.Time { return time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC) }
	ps := plugins.NewStore(c.Store, clock)
	pluginsDir := filepath.Join(c.BaseDir, "plugins")
	host := plugins.NewHost(plugins.HostConfig{
		Store: ps, Bus: c.Bus, Tokens: plugins.NewTokenRegistry(nil),
		PluginsDir: pluginsDir, DaemonSocket: filepath.Join(c.BaseDir, "d.sock"),
		Runner: silentPluginRunner{}, Clock: clock, Log: c.Log,
	})
	t.Cleanup(host.StopAll)
	c.Plugins = host

	echoManifest := `{"name":"echo","version":"1.0.0","protocol_version":1,"types":["channel-source","channel-sink"],"exec":"run.sh",` +
		`"description":"Echo bridge",` +
		`"operator_commands":[{"path":"status","summary":"Show status","action":"status"},` +
		`{"path":"bind","summary":"Bind a chat","action":"bind","args":[{"name":"chat","flag":"chat","type":"string","required":true,"help":"chat id"}]}],` +
		`"settings":{"title":"Echo","status":[{"name":"connected","label":"Connected"}],` +
		`"sections":[{"title":"Token","fields":[{"name":"token","label":"Token","type":"password","required":true,"help":"bot token"}],` +
		`"actions":[{"label":"Save","action":"configure","fields":["token"]}]}]},` +
		`"channels":{"publish":["echo:*"],"subscribe":["chat:in"],"provide":[{"channel":"echo:feed","params_schema":{"type":"object"},"help":"feed"}]}}`
	writePluginPackage(t, filepath.Join(pluginsDir, "echo", "1.0.0"), echoManifest)
	if err := ps.Upsert(plugins.Record{
		Name: "echo", Version: "1.0.0", Types: []string{"channel-source", "channel-sink"}, ProtocolVersion: 1,
		Exec: "run.sh", SourcePath: "/src/echo", Enabled: true,
		Channels: plugins.Channels{Publish: []string{"echo:*"}, Subscribe: []string{"chat:in"},
			Provide: []plugins.Provided{{Channel: "echo:feed", ParamsSchema: []byte(`{"type":"object"}`), Help: "feed"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetActiveVersion("echo", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	writePluginFile(t, filepath.Join(pluginsDir, "echo", "logs", "plugin.log"), "first line\nsecond line\nthird line\n")
	if err := ps.Upsert(plugins.Record{Name: "quiet", Version: "0.1.0", ProtocolVersion: 1, Exec: "run.sh", SourcePath: "/src/quiet"}); err != nil {
		t.Fatal(err)
	}
	writePluginFile(t, filepath.Join(pluginsDir, "quiet", "logs", "plugin.log"), "")

	widgetSrc := filepath.Join(t.TempDir(), "widget")
	writePluginPackage(t, widgetSrc, `{"name":"widget","version":"2.0.0","protocol_version":1,"types":["channel-sink"],"exec":"run.sh","channels":{"publish":[],"subscribe":["chat:*"]}}`)

	for _, name := range []string{"smoke", "bare"} {
		if err := as.Create(agent.Agent{Name: name, OnTimeout: "restart", OnError: "restart"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := as.SecretSet("smoke", "API_KEY", "s3cr3t"); err != nil {
		t.Fatal(err)
	}

	fsRoot := t.TempDir()
	for _, dir := range []string{"proj/sub", "docs"} {
		if err := os.MkdirAll(filepath.Join(fsRoot, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writePluginFile(t, filepath.Join(fsRoot, "notes.txt"), "x")
	return pluginsMiscFixture{c: c, widgetSrc: widgetSrc, fsRoot: fsRoot}
}

func writePluginPackage(t *testing.T, dir, manifest string) {
	t.Helper()
	writePluginFile(t, filepath.Join(dir, "plugin.json"), manifest)
	writePluginFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")
}

func writePluginFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// uploadDir returns the random directory name the single upload created
// under baseDir/files.
func uploadDir(t *testing.T, baseDir string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(baseDir, "files"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("upload directories = %v, %v", entries, err)
	}
	return entries[0].Name()
}

// Catches a typed plugin, harness, secret, fs.list or files.upload response
// changing the JSON a client sees: the set of keys, their JSON types, and
// their values must match the recorded responses.
func TestPluginsMiscResponsesMatchGolden(t *testing.T) {
	// Sequential: sets TARIBOY_FS_ROOT with t.Setenv.
	f := newPluginsMiscFixture(t)
	t.Setenv("TARIBOY_FS_ROOT", f.fsRoot)
	type call struct {
		label, path string
		params      registry.Params
	}
	upload := call{"files.upload", "files.upload", registry.Params{"name": "notes.txt", "content": base64.StdEncoding.EncodeToString([]byte("hello"))}}
	calls := []call{
		{"plugin.ls", "plugin.ls", registry.Params{}},
		{"plugin.inspect echo", "plugin.inspect", registry.Params{"name": "echo"}},
		{"plugin.inspect quiet", "plugin.inspect", registry.Params{"name": "quiet"}},
		{"plugin.contributions", "plugin.contributions", registry.Params{}},
		{"plugin.logs echo", "plugin.logs", registry.Params{"name": "echo", "tail": 2}},
		{"plugin.logs quiet empty", "plugin.logs", registry.Params{"name": "quiet"}},
		{"plugin.logs ghost missing", "plugin.logs", registry.Params{"name": "ghost"}},
		{"plugin.restart echo", "plugin.restart", registry.Params{"name": "echo"}},
		{"plugin.rm quiet", "plugin.rm", registry.Params{"name": "quiet"}},
		{"plugin.install widget", "plugin.install", registry.Params{"path": f.widgetSrc}},
		{"harness.models claude", "harness.models", registry.Params{"type": "claude"}},
		{"harness.models stub", "harness.models", registry.Params{"type": "stub"}},
		{"harness.codex-approval.get default", "harness.codex-approval.get", registry.Params{}},
		{"harness.codex-approval.set", "harness.codex-approval.set", registry.Params{"mode": harness.CodexApprovalApproveForMe}},
		{"harness.codex-approval.get stored", "harness.codex-approval.get", registry.Params{}},
		{"secret.store", "secret.store", registry.Params{"name": "smoke", "key": "TOKEN", "value": "v"}},
		{"secret.ls smoke", "secret.ls", registry.Params{"name": "smoke"}},
		{"secret.ls bare", "secret.ls", registry.Params{"name": "bare"}},
		{"secret.rm", "secret.rm", registry.Params{"name": "smoke", "key": "TOKEN"}},
		{"fs.list root", "fs.list", registry.Params{}},
		{"fs.list proj", "fs.list", registry.Params{"path": "proj"}},
		upload,
	}
	values := map[string]any{}
	for _, call := range calls {
		value, err := h(t, call.path)(f.c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		values[call.label] = value
	}
	recorder := newGoldenRecorder(t, map[string]bool{},
		f.c.BaseDir, "$BASE", f.fsRoot, "$FSROOT", f.widgetSrc, "$WIDGET",
		uploadDir(t, f.c.BaseDir), "$UPLOAD")
	for _, call := range calls {
		recorder.record(call.label, values[call.label])
	}
	recorder.check("plugin_responses.golden.json", *updatePluginsGolden)
}
