package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/script"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func TestErrorResponseMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		msg    string
	}{
		{"user error default", UserError{Code: "bad", Msg: "bad input"}, 400, "bad", "bad input"},
		{"user error status", UserError{Code: "forbidden", Msg: "no", Status: 403}, 403, "forbidden", "no"},
		{"wrapped user error", fmt.Errorf("ctx: %w", UserError{Code: "bad", Msg: "m"}), 400, "bad", "m"},
		{"tasks error", &tasks.Error{Status: 409, Code: "stale", Msg: "revision changed"}, 409, "stale", "revision changed"},
		{"param error", &registry.ParamError{Name: "x", Code: "missing_param", Msg: "missing required argument: x"}, 400, "missing_param", "missing required argument: x"},
		{"script not found", fmt.Errorf("load: %w", script.ErrNotFound), 404, "not_found", "script or run not found"},
		{"script active", fmt.Errorf("cannot remove script %q: %w", "/secret/path", script.ErrActive), 409, "script_active", script.ErrActive.Error()},
		{"script mode", script.ErrMode, 409, "script_mode", script.ErrMode.Error()},
		{"script conflict", script.ErrConflict, 409, "script_conflict", script.ErrConflict.Error()},
		{"script invalid", fmt.Errorf("%w: interval must be positive", script.ErrInvalid), 400, "invalid_script", "invalid script: interval must be positive"},
		{"schedule not found", schedule.ErrNotFound, 404, "not_found", "schedule not found"},
		{"schedule invalid", fmt.Errorf("%w: bad cron", schedule.ErrInvalid), 400, "invalid_schedule", "invalid schedule: bad cron"},
		{"bus not found", fmt.Errorf("unsubscribe: %w", bus.ErrNotFound), 404, "not_found", "not found"},
		{"bus deadline unsupported", bus.ErrDeadlineUnsupported, 400, "deadline_unsupported", bus.ErrDeadlineUnsupported.Error()},
		{"bus invalid", fmt.Errorf("%w: bad deadline", bus.ErrInvalid), 400, "invalid_request", "invalid request: bad deadline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code, msg, _, ok := ErrorResponse(tc.err)
			if !ok || status != tc.status || code != tc.code || msg != tc.msg {
				t.Fatalf("ErrorResponse = %d %q %q ok=%v; want %d %q %q", status, code, msg, ok, tc.status, tc.code, tc.msg)
			}
		})
	}
	if _, _, _, _, ok := ErrorResponse(errors.New("disk on fire")); ok {
		t.Fatal("a plain error must not have a public mapping")
	}
	_, _, _, data, _ := ErrorResponse(&tasks.Error{Status: 409, Code: "c", Data: map[string]any{"k": "v"}})
	if data["k"] != "v" {
		t.Fatalf("tasks error data = %v", data)
	}
}

func TestWriteErrorHidesInternalDetail(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	WriteError(rec, slog.New(slog.NewTextHandler(&logs, nil)), "loop.done", errors.New("secret internal detail"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret internal detail") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if code, msg := errorBody(t, rec); code != "internal" || msg != "internal error, see daemon log" {
		t.Fatalf("envelope = %q %q", code, msg)
	}
	if !strings.Contains(logs.String(), "secret internal detail") || !strings.Contains(logs.String(), "loop.done") {
		t.Fatalf("log = %q", logs.String())
	}
	// A nil logger is safe.
	rec = httptest.NewRecorder()
	WriteError(rec, nil, "x", errors.New("boom"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("nil logger status = %d", rec.Code)
	}
}

func TestDecodeJSONBody(t *testing.T) {
	decode := func(r *http.Request) (*httptest.ResponseRecorder, bool, map[string]any) {
		rec := httptest.NewRecorder()
		v := map[string]any{}
		ok := DecodeJSONBody(rec, r, 16, &v)
		return rec, ok, v
	}
	if _, ok, v := decode(httptest.NewRequest("POST", "/", strings.NewReader(`{"a":1}`))); !ok || v["a"] != float64(1) {
		t.Fatalf("valid body ok=%v v=%v", ok, v)
	}
	if _, ok, _ := decode(httptest.NewRequest("POST", "/", nil)); !ok {
		t.Fatal("an empty body means no params")
	}
	rec, ok, _ := decode(httptest.NewRequest("POST", "/", strings.NewReader(`{"a":`)))
	if code, msg := errorBody(t, rec); ok || rec.Code != 400 || code != "bad_json" || msg != "request body is not valid JSON" {
		t.Fatalf("bad json = %v %d %q %q", ok, rec.Code, code, msg)
	}
	rec, ok, _ = decode(httptest.NewRequest("POST", "/", strings.NewReader(`{"a":"0123456789abcdef"}`)))
	if code, _ := errorBody(t, rec); ok || rec.Code != http.StatusRequestEntityTooLarge || code != "too_large" {
		t.Fatalf("declared oversize = %v %d %q", ok, rec.Code, code)
	}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"a":"0123456789abcdef"}`))
	req.ContentLength = -1
	rec, ok, _ = decode(req)
	if code, _ := errorBody(t, rec); ok || rec.Code != http.StatusRequestEntityTooLarge || code != "too_large" {
		t.Fatalf("streamed oversize = %v %d %q", ok, rec.Code, code)
	}
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return env.Error.Code, env.Error.Message
}
