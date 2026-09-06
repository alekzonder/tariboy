package judge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	basestore "github.com/alekzonder/tariboy/internal/store"
)

func readyRun(t *testing.T) (*Store, Run, Target) {
	t.Helper()
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedJudgeAgent(t, db.DB, "judge-2")
	seedTarget(t, db.DB, "target", "worker", "done", "2026-07-01T10:00:00Z")
	r := request("target")
	r.JudgeAgents = []string{"judge", "judge-2"}
	r.JudgesPerIteration = 2
	r.MaxAttempts = 2
	run, targets, err := js.CreateRun(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`UPDATE judge_targets SET snapshot_status='ready',bundle_hash='bundle' WHERE id=?`, targets[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := js.CreateAssignments(run.ID); err != nil {
		t.Fatal(err)
	}
	return js, run, targets[0]
}

func validAnalysis() AnalysisResult {
	return AnalysisResult{SchemaVersion: 1, Verdict: "pass", Score: .8, Confidence: .7, Summary: "evidence supports it", Strengths: []Strength{{Description: "observed verification", Citations: []Citation{{BundleHash: "bundle", Artifact: "audit", Locator: "line:1"}}}}}
}

func TestClaimExclusivityAndDistinctJudges(t *testing.T) {
	js, run, _ := readyRun(t)
	a, ok, err := js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge", Iteration: "i-1"})
	if err != nil || !ok {
		t.Fatalf("first claim=%+v %v %v", a, ok, err)
	}
	if _, ok, err := js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge", Iteration: "i-1"}); err != nil || ok {
		t.Fatalf("iteration held more than one: ok=%v err=%v", ok, err)
	}
	if _, err := js.SubmitAnalysis(SubmitAnalysisRequest{AssignmentID: a.ID, Agent: "judge", Iteration: "i-1", Result: validAnalysis(), Resolve: CitationResolverFunc(func(Citation) error { return nil })}); err != nil {
		t.Fatal(err)
	}
	b, ok, err := js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge", Iteration: "i-2"})
	if err != nil || ok || b.ID != "" {
		t.Fatalf("same identity received second replica: %+v %v %v", b, ok, err)
	}
	b, ok, err = js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge-2", Iteration: "i-2"})
	if err != nil || !ok {
		t.Fatalf("distinct identity claim=%+v %v %v", b, ok, err)
	}
}

func TestSubmitPreservesInvalidAttemptAndSummaryVersions(t *testing.T) {
	js, run, _ := readyRun(t)
	a, ok, err := js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge", Iteration: "i-1"})
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = js.SubmitAnalysis(SubmitAnalysisRequest{AssignmentID: a.ID, Agent: "judge", Iteration: "i-1", RawSubmission: `{bad`, Result: AnalysisResult{}}); !errors.Is(err, ErrInvalidSubmission) {
		t.Fatalf("invalid=%v", err)
	}
	var raw string
	if err := js.db.QueryRow(`SELECT raw_json FROM judge_submission_attempts WHERE assignment_id=?`, a.ID).Scan(&raw); err != nil || raw != `{bad` {
		t.Fatalf("attempt raw=%q err=%v", raw, err)
	}
	if _, err = js.SubmitAnalysis(SubmitAnalysisRequest{AssignmentID: a.ID, Agent: "judge", Iteration: "i-1", Result: validAnalysis(), Resolve: CitationResolverFunc(func(Citation) error { return nil })}); err != nil {
		t.Fatal(err)
	}
	b, ok, err := js.Claim(ClaimRequest{RunID: run.ID, Agent: "judge-2", Iteration: "i-2"})
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = js.SubmitAnalysis(SubmitAnalysisRequest{AssignmentID: b.ID, Agent: "judge-2", Iteration: "i-2", Result: validAnalysis(), Resolve: CitationResolverFunc(func(Citation) error { return nil })}); err != nil {
		t.Fatal(err)
	}
	if _, err = js.ClaimSummary(run.ID, "lead", "sum-1"); err != nil {
		t.Fatal(err)
	}
	s := SummaryResult{SchemaVersion: 1, ExecutiveConclusion: "good", Coverage: map[string]int{"complete": 2}}
	first, err := js.SubmitSummary(SubmitSummaryRequest{RunID: run.ID, Agent: "lead", Iteration: "sum-1", Result: s})
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 {
		t.Fatalf("version=%d", first.Version)
	}
}

func newJudgeStore(t *testing.T) (*basestore.Store, *Store) {
	t.Helper()
	db, err := basestore.Open(filepath.Join(t.TempDir(), "judge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, NewStore(db, func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) })
}

func seedJudgeAgent(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO agents(name,image_ref,"group",plugins) VALUES(?,?,?,?)`, name, "test", "judges", `["llm-as-judge"]`)
	if err != nil {
		t.Fatal(err)
	}
}

func seedTarget(t *testing.T, db *sql.DB, id, agent, status, started string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO agents(name,image_ref) VALUES(?,?) ON CONFLICT(name) DO NOTHING`, agent, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO iterations(id,agent,trigger,status,started_at) VALUES(?,?,?,?,?)`, id, agent, "manual", status, started)
	if err != nil {
		t.Fatal(err)
	}
}

func request(ids ...string) CreateRunRequest {
	return CreateRunRequest{OriginalRequest: "verify", Selector: Selector{ExplicitIDs: ids}, JudgeGroup: "judges", LeadAgent: "lead", SummaryAgent: "lead", JudgeAgents: []string{"judge"}, JudgesPerIteration: 1}
}

func TestMigrationCreatesJudgeTables(t *testing.T) {
	db, _ := newJudgeStore(t)
	for _, table := range []string{"judge_runs", "judge_targets", "judge_assignments", "judge_submission_attempts", "judge_analyses", "judge_summaries", "judge_retention_pins"} {
		var got string
		if err := db.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&got); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
	}
}

func TestLegacyRunImageIdentityMigrationPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := basestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	js := NewStore(db, nil)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "historical-target", "worker", "done", "2026-07-01T10:00:00Z")
	run, targets, err := js.CreateRun(context.Background(), request("historical-target"))
	if err != nil {
		t.Fatal(err)
	}
	// Reconstitute the pre-identity schema with actual run/target/subject rows.
	if _, err := db.DB.Exec(`ALTER TABLE judge_runs DROP COLUMN judge_images_json; DELETE FROM schema_migrations WHERE name='0039_judge_image_identity.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = basestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	js = NewStore(db, nil)
	got, err := js.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.JudgeImages) != 0 || got.OriginalRequest != run.OriginalRequest {
		t.Fatalf("legacy run rewritten: %+v", got)
	}
	gotTargets, err := js.ListTargets(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotTargets, targets) {
		t.Fatalf("legacy targets changed: %+v", gotTargets)
	}
	subjects, err := js.ListSubjects(run.ID)
	if err != nil || len(subjects) != 1 {
		t.Fatalf("legacy subjects lost: %+v %v", subjects, err)
	}
}

func TestCreateRunFreezesOrderedTargets(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "worker-1", "worker", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "worker-2", "worker", "done", "2026-07-02T10:00:00Z")
	run, targets, err := js.CreateRun(context.Background(), request("worker-2", "worker-1"))
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunSnapshotting {
		t.Fatalf("status=%s", run.Status)
	}
	got := []string{targets[0].Iteration, targets[1].Iteration}
	if want := []string{"worker-2", "worker-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets=%v, want %v", got, want)
	}
	seedTarget(t, db.DB, "worker-3", "worker", "done", "2026-07-03T10:00:00Z")
	frozen, err := js.ListTargets(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 2 {
		t.Fatalf("immutable snapshot grew: %+v", frozen)
	}
}

func TestIterationJudgeReviewsBatchOrdersAndCountsValidAssignments(t *testing.T) {
	db, js := newJudgeStore(t)
	seedTarget(t, db.DB, "iteration-1", "worker", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "other-iteration", "other", "done", "2026-07-01T10:00:00Z")

	for _, row := range []struct {
		run, target, iteration, created, state, verdict string
		score                                           any
	}{
		{"run-zero", "target-zero", "iteration-1", "2026-07-01T11:00:00Z", "terminal", "fail", 0.0},
		{"run-complete", "target-complete", "iteration-1", "2026-07-01T12:00:00Z", "terminal", "pass", 0.8},
		{"run-pending", "target-pending", "iteration-1", "2026-07-01T12:00:00Z", "running", "", nil},
		{"run-other", "target-other", "other-iteration", "2026-07-01T14:00:00Z", "terminal", "pass", 1.0},
	} {
		if _, err := db.DB.Exec(`INSERT INTO judge_runs(id,created_at,updated_at,original_request,spec_json,judge_group,lead_agent,judge_agents_json,summary_agent,judges_per_iteration,status) VALUES(?,?,?,'review','{}','judges','lead','["judge"]','lead',1,'running')`, row.run, row.created, row.created); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DB.Exec(`INSERT INTO judge_targets(id,run_id,target_iteration,target_agent,sequence,snapshot_status,target_state,consensus_verdict,consensus_score) VALUES(?,?,?,?,0,'ready',?,?,?)`, row.target, row.run, row.iteration, map[string]string{"iteration-1": "worker", "other-iteration": "other"}[row.iteration], row.state, row.verdict, row.score); err != nil {
			t.Fatal(err)
		}
		assignmentState := "pending"
		analysisID := ""
		if row.state == "terminal" {
			assignmentState, analysisID = "completed", "analysis-"+row.run
		}
		if _, err := db.DB.Exec(`INSERT INTO judge_assignments(id,run_id,target_id,replica_index,state,analysis_id) VALUES(?,?,?,0,?,?)`, "assignment-"+row.run, row.run, row.target, assignmentState, analysisID); err != nil {
			t.Fatal(err)
		}
		if analysisID != "" {
			if _, err := db.DB.Exec(`INSERT INTO judge_analyses(id,run_id,target_id,assignment_id,judge_agent,judge_iteration,schema_version,result_json,raw_submission,created_at) VALUES(?,?,?,?, 'judge','judge-it',1,'{}','{}',?)`, analysisID, row.run, row.target, "assignment-"+row.run, row.created); err != nil {
				t.Fatal(err)
			}
		}
	}

	got, err := js.ListIterationJudgeReviews([]string{"iteration-1", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	reviews := got["iteration-1"]
	if len(reviews) != 3 || reviews[0].RunID != "run-pending" || reviews[1].RunID != "run-complete" || reviews[2].RunID != "run-zero" {
		t.Fatalf("reviews = %+v", reviews)
	}
	if reviews[0].Pending != 1 || reviews[0].Completed != 0 || reviews[1].Completed != 1 || reviews[1].Score == nil || *reviews[1].Score != 0.8 || reviews[2].Score == nil || *reviews[2].Score != 0 {
		t.Fatalf("review counts/scores = %+v", reviews)
	}
	if _, ok := got["other-iteration"]; ok {
		t.Fatalf("batch leaked another iteration: %+v", got)
	}
	if len(got["missing"]) != 0 {
		t.Fatalf("missing reviews = %+v", got["missing"])
	}
}

func TestIterationJudgeReviewsAcceptsUnboundedIterationHistory(t *testing.T) {
	js, _, _ := readyRun(t)
	ids := make([]string, 40000)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing-%d", i)
	}
	ids[len(ids)-1] = "target"

	got, err := js.ListIterationJudgeReviews(ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["target"]) != 1 {
		t.Fatalf("target reviews = %+v", got["target"])
	}
}

func TestCreateRunGroupsTargetsIntoTaskSubjects(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "manager-it", "manager", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "developer-it", "developer", "done", "2026-07-01T10:01:00Z")
	seedTarget(t, db.DB, "reviewer-it", "reviewer", "done", "2026-07-01T10:02:00Z")
	for _, row := range []struct {
		iteration string
		ref       string
		digest    string
	}{
		{iteration: "manager-it", ref: "manager:v3", digest: "sha256:manager"},
		{iteration: "developer-it", ref: "developer:v5", digest: "sha256:developer"},
		{iteration: "reviewer-it", ref: "reviewer:v7", digest: "sha256:reviewer"},
	} {
		if _, err := db.DB.Exec(`UPDATE iterations SET image_ref=?,image_digest=?,prompt_template_sha256='sha256:prompt' WHERE id=?`, row.ref, row.digest, row.iteration); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB.Exec(`INSERT INTO task_queues(prefix,name,created_at,updated_at) VALUES('TARI','Tariboy','2026-07-01T09:00:00Z','2026-07-01T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	for _, task := range []struct {
		key      string
		position int
		status   string
	}{
		{key: "TARI-42", position: 1, status: "done"},
		{key: "TARI-43", position: 2, status: "cancelled"},
	} {
		if _, err := db.DB.Exec(`INSERT INTO tasks(task_key,queue_prefix,position,title,status,author,customer,group_name,created_at,updated_at,completed_at) VALUES(?,?,?,?,?,'user:operator','user:operator','dev-team','2026-07-01T09:00:00Z','2026-07-01T11:00:00Z','2026-07-01T11:00:00Z')`, task.key, "TARI", task.position, task.key, task.status); err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range []struct {
		iteration string
		agent     string
		task      string
	}{
		{iteration: "manager-it", agent: "manager", task: "TARI-42"},
		{iteration: "developer-it", agent: "developer", task: "TARI-42"},
		{iteration: "reviewer-it", agent: "reviewer", task: "TARI-43"},
	} {
		if _, err := db.DB.Exec(`INSERT INTO ai_requests(id,ts,agent,iteration,task_id) VALUES(?,?,?,?,?)`, fmt.Sprintf("req-%d", i), "2026-07-01T10:30:00Z", item.agent, item.iteration, item.task); err != nil {
			t.Fatal(err)
		}
	}

	run, targets, err := js.CreateRun(context.Background(), request("manager-it", "developer-it", "reviewer-it"))
	if err != nil {
		t.Fatal(err)
	}
	subjects, err := js.ListSubjects(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 2 {
		t.Fatalf("subjects = %+v, want two task subjects", subjects)
	}
	if subjects[0].Type != "task" || subjects[0].ExternalID != "TARI-42" || subjects[0].Snapshot.Status != "done" || subjects[0].Snapshot.Group != "dev-team" || len(subjects[0].Snapshot.Participants) != 2 {
		t.Fatalf("first subject = %+v", subjects[0])
	}
	if subjects[0].Snapshot.Participants[1].ImageDigest != "sha256:developer" {
		t.Fatalf("participants = %+v", subjects[0].Snapshot.Participants)
	}
	if targets[0].SubjectID == "" || targets[0].SubjectID != targets[1].SubjectID || targets[2].SubjectID == targets[0].SubjectID {
		t.Fatalf("target subject ids = %q %q %q", targets[0].SubjectID, targets[1].SubjectID, targets[2].SubjectID)
	}
}

func TestSelectorDeduplicatesExplicitAndFilter(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	seedTarget(t, db.DB, "one", "worker", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "two", "worker", "done", "2026-07-02T10:00:00Z")
	r := request("two")
	r.Selector.Agents = []string{"worker"}
	r.Selector.Order = "oldest"
	_, targets, err := js.CreateRun(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{targets[0].Iteration, targets[1].Iteration}
	if want := []string{"two", "one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targets=%v want=%v", got, want)
	}
}

func TestSelectorFiltersImageRefsAndPreviouslyJudgedBeforeLimit(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	for _, row := range []struct {
		id, image, started string
	}{
		{"old-image", "developer:0.5", "2026-07-01T10:00:00Z"},
		{"already-judged", "developer:0.6", "2026-07-02T10:00:00Z"},
		{"eligible", "developer:0.6", "2026-07-03T10:00:00Z"},
	} {
		seedTarget(t, db.DB, row.id, "worker", "done", row.started)
		if _, err := db.DB.Exec(`UPDATE iterations SET image_ref=? WHERE id=?`, row.image, row.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := js.CreateRun(context.Background(), request("already-judged")); err != nil {
		t.Fatal(err)
	}

	r := request()
	r.Selector = Selector{
		Agents:          []string{"worker"},
		ImageRefs:       []string{"developer:0.6"},
		Statuses:        []string{"done"},
		OnlyUnprocessed: true,
		Order:           "oldest",
		Limit:           1,
	}
	_, targets, err := js.CreateRun(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Iteration != "eligible" {
		t.Fatalf("targets=%+v, want eligible", targets)
	}
}

func TestSelectorRejectsEmptyAndRunning(t *testing.T) {
	db, js := newJudgeStore(t)
	seedJudgeAgent(t, db.DB, "lead")
	seedJudgeAgent(t, db.DB, "judge")
	if _, _, err := js.CreateRun(context.Background(), request()); err != ErrEmptySelection {
		t.Fatalf("empty err=%v", err)
	}
	seedTarget(t, db.DB, "running", "worker", "running", "2026-07-01T10:00:00Z")
	if _, _, err := js.CreateRun(context.Background(), request("running")); err == nil || !isNonTerminal(err) {
		t.Fatalf("running err=%v", err)
	}
}

func isNonTerminal(err error) bool {
	for err != nil {
		if err == ErrNonTerminalIteration {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
