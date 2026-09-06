package judge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/improvement"
)

type recordingImprovements struct {
	request improvement.CreateProposalRequest
}

func (r *recordingImprovements) CreateProposal(_ context.Context, request improvement.CreateProposalRequest) (improvement.Proposal, error) {
	r.request = request
	return improvement.Proposal{ID: "proposal-1", JudgeRunID: request.JudgeRunID, RevisionHash: "sha256:proposal", Status: improvement.StatusAwaitingPlanApproval, Draft: request.Draft}, nil
}

func serviceFixture(t *testing.T) (*Service, *Store, Run, string) {
	t.Helper()
	db, js := newJudgeStore(t)
	for _, name := range []string{"lead", "judge", "other"} {
		seedJudgeAgent(t, db.DB, name)
	}
	if _, err := db.DB.Exec(`UPDATE agents SET "group"='judges' WHERE name IN ('lead','judge')`); err != nil {
		t.Fatal(err)
	}
	gs := groups.NewStore(db, time.Now)
	if err := gs.Upsert(groups.Group{Name: "judges", Lead: "lead"}); err != nil {
		t.Fatal(err)
	}
	seedTarget(t, db.DB, "target", "target-agent", "done", "2026-07-01T10:00:00Z")
	for _, x := range [][2]string{{"lead", "lead-it"}, {"judge", "judge-it"}, {"other", "other-it"}} {
		if _, err := db.DB.Exec(`INSERT INTO iterations(id,agent,trigger,status,started_at) VALUES(?,?,?,?,?)`, x[1], x[0], "manual", "running", "2026-07-02T10:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	r, ts, err := js.CreateRun(context.Background(), request("target"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`UPDATE judge_targets SET snapshot_status='ready',bundle_hash='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE id=?`, ts[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = js.CreateAssignments(r.ID); err != nil {
		t.Fatal(err)
	}
	return NewService(ServiceConfig{Store: js, Agents: agent.NewStore(db), Groups: gs}), js, r, ts[0].ID
}

func TestServiceAuthorizationAndServerIdentity(t *testing.T) {
	s, _, r, _ := serviceFixture(t)
	if _, err := s.AgentAction(context.Background(), "other", "other-it", "work.claim", map[string]any{"run_id": r.ID}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unrelated claim error=%v", err)
	}
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "run.cancel", map[string]any{"run_id": r.ID}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("worker cancel error=%v", err)
	}
	got, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": r.ID, "agent": "lead", "iteration": "forged"})
	if err != nil {
		t.Fatal(err)
	}
	if criteria, ok := got["criteria"].(string); !ok || criteria == "" || criteria != r.OriginalRequest {
		t.Fatalf("claim criteria=%#v, want %q", got["criteria"], r.OriginalRequest)
	}
	a := got["assignment"].(Assignment)
	if a.JudgeAgent != "judge" || a.JudgeIteration != "judge-it" {
		t.Fatalf("body forged identity: %+v", a)
	}
	if _, err = s.AgentAction(context.Background(), "judge", "missing", "work.claim", map[string]any{"run_id": r.ID}); !errors.Is(err, ErrStaleIteration) {
		t.Fatalf("stale error=%v", err)
	}
}

func TestServiceWorkClaimWithoutAssignmentDoesNotReturnCriteria(t *testing.T) {
	s, _, r, _ := serviceFixture(t)
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": r.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["claimed"] != false || got["criteria"] != nil {
		t.Fatalf("unclaimed response=%v", got)
	}
}

func TestServiceRunInspectRemainsLeadOnly(t *testing.T) {
	s, _, r, _ := serviceFixture(t)
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "run.inspect", map[string]any{"run_id": r.ID}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("worker inspect error=%v", err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "run.inspect", map[string]any{"run_id": r.ID}); err != nil {
		t.Fatalf("lead inspect error=%v", err)
	}
}

