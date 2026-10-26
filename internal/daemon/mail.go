package daemon

import (
	"strings"

	"github.com/alekzonder/tariboy/internal/mail"
)

// mailHintAgent is the agent a mail hint belongs to in the event hub: the
// recipient agent, else the sending agent, else none.
func mailHintAgent(h mail.Hint) string {
	for _, p := range []string{h.To, h.From} {
		if name, ok := strings.CutPrefix(p, "agent:"); ok {
			return name
		}
	}
	return ""
}
