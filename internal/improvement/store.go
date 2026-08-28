package improvement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	storedb "github.com/alekzonder/tariboy/internal/store"
	"github.com/google/uuid"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(store *storedb.Store, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: store.DB, now: now}
}

func (s *Store) CreateProposal(ctx context.Context, request CreateProposalRequest) (Proposal, error) {
	if request.JudgeRunID == "" || request.CreatorAgent == "" || request.CreatorIteration == "" {
		return Proposal{}, fmt.Errorf("%w: creator and Judge run are required", ErrInvalidProposal)
	}
	if err := ValidateDraft(request.Draft); err != nil {
		return Proposal{}, err
	}
	raw, err := json.Marshal(request.Draft)
	if err != nil {
		return Proposal{}, err
	}
	hash, err := CanonicalHash(request.Draft)
	if err != nil {
		return Proposal{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	proposal := Proposal{ID: uuid.NewString(), JudgeRunID: request.JudgeRunID, SummaryID: request.SummaryID, CreatorAgent: request.CreatorAgent, CreatorIteration: request.CreatorIteration, Draft: request.Draft, RevisionHash: hash, Status: StatusAwaitingPlanApproval, CreatedAt: now, UpdatedAt: now}
	_, err = s.db.ExecContext(ctx, `INSERT INTO improvement_proposals(id,judge_run_id,summary_id,creator_agent,creator_iteration,document_json,revision_hash,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, proposal.ID, proposal.JudgeRunID, proposal.SummaryID, proposal.CreatorAgent, proposal.CreatorIteration, string(raw), proposal.RevisionHash, proposal.Status, proposal.CreatedAt, proposal.UpdatedAt)
	return proposal, err
}

func (s *Store) GetProposal(ctx context.Context, id string) (Proposal, error) {
	var proposal Proposal
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT id,judge_run_id,summary_id,creator_agent,creator_iteration,document_json,revision_hash,status,branch,pull_request_url,head_commit,merged_commit,last_error,created_at,updated_at FROM improvement_proposals WHERE id=?`, id).Scan(&proposal.ID, &proposal.JudgeRunID, &proposal.SummaryID, &proposal.CreatorAgent, &proposal.CreatorIteration, &raw, &proposal.RevisionHash, &proposal.Status, &proposal.Branch, &proposal.PullRequestURL, &proposal.HeadCommit, &proposal.MergedCommit, &proposal.LastError, &proposal.CreatedAt, &proposal.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	if err := json.Unmarshal([]byte(raw), &proposal.Draft); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func (s *Store) ListProposals(ctx context.Context) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM improvement_proposals ORDER BY created_at DESC, rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	proposals := make([]Proposal, 0, len(ids))
	for _, id := range ids {
		proposal, err := s.GetProposal(ctx, id)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func allowedTransition(from, to Status) bool {
	allowed := map[Status][]Status{
		StatusAwaitingPlanApproval:    {StatusApproved, StatusRejected, StatusCancelled},
		StatusApproved:                {StatusImplementing, StatusCancelled, StatusFailed},
		StatusImplementing:            {StatusPullRequestOpen, StatusFailed, StatusCancelled},
		StatusPullRequestOpen:         {StatusMerged, StatusFailed, StatusCancelled},
		StatusMerged:                  {StatusImageBuilt, StatusFailed},
		StatusImageBuilt:              {StatusAwaitingRolloutApproval, StatusFailed},
		StatusAwaitingRolloutApproval: {StatusRolloutPending, StatusRejected, StatusCancelled},
		StatusRolloutPending:          {StatusRolledOut, StatusFailed},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func (s *Store) TransitionProposal(ctx context.Context, id, revisionHash string, to Status) (Proposal, error) {
	current, err := s.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	if current.RevisionHash != revisionHash {
		return Proposal{}, ErrRevisionMismatch
	}
	if !allowedTransition(current.Status, to) {
		return Proposal{}, ErrInvalidTransition
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE improvement_proposals SET status=?,updated_at=? WHERE id=? AND revision_hash=? AND status=?`, to, now, id, revisionHash, current.Status)
	if err != nil {
		return Proposal{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Proposal{}, err
	}
	if changed != 1 {
		return Proposal{}, ErrRevisionMismatch
	}
	return s.GetProposal(ctx, id)
}

func (s *Store) DecidePlan(ctx context.Context, request ApprovalRequest) (Approval, error) {
	if request.ProposalID == "" || request.ObjectHash == "" || request.Actor == "" || (request.Decision != DecisionApprove && request.Decision != DecisionReject) {
		return Approval{}, fmt.Errorf("%w: incomplete approval", ErrInvalidProposal)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Approval{}, err
	}
	defer tx.Rollback()
	var revision string
	var status Status
	if err := tx.QueryRowContext(ctx, `SELECT revision_hash,status FROM improvement_proposals WHERE id=?`, request.ProposalID).Scan(&revision, &status); errors.Is(err, sql.ErrNoRows) {
		return Approval{}, ErrNotFound
	} else if err != nil {
		return Approval{}, err
	}
	if revision != request.ObjectHash {
		return Approval{}, ErrRevisionMismatch
	}
	if status != StatusAwaitingPlanApproval {
		return Approval{}, ErrInvalidTransition
	}
	next := StatusApproved
	if request.Decision == DecisionReject {
		next = StatusRejected
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	approval := Approval{ID: uuid.NewString(), ProposalID: request.ProposalID, Phase: PhasePlan, ObjectHash: request.ObjectHash, Decision: request.Decision, Actor: request.Actor, Reason: request.Reason, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO improvement_approvals(id,proposal_id,phase,object_hash,decision,actor,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, approval.ID, approval.ProposalID, approval.Phase, approval.ObjectHash, approval.Decision, approval.Actor, approval.Reason, approval.CreatedAt); err != nil {
		return Approval{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE improvement_proposals SET status=?,updated_at=? WHERE id=? AND revision_hash=? AND status=?`, next, now, request.ProposalID, request.ObjectHash, StatusAwaitingPlanApproval)
	if err != nil {
		return Approval{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return Approval{}, err
		}
		return Approval{}, ErrRevisionMismatch
	}
	if err := tx.Commit(); err != nil {
		return Approval{}, err
	}
	return approval, nil
}
