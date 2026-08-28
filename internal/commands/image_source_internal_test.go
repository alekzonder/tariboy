package commands

import (
	"context"
	"testing"

	"github.com/alekzonder/tariboy/internal/imagesource"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestImageSourceBuildCopiesGitProvenanceIntoSnapshot(t *testing.T) {
	c := localCtx(t)
	want := imagesource.Provenance{
		RepositoryID: "production-agent-images",
		GitCommit:    "91ab820",
		LockDigest:   "sha256:lock",
	}
	if _, err := imageSourceStore(c).Create(imagesource.CreateRequest{
		Name: "reviewer", Prompt: "Review the task.", Provenance: want,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := imageSourceBuild().Handler(c, registry.Params{"name": "reviewer", "tag": "v7"})
	if err != nil {
		t.Fatal(err)
	}
	digest := result.(map[string]any)["digest"].(string)
	snapshot, ok, err := imageSnapshotStore(c).LookupDigest(context.Background(), digest)
	if err != nil || !ok {
		t.Fatalf("LookupDigest = ok %v, err %v", ok, err)
	}
	if snapshot.RepositoryID != want.RepositoryID || snapshot.GitCommit != want.GitCommit || snapshot.LockDigest != want.LockDigest {
		t.Fatalf("snapshot provenance = %+v, want %+v", snapshot, want)
	}
}
