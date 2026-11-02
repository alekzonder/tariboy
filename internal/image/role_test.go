package image

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/rolefile"
)

const roleMarker = "ROLE-MARKER-7f3a"

func buildWithRole(t *testing.T, role []byte) (*Store, Ref, Manifest, error) {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("prompt body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if role != nil {
		if err := os.WriteFile(filepath.Join(source, rolefile.FileName), role, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := &Store{Dir: t.TempDir()}
	ref := Ref{Name: "reviewer", Tag: "latest"}
	src := &imagefile.V2{SchemaVersion: 2, Dir: source, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
	man, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, fixedClock(), nil)
	return store, ref, man, err
}

func TestBuildV2RecordsRoleInManifest(t *testing.T) {
	store, ref, man, err := buildWithRole(t, []byte("# Reviewer\n"+roleMarker+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := rolefile.Role{Text: "# Reviewer\n" + roleMarker + "\n"}
	if man.Role == nil || *man.Role != want {
		t.Fatalf("built role = %+v", man.Role)
	}
	archived, err := store.ReadFile(ref, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk struct {
		Role *rolefile.Role `json:"role"`
	}
	if err := json.Unmarshal(archived, &onDisk); err != nil || onDisk.Role == nil || *onDisk.Role != want {
		t.Fatalf("archived manifest.json role = %+v, %v\n%s", onDisk.Role, err, archived)
	}
	inspected, err := store.Inspect(ref)
	if err != nil || inspected.Role == nil || *inspected.Role != want {
		t.Fatalf("inspected role = %+v, %v", inspected.Role, err)
	}
}

func TestBuildV2WithoutRoleWritesNoRoleField(t *testing.T) {
	for name, role := range map[string][]byte{"missing": nil, "whitespace": []byte(" \n\t\n")} {
		store, ref, man, err := buildWithRole(t, role)
		if err != nil {
			t.Fatal(err)
		}
		if man.Role != nil {
			t.Fatalf("%s: role = %+v", name, man.Role)
		}
		archived, err := store.ReadFile(ref, "manifest.json")
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(archived, []byte(`"role"`)) {
			t.Fatalf("%s: manifest.json has role:\n%s", name, archived)
		}
		out, err := json.Marshal(man)
		if err != nil || bytes.Contains(out, []byte(`"role"`)) {
			t.Fatalf("%s: manifest JSON has role: %s", name, out)
		}
	}
}

func TestBuildV2RejectsInvalidUTF8Role(t *testing.T) {
	_, _, _, err := buildWithRole(t, []byte("bad \xff"))
	if err == nil || !strings.Contains(err.Error(), "ROLE.md is not valid UTF-8") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildV2RoleLimit(t *testing.T) {
	body := strings.Repeat("я", rolefile.LimitChars)
	if _, _, man, err := buildWithRole(t, []byte(body)); err != nil || man.Role == nil || man.Role.Text != body {
		t.Fatalf("500 characters: role = %+v, err = %v", man.Role, err)
	}
	_, _, _, err := buildWithRole(t, []byte(body+"я"))
	if err == nil || !strings.Contains(err.Error(), "501 characters") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("501 characters: err = %v", err)
	}
}

func TestRoleStaysOutOfPromptAndAgentDirectory(t *testing.T) {
	store, ref, _, err := buildWithRole(t, []byte(roleMarker))
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := store.RenderPrompt(ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, roleMarker) {
		t.Fatalf("prompt carries ROLE.md: %q", prompt)
	}
	files, err := store.ListFiles(ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Path, rolefile.FileName) {
			t.Fatalf("archive member %s", f.Path)
		}
	}
	dest := t.TempDir()
	if err := store.Unpack(ref, dest); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if d.Name() == "manifest.json" {
			return nil // the manifest carries the role as metadata
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(roleMarker)) {
			t.Errorf("unpacked %s carries ROLE.md", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPortableArchiveWithRoleInstallsOnAnotherStore(t *testing.T) {
	store, ref, man, err := buildWithRole(t, []byte(roleMarker))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePortableArchiveContract(archive, ref, nil); err != nil {
		t.Fatalf("contract validation: %v", err)
	}
	dest := &Store{Dir: t.TempDir()}
	installed, err := dest.InstallArchive(ref, archive)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Role == nil || *installed.Role != *man.Role {
		t.Fatalf("installed role = %+v", installed.Role)
	}
	inspected, err := dest.Inspect(ref)
	if err != nil || inspected.Role == nil || inspected.Role.Text != roleMarker {
		t.Fatalf("inspected role = %+v, %v", inspected.Role, err)
	}
}

func TestPortableArchiveRejectsInvalidRole(t *testing.T) {
	store, ref, _, err := buildWithRole(t, []byte(roleMarker))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := store.ArchiveBytes(ref)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ role, want string }{
		"whitespace text": {`{"text":" \n"}`, "role: text is empty"},
		"over limit":      {`{"text":"` + strings.Repeat("я", rolefile.LimitChars+1) + `"}`, "text is 501 characters"},
		"invalid utf8":    {"{\"text\":\"role \xff\"}", "role: manifest is not valid UTF-8"},
		"unknown field":   {`{"text":"role","extra":1}`, `unknown field "extra"`},
	}
	for name, tc := range cases {
		tampered := rewriteArchiveMember(t, archive, "manifest.json", func(body []byte) []byte {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			fields["role"] = json.RawMessage(tc.role)
			out, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			return out
		})
		_, err := ValidatePortableArchive(tampered, ref)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if name == "unknown field" {
			continue // the lenient check of pull and push ignores unknown fields
		}
		// Pull and the store's push use the lenient ValidateArchive.
		path := filepath.Join(t.TempDir(), "image.tar.gz")
		if err := os.WriteFile(path, tampered, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateArchive(path, ref); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: ValidateArchive err = %v, want %q", name, err, tc.want)
		}
	}
}
