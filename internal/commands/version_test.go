package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/registry"
)

func TestVersionCommandIsLocal(t *testing.T) {
	cmd, ok := BuildRegistry().Get("version")
	if !ok {
		t.Fatal("version command is not registered")
	}
	if cmd.CLIHidden {
		t.Fatal("version command is hidden from the CLI")
	}
	if cmd.HTTP != nil {
		t.Fatalf("version command has an HTTP route: %#v", cmd.HTTP)
	}

	got, err := cmd.Handler(&registry.Ctx{Version: "1.2.3"}, nil)
	if err != nil {
		t.Fatalf("version handler: %v", err)
	}
	if got != "1.2.3" {
		t.Fatalf("version handler returned %#v, want %q", got, "1.2.3")
	}
}
