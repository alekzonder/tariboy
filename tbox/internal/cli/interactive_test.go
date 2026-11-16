package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/tui"
)

// fakeTerm replays keys and counts Close calls.
type fakeTerm struct {
	keys   []tui.Key
	out    bytes.Buffer
	closed int
}

func (f *fakeTerm) ReadKey() (tui.Key, error) {
	if len(f.keys) == 0 {
		return tui.Key{}, io.EOF
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, nil
}
func (f *fakeTerm) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f *fakeTerm) Size() (int, int)            { return 80, 24 }
func (f *fakeTerm) Close() error                { f.closed++; return nil }

var (
	kDown  = tui.Key{Code: tui.KeyDown}
	kEnter = tui.Key{Code: tui.KeyEnter}
	kSpace = tui.Key{Code: tui.KeySpace}
	kEsc   = tui.Key{Code: tui.KeyEsc}
)

func typed(s string) []tui.Key {
	var ks []tui.Key
	for _, r := range s {
		ks = append(ks, tui.Key{Code: tui.KeyRune, Rune: r})
	}
	return ks
}

func keys(parts ...[]tui.Key) []tui.Key {
	var out []tui.Key
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func one(k tui.Key) []tui.Key { return []tui.Key{k} }

type interactiveRun struct {
	code     int
	out, err string
	call     *execCall
	term     *fakeTerm
	opened   int
}

// runInteractive runs args with a fake terminal; term nil means no terminal.
// installed lists the harnesses LookPath finds.
func runInteractive(t *testing.T, home string, term *fakeTerm, installed []string, args ...string) interactiveRun {
	t.Helper()
	claude := fakeClaude(t)
	var r interactiveRun
	var out, errb bytes.Buffer
	env := Env{
		Args:    args,
		Getenv:  func(k string) string { return map[string]string{"TBOX_HOME": home}[k] },
		Environ: func() []string { return []string{"PATH=/bin"} },
		Stdout:  &out,
		Stderr:  &errb,
		Cwd:     "/",
		Now:     func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) },
		LookPath: func(n string) (string, error) {
			for _, h := range installed {
				if h == n {
					return claude, nil
				}
			}
			return "", errors.New("not installed")
		},
		Exec: func(path string, argv, e []string) error {
			r.call = &execCall{path, argv, e}
			return nil
		},
		OpenTerminal: func() (tui.Terminal, error) {
			r.opened++
			if term == nil {
				return nil, tui.ErrNoTerminal
			}
			return term, nil
		},
	}
	r.code = Main(env)
	r.out, r.err, r.term = out.String(), errb.String(), term
	if r.opened > 0 && term != nil && term.closed != 1 {
		t.Errorf("terminal closed %d times, want 1", term.closed)
	}
	return r
}

func twoSkillStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills/a/SKILL.md"), "---\nname: a\ndescription: Skill a\n---\nbody\n")
	writeFile(t, filepath.Join(dir, "skills/b/SKILL.md"), "---\nname: b\ndescription: Skill b\n---\nbody\n")
	writeFile(t, filepath.Join(dir, "prompts/p.md"), "prompt p\n")
	return dir
}

func TestInteractiveImageNew(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	if code, _, e := runIn(t, home, "/", "store", "add", "s", twoSkillStore(t)); code != 0 {
		t.Fatal(e)
	}
	term := &fakeTerm{keys: keys(typed("demo"), one(kEnter),
		one(kSpace), one(kDown), one(kSpace), one(kEnter), // skills a and b
		one(kSpace), one(kEnter))} // new empty prompt
	r := runInteractive(t, home, term, nil, "image", "new")
	if r.code != 0 {
		t.Fatalf("%d %q %q", r.code, r.out, r.err)
	}
	src := filepath.Join(home, "sources", "demo")
	if _, err := os.Stat(filepath.Join(src, "instructions.md")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.out, "built demo:0.1.0 ") {
		t.Fatalf("out %q", r.out)
	}
	_, show, _ := runIn(t, home, "/", "image", "show", "demo")
	if !strings.Contains(show, "a: Skill a") || !strings.Contains(show, "b: Skill b") {
		t.Fatalf("show %q", show)
	}
	if strings.Contains(r.term.out.String(), "built demo") {
		t.Fatal("build output went to the terminal")
	}
}

func TestInteractiveImageNewNameGiven(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	runIn(t, home, "/", "store", "add", "s", twoSkillStore(t))
	term := &fakeTerm{keys: keys(one(kSpace), one(kEnter), one(kDown), one(kSpace), one(kEnter))} // skill a, prompt s/p
	r := runInteractive(t, home, term, nil, "image", "new", "demo", "--no-build")
	if r.code != 0 || !strings.HasPrefix(r.out, "created ") {
		t.Fatalf("%d %q %q", r.code, r.out, r.err)
	}
}

func TestInteractiveFlagsNeverOpenTerminal(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	runIn(t, home, "/", "store", "add", "s", twoSkillStore(t))
	r := runInteractive(t, home, &fakeTerm{}, nil, "image", "new", "demo", "--skill", "s/a")
	if r.code != 0 || r.opened != 0 {
		t.Fatalf("%d opened %d %q", r.code, r.opened, r.err)
	}
}

