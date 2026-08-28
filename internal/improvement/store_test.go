package improvement

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storedb "github.com/alekzonder/tariboy/internal/store"
)

func testStore(t *testing.T) (*storedb.Store, *Store) {
	t.Helper()
	db, err := storedb.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := func() time.Time { return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC) }
	return db, NewStore(db, now)
}

func TestStoreCreatesAndTransitionsProposalWithRevisionGuard(t *testing.T) {
	_, store := testStore(t)
	created, err := store.CreateProposal(context.Background(), CreateProposalRequest{
		JudgeRunID: "judge-run-1", SummaryID: "summary-1", CreatorAgent: "judge-lead", CreatorIteration: "judge-it", Draft: validDraft(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.RevisionHash == "" || created.Status != StatusAwaitingPlanApproval {
		t.Fatalf("created proposal = %+v", created)
	}
	approved, err := store.TransitionProposal(context.Background(), created.ID, created.RevisionHash, StatusApproved)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != StatusApproved {
		t.Fatalf("status = %s", approved.Status)
	}
	if _, err := store.TransitionProposal(context.Background(), created.ID, "sha256:wrong", StatusImplementing); err == nil {
		t.Fatal("transition accepted the wrong revision hash")
	}
}

func TestPlanApprovalBindsRevisionAndIsAppendOnly(t *testing.T) {
	db, store := testStore(t)
	proposal, err := store.CreateProposal(context.Background(), CreateProposalRequest{
		JudgeRunID: "judge-run-1", CreatorAgent: "judge-lead", CreatorIteration: "judge-it", Draft: validDraft(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DecidePlan(context.Background(), ApprovalRequest{ProposalID: proposal.ID, ObjectHash: "sha256:wrong", Decision: DecisionApprove, Actor: "operator"}); err == nil {
		t.Fatal("approval accepted the wrong proposal hash")
	}
	approval, err := store.DecidePlan(context.Background(), ApprovalRequest{ProposalID: proposal.ID, ObjectHash: proposal.RevisionHash, Decision: DecisionApprove, Actor: "operator", Reason: "scope accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if approval.ID == "" || approval.Phase != PhasePlan || approval.ObjectHash != proposal.RevisionHash {
		t.Fatalf("approval = %+v", approval)
	}
	updated, err := store.GetProposal(context.Background(), proposal.ID)
	if err != nil || updated.Status != StatusApproved {
		t.Fatalf("proposal after approval = %+v, %v", updated, err)
	}
	if _, err := db.DB.Exec(`UPDATE improvement_approvals SET reason='rewritten' WHERE id=?`, approval.ID); err == nil {
		t.Fatal("approval row was mutable")
	}
	if _, err := db.DB.Exec(`DELETE FROM improvement_approvals WHERE id=?`, approval.ID); err == nil {
		t.Fatal("approval row was deletable")
	}
}

func TestListProposalsNewestFirst(t *testing.T) {
	_, store := testStore(t)
	first, err := store.CreateProposal(context.Background(), CreateProposalRequest{JudgeRunID: "run-1", CreatorAgent: "judge", CreatorIteration: "it-1", Draft: validDraft()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateProposal(context.Background(), CreateProposalRequest{JudgeRunID: "run-2", CreatorAgent: "judge", CreatorIteration: "it-2", Draft: validDraft()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ListProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("ListProposals() = %#v", got)
	}
}
