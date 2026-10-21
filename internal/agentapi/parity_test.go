package agentapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// TestDomainErrorStatusParity proves the operator API and the agent socket
// answer one domain error with the same status and code, and that neither
// sends the text of an unmapped error.
func TestDomainErrorStatusParity(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"script not found", script.ErrNotFound, 404, "not_found"},
		{"script active", script.ErrActive, 409, "script_active"},
		{"script mode", script.ErrMode, 409, "script_mode"},
		{"script conflict", script.ErrConflict, 409, "script_conflict"},
		{"bus not found", bus.ErrNotFound, 404, "not_found"},
		{"tasks 404", &tasks.Error{Status: 404, Code: "not_found", Msg: "task not found"}, 404, "not_found"},
		{"tasks 409", &tasks.Error{Status: 409, Code: "stale_revision", Msg: "stale"}, 409, "stale_revision"},
		{"user error default", api.UserError{Code: "bad_input", Msg: "bad"}, 400, "bad_input"},
		{"user error 403", api.UserError{Code: "forbidden", Msg: "no", Status: 403}, 403, "forbidden"},
		{"param error", &registry.ParamError{Name: "id", Code: "invalid_param", Msg: "argument id must be text"}, 400, "invalid_param"},
		{"plain error", errors.New("secret internal detail"), 500, "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := registry.New()
			if err := reg.Register(registry.Command{Path: "parity.fail", Summary: "fail",
				HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/parity"},
				Handler: func(*registry.Ctx, registry.Request) (any, error) {
					return nil, tc.err
				}}); err != nil {
				t.Fatal(err)
			}
			operator := api.NewServer(reg, &registry.Ctx{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
			opRec := httptest.NewRecorder()
			operator.Handler().ServeHTTP(opRec, httptest.NewRequest("POST", "/api/parity", strings.NewReader(`{}`)))

			agent := NewServer(Deps{Agent: "a", Plugins: []string{"scripts"},
				RemoveScript: func(string) error { return tc.err }})
			agRec := httptest.NewRecorder()
			agent.Handler().ServeHTTP(agRec, httptest.NewRequest("POST", "/tools/script/rm", strings.NewReader(`{"id":"x"}`)))

			for side, rec := range map[string]*httptest.ResponseRecorder{"operator": opRec, "agent": agRec} {
				var env struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
					t.Fatalf("%s body %q: %v", side, rec.Body.String(), err)
				}
				if rec.Code != tc.status || env.Error.Code != tc.code {
					t.Errorf("%s = %d %q; want %d %q", side, rec.Code, env.Error.Code, tc.status, tc.code)
				}
				if strings.Contains(rec.Body.String(), "secret internal detail") {
					t.Errorf("%s leaks the internal error: %s", side, rec.Body.String())
				}
			}
		})
	}
}

// TestAgentBadJSONHidesDecoderText proves a malformed body is answered with a
// fixed message instead of the decoder's error text.
func TestAgentBadJSONHidesDecoderText(t *testing.T) {
	s := NewServer(Deps{Agent: "a", Plugins: []string{"scripts"}, RemoveScript: func(string) error { return nil }})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/tools/script/rm", strings.NewReader(`{"id":`)))
	ok, e := decode(t, rec.Body.Bytes())
	if ok || rec.Code != http.StatusBadRequest || e["code"] != "bad_json" || e["message"] != "request body is not valid JSON" {
		t.Fatalf("bad json = %d %v", rec.Code, e)
	}
}
