package commands

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/imageprovenance"
	"github.com/alekzonder/tariboy/internal/registry"
	storedb "github.com/alekzonder/tariboy/internal/store"
)

func localCtx(t *testing.T) *registry.Ctx {
	t.Helper()
	base := t.TempDir()
	s, err := storedb.Open(filepath.Join(base, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &registry.Ctx{
		Store:   s,
		BaseDir: base,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func handler(t *testing.T, path string) registry.HandlerFunc {
	t.Helper()
	cmd, ok := BuildRegistry().Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	if cmd.HTTP != nil {
		t.Fatalf("%s should be CLI-local (HTTP == nil)", path)
	}
	return cmd.Handler
}

// cmdHandler returns any registered command's handler without asserting on its
// HTTP disposition.
func cmdHandler(t *testing.T, path string) registry.HandlerFunc {
	t.Helper()
	cmd, ok := BuildRegistry().Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	return cmd.Handler
}

func writeExample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	yaml := "schema_version: 1\nplugins: [ {name: context}, {name: status} ]\nprompts: [task.md]\n"
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("BE A TEST AGENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestImageCommandLifecycle(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)

	res, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": "demo:latest", "path": src})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	if m["name"] != "demo" || m["tag"] != "latest" || m["digest"] == "" {
		t.Fatalf("build result: %v", m)
	}

	ls, err := cmdHandler(t, "image.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if ls.(map[string]any)["count"].(int) != 1 {
		t.Fatalf("ls: %v", ls)
	}

	pr, err := cmdHandler(t, "image.prompt")(c, registry.Params{"ref": "demo:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if s := pr.(map[string]any)["prompt"].(string); !strings.Contains(s, "BE A TEST AGENT") ||
		strings.LastIndex(s, "i-am-done") < strings.Index(s, "BE A TEST AGENT") {
		t.Fatalf("prompt tail not last: %q", s)
	}

	if _, err := cmdHandler(t, "image.inspect")(c, registry.Params{"ref": "demo:latest"}); err != nil {
		t.Fatal(err)
	}

	if _, err := cmdHandler(t, "image.rm")(c, registry.Params{"ref": "demo:latest"}); err != nil {
		t.Fatal(err)
	}
	ls, _ = cmdHandler(t, "image.ls")(c, registry.Params{})
	if ls.(map[string]any)["count"].(int) != 0 {
		t.Fatalf("image not removed: %v", ls)
	}

	// error surfaces as api.UserError
	if _, err := cmdHandler(t, "image.prompt")(c, registry.Params{"ref": "ghost:latest"}); err == nil {
		t.Fatal("prompt of absent image should error")
	}
}

func TestImageBuildV2RequiresNameAndDefaultsTag(t *testing.T) {
	c := localCtx(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nprompts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"path": src}); err == nil {
		t.Fatal("missing name accepted")
	}
	result, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "transparent", "path": src})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(map[string]any)
	if got["name"] != "transparent" || got["tag"] != "latest" {
		t.Fatalf("result = %#v", got)
	}
	manifest, err := imageStore(c).Inspect(image.Ref{Name: "transparent", Tag: "latest"})
	if err != nil || manifest.SchemaVersion != 2 {
		t.Fatalf("manifest = %#v, %v", manifest, err)
	}
}

func TestImageBuildRebuildsMutableTag(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	first, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "reviewer", "tag": "latest", "path": src})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("UPDATED TEST AGENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "reviewer", "tag": "latest", "path": src})
	if err != nil {
		t.Fatal(err)
	}
	if first.(map[string]any)["digest"] == second.(map[string]any)["digest"] {
		t.Fatal("rebuild kept the old digest")
	}
	if !imageStore(c).IsMutable(image.Ref{Name: "reviewer", Tag: "latest"}) {
		t.Fatal("rebuilt ref is not mutable")
	}
}

func TestImageBuildMultipleTags(t *testing.T) {
	c := localCtx(t)
	result, err := cmdHandler(t, "image.build")(c, registry.Params{
		"name": "reviewer", "tag": []string{"latest", "v2"}, "path": writeExample(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	images, ok := result.(map[string]any)["images"].([]map[string]any)
	if !ok || len(images) != 2 {
		t.Fatalf("images = %#v", result)
	}
	for _, tag := range []string{"latest", "v2"} {
		if !imageStore(c).Exists(image.Ref{Name: "reviewer", Tag: tag}) {
			t.Fatalf("reviewer:%s was not published", tag)
		}
	}
}

func TestImageBuildRejectsDuplicateAndReservedTagsBeforePublishing(t *testing.T) {
	for _, test := range []struct {
		name string
		tags []string
	}{
		{name: "reviewer", tags: []string{"latest", "latest"}},
		{name: "basic", tags: []string{"latest", "v2"}},
	} {
		t.Run(test.name+"-"+strings.Join(test.tags, "-"), func(t *testing.T) {
			c := localCtx(t)
			_, err := cmdHandler(t, "image.build")(c, registry.Params{
				"name": test.name, "tag": test.tags, "path": writeExample(t),
			})
			if err == nil {
				t.Fatal("invalid tags were accepted")
			}
			if imageStore(c).Exists(image.Ref{Name: test.name, Tag: "latest"}) {
				t.Fatal("a tag was published before validation completed")
			}
		})
	}
}

func TestImageBuildRejectsReleaseRef(t *testing.T) {
	c := localCtx(t)
	if _, err := c.Store.DB.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = c.Store.DB.Exec(`PRAGMA foreign_keys = ON`) })
	if _, err := c.Store.DB.Exec(`INSERT INTO image_releases(id,proposal_id,repository_id,git_commit,source_name,source_digest,lock_digest,prompt_template_digest,image_ref,image_digest,builder_version,release_hash,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "release", "proposal", "repo", "abc1234", "reviewer", "source", "lock", "prompt", "reviewer:latest", "sha256:release", "test", "sha256:hash", "image_built", "2026-09-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	_, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "reviewer", "tag": "latest", "path": writeExample(t)})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "immutable_release" {
		t.Fatalf("error = %#v, want immutable_release", err)
	}
	if imageStore(c).Exists(image.Ref{Name: "reviewer", Tag: "latest"}) {
		t.Fatal("release ref was published")
	}
}

func TestImageBuildWaitsForPublicationGate(t *testing.T) {
	c := localCtx(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	locked := make(chan error, 1)
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
		_, err := cmdHandler(t, "image.build")(c, registry.Params{"name": "reviewer", "tag": "latest", "path": writeExample(t)})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("build ignored publication gate: %v", err)
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

func TestImageBuildRestoresExistingRefAndMetadataWhenProvenanceFails(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	ref := image.Ref{Name: "reviewer", Tag: "latest"}
	first, err := cmdHandler(t, "image.build")(c, registry.Params{"name": ref.Name, "tag": ref.Tag, "path": src})
	if err != nil {
		t.Fatal(err)
	}
	firstDigest := first.(map[string]any)["digest"].(string)
	previousSnapshot, ok, err := imageSnapshotStore(c).Lookup(context.Background(), ref.String())
	if err != nil || !ok {
		t.Fatalf("previous snapshot = %#v, %v, %v", previousSnapshot, ok, err)
	}
	previousProvenance, ok, err := (imageprovenance.Store{DB: c.Store.DB}).Get(ref.String())
	if err != nil || !ok {
		t.Fatalf("previous provenance = %#v, %v, %v", previousProvenance, ok, err)
	}
	if err := os.WriteFile(filepath.Join(src, "task.md"), []byte("UPDATED TEST AGENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Store.DB.Exec(`CREATE TRIGGER reject_image_provenance BEFORE INSERT ON image_provenance BEGIN SELECT RAISE(FAIL, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = cmdHandler(t, "image.build")(c, registry.Params{"name": ref.Name, "tag": ref.Tag, "path": src})
	if err == nil {
		t.Fatal("rebuild succeeded despite provenance failure")
	}
	current, err := imageStore(c).Inspect(ref)
	if err != nil || current.Digest != firstDigest {
		t.Fatalf("current = %#v, %v; want %s", current, err, firstDigest)
	}
	snapshot, ok, err := imageSnapshotStore(c).Lookup(context.Background(), ref.String())
	if err != nil || !ok || snapshot != previousSnapshot {
		t.Fatalf("snapshot = %#v, %v, %v; want %#v", snapshot, ok, err, previousSnapshot)
	}
	provenance, ok, err := (imageprovenance.Store{DB: c.Store.DB}).Get(ref.String())
	if err != nil || !ok || provenance.Ref != previousProvenance.Ref || provenance.Digest != previousProvenance.Digest || provenance.SourceCWD != previousProvenance.SourceCWD || provenance.BuiltAt != previousProvenance.BuiltAt {
		t.Fatalf("provenance = %#v, %v, %v; want %#v", provenance, ok, err, previousProvenance)
	}
}

func TestImageBuildRestoresImmutableRefWhenProvenanceFails(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	ref := image.Ref{Name: "imported", Tag: "v1"}
	file, err := imagefile.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	first, err := image.Build(file, ref, imageStore(c), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if imageStore(c).IsMutable(ref) {
		t.Fatal("seed ref is mutable")
	}
	if _, err := imageSnapshotStore(c).Capture(context.Background(), ref.String(), first.Digest, ref.Name, src); err != nil {
		t.Fatal(err)
	}
	if err := (imageprovenance.Store{DB: c.Store.DB}).Upsert(imageprovenance.Record{Ref: ref.String(), Digest: first.Digest, SourceCWD: src, BuiltAt: first.BuiltAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Store.DB.Exec(`CREATE TRIGGER reject_imported_provenance BEFORE INSERT ON image_provenance BEGIN SELECT RAISE(FAIL, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = cmdHandler(t, "image.build")(c, registry.Params{"name": ref.Name, "tag": ref.Tag, "path": src})
	if err == nil {
		t.Fatal("rebuild succeeded despite provenance failure")
	}
	current, err := imageStore(c).Inspect(ref)
	if err != nil || current.Digest != first.Digest || imageStore(c).IsMutable(ref) {
		t.Fatalf("restored ref = %#v, %v, mutable=%v", current, err, imageStore(c).IsMutable(ref))
	}
}

func TestImageBuildRecordsExplicitGitProvenance(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	result, err := cmdHandler(t, "image.build")(c, registry.Params{
		"name": "developer", "tag": "v1", "path": src,
		"repository-id": "tariboy", "git-commit": "97cf20ec0c8542f54b68904521be6a2ca85552a1",
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := result.(map[string]any)["digest"].(string)
	snapshot, ok, err := imageSnapshotStore(c).LookupDigest(context.Background(), digest)
	if err != nil || !ok {
		t.Fatalf("LookupDigest = ok %v, err %v", ok, err)
	}
	if snapshot.RepositoryID != "tariboy" || snapshot.GitCommit != "97cf20ec0c8542f54b68904521be6a2ca85552a1" {
		t.Fatalf("snapshot provenance = %+v", snapshot)
	}
	shown, err := cmdHandler(t, "image.provenance")(c, registry.Params{"ref": "developer:v1"})
	if err != nil {
		t.Fatal(err)
	}
	provenance := shown.(map[string]any)
	if provenance["repository_id"] != "tariboy" || provenance["git_commit"] != "97cf20ec0c8542f54b68904521be6a2ca85552a1" || provenance["source_digest"] == "" {
		t.Fatalf("shown provenance = %#v", provenance)
	}
}

func TestImageBuildRejectsPartialGitProvenance(t *testing.T) {
	c := localCtx(t)
	_, err := cmdHandler(t, "image.build")(c, registry.Params{
		"name": "developer", "tag": "v1", "path": writeExample(t), "repository-id": "tariboy",
	})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "bad_provenance" {
		t.Fatalf("error = %#v, want bad_provenance", err)
	}
}

func TestImageBuildRollsBackArtifactWhenProvenanceCannotCommit(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	if err := c.Store.Close(); err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "orphan", Tag: "latest"}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"name": ref.Name, "tag": ref.Tag, "path": src}); err == nil {
		t.Fatal("build succeeded without durable provenance")
	}
	if imageStore(c).Exists(ref) {
		t.Fatal("published image survived failed provenance commit")
	}
}

func TestRegistryDoesNotExposeManagedImageSourceSnapshots(t *testing.T) {
	registry := BuildRegistry()
	for _, command := range []string{
		"image.source.ls", "image.source.create", "image.source.inspect", "image.source.rm",
		"image.source.files", "image.source.file.get", "image.source.file.put",
		"image.source.validate", "image.source.build",
	} {
		if _, ok := registry.Get(command); ok {
			t.Fatalf("obsolete editable source command %s remains public", command)
		}
	}
}

func TestImageListReportsCurrentAndPendingAgents(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	result, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": "shared:latest", "path": src})
	if err != nil {
		t.Fatal(err)
	}
	digest := result.(map[string]any)["digest"].(string)
	as := agent.NewStore(c.Store)
	if err := as.Create(agent.Agent{Name: "active-worker", ImageRef: "shared:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(agent.Agent{Name: "pending-worker", ImageRef: "other:latest"}); err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage("pending-worker", "shared:latest", digest); err != nil {
		t.Fatal(err)
	}

	listed, err := cmdHandler(t, "image.ls")(c, registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	rows := listed.(map[string]any)["images"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("images = %#v", rows)
	}
	if got := rows[0]["current_agents"]; !equalStrings(got, []string{"active-worker"}) {
		t.Fatalf("current_agents = %#v", got)
	}
	if got := rows[0]["pending_agents"]; !equalStrings(got, []string{"pending-worker"}) {
		t.Fatalf("pending_agents = %#v", got)
	}
}

func equalStrings(value any, want []string) bool {
	got, ok := value.([]string)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestImageValidateV2ResolvesPromptFilesWithoutWritingArtifact(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nprompts:\n  - file: ./missing.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := cmdHandler(t, "image.validate")(c, registry.Params{"path": source, "name": "missing-layer"})
	if err != nil {
		t.Fatal(err)
	}
	result := got.(map[string]any)
	if result["valid"] != false {
		t.Fatalf("validate = %#v, want invalid", result)
	}
	images, err := imageStore(c).List()
	if err != nil || len(images) != 0 {
		t.Fatalf("validation wrote images: %#v, %v", images, err)
	}
}

func TestImageValidateChecksTargetRefAndImmutableCollision(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte("schema_version: 2\nplugins: []\nprompts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid, err := cmdHandler(t, "image.validate")(c, registry.Params{"path": source, "name": "bad/name", "tag": "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if invalid.(map[string]any)["valid"] != false {
		t.Fatalf("invalid target accepted: %#v", invalid)
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"path": source, "name": "taken", "tag": "v1"}); err != nil {
		t.Fatal(err)
	}
	collision, err := cmdHandler(t, "image.validate")(c, registry.Params{"path": source, "name": "taken", "tag": "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if collision.(map[string]any)["valid"] != false {
		t.Fatalf("immutable collision accepted: %#v", collision)
	}
}

func TestImageValidateV2ReturnsOrderedResolvedTemplate(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "role.md"), []byte("review carefully"), 0o600); err != nil {
		t.Fatal(err)
	}
	yaml := "schema_version: 2\nplugins: []\nprompts:\n  - runtime: identity\n  - file: ./role.md\n  - runtime: context\n"
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := cmdHandler(t, "image.validate")(c, registry.Params{"path": source, "name": "ordered"})
	if err != nil {
		t.Fatal(err)
	}
	result := got.(map[string]any)
	if result["valid"] != true {
		t.Fatalf("validate = %#v", result)
	}
	template, ok := result["template"].(image.PromptTemplate)
	if !ok || len(template.Entries) != 3 {
		t.Fatalf("template = %#v", result["template"])
	}
	if template.Entries[0].Runtime != "identity" || template.Entries[1].Source != "./role.md" || template.Entries[1].Category != "source" || template.Entries[1].Size != int64(len("review carefully")) || template.Entries[1].SHA256 == "" || template.Entries[2].Runtime != "context" {
		t.Fatalf("entries = %#v", template.Entries)
	}
	if plugins, ok := result["plugins"].([]string); !ok || len(plugins) != 0 {
		t.Fatalf("plugins = %#v", result["plugins"])
	}
	images, err := imageStore(c).List()
	if err != nil || len(images) != 0 {
		t.Fatalf("validation wrote images: %#v, %v", images, err)
	}
}

func TestImageValidateV2ReturnsSkillMetadataAndDuplicateWarnings(t *testing.T) {
	c := localCtx(t)
	source := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeSkill := func(root string) {
		t.Helper()
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: code-review\ndescription: Review changes safely.\n---\n# Review\n"
		if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(filepath.Join(source, "skills", "code-review"))
	writeSkill(filepath.Join(source, ".agents", "skills", "code-review"))
	yaml := "schema_version: 2\nplugins: []\nskills:\n  - dir: ./skills/code-review\nprompts: []\n"
	if err := os.WriteFile(filepath.Join(source, "Tariboyfile.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := cmdHandler(t, "image.validate")(c, registry.Params{"path": source, "name": "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	result := got.(map[string]any)
	skills, ok := result["skills"].([]image.ManifestSkill)
	if !ok || len(skills) != 1 {
		t.Fatalf("skills = %#v", result["skills"])
	}
	skill := skills[0]
	if skill.Name != "code-review" || skill.Description != "Review changes safely." || skill.Source != "./skills/code-review" || skill.Category != "source" || skill.ArchiveRoot != "skills/code-review" || skill.FileCount != 1 || skill.Size <= 0 || len(skill.TreeSHA256) != 64 {
		t.Fatalf("skill = %#v", skill)
	}
	warnings := result["warnings"].([]map[string]string)
	found := false
	for _, warning := range warnings {
		if warning["path"] == "skills[0]" && warning["message"] == "skill code-review is also visible in cwd scope; native harness precedence applies" {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestImageCommandsRejectReservedBasicRef(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	_, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": "basic:latest", "path": src})
	var userErr api.UserError
	if !errors.As(err, &userErr) || userErr.Code != "reserved_image" {
		t.Fatalf("reserved build error = %#v, want reserved_image", err)
	}

	imgFile, err := imagefile.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	ref := image.Ref{Name: "basic", Tag: "latest"}
	if _, err := image.Build(imgFile, ref, imageStore(c), time.Now); err != nil {
		t.Fatal(err)
	}
	_, err = cmdHandler(t, "image.rm")(c, registry.Params{"ref": ref.String()})
	if !errors.As(err, &userErr) || userErr.Code != "reserved_image" {
		t.Fatalf("reserved remove error = %#v, want reserved_image", err)
	}
}

func TestImageRemoveRejectsActiveAndPendingRefsAndClearsProvenance(t *testing.T) {
	c := localCtx(t)
	src := writeExample(t)
	for _, ref := range []string{"active:latest", "pending:latest", "unused:latest"} {
		if _, err := cmdHandler(t, "image.build")(c, registry.Params{"tag": ref, "path": src}); err != nil {
			t.Fatal(err)
		}
	}
	as := agent.NewStore(c.Store)
	if err := as.Create(agent.Agent{Name: "worker", ImageRef: "active:latest"}); err != nil {
		t.Fatal(err)
	}
	pendingManifest, err := imageStore(c).Inspect(image.Ref{Name: "pending", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if err := as.SetPendingImage("worker", "pending:latest", pendingManifest.Digest); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"active:latest", "pending:latest"} {
		_, err := cmdHandler(t, "image.rm")(c, registry.Params{"ref": ref})
		var userErr api.UserError
		if !errors.As(err, &userErr) || userErr.Code != "image_in_use" {
			t.Fatalf("remove %s error = %#v", ref, err)
		}
	}
	provenance := imageprovenance.Store{DB: c.Store.DB}
	if err := provenance.Upsert(imageprovenance.Record{Ref: "unused:latest", Digest: "digest", SourceCWD: t.TempDir(), BuiltAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdHandler(t, "image.rm")(c, registry.Params{"ref": "unused:latest"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := provenance.Get("unused:latest"); err != nil || ok {
		t.Fatalf("provenance survived removal: ok=%v err=%v", ok, err)
	}
}
