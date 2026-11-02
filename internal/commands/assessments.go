package commands

import (
	"net/http"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/assessment"
	"github.com/alekzonder/tariboy/internal/registry"
)

// The operator assessment commands are read-only: an assessment is written
// only by an agent through its tools socket. They serve HTTP only:
// `tariboy assessment` is the assessmentcli client, which calls these routes.

func requireAssessments(c *registry.Ctx) (*assessment.Service, error) {
	if c.Assessments == nil {
		return nil, api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable", Msg: "assessments are not available"}
	}
	return c.Assessments, nil
}

func assessmentCommands() []registry.Command {
	return []registry.Command{
		registry.WithResult[assessment.AssessmentList](registry.Command{
			Path: "assessment.ls", Summary: "List assessments, newest first", CLIHidden: true,
			HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/assessments"},
			Args: []registry.Arg{
				{Name: "iteration", Flag: "iteration", Type: registry.String, Help: "assessed iteration id"},
				{Name: "task", Flag: "task", Type: registry.String, Help: "assessed task key"},
				{Name: "agent", Flag: "agent", Type: registry.String, Help: "assessed agent"},
				{Name: "assessor", Flag: "assessor", Type: registry.String, Help: "assessor agent"},
				{Name: "image", Flag: "image", Type: registry.String, Help: "image reference of the assessed iteration"},
				{Name: "image_version", Flag: "image-version", Type: registry.String, Help: "image version of the assessed iteration"},
				{Name: "limit", Flag: "limit", Type: registry.Int, Help: "most assessments to return (default 100, at most 1000)"},
			},
			Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
				svc, err := requireAssessments(c)
				if err != nil {
					return nil, err
				}
				return svc.List(assessment.AssessmentFilter{Iteration: p.Trimmed("iteration"), Task: p.Trimmed("task"),
					Agent: p.Trimmed("agent"), Assessor: p.Trimmed("assessor"), Image: p.Trimmed("image"),
					ImageVersion: p.Trimmed("image_version"), Limit: p.Int("limit")})
			},
		}),
		registry.WithResult[assessment.AssessmentShowResult](registry.Command{
			Path: "assessment.show", Summary: "Show one assessment with its axes and violations", CLIHidden: true,
			HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/assessments/{id}"},
			Args: []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "assessment id"}},
			Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
				svc, err := requireAssessments(c)
				if err != nil {
					return nil, err
				}
				return svc.Get(p.Trimmed("id"))
			},
		}),
	}
}
