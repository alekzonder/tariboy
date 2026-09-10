package commands

import (
	"errors"
	"net/http"
	"os"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/judge"
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
		Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
		HTTP:    &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			its, err := agentStore(c).ListIterations(str(p, "name"))
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(its))
			for i := range its {
				ids[i] = its[i].ID
			}
			reviews, err := iterationJudgeReviews(c, ids)
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(its))
			for _, it := range its {
				rows = append(rows, map[string]any{
					"id": it.ID, "trigger": it.Trigger, "status": it.Status,
					"started_at": it.StartedAt, "done": it.DoneFlag,
					"productive": it.Productive,
					"judge":      judge.ProjectIterationJudgeReviews(reviews[it.ID]),
				})
			}
			return map[string]any{"iterations": rows, "count": len(rows)}, nil
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
			out := map[string]any{
				"id": it.ID, "agent": it.Agent, "trigger": it.Trigger, "status": it.Status,
				"started_at": it.StartedAt, "ended_at": it.EndedAt, "done": it.DoneFlag,
				"productive":  it.Productive,
				"prompt_path": it.PromptPath,
				"image_ref":   it.ImageRef, "image_version": it.ImageVersion, "image_digest": it.ImageDigest,
				"prompt_template_sha256": it.PromptTemplateSHA256,
			}
			reviews, err := iterationJudgeReviews(c, []string{it.ID})
			if err != nil {
				return nil, err
			}
			out["judge"] = judge.ProjectIterationJudgeReviews(reviews[it.ID])
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

func iterationJudges() registry.Command {
	return registry.Command{
		Path: "iteration.judges", Summary: "List Judge reviews for one iteration",
		Args: []registry.Arg{{Name: "name", Type: registry.String, Required: true}, {Name: "id", Type: registry.String, Required: true}},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/iterations/{id}/judges"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			id := str(p, "id")
			if _, err := agentStore(c).GetIteration(str(p, "name"), id); err != nil {
				return nil, api.UserError{Code: "not_found", Msg: "iteration not found"}
			}
			reviews, err := iterationJudgeReviews(c, []string{id})
			if err != nil {
				return nil, err
			}
			return map[string]any{"reviews": reviews[id]}, nil
		},
	}
}

func iterationJudgeReviews(c *registry.Ctx, ids []string) (map[string][]judge.IterationJudgeReview, error) {
	if c.Judges == nil {
		out := make(map[string][]judge.IterationJudgeReview, len(ids))
		for _, id := range ids {
			out[id] = []judge.IterationJudgeReview{}
		}
		return out, nil
	}
	control, ok := c.Judges.(registry.IterationJudgeControl)
	if !ok {
		return nil, api.UserError{Code: "no_judge_control", Msg: "iteration Judge control is not available"}
	}
	return control.OperatorIterationReviews(ids)
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
