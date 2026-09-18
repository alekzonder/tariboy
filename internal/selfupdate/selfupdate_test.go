package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testVersion = "1.2.3"

func TestResolveLatestReadsTagFromRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		http.Redirect(w, r, "/releases/tag/v9.8.7", http.StatusFound)
	}))
	defer server.Close()

	got, err := resolveLatest(context.Background(), server.Client(), server.URL+"/releases")
	if err != nil {
		t.Fatalf("resolveLatest: %v", err)
	}
	if got != "9.8.7" {
		t.Fatalf("resolved %q, want 9.8.7", got)
	}
}

func TestValidateVersionRejectsUnsafeValues(t *testing.T) {
	for _, bad := range []string{"", "latest", "../etc", "1.2.3/x", ".hidden", "1.2.3 4"} {
		if err := validateVersion(bad); err == nil {
			t.Errorf("validateVersion(%q) accepted an unsafe version", bad)
		}
	}
	if err := validateVersion("0.64.0-rc.1"); err != nil {
		t.Errorf("validateVersion rejected a valid version: %v", err)
	}
}

func TestRunRejectsUnsupportedPlatform(t *testing.T) {
	err := Run(context.Background(), testOptions(t, t.TempDir(), "", testVersion, "darwin", "arm64"))
	if err == nil || !strings.Contains(err.Error(), "linux-x86_64") {
		t.Fatalf("expected an unsupported-platform error, got %v", err)
	}
}

func TestRunRejectsUnmanagedInstall(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".local", "bin", "tariboy"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Run(context.Background(), testOptions(t, home, "", testVersion, "linux", "amd64"))
	if err == nil || !strings.Contains(err.Error(), "not a managed installation") {
		t.Fatalf("expected a managed-installation error, got %v", err)
	}
}

func TestRunSkipsWhenAlreadyAtTargetVersion(t *testing.T) {
	home := managedHome(t, testVersion)
	var out bytes.Buffer
	opts := testOptions(t, home, "", testVersion, "linux", "amd64")
	opts.Out = &out

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Fatalf("output %q does not report the no-op", out.String())
	}
}

func TestRunFailsOnChecksumMismatch(t *testing.T) {
	home := managedHome(t, "0.0.1")
	archive := buildArchive(t, testVersion)
	base := releaseServer(t, testVersion, archive, strings.Repeat("0", 64))

	err := Run(context.Background(), testOptions(t, home, base, testVersion, "linux", "amd64"))
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(home, ".local", "lib", "tariboy")); len(entries) != 1 {
		t.Fatalf("staging was not cleaned up: %v", entries)
	}
}

func TestRunInstallsRelease(t *testing.T) {
	requireInstallTools(t)
	home := managedHome(t, "0.0.1")
	archive := buildArchive(t, testVersion)
	sum := sha256.Sum256(archive)
	base := releaseServer(t, testVersion, archive, hex.EncodeToString(sum[:]))

	var out bytes.Buffer
	opts := testOptions(t, home, base, "latest", "linux", "amd64")
	opts.Out = &out
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}

	link, err := os.Readlink(filepath.Join(home, ".local", "bin", "tariboyd"))
	if err != nil {
		t.Fatalf("read tariboyd link: %v", err)
	}
	want := filepath.Join(home, ".local", "lib", "tariboy", testVersion, "tariboyd")
	if link != want {
		t.Fatalf("tariboyd -> %q, want %q", link, want)
	}
	installed, err := os.ReadFile(filepath.Join(home, ".local", "lib", "tariboy", testVersion, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	if strings.TrimSpace(string(installed)) != testVersion {
		t.Fatalf("installed VERSION %q, want %q", installed, testVersion)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".local", "lib", "tariboy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".stage-") {
			t.Fatalf("staging directory %q was left behind", entry.Name())
		}
	}
}

func testOptions(t *testing.T, home, baseURL, version, goos, goarch string) Options {
	t.Helper()
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	baseDir := filepath.Join(t.TempDir(), "base")
	return Options{
		Version: version,
		BaseURL: baseURL,
		Home:    home,
		GOOS:    goos,
		GOARCH:  goarch,
		Out:     &bytes.Buffer{},
		Getenv: func(key string) string {
			switch key {
			case "HOME":
				return home
			case "TARIBOY_BASE_DIR":
				return baseDir
			case "TARIBOY_RUNTIME_DIR":
				return runtimeDir
			case "TARIBOY_HTTP_ADDR":
				return ""
			}
			return ""
		},
	}
}

// managedHome builds the installer-managed layout Run requires: a published
// release directory plus the managed symlinks in ~/.local/bin.
func managedHome(t *testing.T, version string) string {
	t.Helper()
	home := t.TempDir()
	release := filepath.Join(home, ".local", "lib", "tariboy", version)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(release, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range payloadBinaries {
		if err := os.WriteFile(filepath.Join(release, name), []byte("#!/bin/sh\necho "+version+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(release, name), filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(release, "tariboy-tasks"), filepath.Join(bin, "ttasks")); err != nil {
		t.Fatal(err)
	}
	writeSums(t, release)
	return home
}

func writeSums(t *testing.T, dir string) {
	t.Helper()
	var sums bytes.Buffer
	for _, name := range payloadBinaries {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), sums.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildArchive produces the same payload layout the release publishes: the five
// binaries, VERSION, SHA256SUMS, and the installer script.
func buildArchive(t *testing.T, version string) []byte {
	t.Helper()
	dir := t.TempDir()
	for _, name := range payloadBinaries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho "+version+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSums(t, dir)
	installer, err := os.ReadFile(filepath.Join("..", "..", "desktop", "src-tauri", "src", "remote-install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, installerName), installer, 0o755); err != nil {
		t.Fatal(err)
	}

	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	writer := tar.NewWriter(gz)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, ierr := entry.Info()
		if ierr != nil {
			t.Fatal(ierr)
		}
		header, herr := tar.FileInfoHeader(info, "")
		if herr != nil {
			t.Fatal(herr)
		}
		header.Name = "./" + entry.Name()
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		body, rerr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

// releaseServer serves the GitHub release surface Run consumes and returns its
// base URL.
func releaseServer(t *testing.T, version string, archive []byte, digest string) string {
	t.Helper()
	name := fmt.Sprintf(archiveFormat, version)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v"+version, http.StatusFound)
		case "/releases/download/v" + version + "/" + sumsName:
			fmt.Fprintf(w, "%s  %s\n", digest, name)
		case "/releases/download/v" + version + "/" + name:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/releases"
}

func requireInstallTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the installer contract is exercised on Linux")
	}
	for _, tool := range []string{"sh", "flock", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("missing required tool: %s", tool)
		}
	}
}
