package commands

import (
	"database/sql"
	"errors"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/judge"
	"github.com/alekzonder/tariboy/internal/registry"
)

func requireJudges(c *registry.Ctx) (registry.JudgeControl, error) {
	if c.Judges == nil {
		return nil, api.UserError{Code: "no_judge_control", Msg: "judge control is not available"}
	}
	return c.Judges, nil
}
func requireJudgeAutomation(c *registry.Ctx) (registry.JudgeAutomationControl, error) {
	if c.JudgeAutomation == nil {
		return nil, api.UserError{Code: "no_judge_automation_control", Msg: "judge automation control is not available"}
	}
	return c.JudgeAutomation, nil
}
func judgeError(err error) error {
	if errors.Is(err, judge.ErrNotFound) {
		return api.UserError{Code: "not_found", Msg: err.Error()}
	}
	if errors.Is(err, judge.ErrBadLocator) {
		return api.UserError{Code: "bad_locator", Msg: "evidence locator must be a stable bundle locator"}
	}
	return err
}
func judgeUsage(c *registry.Ctx, targets []judge.Target) ([]map[string]any, error) {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.Iteration)
	}
	rows, err := aiproxy.NewStore(c.Store, nil).AggregateIterations(ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]aiproxy.IterationUsageRow, len(rows))
	for _, r := range rows {
		byID[r.Iteration] = r
	}
	out := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		r := byID[t.Iteration]
		out = append(out, map[string]any{"iteration": t.Iteration, "requests": r.Requests, "input_tokens": r.InputTokens, "output_tokens": r.OutputTokens, "cache_write_tokens": r.CacheWriteTokens, "cache_read_tokens": r.CacheReadTokens, "cost_usd": r.CostUSD})
	}
	return out, nil
}
func judgeLs() registry.Command {
	return registry.Command{
		Path: "judge.ls", Summary: "List LLM-as-Judge runs", Args: []registry.Arg{{Name: "status", Flag: "status", Type: registry.String, Help: "optional run status"}, {Name: "limit", Flag: "limit", Short: "n", Type: registry.Int, Help: "max rows"}}, HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/judges"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			jc, e := requireJudges(c)
			if e != nil {
				return nil, e
			}
			f := judge.ListFilter{Limit: intOf(p, "limit", 0)}
			if s := str(p, "status"); s != "" {
				f.Statuses = []judge.RunStatus{judge.RunStatus(s)}
			}
			rs, e := jc.OperatorList(f)
			if e != nil {
				return nil, judgeError(e)
			}
			return map[string]any{"runs": rs, "count": len(rs)}, nil
		},
	}
}
func judgeInspect() registry.Command {
	return registry.Command{
		Path: "judge.inspect", Summary: "Show an LLM-as-Judge run, targets, analyses, summaries and target usage", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "run id"}}, HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/judges/{id}"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			jc, e := requireJudges(c)
			if e != nil {
				return nil, e
			}
			x, e := jc.OperatorInspect(str(p, "id"))
			if e != nil {
				return nil, judgeError(e)
			}
			ts, _ := x["targets"].([]judge.Target)
			usage, e := judgeUsage(c, ts)
			if e != nil {
				return nil, e
			}
			x["usage"] = usage
			if c.Improvements != nil {
				proposals, err := c.Improvements.List(registry.RequestContext(p))
				if err != nil {
					return nil, err
				}
				linked := proposals[:0]
				for _, proposal := range proposals {
					if proposal.JudgeRunID == str(p, "id") {
						linked = append(linked, proposal)
					}
				}
				x["improvements"] = linked
			}
			return x, nil
		},
	}
}
func judgeEvidence() registry.Command {
	return registry.Command{
		Path: "judge.evidence", Summary: "Read one immutable judge evidence item by stable locator", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "run id"}, {Name: "target", Type: registry.String, Required: true, Help: "target id"}, {Name: "artifact", Flag: "artifact", Type: registry.String, Required: true, Help: "evidence artifact"}, {Name: "locator", Flag: "locator", Type: registry.String, Required: true, Help: "stable evidence locator"}}, HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/judges/{id}/targets/{target}/evidence"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			jc, e := requireJudges(c)
			if e != nil {
				return nil, e
			}
			x, e := jc.OperatorEvidence(str(p, "id"), str(p, "target"), judge.EvidenceLocator{Artifact: str(p, "artifact"), Locator: str(p, "locator")})
			if e != nil {
				return nil, judgeError(e)
			}
			return map[string]any{"evidence": x}, nil
		},
	}
}
func judgeCancel() registry.Command {
	return registry.Command{
		Path: "judge.cancel", Summary: "Cancel an LLM-as-Judge run while preserving immutable artifacts", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "run id"}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/judges/{id}/cancel"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			jc, e := requireJudges(c)
			if e != nil {
				return nil, e
			}
			if e = jc.OperatorCancel(str(p, "id")); e != nil {
				return nil, judgeError(e)
			}
			return map[string]any{"id": str(p, "id"), "cancelled": true}, nil
		},
	}
}
func judgeRetry() registry.Command {
	return registry.Command{
		Path: "judge.retry", Summary: "Retry failed assignments in an LLM-as-Judge run", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "run id"}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/judges/{id}/retry"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			jc, e := requireJudges(c)
			if e != nil {
				return nil, e
			}
			if e = jc.OperatorRetry(str(p, "id")); e != nil {
				return nil, judgeError(e)
			}
			return map[string]any{"id": str(p, "id"), "retried": true}, nil
		},
	}
}

func judgeAutomationGet() registry.Command {
	return registry.Command{Path: "judge.automation.get", Summary: "Read the active Judge automation JSON", HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/judge-automation"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireJudgeAutomation(c)
		if err != nil {
			return nil, err
		}
		revision, err := control.Get(registry.RequestContext(p))
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]any{"configured": false}, nil
		}
		return map[string]any{"configured": err == nil, "revision": revision}, err
	}}
}

func judgeAutomationValidate() registry.Command {
	return registry.Command{Path: "judge.automation.validate", Summary: "Validate Judge automation JSON in tariboyd", Args: []registry.Arg{{Name: "config_json", Flag: "json", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/judge-automation/validate"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireJudgeAutomation(c)
		if err != nil {
			return nil, err
		}
		return control.Validate(registry.RequestContext(p), []byte(str(p, "config_json"))), nil
	}}
}

func judgeAutomationApply() registry.Command {
	return registry.Command{Path: "judge.automation.apply", Summary: "Apply Judge automation JSON without starting a review", Args: []registry.Arg{{Name: "config_json", Flag: "json", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "PUT", Path: "/api/judge-automation"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireJudgeAutomation(c)
		if err != nil {
			return nil, err
		}
		validation := control.Validate(registry.RequestContext(p), []byte(str(p, "config_json")))
		if len(validation.Diagnostics) > 0 {
			return nil, api.UserError{Code: "invalid_judge_automation", Msg: "Judge automation configuration is invalid", Data: map[string]any{"diagnostics": validation.Diagnostics}}
		}
		return control.Apply(registry.RequestContext(p), []byte(str(p, "config_json")))
	}}
}

func judgeAutomationRunOnce() registry.Command {
	return registry.Command{Path: "judge.automation.run-once", Summary: "Queue one automatic Judge cycle through the current scheduler", Args: []registry.Arg{{Name: "limit", Flag: "limit", Type: registry.Int, Default: 100}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/judge-automation/run-once"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireJudgeAutomation(c)
		if err != nil {
			return nil, err
		}
		return control.RunOnce(registry.RequestContext(p), intOf(p, "limit", 100))
	}}
}
