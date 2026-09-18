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
	return registry.Command{
		Path: "iteration.extend-timeout", Summary: "Extend a running iteration timeout",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "id", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/iterations/{id}/extend-timeout"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name, id := str(p, "name"), str(p, "id")
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
			return map[string]any{"id": id, "timeout_deadline": result.TimeoutDeadline,
				"hard_timeout_deadline": result.HardTimeoutDeadline, "timeout_extensions": result.TimeoutExtensions,
				"shim_sync": result.ShimSync}, nil
		},
	}
}

func agentsDir(c *registry.Ctx) string { return paths.Paths{Base: c.BaseDir}.AgentsDir() }

func iterationLs() registry.Command {
	return registry.Command{
		Path:    "iteration.ls",
		Summary: "List an agent's iterations",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "tag", Flag: "tag", Type: registry.String, Help: "keep only iterations carrying any of these tags (comma-separated)"},
			{Name: "started-after", Flag: "started-after", Type: registry.String, Help: "keep iterations started at or after this RFC3339 time or YYYY-MM-DD day"},
			{Name: "started-before", Flag: "started-before", Type: registry.String, Help: "keep iterations started at or before this RFC3339 time or YYYY-MM-DD day"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			store := agentStore(c)
			its, err := store.FindIterations(str(p, "name"), agent.IterationFilter{
				Tags:          stringSliceParam(p, "tag"),
				StartedAfter:  str(p, "started-after"),
				StartedBefore: str(p, "started-before"),
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
			rows := make([]map[string]any, 0, len(its))
			for _, it := range its {
				rows = append(rows, map[string]any{
					"id": it.ID, "trigger": it.Trigger, "status": it.Status,
					"started_at": it.StartedAt, "done": it.DoneFlag,
					"productive": it.Productive, "tags": tags[it.ID],
				})
			}
			return map[string]any{"iterations": rows, "count": len(rows)}, nil
		},
	}
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
	return registry.Command{
		Path:    "iteration.tag." + map[agent.TagOp]string{agent.TagOpAdd: "add", agent.TagOpRemove: "rm", agent.TagOpSet: "set"}[op],
		Summary: summary,
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Flag: "id", Type: registry.String, Repeatable: true, Required: true, Help: "iteration id (repeat for a batch)"},
			{Name: "tag", Flag: "tag", Type: registry.String, Repeatable: true, Help: "tag (repeat for several; empty with set clears every tag)"},
		},
		HTTP: &registry.HTTPRoute{Method: http.MethodPost, Path: "/api/agents/{name}/iterations/tags/" + string(op)},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			name := str(p, "name")
			if _, err := getAgent(c, name); err != nil {
				return nil, err
			}
			ids := stringSliceParam(p, "id")
			if len(ids) == 0 {
				return nil, api.UserError{Code: "missing_id", Msg: "at least one iteration id is required", Status: http.StatusBadRequest}
			}
			tags, err := agentStore(c).MutateIterationTags(name, op, ids, stringSliceParam(p, "tag"))
			if err != nil {
				return nil, iterationTagError(err)
			}
			return map[string]any{"tags": tags, "count": len(tags)}, nil
		},
	}
}

func iterationInspect() registry.Command {
	return registry.Command{
		Path:    "iteration.inspect",
		Summary: "Show one iteration",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "iteration id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			it, err := agentStore(c).GetIteration(str(p, "name"), str(p, "id"))
			if err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "iteration not found"}
			}
			tags, err := agentStore(c).IterationTags([]string{it.ID})
			if err != nil {
				return nil, err
			}
			out := map[string]any{
				"id": it.ID, "agent": it.Agent, "trigger": it.Trigger, "status": it.Status,
				"started_at": it.StartedAt, "ended_at": it.EndedAt, "done": it.DoneFlag,
				"productive":  it.Productive,
				"prompt_path": it.PromptPath,
				"image_ref":   it.ImageRef, "image_version": it.ImageVersion, "image_digest": it.ImageDigest,
				"prompt_template_sha256": it.PromptTemplateSHA256,
				"tags":                   tags[it.ID],
			}
			if it.ExitCode != nil {
				out["exit_code"] = *it.ExitCode
			}
			if it.CPUMs != nil {
				out["cpu_ms"] = *it.CPUMs
			}
			if it.MemPeakKB != nil {
				out["mem_peak_kb"] = *it.MemPeakKB
			}
			return out, nil
		},
	}
}

func iterationLogs() registry.Command {
	return registry.Command{
		Path:    "iteration.logs",
		Summary: "Print an iteration's harness logs",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "id", Type: registry.String, Required: true, Help: "iteration id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}/logs"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if _, err := getAgent(c, str(p, "name")); err != nil {
				return nil, err // api.UserError not_found
			}
			id := str(p, "id")
			if _, err := agentStore(c).GetIteration(str(p, "name"), id); err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "iteration not found"}
			}
			l := agentdir.New(agentsDir(c), str(p, "name"))
			stdout, _ := os.ReadFile(l.HarnessStdout(id))
			stderr, _ := os.ReadFile(l.HarnessStderr(id))
			return map[string]any{"stdout": string(stdout), "stderr": string(stderr)}, nil
		},
	}
}
