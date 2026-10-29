package operations

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// defaultLogLimit bounds one operation log; later lines are dropped.
const defaultLogLimit = 10 << 20

const truncatedLine = "[log truncated: the 10 MiB limit is reached]"

// Log appends lines to an operation log file. A nil *Log drops every line, so
// code shared with synchronous callers passes nil.
type Log struct {
	mu        sync.Mutex
	f         *os.File
	size      int64
	limit     int64
	truncated bool
	onLine    func()
}

func (s *Store) openLog(id int64, onLine func()) (*Log, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.LogPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Log{f: f, limit: defaultLogLimit, onLine: onLine}, nil
}

// Printf writes one formatted line.
func (l *Log) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.write(fmt.Sprintf(format, args...))
}

func (l *Log) write(text string) {
	l.mu.Lock()
	if l.truncated {
		l.mu.Unlock()
		return
	}
	line := strings.TrimSuffix(text, "\n") + "\n"
	if l.size >= l.limit {
		line, l.truncated = truncatedLine+"\n", true
	}
	n, _ := l.f.WriteString(line) // a log write failure must not fail the operation
	l.size += int64(n)
	l.mu.Unlock()
	if l.onLine != nil {
		l.onLine()
	}
}

func (l *Log) close() {
	if l != nil {
		_ = l.f.Close() // the log is append-only; nothing is lost on a close error
	}
}

// Writer returns a writer that logs each line it receives with prefix, for
// the output of a child process. Close writes an unfinished last line.
func (l *Log) Writer(prefix string) io.WriteCloser {
	return &lineWriter{log: l, prefix: prefix}
}

type lineWriter struct {
	log    *Log
	prefix string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			return len(p), nil
		}
		w.log.Printf("%s%s", w.prefix, w.buf[:i])
		w.buf = w.buf[i+1:]
	}
}

func (w *lineWriter) Close() error {
	if len(w.buf) > 0 {
		w.log.Printf("%s%s", w.prefix, w.buf)
		w.buf = nil
	}
	return nil
}
