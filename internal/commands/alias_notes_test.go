package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentAliasSetReadClear(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentAlias().Handler(c, registry.Params{"name": "a1", "value": "Nice"}); err != nil {
		t.Fatal(err)
	}
	res, err := agentAliasGet().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["alias"] != "Nice" {
		t.Fatalf("read=%v", res)
	}
	// clear with empty
	if _, err := agentAlias().Handler(c, registry.Params{"name": "a1", "value": ""}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Alias != "" {
		t.Fatalf("alias not cleared: %q", a.Alias)
	}
}

func TestAgentAliasReadDoesNotWrite(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentAlias().Handler(c, registry.Params{"name": "a1", "value": "keep"}); err != nil {
		t.Fatal(err)
	}
	// POST with NO value key = read, must not clear.
	if _, err := agentAlias().Handler(c, registry.Params{"name": "a1"}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Alias != "keep" {
		t.Fatalf("read clobbered alias: %q", a.Alias)
	}
}

func TestAgentNotesSetRead(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentNotes().Handler(c, registry.Params{"name": "a1", "value": "scratch"}); err != nil {
		t.Fatal(err)
	}
	res, err := agentNotesGet().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["notes"] != "scratch" {
		t.Fatalf("read=%v", res)
	}
}
