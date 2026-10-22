package tidy

import (
	"bytes"
	"database/sql"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestLogSkipsExpectedErrors(t *testing.T) {
	buf := capture(t)
	for _, err := range []error{nil, fs.ErrNotExist, syscall.ESRCH, os.ErrClosed, os.ErrProcessDone, net.ErrClosed, sql.ErrTxDone} {
		Log(nil, "x", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("logged: %s", buf)
	}
}

func TestLogReportsUnexpectedError(t *testing.T) {
	buf := capture(t)
	Log(nil, "x", syscall.EPERM, "k", "v")
	if !strings.Contains(buf.String(), "k=v") || !strings.Contains(buf.String(), "level=WARN") {
		t.Fatalf("log = %q", buf)
	}
}

func TestRemoveMissingIsSilent(t *testing.T) {
	buf := capture(t)
	Remove(nil, filepath.Join(t.TempDir(), "none"))
	RemoveAll(nil, filepath.Join(t.TempDir(), "none"))
	if buf.Len() != 0 {
		t.Fatalf("logged: %s", buf)
	}
}

func TestLogUsesGivenLogger(t *testing.T) {
	def := capture(t)
	var buf bytes.Buffer
	Log(slog.New(slog.NewTextHandler(&buf, nil)), "x", syscall.EPERM)
	if buf.Len() == 0 || def.Len() != 0 {
		t.Fatalf("given=%q default=%q", &buf, def)
	}
}
