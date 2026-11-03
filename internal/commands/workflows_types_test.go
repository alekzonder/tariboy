package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

func TestWorkflowManifestFromKeepsStickyHolder(t *testing.T) {
	no := false
	view, err := workflowManifestFrom(workflowimage.Manifest{Definition: workflowfile.File{
		StickyHolder: true,
		Statuses: []workflowfile.Status{
			{ID: "plan", Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: "developers"}, StickyHolder: &no},
			{ID: "implement", Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: "developers"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Definition.StickyHolder {
		t.Fatal("definition sticky_holder lost")
	}
	if s := view.Definition.Statuses[0].StickyHolder; s == nil || *s {
		t.Fatalf("plan sticky_holder = %v, want false", s)
	}
	if s := view.Definition.Statuses[1].StickyHolder; s != nil {
		t.Fatalf("implement sticky_holder = %v, want unset", *s)
	}
}
