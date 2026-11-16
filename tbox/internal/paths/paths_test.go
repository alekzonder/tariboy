package paths

import (
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveHome(t *testing.T) {
	h, err := Resolve(env(map[string]string{"TBOX_HOME": "/x/tb", "HOME": "/h"}))
	if err != nil || string(h) != "/x/tb" {
		t.Fatalf("TBOX_HOME: got %q, %v", h, err)
	}
	h, err = Resolve(env(map[string]string{"HOME": "/h"}))
	if err != nil || string(h) != filepath.Join("/h", ".tbox") {
		t.Fatalf("HOME fallback: got %q, %v", h, err)
	}
	if _, err = Resolve(env(nil)); err == nil {
		t.Fatal("expected error when neither is set")
	}
}

func TestHomeSubdirs(t *testing.T) {
	h := Home("/r")
	cases := map[string]string{
		h.ImagesDir():                  "/r/images",
		h.RunRoot():                    "/r/run",
		h.RunDir("sha256:ab", "codex"): "/r/run/sha256:ab/codex",
		h.StoresDir():                  "/r/stores",
		h.CacheDir():                   "/r/cache",
		h.SourcesDir():                 "/r/sources",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
