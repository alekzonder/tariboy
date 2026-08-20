package audit

import (
	"bufio"
	"os"
	"path/filepath"
)

// Rotate trims path to at most maxBytes by dropping the oldest lines, keeping the
// newest ones (which preserves seq order and keeps follow cursors valid). A
// non-positive maxBytes, a missing file, or a file already within budget is a
// no-op. The rewrite is atomic (temp file + rename).
func Rotate(path string, maxBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Size() <= maxBytes {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		b := append([]byte(nil), sc.Bytes()...)
		lines = append(lines, b)
	}
	scanErr := sc.Err()
	f.Close()
	if scanErr != nil {
		return scanErr
	}

	// Keep the newest suffix of lines whose total size fits under maxBytes.
	var total int64
	start := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		total += int64(len(lines[i])) + 1 // +1 for the newline
		if total > maxBytes {
			break
		}
		start = i
	}
	kept := lines[start:]

	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	for _, ln := range kept {
		w.Write(ln)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(path))
}
