package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseListenUnix(t *testing.T) {
	l, err := ParseListen("unix", "")
	if err != nil || l.Network != "unix" || l.Addr != "" {
		t.Fatalf("l=%+v err=%v", l, err)
	}
	l, err = ParseListen("unix:/tmp/x.sock", "")
	if err != nil || l.Addr != "/tmp/x.sock" {
		t.Fatalf("l=%+v err=%v", l, err)
	}
}

func TestParseListenLoopbackTCPNoAuth(t *testing.T) {
	l, err := ParseListen("tcp:127.0.0.1:7411", "")
	if err != nil || l.Network != "tcp" || l.Addr != "127.0.0.1:7411" {
		t.Fatalf("l=%+v err=%v", l, err)
	}
}

func TestParseListenPublicTCPRequiresAuth(t *testing.T) {
	if _, err := ParseListen("tcp:0.0.0.0:7411", ""); err == nil {
		t.Fatal("public tcp without auth accepted")
	}
	tokFile := filepath.Join(t.TempDir(), "tok")
	os.WriteFile(tokFile, []byte("  secret-token\n"), 0o600)
	l, err := ParseListen("tcp:0.0.0.0:7411", tokFile)
	if err != nil || l.AuthToken != "secret-token" {
		t.Fatalf("l=%+v err=%v", l, err)
	}
}

func TestParseListenBadSpec(t *testing.T) {
	for _, spec := range []string{"", "udp:1.2.3.4:1", "tcp:", "tcp:hostonly"} {
		if _, err := ParseListen(spec, ""); err == nil {
			t.Fatalf("spec %q accepted", spec)
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := AuthMiddleware("tok", next)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/x", nil))
	if rr.Code != 401 {
		t.Fatalf("no header: code=%d, want 401", rr.Code)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("valid token rejected: %d", rr.Code)
	}

	// empty token = auth disabled
	rr = httptest.NewRecorder()
	AuthMiddleware("", next).ServeHTTP(rr, httptest.NewRequest("GET", "/api/x", nil))
	if rr.Code != 200 {
		t.Fatalf("auth-disabled path broken: %d", rr.Code)
	}
}
