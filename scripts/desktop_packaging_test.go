package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackedCodeAndDocsDoNotReferenceDeletedStoreFixtures(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("list tracked files: %v", err)
	}

	deletedPrefixes := []string{
		"sto" + "re/groups/",
		"sto" + "re/plugins/",
	}
	deletedJoinFragments := []string{
		`"sto` + `re", "groups"`,
		`"sto` + `re", "plugins"`,
	}
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		path := string(rawPath)
		if path == "" {
			continue
		}
		for _, prefix := range deletedPrefixes {
			if strings.HasPrefix(path, prefix) {
				t.Errorf("deleted fixture remains tracked: %s", path)
			}
		}
		if strings.HasPrefix(path, "outside/superpowers/") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, path))
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			t.Errorf("read tracked file %s: %v", path, readErr)
			continue
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for _, prefix := range deletedPrefixes {
			if bytes.Contains(data, []byte(prefix)) {
				t.Errorf("%s references deleted fixture path %q", path, prefix)
			}
		}
		for _, fragment := range deletedJoinFragments {
			if bytes.Contains(data, []byte(fragment)) {
				t.Errorf("%s constructs deleted fixture path with %q", path, fragment)
			}
		}
	}
}

func TestBuildGraphsDoNotReadStore(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	tests := []struct {
		name string
		args []string
	}{
		{name: "build", args: []string{"-n", "build"}},
		{name: "desktop-binaries", args: []string{"-n", "desktop-binaries"}},
		{name: "desktop-linux", args: []string{"-n", "-o", "desktop-tools-check", "-o", "desktop-lock-check", "-o", "ui", "HOST_OS=Linux", "HOST_ARCH=x86_64", "PLATFORM=linux", "desktop"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := exec.Command("make", testCase.args...)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dry-run %s: %v\n%s", testCase.name, err, output)
			}
			if strings.Contains(string(output), "store/") {
				t.Fatalf("%s build graph reads store/:\n%s", testCase.name, output)
			}
		})
	}
}

func TestDesktopPackagingSkipsFinderAutomation(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "-o", "desktop-binaries", "-o", "ui", "HOST_OS=Darwin", "HOST_ARCH=arm64", "PLATFORM=darwin", "desktop")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run desktop packaging: %v\n%s", err, output)
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "cargo tauri build --bundles app,dmg") {
			if !strings.Contains(line, "CI=true TARIBOY_VERSION=") {
				t.Fatalf("Tauri DMG build can invoke Finder automation:\n%s", line)
			}
			return
		}
	}
	t.Fatalf("dry run did not contain the Tauri packaging command:\n%s", output)
}

func TestDesktopLinuxPackagingBuildsDebAndAppImage(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "-o", "desktop-binaries", "-o", "ui", "HOST_OS=Linux", "HOST_ARCH=x86_64", "PLATFORM=linux", "desktop")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Linux desktop packaging: %v\n%s", err, output)
	}

	line := findLine(string(output), "cargo tauri build")
	if line == "" {
		t.Fatalf("dry run did not contain the Linux Tauri packaging command:\n%s", output)
	}
	for _, required := range []string{"TARIBOY_VERSION=", "--bundles deb,appimage"} {
		if !strings.Contains(line, required) {
			t.Fatalf("Linux packaging is missing %q:\n%s", required, line)
		}
	}
	for _, forbidden := range []string{"CI=true", "app,dmg"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("Linux packaging contains macOS-only %q:\n%s", forbidden, line)
		}
	}
}

func TestDesktopPlatformDefaultsToTheNativeHost(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "-o", "desktop-tools-check", "-o", "desktop-lock-check", "-o", "desktop-binaries", "HOST_OS=Linux", "HOST_ARCH=x86_64", "desktop")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run native Desktop packaging: %v\n%s", err, output)
	}
	if line := findLine(string(output), "cargo tauri build"); !strings.Contains(line, "--bundles deb,appimage") {
		t.Fatalf("native Linux host did not default to Linux packages:\n%s", output)
	}
}

