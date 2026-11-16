package tui

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"
)

// fakeSys records what openWith does to the process and the terminal.
type fakeSys struct {
	mu        sync.Mutex
	ttys      map[uintptr]bool
	raw       int
	restores  int
	notified  []os.Signal
	ch        chan<- os.Signal
	stopped   bool
	raised    []os.Signal
	raisedSig chan os.Signal
}

func newFakeSys(ttys ...uintptr) *fakeSys {
	f := &fakeSys{ttys: map[uintptr]bool{}, raisedSig: make(chan os.Signal, 1)}
	for _, fd := range ttys {
		f.ttys[fd] = true
	}
	return f
}

func (f *fakeSys) sys() sysDeps {
	return sysDeps{
		isTerminal: func(fd int) bool { return f.ttys[uintptr(fd)] },
		makeRaw: func(int) (*term.State, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.raw++
			return &term.State{}, nil
		},
		restore: func(int, *term.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.restores++
			return nil
		},
		notify: func(c chan<- os.Signal, sig ...os.Signal) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.ch, f.notified = c, sig
		},
		stop: func(chan<- os.Signal) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.stopped = true
		},
		raise: func(s os.Signal) {
			f.mu.Lock()
			f.raised = append(f.raised, s)
			f.mu.Unlock()
			f.raisedSig <- s
		},
	}
}

func pipeFiles(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r, w
}

func TestOpenRequiresTerminalOutput(t *testing.T) {
	in, out := pipeFiles(t)
	f := newFakeSys(in.Fd())
	if _, err := openWith(in, out, f.sys()); !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("output not a terminal: got %v", err)
	}
	if f.raw != 0 {
		t.Fatal("raw mode entered without a terminal output")
	}
	f = newFakeSys(out.Fd())
	if _, err := openWith(in, out, f.sys()); !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("input not a terminal: got %v", err)
	}
}

func TestSignalRestoresTerminalAndReraises(t *testing.T) {
	in, out := pipeFiles(t)
	f := newFakeSys(in.Fd(), out.Fd())
	tt, err := openWith(in, out, f.sys())
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	want := map[os.Signal]bool{syscall.SIGTERM: true, syscall.SIGHUP: true, syscall.SIGQUIT: true, syscall.SIGINT: true}
	for _, s := range f.notified {
		delete(want, s)
	}
	ch := f.ch
	f.mu.Unlock()
	if len(want) != 0 {
		t.Fatalf("signals not caught: %v", want)
	}
	ch <- syscall.SIGTERM
	select {
	case s := <-f.raisedSig:
		if s != syscall.SIGTERM {
			t.Fatalf("raised %v", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal not re-raised")
	}
	f.mu.Lock()
	restores, stopped := f.restores, f.stopped
	f.mu.Unlock()
	if restores != 1 || !stopped {
		t.Fatalf("restores=%d stopped=%v", restores, stopped)
	}
	_ = tt.Close()
	if f.restores != 1 {
		t.Fatalf("Close after signal restored again: %d", f.restores)
	}
}

func TestCloseStopsSignals(t *testing.T) {
	in, out := pipeFiles(t)
	f := newFakeSys(in.Fd(), out.Fd())
	tt, err := openWith(in, out, f.sys())
	if err != nil {
		t.Fatal(err)
	}
	_ = tt.Close()
	_ = tt.Close()
	if !f.stopped || f.restores != 1 {
		t.Fatalf("stopped=%v restores=%d", f.stopped, f.restores)
	}
	if len(f.raised) != 0 {
		t.Fatalf("raised %v", f.raised)
	}
}
