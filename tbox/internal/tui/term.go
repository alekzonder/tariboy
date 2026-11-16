package tui

import (
	"errors"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// ErrNoTerminal is returned by Open when the input is not a terminal.
var ErrNoTerminal = errors.New("input is not a terminal")

// escWait is how long a lone ESC waits for the rest of an escape sequence.
const escWait = 50 * time.Millisecond

type realTerm struct {
	in, out *os.File
	state   *term.State
	once    sync.Once
	buf     []byte
}

// Open puts in into raw mode and returns a Terminal that reads keys from in
// and writes to out. Close restores the saved state.
func Open(in, out *os.File) (Terminal, error) {
	if !term.IsTerminal(int(in.Fd())) {
		return nil, ErrNoTerminal
	}
	st, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	return &realTerm{in: in, out: out, state: st}, nil
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
		_, _ = r.out.Write([]byte(showCursor))
		err = term.Restore(int(r.in.Fd()), r.state)
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
