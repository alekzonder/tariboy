package agentapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/version"
)

// The agent-facing socket stamps the same version header as the operator API,
// on successful and failing responses alike: the shim an agent runs may be
// years older than the daemon answering it (SUPER-224 §4).
func TestAgentAPIStampsVersionHeader(t *testing.T) {
	srv := NewServer(Deps{
		Agent: "smoke", Cwd: "/tmp", Plugins: []string{"whoami", "loop"},
		CurrentIteration: func() string { return "iter-1" },
	})
	h := srv.Handler()

	cases := []struct {
		name   string
		req    *http.Request
		status int
	}{
		{"ok", httptest.NewRequest("GET", "/tools/whoami", nil), http.StatusOK},
		{"plugin disabled", httptest.NewRequest("POST", "/tools/judge/action/work.claim", bytes.NewBufferString(`{}`)), http.StatusNotFound},
		{"unknown route", httptest.NewRequest("GET", "/tools/nope", nil), http.StatusNotFound},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, tc.req)
		if rr.Code != tc.status {
			t.Fatalf("%s: status=%d, want %d", tc.name, rr.Code, tc.status)
		}
		if got := rr.Result().Header.Get(version.Header); got != version.Version {
			t.Fatalf("%s: header=%q, want %q", tc.name, got, version.Version)
		}
	}
}

// whoami reports the daemon's version in its result, so `tools whoami` can show
// both sides of the conversation.
func TestAgentAPIWhoamiReportsDaemonVersion(t *testing.T) {
	srv := NewServer(Deps{
		Agent: "smoke", Cwd: "/tmp", Plugins: []string{"whoami"},
		CurrentIteration: func() string { return "iter-1" },
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/tools/whoami", nil))
	ok, res := decode(t, rr.Body.Bytes())
	if !ok {
		t.Fatalf("whoami failed: %v", res)
	}
	if res["daemon_version"] != version.Version {
		t.Fatalf("daemon_version=%v, want %q (body %s)", res["daemon_version"], version.Version, strings.TrimSpace(rr.Body.String()))
	}
}