func TestOperatorInspectReturnsExecutionSubjects(t *testing.T) {
	s, _, run, _ := serviceFixture(t)
	got, err := s.OperatorInspect(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	subjects, ok := got["subjects"].([]Subject)
	if !ok || len(subjects) != 1 || subjects[0].Type != "iteration" || subjects[0].ExternalID != "target" {
		t.Fatalf("subjects = %#v", got["subjects"])
	}
}

func TestSummaryAgentSubmitsEvidenceScopedImprovementProposal(t *testing.T) {
	s, js, run, _ := serviceFixture(t)
	if _, err := js.db.Exec(`UPDATE judge_runs SET status='summarizing',last_error='summary claimed by lead-it' WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	subjects, err := js.ListSubjects(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft := improvement.ProposalDraft{
		SubjectIDs: []string{subjects[0].ID},
		Target:     improvement.Target{Repository: "production-agent-images", BaseCommit: "91ab820", Image: "reviewer", ImageDigest: "sha256:image"},
		Findings:   []improvement.Finding{{Severity: "important", Criterion: "review-completeness", Observation: "CI was not checked", Evidence: []improvement.Citation{{BundleHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Artifact: "transcript", Locator: "req-17"}}}},
		Changes:    []improvement.Change{{File: "skills/code-review/SKILL.md", Intent: "Require current CI state"}},
		Acceptance: []string{"Reviewer records current CI state"}, Risk: "medium", RollbackImage: "reviewer:v7",
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingImprovements{}
	s.improvements = recorder
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "improvement.submit", map[string]any{"run_id": run.ID, "result": body}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("worker submit error = %v", err)
	}
	result, err := s.AgentAction(context.Background(), "lead", "lead-it", "improvement.submit", map[string]any{"run_id": run.ID, "result": body})
	if err != nil {
		t.Fatal(err)
	}
	if result["proposal"].(improvement.Proposal).ID != "proposal-1" || recorder.request.CreatorAgent != "lead" || recorder.request.CreatorIteration != "lead-it" || recorder.request.JudgeRunID != run.ID {
		t.Fatalf("proposal result=%+v request=%+v", result, recorder.request)
	}
	if _, err := js.db.Exec(`UPDATE judge_runs SET status='completed',last_error='' WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := js.db.Exec(`INSERT INTO judge_summaries(id,run_id,version,summary_agent,summary_iteration,coverage_json,result_json,raw_submission,created_at) VALUES('summary-1',?,1,'lead','lead-it','[]','{}','{}','2026-08-28T12:00:00Z')`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "improvement.submit", map[string]any{"run_id": run.ID, "result": body}); err != nil {
		t.Fatalf("completed summary submit error = %v", err)
	}
}

func TestServiceIterationsSearchAuthorizesJudgeGroupAndPreservesTargetGroupFilter(t *testing.T) {
	s, _, _, _ := serviceFixture(t)
	if _, err := s.store.db.Exec(`UPDATE agents SET "group"='targets' WHERE name='target-agent'`); err != nil {
		t.Fatal(err)
	}
	seedTarget(t, s.store.db, "other-target", "other-target-agent", "done", "2026-07-01T11:00:00Z")
	if _, err := s.store.db.Exec(`UPDATE agents SET "group"='other-targets' WHERE name='other-target-agent'`); err != nil {
		t.Fatal(err)
	}

	got, err := s.AgentAction(context.Background(), "lead", "lead-it", "iterations.search", map[string]any{
		"judge_group": "judges",
		"selector":    map[string]any{"group": "targets"},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterations := got["iterations"].([]map[string]any)
	if len(iterations) != 1 || iterations[0]["id"] != "target" {
		t.Fatalf("filtered iterations=%v", iterations)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "iterations.search", map[string]any{"group": "judges"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("top-level group authorized search: %v", err)
	}
	if _, err := s.AgentAction(context.Background(), "other", "other-it", "iterations.search", map[string]any{"judge_group": "judges"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("non-lead authorized search: %v", err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "iterations.search", map[string]any{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing judge group authorized search: %v", err)
	}
}

func TestServiceRejectsDisabledCapability(t *testing.T) {
	s, _, r, _ := serviceFixture(t)
	if _, err := s.store.db.Exec(`UPDATE agents SET plugins='[]' WHERE name='judge'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": r.ID}); !errors.Is(err, ErrCapabilityDisabled) {
		t.Fatalf("capability error=%v", err)
	}
}

func TestOperatorReviewCreatesExplicitRunFromDisabledAutomation(t *testing.T) {
	db, js := newJudgeStore(t)
	for _, name := range []string{"lead", "worker-1", "worker-2"} {
		seedJudgeAgent(t, db.DB, name)
	}
	seedTarget(t, db.DB, "target-1", "target-agent", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "not-selected", "target-agent", "done", "2026-07-01T11:00:00Z")
	config := `{"schema_version":1,"enabled":false,"judge":{"lead":"lead","workers":["worker-1","worker-2"],"image_ref":"judge:test"},"schedule":{"spec":"0 * * * *"},"targets":{"agents":["target-agent"],"image_refs":["test"],"only_unprocessed":true}}`
	if _, err := js.SaveAutomation(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	enqueued := ""
	s := NewService(ServiceConfig{Store: js, Enqueue: func(id string) { enqueued = id }})

	run, targets, err := s.OperatorReview(context.Background(), []string{"target-1"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if run.LeadAgent != "lead" || run.SummaryAgent != "lead" || run.JudgeGroup != "judges" || run.JudgesPerIteration != 2 || enqueued != run.ID {
		t.Fatalf("run=%+v enqueued=%q", run, enqueued)
	}
	if len(targets) != 1 || targets[0].Iteration != "target-1" || len(run.Spec.Agents) != 0 || len(run.Spec.ImageRefs) != 0 || run.Spec.OnlyUnprocessed {
		t.Fatalf("targets=%+v selector=%+v", targets, run.Spec)
	}
	criteria, hash, err := ReviewCriteria()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.OriginalRequest, criteria) || !strings.Contains(run.OriginalRequest, "Rubric SHA-256: "+hash) {
		t.Fatalf("request does not freeze criteria: %q", run.OriginalRequest)
	}
}

func TestOperatorReviewRejectsInvalidInputWithoutSideEffects(t *testing.T) {
	db, js := newJudgeStore(t)
	for _, name := range []string{"lead", "worker-1", "worker-2"} {
		seedJudgeAgent(t, db.DB, name)
	}
	seedTarget(t, db.DB, "done", "target-agent", "done", "2026-07-01T10:00:00Z")
	seedTarget(t, db.DB, "running", "target-agent", "running", "2026-07-01T11:00:00Z")
	config := `{"schema_version":1,"enabled":false,"judge":{"lead":"lead","workers":["worker-1","worker-2"],"image_ref":"judge:test"},"schedule":{"spec":"0 * * * *"},"targets":{"agents":[],"image_refs":[],"only_unprocessed":false}}`
	if _, err := js.SaveAutomation(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	s := NewService(ServiceConfig{Store: js})

	cases := []struct {
		name string
		ids  []string
		n    int
	}{
		{"empty", nil, 1},
		{"zero judges", []string{"done"}, 0},
		{"too many judges", []string{"done"}, 3},
		{"unknown iteration", []string{"missing"}, 1},
		{"nonterminal iteration", []string{"running"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := s.OperatorReview(context.Background(), tc.ids, tc.n); err == nil {
				t.Fatal("review succeeded")
			}
			var runs int
			if err := db.DB.QueryRow(`SELECT COUNT(*) FROM judge_runs`).Scan(&runs); err != nil || runs != 0 {
				t.Fatalf("runs=%d err=%v", runs, err)
			}
		})
	}
	if _, err := db.DB.Exec(`UPDATE agents SET plugins='[]' WHERE name='worker-2'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OperatorReview(context.Background(), []string{"done"}, 1); err == nil {
		t.Fatal("ineligible configured worker accepted")
	}
	var runs int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM judge_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("runs=%d err=%v", runs, err)
	}
}