func TestDesktopBinariesCleanManagedPayloadDirectoriesBeforeBuilding(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command(
		"make", "-n", "-o", "desktop-version-check", "-o", "build-basic-image",
		"DESKTOP_DARWIN_BIN=/", "DESKTOP_LINUX_BIN=/tmp/untrusted-desktop-bin",
		"desktop-binaries",
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop payload build: %v\n%s", err, output)
	}

	text := string(output)
	clean := findLine(text, "rm -rf desktop/src-tauri/resources/bin/darwin-arm64 desktop/src-tauri/resources/bin/linux-x86_64")
	firstBuild := findLine(text, "build darwin/arm64")
	if clean == "" || firstBuild == "" || strings.Index(text, clean) > strings.Index(text, firstBuild) {
		t.Fatalf("managed payload directories are not cleaned before compilation:\n%s", output)
	}
}

func TestDesktopPlatformValidationPrecedesToolInstallation(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "HOST_OS=Linux", "HOST_ARCH=x86_64", "PLATFORM=windows", "desktop-preflight")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop preflight: %v\n%s", err, output)
	}

	text := string(output)
	validation := strings.Index(text, `case "windows" in`)
	toolInstall := strings.Index(text, "cargo install tauri-cli")
	if validation < 0 || toolInstall < 0 || validation > toolInstall {
		t.Fatalf("platform validation does not precede tool installation:\n%s", output)
	}
}

func TestDesktopPlatformPreflightRejectsUnknownAndMismatchedValues(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	for _, testCase := range []struct {
		name     string
		platform string
		hostOS   string
		hostArch string
	}{
		{name: "unknown", platform: "windows", hostOS: "Linux", hostArch: "x86_64"},
		{name: "mismatch", platform: "darwin", hostOS: "Linux", hostArch: "x86_64"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := exec.Command("make", "HOST_OS="+testCase.hostOS, "HOST_ARCH="+testCase.hostArch, "PLATFORM="+testCase.platform, "desktop-preflight")
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("preflight accepted PLATFORM=%s on %s-%s:\n%s", testCase.platform, testCase.hostOS, testCase.hostArch, output)
			}
			text := string(output)
			for _, required := range []string{"PLATFORM", "darwin", "linux", testCase.hostOS + "-" + testCase.hostArch} {
				if !strings.Contains(text, required) {
					t.Fatalf("preflight failure is missing %q:\n%s", required, text)
				}
			}
		})
	}
}

func TestDesktopE2EBuildProducesWebDriverExecutableWithoutPackaging(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "-o", "desktop-binaries", "-o", "ui", "desktop-e2e-build")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop E2E build: %v\n%s", err, output)
	}

	line := findLine(string(output), "cargo tauri build")
	if line == "" {
		t.Fatalf("dry run did not contain a Tauri build command:\n%s", output)
	}
	for _, required := range []string{"TARIBOY_VERSION=", "--debug", "--no-bundle"} {
		if !strings.Contains(line, required) {
			t.Fatalf("Desktop E2E build is missing %q:\n%s", required, line)
		}
	}
	if strings.Contains(line, "--bundles") {
		t.Fatalf("Desktop E2E build unexpectedly creates packages:\n%s", line)
	}
}

func TestDesktopE2ERunsDedicatedPlaywrightSuite(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command(
		"make", "-n", "-o", "desktop-e2e-build",
		"DESKTOP_E2E_ARGS=tests/desktop/cold-start.pw.ts", "desktop-e2e",
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop E2E suite: %v\n%s", err, output)
	}

	line := findLine(string(output), "test:desktop-e2e")
	if line == "" {
		t.Fatalf("dry run did not invoke the Desktop Playwright suite:\n%s", output)
	}
	if !strings.Contains(line, "tests/desktop/cold-start.pw.ts") {
		t.Fatalf("Desktop E2E arguments were not forwarded:\n%s", line)
	}
}

