package assessment

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
	"github.com/alekzonder/tariboy/internal/tasks"
)

type fixture struct {
	st  *store.Store
	svc *Service
	// task is assigned to dev; other is assigned to nobody.
	task, other string
}

func steppingClock() func() time.Time {
	var mu sync.Mutex
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Second)
		return now
	}
}

// newFixture stores iterations it-dev (agent dev, image developer:latest),
// it-judge (agent judge, the assessor) and it-x (agent x), a task assigned to
// dev and an unassigned task.
func newFixture(t *testing.T) fixture {
	t.Helper()
	st := storetest.Open(t)
	for _, it := range [][]string{
		{"it-dev", "dev", "developer:latest", "0.23.1", "sha-dev"},
		{"it-judge", "judge", "judge:latest", "1.0.0", "sha-judge"},
		{"it-x", "x", "other:latest", "2.0.0", "sha-x"},
	} {
		if _, err := st.DB.Exec(`INSERT INTO iterations(id, agent, trigger, status, started_at, image_ref, image_version, image_digest)
			VALUES (?, ?, 'manual', 'running', '2026-11-02T07:00:00.000000000Z', ?, ?, ?)`, it[0], it[1], it[2], it[3], it[4]); err != nil {
			t.Fatal(err)
		}
	}
	svc := tasks.NewService(st.DB, "customer", time.Now)
	ctx := context.Background()
	actor := tasks.CustomerActor("customer")
	if _, err := svc.CreateQueue(ctx, actor, tasks.CreateQueueInput{Prefix: "DEV", Name: "Dev"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, actor, tasks.CreateTaskInput{Queue: "DEV", Title: "assigned", Assignee: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateTask(ctx, actor, tasks.CreateTaskInput{Queue: "DEV", Title: "free"})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{st: st, svc: New(st.DB, steppingClock()), task: task.Key, other: other.Key}
}

func score(n int) *int { return &n }

func validInput() AssessmentInput {
	return AssessmentInput{
		RubricVersion: "rubric-1",
		IterationID:   "it-dev",
		Summary:       "Followed the process.",
		Axes: []AssessmentAxis{
			{Axis: AxisImageProcess, Verdict: VerdictScored, Score: score(80), Note: "ok"},
			{Axis: AxisRole, Verdict: VerdictNotApplicable},
			{Axis: AxisTaskFit, Verdict: VerdictScored, Score: score(0)},
		},
		Violations: []AssessmentViolation{
			{Axis: AxisImageProcess, Description: "skipped tests", Quote: "no tests needed",
				Evidence: AssessmentEvidence{Kind: EvidenceTranscript, IterationID: "it-dev", Locator: "event 12"}},
			{Axis: AxisTaskFit, Description: "refused", Quote: "I will not",
				Evidence: AssessmentEvidence{Kind: EvidenceTaskEvent, TaskKey: "DEV-1", Sequence: 4}},
			{Axis: AxisTaskFit, Description: "bad script", Quote: "exit 1",
				Evidence: AssessmentEvidence{Kind: EvidenceScriptRun, RunID: "srun-1"}},
		},
	}
}

func wantInvalid(t *testing.T, label string, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusBadRequest || e.Code != "invalid_assessment" {
		t.Fatalf("%s: error %v; want 400 invalid_assessment", label, err)
	}
}

func TestAddFillsAssessorAndSnapshots(t *testing.T) {
	f := newFixture(t)
	in := validInput()
	in.TaskKey = f.task
	got, err := f.svc.Add("judge", "it-judge", in)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Assessment
	if !strings.HasPrefix(a.ID, "as-20261102T") || !strings.HasSuffix(a.ID, "-000001") {
		t.Fatalf("id = %q", a.ID)
	}
	if a.AssessorAgent != "judge" || a.AssessorImageRef != "judge:latest" || a.AssessorImageVersion != "1.0.0" || a.AssessorImageDigest != "sha-judge" {
		t.Fatalf("assessor = %+v", a.AssessmentRecord)
	}
	if a.SubjectAgent != "dev" || a.ImageRef != "developer:latest" || a.ImageVersion != "0.23.1" || a.ImageDigest != "sha-dev" {
		t.Fatalf("subject snapshot = %+v", a.AssessmentRecord)
	}
	if a.TaskKey != f.task || a.WorkflowDigest != "" || a.WorkflowVersion != "" {
		t.Fatalf("task snapshot = %+v", a.AssessmentRecord)
	}
	if len(a.Axes) != 3 || a.Axes[0].Axis != AxisImageProcess || *a.Axes[0].Score != 80 || a.Axes[1].Score != nil || *a.Axes[2].Score != 0 {
		t.Fatalf("axes = %+v", a.Axes)
	}
	if !reflect.DeepEqual(a.Violations, in.Violations) {
		t.Fatalf("violations = %+v; want %+v", a.Violations, in.Violations)
	}
	shown, err := f.svc.Get(a.ID)
	if err != nil || !reflect.DeepEqual(shown, got) {
		t.Fatalf("Get = %+v, %v; want %+v", shown, err, got)
	}
}

func TestAddSnapshotsTheWorkflow(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.DB.Exec(`INSERT INTO task_workflow_images(digest, name, version, manifest, built_at)
		VALUES ('wf-digest', 'development', '0.10.1', '{}', '2026-11-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.DB.Exec(`UPDATE tasks SET workflow_digest = 'wf-digest' WHERE task_key = ?`, f.other); err != nil {
		t.Fatal(err)
	}
	in := validInput()
	in.IterationID, in.TaskKey = "", f.other
	got, err := f.svc.Add("judge", "it-judge", in)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Assessment
	if a.WorkflowDigest != "wf-digest" || a.WorkflowVersion != "0.10.1" || a.SubjectAgent != "" || a.IterationID != "" {
		t.Fatalf("snapshot = %+v", a.AssessmentRecord)
	}
}

func TestAddLinksIterationAndTask(t *testing.T) {
	f := newFixture(t)
	in := validInput()
	in.TaskKey = f.other
	_, err := f.svc.Add("judge", "it-judge", in)
	wantInvalid(t, "unrelated task", err)

	// An event the iteration's agent wrote on the task links them, even after
	// the task left the agent.
	if _, err := f.st.DB.Exec(`INSERT INTO task_events(event_id, task_id, queue_prefix, kind, actor, created_at)
		SELECT 'te-x', id, queue_prefix, 'comment', 'agent:dev', '2026-11-02T07:30:00.000000000Z' FROM tasks WHERE task_key = ?`, f.other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Add("judge", "it-judge", in); err != nil {
		t.Fatalf("task with an event of the agent: %v", err)
	}
	in.IterationID = "it-x"
	_, err = f.svc.Add("judge", "it-judge", in)
	wantInvalid(t, "event of another agent", err)
}

func TestAddRejectsInvalidInput(t *testing.T) {
	f := newFixture(t)
	cases := map[string]func(*AssessmentInput){
		"no subject":         func(in *AssessmentInput) { in.IterationID, in.TaskKey = "", "" },
		"unknown iteration":  func(in *AssessmentInput) { in.IterationID = "it-none" },
		"unknown task":       func(in *AssessmentInput) { in.IterationID, in.TaskKey = "", "DEV-none" },
		"no rubric":          func(in *AssessmentInput) { in.RubricVersion = " " },
		"no summary":         func(in *AssessmentInput) { in.Summary = "" },
		"two axes":           func(in *AssessmentInput) { in.Axes = in.Axes[:2] },
		"four axes":          func(in *AssessmentInput) { in.Axes = append(in.Axes, in.Axes[0]) },
		"repeated axis":      func(in *AssessmentInput) { in.Axes[2].Axis = AxisRole },
		"unknown axis":       func(in *AssessmentInput) { in.Axes[2].Axis = "speed" },
		"unknown verdict":    func(in *AssessmentInput) { in.Axes[0].Verdict = "good" },
		"scored no score":    func(in *AssessmentInput) { in.Axes[0].Score = nil },
		"score above 100":    func(in *AssessmentInput) { in.Axes[0].Score = score(101) },
		"score below 0":      func(in *AssessmentInput) { in.Axes[0].Score = score(-1) },
		"n/a with score":     func(in *AssessmentInput) { in.Axes[1].Score = score(50) },
		"no description":     func(in *AssessmentInput) { in.Violations[0].Description = "" },
		"no quote":           func(in *AssessmentInput) { in.Violations[0].Quote = "" },
		"violation axis":     func(in *AssessmentInput) { in.Violations[0].Axis = "speed" },
		"evidence kind":      func(in *AssessmentInput) { in.Violations[0].Evidence.Kind = "mail" },
		"transcript no loc":  func(in *AssessmentInput) { in.Violations[0].Evidence.Locator = "" },
		"transcript no iter": func(in *AssessmentInput) { in.Violations[0].Evidence.IterationID = "" },
		"event no task":      func(in *AssessmentInput) { in.Violations[1].Evidence.TaskKey = "" },
		"event no sequence":  func(in *AssessmentInput) { in.Violations[1].Evidence.Sequence = 0 },
		"run no id":          func(in *AssessmentInput) { in.Violations[2].Evidence.RunID = "" },
		"run bad id":         func(in *AssessmentInput) { in.Violations[2].Evidence.RunID = "run-1" },
	}
	for name, mutate := range cases {
		in := validInput()
		mutate(&in)
		_, err := f.svc.Add("judge", "it-judge", in)
		wantInvalid(t, name, err)
	}
	var n int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM assessments`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rejected input wrote %d assessments (%v)", n, err)
	}
}

func TestAddNeedsARunningIteration(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Add("judge", "", validInput())
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusConflict || e.Code != "no_iteration" {
		t.Fatalf("error %v; want 409 no_iteration", err)
	}
}

func TestReassessmentAddsARecordAndKeepsTheFirst(t *testing.T) {
	f := newFixture(t)
	first, err := f.svc.Add("judge", "it-judge", validInput())
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.svc.Get(first.Assessment.ID)
	if err != nil {
		t.Fatal(err)
	}
	again := validInput()
	again.Summary = "Second look."
	again.Axes[0].Score = score(10)
	again.Violations = nil
	second, err := f.svc.Add("judge", "it-judge", again)
	if err != nil {
		t.Fatal(err)
	}
	if second.Assessment.ID == first.Assessment.ID {
		t.Fatal("a reassessment reused the id")
	}
	if second.Assessment.Violations == nil || len(second.Assessment.Violations) != 0 {
		t.Fatalf("violations = %#v; want an empty list", second.Assessment.Violations)
	}
	after, err := f.svc.Get(first.Assessment.ID)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("first assessment changed: %+v -> %+v (%v)", before, after, err)
	}
}

func TestAddWritesNothingWhenAViolationFails(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.DB.Exec(`CREATE TRIGGER fail_violation BEFORE INSERT ON assessment_violations
		BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Add("judge", "it-judge", validInput()); err == nil {
		t.Fatal("Add succeeded with a failing violation insert")
	}
	for _, table := range []string{"assessments", "assessment_axes", "assessment_violations"} {
		var n int
		if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s has %d rows after a failed write (%v)", table, n, err)
		}
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Get("as-none")
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusNotFound || e.Code != "not_found" {
		t.Fatalf("error %v; want 404 not_found", err)
	}
}

func TestListFilters(t *testing.T) {
	f := newFixture(t)
	add := func(assessor, iteration, task string) string {
		t.Helper()
		in := validInput()
		in.IterationID, in.TaskKey = iteration, task
		res, err := f.svc.Add(assessor, "it-"+assessor, in)
		if err != nil {
			t.Fatal(err)
		}
		return res.Assessment.ID
	}
	a1 := add("judge", "it-dev", f.task)
	a2 := add("judge", "it-x", "")
	a3 := add("x", "it-dev", "")
	ids := func(filter AssessmentFilter) []string {
		t.Helper()
		list, err := f.svc.List(filter)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, s := range list.Assessments {
			if s.ViolationCount != 3 || len(s.Axes) != 3 {
				t.Fatalf("summary %s = %+v", s.ID, s)
			}
			out = append(out, s.ID)
		}
		return out
	}
	for name, tc := range map[string]struct {
		filter AssessmentFilter
		want   []string
	}{
		"all newest first": {AssessmentFilter{}, []string{a3, a2, a1}},
		"iteration":        {AssessmentFilter{Iteration: "it-dev"}, []string{a3, a1}},
		"task":             {AssessmentFilter{Task: f.task}, []string{a1}},
		"agent":            {AssessmentFilter{Agent: "x"}, []string{a2}},
		"assessor":         {AssessmentFilter{Assessor: "x"}, []string{a3}},
		"image":            {AssessmentFilter{Image: "developer:latest"}, []string{a3, a1}},
		"image version":    {AssessmentFilter{Image: "developer:latest", ImageVersion: "2.0.0"}, []string{}},
		"version only":     {AssessmentFilter{ImageVersion: "2.0.0"}, []string{a2}},
		"limit":            {AssessmentFilter{Limit: 2}, []string{a3, a2}},
	} {
		if got := ids(tc.filter); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v; want %v", name, got, tc.want)
		}
	}
	_, err := f.svc.List(AssessmentFilter{Limit: 1001})
	wantInvalid(t, "limit above 1000", err)
}

func TestAgentActionIgnoresIdentityInTheBody(t *testing.T) {
	f := newFixture(t)
	body := map[string]any{
		"rubric_version": "rubric-1", "iteration_id": "it-dev", "summary": "s",
		"assessor_agent": "forged", "assessor_image_ref": "forged:latest", "subject_agent": "forged", "image_ref": "forged",
		"axes": []any{
			map[string]any{"axis": "image_process", "verdict": "scored", "score": 1},
			map[string]any{"axis": "role", "verdict": "scored", "score": 2},
			map[string]any{"axis": "task_fit", "verdict": "not_applicable"},
		},
	}
	res, err := f.svc.AgentAction("judge", "it-judge", "add", body)
	if err != nil {
		t.Fatal(err)
	}
	a := res.(AssessmentShowResult).Assessment
	if a.AssessorAgent != "judge" || a.AssessorImageRef != "judge:latest" || a.SubjectAgent != "dev" || a.ImageRef != "developer:latest" {
		t.Fatalf("record = %+v", a.AssessmentRecord)
	}
	shown, err := f.svc.AgentAction("x", "", "show", map[string]any{"id": a.ID})
	if err != nil || shown.(AssessmentShowResult).Assessment.ID != a.ID {
		t.Fatalf("show = %+v, %v", shown, err)
	}
	list, err := f.svc.AgentAction("x", "", "ls", map[string]any{"assessor": "judge", "limit": 5})
	if err != nil || len(list.(AssessmentList).Assessments) != 1 {
		t.Fatalf("ls = %+v, %v", list, err)
	}
	if _, err := f.svc.AgentAction("x", "", "add", map[string]any{"axes": "nope"}); err == nil {
		t.Fatal("a malformed body was accepted")
	}
}

// TestAssessmentOutlivesItsIterationAndAgents removes the assessed and the
// assessing agents with every row agent removal deletes, iterations included,
// and reads the assessment unchanged.
func TestAssessmentOutlivesItsIterationAndAgents(t *testing.T) {
	f := newFixture(t)
	in := validInput()
	in.TaskKey = f.task
	added, err := f.svc.Add("judge", "it-judge", in)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dev", "judge"} {
		tx, err := f.st.DB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		for _, phase := range []store.Phase{store.PhaseCore, store.PhaseData} {
			if _, err := store.DeleteAgentRows(tx, name, phase); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := f.st.DB.QueryRow(`SELECT COUNT(*) FROM iterations WHERE id IN ('it-dev', 'it-judge')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("iterations left: %d (%v)", n, err)
	}
	got, err := f.svc.Get(added.Assessment.ID)
	if err != nil || !reflect.DeepEqual(got, added) {
		t.Fatalf("after removal = %+v, %v; want %+v", got, err, added)
	}
}
