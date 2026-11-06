package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

// Catches a team import applying part of a version 2 file: sections that
// compose up does not apply yet are refused before anything is created.
func TestTeamImportRefusesUnappliedVersion2Sections(t *testing.T) {
	t.Parallel()
	const doc = "version: 2\nstores:\n  team: {path: ./store}\nimages:\n  worker: {path: ./worker}\ngroups:\n  fresh: {lead: newbie}\nagents:\n  newbie:\n    image: worker:latest\n    group: fresh\n"
	c := groupsFixture(t)
	_, previewErr := h(t, "team.import.yaml")(c, registry.Params{"yaml": doc})
	// The apply step is not registered; the archive apply calls it directly.
	_, applyErr := registry.Call(c, context.Background(), teamApplyYAML(), map[string]any{"yaml": doc})
	for name, err := range map[string]error{"preview": previewErr, "apply": applyErr} {
		if !isCode(err, "bad_compose") || !strings.Contains(err.Error(), "section stores is not applied") {
			t.Fatalf("%s err = %#v, want bad_compose naming stores", name, err)
		}
	}
}
