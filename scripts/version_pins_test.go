package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

func TestVersionPinsAgreeWithCanonicalVersion(t *testing.T) {
	root := repositoryRoot(t)
	want := numericVersion(version.Version)

	for _, check := range []struct {
		path string
		read func(t *testing.T, path string) string
	}{
		{"desktop/src-tauri/Cargo.toml", readCargoPackageVersion},
		{"desktop/src-tauri/tauri.conf.json", readTauriVersion},
		{"desktop/src-tauri/Cargo.lock", readDesktopLockVersion},
	} {
		got := check.read(t, filepath.Join(root, check.path))
		if got != want {
			t.Errorf("%s version = %q, want canonical version %q", check.path, got, want)
		}
	}
}

func TestDeclaredReleaseVersionMatchesCanonicalVersion(t *testing.T) {
	root := repositoryRoot(t)
	if got, want := readReleaseVersion(t, filepath.Join(root, "scripts/release-version.txt")), version.Version; got != want {
		t.Fatalf("scripts/release-version.txt version = %q, want canonical version %q", got, want)
	}
}

func TestVersionPinnedFilesAgreeWithCanonicalVersion(t *testing.T) {
	root := repositoryRoot(t)
	allowlistPath := filepath.Join(root, "scripts/version-pinned-files.txt")
	contents, err := os.ReadFile(allowlistPath)
	if err != nil {
		t.Fatalf("read %s: %v", allowlistPath, err)
	}

	paths := make(map[string]struct{})
	for _, line := range strings.Split(string(contents), "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") {
			continue
		}
		paths[path] = struct{}{}
	}
	if len(paths) == 0 {
		t.Fatal("scripts/version-pinned-files.txt contains no paths")
	}
	if _, found := paths["docs/package-lock.json"]; found {
		t.Fatal("scripts/version-pinned-files.txt must not include docs/package-lock.json")
	}

	for path := range paths {
		contents, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("read allowlisted file %s: %v", path, err)
			continue
		}
		if !strings.Contains(string(contents), version.Version) {
			t.Errorf("allowlisted file %s does not contain canonical version %q", path, version.Version)
		}
	}
}

func TestNumericVersionStripsPrerelease(t *testing.T) {
	if got, want := numericVersion("0.28.0-alpha.1"), "0.28.0"; got != want {
		t.Fatalf("numericVersion() = %q, want %q", got, want)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	return filepath.Dir(packageDir)
}

func numericVersion(v string) string {
	return strings.SplitN(v, "-", 2)[0]
}

func readReleaseVersion(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(contents))
}

func readCargoPackageVersion(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "version = ") {
			return strings.Trim(line[len("version = "):], "\"")
		}
	}
	t.Fatalf("parse package version from %s", path)
	return ""
}

func readTauriVersion(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"version\": ") {
			return strings.Trim(strings.TrimSuffix(line[len("\"version\": "):], ","), "\"")
		}
	}
	t.Fatalf("parse top-level version from %s", path)
	return ""
}

func readDesktopLockVersion(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, block := range strings.Split(string(contents), "[[package]]") {
		isDesktopPackage := false
		for _, line := range strings.Split(block, "\n") {
			if line == "name = \"tariboy-desktop\"" {
				isDesktopPackage = true
				break
			}
		}
		if !isDesktopPackage {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "version = ") {
				return strings.Trim(line[len("version = "):], "\"")
			}
		}
	}
	t.Fatalf("parse tariboy-desktop package version from %s", path)
	return ""
}

func TestFindVersionLiterals(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		contents string
		want     []versionLiteral
	}{
		{
			name:     "bare version",
			contents: "0.29.0",
			want:     []versionLiteral{{Line: 1, Literal: "0.29.0"}},
		},
		{
			name:     "underscore delimited artifact name",
			contents: "Tariboy_0.29.0_aarch64.dmg",
			want:     []versionLiteral{{Line: 1, Literal: "0.29.0"}},
		},
		{
			name:     "dotted address is not a version",
			contents: "127.0.0.1:9990",
			want:     nil,
		},
		{
			name:     "four component literal is not a version",
			contents: "1.2.3.4",
			want:     nil,
		},
		{
			name:     "prerelease is part of the literal",
			contents: "0.30.0-alpha.1",
			want:     []versionLiteral{{Line: 1, Literal: "0.30.0-alpha.1"}},
		},
		{
			name:     "two literals on one line keep their line number",
			contents: "intro\nupgrade 0.10.1 to 0.29.0 now\ntail",
			want: []versionLiteral{
				{Line: 2, Literal: "0.10.1"},
				{Line: 2, Literal: "0.29.0"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := findVersionLiterals(testCase.contents)
			if len(got) != len(testCase.want) {
				t.Fatalf("findVersionLiterals(%q) = %v, want %v", testCase.contents, got, testCase.want)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Errorf("findVersionLiterals(%q)[%d] = %v, want %v", testCase.contents, i, got[i], testCase.want[i])
				}
			}
		})
	}
}

