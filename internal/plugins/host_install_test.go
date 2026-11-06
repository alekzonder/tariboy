package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/testutil"
)

func writeInstallSource(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := fmt.Sprintf(`{"name":%q,"version":%q,"protocol_version":1,"types":["tool"],"exec":"run.sh"}`, name, version)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// failInserts makes the store refuse rows of table with this version.
func failInserts(t *testing.T, ps *Store, table, version string) {
	t.Helper()
	stmt := fmt.Sprintf(`CREATE TRIGGER fail_%s BEFORE INSERT ON %s WHEN NEW.version='%s'
		BEGIN SELECT RAISE(ABORT, 'forced %s failure'); END`, table, table, version, table)
	if _, err := ps.db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}

// blockMarker makes active-version a directory, which a rename cannot replace.
// A non-empty one also makes it impossible to remove.
func blockMarker(t *testing.T, h *Host, name string, removable bool) string {
	t.Helper()
	marker := filepath.Join(h.dir(name), activeVersionFile)
	if err := os.RemoveAll(marker); err != nil {
		t.Fatal(err)
	}
	child := marker
	if !removable {
		child = filepath.Join(marker, "child")
	}
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	return marker
}

func runningVersion(h *Host, name string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if rp := h.running[name]; rp != nil {
		return rp.rec.Version
	}
	return ""
}

func stopAndDrain(t *testing.T, h *Host, name string) {
	t.Helper()
	h.mu.Lock()
	rp := h.running[name]
	h.mu.Unlock()
	h.stop(name)
	if rp != nil {
		<-rp.done
	}
}

func TestHostInstallFirstVersionPersistFailureLeavesNothingActive(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, &nameRunner{})
	t.Cleanup(h.StopAll)
	failInserts(t, ps, "plugins", "1.0.0")

	info, err := h.Install(writeInstallSource(t, "fresh", "1.0.0"))
	if info != nil || err == nil || !strings.Contains(err.Error(), "forced plugins failure") || strings.Contains(err.Error(), "roll back") {
		t.Fatalf("Install = %v, %v", info, err)
	}
	if _, ok, err := ps.Get("fresh"); ok || err != nil {
		t.Fatalf("record survived: %v, %v", ok, err)
	}
	if v := runningVersion(h, "fresh"); v != "" {
		t.Fatalf("still running %q", v)
	}
	// The immutable copy stays: only the activation is undone.
	if _, err := os.Stat(filepath.Join(h.versionDir("fresh", "1.0.0"), "run.sh")); err != nil {
		t.Fatal(err)
	}
}

func TestHostInstallFirstVersionMarkerFailureUndoesTheRecord(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, &nameRunner{})
	t.Cleanup(h.StopAll)
	marker := blockMarker(t, h, "fresh", true)

	if _, err := h.Install(writeInstallSource(t, "fresh", "1.0.0")); err == nil || strings.Contains(err.Error(), "roll back") {
		t.Fatalf("Install error = %v", err)
	}
	if _, ok, err := ps.Get("fresh"); ok || err != nil {
		t.Fatalf("record survived: %v, %v", ok, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker survived: %v", err)
	}
}

func TestHostInstallReportsAFailedRollbackWithTheCause(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, &nameRunner{})
	t.Cleanup(h.StopAll)
	blockMarker(t, h, "fresh", false)

	_, err := h.Install(writeInstallSource(t, "fresh", "1.0.0"))
	if err == nil || !strings.Contains(err.Error(), "\nroll back plugin fresh: ") {
		t.Fatalf("Install error = %v", err)
	}
	if _, ok, err := ps.Get("fresh"); ok || err != nil {
		t.Fatalf("record survived: %v, %v", ok, err)
	}
}

func TestHostInstallUpgradePersistFailureRestartsPrevious(t *testing.T) {
	t.Parallel()
	for name, table := range map[string]string{
		"record":         "plugins",
		"active version": "plugin_active_versions",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, _, ps := newHost(t, &nameRunner{})
			t.Cleanup(h.StopAll)
			if _, err := h.Install(writeInstallSource(t, "up", "1.0.0")); err != nil {
				t.Fatal(err)
			}
			failInserts(t, ps, table, "2.0.0")

			if _, err := h.Install(writeInstallSource(t, "up", "2.0.0")); err == nil || !strings.Contains(err.Error(), "forced "+table+" failure") {
				t.Fatalf("Install error = %v", err)
			}
			if rec, ok, err := ps.Get("up"); err != nil || !ok || rec.Version != "1.0.0" || !rec.Enabled {
				t.Fatalf("record = %+v, %v, %v", rec, ok, err)
			}
			if active, ok, err := ps.ActiveVersion("up"); err != nil || !ok || active != "1.0.0" {
				t.Fatalf("active = %q, %v, %v", active, ok, err)
			}
			marker, err := os.ReadFile(filepath.Join(h.dir("up"), activeVersionFile))
			if err != nil || string(marker) != "1.0.0\n" {
				t.Fatalf("marker = %q, %v", marker, err)
			}
			testutil.WaitFor(2*time.Second, func() bool { return runningVersion(h, "up") == "1.0.0" })
			if v := runningVersion(h, "up"); v != "1.0.0" {
				t.Fatalf("running version = %q", v)
			}
		})
	}
}

