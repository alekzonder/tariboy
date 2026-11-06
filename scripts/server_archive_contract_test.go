package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release publishes the archives SSH servers download: `tariboy update`
// on Linux and the Desktop installer on every server platform. The archive
// builder, the packager, the release gate, the Desktop and the updater must
// name the same files, so drift in any one of them is caught here rather than
// by a failed install on a host.
func TestServerArchiveContractIsShared(t *testing.T) {
	root := repositoryRoot(t)
	for _, expectation := range []struct {
		path     string
		fragment string
	}{
		{"Makefile", `SERVER_ARCHIVE_DIR := dist/server-archives/$(VERSION)`},
		{"Makefile", `tariboy_$(VERSION)_$$platform.tar.gz`},
		{filepath.Join("scripts", "package-alpha.sh"), `SERVER_PLATFORMS="linux-x86_64 linux-aarch64 darwin-arm64"`},
		{filepath.Join("scripts", "package-alpha.sh"), `archive="tariboy_${VERSION}_${platform}.tar.gz"`},
		{filepath.Join("scripts", "package-alpha.sh"), `ARCHIVE_ROOT="$ROOT/dist/server-archives/$VERSION"`},
		{filepath.Join("scripts", "package-alpha.sh"), `shasum -a 256 "$ARTIFACT" $SERVER_ARCHIVES release.json > SHA256SUMS`},
		{filepath.Join("scripts", "check-alpha-artifacts.sh"), `SERVER_PLATFORMS="linux-x86_64 linux-aarch64 darwin-arm64"`},
		{filepath.Join("scripts", "check-alpha-artifacts.sh"), `archive="tariboy_${VERSION}_${platform}.tar.gz"`},
		{filepath.Join("desktop", "src-tauri", "src", "bundle.rs"), `format!("tariboy_{version}_{target}.tar.gz")`},
		{filepath.Join("desktop", "src-tauri", "src", "remote-download.sh"), `tariboy_${version}_linux-x86_64.tar.gz`},
		{filepath.Join("desktop", "src-tauri", "src", "remote-download.sh"), `tariboy_${version}_linux-aarch64.tar.gz`},
		{filepath.Join("desktop", "src-tauri", "src", "remote-download.sh"), `tariboy_${version}_darwin-arm64.tar.gz`},
		{filepath.Join("desktop", "src-tauri", "src", "provision.rs"), `"https://github.com/alekzonder/tariboy/releases"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `DefaultBaseURL = "https://github.com/alekzonder/tariboy/releases"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `archiveFormat = "tariboy_%s_linux-%s.tar.gz"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `"amd64": "x86_64",`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `"arm64": "aarch64",`},
	} {
		contents, err := os.ReadFile(filepath.Join(root, expectation.path))
		if err != nil {
			t.Errorf("read %s: %v", expectation.path, err)
			continue
		}
		if !strings.Contains(string(contents), expectation.fragment) {
			t.Errorf("%s does not contain %q", expectation.path, expectation.fragment)
		}
	}
}
