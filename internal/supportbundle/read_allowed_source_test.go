package supportbundle

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func writeSourceFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.log"), []byte("top"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", "leaf.log"), []byte("leaf"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestReadAllowedSourceOutcomes pins the issue code of every rejection and the
// parents the walk reports to its hook before stopping.
func TestReadAllowedSourceOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prepare   func(t *testing.T, root string)
		rootPath  func(root string) string
		path      func(root string) string
		remaining int64
		wantBody  string
		wantCode  string
		wantErr   error
		wantHooks []string
	}{
		{name: "top level file", path: func(r string) string { return filepath.Join(r, "top.log") }, remaining: 3, wantBody: "top"},
		{
			name: "nested file", path: func(r string) string { return filepath.Join(r, "a", "b", "leaf.log") }, remaining: 100,
			wantBody: "leaf", wantHooks: []string{"a", filepath.Join("a", "b")},
		},
		{name: "root itself", path: func(r string) string { return r }, remaining: 100, wantCode: "not_regular"},
		{name: "parent of root", path: filepath.Dir, remaining: 100, wantCode: "not_regular"},
		{name: "outside root", path: func(r string) string { return filepath.Join(filepath.Dir(r), "other") }, remaining: 100, wantCode: "not_regular"},
		{name: "relative against absolute", path: func(string) string { return "relative" }, remaining: 100, wantCode: "not_regular"},
		{
			name: "dot-prefixed name stays inside root", path: func(r string) string { return filepath.Join(r, "..log") }, remaining: 100,
			prepare: func(t *testing.T, r string) {
				if err := os.WriteFile(filepath.Join(r, "..log"), []byte("dots"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantBody: "dots",
		},
		{
			name: "missing root", remaining: 100, wantCode: "missing",
			rootPath: func(r string) string { return r + "-gone" },
			path:     func(r string) string { return filepath.Join(r+"-gone", "top.log") },
		},
		{
			name: "root is a file", remaining: 100, wantCode: "not_regular",
			rootPath: func(r string) string { return filepath.Join(r, "top.log") },
			path:     func(r string) string { return filepath.Join(r, "top.log", "x") },
		},
		{
			name: "root is a symlink", remaining: 100, wantCode: "not_regular",
			prepare: func(t *testing.T, r string) {
				if err := os.Symlink(r, r+"-link"); err != nil {
					t.Fatal(err)
				}
			},
			rootPath: func(r string) string { return r + "-link" },
			path:     func(r string) string { return filepath.Join(r+"-link", "top.log") },
		},
		{
			name: "root cannot be opened", remaining: 100, wantCode: "read_failed",
			prepare: func(t *testing.T, r string) {
				if err := os.Chmod(r, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(r, 0o700) })
			},
			path: func(r string) string { return filepath.Join(r, "top.log") },
		},
		{
			name: "missing parent", remaining: 100, wantCode: "missing", wantHooks: []string{"a"},
			path: func(r string) string { return filepath.Join(r, "a", "gone", "leaf.log") },
		},
		{
			name: "parent is a file", remaining: 100, wantCode: "not_regular",
			path: func(r string) string { return filepath.Join(r, "top.log", "leaf.log") },
		},
		{
			name: "parent is a symlink", remaining: 100, wantCode: "not_regular", wantHooks: []string{"a"},
			prepare: func(t *testing.T, r string) {
				if err := os.Symlink("b", filepath.Join(r, "a", "link")); err != nil {
					t.Fatal(err)
				}
			},
			path: func(r string) string { return filepath.Join(r, "a", "link", "leaf.log") },
		},
		{
			name: "parent cannot be opened", remaining: 100, wantCode: "read_failed", wantHooks: []string{"a"},
			prepare: func(t *testing.T, r string) {
				if err := os.Chmod(filepath.Join(r, "a", "b"), 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(filepath.Join(r, "a", "b"), 0o700) })
			},
			path: func(r string) string { return filepath.Join(r, "a", "b", "leaf.log") },
		},
		{
			name: "missing leaf", remaining: 100, wantCode: "missing", wantHooks: []string{"a"},
			path: func(r string) string { return filepath.Join(r, "a", "gone.log") },
		},
		{
			name: "leaf is a directory", remaining: 100, wantCode: "not_regular",
			path: func(r string) string { return filepath.Join(r, "a") },
		},
		{
			name: "leaf is a symlink", remaining: 100, wantCode: "not_regular",
			prepare: func(t *testing.T, r string) {
				if err := os.Symlink("top.log", filepath.Join(r, "link.log")); err != nil {
					t.Fatal(err)
				}
			},
			path: func(r string) string { return filepath.Join(r, "link.log") },
		},
		{
			name: "leaf is a fifo", remaining: 100, wantCode: "not_regular",
			prepare: func(t *testing.T, r string) {
				if err := syscall.Mkfifo(filepath.Join(r, "fifo.log"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			path: func(r string) string { return filepath.Join(r, "fifo.log") },
		},
		{
			name: "leaf cannot be opened", remaining: 100, wantCode: "read_failed",
			prepare: func(t *testing.T, r string) {
				if err := os.Chmod(filepath.Join(r, "top.log"), 0o000); err != nil {
					t.Fatal(err)
				}
			},
			path: func(r string) string { return filepath.Join(r, "top.log") },
		},
		{name: "one byte over the budget", path: func(r string) string { return filepath.Join(r, "top.log") }, remaining: 2, wantErr: ErrTooLarge},
		{name: "negative budget", path: func(r string) string { return filepath.Join(r, "top.log") }, remaining: -1, wantErr: ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeSourceFixture(t)
			if tc.prepare != nil {
				tc.prepare(t, root)
			}
			rootPath := root
			if tc.rootPath != nil {
				rootPath = tc.rootPath(root)
			}
			var hooks []string
			body, code, err := readAllowedSourceWithHook(rootPath, tc.path(root), tc.remaining, func(current string) {
				hooks = append(hooks, current)
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if code != tc.wantCode || string(body) != tc.wantBody {
				t.Fatalf("body %q code %q, want %q %q", body, code, tc.wantBody, tc.wantCode)
			}
			if !reflect.DeepEqual(hooks, tc.wantHooks) {
				t.Fatalf("hooks = %q, want %q", hooks, tc.wantHooks)
			}
		})
	}
}

// TestReadAllowedSourceDetectsLeafSwappedAtHook pins that the leaf checks run
// after the hook of its parent, so a leaf replaced at that hook is rejected.
func TestReadAllowedSourceDetectsLeafSwappedAtHook(t *testing.T) {
	root := writeSourceFixture(t)
	source := filepath.Join(root, "a", "b", "leaf.log")
	hook := func(current string) {
		if current != filepath.Join("a", "b") {
			return
		}
		if err := os.Remove(source); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "top.log"), source); err != nil {
			t.Fatal(err)
		}
	}
	body, code, err := readAllowedSourceWithHook(root, source, 100, hook)
	if err != nil || code != "not_regular" || body != nil {
		t.Fatalf("body %q code %q err %v", body, code, err)
	}
}

// TestReadAllowedSourceClosesDescriptorsOnEveryPath guards the root handle that
// moves down the walk and the leaf file.
func TestReadAllowedSourceClosesDescriptorsOnEveryPath(t *testing.T) {
	root := writeSourceFixture(t)
	if err := os.Symlink("b", filepath.Join(root, "a", "link")); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(root, "a", "b", "leaf.log"),
		filepath.Join(root, "a", "gone", "leaf.log"),
		filepath.Join(root, "a", "link", "leaf.log"),
		filepath.Join(root, "a", "b"),
		filepath.Join(root, "top.log", "x"),
	}
	readAll := func() {
		for _, path := range paths {
			for _, remaining := range []int64{100, 1} {
				_, _, _ = readAllowedSource(root, path, remaining)
			}
		}
	}
	countFDs := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skip("no /proc/self/fd")
		}
		return len(entries)
	}
	readAll()
	before := countFDs()
	for range 20 {
		readAll()
	}
	if after := countFDs(); after > before {
		t.Fatalf("descriptors grew from %d to %d", before, after)
	}
}
