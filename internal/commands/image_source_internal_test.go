package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/image"
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

func TestImageSourceBuildWaitsForPublicationGate(t *testing.T) {
	c := localCtx(t)
	if _, err := imageSourceStore(c).Create(imagesource.CreateRequest{Name: "reviewer", Prompt: "Review the task."}); err != nil {
		t.Fatal(err)
	}
	entered, release, locked := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		locked <- image.WithPublicationGate(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	done := make(chan error, 1)
	go func() {
		_, err := imageSourceBuild().Handler(c, registry.Params{"name": "reviewer", "tag": "v1"})
		done <- err
	}()
	select {
	case err := <-done:
		close(release)
		<-locked
		t.Fatalf("editable-source publisher ignored publication gate: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-locked; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestImageSourceBuildRejectsDisabledExternalPlugin(t *testing.T) {
	c := localCtx(t)
	installDisabledPlugin(t, c, "widget")
	if _, err := imageSourceStore(c).Create(imagesource.CreateRequest{Name: "reviewer", Prompt: "Review the task."}); err != nil {
		t.Fatal(err)
	}
	yaml := "schema_version: 2\nplugins:\n  - name: widget\n"
	if err := os.WriteFile(filepath.Join(c.BaseDir, "image-sources", "reviewer", "Tariboyfile.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := imageSourceBuild().Handler(c, registry.Params{"name": "reviewer", "tag": "v1"}); err == nil || !strings.Contains(err.Error(), `unknown plugin "widget"`) {
		t.Fatalf("image source build error = %v", err)
	}
}
