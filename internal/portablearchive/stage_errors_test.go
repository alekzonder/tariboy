package portablearchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rawArchive writes entries in order, with no implied manifest. A positive
// truncate drops that many bytes from the end of the last body, leaving a tar
// stream that ends inside the member.
func rawArchive(t *testing.T, entries []tarEntry, truncate int) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i, entry := range entries {
		h := &tar.Header{Name: entry.name, Typeflag: entry.typeflag, Size: int64(len(entry.body)), Mode: 0o600}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		body := entry.body
		last := i == len(entries)-1
		if last && truncate > 0 {
			body = body[:len(body)-truncate]
		}
		if len(body) > 0 {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if truncate > 0 {
		if err := tw.Flush(); err == nil {
			t.Fatal("expected a truncated tar body")
		}
		return gzipBytes(t, buf.Bytes())
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzipBytes(t, buf.Bytes())
}

func manifestEntry(t *testing.T, manifest Manifest) tarEntry {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return tarEntry{name: "manifest.json", typeflag: tar.TypeReg, body: body}
}

// stageLeftovers lists what a failed or finished Stage left in parent other
// than the destination itself.
func stageLeftovers(t *testing.T, parent string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".portable-stage-") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestStageErrorsAndCleanup(t *testing.T) {
	body := []byte("x")
	good := fileManifest("safe", body)
	emptyManifest := Manifest{Format: "tariboy-portable", Version: 1, Kind: "test"}
	regular := func(name string, data []byte) tarEntry {
		return tarEntry{name: name, typeflag: tar.TypeReg, body: data}
	}
	cases := []struct {
		name    string
		archive func(*testing.T) []byte
		limits  Limits
		want    string
		prefix  bool
	}{
		{name: "not gzip", archive: func(*testing.T) []byte { return bytes.Repeat([]byte("z"), 64) }, want: "portable archive: gzip: gzip: invalid header"},
		{name: "not tar", archive: func(t *testing.T) []byte { return gzipBytes(t, bytes.Repeat([]byte("z"), 1024)) }, want: "portable archive: tar: archive/tar: invalid tar header"},
		{name: "invalid path", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("../escape", body)}, 0)
		}, want: `portable archive: invalid path "../escape"`},
		{name: "unclean path", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("./safe", body)}, 0)
		}, want: `portable archive: invalid path "./safe"`},
		{name: "trailing slash", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{{name: "dir/", typeflag: tar.TypeDir}}, 0)
		}, want: `portable archive: invalid path "dir/"`},
		{name: "path too long", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("abcdefghij", body)}, 0)
		}, limits: Limits{MaxCompressedBytes: 1 << 20, MaxExpandedBytes: 100, MaxFiles: 5, MaxPathBytes: 5}, want: `portable archive: invalid path "abcdefghij"`},
		{name: "duplicate wins over count", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("safe", body), regular("safe", body)}, 0)
		}, limits: Limits{MaxCompressedBytes: 1 << 20, MaxExpandedBytes: 100, MaxFiles: 1, MaxPathBytes: 100}, want: "portable archive: duplicate path safe"},
		{name: "too many", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("a", body), regular("b", body)}, 0)
		}, limits: Limits{MaxCompressedBytes: 1 << 20, MaxExpandedBytes: 100, MaxFiles: 1, MaxPathBytes: 100}, want: "portable archive: too many entries"},
		{name: "expanded", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("a", []byte("12")), regular("b", []byte("34"))}, 0)
		}, limits: Limits{MaxCompressedBytes: 1 << 20, MaxExpandedBytes: 3, MaxFiles: 5, MaxPathBytes: 100}, want: "portable archive: expanded size exceeded"},
		{name: "symlink", archive: func(t *testing.T) []byte {
			return makeArchive(t, good, []tarEntry{{name: "safe", typeflag: tar.TypeSymlink, link: "outside"}})
		}, want: "portable archive: unsupported entry type for safe"},
		{name: "manifest directory", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{{name: "manifest.json", typeflag: tar.TypeDir}}, 0)
		}, want: "portable archive: manifest is not a file"},
		{name: "manifest truncated", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, good)}, 3)
		}, want: "portable archive: invalid manifest body"},
		{name: "manifest json", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("manifest.json", []byte(`{"format":`))}, 0)
		}, want: "portable archive: manifest: ", prefix: true},
		{name: "manifest unknown field", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("manifest.json", []byte(`{"mystery":1}`))}, 0)
		}, want: "portable archive: manifest: ", prefix: true},
		{name: "manifest format", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, Manifest{Format: "other", Version: 1, Kind: "k"})}, 0)
		}, want: "portable archive: unsupported manifest"},
		{name: "manifest version", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, Manifest{Format: "tariboy-portable", Version: 2, Kind: "k"})}, 0)
		}, want: "portable archive: unsupported manifest"},
		{name: "manifest kind", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, Manifest{Format: "tariboy-portable", Version: 1})}, 0)
		}, want: "portable archive: unsupported manifest"},
		{name: "manifest checked as it is read", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, Manifest{Format: "other"}), {name: "bad", typeflag: tar.TypeSymlink}}, 0)
		}, want: "portable archive: unsupported manifest"},
		{name: "directory under a file", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("a", body), {name: "a/b", typeflag: tar.TypeDir}}, 0)
		}, want: "mkdir ", prefix: true},
		{name: "file under a file", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("a", body), regular("a/b", body)}, 0)
		}, want: "mkdir ", prefix: true},
		{name: "file over a directory", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("a/b", body), regular("a", body)}, 0)
		}, want: "open ", prefix: true},
		{name: "file truncated", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{manifestEntry(t, good), regular("safe", []byte("abcdef"))}, 3)
		}, want: "portable archive: truncated file"},
		{name: "manifest missing", archive: func(t *testing.T) []byte {
			return rawArchive(t, []tarEntry{regular("safe", body)}, 0)
		}, want: "portable archive: manifest missing"},
		{name: "empty archive", archive: func(t *testing.T) []byte { return rawArchive(t, nil, 0) }, want: "portable archive: manifest missing"},
		{name: "file list bad path", archive: func(t *testing.T) []byte {
			return makeArchive(t, Manifest{Format: "tariboy-portable", Version: 1, Kind: "k", Files: []File{{Path: "../x"}}}, nil)
		}, want: "portable archive: invalid manifest file list"},
		{name: "file list manifest", archive: func(t *testing.T) []byte {
			return makeArchive(t, Manifest{Format: "tariboy-portable", Version: 1, Kind: "k", Files: []File{{Path: "manifest.json"}}}, nil)
		}, want: "portable archive: invalid manifest file list"},
		{name: "file list duplicate", archive: func(t *testing.T) []byte {
			return makeArchive(t, Manifest{Format: "tariboy-portable", Version: 1, Kind: "k", Files: []File{{Path: "a"}, {Path: "a"}}}, nil)
		}, want: "portable archive: invalid manifest file list"},
		{name: "file list before count", archive: func(t *testing.T) []byte {
			return makeArchive(t, Manifest{Format: "tariboy-portable", Version: 1, Kind: "k", Files: []File{{Path: "a"}, {Path: "../b"}}}, []tarEntry{regular("c", body)})
		}, want: "portable archive: invalid manifest file list"},
		{name: "count mismatch", archive: func(t *testing.T) []byte { return makeArchive(t, good, nil) }, want: "portable archive: manifest does not match files"},
		{name: "extra file", archive: func(t *testing.T) []byte {
			return makeArchive(t, emptyManifest, []tarEntry{regular("safe", body)})
		}, want: "portable archive: manifest does not match files"},
		{name: "other name", archive: func(t *testing.T) []byte {
			return makeArchive(t, fileManifest("other", body), []tarEntry{regular("safe", body)})
		}, want: "portable archive: file mismatch safe"},
		{name: "size", archive: func(t *testing.T) []byte {
			m := fileManifest("safe", body)
			m.Files[0].Size = 2
			return makeArchive(t, m, []tarEntry{regular("safe", body)})
		}, want: "portable archive: file mismatch safe"},
		{name: "digest", archive: func(t *testing.T) []byte {
			m := fileManifest("safe", body)
			m.Files[0].SHA256 = strings.Repeat("0", 64)
			return makeArchive(t, m, []tarEntry{regular("safe", body)})
		}, want: "portable archive: file mismatch safe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "staged")
			limits := tc.limits
			if limits == (Limits{}) {
				limits = DefaultLimits()
			}
			archive := tc.archive(t)
			_, err := Stage(bytes.NewReader(archive), int64(len(archive)), destination, limits)
			if err == nil {
				t.Fatal("Stage accepted the archive")
			}
			if tc.prefix && !strings.HasPrefix(err.Error(), tc.want) || !tc.prefix && err.Error() != tc.want {
				t.Fatalf("Stage error = %q, want %q", err, tc.want)
			}
			if left := stageLeftovers(t, parent); len(left) != 0 {
				t.Fatalf("stage directories left behind: %v", left)
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination after failure: %v", err)
			}
		})
	}
}

