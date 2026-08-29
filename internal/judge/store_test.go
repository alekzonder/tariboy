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
	return AnalysisResult{SchemaVersion: 1, Verdict: "pass", Score: .8, Confidence: .7, Summary: "evidence supports it"}
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
