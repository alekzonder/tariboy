package cli_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/compose"
	"github.com/alekzonder/tariboy/internal/portablearchive"
	"github.com/alekzonder/tariboy/internal/teamportable"
)

// readArchive returns the manifest and the member files of a portable archive.
func readArchive(t *testing.T, file string) (portablearchive.Manifest, map[string][]byte) {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var m portablearchive.Manifest
	files := map[string][]byte{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "manifest.json" {
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatal(err)
			}
			continue
		}
		files[h.Name] = body
	}
	return m, files
}

// rewriteArchive writes a changed copy of an archive. With rehash the
// manifest lists the new sha256 and size of every file, so only a deeper
// check can find the change.
func rewriteArchive(t *testing.T, src string, rehash bool, mutate func(*portablearchive.Manifest, map[string][]byte)) string {
	t.Helper()
	m, files := readArchive(t, src)
	mutate(&m, files)
	dir := t.TempDir()
	var paths []string
	for i, f := range m.Files {
		body := files[f.Path]
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if rehash {
			sum := sha256.Sum256(body)
			m.Files[i].SHA256, m.Files[i].Size = hex.EncodeToString(sum[:]), int64(len(body))
		}
		paths = append(paths, f.Path)
	}
	out := filepath.Join(t.TempDir(), "changed.tar.gz")
	w, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := portablearchive.Write(w, dir, m, paths); err != nil {
		t.Fatal(err)
	}
	return out
}

