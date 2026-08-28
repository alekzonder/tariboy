package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodManifest = `{
  "name": "echo",
  "version": "0.1.0",
  "protocol_version": 1,
  "types": ["channel-source", "channel-sink"],
  "exec": "echo.py",
  "description": "echo plugin",
  "channels": {"publish": ["chat:echo-out"], "subscribe": ["chat:echo-in"]}
}`

func TestParseAndValidate(t *testing.T) {
	m, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "echo" || m.ProtocolVersion != 1 || !m.HasType("channel-sink") {
		t.Fatalf("manifest = %+v", m)
	}
	if m.Channels.Publish[0] != "chat:echo-out" {
		t.Fatalf("channels = %+v", m.Channels)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Manifest { m, _ := ParseManifest([]byte(goodManifest)); return m }
	cases := map[string]func(*Manifest){
		"bad name":                       func(m *Manifest) { m.Name = "Echo Plugin!" },
		"empty name":                     func(m *Manifest) { m.Name = "" },
		"abs exec":                       func(m *Manifest) { m.Exec = "/usr/bin/evil" },
		"escaping exec":                  func(m *Manifest) { m.Exec = "../../evil" },
		"empty exec":                     func(m *Manifest) { m.Exec = "" },
		"unknown type":                   func(m *Manifest) { m.Types = []string{"channel-source", "wat"} },
		"no types":                       func(m *Manifest) { m.Types = nil },
		"bad protocol":                   func(m *Manifest) { m.ProtocolVersion = 2 },
		"reserved version active marker": func(m *Manifest) { m.Version = "active-version" },
		"reserved version logs":          func(m *Manifest) { m.Version = "logs" },
		"reserved version workdir":       func(m *Manifest) { m.Version = "workdir" },
		"reserved version socket":        func(m *Manifest) { m.Version = "plugin.sock" },
		"abs prompt":                     func(m *Manifest) { m.Prompt = "/etc/passwd" },
		"escaping prompt":                func(m *Manifest) { m.Prompt = "../../secret.md" },
	}
	for name, mutate := range cases {
		m := base()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestValidateRejectsOperatorCommandWithoutAction(t *testing.T) {
	m, err := ParseManifest([]byte(`{
  "name":"telegram","version":"0.1.0","protocol_version":1,
  "types":["channel-source"],"exec":"telegram",
  "channels":{"publish":["chat:telegram:*"],"subscribe":[]},
  "operator_commands":[{"path":"configure","summary":"Configure Telegram","action":""}]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err == nil {
		t.Fatal("operator command without action should be rejected")
	}
}

func TestValidateOperatorContributions(t *testing.T) {
	valid := `{
  "name":"telegram","version":"0.1.0","protocol_version":1,
  "types":["channel-source","channel-sink"],"exec":"telegram",
  "channels":{"publish":["chat:telegram:*"],"subscribe":["chat:telegram:*"]},
  "operator_commands":[{
    "path":"configure","summary":"Configure Telegram","action":"configure",
    "args":[
      {"name":"token","flag":"token-file","type":"secret-file","help":"token file"},
      {"name":"allowed_uids","flag":"allowed-uids","type":"integer-list","required":true}
    ]
  }],
  "settings":{
    "title":"Telegram",
    "status":[{"name":"token_configured","label":"Token configured"}],
    "sections":[{
      "title":"Bot",
      "fields":[
        {"name":"token","label":"Bot token","type":"password"},
        {"name":"allowed_uids","label":"Allowed UIDs","type":"integer-list"}
      ],
      "actions":[{"label":"Save","action":"configure","fields":["token","allowed_uids"]}]
    }]
  }
}`
	m, err := ParseManifest([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid operator contributions rejected: %v", err)
	}

	cases := map[string]string{
		"absolute command path":    strings.Replace(valid, `"path":"configure"`, `"path":"telegram.configure"`, 1),
		"missing command summary":  strings.Replace(valid, `"summary":"Configure Telegram"`, `"summary":""`, 1),
		"unsupported arg type":     strings.Replace(valid, `"type":"integer-list"`, `"type":"json"`, 1),
		"secret without flag":      strings.Replace(valid, `"flag":"token-file"`, `"flag":""`, 1),
		"unsupported setting type": strings.Replace(valid, `"type":"password"`, `"type":"html"`, 1),
		"unknown action field":     strings.Replace(valid, `"fields":["token","allowed_uids"]`, `"fields":["missing"]`, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := ParseManifest([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Validate(); err == nil {
				t.Fatal("invalid contribution should be rejected")
			}
		})
	}
}

const provideManifest = `{
  "name": "issue-provider",
  "version": "0.1.0",
  "protocol_version": 1,
  "types": ["channel-source"],
  "exec": "plugin.py",
  "description": "issue-provider provider",
  "channels": {
    "publish": ["issue-provider:*"],
    "subscribe": ["issue-provider:outbox"],
    "provide": [
      {"channel": "issue-provider:query",
       "params_schema": {"type": "object", "required": ["query"],
                         "properties": {"query": {"type": "string"}}},
       "help": "Subscribe with {query: ...}"}
    ]
  }
}`

func TestValidateAcceptsProvide(t *testing.T) {
	m, err := ParseManifest([]byte(provideManifest))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid provide manifest rejected: %v", err)
	}
	if len(m.Channels.Provide) != 1 || m.Channels.Provide[0].Channel != "issue-provider:query" {
		t.Fatalf("provide not parsed: %+v", m.Channels.Provide)
	}
}

func TestValidateRejectsProvide(t *testing.T) {
	base := func() Manifest { m, _ := ParseManifest([]byte(provideManifest)); return m }
	cases := map[string]func(*Manifest){
		"provide outside publish scope": func(m *Manifest) {
			m.Channels.Provide[0].Channel = "other:query" // not matched by issue-provider:*
		},
		"provide empty channel": func(m *Manifest) {
			m.Channels.Provide[0].Channel = ""
		},
		"duplicate provided channel": func(m *Manifest) {
			m.Channels.Provide = append(m.Channels.Provide, m.Channels.Provide[0])
		},
		"malformed params_schema type": func(m *Manifest) {
			m.Channels.Provide[0].ParamsSchema = []byte(`{"type": "objekt"}`)
		},
		"non-object params_schema": func(m *Manifest) {
			m.Channels.Provide[0].ParamsSchema = []byte(`"not a schema"`)
		},
	}
	for name, mutate := range cases {
		m := base()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(goodManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(dir)
	if err != nil || m.Name != "echo" {
		t.Fatalf("load = %+v err=%v", m, err)
	}
	if _, err := LoadManifest(t.TempDir()); err == nil {
		t.Fatal("missing plugin.json should error")
	}
}

const promptManifest = `{
  "name": "sample-plugin",
  "version": "0.1.0",
  "protocol_version": 1,
  "types": ["tool"],
  "exec": "plugin.py",
  "description": "sample plugin",
  "prompt": "PROMPT.md",
  "channels": {"publish": [], "subscribe": []}
}`

func TestLoadPrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(promptManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROMPT.md"), []byte("## Sample guidance"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, ok, err := LoadPrompt(dir)
	if err != nil || !ok || body != "## Sample guidance" {
		t.Fatalf("LoadPrompt = %q ok=%v err=%v", body, ok, err)
	}

	// A plugin that declares no prompt resolves to ok=false, no error.
	noprompt := t.TempDir()
	if err := os.WriteFile(filepath.Join(noprompt, "plugin.json"), []byte(goodManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := LoadPrompt(noprompt); ok || err != nil {
		t.Fatalf("no-prompt plugin: ok=%v err=%v", ok, err)
	}

	// Declared but missing file is an error.
	miss := t.TempDir()
	if err := os.WriteFile(filepath.Join(miss, "plugin.json"), []byte(promptManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPrompt(miss); err == nil {
		t.Fatal("missing prompt file should error")
	}
}

func TestResolveInstalledReadsDeclaredPrompt(t *testing.T) {
	root := t.TempDir()
	pdir := filepath.Join(root, "sample-plugin")
	if err := os.MkdirAll(pdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(promptManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "PROMPT.md"), []byte("BD PROMPT"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolve := ResolveInstalled(root)

	got, err := resolve("sample-plugin")
	if err != nil || !got.Installed || !got.HasPrompt || got.Prompt != "BD PROMPT" {
		t.Fatalf("resolve sample plugin = %+v err=%v", got, err)
	}
	// A builtin capability with no installed plugin directory remains absent.
	if got, err := resolve("context"); got.Installed || err != nil {
		t.Fatalf("resolve context: got=%+v err=%v", got, err)
	}
	// A path-traversal name never touches the filesystem.
	if _, err := resolve("../sample-plugin"); err == nil {
		t.Fatal("resolve traversal succeeded")
	}
}

func TestResolveInstalledDistinguishesPromptlessPluginFromMissing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "issue-provider")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name":"issue-provider","version":"1.0.0","protocol_version":1,
  "types":["channel-source"],"exec":"issue-provider",
  "channels":{"publish":["issue:*"],"subscribe":[]}
}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	resolve := ResolveInstalled(root)
	got, err := resolve("issue-provider")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.HasPrompt || got.Prompt != "" {
		t.Fatalf("promptless installed plugin = %+v", got)
	}
	missing, err := resolve("missing-provider")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Installed || missing.HasPrompt || missing.Prompt != "" {
		t.Fatalf("missing plugin = %+v", missing)
	}
}
