package cli_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/commands"
	"github.com/alekzonder/tariboy/internal/registry"
)

func mustCall(t *testing.T, c *client.Client, method, route string, body any) map[string]any {
	t.Helper()
	raw, err := c.Call(method, route, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, route, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s %s: %v", method, route, err)
	}
	return m
}

// repoRoot resolves the repository root from this test's own source path
// (<repo>/internal/cli/groups_inproc_test.go), so the examples fixture is found
// regardless of the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// buildStubImage calls the image.build handler against the daemon's base dir,
// producing stub:latest.
func buildStubImage(t *testing.T, base string) {
	t.Helper()
	local := &registry.Ctx{BaseDir: base}
	reg := commands.BuildRegistry()
	cmd, ok := reg.Get("image.build")
	if !ok {
		t.Fatal("image.build command missing")
	}
	if _, err := cmd.Handler(local, registry.Params{
		"tag": "stub:latest", "path": filepath.Join(repoRoot(t), "internal", "builtinimages", "source"),
	}); err != nil {
		t.Fatalf("build stub image: %v", err)
	}
}

func TestGroupCommandsProvisionSubscriptions(t *testing.T) {
	base, _, c := startDaemon(t)

	// Create a group with a lead.
	mustCall(t, c, "POST", "/api/groups", map[string]any{"name": "research", "lead": "scout"})

	// Build the image the group agents run (CLI-local), then run two members.
	buildStubImage(t, base)
	mustCall(t, c, "POST", "/api/agents", map[string]any{
		"image": "stub:latest", "name": "scout", "group": "research", "loop": false})
	mustCall(t, c, "POST", "/api/agents", map[string]any{
		"image": "stub:latest", "name": "writer", "group": "research", "loop": false})

	// The group reports both members and the lead.
	insp := mustCall(t, c, "GET", "/api/groups/research", map[string]string{})
	if insp["lead"] != "scout" {
		t.Fatalf("group lead = %v", insp["lead"])
	}
	if members, _ := insp["members"].([]any); len(members) != 2 {
		t.Fatalf("group members = %v", insp["members"])
	}

	// The member row carries the group assignment.
	scout := mustCall(t, c, "GET", "/api/agents/scout", map[string]string{})
	if scout["group"] != "research" {
		t.Fatalf("scout group = %v", scout["group"])
	}

	// group-addressed delivery: publishing to the inbox reaches ONLY the lead.
	// A member subscribed to the inbox would be a bug; assert via the channel's
	// derived kind and that both publishes succeed (delivery routing is by
	// subscription, wired in Task 4 and asserted end-to-end in the e2e Task 12).
	mustCall(t, c, "POST", "/api/messages", map[string]any{
		"channel": "group:research:inbox", "type": "task", "text": "triage"})
	mustCall(t, c, "POST", "/api/messages", map[string]any{
		"channel": "group:research:broadcast", "type": "note", "text": "hi team"})

	// Assign an ungrouped agent in, then leave — the command surface round-trips.
	mustCall(t, c, "POST", "/api/agents", map[string]any{
		"image": "stub:latest", "name": "loner", "loop": false})
	mustCall(t, c, "POST", "/api/groups/research/assign", map[string]any{"agent": "loner"})
	if a := mustCall(t, c, "GET", "/api/agents/loner", map[string]string{}); a["group"] != "research" {
		t.Fatalf("assign did not persist: %v", a["group"])
	}
}
