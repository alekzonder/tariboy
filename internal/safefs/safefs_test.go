package safefs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCleanRel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string // empty means rejected
	}{
		{"plain file", "a", "a"},
		{"nested", "a/b/c.md", "a/b/c.md"},
		{"dot-dot prefix in a name", "..foo", "..foo"},
		{"dot-dot suffix in a name", "a/b..", "a/b.."},
		{"hidden file", ".claude/skills", ".claude/skills"},
		{"empty", "", ""},
		{"dot", ".", ""},
		{"bare dot-dot", "..", ""},
		{"leading dot-dot", "../a", ""},
		{"inner dot-dot", "a/../b", ""},
		{"trailing dot-dot", "a/..", ""},
		{"absolute", "/a", ""},
		{"backslash", `a\b`, ""},
		{"backslash traversal", `..\a`, ""},
		{"NUL", "a\x00b", ""},
		{"dot slash prefix", "./a", ""},
		{"double slash", "a//b", ""},
		{"trailing slash", "a/", ""},
		{"inner dot", "a/./b", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CleanRel(tc.in)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("CleanRel(%q) = %q, want error", tc.in, got)
				}
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("CleanRel(%q) error %v is not ErrInvalidPath", tc.in, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CleanRel(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}
}

type walked struct {
	rel string
	dir bool
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWalkRegular(t *testing.T) {
	t.Parallel()
	tree := map[string]string{
		"a.txt":         "aa",
		"b/c.txt":       "ccc",
		"b/d/e.txt":     "e",
		".git/HEAD":     "ref",
		"skip/ignored":  "x",
		"z/big.bin":     "0123456789",
		"metadata.json": "{}",
	}
	cases := []struct {
		name    string
		opts    WalkOptions
		want    []walked
		wantErr error
	}{
		{
			name: "files only in lexical order",
			opts: WalkOptions{},
			want: []walked{{".git/HEAD", false}, {"a.txt", false}, {"b/c.txt", false}, {"b/d/e.txt", false}, {"metadata.json", false}, {"skip/ignored", false}, {"z/big.bin", false}},
		},
		{
			name: "directories when asked",
			opts: WalkOptions{Dirs: true, Skip: func(rel string) bool { return rel == ".git" || rel == "skip" || rel == "z" || rel == "metadata.json" }},
			want: []walked{{"a.txt", false}, {"b", true}, {"b/c.txt", false}, {"b/d", true}, {"b/d/e.txt", false}},
		},
		{
			name: "skip a file and a directory",
			opts: WalkOptions{Skip: func(rel string) bool { return rel == "metadata.json" || rel == ".git" || rel == "skip" }},
			want: []walked{{"a.txt", false}, {"b/c.txt", false}, {"b/d/e.txt", false}, {"z/big.bin", false}},
		},
		{
			name:    "file size limit",
			opts:    WalkOptions{MaxFileSize: 9},
			wantErr: ErrFileTooLarge,
		},
		{
			name: "file size limit at the bound",
			opts: WalkOptions{MaxFileSize: 10},
			want: []walked{{".git/HEAD", false}, {"a.txt", false}, {"b/c.txt", false}, {"b/d/e.txt", false}, {"metadata.json", false}, {"skip/ignored", false}, {"z/big.bin", false}},
		},
		{
			name:    "file count limit",
			opts:    WalkOptions{MaxFiles: 6},
			wantErr: ErrTooManyFiles,
		},
		{
			name:    "total size limit",
			opts:    WalkOptions{MaxTotal: 20},
			wantErr: ErrTotalTooLarge,
		},
	}
	root := t.TempDir()
	writeTree(t, root, tree)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []walked
			err := WalkRegular(root, tc.opts, func(rel string, d fs.DirEntry, info fs.FileInfo) error {
				if d.IsDir() != info.IsDir() {
					t.Fatalf("%s: entry and info disagree", rel)
				}
				got = append(got, walked{rel, d.IsDir()})
				return nil
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("WalkRegular error = %v, want %v", err, tc.wantErr)
				}
				var pathErr *PathError
				if !errors.As(err, &pathErr) || pathErr.Rel == "" {
					t.Fatalf("WalkRegular error %v carries no path", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("WalkRegular: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("walked %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWalkRegularRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		plant   func(t *testing.T, root string)
		wantErr error
		wantRel string
	}{
		{
			name: "symlink to a file",
			plant: func(t *testing.T, root string) {
				if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "sub", "link")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrSymlink,
			wantRel: "sub/link",
		},
		{
			name: "symlink to a directory",
			plant: func(t *testing.T, root string) {
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "sub", "dirlink")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrSymlink,
			wantRel: "sub/dirlink",
		},
		{
			name: "FIFO",
			plant: func(t *testing.T, root string) {
				if err := syscall.Mkfifo(filepath.Join(root, "sub", "pipe"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrNonRegular,
			wantRel: "sub/pipe",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeTree(t, root, map[string]string{"a.txt": "a", "sub/b.txt": "b"})
			tc.plant(t, root)
			done := make(chan error, 1)
			go func() {
				done <- WalkRegular(root, WalkOptions{Dirs: true}, func(rel string, d fs.DirEntry, info fs.FileInfo) error {
					if rel == tc.wantRel {
						t.Errorf("unsafe entry %s reached the callback", rel)
					}
					return nil
				})
			}()
			var err error
			select {
			case err = <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("WalkRegular hung")
			}
			if !errors.Is(err, tc.wantErr) || !errors.Is(err, ErrUnsafeFile) {
				t.Fatalf("WalkRegular error = %v, want %v and ErrUnsafeFile", err, tc.wantErr)
			}
			var pathErr *PathError
			if !errors.As(err, &pathErr) || pathErr.Rel != tc.wantRel {
				t.Fatalf("WalkRegular error %v, want path %s", err, tc.wantRel)
			}
			if !strings.Contains(err.Error(), tc.wantRel) {
				t.Fatalf("error text %q does not name %s", err, tc.wantRel)
			}
		})
	}
}

func TestWalkRegularRoot(t *testing.T) {
	t.Parallel()
	real := t.TempDir()
	writeTree(t, real, map[string]string{"a.txt": "a"})
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	noop := func(string, fs.DirEntry, fs.FileInfo) error { return nil }
	if err := WalkRegular(link, WalkOptions{}, noop); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlinked root error = %v, want ErrSymlink", err)
	}
	if err := WalkRegular(filepath.Join(real, "a.txt"), WalkOptions{}, noop); err == nil {
		t.Fatal("file root accepted")
	}
	if err := WalkRegular(filepath.Join(real, "missing"), WalkOptions{}, noop); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing root error = %v, want ErrNotExist", err)
	}
}

func TestWalkRegularCallbackControl(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a/x": "x", "b/y": "y"})
	var got []string
	err := WalkRegular(root, WalkOptions{Dirs: true}, func(rel string, d fs.DirEntry, _ fs.FileInfo) error {
		got = append(got, rel)
		if rel == "a" {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b", "b/y"}) {
		t.Fatalf("walked %v, %v", got, err)
	}
	stop := errors.New("stop")
	if err := WalkRegular(root, WalkOptions{}, func(string, fs.DirEntry, fs.FileInfo) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("callback error = %v, want stop", err)
	}
}

func TestDecodeStrictJSON(t *testing.T) {
	t.Parallel()
	type doc struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name         string
		in           string
		want         doc
		wantTrailing bool
		wantErr      bool
	}{
		{name: "one object", in: `{"name":"x"}`, want: doc{Name: "x"}},
		{name: "surrounding whitespace", in: " \n{\"name\":\"x\"}\n\t", want: doc{Name: "x"}},
		{name: "unknown field", in: `{"name":"x","extra":1}`, wantErr: true},
		{name: "second value", in: `{"name":"x"}{"name":"y"}`, wantTrailing: true},
		{name: "trailing garbage", in: `{"name":"x"} garbage`, wantTrailing: true},
		{name: "trailing closing brace", in: `{"name":"x"}}`, wantTrailing: true},
		{name: "empty", in: ``, wantErr: true},
		{name: "syntax error", in: `{"name":`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got doc
			err := DecodeStrictJSON(strings.NewReader(tc.in), &got)
			switch {
			case tc.wantTrailing:
				if !errors.Is(err, ErrTrailingData) {
					t.Fatalf("error = %v, want ErrTrailingData", err)
				}
			case tc.wantErr:
				if err == nil || errors.Is(err, ErrTrailingData) {
					t.Fatalf("error = %v, want a decode error", err)
				}
			default:
				if err != nil || got != tc.want {
					t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
				}
			}
		})
	}
}
