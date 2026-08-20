package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeSkills(t *testing.T) {
	img := t.TempDir()
	base := t.TempDir()
	sub := filepath.Join(".claude", "skills")
	dst := filepath.Join(base, sub)

	// Pack two skills the way Unpack would have laid them under image/skills.
	mk := func(rel, body string) {
		p := filepath.Join(img, "skills", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join("greeter", "SKILL.md"), "# Greeter")
	mk(filepath.Join("greeter", "reference", "notes.md"), "notes")
	mk(filepath.Join("hello", "SKILL.md"), "# Hello")

	if err := MaterializeSkills(img, base, sub); err != nil {
		t.Fatalf("MaterializeSkills: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("greeter", "SKILL.md"),
		filepath.Join("greeter", "reference", "notes.md"),
		filepath.Join("hello", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("skill file %s not materialized: %v", rel, err)
		}
	}
}

func TestMaterializeSkillsNoSkillsDirIsNoop(t *testing.T) {
	if err := MaterializeSkills(t.TempDir(), t.TempDir(), "d"); err != nil {
		t.Fatalf("absent skills dir must be a no-op, got %v", err)
	}
}

// TestMaterializeSkillsDoesNotDerefSourceSymlink proves the local copyTree
// guard refuses a symlink in the SOURCE tree: its target content is NOT copied.
// The M15 source is a semi-untrusted image, so confinement must be local and
// not rely on Store.Unpack stripping symlinks.
func TestMaterializeSkillsDoesNotDerefSourceSymlink(t *testing.T) {
	img := t.TempDir()
	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	skill := filepath.Join(img, "skills", "greeter")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Greeter"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symlink inside the source skill tree pointing at an outside secret.
	if err := os.Symlink(secret, filepath.Join(skill, "leak.md")); err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()
	sub := filepath.Join(".claude", "skills")
	dst := filepath.Join(base, sub)
	if err := MaterializeSkills(img, base, sub); err != nil {
		t.Fatalf("MaterializeSkills: %v", err)
	}
	// The real file is copied.
	if _, err := os.Stat(filepath.Join(dst, "greeter", "SKILL.md")); err != nil {
		t.Fatalf("legit skill file not materialized: %v", err)
	}
	// The symlink target's content must NOT have been dereferenced/copied.
	leaked := filepath.Join(dst, "greeter", "leak.md")
	if data, err := os.ReadFile(leaked); err == nil {
		t.Fatalf("source symlink was dereferenced: leaked %q", string(data))
	}
}

// TestMaterializeSkillsDoesNotWriteThroughDestSymlink proves the local guard
// refuses to write through a pre-planted symlinked DESTINATION component. The
// running agent may write to its own cwd between iterations; if it plants
// destDir/<name> as a symlink pointing outside, a re-materialize must not
// escape. The outside target must remain untouched.
func TestMaterializeSkillsDoesNotWriteThroughDestSymlink(t *testing.T) {
	img := t.TempDir()
	skill := filepath.Join(img, "skills", "greeter")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Greeter"), 0o600); err != nil {
		t.Fatal(err)
	}

	// An outside directory the attacker wants writes to land in.
	outside := t.TempDir()
	base := t.TempDir()
	sub := filepath.Join(".claude", "skills")
	dst := filepath.Join(base, sub)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	// Pre-plant destDir/greeter as a symlink to the outside dir.
	if err := os.Symlink(outside, filepath.Join(dst, "greeter")); err != nil {
		t.Fatal(err)
	}

	// Materialize must refuse to write through the symlink.
	if err := MaterializeSkills(img, base, sub); err == nil {
		t.Fatal("expected an error writing through a symlinked dest component, got nil")
	}
	// The outside target must be untouched — no SKILL.md escaped into it.
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err == nil {
		t.Fatal("write escaped through the symlinked dest into the outside dir")
	}
}

func TestMaterializeSkillsRejectsUnsafeName(t *testing.T) {
	img := t.TempDir()
	// A skill dir whose name would escape destDir if joined naively.
	bad := filepath.Join(img, "skills", "..evil")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MaterializeSkills(img, t.TempDir(), "d"); err == nil {
		t.Fatal("expected an error for an unsafe skill name, got nil")
	}
}

// TestMaterializeSkillsRefusesSymlinkedDestAncestor is the M15 security
// regression: the agent workdir is persistent and agent-controlled, so an
// agent can plant a destination ANCESTOR (subDir itself, ".claude/skills", or
// an intermediate like ".claude") as a symlink pointing OUTSIDE the workdir.
// The old code's copyTree guard only Lstat-checked components AT OR BELOW
// destDir/<name>, so its first MkdirAll(destDir/<name>) would FOLLOW the
// symlinked ancestor and write skill content outside the agent cwd. This test
// plants each ancestor as a symlink to an outside dir and asserts (a)
// MaterializeSkills refuses with an error and (b) the outside target is
// UNTOUCHED — no skill content escaped through the symlinked ancestor.
func TestMaterializeSkillsRefusesSymlinkedDestAncestor(t *testing.T) {
	for _, tc := range []struct {
		name        string // ancestor component planted as a symlink
		sub         string
		symlinkPath func(base string) string
	}{
		{
			name: "skills-leaf",
			sub:  filepath.Join(".claude", "skills"),
			symlinkPath: func(base string) string {
				return filepath.Join(base, ".claude", "skills")
			},
		},
		{
			name: "claude-intermediate",
			sub:  filepath.Join(".claude", "skills"),
			symlinkPath: func(base string) string {
				return filepath.Join(base, ".claude")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := t.TempDir()
			skill := filepath.Join(img, "skills", "greeter")
			if err := os.MkdirAll(skill, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Greeter"), 0o600); err != nil {
				t.Fatal(err)
			}

			outside := t.TempDir()
			base := t.TempDir()
			link := tc.symlinkPath(base)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			// Plant the ancestor as a symlink to the outside dir.
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}

			if err := MaterializeSkills(img, base, tc.sub); err == nil {
				t.Fatal("expected refusal for a symlinked dest ancestor, got nil")
			}
			// Nothing may have escaped through the symlinked ancestor.
			if _, err := os.Stat(filepath.Join(outside, "greeter", "SKILL.md")); err == nil {
				t.Fatal("write escaped through symlinked ancestor into outside/greeter")
			}
			if _, err := os.Stat(filepath.Join(outside, "skills", "greeter", "SKILL.md")); err == nil {
				t.Fatal("write escaped through symlinked ancestor into outside/skills/greeter")
			}
		})
	}
}
