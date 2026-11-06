package imagefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repeatedEntries(key, entry string, count int, last string) string {
	var b strings.Builder
	b.WriteString(key + ":\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "  - %s\n", fmt.Sprintf(entry, i))
	}
	if last != "" {
		b.WriteString("  - " + last + "\n")
	}
	return b.String()
}

// TestParseV2ReportsTheFirstFailingCheck pins the text of each validation error
// and the order of the checks: a source that breaks two rules reports the
// earlier one.
func TestParseV2ReportsTheFirstFailingCheck(t *testing.T) {
	const head = "schema_version: 2\n"
	tests := []struct {
		name string
		body string
		want string
	}{
		{"v1 wins over unknown fields", "schema_version: 1\nmystery: true\n", SchemaV1MigrationMessage},
		{"version before schema", "schema_version: 3\nimage_version: 1\n", "image_version must be a SemVer string"},
		{"schema", "schema_version: 3\n", "imagefile schema_version must be 2, got 3"},
		{"schema before extends", "schema_version: 3\nextends: [bad]\n", "imagefile schema_version must be 2, got 3"},
		{"extends", head + "extends: [./a, bad]\n", `extends[1]: path "bad" must use ./, ../, or an absolute path`},
		{"extends before plugins", head + "extends: [bad]\nplugins: [{name: ''}]\n", `extends[0]: path "bad" must use ./, ../, or an absolute path`},
		{"plugin name", head + "plugins: [{name: a}, {name: ''}]\n", "plugins[1]: name is required"},
		{"plugin duplicate", head + "plugins: [{name: a}, {name: b}, {name: a}]\n", `plugins[2]: duplicate plugin "a"`},
		{"plugins before skills", head + "plugins: [{name: ''}]\nskills: [{dir: ''}]\n", "plugins[0]: name is required"},
		{"skill dir", head + "skills: [{dir: ./a}, {dir: ''}]\n", "skills[1]: dir is required"},
		{"skill count", head + repeatedEntries("skills", "{dir: ./s%d}", maxV2SkillEntries+1, ""), "skills has 129 entries, maximum is 128"},
		{"skill count before dir", head + repeatedEntries("skills", "{dir: ./s%d}", maxV2SkillEntries, "{dir: ''}"), "skills has 129 entries, maximum is 128"},
		{"skills before prompts", head + "skills: [{dir: ''}]\nprompts: [{}]\n", "skills[0]: dir is required"},
		{"prompt count", head + repeatedEntries("prompts", "{file: ./p%d}", maxV2PromptEntries+1, ""), "prompts has 1025 entries, maximum is 1024"},
		{"prompt count before entries", head + repeatedEntries("prompts", "{file: ./p%d}", maxV2PromptEntries, "{}"), "prompts has 1025 entries, maximum is 1024"},
		{"prompt neither", head + "prompts: [{file: ./a}, {}]\n", "prompts[1]: exactly one of file or runtime is required"},
		{"prompt both", head + "prompts: [{file: ./a, runtime: goal}]\n", "prompts[0]: exactly one of file or runtime is required"},
		{"prompt runtime", head + "prompts: [{runtime: foreign}]\n", `prompts[0]: unknown runtime placeholder "foreign"`},
		{"prompt runtime duplicate", head + "prompts: [{runtime: goal}, {file: ./a}, {runtime: goal}]\n", `prompts[2]: duplicate runtime placeholder "goal"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseV2(writeV2Source(t, tc.body))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("ParseV2 error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseV2DecodeErrorsNameTheFile(t *testing.T) {
	dir := writeV2Source(t, "schema_version: 2\n---\nschema_version: 2\n")
	path := filepath.Join(dir, DefaultFilename)
	if _, err := ParseV2(dir); err == nil || err.Error() != "parse "+path+": multiple YAML documents are not allowed" {
		t.Fatalf("multiple documents error = %v", err)
	}
	dir = writeV2Source(t, "schema_version: 2\nmystery: true\n")
	path = filepath.Join(dir, DefaultFilename)
	if _, err := ParseV2(dir); err == nil || !strings.HasPrefix(err.Error(), "parse "+path+": ") {
		t.Fatalf("unknown field error = %v", err)
	}
	dir = writeV2Source(t, "schema_version: 2\n---\n: : :\n")
	path = filepath.Join(dir, DefaultFilename)
	if _, err := ParseV2(dir); err == nil || !strings.HasPrefix(err.Error(), "parse "+path+": ") || strings.Contains(err.Error(), "multiple YAML") {
		t.Fatalf("broken second document error = %v", err)
	}
}

func TestParseV2SetsDirFromTheSourceFile(t *testing.T) {
	dir := writeV2Source(t, "schema_version: 2\n")
	for _, input := range []string{dir, filepath.Join(dir, DefaultFilename)} {
		got, err := ParseV2(input)
		if err != nil {
			t.Fatal(err)
		}
		if got.Dir != dir {
			t.Fatalf("ParseV2(%s).Dir = %q, want %q", input, got.Dir, dir)
		}
	}
}

func TestParseV2ReturnsFilesystemErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := ParseV2(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing path error = %v", err)
	}
	if _, err := ParseV2(t.TempDir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory without source error = %v", err)
	}
}
