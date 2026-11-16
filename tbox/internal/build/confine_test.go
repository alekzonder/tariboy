package build

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/skillslock"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

// storeImage is a Store root with the image images/img and a directory
// outside the Store that holds a skill and a prompt.
type storeImage struct {
	root, img, outside string
}

func newStoreImage(t *testing.T) storeImage {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "store")
	img := filepath.Join(root, "images", "img")
	outside := filepath.Join(base, "outside")
	write(t, filepath.Join(root, "skills/inside/SKILL.md"), skillBody("inside", "inside"))
	write(t, filepath.Join(outside, "secret/SKILL.md"), skillBody("secret", "secret"))
	write(t, filepath.Join(outside, "notes.md"), "secret notes\n")
	write(t, filepath.Join(outside, "Tariboyfile.yaml"), "schema_version: 3\nprompts:\n  - file: ./notes.md\n")
	return storeImage{root: root, img: img, outside: outside}
}

func (s storeImage) build(t *testing.T, tfileBody string, store bool) error {
	t.Helper()
	write(t, filepath.Join(s.img, "Tariboyfile.yaml"), tfileBody)
	age(t, filepath.Dir(s.root))
	o := opts(s.img, imgstore.Store{Dir: t.TempDir()})
	if store {
		o.StoreRoot = s.root
	}
	_, err := Build(o)
	return err
}

func TestStoreBuildIsConfinedToStore(t *testing.T) {
	s := newStoreImage(t)
	cases := map[string]string{
		"absolute prompt":    "schema_version: 3\nprompts:\n  - file: " + filepath.Join(s.outside, "notes.md") + "\n",
		"absolute skill dir": "schema_version: 3\nskills:\n  - dir: " + filepath.Join(s.outside, "secret") + "\n",
		"relative skill dir": "schema_version: 3\nskills:\n  - dir: ../../../outside/secret\n",
		"extends outside":    "schema_version: 3\nextends:\n  - ../../../outside\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			err := s.build(t, body, true)
			if err == nil || !strings.Contains(err.Error(), "outside the Store") {
				t.Fatalf("Store build: %v", err)
			}
			if err := s.build(t, body, false); err != nil {
				t.Fatalf("--path build: %v", err)
			}
		})
	}
}

func TestStoreBuildConfinesLocalLockSources(t *testing.T) {
	s := newStoreImage(t)
	body := "schema_version: 2\nskills:\n  - dir: ./.agents/skills/secret\n"
	for name, src := range map[string]string{
		"absolute": filepath.Join(s.outside, "secret"),
		"relative": "../../../outside/secret",
	} {
		t.Run(name, func(t *testing.T) {
			write(t, filepath.Join(s.img, skillslock.Filename),
				`{"version":1,"skills":{"secret":{"source":"`+src+`","sourceType":"local","computedHash":"x"}}}`)
			err := s.build(t, body, true)
			if err == nil || !strings.Contains(err.Error(), "outside the Store") {
				t.Fatalf("Store build: %v", err)
			}
			if err := s.build(t, body, false); err != nil {
				t.Fatalf("--path build: %v", err)
			}
		})
	}
}

func TestStoreBuildInsideStoreStillBuilds(t *testing.T) {
	s := newStoreImage(t)
	write(t, filepath.Join(s.img, skillslock.Filename),
		`{"version":1,"skills":{"inside":{"source":"../../skills/inside","sourceType":"local","computedHash":"x"}}}`)
	write(t, filepath.Join(s.root, "prompts/p.md"), "store prompt\n")
	write(t, filepath.Join(s.root, "base/Tariboyfile.yaml"), "schema_version: 3\nskills:\n  - dir: ../skills/inside\n")
	body := "schema_version: 2\nextends:\n  - ../../base\nskills:\n  - dir: ./.agents/skills/inside\nprompts:\n  - file: " +
		filepath.Join(s.root, "prompts/p.md") + "\n"
	if err := s.build(t, body, true); err != nil {
		t.Fatal(err)
	}
}

func TestLabelsHideAddressCredentials(t *testing.T) {
	for _, got := range []string{
		skillLabel(tfile.SkillEntry{Git: "https://alice:s3cr3t@host/x.git", Path: "a"}),
		skillLabel(tfile.SkillEntry{URL: "https://host/a.tar.gz?t=s3cr3t"}),
		promptLabel(tfile.PromptEntry{Git: "https://alice:s3cr3t@host/x.git", Path: "a.md"}),
		promptLabel(tfile.PromptEntry{URL: "https://alice:s3cr3t@host/a.md"}),
	} {
		if strings.Contains(got, "s3cr3t") {
			t.Errorf("label %q carries the secret", got)
		}
	}
}
