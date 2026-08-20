package plugins

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBackoffCapped(t *testing.T) {
	if backoff(1) != 500*time.Millisecond {
		t.Fatalf("backoff(1) = %v", backoff(1))
	}
	if backoff(2) != time.Second || backoff(3) != 2*time.Second {
		t.Fatalf("backoff growth wrong: %v %v", backoff(2), backoff(3))
	}
	if backoff(100) != 30*time.Second {
		t.Fatalf("backoff not capped: %v", backoff(100))
	}
}

// fakeHandle is an in-memory ProcHandle the fake Runner returns.
type fakeHandle struct {
	hs    chan HandshakeResult
	done  chan struct{}
	mu    sync.Mutex
	stops int
}

func (f *fakeHandle) Handshake() <-chan HandshakeResult { return f.hs }
func (f *fakeHandle) Done() <-chan struct{}             { return f.done }
func (f *fakeHandle) Pid() int                          { return 4242 }
func (f *fakeHandle) Stop(time.Duration) error {
	f.mu.Lock()
	f.stops++
	f.mu.Unlock()
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

type fakeRunner struct {
	line string
	last *fakeHandle
}

func (r *fakeRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	h.hs <- HandshakeResult{Line: []byte(r.line)}
	r.last = h
	return h, nil
}

// healthServer starts a unix-socket /health server the supervisor probes.
func healthServer(t *testing.T, sock string, status *int) {
	t.Helper()
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(*status) })
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
}

