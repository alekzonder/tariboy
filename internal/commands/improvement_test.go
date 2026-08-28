package commands

import (
	"context"
	"testing"

	"github.com/alekzonder/tariboy/internal/improvement"
	"github.com/alekzonder/tariboy/internal/registry"
)

type recordingImprovementControl struct {
	actor, id, hash, reason string
	decision                improvement.ApprovalDecision
}

func (r *recordingImprovementControl) List(context.Context) ([]improvement.Proposal, error) {
	return nil, nil
}
func (r *recordingImprovementControl) Get(context.Context, string) (improvement.Proposal, error) {
	return improvement.Proposal{}, nil
}
func (r *recordingImprovementControl) DecidePlan(_ context.Context, id, hash, actor string, decision improvement.ApprovalDecision, reason string) (improvement.Approval, error) {
	r.id, r.hash, r.actor, r.decision, r.reason = id, hash, actor, decision, reason
	return improvement.Approval{ProposalID: id, ObjectHash: hash, Actor: actor, Decision: decision}, nil
}

func TestImprovementPlanApprovalUsesContextOperator(t *testing.T) {
	control := &recordingImprovementControl{}
	cmd, ok := BuildRegistry().Get("improvement.plan.approve")
	if !ok {
		t.Fatal("improvement.plan.approve is not registered")
	}
	_, err := cmd.Handler(&registry.Ctx{Improvements: control, Operator: "alice@example.com"}, registry.Params{"id": "proposal-1", "revision": "sha256:revision", "reason": "reviewed", "actor": "mallory"})
	if err != nil {
		t.Fatal(err)
	}
	if control.actor != "alice@example.com" || control.id != "proposal-1" || control.hash != "sha256:revision" || control.decision != improvement.DecisionApprove {
		t.Fatalf("approval call = %+v", control)
	}
}