func TestInteractiveImageNewCancel(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	runIn(t, home, "/", "store", "add", "s", twoSkillStore(t))
	term := &fakeTerm{keys: keys(typed("demo"), one(kEnter), one(kEsc))}
	r := runInteractive(t, home, term, nil, "image", "new")
	if r.code != 130 {
		t.Fatalf("%d %q", r.code, r.err)
	}
	if es, _ := os.ReadDir(filepath.Join(home, "sources")); len(es) != 0 {
		t.Fatalf("sources not empty: %v", es)
	}
}

func TestInteractiveNoTerminal(t *testing.T) {
	home := t.TempDir()
	r := runInteractive(t, home, nil, nil, "run", "demo")
	if r.code != 2 || !strings.Contains(r.err, "tbox run: IMAGE and HARNESS are required when stdin is not a terminal") {
		t.Fatalf("run: %d %q", r.code, r.err)
	}
	r = runInteractive(t, home, nil, nil, "image", "new")
	if r.code != 2 || !strings.Contains(r.err, "--skill, --prompt or --new-prompt") {
		t.Fatalf("new: %d %q", r.code, r.err)
	}
}

func TestInteractiveNothingToChoose(t *testing.T) {
	home := t.TempDir()
	term := &fakeTerm{}
	r := runInteractive(t, home, term, nil, "image", "new")
	if r.code != 1 || !strings.Contains(r.err, "no skills or prompts found; add a Store with: tbox store add NAME SOURCE") {
		t.Fatalf("new: %d %q", r.code, r.err)
	}
	term = &fakeTerm{}
	r = runInteractive(t, home, term, nil, "run")
	if r.code != 1 || !strings.Contains(r.err, "tbox build") || !strings.Contains(r.err, "tbox store add") {
		t.Fatalf("run: %d %q", r.code, r.err)
	}
	src := demoSource(t)
	if c, _, e := runIn(t, home, "/", "build", "--path", src); c != 0 {
		t.Fatal(e)
	}
	term = &fakeTerm{keys: one(kEnter)}
	r = runInteractive(t, home, term, nil, "run", "demo")
	if r.code != 1 || !strings.Contains(r.err, "claude, codex, opencode") {
		t.Fatalf("harness: %d %q", r.code, r.err)
	}
}

func buildTwo(t *testing.T, home string) {
	t.Helper()
	for _, n := range []string{"alpha", "beta"} {
		src := demoSource(t)
		writeFile(t, filepath.Join(src, "Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\nskills:\n  - dir: ./skills/hello\n")
		if c, _, e := runIn(t, home, "/", "build", "--path", src, "--name", n); c != 0 {
			t.Fatal(e)
		}
	}
}

func TestInteractiveRunPicksImageAndOnlyHarness(t *testing.T) {
	home := t.TempDir()
	buildTwo(t, home)
	term := &fakeTerm{keys: keys(one(kDown), one(kDown), one(kDown), one(kEnter))} // beta:latest
	r := runInteractive(t, home, term, []string{"claude"}, "run", "--", "--model", "x")
	if r.call == nil {
		t.Fatalf("no exec: %d %q", r.code, r.err)
	}
	found := false
	for _, kv := range r.call.env {
		if kv == "TBOX_IMAGE=beta:latest" {
			found = true
		}
	}
	if !found {
		t.Fatalf("env %v\n%q", r.call.env, term.out.String())
	}
	if !strings.Contains(r.err, "claude") || !strings.Contains(r.err, "harness") {
		t.Fatalf("no harness line: %q", r.err)
	}
	if n := len(r.call.argv); n < 2 || r.call.argv[n-2] != "--model" || r.call.argv[n-1] != "x" {
		t.Fatalf("argv %v", r.call.argv)
	}
}

func TestInteractiveRunOneArgAsksHarness(t *testing.T) {
	home := t.TempDir()
	buildTwo(t, home)
	term := &fakeTerm{keys: keys(one(kDown), one(kEnter))}
	r := runInteractive(t, home, term, []string{"claude", "codex"}, "run", "alpha")
	// second installed harness is codex; its adapter may reject the fake
	// binary, so only the image and the single list matter here.
	if len(term.keys) != 0 {
		t.Fatalf("keys left: %v", term.keys)
	}
	if strings.Contains(term.out.String(), "alpha") && strings.Contains(term.out.String(), "beta") {
		t.Fatal("image list was shown")
	}
	_ = r
}

func TestInteractiveRunCancel(t *testing.T) {
	home := t.TempDir()
	buildTwo(t, home)
	r := runInteractive(t, home, &fakeTerm{keys: one(kEsc)}, []string{"claude"}, "run")
	if r.code != 130 || r.call != nil {
		t.Fatalf("%d %v", r.code, r.call)
	}
}

func TestInteractiveRunStoreImageRows(t *testing.T) {
	localRegistry(t)
	home := t.TempDir()
	store := t.TempDir()
	writeFile(t, filepath.Join(store, "images/img/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\n")
	if c, _, e := runIn(t, home, "/", "store", "add", "s", store); c != 0 {
		t.Fatal(e)
	}
	term := &fakeTerm{keys: one(kEsc)}
	r := runInteractive(t, home, term, []string{"claude"}, "run")
	if r.code != 130 {
		t.Fatalf("%d %q", r.code, r.err)
	}
	if got := term.out.String(); !strings.Contains(got, "s/img") || !strings.Contains(got, "not built") {
		t.Fatalf("list %q", got)
	}
}
