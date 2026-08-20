package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebHostMiddlewareAllowsLoopbackRejectsOthers(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := webHostMiddleware(next)

	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:8765", 200},
		{"localhost:8765", 200},
		{"[::1]:8765", 200},
		{"localhost:9993", 200}, // forwarded/tunneled to another local port
		{"127.0.0.1:9999", 200}, // any port on a loopback host is fine
		{"127.0.0.1", 200},      // bare loopback host, no port
		{"[::1]", 200},          // bare IPv6 loopback, no port
		{"evil.com:8765", http.StatusMisdirectedRequest},
		{"127.0.0.1.evil.com:8765", http.StatusMisdirectedRequest}, // not loopback
		{"", http.StatusMisdirectedRequest},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/agents", nil)
		req.Host = tc.host
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			b, _ := io.ReadAll(rr.Result().Body)
			t.Fatalf("host %q -> %d, want %d (%s)", tc.host, rr.Code, tc.want, b)
		}
	}
}
