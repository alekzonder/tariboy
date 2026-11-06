package scriptcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

// -update-golden rewrites the expected request and output of every case in
// testdata/golden.json from the reference scripts.py named by
// TARIBOY_SCRIPTS_PY, run on the same fake daemon.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden.json from TARIBOY_SCRIPTS_PY")

const goldenPath = "testdata/golden.json"

type goldenCase struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	// Socket is how TARIBOY_TOOLS_SOCKET is set: "" (a fake daemon), "unset",
	// "empty" or "missing" (a path with no listener).
	Socket    string          `json:"socket,omitempty"`
	ToolsJSON string          `json:"tools_json,omitempty"`
	Response  *goldenResponse `json:"response,omitempty"`
	Request   *goldenRequest  `json:"request,omitempty"`
	Stdout    string          `json:"stdout"`
	Stderr    string          `json:"stderr"`
	Exit      int             `json:"exit"`
}

type goldenResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type goldenRequest struct {
	Method string          `json:"method"`
	Route  string          `json:"route"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type result struct {
	stdout, stderr string
	exit           int
}

// fakeDaemon answers every request on a Unix socket with one canned response
// and records the request it got.
type fakeDaemon struct {
	socket   string
	mu       sync.Mutex
	response *goldenResponse
	request  *goldenRequest
}

func startFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	// Keep the Unix socket path short on hosts with long test temp paths.
	dir, err := os.MkdirTemp("", "tscripts-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	d := &fakeDaemon{socket: filepath.Join(dir, "agent.sock")}
	listener, err := net.Listen("unix", d.socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(d.serve)}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return d
}

func (d *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.request = &goldenRequest{Method: r.Method, Route: r.URL.RequestURI()}
	if len(bytes.TrimSpace(body)) > 0 {
		d.request.Body = body
	}
	w.Header().Set("Content-Type", "application/json")
	if d.response == nil {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"unexpected","message":"unexpected request"}}`))
		return
	}
	w.WriteHeader(d.response.Status)
	_, _ = w.Write(append(d.response.Body, '\n'))
}

func (d *fakeDaemon) reset(response *goldenResponse) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.response, d.request = response, nil
}

func (d *fakeDaemon) taken() *goldenRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.request
}

// caseEnv is the environment of a case as a map; an absent key is unset.
func caseEnv(c goldenCase, d *fakeDaemon) map[string]string {
	env := map[string]string{}
	switch c.Socket {
	case "":
		env["TARIBOY_TOOLS_SOCKET"] = d.socket
	case "empty":
		env["TARIBOY_TOOLS_SOCKET"] = ""
	case "missing":
		env["TARIBOY_TOOLS_SOCKET"] = filepath.Join(filepath.Dir(d.socket), "missing.sock")
	}
	if c.ToolsJSON != "" {
		env["TARIBOY_TOOLS_JSON"] = c.ToolsJSON
	}
	return env
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func runGo(c goldenCase, env map[string]string) result {
	var stdout, stderr bytes.Buffer
	lookup := func(key string) (string, bool) { v, ok := env[key]; return v, ok }
	code := Run(append([]string(nil), c.Argv...), lookup, &stdout, &stderr)
	return result{stdout.String(), stderr.String(), code}
}

func runPython(t *testing.T, script string, c goldenCase, env map[string]string) result {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-B", script}, c.Argv...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "PYTHONUTF8=1", "TARIBOY_CLIENT_VERSION=golden"}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("%s: run scripts.py: %v", c.Name, err)
	}
	return result{stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()}
}

func sameJSON(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// TestGolden checks that tscripts prints what the reference scripts.py prints,
// byte for byte, and sends the daemon the same request.
func TestGolden(t *testing.T) {
	cases := loadGolden(t)
	daemon := startFakeDaemon(t)
	if *updateGolden {
		script := os.Getenv("TARIBOY_SCRIPTS_PY")
		if script == "" {
			t.Fatal("-update-golden needs TARIBOY_SCRIPTS_PY=<path to scripts.py>")
		}
		for i, c := range cases {
			daemon.reset(c.Response)
			got := runPython(t, script, c, caseEnv(c, daemon))
			cases[i].Request = daemon.taken()
			cases[i].Stdout, cases[i].Stderr, cases[i].Exit = got.stdout, got.stderr, got.exit
		}
		writeGolden(t, cases)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			daemon.reset(c.Response)
			got := runGo(c, caseEnv(c, daemon))
			want := result{c.Stdout, c.Stderr, c.Exit}
			if got != want {
				t.Errorf("argv %q\n got stdout %q stderr %q exit %d\nwant stdout %q stderr %q exit %d",
					c.Argv, got.stdout, got.stderr, got.exit, want.stdout, want.stderr, want.exit)
			}
			req := daemon.taken()
			switch {
			case (req == nil) != (c.Request == nil):
				t.Errorf("request %+v, want %+v", req, c.Request)
			case req != nil && (req.Method != c.Request.Method || req.Route != c.Request.Route || !sameJSON(req.Body, c.Request.Body)):
				t.Errorf("request %s %s %s, want %s %s %s", req.Method, req.Route, req.Body, c.Request.Method, c.Request.Route, c.Request.Body)
			}
		})
	}
}

func writeGolden(t *testing.T, cases []goldenCase) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cases); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVersion(t *testing.T) {
	for _, argv := range [][]string{{"--version"}, {"--version", "--json"}} {
		var stdout, stderr bytes.Buffer
		code := Run(argv, func(string) (string, bool) { return "", false }, &stdout, &stderr)
		if code != 0 || stdout.String() != version.Version+"\n" || stderr.Len() != 0 {
			t.Errorf("%q: exit %d stdout %q stderr %q", argv, code, stdout.String(), stderr.String())
		}
	}
}

func TestVersionWithOtherArgumentsIsUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer
	code := Run([]string{"--version", "ls"}, func(string) (string, bool) { return "", false }, io.Discard, &stderr)
	if code != 2 || !strings.HasPrefix(stderr.String(), "tools script: unknown command --version ls") {
		t.Errorf("exit %d stderr %q", code, stderr.String())
	}
}
