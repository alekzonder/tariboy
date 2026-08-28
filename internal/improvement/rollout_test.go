package improvement

import (
	"context"
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
)

func TestApprovedSingleRolloutStagesPendingImage(t *testing.T) {
	db, proposals := testStore(t)
	agents := agent.NewStore(db)
	if err := agents.Create(agent.Agent{Name: "worker", ImageRef: "worker:v1", ImageDigest: "sha256:old"}); err != nil {
		t.Fatal(err)
	}
	draft := validDraft()
	proposal, err := proposals.CreateProposal(context.Background(), CreateProposalRequest{JudgeRunID: "run", CreatorAgent: "judge", CreatorIteration: "it", Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`UPDATE improvement_proposals SET status='image_built' WHERE id=?`, proposal.ID); err != nil {
		t.Fatal(err)
	}
	release := Release{ID: "release-1", ProposalID: proposal.ID, RepositoryID: "images", GitCommit: "abc1234", SourceName: "worker", SourceDigest: "sha256:source", LockDigest: "sha256:lock", PromptTemplateDigest: "sha256:prompt", ImageRef: "worker:v2", ImageDigest: "sha256:new", BuilderVersion: "test", ReleaseHash: "sha256:release", Status: StatusImageBuilt, CreatedAt: "2026-08-28T12:00:00Z"}
	if _, err := db.DB.Exec(`INSERT INTO image_releases(id,proposal_id,repository_id,git_commit,source_name,source_digest,lock_digest,prompt_template_digest,image_ref,image_digest,builder_version,release_hash,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, release.ID, release.ProposalID, release.RepositoryID, release.GitCommit, release.SourceName, release.SourceDigest, release.LockDigest, release.PromptTemplateDigest, release.ImageRef, release.ImageDigest, release.BuilderVersion, release.ReleaseHash, release.Status, release.CreatedAt); err != nil {
		t.Fatal(err)
	}
	service := NewService(proposals, nil)
	if _, err := service.StageSingleRollout(context.Background(), release.ID, "worker", release.ReleaseHash); err == nil {
		t.Fatal("unapproved rollout staged")
	}
	if _, err := service.DecideRollout(context.Background(), release.ID, release.ReleaseHash, "operator", DecisionApprove, "safe"); err != nil {
		t.Fatal(err)
	}
	rollout, err := service.StageSingleRollout(context.Background(), release.ID, "worker", release.ReleaseHash)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := agents.PendingImage("worker")
	if err != nil || pending.Ref != release.ImageRef || pending.Digest != release.ImageDigest {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if rollout.PriorImageRef != "worker:v1" || rollout.Status != StatusRolloutPending {
		t.Fatalf("rollout = %+v", rollout)
	}
	second, err := service.StageSingleRollout(context.Background(), release.ID, "worker", release.ReleaseHash)
	if err != nil || second.ID != rollout.ID {
		t.Fatalf("idempotent rollout = %+v, %v", second, err)
	}
	if err := agents.PromotePendingImage("worker"); err != nil {
		t.Fatal(err)
	}
	var rolloutStatus, releaseStatus, proposalStatus string
	if err := db.DB.QueryRow(`SELECT status FROM image_rollouts WHERE id=?`, rollout.ID).Scan(&rolloutStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRow(`SELECT status FROM image_releases WHERE id=?`, release.ID).Scan(&releaseStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRow(`SELECT status FROM improvement_proposals WHERE id=?`, proposal.ID).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if rolloutStatus != "rolled_out" || releaseStatus != "rolled_out" || proposalStatus != "rolled_out" {
		t.Fatalf("activation statuses = %s, %s, %s", rolloutStatus, releaseStatus, proposalStatus)
	}
	rollback, err := service.StageRollback(context.Background(), rollout.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = agents.PendingImage("worker")
	if err != nil || rollback.RollbackOf != rollout.ID || pending.Ref != "worker:v1" || pending.Digest != "sha256:old" {
		t.Fatalf("rollback = %+v pending = %+v err = %v", rollback, pending, err)
	}
}
