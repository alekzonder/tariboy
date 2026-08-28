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

func (s *Store) RecordRelease(ctx context.Context, release Release, proposalRevision string) (Release, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()
	var approvals int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM improvement_approvals WHERE proposal_id=? AND phase='plan' AND object_hash=? AND decision='approve'`, release.ProposalID, proposalRevision).Scan(&approvals); err != nil {
		return Release{}, err
	}
	if approvals == 0 {
		return Release{}, ErrInvalidTransition
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO image_releases(id,proposal_id,repository_id,git_commit,source_name,source_digest,lock_digest,prompt_template_digest,image_ref,image_digest,builder_version,release_hash,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, release.ID, release.ProposalID, release.RepositoryID, release.GitCommit, release.SourceName, release.SourceDigest, release.LockDigest, release.PromptTemplateDigest, release.ImageRef, release.ImageDigest, release.BuilderVersion, release.ReleaseHash, release.Status, release.CreatedAt)
	if err != nil {
		return Release{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE improvement_proposals SET status='image_built',updated_at=? WHERE id=? AND revision_hash=? AND status='merged' AND merged_commit=?`, release.CreatedAt, release.ProposalID, proposalRevision, release.GitCommit)
	if err != nil {
		return Release{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return Release{}, err
		}
		return Release{}, ErrInvalidTransition
	}
	if err := tx.Commit(); err != nil {
		return Release{}, err
	}
	return release, nil
}

