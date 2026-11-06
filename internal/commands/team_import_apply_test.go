package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/teamportable"
)

const (
	importFreshGhost   = "version: 1\ngroups:\n  fresh: {lead: newbie}\nagents:\n  newbie:\n    image: ghost:1\n    group: fresh\n"
	importFreshWorker  = "version: 1\ngroups:\n  fresh: {lead: newbie}\nagents:\n  newbie:\n    image: worker:latest\n    group: fresh\n"
	importExistingCrew = "version: 1\ngroups:\n  crew: {}\nagents:\n  lead1:\n    image: worker:latest\n    group: crew\n"
)

// hookGroups runs a test hook inside the group creation of the team apply.
type hookGroups struct {
	registry.GroupControl
	create func(name, lead string) (map[string]any, error)
}

func (g hookGroups) Create(name, lead string) (map[string]any, error) { return g.create(name, lead) }

// failingRunControl fails every agent creation.
type failingRunControl struct {
	*fakeControl
	err error
}

func (f failingRunControl) Run(registry.RunSpec) (string, error) { return "", f.err }

func importService(c *registry.Ctx) teamportable.Service {
	return teamportable.Service{StagingRoot: filepath.Join(c.BaseDir, "team-imports")}
}

func stageTeamImport(t *testing.T, c *registry.Ctx, yaml string) string {
	t.Helper()
	value, err := h(t, "team.import.yaml")(c, registry.Params{"yaml": yaml})
	if err != nil {
		t.Fatal(err)
	}
	return importID(t, value)
}

func importOperation(t *testing.T, c *registry.Ctx, id string) teamportable.Operation {
	t.Helper()
	operation, err := importService(c).Operation(id)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func importLeasePath(c *registry.Ctx, id string) string {
	return filepath.Join(c.BaseDir, "team-imports", id, ".operation.lease")
}

func assertNoImportLease(t *testing.T, c *registry.Ctx, id string) {
	t.Helper()
	if _, err := os.Stat(importLeasePath(c, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease stat = %v, want no lease", err)
	}
}

// freezeImportDir makes the next SaveOperation of the import fail.
func freezeImportDir(t *testing.T, c *registry.Ctx, id string) {
	t.Helper()
	dir := filepath.Join(c.BaseDir, "team-imports", id)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// Catches a rejected YAML edit starting the import: nothing is saved and no
// lease is taken.
func TestTeamImportApplyRejectsBadEditedYAML(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id, "yaml": "version: 1\nagents:\n  x: {}\n"})
	if !isCode(err, "bad_compose") {
		t.Fatalf("err = %#v, want bad_compose", err)
	}
	if got := importOperation(t, c, id); got.Status != "pending" || got.ComposeYAML != "" {
		t.Fatalf("operation = %#v", got)
	}
	assertNoImportLease(t, c, id)
}

// Catches an edited YAML being ignored, or not persisted for a resume.
func TestTeamImportApplyPersistsEditedYAML(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshGhost)
	value, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id, "yaml": importFreshWorker})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(TeamImportApplyResult)
	if !result.Complete || result.ImportID != id || result.Operation.Status != "complete" {
		t.Fatalf("result = %#v", result)
	}
	got := importOperation(t, c, id)
	if got.Status != "complete" || !strings.Contains(got.ComposeYAML, "worker:latest") || got.Error != "" {
		t.Fatalf("operation = %#v", got)
	}
	assertNoImportLease(t, c, id)
}

// Catches a resume losing the choices the saved operation holds: its YAML,
// its resolved refs and its update_existing decision.
func TestTeamImportApplyResumesSavedChoices(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	if _, err := h(t, "group.create")(c, registry.Params{"name": "crew"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "group.assign")(c, registry.Params{"name": "crew", "agent": "lead1"}); err != nil {
		t.Fatal(err)
	}
	id := stageTeamImport(t, c, importFreshGhost)
	operation := importOperation(t, c, id)
	operation.ComposeYAML = strings.ReplaceAll(importExistingCrew, "worker:latest", "ghost:2")
	operation.ResolvedRefs = map[string]string{"ghost:2": "worker:latest"}
	operation.UpdateExisting = true
	if err := importService(c).SaveOperation(operation); err != nil {
		t.Fatal(err)
	}
	// A saved ref survives a request that names other refs, and a value that is
	// not a non-empty string resolves nothing.
	value, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id, "refs": map[string]any{"ghost:2": "", "other:1": float64(3), "added:1": "worker:latest"}})
	if err != nil {
		t.Fatal(err)
	}
	if result := value.(TeamImportApplyResult); !result.Complete {
		t.Fatalf("result = %#v", result)
	}
	got := importOperation(t, c, id)
	if !got.UpdateExisting || got.ResolvedRefs["ghost:2"] != "worker:latest" || got.ResolvedRefs["added:1"] != "worker:latest" || len(got.ResolvedRefs) != 2 {
		t.Fatalf("operation = %#v", got)
	}
	if !strings.Contains(got.ComposeYAML, "image: worker:latest") {
		t.Fatalf("compose = %s", got.ComposeYAML)
	}
}

