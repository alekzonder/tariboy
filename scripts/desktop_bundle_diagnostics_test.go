package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	return filepath.Dir(packageDir)
}

// The macOS bundler reports only "failed to run bundle_dmg.sh", so the release
// packager has to describe the disk-image state itself. The reporter runs after
// a failure and must never fail, never require macOS-only tools, and never stop
// at the first missing one.
func TestDesktopBundleDiagnosticsReportsDiskImageState(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "desktop-bundle-diagnostics.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("diagnostics reporter is missing: %v", err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("diagnostics reporter must not fail: %v\n%s", err, output)
	}
	report := string(output)

	for _, section := range []string{
		"host",
		"disk space",
		"attached disk images",
		"mounted volumes",
		"disk image processes",
		"bundle output",
	} {
		if !strings.Contains(report, section) {
			t.Errorf("diagnostics report is missing the %q section:\n%s", section, report)
		}
	}
	if !strings.Contains(report, "end of desktop bundle diagnostics") {
		t.Errorf("diagnostics report must be delimited:\n%s", report)
	}
	if !strings.Contains(report, "unavailable") {
		t.Errorf("a host without hdiutil must report the tool as unavailable:\n%s", report)
	}
}

// A failed bundling attempt must produce the diagnostics report and one verbose
// retry that streams bundle_dmg.sh's own output.
func TestPackageAlphaCollectsDiagnosticsWhenBundlingFails(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "package-alpha.sh"))
	if err != nil {
		t.Fatalf("read package-alpha.sh: %v", err)
	}
	script := string(data)

	if !strings.Contains(script, "desktop-bundle-diagnostics.sh") {
		t.Error("package-alpha.sh must report bundle diagnostics when bundling fails")
	}
	if !strings.Contains(script, "DESKTOP_BUNDLE_VERBOSE=1") {
		t.Error("package-alpha.sh must retry the failed bundling with verbose bundler output")
	}
}

// Verbose bundling is opt-in: ordinary release builds keep the quiet bundler
// output, and the retry asks for the bundler's own log.
func TestDesktopBundleVerbosityIsOptIn(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name    string
		args    []string
		verbose bool
	}{
		{name: "quiet", args: []string{"-n", "desktop", "PLATFORM=darwin"}, verbose: false},
		{name: "verbose", args: []string{"-n", "desktop", "PLATFORM=darwin", "DESKTOP_BUNDLE_VERBOSE=1"}, verbose: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("make", test.args...)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make %v: %v\n%s", test.args, err, output)
			}
			recipe := string(output)
			if !strings.Contains(recipe, "cargo tauri build") {
				t.Fatalf("recipe does not bundle the desktop app:\n%s", recipe)
			}
			if got := strings.Contains(recipe, "--verbose"); got != test.verbose {
				t.Errorf("verbose bundling = %v, want %v:\n%s", got, test.verbose, recipe)
			}
		})
	}
}
