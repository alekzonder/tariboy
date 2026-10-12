package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release publishes the archives `tariboy update` downloads, one per Linux
// architecture. The packager, the release gate, and the updater must name the
// same files, so drift in any one of them is caught here rather than by a
// failed update on a host.
func TestServerArchiveContractIsShared(t *testing.T) {
	root := repositoryRoot(t)
	for _, expectation := range []struct {
		path     string
		fragment string
	}{
		{filepath.Join("scripts", "package-alpha.sh"), `SERVER_ARCHES="x86_64 aarch64"`},
		{filepath.Join("scripts", "package-alpha.sh"), `archive="tariboy_${VERSION}_linux-${arch}.tar.gz"`},
		{filepath.Join("scripts", "package-alpha.sh"), `shasum -a 256 "$ARTIFACT" $SERVER_ARCHIVES release.json > SHA256SUMS`},
		{filepath.Join("scripts", "check-alpha-artifacts.sh"), `SERVER_ARCHES="x86_64 aarch64"`},
		{filepath.Join("scripts", "check-alpha-artifacts.sh"), `archive="tariboy_${VERSION}_linux-${arch}.tar.gz"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `archiveFormat = "tariboy_%s_linux-%s.tar.gz"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `"amd64": "x86_64",`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `"arm64": "aarch64",`},
	} {
		contents, err := os.ReadFile(filepath.Join(root, expectation.path))
		if err != nil {
			t.Fatalf("read %s: %v", expectation.path, err)
		}
		if !strings.Contains(string(contents), expectation.fragment) {
			t.Errorf("%s does not contain %q", expectation.path, expectation.fragment)
		}
	}
}
