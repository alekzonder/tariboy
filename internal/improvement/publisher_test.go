package improvement

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
)

func TestPublisherBuildsInertImmutableRelease(t *testing.T) {
	db, proposals := testStore(t)
	source := t.TempDir()
	prompt := "locked prompt\n"
	if err := os.MkdirAll(filepath.Join(source, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "vendor", "messages.md"), []byte(prompt), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nskills: []\nprompts:\n  - file: ./vendor/messages.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := "schema_version: 1\ntariboy_version: 0.18.0\nprompt_dependencies:\n  messages:\n    repository: tariboy-core\n    upstream_commit: 82fd301\n    upstream_path: store/skills/messages/prompt.md\n    upstream_sha256: " + digestText(prompt) + "\n    local_path: vendor/messages.md\n    mode: upstream\n"
	if err := os.WriteFile(filepath.Join(source, "tariboy.lock.yaml"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	draft := validDraft()
	draft.Target.Repository = "images"
	draft.Target.BaseCommit = "91ab820"
	proposal, err := proposals.CreateProposal(context.Background(), CreateProposalRequest{JudgeRunID: "run", CreatorAgent: "judge", CreatorIteration: "it", Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proposals.DecidePlan(context.Background(), ApprovalRequest{ProposalID: proposal.ID, ObjectHash: proposal.RevisionHash, Decision: DecisionApprove, Actor: "operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`UPDATE improvement_proposals SET status='merged',merged_commit='a91ab820' WHERE id=?`, proposal.ID); err != nil {
		t.Fatal(err)
	}
	images := &image.Store{Dir: filepath.Join(t.TempDir(), "images")}
	publisher := NewPublisher(PublisherConfig{Store: proposals, Images: images, Snapshots: &imagesnapshot.Store{DB: db.DB, Root: filepath.Join(t.TempDir(), "snapshots"), Clock: time.Now}, Clock: time.Now})
	release, err := publisher.Build(context.Background(), BuildRequest{ProposalID: proposal.ID, RepositoryID: "images", GitCommit: "a91ab820", SourceDir: source, SourceName: "reviewer", ImageRef: "reviewer:2026.08.28-a91ab820"})
	if err != nil {
		t.Fatal(err)
	}
	if release.Status != StatusImageBuilt || release.ReleaseHash == "" || release.ImageDigest == "" || release.LockDigest == "" || !images.Exists(image.Ref{Name: "reviewer", Tag: "2026.08.28-a91ab820"}) {
		t.Fatalf("release = %+v", release)
	}
	stored, err := proposals.GetProposal(context.Background(), proposal.ID)
	if err != nil || stored.Status != StatusImageBuilt {
		t.Fatalf("proposal = %+v, %v", stored, err)
	}
}

func TestPublisherRejectsLatestAndUnapprovedProposal(t *testing.T) {
	_, proposals := testStore(t)
	publisher := NewPublisher(PublisherConfig{Store: proposals, Images: &image.Store{Dir: t.TempDir()}})
	if _, err := publisher.Build(context.Background(), BuildRequest{ImageRef: "reviewer:latest"}); err == nil {
		t.Fatal("latest accepted")
	}
}

func TestPublisherRejectsMutableStoreInput(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nskills: []\nprompts:\n  - file: $CURRENT_VERSION_STORE/skills/messages/prompt.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := imagefile.ParseV2(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateVendoredSource(parsed); err == nil {
		t.Fatal("mutable Store input accepted")
	}
}
