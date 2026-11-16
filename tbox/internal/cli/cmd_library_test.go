package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// twoStores registers two dir Stores that both hold skill x and a prompt p.
func twoStores(t *testing.T) (home, a, b string) {
	t.Helper()
	localRegistry(t)
	home, a, b = t.TempDir(), t.TempDir(), t.TempDir()
	for i, name := range []string{"alpha", "beta"} {
		dir := []string{a, b}[i]
		writeFile(t, filepath.Join(dir, "skills/x/SKILL.md"), "---\nname: x\ndescription: Skill x of "+name+"\n---\nbody of "+name+"\n")
		writeFile(t, filepath.Join(dir, "skills/x/scripts/run.sh"), "echo\n")
		writeFile(t, filepath.Join(dir, "prompts/p.md"), "prompt of "+name+"\n")
		writeFile(t, filepath.Join(dir, "images/demo/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 1.0.0\nskills:\n  - dir: ../../skills/x\n")
		if code, _, errs := runIn(t, home, "/", "store", "add", name, dir); code != 0 {
			t.Fatalf("add %s: %q", name, errs)
		}
	}
	return home, a, b
}

func TestSkillsListsEveryStore(t *testing.T) {
	home, _, _ := twoStores(t)
	code, out, errs := runIn(t, home, "/", "skills")
	want := "alpha/x\tSkill x of alpha\nbeta/x\tSkill x of beta\n"
	if code != 0 || out != want || errs != "" {
		t.Fatalf("%d %q %q", code, out, errs)
	}
}

func TestSkillShowsDescriptionFilesAndBody(t *testing.T) {
	home, _, _ := twoStores(t)
	code, out, errs := runIn(t, home, "/", "skill", "beta/x")
	want := "name: x\ndescription: Skill x of beta\nfiles:\n  SKILL.md\n  scripts/run.sh\n\n---\nname: x\ndescription: Skill x of beta\n---\nbody of beta\n"
	if code != 0 || out != want || errs != "" {
		t.Fatalf("%d %q %q", code, out, errs)
	}
}

func TestBareSkillNameListsCandidates(t *testing.T) {
	home, _, _ := twoStores(t)
	code, out, errs := runIn(t, home, "/", "skill", "x")
	if code != 1 || out != "" || !strings.Contains(errs, "alpha/x") || !strings.Contains(errs, "beta/x") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	if code, _, errs = runIn(t, home, "/", "skill", "gamma/x"); code != 1 || !strings.Contains(errs, "gamma") {
		t.Fatalf("unknown store %d %q", code, errs)
	}
	if code, _, errs = runIn(t, home, "/", "skill", "alpha/nope"); code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("unknown skill %d %q", code, errs)
	}
	if code, _, _ = runIn(t, home, "/", "skill"); code != 2 {
		t.Fatalf("no argument %d", code)
	}
}

func TestSkillDoesNotFollowSymlinksInFileList(t *testing.T) {
	home, a, _ := twoStores(t)
	if err := os.Symlink("/etc", filepath.Join(a, "skills/x/link")); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runIn(t, home, "/", "skill", "alpha/x")
	if code != 0 || strings.Contains(out, "link") || strings.Contains(out, "passwd") {
		t.Fatalf("%d %q", code, out)
	}
}

