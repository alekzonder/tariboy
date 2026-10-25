package commands

import (
	"context"
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentAliasSetReadClear(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentAlias(), registry.Params{"name": "a1", "value": "Nice"}); err != nil {
		t.Fatal(err)
	}
	res, err := registry.Call(c, context.Background(), agentAliasGet(), registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(AgentAliasResult).Alias != "Nice" {
		t.Fatalf("read=%v", res)
	}
	// clear with empty
	if _, err := registry.Call(c, context.Background(), agentAlias(), registry.Params{"name": "a1", "value": ""}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Alias != "" {
		t.Fatalf("alias not cleared: %q", a.Alias)
	}
}

func TestAgentAliasReadDoesNotWrite(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentAlias(), registry.Params{"name": "a1", "value": "keep"}); err != nil {
		t.Fatal(err)
	}
	// POST with NO value key = read, must not clear.
	if _, err := registry.Call(c, context.Background(), agentAlias(), registry.Params{"name": "a1"}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Alias != "keep" {
		t.Fatalf("read clobbered alias: %q", a.Alias)
	}
}

func TestAgentNotesSetRead(t *testing.T) {
	t.Parallel()
	c, _ := seedAgent(t)
	if _, err := registry.Call(c, context.Background(), agentNotes(), registry.Params{"name": "a1", "value": "scratch"}); err != nil {
		t.Fatal(err)
	}
	res, err := registry.Call(c, context.Background(), agentNotesGet(), registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(AgentNotesResult).Notes != "scratch" {
		t.Fatalf("read=%v", res)
	}
}
