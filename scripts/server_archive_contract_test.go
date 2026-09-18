package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release publishes the archive `tariboy update` downloads. The packager,
// the release gate, and the updater must name the same file, so drift in any
// one of them is caught here rather than by a failed update on a host.
func TestServerArchiveContractIsShared(t *testing.T) {
	root := repositoryRoot(t)
	for _, expectation := range []struct {
		path     string
		fragment string
	}{
		{filepath.Join("scripts", "package-alpha.sh"), `SERVER_ARCHIVE="tariboy_${VERSION}_linux-x86_64.tar.gz"`},
		{filepath.Join("scripts", "package-alpha.sh"), `shasum -a 256 "$ARTIFACT" "$SERVER_ARCHIVE" release.json > SHA256SUMS`},
		{filepath.Join("scripts", "check-alpha-artifacts.sh"), `EXPECTED_SERVER_ARCHIVE="tariboy_${VERSION}_linux-x86_64.tar.gz"`},
		{filepath.Join("internal", "selfupdate", "selfupdate.go"), `archiveFormat = "tariboy_%s_linux-x86_64.tar.gz"`},
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
