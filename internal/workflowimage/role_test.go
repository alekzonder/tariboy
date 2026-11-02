package workflowimage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/rolefile"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

func sourceWithRole(t *testing.T, role string) *workflowfile.File {
	t.Helper()
	f := writeSource(t, "1.0.0")
	put(t, f.Dir, "ROLE.md", role, 0o644)
	return f
}

func TestPublishRecordsRole(t *testing.T) {
	s := newStore(t)
	m := publish(t, s, sourceWithRole(t, "# Workflow role\n"))
	want := rolefile.Role{Text: "# Workflow role\n", LimitBytes: rolefile.LimitBytes}
	if m.Role == nil || *m.Role != want {
		t.Fatalf("published role = %+v", m.Role)
	}
	got, err := s.Inspect("demo", "latest")
	if err != nil || got.Role == nil || *got.Role != want {
		t.Fatalf("inspected role = %+v, %v", got.Role, err)
	}
}

func TestPublishWithoutRoleHasNoRoleField(t *testing.T) {
	for name, f := range map[string]*workflowfile.File{
		"missing":    writeSource(t, "1.0.0"),
		"whitespace": sourceWithRole(t, "  \n\t\n"),
	} {
		s := newStore(t)
		m := publish(t, s, f)
		if m.Role != nil {
			t.Fatalf("%s: role = %+v", name, m.Role)
		}
		data, err := os.ReadFile(filepath.Join(s.ContentDir("demo", m.Digest), manifestName))
		if err != nil || strings.Contains(string(data), `"role"`) {
			t.Fatalf("%s: stored manifest has role: %s, %v", name, data, err)
		}
	}
}

func TestPublishRejectsInvalidUTF8Role(t *testing.T) {
	s := newStore(t)
	_, _, err := s.Publish(sourceWithRole(t, "bad \xff"), t0)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "ROLE.md is not valid UTF-8") {
		t.Fatalf("err = %v", err)
	}
	noRefs(t, s)
	if errs := Validate(sourceWithRole(t, "bad \xff")); len(errs) == 0 {
		t.Fatal("Validate accepted invalid UTF-8 ROLE.md")
	}
}

func TestPublishTruncatesLargeRole(t *testing.T) {
	m := publish(t, newStore(t), sourceWithRole(t, strings.Repeat("w", rolefile.LimitBytes+1)))
	if m.Role == nil || !m.Role.Truncated || len(m.Role.Text) != rolefile.LimitBytes {
		t.Fatalf("role = %+v", m.Role)
	}
}

// TestRoleDoesNotChangeDigestRule pins the digest of a source with ROLE.md
// to the documented file stream: ROLE.md is one more file, nothing else.
func TestRoleDoesNotChangeDigestRule(t *testing.T) {
	line := func(path, exec, content string) string {
		sum := sha256.Sum256([]byte(content))
		return path + "\x00" + exec + "\x00" + hex.EncodeToString(sum[:]) + "\n"
	}
	const role = "# Role\n"
	stream := line("ROLE.md", "0", role) + line("Workflowfile.yaml", "0", goldenManifest) + line("scripts/run.sh", "1", goldenScript)
	sum := sha256.Sum256([]byte(stream))
	dir := t.TempDir()
	put(t, dir, "Workflowfile.yaml", goldenManifest, 0o644)
	put(t, dir, "scripts/run.sh", goldenScript, 0o755)
	put(t, dir, "ROLE.md", role, 0o644)
	src, err := workflowfile.Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m := publish(t, newStore(t), src); m.Digest != hex.EncodeToString(sum[:]) || m.Role == nil {
		t.Fatalf("digest = %s role = %+v, want %x", m.Digest, m.Role, sum)
	}
}

func TestExportImportKeepsRole(t *testing.T) {
	src, _ := newRegistry(t)
	want, _, err := src.Publish(sourceWithRole(t, "# Role\n"), t0)
	if err != nil {
		t.Fatal(err)
	}
	archive := exportArchive(t, src, "demo", "1.0.0")
	dst, _ := newRegistry(t)
	got, _, err := importArchive(dst, archive, filepath.Join(t.TempDir(), "imports"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Role == nil || *got.Role != *want.Role || got.Digest != want.Digest {
		t.Fatalf("imported role = %+v digest %s", got.Role, got.Digest)
	}
}

// stripStoredRole turns a published image into one published before ROLE.md
// was read: the stored manifest and its row have no role.
func stripStoredRole(t *testing.T, r *Registry, m Manifest) {
	t.Helper()
	dir := r.Store.ContentDir(m.Name, m.Digest)
	path := filepath.Join(dir, manifestName)
	old := m
	old.Role = nil
	data, err := json.MarshalIndent(old, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.Exec(`UPDATE task_workflow_images SET manifest = ? WHERE digest = ?`, string(data), m.Digest); err != nil {
		t.Fatal(err)
	}
}

func TestRepublishAddsRoleToOldManifest(t *testing.T) {
	r, _ := newRegistry(t)
	src := sourceWithRole(t, "# Role\n")
	first, _, err := r.Publish(src, t0)
	if err != nil {
		t.Fatal(err)
	}
	stripStoredRole(t, r, first)
	if m, err := r.Store.Inspect("demo", "latest"); err != nil || m.Role != nil {
		t.Fatalf("setup: role = %+v, %v", m.Role, err)
	}

	again, created, err := r.Publish(src, t0.Add(1))
	if err != nil || created {
		t.Fatalf("republish created=%v err=%v", created, err)
	}
	if again.Digest != first.Digest || again.BuiltAt != first.BuiltAt || again.Role == nil || again.Role.Text != "# Role\n" {
		t.Fatalf("republished = %s %s role %+v", again.Digest, again.BuiltAt, again.Role)
	}
	stored, err := r.Store.Inspect("demo", "latest")
	if err != nil || stored.Role == nil || stored.Role.Text != "# Role\n" {
		t.Fatalf("stored role = %+v, %v", stored.Role, err)
	}
	row, err := r.Get(first.Digest)
	if err != nil || row.Role == nil || row.Role.Text != "# Role\n" {
		t.Fatalf("recorded role = %+v, %v", row.Role, err)
	}
	info, err := os.Stat(r.Store.ContentDir("demo", first.Digest))
	if err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("content dir mode = %v, %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(r.Store.ContentDir("demo", first.Digest), manifestName)); err != nil || info.Mode().Perm() != 0o400 {
		t.Fatalf("manifest mode = %v, %v", info, err)
	}
}

func TestReadOldManifestWithoutRole(t *testing.T) {
	r, _ := newRegistry(t)
	first, _, err := r.Publish(writeSource(t, "1.0.0"), t0)
	if err != nil {
		t.Fatal(err)
	}
	stripStoredRole(t, r, first)
	if m, err := r.Store.Inspect("demo", "1.0.0"); err != nil || m.Role != nil {
		t.Fatalf("inspect = %+v, %v", m.Role, err)
	}
	if m, err := r.Get(first.Digest); err != nil || m.Role != nil {
		t.Fatalf("get = %+v, %v", m.Role, err)
	}
}
