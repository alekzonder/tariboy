package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBundledInstallsUpgradesAndPreservesWorkdir(t *testing.T) {
	h, _, store := newHost(t, &nameRunner{})
	manifest := Manifest{
		Name: "telegram", Version: "1.0.0", ProtocolVersion: ProtocolVersion,
		Types: []string{"tool"}, Exec: "tariboy-plugin-telegram",
		Channels: Channels{},
	}
	executable := filepath.Join(t.TempDir(), manifest.Exec)
	if err := os.WriteFile(executable, []byte("version one"), 0o700); err != nil {
		t.Fatal(err)
	}
	installed, err := h.EnsureBundled(executable, manifest)
	if err != nil || !installed {
		t.Fatalf("EnsureBundled installed=%v err=%v", installed, err)
	}
	record, ok, err := store.Get("telegram")
	if err != nil || !ok || !record.Enabled || record.Version != "1.0.0" {
		t.Fatalf("record = %+v ok=%v err=%v", record, ok, err)
	}
	marker := filepath.Join(h.workdir("telegram"), "state.json")
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest.Version = "2.0.0"
	executable = filepath.Join(t.TempDir(), manifest.Exec)
	if err := os.WriteFile(executable, []byte("version two"), 0o700); err != nil {
		t.Fatal(err)
	}
	installed, err = h.EnsureBundled(executable, manifest)
	if err != nil || !installed {
		t.Fatalf("upgrade installed=%v err=%v", installed, err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("workdir marker = %q err=%v", got, err)
	}
	h.StopAll()
}

func TestEnsureBundledSkipsMissingDeveloperBinary(t *testing.T) {
	h, _, _ := newHost(t, &nameRunner{})
	installed, err := h.EnsureBundled(filepath.Join(t.TempDir(), "missing"), Manifest{Name: "telegram"})
	if err != nil || installed {
		t.Fatalf("missing bundled executable installed=%v err=%v", installed, err)
	}
}
