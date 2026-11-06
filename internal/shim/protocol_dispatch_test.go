package shim

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
)

// dispatchBaseHandler implements only the mandatory Handler methods.
type dispatchBaseHandler struct {
	err   error
	calls []string
}

func (h *dispatchBaseHandler) Status() StatusResult { return StatusResult{Running: true, PID: 7} }
func (h *dispatchBaseHandler) Kill() error {
	h.calls = append(h.calls, "kill")
	return h.err
}
func (h *dispatchBaseHandler) Screen() (string, error) { return "screen", h.err }
func (h *dispatchBaseHandler) SendKeys(p SendKeysParams) error {
	h.calls = append(h.calls, "sendkeys:"+p.Keys)
	return h.err
}
func (h *dispatchBaseHandler) Report() ReportResult { return ReportResult{Finished: true} }

// dispatchFullHandler also implements every optional handler interface.
type dispatchFullHandler struct{ dispatchBaseHandler }

func (h *dispatchFullHandler) Attach(net.Conn, AttachParams) error { return nil }
func (h *dispatchFullHandler) Resize(ResizeParams) error {
	h.calls = append(h.calls, "resize")
	return h.err
}
func (h *dispatchFullHandler) SetHardDeadline(deadline string) error {
	h.calls = append(h.calls, "deadline:"+deadline)
	return h.err
}
func (h *dispatchFullHandler) HarnessExited(token string) error {
	h.calls = append(h.calls, "exited:"+token)
	return h.err
}
func (h *dispatchFullHandler) HarnessStarted(_ int, token string) error {
	h.calls = append(h.calls, "started:"+token)
	return h.err
}