func TestUnexpectedVersionLiterals(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		path       string
		contents   string
		canonical  string
		exceptions []versionException
		want       []versionLiteral
	}{
		{
			name:      "canonical version alone is accepted",
			path:      "README.md",
			contents:  "ships 0.29.0 today",
			canonical: "0.29.0",
			want:      nil,
		},
		{
			name:      "numeric form of a prerelease canonical version is accepted",
			path:      "README.md",
			contents:  "ships 0.30.0 today",
			canonical: "0.30.0-alpha.1",
			want:      nil,
		},
		{
			name:      "planted stale literal is reported",
			path:      "docs/docs/support.mdx",
			contents:  "current 0.29.0\nstale 0.28.1\n",
			canonical: "0.29.0",
			want:      []versionLiteral{{Line: 2, Literal: "0.28.1"}},
		},
		{
			name:       "declared exception for this path is accepted",
			path:       "docs/docs/support.mdx",
			contents:   "current 0.29.0\nolder than 0.10.1\n",
			canonical:  "0.29.0",
			exceptions: []versionException{{Path: "docs/docs/support.mdx", Literal: "0.10.1", Reason: "upgrade floor"}},
			want:       nil,
		},
		{
			name:       "exception declared for another path does not apply",
			path:       "README.md",
			contents:   "current 0.29.0\nolder than 0.10.1\n",
			canonical:  "0.29.0",
			exceptions: []versionException{{Path: "docs/docs/support.mdx", Literal: "0.10.1", Reason: "upgrade floor"}},
			want:       []versionLiteral{{Line: 2, Literal: "0.10.1"}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := unexpectedVersionLiterals(testCase.path, testCase.contents, testCase.canonical, testCase.exceptions)
			if len(got) != len(testCase.want) {
				t.Fatalf("unexpectedVersionLiterals(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Errorf("unexpectedVersionLiterals(%q)[%d] = %v, want %v", testCase.path, i, got[i], testCase.want[i])
				}
			}
		})
	}
}

// versionLiteral is one version-shaped string found in a file, with the 1-based
// line number it was found on.
type versionLiteral struct {
	Line    int
	Literal string
}

// versionException records a version literal that is allowed to appear in a
// prose-pinned file even though it is not the canonical version.
type versionException struct {
	Path    string
	Literal string
	Reason  string
}

