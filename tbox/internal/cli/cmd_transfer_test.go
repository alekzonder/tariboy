package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportImportCommands(t *testing.T) {
	home, src, work := t.TempDir(), demoSource(t), t.TempDir()
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("build: %s", errs)
	}

	code, out, errs := runIn(t, home, work, "image", "export", "demo:1.0.0")
	file := filepath.Join(work, "demo-1.0.0.tariboy-image.tar.gz")
	if code != 0 || errs != "" || !strings.Contains(out, file) {
		t.Fatalf("export %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	code, _, errs = runIn(t, home, work, "image", "export", "demo", "-o", "named.tar.gz")
	if code != 0 {
		t.Fatalf("export -o: %s", errs)
	}
	if _, err := os.Stat(filepath.Join(work, "named.tar.gz")); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	code, out, errs = runIn(t, other, work, "image", "import", "demo-1.0.0.tariboy-image.tar.gz")
	if code != 0 || !strings.HasPrefix(out, "imported demo:1.0.0 ") {
		t.Fatalf("import %d %q %q", code, out, errs)
	}
	code, out, _ = runIn(t, other, "/", "image", "show", "demo:1.0.0")
	if code != 0 || !strings.Contains(out, "hello: Says hello") {
		t.Fatalf("show %d %q", code, out)
	}
}

func TestTransferUsageAndErrors(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	for _, args := range [][]string{{"image", "export"}, {"image", "import"}, {"image", "export", "a", "b"}} {
		if code, _, _ := runIn(t, home, work, args...); code != 2 {
			t.Fatalf("%v code %d, want 2", args, code)
		}
	}
	if code, _, errs := runIn(t, home, work, "image", "export", "missing"); code != 1 || errs == "" {
		t.Fatalf("missing image: %d %q", code, errs)
	}
	writeFile(t, filepath.Join(work, "bad.tar.gz"), "not gzip")
	if code, _, errs := runIn(t, home, work, "image", "import", "bad.tar.gz"); code != 1 || !strings.Contains(errs, "gzip") {
		t.Fatalf("bad import: %d %q", code, errs)
	}
	if code, _, _ := runIn(t, home, "/", "image", "bogus"); code != 2 {
		t.Fatalf("delegated usage code %d", code)
	}
}

func TestExportRefusesOverwrite(t *testing.T) {
	home, src, work := t.TempDir(), demoSource(t), t.TempDir()
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	file := filepath.Join(work, "out.tar.gz")
	writeFile(t, file, "keep")
	code, _, errs := runIn(t, home, work, "image", "export", "demo", "-o", "out.tar.gz")
	if code != 1 || !strings.Contains(errs, file) {
		t.Fatalf("export over existing: %d %q", code, errs)
	}
	if b, _ := os.ReadFile(file); string(b) != "keep" {
		t.Fatalf("file was overwritten: %q", b)
	}
	if code, _, errs := runIn(t, home, work, "image", "export", "demo", "-o", "out.tar.gz", "--force"); code != 0 {
		t.Fatalf("export --force: %d %q", code, errs)
	}
	if b, _ := os.ReadFile(file); string(b) == "keep" {
		t.Fatal("--force did not overwrite")
	}
}

func TestImportRefusesToMoveATagWithoutForce(t *testing.T) {
	home, src, work := t.TempDir(), demoSource(t), t.TempDir()
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("build: %s", errs)
	}
	if code, _, errs := runIn(t, home, work, "image", "export", "demo:1.0.0", "-o", "one.tar.gz"); code != 0 {
		t.Fatalf("export: %s", errs)
	}
	writeFile(t, filepath.Join(src, "p.md"), "changed prompt\n")
	if code, _, errs := runIn(t, home, "/", "build", "--path", src); code != 0 {
		t.Fatalf("rebuild: %s", errs)
	}
	if code, _, errs := runIn(t, home, work, "image", "export", "demo:1.0.0", "-o", "two.tar.gz"); code != 0 {
		t.Fatalf("export: %s", errs)
	}

	other := t.TempDir()
	if code, _, errs := runIn(t, other, work, "image", "import", "one.tar.gz"); code != 0 {
		t.Fatalf("import one: %s", errs)
	}
	_, before, _ := runIn(t, other, "/", "image", "show", "demo:1.0.0")
	code, _, errs := runIn(t, other, work, "image", "import", "two.tar.gz")
	if code != 1 || !strings.Contains(errs, "demo:1.0.0") || !strings.Contains(errs, "--force") {
		t.Fatalf("import over other content: %d %q", code, errs)
	}
	if _, after, _ := runIn(t, other, "/", "image", "show", "demo:1.0.0"); after != before {
		t.Fatalf("tag moved without --force:\n%s\n%s", before, after)
	}
	if code, _, errs := runIn(t, other, work, "image", "import", "one.tar.gz"); code != 0 {
		t.Fatalf("re-import of the same content: %d %q", code, errs)
	}
	code, out, errs := runIn(t, other, work, "image", "import", "two.tar.gz", "--force")
	if code != 0 || !strings.Contains(out, "moved 1.0.0 to new content") {
		t.Fatalf("import --force: %d %q %q", code, out, errs)
	}
}
