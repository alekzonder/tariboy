package harness

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeInstantMailPluginRunsTmailInstant(t *testing.T) {
	hooks, err := opencode{}.InstantMailHooks("/opt/tariboy/bin/tmail", "/agents/a/harness-hooks/opencode", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks.Files) != 1 || hooks.Files[0].Path != "tariboy-instant-mail.js" || hooks.Files[0].Mode != 0o600 {
		t.Fatalf("files = %+v", hooks.Files)
	}
	js := string(hooks.Files[0].Body)
	for _, want := range []string{`"/opt/tariboy/bin/tmail"`, `"opencode-tool-after"`, `"tool.execute.after"`, "TARIBOY_INSTANT_MAIL_FLAG", "Bun.spawn"} {
		if !strings.Contains(js, want) {
			t.Errorf("plugin lacks %s:\n%s", want, js)
		}
	}
	if len(hooks.Args) != 0 {
		t.Fatalf("args = %q; want none, the plugin goes into the config overlay", hooks.Args)
	}
	if hooks.ConfigPlugin != "file:///agents/a/harness-hooks/opencode/tariboy-instant-mail.js" {
		t.Fatalf("config plugin = %q", hooks.ConfigPlugin)
	}
	if hooks.MinimumVersion != minimumOpenCodeInstantPluginVersion {
		t.Fatalf("minimum version = %q", hooks.MinimumVersion)
	}
}

// A path with a space, a quote and a backslash stays one JSON string in the
// script and one file URL that names the same path.
func TestOpenCodeInstantMailPluginEscapesPaths(t *testing.T) {
	dir := `/agents/it's a "dir"\x/opencode`
	tmail := `/opt/my "bin"/t'mail`
	hooks, err := opencode{}.InstantMailHooks(tmail, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(tmail)
	if !strings.Contains(string(hooks.Files[0].Body), "const TMAIL = "+string(quoted)+";") {
		t.Fatalf("tmail not a JSON string:\n%s", hooks.Files[0].Body)
	}
	u, err := url.Parse(hooks.ConfigPlugin)
	if err != nil || u.Scheme != "file" || u.Path != filepath.Join(dir, "tariboy-instant-mail.js") {
		t.Fatalf("config plugin %q parses to %+v, %v", hooks.ConfigPlugin, u, err)
	}
}

func TestOpenCodeInstantMailPluginNeedsAbsolutePaths(t *testing.T) {
	if _, err := (opencode{}).InstantMailHooks("tmail", "/x/opencode", Config{}); err == nil {
		t.Fatal("relative tmail accepted")
	}
	if _, err := (opencode{}).InstantMailHooks("/bin/tmail", "rel/opencode", Config{}); err == nil {
		t.Fatal("relative plugin dir accepted")
	}
}
