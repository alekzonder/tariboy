package improvement

import (
	"context"
	"testing"
)

func TestServiceApprovesExactPlanAsOperator(t *testing.T) {
	_, store := testStore(t)
	proposal, err := store.CreateProposal(context.Background(), CreateProposalRequest{JudgeRunID: "run", CreatorAgent: "judge", CreatorIteration: "iteration", Draft: validDraft()})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, nil)
	approval, err := service.DecidePlan(context.Background(), proposal.ID, proposal.RevisionHash, "operator@example.com", DecisionApprove, "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if approval.Actor != "operator@example.com" || approval.ObjectHash != proposal.RevisionHash {
		t.Fatalf("approval = %+v", approval)
	}
	if _, err := service.DecidePlan(context.Background(), proposal.ID, proposal.RevisionHash, "", DecisionApprove, ""); err == nil {
		t.Fatal("empty operator actor was accepted")
	}
}
