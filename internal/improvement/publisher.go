package improvement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imagesnapshot"
	"github.com/alekzonder/tariboy/internal/imagesource"
	"github.com/alekzonder/tariboy/internal/plugincaps"
	"github.com/google/uuid"
)

type PublisherConfig struct {
	Store          *Store
	Images         *image.Store
	Snapshots      *imagesnapshot.Store
	Roots          imagefile.ResolveRoots
	Plugins        plugincaps.ExternalResolver
	Clock          func() time.Time
	BuilderVersion string
}

type Publisher struct{ config PublisherConfig }

func NewPublisher(config PublisherConfig) *Publisher {
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.BuilderVersion == "" {
		config.BuilderVersion = "tariboy"
	}
	return &Publisher{config: config}
}

func (p *Publisher) Build(ctx context.Context, request BuildRequest) (Release, error) {
	var release Release
	err := image.WithPublicationGate(func() error {
		var err error
		release, err = p.build(ctx, request)
		return err
	})
	return release, err
}

func (p *Publisher) build(ctx context.Context, request BuildRequest) (Release, error) {
	ref, err := image.ParseRef(request.ImageRef)
	if err != nil || ref.Tag == "latest" || p.config.Store == nil || p.config.Images == nil || p.config.Snapshots == nil {
		return Release{}, fmt.Errorf("invalid immutable release request")
	}
	if p.config.Images.Exists(ref) {
		return Release{}, fmt.Errorf("immutable image %s already exists", ref.String())
	}
	proposal, err := p.config.Store.GetProposal(ctx, request.ProposalID)
	if err != nil {
		return Release{}, err
	}
	if proposal.Status != StatusMerged || proposal.Draft.Target.Repository != request.RepositoryID || proposal.MergedCommit != request.GitCommit {
		return Release{}, ErrInvalidTransition
	}
	lock, err := LoadLock(request.SourceDir)
	if err != nil {
		return Release{}, err
	}
	if err := ValidateLock(request.SourceDir, lock); err != nil {
		return Release{}, err
	}
	lockRaw, err := os.ReadFile(request.SourceDir + "/tariboy.lock.yaml")
	if err != nil {
		return Release{}, err
	}
	lockSum := sha256.Sum256(lockRaw)
	lockDigest := "sha256:" + hex.EncodeToString(lockSum[:])
	source, err := imagefile.ParseV2(request.SourceDir)
	if err != nil {
		return Release{}, err
	}
	if err := validateVendoredSource(source); err != nil {
		return Release{}, err
	}
	manifest, err := image.BuildV2(source, p.config.Roots, ref, p.config.Images, p.config.Clock, p.config.Plugins)
	if err != nil {
		return Release{}, err
	}
	cleanup := func() {
		_ = p.config.Images.Remove(ref)
		_, _ = p.config.Store.db.ExecContext(ctx, `DELETE FROM image_source_snapshots WHERE image_ref=?`, ref.String())
	}
	snapshot, err := p.config.Snapshots.CaptureWithProvenance(ctx, ref.String(), manifest.Digest, request.SourceName, request.SourceDir, imagesource.Provenance{RepositoryID: request.RepositoryID, GitCommit: request.GitCommit, LockDigest: lockDigest})
	if err != nil {
		cleanup()
		return Release{}, err
	}
	release := Release{ID: uuid.NewString(), ProposalID: proposal.ID, RepositoryID: request.RepositoryID, GitCommit: request.GitCommit, SourceName: request.SourceName, SourceDigest: snapshot.SourceDigest, LockDigest: lockDigest, PromptTemplateDigest: manifest.PromptTemplateSHA256, ImageRef: ref.String(), ImageDigest: manifest.Digest, BuilderVersion: p.config.BuilderVersion, Status: StatusImageBuilt, CreatedAt: p.config.Clock().UTC().Format(time.RFC3339Nano)}
	release.ReleaseHash, err = CanonicalHash(release)
	if err != nil {
		cleanup()
		return Release{}, err
	}
	release, err = p.config.Store.RecordRelease(ctx, release, proposal.RevisionHash)
	if err != nil {
		cleanup()
	}
	return release, err
}

func validateVendoredSource(source *imagefile.V2) error {
	for _, prompt := range source.Prompts {
		if prompt.File != "" && !strings.HasPrefix(prompt.File, "./") {
			return fmt.Errorf("release prompt %q must be vendored in the image source", prompt.File)
		}
	}
	for _, skill := range source.Skills {
		if !strings.HasPrefix(skill.Dir, "./") {
			return fmt.Errorf("release skill %q must be vendored in the image source", skill.Dir)
		}
	}
	return nil
}
