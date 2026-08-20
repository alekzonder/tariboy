package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/shim"
)

// toolsHTTP dials the per-agent tools socket the way tariboy-tools does: a
// fresh connection per call (the client is a one-shot process), so a rebound
// socket is picked up by the next call without any reconnect logic.
func toolsHTTP(sock string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
}

// toolsCall performs one tools request and decodes the api envelope. It returns
// the transport error instead of failing, so callers can assert a socket is
// gone as well as that it answers.
func toolsCall(sock, method, path, body string) (int, map[string]any, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://tools"+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	resp, err := toolsHTTP(sock).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	var env struct {
		OK     bool           `json:"ok"`
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, env.Result, nil
}

// socketIno identifies the bound socket file itself: agentapi.Listen unlinks
// the path before binding, so a rebind always yields a different inode even
// though the path is stable.
func socketIno(t *testing.T, sock string) uint64 {
	t.Helper()
	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat tools socket: %v", err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("stat tools socket: no syscall.Stat_t")
	}
	return uint64(st.Ino)
}

// liveIterationOnDisk recreates the pre-restart state of an agent that is
// mid-iteration: an iteration dir, a live shim socket and a running iteration
// row, with no result.json (so the adoption stays pending, exactly as it does
// while the harness is still working).
func liveIterationOnDisk(t *testing.T, m *Manager, as *agent.Store, agentsDir, name string, enabled bool) (agentdir.Layout, string) {
	t.Helper()
	ag := agent.Agent{Name: name, ImageRef: "basic:latest", HarnessType: "stub",
		Enabled: enabled, LoopEnabled: enabled, IntervalS: 60, Plugins: []string{"whoami", "loop", "messages", "context", "status"}}
	pinBasicImage(t, m, &ag)
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(agentsDir, name).WithRuntime(m.cfg.RuntimeDir)
	id := name + "-20260806170459-1"
	if err := l.EnsureIteration(id); err != nil {
		t.Fatal(err)
	}
	serveStubShim(t, l.ShimSock()) // live shim: Status probes succeed, adoption never goes stale
	if err := as.CreateIteration(agent.Iteration{ID: id, Agent: name, Trigger: "manual",
		Status: "running", StartedAt: time.Now().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	return l, id
}

// SUPER-284: a daemon restart must rebind the per-agent tools socket for every
// agent that still owns an unfinished iteration — whether its engine is waiting
// on the adoption or (loop switched off, iteration launched by hand) never
// starts at all. Without the socket the harness loses tools/tasks AND i-am-done,
// so completed work is recorded as no_i_am_done.
//
// The assertion is on the answer to a request, not on the socket file: the file
// alone proves nothing about who, if anyone, is serving it.
func TestStartAllBindsToolsSocketForUnfinishedIterationWithoutEngine(t *testing.T) {
	cases := []struct {
		name    string
		agent   string
		enabled bool
	}{
		// Engine never starts: startAfter returns silently for a disabled agent.
		{name: "loop disabled", agent: "quiet", enabled: false},
		// Engine start is deferred until the adoption finishes, and the adoption
		// waits for a result.json the harness cannot produce without i-am-done.
		{name: "adoption pending", agent: "waiting", enabled: true},
	}
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	socks := map[string]agentdir.Layout{}
	ids := map[string]string{}
	for _, c := range cases {
		l, id := liveIterationOnDisk(t, m, as, agentsDir, c.agent, c.enabled)
		socks[c.agent], ids[c.agent] = l, id
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.StartAll(ctx); err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sock, id := socks[c.agent].Sock(), ids[c.agent]
			// The bind is synchronous in StartAll, so no polling: by the time
			// StartAll has returned the harness must already be able to call in.
			code, res, err := toolsCall(sock, "GET", "/tools/whoami", "")
			if err != nil {
				t.Fatalf("tools whoami over %s: %v", sock, err)
			}
			if code != http.StatusOK || res["agent"] != c.agent {
				t.Fatalf("tools whoami = %d %v, want 200 for agent %q", code, res, c.agent)
			}
			// The harness must be recognized as the caller of ITS iteration:
			// i-am-done resolves the id through the same accessor, so an empty
			// one turns every completed iteration into no_i_am_done.
			if res["iteration"] != id {
				t.Fatalf("tools whoami iteration = %v, want %q", res["iteration"], id)
			}
			code, res, err = toolsCall(sock, "POST", "/tools/loop/done", `{"idle":false}`)
			if err != nil {
				t.Fatalf("i-am-done over %s: %v", sock, err)
			}
			if code != http.StatusOK || res["iteration"] != id {
				t.Fatalf("i-am-done = %d %v, want 200 for iteration %q", code, res, id)
			}
			it, err := as.GetIteration(c.agent, id)
			if err != nil {
				t.Fatal(err)
			}
			if !it.DoneFlag {
				t.Fatalf("i-am-done did not mark the iteration done: %+v", it)
			}
		})
	}
}

