package commands

import (
	"errors"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/improvement"
	"github.com/alekzonder/tariboy/internal/registry"
)

func requireImprovements(c *registry.Ctx) (registry.ImprovementControl, error) {
	if c.Improvements == nil {
		return nil, api.UserError{Code: "no_improvement_control", Msg: "improvement control is not available"}
	}
	return c.Improvements, nil
}

func improvementError(err error) error {
	switch {
	case errors.Is(err, improvement.ErrNotFound):
		return api.UserError{Code: "not_found", Msg: err.Error()}
	case errors.Is(err, improvement.ErrRevisionMismatch):
		return api.UserError{Code: "revision_mismatch", Msg: err.Error()}
	case errors.Is(err, improvement.ErrInvalidProposal), errors.Is(err, improvement.ErrInvalidTransition):
		return api.UserError{Code: "invalid_improvement", Msg: err.Error()}
	default:
		return err
	}
}

func improvementLs() registry.Command {
	return registry.Command{Path: "improvement.ls", Summary: "List agent improvement proposals", HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/improvements"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		proposals, err := control.List(registry.RequestContext(p))
		return map[string]any{"proposals": proposals, "count": len(proposals)}, improvementError(err)
	}}
}

func improvementInspect() registry.Command {
	return registry.Command{Path: "improvement.inspect", Summary: "Show an agent improvement proposal", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/improvements/{id}"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		proposal, err := control.Get(registry.RequestContext(p), str(p, "id"))
		return proposal, improvementError(err)
	}}
}

func improvementPlanDecision(decision improvement.ApprovalDecision) registry.Command {
	name := string(decision)
	return registry.Command{Path: "improvement.plan." + name, Summary: name + " an exact improvement plan revision", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}, {Name: "revision", Flag: "revision", Type: registry.String, Required: true}, {Name: "reason", Flag: "reason", Type: registry.String}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/improvements/{id}/plan/" + name}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		approval, err := control.DecidePlan(registry.RequestContext(p), str(p, "id"), str(p, "revision"), c.Operator, decision, str(p, "reason"))
		return approval, improvementError(err)
	}}
}
