package assessment

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// addAs stores an assessment of iteration (or of task when iteration is
// empty) by assessor with rubric and returns its id.
func (f fixture) addAs(t *testing.T, assessor, iteration, task, rubric string) string {
	t.Helper()
	in := validInput()
	in.IterationID, in.TaskKey, in.RubricVersion = iteration, task, rubric
	res, err := f.svc.Add(assessor, "it-"+assessor, in)
	if err != nil {
		t.Fatal(err)
	}
	return res.Assessment.ID
}

func (f fixture) listIDs(t *testing.T, filter AssessmentFilter) []string {
	t.Helper()
	list, err := f.svc.List(filter)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, s := range list.Assessments {
		out = append(out, s.ID)
	}
	return out
}

func wantIDs(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %v; want %v", label, got, want)
	}
}

// sameTime makes every later Add of f record one created_at.
func (f *fixture) sameTime() {
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	f.svc = New(f.st.DB, func() time.Time { return at })
}

func TestListShowsOnlyTheLatestAssessmentOfAnIteration(t *testing.T) {
	f := newFixture(t)
	first := f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	second := f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	wantIDs(t, "latest", f.listIDs(t, AssessmentFilter{}), []string{second})
	wantIDs(t, "history", f.listIDs(t, AssessmentFilter{History: true}), []string{second, first})
}

func TestListShowsOnlyTheLatestAssessmentOfATask(t *testing.T) {
	f := newFixture(t)
	first := f.addAs(t, "judge", "", f.task, "task-review/1")
	second := f.addAs(t, "judge", "", f.task, "task-review/1")
	wantIDs(t, "latest", f.listIDs(t, AssessmentFilter{}), []string{second})
	wantIDs(t, "history", f.listIDs(t, AssessmentFilter{History: true}), []string{second, first})
}

func TestListAtEqualTimeTakesTheLatestRubricVersion(t *testing.T) {
	f := newFixture(t)
	f.sameTime()
	// Each pair: the winner is added first, so seq alone would pick the loser.
	cases := []struct{ iteration, winner, loser string }{
		{"it-dev", "iteration-review/2", "iteration-review/1"},
		{"it-x", "iteration-review/10", "iteration-review/2"},
		{"it-judge", "iteration-review/1", "iteration-review"},
	}
	for _, c := range cases {
		win := f.addAs(t, "judge", c.iteration, "", c.winner)
		f.addAs(t, "judge", c.iteration, "", c.loser)
		wantIDs(t, c.winner+" over "+c.loser, f.listIDs(t, AssessmentFilter{Iteration: c.iteration}), []string{win})
	}
}

func TestListAtEqualTimeAndRubricTakesTheHigherSeq(t *testing.T) {
	f := newFixture(t)
	f.sameTime()
	f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	second := f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	wantIDs(t, "latest", f.listIDs(t, AssessmentFilter{}), []string{second})
}

func TestListFiltersAndLimitsAfterChoosingTheLatest(t *testing.T) {
	f := newFixture(t)
	old := f.addAs(t, "x", "it-dev", "", "iteration-review/1")
	latest := f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	wantIDs(t, "assessor of the replaced record", f.listIDs(t, AssessmentFilter{Assessor: "x"}), []string{})
	wantIDs(t, "assessor with history", f.listIDs(t, AssessmentFilter{Assessor: "x", History: true}), []string{old})
	other := f.addAs(t, "judge", "it-x", "", "iteration-review/1")
	wantIDs(t, "limit", f.listIDs(t, AssessmentFilter{Limit: 2}), []string{other, latest})
}

func TestListKeepsIterationAndTaskAssessmentsApart(t *testing.T) {
	f := newFixture(t)
	ofIteration := f.addAs(t, "judge", "it-dev", f.task, "iteration-review/1")
	ofTask := f.addAs(t, "judge", "", f.task, "task-review/1")
	wantIDs(t, "both objects", f.listIDs(t, AssessmentFilter{Task: f.task}), []string{ofTask, ofIteration})
}

func TestAgentActionLsTakesHistory(t *testing.T) {
	f := newFixture(t)
	f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	for history, want := range map[any]int{nil: 1, false: 1, true: 2} {
		body := map[string]any{}
		if history != nil {
			body["history"] = history
		}
		res, err := f.svc.AgentAction("x", "", "ls", body)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(res.(AssessmentList).Assessments); n != want {
			t.Errorf("history %v: %d assessments; want %d", history, n, want)
		}
	}
}

// TestMeasureQuery runs testdata/measure.sql, the query the operator runs on
// the daemon database, over replaced, tied and single assessments. The query
// must embed IsLatest, the selection metrics and ls use.
func TestMeasureQuery(t *testing.T) {
	raw, err := os.ReadFile("testdata/measure.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), IsLatest) {
		t.Fatalf("testdata/measure.sql does not embed IsLatest:\n%s", IsLatest)
	}
	f := newFixture(t)
	f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	f.addAs(t, "judge", "it-dev", "", "iteration-review/1")
	f.addAs(t, "judge", "", f.task, "task-review/1")
	f.addAs(t, "judge", "", f.task, "task-review/1")
	f.addAs(t, "judge", "", f.task, "task-review/1")
	f.addAs(t, "judge", "it-x", "", "iteration-review/1")
	f.sameTime()
	f.addAs(t, "judge", "it-judge", "", "iteration-review/2")
	f.addAs(t, "judge", "it-judge", "", "iteration-review/1")

	var several, rows, latestRows, overTaken int
	if err := f.st.DB.QueryRow(string(raw)).Scan(&several, &rows, &latestRows, &overTaken); err != nil {
		t.Fatal(err)
	}
	if several != 3 || rows != 7 || latestRows != 3 || overTaken != 0 {
		t.Fatalf("objects with several = %d, their rows = %d, their latest rows = %d, objects with more than one latest = %d; want 3, 7, 3, 0",
			several, rows, latestRows, overTaken)
	}
}
