package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/testutil"
)

// startIsolatedDaemon runs the daemon on base with no HTTP listener and
// returns its client and a stop function that waits for a clean exit.
func startIsolatedDaemon(t *testing.T, base string) (*client.Client, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, daemonTestOptions(Options{BaseDir: base, Listen: "unix", LogLevel: "error", HTTPAddr: ""}))
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("daemon shutdown: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("daemon did not shut down")
			}
		})
	}
	t.Cleanup(stop)
	waitDaemonReady(t, base)
	return client.New(paths.New(base).Socket()), stop
}

func TestDaemonStartRemovesHandoffMarker(t *testing.T) {
	base := t.TempDir()
	storetest.Seed(t, paths.New(base).DB())
	marker := paths.New(base).HandoffMarker()
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, stop := startIsolatedDaemon(t, base)
	defer stop()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("handoff marker after daemon start: stat err = %v", err)
	}
}

// TestDaemonHandoffPreservesScriptRunExitCode restarts an isolated daemon
// through the handoff marker while a script runs, and checks that the agent
// gets the script's own exit code from the second daemon.
func TestDaemonHandoffPreservesScriptRunExitCode(t *testing.T) {
	t.Setenv("TARIBOY_STUB_HARNESS", "/usr/bin/true")
	t.Setenv("TARIBOY_SHIM_BIN", os.Args[0])
	for _, code := range []int{0, 3} {
		t.Run("exit_"+strconv.Itoa(code), func(t *testing.T) {
			base := t.TempDir()
			storetest.Seed(t, paths.New(base).DB())
			buildBasicImage(t, filepath.Join(base, "images"))
			release := filepath.Join(t.TempDir(), "release")

			first, stopFirst := startIsolatedDaemon(t, base)
			if _, err := first.Call("POST", "/api/agents", map[string]any{
				"name": "scripter", "image": "basic:latest", "harness": "stub", "plugins": "context",
			}); err != nil {
				t.Fatal(err)
			}
			raw, err := first.Call("POST", "/api/agents/scripter/scripts/run", map[string]any{
				"script_name": "wait", "description": "handoff test",
				"command": `echo before; while [ ! -e "` + release + `" ]; do sleep 0.05; done; echo after; exit ` + strconv.Itoa(code),
			})
			if err != nil {
				t.Fatal(err)
			}
			var started struct {
				Run struct {
					ID string `json:"id"`
				} `json:"run"`
			}
			if err := json.Unmarshal(raw, &started); err != nil || started.Run.ID == "" {
				t.Fatalf("script run response %s: %v", raw, err)
			}
			runID := started.Run.ID
			logPath := filepath.Join(paths.New(base).AgentsDir(), "scripter", "scripts", runID+".log")
			if !testutil.WaitFor(10*time.Second, func() bool {
				out, _ := os.ReadFile(logPath)
				return strings.Contains(string(out), "before")
			}) {
				t.Fatal("script did not start")
			}

			if err := os.WriteFile(paths.New(base).HandoffMarker(), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			stopFirst()
			if status, _ := scriptRunRow(t, base, runID); status != "running" {
				t.Fatalf("run status after handoff stop = %q, want running", status)
			}

			_, stopSecond := startIsolatedDaemon(t, base)
			defer stopSecond()
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			want := "succeeded"
			if code != 0 {
				want = "failed"
			}
			var letter map[string]any
			if !testutil.WaitFor(15*time.Second, func() bool {
				letter = scriptResultLetter(t, base, runID)
				return letter != nil
			}) {
				status, exit := scriptRunRow(t, base, runID)
				t.Fatalf("no script.result letter; run status=%q exit=%v", status, exit)
			}
			if letter["status"] != want || letter["exit_code"] != float64(code) {
				t.Fatalf("script.result data = %#v, want %s with exit code %d", letter, want, code)
			}
			if out, _ := os.ReadFile(logPath); !strings.Contains(string(out), "after") {
				t.Fatalf("log = %q, want output written after the handoff", out)
			}
		})
	}
}

func scriptRunRow(t *testing.T, base, runID string) (string, *int) {
	t.Helper()
	st, err := store.Open(paths.New(base).DB())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var status string
	var exit *int
	if err := st.DB.QueryRow(`SELECT status, exit_code FROM script_runs WHERE id=?`, runID).Scan(&status, &exit); err != nil {
		t.Fatal(err)
	}
	return status, exit
}

func scriptResultLetter(t *testing.T, base, runID string) map[string]any {
	t.Helper()
	st, err := store.Open(paths.New(base).DB())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rows, err := st.DB.Query(`SELECT data FROM mail WHERE kind='script.result' AND recipient='agent:scripter'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if json.Unmarshal([]byte(raw), &data) == nil && data["run_id"] == runID {
			return data
		}
	}
	return nil
}
