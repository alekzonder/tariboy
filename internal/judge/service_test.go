package judge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/groups"
)

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
