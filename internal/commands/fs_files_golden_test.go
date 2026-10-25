package commands

import (
	"encoding/base64"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

var updateFsFilesGolden = flag.Bool("update-fs-files-golden", false, "rewrite testdata/fs_files_responses.golden.json")

// fsFilesFixture points TARIBOY_FS_ROOT at a new root holding the
// directories proj (with the child dir src and a file) and empty, and
// returns a context with its own base directory and the root.
func fsFilesFixture(t *testing.T) (*registry.Ctx, string) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"proj/src", "empty"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "proj", "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TARIBOY_FS_ROOT", root)
	return &registry.Ctx{BaseDir: t.TempDir()}, root
}

// Catches a typed fs.list or files.upload response changing the JSON a
// client sees: the set of keys, their JSON types, and their values must
// match the recorded responses.
func TestFsFilesResponsesMatchGolden(t *testing.T) {
	// Sequential: fsFilesFixture sets TARIBOY_FS_ROOT with t.Setenv.
	c, root := fsFilesFixture(t)
	calls := []struct {
		label, path string
		params      registry.Params
	}{
		{"fs.list root", "fs.list", registry.Params{}},
		{"fs.list child", "fs.list", registry.Params{"path": "proj"}},
		{"fs.list empty", "fs.list", registry.Params{"path": "~/empty"}},
		{"files.upload", "files.upload", registry.Params{"name": "notes.txt", "content": base64.StdEncoding.EncodeToString([]byte("hello"))}},
	}
	values := make([]any, len(calls))
	for i, call := range calls {
		value, err := h(t, call.path)(c, call.params)
		if err != nil {
			t.Fatalf("%s: %v", call.label, err)
		}
		values[i] = value
	}
	// The upload lands in one random directory under files/.
	uploads, err := os.ReadDir(filepath.Join(c.BaseDir, "files"))
	if err != nil || len(uploads) != 1 {
		t.Fatalf("upload directories: %v, %v", uploads, err)
	}
	recorder := newGoldenRecorder(t, nil, root, "$ROOT", c.BaseDir, "$BASE", uploads[0].Name(), "$UPLOAD")
	for i, call := range calls {
		recorder.record(call.label, values[i])
	}
	recorder.check("fs_files_responses.golden.json", *updateFsFilesGolden)
}

func init() { typedCallSets["fs-files"] = fsFilesTypedCalls }

// fsFilesTypedCalls covers fs.list at the root, where parent is empty, in an
// empty directory, and one server upload.
func fsFilesTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, _ := fsFilesFixture(t)
	return c, []typedCall{
		{"GET", "/api/fs/list", "/api/fs/list", nil},
		{"GET", "/api/fs/list", "/api/fs/list?path=empty", nil},
		{"PUT", "/api/files", "/api/files", map[string]any{"name": "notes.txt", "content": base64.StdEncoding.EncodeToString([]byte("hello"))}},
	}
}
