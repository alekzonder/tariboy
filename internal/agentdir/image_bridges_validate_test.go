package agentdir

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/alekzonder/tariboy/internal/agentskills"
)

const generatedBridgeName = ".claude-plugin/plugin.json"

func publishedBridge(t *testing.T, harness string, files ...BridgeFile) (finalDir string, expected []agentskills.Metadata, plan BridgePlan) {
	t.Helper()
	source, expected := bridgeFixture(t)
	finalDir, err := New(t.TempDir(), "worker").ImageBridgeDir(strings.Repeat("c", 64), "1", harness)
	if err != nil {
		t.Fatal(err)
	}
	plan = BridgePlan{SkillDestination: "skills", Files: files}
	if err := PrepareImageBridge(source, finalDir, expected, plan); err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedBridge(finalDir, expected, plan); err != nil {
		t.Fatalf("fresh bridge does not validate: %v", err)
	}
	return finalDir, expected, plan
}

func editBridgeManifest(t *testing.T, finalDir string, edit func(*BridgeManifest)) {
	t.Helper()
	name := filepath.Join(finalDir, "bridge-manifest.json")
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BridgeManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	edit(&manifest)
	body, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBridgeMember(t *testing.T, finalDir, rel, body string, mode os.FileMode) {
	t.Helper()
	name := filepath.Join(finalDir, filepath.FromSlash(rel))
	if err := os.WriteFile(name, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, mode); err != nil {
		t.Fatal(err)
	}
}

// TestValidatePublishedBridgeReportsEachViolation pins the text of each
// rejection. Several cases break a later rule on purpose so a reordered check
// shows up as a different message.
func TestValidatePublishedBridgeReportsEachViolation(t *testing.T) {
	const (
		skillFile  = "skills/review/SKILL.md"
		scriptFile = "skills/review/scripts/check.sh"
	)
	generatedFile := BridgeFile{Path: generatedBridgeName, Body: []byte("{}\n"), Mode: 0o600}
	tests := []struct {
		name     string
		tamper   func(t *testing.T, finalDir string)
		want     string
		prefix   bool
		contains bool
	}{
		{name: "trailing json", tamper: func(t *testing.T, finalDir string) {
			f, err := os.OpenFile(filepath.Join(finalDir, "bridge-manifest.json"), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if _, err := f.WriteString(" {}"); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge manifest contains trailing JSON"},
		{name: "broken json", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, "bridge-manifest.json", "{", 0o600)
		}, want: "unexpected EOF", contains: true},
		{name: "unknown manifest field", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, "bridge-manifest.json", `{"mystery":1}`, 0o600)
		}, want: "json: unknown field", prefix: true},
		{name: "missing manifest", tamper: func(t *testing.T, finalDir string) {
			if err := os.Remove(filepath.Join(finalDir, "bridge-manifest.json")); err != nil {
				t.Fatal(err)
			}
		}, want: "open ", prefix: true},
		{name: "digest", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.ImageDigest = "other" })
		}, want: "image bridge manifest identity mismatch"},
		{name: "contract", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.ContractVersion = "2" })
		}, want: "image bridge manifest identity mismatch"},
		{name: "harness", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.Harness = "other" })
		}, want: "image bridge manifest identity mismatch"},
		{name: "destination", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.SkillDestination = "other" })
		}, want: "image bridge manifest identity mismatch"},
		{name: "identity before skills", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.ImageDigest, m.Skills = "other", nil })
		}, want: "image bridge manifest identity mismatch"},
		{name: "skills", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.Skills[0].Size++ })
		}, want: "image bridge skill metadata mismatch"},
		{name: "skills before generated", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) {
				m.Skills = nil
				delete(m.Files, generatedBridgeName)
			})
		}, want: "image bridge skill metadata mismatch"},
		{name: "generated record", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { delete(m.Files, generatedBridgeName) })
		}, want: `generated bridge file ".claude-plugin/plugin.json" metadata mismatch`},
		{name: "generated before root mode", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { delete(m.Files, generatedBridgeName) })
			if err := os.Chmod(finalDir, 0o750); err != nil {
				t.Fatal(err)
			}
		}, want: `generated bridge file ".claude-plugin/plugin.json" metadata mismatch`},
		{name: "root mode", tamper: func(t *testing.T, finalDir string) {
			if err := os.Chmod(finalDir, 0o750); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge directory has invalid mode"},
		{name: "nested directory mode", tamper: func(t *testing.T, finalDir string) {
			if err := os.Chmod(filepath.Join(finalDir, "skills"), 0o750); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge directory has invalid mode"},
		{name: "unexpected root file", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, "extra.txt", "x", 0o600)
		}, want: `unexpected image bridge file "extra.txt"`},
		{name: "other skill", tamper: func(t *testing.T, finalDir string) {
			if err := os.Mkdir(filepath.Join(finalDir, "skills", "other"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeBridgeMember(t, finalDir, "skills/other/SKILL.md", "x", 0o600)
		}, want: `unexpected image bridge file "skills/other/SKILL.md"`},
		{name: "unlisted skill file", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, "skills/review/extra.txt", "x", 0o600)
		}, want: `image bridge file "skills/review/extra.txt" metadata mismatch`},
		{name: "file mode", tamper: func(t *testing.T, finalDir string) {
			if err := os.Chmod(filepath.Join(finalDir, filepath.FromSlash(scriptFile)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: `image bridge file "skills/review/scripts/check.sh" metadata mismatch`},
		{name: "file size", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, skillFile, "short", 0o600)
		}, want: `image bridge file "skills/review/SKILL.md" metadata mismatch`},
		{name: "file bytes", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, scriptFile, "#!/bin/sh\nexit 1\n", 0o700)
		}, want: `image bridge file "skills/review/scripts/check.sh" integrity mismatch`},
		{name: "generated bytes", tamper: func(t *testing.T, finalDir string) {
			writeBridgeMember(t, finalDir, generatedBridgeName, "[]\n", 0o600)
		}, want: `image bridge file ".claude-plugin/plugin.json" integrity mismatch`},
		{name: "symlink", tamper: func(t *testing.T, finalDir string) {
			if err := os.Symlink("SKILL.md", filepath.Join(finalDir, "skills", "review", "link")); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge contains a symlink"},
		{name: "special file", tamper: func(t *testing.T, finalDir string) {
			if err := syscall.Mkfifo(filepath.Join(finalDir, "skills", "review", "pipe"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge contains a special file"},
		{name: "deleted file", tamper: func(t *testing.T, finalDir string) {
			if err := os.Remove(filepath.Join(finalDir, filepath.FromSlash(scriptFile))); err != nil {
				t.Fatal(err)
			}
		}, want: "image bridge files are missing"},
		{name: "extra record", tamper: func(t *testing.T, finalDir string) {
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.Files["skills/review/gone"] = bridgeFileRecord{} })
		}, want: "image bridge files are missing"},
		{name: "skill no longer prepares", tamper: func(t *testing.T, finalDir string) {
			if err := os.Remove(filepath.Join(finalDir, filepath.FromSlash(skillFile))); err != nil {
				t.Fatal(err)
			}
			editBridgeManifest(t, finalDir, func(m *BridgeManifest) { delete(m.Files, skillFile) })
		}, want: "SKILL.md", contains: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			finalDir, expected, plan := publishedBridge(t, "claude", generatedFile)
			tc.tamper(t, finalDir)
			err := validatePublishedBridge(finalDir, expected, plan)
			if err == nil {
				t.Fatal("tampered bridge validated")
			}
			switch {
			case tc.contains:
				if !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.want)
				}
			case tc.prefix:
				if !strings.HasPrefix(err.Error(), tc.want) {
					t.Fatalf("error = %q, want prefix %q", err, tc.want)
				}
			case err.Error() != tc.want:
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
			if errors.Is(err, ErrImageCopyMismatch) {
				t.Fatalf("%q is not an image copy mismatch", err)
			}
		})
	}
}

func TestValidatePublishedBridgeReportsSkillDrift(t *testing.T) {
	t.Parallel()
	finalDir, expected, plan := publishedBridge(t, "claude")
	const script = "skills/review/scripts/check.sh"
	body := "#!/bin/sh\nexit 1\n"
	writeBridgeMember(t, finalDir, script, body, 0o700)
	editBridgeManifest(t, finalDir, func(m *BridgeManifest) { m.Files[script] = fileRecord([]byte(body), 0o700) })
	err := validatePublishedBridge(finalDir, expected, plan)
	if !errors.Is(err, ErrImageCopyMismatch) || err.Error() != `skill "review" does not match image manifest` {
		t.Fatalf("error = %v", err)
	}
}

func TestValidatePublishedBridgeChecksIdentityBeforePlan(t *testing.T) {
	t.Parallel()
	_, expected := bridgeFixture(t)
	bad := BridgePlan{SkillDestination: "../outside"}
	if err := validatePublishedBridge(t.TempDir(), expected, bad); err == nil || err.Error() != "invalid final image bridge path" {
		t.Fatalf("error = %v", err)
	}
	finalDir, _ := New(t.TempDir(), "worker").ImageBridgeDir(strings.Repeat("d", 64), "1", "claude")
	if err := validatePublishedBridge(finalDir, expected, bad); err == nil || err.Error() != `invalid bridge path "../outside"` {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenCodeBridgeStillChecksGeneratedInstallFileNames(t *testing.T) {
	t.Parallel()
	generated := BridgeFile{Path: "package.json", Body: []byte("{}\n"), Mode: 0o600}
	finalDir, expected, plan := publishedBridge(t, "opencode", generated)
	writeBridgeMember(t, finalDir, "package.json", "[]\n", 0o600)
	err := validatePublishedBridge(finalDir, expected, plan)
	if err == nil || err.Error() != `image bridge file "package.json" integrity mismatch` {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenCodeBridgeSkipsInstallFilesOnlyAtRoot(t *testing.T) {
	t.Parallel()
	finalDir, expected, plan := publishedBridge(t, "opencode")
	writeBridgeMember(t, finalDir, "package.json", "{}\n", 0o600)
	if err := validatePublishedBridge(finalDir, expected, plan); err != nil {
		t.Fatalf("root install file was checked: %v", err)
	}
	writeBridgeMember(t, finalDir, "skills/review/package.json", "{}\n", 0o600)
	err := validatePublishedBridge(finalDir, expected, plan)
	if err == nil || err.Error() != `image bridge file "skills/review/package.json" metadata mismatch` {
		t.Fatalf("error = %v", err)
	}
}
