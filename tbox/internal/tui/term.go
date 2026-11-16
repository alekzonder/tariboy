package tui

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// ErrNoTerminal is returned by Open when the input or the output is not a
// terminal.
var ErrNoTerminal = errors.New("input or output is not a terminal")

// escWait is how long a lone ESC waits for the rest of an escape sequence.
const escWait = 50 * time.Millisecond

// caughtSignals end the process while a list is shown. The terminal is
// restored before the signal takes its default effect. In raw mode Ctrl-C and
// Ctrl-\ arrive as bytes, so SIGINT and SIGQUIT come only from kill.
var caughtSignals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGINT}

// sysDeps are the process and terminal calls of Open, replaced in tests.
type sysDeps struct {
	isTerminal func(fd int) bool
	makeRaw    func(fd int) (*term.State, error)
	restore    func(fd int, st *term.State) error
	notify     func(c chan<- os.Signal, sig ...os.Signal)
	stop       func(c chan<- os.Signal)
	raise      func(sig os.Signal)
}

var realSys = sysDeps{
	isTerminal: term.IsTerminal,
	makeRaw:    term.MakeRaw,
	restore:    term.Restore,
	notify:     signal.Notify,
	stop:       signal.Stop,
	raise:      raiseDefault,
}

// raiseDefault delivers sig again with its default disposition, so the
// process ends the way it would have without tbox's handler. SIGQUIT would
// make the Go runtime print a goroutine dump, so it exits with 128+3 instead.
func raiseDefault(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok || s == syscall.SIGQUIT {
		os.Exit(128 + int(syscall.SIGQUIT))
	}
	signal.Reset(s)
	_ = syscall.Kill(os.Getpid(), s)
	time.Sleep(time.Second)
	os.Exit(128 + int(s))
}

type realTerm struct {
	in, out *os.File
	state   *term.State
	sys     sysDeps
	sigs    chan os.Signal
	done    chan struct{}
	once    sync.Once
	buf     []byte
}

// Open puts in into raw mode and returns a Terminal that reads keys from in
// and writes to out. Both must be terminals: frames written to a file or a
// pipe would be invisible while the list waits for keys. Close restores the
// saved state; a terminating signal restores it too before the process ends.
func Open(in, out *os.File) (Terminal, error) {
	return openWith(in, out, realSys)
}

func openWith(in, out *os.File, sys sysDeps) (Terminal, error) {
	if !sys.isTerminal(int(in.Fd())) || !sys.isTerminal(int(out.Fd())) {
		return nil, ErrNoTerminal
	}
	st, err := sys.makeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	r := &realTerm{in: in, out: out, state: st, sys: sys, sigs: make(chan os.Signal, 1), done: make(chan struct{})}
	sys.notify(r.sigs, caughtSignals...)
	go r.watch()
	return r, nil
}

// watch restores the terminal on a terminating signal and re-raises it. A
// signal queued while Close ran is re-raised too: select may pick done first.
func (r *realTerm) watch() {
	select {
	case sig := <-r.sigs:
		_ = r.Close()
		r.sys.raise(sig)
	case <-r.done:
		select {
		case sig := <-r.sigs:
			r.sys.raise(sig)
		default:
		}
	}
}

func (r *realTerm) Write(p []byte) (int, error) { return r.out.Write(p) }

func (r *realTerm) Size() (int, int) {
	c, rows, err := term.GetSize(int(r.out.Fd()))
	if err != nil || c < 1 || rows < 1 {
		return 80, 24
	}
	return c, rows
}

func (r *realTerm) Close() error {
	var err error
	r.once.Do(func() {
		r.sys.stop(r.sigs)
		close(r.done)
		_, _ = r.out.Write([]byte(showCursor))
		err = r.sys.restore(int(r.in.Fd()), r.state)
	})
	return err
}

