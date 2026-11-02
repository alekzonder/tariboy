package agentapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekzonder/tariboy/internal/assessment"
)

type assessmentCall struct {
	action string
	body   map[string]any
}

func assessmentServer(plugins []string, calls *[]assessmentCall, err error) *Server {
	return NewServer(Deps{
		Agent: "judge", Plugins: plugins,
		AssessmentAction: func(action string, body map[string]any) (any, error) {
			*calls = append(*calls, assessmentCall{action, body})
			if err != nil {
				return nil, err
			}
			return map[string]any{"action": action}, nil
		},
	})
}

func serve(s *Server, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	return rec
}

func TestAssessmentAddNeedsTheCapability(t *testing.T) {
	var calls []assessmentCall
	without := assessmentServer([]string{"whoami", "loop", "mail"}, &calls, nil)
	rec := serve(without, "POST", "/tools/assessment/add", `{"summary":"s"}`)
	if ok, e := decode(t, rec.Body.Bytes()); ok || rec.Code != http.StatusNotFound || e["code"] != "plugin_disabled" {
		t.Fatalf("add without assessments = %d %s", rec.Code, rec.Body.String())
	}
	if len(calls) != 0 {
		t.Fatalf("a refused add reached the service: %v", calls)
	}
	// Reads are open to any agent.
	if rec := serve(without, "GET", "/tools/assessment/ls?iteration=it-1&limit=5", ""); rec.Code != http.StatusOK {
		t.Fatalf("ls = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(without, "GET", "/tools/assessment/show/as-1", ""); rec.Code != http.StatusOK {
		t.Fatalf("show = %d %s", rec.Code, rec.Body.String())
	}
	if len(calls) != 2 || calls[0].action != "ls" || calls[0].body["iteration"] != "it-1" || calls[0].body["limit"] != 5 ||
		calls[1].action != "show" || calls[1].body["id"] != "as-1" {
		t.Fatalf("read calls = %+v", calls)
	}
}

func TestAssessmentAddScrubsIdentity(t *testing.T) {
	var calls []assessmentCall
	s := assessmentServer([]string{"whoami", "loop", "mail", "assessments"}, &calls, nil)
	rec := serve(s, "POST", "/tools/assessment/add", `{"summary":"s","assessor_agent":"forged","assessor_image_ref":"x",
		"assessor_image_version":"x","assessor_image_digest":"x","subject_agent":"x","image_ref":"x","image_version":"x",
		"image_digest":"x","workflow_digest":"x","workflow_version":"x","id":"as-forged","created_at":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	if len(calls) != 1 || calls[0].action != "add" || len(calls[0].body) != 1 || calls[0].body["summary"] != "s" {
		t.Fatalf("add body = %+v; want only summary", calls)
	}
}

func TestAssessmentErrorsKeepTheirStatusAndCode(t *testing.T) {
	var calls []assessmentCall
	s := assessmentServer([]string{"assessments"}, &calls,
		&assessment.Error{Status: http.StatusBadRequest, Code: "invalid_assessment", Msg: "summary is required"})
	rec := serve(s, "POST", "/tools/assessment/add", `{}`)
	if ok, e := decode(t, rec.Body.Bytes()); ok || rec.Code != http.StatusBadRequest || e["code"] != "invalid_assessment" || e["message"] != "summary is required" {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
}
