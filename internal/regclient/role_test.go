package regclient

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/storesvc"
)

func TestPushThenPullKeepsRole(t *testing.T) {
	storeDir := t.TempDir()
	srv, err := storesvc.New(storesvc.Config{AllowInsecure: true, DataDir: storeDir, DBPath: filepath.Join(storeDir, "s.db")})
	if err != nil {
		t.Fatal(err)
	}
	srv.SeedToken("rw", storesvc.ScopeReadWrite)
	ts := httptest.NewServer(srv.Handler())
	defer func() { ts.Close(); srv.Close() }()
	cl, err := NewClient(ts.URL, "rw", "")
	if err != nil {
		t.Fatal(err)
	}

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nskills: []\nprompts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "ROLE.md"), []byte("# Reviewer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := imagefile.ParseV2(src)
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "demo", Tag: "latest"}
	srcImages := t.TempDir()
	if _, err := image.BuildV2(parsed, imagefile.ResolveRoots{}, ref, &image.Store{Dir: srcImages}, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), srcImages, ref, cl); err != nil {
		t.Fatalf("Push: %v", err)
	}
	dstImages := t.TempDir()
	if _, err := Pull(context.Background(), dstImages, ref, cl); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	pulled, err := (&image.Store{Dir: dstImages}).Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if pulled.Role == nil || pulled.Role.Text != "# Reviewer\n" {
		t.Fatalf("pulled role = %+v", pulled.Role)
	}
}
