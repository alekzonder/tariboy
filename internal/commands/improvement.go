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

func imageReleaseInspect() registry.Command {
	return registry.Command{Path: "image-release.inspect", Summary: "Show immutable image release provenance", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/image-releases/{id}"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		release, err := control.GetRelease(registry.RequestContext(p), str(p, "id"))
		return release, improvementError(err)
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

func imageReleaseRolloutDecision(decision improvement.ApprovalDecision) registry.Command {
	name := string(decision)
	return registry.Command{Path: "image-release.rollout." + name, Summary: name + " an exact image release rollout", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}, {Name: "release-hash", Flag: "release-hash", Type: registry.String, Required: true}, {Name: "reason", Flag: "reason", Type: registry.String}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/image-releases/{id}/rollout/" + name}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		approval, err := control.DecideRollout(registry.RequestContext(p), str(p, "id"), str(p, "release-hash"), c.Operator, decision, str(p, "reason"))
		return approval, improvementError(err)
	}}
}

func imageReleaseRolloutStage() registry.Command {
	return registry.Command{Path: "image-release.rollout.stage", Summary: "Stage an approved release for one agent", Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true}, {Name: "agent", Flag: "agent", Type: registry.String, Required: true}, {Name: "release-hash", Flag: "release-hash", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/image-releases/{id}/rollout/stage"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		rollout, err := control.StageSingleRollout(registry.RequestContext(p), str(p, "id"), str(p, "agent"), str(p, "release-hash"))
		return rollout, improvementError(err)
	}}
}

func imageReleaseRollback() registry.Command {
	return registry.Command{Path: "image-release.rollback", Summary: "Stage the prior immutable image from a completed rollout", Args: []registry.Arg{{Name: "rollout", Flag: "rollout", Type: registry.String, Required: true}}, HTTP: &registry.HTTPRoute{Method: "POST", Path: "/api/image-rollouts/{rollout}/rollback"}, Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
		control, err := requireImprovements(c)
		if err != nil {
			return nil, err
		}
		rollout, err := control.StageRollback(registry.RequestContext(p), str(p, "rollout"))
		return rollout, improvementError(err)
	}}
}