func TestSupervisorReachesRunningThenStopsOnCancel(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "p.sock")
	status := 200
	healthServer(t, sock, &status)
	m, _ := ParseManifest([]byte(goodManifest))
	line := `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + sock + `"}`
	states := make(chan string, 8)
	sup := NewSupervisor(SupervisorConfig{
		Name: "echo", Manifest: m,
		Spec:    SpawnSpec{Name: "echo", Socket: sock},
		Runner:  &fakeRunner{line: line},
		Client:  NewClient(sock),
		OnState: func(state string, _ map[string]any) { states <- state },
		Clock:   func() time.Time { return time.Unix(0, 0) },
		After:   func(time.Duration) <-chan time.Time { return time.After(time.Millisecond) },
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()
	if got := <-states; got != "running" {
		t.Fatalf("first state = %q, want running", got)
	}
	cancel()
	<-done
	select {
	case s := <-states:
		if s != "stopped" {
			t.Fatalf("last state = %q, want stopped", s)
		}
	default:
		t.Fatal("no stopped state emitted on cancel")
	}
}

func TestExecRunnerRealProcess(t *testing.T) {
	// Drive the real execRunner against the re-exec fake plugin.
	sock := filepath.Join(t.TempDir(), "real.sock")
	spec := SpawnSpec{
		Name: "echo", Exec: os.Args[0], Dir: t.TempDir(), Socket: sock,
		Env: append(os.Environ(),
			"TARIBOY_FAKE_PLUGIN=1",
			"TARIBOY_PLUGIN_NAME=echo",
			"TARIBOY_PLUGIN_SOCKET="+sock),
	}
	h, err := NewExecRunner().Start(spec, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case hr := <-h.Handshake():
		if hr.Err != nil {
			t.Fatalf("handshake err: %v", hr.Err)
		}
		hsk, err := ReadHandshake(bytesReader(hr.Line))
		if err != nil || hsk.Name != "echo" {
			t.Fatalf("handshake = %+v err=%v", hsk, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no handshake from real process")
	}
	// The plugin should be serving /health on its socket.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := NewClient(sock).Health(context.Background()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin never became healthy")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := h.Stop(time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process not reaped after Stop")
	}
}

// sequencedRunner returns a different fakeHandle behaviour on each successive
// Start() call: the first two lives fail the handshake, the third reaches
// running and then exits (simulating a healthy-then-crashed plugin), and any
// further life hangs in the handshake select until the caller cancels ctx.
type sequencedRunner struct {
	mu     sync.Mutex
	n      int
	okLine string
}

func (r *sequencedRunner) Start(spec SpawnSpec, logw io.Writer) (ProcHandle, error) {
	r.mu.Lock()
	r.n++
	n := r.n
	r.mu.Unlock()

	h := &fakeHandle{hs: make(chan HandshakeResult, 1), done: make(chan struct{})}
	switch {
	case n <= 2:
		h.hs <- HandshakeResult{Err: errors.New("boom")}
	case n == 3:
		h.hs <- HandshakeResult{Line: []byte(r.okLine)}
		go func() {
			time.Sleep(20 * time.Millisecond)
			close(h.done)
		}()
	default:
		// Hang: no handshake ever arrives and the process never exits, so
		// this life blocks on ctx.Done() until the test cancels.
	}
	return h, nil
}

// TestBackoffResetsAfterHealthyLife is FINDING 1: a plugin that crashes a few
// times, then reaches "running" and stays healthy for a while before crashing
// again, must restart at the backoff FLOOR, not at the grown value the early
// crashes accumulated.
func TestBackoffResetsAfterHealthyLife(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "p.sock")
	status := 200
	healthServer(t, sock, &status)
	m, _ := ParseManifest([]byte(goodManifest))
	line := `{"name":"echo","version":"0.1.0","types":["channel-sink"],"protocol_version":1,"socket":"` + sock + `"}`
	r := &sequencedRunner{okLine: line}

	const handshakeTimeout = 777 * time.Millisecond
	const healthInterval = 999 * time.Millisecond

	var mu sync.Mutex
	var recorded []time.Duration
	fakeAfter := func(d time.Duration) <-chan time.Time {
		mu.Lock()
		recorded = append(recorded, d)
		mu.Unlock()
		ch := make(chan time.Time, 1)
		if d != handshakeTimeout && d != healthInterval {
			// A backoff sleep (not a handshake/health-interval timer): fire
			// immediately so the test runs fast.
			ch <- time.Now()
		}
		return ch
	}

	sup := NewSupervisor(SupervisorConfig{
		Name: "echo", Manifest: m,
		Spec:             SpawnSpec{Name: "echo", Socket: sock},
		Runner:           r,
		Client:           NewClient(sock),
		OnState:          func(string, map[string]any) {},
		After:            fakeAfter,
		Log:              discardLog,
		HandshakeTimeout: handshakeTimeout,
		HealthInterval:   healthInterval,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sup.Run(ctx); close(done) }()

	// backoffDelays filters the recorded After() calls down to the ones that
	// are backoff sleeps (as opposed to the handshake-timeout/health-interval
	// timers, which use distinct sentinel durations above).
	backoffDelays := func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		var vals []time.Duration
		for _, d := range recorded {
			if d != handshakeTimeout && d != healthInterval {
				vals = append(vals, d)
			}
		}
		return vals
	}

	deadline := time.After(3 * time.Second)
	for {
		vals := backoffDelays()
		if len(vals) >= 3 {
			if vals[0] != 500*time.Millisecond {
				t.Fatalf("life1 backoff = %v, want 500ms (floor)", vals[0])
			}
			if vals[1] != time.Second {
				t.Fatalf("life2 backoff = %v, want 1s (grown)", vals[1])
			}
			if vals[2] != 500*time.Millisecond {
				t.Fatalf("post-healthy-life backoff = %v, want floor 500ms (attempt not reset)", vals[2])
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for 3 backoff delays, got %d: %v", len(vals), vals)
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	<-done
}

// TestHandshakeLineTooLong is FINDING 2: a plugin stdout stream that never
// emits a newline must not be read without bound. readHandshake must give up
// once the cap is hit and report a clear error instead of blocking/growing
// memory forever.
type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func TestHandshakeLineTooLong(t *testing.T) {
	h := &execHandle{hs: make(chan HandshakeResult, 1)}
	done := make(chan struct{})
	go func() {
		h.readHandshake(infiniteReader{}, io.Discard)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readHandshake did not return for an unbounded stream (unbounded read)")
	}
	select {
	case hr := <-h.hs:
		if hr.Err == nil {
			t.Fatalf("expected an error for an oversized handshake line, got line %q", hr.Line)
		}
	default:
		t.Fatal("readHandshake returned without sending a HandshakeResult")
	}
}

func bytesReader(b []byte) *stringsReader { return &stringsReader{b: b} }

type stringsReader struct {
	b []byte
	i int
}

func (r *stringsReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