type adoptedDoneGraceShim struct {
	resultPath string
	kills      atomic.Int32
}

func (*adoptedDoneGraceShim) Status() shim.StatusResult {
	return shim.StatusResult{Running: true, PID: 1}
}

func (s *adoptedDoneGraceShim) Kill() error {
	s.kills.Add(1)
	return os.WriteFile(s.resultPath, []byte(`{"exit_code":0}`), 0o600)
}

func (*adoptedDoneGraceShim) Screen() (string, error)            { return "", nil }
func (*adoptedDoneGraceShim) SendKeys(shim.SendKeysParams) error { return nil }
func (*adoptedDoneGraceShim) Report() shim.ReportResult          { return shim.ReportResult{} }

func TestAdoptedIterationHonorsDoneGrace(t *testing.T) {
	defer swapAdoptTiming(5*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond)()

	for _, idle := range []bool{false, true} {
		t.Run(map[bool]string{false: "productive", true: "idle"}[idle], func(t *testing.T) {
			m, as, agentsDir, _ := newManager(t, &fakeRunner{})
			m.cfg.DoneGrace = 20 * time.Millisecond
			t.Cleanup(m.Shutdown)
			ag := agent.Agent{Name: "adopted", ImageRef: "basic:latest", HarnessType: "stub", Plugins: []string{"loop"}}
			pinBasicImage(t, m, &ag)
			if err := as.Create(ag); err != nil {
				t.Fatal(err)
			}
			l := agentdir.New(agentsDir, ag.Name).WithRuntime(m.cfg.RuntimeDir)
			id := "adopted-iteration"
			if err := l.EnsureIteration(id); err != nil {
				t.Fatal(err)
			}
			if err := as.CreateIteration(agent.Iteration{ID: id, Agent: ag.Name, Trigger: "manual", Status: "running"}); err != nil {
				t.Fatal(err)
			}

			handler := &adoptedDoneGraceShim{resultPath: l.ResultPath(id)}
			ln, err := net.Listen("unix", l.ShimSock())
			if err != nil {
				t.Fatal(err)
			}
			go func() { _ = shim.Serve(ln, handler) }()
			t.Cleanup(func() { _ = ln.Close() })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := m.StartAll(ctx); err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			code, res, err := toolsCall(l.Sock(), "POST", "/tools/loop/done", fmt.Sprintf(`{"idle":%t}`, idle))
			if err != nil {
				t.Fatalf("i-am-done: %v", err)
			}
			if code != http.StatusOK || res["iteration"] != id {
				t.Fatalf("i-am-done = %d %v, want 200 for iteration %q", code, res, id)
			}

			deadline := time.Now().Add(time.Second)
			var it agent.Iteration
			for time.Now().Before(deadline) {
				it, err = as.GetIteration(ag.Name, id)
				if err == nil && it.Status != "running" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if it.Status != "done" || !it.DoneFlag || it.Productive == idle {
				t.Fatalf("adopted iteration = %+v, want done flag and productive=%t", it, !idle)
			}
			if got := handler.kills.Load(); got != 1 {
				t.Fatalf("kill calls = %d, want 1", got)
			}
			if _, ok := readResult(l.ResultPath(id)); !ok {
				t.Fatal("cooperative Kill did not produce result.json")
			}
			if elapsed := time.Since(started); elapsed < 15*time.Millisecond {
				t.Fatalf("kill happened before done grace elapsed: %v", elapsed)
			}
		})
	}
}

func TestAdoptedIterationDoneGraceAndSoftTimeoutSendOneKill(t *testing.T) {
	defer swapAdoptTiming(5*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond)()

	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	m.cfg.DoneGrace = 20 * time.Millisecond
	t.Cleanup(m.Shutdown)
	base := time.Now().UTC()
	var clockNanos atomic.Int64
	clockNanos.Store(base.UnixNano())
	m.cfg.Clock = func() time.Time { return time.Unix(0, clockNanos.Load()).UTC() }

	ag := agent.Agent{Name: "adopted", ImageRef: "basic:latest", HarnessType: "stub", Plugins: []string{"loop"}}
	pinBasicImage(t, m, &ag)
	if err := as.Create(ag); err != nil {
		t.Fatal(err)
	}
	l := agentdir.New(agentsDir, ag.Name).WithRuntime(m.cfg.RuntimeDir)
	id := "adopted-overlapping-timeout"
	if err := l.EnsureIteration(id); err != nil {
		t.Fatal(err)
	}
	if err := as.CreateIteration(agent.Iteration{ID: id, Agent: ag.Name, Trigger: "manual", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := as.InitializeIterationTimeout(id, 1, 10, base); err != nil {
		t.Fatal(err)
	}

	handler := &adoptedDoneGraceShim{resultPath: l.ResultPath(id)}
	ln, err := net.Listen("unix", l.ShimSock())
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = shim.Serve(ln, handler) }()
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.StartAll(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _, err := toolsCall(l.Sock(), "POST", "/tools/loop/done", `{"idle":false}`); err != nil || code != http.StatusOK {
		t.Fatalf("i-am-done: code=%d err=%v", code, err)
	}
	// Let adoption observe the done flag while both deadlines are still pending,
	// then cross the grace and soft-timeout boundaries in the same poll.
	time.Sleep(50 * time.Millisecond)
	clockNanos.Store(base.Add(2 * time.Second).UnixNano())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		it, getErr := as.GetIteration(ag.Name, id)
		if getErr == nil && it.Status != "running" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := handler.kills.Load(); got != 1 {
		t.Fatalf("overlapping done grace and soft timeout sent %d Kill requests, want 1", got)
	}
}

// SUPER-284, the hazard the early bind introduces: agentapi.Listen unlinks the
// path before binding, so a second Listen on the same agent would leave the
// first listener serving an unlinked socket — a harness that already dialled it
// would keep talking to an orphan. The engine that starts after the early bind
// must therefore take over the very same server, and teardown must close and
// unlink exactly one.
func TestEngineStartReusesEarlyBoundToolsSocket(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{outcomes: []Outcome{{Status: "done", DoneFlag: true}}})
	l, id := liveIterationOnDisk(t, m, as, agentsDir, "adopted", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.StartAll(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _, err := toolsCall(l.Sock(), "GET", "/tools/whoami", ""); err != nil || code != http.StatusOK {
		t.Fatalf("early bound tools socket: code=%d err=%v", code, err)
	}
	early := socketIno(t, l.Sock())

	// Release the adoption: the engine now starts for real.
	if err := os.WriteFile(l.ResultPath(id),
		[]byte(`{"exit_code":0,"ended_at":"t","cpu_ms":3,"mem_peak_kb":4}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var rt *runtime
	for time.Now().Before(deadline) {
		m.mu.Lock()
		rt = m.runs["adopted"]
		m.mu.Unlock()
		if rt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rt == nil {
		t.Fatal("engine did not start after the adoption finished")
	}
	// A second Listen would unlink and recreate the path, so a changed inode is
	// proof that two listeners now exist for one agent — the orphan hazard.
	if got := socketIno(t, l.Sock()); got != early {
		t.Fatalf("tools socket rebound by the engine start: inode %d -> %d", early, got)
	}
	if code, _, err := toolsCall(l.Sock(), "GET", "/tools/whoami", ""); err != nil || code != http.StatusOK {
		t.Fatalf("tools socket after engine start: code=%d err=%v", code, err)
	}

	m.Shutdown()
	if _, err := os.Stat(l.Sock()); !os.IsNotExist(err) {
		t.Fatalf("tools socket left behind after Shutdown: err=%v", err)
	}
	if _, _, err := toolsCall(l.Sock(), "GET", "/tools/whoami", ""); err == nil {
		t.Fatal("tools socket still answers after Shutdown")
	}
}

// The early bind must not leak a listener for an agent that is then removed:
// Remove already tears down a running agent's server, and it owes the same to
// one that only ever had the early bind.
func TestRemoveClosesEarlyBoundToolsSocket(t *testing.T) {
	m, as, agentsDir, _ := newManager(t, &fakeRunner{})
	t.Cleanup(m.Shutdown)
	l, _ := liveIterationOnDisk(t, m, as, agentsDir, "quiet", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.StartAll(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _, err := toolsCall(l.Sock(), "GET", "/tools/whoami", ""); err != nil || code != http.StatusOK {
		t.Fatalf("early bound tools socket: code=%d err=%v", code, err)
	}
	if err := m.Remove("quiet", true, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := toolsCall(l.Sock(), "GET", "/tools/whoami", ""); err == nil {
		t.Fatal("tools socket still answers after Remove")
	}
}
