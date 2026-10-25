package commands

import (
	"errors"
	"net/http"
	"os"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/paths"
	"github.com/alekzonder/tariboy/internal/registry"
)

func iterationExtendTimeout() registry.Command {
	return registry.WithResult[IterationTimeoutResult](registry.Command{
		Path: "iteration.extend-timeout", Summary: "Extend a running iteration timeout",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "id", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/iterations/{id}/extend-timeout"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name, id := p.String("name"), p.String("id")
			if _, err := getAgent(c, name); err != nil {
				return nil, err
			}
			result, err := c.Control.ExtendIterationTimeout(name, id)
			if err != nil {
				if errors.Is(err, agent.ErrNotFound) {
					return nil, api.UserError{Code: "not_found", Msg: "iteration not found", Status: http.StatusNotFound}
				}
				if errors.Is(err, agent.ErrNoIterationTimeout) {
					return nil, api.UserError{Code: "no_timeout", Msg: "iteration has no soft timeout", Status: http.StatusUnprocessableEntity}
				}
				if errors.Is(err, agent.ErrTimeoutNotExtendable) {
					return nil, api.UserError{Code: "timeout_not_extendable", Msg: "iteration timeout is not extendable", Status: http.StatusConflict}
				}
				return nil, err
			}
			return IterationTimeoutResult{ID: id, TimeoutDeadline: result.TimeoutDeadline,
				HardTimeoutDeadline: result.HardTimeoutDeadline, TimeoutExtensions: result.TimeoutExtensions,
				ShimSync: result.ShimSync}, nil
		},
	})
}

func agentsDir(c *registry.Ctx) string { return paths.Paths{Base: c.BaseDir}.AgentsDir() }

func iterationLs() registry.Command {
	return registry.WithResult[IterationList](registry.Command{
		Path:    "iteration.ls",
		Summary: "List an agent's iterations",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Help: "keep only iterations carrying any of these tags (comma-separated)"},
			{Name: "started-after", Flag: "started-after", Type: registry.String, Help: "keep iterations started at or after this RFC 3339 time or YYYY-MM-DD day"},
			{Name: "started-before", Flag: "started-before", Type: registry.String, Help: "keep iterations started at or before this RFC 3339 time or YYYY-MM-DD day"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			store := agentStore(c)
			its, err := store.FindIterations(p.String("name"), agent.IterationFilter{
				Tags:          p.Strings("tag"),
				StartedAfter:  p.String("started-after"),
				StartedBefore: p.String("started-before"),
			})
			if err != nil {
				return nil, iterationTagError(err)
			}
			ids := make([]string, 0, len(its))
			for _, it := range its {
				ids = append(ids, it.ID)
			}
			tags, err := store.IterationTags(ids)
			if err != nil {
				return nil, err
			}
			rows := make([]IterationRow, 0, len(its))
			for _, it := range its {
				rows = append(rows, IterationRow{
					ID: it.ID, Trigger: it.Trigger, Status: string(it.Status),
					StartedAt: it.StartedAt, Done: it.DoneFlag,
					Productive: it.Productive, Tags: tags[it.ID],
				})
			}
			return IterationList{Iterations: rows, Count: len(rows)}, nil
		},
	})
}

// iterationTagError maps the tag/filter validation errors onto the HTTP surface.
func iterationTagError(err error) error {
	switch {
	case errors.Is(err, agent.ErrInvalidTag):
		return api.UserError{Code: "invalid_tag", Msg: err.Error(), Status: http.StatusUnprocessableEntity}
	case errors.Is(err, agent.ErrInvalidTime):
		return api.UserError{Code: "invalid_time", Msg: err.Error(), Status: http.StatusUnprocessableEntity}
	case errors.Is(err, agent.ErrNotFound):
		return api.UserError{Code: "not_found", Msg: err.Error(), Status: http.StatusNotFound}
	}
	return err
}

// iterationTag builds one of the three tag mutations. They share a route and
// differ only in op, so a batch of ids and tags travels in one request and one
// transaction: the whole batch applies or none of it does.
func iterationTag(op agent.TagOp, summary string) registry.Command {
	return registry.WithResult[IterationTagResult](registry.Command{
		Path:    "iteration.tag." + map[agent.TagOp]string{agent.TagOpAdd: "add", agent.TagOpRemove: "rm", agent.TagOpSet: "set"}[op],
		Summary: summary,
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Flag: "id", Type: registry.String, Repeatable: true, Required: true, Help: "iteration id (repeat for a batch)"},
			{Name: "tag", Flag: "tag", Type: registry.String, Repeatable: true, Help: "tag (repeat for several; empty with set clears every tag)"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/iterations/tags/" + string(op)},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			name := p.String("name")
			if _, err := getAgent(c, name); err != nil {
				return nil, err
			}
			ids := p.Strings("id")
			if len(ids) == 0 {
				return nil, api.UserError{Code: "missing_id", Msg: "at least one iteration id is required", Status: http.StatusBadRequest}
			}
			tags, err := agentStore(c).MutateIterationTags(name, op, ids, p.Strings("tag"))
			if err != nil {
				return nil, iterationTagError(err)
			}
			return IterationTagResult{Tags: tags, Count: len(tags)}, nil
		},
	})
}

func iterationInspect() registry.Command {
	return registry.WithResult[IterationView](registry.Command{
		Path:    "iteration.inspect",
		Summary: "Show one iteration",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "iteration id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			it, err := agentStore(c).GetIteration(p.String("name"), p.String("id"))
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "iteration not found"}
			}
			tags, err := agentStore(c).IterationTags([]string{it.ID})
			if err != nil {
				return nil, err
			}
			return IterationView{
				ID: it.ID, Agent: it.Agent, Trigger: it.Trigger, Status: string(it.Status),
				StartedAt: it.StartedAt, EndedAt: it.EndedAt, Done: it.DoneFlag,
				Productive: it.Productive, PromptPath: it.PromptPath,
				ImageRef: it.ImageRef, ImageVersion: it.ImageVersion, ImageDigest: it.ImageDigest,
				PromptTemplateSHA256: it.PromptTemplateSHA256, Tags: tags[it.ID],
				ExitCode: it.ExitCode, CPUMs: it.CPUMs, MemPeakKB: it.MemPeakKB,
			}, nil
		},
	})
}

func iterationLogs() registry.Command {
	return registry.WithResult[IterationLogs](registry.Command{
		Path:    "iteration.logs",
		Summary: "Print an iteration's harness logs",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "iteration id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}/logs"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			if _, err := getAgent(c, p.String("name")); err != nil {
				return nil, err // api.UserError not_found
			}
			id := p.String("id")
			if _, err := agentStore(c).GetIteration(p.String("name"), id); err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "iteration not found"}
			}
			l := agentdir.New(agentsDir(c), p.String("name"))
			stdout, _ := os.ReadFile(l.HarnessStdout(id))
			stderr, _ := os.ReadFile(l.HarnessStderr(id))
			return IterationLogs{Stdout: string(stdout), Stderr: string(stderr)}, nil
		},
	})
}