func TestDesktopE2EUsesBoundedParallelWorkersWithoutReinstallingDependencies(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command(
		"make", "-n", "-o", "desktop-e2e-tools-check", "-o", "desktop-tools-check",
		"-o", "desktop-binaries", "-o", "desktop-lock-check", "desktop-e2e",
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop E2E suite: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "ui/node_modules is missing") {
		t.Fatalf("Desktop E2E build does not require prepared UI dependencies:\n%s", output)
	}
	if strings.Contains(text, "cd ui && npm ci &&") {
		t.Fatalf("Desktop E2E build reinstalls UI dependencies:\n%s", output)
	}
	line := findLine(text, "test:desktop-e2e")
	if !strings.Contains(line, "--workers=2") {
		t.Fatalf("Desktop E2E suite does not select its bounded parallel worker count:\n%s", output)
	}

	config, err := os.ReadFile(filepath.Join(root, "ui", "playwright.desktop.config.ts"))
	if err != nil {
		t.Fatalf("read Desktop Playwright config: %v", err)
	}
	if !strings.Contains(string(config), "fullyParallel: true") {
		t.Fatalf("Desktop Playwright config does not permit independent specs to run in parallel:\n%s", config)
	}
}

func TestGoQualityTargetsExcludeNodeModulesPackages(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	cmd := exec.Command("make", "-n", "fmt-check", "vet", "test", "check")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Go quality targets: %v\n%s", err, output)
	}

	text := string(output)
	for _, command := range []string{"go test ./...", "go vet ./..."} {
		if strings.Contains(text, command) {
			t.Fatalf("Go quality target still recursively scans all directories (%q):\n%s", command, output)
		}
	}
	if !strings.Contains(text, "node_modules") {
		t.Fatalf("Go quality target does not explicitly filter node_modules packages:\n%s", output)
	}
}

func TestGoQualityTargetsFailWhenPackageEnumerationFails(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	fakeGo := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nif [ \"$1\" = list ]; then echo partial-package; echo list-failed >&2; exit 17; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write failing Go wrapper: %v", err)
	}

	for _, target := range []string{"test", "vet", "fmt-check"} {
		cmd := exec.Command("make", "GO="+fakeGo, target)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("%s succeeded after go list failed:\n%s", target, output)
		}
		if !strings.Contains(string(output), "list-failed") {
			t.Fatalf("%s hid the go list failure:\n%s", target, output)
		}
	}
}

