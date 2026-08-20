// Package storeui embeds the built tariboy-store single-page UI and serves it
// over HTTP with SPA fallback, confined to the embedded filesystem (no
// path-traversal escape). It is the only UI any Go binary still serves:
// tariboyd is API/WS only, and its SPA ships inside the desktop app instead.
package storeui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler returns an http.Handler serving the embedded dist/ tree: exact-match
// files (assets, index.html) are served with their content type; any other path
// falls back to dist/index.html so client-side routes (e.g. /repo/demo) load the
// SPA. Paths are cleaned and looked up only inside the embedded FS, so a "../"
// request cannot escape the embed.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean == "" || clean == "." {
			serveIndex(w)
			return
		}
		if f, err := sub.Open(clean); err == nil {
			st, serr := f.Stat()
			_ = f.Close()
			if serr == nil && !st.IsDir() {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + clean
				fileServer.ServeHTTP(w, r2)
				return
			}
		}
		serveIndex(w)
	}), nil
}