func TestPromptsAndPrompt(t *testing.T) {
	home, _, _ := twoStores(t)
	code, out, errs := runIn(t, home, "/", "prompts")
	if code != 0 || out != "alpha/p\nbeta/p\n" || errs != "" {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	code, out, errs = runIn(t, home, "/", "prompt", "beta/p")
	if code != 0 || out != "prompt of beta\n" || errs != "" {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	if code, _, errs = runIn(t, home, "/", "prompt", "p"); code != 1 || !strings.Contains(errs, "alpha/p") {
		t.Fatalf("bare %d %q", code, errs)
	}
	if code, _, _ = runIn(t, home, "/", "prompt", "beta/none"); code != 1 {
		t.Fatalf("missing %d", code)
	}
}

func TestPromptRefusesSymlinkAndLargeFile(t *testing.T) {
	home, a, _ := twoStores(t)
	if err := os.Symlink("/etc/hostname", filepath.Join(a, "prompts/link.md")); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runIn(t, home, "/", "prompt", "alpha/link"); code != 1 {
		t.Fatalf("symlink %d", code)
	}
	writeFile(t, filepath.Join(a, "prompts/big.md"), strings.Repeat("x", 4<<20+1))
	if code, _, errs := runIn(t, home, "/", "prompt", "alpha/big"); code != 1 || !strings.Contains(errs, "exceeds") {
		t.Fatalf("big %d %q", code, errs)
	}
}

func TestLibraryCommandsSanitizeUntrustedText(t *testing.T) {
	localRegistry(t)
	home, dir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(dir, "skills/x/SKILL.md"), "---\nname: x\ndescription: \"a\\e[31mred\\nforged\"\n---\nb\n")
	writeFile(t, filepath.Join(dir, "prompts/p\x1b[2Jq.md"), "p")
	if code, _, errs := runIn(t, home, "/", "store", "add", "s", dir); code != 0 {
		t.Fatal(errs)
	}
	_, out, _ := runIn(t, home, "/", "skills")
	if strings.Contains(out, "\x1b") || strings.Count(out, "\n") != 1 {
		t.Fatalf("skills %q", out)
	}
	_, out, _ = runIn(t, home, "/", "prompts")
	if strings.Contains(out, "\x1b") || strings.Count(out, "\n") != 1 {
		t.Fatalf("prompts %q", out)
	}
	_, out, _ = runIn(t, home, "/", "skill", "s/x")
	if strings.Contains(strings.SplitN(out, "\n", 3)[1], "\x1b") {
		t.Fatalf("skill %q", out)
	}
}

func TestInsideSessionLibraryCommandsStayAgentMode(t *testing.T) {
	if _, ok := commands["skills"]; !ok {
		t.Fatal("skills not registered")
	}
	code, _, errs := run2(t, map[string]string{"TBOX_IMAGE_DIR": t.TempDir(), "TBOX_IMAGE": "demo:latest"}, "prompts")
	if code != 2 || !strings.Contains(errs, "not available inside an image session") {
		t.Fatalf("%d %q", code, errs)
	}
}

func TestImagesShowsStoreImageStatus(t *testing.T) {
	home, a, _ := twoStores(t)
	code, out, _ := runIn(t, home, "/", "images")
	if code != 0 || !strings.Contains(out, "STORE/IMAGE") || !strings.Contains(out, "alpha/demo") || strings.Count(out, "not built") != 2 {
		t.Fatalf("%d %q", code, out)
	}
	if code, _, errs := runIn(t, home, "/", "build", "alpha/demo"); code != 0 {
		t.Fatal(errs)
	}
	_, out, _ = runIn(t, home, "/", "images")
	if !strings.Contains(out, "NAME") || !regexpRow(out, `alpha/demo\s+1\.0\.0\s+built`) || !regexpRow(out, `beta/demo\s+1\.0\.0\s+built`) {
		t.Fatalf("after build %q", out)
	}
	writeFile(t, filepath.Join(a, "images/demo/Tariboyfile.yaml"), "schema_version: 3\nimage_version: 2.0.0\nskills:\n  - dir: ../../skills/x\n")
	writeFile(t, filepath.Join(a, "images/broken/Tariboyfile.yaml"), "schema_version: nine\n")
	_, out, _ = runIn(t, home, "/", "images")
	if !regexpRow(out, `alpha/demo\s+2\.0\.0\s+not built`) || !strings.Contains(out, "alpha/broken") || !strings.Contains(out, "error") {
		t.Fatalf("changed %q", out)
	}
}

func regexpRow(out, pattern string) bool { return regexp.MustCompile(pattern).MatchString(out) }

// run2 runs tbox with a custom environment.
func run2(t *testing.T, vars map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(Env{Args: args, Getenv: func(k string) string { return vars[k] }, Stdout: &out, Stderr: &errb, Cwd: "/", Now: time.Now})
	return code, out.String(), errb.String()
}

func TestBuildStoreImage(t *testing.T) {
	home, _, _ := twoStores(t)
	code, out, errs := runIn(t, home, "/", "build", "alpha/demo", "--tag", "t1")
	if code != 0 || errs != "" || !strings.Contains(out, "built demo:t1 ") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
	if code, out, _ = runIn(t, home, "/", "image", "show", "demo:t1"); code != 0 || !strings.Contains(out, "x: Skill x of alpha") {
		t.Fatalf("show %d %q", code, out)
	}
	code, out, _ = runIn(t, home, "/", "build", "alpha/demo", "--tag", "t1")
	if code != 0 || !strings.HasPrefix(out, "up to date demo ") {
		t.Fatalf("second %d %q", code, out)
	}
	if code, _, errs = runIn(t, home, "/", "build", "alpha/demo", "--name", "other"); code != 0 {
		t.Fatalf("--name %d %q", code, errs)
	}
	if _, out, _ = runIn(t, home, "/", "images"); !strings.Contains(out, "other") {
		t.Fatalf("images %q", out)
	}
}

func TestBuildSelectorErrors(t *testing.T) {
	home, _, _ := twoStores(t)
	cases := [][]string{
		{"build", "alpha/demo", "--path", "/tmp"},
		{"build", "alpha/nope"},
		{"build", "gamma/demo"},
		{"build", "alpha/.."},
		{"build", "alpha"},
		{"build", "alpha/demo", "extra"},
	}
	for _, c := range cases {
		if code, out, errs := runIn(t, home, "/", c...); code == 0 || out != "" || errs == "" {
			t.Errorf("%v -> %d %q %q", c, code, out, errs)
		}
	}
}

func TestBuildNotesWhenAnotherSourceTakesTheName(t *testing.T) {
	home, a, _ := twoStores(t)
	writeFile(t, filepath.Join(a, "images/demo/note.txt"), "x")
	if _, out, errs := runIn(t, home, "/", "build", "alpha/demo"); strings.Contains(out, "note:") || errs != "" {
		t.Fatalf("first %q %q", out, errs)
	}
	if _, out, _ := runIn(t, home, "/", "build", "alpha/demo"); strings.Contains(out, "note:") {
		t.Fatalf("same source noted %q", out)
	}
	// beta/demo holds different skill text, so its content differs.
	code, out, errs := runIn(t, home, "/", "build", "beta/demo")
	if code != 0 || errs != "" || !strings.Contains(out, "moved latest to new content") || !strings.Contains(out, "note: image demo now comes from beta/demo\n") {
		t.Fatalf("%d %q %q", code, out, errs)
	}
}

func TestRunStoreImageBuildsThenExecs(t *testing.T) {
	home, a, _ := twoStores(t)
	code, errs, call := runWithExec(t, home, "/", "run", "alpha/demo", "claude")
	if call == nil || code != 0 || !strings.Contains(errs, "built demo:latest ") {
		t.Fatalf("%d %q", code, errs)
	}
	var ok bool
	for _, kv := range call.env {
		ok = ok || kv == "TBOX_IMAGE=demo:latest"
	}
	if !ok {
		t.Fatalf("env %v", call.env)
	}
	_, errs, call = runWithExec(t, home, "/", "run", "alpha/demo", "claude")
	if call == nil || strings.Contains(errs, "built ") {
		t.Fatalf("second %q", errs)
	}
	writeFile(t, filepath.Join(a, "skills/x/SKILL.md"), "---\nname: x\ndescription: changed\n---\nnew\n")
	_, errs, call = runWithExec(t, home, "/", "run", "alpha/demo", "claude")
	if call == nil || !strings.Contains(errs, "built demo:latest ") || !strings.Contains(errs, "moved latest to new content") {
		t.Fatalf("rebuild %q", errs)
	}
	if code, errs, _ = runWithExec(t, home, "/", "run", "gamma/demo", "claude"); code != 1 || !strings.Contains(errs, "gamma") {
		t.Fatalf("unknown %d %q", code, errs)
	}
	if code, _, _ = runWithExec(t, home, "/", "run", "--path", a, "alpha/demo", "claude"); code == 0 {
		t.Fatal("--path with selector accepted")
	}
}

func TestRunWithNilExecReturnIsSuccess(t *testing.T) {
	home := t.TempDir()
	if code, _, e := runIn(t, home, "/", "build", "--path", demoSource(t)); code != 0 {
		t.Fatal(e)
	}
	code, errs, call := runWithExec(t, home, "/", "run", "demo", "claude")
	if call == nil || code != 0 || strings.Contains(errs, "%!w") {
		t.Fatalf("%d %q", code, errs)
	}
}
