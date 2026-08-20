package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestAgentColorSetReadClear(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentColor().Handler(c, registry.Params{"name": "a1", "value": "#4f8cff"}); err != nil {
		t.Fatal(err)
	}
	res, err := agentColorGet().Handler(c, registry.Params{"name": "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["color"] != "#4f8cff" {
		t.Fatalf("read=%v", res)
	}
	// clear with empty
	if _, err := agentColor().Handler(c, registry.Params{"name": "a1", "value": ""}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Color != "" {
		t.Fatalf("color not cleared: %q", a.Color)
	}
}

func TestAgentColorReadDoesNotWrite(t *testing.T) {
	c, _ := seedAgent(t)
	if _, err := agentColor().Handler(c, registry.Params{"name": "a1", "value": "#ff8800"}); err != nil {
		t.Fatal(err)
	}
	// POST with NO value key = read, must not clear.
	if _, err := agentColor().Handler(c, registry.Params{"name": "a1"}); err != nil {
		t.Fatal(err)
	}
	a, _ := getAgent(c, "a1")
	if a.Color != "#ff8800" {
		t.Fatalf("read clobbered color: %q", a.Color)
	}
}

func TestAgentColorRejectsInvalidHex(t *testing.T) {
	c, _ := seedAgent(t)
	_, err := agentColor().Handler(c, registry.Params{"name": "a1", "value": "blue"})
	if _, ok := err.(api.UserError); !ok {
		t.Fatalf("want UserError for invalid hex, got %v", err)
	}
	// invalid value must not persist
	a, _ := getAgent(c, "a1")
	if a.Color != "" {
		t.Fatalf("invalid color persisted: %q", a.Color)
	}
}

func TestIsValidHex(t *testing.T) {
	good := []string{"#000000", "#4f8cff", "#ABCDEF", "#ff8800"}
	for _, s := range good {
		if !isValidHex(s) {
			t.Errorf("isValidHex(%q) = false, want true", s)
		}
	}
	// 3-digit shorthand (#fff, #f80) is now rejected: canonical form is 6-digit
	// only, matching the frontend's isValidHex.
	bad := []string{"", "fff", "#ff", "#fff", "#f80", "#gggggg", "#12345", "#1234567", "blue", "4f8cff"}
	for _, s := range bad {
		if isValidHex(s) {
			t.Errorf("isValidHex(%q) = true, want false", s)
		}
	}
}