// A record without an active version predates version directories. Rolling
// back clears what the failed install wrote, and there is nothing to restart.
func TestHostInstallRollbackOverLegacyRecordClearsActiveVersion(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, &nameRunner{})
	t.Cleanup(h.StopAll)
	if err := ps.Upsert(Record{Name: "legacy", Version: "1.0.0", Types: []string{"tool"}, ProtocolVersion: 1, Exec: "run.sh", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	marker := blockMarker(t, h, "legacy", true)
	if _, err := h.Install(writeInstallSource(t, "legacy", "2.0.0")); err == nil || strings.Contains(err.Error(), "roll back") {
		t.Fatalf("Install error = %v", err)
	}
	if rec, ok, err := ps.Get("legacy"); err != nil || !ok || rec.Version != "1.0.0" {
		t.Fatalf("record = %+v, %v, %v", rec, ok, err)
	}
	if active, ok, err := ps.ActiveVersion("legacy"); err != nil || ok {
		t.Fatalf("active = %q, %v, %v", active, ok, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker survived: %v", err)
	}
	if v := runningVersion(h, "legacy"); v != "" {
		t.Fatalf("legacy plugin restarted: %q", v)
	}
}

func TestHostInstallRollbackLeavesDisabledPreviousStopped(t *testing.T) {
	t.Parallel()
	h, _, ps := newHost(t, &nameRunner{})
	t.Cleanup(h.StopAll)
	if _, err := h.Install(writeInstallSource(t, "off", "1.0.0")); err != nil {
		t.Fatal(err)
	}
	stopAndDrain(t, h, "off")
	rec, ok, err := ps.Get("off")
	if err != nil || !ok {
		t.Fatalf("record = %v, %v", ok, err)
	}
	rec.Enabled = false
	if err := ps.Upsert(rec); err != nil {
		t.Fatal(err)
	}
	failInserts(t, ps, "plugin_active_versions", "2.0.0")

	if _, err := h.Install(writeInstallSource(t, "off", "2.0.0")); err == nil {
		t.Fatal("install unexpectedly succeeded")
	}
	if rec, ok, err := ps.Get("off"); err != nil || !ok || rec.Version != "1.0.0" || rec.Enabled {
		t.Fatalf("record = %+v, %v, %v", rec, ok, err)
	}
	if v := runningVersion(h, "off"); v != "" {
		t.Fatalf("disabled plugin restarted: %q", v)
	}
}

// A package that cannot be placed fails before the running version is touched.
func TestHostInstallPackageFailuresKeepTheRunningVersion(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		source func(t *testing.T) string
		want   string
	}{
		"exec missing": {
			source: func(t *testing.T) string {
				dir := writeInstallSource(t, "live", "2.0.0")
				if err := os.Remove(filepath.Join(dir, "run.sh")); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			want: "run.sh: no such file or directory",
		},
		"same version, other bytes": {
			source: func(t *testing.T) string {
				dir := writeInstallSource(t, "live", "1.0.0")
				if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("changed"), 0o700); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			want: "plugin live version 1.0.0 already exists with different bytes",
		},
		"unreadable file": {
			source: func(t *testing.T) string {
				if os.Geteuid() == 0 {
					t.Skip("root reads every file")
				}
				dir := writeInstallSource(t, "live", "2.0.0")
				if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("x"), 0o000); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			want: "permission denied",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, _, ps := newHost(t, &nameRunner{})
			t.Cleanup(h.StopAll)
			if _, err := h.Install(writeInstallSource(t, "live", "1.0.0")); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Install(tc.source(t)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Install error = %v, want %q", err, tc.want)
			}
			if v := runningVersion(h, "live"); v != "1.0.0" {
				t.Fatalf("running version = %q", v)
			}
			if active, ok, err := ps.ActiveVersion("live"); err != nil || !ok || active != "1.0.0" {
				t.Fatalf("active = %q, %v, %v", active, ok, err)
			}
			entries, err := os.ReadDir(h.dir("live"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".tmp-") {
					t.Fatalf("temporary copy left behind: %s", entry.Name())
				}
			}
		})
	}
}
