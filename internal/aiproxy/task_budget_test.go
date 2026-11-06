package aiproxy

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskBudgetRefusesARequestOfAnExhaustedTask(t *testing.T) {
	t.Parallel()
	agentsDir := t.TempDir()
	mkIter(t, agentsDir, "dev", "dev-1")
	var forwarded atomic.Int32
	var checked, exhausted []string
	var rows []AIRequest
	limit := "TASK-root"
	var checkErr error
	p := New(Config{
		Tokens: NewTokenRegistry(nil), Pricing: &Pricing{table: DefaultPricing()}, Store: newStore(t),
		Router: NewRouter(), AgentsDir: agentsDir, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Ingest: func(r AIRequest) { rows = append(rows, r) },
		TaskBudget: func(key string) (string, bool, error) {
			checked = append(checked, key)
			return limit, limit != "", checkErr
		},
		TaskBudgetExhausted: func(key string) { exhausted = append(exhausted, key) },
	})
	p.forward = func(ex *Exchange) error {
		forwarded.Add(1)
		ex.W.WriteHeader(http.StatusTeapot)
		return nil
	}
	send := func(attr Attribution) *httptest.ResponseRecorder {
		t.Helper()
		tok, err := p.Mint(attr)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/_tariboy/"+tok+"/v1/messages", strings.NewReader(`{"model":"claude-x"}`))
		rr := httptest.NewRecorder()
		p.ServeHTTP(rr, req)
		return rr
	}

	rr := send(Attribution{Agent: "dev", Iteration: "dev-1", TaskID: "TASK-child", EpicID: "TASK-root"})
	if rr.Code != http.StatusTooManyRequests || !strings.Contains(rr.Body.String(), "budget exceeded: task TASK-root") {
		t.Fatalf("refusal = %d %s", rr.Code, rr.Body.String())
	}
	if forwarded.Load() != 0 || len(rows) != 1 || rows[0].Status != "budget_block" {
		t.Fatalf("forwarded=%d rows=%#v", forwarded.Load(), rows)
	}
	if len(checked) != 1 || checked[0] != "TASK-child" || len(exhausted) != 1 || exhausted[0] != "TASK-child" {
		t.Fatalf("checked=%v exhausted=%v", checked, exhausted)
	}

	// A request without a task is not checked; a failed check lets it through.
	if rr := send(Attribution{Agent: "dev", Iteration: "dev-1"}); rr.Code != http.StatusTeapot || len(checked) != 1 {
		t.Fatalf("untagged = %d checked=%v", rr.Code, checked)
	}
	checkErr, limit = errors.New("db down"), ""
	if rr := send(Attribution{Agent: "dev", Iteration: "dev-1", TaskID: "TASK-child"}); rr.Code != http.StatusTeapot || len(exhausted) != 1 {
		t.Fatalf("failed check = %d exhausted=%v", rr.Code, exhausted)
	}
}
