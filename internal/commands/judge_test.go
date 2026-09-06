package commands

import (
	"context"
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/judge"
	"github.com/alekzonder/tariboy/internal/registry"
)

type recordingJudgeControl struct {
	iterations []string
	judges     int
}

func (r *recordingJudgeControl) OperatorReview(_ context.Context, iterations []string, judges int) (judge.Run, []judge.Target, error) {
	r.iterations, r.judges = iterations, judges
	return judge.Run{ID: "run-1", Status: judge.RunSnapshotting}, []judge.Target{{Iteration: "iteration-1"}}, nil
}
func (*recordingJudgeControl) OperatorList(judge.ListFilter) ([]judge.Run, error) { return nil, nil }
func (*recordingJudgeControl) OperatorInspect(string) (map[string]any, error)     { return nil, nil }
func (*recordingJudgeControl) OperatorEvidence(string, string, judge.EvidenceLocator) (map[string]any, error) {
	return nil, nil
}
func (*recordingJudgeControl) OperatorCancel(string) error { return nil }
func (*recordingJudgeControl) OperatorRetry(string) error  { return nil }

func TestJudgeReviewCommandPassesRepeatedExplicitIterations(t *testing.T) {
	control := &recordingJudgeControl{}
	command, ok := BuildRegistry().Get("judge.review")
	if !ok {
		t.Fatal("judge.review is not registered")
	}
	result, err := command.Handler(&registry.Ctx{Judges: control}, registry.Params{
		"iteration": []string{"iteration-1", "iteration-2"}, "judges_per_iteration": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(control.iterations, []string{"iteration-1", "iteration-2"}) || control.judges != 2 {
		t.Fatalf("iterations=%v judges=%d", control.iterations, control.judges)
	}
	got := result.(map[string]any)
	if got["id"] != "run-1" || got["status"] != judge.RunSnapshotting || got["targets"] != 1 || got["run"] != nil {
		t.Fatalf("result=%+v", got)
	}
}
