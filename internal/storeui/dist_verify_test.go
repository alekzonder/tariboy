package storeui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// hashedAssetRef matches a Vite-style build output filename, e.g.
// index-DfzTSbDK.js or index-tJCfNDVb.css. A hand-written placeholder
// index.html (no bundler pass) will never contain this.
var hashedAssetRef = regexp.MustCompile(`assets/[\w.-]+-[A-Za-z0-9_-]{6,}\.(js|css)`)

// The committed dist must contain a real index.html and at least one built asset,
// so a missing/empty `make store-ui` output fails CI (make test) rather than
// shipping a blank store UI in the static binary.
//
// This also guards against an accidental revert to the Task 4 hand-written
// placeholder (which has `id="root"` but no bundler-hashed asset references),
// which the earlier version of this test could not detect.
func TestEmbeddedDistIsReal(t *testing.T) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	idx, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("dist/index.html missing: %v", err)
	}
	if !strings.Contains(string(idx), `id="root"`) {
		t.Fatalf("index.html has no root div (placeholder shipped?)")
	}
	if !hashedAssetRef.MatchString(string(idx)) {
		t.Fatalf("index.html has no hashed assets/*.{js,css} reference (placeholder shipped instead of a real build? run `make store-ui`)")
	}
	var assetCount int
	var hashedCount int
	_ = fs.WalkDir(sub, "assets", func(p string, d fs.DirEntry, err error) error {
		if err == nil && d != nil && !d.IsDir() {
			assetCount++
			name := d.Name()
			if (strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css")) &&
				hashedAssetRef.MatchString("assets/"+name) {
				hashedCount++
			}
		}
		return nil
	})
	if assetCount == 0 {
		t.Fatalf("dist/assets has no built files — run `make store-ui`")
	}
	if hashedCount == 0 {
		t.Fatalf("dist/assets has no hashed .js/.css build output (placeholder shipped? run `make store-ui`)")
	}
}
