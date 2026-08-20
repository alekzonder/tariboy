package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/shim"
	"github.com/coder/websocket"
)

// fakeTermControl is a minimal registry.ServiceControl used to exercise the
// websocket terminal route. Attach returns one end of a net.Pipe so the test
// can act as the shim PTY on the other end; Resize records its last call.
type fakeTermControl struct {
	conn      net.Conn // returned by Attach (one end of a pipe)
	attachErr error

	mu         sync.Mutex
	resizeCols int
	resizeRows int
	resized    bool
}

func (f *fakeTermControl) Run(registry.RunSpec) (string, error) { return "", nil }
func (f *fakeTermControl) Start(string) error                   { return nil }
func (f *fakeTermControl) Stop(string) error                    { return nil }
func (f *fakeTermControl) Restart(string) error                 { return nil }
func (f *fakeTermControl) Kill(string) error                    { return nil }
func (f *fakeTermControl) Remove(string, bool, bool) error      { return nil }
func (f *fakeTermControl) Reprovision(string, string) error     { return nil }
func (f *fakeTermControl) Exec(string, string) (string, error)  { return "", nil }
func (f *fakeTermControl) LiveState(string) (string, error)     { return "idle", nil }
func (f *fakeTermControl) Screen(string) (string, error)        { return "", nil }
func (f *fakeTermControl) SendKeys(string, string) error        { return nil }
func (f *fakeTermControl) SendKeysItems(string, []shim.KeyItem) error {
	return nil
}
func (f *fakeTermControl) ExtendIterationTimeout(string, string) (registry.IterationTimeoutExtension, error) {
	return registry.IterationTimeoutExtension{}, nil
}
func (f *fakeTermControl) Attach(_ string, _, _ int) (net.Conn, error) {
	if f.attachErr != nil {
		return nil, f.attachErr
	}
	return f.conn, nil
}
func (f *fakeTermControl) Resize(_ string, cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizeCols, f.resizeRows, f.resized = cols, rows, true
	return nil
}

func termServer(t *testing.T, ctrl registry.ServiceControl) *httptest.Server {
	t.Helper()
	reg := registry.New()
	cctx := &registry.Ctx{
		Version: "t",
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Control: ctrl,
	}
	srv := NewServer(reg, cctx)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func wsURL(base string) string { return "ws" + strings.TrimPrefix(base, "http") }

func TestServeTerminalBridges(t *testing.T) {
	shimEnd, handlerEnd := net.Pipe()
	ctrl := &fakeTermControl{conn: handlerEnd}
	ts := termServer(t, ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/api/agents/smoke/terminal?cols=80&rows=24", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	// (a) bytes written to the ws arrive on the shim pipe.
	if err := ws.Write(ctx, websocket.MessageBinary, []byte("hello")); err != nil {
		t.Fatalf("ws write: %v", err)
	}
	buf := make([]byte, 64)
	shimEnd.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := shimEnd.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("shim read = %q err=%v", string(buf[:n]), err)
	}

	// (b) bytes on the shim pipe arrive as a binary ws frame at the client.
	go func() {
		shimEnd.SetWriteDeadline(time.Now().Add(2 * time.Second))
		shimEnd.Write([]byte("world"))
	}()
	typ, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if typ != websocket.MessageBinary || string(data) != "world" {
		t.Fatalf("got typ=%v data=%q", typ, data)
	}

	// (c) a text frame {"cols":100,"rows":30} calls Resize(100,30).
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"cols":100,"rows":30}`)); err != nil {
		t.Fatalf("ws write text: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ctrl.mu.Lock()
		ok := ctrl.resized
		ctrl.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctrl.mu.Lock()
	if !ctrl.resized || ctrl.resizeCols != 100 || ctrl.resizeRows != 30 {
		ctrl.mu.Unlock()
		t.Fatalf("resize not applied: %+v", ctrl)
	}
	ctrl.mu.Unlock()
}

// TestServeTerminalCrossOrigin exercises the federated /terminals case: a
// browser on daemon A dialing daemon B's terminal websocket, where the
// Origin header (daemon A's origin) never matches Host (daemon B). Auth is
// the bearer token, not same-origin, so the upgrade must not be rejected by
// coder/websocket's default same-origin check.
func TestServeTerminalCrossOrigin(t *testing.T) {
	shimEnd, handlerEnd := net.Pipe()
	defer shimEnd.Close()
	ctrl := &fakeTermControl{conn: handlerEnd}
	ts := termServer(t, ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, resp, err := websocket.Dial(ctx, wsURL(ts.URL)+"/api/agents/smoke/terminal", &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Origin": {"https://other.example"}},
	})
	if err != nil {
		t.Fatalf("dial rejected (likely origin check): %v (status=%d)", err, statusOf(resp))
	}
	defer ws.CloseNow()
}

func statusOf(r *http.Response) int {
	if r == nil {
		return 0
	}
	return r.StatusCode
}

func TestServeTerminalNoSession(t *testing.T) {
	ctrl := &fakeTermControl{attachErr: errors.New("no interactive iteration")}
	ts := termServer(t, ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/api/agents/smoke/terminal", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	// Any read should fail with the 4404 close status.
	_, _, err = ws.Read(ctx)
	if websocket.CloseStatus(err) != 4404 {
		t.Fatalf("want close 4404, got err=%v (status=%d)", err, websocket.CloseStatus(err))
	}
}

func TestServeTerminalSessionEOF(t *testing.T) {
	shimEnd, handlerEnd := net.Pipe()
	ctrl := &fakeTermControl{conn: handlerEnd}
	ts := termServer(t, ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/api/agents/smoke/terminal", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	// Closing the shim stream models tmux/PTY completion. This is a terminal
	// lifecycle signal, distinct from a transport failure: the browser needs
	// both the normal status and the exact reason to suppress reconnects.
	if err := shimEnd.Close(); err != nil {
		t.Fatalf("close shim stream: %v", err)
	}
	_, _, err = ws.Read(ctx)
	var closeErr websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("want websocket close error, got %v", err)
	}
	if closeErr.Code != websocket.StatusNormalClosure || closeErr.Reason != "eof" {
		t.Fatalf("close = %d/%q, want %d/eof", closeErr.Code, closeErr.Reason, websocket.StatusNormalClosure)
	}
}