// Catches an explicit update_existing=false not overriding the saved value,
// and a conflict being reported after the import has started.
func TestTeamImportApplyTeamConflictPrecedesStart(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	if _, err := h(t, "group.create")(c, registry.Params{"name": "crew"}); err != nil {
		t.Fatal(err)
	}
	// Both the team and the agent conflict: dev1 is in no group.
	id := stageTeamImport(t, c, "version: 1\ngroups:\n  crew: {}\nagents:\n  dev1:\n    image: worker:latest\n    group: crew\n")
	operation := importOperation(t, c, id)
	operation.UpdateExisting = true
	if err := importService(c).SaveOperation(operation); err != nil {
		t.Fatal(err)
	}
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id, "update_existing": false})
	if !isCode(err, "team_conflict") {
		t.Fatalf("err = %#v, want team_conflict", err)
	}
	_, err = h(t, "team.import.archive.apply")(c, registry.Params{"id": id, "update_existing": true})
	if !isCode(err, "agent_conflict") || !strings.Contains(err.Error(), "dev1: destination agent has different group or image") {
		t.Fatalf("err = %#v, want agent_conflict", err)
	}
	if got := importOperation(t, c, id); got.Status != "pending" || !got.UpdateExisting {
		t.Fatalf("operation = %#v", got)
	}
	assertNoImportLease(t, c, id)
}

// Catches the lease being released before the apply runs, and a failed apply
// not being recorded with its error before the lease goes.
func TestTeamImportApplyHoldsLeaseAndRecordsFailure(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	createErr := errors.New("disk full")
	c.Groups = hookGroups{GroupControl: c.Groups, create: func(string, string) (map[string]any, error) {
		if _, err := os.Stat(importLeasePath(c, id)); err != nil {
			t.Errorf("lease during apply: %v", err)
		}
		if got := importOperation(t, c, id); got.Status != "running" || !strings.Contains(got.ComposeYAML, "worker:latest") {
			t.Errorf("operation during apply = %#v", got)
		}
		return nil, createErr
	}}
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id})
	if !isCode(err, "create_failed") {
		t.Fatalf("err = %#v, want create_failed", err)
	}
	if got := importOperation(t, c, id); got.Status != "failed" || got.Error != err.Error() || len(got.Steps) != 0 {
		t.Fatalf("operation = %#v", got)
	}
	assertNoImportLease(t, c, id)
}

// Catches an incomplete apply being reported as an error or as complete: the
// result carries the failed agents, and the operation their steps.
func TestTeamImportApplyRecordsIncompleteResult(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	c.Control = failingRunControl{fakeControl: c.Control.(*fakeControl), err: errors.New("no provision")}
	value, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(TeamImportApplyResult)
	if result.Complete || result.Operation.Status != "failed" {
		t.Fatalf("result = %#v", result)
	}
	got := importOperation(t, c, id)
	if got.Status != "failed" || got.Error != "" || len(got.Steps) != 1 {
		t.Fatalf("operation = %#v", got)
	}
	if step := got.Steps[0]; step.Kind != "agent" || step.Name != "newbie" || step.Status != "failed" || step.Error != "no provision" {
		t.Fatalf("step = %#v", step)
	}
	assertNoImportLease(t, c, id)
}

// Catches an unsaved "running" state starting the apply: resume could not tell
// what ran.
func TestTeamImportApplyStopsWhenRunningStateIsUnsaved(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	freezeImportDir(t, c, id)
	c.Groups = hookGroups{GroupControl: c.Groups, create: func(string, string) (map[string]any, error) {
		t.Error("apply ran without a saved operation")
		return nil, nil
	}}
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id})
	if !isCode(err, "import_state_unsaved") || !strings.Contains(err.Error(), `at step "running"`) {
		t.Fatalf("err = %#v, want import_state_unsaved at running", err)
	}
}

// Catches a failed apply losing either error when its state cannot be saved.
func TestTeamImportApplyJoinsApplyAndSaveErrors(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	c.Groups = hookGroups{GroupControl: c.Groups, create: func(string, string) (map[string]any, error) {
		freezeImportDir(t, c, id)
		return nil, errors.New("disk full")
	}}
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id})
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) || len(joined.Unwrap()) != 2 {
		t.Fatalf("err = %#v, want two joined errors", err)
	}
	var first, second api.UserError
	if !errors.As(joined.Unwrap()[0], &first) || first.Code != "create_failed" {
		t.Fatalf("apply error = %#v", joined.Unwrap()[0])
	}
	if !errors.As(joined.Unwrap()[1], &second) || second.Code != "import_state_unsaved" || !strings.Contains(second.Msg, `at step "failed"`) {
		t.Fatalf("save error = %#v", joined.Unwrap()[1])
	}
}

// Catches a finished import reporting success although its final state was
// not saved.
func TestTeamImportApplyFailsWhenFinalStateIsUnsaved(t *testing.T) {
	t.Parallel()
	c := groupsFixture(t)
	id := stageTeamImport(t, c, importFreshWorker)
	real := c.Groups
	c.Groups = hookGroups{GroupControl: real, create: func(name, lead string) (map[string]any, error) {
		freezeImportDir(t, c, id)
		return real.Create(name, lead)
	}}
	_, err := h(t, "team.import.archive.apply")(c, registry.Params{"id": id})
	if !isCode(err, "import_state_unsaved") || !strings.Contains(err.Error(), `at step "complete"`) {
		t.Fatalf("err = %#v, want import_state_unsaved at complete", err)
	}
}
