package improvement

import (
	"context"
	"fmt"

	"github.com/alekzonder/tariboy/internal/bus"
)

type Service struct {
	store *Store
	bus   *bus.Bus
}

func NewService(store *Store, messages *bus.Bus) *Service {
	return &Service{store: store, bus: messages}
}

func (s *Service) List(ctx context.Context) ([]Proposal, error) { return s.store.ListProposals(ctx) }
func (s *Service) Get(ctx context.Context, id string) (Proposal, error) {
	return s.store.GetProposal(ctx, id)
}

func (s *Service) DecidePlan(ctx context.Context, id, hash, actor string, decision ApprovalDecision, reason string) (Approval, error) {
	if actor == "" {
		return Approval{}, fmt.Errorf("%w: operator actor is required", ErrInvalidProposal)
	}
	approval, err := s.store.DecidePlan(ctx, ApprovalRequest{ProposalID: id, ObjectHash: hash, Actor: actor, Decision: decision, Reason: reason})
	if err != nil || s.bus == nil {
		return approval, err
	}
	typ := "improvement.plan.approved"
	if decision == DecisionReject {
		typ = "improvement.plan.rejected"
	}
	_, err = s.bus.Publish(bus.Message{Channel: "system:improvements", Source: "system", Type: typ, Data: map[string]any{"proposal_id": id, "revision_hash": hash}})
	return approval, err
}