func TestDesktopE2EDaemonWrapperRejectsLiveListener(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := filepath.Dir(packageDir)
	wrapper := filepath.Join(root, "scripts", "desktop-e2e-daemon.sh")
	temp := t.TempDir()
	fakeDaemon := filepath.Join(temp, "tariboyd")
	fakeSource := `#!/bin/sh
printf 'base=%s\nruntime=%s\nshell_env=%s\n' "$TARIBOY_BASE_DIR" "$TARIBOY_RUNTIME_DIR" "${TARIBOY_SHELL_ENV:-}"
printf 'arg=<%s>\n' "$@"
`
	if err := os.WriteFile(fakeDaemon, []byte(fakeSource), 0o700); err != nil {
		t.Fatalf("write fake daemon: %v", err)
	}

	base := filepath.Join(temp, "base")
	runtime := filepath.Join(temp, "runtime")
	cmd := exec.Command(wrapper, "--http-addr", "127.0.0.1:32123", "daemon", "status")
	cmd.Env = append(os.Environ(),
		"TARIBOY_REAL_DAEMON_BIN="+fakeDaemon,
		"TARIBOY_BASE_DIR="+base,
		"TARIBOY_RUNTIME_DIR="+runtime,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run daemon wrapper: %v\n%s", err, output)
	}
	want := strings.Join([]string{
		"base=" + base,
		"runtime=" + runtime,
		"shell_env=1",
		"arg=<--http-addr>",
		"arg=<127.0.0.1:32123>",
		"arg=<daemon>",
		"arg=<status>",
		"",
	}, "\n")
	if string(output) != want {
		t.Fatalf("wrapper invocation mismatch (-want +got):\nwant:\n%s\ngot:\n%s", want, output)
	}

	live := exec.Command(wrapper, "--http-addr", "127.0.0.1:9990")
	live.Env = cmd.Env
	liveOutput, err := live.CombinedOutput()
	if err == nil {
		t.Fatalf("wrapper accepted the live listener:\n%s", liveOutput)
	}
	if !strings.Contains(string(liveOutput), "refusing live Tariboy listener") {
		t.Fatalf("wrapper returned an unactionable live-listener error:\n%s", liveOutput)
	}

	for _, args := range [][]string{
		{"--http-addr=127.0.0.1:9990"},
		{"--web-addr", "127.0.0.1:9990"},
		{"--web-addr=127.0.0.1:9990"},
	} {
		attempt := exec.Command(wrapper, args...)
		attempt.Env = cmd.Env
		if output, err := attempt.CombinedOutput(); err == nil || !strings.Contains(string(output), "refusing live Tariboy listener") {
			t.Fatalf("wrapper accepted live listener form %q (err=%v):\n%s", args, err, output)
		}
	}

	liveBase := filepath.Join(temp, "live-base")
	if err := os.Symlink(filepath.Join(os.Getenv("HOME"), ".tariboy"), liveBase); err != nil {
		t.Fatalf("create live-state symlink: %v", err)
	}
	stateAttempt := exec.Command(wrapper)
	stateAttempt.Env = append(os.Environ(),
		"TARIBOY_REAL_DAEMON_BIN="+fakeDaemon,
		"TARIBOY_BASE_DIR="+liveBase+string(os.PathSeparator),
		"TARIBOY_RUNTIME_DIR="+runtime,
	)
	if output, err := stateAttempt.CombinedOutput(); err == nil || !strings.Contains(string(output), "refusing live Tariboy state directory") {
		t.Fatalf("wrapper accepted symlinked live state (err=%v):\n%s", err, output)
	}
}

func findLine(output, fragment string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, fragment) {
			return line
		}
	}
	return ""
}

// The three host binaries the Desktop E2E run needs are looked up by
// ui/tests/desktop/fixture.ts in worker scope, i.e. only after the whole build
// is done. desktop-e2e-tools-check moves that lookup in front of the first
// expensive step, so these tests pin both the ordering and the lookup rules.
func desktopToolsCheck(t *testing.T, env ...string) (string, error) {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	cmd := exec.Command("make", "desktop-e2e-tools-check")
	cmd.Dir = filepath.Dir(packageDir)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestDesktopE2EToolsCheckPrecedesEveryExpensiveStep(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	cmd := exec.Command("make", "-n", "desktop-e2e-build")
	cmd.Dir = filepath.Dir(packageDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run Desktop E2E build: %v\n%s", err, output)
	}

	text := string(output)
	check := strings.Index(text, "desktop e2e tools ok")
	if check < 0 {
		t.Fatalf("Desktop E2E build does not check for the WebDriver tools at all:\n%s", text)
	}
	for _, expensive := range []string{"go build", "cargo tauri build"} {
		at := strings.Index(text, expensive)
		if at < 0 {
			t.Fatalf("dry run did not contain %q:\n%s", expensive, text)
		}
		if check > at {
			t.Fatalf("%q runs before the Desktop E2E tools check:\n%s", expensive, text)
		}
	}
}

// An executable file the check is allowed to accept, so a test pins the tools
// it is not exercising instead of resolving them through the host PATH.
func desktopToolsStandIn(t *testing.T) string {
	t.Helper()
	stand := filepath.Join(t.TempDir(), "stand-in")
	if err := os.WriteFile(stand, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write stand-in binary: %v", err)
	}
	return stand
}

func TestDesktopE2EToolsCheckNamesTheMissingToolAndHowToInstallIt(t *testing.T) {
	variables := []string{"TAURI_DRIVER_BIN", "WEBKIT_WEBDRIVER_BIN", "TARIBOY_XVFB_BIN"}
	for _, testCase := range []struct{ name, env, hint string }{
		{"tauri-driver", "TAURI_DRIVER_BIN", "cargo install tauri-driver"},
		{"WebKitWebDriver", "WEBKIT_WEBDRIVER_BIN", "apt-get install -y webkit2gtk-driver"},
		{"Xvfb", "TARIBOY_XVFB_BIN", "apt-get install -y xvfb"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			// The check stops at the first missing tool. Setting only the
			// variable under test would leave the earlier ones to the host
			// PATH, so on a host without them the subcase fails naming
			// somebody else's binary — and drags make test down with it.
			stand := desktopToolsStandIn(t)
			missing := filepath.Join(t.TempDir(), "nonexistent")
			env := []string{"DISPLAY="}
			for _, variable := range variables {
				value := stand
				if variable == testCase.env {
					value = missing
				}
				env = append(env, variable+"="+value)
			}

			output, err := desktopToolsCheck(t, env...)
			if err == nil {
				t.Fatalf("tools check accepted a missing %s:\n%s", testCase.name, output)
			}
			if !strings.Contains(output, testCase.name) || !strings.Contains(output, missing) {
				t.Fatalf("failure does not name %s or the path it tried:\n%s", testCase.name, output)
			}
			if !strings.Contains(output, testCase.hint) {
				t.Fatalf("failure does not say how to install %s:\n%s", testCase.name, output)
			}
		})
	}
}

