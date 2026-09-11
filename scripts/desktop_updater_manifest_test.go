package scripts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopUpdaterManifestRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		archive   string
		signature string
		metadata  string
	}{
		{name: "empty signature", version: "1.2.3", archive: "Tariboy.app.tar.gz", metadata: `{"version":"1.2.3"}`},
		{name: "version mismatch", version: "1.2.4", archive: "Tariboy.app.tar.gz", signature: updaterSignature(), metadata: `{"version":"1.2.3"}`},
		{name: "missing archive", version: "1.2.3", archive: "missing.app.tar.gz", signature: updaterSignature(), metadata: `{"version":"1.2.3"}`},
		{name: "malformed signature", version: "1.2.3", archive: "Tariboy.app.tar.gz", signature: "not base64!", metadata: `{"version":"1.2.3"}`},
		{name: "unsafe archive basename", version: "1.2.3", archive: "Tariboy unsafe.app.tar.gz", signature: updaterSignature(), metadata: `{"version":"1.2.3"}`},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			releaseDir := filepath.Join(dir, "release")
			if err := os.Mkdir(releaseDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(releaseDir, "release.json"), []byte(testCase.metadata), 0o644); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(dir, testCase.archive)
			if !strings.HasPrefix(testCase.archive, "missing") {
				if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			signature := archive + ".sig"
			if err := os.WriteFile(signature, []byte(testCase.signature), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("python3", "desktop-updater-manifest.py",
				testCase.version, archive, signature, releaseDir)
			if err := cmd.Run(); err == nil {
				t.Fatalf("accepted %s", testCase.name)
			}
			if _, err := os.Stat(filepath.Join(releaseDir, "latest.json")); !os.IsNotExist(err) {
				t.Fatal("published manifest for invalid release")
			}
		})
	}
}

func TestDesktopUpdaterManifestStagesValidMetadata(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "release")
	if err := os.Mkdir(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "release.json"), []byte(`{"version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "SHA256SUMS"), []byte("existing  Tariboy_1.2.3_aarch64.dmg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "Tariboy.app.tar.gz")
	archiveBytes := []byte("signed updater archive")
	if err := os.WriteFile(archive, archiveBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	signatureValue := updaterSignature()
	signature := archive + ".sig"
	if err := os.WriteFile(signature, []byte(signatureValue+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("python3", "desktop-updater-manifest.py", "1.2.3", archive, signature, releaseDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate updater manifest: %v\n%s", err, output)
	}

	var manifest struct {
		Version   string `json:"version"`
		Notes     string `json:"notes"`
		PubDate   string `json:"pub_date"`
		Platforms map[string]struct {
			URL       string `json:"url"`
			Signature string `json:"signature"`
		} `json:"platforms"`
	}
	manifestBytes, err := os.ReadFile(filepath.Join(releaseDir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.2.3" || manifest.Notes != "Tariboy 1.2.3" || manifest.PubDate == "" {
		t.Fatalf("unexpected manifest metadata: %+v", manifest)
	}
	if len(manifest.Platforms) != 1 {
		t.Fatalf("platforms = %v, want exactly darwin-aarch64", manifest.Platforms)
	}
	platform, ok := manifest.Platforms["darwin-aarch64"]
	if !ok {
		t.Fatalf("platforms = %v, want darwin-aarch64", manifest.Platforms)
	}
	if want := "https://github.com/alekzonder/tariboy/releases/download/v1.2.3/Tariboy.app.tar.gz"; platform.URL != want {
		t.Fatalf("url = %q, want %q", platform.URL, want)
	}
	if platform.Signature != signatureValue {
		t.Fatalf("signature = %q, want fixture signature", platform.Signature)
	}
	for _, name := range []string{"Tariboy.app.tar.gz", "Tariboy.app.tar.gz.sig"} {
		if _, err := os.Stat(filepath.Join(releaseDir, name)); err != nil {
			t.Fatalf("staged %s: %v", name, err)
		}
	}
	checksums, err := os.ReadFile(filepath.Join(releaseDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	archiveDigest := sha256.Sum256(archiveBytes)
	for _, want := range []string{
		"existing  Tariboy_1.2.3_aarch64.dmg",
		hex.EncodeToString(archiveDigest[:]) + "  Tariboy.app.tar.gz",
		"  Tariboy.app.tar.gz.sig",
	} {
		if !strings.Contains(string(checksums), want) {
			t.Errorf("SHA256SUMS missing %q:\n%s", want, checksums)
		}
	}
}

func updaterSignature() string {
	packet := make([]byte, 74)
	copy(packet, "Ed")
	global := make([]byte, 64)
	text := "untrusted comment: signature from minisign secret key\n" +
		base64.StdEncoding.EncodeToString(packet) + "\n" +
		"trusted comment: timestamp:1789084800\n" +
		base64.StdEncoding.EncodeToString(global) + "\n"
	return base64.StdEncoding.EncodeToString([]byte(text))
}