func (s *Store) GetRelease(ctx context.Context, id string) (Release, error) {
	var release Release
	err := s.db.QueryRowContext(ctx, `SELECT id,proposal_id,repository_id,git_commit,source_name,source_digest,lock_digest,prompt_template_digest,image_ref,image_digest,builder_version,release_hash,status,created_at FROM image_releases WHERE id=?`, id).Scan(&release.ID, &release.ProposalID, &release.RepositoryID, &release.GitCommit, &release.SourceName, &release.SourceDigest, &release.LockDigest, &release.PromptTemplateDigest, &release.ImageRef, &release.ImageDigest, &release.BuilderVersion, &release.ReleaseHash, &release.Status, &release.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	return release, err
}

func (s *Store) ListReleases(ctx context.Context, proposalID string) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM image_releases WHERE proposal_id=? ORDER BY created_at DESC,id DESC`, proposalID)
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
	releases := make([]Release, 0, len(ids))
	for _, id := range ids {
		release, err := s.GetRelease(ctx, id)
		if err != nil {
			return nil, err
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func (s *Store) DecideRollout(ctx context.Context, releaseID, hash, actor string, decision ApprovalDecision, reason string) (Approval, error) {
	if actor == "" || (decision != DecisionApprove && decision != DecisionReject) {
		return Approval{}, ErrInvalidProposal
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Approval{}, err
	}
	defer tx.Rollback()
	var proposalID, releaseHash string
	var status Status
	if err := tx.QueryRowContext(ctx, `SELECT proposal_id,release_hash,status FROM image_releases WHERE id=?`, releaseID).Scan(&proposalID, &releaseHash, &status); errors.Is(err, sql.ErrNoRows) {
		return Approval{}, ErrNotFound
	} else if err != nil {
		return Approval{}, err
	}
	if releaseHash != hash {
		return Approval{}, ErrRevisionMismatch
	}
	if status != StatusImageBuilt {
		return Approval{}, ErrInvalidTransition
	}
	var decisions int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM improvement_approvals WHERE proposal_id=? AND phase='rollout' AND object_hash=?`, proposalID, hash).Scan(&decisions); err != nil {
		return Approval{}, err
	}
	if decisions != 0 {
		return Approval{}, ErrInvalidTransition
	}
	approval := Approval{ID: uuid.NewString(), ProposalID: proposalID, Phase: PhaseRollout, ObjectHash: hash, Decision: decision, Actor: actor, Reason: reason, CreatedAt: s.now().UTC().Format(time.RFC3339Nano)}
	if _, err := tx.ExecContext(ctx, `INSERT INTO improvement_approvals(id,proposal_id,phase,object_hash,decision,actor,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, approval.ID, proposalID, approval.Phase, hash, decision, actor, reason, approval.CreatedAt); err != nil {
		return Approval{}, err
	}
	if err := tx.Commit(); err != nil {
		return Approval{}, err
	}
	return approval, nil
}

func scanRollout(row interface{ Scan(...any) error }) (Rollout, error) {
	var rollout Rollout
	err := row.Scan(&rollout.ID, &rollout.ReleaseID, &rollout.TargetAgent, &rollout.PriorImageRef, &rollout.PriorImageDigest, &rollout.ImageRef, &rollout.ImageDigest, &rollout.Status, &rollout.CreatedAt, &rollout.CompletedAt, &rollout.RollbackOf)
	return rollout, err
}

func (s *Store) StageSingleRollout(ctx context.Context, releaseID, agentName, hash string) (Rollout, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Rollout{}, err
	}
	defer tx.Rollback()
	var release Release
	if err := tx.QueryRowContext(ctx, `SELECT id,proposal_id,repository_id,git_commit,source_name,source_digest,lock_digest,prompt_template_digest,image_ref,image_digest,builder_version,release_hash,status,created_at FROM image_releases WHERE id=?`, releaseID).Scan(&release.ID, &release.ProposalID, &release.RepositoryID, &release.GitCommit, &release.SourceName, &release.SourceDigest, &release.LockDigest, &release.PromptTemplateDigest, &release.ImageRef, &release.ImageDigest, &release.BuilderVersion, &release.ReleaseHash, &release.Status, &release.CreatedAt); err != nil {
		return Rollout{}, err
	}
	if release.ReleaseHash != hash {
		return Rollout{}, ErrRevisionMismatch
	}
	if existing, err := scanRollout(tx.QueryRowContext(ctx, `SELECT id,release_id,target_agent,prior_image_ref,prior_image_digest,image_ref,image_digest,status,created_at,completed_at,rollback_of FROM image_rollouts WHERE release_id=? AND target_agent=? AND rollback_of=''`, releaseID, agentName)); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Rollout{}, err
	}
	if release.Status != StatusImageBuilt {
		return Rollout{}, ErrInvalidTransition
	}
	var approvals int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM improvement_approvals WHERE proposal_id=? AND phase='rollout' AND object_hash=? AND decision='approve'`, release.ProposalID, hash).Scan(&approvals); err != nil || approvals == 0 {
		if err != nil {
			return Rollout{}, err
		}
		return Rollout{}, ErrInvalidTransition
	}
	var currentRef, currentDigest, pendingRef, pendingDigest string
	if err := tx.QueryRowContext(ctx, `SELECT image_ref,image_digest,pending_image_ref,pending_image_digest FROM agents WHERE name=?`, agentName).Scan(&currentRef, &currentDigest, &pendingRef, &pendingDigest); errors.Is(err, sql.ErrNoRows) {
		return Rollout{}, ErrNotFound
	} else if err != nil {
		return Rollout{}, err
	}
	if pendingRef != "" && (pendingRef != release.ImageRef || pendingDigest != release.ImageDigest) {
		return Rollout{}, ErrInvalidTransition
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rollout := Rollout{ID: uuid.NewString(), ReleaseID: releaseID, TargetAgent: agentName, PriorImageRef: currentRef, PriorImageDigest: currentDigest, ImageRef: release.ImageRef, ImageDigest: release.ImageDigest, Status: StatusRolloutPending, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO image_rollouts(id,release_id,target_agent,prior_image_ref,prior_image_digest,image_ref,image_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, rollout.ID, releaseID, agentName, currentRef, currentDigest, release.ImageRef, release.ImageDigest, rollout.Status, now); err != nil {
		return Rollout{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET pending_image_ref=?,pending_image_digest=?,pending_image_error='' WHERE name=?`, release.ImageRef, release.ImageDigest, agentName); err != nil {
		return Rollout{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE image_releases SET status=? WHERE id=?`, StatusRolloutPending, releaseID); err != nil {
		return Rollout{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE improvement_proposals SET status=?,updated_at=? WHERE id=?`, StatusRolloutPending, now, release.ProposalID); err != nil {
		return Rollout{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rollout{}, err
	}
	return rollout, nil
}

func (s *Store) StageRollback(ctx context.Context, rolloutID string) (Rollout, error) {
	columns := `id,release_id,target_agent,prior_image_ref,prior_image_digest,image_ref,image_digest,status,created_at,completed_at,rollback_of`
	if existing, err := scanRollout(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM image_rollouts WHERE rollback_of=?`, rolloutID)); err == nil {
		return existing, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Rollout{}, err
	}
	defer tx.Rollback()
	original, err := scanRollout(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM image_rollouts WHERE id=?`, rolloutID))
	if errors.Is(err, sql.ErrNoRows) {
		return Rollout{}, ErrNotFound
	}
	if err != nil {
		return Rollout{}, err
	}
	if original.Status != StatusRolledOut || original.RollbackOf != "" {
		return Rollout{}, ErrInvalidTransition
	}
	var currentRef, currentDigest string
	if err := tx.QueryRowContext(ctx, `SELECT image_ref,image_digest FROM agents WHERE name=?`, original.TargetAgent).Scan(&currentRef, &currentDigest); err != nil {
		return Rollout{}, err
	}
	if currentRef != original.ImageRef || currentDigest != original.ImageDigest {
		return Rollout{}, ErrInvalidTransition
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rollback := Rollout{ID: uuid.NewString(), ReleaseID: original.ReleaseID, TargetAgent: original.TargetAgent, PriorImageRef: currentRef, PriorImageDigest: currentDigest, ImageRef: original.PriorImageRef, ImageDigest: original.PriorImageDigest, Status: StatusRolloutPending, CreatedAt: now, RollbackOf: original.ID}
	if _, err := tx.ExecContext(ctx, `INSERT INTO image_rollouts(id,release_id,target_agent,prior_image_ref,prior_image_digest,image_ref,image_digest,status,created_at,rollback_of) VALUES(?,?,?,?,?,?,?,?,?,?)`, rollback.ID, rollback.ReleaseID, rollback.TargetAgent, rollback.PriorImageRef, rollback.PriorImageDigest, rollback.ImageRef, rollback.ImageDigest, rollback.Status, rollback.CreatedAt, rollback.RollbackOf); err != nil {
		return Rollout{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET pending_image_ref=?,pending_image_digest=?,pending_image_error='' WHERE name=?`, rollback.ImageRef, rollback.ImageDigest, rollback.TargetAgent); err != nil {
		return Rollout{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rollout{}, err
	}
	return rollback, nil
}
