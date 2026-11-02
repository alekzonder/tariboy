package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/testutil"
)

// serveRoutes answers every request on a new unix socket with result and
// records "METHOD path?query".
func serveRoutes(t *testing.T, result string) (string, func() []string) {
	t.Helper()
	sock := filepath.Join(testutil.ShortDir(t), "d.sock")
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var routes []string
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		routes = append(routes, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return sock, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), routes...)
	}
}

func TestAssessmentRunsTheAssessmentClient(t *testing.T) {
	isolate(t)
	sock, routes := serveRoutes(t, `{"assessments":[]}`)
	out, errOut, code := runCLI(t, "--socket", sock, "assessment", "ls", "--agent", "dev", "--json")
	if got := routes(); code != 0 || len(got) != 1 || got[0] != "GET /api/assessments?agent=dev" {
		t.Fatalf("operator ls: code=%d stdout=%q stderr=%q routes=%q", code, out, errOut, got)
	}
	_, errOut, code = runCLI(t, "--socket", sock, "assessment", "add", "--file", "-")
	if code != 2 || !strings.Contains(errOut, "operators cannot record assessments") {
		t.Fatalf("operator add: code=%d stderr=%q", code, errOut)
	}

	tools, toolRoutes := serveRoutes(t, `{"assessments":[]}`)
	t.Setenv("TARIBOY_TOOLS_SOCKET", tools)
	file := filepath.Join(t.TempDir(), "a.json")
	if err := os.WriteFile(file, []byte(`{"summary":"s"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = runCLI(t, "assessment", "add", "--file", file, "--json")
	if got := toolRoutes(); code != 0 || len(got) != 1 || got[0] != "POST /tools/assessment/add?" {
		t.Fatalf("agent add: code=%d stdout=%q stderr=%q routes=%q", code, out, errOut, got)
	}
}
