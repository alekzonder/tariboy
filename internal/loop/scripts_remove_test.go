package loop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/store"
)

type scriptRemovalFixture struct {
	m       *Manager
	st      *script.Store
	raw     *store.Store
	logsDir string
}

func newScriptRemovalFixture(t *testing.T) *scriptRemovalFixture {
	t.Helper()
	m, as, _, raw := newManager(t, &fakeRunner{})
	st := script.NewStore(raw, time.Now)
	m.cfg.Scripts = st
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "basic:latest", Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	logsDir := filepath.Join(agentdir.New(m.cfg.AgentsDir, "worker").Root, "scripts")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return &scriptRemovalFixture{m: m, st: st, raw: raw, logsDir: logsDir}
}

func (f *scriptRemovalFixture) finish(t *testing.T, runID, logPath string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if claimed, err := f.st.ClaimRun("worker", runID, now, logPath); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	exit := 0
	if _, err := f.st.CompleteRun("worker", runID, script.Completion{Status: script.RunSucceeded, ExitCode: &exit, FinishedAt: now, LogPath: logPath}); err != nil {
		t.Fatal(err)
	}
}

// script creates a finished definition with one finished run per log name. A
// name is created as a log and a state file unless it starts with "missing";
// an empty name is a run without a log.
func (f *scriptRemovalFixture) script(t *testing.T, names ...string) (string, []string) {
	t.Helper()
	definition, run, err := f.st.CreateOnce("worker", script.CreateOnce{Name: "s", Description: "test", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for i, name := range names {
		if i > 0 {
			if run, err = f.st.Rerun("worker", definition.ID); err != nil {
				t.Fatal(err)
			}
		}
		path := ""
		if name != "" {
			path = filepath.Join(f.logsDir, name+".log")
			paths = append(paths, path)
			if !strings.HasPrefix(name, "missing") {
				if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(scriptStatePath(path), []byte("state"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		f.finish(t, run.ID, path)
	}
	return definition.ID, paths
}

func (f *scriptRemovalFixture) entries(t *testing.T) []string {
	t.Helper()
	items, err := os.ReadDir(f.logsDir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name())
	}
	return names
}

func (f *scriptRemovalFixture) wantKept(t *testing.T, id string, paths ...string) {
	t.Helper()
	if _, err := f.st.GetDefinition("worker", id); err != nil {
		t.Fatalf("definition was removed: %v", err)
	}
	for _, path := range paths {
		for _, name := range []string{path, scriptStatePath(path)} {
			if _, err := os.Stat(name); err != nil {
				t.Fatalf("%s was not kept: %v", filepath.Base(name), err)
			}
		}
	}
	for _, name := range f.entries(t) {
		if strings.Contains(name, ".deleting-") {
			t.Fatalf("quarantine file left behind: %s", name)
		}
	}
}

func TestRemoveScriptWithoutStore(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newManager(t, &fakeRunner{})
	m.cfg.Scripts = nil
	if err := m.scripts.RemoveScript("worker", "id"); err == nil || err.Error() != "script store not configured" {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveScriptUnknownDefinition(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	if err := f.m.scripts.RemoveScript("worker", "missing"); !errors.Is(err, script.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveScriptDeletesLogsAndStateOfEveryRun(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, _ := f.script(t, "first", "", "missing-log", "second")
	if err := os.WriteFile(filepath.Join(f.logsDir, "missing-log.state"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.logsDir, "other.log"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.m.scripts.RemoveScript("worker", id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.GetDefinition("worker", id); !errors.Is(err, script.ErrNotFound) {
		t.Fatalf("definition still exists: %v", err)
	}
	if got := f.entries(t); len(got) != 1 || got[0] != "other.log" {
		t.Fatalf("scripts directory = %q, want only other.log", got)
	}
}

func TestRemoveScriptRejectsActiveRunOfInactiveDefinition(t *testing.T) {
	t.Parallel()
	for _, status := range []string{script.RunPending, script.RunRunning} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			f := newScriptRemovalFixture(t)
			id, paths := f.script(t, "done", "busy")
			if _, err := f.raw.DB.Exec(`UPDATE script_runs SET status=? WHERE agent='worker' AND log_path=?`, status, paths[1]); err != nil {
				t.Fatal(err)
			}
			err := f.m.scripts.RemoveScript("worker", id)
			want := fmt.Sprintf("cannot remove script %q with an active run: %v", id, script.ErrActive)
			if !errors.Is(err, script.ErrActive) || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
			f.wantKept(t, id, paths...)
		})
	}
}

func TestRemoveScriptRejectsLogOutsideScriptsDirectoryBeforeMoving(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, paths := f.script(t, "inside", "outside")
	outside := filepath.Join(t.TempDir(), "outside.log")
	if _, err := f.raw.DB.Exec(`UPDATE script_runs SET log_path=? WHERE agent='worker' AND log_path=?`, outside, paths[1]); err != nil {
		t.Fatal(err)
	}
	err := f.m.scripts.RemoveScript("worker", id)
	if err == nil || err.Error() != "script log path is outside the agent scripts directory" {
		t.Fatalf("error = %v", err)
	}
	f.wantKept(t, id, paths[0])
}

func TestRemoveScriptRollsBackWhenAQuarantinePathExists(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	clock := time.Unix(100, 0)
	f.m.cfg.Clock = func() time.Time { return clock }
	id, paths := f.script(t, "first", "second")
	runs, err := f.st.ListRuns("worker", id)
	if err != nil {
		t.Fatal(err)
	}
	quarantine := fmt.Sprintf("%s.deleting-%d-1", runs[1].LogPath, clock.UnixNano())
	if err := os.WriteFile(quarantine, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = f.m.scripts.RemoveScript("worker", id)
	if err == nil || err.Error() != "script log quarantine path already exists: "+quarantine {
		t.Fatalf("error = %v", err)
	}
	if err := os.Remove(quarantine); err != nil {
		t.Fatal(err)
	}
	f.wantKept(t, id, paths...)
}

func TestRemoveScriptRollsBackWhenQuarantineCannotBeInspected(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, paths := f.script(t, "first", strings.Repeat("n", 240))
	err := f.m.scripts.RemoveScript("worker", id)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), "file name too long") {
		t.Fatalf("error = %v", err)
	}
	f.wantKept(t, id, paths...)
}

func TestRemoveScriptReportsUnreadableLogBeforeMoving(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, paths := f.script(t, "first")
	if err := os.Chmod(f.logsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.logsDir, 0o700) })
	if err := f.m.scripts.RemoveScript("worker", id); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v", err)
	}
	if err := os.Chmod(f.logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	f.wantKept(t, id, paths...)
}

func TestRemoveScriptReportsFailedQuarantineRename(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, paths := f.script(t, "first")
	if err := os.Chmod(f.logsDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.logsDir, 0o700) })
	if err := f.m.scripts.RemoveScript("worker", id); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v", err)
	}
	f.wantKept(t, id, paths...)
}

func TestRemoveScriptRollsBackWhenDefinitionRemovalFails(t *testing.T) {
	t.Parallel()
	f := newScriptRemovalFixture(t)
	id, paths := f.script(t, "first", "second")
	if _, err := f.raw.DB.Exec(`CREATE TRIGGER refuse_script_delete BEFORE DELETE ON scripts BEGIN SELECT RAISE(ABORT, 'refused by test'); END`); err != nil {
		t.Fatal(err)
	}
	err := f.m.scripts.RemoveScript("worker", id)
	if err == nil || !strings.Contains(err.Error(), "refused by test") {
		t.Fatalf("error = %v", err)
	}
	f.wantKept(t, id, paths...)
}
