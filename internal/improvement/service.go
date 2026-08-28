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
func (s *Service) GetRelease(ctx context.Context, id string) (Release, error) {
	return s.store.GetRelease(ctx, id)
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

func (s *Service) DecideRollout(ctx context.Context, releaseID, hash, actor string, decision ApprovalDecision, reason string) (Approval, error) {
	approval, err := s.store.DecideRollout(ctx, releaseID, hash, actor, decision, reason)
	if err == nil && s.bus != nil {
		typ := "image.rollout.approved"
		if decision == DecisionReject {
			typ = "image.rollout.rejected"
		}
		_, err = s.bus.Publish(bus.Message{Channel: "system:improvements", Source: "system", Type: typ, Data: map[string]any{"release_id": releaseID, "release_hash": hash}})
	}
	return approval, err
}

func (s *Service) StageSingleRollout(ctx context.Context, releaseID, agent, hash string) (Rollout, error) {
	return s.store.StageSingleRollout(ctx, releaseID, agent, hash)
}

func (s *Service) StageRollback(ctx context.Context, rolloutID string) (Rollout, error) {
	return s.store.StageRollback(ctx, rolloutID)
}