func TestStageRejectsBadLimitsBeforeTouchingTheDisk(t *testing.T) {
	limits := DefaultLimits()
	cases := map[string]struct {
		size   int64
		limits Limits
	}{
		"negative size": {-1, limits},
		"over limit":    {limits.MaxCompressedBytes + 1, limits},
		"no files":      {1, Limits{MaxCompressedBytes: 10, MaxExpandedBytes: 1, MaxPathBytes: 1}},
		"no expanded":   {1, Limits{MaxCompressedBytes: 10, MaxFiles: 1, MaxPathBytes: 1}},
		"no paths":      {1, Limits{MaxCompressedBytes: 10, MaxFiles: 1, MaxExpandedBytes: 1}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "parent")
			_, err := Stage(bytes.NewReader(nil), tc.size, filepath.Join(parent, "staged"), tc.limits)
			if err == nil || err.Error() != "portable archive: limit exceeded" {
				t.Fatalf("Stage error = %v", err)
			}
			if _, err := os.Lstat(parent); !os.IsNotExist(err) {
				t.Fatalf("parent created before the limit check: %v", err)
			}
		})
	}
}

func TestStageCreatesDirectoriesAndKeepsThemOutOfTheManifest(t *testing.T) {
	body := []byte("x")
	archive := makeArchive(t, fileManifest("sub/deep/file", body), []tarEntry{
		{name: "empty", typeflag: tar.TypeDir},
		{name: "sub", typeflag: tar.TypeDir},
		{name: "sub/deep/file", typeflag: tar.TypeReg, body: body},
	})
	parent := t.TempDir()
	destination := filepath.Join(parent, "nested", "staged")
	manifest, err := Stage(bytes.NewReader(archive), int64(len(archive)), destination, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != "sub/deep/file" {
		t.Fatalf("manifest = %#v", manifest)
	}
	info, err := os.Stat(filepath.Join(destination, "empty"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("empty directory = %v, %v", info, err)
	}
	info, err = os.Stat(filepath.Join(destination, "sub", "deep", "file"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("staged file = %v, %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(destination, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest.json was staged: %v", err)
	}
	if left := stageLeftovers(t, filepath.Dir(destination)); len(left) != 0 {
		t.Fatalf("stage directories left behind: %v", left)
	}
}

func TestStageAcceptsTheManifestAfterItsFiles(t *testing.T) {
	body := []byte("x")
	archive := rawArchive(t, []tarEntry{{name: "safe", typeflag: tar.TypeReg, body: body}, manifestEntry(t, fileManifest("safe", body))}, 0)
	destination := filepath.Join(t.TempDir(), "staged")
	if _, err := Stage(bytes.NewReader(archive), int64(len(archive)), destination, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
}

func TestStageDoesNotReplaceAnExistingDestination(t *testing.T) {
	body := []byte("x")
	archive := makeArchive(t, fileManifest("safe", body), []tarEntry{{name: "safe", typeflag: tar.TypeReg, body: body}})
	parent := t.TempDir()
	destination := filepath.Join(parent, "staged")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Stage(bytes.NewReader(archive), int64(len(archive)), destination, DefaultLimits())
	if err == nil || err.Error() != "portable archive: destination exists" {
		t.Fatalf("Stage error = %v", err)
	}
	if _, err := os.Lstat(marker); err != nil {
		t.Fatalf("existing destination changed: %v", err)
	}
	if left := stageLeftovers(t, parent); len(left) != 0 {
		t.Fatalf("stage directories left behind: %v", left)
	}
}

func TestStageReturnsDestinationLstatError(t *testing.T) {
	body := []byte("x")
	archive := makeArchive(t, fileManifest("safe", body), []tarEntry{{name: "safe", typeflag: tar.TypeReg, body: body}})
	parent := t.TempDir()
	destination := filepath.Join(parent, strings.Repeat("n", 300))
	_, err := Stage(bytes.NewReader(archive), int64(len(archive)), destination, DefaultLimits())
	if !errors.Is(err, syscall.ENAMETOOLONG) {
		t.Fatalf("Stage error = %v", err)
	}
	if left := stageLeftovers(t, parent); len(left) != 0 {
		t.Fatalf("stage directories left behind: %v", left)
	}
}
