package portable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/rundir"
)

// TestImportTariboyExport imports the envelope tariboy's own Export wrote
// (TestTboxTariboyEnvelope in internal/imageportable regenerates it): a
// built-in plugin, its packaged skill, and runtime prompt entries around a
// file prompt. The run directory's prompt holds the file prompt only.
func TestImportTariboyExport(t *testing.T) {
	f, err := os.Open(filepath.Join(compatDir, "v2-tariboy.tariboy-image.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, ref, err := Import(f)
	if err != nil {
		t.Fatal(err)
	}
	if ref.String() != "compat-tariboy:latest" || len(img.Manifest.Plugins) != 1 || img.Manifest.Plugins[0].Name != "whoami" {
		t.Fatalf("ref %s plugins %+v", ref, img.Manifest.Plugins)
	}
	kinds := []string{}
	for _, e := range img.Template.Entries {
		kinds = append(kinds, e.Kind+":"+e.Runtime)
	}
	if strings.Join(kinds, ",") != "runtime:identity,file:,runtime:user-prompt" {
		t.Fatalf("template entries %v", kinds)
	}
	inner := innerArchive(t, filepath.Join(compatDir, "v2-tariboy.tariboy-image.tar.gz"))
	again, err := archive.Read(strings.NewReader(string(inner)))
	if err != nil || again.Manifest.Digest != img.Manifest.Digest {
		t.Fatalf("archive.Read of the inner archive: %v", err)
	}
	run, err := rundir.Prepare(paths.Home(t.TempDir()), img, ref, "claude")
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(run.PromptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "You are the compat fixture built by tariboy.\n\n" {
		t.Fatalf("PROMPT.md = %q", prompt)
	}
}

// innerArchive returns image.tar.gz of the envelope at path.
func innerArchive(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	inner, ok := unpack(t, data)[imageFile]
	if !ok {
		t.Fatal("envelope has no image.tar.gz")
	}
	return inner
}
