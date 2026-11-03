package stores

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSkillHashSortsPathsLikeLocaleCompare pins the computedHash formula of
// the skills installer: a case-insensitive Unicode order puts references/a.md
// before Scripts/b.sh before SKILL.md, where a byte order would reverse them.
func TestSkillHashSortsPathsLikeLocaleCompare(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"SKILL.md":              "skill",
		"references/a.md":       "a",
		"Scripts/b.sh":          "b",
		".git/HEAD":             "ignored",
		"node_modules/x/y.js":   "ignored",
		"sub/node_modules/z.js": "ignored",
	})
	h := sha256.New()
	for _, pair := range [][2]string{{"references/a.md", "a"}, {"Scripts/b.sh", "b"}, {"SKILL.md", "skill"}} {
		h.Write([]byte(pair[0]))
		h.Write([]byte(pair[1]))
	}
	want := hex.EncodeToString(h.Sum(nil))
	got, err := SkillHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SkillHash() = %s, want %s", got, want)
	}
}

func lockJSON(t *testing.T, skills map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"version":1,"skills":{`)
	first := true
	for name, hash := range skills {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + name + `":{"source":"../../skills/` + name + `","sourceType":"local","computedHash":"` + hash + `"}`)
	}
	b.WriteString("}}\n")
	return b.String()
}

func TestLockCheckVerifiesInstalledSkills(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeTree(t, source, map[string]string{"skills/demo/SKILL.md": "fresh"})
	fresh, err := SkillHash(filepath.Join(source, "skills", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		installed map[string]string
		want      string
	}{
		{"match", map[string]string{".agents/skills/demo/SKILL.md": "fresh"}, ""},
		{"stale", map[string]string{".agents/skills/demo/SKILL.md": "stale"}, "skills-lock.json images/basic/skills-lock.json: skill demo: installed content sha256:"},
		{"missing", map[string]string{}, "skills-lock.json images/basic/skills-lock.json: skill demo: is not installed"},
		{"extra", map[string]string{".agents/skills/demo/SKILL.md": "fresh", ".agents/skills/other/SKILL.md": "x"}, "skills-lock.json images/basic/skills-lock.json: skill other: is installed but not in skills-lock.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := filepath.Join(root, "images", "basic")
			writeTree(t, dir, tc.installed)
			writeTree(t, dir, map[string]string{
				"skills-lock.json": lockJSON(t, map[string]string{"demo": fresh}),
				"Tariboyfile.yaml": "schema_version: 2\nplugins: []\nprompts: []\n",
			})
			check, err := NewLockCheck(root, dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := check.Record(dir); err != nil {
				t.Fatal(err)
			}
			err = check.Verify()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Verify() = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrLockMismatch) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Verify() = %v, want %q", err, tc.want)
			}
		})
	}
}
