package judge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/improvement"
)

type recordingImprovements struct {
	request improvement.CreateProposalRequest
}

func seedJudgeImages(t *testing.T, js *Store, names ...string) (*image.Store, image.Manifest) {
	t.Helper()
	images := &image.Store{Dir: t.TempDir()}
	manifest, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Plugins: []imagefile.V2Plugin{{Name: "llm-as-judge"}}, Prompts: []imagefile.PromptEntry{{Runtime: "identity"}}}, imagefile.ResolveRoots{}, image.Ref{Name: "effective-judge", Tag: "latest"}, images, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if _, err := js.db.Exec(`UPDATE agents SET image_ref='effective-judge:latest',image_digest=? WHERE name=?`, manifest.Digest, name); err != nil {
			t.Fatal(err)
		}
	}
	return images, manifest
}

func TestImageIdentityFreezesActiveSnapshotForManualAndAgentRuns(t *testing.T) {
	s, js, _, _ := serviceFixture(t)
	images, first := seedJudgeImages(t, js, "judge")
	s.images = images
	if _, err := js.SaveAutomation(context.Background(), `{"judge":{"lead":"lead","workers":["judge"],"image_ref":"old-label:v1"}}`); err != nil {
		t.Fatal(err)
	}
	// Rebuilding the mutable tag must not mix active digest A with template B.
	if _, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Prompts: []imagefile.PromptEntry{{Runtime: "messages"}}}, imagefile.ResolveRoots{}, image.Ref{Name: "effective-judge", Tag: "latest"}, images, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	manual, _, err := s.OperatorReview(context.Background(), []string{"target"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.BuildV2Mutable(&imagefile.V2{SchemaVersion: 2, Prompts: []imagefile.PromptEntry{{Runtime: "context"}}}, imagefile.ResolveRoots{}, image.Ref{Name: "effective-judge", Tag: "latest"}, images, time.Now, nil); err != nil {
		t.Fatal(err)
	}
	result, err := s.AgentAction(context.Background(), "lead", "lead-it", "run.create", map[string]any{
		"original_request": "verify", "judge_group": "judges", "summary_agent": "lead",
		"judge_agents": []string{"judge"}, "selector": Selector{ExplicitIDs: []string{"target"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []JudgeImageIdentity{{Agent: "judge", ImageRef: "effective-judge:latest", ImageDigest: first.Digest, PromptTemplateSHA256: first.PromptTemplateSHA256}}
	for _, run := range []Run{manual, result["run"].(Run)} {
		stored, err := js.GetRun(run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(run.JudgeImages, want) || !reflect.DeepEqual(stored.JudgeImages, want) {
			t.Fatalf("identity run=%+v stored=%+v want=%+v", run.JudgeImages, stored.JudgeImages, want)
		}
	}
}

func TestImageIdentityClaimAndSubmitUseWorkerIteration(t *testing.T) {
	s, js, run, _ := serviceFixture(t)
	identity := []JudgeImageIdentity{{Agent: "judge", ImageRef: "judge:v1", ImageDigest: strings.Repeat("a", 64), PromptTemplateSHA256: strings.Repeat("c", 64)}}
	raw, _ := json.Marshal(identity)
	if _, err := js.db.Exec(`UPDATE judge_runs SET judge_images_json=? WHERE id=?`, string(raw), run.ID); err != nil {
		t.Fatal(err)
	}
	setIteration := func(ref, digest, hash string) {
		t.Helper()
		if _, err := js.db.Exec(`UPDATE iterations SET image_ref=?,image_digest=?,prompt_template_sha256=? WHERE id='judge-it'`, ref, digest, hash); err != nil {
			t.Fatal(err)
		}
	}
	for _, mismatch := range []JudgeImageIdentity{
		{ImageRef: "judge:v1", ImageDigest: strings.Repeat("b", 64), PromptTemplateSHA256: identity[0].PromptTemplateSHA256},
		{ImageRef: "judge:v2", ImageDigest: identity[0].ImageDigest, PromptTemplateSHA256: identity[0].PromptTemplateSHA256},
		{ImageRef: "judge:v1", ImageDigest: identity[0].ImageDigest, PromptTemplateSHA256: strings.Repeat("d", 64)},
	} {
		setIteration(mismatch.ImageRef, mismatch.ImageDigest, mismatch.PromptTemplateSHA256)
		if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": run.ID}); !errors.Is(err, ErrImageMismatch) {
			t.Fatalf("mismatched claim: %v", err)
		}
		inspected, err := s.OperatorInspect(run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(inspected["run"].(Run).LastError, "new run") {
			t.Fatalf("mismatch is not visible to operator: %+v", inspected["run"])
		}
	}
	setIteration(identity[0].ImageRef, identity[0].ImageDigest, identity[0].PromptTemplateSHA256)
	claimed, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": run.ID})
	if err != nil {
		t.Fatal(err)
	}
	a := claimed["assignment"].(Assignment)
	setIteration("judge:v1", strings.Repeat("b", 64), identity[0].PromptTemplateSHA256)
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "analysis.submit", map[string]any{"assignment_id": a.ID, "result": validAnalysis()}); !errors.Is(err, ErrImageMismatch) || !strings.Contains(err.Error(), "new run") {
		t.Fatalf("mismatched submit: %v", err)
	}
	setIteration(identity[0].ImageRef, identity[0].ImageDigest, identity[0].PromptTemplateSHA256)
	// Current agent configuration is not the execution snapshot.
	if _, err := js.db.Exec(`UPDATE agents SET image_digest=? WHERE name='judge'`, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	result := AnalysisResult{SchemaVersion: 1, Verdict: "uncertain", Score: 0, Confidence: 0, Summary: "No usable evidence", EvidenceGaps: []string{"missing evidence"}}
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "analysis.submit", map[string]any{"assignment_id": a.ID, "result": result}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "summary.claim", map[string]any{"run_id": run.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "summary.inputs", map[string]any{"run_id": run.ID}); err != nil {
		t.Fatalf("mismatch diagnostic blocked valid summary: %v", err)
	}
	setIteration("judge:v1", strings.Repeat("b", 64), identity[0].PromptTemplateSHA256)
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": run.ID}); !errors.Is(err, ErrImageMismatch) {
		t.Fatalf("late mismatched claim: %v", err)
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "summary.inputs", map[string]any{"run_id": run.ID}); err != nil {
		t.Fatalf("late mismatch overwrote summary lease: %v", err)
	}
}

func TestImageIdentityMismatchedPoolMemberDoesNotBlockCompatibleWorker(t *testing.T) {
	s, js, run, _ := serviceFixture(t)
	identity := []JudgeImageIdentity{
		{Agent: "judge", ImageRef: "judge:v1", ImageDigest: strings.Repeat("a", 64), PromptTemplateSHA256: strings.Repeat("c", 64)},
		{Agent: "other", ImageRef: "judge:v2", ImageDigest: strings.Repeat("b", 64), PromptTemplateSHA256: strings.Repeat("d", 64)},
	}
	raw, _ := json.Marshal(identity)
	if _, err := js.db.Exec(`UPDATE judge_runs SET judge_agents_json='["judge","other"]',judge_images_json=? WHERE id=?`, string(raw), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := js.db.Exec(`UPDATE iterations SET image_ref=?,image_digest=?,prompt_template_sha256=? WHERE id='other-it'`, identity[1].ImageRef, identity[1].ImageDigest, identity[1].PromptTemplateSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAction(context.Background(), "judge", "judge-it", "work.claim", map[string]any{"run_id": run.ID}); !errors.Is(err, ErrImageMismatch) {
		t.Fatalf("bad pool member: %v", err)
	}
	got, err := s.AgentAction(context.Background(), "other", "other-it", "work.claim", map[string]any{"run_id": run.ID})
	if err != nil || got["claimed"] != true {
		t.Fatalf("compatible pool member blocked: %+v %v", got, err)
	}
	stored, err := js.GetRun(run.ID)
	if err != nil || stored.Status != RunRunning || stored.JudgesPerIteration != 1 {
		t.Fatalf("run stopped or count changed: %+v %v", stored, err)
	}
}

func TestImageIdentityConcurrentClaimsPreserveSingleAnalysis(t *testing.T) {
	s, js, run, _ := serviceFixture(t)
	identities := []JudgeImageIdentity{{Agent: "judge", ImageRef: "judge:v1", ImageDigest: strings.Repeat("a", 64), PromptTemplateSHA256: strings.Repeat("c", 64)}}
	identities = append(identities, identities[0])
	identities[1].Agent = "other"
	raw, _ := json.Marshal(identities)
	if _, err := js.db.Exec(`UPDATE judge_runs SET judge_agents_json='["judge","other"]',judge_images_json=? WHERE id=?`, string(raw), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := js.db.Exec(`UPDATE iterations SET image_ref=?,image_digest=?,prompt_template_sha256=? WHERE id IN ('judge-it','other-it')`, identities[0].ImageRef, identities[0].ImageDigest, identities[0].PromptTemplateSHA256); err != nil {
		t.Fatal(err)
	}
	type claimResult struct {
		claimed bool
		err     error
	}
	results := make(chan claimResult, 2)
	for _, name := range []string{"judge", "other"} {
		go func() {
			got, err := s.AgentAction(context.Background(), name, name+"-it", "work.claim", map[string]any{"run_id": run.ID})
			results <- claimResult{claimed: got["claimed"] == true, err: err}
		}()
	}
	claimed := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.claimed {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("single-analysis run issued %d claims", claimed)
	}
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

func TestIterationJudgeProjectionKeepsLatestCompletedUnderActiveReview(t *testing.T) {
	zero, pointEight := 0.0, 0.8
	reviews := []IterationJudgeReview{
		{RunID: "pending", CreatedAt: "2026-07-01T13:00:00Z", Pending: 1},
		{RunID: "complete", CreatedAt: "2026-07-01T12:00:00Z", Score: &pointEight, Completed: 1},
		{RunID: "zero", CreatedAt: "2026-07-01T11:00:00Z", Score: &zero, Completed: 1},
	}
	got := ProjectIterationJudgeReviews(reviews)
	if got.LatestCompleted == nil || got.LatestCompleted.Score == nil || *got.LatestCompleted.Score != 0.8 {
		t.Fatalf("latest completed = %+v", got.LatestCompleted)
	}
	if got.Active == nil || got.Active.RunID != "pending" || got.Active.Pending != 1 {
		t.Fatalf("active = %+v", got.Active)
	}
	if onlyZero := ProjectIterationJudgeReviews(reviews[2:]); onlyZero.LatestCompleted == nil || onlyZero.LatestCompleted.Score == nil || *onlyZero.LatestCompleted.Score != 0 {
		t.Fatalf("zero projection = %+v", onlyZero)
	}
	if empty := ProjectIterationJudgeReviews(nil); empty.LatestCompleted != nil || empty.Active != nil {
		t.Fatalf("empty projection = %+v", empty)
	}
	partial := IterationJudgeReview{RunID: "partial", CreatedAt: "2026-07-01T14:00:00Z", State: "terminal", Failed: 1}
	if terminal := ProjectIterationJudgeReviews([]IterationJudgeReview{partial, reviews[1]}); terminal.Active != nil || terminal.LatestCompleted == nil || terminal.LatestCompleted.RunID != "complete" {
		t.Fatalf("terminal partial projection = %+v", terminal)
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
	images, _ := seedJudgeImages(t, js, "worker-1", "worker-2")
	s := NewService(ServiceConfig{Store: js, Images: images, Enqueue: func(id string) { enqueued = id }})

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
	images, _ := seedJudgeImages(t, js, "worker-1", "worker-2")
	s := NewService(ServiceConfig{Store: js, Images: images})

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

func TestImageIdentityPublicCreateRejectsUnverifiableWorkers(t *testing.T) {
	s, js, _, _ := serviceFixture(t)
	config := `{"schema_version":1,"judge":{"lead":"lead","workers":["judge"]}}`
	if _, err := js.SaveAutomation(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	enqueued := 0
	s.enqueue = func(string) { enqueued++ }
	before, err := js.ListRuns(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OperatorReview(context.Background(), []string{"target"}, 1); err == nil {
		t.Error("manual run accepted unverifiable worker image")
	}
	if _, err := s.AgentAction(context.Background(), "lead", "lead-it", "run.create", map[string]any{
		"original_request": "verify", "judge_group": "judges", "summary_agent": "lead",
		"judge_agents": []string{"judge"}, "selector": Selector{ExplicitIDs: []string{"target"}},
	}); err == nil {
		t.Error("agent run accepted unverifiable worker image")
	}
	after, err := js.ListRuns(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) || enqueued != 0 {
		t.Fatalf("unverifiable runs persisted or enqueued: runs %d -> %d, enqueue %d", len(before), len(after), enqueued)
	}
}