// versionLiteralPattern matches MAJOR.MINOR.PATCH with an optional semver
// prerelease suffix. Boundaries are checked in findVersionLiterals rather than
// with \b: '_' is a word byte, so a \b-anchored pattern would miss the version
// inside an artifact name such as Tariboy_0.29.0_aarch64.dmg.
var versionLiteralPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?`)

// findVersionLiterals returns every version-shaped literal in contents together
// with its 1-based line number. A match whose immediately neighbouring byte is a
// digit or '.' is rejected: that is a longer dotted number such as the address
// 127.0.0.1 or the four-component literal 1.2.3.4, not a version.
func findVersionLiterals(contents string) []versionLiteral {
	var found []versionLiteral
	for number, line := range strings.Split(contents, "\n") {
		for _, match := range versionLiteralPattern.FindAllStringIndex(line, -1) {
			start, end := match[0], match[1]
			if start > 0 && isVersionBoundaryByte(line[start-1]) {
				continue
			}
			if end < len(line) && isVersionBoundaryByte(line[end]) {
				continue
			}
			found = append(found, versionLiteral{Line: number + 1, Literal: line[start:end]})
		}
	}
	return found
}

func isVersionBoundaryByte(b byte) bool {
	return b == '.' || (b >= '0' && b <= '9')
}

// unexpectedVersionLiterals returns the literals in contents that are neither
// the canonical version nor an exception declared for path. The numeric form of
// the canonical version counts as canonical too: that is what a prerelease build
// legitimately writes in prose.
func unexpectedVersionLiterals(path, contents, canonical string, exceptions []versionException) []versionLiteral {
	allowed := map[string]struct{}{canonical: {}, numericVersion(canonical): {}}
	for _, exception := range exceptions {
		if exception.Path == path {
			allowed[exception.Literal] = struct{}{}
		}
	}

	var unexpected []versionLiteral
	for _, found := range findVersionLiterals(contents) {
		if _, ok := allowed[found.Literal]; ok {
			continue
		}
		unexpected = append(unexpected, found)
	}
	return unexpected
}

// prosePinnedPaths are the allowlisted files that carry the canonical version in
// prose. TestVersionPinnedFilesAgreeWithCanonicalVersion only asserts that the
// canonical version appears somewhere in them, so a stale version-shaped literal
// elsewhere in the same file would ship unnoticed; these files are scanned for
// foreign literals as well.
var prosePinnedPaths = []string{
	"README.md",
	"docs/internal-alpha-release-runbook.md",
	"docs/docs/quickstart.mdx",
	"docs/docs/security-controls.mdx",
	"docs/docs/support.mdx",
}

// exactlyParsedPinnedPaths are the allowlisted files whose version is parsed and
// compared for equality by TestVersionPinsAgreeWithCanonicalVersion. They are
// deliberately not prose-scanned: desktop/src-tauri/Cargo.toml pins six
// third-party dependency versions (uuid, portable-pty, zip, plist, ureq and
// security-framework), so a scan there would report noise rather than drift.
var exactlyParsedPinnedPaths = []string{
	"desktop/src-tauri/Cargo.toml",
	"desktop/src-tauri/tauri.conf.json",
}

// canonicalVersionSourcePath declares the canonical version and therefore needs
// no guard of its own beyond the substring check.
const canonicalVersionSourcePath = "internal/version/version.go"

// prosePinnedVersionExceptions are the non-canonical version literals that are
// allowed to stay in a prose-pinned file because they are historical statements
// rather than pins that should move with a release.
var prosePinnedVersionExceptions = []versionException{
	{Path: "README.md", Literal: "0.10.1", Reason: "upgrade floor, a historical statement"},
	{Path: "docs/docs/quickstart.mdx", Literal: "0.10.1", Reason: "same upgrade floor"},
	{Path: "docs/docs/support.mdx", Literal: "0.10.1", Reason: "same upgrade floor"},
	{Path: "docs/docs/support.mdx", Literal: "0.14.1", Reason: "behaviour boundary, a historical statement"},
}

func TestProsePinnedFilesCarryNoForeignVersionLiterals(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range prosePinnedPaths {
		contents, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("read prose-pinned file %s: %v", path, err)
			continue
		}
		for _, found := range unexpectedVersionLiterals(path, string(contents), version.Version, prosePinnedVersionExceptions) {
			t.Errorf("%s:%d carries version literal %q, want canonical version %q or a declared exception",
				path, found.Line, found.Literal, version.Version)
		}
	}
}

func TestProsePinnedVersionExceptionsStillOccur(t *testing.T) {
	root := repositoryRoot(t)
	for _, exception := range prosePinnedVersionExceptions {
		contents, err := os.ReadFile(filepath.Join(root, exception.Path))
		if err != nil {
			t.Errorf("read prose-pinned file %s: %v", exception.Path, err)
			continue
		}
		occurs := false
		for _, found := range findVersionLiterals(string(contents)) {
			if found.Literal == exception.Literal {
				occurs = true
				break
			}
		}
		if !occurs {
			t.Errorf("declared exception %q for %s no longer occurs there (%s); remove the exception so the file is guarded again",
				exception.Literal, exception.Path, exception.Reason)
		}
	}
}

func TestEveryVersionPinnedFileIsClaimedByAGuard(t *testing.T) {
	root := repositoryRoot(t)
	allowlistPath := filepath.Join(root, "scripts/version-pinned-files.txt")
	contents, err := os.ReadFile(allowlistPath)
	if err != nil {
		t.Fatalf("read %s: %v", allowlistPath, err)
	}

	allowlisted := make(map[string]struct{})
	for _, path := range versionPinnedAllowlistPaths(string(contents)) {
		allowlisted[path] = struct{}{}
	}

	claimed := make(map[string]struct{})
	for _, path := range prosePinnedPaths {
		claimed[path] = struct{}{}
	}
	for _, path := range exactlyParsedPinnedPaths {
		claimed[path] = struct{}{}
	}
	claimed[canonicalVersionSourcePath] = struct{}{}

	for path := range allowlisted {
		if _, ok := claimed[path]; !ok {
			t.Errorf("scripts/version-pinned-files.txt lists %s but no guard claims it; add it to prosePinnedPaths or to exactlyParsedPinnedPaths", path)
		}
	}
	for path := range claimed {
		if _, ok := allowlisted[path]; !ok {
			t.Errorf("%s is claimed by a guard but is not listed in scripts/version-pinned-files.txt", path)
		}
	}
}

// versionPinnedAllowlistPaths returns the non-comment entries of
// scripts/version-pinned-files.txt.
func versionPinnedAllowlistPaths(contents string) []string {
	var paths []string
	for _, line := range strings.Split(contents, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}
