package commands

import (
	"net/http"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/assessment"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// assessmentFixture stores two iterations and one assessment of it-dev by
// judge, and returns its id.
func assessmentFixture(t *testing.T) (*registry.Ctx, string) {
	t.Helper()
	s := storetest.Open(t)
	for _, it := range [][]string{{"it-dev", "dev", "developer:latest"}, {"it-judge", "judge", "judge:latest"}} {
		if _, err := s.DB.Exec(`INSERT INTO iterations(id, agent, trigger, status, started_at, image_ref, image_version, image_digest)
			VALUES (?, ?, 'manual', 'running', '2026-11-02T07:00:00.000000000Z', ?, '1.0.0', 'sha')`, it[0], it[1], it[2]); err != nil {
			t.Fatal(err)
		}
	}
	svc := assessment.New(s.DB, steppingClock())
	five, zero := 5, 0
	res, err := svc.Add("judge", "it-judge", assessment.AssessmentInput{
		RubricVersion: "rubric-1", IterationID: "it-dev", Summary: "ok",
		Axes: []assessment.AssessmentAxis{
			{Axis: assessment.AxisImageProcess, Verdict: assessment.VerdictScored, Score: &five},
			{Axis: assessment.AxisRole, Verdict: assessment.VerdictNotApplicable},
			{Axis: assessment.AxisTaskFit, Verdict: assessment.VerdictScored, Score: &zero, Note: "n"},
		},
		Violations: []assessment.AssessmentViolation{{Axis: assessment.AxisTaskFit, Description: "d", Quote: "q",
			Evidence: assessment.AssessmentEvidence{Kind: assessment.EvidenceScriptRun, RunID: "srun-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &registry.Ctx{Store: s, Assessments: svc}, res.Assessment.ID
}

func init() { typedCallSets["assessments"] = assessmentTypedCalls }

func assessmentTypedCalls(t *testing.T) (*registry.Ctx, []typedCall) {
	c, id := assessmentFixture(t)
	return c, []typedCall{
		{"GET", "/api/assessments", "/api/assessments", nil},
		{"GET", "/api/assessments", "/api/assessments?iteration=it-dev&agent=dev&assessor=judge&image=developer:latest&image_version=1.0.0&limit=5", nil},
		{"GET", "/api/assessments/{id}", "/api/assessments/" + id, nil},
	}
}

func TestAssessmentCommandsReadOnly(t *testing.T) {
	c, id := assessmentFixture(t)
	list, err := h(t, "assessment.ls")(c, registry.Params{"task": "", "agent": "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if got := list.(assessment.AssessmentList).Assessments; len(got) != 1 || got[0].ID != id || got[0].ViolationCount != 1 {
		t.Fatalf("ls = %+v", got)
	}
	shown, err := h(t, "assessment.show")(c, registry.Params{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	if a := shown.(assessment.AssessmentShowResult).Assessment; a.ID != id || len(a.Violations) != 1 || len(a.Axes) != 3 {
		t.Fatalf("show = %+v", a)
	}
	_, err = h(t, "assessment.show")(c, registry.Params{"id": "as-none"})
	var status int
	if e, ok := err.(*assessment.Error); ok {
		status = e.Status
	}
	if status != http.StatusNotFound {
		t.Fatalf("show unknown: %v", err)
	}
	// The operator API cannot write an assessment: every assessment command
	// is a GET, and none other than ls and show exists.
	n := 0
	for _, cmd := range BuildRegistry().Commands() {
		if !strings.HasPrefix(cmd.Path, "assessment.") {
			continue
		}
		n++
		if cmd.Path != "assessment.ls" && cmd.Path != "assessment.show" {
			t.Errorf("unexpected operator command %s", cmd.Path)
		}
		if cmd.HTTP == nil || cmd.HTTP.Method != "GET" || !cmd.CLIHidden {
			t.Errorf("%s: HTTP=%+v CLIHidden=%v; want an HTTP-only GET", cmd.Path, cmd.HTTP, cmd.CLIHidden)
		}
	}
	if n != 2 {
		t.Fatalf("%d assessment commands; want ls and show", n)
	}
}
