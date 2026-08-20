package cli_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/evals"
	"github.com/alekzonder/tariboy/internal/store"
)

func TestEvalCommandsSurfaceResults(t *testing.T) {
	base, _, c := startDaemon(t)

	// Seed a result directly into the daemon's DB (CLI + daemon share the base dir).
	s, err := store.Open(filepath.Join(base, "tariboyd.db"))
	if err != nil {
		t.Fatal(err)
	}
	es := evals.NewStore(s, func() time.Time { return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) })
	if err := es.Insert(evals.Result{
		Iteration: "scout-1", Agent: "scout", ImageName: "img", ImageTag: "latest",
		ImageDigest: "deadbeef", EvalName: "followed-task", EvalType: "llm-judge",
		Verdict: "pass", Score: 1, Detail: "good",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	ls := mustCall(t, c, "GET", "/api/evals", map[string]string{})
	rows, _ := ls["evals"].([]any)
	if len(rows) != 1 {
		t.Fatalf("eval ls rows = %v", ls["evals"])
	}
	insp := mustCall(t, c, "GET", "/api/evals/scout-1", map[string]string{})
	if insp["iteration"] != "scout-1" {
		t.Fatalf("eval inspect = %v", insp)
	}
	res, _ := insp["results"].([]any)
	if len(res) != 1 {
		t.Fatalf("eval inspect results = %v", insp["results"])
	}
	first, _ := res[0].(map[string]any)
	if first["verdict"] != "pass" || first["eval_name"] != "followed-task" || first["image_digest"] != "deadbeef" {
		t.Fatalf("result row = %v", first)
	}
}