// setMeta changes one key of the compose archive metadata.
func setMeta(t *testing.T, m *portablearchive.Manifest, key string, value any) {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(m.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	meta[key] = value
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata = raw
}

// flipImageByte changes one byte inside the archived image archive.
func flipImageByte(_ *portablearchive.Manifest, files map[string][]byte) {
	body := files["images/stub/latest.tar.gz"]
	body[len(body)/2] ^= 0xff
}

// noCallCaller fails the test on any daemon call.
type noCallCaller struct{ t *testing.T }

func (n noCallCaller) Call(method, route string, _ any) (json.RawMessage, error) {
	n.t.Errorf("validate called the daemon: %s %s", method, route)
	return nil, errors.New("no daemon")
}

// readOnlyCaller fails the test on any call other than a GET.
type readOnlyCaller struct {
	t *testing.T
	c *client.Client
}

func (r readOnlyCaller) Call(method, route string, body any) (json.RawMessage, error) {
	if method != "GET" {
		r.t.Errorf("unexpected mutating call %s %s", method, route)
	}
	return r.c.Call(method, route, body)
}

func (r readOnlyCaller) Upload(route string, _ io.Reader, _ int64) (json.RawMessage, error) {
	r.t.Errorf("unexpected upload to %s", route)
	return nil, errors.New("read only")
}

func validateArchive(t *testing.T, call compose.Caller, archive string, daemon bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := compose.Run(context.Background(), call, compose.Options{Verb: "validate", Archive: archive, Daemon: daemon}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestComposeValidateArchive(t *testing.T) {
	isolateEnv(t)
	archive, src := packOnSource(t)

	if code, out, errOut := validateArchive(t, noCallCaller{t}, archive, false); code != 0 || !strings.Contains(out, "valid") {
		t.Fatalf("valid archive: exit %d\n%s\n%s", code, out, errOut)
	}
	if code, _, errOut := validateArchive(t, readOnlyCaller{t: t, c: src}, archive, true); code != 0 {
		t.Fatalf("valid archive with --daemon: exit %d\n%s", code, errOut)
	}

	cases := []struct {
		name    string
		archive string
		want    string
	}{
		{"changed image byte", rewriteArchive(t, archive, true, flipImageByte), "image stub:latest"},
		{"changed manifest", rewriteArchive(t, archive, false, func(_ *portablearchive.Manifest, files map[string][]byte) {
			files["compose.yaml"] = append(files["compose.yaml"], '\n')
		}), "file mismatch compose.yaml"},
		{"tariboy range", rewriteArchive(t, archive, true, func(_ *portablearchive.Manifest, files map[string][]byte) {
			files["compose.yaml"] = bytes.Replace(files["compose.yaml"], []byte("version: 2\n"), []byte("version: 2\ntariboy:\n  min: 99.0.0\n"), 1)
		}), "does not satisfy tariboy"},
		{"newer producer", rewriteArchive(t, archive, true, func(m *portablearchive.Manifest, _ map[string][]byte) {
			setMeta(t, m, "producer_version", "99.0.0")
		}), "older than 99.0.0"},
		{"unknown schema", rewriteArchive(t, archive, true, func(m *portablearchive.Manifest, _ map[string][]byte) {
			setMeta(t, m, "schema", 2)
		}), "schema 2 is not supported"},
	}
	for _, tc := range cases {
		code, _, errOut := validateArchive(t, noCallCaller{t}, tc.archive, false)
		if code != 1 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want 1 with %q:\n%s", tc.name, code, tc.want, errOut)
		}
	}
}

// daemonLists is what a failed unpack must leave unchanged.
func daemonLists(t *testing.T, c *client.Client) string {
	t.Helper()
	var all []string
	for _, route := range []string{"/api/agents", "/api/groups", "/api/task-queues", "/api/images", "/api/workflow-images"} {
		raw, err := c.Call("GET", route, map[string]string{})
		if err != nil {
			t.Fatalf("GET %s: %v", route, err)
		}
		all = append(all, string(raw))
	}
	return strings.Join(all, "\n")
}

func unpack(t *testing.T, call compose.Caller, o compose.Options) (int, string, string) {
	t.Helper()
	o.Verb = "unpack"
	var out, errOut bytes.Buffer
	code := compose.Run(context.Background(), call, o, &out, &errOut)
	return code, out.String(), errOut.String()
}

// A broken archive, an archive from a newer daemon, a daemon outside the
// file's range, a dry run and a "no" answer change nothing on the daemon.
func TestComposeUnpackRefusalsChangeNothing(t *testing.T) {
	isolateEnv(t)
	archive, _ := packOnSource(t)
	_, dst := startIsolatedDaemon(t)
	before := daemonLists(t, dst)

	refused := []struct {
		name    string
		archive string
		want    string
	}{
		{"changed image byte", rewriteArchive(t, archive, true, flipImageByte), "image stub:latest"},
		{"newer producer", rewriteArchive(t, archive, true, func(m *portablearchive.Manifest, _ map[string][]byte) {
			setMeta(t, m, "producer_version", "99.0.0")
		}), "older than 99.0.0"},
		{"tariboy range", rewriteArchive(t, archive, true, func(_ *portablearchive.Manifest, files map[string][]byte) {
			files["compose.yaml"] = bytes.Replace(files["compose.yaml"], []byte("version: 2\n"), []byte("version: 2\ntariboy:\n  min: 99.0.0\n"), 1)
		}), "does not satisfy tariboy"},
	}
	for _, tc := range refused {
		code, _, errOut := unpack(t, readOnlyCaller{t: t, c: dst}, compose.Options{Args: []string{tc.archive}, Yes: true})
		if code != 1 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want 1 with %q:\n%s", tc.name, code, tc.want, errOut)
		}
	}

	code, out, errOut := unpack(t, readOnlyCaller{t: t, c: dst}, compose.Options{Args: []string{archive}, DryRun: true})
	if code != 0 || !strings.Contains(out, "phase stores") || !strings.Contains(out, "skip     store:team") || !strings.Contains(out, "import   image:stub") {
		t.Errorf("dry run: exit %d\n%s\n%s", code, out, errOut)
	}
	code, _, errOut = unpack(t, readOnlyCaller{t: t, c: dst}, compose.Options{Args: []string{archive}, Stdin: strings.NewReader("n\n")})
	if code != 0 || !strings.Contains(errOut, "archive not applied") {
		t.Errorf("answer n: exit %d\n%s", code, errOut)
	}
	if after := daemonLists(t, dst); after != before {
		t.Fatalf("daemon state changed:\nbefore %s\nafter  %s", before, after)
	}
}

// The deprecated archive and import verbs warn on stderr and run pack and
// unpack.
func TestComposeArchiveAndImportAliases(t *testing.T) {
	isolateEnv(t)
	file := writePackProject(t)
	sock, _ := startIsolatedDaemon(t)
	archive := filepath.Join(t.TempDir(), "alias.tar.gz")
	code, out, errOut := runCompose(t, sock, "archive", "-f", file, "--output", archive)
	if code != 0 || !strings.Contains(errOut, "compose archive is deprecated") {
		t.Fatalf("archive: exit %d\n%s\n%s", code, out, errOut)
	}
	if m, _ := readArchive(t, archive); m.Kind != "compose" {
		t.Fatalf("archive kind = %q, want compose", m.Kind)
	}

	dstSock, dst := startIsolatedDaemon(t)
	code, out, errOut = runCompose(t, dstSock, "import", "--archive", archive, "--yes")
	if code != 0 || !strings.Contains(errOut, "compose import is deprecated") || !strings.Contains(out, "unpack: ") {
		t.Fatalf("import: exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := dst.Call("GET", "/api/agents/scout", map[string]string{}); err != nil {
		t.Fatalf("import did not create the agent: %v", err)
	}
}

// unpack still accepts a team v1 archive through the team import.
func TestComposeUnpackAcceptsTeamV1Archive(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "tariboy-compose.yaml")
	if err := os.WriteFile(file, []byte("version: 1\ngroups:\n  legacy: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "team.tar.gz")
	w, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := teamportable.CreateFromCompose(file, w); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	sock, c := startIsolatedDaemon(t)
	code, out, errOut := runCompose(t, sock, "unpack", archive, "--yes")
	if code != 0 {
		t.Fatalf("unpack team archive: exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := c.Call("GET", "/api/groups/legacy", map[string]string{}); err != nil {
		t.Fatalf("team import did not create the group: %v", err)
	}
}
