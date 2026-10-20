package aiproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// This fails if error bodies keep the established byte layout for ordinary
// values: clients and tests compare these shapes directly.
func TestWriteJSONErrorKeepsBodyShapes(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONError(rr, http.StatusTooManyRequests, anthropicError("rate_limit_error", "budget exceeded: hour, day"))
	if got, want := rr.Body.String(), `{"type":"error","error":{"type":"rate_limit_error","message":"budget exceeded: hour, day"}}`; got != want {
		t.Fatalf("anthropic body = %s, want %s", got, want)
	}
	if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code=%d content-type=%q", rr.Code, rr.Header().Get("Content-Type"))
	}

	rr = httptest.NewRecorder()
	writeJSONError(rr, http.StatusTooManyRequests, connectError{Code: "resource_exhausted", Message: "budget exceeded"})
	if got, want := rr.Body.String(), `{"code":"resource_exhausted","message":"budget exceeded"}`; got != want {
		t.Fatalf("connect body = %s, want %s", got, want)
	}
}

// This fails if a message containing JSON metacharacters can break out of
// the error body.
func TestWriteJSONErrorEscapesMessage(t *testing.T) {
	msg := `budget exceeded: a"b\c","x":"y`
	rr := httptest.NewRecorder()
	writeJSONError(rr, http.StatusTooManyRequests, anthropicError("rate_limit_error", msg))
	var a struct {
		Type  string
		Error struct{ Type, Message string }
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &a); err != nil || a.Error.Message != msg {
		t.Fatalf("anthropic body = %s (err %v)", rr.Body.String(), err)
	}

	rr = httptest.NewRecorder()
	writeJSONError(rr, http.StatusTooManyRequests, connectError{Code: "resource_exhausted", Message: msg})
	var c struct{ Code, Message string }
	if err := json.Unmarshal(rr.Body.Bytes(), &c); err != nil || c.Message != msg {
		t.Fatalf("connect body = %s (err %v)", rr.Body.String(), err)
	}
}

// This fails if the budget_block audit payload is built by concatenation and
// an agent name with quotes or backslashes produces invalid JSON.
func TestBudgetBlockAuditEscapesScope(t *testing.T) {
	agent := `al"i\ce`
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	st := newStore(t)
	st.Insert(sampleReq("r1", agent, "basic", 0.80, now.Add(-time.Minute)))
	st.SetBudget(Budget{Scope: "agent:" + agent, LimitUSD: 0.50, PeriodS: 3600, Mode: "block"})
	cache := NewBudgetCache(st, func() time.Time { return now })
	cache.Refresh()

	p := testProxy(t)
	p.cfg.Budget = cache
	p.cfg.Store = st
	var auditData string
	p.cfg.Audit = func(_, kind, data string) {
		if kind == "budget_block" {
			auditData = data
		}
	}
	p.rebuild()
	p.forward = func(ex *Exchange) error { ex.W.WriteHeader(200); return nil }

	tok, _ := p.Mint(Attribution{Agent: agent, Iteration: "it-1", ImageName: "basic", ImageTag: "latest"})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{}`))
	p.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
	var body struct {
		Error struct{ Type, Message string }
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Error.Type != "rate_limit_error" {
		t.Fatalf("body = %s (err %v)", rr.Body.String(), err)
	}
	var audit struct{ Scope string }
	if err := json.Unmarshal([]byte(auditData), &audit); err != nil || audit.Scope != "agent:"+agent {
		t.Fatalf("audit data = %s (err %v)", auditData, err)
	}
}

// This fails if the proxy server stops accepting HTTP/2 prior-knowledge
// (h2c) connections or plain HTTP/1.1.
func TestProxyServesH2CPriorKnowledgeAndHTTP1(t *testing.T) {
	p := testProxy(t)
	addr, err := p.Listen()
	if err != nil {
		t.Fatal(err)
	}
	go p.Serve()
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	if p.server.ReadHeaderTimeout != 10*time.Second || p.server.IdleTimeout != 120*time.Second ||
		p.server.MaxHeaderBytes != 64<<10 || p.server.ReadTimeout != 0 || p.server.WriteTimeout != 0 {
		t.Fatalf("server timeouts = header %v idle %v maxHeader %d read %v write %v",
			p.server.ReadHeaderTimeout, p.server.IdleTimeout, p.server.MaxHeaderBytes,
			p.server.ReadTimeout, p.server.WriteTimeout)
	}

	h2Protocols := new(http.Protocols)
	h2Protocols.SetUnencryptedHTTP2(true)
	h2 := &http.Client{Transport: &http.Transport{Protocols: h2Protocols}}
	resp, err := h2.Get("http://" + addr + "/v1/messages")
	if err != nil {
		t.Fatalf("h2c request: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("h2c proto = %s, want HTTP/2", resp.Proto)
	}

	resp, err = http.Get("http://" + addr + "/v1/messages")
	if err != nil {
		t.Fatalf("http/1.1 request: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Fatalf("http1 proto = %s, want HTTP/1.1", resp.Proto)
	}
}
