package agentapi

import (
	"net/http"
	"strconv"

	"github.com/alekzonder/tariboy/internal/api"
)

// assessmentIdentityFields are the assessment fields the daemon writes. The
// body of an add never sets them: the assessor is this socket's agent and
// the snapshots come from the iteration and task rows.
var assessmentIdentityFields = []string{
	"id", "created_at", "assessor_agent", "assessor_image_ref", "assessor_image_version", "assessor_image_digest",
	"subject_agent", "image_ref", "image_version", "image_digest", "workflow_digest", "workflow_version",
}

// assessmentFilterFields are the query parameters of GET /tools/assessment/ls.
var assessmentFilterFields = []string{"iteration", "task", "agent", "assessor", "image", "image_version"}

func (s *Server) assessmentRun(w http.ResponseWriter, r *http.Request, action string, body map[string]any) {
	if s.d.AssessmentAction == nil {
		api.WriteErr(w, http.StatusServiceUnavailable, "unavailable", "assessments are unavailable")
		return
	}
	result, err := s.d.AssessmentAction(action, body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	api.WriteOK(w, result)
}

func (s *Server) assessmentAdd(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{}
	if !api.DecodeJSONBody(w, r, maxBodyBytes, &body) {
		return
	}
	for _, key := range assessmentIdentityFields {
		delete(body, key)
	}
	s.assessmentRun(w, r, "add", body)
}

func (s *Server) assessmentLs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	body := map[string]any{}
	for _, key := range assessmentFilterFields {
		if v := q.Get(key); v != "" {
			body[key] = v
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			api.WriteErr(w, http.StatusBadRequest, "invalid_assessment", "limit must be a number")
			return
		}
		body["limit"] = n
	}
	if v := q.Get("history"); v != "" {
		history, err := strconv.ParseBool(v)
		if err != nil {
			api.WriteErr(w, http.StatusBadRequest, "invalid_assessment", "history must be true or false")
			return
		}
		body["history"] = history
	}
	s.assessmentRun(w, r, "ls", body)
}

func (s *Server) assessmentShow(w http.ResponseWriter, r *http.Request) {
	s.assessmentRun(w, r, "show", map[string]any{"id": r.PathValue("id")})
}
