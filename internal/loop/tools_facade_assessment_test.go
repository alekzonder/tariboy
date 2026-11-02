package loop

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agentapi"
	"github.com/alekzonder/tariboy/internal/assessment"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

type fixedIteration string

func (f fixedIteration) currentIterationID(string) string { return string(f) }

// TestAssessmentDepsWriteAsTheSocketAgent checks that an assessment written
// through the tools socket has this agent as the assessor and its current
// iteration as the assessor's image snapshot.
func TestAssessmentDepsWriteAsTheSocketAgent(t *testing.T) {
	st := storetest.Open(t)
	for _, it := range [][]string{{"it-dev", "dev", "developer:latest"}, {"it-judge", "judge", "judge:latest"}} {
		if _, err := st.DB.Exec(`INSERT INTO iterations(id, agent, trigger, status, started_at, image_ref)
			VALUES (?, ?, 'manual', 'running', '2026-11-02T07:00:00.000000000Z', ?)`, it[0], it[1], it[2]); err != nil {
			t.Fatal(err)
		}
	}
	f := toolsFacade{agent: "judge", cfg: &ManagerConfig{Assessments: assessment.New(st.DB, nil)}, iterations: fixedIteration("it-judge")}
	var d agentapi.Deps
	f.assessmentDeps(&d)
	res, err := d.AssessmentAction("add", map[string]any{
		"rubric_version": "r", "iteration_id": "it-dev", "summary": "s",
		"axes": []any{
			map[string]any{"axis": "image_process", "verdict": "not_applicable"},
			map[string]any{"axis": "role", "verdict": "not_applicable"},
			map[string]any{"axis": "task_fit", "verdict": "not_applicable"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := res.(assessment.AssessmentShowResult).Assessment
	if a.AssessorAgent != "judge" || a.AssessorImageRef != "judge:latest" || a.SubjectAgent != "dev" {
		t.Fatalf("record = %+v", a.AssessmentRecord)
	}
	unwired := toolsFacade{agent: "judge", cfg: &ManagerConfig{}, iterations: fixedIteration("it-judge")}
	unwired.assessmentDeps(&d)
	if _, err := d.AssessmentAction("ls", map[string]any{}); err == nil {
		t.Fatal("ls without a service succeeded")
	}
}
