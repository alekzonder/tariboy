package shim

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type fakeHandler struct {
	killed   bool
	keys     string
	deadline string
}

func (f *fakeHandler) Status() StatusResult {
	return StatusResult{Running: true, PID: 42, StartedAt: "t0"}
}
func (f *fakeHandler) Kill() error { f.killed = true; return nil }
func (f *fakeHandler) Screen() (string, error) {
	return "", errors.New("not a tmux session")
}
func (f *fakeHandler) SendKeys(p SendKeysParams) error { f.keys = p.Keys; return nil }
func (f *fakeHandler) Report() ReportResult {
	return ReportResult{Finished: true, Result: &IterationResult{ExitCode: 0, EndedAt: "t1", CPUMs: 10, MemPeakKB: 20}}
}
func (f *fakeHandler) SetHardDeadline(deadline string) error { f.deadline = deadline; return nil }

func TestRPCRoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "shim.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHandler{}
	go Serve(l, h)
	defer l.Close()

	c := Dial(sock)
	st, err := c.Status()
	if err != nil || !st.Running || st.PID != 42 {
		t.Fatalf("status = %+v err=%v", st, err)
	}
	if err := c.SendKeys("hello"); err != nil || h.keys != "hello" {
		t.Fatalf("sendkeys: keys=%q err=%v", h.keys, err)
	}
	if err := c.Kill(); err != nil || !h.killed {
		t.Fatalf("kill: killed=%v err=%v", h.killed, err)
	}
	if _, err := c.Screen(); err == nil {
		t.Fatal("screen in process mode should error")
	}
	rep, err := c.Report()
	if err != nil || !rep.Finished || rep.Result.CPUMs != 10 {
		t.Fatalf("report = %+v err=%v", rep, err)
	}
	deadline := time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
	if err := c.SetHardDeadline(deadline); err != nil || h.deadline != deadline {
		t.Fatalf("set hard deadline: deadline=%q err=%v", h.deadline, err)
	}
}

func TestConnectedClientStaysBoundWhenSocketPathIsReused(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "shim.sock")
	oldListener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	oldHandler := &fakeHandler{}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := oldListener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := Connect(sock)
	if err != nil {
		t.Fatal(err)
	}
	oldConn := <-accepted
	go serveConn(oldConn, oldHandler)
	if err := oldListener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	newListener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer newListener.Close()
	newHandler := &fakeHandler{}
	go Serve(newListener, newHandler)

	if err := client.SendKeys("old"); err != nil {
		t.Fatal(err)
	}
	if oldHandler.keys != "old" {
		t.Fatalf("old shim keys = %q, want old", oldHandler.keys)
	}
	if newHandler.keys != "" {
		t.Fatalf("replacement shim received keys %q", newHandler.keys)
	}
}

func TestConnectTimeoutBoundsFullUnixAcceptQueue(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "full.sock")
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: sock}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Listen(fd, 1); err != nil {
		t.Fatal(err)
	}

	var held []net.Conn
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	for i := 0; i < 8; i++ {
		conn, err := net.DialTimeout("unix", sock, 20*time.Millisecond)
		if err != nil {
			break
		}
		held = append(held, conn)
	}
	if len(held) == 8 {
		t.Fatal("could not fill Unix accept queue")
	}

	start := time.Now()
	client, err := ConnectTimeout(sock, 50*time.Millisecond)
	if client != nil {
		t.Fatal("ConnectTimeout returned a client for a full accept queue")
	}
	if err == nil {
		t.Fatal("ConnectTimeout returned no error for a full accept queue")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("ConnectTimeout took %s, want a bounded dial", elapsed)
	}
}

// echoStream is a fake StreamHandler whose Attach echoes stdin back to
// stdout until EOF, embedding fakeHandler for the non-stream methods.
type echoStream struct{ Handler }

func (echoStream) Resize(ResizeParams) error { return nil }
func (echoStream) Attach(conn net.Conn, _ AttachParams) error {
	_, err := io.Copy(conn, conn) // echo until EOF
	return err
}

func TestServeConnAttachEcho(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go Serve(l, echoStream{Handler: &fakeHandler{}})
	conn, err := Dial(sock).Attach(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	io.ReadFull(conn, buf)
	if string(buf) != "ping" {
		t.Fatalf("got %q", buf)
	}
}

func TestResizeParamsRoundTrip(t *testing.T) {
	b, _ := json.Marshal(ResizeParams{Cols: 120, Rows: 40})
	var p ResizeParams
	if err := json.Unmarshal(b, &p); err != nil || p.Cols != 120 || p.Rows != 40 {
		t.Fatalf("roundtrip: %v %+v", err, p)
	}
}

// TestClientTimeoutOnHungPeer verifies that Client.call does not block
// forever when the peer accepts the connection but never responds.
func TestClientTimeoutOnHungPeer(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "hung.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	accepted := make(chan struct{})
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		close(accepted)
		// Hold the connection open without ever reading or writing,
		// simulating a hung shim.
		<-time.After(2 * time.Second)
		conn.Close()
	}()

	c := Dial(sock)
	c.Timeout = 200 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := c.Status()
		done <- err
	}()

	select {
	case <-accepted:
	case <-time.After(1 * time.Second):
		t.Fatal("server never accepted connection")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Client.call did not return within 1s of a hung peer")
	}
}

// TestServeConnClosesOnSilentClient verifies that serveConn bounds the
// initial request read: a client that connects and sends nothing must be
// closed by the server within serverConnTimeout, not hang the goroutine (and
// the conn) forever. serverConnTimeout is temporarily lowered so the test
// stays fast and deterministic.
func TestServeConnClosesOnSilentClient(t *testing.T) {
	orig := serverConnTimeoutNanos.Load()
	serverConnTimeoutNanos.Store(int64(100 * time.Millisecond))
	defer func() { serverConnTimeoutNanos.Store(orig) }()

	sock := filepath.Join(t.TempDir(), "silent.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go Serve(l, &fakeHandler{})

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send nothing; wait for the server to close its side.
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := conn.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected read error/EOF once server closes idle conn, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close a silent conn within bounded time — pre-decode deadline regression")
	}
}
