package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/fsbrowser"
	"github.com/alekzonder/tariboy/internal/registry"
)

// TestFsListStatusCodes drives the fs.list handler end-to-end and asserts the
// typed error codes map to the right HTTP statuses (bad_path 403, not_found 404,
// not_dir 400) and that a valid listing returns directories only.
func TestFsListStatusCodes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TARIBOY_FS_ROOT", root)

	cmd := fsList()
	call := func(path string) (any, error) {
		return cmd.Handler(&registry.Ctx{}, registry.Params{"path": path})
	}

	// Happy path: directories only, files filtered out.
	res, err := call("")
	if err != nil {
		t.Fatalf("List root: %v", err)
	}
	l, ok := res.(fsbrowser.Listing)
	if !ok {
		t.Fatalf("List root result = %T, want fsbrowser.Listing", res)
	}
	if len(l.Entries) != 1 || l.Entries[0].Name != "proj" {
		t.Fatalf("entries = %+v, want [proj] (files filtered)", l.Entries)
	}

	cases := []struct {
		path       string
		wantCode   string
		wantStatus int
	}{
		{"/etc", "bad_path", 403},
		{"nope/missing", "not_found", 404},
		{"afile", "not_dir", 400},
	}
	for _, tc := range cases {
		_, err := call(tc.path)
		ue, ok := err.(api.UserError)
		if !ok {
			t.Errorf("List(%q) err = %T %v, want api.UserError", tc.path, err, err)
			continue
		}
		if ue.Code != tc.wantCode {
			t.Errorf("List(%q) code = %q, want %q", tc.path, ue.Code, tc.wantCode)
		}
		if ue.Status != tc.wantStatus {
			t.Errorf("List(%q) status = %d, want %d", tc.path, ue.Status, tc.wantStatus)
		}
	}
}
