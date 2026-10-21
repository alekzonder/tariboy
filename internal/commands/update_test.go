package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/cli"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/selfupdate"
)

// TestUpdateParsesArguments is not parallel: it replaces runSelfUpdate.
func TestUpdateParsesArguments(t *testing.T) {
	var got *selfupdate.Options
	saved := runSelfUpdate
	runSelfUpdate = func(_ context.Context, o selfupdate.Options) error { got = &o; return nil }
	t.Cleanup(func() { runSelfUpdate = saved })
	for _, tc := range []struct {
		args    string
		version string
		force   bool
		cleanup bool
		keep    int
	}{
		{args: "update", version: "latest"},
		{args: "update 0.64.0", version: "0.64.0"},
		{args: "update latest --force", version: "latest", force: true},
		{args: "update --cleanup", version: "latest", cleanup: true, keep: 5},
		{args: "update 0.82.0 --cleanup --keep 3", version: "0.82.0", cleanup: true, keep: 3},
	} {
		got = nil
		var out, errOut bytes.Buffer
		code := cli.Run(context.Background(), BuildRegistry(), strings.Fields(tc.args), nil, &registry.Ctx{}, &out, &errOut)
		if code != 0 || got == nil {
			t.Fatalf("%s: code=%d stderr=%q", tc.args, code, errOut.String())
		}
		if got.Version != tc.version || got.Force != tc.force || got.Cleanup != tc.cleanup || got.Keep != tc.keep {
			t.Fatalf("%s: parsed %+v", tc.args, *got)
		}
	}
}
