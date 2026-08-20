package commands

import (
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/evals"
	"github.com/alekzonder/tariboy/internal/registry"
)

func evalResultView(r evals.Result) map[string]any {
	return map[string]any{
		"id": r.ID, "iteration": r.Iteration, "agent": r.Agent,
		"image_name": r.ImageName, "image_tag": r.ImageTag, "image_digest": r.ImageDigest,
		"eval_name": r.EvalName, "eval_type": r.EvalType,
		"verdict": r.Verdict, "score": r.Score, "detail": r.Detail, "created_at": r.CreatedAt,
	}
}

func evalLs() registry.Command {
	return registry.Command{
		Path:    "eval.ls",
		Summary: "List recent eval results (verdict/score per iteration + image version)",
		Args: []registry.Arg{
			{Name: "limit", Flag: "limit", Short: "n", Type: registry.Int, Help: "max rows (default 50)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/evals"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			list, err := evals.NewStore(c.Store, nil).List(intOf(p, "limit", 50))
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, r := range list {
				rows = append(rows, evalResultView(r))
			}
			return map[string]any{"evals": rows, "count": len(rows)}, nil
		},
	}
}

func evalInspect() registry.Command {
	return registry.Command{
		Path:    "eval.inspect",
		Summary: "Show all eval results for an iteration (with the image version)",
		Args: []registry.Arg{
			{Name: "iteration", Type: registry.String, Required: true, Help: "iteration id"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/evals/{iteration}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			iteration := str(p, "iteration")
			if iteration == "" {
				return nil, api.UserError{Code: "bad_iteration", Msg: "iteration is required"}
			}
			list, err := evals.NewStore(c.Store, nil).ListByIteration(iteration)
			if err != nil {
				return nil, err
			}
			rows := make([]map[string]any, 0, len(list))
			for _, r := range list {
				rows = append(rows, evalResultView(r))
			}
			return map[string]any{"iteration": iteration, "results": rows, "count": len(rows)}, nil
		},
	}
}