// TestDispatchWireContract pins the result, error code and error text of every
// method, including the difference between methods that accept absent params
// and methods that do not.
func TestDispatchWireContract(t *testing.T) {
	t.Parallel()
	const ok = `{"ok":true}`
	boom := errors.New("boom")
	for _, tc := range []struct {
		name      string
		base      bool // use the handler without the optional interfaces
		handleErr error
		method    string
		params    string
		want      string
		wantCode  int
		wantMsg   string // substring; stdlib JSON text is matched loosely
		wantCalls string
	}{
		{name: "status", method: MethodStatus, want: `{"running":true,"pid":7,"started_at":""}`},
		{name: "report", method: MethodReport, want: `{"finished":true}`},
		{name: "kill", method: MethodKill, want: ok, wantCalls: "kill"},
		{name: "kill failure", method: MethodKill, handleErr: boom, wantCode: -32000, wantMsg: "boom", wantCalls: "kill"},
		{name: "screen", method: MethodScreen, want: `{"screen":"screen"}`},
		{name: "screen failure", method: MethodScreen, handleErr: boom, wantCode: -32001, wantMsg: "boom"},
		{name: "sendkeys", method: MethodSendKeys, params: `{"keys":"x"}`, want: ok, wantCalls: "sendkeys:x"},
		{name: "sendkeys without params", method: MethodSendKeys, want: ok, wantCalls: "sendkeys:"},
		{name: "sendkeys bad params", method: MethodSendKeys, params: `{"keys":1}`, wantCode: -32602, wantMsg: "cannot unmarshal"},
		{name: "sendkeys failure", method: MethodSendKeys, params: `{}`, handleErr: boom, wantCode: -32002, wantMsg: "boom", wantCalls: "sendkeys:"},
		{name: "resize", method: MethodResize, params: `{"cols":80,"rows":24}`, want: ok, wantCalls: "resize"},
		{name: "resize unsupported", base: true, method: MethodResize, params: `{}`, wantCode: -32601, wantMsg: "resize unsupported"},
		{name: "resize without params", method: MethodResize, wantCode: -32602, wantMsg: "unexpected end of JSON input"},
		{name: "resize failure", method: MethodResize, params: `{}`, handleErr: boom, wantCode: -32003, wantMsg: "boom", wantCalls: "resize"},
		{name: "deadline", method: MethodSetHardDeadline, params: `{"deadline":"d"}`, want: ok, wantCalls: "deadline:d"},
		{name: "deadline unsupported", base: true, method: MethodSetHardDeadline, wantCode: -32601, wantMsg: "hard deadline updates unsupported"},
		{name: "deadline without params", method: MethodSetHardDeadline, wantCode: -32602, wantMsg: "unexpected end of JSON input"},
		{name: "deadline bad params", method: MethodSetHardDeadline, params: `{"deadline":1}`, wantCode: -32602, wantMsg: "cannot unmarshal"},
		{name: "deadline empty", method: MethodSetHardDeadline, params: `{}`, wantCode: -32602, wantMsg: "deadline is required"},
		{name: "deadline failure", method: MethodSetHardDeadline, params: `{"deadline":"d"}`, handleErr: boom, wantCode: -32602, wantMsg: "boom", wantCalls: "deadline:d"},
		{name: "exited", method: MethodHarnessExited, params: `{"token":"t"}`, want: ok, wantCalls: "exited:t"},
		{name: "exited unsupported", base: true, method: MethodHarnessExited, wantCode: -32601, wantMsg: "harness exit report unsupported"},
		{name: "exited without params reaches handler", method: MethodHarnessExited, want: ok, wantCalls: "exited:"},
		{name: "exited bad params reach handler", method: MethodHarnessExited, params: `{"token":1}`, want: ok, wantCalls: "exited:"},
		{name: "exited failure", method: MethodHarnessExited, params: `{"token":"t"}`, handleErr: boom, wantCode: -32602, wantMsg: "boom", wantCalls: "exited:t"},
		{name: "started", method: MethodHarnessStarted, params: `{"pid":3,"token":"t"}`, want: ok, wantCalls: "started:t"},
		{name: "started unsupported", base: true, method: MethodHarnessStarted, wantCode: -32601, wantMsg: "harness start report unsupported"},
		{name: "started without params", method: MethodHarnessStarted, wantCode: -32602, wantMsg: "unexpected end of JSON input"},
		{name: "started bad params", method: MethodHarnessStarted, params: `{"pid":"x"}`, wantCode: -32602, wantMsg: "cannot unmarshal"},
		{name: "started zero pid", method: MethodHarnessStarted, params: `{"pid":0,"token":"t"}`, wantCode: -32602, wantMsg: "pid is required"},
		{name: "started negative pid", method: MethodHarnessStarted, params: `{"pid":-1}`, wantCode: -32602, wantMsg: "pid is required"},
		{name: "started failure", method: MethodHarnessStarted, params: `{"pid":3,"token":"t"}`, handleErr: boom, wantCode: -32602, wantMsg: "boom", wantCalls: "started:t"},
		{name: "unknown", method: "nope", wantCode: -32601, wantMsg: "unknown method nope"},
		{name: "attach is not a one-shot method", method: MethodAttach, wantCode: -32601, wantMsg: "unknown method attach"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			full := &dispatchFullHandler{dispatchBaseHandler{err: tc.handleErr}}
			var h Handler = full
			if tc.base {
				h = &full.dispatchBaseHandler
			}
			req := Request{JSONRPC: "2.0", ID: 1, Method: tc.method}
			if tc.params != "" {
				req.Params = json.RawMessage(tc.params)
			}
			got, rpcErr := dispatch(h, req)
			if calls := strings.Join(full.calls, ","); calls != tc.wantCalls {
				t.Fatalf("handler calls = %q, want %q", calls, tc.wantCalls)
			}
			if tc.wantCode != 0 {
				if rpcErr == nil {
					t.Fatalf("result = %#v, want error %d", got, tc.wantCode)
				}
				if rpcErr.Code != tc.wantCode || !strings.Contains(rpcErr.Message, tc.wantMsg) {
					t.Fatalf("error = %d %q, want %d %q", rpcErr.Code, rpcErr.Message, tc.wantCode, tc.wantMsg)
				}
				if got != nil {
					t.Fatalf("result = %#v alongside error", got)
				}
				return
			}
			if rpcErr != nil {
				t.Fatalf("error = %+v", rpcErr)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Fatalf("result = %s, want %s", raw, tc.want)
			}
		})
	}
}
