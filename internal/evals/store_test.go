package evals

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

func newEvalStore(t *testing.T) *Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewStore(s, func() time.Time { return time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC) })
}

func TestEvalStoreInsertUpsertAndList(t *testing.T) {
	s := newEvalStore(t)
	r := Result{
		ID: newID(nil), Iteration: "scout-20260705-1", Agent: "scout",
		ImageName: "img", ImageTag: "latest", ImageDigest: "deadbeef",
		EvalName: "followed-task", EvalType: "llm-judge", Verdict: "pass", Score: 1,
		Detail: "looks good",
	}
	if err := s.Insert(r); err != nil {
		t.Fatal(err)
	}
	// Re-running the SAME eval (same iteration+digest+name) upserts, not duplicates.
	r.Verdict = "fail"
	r.Score = 0
	r.ID = newID(nil)
	if err := s.Insert(r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get("scout-20260705-1", "deadbeef", "followed-task")
	if err != nil || !ok || got.Verdict != "fail" || got.Score != 0 {
		t.Fatalf("get = %+v ok=%v err=%v", got, ok, err)
	}
	// A second, distinct eval name on the same iteration coexists.
	if err := s.Insert(Result{
		ID: newID(nil), Iteration: "scout-20260705-1", ImageDigest: "deadbeef",
		EvalName: "builds", EvalType: "script", Verdict: "pass", Score: 1,
	}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListByIteration("scout-20260705-1")
	if err != nil || len(list) != 2 || list[0].EvalName != "builds" || list[1].EvalName != "followed-task" {
		t.Fatalf("ListByIteration = %+v err=%v", list, err)
	}
	recent, err := s.List(10)
	if err != nil || len(recent) != 2 {
		t.Fatalf("List = %+v err=%v", recent, err)
	}
	if got.CreatedAt == "" {
		t.Fatal("CreatedAt not defaulted")
	}
}
