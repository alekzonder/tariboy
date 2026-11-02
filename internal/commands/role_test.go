package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/rolefile"
)

const commandRoleMarker = "COMMAND-ROLE-MARKER-41c2"

func writeRole(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rolefile.FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasRoleField(t *testing.T, value any) bool {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	_, ok := fields["role"]
	return ok
}

func TestImageInspectReturnsRole(t *testing.T) {
	c := localCtx(t)
	withRole := writeExample(t)
	writeRole(t, withRole, "# Reviewer\n"+commandRoleMarker+"\n")
	blank := writeExample(t)
	writeRole(t, blank, " \n\n")
	for name, dir := range map[string]string{"withrole": withRole, "plain": writeExample(t), "blank": blank} {
		if _, err := callCommand(t, c, "image.build", registry.Params{"name": name, "path": dir, "wait": true}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	result, err := callCommand(t, c, "image.inspect", registry.Params{"ref": "withrole:latest"})
	if err != nil {
		t.Fatal(err)
	}
	got := result.(ImageManifest)
	want := ManifestRole{Text: "# Reviewer\n" + commandRoleMarker + "\n"}
	if got.Role == nil || *got.Role != want {
		t.Fatalf("role = %+v", got.Role)
	}
	for _, ref := range []string{"plain:latest", "blank:latest"} {
		result, err := callCommand(t, c, "image.inspect", registry.Params{"ref": ref})
		if err != nil {
			t.Fatal(err)
		}
		if hasRoleField(t, result) {
			t.Fatalf("%s: inspect has a role field", ref)
		}
	}
	prompt, err := callCommand(t, c, "image.prompt", registry.Params{"ref": "withrole:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt.(ImagePromptResult).Prompt, commandRoleMarker) {
		t.Fatal("image.prompt carries ROLE.md")
	}
}

func TestImageBuildRejectsRoleOverLimit(t *testing.T) {
	c := localCtx(t)
	dir := writeExample(t)
	writeRole(t, dir, strings.Repeat("я", rolefile.LimitChars+1))
	_, err := callCommand(t, c, "image.build", registry.Params{"name": "big", "path": dir, "wait": true})
	if err == nil || !strings.Contains(err.Error(), "501 characters") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v", err)
	}
	if _, err := callCommand(t, c, "image.inspect", registry.Params{"ref": "big:latest"}); err == nil {
		t.Fatal("the failed build left an image")
	}
}

func TestImageBuildRejectsInvalidUTF8Role(t *testing.T) {
	c := localCtx(t)
	dir := writeExample(t)
	writeRole(t, dir, "bad \xff")
	_, err := callCommand(t, c, "image.build", registry.Params{"name": "bad", "path": dir, "wait": true})
	if err == nil || !strings.Contains(err.Error(), "ROLE.md is not valid UTF-8") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkflowInspectReturnsRole(t *testing.T) {
	c := workflowCtx(t)
	dir := writeWorkflowSource(t, t.TempDir(), "1.0.0")
	writeRole(t, dir, "# Flow\n")
	if _, err := callCommand(t, c, "workflow.build", registry.Params{"path": dir, "wait": true}); err != nil {
		t.Fatal(err)
	}
	result, err := callCommand(t, c, "workflow.inspect", registry.Params{"name": "demo", "tag": "latest"})
	if err != nil {
		t.Fatal(err)
	}
	want := ManifestRole{Text: "# Flow\n"}
	if got := result.(WorkflowImageManifest).Role; got == nil || *got != want {
		t.Fatalf("role = %+v", got)
	}

	plain := workflowCtx(t)
	if _, err := callCommand(t, plain, "workflow.build", registry.Params{"path": writeWorkflowSource(t, t.TempDir(), "1.0.0"), "wait": true}); err != nil {
		t.Fatal(err)
	}
	result, err = callCommand(t, plain, "workflow.inspect", registry.Params{"name": "demo", "tag": "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if hasRoleField(t, result) {
		t.Fatal("workflow without ROLE.md has a role field")
	}
}

func TestWorkflowBuildRejectsRoleOverLimit(t *testing.T) {
	c := workflowCtx(t)
	dir := writeWorkflowSource(t, t.TempDir(), "1.0.0")
	writeRole(t, dir, strings.Repeat("я", rolefile.LimitChars+1))
	_, err := callCommand(t, c, "workflow.build", registry.Params{"path": dir, "wait": true})
	if err == nil || !strings.Contains(err.Error(), "501 characters") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreBuildsCarryRole(t *testing.T) {
	c := workflowCtx(t)
	source := t.TempDir()
	imageDir := filepath.Join(source, "images", "reviewer")
	writeImageV2(t, imageDir, "1.2.3")
	writeRole(t, imageDir, "# Store image role\n")
	workflowDir := writeWorkflowSource(t, filepath.Join(source, "workflows", "demo"), "1.0.0")
	writeRole(t, workflowDir, "# Store workflow role\n")
	if _, err := cmdHandler(t, "store.add")(c, registry.Params{"name": "team", "source": source}); err != nil {
		t.Fatal(err)
	}
	if _, err := cmdHandler(t, "image.build")(c, registry.Params{"source": "team/reviewer"}); err != nil {
		t.Fatal(err)
	}
	image, err := callCommand(t, c, "image.inspect", registry.Params{"ref": "reviewer:1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if role := image.(ImageManifest).Role; role == nil || role.Text != "# Store image role\n" {
		t.Fatalf("image role = %+v", role)
	}
	if _, err := callCommand(t, c, "workflow.build", registry.Params{"source": "team/demo", "wait": true}); err != nil {
		t.Fatal(err)
	}
	workflow, err := callCommand(t, c, "workflow.inspect", registry.Params{"name": "demo", "tag": "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if role := workflow.(WorkflowImageManifest).Role; role == nil || role.Text != "# Store workflow role\n" {
		t.Fatalf("workflow role = %+v", role)
	}
}
