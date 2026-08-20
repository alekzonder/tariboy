package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestEvalClientEvaluate(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req EvalRequestDTO
		_ = json.Unmarshal(body, &req)
		if req.EvalName != "followed-task" {
			http.Error(w, "bad req", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(EvalVerdictDTO{Verdict: "pass", Score: 1, Detail: "ok " + req.Status})
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := NewEvalClient(sock)
	got, err := c.Evaluate(ctx, EvalRequestDTO{EvalName: "followed-task", Status: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != "pass" || got.Score != 1 || got.Detail != "ok done" {
		t.Fatalf("verdict = %+v", got)
	}
}
