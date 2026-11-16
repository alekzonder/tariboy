package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// codexHost answers `codex --version` with version and sets CODEX_HOME.
func codexHost(t *testing.T, version string) (*fakeHost, string) {
	t.Helper()
	home := t.TempDir()
	return &fakeHost{
		installed: map[string]bool{"codex": true},
		env:       map[string]string{"CODEX_HOME": home, "HOME": t.TempDir()},
		output: func(env []string, name string, args []string) ([]byte, error) {
			if name == "/fake/bin/codex" && slices.Equal(args, []string{"--version"}) && len(env) == 0 {
				return []byte("codex-cli " + version + "\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}, home
}

func codexRun(t *testing.T, prompt string) rundir.Run {
	t.Helper()
	run := twoSkillRun(t, prompt)
	run.Digest = testDigest
	return run
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestCodexPrepare(t *testing.T) {
	run := codexRun(t, "be \"helpful\"\n\tC:\\path ü\n")
	f, home := codexHost(t, "0.166.0")
	writeFile(t, filepath.Join(home, "config.toml"), "model = \"x\"\n", 0o600)
	launch, err := codex{}.Prepare(context.Background(), f.host(), run, []string{"--model", "o3"})
	if err != nil {
		t.Fatal(err)
	}
	wantArgv := []string{"codex", "--profile", "tbox-demo", "--model", "o3"}
	if launch.Path != "/fake/bin/codex" || !slices.Equal(launch.Argv, wantArgv) {
		t.Fatalf("launch %+v", launch)
	}
	wantEnv := []string{"TBOX_IMAGE_DIR=" + run.ImageDir, "TBOX_IMAGE=demo:latest"}
	if !slices.Equal(launch.Env, wantEnv) || len(launch.Notes) != 0 {
		t.Fatalf("env %v notes %v", launch.Env, launch.Notes)
	}
	plugin := filepath.Join(home, "plugins", "cache", "tbox", "demo", "1.2.0")
	manifest := readFile(t, filepath.Join(plugin, ".codex-plugin", "plugin.json"))
	var m map[string]string
	if err := json.Unmarshal([]byte(manifest), &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "demo" || m["version"] != "1.2.0" || m["skills"] != "./skills/" || m["description"] == "" || len(m) != 4 {
		t.Fatalf("plugin.json %s", manifest)
	}
	if body := readFile(t, filepath.Join(plugin, "skills", "beta", "SKILL.md")); !strings.Contains(body, "beta body") {
		t.Fatalf("beta %q", body)
	}
	if info, err := os.Stat(filepath.Join(plugin, "skills", "alpha", "scripts", "run.sh")); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("run.sh %v %v", info, err)
	}
	profile := readFile(t, filepath.Join(home, "tbox-demo.config.toml"))
	wantProfile := "# Written by tbox for image demo. `tbox clean` removes it.\n" +
		`developer_instructions = "be \"helpful\"\n\tC:\\path ü\n"` + "\n\n" +
		"[plugins.\"demo@tbox\"]\nenabled = true\n"
	if profile != wantProfile {
		t.Fatalf("profile:\n%s\nwant:\n%s", profile, wantProfile)
	}
	if got := readFile(t, filepath.Join(home, "config.toml")); got != "model = \"x\"\n" {
		t.Fatalf("config.toml changed: %q", got)
	}
	assertCodexHomeEntries(t, home, "config.toml", "plugins", "tbox-demo.config.toml")
}

// assertCodexHomeEntries checks that CODEX_HOME holds exactly names.
func assertCodexHomeEntries(t *testing.T, home string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, names) {
		t.Fatalf("CODEX_HOME entries %v, want %v", got, names)
	}
}

func TestCodexHomeDefaultsToHome(t *testing.T) {
	run := codexRun(t, "p\n")
	f, _ := codexHost(t, "0.166.0")
	delete(f.env, "CODEX_HOME")
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.env["HOME"], ".codex", "tbox-demo.config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestCodexVersionFromDigest(t *testing.T) {
	run := codexRun(t, "")
	run.ImageVersion = ""
	f, home := codexHost(t, "0.170.2")
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "plugins", "cache", "tbox", "demo", testDigest[:12], ".codex-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	profile := readFile(t, filepath.Join(home, "tbox-demo.config.toml"))
	if strings.Contains(profile, "developer_instructions") {
		t.Fatalf("empty prompt written: %s", profile)
	}
}

func TestCodexOldVersion(t *testing.T) {
	run := codexRun(t, "p\n")
	f, home := codexHost(t, "0.100.0")
	_, err := codex{}.Prepare(context.Background(), f.host(), run, nil)
	var unsupported ErrUnsupportedVersion
	if !errors.As(err, &unsupported) || unsupported.Harness != "codex" || unsupported.Found != "0.100.0" || unsupported.Minimum != "0.166.0" {
		t.Fatalf("error %#v", err)
	}
	assertCodexHomeEntries(t, home)
}

func TestCodexMissingBinary(t *testing.T) {
	_, err := codex{}.Prepare(context.Background(), (&fakeHost{}).host(), codexRun(t, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("error %v", err)
	}
}

func TestCodexSecondPrepareSameDigestRewritesNothing(t *testing.T) {
	run := codexRun(t, "p\n")
	f, home := codexHost(t, "0.166.0")
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(home, "plugins", "cache", "tbox", "demo", "1.2.0", ".codex-plugin", "plugin.json"),
		filepath.Join(home, "tbox-demo.config.toml"),
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, p := range files {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		if info, err := os.Stat(p); err != nil || !info.ModTime().Equal(old) {
			t.Fatalf("%s rewritten: %v %v", p, info, err)
		}
	}
}

func TestCodexNewDigestReplacesCache(t *testing.T) {
	run := codexRun(t, "p\n")
	f, home := codexHost(t, "0.166.0")
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(home, "plugins", "cache", "tbox", "demo")
	writeFile(t, filepath.Join(cache, "1.2.0", "skills", "stale", "SKILL.md"), "stale", 0o600)
	run.Digest = strings.Repeat("f", 64)
	run.ImageVersion = "1.3.0"
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(cache)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{".tbox-digest", "1.3.0"}) {
		t.Fatalf("cache entries %v", names)
	}
	if !strings.Contains(readFile(t, filepath.Join(cache, ".tbox-digest")), run.Digest) {
		t.Fatal("digest not recorded")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(home, "plugins", "cache", "tbox", ".plugin-*")); len(leftovers) != 0 {
		t.Fatalf("staging left behind: %v", leftovers)
	}
}

func TestCodexCollidingImageNameIsReplacedWithNote(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	first := codexRun(t, "p\n")
	first.Name = "a-b"
	if _, err := (codex{}).Prepare(context.Background(), f.host(), first, nil); err != nil {
		t.Fatal(err)
	}
	second := codexRun(t, "p\n")
	second.Name = "a.b"
	launch, err := codex{}.Prepare(context.Background(), f.host(), second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(launch.Argv, []string{"codex", "--profile", "tbox-a-b"}) {
		t.Fatalf("argv %v", launch.Argv)
	}
	if len(launch.Notes) != 1 || !strings.Contains(launch.Notes[0], `"a-b"`) || !strings.Contains(launch.Notes[0], `"a.b"`) {
		t.Fatalf("notes %v", launch.Notes)
	}
	if !strings.Contains(readFile(t, filepath.Join(home, "plugins", "cache", "tbox", "a-b", ".tbox-digest")), `"a.b"`) {
		t.Fatal("collision not recorded")
	}
}

func TestCodexRefusesSymlinkedCache(t *testing.T) {
	run := codexRun(t, "p\n")
	f, home := codexHost(t, "0.166.0")
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "plugins", "cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "plugins", "cache", "tbox")); err != nil {
		t.Fatal(err)
	}
	if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error %v", err)
	}
	if err := (codex{}).Clean(f.host(), ""); err == nil {
		t.Fatal("clean followed a symlinked cache")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("wrote through symlink: %v", entries)
	}
}

func TestCodexSimultaneousFirstLaunches(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	run := codexRun(t, "p\n")
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = codex{}.Prepare(context.Background(), f.host(), run, nil)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "plugins", "cache", "tbox", "demo", "1.2.0", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCodexSweepsStaleStaging(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	stale := filepath.Join(home, "plugins", "cache", "tbox", ".plugin-123")
	writeFile(t, filepath.Join(stale, "x"), "x", 0o600)
	if _, err := (codex{}).Prepare(context.Background(), f.host(), codexRun(t, "p\n"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale staging kept: %v", err)
	}
}

func prepareCodexImages(t *testing.T, f *fakeHost, names ...string) {
	t.Helper()
	for _, n := range names {
		run := codexRun(t, "p\n")
		run.Name = n
		if _, err := (codex{}).Prepare(context.Background(), f.host(), run, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodexCleanOneImage(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	prepareCodexImages(t, f, "img", "other")
	if err := (codex{}).Clean(f.host(), "img"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"plugins/cache/tbox/img", "tbox-img.config.toml"} {
		if _, err := os.Lstat(filepath.Join(home, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s kept: %v", gone, err)
		}
	}
	for _, kept := range []string{"plugins/cache/tbox/other/1.2.0/.codex-plugin/plugin.json", "tbox-other.config.toml"} {
		if _, err := os.Stat(filepath.Join(home, kept)); err != nil {
			t.Fatalf("%s removed: %v", kept, err)
		}
	}
}

func TestCodexCleanAll(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	writeFile(t, filepath.Join(home, "config.toml"), "a = 1\n", 0o600)
	writeFile(t, filepath.Join(home, "work.config.toml"), "b = 2\n", 0o600)
	writeFile(t, filepath.Join(home, "plugins", "cache", "openai", "x", "f"), "keep", 0o600)
	prepareCodexImages(t, f, "img", "other")
	if err := (codex{}).Clean(f.host(), ""); err != nil {
		t.Fatal(err)
	}
	assertCodexHomeEntries(t, home, "config.toml", "plugins", "work.config.toml")
	if readFile(t, filepath.Join(home, "config.toml")) != "a = 1\n" || readFile(t, filepath.Join(home, "work.config.toml")) != "b = 2\n" {
		t.Fatal("user config changed")
	}
	if _, err := os.Lstat(filepath.Join(home, "plugins", "cache", "tbox")); !os.IsNotExist(err) {
		t.Fatalf("cache kept: %v", err)
	}
	if readFile(t, filepath.Join(home, "plugins", "cache", "openai", "x", "f")) != "keep" {
		t.Fatal("foreign plugin cache changed")
	}
}

func TestCodexCleanWithoutHomeIsNoop(t *testing.T) {
	if err := (codex{}).Clean((&fakeHost{}).host(), ""); err != nil {
		t.Fatal(err)
	}
}

// TestCodexLive runs the real codex against a temporary CODEX_HOME. It is
// opt-in: TBOX_LIVE_CODEX=1 go test ./internal/harness -run TestCodexLive.
func TestCodexLive(t *testing.T) {
	if os.Getenv("TBOX_LIVE_CODEX") != "1" {
		t.Skip("set TBOX_LIVE_CODEX=1 to run against the installed codex")
	}
	run := codexRun(t, "LIVE-MARKER \"quoted\" \\ back\n")
	home := t.TempDir()
	env := map[string]string{"CODEX_HOME": home, "HOME": os.Getenv("HOME")}
	host := Host{
		Getenv:   func(k string) string { return env[k] },
		LookPath: exec.LookPath,
		Output:   liveOutput(home),
	}
	launch, err := codex{}.Prepare(context.Background(), host, run, nil)
	if err != nil {
		t.Fatal(err)
	}
	args := append(append([]string(nil), launch.Argv[1:]...), "debug", "prompt-input", "hi")
	out, err := liveOutput(home)(context.Background(), nil, launch.Path, args...)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{"demo:alpha", "demo:beta", `LIVE-MARKER \"quoted\" \\ back`, "You are Codex"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("prompt input lacks %q", want)
		}
	}
}

func liveOutput(codexHome string) func(context.Context, []string, string, ...string) ([]byte, error) {
	return func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = append(append(os.Environ(), "CODEX_HOME="+codexHome), env...)
		return cmd.Output()
	}
}

func TestCodexCleanKeepsProfilesNotWrittenByTbox(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	prepareCodexImages(t, f, "img")
	writeFile(t, filepath.Join(home, "tbox-mine.config.toml"), "model = \"x\"\n", 0o600)
	writeFile(t, filepath.Join(home, "tbox-gone.tmp-123.config.toml"), "# Written by tbox for", 0o600)
	if err := (codex{}).Clean(f.host(), ""); err != nil {
		t.Fatal(err)
	}
	assertCodexHomeEntries(t, home, "plugins", "tbox-mine.config.toml")
	if len(f.notes) != 1 || !strings.Contains(f.notes[0], filepath.Join(home, "tbox-mine.config.toml")) {
		t.Fatalf("notes %q", f.notes)
	}
	f.notes = nil
	writeFile(t, filepath.Join(home, "tbox-img.config.toml"), "user = 1\n", 0o600)
	if err := (codex{}).Clean(f.host(), "img"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(home, "tbox-img.config.toml")) != "user = 1\n" || len(f.notes) != 1 {
		t.Fatalf("foreign profile of img: notes %q", f.notes)
	}
}

func TestCodexSweepsStaleTempProfiles(t *testing.T) {
	f, home := codexHost(t, "0.166.0")
	writeFile(t, filepath.Join(home, "tbox-old.tmp-1.config.toml"), "", 0o600)
	prepareCodexImages(t, f, "img")
	if _, err := os.Lstat(filepath.Join(home, "tbox-old.tmp-1.config.toml")); !os.IsNotExist(err) {
		t.Fatalf("prepare kept a stale temp profile: %v", err)
	}
	writeFile(t, filepath.Join(home, "tbox-img.tmp-2.config.toml"), "", 0o600)
	writeFile(t, filepath.Join(home, "tbox-other.tmp-3.config.toml"), "", 0o600)
	if err := (codex{}).Clean(f.host(), "img"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, "tbox-img.tmp-2.config.toml")); !os.IsNotExist(err) {
		t.Fatalf("clean kept the temp profile of img: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, "tbox-other.tmp-3.config.toml")); err != nil {
		t.Fatalf("clean removed the temp profile of another image: %v", err)
	}
}
