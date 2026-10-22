package main

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/testutil"
)

// tariboy tasks runs the ttasks client in operator mode: an agent's
// TARIBOY_TOOLS_SOCKET does not narrow it to the agent's tasks.
func TestTasksRunsTaskClientInOperatorMode(t *testing.T) {
	isolate(t)
	t.Setenv("TARIBOY_TOOLS_SOCKET", filepath.Join(t.TempDir(), "agent.sock"))
	sock := filepath.Join(testutil.ShortDir(t), "d.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var routes []string
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		routes = append(routes, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"tasks":[]}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	out, errOut, code := runCLI(t, "--socket", sock, "tasks", "list", "--json")
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(routes) != 1 || routes[0] != "GET /api/tasks" {
		t.Fatalf("routes = %q, want the operator list route", routes)
	}
}

func TestTasksHelpIsTheTaskClientHelp(t *testing.T) {
	isolate(t)
	for _, argv := range [][]string{{"tasks", "--help"}, {"help", "tasks"}, {"tasks"}, {"tasks", "activity", "--help"}} {
		out, errOut, code := runCLI(t, argv...)
		if code != 0 || !strings.Contains(out, "Usage: tariboy tasks") {
			t.Errorf("%q: code=%d stdout=%q stderr=%q", argv, code, out, errOut)
		}
	}
	out, _, _ := runCLI(t, "--help")
	if !strings.Contains(out, "\n  tasks ") {
		t.Errorf("tariboy --help lost the tasks group:\n%s", out)
	}
}