// fill reads more bytes into the buffer. With wait > 0 it gives up after wait
// and reports false.
func (r *realTerm) fill(wait time.Duration) (bool, error) {
	if wait > 0 {
		fds := []unix.PollFd{{Fd: int32(r.in.Fd()), Events: unix.POLLIN}}
		for {
			n, err := unix.Poll(fds, int(wait/time.Millisecond))
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return false, err
			}
			if n == 0 {
				return false, nil
			}
			break
		}
	}
	tmp := make([]byte, 64)
	n, err := r.in.Read(tmp)
	if n > 0 {
		r.buf = append(r.buf, tmp[:n]...)
		return true, nil
	}
	if err == nil {
		err = errors.New("terminal read returned no data")
	}
	return false, err
}

func (r *realTerm) ReadKey() (Key, error) {
	for {
		if len(r.buf) > 0 {
			k, n := decodeKey(r.buf)
			if n > 0 {
				r.buf = r.buf[n:]
				if k.Code == KeyRune && k.Rune == 0 {
					continue
				}
				return k, nil
			}
			got, err := r.fill(escWait)
			if err != nil {
				return Key{}, err
			}
			if !got {
				// An incomplete sequence that never finished: a lone ESC
				// or a broken rune.
				var k Key
				k, r.buf = dropIncomplete(r.buf)
				if k.Code == KeyEsc {
					return k, nil
				}
			}
			continue
		}
		if _, err := r.fill(0); err != nil {
			return Key{}, err
		}
	}
}

// decodeKey decodes one key from the start of b. It returns the key and the
// number of bytes used; n == 0 means b is an incomplete sequence. An ignored
// key (unknown escape sequence or control byte) is a KeyRune with Rune 0.
func decodeKey(b []byte) (Key, int) {
	if len(b) == 0 {
		return Key{}, 0
	}
	switch c := b[0]; {
	case c == 0x1b:
		return decodeEscape(b)
	case c == 0x03:
		return Key{Code: KeyCtrlC}, 1
	case c == '\r' || c == '\n':
		return Key{Code: KeyEnter}, 1
	case c == ' ':
		return Key{Code: KeySpace}, 1
	case c == 0x7f || c == 0x08:
		return Key{Code: KeyBackspace}, 1
	case c < 0x20:
		return Key{}, 1
	}
	if !utf8.FullRune(b) {
		return Key{}, 0
	}
	r, n := utf8.DecodeRune(b)
	if r == utf8.RuneError {
		return Key{}, n
	}
	return Key{Code: KeyRune, Rune: r}, n
}

func decodeEscape(b []byte) (Key, int) {
	if len(b) < 2 {
		return Key{}, 0
	}
	if b[1] != '[' && b[1] != 'O' {
		// ESC followed by something else: a cancel; the rest is read next.
		return Key{Code: KeyEsc}, 1
	}
	if len(b) < 3 {
		return Key{}, 0
	}
	// CSI parameters run to a final byte in 0x40-0x7e.
	end := 2
	for end < len(b) && (b[end] < 0x40 || b[end] > 0x7e) {
		end++
	}
	if end == len(b) {
		return Key{}, 0
	}
	n := end + 1
	return csiKey(string(b[2:n])), n
}

func csiKey(seq string) Key {
	switch seq {
	case "A":
		return Key{Code: KeyUp}
	case "B":
		return Key{Code: KeyDown}
	case "5~":
		return Key{Code: KeyPageUp}
	case "6~":
		return Key{Code: KeyPageDown}
	}
	return Key{}
}

// dropIncomplete gives up on an incomplete sequence at the start of b after
// the wait ran out. A sequence that starts with ESC is a lone Esc and is
// dropped whole, so a stray "[" or "O" is not read as a rune; a broken rune
// loses one byte and is ignored.
func dropIncomplete(b []byte) (Key, []byte) {
	if len(b) > 0 && b[0] == 0x1b {
		return Key{Code: KeyEsc}, nil
	}
	if len(b) > 0 {
		return Key{}, b[1:]
	}
	return Key{}, b
}
