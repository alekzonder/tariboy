package commands

import (
	"strings"
	"testing"
)

// TestMailRegistryCommandsAreHTTPOnly: `tariboy mail` is the tmail client,
// so the mail.* registry commands serve HTTP only and stay out of the CLI
// help and the generated operator table.
func TestMailRegistryCommandsAreHTTPOnly(t *testing.T) {
	r := BuildRegistry()
	n := 0
	for _, c := range r.Commands() {
		if !strings.HasPrefix(c.Path, "mail.") {
			continue
		}
		n++
		if !c.CLIHidden || c.HTTP == nil {
			t.Errorf("%s: CLIHidden=%v HTTP=%v; want an HTTP-only command", c.Path, c.CLIHidden, c.HTTP)
		}
	}
	if n == 0 {
		t.Fatal("no mail.* commands registered")
	}
	if c, ok := r.Get("agent.mail.clear"); !ok || c.CLIHidden {
		t.Fatal("agent.mail.clear keeps its CLI row")
	}
}
