package commands

import (
	"github.com/alekzonder/tariboy/internal/aiproxy/session"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

// transcriptCommand serves the AI proxy transcript for one iteration, parsed
// into a provider-agnostic SessionTimeline. ?raw=1 returns the decoded raw
// request/response bytes per call instead. Runs in the daemon (reads the agent
// dir on disk); a missing transcript is a normal empty result, not an error.
func transcriptCommand() registry.Command {
	return registry.WithResult[TranscriptResult](registry.Command{
		Path:    "transcript",
		Summary: "Show the AI proxy transcript for an iteration (parsed; ?raw=1 for bytes)",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "iteration id"},
			{Name: "raw", Flag: "raw", Type: registry.Bool, Help: "return raw request/response bytes"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}/transcript"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			agentsDir := paths.New(c.BaseDir).AgentsDir()
			name := p.String("name")
			id := p.String("id")
			entries, err := session.ReadEntries(agentsDir, name, id)
			if err != nil {
				return nil, err
			}
			// toBool (agents.go) is the package's existing idiom for reading a
			// registry.Bool flag: it accepts both the CLI's typed bool and the
			// daemon HTTP layer's stringified query param ("true"/"1"). Same
			// pattern as force (agents.go, plugin.go), volumes (group.go), and
			// disabled (rule.go).
			if p.Bool("raw") {
				calls := session.RawCalls(entries)
				return TranscriptResult{Calls: calls, Count: len(calls)}, nil
			}
			tl := session.Build(entries)
			return TranscriptResult{Calls: tl.Calls, Count: len(tl.Calls)}, nil
		},
	})
}