// With every override pinned above, nothing exercises the other half of the
// lookup rule: no variable set at all means the bare name is searched in PATH.
// An empty PATH keeps that hermetic — the failure must come from the search,
// not from whatever the host happens to have installed.
func TestDesktopE2EToolsCheckFallsBackToPathWhenNoOverrideIsSet(t *testing.T) {
	output, err := desktopToolsCheck(t,
		"DISPLAY=",
		"TAURI_DRIVER_BIN=",
		"WEBKIT_WEBDRIVER_BIN=",
		"TARIBOY_XVFB_BIN=",
		"PATH="+t.TempDir(),
	)
	if err == nil {
		t.Fatalf("tools check resolved a tool through an empty PATH:\n%s", output)
	}
	if !strings.Contains(output, "tauri-driver") || !strings.Contains(output, "not found in PATH") {
		t.Fatalf("failure does not name the tool missing from PATH:\n%s", output)
	}
	if !strings.Contains(output, "cargo install tauri-driver") {
		t.Fatalf("failure does not say how to install tauri-driver:\n%s", output)
	}
}

func TestDesktopE2EToolsCheckHonoursTheSameOverridesAsTheFixture(t *testing.T) {
	stand := desktopToolsStandIn(t)
	overrides := []string{
		"TAURI_DRIVER_BIN=" + stand,
		"WEBKIT_WEBDRIVER_BIN=" + stand,
		"TARIBOY_XVFB_BIN=" + stand,
	}

	output, err := desktopToolsCheck(t, append([]string{"DISPLAY="}, overrides...)...)
	if err != nil {
		t.Fatalf("tools check rejected overrides the fixture would accept: %v\n%s", err, output)
	}
	if strings.Count(output, stand) != len(overrides) {
		t.Fatalf("tools check did not resolve all three tools through their overrides:\n%s", output)
	}

	// startDisplay() only reaches for Xvfb when DISPLAY is empty, so demanding it
	// on a host that already has an X server would break a working setup.
	withDisplay := []string{"DISPLAY=:99", overrides[0], overrides[1], "TARIBOY_XVFB_BIN=" + filepath.Join(t.TempDir(), "nonexistent")}
	if output, err = desktopToolsCheck(t, withDisplay...); err != nil {
		t.Fatalf("tools check demanded Xvfb while DISPLAY is set: %v\n%s", err, output)
	}
}
