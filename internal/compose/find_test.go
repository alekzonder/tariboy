package compose

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNamedFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindFilePrefersTheNewName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := writeNamedFile(t, dir, "Tariboycompose.yaml", goodYAML)
	got, err := FindFile(dir)
	if err != nil || got != want {
		t.Fatalf("FindFile = %q, %v; want %q", got, err, want)
	}
}

func TestFindFileFallsBackToTheLegacyName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := writeNamedFile(t, dir, "tariboy-compose.yaml", goodYAML)
	got, err := FindFile(dir)
	if err != nil || got != want {
		t.Fatalf("FindFile = %q, %v; want %q", got, err, want)
	}
}

func TestFindFileRefusesBothNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeNamedFile(t, dir, "Tariboycompose.yaml", goodYAML)
	writeNamedFile(t, dir, "tariboy-compose.yaml", goodYAML)
	_, err := FindFile(dir)
	if err == nil || !strings.Contains(err.Error(), "Tariboycompose.yaml") || !strings.Contains(err.Error(), "tariboy-compose.yaml") {
		t.Fatalf("FindFile error = %v; want one naming both files", err)
	}
}

func TestFindFileNamesBothWhenNoneExists(t *testing.T) {
	t.Parallel()
	_, err := FindFile(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "Tariboycompose.yaml") || !strings.Contains(err.Error(), "tariboy-compose.yaml") {
		t.Fatalf("FindFile error = %v; want one naming both files", err)
	}
}

// TestRunFindsTheFileInTheWorkingDirectory pins that a verb without -f reads
// the file the lookup rule picks, and refuses a directory with both names.
func TestRunFindsTheFileInTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeNamedFile(t, dir, "Tariboycompose.yaml", goodYAML)
	fc := newFake()
	var errOut strings.Builder
	if code := Run(context.Background(), fc, Options{Verb: "ps"}, io.Discard, &errOut); code != 0 {
		t.Fatalf("ps code=%d stderr=%s", code, errOut.String())
	}
	writeNamedFile(t, dir, "tariboy-compose.yaml", goodYAML)
	fc = newFake()
	errOut.Reset()
	if code := Run(context.Background(), fc, Options{Verb: "ps"}, io.Discard, &errOut); code == 0 || !strings.Contains(errOut.String(), "both Tariboycompose.yaml and tariboy-compose.yaml") {
		t.Fatalf("ps with both files code=%d stderr=%s", code, errOut.String())
	}
	if len(fc.calls) != 0 {
		t.Fatalf("ps called the daemon without a file: %v", fc.calls)
	}
}

// TestRunWithFileFlagSkipsLookup pins that -f reads the named file even when
// the working directory holds both default names.
func TestRunWithFileFlagSkipsLookup(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeNamedFile(t, dir, "Tariboycompose.yaml", goodYAML)
	writeNamedFile(t, dir, "tariboy-compose.yaml", goodYAML)
	path := writeNamedFile(t, t.TempDir(), "team.yaml", goodYAML)
	var errOut strings.Builder
	if code := Run(context.Background(), newFake(), Options{Verb: "ps", File: path}, io.Discard, &errOut); code != 0 {
		t.Fatalf("ps -f code=%d stderr=%s", code, errOut.String())
	}
}
