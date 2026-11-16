package fetch

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, Client) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv, Client{HTTP: srv.Client()}
}

func TestGet(t *testing.T) {
	srv, c := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Write(bytes.Repeat([]byte("x"), 100))
		case "/chunked":
			w.(http.Flusher).Flush()
			w.Write(bytes.Repeat([]byte("x"), 100))
		case "/to-http":
			http.Redirect(w, r, "http://example.com/x", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/hop":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.Write([]byte("hello"))
		}
	}))
	ctx := context.Background()
	if b, err := c.Get(ctx, srv.URL+"/ok", 10); err != nil || string(b) != "hello" {
		t.Fatalf("Get = %q, %v", b, err)
	}
	if b, err := c.Get(ctx, srv.URL+"/hop", 10); err != nil || string(b) != "hello" {
		t.Fatalf("redirect Get = %q, %v", b, err)
	}
	for _, p := range []string{"/big", "/chunked"} {
		if _, err := c.Get(ctx, srv.URL+p, 10); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("%s: want size error, got %v", p, err)
		}
	}
	if _, err := c.Get(ctx, srv.URL+"/to-http", 10); err == nil {
		t.Error("redirect to http accepted")
	}
	if _, err := c.Get(ctx, srv.URL+"/loop", 10); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("loop: want redirect error, got %v", err)
	}
}

func TestGetRejectsURLs(t *testing.T) {
	c := Client{}
	for _, u := range []string{"http://example.com/a", "https://u:p@example.com/a", "https://u@example.com/a", "ftp://x/a", "https:///a", "::"} {
		if _, err := c.Get(context.Background(), u, 10); err == nil {
			t.Errorf("Get(%q) succeeded", u)
		}
	}
	insecure := Client{AllowInsecure: true}
	if _, err := insecure.Get(context.Background(), "http://u:p@example.com/", 10); err == nil {
		t.Error("user-info accepted with AllowInsecure")
	}
}

type entry struct {
	name, body string
	typ        byte
	mode       int64
	link       string
}

func tarGz(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: mode, Linkname: e.link}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipData(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := os.FileMode(0o644)
		if e.mode != 0 {
			mode = os.FileMode(e.mode)
		}
		if e.typ == tar.TypeSymlink {
			mode |= os.ModeSymlink
		}
		if strings.HasSuffix(e.name, "/") {
			mode |= os.ModeDir
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	return buf.Bytes()
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return nil
	})
	return out
}

func TestExtractArchiveStripsTopDir(t *testing.T) {
	files := []entry{
		{name: "proj-1/", typ: tar.TypeDir, mode: 0o755},
		{name: "proj-1/a.txt", body: "A"},
		{name: "proj-1/sub/run.sh", body: "#!/bin/sh", mode: 0o755},
	}
	zfiles := []entry{{name: "proj-1/", body: ""}, files[1], files[2]}
	for name, data := range map[string][]byte{"tar.gz": tarGz(t, files), "zip": zipData(t, zfiles)} {
		dest := filepath.Join(t.TempDir(), "out")
		if err := ExtractArchive(data, dest); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := readTree(t, dest)
		if len(got) != 2 || got["a.txt"] != "A" || got["sub/run.sh"] != "#!/bin/sh" {
			t.Errorf("%s: tree = %v", name, got)
		}
		fi, err := os.Stat(filepath.Join(dest, "sub", "run.sh"))
		if err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("%s: run.sh mode = %v, %v", name, fi, err)
		}
		if m, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".extract-*")); len(m) != 0 {
			t.Errorf("%s: stage left behind: %v", name, m)
		}
	}
}

func TestExtractArchiveKeepsMultipleTopLevel(t *testing.T) {
	data := tarGz(t, []entry{{name: "a.txt", body: "A"}, {name: "d/b.txt", body: "B"}})
	dest := filepath.Join(t.TempDir(), "out")
	if err := ExtractArchive(data, dest); err != nil {
		t.Fatal(err)
	}
	if got := readTree(t, dest); got["a.txt"] != "A" || got["d/b.txt"] != "B" {
		t.Errorf("tree = %v", got)
	}
}

func TestExtractArchiveRejects(t *testing.T) {
	many := make([]entry, 4097)
	for i := range many {
		many[i] = entry{name: fmt.Sprintf("f%d", i)}
	}
	cases := map[string][]byte{
		"symlink":        tarGz(t, []entry{{name: "l", typ: tar.TypeSymlink, link: "/etc/passwd"}}),
		"hardlink":       tarGz(t, []entry{{name: "a", body: "x"}, {name: "h", typ: tar.TypeLink, link: "a"}}),
		"dotdot":         tarGz(t, []entry{{name: "../x", body: "x"}}),
		"inner dotdot":   tarGz(t, []entry{{name: "a/../../x", body: "x"}}),
		"absolute":       tarGz(t, []entry{{name: "/etc/x", body: "x"}}),
		"backslash":      tarGz(t, []entry{{name: `a\b`, body: "x"}}),
		"long path":      tarGz(t, []entry{{name: strings.Repeat("a", 513), body: "x"}}),
		"duplicate":      tarGz(t, []entry{{name: "a", body: "x"}, {name: "a", body: "y"}}),
		"file then dir":  tarGz(t, []entry{{name: "a", body: "x"}, {name: "a/b", body: "y"}}),
		"too many":       tarGz(t, many),
		"zip symlink":    zipData(t, []entry{{name: "l", typ: tar.TypeSymlink, body: "/etc/passwd"}}),
		"zip dotdot":     zipData(t, []entry{{name: "../x", body: "x"}}),
		"zip absolute":   zipData(t, []entry{{name: "/x", body: "x"}}),
		"not an archive": []byte("plain text"),
	}
	for name, data := range cases {
		dest := filepath.Join(t.TempDir(), "out")
		if err := ExtractArchive(data, dest); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := os.Lstat(dest); err == nil {
			t.Errorf("%s: dest exists after failure", name)
		}
		if m, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".extract-*")); len(m) != 0 {
			t.Errorf("%s: stage left behind", name)
		}
	}
}

func TestExtractArchiveSizeLimits(t *testing.T) {
	data := tarGz(t, []entry{{name: "a", body: strings.Repeat("x", 600)}, {name: "b", body: strings.Repeat("y", 600)}})
	lim := DefaultLimits
	lim.MaxExpanded = 1000
	if err := ExtractArchiveWith(data, filepath.Join(t.TempDir(), "o"), lim); err == nil || !strings.Contains(err.Error(), "expanded") {
		t.Errorf("tar.gz expanded: %v", err)
	}
	zdata := zipData(t, []entry{{name: "a", body: strings.Repeat("x", 600)}, {name: "b", body: strings.Repeat("y", 600)}})
	if err := ExtractArchiveWith(zdata, filepath.Join(t.TempDir(), "o"), lim); err == nil || !strings.Contains(err.Error(), "expanded") {
		t.Errorf("zip expanded: %v", err)
	}
	lim = DefaultLimits
	lim.MaxCompressed = 10
	if err := ExtractArchiveWith(data, filepath.Join(t.TempDir(), "o"), lim); err == nil || !strings.Contains(err.Error(), "compressed") {
		t.Errorf("compressed: %v", err)
	}
}

func TestExtractArchiveExistingDest(t *testing.T) {
	dest := t.TempDir()
	if err := ExtractArchive(tarGz(t, []entry{{name: "a", body: "x"}}), dest); err == nil {
		t.Error("existing dest accepted")
	}
}
