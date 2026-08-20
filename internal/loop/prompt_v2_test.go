package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/image"
)

func imageSHA(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func promptTemplateSHA(t *testing.T, template image.PromptTemplate) string {
	t.Helper()
	got, err := image.PromptTemplateHash(template.Entries)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRenderPromptTemplateUsesDeclaredOrderOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompt", "layers"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"000-a.md": "A\n", "002-b.md": "B"} {
		if err := os.WriteFile(filepath.Join(dir, "prompt", "layers", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	template := image.PromptTemplate{SchemaVersion: 2, Entries: []image.TemplateEntry{
		{Kind: "file", ArchivePath: "prompt/layers/000-a.md", Size: 2, SHA256: imageSHA("A\n")}, {Kind: "runtime", Runtime: "identity"},
		{Kind: "file", ArchivePath: "prompt/layers/002-b.md", Size: 1, SHA256: imageSHA("B")}, {Kind: "runtime", Runtime: "messages"},
		{Kind: "runtime", Runtime: "context"}, {Kind: "runtime", Runtime: "workdir"}, {Kind: "runtime", Runtime: "one-shot"},
	}}
	template.SHA256 = promptTemplateSHA(t, template)
	workdir, err := FormatRuntimeWorkdir("/var/lib/tariboy/agents/worker/workdir")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderPromptTemplate(template, dir, RuntimePromptValues{
		Identity: "ID", Messages: "", Context: "CTX\n",
		Workdir: workdir, OneShot: "RUN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "A\n\nID\n\nB\n\nCTX\n\nworkdir: /var/lib/tariboy/agents/worker/workdir\n\nRUN\n"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestRenderPromptTemplateEmptyHasNoImplicitHeaderOrTail(t *testing.T) {
	template := image.PromptTemplate{SchemaVersion: 2, Entries: []image.TemplateEntry{}}
	template.SHA256 = promptTemplateSHA(t, template)
	got, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{})
	if err != nil || got != "" {
		t.Fatalf("prompt = %q, %v", got, err)
	}
}

func TestValidatePromptTemplateAcceptsWorkdirRuntime(t *testing.T) {
	template := image.PromptTemplate{SchemaVersion: 2, Entries: []image.TemplateEntry{{Kind: "runtime", Runtime: "workdir"}}}
	template.SHA256 = promptTemplateSHA(t, template)
	if err := image.ValidatePromptTemplate(template); err != nil {
		t.Fatalf("ValidatePromptTemplate(workdir): %v", err)
	}
}

func TestRenderPromptTemplateRejectsUnsafeLayerPath(t *testing.T) {
	template := image.PromptTemplate{SchemaVersion: 2, Entries: []image.TemplateEntry{{Kind: "file", ArchivePath: "../secret", SHA256: strings.Repeat("0", 64)}}}
	template.SHA256 = promptTemplateSHA(t, template)
	if _, err := RenderPromptTemplate(template, t.TempDir(), RuntimePromptValues{}); err == nil {
		t.Fatal("unsafe layer accepted")
	}
}

func TestFormatRuntimeIdentityUsesCurrentIterationImageAndCWD(t *testing.T) {
	want := "# You are agent worker\nimage: reviewer:v3\nimage-digest: sha256:abc\ncwd: /srv/work\niteration: worker-1"
	if got := FormatRuntimeIdentity("worker", "reviewer:v3", "sha256:abc", "/srv/work", "worker-1"); got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestFormatRuntimeWorkdir(t *testing.T) {
	path := filepath.Join("agents", "worker", "workdir")
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FormatRuntimeWorkdir(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "workdir: " + absolute; got != want {
		t.Fatalf("FormatRuntimeWorkdir() = %q, want %q", got, want)
	}
}

func TestReadPromptTemplateRequiresTrustedIterationDigest(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "prompt"), 0o700); err != nil {
		t.Fatal(err)
	}
	entries := []image.TemplateEntry{{Kind: "runtime", Runtime: "identity"}}
	sha, err := image.PromptTemplateHash(entries)
	if err != nil {
		t.Fatal(err)
	}
	template := image.PromptTemplate{SchemaVersion: 2, Entries: entries, SHA256: sha}
	body, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt", "template.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPromptTemplate(dir, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "trusted iteration") {
		t.Fatalf("accepted locally rewritten template identity: %v", err)
	}
	if got, err := ReadPromptTemplate(dir, sha); err != nil || got.SHA256 != sha {
		t.Fatalf("trusted template = %#v, %v", got, err)
	}
}
